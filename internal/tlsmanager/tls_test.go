package tlsmanager

import (
	"bytes"
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gymbag/internal/config"
)

func TestSelfSignedAndFileProviderMismatch(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	selfProv := &SelfSignedProvider{Directory: dir, Domains: []string{"localhost", "127.0.0.1"}, Days: 1}
	mat, err := selfProv.Issue(ctx)
	if err != nil || mat == nil {
		t.Fatalf("SelfSignedProvider failed: %v", err)
	}
	raw, _ := os.ReadFile(mat.CertPath)
	leaf, err := LoadLeafCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !CertificateCovers(leaf, []string{"localhost", "127.0.0.1"}) {
		t.Fatal("expected cert to cover localhost and 127.0.0.1")
	}
	if exp, _ := CertificateExpiresWithin(mat.CertPath, 30); !exp {
		t.Fatal("expected 1-day cert to expire within 30 days")
	}

	// Long-lived cert + mismatched key test
	certPEM, keyPEM, err := BuildSelfSigned([]string{"localhost"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	_, otherKeyPEM, _ := BuildSelfSigned([]string{"localhost"}, 90)

	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	_ = os.WriteFile(certPath, certPEM, 0o644)
	_ = os.WriteFile(keyPath, otherKeyPEM, 0o600)

	fp := &FileProvider{CertFile: certPath, KeyFile: keyPath, Domains: []string{"localhost"}, RenewWithinDays: 30, Required: false}
	if got, err := fp.Issue(ctx); err != nil || got != nil {
		t.Fatalf("expected nil on mismatched key, got %+v, %v", got, err)
	}

	_ = os.WriteFile(keyPath, keyPEM, 0o600)
	got, err := fp.Issue(ctx)
	if err != nil || got == nil || got.Source != "file" {
		t.Fatalf("expected valid file material, got %+v, %v", got, err)
	}
	if HSTSEnabled(nil, "file") || !HSTSEnabled(nil, "acme") {
		t.Fatal("HSTS auto logic mismatch")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResp(status int, body any, headers map[string]string) *http.Response {
	b, _ := json.Marshal(body)
	h := http.Header{"Content-Type": []string{"application/json"}}
	for k, v := range headers {
		h.Set(k, v)
	}
	return &http.Response{
		StatusCode: status,
		Header:     h,
		Body:       io.NopCloser(bytes.NewReader(b)),
	}
}

func TestJWSSignatureAndACMEHTTP01Flow(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := BuildJWS(key, "https://acme.test/new-acct", "nonce-1", []byte("{}"), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyJWSSignature(&key.PublicKey, doc); err != nil {
		t.Fatalf("VerifyJWSSignature failed: %v", err)
	}
	if err := RejectIneligibleACME([]string{"localhost"}, "owner@example.com"); err == nil {
		t.Fatal("expected localhost to be rejected by ACME")
	}

	board := NewHTTP01Board()
	var seenCSR []byte

	mockHTTP := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			path := req.URL.Path
			nonceHdr := map[string]string{"Replay-Nonce": "nonce-" + strings.Trim(path, "/")}
			switch path {
			case "/directory":
				return jsonResp(200, map[string]string{
					"newNonce":   "https://acme.test/new-nonce",
					"newAccount": "https://acme.test/new-acct",
					"newOrder":   "https://acme.test/new-order",
				}, nil), nil
			case "/new-nonce":
				return jsonResp(204, nil, nonceHdr), nil
			case "/new-acct":
				nonceHdr["Location"] = "https://acme.test/acct/1"
				return jsonResp(201, map[string]any{}, nonceHdr), nil
			case "/new-order":
				nonceHdr["Location"] = "https://acme.test/order/1"
				return jsonResp(201, map[string]any{
					"status":         "pending",
					"authorizations": []string{"https://acme.test/authz/1"},
					"finalize":       "https://acme.test/finalize/1",
				}, nonceHdr), nil
			case "/authz/1":
				if _, ok := board.Lookup("token-1"); ok {
					return jsonResp(200, map[string]any{"status": "valid"}, nonceHdr), nil
				}
				return jsonResp(200, map[string]any{
					"status": "pending",
					"challenges": []map[string]string{
						{"type": "dns-01", "token": "other", "url": "https://acme.test/dns"},
						{"type": "http-01", "token": "token-1", "url": "https://acme.test/chall/1"},
					},
				}, nonceHdr), nil
			case "/chall/1":
				if _, ok := board.Lookup("token-1"); !ok {
					t.Fatal("expected token-1 published on HTTP-01 board")
				}
				return jsonResp(200, map[string]any{}, nonceHdr), nil
			case "/finalize/1":
				bodyBytes, _ := io.ReadAll(req.Body)
				var jws map[string]string
				_ = json.Unmarshal(bodyBytes, &jws)
				payloadRaw, _ := b64urlDecode(jws["payload"])
				var csrObj map[string]string
				_ = json.Unmarshal(payloadRaw, &csrObj)
				seenCSR, _ = b64urlDecode(csrObj["csr"])
				return jsonResp(200, map[string]any{
					"status":      "valid",
					"certificate": "https://acme.test/cert/1",
				}, nonceHdr), nil
			case "/cert/1":
				parsedCSR, err := x509.ParseCertificateRequest(seenCSR)
				if err != nil {
					t.Fatal(err)
				}
				caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
				tmpl := &x509.Certificate{
					SerialNumber: big.NewInt(100),
					Subject:      parsedCSR.Subject,
					Issuer:       pkix.Name{CommonName: "Test ACME CA"},
					NotBefore:    time.Now().Add(-time.Minute),
					NotAfter:     time.Now().Add(90 * 24 * time.Hour),
					DNSNames:     parsedCSR.DNSNames,
					KeyUsage:     x509.KeyUsageDigitalSignature,
					ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
				}
				der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, parsedCSR.PublicKey, caKey)
				if err != nil {
					t.Fatal(err)
				}
				certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
				h := http.Header{"Replay-Nonce": []string{"nonce-cert"}}
				return &http.Response{
					StatusCode: 200,
					Header:     h,
					Body:       io.NopCloser(bytes.NewReader(certPEM)),
				}, nil
			default:
				t.Fatalf("unexpected ACME path: %s", path)
				return nil, nil
			}
		}),
	}

	dir := t.TempDir()
	client := NewACMEClient("https://acme.test/directory", filepath.Join(dir, "acct.pem"), mockHTTP)
	prov := &ACMEProvider{
		Client:    client,
		Board:     board,
		Directory: dir,
		Domains:   []string{"example.com", "api.example.com"},
		Email:     "owner@example.com",
		Required:  true,
	}
	mat, err := prov.Issue(context.Background())
	if err != nil || mat == nil || mat.Source != "acme" {
		t.Fatalf("ACMEProvider.Issue failed: %+v, %v", mat, err)
	}
	csr, err := x509.ParseCertificateRequest(seenCSR)
	if err != nil {
		t.Fatalf("ParseCertificateRequest failed: %v", err)
	}
	if !slices.Contains(csr.DNSNames, "example.com") || !slices.Contains(csr.DNSNames, "api.example.com") {
		t.Fatalf("CSR DNSNames = %v", csr.DNSNames)
	}
	// Board must be cleared after issuance
	if _, ok := board.Lookup("token-1"); ok {
		t.Fatal("expected HTTP-01 board to be cleared after issuance")
	}
}

func TestResolveSSLProviderCatalog(t *testing.T) {
	catalog := ListSSLProviders()
	if len(catalog) < 7 {
		t.Fatalf("expected at least 7 catalog SSL providers, got %d", len(catalog))
	}

	cases := []struct {
		input           string
		wantName        string
		wantURL         string
		wantRequiresEAB bool
		wantAutoEAB     bool
		wantAutoEABURL  string
	}{
		{"", "letsencrypt", "https://acme-v02.api.letsencrypt.org/directory", false, false, ""},
		{"letsencrypt", "letsencrypt", "https://acme-v02.api.letsencrypt.org/directory", false, false, ""},
		{"LE", "letsencrypt", "https://acme-v02.api.letsencrypt.org/directory", false, false, ""},
		{"lets_encrypt", "letsencrypt", "https://acme-v02.api.letsencrypt.org/directory", false, false, ""},
		{"letsencrypt_staging", "letsencrypt_staging", "https://acme-staging-v02.api.letsencrypt.org/directory", false, false, ""},
		{"le_staging", "letsencrypt_staging", "https://acme-staging-v02.api.letsencrypt.org/directory", false, false, ""},
		{"ZeroSSL", "zerossl", "https://acme.zerossl.com/v2/DV90", true, true, "https://api.zerossl.com/acme/eab-credentials-email"},
		{"google", "google", "https://dv.acme-v02.api.pki.goog/directory", true, false, ""},
		{"GTS", "google", "https://dv.acme-v02.api.pki.goog/directory", true, false, ""},
		{"google_trust_services", "google", "https://dv.acme-v02.api.pki.goog/directory", true, false, ""},
		{"ssl.com", "ssl.com", "https://acme.ssl.com/sslcom-dv-ecc", true, false, ""},
		{"sslcom", "ssl.com", "https://acme.ssl.com/sslcom-dv-ecc", true, false, ""},
		{"ssl.com-rsa", "ssl.com-rsa", "https://acme.ssl.com/sslcom-dv-rsa", true, false, ""},
		{"litessl", "litessl", "https://acme.litessl.com/acme/v2/directory", false, false, ""},
		{"TrustAsia", "litessl", "https://acme.litessl.com/acme/v2/directory", false, false, ""},
		{"buypass", "buypass", "https://api.buypass.com/acme/directory", false, false, ""},
		{"https://acme.enterprise.example/v2/directory", "custom", "https://acme.enterprise.example/v2/directory", false, false, ""},
		{"http://step-ca.example.org/acme/acme/directory", "custom", "http://step-ca.example.org/acme/acme/directory", false, false, ""},
	}

	for _, tc := range cases {
		info, err := ResolveSSLProvider(tc.input)
		if err != nil {
			t.Fatalf("ResolveSSLProvider(%q) unexpected error: %v", tc.input, err)
		}
		if info.Name != tc.wantName || info.DirectoryURL != tc.wantURL ||
			info.RequiresEAB != tc.wantRequiresEAB || info.SupportsAutoEAB != tc.wantAutoEAB ||
			info.AutoEABURL != tc.wantAutoEABURL {
			t.Fatalf("ResolveSSLProvider(%q) = %+v, want name=%s url=%s eab=%v autoEAB=%v autoURL=%s",
				tc.input, info, tc.wantName, tc.wantURL, tc.wantRequiresEAB, tc.wantAutoEAB, tc.wantAutoEABURL)
		}
	}

	for _, bad := range []string{"unknown_ca", "https://", "http://bad url/dir"} {
		if _, err := ResolveSSLProvider(bad); err == nil {
			t.Fatalf("expected ResolveSSLProvider(%q) to fail", bad)
		}
	}
}

func newMockACMETransport(
	t *testing.T,
	baseHost string,
	board *HTTP01Board,
	issuerCN string,
	certValidDays int,
	serialCounter *atomic.Int64,
	onNewAccount func(outerJWS map[string]string, outerProtected map[string]any, acctPayload map[string]any),
	extraHandler func(req *http.Request) (*http.Response, bool),
) *http.Client {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	var lastCSR []byte
	baseURL := "https://" + baseHost

	return &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if extraHandler != nil {
				if resp, handled := extraHandler(req); handled {
					return resp, nil
				}
			}
			if req.URL.Host != baseHost {
				t.Fatalf("unexpected request host %s (expected %s)", req.URL.Host, baseHost)
			}
			path := req.URL.Path
			nonceHdr := map[string]string{"Replay-Nonce": "nonce-" + strings.Trim(path, "/")}
			switch path {
			case "/directory":
				return jsonResp(200, map[string]string{
					"newNonce":   baseURL + "/new-nonce",
					"newAccount": baseURL + "/new-acct",
					"newOrder":   baseURL + "/new-order",
				}, nil), nil
			case "/new-nonce":
				return jsonResp(204, nil, nonceHdr), nil
			case "/new-acct":
				rawBody, _ := io.ReadAll(req.Body)
				var outerJWS map[string]string
				if err := json.Unmarshal(rawBody, &outerJWS); err != nil {
					t.Fatalf("invalid newAccount JWS: %v", err)
				}
				protBytes, _ := b64urlDecode(outerJWS["protected"])
				var outerProt map[string]any
				_ = json.Unmarshal(protBytes, &outerProt)
				payBytes, _ := b64urlDecode(outerJWS["payload"])
				var acctPayload map[string]any
				_ = json.Unmarshal(payBytes, &acctPayload)
				if onNewAccount != nil {
					onNewAccount(outerJWS, outerProt, acctPayload)
				}
				nonceHdr["Location"] = baseURL + "/acct/1"
				return jsonResp(201, map[string]any{"status": "valid"}, nonceHdr), nil
			case "/new-order":
				nonceHdr["Location"] = baseURL + "/order/1"
				return jsonResp(201, map[string]any{
					"status":         "pending",
					"authorizations": []string{baseURL + "/authz/1"},
					"finalize":       baseURL + "/finalize/1",
				}, nonceHdr), nil
			case "/authz/1":
				if board != nil {
					if _, ok := board.Lookup("tok-eab"); ok {
						return jsonResp(200, map[string]any{"status": "valid"}, nonceHdr), nil
					}
				}
				return jsonResp(200, map[string]any{
					"status": "pending",
					"challenges": []map[string]string{
						{"type": "http-01", "token": "tok-eab", "url": baseURL + "/chall/1"},
					},
				}, nonceHdr), nil
			case "/chall/1":
				return jsonResp(200, map[string]any{"status": "valid"}, nonceHdr), nil
			case "/finalize/1":
				rawBody, _ := io.ReadAll(req.Body)
				var jws map[string]string
				_ = json.Unmarshal(rawBody, &jws)
				payBytes, _ := b64urlDecode(jws["payload"])
				var csrObj map[string]string
				_ = json.Unmarshal(payBytes, &csrObj)
				lastCSR, _ = b64urlDecode(csrObj["csr"])
				return jsonResp(200, map[string]any{
					"status":      "valid",
					"certificate": baseURL + "/cert/1",
				}, nonceHdr), nil
			case "/cert/1":
				parsedCSR, err := x509.ParseCertificateRequest(lastCSR)
				if err != nil {
					t.Fatalf("invalid CSR: %v", err)
				}
				if err := parsedCSR.CheckSignature(); err != nil {
					t.Fatalf("CSR signature invalid: %v", err)
				}
				seq := int64(1)
				if serialCounter != nil {
					seq = serialCounter.Add(1)
				}
				tmpl := &x509.Certificate{
					SerialNumber: big.NewInt(1000 + seq),
					Subject:      parsedCSR.Subject,
					Issuer:       pkix.Name{CommonName: issuerCN},
					NotBefore:    time.Now().Add(-5 * time.Minute),
					NotAfter:     time.Now().Add(time.Duration(certValidDays) * 24 * time.Hour),
					DNSNames:     parsedCSR.DNSNames,
					KeyUsage:     x509.KeyUsageDigitalSignature,
					ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
				}
				parent := &x509.Certificate{
					Subject: pkix.Name{CommonName: issuerCN},
				}
				der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, parsedCSR.PublicKey, caKey)
				if err != nil {
					t.Fatal(err)
				}
				// Clear tok-eab so subsequent renewals exercise HTTP-01 cleanly too
				if board != nil {
					board.Clear()
				}
				certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
				return &http.Response{
					StatusCode: 200,
					Header:     http.Header{"Replay-Nonce": []string{"nonce-cert"}},
					Body:       io.NopCloser(bytes.NewReader(certPEM)),
				}, nil
			default:
				t.Fatalf("unexpected ACME path: %s", path)
				return nil, nil
			}
		}),
	}
}

