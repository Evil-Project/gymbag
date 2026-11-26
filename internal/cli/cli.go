package cli

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"gymbag/internal/config"
	"gymbag/internal/localpki"
	"gymbag/internal/mcserver"
	"gymbag/internal/server"
)

// Version is the build version of the gymbag binary (overridable via -ldflags).
var Version = "0.2.0"

// Execute parses argv and runs the requested Gymbag command, returning a process exit code.
func Execute(argv []string, stdout, stderr io.Writer) int {
	return ExecuteWithIO(argv, os.Stdin, stdout, stderr)
}

// ExecuteWithIO parses argv and runs the requested Gymbag command with custom I/O streams.
func ExecuteWithIO(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	LoadDotEnv(".env")

	if len(argv) > 0 {
		switch argv[0] {
		case "version", "-v", "--version":
			_, _ = fmt.Fprintf(stdout, "gymbag v%s\n", Version)
			return 0
		case "help", "-h", "--help":
			printUsage(stdout)
			return 0
		case "init", "generate", "gen-config":
			return runInitCmd(argv[1:], stdin, stdout, stderr)
		case "config":
			return runConfigCmd(argv[1:], stdout, stderr)
		case "ruleset", "rulesets", "rules":
			return runRulesetCmd(argv[1:], stdout, stderr)
		case "service":
			return runServiceCmd(argv[1:], stdout, stderr)
		case "validate":
			return runValidate(argv[1:], stdout, stderr)
		case "fmt":
			return runFmt(argv[1:], stdout, stderr)
		case "reverse-proxy":
			return runReverseProxyCmd(argv[1:], stdout, stderr)
		case "console":
			return runConsoleCmd(argv[1:], stdin, stdout, stderr)
		case "run":
			return runServerCmd(argv[1:], stdin, stdout, stderr)
		}
	}
	return runServerCmd(argv, stdin, stdout, stderr)
}

func runServerCmd(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gymbag run", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		cfgFile            string
		domain             string
		upstream           string
		verifyLevel        string
		verifyTimeout      string
		remoteRules        multiStringFlag
		remoteRulesRefresh string
		sslProvider        string
		noProtect          bool
		adminAddr          string
		httpPort           int
		httpsPort          int
		mcConsole          bool
		watch              bool
	)

	fs.StringVar(&cfgFile, "c", "", "Path to a Gymbagfile, Caddyfile, or JSON config file")
	fs.StringVar(&cfgFile, "config", "", "Path to a Gymbagfile, Caddyfile, or JSON config file")
	fs.StringVar(&domain, "d", "", "Downstream domain(s), comma-separated")
	fs.StringVar(&domain, "domain", "", "Downstream domain(s), comma-separated")
	fs.StringVar(&upstream, "u", "", "Upstream URL to reverse proxy to")
	fs.StringVar(&upstream, "upstream", "", "Upstream URL to reverse proxy to")
	fs.StringVar(&verifyLevel, "l", "", "Verify level (0=always, 1=only suspicious, 2=only ai, 3=suspicious & ai)")
	fs.StringVar(&verifyLevel, "verify-level", "", "Verify level (0=always, 1=only suspicious, 2=only ai, 3=suspicious & ai)")
	fs.StringVar(&verifyLevel, "verify", "", "Verify level (0=always, 1=only suspicious, 2=only ai, 3=suspicious & ai)")
	fs.StringVar(&verifyLevel, "m", "", "Alias for --verify")
	fs.StringVar(&verifyLevel, "mode", "", "Alias for --verify")
	fs.StringVar(&verifyTimeout, "verify-timeout", "", "Verified clearance timeout (seconds or duration like 30m, 12h, 7d)")
	fs.StringVar(&verifyTimeout, "clearance-ttl", "", "Alias for --verify-timeout")
	fs.Var(&remoteRules, "remote-rules", "Remote firewall rules URL (http/https, repeatable or comma-separated)")
	fs.StringVar(&remoteRulesRefresh, "remote-rules-refresh", "", "Refresh interval for remote rules (seconds or duration like 5m, 1h)")
	fs.StringVar(&sslProvider, "ssl-provider", "", "SSL/ACME provider (letsencrypt, zerossl, google, ssl.com, litessl, buypass, local, or URL)")
	fs.BoolVar(&noProtect, "no-protect", false, "Disable WAF verification for CLI-configured sites")
	fs.StringVar(&adminAddr, "admin", "", "Local REST Admin API listen address (e.g. 127.0.0.1:2019 or off)")
	fs.IntVar(&httpPort, "http-port", 0, "HTTP redirect/ACME port (default 80)")
	fs.IntVar(&httpsPort, "https-port", 0, "HTTPS gateway port (default 443)")
	fs.BoolVar(&mcConsole, "mc-console", false, "Bring managed Minecraft server console into Gymbag")
	fs.BoolVar(&mcConsole, "minecraft-console", false, "Alias for --mc-console")
	fs.BoolVar(&watch, "watch", false, "Watch config file for changes and hot-reload site policies")

	if err := parseInterspersedFlags(fs, args); err != nil {
		if err == flag.ErrHelp {
			printUsage(stdout)
			return 0
		}
		return 2
	}
	if fs.NArg() > 1 {
		_, _ = fmt.Fprintln(stderr, "error: at most one positional config_file argument is accepted")
		return 2
	}
	if cfgFile == "" && fs.NArg() == 1 {
		cfgFile = fs.Arg(0)
	}

	var protectOpt *bool
	if noProtect {
		f := false
		protectOpt = &f
	}
	var mcConsoleOpt *bool
	if mcConsole {
		t := true
		mcConsoleOpt = &t
	}

	loadOpts := config.LoadOptions{
		ConfigPath:         cfgFile,
		Domain:             domain,
		Upstream:           upstream,
		VerifyLevel:        verifyLevel,
		ClearanceTTL:       verifyTimeout,
		RemoteRules:        remoteRules,
		RemoteRulesRefresh: remoteRulesRefresh,
		Protect:            protectOpt,
		AdminAddr:          adminAddr,
		HTTPPort:           httpPort,
		HTTPSPort:          httpsPort,
		MCConsole:          mcConsoleOpt,
	}

	cfg, err := loadConfigWithCLIFlags(loadOpts, sslProvider)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag: %v\n", err)
		return 1
	}

	return serveWithSignals(cfg, resolveWatchedPath(cfgFile), watch, loadOpts, sslProvider, stdin, stdout, stderr)
}

func runReverseProxyCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gymbag reverse-proxy", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		from               string
		to                 string
		verifyLevel        string
		verifyTimeout      string
		remoteRules        multiStringFlag
		remoteRulesRefresh string
		sslProvider        string
		noProtect          bool
		adminAddr          string
		httpPort           int
		httpsPort          int
	)
	fs.StringVar(&from, "from", "localhost", "Downstream domain(s), comma-separated")
	fs.StringVar(&to, "to", "", "Upstream address or URL (required, e.g. :5173 or http://localhost:3000)")
	fs.StringVar(&verifyLevel, "verify", "0", "Verify level: 0 (always), 1 (only suspicious), 2 (only ai), 3 (suspicious & ai)")
	fs.StringVar(&verifyLevel, "verify-level", "", "Alias for --verify")
	fs.StringVar(&verifyLevel, "l", "", "Alias for --verify")
	fs.StringVar(&verifyLevel, "mode", "", "Alias for --verify")
	fs.StringVar(&verifyTimeout, "verify-timeout", "", "Verified clearance timeout (seconds or duration like 30m, 12h, 7d)")
	fs.StringVar(&verifyTimeout, "clearance-ttl", "", "Alias for --verify-timeout")
	fs.Var(&remoteRules, "remote-rules", "Remote firewall rules URL (http/https, repeatable or comma-separated)")
	fs.StringVar(&remoteRulesRefresh, "remote-rules-refresh", "", "Refresh interval for remote rules (seconds or duration like 5m, 1h)")
	fs.StringVar(&sslProvider, "ssl-provider", "", "SSL/ACME provider (letsencrypt, zerossl, google, ssl.com, litessl, buypass, local, or URL)")
	fs.BoolVar(&noProtect, "no-protect", false, "Disable WAF verification")
	fs.StringVar(&adminAddr, "admin", "", "Local REST Admin API listen address")
	fs.IntVar(&httpPort, "http-port", 0, "HTTP port (default 80)")
	fs.IntVar(&httpsPort, "https-port", 0, "HTTPS port (default 443)")

	if err := parseInterspersedFlags(fs, args); err != nil {
		if err == flag.ErrHelp {
			printUsage(stdout)
			return 0
		}
		return 2
	}
	if strings.TrimSpace(to) == "" {
		_, _ = fmt.Fprintln(stderr, "ERROR gymbag: reverse-proxy requires --to <upstream>")
		return 2
	}
	var protectOpt *bool
	if noProtect {
		f := false
		protectOpt = &f
	}
	loadOpts := config.LoadOptions{
		Domain:             from,
		Upstream:           to,
		VerifyLevel:        verifyLevel,
		ClearanceTTL:       verifyTimeout,
		RemoteRules:        remoteRules,
		RemoteRulesRefresh: remoteRulesRefresh,
		Protect:            protectOpt,
		AdminAddr:          adminAddr,
		HTTPPort:           httpPort,
		HTTPSPort:          httpsPort,
	}
	cfg, err := loadConfigWithCLIFlags(loadOpts, sslProvider)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag: %v\n", err)
		return 1
	}
	return serveWithSignals(cfg, "", false, loadOpts, sslProvider, nil, stdout, stderr)
}

