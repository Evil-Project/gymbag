package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMascotDirectivesBlockAndFormatRoundTrip(t *testing.T) {
	cfg, err := ParseCaddyfile(`
{
	mascot ./assets/global-mascot.png
}

one.example.com {
	reverse_proxy :3000
	mascot {
		default https://cdn.example.com/site-default.webp
		pensive ./assets/one-checking.png
		happy https://cdn.example.com/one-happy.png
		reject /srv/mascots/one-reject.svg
	}
}

two.example.com {
	reverse_proxy :3001
	mascot_pensive ./assets/two-pensive.png
	mascot_happy ./assets/two-happy.png
	mascot_reject ./assets/two-reject.png
}
`)
	if err != nil {
		t.Fatalf("ParseCaddyfile mascot failed: %v", err)
	}
	if cfg.Mascot == nil || cfg.Mascot.Default != "./assets/global-mascot.png" {
		t.Fatalf("unexpected global mascot: %+v", cfg.Mascot)
	}
	if len(cfg.Sites) != 2 {
		t.Fatalf("expected 2 sites, got %d", len(cfg.Sites))
	}
	m1 := cfg.Sites[0].Mascot
	if m1 == nil || m1.Default != "https://cdn.example.com/site-default.webp" ||
		m1.Pensive != "./assets/one-checking.png" ||
		m1.Happy != "https://cdn.example.com/one-happy.png" ||
		m1.Reject != "/srv/mascots/one-reject.svg" {
		t.Fatalf("unexpected site 1 mascot: %+v", m1)
	}
	m2 := cfg.Sites[1].Mascot
	if m2 == nil || m2.Pensive != "./assets/two-pensive.png" || m2.Happy != "./assets/two-happy.png" || m2.Reject != "./assets/two-reject.png" {
		t.Fatalf("unexpected site 2 mascot: %+v", m2)
	}

	formatted := FormatConfig(cfg)
	if !strings.Contains(formatted, "    mascot ./assets/global-mascot.png") ||
		!strings.Contains(formatted, "    mascot {\n") ||
		!strings.Contains(formatted, "        reject /srv/mascots/one-reject.svg") {
		t.Fatalf("unexpected FormatConfig output:\n%s", formatted)
	}

	reparsed, err := ParseCaddyfile(formatted)
	if err != nil {
		t.Fatalf("re-parse FormatConfig mascot failed: %v", err)
	}
	if reparsed.Mascot.Default != cfg.Mascot.Default ||
		reparsed.Sites[0].Mascot.Happy != cfg.Sites[0].Mascot.Happy ||
		reparsed.Sites[1].Mascot.Reject != cfg.Sites[1].Mascot.Reject {
		t.Fatalf("round-trip mismatch")
	}
}

func TestMascotJSONStringAndObjectAndSecurityValidation(t *testing.T) {
	// Concise string form globally + structured form per site
	rawJSON := `{
  "mascot": "https://cdn.example.com/global.png",
  "sites": [
    {
      "domain": "app.example.com",
      "upstreams": ["http://127.0.0.1:3000"],
      "mascot": {
        "default": "./local-dir",
        "reject": "https://cdn.example.com/stop.png"
      }
    }
  ]
}`
	cfg, err := ParseConfigBytes([]byte(rawJSON), filepath.Join(t.TempDir(), "gymbag.json"), "")
	if err != nil {
		t.Fatalf("ParseConfigBytes JSON failed: %v", err)
	}
	if cfg.Mascot == nil || cfg.Mascot.Default != "https://cdn.example.com/global.png" {
		t.Fatalf("unexpected JSON global mascot: %+v", cfg.Mascot)
	}
	if cfg.Sites[0].Mascot == nil || cfg.Sites[0].Mascot.Default != "./local-dir" || cfg.Sites[0].Mascot.Reject != "https://cdn.example.com/stop.png" {
		t.Fatalf("unexpected JSON site mascot: %+v", cfg.Sites[0].Mascot)
	}

	conciseBytes, err := MarshalConciseJSONConfig(cfg)
	if err != nil {
		t.Fatalf("MarshalConciseJSONConfig failed: %v", err)
	}
	if !strings.Contains(string(conciseBytes), `"mascot": "https://cdn.example.com/global.png"`) {
		t.Fatalf("expected single-default mascot marshaled as concise string:\n%s", string(conciseBytes))
	}
	reparsed, err := ParseConfigBytes(conciseBytes, filepath.Join(t.TempDir(), "out.json"), "")
	if err != nil {
		t.Fatalf("re-parse concise JSON failed: %v", err)
	}
	if reparsed.Mascot.Default != "https://cdn.example.com/global.png" ||
		reparsed.Sites[0].Mascot.Reject != "https://cdn.example.com/stop.png" {
		t.Fatalf("concise JSON round-trip mismatch")
	}

	// Unsafe URI schemes must be rejected
	for _, bad := range []string{
		"javascript:alert(1)",
		"data:image/png;base64,AAAA",
		"vbscript:msgbox(1)",
		"file:///etc/passwd",
		"ftp://example.com/mascot.png",
	} {
		c := Default()
		c.Mascot = &MascotConfig{Default: bad}
		if err := c.Validate(); err == nil {
			t.Fatalf("expected validation error for unsafe mascot URL %q", bad)
		}
	}
}

func TestMascotEnvAndLoadOptionsOverrides(t *testing.T) {
	env := map[string]string{
		"GYMBAG_DOMAIN":         "env.example.com",
		"GYMBAG_UPSTREAM":       ":3000",
		"GYMBAG_MASCOT":         "https://cdn.example.com/env-default.png",
		"GYMBAG_MASCOT_PENSIVE": "./env-pensive.webp",
		"GYMBAG_MASCOT_HAPPY":   "https://cdn.example.com/env-happy.png",
		"GYMBAG_MASCOT_REJECT":  "./env-reject.svg",
	}
	cfg, err := LoadWithEnv(LoadOptions{
		MascotHappy: "https://cdn.example.com/cli-happy.png",
	}, func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("LoadWithEnv failed: %v", err)
	}
	if cfg.Mascot == nil ||
		cfg.Mascot.Default != "https://cdn.example.com/env-default.png" ||
		cfg.Mascot.Pensive != "./env-pensive.webp" ||
		cfg.Mascot.Happy != "https://cdn.example.com/cli-happy.png" ||
		cfg.Mascot.Reject != "./env-reject.svg" {
		t.Fatalf("unexpected mascot overrides: %+v", cfg.Mascot)
	}
}
