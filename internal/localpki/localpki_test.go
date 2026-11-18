package localpki

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTwoTierCAAndLeafTrustChain(t *testing.T) {
	certDir := t.TempDir()
	domains := []string{"app.internal", "localhost"}

	issued, err := IssueLeaf(certDir, domains, 0, 0)
	if err != nil {
		t.Fatalf("IssueLeaf failed: %v", err)
	}

	wantRootPath := filepath.Join(certDir, "ca", RootCACertFilename)
	wantInterPath := filepath.Join(certDir, "ca", IntermediateCACertFilename)
	wantCertPath := filepath.Join(certDir, LeafFullchainFilename)
	wantKeyPath := filepath.Join(certDir, LeafPrivkeyFilename)

	if issued.RootCAPath != wantRootPath {
		t.Fatalf("RootCAPath = %q, want %q", issued.RootCAPath, wantRootPath)
	}
	if issued.IntermediateCAPath != wantInterPath {
		t.Fatalf("IntermediateCAPath = %q, want %q", issued.IntermediateCAPath, wantInterPath)
	}
	if issued.CertPath != wantCertPath {
		t.Fatalf("CertPath = %q, want %q", issued.CertPath, wantCertPath)
	}
	if issued.KeyPath != wantKeyPath {
		t.Fatalf("KeyPath = %q, want %q", issued.KeyPath, wantKeyPath)
	}

	if runtime.GOOS != "windows" {
		assertFileMode(t, wantRootPath, 0o644)
		assertFileMode(t, filepath.Join(certDir, "ca", RootCAKeyFilename), 0o600)
		assertFileMode(t, wantInterPath, 0o644)
		assertFileMode(t, filepath.Join(certDir, "ca", IntermediateCAKeyFilename), 0o600)
		assertFileMode(t, wantCertPath, 0o644)
		assertFileMode(t, wantKeyPath, 0o600)
	}

	rootPEM, err := os.ReadFile(issued.RootCAPath)
	if err != nil {
		t.Fatalf("read root CA: %v", err)
	}
	rootCert, err := parseFirstCertificate(rootPEM)
	if err != nil {
		t.Fatalf("parse root CA: %v", err)
	}
	if rootCert.Subject.CommonName != "Neuro-sama Headquarters Root CA" {
		t.Fatalf("Root CA CN = %q, want %q", rootCert.Subject.CommonName, "Neuro-sama Headquarters Root CA")
	}
	if len(rootCert.Subject.Organization) != 1 || rootCert.Subject.Organization[0] != "Neuro-sama Headquarters" {
		t.Fatalf("Root CA Org = %v, want [Neuro-sama Headquarters]", rootCert.Subject.Organization)
	}
	if !rootCert.IsCA || !rootCert.BasicConstraintsValid || rootCert.MaxPathLen != 1 {
		t.Fatalf("Root CA constraints invalid: IsCA=%v BasicConstraintsValid=%v MaxPathLen=%d", rootCert.IsCA, rootCert.BasicConstraintsValid, rootCert.MaxPathLen)
	}
	if len(rootCert.SubjectKeyId) == 0 {
		t.Fatal("Root CA SubjectKeyId must be set")
	}

	interPEM, err := os.ReadFile(issued.IntermediateCAPath)
	if err != nil {
		t.Fatalf("read intermediate CA: %v", err)
	}
	interCert, err := parseFirstCertificate(interPEM)
	if err != nil {
		t.Fatalf("parse intermediate CA: %v", err)
	}
	if interCert.Subject.CommonName != "Gymbag & Swarm SSL Local CA" {
		t.Fatalf("Intermediate CA CN = %q, want %q", interCert.Subject.CommonName, "Gymbag & Swarm SSL Local CA")
	}
	if len(interCert.Subject.Organization) != 1 || interCert.Subject.Organization[0] != "Neuro-sama Headquarters" {
		t.Fatalf("Intermediate CA Org = %v, want [Neuro-sama Headquarters]", interCert.Subject.Organization)
	}
	if !interCert.IsCA || !interCert.BasicConstraintsValid || interCert.MaxPathLen != 0 || !interCert.MaxPathLenZero {
		t.Fatalf("Intermediate CA constraints invalid: IsCA=%v BasicConstraintsValid=%v MaxPathLen=%d MaxPathLenZero=%v",
			interCert.IsCA, interCert.BasicConstraintsValid, interCert.MaxPathLen, interCert.MaxPathLenZero)
	}
	if len(interCert.SubjectKeyId) == 0 || !bytes.Equal(interCert.AuthorityKeyId, rootCert.SubjectKeyId) {
		t.Fatal("Intermediate CA SubjectKeyId / AuthorityKeyId mismatch")
	}

	fullchainPEM, err := os.ReadFile(issued.CertPath)
	if err != nil {
		t.Fatalf("read fullchain: %v", err)
	}
	chainCerts, err := parseAllCertificates(fullchainPEM)
	if err != nil {
		t.Fatalf("parse fullchain: %v", err)
	}
	if len(chainCerts) != 2 {
		t.Fatalf("fullchain cert count = %d, want 2 (leaf + intermediate)", len(chainCerts))
	}
	leafCert := chainCerts[0]
	if !bytes.Equal(chainCerts[1].Raw, interCert.Raw) {
		t.Fatal("second certificate in local-fullchain.pem does not match Intermediate CA")
	}
	if issued.Cert.Leaf == nil || !bytes.Equal(issued.Cert.Leaf.Raw, leafCert.Raw) {
		t.Fatal("issued.Cert.Leaf was not populated with leaf certificate")
	}

	roots := x509.NewCertPool()
	roots.AddCert(rootCert)
	inters := x509.NewCertPool()
	inters.AddCert(interCert)

	for _, target := range []string{"app.internal", "localhost", "127.0.0.1", "::1"} {
		chains, verifyErr := leafCert.Verify(x509.VerifyOptions{
			DNSName:       target,
			Roots:         roots,
			Intermediates: inters,
			KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		})
		if verifyErr != nil {
			t.Fatalf("x509.Certificate.Verify(%q) failed: %v", target, verifyErr)
		}
		if len(chains) == 0 || len(chains[0]) != 3 {
			t.Fatalf("expected 3-cert verified chain for %q, got %v", target, len(chains))
		}
		if chains[0][1].Subject.CommonName != "Gymbag & Swarm SSL Local CA" {
			t.Fatalf("chain intermediate CN = %q", chains[0][1].Subject.CommonName)
		}
		if chains[0][2].Subject.CommonName != "Neuro-sama Headquarters Root CA" {
			t.Fatalf("chain root CN = %q", chains[0][2].Subject.CommonName)
		}
	}
}

