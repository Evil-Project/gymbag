package server

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"gymbag/internal/proxy"
)

// ForwardAuthenticator verifies incoming HTTP/WebSocket requests against an external
// forward-authentication service (such as Authelia) before forwarding to upstreams.
type ForwardAuthenticator struct {
	client         *http.Client
	insecureClient *http.Client
	logger         *slog.Logger
}

var authHopByHopHeaders = map[string]struct{}{
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

func noFollowRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

// NewForwardAuthenticator constructs a ForwardAuthenticator using baseClient (or tuned defaults if nil).
func NewForwardAuthenticator(baseClient *http.Client, logger *slog.Logger) *ForwardAuthenticator {
	if logger == nil {
		logger = slog.Default()
	}
	if baseClient != nil {
		std := *baseClient
		if std.CheckRedirect == nil {
			std.CheckRedirect = noFollowRedirect
		}
		insecure := std
		if tr, ok := std.Transport.(*http.Transport); ok && tr != nil {
			trClone := tr.Clone()
			if trClone.TLSClientConfig == nil {
				trClone.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
			} else {
				tlsCfg := trClone.TLSClientConfig.Clone()
				tlsCfg.InsecureSkipVerify = true
				trClone.TLSClientConfig = tlsCfg
			}
			insecure.Transport = trClone
		} else if std.Transport == nil {
			if defTr, ok := http.DefaultTransport.(*http.Transport); ok {
				trClone := defTr.Clone()
				trClone.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
				insecure.Transport = trClone
			}
		}
		return &ForwardAuthenticator{
			client:         &std,
			insecureClient: &insecure,
			logger:         logger,
		}
	}

	baseTransport := &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        512,
		MaxIdleConnsPerHost: 256,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
	}
	insecureTransport := baseTransport.Clone()
	insecureTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	return &ForwardAuthenticator{
		client: &http.Client{
			Transport:     baseTransport,
			CheckRedirect: noFollowRedirect,
		},
		insecureClient: &http.Client{
			Transport:     insecureTransport,
			CheckRedirect: noFollowRedirect,
		},
		logger: logger,
	}
}

