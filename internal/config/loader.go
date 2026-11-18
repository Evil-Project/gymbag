package config

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"gymbag/internal/host"
)

// LoadOptions specifies CLI overrides applied on top of config file and environment variables.
type LoadOptions struct {
	ConfigPath  string
	Domain      string
	Upstream    string
	VerifyLevel string
	Protect     *bool
	AdminAddr   string
	HTTPPort    int
	HTTPSPort   int
}

// ParseFile reads a Gymbagfile, Caddyfile, or JSON configuration file from disk.
func ParseFile(path string) (Config, error) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return Config{}, fmt.Errorf("config file not found: %s", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read config file %s: %w", path, err)
	}
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasSuffix(strings.ToLower(path), ".json") || (strings.HasPrefix(trimmed, "{") && json.Valid(raw)) {
		cfg := Default()
		cfg.Sites = nil
		dec := json.NewDecoder(strings.NewReader(trimmed))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&cfg); err != nil {
			return Config{}, fmt.Errorf("invalid JSON config file %s: %w", path, err)
		}
		if err := cfg.Validate(); err != nil {
			return Config{}, err
		}
		return cfg, nil
	}
	return ParseCaddyfile(string(raw))
}

// Load resolves configuration from file -> environment variables -> CLI overrides.
func Load(opts LoadOptions) (Config, error) {
	return LoadWithEnv(opts, os.Getenv)
}

// LoadWithEnv resolves configuration using a custom environment lookup function (ideal for unit tests).
func LoadWithEnv(opts LoadOptions, getenv func(string) string) (Config, error) {
	var base *Config
	resolvedPath := strings.TrimSpace(opts.ConfigPath)
	if resolvedPath == "" {
		if envCfg := strings.TrimSpace(getenv("GYMBAG_CONFIG")); envCfg != "" {
			resolvedPath = envCfg
		} else {
			for _, candidate := range DefaultConfigFiles {
				if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
					resolvedPath = candidate
					break
				}
			}
		}
	}

	if resolvedPath != "" {
		parsed, err := ParseFile(resolvedPath)
		if err != nil {
			return Config{}, err
		}
		base = &parsed
	}

	cfg, err := applyEnv(base, getenv)
	if err != nil {
		return Config{}, err
	}

	// Apply CLI flag overrides
	if strings.TrimSpace(opts.Domain) != "" || strings.TrimSpace(opts.Upstream) != "" {
		domains := cfg.Domains()
		if strings.TrimSpace(opts.Domain) != "" {
			domains, err = splitDomains(opts.Domain)
			if err != nil {
				return Config{}, err
			}
		}
		upstream := cfg.Sites[0].PrimaryUpstream()
		if strings.TrimSpace(opts.Upstream) != "" {
			upstream, err = NormalizeUpstream(opts.Upstream, "upstream")
			if err != nil {
				return Config{}, err
			}
		}
		sites := make([]SiteConfig, 0, len(domains))
		for _, d := range domains {
			sites = append(sites, SiteConfig{
				Domain:    d,
				Upstreams: []string{upstream},
				Protect:   true,
				WAF:       cloneSiteWAF(cfg.Defaults),
			})
		}
		cfg.Sites = sites
		if strings.TrimSpace(getenv("GYMBAG_BIND")) == "" {
			cfg.BindHost = DefaultBindHost(domains)
		}
	}

	if strings.TrimSpace(opts.VerifyLevel) != "" {
		lvl, protect, err := ParseVerifyLevel(opts.VerifyLevel)
		if err != nil {
			return Config{}, err
		}
		cfg.Defaults.VerifyLevel = lvl
		for i := range cfg.Sites {
			cfg.Sites[i].WAF.VerifyLevel = lvl
			if !protect {
				cfg.Sites[i].Protect = false
			}
		}
	}

	if opts.Protect != nil {
		for i := range cfg.Sites {
			cfg.Sites[i].Protect = *opts.Protect
		}
	}

	if strings.TrimSpace(opts.AdminAddr) != "" {
		if strings.EqualFold(opts.AdminAddr, "off") {
			cfg.AdminAddr = ""
		} else {
			cfg.AdminAddr = strings.TrimSpace(opts.AdminAddr)
		}
	}
	if opts.HTTPPort > 0 {
		cfg.HTTPPort = opts.HTTPPort
	}
	if opts.HTTPSPort > 0 {
		cfg.HTTPSPort = opts.HTTPSPort
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func splitDomains(raw string) ([]string, error) {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		d := host.Normalize(part)
		if d == "" {
			return nil, fmt.Errorf("at least one domain is required")
		}
		out = append(out, d)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one domain is required")
	}
	return out, nil
}