func TestLeafCachingRenewalAndCARegeneration(t *testing.T) {
	certDir := t.TempDir()
	domains := []string{"app.internal", "localhost"}

	first, err := IssueLeaf(certDir, domains, 90, 30)
	if err != nil {
		t.Fatalf("first IssueLeaf failed: %v", err)
	}
	firstSerial := new(big.Int).Set(first.Cert.Leaf.SerialNumber)

	// Cached reuse when valid and not expiring within renewWithinDays (30).
	second, err := IssueLeaf(certDir, domains, 90, 30)
	if err != nil {
		t.Fatalf("second IssueLeaf failed: %v", err)
	}
	if firstSerial.Cmp(second.Cert.Leaf.SerialNumber) != 0 {
		t.Fatalf("expected cached leaf reuse, got different serial %v vs %v", second.Cert.Leaf.SerialNumber, firstSerial)
	}

	// Trigger renewal via renewWithinDays > remaining days (95 > 90).
	renewed, err := IssueLeaf(certDir, domains, 90, 95)
	if err != nil {
		t.Fatalf("renewal IssueLeaf failed: %v", err)
	}
	if firstSerial.Cmp(renewed.Cert.Leaf.SerialNumber) == 0 {
		t.Fatal("expected leaf certificate regeneration when renewWithinDays exceeds remaining lifetime")
	}

	// Trigger regeneration when a new domain is added.
	renewedSerial := new(big.Int).Set(renewed.Cert.Leaf.SerialNumber)
	expanded, err := IssueLeaf(certDir, []string{"app.internal", "localhost", "api.internal"}, 90, 30)
	if err != nil {
		t.Fatalf("expanded IssueLeaf failed: %v", err)
	}
	if renewedSerial.Cmp(expanded.Cert.Leaf.SerialNumber) == 0 {
		t.Fatal("expected leaf regeneration when new domain api.internal is added")
	}
	if err := expanded.Cert.Leaf.VerifyHostname("api.internal"); err != nil {
		t.Fatalf("expanded leaf does not cover api.internal: %v", err)
	}

	// Corrupt intermediate CA on disk: Root CA should stay intact while Intermediate CA & leaf regenerate cleanly.
	rootBefore, _ := os.ReadFile(expanded.RootCAPath)
	if err := os.WriteFile(expanded.IntermediateCAPath, []byte("corrupt-pem"), 0o644); err != nil {
		t.Fatalf("corrupt intermediate CA: %v", err)
	}
	recovered, err := IssueLeaf(certDir, domains, 90, 30)
	if err != nil {
		t.Fatalf("IssueLeaf after intermediate corruption failed: %v", err)
	}
	rootAfter, _ := os.ReadFile(recovered.RootCAPath)
	if !bytes.Equal(rootBefore, rootAfter) {
		t.Fatal("expected valid Root CA to be preserved when only Intermediate CA was corrupted")
	}

	// Write an expired Root CA to disk: both Root CA and Intermediate CA must regenerate cleanly.
	writeExpiredRootCA(t, filepath.Join(certDir, "ca"))
	rebuilt, err := IssueLeaf(certDir, domains, 90, 30)
	if err != nil {
		t.Fatalf("IssueLeaf after expired Root CA failed: %v", err)
	}
	rebuiltRootPEM, _ := os.ReadFile(rebuilt.RootCAPath)
	rebuiltRoot, err := parseFirstCertificate(rebuiltRootPEM)
	if err != nil {
		t.Fatalf("parse rebuilt Root CA: %v", err)
	}
	if !rebuiltRoot.NotAfter.After(time.Now().Add(365 * 24 * time.Hour)) {
		t.Fatalf("expected regenerated 10-year Root CA, got NotAfter=%v", rebuiltRoot.NotAfter)
	}
}

