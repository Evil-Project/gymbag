package server

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gymbag/internal/config"
	"gymbag/internal/intel"
	"gymbag/internal/localpki"
	"gymbag/internal/proxy"
	"gymbag/internal/store"
	"gymbag/internal/tlsmanager"
	"gymbag/internal/waf"
)

// RuntimeOptions allows dependency injection of custom transports, stores, intelligence managers,
// remote rules managers, ACME clients, hosts file paths, and loggers.
type RuntimeOptions struct {
	Store           store.Store
	Intel           *intel.Manager
	IntelHTTP       *http.Client
	RemoteRules     *waf.RemoteRuleManager
	RemoteRulesHTTP *http.Client
	HTTPTransport   http.RoundTripper
	WSDialer        proxy.DialContextFunc
	ACMEHTTP        *http.Client
	HostsFilePath   string
	Logger          *slog.Logger
}

// Runtime is the composition root wiring configuration, WAF, IP intelligence, remote rules,
// reverse proxy, TLS, local PKI hosts management, and HTTP handlers.
type Runtime struct {
	mu            sync.RWMutex
	cfg           config.Config
	acmeHTTP      *http.Client
	providerChain *dynamicProviderChain

	Logger      *slog.Logger
	Store       store.Store
	Intel       *intel.Manager
	RemoteRules *waf.RemoteRuleManager
	Clearance   *waf.ClearanceTokens
	Challenges  *waf.ChallengeService
	Pages       *waf.Renderer
	Router      *proxy.SiteRouter
	HTTPProxy   *proxy.HTTPProxy
	WSProxy     *proxy.WebSocketProxy
	ACMEBoard   *tlsmanager.HTTP01Board
	ACMEClient  *tlsmanager.ACMEClient
	CertManager *tlsmanager.Manager
	Hosts       *localpki.HostsManager
	HSTS        atomic.Bool

	HTTPSHandler    http.Handler
	RedirectHandler http.Handler
	AdminHandler    http.Handler
}

type dynamicProviderChain struct {
	mu        sync.RWMutex
	providers []tlsmanager.Provider
}

func newDynamicProviderChain(providers []tlsmanager.Provider) *dynamicProviderChain {
	return &dynamicProviderChain{
		providers: append([]tlsmanager.Provider(nil), providers...),
	}
}

func (d *dynamicProviderChain) setProviders(providers []tlsmanager.Provider) {
	d.mu.Lock()
	d.providers = append([]tlsmanager.Provider(nil), providers...)
	d.mu.Unlock()
}

func (d *dynamicProviderChain) snapshot() []tlsmanager.Provider {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return append([]tlsmanager.Provider(nil), d.providers...)
}

func (d *dynamicProviderChain) Issue(ctx context.Context) (*tlsmanager.Material, error) {
	var errs []string
	for _, prov := range d.snapshot() {
		mat, err := prov.Issue(ctx)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if mat != nil {
			return mat, nil
		}
	}
	if len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}
	return nil, nil
}

func (d *dynamicProviderChain) Renew(ctx context.Context, withinDays int, force bool) (*tlsmanager.Material, error) {
	var errs []string
	for _, prov := range d.snapshot() {
		var (
			mat *tlsmanager.Material
			err error
		)
		if rp, ok := prov.(tlsmanager.RenewalProvider); ok {
			mat, err = rp.Renew(ctx, withinDays, force)
		} else {
			subCtx := ctx
			if force {
				subCtx = tlsmanager.WithForceRenew(ctx)
			} else {
				subCtx = tlsmanager.WithRenewalWindow(ctx, withinDays)
			}
			mat, err = prov.Issue(subCtx)
		}
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if mat != nil {
			return mat, nil
		}
	}
	if len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}
	return nil, nil
}

func effectiveKeyPassword(cfg config.Config) string {
	if cfg.TLS.KeyPassword != "" {
		return cfg.TLS.KeyPassword
	}
	return os.Getenv("GYMBAG_KEY_PASSWORD")
}

