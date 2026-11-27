package server

import (
	"fmt"
	"net/http"
	"strings"

	"gymbag/internal/host"
	"gymbag/internal/proxy"
)

var redirectAllowedMethods = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodPost:    {},
	http.MethodPut:     {},
	http.MethodPatch:   {},
	http.MethodDelete:  {},
	http.MethodHead:    {},
	http.MethodOptions: {},
}

// NewRedirectHandler builds the HTTP (:80) handler that serves ACME HTTP-01 challenges
// and 308-redirects all other traffic to HTTPS (or returns 404 when no HTTP sites are configured).
func NewRedirectHandler(router *proxy.SiteRouter, httpsPort int, board ACMEChallengeLookup) http.Handler {
	redirectCore := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "" {
			path = "/"
		}
		if _, ok := redirectAllowedMethods[strings.ToUpper(r.Method)]; !ok {
			w.Header().Set("Allow", "GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		h := host.FormatForURL(host.FromHeader(r.Host))
		portPart := ""
		if httpsPort != 443 && httpsPort != 0 {
			portPart = fmt.Sprintf(":%d", httpsPort)
		}
		escapedPath := r.URL.EscapedPath()
		if escapedPath == "" {
			escapedPath = path
		}
		queryPart := ""
		if r.URL.RawQuery != "" {
			queryPart = "?" + r.URL.RawQuery
		}
		location := fmt.Sprintf("https://%s%s%s%s", h, portPart, escapedPath, queryPart)

		w.Header().Set("Location", location)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusPermanentRedirect)
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte("Redirecting to HTTPS"))
		}
	})

	siteHandler := AllowedHostMiddleware(router, redirectCore)

	core := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "" {
			path = "/"
		}
		if token, ok := strings.CutPrefix(path, "/.well-known/acme-challenge/"); ok {
			if token != "" && !strings.Contains(token, "/") && board != nil {
				if _, found := board.Lookup(token); found {
					WriteACMEChallenge(w, r, board, token)
					return
				}
			}
		}
		if router == nil || len(router.Sites()) == 0 {
			http.NotFound(w, r)
			return
		}
		siteHandler.ServeHTTP(w, r)
	})

	return ServerHeaderMiddleware("gymbag", core)
}
