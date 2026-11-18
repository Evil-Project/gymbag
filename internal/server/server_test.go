package server

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gymbag/internal/config"
	"gymbag/internal/intel"
	"gymbag/internal/localpki"
	"gymbag/internal/tlsmanager"
	"gymbag/internal/waf"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// pipeListener implements net.Listener in memory via net.Pipe (sandbox-safe, no TCP socket bind).
type pipeListener struct {
	ch     chan net.Conn
	closed chan struct{}
}

func newPipeListener() *pipeListener {
	return &pipeListener{
		ch:     make(chan net.Conn, 16),
		closed: make(chan struct{}),
	}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.ch:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	select {
	case <-l.closed:
	default:
		close(l.closed)
	}
	return nil
}

func (l *pipeListener) Addr() net.Addr { return pipeAddr("pipe") }

func (l *pipeListener) DialContext(_ context.Context, _, _ string) (net.Conn, error) {
	c1, c2 := net.Pipe()
	select {
	case l.ch <- c2:
		return c1, nil
	case <-l.closed:
		_ = c1.Close()
		_ = c2.Close()
		return nil, net.ErrClosed
	}
}

type pipeAddr string

func (a pipeAddr) Network() string { return string(a) }
func (a pipeAddr) String() string  { return string(a) }

func newTestMultiSiteConfig(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.ParseCaddyfile(`
		{
			secret test-secret
			cert_dir ` + t.TempDir() + `
			tls selfsigned
			cookie_secure false
			difficulty 1
		}

		always.example.com {
			reverse_proxy http://always-upstream:3000
			verify 0
		}

		suspicion.example.com {
			reverse_proxy http://suspicion-upstream:3001
			verify 1
		}

		ai.example.com {
			reverse_proxy http://ai-upstream:3002
			verify 2
		}

		both.example.com {
			reverse_proxy http://both-upstream:3003
			verify 3
		}

		open.example.com {
			reverse_proxy http://open-upstream:3004
			protect off
		}
	`)
	if err != nil {
		t.Fatalf("ParseCaddyfile failed: %v", err)
	}
	return cfg
}

func addBrowserHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
}

