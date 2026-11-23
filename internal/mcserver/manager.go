package mcserver

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gymbag/internal/config"
)

const (
	defaultRestartBaseDelay  = 3 * time.Second
	maxRestartDelay          = 30 * time.Second
	stableRunReset           = 60 * time.Second
	defaultStopTimeout       = 15 * time.Second
	defaultConsoleBufferSize = 500
	consoleSubscriberBufSize = 128
)

var (
	// ErrNotRunning is returned when a console command is sent while the Minecraft server process is stopped.
	ErrNotRunning = errors.New("minecraft server is not running")
	// ErrAlreadyRunning is returned when RequestStart is called while the Minecraft server process is already running.
	ErrAlreadyRunning = errors.New("minecraft server is already running")
)

// ConsoleLine represents a single line emitted by the managed Minecraft server or console supervisor.
type ConsoleLine struct {
	Seq    uint64    `json:"seq"`
	Time   time.Time `json:"time"`
	Stream string    `json:"stream"`
	Line   string    `json:"line"`
}

// Options configures managed Minecraft server execution inside Gymbag.
type Options struct {
	BaseDir           string
	Config            config.MinecraftConfig
	Discover          DiscoverOptions
	CommandBuilder    func(ctx context.Context, javaPath string, args []string) *exec.Cmd
	Logger            *slog.Logger
	RestartBaseDelay  time.Duration
	ConsoleIn         io.Reader
	ConsoleOut        io.Writer
	ConsoleErr        io.Writer
	ConsoleBufferSize int
}

// Status describes the live runtime state of the managed Minecraft server subprocess.
type Status struct {
	Enabled        bool   `json:"enabled"`
	Running        bool   `json:"running"`
	PID            int    `json:"pid,omitempty"`
	ConsoleEnabled bool   `json:"console_enabled,omitempty"`
	ServerJar      string `json:"server_jar"`
	WorkDir        string `json:"work_dir,omitempty"`
	JavaPath       string `json:"java_path,omitempty"`
	JavaVersion    string `json:"java_version,omitempty"`
	JavaMajor      int    `json:"java_major,omitempty"`
	RequiredJava   int    `json:"required_java,omitempty"`
	MCVersion      string `json:"mc_version,omitempty"`
	Restarts       int    `json:"restarts,omitempty"`
	EULABlocked    bool   `json:"eula_blocked,omitempty"`
	LastError      string `json:"last_error,omitempty"`
}

// Manager supervises an external Minecraft server .jar child process alongside Gymbag's proxy.
type Manager struct {
	opts      Options
	cfg       config.MinecraftConfig
	jarPath   string
	workDir   string
	propsPath string
	logger    *slog.Logger
	baseDelay time.Duration

	mu                  sync.Mutex
	started             bool
	cancel              context.CancelFunc
	runCtx              context.Context
	done                chan struct{}
	stdin               io.WriteCloser
	proc                *os.Process
	status              Status
	consoleEnabled      bool
	consoleInputStarted bool
	manualStop          bool
	restartCh           chan struct{}

	stdinMu      sync.Mutex
	consoleOutMu sync.Mutex

	consoleMu   sync.Mutex
	consoleBuf  []ConsoleLine
	consoleMax  int
	consoleSeq  uint64
	consoleSubs map[uint64]chan ConsoleLine
	nextSubID   uint64
}

// NewManager constructs a managed Minecraft server supervisor from opts.
func NewManager(opts Options) *Manager {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	cfg := opts.Config
	jarPath := resolveRelativePath(opts.BaseDir, cfg.ServerJar)
	workDir := strings.TrimSpace(cfg.WorkDir)
	if workDir != "" {
		workDir = resolveRelativePath(opts.BaseDir, workDir)
	} else if jarPath != "" {
		workDir = filepath.Dir(jarPath)
	}

	propsPath := strings.TrimSpace(cfg.ServerProperties)
	if propsPath != "" {
		propsPath = resolveRelativePath(opts.BaseDir, propsPath)
	} else if workDir != "" {
		propsPath = filepath.Join(workDir, "server.properties")
	}

	baseDelay := opts.RestartBaseDelay
	if baseDelay <= 0 {
		baseDelay = defaultRestartBaseDelay
	}
	bufMax := opts.ConsoleBufferSize
	if bufMax <= 0 {
		bufMax = defaultConsoleBufferSize
	}

	return &Manager{
		opts:           opts,
		cfg:            cfg,
		jarPath:        jarPath,
		workDir:        workDir,
		propsPath:      propsPath,
		logger:         logger,
		baseDelay:      baseDelay,
		consoleEnabled: cfg.Console,
		restartCh:      make(chan struct{}, 1),
		consoleMax:     bufMax,
		consoleSubs:    make(map[uint64]chan ConsoleLine),
		status: Status{
			Enabled:        jarPath != "",
			ConsoleEnabled: cfg.Console,
			ServerJar:      jarPath,
			WorkDir:        workDir,
		},
	}
}

