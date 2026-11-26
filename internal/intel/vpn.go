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
	DefaultIPQueryBaseURL = "https://api.ipquery.io"
	DefaultIPAPIIsBaseURL = "https://api.ipapi.is"
)

// DefaultVPNIPFeeds lists free, unauthenticated, auto-updated VPN and Tor exit IP/CIDR feeds.
var DefaultVPNIPFeeds = []FeedSource{
	{Name: "x4bnet-vpn-ipv4", URL: "https://raw.githubusercontent.com/X4BNet/lists_vpn/main/output/vpn/ipv4.txt"},
	{Name: "x4bnet-vpn-ipv6", URL: "https://raw.githubusercontent.com/X4BNet/lists_vpn/main/output/vpn/ipv6.txt"},
	{Name: "tor-exit-nodes", URL: "https://check.torproject.org/torbulkexitlist"},
}

type vpnCacheEntry struct {
	flagged   bool
	reason    string
	expiresAt time.Time
}

// VPNChecker queries free, keyless VPN/proxy/Tor detection APIs (ipquery.io and ipapi.is)
// with bounded TTL caching, singleflight deduplication, and fail-open timeouts.
type VPNChecker struct {
	client          *http.Client
	ipQueryBaseURL  string
	ipAPIIsBaseURL  string
	cacheTTL        time.Duration
	allowTestRanges bool

	mu       sync.Mutex
	cache    map[netip.Addr]vpnCacheEntry
	inflight map[netip.Addr]chan struct{}
	now      func() time.Time
}

// NewVPNChecker creates a VPNChecker using client (or a fast 1200ms HTTP client if nil).
func NewVPNChecker(client *http.Client, cacheTTL time.Duration) *VPNChecker {
	if client == nil {
		client = &http.Client{Timeout: 1200 * time.Millisecond}
	}
	if cacheTTL <= 0 {
		cacheTTL = time.Hour
	}
	return &VPNChecker{
		client:         client,
		ipQueryBaseURL: DefaultIPQueryBaseURL,
		ipAPIIsBaseURL: DefaultIPAPIIsBaseURL,
		cacheTTL:       cacheTTL,
		cache:          make(map[netip.Addr]vpnCacheEntry),
		inflight:       make(map[netip.Addr]chan struct{}),
		now:            time.Now,
	}
}

// Check queries the cache (or live ipquery.io + ipapi.is endpoints on miss) and reports
// whether ipStr is identified as a VPN, proxy, or Tor exit node.
func (vc *VPNChecker) Check(ctx context.Context, ipStr string) (flagged bool, reason string) {
	if vc == nil {
		return false, ""
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(ipStr))
	if err != nil {
		return false, ""
	}
	addr = addr.Unmap()
	if !isPublicRoutableIP(addr, vc.allowTestRanges) {
		return false, ""
	}

	now := vc.now()
	vc.mu.Lock()
	if entry, ok := vc.cache[addr]; ok && entry.expiresAt.After(now) {
		vc.mu.Unlock()
		return entry.flagged, entry.reason
	}
	if waitCh, pending := vc.inflight[addr]; pending {
		vc.mu.Unlock()
		select {
		case <-waitCh:
			vc.mu.Lock()
			entry := vc.cache[addr]
			vc.mu.Unlock()
			return entry.flagged, entry.reason
		case <-ctx.Done():
			return false, ""
		}
	}
	waitCh := make(chan struct{})
	vc.inflight[addr] = waitCh
	vc.mu.Unlock()

	defer func() {
		vc.mu.Lock()
		delete(vc.inflight, addr)
		close(waitCh)
		vc.mu.Unlock()
	}()

	lookupCtx, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)
	defer cancel()

	isVPN, why, ok := vc.queryProviders(lookupCtx, addr)
	ttl := vc.cacheTTL
	if !ok {
		ttl = 2 * time.Minute
	}

	vc.mu.Lock()
	if len(vc.cache) >= 16384 {
		for k, v := range vc.cache {
			if !v.expiresAt.After(now) {
				delete(vc.cache, k)
			}
		}
		if len(vc.cache) >= 16384 {
			clear(vc.cache)
		}
	}
	vc.cache[addr] = vpnCacheEntry{
		flagged:   isVPN,
		reason:    why,
		expiresAt: now.Add(ttl),
	}
	vc.mu.Unlock()

	return isVPN, why
}

// CacheSize returns the number of cached VPN/proxy reputation entries.
func (vc *VPNChecker) CacheSize() int {
	if vc == nil {
		return 0
	}
	vc.mu.Lock()
	defer vc.mu.Unlock()
	return len(vc.cache)
}

func (vc *VPNChecker) queryProviders(ctx context.Context, addr netip.Addr) (flagged bool, reason string, reachable bool) {
	type result struct {
		flagged bool
		reason  string
		ok      bool
	}
	ch := make(chan result, 2)
	ipText := addr.String()

	go func() {
		bad, why, ok := vc.queryIPQuery(ctx, ipText)
		ch <- result{bad, why, ok}
	}()
	go func() {
		bad, why, ok := vc.queryIPAPIIs(ctx, ipText)
		ch <- result{bad, why, ok}
	}()

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

func (vc *VPNChecker) queryIPQuery(ctx context.Context, ipText string) (bool, string, bool) {
	if vc.ipQueryBaseURL == "" {
		return false, "", true
	}
	reqURL := fmt.Sprintf("%s/%s", strings.TrimRight(vc.ipQueryBaseURL, "/"), url.PathEscape(ipText))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return false, "", false
	}
	req.Header.Set("User-Agent", "gymbag/0.2.0")
	resp, err := vc.client.Do(req)
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
		Risk struct {
			IsVPN   bool `json:"is_vpn"`
			IsProxy bool `json:"is_proxy"`
			IsTor   bool `json:"is_tor"`
		} `json:"risk"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return false, "", false
	}
	switch {
	case parsed.Risk.IsVPN:
		return true, "ipquery:vpn", true
	case parsed.Risk.IsProxy:
		return true, "ipquery:proxy", true
	case parsed.Risk.IsTor:
		return true, "ipquery:tor", true
	default:
		return false, "", true
	}
}

func (vc *VPNChecker) queryIPAPIIs(ctx context.Context, ipText string) (bool, string, bool) {
	if vc.ipAPIIsBaseURL == "" {
		return false, "", true
	}
	reqURL := fmt.Sprintf("%s/?q=%s", strings.TrimRight(vc.ipAPIIsBaseURL, "/"), url.QueryEscape(ipText))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return false, "", false
	}
	req.Header.Set("User-Agent", "gymbag/0.2.0")
	resp, err := vc.client.Do(req)
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
		IsVPN   bool `json:"is_vpn"`
		IsProxy bool `json:"is_proxy"`
		IsTor   bool `json:"is_tor"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return false, "", false
	}
	switch {
	case parsed.IsVPN:
		return true, "ipapi.is:vpn", true
	case parsed.IsProxy:
		return true, "ipapi.is:proxy", true
	case parsed.IsTor:
		return true, "ipapi.is:tor", true
	default:
		return false, "", true
	}
}
