package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gymbag/internal/config"
	"gymbag/internal/host"
)

func resolveMutableConfigPath(explicit string) (string, error) {
	if t := strings.TrimSpace(explicit); t != "" {
		return t, nil
	}
	if p := resolveWatchedPath(""); p != "" {
		return p, nil
	}
	return "", fmt.Errorf(`no config file found (gymbag.json, Gymbagfile, or Caddyfile); run "gymbag init" first or pass --config <path>`)
}

func detectConfigFormatFromFile(path string, raw []byte) string {
	lower := strings.ToLower(strings.TrimSpace(path))
	if strings.HasSuffix(lower, ".json") {
		return "json"
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '{' && json.Valid(trimmed) {
		return "json"
	}
	return "gymbagfile"
}

func loadConfigForEdit(explicitPath string) (string, config.Config, string, error) {
	path, err := resolveMutableConfigPath(explicitPath)
	if err != nil {
		return "", config.Config{}, "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", config.Config{}, "", fmt.Errorf("failed to read %s: %w", path, err)
	}
	cfg, err := config.ParseFile(path)
	if err != nil {
		return "", config.Config{}, "", err
	}
	return path, cfg, detectConfigFormatFromFile(path, raw), nil
}

func writeFileAtomically(path string, data []byte, defaultPerm os.FileMode) error {
	perm := defaultPerm
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".gymbag-edit-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func saveConfigForEdit(path string, cfg config.Config, format string) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	var out []byte
	if format == "json" {
		raw, err := config.MarshalJSONConfig(cfg)
		if err != nil {
			return err
		}
		out = append(raw, '\n')
	} else {
		out = []byte(config.FormatConfig(cfg))
	}
	return writeFileAtomically(path, out, 0o644)
}

func runConfigCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printConfigUsage(stdout)
		return 0
	}
	sub := strings.ToLower(strings.TrimSpace(args[0]))
	switch sub {
	case "help", "-h", "--help":
		printConfigUsage(stdout)
		return 0
	case "show", "print":
		return runConfigShow(args[1:], stdout, stderr)
	case "list", "ls":
		return runConfigList(args[1:], stdout, stderr)
	case "add":
		return runConfigAddSite(args[1:], stdout, stderr)
	case "update":
		return runConfigUpdateSite(args[1:], stdout, stderr)
	case "remove", "rm", "delete":
		return runConfigRemoveSite(args[1:], stdout, stderr)
	case "site", "sites":
		return runConfigSiteSub(args[1:], stdout, stderr)
	case "route", "routes":
		return runConfigRouteCmd(args[1:], stdout, stderr)
	case "set":
		return runConfigSetGlobal(args[1:], stdout, stderr)
	case "unset":
		return runConfigUnsetGlobal(args[1:], stdout, stderr)
	case "convert":
		return runConfigConvert(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config: unknown subcommand %q\n\n", args[0])
		printConfigUsage(stderr)
		return 2
	}
}

func runConfigSiteSub(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return runConfigList(nil, stdout, stderr)
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "list", "ls":
		return runConfigList(args[1:], stdout, stderr)
	case "add":
		return runConfigAddSite(args[1:], stdout, stderr)
	case "update":
		return runConfigUpdateSite(args[1:], stdout, stderr)
	case "remove", "rm", "delete":
		return runConfigRemoveSite(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config site: unknown subcommand %q (expected list, add, update, remove)\n", args[0])
		return 2
	}
}

func runConfigShow(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gymbag config show", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfgPath, outFmt string
	fs.StringVar(&cfgPath, "c", "", "Path to config file")
	fs.StringVar(&cfgPath, "config", "", "Path to config file")
	fs.StringVar(&outFmt, "format", "", "Output format: json or gymbagfile (default matches input)")
	if err := parseInterspersedFlags(fs, args); err != nil {
		return 2
	}
	if cfgPath == "" && fs.NArg() > 0 {
		cfgPath = fs.Arg(0)
	}
	_, cfg, detectedFmt, err := loadConfigForEdit(cfgPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config show: %v\n", err)
		return 1
	}
	targetFmt := detectedFmt
	if strings.TrimSpace(outFmt) != "" {
		norm, nErr := config.NormalizeConfigFormat(outFmt)
		if nErr != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag config show: %v\n", nErr)
			return 2
		}
		targetFmt = norm
	}
	if targetFmt == "json" {
		raw, mErr := config.MarshalJSONConfig(cfg)
		if mErr != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag config show: %v\n", mErr)
			return 1
		}
		_, _ = stdout.Write(append(raw, '\n'))
		return 0
	}
	_, _ = io.WriteString(stdout, config.FormatConfig(cfg))
	return 0
}

