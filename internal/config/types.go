package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gymbag/internal/host"
	"gymbag/internal/intel"
	"gymbag/internal/localpki"
)

// VerifyLevel defines when Gymbag triggers proof-of-work verification (0 to 3):
//   - 0: always
//   - 1: only suspicious
//   - 2: only ai
//   - 3: suspicious & ai
type VerifyLevel int

const (
	VerifyLevelAlways          VerifyLevel = 0
	VerifyLevelOnlySuspicious  VerifyLevel = 1
	VerifyLevelOnlyAI          VerifyLevel = 2
	VerifyLevelSuspiciousAndAI VerifyLevel = 3
)

// Valid reports whether v is a supported verification level in [0, 3].
func (v VerifyLevel) Valid() bool {
	return v >= VerifyLevelAlways && v <= VerifyLevelSuspiciousAndAI
}

// Label returns the human-readable name for the verification level.
func (v VerifyLevel) Label() string {
	switch v {
	case VerifyLevelAlways:
		return "always"
	case VerifyLevelOnlySuspicious:
		return "only suspicious"
	case VerifyLevelOnlyAI:
		return "only ai"
	case VerifyLevelSuspiciousAndAI:
		return "suspicious & ai"
	default:
		return fmt.Sprintf("invalid(%d)", int(v))
	}
}

// UnmarshalJSON strictly parses an integer verify level (0..3) from JSON.
func (v *VerifyLevel) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.ContainsAny(trimmed, ".eE\"") {
		return fmt.Errorf("verify_level must be an integer from 0 to 3 (0=always, 1=only suspicious, 2=only ai, 3=suspicious & ai)")
	}
	var raw int
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return fmt.Errorf("verify_level must be an integer from 0 to 3 (0=always, 1=only suspicious, 2=only ai, 3=suspicious & ai)")
	}
	lvl := VerifyLevel(raw)
	if !lvl.Valid() {
		return fmt.Errorf("verify_level must be from 0 to 3: 0 (always), 1 (only suspicious), 2 (only ai), 3 (suspicious & ai)")
	}
	*v = lvl
	return nil
}

// ParseVerifyLevel parses a verification level (0-3) from config text, env vars, or CLI flags.
// Returns (level, protectEnabled, error). Passing "off"/"none"/"disabled" returns protectEnabled=false.
func ParseVerifyLevel(raw string) (VerifyLevel, bool, error) {
	trimmed := strings.TrimSpace(raw)
	if n, err := strconv.Atoi(trimmed); err == nil {
		lvl := VerifyLevel(n)
		if !lvl.Valid() {
			return 0, false, fmt.Errorf(
				"verify level must be from 0 to 3: 0 (always), 1 (only suspicious), 2 (only ai), 3 (suspicious & ai)",
			)
		}
		return lvl, true, nil
	}

	cleaned := strings.ToLower(trimmed)
	cleaned = strings.ReplaceAll(cleaned, "-", " ")
	cleaned = strings.ReplaceAll(cleaned, "_", " ")
	cleaned = strings.Join(strings.Fields(cleaned), " ")

	switch cleaned {
	case "always":
		return VerifyLevelAlways, true, nil
	case "only suspicious", "only suspicion", "suspicious", "suspicion":
		return VerifyLevelOnlySuspicious, true, nil
	case "only ai", "ai":
		return VerifyLevelOnlyAI, true, nil
	case "suspicious & ai", "suspicious&ai", "suspicious and ai", "suspicion & ai", "suspicion&ai", "suspicion and ai",
		"ai & suspicious", "ai & suspicion":
		return VerifyLevelSuspiciousAndAI, true, nil
	case "off", "none", "disabled", "disable":
		return VerifyLevelAlways, false, nil
	default:
		return 0, false, fmt.Errorf(
			"unsupported verify level %q (expected 0=always, 1=only suspicious, 2=only ai, or 3=suspicious & ai)",
			raw,
		)
	}
}

// RuleAction defines what a user-defined CustomRule does when matched.
type RuleAction string

const (
	RuleActionBlock  RuleAction = "block"
	RuleActionVerify RuleAction = "verify"
	RuleActionAllow  RuleAction = "allow"
)

// ParseRuleAction normalizes a rule action keyword ("block"/"deny", "verify"/"challenge", "allow"/"bypass").
func ParseRuleAction(raw string) (RuleAction, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "block", "deny", "drop", "reject":
		return RuleActionBlock, nil
	case "verify", "challenge", "pow":
		return RuleActionVerify, nil
	case "allow", "pass", "bypass", "whitelist":
		return RuleActionAllow, nil
	default:
		return "", fmt.Errorf("unsupported rule action %q (expected block, verify, or allow)", raw)
	}
}

// KeyValueMatch matches a named HTTP header or cookie against a pattern:
//   - Pattern == ""  -> matches when the header/cookie is absent or empty
//   - Pattern == "*" -> matches when the header/cookie is present and non-empty
//   - Pattern starting with "~" -> regular expression match
//   - Otherwise -> case-insensitive wildcard (*) or substring/exact match
type KeyValueMatch struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
}

// CustomRule defines a user-authored firewall rule matching IP/CIDR, User-Agent, Path,
// Query, Method, Header, and/or Cookie. Multiple values within a field match via OR;
// multiple populated fields within a single CustomRule match via AND.
type CustomRule struct {
	Name       string          `json:"name,omitempty"`
	Action     RuleAction      `json:"action"`
	IPs        []string        `json:"ips,omitempty"`
	UserAgents []string        `json:"user_agents,omitempty"`
	Paths      []string        `json:"paths,omitempty"`
	Queries    []string        `json:"queries,omitempty"`
	Methods    []string        `json:"methods,omitempty"`
	Headers    []KeyValueMatch `json:"headers,omitempty"`
	Cookies    []KeyValueMatch `json:"cookies,omitempty"`
}