func runValidate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gymbag validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		cfgFile     string
		sslProvider string
	)
	fs.StringVar(&cfgFile, "c", "", "Path to config file")
	fs.StringVar(&cfgFile, "config", "", "Path to config file")
	fs.StringVar(&sslProvider, "ssl-provider", "", "Override SSL/ACME provider")
	if err := parseInterspersedFlags(fs, args); err != nil {
		return 2
	}
	if cfgFile == "" && fs.NArg() > 0 {
		cfgFile = fs.Arg(0)
	}
	cfg, err := loadConfigWithCLIFlags(config.LoadOptions{ConfigPath: cfgFile}, sslProvider)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "invalid configuration: %v\n", err)
		return 1
	}
	for _, src := range cfg.AllRuleSources() {
		if src.IsURL() {
			continue
		}
		if _, _, _, _, err := config.LoadRuleSourceFromDisk(src.Source, cfg.BaseDir); err != nil {
			_, _ = fmt.Fprintf(stderr, "invalid rule source %q: %v\n", src.Source, err)
			return 1
		}
	}
	globalDefRules := "on"
	if !cfg.Defaults.DefaultRulesEnabled() {
		globalDefRules = "off"
	}
	_, _ = fmt.Fprintf(stdout, "Valid configuration (%d site(s), SSH: %t, Minecraft: %t, bind %s, HTTP :%d -> HTTPS :%d, TLS %s, SSL provider: %s, default_rules: %s, IP security: %t [%s], VPN detection: %t, AI IP list: %t)\n",
		len(cfg.Sites), cfg.SSHEnabled(), cfg.MinecraftEnabled(), cfg.BindHost, cfg.HTTPPort, cfg.HTTPSPort, cfg.TLS.Mode,
		effectiveSSLProviderLabel(cfg), globalDefRules,
		cfg.Intel.IPSecurity, cfg.Intel.IPSecurityAction, cfg.Intel.VPNDetection, cfg.Intel.AIIPList)
	for _, s := range cfg.Sites {
		defRulesLabel := "on"
		if !s.WAF.DefaultRulesEnabled() {
			defRulesLabel = "off"
		}
		target := strings.Join(s.Upstreams, ", ")
		if len(s.Upstreams) == 0 {
			target = fmt.Sprintf("%d route(s)", len(s.Routes))
		} else if len(s.Routes) > 0 {
			target += fmt.Sprintf(" (+%d route(s))", len(s.Routes))
		}
		state := fmt.Sprintf("protected (verify: %d [%s], difficulty: %d, clearance_ttl: %ds, default_rules: %s, custom rules: %d, remote rules: %d, rule sources: %d)",
			int(s.WAF.VerifyLevel), s.WAF.VerifyLevel.Label(), s.WAF.Difficulty, s.WAF.ClearanceTTL, defRulesLabel, len(s.WAF.Rules), len(s.WAF.RemoteRules), len(s.WAF.RuleSources))
		if !s.Protect {
			state = fmt.Sprintf("unprotected (WAF verification off, custom rules: %d, remote rules: %d, rule sources: %d)", len(s.WAF.Rules), len(s.WAF.RemoteRules), len(s.WAF.RuleSources))
		}
		if s.Auth != nil {
			state += fmt.Sprintf(", auth: %s (%s)", s.Auth.Provider, s.Auth.URL)
		}
		_, _ = fmt.Fprintf(stdout, "  - %s -> %s [%s]\n", s.Domain, target, state)
	}
	if cfg.SSHEnabled() {
		sshState := fmt.Sprintf("protected (verify: %d [%s], difficulty: %d, clearance_ttl: %ds)",
			int(cfg.SSH.VerifyLevel), cfg.SSH.VerifyLevel.Label(), cfg.SSH.Difficulty, cfg.SSH.ClearanceTTL)
		if !cfg.SSH.Protect {
			sshState = "unprotected"
		}
		_, _ = fmt.Fprintf(stdout, "  - ssh :%d -> %s [%s]\n", cfg.SSH.Port, cfg.SSH.Upstream, sshState)
	}
	if cfg.MinecraftEnabled() {
		mcState := fmt.Sprintf("protected (verify: %d [%s], difficulty: %d, clearance_ttl: %ds)",
			int(cfg.Minecraft.VerifyLevel), cfg.Minecraft.VerifyLevel.Label(), cfg.Minecraft.Difficulty, cfg.Minecraft.ClearanceTTL)
		if !cfg.Minecraft.Protect {
			mcState = "unprotected"
		}
		if cfg.Minecraft.Console {
			mcState += ", console=on"
		}
		_, _ = fmt.Fprintf(stdout, "  - minecraft :%d -> %s [%s]\n", cfg.Minecraft.Port, cfg.Minecraft.Upstream, mcState)
		if cfg.Minecraft.Console && cfg.Minecraft.ServerJar == "" {
			_, _ = fmt.Fprintf(stdout, "    console_note=minecraft.console is on without server_jar (only managed server_jar launches attach a subprocess console)\n")
		}
		if cfg.Minecraft.ServerJar != "" {
			jarResolved := cfg.Minecraft.ServerJar
			if !filepath.IsAbs(jarResolved) && cfg.BaseDir != "" {
				jarResolved = filepath.Join(cfg.BaseDir, jarResolved)
			}
			if sel, jErr := mcserver.ResolveJavaForJar(context.Background(), jarResolved, cfg.Minecraft.JavaPath, mcserver.DiscoverOptions{}); jErr == nil {
				_, _ = fmt.Fprintf(stdout, "    server_jar=%s, java=%d (%s)\n", cfg.Minecraft.ServerJar, sel.Java.Major, sel.Java.Path)
				if sel.Warning != "" {
					_, _ = fmt.Fprintf(stdout, "    java_warning=%s\n", sel.Warning)
				}
			} else {
				_, _ = fmt.Fprintf(stdout, "    server_jar=%s (note: %v)\n", cfg.Minecraft.ServerJar, jErr)
			}
		}
	}
	if cfg.NeedsVerifyServer() {
		vTarget := cfg.Verify.URL
		if vTarget == "" {
			vTarget = fmt.Sprintf("https://<server-ip>:%d (auto IP SSL cert)", cfg.Verify.EffectivePort())
		}
		_, _ = fmt.Fprintf(stdout, "  - verify_portal port=%d url=%s\n", cfg.Verify.EffectivePort(), vTarget)
	}
	return 0
}