func runConfigList(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gymbag config list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfgPath string
	fs.StringVar(&cfgPath, "c", "", "Path to config file")
	fs.StringVar(&cfgPath, "config", "", "Path to config file")
	if err := parseInterspersedFlags(fs, args); err != nil {
		return 2
	}
	if cfgPath == "" && fs.NArg() > 0 {
		cfgPath = fs.Arg(0)
	}
	path, cfg, _, err := loadConfigForEdit(cfgPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config list: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "%s (%d site(s)):\n", path, len(cfg.Sites))
	for _, s := range cfg.Sites {
		target := strings.Join(s.Upstreams, ", ")
		if target == "" {
			target = fmt.Sprintf("%d route(s)", len(s.Routes))
		} else if len(s.Routes) > 0 {
			target = fmt.Sprintf("%s (+%d route(s))", target, len(s.Routes))
		}
		prot := fmt.Sprintf("verify %d [%s]", int(s.WAF.VerifyLevel), s.WAF.VerifyLevel.Label())
		if !s.Protect {
			prot = "protect off"
		}
		_, _ = fmt.Fprintf(stdout, "  - %s -> %s (%s, rules=%d, rule_sources=%d)\n",
			s.Domain, target, prot, len(s.WAF.Rules), len(s.WAF.RuleSources)+len(s.WAF.RemoteRules))
	}
	return 0
}

func splitCommaFlags(vals []string) []string {
	var out []string
	for _, v := range vals {
		for _, part := range strings.Split(v, ",") {
			if t := strings.TrimSpace(part); t != "" {
				out = append(out, t)
			}
		}
	}
	return out
}

func parseHeaderFlag(raw string) (string, string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", fmt.Errorf("header must not be empty")
	}
	if strings.HasPrefix(trimmed, "-") && !strings.ContainsAny(trimmed, "=:") {
		return trimmed, "", nil
	}
	if k, v, ok := strings.Cut(trimmed, "="); ok {
		return strings.TrimSpace(k), strings.TrimSpace(v), nil
	}
	if k, v, ok := strings.Cut(trimmed, ":"); ok {
		return strings.TrimSpace(k), strings.TrimSpace(v), nil
	}
	return "", "", fmt.Errorf("invalid --header %q (expected Name=Value or -Name)", raw)
}

