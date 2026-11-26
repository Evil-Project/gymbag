package intel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// FeedSource describes a remote IP/CIDR feed URL.
type FeedSource struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// DefaultAIIPFeeds lists official, free, unauthenticated AI crawler IP range JSON feeds.
var DefaultAIIPFeeds = []FeedSource{
	{Name: "openai-gptbot", URL: "https://openai.com/gptbot.json"},
	{Name: "openai-searchbot", URL: "https://openai.com/searchbot.json"},
	{Name: "openai-chatgpt-user", URL: "https://openai.com/chatgpt-user.json"},
	{Name: "anthropic-bots", URL: "https://claude.com/crawling/bots.json"},
	{Name: "perplexitybot", URL: "https://www.perplexity.ai/perplexitybot.json"},
	{Name: "perplexity-user", URL: "https://www.perplexity.ai/perplexity-user.json"},
	{Name: "google-user-triggered-agents", URL: "https://developers.google.com/static/crawling/ipranges/user-triggered-agents.json"},
	{Name: "google-user-triggered-fetchers", URL: "https://developers.google.com/static/crawling/ipranges/user-triggered-fetchers-google.json"},
	{Name: "applebot", URL: "https://search.developer.apple.com/applebot.json"},
}

// DefaultThreatIPFeeds lists reputable, free, auto-updated IP security blocklists.
var DefaultThreatIPFeeds = []FeedSource{
	{Name: "ipsum-level3", URL: "https://raw.githubusercontent.com/stamparm/ipsum/master/levels/3.txt"},
	{Name: "spamhaus-drop-v4", URL: "https://www.spamhaus.org/drop/drop_v4.json"},
	{Name: "spamhaus-drop-v6", URL: "https://www.spamhaus.org/drop/dropv6.txt"},
	{Name: "feodotracker-c2", URL: "https://feodotracker.abuse.ch/downloads/ipblocklist.txt"},
}