func loadConfigWithCLIFlags(opts config.LoadOptions, sslProvider string) (config.Config, error) {
	cfg, err := config.LoadWithEnv(opts, os.Getenv)
	if err != nil {
		return config.Config{}, err
	}
	if trimmed := strings.TrimSpace(sslProvider); trimmed != "" {
		wasCustom := strings.EqualFold(strings.TrimSpace(cfg.TLS.Provider), "custom")
		prov, mode, customURL, err := config.NormalizeSSLProvider(trimmed)
		if err != nil {
			return config.Config{}, err
		}
		cfg.TLS.Provider = prov
		cfg.TLS.Mode = mode
		if customURL != "" {
			cfg.TLS.ACMEDirectory = customURL
		} else if wasCustom {
			cfg.TLS.ACMEDirectory = config.DefaultTLSConfig().ACMEDirectory
		}
		if err := cfg.Validate(); err != nil {
			return config.Config{}, err
		}
	}
	return cfg, nil
}

func effectiveSSLProviderLabel(cfg config.Config) string {
	if prov := strings.TrimSpace(cfg.TLS.Provider); prov != "" {
		return prov
	}
	switch cfg.TLS.Mode {
	case "local":
		return "local"
	case "selfsigned":
		return "selfsigned"
	case "file":
		return "file"
	case "acme":
		return "letsencrypt"
	default:
		if localpki.AllLocalDomains(cfg.Domains()) {
			return "local"
		}
		return "letsencrypt"
	}
}

