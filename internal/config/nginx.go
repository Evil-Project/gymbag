package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gymbag/internal/host"
)

type nginxStmt struct {
	words []string
	block []nginxStmt
}

func isNginxConfig(text, path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if base == "nginx.conf" || strings.HasSuffix(base, ".nginx") || strings.HasSuffix(base, ".nginx.conf") {
		return true
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || strings.HasPrefix(trimmed, "{") {
		return false
	}
	// Strip comments for detection.
	var cleaned strings.Builder
	for _, line := range strings.Split(trimmed, "\n") {
		s := strings.TrimSpace(stripComment(line))
		if s != "" {
			cleaned.WriteString(s)
			cleaned.WriteByte('\n')
		}
	}
	body := cleaned.String()
	hasSemicolon := strings.Contains(body, ";")
	if !hasSemicolon {
		return false
	}
	if strings.Contains(body, "server_name ") || strings.Contains(body, "proxy_pass ") ||
		strings.Contains(body, "location ") || strings.Contains(body, "events {") ||
		strings.Contains(body, "http {") {
		return true
	}
	return false
}

func tokenizeNginx(text string) ([]string, error) {
	var tokens []string
	var cur strings.Builder
	inToken := false
	var quote rune
	escaped := false

	flush := func() {
		if inToken {
			tokens = append(tokens, cur.String())
			cur.Reset()
			inToken = false
		}
	}

	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		if escaped {
			cur.WriteRune(ch)
			inToken = true
			escaped = false
			continue
		}
		if quote != 0 {
			if ch == '\\' && quote == '"' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
				inToken = true
				continue
			}
			cur.WriteRune(ch)
			inToken = true
			continue
		}
		if ch == '#' && (i == 0 || runes[i-1] == '\n' || runes[i-1] == ' ' || runes[i-1] == '\t' || runes[i-1] == ';') {
			flush()
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
			continue
		}
		if ch == '\\' {
			escaped = true
			inToken = true
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			inToken = true
			continue
		}
		if ch == '{' || ch == '}' || ch == ';' {
			flush()
			tokens = append(tokens, string(ch))
			continue
		}
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			flush()
			continue
		}
		cur.WriteRune(ch)
		inToken = true
	}
	if quote != 0 {
		return nil, fmt.Errorf("nginx config: unclosed quote")
	}
	flush()
	return tokens, nil
}

func parseNginxStmts(tokens []string) ([]nginxStmt, error) {
	stmts, nextIdx, err := parseNginxBlock(tokens, 0, false)
	if err != nil {
		return nil, err
	}
	if nextIdx < len(tokens) {
		return nil, fmt.Errorf("nginx config: unexpected trailing token %q", tokens[nextIdx])
	}
	return stmts, nil
}

func parseNginxBlock(tokens []string, start int, expectClose bool) ([]nginxStmt, int, error) {
	var stmts []nginxStmt
	i := start
	n := len(tokens)
	for i < n {
		if tokens[i] == "}" {
			if !expectClose {
				return nil, i, fmt.Errorf("nginx config: unexpected '}'")
			}
			return stmts, i + 1, nil
		}
		if tokens[i] == ";" {
			i++
			continue
		}
		var words []string
		for i < n && tokens[i] != ";" && tokens[i] != "{" && tokens[i] != "}" {
			words = append(words, tokens[i])
			i++
		}
		if i >= n {
			if len(words) > 0 {
				return nil, i, fmt.Errorf("nginx config: missing ';' or '{' near %q", strings.Join(words, " "))
			}
			break
		}
		if tokens[i] == ";" {
			i++
			if len(words) > 0 {
				stmts = append(stmts, nginxStmt{words: words})
			}
			continue
		}
		if tokens[i] == "{" {
			i++
			sub, nextIdx, err := parseNginxBlock(tokens, i, true)
			if err != nil {
				return nil, nextIdx, err
			}
			i = nextIdx
			stmts = append(stmts, nginxStmt{words: words, block: sub})
			continue
		}
		if tokens[i] == "}" {
			if len(words) > 0 {
				return nil, i, fmt.Errorf("nginx config: missing ';' before '}' near %q", strings.Join(words, " "))
			}
		}
	}
	if expectClose {
		return nil, i, fmt.Errorf("nginx config: unclosed '{'")
	}
	return stmts, i, nil
}

