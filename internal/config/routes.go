package config

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// StaticResponse is a fixed HTTP response written directly by Gymbag (Caddy `respond`, nginx `return 200 "..."`).
type StaticResponse struct {
	Status int    `json:"status,omitempty"` // default 200
	Body   string `json:"body,omitempty"`
}

// RedirectTarget is an HTTP redirect (Caddy `redir`, nginx `return 301 ...`, Apache `Redirect`).
//
// To may contain request placeholders that are expanded at request time:
//   - {scheme} request scheme ("https" or "http")
//   - {host}   request host (without port)
//   - {uri}    request path + "?" + query (after StripPrefix, if any)
//   - {path}   request path (after StripPrefix, if any)
//   - {query}  raw query string (without "?")
type RedirectTarget struct {
	To   string `json:"to"`
	Code int    `json:"code,omitempty"` // default 302
}

// RouteConfig is a path-matched handler inside a site. Routes are evaluated in order; the first route whose
// Path matches handles the request. Requests that match no route fall through to the site's Upstreams
// (or receive 404 when the site has no Upstreams).
//
// Exactly one handler must be set: Upstreams (reverse proxy), Root (static file server), Respond, or Redirect.
//
// Path matcher syntax (see CompilePathMatcher):
//   - "" or "*" or "/*"   matches every request path
//   - "/api/*", "*.php"   glob; "*" matches any sequence of characters (including "/")
//   - "~^/v[0-9]+/"       regular expression (Go RE2 syntax) after the leading "~"
//   - "/exact"            exact path match
type RouteConfig struct {
	Path        string   `json:"path,omitempty"`
	StripPrefix string   `json:"strip_prefix,omitempty"` // removed from the request path before proxying / file lookup / placeholders
	Upstreams   []string `json:"upstreams,omitempty"`

	Root     string   `json:"root,omitempty"`     // static file server root directory
	Index    []string `json:"index,omitempty"`    // directory index files (default ["index.html"])
	Browse   bool     `json:"browse,omitempty"`   // allow directory listings when no index file exists
	Fallback string   `json:"fallback,omitempty"` // file (relative to Root) served when the requested file is missing, e.g. "/index.html" for SPAs

	Respond  *StaticResponse `json:"respond,omitempty"`
	Redirect *RedirectTarget `json:"redirect,omitempty"`

	// Headers mutates response headers. A key prefixed with "-" deletes that header (value ignored);
	// any other key sets (replaces) the header to the given value.
	Headers map[string]string `json:"headers,omitempty"`
}

// HandlerKind reports which handler the route uses: "proxy", "static", "respond", "redirect", or "" if none.
func (r RouteConfig) HandlerKind() string {
	switch {
	case len(r.Upstreams) > 0:
		return "proxy"
	case r.Root != "":
		return "static"
	case r.Respond != nil:
		return "respond"
	case r.Redirect != nil:
		return "redirect"
	default:
		return ""
	}
}

// Clone returns a deep copy of the route.
func (r RouteConfig) Clone() RouteConfig {
	out := r
	out.Upstreams = append([]string(nil), r.Upstreams...)
	out.Index = append([]string(nil), r.Index...)
	if r.Respond != nil {
		v := *r.Respond
		out.Respond = &v
	}
	if r.Redirect != nil {
		v := *r.Redirect
		out.Redirect = &v
	}
	out.Headers = cloneHeaderMap(r.Headers)
	return out
}

func cloneHeaderMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// Validate normalizes and checks the route.
func (r *RouteConfig) Validate() error {
	r.Path = strings.TrimSpace(r.Path)
	if r.Path == "*" || r.Path == "/*" {
		r.Path = ""
	}
	if _, err := CompilePathMatcher(r.Path); err != nil {
		return err
	}
	r.StripPrefix = strings.TrimSpace(r.StripPrefix)
	if r.StripPrefix != "" && !strings.HasPrefix(r.StripPrefix, "/") {
		return fmt.Errorf("route strip_prefix %q must start with '/'", r.StripPrefix)
	}

	handlers := 0
	if len(r.Upstreams) > 0 {
		handlers++
		normalized := make([]string, 0, len(r.Upstreams))
		for _, raw := range r.Upstreams {
			u, err := NormalizeUpstream(raw, "route upstream")
			if err != nil {
				return err
			}
			normalized = append(normalized, u)
		}
		r.Upstreams = normalized
	}
	r.Root = strings.TrimSpace(r.Root)
	if r.Root != "" {
		handlers++
		for _, idx := range r.Index {
			if strings.TrimSpace(idx) == "" || strings.ContainsAny(idx, "/\\") {
				return fmt.Errorf("route index file %q must be a plain file name", idx)
			}
		}
		if r.Fallback != "" && !strings.HasPrefix(r.Fallback, "/") {
			r.Fallback = "/" + r.Fallback
		}
	} else if len(r.Index) > 0 || r.Browse || r.Fallback != "" {
		return fmt.Errorf("route index, browse, and fallback require root")
	}
	if r.Respond != nil {
		handlers++
		if r.Respond.Status == 0 {
			r.Respond.Status = http.StatusOK
		}
		if r.Respond.Status < 100 || r.Respond.Status > 599 {
			return fmt.Errorf("route respond status %d must be between 100 and 599", r.Respond.Status)
		}
	}
	if r.Redirect != nil {
		handlers++
		r.Redirect.To = strings.TrimSpace(r.Redirect.To)
		if r.Redirect.To == "" {
			return fmt.Errorf("route redirect requires a target")
		}
		if r.Redirect.Code == 0 {
			r.Redirect.Code = http.StatusFound
		}
		if r.Redirect.Code < 300 || r.Redirect.Code > 399 {
			return fmt.Errorf("route redirect code %d must be a 3xx status", r.Redirect.Code)
		}
	}
	if handlers == 0 {
		return fmt.Errorf("route %q requires one handler: upstreams, root, respond, or redirect", r.Path)
	}
	if handlers > 1 {
		return fmt.Errorf("route %q must use exactly one handler (upstreams, root, respond, or redirect)", r.Path)
	}
	return validateHeaderMap(r.Headers)
}

func validateHeaderMap(h map[string]string) error {
	for k, v := range h {
		name := strings.TrimSpace(k)
		if strings.HasPrefix(name, "-") || strings.HasPrefix(name, "+") {
			name = strings.TrimSpace(name[1:])
		}
		if name == "" {
			return fmt.Errorf("header name must not be empty")
		}
		if strings.ContainsAny(name, " \t\r\n:") || containsCtl(v) {
			return fmt.Errorf("invalid header %q", k)
		}
	}
	return nil
}

func containsCtl(s string) bool {
	for _, c := range s {
		if (c < 0x20 && c != '\t') || c == 0x7f {
			return true
		}
	}
	return false
}

// CompilePathMatcher compiles a route path pattern into a matcher function. See RouteConfig for syntax.
func CompilePathMatcher(pattern string) (func(string) bool, error) {
	p := strings.TrimSpace(pattern)
	switch {
	case p == "" || p == "*" || p == "/*":
		return func(string) bool { return true }, nil
	case strings.HasPrefix(p, "~"):
		expr := strings.TrimPrefix(p, "~")
		if expr == "" {
			return nil, fmt.Errorf("route path regex must not be empty")
		}
		re, err := regexp.Compile(expr)
		if err != nil {
			return nil, fmt.Errorf("invalid route path regex %q: %w", expr, err)
		}
		return re.MatchString, nil
	case strings.Contains(p, "*"):
		if !strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "*") {
			return nil, fmt.Errorf("route path %q must start with '/', '*', or '~'", p)
		}
		parts := strings.Split(p, "*")
		for i := range parts {
			parts[i] = regexp.QuoteMeta(parts[i])
		}
		re := regexp.MustCompile("^" + strings.Join(parts, ".*") + "$")
		return re.MatchString, nil
	default:
		if !strings.HasPrefix(p, "/") {
			return nil, fmt.Errorf("route path %q must start with '/', '*', or '~'", p)
		}
		return func(s string) bool { return s == p }, nil
	}
}

// ApplyStripPrefix removes prefix from path. The result always starts with "/".
func ApplyStripPrefix(path, prefix string) string {
	if prefix == "" {
		return path
	}
	if rest, ok := strings.CutPrefix(path, prefix); ok {
		path = rest
	} else if rest, ok := strings.CutPrefix(path, strings.TrimRight(prefix, "/")); ok {
		path = rest
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}
