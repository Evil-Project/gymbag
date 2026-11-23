package config

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"gymbag/internal/host"
)

func parsePositiveInt(raw, name string) (int, error) {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	if v <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", name)
	}
	return v, nil
}

func parseBool(raw, name string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on", "enable", "enabled":
		return true, nil
	case "0", "false", "no", "off", "disable", "disabled":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true or false", name)
	}
}

func stripComment(line string) string {
	var inQuote rune
	escaped := false
	runes := []rune(line)
	for i, ch := range runes {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if inQuote != 0 {
			if ch == inQuote {
				inQuote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			inQuote = ch
			continue
		}
		if ch == '#' && (i == 0 || unicode.IsSpace(runes[i-1])) {
			return string(runes[:i])
		}
	}
	return line
}

func shlexSplit(s string) ([]string, error) {
	var tokens []string
	var cur strings.Builder
	inToken := false
	var quote rune
	escaped := false

	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		if escaped {
			cur.WriteRune(ch)
			inToken = true
			escaped = false
			continue
		}
		if ch == '\\' && quote != '\'' {
			escaped = true
			inToken = true
			continue
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
			} else {
				cur.WriteRune(ch)
			}
			inToken = true
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			inToken = true
			continue
		}
		// Keep Caddyfile placeholders such as {path}, {args[0]}, {remote_host}, or /new{uri}
		// intact as literal token text rather than treating their braces as block delimiters.
		if ch == '{' {
			endIdx := -1
			for j := i + 1; j < len(runes); j++ {
				if runes[j] == '{' || unicode.IsSpace(runes[j]) {
					break
				}
				if runes[j] == '}' {
					if j > i+1 {
						endIdx = j
					}
					break
				}
			}
			if endIdx != -1 {
				for k := i; k <= endIdx; k++ {
					cur.WriteRune(runes[k])
				}
				inToken = true
				i = endIdx
				continue
			}
		}
		if ch == '{' || ch == '}' || ch == ';' {
			if inToken {
				tokens = append(tokens, cur.String())
				cur.Reset()
				inToken = false
			}
			tokens = append(tokens, string(ch))
			continue
		}
		if unicode.IsSpace(ch) {
			if inToken {
				tokens = append(tokens, cur.String())
				cur.Reset()
				inToken = false
			}
			continue
		}
		cur.WriteRune(ch)
		inToken = true
	}
	if escaped || quote != 0 {
		return nil, fmt.Errorf("invalid config syntax: unclosed quote or trailing escape")
	}
	if inToken {
		tokens = append(tokens, cur.String())
	}
	return tokens, nil
}

func tokenizeCaddyfile(text string) ([]string, error) {
	var out []string
	for _, rawLine := range strings.Split(text, "\n") {
		cleaned := strings.TrimSpace(stripComment(rawLine))
		if cleaned == "" {
			continue
		}
		lineTokens, err := shlexSplit(cleaned)
		if err != nil {
			return nil, err
		}
		if len(lineTokens) > 0 {
			out = append(out, lineTokens...)
			out = append(out, "\n")
		}
	}
	return out, nil
}

type parsedBlock struct {
	header []string
	body   []statementItem
}

type statementItem struct {
	words []string
	sub   *parsedBlock
}

// parseItems parses a sequence of tokens into statements and nested blocks.
func parseItems(tokens []string) ([]statementItem, error) {
	var items []statementItem
	i := 0
	n := len(tokens)
	for i < n {
		for i < n && (tokens[i] == "\n" || tokens[i] == ";") {
			i++
		}
		if i >= n {
			break
		}
		if tokens[i] == "}" {
			return nil, fmt.Errorf("unexpected '}' in config file")
		}
		var words []string
		for i < n && tokens[i] != "\n" && tokens[i] != ";" && tokens[i] != "{" && tokens[i] != "}" {
			words = append(words, tokens[i])
			i++
		}
		if i < n && tokens[i] == "{" {
			i++ // consume '{'
			depth := 1
			var subTokens []string
			for i < n && depth > 0 {
				tok := tokens[i]
				if tok == "{" {
					depth++
				} else if tok == "}" {
					depth--
					if depth == 0 {
						i++
						break
					}
				}
				subTokens = append(subTokens, tok)
				i++
			}
			if depth != 0 {
				return nil, fmt.Errorf("unclosed '{' in config file")
			}
			subItems, err := parseItems(subTokens)
			if err != nil {
				return nil, err
			}
			items = append(items, statementItem{
				words: words,
				sub:   &parsedBlock{header: words, body: subItems},
			})
			continue
		}
		if len(words) > 0 {
			items = append(items, statementItem{words: words})
		}
	}
	return items, nil
}

var knownDirectiveNames = map[string]struct{}{
	"reverse_proxy":        {},
	"upstream":             {},
	"proxy":                {},
	"domain":               {},
	"domains":              {},
	"host":                 {},
	"hosts":                {},
	"protect":              {},
	"protected":            {},
	"protect_sites":        {},
	"protect_domains":      {},
	"waf":                  {},
	"intel":                {},
	"rule":                 {},
	"block":                {},
	"deny":                 {},
	"drop":                 {},
	"reject":               {},
	"allow":                {},
	"pass":                 {},
	"bypass":               {},
	"whitelist":            {},
	"challenge":            {},
	"pow":                  {},
	"verify":               {},
	"verify_level":         {},
	"verification":         {},
	"verify_mode":          {},
	"level":                {},
	"trigger":              {},
	"mode":                 {},
	"ip_security":          {},
	"ip_security_action":   {},
	"ip_reputation":        {},
	"live_reputation":      {},
	"ai_ip_list":           {},
	"intel_refresh":        {},
	"threat_feed":          {},
	"threat_feeds":         {},
	"ai_ip_feed":           {},
	"ai_ip_feeds":          {},
	"email":                {},
	"acme_email":           {},
	"acme_directory":       {},
	"ssl_provider":         {},
	"acme_provider":        {},
	"ca":                   {},
	"cert_issuer":          {},
	"tls":                  {},
	"eab":                  {},
	"eab_kid":              {},
	"eab_hmac_key":         {},
	"account_key":          {},
	"account_key_file":     {},
	"ssl_key":              {},
	"cert_key":             {},
	"auto_hosts":           {},
	"key_password":         {},
	"bind":                 {},
	"bind_host":            {},
	"http_port":            {},
	"https_port":           {},
	"admin":                {},
	"difficulty":           {},
	"rate_limit":           {},
	"rate_window":          {},
	"challenge_ttl":        {},
	"clearance_ttl":        {},
	"verify_timeout":       {},
	"verified_timeout":     {},
	"clearance_timeout":    {},
	"cookie_name":          {},
	"cookie_secure":        {},
	"default_rules":        {},
	"remote_rules":         {},
	"remote_rule":          {},
	"rules_url":            {},
	"rule_url":             {},
	"remote_rules_url":     {},
	"remote_rules_refresh": {},
	"rules_refresh":        {},
	"rules_watch_interval": {},
	"rules_file":           {},
	"rule_file":            {},
	"load_rules":           {},
	"ruleset":              {},
	"rules_dir":            {},
	"rule_dir":             {},
	"rules_folder":         {},
	"handle":               {},
	"handle_path":          {},
	"route":                {},
	"root":                 {},
	"file_server":          {},
	"try_files":            {},
	"respond":              {},
	"redir":                {},
	"header":               {},
	"encode":               {},
	"log":                  {},
	"import":               {},
	"redis":                {},
	"redis_url":            {},
	"secret":               {},
	"cert_dir":             {},
	"hsts":                 {},
	"renew_interval":       {},
	"renew_within_days":    {},
	"trusted_proxies":      {},
	"ai_bots":              {},
	"suspicious_uas":       {},
	"suspicious_paths":     {},
	"vpn_detection":        {},
	"vpn_detect":           {},
	"vpn_feed":             {},
	"vpn_feeds":            {},
	"verify_url":           {},
	"verify_port":          {},
	"verify_server":        {},
	"ssh":                  {},
	"minecraft":            {},
	"mc":                   {},
	"port":                 {},
	"server_properties":    {},
	"authelia":             {},
	"forward_auth":         {},
}

var ruleFieldKeywords = map[string]struct{}{
	"ip":         {},
	"ips":        {},
	"cidr":       {},
	"cidrs":      {},
	"user_agent": {},
	"user-agent": {},
	"ua":         {},
	"path":       {},
	"paths":      {},
	"query":      {},
	"queries":    {},
	"method":     {},
	"methods":    {},
	"header":     {},
	"cookie":     {},
}

type parseState struct {
	tls                TLSConfig
	intel              IntelConfig
	verify             VerifyServerConfig
	ssh                *SSHConfig
	minecraft          *MinecraftConfig
	authelia           *ForwardAuthConfig
	remoteRulesRefresh int
	rulesWatchInterval int
	bindHost           string
	httpPort           int
	httpsPort          int
	httpsPortSet       bool
	adminAddr          string
	redisURL           string
	secret             string
	trustedProxies     []string
	defaultWAF         SiteWAFConfig
	defaultProtect     bool
	protectedFilter    map[string]struct{}
	flatDomains        []string
	flatUpstreams      []string
	snippets           map[string][]statementItem
}

func applyRuleConditionField(field string, args []string, rule *CustomRule) error {
	switch strings.ToLower(field) {
	case "ip", "ips", "cidr", "cidrs":
		if len(args) == 0 {
			return fmt.Errorf("%s requires at least one IP address or CIDR", field)
		}
		for _, a := range args {
			for _, part := range strings.Split(a, ",") {
				if trimmed := strings.TrimSpace(part); trimmed != "" {
					rule.IPs = append(rule.IPs, trimmed)
				}
			}
		}
		return nil

	case "user_agent", "user-agent", "ua":
		if len(args) == 0 {
			return fmt.Errorf("%s requires at least one pattern", field)
		}
		rule.UserAgents = append(rule.UserAgents, args...)
		return nil

	case "path", "paths":
		if len(args) == 0 {
			return fmt.Errorf("%s requires at least one path pattern", field)
		}
		rule.Paths = append(rule.Paths, args...)
		return nil

	case "query", "queries":
		if len(args) == 0 {
			return fmt.Errorf("%s requires at least one query pattern", field)
		}
		rule.Queries = append(rule.Queries, args...)
		return nil

	case "method", "methods":
		if len(args) == 0 {
			return fmt.Errorf("%s requires at least one HTTP method", field)
		}
		for _, a := range args {
			for _, part := range strings.Split(a, ",") {
				if trimmed := strings.TrimSpace(part); trimmed != "" {
					rule.Methods = append(rule.Methods, strings.ToUpper(trimmed))
				}
			}
		}
		return nil

	case "header":
		if len(args) < 1 || len(args) > 2 {
			return fmt.Errorf("header rule requires <name> [pattern]")
		}
		pat := "*"
		if len(args) == 2 {
			pat = args[1]
		}
		rule.Headers = append(rule.Headers, KeyValueMatch{Name: args[0], Pattern: pat})
		return nil

	case "cookie":
		if len(args) < 1 || len(args) > 2 {
			return fmt.Errorf("cookie rule requires <name> [pattern]")
		}
		pat := "*"
		if len(args) == 2 {
			pat = args[1]
		}
		rule.Cookies = append(rule.Cookies, KeyValueMatch{Name: args[0], Pattern: pat})
		return nil

	default:
		return fmt.Errorf("unknown rule matcher field %q (expected ip, user_agent, path, query, method, header, or cookie)", field)
	}
}

