package waf

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"gymbag/internal/config"
	"gymbag/internal/store"
)

func BenchmarkEngineDecideHTTP(b *testing.B) {
	st := store.NewMemoryStore()
	tokens := NewClearanceTokens([]byte("bench-secret-key"), 86400)

	browserReq := func(path string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "https://app.example.com"+path, nil)
		r.Host = "app.example.com"
		r.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
		r.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		r.Header.Set("Accept-Language", "en-US,en;q=0.9")
		r.Header.Set("Sec-Fetch-Mode", "navigate")
		return r
	}

	b.Run("ProtectOff", func(b *testing.B) {
		site := config.SiteConfig{
			Domain:    "app.example.com",
			Upstreams: []string{"http://127.0.0.1:8080"},
			Protect:   false,
			WAF:       config.DefaultSiteWAFConfig(),
		}
		eng := BuildSiteEngine(site, st, tokens)
		req := browserReq("/index.html")
		ctx := context.Background()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if d := eng.DecideHTTP(ctx, req, "app.example.com", "203.0.113.10"); d.Action != ActionAllow {
				b.Fatalf("unexpected action: %v", d.Action)
			}
		}
	})

	b.Run("Verify3_BrowserPassThrough", func(b *testing.B) {
		wafCfg := config.DefaultSiteWAFConfig()
		wafCfg.VerifyLevel = config.VerifyLevelSuspiciousAndAI
		wafCfg.RateLimit = 100_000_000
		site := config.SiteConfig{
			Domain:    "app.example.com",
			Upstreams: []string{"http://127.0.0.1:8080"},
			Protect:   true,
			WAF:       wafCfg,
		}
		eng := BuildSiteEngine(site, st, tokens)
		req := browserReq("/articles/2026/perf")
		ctx := context.Background()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if d := eng.DecideHTTP(ctx, req, "app.example.com", "203.0.113.10"); d.Action != ActionAllow {
				b.Fatalf("unexpected action: %v", d.Action)
			}
		}
	})

	b.Run("Verify0_ClearedCookie", func(b *testing.B) {
		wafCfg := config.DefaultSiteWAFConfig()
		wafCfg.VerifyLevel = config.VerifyLevelAlways
		wafCfg.RateLimit = 100_000_000
		site := config.SiteConfig{
			Domain:    "app.example.com",
			Upstreams: []string{"http://127.0.0.1:8080"},
			Protect:   true,
			WAF:       wafCfg,
		}
		eng := BuildSiteEngine(site, st, tokens)
		req := browserReq("/dashboard")
		tok := tokens.Issue("app.example.com", "203.0.113.10", 86400)
		req.Header.Set("Cookie", "waf_clearance="+tok)
		ctx := context.Background()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if d := eng.DecideHTTP(ctx, req, "app.example.com", "203.0.113.10"); d.Action != ActionAllow {
				b.Fatalf("unexpected action: %v", d.Action)
			}
		}
	})

	b.Run("Verify3_BlockDotEnv", func(b *testing.B) {
		wafCfg := config.DefaultSiteWAFConfig()
		wafCfg.VerifyLevel = config.VerifyLevelSuspiciousAndAI
		wafCfg.RateLimit = 100_000_000
		site := config.SiteConfig{
			Domain:    "app.example.com",
			Upstreams: []string{"http://127.0.0.1:8080"},
			Protect:   true,
			WAF:       wafCfg,
		}
		eng := BuildSiteEngine(site, st, tokens)
		req := browserReq("/.env")
		ctx := context.Background()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if d := eng.DecideHTTP(ctx, req, "app.example.com", "203.0.113.10"); d.Action != ActionDeny {
				b.Fatalf("unexpected action: %v", d.Action)
			}
		}
	})
}