func resolveRelativePath(baseDir, raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if filepath.IsAbs(trimmed) || strings.TrimSpace(baseDir) == "" {
		return filepath.Clean(trimmed)
	}
	return filepath.Clean(filepath.Join(baseDir, trimmed))
}

// Start resolves a compatible Java runtime for the configured server JAR, ensures server.properties
// binds the protected backend port if absent, and launches the background supervision loop.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.jarPath == "" {
		m.mu.Unlock()
		return nil
	}
	if m.started {
		m.mu.Unlock()
		return nil
	}
	jarPath := m.jarPath
	explicitJava := m.cfg.JavaPath
	discoverOpts := m.opts.Discover
	m.mu.Unlock()

	sel, err := ResolveJavaForJar(ctx, jarPath, explicitJava, discoverOpts)
	if err != nil {
		m.mu.Lock()
		m.status.LastError = err.Error()
		m.mu.Unlock()
		return err
	}
	if sel.Warning != "" {
		m.logger.Warn("minecraft java compatibility warning", "warning", sel.Warning)
	}
	m.logger.Info(
		"selected Java runtime for Minecraft server",
		"server_jar", jarPath,
		"java_path", sel.Java.Path,
		"java_major", sel.Java.Major,
		"java_version", sel.Java.RawVersion,
		"required_java", sel.Requirement.MinMajor,
		"detected_from", sel.Requirement.Source,
		"mc_version", sel.Requirement.MCVersion,
	)

	if err := m.ensureWorkDirAndProperties(); err != nil {
		m.mu.Lock()
		m.status.LastError = err.Error()
		m.mu.Unlock()
		return err
	}

	absJar, err := filepath.Abs(jarPath)
	if err != nil {
		absJar = jarPath
	}

	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	m.started = true
	m.cancel = cancel
	m.runCtx = runCtx
	m.done = done
	m.status.Enabled = true
	m.status.ServerJar = jarPath
	m.status.WorkDir = m.workDir
	m.status.JavaPath = sel.Java.Path
	m.status.JavaVersion = sel.Java.RawVersion
	m.status.JavaMajor = sel.Java.Major
	m.status.RequiredJava = sel.Requirement.MinMajor
	m.status.MCVersion = sel.Requirement.MCVersion
	m.status.LastError = ""
	startConsoleInput := m.consoleEnabled && !m.consoleInputStarted && m.opts.ConsoleIn != nil
	if startConsoleInput {
		m.consoleInputStarted = true
	}
	m.mu.Unlock()

	if startConsoleInput {
		go m.readConsoleInput(runCtx, m.opts.ConsoleIn)
	}
	go m.supervise(runCtx, sel.Java.Path, absJar, done)
	return nil
}

func (m *Manager) ensureWorkDirAndProperties() error {
	if m.workDir != "" {
		if err := os.MkdirAll(m.workDir, 0o755); err != nil {
			return fmt.Errorf("failed to create minecraft work_dir %s: %w", m.workDir, err)
		}
	}
	if m.propsPath == "" || m.cfg.Upstream == "" {
		return nil
	}
	if _, err := os.Stat(m.propsPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to stat %s: %w", m.propsPath, err)
	}

	host, port, err := net.SplitHostPort(m.cfg.Upstream)
	if err != nil {
		return nil
	}
	if dir := filepath.Dir(m.propsPath); dir != "" && dir != "." {
		if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
			return fmt.Errorf("failed to create server.properties directory %s: %w", dir, mkErr)
		}
	}
	content := fmt.Sprintf("# Minecraft server properties (initialized by gymbag)\nserver-ip=%s\nserver-port=%s\n", host, port)
	if err := os.WriteFile(m.propsPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write initial %s: %w", m.propsPath, err)
	}
	m.logger.Info("initialized missing server.properties for managed Minecraft server", "path", m.propsPath, "upstream", m.cfg.Upstream)
	return nil
}

