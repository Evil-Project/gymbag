package intel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	DefaultInternetDBBaseURL    = "https://internetdb.shodan.io"
	DefaultStopForumSpamBaseURL = "https://api.stopforumspam.org/api"
)

var suspiciousInternetDBTags = map[string]struct{}{
	"compromised": {},
	"malware":     {},
	"botnet":      {},
	"c2":          {},
	"tor":         {},
	"scanner":     {},
	"vpn":         {},
	"proxy":       {},
}

// RFC 5737 documentation ranges & benchmark/CGNAT ranges skipped on live public API lookups.
var nonRoutableDocPrefixes = []netip.Prefix{
	netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1
	netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3
	netip.MustParsePrefix("100.64.0.0/10"),   // CGNAT
	netip.MustParsePrefix("2001:db8::/32"),   // IPv6 documentation
}

type reputationEntry struct {
	flagged   bool
	reason    string
	expiresAt time.Time
}

// ReputationChecker queries free, unauthenticated IP security APIs (Shodan InternetDB & StopForumSpam)
// with bounded TTL caching and automatic fail-open resilience.
type ReputationChecker struct {
	client               *http.Client
	internetDBBaseURL    string
	stopForumSpamBaseURL string
	cacheTTL             time.Duration
	allowTestRanges      bool

	mu       sync.Mutex
	cache    map[netip.Addr]reputationEntry
	inflight map[netip.Addr]chan struct{}
	now      func() time.Time
}

// NewReputationChecker creates a ReputationChecker using client (or a fast 1200ms client if nil).
func NewReputationChecker(client *http.Client, cacheTTL time.Duration) *ReputationChecker {
	if client == nil {
		client = &http.Client{Timeout: 1200 * time.Millisecond}
	}
	if cacheTTL <= 0 {
		cacheTTL = time.Hour
	}
	return &ReputationChecker{
		client:               client,
		internetDBBaseURL:    DefaultInternetDBBaseURL,
		stopForumSpamBaseURL: DefaultStopForumSpamBaseURL,
		cacheTTL:             cacheTTL,
		cache:                make(map[netip.Addr]reputationEntry),
		inflight:             make(map[netip.Addr]chan struct{}),
		now:                  time.Now,
	}
}

// Check queries the cache (or live Shodan InternetDB + StopForumSpam APIs on cache miss)
// and reports whether ipStr is flagged as malicious/suspicious.
func (rc *ReputationChecker) Check(ctx context.Context, ipStr string) (flagged bool, reason string) {
	if rc == nil {
		return false, ""
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(ipStr))
	if err != nil {
		return false, ""
	}
	addr = addr.Unmap()
	if !isPublicRoutableIP(addr, rc.allowTestRanges) {
		return false, ""
	}

	now := rc.now()
	rc.mu.Lock()
	if entry, ok := rc.cache[addr]; ok && entry.expiresAt.After(now) {
		rc.mu.Unlock()
		return entry.flagged, entry.reason
	}
	if waitCh, pending := rc.inflight[addr]; pending {
		rc.mu.Unlock()
		select {
		case <-waitCh:
			rc.mu.Lock()
			entry := rc.cache[addr]
			rc.mu.Unlock()
			return entry.flagged, entry.reason
		case <-ctx.Done():
			return false, ""
		}
	}
	waitCh := make(chan struct{})
	rc.inflight[addr] = waitCh
	rc.mu.Unlock()

	defer func() {
		rc.mu.Lock()
		delete(rc.inflight, addr)
		close(waitCh)
		rc.mu.Unlock()
	}()

	lookupCtx, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)
	defer cancel()

	isBad, why, ok := rc.queryProviders(lookupCtx, addr)
	ttl := rc.cacheTTL
	if !ok {
		// Negative/offline backoff so unreachable network never slows down subsequent requests.
		ttl = 2 * time.Minute
	}

	rc.mu.Lock()
	if len(rc.cache) >= 16384 {
		for k, v := range rc.cache {
			if !v.expiresAt.After(now) {
				delete(rc.cache, k)
			}
		}
		if len(rc.cache) >= 16384 {
			clear(rc.cache)
		}
	}
	rc.cache[addr] = reputationEntry{
		flagged:   isBad,
		reason:    why,
		expiresAt: now.Add(ttl),
	}
	rc.mu.Unlock()

	return isBad, why
}

