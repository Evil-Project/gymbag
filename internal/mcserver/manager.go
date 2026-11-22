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
	defaultRestartBaseDelay = 3 * time.Second
	maxRestartDelay         = 30 * time.Second
	stableRunReset          = 60 * time.Second
	defaultStopTimeout      = 15 * time.Second
)

// Options configures managed Minecraft server execution inside Gymbag.
type Options struct {
	BaseDir          string
	Config           config.MinecraftConfig
	Discover         DiscoverOptions
	CommandBuilder   func(ctx context.Context, javaPath string, args []string) *exec.Cmd
	Logger           *slog.Logger
	RestartBaseDelay time.Duration
}

// Status describes the live runtime state of the managed Minecraft server subprocess.
type Status struct {
	Enabled      bool   `json:"enabled"`
	Running      bool   `json:"running"`
	PID          int    `json:"pid,omitempty"`
	ServerJar    string `json:"server_jar"`
	WorkDir      string `json:"work_dir,omitempty"`
	JavaPath     string `json:"java_path,omitempty"`
	JavaVersion  string `json:"java_version,omitempty"`
	JavaMajor    int    `json:"java_major,omitempty"`
	RequiredJava int    `json:"required_java,omitempty"`
	MCVersion    string `json:"mc_version,omitempty"`
	Restarts     int    `json:"restarts,omitempty"`
	EULABlocked  bool   `json:"eula_blocked,omitempty"`
	LastError    string `json:"last_error,omitempty"`
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

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	done    chan struct{}
	stdin   io.WriteCloser
	proc    *os.Process
	status  Status
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

	return &Manager{
		opts:      opts,
		cfg:       cfg,
		jarPath:   jarPath,
		workDir:   workDir,
		propsPath: propsPath,
		logger:    logger,
		baseDelay: baseDelay,
		status: Status{
			Enabled:   jarPath != "",
			ServerJar: jarPath,
			WorkDir:   workDir,
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
	m.mu.Unlock()

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

		startAt := time.Now()
		exitErr, eulaBlocked := m.runOnce(ctx, javaPath, args)
		ranFor := time.Since(startAt)

		m.mu.Lock()
		m.status.Running = false
		m.status.PID = 0
		m.stdin = nil
		m.proc = nil
		if eulaBlocked {
			m.status.EULABlocked = true
		}
		if exitErr != nil && ctx.Err() == nil {
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
			m.logger.Info("minecraft server", "stream", stream, "line", line)
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

	_ = stdinPipe.Close()
	eulaMu.Lock()
	eulaFlag := sawEULA
	eulaMu.Unlock()
	return waitErr, eulaFlag
}

func (m *Manager) gracefulKillProcess(stdin io.WriteCloser, proc *os.Process, waitDone <-chan error) error {
	if stdin != nil {
		_, _ = io.WriteString(stdin, "stop\n")
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
		_, _ = io.WriteString(stdin, "stop\n")
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
