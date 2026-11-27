package config

import (
	"fmt"
	"net/url"
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
	RuleSources  []RuleSource
	CertFile     string
	KeyFile      string
}

// TCPSpec describes an SSH or Minecraft protection service entry for config generation.
type TCPSpec struct {
	Port             int
	Upstream         string
	ServerProperties string
	ServerJar        string
	JavaPath         string
	JVMArgs          []string
	ServerArgs       []string
	WorkDir          string
	AutoRestart      *bool
	Console          bool
	VerifyLevel      string
	Protect          *bool
	Difficulty       int
	ClearanceTTL     string
	VerifyURL        string
	VerifyPort       int
	Rules            []CustomRule
}

// GenerateOptions controls how GenerateConfig and RenderGeneratedConfig construct a Gymbag config.
type GenerateOptions struct {
	Preset             string
	Templates          []string
	Domain             string
	Upstream           string
	Sites              []SiteSpec
	SSH                *TCPSpec
	Minecraft          *TCPSpec
	Verify             VerifyServerConfig
	Authelia           *ForwardAuthConfig
	VerifyLevel        string
	Protect            *bool
	Difficulty         int
	ClearanceTTL       string
	RemoteRules        []string
	RemoteRulesRefresh string
	RuleSources        []RuleSource
	RulesWatchInterval string
	DefaultRules       *bool
	SSLProvider        string
	AutoSSL            *bool
	AcmeShPath         string
	CertFile           string
	KeyFile            string
	DirectHTTP         bool
	HTTPOnly           bool
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

// ResolveTemplateSelection parses template names (http, ssh, minecraft/mc, all) into service flags.
func ResolveTemplateSelection(raw []string) (useHTTP, useSSH, useMC bool, err error) {
	for _, entry := range raw {
		for _, part := range strings.Split(entry, ",") {
			t := strings.ToLower(strings.TrimSpace(part))
			if t == "" {
				continue
			}
			switch t {
			case "http", "https", "web", "proxy":
				useHTTP = true
			case "ssh":
				useSSH = true
			case "minecraft", "mc":
				useMC = true
			case "all":
				useHTTP, useSSH, useMC = true, true, true
			default:
				return false, false, false, fmt.Errorf("unsupported template %q (expected http, ssh, minecraft, or all)", part)
			}
		}
	}
	return useHTTP, useSSH, useMC, nil
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

	useHTTP, useSSH, useMC, err := ResolveTemplateSelection(opts.Templates)
	if err != nil {
		return Config{}, err
	}
	if opts.SSH != nil {
		useSSH = true
	}
	if opts.Minecraft != nil {
		useMC = true
	}

	preset := strings.ToLower(strings.TrimSpace(opts.Preset))
	switch preset {
	case "", "default", "standard":
		// Use standard defaults above.
	case "http", "web":
		useHTTP = true
	case "ssh":
		useSSH = true
	case "minecraft", "mc":
		useMC = true
	case "all":
		useHTTP, useSSH, useMC = true, true, true
	case "local", "dev":
		useHTTP = true
		defaultDomain = "app.internal"
		defaultUpstream = "http://localhost:3000"
		if strings.TrimSpace(opts.SSLProvider) == "" {
			opts.SSLProvider = "local"
		}
	case "public", "production", "prod":
		useHTTP = true
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
		return Config{}, fmt.Errorf("unsupported preset %q (expected standard, local, public, strict, ai-only, suspicious, proxy-only, http, ssh, minecraft, or all)", opts.Preset)
	}

	if len(opts.Sites) > 0 || strings.TrimSpace(opts.Domain) != "" || strings.TrimSpace(opts.Upstream) != "" {
		useHTTP = true
	}
	if !useHTTP && !useSSH && !useMC {
		useHTTP = true
	}

	// Build shared base WAF template without polluting cfg.Defaults.VerifyLevel when not overridden.
	baseWAF := DefaultSiteWAFConfig()
	baseWAF.VerifyLevel = defaultVerify
	baseWAF.Difficulty = defaultDifficulty
	if opts.Difficulty != 0 {
		baseWAF.Difficulty = opts.Difficulty
		cfg.Defaults.Difficulty = opts.Difficulty
	}
	if rawTTL := strings.TrimSpace(opts.ClearanceTTL); rawTTL != "" {
		ttl, err := ParseDurationSeconds(rawTTL, "verify-timeout")
		if err != nil {
			return Config{}, err
		}
		baseWAF.ClearanceTTL = ttl
		cfg.Defaults.ClearanceTTL = ttl
	}
	if len(opts.RemoteRules) > 0 {
		urls, err := splitRemoteRuleURLs(opts.RemoteRules)
		if err != nil {
			return Config{}, err
		}
		baseWAF.RemoteRules = urls
		cfg.Defaults.RemoteRules = urls
	}
	if rawRefresh := strings.TrimSpace(opts.RemoteRulesRefresh); rawRefresh != "" {
		sec, err := ParseDurationSeconds(rawRefresh, "remote-rules-refresh")
		if err != nil {
			return Config{}, err
		}
		cfg.RemoteRulesRefresh = sec
	}
	if len(opts.RuleSources) > 0 {
		for _, rs := range opts.RuleSources {
			if err := rs.Validate(); err != nil {
				return Config{}, err
			}
			baseWAF.RuleSources = append(baseWAF.RuleSources, rs)
			cfg.Defaults.RuleSources = append(cfg.Defaults.RuleSources, rs)
		}
	}
	if rawWatch := strings.TrimSpace(opts.RulesWatchInterval); rawWatch != "" {
		sec, err := ParseDurationSeconds(rawWatch, "rules-watch-interval")
		if err != nil {
			return Config{}, err
		}
		cfg.RulesWatchInterval = sec
	}
	if opts.DefaultRules != nil {
		b := *opts.DefaultRules
		baseWAF.DefaultRules = &b
		cfg.Defaults.DefaultRules = &b
	}

	if strings.TrimSpace(opts.VerifyLevel) != "" {
		lvl, prot, err := ParseVerifyLevel(opts.VerifyLevel)
		if err != nil {
			return Config{}, err
		}
		baseWAF.VerifyLevel = lvl
		defaultProtect = prot
	}
	if opts.Protect != nil {
		defaultProtect = *opts.Protect
	}

	// Build HTTP sites only when HTTP template is enabled.
	if useHTTP {
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
			wafCfg := cloneSiteWAF(baseWAF)
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
			if len(sp.RuleSources) > 0 {
				wafCfg.RuleSources = append(wafCfg.RuleSources, sp.RuleSources...)
			}
			var siteAuth *ForwardAuthConfig
			if opts.Authelia != nil {
				skipPortal := false
				if opts.Authelia.PortalURL != "" {
					if pu, pErr := url.Parse(opts.Authelia.PortalURL); pErr == nil && host.Normalize(pu.Hostname()) == d {
						skipPortal = true
					}
				}
				if !skipPortal {
					siteAuth = opts.Authelia.Clone()
				}
			}
			cfg.Sites = append(cfg.Sites, SiteConfig{
				Domain:    d,
				Upstreams: append([]string(nil), sp.Upstreams...),
				Auth:      siteAuth,
				Protect:   prot,
				WAF:       wafCfg,
				CertFile:  strings.TrimSpace(sp.CertFile),
				KeyFile:   strings.TrimSpace(sp.KeyFile),
			})
		}
	}
	if opts.Authelia != nil {
		cfg.Authelia = opts.Authelia.Clone()
	}

	if useSSH {
		sshCfg := DefaultSSHConfig()
		sshCfg.Enabled = true
		sshCfg.Protect = defaultProtect
		sshCfg.VerifyLevel = baseWAF.VerifyLevel
		sshCfg.Difficulty = baseWAF.Difficulty
		sshCfg.ClearanceTTL = baseWAF.ClearanceTTL
		if opts.SSH != nil {
			if opts.SSH.Port != 0 {
				sshCfg.Port = opts.SSH.Port
			}
			if opts.SSH.Upstream != "" {
				sshCfg.Upstream = opts.SSH.Upstream
			}
			if opts.SSH.VerifyLevel != "" {
				lvl, p, vErr := ParseVerifyLevel(opts.SSH.VerifyLevel)
				if vErr != nil {
					return Config{}, vErr
				}
				sshCfg.VerifyLevel = lvl
				sshCfg.Protect = p
			}
			if opts.SSH.Protect != nil {
				sshCfg.Protect = *opts.SSH.Protect
			}
			if opts.SSH.Difficulty != 0 {
				sshCfg.Difficulty = opts.SSH.Difficulty
			}
			if rawTTL := strings.TrimSpace(opts.SSH.ClearanceTTL); rawTTL != "" {
				ttl, tErr := ParseDurationSeconds(rawTTL, "verify-timeout")
				if tErr != nil {
					return Config{}, tErr
				}
				sshCfg.ClearanceTTL = ttl
			}
			if opts.SSH.VerifyURL != "" {
				sshCfg.VerifyURL = opts.SSH.VerifyURL
			}
			if opts.SSH.VerifyPort != 0 {
				sshCfg.VerifyPort = opts.SSH.VerifyPort
			}
			sshCfg.Rules = append(sshCfg.Rules, opts.SSH.Rules...)
		}
		if strings.TrimSpace(sshCfg.Upstream) == "" {
			sshCfg.Upstream = "127.0.0.1:2222"
		}
		cfg.SSH = &sshCfg
	}

	if useMC {
		mcCfg := DefaultMinecraftConfig()
		mcCfg.Enabled = true
		mcCfg.Protect = defaultProtect
		mcCfg.VerifyLevel = baseWAF.VerifyLevel
		mcCfg.Difficulty = baseWAF.Difficulty
		mcCfg.ClearanceTTL = baseWAF.ClearanceTTL
		if opts.Minecraft != nil {
			if opts.Minecraft.Port != 0 {
				mcCfg.Port = opts.Minecraft.Port
			}
			if opts.Minecraft.Upstream != "" {
				mcCfg.Upstream = opts.Minecraft.Upstream
			}
			if opts.Minecraft.ServerProperties != "" {
				mcCfg.ServerProperties = opts.Minecraft.ServerProperties
			}
			if opts.Minecraft.ServerJar != "" {
				mcCfg.ServerJar = opts.Minecraft.ServerJar
			}
			if opts.Minecraft.JavaPath != "" {
				mcCfg.JavaPath = opts.Minecraft.JavaPath
			}
			if len(opts.Minecraft.JVMArgs) > 0 {
				mcCfg.JVMArgs = append([]string(nil), opts.Minecraft.JVMArgs...)
			}
			if len(opts.Minecraft.ServerArgs) > 0 {
				mcCfg.ServerArgs = append([]string(nil), opts.Minecraft.ServerArgs...)
			}
			if opts.Minecraft.WorkDir != "" {
				mcCfg.WorkDir = opts.Minecraft.WorkDir
			}
			if opts.Minecraft.AutoRestart != nil {
				b := *opts.Minecraft.AutoRestart
				mcCfg.AutoRestart = &b
			}
			mcCfg.Console = opts.Minecraft.Console
			if opts.Minecraft.VerifyLevel != "" {
				lvl, p, vErr := ParseVerifyLevel(opts.Minecraft.VerifyLevel)
				if vErr != nil {
					return Config{}, vErr
				}
				mcCfg.VerifyLevel = lvl
				mcCfg.Protect = p
			}
			if opts.Minecraft.Protect != nil {
				mcCfg.Protect = *opts.Minecraft.Protect
			}
			if opts.Minecraft.Difficulty != 0 {
				mcCfg.Difficulty = opts.Minecraft.Difficulty
			}
			if rawTTL := strings.TrimSpace(opts.Minecraft.ClearanceTTL); rawTTL != "" {
				ttl, tErr := ParseDurationSeconds(rawTTL, "verify-timeout")
				if tErr != nil {
					return Config{}, tErr
				}
				mcCfg.ClearanceTTL = ttl
			}
			if opts.Minecraft.VerifyURL != "" {
				mcCfg.VerifyURL = opts.Minecraft.VerifyURL
			}
			if opts.Minecraft.VerifyPort != 0 {
				mcCfg.VerifyPort = opts.Minecraft.VerifyPort
			}
			mcCfg.Rules = append(mcCfg.Rules, opts.Minecraft.Rules...)
		}
		if strings.TrimSpace(mcCfg.Upstream) == "" {
			mcCfg.Upstream = "127.0.0.1:25566"
		}
		cfg.Minecraft = &mcCfg
	}

	cfg.Verify = opts.Verify

	// TLS & network settings
	if strings.TrimSpace(opts.SSLProvider) != "" {
		if err := applySSLProvider(opts.SSLProvider, &cfg.TLS); err != nil {
			return Config{}, err
		}
	}
	if opts.HTTPOnly {
		cfg.DirectHTTP = true
		cfg.TLS.Mode = "off"
		f := false
		cfg.TLS.AutoSSL = &f
		cfg.TLS.Provider = ""
	}
	if opts.AutoSSL != nil {
		b := *opts.AutoSSL
		cfg.TLS.AutoSSL = &b
	}
	if p := strings.TrimSpace(opts.AcmeShPath); p != "" {
		cfg.TLS.AcmeShPath = p
	}
	if cf := strings.TrimSpace(opts.CertFile); cf != "" {
		cfg.TLS.CertFile = cf
	}
	if kf := strings.TrimSpace(opts.KeyFile); kf != "" {
		cfg.TLS.KeyFile = kf
	}
	if strings.TrimSpace(opts.CertFile) != "" && strings.TrimSpace(opts.KeyFile) != "" && cfg.TLS.Mode == "auto" {
		cfg.TLS.Mode = "file"
	}
	if opts.DirectHTTP {
		cfg.DirectHTTP = true
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
		cfg.BindHost = DefaultBindHostForConfig(cfg)
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
		raw, err := MarshalConciseJSONConfig(cfg)
		if err != nil {
			return nil, Config{}, err
		}
		out := append(raw, '\n')
		if _, err := ParseConfigBytes(out, "gymbag.json", ""); err != nil {
			return nil, Config{}, fmt.Errorf("internal error: generated concise JSON failed validation: %w", err)
		}
		return out, cfg, nil
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
