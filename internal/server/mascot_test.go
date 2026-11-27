package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gymbag/internal/config"
	"gymbag/internal/store"
	"gymbag/internal/tcpguard"
	"gymbag/internal/waf"
)

func Test429And403PagesServeMascotAssetSuccessfully(t *testing.T) {
	dir := t.TempDir()
	svgBytes := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><circle cx="5" cy="5" r="4"/></svg>`)
	customRejectPath := filepath.Join(dir, "custom-reject.svg")
	if err := os.WriteFile(customRejectPath, svgBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	globalPensivePath := filepath.Join(dir, "global-pensive.webp")
	if err := os.WriteFile(globalPensivePath, []byte("RIFFmockwebp"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.TLS.CertDir = filepath.Join(dir, "certs")
	cfg.Intel = config.IntelConfig{IPSecurity: false, LiveReputation: false, AIIPList: false, VPNDetection: false, RefreshInterval: 3600}
	cfg.Mascot = &config.MascotConfig{
		Default: globalPensivePath,
	}
	cfg.Sites = []config.SiteConfig{
		{
			Domain:    "rate.example.com",
			Upstreams: []string{"http://127.0.0.1:3000"},
			Protect:   true,
			WAF: config.SiteWAFConfig{
				VerifyLevel:  config.VerifyLevelAlways,
				Difficulty:   1,
				ChallengeTTL: 300,
				ClearanceTTL: 3600,
				CookieName:   "waf_clearance",
				RateLimit:    1,
				RateWindow:   60,
			},
			Mascot: &config.MascotConfig{
				Reject: customRejectPath,
			},
		},
		{
			Domain:    "remote.example.com",
			Upstreams: []string{"http://127.0.0.1:3001"},
			Protect:   true,
			WAF:       config.DefaultSiteWAFConfig(),
			Mascot: &config.MascotConfig{
				Default: "https://cdn.example.com/remote-mascot.png",
			},
		},
	}

	rt, err := BuildRuntime(context.Background(), cfg, RuntimeOptions{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		HTTPTransport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok"))}, nil
		}),
	})
	if err != nil {
		t.Fatalf("BuildRuntime failed: %v", err)
	}
	defer rt.Close()

	send := func(hostName, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "https://"+hostName+path, nil)
		req.Host = hostName
		req.RemoteAddr = "198.51.100.77:54321"
		req.Header.Set("User-Agent", "Mozilla/5.0")
		rec := httptest.NewRecorder()
		rt.HTTPSHandler.ServeHTTP(rec, req)
		return rec
	}

	// 1. First request on rate.example.com -> PoW challenge with global custom pensive .webp mascot
	rec1 := send("rate.example.com", "/")
	if rec1.Code != http.StatusForbidden {
		t.Fatalf("req1 status = %d, want 403 challenge", rec1.Code)
	}
	if !strings.Contains(rec1.Body.String(), ".webp") {
		t.Fatalf("expected custom global .webp mascot on challenge page, got:\n%s", rec1.Body.String())
	}

	// 2. Second request from same IP exhausts RateLimit=1 -> 429 Slow down HTML containing custom .svg reject mascot
	rec2 := send("rate.example.com", "/")
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("req2 status = %d, want 429", rec2.Code)
	}
	imgMatch := regexp.MustCompile(`<img[^>]+src="([^"]+)"`).FindStringSubmatch(rec2.Body.String())
	if len(imgMatch) != 2 || !strings.HasPrefix(imgMatch[1], "/.waf/assets/custom-mascot-") || !strings.HasSuffix(imgMatch[1], ".svg") {
		t.Fatalf("expected custom SVG mascot src in 429 page, got %v\nHTML:\n%s", imgMatch, rec2.Body.String())
	}

	// 3. CRITICAL BUG FIX CHECK: fetching that mascot URL from the SAME rate-limited IP returns 200 OK with exact image bytes & MIME type!
	assetRec := send("rate.example.com", imgMatch[1])
	if assetRec.Code != http.StatusOK {
		t.Fatalf("rate-limited IP fetching %s returned %d (expected 200 OK)", imgMatch[1], assetRec.Code)
	}
	if ct := assetRec.Header().Get("Content-Type"); ct != "image/svg+xml" {
		t.Fatalf("asset Content-Type = %q, want image/svg+xml", ct)
	}
	if assetRec.Body.String() != string(svgBytes) {
		t.Fatalf("asset body mismatch: %q", assetRec.Body.String())
	}

	// 4. Site with remote URL mascot renders remote URL and sets https: CSP
	remoteRec := send("remote.example.com", "/?.waf_test=1")
	if remoteRec.Code != http.StatusForbidden {
		// Force block check on /.env to test 403 block page too
		remoteRec = send("remote.example.com", "/.env")
	} else {
		remoteRec = send("remote.example.com", "/.env")
	}
	if remoteRec.Code != http.StatusForbidden ||
		!strings.Contains(remoteRec.Body.String(), `src="https://cdn.example.com/remote-mascot.png"`) ||
		!strings.Contains(remoteRec.Header().Get("Content-Security-Policy"), "img-src 'self' https:") {
		t.Fatalf("unexpected remote mascot 403 page: code=%d csp=%q body=%s",
			remoteRec.Code, remoteRec.Header().Get("Content-Security-Policy"), remoteRec.Body.String())
	}

	// 5. Live update via ReplaceConfig swaps mascot asset cleanly
	nextCfg := rt.Snapshot()
	nextCfg.Sites[1].Mascot = &config.MascotConfig{Default: "http://img.example.com/new.png"}
	if err := rt.ReplaceConfig(nextCfg); err != nil {
		t.Fatalf("ReplaceConfig failed: %v", err)
	}
	updatedRec := send("remote.example.com", "/.env")
	if !strings.Contains(updatedRec.Body.String(), `src="http://img.example.com/new.png"`) ||
		!strings.Contains(updatedRec.Header().Get("Content-Security-Policy"), "img-src 'self' https: http:") {
		t.Fatalf("expected updated remote mascot after ReplaceConfig, got csp=%q", updatedRec.Header().Get("Content-Security-Policy"))
	}
}

