package tlsmanager

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gymbag/internal/config"
)

func writeSelfSignedPairWithWindow(t *testing.T, certPath, keyPath string, domains []string, notBefore, notAfter time.Time) *big.Int {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 64)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: domains[0]},
		Issuer:                pkix.Name{CommonName: domains[0]},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              domains,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM, err := MarshalPrivateKeyPEM(priv)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(certPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return serial
}

func TestAcmeShDirectoryDiscoveryPreferences(t *testing.T) {
	home := t.TempDir()
	now := time.Now().UTC()

	// Create both RSA (<domain>/) and ECC (<domain>_ecc/) dirs; ECC must win.
	rsaCert := filepath.Join(home, "app.example.com", "fullchain.cer")
	rsaKey := filepath.Join(home, "app.example.com", "app.example.com.key")
	eccCert := filepath.Join(home, "app.example.com_ecc", "fullchain.cer")
	eccKey := filepath.Join(home, "app.example.com_ecc", "app.example.com.key")

	writeSelfSignedPairWithWindow(t, rsaCert, rsaKey, []string{"app.example.com"}, now.Add(-time.Hour), now.Add(60*24*time.Hour))
	eccSerial := writeSelfSignedPairWithWindow(t, eccCert, eccKey, []string{"app.example.com"}, now.Add(-time.Hour), now.Add(60*24*time.Hour))

	cp, _, _, leaf, ok := ResolveAcmeShPair(home, "", "app.example.com", "")
	if !ok || cp != eccCert || leaf.SerialNumber.Cmp(eccSerial) != 0 {
		t.Fatalf("expected ECC dir to win (%s), got ok=%v path=%s", eccCert, ok, cp)
	}

	// Remove ECC dir -> should fall back to RSA dir
	if err := os.RemoveAll(filepath.Join(home, "app.example.com_ecc")); err != nil {
		t.Fatal(err)
	}
	cp, _, _, _, ok = ResolveAcmeShPair(home, "", "app.example.com", "")
	if !ok || cp != rsaCert {
		t.Fatalf("expected RSA fallback %s, got ok=%v path=%s", rsaCert, ok, cp)
	}

	// Wildcard directory resolution
	wcCert := filepath.Join(home, "*.wild.example.com_ecc", "fullchain.cer")
	wcKey := filepath.Join(home, "*.wild.example.com_ecc", "*.wild.example.com.key")
	writeSelfSignedPairWithWindow(t, wcCert, wcKey, []string{"*.wild.example.com", "wild.example.com"}, now.Add(-time.Hour), now.Add(60*24*time.Hour))
	cp, _, _, _, ok = ResolveAcmeShPair(home, "", "api.wild.example.com", "")
	if !ok || cp != wcCert {
		t.Fatalf("expected wildcard cert %s for subdomain, got ok=%v path=%s", wcCert, ok, cp)
	}

	// Direct single-domain / --install-cert dir
	installDir := filepath.Join(home, "installed-single")
	instCert := filepath.Join(installDir, "fullchain.pem")
	instKey := filepath.Join(installDir, "privkey.pem")
	writeSelfSignedPairWithWindow(t, instCert, instKey, []string{"single.example.com"}, now.Add(-time.Hour), now.Add(60*24*time.Hour))
	cp, _, _, _, ok = ResolveAcmeShPair(installDir, "", "single.example.com", "")
	if !ok || cp != instCert {
		t.Fatalf("expected direct install dir cert %s, got ok=%v path=%s", instCert, ok, cp)
	}
}