func parseRuleBlock(block *parsedBlock) (CustomRule, error) {
	var rule CustomRule
	if len(block.header) > 2 {
		return rule, fmt.Errorf("rule block header accepts at most an optional name: rule [name] { ... }")
	}
	if len(block.header) == 2 {
		rule.Name = block.header[1]
	}
	for _, stmt := range block.body {
		if stmt.sub != nil {
			return rule, fmt.Errorf("unexpected nested block inside rule")
		}
		if len(stmt.words) == 0 {
			continue
		}
		directive := strings.ToLower(stmt.words[0])
		args := stmt.words[1:]
		if directive == "action" {
			if len(args) != 1 {
				return rule, fmt.Errorf("rule action requires block, verify, or allow")
			}
			act, err := ParseRuleAction(args[0])
			if err != nil {
				return rule, err
			}
			rule.Action = act
			continue
		}
		if directive == "name" {
			if len(args) != 1 {
				return rule, fmt.Errorf("rule name requires a single value")
			}
			rule.Name = args[0]
			continue
		}
		if err := applyRuleConditionField(directive, args, &rule); err != nil {
			return rule, err
		}
	}
	if rule.Action == "" {
		return rule, fmt.Errorf("rule block %q requires 'action <block|verify|allow>'", rule.Name)
	}
	if err := rule.Validate(); err != nil {
		return rule, err
	}
	return rule, nil
}

func parseShorthandRule(actionWord string, args []string) (CustomRule, error) {
	act, err := ParseRuleAction(actionWord)
	if err != nil {
		return CustomRule{}, err
	}
	if len(args) < 2 {
		return CustomRule{}, fmt.Errorf("%s rule requires <ip|user_agent|path|query|method|header|cookie> <pattern...>", actionWord)
	}
	r := CustomRule{Action: act}
	if err := applyRuleConditionField(args[0], args[1:], &r); err != nil {
		return CustomRule{}, err
	}
	if err := r.Validate(); err != nil {
		return CustomRule{}, err
	}
	return r, nil
}

func applyIntelDirective(name string, args []string, ic *IntelConfig) (bool, error) {
	switch name {
	case "ip_security":
		if len(args) != 1 {
			return true, fmt.Errorf("ip_security requires on/off or true/false")
		}
		b, err := parseBool(args[0], "ip_security")
		if err != nil {
			return true, err
		}
		ic.IPSecurity = b
		return true, nil

	case "ip_security_action", "security_action":
		if len(args) != 1 {
			return true, fmt.Errorf("ip_security_action requires verify or block")
		}
		val := strings.ToLower(strings.TrimSpace(args[0]))
		if val != "verify" && val != "block" && val != "challenge" && val != "deny" {
			return true, fmt.Errorf("ip_security_action must be verify or block")
		}
		if val == "block" || val == "deny" {
			ic.IPSecurityAction = "block"
		} else {
			ic.IPSecurityAction = "verify"
		}
		return true, nil

	case "ip_reputation", "live_reputation":
		if len(args) != 1 {
			return true, fmt.Errorf("%s requires on/off or true/false", name)
		}
		b, err := parseBool(args[0], name)
		if err != nil {
			return true, err
		}
		ic.LiveReputation = b
		return true, nil

	case "ai_ip_list":
		if len(args) != 1 {
			return true, fmt.Errorf("ai_ip_list requires on/off or true/false")
		}
		b, err := parseBool(args[0], "ai_ip_list")
		if err != nil {
			return true, err
		}
		ic.AIIPList = b
		return true, nil

	case "intel_refresh", "refresh_interval":
		if len(args) != 1 {
			return true, fmt.Errorf("%s requires seconds or a duration (e.g. 6h)", name)
		}
		sec, err := ParseDurationSeconds(args[0], name)
		if err != nil {
			return true, err
		}
		ic.RefreshInterval = sec
		return true, nil

	case "threat_feed", "threat_feeds":
		if len(args) == 0 {
			return true, fmt.Errorf("%s requires at least one feed URL", name)
		}
		for _, u := range args {
			if err := RequireHTTPURL(u, name); err != nil {
				return true, err
			}
			ic.ThreatFeeds = append(ic.ThreatFeeds, strings.TrimSpace(u))
		}
		return true, nil

	case "ai_ip_feed", "ai_ip_feeds":
		if len(args) == 0 {
			return true, fmt.Errorf("%s requires at least one feed URL", name)
		}
		for _, u := range args {
			if err := RequireHTTPURL(u, name); err != nil {
				return true, err
			}
			ic.AIIPFeeds = append(ic.AIIPFeeds, strings.TrimSpace(u))
		}
		return true, nil

	case "vpn_detection", "vpn_detect":
		if len(args) != 1 {
			return true, fmt.Errorf("%s requires on/off or true/false", name)
		}
		b, err := parseBool(args[0], name)
		if err != nil {
			return true, err
		}
		ic.VPNDetection = b
		return true, nil

	case "vpn_feed", "vpn_feeds":
		if len(args) == 0 {
			return true, fmt.Errorf("%s requires at least one feed URL", name)
		}
		for _, u := range args {
			if err := RequireHTTPURL(u, name); err != nil {
				return true, err
			}
			ic.VPNFeeds = append(ic.VPNFeeds, strings.TrimSpace(u))
		}
		return true, nil
	}
	return false, nil
}

func applyWAFDirective(name string, args []string, waf *SiteWAFConfig, intel *IntelConfig, protect *bool, protectExplicit *bool) (bool, error) {
	// Open-box bare 'block' or 'block on': enable strict protection, threat-IP blocking, and default security rules.
	if name == "block" {
		if len(args) == 0 {
			*protect = true
			if protectExplicit != nil {
				*protectExplicit = true
			}
			if intel != nil {
				intel.IPSecurity = true
				intel.IPSecurityAction = "block"
			}
			defOn := true
			waf.DefaultRules = &defOn
			return true, nil
		}
		if len(args) == 1 {
			if b, bErr := parseBool(args[0], "block"); bErr == nil {
				if b {
					*protect = true
					if protectExplicit != nil {
						*protectExplicit = true
					}
					if intel != nil {
						intel.IPSecurity = true
						intel.IPSecurityAction = "block"
					}
					defOn := true
					waf.DefaultRules = &defOn
				} else if intel != nil {
					intel.IPSecurityAction = "verify"
				}
				return true, nil
			}
		}
	}

	// Shorthand custom rules: block/deny/drop/reject, allow/pass/bypass/whitelist, challenge/pow <field> <pattern...>
	switch name {
	case "block", "deny", "drop", "reject", "allow", "pass", "bypass", "whitelist", "challenge", "pow":
		rule, err := parseShorthandRule(name, args)
		if err != nil {
			return true, err
		}
		waf.Rules = append(waf.Rules, rule)
		return true, nil
	}

	// 'verify' can be bare 'verify' / 'verify on', 'verify <0-3>', OR 'verify <ip|user_agent|path|...> <pattern...>' (custom rule)!
	if name == "verify" && len(args) >= 2 {
		if _, isRuleField := ruleFieldKeywords[strings.ToLower(args[0])]; isRuleField {
			rule, err := parseShorthandRule("verify", args)
			if err != nil {
				return true, err
			}
			waf.Rules = append(waf.Rules, rule)
			return true, nil
		}
	}

	switch name {
	case "enable", "enabled":
		if len(args) != 1 {
			return true, fmt.Errorf("%s requires true or false", name)
		}
		b, err := parseBool(args[0], name)
		if err != nil {
			return true, err
		}
		*protect = b
		if protectExplicit != nil {
			*protectExplicit = true
		}
		return true, nil

	case "verify", "verify_level", "verification", "verify_mode", "level", "trigger", "mode":
		if name == "verify" && len(args) == 0 {
			*protect = true
			if protectExplicit != nil {
				*protectExplicit = true
			}
			waf.VerifyLevel = VerifyLevelSuspiciousAndAI
			defOn := true
			waf.DefaultRules = &defOn
			return true, nil
		}
		if name == "verify" && len(args) == 1 {
			switch strings.ToLower(strings.TrimSpace(args[0])) {
			case "on", "true", "yes", "enable", "enabled":
				*protect = true
				if protectExplicit != nil {
					*protectExplicit = true
				}
				waf.VerifyLevel = VerifyLevelSuspiciousAndAI
				defOn := true
				waf.DefaultRules = &defOn
				return true, nil
			}
		}
		if len(args) == 0 {
			return true, fmt.Errorf("%s requires a verify level from 0 to 3 (0=always, 1=only suspicious, 2=only ai, 3=suspicious & ai)", name)
		}
		joined := strings.Join(args, " ")
		lvl, enabled, err := ParseVerifyLevel(joined)
		if err != nil {
			return true, err
		}
		waf.VerifyLevel = lvl
		if !enabled {
			*protect = false
			if protectExplicit != nil {
				*protectExplicit = true
			}
		}
		return true, nil

	case "default_rules":
		if len(args) != 1 {
			return true, fmt.Errorf("default_rules requires on/off or true/false")
		}
		b, err := parseBool(args[0], "default_rules")
		if err != nil {
			return true, err
		}
		waf.DefaultRules = &b
		return true, nil

	case "difficulty":
		if len(args) != 1 {
			return true, fmt.Errorf("difficulty requires an integer from 1 to 5")
		}
		diff, err := parsePositiveInt(args[0], "difficulty")
		if err != nil {
			return true, err
		}
		if diff < 1 || diff > 5 {
			return true, fmt.Errorf("difficulty must be from 1 to 5")
		}
		waf.Difficulty = diff
		return true, nil

	case "rate_limit":
		if len(args) != 1 && len(args) != 2 {
			return true, fmt.Errorf("rate_limit requires a limit and optional window (seconds or duration)")
		}
		limit, err := parsePositiveInt(args[0], "rate_limit")
		if err != nil {
			return true, err
		}
		waf.RateLimit = limit
		if len(args) == 2 {
			win, err := ParseDurationSeconds(args[1], "rate_window")
			if err != nil {
				return true, err
			}
			waf.RateWindow = win
		}
		return true, nil

	case "rate_window":
		if len(args) != 1 {
			return true, fmt.Errorf("rate_window requires seconds or a duration (e.g. 60s, 1m)")
		}
		win, err := ParseDurationSeconds(args[0], "rate_window")
		if err != nil {
			return true, err
		}
		waf.RateWindow = win
		return true, nil

	case "challenge_ttl":
		if len(args) != 1 {
			return true, fmt.Errorf("challenge_ttl requires seconds or a duration (e.g. 5m)")
		}
		ttl, err := ParseDurationSeconds(args[0], "challenge_ttl")
		if err != nil {
			return true, err
		}
		waf.ChallengeTTL = ttl
		return true, nil

	case "clearance_ttl", "verify_timeout", "verified_timeout", "clearance_timeout":
		if len(args) != 1 {
			return true, fmt.Errorf("%s requires seconds or a duration (e.g. 30m, 12h, 7d)", name)
		}
		ttl, err := ParseDurationSeconds(args[0], name)
		if err != nil {
			return true, err
		}
		waf.ClearanceTTL = ttl
		return true, nil

	case "remote_rules", "remote_rule", "rules_url", "rule_url", "remote_rules_url":
		if len(args) == 0 {
			return true, fmt.Errorf("%s requires at least one http:// or https:// URL", name)
		}
		for _, a := range args {
			for _, part := range strings.Split(a, ",") {
				u := strings.TrimSpace(part)
				if u == "" {
					continue
				}
				if err := RequireHTTPURL(u, name); err != nil {
					return true, err
				}
				if !slices.Contains(waf.RemoteRules, u) {
					waf.RemoteRules = append(waf.RemoteRules, u)
				}
			}
		}
		return true, nil

	case "rules_file", "rule_file", "load_rules", "ruleset", "rules_dir", "rule_dir", "rules_folder":
		sources, err := parseRuleSourceArgs(args, nil, name)
		if err != nil {
			return true, err
		}
		waf.RuleSources = append(waf.RuleSources, sources...)
		return true, nil

	case "cookie_name":
		if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
			return true, fmt.Errorf("cookie_name requires a value")
		}
		waf.CookieName = strings.TrimSpace(args[0])
		return true, nil

	case "cookie_secure":
		if len(args) != 1 {
			return true, fmt.Errorf("cookie_secure requires true or false")
		}
		b, err := parseBool(args[0], "cookie_secure")
		if err != nil {
			return true, err
		}
		waf.CookieSecure = b
		return true, nil

	case "ai_bots":
		if len(args) == 0 {
			return true, fmt.Errorf("ai_bots requires at least one user-agent pattern")
		}
		for _, a := range args {
			if p := strings.TrimSpace(a); p != "" {
				waf.CustomAIBots = append(waf.CustomAIBots, strings.ToLower(p))
			}
		}
		return true, nil

	case "suspicious_uas":
		if len(args) == 0 {
			return true, fmt.Errorf("suspicious_uas requires at least one user-agent pattern")
		}
		for _, a := range args {
			if p := strings.TrimSpace(a); p != "" {
				waf.CustomSuspiciousUAs = append(waf.CustomSuspiciousUAs, strings.ToLower(p))
			}
		}
		return true, nil

	case "suspicious_paths":
		if len(args) == 0 {
			return true, fmt.Errorf("suspicious_paths requires at least one path prefix")
		}
		for _, a := range args {
			if p := strings.TrimSpace(a); p != "" {
				waf.CustomSuspiciousPaths = append(waf.CustomSuspiciousPaths, strings.ToLower(p))
			}
		}
		return true, nil
	}
	return false, nil
}

