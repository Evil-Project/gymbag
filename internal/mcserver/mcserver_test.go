package mcserver

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gymbag/internal/config"
)

func buildClassHeader(javaMajor int) []byte {
	var b [8]byte
	binary.BigEndian.PutUint32(b[0:4], 0xCAFEBABE)
	binary.BigEndian.PutUint16(b[4:6], 0)
	binary.BigEndian.PutUint16(b[6:8], uint16(javaMajor+44))
	return b[:]
}

func writeZipBytes(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func writeTestJar(t *testing.T, path string, entries map[string][]byte) string {
	t.Helper()
	data := writeZipBytes(t, entries)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write jar %s: %v", path, err)
	}
	return path
}

func TestInspectServerJarAndVersionMapping(t *testing.T) {
	dir := t.TempDir()

	t.Run("version.json 1.21.4 requires Java 21", func(t *testing.T) {
		jar := writeTestJar(t, filepath.Join(dir, "paper-1.21.4.jar"), map[string][]byte{
			"version.json": []byte(`{"id":"1.21.4","java_version":21}`),
		})
		req, err := InspectServerJar(jar)
		if err != nil {
			t.Fatal(err)
		}
		if req.MinMajor != 21 || req.LegacyMinecraft || req.MCVersion != "1.21.4" || req.Source != "version.json" {
			t.Fatalf("unexpected requirement: %+v", req)
		}
	})

	t.Run("bundler nested jar under META-INF/versions", func(t *testing.T) {
		inner := writeZipBytes(t, map[string][]byte{
			"net/minecraft/server/Main.class": buildClassHeader(17),
		})
		jar := writeTestJar(t, filepath.Join(dir, "bundler-1.20.4.jar"), map[string][]byte{
			"META-INF/versions/1.20.4/server-1.20.4.jar": inner,
			"io/papermc/paperclip/Main.class":            buildClassHeader(8),
		})
		req, err := InspectServerJar(jar)
		if err != nil {
			t.Fatal(err)
		}
		if req.MinMajor != 17 || req.LegacyMinecraft || req.MCVersion != "1.20.4" {
			t.Fatalf("unexpected bundler requirement: %+v", req)
		}
	})

	t.Run("legacy Paperclip patch.properties 1.16.5 selects Java 8 legacy", func(t *testing.T) {
		jar := writeTestJar(t, filepath.Join(dir, "paper-1.16.5.jar"), map[string][]byte{
			"patch.properties": []byte("# Paperclip\nversion=1.16.5\n"),
		})
		req, err := InspectServerJar(jar)
		if err != nil {
			t.Fatal(err)
		}
		if req.MinMajor != 8 || !req.LegacyMinecraft || req.MCVersion != "1.16.5" || req.Source != "patch.properties" {
			t.Fatalf("unexpected legacy requirement: %+v", req)
		}
	})

	t.Run("classfile bytecode ignores multi-release META-INF/versions classes", func(t *testing.T) {
		jar := writeTestJar(t, filepath.Join(dir, "custom.jar"), map[string][]byte{
			"META-INF/MANIFEST.MF":             []byte("Main-Class: com.example.ServerMain\r\n"),
			"com/example/ServerMain.class":     buildClassHeader(21),
			"META-INF/versions/23/Extra.class": buildClassHeader(23),
		})
		req, err := InspectServerJar(jar)
		if err != nil {
			t.Fatal(err)
		}
		if req.MinMajor != 21 || req.Source != "classfile" {
			t.Fatalf("unexpected classfile requirement: %+v", req)
		}
	})

	t.Run("manifest Build-Jdk-Spec fallback", func(t *testing.T) {
		jar := writeTestJar(t, filepath.Join(dir, "manifest-only.jar"), map[string][]byte{
			"META-INF/MANIFEST.MF": []byte("Manifest-Version: 1.0\r\nBuild-Jdk-Spec: 17\r\n"),
		})
		req, err := InspectServerJar(jar)
		if err != nil {
			t.Fatal(err)
		}
		if req.MinMajor != 17 || req.Source != "manifest" {
			t.Fatalf("unexpected manifest requirement: %+v", req)
		}
	})

	for _, tc := range []struct {
		ver        string
		wantJava   int
		wantLegacy bool
	}{
		{"1.21.4", 21, false},
		{"1.20.5", 21, false},
		{"1.20.4-R0.1-SNAPSHOT", 17, false},
		{"1.18.2", 17, false},
		{"1.17.1", 16, false},
		{"1.16.5", 8, true},
		{"1.8.8", 8, true},
		{"26.1", 21, false},
	} {
		maj, leg, ok := MCVersionMinJava(tc.ver)
		if !ok || maj != tc.wantJava || leg != tc.wantLegacy {
			t.Fatalf("MCVersionMinJava(%q) = (%d, %v, %v), want (%d, %v, true)", tc.ver, maj, leg, ok, tc.wantJava, tc.wantLegacy)
		}
	}
}

