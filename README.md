# Gymbag

<p align="center">
  <img src="docs/public/mascot-happy.png" alt="Gymbag mascot" width="140" />
</p>

<p align="center">
  <strong>Single-binary HTTP/HTTPS, SSH &amp; Minecraft Proof-of-Work Gateway in pure Go</strong>
</p>

<p align="center">
  <a href="https://github.com/Evil-Project/gymbag/releases"><img src="https://img.shields.io/github/v/release/Evil-Project/gymbag?include_prereleases&label=Release" alt="Release" /></a>
  <a href="https://github.com/Evil-Project/gymbag/pkgs/container/gymbag"><img src="https://img.shields.io/badge/Docker-GHCR-blue?logo=docker" alt="Docker GHCR" /></a>
  <a href="go.mod"><img src="https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go" alt="Go 1.27+" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache--2.0-green" alt="Apache-2.0 License" /></a>
  <a href="https://evil-project.github.io/gymbag/"><img src="https://img.shields.io/badge/Docs-VitePress-b81d33" alt="Docs" /></a>
</p>

<p align="center">
  <strong>README:</strong>
  <a href="README.md">English</a> ·
  <a href="README.zh.md">简体中文</a> ·
  <a href="README.ja.md">日本語</a> ·
  <a href="README.ko.md">한국어</a> ·
  <a href="README.es.md">Español</a>
  <br />
  <sub>Online Docs: <a href="https://evil-project.github.io/gymbag/">EN</a> · <a href="https://evil-project.github.io/gymbag/zh/">ZH</a> · <a href="https://evil-project.github.io/gymbag/ja/">JA</a> · <a href="https://evil-project.github.io/gymbag/ko/">KO</a> · <a href="https://evil-project.github.io/gymbag/es/">ES</a></sub>
</p>

Gymbag is a single-binary HTTPS reverse proxy and proof-of-work WAF (in the spirit of Anubis) that also guards SSH and Minecraft servers. It uses only the Go standard library with zero third-party Go modules; HTML pages, the SHA-256 browser solver, and the mascot images are compiled into the binary via `go:embed`.

Self-hosted apps, Git forges, SSH ports, and Minecraft servers all get hammered by the same traffic: LLM scrapers ignoring `robots.txt`, bots probing `/.env` and `/wp-admin`, SSH brute-forcers, and login floods. Instead of gluing together nginx or Caddy, a PoW sidecar, fail2ban, and custom firewall scripts, Gymbag handles TLS termination, multi-CA ACME, HTTP/WebSocket proxying, SSH/Minecraft TCP gating, IP threat/VPN/AI lists, and firewall rules in one process, configured with `gymbag.json` or a Caddy-style `Gymbagfile`.

## Quick start

