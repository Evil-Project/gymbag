package waf

import (
	"context"
	"net/http"
	"strings"

	"gymbag/internal/config"
	"gymbag/internal/store"
)

func isAllowedHTTPMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

// RequestShapeRule rejects disallowed HTTP verbs, oversized URIs, null bytes, and path traversal.
type RequestShapeRule struct {
	maxTargetLength int
}

// NewRequestShapeRule constructs a RequestShapeRule (default maxTargetLength 8192).
func NewRequestShapeRule(maxTargetLength int) *RequestShapeRule {
	if maxTargetLength <= 0 {
		maxTargetLength = 8192
	}
	return &RequestShapeRule{maxTargetLength: maxTargetLength}
}

func (r *RequestShapeRule) Evaluate(ctx context.Context, req IncomingRequest) *Decision {
	v := newIncomingReqView(&req)
	return r.evaluateView(ctx, &v)
}

func (r *RequestShapeRule) evaluateView(_ context.Context, v *reqView) *Decision {
	if !isAllowedHTTPMethod(v.method) {
		d := Deny("shape", http.StatusForbidden)
		return &d
	}
	target := v.getRawTarget()
	if len(target) > r.maxTargetLength || strings.IndexByte(target, 0) >= 0 || strings.IndexByte(v.path, 0) >= 0 {
		d := Deny("shape", http.StatusForbidden)
		return &d
	}
	rawPath := target
	if q := strings.IndexByte(target, '?'); q >= 0 {
		rawPath = target[:q]
	}
	if hasTraversalFast(rawPath, target) || (v.path != rawPath && hasTraversalFast(v.path, target)) {
		d := Deny("shape", http.StatusForbidden)
		return &d
	}
	return nil
}

// hasTraversalFast checks for ".." path segments, "%2e%2e" (case-insensitive), "..\", and "\.."
// without allocating any slices or lowercased copies.
func hasTraversalFast(pathPart, fullTarget string) bool {
	// Check "/" delimited segments of pathPart for exact ".."
	start := 0
	for i := 0; i <= len(pathPart); i++ {
		if i == len(pathPart) || pathPart[i] == '/' {
			if i-start == 2 && pathPart[start] == '.' && pathPart[start+1] == '.' {
				return true
			}
			start = i + 1
		}
	}

	// Scan fullTarget once for "..\", "\..", and case-insensitive "%2e%2e"
	n := len(fullTarget)
	for i := 0; i < n; i++ {
		c := fullTarget[i]
		if c == '.' && i+2 < n && fullTarget[i+1] == '.' && fullTarget[i+2] == '\\' {
			return true
		}
		if c == '\\' && i+2 < n && fullTarget[i+1] == '.' && fullTarget[i+2] == '.' {
			return true
		}
		if c == '%' && i+5 < n &&
			fullTarget[i+1] == '2' && (fullTarget[i+2] == 'e' || fullTarget[i+2] == 'E') &&
			fullTarget[i+3] == '%' &&
			fullTarget[i+4] == '2' && (fullTarget[i+5] == 'e' || fullTarget[i+5] == 'E') {
			return true
		}
	}
	return false
}

// PrefixAllowRule short-circuits with ALLOW when req.Path starts with any configured prefix.
type PrefixAllowRule struct {
	prefixes []string
}

func NewPrefixAllowRule(prefixes ...string) *PrefixAllowRule {
	return &PrefixAllowRule{prefixes: prefixes}
}

func (r *PrefixAllowRule) Evaluate(ctx context.Context, req IncomingRequest) *Decision {
	v := newIncomingReqView(&req)
	return r.evaluateView(ctx, &v)
}

func (r *PrefixAllowRule) evaluateView(_ context.Context, v *reqView) *Decision {
	for _, p := range r.prefixes {
		if strings.HasPrefix(v.path, p) {
			d := Allow()
			return &d
		}
	}
	return nil
}