func runFmt(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gymbag fmt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var overwrite bool
	fs.BoolVar(&overwrite, "w", false, "Overwrite config file in place")
	fs.BoolVar(&overwrite, "overwrite", false, "Overwrite config file in place")
	if err := parseInterspersedFlags(fs, args); err != nil {
		return 2
	}
	path := "Gymbagfile"
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag fmt: %v\n", err)
		return 1
	}
	formatted, err := config.FormatCaddyfile(string(raw))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag fmt: %v\n", err)
		return 1
	}
	if overwrite {
		if !bytes.Equal(raw, []byte(formatted)) {
			if err := os.WriteFile(path, []byte(formatted), 0o644); err != nil {
				_, _ = fmt.Fprintf(stderr, "ERROR gymbag fmt: %v\n", err)
				return 1
			}
		}
		return 0
	}
	_, _ = io.WriteString(stdout, formatted)
	return 0
}

func serveWithSignals(cfg config.Config, watchPath string, watch bool, opts config.LoadOptions, sslProvider string, stdin io.Reader, stdout, stderr io.Writer) int {
	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rt, err := server.BuildRuntime(ctx, cfg, server.RuntimeOptions{
		Logger:     logger,
		ConsoleIn:  stdin,
		ConsoleOut: stdout,
		ConsoleErr: stderr,
	})
	if err != nil {
		logger.Error("startup failed", "error", err)
		return 1
	}

	if watch && watchPath != "" {
		go watchConfigFile(ctx, watchPath, opts, sslProvider, rt, logger)
	}

	if err := rt.Serve(ctx); err != nil {
		logger.Error("server exited with error", "error", err)
		return 1
	}
	return 0
}

func resolveWatchedPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if env := strings.TrimSpace(os.Getenv("GYMBAG_CONFIG")); env != "" {
		return env
	}
	for _, candidate := range config.DefaultConfigFiles {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func watchConfigFile(ctx context.Context, path string, opts config.LoadOptions, sslProvider string, rt *server.Runtime, logger *slog.Logger) {
	lastRaw, _ := os.ReadFile(path)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	opts.ConfigPath = path

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			curRaw, err := os.ReadFile(path)
			if err != nil || bytes.Equal(curRaw, lastRaw) {
				continue
			}
			lastRaw = curRaw
			nextCfg, err := loadConfigWithCLIFlags(opts, sslProvider)
			if err != nil {
				logger.Warn("config reload rejected", "file", path, "error", err)
				continue
			}
			if err := rt.ReplaceConfig(nextCfg); err != nil {
				logger.Warn("config apply failed", "file", path, "error", err)
				continue
			}
			logger.Info("hot-reloaded configuration", "file", path, "sites", len(nextCfg.Sites))
		}
	}
}