func TestEndToEndMultiSiteTriggerModesAndChallengeFlow(t *testing.T) {
	cfg := newTestMultiSiteConfig(t)
	upstreamHits := make(map[string]int)

	rt, err := BuildRuntime(context.Background(), cfg, RuntimeOptions{
		HTTPTransport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			upstreamHits[req.URL.Host]++
			body := "upstream=" + req.URL.Host + " path=" + req.URL.Path
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	sendReq := func(method, hostName, path string, modify func(*http.Request)) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "https://"+hostName+path, nil)
		req.Host = hostName
		req.RemoteAddr = "203.0.113.50:54321"
		if modify != nil {
			modify(req)
		}
		rec := httptest.NewRecorder()
		rt.HTTPSHandler.ServeHTTP(rec, req)
		return rec
	}

	// 1. Unknown host rejected with 400
	if rec := sendReq("GET", "unknown.example.com", "/", addBrowserHeaders); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown host status = %d, want 400", rec.Code)
	}

	// 2. Path traversal (/../secret) & TRACE blocked with 403 before upstream
	rawTraversal := httptest.NewRequest("GET", "https://open.example.com/secret", nil)
	rawTraversal.Host = "open.example.com"
	rawTraversal.RequestURI = "/../secret"
	rawTraversal.RemoteAddr = "203.0.113.50:54321"
	recTrav := httptest.NewRecorder()
	rt.HTTPSHandler.ServeHTTP(recTrav, rawTraversal)
	if recTrav.Code != http.StatusForbidden {
		t.Fatalf("traversal status = %d, want 403", recTrav.Code)
	}
	if rec := sendReq("TRACE", "open.example.com", "/", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("TRACE status = %d, want 403", rec.Code)
	}

	// 3. Always mode: challenges browser, solves PoW, sets cookie, proxies to always-upstream:3000, blocks replay
	recBlocked := sendReq("GET", "always.example.com", "/dashboard", addBrowserHeaders)
	if recBlocked.Code != http.StatusForbidden || upstreamHits["always-upstream:3000"] != 0 {
		t.Fatalf("always mode should challenge browser: code=%d", recBlocked.Code)
	}
	specMatch := regexp.MustCompile(`<script type="application/json" id="gymbag-challenge">(.*?)</script>`).FindStringSubmatch(recBlocked.Body.String())
	if len(specMatch) != 2 {
		t.Fatal("missing challenge spec")
	}
	var spec struct {
		ChallengeID string `json:"challenge_id"`
		Difficulty  int    `json:"difficulty"`
	}
	_ = json.Unmarshal([]byte(specMatch[1]), &spec)

	var validNonce int64
	for !waf.Accepts(spec.ChallengeID, validNonce, spec.Difficulty) {
		validNonce++
	}
	var badNonce int64
	for waf.Accepts(spec.ChallengeID, badNonce, spec.Difficulty) {
		badNonce++
	}

	postVerify := func(hostName string, payload map[string]any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "https://"+hostName+"/.waf/verify", bytes.NewReader(raw))
		req.Host = hostName
		req.RemoteAddr = "203.0.113.50:54321"
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		rt.HTTPSHandler.ServeHTTP(rec, req)
		return rec
	}

	// Bad nonce fails with 403
	if rec := postVerify("always.example.com", map[string]any{
		"challenge_id": spec.ChallengeID, "nonce": badNonce, "verified": true,
	}); rec.Code != http.StatusForbidden {
		t.Fatalf("bad nonce status = %d, want 403", rec.Code)
	}

	// Valid nonce succeeds & issues clearance cookie
	recVerified := postVerify("always.example.com", map[string]any{
		"challenge_id": spec.ChallengeID, "nonce": validNonce, "verified": true,
	})
	if recVerified.Code != http.StatusOK {
		t.Fatalf("valid verify status = %d, body = %s", recVerified.Code, recVerified.Body.String())
	}
	cookies := recVerified.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "waf_clearance" {
		t.Fatalf("expected waf_clearance cookie, got %+v", cookies)
	}

	// Replay of same challenge fails
	if rec := postVerify("always.example.com", map[string]any{
		"challenge_id": spec.ChallengeID, "nonce": validNonce, "verified": true,
	}); rec.Code != http.StatusForbidden {
		t.Fatalf("replay status = %d, want 403", rec.Code)
	}

	// Request with clearance cookie proxies to always-upstream:3000
	recCleared := sendReq("GET", "always.example.com", "/dashboard", func(r *http.Request) {
		addBrowserHeaders(r)
		r.AddCookie(cookies[0])
	})
	if recCleared.Code != http.StatusOK || !strings.Contains(recCleared.Body.String(), "always-upstream:3000") {
		t.Fatalf("cleared request failed: %d %s", recCleared.Code, recCleared.Body.String())
	}

	// Cookie from always.example.com MUST NOT clear another site from same IP if replayed manually!
	recCrossSite := sendReq("GET", "suspicion.example.com", "/", func(r *http.Request) {
		r.Header.Set("User-Agent", "curl/8.7.1")
		r.AddCookie(cookies[0])
	})
	if recCrossSite.Code != http.StatusForbidden {
		t.Fatalf("cross-site cookie replay should be challenged, got %d", recCrossSite.Code)
	}

	// 4. "only suspicion": allows human & GPTBot, challenges curl
	if rec := sendReq("GET", "suspicion.example.com", "/", addBrowserHeaders); rec.Code != http.StatusOK {
		t.Fatalf("suspicion site should allow human browser, got %d", rec.Code)
	}
	if rec := sendReq("GET", "suspicion.example.com", "/", func(r *http.Request) {
		r.Header.Set("User-Agent", "Mozilla/5.0 (compatible; GPTBot/1.2; +https://openai.com/gptbot)")
	}); rec.Code != http.StatusOK {
		t.Fatalf("suspicion site should allow GPTBot, got %d", rec.Code)
	}
	if rec := sendReq("GET", "suspicion.example.com", "/", func(r *http.Request) {
		r.Header.Set("User-Agent", "curl/8.7.1")
	}); rec.Code != http.StatusForbidden {
		t.Fatalf("suspicion site should challenge curl, got %d", rec.Code)
	}

	// 5. "only ai": allows human & curl, challenges ClaudeBot
	if rec := sendReq("GET", "ai.example.com", "/", func(r *http.Request) {
		r.Header.Set("User-Agent", "curl/8.7.1")
	}); rec.Code != http.StatusOK {
		t.Fatalf("ai site should allow curl, got %d", rec.Code)
	}
	if rec := sendReq("GET", "ai.example.com", "/", func(r *http.Request) {
		r.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ClaudeBot/1.0; +claudebot@anthropic.com)")
	}); rec.Code != http.StatusForbidden {
		t.Fatalf("ai site should challenge ClaudeBot, got %d", rec.Code)
	}

	// 6. "suspicion & ai": allows human, challenges BOTH curl AND PerplexityBot
	if rec := sendReq("GET", "both.example.com", "/", addBrowserHeaders); rec.Code != http.StatusOK {
		t.Fatalf("both site should allow human browser, got %d", rec.Code)
	}
	if rec := sendReq("GET", "both.example.com", "/", func(r *http.Request) {
		r.Header.Set("User-Agent", "python-requests/2.32.0")
	}); rec.Code != http.StatusForbidden {
		t.Fatalf("both site should challenge python-requests, got %d", rec.Code)
	}
	if rec := sendReq("GET", "both.example.com", "/", func(r *http.Request) {
		r.Header.Set("User-Agent", "PerplexityBot/1.0")
	}); rec.Code != http.StatusForbidden {
		t.Fatalf("both site should challenge PerplexityBot, got %d", rec.Code)
	}

	// 7. Unprotected site ("protect off"): allows curl & GPTBot directly
	if rec := sendReq("GET", "open.example.com", "/public", func(r *http.Request) {
		r.Header.Set("User-Agent", "GPTBot/1.2")
	}); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "open-upstream:3004") {
		t.Fatalf("open site should pass GPTBot without challenge, got %d", rec.Code)
	}

	// 8. Assets & internal /.waf/* firewall behavior
	if rec := sendReq("GET", "always.example.com", "/.waf/assets/"+waf.PensiveAsset, nil); rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for embedded asset, got %d", rec.Code)
	}
	if rec := sendReq("GET", "always.example.com", "/.waf/nope", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 challenge for uncleared /.waf/nope, got %d", rec.Code)
	}
	if rec := sendReq("GET", "always.example.com", "/.waf/nope", func(r *http.Request) {
		r.AddCookie(cookies[0])
	}); rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cleared /.waf/nope (never proxied upstream), got %d", rec.Code)
	}
}