// ExactAllowRule short-circuits with ALLOW when req.Path matches an allowed path exactly.
type ExactAllowRule struct {
	paths map[string]struct{}
}

func NewExactAllowRule(paths ...string) *ExactAllowRule {
	m := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		m[p] = struct{}{}
	}
	return &ExactAllowRule{paths: m}
}

func (r *ExactAllowRule) Evaluate(ctx context.Context, req IncomingRequest) *Decision {
	v := newIncomingReqView(&req)
	return r.evaluateView(ctx, &v)
}

func (r *ExactAllowRule) evaluateView(_ context.Context, v *reqView) *Decision {
	if _, ok := r.paths[v.path]; ok {
		d := Allow()
		return &d
	}
	return nil
}

// MethodAllowRule short-circuits with ALLOW when req.Method is in the allowed set.
type MethodAllowRule struct {
	methods map[string]struct{}
}

func NewMethodAllowRule(methods ...string) *MethodAllowRule {
	m := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		m[strings.ToUpper(method)] = struct{}{}
	}
	return &MethodAllowRule{methods: m}
}

func (r *MethodAllowRule) Evaluate(ctx context.Context, req IncomingRequest) *Decision {
	v := newIncomingReqView(&req)
	return r.evaluateView(ctx, &v)
}

func (r *MethodAllowRule) evaluateView(_ context.Context, v *reqView) *Decision {
	if _, ok := r.methods[v.method]; ok {
		d := Allow()
		return &d
	}
	return nil
}

// RateLimitRule enforces per-site per-IP fixed-window rate limits.
type RateLimitRule struct {
	counter       store.RateCounter
	keyPrefix     string
	limit         int
	windowSeconds int
}

func NewRateLimitRule(counter store.RateCounter, namespace string, limit, windowSeconds int) *RateLimitRule {
	prefix := "ip:"
	if namespace != "" {
		prefix = "ip:" + namespace + ":"
	}
	return &RateLimitRule{
		counter:       counter,
		keyPrefix:     prefix,
		limit:         limit,
		windowSeconds: windowSeconds,
	}
}

func (r *RateLimitRule) Evaluate(ctx context.Context, req IncomingRequest) *Decision {
	v := newIncomingReqView(&req)
	return r.evaluateView(ctx, &v)
}

func (r *RateLimitRule) evaluateView(ctx context.Context, v *reqView) *Decision {
	key := r.keyPrefix + v.clientIP
	count, err := r.counter.Increment(ctx, key, r.windowSeconds)
	if err == nil && count > r.limit {
		d := Deny("rate", http.StatusTooManyRequests)
		return &d
	}
	return nil
}

// ThreatIntelRule blocks or challenges known malicious threat-intel IPs across protected sites.
type ThreatIntelRule struct {
	intel      IntelProvider
	action     string // "block" or "verify"
	tokens     ClearanceManager
	cookieName string
}

func NewThreatIntelRule(intel IntelProvider, action string, tokens ClearanceManager, cookieName string) *ThreatIntelRule {
	return &ThreatIntelRule{
		intel:      intel,
		action:     action,
		tokens:     tokens,
		cookieName: cookieName,
	}
}

func (r *ThreatIntelRule) Evaluate(ctx context.Context, req IncomingRequest) *Decision {
	v := newIncomingReqView(&req)
	return r.evaluateView(ctx, &v)
}

func (r *ThreatIntelRule) evaluateView(ctx context.Context, v *reqView) *Decision {
	if r.intel == nil || !v.checkThreatIP(ctx, r.intel) {
		return nil
	}
	if r.action == "block" {
		d := Deny("threat", http.StatusForbidden)
		return &d
	}
	if v.method == http.MethodOptions || isInternalWAFPublicPath(v.path) {
		return nil
	}
	if r.tokens != nil && r.tokens.Valid(v.getCookie(r.cookieName), v.host, v.clientIP) {
		return nil
	}
	d := Challenge()
	return &d
}

