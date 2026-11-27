package tlsmanager

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gymbag/internal/config"
	"gymbag/internal/host"
	"gymbag/internal/localpki"
)

// Material represents an active PEM certificate & private key pair on disk and in memory.
type Material struct {
	CertPath string
	KeyPath  string
	Source   string
	Provider string
	Cert     tls.Certificate
}

// Provider provisions or loads a TLS certificate (OCP / LSP).
type Provider interface {
	Issue(ctx context.Context) (*Material, error)
}

// RenewalProvider is an optional extension interface for Providers (e.g. Local PKI)
// that accept an explicit renewal window or force-renew flag.
type RenewalProvider interface {
	Provider
	Renew(ctx context.Context, withinDays int, force bool) (*Material, error)
}

// ProviderFunc adapts a function to the Provider interface.
type ProviderFunc func(ctx context.Context) (*Material, error)

func (f ProviderFunc) Issue(ctx context.Context) (*Material, error) {
	return f(ctx)
}

type renewalCtxKey struct{}

type renewalCtxOptions struct {
	withinDays    int
	hasWithinDays bool
	force         bool
}

// WithRenewalWindow returns a context instructing caching certificate providers to treat
// certificates expiring within withinDays as needing renewal.
func WithRenewalWindow(ctx context.Context, withinDays int) context.Context {
	return context.WithValue(ctx, renewalCtxKey{}, renewalCtxOptions{
		withinDays:    withinDays,
		hasWithinDays: true,
	})
}

// WithForceRenew returns a context instructing certificate providers to bypass cached certificates
// and issue or reload fresh TLS material unconditionally.
func WithForceRenew(ctx context.Context) context.Context {
	return context.WithValue(ctx, renewalCtxKey{}, renewalCtxOptions{
		force: true,
	})
}

// RenewalWindowFromContext extracts any renewal window or force-renew flag attached to ctx.
func RenewalWindowFromContext(ctx context.Context, defaultDays int) (withinDays int, force bool) {
	if defaultDays <= 0 {
		defaultDays = 30
	}
	if ctx != nil {
		if opts, ok := ctx.Value(renewalCtxKey{}).(renewalCtxOptions); ok {
			if opts.force {
				return defaultDays, true
			}
			if opts.hasWithinDays {
				return opts.withinDays, false
			}
		}
	}
	return defaultDays, false
}

// FileProvider loads an existing certificate and private key from disk, validating coverage & expiry.
type FileProvider struct {
	CertFile        string
	KeyFile         string
	KeyPassword     string
	Domains         []string
	RenewWithinDays int
	Required        bool
}

func (p *FileProvider) Issue(ctx context.Context) (*Material, error) {
	withinDays := p.RenewWithinDays
	if ctx != nil {
		if opts, ok := ctx.Value(renewalCtxKey{}).(renewalCtxOptions); ok && opts.hasWithinDays && !opts.force {
			withinDays = opts.withinDays
		}
	}
	mat := p.loadWithWindow(withinDays)
	if mat == nil && p.Required {
		return nil, certErrorf("certificate file is missing, expired, or does not cover the configured domains")
	}
	return mat, nil
}

func (p *FileProvider) effectivePassword() string {
	if p.KeyPassword != "" {
		return p.KeyPassword
	}
	return os.Getenv("GYMBAG_KEY_PASSWORD")
}

func (p *FileProvider) load() *Material {
	return p.loadWithWindow(p.RenewWithinDays)
}

func (p *FileProvider) loadWithWindow(withinDays int) *Material {
	if strings.TrimSpace(p.CertFile) == "" || strings.TrimSpace(p.KeyFile) == "" {
		return nil
	}
	pair, leaf, err := LoadKeyPairWithPassword(p.CertFile, p.KeyFile, p.effectivePassword())
	if err != nil {
		return nil
	}
	now := time.Now().UTC()
	if leaf.NotBefore.Add(-5 * time.Minute).After(now) {
		return nil
	}
	if p.Required {
		if !leaf.NotAfter.After(now) {
			return nil
		}
	} else if CertificateNeedsRenewal(leaf, withinDays) {
		return nil
	}
	if !CertificateCovers(leaf, p.Domains) {
		return nil
	}
	return &Material{
		CertPath: p.CertFile,
		KeyPath:  p.KeyFile,
		Source:   "file",
		Cert:     pair,
	}
}

// SelfSignedProvider generates a local ECDSA P-256 certificate covering all configured domains.
type SelfSignedProvider struct {
	Directory    string
	Domains      []string
	Days         int
	CertFilename string
	KeyFilename  string
}

