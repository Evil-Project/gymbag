package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gymbag/internal/host"
)

const maxImportDepth = 16

func cloneStatementItem(item statementItem) statementItem {
	out := statementItem{
		words: slicesClone(item.words),
	}
	if item.sub != nil {
		subBody := make([]statementItem, len(item.sub.body))
		for i, b := range item.sub.body {
			subBody[i] = cloneStatementItem(b)
		}
		out.sub = &parsedBlock{
			header: slicesClone(item.sub.header),
			body:   subBody,
		}
	}
	return out
}

func slicesClone(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func substituteSnippetArgs(items []statementItem, args []string) []statementItem {
	joined := strings.Join(args, " ")
	out := make([]statementItem, len(items))
	for i, it := range items {
		c := cloneStatementItem(it)
		for wi, w := range c.words {
			c.words[wi] = replaceArgsInString(w, args, joined)
		}
		if c.sub != nil {
			for hi, h := range c.sub.header {
				c.sub.header[hi] = replaceArgsInString(h, args, joined)
			}
			c.sub.body = substituteSnippetArgs(c.sub.body, args)
		}
		out[i] = c
	}
	return out
}

func replaceArgsInString(s string, args []string, joined string) string {
	if !strings.Contains(s, "{args") {
		return s
	}
	s = strings.ReplaceAll(s, "{args.*}", joined)
	for i, val := range args {
		s = strings.ReplaceAll(s, fmt.Sprintf("{args[%d]}", i), val)
		s = strings.ReplaceAll(s, fmt.Sprintf("{args.%d}", i), val)
	}
	return s
}

func isSnippetHeader(words []string) (string, bool) {
	if len(words) != 1 {
		return "", false
	}
	w := words[0]
	if len(w) >= 3 && strings.HasPrefix(w, "(") && strings.HasSuffix(w, ")") {
		name := strings.TrimSpace(w[1 : len(w)-1])
		if name != "" {
			return name, true
		}
	}
	return "", false
}

func expandImports(items []statementItem, baseDir string, snippets map[string][]statementItem, depth int) ([]statementItem, error) {
	if depth > maxImportDepth {
		return nil, fmt.Errorf("import recursion limit (%d) exceeded", maxImportDepth)
	}
	out := make([]statementItem, 0, len(items))
	for _, it := range items {
		if it.sub == nil && len(it.words) > 0 && strings.EqualFold(it.words[0], "import") {
			if len(it.words) < 2 {
				return nil, fmt.Errorf("import requires a snippet name or file path")
			}
			target := it.words[1]
			impArgs := it.words[2:]
			if snipBody, ok := snippets[target]; ok {
				substituted := substituteSnippetArgs(snipBody, impArgs)
				expanded, err := expandImports(substituted, baseDir, snippets, depth+1)
				if err != nil {
					return nil, err
				}
				out = append(out, expanded...)
				continue
			}

			pattern := target
			if !filepath.IsAbs(pattern) && baseDir != "" {
				pattern = filepath.Join(baseDir, pattern)
			}
			matches, globErr := filepath.Glob(pattern)
			if globErr != nil {
				return nil, fmt.Errorf("invalid import pattern %q: %w", target, globErr)
			}
			sort.Strings(matches)
			if len(matches) == 0 {
				if strings.ContainsAny(target, "*?[") {
					// Glob matched zero files: non-fatal in Caddyfile.
					continue
				}
				if _, statErr := os.Stat(pattern); statErr != nil {
					return nil, fmt.Errorf("import target %q not found: %w", target, statErr)
				}
				matches = []string{pattern}
			}
			for _, fpath := range matches {
				info, statErr := os.Stat(fpath)
				if statErr != nil || info.IsDir() {
					continue
				}
				raw, readErr := os.ReadFile(fpath)
				if readErr != nil {
					return nil, fmt.Errorf("failed to read imported file %s: %w", fpath, readErr)
				}
				toks, tokErr := tokenizeCaddyfile(string(raw))
				if tokErr != nil {
					return nil, fmt.Errorf("import %s: %w", fpath, tokErr)
				}
				subItems, parseErr := parseItems(toks)
				if parseErr != nil {
					return nil, fmt.Errorf("import %s: %w", fpath, parseErr)
				}
				// Register any snippets defined inside the imported file, then expand remaining items.
				var bodyItems []statementItem
				for _, sub := range subItems {
					if sub.sub != nil {
						if sname, isSnip := isSnippetHeader(sub.words); isSnip {
							snippets[sname] = sub.sub.body
							continue
						}
					}
					bodyItems = append(bodyItems, sub)
				}
				expanded, expErr := expandImports(bodyItems, filepath.Dir(fpath), snippets, depth+1)
				if expErr != nil {
					return nil, expErr
				}
				out = append(out, expanded...)
			}
			continue
		}

		if it.sub != nil {
			expandedBody, err := expandImports(it.sub.body, baseDir, snippets, depth)
			if err != nil {
				return nil, err
			}
			cloned := cloneStatementItem(it)
			cloned.sub.body = expandedBody
			out = append(out, cloned)
			continue
		}
		out = append(out, it)
	}
	return out, nil
}

func parseCaddySiteAddressToken(token string) (hostname string, port int, hasPort bool, err error) {
	trimmed := strings.TrimRight(strings.TrimSpace(token), ",")
	if strings.HasPrefix(trimmed, ":") {
		p, pErr := host.ParsePort(trimmed[1:], "site port")
		if pErr != nil {
			return "", 0, false, pErr
		}
		return "localhost", p, true, nil
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "http://:") || strings.HasPrefix(strings.ToLower(trimmed), "https://:") {
		idx := strings.Index(trimmed, "://:")
		p, pErr := host.ParsePort(trimmed[idx+4:], "site port")
		if pErr != nil {
			return "", 0, false, pErr
		}
		return "localhost", p, true, nil
	}
	return host.ParseSiteAddress(token)
}

func isMatcherToken(tok string) bool {
	return tok == "*" || strings.HasPrefix(tok, "@") || strings.HasPrefix(tok, "/") || strings.HasPrefix(tok, "~")
}

func resolveMatcherPaths(tok string, matchers map[string][]string) ([]string, error) {
	tok = strings.TrimSpace(tok)
	if tok == "" || tok == "*" || tok == "/*" {
		return []string{""}, nil
	}
	if strings.HasPrefix(tok, "@") {
		paths, ok := matchers[tok]
		if !ok {
			return nil, fmt.Errorf("undefined matcher %q", tok)
		}
		if len(paths) == 0 {
			return []string{""}, nil
		}
		return paths, nil
	}
	if strings.HasPrefix(tok, "/") || strings.HasPrefix(tok, "~") {
		return []string{tok}, nil
	}
	return nil, fmt.Errorf("invalid matcher %q", tok)
}

func parseNamedMatcher(item statementItem) (string, []string, error) {
	name := item.words[0]
	var paths []string

	applyLine := func(words []string) error {
		if len(words) == 0 {
			return nil
		}
		sub := strings.ToLower(words[0])
		args := words[1:]
		switch sub {
		case "path":
			for _, a := range args {
				if p := strings.TrimSpace(a); p != "" {
					paths = append(paths, p)
				}
			}
		case "path_regexp":
			switch len(args) {
			case 1:
				paths = append(paths, "~"+args[0])
			case 2:
				paths = append(paths, "~"+args[1])
			default:
				return fmt.Errorf("path_regexp requires [name] <pattern>")
			}
		case "method", "header", "query", "remote_ip", "host", "not", "protocol", "file":
			// Non-path conditions are tolerated; if no path was specified, matcher acts as catch-all path.
		default:
			return fmt.Errorf("unsupported matcher condition %q in %s", words[0], name)
		}
		return nil
	}

	if item.sub != nil {
		for _, subItem := range item.sub.body {
			if len(subItem.words) == 0 {
				continue
			}
			if err := applyLine(subItem.words); err != nil {
				return "", nil, err
			}
		}
	} else {
		if len(item.words) < 2 {
			return "", nil, fmt.Errorf("matcher %s requires a condition (e.g. %s path /api/*)", name, name)
		}
		if err := applyLine(item.words[1:]); err != nil {
			return "", nil, err
		}
	}
	if len(paths) == 0 {
		paths = []string{""}
	}
	return name, paths, nil
}

type routeScope struct {
	prefix      string // for handle_path
	stripPrefix string
	matchPaths  []string
	matchers    map[string][]string

	root          string
	rootMatch     string
	index         []string
	browse        bool
	tryFiles      string
	hasFileServer bool
	fileMatch     string

	headers map[string]string
	auth    *ForwardAuthConfig
	routes  []RouteConfig
}

func newRouteScope(matchers map[string][]string) *routeScope {
	return &routeScope{
		matchers: matchers,
	}
}

func (sc *routeScope) addHeaderMutation(field, value string, hasValue bool) error {
	f := strings.TrimSpace(field)
	if f == "" {
		return fmt.Errorf("header directive requires a header name")
	}
	if sc.headers == nil {
		sc.headers = make(map[string]string)
	}
	if strings.HasPrefix(f, "-") {
		sc.headers[f] = ""
		return nil
	}
	if !hasValue {
		return fmt.Errorf("header %q requires a value", f)
	}
	sc.headers[f] = value
	return nil
}

func (sc *routeScope) flushStatic() {
	if !sc.hasFileServer && sc.root == "" {
		return
	}
	if sc.hasFileServer && sc.root == "" {
		sc.root = "."
	}
	if !sc.hasFileServer {
		// Standalone `root` without `file_server` does not emit a static route on its own
		// unless `file_server` is also present.
		return
	}
	path := sc.fileMatch
	if path == "" {
		path = sc.rootMatch
	}
	patterns := sc.matchPaths
	if len(patterns) == 0 {
		patterns = []string{path}
	}
	for _, p := range patterns {
		if p == "" && path != "" {
			p = path
		}
		sc.routes = append(sc.routes, RouteConfig{
			Path:        p,
			StripPrefix: sc.stripPrefix,
			Root:        sc.root,
			Index:       slicesClone(sc.index),
			Browse:      sc.browse,
			Fallback:    sc.tryFiles,
		})
	}
	sc.hasFileServer = false
	sc.root = ""
	sc.tryFiles = ""
	sc.browse = false
	sc.index = nil
}

func (sc *routeScope) finalize() []RouteConfig {
	sc.flushStatic()
	if len(sc.headers) > 0 {
		for i := range sc.routes {
			merged := cloneHeaderMap(sc.headers)
			for k, v := range sc.routes[i].Headers {
				if merged == nil {
					merged = make(map[string]string)
				}
				merged[k] = v
			}
			sc.routes[i].Headers = merged
		}
	}
	if sc.auth != nil {
		for i := range sc.routes {
			if sc.routes[i].Auth == nil {
				sc.routes[i].Auth = sc.auth.Clone()
			}
		}
	}
	return sc.routes
}

func parseRedirectStatus(raw string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "permanent", "perm":
		return 301, true
	case "temporary", "temp", "html":
		return 302, true
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err == nil && n >= 300 && n <= 399 {
		return n, true
	}
	return 0, false
}

func parseRespondArgs(args []string, matchers map[string][]string) ([]string, StaticResponse, error) {
	if len(args) == 0 {
		return []string{""}, StaticResponse{Status: 200}, nil
	}
	paths := []string{""}
	rest := args
	if len(args) >= 2 && isMatcherToken(args[0]) {
		resolved, err := resolveMatcherPaths(args[0], matchers)
		if err != nil {
			return nil, StaticResponse{}, err
		}
		paths = resolved
		rest = args[1:]
	} else if len(args) == 1 && (args[0] == "*" || strings.HasPrefix(args[0], "@")) {
		resolved, err := resolveMatcherPaths(args[0], matchers)
		if err != nil {
			return nil, StaticResponse{}, err
		}
		paths = resolved
		rest = nil
	}

	resp := StaticResponse{Status: 200}
	switch len(rest) {
	case 0:
	case 1:
		if code, err := strconv.Atoi(rest[0]); err == nil && code >= 100 && code <= 599 {
			resp.Status = code
		} else {
			resp.Body = rest[0]
		}
	case 2:
		code, err := strconv.Atoi(rest[1])
		if err != nil || code < 100 || code > 599 {
			return nil, StaticResponse{}, fmt.Errorf("invalid respond status code %q", rest[1])
		}
		resp.Body = rest[0]
		resp.Status = code
	default:
		return nil, StaticResponse{}, fmt.Errorf("respond accepts at most [matcher] [body] [status]")
	}
	return paths, resp, nil
}

func parseRedirArgs(args []string, matchers map[string][]string) ([]string, RedirectTarget, error) {
	if len(args) == 0 {
		return nil, RedirectTarget{}, fmt.Errorf("redir requires a target URL")
	}
	paths := []string{""}
	var to string
	code := 302

	switch len(args) {
	case 1:
		to = args[0]
	case 2:
		if c, ok := parseRedirectStatus(args[1]); ok {
			to = args[0]
			code = c
		} else if isMatcherToken(args[0]) {
			resolved, err := resolveMatcherPaths(args[0], matchers)
			if err != nil {
				return nil, RedirectTarget{}, err
			}
			paths = resolved
			to = args[1]
		} else {
			return nil, RedirectTarget{}, fmt.Errorf("invalid redir code %q", args[1])
		}
	case 3:
		if !isMatcherToken(args[0]) {
			return nil, RedirectTarget{}, fmt.Errorf("invalid redir matcher %q", args[0])
		}
		resolved, err := resolveMatcherPaths(args[0], matchers)
		if err != nil {
			return nil, RedirectTarget{}, err
		}
		paths = resolved
		to = args[1]
		c, ok := parseRedirectStatus(args[2])
		if !ok {
			return nil, RedirectTarget{}, fmt.Errorf("invalid redir status %q", args[2])
		}
		code = c
	default:
		return nil, RedirectTarget{}, fmt.Errorf("redir accepts [matcher] <to> [code]")
	}
	return paths, RedirectTarget{To: to, Code: code}, nil
}

func extractTryFilesFallback(args []string) string {
	for i := len(args) - 1; i >= 0; i-- {
		tok := strings.TrimSpace(args[i])
		if tok == "" || strings.ContainsAny(tok, "{}") {
			continue
		}
		if strings.HasPrefix(tok, "/") {
			return tok
		}
	}
	return ""
}

func deriveHandlePathStripPrefix(rawPattern string) string {
	p := strings.TrimSpace(rawPattern)
	if p == "" || p == "*" || p == "/*" {
		return ""
	}
	p = strings.TrimSuffix(p, "*")
	if p != "/" {
		p = strings.TrimRight(p, "/")
	}
	if p == "/" {
		return ""
	}
	return p
}

func parseReverseProxyDirective(item statementItem, matchers map[string][]string) ([]string, []string, error) {
	args := item.words[1:]
	paths := []string{""}
	if len(args) > 0 && isMatcherToken(args[0]) && (len(args) > 1 || item.sub != nil) {
		resolved, err := resolveMatcherPaths(args[0], matchers)
		if err != nil {
			return nil, nil, err
		}
		paths = resolved
		args = args[1:]
	}

	var targets []string
	for _, a := range args {
		if strings.EqualFold(a, "to") {
			continue
		}
		u, err := NormalizeUpstream(a, "reverse_proxy")
		if err != nil {
			return nil, nil, err
		}
		targets = append(targets, u)
	}
	if item.sub != nil {
		for _, subItem := range item.sub.body {
			if len(subItem.words) == 0 {
				continue
			}
			sname := strings.ToLower(subItem.words[0])
			if sname == "to" {
				for _, a := range subItem.words[1:] {
					u, err := NormalizeUpstream(a, "reverse_proxy")
					if err != nil {
						return nil, nil, err
					}
					targets = append(targets, u)
				}
			}
			// Other Caddy reverse_proxy subdirectives (lb_policy, header_up, health_uri, transport, etc.)
			// are safely accepted.
		}
	}
	if len(targets) == 0 {
		return nil, nil, fmt.Errorf("reverse_proxy requires at least one upstream URL")
	}
	return paths, targets, nil
}

func parseHeaderBlockOrLine(item statementItem, matchers map[string][]string) (hasMatcher bool, matchPaths []string, mutations map[string]string, err error) {
	args := item.words[1:]
	mutations = make(map[string]string)

	if item.sub != nil {
		if len(args) == 1 && isMatcherToken(args[0]) {
			hasMatcher = true
			matchPaths, err = resolveMatcherPaths(args[0], matchers)
			if err != nil {
				return
			}
		} else if len(args) > 0 {
			return false, nil, nil, fmt.Errorf("header block accepts at most a matcher")
		}
		for _, subItem := range item.sub.body {
			if subItem.sub != nil || len(subItem.words) == 0 {
				continue
			}
			f := subItem.words[0]
			if strings.HasPrefix(f, "-") {
				mutations[f] = ""
			} else if len(subItem.words) >= 2 {
				mutations[f] = strings.Join(subItem.words[1:], " ")
			} else {
				err = fmt.Errorf("header %q requires a value", f)
				return
			}
		}
		return
	}

	if len(args) == 0 {
		err = fmt.Errorf("header requires a field name")
		return
	}
	if len(args) >= 2 && isMatcherToken(args[0]) && (strings.HasPrefix(args[1], "-") || len(args) >= 3) {
		hasMatcher = true
		matchPaths, err = resolveMatcherPaths(args[0], matchers)
		if err != nil {
			return
		}
		args = args[1:]
	}
	if len(args) == 1 {
		if strings.HasPrefix(args[0], "-") {
			mutations[args[0]] = ""
			return
		}
		err = fmt.Errorf("header %q requires a value", args[0])
		return
	}
	mutations[args[0]] = strings.Join(args[1:], " ")
	return
}

func parseHandleScope(item statementItem, matchers map[string][]string, isHandlePath bool, globalAuthelia *ForwardAuthConfig) ([]RouteConfig, error) {
	sc := newRouteScope(matchers)
	args := item.words[1:]
	if len(args) > 0 {
		if !isMatcherToken(args[0]) {
			return nil, fmt.Errorf("invalid matcher %q in %s", args[0], item.words[0])
		}
		resolved, err := resolveMatcherPaths(args[0], matchers)
		if err != nil {
			return nil, err
		}
		sc.matchPaths = resolved
		if isHandlePath {
			sc.stripPrefix = deriveHandlePathStripPrefix(args[0])
		}
	}

	for _, subItem := range item.sub.body {
		if len(subItem.words) == 0 {
			continue
		}
		sname := strings.ToLower(subItem.words[0])
		sargs := subItem.words[1:]

		if sname == "authelia" || sname == "forward_auth" {
			authCfg, handled, aErr := parseAuthDirective(subItem, globalAuthelia, matchers, false)
			if aErr != nil {
				return nil, aErr
			}
			if handled {
				sc.auth = authCfg
				continue
			}
		}

		if subItem.sub != nil {
			switch sname {
			case "handle", "handle_path", "route":
				nested, err := parseHandleScope(subItem, matchers, sname == "handle_path", globalAuthelia)
				if err != nil {
					return nil, err
				}
				for i := range nested {
					if nested[i].StripPrefix == "" && sc.stripPrefix != "" {
						nested[i].StripPrefix = sc.stripPrefix
					}
					if nested[i].Auth == nil && sc.auth != nil {
						nested[i].Auth = sc.auth.Clone()
					}
				}
				sc.routes = append(sc.routes, nested...)
				continue
			case "reverse_proxy", "upstream", "proxy":
				paths, targets, err := parseReverseProxyDirective(subItem, matchers)
				if err != nil {
					return nil, err
				}
				emitProxyRoutes(sc, paths, targets)
				continue
			case "file_server":
				if err := applyFileServerItem(sc, subItem); err != nil {
					return nil, err
				}
				continue
			case "respond":
				paths, resp, err := parseRespondBlock(subItem, matchers)
				if err != nil {
					return nil, err
				}
				emitRespondRoutes(sc, paths, resp)
				continue
			case "header":
				_, _, muts, err := parseHeaderBlockOrLine(subItem, matchers)
				if err != nil {
					return nil, err
				}
				for k, v := range muts {
					if sc.headers == nil {
						sc.headers = make(map[string]string)
					}
					sc.headers[k] = v
				}
				continue
			case "encode", "log":
				continue
			default:
				return nil, fmt.Errorf("unsupported block %q inside %s", subItem.words[0], item.words[0])
			}
		}

		switch sname {
		case "reverse_proxy", "upstream", "proxy":
			paths, targets, err := parseReverseProxyDirective(subItem, matchers)
			if err != nil {
				return nil, err
			}
			emitProxyRoutes(sc, paths, targets)
		case "root":
			if err := applyRootDirective(sc, sargs, matchers); err != nil {
				return nil, err
			}
		case "try_files":
			if fb := extractTryFilesFallback(sargs); fb != "" {
				sc.tryFiles = fb
			}
		case "file_server":
			if err := applyFileServerItem(sc, subItem); err != nil {
				return nil, err
			}
		case "respond":
			paths, resp, err := parseRespondArgs(sargs, matchers)
			if err != nil {
				return nil, err
			}
			emitRespondRoutes(sc, paths, resp)
		case "redir":
			paths, redir, err := parseRedirArgs(sargs, matchers)
			if err != nil {
				return nil, err
			}
			emitRedirRoutes(sc, paths, redir)
		case "header":
			_, _, muts, err := parseHeaderBlockOrLine(subItem, matchers)
			if err != nil {
				return nil, err
			}
			for k, v := range muts {
				if sc.headers == nil {
					sc.headers = make(map[string]string)
				}
				sc.headers[k] = v
			}
		case "encode", "log":
			// accepted
		default:
			return nil, fmt.Errorf("unknown directive inside %s: %s", item.words[0], subItem.words[0])
		}
	}

	return sc.finalize(), nil
}

func emitProxyRoutes(sc *routeScope, paths, targets []string) {
	effectivePaths := paths
	if len(effectivePaths) == 1 && effectivePaths[0] == "" && len(sc.matchPaths) > 0 {
		effectivePaths = sc.matchPaths
	}
	for _, p := range effectivePaths {
		sc.routes = append(sc.routes, RouteConfig{
			Path:        p,
			StripPrefix: sc.stripPrefix,
			Upstreams:   slicesClone(targets),
		})
	}
}

func emitRespondRoutes(sc *routeScope, paths []string, resp StaticResponse) {
	effectivePaths := paths
	if len(effectivePaths) == 1 && effectivePaths[0] == "" && len(sc.matchPaths) > 0 {
		effectivePaths = sc.matchPaths
	}
	for _, p := range effectivePaths {
		rCopy := resp
		sc.routes = append(sc.routes, RouteConfig{
			Path:        p,
			StripPrefix: sc.stripPrefix,
			Respond:     &rCopy,
		})
	}
}

func emitRedirRoutes(sc *routeScope, paths []string, redir RedirectTarget) {
	effectivePaths := paths
	if len(effectivePaths) == 1 && effectivePaths[0] == "" && len(sc.matchPaths) > 0 {
		effectivePaths = sc.matchPaths
	}
	for _, p := range effectivePaths {
		rCopy := redir
		sc.routes = append(sc.routes, RouteConfig{
			Path:        p,
			StripPrefix: sc.stripPrefix,
			Redirect:    &rCopy,
		})
	}
}

func applyRootDirective(sc *routeScope, args []string, matchers map[string][]string) error {
	switch len(args) {
	case 1:
		sc.root = args[0]
	case 2:
		if isMatcherToken(args[0]) {
			resolved, err := resolveMatcherPaths(args[0], matchers)
			if err != nil {
				return err
			}
			sc.rootMatch = resolved[0]
			sc.root = args[1]
		} else {
			return fmt.Errorf("root expects [matcher] <path>")
		}
	default:
		return fmt.Errorf("root expects [matcher] <path>")
	}
	return nil
}

func applyFileServerItem(sc *routeScope, item statementItem) error {
	sc.hasFileServer = true
	for _, a := range item.words[1:] {
		if strings.EqualFold(a, "browse") {
			sc.browse = true
			continue
		}
		if isMatcherToken(a) {
			resolved, err := resolveMatcherPaths(a, sc.matchers)
			if err != nil {
				return err
			}
			if len(resolved) > 0 {
				sc.fileMatch = resolved[0]
			}
			continue
		}
		return fmt.Errorf("unexpected file_server argument %q", a)
	}
	if item.sub != nil {
		for _, subItem := range item.sub.body {
			if len(subItem.words) == 0 {
				continue
			}
			sname := strings.ToLower(subItem.words[0])
			sargs := subItem.words[1:]
			switch sname {
			case "root":
				if len(sargs) != 1 {
					return fmt.Errorf("file_server root requires a directory path")
				}
				sc.root = sargs[0]
			case "index":
				sc.index = append(sc.index, sargs...)
			case "browse":
				sc.browse = true
			case "try_files":
				if fb := extractTryFilesFallback(sargs); fb != "" {
					sc.tryFiles = fb
				}
			case "hide", "precompressed", "status", "pass_thru":
				// tolerated
			default:
				return fmt.Errorf("unknown file_server subdirective: %s", subItem.words[0])
			}
		}
	}
	return nil
}

func parseRespondBlock(item statementItem, matchers map[string][]string) ([]string, StaticResponse, error) {
	paths, resp, err := parseRespondArgs(item.words[1:], matchers)
	if err != nil {
		return nil, StaticResponse{}, err
	}
	for _, subItem := range item.sub.body {
		if len(subItem.words) == 0 {
			continue
		}
		switch strings.ToLower(subItem.words[0]) {
		case "body":
			if len(subItem.words) >= 2 {
				resp.Body = subItem.words[1]
			}
		case "status":
			if len(subItem.words) != 2 {
				return nil, StaticResponse{}, fmt.Errorf("respond status requires a status code")
			}
			code, cErr := strconv.Atoi(subItem.words[1])
			if cErr != nil || code < 100 || code > 599 {
				return nil, StaticResponse{}, fmt.Errorf("invalid respond status %q", subItem.words[1])
			}
			resp.Status = code
		case "close":
			// tolerated
		default:
			return nil, StaticResponse{}, fmt.Errorf("unknown respond subdirective: %s", subItem.words[0])
		}
	}
	return paths, resp, nil
}

func parseRuleSourceArgs(args []string, forceDynamic *bool, directiveName string) ([]RuleSource, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("%s requires a path, directory, or URL", directiveName)
	}
	dynamic := false
	if forceDynamic != nil {
		dynamic = *forceDynamic
	}
	var rawSources []string
	for _, a := range args {
		for _, part := range strings.Split(a, ",") {
			p := strings.TrimSpace(part)
			if p == "" {
				continue
			}
			switch strings.ToLower(p) {
			case "dynamic", "watch", "auto", "on", "true":
				dynamic = true
			case "static", "once", "off", "false":
				dynamic = false
			default:
				rawSources = append(rawSources, p)
			}
		}
	}
	if len(rawSources) == 0 {
		return nil, fmt.Errorf("%s requires a file path, directory path, or URL", directiveName)
	}
	out := make([]RuleSource, 0, len(rawSources))
	for _, s := range rawSources {
		rs := RuleSource{Source: s, Dynamic: dynamic}
		if err := rs.Validate(); err != nil {
			return nil, err
		}
		out = append(out, rs)
	}
	return out, nil
}