func expandNginxIncludes(stmts []nginxStmt, baseDir string, depth int) ([]nginxStmt, error) {
	if depth > maxImportDepth {
		return nil, fmt.Errorf("nginx config: include recursion limit (%d) exceeded", maxImportDepth)
	}
	out := make([]nginxStmt, 0, len(stmts))
	for _, st := range stmts {
		if len(st.words) >= 2 && strings.EqualFold(st.words[0], "include") && st.block == nil {
			pattern := st.words[1]
			if !filepath.IsAbs(pattern) && baseDir != "" {
				pattern = filepath.Join(baseDir, pattern)
			}
			matches, err := filepath.Glob(pattern)
			if err != nil {
				return nil, fmt.Errorf("nginx include %q: %w", st.words[1], err)
			}
			sort.Strings(matches)
			for _, fpath := range matches {
				info, stErr := os.Stat(fpath)
				if stErr != nil || info.IsDir() {
					continue
				}
				raw, rErr := os.ReadFile(fpath)
				if rErr != nil {
					return nil, fmt.Errorf("nginx include %s: %w", fpath, rErr)
				}
				toks, tErr := tokenizeNginx(string(raw))
				if tErr != nil {
					return nil, fmt.Errorf("nginx include %s: %w", fpath, tErr)
				}
				parsed, pErr := parseNginxStmts(toks)
				if pErr != nil {
					return nil, fmt.Errorf("nginx include %s: %w", fpath, pErr)
				}
				exp, eErr := expandNginxIncludes(parsed, filepath.Dir(fpath), depth+1)
				if eErr != nil {
					return nil, eErr
				}
				out = append(out, exp...)
			}
			continue
		}
		if st.block != nil {
			expBlock, err := expandNginxIncludes(st.block, baseDir, depth)
			if err != nil {
				return nil, err
			}
			st.block = expBlock
		}
		out = append(out, st)
	}
	return out, nil
}

type parsedNginxLocation struct {
	modifier string // "", "=", "~", "~*", "^~"
	pattern  string
	rank     int
	order    int
	route    RouteConfig
}

type parsedNginxServer struct {
	domains        []string
	hasSSL         bool
	listenHTTP     int
	listenHTTPS    int
	certFile       string
	keyFile        string
	root           string
	index          []string
	browse         bool
	headers        map[string]string
	rules          []CustomRule
	upstreams      []string
	locations      []parsedNginxLocation
	waf            SiteWAFConfig
	protect        bool
	isHTTPRedirect bool
}

// ParseNginx parses a native nginx configuration into a validated Gymbag Config.
func ParseNginx(text string) (Config, error) {
	return ParseNginxWithBaseDir(text, "")
}

