package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gymbag/internal/config"
	"gymbag/internal/localpki"
	"gymbag/internal/mcserver"
	"gymbag/internal/service"
	"gymbag/internal/setup"
)

var (
	sshSetupRunner = setup.SetupSSHProtection
	mcSetupRunner  = setup.SetupMinecraftProtection
	mcJavaResolver = mcserver.ResolveJavaForJar
)

type multiStringFlag []string

func (m *multiStringFlag) String() string {
	return strings.Join(*m, ", ")
}

func (m *multiStringFlag) Set(val string) error {
	*m = append(*m, val)
	return nil
}

func runInitCmd(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gymbag init", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		outputPath         string
		format             string
		preset             string
		templateFlags      multiStringFlag
		domain             string
		upstream           string
		siteFlags          multiStringFlag
		enableSSH          bool
		sshPort            int
		sshUpstream        string
		sshdConfigPath     string
		enableMC           bool
		mcPort             int
		mcUpstream         string
		mcPropertiesPath   string
		mcJar              string
		mcJava             string
		mcJVMArgs          string
		mcWorkDir          string
		mcConsole          bool
		verifyURL          string
		verifyPort         int
		autheliaURL        string
		autheliaURI        string
		autheliaPortal     string
		noAutoSetup        bool
		noFirewall         bool
		verifyLevel        string
		difficulty         int
		verifyTimeout      string
		remoteRules        multiStringFlag
		remoteRulesRefresh string
		rulesFiles         multiStringFlag
		rulesDirs          multiStringFlag
		rulesDynamic       bool
		rulesWatchInterval string
		noProtect          bool
		sslProvider        string
		email              string
		eabKID             string
		eabHMACKey         string
		httpPort           int
		httpsPort          int
		bindHost           string
		adminAddr          string
		certDir            string
		redisURL           string
		ipSecurityAction   string
		noDefaultRules     bool
		noAutoHosts        bool
		force              bool
		yes                bool
		interactive        bool
		dryRun             bool
		serviceType        string
		serviceName        string
		serviceUser        string
		serviceUserScope   bool
		serviceSysScope    bool
		serviceStart       bool
	)

	fs.StringVar(&outputPath, "o", "", "Output config file path (default gymbag.json, or Gymbagfile for --format gymbagfile)")
	fs.StringVar(&outputPath, "output", "", "Output config file path (default gymbag.json, or Gymbagfile for --format gymbagfile)")
	fs.StringVar(&outputPath, "c", "", "Alias for --output")
	fs.StringVar(&outputPath, "config", "", "Alias for --output")
	fs.StringVar(&format, "format", "", "Config format: json (default) or gymbagfile")
	fs.StringVar(&preset, "preset", "", "Preset template: standard, local, public, strict, ai-only, suspicious, proxy-only, http, ssh, minecraft, or all")
	fs.Var(&templateFlags, "t", "Service template(s): http, ssh, minecraft, or all (comma-separated or repeatable)")
	fs.Var(&templateFlags, "template", "Service template(s): http, ssh, minecraft, or all (comma-separated or repeatable)")
	fs.StringVar(&domain, "d", "", "Downstream domain(s), comma-separated")
	fs.StringVar(&domain, "domain", "", "Downstream domain(s), comma-separated")
	fs.StringVar(&upstream, "u", "", "Upstream URL (e.g. http://localhost:3000 or :3000)")
	fs.StringVar(&upstream, "upstream", "", "Upstream URL (e.g. http://localhost:3000 or :3000)")
	fs.Var(&siteFlags, "site", "Add a site as domain=upstream (repeatable)")
	fs.BoolVar(&enableSSH, "ssh", false, "Enable SSH protection proxy (default port 22)")
	fs.IntVar(&sshPort, "ssh-port", 0, "SSH listen port (default 22)")
	fs.StringVar(&sshUpstream, "ssh-upstream", "", "Existing sshd upstream (skips automatic sshd_config modification)")
	fs.StringVar(&sshdConfigPath, "sshd-config", "", "Path to sshd_config for automatic random-port setup (default /etc/ssh/sshd_config)")
	fs.BoolVar(&enableMC, "minecraft", false, "Enable Minecraft server protection proxy (default port 25565)")
	fs.BoolVar(&enableMC, "mc", false, "Alias for --minecraft")
	fs.IntVar(&mcPort, "mc-port", 0, "Minecraft listen port (default 25565)")
	fs.IntVar(&mcPort, "minecraft-port", 0, "Alias for --mc-port")
	fs.StringVar(&mcUpstream, "mc-upstream", "", "Existing Minecraft server upstream (skips automatic server.properties modification)")
	fs.StringVar(&mcUpstream, "minecraft-upstream", "", "Alias for --mc-upstream")
	fs.StringVar(&mcPropertiesPath, "mc-properties", "", "Path to server.properties for automatic random-port setup (default server.properties)")
	fs.StringVar(&mcPropertiesPath, "server-properties", "", "Alias for --mc-properties")
	fs.StringVar(&mcJar, "mc-jar", "", "Path to Minecraft server .jar (enables auto-start & automatic Java selection)")
	fs.StringVar(&mcJar, "server-jar", "", "Alias for --mc-jar")
	fs.StringVar(&mcJava, "mc-java", "", "Explicit Java binary path for Minecraft server (defaults to auto-select)")
	fs.StringVar(&mcJava, "java", "", "Alias for --mc-java")
	fs.StringVar(&mcJVMArgs, "mc-jvm-args", "", "Space-separated JVM flags for Minecraft server (e.g. '-Xms1G -Xmx2G')")
	fs.StringVar(&mcWorkDir, "mc-work-dir", "", "Working directory for managed Minecraft server (defaults to JAR directory)")
	fs.BoolVar(&mcConsole, "mc-console", false, "Bring managed Minecraft server console into Gymbag foreground & Admin API")
	fs.BoolVar(&mcConsole, "minecraft-console", false, "Alias for --mc-console")
	fs.StringVar(&verifyURL, "verify-url", "", "Public HTTPS verification portal URL for SSH/Minecraft (defaults to server IP)")
	fs.IntVar(&verifyPort, "verify-port", 0, "Shared HTTPS verification portal port (default 8443)")
	fs.StringVar(&autheliaURL, "authelia", "", "Enable Authelia forward auth with this URL (e.g. http://127.0.0.1:9091)")
	fs.StringVar(&autheliaURI, "authelia-uri", "", "Authelia forward-auth endpoint URI (default /api/authz/forward-auth)")
	fs.StringVar(&autheliaPortal, "authelia-portal", "", "Authelia login portal URL for browser 401 redirect / rd=")
	fs.BoolVar(&noAutoSetup, "no-auto-setup", false, "Skip automatic sshd_config/firewall and server.properties modifications")
	fs.BoolVar(&noFirewall, "no-firewall", false, "Skip automatic firewall rules when configuring SSH protection")
	fs.StringVar(&verifyLevel, "l", "", "Verify level: 0 (always), 1 (only suspicious), 2 (only ai), 3 (suspicious & ai), or off")
	fs.StringVar(&verifyLevel, "verify", "", "Verify level: 0..3 or off")
	fs.StringVar(&verifyLevel, "verify-level", "", "Alias for --verify")
	fs.StringVar(&verifyLevel, "m", "", "Alias for --verify")
	fs.StringVar(&verifyLevel, "mode", "", "Alias for --verify")
	fs.IntVar(&difficulty, "difficulty", 0, "SHA-256 PoW difficulty (1-5, default 4)")
	fs.StringVar(&verifyTimeout, "verify-timeout", "", "Verified clearance timeout (seconds or duration like 30m, 12h, 7d)")
	fs.StringVar(&verifyTimeout, "clearance-ttl", "", "Alias for --verify-timeout")
	fs.Var(&remoteRules, "remote-rules", "Remote firewall rules URL (http/https, repeatable or comma-separated)")
	fs.StringVar(&remoteRulesRefresh, "remote-rules-refresh", "", "Refresh interval for remote rules (seconds or duration like 5m, 1h)")
	fs.Var(&rulesFiles, "rules-file", "Dedicated firewall rules file path (repeatable)")
	fs.Var(&rulesDirs, "rules-dir", "Directory of firewall rule files (repeatable)")
	fs.BoolVar(&rulesDynamic, "rules-dynamic", false, "Auto-reload --rules-file / --rules-dir when files change")
	fs.StringVar(&rulesWatchInterval, "rules-watch-interval", "", "Polling interval for dynamic local rule files/folders (default 2s)")
	fs.BoolVar(&noProtect, "no-protect", false, "Disable WAF verification (protect off)")
	fs.StringVar(&sslProvider, "ssl-provider", "", "SSL/ACME provider (letsencrypt, zerossl, google, ssl.com, litessl, buypass, local, or URL)")
	fs.StringVar(&email, "email", "", "ACME contact email")
	fs.StringVar(&eabKID, "eab-kid", "", "ACME External Account Binding Key ID")
	fs.StringVar(&eabHMACKey, "eab-hmac-key", "", "ACME External Account Binding HMAC key")
	fs.IntVar(&httpPort, "http-port", 0, "HTTP redirect/ACME port (default 80)")
	fs.IntVar(&httpsPort, "https-port", 0, "HTTPS gateway port (default 443)")
	fs.StringVar(&bindHost, "bind", "", "Bind host IP (default auto)")
	fs.StringVar(&adminAddr, "admin", "", "Local REST Admin API listen address (e.g. 127.0.0.1:2019)")
	fs.StringVar(&certDir, "cert-dir", "", "Directory for certs, local CA, and intel cache (default certs)")
	fs.StringVar(&redisURL, "redis", "", "Optional Redis URL for shared challenge/rate-limit state")
	fs.StringVar(&ipSecurityAction, "ip-security-action", "", "Action for threat-intel IP hits: verify or block")
	fs.BoolVar(&noDefaultRules, "no-default-rules", false, "Disable built-in baseline firewall rules (default_rules off)")
	fs.BoolVar(&noAutoHosts, "no-auto-hosts", false, "Disable automatic /etc/hosts management for local domains")
	fs.BoolVar(&force, "f", false, "Overwrite existing config file")
	fs.BoolVar(&force, "force", false, "Overwrite existing config file")
	fs.BoolVar(&force, "overwrite", false, "Overwrite existing config file")
	fs.BoolVar(&yes, "y", false, "Non-interactive mode; use defaults for unspecified options")
	fs.BoolVar(&yes, "yes", false, "Non-interactive mode; use defaults for unspecified options")
	fs.BoolVar(&interactive, "i", false, "Run interactive setup wizard")
	fs.BoolVar(&interactive, "interactive", false, "Run interactive setup wizard")
	fs.BoolVar(&dryRun, "dry-run", false, "Print generated config to stdout without writing to disk")
	fs.BoolVar(&dryRun, "stdout", false, "Print generated config to stdout without writing to disk")
	fs.StringVar(&serviceType, "service", "", "Also register a system service (auto, systemd, launchd, openrc, supervisor, freebsd-rc, openbsd-rc, netbsd-rc)")
	fs.StringVar(&serviceName, "service-name", "gymbag", "System service name when --service is used")
	fs.StringVar(&serviceUser, "service-user", "", "Run-as user for system-level service")
	fs.BoolVar(&serviceUserScope, "user", false, "Register user-level service when --service is used")
	fs.BoolVar(&serviceSysScope, "system", false, "Register system-wide service when --service is used")
	fs.BoolVar(&serviceStart, "start", false, "Start the registered system service immediately")

	if err := parseInterspersedFlags(fs, args); err != nil {
		if err == flag.ErrHelp {
			printInitUsage(stdout)
			return 0
		}
		return 2
	}
	if outputPath == "" && fs.NArg() > 0 {
		outputPath = fs.Arg(0)
	}
	if fs.NArg() > 1 {
		_, _ = fmt.Fprintln(stderr, "ERROR gymbag init: at most one positional output file path is accepted")
		return 2
	}
	if serviceUserScope && serviceSysScope {
		_, _ = fmt.Fprintln(stderr, "ERROR gymbag init: --user and --system cannot be used together")
		return 2
	}

	effectiveFmt := strings.TrimSpace(format)
	if effectiveFmt == "" {
		if looksLikeGymbagfilePath(outputPath) {
			effectiveFmt = "gymbagfile"
		} else {
			effectiveFmt = "json"
		}
	}
	normFmt, err := config.NormalizeConfigFormat(effectiveFmt)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag init: %v\n", err)
		return 2
	}
	if outputPath == "" {
		if normFmt == "json" {
			outputPath = "gymbag.json"
		} else {
			outputPath = "Gymbagfile"
		}
	}

	var initRuleSources []config.RuleSource
	for _, p := range append(append([]string(nil), rulesFiles...), rulesDirs...) {
		for _, part := range strings.Split(p, ",") {
			src := strings.TrimSpace(part)
			if src == "" {
				continue
			}
			initRuleSources = append(initRuleSources, config.RuleSource{
				Source:  src,
				Dynamic: rulesDynamic,
			})
		}
	}

	var parsedSites []config.SiteSpec
	for _, rawSite := range siteFlags {
		specs, sErr := config.ParseSiteSpec(rawSite)
		if sErr != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag init: %v\n", sErr)
			return 2
		}
		parsedSites = append(parsedSites, specs...)
	}

	useHTTP, useSSH, useMC, tErr := config.ResolveTemplateSelection(templateFlags)
	if tErr != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag init: %v\n", tErr)
		return 2
	}
	if enableSSH || sshPort != 0 || sshUpstream != "" || sshdConfigPath != "" {
		useSSH = true
	}
	if enableMC || mcPort != 0 || mcUpstream != "" || mcPropertiesPath != "" ||
		mcJar != "" || mcJava != "" || mcJVMArgs != "" || mcWorkDir != "" || mcConsole {
		useMC = true
	}
	if domain != "" || upstream != "" || len(parsedSites) > 0 {
		useHTTP = true
	}
	if preset != "" {
		switch strings.ToLower(strings.TrimSpace(preset)) {
		case "ssh":
			useSSH = true
		case "minecraft", "mc":
			useMC = true
		case "all":
			useHTTP, useSSH, useMC = true, true, true
		default:
			useHTTP = true
		}
	}

	var protectOpt *bool
	if noProtect {
		f := false
		protectOpt = &f
	}
	var defaultRulesOpt *bool
	if noDefaultRules {
		f := false
		defaultRulesOpt = &f
	}
	var autoHostsOpt *bool
	if noAutoHosts {
		f := false
		autoHostsOpt = &f
	}

	if (autheliaURI != "" || autheliaPortal != "") && strings.TrimSpace(autheliaURL) == "" {
		_, _ = fmt.Fprintln(stderr, "ERROR gymbag init: --authelia-uri and --authelia-portal require --authelia <url>")
		return 2
	}

	var autheliaOpt *config.ForwardAuthConfig
	if strings.TrimSpace(autheliaURL) != "" {
		autheliaOpt = &config.ForwardAuthConfig{
			Provider:  "authelia",
			URL:       autheliaURL,
			URI:       autheliaURI,
			PortalURL: autheliaPortal,
		}
	}

	opts := config.GenerateOptions{
		Preset:             preset,
		Domain:             domain,
		Upstream:           upstream,
		Sites:              parsedSites,
		Verify:             config.VerifyServerConfig{URL: verifyURL, Port: verifyPort},
		Authelia:           autheliaOpt,
		VerifyLevel:        verifyLevel,
		Protect:            protectOpt,
		Difficulty:         difficulty,
		ClearanceTTL:       verifyTimeout,
		RemoteRules:        remoteRules,
		RemoteRulesRefresh: remoteRulesRefresh,
		RuleSources:        initRuleSources,
		RulesWatchInterval: rulesWatchInterval,
		DefaultRules:       defaultRulesOpt,
		SSLProvider:        sslProvider,
		Email:              email,
		EABKID:             eabKID,
		EABHMACKey:         eabHMACKey,
		HTTPPort:           httpPort,
		HTTPSPort:          httpsPort,
		BindHost:           bindHost,
		AdminAddr:          adminAddr,
		CertDir:            certDir,
		RedisURL:           redisURL,
		AutoHosts:          autoHostsOpt,
		IPSecurityAction:   ipSecurityAction,
		Format:             normFmt,
		IncludeComments:    true,
	}
	if useSSH {
		opts.SSH = &config.TCPSpec{Port: sshPort, Upstream: sshUpstream}
	}
	if useMC {
		opts.Minecraft = &config.TCPSpec{
			Port:      mcPort,
			Upstream:  mcUpstream,
			ServerJar: mcJar,
			JavaPath:  mcJava,
			JVMArgs:   strings.Fields(mcJVMArgs),
			WorkDir:   mcWorkDir,
			Console:   mcConsole,
		}
	}

	hasConfigFlags := len(templateFlags) > 0 || preset != "" || domain != "" || upstream != "" || len(parsedSites) > 0 ||
		useSSH || useMC || verifyURL != "" || verifyPort != 0 || autheliaURL != "" ||
		verifyLevel != "" || verifyTimeout != "" || len(remoteRules) > 0 || len(initRuleSources) > 0 ||
		rulesWatchInterval != "" || noProtect || sslProvider != "" || email != "" ||
		httpPort != 0 || httpsPort != 0 || adminAddr != ""
	runWizard := interactive || (!yes && !hasConfigFlags && len(args) == 0)
	if !interactive && !yes && !hasConfigFlags && len(args) > 0 {
		// e.g. `gymbag init -o custom.Gymbagfile` without -y still prompts unless -y is given
		runWizard = true
	}

	if runWizard {
		promptOut := stdout
		if dryRun {
			promptOut = stderr
		}
		if !dryRun && !force {
			if _, statErr := os.Stat(outputPath); statErr == nil {
				ok, pErr := promptYesNo(bufio.NewReader(stdin), promptOut, fmt.Sprintf("Config file %s already exists. Overwrite?", outputPath), false)
				if pErr != nil {
					_, _ = fmt.Fprintf(stderr, "ERROR gymbag init: %v\n", pErr)
					return 1
				}
				if !ok {
					_, _ = fmt.Fprintln(promptOut, "Aborted without modifying existing config.")
					return 1
				}
				force = true
			}
		}

		wizardSvc, wErr := runInitWizard(stdin, promptOut, &opts, &useHTTP, &useSSH, &useMC, &sshdConfigPath, &mcPropertiesPath, dryRun || noAutoSetup)
		if wErr != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag init: %v\n", wErr)
			return 1
		}
		if serviceType == "" && wizardSvc != "" {
			serviceType = wizardSvc
		}
	} else if !dryRun && !force {
		if _, statErr := os.Stat(outputPath); statErr == nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag init: %s already exists (use --force to overwrite or --output to specify another path)\n", outputPath)
			return 1
		}
	}

	if !useHTTP && !useSSH && !useMC {
		useHTTP = true
	}
	opts.Templates = nil
	if useHTTP {
		opts.Templates = append(opts.Templates, "http")
	}
	if useSSH {
		opts.Templates = append(opts.Templates, "ssh")
		if opts.SSH == nil {
			opts.SSH = &config.TCPSpec{}
		}
	} else {
		opts.SSH = nil
	}
	if useMC {
		opts.Templates = append(opts.Templates, "minecraft")
		if opts.Minecraft == nil {
			opts.Minecraft = &config.TCPSpec{}
		}
	} else {
		opts.Minecraft = nil
	}

	if !dryRun && !noAutoSetup {
		effectiveVerifyPort := opts.Verify.EffectivePort()
		if useSSH && opts.SSH != nil && strings.TrimSpace(opts.SSH.Upstream) == "" {
			listenPort := opts.SSH.Port
			if listenPort <= 0 {
				listenPort = config.DefaultSSHPort
			}
			res, sErr := sshSetupRunner(context.Background(), setup.SSHSetupOptions{
				ConfigPath:    sshdConfigPath,
				SSHListenPort: listenPort,
				VerifyPort:    effectiveVerifyPort,
				SkipFirewall:  noFirewall,
			})
			if sErr != nil {
				_, _ = fmt.Fprintf(stderr, "ERROR gymbag init: automatic SSH host setup failed: %v\nHint: run with sudo, specify --sshd-config <path>, pass --ssh-upstream 127.0.0.1:<port>, or use --no-auto-setup\n", sErr)
				return 1
			}
			opts.SSH.Upstream = res.Upstream
			_, _ = fmt.Fprintf(stdout, "Configured SSH backend: %s -> %s (backup: %s)\n", res.ConfigPath, res.Upstream, res.BackupPath)
			if res.FirewallBackend != "" && res.FirewallBackend != "none" {
				_, _ = fmt.Fprintf(stdout, "Configured firewall (%s): blocked external access to port %d; allowed %d and %d\n",
					res.FirewallBackend, res.BackendPort, listenPort, effectiveVerifyPort)
			}
			for _, w := range res.Warnings {
				_, _ = fmt.Fprintf(stderr, "WARN gymbag init: %s\n", w)
			}
		}
		if useMC && opts.Minecraft != nil {
			listenPort := opts.Minecraft.Port
			if listenPort <= 0 {
				listenPort = config.DefaultMinecraftPort
			}
			hasJar := strings.TrimSpace(opts.Minecraft.ServerJar) != ""
			if strings.TrimSpace(opts.Minecraft.Upstream) == "" {
				propsPath := strings.TrimSpace(mcPropertiesPath)
				if propsPath == "" && hasJar {
					baseDir := strings.TrimSpace(opts.Minecraft.WorkDir)
					if baseDir == "" {
						baseDir = filepath.Dir(opts.Minecraft.ServerJar)
					}
					propsPath = filepath.Join(baseDir, "server.properties")
				}
				res, mErr := mcSetupRunner(setup.MinecraftSetupOptions{
					PropertiesPath:      propsPath,
					MinecraftListenPort: listenPort,
					VerifyPort:          effectiveVerifyPort,
					BindLoopbackIfEmpty: true,
					CreateIfMissing:     hasJar,
				})
				if mErr != nil {
					_, _ = fmt.Fprintf(stderr, "ERROR gymbag init: automatic Minecraft setup failed: %v\nHint: pass --server-properties <path/to/server.properties>, pass --mc-upstream 127.0.0.1:<port>, or use --no-auto-setup\n", mErr)
					return 1
				}
				opts.Minecraft.Upstream = res.Upstream
				if res.Created {
					_, _ = fmt.Fprintf(stdout, "Initialized Minecraft backend: %s server-port=%d -> %s\n",
						res.PropertiesPath, res.BackendPort, res.Upstream)
				} else {
					_, _ = fmt.Fprintf(stdout, "Configured Minecraft backend: %s server-port=%d -> %s (backup: %s)\n",
						res.PropertiesPath, res.BackendPort, res.Upstream, res.BackupPath)
				}
			}
			if hasJar {
				sel, jErr := mcJavaResolver(context.Background(), opts.Minecraft.ServerJar, opts.Minecraft.JavaPath, mcserver.DiscoverOptions{})
				if jErr != nil {
					_, _ = fmt.Fprintf(stderr, "WARN gymbag init: Java auto-selection note for %s: %v\n", opts.Minecraft.ServerJar, jErr)
				} else {
					_, _ = fmt.Fprintf(stdout, "Selected Java %d (%s) for %s (requires Java %d+)\n",
						sel.Java.Major, sel.Java.Path, opts.Minecraft.ServerJar, sel.Requirement.MinMajor)
					if sel.Warning != "" {
						_, _ = fmt.Fprintf(stderr, "WARN gymbag init: %s\n", sel.Warning)
					}
				}
			}
		}
	}

	rendered, cfg, err := config.RenderGeneratedConfig(opts)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag init: %v\n", err)
		return 1
	}

	if dryRun {
		_, _ = stdout.Write(rendered)
		return 0
	}

	if dir := filepath.Dir(outputPath); dir != "" && dir != "." {
		if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag init: failed to create directory %s: %v\n", dir, mkErr)
			return 1
		}
	}
	if err := os.WriteFile(outputPath, rendered, 0o644); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag init: failed to write %s: %v\n", outputPath, err)
		return 1
	}

	// Print concise summary & 2-command next steps
	var summaries []string
	for _, s := range cfg.Sites {
		mode := fmt.Sprintf("verify: %d [%s]", int(s.WAF.VerifyLevel), s.WAF.VerifyLevel.Label())
		if !s.Protect {
			mode = "protect off"
		}
		summaries = append(summaries, fmt.Sprintf("%s -> %s (%s)", s.Domain, strings.Join(s.Upstreams, ", "), mode))
	}
	if cfg.SSHEnabled() {
		mode := fmt.Sprintf("verify: %d [%s]", int(cfg.SSH.VerifyLevel), cfg.SSH.VerifyLevel.Label())
		if !cfg.SSH.Protect {
			mode = "protect off"
		}
		summaries = append(summaries, fmt.Sprintf("ssh :%d -> %s (%s)", cfg.SSH.Port, cfg.SSH.Upstream, mode))
	}
	if cfg.MinecraftEnabled() {
		mode := fmt.Sprintf("verify: %d [%s]", int(cfg.Minecraft.VerifyLevel), cfg.Minecraft.VerifyLevel.Label())
		if !cfg.Minecraft.Protect {
			mode = "protect off"
		}
		jarNote := ""
		if cfg.Minecraft.ServerJar != "" {
			jarNote = ", jar: " + cfg.Minecraft.ServerJar
		}
		summaries = append(summaries, fmt.Sprintf("minecraft :%d -> %s (%s%s)", cfg.Minecraft.Port, cfg.Minecraft.Upstream, mode, jarNote))
	}
	_, _ = fmt.Fprintf(stdout, "Generated %s (%s)\n", outputPath, strings.Join(summaries, "; "))
	if cfg.NeedsVerifyServer() {
		vURL := cfg.Verify.URL
		if vURL == "" {
			vURL = fmt.Sprintf("https://<server-ip>:%d (auto IP SSL cert)", cfg.Verify.EffectivePort())
		}
		_, _ = fmt.Fprintf(stdout, "Verify portal: %s\n", vURL)
	}

	if strings.TrimSpace(serviceType) != "" {
		mgr, mErr := service.ParseManagerType(serviceType)
		if mErr != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag init: %v\n", mErr)
			return 1
		}
		userScope := resolveUserScope(mgr, serviceUserScope, serviceSysScope)
		portsPriv := (cfg.HTTPEnabled() && (cfg.HTTPPort < 1024 || cfg.HTTPSPort < 1024)) ||
			(cfg.SSHEnabled() && cfg.SSH.Port < 1024) ||
			(cfg.MinecraftEnabled() && cfg.Minecraft.Port < 1024) ||
			(cfg.NeedsVerifyServer() && cfg.Verify.EffectivePort() < 1024)
		svcOpts := service.Options{
			Manager:   mgr,
			Overwrite: force,
			Enable:    true,
			Start:     serviceStart,
			Runner:    serviceCommandRunner,
			Spec: service.Spec{
				Name:            serviceName,
				ConfigPath:      outputPath,
				User:            serviceUser,
				UserScope:       userScope,
				Watch:           true,
				PortsPrivileged: portsPriv,
			},
		}
		res, sErr := service.Install(context.Background(), svcOpts)
		if sErr != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag init: config was written to %s, but service registration failed: %v\n", outputPath, sErr)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "Registered %s service: %s\n", res.Manager, res.UnitPath)
		if !serviceStart {
			_, _ = fmt.Fprintf(stdout, "\nNext step (start service):\n  %s\n", formatServiceCommandHint("start", serviceName, userScope))
		}
		return 0
	}

	_, _ = fmt.Fprintln(stdout, "\nNext steps:")
	if outputPath == "gymbag.json" || outputPath == "Gymbagfile" {
		_, _ = fmt.Fprintln(stdout, "  Start server:     gymbag")
	} else {
		_, _ = fmt.Fprintf(stdout, "  Start server:     gymbag --config %s\n", outputPath)
	}
	_, _ = fmt.Fprintf(stdout, "  System service:   gymbag service install --config %s\n", outputPath)
	return 0
}