func applyTLSDirective(
	name string,
	args []string,
	tlsCfg *TLSConfig,
	siteCert *string,
	siteKey *string,
	isGlobalScope bool,
) (bool, error) {
	switch name {
	case "email", "acme_email":
		if len(args) != 1 {
			return true, fmt.Errorf("%s requires an email address", name)
		}
		tlsCfg.ACMEEmail = strings.TrimSpace(args[0])
		return true, nil

	case "acme_directory":
		if len(args) != 1 {
			return true, fmt.Errorf("acme_directory requires a URL")
		}
		if err := RequireHTTPURL(args[0], "acme_directory"); err != nil {
			return true, err
		}
		tlsCfg.ACMEDirectory = strings.TrimSpace(args[0])
		return true, nil

	case "ssl_provider", "acme_provider", "ca", "cert_issuer":
		if len(args) != 1 {
			return true, fmt.Errorf("%s requires a provider name or ACME directory URL", name)
		}
		if err := applySSLProvider(args[0], tlsCfg); err != nil {
			return true, err
		}
		return true, nil

	case "tls":
		if len(args) == 0 {
			return true, fmt.Errorf("tls requires arguments")
		}
		if len(args) == 1 {
			val := strings.TrimSpace(args[0])
			lower := strings.ToLower(val)
			switch {
			case lower == "selfsigned" || lower == "self_signed":
				tlsCfg.Mode = "selfsigned"
				tlsCfg.Provider = ""
			case lower == "local" || lower == "internal":
				tlsCfg.Mode = "local"
			case lower == "auto" || lower == "acme" || lower == "file":
				tlsCfg.Mode = lower
			case strings.Contains(val, "@") && !strings.Contains(val, "://"):
				tlsCfg.ACMEEmail = val
			default:
				if err := applySSLProvider(val, tlsCfg); err != nil {
					return true, fmt.Errorf("unsupported tls option: %s", val)
				}
			}
			return true, nil
		}
		if len(args) == 2 {
			if !isGlobalScope && siteCert != nil && siteKey != nil {
				*siteCert = args[0]
				*siteKey = args[1]
				if tlsCfg.CertFile == "cert.pem" && tlsCfg.KeyFile == "key.pem" {
					tlsCfg.Mode = "file"
					tlsCfg.CertFile = args[0]
					tlsCfg.KeyFile = args[1]
				}
			} else {
				tlsCfg.Mode = "file"
				tlsCfg.CertFile = args[0]
				tlsCfg.KeyFile = args[1]
			}
			return true, nil
		}
		return true, fmt.Errorf("tls accepts a mode, provider, email, or cert and key file paths")

	case "eab":
		if len(args) != 2 || strings.TrimSpace(args[0]) == "" || strings.TrimSpace(args[1]) == "" {
			return true, fmt.Errorf("eab requires <kid> <hmac_key>")
		}
		tlsCfg.EABKID = strings.TrimSpace(args[0])
		tlsCfg.EABHMACKey = strings.TrimSpace(args[1])
		return true, nil

	case "eab_kid":
		if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
			return true, fmt.Errorf("eab_kid requires a value")
		}
		tlsCfg.EABKID = strings.TrimSpace(args[0])
		return true, nil

	case "eab_hmac_key":
		if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
			return true, fmt.Errorf("eab_hmac_key requires a value")
		}
		tlsCfg.EABHMACKey = strings.TrimSpace(args[0])
		return true, nil

	case "account_key", "account_key_file":
		if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
			return true, fmt.Errorf("%s requires a file path", name)
		}
		tlsCfg.AccountKeyFile = strings.TrimSpace(args[0])
		return true, nil

	case "ssl_key", "cert_key":
		if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
			return true, fmt.Errorf("%s requires a file path", name)
		}
		tlsCfg.KeyFile = strings.TrimSpace(args[0])
		return true, nil

	case "key_password":
		if len(args) != 1 {
			return true, fmt.Errorf("key_password requires a value")
		}
		tlsCfg.KeyPassword = args[0]
		return true, nil

	case "auto_hosts":
		if len(args) != 1 {
			return true, fmt.Errorf("auto_hosts requires on/off or true/false")
		}
		b, err := parseBool(args[0], "auto_hosts")
		if err != nil {
			return true, err
		}
		tlsCfg.AutoHosts = &b
		return true, nil

	case "cert_dir":
		if len(args) != 1 {
			return true, fmt.Errorf("cert_dir requires a directory path")
		}
		tlsCfg.CertDir = args[0]
		return true, nil

	case "hsts":
		if len(args) != 1 {
			return true, fmt.Errorf("hsts requires true or false")
		}
		b, err := parseBool(args[0], "hsts")
		if err != nil {
			return true, err
		}
		tlsCfg.HSTS = &b
		return true, nil

	case "renew_interval":
		if len(args) != 1 {
			return true, fmt.Errorf("renew_interval requires seconds or a duration (e.g. 12h)")
		}
		v, err := ParseDurationSeconds(args[0], "renew_interval")
		if err != nil {
			return true, err
		}
		tlsCfg.RenewInterval = v
		return true, nil

	case "renew_within_days":
		if len(args) != 1 {
			return true, fmt.Errorf("renew_within_days requires days")
		}
		v, err := parsePositiveInt(args[0], "renew_within_days")
		if err != nil {
			return true, err
		}
		tlsCfg.RenewWithinDays = v
		return true, nil
	}
	return false, nil
}

