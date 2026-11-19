package waf

import (
	"context"
	"net/http"
	"net/netip"
	"regexp"
	"strings"

	"gymbag/internal/config"
	"gymbag/internal/intel"
)

type textMatchKind uint8

const (
	textMatchExact textMatchKind = iota
	textMatchSubstring
	textMatchPrefixLower
	textMatchSuffixLower
	textMatchContainsLower
	textMatchRegex
)

type textMatcher struct {
	kind  textMatchKind
	re    *regexp.Regexp
	token string
}

func compileTextMatcher(pattern string, defaultSubstring bool) textMatcher {
	if strings.HasPrefix(pattern, "~") {
		expr := strings.TrimPrefix(pattern, "~")
		if !strings.HasPrefix(expr, "(?") {
			expr = "(?i)" + expr
		}
		if re, err := regexp.Compile(expr); err == nil {
			return textMatcher{kind: textMatchRegex, re: re}
		}
	}
	if strings.Contains(pattern, "*") {
		lower := strings.ToLower(pattern)
		starCount := strings.Count(lower, "*")
		if starCount == 1 {
			if strings.HasSuffix(lower, "*") {
				return textMatcher{kind: textMatchPrefixLower, token: lower[:len(lower)-1]}
			}
			if strings.HasPrefix(lower, "*") {
				return textMatcher{kind: textMatchSuffixLower, token: lower[1:]}
			}
		} else if starCount == 2 && strings.HasPrefix(lower, "*") && strings.HasSuffix(lower, "*") {
			return textMatcher{kind: textMatchContainsLower, token: lower[1 : len(lower)-1]}
		}
		re := wildcardToRegexp(pattern, true)
		return textMatcher{kind: textMatchRegex, re: re}
	}
	if defaultSubstring {
		return textMatcher{kind: textMatchSubstring, token: strings.ToLower(pattern)}
	}
	return textMatcher{kind: textMatchExact, token: pattern}
}

func (m textMatcher) match(val, lowerVal string) bool {
	switch m.kind {
	case textMatchSubstring, textMatchContainsLower:
		return strings.Contains(lowerVal, m.token)
	case textMatchPrefixLower:
		return strings.HasPrefix(lowerVal, m.token)
	case textMatchSuffixLower:
		return strings.HasSuffix(lowerVal, m.token)
	case textMatchExact:
		return val == m.token
	case textMatchRegex:
		return m.re != nil && m.re.MatchString(val)
	default:
		return false
	}
}

func (m textMatcher) matchesEmpty() bool {
	return m.match("", "")
}

type pathMatchKind uint8

const (
	pathMatchExact pathMatchKind = iota
	pathMatchDirPrefix
	pathMatchPrefix
	pathMatchSuffix
	pathMatchContains
	pathMatchRegex
)

type pathMatcher struct {
	kind    pathMatchKind
	re      *regexp.Regexp
	token   string
	trimmed string // for pathMatchDirPrefix: token without trailing "/"
}

func compilePathMatcher(pattern string) pathMatcher {
	if strings.HasPrefix(pattern, "~") {
		expr := strings.TrimPrefix(pattern, "~")
		if re, err := regexp.Compile(expr); err == nil {
			return pathMatcher{kind: pathMatchRegex, re: re}
		}
	}
	if strings.Contains(pattern, "*") {
		starCount := strings.Count(pattern, "*")
		if starCount == 1 {
			if strings.HasSuffix(pattern, "*") {
				return pathMatcher{kind: pathMatchPrefix, token: pattern[:len(pattern)-1]}
			}
			if strings.HasPrefix(pattern, "*") {
				return pathMatcher{kind: pathMatchSuffix, token: pattern[1:]}
			}
		} else if starCount == 2 && strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
			return pathMatcher{kind: pathMatchContains, token: pattern[1 : len(pattern)-1]}
		}
		return pathMatcher{kind: pathMatchRegex, re: wildcardToRegexp(pattern, false)}
	}
	if len(pattern) > 1 && strings.HasSuffix(pattern, "/") {
		return pathMatcher{
			kind:    pathMatchDirPrefix,
			token:   pattern,
			trimmed: strings.TrimSuffix(pattern, "/"),
		}
	}
	return pathMatcher{kind: pathMatchExact, token: pattern}
}

func (m pathMatcher) match(path string) bool {
	switch m.kind {
	case pathMatchExact:
		return path == m.token
	case pathMatchDirPrefix:
		return path == m.trimmed || strings.HasPrefix(path, m.token)
	case pathMatchPrefix:
		return strings.HasPrefix(path, m.token)
	case pathMatchSuffix:
		return strings.HasSuffix(path, m.token)
	case pathMatchContains:
		return strings.Contains(path, m.token)
	case pathMatchRegex:
		return m.re != nil && m.re.MatchString(path)
	default:
		return false
	}
}

type kvCompiledMatcher struct {
	canonicalName string
	lowerName     string
	rawName       string
	wantEmpty     bool
	wantAny       bool
	valMatch      textMatcher
}

