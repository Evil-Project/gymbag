package localpki

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gymbag/internal/host"
)

const (
	// RootCACommonName is the Subject CommonName of the local Root CA.
	RootCACommonName = "Neuro-sama Headquarters Root CA"
	// IntermediateCACommonName is the Subject CommonName of the local Intermediate CA.
	IntermediateCACommonName = "Gymbag & Swarm SSL Local CA"
	// CAOrganization is the Subject Organization shared by the local CA hierarchy.
	CAOrganization = "Neuro-sama Headquarters"

	// RootCACertFilename is the PEM certificate filename for the Root CA.
	RootCACertFilename = "neuro-sama-root-ca.pem"
	// RootCAKeyFilename is the PKCS#8 PEM private key filename for the Root CA.
	RootCAKeyFilename = "neuro-sama-root-ca.key"
	// IntermediateCACertFilename is the PEM certificate filename for the Intermediate CA.
	IntermediateCACertFilename = "gymbag-swarm-local-ca.pem"
	// IntermediateCAKeyFilename is the PKCS#8 PEM private key filename for the Intermediate CA.
	IntermediateCAKeyFilename = "gymbag-swarm-local-ca.key"

	// LeafFullchainFilename is the PEM bundle (leaf + intermediate) filename for local leaf certificates.
	LeafFullchainFilename = "local-fullchain.pem"
	// LeafPrivkeyFilename is the PKCS#8 PEM private key filename for local leaf certificates.
	LeafPrivkeyFilename = "local-privkey.pem"

	// DefaultLeafValidityDays is the default validity period in days for local leaf certificates.
	DefaultLeafValidityDays = 90
	// DefaultRenewWithinDays is the default renewal threshold in days before leaf expiry.
	DefaultRenewWithinDays = 30
)

var localDomainSuffixes = []string{
	".localhost",
	".internal",
	".local",
	".test",
}

// IssuedCert holds an issued or reused local TLS leaf certificate and paths to its chain files.
type IssuedCert struct {
	CertPath           string
	KeyPath            string
	RootCAPath         string
	IntermediateCAPath string
	Cert               tls.Certificate
}

// CAManager manages the two-tier local Certificate Authority hierarchy and issues local leaf certificates.
type CAManager struct {
	mu    sync.Mutex
	caDir string
}

// NewCAManager creates a CAManager storing Root and Intermediate CA files in caDir.
// If caDir is empty, IssueLeaf defaults to "<certDir>/ca".
func NewCAManager(caDir string) *CAManager {
	return &CAManager{
		caDir: strings.TrimSpace(caDir),
	}
}

// CADir returns the configured CA storage directory.
func (m *CAManager) CADir() string {
	return m.caDir
}

// RootCAPath returns the path to neuro-sama-root-ca.pem in CADir.
func (m *CAManager) RootCAPath() string {
	return filepath.Join(m.caDir, RootCACertFilename)
}

// RootCAKeyPath returns the path to neuro-sama-root-ca.key in CADir.
func (m *CAManager) RootCAKeyPath() string {
	return filepath.Join(m.caDir, RootCAKeyFilename)
}

// IntermediateCAPath returns the path to gymbag-swarm-local-ca.pem in CADir.
func (m *CAManager) IntermediateCAPath() string {
	return filepath.Join(m.caDir, IntermediateCACertFilename)
}

// IntermediateCAKeyPath returns the path to gymbag-swarm-local-ca.key in CADir.
func (m *CAManager) IntermediateCAKeyPath() string {
	return filepath.Join(m.caDir, IntermediateCAKeyFilename)
}

// EnsureCA loads the Root CA and Intermediate CA from CADir, cleanly regenerating
// either or both if files are missing, corrupt, expired, or mismatched.
func (m *CAManager) EnsureCA() (rootCert, interCert *x509.Certificate, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.caDir == "" {
		return nil, nil, errors.New("localpki: caDir must not be empty")
	}
	state, err := ensureCAHierarchy(m.caDir)
	if err != nil {
		return nil, nil, err
	}
	return state.rootCert, state.interCert, nil
}