// VerificationRule checks clearance cookies and triggers proof-of-work verification
// according to the site's configured VerifyLevel (0=always, 1=only suspicious, 2=only ai, 3=suspicious & ai).
type VerificationRule struct {
	tokens     ClearanceManager
	cookieName string
	level      config.VerifyLevel
	detector   Detector // nil when level == VerifyLevelAlways (0)
}

func NewVerificationRule(tokens ClearanceManager, cookieName string, level config.VerifyLevel, detector Detector) *VerificationRule {
	return &VerificationRule{
		tokens:     tokens,
		cookieName: cookieName,
		level:      level,
		detector:   detector,
	}
}

func (r *VerificationRule) Evaluate(ctx context.Context, req IncomingRequest) *Decision {
	v := newIncomingReqView(&req)
	return r.evaluateView(ctx, &v)
}

func (r *VerificationRule) evaluateView(ctx context.Context, v *reqView) *Decision {
	if r.tokens.Valid(v.getCookie(r.cookieName), v.host, v.clientIP) {
		d := Allow()
		return &d
	}
	switch r.level {
	case config.VerifyLevelAlways:
		d := Challenge()
		return &d
	case config.VerifyLevelOnlySuspicious, config.VerifyLevelOnlyAI, config.VerifyLevelSuspiciousAndAI:
		if r.detector != nil {
			var matched bool
			if fd, ok := r.detector.(fastDetector); ok {
				matched = fd.matchView(ctx, v)
			} else {
				matched = r.detector.Match(ctx, v.toIncoming())
			}
			if matched {
				d := Challenge()
				return &d
			}
		}
		d := Allow()
		return &d
	default:
		d := Challenge()
		return &d
	}
}

// Engine evaluates an ordered chain of Rules until one returns a Decision.
type Engine struct {
	rules []Rule
}

// NewEngine constructs a WAF Engine from an ordered sequence of Rules.
func NewEngine(rules ...Rule) *Engine {
	return &Engine{rules: append([]Rule(nil), rules...)}
}

// Decide evaluates all rules in order and defaults to Allow if none match.
func (e *Engine) Decide(ctx context.Context, req IncomingRequest) Decision {
	v := newIncomingReqView(&req)
	return e.decideView(ctx, &v)
}

// DecideHTTP evaluates all rules directly against *http.Request with minimal allocations.
func (e *Engine) DecideHTTP(ctx context.Context, r *http.Request, hostName, clientIP string) Decision {
	v := newHTTPReqView(r, hostName, clientIP)
	return e.decideView(ctx, &v)
}

func (e *Engine) decideView(ctx context.Context, v *reqView) Decision {
	for _, rule := range e.rules {
		var dec *Decision
		if fr, ok := rule.(fastRule); ok {
			dec = fr.evaluateView(ctx, v)
		} else {
			dec = rule.Evaluate(ctx, v.toIncoming())
		}
		if dec != nil {
			return *dec
		}
	}
	return Allow()
}

// BuildSiteEngine constructs the complete WAF Engine for a specific SiteConfig.
func BuildSiteEngine(site config.SiteConfig, counter store.RateCounter, tokens ClearanceManager) *Engine {
	return BuildSiteEngineWithProviders(site, counter, tokens, nil, "verify", nil)
}

// BuildSiteEngineWithIntel constructs the complete WAF Engine wired with optional IP threat & AI intelligence.
func BuildSiteEngineWithIntel(
	site config.SiteConfig,
	counter store.RateCounter,
	tokens ClearanceManager,
	intel IntelProvider,
	ipSecurityAction string,
) *Engine {
	return BuildSiteEngineWithProviders(site, counter, tokens, intel, ipSecurityAction, nil)
}

