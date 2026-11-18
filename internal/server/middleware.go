package server

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"

	"gymbag/internal/proxy"
	"gymbag/internal/waf"
)

type routeCtxKey struct{}

func withSiteRoute(ctx context.Context, route *proxy.SiteRoute) context.Context {
	return context.WithValue(ctx, routeCtxKey{}, route)
}

func siteRouteFromContext(ctx context.Context) *proxy.SiteRoute {
	v, _ := ctx.Value(routeCtxKey{}).(*proxy.SiteRoute)
	return v
}

// ParseTrustedProxies parses IP or CIDR strings into netip.Prefix entries.
func ParseTrustedProxies(rawList []string) []netip.Prefix {
	var out []netip.Prefix
	for _, raw := range rawList {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if p, err := netip.ParsePrefix(s); err == nil {
			out = append(out, p)
			continue
		}
		if addr, err := netip.ParseAddr(s); err == nil {
			out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
		}
	}
	return out
}

// ResolveClientIP extracts the client IP from RemoteAddr, consulting X-Forwarded-For / X-Real-IP
// only when the immediate TCP peer belongs to trustedProxies.
func ResolveClientIP(r *http.Request, trustedProxies []netip.Prefix) string {
	peerIP := stripPort(r.RemoteAddr)
	if len(trustedProxies) > 0 && isTrustedIP(peerIP, trustedProxies) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			for i := len(parts) - 1; i >= 0; i-- {
				candidate := strings.TrimSpace(parts[i])
				if candidate != "" && !isTrustedIP(candidate, trustedProxies) {
					return candidate
				}
			}
		}
		if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
			return xri
		}
	}
	return peerIP
}

func stripPort(addr string) string {
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(trimmed); err == nil {
		return strings.Trim(h, "[]")
	}
	return strings.Trim(trimmed, "[]")
}

func isTrustedIP(ipStr string, trusted []netip.Prefix) bool {
	addr, err := netip.ParseAddr(ipStr)
	if err != nil {
		return false
	}
	for _, p := range trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// RequestScheme returns "https" for TLS requests and "http" otherwise.
func RequestScheme(r *http.Request) string {
	if r.TLS != nil || strings.EqualFold(r.URL.Scheme, "https") {
		return "https"
	}
	return "http"
}

// ServerHeaderMiddleware sets a consistent Server response header.
func ServerHeaderMiddleware(serverName string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", serverName)
		next.ServeHTTP(w, r)
	})
}

// AllowedHostMiddleware validates the Host header against the configured SiteRouter
// and attaches the matched *proxy.SiteRoute to the request context.
func AllowedHostMiddleware(router *proxy.SiteRouter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route, ok := router.Resolve(r.Host)
		if !ok {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("Invalid host header"))
			return
		}
		next.ServeHTTP(w, r.WithContext(withSiteRoute(r.Context(), route)))
	})
}

// HSTSMiddleware adds Strict-Transport-Security on non-WebSocket HTTP responses when enabled.
func HSTSMiddleware(enabled *atomic.Bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if enabled != nil && enabled.Load() && !proxy.IsWebSocketUpgrade(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

// WAFMiddleware enforces per-website WAF policies, issuing PoW challenges or block pages as needed.
func WAFMiddleware(
	challenges waf.ChallengeManager,
	pages waf.PageRenderer,
	trustedProxies []netip.Prefix,
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := siteRouteFromContext(r.Context())
		if route == nil {
			http.Error(w, "Invalid host header", http.StatusBadRequest)
			return
		}

		clientIP := ResolveClientIP(r, trustedProxies)
		incoming := waf.FromHTTPRequest(r, clientIP)
		decision := route.Gate.Decide(r.Context(), incoming)

		if proxy.IsWebSocketUpgrade(r) {
			if decision.Action != waf.ActionAllow {
				status := decision.StatusCode
				if status == 0 {
					status = http.StatusForbidden
				}
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(status)
				_, _ = w.Write([]byte("WebSocket clearance required"))
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		switch decision.Action {
		case waf.ActionDeny:
			pages.WriteReject(w, decision.Reason, decision.StatusCode)
			return
		case waf.ActionChallenge:
			chalID, diff, err := challenges.Issue(
				r.Context(),
				incoming.Host,
				clientIP,
				route.Config.WAF.Difficulty,
				route.Config.WAF.ChallengeTTL,
			)
			if err != nil {
				http.Error(w, "failed to issue challenge", http.StatusInternalServerError)
				return
			}
			pages.WriteChallenge(w, chalID, diff)
			return
		default:
			next.ServeHTTP(w, r)
		}
	})
}
