package proxy

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"gymbag/internal/host"
)

func isHopByHopCanonical(k string) bool {
	switch k {
	case "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
		"Te", "Trailer", "Trailers", "Transfer-Encoding", "Upgrade":
		return true
	}
	return false
}

var hopByHopHeaders = map[string]struct{}{
	"connection":          {},
	"keep-alive":          {},
	"proxy-authenticate":  {},
	"proxy-authorization": {},
	"te":                  {},
	"trailer":             {},
	"trailers":            {},
	"transfer-encoding":   {},
	"upgrade":             {},
}

func connectionOptions(h http.Header) map[string]struct{} {
	connVals := h["Connection"]
	if len(connVals) == 0 {
		return nil
	}
	// Fast check: common single "keep-alive" or "close" token has no custom header names to strip.
	if len(connVals) == 1 {
		v := connVals[0]
		if v == "keep-alive" || v == "Keep-Alive" || v == "close" || v == "Close" {
			return nil
		}
	}
	var opts map[string]struct{}
	for _, raw := range connVals {
		for _, part := range strings.Split(raw, ",") {
			tok := strings.ToLower(strings.TrimSpace(part))
			if tok == "" || tok == "keep-alive" || tok == "close" {
				continue
			}
			if opts == nil {
				opts = make(map[string]struct{}, 2)
			}
			opts[tok] = struct{}{}
		}
	}
	return opts
}

// ForwardRequestHeaders builds sanitized upstream request headers with X-Forwarded-* metadata.
func ForwardRequestHeaders(src http.Header, rawHost, clientIP, scheme string) http.Header {
	connOpts := connectionOptions(src)
	dst := make(http.Header, len(src)+4)
	for k, vals := range src {
		if isHopByHopCanonical(k) || k == "Host" || k == "Content-Length" {
			continue
		}
		if len(connOpts) > 0 {
			lower := strings.ToLower(k)
			if _, hop := hopByHopHeaders[lower]; hop || lower == "host" || lower == "content-length" {
				continue
			}
			if _, customHop := connOpts[lower]; customHop {
				continue
			}
		}
		// Copy slice header only when needed; single-element reuse is safe because Transport only reads it.
		dst[k] = vals
	}
	if rawHost != "" {
		dst["X-Forwarded-Host"] = []string{rawHost}
	}
	dst["X-Forwarded-For"] = []string{clientIP}
	dst["X-Real-Ip"] = []string{clientIP}
	dst["X-Forwarded-Proto"] = []string{scheme}
	return dst
}

// PublicOrigin returns the downstream public HTTPS origin for Location header rewriting.
func PublicOrigin(hostHeader string, httpsPort int) string {
	h := host.FormatForURL(host.FromHeader(hostHeader))
	if httpsPort == 443 || httpsPort == 0 {
		return "https://" + h
	}
	return fmt.Sprintf("https://%s:%d", h, httpsPort)
}

func effectivePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if strings.EqualFold(u.Scheme, "https") || strings.EqualFold(u.Scheme, "wss") {
		return "443"
	}
	return "80"
}

// RewriteLocation rewrites an upstream redirect Location URL to the public origin when host & port match upstream.
func RewriteLocation(location, upstream, origin string) string {
	if origin == "" {
		return location
	}
	base, err := url.Parse(upstream)
	if err != nil || base.Hostname() == "" {
		return location
	}
	return rewriteLocationParsed(location, base.Hostname(), effectivePort(base), origin)
}

func rewriteLocationParsed(location, baseHost, basePort, origin string) string {
	if origin == "" || baseHost == "" {
		return location
	}
	target, err := url.Parse(location)
	if err != nil || target.Scheme == "" || target.Hostname() == "" {
		return location
	}
	if !strings.EqualFold(target.Hostname(), baseHost) {
		return location
	}
	if effectivePort(target) != basePort {
		return location
	}
	path := target.EscapedPath()
	if path == "" {
		path = "/"
	}
	if target.RawQuery != "" {
		return origin + path + "?" + target.RawQuery
	}
	return origin + path
}

// CopyResponseHeaders copies upstream response headers to dst, stripping hop-by-hop/HSTS/Alt-Svc
// headers, preserving multi-value headers (Set-Cookie), and rewriting Location headers.
func CopyResponseHeaders(dst, src http.Header, upstream, hostHeader string, httpsPort int) {
	copyResponseHeadersParsed(dst, src, parseUpstream(upstream), hostHeader, httpsPort)
}

func copyResponseHeadersParsed(dst, src http.Header, up ParsedUpstream, hostHeader string, httpsPort int) {
	connOpts := connectionOptions(src)
	var origin string
	originReady := false

	for k, vals := range src {
		if isHopByHopCanonical(k) || k == "Alt-Svc" || k == "Strict-Transport-Security" {
			continue
		}
		if len(connOpts) > 0 {
			lower := strings.ToLower(k)
			if _, hop := hopByHopHeaders[lower]; hop || lower == "alt-svc" || lower == "strict-transport-security" {
				continue
			}
			if _, customHop := connOpts[lower]; customHop {
				continue
			}
		}
		if k == "Location" {
			if !originReady {
				if hostHeader != "" {
					origin = PublicOrigin(hostHeader, httpsPort)
				}
				originReady = true
			}
			if origin != "" {
				for _, v := range vals {
					dst.Add(k, rewriteLocationParsed(v, up.Hostname, up.EffectivePort, origin))
				}
				continue
			}
		}
		if existing, exists := dst[k]; exists && len(existing) > 0 {
			dst[k] = append(existing, vals...)
		} else {
			dst[k] = vals
		}
	}
}

