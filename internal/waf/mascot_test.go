package waf

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gymbag/internal/config"
	"gymbag/internal/store"
)

func TestAssetsAllowedOn429And403AndThreatBlock(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	defer st.Close()

	// 1. Exhaust RateLimit=1 -> normal request hits 429, while /.waf/assets/* stays allowed
	site := config.SiteConfig{
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
	}
	eng := BuildSiteEngine(site, st, NewClearanceTokens([]byte("secret"), 3600))

	dec1 := eng.Decide(ctx, IncomingRequest{Method: "GET", Host: "rate.example.com", Path: "/", ClientIP: "198.51.100.10", Headers: map[string]string{}})
	if dec1.Action != ActionChallenge {
		t.Fatalf("req1 = %+v, want ActionChallenge", dec1)
	}
	dec2 := eng.Decide(ctx, IncomingRequest{Method: "GET", Host: "rate.example.com", Path: "/", ClientIP: "198.51.100.10", Headers: map[string]string{}})
	if dec2.Action != ActionDeny || dec2.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("req2 = %+v, want 429 ActionDeny", dec2)
	}
	for _, assetPath := range []string{
		"/.waf/assets/" + RejectAsset,
		"/.waf/v1/assets/custom-mascot-123456.png",
	} {
		assetDec := eng.Decide(ctx, IncomingRequest{Method: "GET", Host: "rate.example.com", Path: assetPath, ClientIP: "198.51.100.10", Headers: map[string]string{}})
		if assetDec.Action != ActionAllow {
			t.Fatalf("rate-limited IP fetching %s got %+v, want ActionAllow", assetPath, assetDec)
		}
	}

	// 2. Custom IP block rule -> normal path gets 403, while /.waf/assets/* stays allowed
	siteBlocked := config.SiteConfig{
		Domain:    "block.example.com",
		Upstreams: []string{"http://127.0.0.1:3000"},
		Protect:   true,
		WAF: config.SiteWAFConfig{
			VerifyLevel: config.VerifyLevelAlways,
			Difficulty:   1,
			ChallengeTTL: 300,
			ClearanceTTL: 3600,
			CookieName:   "waf_clearance",
			RateLimit:    100,
			RateWindow:   60,
			Rules: []config.CustomRule{
				{Action: config.RuleActionBlock, IPs: []string{"203.0.113.5"}},
			},
		},
	}
	engBlocked := BuildSiteEngine(siteBlocked, st, NewClearanceTokens([]byte("secret"), 3600))
	if dec := engBlocked.Decide(ctx, IncomingRequest{Method: "GET", Host: "block.example.com", Path: "/secret", ClientIP: "203.0.113.5", Headers: map[string]string{}}); dec.Action != ActionDeny {
		t.Fatalf("blocked IP /secret got %+v, want ActionDeny", dec)
	}
	if assetDec := engBlocked.Decide(ctx, IncomingRequest{Method: "GET", Host: "block.example.com", Path: "/.waf/assets/" + RejectAsset, ClientIP: "203.0.113.5", Headers: map[string]string{}}); assetDec.Action != ActionAllow {
		t.Fatalf("blocked IP fetching mascot asset got %+v, want ActionAllow", assetDec)
	}

	// 3. Protect off + block ip rule -> /.waf/assets/* still allowed
	siteOpen := siteBlocked
	siteOpen.Protect = false
	engOpen := BuildSiteEngine(siteOpen, st, NewClearanceTokens([]byte("secret"), 3600))
	if assetDec := engOpen.Decide(ctx, IncomingRequest{Method: "GET", Host: "block.example.com", Path: "/.waf/assets/" + RejectAsset, ClientIP: "203.0.113.5", Headers: map[string]string{}}); assetDec.Action != ActionAllow {
		t.Fatalf("unprotected+blocked IP fetching mascot asset got %+v, want ActionAllow", assetDec)
	}

	// 4. Threat intel IP block -> /.waf/assets/* still allowed
	engIntel := BuildSiteEngineWithIntel(site, st, NewClearanceTokens([]byte("secret"), 3600), &stubIntel{
		threatIPs: map[string]bool{"192.0.2.99": true},
	}, "block")
	if dec := engIntel.Decide(ctx, IncomingRequest{Method: "GET", Host: "rate.example.com", Path: "/", ClientIP: "192.0.2.99", Headers: map[string]string{}}); dec.Action != ActionDeny {
		t.Fatalf("threat IP got %+v, want ActionDeny", dec)
	}
	if assetDec := engIntel.Decide(ctx, IncomingRequest{Method: "GET", Host: "rate.example.com", Path: "/.waf/assets/" + RejectAsset, ClientIP: "192.0.2.99", Headers: map[string]string{}}); assetDec.Action != ActionAllow {
		t.Fatalf("threat IP fetching mascot asset got %+v, want ActionAllow", assetDec)
	}
}

