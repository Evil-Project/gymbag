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
	"strings"
	"sync"
	"time"

	"gymbag/internal/config"
)

// RemoteRuleSourceStatus reports runtime state for a single remote URL, rule file, or rules directory.
type RemoteRuleSourceStatus struct {
	URL       string    `json:"url"`
	Kind      string    `json:"kind,omitempty"` // "url", "file", or "dir"
	Name      string    `json:"name,omitempty"`
	Dynamic   bool      `json:"dynamic,omitempty"`
	Files     int       `json:"files,omitempty"`
	Rules     int       `json:"rules"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	LastError string    `json:"last_error,omitempty"`
	FromCache bool      `json:"from_cache,omitempty"`
}

// RemoteRulesStatus summarizes all configured external rule sources.
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

// SiteRuleProvider resolves CustomRules for a site from both RuleSources and legacy RemoteRules.
type SiteRuleProvider interface {
	RemoteRuleProvider
	RulesForSite(wafCfg config.SiteWAFConfig) []config.CustomRule
}

// RemoteRuleManagerOptions configures the external firewall rules loader, watcher, and cache.
type RemoteRuleManagerOptions struct {
	CacheDir           string
	BaseDir            string
	RefreshInterval    time.Duration
	LocalWatchInterval time.Duration
	URLs               []string
	RuleSources        []config.RuleSource
	HTTPClient         *http.Client
	Logger             *slog.Logger
	OnUpdate           func()
}

type remoteSourceState struct {
	etag         string
	lastModified string
	bodyHash     [32]byte
	hasHash      bool
	rules        []config.CustomRule
	status       RemoteRuleSourceStatus
}

// RemoteRuleManager loads, validates, caches, watches, and refreshes external firewall rule sources
// (remote URLs, dedicated rule files, and rule directories) in Gymbagfile or JSON format.
type RemoteRuleManager struct {
	cacheDir   string
	httpClient *http.Client
	logger     *slog.Logger

	mu                 sync.RWMutex
	baseDir            string
	refreshInterval    time.Duration
	localWatchInterval time.Duration
	urls               []string
	ruleSources        []config.RuleSource
	states             map[string]*remoteSourceState
	lastRefresh        time.Time
	onUpdate           func()
}

// NewRemoteRuleManager creates a RemoteRuleManager, warm-loads cached URLs, and loads local rule files/folders synchronously.
func NewRemoteRuleManager(opts RemoteRuleManagerOptions) *RemoteRuleManager {
	interval := opts.RefreshInterval
	if interval <= 0 {
		interval = time.Duration(config.DefaultRemoteRulesRefresh) * time.Second
	}
	localInterval := opts.LocalWatchInterval
	if localInterval <= 0 {
		localInterval = time.Duration(config.DefaultRulesWatchInterval) * time.Second
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	m := &RemoteRuleManager{
		cacheDir:           opts.CacheDir,
		baseDir:            opts.BaseDir,
		httpClient:         client,
		logger:             opts.Logger,
		refreshInterval:    interval,
		localWatchInterval: localInterval,
		states:             make(map[string]*remoteSourceState),
		onUpdate:           opts.OnUpdate,
	}
	if len(opts.RuleSources) > 0 {
		m.SetRuleSources(opts.BaseDir, opts.RuleSources)
	}
	if len(opts.URLs) > 0 {
		m.SetURLs(opts.URLs)
	}
	return m
}

// SetOnUpdate sets the callback invoked whenever external rules change after a refresh or file reload.
func (m *RemoteRuleManager) SetOnUpdate(fn func()) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.onUpdate = fn
	m.mu.Unlock()
}

// SetRefreshInterval updates the background HTTP refresh interval.
func (m *RemoteRuleManager) SetRefreshInterval(d time.Duration) {
	if m == nil || d <= 0 {
		return
	}
	m.mu.Lock()
	m.refreshInterval = d
	m.mu.Unlock()
}

// SetLocalWatchInterval updates the polling interval for dynamic local rule files/folders.
func (m *RemoteRuleManager) SetLocalWatchInterval(d time.Duration) {
	if m == nil || d <= 0 {
		return
	}
	m.mu.Lock()
	m.localWatchInterval = d
	m.mu.Unlock()
}

// SetURLs replaces the active list of legacy remote rule URLs, warm-loading disk caches for any new URLs,
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

// SetRuleSources updates baseDir and the active list of RuleSources, immediately loading any local files/folders
// synchronously and warm-loading cached URLs. Returns newly added URL sources that should be fetched via HTTP.
func (m *RemoteRuleManager) SetRuleSources(baseDir string, sources []config.RuleSource) []string {
	if m == nil {
		return nil
	}
	idxBySrc := make(map[string]int, len(sources))
	clean := make([]config.RuleSource, 0, len(sources))
	for _, raw := range sources {
		rs := raw
		rs.Source = strings.TrimSpace(rs.Source)
		rs.Name = strings.TrimSpace(rs.Name)
		if rs.Source == "" {
			continue
		}
		if pos, ok := idxBySrc[rs.Source]; ok {
			if rs.Dynamic {
				clean[pos].Dynamic = true
			}
			if clean[pos].Name == "" && rs.Name != "" {
				clean[pos].Name = rs.Name
			}
			continue
		}
		idxBySrc[rs.Source] = len(clean)
		clean = append(clean, rs)
	}

	m.mu.Lock()
	prevURLs := make(map[string]struct{}, len(m.ruleSources)+len(m.urls))
	for _, rs := range m.ruleSources {
		if rs.IsURL() {
			prevURLs[rs.Source] = struct{}{}
		}
	}
	for _, u := range m.urls {
		prevURLs[u] = struct{}{}
	}
	m.baseDir = baseDir
	m.ruleSources = clean
	var addedURLs []string
	for _, rs := range clean {
		if rs.IsURL() {
			if _, existed := prevURLs[rs.Source]; !existed {
				addedURLs = append(addedURLs, rs.Source)
				if _, hasState := m.states[rs.Source]; !hasState {
					if st := m.loadCacheForURL(rs.Source); st != nil {
						st.status.Dynamic = rs.Dynamic
						st.status.Name = rs.Name
						m.states[rs.Source] = st
					}
				}
			}
		}
	}
	m.mu.Unlock()

	for _, rs := range clean {
		if !rs.IsURL() {
			m.refreshLocalSource(baseDir, rs)
		}
	}
	return addedURLs
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

// RulesForSite returns CustomRules resolved from wafCfg.RuleSources followed by wafCfg.RemoteRules.
func (m *RemoteRuleManager) RulesForSite(wafCfg config.SiteWAFConfig) []config.CustomRule {
	if m == nil || (len(wafCfg.RuleSources) == 0 && len(wafCfg.RemoteRules) == 0) {
		return nil
	}
	keys := make([]string, 0, len(wafCfg.RuleSources)+len(wafCfg.RemoteRules))
	seen := make(map[string]struct{}, len(wafCfg.RuleSources)+len(wafCfg.RemoteRules))
	for _, rs := range wafCfg.RuleSources {
		src := strings.TrimSpace(rs.Source)
		if src == "" {
			continue
		}
		if _, ok := seen[src]; !ok {
			seen[src] = struct{}{}
			keys = append(keys, src)
		}
	}
	for _, u := range wafCfg.RemoteRules {
		trimmed := strings.TrimSpace(u)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; !ok {
			seen[trimmed] = struct{}{}
			keys = append(keys, trimmed)
		}
	}
	return m.RulesForURLs(keys)
}

// Start runs initial refreshes and launches periodic background loops for HTTP rules and dynamic local rule files/folders.
func (m *RemoteRuleManager) Start(ctx context.Context) {
	if m == nil {
		return
	}
	// Remote HTTP URL loop
	go func() {
		if len(m.activeHTTPURLs(true)) > 0 {
			m.refreshHTTP(ctx, true)
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
				if len(m.activeHTTPURLs(false)) > 0 {
					m.refreshHTTP(ctx, false)
				}
			}
		}
	}()

	// Local dynamic rule file / directory watcher loop
	go func() {
		for {
			m.mu.RLock()
			interval := m.localWatchInterval
			m.mu.RUnlock()
			if interval <= 0 {
				interval = time.Duration(config.DefaultRulesWatchInterval) * time.Second
			}
			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				m.pollDynamicLocalSources()
			}
		}
	}()
}

func (m *RemoteRuleManager) pollDynamicLocalSources() {
	m.mu.RLock()
	baseDir := m.baseDir
	var dynamicLocal []config.RuleSource
	for _, rs := range m.ruleSources {
		if !rs.IsURL() && rs.Dynamic {
			dynamicLocal = append(dynamicLocal, rs)
		}
	}
	m.mu.RUnlock()

	if len(dynamicLocal) == 0 {
		return
	}
	var changed bool
	for _, rs := range dynamicLocal {
		if m.refreshLocalSource(baseDir, rs) {
			changed = true
		}
	}
	if changed {
		m.mu.RLock()
		cb := m.onUpdate
		m.mu.RUnlock()
		if cb != nil {
			cb()
		}
		if m.logger != nil {
			m.logger.Info("hot-reloaded dynamic local rules", "sources", len(dynamicLocal))
		}
	}
}

// RefreshNow fetches/reloads all configured URL and local rule sources,
// triggers OnUpdate if any rules changed, and returns the latest RemoteRulesStatus.
func (m *RemoteRuleManager) RefreshNow(ctx context.Context) RemoteRulesStatus {
	if m == nil {
		return RemoteRulesStatus{}
	}
	localChanged := m.refreshAllLocalSources()
	httpChanged := m.refreshHTTP(ctx, true)
	if localChanged && !httpChanged {
		m.mu.RLock()
		cb := m.onUpdate
		m.mu.RUnlock()
		if cb != nil {
			cb()
		}
	}
	return m.Status()
}

func (m *RemoteRuleManager) refreshAllLocalSources() bool {
	m.mu.RLock()
	baseDir := m.baseDir
	var locals []config.RuleSource
	for _, rs := range m.ruleSources {
		if !rs.IsURL() {
			locals = append(locals, rs)
		}
	}
	m.mu.RUnlock()

	var changed bool
	for _, rs := range locals {
		if m.refreshLocalSource(baseDir, rs) {
			changed = true
		}
	}
	return changed
}

func (m *RemoteRuleManager) refreshHTTP(ctx context.Context, includeStatic bool) bool {
	urls := m.activeHTTPURLs(includeStatic)
	if len(urls) == 0 {
		return false
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
	return changed
}

// Status returns a thread-safe snapshot of all configured external rule sources.
func (m *RemoteRuleManager) Status() RemoteRulesStatus {
	if m == nil {
		return RemoteRulesStatus{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	ordered := m.orderedSourcesLocked()
	sources := make([]RemoteRuleSourceStatus, 0, len(ordered))
	total := 0
	for _, rs := range ordered {
		if st, ok := m.states[rs.Source]; ok {
			status := st.status
			if status.Name == "" {
				status.Name = rs.Name
			}
			status.Dynamic = rs.Dynamic
			if status.Kind == "" && rs.IsURL() {
				status.Kind = "url"
			}
			sources = append(sources, status)
			total += len(st.rules)
		} else {
			kind := "file"
			if rs.IsURL() {
				kind = "url"
			}
			sources = append(sources, RemoteRuleSourceStatus{
				URL:     rs.Source,
				Kind:    kind,
				Name:    rs.Name,
				Dynamic: rs.Dynamic,
			})
		}
	}
	return RemoteRulesStatus{
		RefreshIntervalSec: int(m.refreshInterval.Seconds()),
		TotalRules:         total,
		LastRefreshAt:      m.lastRefresh,
		Sources:            sources,
	}
}

func (m *RemoteRuleManager) orderedSourcesLocked() []config.RuleSource {
	seen := make(map[string]struct{}, len(m.ruleSources)+len(m.urls))
	out := make([]config.RuleSource, 0, len(m.ruleSources)+len(m.urls))
	for _, rs := range m.ruleSources {
		if _, ok := seen[rs.Source]; !ok {
			seen[rs.Source] = struct{}{}
			out = append(out, rs)
		}
	}
	for _, u := range m.urls {
		if _, ok := seen[u]; !ok {
			seen[u] = struct{}{}
			out = append(out, config.RuleSource{Source: u, Dynamic: true})
		}
	}
	return out
}

func (m *RemoteRuleManager) activeHTTPURLs(includeStatic bool) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	seen := make(map[string]struct{}, len(m.urls)+len(m.ruleSources))
	var out []string
	for _, rs := range m.ruleSources {
		if !rs.IsURL() {
			continue
		}
		if !includeStatic && !rs.Dynamic {
			continue
		}
		if _, ok := seen[rs.Source]; !ok {
			seen[rs.Source] = struct{}{}
			out = append(out, rs.Source)
		}
	}
	for _, u := range m.urls {
		if _, ok := seen[u]; !ok {
			seen[u] = struct{}{}
			out = append(out, u)
		}
	}
	return out
}

func (m *RemoteRuleManager) refreshLocalSource(baseDir string, rs config.RuleSource) bool {
	rules, files, kind, sum, err := config.LoadRuleSourceFromDisk(rs.Source, baseDir)
	if err != nil {
		if m.logger != nil {
			m.logger.Warn("local rules load failed", "source", rs.Source, "error", err)
		}
		m.mu.Lock()
		if existing, ok := m.states[rs.Source]; ok {
			existing.status.LastError = err.Error()
		} else {
			m.states[rs.Source] = &remoteSourceState{
				status: RemoteRuleSourceStatus{
					URL:       rs.Source,
					Kind:      kind,
					Name:      rs.Name,
					Dynamic:   rs.Dynamic,
					LastError: err.Error(),
				},
			}
		}
		m.mu.Unlock()
		return false
	}

	now := time.Now().UTC()
	m.mu.Lock()
	prev := m.states[rs.Source]
	changed := prev == nil || !prev.hasHash || prev.bodyHash != sum
	m.states[rs.Source] = &remoteSourceState{
		bodyHash: sum,
		hasHash:  true,
		rules:    rules,
		status: RemoteRuleSourceStatus{
			URL:       rs.Source,
			Kind:      kind,
			Name:      rs.Name,
			Dynamic:   rs.Dynamic,
			Files:     len(files),
			Rules:     len(rules),
			UpdatedAt: now,
		},
	}
	m.mu.Unlock()
	return changed
}

func (m *RemoteRuleManager) refreshSingle(ctx context.Context, targetURL string) bool {
	m.mu.RLock()
	prev := m.states[targetURL]
	var etag, lastMod string
	var dyn bool
	var name string
	if prev != nil {
		etag = prev.etag
		lastMod = prev.lastModified
		dyn = prev.status.Dynamic
		name = prev.status.Name
	}
	for _, rs := range m.ruleSources {
		if rs.Source == targetURL {
			dyn = rs.Dynamic
			if name == "" {
				name = rs.Name
			}
			break
		}
	}
	m.mu.RUnlock()

	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, targetURL, nil)
	if err != nil {
		m.recordError(targetURL, err)
		return false
	}
	req.Header.Set("User-Agent", "gymbag/0.2.0")
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
			Kind:      "url",
			Name:      name,
			Dynamic:   dyn,
			Files:     1,
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
			Kind:      "url",
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
			Kind:      "url",
			Files:     1,
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