// Clone returns a deep copy of CustomRule.
func (r CustomRule) Clone() CustomRule {
	out := r
	out.IPs = slices.Clone(r.IPs)
	out.UserAgents = slices.Clone(r.UserAgents)
	out.Paths = slices.Clone(r.Paths)
	out.Queries = slices.Clone(r.Queries)
	out.Methods = slices.Clone(r.Methods)
	out.Headers = slices.Clone(r.Headers)
	out.Cookies = slices.Clone(r.Cookies)
	return out
}

// Validate normalizes and checks all matchers in CustomRule.
func (r *CustomRule) Validate() error {
	act, err := ParseRuleAction(string(r.Action))
	if err != nil {
		return err
	}
	r.Action = act

	hasCondition := len(r.IPs) > 0 || len(r.UserAgents) > 0 || len(r.Paths) > 0 ||
		len(r.Queries) > 0 || len(r.Methods) > 0 || len(r.Headers) > 0 || len(r.Cookies) > 0
	if !hasCondition {
		return fmt.Errorf("custom rule %q requires at least one matcher (ip, user_agent, path, query, method, header, or cookie)", r.Name)
	}

	normIPs := make([]string, 0, len(r.IPs))
	for _, rawIP := range r.IPs {
		pref, pErr := intel.ParseIPOrCIDR(rawIP)
		if pErr != nil {
			return fmt.Errorf("invalid IP or CIDR %q in rule: %w", rawIP, pErr)
		}
		if (pref.Addr().Is4() && pref.Bits() == 32) || (pref.Addr().Is6() && pref.Bits() == 128) {
			normIPs = append(normIPs, pref.Addr().String())
		} else {
			normIPs = append(normIPs, pref.String())
		}
	}
	r.IPs = normIPs

	if err := validatePatterns("user_agent", r.UserAgents); err != nil {
		return err
	}
	if err := validatePatterns("path", r.Paths); err != nil {
		return err
	}
	if err := validatePatterns("query", r.Queries); err != nil {
		return err
	}

	normMethods := make([]string, 0, len(r.Methods))
	for _, m := range r.Methods {
		up := strings.ToUpper(strings.TrimSpace(m))
		if up == "" {
			return fmt.Errorf("rule method must not be empty")
		}
		normMethods = append(normMethods, up)
	}
	r.Methods = normMethods

	for i := range r.Headers {
		r.Headers[i].Name = strings.TrimSpace(r.Headers[i].Name)
		if r.Headers[i].Name == "" {
			return fmt.Errorf("rule header name must not be empty")
		}
		if err := validatePattern("header", r.Headers[i].Pattern); err != nil {
			return err
		}
	}
	for i := range r.Cookies {
		r.Cookies[i].Name = strings.TrimSpace(r.Cookies[i].Name)
		if r.Cookies[i].Name == "" {
			return fmt.Errorf("rule cookie name must not be empty")
		}
		if err := validatePattern("cookie", r.Cookies[i].Pattern); err != nil {
			return err
		}
	}
	return nil
}

func validatePatterns(field string, patterns []string) error {
	for _, p := range patterns {
		if strings.TrimSpace(p) == "" {
			return fmt.Errorf("rule %s pattern must not be empty", field)
		}
		if err := validatePattern(field, p); err != nil {
			return err
		}
	}
	return nil
}

func validatePattern(field, p string) error {
	if strings.HasPrefix(p, "~") {
		expr := strings.TrimPrefix(p, "~")
		if expr == "" {
			return fmt.Errorf("rule %s regex must not be empty", field)
		}
		if _, err := regexp.Compile(expr); err != nil {
			return fmt.Errorf("invalid regex %q in rule %s: %w", expr, field, err)
		}
	}
	return nil
}

// IntelConfig configures automatic IP threat intelligence feeds, live IP reputation lookups,
// free VPN/proxy/Tor detection, and official AI crawler IP lists.
type IntelConfig struct {
	IPSecurity       bool     `json:"ip_security"`
	IPSecurityAction string   `json:"ip_security_action"` // "verify" or "block"
	LiveReputation   bool     `json:"live_reputation"`
	AIIPList         bool     `json:"ai_ip_list"`
	VPNDetection     bool     `json:"vpn_detection"`
	RefreshInterval  int      `json:"refresh_interval"`
	ThreatFeeds      []string `json:"threat_feeds,omitempty"`
	AIIPFeeds        []string `json:"ai_ip_feeds,omitempty"`
	VPNFeeds         []string `json:"vpn_feeds,omitempty"`
}

// DefaultIntelConfig returns default automatic IP security, VPN/proxy, and AI crawler IP feed settings.
func DefaultIntelConfig() IntelConfig {
	return IntelConfig{
		IPSecurity:       true,
		IPSecurityAction: "verify",
		LiveReputation:   true,
		AIIPList:         true,
		VPNDetection:     true,
		RefreshInterval:  6 * 60 * 60, // 6 hours
	}
}