func TestParseJavaVersionOutputDiscoverAndSelect(t *testing.T) {
	for _, tc := range []struct {
		raw       string
		wantOK    bool
		wantMajor int
		wantPatch int
	}{
		{`openjdk version "21.0.5" 2024-10-15`, true, 21, 5},
		{`java version "1.8.0_422"`, true, 8, 422},
		{`openjdk version "17"`, true, 17, 0},
		{`openjdk version "24-ea" 2025-03-18`, true, 24, 0},
		{`Unable to locate a Java Runtime.`, false, 0, 0},
	} {
		maj, _, pat, _, ok := ParseJavaVersionOutput(tc.raw)
		if ok != tc.wantOK || (ok && (maj != tc.wantMajor || pat != tc.wantPatch)) {
			t.Fatalf("ParseJavaVersionOutput(%q) = (maj=%d, pat=%d, ok=%v), want (maj=%d, pat=%d, ok=%v)",
				tc.raw, maj, pat, ok, tc.wantMajor, tc.wantPatch, tc.wantOK)
		}
	}

	installs := []JavaInstall{
		{Path: "/usr/lib/jvm/java-8/bin/java", Major: 8, Minor: 0, Patch: 422, RawVersion: "1.8.0_422", Source: "system"},
		{Path: "/usr/lib/jvm/java-17-old/bin/java", Major: 17, Minor: 0, Patch: 8, RawVersion: "17.0.8", Source: "system"},
		{Path: "/Library/Java/temurin-17/bin/java", Major: 17, Minor: 0, Patch: 13, RawVersion: "17.0.13", Source: "path"},
		{Path: "/usr/lib/jvm/java-21/bin/java", Major: 21, Minor: 0, Patch: 5, RawVersion: "21.0.5", Source: "system"},
	}

	// Modern 1.20.4 (MinMajor 17) picks highest patch Java 17 cleanly over Java 21
	best17, err := SelectBestJava(JarRequirement{MinMajor: 17, MCVersion: "1.20.4", Source: "version.json"}, installs)
	if err != nil || best17.Major != 17 || best17.Patch != 13 {
		t.Fatalf("SelectBestJava(17) = %+v, %v", best17, err)
	}

	// Legacy 1.16.5 prefers Java 8 over 17/21
	bestLegacy, err := SelectBestJava(JarRequirement{MinMajor: 8, LegacyMinecraft: true, MCVersion: "1.16.5", Source: "patch.properties"}, installs)
	if err != nil || bestLegacy.Major != 8 {
		t.Fatalf("SelectBestJava(legacy 8) = %+v, %v", bestLegacy, err)
	}

	// Missing Java 25 returns clear error listing installed runtimes
	if _, err := SelectBestJava(JarRequirement{MinMajor: 25, Source: "classfile"}, installs); err == nil || !strings.Contains(err.Error(), "requires Java 25+") || !strings.Contains(err.Error(), "Java 21") {
		t.Fatalf("expected detailed error for missing Java 25, got: %v", err)
	}

	// DiscoverJavaInstalls probes candidates and deduplicates symlinks
	binDir := t.TempDir()
	realJava := filepath.Join(binDir, "real-java")
	if err := os.WriteFile(realJava, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	symJava := filepath.Join(binDir, "java")
	_ = os.Symlink(realJava, symJava)

	probed := 0
	found := DiscoverJavaInstalls(context.Background(), DiscoverOptions{
		Candidates: []string{realJava, symJava},
		GOOS:       "test",
		Getenv:     func(string) string { return "" },
		Prober: func(_ context.Context, bin string) (string, error) {
			probed++
			return `openjdk version "21.0.4"`, nil
		},
	})
	if len(found) != 1 || probed != 1 || found[0].Major != 21 {
		t.Fatalf("expected 1 deduplicated Java 21 install (probed=%d), got %+v", probed, found)
	}

	// Explicit older java_path sets non-fatal warning
	jarDir := t.TempDir()
	jar21 := writeTestJar(t, filepath.Join(jarDir, "mc21.jar"), map[string][]byte{
		"version.json": []byte(`{"id":"1.21.4","java_version":21}`),
	})
	selWarn, err := ResolveJavaForJar(context.Background(), jar21, realJava, DiscoverOptions{
		Prober: func(context.Context, string) (string, error) { return `openjdk version "17.0.1"`, nil },
	})
	if err != nil || selWarn.Warning == "" || !strings.Contains(selWarn.Warning, "requires Java 21+") {
		t.Fatalf("expected compatibility warning for explicit Java 17, got sel=%+v err=%v", selWarn, err)
	}

}

// TestFakeJavaHelperProcess emulates a Java Minecraft server subprocess in tests.
func TestFakeJavaHelperProcess(t *testing.T) {
	mode := os.Getenv("GO_WANT_MC_HELPER")
	if mode == "" {
		return
	}
	switch mode {
	case "graceful":
		fmt.Println("[Server thread/INFO]: Done (0.123s)! For help, type \"help\"")
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "stop" {
				fmt.Println("[Server thread/INFO]: Stopping server")
				os.Exit(0)
			}
			if line != "" {
				fmt.Printf("[Server thread/INFO]: ECHO %s\n", line)
			}
		}
		os.Exit(0)
	case "eula":
		fmt.Println("You need to agree to the EULA in order to run the server. Go to eula.txt for more info.")
		os.Exit(0)
	case "crash-then-stay":
		marker := os.Getenv("GO_MC_CRASH_MARKER")
		if _, err := os.Stat(marker); os.IsNotExist(err) {
			_ = os.WriteFile(marker, []byte("crashed"), 0o644)
			fmt.Println("simulated initial crash")
			os.Exit(1)
		}
		fmt.Println("restarted cleanly")
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			if strings.TrimSpace(sc.Text()) == "stop" {
				os.Exit(0)
			}
		}
		os.Exit(0)
	}
}

