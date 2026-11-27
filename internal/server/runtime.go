package server

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gymbag/internal/config"
	"gymbag/internal/host"
	"gymbag/internal/intel"
	"gymbag/internal/localpki"
	"gymbag/internal/mcserver"
	"gymbag/internal/proxy"
	"gymbag/internal/store"
	"gymbag/internal/tcpguard"
	"gymbag/internal/tlsmanager"
	"gymbag/internal/waf"
)

// RuntimeOptions allows dependency injection of custom transports, stores, intelligence managers,
// remote rules managers, ACME clients, hosts file paths, managed Minecraft options, and loggers.
type RuntimeOptions struct {
	Store                store.Store
	Intel                *intel.Manager
	IntelHTTP            *http.Client
	RemoteRules          *waf.RemoteRuleManager
	RemoteRulesHTTP      *http.Client
	LocalWatchInterval   time.Duration
	HTTPTransport        http.RoundTripper
	AuthHTTPClient       *http.Client
	WSDialer             proxy.DialContextFunc
	ACMEHTTP             *http.Client
	HostsFilePath        string
	DisablePublicIPProbe bool
	ServerIPs            []string
	MCServerOptions      *mcserver.Options
	ConsoleIn            io.Reader
	ConsoleOut           io.Writer
	ConsoleErr           io.Writer
	Logger               *slog.Logger
}