func TestResolveMascotLocalFileDirectoryRemoteAndCSP(t *testing.T) {
	dir := t.TempDir()

	// Create local files of different formats
	svgFile := filepath.Join(dir, "logo.svg")
	if err := os.WriteFile(svgFile, []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"></svg>`), 0o644); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(dir, "mascots")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Minimal valid PNG/WebP/SVG bytes for extension-based Content-Type & lookup tests
	_ = os.WriteFile(filepath.Join(stateDir, "pensive.png"), []byte("png-bytes"), 0o644)
	_ = os.WriteFile(filepath.Join(stateDir, "happy.webp"), []byte("webp-bytes"), 0o644)
	_ = os.WriteFile(filepath.Join(stateDir, "reject.svg"), []byte(`<svg/>`), 0o644)

	// 1. Directory auto-discovery per state
	sources, assets, err := ResolveMascot(nil, &config.MascotConfig{Default: stateDir}, "")
	if err != nil {
		t.Fatalf("ResolveMascot dir failed: %v", err)
	}
	store := NewAssetStore()
	store.Replace(assets)
	if !strings.HasPrefix(sources.PensiveURL, "/.waf/assets/custom-mascot-") || !strings.HasSuffix(sources.PensiveURL, ".png") {
		t.Fatalf("unexpected pensive URL: %q", sources.PensiveURL)
	}
	if !strings.HasSuffix(sources.HappyURL, ".webp") || !strings.HasSuffix(sources.RejectURL, ".svg") {
		t.Fatalf("unexpected happy/reject URLs: %+v", sources)
	}
	happyName := strings.TrimPrefix(sources.HappyURL, "/.waf/assets/")
	if data, ct, _, ok := store.Lookup(happyName); !ok || ct != "image/webp" || string(data) != "webp-bytes" {
		t.Fatalf("AssetStore Lookup happy = (%q, %q, %v)", string(data), ct, ok)
	}
	rejectName := strings.TrimPrefix(sources.RejectURL, "/.waf/assets/")
	if _, ct, _, ok := store.Lookup(rejectName); !ok || ct != "image/svg+xml" {
		t.Fatalf("AssetStore Lookup reject Content-Type = %q ok=%v", ct, ok)
	}
	if csp := BuildPageCSP(sources); !strings.Contains(csp, "img-src 'self';") {
		t.Fatalf("local mascot CSP should keep img-src 'self', got %q", csp)
	}

	// 2. Site overrides global with remote https:// and http:// URLs + rendering check
	siteSources, _, err := ResolveMascot(
		&config.MascotConfig{
			Reject: "https://cdn.example.com/stop.png",
			Happy:  "http://cdn.example.com/ok.png",
		},
		&config.MascotConfig{Default: svgFile},
		"",
	)
	if err != nil {
		t.Fatalf("ResolveMascot mixed site+global failed: %v", err)
	}
	if !strings.HasSuffix(siteSources.PensiveURL, ".svg") ||
		siteSources.HappyURL != "http://cdn.example.com/ok.png" ||
		siteSources.RejectURL != "https://cdn.example.com/stop.png" {
		t.Fatalf("unexpected mixed sources: %+v", siteSources)
	}
	renderer := NewRendererWithMascot("", siteSources)
	chalRec := httptest.NewRecorder()
	renderer.WriteChallenge(chalRec, "chal-xyz", 2)
	if !strings.Contains(chalRec.Body.String(), siteSources.PensiveURL) ||
		!strings.Contains(chalRec.Body.String(), siteSources.HappyURL) ||
		!strings.Contains(chalRec.Body.String(), siteSources.RejectURL) {
		t.Fatalf("challenge HTML missing custom mascot URLs:\n%s", chalRec.Body.String())
	}
	if csp := chalRec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "img-src 'self' https: http:") {
		t.Fatalf("unexpected challenge CSP for http+https remote mascots: %q", csp)
	}

	rateRec := httptest.NewRecorder()
	renderer.WriteReject(rateRec, "rate", http.StatusTooManyRequests)
	if rateRec.Code != http.StatusTooManyRequests || !strings.Contains(rateRec.Body.String(), `src="https://cdn.example.com/stop.png"`) {
		t.Fatalf("429 HTML missing custom reject mascot: code=%d body=%s", rateRec.Code, rateRec.Body.String())
	}

	// 3. Missing local mascot path returns clear error
	if _, _, err := ResolveMascot(&config.MascotConfig{Default: filepath.Join(dir, "does-not-exist.png")}, nil, ""); err == nil {
		t.Fatal("expected error for missing local mascot file")
	}
}
