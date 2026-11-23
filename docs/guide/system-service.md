# System Service Setup

`gymbag service` generates, installs, starts, stops, restarts, checks status for, and removes OS background services across seven init systems (Linux, macOS, and BSD):

| Manager | Target OS / Init | Default Unit Path |
| :--- | :--- | :--- |
| `systemd` | Linux (Ubuntu, Debian, Fedora, Arch, RHEL) | `/etc/systemd/system/<name>.service` (or `~/.config/systemd/user/<name>.service` with `--user`) |
| `launchd` | macOS (LaunchDaemons / LaunchAgents) | `/Library/LaunchDaemons/io.gymbag.<name>.plist` (or `~/Library/LaunchAgents/io.gymbag.<name>.plist` with `--user`) |
| `openrc` | Linux (Alpine, Gentoo, Artix) | `/etc/init.d/<name>` |
| `supervisor` | Unix / Containers (`supervisord`) | `/etc/supervisor/conf.d/<name>.conf` (or `~/.config/supervisor/conf.d/<name>.conf` with `--user`) |
| `freebsd-rc` | FreeBSD / DragonFly BSD (`rc.d` + `sysrc`) | `/usr/local/etc/rc.d/<name>` |
| `openbsd-rc` | OpenBSD (`rc.d` + `rcctl`) | `/etc/rc.d/<name>` |
| `netbsd-rc` | NetBSD (`rc.d` + `/etc/rc.conf`) | `/etc/rc.d/<name>` |

## Two-Command Service Workflow

Once you have a `gymbag.json` or `Gymbagfile`:

```bash
# 1. Register the service (auto-detects systemd / launchd / openrc / supervisor / BSD rc.d)
sudo ./gymbag service install --config /etc/gymbag/gymbag.json

# 2. Start it
sudo ./gymbag service start
```

Or generate the config and install the service in one step:

```bash
sudo ./gymbag init -d app.example.com -u :3000 -o /etc/gymbag/gymbag.json --service auto --system
sudo ./gymbag service start --system
```

::: tip `--watch` and `CAP_NET_BIND_SERVICE`
- `gymbag service install` defaults to `--watch=true`, so edits to the config reload routes, firewall rules, and SNI certs without restarting the daemon.
- System-wide `systemd` units with a non-root `--service-user` and privileged ports (`< 1024`, including `80`, `443`, `22`, or `verify_port < 1024`) get `AmbientCapabilities=CAP_NET_BIND_SERVICE`, `CapabilityBoundingSet=CAP_NET_BIND_SERVICE`, and `NoNewPrivileges=true`.
- OpenRC units receive `capabilities="^cap_net_bind_service"` when `--service-user` is non-root.
:::

## Service Subcommands

```bash
./gymbag service start
./gymbag service stop
./gymbag service restart
./gymbag service status
./gymbag service uninstall
```

### User vs System Scope

- Without `sudo` (`euid != 0`), `gymbag service` defaults to `--user` scope (`systemd --user`, `~/Library/LaunchAgents`) on `systemd`, `launchd`, and `supervisor`.
- As `root`, it defaults to `--system` scope (`/etc/systemd/system`, `/Library/LaunchDaemons`, `/etc/init.d`, `/usr/local/etc/rc.d`, `/etc/rc.d`).
- BSD `rc.d` managers (`freebsd-rc`, `openbsd-rc`, `netbsd-rc`) are system-wide services only.
- Pass `--user` or `--system` to pick explicitly.

## BSD (`FreeBSD`, `OpenBSD`, `NetBSD`, `DragonFly BSD`)

`gymbag service install` auto-detects the native `rc.d` manager on FreeBSD/DragonFly (`freebsd-rc`), OpenBSD (`openbsd-rc`), and NetBSD (`netbsd-rc`), and standalone reference templates live in `packaging/bsd/`:

```bash
# FreeBSD / DragonFly BSD
sudo ./gymbag service install --config /usr/local/etc/gymbag/gymbag.json --start
# Installs /usr/local/etc/rc.d/gymbag, runs sysrc (or /etc/rc.conf) and service gymbag onestart

# OpenBSD
doas ./gymbag service install --config /etc/gymbag/gymbag.json --start
# Installs /etc/rc.d/gymbag, enables via rcctl enable/set flags, and starts via rcctl start

# NetBSD
sudo ./gymbag service install --config /usr/pkg/etc/gymbag/gymbag.json --start
# Installs /etc/rc.d/gymbag, sets gymbag=YES in /etc/rc.conf, and runs /etc/rc.d/gymbag start
```