func TestRedirectACMEAndRESTAdminCRUD(t *testing.T) {
	cfg := newTestMultiSiteConfig(t)
	rt, err := BuildRuntime(context.Background(), cfg, RuntimeOptions{
		HTTPTransport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("ok:" + req.URL.Host)),
			}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// HTTP :80 serves ACME HTTP-01 and 308 redirects everything else
	rt.ACMEBoard.Publish("tok123", "tok123.thumb")
	acmeReq := httptest.NewRequest("GET", "http://always.example.com/.well-known/acme-challenge/tok123", nil)
	acmeReq.Host = "always.example.com"
	acmeRec := httptest.NewRecorder()
	rt.RedirectHandler.ServeHTTP(acmeRec, acmeReq)
	if acmeRec.Code != http.StatusOK || acmeRec.Body.String() != "tok123.thumb" {
		t.Fatalf("ACME HTTP-01 = %d %q", acmeRec.Code, acmeRec.Body.String())
	}

	redReq := httptest.NewRequest("POST", "http://always.example.com/app?x=1", nil)
	redReq.Host = "always.example.com"
	redRec := httptest.NewRecorder()
	rt.RedirectHandler.ServeHTTP(redRec, redReq)
	if redRec.Code != http.StatusPermanentRedirect || redRec.Header().Get("Location") != "https://always.example.com/app?x=1" {
		t.Fatalf("Redirect = %d %q", redRec.Code, redRec.Header().Get("Location"))
	}

	// REST Admin API: POST /api/v1/sites, PATCH /api/v1/sites/{domain}, GET /api/v1/sites/{domain}, DELETE /api/v1/sites/{domain}
	adminCall := func(method, path string, payload any) *httptest.ResponseRecorder {
		var body io.Reader
		if payload != nil {
			b, _ := json.Marshal(payload)
			body = bytes.NewReader(b)
		}
		req := httptest.NewRequest(method, "http://127.0.0.1:2019"+path, body)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		rt.AdminHandler.ServeHTTP(rec, req)
		return rec
	}

	if rec := adminCall("GET", "/api/v1/health", nil); rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/health = %d", rec.Code)
	}

	// Create a brand new website dynamically via REST POST /api/v1/sites with verify_level 2 (only ai)
	createRec := adminCall("POST", "/api/v1/sites", map[string]any{
		"domain":    "dynamic.example.com",
		"upstreams": []string{"http://dynamic-upstream:9090"},
		"protect":   true,
		"waf": map[string]any{
			"verify_level": 2,
			"difficulty":   2,
		},
	})
	if createRec.Code != http.StatusCreated || createRec.Header().Get("Location") != "/api/v1/sites/dynamic.example.com" {
		t.Fatalf("POST /api/v1/sites = %d %s", createRec.Code, createRec.Body.String())
	}

	// Immediately verify live gateway routes dynamic.example.com with level 2 (only ai) policy!
	dynCurl := httptest.NewRequest("GET", "https://dynamic.example.com/", nil)
	dynCurl.Host = "dynamic.example.com"
	dynCurl.Header.Set("User-Agent", "curl/8.7.1")
	dynRec := httptest.NewRecorder()
	rt.HTTPSHandler.ServeHTTP(dynRec, dynCurl)
	if dynRec.Code != http.StatusOK || dynRec.Body.String() != "ok:dynamic-upstream:9090" {
		t.Fatalf("dynamic site curl expected 200, got %d %q", dynRec.Code, dynRec.Body.String())
	}

	// Out-of-range verify_level 4 must be rejected
	if badPatch := adminCall("PATCH", "/api/v1/sites/dynamic.example.com", map[string]any{
		"verify_level": 4,
	}); badPatch.Code != http.StatusBadRequest && badPatch.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 400/422 for verify_level=4, got %d", badPatch.Code)
	}

	// PATCH dynamic.example.com to switch verify_level to 1 (only suspicious) live!
	patchRec := adminCall("PATCH", "/api/v1/sites/dynamic.example.com", map[string]any{
		"verify_level": 1,
	})
	if patchRec.Code != http.StatusOK {
		t.Fatalf("PATCH site = %d %s", patchRec.Code, patchRec.Body.String())
	}

	// Now curl on dynamic.example.com MUST be challenged (403)!
	dynRec2 := httptest.NewRecorder()
	rt.HTTPSHandler.ServeHTTP(dynRec2, dynCurl)
	if dynRec2.Code != http.StatusForbidden {
		t.Fatalf("after PATCH to verify_level 1, curl expected 403, got %d", dynRec2.Code)
	}

	// DELETE dynamic.example.com -> 204 No Content
	delRec := adminCall("DELETE", "/api/v1/sites/dynamic.example.com", nil)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("DELETE site = %d", delRec.Code)
	}
}

func TestLiveIntelAutoFeedsAndCustomRuleRESTAPI(t *testing.T) {
	cfg := newTestMultiSiteConfig(t)
	cfg.Intel.IPSecurityAction = "block"

	intelClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/threat.txt":
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader("91.240.118.172\t9\n")),
				}, nil
			case "/gptbot.json":
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader(`{"prefixes":[{"ipv4Prefix":"20.15.240.64/28"}]}`)),
				}, nil
			default:
				return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(""))}, nil
			}
		}),
	}

	intelMgr := intel.NewManager(intel.ManagerOptions{
		CacheDir:              t.TempDir(),
		IPSecurityEnabled:     true,
		LiveReputationEnabled: false,
		AIIPListEnabled:       true,
		ReplaceThreatFeeds:    []intel.FeedSource{{Name: "ipsum", URL: "https://intel.test/threat.txt"}},
		ReplaceAIFeeds:        []intel.FeedSource{{Name: "openai-gptbot", URL: "https://intel.test/gptbot.json"}},
		HTTPClient:            intelClient,
	})

	rt, err := BuildRuntime(context.Background(), cfg, RuntimeOptions{
		Intel: intelMgr,
		HTTPTransport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("proxied:" + req.URL.Host)),
			}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	adminCall := func(method, path string, payload any) *httptest.ResponseRecorder {
		var body io.Reader
		if payload != nil {
			b, _ := json.Marshal(payload)
			body = bytes.NewReader(b)
		}
		req := httptest.NewRequest(method, "http://127.0.0.1:2019"+path, body)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		rt.AdminHandler.ServeHTTP(rec, req)
		return rec
	}

	// 1. Trigger immediate intel refresh via POST /api/v1/intel/refresh
	refRec := adminCall("POST", "/api/v1/intel/refresh", nil)
	if refRec.Code != http.StatusOK || !strings.Contains(refRec.Body.String(), `"ai_entries":1`) {
		t.Fatalf("POST /api/v1/intel/refresh = %d %s", refRec.Code, refRec.Body.String())
	}

	// 2. Spoofed Chrome browser coming from OpenAI IP 20.15.240.70 hits ai.example.com (verify 2) -> challenged!
	aiReq := httptest.NewRequest("GET", "https://ai.example.com/article", nil)
	aiReq.Host = "ai.example.com"
	aiReq.RemoteAddr = "20.15.240.70:44321"
	addBrowserHeaders(aiReq)
	aiRec := httptest.NewRecorder()
	rt.HTTPSHandler.ServeHTTP(aiRec, aiReq)
	if aiRec.Code != http.StatusForbidden {
		t.Fatalf("expected OpenAI IP 20.15.240.70 to be challenged on ai.example.com, got %d", aiRec.Code)
	}

	// 3. Threat-feed IP 91.240.118.172 blocked (403) because ip_security_action == "block"
	badIPReq := httptest.NewRequest("GET", "https://both.example.com/", nil)
	badIPReq.Host = "both.example.com"
	badIPReq.RemoteAddr = "91.240.118.172:51111"
	addBrowserHeaders(badIPReq)
	badIPRec := httptest.NewRecorder()
	rt.HTTPSHandler.ServeHTTP(badIPRec, badIPReq)
	if badIPRec.Code != http.StatusForbidden || !strings.Contains(badIPRec.Body.String(), "Request blocked") {
		t.Fatalf("expected threat IP to be blocked, got %d %s", badIPRec.Code, badIPRec.Body.String())
	}

	// 4. Add a custom rule via POST /api/v1/sites/both.example.com/rules to block User-Agent "CustomBadUA"
	ruleRec := adminCall("POST", "/api/v1/sites/both.example.com/rules", map[string]any{
		"name":        "block-custom-ua",
		"action":      "block",
		"user_agents": []string{"CustomBadUA"},
	})
	if ruleRec.Code != http.StatusCreated {
		t.Fatalf("POST custom rule = %d %s", ruleRec.Code, ruleRec.Body.String())
	}

	uaReq := httptest.NewRequest("GET", "https://both.example.com/", nil)
	uaReq.Host = "both.example.com"
	uaReq.RemoteAddr = "203.0.113.77:12345"
	addBrowserHeaders(uaReq)
	uaReq.Header.Set("User-Agent", "Mozilla/5.0 CustomBadUA/1.0")
	uaRec := httptest.NewRecorder()
	rt.HTTPSHandler.ServeHTTP(uaRec, uaReq)
	if uaRec.Code != http.StatusForbidden || !strings.Contains(uaRec.Body.String(), "Request blocked") {
		t.Fatalf("expected CustomBadUA to be blocked by custom rule, got %d", uaRec.Code)
	}

	// 5. Delete the custom rule by name via DELETE /api/v1/sites/both.example.com/rules/block-custom-ua
	delRuleRec := adminCall("DELETE", "/api/v1/sites/both.example.com/rules/block-custom-ua", nil)
	if delRuleRec.Code != http.StatusNoContent {
		t.Fatalf("DELETE custom rule = %d", delRuleRec.Code)
	}
}

