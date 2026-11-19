package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"gymbag/internal/config"
	"gymbag/internal/waf"
)

func TestCustomVerifyTimeoutAndRemoteRulesEndToEnd(t *testing.T) {
	remoteRulesText := `
# Remote rule: block /remote-banned and verify /remote-challenge
block path /remote-banned*
verify path /remote-challenge*
`
	remoteURL := "https://rules.example.test/site.rules"

	cfg, err := config.ParseCaddyfile(`
		{
			secret test-secret
			cert_dir ` + t.TempDir() + `
			tls selfsigned
			cookie_secure false
			difficulty 1
			remote_rules_refresh 2m
		}

		app.example.com {
			reverse_proxy http://app-upstream:3000
			verify 3
			verify_timeout 45m
			allow path /local-override
			remote_rules ` + remoteURL + `
		}
	`)
	if err != nil {
		t.Fatalf("ParseCaddyfile failed: %v", err)
	}

	rt, err := BuildRuntime(context.Background(), cfg, RuntimeOptions{
		HTTPTransport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
				Body:       io.NopCloser(strings.NewReader("ok:" + req.URL.Path)),
			}, nil
		}),
		RemoteRulesHTTP: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.String() == remoteURL {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"ETag": []string{`"r1"`}},
						Body:       io.NopCloser(strings.NewReader(remoteRulesText)),
					}, nil
				}
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Header:     http.Header{},
					Body:       io.NopCloser(strings.NewReader("not found")),
				}, nil
			}),
		},
	})
	if err != nil {
		t.Fatalf("BuildRuntime failed: %v", err)
	}
	defer rt.Close()

	// Refresh remote rules via Admin API POST /api/v1/rules/remote/refresh
	refReq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:2019/api/v1/rules/remote/refresh", nil)
	refRec := httptest.NewRecorder()
	rt.AdminHandler.ServeHTTP(refRec, refReq)
	if refRec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/rules/remote/refresh = %d: %s", refRec.Code, refRec.Body.String())
	}
	var remoteSt waf.RemoteRulesStatus
	if err := json.Unmarshal(refRec.Body.Bytes(), &remoteSt); err != nil {
		t.Fatalf("decode remote status: %v", err)
	}
	if remoteSt.TotalRules != 2 || remoteSt.RefreshIntervalSec != 120 {
		t.Fatalf("unexpected remote rules status: %+v", remoteSt)
	}

	// GET /api/v1/rules/remote works too
	getRefRec := httptest.NewRecorder()
	rt.AdminHandler.ServeHTTP(getRefRec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:2019/api/v1/rules/remote", nil))
	if getRefRec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/rules/remote = %d", getRefRec.Code)
	}

	send := func(method, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "https://app.example.com"+path, nil)
		req.Host = "app.example.com"
		req.RemoteAddr = "203.0.113.77:12345"
		addBrowserHeaders(req)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		rt.HTTPSHandler.ServeHTTP(rec, req)
		return rec
	}

	// 1. Remote block rule blocks /remote-banned/secret with 403 reject page
	recBanned := send(http.MethodGet, "/remote-banned/secret", nil)
	if recBanned.Code != http.StatusForbidden || !strings.Contains(recBanned.Body.String(), "Request blocked") {
		t.Fatalf("expected remote block 403, got %d body=%q", recBanned.Code, recBanned.Body.String())
	}

	// 2. Remote verify rule challenges /remote-challenge/item even for normal browser
	recChal := send(http.MethodGet, "/remote-challenge/item", nil)
	if recChal.Code != http.StatusForbidden || !strings.Contains(recChal.Body.String(), "gymbag-challenge") {
		t.Fatalf("expected remote verify challenge, got %d", recChal.Code)
	}

	// Solve PoW and verify custom 45m (2700s) verified timeout on cookie & session endpoint
	specMatch := regexp.MustCompile(`<script type="application/json" id="gymbag-challenge">(.*?)</script>`).FindStringSubmatch(recChal.Body.String())
	if len(specMatch) != 2 {
		t.Fatal("missing challenge spec")
	}
	var spec struct {
		ChallengeID string `json:"challenge_id"`
		Difficulty  int    `json:"difficulty"`
	}
	_ = json.Unmarshal([]byte(specMatch[1]), &spec)
	var nonce int64
	for !waf.Accepts(spec.ChallengeID, nonce, spec.Difficulty) {
		nonce++
	}
	verifyBody, _ := json.Marshal(map[string]any{
		"challenge_id": spec.ChallengeID,
		"nonce":        nonce,
		"verified":     true,
	})
	vReq := httptest.NewRequest(http.MethodPost, "https://app.example.com/.waf/v1/verifications", bytes.NewReader(verifyBody))
	vReq.Host = "app.example.com"
	vReq.RemoteAddr = "203.0.113.77:12345"
	vReq.Header.Set("Content-Type", "application/json")
	vRec := httptest.NewRecorder()
	rt.HTTPSHandler.ServeHTTP(vRec, vReq)
	if vRec.Code != http.StatusOK {
		t.Fatalf("verification failed: %d %s", vRec.Code, vRec.Body.String())
	}
	var verifyResp struct {
		OK        bool `json:"ok"`
		ExpiresIn int  `json:"expires_in"`
	}
	_ = json.Unmarshal(vRec.Body.Bytes(), &verifyResp)
	if !verifyResp.OK || verifyResp.ExpiresIn != 2700 {
		t.Fatalf("unexpected verify response: %+v", verifyResp)
	}
	cookies := vRec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge != 2700 {
		t.Fatalf("expected cookie MaxAge=2700, got %+v", cookies)
	}

	// Session endpoint reports clearance_ttl=2700 and expires_at when cleared
	sessRec := send(http.MethodGet, "/.waf/v1/session", cookies[0])
	if sessRec.Code != http.StatusOK {
		t.Fatalf("session status = %d", sessRec.Code)
	}
	var sessPayload struct {
		Cleared      bool   `json:"cleared"`
		ClearanceTTL int    `json:"clearance_ttl"`
		ExpiresAt    string `json:"expires_at"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	_ = json.Unmarshal(sessRec.Body.Bytes(), &sessPayload)
	if !sessPayload.Cleared || sessPayload.ClearanceTTL != 2700 || sessPayload.ExpiresAt == "" || sessPayload.ExpiresIn <= 0 {
		t.Fatalf("unexpected session payload: %+v", sessPayload)
	}

	// With valid clearance cookie, /remote-challenge/item passes to upstream
	recCleared := send(http.MethodGet, "/remote-challenge/item", cookies[0])
	if recCleared.Code != http.StatusOK || recCleared.Body.String() != "ok:/remote-challenge/item" {
		t.Fatalf("cleared request failed: %d %q", recCleared.Code, recCleared.Body.String())
	}
}