// Validate normalizes and validates IntelConfig.
func (ic *IntelConfig) Validate() error {
	act := strings.ToLower(strings.TrimSpace(ic.IPSecurityAction))
	switch act {
	case "", "verify", "challenge":
		ic.IPSecurityAction = "verify"
	case "block", "deny":
		ic.IPSecurityAction = "block"
	default:
		return fmt.Errorf("ip_security_action must be verify or block")
	}
	if ic.RefreshInterval <= 0 {
		ic.RefreshInterval = 6 * 60 * 60
	}
	for _, u := range ic.ThreatFeeds {
		if err := RequireHTTPURL(u, "threat_feed"); err != nil {
			return err
		}
	}
	for _, u := range ic.AIIPFeeds {
		if err := RequireHTTPURL(u, "ai_ip_feed"); err != nil {
			return err
		}
	}
	for _, u := range ic.VPNFeeds {
		if err := RequireHTTPURL(u, "vpn_feed"); err != nil {
			return err
		}
	}
	return nil
}

// ValidTLSModes lists supported TLS certificate provisioning modes.
var ValidTLSModes = map[string]struct{}{
	"auto":       {},
	"acme":       {},
	"local":      {},
	"selfsigned": {},
	"file":       {},
	"off":        {},
}

// NormalizeSSLProvider normalizes an SSL/ACME CA provider name or custom ACME directory URL.
// It returns (providerName, tlsMode, customDirectoryURL, error).
func NormalizeSSLProvider(raw string) (string, string, string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", "", fmt.Errorf("ssl_provider must not be empty")
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") {
		if err := RequireHTTPURL(trimmed, "ssl_provider"); err != nil {
			return "", "", "", err
		}
		return "custom", "acme", trimmed, nil
	}
	switch lower {
	case "letsencrypt", "le", "lets_encrypt", "lets-encrypt":
		return "letsencrypt", "acme", "", nil
	case "letsencrypt_staging", "le_staging", "letsencrypt-staging", "le-staging":
		return "letsencrypt_staging", "acme", "", nil
	case "zerossl":
		return "zerossl", "acme", "", nil
	case "google", "gts", "google_trust_services", "google-trust-services":
		return "google", "acme", "", nil
	case "ssl.com", "sslcom", "ssl_com":
		return "ssl.com", "acme", "", nil
	case "ssl.com-rsa", "sslcom-rsa", "ssl_com_rsa":
		return "ssl.com-rsa", "acme", "", nil
	case "litessl", "trustasia", "lite_ssl", "lite-ssl":
		return "litessl", "acme", "", nil
	case "buypass":
		return "buypass", "acme", "", nil
	case "local", "internal":
		return "local", "local", "", nil
	default:
		return "", "", "", fmt.Errorf(
			"unsupported ssl_provider %q (expected letsencrypt, letsencrypt_staging, zerossl, google, ssl.com, litessl, buypass, local, or an https:// ACME directory URL)",
			raw,
		)
	}
}

func applySSLProvider(raw string, tlsCfg *TLSConfig) error {
	wasCustom := strings.EqualFold(strings.TrimSpace(tlsCfg.Provider), "custom")
	prov, mode, customURL, err := NormalizeSSLProvider(raw)
	if err != nil {
		return err
	}
	tlsCfg.Provider = prov
	tlsCfg.Mode = mode
	if customURL != "" {
		tlsCfg.ACMEDirectory = customURL
	} else if wasCustom {
		tlsCfg.ACMEDirectory = DefaultTLSConfig().ACMEDirectory
	}
	return nil
}

// DefaultConfigFiles lists automatic config filenames probed in the working directory.
var DefaultConfigFiles = []string{"gymbag.json", "Gymbagfile", "Caddyfile"}

// RuleSource references an external firewall rule file, directory, or URL.
// When Dynamic is true, Gymbag watches local files/folders for changes or refreshes URLs periodically.
type RuleSource struct {
	Source  string `json:"source"`
	Dynamic bool   `json:"dynamic,omitempty"`
	Name    string `json:"name,omitempty"`
}

