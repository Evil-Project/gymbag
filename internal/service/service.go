package service

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// ManagerType identifies a supported OS/init service manager.
type ManagerType string

const (
	ManagerAuto       ManagerType = "auto"
	ManagerSystemd    ManagerType = "systemd"
	ManagerLaunchd    ManagerType = "launchd"
	ManagerOpenRC     ManagerType = "openrc"
	ManagerSupervisor ManagerType = "supervisor"
)

// SupportedManagers lists the concrete service manager types supported by Gymbag.
var SupportedManagers = []ManagerType{
	ManagerSystemd,
	ManagerLaunchd,
	ManagerOpenRC,
	ManagerSupervisor,
}

// ParseManagerType normalizes a service manager name ("auto", "systemd", "launchd", "openrc", "supervisor").
func ParseManagerType(raw string) (ManagerType, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "auto":
		return ManagerAuto, nil
	case "systemd", "systemctl":
		return ManagerSystemd, nil
	case "launchd", "launchctl", "macos", "darwin":
		return ManagerLaunchd, nil
	case "openrc", "alpine":
		return ManagerOpenRC, nil
	case "supervisor", "supervisord":
		return ManagerSupervisor, nil
	default:
		return "", fmt.Errorf("unsupported service manager %q (expected auto, systemd, launchd, openrc, or supervisor)", raw)
	}
}

// DetectManager returns the active service manager for the host OS.
func DetectManager() (ManagerType, error) {
	return detectManagerWith(runtime.GOOS, os.Stat, exec.LookPath)
}

func detectManagerWith(
	goos string,
	statFn func(string) (os.FileInfo, error),
	lookPathFn func(string) (string, error),
) (ManagerType, error) {
	switch goos {
	case "darwin":
		return ManagerLaunchd, nil
	case "linux":
		if _, err := statFn("/run/systemd/system"); err == nil {
			return ManagerSystemd, nil
		}
		if _, err := lookPathFn("systemctl"); err == nil {
			return ManagerSystemd, nil
		}
		if _, err := statFn("/sbin/openrc-run"); err == nil {
			return ManagerOpenRC, nil
		}
		if _, err := lookPathFn("rc-service"); err == nil {
			return ManagerOpenRC, nil
		}
		if _, err := lookPathFn("supervisorctl"); err == nil {
			return ManagerSupervisor, nil
		}
		return "", fmt.Errorf("could not auto-detect init system on linux; specify --type (systemd, openrc, or supervisor)")
	default:
		return "", fmt.Errorf("automatic service registration is not supported on %s; use --type systemd|launchd|openrc|supervisor with render to generate a template", goos)
	}
}

// Spec describes a Gymbag service registration.
type Spec struct {
	Name            string
	Label           string
	Description     string
	BinaryPath      string
	ConfigPath      string
	WorkingDir      string
	EnvFile         string
	LogDir          string
	UnitPath        string
	User            string
	Group           string
	UserScope       bool
	Watch           bool
	PortsPrivileged bool
}