// CacheSize returns the number of currently cached IP reputation entries.
func (rc *ReputationChecker) CacheSize() int {
	if rc == nil {
		return 0
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return len(rc.cache)
}

func (rc *ReputationChecker) queryProviders(ctx context.Context, addr netip.Addr) (flagged bool, reason string, reachable bool) {
	type result struct {
		flagged bool
		reason  string
		ok      bool
	}
	ch := make(chan result, 2)
	ipText := addr.String()

	// 1. StopForumSpam (supports both IPv4 and IPv6)
	go func() {
		bad, why, ok := rc.queryStopForumSpam(ctx, ipText)
		ch <- result{bad, why, ok}
	}()

	// 2. Shodan InternetDB (supports IPv4 tags: tor, scanner, malware, compromised, vpn, proxy)
	if addr.Is4() && rc.internetDBBaseURL != "" {
		go func() {
			bad, why, ok := rc.queryInternetDB(ctx, ipText)
			ch <- result{bad, why, ok}
		}()
	} else {
		ch <- result{false, "", true}
	}

	anyOK := false
	for i := 0; i < 2; i++ {
		res := <-ch
		if res.ok {
			anyOK = true
		}
		if res.flagged {
			return true, res.reason, true
		}
	}
	return false, "", anyOK
}

func (rc *ReputationChecker) queryStopForumSpam(ctx context.Context, ipText string) (bool, string, bool) {
	if rc.stopForumSpamBaseURL == "" {
		return false, "", true
	}
	reqURL := fmt.Sprintf("%s?ip=%s&json=true", strings.TrimRight(rc.stopForumSpamBaseURL, "/"), url.QueryEscape(ipText))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return false, "", false
	}
	req.Header.Set("User-Agent", "gymbag/0.1.0")
	resp, err := rc.client.Do(req)
	if err != nil {
		return false, "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return false, "", false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return false, "", false
	}
	var parsed struct {
		Success int `json:"success"`
		IP      struct {
			Appears    int     `json:"appears"`
			Frequency  int     `json:"frequency"`
			Confidence float64 `json:"confidence"`
		} `json:"ip"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return false, "", false
	}
	if parsed.Success == 1 && parsed.IP.Appears > 0 && (parsed.IP.Confidence >= 25.0 || parsed.IP.Frequency >= 2) {
		return true, "stopforumspam", true
	}
	return false, "", true
}

func (rc *ReputationChecker) queryInternetDB(ctx context.Context, ipText string) (bool, string, bool) {
	reqURL := fmt.Sprintf("%s/%s", strings.TrimRight(rc.internetDBBaseURL, "/"), url.PathEscape(ipText))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return false, "", false
	}
	req.Header.Set("User-Agent", "gymbag/0.1.0")
	resp, err := rc.client.Do(req)
	if err != nil {
		return false, "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, "", true
	}
	if resp.StatusCode >= 400 {
		return false, "", false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return false, "", false
	}
	var parsed struct {
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return false, "", false
	}
	for _, tag := range parsed.Tags {
		lower := strings.ToLower(strings.TrimSpace(tag))
		if _, match := suspiciousInternetDBTags[lower]; match {
			return true, "internetdb:" + lower, true
		}
	}
	return false, "", true
}

func isPublicRoutableIP(addr netip.Addr, allowTestRanges bool) bool {
	if !addr.IsValid() || addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	if !allowTestRanges {
		for _, p := range nonRoutableDocPrefixes {
			if p.Contains(addr) {
				return false
			}
		}
	}
	return true
}
