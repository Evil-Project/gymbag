package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gymbag/internal/config"
)

func TestRuntimeRoutesExecution(t *testing.T) {
	parentDir := t.TempDir()
	staticRoot := filepath.Join(parentDir, "public")
	browseRoot := filepath.Join(parentDir, "browse")
	if err := os.MkdirAll(filepath.Join(staticRoot, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(browseRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parentDir, "secret.txt"), []byte("TOP_SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticRoot, "index.html"), []byte("<html>spa-index</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticRoot, "assets", "app.js"), []byte("console.log('ok');"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(browseRoot, "report.txt"), []byte("report-body"), 0o644); err != nil {
		t.Fatal(err)
	}

	defOff := false
	cfg := config.Default()
	cfg.Secret = "test-routes-secret"
	cfg.TLS.Mode = "selfsigned"
	cfg.TLS.CertDir = t.TempDir()
	cfg.Intel.IPSecurity = false
	cfg.Intel.LiveReputation = false
	cfg.Intel.AIIPList = false
	cfg.Sites = []config.SiteConfig{
		{
			Domain:  "routes.example.com",
			Protect: false,
			Headers: map[string]string{
				"X-Site":   "on",
				"+X-Multi": "site",
				"-Server":  "",
			},
			WAF: config.SiteWAFConfig{
				VerifyLevel:  config.VerifyLevelAlways,
				DefaultRules: &defOff,
			},
			Routes: []config.RouteConfig{
				{
					Path:        "/api/*",
					StripPrefix: "/api",
					Upstreams:   []string{"http://api-backend:8080"},
					Headers: map[string]string{
						"X-Route":  "api",
						"+X-Multi": "route",
					},
				},
				{
					Path: "/healthz",
					Respond: &config.StaticResponse{
						Status: http.StatusCreated,
						Body:   "ok",
					},
					Headers: map[string]string{
						"X-Health": "yes",
					},
				},
				{
					Path:        "/old/*",
					StripPrefix: "/old",
					Redirect: &config.RedirectTarget{
						To:   "https://{host}/new{uri}",
						Code: http.StatusMovedPermanently,
					},
				},
				{
					Path:        "/browse/*",
					StripPrefix: "/browse",
					Root:        browseRoot,
					Browse:      true,
				},
				{
					Path:        "/static/*",
					StripPrefix: "/static",
					Root:        staticRoot,
					Index:       []string{"index.html"},
					Browse:      true,
					Fallback:    "/index.html",
				},
			},
		},
	}

	var gotUpstreamURI string
	rt, err := BuildRuntime(context.Background(), cfg, RuntimeOptions{
		HTTPTransport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			gotUpstreamURI = req.URL.RequestURI()
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
					"Server":       []string{"upstream-hidden"},
					"X-Multi":      []string{"upstream"},
				},
				Body: io.NopCloser(strings.NewReader(`{"proxied":true}`)),
			}, nil
		}),
	})
	if err != nil {
		t.Fatalf("BuildRuntime failed: %v", err)
	}
	defer rt.Close()

	send := func(method, reqURI string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "https://routes.example.com"+reqURI, nil)
		req.Host = "routes.example.com"
		req.RemoteAddr = "203.0.113.10:12345"
		addBrowserHeaders(req)
		rec := httptest.NewRecorder()
		rt.HTTPSHandler.ServeHTTP(rec, req)
		return rec
	}

	// 1. Proxy + StripPrefix + site/route header mutations (-Server, +X-Multi, X-Site, X-Route)
	recAPI := send("GET", "/api/users?a=1")
	if recAPI.Code != http.StatusOK {
		t.Fatalf("GET /api/users status = %d, want 200 (body=%s)", recAPI.Code, recAPI.Body.String())
	}
	if gotUpstreamURI != "/users?a=1" {
		t.Fatalf("upstream RequestURI = %q, want /users?a=1", gotUpstreamURI)
	}
	if recAPI.Header().Get("X-Site") != "on" || recAPI.Header().Get("X-Route") != "api" {
		t.Fatalf("expected X-Site=on and X-Route=api headers, got: %+v", recAPI.Header())
	}
	if recAPI.Header().Get("Server") != "" {
		t.Fatalf("expected Server header deleted by -Server, got %q", recAPI.Header().Get("Server"))
	}
	multiVals := recAPI.Header().Values("X-Multi")
	if len(multiVals) < 2 {
		t.Fatalf("expected appended +X-Multi values, got %v", multiVals)
	}

	// 2. Static respond route
	recHealth := send("GET", "/healthz")
	if recHealth.Code != http.StatusCreated || recHealth.Body.String() != "ok" {
		t.Fatalf("GET /healthz = (%d, %q), want (201, %q)", recHealth.Code, recHealth.Body.String(), "ok")
	}
	if recHealth.Header().Get("X-Health") != "yes" || recHealth.Header().Get("X-Site") != "on" {
		t.Fatalf("expected X-Health and X-Site on respond route, got %+v", recHealth.Header())
	}

	// 3. Redirect with placeholders + StripPrefix
	recRedir := send("GET", "/old/page?x=1")
	if recRedir.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /old/page status = %d, want 301", recRedir.Code)
	}
	if loc := recRedir.Header().Get("Location"); loc != "https://routes.example.com/new/page?x=1" {
		t.Fatalf("Location = %q, want https://routes.example.com/new/page?x=1", loc)
	}

	// 4. Static file_server: index, file, SPA fallback, browse listing, traversal rejection
	if recIdx := send("GET", "/static/"); recIdx.Code != http.StatusOK || !strings.Contains(recIdx.Body.String(), "spa-index") {
		t.Fatalf("GET /static/ = (%d, %q), want 200 with spa-index", recIdx.Code, recIdx.Body.String())
	}
	if recJS := send("GET", "/static/assets/app.js"); recJS.Code != http.StatusOK || !strings.Contains(recJS.Body.String(), "console.log") {
		t.Fatalf("GET /static/assets/app.js = (%d, %q)", recJS.Code, recJS.Body.String())
	}
	if recSPA := send("GET", "/static/missing/route"); recSPA.Code != http.StatusOK || !strings.Contains(recSPA.Body.String(), "spa-index") {
		t.Fatalf("GET /static/missing/route SPA fallback = (%d, %q)", recSPA.Code, recSPA.Body.String())
	}
	if recBrowse := send("GET", "/browse/"); recBrowse.Code != http.StatusOK || !strings.Contains(recBrowse.Body.String(), "report.txt") {
		t.Fatalf("GET /browse/ listing = (%d, %q), want report.txt link", recBrowse.Code, recBrowse.Body.String())
	}

	// Encoded traversal outside staticRoot must never leak parentDir/secret.txt
	reqTrav := httptest.NewRequest("GET", "https://routes.example.com/static/../secret.txt", nil)
	reqTrav.Host = "routes.example.com"
	reqTrav.RequestURI = "/static/..%2fsecret.txt"
	reqTrav.RemoteAddr = "203.0.113.10:12345"
	recTrav := httptest.NewRecorder()
	rt.HTTPSHandler.ServeHTTP(recTrav, reqTrav)
	if recTrav.Code == http.StatusOK && strings.Contains(recTrav.Body.String(), "TOP_SECRET") {
		t.Fatalf("path traversal leaked parent file secret.txt!")
	}

	// 5. Unmatched path on route-only site returns 404
	if recMiss := send("GET", "/unmatched"); recMiss.Code != http.StatusNotFound {
		t.Fatalf("GET /unmatched status = %d, want 404", recMiss.Code)
	}
}

