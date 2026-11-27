package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestTLSOffAutoSSLAndDirectHTTPDirectives(t *testing.T) {
	cfg, err := ParseCaddyfile(`
{
	tls off
	direct_http on
}
example.com {
	reverse_proxy http://127.0.0.1:3000
}
`)
	if err != nil {
		t.Fatalf("ParseCaddyfile tls off failed: %v", err)
	}
	if cfg.TLS.Mode != "off" {
		t.Fatalf("expected tls.mode=off, got %q", cfg.TLS.Mode)
	}
	if cfg.TLS.AutoSSLEnabled() {
		t.Fatal("expected AutoSSLEnabled=false for tls off")
	}
	if !cfg.TLSDisabled() {
		t.Fatal("expected TLSDisabled=true for tls off")
	}
	if cfg.HTTPSEnabled() {
		t.Fatal("expected HTTPSEnabled=false for tls off")
	}
	if !cfg.DirectHTTPEnabled() {
		t.Fatal("expected DirectHTTPEnabled=true")
	}

	cfg2, err := ParseCaddyfile(`
{
	auto_https disable_redirects
	ssl_provider zerossl
}
example.com {
	reverse_proxy :3000
}
`)
	if err != nil {
		t.Fatalf("ParseCaddyfile auto_https disable_redirects failed: %v", err)
	}
	if !cfg2.DirectHTTP || !cfg2.DirectHTTPEnabled() {
		t.Fatal("expected DirectHTTP=true for auto_https disable_redirects")
	}
	if cfg2.TLSDisabled() || !cfg2.HTTPSEnabled() {
		t.Fatal("expected HTTPS to remain active when only disable_redirects is set")
	}

	cfg3, err := ParseCaddyfile(`
{
	http_only
}
example.com {
	reverse_proxy :3000
}
`)
	if err != nil {
		t.Fatalf("ParseCaddyfile http_only failed: %v", err)
	}
	if !cfg3.TLSDisabled() || !cfg3.DirectHTTP || cfg3.TLS.Mode != "off" {
		t.Fatalf("unexpected http_only config: %+v", cfg3.TLS)
	}

	// auto_ssl off with no external certs disables TLS cleanly
	cfg4, err := ParseCaddyfile(`
{
	auto_ssl off
}
example.com {
	reverse_proxy :3000
}
`)
	if err != nil {
		t.Fatalf("ParseCaddyfile auto_ssl off failed: %v", err)
	}
	if !cfg4.TLSDisabled() || cfg4.HTTPSEnabled() || !cfg4.DirectHTTPEnabled() {
		t.Fatalf("expected auto_ssl off without certs to disable TLS & allow direct HTTP")
	}

	// Contradiction check: auto_ssl off cannot be paired with active auto/ACME/local/selfsigned modes
	if _, err := ParseCaddyfile(`
{
	auto_ssl off
	tls acme
}
example.com {
	reverse_proxy :3000
}
`); err == nil {
		t.Fatal("expected validation error for auto_ssl off + tls acme")
	}
}

func TestAcmeShPathGlobalAndPerSite(t *testing.T) {
	cfg, err := ParseCaddyfile(`
{
	auto_ssl off
	acme_sh_path ~/.acme.sh
}
alpha.example.com {
	reverse_proxy :3000
}
beta.example.com {
	reverse_proxy :3001
	tls acme.sh /etc/ssl/acme-beta
}
`)
	if err != nil {
		t.Fatalf("ParseCaddyfile acme_sh failed: %v", err)
	}
	if cfg.TLS.AcmeShPath != "~/.acme.sh" {
		t.Fatalf("expected global acme_sh_path ~/.acme.sh, got %q", cfg.TLS.AcmeShPath)
	}
	if cfg.TLSDisabled() || !cfg.HTTPSEnabled() {
		t.Fatal("expected HTTPS to be active when acme_sh_path is set with auto_ssl off")
	}
	if cfg.DirectHTTP {
		t.Fatal("expected direct_http=false unless requested")
	}
	if len(cfg.Sites) != 2 || cfg.Sites[1].AcmeShPath != "/etc/ssl/acme-beta" {
		t.Fatalf("expected per-site acme_sh_path, got %+v", cfg.Sites)
	}

	// Also test nested inside tls { acme_sh ... }
	cfgNested, err := ParseCaddyfile(`
{
	tls {
		auto_ssl off
		acme_sh /var/lib/acme.sh
	}
}
app.example.com {
	reverse_proxy :3000
}
`)
	if err != nil {
		t.Fatalf("ParseCaddyfile nested tls block failed: %v", err)
	}
	if cfgNested.TLS.AcmeShPath != "/var/lib/acme.sh" || cfgNested.TLS.AutoSSLEnabled() {
		t.Fatalf("unexpected nested tls parse: %+v", cfgNested.TLS)
	}
}

