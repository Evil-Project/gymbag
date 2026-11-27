package server

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"gymbag/internal/config"
	"gymbag/internal/waf"
)

func reserveUnusedPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func waitForHTTPReady(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("listener %s did not become ready in time", addr)
}

func solvePoWNonce(t *testing.T, challengeID string, difficulty int) int64 {
	t.Helper()
	for n := int64(0); n < 5_000_000; n++ {
		if waf.Accepts(challengeID, n, difficulty) {
			return n
		}
	}
	t.Fatalf("failed to solve difficulty %d challenge", difficulty)
	return -1
}

func writeTestAcmeShCert(t *testing.T, dir, domain string) *big.Int {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 64))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: domain},
		Issuer:                pkix.Name{CommonName: domain},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(60 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{domain},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	domDir := filepath.Join(dir, domain+"_ecc")
	if err := os.MkdirAll(domDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(domDir, "fullchain.cer"), certPEM, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(domDir, domain+".key"), keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return serial
}

func TestHTTPOnlyDisabledAutoSSLRuntime(t *testing.T) {
	var upstreamAddr string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			w.Header().Set("Location", fmt.Sprintf("http://%s/after", upstreamAddr))
			w.WriteHeader(http.StatusFound)
			return
		}
		_, _ = io.WriteString(w, "plain-http-ok:"+r.Header.Get("X-Forwarded-Proto"))
	}))
	defer upstream.Close()
	upstreamAddr = strings.TrimPrefix(upstream.URL, "http://")

	certDir := filepath.Join(t.TempDir(), "certs")
	httpPort := reserveUnusedPort(t)
	httpsPort := reserveUnusedPort(t)

	cfg := config.Default()
	cfg.BindHost = "127.0.0.1"
	cfg.HTTPPort = httpPort
	cfg.HTTPSPort = httpsPort
	cfg.TLS.Mode = "off"
	f := false
	cfg.TLS.AutoSSL = &f
	cfg.TLS.AutoHosts = &f
	cfg.TLS.CertDir = certDir
	cfg.Intel = config.IntelConfig{IPSecurity: false, LiveReputation: false, AIIPList: false, VPNDetection: false, RefreshInterval: 3600}
	cfg.Sites = []config.SiteConfig{
		{
			Domain:    "localhost",
			Upstreams: []string{upstream.URL},
			Protect:   true,
			WAF: config.SiteWAFConfig{
				VerifyLevel:  config.VerifyLevelAlways,
				Difficulty:   1,
				ChallengeTTL: 300,
				ClearanceTTL: 3600,
				CookieName:   "waf_clearance",
				CookieSecure: true,
				RateLimit:    100,
				RateWindow:   60,
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt, err := BuildRuntime(ctx, cfg, RuntimeOptions{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- rt.Serve(ctx) }()
	httpAddr := fmt.Sprintf("127.0.0.1:%d", httpPort)
	waitForHTTPReady(t, httpAddr)

	// Confirm HTTPS port is NOT bound and TLSStatus reports disabled
	if c, dialErr := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", httpsPort), 150*time.Millisecond); dialErr == nil {
		_ = c.Close()
		t.Fatal("expected HTTPS port NOT to be bound when tls.mode=off")
	}
	tlsSt := rt.TLSStatus()
	if tlsSt.Mode != "off" || tlsSt.Source != "disabled" || !tlsSt.DirectHTTP || tlsSt.HSTS {
		t.Fatalf("unexpected TLSStatus when disabled: %+v", tlsSt)
	}

	// Ensure no TLS certificate files were generated inside certDir
	entries, _ := os.ReadDir(certDir)
	for _, ent := range entries {
		if strings.HasSuffix(ent.Name(), ".pem") || strings.HasSuffix(ent.Name(), ".key") {
			t.Fatalf("unexpected TLS file generated when auto_ssl disabled: %s", ent.Name())
		}
	}

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	baseURL := fmt.Sprintf("http://localhost:%d", httpPort)

	// 1. Initial plain HTTP GET receives PoW challenge (NOT 308 redirect, NO HSTS)
	resp, err := client.Get(baseURL + "/")
	if err != nil {
		t.Fatalf("HTTP GET / failed: %v", err)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusPermanentRedirect || resp.StatusCode == http.StatusMovedPermanently {
		t.Fatalf("expected PoW challenge over direct HTTP, got redirect %d", resp.StatusCode)
	}
	if resp.Header.Get("Strict-Transport-Security") != "" {
		t.Fatalf("expected no HSTS header on plain HTTP response, got %q", resp.Header.Get("Strict-Transport-Security"))
	}
	m := regexp.MustCompile(`id=([^;\s"]+)`).FindSubmatch(bodyBytes)
	if len(m) < 2 {
		// Challenge page embeds challenge ID in JSON/HTML
		m = regexp.MustCompile(`"challenge_id"\s*:\s*"([^"]+)"`).FindSubmatch(bodyBytes)
	}
	var chalID string
	if len(m) >= 2 {
		chalID = string(m[1])
	} else {
		// Issue via REST PoW endpoint if HTML format differs
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/.waf/v1/challenges", nil)
		cResp, cErr := client.Do(req)
		if cErr != nil {
			t.Fatal(cErr)
		}
		var out map[string]any
		_ = json.NewDecoder(cResp.Body).Decode(&out)
		_ = cResp.Body.Close()
		chalID, _ = out["challenge_id"].(string)
	}
	if chalID == "" {
		t.Fatalf("could not obtain challenge_id from HTTP PoW response:\n%s", string(bodyBytes))
	}

	// 2. Submit PoW solution over plain HTTP -> clearance cookie must NOT have Secure=true
	nonce := solvePoWNonce(t, chalID, 1)
	payload, _ := json.Marshal(map[string]any{
		"challenge_id": chalID,
		"nonce":        nonce,
		"verified":     true,
	})
	vReq, _ := http.NewRequest(http.MethodPost, baseURL+"/.waf/v1/verifications", bytes.NewReader(payload))
	vReq.Header.Set("Content-Type", "application/json")
	vResp, err := client.Do(vReq)
	if err != nil {
		t.Fatalf("PoW verification over HTTP failed: %v", err)
	}
	_ = vResp.Body.Close()
	if vResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from PoW verification, got %d", vResp.StatusCode)
	}
	cookies := vResp.Cookies()
	if len(cookies) == 0 || cookies[0].Name != "waf_clearance" {
		t.Fatalf("expected waf_clearance cookie, got %+v", cookies)
	}
	if cookies[0].Secure {
		t.Fatal("expected waf_clearance cookie NOT to set Secure over plain HTTP")
	}

	// 3. Follow up with clearance cookie -> proxied response + X-Forwarded-Proto=http
	pReq, _ := http.NewRequest(http.MethodGet, baseURL+"/", nil)
	pReq.AddCookie(cookies[0])
	pResp, err := client.Do(pReq)
	if err != nil {
		t.Fatal(err)
	}
	pBody, _ := io.ReadAll(pResp.Body)
	_ = pResp.Body.Close()
	if pResp.StatusCode != http.StatusOK || string(pBody) != "plain-http-ok:http" {
		t.Fatalf("unexpected proxied HTTP response: status=%d body=%q", pResp.StatusCode, string(pBody))
	}

	// 4. Location rewrite on plain HTTP rewrites to http://localhost:<httpPort>/after
	rReq, _ := http.NewRequest(http.MethodGet, baseURL+"/redirect", nil)
	rReq.AddCookie(cookies[0])
	rResp, err := client.Do(rReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = rResp.Body.Close()
	wantLoc := fmt.Sprintf("http://localhost:%d/after", httpPort)
	if gotLoc := rResp.Header.Get("Location"); gotLoc != wantLoc {
		t.Fatalf("expected Location %q on direct HTTP, got %q", wantLoc, gotLoc)
	}

	cancel()
	select {
	case serveErr := <-errCh:
		if serveErr != nil {
			t.Fatalf("Serve returned error: %v", serveErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Serve shutdown")
	}
}

func TestDirectHTTPAlongsideHTTPSWithAcmeShAndHotReload(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "proto="+r.Header.Get("X-Forwarded-Proto")+",host="+r.Header.Get("X-Forwarded-Host"))
	}))
	defer upstream.Close()

	acmeHome := t.TempDir()
	alphaSerial := writeTestAcmeShCert(t, acmeHome, "alpha.example.test")
	betaSerial1 := writeTestAcmeShCert(t, acmeHome, "beta.example.test")

	httpPort := reserveUnusedPort(t)
	httpsPort := reserveUnusedPort(t)
	f := false

	cfg := config.Default()
	cfg.BindHost = "127.0.0.1"
	cfg.HTTPPort = httpPort
	cfg.HTTPSPort = httpsPort
	cfg.DirectHTTP = true
	cfg.TLS.AutoSSL = &f
	cfg.TLS.AcmeShPath = acmeHome
	cfg.TLS.AutoHosts = &f
	cfg.TLS.CertDir = filepath.Join(t.TempDir(), "certs")
	cfg.Intel = config.IntelConfig{IPSecurity: false, LiveReputation: false, AIIPList: false, VPNDetection: false, RefreshInterval: 3600}
	cfg.Sites = []config.SiteConfig{
		{Domain: "alpha.example.test", Upstreams: []string{upstream.URL}, Protect: false, WAF: config.DefaultSiteWAFConfig()},
		{Domain: "beta.example.test", Upstreams: []string{upstream.URL}, Protect: false, WAF: config.DefaultSiteWAFConfig()},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt, err := BuildRuntime(ctx, cfg, RuntimeOptions{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	rt.ACMEBoard.Publish("tok-123", "authz-xyz")

	errCh := make(chan error, 1)
	go func() { errCh <- rt.Serve(ctx) }()
	httpAddr := fmt.Sprintf("127.0.0.1:%d", httpPort)
	httpsAddr := fmt.Sprintf("127.0.0.1:%d", httpsPort)
	waitForHTTPReady(t, httpAddr)
	waitForHTTPReady(t, httpsAddr)

	// 1. Plain HTTP direct request -> proxies directly, no HSTS
	httpClient := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	hReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/", httpAddr), nil)
	hReq.Host = fmt.Sprintf("alpha.example.test:%d", httpPort)
	hResp, err := httpClient.Do(hReq)
	if err != nil {
		t.Fatal(err)
	}
	hBody, _ := io.ReadAll(hResp.Body)
	_ = hResp.Body.Close()
	if hResp.StatusCode != http.StatusOK || !strings.Contains(string(hBody), "proto=http") {
		t.Fatalf("expected direct HTTP 200 proxied body, got status=%d body=%q", hResp.StatusCode, string(hBody))
	}
	if hResp.Header.Get("Strict-Transport-Security") != "" {
		t.Fatal("expected no HSTS on direct HTTP response even when HTTPS HSTS is active")
	}

	// 2. ACME HTTP-01 challenge still served on direct HTTP listener
	chalResp, err := httpClient.Get(fmt.Sprintf("http://%s/.well-known/acme-challenge/tok-123", httpAddr))
	if err != nil {
		t.Fatal(err)
	}
	chalBody, _ := io.ReadAll(chalResp.Body)
	_ = chalResp.Body.Close()
	if chalResp.StatusCode != http.StatusOK || string(chalBody) != "authz-xyz" {
		t.Fatalf("expected ACME HTTP-01 response on direct HTTP listener, got %d %q", chalResp.StatusCode, string(chalBody))
	}

	// 3. HTTPS requests verify SNI selection for alpha and beta + HSTS on HTTPS
	dialTLS := func(sniHost string) (*tls.ConnectionState, string) {
		c := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
					ServerName:         sniHost,
				},
				DialContext: func(dctx context.Context, network, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(dctx, network, httpsAddr)
				},
			},
		}
		req, _ := http.NewRequest(http.MethodGet, "https://"+sniHost+"/", nil)
		resp, err := c.Do(req)
		if err != nil {
			t.Fatalf("HTTPS request for %s failed: %v", sniHost, err)
		}
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.Header.Get("Strict-Transport-Security") == "" {
			t.Fatalf("expected HSTS header on HTTPS response for %s", sniHost)
		}
		return resp.TLS, string(b)
	}

	stAlpha, bodyAlpha := dialTLS("alpha.example.test")
	if stAlpha.PeerCertificates[0].SerialNumber.Cmp(alphaSerial) != 0 || !strings.Contains(bodyAlpha, "proto=https") {
		t.Fatalf("unexpected alpha TLS result: serial=%v body=%q", stAlpha.PeerCertificates[0].SerialNumber, bodyAlpha)
	}
	stBeta1, _ := dialTLS("beta.example.test")
	if stBeta1.PeerCertificates[0].SerialNumber.Cmp(betaSerial1) != 0 {
		t.Fatalf("unexpected beta initial TLS serial: %v vs %v", stBeta1.PeerCertificates[0].SerialNumber, betaSerial1)
	}

	// 4. Replace beta cert on disk and trigger RenewTLS -> new cert served seamlessly
	betaSerial2 := writeTestAcmeShCert(t, acmeHome, "beta.example.test")
	if _, err := rt.RenewTLS(ctx); err != nil {
		t.Fatalf("RenewTLS failed: %v", err)
	}
	stBeta2, _ := dialTLS("beta.example.test")
	if stBeta2.PeerCertificates[0].SerialNumber.Cmp(betaSerial2) != 0 {
		t.Fatalf("expected hot-reloaded beta serial %v, got %v", betaSerial2, stBeta2.PeerCertificates[0].SerialNumber)
	}

	cancel()
	select {
	case serveErr := <-errCh:
		if serveErr != nil {
			t.Fatalf("Serve returned error: %v", serveErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Serve shutdown")
	}
}
