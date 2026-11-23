package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"gymbag/internal/config"
)

func TestCLIAutheliaInitConfigAndValidate(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "gymbag.json")

	var out, errBuf bytes.Buffer
	code := ExecuteWithIO([]string{
		"init",
		"-o", cfgPath,
		"-d", "app.example.com",
		"-u", ":3000",
		"--authelia", "http://127.0.0.1:9091",
		"--authelia-portal", "https://auth.example.com",
		"-y",
	}, strings.NewReader(""), &out, &errBuf)
	if code != 0 {
		t.Fatalf("init with --authelia failed (%d): %s", code, errBuf.String())
	}

	cfg, err := config.ParseFile(cfgPath)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	if cfg.Authelia == nil || cfg.Authelia.URL != "http://127.0.0.1:9091" || cfg.Authelia.PortalURL != "https://auth.example.com" {
		t.Fatalf("unexpected global authelia: %+v", cfg.Authelia)
	}
	if len(cfg.Sites) != 1 || cfg.Sites[0].Auth == nil || cfg.Sites[0].Auth.URL != "http://127.0.0.1:9091" {
		t.Fatalf("unexpected site authelia: %+v", cfg.Sites)
	}

	// validate prints auth summary
	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"validate", "-c", cfgPath}, &out, &errBuf); code != 0 {
		t.Fatalf("validate failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "auth: authelia (http://127.0.0.1:9091)") {
		t.Fatalf("expected validate to print auth summary, got:\n%s", out.String())
	}

	// config add with --authelia
	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{
		"config", "add", "-c", cfgPath,
		"-d", "admin.example.com", "-u", ":8080",
		"--authelia", "http://127.0.0.1:9091",
	}, &out, &errBuf); code != 0 {
		t.Fatalf("config add failed (%d): %s", code, errBuf.String())
	}

	// config list shows auth=
	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"config", "list", "-c", cfgPath}, &out, &errBuf); code != 0 {
		t.Fatalf("config list failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "auth=http://127.0.0.1:9091") {
		t.Fatalf("expected config list to include auth=, got:\n%s", out.String())
	}

	// config update --clear-authelia removes site auth
	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{
		"config", "update", "-c", cfgPath,
		"-d", "admin.example.com", "--clear-authelia",
	}, &out, &errBuf); code != 0 {
		t.Fatalf("config update --clear-authelia failed (%d): %s", code, errBuf.String())
	}
	cfg, err = config.ParseFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sites[1].Domain != "admin.example.com" || cfg.Sites[1].Auth != nil {
		t.Fatalf("expected admin.example.com auth cleared, got %+v", cfg.Sites[1].Auth)
	}

	// config set / unset authelia
	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"config", "set", "-c", cfgPath, "authelia", "http://10.0.0.9:9091"}, &out, &errBuf); code != 0 {
		t.Fatalf("config set authelia failed (%d): %s", code, errBuf.String())
	}
	cfg, err = config.ParseFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Authelia == nil || cfg.Authelia.URL != "http://10.0.0.9:9091" {
		t.Fatalf("expected updated global authelia URL, got %+v", cfg.Authelia)
	}

	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"config", "unset", "-c", cfgPath, "authelia"}, &out, &errBuf); code != 0 {
		t.Fatalf("config unset authelia failed (%d): %s", code, errBuf.String())
	}
	cfg, err = config.ParseFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Authelia != nil {
		t.Fatalf("expected global authelia unset, got %+v", cfg.Authelia)
	}
}