func TestOpenBoxDefaultSecurityRulesEndToEnd(t *testing.T) {
	cfg, err := config.ParseCaddyfile(`
		{
			secret openbox-secret
			cert_dir ` + t.TempDir() + `
			tls selfsigned
			cookie_secure false
			difficulty 1
		}

		minimal.example.com {
			reverse_proxy http://minimal-upstream:3000
			verify
		}

		strict.example.com {
			reverse_proxy http://strict-upstream:3001
			block
		}
	`)
	if err != nil {
		t.Fatalf("ParseCaddyfile failed: %v", err)
	}

	var upstreamHits atomic.Int32
	rt, err := BuildRuntime(context.Background(), cfg, RuntimeOptions{
		HTTPTransport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			upstreamHits.Add(1)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
				Body:       io.NopCloser(strings.NewReader("ok:" + req.URL.Host + req.URL.Path)),
			}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	send := func(hostName, target string, modify func(*http.Request)) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "https://"+hostName+target, nil)
		req.Host = hostName
		req.RemoteAddr = "203.0.113.88:50000"
		if modify != nil {
			modify(req)
		}
		rec := httptest.NewRecorder()
		rt.HTTPSHandler.ServeHTTP(rec, req)
		return rec
	}

	// 1. Out-of-the-box default block rules reject sensitive files, CMS probes, scanners, & SQLi/XSS with 403
	blockedProbes := []struct {
		name   string
		target string
		modify func(*http.Request)
	}{
		{"dotenv", "/.env", addBrowserHeaders},
		{"git-config", "/.git/config", addBrowserHeaders},
		{"wp-login", "/wp-login.php", addBrowserHeaders},
		{"wp-admin", "/wp-admin/install.php", addBrowserHeaders},
		{"sqlmap-ua", "/", func(r *http.Request) {
			addBrowserHeaders(r)
			r.Header.Set("User-Agent", "sqlmap/1.8")
		}},
		{"nuclei-ua", "/", func(r *http.Request) {
			addBrowserHeaders(r)
			r.Header.Set("User-Agent", "Mozilla/5.0 nuclei/3.2.0")
		}},
		{"sqli-query", "/?q=1+UNION+SELECT+1", addBrowserHeaders},
		{"xss-query", "/search?term=%3Cscript%3Ealert(1)%3C/script%3E", func(r *http.Request) {
			addBrowserHeaders(r)
			r.URL.RawQuery = "term=<script>alert(1)</script>"
		}},
	}
	for _, tc := range blockedProbes {
		rec := send("minimal.example.com", tc.target, tc.modify)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "Request blocked") {
			t.Fatalf("probe %s (%s): status=%d body=%q, want 403 Request blocked", tc.name, tc.target, rec.Code, rec.Body.String())
		}
	}
	if upstreamHits.Load() != 0 {
		t.Fatalf("expected 0 upstream hits during blocked probes, got %d", upstreamHits.Load())
	}

	// 2. Automated HTTP client (curl/8.7.1) is challenged with PoW (403 challenge page)
	recCurl := send("minimal.example.com", "/docs", func(r *http.Request) {
		r.Header.Set("User-Agent", "curl/8.7.1")
	})
	if recCurl.Code != http.StatusForbidden || !strings.Contains(recCurl.Body.String(), `id="gymbag-challenge"`) {
		t.Fatalf("expected curl/8.7.1 to receive PoW challenge page, got %d %s", recCurl.Code, recCurl.Body.String())
	}
	if upstreamHits.Load() != 0 {
		t.Fatalf("expected 0 upstream hits after curl challenge, got %d", upstreamHits.Load())
	}

	// 3. Normal Chrome browser request passes straight through to upstream!
	recBrowser := send("minimal.example.com", "/welcome", addBrowserHeaders)
	if recBrowser.Code != http.StatusOK || recBrowser.Body.String() != "ok:minimal-upstream:3000/welcome" {
		t.Fatalf("expected Chrome browser request to be allowed (200), got %d %q", recBrowser.Code, recBrowser.Body.String())
	}

	// 4. Bare `block` enables default block rules + ip_security_action=block while keeping PoW active
	if rt.Snapshot().Intel.IPSecurityAction != "block" {
		t.Fatalf("expected bare block directive to set ip_security_action=block, got %q", rt.Snapshot().Intel.IPSecurityAction)
	}
	if rec := send("strict.example.com", "/.env", addBrowserHeaders); rec.Code != http.StatusForbidden ||
		!strings.Contains(rec.Body.String(), "Request blocked") {
		t.Fatalf("expected bare block site to hard-reject /.env with 403 Request blocked, got %d %s", rec.Code, rec.Body.String())
	}
	if rec := send("strict.example.com", "/home", addBrowserHeaders); rec.Code != http.StatusForbidden ||
		!strings.Contains(rec.Body.String(), `id="gymbag-challenge"`) {
		t.Fatalf("expected bare block site (default verify 0) to PoW-challenge /home, got %d", rec.Code)
	}
}

