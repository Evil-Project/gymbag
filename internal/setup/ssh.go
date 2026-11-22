package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	DefaultSSHDConfigPath = "/etc/ssh/sshd_config"

	sshdManagedStart = "# gymbag-managed:start"
	sshdManagedEnd   = "# gymbag-managed:end"
)

// CommandRunner executes external setup commands (injectable for tests).
type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// LookPathFunc resolves an executable in PATH (injectable for tests).
type LookPathFunc func(file string) (string, error)

// SSHSetupOptions configures automated sshd_config port randomization and firewall rules.
type SSHSetupOptions struct {
	ConfigPath    string
	BackendPort   int
	SSHListenPort int
	VerifyPort    int
	SkipFirewall  bool
	Runner        CommandRunner
	LookPath      LookPathFunc
	GOOS          string
}

// SSHSetupResult summarizes automated SSH protection changes made on the host.
type SSHSetupResult struct {
	ConfigPath       string
	BackupPath       string
	BackendPort      int
	Upstream         string
	FirewallBackend  string
	FirewallCommands []string
	Warnings         []string
}

func defaultCommandRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// RewriteSSHDConfig comments out active top-level Port lines and inserts a managed Port directive
// before the first Match block (idempotent across repeated invocations).
func RewriteSSHDConfig(content string, newPort int) string {
	lineEnding := "\n"
	if strings.Contains(content, "\r\n") {
		lineEnding = "\r\n"
	}
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	rawLines := strings.Split(normalized, "\n")

	cleaned := make([]string, 0, len(rawLines)+4)
	inManaged := false
	inMatch := false

	for _, line := range rawLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == sshdManagedStart {
			inManaged = true
			continue
		}
		if trimmed == sshdManagedEnd {
			inManaged = false
			continue
		}
		if inManaged {
			continue
		}
		if !inMatch && isSSHDMatchDirective(trimmed) {
			inMatch = true
		}
		if !inMatch && isSSHDPortDirective(trimmed) {
			cleaned = append(cleaned, "# "+trimmed+" # disabled by gymbag")
			continue
		}
		cleaned = append(cleaned, line)
	}

	// Trim trailing empty placeholder line created by Split if content ended with "\n"
	hadTrailingNL := strings.HasSuffix(normalized, "\n")
	if hadTrailingNL && len(cleaned) > 0 && cleaned[len(cleaned)-1] == "" {
		cleaned = cleaned[:len(cleaned)-1]
	}

	managedBlock := []string{
		sshdManagedStart,
		fmt.Sprintf("Port %d", newPort),
		sshdManagedEnd,
	}

	insertIdx := len(cleaned)
	for i, line := range cleaned {
		if isSSHDMatchDirective(strings.TrimSpace(line)) {
			insertIdx = i
			break
		}
	}

	out := make([]string, 0, len(cleaned)+len(managedBlock))
	out = append(out, cleaned[:insertIdx]...)
	out = append(out, managedBlock...)
	out = append(out, cleaned[insertIdx:]...)

	res := strings.Join(out, lineEnding)
	if hadTrailingNL || res != "" {
		res += lineEnding
	}
	return res
}

func isSSHDPortDirective(trimmed string) bool {
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return false
	}
	fields := strings.Fields(trimmed)
	return len(fields) >= 2 && strings.EqualFold(fields[0], "Port")
}

func isSSHDMatchDirective(trimmed string) bool {
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return false
	}
	fields := strings.Fields(trimmed)
	return len(fields) >= 2 && strings.EqualFold(fields[0], "Match")
}

