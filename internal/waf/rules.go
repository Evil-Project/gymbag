package waf

import (
	"context"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"

	"gymbag/internal/config"
	"gymbag/internal/intel"
)

type textMatcher struct {
	re        *regexp.Regexp
	substring string
	exact     string
}

func compileTextMatcher(pattern string, defaultSubstring bool) textMatcher {
	if strings.HasPrefix(pattern, "~") {
		expr := strings.TrimPrefix(pattern, "~")
		if !strings.HasPrefix(expr, "(?") {
			expr = "(?i)" + expr
		}
		if re, err := regexp.Compile(expr); err == nil {
			return textMatcher{re: re}
		}
	}
	if strings.Contains(pattern, "*") {
		re := wildcardToRegexp(pattern, true)
		return textMatcher{re: re}
	}
	if defaultSubstring {
		return textMatcher{substring: strings.ToLower(pattern)}
	}
	return textMatcher{exact: pattern}
}

func (m textMatcher) match(val string) bool {
	if m.re != nil {
		return m.re.MatchString(val)
	}
	if m.substring != "" {
		return strings.Contains(strings.ToLower(val), m.substring)
	}
	return val == m.exact
}

type pathMatcher struct {
	re     *regexp.Regexp
	prefix string
	exact  string
}

func compilePathMatcher(pattern string) pathMatcher {
	if strings.HasPrefix(pattern, "~") {
		expr := strings.TrimPrefix(pattern, "~")
		if re, err := regexp.Compile(expr); err == nil {
			return pathMatcher{re: re}
		}
	}
	if strings.Contains(pattern, "*") {
		return pathMatcher{re: wildcardToRegexp(pattern, false)}
	}
	if len(pattern) > 1 && strings.HasSuffix(pattern, "/") {
		return pathMatcher{prefix: pattern}
	}
	return pathMatcher{exact: pattern}
}

func (m pathMatcher) match(path string) bool {
	if m.re != nil {
		return m.re.MatchString(path)
	}
	if m.prefix != "" {
		trimmed := strings.TrimSuffix(m.prefix, "/")
		return path == trimmed || strings.HasPrefix(path, m.prefix)
	}
	return path == m.exact
}

type kvCompiledMatcher struct {
	name      string
	wantEmpty bool
	wantAny   bool
	valMatch  textMatcher
}

func compileKVMatcher(kv config.KeyValueMatch, lowerName bool) kvCompiledMatcher {
	name := strings.TrimSpace(kv.Name)
	if lowerName {
		name = strings.ToLower(name)
	}
	if kv.Pattern == "" {
		return kvCompiledMatcher{name: name, wantEmpty: true}
	}
	if kv.Pattern == "*" {
		return kvCompiledMatcher{name: name, wantAny: true}
	}
	return kvCompiledMatcher{
		name:     name,
		valMatch: compileTextMatcher(kv.Pattern, true),
	}
}

func (m kvCompiledMatcher) matchMap(values map[string]string) bool {
	actual := strings.TrimSpace(values[m.name])
	if m.wantEmpty {
		return actual == ""
	}
	if actual == "" {
		return false
	}
	if m.wantAny {
		return true
	}
	return m.valMatch.match(actual)
}

func wildcardToRegexp(glob string, caseInsensitive bool) *regexp.Regexp {
	var b strings.Builder
	if caseInsensitive {
		b.WriteString("(?i)")
	}
	b.WriteByte('^')
	for _, ch := range glob {
		if ch == '*' {
			b.WriteString(".*")
		} else {
			b.WriteString(regexp.QuoteMeta(string(ch)))
		}
	}
	b.WriteByte('$')
	re, _ := regexp.Compile(b.String())
	return re
}

type compiledCustomRule struct {
	name       string
	action     config.RuleAction
	ipSet      *intel.IPSet
	userAgents []textMatcher
	paths      []pathMatcher
	queries    []textMatcher
	methods    map[string]struct{}
	headers    []kvCompiledMatcher
	cookies    []kvCompiledMatcher
}