func verifyMockEAB(t *testing.T, eabRaw any, expectedKID string, expectedMACKey []byte, expectedURL string, outerJWK any) {
	t.Helper()
	eabMap, ok := eabRaw.(map[string]any)
	if !ok {
		t.Fatalf("missing or invalid externalAccountBinding: %#v", eabRaw)
	}
	protStr, _ := eabMap["protected"].(string)
	payStr, _ := eabMap["payload"].(string)
	sigStr, _ := eabMap["signature"].(string)

	protBytes, err := b64urlDecode(protStr)
	if err != nil {
		t.Fatalf("decode EAB protected: %v", err)
	}
	var hdr map[string]any
	if err := json.Unmarshal(protBytes, &hdr); err != nil {
		t.Fatal(err)
	}
	if hdr["alg"] != "HS256" || hdr["kid"] != expectedKID || hdr["url"] != expectedURL {
		t.Fatalf("unexpected EAB protected header: %+v", hdr)
	}
	if _, hasNonce := hdr["nonce"]; hasNonce {
		t.Fatalf("RFC 8555 §7.3.4 forbids nonce in EAB protected header: %+v", hdr)
	}

	mac := hmac.New(sha256.New, expectedMACKey)
	mac.Write([]byte(protStr + "." + payStr))
	gotSig, err := b64urlDecode(sigStr)
	if err != nil || !hmac.Equal(gotSig, mac.Sum(nil)) {
		t.Fatal("EAB HS256 HMAC verification failed")
	}

	eabJWKBytes, _ := b64urlDecode(payStr)
	outerJWKBytes, _ := json.Marshal(outerJWK)
	if !bytes.Equal(eabJWKBytes, outerJWKBytes) {
		t.Fatalf("EAB inner JWK %s != outer JWS JWK %s", eabJWKBytes, outerJWKBytes)
	}
}