func applyGlobalDirective(
	item statementItem,
	st *parseState,
	waf *SiteWAFConfig,
	protect *bool,
	protectExplicit *bool,
	siteCert *string,
	siteKey *string,
	isGlobalScope bool,
) ([]string, error) {
	words := item.words
	if len(words) == 0 {
		return nil, nil
	}

	if isGlobalScope && len(words) > 0 {
		if lw := strings.ToLower(words[0]); lw == "authelia" || lw == "forward_auth" {
			authCfg, handled, err := parseAuthDirective(item, st.authelia, nil, true)
			if err != nil {
				return nil, err
			}
			if handled {
				st.authelia = authCfg
				return nil, nil
			}
		}
	}

	if item.sub != nil {
		subName := strings.ToLower(words[0])
		switch subName {
		case "rule":
			rule, err := parseRuleBlock(item.sub)
			if err != nil {
				return nil, err
			}
			waf.Rules = append(waf.Rules, rule)
			return nil, nil

		case "intel":
			if len(words) > 1 {
				return nil, fmt.Errorf("intel block does not take header arguments")
			}
			for _, subItem := range item.sub.body {
				if subItem.sub != nil {
					return nil, fmt.Errorf("unexpected nested block inside intel")
				}
				if len(subItem.words) == 0 {
					continue
				}
				handled, err := applyIntelDirective(strings.ToLower(subItem.words[0]), subItem.words[1:], &st.intel)
				if err != nil {
					return nil, err
				}
				if !handled {
					return nil, fmt.Errorf("unknown intel directive: %s", subItem.words[0])
				}
			}
			return nil, nil

		case "tls":
			if len(words) > 1 {
				if _, err := applyTLSDirective("tls", words[1:], &st.tls, siteCert, siteKey, isGlobalScope); err != nil {
					return nil, err
				}
			}
			for _, subItem := range item.sub.body {
				if subItem.sub != nil {
					return nil, fmt.Errorf("unexpected nested block inside tls")
				}
				if len(subItem.words) == 0 {
					continue
				}
				tname := strings.ToLower(subItem.words[0])
				if tname == "provider" || tname == "issuer" {
					tname = "ssl_provider"
				}
				handled, err := applyTLSDirective(tname, subItem.words[1:], &st.tls, siteCert, siteKey, isGlobalScope)
				if err != nil {
					return nil, err
				}
				if !handled {
					return nil, fmt.Errorf("unknown tls directive: %s", subItem.words[0])
				}
			}
			return nil, nil

		case "ruleset":
			rs, err := parseRuleSourceBlock(item.sub)
			if err != nil {
				return nil, err
			}
			waf.RuleSources = append(waf.RuleSources, rs)
			return nil, nil

		case "waf":
			if len(words) > 1 {
				joined := strings.Join(words[1:], " ")
				if lvl, enabled, modeErr := ParseVerifyLevel(joined); modeErr == nil {
					waf.VerifyLevel = lvl
					*protect = enabled
					if protectExplicit != nil {
						*protectExplicit = true
					}
				} else if b, err := parseBool(joined, "waf"); err == nil {
					*protect = b
					if protectExplicit != nil {
						*protectExplicit = true
					}
				} else {
					return nil, modeErr
				}
			}
			for _, subItem := range item.sub.body {
				if subItem.sub != nil && len(subItem.words) > 0 && strings.EqualFold(subItem.words[0], "ruleset") {
					rs, err := parseRuleSourceBlock(subItem.sub)
					if err != nil {
						return nil, err
					}
					waf.RuleSources = append(waf.RuleSources, rs)
					continue
				}
				if subItem.sub != nil {
					if strings.EqualFold(subItem.words[0], "rule") {
						rule, err := parseRuleBlock(subItem.sub)
						if err != nil {
							return nil, err
						}
						waf.Rules = append(waf.Rules, rule)
						continue
					}
					return nil, fmt.Errorf("unexpected nested block inside waf: %s", subItem.words[0])
				}
				if len(subItem.words) == 0 {
					continue
				}
				sname := strings.ToLower(subItem.words[0])
				sargs := subItem.words[1:]
				if sname == "protect" || sname == "protected" {
					if len(sargs) != 1 {
						return nil, fmt.Errorf("protect requires true or false")
					}
					b, err := parseBool(sargs[0], "protect")
					if err != nil {
						return nil, err
					}
					*protect = b
					if protectExplicit != nil {
						*protectExplicit = true
					}
					continue
				}
				handled, err := applyWAFDirective(sname, sargs, waf, &st.intel, protect, protectExplicit)
				if err != nil {
					return nil, err
				}
				if !handled {
					return nil, fmt.Errorf("unknown waf directive: %s", subItem.words[0])
				}
			}
			return nil, nil

		case "encode", "log":
			// Tolerate Caddyfile v2 compression/logging blocks.
			return nil, nil

		case "verify_server":
			if !isGlobalScope {
				return nil, fmt.Errorf("verify_server block is only allowed in global options")
			}
			for _, subItem := range item.sub.body {
				if subItem.sub != nil {
					return nil, fmt.Errorf("unexpected nested block inside verify_server")
				}
				if len(subItem.words) == 0 {
					continue
				}
				k := strings.ToLower(subItem.words[0])
				vargs := subItem.words[1:]
				switch k {
				case "url":
					if len(vargs) != 1 {
						return nil, fmt.Errorf("verify_server url requires a value")
					}
					st.verify.URL = strings.TrimSpace(vargs[0])
				case "port":
					if len(vargs) != 1 {
						return nil, fmt.Errorf("verify_server port requires a port number")
					}
					p, pErr := host.ParsePort(vargs[0], "verify_port")
					if pErr != nil {
						return nil, pErr
					}
					st.verify.Port = p
				case "tls":
					if len(vargs) != 2 {
						return nil, fmt.Errorf("verify_server tls requires <cert_file> <key_file>")
					}
					st.verify.CertFile = vargs[0]
					st.verify.KeyFile = vargs[1]
				default:
					return nil, fmt.Errorf("unknown verify_server directive: %s", subItem.words[0])
				}
			}
			return nil, nil

		default:
			return nil, fmt.Errorf("unsupported nested block: %s", words[0])
		}
	}

	if len(words) == 3 && words[1] == "->" {
		st.flatDomains = append(st.flatDomains, words[0])
		u, err := NormalizeUpstream(words[2], "reverse_proxy")
		if err != nil {
			return nil, err
		}
		return []string{u}, nil
	}

	name := strings.ToLower(words[0])
	args := words[1:]

	if name == "reverse_proxy" || name == "upstream" || name == "proxy" {
		var targets []string
		for _, a := range args {
			if strings.EqualFold(a, "to") {
				continue
			}
			u, err := NormalizeUpstream(a, name)
			if err != nil {
				return nil, err
			}
			targets = append(targets, u)
		}
		if len(targets) == 0 {
			return nil, fmt.Errorf("%s requires an upstream URL", name)
		}
		return targets, nil
	}

	if len(words) == 1 && (strings.Contains(words[0], "://") || strings.HasPrefix(words[0], ":")) {
		u, err := NormalizeUpstream(words[0], "reverse_proxy")
		if err != nil {
			return nil, err
		}
		return []string{u}, nil
	}

	if handled, err := applyIntelDirective(name, args, &st.intel); handled {
		return nil, err
	}

	if handled, err := applyWAFDirective(name, args, waf, &st.intel, protect, protectExplicit); handled {
		return nil, err
	}

	if handled, err := applyTLSDirective(name, args, &st.tls, siteCert, siteKey, isGlobalScope); handled {
		return nil, err
	}

	switch name {
	case "protect", "protected":
		if len(args) == 0 {
			return nil, fmt.Errorf("%s requires true/false or domain names", name)
		}
		if len(args) == 1 {
			if b, err := parseBool(args[0], name); err == nil {
				*protect = b
				if protectExplicit != nil {
					*protectExplicit = true
				}
				return nil, nil
			}
		}
		if !isGlobalScope {
			return nil, fmt.Errorf("%s inside a site block must be on/off or true/false", name)
		}
		if st.protectedFilter == nil {
			st.protectedFilter = make(map[string]struct{})
		}
		for _, a := range args {
			for _, part := range strings.Split(a, ",") {
				if strings.TrimSpace(part) == "" {
					continue
				}
				h, _, _, err := host.ParseSiteAddress(part)
				if err != nil {
					return nil, err
				}
				st.protectedFilter[h] = struct{}{}
			}
		}
		return nil, nil

	case "protect_sites", "protect_domains":
		if len(args) == 0 {
			return nil, fmt.Errorf("%s requires at least one domain", name)
		}
		if st.protectedFilter == nil {
			st.protectedFilter = make(map[string]struct{})
		}
		for _, a := range args {
			for _, part := range strings.Split(a, ",") {
				if strings.TrimSpace(part) == "" {
					continue
				}
				h, _, _, err := host.ParseSiteAddress(part)
				if err != nil {
					return nil, err
				}
				st.protectedFilter[h] = struct{}{}
			}
		}
		return nil, nil

	case "waf":
		if len(args) == 0 {
			return nil, fmt.Errorf("waf requires on/off, a verify level (0-3), or a { ... } block")
		}
		if len(args) == 1 {
			if _, convErr := strconv.Atoi(strings.TrimSpace(args[0])); convErr != nil {
				if b, err := parseBool(args[0], "waf"); err == nil {
					*protect = b
					if protectExplicit != nil {
						*protectExplicit = true
					}
					return nil, nil
				}
			}
		}
		joined := strings.Join(args, " ")
		lvl, enabled, err := ParseVerifyLevel(joined)
		if err != nil {
			return nil, err
		}
		waf.VerifyLevel = lvl
		*protect = enabled
		if protectExplicit != nil {
			*protectExplicit = true
		}
		return nil, nil

	case "domain", "domains", "host", "hosts":
		if len(args) == 0 {
			return nil, fmt.Errorf("%s requires at least one domain", name)
		}
		var added int
		for _, a := range args {
			for _, part := range strings.Split(a, ",") {
				if strings.TrimSpace(part) == "" {
					continue
				}
				h, _, _, err := host.ParseSiteAddress(part)
				if err != nil {
					return nil, err
				}
				st.flatDomains = append(st.flatDomains, h)
				added++
			}
		}
		if added == 0 {
			return nil, fmt.Errorf("%s requires at least one domain", name)
		}
		return nil, nil

	case "bind", "bind_host":
		if len(args) != 1 {
			return nil, fmt.Errorf("bind requires a host address")
		}
		st.bindHost = strings.TrimSpace(args[0])
		return nil, nil

	case "http_port":
		if len(args) != 1 {
			return nil, fmt.Errorf("http_port requires a port number")
		}
		p, err := host.ParsePort(args[0], "http_port")
		if err != nil {
			return nil, err
		}
		st.httpPort = p
		return nil, nil

	case "https_port":
		if len(args) != 1 {
			return nil, fmt.Errorf("https_port requires a port number")
		}
		p, err := host.ParsePort(args[0], "https_port")
		if err != nil {
			return nil, err
		}
		st.httpsPort = p
		st.httpsPortSet = true
		return nil, nil

	case "admin":
		if len(args) != 1 {
			return nil, fmt.Errorf("admin requires a listen address or 'off'")
		}
		val := strings.TrimSpace(args[0])
		if strings.EqualFold(val, "off") || strings.EqualFold(val, "false") || strings.EqualFold(val, "none") {
			st.adminAddr = ""
		} else {
			st.adminAddr = val
		}
		return nil, nil

	case "redis", "redis_url":
		if len(args) != 1 {
			return nil, fmt.Errorf("redis requires a Redis URL")
		}
		st.redisURL = strings.TrimSpace(args[0])
		return nil, nil

	case "secret":
		if len(args) != 1 {
			return nil, fmt.Errorf("secret requires a value")
		}
		st.secret = args[0]
		return nil, nil

	case "remote_rules_refresh", "rules_refresh":
		if len(args) != 1 {
			return nil, fmt.Errorf("%s requires seconds or a duration (e.g. 5m)", name)
		}
		sec, err := ParseDurationSeconds(args[0], name)
		if err != nil {
			return nil, err
		}
		st.remoteRulesRefresh = sec
		return nil, nil

	case "rules_watch_interval":
		if len(args) != 1 {
			return nil, fmt.Errorf("%s requires seconds or a duration (e.g. 2s)", name)
		}
		sec, err := ParseDurationSeconds(args[0], name)
		if err != nil {
			return nil, err
		}
		st.rulesWatchInterval = sec
		return nil, nil

	case "encode", "log":
		// Tolerate Caddyfile v2 compression/logging directives.
		return nil, nil

	case "trusted_proxies":
		if len(args) == 0 {
			return nil, fmt.Errorf("trusted_proxies requires at least one IP or CIDR")
		}
		st.trustedProxies = append(st.trustedProxies, args...)
		return nil, nil

	case "verify_url":
		if len(args) != 1 {
			return nil, fmt.Errorf("verify_url requires a hostname or https:// URL")
		}
		st.verify.URL = strings.TrimSpace(args[0])
		return nil, nil

	case "verify_port":
		if len(args) != 1 {
			return nil, fmt.Errorf("verify_port requires a port number")
		}
		p, err := host.ParsePort(args[0], "verify_port")
		if err != nil {
			return nil, err
		}
		st.verify.Port = p
		return nil, nil
	}

	return nil, fmt.Errorf("unknown directive: %s", words[0])
}

// ParseCaddyfile parses a Gymbagfile/Caddyfile document into a validated Config.
func ParseCaddyfile(text string) (Config, error) {
	return ParseCaddyfileWithBaseDir(text, "")
}

