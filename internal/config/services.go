package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"gymbag/internal/host"
)

const (
	DefaultSSHPort       = 22
	DefaultMinecraftPort = 25565
	DefaultVerifyPort    = 8443
)

// VerifyServerConfig configures the shared HTTPS verification portal URL, port, and optional certificate files
// used by SSH and Minecraft verification prompts.
type VerifyServerConfig struct {
	URL      string `json:"url,omitempty"`
	Port     int    `json:"port,omitempty"`
	CertFile string `json:"cert_file,omitempty"`
	KeyFile  string `json:"key_file,omitempty"`
}

// EffectivePort returns the effective HTTPS port for the verification portal (default 8443).
func (v VerifyServerConfig) EffectivePort() int {
	if v.Port > 0 {
		return v.Port
	}
	if _, explicitPort, _, err := v.ParsedHost(); err == nil && explicitPort > 0 {
		return explicitPort
	}
	return DefaultVerifyPort
}

// ParsedHost parses VerifyServerConfig.URL into (hostname, explicitPort, pathPrefix, err).
// Returns ("", 0, "", nil) when URL is empty (defaulting to IP + IP SSL cert at runtime).
func (v VerifyServerConfig) ParsedHost() (hostname string, explicitPort int, pathPrefix string, err error) {
	raw := strings.TrimSpace(v.URL)
	if raw == "" {
		return "", 0, "", nil
	}
	if strings.Contains(raw, "://") {
		parsed, parseErr := url.Parse(raw)
		if parseErr != nil || parsed.Hostname() == "" {
			return "", 0, "", fmt.Errorf("invalid verify_url %q", v.URL)
		}
		if !strings.EqualFold(parsed.Scheme, "https") {
			return "", 0, "", fmt.Errorf("verify_url must use https:// (SSL is required): %q", v.URL)
		}
		if p := parsed.Port(); p != "" {
			portVal, portErr := host.ParsePort(p, "verify_url port")
			if portErr != nil {
				return "", 0, "", portErr
			}
			explicitPort = portVal
		}
		return host.Normalize(parsed.Hostname()), explicitPort, strings.TrimRight(parsed.Path, "/"), nil
	}

	hostPart := raw
	if slash := strings.IndexByte(raw, '/'); slash != -1 {
		hostPart = raw[:slash]
		pathPrefix = strings.TrimRight(raw[slash:], "/")
	}
	h, p, hasP, pErr := host.ParseSiteAddress(hostPart)
	if pErr != nil {
		return "", 0, "", fmt.Errorf("invalid verify_url %q: %w", v.URL, pErr)
	}
	if hasP {
		explicitPort = p
	}
	return h, explicitPort, pathPrefix, nil
}

// Validate normalizes and checks VerifyServerConfig.
func (v *VerifyServerConfig) Validate() error {
	v.URL = strings.TrimSpace(v.URL)
	v.CertFile = strings.TrimSpace(v.CertFile)
	v.KeyFile = strings.TrimSpace(v.KeyFile)
	if _, _, _, err := v.ParsedHost(); err != nil {
		return err
	}
	if v.Port < 0 || v.Port > 65535 {
		return fmt.Errorf("verify port must be between 1 and 65535")
	}
	if (v.CertFile == "") != (v.KeyFile == "") {
		return fmt.Errorf("verify cert_file and key_file must both be specified or neither")
	}
	return nil
}

// SSHConfig configures Gymbag's SSH protection proxy.
type SSHConfig struct {
	Enabled      bool         `json:"enabled"`
	Port         int          `json:"port,omitempty"`
	Upstream     string       `json:"upstream"`
	Protect      bool         `json:"protect"`
	VerifyLevel  VerifyLevel  `json:"verify_level"`
	Difficulty   int          `json:"difficulty,omitempty"`
	ChallengeTTL int          `json:"challenge_ttl,omitempty"`
	ClearanceTTL int          `json:"clearance_ttl,omitempty"`
	RateLimit    int          `json:"rate_limit,omitempty"`
	RateWindow   int          `json:"rate_window,omitempty"`
	VerifyURL    string       `json:"verify_url,omitempty"`
	VerifyPort   int          `json:"verify_port,omitempty"`
	Rules        []CustomRule `json:"rules,omitempty"`
}

// DefaultSSHConfig returns baseline defaults for SSH protection (port 22, verify level 3).
func DefaultSSHConfig() SSHConfig {
	return SSHConfig{
		Port:         DefaultSSHPort,
		Protect:      true,
		VerifyLevel:  VerifyLevelSuspiciousAndAI,
		Difficulty:   4,
		ChallengeTTL: 300,
		ClearanceTTL: 86400,
		RateLimit:    60,
		RateWindow:   60,
	}
}

