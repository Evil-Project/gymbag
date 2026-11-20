package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type apacheStmt struct {
	words []string
	tag   string
	args  string
	block []apacheStmt
}

func isApacheConfig(text, path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if base == "httpd.conf" || base == "apache2.conf" || base == ".htaccess" || strings.HasSuffix(base, ".apache.conf") {
		return true
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || strings.HasPrefix(trimmed, "{") {
		return false
	}
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "<virtualhost") || strings.Contains(lower, "<ifmodule") ||
		strings.Contains(lower, "<location") || strings.Contains(lower, "<directory") ||
		strings.Contains(lower, "<proxy") {
		return true
	}
	if strings.Contains(lower, "servername ") && (strings.Contains(lower, "proxypass") || strings.Contains(lower, "documentroot")) {
		return true
	}
	return false
}

func preprocessApacheLines(text string) []string {
	rawLines := strings.Split(text, "\n")
	var out []string
	var buf strings.Builder
	for _, raw := range rawLines {
		cleaned := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(cleaned)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			if buf.Len() > 0 {
				out = append(out, buf.String())
				buf.Reset()
			}
			continue
		}
		if strings.HasSuffix(trimmed, `\`) {
			buf.WriteString(strings.TrimSuffix(trimmed, `\`))
			buf.WriteByte(' ')
			continue
		}
		if buf.Len() > 0 {
			buf.WriteString(trimmed)
			out = append(out, buf.String())
			buf.Reset()
		} else {
			out = append(out, trimmed)
		}
	}
	if buf.Len() > 0 {
		out = append(out, buf.String())
	}
	return out
}

func parseApacheStmts(lines []string) ([]apacheStmt, error) {
	stmts, nextIdx, err := parseApacheBlock(lines, 0, "")
	if err != nil {
		return nil, err
	}
	if nextIdx < len(lines) {
		return nil, fmt.Errorf("apache config: unexpected trailing line %q", lines[nextIdx])
	}
	return stmts, nil
}

func parseApacheBlock(lines []string, start int, closeTag string) ([]apacheStmt, int, error) {
	var stmts []apacheStmt
	i := start
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			i++
			continue
		}
		if strings.HasPrefix(line, "</") && strings.HasSuffix(line, ">") {
			tag := strings.ToLower(strings.TrimSpace(line[2 : len(line)-1]))
			if closeTag == "" {
				return nil, i, fmt.Errorf("apache config: unexpected closing tag </%s>", tag)
			}
			if tag != closeTag {
				return nil, i, fmt.Errorf("apache config: expected </%s>, got </%s>", closeTag, tag)
			}
			return stmts, i + 1, nil
		}
		if strings.HasPrefix(line, "<") && strings.HasSuffix(line, ">") {
			inner := strings.TrimSpace(line[1 : len(line)-1])
			tag, rest, _ := strings.Cut(inner, " ")
			lowerTag := strings.ToLower(strings.TrimSpace(tag))
			i++
			sub, nextIdx, err := parseApacheBlock(lines, i, lowerTag)
			if err != nil {
				return nil, nextIdx, err
			}
			i = nextIdx
			stmts = append(stmts, apacheStmt{
				tag:   lowerTag,
				args:  strings.TrimSpace(rest),
				block: sub,
			})
			continue
		}
		words, err := shlexSplit(stripComment(line))
		if err != nil {
			return nil, i, fmt.Errorf("apache config: %w", err)
		}
		if len(words) > 0 {
			stmts = append(stmts, apacheStmt{words: words})
		}
		i++
	}
	if closeTag != "" {
		return nil, i, fmt.Errorf("apache config: unclosed <%s>", closeTag)
	}
	return stmts, i, nil
}

func expandApacheIncludes(stmts []apacheStmt, baseDir string, depth int) ([]apacheStmt, error) {
	if depth > maxImportDepth {
		return nil, fmt.Errorf("apache config: Include recursion limit (%d) exceeded", maxImportDepth)
	}
	out := make([]apacheStmt, 0, len(stmts))
	for _, st := range stmts {
		if st.tag == "" && len(st.words) >= 2 && (strings.EqualFold(st.words[0], "Include") || strings.EqualFold(st.words[0], "IncludeOptional")) {
			optional := strings.EqualFold(st.words[0], "IncludeOptional")
			pattern := st.words[1]
			if !filepath.IsAbs(pattern) && baseDir != "" {
				pattern = filepath.Join(baseDir, pattern)
			}
			matches, err := filepath.Glob(pattern)
			if err != nil {
				return nil, fmt.Errorf("apache %s %q: %w", st.words[0], st.words[1], err)
			}
			sort.Strings(matches)
			if len(matches) == 0 && !optional && !strings.ContainsAny(st.words[1], "*?[") {
				if _, statErr := os.Stat(pattern); statErr != nil {
					return nil, fmt.Errorf("apache Include %q not found: %w", st.words[1], statErr)
				}
			}
			for _, fpath := range matches {
				info, stErr := os.Stat(fpath)
				if stErr != nil || info.IsDir() {
					continue
				}
				raw, rErr := os.ReadFile(fpath)
				if rErr != nil {
					return nil, fmt.Errorf("apache Include %s: %w", fpath, rErr)
				}
				parsed, pErr := parseApacheStmts(preprocessApacheLines(string(raw)))
				if pErr != nil {
					return nil, fmt.Errorf("apache Include %s: %w", fpath, pErr)
				}
				exp, eErr := expandApacheIncludes(parsed, filepath.Dir(fpath), depth+1)
				if eErr != nil {
					return nil, eErr
				}
				out = append(out, exp...)
			}
			continue
		}
		if st.block != nil {
			expBlock, err := expandApacheIncludes(st.block, baseDir, depth)
			if err != nil {
				return nil, err
			}
			st.block = expBlock
		}
		out = append(out, st)
	}
	return out, nil
}

type parsedApacheVHost struct {
	domains        []string
	port           int
	hasSSL         bool
	certFile       string
	keyFile        string
	docRoot        string
	dirIndex       []string
	browse         bool
	headers        map[string]string
	rules          []CustomRule
	upstreams      []string
	routes         []RouteConfig
	waf            SiteWAFConfig
	protect        bool
	isHTTPRedirect bool
}

// ParseApache parses a native Apache httpd/VirtualHost configuration into a validated Gymbag Config.
func ParseApache(text string) (Config, error) {
	return ParseApacheWithBaseDir(text, "")
}

// ParseApacheWithBaseDir parses a native Apache configuration, resolving Include directives against baseDir.
func ParseApacheWithBaseDir(text, baseDir string) (Config, error) {
	rawStmts, err := parseApacheStmts(preprocessApacheLines(text))
	if err != nil {
		return Config{}, err
	}
	stmts, err := expandApacheIncludes(rawStmts, baseDir, 0)
	if err != nil {
		return Config{}, err
	}

	cfg := Default()
	cfg.Sites = nil
	cfg.BaseDir = baseDir
	cfg.Defaults.VerifyLevel = VerifyLevelSuspiciousAndAI

	balancers := make(map[string][]string)
	var vhosts []apacheStmt

	var collectTop func(list []apacheStmt) error
	collectTop = func(list []apacheStmt) error {
		for _, st := range list {
			switch st.tag {
			case "virtualhost":
				vhosts = append(vhosts, st)
			case "ifmodule", "ifdefine":
				if err := collectTop(st.block); err != nil {
					return err
				}
			case "proxy":
				bName := strings.Trim(st.args, `"'`)
				if strings.HasPrefix(strings.ToLower(bName), "balancer://") {
					key := strings.TrimRight(strings.ToLower(bName), "/")
					var members []string
					for _, sub := range st.block {
						if len(sub.words) >= 2 && strings.EqualFold(sub.words[0], "BalancerMember") {
							u, uErr := NormalizeUpstream(sub.words[1], "BalancerMember")
							if uErr != nil {
								return uErr
							}
							members = append(members, u)
						}
					}
					if len(members) > 0 {
						balancers[key] = members
					}
				}
			case "":
				if len(st.words) >= 2 && strings.EqualFold(st.words[0], "Listen") {
					p := parseApachePort(st.words[1])
					if p == 443 || strings.Contains(strings.ToLower(strings.Join(st.words, " ")), "https") {
						cfg.HTTPSPort = p
					} else if p > 0 && p != 443 && cfg.HTTPPort == 80 {
						cfg.HTTPPort = p
					}
				}
			}
		}
		return nil
	}
	if err := collectTop(stmts); err != nil {
		return Config{}, err
	}
	if len(vhosts) == 0 {
		return Config{}, fmt.Errorf("apache config contains no <VirtualHost> blocks")
	}

	var parsedVHosts []parsedApacheVHost
	for _, vh := range vhosts {
		pv, err := parseApacheVHost(vh, balancers, &cfg)
		if err != nil {
			return Config{}, err
		}
		parsedVHosts = append(parsedVHosts, pv)
	}

	sslDomains := make(map[string]struct{})
	for _, pv := range parsedVHosts {
		if pv.hasSSL || pv.certFile != "" || pv.port == 443 {
			for _, d := range pv.domains {
				sslDomains[d] = struct{}{}
			}
		}
	}

	var allSites []SiteConfig
	allFileCerts := true
	anyFileCerts := false

	for _, pv := range parsedVHosts {
		if pv.isHTTPRedirect && !pv.hasSSL && pv.port != 443 {
			allCovered := true
			for _, d := range pv.domains {
				if _, ok := sslDomains[d]; !ok {
					allCovered = false
					break
				}
			}
			if allCovered {
				continue
			}
		}

		if pv.port > 0 {
			if pv.hasSSL || pv.port == 443 {
				if cfg.HTTPSPort == 443 {
					cfg.HTTPSPort = pv.port
				}
			} else if cfg.HTTPPort == 80 {
				cfg.HTTPPort = pv.port
			}
		}

		if pv.certFile != "" && pv.keyFile != "" {
			anyFileCerts = true
		} else {
			allFileCerts = false
		}

		routes := make([]RouteConfig, 0, len(pv.routes)+1)
		routes = append(routes, pv.routes...)
		hasCatchAll := false
		for _, r := range routes {
			if r.Path == "" {
				hasCatchAll = true
				break
			}
		}
		if len(pv.upstreams) == 0 && !hasCatchAll && pv.docRoot != "" {
			routes = append(routes, RouteConfig{
				Root:   pv.docRoot,
				Index:  slicesClone(pv.dirIndex),
				Browse: pv.browse,
			})
		}

		if len(pv.upstreams) == 0 && len(routes) == 0 {
			return Config{}, fmt.Errorf("apache VirtualHost for %s has no ProxyPass, DocumentRoot, or Redirect handler", strings.Join(pv.domains, ", "))
		}

		for _, d := range pv.domains {
			clonedRoutes := make([]RouteConfig, len(routes))
			for i, r := range routes {
				clonedRoutes[i] = r.Clone()
			}
			allSites = append(allSites, SiteConfig{
				Domain:    d,
				Upstreams: slicesClone(pv.upstreams),
				Routes:    clonedRoutes,
				Headers:   cloneHeaderMap(pv.headers),
				Protect:   pv.protect,
				WAF:       cloneSiteWAF(pv.waf),
				CertFile:  pv.certFile,
				KeyFile:   pv.keyFile,
			})
		}
	}

	if len(allSites) == 0 {
		return Config{}, fmt.Errorf("apache config produced no usable VirtualHost blocks")
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

func parseApachePort(token string) int {
	t := strings.TrimSpace(token)
	if idx := strings.LastIndex(t, ":"); idx != -1 {
		if p, err := strconv.Atoi(t[idx+1:]); err == nil && p >= 1 && p <= 65535 {
			return p
		}
	}
	if p, err := strconv.Atoi(t); err == nil && p >= 1 && p <= 65535 {
		return p
	}
	return 0
}

func parseApacheVHost(vh apacheStmt, balancers map[string][]string, cfg *Config) (parsedApacheVHost, error) {
	pv := parsedApacheVHost{
		port:    parseApachePort(vh.args),
		waf:     cloneSiteWAF(cfg.Defaults),
		protect: true,
	}

	localBalancers := make(map[string][]string, len(balancers))
	for k, v := range balancers {
		localBalancers[k] = v
	}

	var applyStmts func(list []apacheStmt) error
	applyStmts = func(list []apacheStmt) error {
		for _, st := range list {
			switch st.tag {
			case "ifmodule", "ifdefine":
				if err := applyStmts(st.block); err != nil {
					return err
				}
			case "proxy":
				bName := strings.Trim(st.args, `"'`)
				if strings.HasPrefix(strings.ToLower(bName), "balancer://") {
					key := strings.TrimRight(strings.ToLower(bName), "/")
					var members []string
					for _, sub := range st.block {
						if len(sub.words) >= 2 && strings.EqualFold(sub.words[0], "BalancerMember") {
							u, uErr := NormalizeUpstream(sub.words[1], "BalancerMember")
							if uErr != nil {
								return uErr
							}
							members = append(members, u)
						}
					}
					if len(members) > 0 {
						localBalancers[key] = members
					}
				}
			case "location", "locationmatch":
				isRegex := st.tag == "locationmatch"
				pat := strings.Trim(st.args, `"'`)
				routePat := pat
				if isRegex {
					routePat = "~" + pat
				} else if pat == "/" {
					routePat = ""
				} else if !strings.HasSuffix(pat, "*") {
					routePat = strings.TrimRight(pat, "/") + "/*"
				}
				locHeaders := make(map[string]string)
				var locUpstreams []string
				var locRedirect *RedirectTarget
				for _, sub := range st.block {
					if len(sub.words) == 0 {
						continue
					}
					kw := strings.ToLower(sub.words[0])
					args := sub.words[1:]
					switch kw {
					case "proxypass":
						if len(args) >= 1 && args[0] != "!" {
							ups, uErr := resolveApacheTarget(args[0], localBalancers)
							if uErr != nil {
								return uErr
							}
							locUpstreams = ups
						}
					case "header":
						if err := applyApacheHeader(args, locHeaders); err != nil {
							return err
						}
					case "redirect", "redirectpermanent", "redirecttemp":
						_, redir, resp, rErr := parseApacheRedirect(kw, args)
						if rErr != nil {
							return rErr
						}
						if redir != nil {
							locRedirect = redir
						} else if resp != nil {
							pv.routes = append(pv.routes, RouteConfig{Path: routePat, Respond: resp, Headers: cloneHeaderMap(locHeaders)})
						}
					case "require":
						if err := applyApacheRequire(args, &pv.rules); err != nil {
							return err
						}
					}
				}
				if locRedirect != nil {
					pv.routes = append(pv.routes, RouteConfig{Path: routePat, Redirect: locRedirect, Headers: cloneHeaderMap(locHeaders)})
				} else if len(locUpstreams) > 0 {
					pv.routes = append(pv.routes, RouteConfig{Path: routePat, Upstreams: locUpstreams, Headers: cloneHeaderMap(locHeaders)})
				} else if len(locHeaders) > 0 && routePat == "" {
					if pv.headers == nil {
						pv.headers = make(map[string]string)
					}
					for k, v := range locHeaders {
						pv.headers[k] = v
					}
				}
			case "directory":
				dirPath := strings.TrimRight(strings.Trim(st.args, `"`), "/")
				var dirBrowse *bool
				var dirIdx []string
				for _, sub := range st.block {
					if len(sub.words) == 0 {
						continue
					}
					kw := strings.ToLower(sub.words[0])
					args := sub.words[1:]
					switch kw {
					case "options":
						for _, o := range args {
							lo := strings.ToLower(o)
							if lo == "indexes" || lo == "+indexes" {
								v := true
								dirBrowse = &v
								pv.browse = true
							} else if lo == "-indexes" {
								v := false
								dirBrowse = &v
								pv.browse = false
							}
						}
					case "directoryindex":
						dirIdx = append(dirIdx, args...)
						pv.dirIndex = append(pv.dirIndex, args...)
					case "require":
						if err := applyApacheRequire(args, &pv.rules); err != nil {
							return err
						}
					}
				}
				if dirPath != "" && (dirBrowse != nil || len(dirIdx) > 0) {
					for i := range pv.routes {
						if strings.TrimRight(pv.routes[i].Root, "/") == dirPath {
							if dirBrowse != nil {
								pv.routes[i].Browse = *dirBrowse
							}
							if len(dirIdx) > 0 {
								pv.routes[i].Index = append(pv.routes[i].Index, dirIdx...)
							}
						}
					}
				}
			case "":
				if len(st.words) == 0 {
					continue
				}
				kw := strings.ToLower(st.words[0])
				args := st.words[1:]
				switch kw {
				case "servername":
					if len(args) >= 1 {
						h, _, _, err := parseCaddySiteAddressToken(args[0])
						if err != nil {
							return err
						}
						pv.domains = append(pv.domains, h)
					}
				case "serveralias":
					for _, a := range args {
						h, _, _, err := parseCaddySiteAddressToken(a)
						if err != nil {
							return err
						}
						pv.domains = append(pv.domains, h)
					}
				case "serveradmin":
					if len(args) >= 1 && strings.Contains(args[0], "@") && cfg.TLS.ACMEEmail == "" {
						cfg.TLS.ACMEEmail = args[0]
					}
				case "sslengine":
					if len(args) >= 1 && strings.EqualFold(args[0], "on") {
						pv.hasSSL = true
					}
				case "sslcertificatefile":
					if len(args) >= 1 {
						pv.certFile = args[0]
						pv.hasSSL = true
					}
				case "sslcertificatekeyfile":
					if len(args) >= 1 {
						pv.keyFile = args[0]
						pv.hasSSL = true
					}
				case "documentroot":
					if len(args) >= 1 {
						pv.docRoot = args[0]
					}
				case "directoryindex":
					pv.dirIndex = append(pv.dirIndex, args...)
				case "alias":
					if len(args) >= 2 {
						prefix := strings.TrimRight(args[0], "/")
						if prefix == "" {
							prefix = "/"
						}
						routePat := prefix + "/*"
						if prefix == "/" {
							routePat = ""
							prefix = ""
						}
						pv.routes = append(pv.routes, RouteConfig{
							Path:        routePat,
							StripPrefix: prefix,
							Root:        args[1],
							Index:       slicesClone(pv.dirIndex),
							Browse:      pv.browse,
						})
					}
				case "proxypass":
					if len(args) >= 2 && args[1] != "!" {
						ups, uErr := resolveApacheTarget(args[1], localBalancers)
						if uErr != nil {
							return uErr
						}
						if args[0] == "/" {
							pv.upstreams = append(pv.upstreams, ups...)
						} else {
							prefix := strings.TrimRight(args[0], "/")
							strip := ""
							if strings.HasSuffix(strings.TrimSpace(args[1]), "/") || strings.HasPrefix(args[1], "balancer://") {
								strip = prefix
							}
							pv.routes = append(pv.routes, RouteConfig{
								Path:        prefix + "*",
								StripPrefix: strip,
								Upstreams:   ups,
							})
						}
					}
				case "proxypassmatch":
					if len(args) >= 2 && args[1] != "!" {
						ups, uErr := resolveApacheTarget(args[1], localBalancers)
						if uErr != nil {
							return uErr
						}
						pv.routes = append(pv.routes, RouteConfig{
							Path:      "~" + args[0],
							Upstreams: ups,
						})
					}
				case "redirect", "redirectpermanent", "redirecttemp", "redirectmatch":
					fromPat, redir, resp, rErr := parseApacheRedirect(kw, args)
					if rErr != nil {
						return rErr
					}
					routePat := fromPat
					if routePat == "/" {
						routePat = ""
					} else if kw == "redirectmatch" && routePat != "" {
						routePat = "~" + routePat
					} else if routePat != "" && !strings.HasSuffix(routePat, "*") {
						routePat += "*"
					}
					if redir != nil {
						pv.routes = append(pv.routes, RouteConfig{Path: routePat, Redirect: redir})
						if strings.Contains(strings.ToLower(redir.To), "https://") && pv.port != 443 && !pv.hasSSL {
							pv.isHTTPRedirect = true
						}
					} else if resp != nil {
						pv.routes = append(pv.routes, RouteConfig{Path: routePat, Respond: resp})
					}
				case "header":
					if pv.headers == nil {
						pv.headers = make(map[string]string)
					}
					if err := applyApacheHeader(args, pv.headers); err != nil {
						return err
					}
				case "gymbagverify":
					if len(args) >= 1 {
						lvl, prot, err := ParseVerifyLevel(strings.Join(args, " "))
						if err != nil {
							return err
						}
						pv.waf.VerifyLevel = lvl
						pv.protect = prot
					}
				case "gymbagrulesfile", "gymbagrulesdir":
					sources, err := parseRuleSourceArgs(args, nil, kw)
					if err != nil {
						return err
					}
					pv.waf.RuleSources = append(pv.waf.RuleSources, sources...)
				}
			}
		}
		return nil
	}

	if err := applyStmts(vh.block); err != nil {
		return pv, err
	}
	if len(pv.domains) == 0 {
		pv.domains = []string{"localhost"}
	}
	if len(pv.rules) > 0 {
		pv.waf.Rules = append(pv.waf.Rules, pv.rules...)
	}
	if len(pv.headers) == 0 {
		pv.headers = nil
	}
	return pv, nil
}

func resolveApacheTarget(raw string, balancers map[string][]string) ([]string, error) {
	trimmed := strings.TrimSpace(raw)
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "balancer://") {
		key := strings.TrimRight(lower, "/")
		if ups, ok := balancers[key]; ok {
			return slicesClone(ups), nil
		}
		return nil, fmt.Errorf("apache undefined balancer %q", raw)
	}
	if strings.HasPrefix(lower, "ws://") {
		trimmed = "http://" + trimmed[5:]
	} else if strings.HasPrefix(lower, "wss://") {
		trimmed = "https://" + trimmed[6:]
	}
	u, err := NormalizeUpstream(trimmed, "ProxyPass")
	if err != nil {
		return nil, err
	}
	return []string{u}, nil
}