// ParseCaddyfileWithBaseDir parses a Gymbagfile/Caddyfile document resolving relative imports against baseDir.
func ParseCaddyfileWithBaseDir(text, baseDir string) (Config, error) {
	tokens, err := tokenizeCaddyfile(text)
	if err != nil {
		return Config{}, err
	}
	st := &parseState{
		tls:                DefaultTLSConfig(),
		intel:              DefaultIntelConfig(),
		remoteRulesRefresh: DefaultRemoteRulesRefresh,
		rulesWatchInterval: DefaultRulesWatchInterval,
		httpPort:           80,
		httpsPort:          443,
		defaultWAF:         DefaultSiteWAFConfig(),
		defaultProtect:     true,
		snippets:           make(map[string][]statementItem),
	}

	rawItems, err := parseItems(tokens)
	if err != nil {
		return Config{}, err
	}

	// Pass 1: collect top-level reusable snippets `(name) { ... }`.
	var nonSnippetItems []statementItem
	for _, it := range rawItems {
		if it.sub != nil {
			if sname, ok := isSnippetHeader(it.words); ok {
				st.snippets[sname] = it.sub.body
				continue
			}
		}
		nonSnippetItems = append(nonSnippetItems, it)
	}

	// Pass 2: expand `import` directives across top-level and nested blocks.
	items, err := expandImports(nonSnippetItems, baseDir, st.snippets, 0)
	if err != nil {
		return Config{}, err
	}

	hasBraces := false
	for _, it := range items {
		if it.sub != nil {
			hasBraces = true
			break
		}
	}

	var sites []SiteConfig

	if hasBraces {
		for _, item := range items {
			if item.sub == nil {
				return Config{}, fmt.Errorf("unexpected token or missing '{' in config file near %q", strings.Join(item.words, " "))
			}
			block := item.sub
			if len(block.header) == 0 {
				for _, stmt := range block.body {
					upstreams, err := applyGlobalDirective(stmt, st, &st.defaultWAF, &st.defaultProtect, nil, nil, nil, true)
					if err != nil {
						return Config{}, err
					}
					if len(upstreams) > 0 {
						st.flatUpstreams = upstreams
					}
				}
				continue
			}

			if strings.EqualFold(block.header[0], "ssh") && len(block.header) <= 2 {
				sshCfg, sErr := parseSSHBlock(block, st)
				if sErr != nil {
					return Config{}, sErr
				}
				st.ssh = &sshCfg
				continue
			}

			if (strings.EqualFold(block.header[0], "minecraft") || strings.EqualFold(block.header[0], "mc")) && len(block.header) <= 2 {
				mcCfg, mErr := parseMinecraftBlock(block, st)
				if mErr != nil {
					return Config{}, mErr
				}
				st.minecraft = &mcCfg
				continue
			}

			var siteHosts []string
			for _, rawAddr := range block.header {
				for _, part := range strings.Split(rawAddr, ",") {
					if strings.TrimSpace(part) == "" {
						continue
					}
					h, port, hasPort, err := parseCaddySiteAddressToken(part)
					if err != nil {
						return Config{}, err
					}
					siteHosts = append(siteHosts, h)
					if hasPort && !st.httpsPortSet {
						st.httpsPort = port
						st.httpsPortSet = true
					}
				}
			}
			if len(siteHosts) == 0 {
				return Config{}, fmt.Errorf("site block requires at least one domain")
			}

			siteWAF := cloneSiteWAF(st.defaultWAF)
			siteProtect := st.defaultProtect
			protectExplicit := false
			var siteUpstreams []string
			var siteCert, siteKey string
			var siteAuth *ForwardAuthConfig
			matchers := make(map[string][]string)
			siteScope := newRouteScope(matchers)
			siteHeaders := make(map[string]string)

			for _, stmt := range block.body {
				if len(stmt.words) == 0 {
					continue
				}
				firstWord := stmt.words[0]
				lowerFirst := strings.ToLower(firstWord)

				// Named matcher: `@api path /api/*` or `@api { ... }`
				if strings.HasPrefix(firstWord, "@") {
					mName, mPaths, mErr := parseNamedMatcher(stmt)
					if mErr != nil {
						return Config{}, mErr
					}
					matchers[mName] = mPaths
					continue
				}

				if lowerFirst == "authelia" || lowerFirst == "forward_auth" {
					authCfg, handled, aErr := parseAuthDirective(stmt, st.authelia, matchers, false)
					if aErr != nil {
						return Config{}, aErr
					}
					if handled {
						siteAuth = authCfg
						continue
					}
				}

				if stmt.sub != nil {
					switch lowerFirst {
					case "handle", "handle_path", "route":
						routes, hErr := parseHandleScope(stmt, matchers, lowerFirst == "handle_path", st.authelia)
						if hErr != nil {
							return Config{}, hErr
						}
						siteScope.routes = append(siteScope.routes, routes...)
						continue
					case "reverse_proxy", "upstream", "proxy":
						paths, targets, rpErr := parseReverseProxyDirective(stmt, matchers)
						if rpErr != nil {
							return Config{}, rpErr
						}
						if len(paths) == 1 && paths[0] == "" {
							siteUpstreams = append(siteUpstreams, targets...)
						} else {
							emitProxyRoutes(siteScope, paths, targets)
						}
						continue
					case "file_server":
						if fsErr := applyFileServerItem(siteScope, stmt); fsErr != nil {
							return Config{}, fsErr
						}
						continue
					case "respond":
						paths, resp, rErr := parseRespondBlock(stmt, matchers)
						if rErr != nil {
							return Config{}, rErr
						}
						emitRespondRoutes(siteScope, paths, resp)
						continue
					case "header":
						hasM, mPaths, muts, hErr := parseHeaderBlockOrLine(stmt, matchers)
						if hErr != nil {
							return Config{}, hErr
						}
						if !hasM || (len(mPaths) == 1 && mPaths[0] == "") {
							for k, v := range muts {
								siteHeaders[k] = v
							}
						} else {
							for k, v := range muts {
								if siteScope.headers == nil {
									siteScope.headers = make(map[string]string)
								}
								siteScope.headers[k] = v
							}
						}
						continue
					}
				} else {
					switch lowerFirst {
					case "reverse_proxy", "upstream", "proxy":
						paths, targets, rpErr := parseReverseProxyDirective(stmt, matchers)
						if rpErr != nil {
							return Config{}, rpErr
						}
						if len(paths) == 1 && paths[0] == "" {
							siteUpstreams = append(siteUpstreams, targets...)
						} else {
							emitProxyRoutes(siteScope, paths, targets)
						}
						continue
					case "root":
						if rErr := applyRootDirective(siteScope, stmt.words[1:], matchers); rErr != nil {
							return Config{}, rErr
						}
						continue
					case "try_files":
						if fb := extractTryFilesFallback(stmt.words[1:]); fb != "" {
							siteScope.tryFiles = fb
						}
						continue
					case "file_server":
						if fsErr := applyFileServerItem(siteScope, stmt); fsErr != nil {
							return Config{}, fsErr
						}
						continue
					case "respond":
						paths, resp, rErr := parseRespondArgs(stmt.words[1:], matchers)
						if rErr != nil {
							return Config{}, rErr
						}
						emitRespondRoutes(siteScope, paths, resp)
						continue
					case "redir":
						paths, redir, rErr := parseRedirArgs(stmt.words[1:], matchers)
						if rErr != nil {
							return Config{}, rErr
						}
						emitRedirRoutes(siteScope, paths, redir)
						continue
					case "header":
						hasM, mPaths, muts, hErr := parseHeaderBlockOrLine(stmt, matchers)
						if hErr != nil {
							return Config{}, hErr
						}
						if !hasM || (len(mPaths) == 1 && mPaths[0] == "") {
							for k, v := range muts {
								siteHeaders[k] = v
							}
						} else {
							for k, v := range muts {
								if siteScope.headers == nil {
									siteScope.headers = make(map[string]string)
								}
								siteScope.headers[k] = v
							}
						}
						continue
					}
				}

				upstreams, err := applyGlobalDirective(stmt, st, &siteWAF, &siteProtect, &protectExplicit, &siteCert, &siteKey, false)
				if err != nil {
					return Config{}, err
				}
				if len(upstreams) > 0 {
					siteUpstreams = append(siteUpstreams, upstreams...)
				}
			}

			siteRoutes := siteScope.finalize()
			if len(siteUpstreams) == 0 && len(siteRoutes) == 0 && len(st.flatUpstreams) > 0 {
				siteUpstreams = append(siteUpstreams, st.flatUpstreams...)
			}
			if len(siteUpstreams) == 0 && len(siteRoutes) == 0 {
				return Config{}, fmt.Errorf("site block for %s requires reverse_proxy <upstream> or a route handler (file_server, respond, redir, handle)", strings.Join(siteHosts, ", "))
			}
			if len(siteHeaders) == 0 {
				siteHeaders = nil
			}

			for _, h := range siteHosts {
				effectiveProtect := siteProtect
				if !protectExplicit && st.protectedFilter != nil {
					_, effectiveProtect = st.protectedFilter[h]
				}
				clonedRoutes := make([]RouteConfig, len(siteRoutes))
				for i, r := range siteRoutes {
					clonedRoutes[i] = r.Clone()
				}
				sites = append(sites, SiteConfig{
					Domain:    h,
					Upstreams: append([]string(nil), siteUpstreams...),
					Routes:    clonedRoutes,
					Headers:   cloneHeaderMap(siteHeaders),
					Auth:      siteAuth.Clone(),
					Protect:   effectiveProtect,
					WAF:       cloneSiteWAF(siteWAF),
					CertFile:  siteCert,
					KeyFile:   siteKey,
				})
			}
		}
	} else {
		protectExplicit := false
		for idx, stmt := range items {
			words := stmt.words
			if len(words) == 0 {
				continue
			}
			_, known := knownDirectiveNames[strings.ToLower(words[0])]
			isArrow := len(words) == 3 && words[1] == "->"
			isBareUpstream := len(words) == 1 && (strings.Contains(words[0], "://") || strings.HasPrefix(words[0], ":"))
			if idx == 0 && !known && !isArrow && !isBareUpstream {
				for _, tok := range words {
					for _, part := range strings.Split(tok, ",") {
						if strings.TrimSpace(part) == "" {
							continue
						}
						h, port, hasPort, err := parseCaddySiteAddressToken(part)
						if err != nil {
							return Config{}, err
						}
						st.flatDomains = append(st.flatDomains, h)
						if hasPort && !st.httpsPortSet {
							st.httpsPort = port
							st.httpsPortSet = true
						}
					}
				}
				continue
			}
			upstreams, err := applyGlobalDirective(stmt, st, &st.defaultWAF, &st.defaultProtect, &protectExplicit, nil, nil, true)
			if err != nil {
				return Config{}, err
			}
			if len(upstreams) > 0 {
				st.flatUpstreams = upstreams
			}
		}

		if len(st.flatDomains) == 0 || len(st.flatUpstreams) == 0 {
			return Config{}, fmt.Errorf("config file must specify both a domain and an upstream URL")
		}
		for _, h := range st.flatDomains {
			effectiveProtect := st.defaultProtect
			if !protectExplicit && st.protectedFilter != nil {
				_, effectiveProtect = st.protectedFilter[h]
			}
			sites = append(sites, SiteConfig{
				Domain:    h,
				Upstreams: append([]string(nil), st.flatUpstreams...),
				Auth:      st.authelia.Clone(),
				Protect:   effectiveProtect,
				WAF:       cloneSiteWAF(st.defaultWAF),
			})
		}
	}

	if len(sites) == 0 && (st.ssh == nil || !st.ssh.Enabled) && (st.minecraft == nil || !st.minecraft.Enabled) {
		return Config{}, fmt.Errorf("config file must define at least one site, ssh, or minecraft service")
	}

	cfg := Config{
		Sites:              sites,
		SSH:                st.ssh,
		Minecraft:          st.minecraft,
		Verify:             st.verify,
		Authelia:           st.authelia,
		BindHost:           st.bindHost,
		HTTPPort:           st.httpPort,
		HTTPSPort:          st.httpsPort,
		AdminAddr:          st.adminAddr,
		TLS:                st.tls,
		Intel:              st.intel,
		RemoteRulesRefresh: st.remoteRulesRefresh,
		RulesWatchInterval: st.rulesWatchInterval,
		RedisURL:           st.redisURL,
		Secret:             st.secret,
		TrustedProxies:     st.trustedProxies,
		Defaults:           st.defaultWAF,
		BaseDir:            baseDir,
	}
	if cfg.BindHost == "" {
		cfg.BindHost = DefaultBindHostForConfig(cfg)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

type tcpBlockFields struct {
	enabled          bool
	port             int
	upstream         string
	serverProperties string
	serverJar        string
	javaPath         string
	jvmArgs          []string
	serverArgs       []string
	workDir          string
	autoRestart      *bool
	protect          bool
	waf              SiteWAFConfig
	verifyURL        string
	verifyPort       int
}

func parseTCPServiceBlock(block *parsedBlock, service string, defaultPort int, st *parseState) (tcpBlockFields, error) {
	out := tcpBlockFields{
		enabled: true,
		port:    defaultPort,
		protect: st.defaultProtect,
		waf:     cloneSiteWAF(st.defaultWAF),
	}
	// Default verify level for TCP services is 3 (suspicious & ai) unless overridden.
	if out.waf.VerifyLevel == VerifyLevelAlways {
		out.waf.VerifyLevel = VerifyLevelSuspiciousAndAI
	}
	out.waf.RateLimit = 60
	out.waf.RateWindow = 60

	if len(block.header) == 2 {
		p, err := host.ParsePort(block.header[1], service+" port")
		if err != nil {
			return out, err
		}
		out.port = p
	}

	for _, stmt := range block.body {
		if stmt.sub != nil {
			if strings.EqualFold(stmt.words[0], "rule") {
				rule, err := parseRuleBlock(stmt.sub)
				if err != nil {
					return out, err
				}
				out.waf.Rules = append(out.waf.Rules, rule)
				continue
			}
			return out, fmt.Errorf("unexpected nested block inside %s: %s", service, stmt.words[0])
		}
		if len(stmt.words) == 0 {
			continue
		}
		dname := strings.ToLower(stmt.words[0])
		args := stmt.words[1:]
		switch dname {
		case "enabled":
			if len(args) != 1 {
				return out, fmt.Errorf("%s enabled requires true or false", service)
			}
			b, err := parseBool(args[0], service+" enabled")
			if err != nil {
				return out, err
			}
			out.enabled = b
		case "port":
			if len(args) != 1 {
				return out, fmt.Errorf("%s port requires a port number", service)
			}
			p, err := host.ParsePort(args[0], service+" port")
			if err != nil {
				return out, err
			}
			out.port = p
		case "upstream", "reverse_proxy", "proxy":
			if len(args) != 1 {
				return out, fmt.Errorf("%s %s requires a single host:port target", service, dname)
			}
			up, err := NormalizeTCPUpstream(args[0], service+" upstream")
			if err != nil {
				return out, err
			}
			out.upstream = up
		case "server_properties":
			if service != "minecraft" {
				return out, fmt.Errorf("server_properties is only valid inside minecraft block")
			}
			if len(args) != 1 {
				return out, fmt.Errorf("server_properties requires a file path")
			}
			out.serverProperties = strings.TrimSpace(args[0])
		case "server_jar", "jar":
			if service != "minecraft" {
				return out, fmt.Errorf("%s is only valid inside minecraft block", dname)
			}
			if len(args) != 1 {
				return out, fmt.Errorf("%s requires a path to the server .jar file", dname)
			}
			out.serverJar = strings.TrimSpace(args[0])
		case "java_path", "java":
			if service != "minecraft" {
				return out, fmt.Errorf("%s is only valid inside minecraft block", dname)
			}
			if len(args) != 1 {
				return out, fmt.Errorf("%s requires a path to the java binary", dname)
			}
			out.javaPath = strings.TrimSpace(args[0])
		case "jvm_args":
			if service != "minecraft" {
				return out, fmt.Errorf("jvm_args is only valid inside minecraft block")
			}
			if len(args) == 0 {
				return out, fmt.Errorf("jvm_args requires at least one argument")
			}
			if len(args) == 1 && strings.ContainsAny(args[0], " \t") {
				out.jvmArgs = append(out.jvmArgs, strings.Fields(args[0])...)
			} else {
				out.jvmArgs = append(out.jvmArgs, args...)
			}
		case "server_args":
			if service != "minecraft" {
				return out, fmt.Errorf("server_args is only valid inside minecraft block")
			}
			if len(args) == 0 {
				return out, fmt.Errorf("server_args requires at least one argument")
			}
			if len(args) == 1 && strings.ContainsAny(args[0], " \t") {
				out.serverArgs = append(out.serverArgs, strings.Fields(args[0])...)
			} else {
				out.serverArgs = append(out.serverArgs, args...)
			}
		case "work_dir":
			if service != "minecraft" {
				return out, fmt.Errorf("work_dir is only valid inside minecraft block")
			}
			if len(args) != 1 {
				return out, fmt.Errorf("work_dir requires a directory path")
			}
			out.workDir = strings.TrimSpace(args[0])
		case "auto_restart":
			if service != "minecraft" {
				return out, fmt.Errorf("auto_restart is only valid inside minecraft block")
			}
			if len(args) != 1 {
				return out, fmt.Errorf("auto_restart requires on/off or true/false")
			}
			b, err := parseBool(args[0], "minecraft auto_restart")
			if err != nil {
				return out, err
			}
			out.autoRestart = &b
		case "verify_url":
			if len(args) != 1 {
				return out, fmt.Errorf("%s verify_url requires a hostname or https:// URL", service)
			}
			out.verifyURL = strings.TrimSpace(args[0])
		case "verify_port":
			if len(args) != 1 {
				return out, fmt.Errorf("%s verify_port requires a port number", service)
			}
			p, err := host.ParsePort(args[0], service+" verify_port")
			if err != nil {
				return out, err
			}
			out.verifyPort = p
		case "protect", "protected":
			if len(args) != 1 {
				return out, fmt.Errorf("%s protect requires on/off or true/false", service)
			}
			b, err := parseBool(args[0], service+" protect")
			if err != nil {
				return out, err
			}
			out.protect = b
		default:
			handled, err := applyWAFDirective(dname, args, &out.waf, &st.intel, &out.protect, nil)
			if err != nil {
				return out, err
			}
			if !handled {
				return out, fmt.Errorf("unknown directive inside %s block: %s", service, stmt.words[0])
			}
		}
	}
	return out, nil
}

func parseSSHBlock(block *parsedBlock, st *parseState) (SSHConfig, error) {
	f, err := parseTCPServiceBlock(block, "ssh", DefaultSSHPort, st)
	if err != nil {
		return SSHConfig{}, err
	}
	return SSHConfig{
		Enabled:      f.enabled,
		Port:         f.port,
		Upstream:     f.upstream,
		Protect:      f.protect,
		VerifyLevel:  f.waf.VerifyLevel,
		Difficulty:   f.waf.Difficulty,
		ChallengeTTL: f.waf.ChallengeTTL,
		ClearanceTTL: f.waf.ClearanceTTL,
		RateLimit:    f.waf.RateLimit,
		RateWindow:   f.waf.RateWindow,
		VerifyURL:    f.verifyURL,
		VerifyPort:   f.verifyPort,
		Rules:        f.waf.Rules,
	}, nil
}

func parseMinecraftBlock(block *parsedBlock, st *parseState) (MinecraftConfig, error) {
	f, err := parseTCPServiceBlock(block, "minecraft", DefaultMinecraftPort, st)
	if err != nil {
		return MinecraftConfig{}, err
	}
	return MinecraftConfig{
		Enabled:          f.enabled,
		Port:             f.port,
		Upstream:         f.upstream,
		ServerProperties: f.serverProperties,
		ServerJar:        f.serverJar,
		JavaPath:         f.javaPath,
		JVMArgs:          f.jvmArgs,
		ServerArgs:       f.serverArgs,
		WorkDir:          f.workDir,
		AutoRestart:      f.autoRestart,
		Protect:          f.protect,
		VerifyLevel:      f.waf.VerifyLevel,
		Difficulty:       f.waf.Difficulty,
		ChallengeTTL:     f.waf.ChallengeTTL,
		ClearanceTTL:     f.waf.ClearanceTTL,
		RateLimit:        f.waf.RateLimit,
		RateWindow:       f.waf.RateWindow,
		VerifyURL:        f.verifyURL,
		VerifyPort:       f.verifyPort,
		Rules:            f.waf.Rules,
	}, nil
}

func cloneSiteWAF(in SiteWAFConfig) SiteWAFConfig {
	out := in
	if in.DefaultRules != nil {
		v := *in.DefaultRules
		out.DefaultRules = &v
	}
	if len(in.Rules) > 0 {
		out.Rules = make([]CustomRule, len(in.Rules))
		for i, r := range in.Rules {
			out.Rules[i] = r.Clone()
		}
	}
	out.RemoteRules = slices.Clone(in.RemoteRules)
	out.RuleSources = slices.Clone(in.RuleSources)
	out.CustomAIBots = slices.Clone(in.CustomAIBots)
	out.CustomSuspiciousUAs = slices.Clone(in.CustomSuspiciousUAs)
	out.CustomSuspiciousPaths = slices.Clone(in.CustomSuspiciousPaths)
	return out
}

func quoteIfNeeded(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, " \t\n\"'{};#") {
		return strconv.Quote(s)
	}
	return s
}

func formatCustomRule(b *strings.Builder, r CustomRule) {
	// Count distinct matcher groups to choose shorthand one-liner vs compound rule block.
	groups := 0
	if len(r.IPs) > 0 {
		groups++
	}
	if len(r.UserAgents) > 0 {
		groups++
	}
	if len(r.Paths) > 0 {
		groups++
	}
	if len(r.Queries) > 0 {
		groups++
	}
	if len(r.Methods) > 0 {
		groups++
	}
	groups += len(r.Headers)
	groups += len(r.Cookies)

	if groups == 1 && r.Name == "" {
		switch {
		case len(r.IPs) > 0:
			b.WriteString(fmt.Sprintf("    %s ip %s\n", r.Action, strings.Join(r.IPs, " ")))
		case len(r.UserAgents) > 0:
			vals := make([]string, len(r.UserAgents))
			for i, v := range r.UserAgents {
				vals[i] = quoteIfNeeded(v)
			}
			b.WriteString(fmt.Sprintf("    %s user_agent %s\n", r.Action, strings.Join(vals, " ")))
		case len(r.Paths) > 0:
			vals := make([]string, len(r.Paths))
			for i, v := range r.Paths {
				vals[i] = quoteIfNeeded(v)
			}
			b.WriteString(fmt.Sprintf("    %s path %s\n", r.Action, strings.Join(vals, " ")))
		case len(r.Queries) > 0:
			vals := make([]string, len(r.Queries))
			for i, v := range r.Queries {
				vals[i] = quoteIfNeeded(v)
			}
			b.WriteString(fmt.Sprintf("    %s query %s\n", r.Action, strings.Join(vals, " ")))
		case len(r.Methods) > 0:
			b.WriteString(fmt.Sprintf("    %s method %s\n", r.Action, strings.Join(r.Methods, " ")))
		case len(r.Headers) == 1:
			b.WriteString(fmt.Sprintf("    %s header %s %s\n", r.Action, quoteIfNeeded(r.Headers[0].Name), quoteIfNeeded(r.Headers[0].Pattern)))
		case len(r.Cookies) == 1:
			b.WriteString(fmt.Sprintf("    %s cookie %s %s\n", r.Action, quoteIfNeeded(r.Cookies[0].Name), quoteIfNeeded(r.Cookies[0].Pattern)))
		}
		return
	}

	if r.Name != "" {
		b.WriteString(fmt.Sprintf("    rule %s {\n", quoteIfNeeded(r.Name)))
	} else {
		b.WriteString("    rule {\n")
	}
	b.WriteString(fmt.Sprintf("        action %s\n", r.Action))
	if len(r.IPs) > 0 {
		b.WriteString(fmt.Sprintf("        ip %s\n", strings.Join(r.IPs, " ")))
	}
	if len(r.UserAgents) > 0 {
		vals := make([]string, len(r.UserAgents))
		for i, v := range r.UserAgents {
			vals[i] = quoteIfNeeded(v)
		}
		b.WriteString(fmt.Sprintf("        user_agent %s\n", strings.Join(vals, " ")))
	}
	if len(r.Paths) > 0 {
		vals := make([]string, len(r.Paths))
		for i, v := range r.Paths {
			vals[i] = quoteIfNeeded(v)
		}
		b.WriteString(fmt.Sprintf("        path %s\n", strings.Join(vals, " ")))
	}
	if len(r.Queries) > 0 {
		vals := make([]string, len(r.Queries))
		for i, v := range r.Queries {
			vals[i] = quoteIfNeeded(v)
		}
		b.WriteString(fmt.Sprintf("        query %s\n", strings.Join(vals, " ")))
	}
	if len(r.Methods) > 0 {
		b.WriteString(fmt.Sprintf("        method %s\n", strings.Join(r.Methods, " ")))
	}
	for _, h := range r.Headers {
		b.WriteString(fmt.Sprintf("        header %s %s\n", quoteIfNeeded(h.Name), quoteIfNeeded(h.Pattern)))
	}
	for _, c := range r.Cookies {
		b.WriteString(fmt.Sprintf("        cookie %s %s\n", quoteIfNeeded(c.Name), quoteIfNeeded(c.CookiePatternOrDefault())))
	}
	b.WriteString("    }\n")
}

func (kv KeyValueMatch) CookiePatternOrDefault() string {
	return kv.Pattern
}

// FormatCaddyfile formats a Gymbagfile/Caddyfile into canonical 4-space indented layout.
func FormatCaddyfile(text string) (string, error) {
	cfg, err := ParseCaddyfile(text)
	if err != nil {
		return "", err
	}
	return FormatConfig(cfg), nil
}

// FormatConfig renders a Config into canonical 4-space indented Gymbagfile format.
func FormatConfig(cfg Config) string {
	var b strings.Builder
	defTLS := DefaultTLSConfig()
	defWAF := DefaultSiteWAFConfig()
	defIntel := DefaultIntelConfig()

	var globalLines []string
	if cfg.TLS.ACMEEmail != "" {
		globalLines = append(globalLines, fmt.Sprintf("    email %s", cfg.TLS.ACMEEmail))
	}
	if cfg.TLS.Provider == "custom" {
		globalLines = append(globalLines, fmt.Sprintf("    ssl_provider %s", quoteIfNeeded(cfg.TLS.ACMEDirectory)))
	} else if cfg.TLS.Provider != "" {
		globalLines = append(globalLines, fmt.Sprintf("    ssl_provider %s", cfg.TLS.Provider))
	}
	if cfg.TLS.ACMEDirectory != defTLS.ACMEDirectory && cfg.TLS.Provider != "custom" {
		globalLines = append(globalLines, fmt.Sprintf("    acme_directory %s", cfg.TLS.ACMEDirectory))
	}
	if cfg.TLS.Mode != defTLS.Mode && cfg.TLS.Mode != "file" {
		impliedMode := ""
		if cfg.TLS.Provider == "local" {
			impliedMode = "local"
		} else if cfg.TLS.Provider != "" {
			impliedMode = "acme"
		}
		if cfg.TLS.Mode != impliedMode {
			globalLines = append(globalLines, fmt.Sprintf("    tls %s", cfg.TLS.Mode))
		}
	} else if cfg.TLS.Mode == "file" && (cfg.TLS.CertFile != defTLS.CertFile || cfg.TLS.KeyFile != defTLS.KeyFile) {
		globalLines = append(globalLines, fmt.Sprintf("    tls %s %s", quoteIfNeeded(cfg.TLS.CertFile), quoteIfNeeded(cfg.TLS.KeyFile)))
	}
	if cfg.TLS.Mode != "file" && cfg.TLS.KeyFile != defTLS.KeyFile {
		globalLines = append(globalLines, fmt.Sprintf("    ssl_key %s", quoteIfNeeded(cfg.TLS.KeyFile)))
	}
	if cfg.TLS.KeyPassword != "" {
		globalLines = append(globalLines, fmt.Sprintf("    key_password %s", quoteIfNeeded(cfg.TLS.KeyPassword)))
	}
	if cfg.TLS.AccountKeyFile != "" {
		globalLines = append(globalLines, fmt.Sprintf("    account_key %s", quoteIfNeeded(cfg.TLS.AccountKeyFile)))
	}
	if cfg.TLS.EABKID != "" {
		globalLines = append(globalLines, fmt.Sprintf("    eab_kid %s", quoteIfNeeded(cfg.TLS.EABKID)))
	}
	if cfg.TLS.EABHMACKey != "" {
		globalLines = append(globalLines, fmt.Sprintf("    eab_hmac_key %s", quoteIfNeeded(cfg.TLS.EABHMACKey)))
	}
	if cfg.TLS.AutoHosts != nil {
		autoHostsVal := "off"
		if *cfg.TLS.AutoHosts {
			autoHostsVal = "on"
		}
		globalLines = append(globalLines, fmt.Sprintf("    auto_hosts %s", autoHostsVal))
	}
	if cfg.TLS.CertDir != defTLS.CertDir {
		globalLines = append(globalLines, fmt.Sprintf("    cert_dir %s", cfg.TLS.CertDir))
	}
	if cfg.TLS.RenewInterval != defTLS.RenewInterval {
		globalLines = append(globalLines, fmt.Sprintf("    renew_interval %d", cfg.TLS.RenewInterval))
	}
	if cfg.TLS.RenewWithinDays != defTLS.RenewWithinDays {
		globalLines = append(globalLines, fmt.Sprintf("    renew_within_days %d", cfg.TLS.RenewWithinDays))
	}
	if cfg.HTTPPort != 80 {
		globalLines = append(globalLines, fmt.Sprintf("    http_port %d", cfg.HTTPPort))
	}
	if cfg.HTTPSPort != 443 {
		globalLines = append(globalLines, fmt.Sprintf("    https_port %d", cfg.HTTPSPort))
	}
	if cfg.BindHost != DefaultBindHostForConfig(cfg) {
		globalLines = append(globalLines, fmt.Sprintf("    bind %s", cfg.BindHost))
	}
	if cfg.Verify.URL != "" {
		globalLines = append(globalLines, fmt.Sprintf("    verify_url %s", quoteIfNeeded(cfg.Verify.URL)))
	}
	if cfg.Verify.Port != 0 && cfg.Verify.Port != DefaultVerifyPort {
		globalLines = append(globalLines, fmt.Sprintf("    verify_port %d", cfg.Verify.Port))
	}
	if cfg.Verify.CertFile != "" && cfg.Verify.KeyFile != "" {
		globalLines = append(globalLines, fmt.Sprintf("    verify_server {\n        tls %s %s\n    }", quoteIfNeeded(cfg.Verify.CertFile), quoteIfNeeded(cfg.Verify.KeyFile)))
	}
	if cfg.AdminAddr != "" {
		globalLines = append(globalLines, fmt.Sprintf("    admin %s", cfg.AdminAddr))
	}
	if cfg.RedisURL != "" {
		globalLines = append(globalLines, fmt.Sprintf("    redis %s", cfg.RedisURL))
	}
	if cfg.TLS.HSTS != nil {
		globalLines = append(globalLines, fmt.Sprintf("    hsts %t", *cfg.TLS.HSTS))
	}
	if cfg.Intel.IPSecurity != defIntel.IPSecurity {
		globalLines = append(globalLines, fmt.Sprintf("    ip_security %t", cfg.Intel.IPSecurity))
	}
	if cfg.Intel.IPSecurityAction != defIntel.IPSecurityAction {
		globalLines = append(globalLines, fmt.Sprintf("    ip_security_action %s", cfg.Intel.IPSecurityAction))
	}
	if cfg.Intel.LiveReputation != defIntel.LiveReputation {
		globalLines = append(globalLines, fmt.Sprintf("    ip_reputation %t", cfg.Intel.LiveReputation))
	}
	if cfg.Intel.AIIPList != defIntel.AIIPList {
		globalLines = append(globalLines, fmt.Sprintf("    ai_ip_list %t", cfg.Intel.AIIPList))
	}
	if cfg.Intel.VPNDetection != defIntel.VPNDetection {
		globalLines = append(globalLines, fmt.Sprintf("    vpn_detection %t", cfg.Intel.VPNDetection))
	}
	if cfg.Intel.RefreshInterval != defIntel.RefreshInterval {
		globalLines = append(globalLines, fmt.Sprintf("    intel_refresh %d", cfg.Intel.RefreshInterval))
	}
	for _, u := range cfg.Intel.ThreatFeeds {
		globalLines = append(globalLines, fmt.Sprintf("    threat_feed %s", quoteIfNeeded(u)))
	}
	for _, u := range cfg.Intel.AIIPFeeds {
		globalLines = append(globalLines, fmt.Sprintf("    ai_ip_feed %s", quoteIfNeeded(u)))
	}
	for _, u := range cfg.Intel.VPNFeeds {
		globalLines = append(globalLines, fmt.Sprintf("    vpn_feed %s", quoteIfNeeded(u)))
	}
	if cfg.RemoteRulesRefresh != 0 && cfg.RemoteRulesRefresh != DefaultRemoteRulesRefresh {
		globalLines = append(globalLines, fmt.Sprintf("    remote_rules_refresh %d", cfg.RemoteRulesRefresh))
	}
	if cfg.RulesWatchInterval != 0 && cfg.RulesWatchInterval != DefaultRulesWatchInterval {
		globalLines = append(globalLines, fmt.Sprintf("    rules_watch_interval %d", cfg.RulesWatchInterval))
	}
	for _, rs := range cfg.Defaults.RuleSources {
		globalLines = append(globalLines, formatRuleSourceLines("    ", rs)...)
	}
	if cfg.Authelia != nil {
		globalLines = append(globalLines, formatAuthLines("    ", cfg.Authelia, nil)...)
	}

	if len(globalLines) > 0 {
		b.WriteString("{\n")
		for _, line := range globalLines {
			b.WriteString(line)
			b.WriteByte('\n')
		}
		b.WriteString("}\n\n")
	}

	for idx, site := range cfg.Sites {
		if idx > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(site.Domain)
		b.WriteString(" {\n")
		for _, rc := range site.Routes {
			formatRouteConfig(&b, rc)
		}
		if len(site.Upstreams) > 0 {
			b.WriteString("    reverse_proxy ")
			b.WriteString(strings.Join(site.Upstreams, " "))
			b.WriteByte('\n')
		}
		formatHeaderLines(&b, "    ", site.Headers)
		for _, ln := range formatAuthLines("    ", site.Auth, cfg.Authelia) {
			b.WriteString(ln)
			b.WriteByte('\n')
		}
		if !site.Protect {
			b.WriteString("    protect off\n")
		} else {
			if site.WAF.VerifyLevel != defWAF.VerifyLevel {
				b.WriteString(fmt.Sprintf("    verify %d\n", int(site.WAF.VerifyLevel)))
			}
			if site.WAF.Difficulty != defWAF.Difficulty {
				b.WriteString(fmt.Sprintf("    difficulty %d\n", site.WAF.Difficulty))
			}
			if site.WAF.RateLimit != defWAF.RateLimit || site.WAF.RateWindow != defWAF.RateWindow {
				b.WriteString(fmt.Sprintf("    rate_limit %d %d\n", site.WAF.RateLimit, site.WAF.RateWindow))
			}
			if site.WAF.ChallengeTTL != defWAF.ChallengeTTL {
				b.WriteString(fmt.Sprintf("    challenge_ttl %d\n", site.WAF.ChallengeTTL))
			}
			if site.WAF.ClearanceTTL != defWAF.ClearanceTTL {
				b.WriteString(fmt.Sprintf("    clearance_ttl %d\n", site.WAF.ClearanceTTL))
			}
			if site.WAF.CookieName != defWAF.CookieName {
				b.WriteString(fmt.Sprintf("    cookie_name %s\n", site.WAF.CookieName))
			}
			if site.WAF.CookieSecure != defWAF.CookieSecure {
				b.WriteString(fmt.Sprintf("    cookie_secure %t\n", site.WAF.CookieSecure))
			}
		}
		if !site.WAF.DefaultRulesEnabled() {
			b.WriteString("    default_rules off\n")
		}
		for _, r := range site.WAF.Rules {
			formatCustomRule(&b, r)
		}
		for _, u := range site.WAF.RemoteRules {
			b.WriteString(fmt.Sprintf("    remote_rules %s\n", quoteIfNeeded(u)))
		}
		for _, rs := range site.WAF.RuleSources {
			if slices.Contains(cfg.Defaults.RuleSources, rs) {
				continue
			}
			for _, ln := range formatRuleSourceLines("    ", rs) {
				b.WriteString(ln)
				b.WriteByte('\n')
			}
		}
		if site.CertFile != "" && site.KeyFile != "" {
			b.WriteString(fmt.Sprintf("    tls %s %s\n", site.CertFile, site.KeyFile))
		}
		b.WriteString("}\n")
	}

	wroteBlock := len(cfg.Sites) > 0
	if cfg.SSHEnabled() {
		if wroteBlock {
			b.WriteByte('\n')
		}
		formatTCPServiceBlock(&b, tcpFormatSpec{
			name:         "ssh",
			defaultPort:  DefaultSSHPort,
			port:         cfg.SSH.Port,
			upstream:     cfg.SSH.Upstream,
			protect:      cfg.SSH.Protect,
			verifyLevel:  cfg.SSH.VerifyLevel,
			difficulty:   cfg.SSH.Difficulty,
			rateLimit:    cfg.SSH.RateLimit,
			rateWindow:   cfg.SSH.RateWindow,
			challengeTTL: cfg.SSH.ChallengeTTL,
			clearanceTTL: cfg.SSH.ClearanceTTL,
			verifyURL:    cfg.SSH.VerifyURL,
			verifyPort:   cfg.SSH.VerifyPort,
			rules:        cfg.SSH.Rules,
		})
		wroteBlock = true
	}
	if cfg.MinecraftEnabled() {
		if wroteBlock {
			b.WriteByte('\n')
		}
		formatTCPServiceBlock(&b, tcpFormatSpec{
			name:         "minecraft",
			defaultPort:  DefaultMinecraftPort,
			port:         cfg.Minecraft.Port,
			upstream:     cfg.Minecraft.Upstream,
			serverProps:  cfg.Minecraft.ServerProperties,
			serverJar:    cfg.Minecraft.ServerJar,
			javaPath:     cfg.Minecraft.JavaPath,
			jvmArgs:      cfg.Minecraft.JVMArgs,
			serverArgs:   cfg.Minecraft.ServerArgs,
			workDir:      cfg.Minecraft.WorkDir,
			autoRestart:  cfg.Minecraft.AutoRestart,
			protect:      cfg.Minecraft.Protect,
			verifyLevel:  cfg.Minecraft.VerifyLevel,
			difficulty:   cfg.Minecraft.Difficulty,
			rateLimit:    cfg.Minecraft.RateLimit,
			rateWindow:   cfg.Minecraft.RateWindow,
			challengeTTL: cfg.Minecraft.ChallengeTTL,
			clearanceTTL: cfg.Minecraft.ClearanceTTL,
			verifyURL:    cfg.Minecraft.VerifyURL,
			verifyPort:   cfg.Minecraft.VerifyPort,
			rules:        cfg.Minecraft.Rules,
		})
	}
	return b.String()
}

type tcpFormatSpec struct {
	name         string
	defaultPort  int
	port         int
	upstream     string
	serverProps  string
	serverJar    string
	javaPath     string
	jvmArgs      []string
	serverArgs   []string
	workDir      string
	autoRestart  *bool
	protect      bool
	verifyLevel  VerifyLevel
	difficulty   int
	rateLimit    int
	rateWindow   int
	challengeTTL int
	clearanceTTL int
	verifyURL    string
	verifyPort   int
	rules        []CustomRule
}

func formatQuotedArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = quoteIfNeeded(a)
	}
	return strings.Join(quoted, " ")
}