// IssueLeaf reuses or issues an ECDSA P-256 leaf certificate signed by "Gymbag & Swarm SSL Local CA"
// (which chains to "Neuro-sama Headquarters Root CA") for domains inside certDir.
func IssueLeaf(certDir string, domains []string, validityDays, renewWithinDays int) (*IssuedCert, error) {
	if strings.TrimSpace(certDir) == "" {
		return nil, errors.New("localpki: certDir must not be empty")
	}
	mgr := NewCAManager(filepath.Join(certDir, "ca"))
	return mgr.IssueLeaf(certDir, domains, validityDays, renewWithinDays)
}

// IssueLeaf reuses <certDir>/local-fullchain.pem and <certDir>/local-privkey.pem when valid,
// not expiring within renewWithinDays (default 30), covering all domains (plus 127.0.0.1 and ::1
// whenever localhost, *.localhost, or *.internal is present), and verifiable against the local CA chain.
// Otherwise it generates and persists a new leaf certificate (default 90 days).
func (m *CAManager) IssueLeaf(certDir string, domains []string, validityDays, renewWithinDays int) (*IssuedCert, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	certDir = strings.TrimSpace(certDir)
	if certDir == "" {
		return nil, errors.New("localpki: certDir must not be empty")
	}

	normalizedDomains, dnsNames, ipAddrs, effectiveTargets, err := expandLeafSANs(domains)
	if err != nil {
		return nil, err
	}

	if validityDays <= 0 {
		validityDays = DefaultLeafValidityDays
	}
	if renewWithinDays <= 0 {
		renewWithinDays = DefaultRenewWithinDays
	}

	caDir := m.caDir
	defaultCADir := filepath.Join(certDir, "ca")
	if caDir == "" {
		caDir = defaultCADir
	}

	caState, err := ensureCAHierarchy(caDir)
	if err != nil {
		return nil, err
	}

	// Ensure Root CA (and Intermediate CA) public PEMs are also kept under <certDir>/ca/
	// even if CAManager was initialized with an external caDir.
	if filepath.Clean(caDir) != filepath.Clean(defaultCADir) {
		if err := writePublicFile(filepath.Join(defaultCADir, RootCACertFilename), caState.rootPEM); err != nil {
			return nil, fmt.Errorf("localpki: failed to mirror root CA to certDir: %w", err)
		}
		if err := writePublicFile(filepath.Join(defaultCADir, IntermediateCACertFilename), caState.interPEM); err != nil {
			return nil, fmt.Errorf("localpki: failed to mirror intermediate CA to certDir: %w", err)
		}
	}

	rootCAPath := filepath.Join(caDir, RootCACertFilename)
	interCAPath := filepath.Join(caDir, IntermediateCACertFilename)
	certPath := filepath.Join(certDir, LeafFullchainFilename)
	keyPath := filepath.Join(certDir, LeafPrivkeyFilename)

	now := time.Now().UTC()
	if pair, ok := tryReuseLeaf(certPath, keyPath, effectiveTargets, renewWithinDays, caState, now); ok {
		return &IssuedCert{
			CertPath:           certPath,
			KeyPath:            keyPath,
			RootCAPath:         rootCAPath,
			IntermediateCAPath: interCAPath,
			Cert:               pair,
		}, nil
	}

	leafPEM, keyPEM, err := buildSignedLeaf(normalizedDomains[0], dnsNames, ipAddrs, validityDays, caState, now)
	if err != nil {
		return nil, err
	}

	fullchainPEM := make([]byte, 0, len(leafPEM)+len(caState.interPEM))
	fullchainPEM = append(fullchainPEM, leafPEM...)
	fullchainPEM = append(fullchainPEM, caState.interPEM...)

	if err := writePublicFile(certPath, fullchainPEM); err != nil {
		return nil, fmt.Errorf("localpki: failed to write leaf fullchain: %w", err)
	}
	if err := writePrivateFile(keyPath, keyPEM); err != nil {
		return nil, fmt.Errorf("localpki: failed to write leaf private key: %w", err)
	}

	pair, err := tls.X509KeyPair(fullchainPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("localpki: failed to load generated leaf keypair: %w", err)
	}
	leafCert, err := parseFirstCertificate(leafPEM)
	if err != nil {
		return nil, err
	}
	pair.Leaf = leafCert

	return &IssuedCert{
		CertPath:           certPath,
		KeyPath:            keyPath,
		RootCAPath:         rootCAPath,
		IntermediateCAPath: interCAPath,
		Cert:               pair,
	}, nil
}