func TestACMEExternalAccountBindingAndZeroSSLAutoEAB(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	// 1. Direct BuildEABJWS test with standard padded base64 containing '+' and '/'
	rawHMAC := []byte{0xfb, 0xff, 0xfe, 0x01, 0x02, 0x03, 0xef, 0xfe, 0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80, 0x99}
	stdB64HMAC := base64.StdEncoding.EncodeToString(rawHMAC) // contains '+', '/', and '=' padding
	if !strings.ContainsAny(stdB64HMAC, "+/") || !strings.HasSuffix(stdB64HMAC, "=") {
		t.Fatalf("expected std base64 test vector to have +/ and =, got %q", stdB64HMAC)
	}

	board := NewHTTP01Board()
	var verifiedExplicitEAB bool
	gtsHTTP := newMockACMETransport(t, "gts.acme.test", board, "Google Trust Services Mock", 90, nil,
		func(_ map[string]string, outerProt map[string]any, acctPayload map[string]any) {
			verifyMockEAB(t, acctPayload["externalAccountBinding"], "gts-kid-123", rawHMAC, "https://gts.acme.test/new-acct", outerProt["jwk"])
			verifiedExplicitEAB = true
		},
		nil,
	)

	gtsClient := NewACMEClientWithOptions(ACMEOptions{
		ProviderName:   "google",
		DirectoryURL:   "https://gts.acme.test/directory",
		AccountKeyPath: filepath.Join(dir, "gts-acct.pem"),
		EABKID:         "gts-kid-123",
		EABHMACKey:     stdB64HMAC,
		HTTPClient:     gtsHTTP,
	})
	certPEM, _, err := gtsClient.Issue(ctx, []string{"secure.example.com"}, "secops@example.com", board)
	if err != nil || !verifiedExplicitEAB || len(certPEM) == 0 {
		t.Fatalf("explicit EAB issuance failed: verified=%v err=%v", verifiedExplicitEAB, err)
	}

	// 2. ZeroSSL Auto-EAB flow (POST email=<email> -> eab_kid + base64url eab_hmac_key)
	autoMAC := []byte("zerossl-auto-provisioned-hmac-secret-32b")
	autoB64URL := base64.RawURLEncoding.EncodeToString(autoMAC)
	var sawAutoEABPost, verifiedAutoEAB bool

	zsslHTTP := newMockACMETransport(t, "zerossl.acme.test", board, "ZeroSSL Mock CA", 90, nil,
		func(_ map[string]string, outerProt map[string]any, acctPayload map[string]any) {
			verifyMockEAB(t, acctPayload["externalAccountBinding"], "zssl-auto-kid-99", autoMAC, "https://zerossl.acme.test/new-acct", outerProt["jwk"])
			verifiedAutoEAB = true
		},
		func(req *http.Request) (*http.Response, bool) {
			if req.URL.String() == "https://api.zerossl.test/acme/eab-credentials-email" {
				if req.Method != http.MethodPost {
					t.Fatalf("expected POST to AutoEABURL, got %s", req.Method)
				}
				body, _ := io.ReadAll(req.Body)
				vals, _ := url.ParseQuery(string(body))
				if vals.Get("email") != "auto@example.com" {
					t.Fatalf("expected form email=auto@example.com, got %q", string(body))
				}
				sawAutoEABPost = true
				return jsonResp(200, map[string]any{
					"success":      true,
					"eab_kid":      "zssl-auto-kid-99",
					"eab_hmac_key": autoB64URL,
				}, nil), true
			}
			return nil, false
		},
	)

	zsslClient := NewACMEClientWithOptions(ACMEOptions{
		ProviderName:   "zerossl",
		DirectoryURL:   "https://zerossl.acme.test/directory",
		AccountKeyPath: filepath.Join(dir, "zssl-acct.pem"),
		AutoEABURL:     "https://api.zerossl.test/acme/eab-credentials-email",
		HTTPClient:     zsslHTTP,
	})
	if _, _, err := zsslClient.Issue(ctx, []string{"app.example.com"}, "auto@example.com", board); err != nil {
		t.Fatalf("ZeroSSL auto-EAB issuance failed: %v", err)
	}
	if !sawAutoEABPost || !verifiedAutoEAB {
		t.Fatalf("ZeroSSL auto-EAB flow incomplete: sawPost=%v verifiedEAB=%v", sawAutoEABPost, verifiedAutoEAB)
	}
}

