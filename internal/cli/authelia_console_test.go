package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCLIMinecraftConsoleConfigAndCommand(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "gymbag.json")
	fakeJar := filepath.Join(dir, "paper.jar")

	// 1. init --mc-console enables minecraft.console
	var out, errBuf bytes.Buffer
	code := ExecuteWithIO([]string{
		"init",
		"-o", cfgPath,
		"-t", "minecraft",
		"--server-jar", fakeJar,
		"--mc-upstream", "127.0.0.1:25566",
		"--admin", "127.0.0.1:2019",
		"--mc-console",
		"--no-auto-setup",
		"-y",
	}, strings.NewReader(""), &out, &errBuf)
	if code != 0 {
		t.Fatalf("init --mc-console failed (%d): %s", code, errBuf.String())
	}
	cfg, err := config.ParseFile(cfgPath)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	if cfg.Minecraft == nil || !cfg.Minecraft.Console || cfg.Minecraft.ServerJar != fakeJar {
		t.Fatalf("expected minecraft console enabled with jar, got %+v", cfg.Minecraft)
	}

	// 2. validate & config list show console=on
	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"validate", "-c", cfgPath}, &out, &errBuf); code != 0 {
		t.Fatalf("validate failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "console=on") {
		t.Fatalf("expected validate to print console=on, got:\n%s", out.String())
	}

	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"config", "list", "-c", cfgPath}, &out, &errBuf); code != 0 {
		t.Fatalf("config list failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "console=on") {
		t.Fatalf("expected config list to print console=on, got:\n%s", out.String())
	}

	// 3. gymbag console (--cmd, --no-stream, and interactive SSE + stdin) using mocked Admin API client
	origFactory := consoleHTTPClientFactory
	t.Cleanup(func() { consoleHTTPClientFactory = origFactory })

	var (
		mu       sync.Mutex
		posted   []string
		lastURLs []string
	)
	consoleHTTPClientFactory = func(timeout time.Duration) *http.Client {
		return &http.Client{
			Timeout: timeout,
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				mu.Lock()
				lastURLs = append(lastURLs, req.Method+" "+req.URL.String())
				mu.Unlock()

				if req.Method == http.MethodPost && req.URL.Path == "/api/v1/minecraft/console" {
					raw, _ := io.ReadAll(req.Body)
					var payload struct {
						Command string `json:"command"`
					}
					_ = json.Unmarshal(raw, &payload)
					mu.Lock()
					posted = append(posted, payload.Command)
					mu.Unlock()
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body:       io.NopCloser(strings.NewReader(`{"ok":true,"command":"` + payload.Command + `"}`)),
					}, nil
				}

				if req.Method == http.MethodGet && req.URL.Path == "/api/v1/minecraft/console" {
					if req.URL.Query().Get("stream") == "1" {
						sseBody := "id: 10\nevent: console\ndata: {\"seq\":10,\"stream\":\"stdout\",\"line\":\"[INFO]: Done!\"}\n\n"
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
							Body:       io.NopCloser(strings.NewReader(sseBody)),
						}, nil
					}
					snapJSON := `{"running":true,"pid":123,"console_enabled":true,"lines":[{"seq":1,"stream":"stdout","line":"line-one"},{"seq":2,"stream":"stdout","line":"line-two"}]}`
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body:       io.NopCloser(strings.NewReader(snapJSON)),
					}, nil
				}

				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader(`not found`)),
				}, nil
			}),
		}
	}

	// 3a. --cmd
	out.Reset()
	errBuf.Reset()
	if code := ExecuteWithIO([]string{"console", "-c", cfgPath, "--cmd", "list"}, strings.NewReader(""), &out, &errBuf); code != 0 {
		t.Fatalf("console --cmd failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "Sent command: list") {
		t.Fatalf("unexpected console --cmd stdout: %q", out.String())
	}

	// 3b. --no-stream
	out.Reset()
	errBuf.Reset()
	if code := ExecuteWithIO([]string{"console", "--admin", "127.0.0.1:2019", "--no-stream", "--tail", "50"}, strings.NewReader(""), &out, &errBuf); code != 0 {
		t.Fatalf("console --no-stream failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "line-one\nline-two") {
		t.Fatalf("unexpected console --no-stream stdout: %q", out.String())
	}

	// 3c. interactive SSE + stdin (.exit after sending command)
	pr, pw := io.Pipe()
	wroteSSE := make(chan struct{})
	var wroteOnce sync.Once
	consoleHTTPClientFactory = func(timeout time.Duration) *http.Client {
		return &http.Client{
			Timeout: timeout,
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet && req.URL.Query().Get("stream") == "1" {
					go func() {
						_, _ = io.WriteString(pw, "id: 10\nevent: console\ndata: {\"seq\":10,\"stream\":\"stdout\",\"line\":\"[INFO]: Server ready\"}\n\n")
						wroteOnce.Do(func() { close(wroteSSE) })
					}()
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
						Body:       pr,
					}, nil
				}
				if req.Method == http.MethodPost {
					raw, _ := io.ReadAll(req.Body)
					var payload struct {
						Command string `json:"command"`
					}
					_ = json.Unmarshal(raw, &payload)
					mu.Lock()
					posted = append(posted, payload.Command)
					mu.Unlock()
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
					}, nil
				}
				return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}, nil
			}),
		}
	}

	inR, inW := io.Pipe()
	go func() {
		<-wroteSSE
		_, _ = io.WriteString(inW, "say hi-from-cli\n.exit\n")
		_ = inW.Close()
	}()
	out.Reset()
	errBuf.Reset()
	if code := ExecuteWithIO([]string{"console", "-c", cfgPath}, inR, &out, &errBuf); code != 0 {
		t.Fatalf("interactive console failed (%d): %s", code, errBuf.String())
	}
	_ = pw.Close()
	_ = pr.Close()
	if !strings.Contains(out.String(), "[INFO]: Server ready") {
		t.Fatalf("expected streamed SSE line in stdout, got %q", out.String())
	}

	mu.Lock()
	gotPosted := append([]string(nil), posted...)
	gotURLs := append([]string(nil), lastURLs...)
	mu.Unlock()
	if len(gotPosted) < 2 || gotPosted[0] != "list" || gotPosted[1] != "say hi-from-cli" {
		t.Fatalf("expected posted commands [list, say hi-from-cli], got %v", gotPosted)
	}
	if len(gotURLs) == 0 || !strings.Contains(gotURLs[0], "http://127.0.0.1:2019") {
		t.Fatalf("expected Admin URL resolved from config, got %v", gotURLs)
	}

	// 4. config set mc_console off & config unset mc_console
	out.Reset()
	errBuf.Reset()
	if code := Execute([]string{"config", "set", "-c", cfgPath, "mc_console", "off"}, &out, &errBuf); code != 0 {
		t.Fatalf("config set mc_console off failed (%d): %s", code, errBuf.String())
	}
	cfg, err = config.ParseFile(cfgPath)
	if err != nil || cfg.Minecraft == nil || cfg.Minecraft.Console {
		t.Fatalf("expected mc_console=false, got err=%v mc=%+v", err, cfg.Minecraft)
	}
}