func TestExternalCertsValidWithinRenewalWindow(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	now := time.Now().UTC()

	// Valid 90-day cert expiring in 10 days (< 30d default RenewWithinDays).
	shortCert := filepath.Join(dir, "short.example.com_ecc", "fullchain.cer")
	shortKey := filepath.Join(dir, "short.example.com_ecc", "short.example.com.key")
	writeSelfSignedPairWithWindow(t, shortCert, shortKey, []string{"short.example.com"}, now.Add(-80*24*time.Hour), now.Add(10*24*time.Hour))

	acmeProv := &AcmeShProvider{Path: dir, Domains: []string{"short.example.com"}, Required: true}
	mat, err := acmeProv.Issue(ctx)
	if err != nil || mat == nil {
		t.Fatalf("expected valid 10d acme.sh cert to be accepted, got mat=%v err=%v", mat, err)
	}

	reqFile := &FileProvider{CertFile: shortCert, KeyFile: shortKey, Domains: []string{"short.example.com"}, RenewWithinDays: 30, Required: true}
	if fMat, fErr := reqFile.Issue(ctx); fErr != nil || fMat == nil {
		t.Fatalf("expected FileProvider{Required: true} to accept 10d cert, got mat=%v err=%v", fMat, fErr)
	}

	// Cache check mode (Required: false) still requests renewal within 30 days
	cacheFile := &FileProvider{CertFile: shortCert, KeyFile: shortKey, Domains: []string{"short.example.com"}, RenewWithinDays: 30, Required: false}
	if cMat, cErr := cacheFile.Issue(ctx); cErr != nil || cMat != nil {
		t.Fatalf("expected FileProvider{Required: false} to treat 10d cert as needing renewal, got %v %v", cMat, cErr)
	}

	// Genuinely expired cert must be rejected by both
	expCert := filepath.Join(dir, "expired.example.com_ecc", "fullchain.cer")
	expKey := filepath.Join(dir, "expired.example.com_ecc", "expired.example.com.key")
	writeSelfSignedPairWithWindow(t, expCert, expKey, []string{"expired.example.com"}, now.Add(-48*time.Hour), now.Add(-time.Hour))

	if _, err := (&AcmeShProvider{Path: dir, Domains: []string{"expired.example.com"}, Required: true}).Issue(ctx); err == nil {
		t.Fatal("expected AcmeShProvider to reject expired cert")
	}
	if _, err := (&FileProvider{CertFile: expCert, KeyFile: expKey, Domains: []string{"expired.example.com"}, RenewWithinDays: 30, Required: true}).Issue(ctx); err == nil {
		t.Fatal("expected FileProvider to reject expired cert")
	}
}

func TestMultiDomainAcmeShSNIAndHotReload(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	now := time.Now().UTC()

	alphaCert := filepath.Join(home, "alpha.example.com_ecc", "fullchain.cer")
	alphaKey := filepath.Join(home, "alpha.example.com_ecc", "alpha.example.com.key")
	betaCert := filepath.Join(home, "beta.example.com_ecc", "fullchain.cer")
	betaKey := filepath.Join(home, "beta.example.com_ecc", "beta.example.com.key")

	alphaSerial := writeSelfSignedPairWithWindow(t, alphaCert, alphaKey, []string{"alpha.example.com"}, now.Add(-time.Hour), now.Add(60*24*time.Hour))
	betaSerial1 := writeSelfSignedPairWithWindow(t, betaCert, betaKey, []string{"beta.example.com"}, now.Add(-time.Hour), now.Add(60*24*time.Hour))

	f := false
	cfg := config.Default()
	cfg.Sites = []config.SiteConfig{
		{Domain: "alpha.example.com", Upstreams: []string{"http://127.0.0.1:3000"}, Protect: true, WAF: config.DefaultSiteWAFConfig()},
		{Domain: "beta.example.com", Upstreams: []string{"http://127.0.0.1:3001"}, Protect: true, WAF: config.DefaultSiteWAFConfig()},
	}
	cfg.TLS.AutoSSL = &f
	cfg.TLS.AcmeShPath = home

	providers := BuildProvidersWithLocal(cfg, nil, nil, nil)
	if len(providers) != 1 {
		t.Fatalf("expected 1 provider (AcmeShProvider only), got %d", len(providers))
	}
	mgr := NewManager(providers)
	if err := mgr.RegisterSiteCertificatesWithOptions(cfg.Sites, cfg.TLS.AcmeShPath, "", ""); err != nil {
		t.Fatalf("RegisterSiteCertificatesWithOptions failed: %v", err)
	}
	mat, err := mgr.Obtain(ctx)
	if err != nil {
		t.Fatalf("Obtain failed: %v", err)
	}
	if mat.Source != "acme.sh" {
		t.Fatalf("expected source acme.sh, got %q", mat.Source)
	}

	gotAlpha, err := mgr.GetCertificate(&tls.ClientHelloInfo{ServerName: "alpha.example.com"})
	if err != nil || gotAlpha.Leaf.SerialNumber.Cmp(alphaSerial) != 0 {
		t.Fatalf("unexpected alpha SNI cert: err=%v", err)
	}
	gotBeta1, err := mgr.GetCertificate(&tls.ClientHelloInfo{ServerName: "beta.example.com"})
	if err != nil || gotBeta1.Leaf.SerialNumber.Cmp(betaSerial1) != 0 {
		t.Fatalf("unexpected beta SNI cert: err=%v", err)
	}

	// Replace beta cert on disk (simulating acme.sh cron renewal) & hot-reload
	betaSerial2 := writeSelfSignedPairWithWindow(t, betaCert, betaKey, []string{"beta.example.com"}, now.Add(-time.Minute), now.Add(90*24*time.Hour))
	updated, err := mgr.RenewIfNeeded(ctx, 30)
	if err != nil {
		t.Fatalf("RenewIfNeeded failed: %v", err)
	}
	if updated == nil {
		t.Fatal("expected RenewIfNeeded to report update when beta SNI cert changed on disk")
	}
	gotBeta2, err := mgr.GetCertificate(&tls.ClientHelloInfo{ServerName: "beta.example.com"})
	if err != nil || gotBeta2.Leaf.SerialNumber.Cmp(betaSerial2) != 0 {
		t.Fatalf("expected reloaded beta cert serial %v, got %v (err=%v)", betaSerial2, gotBeta2.Leaf.SerialNumber, err)
	}
	if bytes.Equal(gotBeta1.Certificate[0], gotBeta2.Certificate[0]) {
		t.Fatal("expected new certificate bytes after acme.sh disk replacement")
	}
}

