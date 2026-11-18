package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gymbag/internal/config"
	"gymbag/internal/store"
	"gymbag/internal/waf"
)

func TestHeadersURLsAndLocationRewrite(t *testing.T) {
	in := http.Header{}
	in.Set("Host", "app.example")
	in.Set("Connection", "close, x-secret")
	in.Set("X-Secret", "no")
	in.Set("X-Request", "yes")
	in.Set("Content-Length", "4")

	fwd := ForwardRequestHeaders(in, "app.example", "203.0.113.10", "https")
	if fwd.Get("Connection") != "" || fwd.Get("X-Secret") != "" || fwd.Get("Host") != "" || fwd.Get("Content-Length") != "" {
		t.Fatalf("expected hop-by-hop and host headers dropped, got %+v", fwd)
	}
	if fwd.Get("X-Request") != "yes" ||
		fwd.Get("X-Forwarded-For") != "203.0.113.10" ||
		fwd.Get("X-Forwarded-Proto") != "https" ||
		fwd.Get("X-Forwarded-Host") != "app.example" {
		t.Fatalf("unexpected forwarding headers: %+v", fwd)
	}

	u, err := UpstreamURL("http://127.0.0.1:5173/app", "/assets/main.js", "v=1")
	if err != nil || u.String() != "http://127.0.0.1:5173/app/assets/main.js?v=1" {
		t.Fatalf("UpstreamURL = %v, %v", u, err)
	}

	rewritten := RewriteLocation("http://127.0.0.1:5173/moved?ok=1", "http://127.0.0.1:5173", "https://app.example")
	if rewritten != "https://app.example/moved?ok=1" {
		t.Fatalf("RewriteLocation = %q", rewritten)
	}
	untouched := RewriteLocation("https://other.example/stay", "http://127.0.0.1:5173", "https://app.example")
	if untouched != "https://other.example/stay" {
		t.Fatalf("RewriteLocation external = %q", untouched)
	}

	ws1, err := WebSocketUpstreamURL("http://127.0.0.1:5173", "/hmr", "token=1")
	if err != nil || ws1.String() != "ws://127.0.0.1:5173/hmr?token=1" {
		t.Fatalf("ws1 = %v, %v", ws1, err)
	}
	ws2, err := WebSocketUpstreamURL("https://upstream.internal", "/hmr", "")
	if err != nil || ws2.String() != "wss://upstream.internal/hmr" {
		t.Fatalf("ws2 = %v, %v", ws2, err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHTTPProxyStreamingLocationAndOutage(t *testing.T) {
	st := store.NewMemoryStore()
	tokens := waf.NewClearanceTokens([]byte("secret"), 60)
	router := NewSiteRouter([]config.SiteConfig{
		{
			Domain:    "app.example",
			Upstreams: []string{"http://localhost:5173"},
			Protect:   true,
			WAF:       config.DefaultSiteWAFConfig(),
		},
	}, st, tokens)
	route, ok := router.Resolve("app.example")
	if !ok {
		t.Fatal("expected app.example route")
	}

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		bodyBytes, _ := io.ReadAll(req.Body)
		h := http.Header{}
		h.Set("X-Echo-Method", req.Method)
		h.Set("X-Echo-Proto", req.Header.Get("X-Forwarded-Proto"))
		h.Set("X-Echo-Secret", req.Header.Get("X-Secret"))
		h.Set("Location", "http://localhost:5173/moved")
		h.Set("Connection", "close")
		h.Add("Set-Cookie", "a=1")
		h.Add("Set-Cookie", "b=2")
		return &http.Response{
			StatusCode: http.StatusCreated,
			Header:     h,
			Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
		}, nil
	})

	hp := NewHTTPProxy(443, transport, nil)
	req := httptest.NewRequest(http.MethodPost, "http://app.example/", strings.NewReader("abc"))
	req.Header.Set("Connection", "close, x-secret")
	req.Header.Set("X-Secret", "no")
	rec := httptest.NewRecorder()

	hp.Forward(rec, req, route, "203.0.113.9", "https")
	res := rec.Result()
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)

	if res.StatusCode != http.StatusCreated || string(body) != "abc" {
		t.Fatalf("status=%d body=%q", res.StatusCode, string(body))
	}
	if res.Header.Get("Location") != "https://app.example/moved" {
		t.Fatalf("Location = %q", res.Header.Get("Location"))
	}
	if res.Header.Get("X-Echo-Secret") != "" || res.Header.Get("Connection") != "" {
		t.Fatalf("expected hop-by-hop stripped, got %+v", res.Header)
	}
	if len(res.Header.Values("Set-Cookie")) != 2 {
		t.Fatalf("expected both Set-Cookie values preserved, got %v", res.Header.Values("Set-Cookie"))
	}

	// Outage returns 502 without leaking internal error details
	failProxy := NewHTTPProxy(443, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial tcp 10.0.0.9:5173: connection refused")
	}), nil)
	rec502 := httptest.NewRecorder()
	failProxy.Forward(rec502, httptest.NewRequest(http.MethodGet, "http://app.example/health", nil), route, "203.0.113.9", "https")
	if rec502.Code != http.StatusBadGateway || strings.Contains(rec502.Body.String(), "10.0.0.9") {
		t.Fatalf("expected clean 502, got %d %q", rec502.Code, rec502.Body.String())
	}
}

