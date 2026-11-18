package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateConfigDefaultsAndPresets(t *testing.T) {
	// Default standard preset
	raw, cfg, err := RenderGeneratedConfig(GenerateOptions{IncludeComments: true})
	if err != nil {
		t.Fatalf("RenderGeneratedConfig default failed: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "localhost {") ||
		!strings.Contains(text, "reverse_proxy http://localhost:5173") ||
		!strings.Contains(text, "verify 3") ||
		!strings.Contains(text, "certs/ca/neuro-sama-root-ca.pem") {
		t.Fatalf("unexpected default generated config:\n%s", text)
	}
	if cfg.Sites[0].Domain != "localhost" || cfg.Sites[0].WAF.VerifyLevel != VerifyLevelSuspiciousAndAI {
		t.Fatalf("unexpected generated Config struct: %+v", cfg.Sites[0])
	}

	// Local preset
	rawLocal, localCfg, err := RenderGeneratedConfig(GenerateOptions{
		Preset:          "local",
		CertDir:         "/var/lib/gymbag/certs",
		IncludeComments: true,
	})
	if err != nil {
		t.Fatalf("RenderGeneratedConfig local failed: %v", err)
	}
	if localCfg.Sites[0].Domain != "app.internal" || localCfg.TLS.Provider != "local" || localCfg.TLS.Mode != "local" {
		t.Fatalf("unexpected local preset config: sites=%+v tls=%+v", localCfg.Sites, localCfg.TLS)
	}
	if !strings.Contains(string(rawLocal), "/var/lib/gymbag/certs/ca/neuro-sama-root-ca.pem") {
		t.Fatalf("expected custom cert_dir in comment:\n%s", string(rawLocal))
	}

	// Public preset requires domain
	if _, _, err := RenderGeneratedConfig(GenerateOptions{Preset: "public"}); err == nil {
		t.Fatalf("expected error for public preset without domain")
	}

	// Proxy-only preset
	rawProxy, proxyCfg, err := RenderGeneratedConfig(GenerateOptions{
		Preset:   "proxy-only",
		Domain:   "static.example.com",
		Upstream: ":9000",
	})
	if err != nil {
		t.Fatalf("proxy-only preset failed: %v", err)
	}
	if proxyCfg.Sites[0].Protect || !strings.Contains(string(rawProxy), "protect off") {
		t.Fatalf("expected protect off in proxy-only config:\n%s", string(rawProxy))
	}
}

func TestGenerateConfigMultiSiteAndJSON(t *testing.T) {
	specs1, err := ParseSiteSpec("app.example.com=http://localhost:3000")
	if err != nil {
		t.Fatalf("ParseSiteSpec 1 failed: %v", err)
	}
	specs2, err := ParseSiteSpec("api.example.com=:8080,:8081")
	if err != nil {
		t.Fatalf("ParseSiteSpec 2 failed: %v", err)
	}
	allSpecs := append(specs1, specs2...)

	rawGym, _, err := RenderGeneratedConfig(GenerateOptions{
		Sites:            allSpecs,
		SSLProvider:      "zerossl",
		Email:            "ops@example.com",
		AdminAddr:        "127.0.0.1:2019",
		HTTPPort:         8080,
		HTTPSPort:        8443,
		IPSecurityAction: "block",
		VerifyLevel:      "2",
	})
	if err != nil {
		t.Fatalf("multi-site Gymbagfile generation failed: %v", err)
	}
	parsedGym, err := ParseCaddyfile(string(rawGym))
	if err != nil {
		t.Fatalf("ParseCaddyfile on generated multi-site failed: %v\n%s", err, string(rawGym))
	}
	if len(parsedGym.Sites) != 2 ||
		parsedGym.Sites[0].Domain != "app.example.com" ||
		len(parsedGym.Sites[1].Upstreams) != 2 ||
		parsedGym.TLS.Provider != "zerossl" ||
		parsedGym.TLS.ACMEEmail != "ops@example.com" ||
		parsedGym.AdminAddr != "127.0.0.1:2019" ||
		parsedGym.HTTPPort != 8080 ||
		parsedGym.HTTPSPort != 8443 ||
		parsedGym.Intel.IPSecurityAction != "block" ||
		parsedGym.Sites[0].WAF.VerifyLevel != VerifyLevelOnlyAI {
		t.Fatalf("parsed multi-site mismatch: %+v", parsedGym)
	}

	rawJSON, _, err := RenderGeneratedConfig(GenerateOptions{
		Sites:  allSpecs,
		Format: "json",
	})
	if err != nil {
		t.Fatalf("JSON generation failed: %v", err)
	}
	tmpJSON := filepath.Join(t.TempDir(), "gymbag.json")
	if err := os.WriteFile(tmpJSON, rawJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	parsedJSON, err := ParseFile(tmpJSON)
	if err != nil {
		t.Fatalf("ParseFile on generated JSON failed: %v", err)
	}
	if len(parsedJSON.Sites) != 2 || parsedJSON.Sites[0].Domain != "app.example.com" {
		t.Fatalf("JSON roundtrip mismatch: %+v", parsedJSON.Sites)
	}
}
