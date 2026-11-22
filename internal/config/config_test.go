package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestVerifyLevelParsing(t *testing.T) {
	cases := []struct {
		raw     string
		want    VerifyLevel
		protect bool
		wantLbl string
	}{
		{"0", VerifyLevelAlways, true, "always"},
		{"1", VerifyLevelOnlySuspicious, true, "only suspicious"},
		{"2", VerifyLevelOnlyAI, true, "only ai"},
		{"3", VerifyLevelSuspiciousAndAI, true, "suspicious & ai"},
		{"always", VerifyLevelAlways, true, "always"},
		{"only suspicious", VerifyLevelOnlySuspicious, true, "only suspicious"},
		{"only ai", VerifyLevelOnlyAI, true, "only ai"},
		{"suspicious & ai", VerifyLevelSuspiciousAndAI, true, "suspicious & ai"},
		{"off", VerifyLevelAlways, false, "always"},
	}
	for _, tc := range cases {
		got, protect, err := ParseVerifyLevel(tc.raw)
		if err != nil {
			t.Fatalf("ParseVerifyLevel(%q) unexpected error: %v", tc.raw, err)
		}
		if got != tc.want || protect != tc.protect || got.Label() != tc.wantLbl {
			t.Fatalf("ParseVerifyLevel(%q) = (%d [%s], %v), want (%d [%s], %v)",
				tc.raw, got, got.Label(), protect, tc.want, tc.wantLbl, tc.protect)
		}
	}
	for _, invalid := range []string{"-1", "4", "99", "unknown_mode"} {
		if _, _, err := ParseVerifyLevel(invalid); err == nil {
			t.Fatalf("expected error for invalid verify level %q", invalid)
		}
	}
}

func TestParseCaddyfileSingleMultiAndPerSiteLevels(t *testing.T) {
	simple, err := ParseCaddyfile(`
		example.com {
			reverse_proxy http://localhost:5173
		}
	`)
	if err != nil {
		t.Fatalf("ParseCaddyfile simple failed: %v", err)
	}
	if !slices.Equal(simple.Domains(), []string{"example.com"}) {
		t.Fatalf("Domains = %v", simple.Domains())
	}
	if simple.BindHost != "0.0.0.0" {
		t.Fatalf("BindHost = %q, want 0.0.0.0", simple.BindHost)
	}
	if simple.Sites[0].WAF.VerifyLevel != VerifyLevelAlways || !simple.Sites[0].Protect {
		t.Fatalf("unexpected default WAF config: %+v", simple.Sites[0])
	}

	multi, err := ParseCaddyfile(`
		{
			difficulty 3
			email admin@example.com
			verify 3
			admin 127.0.0.1:2019
		}

		# Site 1: level 0 (always) overrides global level 3
		app.example.com, www.example.com {
			reverse_proxy localhost:3000
			verify 0
			difficulty 5
		}

		# Site 2: level 2 (only ai)
		docs.example.com {
			reverse_proxy :4000
			waf {
				verify 2
				rate_limit 120 30
			}
		}

		# Site 3: level 1 (only suspicious)
		api.example.com {
			reverse_proxy :8080
			verify 1
		}

		# Site 4: inherits global verify 3 (suspicious & ai)
		blog.example.com {
			reverse_proxy :5000
		}

		# Site 5: unprotected website
		static.example.com {
			reverse_proxy 127.0.0.1:9000
			protect off
		}
	`)
	if err != nil {
		t.Fatalf("ParseCaddyfile multi failed: %v", err)
	}

	wantDomains := []string{"app.example.com", "www.example.com", "docs.example.com", "api.example.com", "blog.example.com", "static.example.com"}
	if !slices.Equal(multi.Domains(), wantDomains) {
		t.Fatalf("Domains = %v, want %v", multi.Domains(), wantDomains)
	}
	if multi.TLS.ACMEEmail != "admin@example.com" || multi.AdminAddr != "127.0.0.1:2019" {
		t.Fatalf("unexpected global fields: %+v", multi)
	}

	byDomain := make(map[string]SiteConfig)
	for _, s := range multi.Sites {
		byDomain[s.Domain] = s
	}

	if s := byDomain["app.example.com"]; !s.Protect || s.WAF.VerifyLevel != VerifyLevelAlways || s.WAF.Difficulty != 5 || s.PrimaryUpstream() != "http://localhost:3000" {
		t.Fatalf("app.example.com mismatch: %+v", s)
	}
	if s := byDomain["docs.example.com"]; !s.Protect || s.WAF.VerifyLevel != VerifyLevelOnlyAI || s.WAF.Difficulty != 3 || s.WAF.RateLimit != 120 || s.WAF.RateWindow != 30 {
		t.Fatalf("docs.example.com mismatch: %+v", s)
	}
	if s := byDomain["api.example.com"]; !s.Protect || s.WAF.VerifyLevel != VerifyLevelOnlySuspicious || s.WAF.Difficulty != 3 || s.PrimaryUpstream() != "http://localhost:8080" {
		t.Fatalf("api.example.com mismatch: %+v", s)
	}
	if s := byDomain["blog.example.com"]; !s.Protect || s.WAF.VerifyLevel != VerifyLevelSuspiciousAndAI {
		t.Fatalf("blog.example.com mismatch: %+v", s)
	}
	if s := byDomain["static.example.com"]; s.Protect || s.PrimaryUpstream() != "http://127.0.0.1:9000" {
		t.Fatalf("static.example.com should be unprotected: %+v", s)
	}
}

