package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gymbag/internal/config"
	"gymbag/internal/localpki"
	"gymbag/internal/service"
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
		outputPath       string
		format           string
		preset           string
		domain           string
		upstream         string
		siteFlags        multiStringFlag
		verifyLevel        string
		difficulty         int
		verifyTimeout      string
		remoteRules        multiStringFlag
		remoteRulesRefresh string
		noProtect          bool
		sslProvider      string
		email            string
		eabKID           string
		eabHMACKey       string
		httpPort         int
		httpsPort        int
		bindHost         string
		adminAddr        string
		certDir          string
		redisURL         string
		ipSecurityAction string
		noDefaultRules   bool
		noAutoHosts      bool
		force            bool
		yes              bool
		interactive      bool
		dryRun           bool
		serviceType      string
		serviceName      string
		serviceUser      string
		serviceUserScope bool
		serviceSysScope  bool
		serviceStart     bool
	)

	fs.StringVar(&outputPath, "o", "", "Output config file path (default Gymbagfile, or gymbag.json for --format json)")
	fs.StringVar(&outputPath, "output", "", "Output config file path (default Gymbagfile, or gymbag.json for --format json)")
	fs.StringVar(&outputPath, "c", "", "Alias for --output")
	fs.StringVar(&outputPath, "config", "", "Alias for --output")
	fs.StringVar(&format, "format", "gymbagfile", "Config format: gymbagfile or json")
	fs.StringVar(&preset, "preset", "", "Preset template: standard, local, public, strict, ai-only, suspicious, or proxy-only")
	fs.StringVar(&domain, "d", "", "Downstream domain(s), comma-separated")
	fs.StringVar(&domain, "domain", "", "Downstream domain(s), comma-separated")
	fs.StringVar(&upstream, "u", "", "Upstream URL (e.g. http://localhost:3000 or :3000)")
	fs.StringVar(&upstream, "upstream", "", "Upstream URL (e.g. http://localhost:3000 or :3000)")
	fs.Var(&siteFlags, "site", "Add a site as domain=upstream (repeatable)")
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
	fs.StringVar(&serviceType, "service", "", "Also register a system service (auto, systemd, launchd, openrc, supervisor)")
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

	normFmt, err := config.NormalizeConfigFormat(format)
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

	var parsedSites []config.SiteSpec
	for _, rawSite := range siteFlags {
		specs, sErr := config.ParseSiteSpec(rawSite)
		if sErr != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag init: %v\n", sErr)
			return 2
		}
		parsedSites = append(parsedSites, specs...)
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

	opts := config.GenerateOptions{
		Preset:           preset,
		Domain:           domain,
		Upstream:         upstream,
		Sites:            parsedSites,
		VerifyLevel:        verifyLevel,
		Protect:            protectOpt,
		Difficulty:         difficulty,
		ClearanceTTL:       verifyTimeout,
		RemoteRules:        remoteRules,
		RemoteRulesRefresh: remoteRulesRefresh,
		DefaultRules:       defaultRulesOpt,
		SSLProvider:      sslProvider,
		Email:            email,
		EABKID:           eabKID,
		EABHMACKey:       eabHMACKey,
		HTTPPort:         httpPort,
		HTTPSPort:        httpsPort,
		BindHost:         bindHost,
		AdminAddr:        adminAddr,
		CertDir:          certDir,
		RedisURL:         redisURL,
		AutoHosts:        autoHostsOpt,
		IPSecurityAction: ipSecurityAction,
		Format:           normFmt,
		IncludeComments:  true,
	}

	hasConfigFlags := preset != "" || domain != "" || upstream != "" || len(parsedSites) > 0 ||
		verifyLevel != "" || verifyTimeout != "" || len(remoteRules) > 0 || noProtect || sslProvider != "" || email != "" ||
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

		wizardSvc, wErr := runInitWizard(stdin, promptOut, &opts)
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
	siteSummaries := make([]string, 0, len(cfg.Sites))
	for _, s := range cfg.Sites {
		mode := fmt.Sprintf("verify: %d [%s]", int(s.WAF.VerifyLevel), s.WAF.VerifyLevel.Label())
		if !s.Protect {
			mode = "protect off"
		}
		siteSummaries = append(siteSummaries, fmt.Sprintf("%s -> %s (%s)", s.Domain, strings.Join(s.Upstreams, ", "), mode))
	}
	_, _ = fmt.Fprintf(stdout, "Generated %s (%d site(s): %s)\n", outputPath, len(cfg.Sites), strings.Join(siteSummaries, "; "))

	if strings.TrimSpace(serviceType) != "" {
		mgr, mErr := service.ParseManagerType(serviceType)
		if mErr != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag init: %v\n", mErr)
			return 1
		}
		userScope := resolveUserScope(mgr, serviceUserScope, serviceSysScope)
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
				PortsPrivileged: cfg.HTTPPort < 1024 || cfg.HTTPSPort < 1024,
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
	if outputPath == "Gymbagfile" {
		_, _ = fmt.Fprintln(stdout, "  Start server:     gymbag")
	} else {
		_, _ = fmt.Fprintf(stdout, "  Start server:     gymbag --config %s\n", outputPath)
	}
	_, _ = fmt.Fprintf(stdout, "  System service:   gymbag service install --config %s\n", outputPath)
	return 0
}

