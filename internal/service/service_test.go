package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectManagerWith(t *testing.T) {
	noStat := func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	noLook := func(string) (string, error) { return "", errors.New("not found") }

	if mgr, err := detectManagerWith("darwin", noStat, noLook); err != nil || mgr != ManagerLaunchd {
		t.Fatalf("darwin detection = (%q, %v)", mgr, err)
	}

	systemdStat := func(p string) (os.FileInfo, error) {
		if p == "/run/systemd/system" {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}
	if mgr, err := detectManagerWith("linux", systemdStat, noLook); err != nil || mgr != ManagerSystemd {
		t.Fatalf("linux systemd detection = (%q, %v)", mgr, err)
	}

	openrcLook := func(bin string) (string, error) {
		if bin == "rc-service" {
			return "/sbin/rc-service", nil
		}
		return "", errors.New("not found")
	}
	if mgr, err := detectManagerWith("linux", noStat, openrcLook); err != nil || mgr != ManagerOpenRC {
		t.Fatalf("linux openrc detection = (%q, %v)", mgr, err)
	}

	if _, err := detectManagerWith("linux", noStat, noLook); err == nil {
		t.Fatalf("expected error when no init system detected on linux")
	}

	freebsdStat := func(p string) (os.FileInfo, error) {
		if p == "/etc/rc.subr" {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}
	if mgr, err := detectManagerWith("freebsd", freebsdStat, noLook); err != nil || mgr != ManagerFreeBSDRC {
		t.Fatalf("freebsd detection = (%q, %v)", mgr, err)
	}
	if mgr, err := detectManagerWith("dragonfly", freebsdStat, noLook); err != nil || mgr != ManagerFreeBSDRC {
		t.Fatalf("dragonfly detection = (%q, %v)", mgr, err)
	}
	if mgr, err := detectManagerWith("netbsd", freebsdStat, noLook); err != nil || mgr != ManagerNetBSDRC {
		t.Fatalf("netbsd detection = (%q, %v)", mgr, err)
	}
	openbsdLook := func(bin string) (string, error) {
		if bin == "rcctl" {
			return "/usr/sbin/rcctl", nil
		}
		return "", errors.New("not found")
	}
	if mgr, err := detectManagerWith("openbsd", noStat, openbsdLook); err != nil || mgr != ManagerOpenBSDRC {
		t.Fatalf("openbsd detection = (%q, %v)", mgr, err)
	}

	for raw, want := range map[string]ManagerType{
		"freebsd":    ManagerFreeBSDRC,
		"freebsd-rc": ManagerFreeBSDRC,
		"bsd":        ManagerFreeBSDRC,
		"dragonfly":  ManagerFreeBSDRC,
		"openbsd":    ManagerOpenBSDRC,
		"openbsd-rc": ManagerOpenBSDRC,
		"rcctl":      ManagerOpenBSDRC,
		"netbsd":     ManagerNetBSDRC,
		"netbsd-rc":  ManagerNetBSDRC,
	} {
		got, err := ParseManagerType(raw)
		if err != nil || got != want {
			t.Fatalf("ParseManagerType(%q) = (%q, %v), want %q", raw, got, err, want)
		}
	}
}

func TestRenderUnitsAllManagers(t *testing.T) {
	home := "/home/alice"
	baseSpec := Spec{
		Name:            "gymbag",
		BinaryPath:      "/usr/local/bin/gymbag",
		ConfigPath:      "/etc/gymbag/Gymbagfile",
		WorkingDir:      "/etc/gymbag",
		Watch:           true,
		PortsPrivileged: true,
	}

	// 1. systemd system unit with non-root user + CAP_NET_BIND_SERVICE
	sysSpec := baseSpec
	sysSpec.User = "gymbag"
	sysSpec.EnvFile = "/etc/gymbag/.env"
	sysUnit, err := RenderUnit(ManagerSystemd, sysSpec, home)
	if err != nil {
		t.Fatalf("RenderUnit systemd system failed: %v", err)
	}
	if sysUnit.UnitPath != "/etc/systemd/system/gymbag.service" {
		t.Fatalf("unexpected systemd system unit path: %s", sysUnit.UnitPath)
	}
	for _, want := range []string{
		"User=gymbag",
		"WorkingDirectory=/etc/gymbag",
		"EnvironmentFile=-/etc/gymbag/.env",
		"ExecStart=/usr/local/bin/gymbag run --config /etc/gymbag/Gymbagfile --watch",
		"AmbientCapabilities=CAP_NET_BIND_SERVICE",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(sysUnit.Content, want) {
			t.Fatalf("systemd unit missing %q:\n%s", want, sysUnit.Content)
		}
	}

	// 2. systemd --user unit
	userSpec := baseSpec
	userSpec.UserScope = true
	userUnit, err := RenderUnit(ManagerSystemd, userSpec, home)
	if err != nil {
		t.Fatalf("RenderUnit systemd user failed: %v", err)
	}
	if userUnit.UnitPath != "/home/alice/.config/systemd/user/gymbag.service" {
		t.Fatalf("unexpected systemd user path: %s", userUnit.UnitPath)
	}
	if strings.Contains(userUnit.Content, "AmbientCapabilities") || !strings.Contains(userUnit.Content, "WantedBy=default.target") {
		t.Fatalf("unexpected systemd user unit content:\n%s", userUnit.Content)
	}

	// 3. launchd user plist (with XML-sensitive chars)
	ldSpec := baseSpec
	ldSpec.UserScope = true
	ldSpec.Description = "Gymbag & Proxy"
	ldSpec.ConfigPath = "/Users/alice/My Apps/Gymbagfile"
	ldUnit, err := RenderUnit(ManagerLaunchd, ldSpec, "/Users/alice")
	if err != nil {
		t.Fatalf("RenderUnit launchd failed: %v", err)
	}
	if ldUnit.UnitPath != "/Users/alice/Library/LaunchAgents/io.gymbag.gymbag.plist" {
		t.Fatalf("unexpected launchd path: %s", ldUnit.UnitPath)
	}
	if !strings.Contains(ldUnit.Content, "<string>io.gymbag.gymbag</string>") ||
		!strings.Contains(ldUnit.Content, "<string>/Users/alice/My Apps/Gymbagfile</string>") ||
		!strings.Contains(ldUnit.Content, "<key>SuccessfulExit</key>") {
		t.Fatalf("unexpected launchd plist:\n%s", ldUnit.Content)
	}

	// 4. OpenRC script
	orcSpec := baseSpec
	orcSpec.User = "gymbag"
	orcUnit, err := RenderUnit(ManagerOpenRC, orcSpec, home)
	if err != nil {
		t.Fatalf("RenderUnit openrc failed: %v", err)
	}
	if orcUnit.FileMode != 0o755 || orcUnit.UnitPath != "/etc/init.d/gymbag" {
		t.Fatalf("unexpected openrc metadata: mode=%v path=%s", orcUnit.FileMode, orcUnit.UnitPath)
	}
	if !strings.Contains(orcUnit.Content, "#!/sbin/openrc-run") ||
		!strings.Contains(orcUnit.Content, `capabilities="^cap_net_bind_service"`) {
		t.Fatalf("unexpected openrc script:\n%s", orcUnit.Content)
	}

	// 5. Supervisor conf
	supUnit, err := RenderUnit(ManagerSupervisor, baseSpec, home)
	if err != nil {
		t.Fatalf("RenderUnit supervisor failed: %v", err)
	}
	if supUnit.UnitPath != "/etc/supervisor/conf.d/gymbag.conf" || !strings.Contains(supUnit.Content, "[program:gymbag]") {
		t.Fatalf("unexpected supervisor unit: path=%s\n%s", supUnit.UnitPath, supUnit.Content)
	}

	// 6. FreeBSD rc.d script (including sanitized rcvar for hyphenated name)
	fbsdSpec := baseSpec
	fbsdSpec.Name = "gymbag-edge"
	fbsdSpec.User = "www"
	fbsdSpec.EnvFile = "/usr/local/etc/gymbag/.env"
	fbsdUnit, err := RenderUnit(ManagerFreeBSDRC, fbsdSpec, home)
	if err != nil {
		t.Fatalf("RenderUnit freebsd-rc failed: %v", err)
	}
	if fbsdUnit.FileMode != 0o755 || fbsdUnit.UnitPath != "/usr/local/etc/rc.d/gymbag-edge" {
		t.Fatalf("unexpected freebsd unit metadata: mode=%v path=%s", fbsdUnit.FileMode, fbsdUnit.UnitPath)
	}
	for _, want := range []string{
		`name="gymbag_edge"`,
		`: ${gymbag_edge_enable:="NO"}`,
		`command="/usr/sbin/daemon"`,
		`-u 'www'`,
		`gymbag_edge_prestart()`,
		`run_rc_command "$1"`,
	} {
		if !strings.Contains(fbsdUnit.Content, want) {
			t.Fatalf("freebsd rc.d script missing %q:\n%s", want, fbsdUnit.Content)
		}
	}

	// 7. OpenBSD rc.d script
	obsdSpec := baseSpec
	obsdSpec.User = "_gymbag"
	obsdSpec.EnvFile = "/etc/gymbag/.env"
	obsdUnit, err := RenderUnit(ManagerOpenBSDRC, obsdSpec, home)
	if err != nil {
		t.Fatalf("RenderUnit openbsd-rc failed: %v", err)
	}
	if obsdUnit.FileMode != 0o755 || obsdUnit.UnitPath != "/etc/rc.d/gymbag" {
		t.Fatalf("unexpected openbsd unit metadata: mode=%v path=%s", obsdUnit.FileMode, obsdUnit.UnitPath)
	}
	for _, want := range []string{
		`#!/bin/ksh`,
		`daemon='/usr/local/bin/gymbag'`,
		`daemon_user='_gymbag'`,
		`. /etc/rc.d/rc.subr`,
		`rc_bg=YES`,
		`rc_pre()`,
		`rc_cmd $1`,
	} {
		if !strings.Contains(obsdUnit.Content, want) {
			t.Fatalf("openbsd rc.d script missing %q:\n%s", want, obsdUnit.Content)
		}
	}

	// 8. NetBSD rc.d script
	nbsdSpec := baseSpec
	nbsdSpec.Name = "gymbag-gw"
	nbsdSpec.User = "gymbag"
	nbsdUnit, err := RenderUnit(ManagerNetBSDRC, nbsdSpec, home)
	if err != nil {
		t.Fatalf("RenderUnit netbsd-rc failed: %v", err)
	}
	if nbsdUnit.FileMode != 0o755 || nbsdUnit.UnitPath != "/etc/rc.d/gymbag-gw" {
		t.Fatalf("unexpected netbsd unit metadata: mode=%v path=%s", nbsdUnit.FileMode, nbsdUnit.UnitPath)
	}
	for _, want := range []string{
		`$_rc_subr_loaded . /etc/rc.subr`,
		`name="gymbag_gw"`,
		`gymbag_gw_start()`,
		`su -m 'gymbag' -c`,
		`run_rc_command "$1"`,
	} {
		if !strings.Contains(nbsdUnit.Content, want) {
			t.Fatalf("netbsd rc.d script missing %q:\n%s", want, nbsdUnit.Content)
		}
	}
}

func TestRenderUnitRejectsUnsafeNamesAndPaths(t *testing.T) {
	bad := Spec{
		Name:       "../evil",
		BinaryPath: "/usr/bin/gymbag",
	}
	if _, err := RenderUnit(ManagerSystemd, bad, "/home/u"); err == nil {
		t.Fatalf("expected error for unsafe service name")
	}

	badPath := Spec{
		Name:       "gymbag",
		BinaryPath: "/usr/bin/gymbag\nExecStart=/bin/sh",
	}
	if _, err := RenderUnit(ManagerSystemd, badPath, "/home/u"); err == nil {
		t.Fatalf("expected error for newline in binary path")
	}
}

func TestInstallControlAndUninstallWithMockRunner(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	binPath := filepath.Join(tmp, "gymbag")
	cfgPath := filepath.Join(tmp, "Gymbagfile")
	if err := os.WriteFile(binPath, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte("localhost {\n    reverse_proxy :3000\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var calls []string
	runner := func(_ context.Context, name string, args ...string) ([]byte, error) {
		line := name + " " + strings.Join(args, " ")
		calls = append(calls, line)
		if name == "launchctl" && len(args) > 0 && args[0] == "print" {
			return []byte("not loaded"), errors.New("exit 113")
		}
		return []byte("ok"), nil
	}

	// Systemd user install + start + stop + uninstall
	sysOpts := Options{
		Manager: ManagerSystemd,
		HomeDir: home,
		UID:     1000,
		Enable:  true,
		Start:   true,
		Runner:  runner,
		Spec: Spec{
			Name:       "gymbag-test",
			BinaryPath: binPath,
			ConfigPath: cfgPath,
			UserScope:  true,
			Watch:      true,
		},
	}

	res, err := Install(context.Background(), sysOpts)
	if err != nil {
		t.Fatalf("Install systemd failed: %v", err)
	}
	if _, err := os.Stat(res.UnitPath); err != nil {
		t.Fatalf("unit file not created at %s: %v", res.UnitPath, err)
	}
	// Attempting second install without Overwrite should fail.
	if _, err := Install(context.Background(), sysOpts); err == nil {
		t.Fatalf("expected error on duplicate install without Overwrite")
	}

	wantCmds := []string{
		"systemctl --user daemon-reload",
		"systemctl --user enable gymbag-test.service",
		"systemctl --user start gymbag-test.service",
	}
	if strings.Join(calls, "|") != strings.Join(wantCmds, "|") {
		t.Fatalf("unexpected systemd install commands: got %v, want %v", calls, wantCmds)
	}

	calls = nil
	if _, err := Control(context.Background(), "restart", sysOpts); err != nil {
		t.Fatalf("Control restart failed: %v", err)
	}
	if len(calls) != 1 || calls[0] != "systemctl --user restart gymbag-test.service" {
		t.Fatalf("unexpected restart command: %v", calls)
	}

	calls = nil
	if _, err := Uninstall(context.Background(), sysOpts); err != nil {
		t.Fatalf("Uninstall systemd failed: %v", err)
	}
	if _, err := os.Stat(res.UnitPath); !os.IsNotExist(err) {
		t.Fatalf("expected unit file removed after uninstall, stat err=%v", err)
	}

	// Launchd user install + start (bootstrap) + stop (bootout) + uninstall
	calls = nil
	ldOpts := Options{
		Manager: ManagerLaunchd,
		HomeDir: home,
		UID:     501,
		Start:   true,
		Runner:  runner,
		Spec: Spec{
			Name:       "gymbag-ld",
			BinaryPath: binPath,
			ConfigPath: cfgPath,
			UserScope:  true,
			Watch:      true,
		},
	}
	ldRes, err := Install(context.Background(), ldOpts)
	if err != nil {
		t.Fatalf("Install launchd failed: %v", err)
	}
	if len(calls) != 2 ||
		calls[0] != "launchctl print gui/501/io.gymbag.gymbag-ld" ||
		!strings.HasPrefix(calls[1], "launchctl bootstrap gui/501 ") {
		t.Fatalf("unexpected launchd start calls: %v", calls)
	}

	calls = nil
	if _, err := Control(context.Background(), "stop", ldOpts); err != nil {
		t.Fatalf("Control launchd stop failed: %v", err)
	}
	if len(calls) != 1 || calls[0] != "launchctl bootout gui/501/io.gymbag.gymbag-ld" {
		t.Fatalf("unexpected launchd stop calls: %v", calls)
	}
	if _, err := Uninstall(context.Background(), ldOpts); err != nil {
		t.Fatalf("Uninstall launchd failed: %v", err)
	}
	if _, err := os.Stat(ldRes.UnitPath); !os.IsNotExist(err) {
		t.Fatalf("expected plist removed after uninstall")
	}

	// FreeBSD rc.d install + control + uninstall
	calls = nil
	fbsdUnitPath := filepath.Join(tmp, "usr", "local", "etc", "rc.d", "gymbag-bsd")
	fbsdOpts := Options{
		Manager: ManagerFreeBSDRC,
		Enable:  true,
		Start:   true,
		Runner:  runner,
		Spec: Spec{
			Name:       "gymbag-bsd",
			BinaryPath: binPath,
			ConfigPath: cfgPath,
			UnitPath:   fbsdUnitPath,
		},
	}
	if _, err := Install(context.Background(), fbsdOpts); err != nil {
		t.Fatalf("Install freebsd-rc failed: %v", err)
	}
	wantFbsd := []string{
		"sysrc gymbag_bsd_enable=YES",
		"service gymbag-bsd start",
	}
	if strings.Join(calls, "|") != strings.Join(wantFbsd, "|") {
		t.Fatalf("unexpected freebsd install calls: got %v, want %v", calls, wantFbsd)
	}
	calls = nil
	if _, err := Control(context.Background(), "status", fbsdOpts); err != nil {
		t.Fatalf("Control freebsd-rc status failed: %v", err)
	}
	if len(calls) != 1 || calls[0] != "service gymbag-bsd status" {
		t.Fatalf("unexpected freebsd status calls: %v", calls)
	}
	calls = nil
	if _, err := Uninstall(context.Background(), fbsdOpts); err != nil {
		t.Fatalf("Uninstall freebsd-rc failed: %v", err)
	}
	if _, err := os.Stat(fbsdUnitPath); !os.IsNotExist(err) {
		t.Fatalf("expected freebsd rc script removed")
	}

	// OpenBSD rc.d install + status (rcctl check) + uninstall
	calls = nil
	obsdUnitPath := filepath.Join(tmp, "etc", "rc.d", "gymbag-obsd")
	obsdOpts := Options{
		Manager: ManagerOpenBSDRC,
		Enable:  true,
		Start:   true,
		Runner:  runner,
		Spec: Spec{
			Name:       "gymbag-obsd",
			BinaryPath: binPath,
			ConfigPath: cfgPath,
			UnitPath:   obsdUnitPath,
		},
	}
	if _, err := Install(context.Background(), obsdOpts); err != nil {
		t.Fatalf("Install openbsd-rc failed: %v", err)
	}
	wantObsd := []string{
		"rcctl enable gymbag-obsd",
		"rcctl start gymbag-obsd",
	}
	if strings.Join(calls, "|") != strings.Join(wantObsd, "|") {
		t.Fatalf("unexpected openbsd install calls: got %v, want %v", calls, wantObsd)
	}
	calls = nil
	if _, err := Control(context.Background(), "status", obsdOpts); err != nil {
		t.Fatalf("Control openbsd-rc status failed: %v", err)
	}
	if len(calls) != 1 || calls[0] != "rcctl check gymbag-obsd" {
		t.Fatalf("unexpected openbsd status calls: %v", calls)
	}
	if _, err := Uninstall(context.Background(), obsdOpts); err != nil {
		t.Fatalf("Uninstall openbsd-rc failed: %v", err)
	}

	// NetBSD rc.d install (creates rc.conf.d entry) + start + uninstall
	calls = nil
	nbsdUnitPath := filepath.Join(tmp, "netbsd-etc", "rc.d", "gymbag-nbsd")
	nbsdConfPath := filepath.Join(tmp, "netbsd-etc", "rc.conf.d", "gymbag_nbsd")
	nbsdOpts := Options{
		Manager: ManagerNetBSDRC,
		Enable:  true,
		Start:   true,
		Runner:  runner,
		Spec: Spec{
			Name:       "gymbag-nbsd",
			BinaryPath: binPath,
			ConfigPath: cfgPath,
			UnitPath:   nbsdUnitPath,
		},
	}
	if _, err := Install(context.Background(), nbsdOpts); err != nil {
		t.Fatalf("Install netbsd-rc failed: %v", err)
	}
	confData, err := os.ReadFile(nbsdConfPath)
	if err != nil || string(confData) != "gymbag_nbsd=YES\n" {
		t.Fatalf("unexpected netbsd rc.conf.d content (%v): %q", err, string(confData))
	}
	if len(calls) != 1 || calls[0] != "service gymbag-nbsd start" {
		t.Fatalf("unexpected netbsd start calls: %v", calls)
	}
	if _, err := Uninstall(context.Background(), nbsdOpts); err != nil {
		t.Fatalf("Uninstall netbsd-rc failed: %v", err)
	}
	if _, err := os.Stat(nbsdConfPath); !os.IsNotExist(err) {
		t.Fatalf("expected netbsd rc.conf.d file removed on uninstall")
	}
}