func applyApacheHeader(args []string, dst map[string]string) error {
	rest := args
	if len(rest) > 0 && strings.EqualFold(rest[0], "always") {
		rest = rest[1:]
	}
	if len(rest) < 2 {
		return nil
	}
	action := strings.ToLower(rest[0])
	field := rest[1]
	switch action {
	case "set", "setifempty":
		val := ""
		if len(rest) >= 3 {
			val = rest[2]
		}
		dst[field] = val
	case "add", "append":
		val := ""
		if len(rest) >= 3 {
			val = rest[2]
		}
		dst["+"+field] = val
	case "unset":
		dst["-"+field] = ""
	}
	return nil
}

func parseApacheRedirect(kw string, args []string) (fromPath string, redir *RedirectTarget, resp *StaticResponse, err error) {
	if len(args) < 2 {
		return "", nil, nil, fmt.Errorf("%s requires source path and target", kw)
	}
	code := 302
	switch kw {
	case "redirectpermanent":
		code = 301
	case "redirecttemp":
		code = 302
	}

	idx := 0
	if len(args) >= 2 {
		switch strings.ToLower(args[0]) {
		case "permanent":
			code = 301
			idx = 1
		case "temp":
			code = 302
			idx = 1
		case "seeother":
			code = 303
			idx = 1
		case "gone":
			return args[1], nil, &StaticResponse{Status: 410, Body: "Gone"}, nil
		default:
			if n, convErr := strconv.Atoi(args[0]); convErr == nil && n >= 300 && n <= 499 {
				code = n
				idx = 1
			}
		}
	}
	rem := args[idx:]
	if len(rem) < 1 {
		return "", nil, nil, fmt.Errorf("%s requires source path", kw)
	}
	if code == 410 {
		return rem[0], nil, &StaticResponse{Status: 410, Body: "Gone"}, nil
	}
	if len(rem) < 2 {
		return "", nil, nil, fmt.Errorf("%s requires source path and target URL", kw)
	}
	return rem[0], &RedirectTarget{To: rem[1], Code: code}, nil, nil
}

func applyApacheRequire(args []string, rules *[]CustomRule) error {
	if len(args) < 2 {
		return nil
	}
	if strings.EqualFold(args[0], "ip") {
		r := CustomRule{Action: RuleActionAllow, IPs: args[1:]}
		if err := r.Validate(); err != nil {
			return err
		}
		*rules = append(*rules, r)
		return nil
	}
	if len(args) >= 3 && strings.EqualFold(args[0], "not") && strings.EqualFold(args[1], "ip") {
		r := CustomRule{Action: RuleActionBlock, IPs: args[2:]}
		if err := r.Validate(); err != nil {
			return err
		}
		*rules = append(*rules, r)
	}
	return nil
}