func (m *Manager) supervise(ctx context.Context, javaPath, absJar string, done chan struct{}) {
	defer close(done)

	backoff := m.baseDelay
	args := m.buildArgs(absJar)

	for {
		if ctx.Err() != nil {
			return
		}

		m.mu.Lock()
		m.manualStop = false
		m.mu.Unlock()

		startAt := time.Now()
		exitErr, eulaBlocked := m.runOnce(ctx, javaPath, args)
		ranFor := time.Since(startAt)

		m.mu.Lock()
		m.status.Running = false
		m.status.PID = 0
		m.stdin = nil
		m.proc = nil
		wasManualStop := m.manualStop
		m.manualStop = false
		if eulaBlocked {
			m.status.EULABlocked = true
		}
		if exitErr != nil && ctx.Err() == nil && !wasManualStop {
			m.status.LastError = exitErr.Error()
		}
		m.mu.Unlock()

		if eulaBlocked {
			m.logger.Warn("Minecraft server exited because eula.txt has not been accepted; set eula=true in work_dir/eula.txt and restart Gymbag", "work_dir", m.workDir)
			return
		}
		if ctx.Err() != nil {
			return
		}
		if wasManualStop {
			m.recordConsoleLine("system", "[gymbag] Minecraft server stopped cleanly. Press Ctrl+C to exit Gymbag or type 'start' to relaunch.")
			select {
			case <-ctx.Done():
				return
			case <-m.restartCh:
				backoff = m.baseDelay
				continue
			}
		}
		if !m.cfg.AutoRestartEnabled() {
			if exitErr != nil {
				m.logger.Warn("managed Minecraft server exited", "error", exitErr)
			} else {
				m.logger.Info("managed Minecraft server stopped cleanly")
			}
			return
		}

		if ranFor >= stableRunReset {
			backoff = m.baseDelay
		}
		m.mu.Lock()
		m.status.Restarts++
		restarts := m.status.Restarts
		m.mu.Unlock()

		m.logger.Warn("managed Minecraft server exited unexpectedly; restarting",
			"error", exitErr,
			"restart_count", restarts,
			"backoff", backoff,
		)

		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		backoff *= 2
		if backoff > maxRestartDelay {
			backoff = maxRestartDelay
		}
	}
}

func (m *Manager) buildArgs(absJar string) []string {
	args := make([]string, 0, len(m.cfg.JVMArgs)+2+max(1, len(m.cfg.ServerArgs)))
	args = append(args, m.cfg.JVMArgs...)
	args = append(args, "-jar", absJar)
	if len(m.cfg.ServerArgs) > 0 {
		args = append(args, m.cfg.ServerArgs...)
	} else {
		args = append(args, "nogui")
	}
	return args
}

func (m *Manager) runOnce(ctx context.Context, javaPath string, args []string) (exitErr error, eulaBlocked bool) {
	var cmd *exec.Cmd
	if m.opts.CommandBuilder != nil {
		cmd = m.opts.CommandBuilder(context.Background(), javaPath, args)
	} else {
		cmd = exec.Command(javaPath, args...)
	}
	if cmd.Dir == "" && m.workDir != "" {
		cmd.Dir = m.workDir
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("minecraft stdin pipe failed: %w", err), false
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdinPipe.Close()
		return fmt.Errorf("minecraft stdout pipe failed: %w", err), false
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		_ = stdinPipe.Close()
		return fmt.Errorf("minecraft stderr pipe failed: %w", err), false
	}

	if err := cmd.Start(); err != nil {
		_ = stdinPipe.Close()
		return fmt.Errorf("failed to start minecraft server: %w", err), false
	}

	m.mu.Lock()
	m.stdin = stdinPipe
	m.proc = cmd.Process
	m.status.Running = true
	if cmd.Process != nil {
		m.status.PID = cmd.Process.Pid
	}
	m.mu.Unlock()

	m.logger.Info("started managed Minecraft server", "pid", cmd.Process.Pid, "work_dir", m.workDir)

	var (
		eulaMu   sync.Mutex
		sawEULA  bool
		pipesWG  sync.WaitGroup
		waitDone = make(chan error, 1)
	)
	scanPipe := func(stream string, r io.Reader) {
		defer pipesWG.Done()
		sc := bufio.NewScanner(r)
		buf := make([]byte, 0, 64*1024)
		sc.Buffer(buf, 512*1024)
		for sc.Scan() {
			line := sc.Text()
			m.recordConsoleLine(stream, line)
			lower := strings.ToLower(line)
			if strings.Contains(lower, "agree to the eula") {
				eulaMu.Lock()
				sawEULA = true
				eulaMu.Unlock()
			}
		}
	}

	pipesWG.Add(2)
	go scanPipe("stdout", stdoutPipe)
	go scanPipe("stderr", stderrPipe)

	go func() {
		pipesWG.Wait()
		waitDone <- cmd.Wait()
	}()

	var waitErr error
	select {
	case waitErr = <-waitDone:
	case <-ctx.Done():
		waitErr = m.gracefulKillProcess(stdinPipe, cmd.Process, waitDone)
	}

	m.mu.Lock()
	m.stdin = nil
	m.proc = nil
	m.status.Running = false
	m.status.PID = 0
	m.mu.Unlock()

	m.stdinMu.Lock()
	_ = stdinPipe.Close()
	m.stdinMu.Unlock()
	eulaMu.Lock()
	eulaFlag := sawEULA
	eulaMu.Unlock()
	return waitErr, eulaFlag
}