func parseRuleSourceBlock(block *parsedBlock) (RuleSource, error) {
	var rs RuleSource
	if len(block.header) > 1 {
		rs.Name = strings.TrimSpace(block.header[1])
	}
	for _, subItem := range block.body {
		if subItem.sub != nil || len(subItem.words) == 0 {
			continue
		}
		k := strings.ToLower(subItem.words[0])
		args := subItem.words[1:]
		switch k {
		case "source", "path", "file", "dir", "folder", "url":
			if len(args) != 1 {
				return RuleSource{}, fmt.Errorf("ruleset %s requires a single path or URL", k)
			}
			rs.Source = strings.TrimSpace(args[0])
		case "dynamic", "watch":
			if len(args) != 1 {
				return RuleSource{}, fmt.Errorf("ruleset %s requires on/off or true/false", k)
			}
			b, err := parseBool(args[0], k)
			if err != nil {
				return RuleSource{}, err
			}
			rs.Dynamic = b
		case "name":
			if len(args) != 1 {
				return RuleSource{}, fmt.Errorf("ruleset name requires a value")
			}
			rs.Name = strings.TrimSpace(args[0])
		default:
			return RuleSource{}, fmt.Errorf("unknown ruleset directive: %s", subItem.words[0])
		}
	}
	if err := rs.Validate(); err != nil {
		return RuleSource{}, err
	}
	return rs, nil
}