func runConfigAddSite(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gymbag config add", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		cfgPath        string
		domain         string
		upstreams      multiStringFlag
		verifyLevel    string
		difficulty     int
		verifyTimeout  string
		noProtect      bool
		noDefaultRules bool
		certFile       string
		keyFile        string
		rootDir        string
		browse         bool
		fallback       string
		respondBody    string
		respondStatus  int
		redirTo        string
		redirCode      int
		headerFlags    multiStringFlag
		force          bool
	)
	fs.StringVar(&cfgPath, "c", "", "Path to config file")
	fs.StringVar(&cfgPath, "config", "", "Path to config file")
	fs.StringVar(&domain, "d", "", "Domain name(s), comma-separated")
	fs.StringVar(&domain, "domain", "", "Domain name(s), comma-separated")
	fs.Var(&upstreams, "u", "Upstream URL (repeatable or comma-separated)")
	fs.Var(&upstreams, "upstream", "Upstream URL (repeatable or comma-separated)")
	fs.StringVar(&verifyLevel, "l", "", "Verify level (0..3 or off)")
	fs.StringVar(&verifyLevel, "verify", "", "Verify level (0..3 or off)")
	fs.IntVar(&difficulty, "difficulty", 0, "SHA-256 PoW difficulty (1-5)")
	fs.StringVar(&verifyTimeout, "verify-timeout", "", "Verified clearance timeout (e.g. 30m, 12h)")
	fs.BoolVar(&noProtect, "no-protect", false, "Disable WAF verification (protect off)")
	fs.BoolVar(&noDefaultRules, "no-default-rules", false, "Disable built-in baseline rules")
	fs.StringVar(&certFile, "cert", "", "Per-site TLS certificate path")
	fs.StringVar(&keyFile, "key", "", "Per-site TLS private key path")
	fs.StringVar(&rootDir, "root", "", "Static file root directory")
	fs.BoolVar(&browse, "browse", false, "Enable directory listing when --root is used")
	fs.StringVar(&fallback, "fallback", "", "Fallback file for SPA static routes (e.g. /index.html)")
	fs.StringVar(&respondBody, "respond", "", "Static response body")
	fs.IntVar(&respondStatus, "status", 0, "Static response HTTP status code")
	fs.StringVar(&redirTo, "redir", "", "Redirect target URL")
	fs.IntVar(&redirCode, "code", 0, "Redirect HTTP status code (default 302)")
	fs.Var(&headerFlags, "header", "Response header mutation Name=Value or -Name (repeatable)")
	fs.BoolVar(&force, "f", false, "Overwrite existing site with the same domain")
	fs.BoolVar(&force, "force", false, "Overwrite existing site with the same domain")
	if err := parseInterspersedFlags(fs, args); err != nil {
		return 2
	}
	if domain == "" && fs.NArg() > 0 {
		domain = fs.Arg(0)
	}
	if strings.TrimSpace(domain) == "" {
		_, _ = fmt.Fprintln(stderr, "ERROR gymbag config add: --domain is required")
		return 2
	}

	path, cfg, format, err := loadConfigForEdit(cfgPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config add: %v\n", err)
		return 1
	}

	var normUpstreams []string
	for _, raw := range splitCommaFlags(upstreams) {
		u, uErr := config.NormalizeUpstream(raw, "upstream")
		if uErr != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag config add: %v\n", uErr)
			return 1
		}
		normUpstreams = append(normUpstreams, u)
	}

	var routes []config.RouteConfig
	if rootDir != "" {
		routes = append(routes, config.RouteConfig{
			Root:     rootDir,
			Browse:   browse,
			Fallback: fallback,
		})
	} else if respondBody != "" || respondStatus != 0 {
		st := respondStatus
		if st == 0 {
			st = 200
		}
		routes = append(routes, config.RouteConfig{
			Respond: &config.StaticResponse{Status: st, Body: respondBody},
		})
	} else if redirTo != "" {
		rc := redirCode
		if rc == 0 {
			rc = 302
		}
		routes = append(routes, config.RouteConfig{
			Redirect: &config.RedirectTarget{To: redirTo, Code: rc},
		})
	}

	var hdrs map[string]string
	for _, rawH := range headerFlags {
		k, v, hErr := parseHeaderFlag(rawH)
		if hErr != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag config add: %v\n", hErr)
			return 2
		}
		if hdrs == nil {
			hdrs = make(map[string]string)
		}
		hdrs[k] = v
	}

	var domains []string
	for _, part := range strings.Split(domain, ",") {
		if d := host.Normalize(part); d != "" {
			domains = append(domains, d)
		}
	}
	if len(domains) == 0 {
		_, _ = fmt.Fprintln(stderr, "ERROR gymbag config add: at least one valid domain is required")
		return 2
	}

	for _, d := range domains {
		waf := cloneWAFForCLI(cfg.Defaults)
		prot := true
		if verifyLevel != "" {
			lvl, p, vErr := config.ParseVerifyLevel(verifyLevel)
			if vErr != nil {
				_, _ = fmt.Fprintf(stderr, "ERROR gymbag config add: %v\n", vErr)
				return 1
			}
			waf.VerifyLevel = lvl
			prot = p
		} else {
			waf.VerifyLevel = config.VerifyLevelSuspiciousAndAI
		}
		if noProtect {
			prot = false
		}
		if difficulty != 0 {
			waf.Difficulty = difficulty
		}
		if verifyTimeout != "" {
			ttl, tErr := config.ParseDurationSeconds(verifyTimeout, "verify-timeout")
			if tErr != nil {
				_, _ = fmt.Fprintf(stderr, "ERROR gymbag config add: %v\n", tErr)
				return 1
			}
			waf.ClearanceTTL = ttl
		}
		if noDefaultRules {
			f := false
			waf.DefaultRules = &f
		}

		site := config.SiteConfig{
			Domain:    d,
			Upstreams: append([]string(nil), normUpstreams...),
			Routes:    routes,
			Headers:   hdrs,
			Protect:   prot,
			WAF:       waf,
			CertFile:  certFile,
			KeyFile:   keyFile,
		}

		existingIdx := -1
		for i, existing := range cfg.Sites {
			if existing.Domain == d {
				existingIdx = i
				break
			}
		}
		if existingIdx != -1 {
			if !force {
				_, _ = fmt.Fprintf(stderr, "ERROR gymbag config add: site %q already exists (use update or --force)\n", d)
				return 1
			}
			cfg.Sites[existingIdx] = site
		} else {
			cfg.Sites = append(cfg.Sites, site)
		}
	}

	if err := saveConfigForEdit(path, cfg, format); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config add: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "Added %s to %s\n", strings.Join(domains, ", "), path)
	return 0
}

func cloneWAFForCLI(in config.SiteWAFConfig) config.SiteWAFConfig {
	out := in
	if in.DefaultRules != nil {
		v := *in.DefaultRules
		out.DefaultRules = &v
	}
	out.Rules = make([]config.CustomRule, len(in.Rules))
	for i, r := range in.Rules {
		out.Rules[i] = r.Clone()
	}
	out.RemoteRules = append([]string(nil), in.RemoteRules...)
	out.RuleSources = append([]config.RuleSource(nil), in.RuleSources...)
	out.CustomAIBots = append([]string(nil), in.CustomAIBots...)
	out.CustomSuspiciousUAs = append([]string(nil), in.CustomSuspiciousUAs...)
	out.CustomSuspiciousPaths = append([]string(nil), in.CustomSuspiciousPaths...)
	return out
}