func TestGlobalProtectDomainSelectorAndFlatSyntax(t *testing.T) {
	selective, err := ParseCaddyfile(`
		{
			protect secure.example.com
			verify 3
		}
		secure.example.com {
			reverse_proxy :3000
		}
		open.example.com {
			reverse_proxy :4000
		}
	`)
	if err != nil {
		t.Fatalf("selective parse failed: %v", err)
	}
	if !selective.Sites[0].Protect || selective.Sites[0].WAF.VerifyLevel != VerifyLevelSuspiciousAndAI {
		t.Fatalf("secure.example.com should be protected at level 3: %+v", selective.Sites[0])
	}
	if selective.Sites[1].Protect {
		t.Fatalf("open.example.com should not be protected: %+v", selective.Sites[1])
	}

	flat, err := ParseCaddyfile(`
		example.org
		reverse_proxy http://127.0.0.1:9000
	`)
	if err != nil {
		t.Fatalf("flat parse failed: %v", err)
	}
	if !slices.Equal(flat.Domains(), []string{"example.org"}) || flat.Sites[0].PrimaryUpstream() != "http://127.0.0.1:9000" {
		t.Fatalf("flat mismatch: %+v", flat)
	}
}

func TestEnvAndJSONConfigLoading(t *testing.T) {
	env := map[string]string{
		"GYMBAG_DOMAINS":      "Example.COM, www.example.com.",
		"GYMBAG_ACME_EMAIL":   "owner@example.com",
		"GYMBAG_VERIFY_LEVEL": "2",
	}
	cfg, err := LoadWithEnv(LoadOptions{}, func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("LoadWithEnv failed: %v", err)
	}
	if !slices.Equal(cfg.Domains(), []string{"example.com", "www.example.com"}) {
		t.Fatalf("domains = %v", cfg.Domains())
	}
	if cfg.BindHost != "0.0.0.0" || cfg.Sites[0].WAF.VerifyLevel != VerifyLevelOnlyAI {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}

	env["GYMBAG_DIFFICULTY"] = "9"
	if _, err := LoadWithEnv(LoadOptions{}, func(k string) string { return env[k] }); err == nil {
		t.Fatal("expected error when GYMBAG_DIFFICULTY=9")
	}

	// JSON roundtrip test
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "gymbag.json")
	rawJSON, err := MarshalJSONConfig(cfg)
	if err != nil {
		t.Fatalf("MarshalJSONConfig failed: %v", err)
	}
	if err := os.WriteFile(jsonPath, rawJSON, 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	loadedJSON, err := ParseFile(jsonPath)
	if err != nil {
		t.Fatalf("ParseFile JSON failed: %v", err)
	}
	if !slices.Equal(loadedJSON.Domains(), cfg.Domains()) || loadedJSON.Sites[0].WAF.VerifyLevel != VerifyLevelOnlyAI {
		t.Fatalf("JSON roundtrip mismatch: %+v", loadedJSON)
	}

	// FormatCaddyfile roundtrip test
	formatted, err := FormatCaddyfile(`
		example.com {
			reverse_proxy :8080
			verify 3
			difficulty 2
		}
	`)
	if err != nil {
		t.Fatalf("FormatCaddyfile failed: %v", err)
	}
	reparsed, err := ParseCaddyfile(formatted)
	if err != nil {
		t.Fatalf("reparse formatted failed: %v", err)
	}
	if reparsed.Sites[0].WAF.VerifyLevel != VerifyLevelSuspiciousAndAI || reparsed.Sites[0].WAF.Difficulty != 2 {
		t.Fatalf("reparsed mismatch: %+v", reparsed.Sites[0])
	}
}

