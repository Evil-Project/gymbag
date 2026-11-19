package server

import (
	"encoding/json"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"gymbag/internal/host"
	"gymbag/internal/proxy"
	"gymbag/internal/tlsmanager"
	"gymbag/internal/waf"
)

// ACMEChallengeLookup looks up HTTP-01 validation tokens (ISP).
type ACMEChallengeLookup interface {
	Lookup(token string) (string, bool)
}

// WriteACMEChallenge serves GET/HEAD /.well-known/acme-challenge/{token}.
func WriteACMEChallenge(w http.ResponseWriter, r *http.Request, board ACMEChallengeLookup, token string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	authz, ok := board.Lookup(token)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write([]byte(authz))
	}
}

// GatewayHandler handles ACME challenges, RESTful WAF verification endpoints, static assets,
// WebSocket upgrades, and HTTP reverse proxying.
type GatewayHandler struct {
	challenges     waf.ChallengeManager
	clearance      waf.ClearanceManager
	httpProxy      *proxy.HTTPProxy
	wsProxy        *proxy.WebSocketProxy
	acmeBoard      ACMEChallengeLookup
	trustedProxies []netip.Prefix
}

// NewGatewayHandler constructs the core HTTPS GatewayHandler.
func NewGatewayHandler(
	challenges waf.ChallengeManager,
	clearance waf.ClearanceManager,
	httpProxy *proxy.HTTPProxy,
	wsProxy *proxy.WebSocketProxy,
	acmeBoard *tlsmanager.HTTP01Board,
	trustedProxies []netip.Prefix,
) *GatewayHandler {
	return &GatewayHandler{
		challenges:     challenges,
		clearance:      clearance,
		httpProxy:      httpProxy,
		wsProxy:        wsProxy,
		acmeBoard:      acmeBoard,
		trustedProxies: trustedProxies,
	}
}

func (g *GatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route := siteRouteFromContext(r.Context())
	if route == nil {
		http.Error(w, "Invalid host header", http.StatusBadRequest)
		return
	}
	clientIP := ResolveClientIP(r, g.trustedProxies)
	g.ServeRouted(w, r, route, host.FromHeader(r.Host), clientIP, proxy.IsWebSocketUpgrade(r))
}

// ServeRouted serves a request whose route, normalized host, client IP, and WebSocket upgrade state
// have already been resolved by the fused gateway fast path.
func (g *GatewayHandler) ServeRouted(
	w http.ResponseWriter,
	r *http.Request,
	route *proxy.SiteRoute,
	hostName, clientIP string,
	isWS bool,
) {
	path := r.URL.Path
	if path == "" {
		path = "/"
	}

	// Fast prefix check: internal endpoints all start with "/."
	if len(path) >= 2 && path[0] == '/' && path[1] == '.' {
		// 1. ACME HTTP-01 challenge endpoint
		if token, ok := strings.CutPrefix(path, "/.well-known/acme-challenge/"); ok {
			if token == "" || strings.Contains(token, "/") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			WriteACMEChallenge(w, r, g.acmeBoard, token)
			return
		}

		// 2. WAF internal & RESTful endpoints (/.waf/*)
		if strings.HasPrefix(path, "/.waf/") || path == "/.waf" {
			g.serveWAFEndpoint(w, r, route, path, hostName, clientIP)
			return
		}
	}

	scheme := RequestScheme(r)

	// 3. WebSocket upgrade tunnel
	if isWS {
		g.wsProxy.Forward(w, r, route, clientIP, scheme)
		return
	}

	// 4. Streaming HTTP reverse proxy
	g.httpProxy.Forward(w, r, route, clientIP, scheme)
}