func (p *SelfSignedProvider) Issue(_ context.Context) (*Material, error) {
	days := p.Days
	if days <= 0 {
		days = 90
	}
	certPEM, keyPEM, err := BuildSelfSigned(p.Domains, days)
	if err != nil {
		return nil, err
	}
	certName := strings.TrimSpace(p.CertFilename)
	if certName == "" {
		certName = "selfsigned.pem"
	}
	keyName := strings.TrimSpace(p.KeyFilename)
	if keyName == "" {
		keyName = "selfsigned.key"
	}
	certPath := filepath.Join(p.Directory, certName)
	keyPath := filepath.Join(p.Directory, keyName)
	if err := writePublicFile(certPath, certPEM); err != nil {
		return nil, certErrorf("failed to write self-signed cert: %v", err)
	}
	if err := writePrivateFile(keyPath, keyPEM); err != nil {
		return nil, certErrorf("failed to write self-signed key: %v", err)
	}
	pair, _, err := LoadKeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	return &Material{
		CertPath: certPath,
		KeyPath:  keyPath,
		Source:   "selfsigned",
		Cert:     pair,
	}, nil
}

// LocalCAProvider provisions two-tier local PKI leaf certificates signed by
// "Gymbag & Swarm SSL Local CA" -> "Neuro-sama Headquarters Root CA".
type LocalCAProvider struct {
	Directory       string
	Domains         []string
	Days            int
	RenewWithinDays int
	Required        bool
	AllowNonLocal   bool
	CertFilename    string
	KeyFilename     string
	CA              *localpki.CAManager
}

func (p *LocalCAProvider) effectiveRenewWithinDays() int {
	if p.RenewWithinDays <= 0 {
		return 30
	}
	return p.RenewWithinDays
}

func (p *LocalCAProvider) Issue(ctx context.Context) (*Material, error) {
	withinDays, force := RenewalWindowFromContext(ctx, p.effectiveRenewWithinDays())
	return p.Renew(ctx, withinDays, force)
}

func (p *LocalCAProvider) Renew(_ context.Context, withinDays int, force bool) (*Material, error) {
	if len(p.Domains) == 0 {
		if p.Required {
			return nil, certErrorf("local CA certificate requires at least one domain")
		}
		return nil, nil
	}
	if !p.Required && !p.AllowNonLocal && !localpki.AllLocalDomains(p.Domains) {
		return nil, nil
	}

	days := p.Days
	if days <= 0 {
		days = localpki.DefaultLeafValidityDays
	}
	effectiveWithin := withinDays
	if effectiveWithin <= 0 {
		effectiveWithin = p.effectiveRenewWithinDays()
	}
	if force {
		effectiveWithin = days + 36500
	}

	caMgr := p.CA
	if caMgr == nil {
		caMgr = localpki.NewCAManager(filepath.Join(p.Directory, "ca"))
	}
	issued, err := caMgr.IssueLeafNamed(p.Directory, p.CertFilename, p.KeyFilename, p.Domains, days, effectiveWithin)
	if err != nil {
		return nil, certErrorf("local CA issuance failed: %v", err)
	}
	return &Material{
		CertPath: issued.CertPath,
		KeyPath:  issued.KeyPath,
		Source:   "local",
		Provider: "local",
		Cert:     issued.Cert,
	}, nil
}

func sanitizeProviderSlug(raw string) string {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return "letsencrypt"
	}
	if info, err := ResolveSSLProvider(trimmed); err == nil {
		trimmed = info.Name
	}
	var b strings.Builder
	for i := 0; i < len(trimmed); i++ {
		c := trimmed[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-' {
			b.WriteByte(c)
		} else {
			b.WriteByte('-')
		}
	}
	slug := strings.Trim(b.String(), "-.")
	if slug == "" {
		return "custom"
	}
	return slug
}

// ACMECachePaths returns the provider-scoped cached certificate and private key paths inside certDir.
// Default/Let's Encrypt retains "acme-fullchain.pem" and "acme-privkey.pem" for backward compatibility;
// other providers use "acme-<provider>-fullchain.pem" and "acme-<provider>-privkey.pem".
func ACMECachePaths(certDir, providerName string) (certPath, keyPath string) {
	slug := sanitizeProviderSlug(providerName)
	if slug == "" || slug == "letsencrypt" {
		return filepath.Join(certDir, "acme-fullchain.pem"), filepath.Join(certDir, "acme-privkey.pem")
	}
	return filepath.Join(certDir, "acme-"+slug+"-fullchain.pem"), filepath.Join(certDir, "acme-"+slug+"-privkey.pem")
}

// ACMEProvider obtains or reuses a provider-scoped cached ACME HTTP-01 certificate.
type ACMEProvider struct {
	Client          *ACMEClient
	Board           *HTTP01Board
	Directory       string
	Domains         []string
	Email           string
	ProviderName    string
	RenewWithinDays int
	Required        bool
	AllowIPs        bool
	CertFilename    string
	KeyFilename     string
}

