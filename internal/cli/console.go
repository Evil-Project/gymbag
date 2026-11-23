package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"gymbag/internal/config"
	"gymbag/internal/mcserver"
)

var consoleHTTPClientFactory = func(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

func runConsoleCmd(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gymbag console", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		cfgPath   string
		adminAddr string
		cmdLine   string
		tail      int
		noStream  bool
	)
	fs.StringVar(&cfgPath, "c", "", "Path to config file (used to look up admin_addr)")
	fs.StringVar(&cfgPath, "config", "", "Path to config file (used to look up admin_addr)")
	fs.StringVar(&adminAddr, "admin", "", "Admin API listen address or URL (defaults to config admin_addr or 127.0.0.1:2019)")
	fs.StringVar(&cmdLine, "cmd", "", "Send a single console command to the managed Minecraft server and exit")
	fs.IntVar(&tail, "tail", 200, "Number of buffered console lines to display on attach")
	fs.BoolVar(&noStream, "no-stream", false, "Print buffered console output and exit without live streaming")

	if err := parseInterspersedFlags(fs, args); err != nil {
		if err == flag.ErrHelp {
			printConsoleUsage(stdout)
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 && strings.TrimSpace(cmdLine) == "" {
		cmdLine = strings.Join(fs.Args(), " ")
	}
	if tail < 0 {
		_, _ = fmt.Fprintln(stderr, "ERROR gymbag console: --tail must be >= 0")
		return 2
	}

	baseURL, err := resolveConsoleAdminBaseURL(adminAddr, cfgPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag console: %v\n", err)
		return 1
	}

	if strings.TrimSpace(cmdLine) != "" {
		client := consoleHTTPClientFactory(10 * time.Second)
		action, err := sendConsoleCommandHTTP(context.Background(), client, baseURL, cmdLine)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag console: %v\n", err)
			return 1
		}
		if action == "started" {
			_, _ = fmt.Fprintln(stdout, "Requested Minecraft server start.")
		} else {
			_, _ = fmt.Fprintf(stdout, "Sent command: %s\n", strings.TrimSpace(cmdLine))
		}
		return 0
	}

	if noStream {
		client := consoleHTTPClientFactory(10 * time.Second)
		lines, err := fetchConsoleSnapshotHTTP(context.Background(), client, baseURL, tail)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag console: %v\n", err)
			return 1
		}
		for _, ln := range lines {
			_, _ = fmt.Fprintln(stdout, ln.Line)
		}
		return 0
	}

	return streamInteractiveConsole(baseURL, tail, stdin, stdout, stderr)
}