func looksLikeGymbagfilePath(p string) bool {
	if strings.TrimSpace(p) == "" {
		return false
	}
	base := strings.ToLower(filepath.Base(p))
	return base == "gymbagfile" || base == "caddyfile" ||
		strings.HasSuffix(base, ".gymbagfile") ||
		strings.HasSuffix(base, ".caddy") ||
		strings.HasSuffix(base, ".caddyfile")
}

func runInitWizard(
	stdin io.Reader,
	out io.Writer,
	opts *config.GenerateOptions,
	useHTTP, useSSH, useMC *bool,
	sshdConfigPath, mcPropertiesPath *string,
	skipAutoSetup bool,
) (string, error) {
	r := bufio.NewReader(stdin)

	_, _ = fmt.Fprintln(out, "Gymbag config generator — press Enter to accept [default].")

	if !*useHTTP && !*useSSH && !*useMC {
		ans, err := promptLine(r, out, "Services to protect (http, ssh, minecraft, or comma-separated like http,ssh,minecraft)", "http")
		if err != nil {
			return "", err
		}
		h, s, m, tErr := config.ResolveTemplateSelection([]string{ans})
		if tErr != nil {
			return "", tErr
		}
		if !h && !s && !m {
			h = true
		}
		*useHTTP, *useSSH, *useMC = h, s, m
	}

	if *useHTTP {
		if opts.Domain == "" && len(opts.Sites) == 0 {
			ans, err := promptLine(r, out, "Downstream domain(s) (e.g. app.example.com, app.internal)", "localhost")
			if err != nil {
				return "", err
			}
			opts.Domain = ans
		}

		if opts.Upstream == "" && len(opts.Sites) == 0 {
			defUp := "http://localhost:5173"
			if opts.Domain != "" && opts.Domain != "localhost" {
				defUp = "http://localhost:3000"
			}
			ans, err := promptLine(r, out, "Upstream URL (e.g. http://localhost:3000 or :3000)", defUp)
			if err != nil {
				return "", err
			}
			opts.Upstream = ans
		}
	}

	if *useSSH {
		if opts.SSH == nil {
			opts.SSH = &config.TCPSpec{}
		}
		if opts.SSH.Port == 0 {
			ans, err := promptLine(r, out, "SSH listen port", "22")
			if err != nil {
				return "", err
			}
			p, pErr := parseSinglePort(ans, "SSH port")
			if pErr != nil {
				return "", pErr
			}
			opts.SSH.Port = p
		}
		if opts.SSH.Upstream == "" {
			ans, err := promptLine(r, out, "SSH backend upstream ('auto' to randomize sshd port + set firewall, or host:port)", "auto")
			if err != nil {
				return "", err
			}
			if !strings.EqualFold(ans, "auto") {
				opts.SSH.Upstream = ans
			}
		}
		if opts.SSH.Upstream == "" && !skipAutoSetup && *sshdConfigPath == "" {
			ans, err := promptLine(r, out, "Path to sshd_config", setup.DefaultSSHDConfigPath)
			if err != nil {
				return "", err
			}
			*sshdConfigPath = ans
		}
	}

	if *useMC {
		if opts.Minecraft == nil {
			opts.Minecraft = &config.TCPSpec{}
		}
		if opts.Minecraft.Port == 0 {
			ans, err := promptLine(r, out, "Minecraft listen port", "25565")
			if err != nil {
				return "", err
			}
			p, pErr := parseSinglePort(ans, "Minecraft port")
			if pErr != nil {
				return "", pErr
			}
			opts.Minecraft.Port = p
		}
		if opts.Minecraft.ServerJar == "" {
			ans, err := promptLine(r, out, "Minecraft server .jar path (optional; auto-starts server & selects Java automatically)", "")
			if err != nil {
				return "", err
			}
			opts.Minecraft.ServerJar = ans
		}
		if opts.Minecraft.ServerJar != "" && !opts.Minecraft.Console {
			enableConsole, err := promptYesNo(r, out, "Bring Minecraft server console into Gymbag app?", true)
			if err != nil {
				return "", err
			}
			opts.Minecraft.Console = enableConsole
		}
		if opts.Minecraft.Upstream == "" {
			ans, err := promptLine(r, out, "Minecraft backend upstream ('auto' to randomize server.properties port, or host:port)", "auto")
			if err != nil {
				return "", err
			}
			if !strings.EqualFold(ans, "auto") {
				opts.Minecraft.Upstream = ans
			}
		}
		if opts.Minecraft.Upstream == "" && !skipAutoSetup && *mcPropertiesPath == "" {
			defProps := "server.properties"
			if opts.Minecraft.WorkDir != "" {
				defProps = filepath.Join(opts.Minecraft.WorkDir, "server.properties")
			} else if opts.Minecraft.ServerJar != "" {
				defProps = filepath.Join(filepath.Dir(opts.Minecraft.ServerJar), "server.properties")
			}
			ans, err := promptLine(r, out, "Path to Minecraft server.properties", defProps)
			if err != nil {
				return "", err
			}
			*mcPropertiesPath = ans
		}
	}

	if opts.VerifyLevel == "" && opts.Protect == nil {
		ans, err := promptLine(r, out, "Verify level (0=always, 1=suspicious, 2=ai, 3=suspicious & ai, off=proxy only)", "3")
		if err != nil {
			return "", err
		}
		opts.VerifyLevel = ans
	}

	if *useSSH || *useMC {
		if opts.Verify.URL == "" {
			ans, err := promptLine(r, out, "Verification page URL (optional https://domain; blank defaults to server IP + IP SSL cert)", "")
			if err != nil {
				return "", err
			}
			opts.Verify.URL = ans
		}
		if opts.Verify.Port == 0 {
			ans, err := promptLine(r, out, "Verification page HTTPS port", "8443")
			if err != nil {
				return "", err
			}
			p, pErr := parseSinglePort(ans, "Verification HTTPS port")
			if pErr != nil {
				return "", pErr
			}
			opts.Verify.Port = p
		}
	}

	// Ask TLS/ACME questions only when at least one public DNS domain is configured.
	var candidateDomains []string
	if *useHTTP {
		for _, part := range strings.Split(opts.Domain, ",") {
			if t := strings.TrimSpace(part); t != "" {
				candidateDomains = append(candidateDomains, t)
			}
		}
		for _, s := range opts.Sites {
			candidateDomains = append(candidateDomains, s.Domain)
		}
	}
	if *useSSH || *useMC {
		if vHost, _, _, err := opts.Verify.ParsedHost(); err == nil && vHost != "" && net.ParseIP(vHost) == nil {
			candidateDomains = append(candidateDomains, vHost)
		}
	}
	if len(candidateDomains) > 0 && !localpki.AllLocalDomains(candidateDomains) {
		if opts.SSLProvider == "" {
			ans, err := promptLine(r, out, "SSL provider (letsencrypt, zerossl, google, ssl.com, litessl, buypass, local)", "letsencrypt")
			if err != nil {
				return "", err
			}
			opts.SSLProvider = ans
		}
		provLower := strings.ToLower(strings.TrimSpace(opts.SSLProvider))
		if provLower != "local" && opts.Email == "" {
			ans, err := promptLine(r, out, "ACME contact email (optional; enables ZeroSSL auto-EAB)", "")
			if err != nil {
				return "", err
			}
			opts.Email = ans
		}
		if (provLower == "google" || provLower == "gts" || provLower == "ssl.com" || provLower == "sslcom" || provLower == "ssl.com-rsa") && opts.EABKID == "" {
			kid, err := promptLine(r, out, "EAB Key ID (optional, leave blank to configure later)", "")
			if err != nil {
				return "", err
			}
			opts.EABKID = kid
			if kid != "" && opts.EABHMACKey == "" {
				hmacKey, err := promptLine(r, out, "EAB HMAC Key", "")
				if err != nil {
					return "", err
				}
				opts.EABHMACKey = hmacKey
			}
		}
	}

	if *useHTTP && opts.HTTPPort == 0 && opts.HTTPSPort == 0 {
		ans, err := promptLine(r, out, "HTTP/HTTPS ports (use 8080/8443 for non-root)", "80/443")
		if err != nil {
			return "", err
		}
		httpP, httpsP, pErr := parsePortsPair(ans)
		if pErr != nil {
			return "", pErr
		}
		opts.HTTPPort = httpP
		opts.HTTPSPort = httpsP
	}

	if opts.AdminAddr == "" {
		enableAdmin, err := promptYesNo(r, out, "Enable local Admin API on 127.0.0.1:2019?", false)
		if err != nil {
			return "", err
		}
		if enableAdmin {
			opts.AdminAddr = "127.0.0.1:2019"
		}
	}

	svcAns, err := promptLine(r, out, "Register system service? (none, auto, systemd, launchd, openrc, supervisor, freebsd-rc, openbsd-rc, netbsd-rc)", "none")
	if err != nil {
		return "", err
	}
	trimmedSvc := strings.ToLower(strings.TrimSpace(svcAns))
	if trimmedSvc == "" || trimmedSvc == "none" || trimmedSvc == "no" || trimmedSvc == "n" {
		return "", nil
	}
	return trimmedSvc, nil
}