func (p *ACMEProvider) effectiveProviderName() string {
	if strings.TrimSpace(p.ProviderName) != "" {
		return sanitizeProviderSlug(p.ProviderName)
	}
	if p.Client != nil && p.Client.ProviderName() != "" {
		return sanitizeProviderSlug(p.Client.ProviderName())
	}
	return "letsencrypt"
}

// CachePaths returns the on-disk fullchain and private key cache paths for this ACMEProvider.
func (p *ACMEProvider) CachePaths() (certPath, keyPath string) {
	if strings.TrimSpace(p.CertFilename) != "" && strings.TrimSpace(p.KeyFilename) != "" {
		return filepath.Join(p.Directory, p.CertFilename), filepath.Join(p.Directory, p.KeyFilename)
	}
	return ACMECachePaths(p.Directory, p.effectiveProviderName())
}

func (p *ACMEProvider) Issue(ctx context.Context) (*Material, error) {
	if !p.eligible() {
		if p.Required {
			return nil, certErrorf("ACME needs a public domain and a valid email if specified")
		}
		return nil, nil
	}
	provSlug := p.effectiveProviderName()
	certPath, keyPath := p.CachePaths()

	defaultDays := p.RenewWithinDays
	if defaultDays <= 0 {
		defaultDays = 30
	}
	withinDays, force := RenewalWindowFromContext(ctx, defaultDays)

	if !force {
		cached := (&FileProvider{
			CertFile:        certPath,
			KeyFile:         keyPath,
			Domains:         p.Domains,
			RenewWithinDays: withinDays,
		}).load()
		if cached != nil {
			cached.Source = "acme"
			cached.Provider = provSlug
			return cached, nil
		}
	}

	if p.Board != nil {
		defer p.Board.Clear()
	}
	certPEM, keyPEM, err := p.Client.IssueWithOptions(ctx, p.Domains, p.Email, p.Board, p.AllowIPs)
	if err != nil {
		return nil, err
	}
	if err := writePublicFile(certPath, certPEM); err != nil {
		return nil, certErrorf("failed to write ACME cert: %v", err)
	}
	if err := writePrivateFile(keyPath, keyPEM); err != nil {
		return nil, certErrorf("failed to write ACME key: %v", err)
	}
	pair, _, err := LoadKeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	return &Material{
		CertPath: certPath,
		KeyPath:  keyPath,
		Source:   "acme",
		Provider: provSlug,
		Cert:     pair,
	}, nil
}

func (p *ACMEProvider) eligible() bool {
	if p.Email != "" && (!strings.Contains(p.Email, "@") || strings.HasPrefix(p.Email, "@") || strings.HasSuffix(p.Email, "@")) {
		return false
	}
	if len(p.Domains) == 0 {
		return false
	}
	for _, d := range p.Domains {
		if !isPublicACMEIdentifier(d, p.AllowIPs) {
			return false
		}
	}
	return true
}

// BuildACMEClientFromConfig constructs an ACMEClient wired to cfg.TLS (resolving provider catalog,
// custom directory overrides, EAB credentials, ZeroSSL auto-EAB, and account key paths).
func BuildACMEClientFromConfig(cfg config.Config, httpClient *http.Client) (*ACMEClient, error) {
	provRaw := strings.TrimSpace(cfg.TLS.Provider)
	dirURL := strings.TrimSpace(cfg.TLS.ACMEDirectory)
	var autoEABURL string
	provName := "letsencrypt"

	if strings.EqualFold(provRaw, "custom") {
		if dirURL == "" {
			return nil, certErrorf("custom SSL provider requires acme_directory URL")
		}
		info, err := ResolveSSLProvider(dirURL)
		if err != nil {
			return nil, err
		}
		provName = info.Name
		dirURL = info.DirectoryURL
	} else if provRaw != "" && !strings.EqualFold(provRaw, "local") {
		info, err := ResolveSSLProvider(provRaw)
		if err != nil {
			return nil, err
		}
		provName = info.Name
		if info.SupportsAutoEAB {
			autoEABURL = info.AutoEABURL
		}
		// Use the provider's directory URL unless the user explicitly overrode acme_directory away from default LE.
		if dirURL == "" || (info.Name != "letsencrypt" && dirURL == LetsEncryptDirectoryURL) {
			dirURL = info.DirectoryURL
		}
	} else if dirURL != "" {
		if info, ok := inferProviderFromDirectoryURL(dirURL); ok {
			provName = info.Name
			if info.SupportsAutoEAB {
				autoEABURL = info.AutoEABURL
			}
		} else if dirURL != LetsEncryptDirectoryURL {
			provName = "custom"
		}
	}

	acctKeyPath := strings.TrimSpace(cfg.TLS.AccountKeyFile)
	if acctKeyPath == "" {
		if provName == "" || provName == "letsencrypt" {
			acctKeyPath = filepath.Join(cfg.TLS.CertDir, "acme-account.pem")
		} else {
			acctKeyPath = filepath.Join(cfg.TLS.CertDir, "acme-"+sanitizeProviderSlug(provName)+"-account.pem")
		}
	}

	var certKeyPath string
	// If the user explicitly supplied a custom private key without an accompanying cert file (e.g. ssl_key),
	// sign the ACME CSR using their private key!
	if cfg.TLS.KeyFile != "" && cfg.TLS.KeyFile != "key.pem" {
		if _, err := os.Stat(cfg.TLS.CertFile); err != nil {
			certKeyPath = cfg.TLS.KeyFile
		}
	}

	return NewACMEClientWithOptions(ACMEOptions{
		ProviderName:   provName,
		DirectoryURL:   dirURL,
		AccountKeyPath: acctKeyPath,
		CertKeyPath:    certKeyPath,
		EABKID:         cfg.TLS.EABKID,
		EABHMACKey:     cfg.TLS.EABHMACKey,
		AutoEABURL:     autoEABURL,
		HTTPClient:     httpClient,
	}), nil
}