func TestBuildProvidersAutoSSLDisabled(t *testing.T) {
	ctx := context.Background()
	cfgOff := config.Default()
	cfgOff.TLS.Mode = "off"
	if got := BuildProvidersWithLocal(cfgOff, nil, nil, nil); len(got) != 0 {
		t.Fatalf("expected nil providers when tls.mode=off, got %d", len(got))
	}

	// AutoSSL=false with per-site certs ONLY -> no global providers, Obtain succeeds via SNI entries
	home := t.TempDir()
	now := time.Now().UTC()
	siteCert := filepath.Join(home, "site.example.com_ecc", "fullchain.cer")
	siteKey := filepath.Join(home, "site.example.com_ecc", "site.example.com.key")
	writeSelfSignedPairWithWindow(t, siteCert, siteKey, []string{"site.example.com"}, now.Add(-time.Hour), now.Add(60*24*time.Hour))

	f := false
	cfgPerSite := config.Default()
	cfgPerSite.TLS.AutoSSL = &f
	cfgPerSite.Sites = []config.SiteConfig{
		{
			Domain:     "site.example.com",
			Upstreams:  []string{"http://127.0.0.1:3000"},
			Protect:    true,
			WAF:        config.DefaultSiteWAFConfig(),
			AcmeShPath: filepath.Join(home, "site.example.com_ecc"),
		},
	}
	if got := BuildProvidersWithLocal(cfgPerSite, nil, nil, nil); len(got) != 0 {
		t.Fatalf("expected 0 global providers when only per-site acme_sh_path is set with auto_ssl=false, got %d", len(got))
	}
	mgr := NewManager(nil)
	if err := mgr.RegisterSiteCertificatesWithOptions(cfgPerSite.Sites, "", "", ""); err != nil {
		t.Fatalf("RegisterSiteCertificatesWithOptions failed: %v", err)
	}
	mat, err := mgr.Obtain(ctx)
	if err != nil || mat == nil || mat.Source != "acme.sh" {
		t.Fatalf("expected Obtain to succeed via per-site SNI cert, got mat=%v err=%v", mat, err)
	}
}
