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

func TestGenerateConfigConciseTemplates(t *testing.T) {
	// 1. Concise HTTP JSON config omits boilerplate defaults
	httpJSON, _, err := RenderGeneratedConfig(GenerateOptions{
		Templates: []string{"http"},
		Format:    "json",
	})
	if err != nil {
		t.Fatalf("RenderGeneratedConfig http json failed: %v", err)
	}
	httpStr := string(httpJSON)
	for _, unwanted := range []string{`"tls"`, `"intel"`, `"defaults"`, `"bind_host"`, `"http_port"`, `"https_port"`, `"challenge_ttl"`} {
		if strings.Contains(httpStr, unwanted) {
			t.Fatalf("expected concise HTTP JSON without %s, got:\n%s", unwanted, httpStr)
		}
	}

	// 2. SSH-only concise JSON template
	sshJSON, sshCfg, err := RenderGeneratedConfig(GenerateOptions{
		Templates: []string{"ssh"},
		SSH:       &TCPSpec{Upstream: "127.0.0.1:44422"},
		Format:    "json",
	})
	if err != nil {
		t.Fatalf("RenderGeneratedConfig ssh json failed: %v", err)
	}
	if sshCfg.HTTPEnabled() || !sshCfg.SSHEnabled() || sshCfg.SSH.Port != 22 || sshCfg.SSH.Upstream != "127.0.0.1:44422" {
		t.Fatalf("unexpected SSH template config: %+v", sshCfg)
	}
	if strings.Contains(string(sshJSON), `"sites"`) {
		t.Fatalf("expected no sites in SSH-only JSON:\n%s", string(sshJSON))
	}

	// 3. Minecraft-only concise JSON template
	mcJSON, mcCfg, err := RenderGeneratedConfig(GenerateOptions{
		Templates: []string{"minecraft"},
		Minecraft: &TCPSpec{Upstream: "127.0.0.1:45565", ServerProperties: "./server.properties"},
		Format:    "json",
	})
	if err != nil {
		t.Fatalf("RenderGeneratedConfig minecraft json failed: %v", err)
	}
	if mcCfg.HTTPEnabled() || !mcCfg.MinecraftEnabled() || mcCfg.Minecraft.Port != 25565 || mcCfg.Minecraft.Upstream != "127.0.0.1:45565" {
		t.Fatalf("unexpected Minecraft template config: %+v", mcCfg)
	}
	if strings.Contains(string(mcJSON), `"sites"`) {
		t.Fatalf("expected no sites in MC-only JSON:\n%s", string(mcJSON))
	}

	// 4. Combined HTTP + SSH + Minecraft in one app
	allRaw, allCfg, err := RenderGeneratedConfig(GenerateOptions{
		Templates: []string{"http", "ssh", "minecraft"},
		Domain:    "play.example.com",
		Upstream:  "http://127.0.0.1:3000",
		SSH:       &TCPSpec{Upstream: "127.0.0.1:44422"},
		Minecraft: &TCPSpec{Upstream: "127.0.0.1:45565"},
		Verify:    VerifyServerConfig{URL: "https://verify.example.com", Port: 8443},
		Format:    "json",
	})
	if err != nil {
		t.Fatalf("RenderGeneratedConfig all-in-one failed: %v", err)
	}
	if !allCfg.HTTPEnabled() || !allCfg.SSHEnabled() || !allCfg.MinecraftEnabled() {
		t.Fatalf("expected all 3 services enabled: %+v", allCfg)
	}
	reparsedAll, err := ParseConfigBytes(allRaw, "gymbag.json", "")
	if err != nil {
		t.Fatalf("ParseConfigBytes all-in-one failed: %v\n%s", err, string(allRaw))
	}
	if !reparsedAll.SSHEnabled() || !reparsedAll.MinecraftEnabled() || reparsedAll.Verify.URL != "https://verify.example.com" {
		t.Fatalf("reparsed all-in-one mismatch: %+v", reparsedAll)
	}
}