// IsLocalDomain reports whether domain is a local development domain or loopback IP
// (localhost, *.localhost, *.internal, *.local, *.test, 127.0.0.1, ::1).
func IsLocalDomain(domain string) bool {
	name := normalizeDomainToken(domain)
	if name == "" {
		return false
	}
	if addr, err := netip.ParseAddr(name); err == nil {
		addr = addr.Unmap()
		return addr.IsLoopback()
	}
	if name == "localhost" {
		return true
	}
	for _, suffix := range localDomainSuffixes {
		if strings.HasSuffix(name, suffix) {
			prefix := strings.TrimSuffix(name, suffix)
			return isValidDomainPrefix(prefix)
		}
	}
	return false
}

// AllLocalDomains reports whether domains is non-empty and every entry satisfies IsLocalDomain.
func AllLocalDomains(domains []string) bool {
	if len(domains) == 0 {
		return false
	}
	for _, d := range domains {
		if !IsLocalDomain(d) {
			return false
		}
	}
	return true
}

type caMaterial struct {
	rootCert  *x509.Certificate
	rootPriv  *ecdsa.PrivateKey
	rootPEM   []byte
	interCert *x509.Certificate
	interPriv *ecdsa.PrivateKey
	interPEM  []byte
}

func ensureCAHierarchy(caDir string) (*caMaterial, error) {
	now := time.Now().UTC()

	rootCertPath := filepath.Join(caDir, RootCACertFilename)
	rootKeyPath := filepath.Join(caDir, RootCAKeyFilename)
	interCertPath := filepath.Join(caDir, IntermediateCACertFilename)
	interKeyPath := filepath.Join(caDir, IntermediateCAKeyFilename)

	rootCert, rootPriv, rootPEM, rootOK := loadValidRootCA(rootCertPath, rootKeyPath, now)
	if !rootOK {
		var rootKeyPEM []byte
		var err error
		rootCert, rootPriv, rootPEM, rootKeyPEM, err = generateRootCA(now)
		if err != nil {
			return nil, err
		}
		if err := writePublicFile(rootCertPath, rootPEM); err != nil {
			return nil, fmt.Errorf("localpki: failed to write root CA certificate: %w", err)
		}
		if err := writePrivateFile(rootKeyPath, rootKeyPEM); err != nil {
			return nil, fmt.Errorf("localpki: failed to write root CA private key: %w", err)
		}
	}

	var interCert *x509.Certificate
	var interPriv *ecdsa.PrivateKey
	var interPEM []byte
	interOK := false
	if rootOK {
		interCert, interPriv, interPEM, interOK = loadValidIntermediateCA(interCertPath, interKeyPath, rootCert, now)
	}
	if !interOK {
		var interKeyPEM []byte
		var err error
		interCert, interPriv, interPEM, interKeyPEM, err = generateIntermediateCA(rootCert, rootPriv, now)
		if err != nil {
			return nil, err
		}
		if err := writePublicFile(interCertPath, interPEM); err != nil {
			return nil, fmt.Errorf("localpki: failed to write intermediate CA certificate: %w", err)
		}
		if err := writePrivateFile(interKeyPath, interKeyPEM); err != nil {
			return nil, fmt.Errorf("localpki: failed to write intermediate CA private key: %w", err)
		}
	}

	return &caMaterial{
		rootCert:  rootCert,
		rootPriv:  rootPriv,
		rootPEM:   rootPEM,
		interCert: interCert,
		interPriv: interPriv,
		interPEM:  interPEM,
	}, nil
}