// ResolveSecret returns the configured clearance secret or loads/generates <cert_dir>/secret (0600).
func ResolveSecret(cfg config.Config, logger *slog.Logger) ([]byte, error) {
	if cfg.Secret != "" {
		return []byte(cfg.Secret), nil
	}
	secretPath := filepath.Join(cfg.TLS.CertDir, "secret")
	if raw, err := os.ReadFile(secretPath); err == nil {
		trimmed := strings.TrimSpace(string(raw))
		if trimmed != "" {
			return []byte(trimmed), nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(secretPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create cert_dir %s: %w", cfg.TLS.CertDir, err)
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	generated := base64.RawURLEncoding.EncodeToString(buf)
	if err := os.WriteFile(secretPath, []byte(generated+"\n"), 0o600); err != nil {
		return nil, fmt.Errorf("failed to persist clearance secret: %w", err)
	}
	_ = os.Chmod(secretPath, 0o600)
	if logger != nil {
		logger.Info("generated a new clearance secret")
	}
	return []byte(generated), nil
}

// BuildRuntime wires all application components from cfg and optional test overrides.
func BuildRuntime(ctx context.Context, cfg config.Config, opts RuntimeOptions) (*Runtime, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	st := opts.Store
	if st == nil {
		if cfg.RedisURL != "" {
			logger.Info("connecting to Redis for WAF state")
		}
		st = store.Open(ctx, cfg.RedisURL, logger)
	}

	secret, err := ResolveSecret(cfg, logger)
	if err != nil {
		_ = st.Close()
		return nil, err
	}

	intelMgr := opts.Intel
	if intelMgr == nil {
		intelMgr = intel.NewManager(intel.ManagerOptions{
			CacheDir:              filepath.Join(cfg.TLS.CertDir, "intel-cache"),
			IPSecurityEnabled:     cfg.Intel.IPSecurity,
			LiveReputationEnabled: cfg.Intel.LiveReputation,
			AIIPListEnabled:       cfg.Intel.AIIPList,
			RefreshInterval:       time.Duration(cfg.Intel.RefreshInterval) * time.Second,
			ExtraThreatFeeds:      cfg.Intel.ThreatFeeds,
			ExtraAIFeeds:          cfg.Intel.AIIPFeeds,
			HTTPClient:            opts.IntelHTTP,
			Logger:                logger,
		})
	}

	remoteMgr := opts.RemoteRules
	if remoteMgr == nil {
		remoteMgr = waf.NewRemoteRuleManager(waf.RemoteRuleManagerOptions{
			CacheDir:        filepath.Join(cfg.TLS.CertDir, "rules-cache"),
			RefreshInterval: time.Duration(cfg.RemoteRulesRefresh) * time.Second,
			URLs:            cfg.RemoteRuleURLs(),
			HTTPClient:      opts.RemoteRulesHTTP,
			Logger:          logger,
		})
	} else {
		remoteMgr.SetURLs(cfg.RemoteRuleURLs())
	}

	clearance := waf.NewClearanceTokens(secret, cfg.Defaults.ClearanceTTL)
	challenges := waf.NewChallengeService(st, cfg.Defaults.Difficulty, cfg.Defaults.ChallengeTTL)
	pages := waf.NewRenderer("")
	router := proxy.NewSiteRouterWithProviders(cfg.Sites, st, clearance, intelMgr, cfg.Intel.IPSecurityAction, remoteMgr)
	httpProxy := proxy.NewHTTPProxy(cfg.HTTPSPort, opts.HTTPTransport, logger)
	wsProxy := proxy.NewWebSocketProxy(opts.WSDialer, logger)

	board := tlsmanager.NewHTTP01Board()
	acmeClient, err := tlsmanager.BuildACMEClientFromConfig(cfg, opts.ACMEHTTP)
	if err != nil {
		_ = st.Close()
		return nil, err
	}
	providerChain := newDynamicProviderChain(tlsmanager.BuildProviders(cfg, board, acmeClient))
	certMgr := tlsmanager.NewManager([]tlsmanager.Provider{providerChain})
	if err := certMgr.RegisterSiteCertificatesWithPassword(cfg.Sites, effectiveKeyPassword(cfg)); err != nil {
		_ = st.Close()
		return nil, err
	}

	hostsMgr := localpki.NewHostsManager(opts.HostsFilePath, "", logger)
	trustedNets := ParseTrustedProxies(cfg.TrustedProxies)

	rt := &Runtime{
		cfg:           cfg,
		acmeHTTP:      opts.ACMEHTTP,
		providerChain: providerChain,
		Logger:        logger,
		Store:         st,
		Intel:         intelMgr,
		RemoteRules:   remoteMgr,
		Clearance:     clearance,
		Challenges:    challenges,
		Pages:         pages,
		Router:        router,
		HTTPProxy:     httpProxy,
		WSProxy:       wsProxy,
		ACMEBoard:     board,
		ACMEClient:    acmeClient,
		CertManager:   certMgr,
		Hosts:         hostsMgr,
	}
	if cfg.TLS.HSTS != nil {
		rt.HSTS.Store(*cfg.TLS.HSTS)
	}
	remoteMgr.SetOnUpdate(rt.reloadRouter)

	gateway := NewGatewayHandler(challenges, clearance, httpProxy, wsProxy, board, trustedNets)
	// Fused high-throughput HTTPS handler: avoids per-request context cloning, duplicate IP resolution,
	// and duplicate WebSocket upgrade checks while preserving identical security semantics.
	rt.HTTPSHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr := w.Header()
		hdr["Server"] = serverHeaderValue

		route, hostName, ok := router.ResolveWithHost(r.Host)
		if !ok {
			hdr.Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(invalidHostBytes)
			return
		}

		isWS := proxy.IsWebSocketUpgrade(r)
		if rt.HSTS.Load() && !isWS {
			hdr["Strict-Transport-Security"] = hstsHeaderValue
		}

		clientIP := ResolveClientIP(r, trustedNets)
		var decision waf.Decision
		if fastGate, isFast := route.Gate.(waf.HTTPAccessGate); isFast {
			decision = fastGate.DecideHTTP(r.Context(), r, hostName, clientIP)
		} else {
			incoming := waf.FromHTTPRequest(r, clientIP)
			decision = route.Gate.Decide(r.Context(), incoming)
		}

		if isWS {
			if decision.Action != waf.ActionAllow {
				status := decision.StatusCode
				if status == 0 {
					status = http.StatusForbidden
				}
				hdr.Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(status)
				_, _ = w.Write(wsClearanceBytes)
				return
			}
			gateway.ServeRouted(w, r, route, hostName, clientIP, true)
			return
		}

		switch decision.Action {
		case waf.ActionDeny:
			pages.WriteReject(w, decision.Reason, decision.StatusCode)
			return
		case waf.ActionChallenge:
			chalID, diff, cErr := challenges.Issue(
				r.Context(),
				hostName,
				clientIP,
				route.Config.WAF.Difficulty,
				route.Config.WAF.ChallengeTTL,
			)
			if cErr != nil {
				http.Error(w, "failed to issue challenge", http.StatusInternalServerError)
				return
			}
			pages.WriteChallenge(w, chalID, diff)
			return
		default:
			gateway.ServeRouted(w, r, route, hostName, clientIP, false)
		}
	})

	rt.RedirectHandler = NewRedirectHandler(router, cfg.HTTPSPort, board)
	rt.AdminHandler = NewAdminHandler(rt)
	return rt, nil
}

var (
	serverHeaderValue = []string{"gymbag"}
	hstsHeaderValue   = []string{"max-age=31536000"}
	invalidHostBytes  = []byte("Invalid host header")
	wsClearanceBytes  = []byte("WebSocket clearance required")
)

func (rt *Runtime) reloadRouter() {
	rt.mu.RLock()
	cfg := rt.cfg
	rt.mu.RUnlock()
	rt.Router.ReloadWithProviders(cfg.Sites, rt.Store, rt.Clearance, rt.Intel, cfg.Intel.IPSecurityAction, rt.RemoteRules)
}

// Snapshot returns a thread-safe copy of the active Config (implements ConfigController).
func (rt *Runtime) Snapshot() config.Config {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	snap := rt.cfg
	snap.Sites = rt.Router.Sites()
	return snap
}

// ReplaceConfig validates next and atomically reloads site routing, custom rules, TLS providers,
// and per-site SNI certificates.
func (rt *Runtime) ReplaceConfig(next config.Config) error {
	if err := next.Validate(); err != nil {
		return err
	}
	acmeClient, err := tlsmanager.BuildACMEClientFromConfig(next, rt.acmeHTTP)
	if err != nil {
		return err
	}
	if err := rt.CertManager.RegisterSiteCertificatesWithPassword(next.Sites, effectiveKeyPassword(next)); err != nil {
		return err
	}
	if rt.providerChain != nil {
		rt.providerChain.setProviders(tlsmanager.BuildProviders(next, rt.ACMEBoard, acmeClient))
	}
	var addedURLs []string
	if rt.RemoteRules != nil {
		rt.RemoteRules.SetRefreshInterval(time.Duration(next.RemoteRulesRefresh) * time.Second)
		addedURLs = rt.RemoteRules.SetURLs(next.RemoteRuleURLs())
	}
	rt.mu.Lock()
	rt.cfg = next
	rt.ACMEClient = acmeClient
	rt.mu.Unlock()

	rt.Router.ReloadWithProviders(next.Sites, rt.Store, rt.Clearance, rt.Intel, next.Intel.IPSecurityAction, rt.RemoteRules)
	if next.TLS.HSTS != nil {
		rt.HSTS.Store(*next.TLS.HSTS)
	} else {
		rt.HSTS.Store(tlsmanager.HSTSEnabled(nil, rt.TLSSource()))
	}
	if rt.RemoteRules != nil && len(addedURLs) > 0 {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			rt.RemoteRules.RefreshNow(ctx)
		}()
	}
	return nil
}

