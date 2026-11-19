package waf

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"gymbag/internal/config"
)

// RemoteRuleSourceStatus reports runtime state for a single remote rules URL.
type RemoteRuleSourceStatus struct {
	URL       string    `json:"url"`
	Rules     int       `json:"rules"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	LastError string    `json:"last_error,omitempty"`
	FromCache bool      `json:"from_cache,omitempty"`
}

// RemoteRulesStatus summarizes all configured remote rule sources.
type RemoteRulesStatus struct {
	RefreshIntervalSec int                      `json:"refresh_interval_seconds"`
	TotalRules         int                      `json:"total_rules"`
	LastRefreshAt      time.Time                `json:"last_refresh_at,omitempty"`
	Sources            []RemoteRuleSourceStatus `json:"sources"`
}

// RemoteRuleProvider resolves currently loaded CustomRules for a site's remote rule URLs.
type RemoteRuleProvider interface {
	RulesForURLs(urls []string) []config.CustomRule
}

// RemoteRuleManagerOptions configures the remote firewall rules fetcher and cache.
type RemoteRuleManagerOptions struct {
	CacheDir        string
	RefreshInterval time.Duration
	URLs            []string
	HTTPClient      *http.Client
	Logger          *slog.Logger
	OnUpdate        func()
}

type remoteSourceState struct {
	etag         string
	lastModified string
	bodyHash     [32]byte
	hasHash      bool
	rules        []config.CustomRule
	status       RemoteRuleSourceStatus
}

// RemoteRuleManager fetches, validates, caches, and periodically refreshes remote firewall rule files
// in either Gymbagfile rule syntax or JSON format.
type RemoteRuleManager struct {
	cacheDir   string
	httpClient *http.Client
	logger     *slog.Logger

	mu              sync.RWMutex
	refreshInterval time.Duration
	urls            []string
	states          map[string]*remoteSourceState
	lastRefresh     time.Time
	onUpdate        func()
}

// NewRemoteRuleManager creates a RemoteRuleManager and warm-loads any cached remote rule files from CacheDir.
func NewRemoteRuleManager(opts RemoteRuleManagerOptions) *RemoteRuleManager {
	interval := opts.RefreshInterval
	if interval <= 0 {
		interval = time.Duration(config.DefaultRemoteRulesRefresh) * time.Second
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	m := &RemoteRuleManager{
		cacheDir:        opts.CacheDir,
		httpClient:      client,
		logger:          opts.Logger,
		refreshInterval: interval,
		states:          make(map[string]*remoteSourceState),
		onUpdate:        opts.OnUpdate,
	}
	m.SetURLs(opts.URLs)
	return m
}

// SetOnUpdate sets the callback invoked whenever remote rules change after a refresh.
func (m *RemoteRuleManager) SetOnUpdate(fn func()) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.onUpdate = fn
	m.mu.Unlock()
}

// SetRefreshInterval updates the background refresh interval.
func (m *RemoteRuleManager) SetRefreshInterval(d time.Duration) {
	if m == nil || d <= 0 {
		return
	}
	m.mu.Lock()
	m.refreshInterval = d
	m.mu.Unlock()
}

// SetURLs replaces the active list of watched remote rule URLs, warm-loading disk caches for any new URLs,
// and returns the list of newly added URLs.
func (m *RemoteRuleManager) SetURLs(urls []string) []string {
	if m == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(urls))
	clean := make([]string, 0, len(urls))
	for _, raw := range urls {
		u := strings.TrimSpace(raw)
		if u == "" {
			continue
		}
		if _, ok := seen[u]; !ok {
			seen[u] = struct{}{}
			clean = append(clean, u)
		}
	}

	m.mu.Lock()
	prevSet := make(map[string]struct{}, len(m.urls))
	for _, u := range m.urls {
		prevSet[u] = struct{}{}
	}
	var added []string
	for _, u := range clean {
		if _, existed := prevSet[u]; !existed {
			added = append(added, u)
			if _, hasState := m.states[u]; !hasState {
				if st := m.loadCacheForURL(u); st != nil {
					m.states[u] = st
				}
			}
		}
	}
	m.urls = clean
	m.mu.Unlock()
	return added
}

// RulesForURLs returns a flat ordered slice of validated CustomRules currently loaded for urls.
func (m *RemoteRuleManager) RulesForURLs(urls []string) []config.CustomRule {
	if m == nil || len(urls) == 0 {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	var total int
	for _, u := range urls {
		if st, ok := m.states[strings.TrimSpace(u)]; ok {
			total += len(st.rules)
		}
	}
	if total == 0 {
		return nil
	}
	out := make([]config.CustomRule, 0, total)
	for _, u := range urls {
		if st, ok := m.states[strings.TrimSpace(u)]; ok {
			for _, r := range st.rules {
				out = append(out, r.Clone())
			}
		}
	}
	return out
}

// Start runs an immediate non-blocking refresh followed by periodic background updates until ctx is cancelled.
func (m *RemoteRuleManager) Start(ctx context.Context) {
	if m == nil {
		return
	}
	go func() {
		if len(m.activeURLs()) > 0 {
			m.RefreshNow(ctx)
		}
		for {
			m.mu.RLock()
			interval := m.refreshInterval
			m.mu.RUnlock()
			if interval <= 0 {
				interval = time.Duration(config.DefaultRemoteRulesRefresh) * time.Second
			}
			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				if len(m.activeURLs()) > 0 {
					m.RefreshNow(ctx)
				}
			}
		}
	}()
}

// RefreshNow fetches all configured remote rule URLs concurrently, updates cached ruleset state,
// triggers OnUpdate if any rules changed, and returns the latest RemoteRulesStatus.
func (m *RemoteRuleManager) RefreshNow(ctx context.Context) RemoteRulesStatus {
	if m == nil {
		return RemoteRulesStatus{}
	}
	urls := m.activeURLs()
	if len(urls) == 0 {
		return m.Status()
	}

	sem := make(chan struct{}, 4)
	var (
		wg      sync.WaitGroup
		chgMu   sync.Mutex
		changed bool
	)
	for _, u := range urls {
		wg.Add(1)
		go func(target string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			if m.refreshSingle(ctx, target) {
				chgMu.Lock()
				changed = true
				chgMu.Unlock()
			}
		}(u)
	}
	wg.Wait()

	m.mu.Lock()
	m.lastRefresh = time.Now().UTC()
	cb := m.onUpdate
	m.mu.Unlock()

	if changed && cb != nil {
		cb()
	}

	st := m.Status()
	if m.logger != nil && len(urls) > 0 {
		m.logger.Info("refreshed remote firewall rules", "sources", len(urls), "total_rules", st.TotalRules)
	}
	return st
}

// Status returns a thread-safe snapshot of all configured remote rule sources.
func (m *RemoteRuleManager) Status() RemoteRulesStatus {
	if m == nil {
		return RemoteRulesStatus{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	sources := make([]RemoteRuleSourceStatus, 0, len(m.urls))
	total := 0
	for _, u := range m.urls {
		if st, ok := m.states[u]; ok {
			sources = append(sources, st.status)
			total += len(st.rules)
		} else {
			sources = append(sources, RemoteRuleSourceStatus{URL: u})
		}
	}
	return RemoteRulesStatus{
		RefreshIntervalSec: int(m.refreshInterval.Seconds()),
		TotalRules:         total,
		LastRefreshAt:      m.lastRefresh,
		Sources:            sources,
	}
}

func (m *RemoteRuleManager) activeURLs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return slices.Clone(m.urls)
}

func (m *RemoteRuleManager) refreshSingle(ctx context.Context, targetURL string) bool {
	m.mu.RLock()
	prev := m.states[targetURL]
	var etag, lastMod string
	if prev != nil {
		etag = prev.etag
		lastMod = prev.lastModified
	}
	m.mu.RUnlock()

	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, targetURL, nil)
	if err != nil {
		m.recordError(targetURL, err)
		return false
	}
	req.Header.Set("User-Agent", "gymbag/0.1.0")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastMod != "" {
		req.Header.Set("If-Modified-Since", lastMod)
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		m.recordError(targetURL, err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified && prev != nil {
		m.mu.Lock()
		if cur, ok := m.states[targetURL]; ok {
			cur.status.UpdatedAt = time.Now().UTC()
			cur.status.LastError = ""
			cur.status.FromCache = false
		}
		m.mu.Unlock()
		return false
	}
	if resp.StatusCode >= 400 {
		m.recordError(targetURL, fmt.Errorf("HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode)))
		return false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4 MiB cap
	if err != nil {
		m.recordError(targetURL, err)
		return false
	}

	rules, err := config.ParseRemoteRules(body)
	if err != nil {
		// Fail-safe: keep last known good ruleset and record error.
		m.recordError(targetURL, err)
		return false
	}

	sum := sha256.Sum256(body)
	m.saveCacheForURL(targetURL, body)

	now := time.Now().UTC()
	m.mu.Lock()
	changed := prev == nil || !prev.hasHash || prev.bodyHash != sum
	m.states[targetURL] = &remoteSourceState{
		etag:         resp.Header.Get("ETag"),
		lastModified: resp.Header.Get("Last-Modified"),
		bodyHash:     sum,
		hasHash:      true,
		rules:        rules,
		status: RemoteRuleSourceStatus{
			URL:       targetURL,
			Rules:     len(rules),
			UpdatedAt: now,
		},
	}
	m.mu.Unlock()
	return changed
}

func (m *RemoteRuleManager) recordError(targetURL string, err error) {
	if m.logger != nil {
		m.logger.Warn("remote rules fetch failed", "url", targetURL, "error", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.states[targetURL]; ok {
		existing.status.LastError = err.Error()
		return
	}
	m.states[targetURL] = &remoteSourceState{
		status: RemoteRuleSourceStatus{
			URL:       targetURL,
			LastError: err.Error(),
		},
	}
}

func (m *RemoteRuleManager) cachePath(targetURL string) string {
	sum := sha256.Sum256([]byte(targetURL))
	return filepath.Join(m.cacheDir, hex.EncodeToString(sum[:16])+".cache")
}

func (m *RemoteRuleManager) loadCacheForURL(targetURL string) *remoteSourceState {
	if m.cacheDir == "" {
		return nil
	}
	path := m.cachePath(targetURL)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	rules, err := config.ParseRemoteRules(raw)
	if err != nil {
		return nil
	}
	var modTime time.Time
	if info, statErr := os.Stat(path); statErr == nil {
		modTime = info.ModTime().UTC()
	}
	return &remoteSourceState{
		bodyHash: sha256.Sum256(raw),
		hasHash:  true,
		rules:    rules,
		status: RemoteRuleSourceStatus{
			URL:       targetURL,
			Rules:     len(rules),
			UpdatedAt: modTime,
			FromCache: true,
		},
	}
}

func (m *RemoteRuleManager) saveCacheForURL(targetURL string, raw []byte) {
	if m.cacheDir == "" {
		return
	}
	path := m.cachePath(targetURL)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, raw, 0o600)
}
