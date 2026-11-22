# Two-Command Quick Start

Gymbag compiles to a single binary using only the Go standard library. HTML templates, the SHA-256 browser solver script, and the mascot PNGs are embedded directly via `go:embed`.

## Installation

Each tagged release on [GitHub Releases](https://github.com/Evil-Project/gymbag/releases) publishes static `CGO_ENABLED=0` binaries (`linux`, `darwin`, `windows` for `amd64`/`arm64`), Debian/Ubuntu `.deb` packages, Arch Linux `.pkg.tar.zst` packages, NixOS bundles, and multi-arch GHCR container images (`ghcr.io/evil-project/gymbag`):

::: code-group
```bash [Debian / Ubuntu (.deb)]
# Install a release .deb package (sets cap_net_bind_service and systemd unit)
sudo apt install ./gymbag_<version>_linux_amd64.deb

# Generate config and enable the service
sudo gymbag init -o /etc/gymbag/Gymbagfile --format gymbagfile
sudo systemctl enable --now gymbag
```

```bash [Docker (GHCR)]
docker run -d --name gymbag \
  -p 80:80 -p 443:443 \
  --cap-add NET_BIND_SERVICE \
  -v "$(pwd)/Gymbagfile:/app/Gymbagfile:ro" \
  -v gymbag-certs:/app/certs \
  ghcr.io/evil-project/gymbag:latest
```

```bash [Arch Linux (pacman)]
# Install a release .pkg.tar.zst package
sudo pacman -U gymbag-<version>-1-x86_64.pkg.tar.zst

# Or build from packaging/arch
git clone https://github.com/Evil-Project/gymbag.git
cd gymbag/packaging/arch
makepkg -si
```

```bash [Nix / NixOS]
nix run github:Evil-Project/gymbag -- --help
nix profile install github:Evil-Project/gymbag

# Static Linux release binaries (CGO_ENABLED=0) also run directly on NixOS
```

```bash [Build from Source (Go)]
git clone https://github.com/Evil-Project/gymbag.git
cd gymbag
go build -o gymbag ./cmd/gymbag
```
:::

See the [Docker & Docker Compose guide](/guide/docker) for container deployments, Compose + Redis stacks, and volume permissions.

## Start in Two Commands

`gymbag init` writes `gymbag.json` by default, or `Gymbagfile` when you pass `--format gymbagfile` (or `-o Gymbagfile`). Every generated config is validated before being written to disk.

### HTTP / HTTPS Website

```bash
# 1. Generate gymbag.json (add --format gymbagfile for Caddy syntax)
./gymbag init -d app.example.com -u http://localhost:3000

# 2. Start Gymbag (auto-loads ./gymbag.json, ./Gymbagfile, then ./Caddyfile)
./gymbag
```

### SSH Protection

Moves `sshd` to a random free port in `20000–59999` inside `/etc/ssh/sshd_config` (keeping a `.gymbag.bak` backup), validates with `sshd -t` (rolling back if anything fails), reloads `sshd`, and updates the host firewall (`ufw`, `firewalld`, `nftables`, or `iptables`+`ip6tables`) so external traffic can only enter through Gymbag on port `22`:

```bash
# Keep a second SSH terminal open while running this on a remote host
sudo ./gymbag init -t ssh
sudo ./gymbag
```

### Minecraft Server Protection

Configure `--server-jar` to have Gymbag **automatically start your Minecraft server** alongside the protection proxy and **automatically select the best compatible Java runtime** on the machine (inspecting `version.json`, bundler nested JARs, `patch.properties`, or `.class` bytecode headers to distinguish Java 8 legacy servers from Java 17/21+ servers, and sending a graceful console `stop` on shutdown). Gymbag also moves `server-port` to a random free port in `20000–59999` inside `server.properties` (creating it if missing when `server_jar` is set, and binding empty/`0.0.0.0`/`::` `server-ip` to `127.0.0.1`), passes Server List Pings through untouched, and shows the HTTPS verification link inside Minecraft's **"Connection Lost"** dialog on login:

```bash
# Auto-start server.jar with automatically selected Java + random backend port:
./gymbag init -t minecraft --server-jar /srv/mc/paper.jar --mc-jvm-args "-Xms1G -Xmx4G"
./gymbag

# Or guard an already-running server via server.properties:
./gymbag init -t minecraft --server-properties /path/to/server.properties
./gymbag
```

### All-in-One (`http,ssh,minecraft`) or Interactive Wizard

One process can guard websites, SSH, and Minecraft together:

```bash
# Non-interactive combined config:
sudo ./gymbag init -t http,ssh,minecraft -d app.example.com -u :3000 --server-jar /srv/mc/paper.jar
sudo ./gymbag

# Or run the interactive wizard:
./gymbag init
./gymbag
```

> **Verification Portal & IP SSL Certs:** SSH and Minecraft share an HTTPS verification portal on port `8443` by default (`--verify-port`). Pass `--verify-url https://verify.example.com:8443` to use a custom domain, or leave it blank to serve the check on the server's detected IP using an RFC 8738 ACME IP certificate (falling back to Gymbag's Local CA or a self-signed IP cert when HTTP-01 isn't reachable). See the [SSH & Minecraft Protection guide](/guide/ssh-minecraft).

### Local Development (`*.internal`)

```bash
# 1. Generate app.internal -> http://localhost:3000 with Local PKI
./gymbag init --preset local

# 2. Run on unprivileged ports (or use sudo for 80/443 + /etc/hosts)
./gymbag --http-port 8080 --https-port 8443
```

### Register as a System Service

```bash
./gymbag init -d app.example.com -u :3000 --service auto
./gymbag service start
```

See [System Service Setup](/guide/system-service) for `systemd`, `launchd`, `openrc`, and `supervisor`.

## Existing nginx / Apache / Caddyfile Configs

Gymbag parses native `nginx.conf`, Apache `httpd.conf` / `apache2.conf`, and Caddyfile v2 files directly:

```bash
./gymbag run -c /etc/nginx/nginx.conf
./gymbag config convert -c Caddyfile -o gymbag.json --format json
```

See [Routes & Config Import](/guide/routes-and-imports) for path routes, static files, redirects, and conversion details.

## Ad-Hoc Reverse Proxy (No Config File)

```bash
./gymbag reverse-proxy --from app.internal --to :3000 --verify 3 --verify-timeout 12h
```

## `gymbag init` Flags

| Flag | Description |
| :--- | :--- |
| `-o`, `--output <path>` | Output path (`gymbag.json` by default, or `Gymbagfile` with `--format gymbagfile`) |
| `--format <fmt>` | `json` (default) or `gymbagfile` |
| `-t`, `--template <list>` | Service template(s): `http`, `ssh`, `minecraft`, or `all` (comma-separated or repeatable) |
| `--preset <name>` | `standard`, `local`, `public`, `strict`, `ai-only`, `suspicious`, `proxy-only`, `http`, `ssh`, `minecraft`, or `all` |
| `-d`, `--domain <domains>` | Downstream domain(s), comma-separated |
| `-u`, `--upstream <url>` | Upstream URL (`http://localhost:3000` or `:3000`) |
| `--site <domain=url>` | Add multiple site entries (repeatable) |
| `--ssh` / `--ssh-port <port>` | Enable SSH protection (default port `22`) |
| `--ssh-upstream <addr>` / `--sshd-config <p>` | Explicit `sshd` target or `sshd_config` path (default `/etc/ssh/sshd_config`) |
| `--minecraft`, `--mc` / `--mc-port <port>` | Enable Minecraft protection (default port `25565`) |
| `--mc-upstream <addr>` / `--server-properties <p>` | Explicit Minecraft backend or `server.properties` path |
| `--mc-jar`, `--server-jar <path>` | Path to Minecraft server `.jar` (auto-starts server with auto-selected Java) |
| `--mc-java`, `--java <bin>` | Explicit Java binary path for Minecraft server (defaults to auto-select) |
| `--mc-jvm-args <args>` / `--mc-work-dir <dir>` | Space-separated JVM flags and working directory for managed Minecraft server |
| `--verify-url <url>` | Shared HTTPS verification portal URL (blank = server IP + IP SSL cert) |
| `--verify-port <port>` | Shared HTTPS verification portal port (default `8443`) |
| `--no-auto-setup` / `--no-firewall` | Skip automatic `sshd_config`/firewall or `server.properties` edits |
| `-l`, `--verify <0-3\|off>` | Verify level (`0`, `1`, `2`, `3`, or `off`) |
| `--difficulty <1-5>` | SHA-256 PoW difficulty (default `4`) |
| `--verify-timeout <dur>` | Clearance lifetime (`30m`, `12h`, `7d`, or seconds; alias `--clearance-ttl`) |
| `--remote-rules <url>` | Remote firewall rules URL (repeatable or comma-separated) |
| `--remote-rules-refresh <dur>` | Refresh interval for remote rules (default `5m`) |
| `--rules-file <path>` / `--rules-dir <path>` | Local rules file or directory (repeatable) |
| `--rules-dynamic` / `--rules-watch-interval <dur>` | Watch local rule sources for changes (default `2s`) |
| `--no-protect` / `--no-default-rules` | Disable PoW verification or built-in baseline rules |
| `--ssl-provider <name>` | `letsencrypt`, `zerossl`, `google`, `ssl.com`, `litessl`, `buypass`, `local`, or URL |
| `--email <addr>` | ACME contact email (also enables ZeroSSL auto-EAB) |
| `--eab-kid` / `--eab-hmac-key` | ACME External Account Binding credentials |
| `--http-port` / `--https-port` | Custom HTTP/HTTPS ports (defaults `80` / `443`) |
| `--bind <ip>` | Bind IP address (auto-selected when omitted) |
| `--admin <addr>` | Enable local REST Admin API (e.g. `127.0.0.1:2019`) |
| `--cert-dir <dir>` | Directory for certs, local CA, and caches (default `certs`) |
| `--redis <url>` | Redis URL for shared challenge, rate-limit, and SSH/MC gate state |
| `--ip-security-action <act>` | Threat-intel hit action (`verify` or `block`) |
| `--no-auto-hosts` | Disable `/etc/hosts` management for local domains |
| `-y`, `--yes` / `-i`, `--interactive` | Skip wizard prompts or force interactive wizard |
| `--dry-run` / `-f`, `--force` | Print config to stdout without writing / overwrite existing file |
| `--service <type>` | Register OS service (`auto`, `systemd`, `launchd`, `openrc`, `supervisor`) |

## What Works Without Writing Rules

Writing bare `verify` (sets verify level `3`: `suspicious & ai`) or `block` enables three layers right away:

1. **Built-in baseline rules (`default_rules on`)**: Returns `403` on `/.env`, `/.git/*`, `/.aws/*`, `/wp-admin/*`, `sqlmap`/`nikto`/`nuclei`, and SQLi/XSS query probes, and sends CLI/headless HTTP clients to the PoW screen.
2. **Threat feeds, live IP reputation, and VPN/Tor detection**: Refreshes IPsum (level 3+), Spamhaus DROP v4/v6, Feodo Tracker, X4BNet VPN lists, and Tor exit nodes at startup and every 6 hours, and queries Shodan InternetDB, StopForumSpam, `ipquery.io`, and `ipapi.is` with fail-open timeouts. VPN/proxy/Tor IPs count as suspicious at levels `1` and `3` across HTTP, SSH, and Minecraft.
3. **Official AI crawler IP lists**: Matches published CIDR blocks for OpenAI, Anthropic, Perplexity, Google, and Apple even if a bot spoofs a Chrome `User-Agent`.