// IsURL reports whether the rule source is an http:// or https:// URL.
func (rs RuleSource) IsURL() bool {
	lower := strings.ToLower(strings.TrimSpace(rs.Source))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// Validate normalizes and checks a RuleSource.
func (rs *RuleSource) Validate() error {
	rs.Source = strings.TrimSpace(rs.Source)
	rs.Name = strings.TrimSpace(rs.Name)
	if rs.Source == "" {
		return fmt.Errorf("rule source must not be empty")
	}
	if strings.ContainsAny(rs.Source, "\r\n\x00") || strings.ContainsAny(rs.Name, "\r\n\x00") {
		return fmt.Errorf("rule source contains invalid control characters")
	}
	if rs.IsURL() {
		return RequireHTTPURL(rs.Source, "rule_source")
	}
	return nil
}

// SiteWAFConfig holds per-website WAF, verification level, and custom rules.
type SiteWAFConfig struct {
	VerifyLevel           VerifyLevel  `json:"verify_level"`
	Difficulty            int          `json:"difficulty"`
	ChallengeTTL          int          `json:"challenge_ttl"`
	ClearanceTTL          int          `json:"clearance_ttl"`
	CookieName            string       `json:"cookie_name"`
	CookieSecure          bool         `json:"cookie_secure"`
	RateLimit             int          `json:"rate_limit"`
	RateWindow            int          `json:"rate_window"`
	DefaultRules          *bool        `json:"default_rules,omitempty"`
	Rules                 []CustomRule `json:"rules,omitempty"`
	RemoteRules           []string     `json:"remote_rules,omitempty"`
	RuleSources           []RuleSource `json:"rule_sources,omitempty"`
	CustomAIBots          []string     `json:"custom_ai_bots,omitempty"`
	CustomSuspiciousUAs   []string     `json:"custom_suspicious_uas,omitempty"`
	CustomSuspiciousPaths []string     `json:"custom_suspicious_paths,omitempty"`
}

// DefaultRulesEnabled reports whether built-in open-box security rules are active (true when nil or true).
func (w SiteWAFConfig) DefaultRulesEnabled() bool {
	return w.DefaultRules == nil || *w.DefaultRules
}

// DefaultSiteWAFConfig returns the default WAF settings inherited by protected sites.
func DefaultSiteWAFConfig() SiteWAFConfig {
	return SiteWAFConfig{
		VerifyLevel:  VerifyLevelAlways,
		Difficulty:   4,
		ChallengeTTL: 300,
		ClearanceTTL: 86400,
		CookieName:   "waf_clearance",
		CookieSecure: true,
		RateLimit:    240,
		RateWindow:   60,
	}
}

// Validate checks SiteWAFConfig invariants and validates all custom rules.
func (w *SiteWAFConfig) Validate() error {
	if !w.VerifyLevel.Valid() {
		return fmt.Errorf("verify_level must be from 0 to 3: 0 (always), 1 (only suspicious), 2 (only ai), 3 (suspicious & ai)")
	}
	if w.Difficulty < 1 || w.Difficulty > 5 {
		return fmt.Errorf("difficulty must be from 1 to 5")
	}
	if w.ChallengeTTL <= 0 {
		return fmt.Errorf("challenge_ttl must be greater than zero")
	}
	if w.ClearanceTTL <= 0 {
		return fmt.Errorf("clearance_ttl must be greater than zero")
	}
	if strings.TrimSpace(w.CookieName) == "" {
		return fmt.Errorf("cookie_name must not be empty")
	}
	if w.RateLimit <= 0 {
		return fmt.Errorf("rate_limit must be greater than zero")
	}
	if w.RateWindow <= 0 {
		return fmt.Errorf("rate_window must be greater than zero")
	}
	for i := range w.Rules {
		if err := w.Rules[i].Validate(); err != nil {
			return err
		}
	}
	if len(w.RemoteRules) > 0 {
		seen := make(map[string]struct{}, len(w.RemoteRules))
		cleaned := make([]string, 0, len(w.RemoteRules))
		for _, rawURL := range w.RemoteRules {
			u := strings.TrimSpace(rawURL)
			if u == "" {
				continue
			}
			if err := RequireHTTPURL(u, "remote_rules"); err != nil {
				return err
			}
			if _, ok := seen[u]; !ok {
				seen[u] = struct{}{}
				cleaned = append(cleaned, u)
			}
		}
		w.RemoteRules = cleaned
	}
	if len(w.RuleSources) > 0 {
		idxBySrc := make(map[string]int, len(w.RuleSources))
		cleaned := make([]RuleSource, 0, len(w.RuleSources))
		for i := range w.RuleSources {
			rs := w.RuleSources[i]
			if err := rs.Validate(); err != nil {
				return err
			}
			if pos, ok := idxBySrc[rs.Source]; ok {
				if rs.Dynamic {
					cleaned[pos].Dynamic = true
				}
				if cleaned[pos].Name == "" && rs.Name != "" {
					cleaned[pos].Name = rs.Name
				}
				continue
			}
			idxBySrc[rs.Source] = len(cleaned)
			cleaned = append(cleaned, rs)
		}
		w.RuleSources = cleaned
	}
	return nil
}

// MascotConfig customizes mascot images shown on WAF challenge, block, rate-limit, and verification pages.
// Each field accepts either a local file/directory path or an http(s):// image URL.
type MascotConfig struct {
	Default string `json:"default,omitempty"`
	Pensive string `json:"pensive,omitempty"`
	Happy   string `json:"happy,omitempty"`
	Reject  string `json:"reject,omitempty"`
}

// IsEmpty reports whether m is nil or has no configured mascot paths or URLs.
func (m *MascotConfig) IsEmpty() bool {
	return m == nil || (strings.TrimSpace(m.Default) == "" &&
		strings.TrimSpace(m.Pensive) == "" &&
		strings.TrimSpace(m.Happy) == "" &&
		strings.TrimSpace(m.Reject) == "")
}

// Clone returns a deep copy of MascotConfig, or nil when m is nil.
func (m *MascotConfig) Clone() *MascotConfig {
	if m == nil {
		return nil
	}
	cp := *m
	return &cp
}

// UnmarshalJSON decodes a MascotConfig from either a single path/URL string or a structured object.
func (m *MascotConfig) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	if trimmed[0] == '"' {
		var single string
		if err := json.Unmarshal(trimmed, &single); err != nil {
			return err
		}
		m.Default = strings.TrimSpace(single)
		return nil
	}
	type mascotAlias struct {
		Default  string `json:"default,omitempty"`
		Image    string `json:"image,omitempty"`
		URL      string `json:"url,omitempty"`
		Pensive  string `json:"pensive,omitempty"`
		Checking string `json:"checking,omitempty"`
		Happy    string `json:"happy,omitempty"`
		Pass     string `json:"pass,omitempty"`
		Reject   string `json:"reject,omitempty"`
		Fail     string `json:"fail,omitempty"`
	}
	var a mascotAlias
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return err
	}
	pick := func(vals ...string) string {
		for _, v := range vals {
			if t := strings.TrimSpace(v); t != "" {
				return t
			}
		}
		return ""
	}
	m.Default = pick(a.Default, a.Image, a.URL)
	m.Pensive = pick(a.Pensive, a.Checking)
	m.Happy = pick(a.Happy, a.Pass)
	m.Reject = pick(a.Reject, a.Fail)
	return nil
}

