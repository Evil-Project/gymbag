package config

import (
	"slices"
	"strings"
	"testing"
)

func TestParseDurationSeconds(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{"1", 1},
		{"300", 300},
		{"86400", 86400},
		{"45s", 45},
		{"30m", 1800},
		{"12h", 43200},
		{"1h30m", 5400},
		{"7d", 7 * 86400},
		{"1d12h", 36 * 3600},
		{"2w", 14 * 86400},
	}
	for _, tc := range cases {
		got, err := ParseDurationSeconds(tc.raw, "verify_timeout")
		if err != nil {
			t.Fatalf("ParseDurationSeconds(%q) unexpected error: %v", tc.raw, err)
		}
		if got != tc.want {
			t.Fatalf("ParseDurationSeconds(%q) = %d, want %d", tc.raw, got, tc.want)
		}
	}

	for _, invalid := range []string{"", "0", "-10", "-5m", "500ms", "abc", "7x", "d"} {
		if _, err := ParseDurationSeconds(invalid, "verify_timeout"); err == nil {
			t.Fatalf("expected error for invalid duration %q", invalid)
		}
	}
}

func TestVerifyTimeoutAndRemoteRulesInGymbagfileAndEnv(t *testing.T) {
	raw := `
{
    verify_timeout 12h
    remote_rules https://rules.example.com/global.rules
    remote_rules_refresh 10m
}

app.example.com {
    reverse_proxy :3000
    verify 3
    verified_timeout 30m
    remote_rules https://rules.example.com/app.json
}

blog.example.com {
    reverse_proxy :4000
    verify 2
}
`
	cfg, err := ParseCaddyfile(raw)
	if err != nil {
		t.Fatalf("ParseCaddyfile failed: %v", err)
	}
	if cfg.Defaults.ClearanceTTL != 12*3600 {
		t.Fatalf("Defaults.ClearanceTTL = %d, want %d", cfg.Defaults.ClearanceTTL, 12*3600)
	}
	if cfg.RemoteRulesRefresh != 600 {
		t.Fatalf("RemoteRulesRefresh = %d, want 600", cfg.RemoteRulesRefresh)
	}
	if cfg.Sites[0].WAF.ClearanceTTL != 1800 {
		t.Fatalf("app.example.com ClearanceTTL = %d, want 1800", cfg.Sites[0].WAF.ClearanceTTL)
	}
	wantAppRemote := []string{"https://rules.example.com/global.rules", "https://rules.example.com/app.json"}
	if !slices.Equal(cfg.Sites[0].WAF.RemoteRules, wantAppRemote) {
		t.Fatalf("app RemoteRules = %v, want %v", cfg.Sites[0].WAF.RemoteRules, wantAppRemote)
	}
	if cfg.Sites[1].WAF.ClearanceTTL != 12*3600 {
		t.Fatalf("blog.example.com ClearanceTTL = %d, want %d", cfg.Sites[1].WAF.ClearanceTTL, 12*3600)
	}
	if !slices.Equal(cfg.Sites[1].WAF.RemoteRules, []string{"https://rules.example.com/global.rules"}) {
		t.Fatalf("blog RemoteRules = %v", cfg.Sites[1].WAF.RemoteRules)
	}

	// Format & re-parse round-trip
	formatted := FormatConfig(cfg)
	if !strings.Contains(formatted, "remote_rules_refresh 600") ||
		!strings.Contains(formatted, "remote_rules https://rules.example.com/app.json") ||
		!strings.Contains(formatted, "clearance_ttl 1800") {
		t.Fatalf("unexpected formatted config:\n%s", formatted)
	}
	reparsed, err := ParseCaddyfile(formatted)
	if err != nil {
		t.Fatalf("re-parse formatted config failed: %v", err)
	}
	if reparsed.Sites[0].WAF.ClearanceTTL != 1800 || !slices.Equal(reparsed.Sites[0].WAF.RemoteRules, wantAppRemote) {
		t.Fatalf("round-trip mismatch: %+v", reparsed.Sites[0].WAF)
	}

	// Environment variable overrides
	env := map[string]string{
		"GYMBAG_DOMAIN":               "env.example.com",
		"GYMBAG_UPSTREAM":             "http://localhost:8080",
		"GYMBAG_VERIFY_TIMEOUT":       "7d",
		"GYMBAG_REMOTE_RULES":         "https://rules.example.com/a.rules, https://rules.example.com/b.json",
		"GYMBAG_REMOTE_RULES_REFRESH": "2m",
	}
	envCfg, err := LoadWithEnv(LoadOptions{}, func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("LoadWithEnv failed: %v", err)
	}
	if envCfg.Sites[0].WAF.ClearanceTTL != 7*86400 {
		t.Fatalf("env ClearanceTTL = %d, want %d", envCfg.Sites[0].WAF.ClearanceTTL, 7*86400)
	}
	if envCfg.RemoteRulesRefresh != 120 {
		t.Fatalf("env RemoteRulesRefresh = %d, want 120", envCfg.RemoteRulesRefresh)
	}
	wantEnvRemote := []string{"https://rules.example.com/a.rules", "https://rules.example.com/b.json"}
	if !slices.Equal(envCfg.Sites[0].WAF.RemoteRules, wantEnvRemote) {
		t.Fatalf("env RemoteRules = %v, want %v", envCfg.Sites[0].WAF.RemoteRules, wantEnvRemote)
	}
}

func TestParseRemoteRulesFormats(t *testing.T) {
	// 1. Gymbagfile rule syntax
	textPayload := `
# Remote ruleset
allow ip 10.0.0.0/8
block user_agent EvilHarvester ~^BadBot/
rule "protect-admin" {
    action verify
    method POST
    path /admin/*
}
`
	rules, err := ParseRemoteRules([]byte(textPayload))
	if err != nil {
		t.Fatalf("ParseRemoteRules text failed: %v", err)
	}
	if len(rules) != 3 || rules[0].Action != RuleActionAllow || rules[1].Action != RuleActionBlock || rules[2].Name != "protect-admin" {
		t.Fatalf("unexpected text rules: %+v", rules)
	}

	// 2. JSON array
	jsonArr := `[{"name":"drop-cidr","action":"block","ips":["198.51.100.0/24"]},{"action":"allow","paths":["/healthz"]}]`
	arrRules, err := ParseRemoteRules([]byte(jsonArr))
	if err != nil {
		t.Fatalf("ParseRemoteRules JSON array failed: %v", err)
	}
	if len(arrRules) != 2 || arrRules[0].Name != "drop-cidr" || arrRules[1].Action != RuleActionAllow {
		t.Fatalf("unexpected JSON array rules: %+v", arrRules)
	}

	// 3. JSON object wrapper
	jsonObj := `{"rules":[{"action":"verify","methods":["POST"],"paths":["/login"]}]}`
	objRules, err := ParseRemoteRules([]byte(jsonObj))
	if err != nil {
		t.Fatalf("ParseRemoteRules JSON object failed: %v", err)
	}
	if len(objRules) != 1 || objRules[0].Action != RuleActionVerify {
		t.Fatalf("unexpected JSON object rules: %+v", objRules)
	}

	// 4. Invalid payload rejected
	if _, err := ParseRemoteRules([]byte(`block invalid_field foo`)); err == nil {
		t.Fatal("expected error for invalid remote rule field")
	}
	if _, err := ParseRemoteRules([]byte(`[{"action":"block"}]`)); err == nil {
		t.Fatal("expected error for remote rule with no matchers")
	}
}
