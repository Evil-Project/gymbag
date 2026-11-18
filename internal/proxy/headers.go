package proxy

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"gymbag/internal/host"
)

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
	opts := make(map[string]struct{})
	for _, raw := range h.Values("Connection") {
		for _, part := range strings.Split(raw, ",") {
			if tok := strings.ToLower(strings.TrimSpace(part)); tok != "" {
				opts[tok] = struct{}{}
			}
		}
	}
	return opts
}

// ForwardRequestHeaders builds sanitized upstream request headers with X-Forwarded-* metadata.
func ForwardRequestHeaders(src http.Header, rawHost, clientIP, scheme string) http.Header {
	connOpts := connectionOptions(src)
	dst := make(http.Header, len(src)+4)
	for k, vals := range src {
		lower := strings.ToLower(k)
		if _, hop := hopByHopHeaders[lower]; hop {
			continue
		}
		if lower == "host" || lower == "content-length" {
			continue
		}
		if _, customHop := connOpts[lower]; customHop {
			continue
		}
		copied := make([]string, len(vals))
		copy(copied, vals)
		dst[k] = copied
	}
	if rawHost != "" {
		dst.Set("X-Forwarded-Host", rawHost)
	}
	dst.Set("X-Forwarded-For", clientIP)
	dst.Set("X-Real-IP", clientIP)
	dst.Set("X-Forwarded-Proto", scheme)
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
	target, err := url.Parse(location)
	if err != nil || target.Scheme == "" || target.Hostname() == "" {
		return location
	}
	base, err := url.Parse(upstream)
	if err != nil || base.Hostname() == "" {
		return location
	}
	if !strings.EqualFold(target.Hostname(), base.Hostname()) {
		return location
	}
	if effectivePort(target) != effectivePort(base) {
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
	connOpts := connectionOptions(src)
	var origin string
	if hostHeader != "" {
		origin = PublicOrigin(hostHeader, httpsPort)
	}
	for k, vals := range src {
		lower := strings.ToLower(k)
		if _, hop := hopByHopHeaders[lower]; hop {
			continue
		}
		if lower == "alt-svc" || lower == "strict-transport-security" {
			continue
		}
		if _, customHop := connOpts[lower]; customHop {
			continue
		}
		for _, v := range vals {
			if lower == "location" && origin != "" {
				v = RewriteLocation(v, upstream, origin)
			}
			dst.Add(k, v)
		}
	}
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
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
		return false
	}
	for _, raw := range r.Header.Values("Connection") {
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