// MarshalJSON encodes a single-default MascotConfig concisely as a string, or as an object when per-state overrides exist.
func (m MascotConfig) MarshalJSON() ([]byte, error) {
	if m.Default != "" && m.Pensive == "" && m.Happy == "" && m.Reject == "" {
		return json.Marshal(m.Default)
	}
	type rawMascot struct {
		Default string `json:"default,omitempty"`
		Pensive string `json:"pensive,omitempty"`
		Happy   string `json:"happy,omitempty"`
		Reject  string `json:"reject,omitempty"`
	}
	return json.Marshal(rawMascot{
		Default: m.Default,
		Pensive: m.Pensive,
		Happy:   m.Happy,
		Reject:  m.Reject,
	})
}

// Validate trims and validates mascot image paths and remote URLs, rejecting unsafe schemes.
func (m *MascotConfig) Validate() error {
	if m == nil {
		return nil
	}
	fields := []struct {
		name string
		ptr  *string
	}{
		{"mascot.default", &m.Default},
		{"mascot.pensive", &m.Pensive},
		{"mascot.happy", &m.Happy},
		{"mascot.reject", &m.Reject},
	}
	for _, f := range fields {
		v := strings.TrimSpace(*f.ptr)
		*f.ptr = v
		if v == "" {
			continue
		}
		lower := strings.ToLower(v)
		for _, bad := range []string{"javascript:", "data:", "vbscript:", "file:"} {
			if strings.HasPrefix(lower, bad) {
				return fmt.Errorf("%s uses unsupported scheme %q (use a local path or http:// / https:// URL)", f.name, strings.TrimSuffix(bad, ":"))
			}
		}
		if strings.Contains(v, "://") {
			if err := RequireHTTPURL(v, f.name); err != nil {
				return err
			}
		}
	}
	return nil
}

// SiteConfig describes a single downstream website hosted and optionally protected by Gymbag.
type SiteConfig struct {
	Domain     string             `json:"domain"`
	Upstreams  []string           `json:"upstreams,omitempty"`
	Routes     []RouteConfig      `json:"routes,omitempty"`  // path-matched handlers evaluated before Upstreams
	Headers    map[string]string  `json:"headers,omitempty"` // response header mutations applied to every response ("-Name" deletes)
	Auth       *ForwardAuthConfig `json:"auth,omitempty"`    // optional forward authentication (Authelia / Caddy forward_auth)
	Protect    bool               `json:"protect"`
	WAF        SiteWAFConfig      `json:"waf"`
	CertFile   string             `json:"cert_file,omitempty"`
	KeyFile    string             `json:"key_file,omitempty"`
	AcmeShPath string             `json:"acme_sh_path,omitempty"`
	Mascot     *MascotConfig      `json:"mascot,omitempty"`
}

// UnmarshalJSON initializes sensible defaults when decoding a new SiteConfig from concise JSON.
func (s *SiteConfig) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	if !s.Protect && s.Domain == "" && len(s.Upstreams) == 0 && len(s.Routes) == 0 && s.WAF.Difficulty == 0 && s.WAF.ClearanceTTL == 0 {
		s.Protect = true
		s.WAF = DefaultSiteWAFConfig()
	}
	type siteAlias SiteConfig
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode((*siteAlias)(s))
}

// Clone returns a deep copy of the site configuration.
func (s SiteConfig) Clone() SiteConfig {
	out := s
	out.Upstreams = append([]string(nil), s.Upstreams...)
	if s.Routes != nil {
		out.Routes = make([]RouteConfig, len(s.Routes))
		for i, r := range s.Routes {
			out.Routes[i] = r.Clone()
		}
	}
	out.Headers = cloneHeaderMap(s.Headers)
	out.Auth = s.Auth.Clone()
	out.WAF = cloneSiteWAF(s.WAF)
	out.Mascot = s.Mascot.Clone()
	return out
}

// PrimaryUpstream returns the site's first upstream URL.
func (s SiteConfig) PrimaryUpstream() string {
	if len(s.Upstreams) == 0 {
		return ""
	}
	return s.Upstreams[0]
}

// Validate normalizes and checks a SiteConfig.
func (s *SiteConfig) Validate() error {
	s.Domain = host.Normalize(s.Domain)
	if s.Domain == "" {
		return fmt.Errorf("site domain must not be empty")
	}
	if len(s.Upstreams) == 0 && len(s.Routes) == 0 {
		return fmt.Errorf("site %s requires at least one upstream URL or route", s.Domain)
	}
	normalized := make([]string, 0, len(s.Upstreams))
	for _, raw := range s.Upstreams {
		u, err := NormalizeUpstream(raw, "upstream")
		if err != nil {
			return err
		}
		normalized = append(normalized, u)
	}
	s.Upstreams = normalized
	for i := range s.Routes {
		if err := s.Routes[i].Validate(); err != nil {
			return fmt.Errorf("site %s: %w", s.Domain, err)
		}
	}
	if err := validateHeaderMap(s.Headers); err != nil {
		return fmt.Errorf("site %s: %w", s.Domain, err)
	}
	if s.Auth != nil {
		if err := s.Auth.Validate(); err != nil {
			return fmt.Errorf("site %s: %w", s.Domain, err)
		}
	}
	if s.WAF.Difficulty == 0 {
		s.WAF.Difficulty = 4
	}
	if s.WAF.ChallengeTTL == 0 {
		s.WAF.ChallengeTTL = 300
	}
	if s.WAF.ClearanceTTL == 0 {
		s.WAF.ClearanceTTL = 86400
	}
	if s.WAF.CookieName == "" {
		s.WAF.CookieName = "waf_clearance"
	}
	if s.WAF.RateLimit == 0 {
		s.WAF.RateLimit = 240
	}
	if s.WAF.RateWindow == 0 {
		s.WAF.RateWindow = 60
	}
	s.AcmeShPath = strings.TrimSpace(s.AcmeShPath)
	if (s.CertFile == "") != (s.KeyFile == "") {
		return fmt.Errorf("site %s must specify both cert_file and key_file or neither", s.Domain)
	}
	if s.Mascot != nil {
		if err := s.Mascot.Validate(); err != nil {
			return fmt.Errorf("site %s: %w", s.Domain, err)
		}
		if s.Mascot.IsEmpty() {
			s.Mascot = nil
		}
	}
	return s.WAF.Validate()
}

