package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gymbag/internal/config"
	"gymbag/internal/intel"
	"gymbag/internal/mcserver"
	"gymbag/internal/tlsmanager"
	"gymbag/internal/waf"
)

type mockConsoleController struct {
	mu             sync.Mutex
	hasServer      bool
	consoleEnabled bool
	running        bool
	pid            int
	lines          []mcserver.ConsoleLine
	seq            uint64
	subs           map[uint64]chan mcserver.ConsoleLine
	nextSubID      uint64
	sentCommands   []string
	startRequests  int
}

func newMockConsoleController() *mockConsoleController {
	return &mockConsoleController{
		hasServer:      true,
		consoleEnabled: true,
		running:        true,
		pid:            4242,
		subs:           make(map[uint64]chan mcserver.ConsoleLine),
	}
}

func (m *mockConsoleController) pushLine(stream, line string) mcserver.ConsoleLine {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	ev := mcserver.ConsoleLine{
		Seq:    m.seq,
		Time:   time.Now().UTC(),
		Stream: stream,
		Line:   line,
	}
	m.lines = append(m.lines, ev)
	for _, ch := range m.subs {
		select {
		case ch <- ev:
		default:
		}
	}
	return ev
}

func (m *mockConsoleController) Snapshot() config.Config           { return config.Default() }
func (m *mockConsoleController) ReplaceConfig(config.Config) error { return nil }
func (m *mockConsoleController) TLSSource() string                 { return "none" }
func (m *mockConsoleController) TLSStatus() TLSStatus              { return TLSStatus{} }
func (m *mockConsoleController) RenewTLS(context.Context) (TLSStatus, error) {
	return TLSStatus{}, nil
}
func (m *mockConsoleController) IntelStatus() intel.Status { return intel.Status{} }
func (m *mockConsoleController) RefreshIntel(context.Context) intel.Status {
	return intel.Status{}
}
func (m *mockConsoleController) RemoteRulesStatus() waf.RemoteRulesStatus {
	return waf.RemoteRulesStatus{}
}
func (m *mockConsoleController) RefreshRemoteRules(context.Context) waf.RemoteRulesStatus {
	return waf.RemoteRulesStatus{}
}
func (m *mockConsoleController) RenewTLSStatus() TLSStatus { return TLSStatus{} }
func (m *mockConsoleController) Providers() []tlsmanager.CAProviderInfo {
	return nil
}

func (m *mockConsoleController) MinecraftServerStatus() *mcserver.Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.hasServer {
		return nil
	}
	return &mcserver.Status{
		Enabled:        true,
		Running:        m.running,
		PID:            m.pid,
		ConsoleEnabled: m.consoleEnabled,
		ServerJar:      "/srv/mc/server.jar",
	}
}

func (m *mockConsoleController) MinecraftConsoleEnabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.consoleEnabled
}

func (m *mockConsoleController) MinecraftConsoleLines(sinceSeq uint64, limit int) []mcserver.ConsoleLine {
	m.mu.Lock()
	defer m.mu.Unlock()
	var matched []mcserver.ConsoleLine
	for _, ev := range m.lines {
		if ev.Seq > sinceSeq {
			matched = append(matched, ev)
		}
	}
	if limit > 0 && len(matched) > limit {
		matched = matched[len(matched)-limit:]
	}
	out := make([]mcserver.ConsoleLine, len(matched))
	copy(out, matched)
	return out
}

func (m *mockConsoleController) MinecraftConsoleSubscribe() ([]mcserver.ConsoleLine, <-chan mcserver.ConsoleLine, func()) {
	m.mu.Lock()
	backlog := make([]mcserver.ConsoleLine, len(m.lines))
	copy(backlog, m.lines)
	m.nextSubID++
	id := m.nextSubID
	ch := make(chan mcserver.ConsoleLine, 32)
	m.subs[id] = ch
	m.mu.Unlock()

	var once sync.Once
	return backlog, ch, func() {
		once.Do(func() {
			m.mu.Lock()
			if existing, ok := m.subs[id]; ok {
				delete(m.subs, id)
				close(existing)
			}
			m.mu.Unlock()
		})
	}
}

func (m *mockConsoleController) MinecraftSendCommand(cmd string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return mcserver.ErrNotRunning
	}
	m.sentCommands = append(m.sentCommands, cmd)
	if strings.EqualFold(cmd, "stop") || strings.EqualFold(cmd, "/stop") {
		m.running = false
		m.pid = 0
	}
	return nil
}

func (m *mockConsoleController) MinecraftStartServer() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return errors.New("already running")
	}
	m.running = true
	m.pid = 5252
	m.startRequests++
	return nil
}

type ssePipeResponseWriter struct {
	hdr     http.Header
	pw      *io.PipeWriter
	wrote   bool
	code    int
	hdrOnce sync.Once
	hdrDone chan struct{}
}

func newSSEPipeResponseWriter(pw *io.PipeWriter) *ssePipeResponseWriter {
	return &ssePipeResponseWriter{
		hdr:     make(http.Header),
		pw:      pw,
		hdrDone: make(chan struct{}),
	}
}

func (w *ssePipeResponseWriter) Header() http.Header { return w.hdr }

func (w *ssePipeResponseWriter) WriteHeader(code int) {
	w.hdrOnce.Do(func() {
		w.wrote = true
		w.code = code
		close(w.hdrDone)
	})
}

func (w *ssePipeResponseWriter) Write(p []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.pw.Write(p)
}

func (w *ssePipeResponseWriter) Flush() {}