func TestCustomRulesAndIntelDirectivesParsingAndFmt(t *testing.T) {
	cfg, err := ParseCaddyfile(`
		{
			ip_security on
			ip_security_action block
			ip_reputation on
			ai_ip_list on
			intel_refresh 3600
			threat_feed https://feeds.example.com/bad-ips.txt
			ai_ip_feed https://feeds.example.com/ai-cidrs.json

			block ip 198.51.100.99 192.0.2.0/24
		}

		app.example.com {
			reverse_proxy :3000
			verify 3

			allow ip 127.0.0.1 10.0.0.0/8
			verify ip 203.0.113.0/24
			block user_agent BadScraper ~^Scrapy/
			verify path /api/export*

			rule "protect-login-post" {
				action verify
				method POST
				path /login*
				header X-Internal-Bypass ""
			}
		}
	`)
	if err != nil {
		t.Fatalf("ParseCaddyfile with rules & intel failed: %v", err)
	}
	if !cfg.Intel.IPSecurity || cfg.Intel.IPSecurityAction != "block" || cfg.Intel.RefreshInterval != 3600 {
		t.Fatalf("unexpected IntelConfig: %+v", cfg.Intel)
	}
	if len(cfg.Sites[0].WAF.Rules) != 6 {
		t.Fatalf("expected 6 rules (1 global + 5 site), got %d: %+v", len(cfg.Sites[0].WAF.Rules), cfg.Sites[0].WAF.Rules)
	}

	// Verify fmt round-trip preserves all rules and intel settings
	formatted, err := FormatCaddyfile(`
		{
			ip_security_action block
			intel_refresh 3600
		}
		app.example.com {
			reverse_proxy :3000
			verify 3
			block ip 198.51.100.99
			verify user_agent CustomBot
			rule "login-guard" {
				action verify
				method POST
				path /login*
			}
		}
	`)
	if err != nil {
		t.Fatalf("FormatCaddyfile failed: %v", err)
	}
	reparsed, err := ParseCaddyfile(formatted)
	if err != nil {
		t.Fatalf("reparse formatted failed: %v\nformatted:\n%s", err, formatted)
	}
	if reparsed.Intel.IPSecurityAction != "block" || len(reparsed.Sites[0].WAF.Rules) != 3 {
		t.Fatalf("round-trip lost rules or intel: %+v", reparsed)
	}

	// Reject invalid CIDR or broken regex in custom rule
	if _, err := ParseCaddyfile(`
		app.example.com {
			reverse_proxy :3000
			block ip not-an-ip
		}
	`); err == nil {
		t.Fatal("expected error on invalid IP in block rule")
	}
	if _, err := ParseCaddyfile(`
		app.example.com {
			reverse_proxy :3000
			block user_agent ~[unclosed
		}
	`); err == nil {
		t.Fatal("expected error on invalid regex in user_agent rule")
	}
}

