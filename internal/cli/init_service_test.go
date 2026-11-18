package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gymbag/internal/config"
)

func TestCLIInitNonInteractiveAndValidate(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "Gymbagfile")

	var out, errBuf bytes.Buffer
	code := ExecuteWithIO([]string{
		"init",
		"-o", cfgPath,
		"-d", "app.example.com",
		"-u", ":3000",
		"--ssl-provider", "zerossl",
		"--email", "ops@example.com",
		"--admin", "127.0.0.1:2019",
	}, strings.NewReader(""), &out, &errBuf)
	if code != 0 {
		t.Fatalf("init failed (%d): stdout=%s stderr=%s", code, out.String(), errBuf.String())
	}
	if !strings.Contains(out.String(), "Generated "+cfgPath) || !strings.Contains(out.String(), "gymbag --config "+cfgPath) {
		t.Fatalf("unexpected init output:\n%s", out.String())
	}

	// Refuse overwrite without --force
	out.Reset()
	errBuf.Reset()
	if code := ExecuteWithIO([]string{"init", "-o", cfgPath, "-y"}, strings.NewReader(""), &out, &errBuf); code == 0 {
		t.Fatalf("expected non-zero exit when overwriting without --force")
	}

	// Generated config must pass `gymbag validate`
	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"validate", "-c", cfgPath}, &out, &errBuf); code != 0 {
		t.Fatalf("validate generated config failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "app.example.com -> http://localhost:3000") ||
		!strings.Contains(out.String(), "SSL provider: zerossl") ||
		!strings.Contains(out.String(), "verify: 3 [suspicious & ai]") {
		t.Fatalf("unexpected validate output:\n%s", out.String())
	}

	// --dry-run prints to stdout without writing file
	dryPath := filepath.Join(dir, "never-created.Gymbagfile")
	out.Reset()
	errBuf.Reset()
	code = ExecuteWithIO([]string{
		"init", "--dry-run", "-o", dryPath, "--site", "api.example.com=:8080", "--format", "json",
	}, strings.NewReader(""), &out, &errBuf)
	if code != 0 {
		t.Fatalf("init --dry-run failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), `"domain": "api.example.com"`) {
		t.Fatalf("expected JSON config on stdout, got:\n%s", out.String())
	}
	if _, err := os.Stat(dryPath); !os.IsNotExist(err) {
		t.Fatalf("expected --dry-run not to create %s", dryPath)
	}
}

func TestCLIInitTwoCommandDefaultWorkingDir(t *testing.T) {
	dir := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	// Step 1: generate default ./Gymbagfile
	var out, errBuf bytes.Buffer
	if code := ExecuteWithIO([]string{"init", "--yes"}, strings.NewReader(""), &out, &errBuf); code != 0 {
		t.Fatalf("init --yes failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "Start server:     gymbag") {
		t.Fatalf("expected 2nd-command hint `gymbag`, got:\n%s", out.String())
	}

	// Step 2: verify `gymbag` (with no config flag) resolves the generated ./Gymbagfile
	cfg, err := config.Load(config.LoadOptions{})
	if err != nil {
		t.Fatalf("config.Load after init failed: %v", err)
	}
	if len(cfg.Sites) != 1 || cfg.Sites[0].Domain != "localhost" || cfg.Sites[0].PrimaryUpstream() != "http://localhost:5173" {
		t.Fatalf("loaded config mismatch: %+v", cfg.Sites)
	}
}

func TestCLIInitInteractiveWizard(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "Gymbagfile")

	// Simulate interactive answers for a public domain + zerossl + email + custom ports + admin + no service
	answers := strings.Join([]string{
		"blog.example.com", // domain
		":4321",            // upstream
		"2",                // verify level (only ai)
		"zerossl",          // ssl provider
		"me@example.com",   // acme email
		"8080/8443",        // ports
		"y",                // enable admin API
		"none",             // register service
	}, "\n") + "\n"

	var out, errBuf bytes.Buffer
	code := ExecuteWithIO([]string{"init", "-o", cfgPath}, strings.NewReader(answers), &out, &errBuf)
	if code != 0 {
		t.Fatalf("interactive init failed (%d): stdout=%s stderr=%s", code, out.String(), errBuf.String())
	}

	cfg, err := config.ParseFile(cfgPath)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	if cfg.Sites[0].Domain != "blog.example.com" ||
		cfg.Sites[0].PrimaryUpstream() != "http://localhost:4321" ||
		cfg.Sites[0].WAF.VerifyLevel != config.VerifyLevelOnlyAI ||
		cfg.TLS.Provider != "zerossl" ||
		cfg.TLS.ACMEEmail != "me@example.com" ||
		cfg.HTTPPort != 8080 ||
		cfg.HTTPSPort != 8443 ||
		cfg.AdminAddr != "127.0.0.1:2019" {
		t.Fatalf("interactive generated config mismatch: %+v", cfg)
	}
}

func TestCLIServiceRenderInstallControlAndInitWithService(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cfgPath := filepath.Join(dir, "Gymbagfile")
	unitOut := filepath.Join(dir, "gymbag.service")

	var recorded []string
	origRunner := serviceCommandRunner
	serviceCommandRunner = func(_ context.Context, name string, args ...string) ([]byte, error) {
		recorded = append(recorded, name+" "+strings.Join(args, " "))
		return []byte("active"), nil
	}
	t.Cleanup(func() { serviceCommandRunner = origRunner })

	// 1. Combined `gymbag init --service systemd`
	var out, errBuf bytes.Buffer
	code := ExecuteWithIO([]string{
		"init",
		"-o", cfgPath,
		"-d", "app.internal",
		"-u", ":3000",
		"--http-port", "8080",
		"--https-port", "8443",
		"--service", "systemd",
		"--service-name", "gymbag-cli-test",
		"--user",
	}, strings.NewReader(""), &out, &errBuf)
	// Note: `init --service` writes to default user unit path unless tested via `service install -o`,
	// so let's clean up that user unit path if created, OR test `service install -o` directly!
	if code == 0 {
		t.Cleanup(func() {
			_, _ = out.WriteString("")
			_ = Execute([]string{"service", "uninstall", "--type", "systemd", "--name", "gymbag-cli-test", "--user", "--no-reload"}, &bytes.Buffer{}, &bytes.Buffer{})
		})
	} else {
		t.Fatalf("init --service failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "Registered systemd service:") ||
		!strings.Contains(out.String(), "gymbag service start --name gymbag-cli-test --user") {
		t.Fatalf("unexpected init --service output:\n%s", out.String())
	}

	// 2. `gymbag service render --type systemd`
	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"service", "render", "--type", "systemd", "-c", cfgPath, "-o", unitOut}, &out, &errBuf); code != 0 {
		t.Fatalf("service render failed (%d): %s", code, errBuf.String())
	}
	renderedBytes, err := os.ReadFile(unitOut)
	if err != nil {
		t.Fatalf("failed reading rendered unit: %v", err)
	}
	if !strings.Contains(string(renderedBytes), "--config "+cfgPath) || !strings.Contains(string(renderedBytes), "--watch") {
		t.Fatalf("unexpected rendered unit:\n%s", string(renderedBytes))
	}

	// 3. `gymbag service install/start/status/stop/uninstall` with custom -o path
	recorded = nil
	customUnit := filepath.Join(dir, "custom-gymbag.service")
	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{
		"service", "install", "--type", "systemd", "-c", cfgPath, "-o", customUnit, "--name", "custom-gymbag", "--user", "--start",
	}, &out, &errBuf); code != 0 {
		t.Fatalf("service install failed (%d): %s", code, errBuf.String())
	}
	if len(recorded) != 3 ||
		recorded[0] != "systemctl --user daemon-reload" ||
		recorded[1] != "systemctl --user enable custom-gymbag.service" ||
		recorded[2] != "systemctl --user start custom-gymbag.service" {
		t.Fatalf("unexpected systemctl install calls: %v", recorded)
	}

	recorded = nil
	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"service", "status", "--type", "systemd", "--name", "custom-gymbag", "--user"}, &out, &errBuf); code != 0 {
		t.Fatalf("service status failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "active") {
		t.Fatalf("expected status output to contain command output, got: %q", out.String())
	}

	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"service", "uninstall", "--type", "systemd", "-o", customUnit, "--name", "custom-gymbag", "--user"}, &out, &errBuf); code != 0 {
		t.Fatalf("service uninstall failed (%d): %s", code, errBuf.String())
	}
	if _, err := os.Stat(customUnit); !os.IsNotExist(err) {
		t.Fatalf("expected customUnit removed on uninstall")
	}
}