// Runtime is the composition root wiring configuration, WAF, IP intelligence, remote rules,
// reverse proxy, SSH/Minecraft protection proxies, managed Minecraft server, verify portal, TLS,
// local PKI hosts management, and HTTP handlers.
type Runtime struct {
	mu                  sync.RWMutex
	cfg                 config.Config
	acmeHTTP            *http.Client
	providerChain       *dynamicProviderChain
	verifyProviderChain *dynamicProviderChain
	detectedPublicIP    string
	localIPs            []string
	fallbackGateStore   store.Store

	Logger            *slog.Logger
	Store             store.Store
	Intel             *intel.Manager
	RemoteRules       *waf.RemoteRuleManager
	Clearance         *waf.ClearanceTokens
	Challenges        *waf.ChallengeService
	Pages             *waf.Renderer
	Router            *proxy.SiteRouter
	HTTPProxy         *proxy.HTTPProxy
	WSProxy           *proxy.WebSocketProxy
	ACMEBoard         *tlsmanager.HTTP01Board
	ACMEClient        *tlsmanager.ACMEClient
	CertManager       *tlsmanager.Manager
	VerifyCertManager *tlsmanager.Manager
	VerifyPortal      *tcpguard.VerifyPortal
	SSHProxy          *tcpguard.SSHProxy
	MinecraftProxy    *tcpguard.MinecraftProxy
	MinecraftServer   *mcserver.Manager
	Hosts             *localpki.HostsManager
	HSTS              atomic.Bool
	directHTTP        atomic.Bool

	HTTPSHandler      http.Handler
	HTTPDirectHandler http.Handler
	HTTPHandler       http.Handler
	RedirectHandler   http.Handler
	AdminHandler      http.Handler
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

func (d *dynamicProviderChain) len() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.providers)
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
			VPNDetectionEnabled:   cfg.Intel.VPNDetection,
			RefreshInterval:       time.Duration(cfg.Intel.RefreshInterval) * time.Second,
			ExtraThreatFeeds:      cfg.Intel.ThreatFeeds,
			ExtraAIFeeds:          cfg.Intel.AIIPFeeds,
			ExtraVPNFeeds:         cfg.Intel.VPNFeeds,
			HTTPClient:            opts.IntelHTTP,
			Logger:                logger,
		})
	}

	remoteMgr := opts.RemoteRules
	localWatch := opts.LocalWatchInterval
	if localWatch <= 0 {
		localWatch = time.Duration(cfg.RulesWatchInterval) * time.Second
	}
	if remoteMgr == nil {
		remoteMgr = waf.NewRemoteRuleManager(waf.RemoteRuleManagerOptions{
			CacheDir:           filepath.Join(cfg.TLS.CertDir, "rules-cache"),
			BaseDir:            cfg.BaseDir,
			RefreshInterval:    time.Duration(cfg.RemoteRulesRefresh) * time.Second,
			LocalWatchInterval: localWatch,
			URLs:               cfg.RemoteRuleURLs(),
			RuleSources:        cfg.AllRuleSources(),
			HTTPClient:         opts.RemoteRulesHTTP,
			Logger:             logger,
		})
	} else {
		if opts.LocalWatchInterval > 0 {
			remoteMgr.SetLocalWatchInterval(opts.LocalWatchInterval)
		}
		remoteMgr.SetRuleSources(cfg.BaseDir, cfg.AllRuleSources())
		remoteMgr.SetURLs(cfg.RemoteRuleURLs())
	}

	clearance := waf.NewClearanceTokens(secret, cfg.Defaults.ClearanceTTL)
	challenges := waf.NewChallengeService(st, cfg.Defaults.Difficulty, cfg.Defaults.ChallengeTTL)
	pages := waf.NewRenderer("")
	router := proxy.NewSiteRouterWithProviders(cfg.Sites, st, clearance, intelMgr, cfg.Intel.IPSecurityAction, remoteMgr)
	httpProxy := proxy.NewHTTPProxy(cfg.HTTPSPort, opts.HTTPTransport, logger)
	httpProxy.SetHTTPPort(cfg.HTTPPort)
	wsProxy := proxy.NewWebSocketProxy(opts.WSDialer, logger)

	board := tlsmanager.NewHTTP01Board()
	acmeClient, err := tlsmanager.BuildACMEClientFromConfig(cfg, opts.ACMEHTTP)
	if err != nil {
		_ = st.Close()
		return nil, err
	}
	var initSiteProviders []tlsmanager.Provider
	if cfg.HTTPSEnabled() {
		initSiteProviders = tlsmanager.BuildProviders(cfg, board, acmeClient)
	}
	providerChain := newDynamicProviderChain(initSiteProviders)
	certMgr := tlsmanager.NewManager([]tlsmanager.Provider{providerChain})
	if cfg.HTTPSEnabled() {
		if err := certMgr.RegisterSiteCertificatesWithOptions(cfg.Sites, cfg.TLS.AcmeShPath, cfg.BaseDir, effectiveKeyPassword(cfg)); err != nil {
			_ = st.Close()
			return nil, err
		}
	}

	hostsMgr := localpki.NewHostsManager(opts.HostsFilePath, "", logger)
	trustedNets := ParseTrustedProxies(cfg.TrustedProxies)

	var (
		detectedPublicIP    string
		localIPs            []string
		fallbackGateStore   store.Store
		verifyProviderChain *dynamicProviderChain
		verifyCertMgr       *tlsmanager.Manager
		verifyPortal        *tcpguard.VerifyPortal
		sshProxy            *tcpguard.SSHProxy
		mcProxy             *tcpguard.MinecraftProxy
		mcServer            *mcserver.Manager
	)
	if cfg.NeedsVerifyServer() {
		gateStore, ok := st.(store.GateStore)
		if !ok {
			memFallback := store.NewMemoryStore()
			gateStore = memFallback
			fallbackGateStore = memFallback
		}
		if len(opts.ServerIPs) > 0 {
			for _, candidate := range opts.ServerIPs {
				trimmed := strings.TrimSpace(candidate)
				if trimmed == "" {
					continue
				}
				localIPs = append(localIPs, trimmed)
				if detectedPublicIP == "" {
					if addr, pErr := netip.ParseAddr(trimmed); pErr == nil {
						addr = addr.Unmap()
						if !addr.IsLoopback() && !addr.IsPrivate() && !addr.IsLinkLocalUnicast() && !addr.IsUnspecified() {
							detectedPublicIP = addr.String()
						}
					}
				}
			}
			if detectedPublicIP == "" && len(localIPs) > 0 {
				detectedPublicIP = localIPs[0]
			}
		} else {
			var probeClient *http.Client
			if !opts.DisablePublicIPProbe {
				if opts.ACMEHTTP != nil {
					probeClient = opts.ACMEHTTP
				} else {
					probeClient = &http.Client{Timeout: 2 * time.Second}
				}
			}
			detectedPublicIP, localIPs = tcpguard.DetectServerIPs(ctx, probeClient)
		}

		verifyTargets := tcpguard.VerifyCertTargets(cfg.Verify, detectedPublicIP, localIPs)
		verifyProviderChain = newDynamicProviderChain(tlsmanager.BuildVerifyProviders(cfg, verifyTargets, board, acmeClient))
		verifyCertMgr = tlsmanager.NewManager([]tlsmanager.Provider{verifyProviderChain})
		verifyPortal = tcpguard.NewVerifyPortal(gateStore, challenges, board, trustedNets)

		if cfg.SSHEnabled() {
			sshProxy = tcpguard.NewSSHProxy(*cfg.SSH, cfg.Verify, cfg.Defaults.Rules, cfg.Intel.IPSecurityAction, st, gateStore, intelMgr, detectedPublicIP, logger)
		}
		if cfg.MinecraftEnabled() {
			mcProxy = tcpguard.NewMinecraftProxy(*cfg.Minecraft, cfg.Verify, cfg.Defaults.Rules, cfg.Intel.IPSecurityAction, st, gateStore, intelMgr, detectedPublicIP, logger)
			verifyPortal.SetOnCleared(func(service, clientIP string) {
				if service == "minecraft" && mcProxy != nil {
					mcProxy.NotifyCleared(clientIP)
				}
			})
			if cfg.Minecraft.ServerJar != "" {
				mcOpts := mcserver.Options{
					BaseDir:    cfg.BaseDir,
					Config:     *cfg.Minecraft,
					Logger:     logger,
					ConsoleIn:  opts.ConsoleIn,
					ConsoleOut: opts.ConsoleOut,
					ConsoleErr: opts.ConsoleErr,
				}
				if opts.MCServerOptions != nil {
					mcOpts.Discover = opts.MCServerOptions.Discover
					mcOpts.CommandBuilder = opts.MCServerOptions.CommandBuilder
					mcOpts.RestartBaseDelay = opts.MCServerOptions.RestartBaseDelay
					if opts.MCServerOptions.ConsoleIn != nil {
						mcOpts.ConsoleIn = opts.MCServerOptions.ConsoleIn
					}
					if opts.MCServerOptions.ConsoleOut != nil {
						mcOpts.ConsoleOut = opts.MCServerOptions.ConsoleOut
					}
					if opts.MCServerOptions.ConsoleErr != nil {
						mcOpts.ConsoleErr = opts.MCServerOptions.ConsoleErr
					}
					if opts.MCServerOptions.ConsoleBufferSize > 0 {
						mcOpts.ConsoleBufferSize = opts.MCServerOptions.ConsoleBufferSize
					}
				}
				mcServer = mcserver.NewManager(mcOpts)
			}
		}
	}

	rt := &Runtime{
		cfg:                 cfg,
		acmeHTTP:            opts.ACMEHTTP,
		providerChain:       providerChain,
		verifyProviderChain: verifyProviderChain,
		detectedPublicIP:    detectedPublicIP,
		localIPs:            localIPs,
		fallbackGateStore:   fallbackGateStore,
		Logger:              logger,
		Store:               st,
		Intel:               intelMgr,
		RemoteRules:         remoteMgr,
		Clearance:           clearance,
		Challenges:          challenges,
		Pages:               pages,
		Router:              router,
		HTTPProxy:           httpProxy,
		WSProxy:             wsProxy,
		ACMEBoard:           board,
		ACMEClient:          acmeClient,
		CertManager:         certMgr,
		VerifyCertManager:   verifyCertMgr,
		VerifyPortal:        verifyPortal,
		SSHProxy:            sshProxy,
		MinecraftProxy:      mcProxy,
		MinecraftServer:     mcServer,
		Hosts:               hostsMgr,
	}
	if cfg.HTTPSEnabled() && cfg.TLS.HSTS != nil {
		rt.HSTS.Store(*cfg.TLS.HSTS)
	}
	rt.directHTTP.Store(cfg.DirectHTTPEnabled())
	remoteMgr.SetOnUpdate(rt.reloadRouter)

	gateway := NewGatewayHandler(challenges, clearance, httpProxy, wsProxy, board, trustedNets)
	authClient := opts.AuthHTTPClient
	if authClient == nil && opts.HTTPTransport != nil {
		authClient = &http.Client{Transport: opts.HTTPTransport}
	}
	gateway.SetAuthenticator(NewForwardAuthenticator(authClient, logger))
	// Fused high-throughput HTTPS handler: avoids per-request context cloning, duplicate IP resolution,
	// and duplicate WebSocket upgrade checks while preserving identical security semantics.
	rt.HTTPSHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tcpguard.IsGatePath(r.URL.Path) && rt.VerifyPortal != nil && rt.verifySharesHTTPS() {
			rt.VerifyPortal.ServeHTTP(w, r)
			return
		}

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
		if rt.HSTS.Load() && !isWS && RequestScheme(r) == "https" {
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
	rt.HTTPDirectHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "" {
			p = "/"
		}
		if token, ok := strings.CutPrefix(p, "/.well-known/acme-challenge/"); ok {
			if token != "" && !strings.Contains(token, "/") && board != nil {
				if _, found := board.Lookup(token); found {
					w.Header()["Server"] = serverHeaderValue
					WriteACMEChallenge(w, r, board, token)
					return
				}
			}
		}
		if len(router.Sites()) == 0 {
			w.Header()["Server"] = serverHeaderValue
			http.NotFound(w, r)
			return
		}
		rt.HTTPSHandler.ServeHTTP(w, r)
	})
	rt.HTTPHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rt.directHTTP.Load() {
			rt.HTTPDirectHandler.ServeHTTP(w, r)
			return
		}
		rt.RedirectHandler.ServeHTTP(w, r)
	})
	rt.AdminHandler = NewAdminHandler(rt)
	return rt, nil
}