func TestIsLocalDomainAndAllLocalDomains(t *testing.T) {
	cases := []struct {
		domain string
		want   bool
	}{
		{"localhost", true},
		{"LOCALHOST.", true},
		{"*.localhost", true},
		{"app.localhost", true},
		{"*.internal", true},
		{"xxx.internal", true},
		{"app.internal", true},
		{"api.v1.internal", true},
		{"*.local", true},
		{"nas.local", true},
		{"*.test", true},
		{"svc.test", true},
		{"127.0.0.1", true},
		{"::1", true},
		{"[::1]:8443", true},
		{"https://app.internal:8443", true},
		{"example.com", false},
		{"*.example.com", false},
		{"internal", false},
		{".internal", false},
		{"*.*.internal", false},
		{"8.8.8.8", false},
		{"192.168.1.1", false},
		{"", false},
	}

	for _, tc := range cases {
		if got := IsLocalDomain(tc.domain); got != tc.want {
			t.Errorf("IsLocalDomain(%q) = %v, want %v", tc.domain, got, tc.want)
		}
	}

	if AllLocalDomains(nil) {
		t.Fatal("AllLocalDomains(nil) = true, want false")
	}
	if !AllLocalDomains([]string{"localhost", "app.internal", "*.internal", "127.0.0.1", "::1"}) {
		t.Fatal("AllLocalDomains returned false for valid local set")
	}
	if AllLocalDomains([]string{"app.internal", "example.com"}) {
		t.Fatal("AllLocalDomains returned true when public domain was present")
	}
}

func TestHostsManagerSetupAndCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	hostsPath := filepath.Join(tmpDir, "hosts")

	initial := strings.Join([]string{
		"##",
		"# Host Database",
		"##",
		"127.0.0.1\tlocalhost",
		"::1\t\tlocalhost",
		"127.0.0.1\tdevbox.lan # user entry",
		"",
	}, "\n")
	if err := os.WriteFile(hostsPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("write initial hosts: %v", err)
	}

	mgr := NewHostsManager(hostsPath, "inst01", slog.Default())
	eligible := mgr.EligibleDomains([]string{
		"localhost",
		"app.internal",
		"api.internal",
		"app.localhost",
		"*.internal",
		"127.0.0.1",
		"::1",
		"example.com",
		"APP.INTERNAL",
	})
	wantEligible := []string{"localhost", "app.internal", "api.internal", "app.localhost"}
	if !reflect.DeepEqual(eligible, wantEligible) {
		t.Fatalf("EligibleDomains = %v, want %v", eligible, wantEligible)
	}

	added, err := mgr.Setup([]string{"localhost", "app.internal", "api.internal", "*.internal", "127.0.0.1"})
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	wantAdded := []string{"app.internal", "api.internal"}
	if !reflect.DeepEqual(added, wantAdded) {
		t.Fatalf("Setup added = %v, want %v", added, wantAdded)
	}

	afterSetupBytes, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("read hosts after Setup: %v", err)
	}
	afterSetup := string(afterSetupBytes)

	for _, mustContain := range []string{
		"# Host Database",
		"127.0.0.1\tdevbox.lan # user entry",
		"127.0.0.1 app.internal # gymbag-managed:inst01",
		"::1 app.internal # gymbag-managed:inst01",
		"127.0.0.1 api.internal # gymbag-managed:inst01",
		"::1 api.internal # gymbag-managed:inst01",
	} {
		if !strings.Contains(afterSetup, mustContain) {
			t.Fatalf("hosts file missing %q:\n%s", mustContain, afterSetup)
		}
	}
	if strings.Contains(afterSetup, "localhost # gymbag-managed:") {
		t.Fatalf("Setup should skip already-present localhost:\n%s", afterSetup)
	}
	if !strings.HasSuffix(afterSetup, "\n") {
		t.Fatal("hosts file should preserve trailing newline")
	}

	// Second Setup call should be a no-op since entries now resolve.
	addedAgain, err := mgr.Setup([]string{"localhost", "app.internal", "api.internal"})
	if err != nil {
		t.Fatalf("second Setup failed: %v", err)
	}
	if len(addedAgain) != 0 {
		t.Fatalf("expected second Setup to add nothing, got %v", addedAgain)
	}

	// Simulate another tool / another Gymbag instance editing hosts while running.
	externalLine := "127.0.0.1 other.internal # gymbag-managed:inst0199\n10.0.0.9 k8s.local\n"
	if err := os.WriteFile(hostsPath, []byte(afterSetup+externalLine), 0o644); err != nil {
		t.Fatalf("append external lines: %v", err)
	}

	if err := mgr.Cleanup(); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	afterCleanupBytes, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("read hosts after Cleanup: %v", err)
	}
	afterCleanup := string(afterCleanupBytes)
	wantAfterCleanup := initial + externalLine
	if afterCleanup != wantAfterCleanup {
		t.Fatalf("Cleanup content mismatch:\ngot:\n%s\nwant:\n%s", afterCleanup, wantAfterCleanup)
	}

	// Idempotent second Cleanup.
	if err := mgr.Cleanup(); err != nil {
		t.Fatalf("idempotent Cleanup failed: %v", err)
	}
}