func (m *Manager) gracefulKillProcess(stdin io.WriteCloser, proc *os.Process, waitDone <-chan error) error {
	if stdin != nil {
		m.stdinMu.Lock()
		_, _ = io.WriteString(stdin, "stop\n")
		m.stdinMu.Unlock()
	}
	softWait := defaultStopTimeout * 3 / 4
	select {
	case err := <-waitDone:
		return err
	case <-time.After(softWait):
	}
	if proc != nil {
		_ = proc.Signal(os.Interrupt)
	}
	select {
	case err := <-waitDone:
		return err
	case <-time.After(defaultStopTimeout - softWait):
	}
	if proc != nil {
		_ = proc.Kill()
	}
	return <-waitDone
}

// Stop sends "stop\n" to the managed Minecraft console, waits up to timeout for graceful exit,
// and falls back to Interrupt then Kill if the server has not exited in time.
func (m *Manager) Stop(timeout time.Duration) error {
	if timeout <= 0 {
		timeout = defaultStopTimeout
	}

	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return nil
	}
	cancel := m.cancel
	done := m.done
	stdin := m.stdin
	proc := m.proc
	m.mu.Unlock()

	if stdin != nil {
		m.stdinMu.Lock()
		_, _ = io.WriteString(stdin, "stop\n")
		m.stdinMu.Unlock()
	}
	if cancel != nil {
		cancel()
	}
	if done == nil {
		return nil
	}

	interruptAfter := timeout * 3 / 4
	if interruptAfter <= 0 {
		interruptAfter = timeout
	}
	select {
	case <-done:
		return nil
	case <-time.After(interruptAfter):
	}

	if proc != nil {
		_ = proc.Signal(os.Interrupt)
	}
	remaining := timeout - interruptAfter
	if remaining < 50*time.Millisecond {
		remaining = 50 * time.Millisecond
	}
	select {
	case <-done:
		return nil
	case <-time.After(remaining):
	}

	if proc != nil {
		_ = proc.Kill()
	}
	<-done
	return nil
}

// Status returns a thread-safe snapshot of the managed Minecraft server status.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

// ConsoleEnabled reports whether Gymbag's interactive Minecraft console integration is active.
func (m *Manager) ConsoleEnabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.consoleEnabled
}

// SetConsoleEnabled dynamically enables or disables Minecraft console integration.
func (m *Manager) SetConsoleEnabled(enabled bool) {
	m.mu.Lock()
	m.consoleEnabled = enabled
	m.status.ConsoleEnabled = enabled
	startInput := enabled && m.started && !m.consoleInputStarted && m.opts.ConsoleIn != nil && m.runCtx != nil
	runCtx := m.runCtx
	if startInput {
		m.consoleInputStarted = true
	}
	m.mu.Unlock()

	if startInput {
		go m.readConsoleInput(runCtx, m.opts.ConsoleIn)
	}
}

// ConsoleLines returns buffered console lines with Seq > sinceSeq, bounded to the latest limit lines when limit > 0.
func (m *Manager) ConsoleLines(sinceSeq uint64, limit int) []ConsoleLine {
	m.consoleMu.Lock()
	defer m.consoleMu.Unlock()

	var matched []ConsoleLine
	for _, entry := range m.consoleBuf {
		if entry.Seq > sinceSeq {
			matched = append(matched, entry)
		}
	}
	if limit > 0 && len(matched) > limit {
		matched = matched[len(matched)-limit:]
	}
	out := make([]ConsoleLine, len(matched))
	copy(out, matched)
	return out
}

