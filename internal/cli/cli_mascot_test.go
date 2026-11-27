package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIMascotFlagsAndValidation(t *testing.T) {
	dir := t.TempDir()

	// 1. gymbag init --mascot https://... --dry-run (JSON & Gymbagfile)
	var out, errBuf bytes.Buffer
	if code := Execute([]string{
		"init", "-d", "app.example.com", "-u", ":3000",
		"--mascot", "https://cdn.example.com/mascot.png",
		"--dry-run",
	}, &out, &errBuf); code != 0 {
		t.Fatalf("init --mascot JSON failed (%d): %s", code, errBuf.String())
	}
	var parsed map[string]any
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if parsed["mascot"] != "https://cdn.example.com/mascot.png" {
		t.Fatalf("expected mascot string in JSON, got %v", parsed["mascot"])
	}

	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{
		"init", "--format", "gymbagfile", "-d", "app.example.com", "-u", ":3000",
		"--mascot", "./assets/mascot", "--mascot-reject", "https://cdn.example.com/stop.png",
		"--dry-run",
	}, &out, &errBuf); code != 0 {
		t.Fatalf("init --mascot Gymbagfile failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "default ./assets/mascot") ||
		!strings.Contains(out.String(), "reject https://cdn.example.com/stop.png") {
		t.Fatalf("unexpected Gymbagfile output:\n%s", out.String())
	}

	// 2. gymbag validate succeeds with existing local mascot file and fails on missing local file
	imgPath := filepath.Join(dir, "custom.png")
	if err := os.WriteFile(imgPath, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	goodCfg := filepath.Join(dir, "good.Gymbagfile")
	if err := os.WriteFile(goodCfg, []byte("{\n    mascot "+imgPath+"\n}\napp.example.com {\n    reverse_proxy :3000\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"validate", "-c", goodCfg}, &out, &errBuf); code != 0 {
		t.Fatalf("validate good mascot failed (%d): %s", code, errBuf.String())
	}

	badCfg := filepath.Join(dir, "bad.Gymbagfile")
	if err := os.WriteFile(badCfg, []byte("app.example.com {\n    reverse_proxy :3000\n    mascot ./missing.png\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"validate", "-c", badCfg}, &out, &errBuf); code == 0 {
		t.Fatal("expected validate to fail for missing local mascot file")
	}

	// 3. --help documents --mascot flags
	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"--help"}, &out, &errBuf); code != 0 || !strings.Contains(out.String(), "--mascot") {
		t.Fatalf("expected --mascot in help output")
	}
}