func generateRootCA(now time.Time) (*x509.Certificate, *ecdsa.PrivateKey, []byte, []byte, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("localpki: failed to generate root CA key: %w", err)
	}
	ski, err := computeSubjectKeyID(&priv.PublicKey)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("localpki: failed to compute root CA SKI: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("localpki: failed to generate root CA serial: %w", err)
	}

	subject := pkix.Name{
		CommonName:   RootCACommonName,
		Organization: []string{CAOrganization},
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               subject,
		Issuer:                subject,
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
		SubjectKeyId:          ski,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("localpki: failed to sign root CA certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("localpki: failed to parse root CA certificate: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM, err := marshalPKCS8PEM(priv)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return cert, priv, certPEM, keyPEM, nil
}

func generateIntermediateCA(rootCert *x509.Certificate, rootPriv *ecdsa.PrivateKey, now time.Time) (*x509.Certificate, *ecdsa.PrivateKey, []byte, []byte, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("localpki: failed to generate intermediate CA key: %w", err)
	}
	ski, err := computeSubjectKeyID(&priv.PublicKey)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("localpki: failed to compute intermediate CA SKI: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("localpki: failed to generate intermediate CA serial: %w", err)
	}

	notAfter := now.AddDate(5, 0, 0)
	if notAfter.After(rootCert.NotAfter) {
		notAfter = rootCert.NotAfter
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   IntermediateCACommonName,
			Organization: []string{CAOrganization},
		},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
		SubjectKeyId:          ski,
		AuthorityKeyId:        rootCert.SubjectKeyId,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, tmpl, rootCert, &priv.PublicKey, rootPriv)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("localpki: failed to sign intermediate CA certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("localpki: failed to parse intermediate CA certificate: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM, err := marshalPKCS8PEM(priv)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return cert, priv, certPEM, keyPEM, nil
}

func buildSignedLeaf(primaryDomain string, dnsNames []string, ipAddrs []net.IP, validityDays int, ca *caMaterial, now time.Time) (leafPEM, keyPEM []byte, err error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("localpki: failed to generate leaf ECDSA key: %w", err)
	}
	ski, err := computeSubjectKeyID(&priv.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("localpki: failed to compute leaf SKI: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, nil, fmt.Errorf("localpki: failed to generate leaf serial: %w", err)
	}

	cn := primaryDomain
	if len(cn) > 64 {
		cn = cn[:64]
	}

	notAfter := now.Add(time.Duration(validityDays) * 24 * time.Hour)
	if notAfter.After(ca.interCert.NotAfter) {
		notAfter = ca.interCert.NotAfter
	}

	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		SubjectKeyId:          ski,
		AuthorityKeyId:        ca.interCert.SubjectKeyId,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddrs,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, tmpl, ca.interCert, &priv.PublicKey, ca.interPriv)
	if err != nil {
		return nil, nil, fmt.Errorf("localpki: failed to sign leaf certificate: %w", err)
	}
	leafPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM, err = marshalPKCS8PEM(priv)
	if err != nil {
		return nil, nil, err
	}
	return leafPEM, keyPEM, nil
}

func loadValidRootCA(certPath, keyPath string, now time.Time) (*x509.Certificate, *ecdsa.PrivateKey, []byte, bool) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, nil, nil, false
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, nil, false
	}
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return nil, nil, nil, false
	}
	cert, err := parseFirstCertificate(certPEM)
	if err != nil {
		return nil, nil, nil, false
	}
	priv, err := parsePKCS8ECDSAKey(keyPEM)
	if err != nil {
		return nil, nil, nil, false
	}
	if !cert.IsCA || !cert.BasicConstraintsValid || cert.MaxPathLen < 1 {
		return nil, nil, nil, false
	}
	if cert.Subject.CommonName != RootCACommonName {
		return nil, nil, nil, false
	}
	if len(cert.Subject.Organization) == 0 || cert.Subject.Organization[0] != CAOrganization {
		return nil, nil, nil, false
	}
	if len(cert.SubjectKeyId) == 0 {
		return nil, nil, nil, false
	}
	reqUsage := x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	if cert.KeyUsage&reqUsage != reqUsage {
		return nil, nil, nil, false
	}
	if now.Before(cert.NotBefore.Add(-5*time.Minute)) || !cert.NotAfter.After(now.Add(24*time.Hour)) {
		return nil, nil, nil, false
	}
	if err := cert.CheckSignatureFrom(cert); err != nil {
		return nil, nil, nil, false
	}
	return cert, priv, certPEM, true
}

