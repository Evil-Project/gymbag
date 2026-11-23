package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gymbag/internal/config"
)

func TestRuntimeAutheliaForwardAuthExecution(t *testing.T) {
	defOff := false
	cfg := config.Default()
	cfg.Secret = "test-authelia-secret"
	cfg.TLS.Mode = "selfsigned"
	cfg.TLS.CertDir = t.TempDir()
	cfg.Intel.IPSecurity = false
	cfg.Intel.LiveReputation = false
	cfg.Intel.AIIPList = false
	cfg.Sites = []config.SiteConfig{
		{
			Domain:    "app.example.com",
			Upstreams: []string{"http://backend:3000"},
			Protect:   true,
			WAF: config.SiteWAFConfig{
				VerifyLevel: config.VerifyLevelOnlySuspicious,
			},
			Auth: &config.ForwardAuthConfig{
				Provider:    "authelia",
				URL:         "http://authelia:9091",
				PortalURL:   "https://auth.example.com",
				CopyHeaders: []string{"Remote-User", "Remote-Groups", "Remote-Email>X-User-Email"},
				ExceptPaths: []string{"/healthz"},
				Headers:     map[string]string{"X-Edge-Proxy": "gymbag"},
			},
		},
		{
			Domain:    "portal.example.com",
			Upstreams: []string{"http://backend:3001"},
			Protect:   false,
			WAF: config.SiteWAFConfig{
				VerifyLevel:  config.VerifyLevelAlways,
				DefaultRules: &defOff,
			},
			Routes: []config.RouteConfig{
				{
					Path:      "/admin/*",
					Upstreams: []string{"http://admin-backend:8080"},
					Auth: &config.ForwardAuthConfig{
						Provider: "authelia",
						URL:      "http://authelia:9091",
					},
				},
			},
		},
	}

	var (
		authCalls        int
		lastAuthReqHdrs  http.Header
		lastAuthURL      string
		upstreamCalls    int
		lastUpstreamHdrs http.Header
		authMode         string
	)

	authClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			authCalls++
			lastAuthReqHdrs = req.Header.Clone()
			lastAuthURL = req.URL.String()

			switch authMode {
			case "allow":
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Remote-User":     []string{"alice"},
						"Remote-Email":    []string{"alice@example.com"},
						"Set-Cookie":      []string{"authelia_session=refreshed; Path=/; HttpOnly"},
						"X-Extra-Ignored": []string{"ignore"},
					},
					Body: io.NopCloser(strings.NewReader("")),
				}, nil
			case "redirect":
				return &http.Response{
					StatusCode: http.StatusFound,
					Header: http.Header{
						"Location": []string{"https://auth.example.com/?rd=https%3A%2F%2Fapp.example.com%2Fdashboard"},
					},
					Body: io.NopCloser(strings.NewReader("")),
				}, nil
			case "401":
				return &http.Response{
					StatusCode: http.StatusUnauthorized,
					Header:     http.Header{"Content-Type": []string{"text/plain"}},
					Body:       io.NopCloser(strings.NewReader("Unauthorized")),
				}, nil
			default:
				return nil, context.DeadlineExceeded
			}
		}),
	}

	rt, err := BuildRuntime(context.Background(), cfg, RuntimeOptions{
		AuthHTTPClient: authClient,
		HTTPTransport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			upstreamCalls++
			lastUpstreamHdrs = req.Header.Clone()
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

	send := func(hostName, method, path string, extraHeaders map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "https://"+hostName+path, nil)
		req.Host = hostName
		req.RemoteAddr = "203.0.113.50:54321"
		addBrowserHeaders(req)
		for k, v := range extraHeaders {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		rt.HTTPSHandler.ServeHTTP(rec, req)
		return rec
	}

	// 1. Authenticated 200 OK: strips client-spoofed Remote-User/Remote-Groups, copies Authelia headers & Set-Cookie.
	authMode = "allow"
	authCalls, upstreamCalls = 0, 0
	rec := send("app.example.com", http.MethodGet, "/dashboard?tab=1", map[string]string{
		"Cookie":        "authelia_session=abc",
		"Remote-User":   "evil-spoof",
		"Remote-Groups": "admins",
	})
	if rec.Code != http.StatusOK || rec.Body.String() != "upstream-ok" {
		t.Fatalf("authenticated request failed: code=%d body=%q", rec.Code, rec.Body.String())
	}
	if authCalls != 1 || upstreamCalls != 1 {
		t.Fatalf("expected 1 auth call and 1 upstream call, got auth=%d upstream=%d", authCalls, upstreamCalls)
	}
	if lastAuthURL != "http://authelia:9091/api/authz/forward-auth" {
		t.Fatalf("unexpected auth endpoint URL: %q", lastAuthURL)
	}
	if lastAuthReqHdrs.Get("Remote-User") != "" || lastAuthReqHdrs.Get("Remote-Groups") != "" {
		t.Fatalf("expected spoofed copy headers stripped before auth subrequest, got %+v", lastAuthReqHdrs)
	}
	if lastAuthReqHdrs.Get("X-Forwarded-Uri") != "/dashboard?tab=1" ||
		lastAuthReqHdrs.Get("X-Original-URL") != "https://app.example.com/dashboard?tab=1" ||
		lastAuthReqHdrs.Get("X-Forwarded-Method") != "GET" ||
		lastAuthReqHdrs.Get("X-Edge-Proxy") != "gymbag" {
		t.Fatalf("missing X-Forwarded-* headers on auth subrequest: %+v", lastAuthReqHdrs)
	}
	if lastUpstreamHdrs.Get("Remote-User") != "alice" ||
		lastUpstreamHdrs.Get("X-User-Email") != "alice@example.com" ||
		lastUpstreamHdrs.Get("Remote-Groups") != "" {
		t.Fatalf("unexpected upstream identity headers: %+v", lastUpstreamHdrs)
	}
	if !strings.Contains(rec.Header().Get("Set-Cookie"), "authelia_session=refreshed") {
		t.Fatalf("expected Set-Cookie propagated from Authelia, got %+v", rec.Header())
	}

	// 2. Authelia 302 redirect is returned directly to client without following internally.
	authMode = "redirect"
	authCalls, upstreamCalls = 0, 0
	rec = send("app.example.com", http.MethodGet, "/dashboard", nil)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://auth.example.com/?rd=https%3A%2F%2Fapp.example.com%2Fdashboard" {
		t.Fatalf("expected 302 redirect from Authelia, got code=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
	if upstreamCalls != 0 {
		t.Fatalf("upstream must not be called when auth redirects")
	}

	// 3. Authelia 401 without Location: HTML browser redirects to portal_url?rd=..., JSON/API client gets 401.
	authMode = "401"
	recHTML := send("app.example.com", http.MethodGet, "/settings", map[string]string{"Accept": "text/html"})
	if recHTML.Code != http.StatusFound || recHTML.Header().Get("Location") != "https://auth.example.com?rd=https%3A%2F%2Fapp.example.com%2Fsettings" {
		t.Fatalf("expected browser 401->302 portal redirect, got code=%d loc=%q", recHTML.Code, recHTML.Header().Get("Location"))
	}
	recAPI := send("app.example.com", http.MethodGet, "/api/me", map[string]string{"Accept": "application/json"})
	if recAPI.Code != http.StatusUnauthorized || recAPI.Body.String() != "Unauthorized" {
		t.Fatalf("expected API 401 passthrough, got code=%d body=%q", recAPI.Code, recAPI.Body.String())
	}

	// 4. except_paths bypasses Authelia.
	authCalls, upstreamCalls = 0, 0
	recHealth := send("app.example.com", http.MethodGet, "/healthz", nil)
	if recHealth.Code != http.StatusOK || authCalls != 0 || upstreamCalls != 1 {
		t.Fatalf("expected /healthz to bypass Authelia, got code=%d auth=%d up=%d", recHealth.Code, authCalls, upstreamCalls)
	}

	// 5. Auth service failure fails closed with 502 Bad Gateway.
	authMode = "error"
	upstreamCalls = 0
	recErr := send("app.example.com", http.MethodGet, "/dashboard", nil)
	if recErr.Code != http.StatusBadGateway || upstreamCalls != 0 {
		t.Fatalf("expected 502 fail-closed on auth error, got code=%d up=%d", recErr.Code, upstreamCalls)
	}

	// 6. Route-level auth only applies to /admin/* on portal.example.com.
	authMode = "401"
	authCalls, upstreamCalls = 0, 0
	recPublic := send("portal.example.com", http.MethodGet, "/home", nil)
	if recPublic.Code != http.StatusOK || authCalls != 0 || upstreamCalls != 1 {
		t.Fatalf("expected /home to skip route auth, got code=%d auth=%d up=%d", recPublic.Code, authCalls, upstreamCalls)
	}
	recAdmin := send("portal.example.com", http.MethodGet, "/admin/users", map[string]string{"Accept": "application/json"})
	if recAdmin.Code != http.StatusUnauthorized || authCalls != 1 {
		t.Fatalf("expected /admin/users to hit route auth and return 401, got code=%d auth=%d", recAdmin.Code, authCalls)
	}

	// 7. WebSocket upgrade checks Authelia before dialing upstream.
	authMode = "401"
	authCalls = 0
	wsReq := httptest.NewRequest(http.MethodGet, "https://app.example.com/ws", nil)
	wsReq.Host = "app.example.com"
	wsReq.RemoteAddr = "203.0.113.50:54321"
	addBrowserHeaders(wsReq)
	wsReq.Header.Set("Connection", "Upgrade")
	wsReq.Header.Set("Upgrade", "websocket")
	wsRec := httptest.NewRecorder()
	rt.HTTPSHandler.ServeHTTP(wsRec, wsReq)
	if wsRec.Code != http.StatusUnauthorized || authCalls != 1 {
		t.Fatalf("expected WebSocket rejected by Authelia 401, got code=%d auth=%d", wsRec.Code, authCalls)
	}

	// 8. Admin API PATCH /api/v1/sites/{domain} can clear and set auth live.
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/sites/app.example.com", bytes.NewBufferString(`{"clear_auth":true}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchRec := httptest.NewRecorder()
	rt.AdminHandler.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("PATCH clear_auth failed (%d): %s", patchRec.Code, patchRec.Body.String())
	}
	authCalls, upstreamCalls = 0, 0
	recAfterClear := send("app.example.com", http.MethodGet, "/dashboard", nil)
	if recAfterClear.Code != http.StatusOK || authCalls != 0 || upstreamCalls != 1 {
		t.Fatalf("expected auth bypassed after clear_auth, got code=%d auth=%d up=%d", recAfterClear.Code, authCalls, upstreamCalls)
	}

	// 9. /.well-known/acme-challenge/ and /.waf/ bypass Authelia forward auth.
	authMode = "401"
	rt.ACMEBoard.Publish("acme-tok-1", "acme-keyauth-1")
	defer rt.ACMEBoard.Clear()
	authCalls, upstreamCalls = 0, 0
	recACME := send("portal.example.com", http.MethodGet, "/.well-known/acme-challenge/acme-tok-1", nil)
	if recACME.Code != http.StatusOK || recACME.Body.String() != "acme-keyauth-1" || authCalls != 0 {
		t.Fatalf("expected ACME challenge to bypass Authelia on /admin/ or site auth, got code=%d body=%q auth=%d", recACME.Code, recACME.Body.String(), authCalls)
	}
	// Also verify site-wide auth skips ACME challenge & /.waf/ endpoints cleanly.
	patchSetReq := httptest.NewRequest(http.MethodPatch, "/api/v1/sites/app.example.com", bytes.NewBufferString(`{"auth":{"provider":"authelia","url":"http://authelia:9091"}}`))
	patchSetReq.Header.Set("Content-Type", "application/json")
	patchSetRec := httptest.NewRecorder()
	rt.AdminHandler.ServeHTTP(patchSetRec, patchSetReq)
	if patchSetRec.Code != http.StatusOK {
		t.Fatalf("PATCH set auth failed (%d): %s", patchSetRec.Code, patchSetRec.Body.String())
	}
	authCalls, upstreamCalls = 0, 0
	recACME2 := send("app.example.com", http.MethodGet, "/.well-known/acme-challenge/acme-tok-1", nil)
	if recACME2.Code != http.StatusOK || recACME2.Body.String() != "acme-keyauth-1" || authCalls != 0 || upstreamCalls != 0 {
		t.Fatalf("expected site auth to bypass ACME challenge, got code=%d auth=%d up=%d", recACME2.Code, authCalls, upstreamCalls)
	}
	recWAF := send("app.example.com", http.MethodGet, "/.waf/v1/session", nil)
	if recWAF.Code != http.StatusOK || authCalls != 0 || upstreamCalls != 0 {
		t.Fatalf("expected site auth to bypass /.waf/v1/session, got code=%d body=%q auth=%d up=%d", recWAF.Code, recWAF.Body.String(), authCalls, upstreamCalls)
	}

	// 10. WAF exploit probes are denied before invoking Authelia forward auth.
	for _, probe := range []string{"/.env", "/?q=1+UNION+SELECT+1"} {
		authCalls, upstreamCalls = 0, 0
		recProbe := send("app.example.com", http.MethodGet, probe, nil)
		if recProbe.Code != http.StatusForbidden || authCalls != 0 || upstreamCalls != 0 {
			t.Fatalf("expected WAF to block probe %s before Authelia, got code=%d auth=%d up=%d", probe, recProbe.Code, authCalls, upstreamCalls)
		}
	}
}