// BuildProviders builds the ordered Provider chain according to cfg.TLS.Mode.
func BuildProviders(cfg config.Config, board *HTTP01Board, client *ACMEClient) []Provider {
	return BuildProvidersWithLocal(cfg, board, client, nil)
}

// BuildProvidersWithLocal builds the ordered Provider chain including an optional Local PKI CA Provider.
func BuildProvidersWithLocal(cfg config.Config, board *HTTP01Board, client *ACMEClient, localProvider Provider) []Provider {
	if cfg.TLSDisabled() {
		return nil
	}

	domains := cfg.Domains()
	explicitFile := (strings.TrimSpace(cfg.TLS.CertFile) != "" && cfg.TLS.CertFile != "cert.pem") ||
		(strings.TrimSpace(cfg.TLS.KeyFile) != "" && cfg.TLS.KeyFile != "key.pem")

	fileProv := &FileProvider{
		CertFile:        cfg.TLS.CertFile,
		KeyFile:         cfg.TLS.KeyFile,
		KeyPassword:     cfg.TLS.KeyPassword,
		Domains:         domains,
		RenewWithinDays: cfg.TLS.RenewWithinDays,
		Required:        cfg.TLS.Mode == "file" || explicitFile,
	}

	var acmeShProv *AcmeShProvider
	if strings.TrimSpace(cfg.TLS.AcmeShPath) != "" {
		acmeShProv = &AcmeShProvider{
			Path:        cfg.TLS.AcmeShPath,
			BaseDir:     cfg.BaseDir,
			Domains:     domains,
			KeyPassword: cfg.TLS.KeyPassword,
			Required:    true,
		}
	}

	if !cfg.TLS.AutoSSLEnabled() {
		var providers []Provider
		if acmeShProv != nil {
			if explicitFile {
				acmeShProv.Required = false
			}
			providers = append(providers, acmeShProv)
		}
		if cfg.TLS.Mode == "file" || explicitFile {
			if acmeShProv == nil {
				fileProv.Required = true
			}
			providers = append(providers, fileProv)
		}
		return providers
	}

	provName := strings.TrimSpace(cfg.TLS.Provider)
	if strings.EqualFold(provName, "local") {
		provName = ""
	}
	acmeProv := &ACMEProvider{
		Client:          client,
		Board:           board,
		Directory:       cfg.TLS.CertDir,
		Domains:         domains,
		Email:           cfg.TLS.ACMEEmail,
		ProviderName:    provName,
		RenewWithinDays: cfg.TLS.RenewWithinDays,
		Required:        cfg.TLS.Mode == "acme",
	}
	selfProv := &SelfSignedProvider{
		Directory: cfg.TLS.CertDir,
		Domains:   domains,
		Days:      90,
	}
	if localProvider == nil {
		localProvider = &LocalCAProvider{
			Directory:       cfg.TLS.CertDir,
			Domains:         domains,
			Days:            90,
			RenewWithinDays: cfg.TLS.RenewWithinDays,
			Required:        cfg.TLS.Mode == "local" || strings.EqualFold(cfg.TLS.Provider, "local"),
		}
	}

	switch cfg.TLS.Mode {
	case "off":
		return nil
	case "file":
		if acmeShProv != nil {
			if explicitFile {
				acmeShProv.Required = false
				return []Provider{acmeShProv, fileProv}
			}
			return []Provider{acmeShProv}
		}
		return []Provider{fileProv}
	case "acme":
		return []Provider{acmeProv}
	case "local":
		return []Provider{localProvider}
	case "selfsigned":
		return []Provider{selfProv}
	default:
		if acmeShProv != nil {
			return []Provider{acmeShProv}
		}
		return []Provider{fileProv, acmeProv, localProvider, selfProv}
	}
}