// SubscribeConsole returns a snapshot of the current buffer, a live stream channel, and an idempotent unsubscribe function.
func (m *Manager) SubscribeConsole() ([]ConsoleLine, <-chan ConsoleLine, func()) {
	m.consoleMu.Lock()
	backlog := make([]ConsoleLine, len(m.consoleBuf))
	copy(backlog, m.consoleBuf)

	if m.consoleSubs == nil {
		m.consoleSubs = make(map[uint64]chan ConsoleLine)
	}
	m.nextSubID++
	subID := m.nextSubID
	ch := make(chan ConsoleLine, consoleSubscriberBufSize)
	m.consoleSubs[subID] = ch
	m.consoleMu.Unlock()

	var once sync.Once
	unsub := func() {
		once.Do(func() {
			m.consoleMu.Lock()
			if existing, ok := m.consoleSubs[subID]; ok {
				delete(m.consoleSubs, subID)
				close(existing)
			}
			m.consoleMu.Unlock()
		})
	}
	return backlog, ch, unsub
}

// SendCommand writes a single command line to the running Minecraft server's standard input.
func (m *Manager) SendCommand(raw string) error {
	trimmed := strings.Trim(raw, "\r\n")
	if strings.ContainsAny(trimmed, "\r\n") {
		return errors.New("minecraft console command must be a single line")
	}
	cmd := strings.TrimSpace(trimmed)
	if cmd == "" {
		return errors.New("minecraft console command must not be empty")
	}

	m.mu.Lock()
	if m.stdin == nil || !m.status.Running {
		m.mu.Unlock()
		return ErrNotRunning
	}
	stdin := m.stdin
	if strings.EqualFold(cmd, "stop") || strings.EqualFold(cmd, "/stop") {
		m.manualStop = true
	}
	m.mu.Unlock()

	m.stdinMu.Lock()
	_, err := io.WriteString(stdin, cmd+"\n")
	m.stdinMu.Unlock()
	return err
}

// RequestStart relaunches the managed Minecraft server after a manual stop.
func (m *Manager) RequestStart() error {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return errors.New("minecraft server supervisor has not been started")
	}
	if m.status.Running {
		m.mu.Unlock()
		return ErrAlreadyRunning
	}
	if m.status.EULABlocked {
		m.mu.Unlock()
		return errors.New("minecraft server cannot start until eula=true is set in eula.txt")
	}
	m.manualStop = false
	m.mu.Unlock()

	select {
	case m.restartCh <- struct{}{}:
	default:
	}
	return nil
}

func (m *Manager) recordConsoleLine(stream, line string) {
	ev := ConsoleLine{
		Time:   time.Now().UTC(),
		Stream: stream,
		Line:   line,
	}

	m.consoleMu.Lock()
	m.consoleSeq++
	ev.Seq = m.consoleSeq
	if m.consoleMax > 0 {
		if len(m.consoleBuf) >= m.consoleMax {
			copy(m.consoleBuf, m.consoleBuf[1:])
			m.consoleBuf[len(m.consoleBuf)-1] = ev
		} else {
			m.consoleBuf = append(m.consoleBuf, ev)
		}
	}
	for _, ch := range m.consoleSubs {
		select {
		case ch <- ev:
		default:
		}
	}
	m.consoleMu.Unlock()

	if m.ConsoleEnabled() {
		out := m.opts.ConsoleOut
		if out == nil {
			out = os.Stdout
		}
		if stream == "stderr" {
			out = m.opts.ConsoleErr
			if out == nil {
				out = os.Stderr
			}
		}
		m.consoleOutMu.Lock()
		_, _ = io.WriteString(out, line+"\n")
		m.consoleOutMu.Unlock()
		return
	}

	if stream == "system" {
		m.logger.Info(line)
	} else {
		m.logger.Info("minecraft server", "stream", stream, "line", line)
	}
}

func (m *Manager) readConsoleInput(ctx context.Context, in io.Reader) {
	sc := bufio.NewScanner(in)
	buf := make([]byte, 0, 16*1024)
	sc.Buffer(buf, 256*1024)
	for sc.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if !m.ConsoleEnabled() {
			continue
		}
		st := m.Status()
		lower := strings.ToLower(line)
		if !st.Running && (lower == "start" || lower == "/start" || lower == "restart") {
			if err := m.RequestStart(); err != nil {
				m.recordConsoleLine("system", fmt.Sprintf("[gymbag] failed to start Minecraft server: %v", err))
			}
			continue
		}
		if err := m.SendCommand(line); err != nil {
			if errors.Is(err, ErrNotRunning) {
				m.recordConsoleLine("system", "[gymbag] Minecraft server is not running. Type 'start' to relaunch.")
			} else {
				m.recordConsoleLine("system", fmt.Sprintf("[gymbag] command error: %v", err))
			}
		}
	}
}