func parseSinglePort(raw, label string) (int, error) {
	p, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || p < 1 || p > 65535 {
		return 0, fmt.Errorf("invalid %s %q (expected 1-65535)", label, raw)
	}
	return p, nil
}

func parsePortsPair(raw string) (int, int, error) {
	cleaned := strings.TrimSpace(raw)
	if cleaned == "" || cleaned == "80/443" {
		return 80, 443, nil
	}
	sep := "/"
	if strings.Contains(cleaned, ",") {
		sep = ","
	} else if strings.Contains(cleaned, ":") {
		sep = ":"
	}
	parts := strings.Split(cleaned, sep)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid ports %q (expected HTTP/HTTPS like 80/443 or 8080/8443)", raw)
	}
	h, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || h < 1 || h > 65535 {
		return 0, 0, fmt.Errorf("invalid HTTP port %q", parts[0])
	}
	hs, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || hs < 1 || hs > 65535 {
		return 0, 0, fmt.Errorf("invalid HTTPS port %q", parts[1])
	}
	return h, hs, nil
}

func promptLine(r *bufio.Reader, w io.Writer, label, defaultVal string) (string, error) {
	if defaultVal != "" {
		_, _ = fmt.Fprintf(w, "%s [%s]: ", label, defaultVal)
	} else {
		_, _ = fmt.Fprintf(w, "%s: ", label)
	}
	line, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return defaultVal, nil
	}
	return trimmed, nil
}

