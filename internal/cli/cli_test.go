package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gymbag/internal/config"
)

func TestCLIVersionValidateAndFmt(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := Execute([]string{"version"}, &out, &errBuf); code != 0 || !strings.Contains(out.String(), "gymbag v") {
		t.Fatalf("version = (%d, %q)", code, out.String())
	}

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "Gymbagfile")
	raw := `
app.example.com {
reverse_proxy :3000
verify 3
}
docs.example.com {
reverse_proxy :4000
protect off
}
`
	if err := os.WriteFile(cfgPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"validate", "-c", cfgPath}, &out, &errBuf); code != 0 {
		t.Fatalf("validate failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "verify: 3 [suspicious & ai]") ||
		!strings.Contains(out.String(), "unprotected") ||
		!strings.Contains(out.String(), "SSL provider: letsencrypt") ||
		!strings.Contains(out.String(), "default_rules: on") {
		t.Fatalf("unexpected validate output:\n%s", out.String())
	}

	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"validate", "-c", cfgPath, "--ssl-provider", "zerossl"}, &out, &errBuf); code != 0 {
		t.Fatalf("validate --ssl-provider zerossl failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "TLS acme") || !strings.Contains(out.String(), "SSL provider: zerossl") {
		t.Fatalf("expected zerossl provider override in validate output, got:\n%s", out.String())
	}

	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"validate", "-c", cfgPath, "--ssl-provider", "invalid-ca"}, &out, &errBuf); code == 0 {
		t.Fatalf("expected non-zero exit code for invalid --ssl-provider, got 0")
	}

	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"--help"}, &out, &errBuf); code != 0 || !strings.Contains(out.String(), "--ssl-provider") {
		t.Fatalf("expected --ssl-provider in help output, got (%d):\n%s", code, out.String())
	}

	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"fmt", "--overwrite", cfgPath}, &out, &errBuf); code != 0 {
		t.Fatalf("fmt --overwrite failed (%d): %s", code, errBuf.String())
	}
	formattedBytes, _ := os.ReadFile(cfgPath)
	if !strings.Contains(string(formattedBytes), "    reverse_proxy http://localhost:3000") ||
		!strings.Contains(string(formattedBytes), "    verify 3") {
		t.Fatalf("unexpected formatted file:\n%s", string(formattedBytes))
	}

	// Verify default_rules off and local domain SSL provider reporting
	localCfgPath := filepath.Join(dir, "local.Gymbagfile")
	localRaw := `
app.internal {
    reverse_proxy :3000
    verify
    default_rules off
}
`
	if err := os.WriteFile(localCfgPath, []byte(localRaw), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"validate", "-c", localCfgPath}, &out, &errBuf); code != 0 {
		t.Fatalf("validate local config failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "SSL provider: local") || !strings.Contains(out.String(), "default_rules: off") {
		t.Fatalf("expected local SSL provider and default_rules: off, got:\n%s", out.String())
	}

	// Verify --ssl-provider named override resets custom ACME directory URL
	customCACfgPath := filepath.Join(dir, "custom-ca.Gymbagfile")
	customCARaw := `
{
    ssl_provider https://acme.custom.example/v2/directory
}
app.example.com {
    reverse_proxy :3000
    verify
}
`
	if err := os.WriteFile(customCACfgPath, []byte(customCARaw), 0o644); err != nil {
		t.Fatal(err)
	}
	overriddenCfg, err := loadConfigWithCLIFlags(config.LoadOptions{ConfigPath: customCACfgPath}, "zerossl")
	if err != nil {
		t.Fatalf("loadConfigWithCLIFlags custom->zerossl failed: %v", err)
	}
	if overriddenCfg.TLS.Provider != "zerossl" || overriddenCfg.TLS.ACMEDirectory != config.DefaultTLSConfig().ACMEDirectory {
		t.Fatalf("expected custom ACME directory reset on named CLI override, got provider=%q dir=%q",
			overriddenCfg.TLS.Provider, overriddenCfg.TLS.ACMEDirectory)
	}

	// Verify init & validate with --verify-timeout and --remote-rules
	initOutPath := filepath.Join(dir, "remote-timeout.Gymbagfile")
	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{
		"init", "-y", "-f",
		"-o", initOutPath,
		"-d", "app.example.com",
		"-u", ":3000",
		"-l", "3",
		"--verify-timeout", "6h",
		"--remote-rules", "https://rules.example.com/waf.rules",
		"--remote-rules-refresh", "10m",
	}, &out, &errBuf); code != 0 {
		t.Fatalf("init with verify-timeout/remote-rules failed (%d): %s", code, errBuf.String())
	}
	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"validate", "-c", initOutPath}, &out, &errBuf); code != 0 {
		t.Fatalf("validate generated config failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "clearance_ttl: 21600s") || !strings.Contains(out.String(), "remote rules: 1") {
		t.Fatalf("expected clearance_ttl and remote rules in validate output, got:\n%s", out.String())
	}
}