// TLSConfig holds server-wide TLS & ACME certificate configuration.
type TLSConfig struct {
	Mode            string `json:"mode"`
	AutoSSL         *bool  `json:"auto_ssl,omitempty"`
	Provider        string `json:"provider,omitempty"`
	CertFile        string `json:"cert_file"`
	KeyFile         string `json:"key_file"`
	AcmeShPath      string `json:"acme_sh_path,omitempty"`
	KeyPassword     string `json:"key_password,omitempty"`
	AccountKeyFile  string `json:"account_key_file,omitempty"`
	EABKID          string `json:"eab_kid,omitempty"`
	EABHMACKey      string `json:"eab_hmac_key,omitempty"`
	AutoHosts       *bool  `json:"auto_hosts,omitempty"`
	CertDir         string `json:"cert_dir"`
	ACMEEmail       string `json:"acme_email,omitempty"`
	ACMEDirectory   string `json:"acme_directory"`
	RenewInterval   int    `json:"renew_interval"`
	RenewWithinDays int    `json:"renew_within_days"`
	HSTS            *bool  `json:"hsts,omitempty"`
}

// AutoSSLEnabled reports whether automatic certificate provisioning (ACME, local CA, self-signed) is enabled.
func (t TLSConfig) AutoSSLEnabled() bool {
	return t.AutoSSL == nil || *t.AutoSSL
}

// AutoHostsEnabled reports whether automatic /etc/hosts setup & cleanup for local domains is active (true when nil or true).
func (t TLSConfig) AutoHostsEnabled() bool {
	return t.AutoHosts == nil || *t.AutoHosts
}

// DefaultTLSConfig returns default TLS settings.
func DefaultTLSConfig() TLSConfig {
	return TLSConfig{
		Mode:            "auto",
		CertFile:        "cert.pem",
		KeyFile:         "key.pem",
		CertDir:         "certs",
		ACMEDirectory:   "https://acme-v02.api.letsencrypt.org/directory",
		RenewInterval:   12 * 60 * 60,
		RenewWithinDays: 30,
	}
}

// DefaultRemoteRulesRefresh is the default refresh interval (in seconds) for remote rule URLs.
const DefaultRemoteRulesRefresh = 300

// DefaultRulesWatchInterval is the default polling interval (in seconds) for dynamic local rule files/folders.
const DefaultRulesWatchInterval = 2

// Config is the root configuration model for Gymbag.
type Config struct {
	Sites              []SiteConfig       `json:"sites,omitempty"`
	SSH                *SSHConfig         `json:"ssh,omitempty"`
	Minecraft          *MinecraftConfig   `json:"minecraft,omitempty"`
	Verify             VerifyServerConfig `json:"verify,omitempty"`
	Authelia           *ForwardAuthConfig `json:"authelia,omitempty"`
	BindHost           string             `json:"bind_host"`
	HTTPPort           int                `json:"http_port"`
	HTTPSPort          int                `json:"https_port"`
	DirectHTTP         bool               `json:"direct_http,omitempty"`
	AdminAddr          string             `json:"admin_addr,omitempty"`
	TLS                TLSConfig          `json:"tls"`
	Intel              IntelConfig        `json:"intel"`
	RemoteRulesRefresh int                `json:"remote_rules_refresh,omitempty"`
	RulesWatchInterval int                `json:"rules_watch_interval,omitempty"`
	RedisURL           string             `json:"redis_url,omitempty"`
	Secret             string             `json:"secret,omitempty"`
	TrustedProxies     []string           `json:"trusted_proxies,omitempty"`
	Mascot             *MascotConfig      `json:"mascot,omitempty"`
	Defaults           SiteWAFConfig      `json:"defaults"`
	BaseDir            string             `json:"-"`
}

// HTTPEnabled reports whether at least one HTTP/HTTPS reverse proxy site is configured.
func (c Config) HTTPEnabled() bool {
	return len(c.Sites) > 0
}

// HasExternalTLSCerts reports whether user-managed certificates (file paths, cert/key, or acme.sh path) are configured.
func (c Config) HasExternalTLSCerts() bool {
	if strings.TrimSpace(c.TLS.AcmeShPath) != "" {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(c.TLS.Mode), "file") {
		return true
	}
	if cert := strings.TrimSpace(c.TLS.CertFile); cert != "" && cert != "cert.pem" {
		return true
	}
	if key := strings.TrimSpace(c.TLS.KeyFile); key != "" && key != "key.pem" {
		return true
	}
	for _, s := range c.Sites {
		if (strings.TrimSpace(s.CertFile) != "" && strings.TrimSpace(s.KeyFile) != "") ||
			strings.TrimSpace(s.AcmeShPath) != "" {
			return true
		}
	}
	return false
}

// TLSDisabled reports whether HTTPS is disabled for HTTP/HTTP-proxy sites.
func (c Config) TLSDisabled() bool {
	if strings.EqualFold(strings.TrimSpace(c.TLS.Mode), "off") {
		return true
	}
	return !c.TLS.AutoSSLEnabled() && !c.HasExternalTLSCerts()
}