func (g *GatewayHandler) serveWAFEndpoint(w http.ResponseWriter, r *http.Request, route *proxy.SiteRoute, path, hostName, clientIP string) {
	// Static mascot assets
	if name, ok := strings.CutPrefix(path, "/.waf/assets/"); ok {
		serveAsset(w, r, name)
		return
	}
	if name, ok := strings.CutPrefix(path, "/.waf/v1/assets/"); ok {
		serveAsset(w, r, name)
		return
	}

	switch path {
	case "/.waf/verify", "/.waf/v1/verifications":
		g.handleVerify(w, r, route, hostName, clientIP)
	case "/.waf/v1/challenges":
		g.handleCreateChallenge(w, r, route, hostName, clientIP)
	case "/.waf/v1/session":
		g.handleSession(w, r, route, hostName, clientIP)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func serveAsset(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	data, etag, ok := waf.AssetData(name)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("ETag", etag)
	if inm := r.Header.Get("If-None-Match"); inm != "" && inm == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

func (g *GatewayHandler) handleVerify(w http.ResponseWriter, r *http.Request, route *proxy.SiteRoute, hostName, clientIP string) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Allow", "POST, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST, OPTIONS")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"ok":    false,
			"error": map[string]string{"code": "method_not_allowed", "message": "POST is required"},
		})
		return
	}
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(strings.ToLower(ct), "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]any{
			"ok":    false,
			"error": map[string]string{"code": "unsupported_media_type", "message": "Content-Type must be application/json"},
		})
		return
	}

	sub, errStatus, err := waf.ParseSubmission(r.Body)
	if err != nil {
		writeJSON(w, errStatus, map[string]any{
			"ok":    false,
			"error": map[string]string{"code": "invalid_submission", "message": err.Error()},
		})
		return
	}

	accepted, verifyErr := g.challenges.Verify(r.Context(), hostName, clientIP, sub)
	if verifyErr != nil || !accepted {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"ok": false,
		})
		return
	}

	wafCfg := route.Config.WAF
	cookieVal := g.clearance.Issue(hostName, clientIP, wafCfg.ClearanceTTL)
	http.SetCookie(w, &http.Cookie{
		Name:     wafCfg.CookieName,
		Value:    cookieVal,
		Path:     "/",
		MaxAge:   wafCfg.ClearanceTTL,
		HttpOnly: true,
		Secure:   wafCfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"expires_in": wafCfg.ClearanceTTL,
	})
}

func (g *GatewayHandler) handleCreateChallenge(w http.ResponseWriter, r *http.Request, route *proxy.SiteRoute, hostName, clientIP string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"error": map[string]string{"code": "method_not_allowed", "message": "POST is required"},
		})
		return
	}
	chalID, diff, err := g.challenges.Issue(
		r.Context(),
		hostName,
		clientIP,
		route.Config.WAF.Difficulty,
		route.Config.WAF.ChallengeTTL,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": map[string]string{"code": "internal_error", "message": "unable to create challenge"},
		})
		return
	}
	w.Header().Set("Location", "/.waf/v1/challenges/"+chalID)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]any{
		"challenge_id": chalID,
		"difficulty":   diff,
		"expires_in":   route.Config.WAF.ChallengeTTL,
	})
}

func (g *GatewayHandler) handleSession(w http.ResponseWriter, r *http.Request, route *proxy.SiteRoute, hostName, clientIP string) {
	wafCfg := route.Config.WAF

	switch r.Method {
	case http.MethodGet:
		var cookieVal string
		if c, err := r.Cookie(wafCfg.CookieName); err == nil {
			cookieVal = c.Value
		}
		var (
			cleared   bool
			expiresAt int64
		)
		if !route.Config.Protect {
			cleared = true
		} else if inspector, ok := g.clearance.(waf.ClearanceInspector); ok {
			expiresAt, cleared = inspector.Parse(cookieVal, hostName, clientIP)
		} else {
			cleared = g.clearance.Valid(cookieVal, hostName, clientIP)
		}
		w.Header().Set("Cache-Control", "no-store")
		payload := map[string]any{
			"site":          route.Config.Domain,
			"protected":     route.Config.Protect,
			"verify_level":  int(wafCfg.VerifyLevel),
			"verify_mode":   wafCfg.VerifyLevel.Label(),
			"clearance_ttl": wafCfg.ClearanceTTL,
			"cleared":       cleared,
		}
		if cleared && expiresAt > 0 {
			payload["expires_at"] = time.Unix(expiresAt, 0).UTC().Format(time.RFC3339)
			if rem := expiresAt - time.Now().Unix(); rem >= 0 {
				payload["expires_in"] = rem
			}
		}
		writeJSON(w, http.StatusOK, payload)
	case http.MethodDelete:
		http.SetCookie(w, &http.Cookie{
			Name:     wafCfg.CookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   wafCfg.CookieSecure,
			SameSite: http.SameSiteLaxMode,
		})
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "GET, DELETE")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"error": map[string]string{"code": "method_not_allowed", "message": "GET or DELETE is required"},
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}