func formatTCPServiceBlock(b *strings.Builder, s tcpFormatSpec) {
	b.WriteString(s.name)
	b.WriteString(" {\n")
	if s.port != 0 && s.port != s.defaultPort {
		b.WriteString(fmt.Sprintf("    port %d\n", s.port))
	}
	b.WriteString(fmt.Sprintf("    upstream %s\n", s.upstream))
	if s.serverProps != "" {
		b.WriteString(fmt.Sprintf("    server_properties %s\n", quoteIfNeeded(s.serverProps)))
	}
	if s.serverJar != "" {
		b.WriteString(fmt.Sprintf("    server_jar %s\n", quoteIfNeeded(s.serverJar)))
	}
	if s.javaPath != "" {
		b.WriteString(fmt.Sprintf("    java_path %s\n", quoteIfNeeded(s.javaPath)))
	}
	if len(s.jvmArgs) > 0 {
		b.WriteString(fmt.Sprintf("    jvm_args %s\n", formatQuotedArgs(s.jvmArgs)))
	}
	if len(s.serverArgs) > 0 {
		b.WriteString(fmt.Sprintf("    server_args %s\n", formatQuotedArgs(s.serverArgs)))
	}
	if s.workDir != "" {
		b.WriteString(fmt.Sprintf("    work_dir %s\n", quoteIfNeeded(s.workDir)))
	}
	if s.autoRestart != nil && !*s.autoRestart {
		b.WriteString("    auto_restart off\n")
	}
	if !s.protect {
		b.WriteString("    protect off\n")
	} else {
		b.WriteString(fmt.Sprintf("    verify %d\n", int(s.verifyLevel)))
		if s.difficulty != 0 && s.difficulty != 4 {
			b.WriteString(fmt.Sprintf("    difficulty %d\n", s.difficulty))
		}
		if (s.rateLimit != 0 && s.rateLimit != 60) || (s.rateWindow != 0 && s.rateWindow != 60) {
			b.WriteString(fmt.Sprintf("    rate_limit %d %d\n", s.rateLimit, s.rateWindow))
		}
		if s.challengeTTL != 0 && s.challengeTTL != 300 {
			b.WriteString(fmt.Sprintf("    challenge_ttl %d\n", s.challengeTTL))
		}
		if s.clearanceTTL != 0 && s.clearanceTTL != 86400 {
			b.WriteString(fmt.Sprintf("    clearance_ttl %d\n", s.clearanceTTL))
		}
	}
	if s.verifyURL != "" {
		b.WriteString(fmt.Sprintf("    verify_url %s\n", quoteIfNeeded(s.verifyURL)))
	}
	if s.verifyPort != 0 {
		b.WriteString(fmt.Sprintf("    verify_port %d\n", s.verifyPort))
	}
	for _, r := range s.rules {
		formatCustomRule(b, r)
	}
	b.WriteString("}\n")
}