func TestAdminMinecraftConsoleAPI(t *testing.T) {
	ctrl := newMockConsoleController()
	ctrl.pushLine("stdout", "[INFO]: Starting minecraft server")
	ctrl.pushLine("stdout", "[INFO]: Done (1.0s)!")
	ctrl.pushLine("stderr", "[WARN]: Example warning")

	handler := NewAdminHandler(ctrl)

	// 1. GET JSON snapshot with since & limit
	req := httptest.NewRequest(http.MethodGet, "/api/v1/minecraft/console?since=1&limit=1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET console failed (%d): %s", rec.Code, rec.Body.String())
	}
	var snap struct {
		Running        bool                   `json:"running"`
		PID            int                    `json:"pid"`
		ConsoleEnabled bool                   `json:"console_enabled"`
		Lines          []mcserver.ConsoleLine `json:"lines"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if !snap.Running || snap.PID != 4242 || !snap.ConsoleEnabled || len(snap.Lines) != 1 || snap.Lines[0].Seq != 3 {
		t.Fatalf("unexpected console snapshot: %+v", snap)
	}

	// 2. GET SSE live stream: backlog + live pushed line + clean cancellation
	pr, pw := io.Pipe()
	defer pr.Close()
	sseW := newSSEPipeResponseWriter(pw)
	sseCtx, cancelSSE := context.WithCancel(context.Background())
	defer cancelSSE()
	sseReq := httptest.NewRequest(http.MethodGet, "/api/v1/minecraft/console?stream=1&since=2", nil).WithContext(sseCtx)

	sseDone := make(chan struct{})
	go func() {
		defer close(sseDone)
		defer pw.Close()
		handler.ServeHTTP(sseW, sseReq)
	}()

	<-sseW.hdrDone
	if sseW.code != http.StatusOK || !strings.Contains(sseW.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("unexpected SSE headers/status: code=%d hdr=%+v", sseW.code, sseW.Header())
	}

	sc := bufio.NewScanner(pr)
	var received []string
	// Read backlog event (seq 3)
	for sc.Scan() {
		line := sc.Text()
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			received = append(received, data)
			break
		}
	}
	// Push live event (seq 4)
	ctrl.pushLine("stdout", "[INFO]: Player Alice joined")
	for sc.Scan() {
		line := sc.Text()
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			received = append(received, data)
			break
		}
	}
	cancelSSE()
	<-sseDone
	if len(received) != 2 || !strings.Contains(received[0], "Example warning") || !strings.Contains(received[1], "Player Alice joined") {
		t.Fatalf("unexpected SSE events: %v", received)
	}

	// 3. POST command succeeds when running
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/minecraft/console", bytes.NewBufferString(`{"command":"list"}`))
	postReq.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()
	handler.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusOK || !strings.Contains(postRec.Body.String(), `"ok":true`) {
		t.Fatalf("POST command failed (%d): %s", postRec.Code, postRec.Body.String())
	}

	// 4. POST "stop" marks server stopped, next command returns 409 not_running, then POST "start" starts it
	stopReq := httptest.NewRequest(http.MethodPost, "/api/v1/minecraft/console", bytes.NewBufferString(`{"command":"stop"}`))
	stopReq.Header.Set("Content-Type", "application/json")
	stopRec := httptest.NewRecorder()
	handler.ServeHTTP(stopRec, stopReq)
	if stopRec.Code != http.StatusOK {
		t.Fatalf("POST stop failed (%d): %s", stopRec.Code, stopRec.Body.String())
	}

	stoppedCmdReq := httptest.NewRequest(http.MethodPost, "/api/v1/minecraft/console", bytes.NewBufferString(`{"command":"say hi"}`))
	stoppedCmdReq.Header.Set("Content-Type", "application/json")
	stoppedCmdRec := httptest.NewRecorder()
	handler.ServeHTTP(stoppedCmdRec, stoppedCmdReq)
	if stoppedCmdRec.Code != http.StatusConflict || !strings.Contains(stoppedCmdRec.Body.String(), "not_running") {
		t.Fatalf("expected 409 not_running while stopped, got (%d): %s", stoppedCmdRec.Code, stoppedCmdRec.Body.String())
	}

	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/minecraft/console", bytes.NewBufferString(`{"command":"start"}`))
	startReq.Header.Set("Content-Type", "application/json")
	startRec := httptest.NewRecorder()
	handler.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK || !strings.Contains(startRec.Body.String(), `"started"`) {
		t.Fatalf("POST start failed (%d): %s", startRec.Code, startRec.Body.String())
	}
	if !ctrl.MinecraftServerStatus().Running {
		t.Fatalf("expected mock server running after POST start")
	}

	// 5. 403 console_disabled when console is off
	ctrl.mu.Lock()
	ctrl.consoleEnabled = false
	ctrl.mu.Unlock()
	disReq := httptest.NewRequest(http.MethodGet, "/api/v1/minecraft/console", nil)
	disRec := httptest.NewRecorder()
	handler.ServeHTTP(disRec, disReq)
	if disRec.Code != http.StatusForbidden || !strings.Contains(disRec.Body.String(), "console_disabled") {
		t.Fatalf("expected 403 console_disabled, got (%d): %s", disRec.Code, disRec.Body.String())
	}

	// 6. 404 not_found when no managed server is configured
	ctrl.mu.Lock()
	ctrl.hasServer = false
	ctrl.mu.Unlock()
	missingReq := httptest.NewRequest(http.MethodGet, "/api/v1/minecraft/console", nil)
	missingRec := httptest.NewRecorder()
	handler.ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when no managed MC server, got (%d): %s", missingRec.Code, missingRec.Body.String())
	}
}