Prebuilt static binaries (`CGO_ENABLED=0` for Linux, macOS, and Windows on `amd64`/`arm64`), Debian/Ubuntu `.deb` packages, Arch `.pkg.tar.zst` packages, and NixOS tarballs/NARs are published on [GitHub Releases](https://github.com/Evil-Project/gymbag/releases). You can also run the GHCR container image, use the Nix flake, or build from source:

```bash
go build -o gymbag ./cmd/gymbag
```

**Debian / Ubuntu:**
```bash
sudo apt install ./gymbag_<version>_linux_amd64.deb
sudo gymbag init -o /etc/gymbag/Gymbagfile --format gymbagfile
sudo systemctl enable --now gymbag
```

**Arch Linux:**
```bash
sudo pacman -U gymbag-<version>-1-x86_64.pkg.tar.zst
# Or build from packaging/arch:
cd packaging/arch && makepkg -si
```

**Nix / NixOS:**
```bash
nix run github:Evil-Project/gymbag -- --help
nix profile install github:Evil-Project/gymbag
```
Static Linux release binaries also run directly on NixOS without `patchelf` or `nix-ld`, and `nixosModules.default` exposes a declarative `services.gymbag` module.

Getting started takes two commands: generate a config (which writes `gymbag.json` by default, or `Gymbagfile` with `--format gymbagfile`), then start:

```bash
# Protect an HTTP/HTTPS website
./gymbag init -d app.example.com -u http://localhost:3000
./gymbag

# Protect SSH (moves sshd to a random port and sets up the firewall)
sudo ./gymbag init -t ssh
sudo ./gymbag

# Protect Minecraft Java Edition (auto-starts server.jar with auto-selected Java + randomizes port)
./gymbag init -t minecraft --server-jar /srv/mc/paper.jar --mc-jvm-args "-Xms1G -Xmx4G"
./gymbag

# Protect an existing Minecraft server without managed startup (randomizes server.properties port)
./gymbag init -t minecraft --server-properties /path/to/server.properties
./gymbag

# Protect all three from one process
sudo ./gymbag init -t http,ssh,minecraft -d app.example.com -u :3000 --server-jar /srv/mc/paper.jar
sudo ./gymbag
```

You can also point Gymbag directly at an existing **nginx** (`nginx.conf`), **Apache** (`httpd.conf` / `apache2.conf`), or **Caddyfile v2** config, or register it as a system service (`systemd`, `launchd`, `openrc`, or `supervisor`):

```bash
./gymbag init -d app.example.com -u :3000 --service auto
./gymbag service start
```

For quick one-off testing without writing any config file:

```bash
./gymbag reverse-proxy --from app.internal --to :3000 --verify 3
```

> **Ports below 1024:** `:80` handles HTTP-to-HTTPS redirects and ACME HTTP-01, `:443` serves HTTPS, and `:22` listens for SSH. On Linux, `sudo setcap 'cap_net_bind_service=+ep' ./gymbag` lets Gymbag bind them without running as root (the `.deb` and Arch packages set this capability for you). For unprivileged local web testing, use `--http-port 8080 --https-port 8443`.

### What runs with no rules written

You do not need to write regexes or import blocklists before putting Gymbag in front of traffic. Writing bare `verify` (sets level `3`: `suspicious & ai`) or bare `block` (switches threat-intel hits from a challenge to `403`) enables three default layers:

1. **Built-in baseline rules (`default_rules on`)**
   - **Sensitive paths (`403`)**: `/.env`, `/.env.*`, `/.git`, `/.git/*`, `/.svn/*`, `/.hg/*`, `/.DS_Store`, `/.aws/*`, `/.ssh/*`, `/docker-compose.yml`, `/docker-compose.yaml`, `*.bak`, `*.sql`, `*.sqlite`.
   - **CMS and framework probes (`403`)**: `/wp-admin`, `/wp-admin/*`, `/wp-login.php`, `/xmlrpc.php`, `/phpmyadmin*`, `/vendor/phpunit*`, `/cgi-bin/*`, `/actuator/env*`, `/actuator/heapdump*`.
   - **Vulnerability scanners (`403`)**: `sqlmap`, `nikto`, `nuclei`, `masscan`, `zgrab`, `gobuster`, `dirbuster`, `ffuf`, `wfuzz`, `feroxbuster`, `acunetix`, `nessus`, `openvas`.
   - **Injection and traversal queries (`403`)**: `UNION SELECT`, `<script`, `javascript:`, `/etc/passwd`, `information_schema`, `../`, `cmd.exe`, `/bin/sh` in query strings, plus request-shape checks rejecting null bytes and `%2e%2e` traversal.
   - **Automation clients (PoW challenge)**: when a site has no custom rules yet, CLI libraries (`curl`, `wget`, `python-requests`, `python-httpx`, `go-http-client`, `aiohttp`, `okhttp`, `axios`, `node-fetch`, `scrapy`), headless browsers (`headlesschrome`, `playwright`, `puppeteer`, `selenium`, `phantomjs`), and empty `User-Agent` headers go to the proof-of-work page.
2. **Self-updating IP threat intel and VPN/Tor detection**
   - Fetches [IPsum (level 3+)](https://raw.githubusercontent.com/stamparm/ipsum/master/levels/3.txt), [Spamhaus DROP IPv4](https://www.spamhaus.org/drop/drop_v4.json) / [IPv6](https://www.spamhaus.org/drop/dropv6.txt), and [abuse.ch Feodo Tracker](https://feodotracker.abuse.ch/downloads/ipblocklist.txt) at startup and every 6 hours, cached under `<cert_dir>/intel-cache` using `ETag` and `If-Modified-Since`.
   - Runs keyless live reputation lookups against **Shodan InternetDB** (`internetdb.shodan.io`) and **StopForumSpam** (`api.stopforumspam.org`) with a 1-hour in-memory cache and fail-open timeouts so upstream slowness never blocks visitors.
   - **Free VPN/proxy/Tor detection (`vpn_detection on`)**: loads auto-updating X4BNet VPN IPv4/IPv6 lists and the Tor bulk exit list alongside live keyless lookups (`api.ipquery.io` and `api.ipapi.is`, cached for 1 hour, 1.2s fail-open timeout). Matching IPs are flagged as **suspicious** at verify levels `1` and `3` across HTTP, SSH, and Minecraft.
3. **Official AI crawler IP lists**
   - Tracks published CIDR lists from **OpenAI** (`gptbot`, `searchbot`, `chatgpt-user`), **Anthropic** (`claude.com/crawling/bots.json`), **Perplexity** (`perplexitybot`, `perplexity-user`), **Google** (`user-triggered-agents`, `user-triggered-fetchers-google`), and **Apple** (`applebot`), catching crawlers by source IP even if they spoof a Chrome `User-Agent`.

## SSH and Minecraft protection

When an unverified SSH or Minecraft connection hits Gymbag and triggers the configured verify level, Gymbag does not forward it to the backend daemon or game server. Instead, it replies inside the protocol's own disconnect message with a one-time HTTPS link (`/v/<token>`):

- **SSH (`port` `22` by default)**: `sudo ./gymbag init -t ssh` picks an unused local port in `20000–59999`, backs up `/etc/ssh/sshd_config` to `/etc/ssh/sshd_config.gymbag.bak`, writes the new `Port` before any `Match` blocks, runs `sshd -t` (rolling back automatically if validation fails), reloads `sshd`, and updates the active Linux firewall (`ufw`, `firewalld`, `nftables`, or `iptables`+`ip6tables`) so the backend port is only reachable from loopback while `22` and `8443` stay open. (Keep a second SSH shell open while changing `sshd_config`, or pass `--ssh-upstream 127.0.0.1:2222`, `--no-auto-setup`, or `--no-firewall` to handle it manually.) Once an IP passes the browser challenge, connections go straight through via raw TCP relay without touching the SSH handshake. At levels `1` and `3`, malformed banners or scripted SSH clients (`libssh`, `paramiko`, `zgrab`, `masscan`, `nmap`, `asyncssh`, `ssh2go`, `jsch`, `hydra`, `medusa`, `patator`, `python`, `curl`, `ssh-2.0-go`) are challenged; everyone else sees RFC 4253 `SSH_MSG_DISCONNECT` with the verification URL right in the terminal.
- **Minecraft Java (`port` `25565` by default)**: `./gymbag init -t minecraft --server-properties ./server.properties` backs up `server.properties` to `server.properties.gymbag.bak`, moves `server-port` to a free random port in `20000–59999`, and sets empty/wildcard `server-ip` to `127.0.0.1`. Server List Pings (`nextState == 1` and legacy `0xFE`) still pass through unless blocked by a rule, so players continue to see MOTD and player counts. On Login or Transfer, if verification is needed, Gymbag returns a Minecraft Login Disconnect packet (`0x00`) displaying the verification link inside the **"Connection Lost"** dialog.
- **Shared HTTPS Verification Portal (default `:8443`)**: Both SSH and Minecraft share `verify_port` (set `--verify-url https://verify.example.com:8443` to use your own domain, or leave it blank to use the server's detected IP). When using the server IP over ACME, Gymbag requests an **RFC 8738** IP certificate (using the CA's `shortlived` profile if offered) and falls back to Gymbag's Local CA or a self-signed IP cert when HTTP-01 isn't reachable. If `verify_port` matches `https_port` and HTTP sites are configured, the portal shares port `443`.

```caddyfile
{
    verify_url https://verify.example.com:8443
    verify_port 8443
}

ssh {
    port 22
    upstream 127.0.0.1:41822
    verify 3
    rate_limit 60 60s
}

minecraft {
    port 25565
    upstream 127.0.0.1:48291
    server_properties /srv/mc/server.properties
    verify 3
    allow ip 192.168.1.0/24
}
```

See the [SSH & Minecraft Protection Guide](docs/guide/ssh-minecraft.md) for full details and `gymbag.json` examples.

## Local development (`localhost` and `*.internal`)

Gymbag manages `/etc/hosts` and local TLS certificates automatically for any site on `localhost`, `*.localhost`, `*.internal` (`app.internal`, `api.internal`), `*.local`, or `*.test`:

```caddyfile
app.internal {
    reverse_proxy http://localhost:3000
    verify 3
}

api.internal {
    reverse_proxy http://localhost:8080
    protect off
}
```

### Automatic `/etc/hosts` entries

With `auto_hosts on` (the default), Gymbag checks `/etc/hosts` (`C:\Windows\System32\drivers\etc\hosts` on Windows) at startup and adds IPv4 and IPv6 loopback entries for local domains not already present:

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

Existing `localhost` entries are untouched. On `Ctrl+C` or `SIGTERM`, Gymbag re-reads the file (preserving edits you made in between) and deletes only its `# gymbag-managed:<instanceID>` lines. Writing `/etc/hosts` needs root on macOS/Linux; if run unprivileged, Gymbag logs a warning and continues. Set `auto_hosts off` (`GYMBAG_AUTO_HOSTS=off`) if you handle `.internal` via `dnsmasq` or manual edits.

### Local CA

Instead of throwing away ephemeral self-signed certs on every restart, Gymbag maintains a two-tier ECDSA P-256 CA under `<cert_dir>/ca/` (default `certs/ca/`):

1. **Root CA**: `"Neuro-sama Headquarters Root CA"` (`certs/ca/neuro-sama-root-ca.pem`, 10-year validity, `MaxPathLen: 1`)
2. **Intermediate CA**: `"Gymbag & Swarm SSL Local CA"` (`certs/ca/gymbag-swarm-local-ca.pem`, 5-year validity, `MaxPathLen: 0`)
3. **Leaf Certificate**: `certs/local-fullchain.pem` + `certs/local-privkey.pem` (90-day validity, renewed within 30 days, with DNS SANs plus `127.0.0.1` and `::1`)

Trust `"Neuro-sama Headquarters Root CA"` once and browsers plus `curl` accept all local sites:

```bash
# macOS
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain certs/ca/neuro-sama-root-ca.pem

# Debian / Ubuntu
sudo cp certs/ca/neuro-sama-root-ca.pem /usr/local/share/ca-certificates/neuro-sama-root-ca.crt && sudo update-ca-certificates

# Fedora / RHEL / Arch Linux
sudo trust anchor --store certs/ca/neuro-sama-root-ca.pem
```

> **Firefox:** Set `security.enterprise_roots.enabled` to `true` in `about:config` on macOS/Windows (and Linux with `p11-kit`), or import `certs/ca/neuro-sama-root-ca.pem` once in *Settings -> Privacy & Security -> Certificates -> View Certificates -> Authorities*.

## Multiple sites and verify levels (`0`–`3`)

One process handles many domains with round-robin upstream balancing and RFC 6455 WebSocket upgrades. Each site (and SSH/Minecraft service) picks a verify level (`0`–`3`) or disables PoW with `protect off`:

| Level | Name | Who gets challenged | Typical use |
| :---: | :--- | :--- | :--- |
| `0` | `always` | Anyone without a valid clearance cookie or cleared IP | Private admin tools, staging apps, L7 flood mitigation |
| `1` | `only suspicious` | Threat feeds, live bad-rep IPs, VPN/proxy/Tor IPs, scanners, CLI libs, headless browsers, missing/spoofed headers, exploit probes | Public apps and APIs where normal browsers should not see challenges |
| `2` | `only ai` | Official AI crawler IP ranges or AI bot `User-Agent` / `X-Ai-Agent` / `Sec-Agent` headers | Docs, blogs, and Git forges where humans and `curl` pass untouched |
| `3` | `suspicious & ai` | Combines `1` and `2` (default for bare `verify`, SSH, and Minecraft) | Public production websites |
| n/a | `protect off` | No PoW challenges; plain proxy (explicit custom rules still run) | Static asset hosts, webhooks, mirrors |

```caddyfile
app.example.com, www.example.com {
    reverse_proxy http://localhost:3000 http://localhost:3001
    verify 3
    difficulty 4
}

docs.example.com {
    reverse_proxy http://localhost:4000
    verify 2
}

api.example.com {
    reverse_proxy http://localhost:8080
    verify 1
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
    verify_timeout 30m
}

static.example.com {
    reverse_proxy http://localhost:9000
    protect off
}
```

Solving the SHA-256 challenge (`difficulty` `1`–`5`, default `4`) sets an `HttpOnly`, `SameSite=Lax` HMAC clearance cookie (`waf_clearance`, default `24h` / `86400s`) bound to host and client IP. Customize its lifetime globally or per site with `verify_timeout` / `clearance_ttl` (seconds or durations like `30m`, `12h`, `1h30m`, `7d`, `2w`), `--verify-timeout`, `GYMBAG_VERIFY_TIMEOUT`, or `PATCH /api/v1/sites/{domain}`. `GET /.waf/v1/session` returns `clearance_ttl`, `expires_at`, and `expires_in` for cleared visitors.

## Path routes, static files, redirects, and config imports

Sites can define ordered **path routes** alongside `reverse_proxy`. The first matching route handles the request; unmatched paths fall through to the site's default upstreams (or `404` if none are set). Each route has one handler: reverse proxy, static file server (`root`), static `respond`, or `redir`.

```caddyfile
app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    handle_path /static/* {
        root ./dist
        file_server browse
    }
    @health path /healthz /readyz
    respond @health "ok" 200
    redir /old/* https://{host}/new{uri} 301
    handle /api/* {
        reverse_proxy http://localhost:8080
        header X-Served-By gymbag
    }
    try_files {path} /index.html
}
```

Path matchers accept exact paths (`/exact`), globs (`/api/*`, `*.php`), or Go RE2 regexes (`~^/v[0-9]+/`). Redirects expand `{scheme}`, `{host}`, `{uri}`, `{path}`, and `{query}`. Static routes support `index`, `browse`, and SPA `fallback`. `header` sets a response header or removes one prefixed with `-`. Firewall rules and verification run before any route handler.

The equivalent `routes` array in `gymbag.json`:

```json
{
  "routes": [
    {"path": "/static/*", "strip_prefix": "/static", "root": "./dist", "browse": true},
    {"path": "/old/*", "redirect": {"to": "https://{host}/new{uri}", "code": 301}},
    {"path": "/api/*", "upstreams": ["http://localhost:8080"]}
  ]
}
```

Manage routes with `gymbag config route list|add|update|remove` or `PATCH /api/v1/sites/{domain}`.

**Importing existing configs:** `gymbag run -c /etc/nginx/nginx.conf` (or `httpd.conf` / `apache2.conf`) runs native nginx or Apache configs directly, and `gymbag config convert -c <file> -o gymbag.json --format json` converts nginx, Apache, and Caddyfile v2 (including snippets, `import`, `@matchers`, `handle`, `route`, `file_server`, `respond`, `redir`, and `header`).

## SSL providers, custom keys, and renewal

Public domains default to `tls auto`, which uses Let's Encrypt over ACME HTTP-01. Switch CAs with `ssl_provider` (aliases: `acme_provider`, `ca`, `cert_issuer`) or `tls <provider>`:

| Provider Keyword | Certificate Authority | EAB Requirement |
| :--- | :--- | :--- |
| `letsencrypt` (`le`) | Let's Encrypt (Production) | None |
| `letsencrypt_staging` | Let's Encrypt (Staging) | None |
| `zerossl` | ZeroSSL | Automatic via `email`, or explicit `eab <kid> <hmac_key>` |
| `google` (`gts`) | Google Trust Services | Requires `eab <kid> <hmac_key>` |
| `ssl.com` (`sslcom`, `ssl.com-rsa`) | SSL.com (ECC or RSA) | Requires `eab <kid> <hmac_key>` |
| `litessl` (`trustasia`) | LiteSSL by TrustAsia | None for standard DV |
| `buypass` | Buypass Go SSL | None |
| `local` | Local Two-Tier PKI | Local CA (`Neuro-sama Headquarters Root CA`) |
| `https://...` | Any RFC 8555 ACME v2 CA | Optional `eab <kid> <hmac_key>` (e.g. `step-ca`, Pebble) |

For **ZeroSSL**, setting `email` automatically fetches EAB credentials unless you pass `eab` yourself. For **Google Trust Services** and **SSL.com**, pass `eab YOUR_EAB_KEY_ID YOUR_EAB_HMAC_KEY` (both padded base64 and unpadded base64url work, and `eab_hmac_key` is redacted in Admin API output). ACME certs are cached per provider (`certs/acme-zerossl-fullchain.pem`, `certs/acme-fullchain.pem` for Let's Encrypt), so changing `ssl_provider` requests from the new CA right away.

You can also bring your own PEM files:
- `account_key /etc/gymbag/account.pem`: existing ECDSA (P-256/P-384) or RSA ACME account key.
- `ssl_key /etc/gymbag/domain.key`: existing ECDSA or RSA key for ACME CSR signing.
- `tls <cert> <key>`: externally managed certificates globally or per site (SNI), with `key_password` (`GYMBAG_KEY_PASSWORD`) if the private key is encrypted.

A background task checks certificates every `renew_interval` seconds (`43200` / 12h by default) and renews or reloads anything within `renew_within_days` (`30` days), swapping the `tls.Certificate` atomically without dropping active HTTP streams or WebSockets.

## Writing your own firewall rules

Protected HTTP requests pass through these checks in order:

1. Request sanity (HTTP verb, max URI length, null-byte and `..` / `%2e%2e` rejection)
2. ACME HTTP-01 (`/.well-known/acme-challenge/*`)
3. Local custom rules (global rules, then site rules, top to bottom; first match wins)
4. Remote rules (`remote_rules`, auto-refreshed over HTTP/HTTPS)
5. Built-in baseline rules (unless `default_rules off`)
6. Per-site per-IP rate limiter (`rate_limit` / `rate_window`)
7. IP threat intelligence (`ip_security_action verify` or `block`)
8. Proof-of-work check (`verify` `0`–`3`)

Because custom rules run before baseline rules, rate limits, and threat feeds, putting an `allow` at the top of a site block whitelists an office CIDR, health check, or preview cookie cleanly.

### Shorthand rules

Syntax: `<allow | verify | block> <field> <pattern...>` (aliases `bypass`/`pass`, `challenge`/`pow`, `deny`/`drop`/`reject` also work):

| Field | Values & Matching | Example |
| :--- | :--- | :--- |
| `ip` | IPv4, IPv6, or CIDRs | `allow ip 127.0.0.1 10.0.0.0/8 2001:db8::/32` |
| `user_agent` | Case-insensitive substring, `*` glob, or `~regex` | `block user_agent BadBot ~^(?i)python-urllib/` |
| `path` | Exact/substring, `*` glob, or `~regex` | `verify path /api/export* ~^/v[0-9]+/search` |
| `query` | Raw/decoded query match, `*` glob, or `~regex` | `block query "union select" ~(?i)<script` |
| `method` | HTTP methods (case-insensitive) | `verify method POST DELETE` |
| `header` | `<Header-Name> [pattern]` (`""` = missing/empty, `*` = present) | `verify header Accept-Language ""` |
| `cookie` | `<cookie_name> [pattern]` (`""` = missing/empty, `*` = present) | `allow cookie vip_session *` |

### Multi-condition `rule` blocks

Inside `rule "name" { ... }`, multiple values on the same field are OR'd and different fields are AND'd:

```caddyfile
app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    rule "challenge-auth-mutations" {
        action verify
        method POST
        path /login* /api/auth/* /reset-password*
    }

    rule "block-headless-api-posts" {
        action block
        method POST
        path /api/*
        header Origin ""
    }
}
```

### Remote rules and dynamic local rule files

Point `remote_rules` globally or per site at HTTP/HTTPS URLs serving Gymbagfile rule syntax or JSON (`[ ... ]` or `{"rules": [ ... ]}`). Validated payloads are cached under `<cert_dir>/rules-cache` with conditional `ETag` / `If-Modified-Since` checks, and malformed updates leave the previous ruleset active.

Local `.rules` or `.json` files and folders can also be loaded via `rules_file`, `rules_dir`, or `ruleset` blocks (or `gymbag ruleset add`). Mark a source `dynamic` to watch and hot-reload changes every `rules_watch_interval` (`2s` by default):

```caddyfile
{
    remote_rules https://rules.example.com/org-baseline.rules
    remote_rules_refresh 5m
    rules_watch_interval 2s
}

app.example.com {
    reverse_proxy http://localhost:3000
    rules_file ./rules/custom.rules dynamic
    rules_dir ./rules.d dynamic
}
```

`default_rules` stays `on` by default: when you add custom rules to a site, your rules run first and the built-in **block** rules stay appended afterward so adding one `verify path` rule doesn't accidentally expose `/.env`. Set `default_rules off` to turn built-in rules off completely.

## CLI, Docker Compose, environment variables, and REST APIs

### CLI reference

Gymbag loads `.env` from the working directory if present, then probes `./gymbag.json`, `./Gymbagfile`, and `./Caddyfile` (or `-c` with JSON, Gymbagfile, Caddyfile v2, `nginx.conf`, or Apache configs):

```bash
# Config generation (writes gymbag.json by default, or Gymbagfile with --format gymbagfile)
./gymbag init
./gymbag init -d app.example.com -u http://localhost:3000 -l 3 --rules-dir ./rules.d --rules-dynamic
./gymbag init -t ssh --verify-url https://verify.example.com:8443
./gymbag init -t minecraft --server-properties ./server.properties
./gymbag init -t http,ssh,minecraft -d app.example.com -u :3000 --server-properties ./server.properties
./gymbag init --format gymbagfile -o Gymbagfile -d app.example.com -u :3000

# Manage sites, routes, global settings, and convert configs
./gymbag config list
./gymbag config add -d api.example.com -u :8080 -l 1
./gymbag config update -d api.example.com -l 3 --verify-timeout 6h
./gymbag config route add -d api.example.com --path /static/* --strip-prefix /static --root ./dist
./gymbag config remove -d api.example.com
./gymbag config set admin 127.0.0.1:2019
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json

# Manage rule files, folders, and individual rules
./gymbag ruleset init ./rules/custom.rules
./gymbag ruleset add --source ./rules --dynamic --name local-dir
./gymbag ruleset update local-dir --static
./gymbag ruleset remove local-dir
./gymbag ruleset validate ./rules
./gymbag ruleset rule add --file ./rules/custom.rules --action block --name bad-bot --ua "*BadBot*"

# Run, watch, validate, format, or manage OS services
./gymbag
./gymbag run --config gymbag.json --watch
./gymbag service install --config gymbag.json
./gymbag service start
./gymbag service render --type systemd
./gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3
./gymbag validate --config gymbag.json
./gymbag fmt --overwrite Gymbagfile
./gymbag version
```

### Docker and Docker Compose

Multi-arch Alpine images (`linux/amd64`, `linux/arm64`) live at `ghcr.io/evil-project/gymbag`, running as UID `10001` with `cap_net_bind_service=+ep`.

```bash
# Quick docker run with a mounted Gymbagfile
docker run -d \
  --name gymbag \
  --restart unless-stopped \
  -p 80:80 -p 443:443 \
  --cap-add NET_BIND_SERVICE \
  --add-host=host.docker.internal:host-gateway \
  -v "$(pwd)/Gymbagfile:/app/Gymbagfile:ro" \
  -v gymbag-certs:/app/certs \
  ghcr.io/evil-project/gymbag:latest
```

`compose.yaml` pairs Gymbag with `redis:8-alpine` using Gymbag's pure-Go RESP2 Redis client so PoW challenge nonces, fixed-window rate limits, and SSH/Minecraft verification tickets and IP clearances are shared across restarts or replicas (`docker compose up -d --build`). When protecting SSH or Minecraft from Docker, publish `-p 22:22` / `-p 25565:25565` and `-p 8443:8443` as well. See the [Docker & Docker Compose Guide](docs/guide/docker.md) for bind-mount permissions (`chown 10001:10001`), `--watch`, and env-only setups.

### Environment variables (`GYMBAG_*`)

Precedence order is **config file -> `.env` / `GYMBAG_*` environment variables -> CLI flags**.

| Variable | Description |
| :--- | :--- |
| `GYMBAG_CONFIG` | Path to `gymbag.json`, `Gymbagfile`, `Caddyfile`, `nginx.conf`, or Apache config |
| `GYMBAG_BIND` | Bind IP (`127.0.0.1` for local-only HTTP, `0.0.0.0` when public domains, SSH, or Minecraft are active) |
| `GYMBAG_HTTP_PORT` / `GYMBAG_HTTPS_PORT` | HTTP and HTTPS listen ports (defaults `80` and `443`) |
| `GYMBAG_DOMAIN` / `GYMBAG_DOMAINS` | Comma-separated downstream domain(s) |
| `GYMBAG_UPSTREAM` | Default upstream target URL |
| `GYMBAG_UPSTREAMS` | Semicolon-separated host map: `app.example.com=http://web:3000;api.example.com=http://api:8080` |
| `GYMBAG_PROTECT` / `GYMBAG_PROTECT_DOMAINS` | Enable/disable WAF protection (`on`/`off`), or restrict protection to listed domains |
| `GYMBAG_VERIFY_LEVEL` | Verify level `0`–`3` or `off` (aliases: `GYMBAG_VERIFY`, `GYMBAG_VERIFY_MODE`, `GYMBAG_MODE`, `GYMBAG_TRIGGER`) |
| `GYMBAG_DEFAULT_RULES` | Built-in baseline rules (`on` / `off`, default `on`) |
| `GYMBAG_REMOTE_RULES` / `GYMBAG_REMOTE_RULES_REFRESH` | Remote firewall rule URLs and refresh interval (default `300`s / `5m`) |
| `GYMBAG_RULES_WATCH_INTERVAL` | Polling interval for `dynamic` local rule files/folders (default `2`s / `2s`) |
| `GYMBAG_DIFFICULTY` | SHA-256 PoW difficulty (`1`–`5`, default `4`) |
| `GYMBAG_CHALLENGE_TTL` / `GYMBAG_CLEARANCE_TTL` | Challenge expiry (`300`s / `5m`) and clearance lifetime (`86400`s / `24h`; aliases: `GYMBAG_VERIFY_TIMEOUT`, `GYMBAG_VERIFIED_TIMEOUT`) |
| `GYMBAG_COOKIE_NAME` / `GYMBAG_COOKIE_SECURE` | Clearance cookie name (`waf_clearance`) and `Secure` flag (`true`) |
| `GYMBAG_RATE_LIMIT` / `GYMBAG_RATE_WINDOW` | Per-IP request limit (`240`) and window (`60`s) |
| `GYMBAG_TLS_MODE` / `GYMBAG_SSL_PROVIDER` | TLS mode (`auto`, `acme`, `local`, `selfsigned`, `file`) and CA (`letsencrypt`, `letsencrypt_staging`, `zerossl`, `google`, `ssl.com`, `litessl`, `buypass`, `local`, or URL) |
| `GYMBAG_ACME_EMAIL` / `GYMBAG_ACME_DIRECTORY` | ACME contact email (enables ZeroSSL auto-EAB) and explicit ACME directory URL |
| `GYMBAG_EAB_KID` / `GYMBAG_EAB_HMAC_KEY` | External Account Binding Key ID and base64/base64url HMAC key |
| `GYMBAG_ACCOUNT_KEY_FILE` | Path to ECDSA/RSA ACME account key PEM |
| `GYMBAG_CERT_FILE` / `GYMBAG_KEY_FILE` / `GYMBAG_KEY_PASSWORD` | Cert, key, and encrypted key passphrase for `file` mode |
| `GYMBAG_CERT_DIR` | Directory for certs, CA keys, secret, intel cache, and rule cache (default `certs`) |
| `GYMBAG_RENEW_INTERVAL` / `GYMBAG_RENEW_WITHIN_DAYS` | Renewal check interval (`43200`s) and threshold (`30` days) |
| `GYMBAG_AUTO_HOSTS` / `GYMBAG_HSTS` | Automatic `/etc/hosts` management (`on`/`off`) and `Strict-Transport-Security` override (`on`/`off`) |
| `GYMBAG_IP_SECURITY` / `GYMBAG_IP_SECURITY_ACTION` | Threat feeds (`on`/`off`) and hit action (`verify` or `block`) |
| `GYMBAG_IP_REPUTATION` / `GYMBAG_AI_IP_LIST` | Live Shodan InternetDB + StopForumSpam lookups (`on`/`off`) and official AI crawler CIDR lists (`on`/`off`) |
| `GYMBAG_VPN_DETECTION` | Free VPN/proxy/Tor detection in suspicious checks (`on`/`off`, default `on`) |
| `GYMBAG_INTEL_REFRESH` | Threat, VPN, and AI feed refresh interval (default `21600`s / `6h`) |
| `GYMBAG_VERIFY_URL` / `GYMBAG_VERIFY_PORT` | Shared HTTPS verify portal URL (blank = server IP + IP cert) and port (default `8443`) |
| `GYMBAG_SSH_ENABLED` / `GYMBAG_SSH_PORT` / `GYMBAG_SSH_UPSTREAM` | Enable SSH protection (default port `22`) and backend `sshd` target |
| `GYMBAG_SSH_VERIFY_LEVEL` / `GYMBAG_SSH_PROTECT` | SSH verify level (`0`–`3` or `off`) and protection toggle (`on`/`off`) |
| `GYMBAG_MC_ENABLED` / `GYMBAG_MC_PORT` / `GYMBAG_MC_UPSTREAM` | Enable Minecraft protection (default port `25565`) and backend server target (`GYMBAG_MINECRAFT_*` aliases work) |
| `GYMBAG_MC_SERVER_JAR` / `GYMBAG_MC_JAVA` | Path to Minecraft `server.jar` (auto-starts server with auto-selected Java) and optional Java binary override |
| `GYMBAG_MC_JVM_ARGS` / `GYMBAG_MC_WORK_DIR` / `GYMBAG_MC_AUTO_RESTART` | Space-separated JVM flags, Minecraft working directory, and crash auto-restart toggle (`true`/`false`, default `true`) |
| `GYMBAG_MC_VERIFY_LEVEL` / `GYMBAG_MC_PROTECT` | Minecraft verify level (`0`–`3` or `off`) and protection toggle (`on`/`off`) |
| `GYMBAG_REDIS_URL` / `GYMBAG_SECRET` / `GYMBAG_ADMIN` | Redis URL (`redis://` or `rediss://`), HMAC secret, and Admin API listen address (e.g. `127.0.0.1:2019` or `off`) |

### REST APIs

**Public WAF & Verification Portal endpoints:**

| Method | Path | Description |
| :--- | :--- | :--- |
| `GET` | `/.waf/v1/session` | Clearance state (`cleared`), `protected`, `verify_level`, `verify_mode`, `clearance_ttl`, `expires_at`, `expires_in` |
| `DELETE` | `/.waf/v1/session` | Clears visitor's clearance cookie (`204 No Content`) |
| `POST` | `/.waf/v1/challenges` | Issues a single-use SHA-256 PoW challenge (`201 Created`) |
| `POST` | `/.waf/v1/verifications` | Verifies PoW solution and sets signed clearance cookie (`/.waf/verify` alias supported) |
| `GET` | `/v/{token}` | Serves the SSH/Minecraft verification PoW page (`/verify/{token}` alias supported) |
| `POST` | `/.waf/gate/verify` | Verifies SSH/Minecraft PoW solution and stores IP clearance |
| `GET` | `/.waf/assets/{name}` | Serves embedded challenge PNG assets (`/.waf/v1/assets/{name}` alias, cached via `ETag`) |

**Local Admin API (`admin 127.0.0.1:2019` or `--admin`):**

Keep the Admin API bound to loopback (`127.0.0.1:2019`) or a private interface — it takes no auth token and mutates live routing and firewall state.

| Method | Path | Description |
| :--- | :--- | :--- |
| `GET` | `/api/v1/health` | Status, `sites_count`, `ssh_enabled`, `minecraft_enabled`, `tls_source`, `threat_entries`, `ai_entries`, `vpn_entries`, `remote_rules` |
| `GET` / `POST` | `/api/v1/tls` / `/api/v1/tls/renew` | Active TLS status & CA catalog; force cert renewal and atomic swap |
| `GET` / `POST` | `/api/v1/intel` / `/api/v1/intel/refresh` | Per-feed threat/AI/VPN counts and cache sizes; trigger immediate refresh |
| `GET` / `POST` | `/api/v1/rules/remote` / `/api/v1/rules/remote/refresh` | Remote rule counts and fetch errors; force refresh and WAF reload |
| `GET` / `PUT` | `/api/v1/config` | Dump redacted config; validate and replace active runtime config |
| `GET` / `POST` | `/api/v1/sites` | List sites or add a site at runtime |
| `GET` / `PUT` / `PATCH` / `DELETE` | `/api/v1/sites/{domain}` | Inspect, replace, live-patch (`verify_level`, `protect`, `difficulty`, `clearance_ttl`, `upstreams`, `routes`, `headers`, `rules`, `remote_rules`, `rule_sources`), or delete a site |
| `GET` / `POST` / `PUT` / `DELETE` | `/api/v1/sites/{domain}/rules` & `.../{index_or_name}` | List, append, replace, or remove custom rules on a site |

## Benchmarks against Caddy and Nginx

Measured on Apple M4 (10-core, macOS, Go `1.27.0`), TLS 1.3 with the same ECDSA P-256 cert, 64 concurrent workers, `1 KiB` response body, `1024` upstream keep-alive connections, `0` errors:

| Server & Mode | HTTP/1.1 TLS (RPS) | H1 p50 / p99 | HTTP/2 TLS (RPS) | H2 p50 / p99 | Binary Size | RSS (Under Load) |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **85,203** | 0.45 / 6.20 ms | **88,775** | 0.65 / 1.96 ms | 1.5 MB (+libs) | 5.8 MiB |
| **Gymbag `v0.1.0`** (`verify 3`, full WAF active) | **72,517** | 0.74 / 3.14 ms | **66,096** | 0.82 / 3.48 ms | 12.9 MB (static) | 54.3 MiB |
| **Gymbag `v0.1.0`** (`protect off`, pure proxy) | **72,479** | 0.74 / 3.15 ms | **64,048** | 0.83 / 3.94 ms | 12.9 MB (static) | 52.1 MiB |
| **Gymbag `v0.1.0`** (`verify 0`, cleared cookie) | **65,490** | 0.79 / 3.93 ms | **62,108** | 0.84 / 4.27 ms | 12.9 MB (static) | ~54 MiB |
| **Caddy `v2.11.9`** (Go, tuned keep-alive) | **54,741** | 0.90 / 4.90 ms | **39,207** | 1.30 / 6.18 ms | 49.0 MB | 76.8 MiB |

Nginx in C is still faster in raw throughput, though Gymbag keeps lower p99 latency (`3.14 ms` vs `6.20 ms`) on HTTP/1.1 while running the WAF inline, and outpaces Caddy (`+32%` on HTTP/1.1, `+63%` to `+69%` on HTTP/2) with roughly 30% less memory and a 3.8x smaller binary. Known exploit probes like `/.env` drop at the edge over TLS at **140,680 req/s** (`0.35 ms` p50) without hitting the upstream.

Methodology, `go test -bench` microbenchmarks, and exact configs are in [Benchmarks & Performance](docs/guide/benchmarks.md).

## Project layout

```text
.
├── cmd/gymbag/          # CLI entrypoint
├── docs/                # Multilingual VitePress docs (EN, 简体中文, 日本語, 한국어, Español)
├── internal/
│   ├── cli/             # Subcommands (init, config, ruleset, service, run, reverse-proxy, validate, fmt, version), .env & --watch
│   ├── config/          # Config types, Gymbagfile/Caddyfile v2/nginx/Apache parsers, duration & rule sources, GYMBAG_* env loader
│   ├── host/            # Domain normalization, IPv6 brackets, wildcard matching
│   ├── intel/           # O(1) prefix set, auto-updating threat/VPN/AI feeds, live IP & VPN reputation
│   ├── localpki/        # Two-tier local CA ("Neuro-sama Headquarters Root CA") & /etc/hosts manager
│   ├── proxy/           # Lock-free multi-site & path router, pooled HTTP & RFC 6455 WebSocket proxies
│   ├── server/          # Fused HTTPS gateway, HTTP->HTTPS/ACME redirector, Admin REST API
│   ├── service/         # System service renderer & manager (systemd, launchd, openrc, supervisor)
│   ├── setup/           # Automated sshd_config/firewall setup and Minecraft server.properties port randomization
│   ├── store/           # 64-shard in-memory store, pure-Go RESP2 Redis client (challenges, rate limits, gate tickets/clearances)
│   ├── tcpguard/        # SSH & Minecraft Java TCP protection proxies, policy engine, shared HTTPS verify portal
│   ├── tlsmanager/      # Multi-CA ACME v2 (RFC 8738 IP certs, EAB & ZeroSSL auto-EAB), local PKI, atomic SNI cert manager
│   └── waf/             # PoW verifier, sharded HMAC clearance cache, Aho-Corasick DFA, remote/local rules, embedded UI
├── nix/                 # NixOS module (services.gymbag)
├── packaging/           # Arch Linux PKGBUILD & Debian/Ubuntu .deb maintainer scripts/unit
├── Dockerfile
├── compose.yaml
├── flake.nix
├── Gymbagfile
└── go.mod
```

## Star history

<a href="https://www.star-history.com/#Evil-Project/gymbag&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
 </picture>
</a>
