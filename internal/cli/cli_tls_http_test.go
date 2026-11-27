package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLITLSOffAcmeShAndDirectHTTPFlags(t *testing.T) {
	dir := t.TempDir()

	// 1. validate Gymbagfile with tls off + direct_http
	httpOnlyPath := filepath.Join(dir, "http_only.Gymbagfile")
	httpOnlyRaw := `
{
	tls off
	direct_http on
}
app.example.com {
	reverse_proxy :3000
}
`
	if err := os.WriteFile(httpOnlyPath, []byte(httpOnlyRaw), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	if code := Execute([]string{"validate", "-c", httpOnlyPath}, &out, &errBuf); code != 0 {
		t.Fatalf("validate tls off failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "HTTPS off") || !strings.Contains(out.String(), "SSL provider: off") {
		t.Fatalf("expected HTTPS off & provider off in validate output:\n%s", out.String())
	}

	// 2. validate Gymbagfile with auto_ssl off + acme_sh_path + direct_http on
	acmeShCfg := filepath.Join(dir, "acmesh.Gymbagfile")
	acmeShRaw := `
{
	auto_ssl off
	acme_sh_path /root/.acme.sh
	direct_http on
}
app.example.com {
	reverse_proxy :3000
}
`
	if err := os.WriteFile(acmeShCfg, []byte(acmeShRaw), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"validate", "-c", acmeShCfg}, &out, &errBuf); code != 0 {
		t.Fatalf("validate acme_sh failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "HTTP :80 (direct) + HTTPS :443") || !strings.Contains(out.String(), "SSL provider: acme.sh") {
		t.Fatalf("expected direct + HTTPS and acme.sh provider in validate output:\n%s", out.String())
	}

	// 3. gymbag init --http-only --dry-run (JSON)
	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"init", "-d", "plain.example.com", "-u", ":8080", "--http-only", "--dry-run"}, &out, &errBuf); code != 0 {
		t.Fatalf("init --http-only failed (%d): %s", code, errBuf.String())
	}
	var parsed map[string]any
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("init --http-only invalid JSON: %v\n%s", err, out.String())
	}
	if parsed["direct_http"] != true {
		t.Fatalf("expected direct_http=true in JSON output:\n%s", out.String())
	}
	tlsObj, _ := parsed["tls"].(map[string]any)
	if tlsObj["mode"] != "off" || tlsObj["auto_ssl"] != false {
		t.Fatalf("expected tls.mode=off and auto_ssl=false in JSON:\n%s", out.String())
	}

	// 4. gymbag init --no-auto-ssl --acme-sh-path --direct-http --format gymbagfile --dry-run
	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{
		"init", "--format", "gymbagfile",
		"-d", "site.example.com", "-u", ":3000",
		"--no-auto-ssl", "--acme-sh-path", "/root/.acme.sh", "--direct-http", "--dry-run",
	}, &out, &errBuf); code != 0 {
		t.Fatalf("init --no-auto-ssl --acme-sh-path failed (%d): %s", code, errBuf.String())
	}
	gymbagfileOut := out.String()
	if !strings.Contains(gymbagfileOut, "auto_ssl off") ||
		!strings.Contains(gymbagfileOut, "acme_sh_path /root/.acme.sh") ||
		!strings.Contains(gymbagfileOut, "direct_http on") {
		t.Fatalf("unexpected generated Gymbagfile:\n%s", gymbagfileOut)
	}

	// 5. Help text mentions the new flags
	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"--help"}, &out, &errBuf); code != 0 {
		t.Fatalf("--help failed: %d", code)
	}
	for _, flagName := range []string{"--no-auto-ssl", "--acme-sh-path", "--direct-http", "--http-only"} {
		if !strings.Contains(out.String(), flagName) {
			t.Fatalf("expected %s in --help output", flagName)
		}
	}
}
