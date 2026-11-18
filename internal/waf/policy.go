package waf

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"gymbag/internal/config"
	"gymbag/internal/store"
)

var allowedHTTPMethods = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodPost:    {},
	http.MethodPut:     {},
	http.MethodPatch:   {},
	http.MethodDelete:  {},
	http.MethodHead:    {},
	http.MethodOptions: {},
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

func (r *RequestShapeRule) Evaluate(_ context.Context, req IncomingRequest) *Decision {
	if _, ok := allowedHTTPMethods[strings.ToUpper(req.Method)]; !ok {
		d := Deny("shape", http.StatusForbidden)
		return &d
	}
	target := req.RawTarget
	if target == "" {
		if req.Query != "" {
			target = req.Path + "?" + req.Query
		} else {
			target = req.Path
		}
	}
	if len(target) > r.maxTargetLength || strings.ContainsRune(target, '\x00') || strings.ContainsRune(req.Path, '\x00') {
		d := Deny("shape", http.StatusForbidden)
		return &d
	}
	rawPath, _, _ := strings.Cut(target, "?")
	if hasTraversal(rawPath, target) || hasTraversal(req.Path, target) {
		d := Deny("shape", http.StatusForbidden)
		return &d
	}
	return nil
}

func hasTraversal(pathPart, fullTarget string) bool {
	if slices.Contains(strings.Split(pathPart, "/"), "..") {
		return true
	}
	folded := strings.ToLower(fullTarget)
	return strings.Contains(folded, "%2e%2e") ||
		strings.Contains(fullTarget, `..\`) ||
		strings.Contains(fullTarget, `\..`)
}

// PrefixAllowRule short-circuits with ALLOW when req.Path starts with any configured prefix.
type PrefixAllowRule struct {
	prefixes []string
}

func NewPrefixAllowRule(prefixes ...string) *PrefixAllowRule {
	return &PrefixAllowRule{prefixes: prefixes}
}

func (r *PrefixAllowRule) Evaluate(_ context.Context, req IncomingRequest) *Decision {
	for _, p := range r.prefixes {
		if strings.HasPrefix(req.Path, p) {
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

func (r *ExactAllowRule) Evaluate(_ context.Context, req IncomingRequest) *Decision {
	if _, ok := r.paths[req.Path]; ok {
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

func (r *MethodAllowRule) Evaluate(_ context.Context, req IncomingRequest) *Decision {
	if _, ok := r.methods[strings.ToUpper(req.Method)]; ok {
		d := Allow()
		return &d
	}
	return nil
}

// RateLimitRule enforces per-site per-IP fixed-window rate limits.
type RateLimitRule struct {
	counter       store.RateCounter
	namespace     string
	limit         int
	windowSeconds int
}

func NewRateLimitRule(counter store.RateCounter, namespace string, limit, windowSeconds int) *RateLimitRule {
	return &RateLimitRule{
		counter:       counter,
		namespace:     namespace,
		limit:         limit,
		windowSeconds: windowSeconds,
	}
}

func (r *RateLimitRule) Evaluate(ctx context.Context, req IncomingRequest) *Decision {
	var key string
	if r.namespace != "" {
		key = fmt.Sprintf("ip:%s:%s", r.namespace, req.ClientIP)
	} else {
		key = fmt.Sprintf("ip:%s", req.ClientIP)
	}
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
	if r.intel == nil || !r.intel.IsThreatIP(ctx, req.ClientIP) {
		return nil
	}
	if r.action == "block" {
		d := Deny("threat", http.StatusForbidden)
		return &d
	}
	if strings.EqualFold(req.Method, http.MethodOptions) || isInternalWAFPublicPath(req.Path) {
		return nil
	}
	if r.tokens != nil && r.tokens.Valid(req.Cookies[r.cookieName], req.Host, req.ClientIP) {
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
	if r.tokens.Valid(req.Cookies[r.cookieName], req.Host, req.ClientIP) {
		d := Allow()
		return &d
	}
	switch r.level {
	case config.VerifyLevelAlways:
		d := Challenge()
		return &d
	case config.VerifyLevelOnlySuspicious, config.VerifyLevelOnlyAI, config.VerifyLevelSuspiciousAndAI:
		if r.detector != nil && r.detector.Match(ctx, req) {
			d := Challenge()
			return &d
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
	for _, rule := range e.rules {
		if dec := rule.Evaluate(ctx, req); dec != nil {
			return *dec
		}
	}
	return Allow()
}

// BuildSiteEngine constructs the complete WAF Engine for a specific SiteConfig.
func BuildSiteEngine(site config.SiteConfig, counter store.RateCounter, tokens ClearanceManager) *Engine {
	return BuildSiteEngineWithIntel(site, counter, tokens, nil, "verify")
}

// BuildSiteEngineWithIntel constructs the complete WAF Engine wired with optional IP threat & AI intelligence.
func BuildSiteEngineWithIntel(
	site config.SiteConfig,
	counter store.RateCounter,
	tokens ClearanceManager,
	intel IntelProvider,
	ipSecurityAction string,
) *Engine {
	shapeRule := NewRequestShapeRule(8192)
	acmeAllow := NewPrefixAllowRule("/.well-known/acme-challenge/")

	if !site.Protect {
		if len(site.WAF.Rules) > 0 {
			return NewEngine(shapeRule, acmeAllow, NewCustomRulesRule(site.WAF.Rules, tokens, site.WAF.CookieName))
		}
		return NewEngine(shapeRule, acmeAllow)
	}

	var preRateRules []config.CustomRule
	var postIntelVerifyRule Rule

	if site.WAF.DefaultRulesEnabled() {
		if len(site.WAF.Rules) == 0 {
			preRateRules = DefaultSecurityRules(false)
			if site.WAF.VerifyLevel != config.VerifyLevelOnlyAI {
				postIntelVerifyRule = NewCustomRulesRule(defaultVerifyRules(), tokens, site.WAF.CookieName)
			}
		} else {
			preRateRules = make([]config.CustomRule, 0, len(site.WAF.Rules)+4)
			preRateRules = append(preRateRules, site.WAF.Rules...)
			preRateRules = append(preRateRules, DefaultSecurityRules(false)...)
		}
	} else if len(site.WAF.Rules) > 0 {
		preRateRules = site.WAF.Rules
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