func TestHTTPProtocolSiteBlockHeaders(t *testing.T) {
	// Pure http:// site without TLS config disables auto SSL & serves directly on http_port
	cfg, err := ParseCaddyfile(`
http://app.example.com:8080 {
	reverse_proxy :3000
}
`)
	if err != nil {
		t.Fatalf("ParseCaddyfile http:// site failed: %v", err)
	}
	if len(cfg.Sites) != 1 || cfg.Sites[0].Domain != "app.example.com" {
		t.Fatalf("unexpected sites: %+v", cfg.Sites)
	}
	if cfg.HTTPPort != 8080 || cfg.HTTPSPort != 443 {
		t.Fatalf("expected HTTPPort=8080, HTTPSPort=443, got %d / %d", cfg.HTTPPort, cfg.HTTPSPort)
	}
	if !cfg.DirectHTTP || !cfg.TLSDisabled() {
		t.Fatalf("expected DirectHTTP=true and TLSDisabled=true for pure http:// site")
	}

	// Mixed http:// and https:// keeps TLS enabled for the HTTPS site while allowing direct HTTP
	mixed, err := ParseCaddyfile(`
http://plain.example.com {
	reverse_proxy :3000
}
https://secure.example.com {
	reverse_proxy :3001
}
`)
	if err != nil {
		t.Fatalf("ParseCaddyfile mixed http/https failed: %v", err)
	}
	if !mixed.DirectHTTP || mixed.TLSDisabled() {
		t.Fatalf("expected DirectHTTP=true and TLSDisabled=false for mixed config")
	}
}

func TestPortConflictSkippedWhenTLSDisabled(t *testing.T) {
	// When TLS is off, https_port can share a port without failing conflict validation
	cfg := Default()
	cfg.TLS.Mode = "off"
	cfg.HTTPPort = 8080
	cfg.HTTPSPort = 8080
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no port conflict when tls.mode=off, got: %v", err)
	}

	// When TLS is active, matching http_port and https_port fails validation
	cfgActive := Default()
	cfgActive.HTTPPort = 8080
	cfgActive.HTTPSPort = 8080
	if err := cfgActive.Validate(); err == nil {
		t.Fatal("expected port conflict when TLS is enabled and http_port == https_port")
	}
}

func TestTLSHttpModesRoundTripAndEnv(t *testing.T) {
	orig := Default()
	f := false
	orig.DirectHTTP = true
	orig.TLS.AutoSSL = &f
	orig.TLS.AcmeShPath = "/root/.acme.sh"
	orig.Sites = []SiteConfig{
		{
			Domain:     "one.example.com",
			Upstreams:  []string{"http://127.0.0.1:3000"},
			Protect:    true,
			WAF:        DefaultSiteWAFConfig(),
			AcmeShPath: "/etc/ssl/one",
		},
	}
	if err := orig.Validate(); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	// Gymbagfile round-trip
	formatted := FormatConfig(orig)
	if !strings.Contains(formatted, "auto_ssl off") ||
		!strings.Contains(formatted, "acme_sh_path /root/.acme.sh") ||
		!strings.Contains(formatted, "direct_http on") ||
		!strings.Contains(formatted, "acme_sh_path /etc/ssl/one") {
		t.Fatalf("unexpected formatted Gymbagfile:\n%s", formatted)
	}
	reparsed, err := ParseCaddyfile(formatted)
	if err != nil {
		t.Fatalf("re-parse Gymbagfile failed: %v", err)
	}
	if !reparsed.DirectHTTP || reparsed.TLS.AutoSSLEnabled() || reparsed.TLS.AcmeShPath != "/root/.acme.sh" || reparsed.Sites[0].AcmeShPath != "/etc/ssl/one" {
		t.Fatalf("round-trip mismatch: %+v / %+v", reparsed.TLS, reparsed.Sites[0])
	}

	// Concise JSON round-trip
	jsonBytes, err := MarshalConciseJSONConfig(orig)
	if err != nil {
		t.Fatalf("MarshalConciseJSONConfig failed: %v", err)
	}
	fromJSON, err := ParseConfigBytes(jsonBytes, filepath.Join(t.TempDir(), "gymbag.json"), "")
	if err != nil {
		t.Fatalf("ParseConfigBytes JSON failed: %v\nJSON:\n%s", err, string(jsonBytes))
	}
	if !fromJSON.DirectHTTP || fromJSON.TLS.AutoSSLEnabled() || fromJSON.TLS.AcmeShPath != "/root/.acme.sh" || fromJSON.Sites[0].AcmeShPath != "/etc/ssl/one" {
		t.Fatalf("JSON round-trip mismatch: %+v / %+v", fromJSON.TLS, fromJSON.Sites[0])
	}

	// Env var overrides
	envMap := map[string]string{
		"GYMBAG_DOMAIN":       "env.example.com",
		"GYMBAG_UPSTREAM":     ":5000",
		"GYMBAG_AUTO_SSL":     "off",
		"GYMBAG_ACME_SH_PATH": "/opt/acme.sh",
		"GYMBAG_DIRECT_HTTP":  "on",
	}
	envCfg, err := LoadWithEnv(LoadOptions{}, func(k string) string { return envMap[k] })
	if err != nil {
		t.Fatalf("LoadWithEnv failed: %v", err)
	}
	if envCfg.TLS.AutoSSLEnabled() || envCfg.TLS.AcmeShPath != "/opt/acme.sh" || !envCfg.DirectHTTP {
		t.Fatalf("unexpected env config: directHTTP=%v tls=%+v", envCfg.DirectHTTP, envCfg.TLS)
	}

	httpOnlyEnv, err := LoadWithEnv(LoadOptions{}, func(k string) string {
		if k == "GYMBAG_HTTP_ONLY" {
			return "true"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("LoadWithEnv GYMBAG_HTTP_ONLY failed: %v", err)
	}
	if !httpOnlyEnv.TLSDisabled() || !httpOnlyEnv.DirectHTTP || httpOnlyEnv.TLS.Mode != "off" {
		t.Fatalf("expected TLS disabled + DirectHTTP for GYMBAG_HTTP_ONLY")
	}
}