func runConfigUpdateSite(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gymbag config update", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		cfgPath       string
		domain        string
		upstreams     multiStringFlag
		verifyLevel   string
		difficulty    int
		verifyTimeout string
		protectStr    string
		defRulesStr   string
		certFile      string
		keyFile       string
		clearTLS      bool
		headerFlags   multiStringFlag
		removeHeaders multiStringFlag
	)
	fs.StringVar(&cfgPath, "c", "", "Path to config file")
	fs.StringVar(&cfgPath, "config", "", "Path to config file")
	fs.StringVar(&domain, "d", "", "Domain name to update")
	fs.StringVar(&domain, "domain", "", "Domain name to update")
	fs.Var(&upstreams, "u", "Replace upstream URL(s) (repeatable or comma-separated)")
	fs.Var(&upstreams, "upstream", "Replace upstream URL(s) (repeatable or comma-separated)")
	fs.StringVar(&verifyLevel, "l", "", "Verify level (0..3 or off)")
	fs.StringVar(&verifyLevel, "verify", "", "Verify level (0..3 or off)")
	fs.IntVar(&difficulty, "difficulty", 0, "SHA-256 PoW difficulty (1-5)")
	fs.StringVar(&verifyTimeout, "verify-timeout", "", "Verified clearance timeout (e.g. 30m, 12h)")
	fs.StringVar(&protectStr, "protect", "", "Enable or disable WAF protection (on/off)")
	fs.StringVar(&defRulesStr, "default-rules", "", "Enable or disable built-in baseline rules (on/off)")
	fs.StringVar(&certFile, "cert", "", "Per-site TLS certificate path")
	fs.StringVar(&keyFile, "key", "", "Per-site TLS private key path")
	fs.BoolVar(&clearTLS, "clear-tls", false, "Clear per-site TLS cert and key")
	fs.Var(&headerFlags, "header", "Set/delete response header Name=Value or -Name (repeatable)")
	fs.Var(&removeHeaders, "remove-header", "Remove a configured site response header mutation by key")
	if err := parseInterspersedFlags(fs, args); err != nil {
		return 2
	}
	if domain == "" && fs.NArg() > 0 {
		domain = fs.Arg(0)
	}
	targetDomain := host.Normalize(domain)
	if targetDomain == "" {
		_, _ = fmt.Fprintln(stderr, "ERROR gymbag config update: --domain is required")
		return 2
	}

	path, cfg, format, err := loadConfigForEdit(cfgPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config update: %v\n", err)
		return 1
	}

	idx := -1
	for i, s := range cfg.Sites {
		if s.Domain == targetDomain {
			idx = i
			break
		}
	}
	if idx == -1 {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config update: site %q not found\n", targetDomain)
		return 1
	}

	s := cfg.Sites[idx].Clone()
	if len(upstreams) > 0 {
		var norm []string
		for _, raw := range splitCommaFlags(upstreams) {
			u, uErr := config.NormalizeUpstream(raw, "upstream")
			if uErr != nil {
				_, _ = fmt.Fprintf(stderr, "ERROR gymbag config update: %v\n", uErr)
				return 1
			}
			norm = append(norm, u)
		}
		s.Upstreams = norm
	}
	if verifyLevel != "" {
		lvl, prot, vErr := config.ParseVerifyLevel(verifyLevel)
		if vErr != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag config update: %v\n", vErr)
			return 1
		}
		s.WAF.VerifyLevel = lvl
		s.Protect = prot
	}
	if protectStr != "" {
		b, bErr := parseCLIPositiveBool(protectStr, "protect")
		if bErr != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag config update: %v\n", bErr)
			return 1
		}
		s.Protect = b
	}
	if difficulty != 0 {
		s.WAF.Difficulty = difficulty
	}
	if verifyTimeout != "" {
		ttl, tErr := config.ParseDurationSeconds(verifyTimeout, "verify-timeout")
		if tErr != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag config update: %v\n", tErr)
			return 1
		}
		s.WAF.ClearanceTTL = ttl
	}
	if defRulesStr != "" {
		b, bErr := parseCLIPositiveBool(defRulesStr, "default-rules")
		if bErr != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag config update: %v\n", bErr)
			return 1
		}
		s.WAF.DefaultRules = &b
	}
	if clearTLS {
		s.CertFile = ""
		s.KeyFile = ""
	} else {
		if certFile != "" {
			s.CertFile = certFile
		}
		if keyFile != "" {
			s.KeyFile = keyFile
		}
	}
	if len(headerFlags) > 0 {
		if s.Headers == nil {
			s.Headers = make(map[string]string)
		}
		for _, rawH := range headerFlags {
			k, v, hErr := parseHeaderFlag(rawH)
			if hErr != nil {
				_, _ = fmt.Fprintf(stderr, "ERROR gymbag config update: %v\n", hErr)
				return 2
			}
			s.Headers[k] = v
		}
	}
	for _, rk := range removeHeaders {
		if s.Headers != nil {
			delete(s.Headers, strings.TrimSpace(rk))
		}
	}

	cfg.Sites[idx] = s
	if err := saveConfigForEdit(path, cfg, format); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config update: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "Updated %s in %s\n", targetDomain, path)
	return 0
}