// BuildSiteEngineWithProviders constructs the complete WAF Engine wired with IP intelligence and remote rules.
func BuildSiteEngineWithProviders(
	site config.SiteConfig,
	counter store.RateCounter,
	tokens ClearanceManager,
	intel IntelProvider,
	ipSecurityAction string,
	remoteRules RemoteRuleProvider,
) *Engine {
	shapeRule := NewRequestShapeRule(8192)
	acmeAllow := NewPrefixAllowRule("/.well-known/acme-challenge/")

	var resolvedRemote []config.CustomRule
	if remoteRules != nil && len(site.WAF.RemoteRules) > 0 {
		resolvedRemote = remoteRules.RulesForURLs(site.WAF.RemoteRules)
	}
	userRulesCount := len(site.WAF.Rules) + len(resolvedRemote)

	if !site.Protect {
		if userRulesCount > 0 {
			combined := make([]config.CustomRule, 0, userRulesCount)
			combined = append(combined, site.WAF.Rules...)
			combined = append(combined, resolvedRemote...)
			return NewEngine(shapeRule, acmeAllow, NewCustomRulesRule(combined, tokens, site.WAF.CookieName))
		}
		return NewEngine(shapeRule, acmeAllow)
	}

	var preRateRules []config.CustomRule
	var postIntelVerifyRule Rule

	if site.WAF.DefaultRulesEnabled() {
		if userRulesCount == 0 {
			preRateRules = DefaultSecurityRules(false)
			if site.WAF.VerifyLevel != config.VerifyLevelOnlyAI {
				postIntelVerifyRule = NewCustomRulesRule(defaultVerifyRules(), tokens, site.WAF.CookieName)
			}
		} else {
			preRateRules = make([]config.CustomRule, 0, userRulesCount+4)
			preRateRules = append(preRateRules, site.WAF.Rules...)
			preRateRules = append(preRateRules, resolvedRemote...)
			preRateRules = append(preRateRules, DefaultSecurityRules(false)...)
		}
	} else if userRulesCount > 0 {
		preRateRules = make([]config.CustomRule, 0, userRulesCount)
		preRateRules = append(preRateRules, site.WAF.Rules...)
		preRateRules = append(preRateRules, resolvedRemote...)
	}

	var customRule Rule
	if len(preRateRules) > 0 {
		customRule = NewCustomRulesRule(preRateRules, tokens, site.WAF.CookieName)
	}

	aiDetector := NewAIDetector(site.WAF.CustomAIBots, intel)
	suspicionDetector := NewSuspicionDetector(site.WAF.CustomSuspiciousUAs, site.WAF.CustomSuspiciousPaths, aiDetector, intel)

	var activeDetector Detector
	switch site.WAF.VerifyLevel {
	case config.VerifyLevelOnlySuspicious:
		activeDetector = suspicionDetector
	case config.VerifyLevelOnlyAI:
		activeDetector = aiDetector
	case config.VerifyLevelSuspiciousAndAI:
		activeDetector = NewOrDetector(suspicionDetector, aiDetector)
	case config.VerifyLevelAlways:
		activeDetector = nil
	}

	var chain []Rule
	chain = append(chain, shapeRule, acmeAllow)
	if customRule != nil {
		chain = append(chain, customRule)
	}
	chain = append(chain, NewRateLimitRule(counter, site.Domain, site.WAF.RateLimit, site.WAF.RateWindow))
	if intel != nil {
		chain = append(chain, NewThreatIntelRule(intel, ipSecurityAction, tokens, site.WAF.CookieName))
	}
	chain = append(
		chain,
		NewMethodAllowRule(http.MethodOptions),
		NewPrefixAllowRule("/.waf/assets/", "/.waf/v1/assets/"),
		NewExactAllowRule(
			"/.waf/verify",
			"/.waf/v1/challenges",
			"/.waf/v1/verifications",
			"/.waf/v1/session",
		),
	)
	if postIntelVerifyRule != nil {
		chain = append(chain, postIntelVerifyRule)
	}
	chain = append(chain, NewVerificationRule(tokens, site.WAF.CookieName, site.WAF.VerifyLevel, activeDetector))
	return NewEngine(chain...)
}