func TestOpenBoxBareVerifyAndBlockDirectives(t *testing.T) {
	// Bare 'verify' -> Protect=true, VerifyLevel=3 (suspicious & ai), DefaultRulesEnabled()=true
	cfgVerify, err := ParseCaddyfile(`
		app.internal {
			reverse_proxy :3000
			verify
		}
	`)
	if err != nil {
		t.Fatalf("ParseCaddyfile bare verify failed: %v", err)
	}
	s := cfgVerify.Sites[0]
	if !s.Protect || s.WAF.VerifyLevel != VerifyLevelSuspiciousAndAI || !s.WAF.DefaultRulesEnabled() {
		t.Fatalf("bare verify mismatch: protect=%v level=%d defaultRules=%v", s.Protect, s.WAF.VerifyLevel, s.WAF.DefaultRulesEnabled())
	}

	// 'verify on' inside waf block + 'default_rules off' on a second site
	cfgVerifyOn, err := ParseCaddyfile(`
		a.example.com {
			reverse_proxy :3000
			waf {
				verify on
			}
		}
		b.example.com {
			reverse_proxy :4000
			verify
			default_rules off
		}
	`)
	if err != nil {
		t.Fatalf("ParseCaddyfile verify on / default_rules off failed: %v", err)
	}
	if !cfgVerifyOn.Sites[0].Protect || cfgVerifyOn.Sites[0].WAF.VerifyLevel != VerifyLevelSuspiciousAndAI || !cfgVerifyOn.Sites[0].WAF.DefaultRulesEnabled() {
		t.Fatalf("a.example.com mismatch: %+v", cfgVerifyOn.Sites[0])
	}
	if !cfgVerifyOn.Sites[1].Protect || cfgVerifyOn.Sites[1].WAF.DefaultRulesEnabled() {
		t.Fatalf("b.example.com expected default_rules off: %+v", cfgVerifyOn.Sites[1])
	}

	// Bare 'block' -> Protect=true, IPSecurity=true, IPSecurityAction="block", DefaultRulesEnabled()=true
	cfgBlock, err := ParseCaddyfile(`
		strict.example.com {
			reverse_proxy :5000
			block
		}
	`)
	if err != nil {
		t.Fatalf("ParseCaddyfile bare block failed: %v", err)
	}
	if !cfgBlock.Sites[0].Protect || !cfgBlock.Sites[0].WAF.DefaultRulesEnabled() {
		t.Fatalf("bare block site mismatch: %+v", cfgBlock.Sites[0])
	}
	if !cfgBlock.Intel.IPSecurity || cfgBlock.Intel.IPSecurityAction != "block" {
		t.Fatalf("bare block intel mismatch: %+v", cfgBlock.Intel)
	}

	// 'block on' works identically
	cfgBlockOn, err := ParseCaddyfile(`
		strict2.example.com {
			reverse_proxy :5000
			block on
		}
	`)
	if err != nil {
		t.Fatalf("ParseCaddyfile block on failed: %v", err)
	}
	if !cfgBlockOn.Intel.IPSecurity || cfgBlockOn.Intel.IPSecurityAction != "block" || !cfgBlockOn.Sites[0].WAF.DefaultRulesEnabled() {
		t.Fatalf("block on mismatch: intel=%+v site=%+v", cfgBlockOn.Intel, cfgBlockOn.Sites[0])
	}
}