func parseAuthDirective(
	item statementItem,
	globalAuthelia *ForwardAuthConfig,
	matchers map[string][]string,
	isGlobal bool,
) (*ForwardAuthConfig, bool, error) {
	if len(item.words) == 0 {
		return nil, false, nil
	}
	dname := strings.ToLower(item.words[0])
	if dname != "authelia" && dname != "forward_auth" {
		return nil, false, nil
	}
	args := item.words[1:]

	var cfg *ForwardAuthConfig
	if dname == "authelia" {
		if item.sub == nil {
			switch len(args) {
			case 0:
				if isGlobal || globalAuthelia == nil {
					return nil, true, fmt.Errorf("authelia requires a URL or a global authelia { ... } configuration")
				}
				return globalAuthelia.Clone(), true, nil
			case 1:
				if b, err := parseBool(args[0], "authelia"); err == nil {
					if !b {
						return nil, true, nil
					}
					if isGlobal || globalAuthelia == nil {
						return nil, true, fmt.Errorf("authelia on requires a global authelia { ... } configuration")
					}
					return globalAuthelia.Clone(), true, nil
				}
				cfg = &ForwardAuthConfig{
					Provider: "authelia",
					URL:      args[0],
				}
			default:
				return nil, true, fmt.Errorf("authelia accepts at most a URL or on/off")
			}
		} else {
			if len(args) > 1 {
				return nil, true, fmt.Errorf("authelia block accepts at most a URL")
			}
			if len(args) == 0 && !isGlobal && globalAuthelia != nil {
				cfg = globalAuthelia.Clone()
			} else {
				cfg = &ForwardAuthConfig{Provider: "authelia"}
				if len(args) == 1 {
					cfg.URL = args[0]
				}
			}
		}
	} else {
		// forward_auth [matcher] <target> [ { ... } ]
		cfg = &ForwardAuthConfig{Provider: "forward_auth"}
		rest := args
		if len(rest) >= 1 && isMatcherToken(rest[0]) {
			if len(rest) == 1 && item.sub == nil {
				return nil, true, fmt.Errorf("forward_auth requires an upstream URL after matcher %q", rest[0])
			}
			if len(rest) >= 2 || item.sub != nil {
				if len(rest) >= 2 || rest[0] == "*" || strings.HasPrefix(rest[0], "@") || strings.HasPrefix(rest[0], "/") || strings.HasPrefix(rest[0], "~") {
					if len(rest) >= 2 {
						resolved, err := resolveMatcherPaths(rest[0], matchers)
						if err != nil {
							return nil, true, err
						}
						for _, p := range resolved {
							if p != "" {
								cfg.Paths = append(cfg.Paths, p)
							}
						}
						rest = rest[1:]
					}
				}
			}
		}
		if len(rest) > 0 {
			cfg.URL = rest[0]
		} else if item.sub == nil {
			return nil, true, fmt.Errorf("forward_auth requires an upstream URL")
		}
	}

	if item.sub != nil {
		sawCopyHeaders := false
		sawPaths := false
		sawExcept := false
		for _, subItem := range item.sub.body {
			if len(subItem.words) == 0 {
				continue
			}
			sname := strings.ToLower(subItem.words[0])
			sargs := subItem.words[1:]

			if subItem.sub != nil {
				switch sname {
				case "copy_headers":
					if !sawCopyHeaders {
						cfg.CopyHeaders = nil
						sawCopyHeaders = true
					}
					for _, chLine := range subItem.sub.body {
						if len(chLine.words) == 0 {
							continue
						}
						if len(chLine.words) == 3 && chLine.words[1] == ">" {
							cfg.CopyHeaders = append(cfg.CopyHeaders, chLine.words[0]+">"+chLine.words[2])
						} else {
							cfg.CopyHeaders = append(cfg.CopyHeaders, chLine.words...)
						}
					}
				case "transport":
					for _, tLine := range subItem.sub.body {
						if len(tLine.words) == 0 {
							continue
						}
						tk := strings.ToLower(tLine.words[0])
						if tk == "tls_insecure_skip_verify" {
							if len(tLine.words) == 1 {
								cfg.InsecureSkipTLS = true
							} else {
								b, err := parseBool(tLine.words[1], tk)
								if err != nil {
									return nil, true, err
								}
								cfg.InsecureSkipTLS = b
							}
						}
					}
				default:
					return nil, true, fmt.Errorf("unexpected nested block %q inside %s", subItem.words[0], dname)
				}
				continue
			}

			switch sname {
			case "url", "upstream", "to":
				if len(sargs) != 1 {
					return nil, true, fmt.Errorf("%s %s requires a single URL", dname, sname)
				}
				cfg.URL = sargs[0]
			case "uri":
				if len(sargs) != 1 {
					return nil, true, fmt.Errorf("%s uri requires a path", dname)
				}
				cfg.URI = sargs[0]
			case "copy_headers", "copy_header":
				if len(sargs) == 0 {
					return nil, true, fmt.Errorf("%s %s requires at least one header name", dname, sname)
				}
				if !sawCopyHeaders {
					cfg.CopyHeaders = nil
					sawCopyHeaders = true
				}
				for _, a := range sargs {
					for _, part := range strings.Split(a, ",") {
						if trimmed := strings.TrimSpace(part); trimmed != "" {
							cfg.CopyHeaders = append(cfg.CopyHeaders, trimmed)
						}
					}
				}
			case "header", "header_up":
				if len(sargs) == 0 {
					return nil, true, fmt.Errorf("%s %s requires a header name", dname, sname)
				}
				if cfg.Headers == nil {
					cfg.Headers = make(map[string]string)
				}
				if len(sargs) == 1 && strings.HasPrefix(sargs[0], "-") {
					cfg.Headers[sargs[0]] = ""
				} else if len(sargs) >= 2 {
					cfg.Headers[sargs[0]] = strings.Join(sargs[1:], " ")
				} else {
					return nil, true, fmt.Errorf("%s %s %q requires a value", dname, sname, sargs[0])
				}
			case "path", "paths":
				if len(sargs) == 0 {
					return nil, true, fmt.Errorf("%s %s requires at least one path pattern", dname, sname)
				}
				if !sawPaths {
					cfg.Paths = nil
					sawPaths = true
				}
				cfg.Paths = append(cfg.Paths, sargs...)
			case "except", "except_paths", "bypass":
				if len(sargs) == 0 {
					return nil, true, fmt.Errorf("%s %s requires at least one path pattern", dname, sname)
				}
				if !sawExcept {
					cfg.ExceptPaths = nil
					sawExcept = true
				}
				cfg.ExceptPaths = append(cfg.ExceptPaths, sargs...)
			case "portal_url", "portal", "rd":
				if len(sargs) != 1 {
					return nil, true, fmt.Errorf("%s %s requires a URL", dname, sname)
				}
				cfg.PortalURL = sargs[0]
			case "timeout":
				if len(sargs) != 1 {
					return nil, true, fmt.Errorf("%s timeout requires seconds or a duration (e.g. 5s)", dname)
				}
				sec, err := ParseDurationSeconds(sargs[0], "auth timeout")
				if err != nil {
					return nil, true, err
				}
				cfg.Timeout = sec
			case "insecure_skip_tls", "tls_insecure_skip_verify":
				if len(sargs) == 0 {
					cfg.InsecureSkipTLS = true
				} else if len(sargs) == 1 {
					b, err := parseBool(sargs[0], sname)
					if err != nil {
						return nil, true, err
					}
					cfg.InsecureSkipTLS = b
				} else {
					return nil, true, fmt.Errorf("%s %s accepts at most true or false", dname, sname)
				}
			default:
				return nil, true, fmt.Errorf("unknown directive inside %s: %s", dname, subItem.words[0])
			}
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, true, err
	}
	return cfg, true, nil
}