// Clone returns a deep copy of SSHConfig.
func (s *SSHConfig) Clone() *SSHConfig {
	if s == nil {
		return nil
	}
	cp := *s
	if len(s.Rules) > 0 {
		cp.Rules = make([]CustomRule, len(s.Rules))
		for i, r := range s.Rules {
			cp.Rules[i] = r.Clone()
		}
	}
	return &cp
}

// UnmarshalJSON populates defaults (including Enabled=true and Protect=true) before decoding strict JSON.
func (s *SSHConfig) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	if s.Port == 0 && s.Upstream == "" && !s.Enabled && !s.Protect && s.Difficulty == 0 {
		*s = DefaultSSHConfig()
		s.Enabled = true
	}
	type sshAlias SSHConfig
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode((*sshAlias)(s))
}

// Validate normalizes and validates SSHConfig.
func (s *SSHConfig) Validate() error {
	if s == nil {
		return nil
	}
	if !s.Enabled {
		if s.Port < 0 || s.Port > 65535 {
			return fmt.Errorf("ssh port must be between 1 and 65535")
		}
		return nil
	}
	if s.Port == 0 {
		s.Port = DefaultSSHPort
	}
	if s.Port < 1 || s.Port > 65535 {
		return fmt.Errorf("ssh port must be between 1 and 65535")
	}
	up, err := NormalizeTCPUpstream(s.Upstream, "ssh upstream")
	if err != nil {
		return err
	}
	s.Upstream = up
	if !s.VerifyLevel.Valid() {
		return fmt.Errorf("ssh verify_level must be from 0 to 3")
	}
	if s.Difficulty == 0 {
		s.Difficulty = 4
	}
	if s.Difficulty < 1 || s.Difficulty > 5 {
		return fmt.Errorf("ssh difficulty must be from 1 to 5")
	}
	if s.ChallengeTTL == 0 {
		s.ChallengeTTL = 300
	}
	if s.ChallengeTTL < 0 {
		return fmt.Errorf("ssh challenge_ttl must be greater than zero")
	}
	if s.ClearanceTTL == 0 {
		s.ClearanceTTL = 86400
	}
	if s.ClearanceTTL < 0 {
		return fmt.Errorf("ssh clearance_ttl must be greater than zero")
	}
	if s.RateLimit == 0 {
		s.RateLimit = 60
	}
	if s.RateLimit < 0 {
		return fmt.Errorf("ssh rate_limit must be greater than zero")
	}
	if s.RateWindow == 0 {
		s.RateWindow = 60
	}
	if s.RateWindow < 0 {
		return fmt.Errorf("ssh rate_window must be greater than zero")
	}
	s.VerifyURL = strings.TrimSpace(s.VerifyURL)
	if s.VerifyURL != "" {
		v := VerifyServerConfig{URL: s.VerifyURL}
		if _, _, _, err := v.ParsedHost(); err != nil {
			return err
		}
	}
	if s.VerifyPort < 0 || s.VerifyPort > 65535 {
		return fmt.Errorf("ssh verify_port must be between 1 and 65535")
	}
	for i := range s.Rules {
		if err := s.Rules[i].Validate(); err != nil {
			return fmt.Errorf("ssh rule: %w", err)
		}
	}
	return nil
}

// MinecraftConfig configures Gymbag's Minecraft Java Edition protection proxy and optional managed server process.
type MinecraftConfig struct {
	Enabled          bool         `json:"enabled"`
	Port             int          `json:"port,omitempty"`
	Upstream         string       `json:"upstream"`
	ServerProperties string       `json:"server_properties,omitempty"`
	ServerJar        string       `json:"server_jar,omitempty"`
	JavaPath         string       `json:"java_path,omitempty"`
	JVMArgs          []string     `json:"jvm_args,omitempty"`
	ServerArgs       []string     `json:"server_args,omitempty"`
	WorkDir          string       `json:"work_dir,omitempty"`
	AutoRestart      *bool        `json:"auto_restart,omitempty"`
	Console          bool         `json:"console,omitempty"`
	Protect          bool         `json:"protect"`
	VerifyLevel      VerifyLevel  `json:"verify_level"`
	Difficulty       int          `json:"difficulty,omitempty"`
	ChallengeTTL     int          `json:"challenge_ttl,omitempty"`
	ClearanceTTL     int          `json:"clearance_ttl,omitempty"`
	RateLimit        int          `json:"rate_limit,omitempty"`
	RateWindow       int          `json:"rate_window,omitempty"`
	VerifyURL        string       `json:"verify_url,omitempty"`
	VerifyPort       int          `json:"verify_port,omitempty"`
	Rules            []CustomRule `json:"rules,omitempty"`
}