func resolveConsoleAdminBaseURL(adminFlag, cfgPath string) (string, error) {
	raw := strings.TrimSpace(adminFlag)
	if strings.EqualFold(raw, "off") || strings.EqualFold(raw, "none") {
		return "", fmt.Errorf("admin API is required to attach to Minecraft console")
	}
	if raw == "" {
		resolvedCfg := strings.TrimSpace(cfgPath)
		if resolvedCfg == "" {
			resolvedCfg = resolveWatchedPath("")
		}
		if resolvedCfg != "" {
			cfg, err := config.LoadWithEnv(config.LoadOptions{ConfigPath: resolvedCfg}, os.Getenv)
			if err != nil {
				return "", err
			}
			raw = strings.TrimSpace(cfg.AdminAddr)
		}
	}
	if raw == "" {
		raw = "127.0.0.1:2019"
	}
	if strings.HasPrefix(raw, ":") {
		raw = "127.0.0.1" + raw
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	return strings.TrimRight(raw, "/"), nil
}

func decodeAdminAPIError(body []byte, statusCode int) string {
	var parsed struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Error.Message != "" {
		if parsed.Error.Code != "" {
			return fmt.Sprintf("%s (%s)", parsed.Error.Message, parsed.Error.Code)
		}
		return parsed.Error.Message
	}
	msg := strings.TrimSpace(string(body))
	if msg != "" {
		return msg
	}
	return fmt.Sprintf("HTTP %d", statusCode)
}

func fetchConsoleSnapshotHTTP(ctx context.Context, client *http.Client, baseURL string, limit int) ([]mcserver.ConsoleLine, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/v1/minecraft/console?limit=%d", baseURL, limit), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Admin API at %s (ensure Gymbag is running with admin_addr enabled): %w", baseURL, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s", decodeAdminAPIError(raw, resp.StatusCode))
	}

	var payload struct {
		Lines []mcserver.ConsoleLine `json:"lines"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("invalid console response: %w", err)
	}
	return payload.Lines, nil
}

func sendConsoleCommandHTTP(ctx context.Context, client *http.Client, baseURL, cmd string) (string, error) {
	bodyBytes, err := json.Marshal(map[string]string{"command": cmd})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/minecraft/console", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to connect to Admin API at %s: %w", baseURL, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s", decodeAdminAPIError(raw, resp.StatusCode))
	}
	var ack struct {
		Action string `json:"action"`
	}
	_ = json.Unmarshal(raw, &ack)
	return ack.Action, nil
}

func streamInteractiveConsole(baseURL string, tail int, stdin io.Reader, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	streamClient := consoleHTTPClientFactory(0)
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("%s/api/v1/minecraft/console?stream=1&limit=%d", baseURL, tail),
		nil,
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag console: %v\n", err)
		return 1
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := streamClient.Do(req)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag console: failed to connect to Admin API at %s (ensure Gymbag is running with admin_addr enabled): %v\n", baseURL, err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag console: %s\n", decodeAdminAPIError(raw, resp.StatusCode))
		return 1
	}

	var ioMu sync.Mutex
	writeOut := func(s string) {
		ioMu.Lock()
		_, _ = fmt.Fprintln(stdout, s)
		ioMu.Unlock()
	}
	writeErr := func(format string, args ...any) {
		ioMu.Lock()
		_, _ = fmt.Fprintf(stderr, format, args...)
		ioMu.Unlock()
	}

	writeErr("Attached to Gymbag Minecraft console (%s). Type commands directly, or '.exit' / Ctrl+C to detach.\n", baseURL)

	streamErrCh := make(chan error, 1)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		buf := make([]byte, 0, 64*1024)
		sc.Buffer(buf, 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if dataPayload, ok := strings.CutPrefix(line, "data:"); ok {
				dataPayload = strings.TrimSpace(dataPayload)
				if dataPayload == "" {
					continue
				}
				var entry mcserver.ConsoleLine
				if json.Unmarshal([]byte(dataPayload), &entry) == nil {
					writeOut(entry.Line)
				} else {
					writeOut(dataPayload)
				}
			}
		}
		streamErrCh <- sc.Err()
	}()

	if stdin != nil {
		go func() {
			cmdClient := consoleHTTPClientFactory(10 * time.Second)
			sc := bufio.NewScanner(stdin)
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
				if strings.EqualFold(line, ".exit") || strings.EqualFold(line, ".quit") {
					stop()
					return
				}
				action, err := sendConsoleCommandHTTP(ctx, cmdClient, baseURL, line)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					writeErr("ERROR gymbag console: %v\n", err)
					continue
				}
				if action == "started" {
					writeErr("[gymbag] requested Minecraft server start\n")
				}
			}
		}()
	}

	select {
	case <-ctx.Done():
		_ = resp.Body.Close()
		<-streamErrCh
		return 0
	case scanErr := <-streamErrCh:
		if scanErr != nil && ctx.Err() == nil {
			writeErr("ERROR gymbag console: stream closed: %v\n", scanErr)
			return 1
		}
		return 0
	}
}

func printConsoleUsage(w io.Writer) {
	_, _ = fmt.Fprintf(w, `Usage:
  gymbag console [flags] [command...]

Examples:
  gymbag console                                  # Attach interactively via Admin API (config or 127.0.0.1:2019)
  gymbag console -c gymbag.json                   # Use admin_addr from gymbag.json
  gymbag console --admin 127.0.0.1:2019 --cmd "say Hello from Gymbag"
  gymbag console --no-stream --tail 100           # Print last 100 buffered console lines and exit

Flags:
  -c, --config <path>   Path to config file (used to look up admin_addr)
      --admin <addr>    Admin API address or URL (defaults to config admin_addr or 127.0.0.1:2019)
      --cmd <command>   Send a single Minecraft console command and exit
      --tail <n>        Number of buffered console lines to show on attach (default 200)
      --no-stream       Print buffered console lines and exit without live streaming
`)
}