func applyEnv(base *Config, getenv func(string) string) (Config, error) {
	hasBase := base != nil
	var cfg Config
	if base != nil {
		cfg = *base
		cfg.Sites = slices.Clone(base.Sites)
	} else {
		cfg = Default()
	}

	baseDomains := cfg.Domains()
	domains := baseDomains
	if rawList := strings.TrimSpace(getenv("GYMBAG_DOMAINS")); rawList != "" {
		parsed, err := splitDomains(rawList)
		if err != nil {
			return Config{}, err
		}
		domains = parsed
	} else if rawSingle := strings.TrimSpace(getenv("GYMBAG_DOMAIN")); rawSingle != "" {
		parsed, err := splitDomains(rawSingle)
		if err != nil {
			return Config{}, err
		}
		domains = parsed
	}

	defaultUpstream := cfg.Sites[0].PrimaryUpstream()
	if rawUp := strings.TrimSpace(getenv("GYMBAG_UPSTREAM")); rawUp != "" {
		norm, err := NormalizeUpstream(rawUp, "GYMBAG_UPSTREAM")
		if err != nil {
			return Config{}, err
		}
		defaultUpstream = norm
	} else {
		if err := RequireHTTPURL(defaultUpstream, "GYMBAG_UPSTREAM"); err != nil {
			return Config{}, err
		}
	}

	// Parse GYMBAG_UPSTREAMS="host1=url1;host2=url2"
	envRoutes := make(map[string]string)
	var envRouteOrder []string
	if rawUpstreams := strings.TrimSpace(getenv("GYMBAG_UPSTREAMS")); rawUpstreams != "" {
		for _, item := range strings.Split(rawUpstreams, ";") {
			entry := strings.TrimSpace(item)
			if entry == "" {
				continue
			}
			lhs, rhs, ok := strings.Cut(entry, "=")
			if !ok || strings.TrimSpace(lhs) == "" || strings.TrimSpace(rhs) == "" {
				return Config{}, fmt.Errorf("invalid GYMBAG_UPSTREAMS entry: %s", entry)
			}
			normTarget, err := NormalizeUpstream(rhs, "GYMBAG_UPSTREAMS")
			if err != nil {
				return Config{}, err
			}
			h := host.FromHeader(lhs)
			if _, exists := envRoutes[h]; !exists {
				envRouteOrder = append(envRouteOrder, h)
			}
			envRoutes[h] = normTarget
		}
	}

	// Rebuild sites if domains or upstreams env vars were explicitly supplied
	if !slices.Equal(domains, baseDomains) || strings.TrimSpace(getenv("GYMBAG_UPSTREAM")) != "" || len(envRoutes) > 0 {
		existingByDomain := make(map[string]SiteConfig, len(cfg.Sites))
		for _, s := range cfg.Sites {
			existingByDomain[s.Domain] = s
		}
		mergedDomains := slices.Clone(domains)
		for _, h := range envRouteOrder {
			if !slices.Contains(mergedDomains, h) {
				mergedDomains = append(mergedDomains, h)
			}
		}
		rebuilt := make([]SiteConfig, 0, len(mergedDomains))
		for _, d := range mergedDomains {
			if existing, ok := existingByDomain[d]; ok {
				if target, hasOverride := envRoutes[d]; hasOverride {
					existing.Upstreams = []string{target}
				} else if strings.TrimSpace(getenv("GYMBAG_UPSTREAM")) != "" {
					existing.Upstreams = []string{defaultUpstream}
				}
				rebuilt = append(rebuilt, existing)
			} else {
				target := defaultUpstream
				if override, ok := envRoutes[d]; ok {
					target = override
				}
				rebuilt = append(rebuilt, SiteConfig{
					Domain:    d,
					Upstreams: []string{target},
					Protect:   true,
					WAF:       cloneSiteWAF(cfg.Defaults),
				})
			}
		}
		cfg.Sites = rebuilt
	}

	if rawProv := strings.TrimSpace(getenv("GYMBAG_SSL_PROVIDER")); rawProv != "" {
		if err := applySSLProvider(rawProv, &cfg.TLS); err != nil {
			return Config{}, err
		}
	}

	if rawMode := strings.TrimSpace(getenv("GYMBAG_TLS_MODE")); rawMode != "" {
		m := strings.ToLower(rawMode)
		if _, ok := ValidTLSModes[m]; !ok {
			return Config{}, fmt.Errorf("GYMBAG_TLS_MODE must be auto, acme, local, selfsigned, or file")
		}
		cfg.TLS.Mode = m
	}

	// WAF env overrides
	rawVerify := firstNonEmpty(
		getenv("GYMBAG_VERIFY_LEVEL"),
		getenv("GYMBAG_VERIFY"),
		getenv("GYMBAG_VERIFY_MODE"),
		getenv("GYMBAG_MODE"),
		getenv("GYMBAG_TRIGGER"),
	)
	if rawVerify != "" {
		if strings.EqualFold(rawVerify, "on") || strings.EqualFold(rawVerify, "true") {
			cfg.Defaults.VerifyLevel = VerifyLevelSuspiciousAndAI
			for i := range cfg.Sites {
				cfg.Sites[i].WAF.VerifyLevel = VerifyLevelSuspiciousAndAI
				cfg.Sites[i].Protect = true
			}
		} else {
			lvl, protectEnabled, err := ParseVerifyLevel(rawVerify)
			if err != nil {
				return Config{}, err
			}
			cfg.Defaults.VerifyLevel = lvl
			for i := range cfg.Sites {
				cfg.Sites[i].WAF.VerifyLevel = lvl
				if !protectEnabled {
					cfg.Sites[i].Protect = false
				}
			}
		}
	}

	if rawDefRules := strings.TrimSpace(getenv("GYMBAG_DEFAULT_RULES")); rawDefRules != "" {
		b, err := parseBool(rawDefRules, "GYMBAG_DEFAULT_RULES")
		if err != nil {
			return Config{}, err
		}
		cfg.Defaults.DefaultRules = &b
		for i := range cfg.Sites {
			v := b
			cfg.Sites[i].WAF.DefaultRules = &v
		}
	}

	if rawProtect := strings.TrimSpace(getenv("GYMBAG_PROTECT")); rawProtect != "" {
		b, err := parseBool(rawProtect, "GYMBAG_PROTECT")
		if err != nil {
			return Config{}, err
		}
		for i := range cfg.Sites {
			cfg.Sites[i].Protect = b
		}
	}

	if rawProtectDomains := strings.TrimSpace(getenv("GYMBAG_PROTECT_DOMAINS")); rawProtectDomains != "" {
		protectedList, err := splitDomains(rawProtectDomains)
		if err != nil {
			return Config{}, err
		}
		protectedSet := make(map[string]struct{}, len(protectedList))
		for _, d := range protectedList {
			protectedSet[d] = struct{}{}
		}
		for i := range cfg.Sites {
			_, cfg.Sites[i].Protect = protectedSet[cfg.Sites[i].Domain]
		}
	}

	if v, ok, err := envPositiveInt(getenv, "GYMBAG_DIFFICULTY"); err != nil {
		return Config{}, err
	} else if ok {
		if v < 1 || v > 5 {
			return Config{}, fmt.Errorf("GYMBAG_DIFFICULTY must be from 1 to 5")
		}
		cfg.Defaults.Difficulty = v
		for i := range cfg.Sites {
			cfg.Sites[i].WAF.Difficulty = v
		}
	}

	if v, ok, err := envPositiveInt(getenv, "GYMBAG_CHALLENGE_TTL"); err != nil {
		return Config{}, err
	} else if ok {
		cfg.Defaults.ChallengeTTL = v
		for i := range cfg.Sites {
			cfg.Sites[i].WAF.ChallengeTTL = v
		}
	}

	if v, ok, err := envPositiveInt(getenv, "GYMBAG_CLEARANCE_TTL"); err != nil {
		return Config{}, err
	} else if ok {
		cfg.Defaults.ClearanceTTL = v
		for i := range cfg.Sites {
			cfg.Sites[i].WAF.ClearanceTTL = v
		}
	}

	if rawCookie := strings.TrimSpace(getenv("GYMBAG_COOKIE_NAME")); rawCookie != "" {
		cfg.Defaults.CookieName = rawCookie
		for i := range cfg.Sites {
			cfg.Sites[i].WAF.CookieName = rawCookie
		}
	}

	if rawSecure := strings.TrimSpace(getenv("GYMBAG_COOKIE_SECURE")); rawSecure != "" {
		b, err := parseBool(rawSecure, "GYMBAG_COOKIE_SECURE")
		if err != nil {
			return Config{}, err
		}
		cfg.Defaults.CookieSecure = b
		for i := range cfg.Sites {
			cfg.Sites[i].WAF.CookieSecure = b
		}
	}

	if v, ok, err := envPositiveInt(getenv, "GYMBAG_RATE_LIMIT"); err != nil {
		return Config{}, err
	} else if ok {
		cfg.Defaults.RateLimit = v
		for i := range cfg.Sites {
			cfg.Sites[i].WAF.RateLimit = v
		}
	}

	if v, ok, err := envPositiveInt(getenv, "GYMBAG_RATE_WINDOW"); err != nil {
		return Config{}, err
	} else if ok {
		cfg.Defaults.RateWindow = v
		for i := range cfg.Sites {
			cfg.Sites[i].WAF.RateWindow = v
		}
	}

	if v, ok, err := envPort(getenv, "GYMBAG_HTTP_PORT"); err != nil {
		return Config{}, err
	} else if ok {
		cfg.HTTPPort = v
	}

	if v, ok, err := envPort(getenv, "GYMBAG_HTTPS_PORT"); err != nil {
		return Config{}, err
	} else if ok {
		cfg.HTTPSPort = v
	}

	if rawCert := strings.TrimSpace(getenv("GYMBAG_CERT_FILE")); rawCert != "" {
		cfg.TLS.CertFile = rawCert
	}
	if rawKey := strings.TrimSpace(getenv("GYMBAG_KEY_FILE")); rawKey != "" {
		cfg.TLS.KeyFile = rawKey
	}
	if rawPwd := getenv("GYMBAG_KEY_PASSWORD"); rawPwd != "" {
		cfg.TLS.KeyPassword = rawPwd
	}
	if rawAcctKey := strings.TrimSpace(getenv("GYMBAG_ACCOUNT_KEY_FILE")); rawAcctKey != "" {
		cfg.TLS.AccountKeyFile = rawAcctKey
	}
	if rawEABKID := strings.TrimSpace(getenv("GYMBAG_EAB_KID")); rawEABKID != "" {
		cfg.TLS.EABKID = rawEABKID
	}
	if rawEABHMAC := strings.TrimSpace(getenv("GYMBAG_EAB_HMAC_KEY")); rawEABHMAC != "" {
		cfg.TLS.EABHMACKey = rawEABHMAC
	}
	if rawAutoHosts := strings.TrimSpace(getenv("GYMBAG_AUTO_HOSTS")); rawAutoHosts != "" {
		b, err := parseBool(rawAutoHosts, "GYMBAG_AUTO_HOSTS")
		if err != nil {
			return Config{}, err
		}
		cfg.TLS.AutoHosts = &b
	}
	if rawDir := strings.TrimSpace(getenv("GYMBAG_CERT_DIR")); rawDir != "" {
		cfg.TLS.CertDir = rawDir
	}
	if rawEmail := strings.TrimSpace(getenv("GYMBAG_ACME_EMAIL")); rawEmail != "" {
		cfg.TLS.ACMEEmail = rawEmail
	}
	if rawAcmeDir := strings.TrimSpace(getenv("GYMBAG_ACME_DIRECTORY")); rawAcmeDir != "" {
		cfg.TLS.ACMEDirectory = rawAcmeDir
	}
	if rawRedis := strings.TrimSpace(getenv("GYMBAG_REDIS_URL")); rawRedis != "" {
		cfg.RedisURL = rawRedis
	}
	if rawSecret := getenv("GYMBAG_SECRET"); rawSecret != "" {
		cfg.Secret = rawSecret
	}
	if rawAdmin := strings.TrimSpace(getenv("GYMBAG_ADMIN")); rawAdmin != "" {
		if strings.EqualFold(rawAdmin, "off") {
			cfg.AdminAddr = ""
		} else {
			cfg.AdminAddr = rawAdmin
		}
	}
	if v, ok, err := envPositiveInt(getenv, "GYMBAG_RENEW_INTERVAL"); err != nil {
		return Config{}, err
	} else if ok {
		cfg.TLS.RenewInterval = v
	}
	if v, ok, err := envPositiveInt(getenv, "GYMBAG_RENEW_WITHIN_DAYS"); err != nil {
		return Config{}, err
	} else if ok {
		cfg.TLS.RenewWithinDays = v
	}
	if rawHSTS := strings.TrimSpace(getenv("GYMBAG_HSTS")); rawHSTS != "" {
		b, err := parseBool(rawHSTS, "GYMBAG_HSTS")
		if err != nil {
			return Config{}, err
		}
		cfg.TLS.HSTS = &b
	}

	if rawIPSec := strings.TrimSpace(getenv("GYMBAG_IP_SECURITY")); rawIPSec != "" {
		b, err := parseBool(rawIPSec, "GYMBAG_IP_SECURITY")
		if err != nil {
			return Config{}, err
		}
		cfg.Intel.IPSecurity = b
	}
	if rawAct := strings.TrimSpace(getenv("GYMBAG_IP_SECURITY_ACTION")); rawAct != "" {
		cfg.Intel.IPSecurityAction = rawAct
	}
	if rawRep := strings.TrimSpace(getenv("GYMBAG_IP_REPUTATION")); rawRep != "" {
		b, err := parseBool(rawRep, "GYMBAG_IP_REPUTATION")
		if err != nil {
			return Config{}, err
		}
		cfg.Intel.LiveReputation = b
	}
	if rawAIList := strings.TrimSpace(getenv("GYMBAG_AI_IP_LIST")); rawAIList != "" {
		b, err := parseBool(rawAIList, "GYMBAG_AI_IP_LIST")
		if err != nil {
			return Config{}, err
		}
		cfg.Intel.AIIPList = b
	}
	if v, ok, err := envPositiveInt(getenv, "GYMBAG_INTEL_REFRESH"); err != nil {
		return Config{}, err
	} else if ok {
		cfg.Intel.RefreshInterval = v
	}

	rawBind := strings.TrimSpace(getenv("GYMBAG_BIND"))
	if rawBind != "" {
		cfg.BindHost = rawBind
	} else if !hasBase || !slices.Equal(cfg.Domains(), baseDomains) {
		cfg.BindHost = DefaultBindHost(cfg.Domains())
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func envPositiveInt(getenv func(string) string, name string) (int, bool, error) {
	raw := strings.TrimSpace(getenv(name))
	if raw == "" {
		return 0, false, nil
	}
	v, err := parsePositiveInt(raw, name)
	if err != nil {
		return 0, false, err
	}
	return v, true, nil
}

func envPort(getenv func(string) string, name string) (int, bool, error) {
	raw := strings.TrimSpace(getenv(name))
	if raw == "" {
		return 0, false, nil
	}
	v, err := host.ParsePort(raw, name)
	if err != nil {
		return 0, false, err
	}
	return v, true, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