// NormalizeAndValidate fills sensible defaults and checks Spec for invalid or unsafe values.
func (s *Spec) NormalizeAndValidate(mgr ManagerType, homeDir string) error {
	if strings.TrimSpace(s.Name) == "" {
		s.Name = "gymbag"
	}
	s.Name = strings.TrimSpace(s.Name)
	if !isSafeIdentifier(s.Name) {
		return fmt.Errorf("invalid service name %q (allowed: letters, digits, '.', '_', '-')", s.Name)
	}

	if strings.TrimSpace(s.Label) == "" {
		s.Label = "io.gymbag." + s.Name
	}
	s.Label = strings.TrimSpace(s.Label)
	if !isSafeIdentifier(s.Label) {
		return fmt.Errorf("invalid service label %q (allowed: letters, digits, '.', '_', '-')", s.Label)
	}

	if strings.TrimSpace(s.Description) == "" {
		s.Description = "Gymbag HTTPS Reverse Proxy & Proof-of-Work WAF"
	}
	if containsControlChars(s.Description) {
		return fmt.Errorf("service description must not contain control characters")
	}

	if s.User != "" && !isSafeIdentifier(s.User) {
		return fmt.Errorf("invalid service user %q", s.User)
	}
	if s.Group != "" && !isSafeIdentifier(s.Group) {
		return fmt.Errorf("invalid service group %q", s.Group)
	}

	if strings.TrimSpace(s.BinaryPath) == "" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("failed to locate gymbag binary: %w", err)
		}
		s.BinaryPath = exe
	}
	if err := checkSafePath("binary path", s.BinaryPath); err != nil {
		return err
	}
	if abs, err := filepath.Abs(s.BinaryPath); err == nil {
		s.BinaryPath = abs
	}

	if strings.TrimSpace(s.ConfigPath) != "" {
		if err := checkSafePath("config path", s.ConfigPath); err != nil {
			return err
		}
		if abs, err := filepath.Abs(s.ConfigPath); err == nil {
			s.ConfigPath = abs
		}
	}

	if strings.TrimSpace(s.WorkingDir) == "" {
		if s.ConfigPath != "" {
			s.WorkingDir = filepath.Dir(s.ConfigPath)
		} else if cwd, err := os.Getwd(); err == nil {
			s.WorkingDir = cwd
		}
	}
	if s.WorkingDir != "" {
		if err := checkSafePath("working directory", s.WorkingDir); err != nil {
			return err
		}
		if abs, err := filepath.Abs(s.WorkingDir); err == nil {
			s.WorkingDir = abs
		}
	}

	if strings.TrimSpace(s.EnvFile) != "" {
		if err := checkSafePath("env file", s.EnvFile); err != nil {
			return err
		}
		if abs, err := filepath.Abs(s.EnvFile); err == nil {
			s.EnvFile = abs
		}
	}

	if strings.TrimSpace(s.LogDir) == "" {
		switch mgr {
		case ManagerLaunchd:
			if s.UserScope && homeDir != "" {
				s.LogDir = filepath.Join(homeDir, "Library", "Logs", s.Name)
			} else {
				s.LogDir = filepath.Join("/var/log", s.Name)
			}
		case ManagerSupervisor:
			if s.UserScope && s.WorkingDir != "" {
				s.LogDir = filepath.Join(s.WorkingDir, "logs")
			} else {
				s.LogDir = filepath.Join("/var/log", s.Name)
			}
		case ManagerOpenRC:
			s.LogDir = "/var/log"
		}
	}
	if s.LogDir != "" {
		if err := checkSafePath("log directory", s.LogDir); err != nil {
			return err
		}
	}

	if strings.TrimSpace(s.UnitPath) == "" {
		uPath, err := DefaultUnitPath(mgr, s.Name, s.Label, s.UserScope, homeDir)
		if err != nil {
			return err
		}
		s.UnitPath = uPath
	} else if err := checkSafePath("unit path", s.UnitPath); err != nil {
		return err
	}

	return nil
}

// DefaultUnitPath returns the standard service definition path for mgr.
func DefaultUnitPath(mgr ManagerType, name, label string, userScope bool, homeDir string) (string, error) {
	switch mgr {
	case ManagerSystemd:
		if userScope {
			if homeDir == "" {
				return "", fmt.Errorf("cannot resolve user systemd unit directory without home directory")
			}
			return filepath.Join(homeDir, ".config", "systemd", "user", name+".service"), nil
		}
		return filepath.Join("/etc/systemd/system", name+".service"), nil
	case ManagerLaunchd:
		if userScope {
			if homeDir == "" {
				return "", fmt.Errorf("cannot resolve user LaunchAgents directory without home directory")
			}
			return filepath.Join(homeDir, "Library", "LaunchAgents", label+".plist"), nil
		}
		return filepath.Join("/Library/LaunchDaemons", label+".plist"), nil
	case ManagerOpenRC:
		return filepath.Join("/etc/init.d", name), nil
	case ManagerSupervisor:
		if userScope && homeDir != "" {
			return filepath.Join(homeDir, ".config", "supervisor", "conf.d", name+".conf"), nil
		}
		return filepath.Join("/etc/supervisor/conf.d", name+".conf"), nil
	default:
		return "", fmt.Errorf("unsupported service manager %q", mgr)
	}
}

// RenderedUnit holds the rendered service definition and file metadata.
type RenderedUnit struct {
	Manager  ManagerType
	UnitPath string
	FileMode os.FileMode
	Content  string
}