// BuildVerifyProviders builds the TLS Provider chain for the SSH/Minecraft verification HTTPS portal,
// supporting custom Verify.CertFile/KeyFile, acme.sh paths, RFC 8738 ACME IP/domain certs, Local CA certs, and SelfSigned fallback.
func BuildVerifyProviders(cfg config.Config, targets []string, board *HTTP01Board, client *ACMEClient) []Provider {
	if len(targets) == 0 {
		targets = []string{"127.0.0.1", "::1"}
	}
	if cfg.TLSDisabled() && (cfg.Verify.CertFile == "" || cfg.Verify.KeyFile == "") {
		return nil
	}
	certFile, keyFile := cfg.Verify.CertFile, cfg.Verify.KeyFile
	fileRequired := certFile != "" && keyFile != ""
	if !fileRequired && cfg.TLS.Mode == "file" {
		certFile, keyFile = cfg.TLS.CertFile, cfg.TLS.KeyFile
		fileRequired = true
	}
	fileProv := &FileProvider{
		CertFile:        certFile,
		KeyFile:         keyFile,
		KeyPassword:     cfg.TLS.KeyPassword,
		Domains:         targets,
		RenewWithinDays: cfg.TLS.RenewWithinDays,
		Required:        fileRequired,
	}

	var verifyAcmeSh *AcmeShProvider
	if strings.TrimSpace(cfg.TLS.AcmeShPath) != "" {
		verifyAcmeSh = &AcmeShProvider{
			Path:        cfg.TLS.AcmeShPath,
			BaseDir:     cfg.BaseDir,
			Domains:     targets,
			KeyPassword: cfg.TLS.KeyPassword,
			Required:    !cfg.TLS.AutoSSLEnabled() || cfg.TLS.Mode == "file",
		}
	}

	var acmeTargets []string
	for _, t := range targets {
		if isPublicACMEIdentifier(t, true) {
			acmeTargets = append(acmeTargets, t)
		}
	}

	provName := strings.TrimSpace(cfg.TLS.Provider)
	if strings.EqualFold(provName, "local") {
		provName = ""
	}
	acmeCertFile, acmeKeyFile := "verify-acme-fullchain.pem", "verify-acme-privkey.pem"
	if slug := sanitizeProviderSlug(provName); slug != "" && slug != "letsencrypt" {
		acmeCertFile = "verify-acme-" + slug + "-fullchain.pem"
		acmeKeyFile = "verify-acme-" + slug + "-privkey.pem"
	}
	acmeProv := &ACMEProvider{
		Client:          client,
		Board:           board,
		Directory:       cfg.TLS.CertDir,
		Domains:         acmeTargets,
		Email:           cfg.TLS.ACMEEmail,
		ProviderName:    provName,
		RenewWithinDays: cfg.TLS.RenewWithinDays,
		Required:        cfg.TLS.Mode == "acme",
		AllowIPs:        true,
		CertFilename:    acmeCertFile,
		KeyFilename:     acmeKeyFile,
	}

	localProv := &LocalCAProvider{
		Directory:       cfg.TLS.CertDir,
		Domains:         targets,
		Days:            90,
		RenewWithinDays: cfg.TLS.RenewWithinDays,
		Required:        cfg.TLS.Mode == "local" || strings.EqualFold(cfg.TLS.Provider, "local"),
		AllowNonLocal:   true,
		CertFilename:    "verify-local-fullchain.pem",
		KeyFilename:     "verify-local-privkey.pem",
	}

	selfProv := &SelfSignedProvider{
		Directory:    cfg.TLS.CertDir,
		Domains:      targets,
		Days:         90,
		CertFilename: "verify-selfsigned.pem",
		KeyFilename:  "verify-selfsigned.key",
	}

	if cfg.Verify.CertFile != "" && cfg.Verify.KeyFile != "" {
		return []Provider{fileProv}
	}
	if !cfg.TLS.AutoSSLEnabled() {
		var out []Provider
		if verifyAcmeSh != nil {
			if cfg.TLS.Mode == "file" && cfg.TLS.CertFile != "cert.pem" {
				verifyAcmeSh.Required = false
			}
			out = append(out, verifyAcmeSh)
		}
		if cfg.TLS.Mode == "file" && (verifyAcmeSh == nil || cfg.TLS.CertFile != "cert.pem") {
			out = append(out, fileProv)
		}
		return out
	}
	switch cfg.TLS.Mode {
	case "off":
		return nil
	case "file":
		if verifyAcmeSh != nil {
			if cfg.TLS.CertFile != "cert.pem" {
				verifyAcmeSh.Required = false
				return []Provider{verifyAcmeSh, fileProv}
			}
			return []Provider{verifyAcmeSh}
		}
		return []Provider{fileProv}
	case "acme":
		if len(acmeTargets) > 0 {
			return []Provider{acmeProv}
		}
		return []Provider{localProv, selfProv}
	case "local":
		return []Provider{localProv}
	case "selfsigned":
		return []Provider{selfProv}
	default:
		if len(acmeTargets) > 0 && !strings.EqualFold(cfg.TLS.Provider, "local") {
			if verifyAcmeSh != nil {
				verifyAcmeSh.Required = false
				return []Provider{verifyAcmeSh, acmeProv, localProv, selfProv}
			}
			return []Provider{acmeProv, localProv, selfProv}
		}
		if verifyAcmeSh != nil {
			verifyAcmeSh.Required = false
			return []Provider{verifyAcmeSh, localProv, selfProv}
		}
		return []Provider{localProv, selfProv}
	}
}