// HTTPSEnabled reports whether the HTTPS gateway listener should run for configured sites.
func (c Config) HTTPSEnabled() bool {
	return c.HTTPEnabled() && !c.TLSDisabled()
}

// DirectHTTPEnabled reports whether sites are served directly on HTTP without redirecting to HTTPS.
func (c Config) DirectHTTPEnabled() bool {
	return c.DirectHTTP || c.TLSDisabled()
}

// SSHEnabled reports whether SSH protection is enabled.
func (c Config) SSHEnabled() bool {
	return c.SSH != nil && c.SSH.Enabled
}

// MinecraftEnabled reports whether Minecraft Java server protection is enabled.
func (c Config) MinecraftEnabled() bool {
	return c.Minecraft != nil && c.Minecraft.Enabled
}

// NeedsVerifyServer reports whether the shared HTTPS verification portal should be served.
func (c Config) NeedsVerifyServer() bool {
	return (c.SSHEnabled() && c.SSH.Protect) ||
		(c.MinecraftEnabled() && c.Minecraft.Protect) ||
		strings.TrimSpace(c.Verify.URL) != "" ||
		c.Verify.Port > 0
}

// Default returns a baseline Config for localhost -> http://localhost:5173.
func Default() Config {
	waf := DefaultSiteWAFConfig()
	return Config{
		Sites: []SiteConfig{
			{
				Domain:    "localhost",
				Upstreams: []string{"http://localhost:5173"},
				Protect:   true,
				WAF:       waf,
			},
		},
		BindHost:           "127.0.0.1",
		HTTPPort:           80,
		HTTPSPort:          443,
		TLS:                DefaultTLSConfig(),
		Intel:              DefaultIntelConfig(),
		RemoteRulesRefresh: DefaultRemoteRulesRefresh,
		RulesWatchInterval: DefaultRulesWatchInterval,
		Defaults:           waf,
	}
}

// AllRuleSources returns the ordered deduplicated list of external rule sources across global defaults and all sites.
func (c Config) AllRuleSources() []RuleSource {
	idxBySrc := make(map[string]int)
	var out []RuleSource
	add := func(rs RuleSource) {
		rs.Source = strings.TrimSpace(rs.Source)
		rs.Name = strings.TrimSpace(rs.Name)
		if rs.Source == "" {
			return
		}
		if pos, ok := idxBySrc[rs.Source]; ok {
			if rs.Dynamic {
				out[pos].Dynamic = true
			}
			if out[pos].Name == "" && rs.Name != "" {
				out[pos].Name = rs.Name
			}
			return
		}
		idxBySrc[rs.Source] = len(out)
		out = append(out, rs)
	}
	for _, rs := range c.Defaults.RuleSources {
		add(rs)
	}
	for _, u := range c.Defaults.RemoteRules {
		add(RuleSource{Source: u, Dynamic: true})
	}
	for _, s := range c.Sites {
		for _, rs := range s.WAF.RuleSources {
			add(rs)
		}
		for _, u := range s.WAF.RemoteRules {
			add(RuleSource{Source: u, Dynamic: true})
		}
	}
	return out
}

// RemoteRuleURLs returns the ordered unique list of remote rule URLs referenced across all sites.
func (c Config) RemoteRuleURLs() []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(u string) {
		u = strings.TrimSpace(u)
		if u == "" {
			return
		}
		if _, ok := seen[u]; !ok {
			seen[u] = struct{}{}
			out = append(out, u)
		}
	}
	for _, u := range c.Defaults.RemoteRules {
		add(u)
	}
	for _, s := range c.Sites {
		for _, u := range s.WAF.RemoteRules {
			add(u)
		}
	}
	return out
}

// Domains returns the ordered unique list of site domains.
func (c Config) Domains() []string {
	seen := make(map[string]struct{}, len(c.Sites))
	out := make([]string, 0, len(c.Sites))
	for _, s := range c.Sites {
		d := host.Normalize(s.Domain)
		if _, ok := seen[d]; !ok && d != "" {
			seen[d] = struct{}{}
			out = append(out, d)
		}
	}
	return out
}

// Redacted returns a shallow copy with secrets masked for safe API output.
func (c Config) Redacted() Config {
	clone := c
	if clone.Secret != "" {
		clone.Secret = "***REDACTED***"
	}
	if clone.TLS.KeyPassword != "" {
		clone.TLS.KeyPassword = "***REDACTED***"
	}
	if clone.TLS.EABHMACKey != "" {
		clone.TLS.EABHMACKey = "***REDACTED***"
	}
	return clone
}