func loadValidIntermediateCA(certPath, keyPath string, rootCert *x509.Certificate, now time.Time) (*x509.Certificate, *ecdsa.PrivateKey, []byte, bool) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, nil, nil, false
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, nil, false
	}
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return nil, nil, nil, false
	}
	cert, err := parseFirstCertificate(certPEM)
	if err != nil {
		return nil, nil, nil, false
	}
	priv, err := parsePKCS8ECDSAKey(keyPEM)
	if err != nil {
		return nil, nil, nil, false
	}
	if !cert.IsCA || !cert.BasicConstraintsValid || cert.MaxPathLen != 0 || !cert.MaxPathLenZero {
		return nil, nil, nil, false
	}
	if cert.Subject.CommonName != IntermediateCACommonName {
		return nil, nil, nil, false
	}
	if len(cert.Subject.Organization) == 0 || cert.Subject.Organization[0] != CAOrganization {
		return nil, nil, nil, false
	}
	if len(cert.SubjectKeyId) == 0 || !bytes.Equal(cert.AuthorityKeyId, rootCert.SubjectKeyId) {
		return nil, nil, nil, false
	}
	reqUsage := x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	if cert.KeyUsage&reqUsage != reqUsage {
		return nil, nil, nil, false
	}
	if now.Before(cert.NotBefore.Add(-5*time.Minute)) || !cert.NotAfter.After(now.Add(24*time.Hour)) {
		return nil, nil, nil, false
	}
	if err := cert.CheckSignatureFrom(rootCert); err != nil {
		return nil, nil, nil, false
	}
	return cert, priv, certPEM, true
}

func tryReuseLeaf(certPath, keyPath string, requiredTargets []string, renewWithinDays int, ca *caMaterial, now time.Time) (tls.Certificate, bool) {
	fullchainPEM, err := os.ReadFile(certPath)
	if err != nil {
		return tls.Certificate{}, false
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return tls.Certificate{}, false
	}
	certs, err := parseAllCertificates(fullchainPEM)
	if err != nil || len(certs) < 2 {
		return tls.Certificate{}, false
	}
	leaf := certs[0]
	bundledInter := certs[1]
	if !bytes.Equal(bundledInter.Raw, ca.interCert.Raw) {
		return tls.Certificate{}, false
	}

	pair, err := tls.X509KeyPair(fullchainPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, false
	}

	if now.Before(leaf.NotBefore.Add(-5 * time.Minute)) {
		return tls.Certificate{}, false
	}
	if leaf.NotAfter.Sub(now) <= time.Duration(renewWithinDays)*24*time.Hour {
		return tls.Certificate{}, false
	}
	if !leafCoversAll(leaf, requiredTargets) {
		return tls.Certificate{}, false
	}
	if err := verifyLeafChain(leaf, ca.interCert, ca.rootCert, now); err != nil {
		return tls.Certificate{}, false
	}

	pair.Leaf = leaf
	return pair, true
}

func verifyLeafChain(leaf, inter, root *x509.Certificate, now time.Time) error {
	roots := x509.NewCertPool()
	roots.AddCert(root)
	inters := x509.NewCertPool()
	inters.AddCert(inter)

	chains, err := leaf.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: inters,
		CurrentTime:   now,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	if err != nil {
		return err
	}
	for _, chain := range chains {
		if len(chain) >= 3 &&
			chain[1].Subject.CommonName == IntermediateCACommonName &&
			chain[len(chain)-1].Subject.CommonName == RootCACommonName {
			return nil
		}
	}
	return errors.New("localpki: leaf did not chain to expected local intermediate and root CAs")
}