var (
	serverHeaderValue = []string{"gymbag"}
	hstsHeaderValue   = []string{"max-age=31536000"}
	invalidHostBytes  = []byte("Invalid host header")
	wsClearanceBytes  = []byte("WebSocket clearance required")
)

func containsString(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

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

func (rt *Runtime) verifySharesHTTPS() bool {
	rt.mu.RLock()
	cfg := rt.cfg
	rt.mu.RUnlock()
	return cfg.HTTPSEnabled() && cfg.NeedsVerifyServer() && cfg.Verify.EffectivePort() == cfg.HTTPSPort
}

func (rt *Runtime) httpsTLSConfig() *tls.Config {
	if !rt.verifySharesHTTPS() || rt.VerifyCertManager == nil {
		return rt.CertManager.TLSConfig()
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{"h2", "http/1.1"},
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			if hello == nil || hello.ServerName == "" {
				if cert, err := rt.VerifyCertManager.GetCertificate(hello); err == nil {
					return cert, nil
				}
				return rt.CertManager.GetCertificate(hello)
			}
			if _, isSite := rt.Router.Resolve(host.Normalize(hello.ServerName)); isSite {
				return rt.CertManager.GetCertificate(hello)
			}
			if cert, err := rt.VerifyCertManager.GetCertificate(hello); err == nil {
				return cert, nil
			}
			return rt.CertManager.GetCertificate(hello)
		},
	}
}