func TestSSLProvidersEABAutoHostsEnvAndRedaction(t *testing.T) {
	providerCases := []struct {
		directive string
		wantMode  string
		wantProv  string
		wantDir   string
	}{
		{"ssl_provider letsencrypt", "acme", "letsencrypt", "https://acme-v02.api.letsencrypt.org/directory"},
		{"ssl_provider letsencrypt_staging", "acme", "letsencrypt_staging", "https://acme-v02.api.letsencrypt.org/directory"},
		{"tls zerossl", "acme", "zerossl", "https://acme-v02.api.letsencrypt.org/directory"},
		{"acme_provider google", "acme", "google", "https://acme-v02.api.letsencrypt.org/directory"},
		{"ca gts", "acme", "google", "https://acme-v02.api.letsencrypt.org/directory"},
		{"cert_issuer ssl.com", "acme", "ssl.com", "https://acme-v02.api.letsencrypt.org/directory"},
		{"tls sslcom", "acme", "ssl.com", "https://acme-v02.api.letsencrypt.org/directory"},
		{"tls litessl", "acme", "litessl", "https://acme-v02.api.letsencrypt.org/directory"},
		{"ssl_provider trustasia", "acme", "litessl", "https://acme-v02.api.letsencrypt.org/directory"},
		{"tls buypass", "acme", "buypass", "https://acme-v02.api.letsencrypt.org/directory"},
		{"ssl_provider local", "local", "local", "https://acme-v02.api.letsencrypt.org/directory"},
		{"tls local", "local", "", "https://acme-v02.api.letsencrypt.org/directory"},
		{"tls internal", "local", "", "https://acme-v02.api.letsencrypt.org/directory"},
		{"tls selfsigned", "selfsigned", "", "https://acme-v02.api.letsencrypt.org/directory"},
		{"ssl_provider https://acme.custom-ca.example/directory", "acme", "custom", "https://acme.custom-ca.example/directory"},
	}

	for _, tc := range providerCases {
		cfg, err := ParseCaddyfile(`
			{
				` + tc.directive + `
			}
			app.example.com {
				reverse_proxy :3000
			}
		`)
		if err != nil {
			t.Fatalf("directive %q failed: %v", tc.directive, err)
		}
		if cfg.TLS.Mode != tc.wantMode || cfg.TLS.Provider != tc.wantProv || cfg.TLS.ACMEDirectory != tc.wantDir {
			t.Fatalf("directive %q -> mode=%q prov=%q dir=%q, want mode=%q prov=%q dir=%q",
				tc.directive, cfg.TLS.Mode, cfg.TLS.Provider, cfg.TLS.ACMEDirectory, tc.wantMode, tc.wantProv, tc.wantDir)
		}
		if !cfg.TLS.AutoHostsEnabled() {
			t.Fatalf("directive %q expected AutoHostsEnabled() default true", tc.directive)
		}
	}

	// Full TLS EAB + account_key + ssl_key + auto_hosts + default_rules round-trip via FormatCaddyfile
	fullSrc := `
		{
			ssl_provider zerossl
			email ops@example.com
			eab kid-12345 hmac-secret-xyz
			account_key /etc/gymbag/account.pem
			ssl_key /etc/gymbag/domain.key
			key_password keypass
			auto_hosts off
			secret appsecret
		}
		app.internal {
			reverse_proxy :3000
			verify
			default_rules off
		}
	`
	fullCfg, err := ParseCaddyfile(fullSrc)
	if err != nil {
		t.Fatalf("ParseCaddyfile full TLS failed: %v", err)
	}
	if fullCfg.TLS.Provider != "zerossl" || fullCfg.TLS.Mode != "acme" ||
		fullCfg.TLS.EABKID != "kid-12345" || fullCfg.TLS.EABHMACKey != "hmac-secret-xyz" ||
		fullCfg.TLS.AccountKeyFile != "/etc/gymbag/account.pem" || fullCfg.TLS.KeyFile != "/etc/gymbag/domain.key" ||
		fullCfg.TLS.AutoHostsEnabled() {
		t.Fatalf("unexpected TLSConfig: %+v", fullCfg.TLS)
	}

	// Redacted() must mask Secret, KeyPassword, and EABHMACKey without mutating original
	redacted := fullCfg.Redacted()
	if redacted.Secret != "***REDACTED***" ||
		redacted.TLS.KeyPassword != "***REDACTED***" ||
		redacted.TLS.EABHMACKey != "***REDACTED***" {
		t.Fatalf("Redacted() failed to mask secrets: %+v", redacted)
	}
	if fullCfg.TLS.EABHMACKey != "hmac-secret-xyz" {
		t.Fatalf("Redacted() mutated original EABHMACKey: %q", fullCfg.TLS.EABHMACKey)
	}

	formatted, err := FormatCaddyfile(fullSrc)
	if err != nil {
		t.Fatalf("FormatCaddyfile failed: %v", err)
	}
	for _, needle := range []string{"ssl_provider zerossl", "eab_kid kid-12345", "account_key /etc/gymbag/account.pem", "auto_hosts off", "default_rules off"} {
		if !strings.Contains(formatted, needle) {
			t.Fatalf("formatted Caddyfile missing %q:\n%s", needle, formatted)
		}
	}
	reparsed, err := ParseCaddyfile(formatted)
	if err != nil {
		t.Fatalf("reparse formatted failed: %v\n%s", err, formatted)
	}
	if reparsed.TLS.Provider != "zerossl" || reparsed.TLS.EABKID != "kid-12345" ||
		reparsed.TLS.EABHMACKey != "hmac-secret-xyz" || reparsed.TLS.AccountKeyFile != "/etc/gymbag/account.pem" ||
		reparsed.TLS.KeyFile != "/etc/gymbag/domain.key" || reparsed.TLS.AutoHostsEnabled() ||
		reparsed.Sites[0].WAF.DefaultRulesEnabled() {
		t.Fatalf("round-trip mismatch: tls=%+v waf=%+v", reparsed.TLS, reparsed.Sites[0].WAF)
	}

	// Environment variable overrides
	env := map[string]string{
		"GYMBAG_DOMAIN":           "api.internal",
		"GYMBAG_UPSTREAM":         ":8080",
		"GYMBAG_SSL_PROVIDER":     "litessl",
		"GYMBAG_EAB_KID":          "env-kid",
		"GYMBAG_EAB_HMAC_KEY":     "env-hmac",
		"GYMBAG_ACCOUNT_KEY_FILE": "/tmp/acct.key",
		"GYMBAG_AUTO_HOSTS":       "false",
		"GYMBAG_DEFAULT_RULES":    "off",
	}
	envCfg, err := LoadWithEnv(LoadOptions{}, func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("LoadWithEnv failed: %v", err)
	}
	if envCfg.TLS.Provider != "litessl" || envCfg.TLS.Mode != "acme" ||
		envCfg.TLS.EABKID != "env-kid" || envCfg.TLS.EABHMACKey != "env-hmac" ||
		envCfg.TLS.AccountKeyFile != "/tmp/acct.key" || envCfg.TLS.AutoHostsEnabled() ||
		envCfg.Sites[0].WAF.DefaultRulesEnabled() || envCfg.BindHost != "127.0.0.1" {
		t.Fatalf("env override mismatch: bind=%q tls=%+v site=%+v", envCfg.BindHost, envCfg.TLS, envCfg.Sites[0].WAF)
	}
}

