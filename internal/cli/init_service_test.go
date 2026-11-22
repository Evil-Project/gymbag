package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gymbag/internal/config"
	"gymbag/internal/setup"
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

	// Step 1: generate default ./gymbag.json
	var out, errBuf bytes.Buffer
	if code := ExecuteWithIO([]string{"init", "--yes"}, strings.NewReader(""), &out, &errBuf); code != 0 {
		t.Fatalf("init --yes failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "Start server:     gymbag") {
		t.Fatalf("expected 2nd-command hint `gymbag`, got:\n%s", out.String())
	}
	rawJSON, err := os.ReadFile(filepath.Join(dir, "gymbag.json"))
	if err != nil {
		t.Fatalf("expected ./gymbag.json to be created by default: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(rawJSON, &parsed); err != nil {
		t.Fatalf("expected valid JSON in ./gymbag.json: %v\n%s", err, string(rawJSON))
	}

	// Step 2: verify `gymbag` (with no config flag) resolves the generated ./gymbag.json
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

	// Simulate interactive answers for HTTP + public domain + zerossl + email + custom ports + admin + no service
	answers := strings.Join([]string{
		"http",             // services template
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

func TestCLIInitSSHMinecraftAndCombinedTemplates(t *testing.T) {
	origSSHRunner := sshSetupRunner
	t.Cleanup(func() { sshSetupRunner = origSSHRunner })
	sshSetupRunner = func(ctx context.Context, opts setup.SSHSetupOptions) (*setup.SSHSetupResult, error) {
		opts.GOOS = "linux"
		opts.LookPath = func(file string) (string, error) {
			if slices.Contains([]string{"systemctl", "ufw"}, file) {
				return "/usr/bin/" + file, nil
			}
			return "", errors.New("not found")
		}
		opts.Runner = func(_ context.Context, name string, args ...string) ([]byte, error) {
			if strings.HasSuffix(name, "ufw") && len(args) > 0 && args[0] == "status" {
				return []byte("Status: active\n"), nil
			}
			return []byte("ok"), nil
		}
		return setup.SetupSSHProtection(ctx, opts)
	}

	// 1. SSH template auto-setup + firewall + concise JSON + validate
	t.Run("ssh template with auto sshd_config and ufw firewall", func(t *testing.T) {
		dir := t.TempDir()
		sshdPath := filepath.Join(dir, "sshd_config")
		if err := os.WriteFile(sshdPath, []byte("Port 22\nPermitRootLogin prohibit-password\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		outCfgPath := filepath.Join(dir, "ssh.json")

		var out, errBuf bytes.Buffer
		code := ExecuteWithIO([]string{
			"init", "-t", "ssh", "--sshd-config", sshdPath, "-o", outCfgPath,
		}, strings.NewReader(""), &out, &errBuf)
		if code != 0 {
			t.Fatalf("init -t ssh failed (%d): stdout=%s stderr=%s", code, out.String(), errBuf.String())
		}
		if !strings.Contains(out.String(), "Configured firewall (ufw)") {
			t.Fatalf("expected ufw firewall summary, got:\n%s", out.String())
		}

		updatedSSHD, _ := os.ReadFile(sshdPath)
		if !strings.Contains(string(updatedSSHD), "# Port 22 # disabled by gymbag") {
			t.Fatalf("expected sshd_config Port 22 commented out:\n%s", string(updatedSSHD))
		}
		if _, err := os.Stat(sshdPath + ".gymbag.bak"); err != nil {
			t.Fatalf("expected sshd_config backup: %v", err)
		}

		rawJSON, err := os.ReadFile(outCfgPath)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := json.Unmarshal(rawJSON, &doc); err != nil {
			t.Fatal(err)
		}
		if _, hasSites := doc["sites"]; hasSites {
			t.Fatalf("expected concise SSH-only config without sites key:\n%s", string(rawJSON))
		}
		if _, hasDefaults := doc["defaults"]; hasDefaults {
			t.Fatalf("expected concise SSH-only config without defaults boilerplate:\n%s", string(rawJSON))
		}
		if _, hasSSH := doc["ssh"]; !hasSSH {
			t.Fatalf("expected ssh block in generated config:\n%s", string(rawJSON))
		}

		out.Reset()
		errBuf.Reset()
		if code := Execute([]string{"validate", "-c", outCfgPath}, &out, &errBuf); code != 0 {
			t.Fatalf("validate ssh config failed (%d): %s", code, errBuf.String())
		}
		if !strings.Contains(out.String(), "SSH: true") || !strings.Contains(out.String(), "verify_portal port=8443") {
			t.Fatalf("unexpected validate output:\n%s", out.String())
		}
	})

	// 2. Minecraft template auto-setup + server.properties rewrite + concise JSON
	t.Run("minecraft template with auto server.properties randomization", func(t *testing.T) {
		dir := t.TempDir()
		propsPath := filepath.Join(dir, "server.properties")
		if err := os.WriteFile(propsPath, []byte("server-ip=\nserver-port=25565\nonline-mode=true\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		outCfgPath := filepath.Join(dir, "mc.json")

		var out, errBuf bytes.Buffer
		code := ExecuteWithIO([]string{
			"init", "-t", "minecraft", "--server-properties", propsPath, "-o", outCfgPath,
		}, strings.NewReader(""), &out, &errBuf)
		if code != 0 {
			t.Fatalf("init -t minecraft failed (%d): stdout=%s stderr=%s", code, out.String(), errBuf.String())
		}

		updatedProps, _ := os.ReadFile(propsPath)
		if strings.Contains(string(updatedProps), "server-port=25565") || !strings.Contains(string(updatedProps), "server-ip=127.0.0.1") {
			t.Fatalf("expected randomized server-port and loopback bind in server.properties:\n%s", string(updatedProps))
		}
		if _, err := os.Stat(propsPath + ".gymbag.bak"); err != nil {
			t.Fatalf("expected server.properties backup: %v", err)
		}

		cfg, err := config.ParseFile(outCfgPath)
		if err != nil {
			t.Fatalf("ParseFile mc.json: %v", err)
		}
		if !cfg.MinecraftEnabled() || cfg.Minecraft.Port != 25565 || !strings.HasPrefix(cfg.Minecraft.Upstream, "127.0.0.1:") {
			t.Fatalf("unexpected minecraft config: %+v", cfg.Minecraft)
		}
		if cfg.Verify.EffectivePort() != 8443 {
			t.Fatalf("expected verify port 8443, got %d", cfg.Verify.EffectivePort())
		}

		out.Reset()
		errBuf.Reset()
		if code := Execute([]string{"validate", "-c", outCfgPath}, &out, &errBuf); code != 0 {
			t.Fatalf("validate mc config failed (%d): %s", code, errBuf.String())
		}
		if !strings.Contains(out.String(), "Minecraft: true") || !strings.Contains(out.String(), "minecraft :25565 -> 127.0.0.1:") {
			t.Fatalf("unexpected validate output:\n%s", out.String())
		}
	})

	// 3. Combined HTTP + SSH + Minecraft in one concise config
	t.Run("combined http ssh minecraft template", func(t *testing.T) {
		dir := t.TempDir()
		outCfgPath := filepath.Join(dir, "all.json")

		var out, errBuf bytes.Buffer
		code := ExecuteWithIO([]string{
			"init",
			"-t", "http,ssh,minecraft",
			"-d", "app.example.com",
			"-u", ":3000",
			"--ssh-upstream", "127.0.0.1:2222",
			"--mc-upstream", "127.0.0.1:25566",
			"--verify-url", "https://verify.example.com:8443",
			"-o", outCfgPath,
		}, strings.NewReader(""), &out, &errBuf)
		if code != 0 {
			t.Fatalf("init all template failed (%d): stdout=%s stderr=%s", code, out.String(), errBuf.String())
		}

		cfg, err := config.ParseFile(outCfgPath)
		if err != nil {
			t.Fatalf("ParseFile all.json: %v", err)
		}
		if !cfg.HTTPEnabled() || !cfg.SSHEnabled() || !cfg.MinecraftEnabled() {
			t.Fatalf("expected all 3 services enabled: http=%t ssh=%t mc=%t", cfg.HTTPEnabled(), cfg.SSHEnabled(), cfg.MinecraftEnabled())
		}
		if cfg.Verify.URL != "https://verify.example.com:8443" {
			t.Fatalf("verify URL = %q", cfg.Verify.URL)
		}

		out.Reset()
		errBuf.Reset()
		if code := Execute([]string{"config", "list", "-c", outCfgPath}, &out, &errBuf); code != 0 {
			t.Fatalf("config list failed (%d): %s", code, errBuf.String())
		}
		if !strings.Contains(out.String(), "ssh=true, minecraft=true") || !strings.Contains(out.String(), "verify_portal port=8443") {
			t.Fatalf("unexpected config list output:\n%s", out.String())
		}
	})

	// 4. Interactive wizard for ssh,minecraft with explicit upstreams
	t.Run("interactive wizard ssh and minecraft", func(t *testing.T) {
		dir := t.TempDir()
		outCfgPath := filepath.Join(dir, "wizard-tcp.json")
		answers := strings.Join([]string{
			"ssh,minecraft",               // services
			"2200",                        // ssh listen port
			"127.0.0.1:2201",              // ssh upstream
			"25570",                       // minecraft listen port
			"127.0.0.1:25571",             // minecraft upstream
			"1",                           // verify level (only suspicious)
			"https://gate.example.com:9443", // verify URL
			"9443",                        // verify port
			"letsencrypt",                 // ssl provider (because verify URL host is public DNS)
			"ops@example.com",             // acme email
			"n",                           // admin API
			"none",                        // service
		}, "\n") + "\n"

		var out, errBuf bytes.Buffer
		code := ExecuteWithIO([]string{"init", "-o", outCfgPath}, strings.NewReader(answers), &out, &errBuf)
		if code != 0 {
			t.Fatalf("interactive tcp init failed (%d): stdout=%s stderr=%s", code, out.String(), errBuf.String())
		}

		cfg, err := config.ParseFile(outCfgPath)
		if err != nil {
			t.Fatalf("ParseFile wizard-tcp.json: %v", err)
		}
		if cfg.HTTPEnabled() || !cfg.SSHEnabled() || !cfg.MinecraftEnabled() {
			t.Fatalf("unexpected service flags: %+v", cfg)
		}
		if cfg.SSH.Port != 2200 || cfg.SSH.Upstream != "127.0.0.1:2201" || cfg.SSH.VerifyLevel != config.VerifyLevelOnlySuspicious {
			t.Fatalf("unexpected SSH config: %+v", cfg.SSH)
		}
		if cfg.Minecraft.Port != 25570 || cfg.Minecraft.Upstream != "127.0.0.1:25571" {
			t.Fatalf("unexpected Minecraft config: %+v", cfg.Minecraft)
		}
		if cfg.Verify.URL != "https://gate.example.com:9443" || cfg.Verify.EffectivePort() != 9443 {
			t.Fatalf("unexpected Verify config: %+v", cfg.Verify)
		}
	})
}