// AutoRestartEnabled reports whether the managed Minecraft server process should restart on exit (default true).
func (m MinecraftConfig) AutoRestartEnabled() bool {
	if m.AutoRestart == nil {
		return true
	}
	return *m.AutoRestart
}

// DefaultMinecraftConfig returns baseline defaults for Minecraft protection (port 25565, verify level 3).
func DefaultMinecraftConfig() MinecraftConfig {
	return MinecraftConfig{
		Port:         DefaultMinecraftPort,
		Protect:      true,
		VerifyLevel:  VerifyLevelSuspiciousAndAI,
		Difficulty:   4,
		ChallengeTTL: 300,
		ClearanceTTL: 86400,
		RateLimit:    60,
		RateWindow:   60,
	}
}

// Clone returns a deep copy of MinecraftConfig.
func (m *MinecraftConfig) Clone() *MinecraftConfig {
	if m == nil {
		return nil
	}
	cp := *m
	if len(m.JVMArgs) > 0 {
		cp.JVMArgs = append([]string(nil), m.JVMArgs...)
	}
	if len(m.ServerArgs) > 0 {
		cp.ServerArgs = append([]string(nil), m.ServerArgs...)
	}
	if m.AutoRestart != nil {
		b := *m.AutoRestart
		cp.AutoRestart = &b
	}
	if len(m.Rules) > 0 {
		cp.Rules = make([]CustomRule, len(m.Rules))
		for i, r := range m.Rules {
			cp.Rules[i] = r.Clone()
		}
	}
	return &cp
}

// UnmarshalJSON populates defaults (including Enabled=true and Protect=true) before decoding strict JSON.
func (m *MinecraftConfig) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	if m.Port == 0 && m.Upstream == "" && m.ServerJar == "" && !m.Enabled && !m.Protect && m.Difficulty == 0 {
		*m = DefaultMinecraftConfig()
		m.Enabled = true
	}
	type mcAlias MinecraftConfig
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode((*mcAlias)(m))
}

// Validate normalizes and validates MinecraftConfig.
func (m *MinecraftConfig) Validate() error {
	if m == nil {
		return nil
	}
	m.ServerProperties = strings.TrimSpace(m.ServerProperties)
	m.ServerJar = strings.TrimSpace(m.ServerJar)
	m.JavaPath = strings.TrimSpace(m.JavaPath)
	m.WorkDir = strings.TrimSpace(m.WorkDir)
	m.JVMArgs = compactNonEmpty(m.JVMArgs)
	m.ServerArgs = compactNonEmpty(m.ServerArgs)
	if !m.Enabled {
		if m.Port < 0 || m.Port > 65535 {
			return fmt.Errorf("minecraft port must be between 1 and 65535")
		}
		return nil
	}
	if m.Port == 0 {
		m.Port = DefaultMinecraftPort
	}
	if m.Port < 1 || m.Port > 65535 {
		return fmt.Errorf("minecraft port must be between 1 and 65535")
	}
	if strings.TrimSpace(m.Upstream) == "" && m.ServerJar != "" {
		if m.Port == 25566 {
			m.Upstream = "127.0.0.1:25567"
		} else {
			m.Upstream = "127.0.0.1:25566"
		}
	}
	up, err := NormalizeTCPUpstream(m.Upstream, "minecraft upstream")
	if err != nil {
		return err
	}
	m.Upstream = up
	if !m.VerifyLevel.Valid() {
		return fmt.Errorf("minecraft verify_level must be from 0 to 3")
	}
	if m.Difficulty == 0 {
		m.Difficulty = 4
	}
	if m.Difficulty < 1 || m.Difficulty > 5 {
		return fmt.Errorf("minecraft difficulty must be from 1 to 5")
	}
	if m.ChallengeTTL == 0 {
		m.ChallengeTTL = 300
	}
	if m.ChallengeTTL < 0 {
		return fmt.Errorf("minecraft challenge_ttl must be greater than zero")
	}
	if m.ClearanceTTL == 0 {
		m.ClearanceTTL = 86400
	}
	if m.ClearanceTTL < 0 {
		return fmt.Errorf("minecraft clearance_ttl must be greater than zero")
	}
	if m.RateLimit == 0 {
		m.RateLimit = 60
	}
	if m.RateLimit < 0 {
		return fmt.Errorf("minecraft rate_limit must be greater than zero")
	}
	if m.RateWindow == 0 {
		m.RateWindow = 60
	}
	if m.RateWindow < 0 {
		return fmt.Errorf("minecraft rate_window must be greater than zero")
	}
	m.VerifyURL = strings.TrimSpace(m.VerifyURL)
	if m.VerifyURL != "" {
		v := VerifyServerConfig{URL: m.VerifyURL}
		if _, _, _, err := v.ParsedHost(); err != nil {
			return err
		}
	}
	if m.VerifyPort < 0 || m.VerifyPort > 65535 {
		return fmt.Errorf("minecraft verify_port must be between 1 and 65535")
	}
	for i := range m.Rules {
		if err := m.Rules[i].Validate(); err != nil {
			return fmt.Errorf("minecraft rule: %w", err)
		}
	}
	return nil
}

