package config

import (
	"fmt"
	"strings"

	"gymbag/internal/host"
	"gymbag/internal/localpki"
)

// SiteSpec describes a single downstream site entry for config generation.
type SiteSpec struct {
	Domain       string
	Upstreams    []string
	VerifyLevel  string
	Protect      *bool
	Difficulty   int
	ClearanceTTL string
	RemoteRules  []string
	CertFile     string
	KeyFile      string
}

// GenerateOptions controls how GenerateConfig and RenderGeneratedConfig construct a Gymbag config.
type GenerateOptions struct {
	Preset             string
	Domain             string
	Upstream           string
	Sites              []SiteSpec
	VerifyLevel        string
	Protect            *bool
	Difficulty         int
	ClearanceTTL       string
	RemoteRules        []string
	RemoteRulesRefresh string
	DefaultRules       *bool
	SSLProvider        string
	Email              string
	EABKID             string
	EABHMACKey         string
	HTTPPort           int
	HTTPSPort          int
	BindHost           string
	AdminAddr          string
	CertDir            string
	RedisURL           string
	AutoHosts          *bool
	IPSecurityAction   string
	Format             string // "gymbagfile" (default) or "json"
	IncludeComments    bool
}

// ParseSiteSpec parses a "domain=upstream" (or "domain1,domain2=upstream") CLI specification.
func ParseSiteSpec(raw string) ([]SiteSpec, error) {
	lhs, rhs, ok := strings.Cut(strings.TrimSpace(raw), "=")
	if !ok || strings.TrimSpace(lhs) == "" || strings.TrimSpace(rhs) == "" {
		return nil, fmt.Errorf("invalid --site %q (expected domain=upstream, e.g. app.example.com=http://localhost:3000)", raw)
	}
	domains, err := splitDomains(lhs)
	if err != nil {
		return nil, err
	}
	upstreams, err := splitAndNormalizeUpstreams(rhs)
	if err != nil {
		return nil, err
	}
	out := make([]SiteSpec, 0, len(domains))
	for _, d := range domains {
		out = append(out, SiteSpec{
			Domain:    d,
			Upstreams: append([]string(nil), upstreams...),
		})
	}
	return out, nil
}

func splitAndNormalizeUpstreams(raw string) ([]string, error) {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	if len(fields) == 0 {
		return nil, fmt.Errorf("upstream URL must not be empty")
	}
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		norm, err := NormalizeUpstream(f, "upstream")
		if err != nil {
			return nil, err
		}
		out = append(out, norm)
	}
	return out, nil
}

