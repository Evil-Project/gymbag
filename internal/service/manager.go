package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// CommandRunner executes an OS command and returns combined stdout/stderr output.
type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

func defaultCommandRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// Options configures service installation and lifecycle management.
type Options struct {
	Manager   ManagerType
	Spec      Spec
	Overwrite bool
	Enable    bool
	Start     bool
	NoReload  bool
	HomeDir   string
	UID       int
	Runner    CommandRunner
}

// InstallResult describes the outcome of Install.
type InstallResult struct {
	Manager     ManagerType
	UnitPath    string
	Content     string
	CommandsRun []string
	Output      string
}

// ActionResult describes the outcome of Start/Stop/Restart/Status/Uninstall.
type ActionResult struct {
	Manager     ManagerType
	UnitPath    string
	CommandsRun []string
	Output      string
}

func (o *Options) prepare() (ManagerType, string, int, CommandRunner, error) {
	mgr := o.Manager
	if mgr == "" || mgr == ManagerAuto {
		detected, err := DetectManager()
		if err != nil {
			return "", "", 0, nil, err
		}
		mgr = detected
	}
	home := o.HomeDir
	if home == "" {
		if h, err := os.UserHomeDir(); err == nil {
			home = h
		}
	}
	uid := o.UID
	if uid == 0 {
		uid = os.Getuid()
	}
	runner := o.Runner
	if runner == nil {
		runner = defaultCommandRunner
	}
	return mgr, home, uid, runner, nil
}

// Install renders and writes the service unit file and optionally reloads, enables, and starts it.
func Install(ctx context.Context, opts Options) (InstallResult, error) {
	mgr, home, uid, runner, err := opts.prepare()
	if err != nil {
		return InstallResult{}, err
	}

	rendered, err := RenderUnit(mgr, opts.Spec, home)
	if err != nil {
		return InstallResult{}, err
	}

	// Re-normalize spec so Spec.Label / Spec.LogDir / Spec.UnitPath are populated for lifecycle calls.
	spec := opts.Spec
	if err := spec.NormalizeAndValidate(mgr, home); err != nil {
		return InstallResult{}, err
	}

	if !opts.Overwrite {
		if _, statErr := os.Stat(rendered.UnitPath); statErr == nil {
			return InstallResult{}, fmt.Errorf("service file %s already exists (use --force to overwrite)", rendered.UnitPath)
		}
	}

	if err := os.MkdirAll(filepath.Dir(rendered.UnitPath), 0o755); err != nil {
		return InstallResult{}, wrapPermErr(err, filepath.Dir(rendered.UnitPath), mgr, spec.UserScope)
	}
	if (mgr == ManagerLaunchd || mgr == ManagerSupervisor) && spec.LogDir != "" {
		if err := os.MkdirAll(spec.LogDir, 0o755); err != nil {
			return InstallResult{}, wrapPermErr(err, spec.LogDir, mgr, spec.UserScope)
		}
	}

	if err := os.WriteFile(rendered.UnitPath, []byte(rendered.Content), rendered.FileMode); err != nil {
		return InstallResult{}, wrapPermErr(err, rendered.UnitPath, mgr, spec.UserScope)
	}

	res := InstallResult{
		Manager:  mgr,
		UnitPath: rendered.UnitPath,
		Content:  rendered.Content,
	}

	if opts.NoReload {
		return res, nil
	}

	var outputs []string
	run := func(bin string, args ...string) error {
		res.CommandsRun = append(res.CommandsRun, bin+" "+strings.Join(args, " "))
		out, runErr := runner(ctx, bin, args...)
		if trimmed := strings.TrimSpace(string(out)); trimmed != "" {
			outputs = append(outputs, trimmed)
		}
		if runErr != nil {
			return fmt.Errorf("%s %s failed: %w (%s)", bin, strings.Join(args, " "), runErr, strings.TrimSpace(string(out)))
		}
		return nil
	}

	switch mgr {
	case ManagerSystemd:
		if err := run("systemctl", systemdArgs(spec.UserScope, "daemon-reload")...); err != nil {
			return res, err
		}
		if opts.Enable {
			if err := run("systemctl", systemdArgs(spec.UserScope, "enable", spec.Name+".service")...); err != nil {
				return res, err
			}
		}
		if opts.Start {
			if err := run("systemctl", systemdArgs(spec.UserScope, "start", spec.Name+".service")...); err != nil {
				return res, err
			}
		}
	case ManagerLaunchd:
		if opts.Start {
			if err := startLaunchd(ctx, runner, spec, uid, &res.CommandsRun, &outputs); err != nil {
				return res, err
			}
		}
	case ManagerOpenRC:
		if opts.Enable {
			if err := run("rc-update", "add", spec.Name, "default"); err != nil {
				return res, err
			}
		}
		if opts.Start {
			if err := run("rc-service", spec.Name, "start"); err != nil {
				return res, err
			}
		}
	case ManagerSupervisor:
		if err := run("supervisorctl", "reread"); err != nil {
			return res, err
		}
		if err := run("supervisorctl", "update"); err != nil {
			return res, err
		}
		if opts.Start {
			if err := run("supervisorctl", "start", spec.Name); err != nil {
				return res, err
			}
		}
	}

	res.Output = strings.Join(outputs, "\n")
	return res, nil
}