// SetupSSHProtection updates sshd_config to a random internal port, validates with `sshd -t`,
// reloads sshd, and configures the host firewall to block external access to the backend port.
func SetupSSHProtection(ctx context.Context, opts SSHSetupOptions) (*SSHSetupResult, error) {
	cfgPath := strings.TrimSpace(opts.ConfigPath)
	if cfgPath == "" {
		cfgPath = DefaultSSHDConfigPath
	}
	sshPort := opts.SSHListenPort
	if sshPort <= 0 {
		sshPort = 22
	}
	verifyPort := opts.VerifyPort
	if verifyPort <= 0 {
		verifyPort = 8443
	}
	backendPort := opts.BackendPort
	if backendPort <= 0 {
		var err error
		backendPort, err = PickRandomFreePort(sshPort, verifyPort)
		if err != nil {
			return nil, err
		}
	}

	runner := opts.Runner
	if runner == nil {
		runner = defaultCommandRunner
	}
	customLookPath := opts.LookPath != nil
	lookPath := opts.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}

	info, err := os.Stat(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat sshd config %s: %w", cfgPath, err)
	}
	origBytes, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read sshd config %s: %w", cfgPath, err)
	}

	backupPath := cfgPath + ".gymbag.bak"
	if _, statErr := os.Stat(backupPath); errors.Is(statErr, os.ErrNotExist) {
		if wErr := os.WriteFile(backupPath, origBytes, info.Mode().Perm()); wErr != nil {
			return nil, fmt.Errorf("failed to create sshd backup %s: %w", backupPath, wErr)
		}
	}

	updated := RewriteSSHDConfig(string(origBytes), backendPort)
	if err := writeFilePreserveMode(cfgPath, []byte(updated), info.Mode().Perm()); err != nil {
		return nil, fmt.Errorf("failed to update %s: %w", cfgPath, err)
	}

	if sshdBin := findSSHDBinary(lookPath, !customLookPath); sshdBin != "" {
		if out, tErr := runner(ctx, sshdBin, "-t", "-f", cfgPath); tErr != nil {
			_ = writeFilePreserveMode(cfgPath, origBytes, info.Mode().Perm())
			return nil, fmt.Errorf("sshd -t validation failed (rolled back %s): %v (%s)", cfgPath, tErr, strings.TrimSpace(string(out)))
		}
	}

	res := &SSHSetupResult{
		ConfigPath:  cfgPath,
		BackupPath:  backupPath,
		BackendPort: backendPort,
		Upstream:    fmt.Sprintf("127.0.0.1:%d", backendPort),
	}

	if warn := reloadSSHD(ctx, runner, lookPath, goos); warn != "" {
		res.Warnings = append(res.Warnings, warn)
	}

	if !opts.SkipFirewall {
		fwBackend, cmds, fwWarnings, fwErr := ConfigureSSHFirewall(ctx, backendPort, sshPort, verifyPort, runner, lookPath, goos)
		res.FirewallBackend = fwBackend
		res.FirewallCommands = cmds
		res.Warnings = append(res.Warnings, fwWarnings...)
		if fwErr != nil {
			return nil, fwErr
		}
	}
	return res, nil
}