func runConfigRemoveSite(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gymbag config remove", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfgPath, domain string
	fs.StringVar(&cfgPath, "c", "", "Path to config file")
	fs.StringVar(&cfgPath, "config", "", "Path to config file")
	fs.StringVar(&domain, "d", "", "Domain name to remove")
	fs.StringVar(&domain, "domain", "", "Domain name to remove")
	if err := parseInterspersedFlags(fs, args); err != nil {
		return 2
	}
	if domain == "" && fs.NArg() > 0 {
		domain = fs.Arg(0)
	}
	targetDomain := host.Normalize(domain)
	if targetDomain == "" {
		_, _ = fmt.Fprintln(stderr, "ERROR gymbag config remove: --domain is required")
		return 2
	}

	path, cfg, format, err := loadConfigForEdit(cfgPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config remove: %v\n", err)
		return 1
	}
	idx := -1
	for i, s := range cfg.Sites {
		if s.Domain == targetDomain {
			idx = i
			break
		}
	}
	if idx == -1 {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config remove: site %q not found\n", targetDomain)
		return 1
	}
	if len(cfg.Sites) <= 1 {
		_, _ = fmt.Fprintln(stderr, "ERROR gymbag config remove: cannot remove the last remaining site")
		return 1
	}
	cfg.Sites = append(cfg.Sites[:idx], cfg.Sites[idx+1:]...)
	if err := saveConfigForEdit(path, cfg, format); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config remove: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "Removed %s from %s\n", targetDomain, path)
	return 0
}

func runConfigRouteCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "Usage: gymbag config route <list|add|update|remove> [flags]")
		return 2
	}
	sub := strings.ToLower(strings.TrimSpace(args[0]))
	fs := flag.NewFlagSet("gymbag config route "+sub, flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		cfgPath     string
		domain      string
		routePath   string
		stripPrefix string
		upstreams   multiStringFlag
		rootDir     string
		browse      bool
		fallback    string
		respondBody string
		status      int
		redirTo     string
		code        int
		headerFlags multiStringFlag
		indexFlag   int
	)
	indexFlag = -1
	fs.StringVar(&cfgPath, "c", "", "Path to config file")
	fs.StringVar(&cfgPath, "config", "", "Path to config file")
	fs.StringVar(&domain, "d", "", "Site domain (defaults to first site)")
	fs.StringVar(&domain, "domain", "", "Site domain (defaults to first site)")
	fs.StringVar(&routePath, "path", "", "Route path pattern (e.g. /api/*, ~^/v[0-9]+/, or empty for catch-all)")
	fs.StringVar(&stripPrefix, "strip-prefix", "", "Prefix to strip before proxying/serving")
	fs.Var(&upstreams, "u", "Upstream URL(s)")
	fs.Var(&upstreams, "upstream", "Upstream URL(s)")
	fs.StringVar(&rootDir, "root", "", "Static files root directory")
	fs.BoolVar(&browse, "browse", false, "Allow directory listing")
	fs.StringVar(&fallback, "fallback", "", "SPA fallback file (e.g. /index.html)")
	fs.StringVar(&respondBody, "respond", "", "Static response body")
	fs.IntVar(&status, "status", 0, "Static response status code")
	fs.StringVar(&redirTo, "redir", "", "Redirect target URL")
	fs.IntVar(&code, "code", 0, "Redirect HTTP status code")
	fs.Var(&headerFlags, "header", "Response header mutation Name=Value or -Name")
	fs.IntVar(&indexFlag, "index", -1, "0-based route index for update/remove")
	if err := parseInterspersedFlags(fs, args[1:]); err != nil {
		return 2
	}

	path, cfg, format, err := loadConfigForEdit(cfgPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config route %s: %v\n", sub, err)
		return 1
	}

	siteIdx := 0
	if domain != "" {
		norm := host.Normalize(domain)
		siteIdx = -1
		for i, s := range cfg.Sites {
			if s.Domain == norm {
				siteIdx = i
				break
			}
		}
		if siteIdx == -1 {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag config route %s: site %q not found\n", sub, norm)
			return 1
		}
	}

	site := cfg.Sites[siteIdx].Clone()
	switch sub {
	case "list", "ls":
		_, _ = fmt.Fprintf(stdout, "%s routes (%d):\n", site.Domain, len(site.Routes))
		for i, rc := range site.Routes {
			p := rc.Path
			if p == "" {
				p = "*"
			}
			_, _ = fmt.Fprintf(stdout, "  [%d] %s -> %s\n", i, p, rc.HandlerKind())
		}
		return 0

	case "add", "update":
		var hdrs map[string]string
		for _, rawH := range headerFlags {
			k, v, hErr := parseHeaderFlag(rawH)
			if hErr != nil {
				_, _ = fmt.Fprintf(stderr, "ERROR gymbag config route %s: %v\n", sub, hErr)
				return 2
			}
			if hdrs == nil {
				hdrs = make(map[string]string)
			}
			hdrs[k] = v
		}
		rc := config.RouteConfig{
			Path:        routePath,
			StripPrefix: stripPrefix,
			Headers:     hdrs,
		}
		if len(upstreams) > 0 {
			for _, raw := range splitCommaFlags(upstreams) {
				u, uErr := config.NormalizeUpstream(raw, "upstream")
				if uErr != nil {
					_, _ = fmt.Fprintf(stderr, "ERROR gymbag config route %s: %v\n", sub, uErr)
					return 1
				}
				rc.Upstreams = append(rc.Upstreams, u)
			}
		} else if rootDir != "" {
			rc.Root = rootDir
			rc.Browse = browse
			rc.Fallback = fallback
		} else if respondBody != "" || status != 0 {
			st := status
			if st == 0 {
				st = 200
			}
			rc.Respond = &config.StaticResponse{Status: st, Body: respondBody}
		} else if redirTo != "" {
			c := code
			if c == 0 {
				c = 302
			}
			rc.Redirect = &config.RedirectTarget{To: redirTo, Code: c}
		}

		if sub == "add" {
			if err := rc.Validate(); err != nil {
				_, _ = fmt.Fprintf(stderr, "ERROR gymbag config route add: %v\n", err)
				return 1
			}
			site.Routes = append(site.Routes, rc)
		} else {
			targetIdx := indexFlag
			if targetIdx == -1 {
				for i, existing := range site.Routes {
					if existing.Path == strings.TrimSpace(routePath) {
						targetIdx = i
						break
					}
				}
			}
			if targetIdx < 0 || targetIdx >= len(site.Routes) {
				_, _ = fmt.Fprintln(stderr, "ERROR gymbag config route update: matching route not found (pass --index or exact --path)")
				return 1
			}
			merged := site.Routes[targetIdx].Clone()
			if routePath != "" {
				merged.Path = routePath
			}
			if stripPrefix != "" {
				merged.StripPrefix = stripPrefix
			}
			if len(rc.Upstreams) > 0 {
				merged.Upstreams = rc.Upstreams
				merged.Root = ""
				merged.Respond = nil
				merged.Redirect = nil
			} else if rc.Root != "" {
				merged.Root = rc.Root
				merged.Browse = rc.Browse
				merged.Fallback = rc.Fallback
				merged.Upstreams = nil
				merged.Respond = nil
				merged.Redirect = nil
			} else if rc.Respond != nil {
				merged.Respond = rc.Respond
				merged.Upstreams = nil
				merged.Root = ""
				merged.Redirect = nil
			} else if rc.Redirect != nil {
				merged.Redirect = rc.Redirect
				merged.Upstreams = nil
				merged.Root = ""
				merged.Respond = nil
			}
			if len(hdrs) > 0 {
				if merged.Headers == nil {
					merged.Headers = make(map[string]string)
				}
				for k, v := range hdrs {
					merged.Headers[k] = v
				}
			}
			site.Routes[targetIdx] = merged
		}

	case "remove", "rm", "delete":
		targetIdx := indexFlag
		if targetIdx == -1 {
			for i, existing := range site.Routes {
				if existing.Path == strings.TrimSpace(routePath) {
					targetIdx = i
					break
				}
			}
		}
		if targetIdx < 0 || targetIdx >= len(site.Routes) {
			_, _ = fmt.Fprintln(stderr, "ERROR gymbag config route remove: matching route not found (pass --index or exact --path)")
			return 1
		}
		site.Routes = append(site.Routes[:targetIdx], site.Routes[targetIdx+1:]...)

	default:
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config route: unknown subcommand %q\n", sub)
		return 2
	}

	cfg.Sites[siteIdx] = site
	if err := saveConfigForEdit(path, cfg, format); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config route %s: %v\n", sub, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "Updated routes for %s in %s\n", site.Domain, path)
	return 0
}