type sniCertEntry struct {
	domain   string
	certPath string
	keyPath  string
	password string
	source   string
	leafDER  []byte
	cert     tls.Certificate
}

// Manager coordinates certificate issuance, hot-reloading, and SNI per-site certificate selection.
type Manager struct {
	mu          sync.RWMutex
	providers   []Provider
	current     *Material
	active      atomic.Pointer[tls.Certificate]
	sniCerts    atomic.Pointer[map[string]*tls.Certificate]
	sniEntries  map[string]sniCertEntry
	siteDomains []string
}

// NewManager creates a certificate Manager from an ordered Provider slice.
func NewManager(providers []Provider) *Manager {
	m := &Manager{
		providers: providers,
	}
	empty := make(map[string]*tls.Certificate)
	m.sniCerts.Store(&empty)
	return m
}

// RegisterSiteCertificates loads any per-site custom certificate pairs for SNI selection.
func (m *Manager) RegisterSiteCertificates(sites []config.SiteConfig) error {
	return m.RegisterSiteCertificatesWithOptions(sites, "", "", os.Getenv("GYMBAG_KEY_PASSWORD"))
}

// RegisterSiteCertificatesWithPassword loads per-site custom certificate pairs using an optional key password.
func (m *Manager) RegisterSiteCertificatesWithPassword(sites []config.SiteConfig, password string) error {
	return m.RegisterSiteCertificatesWithOptions(sites, "", "", password)
}

// RegisterSiteCertificatesWithOptions loads per-site cert/key pairs and acme.sh certificates for SNI selection.
func (m *Manager) RegisterSiteCertificatesWithOptions(sites []config.SiteConfig, globalAcmeShPath, baseDir, password string) error {
	if password == "" {
		password = os.Getenv("GYMBAG_KEY_PASSWORD")
	}
	sni := make(map[string]*tls.Certificate)
	entries := make(map[string]sniCertEntry)
	domains := make([]string, 0, len(sites))
	seenDomains := make(map[string]struct{}, len(sites))
	now := time.Now().UTC()

	for _, site := range sites {
		norm := host.Normalize(site.Domain)
		if norm != "" {
			if _, ok := seenDomains[norm]; !ok {
				seenDomains[norm] = struct{}{}
				domains = append(domains, norm)
			}
		}
		if site.CertFile != "" && site.KeyFile != "" {
			pair, leaf, err := LoadKeyPairWithPassword(site.CertFile, site.KeyFile, password)
			if err != nil {
				return certErrorf("failed to load TLS certificate for site %s: %v", site.Domain, err)
			}
			certCopy := pair
			sni[norm] = &certCopy
			var der []byte
			if leaf != nil {
				der = leaf.Raw
			}
			entries[norm] = sniCertEntry{
				domain:   norm,
				certPath: site.CertFile,
				keyPath:  site.KeyFile,
				password: password,
				source:   "file",
				leafDER:  der,
				cert:     certCopy,
			}
			continue
		}
		if site.AcmeShPath != "" {
			cp, kp, pair, leaf, ok := ResolveAcmeShPair(site.AcmeShPath, baseDir, site.Domain, password)
			if !ok {
				return certErrorf("no valid acme.sh certificate found for site %s in %s", site.Domain, site.AcmeShPath)
			}
			certCopy := pair
			sni[norm] = &certCopy
			entries[norm] = sniCertEntry{
				domain:   norm,
				certPath: cp,
				keyPath:  kp,
				password: password,
				source:   "acme.sh",
				leafDER:  leaf.Raw,
				cert:     certCopy,
			}
			continue
		}
		if strings.TrimSpace(globalAcmeShPath) != "" {
			cp, kp, pair, leaf, ok := ResolveAcmeShPair(globalAcmeShPath, baseDir, site.Domain, password)
			if ok && leaf != nil && leaf.NotAfter.After(now) {
				certCopy := pair
				sni[norm] = &certCopy
				entries[norm] = sniCertEntry{
					domain:   norm,
					certPath: cp,
					keyPath:  kp,
					password: password,
					source:   "acme.sh",
					leafDER:  leaf.Raw,
					cert:     certCopy,
				}
			}
		}
	}
	m.mu.Lock()
	m.sniEntries = entries
	m.siteDomains = domains
	m.mu.Unlock()
	m.sniCerts.Store(&sni)
	return nil
}