// TLSSource returns the active certificate provider source name ("file", "acme", "local", "selfsigned", or "none").
func (rt *Runtime) TLSSource() string {
	if cur := rt.CertManager.Current(); cur != nil {
		return cur.Source
	}
	return "none"
}

// TLSStatus returns active TLS certificate metadata, configured/effective SSL provider, HSTS state,
// and the full catalog of supported ACME SSL providers.
func (rt *Runtime) TLSStatus() TLSStatus {
	snap := rt.Snapshot()
	cur := rt.CertManager.Current()

	source := "none"
	provider := strings.TrimSpace(snap.TLS.Provider)
	var certPath, keyPath, issuer, subject, serial string
	var dnsNames []string
	var notBefore, notAfter *time.Time

	if cur != nil {
		if cur.Source != "" {
			source = cur.Source
		}
		if cur.Provider != "" {
			provider = cur.Provider
		} else if provider == "" && cur.Source != "" {
			provider = cur.Source
		}
		certPath = cur.CertPath
		keyPath = cur.KeyPath

		leaf := cur.Cert.Leaf
		if leaf == nil && len(cur.Cert.Certificate) > 0 {
			if parsed, err := x509.ParseCertificate(cur.Cert.Certificate[0]); err == nil {
				leaf = parsed
			}
		}
		if leaf != nil {
			issuer = leaf.Issuer.CommonName
			subject = leaf.Subject.CommonName
			if len(leaf.DNSNames) > 0 {
				dnsNames = append([]string(nil), leaf.DNSNames...)
			}
			if leaf.SerialNumber != nil {
				serial = leaf.SerialNumber.String()
			}
			nb := leaf.NotBefore.UTC()
			na := leaf.NotAfter.UTC()
			notBefore = &nb
			notAfter = &na
		}
	}

	if provider == "" {
		switch snap.TLS.Mode {
		case "local":
			provider = "local"
		case "selfsigned":
			provider = "selfsigned"
		case "file":
			provider = "file"
		case "acme":
			provider = "letsencrypt"
		default:
			if localpki.AllLocalDomains(snap.Domains()) {
				provider = "local"
			} else {
				provider = "letsencrypt"
			}
		}
	}

	return TLSStatus{
		Mode:               snap.TLS.Mode,
		Source:             source,
		Provider:           provider,
		ConfiguredProvider: snap.TLS.Provider,
		CertPath:           certPath,
		KeyPath:            keyPath,
		Issuer:             issuer,
		Subject:            subject,
		DNSNames:           dnsNames,
		SerialNumber:       serial,
		NotBefore:          notBefore,
		NotAfter:           notAfter,
		HSTS:               rt.HSTS.Load(),
		Providers:          tlsmanager.ListSSLProviders(),
	}
}