func TestLocalPKITwoTierCAAndAutoHostsLifecycle(t *testing.T) {
	ctx := context.Background()
	certDir := t.TempDir()
	hostsDir := t.TempDir()
	hostsFile := filepath.Join(hostsDir, "hosts")

	initialHosts := "127.0.0.1 localhost\n::1 localhost\n10.0.0.5 db.corp # keep-existing\n"
	if err := os.WriteFile(hostsFile, []byte(initialHosts), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.ParseCaddyfile(`
		{
			secret local-pki-secret
			cert_dir ` + certDir + `
			cookie_secure false
		}

		app.internal {
			reverse_proxy http://app-upstream:3000
			verify
		}

		localhost {
			reverse_proxy http://localhost-upstream:5173
			protect off
		}
	`)
	if err != nil {
		t.Fatalf("ParseCaddyfile failed: %v", err)
	}

	rt, err := BuildRuntime(ctx, cfg, RuntimeOptions{
		HostsFilePath: hostsFile,
		HTTPTransport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
				Body:       io.NopCloser(strings.NewReader("local-ok:" + req.URL.Host)),
			}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. SetupHosts writes 127.0.0.1 app.internal & ::1 app.internal without duplicating localhost
	added, err := rt.SetupHosts()
	if err != nil {
		t.Fatalf("SetupHosts failed: %v", err)
	}
	if !slices.Equal(added, []string{"app.internal"}) {
		t.Fatalf("SetupHosts added = %v, want [app.internal]", added)
	}
	hostsRaw, err := os.ReadFile(hostsFile)
	if err != nil {
		t.Fatal(err)
	}
	hostsText := string(hostsRaw)
	if !strings.Contains(hostsText, "127.0.0.1 app.internal # gymbag-managed:") ||
		!strings.Contains(hostsText, "::1 app.internal # gymbag-managed:") {
		t.Fatalf("expected IPv4 and IPv6 managed entries for app.internal, got:\n%s", hostsText)
	}
	if !strings.Contains(hostsText, "10.0.0.5 db.corp # keep-existing") {
		t.Fatalf("expected pre-existing hosts entry preserved, got:\n%s", hostsText)
	}

	// 2. CertManager.Obtain issues two-tier local PKI leaf under tls auto
	mat, err := rt.CertManager.Obtain(ctx)
	if err != nil {
		t.Fatalf("CertManager.Obtain failed: %v", err)
	}
	if mat.Source != "local" || mat.Provider != "local" || filepath.Base(mat.CertPath) != localpki.LeafFullchainFilename {
		t.Fatalf("expected local PKI material, got %+v", mat)
	}

	fullchainPEM, err := os.ReadFile(mat.CertPath)
	if err != nil {
		t.Fatal(err)
	}
	var chainCerts []*x509.Certificate
	rest := fullchainPEM
	for len(rest) > 0 {
		var blk *pem.Block
		blk, rest = pem.Decode(rest)
		if blk == nil {
			break
		}
		if blk.Type == "CERTIFICATE" {
			c, err := x509.ParseCertificate(blk.Bytes)
			if err != nil {
				t.Fatal(err)
			}
			chainCerts = append(chainCerts, c)
		}
	}
	if len(chainCerts) != 2 {
		t.Fatalf("expected leaf + intermediate in local-fullchain.pem, got %d certs", len(chainCerts))
	}
	leafCert, interCert := chainCerts[0], chainCerts[1]

	rootPEM, err := os.ReadFile(filepath.Join(certDir, "ca", localpki.RootCACertFilename))
	if err != nil {
		t.Fatalf("failed to read Root CA PEM: %v", err)
	}
	rootCert, err := tlsmanager.LoadLeafCertificate(rootPEM)
	if err != nil {
		t.Fatal(err)
	}

	if rootCert.Subject.CommonName != localpki.RootCACommonName {
		t.Fatalf("Root CA CN = %q, want %q", rootCert.Subject.CommonName, localpki.RootCACommonName)
	}
	if interCert.Subject.CommonName != localpki.IntermediateCACommonName ||
		interCert.Issuer.CommonName != localpki.RootCACommonName {
		t.Fatalf("Intermediate CA subject=%q issuer=%q", interCert.Subject.CommonName, interCert.Issuer.CommonName)
	}
	if leafCert.Issuer.CommonName != localpki.IntermediateCACommonName {
		t.Fatalf("Leaf Issuer CN = %q, want %q", leafCert.Issuer.CommonName, localpki.IntermediateCACommonName)
	}

	roots := x509.NewCertPool()
	roots.AddCert(rootCert)
	inters := x509.NewCertPool()
	inters.AddCert(interCert)

	for _, targetName := range []string{"app.internal", "localhost", "127.0.0.1"} {
		if _, err := leafCert.Verify(x509.VerifyOptions{
			DNSName:       targetName,
			Roots:         roots,
			Intermediates: inters,
			KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		}); err != nil {
			t.Fatalf("leaf verification failed for %s: %v", targetName, err)
		}
	}

	// Verify live in-memory TLS handshake trusting ONLY "Neuro-sama Headquarters Root CA" (no TCP socket bind)
	pipeLn := newPipeListener()
	tlsLn := tls.NewListener(pipeLn, rt.CertManager.TLSConfig())
	httpsSrv := &http.Server{Handler: rt.HTTPSHandler}
	go func() {
		_ = httpsSrv.Serve(tlsLn)
	}()
	defer func() {
		_ = httpsSrv.Close()
		_ = pipeLn.Close()
	}()

	httpsTransport := &http.Transport{
		DialContext: pipeLn.DialContext,
		TLSClientConfig: &tls.Config{
			RootCAs:    roots,
			ServerName: "app.internal",
		},
	}
	defer httpsTransport.CloseIdleConnections()

	httpsClient := &http.Client{Transport: httpsTransport}
	liveReq, err := http.NewRequest("GET", "https://app.internal/", nil)
	if err != nil {
		t.Fatal(err)
	}
	liveReq.Host = "app.internal"
	addBrowserHeaders(liveReq)
	liveResp, err := httpsClient.Do(liveReq)
	if err != nil {
		t.Fatalf("live HTTPS handshake trusting Root CA failed: %v", err)
	}
	liveBody, _ := io.ReadAll(liveResp.Body)
	_ = liveResp.Body.Close()
	if liveResp.StatusCode != http.StatusOK || string(liveBody) != "local-ok:app-upstream:3000" {
		t.Fatalf("unexpected HTTPS response: %d %q", liveResp.StatusCode, string(liveBody))
	}

	// 3. rt.Close() removes managed hosts lines cleanly while preserving user lines
	if err := rt.Close(); err != nil {
		t.Fatalf("rt.Close() failed: %v", err)
	}
	cleanedRaw, err := os.ReadFile(hostsFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(cleanedRaw) != initialHosts {
		t.Fatalf("hosts file after Close():\ngot:\n%s\nwant:\n%s", string(cleanedRaw), initialHosts)
	}

	// 4. Permission-denied hosts file returns ErrPermissionDenied on SetupHosts and closes cleanly
	if os.Geteuid() != 0 {
		roFile := filepath.Join(hostsDir, "readonly-hosts")
		if err := os.WriteFile(roFile, []byte("127.0.0.1 localhost\n"), 0o400); err != nil {
			t.Fatal(err)
		}
		roRT, err := BuildRuntime(ctx, cfg, RuntimeOptions{HostsFilePath: roFile})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := roRT.SetupHosts(); !errors.Is(err, localpki.ErrPermissionDenied) {
			t.Fatalf("expected ErrPermissionDenied on read-only hosts file, got %v", err)
		}
		if err := roRT.Close(); err != nil {
			t.Fatalf("expected clean Close() even when hosts file is read-only, got %v", err)
		}
	}
}

func TestSSLProviderSwitchingAutoRenewalAndAdminTLSAPI(t *testing.T) {
	ctx := context.Background()
	certDir := t.TempDir()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	autoEABMAC := []byte("zerossl-auto-eab-secret-hmac-key")
	explicitEABMAC := []byte("google-explicit-eab-secret-hmac!")
	explicitEABB64 := base64.RawURLEncoding.EncodeToString(explicitEABMAC)

	var (
		boardRef          atomic.Pointer[tlsmanager.HTTP01Board]
		serialSeq         atomic.Int64
		sawZeroSSLAutoEAB atomic.Bool
		sawGoogleEAB      atomic.Bool
		lastCSRByHost     = make(map[string][]byte)
	)

	b64Decode := func(raw string) []byte {
		out, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(raw, "="))
		if err != nil {
			t.Fatalf("base64url decode failed: %v", err)
		}
		return out
	}

	jsonHTTPResp := func(status int, payload any, hdrs map[string]string) *http.Response {
		b, _ := json.Marshal(payload)
		h := http.Header{"Content-Type": []string{"application/json"}}
		for k, v := range hdrs {
			h.Set(k, v)
		}
		return &http.Response{
			StatusCode: status,
			Header:     h,
			Body:       io.NopCloser(bytes.NewReader(b)),
		}
	}

	mockACMEHTTP := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			// ZeroSSL auto-EAB endpoint
			if req.URL.String() == tlsmanager.ZeroSSLAutoEABURL {
				rawForm, _ := io.ReadAll(req.Body)
				vals, _ := url.ParseQuery(string(rawForm))
				if vals.Get("email") != "ops@example.com" {
					t.Fatalf("unexpected ZeroSSL auto-EAB email: %q", string(rawForm))
				}
				sawZeroSSLAutoEAB.Store(true)
				return jsonHTTPResp(http.StatusOK, map[string]any{
					"success":      true,
					"eab_kid":      "zssl-auto-kid-1",
					"eab_hmac_key": base64.RawURLEncoding.EncodeToString(autoEABMAC),
				}, nil), nil
			}

			hostName := req.URL.Host
			baseURL := "https://" + hostName
			path := req.URL.Path
			nonceHdr := map[string]string{"Replay-Nonce": "nonce-" + hostName}

			issuerCN := "Unknown Mock CA"
			validDays := 90
			switch hostName {
			case "acme.zerossl.com":
				issuerCN = "ZeroSSL ECC Domain Secure Site CA"
			case "dv.acme-v02.api.pki.goog":
				issuerCN = "GTS CA 1P5"
			case "acme.litessl.com":
				issuerCN = "LiteSSL TrustAsia DV CA"
				validDays = 15 // Expiring within 30 days to verify RenewIfNeeded!
			default:
				t.Fatalf("unexpected ACME host: %s", hostName)
			}

			// Normalize provider-specific directory paths (/v2/DV90, /directory, /acme/v2/directory)
			if path == "/v2/DV90" || path == "/directory" || path == "/acme/v2/directory" {
				return jsonHTTPResp(http.StatusOK, map[string]string{
					"newNonce":   baseURL + "/new-nonce",
					"newAccount": baseURL + "/new-acct",
					"newOrder":   baseURL + "/new-order",
				}, nil), nil
			}

			switch path {
			case "/new-nonce":
				return jsonHTTPResp(http.StatusNoContent, nil, nonceHdr), nil
			case "/new-acct":
				rawBody, _ := io.ReadAll(req.Body)
				var outerJWS map[string]string
				_ = json.Unmarshal(rawBody, &outerJWS)
				var acctPayload map[string]any
				_ = json.Unmarshal(b64Decode(outerJWS["payload"]), &acctPayload)

				verifyEAB := func(wantKID string, wantKey []byte) {
					eabMap, ok := acctPayload["externalAccountBinding"].(map[string]any)
					if !ok {
						t.Fatalf("expected externalAccountBinding on %s, got %+v", hostName, acctPayload)
					}
					prot := eabMap["protected"].(string)
					pay := eabMap["payload"].(string)
					sig := b64Decode(eabMap["signature"].(string))
					var hdr map[string]any
					_ = json.Unmarshal(b64Decode(prot), &hdr)
					if hdr["kid"] != wantKID || hdr["alg"] != "HS256" {
						t.Fatalf("unexpected EAB header on %s: %+v", hostName, hdr)
					}
					mac := hmac.New(sha256.New, wantKey)
					mac.Write([]byte(prot + "." + pay))
					if !hmac.Equal(sig, mac.Sum(nil)) {
						t.Fatalf("EAB HMAC signature mismatch on %s", hostName)
					}
				}

				if hostName == "acme.zerossl.com" {
					verifyEAB("zssl-auto-kid-1", autoEABMAC)
				} else if hostName == "dv.acme-v02.api.pki.goog" {
					if _, onlyExisting := acctPayload["onlyReturnExisting"]; !onlyExisting {
						verifyEAB("gts-kid-777", explicitEABMAC)
						sawGoogleEAB.Store(true)
					}
				}
				nonceHdr["Location"] = baseURL + "/acct/1"
				return jsonHTTPResp(http.StatusCreated, map[string]any{"status": "valid"}, nonceHdr), nil

			case "/new-order":
				nonceHdr["Location"] = baseURL + "/order/1"
				return jsonHTTPResp(http.StatusCreated, map[string]any{
					"status":         "pending",
					"authorizations": []string{baseURL + "/authz/1"},
					"finalize":       baseURL + "/finalize/1",
				}, nonceHdr), nil

			case "/authz/1":
				b := boardRef.Load()
				if b != nil {
					if _, ok := b.Lookup("tok-provider"); ok {
						return jsonHTTPResp(http.StatusOK, map[string]any{"status": "valid"}, nonceHdr), nil
					}
				}
				return jsonHTTPResp(http.StatusOK, map[string]any{
					"status": "pending",
					"challenges": []map[string]string{
						{"type": "http-01", "token": "tok-provider", "url": baseURL + "/chall/1"},
					},
				}, nonceHdr), nil

			case "/chall/1":
				return jsonHTTPResp(http.StatusOK, map[string]any{"status": "valid"}, nonceHdr), nil

			case "/finalize/1":
				rawBody, _ := io.ReadAll(req.Body)
				var jws map[string]string
				_ = json.Unmarshal(rawBody, &jws)
				var csrObj map[string]string
				_ = json.Unmarshal(b64Decode(jws["payload"]), &csrObj)
				lastCSRByHost[hostName] = b64Decode(csrObj["csr"])
				return jsonHTTPResp(http.StatusOK, map[string]any{
					"status":      "valid",
					"certificate": baseURL + "/cert/1",
				}, nonceHdr), nil

			case "/cert/1":
				csr, err := x509.ParseCertificateRequest(lastCSRByHost[hostName])
				if err != nil {
					t.Fatal(err)
				}
				seq := serialSeq.Add(1)
				tmpl := &x509.Certificate{
					SerialNumber: big.NewInt(5000 + seq),
					Subject:      csr.Subject,
					Issuer:       pkix.Name{CommonName: issuerCN},
					NotBefore:    time.Now().Add(-5 * time.Minute),
					NotAfter:     time.Now().Add(time.Duration(validDays) * 24 * time.Hour),
					DNSNames:     csr.DNSNames,
					KeyUsage:     x509.KeyUsageDigitalSignature,
					ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
				}
				parent := &x509.Certificate{Subject: pkix.Name{CommonName: issuerCN}}
				der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, csr.PublicKey, caKey)
				if err != nil {
					t.Fatal(err)
				}
				if b := boardRef.Load(); b != nil {
					b.Clear()
				}
				certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Replay-Nonce": []string{"nonce-cert"}},
					Body:       io.NopCloser(bytes.NewReader(certPEM)),
				}, nil
			default:
				t.Fatalf("unexpected path %s on %s", path, hostName)
				return nil, nil
			}
		}),
	}

	// 1. Start with ZeroSSL using automatic email-to-EAB provisioning
	cfg, err := config.ParseCaddyfile(`
		{
			secret provider-secret
			cert_dir ` + certDir + `
			email ops@example.com
			ssl_provider zerossl
		}

		portal.example.com {
			reverse_proxy http://portal-upstream:3000
			verify
		}
	`)
	if err != nil {
		t.Fatalf("ParseCaddyfile failed: %v", err)
	}

	rt, err := BuildRuntime(ctx, cfg, RuntimeOptions{ACMEHTTP: mockACMEHTTP})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	boardRef.Store(rt.ACMEBoard)

	zsslMat, err := rt.CertManager.Obtain(ctx)
	if err != nil {
		t.Fatalf("ZeroSSL Obtain failed: %v", err)
	}
	rt.HSTS.Store(tlsmanager.HSTSEnabled(cfg.TLS.HSTS, zsslMat.Source))
	if !sawZeroSSLAutoEAB.Load() || filepath.Base(zsslMat.CertPath) != "acme-zerossl-fullchain.pem" {
		t.Fatalf("expected ZeroSSL auto-EAB and acme-zerossl-fullchain.pem, got sawAuto=%v mat=%+v", sawZeroSSLAutoEAB.Load(), zsslMat)
	}

	adminCall := func(method, path string, payload any) *httptest.ResponseRecorder {
		var body io.Reader
		if payload != nil {
			b, _ := json.Marshal(payload)
			body = bytes.NewReader(b)
		}
		req := httptest.NewRequest(method, "http://127.0.0.1:2019"+path, body)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		rt.AdminHandler.ServeHTTP(rec, req)
		return rec
	}

	// 2. Verify GET /api/v1/tls returns active TLS source, cert path, provider, HSTS, and full SSL provider catalog
	tlsRec := adminCall("GET", "/api/v1/tls", nil)
	if tlsRec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/tls = %d %s", tlsRec.Code, tlsRec.Body.String())
	}
	var tlsStatus TLSStatus
	if err := json.Unmarshal(tlsRec.Body.Bytes(), &tlsStatus); err != nil {
		t.Fatalf("decode TLSStatus: %v", err)
	}
	if tlsStatus.Source != "acme" || tlsStatus.Provider != "zerossl" || !tlsStatus.HSTS ||
		!strings.HasSuffix(tlsStatus.CertPath, "acme-zerossl-fullchain.pem") {
		t.Fatalf("unexpected TLSStatus: %+v", tlsStatus)
	}
	wantProviders := []string{"letsencrypt", "letsencrypt_staging", "zerossl", "google", "ssl.com", "ssl.com-rsa", "litessl", "buypass"}
	gotNames := make([]string, 0, len(tlsStatus.Providers))
	for _, p := range tlsStatus.Providers {
		gotNames = append(gotNames, p.Name)
	}
	for _, want := range wantProviders {
		if !slices.Contains(gotNames, want) {
			t.Fatalf("GET /api/v1/tls missing catalog provider %q in %v", want, gotNames)
		}
	}

	// 3. Verify POST /api/v1/tls/renew triggers ForceRenew via REST API and rotates the certificate serial
	renewRec := adminCall("POST", "/api/v1/tls/renew", nil)
	if renewRec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/tls/renew = %d %s", renewRec.Code, renewRec.Body.String())
	}
	var renewedStatus TLSStatus
	_ = json.Unmarshal(renewRec.Body.Bytes(), &renewedStatus)
	if renewedStatus.SerialNumber == "" || renewedStatus.SerialNumber == tlsStatus.SerialNumber {
		t.Fatalf("expected POST /api/v1/tls/renew to rotate serial, before=%s after=%s", tlsStatus.SerialNumber, renewedStatus.SerialNumber)
	}

	// 4. Switch SSL provider live via PUT /api/v1/config to Google Trust Services (with EAB) and renew!
	googleCfg := rt.Snapshot()
	googleCfg.TLS.Provider = "google"
	googleCfg.TLS.Mode = "acme"
	googleCfg.TLS.ACMEDirectory = ""
	googleCfg.TLS.EABKID = "gts-kid-777"
	googleCfg.TLS.EABHMACKey = explicitEABB64

	putRec := adminCall("PUT", "/api/v1/config", googleCfg)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT /api/v1/config (google) = %d %s", putRec.Code, putRec.Body.String())
	}
	if !strings.Contains(putRec.Body.String(), `"eab_hmac_key":"***REDACTED***"`) {
		t.Fatalf("expected eab_hmac_key to be redacted in PUT /api/v1/config response: %s", putRec.Body.String())
	}

	gtsRenewRec := adminCall("POST", "/api/v1/tls/renew", nil)
	if gtsRenewRec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/tls/renew (google) = %d %s", gtsRenewRec.Code, gtsRenewRec.Body.String())
	}
	var gtsStatus TLSStatus
	_ = json.Unmarshal(gtsRenewRec.Body.Bytes(), &gtsStatus)
	if !sawGoogleEAB.Load() || gtsStatus.Provider != "google" || gtsStatus.Issuer != "GTS CA 1P5" ||
		!strings.HasSuffix(gtsStatus.CertPath, "acme-google-fullchain.pem") {
		t.Fatalf("unexpected Google Trust Services renewal status: sawEAB=%v status=%+v", sawGoogleEAB.Load(), gtsStatus)
	}

	// 5. Switch to LiteSSL (TrustAsia) with 15-day cert and verify automatic RenewIfNeeded(ctx, 30)
	liteCfg := rt.Snapshot()
	liteCfg.TLS.Provider = "litessl"
	liteCfg.TLS.Mode = "acme"
	liteCfg.TLS.ACMEDirectory = ""
	liteCfg.TLS.EABKID = ""
	liteCfg.TLS.EABHMACKey = ""
	if err := rt.ReplaceConfig(liteCfg); err != nil {
		t.Fatalf("ReplaceConfig(litessl) failed: %v", err)
	}
	liteMat1, err := rt.CertManager.ForceRenew(ctx)
	if err != nil || filepath.Base(liteMat1.CertPath) != "acme-litessl-fullchain.pem" {
		t.Fatalf("LiteSSL ForceRenew failed: %+v, %v", liteMat1, err)
	}
	liteMat2, err := rt.CertManager.RenewIfNeeded(ctx, 30)
	if err != nil || liteMat2 == nil || liteMat2.Cert.Leaf.SerialNumber.Cmp(liteMat1.Cert.Leaf.SerialNumber) == 0 {
		t.Fatalf("expected 15-day LiteSSL cert to auto-renew via RenewIfNeeded(30), mat2=%+v err=%v", liteMat2, err)
	}

	// 6. Verify encrypted private key (cfg.TLS.KeyPassword) works with FileProvider & per-site SNI
	encDir := t.TempDir()
	rawCertPEM, rawKeyPEM, err := tlsmanager.BuildSelfSigned([]string{"vault.example.com"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := tlsmanager.ParsePrivateKeyPEM(rawKeyPEM, "")
	if err != nil {
		t.Fatal(err)
	}
	encKeyPEM, err := tlsmanager.EncryptPKCS8PrivateKeyPEM(signer, "vault-pass-999")
	if err != nil {
		t.Fatal(err)
	}
	encCertFile := filepath.Join(encDir, "vault.pem")
	encKeyFile := filepath.Join(encDir, "vault.key")
	_ = os.WriteFile(encCertFile, rawCertPEM, 0o644)
	_ = os.WriteFile(encKeyFile, encKeyPEM, 0o600)

	encCfg, err := config.ParseCaddyfile(`
		{
			secret vault-secret
			cert_dir ` + encDir + `
			tls ` + encCertFile + ` ` + encKeyFile + `
			key_password vault-pass-999
		}

		vault.example.com {
			reverse_proxy http://vault-upstream:8080
			tls ` + encCertFile + ` ` + encKeyFile + `
			verify
		}
	`)
	if err != nil {
		t.Fatalf("ParseCaddyfile (encrypted key) failed: %v", err)
	}
	encRT, err := BuildRuntime(ctx, encCfg, RuntimeOptions{})
	if err != nil {
		t.Fatalf("BuildRuntime with encrypted private key failed: %v", err)
	}
	defer encRT.Close()
	encMat, err := encRT.CertManager.Obtain(ctx)
	if err != nil || encMat.Source != "file" {
		t.Fatalf("Obtain with encrypted private key failed: %+v, %v", encMat, err)
	}
}