// BuildUpstreamURL joins a pre-parsed upstream URL with reqPath and rawQuery without calling url.Parse.
func BuildUpstreamURL(up ParsedUpstream, reqPath, rawQuery string) (url.URL, error) {
	if up.URL == nil {
		u, err := UpstreamURL(up.Raw, reqPath, rawQuery)
		if err != nil {
			return url.URL{}, err
		}
		return *u, nil
	}
	suffix := reqPath
	if suffix == "" {
		suffix = "/"
	} else if suffix[0] != '/' {
		suffix = "/" + suffix
	}
	joined := suffix
	if up.BasePath != "" {
		joined = up.BasePath + suffix
	}
	if joined == "" {
		joined = "/"
	}

	out := *up.URL
	if strings.IndexByte(joined, '%') >= 0 {
		out.Path = unescapeOrKeep(joined)
		out.RawPath = joined
	} else {
		out.Path = joined
		out.RawPath = ""
	}
	out.RawQuery = rawQuery
	return out, nil
}

// UpstreamURL joins an upstream base URL with an incoming request path and raw query string.
func UpstreamURL(upstream, reqPath, rawQuery string) (*url.URL, error) {
	base, err := url.Parse(upstream)
	if err != nil {
		return nil, err
	}
	prefix := strings.TrimRight(base.EscapedPath(), "/")
	suffix := reqPath
	if suffix == "" {
		suffix = "/"
	} else if !strings.HasPrefix(suffix, "/") {
		suffix = "/" + suffix
	}
	joined := prefix + suffix
	if joined == "" {
		joined = "/"
	}
	out := *base
	out.Path = unescapeOrKeep(joined)
	out.RawPath = joined
	out.RawQuery = rawQuery
	return &out, nil
}

// WebSocketUpstreamURL builds the ws:// or wss:// upstream URL for a WebSocket upgrade.
func WebSocketUpstreamURL(upstream, reqPath, rawQuery string) (*url.URL, error) {
	u, err := UpstreamURL(upstream, reqPath, rawQuery)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(u.Scheme, "https") {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	// Strip default port from Host to match RFC 6455 Host header expectations
	h := host.FormatForURL(u.Hostname())
	p := u.Port()
	if (u.Scheme == "wss" && p == "443") || (u.Scheme == "ws" && p == "80") || p == "" {
		u.Host = h
	} else {
		u.Host = fmt.Sprintf("%s:%s", h, p)
	}
	return u, nil
}

// IsWebSocketUpgrade reports whether r is an HTTP/1.1 WebSocket upgrade handshake.
func IsWebSocketUpgrade(r *http.Request) bool {
	upgrades := r.Header["Upgrade"]
	if len(upgrades) == 0 {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(upgrades[0]), "websocket") {
		return false
	}
	for _, raw := range r.Header["Connection"] {
		if strings.EqualFold(strings.TrimSpace(raw), "upgrade") {
			return true
		}
		for _, part := range strings.Split(raw, ",") {
			if strings.EqualFold(strings.TrimSpace(part), "upgrade") {
				return true
			}
		}
	}
	return false
}

// WebSocketForwardHeaders prepares headers for an upstream HTTP/1.1 WebSocket upgrade request.
func WebSocketForwardHeaders(src http.Header, rawHost, clientIP, scheme string) http.Header {
	connOpts := connectionOptions(src)
	dst := make(http.Header, len(src)+6)
	for k, vals := range src {
		lower := strings.ToLower(k)
		if lower == "host" || lower == "content-length" {
			continue
		}
		// Strip hop-by-hop headers except Upgrade/Connection needed for WebSocket handshake
		if lower != "upgrade" && lower != "connection" {
			if _, hop := hopByHopHeaders[lower]; hop {
				continue
			}
			if _, customHop := connOpts[lower]; customHop {
				continue
			}
		}
		copied := make([]string, len(vals))
		copy(copied, vals)
		dst[k] = copied
	}
	dst.Set("Connection", "Upgrade")
	dst.Set("Upgrade", "websocket")
	if rawHost != "" {
		dst.Set("X-Forwarded-Host", rawHost)
	}
	dst.Set("X-Forwarded-For", clientIP)
	dst.Set("X-Real-IP", clientIP)
	dst.Set("X-Forwarded-Proto", scheme)
	return dst
}

func unescapeOrKeep(escapedPath string) string {
	u, err := url.PathUnescape(escapedPath)
	if err != nil {
		return escapedPath
	}
	return u
}