func TestUserSuppliedAccountAndCSRPrivateKeys(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	board := NewHTTP01Board()

	// 1. User-supplied RSA-2048 account key (RS256) + user-supplied RSA-2048 CSR private key
	rsaAcctKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsaAcctPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(rsaAcctKey),
	})
	rsaAcctPath := filepath.Join(dir, "user-rsa-account.pem")
	if err := os.WriteFile(rsaAcctPath, rsaAcctPEM, 0o600); err != nil {
		t.Fatal(err)
	}

	rsaCSRKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsaCSRPEM, err := MarshalPrivateKeyPEM(rsaCSRKey)
	if err != nil {
		t.Fatal(err)
	}
	rsaCSRPath := filepath.Join(dir, "user-csr-key.pem")
	if err := os.WriteFile(rsaCSRPath, rsaCSRPEM, 0o600); err != nil {
		t.Fatal(err)
	}

	var sawRS256 bool
	rsaHTTP := newMockACMETransport(t, "rsa.acme.test", board, "RSA Test CA", 90, nil,
		func(outerJWS map[string]string, outerProt map[string]any, _ map[string]any) {
			if outerProt["alg"] != "RS256" {
				t.Fatalf("expected outer JWS alg=RS256 for RSA account key, got %v", outerProt["alg"])
			}
			digest := sha256.Sum256([]byte(outerJWS["protected"] + "." + outerJWS["payload"]))
			sig, _ := b64urlDecode(outerJWS["signature"])
			if err := rsa.VerifyPKCS1v15(&rsaAcctKey.PublicKey, crypto.SHA256, digest[:], sig); err != nil {
				t.Fatalf("RS256 signature verification failed: %v", err)
			}
			sawRS256 = true
		},
		nil,
	)

	rsaClient := NewACMEClientWithOptions(ACMEOptions{
		ProviderName:   "ssl.com-rsa",
		DirectoryURL:   "https://rsa.acme.test/directory",
		AccountKeyPath: rsaAcctPath,
		CertKeyPath:    rsaCSRPath,
		HTTPClient:     rsaHTTP,
	})
	certPEM, issuedKeyPEM, err := rsaClient.Issue(ctx, []string{"rsa.example.com"}, "rsa@example.com", board)
	if err != nil || !sawRS256 {
		t.Fatalf("RSA account/CSR issuance failed: sawRS256=%v err=%v", sawRS256, err)
	}
	leaf, err := LoadLeafCertificate(certPEM)
	if err != nil {
		t.Fatal(err)
	}
	leafRSAPub, ok := leaf.PublicKey.(*rsa.PublicKey)
	if !ok || leafRSAPub.N.Cmp(rsaCSRKey.N) != 0 {
		t.Fatal("issued certificate public key did not match user-supplied RSA CSR private key")
	}
	if _, err := tls.X509KeyPair(certPEM, issuedKeyPEM); err != nil {
		t.Fatalf("issued cert and returned RSA keyPEM mismatch: %v", err)
	}

	// 2. User-supplied ECDSA P-384 account key (ES384) + auto-generated CertKeyPath when missing on disk
	p384Key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p384DER, _ := x509.MarshalECPrivateKey(p384Key)
	p384AcctPath := filepath.Join(dir, "p384-acct.pem")
	_ = os.WriteFile(p384AcctPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: p384DER}), 0o600)

	autoCertKeyPath := filepath.Join(dir, "generated-csr-key.pem")
	var sawES384 bool
	p384HTTP := newMockACMETransport(t, "p384.acme.test", board, "P384 Test CA", 90, nil,
		func(_ map[string]string, outerProt map[string]any, _ map[string]any) {
			if outerProt["alg"] != "ES384" {
				t.Fatalf("expected alg=ES384 for P-384 account key, got %v", outerProt["alg"])
			}
			sawES384 = true
		},
		nil,
	)
	p384Client := NewACMEClientWithOptions(ACMEOptions{
		DirectoryURL:   "https://p384.acme.test/directory",
		AccountKeyPath: p384AcctPath,
		CertKeyPath:    autoCertKeyPath,
		HTTPClient:     p384HTTP,
	})
	cert1, _, err := p384Client.Issue(ctx, []string{"p384.example.com"}, "ec@example.com", board)
	if err != nil || !sawES384 {
		t.Fatalf("P-384 issuance failed: sawES384=%v err=%v", sawES384, err)
	}
	if _, err := os.Stat(autoCertKeyPath); err != nil {
		t.Fatalf("expected CertKeyPath to be persisted on disk: %v", err)
	}
	// Second issue must reuse the persisted CertKeyPath key
	cert2, _, err := p384Client.Issue(ctx, []string{"p384.example.com"}, "ec@example.com", board)
	if err != nil {
		t.Fatal(err)
	}
	leaf1, _ := LoadLeafCertificate(cert1)
	leaf2, _ := LoadLeafCertificate(cert2)
	if !leaf1.PublicKey.(*ecdsa.PublicKey).Equal(leaf2.PublicKey) {
		t.Fatal("expected persisted CertKeyPath key to be reused across renewals")
	}
}