func TestRuleActionShorthandAliasesLocalBindHostAndCustomProviderReset(t *testing.T) {
	// 1. Shorthand rule action aliases: drop, reject, pow, pass, bypass, whitelist
	aliasCfg, err := ParseCaddyfile(`
		app.internal, svc.test {
			reverse_proxy :3000
			drop ip 198.51.100.1
			reject path /secret*
			pow user_agent SomeBot
			pass ip 127.0.0.1
			bypass header X-Internal 1
			whitelist cookie preview ok
		}
	`)
	if err != nil {
		t.Fatalf("ParseCaddyfile with shorthand aliases failed: %v", err)
	}
	if aliasCfg.BindHost != "127.0.0.1" {
		t.Fatalf("DefaultBindHost for app.internal & svc.test = %q, want 127.0.0.1", aliasCfg.BindHost)
	}
	rules := aliasCfg.Sites[0].WAF.Rules
	wantActions := []RuleAction{
		RuleActionBlock,
		RuleActionBlock,
		RuleActionVerify,
		RuleActionAllow,
		RuleActionAllow,
		RuleActionAllow,
	}
	if len(rules) != len(wantActions) {
		t.Fatalf("got %d rules, want %d", len(rules), len(wantActions))
	}
	for i, want := range wantActions {
		if rules[i].Action != want {
			t.Fatalf("rules[%d].Action = %q, want %q", i, rules[i].Action, want)
		}
	}

	// 2. Overriding custom ACME URL ssl_provider with a named provider resets ACMEDirectory
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "Gymbagfile")
	if err := os.WriteFile(cfgPath, []byte(`
		{
			ssl_provider https://acme.custom-ca.example/directory
		}
		app.example.com {
			reverse_proxy :3000
		}
	`), 0o644); err != nil {
		t.Fatal(err)
	}
	overridden, err := LoadWithEnv(LoadOptions{ConfigPath: cfgPath}, func(k string) string {
		if k == "GYMBAG_SSL_PROVIDER" {
			return "zerossl"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("LoadWithEnv custom->named provider override failed: %v", err)
	}
	if overridden.TLS.Provider != "zerossl" || overridden.TLS.ACMEDirectory != DefaultTLSConfig().ACMEDirectory {
		t.Fatalf("expected ACMEDirectory reset on named provider override, got provider=%q dir=%q",
			overridden.TLS.Provider, overridden.TLS.ACMEDirectory)
	}
}

func TestSSHMinecraftAndVerifyConfigParsingFormattingAndEnv(t *testing.T) {
	// 1. Combined HTTP + SSH + Minecraft Gymbagfile parsing & formatting roundtrip
	cfg, err := ParseCaddyfile(`
		{
			verify_url https://verify.example.com
			verify_port 9443
			vpn_detection off
		}

		app.example.com {
			reverse_proxy :3000
			verify 3
		}

		ssh {
			upstream 127.0.0.1:41234
			verify 1
			allow ip 10.0.0.0/8
		}

		minecraft {
			upstream 127.0.0.1:42556
			server_properties ./server.properties
			verify 3
			difficulty 3
		}
	`)
	if err != nil {
		t.Fatalf("ParseCaddyfile all-in-one failed: %v", err)
	}
	if !cfg.HTTPEnabled() || !cfg.SSHEnabled() || !cfg.MinecraftEnabled() || !cfg.NeedsVerifyServer() {
		t.Fatalf("expected all 3 services + verify enabled: %+v", cfg)
	}
	if cfg.SSH.Port != DefaultSSHPort || cfg.Minecraft.Port != DefaultMinecraftPort {
		t.Fatalf("expected default ports 22/25565, got ssh=%d mc=%d", cfg.SSH.Port, cfg.Minecraft.Port)
	}
	if cfg.Verify.URL != "https://verify.example.com" || cfg.Verify.EffectivePort() != 9443 || cfg.Intel.VPNDetection {
		t.Fatalf("verify/intel mismatch: verify=%+v intel=%+v", cfg.Verify, cfg.Intel)
	}

	formatted := FormatConfig(cfg)
	reparsed, err := ParseCaddyfile(formatted)
	if err != nil {
		t.Fatalf("FormatConfig roundtrip failed: %v\n%s", err, formatted)
	}
	if !reparsed.SSHEnabled() || !reparsed.MinecraftEnabled() || reparsed.SSH.Upstream != "127.0.0.1:41234" || reparsed.Minecraft.Difficulty != 3 {
		t.Fatalf("reparsed mismatch: ssh=%+v mc=%+v", reparsed.SSH, reparsed.Minecraft)
	}

	// 2. Concise JSON roundtrip for SSH-only (zero sites)
	conciseJSON := []byte(`{
		"ssh": {
			"port": 22,
			"upstream": "127.0.0.1:40022",
			"verify_level": 3
		}
	}`)
	sshOnly, err := ParseConfigBytes(conciseJSON, "gymbag.json", "")
	if err != nil {
		t.Fatalf("ParseConfigBytes concise SSH JSON failed: %v", err)
	}
	if sshOnly.HTTPEnabled() || !sshOnly.SSHEnabled() || !sshOnly.SSH.Protect || sshOnly.BindHost != "0.0.0.0" || sshOnly.Verify.EffectivePort() != 8443 {
		t.Fatalf("concise SSH JSON mismatch: %+v ssh=%+v", sshOnly, sshOnly.SSH)
	}

	// 3. Env-only Minecraft configuration clears default localhost HTTP site
	mcEnvCfg, err := LoadWithEnv(LoadOptions{}, func(k string) string {
		switch k {
		case "GYMBAG_MC_ENABLED":
			return "true"
		case "GYMBAG_MC_UPSTREAM":
			return ":35565"
		case "GYMBAG_MC_VERIFY_LEVEL":
			return "1"
		case "GYMBAG_VERIFY_PORT":
			return "8444"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("LoadWithEnv MC-only failed: %v", err)
	}
	if mcEnvCfg.HTTPEnabled() || !mcEnvCfg.MinecraftEnabled() || mcEnvCfg.Minecraft.Port != 25565 || mcEnvCfg.Minecraft.Upstream != "127.0.0.1:35565" || mcEnvCfg.Verify.EffectivePort() != 8444 {
		t.Fatalf("env MC-only mismatch: %+v mc=%+v", mcEnvCfg, mcEnvCfg.Minecraft)
	}

	// 4. Port conflict detection
	if _, err := ParseCaddyfile(`
		{
			verify_port 25565
		}
		minecraft {
			upstream 127.0.0.1:25566
		}
	`); err == nil {
		t.Fatal("expected error when verify_port conflicts with minecraft.port")
	}
}