func leafCoversAll(leaf *x509.Certificate, targets []string) bool {
	var certIPs []netip.Addr
	for _, ip := range leaf.IPAddresses {
		if addr, ok := netip.AddrFromSlice(ip); ok {
			certIPs = append(certIPs, addr.Unmap())
		}
	}

	for _, target := range targets {
		if addr, err := netip.ParseAddr(target); err == nil {
			want := addr.Unmap()
			matched := false
			for _, got := range certIPs {
				if got == want {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
			continue
		}

		matched := false
		for _, dnsSAN := range leaf.DNSNames {
			if host.Covers(dnsSAN, target) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func expandLeafSANs(domains []string) (normalized []string, dnsNames []string, ipAddrs []net.IP, allTargets []string, err error) {
	seenDNS := make(map[string]struct{})
	seenIP := make(map[netip.Addr]struct{})
	seenNorm := make(map[string]struct{})
	addLoopback := false

	addTarget := func(norm string) {
		if norm == "" {
			return
		}
		if addr, parseErr := netip.ParseAddr(norm); parseErr == nil {
			unmapped := addr.Unmap()
			if _, exists := seenIP[unmapped]; !exists {
				seenIP[unmapped] = struct{}{}
				ip := net.IP(unmapped.AsSlice())
				if ip4 := ip.To4(); ip4 != nil {
					ip = ip4
				}
				ipAddrs = append(ipAddrs, ip)
				allTargets = append(allTargets, unmapped.String())
			}
			return
		}
		if _, exists := seenDNS[norm]; !exists {
			seenDNS[norm] = struct{}{}
			dnsNames = append(dnsNames, norm)
			allTargets = append(allTargets, norm)
		}
	}

	for _, raw := range domains {
		norm := normalizeDomainToken(raw)
		if norm == "" {
			continue
		}
		if _, exists := seenNorm[norm]; !exists {
			seenNorm[norm] = struct{}{}
			normalized = append(normalized, norm)
		}
		if needsLoopbackIPs(norm) {
			addLoopback = true
		}
		addTarget(norm)
	}

	if len(normalized) == 0 {
		return nil, nil, nil, nil, errors.New("localpki: at least one domain is required")
	}

	if addLoopback {
		addTarget("127.0.0.1")
		addTarget("::1")
	}

	return normalized, dnsNames, ipAddrs, allTargets, nil
}

func needsLoopbackIPs(norm string) bool {
	return norm == "localhost" ||
		strings.HasSuffix(norm, ".localhost") ||
		strings.HasSuffix(norm, ".internal")
}

func normalizeDomainToken(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "://") {
		if h, _, _, err := host.ParseSiteAddress(trimmed); err == nil {
			return h
		}
	}
	return host.FromHeader(trimmed)
}

func isValidDomainPrefix(prefix string) bool {
	if prefix == "" {
		return false
	}
	if prefix == "*" {
		return true
	}
	if strings.HasPrefix(prefix, "*.") {
		prefix = prefix[2:]
	}
	if prefix == "" || strings.Contains(prefix, "*") {
		return false
	}
	if strings.HasPrefix(prefix, ".") || strings.HasSuffix(prefix, ".") || strings.Contains(prefix, "..") {
		return false
	}
	return true
}

func computeSubjectKeyID(pub *ecdsa.PublicKey) ([]byte, error) {
	ecdhPub, err := pub.ECDH()
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(ecdhPub.Bytes())
	return sum[:20], nil
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	n, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, err
	}
	return n.Add(n, big.NewInt(1)), nil
}

func marshalPKCS8PEM(priv *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("localpki: failed to marshal PKCS#8 private key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

func parsePKCS8ECDSAKey(keyPEM []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, errors.New("localpki: key file does not contain a PKCS#8 PRIVATE KEY PEM block")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	ecKey, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("localpki: private key is not ECDSA")
	}
	if ecKey.Curve != elliptic.P256() {
		return nil, errors.New("localpki: private key is not ECDSA P-256")
	}
	return ecKey, nil
}

func parseFirstCertificate(pemBytes []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("localpki: PEM data does not contain a CERTIFICATE block")
	}
	return x509.ParseCertificate(block.Bytes)
}

func parseAllCertificates(pemBytes []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	rest := pemBytes
	for len(rest) > 0 {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		certs = append(certs, c)
	}
	if len(certs) == 0 {
		return nil, errors.New("localpki: no certificates found in PEM data")
	}
	return certs, nil
}

func writePrivateFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func writePublicFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	return os.Chmod(path, 0o644)
}