// Control performs start, stop, restart, or status on a registered service.
func Control(ctx context.Context, action string, opts Options) (ActionResult, error) {
	mgr, home, uid, runner, err := opts.prepare()
	if err != nil {
		return ActionResult{}, err
	}
	spec := opts.Spec
	if strings.TrimSpace(spec.Name) == "" {
		spec.Name = "gymbag"
	}
	if !isSafeIdentifier(spec.Name) {
		return ActionResult{}, fmt.Errorf("invalid service name %q", spec.Name)
	}
	if strings.TrimSpace(spec.Label) == "" {
		spec.Label = "io.gymbag." + spec.Name
	}
	if strings.TrimSpace(spec.UnitPath) == "" {
		uPath, err := DefaultUnitPath(mgr, spec.Name, spec.Label, spec.UserScope, home)
		if err != nil {
			return ActionResult{}, err
		}
		spec.UnitPath = uPath
	}

	res := ActionResult{
		Manager:  mgr,
		UnitPath: spec.UnitPath,
	}
	var outputs []string
	run := func(bin string, args ...string) error {
		res.CommandsRun = append(res.CommandsRun, bin+" "+strings.Join(args, " "))
		out, runErr := runner(ctx, bin, args...)
		if trimmed := strings.TrimSpace(string(out)); trimmed != "" {
			outputs = append(outputs, trimmed)
		}
		if runErr != nil {
			return fmt.Errorf("%s %s failed: %w (%s)", bin, strings.Join(args, " "), runErr, strings.TrimSpace(string(out)))
		}
		return nil
	}

	switch mgr {
	case ManagerSystemd:
		switch action {
		case "start", "stop", "restart", "status":
			err = run("systemctl", systemdArgs(spec.UserScope, action, spec.Name+".service")...)
		default:
			return res, fmt.Errorf("unsupported service action %q", action)
		}
	case ManagerLaunchd:
		domain := launchdDomain(spec.UserScope, uid)
		target := domain + "/" + spec.Label
		switch action {
		case "start":
			err = startLaunchd(ctx, runner, spec, uid, &res.CommandsRun, &outputs)
		case "stop":
			err = run("launchctl", "bootout", target)
		case "restart":
			err = run("launchctl", "kickstart", "-k", target)
			if err != nil {
				// If not yet bootstrapped, fall back to bootstrap.
				outputs = nil
				err = startLaunchd(ctx, runner, spec, uid, &res.CommandsRun, &outputs)
			}
		case "status":
			err = run("launchctl", "print", target)
		default:
			return res, fmt.Errorf("unsupported service action %q", action)
		}
	case ManagerOpenRC:
		switch action {
		case "start", "stop", "restart", "status":
			err = run("rc-service", spec.Name, action)
		default:
			return res, fmt.Errorf("unsupported service action %q", action)
		}
	case ManagerSupervisor:
		switch action {
		case "start", "stop", "restart", "status":
			err = run("supervisorctl", action, spec.Name)
		default:
			return res, fmt.Errorf("unsupported service action %q", action)
		}
	default:
		return res, fmt.Errorf("unsupported service manager %q", mgr)
	}

	res.Output = strings.Join(outputs, "\n")
	return res, err
}