// RenewTLS triggers an immediate forced certificate issuance/renewal and refreshes HSTS state.
func (rt *Runtime) RenewTLS(ctx context.Context) (TLSStatus, error) {
	mat, err := rt.CertManager.ForceRenew(ctx)
	if err != nil {
		return TLSStatus{}, err
	}
	snap := rt.Snapshot()
	if mat != nil {
		rt.HSTS.Store(tlsmanager.HSTSEnabled(snap.TLS.HSTS, mat.Source))
	}
	return rt.TLSStatus(), nil
}

// SetupHosts adds managed loopback entries to the hosts file for any eligible local domains
// when auto_hosts is enabled.
func (rt *Runtime) SetupHosts() ([]string, error) {
	if rt.Hosts == nil {
		return nil, nil
	}
	snap := rt.Snapshot()
	if !snap.TLS.AutoHostsEnabled() {
		return nil, nil
	}
	return rt.Hosts.Setup(snap.Domains())
}

// CleanupHosts removes any hosts file entries tagged by this Runtime instance.
func (rt *Runtime) CleanupHosts() error {
	if rt.Hosts == nil {
		return nil
	}
	return rt.Hosts.Cleanup()
}

// IntelStatus returns live statistics for IP security feeds, AI robot IP lists, and reputation cache.
func (rt *Runtime) IntelStatus() intel.Status {
	return rt.Intel.Status()
}