// Validate normalizes and validates the complete Config.
func (c *Config) Validate() error {
	if err := c.Verify.Validate(); err != nil {
		return err
	}
	if c.Authelia != nil {
		if err := c.Authelia.Validate(); err != nil {
			return err
		}
	}
	if c.SSH != nil {
		if err := c.SSH.Validate(); err != nil {
			return err
		}
	}
	if c.Minecraft != nil {
		if err := c.Minecraft.Validate(); err != nil {
			return err
		}
	}
	if len(c.Sites) == 0 && !c.SSHEnabled() && !c.MinecraftEnabled() {
		return fmt.Errorf("at least one service (HTTP site, SSH, or Minecraft) is required")
	}
	seen := make(map[string]int, len(c.Sites))
	deduped := make([]SiteConfig, 0, len(c.Sites))
	for i := range c.Sites {
		if err := c.Sites[i].Validate(); err != nil {
			return err
		}
		d := c.Sites[i].Domain
		if prevIdx, exists := seen[d]; exists {
			deduped[prevIdx] = c.Sites[i]
		} else {
			seen[d] = len(deduped)
			deduped = append(deduped, c.Sites[i])
		}
	}
	c.Sites = deduped

	if c.HTTPPort == 0 {
		c.HTTPPort = 80
	}
	if c.HTTPPort < 1 || c.HTTPPort > 65535 {
		return fmt.Errorf("http_port must be between 1 and 65535")
	}
	if c.HTTPSPort == 0 {
		c.HTTPSPort = 443
	}
	if c.HTTPSPort < 1 || c.HTTPSPort > 65535 {
		return fmt.Errorf("https_port must be between 1 and 65535")
	}
	c.TLS.AcmeShPath = strings.TrimSpace(c.TLS.AcmeShPath)
	c.TLS.Mode = strings.ToLower(strings.TrimSpace(c.TLS.Mode))
	switch c.TLS.Mode {
	case "none", "disabled", "disable", "false":
		c.TLS.Mode = "off"
	}
	if c.TLS.Mode == "off" {
		c.TLS.Provider = ""
	} else if rawProv := strings.TrimSpace(c.TLS.Provider); rawProv != "" {
		if strings.EqualFold(rawProv, "custom") {
			c.TLS.Provider = "custom"
			if c.TLS.Mode == "" || c.TLS.Mode == "auto" {
				c.TLS.Mode = "acme"
			}
		} else {
			prov, mode, customURL, err := NormalizeSSLProvider(rawProv)
			if err != nil {
				return err
			}
			c.TLS.Provider = prov
			if c.TLS.Mode == "" || c.TLS.Mode == "auto" {
				c.TLS.Mode = mode
			}
			if customURL != "" {
				c.TLS.ACMEDirectory = customURL
			}
		}
	}
	if c.TLS.Mode == "" {
		c.TLS.Mode = "auto"
	}
	if _, ok := ValidTLSModes[c.TLS.Mode]; !ok {
		return fmt.Errorf("tls mode must be auto, acme, local, selfsigned, file, or off")
	}
	if !c.TLS.AutoSSLEnabled() {
		if c.TLS.Mode == "acme" || c.TLS.Mode == "local" || c.TLS.Mode == "selfsigned" {
			return fmt.Errorf("tls.auto_ssl=false cannot be combined with tls.mode=%q (use \"off\" or \"file\")", c.TLS.Mode)
		}
	}
	if err := checkPortConflict(c); err != nil {
		return err
	}
	if c.TLS.CertFile == "" {
		c.TLS.CertFile = "cert.pem"
	}
	if c.TLS.KeyFile == "" {
		c.TLS.KeyFile = "key.pem"
	}
	if c.TLS.CertDir == "" {
		c.TLS.CertDir = "certs"
	}
	if c.TLS.ACMEDirectory == "" {
		c.TLS.ACMEDirectory = "https://acme-v02.api.letsencrypt.org/directory"
	}
	if err := RequireHTTPURL(c.TLS.ACMEDirectory, "acme_directory"); err != nil {
		return err
	}
	if c.TLS.RenewInterval <= 0 {
		c.TLS.RenewInterval = 12 * 60 * 60
	}
	if c.TLS.RenewWithinDays <= 0 {
		c.TLS.RenewWithinDays = 30
	}
	if err := c.Intel.Validate(); err != nil {
		return err
	}
	if c.Mascot != nil {
		if err := c.Mascot.Validate(); err != nil {
			return err
		}
		if c.Mascot.IsEmpty() {
			c.Mascot = nil
		}
	}
	if c.RemoteRulesRefresh <= 0 {
		c.RemoteRulesRefresh = DefaultRemoteRulesRefresh
	}
	if c.RulesWatchInterval <= 0 {
		c.RulesWatchInterval = DefaultRulesWatchInterval
	}
	if err := c.Defaults.Validate(); err != nil {
		c.Defaults = DefaultSiteWAFConfig()
	}
	if strings.TrimSpace(c.BindHost) == "" {
		c.BindHost = DefaultBindHostForConfig(*c)
	}
	return nil
}

// DefaultBindHost selects 0.0.0.0 when any configured domain is public (and not a local PKI domain), otherwise 127.0.0.1.
func DefaultBindHost(domains []string) string {
	for _, d := range domains {
		if !localpki.IsLocalDomain(d) && host.IsPublic(d) {
			return "0.0.0.0"
		}
	}
	return "127.0.0.1"
}

// RequireHTTPURL validates that raw is an absolute http:// or https:// URL with a host.
func RequireHTTPURL(raw, fieldName string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Hostname() == "" {
		return fmt.Errorf("%s must be an absolute http or https URL", fieldName)
	}
	return nil
}

// NormalizeUpstream expands shorthand upstream addresses (":8080", "localhost:3000")
// into validated http:// or https:// URLs.
func NormalizeUpstream(raw, fieldName string) (string, error) {
	target := strings.TrimSpace(raw)
	if target == "" {
		return "", fmt.Errorf("%s must not be empty", fieldName)
	}
	if strings.HasPrefix(target, ":") {
		target = "http://localhost" + target
	} else if !strings.Contains(target, "://") {
		target = "http://" + target
	}
	if err := RequireHTTPURL(target, fieldName); err != nil {
		return "", err
	}
	return target, nil
}

// MarshalJSONConfig serializes Config with indentation.
func MarshalJSONConfig(c Config) ([]byte, error) {
	return json.MarshalIndent(c, "", "  ")
}