// parseInterspersedFlags allows flags before or after positional arguments (GNU/Python argparse style).
func parseInterspersedFlags(fs *flag.FlagSet, args []string) error {
	var flagsPart []string
	var posPart []string

	// Identify boolean flags on fs so we don't consume the next token as their argument.
	boolFlags := make(map[string]bool)
	fs.VisitAll(func(f *flag.Flag) {
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			boolFlags[f.Name] = true
		}
	})

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			posPart = append(posPart, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			flagsPart = append(flagsPart, arg)
			trimmed := strings.TrimLeft(arg, "-")
			if !strings.Contains(trimmed, "=") && !boolFlags[trimmed] && i+1 < len(args) {
				i++
				flagsPart = append(flagsPart, args[i])
			}
		} else {
			posPart = append(posPart, arg)
		}
	}
	return fs.Parse(append(flagsPart, posPart...))
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintf(w, `Gymbag v%s — All-in-One HTTP/HTTPS, SSH & Minecraft PoW Protection Gateway

Usage:
  gymbag [config_file] [flags]
  gymbag init [flags]
  gymbag config <list|show|add|update|remove|site|route|set|unset|convert> [flags]
  gymbag ruleset <list|add|update|remove|validate|init|rule> [flags]
  gymbag service <install|start|stop|restart|status|uninstall|render> [flags]
  gymbag run [--config Gymbagfile|gymbag.json] [--watch] [flags]
  gymbag console [--config gymbag.json] [--admin 127.0.0.1:2019] [--cmd "..."]
  gymbag reverse-proxy --from <domains> --to <upstream> [--verify <0-3>]
  gymbag validate [--config gymbag.json]
  gymbag fmt [--overwrite] [Gymbagfile]
  gymbag version

Quick Start (Two Commands):
  HTTP/HTTPS:
    1. gymbag init -d app.example.com -u http://localhost:3000
    2. gymbag
  SSH Protection (randomizes sshd port + sets firewall):
    1. sudo gymbag init -t ssh
    2. sudo gymbag
  Minecraft Protection (auto-starts server jar with best Java + randomizes port):
    1. gymbag init -t minecraft --server-jar /srv/mc/server.jar
    2. gymbag
  All-in-One (HTTP + SSH + Minecraft in one process):
    1. gymbag init -t http,ssh,minecraft -d app.example.com -u :3000
    2. gymbag

Supported Config Formats:
  - gymbag.json (native concise JSON config, generated by 'gymbag init' by default)
  - Gymbagfile / Caddyfile (including Caddyfile v2 snippets, '@matchers', 'handle', 'route', 'file_server', 'respond', 'redir', 'header')
  - nginx.conf (native nginx 'http'/'server'/'location'/'upstream' blocks)
  - httpd.conf / apache2.conf (native Apache '<VirtualHost>', 'ProxyPass', 'Alias', 'DocumentRoot', 'Redirect', 'Header', 'Include')

System Service (systemd / launchd / openrc / supervisor / BSD rc.d):
  1. gymbag service install --config gymbag.json
  2. gymbag service start

Verify Levels (--verify / Gymbagfile 'verify' directive — shared across HTTP, SSH, & Minecraft):
  0   always          — Require PoW verification for all visitors/connections
  1   only suspicious — Trigger verification on suspicious clients, probes, threat IPs, and VPN/proxy/Tor IPs
  2   only ai         — Trigger verification on AI crawlers & LLM agents
  3   suspicious & ai — Trigger verification on suspicious traffic, VPN/proxy IPs, and AI crawlers (default)

Flags:
  -c, --config <path>        Path to gymbag.json, Gymbagfile, Caddyfile, nginx.conf, or Apache config
  -d, --domain <domains>     Downstream domain(s), comma-separated
  -u, --upstream <url>       Upstream URL to reverse proxy to
  -l, --verify <0-3>              Verify level (0=always, 1=only suspicious, 2=only ai, 3=suspicious & ai)
      --verify-timeout <dur>      Verified clearance timeout (e.g. 30m, 12h, 7d, or seconds; default 86400s)
      --remote-rules <url>        Remote firewall rules URL (repeatable or comma-separated)
      --remote-rules-refresh <d>  Refresh interval for remote rules (default 5m)
      --ssl-provider <name>       SSL/ACME provider (letsencrypt, zerossl, google, ssl.com, litessl, buypass, local, or URL)
      --no-protect           Disable WAF verification for CLI-configured websites
      --admin <addr>         Enable local REST Admin API (e.g. 127.0.0.1:2019)
      --http-port <port>     HTTP redirect & ACME HTTP-01 port (default 80)
      --https-port <port>    HTTPS gateway port (default 443)
      --mc-console           Bring managed Minecraft server console into Gymbag foreground & Admin API
      --watch                Hot-reload config file changes automatically
`, Version)
}