// Uninstall stops/disables the service (best-effort) and deletes the service definition file.
func Uninstall(ctx context.Context, opts Options) (ActionResult, error) {
	mgr, home, uid, runner, err := opts.prepare()
	if err != nil {
		return ActionResult{}, err
	}
	spec := opts.Spec
	if strings.TrimSpace(spec.Name) == "" {
		spec.Name = "gymbag"
	}
	if !isSafeIdentifier(spec.Name) {
		return ActionResult{}, fmt.Errorf("invalid service name %q", spec.Name)
	}
	if strings.TrimSpace(spec.Label) == "" {
		spec.Label = "io.gymbag." + spec.Name
	}
	if strings.TrimSpace(spec.UnitPath) == "" {
		uPath, err := DefaultUnitPath(mgr, spec.Name, spec.Label, spec.UserScope, home)
		if err != nil {
			return ActionResult{}, err
		}
		spec.UnitPath = uPath
	}

	res := ActionResult{
		Manager:  mgr,
		UnitPath: spec.UnitPath,
	}
	var outputs []string
	bestEffort := func(bin string, args ...string) {
		res.CommandsRun = append(res.CommandsRun, bin+" "+strings.Join(args, " "))
		out, _ := runner(ctx, bin, args...)
		if trimmed := strings.TrimSpace(string(out)); trimmed != "" {
			outputs = append(outputs, trimmed)
		}
	}

	if !opts.NoReload {
		switch mgr {
		case ManagerSystemd:
			bestEffort("systemctl", systemdArgs(spec.UserScope, "stop", spec.Name+".service")...)
			bestEffort("systemctl", systemdArgs(spec.UserScope, "disable", spec.Name+".service")...)
		case ManagerLaunchd:
			bestEffort("launchctl", "bootout", launchdDomain(spec.UserScope, uid)+"/"+spec.Label)
		case ManagerOpenRC:
			bestEffort("rc-service", spec.Name, "stop")
			bestEffort("rc-update", "del", spec.Name, "default")
		case ManagerSupervisor:
			bestEffort("supervisorctl", "stop", spec.Name)
		}
	}

	if rmErr := os.Remove(spec.UnitPath); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
		return res, wrapPermErr(rmErr, spec.UnitPath, mgr, spec.UserScope)
	}

	if !opts.NoReload {
		switch mgr {
		case ManagerSystemd:
			bestEffort("systemctl", systemdArgs(spec.UserScope, "daemon-reload")...)
		case ManagerSupervisor:
			bestEffort("supervisorctl", "reread")
			bestEffort("supervisorctl", "update")
		}
	}

	res.Output = strings.Join(outputs, "\n")
	return res, nil
}

func startLaunchd(ctx context.Context, runner CommandRunner, spec Spec, uid int, cmds *[]string, outputs *[]string) error {
	domain := launchdDomain(spec.UserScope, uid)
	target := domain + "/" + spec.Label

	*cmds = append(*cmds, "launchctl print "+target)
	if _, err := runner(ctx, "launchctl", "print", target); err == nil {
		*cmds = append(*cmds, "launchctl kickstart -k "+target)
		out, kErr := runner(ctx, "launchctl", "kickstart", "-k", target)
		if trimmed := strings.TrimSpace(string(out)); trimmed != "" {
			*outputs = append(*outputs, trimmed)
		}
		if kErr != nil {
			return fmt.Errorf("launchctl kickstart -k %s failed: %w (%s)", target, kErr, strings.TrimSpace(string(out)))
		}
		return nil
	}

	*cmds = append(*cmds, "launchctl bootstrap "+domain+" "+spec.UnitPath)
	out, bErr := runner(ctx, "launchctl", "bootstrap", domain, spec.UnitPath)
	if trimmed := strings.TrimSpace(string(out)); trimmed != "" {
		*outputs = append(*outputs, trimmed)
	}
	if bErr != nil {
		return fmt.Errorf("launchctl bootstrap %s %s failed: %w (%s)", domain, spec.UnitPath, bErr, strings.TrimSpace(string(out)))
	}
	return nil
}

func systemdArgs(userScope bool, rest ...string) []string {
	if userScope {
		return append([]string{"--user"}, rest...)
	}
	return rest
}

func launchdDomain(userScope bool, uid int) string {
	if userScope {
		return "gui/" + strconv.Itoa(uid)
	}
	return "system"
}

func wrapPermErr(err error, path string, mgr ManagerType, userScope bool) error {
	if errors.Is(err, os.ErrPermission) {
		if !userScope && (mgr == ManagerSystemd || mgr == ManagerLaunchd || mgr == ManagerSupervisor) {
			return fmt.Errorf("permission denied accessing %s (run with sudo for system-wide service, or pass --user for user-level service): %w", path, err)
		}
		return fmt.Errorf("permission denied accessing %s (try running with sudo): %w", path, err)
	}
	return err
}
