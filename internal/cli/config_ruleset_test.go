package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gymbag/internal/config"
)

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code := ExecuteWithIO(args, strings.NewReader(""), &out, &errBuf)
	return code, out.String(), errBuf.String()
}

func TestCLIInitJSONAndRuleFlags(t *testing.T) {
	dir := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	if err := os.WriteFile(filepath.Join(dir, "custom.rules"), []byte("block ua *BadBot*\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rulesDir := filepath.Join(dir, "rules.d")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "01.rules"), []byte("allow path /healthz\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "gymbag.json")
	code, out, errOut := runCLI(t,
		"init", "--yes",
		"-o", cfgPath,
		"-d", "app.example.com",
		"-u", ":3000",
		"--rules-file", "./custom.rules",
		"--rules-dir", "./rules.d",
		"--rules-dynamic",
		"--rules-watch-interval", "5s",
	)
	if code != 0 {
		t.Fatalf("init failed (%d): %s\nstderr:\n%s", code, out, errOut)
	}

	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Fatalf("generated config is not valid JSON:\n%s", string(raw))
	}

	cfg, err := config.ParseFile(cfgPath)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	if cfg.RulesWatchInterval != 5 {
		t.Fatalf("RulesWatchInterval = %d, want 5", cfg.RulesWatchInterval)
	}
	if len(cfg.Sites) != 1 || len(cfg.Sites[0].WAF.RuleSources) != 2 {
		t.Fatalf("unexpected site RuleSources: %+v", cfg.Sites)
	}
	for _, rs := range cfg.Sites[0].WAF.RuleSources {
		if !rs.Dynamic {
			t.Fatalf("expected --rules-dynamic to mark RuleSource dynamic: %+v", rs)
		}
	}

	if vCode, _, vErr := runCLI(t, "validate", "-c", cfgPath); vCode != 0 {
		t.Fatalf("validate failed (%d): %s", vCode, vErr)
	}
}