// NormalizeTCPUpstream normalizes raw TCP target addresses (":2222", "127.0.0.1:2222",
// "tcp://127.0.0.1:25566", "[::1]:2222") into "host:port".
func NormalizeTCPUpstream(raw, fieldName string) (string, error) {
	target := strings.TrimSpace(raw)
	if target == "" {
		return "", fmt.Errorf("%s must not be empty", fieldName)
	}
	if strings.HasPrefix(strings.ToLower(target), "tcp://") {
		target = target[6:]
	} else if strings.Contains(target, "://") {
		return "", fmt.Errorf("%s must be a host:port or tcp://host:port address", fieldName)
	}
	if strings.HasPrefix(target, ":") {
		target = "127.0.0.1" + target
	}
	h, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return "", fmt.Errorf("%s must include host and port (e.g. 127.0.0.1:2222): %w", fieldName, err)
	}
	h = host.Normalize(h)
	if h == "" {
		return "", fmt.Errorf("%s host must not be empty", fieldName)
	}
	p, err := host.ParsePort(portStr, fieldName+" port")
	if err != nil {
		return "", err
	}
	if addr, err := netip.ParseAddr(h); err == nil {
		return net.JoinHostPort(addr.Unmap().String(), strconv.Itoa(p)), nil
	}
	return net.JoinHostPort(h, strconv.Itoa(p)), nil
}