// pipeListener implements net.Listener completely in memory via net.Pipe (sandbox-safe).
type pipeListener struct {
	ch     chan net.Conn
	closed chan struct{}
}

func newPipeListener() *pipeListener {
	return &pipeListener{
		ch:     make(chan net.Conn, 16),
		closed: make(chan struct{}),
	}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.ch:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	select {
	case <-l.closed:
	default:
		close(l.closed)
	}
	return nil
}

func (l *pipeListener) Addr() net.Addr { return pipeAddr("pipe") }

func (l *pipeListener) DialContext(_ context.Context, _, _ string) (net.Conn, error) {
	c1, c2 := net.Pipe()
	select {
	case l.ch <- c2:
		return c1, nil
	case <-l.closed:
		_ = c1.Close()
		_ = c2.Close()
		return nil, net.ErrClosed
	}
}

type pipeAddr string

func (a pipeAddr) Network() string { return string(a) }
func (a pipeAddr) String() string  { return string(a) }

func TestWebSocketProxyTunnelOverPipe(t *testing.T) {
	// Upstream RFC 6455 echo server over in-memory pipeListener
	upLn := newPipeListener()
	defer upLn.Close()

	const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	upSrv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !IsWebSocketUpgrade(r) {
				http.Error(w, "expected ws", http.StatusBadRequest)
				return
			}
			key := r.Header.Get("Sec-WebSocket-Key")
			sum := sha1.Sum([]byte(key + wsGUID))
			accept := base64.StdEncoding.EncodeToString(sum[:])

			rc := http.NewResponseController(w)
			conn, _, err := rc.Hijack()
			if err != nil {
				return
			}
			defer conn.Close()
			_, _ = io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: "+accept+"\r\n\r\n")
			_, _ = io.Copy(conn, conn) // echo raw stream
		}),
	}
	go func() { _ = upSrv.Serve(upLn) }()
	defer upSrv.Close()

	// Downstream proxy server over a second in-memory pipeListener
	downLn := newPipeListener()
	defer downLn.Close()

	st := store.NewMemoryStore()
	tokens := waf.NewClearanceTokens([]byte("secret"), 60)
	router := NewSiteRouter([]config.SiteConfig{
		{
			Domain:    "ws.example",
			Upstreams: []string{"http://127.0.0.1:9000"},
			Protect:   false,
			WAF:       config.DefaultSiteWAFConfig(),
		},
	}, st, tokens)
	route, _ := router.Resolve("ws.example")
	wsProxy := NewWebSocketProxy(upLn.DialContext, nil)

	downSrv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			wsProxy.Forward(w, r, route, "203.0.113.7", "https")
		}),
	}
	go func() { _ = downSrv.Serve(downLn) }()
	defer downSrv.Close()

	clientConn, err := downLn.DialContext(context.Background(), "tcp", "ws.example:443")
	if err != nil {
		t.Fatal(err)
	}
	defer clientConn.Close()

	handshake := "GET /chat?room=1 HTTP/1.1\r\nHost: ws.example\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n"
	if _, err := io.WriteString(clientConn, handshake); err != nil {
		t.Fatal(err)
	}
	rd := bufio.NewReader(clientConn)
	resp, err := http.ReadResponse(rd, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101 Switching Protocols, got %d", resp.StatusCode)
	}
	if _, err := io.WriteString(clientConn, "hello-gymbag-ws\n"); err != nil {
		t.Fatal(err)
	}
	echoed, err := rd.ReadString('\n')
	if err != nil || echoed != "hello-gymbag-ws\n" {
		t.Fatalf("echoed = %q, err = %v", echoed, err)
	}
}