func promptYesNo(r *bufio.Reader, w io.Writer, label string, defaultYes bool) (bool, error) {
	hint := "[y/N]"
	if defaultYes {
		hint = "[Y/n]"
	}
	_, _ = fmt.Fprintf(w, "%s %s: ", label, hint)
	line, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	ans := strings.ToLower(strings.TrimSpace(line))
	if ans == "" {
		return defaultYes, nil
	}
	switch ans {
	case "y", "yes", "true", "1":
		return true, nil
	case "n", "no", "false", "0":
		return false, nil
	default:
		return defaultYes, nil
	}
}

func printInitUsage(w io.Writer) {
	_, _ = fmt.Fprintf(w, `Usage:
  gymbag init [flags] [output_path]

Two-command quickstart:
  HTTP/HTTPS:
    1. gymbag init -d app.example.com -u :3000
    2. gymbag
  SSH protection (moves sshd to random port + configures firewall):
    1. sudo gymbag init -t ssh
    2. sudo gymbag
  Minecraft protection (auto-starts server jar with best Java + randomizes port):
    1. gymbag init -t minecraft --server-jar /srv/mc/server.jar
    2. gymbag
  All-in-One (HTTP + SSH + Minecraft in one process):
    1. gymbag init -t http,ssh,minecraft -d app.example.com -u :3000 --server-jar /srv/mc/server.jar
    2. gymbag

Examples:
  gymbag init                                         # Interactive wizard -> ./gymbag.json
  gymbag init --yes                                   # Non-interactive default localhost JSON config
  gymbag init -t ssh --verify-url https://verify.example.com:8443
  gymbag init -t minecraft --server-jar /srv/mc/paper.jar --mc-jvm-args "-Xms1G -Xmx4G"
  gymbag init -t minecraft --server-properties ./server.properties --verify-port 8443
  gymbag init -t http,ssh,minecraft -d app.example.com -u :3000 --ssh-upstream 127.0.0.1:2222 --mc-upstream 127.0.0.1:25566
  gymbag init --format gymbagfile                     # Generate Caddyfile-style Gymbagfile instead of JSON
  gymbag init --preset local                          # Local *.internal dev config with local PKI
  gymbag init -d app.example.com -u :3000 --service auto  # Generate config + register system service

Flags:
  -o, --output <path>          Output config file path (default gymbag.json, or Gymbagfile for --format gymbagfile)
      --format <fmt>           Config format: json (default) or gymbagfile
  -t, --template <list>        Service templates: http, ssh, minecraft, or all (comma-separated or repeatable)
      --preset <name>          Preset: standard, local, public, strict, ai-only, suspicious, proxy-only, http, ssh, minecraft, all
  -d, --domain <domains>       Downstream domain(s), comma-separated
  -u, --upstream <url>         Upstream target URL (e.g. http://localhost:3000 or :3000)
      --site <domain=url>      Add a site entry (repeatable)
      --ssh                    Enable SSH protection proxy (default port 22)
      --ssh-port <port>        SSH listen port (default 22)
      --ssh-upstream <addr>    Explicit sshd upstream (skips automatic sshd_config changes)
      --sshd-config <path>     Path to sshd_config (default /etc/ssh/sshd_config)
      --minecraft, --mc        Enable Minecraft protection proxy (default port 25565)
      --mc-port <port>         Minecraft listen port (default 25565)
      --mc-upstream <addr>     Explicit Minecraft upstream (skips automatic server.properties changes)
      --server-properties <p>  Path to Minecraft server.properties (default server.properties)
      --mc-jar, --server-jar   Path to Minecraft server .jar (auto-starts server & selects Java automatically)
      --mc-java, --java <bin>  Explicit Java binary path for Minecraft server (defaults to auto-select)
      --mc-jvm-args <args>     Space-separated JVM flags (e.g. "-Xms1G -Xmx4G")
      --mc-work-dir <dir>      Working directory for managed Minecraft server (defaults to JAR directory)
      --mc-console             Bring managed Minecraft server console into Gymbag foreground & Admin API
      --verify-url <url>       Public HTTPS verify portal URL for SSH/Minecraft (defaults to server IP + IP SSL cert)
      --verify-port <port>     Shared HTTPS verify portal port for SSH/Minecraft (default 8443)
      --authelia <url>         Enable Authelia forward auth using this URL (e.g. http://127.0.0.1:9091)
      --authelia-uri <path>    Authelia verification URI (default /api/authz/forward-auth)
      --authelia-portal <url>  Authelia portal URL for browser 401 redirect / rd=
      --no-auto-setup          Skip automatic sshd_config/firewall and server.properties modifications
      --no-firewall            Skip automatic firewall setup when configuring SSH protection
  -l, --verify <0-3|off>       Verify level (0=always, 1=suspicious, 2=ai, 3=suspicious & ai, off=disable)
      --difficulty <1-5>           SHA-256 PoW difficulty (default 4)
      --verify-timeout <duration>  Verified clearance timeout (e.g. 30m, 12h, 7d, or seconds)
      --remote-rules <url>         Remote firewall rules URL (repeatable or comma-separated)
      --remote-rules-refresh <d>   Refresh interval for remote rules (default 5m)
      --rules-file <path>          Dedicated firewall rules file path (repeatable)
      --rules-dir <path>           Directory of firewall rule files (repeatable)
      --rules-dynamic              Auto-reload --rules-file / --rules-dir when files change
      --rules-watch-interval <d>   Polling interval for dynamic local rule files/folders (default 2s)
      --no-protect                 Disable WAF verification (protect off)
      --ssl-provider <name>    SSL/ACME provider (letsencrypt, zerossl, google, ssl.com, litessl, buypass, local, URL)
      --email <addr>           ACME contact email
      --eab-kid <id>           ACME EAB Key ID
      --eab-hmac-key <key>     ACME EAB HMAC key
      --http-port <port>       HTTP redirect/ACME port (default 80)
      --https-port <port>      HTTPS gateway port (default 443)
      --bind <ip>              Bind address (auto-selected when omitted)
      --admin <addr>           Enable local REST Admin API (e.g. 127.0.0.1:2019)
      --cert-dir <dir>         Certificate & intel cache directory (default certs)
      --redis <url>            Optional Redis URL
      --ip-security-action <a> Action for threat-intel IP hits: verify or block
      --no-default-rules       Disable built-in baseline firewall rules
      --no-auto-hosts          Disable automatic /etc/hosts entries for local domains
  -f, --force                  Overwrite existing config file
  -y, --yes                    Non-interactive mode (use defaults for omitted flags)
  -i, --interactive            Force interactive setup wizard
      --dry-run                Print generated config to stdout without writing a file
      --service <type>         Also register a system service (auto, systemd, launchd, openrc, supervisor, freebsd-rc, openbsd-rc, netbsd-rc)
      --service-name <name>    Service name when --service is used (default gymbag)
      --service-user <user>    Run-as user for system-level service
      --user                   Register user-level service
      --system                 Register system-wide service
      --start                  Start registered service immediately
`)
}