// MarshalConciseJSONConfig serializes Config omitting default boilerplate and inactive services.
func MarshalConciseJSONConfig(c Config) ([]byte, error) {
	defTLS := DefaultTLSConfig()
	defIntel := DefaultIntelConfig()
	defWAF := DefaultSiteWAFConfig()

	type conciseWAF struct {
		VerifyLevel           *VerifyLevel `json:"verify_level,omitempty"`
		Difficulty            int          `json:"difficulty,omitempty"`
		ChallengeTTL          int          `json:"challenge_ttl,omitempty"`
		ClearanceTTL          int          `json:"clearance_ttl,omitempty"`
		CookieName            string       `json:"cookie_name,omitempty"`
		CookieSecure          *bool        `json:"cookie_secure,omitempty"`
		RateLimit             int          `json:"rate_limit,omitempty"`
		RateWindow            int          `json:"rate_window,omitempty"`
		DefaultRules          *bool        `json:"default_rules,omitempty"`
		Rules                 []CustomRule `json:"rules,omitempty"`
		RemoteRules           []string     `json:"remote_rules,omitempty"`
		RuleSources           []RuleSource `json:"rule_sources,omitempty"`
		CustomAIBots          []string     `json:"custom_ai_bots,omitempty"`
		CustomSuspiciousUAs   []string     `json:"custom_suspicious_uas,omitempty"`
		CustomSuspiciousPaths []string     `json:"custom_suspicious_paths,omitempty"`
	}

	buildConciseWAF := func(w SiteWAFConfig, includeVerify bool) *conciseWAF {
		cw := &conciseWAF{}
		hasField := false
		if includeVerify {
			v := w.VerifyLevel
			cw.VerifyLevel = &v
			hasField = true
		}
		if w.Difficulty != 0 && w.Difficulty != defWAF.Difficulty {
			cw.Difficulty = w.Difficulty
			hasField = true
		}
		if w.ChallengeTTL != 0 && w.ChallengeTTL != defWAF.ChallengeTTL {
			cw.ChallengeTTL = w.ChallengeTTL
			hasField = true
		}
		if w.ClearanceTTL != 0 && w.ClearanceTTL != defWAF.ClearanceTTL {
			cw.ClearanceTTL = w.ClearanceTTL
			hasField = true
		}
		if w.CookieName != "" && w.CookieName != defWAF.CookieName {
			cw.CookieName = w.CookieName
			hasField = true
		}
		if w.CookieSecure != defWAF.CookieSecure {
			b := w.CookieSecure
			cw.CookieSecure = &b
			hasField = true
		}
		if w.RateLimit != 0 && w.RateLimit != defWAF.RateLimit {
			cw.RateLimit = w.RateLimit
			hasField = true
		}
		if w.RateWindow != 0 && w.RateWindow != defWAF.RateWindow {
			cw.RateWindow = w.RateWindow
			hasField = true
		}
		if w.DefaultRules != nil && !*w.DefaultRules {
			b := false
			cw.DefaultRules = &b
			hasField = true
		}
		if len(w.Rules) > 0 {
			cw.Rules = w.Rules
			hasField = true
		}
		if len(w.RemoteRules) > 0 {
			cw.RemoteRules = w.RemoteRules
			hasField = true
		}
		if len(w.RuleSources) > 0 {
			cw.RuleSources = w.RuleSources
			hasField = true
		}
		if len(w.CustomAIBots) > 0 {
			cw.CustomAIBots = w.CustomAIBots
			hasField = true
		}
		if len(w.CustomSuspiciousUAs) > 0 {
			cw.CustomSuspiciousUAs = w.CustomSuspiciousUAs
			hasField = true
		}
		if len(w.CustomSuspiciousPaths) > 0 {
			cw.CustomSuspiciousPaths = w.CustomSuspiciousPaths
			hasField = true
		}
		if !hasField {
			return nil
		}
		return cw
	}

	type conciseSite struct {
		Domain     string             `json:"domain"`
		Upstreams  []string           `json:"upstreams,omitempty"`
		Routes     []RouteConfig      `json:"routes,omitempty"`
		Headers    map[string]string  `json:"headers,omitempty"`
		Auth       *ForwardAuthConfig `json:"auth,omitempty"`
		Protect    *bool              `json:"protect,omitempty"`
		WAF        *conciseWAF        `json:"waf,omitempty"`
		CertFile   string             `json:"cert_file,omitempty"`
		KeyFile    string             `json:"key_file,omitempty"`
		AcmeShPath string             `json:"acme_sh_path,omitempty"`
		Mascot     *MascotConfig      `json:"mascot,omitempty"`
	}

	type conciseTCP struct {
		Port             int          `json:"port,omitempty"`
		Upstream         string       `json:"upstream"`
		ServerProperties string       `json:"server_properties,omitempty"`
		ServerJar        string       `json:"server_jar,omitempty"`
		JavaPath         string       `json:"java_path,omitempty"`
		JVMArgs          []string     `json:"jvm_args,omitempty"`
		ServerArgs       []string     `json:"server_args,omitempty"`
		WorkDir          string       `json:"work_dir,omitempty"`
		AutoRestart      *bool        `json:"auto_restart,omitempty"`
		Console          bool         `json:"console,omitempty"`
		Protect          *bool        `json:"protect,omitempty"`
		VerifyLevel      *VerifyLevel `json:"verify_level,omitempty"`
		Difficulty       int          `json:"difficulty,omitempty"`
		ChallengeTTL     int          `json:"challenge_ttl,omitempty"`
		ClearanceTTL     int          `json:"clearance_ttl,omitempty"`
		RateLimit        int          `json:"rate_limit,omitempty"`
		RateWindow       int          `json:"rate_window,omitempty"`
		VerifyURL        string       `json:"verify_url,omitempty"`
		VerifyPort       int          `json:"verify_port,omitempty"`
		Rules            []CustomRule `json:"rules,omitempty"`
	}

	type conciseVerify struct {
		URL      string `json:"url,omitempty"`
		Port     int    `json:"port,omitempty"`
		CertFile string `json:"cert_file,omitempty"`
		KeyFile  string `json:"key_file,omitempty"`
	}

	type conciseConfig struct {
		Sites              []conciseSite      `json:"sites,omitempty"`
		SSH                *conciseTCP        `json:"ssh,omitempty"`
		Minecraft          *conciseTCP        `json:"minecraft,omitempty"`
		Verify             *conciseVerify     `json:"verify,omitempty"`
		Authelia           *ForwardAuthConfig `json:"authelia,omitempty"`
		BindHost           string             `json:"bind_host,omitempty"`
		HTTPPort           int                `json:"http_port,omitempty"`
		HTTPSPort          int                `json:"https_port,omitempty"`
		DirectHTTP         bool               `json:"direct_http,omitempty"`
		AdminAddr          string             `json:"admin_addr,omitempty"`
		TLS                map[string]any     `json:"tls,omitempty"`
		Intel              map[string]any     `json:"intel,omitempty"`
		RemoteRulesRefresh int                `json:"remote_rules_refresh,omitempty"`
		RulesWatchInterval int                `json:"rules_watch_interval,omitempty"`
		RedisURL           string             `json:"redis_url,omitempty"`
		Secret             string             `json:"secret,omitempty"`
		TrustedProxies     []string           `json:"trusted_proxies,omitempty"`
		Mascot             *MascotConfig      `json:"mascot,omitempty"`
		Defaults           *conciseWAF        `json:"defaults,omitempty"`
	}

	out := conciseConfig{
		Authelia:       c.Authelia,
		DirectHTTP:     c.DirectHTTP,
		AdminAddr:      c.AdminAddr,
		RedisURL:       c.RedisURL,
		Secret:         c.Secret,
		TrustedProxies: c.TrustedProxies,
	}
	if !c.Mascot.IsEmpty() {
		out.Mascot = c.Mascot.Clone()
	}

	if c.BindHost != "" && c.BindHost != DefaultBindHostForConfig(c) {
		out.BindHost = c.BindHost
	}
	if c.HTTPPort != 0 && c.HTTPPort != 80 {
		out.HTTPPort = c.HTTPPort
	}
	if c.HTTPSPort != 0 && c.HTTPSPort != 443 {
		out.HTTPSPort = c.HTTPSPort
	}
	if c.RemoteRulesRefresh != 0 && c.RemoteRulesRefresh != DefaultRemoteRulesRefresh {
		out.RemoteRulesRefresh = c.RemoteRulesRefresh
	}
	if c.RulesWatchInterval != 0 && c.RulesWatchInterval != DefaultRulesWatchInterval {
		out.RulesWatchInterval = c.RulesWatchInterval
	}

	for _, s := range c.Sites {
		cs := conciseSite{
			Domain:     s.Domain,
			Upstreams:  s.Upstreams,
			Routes:     s.Routes,
			Headers:    s.Headers,
			Auth:       s.Auth,
			CertFile:   s.CertFile,
			KeyFile:    s.KeyFile,
			AcmeShPath: s.AcmeShPath,
		}
		if !s.Mascot.IsEmpty() {
			cs.Mascot = s.Mascot.Clone()
		}
		if !s.Protect {
			f := false
			cs.Protect = &f
			cs.WAF = buildConciseWAF(s.WAF, false)
		} else {
			cs.WAF = buildConciseWAF(s.WAF, true)
		}
		out.Sites = append(out.Sites, cs)
	}

	if c.SSHEnabled() {
		def := DefaultSSHConfig()
		ct := &conciseTCP{
			Port:     c.SSH.Port,
			Upstream: c.SSH.Upstream,
		}
		if !c.SSH.Protect {
			f := false
			ct.Protect = &f
		} else {
			v := c.SSH.VerifyLevel
			ct.VerifyLevel = &v
		}
		if c.SSH.Difficulty != 0 && c.SSH.Difficulty != def.Difficulty {
			ct.Difficulty = c.SSH.Difficulty
		}
		if c.SSH.ChallengeTTL != 0 && c.SSH.ChallengeTTL != def.ChallengeTTL {
			ct.ChallengeTTL = c.SSH.ChallengeTTL
		}
		if c.SSH.ClearanceTTL != 0 && c.SSH.ClearanceTTL != def.ClearanceTTL {
			ct.ClearanceTTL = c.SSH.ClearanceTTL
		}
		if c.SSH.RateLimit != 0 && c.SSH.RateLimit != def.RateLimit {
			ct.RateLimit = c.SSH.RateLimit
		}
		if c.SSH.RateWindow != 0 && c.SSH.RateWindow != def.RateWindow {
			ct.RateWindow = c.SSH.RateWindow
		}
		ct.VerifyURL = c.SSH.VerifyURL
		ct.VerifyPort = c.SSH.VerifyPort
		ct.Rules = c.SSH.Rules
		out.SSH = ct
	}

	if c.MinecraftEnabled() {
		def := DefaultMinecraftConfig()
		ct := &conciseTCP{
			Port:             c.Minecraft.Port,
			Upstream:         c.Minecraft.Upstream,
			ServerProperties: c.Minecraft.ServerProperties,
			ServerJar:        c.Minecraft.ServerJar,
			JavaPath:         c.Minecraft.JavaPath,
			JVMArgs:          c.Minecraft.JVMArgs,
			ServerArgs:       c.Minecraft.ServerArgs,
			WorkDir:          c.Minecraft.WorkDir,
			Console:          c.Minecraft.Console,
		}
		if c.Minecraft.AutoRestart != nil && !*c.Minecraft.AutoRestart {
			f := false
			ct.AutoRestart = &f
		}
		if !c.Minecraft.Protect {
			f := false
			ct.Protect = &f
		} else {
			v := c.Minecraft.VerifyLevel
			ct.VerifyLevel = &v
		}
		if c.Minecraft.Difficulty != 0 && c.Minecraft.Difficulty != def.Difficulty {
			ct.Difficulty = c.Minecraft.Difficulty
		}
		if c.Minecraft.ChallengeTTL != 0 && c.Minecraft.ChallengeTTL != def.ChallengeTTL {
			ct.ChallengeTTL = c.Minecraft.ChallengeTTL
		}
		if c.Minecraft.ClearanceTTL != 0 && c.Minecraft.ClearanceTTL != def.ClearanceTTL {
			ct.ClearanceTTL = c.Minecraft.ClearanceTTL
		}
		if c.Minecraft.RateLimit != 0 && c.Minecraft.RateLimit != def.RateLimit {
			ct.RateLimit = c.Minecraft.RateLimit
		}
		if c.Minecraft.RateWindow != 0 && c.Minecraft.RateWindow != def.RateWindow {
			ct.RateWindow = c.Minecraft.RateWindow
		}
		ct.VerifyURL = c.Minecraft.VerifyURL
		ct.VerifyPort = c.Minecraft.VerifyPort
		ct.Rules = c.Minecraft.Rules
		out.Minecraft = ct
	}

	if c.Verify.URL != "" || (c.Verify.Port != 0 && c.Verify.Port != DefaultVerifyPort) || c.Verify.CertFile != "" || c.Verify.KeyFile != "" {
		cv := &conciseVerify{
			URL:      c.Verify.URL,
			CertFile: c.Verify.CertFile,
			KeyFile:  c.Verify.KeyFile,
		}
		if c.Verify.Port != 0 && c.Verify.Port != DefaultVerifyPort {
			cv.Port = c.Verify.Port
		}
		out.Verify = cv
	}

	tlsMap := make(map[string]any)
	if c.TLS.Provider != "" {
		tlsMap["provider"] = c.TLS.Provider
	}
	impliedMode := ""
	if c.TLS.Provider == "local" {
		impliedMode = "local"
	} else if c.TLS.Provider != "" {
		impliedMode = "acme"
	}
	if c.TLS.AutoSSL != nil {
		tlsMap["auto_ssl"] = *c.TLS.AutoSSL
	}
	if c.TLS.Mode != "" && c.TLS.Mode != defTLS.Mode && c.TLS.Mode != impliedMode {
		tlsMap["mode"] = c.TLS.Mode
	}
	if c.TLS.CertFile != "" && c.TLS.CertFile != defTLS.CertFile {
		tlsMap["cert_file"] = c.TLS.CertFile
	}
	if c.TLS.KeyFile != "" && c.TLS.KeyFile != defTLS.KeyFile {
		tlsMap["key_file"] = c.TLS.KeyFile
	}
	if c.TLS.AcmeShPath != "" {
		tlsMap["acme_sh_path"] = c.TLS.AcmeShPath
	}
	if c.TLS.KeyPassword != "" {
		tlsMap["key_password"] = c.TLS.KeyPassword
	}
	if c.TLS.AccountKeyFile != "" {
		tlsMap["account_key_file"] = c.TLS.AccountKeyFile
	}
	if c.TLS.EABKID != "" {
		tlsMap["eab_kid"] = c.TLS.EABKID
	}
	if c.TLS.EABHMACKey != "" {
		tlsMap["eab_hmac_key"] = c.TLS.EABHMACKey
	}
	if c.TLS.AutoHosts != nil {
		tlsMap["auto_hosts"] = *c.TLS.AutoHosts
	}
	if c.TLS.CertDir != "" && c.TLS.CertDir != defTLS.CertDir {
		tlsMap["cert_dir"] = c.TLS.CertDir
	}
	if c.TLS.ACMEEmail != "" {
		tlsMap["acme_email"] = c.TLS.ACMEEmail
	}
	if c.TLS.ACMEDirectory != "" && c.TLS.ACMEDirectory != defTLS.ACMEDirectory && c.TLS.Provider != "custom" {
		tlsMap["acme_directory"] = c.TLS.ACMEDirectory
	}
	if c.TLS.RenewInterval != 0 && c.TLS.RenewInterval != defTLS.RenewInterval {
		tlsMap["renew_interval"] = c.TLS.RenewInterval
	}
	if c.TLS.RenewWithinDays != 0 && c.TLS.RenewWithinDays != defTLS.RenewWithinDays {
		tlsMap["renew_within_days"] = c.TLS.RenewWithinDays
	}
	if c.TLS.HSTS != nil {
		tlsMap["hsts"] = *c.TLS.HSTS
	}
	if len(tlsMap) > 0 {
		out.TLS = tlsMap
	}

	intelMap := make(map[string]any)
	if c.Intel.IPSecurity != defIntel.IPSecurity {
		intelMap["ip_security"] = c.Intel.IPSecurity
	}
	if c.Intel.IPSecurityAction != "" && c.Intel.IPSecurityAction != defIntel.IPSecurityAction {
		intelMap["ip_security_action"] = c.Intel.IPSecurityAction
	}
	if c.Intel.LiveReputation != defIntel.LiveReputation {
		intelMap["live_reputation"] = c.Intel.LiveReputation
	}
	if c.Intel.AIIPList != defIntel.AIIPList {
		intelMap["ai_ip_list"] = c.Intel.AIIPList
	}
	if c.Intel.VPNDetection != defIntel.VPNDetection {
		intelMap["vpn_detection"] = c.Intel.VPNDetection
	}
	if c.Intel.RefreshInterval != 0 && c.Intel.RefreshInterval != defIntel.RefreshInterval {
		intelMap["refresh_interval"] = c.Intel.RefreshInterval
	}
	if len(c.Intel.ThreatFeeds) > 0 {
		intelMap["threat_feeds"] = c.Intel.ThreatFeeds
	}
	if len(c.Intel.AIIPFeeds) > 0 {
		intelMap["ai_ip_feeds"] = c.Intel.AIIPFeeds
	}
	if len(c.Intel.VPNFeeds) > 0 {
		intelMap["vpn_feeds"] = c.Intel.VPNFeeds
	}
	if len(intelMap) > 0 {
		out.Intel = intelMap
	}

	includeDefVerify := c.Defaults.VerifyLevel != defWAF.VerifyLevel
	if defWAFConcise := buildConciseWAF(c.Defaults, includeDefVerify); defWAFConcise != nil {
		out.Defaults = defWAFConcise
	}

	return json.MarshalIndent(out, "", "  ")
}