// FeedStatus reports runtime statistics for an individual feed.
type FeedStatus struct {
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Category  string    `json:"category"` // "threat" or "ai"
	Entries   int       `json:"entries"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	LastError string    `json:"last_error,omitempty"`
	FromCache bool      `json:"from_cache,omitempty"`
}

// Status summarizes the live state of all IP security feeds, AI bot IP lists, VPN lists, and reputation cache.
type Status struct {
	IPSecurityEnabled     bool         `json:"ip_security_enabled"`
	LiveReputationEnabled bool         `json:"live_reputation_enabled"`
	AIIPListEnabled       bool         `json:"ai_ip_list_enabled"`
	VPNDetectionEnabled   bool         `json:"vpn_detection_enabled"`
	RefreshIntervalSec    int          `json:"refresh_interval_seconds"`
	ThreatEntries         int          `json:"threat_entries"`
	AIEntries             int          `json:"ai_entries"`
	VPNEntries            int          `json:"vpn_entries"`
	ReputationCacheSize   int          `json:"reputation_cache_size"`
	VPNCacheSize          int          `json:"vpn_cache_size"`
	LastRefreshAt         time.Time    `json:"last_refresh_at,omitempty"`
	Feeds                 []FeedStatus `json:"feeds"`
}

// ManagerOptions configures the automatic threat-intelligence, VPN/proxy, and AI-bot IP manager.
type ManagerOptions struct {
	CacheDir              string
	IPSecurityEnabled     bool
	LiveReputationEnabled bool
	AIIPListEnabled       bool
	VPNDetectionEnabled   bool
	RefreshInterval       time.Duration
	ExtraThreatFeeds      []string
	ExtraAIFeeds          []string
	ExtraVPNFeeds         []string
	ReplaceThreatFeeds    []FeedSource // optional full override for tests
	ReplaceAIFeeds        []FeedSource // optional full override for tests
	ReplaceVPNFeeds       []FeedSource // optional full override for tests
	HTTPClient            *http.Client
	Reputation            *ReputationChecker
	VPNChecker            *VPNChecker
	Logger                *slog.Logger
}

type feedCacheState struct {
	etag         string
	lastModified string
	prefixes     []netip.Prefix
	status       FeedStatus
}

// Manager maintains lock-free atomic IPSets for malicious IPs, AI robot IPs, and VPN/proxy IPs,
// persists feed snapshots on disk, and automatically updates them on a background schedule.
type Manager struct {
	opts       ManagerOptions
	httpClient *http.Client
	reputation *ReputationChecker
	vpn        *VPNChecker
	logger     *slog.Logger

	threatSources []FeedSource
	aiSources     []FeedSource
	vpnSources    []FeedSource

	threatSet atomic.Pointer[IPSet]
	aiSet     atomic.Pointer[IPSet]
	vpnSet    atomic.Pointer[IPSet]

	mu          sync.Mutex
	feedStates  map[string]*feedCacheState
	lastRefresh time.Time
}

// NewManager constructs a Manager, immediately warm-loading any cached feed snapshots from CacheDir.
func NewManager(opts ManagerOptions) *Manager {
	if opts.RefreshInterval <= 0 {
		opts.RefreshInterval = 6 * time.Hour
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	rep := opts.Reputation
	if rep == nil && opts.LiveReputationEnabled {
		rep = NewReputationChecker(nil, time.Hour)
	}
	vpnChk := opts.VPNChecker
	if vpnChk == nil && opts.VPNDetectionEnabled && opts.LiveReputationEnabled {
		vpnChk = NewVPNChecker(nil, time.Hour)
	}

	var threatSources []FeedSource
	if opts.IPSecurityEnabled {
		if opts.ReplaceThreatFeeds != nil {
			threatSources = append(threatSources, opts.ReplaceThreatFeeds...)
		} else {
			threatSources = append(threatSources, DefaultThreatIPFeeds...)
		}
		for i, u := range opts.ExtraThreatFeeds {
			threatSources = append(threatSources, FeedSource{
				Name: customFeedName("custom-threat", i+1, u),
				URL:  u,
			})
		}
	}

	var aiSources []FeedSource
	if opts.AIIPListEnabled {
		if opts.ReplaceAIFeeds != nil {
			aiSources = append(aiSources, opts.ReplaceAIFeeds...)
		} else {
			aiSources = append(aiSources, DefaultAIIPFeeds...)
		}
		for i, u := range opts.ExtraAIFeeds {
			aiSources = append(aiSources, FeedSource{
				Name: customFeedName("custom-ai", i+1, u),
				URL:  u,
			})
		}
	}

	var vpnSources []FeedSource
	if opts.VPNDetectionEnabled {
		if opts.ReplaceVPNFeeds != nil {
			vpnSources = append(vpnSources, opts.ReplaceVPNFeeds...)
		} else {
			vpnSources = append(vpnSources, DefaultVPNIPFeeds...)
		}
		for i, u := range opts.ExtraVPNFeeds {
			vpnSources = append(vpnSources, FeedSource{
				Name: customFeedName("custom-vpn", i+1, u),
				URL:  u,
			})
		}
	}

	m := &Manager{
		opts:          opts,
		httpClient:    httpClient,
		reputation:    rep,
		vpn:           vpnChk,
		logger:        opts.Logger,
		threatSources: threatSources,
		aiSources:     aiSources,
		vpnSources:    vpnSources,
		feedStates:    make(map[string]*feedCacheState),
	}
	m.threatSet.Store(NewIPSet(nil))
	m.aiSet.Store(NewIPSet(nil))
	m.vpnSet.Store(NewIPSet(nil))

	m.loadDiskCache()
	return m
}

// IsThreatIP reports whether ipStr matches an active threat-intelligence feed or live reputation service.
func (m *Manager) IsThreatIP(ctx context.Context, ipStr string) bool {
	if m == nil || !m.opts.IPSecurityEnabled {
		return false
	}
	if set := m.threatSet.Load(); set != nil && set.ContainsString(ipStr) {
		return true
	}
	if m.opts.LiveReputationEnabled && m.reputation != nil {
		flagged, _ := m.reputation.Check(ctx, ipStr)
		return flagged
	}
	return false
}

// IsThreatFeedIP reports whether ipStr specifically matches a downloaded threat blocklist CIDR/IP.
func (m *Manager) IsThreatFeedIP(ipStr string) bool {
	if m == nil || !m.opts.IPSecurityEnabled {
		return false
	}
	if set := m.threatSet.Load(); set != nil {
		return set.ContainsString(ipStr)
	}
	return false
}

// IsAIIP reports whether ipStr belongs to a known AI crawler / robot IP range.
func (m *Manager) IsAIIP(ipStr string) bool {
	if m == nil || !m.opts.AIIPListEnabled {
		return false
	}
	if set := m.aiSet.Load(); set != nil {
		return set.ContainsString(ipStr)
	}
	return false
}

// IsVPNOrProxyIP reports whether ipStr matches a VPN/proxy/Tor feed or live free VPN detection API.
func (m *Manager) IsVPNOrProxyIP(ctx context.Context, ipStr string) bool {
	if m == nil || !m.opts.VPNDetectionEnabled {
		return false
	}
	if set := m.vpnSet.Load(); set != nil && set.ContainsString(ipStr) {
		return true
	}
	if m.opts.LiveReputationEnabled && m.vpn != nil {
		flagged, _ := m.vpn.Check(ctx, ipStr)
		return flagged
	}
	return false
}

// IsVPNFeedIP reports whether ipStr specifically matches a downloaded VPN/proxy/Tor CIDR/IP feed.
func (m *Manager) IsVPNFeedIP(ipStr string) bool {
	if m == nil || !m.opts.VPNDetectionEnabled {
		return false
	}
	if set := m.vpnSet.Load(); set != nil {
		return set.ContainsString(ipStr)
	}
	return false
}

// Start runs an immediate non-blocking refresh followed by automatic periodic updates until ctx ends.
func (m *Manager) Start(ctx context.Context) {
	if m == nil || (!m.opts.IPSecurityEnabled && !m.opts.AIIPListEnabled && !m.opts.VPNDetectionEnabled) {
		return
	}
	go func() {
		m.RefreshNow(ctx)
		ticker := time.NewTicker(m.opts.RefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.RefreshNow(ctx)
			}
		}
	}()
}

// RefreshNow fetches all configured threat and AI robot feeds concurrently and atomically swaps IPSets.
func (m *Manager) RefreshNow(ctx context.Context) Status {
	if m == nil {
		return Status{}
	}
	type fetchJob struct {
		src      FeedSource
		category string
	}
	var jobs []fetchJob
	for _, s := range m.threatSources {
		jobs = append(jobs, fetchJob{src: s, category: "threat"})
	}
	for _, s := range m.aiSources {
		jobs = append(jobs, fetchJob{src: s, category: "ai"})
	}
	for _, s := range m.vpnSources {
		jobs = append(jobs, fetchJob{src: s, category: "vpn"})
	}

	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, job := range jobs {
		wg.Add(1)
		go func(j fetchJob) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			m.refreshSingleFeed(ctx, j.src, j.category)
		}(job)
	}
	wg.Wait()

	m.rebuildSets()
	st := m.Status()
	if m.logger != nil && (len(m.threatSources) > 0 || len(m.aiSources) > 0 || len(m.vpnSources) > 0) {
		m.logger.Info(
			"updated IP security, VPN, and AI robot lists",
			"threat_entries", st.ThreatEntries,
			"ai_entries", st.AIEntries,
			"vpn_entries", st.VPNEntries,
		)
	}
	return st
}

// Status returns a thread-safe snapshot of all feeds and IPSet counts.
func (m *Manager) Status() Status {
	if m == nil {
		return Status{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	var feeds []FeedStatus
	for _, s := range m.threatSources {
		if st, ok := m.feedStates[stateKey("threat", s.URL)]; ok {
			feeds = append(feeds, st.status)
		} else {
			feeds = append(feeds, FeedStatus{Name: s.Name, URL: s.URL, Category: "threat"})
		}
	}
	for _, s := range m.aiSources {
		if st, ok := m.feedStates[stateKey("ai", s.URL)]; ok {
			feeds = append(feeds, st.status)
		} else {
			feeds = append(feeds, FeedStatus{Name: s.Name, URL: s.URL, Category: "ai"})
		}
	}
	for _, s := range m.vpnSources {
		if st, ok := m.feedStates[stateKey("vpn", s.URL)]; ok {
			feeds = append(feeds, st.status)
		} else {
			feeds = append(feeds, FeedStatus{Name: s.Name, URL: s.URL, Category: "vpn"})
		}
	}

	var threatCount, aiCount, vpnCount int
	if ts := m.threatSet.Load(); ts != nil {
		threatCount = ts.Len()
	}
	if as := m.aiSet.Load(); as != nil {
		aiCount = as.Len()
	}
	if vs := m.vpnSet.Load(); vs != nil {
		vpnCount = vs.Len()
	}

	return Status{
		IPSecurityEnabled:     m.opts.IPSecurityEnabled,
		LiveReputationEnabled: m.opts.LiveReputationEnabled,
		AIIPListEnabled:       m.opts.AIIPListEnabled,
		VPNDetectionEnabled:   m.opts.VPNDetectionEnabled,
		RefreshIntervalSec:    int(m.opts.RefreshInterval.Seconds()),
		ThreatEntries:         threatCount,
		AIEntries:             aiCount,
		VPNEntries:            vpnCount,
		ReputationCacheSize:   m.reputation.CacheSize(),
		VPNCacheSize:          m.vpn.CacheSize(),
		LastRefreshAt:         m.lastRefresh,
		Feeds:                 feeds,
	}
}

func (m *Manager) refreshSingleFeed(ctx context.Context, src FeedSource, category string) {
	key := stateKey(category, src.URL)

	m.mu.Lock()
	prev := m.feedStates[key]
	var etag, lastMod string
	if prev != nil {
		etag = prev.etag
		lastMod = prev.lastModified
	}
	m.mu.Unlock()

	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, src.URL, nil)
	if err != nil {
		m.recordError(key, src, category, err)
		return
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
		m.recordError(key, src, category, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified && prev != nil {
		m.mu.Lock()
		prev.status.UpdatedAt = time.Now().UTC()
		prev.status.LastError = ""
		prev.status.FromCache = false
		m.mu.Unlock()
		return
	}
	if resp.StatusCode >= 400 {
		m.recordError(key, src, category, &httpError{code: resp.StatusCode})
		return
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20)) // 16 MiB cap per feed
	if err != nil {
		m.recordError(key, src, category, err)
		return
	}
	prefixes := ParseFeedPayload(body)
	if len(prefixes) == 0 && prev != nil && len(prev.prefixes) > 0 {
		// Guard against empty captive-portal/truncated responses clobbering good data.
		return
	}

	m.saveDiskCacheFile(src.URL, body)

	now := time.Now().UTC()
	m.mu.Lock()
	m.feedStates[key] = &feedCacheState{
		etag:         resp.Header.Get("ETag"),
		lastModified: resp.Header.Get("Last-Modified"),
		prefixes:     prefixes,
		status: FeedStatus{
			Name:      src.Name,
			URL:       src.URL,
			Category:  category,
			Entries:   len(prefixes),
			UpdatedAt: now,
		},
	}
	m.mu.Unlock()
}

func (m *Manager) recordError(key string, src FeedSource, category string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.feedStates[key]; ok {
		existing.status.LastError = err.Error()
		return
	}
	m.feedStates[key] = &feedCacheState{
		status: FeedStatus{
			Name:      src.Name,
			URL:       src.URL,
			Category:  category,
			LastError: err.Error(),
		},
	}
}

func (m *Manager) loadDiskCache() {
	if m.opts.CacheDir == "" {
		return
	}
	loadGroup := func(sources []FeedSource, category string) {
		for _, src := range sources {
			path := m.cacheFilePath(src.URL)
			raw, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			prefixes := ParseFeedPayload(raw)
			if len(prefixes) == 0 {
				continue
			}
			info, _ := os.Stat(path)
			var modTime time.Time
			if info != nil {
				modTime = info.ModTime().UTC()
			}
			key := stateKey(category, src.URL)
			m.feedStates[key] = &feedCacheState{
				prefixes: prefixes,
				status: FeedStatus{
					Name:      src.Name,
					URL:       src.URL,
					Category:  category,
					Entries:   len(prefixes),
					UpdatedAt: modTime,
					FromCache: true,
				},
			}
		}
	}
	loadGroup(m.threatSources, "threat")
	loadGroup(m.aiSources, "ai")
	loadGroup(m.vpnSources, "vpn")
	m.rebuildSets()
}

func (m *Manager) saveDiskCacheFile(feedURL string, raw []byte) {
	if m.opts.CacheDir == "" {
		return
	}
	path := m.cacheFilePath(feedURL)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, raw, 0o600)
}

func (m *Manager) cacheFilePath(feedURL string) string {
	sum := sha256.Sum256([]byte(feedURL))
	return filepath.Join(m.opts.CacheDir, hex.EncodeToString(sum[:16])+".cache")
}

func (m *Manager) rebuildSets() {
	m.mu.Lock()
	var threatPrefixes []netip.Prefix
	for _, src := range m.threatSources {
		if st, ok := m.feedStates[stateKey("threat", src.URL)]; ok {
			threatPrefixes = append(threatPrefixes, st.prefixes...)
		}
	}
	var aiPrefixes []netip.Prefix
	for _, src := range m.aiSources {
		if st, ok := m.feedStates[stateKey("ai", src.URL)]; ok {
			aiPrefixes = append(aiPrefixes, st.prefixes...)
		}
	}
	var vpnPrefixes []netip.Prefix
	for _, src := range m.vpnSources {
		if st, ok := m.feedStates[stateKey("vpn", src.URL)]; ok {
			vpnPrefixes = append(vpnPrefixes, st.prefixes...)
		}
	}
	m.lastRefresh = time.Now().UTC()
	m.mu.Unlock()

	m.threatSet.Store(NewIPSet(threatPrefixes))
	m.aiSet.Store(NewIPSet(aiPrefixes))
	m.vpnSet.Store(NewIPSet(vpnPrefixes))
}

func stateKey(category, feedURL string) string {
	return category + "|" + feedURL
}

func customFeedName(prefix string, idx int, feedURL string) string {
	sum := sha256.Sum256([]byte(feedURL))
	return prefix + "-" + hex.EncodeToString(sum[:3])
}

type httpError struct {
	code int
}

func (e *httpError) Error() string {
	return http.StatusText(e.code)
}
