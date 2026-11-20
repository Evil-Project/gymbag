package proxy

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

var copyBufPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 32*1024)
		return &buf
	},
}

// HTTPProxy streams HTTP requests to per-site upstreams and rewrites response headers.
type HTTPProxy struct {
	transport http.RoundTripper
	httpsPort int
	logger    *slog.Logger
}

// NewHTTPProxy constructs an HTTPProxy using transport (or a high-throughput tuned http.Transport if nil).
func NewHTTPProxy(httpsPort int, transport http.RoundTripper, logger *slog.Logger) *HTTPProxy {
	if transport == nil {
		transport = &http.Transport{
			Proxy: nil,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:          4096,
			MaxIdleConnsPerHost:   1024,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			DisableCompression:    true,
			ReadBufferSize:        64 * 1024,
			WriteBufferSize:       64 * 1024,
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
	reqPath := r.URL.RawPath
	if reqPath == "" {
		reqPath = r.URL.Path
		if reqPath != "" && strings.ContainsAny(reqPath, " %?#") {
			reqPath = r.URL.EscapedPath()
		}
	}
	p.ForwardUpstream(w, r, route.Config.Domain, route.NextParsedUpstream(), reqPath, clientIP, scheme, route.Config.Headers)
}

// ForwardUpstream proxies r to a specific ParsedUpstream using reqPath and applies response header mutations.
func (p *HTTPProxy) ForwardUpstream(
	w http.ResponseWriter,
	r *http.Request,
	domain string,
	up ParsedUpstream,
	reqPath, clientIP, scheme string,
	headerSets ...map[string]string,
) {
	targetURL, err := BuildUpstreamURL(up, reqPath, r.URL.RawQuery)
	if err != nil {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}

	method := r.Method
	if method == "" {
		method = http.MethodGet
	}
	var body io.ReadCloser
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions {
		body = r.Body
	}

	upReq := (&http.Request{
		Method:        method,
		URL:           &targetURL,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        ForwardRequestHeaders(r.Header, r.Host, clientIP, scheme),
		Host:          targetURL.Host,
		Body:          body,
		ContentLength: 0,
	}).WithContext(r.Context())
	if body != nil && r.ContentLength != 0 {
		upReq.ContentLength = r.ContentLength
	}

	resp, err := p.transport.RoundTrip(upReq)
	if err != nil {
		if p.logger != nil {
			p.logger.Warn("upstream request failed", "path", r.URL.Path, "site", domain, "error", err)
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "upstream unavailable")
		return
	}
	defer resp.Body.Close()

	copyResponseHeadersParsed(w.Header(), resp.Header, up, r.Host, p.httpsPort)
	for _, hs := range headerSets {
		ApplyResponseHeaders(w.Header(), hs)
	}
	w.WriteHeader(resp.StatusCode)

	if method == http.MethodHead || resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotModified {
		return
	}

	bufPtr := copyBufPool.Get().(*[]byte)
	buf := *bufPtr
	defer copyBufPool.Put(bufPtr)

	// Stream with per-chunk Flush for SSE or unknown-length streaming bodies; otherwise let net/http
	// coalesce headers and body writes for maximum throughput.
	isStream := resp.ContentLength < 0 || strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream")
	if !isStream {
		_, _ = io.CopyBuffer(w, resp.Body, buf)
		return
	}

	rc := http.NewResponseController(w)
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
