package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gymbag/internal/config"
	"gymbag/internal/service"
)

// serviceCommandRunner is nil in production (uses real OS commands) and can be overridden in tests.
var serviceCommandRunner service.CommandRunner

func runServiceCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printServiceUsage(stdout)
		return 0
	}
	sub := strings.ToLower(strings.TrimSpace(args[0]))
	switch sub {
	case "help", "-h", "--help":
		printServiceUsage(stdout)
		return 0
	case "render", "print", "generate":
		return runServiceAction("render", args[1:], stdout, stderr)
	case "install", "register":
		return runServiceAction("install", args[1:], stdout, stderr)
	case "uninstall", "remove", "unregister":
		return runServiceAction("uninstall", args[1:], stdout, stderr)
	case "start", "stop", "restart", "status":
		return runServiceAction(sub, args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag service: unknown subcommand %q\n\n", args[0])
		printServiceUsage(stderr)
		return 2
	}
}

func runServiceAction(action string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gymbag service "+action, flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		mgrType      string
		name         string
		label        string
		cfgPath      string
		binPath      string
		workdir      string
		envFile      string
		logDir       string
		unitPath     string
		serviceUser  string
		serviceGroup string
		userScope    bool
		systemScope  bool
		watch        bool
		enable       bool
		startNow     bool
		noReload     bool
		force        bool
	)

	fs.StringVar(&mgrType, "t", "auto", "Service manager type: auto, systemd, launchd, openrc, supervisor, freebsd-rc, openbsd-rc, or netbsd-rc")
	fs.StringVar(&mgrType, "type", "auto", "Service manager type: auto, systemd, launchd, openrc, supervisor, freebsd-rc, openbsd-rc, or netbsd-rc")
	fs.StringVar(&name, "n", "gymbag", "Service name")
	fs.StringVar(&name, "name", "gymbag", "Service name")
	fs.StringVar(&label, "label", "", "Launchd service label (default io.gymbag.<name>)")
	fs.StringVar(&cfgPath, "c", "", "Path to Gymbagfile, Caddyfile, or JSON config")
	fs.StringVar(&cfgPath, "config", "", "Path to Gymbagfile, Caddyfile, or JSON config")
	fs.StringVar(&binPath, "bin", "", "Path to gymbag binary (default current executable)")
	fs.StringVar(&workdir, "workdir", "", "Working directory for the service (default config file directory)")
	fs.StringVar(&envFile, "env-file", "", "Optional .env file path")
	fs.StringVar(&logDir, "log-dir", "", "Log directory for launchd/openrc/supervisor/bsd")
	fs.StringVar(&unitPath, "unit-path", "", "Override output path for service unit/plist/script file")
	fs.StringVar(&unitPath, "o", "", "Alias for --unit-path")
	fs.StringVar(&unitPath, "output", "", "Alias for --unit-path")
	fs.StringVar(&serviceUser, "service-user", "", "Run-as user for system-level service")
	fs.StringVar(&serviceGroup, "service-group", "", "Run-as group for system-level service")
	fs.BoolVar(&userScope, "user", false, "Use user-level service scope (systemd --user / launchd LaunchAgents)")
	fs.BoolVar(&systemScope, "system", false, "Use system-wide service scope")
	fs.BoolVar(&watch, "watch", true, "Pass --watch to gymbag run for automatic config reload")
	fs.BoolVar(&enable, "enable", true, "Enable service on boot during install")
	fs.BoolVar(&startNow, "start", false, "Start service immediately after install")
	fs.BoolVar(&startNow, "now", false, "Alias for --start")
	fs.BoolVar(&noReload, "no-reload", false, "Write/remove unit file without invoking systemctl/launchctl/rc-update/supervisorctl/service/rcctl")
	fs.BoolVar(&force, "f", false, "Overwrite existing service file")
	fs.BoolVar(&force, "force", false, "Overwrite existing service file")
	fs.BoolVar(&force, "overwrite", false, "Overwrite existing service file")

	if err := parseInterspersedFlags(fs, args); err != nil {
		if err == flag.ErrHelp {
			printServiceUsage(stdout)
			return 0
		}
		return 2
	}
	if userScope && systemScope {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag service %s: --user and --system cannot be used together\n", action)
		return 2
	}

	mgr, err := service.ParseManagerType(mgrType)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR gymbag service %s: %v\n", action, err)
		return 2
	}
	if mgr == service.ManagerAuto {
		detected, dErr := service.DetectManager()
		if dErr != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag service %s: %v\n", action, dErr)
			return 1
		}
		mgr = detected
	}

	resolvedUserScope := resolveUserScope(mgr, userScope, systemScope)

	spec := service.Spec{
		Name:       name,
		Label:      label,
		BinaryPath: binPath,
		ConfigPath: cfgPath,
		WorkingDir: workdir,
		EnvFile:    envFile,
		LogDir:     logDir,
		UnitPath:   unitPath,
		User:       serviceUser,
		Group:      serviceGroup,
		UserScope:  resolvedUserScope,
		Watch:      watch,
	}

	if action == "render" || action == "install" {
		resolvedCfg := strings.TrimSpace(cfgPath)
		if resolvedCfg == "" {
			resolvedCfg = resolveWatchedPath("")
		}
		if action == "install" && resolvedCfg == "" {
			_, _ = fmt.Fprintln(stderr, `ERROR gymbag service install: no config file found (Gymbagfile or Caddyfile); run "gymbag init" first or pass --config <path>`)
			return 1
		}
		if resolvedCfg != "" {
			cfg, pErr := config.ParseFile(resolvedCfg)
			if pErr != nil {
				_, _ = fmt.Fprintf(stderr, "ERROR gymbag service %s: invalid config %s: %v\n", action, resolvedCfg, pErr)
				return 1
			}
			spec.ConfigPath = resolvedCfg
			spec.PortsPrivileged = (cfg.HTTPEnabled() && (cfg.HTTPPort < 1024 || cfg.HTTPSPort < 1024)) ||
				(cfg.SSHEnabled() && cfg.SSH.Port < 1024) ||
				(cfg.MinecraftEnabled() && cfg.Minecraft.Port < 1024) ||
				(cfg.NeedsVerifyServer() && cfg.Verify.EffectivePort() < 1024)
		} else {
			spec.ConfigPath = "Gymbagfile"
			spec.PortsPrivileged = true
		}

		if spec.EnvFile == "" {
			wd := spec.WorkingDir
			if wd == "" && spec.ConfigPath != "" {
				wd = filepath.Dir(spec.ConfigPath)
			}
			if wd != "" {
				candidateEnv := filepath.Join(wd, ".env")
				if info, sErr := os.Stat(candidateEnv); sErr == nil && !info.IsDir() {
					spec.EnvFile = candidateEnv
				}
			}
		}
	}

	if action == "render" {
		home, _ := os.UserHomeDir()
		rendered, rErr := service.RenderUnit(mgr, spec, home)
		if rErr != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag service render: %v\n", rErr)
			return 1
		}
		if unitPath != "" {
			if dir := filepath.Dir(rendered.UnitPath); dir != "" && dir != "." {
				if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
					_, _ = fmt.Fprintf(stderr, "ERROR gymbag service render: %v\n", mkErr)
					return 1
				}
			}
			if wErr := os.WriteFile(rendered.UnitPath, []byte(rendered.Content), rendered.FileMode); wErr != nil {
				_, _ = fmt.Fprintf(stderr, "ERROR gymbag service render: %v\n", wErr)
				return 1
			}
			_, _ = fmt.Fprintf(stdout, "Wrote %s service file to %s\n", rendered.Manager, rendered.UnitPath)
			return 0
		}
		_, _ = fmt.Fprintf(stdout, "# Target path: %s\n", rendered.UnitPath)
		_, _ = io.WriteString(stdout, rendered.Content)
		return 0
	}

	opts := service.Options{
		Manager:   mgr,
		Spec:      spec,
		Overwrite: force,
		Enable:    enable,
		Start:     startNow,
		NoReload:  noReload,
		Runner:    serviceCommandRunner,
	}

	ctx := context.Background()
	switch action {
	case "install":
		res, iErr := service.Install(ctx, opts)
		if iErr != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag service install: %v\n", iErr)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "Installed %s service (%s) at %s\n", res.Manager, name, res.UnitPath)
		if res.Output != "" {
			_, _ = fmt.Fprintln(stdout, res.Output)
		}
		if !startNow {
			_, _ = fmt.Fprintf(stdout, "\nNext step (start service):\n  %s\n", formatServiceCommandHint("start", name, resolvedUserScope))
		}
		return 0

	case "uninstall":
		res, uErr := service.Uninstall(ctx, opts)
		if uErr != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag service uninstall: %v\n", uErr)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "Uninstalled %s service (%s) from %s\n", res.Manager, name, res.UnitPath)
		return 0

	case "start", "stop", "restart", "status":
		res, cErr := service.Control(ctx, action, opts)
		if res.Output != "" {
			_, _ = fmt.Fprintln(stdout, res.Output)
		}
		if cErr != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR gymbag service %s: %v\n", action, cErr)
			return 1
		}
		if res.Output == "" && action != "status" {
			_, _ = fmt.Fprintf(stdout, "Service %s (%s): %s OK\n", name, res.Manager, action)
		}
		return 0
	}

	return 1
}