func TestHostsManagerPermissionDeniedAndConcurrency(t *testing.T) {
	tmpDir := t.TempDir()
	roHostsPath := filepath.Join(tmpDir, "hosts-readonly")
	if err := os.WriteFile(roHostsPath, []byte("127.0.0.1 localhost\n"), 0o444); err != nil {
		t.Fatalf("write readonly hosts: %v", err)
	}
	if err := os.Chmod(roHostsPath, 0o444); err != nil {
		t.Fatalf("chmod readonly hosts: %v", err)
	}

	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		var logBuf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logBuf, nil))
		roMgr := NewHostsManager(roHostsPath, "", logger)
		if roMgr.InstanceID() == "" {
			t.Fatal("expected non-empty auto-generated instanceID")
		}
		added, err := roMgr.Setup([]string{"app.internal"})
		if !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("Setup error = %v, want ErrPermissionDenied", err)
		}
		if added != nil {
			t.Fatalf("Setup added = %v, want nil on permission error", added)
		}
		if !strings.Contains(logBuf.String(), "app.internal") {
			t.Fatalf("expected warning log mentioning app.internal, got: %s", logBuf.String())
		}
	}

	// Concurrency test under -race.
	concHostsPath := filepath.Join(tmpDir, "hosts-concurrent")
	if err := os.WriteFile(concHostsPath, []byte("127.0.0.1 localhost\n"), 0o644); err != nil {
		t.Fatalf("write concurrent hosts: %v", err)
	}
	concMgr := NewHostsManager(concHostsPath, "", nil)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := concMgr.Setup([]string{"app.internal", "api.internal"}); err != nil {
				t.Errorf("concurrent Setup failed: %v", err)
			}
			if err := concMgr.Cleanup(); err != nil {
				t.Errorf("concurrent Cleanup failed: %v", err)
			}
		}()
	}
	wg.Wait()
	if err := concMgr.Cleanup(); err != nil {
		t.Fatalf("final Cleanup failed: %v", err)
	}
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("file %s mode = %04o, want %04o", path, got, want)
	}
}

func writeExpiredRootCA(t *testing.T, caDir string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	ski, err := computeSubjectKeyID(&priv.PublicKey)
	if err != nil {
		t.Fatalf("compute SKI: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   RootCACommonName,
			Organization: []string{CAOrganization},
		},
		NotBefore:             time.Now().Add(-48 * time.Hour),
		NotAfter:              time.Now().Add(-24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
		SubjectKeyId:          ski,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create expired cert: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM, err := marshalPKCS8PEM(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	if err := writePublicFile(filepath.Join(caDir, RootCACertFilename), certPEM); err != nil {
		t.Fatalf("write expired root cert: %v", err)
	}
	if err := writePrivateFile(filepath.Join(caDir, RootCAKeyFilename), keyPEM); err != nil {
		t.Fatalf("write expired root key: %v", err)
	}
}