func helperCommandBuilder(mode string, extraEnv ...string) func(context.Context, string, []string) *exec.Cmd {
	return func(ctx context.Context, _ string, _ []string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestFakeJavaHelperProcess$")
		cmd.Env = append(os.Environ(), "GO_WANT_MC_HELPER="+mode)
		cmd.Env = append(cmd.Env, extraEnv...)
		return cmd
	}
}

func TestManagerLifecycleGracefulStopEULAAndAutoRestart(t *testing.T) {
	t.Run("graceful start, properties seeding, and stop command", func(t *testing.T) {
		dir := t.TempDir()
		jarPath := writeTestJar(t, filepath.Join(dir, "server.jar"), map[string][]byte{
			"version.json": []byte(`{"id":"1.21.4","java_version":21}`),
		})

		mgr := NewManager(Options{
			Config: config.MinecraftConfig{
				Enabled:   true,
				Port:      25565,
				Upstream:  "127.0.0.1:41234",
				ServerJar: jarPath,
			},
			Discover: DiscoverOptions{
				Installs: []JavaInstall{
					{Path: "/usr/bin/java17", Major: 17, RawVersion: "17.0.10", Source: "system"},
					{Path: "/usr/bin/java21", Major: 21, RawVersion: "21.0.5", Source: "system"},
				},
			},
			CommandBuilder: helperCommandBuilder("graceful"),
			Logger:         slog.Default(),
		})

		if err := mgr.Start(context.Background()); err != nil {
			t.Fatalf("Manager.Start: %v", err)
		}

		// Verify missing server.properties was created with upstream host/port
		propsBytes, err := os.ReadFile(filepath.Join(dir, "server.properties"))
		if err != nil || !strings.Contains(string(propsBytes), "server-port=41234") || !strings.Contains(string(propsBytes), "server-ip=127.0.0.1") {
			t.Fatalf("unexpected seeded server.properties (%v):\n%s", err, string(propsBytes))
		}

		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			st := mgr.Status()
			if st.Running && st.PID > 0 {
				break
			}
			time.Sleep(15 * time.Millisecond)
		}
		st := mgr.Status()
		if !st.Running || st.JavaMajor != 21 || st.JavaPath != "/usr/bin/java21" || st.RequiredJava != 21 {
			t.Fatalf("unexpected running status: %+v", st)
		}

		if err := mgr.Stop(5 * time.Second); err != nil {
			t.Fatalf("Manager.Stop: %v", err)
		}
		stAfter := mgr.Status()
		if stAfter.Running || stAfter.PID != 0 {
			t.Fatalf("expected stopped status after Stop, got %+v", stAfter)
		}
	})

	t.Run("eula refusal halts without crash loop", func(t *testing.T) {
		dir := t.TempDir()
		jarPath := writeTestJar(t, filepath.Join(dir, "server.jar"), map[string][]byte{
			"version.json": []byte(`{"id":"1.21.4","java_version":21}`),
		})
		mgr := NewManager(Options{
			Config: config.MinecraftConfig{
				Enabled:   true,
				Upstream:  "127.0.0.1:41235",
				ServerJar: jarPath,
			},
			Discover: DiscoverOptions{
				Installs: []JavaInstall{{Path: "/usr/bin/java21", Major: 21, RawVersion: "21.0.5", Source: "system"}},
			},
			CommandBuilder:   helperCommandBuilder("eula"),
			RestartBaseDelay: 50 * time.Millisecond,
		})
		if err := mgr.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = mgr.Stop(2 * time.Second) }()

		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			st := mgr.Status()
			if st.EULABlocked && !st.Running {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		st := mgr.Status()
		if !st.EULABlocked || st.Running || st.Restarts != 0 {
			t.Fatalf("expected EULABlocked without restarts, got %+v", st)
		}
	})

	t.Run("unexpected exit triggers auto restart", func(t *testing.T) {
		dir := t.TempDir()
		jarPath := writeTestJar(t, filepath.Join(dir, "server.jar"), map[string][]byte{
			"version.json": []byte(`{"id":"1.20.4","java_version":17}`),
		})
		marker := filepath.Join(dir, "crashed.once")
		mgr := NewManager(Options{
			Config: config.MinecraftConfig{
				Enabled:   true,
				Upstream:  "127.0.0.1:41236",
				ServerJar: jarPath,
			},
			Discover: DiscoverOptions{
				Installs: []JavaInstall{{Path: "/usr/bin/java17", Major: 17, RawVersion: "17.0.13", Source: "system"}},
			},
			CommandBuilder:   helperCommandBuilder("crash-then-stay", "GO_MC_CRASH_MARKER="+marker),
			RestartBaseDelay: 30 * time.Millisecond,
		})
		if err := mgr.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = mgr.Stop(3 * time.Second) }()

		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			st := mgr.Status()
			if st.Restarts >= 1 && st.Running {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("expected auto-restart to bring child back up, status=%+v", mgr.Status())
	})
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestManagerInteractiveConsoleBufferStreamAndManualStop(t *testing.T) {
	dir := t.TempDir()
	jarPath := writeTestJar(t, filepath.Join(dir, "server.jar"), map[string][]byte{
		"version.json": []byte(`{"id":"1.21.4","java_version":21}`),
	})

	consoleOut := &syncBuffer{}
	consoleErr := &syncBuffer{}
	inR, inW := io.Pipe()
	defer func() {
		_ = inW.Close()
		_ = inR.Close()
	}()

	mgr := NewManager(Options{
		Config: config.MinecraftConfig{
			Enabled:   true,
			Upstream:  "127.0.0.1:41237",
			ServerJar: jarPath,
			Console:   true,
		},
		Discover: DiscoverOptions{
			Installs: []JavaInstall{{Path: "/usr/bin/java21", Major: 21, RawVersion: "21.0.5", Source: "system"}},
		},
		CommandBuilder:    helperCommandBuilder("graceful"),
		ConsoleIn:         inR,
		ConsoleOut:        consoleOut,
		ConsoleErr:        consoleErr,
		ConsoleBufferSize: 50,
		RestartBaseDelay:  20 * time.Millisecond,
	})

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Manager.Start: %v", err)
	}
	defer func() { _ = mgr.Stop(3 * time.Second) }()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if st := mgr.Status(); st.Running && st.ConsoleEnabled && strings.Contains(consoleOut.String(), `Done (0.123s)!`) {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if !strings.Contains(consoleOut.String(), `Done (0.123s)!`) {
		t.Fatalf("expected raw Minecraft startup line in ConsoleOut, got:\n%s", consoleOut.String())
	}
	// Verify raw format without slog level/msg wrapping
	if strings.Contains(consoleOut.String(), "level=INFO") || strings.Contains(consoleOut.String(), "stream=stdout") {
		t.Fatalf("expected raw Minecraft console lines without slog wrapping, got:\n%s", consoleOut.String())
	}

	backlog, subCh, unsub := mgr.SubscribeConsole()
	defer unsub()
	if len(backlog) == 0 {
		t.Fatalf("expected non-empty console backlog from startup")
	}

	// Interactive ConsoleIn -> subprocess -> ConsoleOut + SubscribeConsole + ConsoleLines
	if _, err := io.WriteString(inW, "say hello-gymbag\n"); err != nil {
		t.Fatal(err)
	}
	gotEcho := false
	subDeadline := time.After(3 * time.Second)
	for !gotEcho {
		select {
		case ev := <-subCh:
			if strings.Contains(ev.Line, "ECHO say hello-gymbag") {
				gotEcho = true
			}
		case <-subDeadline:
			t.Fatalf("timed out waiting for streamed ECHO line; out=%q buffered=%+v", consoleOut.String(), mgr.ConsoleLines(0, 0))
		}
	}
	if !strings.Contains(consoleOut.String(), "ECHO say hello-gymbag") {
		t.Fatalf("expected echoed command in ConsoleOut, got:\n%s", consoleOut.String())
	}
	lines := mgr.ConsoleLines(0, 0)
	if len(lines) < 2 {
		t.Fatalf("expected >=2 buffered console lines, got %+v", lines)
	}

	// SendCommand("stop") stops cleanly without auto-restart crash-looping
	if err := mgr.SendCommand("stop"); err != nil {
		t.Fatalf("SendCommand(stop): %v", err)
	}
	stopDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(stopDeadline) {
		st := mgr.Status()
		if !st.Running && strings.Contains(consoleOut.String(), "type 'start' to relaunch") {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	stStopped := mgr.Status()
	if stStopped.Running || stStopped.Restarts != 0 {
		t.Fatalf("expected manual stop to pause cleanly without Restarts, got %+v", stStopped)
	}
	time.Sleep(80 * time.Millisecond)
	if stCheck := mgr.Status(); stCheck.Running || stCheck.Restarts != 0 {
		t.Fatalf("expected server to stay paused after manual stop, got %+v", stCheck)
	}

	// Typing 'start' or calling RequestStart() relaunches cleanly
	if _, err := io.WriteString(inW, "start\n"); err != nil {
		t.Fatal(err)
	}
	relaunchDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(relaunchDeadline) {
		if mgr.Status().Running {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if !mgr.Status().Running {
		t.Fatalf("expected 'start' via console stdin to relaunch Minecraft server, status=%+v", mgr.Status())
	}
}
