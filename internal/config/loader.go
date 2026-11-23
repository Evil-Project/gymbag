package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gymbag/internal/host"
)

// LoadOptions specifies CLI overrides applied on top of config file and environment variables.
type LoadOptions struct {
	ConfigPath         string
	Domain             string
	Upstream           string
	VerifyLevel        string
	ClearanceTTL       string
	RemoteRules        []string
	RemoteRulesRefresh string
	Protect            *bool
	AdminAddr          string
	HTTPPort           int
	HTTPSPort          int
}

// ParseFile reads a Gymbagfile, Caddyfile, nginx, Apache, or JSON configuration file from disk.
func ParseFile(path string) (Config, error) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return Config{}, fmt.Errorf("config file not found: %s", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read config file %s: %w", path, err)
	}
	baseDir := filepath.Dir(path)
	if abs, absErr := filepath.Abs(path); absErr == nil {
		baseDir = filepath.Dir(abs)
	}
	return ParseConfigBytes(raw, path, baseDir)
}

// ParseConfigBytes parses raw configuration bytes according to file extension and content sniffing
// (JSON, Apache httpd.conf, nginx.conf, or Gymbagfile/Caddyfile), resolving relative includes from baseDir.
func ParseConfigBytes(raw []byte, path, baseDir string) (Config, error) {
	trimmed := strings.TrimSpace(string(raw))
	var (
		cfg Config
		err error
	)
	switch {
	case strings.HasSuffix(strings.ToLower(path), ".json") || (strings.HasPrefix(trimmed, "{") && json.Valid(raw)):
		cfg = Default()
		cfg.Sites = nil
		cfg.BindHost = ""
		dec := json.NewDecoder(strings.NewReader(trimmed))
		dec.DisallowUnknownFields()
		if err = dec.Decode(&cfg); err != nil {
			return Config{}, fmt.Errorf("invalid JSON config file %s: %w", path, err)
		}
		if err = cfg.Validate(); err != nil {
			return Config{}, err
		}
	case isApacheConfig(string(raw), path):
		cfg, err = ParseApacheWithBaseDir(string(raw), baseDir)
	case isNginxConfig(string(raw), path):
		cfg, err = ParseNginxWithBaseDir(string(raw), baseDir)
	default:
		cfg, err = ParseCaddyfileWithBaseDir(string(raw), baseDir)
	}
	if err != nil {
		return Config{}, err
	}
	cfg.BaseDir = baseDir
	return cfg, nil
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
		var upstream string
		if len(cfg.Sites) > 0 {
			upstream = cfg.Sites[0].PrimaryUpstream()
		}
		if strings.TrimSpace(opts.Upstream) != "" {
			upstream, err = NormalizeUpstream(opts.Upstream, "upstream")
			if err != nil {
				return Config{}, err
			}
		}
		sites := make([]SiteConfig, 0, len(domains))
		var baseRoutes []RouteConfig
		var baseHeaders map[string]string
		var baseAuth *ForwardAuthConfig
		if len(cfg.Sites) > 0 {
			baseAuth = cfg.Sites[0].Auth
			if upstream == "" && len(cfg.Sites[0].Routes) > 0 {
				baseRoutes = cfg.Sites[0].Routes
				baseHeaders = cfg.Sites[0].Headers
			}
		}
		for _, d := range domains {
			sc := SiteConfig{
				Domain:  d,
				Protect: true,
				WAF:     cloneSiteWAF(cfg.Defaults),
				Headers: cloneHeaderMap(baseHeaders),
				Auth:    baseAuth.Clone(),
			}
			if upstream != "" {
				sc.Upstreams = []string{upstream}
			}
			if len(baseRoutes) > 0 {
				clonedRoutes := make([]RouteConfig, len(baseRoutes))
				for i, r := range baseRoutes {
					clonedRoutes[i] = r.Clone()
				}
				sc.Routes = clonedRoutes
			}
			sites = append(sites, sc)
		}
		cfg.Sites = sites
		if strings.TrimSpace(getenv("GYMBAG_BIND")) == "" {
			cfg.BindHost = DefaultBindHostForConfig(cfg)
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
		if cfg.SSH != nil {
			cfg.SSH.VerifyLevel = lvl
			if !protect {
				cfg.SSH.Protect = false
			}
		}
		if cfg.Minecraft != nil {
			cfg.Minecraft.VerifyLevel = lvl
			if !protect {
				cfg.Minecraft.Protect = false
			}
		}
	}

	if opts.Protect != nil {
		for i := range cfg.Sites {
			cfg.Sites[i].Protect = *opts.Protect
		}
		if cfg.SSH != nil {
			cfg.SSH.Protect = *opts.Protect
		}
		if cfg.Minecraft != nil {
			cfg.Minecraft.Protect = *opts.Protect
		}
	}

	if rawTTL := strings.TrimSpace(opts.ClearanceTTL); rawTTL != "" {
		ttl, err := ParseDurationSeconds(rawTTL, "verify-timeout")
		if err != nil {
			return Config{}, err
		}
		cfg.Defaults.ClearanceTTL = ttl
		for i := range cfg.Sites {
			cfg.Sites[i].WAF.ClearanceTTL = ttl
		}
		if cfg.SSH != nil {
			cfg.SSH.ClearanceTTL = ttl
		}
		if cfg.Minecraft != nil {
			cfg.Minecraft.ClearanceTTL = ttl
		}
	}

	if len(opts.RemoteRules) > 0 {
		urls, err := splitRemoteRuleURLs(opts.RemoteRules)
		if err != nil {
			return Config{}, err
		}
		for _, u := range urls {
			if !slices.Contains(cfg.Defaults.RemoteRules, u) {
				cfg.Defaults.RemoteRules = append(cfg.Defaults.RemoteRules, u)
			}
			for i := range cfg.Sites {
				if !slices.Contains(cfg.Sites[i].WAF.RemoteRules, u) {
					cfg.Sites[i].WAF.RemoteRules = append(cfg.Sites[i].WAF.RemoteRules, u)
				}
			}
		}
	}

	if rawRefresh := strings.TrimSpace(opts.RemoteRulesRefresh); rawRefresh != "" {
		sec, err := ParseDurationSeconds(rawRefresh, "remote-rules-refresh")
		if err != nil {
			return Config{}, err
		}
		cfg.RemoteRulesRefresh = sec
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
		if base.SSH != nil {
			cfg.SSH = base.SSH.Clone()
		}
		if base.Minecraft != nil {
			cfg.Minecraft = base.Minecraft.Clone()
		}
		if base.Authelia != nil {
			cfg.Authelia = base.Authelia.Clone()
		}
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

	var defaultUpstream string
	if len(cfg.Sites) > 0 {
		defaultUpstream = cfg.Sites[0].PrimaryUpstream()
	}
	if rawUp := strings.TrimSpace(getenv("GYMBAG_UPSTREAM")); rawUp != "" {
		norm, err := NormalizeUpstream(rawUp, "GYMBAG_UPSTREAM")
		if err != nil {
			return Config{}, err
		}
		defaultUpstream = norm
	} else if defaultUpstream != "" {
		if err := RequireHTTPURL(defaultUpstream, "GYMBAG_UPSTREAM"); err != nil {
			return Config{}, err
		}
	} else {
		defaultUpstream = "http://localhost:5173"
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

	if v, ok, err := envDurationSeconds(getenv, "GYMBAG_CHALLENGE_TTL"); err != nil {
		return Config{}, err
	} else if ok {
		cfg.Defaults.ChallengeTTL = v
		for i := range cfg.Sites {
			cfg.Sites[i].WAF.ChallengeTTL = v
		}
	}

	rawClearance := firstNonEmpty(
		getenv("GYMBAG_CLEARANCE_TTL"),
		getenv("GYMBAG_VERIFY_TIMEOUT"),
		getenv("GYMBAG_VERIFIED_TIMEOUT"),
	)
	if rawClearance != "" {
		v, err := ParseDurationSeconds(rawClearance, "GYMBAG_CLEARANCE_TTL")
		if err != nil {
			return Config{}, err
		}
		cfg.Defaults.ClearanceTTL = v
		for i := range cfg.Sites {
			cfg.Sites[i].WAF.ClearanceTTL = v
		}
	}

	if rawRemote := strings.TrimSpace(getenv("GYMBAG_REMOTE_RULES")); rawRemote != "" {
		urls, err := splitRemoteRuleURLs([]string{rawRemote})
		if err != nil {
			return Config{}, err
		}
		for _, u := range urls {
			if !slices.Contains(cfg.Defaults.RemoteRules, u) {
				cfg.Defaults.RemoteRules = append(cfg.Defaults.RemoteRules, u)
			}
			for i := range cfg.Sites {
				if !slices.Contains(cfg.Sites[i].WAF.RemoteRules, u) {
					cfg.Sites[i].WAF.RemoteRules = append(cfg.Sites[i].WAF.RemoteRules, u)
				}
			}
		}
	}

	if v, ok, err := envDurationSeconds(getenv, "GYMBAG_REMOTE_RULES_REFRESH"); err != nil {
		return Config{}, err
	} else if ok {
		cfg.RemoteRulesRefresh = v
	}

	if v, ok, err := envDurationSeconds(getenv, "GYMBAG_RULES_WATCH_INTERVAL"); err != nil {
		return Config{}, err
	} else if ok {
		cfg.RulesWatchInterval = v
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

	if v, ok, err := envDurationSeconds(getenv, "GYMBAG_RATE_WINDOW"); err != nil {
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
	if v, ok, err := envDurationSeconds(getenv, "GYMBAG_RENEW_INTERVAL"); err != nil {
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
	if v, ok, err := envDurationSeconds(getenv, "GYMBAG_INTEL_REFRESH"); err != nil {
		return Config{}, err
	} else if ok {
		cfg.Intel.RefreshInterval = v
	}
	if rawVPN := strings.TrimSpace(getenv("GYMBAG_VPN_DETECTION")); rawVPN != "" {
		b, err := parseBool(rawVPN, "GYMBAG_VPN_DETECTION")
		if err != nil {
			return Config{}, err
		}
		cfg.Intel.VPNDetection = b
	}

	if rawVURL := strings.TrimSpace(getenv("GYMBAG_VERIFY_URL")); rawVURL != "" {
		cfg.Verify.URL = rawVURL
	}
	if v, ok, err := envPort(getenv, "GYMBAG_VERIFY_PORT"); err != nil {
		return Config{}, err
	} else if ok {
		cfg.Verify.Port = v
	}

	sshEnvSet, err := applySSHEnv(&cfg, getenv)
	if err != nil {
		return Config{}, err
	}
	mcEnvSet, err := applyMinecraftEnv(&cfg, getenv)
	if err != nil {
		return Config{}, err
	}
	httpEnvSet := strings.TrimSpace(getenv("GYMBAG_DOMAIN")) != "" ||
		strings.TrimSpace(getenv("GYMBAG_DOMAINS")) != "" ||
		strings.TrimSpace(getenv("GYMBAG_UPSTREAM")) != "" ||
		strings.TrimSpace(getenv("GYMBAG_UPSTREAMS")) != ""
	if !hasBase && !httpEnvSet && (sshEnvSet || mcEnvSet) {
		cfg.Sites = nil
	}

	if err := applyAutheliaEnv(&cfg, getenv); err != nil {
		return Config{}, err
	}

	rawBind := strings.TrimSpace(getenv("GYMBAG_BIND"))
	if rawBind != "" {
		cfg.BindHost = rawBind
	} else if !hasBase || !slices.Equal(cfg.Domains(), baseDomains) {
		cfg.BindHost = DefaultBindHostForConfig(cfg)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applySSHEnv(cfg *Config, getenv func(string) string) (bool, error) {
	rawEnabled := strings.TrimSpace(getenv("GYMBAG_SSH_ENABLED"))
	rawPort := strings.TrimSpace(getenv("GYMBAG_SSH_PORT"))
	rawUp := strings.TrimSpace(getenv("GYMBAG_SSH_UPSTREAM"))
	rawVerify := strings.TrimSpace(getenv("GYMBAG_SSH_VERIFY_LEVEL"))
	rawProtect := strings.TrimSpace(getenv("GYMBAG_SSH_PROTECT"))
	if rawEnabled == "" && rawPort == "" && rawUp == "" && rawVerify == "" && rawProtect == "" {
		return false, nil
	}
	if cfg.SSH == nil {
		def := DefaultSSHConfig()
		cfg.SSH = &def
	}
	if rawEnabled != "" {
		b, err := parseBool(rawEnabled, "GYMBAG_SSH_ENABLED")
		if err != nil {
			return false, err
		}
		cfg.SSH.Enabled = b
	} else if rawUp != "" || rawPort != "" {
		cfg.SSH.Enabled = true
	}
	if v, ok, err := envPort(getenv, "GYMBAG_SSH_PORT"); err != nil {
		return false, err
	} else if ok {
		cfg.SSH.Port = v
	}
	if rawUp != "" {
		norm, err := NormalizeTCPUpstream(rawUp, "GYMBAG_SSH_UPSTREAM")
		if err != nil {
			return false, err
		}
		cfg.SSH.Upstream = norm
	}
	if rawVerify != "" {
		lvl, prot, err := ParseVerifyLevel(rawVerify)
		if err != nil {
			return false, err
		}
		cfg.SSH.VerifyLevel = lvl
		if !prot {
			cfg.SSH.Protect = false
		}
	}
	if rawProtect != "" {
		b, err := parseBool(rawProtect, "GYMBAG_SSH_PROTECT")
		if err != nil {
			return false, err
		}
		cfg.SSH.Protect = b
	}
	return cfg.SSH.Enabled, nil
}

func applyMinecraftEnv(cfg *Config, getenv func(string) string) (bool, error) {
	rawEnabled := firstNonEmpty(getenv("GYMBAG_MC_ENABLED"), getenv("GYMBAG_MINECRAFT_ENABLED"))
	rawPort := firstNonEmpty(getenv("GYMBAG_MC_PORT"), getenv("GYMBAG_MINECRAFT_PORT"))
	rawUp := firstNonEmpty(getenv("GYMBAG_MC_UPSTREAM"), getenv("GYMBAG_MINECRAFT_UPSTREAM"))
	rawJar := firstNonEmpty(getenv("GYMBAG_MC_SERVER_JAR"), getenv("GYMBAG_MINECRAFT_SERVER_JAR"), getenv("GYMBAG_MC_JAR"))
	rawJava := firstNonEmpty(getenv("GYMBAG_MC_JAVA"), getenv("GYMBAG_MINECRAFT_JAVA"), getenv("GYMBAG_MC_JAVA_PATH"))
	rawJVM := firstNonEmpty(getenv("GYMBAG_MC_JVM_ARGS"), getenv("GYMBAG_MINECRAFT_JVM_ARGS"))
	rawWorkDir := firstNonEmpty(getenv("GYMBAG_MC_WORK_DIR"), getenv("GYMBAG_MINECRAFT_WORK_DIR"))
	rawAutoRestart := firstNonEmpty(getenv("GYMBAG_MC_AUTO_RESTART"), getenv("GYMBAG_MINECRAFT_AUTO_RESTART"))
	rawVerify := firstNonEmpty(getenv("GYMBAG_MC_VERIFY_LEVEL"), getenv("GYMBAG_MINECRAFT_VERIFY_LEVEL"))
	rawProtect := firstNonEmpty(getenv("GYMBAG_MC_PROTECT"), getenv("GYMBAG_MINECRAFT_PROTECT"))
	if rawEnabled == "" && rawPort == "" && rawUp == "" && rawJar == "" && rawJava == "" &&
		rawJVM == "" && rawWorkDir == "" && rawAutoRestart == "" && rawVerify == "" && rawProtect == "" {
		return false, nil
	}
	if cfg.Minecraft == nil {
		def := DefaultMinecraftConfig()
		cfg.Minecraft = &def
	}
	if rawEnabled != "" {
		b, err := parseBool(rawEnabled, "GYMBAG_MC_ENABLED")
		if err != nil {
			return false, err
		}
		cfg.Minecraft.Enabled = b
	} else if rawUp != "" || rawPort != "" || rawJar != "" {
		cfg.Minecraft.Enabled = true
	}
	if rawPort != "" {
		p, err := host.ParsePort(rawPort, "GYMBAG_MC_PORT")
		if err != nil {
			return false, err
		}
		cfg.Minecraft.Port = p
	}
	if rawUp != "" {
		norm, err := NormalizeTCPUpstream(rawUp, "GYMBAG_MC_UPSTREAM")
		if err != nil {
			return false, err
		}
		cfg.Minecraft.Upstream = norm
	}
	if rawJar != "" {
		cfg.Minecraft.ServerJar = rawJar
		if cfg.Minecraft.Upstream == "" {
			cfg.Minecraft.Upstream = "127.0.0.1:25566"
		}
	}
	if rawJava != "" {
		cfg.Minecraft.JavaPath = rawJava
	}
	if rawJVM != "" {
		cfg.Minecraft.JVMArgs = strings.Fields(rawJVM)
	}
	if rawWorkDir != "" {
		cfg.Minecraft.WorkDir = rawWorkDir
	}
	if rawAutoRestart != "" {
		b, err := parseBool(rawAutoRestart, "GYMBAG_MC_AUTO_RESTART")
		if err != nil {
			return false, err
		}
		cfg.Minecraft.AutoRestart = &b
	}
	if rawVerify != "" {
		lvl, prot, err := ParseVerifyLevel(rawVerify)
		if err != nil {
			return false, err
		}
		cfg.Minecraft.VerifyLevel = lvl
		if !prot {
			cfg.Minecraft.Protect = false
		}
	}
	if rawProtect != "" {
		b, err := parseBool(rawProtect, "GYMBAG_MC_PROTECT")
		if err != nil {
			return false, err
		}
		cfg.Minecraft.Protect = b
	}
	return cfg.Minecraft.Enabled, nil
}

func applyAutheliaEnv(cfg *Config, getenv func(string) string) error {
	rawURL := firstNonEmpty(getenv("GYMBAG_AUTHELIA_URL"), getenv("GYMBAG_FORWARD_AUTH_URL"))
	rawURI := strings.TrimSpace(getenv("GYMBAG_AUTHELIA_URI"))
	rawCopy := strings.TrimSpace(getenv("GYMBAG_AUTHELIA_COPY_HEADERS"))
	rawPortal := strings.TrimSpace(getenv("GYMBAG_AUTHELIA_PORTAL_URL"))
	rawTimeout := strings.TrimSpace(getenv("GYMBAG_AUTHELIA_TIMEOUT"))
	rawInsecure := strings.TrimSpace(getenv("GYMBAG_AUTHELIA_INSECURE_TLS"))
	rawDomains := strings.TrimSpace(getenv("GYMBAG_AUTHELIA_DOMAINS"))

	if rawURL == "" && rawURI == "" && rawCopy == "" && rawPortal == "" &&
		rawTimeout == "" && rawInsecure == "" && rawDomains == "" {
		return nil
	}

	var authCfg *ForwardAuthConfig
	if cfg.Authelia != nil {
		authCfg = cfg.Authelia.Clone()
	} else {
		authCfg = &ForwardAuthConfig{}
	}
	if rawURL != "" {
		authCfg.URL = rawURL
		if strings.TrimSpace(getenv("GYMBAG_AUTHELIA_URL")) != "" {
			authCfg.Provider = "authelia"
		}
	}
	if authCfg.URL == "" {
		return fmt.Errorf("GYMBAG_AUTHELIA_URL is required when configuring Authelia via environment variables")
	}
	if rawURI != "" {
		authCfg.URI = rawURI
	}
	if rawCopy != "" {
		fields := strings.FieldsFunc(rawCopy, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t'
		})
		authCfg.CopyHeaders = fields
	}
	if rawPortal != "" {
		authCfg.PortalURL = rawPortal
	}
	if rawTimeout != "" {
		sec, err := ParseDurationSeconds(rawTimeout, "GYMBAG_AUTHELIA_TIMEOUT")
		if err != nil {
			return err
		}
		authCfg.Timeout = sec
	}
	if rawInsecure != "" {
		b, err := parseBool(rawInsecure, "GYMBAG_AUTHELIA_INSECURE_TLS")
		if err != nil {
			return err
		}
		authCfg.InsecureSkipTLS = b
	}
	if err := authCfg.Validate(); err != nil {
		return err
	}
	cfg.Authelia = authCfg

	if rawDomains != "" {
		doms, err := splitDomains(rawDomains)
		if err != nil {
			return err
		}
		targetSet := make(map[string]struct{}, len(doms))
		for _, d := range doms {
			targetSet[d] = struct{}{}
		}
		for i := range cfg.Sites {
			if _, ok := targetSet[cfg.Sites[i].Domain]; ok {
				cfg.Sites[i].Auth = authCfg.Clone()
			}
		}
		return nil
	}

	var portalHost string
	if authCfg.PortalURL != "" {
		if pu, pErr := url.Parse(authCfg.PortalURL); pErr == nil {
			portalHost = host.Normalize(pu.Hostname())
		}
	}
	for i := range cfg.Sites {
		if portalHost != "" && cfg.Sites[i].Domain == portalHost {
			continue
		}
		if cfg.Sites[i].Auth == nil || rawURL != "" {
			cfg.Sites[i].Auth = authCfg.Clone()
		}
	}
	return nil
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

func envDurationSeconds(getenv func(string) string, name string) (int, bool, error) {
	raw := strings.TrimSpace(getenv(name))
	if raw == "" {
		return 0, false, nil
	}
	v, err := ParseDurationSeconds(raw, name)
	if err != nil {
		return 0, false, err
	}
	return v, true, nil
}

func splitRemoteRuleURLs(inputs []string) ([]string, error) {
	var out []string
	for _, raw := range inputs {
		fields := strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
		})
		for _, f := range fields {
			u := strings.TrimSpace(f)
			if u == "" {
				continue
			}
			if err := RequireHTTPURL(u, "remote_rules"); err != nil {
				return nil, err
			}
			if !slices.Contains(out, u) {
				out = append(out, u)
			}
		}
	}
	return out, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