func findSSHDBinary(lookPath LookPathFunc, fallbackPaths bool) string {
	if p, err := lookPath("sshd"); err == nil && p != "" {
		return p
	}
	if !fallbackPaths {
		return ""
	}
	for _, cand := range []string{"/usr/sbin/sshd", "/sbin/sshd"} {
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	return ""
}

func reloadSSHD(ctx context.Context, runner CommandRunner, lookPath LookPathFunc, goos string) string {
	if goos != "linux" {
		return ""
	}
	if _, err := lookPath("systemctl"); err == nil {
		for _, unit := range []string{"sshd", "ssh"} {
			if _, rErr := runner(ctx, "systemctl", "reload", unit); rErr == nil {
				return ""
			}
		}
	}
	if _, err := lookPath("service"); err == nil {
		for _, svc := range []string{"ssh", "sshd"} {
			if _, rErr := runner(ctx, "service", svc, "reload"); rErr == nil {
				return ""
			}
		}
	}
	if _, err := lookPath("rc-service"); err == nil {
		if _, rErr := runner(ctx, "rc-service", "sshd", "reload"); rErr == nil {
			return ""
		}
	}
	return "sshd_config updated, but automatic sshd reload failed; reload sshd manually (e.g. sudo systemctl reload sshd)"
}

// ConfigureSSHFirewall configures the active host firewall to block external access to backendPort
// while allowing loopback access and exposing sshListenPort and verifyPort.
func ConfigureSSHFirewall(
	ctx context.Context,
	backendPort, sshListenPort, verifyPort int,
	runner CommandRunner,
	lookPath LookPathFunc,
	goos string,
) (backend string, cmds []string, warnings []string, err error) {
	if runner == nil {
		runner = defaultCommandRunner
	}
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if goos == "" {
		goos = runtime.GOOS
	}
	if goos != "linux" {
		return "none", nil, []string{
			fmt.Sprintf("automatic SSH firewall setup is Linux-only; ensure port %d/tcp is blocked externally on %s", backendPort, goos),
		}, nil
	}

	run := func(name string, args ...string) ([]byte, error) {
		cmds = append(cmds, name+" "+strings.Join(args, " "))
		return runner(ctx, name, args...)
	}

	bPort := strconv.Itoa(backendPort)
	sPort := strconv.Itoa(sshListenPort)
	vPort := strconv.Itoa(verifyPort)

	// 1. ufw (active)
	if _, lpErr := lookPath("ufw"); lpErr == nil {
		if stOut, stErr := runner(ctx, "ufw", "status"); stErr == nil && strings.Contains(strings.ToLower(string(stOut)), "status: active") {
			if _, rErr := run("ufw", "allow", "in", "on", "lo", "to", "any", "port", bPort, "proto", "tcp"); rErr != nil {
				return "ufw", cmds, nil, fmt.Errorf("ufw loopback allow failed: %w", rErr)
			}
			if _, rErr := run("ufw", "deny", bPort+"/tcp"); rErr != nil {
				return "ufw", cmds, nil, fmt.Errorf("ufw deny %s/tcp failed: %w", bPort, rErr)
			}
			_, _ = run("ufw", "allow", sPort+"/tcp")
			_, _ = run("ufw", "allow", vPort+"/tcp")
			return "ufw", cmds, nil, nil
		}
	}

	// 2. firewalld (running)
	if _, lpErr := lookPath("firewall-cmd"); lpErr == nil {
		if stOut, stErr := runner(ctx, "firewall-cmd", "--state"); stErr == nil && strings.Contains(strings.ToLower(string(stOut)), "running") {
			_, _ = run("firewall-cmd", "--permanent", "--remove-port="+bPort+"/tcp")
			richRule := fmt.Sprintf(`rule family="ipv4" port port="%s" protocol="tcp" drop`, bPort)
			if _, rErr := run("firewall-cmd", "--permanent", "--add-rich-rule="+richRule); rErr != nil {
				return "firewalld", cmds, nil, fmt.Errorf("firewalld rich-rule failed: %w", rErr)
			}
			_, _ = run("firewall-cmd", "--permanent", "--add-port="+sPort+"/tcp")
			_, _ = run("firewall-cmd", "--permanent", "--add-port="+vPort+"/tcp")
			if _, rErr := run("firewall-cmd", "--reload"); rErr != nil {
				return "firewalld", cmds, nil, fmt.Errorf("firewalld reload failed: %w", rErr)
			}
			return "firewalld", cmds, nil, nil
		}
	}

	// 3. nftables
	if _, lpErr := lookPath("nft"); lpErr == nil {
		if _, stErr := runner(ctx, "nft", "list", "tables"); stErr == nil {
			if _, rErr := run("nft", "add", "table", "inet", "gymbag_ssh"); rErr != nil {
				return "nftables", cmds, nil, fmt.Errorf("nft add table failed: %w", rErr)
			}
			_, _ = run("nft", "flush", "table", "inet", "gymbag_ssh")
			if _, rErr := run("nft", "add", "chain", "inet", "gymbag_ssh", "input", "{", "type", "filter", "hook", "input", "priority", "-10", ";", "policy", "accept", ";", "}"); rErr != nil {
				return "nftables", cmds, nil, fmt.Errorf("nft add chain failed: %w", rErr)
			}
			_, _ = run("nft", "add", "rule", "inet", "gymbag_ssh", "input", "iifname", `"lo"`, "accept")
			if _, rErr := run("nft", "add", "rule", "inet", "gymbag_ssh", "input", "tcp", "dport", bPort, "drop"); rErr != nil {
				return "nftables", cmds, nil, fmt.Errorf("nft add drop rule failed: %w", rErr)
			}
			return "nftables", cmds, nil, nil
		}
	}

	// 4. iptables (+ ip6tables)
	if _, lpErr := lookPath("iptables"); lpErr == nil {
		dropArgs := []string{"INPUT", "!", "-i", "lo", "-p", "tcp", "--dport", bPort, "-j", "DROP"}
		if _, checkErr := runner(ctx, "iptables", append([]string{"-C"}, dropArgs...)...); checkErr != nil {
			if _, rErr := run("iptables", append([]string{"-I", "INPUT", "1"}, dropArgs[1:]...)...); rErr != nil {
				return "iptables", cmds, nil, fmt.Errorf("iptables rule failed: %w", rErr)
			}
		}
		for _, openPort := range []string{sPort, vPort} {
			allowArgs := []string{"INPUT", "-p", "tcp", "--dport", openPort, "-j", "ACCEPT"}
			if _, checkErr := runner(ctx, "iptables", append([]string{"-C"}, allowArgs...)...); checkErr != nil {
				_, _ = run("iptables", append([]string{"-I", "INPUT", "1"}, allowArgs[1:]...)...)
			}
		}
		if _, lp6Err := lookPath("ip6tables"); lp6Err == nil {
			if _, checkErr := runner(ctx, "ip6tables", append([]string{"-C"}, dropArgs...)...); checkErr != nil {
				_, _ = run("ip6tables", append([]string{"-I", "INPUT", "1"}, dropArgs[1:]...)...)
			}
		}
		return "iptables", cmds, nil, nil
	}

	return "none", nil, []string{
		fmt.Sprintf("no active firewall manager detected (ufw, firewalld, nft, iptables); manually block external TCP port %d while keeping %d and %d open", backendPort, sshListenPort, verifyPort),
	}, nil
}

func writeFilePreserveMode(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	tmp, err := os.CreateTemp(dir, ".gymbag-setup-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}