// Obtain tries each configured Provider in order until one yields valid TLS material.
func (m *Manager) Obtain(ctx context.Context) (*Material, error) {
	var hasRenewOpts bool
	var renewOpts renewalCtxOptions
	if ctx != nil {
		renewOpts, hasRenewOpts = ctx.Value(renewalCtxKey{}).(renewalCtxOptions)
	}

	var errs []string
	for _, prov := range m.providers {
		var (
			mat *Material
			err error
		)
		if hasRenewOpts {
			if rp, ok := prov.(RenewalProvider); ok {
				within := renewOpts.withinDays
				if !renewOpts.hasWithinDays {
					within = 30
				}
				mat, err = rp.Renew(ctx, within, renewOpts.force)
			} else {
				mat, err = prov.Issue(ctx)
			}
		} else {
			mat, err = prov.Issue(ctx)
		}
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if mat != nil {
			if mat.Cert.Leaf == nil && len(mat.Cert.Certificate) > 0 {
				if parsed, lErr := x509.ParseCertificate(mat.Cert.Certificate[0]); lErr == nil {
					mat.Cert.Leaf = parsed
				}
			}
			certCopy := mat.Cert
			m.active.Store(&certCopy)
			m.mu.Lock()
			m.current = mat
			m.mu.Unlock()
			return mat, nil
		}
	}

	if len(errs) == 0 {
		m.mu.RLock()
		domains := append([]string(nil), m.siteDomains...)
		entries := m.sniEntries
		m.mu.RUnlock()
		if len(domains) > 0 && len(entries) >= len(domains) {
			allCovered := true
			for _, d := range domains {
				if _, ok := entries[d]; !ok {
					allCovered = false
					break
				}
			}
			if allCovered {
				first := entries[domains[0]]
				mat := &Material{
					CertPath: first.certPath,
					KeyPath:  first.keyPath,
					Source:   first.source,
					Provider: first.source,
					Cert:     first.cert,
				}
				certCopy := mat.Cert
				m.active.Store(&certCopy)
				m.mu.Lock()
				m.current = mat
				m.mu.Unlock()
				return mat, nil
			}
		}
	}

	detail := strings.Join(errs, "; ")
	if detail == "" {
		detail = "no certificate provider could issue a certificate"
	}
	return nil, certErrorf("%s", detail)
}

