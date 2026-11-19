# Two-Command Quick Start

Gymbag is compiled into a single standalone binary with zero external Go dependencies — all HTML templates, SHA-256 browser solver scripts, and mascot PNGs are embedded via `go:embed`.

## Installation

### Prebuilt Static Binaries, Docker, Arch Linux & NixOS

Every tagged release on [GitHub Releases](https://github.com/Evil-Project/gymbag/releases) publishes `CGO_ENABLED=0` static binaries (`linux`, `darwin`, `windows` for `amd64`/`arm64`), Arch Linux `.pkg.tar.zst` packages (`x86_64` and `aarch64`), and multi-arch GHCR container images (`ghcr.io/evil-project/gymbag`):

::: code-group
```bash [Docker (GHCR)]
docker run -d --name gymbag \
  -p 80:80 -p 443:443 \
  --cap-add NET_BIND_SERVICE \
  -v "$(pwd)/Gymbagfile:/app/Gymbagfile:ro" \
  -v gymbag-certs:/app/certs \
  ghcr.io/evil-project/gymbag:latest
```

```bash [Arch Linux (pacman)]
# Install a release .pkg.tar.zst binary package
sudo pacman -U gymbag-<version>-1-x86_64.pkg.tar.zst

# Or build from source with PKGBUILD
git clone https://github.com/Evil-Project/gymbag.git
cd gymbag/packaging/arch
makepkg -si
```

```bash [Nix / NixOS]
# Run or install directly via Nix Flakes
nix run github:Evil-Project/gymbag -- --help
nix profile install github:Evil-Project/gymbag

# Static Linux release tarballs (CGO_ENABLED=0) also run directly on NixOS
```

```bash [Build from Source (Go)]
git clone https://github.com/Evil-Project/gymbag.git
cd gymbag
go build -o gymbag ./cmd/gymbag
```
:::

See the [Docker & Docker Compose guide](/guide/docker) for full container deployment, Compose + Redis stacks, and volume permissions.

## Start in Two Commands

### Option A: Flag-driven (Non-interactive)

```bash
# 1. Generate Gymbagfile
./gymbag init -d app.example.com -u http://localhost:3000

# 2. Start Gymbag
./gymbag
```

### Option B: Interactive Setup Wizard

Run `gymbag init` without flags to walk through domains, upstream URL, verify level, SSL provider, ports, Admin API, and optional system service registration:

```bash
# 1. Interactive config generator
./gymbag init

# 2. Start Gymbag
./gymbag
```

### Option C: Local Development Preset (`*.internal`)

```bash
# 1. Generate app.internal -> http://localhost:3000 with local PKI
./gymbag init --preset local

# 2. Start Gymbag (use unprivileged ports or sudo for 80/443 + /etc/hosts)
./gymbag --http-port 8080 --https-port 8443
```

### Option D: Register as a System Service

```bash
# 1. Generate Gymbagfile and register systemd / launchd / openrc / supervisor
./gymbag init -d app.example.com -u :3000 --service auto

# 2. Start the registered service
./gymbag service start
```

See [System Service Setup](/guide/system-service) for `systemd`, `launchd`, `openrc`, and `supervisor` options.

---

## Ad-Hoc Reverse Proxy (Zero Config File)

For quick one-off testing without creating any file on disk:

```bash
./gymbag reverse-proxy --from app.internal --to :3000 --verify 3 --verify-timeout 12h
```

---

## Config Generator Options (`gymbag init`)

`gymbag init` validates every generated config before writing it to disk.

| Flag | Description |
| :--- | :--- |
| `-o`, `--output <path>` | Output path (`Gymbagfile` by default, or `gymbag.json` with `--format json`) |
| `--format <fmt>` | `gymbagfile` (default) or `json` |
| `--preset <name>` | `standard`, `local`, `public`, `strict`, `ai-only`, `suspicious`, or `proxy-only` |
| `-d`, `--domain <domains>` | Downstream domain(s), comma-separated |
| `-u`, `--upstream <url>` | Upstream URL (e.g. `http://localhost:3000` or `:3000`) |
| `--site <domain=url>` | Add multiple site blocks (repeatable, e.g. `--site app.example.com=:3000 --site api.example.com=:8080`) |
| `-l`, `--verify <0-3\|off>` | Verify level (`0`, `1`, `2`, `3`, or `off`) |
| `--difficulty <1-5>` | SHA-256 PoW leading-hex-zero difficulty (default `4`) |
| `--verify-timeout <dur>` | Verified clearance timeout (e.g. `30m`, `12h`, `7d`, or seconds; alias `--clearance-ttl`) |
| `--remote-rules <url>` | Remote firewall rules URL (repeatable or comma-separated) |
| `--remote-rules-refresh <dur>` | Refresh interval for remote rules (default `5m`) |
| `--no-protect` | Disable WAF verification (`protect off`) |
| `--no-default-rules` | Disable built-in baseline firewall rules (`default_rules off`) |
| `--ssl-provider <name>` | `letsencrypt`, `zerossl`, `google`, `ssl.com`, `litessl`, `buypass`, `local`, or custom URL |
| `--email <addr>` | ACME contact email (also enables ZeroSSL auto-EAB) |
| `--eab-kid` / `--eab-hmac-key` | ACME External Account Binding credentials |
| `--http-port` / `--https-port` | Custom HTTP/HTTPS ports (defaults `80` / `443`) |
| `--bind <ip>` | Bind address (auto-selected when omitted) |
| `--admin <addr>` | Enable local REST Admin API (e.g. `127.0.0.1:2019`) |
| `--cert-dir <dir>` | Directory for certificates, local CA, and caches (default `certs`) |
| `--redis <url>` | Optional Redis URL for shared challenge and rate-limit state |
| `--ip-security-action <act>` | Action for threat-intel IP hits (`verify` or `block`) |
| `--no-auto-hosts` | Disable automatic `/etc/hosts` entries for local domains |
| `-y`, `--yes` | Skip interactive prompts and accept defaults |
| `-i`, `--interactive` | Force interactive wizard even when flags are present |
| `--dry-run` | Print generated config to stdout without writing a file |
| `-f`, `--force` | Overwrite an existing config file |
| `--service <type>` | Also register a system service (`auto`, `systemd`, `launchd`, `openrc`, `supervisor`) |

---

## What Open-Box Protection Includes Out of the Box

Writing bare `verify` (verify level `3`: `suspicious & ai`) or `block` activates three layers immediately without manual regex writing:

1. **Baseline firewall rules (`default_rules on`)** — Blocks `/.env`, `/.git/*`, `/.aws/*`, `/wp-admin/*`, `sqlmap`/`nikto`/`nuclei`, and SQLi/XSS query probes (`403`), and challenges CLI/headless automation clients.
2. **Auto-updating IP threat intelligence** — Pulls IPsum (level 3+), Spamhaus DROP v4/v6, and abuse.ch Feodo Tracker on startup and every 6 hours, plus keyless live Shodan InternetDB and StopForumSpam reputation lookups.
3. **Official AI crawler IP lists** — Tracks published CIDR ranges for OpenAI, Anthropic, Perplexity, Google, and Apple so crawlers are detected even when spoofing desktop User-Agents.