func TestCLIConfigCRUDAndConvert(t *testing.T) {
	dir := t.TempDir()
	jsonCfg := filepath.Join(dir, "gymbag.json")
	if code, _, errOut := runCLI(t, "init", "--yes", "-o", jsonCfg, "-d", "app.example.com", "-u", ":3000"); code != 0 {
		t.Fatalf("init failed: %s", errOut)
	}

	for _, args := range [][]string{
		{"config", "add", "-c", jsonCfg, "-d", "api.example.com", "-u", ":8080", "-l", "1"},
		{"config", "add", "-c", jsonCfg, "-d", "static.example.com", "--root", "./public"},
		{"config", "update", "-c", jsonCfg, "-d", "api.example.com", "-l", "3", "--verify-timeout", "6h", "--header", "X-Env=prod"},
		{"config", "route", "add", "-c", jsonCfg, "-d", "api.example.com", "--path", "/v1/*", "--strip-prefix", "/v1", "-u", ":8081"},
		{"config", "route", "update", "-c", jsonCfg, "-d", "api.example.com", "--path", "/v1/*", "-u", ":8082", "--header", "X-Ver=2"},
		{"config", "set", "-c", jsonCfg, "admin", "127.0.0.1:2019"},
		{"config", "set", "-c", jsonCfg, "rules_watch_interval", "4s"},
		{"config", "unset", "-c", jsonCfg, "admin"},
		{"config", "remove", "-c", jsonCfg, "-d", "static.example.com"},
	} {
		if code, _, errOut := runCLI(t, args...); code != 0 {
			t.Fatalf("%v failed (%d): %s", args, code, errOut)
		}
	}

	code, listOut, errOut := runCLI(t, "config", "list", "-c", jsonCfg)
	if code != 0 || !strings.Contains(listOut, "api.example.com") || strings.Contains(listOut, "static.example.com") {
		t.Fatalf("unexpected config list (%d): out=%q err=%q", code, listOut, errOut)
	}
	if code, rListOut, errOut := runCLI(t, "config", "route", "list", "-c", jsonCfg, "-d", "api.example.com"); code != 0 || !strings.Contains(rListOut, "/v1/*") {
		t.Fatalf("unexpected route list (%d): out=%q err=%q", code, rListOut, errOut)
	}
	if code, showOut, errOut := runCLI(t, "config", "show", "-c", jsonCfg); code != 0 || !json.Valid([]byte(showOut)) {
		t.Fatalf("config show did not emit valid JSON (%d): err=%q", code, errOut)
	}

	if code, _, errOut := runCLI(t, "config", "route", "remove", "-c", jsonCfg, "-d", "api.example.com", "--index", "0"); code != 0 {
		t.Fatalf("route remove failed: %s", errOut)
	}

	cfg, err := config.ParseFile(jsonCfg)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AdminAddr != "" || cfg.RulesWatchInterval != 4 {
		t.Fatalf("unexpected global config after set/unset: admin=%q watch=%d", cfg.AdminAddr, cfg.RulesWatchInterval)
	}
	if len(cfg.Sites) != 2 || cfg.Sites[1].WAF.VerifyLevel != config.VerifyLevelSuspiciousAndAI || cfg.Sites[1].WAF.ClearanceTTL != 21600 || cfg.Sites[1].Headers["X-Env"] != "prod" || len(cfg.Sites[1].Routes) != 0 {
		t.Fatalf("unexpected updated site: %+v", cfg.Sites)
	}

	// Verify same mutations work on Gymbagfile too
	caddyCfg := filepath.Join(dir, "Gymbagfile")
	if code, _, errOut := runCLI(t, "init", "--yes", "-o", caddyCfg, "-d", "caddy.example.com", "-u", ":4000"); code != 0 {
		t.Fatalf("init Gymbagfile failed: %s", errOut)
	}
	if code, _, errOut := runCLI(t, "config", "add", "-c", caddyCfg, "-d", "extra.example.com", "-u", ":4001"); code != 0 {
		t.Fatalf("config add on Gymbagfile failed: %s", errOut)
	}

	// Test `config convert` from native nginx and Apache configs
	nginxPath := filepath.Join(dir, "nginx.conf")
	if err := os.WriteFile(nginxPath, []byte(`
server {
	listen 443 ssl;
	server_name nginx.example.com;
	location /api/ {
		proxy_pass http://127.0.0.1:9000/;
	}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	convertedNginx := filepath.Join(dir, "from-nginx.json")
	if code, _, errOut := runCLI(t, "config", "convert", "-c", nginxPath, "-o", convertedNginx, "--format", "json"); code != 0 {
		t.Fatalf("convert nginx->json failed: %s", errOut)
	}
	parsedNginx, err := config.ParseFile(convertedNginx)
	if err != nil || len(parsedNginx.Sites) != 1 || parsedNginx.Sites[0].Domain != "nginx.example.com" {
		t.Fatalf("converted nginx parse mismatch: cfg=%+v err=%v", parsedNginx, err)
	}

	apachePath := filepath.Join(dir, "httpd.conf")
	if err := os.WriteFile(apachePath, []byte(`
<VirtualHost *:443>
	ServerName apache.example.com
	ProxyPass /api http://127.0.0.1:9100/api
	ProxyPass / http://127.0.0.1:9101
</VirtualHost>
`), 0o644); err != nil {
		t.Fatal(err)
	}
	convertedApache := filepath.Join(dir, "from-apache.Gymbagfile")
	if code, _, errOut := runCLI(t, "config", "convert", "-c", apachePath, "-o", convertedApache, "--format", "gymbagfile"); code != 0 {
		t.Fatalf("convert apache->gymbagfile failed: %s", errOut)
	}
	parsedApache, err := config.ParseFile(convertedApache)
	if err != nil || len(parsedApache.Sites) != 1 || parsedApache.Sites[0].Domain != "apache.example.com" || len(parsedApache.Sites[0].Routes) == 0 {
		t.Fatalf("converted apache parse mismatch: cfg=%+v err=%v", parsedApache, err)
	}
}

func TestCLIRulesetAndRuleCommands(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "gymbag.json")
	if code, _, errOut := runCLI(t, "init", "--yes", "-o", cfgPath, "-d", "app.example.com", "-u", ":3000"); code != 0 {
		t.Fatalf("init failed: %s", errOut)
	}

	rulesText := filepath.Join(dir, "custom.rules")
	rulesJSON := filepath.Join(dir, "custom.json")
	rulesDir := filepath.Join(dir, "rules.d")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "01.rules"), []byte("block path /wp-login.php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	badFile := filepath.Join(dir, "bad.rules")
	if err := os.WriteFile(badFile, []byte("nonsense_rule foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Init & validate dedicated rule files and folders
	for _, args := range [][]string{
		{"ruleset", "init", rulesText},
		{"ruleset", "init", rulesJSON},
		{"ruleset", "validate", rulesText},
		{"ruleset", "validate", rulesJSON},
		{"ruleset", "validate", rulesDir},
	} {
		if code, _, errOut := runCLI(t, args...); code != 0 {
			t.Fatalf("%v failed (%d): %s", args, code, errOut)
		}
	}
	if code, _, _ := runCLI(t, "ruleset", "validate", badFile); code == 0 {
		t.Fatalf("expected ruleset validate on invalid rule file to fail")
	}

	// Attach, list, update, remove rulesets in config
	if code, _, errOut := runCLI(t, "ruleset", "add", "-c", cfgPath, "--source", rulesText, "--dynamic", "--name", "corp"); code != 0 {
		t.Fatalf("ruleset add failed: %s", errOut)
	}
	if code, out, errOut := runCLI(t, "ruleset", "list", "-c", cfgPath); code != 0 || !strings.Contains(out, "corp") {
		t.Fatalf("ruleset list unexpected (%d): out=%q err=%q", code, out, errOut)
	}
	if code, _, errOut := runCLI(t, "ruleset", "update", "-c", cfgPath, "corp", "--static", "--name", "corp-static"); code != 0 {
		t.Fatalf("ruleset update failed: %s", errOut)
	}
	cfg, err := config.ParseFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sites[0].WAF.RuleSources) != 1 || cfg.Sites[0].WAF.RuleSources[0].Dynamic || cfg.Sites[0].WAF.RuleSources[0].Name != "corp-static" {
		t.Fatalf("unexpected RuleSources after update: %+v", cfg.Sites[0].WAF.RuleSources)
	}
	if code, _, errOut := runCLI(t, "ruleset", "remove", "-c", cfgPath, "corp-static"); code != 0 {
		t.Fatalf("ruleset remove failed: %s", errOut)
	}

	// Rule CRUD on .rules, .json, and inline site config
	for _, target := range []string{rulesText, rulesJSON} {
		if code, _, errOut := runCLI(t, "ruleset", "rule", "add", "--file", target, "--action", "block", "--name", "bad-bot", "--ua", "*BadBot*"); code != 0 {
			t.Fatalf("rule add %s failed: %s", target, errOut)
		}
		if code, out, errOut := runCLI(t, "ruleset", "rule", "list", "--file", target); code != 0 || !strings.Contains(out, "bad-bot") {
			t.Fatalf("rule list %s unexpected: out=%q err=%q", target, out, errOut)
		}
		if code, _, errOut := runCLI(t, "ruleset", "rule", "update", "--file", target, "--name", "bad-bot", "--action", "verify", "--path", "/api/*"); code != 0 {
			t.Fatalf("rule update %s failed: %s", target, errOut)
		}
		raw, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := config.ParseRemoteRules(raw)
		if err != nil {
			t.Fatalf("ParseRemoteRules(%s) failed: %v", target, err)
		}
		found := false
		for _, r := range parsed {
			if r.Name == "bad-bot" && r.Action == config.RuleActionVerify && len(r.Paths) == 1 && r.Paths[0] == "/api/*" {
				found = true
			}
		}
		if !found {
			t.Fatalf("updated bad-bot rule missing in %s: %+v", target, parsed)
		}
		if code, _, errOut := runCLI(t, "ruleset", "rule", "remove", "--file", target, "--name", "bad-bot"); code != 0 {
			t.Fatalf("rule remove %s failed: %s", target, errOut)
		}
	}

	// Inline rule CRUD in gymbag.json
	if code, _, errOut := runCLI(t, "ruleset", "rule", "add", "-c", cfgPath, "-d", "app.example.com", "--action", "block", "--name", "inline-block", "--ip", "198.51.100.0/24"); code != 0 {
		t.Fatalf("inline rule add failed: %s", errOut)
	}
	if code, _, errOut := runCLI(t, "ruleset", "rule", "update", "-c", cfgPath, "-d", "app.example.com", "--name", "inline-block", "--action", "allow"); code != 0 {
		t.Fatalf("inline rule update failed: %s", errOut)
	}
	cfgInline, err := config.ParseFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfgInline.Sites[0].WAF.Rules) != 1 || cfgInline.Sites[0].WAF.Rules[0].Action != config.RuleActionAllow {
		t.Fatalf("unexpected inline site rules after update: %+v", cfgInline.Sites[0].WAF.Rules)
	}
	if code, _, errOut := runCLI(t, "ruleset", "rule", "remove", "-c", cfgPath, "-d", "app.example.com", "--name", "inline-block"); code != 0 {
		t.Fatalf("inline rule remove failed: %s", errOut)
	}
}