> **Privileged ports on FreeBSD without root:** enable non-root low-port binding with `sysctl net.inet.ip.portrange.reservedhigh=0` or run under a dedicated daemon user / pf redirect.

## Debian / Ubuntu (`.deb` + `systemd`)

Installing the `.deb` release package puts `/usr/bin/gymbag` (with `cap_net_bind_service=+ep`), `/etc/gymbag/Gymbagfile` (as a conffile), and `/lib/systemd/system/gymbag.service` (`DynamicUser=yes`, `StateDirectory=gymbag`, `GYMBAG_CERT_DIR=/var/lib/gymbag/certs`) in place:

```bash
sudo apt install ./gymbag_<version>_linux_amd64.deb
sudo gymbag init -o /etc/gymbag/Gymbagfile --format gymbagfile
sudo systemctl enable --now gymbag
```

## Arch Linux (`pacman` + `systemd`)

Installing the `.pkg.tar.zst` package (or building from `packaging/arch/PKGBUILD`) puts `/usr/bin/gymbag` (with `cap_net_bind_service=+ep`), `/etc/gymbag/Gymbagfile`, and `/usr/lib/systemd/system/gymbag.service` in place:

```bash
sudo gymbag init -o /etc/gymbag/Gymbagfile --format gymbagfile
sudo systemctl enable --now gymbag
```

## NixOS Module (`services.gymbag`)

On NixOS, import `nixosModules.default` from the flake:

```nix
{
  inputs.gymbag.url = "github:Evil-Project/gymbag";

  outputs = { nixpkgs, gymbag, ... }: {
    nixosConfigurations.my-server = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        gymbag.nixosModules.default
        {
          services.gymbag = {
            enable = true;
            openFirewall = true;
            configText = ''
              app.example.com {
                  reverse_proxy http://127.0.0.1:3000
                  verify 3
              }
            '';
          };
        }
      ];
    };
  };
}
```

The module runs Gymbag with `DynamicUser = true`, `StateDirectory = "gymbag"`, and `CAP_NET_BIND_SERVICE` so ports `80` and `443` work without root.

## Rendering Unit Files Cross-Platform (`render`)

`render` outputs the service definition for any supported init system on any OS—useful when generating a Linux `systemd` unit on macOS before deploying:

```bash
./gymbag service render --type systemd --system --service-user gymbag --config /etc/gymbag/gymbag.json
./gymbag service render --type systemd --system -o ./deploy/gymbag.service
```

## `gymbag service` Flags

| Flag | Description |
| :--- | :--- |
| `-t`, `--type <mgr>` | `auto` (default), `systemd`, `launchd`, `openrc`, `supervisor`, `freebsd-rc`, `openbsd-rc`, or `netbsd-rc` |
| `-n`, `--name <name>` | Service name (default `gymbag`) |
| `--label <id>` | `launchd` label (default `io.gymbag.<name>`) |
| `-c`, `--config <path>` | Path to `gymbag.json`, `Gymbagfile`, or `Caddyfile` |
| `--bin <path>` | Path to `gymbag` binary (default current executable) |
| `--workdir <dir>` | Working directory (default directory of the config file) |
| `--env-file <path>` | Optional `.env` file (auto-detected if `<workdir>/.env` exists) |
| `--log-dir <dir>` | Log directory for `launchd`, `openrc`, `supervisor`, `freebsd-rc`, `openbsd-rc`, and `netbsd-rc` |
| `-o`, `--output <path>` | Override target unit/plist/script path |
| `--service-user <user>` | Run-as OS user for system-level service |
| `--service-group <grp>` | Run-as OS group for system-level service |
| `--user` / `--system` | Force user-level or system-wide scope |
| `--watch` | Pass `--watch` to `gymbag run` (default `true`) |
| `--enable` | Enable service on boot during `install` (default `true`) |
| `--start`, `--now` | Start service immediately after `install` |
| `--no-reload` | Write/remove unit file without running service-manager reload/enable commands |
| `-f`, `--force` | Overwrite an existing service file |