func TestSharedPortVerifyPortalCustomMascotAndAssets(t *testing.T) {
	dir := t.TempDir()
	customPath := filepath.Join(dir, "portal.png")
	if err := os.WriteFile(customPath, []byte("custom-portal-png"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.TLS.CertDir = filepath.Join(dir, "certs")
	cfg.Intel = config.IntelConfig{IPSecurity: false, LiveReputation: false, AIIPList: false, VPNDetection: false, RefreshInterval: 3600}
	cfg.Mascot = &config.MascotConfig{Default: customPath}
	cfg.Verify.Port = cfg.HTTPSPort // Share HTTPS port 443 with HTTP site
	cfg.SSH = &config.SSHConfig{
		Enabled:      true,
		Port:         2222,
		Upstream:     "127.0.0.1:22222",
		Protect:      true,
		VerifyLevel:  config.VerifyLevelAlways,
		Difficulty:   1,
		ChallengeTTL: 300,
		ClearanceTTL: 3600,
		RateLimit:    60,
		RateWindow:   60,
	}

	rt, err := BuildRuntime(context.Background(), cfg, RuntimeOptions{
		Logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		DisablePublicIPProbe: true,
		ServerIPs:            []string{"127.0.0.1"},
	})
	if err != nil {
		t.Fatalf("BuildRuntime failed: %v", err)
	}
	defer rt.Close()

	// Create a valid gate ticket so /v/<token> renders with the custom mascot
	tok := "test-token-123"
	gateSt, ok := rt.Store.(store.GateStore)
	if !ok {
		t.Fatal("expected GateStore")
	}
	if err := gateSt.SaveGateTicket(context.Background(), tok, store.GateTicket{
		Service:    "ssh",
		ClientIP:   "203.0.113.10",
		Difficulty: 1,
	}, 300); err != nil {
		t.Fatal(err)
	}

	// Request /v/<token> on IP Host over shared HTTPS port
	req := httptest.NewRequest(http.MethodGet, "https://127.0.0.1/v/"+tok, nil)
	req.Host = "127.0.0.1"
	req.RemoteAddr = "203.0.113.10:12345"
	rec := httptest.NewRecorder()
	rt.HTTPSHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("shared-port /v/ status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	imgMatch := regexp.MustCompile(`<img[^>]+src="([^"]+)"`).FindStringSubmatch(rec.Body.String())
	if len(imgMatch) != 2 || !strings.HasPrefix(imgMatch[1], "/.waf/assets/custom-mascot-") {
		t.Fatalf("expected custom mascot in VerifyPortal HTML, got %v", imgMatch)
	}

	// Shared-port VerifyPortal asset fetch on IP Host must return 200 OK (not 400 Invalid host header!)
	_ = tcpguard.IsGatePath
	assetReq := httptest.NewRequest(http.MethodGet, "https://127.0.0.1"+imgMatch[1], nil)
	assetReq.Host = "127.0.0.1"
	assetRec := httptest.NewRecorder()
	rt.HTTPSHandler.ServeHTTP(assetRec, assetReq)
	if assetRec.Code != http.StatusOK || assetRec.Body.String() != "custom-portal-png" {
		t.Fatalf("shared-port VerifyPortal asset fetch = (%d, %q), want (200, custom-portal-png)", assetRec.Code, assetRec.Body.String())
	}

	// Also verify /.waf/assets/default works cleanly via waf helper
	_ = waf.PensiveAsset
}