// GenerateConfig constructs and validates a Config from GenerateOptions.
func GenerateConfig(opts GenerateOptions) (Config, error) {
	cfg := Default()
	cfg.Sites = nil

	defaultDomain := "localhost"
	defaultUpstream := "http://localhost:5173"
	defaultVerify := VerifyLevelSuspiciousAndAI
	defaultProtect := true
	defaultDifficulty := 4

	preset := strings.ToLower(strings.TrimSpace(opts.Preset))
	switch preset {
	case "", "default", "standard":
		// Use standard defaults above.
	case "local", "dev":
		defaultDomain = "app.internal"
		defaultUpstream = "http://localhost:3000"
		if strings.TrimSpace(opts.SSLProvider) == "" {
			opts.SSLProvider = "local"
		}
	case "public", "production", "prod":
		defaultUpstream = "http://localhost:3000"
		if strings.TrimSpace(opts.Domain) == "" && len(opts.Sites) == 0 {
			return Config{}, fmt.Errorf("preset %q requires --domain or --site", opts.Preset)
		}
	case "strict":
		defaultVerify = VerifyLevelAlways
	case "ai-only", "ai":
		defaultVerify = VerifyLevelOnlyAI
	case "suspicious":
		defaultVerify = VerifyLevelOnlySuspicious
	case "proxy-only", "unprotected", "off":
		defaultProtect = false
	default:
		return Config{}, fmt.Errorf("unsupported preset %q (expected standard, local, public, strict, ai-only, suspicious, or proxy-only)", opts.Preset)
	}

	// Global defaults & overrides
	cfg.Defaults.VerifyLevel = defaultVerify
	cfg.Defaults.Difficulty = defaultDifficulty
	if opts.Difficulty != 0 {
		cfg.Defaults.Difficulty = opts.Difficulty
	}
	if rawTTL := strings.TrimSpace(opts.ClearanceTTL); rawTTL != "" {
		ttl, err := ParseDurationSeconds(rawTTL, "verify-timeout")
		if err != nil {
			return Config{}, err
		}
		cfg.Defaults.ClearanceTTL = ttl
	}
	if len(opts.RemoteRules) > 0 {
		urls, err := splitRemoteRuleURLs(opts.RemoteRules)
		if err != nil {
			return Config{}, err
		}
		cfg.Defaults.RemoteRules = urls
	}
	if rawRefresh := strings.TrimSpace(opts.RemoteRulesRefresh); rawRefresh != "" {
		sec, err := ParseDurationSeconds(rawRefresh, "remote-rules-refresh")
		if err != nil {
			return Config{}, err
		}
		cfg.RemoteRulesRefresh = sec
	}
	if opts.DefaultRules != nil {
		b := *opts.DefaultRules
		cfg.Defaults.DefaultRules = &b
	}

	if strings.TrimSpace(opts.VerifyLevel) != "" {
		lvl, prot, err := ParseVerifyLevel(opts.VerifyLevel)
		if err != nil {
			return Config{}, err
		}
		cfg.Defaults.VerifyLevel = lvl
		defaultProtect = prot
	}
	if opts.Protect != nil {
		defaultProtect = *opts.Protect
	}

	// Build sites
	specs := append([]SiteSpec(nil), opts.Sites...)
	if len(specs) == 0 || strings.TrimSpace(opts.Domain) != "" || strings.TrimSpace(opts.Upstream) != "" {
		rawDomains := strings.TrimSpace(opts.Domain)
		if rawDomains == "" {
			rawDomains = defaultDomain
		}
		domains, err := splitDomains(rawDomains)
		if err != nil {
			return Config{}, err
		}
		rawUpstream := strings.TrimSpace(opts.Upstream)
		if rawUpstream == "" {
			rawUpstream = defaultUpstream
		}
		upstreams, err := splitAndNormalizeUpstreams(rawUpstream)
		if err != nil {
			return Config{}, err
		}
		for _, d := range domains {
			specs = append(specs, SiteSpec{
				Domain:    d,
				Upstreams: append([]string(nil), upstreams...),
			})
		}
	}

	for _, sp := range specs {
		d := host.Normalize(sp.Domain)
		if d == "" {
			return Config{}, fmt.Errorf("site domain must not be empty")
		}
		if len(sp.Upstreams) == 0 {
			return Config{}, fmt.Errorf("site %s requires at least one upstream URL", d)
		}
		wafCfg := cloneSiteWAF(cfg.Defaults)
		prot := defaultProtect
		if sp.VerifyLevel != "" {
			lvl, p, err := ParseVerifyLevel(sp.VerifyLevel)
			if err != nil {
				return Config{}, err
			}
			wafCfg.VerifyLevel = lvl
			prot = p
		}
		if sp.Protect != nil {
			prot = *sp.Protect
		}
		if sp.Difficulty != 0 {
			wafCfg.Difficulty = sp.Difficulty
		}
		if rawSiteTTL := strings.TrimSpace(sp.ClearanceTTL); rawSiteTTL != "" {
			ttl, err := ParseDurationSeconds(rawSiteTTL, "verify-timeout")
			if err != nil {
				return Config{}, err
			}
			wafCfg.ClearanceTTL = ttl
		}
		if len(sp.RemoteRules) > 0 {
			urls, err := splitRemoteRuleURLs(sp.RemoteRules)
			if err != nil {
				return Config{}, err
			}
			for _, u := range urls {
				if !stringsContainsSlice(wafCfg.RemoteRules, u) {
					wafCfg.RemoteRules = append(wafCfg.RemoteRules, u)
				}
			}
		}
		cfg.Sites = append(cfg.Sites, SiteConfig{
			Domain:    d,
			Upstreams: append([]string(nil), sp.Upstreams...),
			Protect:   prot,
			WAF:       wafCfg,
			CertFile:  strings.TrimSpace(sp.CertFile),
			KeyFile:   strings.TrimSpace(sp.KeyFile),
		})
	}

	// TLS & network settings
	if strings.TrimSpace(opts.SSLProvider) != "" {
		if err := applySSLProvider(opts.SSLProvider, &cfg.TLS); err != nil {
			return Config{}, err
		}
	}
	if email := strings.TrimSpace(opts.Email); email != "" {
		cfg.TLS.ACMEEmail = email
	}
	if kid := strings.TrimSpace(opts.EABKID); kid != "" {
		cfg.TLS.EABKID = kid
	}
	if hmacKey := strings.TrimSpace(opts.EABHMACKey); hmacKey != "" {
		cfg.TLS.EABHMACKey = hmacKey
	}
	if opts.AutoHosts != nil {
		b := *opts.AutoHosts
		cfg.TLS.AutoHosts = &b
	}
	if certDir := strings.TrimSpace(opts.CertDir); certDir != "" {
		cfg.TLS.CertDir = certDir
	}
	if opts.HTTPPort != 0 {
		cfg.HTTPPort = opts.HTTPPort
	}
	if opts.HTTPSPort != 0 {
		cfg.HTTPSPort = opts.HTTPSPort
	}
	if admin := strings.TrimSpace(opts.AdminAddr); admin != "" && !strings.EqualFold(admin, "off") {
		cfg.AdminAddr = admin
	}
	if redisURL := strings.TrimSpace(opts.RedisURL); redisURL != "" {
		cfg.RedisURL = redisURL
	}
	if act := strings.TrimSpace(opts.IPSecurityAction); act != "" {
		cfg.Intel.IPSecurityAction = act
	}
	if bind := strings.TrimSpace(opts.BindHost); bind != "" {
		cfg.BindHost = bind
	} else {
		cfg.BindHost = DefaultBindHost(cfg.Domains())
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func stringsContainsSlice(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

// NormalizeConfigFormat normalizes a config output format ("gymbagfile" or "json").
func NormalizeConfigFormat(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "gymbagfile", "caddyfile", "caddy":
		return "gymbagfile", nil
	case "json":
		return "json", nil
	default:
		return "", fmt.Errorf("unsupported config format %q (expected gymbagfile or json)", raw)
	}
}

// RenderGeneratedConfig builds, validates, and serializes a Config in Gymbagfile or JSON format.
func RenderGeneratedConfig(opts GenerateOptions) ([]byte, Config, error) {
	format, err := NormalizeConfigFormat(opts.Format)
	if err != nil {
		return nil, Config{}, err
	}
	cfg, err := GenerateConfig(opts)
	if err != nil {
		return nil, Config{}, err
	}

	if format == "json" {
		raw, err := MarshalJSONConfig(cfg)
		if err != nil {
			return nil, Config{}, err
		}
		return append(raw, '\n'), cfg, nil
	}

	body := FormatConfig(cfg)
	var b strings.Builder
	if opts.IncludeComments {
		b.WriteString("# Gymbag configuration generated by `gymbag init`\n")
		b.WriteString("# Validate: gymbag validate\n")
		b.WriteString("# Start:    gymbag\n")
		if localpki.AllLocalDomains(cfg.Domains()) {
			b.WriteString(fmt.Sprintf("# Local PKI root CA: %s/ca/neuro-sama-root-ca.pem\n", strings.TrimRight(cfg.TLS.CertDir, "/\\")))
		}
		b.WriteByte('\n')
	}
	b.WriteString(body)

	// Verify generated Gymbagfile parses cleanly back into a valid Config.
	if _, err := ParseCaddyfile(b.String()); err != nil {
		return nil, Config{}, fmt.Errorf("internal error: generated Gymbagfile failed validation: %w", err)
	}
	return []byte(b.String()), cfg, nil
}
