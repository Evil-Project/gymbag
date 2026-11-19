# System Service Setup

Gymbag includes a built-in service manager (`gymbag service`) that generates, registers, starts, stops, restarts, inspects, and uninstalls background services across:

| Manager | Target OS / Init | Default Unit Path |
| :--- | :--- | :--- |
| `systemd` | Linux (Ubuntu, Debian, Fedora, Arch, RHEL) | `/etc/systemd/system/<name>.service` (or `~/.config/systemd/user/<name>.service` with `--user`) |
| `launchd` | macOS (LaunchDaemons / LaunchAgents) | `/Library/LaunchDaemons/io.gymbag.<name>.plist` (or `~/Library/LaunchAgents/io.gymbag.<name>.plist` with `--user`) |
| `openrc` | Linux (Alpine, Gentoo, Artix) | `/etc/init.d/<name>` |
| `supervisor` | Unix / Containers (`supervisord`) | `/etc/supervisor/conf.d/<name>.conf` (or `~/.config/supervisor/conf.d/<name>.conf` with `--user`) |

---

## Two-Command Service Workflow

Once you have a `Gymbagfile` (from `gymbag init`):

```bash
# 1. Register the system service (auto-detects systemd / launchd / openrc / supervisor)
sudo ./gymbag service install --config /etc/gymbag/Gymbagfile

# 2. Start the service
sudo ./gymbag service start
```

Or combine config generation and registration into step 1:

```bash
sudo ./gymbag init -d app.example.com -u :3000 -o /etc/gymbag/Gymbagfile --service auto --system
sudo ./gymbag service start --system
```

::: tip Automatic `--watch` and `CAP_NET_BIND_SERVICE`
- By default, `gymbag service install` configures `gymbag run --config <path> --watch`, so editing `Gymbagfile` hot-reloads routes, WAF policies, and rules without restarting the process.
- When registering a system-wide `systemd` unit with a non-root `--service-user` and privileged ports (`80`/`443`), Gymbag automatically adds `AmbientCapabilities=CAP_NET_BIND_SERVICE`, `CapabilityBoundingSet=CAP_NET_BIND_SERVICE`, and `NoNewPrivileges=true`.
- OpenRC units automatically include `capabilities="^cap_net_bind_service"` when `--service-user` is non-root.
:::

---

## Service Lifecycle Subcommands

```bash
# Start, stop, restart, or inspect status
./gymbag service start
./gymbag service stop
./gymbag service restart
./gymbag service status

# Remove service registration and unit file
./gymbag service uninstall
```

### User-Level vs System-Wide Scope

- When running without `sudo` (`euid != 0`), `gymbag service` defaults to `--user` scope (`systemd --user` or `~/Library/LaunchAgents`) for `systemd`, `launchd`, and `supervisor`.
- When running as `root` (`sudo`), it defaults to `--system` scope (`/etc/systemd/system`, `/Library/LaunchDaemons`, `/etc/init.d`).
- Pass `--user` or `--system` explicitly to override the default scope.

---

## NixOS Declarative Module (`services.gymbag`)

On NixOS, `/etc/systemd/system` is managed declaratively. Add the flake input and import `nixosModules.default`:

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

The module runs Gymbag with `DynamicUser = true`, `StateDirectory = "gymbag"`, and `CAP_NET_BIND_SERVICE` so it binds ports `80`/`443` safely without root.

---

## Arch Linux (`pacman` + `systemd`)

When installed via the Arch Linux `.pkg.tar.zst` package (or `packaging/arch/PKGBUILD`), Gymbag installs `/usr/bin/gymbag` (with `cap_net_bind_service=+ep`), `/etc/gymbag/Gymbagfile`, and `/usr/lib/systemd/system/gymbag.service`:

```bash
sudo gymbag init -o /etc/gymbag/Gymbagfile
sudo systemctl enable --now gymbag
```

---

## Previewing or Generating Unit Files Cross-Platform (`render`)

You can render a service file for **any** supported manager from **any** OS (for example, generating a Linux `systemd` unit on macOS before deploying to a VPS):

```bash
# Preview systemd unit on stdout
./gymbag service render --type systemd --system --service-user gymbag --config /etc/gymbag/Gymbagfile

# Write unit file to a specific path without registering locally
./gymbag service render --type systemd --system -o ./deploy/gymbag.service
```

---

## `gymbag service` Flags Reference

| Flag | Description |
| :--- | :--- |
| `-t`, `--type <mgr>` | `auto` (default), `systemd`, `launchd`, `openrc`, or `supervisor` |
| `-n`, `--name <name>` | Service name (default `gymbag`) |
| `--label <id>` | `launchd` reverse-DNS label (default `io.gymbag.<name>`) |
| `-c`, `--config <path>` | Path to `Gymbagfile`, `Caddyfile`, or JSON config |
| `--bin <path>` | Path to `gymbag` binary (default current executable) |
| `--workdir <dir>` | Service working directory (default directory of config file) |
| `--env-file <path>` | Optional `.env` file (auto-detected if `<workdir>/.env` exists) |
| `--log-dir <dir>` | Log directory for `launchd`, `openrc`, and `supervisor` |
| `-o`, `--output <path>` | Override target unit/plist/script file path |
| `--service-user <user>` | OS user to run system-level service as |
| `--service-group <grp>` | OS group for system-level service |
| `--user` / `--system` | Force user-level or system-wide scope |
| `--watch` | Pass `--watch` to `gymbag run` (default `true`) |
| `--enable` | Enable service at boot during `install` (default `true`) |
| `--start`, `--now` | Start service immediately after `install` |
| `--no-reload` | Write/remove unit file without invoking `systemctl`/`launchctl`/`rc-update`/`supervisorctl` |
| `-f`, `--force` | Overwrite existing unit/plist/script file |