func resolveUserScope(mgr service.ManagerType, explicitUser, explicitSystem bool) bool {
	if explicitUser {
		return true
	}
	if explicitSystem {
		return false
	}
	// OpenRC and BSD rc.d systems have no standard per-user daemon scope.
	if mgr == service.ManagerOpenRC ||
		mgr == service.ManagerFreeBSDRC ||
		mgr == service.ManagerOpenBSDRC ||
		mgr == service.ManagerNetBSDRC {
		return false
	}
	if runtime.GOOS == "windows" {
		return false
	}
	return os.Geteuid() != 0
}

func formatServiceCommandHint(action, name string, userScope bool) string {
	var parts []string
	parts = append(parts, "gymbag", "service", action)
	if name != "" && name != "gymbag" {
		parts = append(parts, "--name", name)
	}
	if userScope {
		parts = append(parts, "--user")
	} else if os.Geteuid() != 0 {
		parts = append(parts, "--system")
	}
	return strings.Join(parts, " ")
}

func printServiceUsage(w io.Writer) {
	_, _ = fmt.Fprintf(w, `Usage:
  gymbag service <command> [flags]

Commands:
  install     Generate and register a system service (systemd, launchd, openrc, supervisor, freebsd-rc, openbsd-rc, netbsd-rc)
  start       Start the registered service
  stop        Stop the registered service
  restart     Restart the registered service
  status      Show service status
  uninstall   Stop, disable, and remove the service definition
  render      Print (or write with --output) the service unit/plist/script without registering

Examples:
  gymbag service install --config Gymbagfile             # Register user/system service (auto-detected)
  gymbag service start                                   # Start the registered service
  gymbag service install --type systemd --system --start # Install & start system-wide systemd unit
  gymbag service render --type freebsd-rc                # Preview a FreeBSD rc.d script on any OS

Flags:
  -t, --type <mgr>            Service manager: auto, systemd, launchd, openrc, supervisor, freebsd-rc, openbsd-rc, netbsd-rc
  -n, --name <name>           Service name (default gymbag)
      --label <id>            launchd label (default io.gymbag.<name>)
  -c, --config <path>         Path to Gymbagfile, Caddyfile, or JSON config
      --bin <path>            Path to gymbag binary (default current executable)
      --workdir <dir>         Working directory (default config file directory)
      --env-file <path>       Optional .env file path
      --log-dir <dir>         Log directory (launchd, openrc, supervisor, bsd)
  -o, --output <path>         Override target unit/plist/script file path
      --service-user <user>   Run-as user for system-level service
      --service-group <grp>   Run-as group for system-level service
      --user                  Register/control user-level service (systemd --user / LaunchAgents)
      --system                Register/control system-wide service
      --watch                 Enable config hot-reload via --watch (default true)
      --enable                Enable service on boot during install (default true)
      --start                 Start service immediately after install
      --no-reload             Skip daemon-reload / enable / start commands
  -f, --force                 Overwrite existing service file
`)
}