func compileKVMatcher(kv config.KeyValueMatch, isHeader bool) kvCompiledMatcher {
	raw := strings.TrimSpace(kv.Name)
	lower := strings.ToLower(raw)
	canonical := raw
	if isHeader {
		canonical = canonicalHeaderName(raw)
	}
	if kv.Pattern == "" {
		return kvCompiledMatcher{canonicalName: canonical, lowerName: lower, rawName: raw, wantEmpty: true}
	}
	if kv.Pattern == "*" {
		return kvCompiledMatcher{canonicalName: canonical, lowerName: lower, rawName: raw, wantAny: true}
	}
	return kvCompiledMatcher{
		canonicalName: canonical,
		lowerName:     lower,
		rawName:       raw,
		valMatch:      compileTextMatcher(kv.Pattern, true),
	}
}

func (m kvCompiledMatcher) matchValue(rawVal string) bool {
	actual := trimASCIISpace(rawVal)
	if m.wantEmpty {
		return actual == ""
	}
	if actual == "" {
		return false
	}
	if m.wantAny {
		return true
	}
	return m.valMatch.match(actual, asciiLowerIfNeeded(actual))
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
	name              string
	action            config.RuleAction
	ipSet             *intel.IPSet
	userAgentAC       *asciiAC
	userAgents        []textMatcher
	hasUserAgents     bool
	exactPaths        map[string]struct{}
	paths             []pathMatcher
	hasPaths          bool
	queries           []textMatcher
	queryMatchesEmpty bool
	methods           map[string]struct{}
	headers           []kvCompiledMatcher
	cookies           []kvCompiledMatcher
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
	if len(r.UserAgents) > 0 {
		out.hasUserAgents = true
		var acSubs []string
		for _, ua := range r.UserAgents {
			tm := compileTextMatcher(ua, true)
			if tm.kind == textMatchSubstring || tm.kind == textMatchContainsLower {
				acSubs = append(acSubs, tm.token)
			} else {
				out.userAgents = append(out.userAgents, tm)
			}
		}
		if len(acSubs) > 0 {
			out.userAgentAC = compileASCIIAC(acSubs)
		}
	}
	if len(r.Paths) > 0 {
		out.hasPaths = true
		for _, p := range r.Paths {
			pm := compilePathMatcher(p)
			if pm.kind == pathMatchExact {
				if out.exactPaths == nil {
					out.exactPaths = make(map[string]struct{})
				}
				out.exactPaths[pm.token] = struct{}{}
			} else {
				out.paths = append(out.paths, pm)
			}
		}
	}
	for _, q := range r.Queries {
		qm := compileTextMatcher(q, true)
		if qm.matchesEmpty() {
			out.queryMatchesEmpty = true
		}
		out.queries = append(out.queries, qm)
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

func (cr *compiledCustomRule) matchesView(v *reqView) bool {
	if cr.ipSet != nil && !cr.ipSet.ContainsString(v.clientIP) {
		return false
	}
	if len(cr.methods) > 0 {
		if _, ok := cr.methods[v.method]; !ok {
			return false
		}
	}
	if cr.hasPaths {
		matched := false
		if cr.exactPaths != nil {
			_, matched = cr.exactPaths[v.path]
		}
		if !matched {
			for i := range cr.paths {
				if cr.paths[i].match(v.path) {
					matched = true
					break
				}
			}
		}
		if !matched {
			return false
		}
	}
	if len(cr.queries) > 0 {
		if v.query == "" && !cr.queryMatchesEmpty {
			return false
		}
		decoded := v.getDecodedQuery()
		lowerDecoded := v.getLowerDecodedQuery()
		rawDiffers := decoded != v.query
		var lowerRaw string
		if rawDiffers {
			lowerRaw = asciiLowerIfNeeded(v.query)
		}
		matched := false
		for i := range cr.queries {
			m := &cr.queries[i]
			if m.match(decoded, lowerDecoded) || (rawDiffers && m.match(v.query, lowerRaw)) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if cr.hasUserAgents {
		ua := v.getUserAgent()
		matched := cr.userAgentAC != nil && cr.userAgentAC.MatchAny(ua)
		if !matched && len(cr.userAgents) > 0 {
			lowerUA := v.getLowerUA()
			for i := range cr.userAgents {
				if cr.userAgents[i].match(ua, lowerUA) {
					matched = true
					break
				}
			}
		}
		if !matched {
			return false
		}
	}
	for i := range cr.headers {
		hm := &cr.headers[i]
		if !hm.matchValue(v.getHeader(hm.canonicalName, hm.lowerName)) {
			return false
		}
	}
	for i := range cr.cookies {
		cm := &cr.cookies[i]
		if !cm.matchValue(v.getCookie(cm.rawName)) {
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

func (r *CustomRulesRule) Evaluate(ctx context.Context, req IncomingRequest) *Decision {
	v := newIncomingReqView(&req)
	return r.evaluateView(ctx, &v)
}

func (r *CustomRulesRule) evaluateView(_ context.Context, v *reqView) *Decision {
	for i := range r.rules {
		rule := &r.rules[i]
		if !rule.matchesView(v) {
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
			if v.method == http.MethodOptions || isInternalWAFPublicPath(v.path) {
				d := Allow()
				return &d
			}
			if r.tokens != nil && r.tokens.Valid(v.getCookie(r.cookieName), v.host, v.clientIP) {
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
	if len(path) < 11 || path[0] != '/' || path[1] != '.' || path[2] != 'w' {
		return false
	}
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
