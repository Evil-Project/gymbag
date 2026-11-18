package proxy

import (
	"sync/atomic"

	"gymbag/internal/config"
	"gymbag/internal/host"
	"gymbag/internal/store"
	"gymbag/internal/waf"
)

// SiteRoute bundles a website's immutable SiteConfig, compiled WAF AccessGate, and upstream load-balancer index.
type SiteRoute struct {
	Config config.SiteConfig
	Gate   waf.AccessGate
	rr     atomic.Uint64
}

// NextUpstream selects the next upstream URL for this website using lock-free round-robin.
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
	return NewSiteRouterWithIntel(sites, counter, tokens, nil, "verify")
}

// NewSiteRouterWithIntel builds a SiteRouter wired with IP threat & AI crawler intelligence.
func NewSiteRouterWithIntel(
	sites []config.SiteConfig,
	counter store.RateCounter,
	tokens waf.ClearanceManager,
	intel waf.IntelProvider,
	ipSecurityAction string,
) *SiteRouter {
	r := &SiteRouter{}
	r.ReloadWithIntel(sites, counter, tokens, intel, ipSecurityAction)
	return r
}

// Reload atomically replaces the active website routing table without intelligence provider.
func (r *SiteRouter) Reload(sites []config.SiteConfig, counter store.RateCounter, tokens waf.ClearanceManager) {
	r.ReloadWithIntel(sites, counter, tokens, nil, "verify")
}

// ReloadWithIntel atomically replaces the active website routing table and per-site WAF engines.
func (r *SiteRouter) ReloadWithIntel(
	sites []config.SiteConfig,
	counter store.RateCounter,
	tokens waf.ClearanceManager,
	intel waf.IntelProvider,
	ipSecurityAction string,
) {
	exact := make(map[string]*SiteRoute, len(sites))
	var ordered []*SiteRoute
	var wildcards []*SiteRoute

	for _, sc := range sites {
		entry := &SiteRoute{
			Config: sc,
			Gate:   waf.BuildSiteEngineWithIntel(sc, counter, tokens, intel, ipSecurityAction),
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
	s := r.snap.Load()
	if s == nil {
		return nil, false
	}
	h := host.FromHeader(hostHeader)
	if h == "" {
		return nil, false
	}
	if route, ok := s.exact[h]; ok {
		return route, true
	}
	for _, wc := range s.wildcards {
		if host.Covers(wc.Config.Domain, h) {
			return wc, true
		}
	}
	return nil, false
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