// RenderUnit generates the service configuration text and target path for mgr.
func RenderUnit(mgr ManagerType, spec Spec, homeDir string) (RenderedUnit, error) {
	if err := spec.NormalizeAndValidate(mgr, homeDir); err != nil {
		return RenderedUnit{}, err
	}

	args := buildExecArgs(spec)

	switch mgr {
	case ManagerSystemd:
		return RenderedUnit{
			Manager:  mgr,
			UnitPath: spec.UnitPath,
			FileMode: 0o644,
			Content:  renderSystemdUnit(spec, args),
		}, nil
	case ManagerLaunchd:
		content, err := renderLaunchdPlist(spec, args)
		if err != nil {
			return RenderedUnit{}, err
		}
		return RenderedUnit{
			Manager:  mgr,
			UnitPath: spec.UnitPath,
			FileMode: 0o644,
			Content:  content,
		}, nil
	case ManagerOpenRC:
		return RenderedUnit{
			Manager:  mgr,
			UnitPath: spec.UnitPath,
			FileMode: 0o755,
			Content:  renderOpenRCScript(spec, args),
		}, nil
	case ManagerSupervisor:
		return RenderedUnit{
			Manager:  mgr,
			UnitPath: spec.UnitPath,
			FileMode: 0o644,
			Content:  renderSupervisorConf(spec, args),
		}, nil
	default:
		return RenderedUnit{}, fmt.Errorf("unsupported service manager %q", mgr)
	}
}

func buildExecArgs(spec Spec) []string {
	args := []string{"run"}
	if spec.ConfigPath != "" {
		args = append(args, "--config", spec.ConfigPath)
	}
	if spec.Watch {
		args = append(args, "--watch")
	}
	return args
}

func renderSystemdUnit(spec Spec, args []string) string {
	var b strings.Builder
	b.WriteString("[Unit]\n")
	b.WriteString("Description=" + spec.Description + "\n")
	b.WriteString("After=network-online.target\n")
	b.WriteString("Wants=network-online.target\n\n")

	b.WriteString("[Service]\n")
	b.WriteString("Type=simple\n")
	if !spec.UserScope {
		if spec.User != "" {
			b.WriteString("User=" + spec.User + "\n")
		}
		if spec.Group != "" {
			b.WriteString("Group=" + spec.Group + "\n")
		}
	}
	if spec.WorkingDir != "" {
		b.WriteString("WorkingDirectory=" + quoteSystemdValue(spec.WorkingDir) + "\n")
	}
	if spec.EnvFile != "" {
		b.WriteString("EnvironmentFile=-" + quoteSystemdValue(spec.EnvFile) + "\n")
	}

	execParts := make([]string, 0, 1+len(args))
	execParts = append(execParts, quoteSystemdValue(spec.BinaryPath))
	for _, a := range args {
		execParts = append(execParts, quoteSystemdValue(a))
	}
	b.WriteString("ExecStart=" + strings.Join(execParts, " ") + "\n")
	b.WriteString("Restart=on-failure\n")
	b.WriteString("RestartSec=5s\n")
	b.WriteString("LimitNOFILE=65536\n")

	if !spec.UserScope && spec.PortsPrivileged && spec.User != "" && spec.User != "root" {
		b.WriteString("AmbientCapabilities=CAP_NET_BIND_SERVICE\n")
		b.WriteString("CapabilityBoundingSet=CAP_NET_BIND_SERVICE\n")
		b.WriteString("NoNewPrivileges=true\n")
	}

	b.WriteString("\n[Install]\n")
	if spec.UserScope {
		b.WriteString("WantedBy=default.target\n")
	} else {
		b.WriteString("WantedBy=multi-user.target\n")
	}
	return b.String()
}

func renderLaunchdPlist(spec Spec, args []string) (string, error) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString(`<plist version="1.0">` + "\n")
	b.WriteString("<dict>\n")
	writePlistKeyString(&b, "Label", spec.Label)

	b.WriteString("    <key>ProgramArguments</key>\n")
	b.WriteString("    <array>\n")
	writePlistArrayString(&b, spec.BinaryPath)
	for _, a := range args {
		writePlistArrayString(&b, a)
	}
	b.WriteString("    </array>\n")

	if spec.WorkingDir != "" {
		writePlistKeyString(&b, "WorkingDirectory", spec.WorkingDir)
	}
	b.WriteString("    <key>RunAtLoad</key>\n")
	b.WriteString("    <true/>\n")
	b.WriteString("    <key>KeepAlive</key>\n")
	b.WriteString("    <dict>\n")
	b.WriteString("        <key>SuccessfulExit</key>\n")
	b.WriteString("        <false/>\n")
	b.WriteString("    </dict>\n")

	if spec.LogDir != "" {
		writePlistKeyString(&b, "StandardOutPath", filepath.Join(spec.LogDir, spec.Name+".log"))
		writePlistKeyString(&b, "StandardErrorPath", filepath.Join(spec.LogDir, spec.Name+".err.log"))
	}
	b.WriteString("</dict>\n")
	b.WriteString("</plist>\n")
	return b.String(), nil
}