// RefreshIntel triggers an immediate refresh of all configured IP security and AI robot feeds.
func (rt *Runtime) RefreshIntel(ctx context.Context) intel.Status {
	return rt.Intel.RefreshNow(ctx)
}

// RemoteRulesStatus returns live statistics for all configured remote firewall rule URLs.
func (rt *Runtime) RemoteRulesStatus() waf.RemoteRulesStatus {
	return rt.RemoteRules.Status()
}

// RefreshRemoteRules triggers an immediate refresh of all configured remote firewall rule URLs.
func (rt *Runtime) RefreshRemoteRules(ctx context.Context) waf.RemoteRulesStatus {
	return rt.RemoteRules.RefreshNow(ctx)
}

// Close removes any managed hosts entries and releases underlying state store resources.
func (rt *Runtime) Close() error {
	hostsErr := rt.CleanupHosts()
	if errors.Is(hostsErr, localpki.ErrPermissionDenied) {
		hostsErr = nil
	}
	storeErr := rt.Store.Close()
	return errors.Join(hostsErr, storeErr)
}

// Serve starts automatic /etc/hosts setup (when enabled), background IP/AI feed auto-updates,
// the HTTP (:80) redirect/ACME server, obtains TLS certificates, starts the HTTPS (:443) gateway
// server (plus optional Admin REST server), and blocks until ctx is cancelled or a server fails.
func (rt *Runtime) Serve(ctx context.Context) error {
	defer rt.Close()
	cfg := rt.Snapshot()

	if cfg.TLS.AutoHostsEnabled() {
		if added, err := rt.SetupHosts(); err != nil {
			if errors.Is(err, localpki.ErrPermissionDenied) {
				rt.Logger.Warn(
					"skipping automatic hosts setup due to insufficient permissions",
					"hosts_file", rt.Hosts.HostsFilePath(),
					"error", err,
				)
			} else {
				rt.Logger.Warn(
					"automatic hosts setup failed",
					"hosts_file", rt.Hosts.HostsFilePath(),
					"error", err,
				)
			}
		} else if len(added) > 0 {
			rt.Logger.Info(
				"configured local domains in hosts file",
				"hosts_file", rt.Hosts.HostsFilePath(),
				"domains", strings.Join(added, ", "),
			)
		}
		defer func() {
			_ = rt.CleanupHosts()
		}()
	}

	// Start automatic background updates for IP security feeds, AI robot IP lists, and remote firewall rules.
	rt.Intel.Start(ctx)
	if rt.RemoteRules != nil {
		rt.RemoteRules.Start(ctx)
	}

	rt.Logger.Info(
		"starting Gymbag gateway",
		"http_port", cfg.HTTPPort,
		"https_port", cfg.HTTPSPort,
		"domains", strings.Join(cfg.Domains(), ", "),
		"ip_security", cfg.Intel.IPSecurity,
		"ai_ip_list", cfg.Intel.AIIPList,
	)
	for _, site := range cfg.Sites {
		rt.Logger.Info(
			"configured website",
			"domain", site.Domain,
			"upstreams", strings.Join(site.Upstreams, ","),
			"protected", site.Protect,
			"verify_level", int(site.WAF.VerifyLevel),
			"verify_mode", site.WAF.VerifyLevel.Label(),
			"custom_rules", len(site.WAF.Rules),
		)
	}

	if (cfg.HTTPPort < 1024 || cfg.HTTPSPort < 1024) && os.Geteuid() != 0 {
		rt.Logger.Warn(
			"ports below 1024 usually need administrator rights; set GYMBAG_HTTP_PORT and GYMBAG_HTTPS_PORT for local use",
		)
	}

	httpAddr := net.JoinHostPort(cfg.BindHost, fmt.Sprintf("%d", cfg.HTTPPort))
	httpLn, err := net.Listen("tcp", httpAddr)
	if err != nil {
		return fmt.Errorf("failed to bind HTTP listener on %s: %w", httpAddr, err)
	}
	httpSrv := &http.Server{
		Handler:           rt.RedirectHandler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 3)
	go func() {
		if serveErr := httpSrv.Serve(httpLn); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- fmt.Errorf("HTTP server failed: %w", serveErr)
		}
	}()

	shutdownHTTP := func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutCtx)
	}

	material, err := rt.CertManager.Obtain(ctx)
	if err != nil {
		shutdownHTTP()
		return err
	}
	rt.HSTS.Store(tlsmanager.HSTSEnabled(cfg.TLS.HSTS, material.Source))
	rt.Logger.Info("TLS certificate ready", "source", material.Source, "file", material.CertPath)

	var adminSrv *http.Server
	if cfg.AdminAddr != "" {
		adminLn, adminErr := net.Listen("tcp", cfg.AdminAddr)
		if adminErr != nil {
			shutdownHTTP()
			return fmt.Errorf("failed to bind Admin REST API on %s: %w", cfg.AdminAddr, adminErr)
		}
		adminSrv = &http.Server{
			Handler:           rt.AdminHandler,
			ReadHeaderTimeout: 10 * time.Second,
		}
		rt.Logger.Info("Admin REST API listening", "addr", cfg.AdminAddr)
		go func() {
			if serveErr := adminSrv.Serve(adminLn); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
				errCh <- fmt.Errorf("Admin REST server failed: %w", serveErr)
			}
		}()
	}

	httpsAddr := net.JoinHostPort(cfg.BindHost, fmt.Sprintf("%d", cfg.HTTPSPort))
	rawHTTPSLn, err := net.Listen("tcp", httpsAddr)
	if err != nil {
		if adminSrv != nil {
			_ = adminSrv.Close()
		}
		shutdownHTTP()
		return fmt.Errorf("failed to bind HTTPS listener on %s: %w", httpsAddr, err)
	}
	httpsSrv := &http.Server{
		Handler:           rt.HTTPSHandler,
		TLSConfig:         rt.CertManager.TLSConfig(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	renewCtx, stopRenew := context.WithCancel(ctx)
	defer stopRenew()
	go rt.renewalLoop(renewCtx, cfg.TLS.RenewInterval, cfg.TLS.RenewWithinDays)

	go func() {
		if serveErr := httpsSrv.ServeTLS(rawHTTPSLn, "", ""); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- fmt.Errorf("HTTPS server failed: %w", serveErr)
		}
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpsSrv.Shutdown(shutCtx)
		if adminSrv != nil {
			_ = adminSrv.Shutdown(shutCtx)
		}
		_ = httpSrv.Shutdown(shutCtx)
		return nil
	case fatalErr := <-errCh:
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpsSrv.Shutdown(shutCtx)
		if adminSrv != nil {
			_ = adminSrv.Shutdown(shutCtx)
		}
		_ = httpSrv.Shutdown(shutCtx)
		return fatalErr
	}
}

func (rt *Runtime) renewalLoop(ctx context.Context, intervalSeconds, withinDays int) {
	if intervalSeconds <= 0 {
		intervalSeconds = 12 * 60 * 60
	}
	ticker := time.NewTicker(time.Duration(intervalSeconds) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			updated, err := rt.CertManager.RenewIfNeeded(ctx, withinDays)
			if err != nil {
				rt.Logger.Error("certificate renewal failed", "error", err)
				continue
			}
			if updated != nil {
				rt.Logger.Info("reloaded TLS certificate", "source", updated.Source, "file", updated.CertPath)
			}
		}
	}
}