// ReplaceConfig validates next and atomically reloads site routing, custom rules, TLS providers,
// per-site SNI certificates, and SSH/Minecraft protection policies.
func (rt *Runtime) ReplaceConfig(next config.Config) error {
	if err := next.Validate(); err != nil {
		return err
	}
	acmeClient, err := tlsmanager.BuildACMEClientFromConfig(next, rt.acmeHTTP)
	if err != nil {
		return err
	}
	if next.HTTPSEnabled() {
		if err := rt.CertManager.RegisterSiteCertificatesWithOptions(next.Sites, next.TLS.AcmeShPath, next.BaseDir, effectiveKeyPassword(next)); err != nil {
			return err
		}
		if rt.providerChain != nil {
			rt.providerChain.setProviders(tlsmanager.BuildProviders(next, rt.ACMEBoard, acmeClient))
		}
	} else {
		if err := rt.CertManager.RegisterSiteCertificatesWithPassword(nil, ""); err != nil {
			return err
		}
		if rt.providerChain != nil {
			rt.providerChain.setProviders(nil)
		}
	}
	if rt.verifyProviderChain != nil && next.NeedsVerifyServer() {
		targets := tcpguard.VerifyCertTargets(next.Verify, rt.detectedPublicIP, rt.localIPs)
		rt.verifyProviderChain.setProviders(tlsmanager.BuildVerifyProviders(next, targets, rt.ACMEBoard, acmeClient))
	}
	var addedURLs []string
	if rt.RemoteRules != nil {
		rt.RemoteRules.SetRefreshInterval(time.Duration(next.RemoteRulesRefresh) * time.Second)
		if next.RulesWatchInterval > 0 {
			rt.RemoteRules.SetLocalWatchInterval(time.Duration(next.RulesWatchInterval) * time.Second)
		}
		addedURLs = rt.RemoteRules.SetRuleSources(next.BaseDir, next.AllRuleSources())
		for _, u := range rt.RemoteRules.SetURLs(next.RemoteRuleURLs()) {
			if !containsString(addedURLs, u) {
				addedURLs = append(addedURLs, u)
			}
		}
	}
	rt.mu.Lock()
	rt.cfg = next
	rt.ACMEClient = acmeClient
	rt.mu.Unlock()

	rt.Router.ReloadWithProviders(next.Sites, rt.Store, rt.Clearance, rt.Intel, next.Intel.IPSecurityAction, rt.RemoteRules)
	if rt.SSHProxy != nil && next.SSHEnabled() {
		rt.SSHProxy.UpdateConfig(*next.SSH, next.Verify, next.Defaults.Rules, next.Intel.IPSecurityAction)
	}
	if rt.MinecraftProxy != nil && next.MinecraftEnabled() {
		rt.MinecraftProxy.UpdateConfig(*next.Minecraft, next.Verify, next.Defaults.Rules, next.Intel.IPSecurityAction)
	}
	if rt.MinecraftServer != nil && next.Minecraft != nil {
		rt.MinecraftServer.SetConsoleEnabled(next.Minecraft.Console)
	}
	rt.directHTTP.Store(next.DirectHTTPEnabled())
	if !next.HTTPSEnabled() {
		rt.HSTS.Store(false)
	} else if next.TLS.HSTS != nil {
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

func (rt *Runtime) activeTLSMaterial() *tlsmanager.Material {
	if cur := rt.CertManager.Current(); cur != nil {
		return cur
	}
	if rt.VerifyCertManager != nil {
		return rt.VerifyCertManager.Current()
	}
	return nil
}

// TLSSource returns the active certificate provider source name ("file", "acme", "local", "selfsigned", or "none").
func (rt *Runtime) TLSSource() string {
	if cur := rt.activeTLSMaterial(); cur != nil {
		return cur.Source
	}
	return "none"
}

// TLSStatus returns active TLS certificate metadata, configured/effective SSL provider, HSTS state,
// and the full catalog of supported ACME SSL providers.
func (rt *Runtime) TLSStatus() TLSStatus {
	snap := rt.Snapshot()
	if snap.TLSDisabled() {
		return TLSStatus{
			Mode:               "off",
			Source:             "disabled",
			Provider:           "off",
			ConfiguredProvider: snap.TLS.Provider,
			AutoSSL:            false,
			DirectHTTP:         snap.DirectHTTPEnabled(),
			HSTS:               false,
			Providers:          tlsmanager.ListSSLProviders(),
		}
	}
	cur := rt.activeTLSMaterial()

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
			if snap.TLS.AcmeShPath != "" {
				provider = "acme.sh"
			} else {
				provider = "file"
			}
		case "acme":
			provider = "letsencrypt"
		default:
			if snap.TLS.AcmeShPath != "" {
				provider = "acme.sh"
			} else if localpki.AllLocalDomains(snap.Domains()) {
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
		AutoSSL:            snap.TLS.AutoSSLEnabled(),
		DirectHTTP:         snap.DirectHTTPEnabled(),
		HSTS:               rt.HSTS.Load(),
		Providers:          tlsmanager.ListSSLProviders(),
	}
}

// RenewTLS triggers an immediate forced certificate issuance/renewal and refreshes HSTS state.
func (rt *Runtime) RenewTLS(ctx context.Context) (TLSStatus, error) {
	snap := rt.Snapshot()
	var mat *tlsmanager.Material
	var err error
	if snap.HTTPSEnabled() {
		mat, err = rt.CertManager.ForceRenew(ctx)
		if err != nil {
			return TLSStatus{}, err
		}
	}
	if snap.NeedsVerifyServer() && rt.VerifyCertManager != nil && (rt.verifyProviderChain == nil || rt.verifyProviderChain.len() > 0) {
		vMat, vErr := rt.VerifyCertManager.ForceRenew(ctx)
		if vErr != nil {
			return TLSStatus{}, vErr
		}
		if mat == nil {
			mat = vMat
		}
	}
	if mat != nil && snap.HTTPSEnabled() {
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
	if !snap.HTTPEnabled() || !snap.TLS.AutoHostsEnabled() {
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

// MinecraftServerStatus returns live managed Minecraft server subprocess status, or nil when unused.
func (rt *Runtime) MinecraftServerStatus() *mcserver.Status {
	if rt.MinecraftServer == nil {
		return nil
	}
	st := rt.MinecraftServer.Status()
	return &st
}

// MinecraftConsoleEnabled reports whether Minecraft server console integration is active.
func (rt *Runtime) MinecraftConsoleEnabled() bool {
	if rt.MinecraftServer == nil {
		return false
	}
	return rt.MinecraftServer.ConsoleEnabled()
}

// MinecraftConsoleLines returns buffered console lines with Seq > sinceSeq.
func (rt *Runtime) MinecraftConsoleLines(sinceSeq uint64, limit int) []mcserver.ConsoleLine {
	if rt.MinecraftServer == nil {
		return nil
	}
	return rt.MinecraftServer.ConsoleLines(sinceSeq, limit)
}

// MinecraftConsoleSubscribe subscribes to the managed Minecraft server console stream.
func (rt *Runtime) MinecraftConsoleSubscribe() ([]mcserver.ConsoleLine, <-chan mcserver.ConsoleLine, func()) {
	if rt.MinecraftServer == nil {
		ch := make(chan mcserver.ConsoleLine)
		close(ch)
		return nil, ch, func() {}
	}
	return rt.MinecraftServer.SubscribeConsole()
}

// MinecraftSendCommand writes a command line to the running Minecraft server's console.
func (rt *Runtime) MinecraftSendCommand(cmd string) error {
	if rt.MinecraftServer == nil {
		return mcserver.ErrNotRunning
	}
	return rt.MinecraftServer.SendCommand(cmd)
}

// MinecraftStartServer relaunches the managed Minecraft server after a manual stop.
func (rt *Runtime) MinecraftStartServer() error {
	if rt.MinecraftServer == nil {
		return errors.New("no managed Minecraft server is configured")
	}
	return rt.MinecraftServer.RequestStart()
}

// Close stops any managed Minecraft server, removes managed hosts entries, and releases state store resources.
func (rt *Runtime) Close() error {
	var mcErr error
	if rt.MinecraftServer != nil {
		mcErr = rt.MinecraftServer.Stop(5 * time.Second)
	}
	hostsErr := rt.CleanupHosts()
	if errors.Is(hostsErr, localpki.ErrPermissionDenied) {
		hostsErr = nil
	}
	storeErr := rt.Store.Close()
	var fallbackErr error
	if rt.fallbackGateStore != nil {
		fallbackErr = rt.fallbackGateStore.Close()
	}
	return errors.Join(mcErr, hostsErr, storeErr, fallbackErr)
}

// Serve starts automatic /etc/hosts setup (when enabled), background IP/AI/VPN feed auto-updates,
// the HTTP (:80) redirect/ACME server, obtains TLS certificates, starts the HTTPS (:443) gateway,
// standalone Verify portal HTTPS (:8443), SSH protection proxy (:22), Minecraft protection proxy (:25565),
// and optional Admin REST server, blocking until ctx is cancelled or a fatal server error occurs.
func (rt *Runtime) Serve(ctx context.Context) error {
	defer rt.Close()
	serveCtx, cancelServe := context.WithCancel(ctx)
	defer cancelServe()

	cfg := rt.Snapshot()

	if cfg.HTTPEnabled() && cfg.TLS.AutoHostsEnabled() {
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

	// Start automatic background updates for IP security feeds, VPN/proxy lists, AI robot IPs, and remote firewall rules.
	rt.Intel.Start(serveCtx)
	if rt.RemoteRules != nil {
		rt.RemoteRules.Start(serveCtx)
	}

	rt.Logger.Info(
		"starting Gymbag gateway",
		"http_enabled", cfg.HTTPEnabled(),
		"https_enabled", cfg.HTTPSEnabled(),
		"direct_http", cfg.DirectHTTPEnabled(),
		"auto_ssl", cfg.TLS.AutoSSLEnabled(),
		"acme_sh_path", cfg.TLS.AcmeShPath,
		"http_port", cfg.HTTPPort,
		"https_port", cfg.HTTPSPort,
		"domains", strings.Join(cfg.Domains(), ", "),
		"ssh_enabled", cfg.SSHEnabled(),
		"minecraft_enabled", cfg.MinecraftEnabled(),
		"verify_port", cfg.Verify.EffectivePort(),
		"ip_security", cfg.Intel.IPSecurity,
		"vpn_detection", cfg.Intel.VPNDetection,
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
	if cfg.SSHEnabled() {
		rt.Logger.Info(
			"configured SSH protection",
			"port", cfg.SSH.Port,
			"upstream", cfg.SSH.Upstream,
			"protected", cfg.SSH.Protect,
			"verify_level", int(cfg.SSH.VerifyLevel),
		)
	}
	if cfg.MinecraftEnabled() {
		rt.Logger.Info(
			"configured Minecraft protection",
			"port", cfg.Minecraft.Port,
			"upstream", cfg.Minecraft.Upstream,
			"server_jar", cfg.Minecraft.ServerJar,
			"console", cfg.Minecraft.Console,
			"protected", cfg.Minecraft.Protect,
			"verify_level", int(cfg.Minecraft.VerifyLevel),
		)
		if cfg.Minecraft.Console && cfg.Minecraft.ServerJar == "" {
			rt.Logger.Warn("minecraft.console is enabled without minecraft.server_jar; set minecraft.server_jar so Gymbag manages the Minecraft server process and attaches its console")
		}
	}
	if cfg.NeedsVerifyServer() {
		rt.Logger.Info(
			"configured verification portal",
			"verify_url", cfg.Verify.URL,
			"verify_port", cfg.Verify.EffectivePort(),
			"shares_https_port", rt.verifySharesHTTPS(),
		)
	}

	needsPriv := (cfg.HTTPEnabled() && (cfg.HTTPPort < 1024 || (cfg.HTTPSEnabled() && cfg.HTTPSPort < 1024))) ||
		(cfg.SSHEnabled() && cfg.SSH.Port < 1024) ||
		(cfg.MinecraftEnabled() && cfg.Minecraft.Port < 1024) ||
		(cfg.NeedsVerifyServer() && cfg.Verify.EffectivePort() < 1024)
	if needsPriv && os.Geteuid() != 0 {
		rt.Logger.Warn(
			"ports below 1024 usually need administrator rights",
		)
	}

	var (
		httpSrv   *http.Server
		httpsSrv  *http.Server
		verifySrv *http.Server
		adminSrv  *http.Server
		sshLn     net.Listener
		mcLn      net.Listener
	)
	shutdownAll := func(timeout time.Duration) {
		cancelServe()
		if sshLn != nil {
			_ = sshLn.Close()
		}
		if mcLn != nil {
			_ = mcLn.Close()
		}
		if rt.MinecraftServer != nil {
			_ = rt.MinecraftServer.Stop(timeout)
		}
		shutCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if httpsSrv != nil {
			_ = httpsSrv.Shutdown(shutCtx)
		}
		if verifySrv != nil {
			_ = verifySrv.Shutdown(shutCtx)
		}
		if adminSrv != nil {
			_ = adminSrv.Shutdown(shutCtx)
		}
		if httpSrv != nil {
			_ = httpSrv.Shutdown(shutCtx)
		}
	}

	errCh := make(chan error, 8)

	needHTTPListener := cfg.HTTPEnabled() ||
		(cfg.NeedsVerifyServer() && !cfg.TLSDisabled() && cfg.TLS.Mode != "local" && cfg.TLS.Mode != "selfsigned" && cfg.TLS.Mode != "file" && cfg.TLS.AcmeShPath == "")
	if needHTTPListener && cfg.HTTPPort > 0 {
		httpAddr := net.JoinHostPort(cfg.BindHost, strconv.Itoa(cfg.HTTPPort))
		httpLn, lErr := net.Listen("tcp", httpAddr)
		if lErr != nil {
			if cfg.HTTPEnabled() || cfg.TLS.Mode == "acme" {
				return fmt.Errorf("failed to bind HTTP listener on %s: %w", httpAddr, lErr)
			}
			rt.Logger.Warn(
				"skipping HTTP ACME challenge listener; falling back to local/self-signed IP certificate for verify portal",
				"addr", httpAddr,
				"error", lErr,
			)
		} else {
			httpSrv = &http.Server{
				Handler:           rt.HTTPHandler,
				ReadHeaderTimeout: 10 * time.Second,
				IdleTimeout:       120 * time.Second,
			}
			go func() {
				if serveErr := httpSrv.Serve(httpLn); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
					errCh <- fmt.Errorf("HTTP server failed: %w", serveErr)
				}
			}()
		}
	}

	if cfg.HTTPSEnabled() {
		material, err := rt.CertManager.Obtain(serveCtx)
		if err != nil {
			shutdownAll(5 * time.Second)
			return err
		}
		rt.HSTS.Store(tlsmanager.HSTSEnabled(cfg.TLS.HSTS, material.Source))
		rt.Logger.Info("TLS certificate ready", "source", material.Source, "file", material.CertPath)
	} else if cfg.HTTPEnabled() {
		rt.Logger.Info("HTTPS disabled; serving websites directly over HTTP", "http_port", cfg.HTTPPort, "auto_ssl", cfg.TLS.AutoSSLEnabled())
	}

	verifyHasTLS := cfg.NeedsVerifyServer() && rt.VerifyCertManager != nil && (rt.verifyProviderChain == nil || rt.verifyProviderChain.len() > 0)
	if verifyHasTLS {
		verifyMat, err := rt.VerifyCertManager.Obtain(serveCtx)
		if err != nil {
			shutdownAll(5 * time.Second)
			return err
		}
		if !cfg.HTTPSEnabled() {
			rt.HSTS.Store(tlsmanager.HSTSEnabled(cfg.TLS.HSTS, verifyMat.Source))
		}
		rt.Logger.Info("verify portal TLS certificate ready", "source", verifyMat.Source, "file", verifyMat.CertPath)
	}

	if cfg.AdminAddr != "" {
		adminLn, adminErr := net.Listen("tcp", cfg.AdminAddr)
		if adminErr != nil {
			shutdownAll(5 * time.Second)
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

	if cfg.HTTPSEnabled() {
		httpsAddr := net.JoinHostPort(cfg.BindHost, strconv.Itoa(cfg.HTTPSPort))
		rawHTTPSLn, err := net.Listen("tcp", httpsAddr)
		if err != nil {
			shutdownAll(5 * time.Second)
			return fmt.Errorf("failed to bind HTTPS listener on %s: %w", httpsAddr, err)
		}
		httpsSrv = &http.Server{
			Handler:           rt.HTTPSHandler,
			TLSConfig:         rt.httpsTLSConfig(),
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
		go func() {
			if serveErr := httpsSrv.ServeTLS(rawHTTPSLn, "", ""); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
				errCh <- fmt.Errorf("HTTPS server failed: %w", serveErr)
			}
		}()
	}

	if cfg.NeedsVerifyServer() && rt.VerifyPortal != nil && !rt.verifySharesHTTPS() {
		verifyAddr := net.JoinHostPort(cfg.BindHost, strconv.Itoa(cfg.Verify.EffectivePort()))
		rawVerifyLn, err := net.Listen("tcp", verifyAddr)
		if err != nil {
			shutdownAll(5 * time.Second)
			return fmt.Errorf("failed to bind Verify portal listener on %s: %w", verifyAddr, err)
		}
		if verifyHasTLS {
			verifySrv = &http.Server{
				Handler:           rt.VerifyPortal,
				TLSConfig:         rt.VerifyCertManager.TLSConfig(),
				ReadHeaderTimeout: 10 * time.Second,
				IdleTimeout:       120 * time.Second,
			}
			rt.Logger.Info("verify portal HTTPS listening", "addr", verifyAddr)
			go func() {
				if serveErr := verifySrv.ServeTLS(rawVerifyLn, "", ""); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
					errCh <- fmt.Errorf("Verify HTTPS server failed: %w", serveErr)
				}
			}()
		} else {
			verifySrv = &http.Server{
				Handler:           rt.VerifyPortal,
				ReadHeaderTimeout: 10 * time.Second,
				IdleTimeout:       120 * time.Second,
			}
			rt.Logger.Info("verify portal HTTP listening", "addr", verifyAddr)
			go func() {
				if serveErr := verifySrv.Serve(rawVerifyLn); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
					errCh <- fmt.Errorf("Verify HTTP server failed: %w", serveErr)
				}
			}()
		}
	}

	if cfg.SSHEnabled() && rt.SSHProxy != nil {
		sshAddr := net.JoinHostPort(cfg.BindHost, strconv.Itoa(cfg.SSH.Port))
		ln, err := net.Listen("tcp", sshAddr)
		if err != nil {
			shutdownAll(5 * time.Second)
			return fmt.Errorf("failed to bind SSH listener on %s: %w", sshAddr, err)
		}
		sshLn = ln
		rt.Logger.Info("SSH protection proxy listening", "addr", sshAddr, "upstream", cfg.SSH.Upstream)
		go func() {
			if serveErr := rt.SSHProxy.Serve(serveCtx, ln); serveErr != nil && serveCtx.Err() == nil && !errors.Is(serveErr, net.ErrClosed) {
				errCh <- fmt.Errorf("SSH proxy failed: %w", serveErr)
			}
		}()
	}

	if cfg.MinecraftEnabled() && rt.MinecraftProxy != nil {
		if rt.MinecraftServer != nil {
			if startErr := rt.MinecraftServer.Start(serveCtx); startErr != nil {
				shutdownAll(5 * time.Second)
				return fmt.Errorf("failed to start Minecraft server: %w", startErr)
			}
		}
		mcAddr := net.JoinHostPort(cfg.BindHost, strconv.Itoa(cfg.Minecraft.Port))
		ln, err := net.Listen("tcp", mcAddr)
		if err != nil {
			shutdownAll(5 * time.Second)
			return fmt.Errorf("failed to bind Minecraft listener on %s: %w", mcAddr, err)
		}
		mcLn = ln
		rt.Logger.Info("Minecraft protection proxy listening", "addr", mcAddr, "upstream", cfg.Minecraft.Upstream)
		go func() {
			if serveErr := rt.MinecraftProxy.Serve(serveCtx, ln); serveErr != nil && serveCtx.Err() == nil && !errors.Is(serveErr, net.ErrClosed) {
				errCh <- fmt.Errorf("Minecraft proxy failed: %w", serveErr)
			}
		}()
	}

	go rt.renewalLoop(serveCtx, cfg.TLS.RenewInterval, cfg.TLS.RenewWithinDays)

	select {
	case <-ctx.Done():
		shutdownAll(10 * time.Second)
		return nil
	case fatalErr := <-errCh:
		shutdownAll(5 * time.Second)
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
			if rt.CertManager != nil && rt.CertManager.Current() != nil {
				updated, err := rt.CertManager.RenewIfNeeded(ctx, withinDays)
				if err != nil {
					rt.Logger.Error("certificate renewal failed", "error", err)
				} else if updated != nil {
					rt.Logger.Info("reloaded TLS certificate", "source", updated.Source, "file", updated.CertPath)
				}
			}
			if rt.VerifyCertManager != nil && rt.VerifyCertManager.Current() != nil {
				updated, err := rt.VerifyCertManager.RenewIfNeeded(ctx, withinDays)
				if err != nil {
					rt.Logger.Error("verify portal certificate renewal failed", "error", err)
				} else if updated != nil {
					rt.Logger.Info("reloaded verify portal TLS certificate", "source", updated.Source, "file", updated.CertPath)
				}
			}
		}
	}
}
