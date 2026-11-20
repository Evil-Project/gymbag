package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/netip"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"

	"gymbag/internal/config"
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

	// 3. Path-matched routes (proxy, static files, respond, redirect)
	if matched := route.MatchRoute(path); matched != nil {
		g.serveCompiledRoute(w, r, route, matched, path, hostName, clientIP, scheme, isWS)
		return
	}

	// 4. Fallback when no site-wide upstreams are configured
	if len(route.Config.Upstreams) == 0 {
		proxy.ApplyResponseHeaders(w.Header(), route.Config.Headers)
		http.NotFound(w, r)
		return
	}

	// 5. WebSocket upgrade tunnel
	if isWS {
		g.wsProxy.Forward(w, r, route, clientIP, scheme)
		return
	}

	// 6. Streaming HTTP reverse proxy
	g.httpProxy.Forward(w, r, route, clientIP, scheme)
}

func (g *GatewayHandler) serveCompiledRoute(
	w http.ResponseWriter,
	r *http.Request,
	site *proxy.SiteRoute,
	matched *proxy.CompiledRoute,
	origPath, hostName, clientIP, scheme string,
	isWS bool,
) {
	rc := matched.Config
	effPath := config.ApplyStripPrefix(origPath, rc.StripPrefix)

	switch rc.HandlerKind() {
	case "redirect":
		target := expandRedirectPlaceholders(rc.Redirect.To, scheme, hostName, effPath, r.URL.RawQuery)
		proxy.ApplyResponseHeaders(w.Header(), site.Config.Headers)
		proxy.ApplyResponseHeaders(w.Header(), rc.Headers)
		http.Redirect(w, r, target, rc.Redirect.Code)

	case "respond":
		proxy.ApplyResponseHeaders(w.Header(), site.Config.Headers)
		proxy.ApplyResponseHeaders(w.Header(), rc.Headers)
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		}
		w.WriteHeader(rc.Respond.Status)
		if r.Method != http.MethodHead && rc.Respond.Body != "" {
			_, _ = io.WriteString(w, rc.Respond.Body)
		}

	case "static":
		serveStaticRoute(w, r, rc, effPath, site.Config.Headers)

	case "proxy":
		reqPath := effPath
		if rc.StripPrefix == "" && r.URL.RawPath != "" {
			reqPath = r.URL.RawPath
		} else if reqPath != "" && strings.ContainsAny(reqPath, " %?#") {
			// Escape if needed; leave clean paths untouched.
			u := *r.URL
			u.Path = reqPath
			reqPath = u.EscapedPath()
		}
		if isWS {
			g.wsProxy.ForwardUpstream(w, r, site.Config.Domain, matched.NextUpstream(), reqPath, clientIP, scheme)
			return
		}
		g.httpProxy.ForwardUpstream(w, r, site.Config.Domain, matched.NextParsedUpstream(), reqPath, clientIP, scheme, site.Config.Headers, rc.Headers)
	}
}

func expandRedirectPlaceholders(target, scheme, hostName, effPath, rawQuery string) string {
	uri := effPath
	if rawQuery != "" {
		uri += "?" + rawQuery
	}
	repl := strings.NewReplacer(
		"{scheme}", scheme,
		"{host}", hostName,
		"{uri}", uri,
		"{path}", effPath,
		"{query}", rawQuery,
	)
	return repl.Replace(target)
}

func resolveSafePath(absRoot, urlPath string) (string, string, bool) {
	if strings.ContainsRune(urlPath, 0) {
		return "", "", false
	}
	cleaned := pathpkg.Clean("/" + urlPath)
	rel := strings.TrimPrefix(cleaned, "/")
	full := filepath.Join(absRoot, filepath.FromSlash(rel))
	if full != absRoot && !strings.HasPrefix(full, absRoot+string(filepath.Separator)) {
		return "", "", false
	}
	return full, cleaned, true
}

func serveStaticRoute(w http.ResponseWriter, r *http.Request, rc config.RouteConfig, effPath string, siteHeaders map[string]string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	absRoot, err := filepath.Abs(rc.Root)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	full, cleanedURLPath, ok := resolveSafePath(absRoot, effPath)
	if !ok {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	serveFile := func(filePath string) {
		f, openErr := os.Open(filePath)
		if openErr != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		st, stErr := f.Stat()
		if stErr != nil || st.IsDir() {
			http.NotFound(w, r)
			return
		}
		proxy.ApplyResponseHeaders(w.Header(), siteHeaders)
		proxy.ApplyResponseHeaders(w.Header(), rc.Headers)
		http.ServeContent(w, r, st.Name(), st.ModTime(), f)
	}

	serveFallbackOrNotFound := func() {
		if rc.Fallback != "" {
			if fbPath, _, fbOK := resolveSafePath(absRoot, rc.Fallback); fbOK {
				if st, stErr := os.Stat(fbPath); stErr == nil && !st.IsDir() {
					serveFile(fbPath)
					return
				}
			}
		}
		http.NotFound(w, r)
	}

	info, statErr := os.Stat(full)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			serveFallbackOrNotFound()
			return
		}
		http.NotFound(w, r)
		return
	}

	if info.IsDir() {
		indexes := rc.Index
		if len(indexes) == 0 {
			indexes = []string{"index.html"}
		}
		for _, idx := range indexes {
			cand := filepath.Join(full, idx)
			if st, err := os.Stat(cand); err == nil && !st.IsDir() {
				serveFile(cand)
				return
			}
		}
		if rc.Browse {
			proxy.ApplyResponseHeaders(w.Header(), siteHeaders)
			proxy.ApplyResponseHeaders(w.Header(), rc.Headers)
			cloneReq := r.Clone(r.Context())
			u := *r.URL
			u.Path = cleanedURLPath
			if !strings.HasSuffix(u.Path, "/") {
				u.Path += "/"
			}
			cloneReq.URL = &u
			http.FileServer(http.Dir(absRoot)).ServeHTTP(w, cloneReq)
			return
		}
		serveFallbackOrNotFound()
		return
	}

	serveFile(full)
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