// ParseNginxWithBaseDir parses a native nginx configuration, resolving `include` paths against baseDir.
func ParseNginxWithBaseDir(text, baseDir string) (Config, error) {
	tokens, err := tokenizeNginx(text)
	if err != nil {
		return Config{}, err
	}
	rawStmts, err := parseNginxStmts(tokens)
	if err != nil {
		return Config{}, err
	}
	stmts, err := expandNginxIncludes(rawStmts, baseDir, 0)
	if err != nil {
		return Config{}, err
	}

	cfg := Default()
	cfg.Sites = nil
	cfg.BaseDir = baseDir
	cfg.Defaults.VerifyLevel = VerifyLevelSuspiciousAndAI

	upstreams := make(map[string][]string)
	var serverBlocks []nginxStmt

	var collectTopLevel func(list []nginxStmt) error
	collectTopLevel = func(list []nginxStmt) error {
		for _, st := range list {
			if len(st.words) == 0 {
				continue
			}
			kw := strings.ToLower(st.words[0])
			switch kw {
			case "http":
				if err := collectTopLevel(st.block); err != nil {
					return err
				}
			case "events", "types", "stream", "mail":
				// ignored top-level blocks
			case "upstream":
				if len(st.words) < 2 {
					return fmt.Errorf("nginx upstream block requires a name")
				}
				name := st.words[1]
				var targets []string
				for _, sub := range st.block {
					if len(sub.words) >= 2 && strings.EqualFold(sub.words[0], "server") {
						skip := false
						for _, opt := range sub.words[2:] {
							if strings.EqualFold(opt, "down") {
								skip = true
								break
							}
						}
						if skip {
							continue
						}
						u, uErr := NormalizeUpstream(sub.words[1], "nginx upstream "+name)
						if uErr != nil {
							return uErr
						}
						targets = append(targets, u)
					}
				}
				if len(targets) > 0 {
					upstreams[name] = targets
				}
			case "server":
				serverBlocks = append(serverBlocks, st)
			case "add_header":
				// top-level http add_header ignored or could apply later
			}
		}
		return nil
	}
	if err := collectTopLevel(stmts); err != nil {
		return Config{}, err
	}
	if len(serverBlocks) == 0 {
		return Config{}, fmt.Errorf("nginx config contains no server blocks")
	}

	var parsedServers []parsedNginxServer
	for _, sb := range serverBlocks {
		ps, err := parseNginxServerBlock(sb, upstreams, cfg.Defaults)
		if err != nil {
			return Config{}, err
		}
		parsedServers = append(parsedServers, ps)
	}

	// Track which domains have an SSL/HTTPS server block so HTTP-only redirect blocks can be skipped.
	sslDomains := make(map[string]struct{})
	for _, ps := range parsedServers {
		if ps.hasSSL || ps.certFile != "" {
			for _, d := range ps.domains {
				sslDomains[d] = struct{}{}
			}
		}
	}

	var allSites []SiteConfig
	allFileCerts := true
	anyFileCerts := false

	for _, ps := range parsedServers {
		if ps.isHTTPRedirect && !ps.hasSSL {
			allCoveredBySSL := true
			for _, d := range ps.domains {
				if _, ok := sslDomains[d]; !ok {
					allCoveredBySSL = false
					break
				}
			}
			if allCoveredBySSL {
				continue
			}
		}

		if ps.listenHTTP > 0 && cfg.HTTPPort == 80 {
			cfg.HTTPPort = ps.listenHTTP
		}
		if ps.listenHTTPS > 0 && cfg.HTTPSPort == 443 {
			cfg.HTTPSPort = ps.listenHTTPS
		}

		if ps.certFile != "" && ps.keyFile != "" {
			anyFileCerts = true
		} else {
			allFileCerts = false
		}

		sortNginxLocations(ps.locations)

		var siteUpstreams []string
		var siteRoutes []RouteConfig

		if len(ps.locations) == 1 &&
			ps.locations[0].modifier == "" &&
			ps.locations[0].pattern == "/" &&
			ps.locations[0].route.HandlerKind() == "proxy" &&
			ps.locations[0].route.StripPrefix == "" &&
			len(ps.locations[0].route.Headers) == 0 {
			siteUpstreams = ps.locations[0].route.Upstreams
		} else {
			hasCatchAll := false
			for _, loc := range ps.locations {
				rc := loc.route
				if rc.Path == "" {
					hasCatchAll = true
				}
				siteRoutes = append(siteRoutes, rc)
			}
			if !hasCatchAll && ps.root != "" {
				siteRoutes = append(siteRoutes, RouteConfig{
					Root:   ps.root,
					Index:  slicesClone(ps.index),
					Browse: ps.browse,
				})
			}
		}
		if len(siteUpstreams) == 0 && len(siteRoutes) == 0 && len(ps.upstreams) > 0 {
			siteUpstreams = ps.upstreams
		}
		if len(siteUpstreams) == 0 && len(siteRoutes) == 0 {
			if ps.root != "" {
				siteRoutes = append(siteRoutes, RouteConfig{
					Root:   ps.root,
					Index:  slicesClone(ps.index),
					Browse: ps.browse,
				})
			} else {
				return Config{}, fmt.Errorf("nginx server block for %s has no proxy_pass, root, or return handler", strings.Join(ps.domains, ", "))
			}
		}

		for _, d := range ps.domains {
			clonedRoutes := make([]RouteConfig, len(siteRoutes))
			for i, r := range siteRoutes {
				clonedRoutes[i] = r.Clone()
			}
			allSites = append(allSites, SiteConfig{
				Domain:    d,
				Upstreams: slicesClone(siteUpstreams),
				Routes:    clonedRoutes,
				Headers:   cloneHeaderMap(ps.headers),
				Protect:   ps.protect,
				WAF:       cloneSiteWAF(ps.waf),
				CertFile:  ps.certFile,
				KeyFile:   ps.keyFile,
			})
		}
	}

	if len(allSites) == 0 {
		return Config{}, fmt.Errorf("nginx config produced no usable server blocks")
	}
	cfg.Sites = allSites
	if anyFileCerts && allFileCerts {
		cfg.TLS.Mode = "file"
	}
	cfg.BindHost = DefaultBindHost(cfg.Domains())
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func parseNginxServerBlock(sb nginxStmt, upstreams map[string][]string, defWAF SiteWAFConfig) (parsedNginxServer, error) {
	ps := parsedNginxServer{
		waf:     cloneSiteWAF(defWAF),
		protect: true,
	}

	for _, st := range sb.block {
		if len(st.words) == 0 {
			continue
		}
		kw := strings.ToLower(st.words[0])
		args := st.words[1:]

		switch kw {
		case "listen":
			port, isSSL := parseNginxListen(args)
			if isSSL {
				ps.hasSSL = true
				if port > 0 {
					ps.listenHTTPS = port
				}
			} else if port == 443 {
				ps.listenHTTPS = port
			} else if port > 0 {
				ps.listenHTTP = port
			}
		case "server_name":
			for _, a := range args {
				if a == "_" || a == `""` || a == "" {
					continue
				}
				ps.domains = append(ps.domains, host.Normalize(a))
			}
		case "ssl_certificate":
			if len(args) >= 1 {
				ps.certFile = args[0]
			}
		case "ssl_certificate_key":
			if len(args) >= 1 {
				ps.keyFile = args[0]
			}
		case "root":
			if len(args) >= 1 {
				ps.root = args[0]
			}
		case "index":
			ps.index = append(ps.index, args...)
		case "autoindex":
			if len(args) >= 1 && strings.EqualFold(args[0], "on") {
				ps.browse = true
			}
		case "add_header":
			if len(args) >= 2 {
				if ps.headers == nil {
					ps.headers = make(map[string]string)
				}
				ps.headers[args[0]] = args[1]
			}
		case "allow", "deny":
			if len(args) >= 1 && !strings.EqualFold(args[0], "all") {
				act := RuleActionAllow
				if kw == "deny" {
					act = RuleActionBlock
				}
				r := CustomRule{Action: act, IPs: []string{args[0]}}
				if err := r.Validate(); err != nil {
					return ps, err
				}
				ps.rules = append(ps.rules, r)
			}
		case "return":
			rc, isRedir, err := parseNginxReturn(args)
			if err != nil {
				return ps, err
			}
			ps.locations = append(ps.locations, parsedNginxLocation{
				pattern: "/",
				rank:    5,
				order:   len(ps.locations),
				route:   rc,
			})
			if isRedir {
				ps.isHTTPRedirect = true
			}
		case "location":
			loc, err := parseNginxLocation(st, ps.root, ps.index, ps.browse, upstreams, len(ps.locations))
			if err != nil {
				return ps, err
			}
			ps.locations = append(ps.locations, loc)
		case "gymbag_verify":
			if len(args) >= 1 {
				lvl, prot, err := ParseVerifyLevel(strings.Join(args, " "))
				if err != nil {
					return ps, err
				}
				ps.waf.VerifyLevel = lvl
				ps.protect = prot
			}
		case "gymbag_protect":
			if len(args) >= 1 {
				b, err := parseBool(args[0], "gymbag_protect")
				if err != nil {
					return ps, err
				}
				ps.protect = b
			}
		case "gymbag_rules_file", "gymbag_rules_dir":
			sources, err := parseRuleSourceArgs(args, nil, kw)
			if err != nil {
				return ps, err
			}
			ps.waf.RuleSources = append(ps.waf.RuleSources, sources...)
		case "gymbag_remote_rules":
			for _, u := range args {
				if err := RequireHTTPURL(u, kw); err != nil {
					return ps, err
				}
				ps.waf.RemoteRules = append(ps.waf.RemoteRules, u)
			}
		}
	}

	if len(ps.domains) == 0 {
		ps.domains = []string{"localhost"}
	}
	if len(ps.rules) > 0 {
		ps.waf.Rules = append(ps.waf.Rules, ps.rules...)
	}
	if len(ps.locations) == 1 && !ps.hasSSL && ps.locations[0].route.Redirect != nil &&
		strings.Contains(strings.ToLower(ps.locations[0].route.Redirect.To), "https://") {
		ps.isHTTPRedirect = true
	}
	return ps, nil
}

func parseNginxListen(args []string) (port int, isSSL bool) {
	for _, a := range args {
		if strings.EqualFold(a, "ssl") {
			isSSL = true
			continue
		}
		if port == 0 {
			candidate := a
			if strings.HasPrefix(candidate, "[") {
				if idx := strings.LastIndex(candidate, "]:"); idx != -1 {
					candidate = candidate[idx+2:]
				}
			} else if idx := strings.LastIndex(candidate, ":"); idx != -1 {
				candidate = candidate[idx+1:]
			}
			if p, err := strconv.Atoi(candidate); err == nil && p >= 1 && p <= 65535 {
				port = p
			}
		}
	}
	return port, isSSL
}

func parseNginxLocation(
	st nginxStmt,
	serverRoot string,
	serverIndex []string,
	serverBrowse bool,
	upstreams map[string][]string,
	order int,
) (parsedNginxLocation, error) {
	if len(st.words) < 2 {
		return parsedNginxLocation{}, fmt.Errorf("nginx location requires a pattern")
	}
	var mod, rawPat string
	if len(st.words) >= 3 && (st.words[1] == "=" || st.words[1] == "~" || st.words[1] == "~*" || st.words[1] == "^~") {
		mod = st.words[1]
		rawPat = st.words[2]
	} else {
		rawPat = st.words[1]
	}

	var routePath string
	var rank int
	switch mod {
	case "=":
		rank = 1
		routePath = rawPat
	case "^~":
		rank = 2
		routePath = nginxPrefixToGlob(rawPat)
	case "~":
		rank = 3
		routePath = "~" + rawPat
	case "~*":
		rank = 3
		routePath = "~(?i)" + rawPat
	default:
		if rawPat == "/" {
			rank = 5
			routePath = ""
		} else {
			rank = 4
			routePath = nginxPrefixToGlob(rawPat)
		}
	}

	locRoot := serverRoot
	locIndex := slicesClone(serverIndex)
	locBrowse := serverBrowse
	var locTryFiles string
	var locAlias string
	var locProxy string
	var locHeaders map[string]string
	var locReturn *RouteConfig

	for _, sub := range st.block {
		if len(sub.words) == 0 {
			continue
		}
		kw := strings.ToLower(sub.words[0])
		args := sub.words[1:]
		switch kw {
		case "proxy_pass":
			if len(args) >= 1 {
				locProxy = args[0]
			}
		case "root":
			if len(args) >= 1 {
				locRoot = args[0]
			}
		case "alias":
			if len(args) >= 1 {
				locAlias = args[0]
			}
		case "index":
			locIndex = append([]string(nil), args...)
		case "autoindex":
			if len(args) >= 1 {
				locBrowse = strings.EqualFold(args[0], "on")
			}
		case "try_files":
			if fb := extractTryFilesFallback(args); fb != "" {
				locTryFiles = fb
			}
		case "add_header":
			if len(args) >= 2 {
				if locHeaders == nil {
					locHeaders = make(map[string]string)
				}
				locHeaders[args[0]] = args[1]
			}
		case "return":
			rc, _, err := parseNginxReturn(args)
			if err != nil {
				return parsedNginxLocation{}, err
			}
			locReturn = &rc
		}
	}

	rc := RouteConfig{
		Path:    routePath,
		Headers: locHeaders,
	}

	switch {
	case locReturn != nil:
		rc.Respond = locReturn.Respond
		rc.Redirect = locReturn.Redirect
	case locProxy != "":
		targets, strip, err := resolveNginxProxyPass(locProxy, rawPat, mod, upstreams)
		if err != nil {
			return parsedNginxLocation{}, err
		}
		rc.Upstreams = targets
		rc.StripPrefix = strip
	case locAlias != "":
		rc.Root = locAlias
		rc.Index = locIndex
		rc.Browse = locBrowse
		rc.Fallback = locTryFiles
		if mod == "" || mod == "^~" {
			sp := strings.TrimRight(rawPat, "/")
			if sp != "" && sp != "/" {
				rc.StripPrefix = sp
			}
		}
	case locRoot != "":
		rc.Root = locRoot
		rc.Index = locIndex
		rc.Browse = locBrowse
		rc.Fallback = locTryFiles
	default:
		rc.Respond = &StaticResponse{Status: 200}
	}

	return parsedNginxLocation{
		modifier: mod,
		pattern:  rawPat,
		rank:     rank,
		order:    order,
		route:    rc,
	}, nil
}

func nginxPrefixToGlob(prefix string) string {
	if prefix == "" || prefix == "/" {
		return ""
	}
	if strings.HasSuffix(prefix, "/") {
		return prefix + "*"
	}
	return prefix + "*"
}

func resolveNginxProxyPass(rawTarget, locPattern, mod string, upstreams map[string][]string) ([]string, string, error) {
	target := strings.TrimSpace(rawTarget)
	lowerTarget := strings.ToLower(target)
	if strings.HasPrefix(lowerTarget, "ws://") {
		target = "http://" + target[5:]
	} else if strings.HasPrefix(lowerTarget, "wss://") {
		target = "https://" + target[6:]
	}
	u, err := url.Parse(target)
	if err != nil || u.Scheme == "" || u.Host == "" {
		// Try expanding bare or named upstream directly.
		if ups, ok := upstreams[target]; ok {
			return slicesClone(ups), "", nil
		}
		norm, nErr := NormalizeUpstream(target, "proxy_pass")
		if nErr != nil {
			return nil, "", nErr
		}
		return []string{norm}, "", nil
	}

	var resolved []string
	if ups, ok := upstreams[u.Host]; ok {
		resolved = slicesClone(ups)
		if u.Path != "" && u.Path != "/" {
			suffix := strings.TrimRight(u.Path, "/")
			for i, up := range resolved {
				resolved[i] = strings.TrimRight(up, "/") + suffix
			}
		}
	} else {
		base := u.Scheme + "://" + u.Host
		if u.Path != "" && u.Path != "/" {
			base += strings.TrimRight(u.Path, "/")
		}
		norm, nErr := NormalizeUpstream(base, "proxy_pass")
		if nErr != nil {
			return nil, "", nErr
		}
		resolved = []string{norm}
	}

	// In nginx, a URI component on proxy_pass (even just trailing "/") replaces the matched location prefix.
	var stripPrefix string
	if u.Path != "" && (mod == "" || mod == "^~") && locPattern != "/" {
		stripPrefix = strings.TrimRight(locPattern, "/")
	}
	return resolved, stripPrefix, nil
}

func parseNginxReturn(args []string) (RouteConfig, bool, error) {
	if len(args) == 0 {
		return RouteConfig{}, false, fmt.Errorf("nginx return requires a status code")
	}
	code, err := strconv.Atoi(args[0])
	if err != nil || code < 100 || code > 599 {
		return RouteConfig{}, false, fmt.Errorf("invalid nginx return status %q", args[0])
	}
	val := ""
	if len(args) >= 2 {
		val = translateNginxVars(args[1])
	}
	if code >= 300 && code <= 399 {
		if val == "" {
			return RouteConfig{}, false, fmt.Errorf("nginx return %d requires a redirect URL", code)
		}
		return RouteConfig{
			Redirect: &RedirectTarget{To: val, Code: code},
		}, true, nil
	}
	return RouteConfig{
		Respond: &StaticResponse{Status: code, Body: val},
	}, false, nil
}

func translateNginxVars(s string) string {
	repl := strings.NewReplacer(
		"$scheme", "{scheme}",
		"$host", "{host}",
		"$http_host", "{host}",
		"$request_uri", "{uri}",
		"$uri", "{path}",
		"$args", "{query}",
		"$query_string", "{query}",
	)
	return repl.Replace(s)
}

func sortNginxLocations(locs []parsedNginxLocation) {
	sort.SliceStable(locs, func(i, j int) bool {
		if locs[i].rank != locs[j].rank {
			return locs[i].rank < locs[j].rank
		}
		if locs[i].rank == 2 || locs[i].rank == 4 {
			if len(locs[i].pattern) != len(locs[j].pattern) {
				return len(locs[i].pattern) > len(locs[j].pattern)
			}
		}
		return locs[i].order < locs[j].order
	})
}