func encryptLegacyRFC1423PEM(t *testing.T, pemType string, der []byte, password string) []byte {
	t.Helper()
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		t.Fatal(err)
	}
	key := evpBytesToKeyMD5([]byte(password), iv[:8], 32)
	ciph, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	padLen := aes.BlockSize - (len(der) % aes.BlockSize)
	padded := make([]byte, len(der)+padLen)
	copy(padded, der)
	for i := len(der); i < len(padded); i++ {
		padded[i] = byte(padLen)
	}
	enc := make([]byte, len(padded))
	cipher.NewCBCEncrypter(ciph, iv).CryptBlocks(enc, padded)
	return pem.EncodeToMemory(&pem.Block{
		Type: pemType,
		Headers: map[string]string{
			"Proc-Type": "4,ENCRYPTED",
			"DEK-Info":  "AES-256-CBC," + strings.ToUpper(hex.EncodeToString(iv)),
		},
		Bytes: enc,
	})
}

func TestEncryptedPrivateKeySupport(t *testing.T) {
	dir := t.TempDir()
	certPEM, rawKeyPEM, err := BuildSelfSigned([]string{"vault.example.com"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ParsePrivateKeyPEM(rawKeyPEM, "")
	if err != nil {
		t.Fatal(err)
	}

	// 1. PKCS#8 PBES2 encrypted private key
	encPKCS8PEM, err := EncryptPKCS8PrivateKeyPEM(signer, "s3cr3t-pass!")
	if err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(dir, "vault.pem")
	pkcs8KeyPath := filepath.Join(dir, "vault-pkcs8.key")
	_ = os.WriteFile(certPath, certPEM, 0o644)
	_ = os.WriteFile(pkcs8KeyPath, encPKCS8PEM, 0o600)

	if _, _, err := LoadKeyPair(certPath, pkcs8KeyPath); err == nil {
		t.Fatal("expected LoadKeyPair without password to fail on encrypted PKCS#8 key")
	}
	if _, _, err := LoadKeyPairWithPassword(certPath, pkcs8KeyPath, "wrong-pass"); err == nil {
		t.Fatal("expected LoadKeyPairWithPassword with wrong password to fail")
	}
	pair, leaf, err := LoadKeyPairWithPassword(certPath, pkcs8KeyPath, "s3cr3t-pass!")
	if err != nil || leaf == nil || len(pair.Certificate) == 0 {
		t.Fatalf("LoadKeyPairWithPassword (PKCS#8) failed: %v", err)
	}

	// 2. Legacy RFC 1423 PEM encrypted EC private key + GYMBAG_KEY_PASSWORD env var via FileProvider
	ecDER, err := x509.MarshalECPrivateKey(signer.(*ecdsa.PrivateKey))
	if err != nil {
		t.Fatal(err)
	}
	legacyPEM := encryptLegacyRFC1423PEM(t, "EC PRIVATE KEY", ecDER, "env-secret-pw")
	legacyKeyPath := filepath.Join(dir, "vault-rfc1423.key")
	_ = os.WriteFile(legacyKeyPath, legacyPEM, 0o600)

	t.Setenv("GYMBAG_KEY_PASSWORD", "env-secret-pw")
	fp := &FileProvider{
		CertFile:        certPath,
		KeyFile:         legacyKeyPath,
		Domains:         []string{"vault.example.com"},
		RenewWithinDays: 30,
		Required:        true,
	}
	mat, err := fp.Issue(context.Background())
	if err != nil || mat == nil || mat.Source != "file" {
		t.Fatalf("FileProvider with GYMBAG_KEY_PASSWORD failed: %+v, %v", mat, err)
	}
}

type mockLocalRenewalProvider struct {
	dir     string
	domains []string
	calls   atomic.Int32
}

func (m *mockLocalRenewalProvider) Issue(ctx context.Context) (*Material, error) {
	within, force := RenewalWindowFromContext(ctx, 30)
	return m.Renew(ctx, within, force)
}

func (m *mockLocalRenewalProvider) Renew(_ context.Context, withinDays int, force bool) (*Material, error) {
	certPath := filepath.Join(m.dir, "local-fullchain.pem")
	keyPath := filepath.Join(m.dir, "local-privkey.pem")
	if !force {
		if cached := (&FileProvider{
			CertFile:        certPath,
			KeyFile:         keyPath,
			Domains:         m.domains,
			RenewWithinDays: withinDays,
		}).load(); cached != nil {
			cached.Source = "local"
			cached.Provider = "local"
			return cached, nil
		}
	}
	m.calls.Add(1)
	certPEM, keyPEM, err := BuildSelfSigned(m.domains, 60)
	if err != nil {
		return nil, err
	}
	_ = writePublicFile(certPath, certPEM)
	_ = writePrivateFile(keyPath, keyPEM)
	pair, _, err := LoadKeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	return &Material{
		CertPath: certPath,
		KeyPath:  keyPath,
		Source:   "local",
		Provider: "local",
		Cert:     pair,
	}, nil
}

func TestProviderScopedCachingAndAutomaticRenewal(t *testing.T) {
	ctx := context.Background()
	certDir := t.TempDir()
	board := NewHTTP01Board()

	var leSerial, zsslSerial, liteSerial atomic.Int64
	leHTTP := newMockACMETransport(t, "le.acme.test", board, "Let's Encrypt Mock", 90, &leSerial, nil, nil)
	zsslHTTP := newMockACMETransport(t, "zssl.acme.test", board, "ZeroSSL Mock", 90, &zsslSerial, nil, nil)
	liteHTTP := newMockACMETransport(t, "lite.acme.test", board, "LiteSSL TrustAsia Mock", 20, &liteSerial, nil, nil)

	// 1. Let's Encrypt uses legacy-compatible acme-fullchain.pem & caches on repeat Obtain
	leProv := &ACMEProvider{
		Client: NewACMEClientWithOptions(ACMEOptions{
			ProviderName:   "letsencrypt",
			DirectoryURL:   "https://le.acme.test/directory",
			AccountKeyPath: filepath.Join(certDir, "le-acct.pem"),
			HTTPClient:     leHTTP,
		}),
		Board:     board,
		Directory: certDir,
		Domains:   []string{"shop.example.com"},
		Email:     "ops@example.com",
		Required:  true,
	}
	leMat1, err := leProv.Issue(ctx)
	if err != nil || filepath.Base(leMat1.CertPath) != "acme-fullchain.pem" {
		t.Fatalf("unexpected LE material: %+v, %v", leMat1, err)
	}
	leMat2, err := leProv.Issue(ctx)
	if err != nil || leSerial.Load() != 1 || leMat2.Cert.Leaf.Issuer.CommonName != "Let's Encrypt Mock" {
		t.Fatalf("expected LE cache hit (serial=1), got serial=%d err=%v", leSerial.Load(), err)
	}

	// 2. Switching to ZeroSSL in the SAME certDir immediately issues a new cert at acme-zerossl-fullchain.pem!
	zsslProv := &ACMEProvider{
		Client: NewACMEClientWithOptions(ACMEOptions{
			ProviderName:   "zerossl",
			DirectoryURL:   "https://zssl.acme.test/directory",
			AccountKeyPath: filepath.Join(certDir, "zssl-acct.pem"),
			HTTPClient:     zsslHTTP,
		}),
		Board:        board,
		Directory:    certDir,
		Domains:      []string{"shop.example.com"},
		Email:        "ops@example.com",
		ProviderName: "zerossl",
		Required:     true,
	}
	zsslMat, err := zsslProv.Issue(ctx)
	if err != nil || filepath.Base(zsslMat.CertPath) != "acme-zerossl-fullchain.pem" {
		t.Fatalf("expected acme-zerossl-fullchain.pem, got %+v, %v", zsslMat, err)
	}
	if zsslMat.Cert.Leaf.Issuer.CommonName != "ZeroSSL Mock" || zsslSerial.Load() != 1 {
		t.Fatalf("expected fresh ZeroSSL certificate, got issuer=%q serial=%d", zsslMat.Cert.Leaf.Issuer.CommonName, zsslSerial.Load())
	}

	// 3. Manager.RenewIfNeeded & ForceRenew on ZeroSSL + LiteSSL
	mgr := NewManager([]Provider{zsslProv})
	if _, err := mgr.Obtain(ctx); err != nil {
		t.Fatal(err)
	}
	if zsslSerial.Load() != 1 {
		t.Fatalf("expected cached ZeroSSL cert on Manager.Obtain, got serial=%d", zsslSerial.Load())
	}
	// Not expiring within 30 days -> nil, nil
	if renewed, err := mgr.RenewIfNeeded(ctx, 30); err != nil || renewed != nil {
		t.Fatalf("expected no renewal for 90d cert within 30d, got %+v, %v", renewed, err)
	}
	// Expiring within 95 days -> triggers ACME renewal & hot-reloads GetCertificate!
	renewed, err := mgr.RenewIfNeeded(ctx, 95)
	if err != nil || renewed == nil || zsslSerial.Load() != 2 {
		t.Fatalf("expected RenewIfNeeded(95) to renew ZeroSSL cert (serial=2), got %+v, serial=%d, err=%v", renewed, zsslSerial.Load(), err)
	}
	activeTLS, err := mgr.GetCertificate(&tls.ClientHelloInfo{ServerName: "shop.example.com"})
	if err != nil || activeTLS.Leaf.SerialNumber.Int64() != 1002 {
		t.Fatalf("expected hot-reloaded cert serial 1002, got %v (err=%v)", activeTLS.Leaf.SerialNumber, err)
	}

	// ForceRenew unconditionally re-issues even when withinDays wouldn't trigger
	forced, err := mgr.ForceRenew(ctx)
	if err != nil || forced == nil || zsslSerial.Load() != 3 {
		t.Fatalf("expected ForceRenew to issue serial=3, got %+v, serial=%d, err=%v", forced, zsslSerial.Load(), err)
	}

	// Switch to LiteSSL / TrustAsia with a 20-day cert -> RenewIfNeeded(30) renews immediately!
	liteProv := &ACMEProvider{
		Client: NewACMEClientWithOptions(ACMEOptions{
			ProviderName:   "trustasia",
			DirectoryURL:   "https://lite.acme.test/directory",
			AccountKeyPath: filepath.Join(certDir, "lite-acct.pem"),
			HTTPClient:     liteHTTP,
		}),
		Board:        board,
		Directory:    certDir,
		Domains:      []string{"shop.example.com"},
		Email:        "ops@example.com",
		ProviderName: "trustasia",
		Required:     true,
	}
	liteMgr := NewManager([]Provider{liteProv})
	liteMat1, err := liteMgr.Obtain(ctx)
	if err != nil || filepath.Base(liteMat1.CertPath) != "acme-litessl-fullchain.pem" {
		t.Fatalf("expected acme-litessl-fullchain.pem, got %+v, %v", liteMat1, err)
	}
	liteMat2, err := liteMgr.RenewIfNeeded(ctx, 30)
	if err != nil || liteMat2 == nil || liteSerial.Load() != 2 {
		t.Fatalf("expected 20-day LiteSSL cert to renew under withinDays=30, got serial=%d err=%v", liteSerial.Load(), err)
	}

	// 4. Auto mode with .internal domain skips ACME and uses Local CA RenewalProvider seamlessly
	localDir := t.TempDir()
	localProv := &mockLocalRenewalProvider{dir: localDir, domains: []string{"app.internal"}}
	cfg := config.Default()
	cfg.Sites = []config.SiteConfig{{Domain: "app.internal", Upstreams: []string{"http://localhost:3000"}}}
	cfg.TLS.Mode = "auto"
	cfg.TLS.CertDir = localDir
	cfg.TLS.CertFile = filepath.Join(localDir, "missing-cert.pem")
	cfg.TLS.KeyFile = filepath.Join(localDir, "missing-key.pem")

	localMgr := NewManager(BuildProvidersWithLocal(cfg, board, zsslProv.Client, localProv))
	localMat, err := localMgr.Obtain(ctx)
	if err != nil || localMat.Source != "local" || localProv.calls.Load() != 1 {
		t.Fatalf("expected auto mode on app.internal to use local provider, got %+v, calls=%d, err=%v", localMat, localProv.calls.Load(), err)
	}
	if r, err := localMgr.RenewIfNeeded(ctx, 30); err != nil || r != nil {
		t.Fatalf("expected 60d local cert not to renew at 30d, got %+v, %v", r, err)
	}
	if r, err := localMgr.RenewIfNeeded(ctx, 70); err != nil || r == nil || localProv.calls.Load() != 2 {
		t.Fatalf("expected 60d local cert to renew at withinDays=70, got calls=%d err=%v", localProv.calls.Load(), err)
	}

	// 5. FileProvider hot-reloads when external tool updates cert.pem on disk
	fileDir := t.TempDir()
	fCertPath := filepath.Join(fileDir, "ext-cert.pem")
	fKeyPath := filepath.Join(fileDir, "ext-key.pem")
	c1, k1, _ := BuildSelfSigned([]string{"ext.example.com"}, 90)
	_ = os.WriteFile(fCertPath, c1, 0o644)
	_ = os.WriteFile(fKeyPath, k1, 0o600)

	fileMgr := NewManager([]Provider{&FileProvider{
		CertFile:        fCertPath,
		KeyFile:         fKeyPath,
		Domains:         []string{"ext.example.com"},
		RenewWithinDays: 30,
		Required:        true,
	}})
	if _, err := fileMgr.Obtain(ctx); err != nil {
		t.Fatal(err)
	}
	c2, k2, _ := BuildSelfSigned([]string{"ext.example.com"}, 90)
	_ = os.WriteFile(fCertPath, c2, 0o644)
	_ = os.WriteFile(fKeyPath, k2, 0o600)
	reloaded, err := fileMgr.RenewIfNeeded(ctx, 30)
	if err != nil || reloaded == nil {
		t.Fatalf("expected FileProvider disk replacement to hot-reload on RenewIfNeeded, got %+v, %v", reloaded, err)
	}

	// 6. Default BuildProviders wires built-in LocalCAProvider automatically for local domains & BuildACMEClientFromConfig
	builtInLocalDir := t.TempDir()
	builtInCfg := config.Default()
	builtInCfg.Sites = []config.SiteConfig{{Domain: "dashboard.internal", Upstreams: []string{"http://localhost:8080"}}}
	builtInCfg.TLS.CertDir = builtInLocalDir
	builtInCfg.TLS.CertFile = filepath.Join(builtInLocalDir, "none.pem")
	builtInCfg.TLS.KeyFile = filepath.Join(builtInLocalDir, "none.key")

	builtInMgr := NewManager(BuildProviders(builtInCfg, board, zsslProv.Client))
	builtInMat, err := builtInMgr.Obtain(ctx)
	if err != nil || builtInMat.Source != "local" || builtInMat.Cert.Leaf.Issuer.CommonName != "Gymbag & Swarm SSL Local CA" {
		t.Fatalf("expected built-in LocalCAProvider cert signed by Gymbag & Swarm SSL Local CA, got %+v, err=%v", builtInMat, err)
	}
	if forcedLocal, err := builtInMgr.ForceRenew(ctx); err != nil || forcedLocal == nil ||
		forcedLocal.Cert.Leaf.SerialNumber.Cmp(builtInMat.Cert.Leaf.SerialNumber) == 0 {
		t.Fatalf("expected ForceRenew on LocalCAProvider to mint a new leaf serial, err=%v", err)
	}

	customCfg := config.Default()
	customCfg.TLS.Mode = "acme"
	customCfg.TLS.Provider = "custom"
	customCfg.TLS.ACMEDirectory = "https://ca.internal.example/acme/directory"
	customClient, err := BuildACMEClientFromConfig(customCfg, zsslHTTP)
	if err != nil || customClient.ProviderName() != "custom" || customClient.DirectoryURL() != "https://ca.internal.example/acme/directory" {
		t.Fatalf("BuildACMEClientFromConfig custom failed: %+v, %v", customClient, err)
	}
}