func runConfigSetGlobal(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gymbag config set", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfgPath string
	fs.StringVar(&cfgPath, "c", "", "Path to config file")
	fs.StringVar(&cfgPath, "config", "", "Path to config file")
	if err := parseInterspersedFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		_, _ = fmt.Fprintln(stderr, "Usage: gymbag config set [-c <file>] <key> <value>")
		return 2
	}
	key := strings.ToLower(strings.TrimSpace(fs.Arg(0)))
	val := strings.TrimSpace(fs.Arg(1))

	path, cfg, format, err := loadConfigForEdit(cfgPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config set: %v\n", err)
		return 1
	}
	if err := applyGlobalKeySet(&cfg, key, val); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config set: %v\n", err)
		return 1
	}
	if err := saveConfigForEdit(path, cfg, format); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config set: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "Set %s = %s in %s\n", key, val, path)
	return 0
}

func runConfigUnsetGlobal(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gymbag config unset", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfgPath string
	fs.StringVar(&cfgPath, "c", "", "Path to config file")
	fs.StringVar(&cfgPath, "config", "", "Path to config file")
	if err := parseInterspersedFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintln(stderr, "Usage: gymbag config unset [-c <file>] <key>")
		return 2
	}
	key := strings.ToLower(strings.TrimSpace(fs.Arg(0)))

	path, cfg, format, err := loadConfigForEdit(cfgPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config unset: %v\n", err)
		return 1
	}
	if err := applyGlobalKeyUnset(&cfg, key); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config unset: %v\n", err)
		return 1
	}
	if err := saveConfigForEdit(path, cfg, format); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config unset: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "Unset %s in %s\n", key, path)
	return 0
}

func applyGlobalKeySet(cfg *config.Config, key, val string) error {
	switch key {
	case "email", "acme_email":
		cfg.TLS.ACMEEmail = val
	case "ssl_provider", "acme_provider", "ca":
		prov, mode, customURL, err := config.NormalizeSSLProvider(val)
		if err != nil {
			return err
		}
		cfg.TLS.Provider = prov
		cfg.TLS.Mode = mode
		if customURL != "" {
			cfg.TLS.ACMEDirectory = customURL
		}
	case "http_port":
		p, err := host.ParsePort(val, "http_port")
		if err != nil {
			return err
		}
		cfg.HTTPPort = p
	case "https_port":
		p, err := host.ParsePort(val, "https_port")
		if err != nil {
			return err
		}
		cfg.HTTPSPort = p
	case "bind", "bind_host":
		cfg.BindHost = val
	case "admin", "admin_addr":
		if strings.EqualFold(val, "off") || strings.EqualFold(val, "none") {
			cfg.AdminAddr = ""
		} else {
			cfg.AdminAddr = val
		}
	case "cert_dir":
		cfg.TLS.CertDir = val
	case "redis", "redis_url":
		cfg.RedisURL = val
	case "ip_security":
		b, err := parseCLIPositiveBool(val, key)
		if err != nil {
			return err
		}
		cfg.Intel.IPSecurity = b
	case "ip_security_action":
		cfg.Intel.IPSecurityAction = val
	case "ip_reputation", "live_reputation":
		b, err := parseCLIPositiveBool(val, key)
		if err != nil {
			return err
		}
		cfg.Intel.LiveReputation = b
	case "ai_ip_list":
		b, err := parseCLIPositiveBool(val, key)
		if err != nil {
			return err
		}
		cfg.Intel.AIIPList = b
	case "intel_refresh":
		sec, err := config.ParseDurationSeconds(val, key)
		if err != nil {
			return err
		}
		cfg.Intel.RefreshInterval = sec
	case "remote_rules_refresh":
		sec, err := config.ParseDurationSeconds(val, key)
		if err != nil {
			return err
		}
		cfg.RemoteRulesRefresh = sec
	case "rules_watch_interval":
		sec, err := config.ParseDurationSeconds(val, key)
		if err != nil {
			return err
		}
		cfg.RulesWatchInterval = sec
	case "verify_timeout", "clearance_ttl":
		sec, err := config.ParseDurationSeconds(val, key)
		if err != nil {
			return err
		}
		cfg.Defaults.ClearanceTTL = sec
		for i := range cfg.Sites {
			cfg.Sites[i].WAF.ClearanceTTL = sec
		}
	case "auto_hosts":
		b, err := parseCLIPositiveBool(val, key)
		if err != nil {
			return err
		}
		cfg.TLS.AutoHosts = &b
	case "hsts":
		b, err := parseCLIPositiveBool(val, key)
		if err != nil {
			return err
		}
		cfg.TLS.HSTS = &b
	case "eab_kid":
		cfg.TLS.EABKID = val
	case "eab_hmac_key":
		cfg.TLS.EABHMACKey = val
	default:
		return fmt.Errorf("unsupported global key %q", key)
	}
	return nil
}