func renderOpenRCScript(spec Spec, args []string) string {
	var b strings.Builder
	b.WriteString("#!/sbin/openrc-run\n\n")
	b.WriteString("name=" + quoteShell(spec.Name) + "\n")
	b.WriteString("description=" + quoteShell(spec.Description) + "\n")
	b.WriteString("command=" + quoteShell(spec.BinaryPath) + "\n")

	quotedArgs := make([]string, len(args))
	for i, a := range args {
		quotedArgs[i] = quoteShell(a)
	}
	b.WriteString("command_args=" + quoteShell(strings.Join(quotedArgs, " ")) + "\n")
	b.WriteString("command_background=true\n")
	if spec.WorkingDir != "" {
		b.WriteString("directory=" + quoteShell(spec.WorkingDir) + "\n")
	}
	if spec.User != "" {
		if spec.Group != "" {
			b.WriteString("command_user=" + quoteShell(spec.User+":"+spec.Group) + "\n")
		} else {
			b.WriteString("command_user=" + quoteShell(spec.User) + "\n")
		}
	}
	if spec.PortsPrivileged && spec.User != "" && spec.User != "root" {
		b.WriteString(`capabilities="^cap_net_bind_service"` + "\n")
	}
	b.WriteString("pidfile=" + quoteShell("/run/"+spec.Name+".pid") + "\n")
	if spec.LogDir != "" {
		b.WriteString("output_log=" + quoteShell(filepath.Join(spec.LogDir, spec.Name+".log")) + "\n")
		b.WriteString("error_log=" + quoteShell(filepath.Join(spec.LogDir, spec.Name+".err.log")) + "\n")
	}
	b.WriteString("\ndepend() {\n")
	b.WriteString("    need net\n")
	b.WriteString("    after firewall\n")
	b.WriteString("}\n")
	return b.String()
}

func renderSupervisorConf(spec Spec, args []string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[program:%s]\n", spec.Name))
	cmdParts := make([]string, 0, 1+len(args))
	cmdParts = append(cmdParts, quoteSystemdValue(spec.BinaryPath))
	for _, a := range args {
		cmdParts = append(cmdParts, quoteSystemdValue(a))
	}
	b.WriteString("command=" + strings.Join(cmdParts, " ") + "\n")
	if spec.WorkingDir != "" {
		b.WriteString("directory=" + spec.WorkingDir + "\n")
	}
	if spec.User != "" && !spec.UserScope {
		b.WriteString("user=" + spec.User + "\n")
	}
	b.WriteString("autostart=true\n")
	b.WriteString("autorestart=true\n")
	b.WriteString("stopasgroup=true\n")
	b.WriteString("killasgroup=true\n")
	b.WriteString("stopsignal=TERM\n")
	if spec.LogDir != "" {
		b.WriteString("stdout_logfile=" + filepath.Join(spec.LogDir, spec.Name+".log") + "\n")
		b.WriteString("stderr_logfile=" + filepath.Join(spec.LogDir, spec.Name+".err.log") + "\n")
	}
	return b.String()
}

func writePlistKeyString(b *strings.Builder, key, val string) {
	b.WriteString("    <key>" + escapeXML(key) + "</key>\n")
	b.WriteString("    <string>" + escapeXML(val) + "</string>\n")
}

func writePlistArrayString(b *strings.Builder, val string) {
	b.WriteString("        <string>" + escapeXML(val) + "</string>\n")
}

func escapeXML(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

func quoteSystemdValue(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, " \t\"'\\$") {
		return strconv.Quote(s)
	}
	return s
}

func quoteShell(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func isSafeIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if !isAlphaNum && r != '.' && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

func containsControlChars(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func checkSafePath(label, p string) error {
	if strings.TrimSpace(p) == "" {
		return fmt.Errorf("%s must not be empty", label)
	}
	if containsControlChars(p) {
		return fmt.Errorf("%s must not contain control characters or newlines", label)
	}
	return nil
}
