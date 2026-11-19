package proxy

import (
	"net/url"
	"strings"
	"sync/atomic"

	"gymbag/internal/config"
	"gymbag/internal/host"
	"gymbag/internal/store"
	"gymbag/internal/waf"
)

// ParsedUpstream holds a pre-parsed upstream URL and derived metadata to avoid per-request url.Parse overhead.
type ParsedUpstream struct {
	Raw           string
	URL           *url.URL
	BasePath      string // trimmed escaped base path (e.g. "" or "/app")
	Hostname      string
	EffectivePort string
}

func parseUpstream(raw string) ParsedUpstream {
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return ParsedUpstream{Raw: raw}
	}
	return ParsedUpstream{
		Raw:           raw,
		URL:           u,
		BasePath:      strings.TrimRight(u.EscapedPath(), "/"),
		Hostname:      u.Hostname(),
		EffectivePort: effectivePort(u),
	}
}

// SiteRoute bundles a website's immutable SiteConfig, pre-parsed upstreams, compiled WAF AccessGate,
// and upstream load-balancer index.
type SiteRoute struct {
	Config    config.SiteConfig
	Gate      waf.AccessGate
	upstreams []ParsedUpstream
	rr        atomic.Uint64
}

// NextUpstream selects the next upstream URL string for this website using lock-free round-robin.
func (r *SiteRoute) NextUpstream() string {
	n := len(r.Config.Upstreams)
	if n == 0 {
		return ""
	}
	if n == 1 {
		return r.Config.Upstreams[0]
	}
	idx := (r.rr.Add(1) - 1) % uint64(n)
	return r.Config.Upstreams[idx]
}

// NextParsedUpstream selects the next pre-parsed upstream using lock-free round-robin.
func (r *SiteRoute) NextParsedUpstream() ParsedUpstream {
	n := len(r.upstreams)
	if n == 0 {
		if len(r.Config.Upstreams) == 0 {
			return ParsedUpstream{}
		}
		return parseUpstream(r.NextUpstream())
	}
	if n == 1 {
		return r.upstreams[0]
	}
	idx := (r.rr.Add(1) - 1) % uint64(n)
	return r.upstreams[idx]
}

type routeSnapshot struct {
	ordered   []*SiteRoute
	exact     map[string]*SiteRoute
	wildcards []*SiteRoute
}

// SiteRouter provides lock-free multi-website lookup and atomic live reconfiguration.
type SiteRouter struct {
	snap atomic.Pointer[routeSnapshot]
}

// NewSiteRouter builds a SiteRouter from sites, rate counter, and clearance token manager.
func NewSiteRouter(sites []config.SiteConfig, counter store.RateCounter, tokens waf.ClearanceManager) *SiteRouter {
	return NewSiteRouterWithProviders(sites, counter, tokens, nil, "verify", nil)
}

// NewSiteRouterWithIntel builds a SiteRouter wired with IP threat & AI crawler intelligence.
func NewSiteRouterWithIntel(
	sites []config.SiteConfig,
	counter store.RateCounter,
	tokens waf.ClearanceManager,
	intel waf.IntelProvider,
	ipSecurityAction string,
) *SiteRouter {
	return NewSiteRouterWithProviders(sites, counter, tokens, intel, ipSecurityAction, nil)
}

// NewSiteRouterWithProviders builds a SiteRouter wired with IP intelligence and remote firewall rules.
func NewSiteRouterWithProviders(
	sites []config.SiteConfig,
	counter store.RateCounter,
	tokens waf.ClearanceManager,
	intel waf.IntelProvider,
	ipSecurityAction string,
	remoteRules waf.RemoteRuleProvider,
) *SiteRouter {
	r := &SiteRouter{}
	r.ReloadWithProviders(sites, counter, tokens, intel, ipSecurityAction, remoteRules)
	return r
}

// Reload atomically replaces the active website routing table without intelligence provider.
func (r *SiteRouter) Reload(sites []config.SiteConfig, counter store.RateCounter, tokens waf.ClearanceManager) {
	r.ReloadWithProviders(sites, counter, tokens, nil, "verify", nil)
}

// ReloadWithIntel atomically replaces the active website routing table and per-site WAF engines.
func (r *SiteRouter) ReloadWithIntel(
	sites []config.SiteConfig,
	counter store.RateCounter,
	tokens waf.ClearanceManager,
	intel waf.IntelProvider,
	ipSecurityAction string,
) {
	r.ReloadWithProviders(sites, counter, tokens, intel, ipSecurityAction, nil)
}

// ReloadWithProviders atomically replaces the active website routing table and per-site WAF engines.
func (r *SiteRouter) ReloadWithProviders(
	sites []config.SiteConfig,
	counter store.RateCounter,
	tokens waf.ClearanceManager,
	intel waf.IntelProvider,
	ipSecurityAction string,
	remoteRules waf.RemoteRuleProvider,
) {
	exact := make(map[string]*SiteRoute, len(sites))
	ordered := make([]*SiteRoute, 0, len(sites))
	var wildcards []*SiteRoute

	for _, sc := range sites {
		parsed := make([]ParsedUpstream, len(sc.Upstreams))
		for i, u := range sc.Upstreams {
			parsed[i] = parseUpstream(u)
		}
		entry := &SiteRoute{
			Config:    sc,
			Gate:      waf.BuildSiteEngineWithProviders(sc, counter, tokens, intel, ipSecurityAction, remoteRules),
			upstreams: parsed,
		}
		ordered = append(ordered, entry)
		norm := host.Normalize(sc.Domain)
		if len(norm) > 2 && norm[:2] == "*." {
			wildcards = append(wildcards, entry)
		} else {
			exact[norm] = entry
		}
	}
	r.snap.Store(&routeSnapshot{
		ordered:   ordered,
		exact:     exact,
		wildcards: wildcards,
	})
}

// Resolve looks up the SiteRoute matching hostHeader. Returns (route, true) if allowed, or (nil, false).
func (r *SiteRouter) Resolve(hostHeader string) (*SiteRoute, bool) {
	route, _, ok := r.ResolveWithHost(hostHeader)
	return route, ok
}

// ResolveWithHost looks up the SiteRoute matching hostHeader and also returns the normalized hostname.
func (r *SiteRouter) ResolveWithHost(hostHeader string) (*SiteRoute, string, bool) {
	s := r.snap.Load()
	if s == nil {
		return nil, "", false
	}
	h := host.FromHeader(hostHeader)
	if h == "" {
		return nil, "", false
	}
	if route, ok := s.exact[h]; ok {
		return route, h, true
	}
	for _, wc := range s.wildcards {
		if host.Covers(wc.Config.Domain, h) {
			return wc, h, true
		}
	}
	return nil, h, false
}

// Sites returns a copy of all currently configured SiteConfigs in order.
func (r *SiteRouter) Sites() []config.SiteConfig {
	s := r.snap.Load()
	if s == nil {
		return nil
	}
	out := make([]config.SiteConfig, len(s.ordered))
	for i, entry := range s.ordered {
		out[i] = entry.Config
	}
	return out
}