func formatRuleSourceLines(indent string, rs RuleSource) []string {
	if rs.Name != "" {
		lines := []string{fmt.Sprintf("%sruleset %s {", indent, quoteIfNeeded(rs.Name))}
		lines = append(lines, fmt.Sprintf("%s    source %s", indent, quoteIfNeeded(rs.Source)))
		if rs.Dynamic {
			lines = append(lines, fmt.Sprintf("%s    dynamic on", indent))
		}
		lines = append(lines, indent+"}")
		return lines
	}
	if rs.Dynamic {
		return []string{fmt.Sprintf("%srules_file %s dynamic", indent, quoteIfNeeded(rs.Source))}
	}
	return []string{fmt.Sprintf("%srules_file %s", indent, quoteIfNeeded(rs.Source))}
}

func formatHeaderLines(b *strings.Builder, indent string, headers map[string]string) {
	if len(headers) == 0 {
		return
	}
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if strings.HasPrefix(k, "-") {
			b.WriteString(fmt.Sprintf("%sheader %s\n", indent, quoteIfNeeded(k)))
		} else {
			b.WriteString(fmt.Sprintf("%sheader %s %s\n", indent, quoteIfNeeded(k), quoteIfNeeded(headers[k])))
		}
	}
}

func formatAuthLines(indent string, auth, globalAuth *ForwardAuthConfig) []string {
	if auth == nil {
		return nil
	}
	if globalAuth != nil && reflect.DeepEqual(auth, globalAuth) {
		return []string{indent + "authelia"}
	}

	kw := "authelia"
	if auth.Provider == "forward_auth" {
		kw = "forward_auth"
	}

	isDefaultCopy := slices.Equal(auth.CopyHeaders, DefaultAutheliaCopyHeaders)
	isDefaultURI := auth.URI == "" || (kw == "authelia" && auth.URI == DefaultAutheliaURI)
	hasExtra := !isDefaultURI ||
		(kw == "authelia" && !isDefaultCopy && len(auth.CopyHeaders) > 0) ||
		(kw == "forward_auth" && len(auth.CopyHeaders) > 0) ||
		len(auth.Headers) > 0 ||
		len(auth.Paths) > 0 ||
		len(auth.ExceptPaths) > 0 ||
		auth.PortalURL != "" ||
		(auth.Timeout != 0 && auth.Timeout != DefaultAuthTimeout) ||
		auth.InsecureSkipTLS

	if !hasExtra {
		return []string{fmt.Sprintf("%s%s %s", indent, kw, quoteIfNeeded(auth.URL))}
	}

	lines := []string{fmt.Sprintf("%s%s %s {", indent, kw, quoteIfNeeded(auth.URL))}
	sub := indent + "    "
	if auth.URI != "" && !(kw == "authelia" && auth.URI == DefaultAutheliaURI) {
		lines = append(lines, fmt.Sprintf("%suri %s", sub, quoteIfNeeded(auth.URI)))
	}
	if auth.PortalURL != "" {
		lines = append(lines, fmt.Sprintf("%sportal_url %s", sub, quoteIfNeeded(auth.PortalURL)))
	}
	if len(auth.CopyHeaders) > 0 && !(kw == "authelia" && isDefaultCopy) {
		lines = append(lines, fmt.Sprintf("%scopy_headers %s", sub, formatQuotedArgs(auth.CopyHeaders)))
	}
	if len(auth.Paths) > 0 {
		lines = append(lines, fmt.Sprintf("%spath %s", sub, formatQuotedArgs(auth.Paths)))
	}
	if len(auth.ExceptPaths) > 0 {
		lines = append(lines, fmt.Sprintf("%sexcept %s", sub, formatQuotedArgs(auth.ExceptPaths)))
	}
	if len(auth.Headers) > 0 {
		keys := make([]string, 0, len(auth.Headers))
		for k := range auth.Headers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if strings.HasPrefix(k, "-") {
				lines = append(lines, fmt.Sprintf("%sheader %s", sub, quoteIfNeeded(k)))
			} else {
				lines = append(lines, fmt.Sprintf("%sheader %s %s", sub, quoteIfNeeded(k), quoteIfNeeded(auth.Headers[k])))
			}
		}
	}
	if auth.Timeout != 0 && auth.Timeout != DefaultAuthTimeout {
		lines = append(lines, fmt.Sprintf("%stimeout %d", sub, auth.Timeout))
	}
	if auth.InsecureSkipTLS {
		lines = append(lines, fmt.Sprintf("%sinsecure_skip_tls on", sub))
	}
	lines = append(lines, indent+"}")
	return lines
}