func TestDynamicAndStaticLocalRuleSources(t *testing.T) {
	dir := t.TempDir()
	dynFile := filepath.Join(dir, "dynamic.rules")
	staticFile := filepath.Join(dir, "static.rules")
	dynDir := filepath.Join(dir, "rules.d")
	if err := os.MkdirAll(dynDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dynFile, []byte("block ua *InitialBot*\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staticFile, []byte("block ua *StaticBot*\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dynDir, "01-base.rules"), []byte("block path /blocked-one\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	defOff := false
	cfg := config.Default()
	cfg.Secret = "test-dynamic-rules-secret"
	cfg.TLS.Mode = "selfsigned"
	cfg.TLS.CertDir = t.TempDir()
	cfg.Intel.IPSecurity = false
	cfg.Intel.LiveReputation = false
	cfg.Intel.AIIPList = false
	cfg.Sites = []config.SiteConfig{
		{
			Domain:    "rules.example.com",
			Upstreams: []string{"http://backend:8080"},
			Protect:   true,
			WAF: config.SiteWAFConfig{
				VerifyLevel:  config.VerifyLevelOnlySuspicious,
				DefaultRules: &defOff,
				RuleSources: []config.RuleSource{
					{Source: dynFile, Dynamic: true, Name: "dyn-file"},
					{Source: dynDir, Dynamic: true, Name: "dyn-dir"},
					{Source: staticFile, Dynamic: false, Name: "static-file"},
				},
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := BuildRuntime(ctx, cfg, RuntimeOptions{
		LocalWatchInterval: 30 * time.Millisecond,
		HTTPTransport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
				Body:       io.NopCloser(strings.NewReader("upstream-ok")),
			}, nil
		}),
	})
	if err != nil {
		t.Fatalf("BuildRuntime failed: %v", err)
	}
	defer rt.Close()
	rt.RemoteRules.Start(ctx)

	doReq := func(path, uaSuffix string) int {
		req := httptest.NewRequest("GET", "https://rules.example.com"+path, nil)
		req.Host = "rules.example.com"
		req.RemoteAddr = "203.0.113.20:23456"
		addBrowserHeaders(req)
		if uaSuffix != "" {
			req.Header.Set("User-Agent", req.Header.Get("User-Agent")+" "+uaSuffix)
		}
		rec := httptest.NewRecorder()
		rt.HTTPSHandler.ServeHTTP(rec, req)
		return rec.Code
	}

	waitForStatus := func(name, path, uaSuffix string, want int) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for {
			got := doReq(path, uaSuffix)
			if got == want {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: status = %d, want %d (path=%s ua=%s)", name, got, want, path, uaSuffix)
			}
			time.Sleep(25 * time.Millisecond)
		}
	}

	// 1. Initial rules active synchronously at startup
	if code := doReq("/ok", ""); code != http.StatusOK {
		t.Fatalf("normal request status = %d, want 200", code)
	}
	if code := doReq("/ok", "InitialBot"); code != http.StatusForbidden {
		t.Fatalf("InitialBot status = %d, want 403", code)
	}
	if code := doReq("/blocked-one", ""); code != http.StatusForbidden {
		t.Fatalf("/blocked-one status = %d, want 403", code)
	}
	if code := doReq("/ok", "StaticBot"); code != http.StatusForbidden {
		t.Fatalf("StaticBot status = %d, want 403", code)
	}

	// 2. Modify dynamic single rule file -> auto-reloads
	if err := os.WriteFile(dynFile, []byte("block ua *UpdatedBot*\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitForStatus("UpdatedBot blocked", "/ok", "UpdatedBot", http.StatusForbidden)
	waitForStatus("InitialBot unblocked", "/ok", "InitialBot", http.StatusOK)

	// 3. Invalid rule syntax preserves last-known-good dynamic rules
	if err := os.WriteFile(dynFile, []byte("invalid_action ???\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(120 * time.Millisecond)
	if code := doReq("/ok", "UpdatedBot"); code != http.StatusForbidden {
		t.Fatalf("invalid syntax should preserve last-known-good UpdatedBot block, got %d", code)
	}

	// 4. Reverting dynamic file to valid empty rules unblocks UpdatedBot
	if err := os.WriteFile(dynFile, []byte("# cleared\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitForStatus("UpdatedBot unblocked", "/ok", "UpdatedBot", http.StatusOK)

	// 5. Adding a file inside dynamic folder auto-reloads and blocks new path
	extraFile := filepath.Join(dynDir, "02-extra.rules")
	if err := os.WriteFile(extraFile, []byte("block path /blocked-two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitForStatus("/blocked-two blocked", "/blocked-two", "", http.StatusForbidden)

	// 6. Deleting file inside dynamic folder unblocks /blocked-two while keeping 01-base.rules active
	if err := os.Remove(extraFile); err != nil {
		t.Fatal(err)
	}
	waitForStatus("/blocked-two unblocked", "/blocked-two", "", http.StatusOK)
	if code := doReq("/blocked-one", ""); code != http.StatusForbidden {
		t.Fatalf("/blocked-one should stay blocked after deleting 02-extra.rules, got %d", code)
	}

	// 7. Static file (Dynamic: false) does NOT auto-reload when edited on disk
	if err := os.WriteFile(staticFile, []byte("# cleared\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if code := doReq("/ok", "StaticBot"); code != http.StatusForbidden {
		t.Fatalf("static rule file should NOT auto-reload, got status %d (want 403)", code)
	}
}