// Authenticate evaluates auth for r. When allowed is true, the caller must use the returned
// *http.Request (which has spoofed copy headers stripped and verified identity headers injected).
// When allowed is false, Authenticate has already written the HTTP response (302/401/403/502).
func (fa *ForwardAuthenticator) Authenticate(
	w http.ResponseWriter,
	r *http.Request,
	auth *proxy.CompiledAuth,
	hostName, clientIP, scheme string,
) (*http.Request, bool) {
	if fa == nil || auth == nil || !auth.AppliesTo(r.URL.Path) {
		return r, true
	}

	timeout := time.Duration(auth.Config.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	subReq, err := http.NewRequestWithContext(ctx, http.MethodGet, auth.EndpointURL, nil)
	if err != nil {
		fa.logger.Warn("failed to construct forward-auth request", "url", auth.EndpointURL, "error", err)
		http.Error(w, "authentication service unavailable", http.StatusBadGateway)
		return nil, false
	}

	reqHost := r.Host
	if reqHost == "" {
		reqHost = hostName
	}
	reqURI := r.URL.RequestURI()
	if reqURI == "" {
		reqURI = "/"
	}
	origURL := scheme + "://" + reqHost + reqURI

	stripSet := make(map[string]struct{}, len(auth.StripDst))
	for _, dst := range auth.StripDst {
		stripSet[dst] = struct{}{}
	}

	connOpts := authConnectionOptions(r.Header)
	for k, vals := range r.Header {
		lower := strings.ToLower(k)
		if lower == "host" || lower == "content-length" || lower == "content-type" || lower == "content-encoding" {
			continue
		}
		if _, hop := authHopByHopHeaders[lower]; hop {
			continue
		}
		if _, customHop := connOpts[lower]; customHop {
			continue
		}
		if _, isStrip := stripSet[http.CanonicalHeaderKey(k)]; isStrip {
			continue
		}
		subReq.Header[k] = slices.Clone(vals)
	}

	subReq.Header.Set("X-Forwarded-Method", r.Method)
	subReq.Header.Set("X-Original-Method", r.Method)
	subReq.Header.Set("X-Forwarded-Proto", scheme)
	if reqHost != "" {
		subReq.Header.Set("X-Forwarded-Host", reqHost)
	}
	subReq.Header.Set("X-Forwarded-Uri", reqURI)
	subReq.Header.Set("X-Original-URL", origURL)
	if clientIP != "" {
		subReq.Header.Set("X-Forwarded-For", clientIP)
		subReq.Header.Set("X-Real-IP", clientIP)
	}

	if len(auth.Config.Headers) > 0 {
		repl := strings.NewReplacer(
			"{scheme}", scheme,
			"{host}", reqHost,
			"{uri}", reqURI,
			"{path}", r.URL.Path,
			"{query}", r.URL.RawQuery,
			"{method}", r.Method,
			"{remote_host}", clientIP,
		)
		for rawK, rawV := range auth.Config.Headers {
			k := strings.TrimSpace(rawK)
			if strings.HasPrefix(k, "-") {
				if name := strings.TrimSpace(k[1:]); name != "" {
					subReq.Header.Del(name)
				}
				continue
			}
			if k == "" {
				continue
			}
			val := repl.Replace(rawV)
			if strings.EqualFold(k, "Host") {
				if strings.Contains(val, "{upstream_hostport}") || strings.Contains(val, "{hostport}") || val == "" {
					continue
				}
				subReq.Host = val
				continue
			}
			subReq.Header.Set(k, val)
		}
	}

	client := fa.client
	if auth.Config.InsecureSkipTLS && fa.insecureClient != nil {
		client = fa.insecureClient
	}

	resp, err := client.Do(subReq)
	if err != nil {
		fa.logger.Warn("forward-auth request failed", "url", auth.EndpointURL, "site", hostName, "error", err)
		http.Error(w, "authentication service unavailable", http.StatusBadGateway)
		return nil, false
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
		for _, sc := range resp.Header.Values("Set-Cookie") {
			w.Header().Add("Set-Cookie", sc)
		}

		outReq := *r
		outReq.Header = r.Header.Clone()
		for _, dst := range auth.StripDst {
			outReq.Header.Del(dst)
		}
		for _, pair := range auth.CopyPairs {
			if vals := resp.Header.Values(pair.Src); len(vals) > 0 {
				outReq.Header[pair.Dst] = slices.Clone(vals)
			}
		}
		return &outReq, true
	}

	if resp.StatusCode == http.StatusUnauthorized &&
		resp.Header.Get("Location") == "" &&
		auth.Config.PortalURL != "" &&
		acceptsHTML(r) {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
		for _, sc := range resp.Header.Values("Set-Cookie") {
			w.Header().Add("Set-Cookie", sc)
		}
		http.Redirect(w, r, buildPortalRedirectURL(auth.Config.PortalURL, origURL), http.StatusFound)
		return nil, false
	}

	respConnOpts := authConnectionOptions(resp.Header)
	for k, vals := range resp.Header {
		lower := strings.ToLower(k)
		if lower == "content-length" {
			continue
		}
		if _, hop := authHopByHopHeaders[lower]; hop {
			continue
		}
		if _, customHop := respConnOpts[lower]; customHop {
			continue
		}
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, io.LimitReader(resp.Body, 1<<20))
	}
	return nil, false
}

func authConnectionOptions(h http.Header) map[string]struct{} {
	connVals := h["Connection"]
	if len(connVals) == 0 {
		return nil
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

func acceptsHTML(r *http.Request) bool {
	if (r.Method != http.MethodGet && r.Method != http.MethodHead) || proxy.IsWebSocketUpgrade(r) {
		return false
	}
	lower := strings.ToLower(r.Header.Get("Accept"))
	return strings.Contains(lower, "text/html") || strings.Contains(lower, "application/xhtml+xml")
}

func buildPortalRedirectURL(portalURL, origURL string) string {
	u, err := url.Parse(portalURL)
	if err != nil {
		return portalURL
	}
	q := u.Query()
	if q.Get("rd") == "" {
		q.Set("rd", origURL)
		u.RawQuery = q.Encode()
	}
	return u.String()
}
