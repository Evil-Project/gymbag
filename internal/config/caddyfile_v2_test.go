package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaddyfileV2TutorialFeaturesAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	confDir := filepath.Join(dir, "conf.d")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatal(err)
	}
	snippetFile := filepath.Join(confDir, "extras.caddy")
	if err := os.WriteFile(snippetFile, []byte(`
(std_headers) {
	header {
		+X-Frame-Options SAMEORIGIN
		-Server
	}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	rulesFile := filepath.Join(dir, "static.rules")
	if err := os.WriteFile(rulesFile, []byte("block ua *BadBot*\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	siteRulesFile := filepath.Join(dir, "site.rules")
	if err := os.WriteFile(siteRulesFile, []byte("verify ua *Scraper*\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rulesDir := filepath.Join(dir, "rules.d")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "01.rules"), []byte("allow path /healthz\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	caddyText := `
{
	rules_watch_interval 3s
	rules_file ./static.rules
	ruleset shared-dir {
		dir ./rules.d
		dynamic on
	}
}

import conf.d/*.caddy

(api_proxy) {
	reverse_proxy {
		to {args[0]}
		header_up X-Real-IP {remote_host}
	}
}

:2015 {
	respond /ping "pong" 200
}

app.example.com {
	import std_headers
	rules_dir ./rules.d dynamic
	ruleset site-static {
		source ./site.rules
		dynamic off
	}

	encode gzip
	log {
		output stdout
	}

	@health path /healthz /readyz
	respond @health "ok" 200

	@ver {
		path_regexp ^/v[0-9]+/
		method GET
	}
	handle @ver {
		reverse_proxy 127.0.0.1:9001
	}

	handle_path /api/* {
		import api_proxy 127.0.0.1:8080
	}

	redir /legacy/* https://app.example.com/new{uri} 301

	route {
		root * ./public
		try_files {path} /index.html
		file_server browse
	}
}
`

	cfg, err := ParseCaddyfileWithBaseDir(caddyText, dir)
	if err != nil {
		t.Fatalf("ParseCaddyfileWithBaseDir failed: %v", err)
	}

	if cfg.RulesWatchInterval != 3 {
		t.Fatalf("RulesWatchInterval = %d, want 3", cfg.RulesWatchInterval)
	}
	if len(cfg.Defaults.RuleSources) != 2 {
		t.Fatalf("global RuleSources len = %d, want 2: %+v", len(cfg.Defaults.RuleSources), cfg.Defaults.RuleSources)
	}
	if cfg.Defaults.RuleSources[0].Dynamic {
		t.Fatalf("expected rules_file ./static.rules to default Dynamic=false")
	}
	if !cfg.Defaults.RuleSources[1].Dynamic || cfg.Defaults.RuleSources[1].Name != "shared-dir" {
		t.Fatalf("unexpected second global RuleSource: %+v", cfg.Defaults.RuleSources[1])
	}

	if len(cfg.Sites) != 2 {
		t.Fatalf("len(Sites) = %d, want 2: %+v", len(cfg.Sites), cfg.Sites)
	}

	portSite := cfg.Sites[0]
	if portSite.Domain != "localhost:2015" && portSite.Domain != ":2015" {
		// Accept normalized port-only host representation as long as route exists
		t.Logf("port-only site normalized domain = %q", portSite.Domain)
	}
	if len(portSite.Routes) != 1 || portSite.Routes[0].Respond == nil || portSite.Routes[0].Respond.Body != "pong" {
		t.Fatalf("unexpected :2015 site routes: %+v", portSite.Routes)
	}

	app := cfg.Sites[1]
	if app.Domain != "app.example.com" {
		t.Fatalf("site[1].Domain = %q, want app.example.com", app.Domain)
	}
	if app.Headers["+X-Frame-Options"] != "SAMEORIGIN" {
		t.Fatalf("expected +X-Frame-Options header mutation, got: %+v", app.Headers)
	}
	if _, ok := app.Headers["-Server"]; !ok {
		t.Fatalf("expected -Server header removal mutation, got: %+v", app.Headers)
	}

	// Site inherits global defaults (static.rules, rules.d) + defines site-static (site.rules).
	if len(app.WAF.RuleSources) != 3 {
		t.Fatalf("site RuleSources len = %d, want 3: %+v", len(app.WAF.RuleSources), app.WAF.RuleSources)
	}
	if app.WAF.RuleSources[0].Source != "./static.rules" || app.WAF.RuleSources[0].Dynamic {
		t.Fatalf("unexpected site RuleSources[0]: %+v", app.WAF.RuleSources[0])
	}
	if app.WAF.RuleSources[1].Source != "./rules.d" || !app.WAF.RuleSources[1].Dynamic {
		t.Fatalf("unexpected site RuleSources[1]: %+v", app.WAF.RuleSources[1])
	}
	if app.WAF.RuleSources[2].Name != "site-static" || app.WAF.RuleSources[2].Source != "./site.rules" || app.WAF.RuleSources[2].Dynamic {
		t.Fatalf("unexpected site RuleSources[2]: %+v", app.WAF.RuleSources[2])
	}

	// Verify routes from @health, @ver, handle_path /api/*, redir, and file_server
	var sawHealth, sawReady, sawRegex, sawAPIStrip, sawRedir, sawStatic bool
	for _, r := range app.Routes {
		switch {
		case r.Path == "/healthz" && r.Respond != nil && r.Respond.Body == "ok":
			sawHealth = true
		case r.Path == "/readyz" && r.Respond != nil && r.Respond.Status == 200:
			sawReady = true
		case r.Path == "~^/v[0-9]+/" && len(r.Upstreams) == 1 && r.Upstreams[0] == "http://127.0.0.1:9001":
			sawRegex = true
		case r.Path == "/api/*" && r.StripPrefix == "/api" && len(r.Upstreams) == 1 && r.Upstreams[0] == "http://127.0.0.1:8080":
			sawAPIStrip = true
		case r.Path == "/legacy/*" && r.Redirect != nil && r.Redirect.Code == 301 && strings.Contains(r.Redirect.To, "{uri}"):
			sawRedir = true
		case r.Root == "./public" && r.Browse && r.Fallback == "/index.html":
			sawStatic = true
		}
	}
	if !sawHealth || !sawReady || !sawRegex || !sawAPIStrip || !sawRedir || !sawStatic {
		t.Fatalf("missing expected Caddyfile v2 routes (health=%t ready=%t regex=%t api=%t redir=%t static=%t) in %+v",
			sawHealth, sawReady, sawRegex, sawAPIStrip, sawRedir, sawStatic, app.Routes)
	}

	// FormatConfig round-trip
	formatted := FormatConfig(cfg)
	reparsed, err := ParseCaddyfileWithBaseDir(formatted, dir)
	if err != nil {
		t.Fatalf("FormatConfig round-trip parse failed: %v\nFormatted:\n%s", err, formatted)
	}
	if reparsed.RulesWatchInterval != cfg.RulesWatchInterval {
		t.Fatalf("round-trip RulesWatchInterval = %d, want %d", reparsed.RulesWatchInterval, cfg.RulesWatchInterval)
	}
	if len(reparsed.Defaults.RuleSources) != len(cfg.Defaults.RuleSources) {
		t.Fatalf("round-trip global RuleSources = %d, want %d", len(reparsed.Defaults.RuleSources), len(cfg.Defaults.RuleSources))
	}
	if len(reparsed.Sites) != len(cfg.Sites) || len(reparsed.Sites[1].Routes) != len(cfg.Sites[1].Routes) {
		t.Fatalf("round-trip route count = %d, want %d", len(reparsed.Sites[1].Routes), len(cfg.Sites[1].Routes))
	}
	if len(reparsed.Sites[1].Headers) != len(cfg.Sites[1].Headers) {
		t.Fatalf("round-trip headers mismatch: %+v vs %+v", reparsed.Sites[1].Headers, cfg.Sites[1].Headers)
	}
}