func compileCustomRule(r config.CustomRule) compiledCustomRule {
	out := compiledCustomRule{
		name:   r.Name,
		action: r.Action,
	}
	if len(r.IPs) > 0 {
		prefs := make([]netip.Prefix, 0, len(r.IPs))
		for _, raw := range r.IPs {
			if p, err := intel.ParseIPOrCIDR(raw); err == nil {
				prefs = append(prefs, p)
			}
		}
		out.ipSet = intel.NewIPSet(prefs)
	}
	for _, ua := range r.UserAgents {
		out.userAgents = append(out.userAgents, compileTextMatcher(ua, true))
	}
	for _, p := range r.Paths {
		out.paths = append(out.paths, compilePathMatcher(p))
	}
	for _, q := range r.Queries {
		out.queries = append(out.queries, compileTextMatcher(q, true))
	}
	if len(r.Methods) > 0 {
		out.methods = make(map[string]struct{}, len(r.Methods))
		for _, m := range r.Methods {
			out.methods[strings.ToUpper(strings.TrimSpace(m))] = struct{}{}
		}
	}
	for _, h := range r.Headers {
		out.headers = append(out.headers, compileKVMatcher(h, true))
	}
	for _, c := range r.Cookies {
		out.cookies = append(out.cookies, compileKVMatcher(c, false))
	}
	return out
}

func (cr *compiledCustomRule) matches(req IncomingRequest) bool {
	if cr.ipSet != nil && !cr.ipSet.ContainsString(req.ClientIP) {
		return false
	}
	if len(cr.methods) > 0 {
		if _, ok := cr.methods[strings.ToUpper(req.Method)]; !ok {
			return false
		}
	}
	if len(cr.userAgents) > 0 {
		ua := req.Headers["user-agent"]
		matched := false
		for _, m := range cr.userAgents {
			if m.match(ua) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(cr.paths) > 0 {
		matched := false
		for _, m := range cr.paths {
			if m.match(req.Path) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(cr.queries) > 0 {
		decoded, err := url.QueryUnescape(req.Query)
		if err != nil {
			decoded = req.Query
		}
		matched := false
		for _, m := range cr.queries {
			if m.match(decoded) || m.match(req.Query) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, hm := range cr.headers {
		if !hm.matchMap(req.Headers) {
			return false
		}
	}
	for _, cm := range cr.cookies {
		if !cm.matchMap(req.Cookies) {
			return false
		}
	}
	return true
}

// CustomRulesRule evaluates user-authored CustomRules in declaration order (first match wins).
type CustomRulesRule struct {
	rules      []compiledCustomRule
	tokens     ClearanceManager
	cookieName string
}

// NewCustomRulesRule pre-compiles custom rules for fast per-request evaluation.
func NewCustomRulesRule(rules []config.CustomRule, tokens ClearanceManager, cookieName string) *CustomRulesRule {
	compiled := make([]compiledCustomRule, 0, len(rules))
	for _, r := range rules {
		compiled = append(compiled, compileCustomRule(r))
	}
	return &CustomRulesRule{
		rules:      compiled,
		tokens:     tokens,
		cookieName: cookieName,
	}
}

func (r *CustomRulesRule) Evaluate(_ context.Context, req IncomingRequest) *Decision {
	for i := range r.rules {
		rule := &r.rules[i]
		if !rule.matches(req) {
			continue
		}
		switch rule.action {
		case config.RuleActionAllow:
			d := Allow()
			return &d
		case config.RuleActionBlock:
			d := Deny("rule", http.StatusForbidden)
			return &d
		case config.RuleActionVerify:
			// Allow CORS preflight and WAF challenge-completion endpoints so browser PoW succeeds.
			if strings.EqualFold(req.Method, http.MethodOptions) || isInternalWAFPublicPath(req.Path) {
				d := Allow()
				return &d
			}
			if r.tokens != nil && r.tokens.Valid(req.Cookies[r.cookieName], req.Host, req.ClientIP) {
				d := Allow()
				return &d
			}
			d := Challenge()
			return &d
		}
	}
	return nil
}

func isInternalWAFPublicPath(path string) bool {
	if strings.HasPrefix(path, "/.waf/assets/") || strings.HasPrefix(path, "/.waf/v1/assets/") {
		return true
	}
	switch path {
	case "/.waf/verify", "/.waf/v1/challenges", "/.waf/v1/verifications", "/.waf/v1/session":
		return true
	default:
		return false
	}
}
