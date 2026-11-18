package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// DialContextFunc dials an upstream TCP stream (injectable for sandboxed tests).
type DialContextFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// WebSocketProxy tunnels HTTP/1.1 WebSocket upgrades bidirectionally to site upstreams.
type WebSocketProxy struct {
	dial   DialContextFunc
	logger *slog.Logger
}

// NewWebSocketProxy constructs a WebSocketProxy with an optional custom dialer.
func NewWebSocketProxy(dial DialContextFunc, logger *slog.Logger) *WebSocketProxy {
	if dial == nil {
		dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
		dial = dialer.DialContext
	}
	return &WebSocketProxy{
		dial:   dial,
		logger: logger,
	}
}

// Forward upgrades and splices a client WebSocket connection to route's upstream.
func (ws *WebSocketProxy) Forward(w http.ResponseWriter, r *http.Request, route *SiteRoute, clientIP, scheme string) {
	upstreamBase := route.NextUpstream()
	reqPath := r.URL.EscapedPath()
	if reqPath == "" {
		reqPath = r.URL.Path
	}
	wsURL, err := WebSocketUpstreamURL(upstreamBase, reqPath, r.URL.RawQuery)
	if err != nil {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}

	dialAddr := wsURL.Host
	if wsURL.Port() == "" {
		if wsURL.Scheme == "wss" {
			dialAddr = net.JoinHostPort(wsURL.Hostname(), "443")
		} else {
			dialAddr = net.JoinHostPort(wsURL.Hostname(), "80")
		}
	}

	dialCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	upConn, err := ws.dial(dialCtx, "tcp", dialAddr)
	if err != nil {
		if ws.logger != nil {
			ws.logger.Warn("websocket upstream unavailable", "path", r.URL.Path, "site", route.Config.Domain, "error", err)
		}
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	if wsURL.Scheme == "wss" {
		tlsConn := tls.Client(upConn, &tls.Config{
			ServerName: wsURL.Hostname(),
			MinVersion: tls.VersionTLS12,
		})
		if hsErr := tlsConn.HandshakeContext(dialCtx); hsErr != nil {
			_ = upConn.Close()
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
			return
		}
		upConn = tlsConn
	}
	defer upConn.Close()

	// Construct HTTP/1.1 Upgrade request for upstream
	targetURI := wsURL.EscapedPath()
	if targetURI == "" {
		targetURI = "/"
	}
	if wsURL.RawQuery != "" {
		targetURI += "?" + wsURL.RawQuery
	}

	fwdHeaders := WebSocketForwardHeaders(r.Header, r.Host, clientIP, scheme)
	upReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "http://"+wsURL.Host+targetURI, nil)
	if err != nil {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	upReq.Header = fwdHeaders
	upReq.Host = wsURL.Host

	if err := upReq.Write(upConn); err != nil {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}

	upBuf := bufio.NewReader(upConn)
	upResp, err := http.ReadResponse(upBuf, upReq)
	if err != nil {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	if upResp.StatusCode != http.StatusSwitchingProtocols {
		defer upResp.Body.Close()
		for k, vals := range upResp.Header {
			for _, v := range vals {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(upResp.StatusCode)
		_, _ = io.Copy(w, upResp.Body)
		return
	}

	rc := http.NewResponseController(w)
	clientConn, clientBuf, err := rc.Hijack()
	if err != nil {
		http.Error(w, "websocket hijack unsupported", http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	// Write upstream 101 Switching Protocols response back to client
	var respHead string
	respHead = fmt.Sprintf("HTTP/1.1 101 Switching Protocols\r\n")
	if _, err := io.WriteString(clientConn, respHead); err != nil {
		return
	}
	if err := upResp.Header.Write(clientConn); err != nil {
		return
	}
	if _, err := io.WriteString(clientConn, "\r\n"); err != nil {
		return
	}

	// Flush any already-buffered bytes from both sides before bidirectional splice
	if upBuf.Buffered() > 0 {
		buffered := make([]byte, upBuf.Buffered())
		_, _ = io.ReadFull(upBuf, buffered)
		if _, err := clientConn.Write(buffered); err != nil {
			return
		}
	}
	if clientBuf != nil && clientBuf.Reader.Buffered() > 0 {
		buffered := make([]byte, clientBuf.Reader.Buffered())
		_, _ = io.ReadFull(clientBuf.Reader, buffered)
		if _, err := upConn.Write(buffered); err != nil {
			return
		}
	}

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(upConn, clientConn)
		_ = upConn.Close()
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(clientConn, upConn)
		_ = clientConn.Close()
		done <- struct{}{}
	}()
	<-done
}