// Current returns the currently active Material.
func (m *Manager) Current() *Material {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

func materialLeaf(mat *Material) *x509.Certificate {
	if mat == nil {
		return nil
	}
	if mat.Cert.Leaf != nil {
		return mat.Cert.Leaf
	}
	if len(mat.Cert.Certificate) > 0 {
		if leaf, err := x509.ParseCertificate(mat.Cert.Certificate[0]); err == nil {
			return leaf
		}
	}
	if mat.CertPath != "" {
		if raw, err := os.ReadFile(mat.CertPath); err == nil {
			if leaf, err := LoadLeafCertificate(raw); err == nil {
				return leaf
			}
		}
	}
	return nil
}

func (m *Manager) reloadSNICerts(force bool) bool {
	m.mu.RLock()
	prevEntries := make(map[string]sniCertEntry, len(m.sniEntries))
	for k, v := range m.sniEntries {
		prevEntries[k] = v
	}
	cur := m.current
	m.mu.RUnlock()

	if len(prevEntries) == 0 {
		return false
	}

	changed := false
	nextEntries := make(map[string]sniCertEntry, len(prevEntries))
	nextSNI := make(map[string]*tls.Certificate, len(prevEntries))
	var updatedActive *tls.Certificate
	var activeCertPath string
	if cur != nil {
		activeCertPath = cur.CertPath
	}

	for domain, entry := range prevEntries {
		needReload := force
		if !needReload && entry.certPath != "" {
			if raw, err := os.ReadFile(entry.certPath); err == nil {
				if diskLeaf, lErr := LoadLeafCertificate(raw); lErr == nil {
					if len(entry.leafDER) == 0 || !bytes.Equal(entry.leafDER, diskLeaf.Raw) {
						needReload = true
					}
				}
			}
		}
		if needReload && entry.certPath != "" && entry.keyPath != "" {
			if pair, leaf, err := LoadKeyPairWithPassword(entry.certPath, entry.keyPath, entry.password); err == nil {
				if leaf != nil && (force || !bytes.Equal(entry.leafDER, leaf.Raw)) {
					changed = true
				}
				entry.cert = pair
				if leaf != nil {
					entry.leafDER = leaf.Raw
				}
				if activeCertPath != "" && entry.certPath == activeCertPath {
					c := entry.cert
					updatedActive = &c
				}
			}
		}
		nextEntries[domain] = entry
		c := entry.cert
		nextSNI[domain] = &c
	}

	if changed || force {
		m.mu.Lock()
		m.sniEntries = nextEntries
		if updatedActive != nil && m.current != nil && m.current.CertPath == activeCertPath {
			matCopy := *m.current
			matCopy.Cert = *updatedActive
			m.current = &matCopy
			m.active.Store(updatedActive)
		}
		m.mu.Unlock()
		m.sniCerts.Store(&nextSNI)
	}
	return changed
}

// RenewIfNeeded checks whether the active certificate expires within withinDays (or was replaced on disk)
// and re-obtains TLS material seamlessly across ACME, Local CA, SelfSigned, File, and acme.sh providers.
func (m *Manager) RenewIfNeeded(ctx context.Context, withinDays int) (*Material, error) {
	sniUpdated := m.reloadSNICerts(false)

	cur := m.Current()
	if cur == nil {
		return nil, nil
	}

	isExternal := cur.Source == "file" || cur.Source == "acme.sh"
	prevLeaf := materialLeaf(cur)
	var prevDER []byte
	inMemExpiring := !isExternal
	if prevLeaf != nil {
		prevDER = prevLeaf.Raw
		if !isExternal {
			inMemExpiring = CertificateNeedsRenewal(prevLeaf, withinDays)
		}
	}

	diskChangedOrExpiring := false
	if cur.CertPath != "" {
		if raw, err := os.ReadFile(cur.CertPath); err != nil {
			diskChangedOrExpiring = true
		} else if diskLeaf, err := LoadLeafCertificate(raw); err != nil {
			diskChangedOrExpiring = true
		} else {
			if len(prevDER) > 0 && !bytes.Equal(prevDER, diskLeaf.Raw) {
				diskChangedOrExpiring = true
			} else if !isExternal && CertificateNeedsRenewal(diskLeaf, withinDays) {
				diskChangedOrExpiring = true
			}
		}
	}

	if !inMemExpiring && !diskChangedOrExpiring {
		if sniUpdated {
			return m.Current(), nil
		}
		return nil, nil
	}

	fresh, err := m.Obtain(WithRenewalWindow(ctx, withinDays))
	if err != nil {
		return nil, err
	}
	freshLeaf := materialLeaf(fresh)
	if !sniUpdated && fresh.CertPath == cur.CertPath && freshLeaf != nil && len(prevDER) > 0 && bytes.Equal(prevDER, freshLeaf.Raw) {
		return nil, nil
	}
	return fresh, nil
}

// ForceRenew unconditionally re-issues or reloads the active TLS certificate, bypassing cached material.
func (m *Manager) ForceRenew(ctx context.Context) (*Material, error) {
	m.reloadSNICerts(true)
	return m.Obtain(WithForceRenew(ctx))
}

// GetCertificate implements crypto/tls.Config.GetCertificate for zero-downtime hot-reload & SNI.
func (m *Manager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if hello != nil && hello.ServerName != "" {
		if sniMap := m.sniCerts.Load(); sniMap != nil && len(*sniMap) > 0 {
			sni := host.Normalize(hello.ServerName)
			if custom, ok := (*sniMap)[sni]; ok {
				return custom, nil
			}
		}
	}
	cert := m.active.Load()
	if cert == nil {
		return nil, certErrorf("no active TLS certificate loaded")
	}
	return cert, nil
}

// TLSConfig returns a production crypto/tls.Config wired to m.GetCertificate.
func (m *Manager) TLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion:     tls.VersionTLS12,
		NextProtos:     []string{"h2", "http/1.1"},
		GetCertificate: m.GetCertificate,
	}
}