func runInitWizard(stdin io.Reader, out io.Writer, opts *config.GenerateOptions) (string, error) {
	r := bufio.NewReader(stdin)

	_, _ = fmt.Fprintln(out, "Gymbag config generator — press Enter to accept [default].")

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

	if opts.VerifyLevel == "" && opts.Protect == nil {
		ans, err := promptLine(r, out, "Verify level (0=always, 1=suspicious, 2=ai, 3=suspicious & ai, off=proxy only)", "3")
		if err != nil {
			return "", err
		}
		opts.VerifyLevel = ans
	}

	// Ask TLS/ACME questions only when at least one domain is public.
	var candidateDomains []string
	for _, part := range strings.Split(opts.Domain, ",") {
		if t := strings.TrimSpace(part); t != "" {
			candidateDomains = append(candidateDomains, t)
		}
	}
	for _, s := range opts.Sites {
		candidateDomains = append(candidateDomains, s.Domain)
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

	if opts.HTTPPort == 0 && opts.HTTPSPort == 0 {
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

	svcAns, err := promptLine(r, out, "Register system service? (none, auto, systemd, launchd, openrc, supervisor)", "none")
	if err != nil {
		return "", err
	}
	trimmedSvc := strings.ToLower(strings.TrimSpace(svcAns))
	if trimmedSvc == "" || trimmedSvc == "none" || trimmedSvc == "no" || trimmedSvc == "n" {
		return "", nil
	}
	return trimmedSvc, nil
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
  1. gymbag init -d app.example.com -u :3000
  2. gymbag

Examples:
  gymbag init                                         # Interactive wizard -> ./Gymbagfile
  gymbag init --yes                                   # Non-interactive default localhost config
  gymbag init --preset local                          # Local *.internal dev config with local PKI
  gymbag init -d app.example.com -u :3000 -l 3        # Public site config -> ./Gymbagfile
  gymbag init -d app.example.com -u :3000 --service auto  # Generate config + register system service

Flags:
  -o, --output <path>          Output config file path (default Gymbagfile, or gymbag.json for --format json)
      --format <fmt>           Config format: gymbagfile or json
      --preset <name>          Preset: standard, local, public, strict, ai-only, suspicious, proxy-only
  -d, --domain <domains>       Downstream domain(s), comma-separated
  -u, --upstream <url>         Upstream target URL (e.g. http://localhost:3000 or :3000)
      --site <domain=url>      Add a site entry (repeatable)
  -l, --verify <0-3|off>       Verify level (0=always, 1=suspicious, 2=ai, 3=suspicious & ai, off=disable)
      --difficulty <1-5>           SHA-256 PoW difficulty (default 4)
      --verify-timeout <duration>  Verified clearance timeout (e.g. 30m, 12h, 7d, or seconds)
      --remote-rules <url>         Remote firewall rules URL (repeatable or comma-separated)
      --remote-rules-refresh <d>   Refresh interval for remote rules (default 5m)
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
      --service <type>         Also register a system service (auto, systemd, launchd, openrc, supervisor)
      --service-name <name>    Service name when --service is used (default gymbag)
      --service-user <user>    Run-as user for system-level service
      --user                   Register user-level service
      --system                 Register system-wide service
      --start                  Start registered service immediately
`)
}