func formatRouteConfig(b *strings.Builder, rc RouteConfig) {
	useBlock := rc.StripPrefix != "" || len(rc.Headers) > 0 || rc.Auth != nil ||
		(rc.Path == "" && rc.HandlerKind() == "proxy") ||
		(rc.Path != "" && rc.HandlerKind() == "static")
	if useBlock {
		headerKw := "handle"
		headerArg := rc.Path
		if rc.StripPrefix != "" {
			headerKw = "handle_path"
			if headerArg == "" {
				headerArg = strings.TrimRight(rc.StripPrefix, "/") + "/*"
			}
		}
		if headerArg != "" {
			b.WriteString(fmt.Sprintf("    %s %s {\n", headerKw, quoteIfNeeded(headerArg)))
		} else {
			b.WriteString(fmt.Sprintf("    %s {\n", headerKw))
		}
		formatHeaderLines(b, "        ", rc.Headers)
		for _, ln := range formatAuthLines("        ", rc.Auth, nil) {
			b.WriteString(ln)
			b.WriteByte('\n')
		}
		formatRouteHandlerBody(b, "        ", rc, false)
		b.WriteString("    }\n")
		return
	}
	formatRouteHandlerBody(b, "    ", rc, true)
}

func formatRouteHandlerBody(b *strings.Builder, indent string, rc RouteConfig, includePath bool) {
	matcherPrefix := ""
	if includePath && rc.Path != "" {
		matcherPrefix = quoteIfNeeded(rc.Path) + " "
	}
	switch rc.HandlerKind() {
	case "proxy":
		b.WriteString(fmt.Sprintf("%sreverse_proxy %s%s\n", indent, matcherPrefix, strings.Join(rc.Upstreams, " ")))
	case "static":
		b.WriteString(fmt.Sprintf("%sroot * %s\n", indent, quoteIfNeeded(rc.Root)))
		if rc.Fallback != "" {
			b.WriteString(fmt.Sprintf("%stry_files {path} %s\n", indent, quoteIfNeeded(rc.Fallback)))
		}
		if len(rc.Index) > 0 {
			b.WriteString(fmt.Sprintf("%sfile_server {\n", indent))
			b.WriteString(fmt.Sprintf("%s    index %s\n", indent, strings.Join(rc.Index, " ")))
			if rc.Browse {
				b.WriteString(fmt.Sprintf("%s    browse\n", indent))
			}
			b.WriteString(fmt.Sprintf("%s}\n", indent))
		} else if rc.Browse {
			b.WriteString(fmt.Sprintf("%sfile_server browse\n", indent))
		} else {
			b.WriteString(fmt.Sprintf("%sfile_server\n", indent))
		}
	case "respond":
		b.WriteString(fmt.Sprintf("%srespond %s%s %d\n", indent, matcherPrefix, quoteIfNeeded(rc.Respond.Body), rc.Respond.Status))
	case "redirect":
		b.WriteString(fmt.Sprintf("%sredir %s%s %d\n", indent, matcherPrefix, quoteIfNeeded(rc.Redirect.To), rc.Redirect.Code))
	}
}
