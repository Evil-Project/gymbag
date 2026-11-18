package proxy

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// HTTPProxy streams HTTP requests to per-site upstreams and rewrites response headers.
type HTTPProxy struct {
	transport http.RoundTripper
	httpsPort int
	logger    *slog.Logger
}

// NewHTTPProxy constructs an HTTPProxy using transport (or a tuned default http.Transport if nil).
func NewHTTPProxy(httpsPort int, transport http.RoundTripper, logger *slog.Logger) *HTTPProxy {
	if transport == nil {
		transport = &http.Transport{
			Proxy: nil,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}
	}
	return &HTTPProxy{
		transport: transport,
		httpsPort: httpsPort,
		logger:    logger,
	}
}

// Forward proxies r to the selected upstream of route and streams the upstream response back to w.
func (p *HTTPProxy) Forward(w http.ResponseWriter, r *http.Request, route *SiteRoute, clientIP, scheme string) {
	upstreamBase := route.NextUpstream()
	reqPath := r.URL.EscapedPath()
	if reqPath == "" {
		reqPath = r.URL.Path
	}
	targetURL, err := UpstreamURL(upstreamBase, reqPath, r.URL.RawQuery)
	if err != nil {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}

	var body io.Reader
	method := strings.ToUpper(r.Method)
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions {
		body = r.Body
	}

	upReq, err := http.NewRequestWithContext(r.Context(), method, targetURL.String(), body)
	if err != nil {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	upReq.Header = ForwardRequestHeaders(r.Header, r.Host, clientIP, scheme)
	upReq.Host = targetURL.Host
	if body != nil && r.ContentLength >= 0 {
		upReq.ContentLength = r.ContentLength
	}

	resp, err := p.transport.RoundTrip(upReq)
	if err != nil {
		if p.logger != nil {
			p.logger.Warn("upstream request failed", "path", r.URL.Path, "site", route.Config.Domain, "error", err)
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "upstream unavailable")
		return
	}
	defer resp.Body.Close()

	CopyResponseHeaders(w.Header(), resp.Header, upstreamBase, r.Host, p.httpsPort)
	w.WriteHeader(resp.StatusCode)

	rc := http.NewResponseController(w)
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return
			}
			_ = rc.Flush()
		}
		if readErr != nil {
			return
		}
	}
}