// DefaultBindHostForConfig returns 0.0.0.0 when SSH, Minecraft, or any public HTTP domain is active,
// otherwise 127.0.0.1.
func DefaultBindHostForConfig(c Config) string {
	if c.SSHEnabled() || c.MinecraftEnabled() {
		return "0.0.0.0"
	}
	return DefaultBindHost(c.Domains())
}

func checkPortConflict(c *Config) error {
	type portEntry struct {
		name string
		port int
	}
	var ports []portEntry
	if c.HTTPEnabled() {
		ports = append(ports, portEntry{"http_port", c.HTTPPort})
		if c.HTTPSEnabled() {
			ports = append(ports, portEntry{"https_port", c.HTTPSPort})
		}
	}
	if c.SSHEnabled() {
		ports = append(ports, portEntry{"ssh.port", c.SSH.Port})
	}
	if c.MinecraftEnabled() {
		ports = append(ports, portEntry{"minecraft.port", c.Minecraft.Port})
	}
	if c.NeedsVerifyServer() {
		vPort := c.Verify.EffectivePort()
		if !(c.HTTPSEnabled() && vPort == c.HTTPSPort) {
			ports = append(ports, portEntry{"verify.port", vPort})
		}
	}
	for i := 0; i < len(ports); i++ {
		for j := i + 1; j < len(ports); j++ {
			if ports[i].port == ports[j].port {
				return fmt.Errorf("listen port %d conflicts between %s and %s", ports[i].port, ports[i].name, ports[j].name)
			}
		}
	}
	// Check duplicate upstream ports vs listen ports when loopback
	_ = slices.Clip(ports)
	return nil
}

func compactNonEmpty(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	var out []string
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}