func applyGlobalKeyUnset(cfg *config.Config, key string) error {
	switch key {
	case "email", "acme_email":
		cfg.TLS.ACMEEmail = ""
	case "ssl_provider", "acme_provider", "ca":
		cfg.TLS.Provider = ""
		cfg.TLS.Mode = "auto"
		cfg.TLS.ACMEDirectory = config.DefaultTLSConfig().ACMEDirectory
	case "admin", "admin_addr":
		cfg.AdminAddr = ""
	case "redis", "redis_url":
		cfg.RedisURL = ""
	case "auto_hosts":
		cfg.TLS.AutoHosts = nil
	case "hsts":
		cfg.TLS.HSTS = nil
	case "eab_kid":
		cfg.TLS.EABKID = ""
	case "eab_hmac_key":
		cfg.TLS.EABHMACKey = ""
	case "cert_dir":
		cfg.TLS.CertDir = config.DefaultTLSConfig().CertDir
	case "http_port":
		cfg.HTTPPort = 80
	case "https_port":
		cfg.HTTPSPort = 443
	case "rules_watch_interval":
		cfg.RulesWatchInterval = config.DefaultRulesWatchInterval
	case "remote_rules_refresh":
		cfg.RemoteRulesRefresh = config.DefaultRemoteRulesRefresh
	default:
		return fmt.Errorf("unsupported unset key %q", key)
	}
	return nil
}

func runConfigConvert(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gymbag config convert", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var inPath, outPath, format string
	var force bool
	fs.StringVar(&inPath, "c", "", "Input config file (Gymbagfile, Caddyfile, nginx.conf, httpd.conf, or JSON)")
	fs.StringVar(&inPath, "config", "", "Input config file")
	fs.StringVar(&outPath, "o", "", "Output file path (prints to stdout if omitted)")
	fs.StringVar(&outPath, "output", "", "Output file path")
	fs.StringVar(&format, "format", "json", "Output format: json or gymbagfile")
	fs.BoolVar(&force, "f", false, "Overwrite existing output file")
	fs.BoolVar(&force, "force", false, "Overwrite existing output file")
	if err := parseInterspersedFlags(fs, args); err != nil {
		return 2
	}
	if inPath == "" && fs.NArg() > 0 {
		inPath = fs.Arg(0)
	}
	if inPath == "" {
		var err error
		inPath, err = resolveMutableConfigPath("")
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag config convert: %v\n", err)
			return 1
		}
	}
	cfg, err := config.ParseFile(inPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config convert: %v\n", err)
		return 1
	}
	normFmt, err := config.NormalizeConfigFormat(format)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config convert: %v\n", err)
		return 2
	}
	var out []byte
	if normFmt == "json" {
		raw, mErr := config.MarshalJSONConfig(cfg)
		if mErr != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag config convert: %v\n", mErr)
			return 1
		}
		out = append(raw, '\n')
	} else {
		out = []byte(config.FormatConfig(cfg))
	}
	if outPath == "" {
		_, _ = stdout.Write(out)
		return 0
	}
	if !force {
		if _, stErr := os.Stat(outPath); stErr == nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag config convert: %s already exists (use --force to overwrite)\n", outPath)
			return 1
		}
	}
	if err := writeFileAtomically(outPath, out, 0o644); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag config convert: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "Converted %s -> %s (%s, %d site(s))\n", inPath, outPath, normFmt, len(cfg.Sites))
	return 0
}

func parseCLIPositiveBool(raw, name string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on", "enable", "enabled":
		return true, nil
	case "0", "false", "no", "off", "disable", "disabled":
		return false, nil
	default:
		if n, err := strconv.Atoi(raw); err == nil {
			return n != 0, nil
		}
		return false, fmt.Errorf("%s must be on/off or true/false", name)
	}
}

func printConfigUsage(w io.Writer) {
	_, _ = fmt.Fprintf(w, `Usage:
  gymbag config <subcommand> [flags]

Subcommands:
  list                   List configured sites
  show                   Print formatted config (JSON or Gymbagfile)
  add                    Add a new site (-d <domain> -u <upstream> | --root | --respond | --redir)
  update                 Update an existing site's upstreams, verify level, TLS, or headers
  remove                 Remove a site from the config
  route <cmd>            Manage path-matched routes on a site (list, add, update, remove)
  set <key> <value>      Set a global config option (email, ssl_provider, admin, http_port, ...)
  unset <key>            Reset a global config option
  convert                Convert nginx.conf, httpd.conf, Caddyfile, Gymbagfile, or JSON

Examples:
  gymbag config add -d api.example.com -u :8080 -l 1
  gymbag config update -d api.example.com -l 3 --verify-timeout 6h
  gymbag config remove -d old.example.com
  gymbag config route add -d app.example.com --path /static/* --strip-prefix /static --root ./dist
  gymbag config set admin 127.0.0.1:2019
  gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
`)
}
