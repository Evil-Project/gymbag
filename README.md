# Gymbag

<p align="center">
  <img src="docs/public/mascot-happy.png" alt="Gymbag mascot" width="140" />
</p>

<p align="center">
  <strong>Single-binary HTTPS Reverse Proxy &amp; Proof-of-Work WAF in pure Go</strong>
</p>

<p align="center">
  <a href="https://github.com/Evil-Project/gymbag/releases"><img src="https://img.shields.io/github/v/release/Evil-Project/gymbag?include_prereleases&label=Release" alt="Release" /></a>
  <a href="https://github.com/Evil-Project/gymbag/pkgs/container/gymbag"><img src="https://img.shields.io/badge/Docker-GHCR-blue?logo=docker" alt="Docker GHCR" /></a>
  <a href="go.mod"><img src="https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go" alt="Go 1.27+" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache--2.0-green" alt="Apache-2.0 License" /></a>
  <a href="https://evil-project.github.io/gymbag/"><img src="https://img.shields.io/badge/Docs-VitePress-b81d33" alt="Docs" /></a>
</p>

<p align="center">
  <strong>Docs Languages:</strong>
  <a href="https://evil-project.github.io/gymbag/">English</a> ·
  <a href="https://evil-project.github.io/gymbag/zh/">简体中文</a> ·
  <a href="https://evil-project.github.io/gymbag/ja/">日本語</a> ·
  <a href="https://evil-project.github.io/gymbag/ko/">한국어</a> ·
  <a href="https://evil-project.github.io/gymbag/es/">Español</a>
  <br />
  <sub>Repository Markdown: <a href="docs/index.md">EN</a> · <a href="docs/zh/index.md">ZH</a> · <a href="docs/ja/index.md">JA</a> · <a href="docs/ko/index.md">KO</a> · <a href="docs/es/index.md">ES</a></sub>
</p>

Gymbag is a single-binary HTTPS reverse proxy with a built-in, Anubis-style proof-of-work Web Application Firewall. It is written in pure Go using only the standard library — zero external Go dependencies, and every HTML template, SHA-256 browser solver script, and mascot PNG is compiled directly into the executable via `go:embed`.

If you self-host web apps, Gitea/Forgejo instances, documentation sites, or internal tools, you have probably run into the same problem that inspired Anubis: aggressive LLM scrapers ignoring `robots.txt` and vulnerability scanners hammering `/.env` and `/wp-admin` around the clock. Instead of stitching together Nginx/Caddy, a separate PoW sidecar container, fail2ban, and hand-curated CIDR lists, Gymbag puts TLS termination, multi-CA ACME, HTTP/WebSocket proxying, local & remote firewall rules, IP threat intelligence, and per-site PoW policies into one binary configured by a clean `Gymbagfile`.

---

## Quick Start (Open-Box Protection)

Download a prebuilt static binary from [GitHub Releases](https://github.com/Evil-Project/gymbag/releases) (Linux/macOS/Windows, plus `.pkg.tar.zst` for Arch Linux), install via Nix Flakes, or build from source:

```bash
go build -o gymbag ./cmd/gymbag
```

**Arch Linux:**
```bash
# Install prebuilt release package (.pkg.tar.zst)
sudo pacman -U gymbag-0.1.0-1-x86_64.pkg.tar.zst
# Or build locally with makepkg
cd packaging/arch && makepkg -si
```

**Nix / NixOS:**
```bash
# Run or install via flake
nix run github:Evil-Project/gymbag -- --help
nix profile install github:Evil-Project/gymbag
```
Static `CGO_ENABLED=0` Linux release binaries also execute directly on NixOS without `patchelf` or `nix-ld`, and `nixosModules.default` provides declarative `services.gymbag` configuration.

Get started in **two commands** — generate `Gymbagfile` and start the server:

```bash
# 1. Generate a config (interactive wizard, or pass flags for one-liner setup)
./gymbag init -d app.example.com -u http://localhost:3000

# 2. Start the server
./gymbag
```

Or register it as a system service (`systemd`, `launchd`, `openrc`, or `supervisor`) and start it:

```bash
./gymbag init -d app.example.com -u :3000 --service auto
./gymbag service start
```

Or skip the config file altogether for quick ad-hoc proxying:

```bash
./gymbag reverse-proxy --from app.internal --to :3000 --verify 3
```

### What "Open-Box" Means in Practice

You do not need to write a single regex or import any blocklists before putting Gymbag on the public internet. Writing bare `verify` (which turns on verify level `3`: `suspicious & ai`) or bare `block` (which rejects threat-intel IPs with `403 Forbidden` outright instead of challenging them) immediately activates three layers of protection:

1. **Built-in baseline firewall rules (`default_rules on`)**
   - **Sensitive files (`403`)**: Blocks requests for `/.env`, `/.env.*`, `/.git/*`, `/.svn/*`, `/.hg/*`, `/.DS_Store`, `/.aws/*`, `/.ssh/*`, `/docker-compose.yml`, `/docker-compose.yaml`, `*.bak`, `*.sql`, and `*.sqlite`.
   - **CMS & framework exploit probes (`403`)**: Drops `/wp-admin/*`, `/wp-login.php`, `/xmlrpc.php`, `/phpmyadmin*`, `/vendor/phpunit*`, `/cgi-bin/*`, `/actuator/env*`, and `/actuator/heapdump*`.
   - **Vulnerability scanners (`403`)**: Rejects `sqlmap`, `nikto`, `nuclei`, `masscan`, `zgrab`, `gobuster`, `dirbuster`, `ffuf`, `wfuzz`, `feroxbuster`, `acunetix`, `nessus`, and `openvas`.
   - **Injection & traversal queries (`403`)**: Catches `UNION SELECT`, `<script`, `javascript:`, `/etc/passwd`, `information_schema`, `../`, `cmd.exe`, and `/bin/sh` in query strings (on top of strict request-shape checks against null bytes and `%2e%2e` path traversal).
   - **Automation clients (`PoW challenge`)**: When you haven't defined custom rules yet, CLI HTTP clients (`curl`, `wget`, `python-requests`, `python-httpx`, `go-http-client`, `scrapy`), headless browsers (`headlesschrome`, `playwright`, `puppeteer`, `selenium`), and empty `User-Agent` headers are sent to the proof-of-work screen.
2. **Auto-updating IP threat intelligence**
   - Pulls [IPsum (level 3+)](https://raw.githubusercontent.com/stamparm/ipsum/master/levels/3.txt), [Spamhaus DROP IPv4](https://www.spamhaus.org/drop/drop_v4.json) & [IPv6](https://www.spamhaus.org/drop/dropv6.txt), and [abuse.ch Feodo Tracker](https://feodotracker.abuse.ch/downloads/ipblocklist.txt) on startup and every 6 hours in the background, caching feeds in `<cert_dir>/intel-cache` with conditional `ETag` / `If-Modified-Since` headers.
   - Runs keyless live IP reputation lookups against **Shodan InternetDB** (`internetdb.shodan.io`) and **StopForumSpam** (`api.stopforumspam.org`) with in-memory TTL caching and fail-open timeouts so an upstream outage never stalls your traffic.
3. **Official AI crawler IP tracking**
   - Tracks published CIDR lists from **OpenAI** (`gptbot`, `searchbot`, `chatgpt-user`), **Anthropic** (`claude.com/crawling/bots.json`), **Perplexity** (`perplexitybot`, `perplexity-user`), **Google** (`user-triggered-agents`, `user-triggered-fetchers-google`), and **Apple** (`applebot`). Even if a crawler spoofs a normal desktop Chrome `User-Agent`, its source IP still matches the AI range.

> **Operational note on ports 80 and 443:** By default Gymbag binds `:80` (HTTP-to-HTTPS redirect + ACME HTTP-01 responder) and `:443` (HTTPS gateway). On Linux, grant the binary `CAP_NET_BIND_SERVICE` so it never has to run as root:
> ```bash
> sudo setcap 'cap_net_bind_service=+ep' ./gymbag
> ```
> For unprivileged local testing without capabilities or `sudo`, pass `--http-port 8080 --https-port 8443` (or `http_port` / `https_port` in `Gymbagfile`).

---

## Local Development (`localhost` & `*.internal`)

Testing HTTPS locally with custom hostnames usually means juggling `mkcert`, manual `/etc/hosts` edits, and self-signed browser warnings. Gymbag handles both hostname resolution and local X.509 issuance out of the box whenever a site uses `localhost`, `*.localhost`, `*.internal` (such as `app.internal` or `api.internal`), `*.local`, or `*.test`:

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

### Automatic `/etc/hosts` Setup & Cleanup

When `auto_hosts` is enabled (the default), Gymbag inspects `/etc/hosts` on startup (or `C:\Windows\System32\drivers\etc\hosts` on Windows). For any concrete local domain like `app.internal` or `api.internal` that doesn't already resolve in `/etc/hosts`, Gymbag appends IPv4 and IPv6 loopback entries tagged with its instance ID:

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

Already-present `localhost` lines are left untouched. When you stop Gymbag (`Ctrl+C` / `SIGTERM`), it re-reads `/etc/hosts` so any edits you made while it was running are preserved, and removes **only** its own `# gymbag-managed:<instanceID>` lines.

- **Permissions on macOS & Linux:** Writing `/etc/hosts` requires root privileges. If you start `./gymbag` as a normal user without write permission on `/etc/hosts`, Gymbag logs a clear warning and **continues running normally** — it will not crash. Run `sudo ./gymbag` when you want automatic `/etc/hosts` management (and ports `80`/`443`), or set `auto_hosts off` (or `GYMBAG_AUTO_HOSTS=off`) if you resolve `.internal` via `dnsmasq` or manage `/etc/hosts` yourself.

### Two-Tier Local PKI & Trusting the Root CA

Instead of generating disposable self-signed leaf certs that browsers refuse to cache trust for, Gymbag maintains a two-tier ECDSA P-256 Certificate Authority inside `<cert_dir>/ca/` (default `certs/ca/`):

1. **Root CA**: **`"Neuro-sama Headquarters Root CA"`** (`certs/ca/neuro-sama-root-ca.pem`, 10-year validity, `MaxPathLen: 1`)
2. **Intermediate CA**: **`"Gymbag & Swarm SSL Local CA"`** (`certs/ca/gymbag-swarm-local-ca.pem`, 5-year validity, signed by `"Neuro-sama Headquarters Root CA"`, `MaxPathLen: 0`)
3. **Leaf Certificate**: `certs/local-fullchain.pem` + `certs/local-privkey.pem` (90-day validity, auto-renewed within 30 days, serving the leaf + intermediate chain with DNS SANs and automatic `127.0.0.1` / `::1` IP SANs)

Trust **`"Neuro-sama Headquarters Root CA"`** once in your OS trust store and every `https://localhost` and `https://*.internal` domain served by Gymbag will show a clean green lock in browsers and `curl`:

**macOS (System Keychain):**
```bash
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain certs/ca/neuro-sama-root-ca.pem
```

**Debian / Ubuntu:**
```bash
sudo cp certs/ca/neuro-sama-root-ca.pem /usr/local/share/ca-certificates/neuro-sama-root-ca.crt && sudo update-ca-certificates
```

**Fedora / RHEL / Arch Linux:**
```bash
sudo trust anchor --store certs/ca/neuro-sama-root-ca.pem
```

> **Firefox note:** On macOS and Windows, Firefox automatically trusts OS roots when `security.enterprise_roots.enabled` is `true` in `about:config`. On Linux, if Firefox uses its own NSS database rather than p11-kit, either toggle `security.enterprise_roots.enabled` or import `certs/ca/neuro-sama-root-ca.pem` once under *Settings -> Privacy & Security -> Certificates -> View Certificates -> Authorities*.

---

## Protecting Multiple Websites & Verify Levels (`0`–`3`)

A single Gymbag process can host and reverse-proxy dozens of domains at once, including multi-upstream round-robin load balancing and full-duplex RFC 6455 WebSocket upgrades. Each site chooses its own verification policy with `verify <0-3>` (or disables WAF challenges altogether with `protect off`):

| Level | Name | Who Gets Challenged | Best Fit |
| :---: | :--- | :--- | :--- |
| `0` | `always` | Every visitor without a valid HMAC clearance cookie. | Private tools, staging servers, or sites under active L7 flood. |
| `1` | `only suspicious` | Threat-feed / bad-reputation IPs, scanners, CLI HTTP libraries, headless browsers, missing/spoofed browser headers, and exploit probes. | Public web apps and APIs where normal browsers shouldn't see a challenge screen. |
| `2` | `only ai` | AI crawlers and LLM agents matched by **official AI crawler IP ranges** *or* AI bot `User-Agent` / agent headers. | Docs, blogs, and Git forges that want zero friction for humans and `curl` users, while taxing LLM scrapers. |
| `3` | `suspicious & ai` | Combines `1` and `2`: challenges both suspicious clients/IPs and AI crawlers; normal human browsers pass straight through. | Recommended default for public production websites (also enabled by bare `verify`). |
| — | `protect off` | Proof-of-work challenges disabled; acts as a straight HTTPS & WebSocket reverse proxy (explicit custom rules on the site still run if configured). | Static asset hosts, webhooks, and public package mirrors. |

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
}

static.example.com {
    reverse_proxy http://localhost:9000
    protect off
}
```

Once a browser solves a SHA-256 proof-of-work challenge (`difficulty` `1`–`5`, default `4`), Gymbag sets an `HttpOnly`, `SameSite=Lax` HMAC-signed clearance cookie (`waf_clearance`, default TTL `24h` / `86400s`) bound to both the host and client IP.

### Custom Verified Status Timeout (`verify_timeout` / `clearance_ttl`)

Customize how long a visitor remains verified globally or per site using integer seconds **or** human-readable durations (`30m`, `12h`, `1h30m`, `7d`, `2w`):

```caddyfile
{
    verify_timeout 12h     # Global default (aliases: verified_timeout, clearance_ttl)
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
    verify_timeout 30m     # Per-site override
}
```

You can also pass `--verify-timeout 6h` (or `--clearance-ttl 6h`) to `gymbag init`, `gymbag run`, or `gymbag reverse-proxy`, set `GYMBAG_VERIFY_TIMEOUT` / `GYMBAG_CLEARANCE_TTL`, or live-patch `clearance_ttl` via `PATCH /api/v1/sites/{domain}`. `GET /.waf/v1/session` reports `clearance_ttl`, `expires_at`, and remaining `expires_in` seconds for cleared visitors.

---

## Switching SSL Providers, Custom Keys, & Auto-Renewal

For public domains, `tls auto` (the default) automatically obtains certificates over RFC 8555 ACME HTTP-01 using Let's Encrypt. You can switch Certificate Authorities with a single `ssl_provider` directive (aliases: `acme_provider`, `ca`, `cert_issuer`) or `tls <provider>`:

| Provider Keyword | Certificate Authority | EAB Requirement |
| :--- | :--- | :--- |
| `letsencrypt` (`le`) | Let's Encrypt (Production) | None |
| `letsencrypt_staging` | Let's Encrypt (Staging) | None |
| `zerossl` | ZeroSSL | **Automatic** when `email` is set, or explicit `eab <kid> <hmac_key>` |
| `google` (`gts`) | Google Trust Services | Requires `eab <kid> <hmac_key>` |
| `ssl.com` (`sslcom`, `ssl.com-rsa`) | SSL.com (ECC or RSA) | Requires `eab <kid> <hmac_key>` |
| `litessl` (`trustasia`) | LiteSSL by TrustAsia | None for standard DV (or override URL via `acme_directory`) |
| `buypass` | Buypass Go SSL | None |
| `local` | Local Two-Tier PKI | Local CA (`Neuro-sama Headquarters Root CA` -> `Gymbag & Swarm SSL Local CA`) |
| `https://...` | Any RFC 8555 ACME v2 CA | Optional `eab <kid> <hmac_key>` (e.g. Smallstep `step-ca`, Pebble) |

### Switching ACME CAs & External Account Binding (EAB)

**ZeroSSL with zero-click email EAB:**
When you select `zerossl` and configure `email`, Gymbag automatically exchanges your email with ZeroSSL's EAB endpoint (`https://api.zerossl.com/acme/eab-credentials-email`) if you haven't supplied explicit EAB keys:

```caddyfile
{
    email ops@example.com
    ssl_provider zerossl
}

example.com {
    reverse_proxy http://localhost:3000
    verify 3
}
```

**Google Trust Services, SSL.com, or explicit EAB credentials:**
Pass your Key ID and HMAC key (both URL-safe unpadded base64 and standard padded base64 are accepted, and `eab_hmac_key` is automatically masked in Admin API outputs):

```caddyfile
{
    email ops@example.com
    ssl_provider google
    eab YOUR_EAB_KEY_ID YOUR_EAB_HMAC_KEY
}
```

**LiteSSL by TrustAsia or a custom ACME directory:**

```caddyfile
{
    email ops@example.com
    ssl_provider litessl
    # Or point directly at any ACME v2 directory URL:
    # acme_directory https://ca.internal.corp/acme/acme/directory
}
```

Gymbag scopes cached ACME certificates on disk by provider name (for example `certs/acme-zerossl-fullchain.pem` and `certs/acme-google-fullchain.pem`, while keeping `certs/acme-fullchain.pem` for Let's Encrypt). If you change `ssl_provider` and restart or reload, Gymbag immediately requests a certificate from the new CA rather than serving a leftover cert from the previous issuer.

### Bringing Your Own Keys or Certificates

- **Custom ACME account key (`account_key`)**: By default Gymbag creates and reuses an ECDSA P-256 account key at `certs/acme-account.pem`. If your organization already has a registered ECDSA (P-256/P-384) or RSA ACME account key, point to it with `account_key /etc/gymbag/account.pem` (or `GYMBAG_ACCOUNT_KEY_FILE`).
- **Custom CSR private key (`ssl_key`)**: Use `ssl_key /etc/gymbag/domain.key` to sign ACME CSRs with your own ECDSA or RSA private key instead of generating a fresh one.
- **Custom TLS certificate & key (`tls <cert> <key>`)**: Supply commercial, Cloudflare Origin, or externally managed PEM files either globally or per-site (routed via SNI). Encrypted private keys are supported via `key_password` (or `GYMBAG_KEY_PASSWORD`):

```caddyfile
{
    key_password super-secret-passphrase
}

corp.example.com {
    reverse_proxy http://localhost:8080
    tls /etc/ssl/corp-fullchain.pem /etc/ssl/corp-privkey.pem
    verify 1
}
```

### Zero-Downtime Background Renewal

Gymbag runs a background renewal loop every `renew_interval` seconds (default `43200` = 12 hours). Whenever an ACME certificate, local PKI leaf certificate, or file-backed certificate is within `renew_within_days` of expiration (default `30` days), Gymbag renews or reloads it in the background and swaps the active `tls.Certificate` pointer atomically. Active HTTP streams and WebSocket tunnels stay up without a single dropped TCP connection.

---

## Writing Your Own Firewall Rules

For each incoming request on a protected site, Gymbag evaluates layers in a predictable order:

1. **Request shape sanity** (HTTP verb check, max URI length, null-byte and `..` traversal rejection)
2. **ACME HTTP-01 bypass** (`/.well-known/acme-challenge/*`)
3. **Your local custom rules** (global rules first, then site rules, top-to-bottom — first match wins)
4. **Remote rules (`remote_rules`)** (auto-refreshed HTTP/HTTPS rulesets in Gymbagfile syntax or JSON)
5. **Built-in baseline security rules** (unless `default_rules off` is set)
6. **Per-site per-IP rate limiter** (`rate_limit` / `rate_window`)
7. **IP threat intelligence** (`ip_security_action verify` or `block`)
8. **Proof-of-work verification** (`verify` level `0`–`3`)

Because your custom rules run **before** the built-in baseline rules, rate limiter, and threat feeds, putting an `allow` rule at the top of a site block cleanly whitelists trusted office CIDRs, internal health checks, or preview cookies.

### One-Liner Shorthand Rules

Syntax: `<allow | verify | block> <field> <pattern...>` (aliases `bypass`/`pass`, `challenge`/`pow`, and `deny`/`drop` also work).

| Field | Accepted Values & Pattern Matching | Example |
| :--- | :--- | :--- |
| `ip` | IPv4, IPv6, or CIDR ranges (space- or comma-separated) | `allow ip 127.0.0.1 10.0.0.0/8 2001:db8::/32` |
| `user_agent` | Case-insensitive substring, `*` glob wildcard, or `~regex` | `block user_agent BadBot ~^(?i)python-urllib/` |
| `path` | Exact/substring, `*` glob wildcard, or `~regex` | `verify path /api/export* ~^/v[0-9]+/search` |
| `query` | Raw query string match, `*` wildcard, or `~regex` | `block query "union select" ~(?i)<script` |
| `method` | HTTP verbs (case-insensitive) | `verify method POST DELETE` |
| `header` | `<Header-Name> [pattern]` (`""` = missing/empty, `*` = present) | `verify header Accept-Language ""` |
| `cookie` | `<cookie_name> [pattern]` (`""` = missing/empty, `*` = present) | `allow cookie vip_session *` |

### Multi-Condition `rule` Blocks

When a single field isn't specific enough, use a `rule "name" { ... }` block. Multiple values on the **same** field match via **OR**; different fields inside the same rule block match via **AND**:

```caddyfile
app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    # Require PoW on POST requests hitting login or password-reset endpoints
    rule "challenge-auth-mutations" {
        action verify
        method POST
        path /login* /api/auth/* /reset-password*
    }

    # Block POST requests to /api/* that omit the Origin header from external IPs
    rule "block-headless-api-posts" {
        action block
        method POST
        path /api/*
        header Origin ""
    }
}
```

### Remote Firewall Rules (`remote_rules`)

Pull shared firewall rules from one or more HTTP/HTTPS URLs globally or per site. Gymbag supports both **Gymbagfile rule syntax** (`allow`/`verify`/`block` one-liners and `rule "name" { ... }` blocks) and **JSON** (`[ ... ]` or `{"rules": [ ... ]}`), caches validated payloads under `<cert_dir>/rules-cache` with conditional `ETag` / `If-Modified-Since` requests, and retains the last known good ruleset if a remote source is unreachable or malformed:

```caddyfile
{
    remote_rules https://rules.example.com/org-baseline.rules
    remote_rules_refresh 5m
}

app.example.com {
    reverse_proxy http://localhost:3000
    verify 3
    allow ip 10.0.0.0/8                              # Local rules always run first
    remote_rules https://rules.example.com/app.json  # Site-specific remote rules
}
```

Inspect or trigger an immediate refresh via `GET /api/v1/rules/remote` and `POST /api/v1/rules/remote/refresh`, or pass `--remote-rules` / `GYMBAG_REMOTE_RULES`.

### Controlling Built-In Rules (`default_rules on|off`)

`default_rules` is `on` by default:
- When a protected site has **no** custom rules, Gymbag enforces both the built-in block presets (`/.env`, `/wp-admin`, scanners, SQLi/XSS queries) and the built-in automation verify presets.
- When you add your own custom rules, your rules run first and the built-in **block** presets remain appended after them so you don't accidentally expose `/.git` or `/.env` just because you added one `verify path` rule.
- If you want complete manual control with zero built-in rules on a site (or globally), set `default_rules off` (or `GYMBAG_DEFAULT_RULES=off`).

---

## CLI, Docker Compose, Environment Variables, & REST APIs

### CLI Reference

Gymbag automatically loads `.env` from the working directory if present, then resolves `./Gymbagfile` or `./Caddyfile` unless overridden by flags:

```bash
# Generate Gymbagfile via interactive wizard or flags
./gymbag init
./gymbag init -d app.example.com -u http://localhost:3000 -l 3

# Start server using ./Gymbagfile or ./Caddyfile
./gymbag

# Start with an explicit config file and hot-reload on file save (polls every 2s)
./gymbag run --config Gymbagfile --watch

# Register & manage a system service (systemd, launchd, openrc, supervisor)
./gymbag service install --config Gymbagfile
./gymbag service start
./gymbag service status
./gymbag service render --type systemd   # Preview a systemd unit on any OS

# Ad-hoc reverse proxy without a config file
./gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3

# Validate syntax and print the resolved per-site security summary
./gymbag validate --config Gymbagfile

# Format a Gymbagfile into canonical 4-space layout in place
./gymbag fmt --overwrite Gymbagfile

# Print version
./gymbag version
```

| Flag | Description |
| :--- | :--- |
| `-c`, `--config <path>` | Path to `Gymbagfile`, `Caddyfile`, or JSON config file |
| `-d`, `--domain <list>` | Override downstream domain(s), comma-separated |
| `-u`, `--upstream <url>` | Override upstream URL (e.g. `http://localhost:3000` or `:3000`) |
| `-l`, `--verify <0-3>` | Override verify level (`0`, `1`, `2`, `3`, or `off`) |
| `--verify-timeout <dur>` | Override verified clearance timeout (e.g. `30m`, `12h`, `7d`, or seconds; alias `--clearance-ttl`) |
| `--remote-rules <url>` | Attach remote firewall rules URL (http/https, repeatable or comma-separated) |
| `--remote-rules-refresh <dur>` | Refresh interval for remote rules (default `5m` / `300s`) |
| `--no-protect` | Disable WAF verification for CLI-configured sites |
| `--ssl-provider <name>` | Override ACME / SSL provider (e.g. `letsencrypt`, `zerossl`, `google`, `litessl`, `local`) |
| `--admin <addr>` | Enable local REST Admin API (e.g. `127.0.0.1:2019` or `off`) |
| `--http-port <port>` | HTTP redirect & ACME HTTP-01 port (default `80`) |
| `--https-port <port>` | HTTPS gateway port (default `443`) |
| `--watch` | Watch config file and hot-reload site routes, rules, & SNI certs |

### Docker & Docker Compose

Official multi-arch (`linux/amd64`, `linux/arm64`) Alpine container images are published to GitHub Container Registry (`ghcr.io/evil-project/gymbag`) as non-root UID `10001` with `cap_net_bind_service=+ep`.

**Quick `docker run` with a mounted `Gymbagfile`:**

```bash
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

**Zero-config `docker run` using environment variables:**

```bash
docker run -d \
  --name gymbag \
  -p 80:80 -p 443:443 \
  --cap-add NET_BIND_SERVICE \
  --add-host=host.docker.internal:host-gateway \
  -e GYMBAG_DOMAIN=app.example.com \
  -e GYMBAG_UPSTREAM=http://host.docker.internal:3000 \
  -e GYMBAG_ACME_EMAIL=ops@example.com \
  -e GYMBAG_VERIFY_LEVEL=3 \
  -e GYMBAG_VERIFY_TIMEOUT=12h \
  -e GYMBAG_REMOTE_RULES=https://rules.example.com/baseline.rules \
  -v gymbag-certs:/app/certs \
  ghcr.io/evil-project/gymbag:latest
```

**Docker Compose with Redis:**

The included `compose.yaml` runs Gymbag alongside `redis:8-alpine` using Gymbag's pure-Go RESP2 Redis client so single-use PoW challenge nonces and fixed-window rate limits are shared across container restarts or multiple gateway replicas:

```bash
docker compose up -d --build
```

Configuration comes from `./Gymbagfile` (mounted read-only at `/app/Gymbagfile`), and certificates, secrets, threat-intel caches, and remote-rule caches persist in the `gymbag-certs` named volume. Any `GYMBAG_*` variable in a local `.env` file overrides the file config cleanly:

```dotenv
GYMBAG_ACME_EMAIL=ops@example.com
GYMBAG_SSL_PROVIDER=zerossl
GYMBAG_VERIFY_LEVEL=3
GYMBAG_CLEARANCE_TTL=12h
GYMBAG_REMOTE_RULES=https://rules.example.com/baseline.rules
```

See the full [Docker & Docker Compose Guide](docs/guide/docker.md) for bind-mount permissions (`chown 10001:10001`), `--watch` hot-reloading, multi-container proxying, and local builds.

### Environment Variables (`GYMBAG_*`)

Precedence order is **Config file -> `GYMBAG_*` environment variables -> CLI flags**.

| Variable | Description |
| :--- | :--- |
| `GYMBAG_CONFIG` | Path to `Gymbagfile`, `Caddyfile`, or `.json` config |
| `GYMBAG_BIND` | Bind address (`127.0.0.1` for local domains, `0.0.0.0` when public domains exist) |
| `GYMBAG_HTTP_PORT` / `GYMBAG_HTTPS_PORT` | HTTP and HTTPS listen ports (defaults `80` and `443`) |
| `GYMBAG_DOMAIN` / `GYMBAG_DOMAINS` | Comma-separated downstream domain(s) |
| `GYMBAG_UPSTREAM` | Default upstream target URL |
| `GYMBAG_UPSTREAMS` | Semicolon-separated host map: `app.example.com=http://web:3000;api.example.com=http://api:8080` |
| `GYMBAG_PROTECT` | Enable or disable WAF protection across sites (`on` / `off`) |
| `GYMBAG_PROTECT_DOMAINS` | Comma-separated subset of domains that should have WAF protection enabled |
| `GYMBAG_VERIFY_LEVEL` | Verify level: `0` (`always`), `1` (`only suspicious`), `2` (`only ai`), `3` (`suspicious & ai`) |
| `GYMBAG_DEFAULT_RULES` | Enable or disable built-in baseline firewall rules (`on` / `off`, default `on`) |
| `GYMBAG_REMOTE_RULES` | Comma- or whitespace-separated HTTP/HTTPS remote firewall rules URLs |
| `GYMBAG_REMOTE_RULES_REFRESH` | Remote rules refresh interval in seconds or duration (default `300` / `5m`) |
| `GYMBAG_DIFFICULTY` | SHA-256 PoW leading-hex-zero difficulty (`1`–`5`, default `4`) |
| `GYMBAG_CHALLENGE_TTL` / `GYMBAG_CLEARANCE_TTL` | Challenge expiry (`300`s / `5m`) and verified status timeout (`86400`s / `24h`; aliases `GYMBAG_VERIFY_TIMEOUT`, `GYMBAG_VERIFIED_TIMEOUT`; accepts durations like `30m`, `12h`, `7d`) |
| `GYMBAG_COOKIE_NAME` / `GYMBAG_COOKIE_SECURE` | Clearance cookie name (`waf_clearance`) and `Secure` flag (`true`) |
| `GYMBAG_RATE_LIMIT` / `GYMBAG_RATE_WINDOW` | Per-IP request limit (`240`) and window in seconds (`60`) |
| `GYMBAG_TLS_MODE` | `auto`, `acme`, `local`, `selfsigned`, or `file` |
| `GYMBAG_SSL_PROVIDER` | `letsencrypt`, `letsencrypt_staging`, `zerossl`, `google`, `ssl.com`, `litessl`, `buypass`, `local`, or URL |
| `GYMBAG_ACME_EMAIL` | Contact email for ACME registration (and automatic ZeroSSL EAB) |
| `GYMBAG_ACME_DIRECTORY` | Explicit RFC 8555 ACME v2 directory URL |
| `GYMBAG_EAB_KID` / `GYMBAG_EAB_HMAC_KEY` | External Account Binding Key ID and base64/base64url HMAC key |
| `GYMBAG_ACCOUNT_KEY_FILE` | Path to user-supplied ECDSA or RSA ACME account key PEM |
| `GYMBAG_CERT_FILE` / `GYMBAG_KEY_FILE` | TLS certificate and private key paths for `file` mode |
| `GYMBAG_KEY_PASSWORD` | Passphrase for encrypted PEM private keys |
| `GYMBAG_CERT_DIR` | Directory for CA certs, ACME state, and intel cache (default `certs`) |
| `GYMBAG_RENEW_INTERVAL` / `GYMBAG_RENEW_WITHIN_DAYS` | Renewal check interval in seconds (`43200`) and threshold in days (`30`) |
| `GYMBAG_AUTO_HOSTS` | Automatic `/etc/hosts` management for `localhost` & `*.internal` (`on` / `off`, default `on`) |
| `GYMBAG_HSTS` | Override `Strict-Transport-Security` header (`on` / `off`) |
| `GYMBAG_IP_SECURITY` / `GYMBAG_IP_SECURITY_ACTION` | Toggle threat feeds (`on`/`off`) and action (`verify` or `block`) |
| `GYMBAG_IP_REPUTATION` | Toggle live Shodan InternetDB + StopForumSpam lookups (`on`/`off`) |
| `GYMBAG_AI_IP_LIST` | Toggle official OpenAI, Anthropic, Perplexity, Google, & Applebot IP lists (`on`/`off`) |
| `GYMBAG_INTEL_REFRESH` | Threat & AI feed refresh interval in seconds (default `21600`) |
| `GYMBAG_REDIS_URL` | Optional Redis connection URL (`redis://[:password@]host:port/db`) |
| `GYMBAG_SECRET` | HMAC secret for clearance cookies (auto-generated at `<cert_dir>/secret` if unset) |
| `GYMBAG_ADMIN` | Local Admin REST API listen address (e.g. `127.0.0.1:2019` or `off`) |

### REST APIs

#### Public WAF Endpoints (Served on Protected Sites)

| Method | Path | Description |
| :--- | :--- | :--- |
| `GET` | `/.waf/v1/session` | Returns current site clearance status, `protected`, `verify_level` (`0`–`3`), `verify_mode`, `clearance_ttl`, and (when cleared) `expires_at` / `expires_in` |
| `DELETE` | `/.waf/v1/session` | Clears the visitor's clearance cookie (`204 No Content`) |
| `POST` | `/.waf/v1/challenges` | Issues a single-use SHA-256 PoW challenge (`201 Created`) |
| `POST` | `/.waf/v1/verifications` | Verifies a PoW solution, sets the signed clearance cookie, and returns `expires_in` (`/.waf/verify` alias supported) |
| `GET` | `/.waf/assets/{name}` | Serves embedded PNG challenge assets (`/.waf/v1/assets/{name}` alias, cached via `ETag`) |

#### Local Admin API (Enabled via `admin 127.0.0.1:2019` or `--admin`)

Bind the Admin API to loopback (`127.0.0.1:2019`) or a Unix/WireGuard management interface — it requires no auth token and can mutate live routing and firewall rules without restarting the server.

| Method | Path | Description |
| :--- | :--- | :--- |
| `GET` | `/api/v1/health` | Gateway status, site count, active `tls_source`, loaded `threat_entries`, `ai_entries`, and `remote_rules` |
| `GET` | `/api/v1/tls` | Active TLS source, certificate path, configured SSL provider, HSTS status, and supported CA catalog |
| `POST` | `/api/v1/tls/renew` | Force an immediate certificate renewal and atomic hot-reload |
| `GET` | `/api/v1/intel` | Per-feed threat & AI crawler IP counts, last refresh timestamps, and reputation cache stats |
| `POST` | `/api/v1/intel/refresh` | Trigger an immediate background feed refresh and return updated counts |
| `GET` | `/api/v1/rules/remote` | Per-URL remote firewall rule counts, cache status, refresh interval, and last fetch errors |
| `POST` | `/api/v1/rules/remote/refresh` | Trigger an immediate fetch & atomic WAF engine reload for all remote rule URLs |
| `GET` | `/api/v1/config` | Dump active runtime configuration (`secret`, `key_password`, and `eab_hmac_key` redacted) |
| `PUT` | `/api/v1/config` | Atomically validate and replace the entire runtime configuration |
| `GET` | `/api/v1/sites` | List all configured websites and their WAF policies |
| `POST` | `/api/v1/sites` | Add a new website at runtime (`201 Created`) |
| `GET` | `/api/v1/sites/{domain}` | Inspect a single website's configuration |
| `PUT` | `/api/v1/sites/{domain}` | Replace a website's configuration |
| `PATCH` | `/api/v1/sites/{domain}` | Live-patch `verify_level`, `protect`, `difficulty`, `clearance_ttl`, `upstreams`, `rate_limit`, `rules`, or `remote_rules` |
| `DELETE` | `/api/v1/sites/{domain}` | Remove a website (`204 No Content`; refuses to delete the last remaining site) |
| `GET` | `/api/v1/sites/{domain}/rules` | List custom firewall rules on a website |
| `POST` | `/api/v1/sites/{domain}/rules` | Append a custom firewall rule live (`201 Created`) |
| `PUT` | `/api/v1/sites/{domain}/rules` | Replace all custom firewall rules on a website |
| `DELETE` | `/api/v1/sites/{domain}/rules/{index_or_name}` | Delete a custom rule by numeric index or rule `name` (`204 No Content`) |

**Live emergency mitigation examples:**

```bash
# Tighten app.example.com to verify level 0 (always challenge) during an active traffic spike
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"verify_level": 0, "difficulty": 5}'

# Block an offending CIDR immediately without editing Gymbagfile
curl -X POST http://127.0.0.1:2019/api/v1/sites/app.example.com/rules \
  -H "Content-Type: application/json" \
  -d '{"name": "drop-abusive-subnet", "action": "block", "ips": ["198.51.100.0/24"]}'

# Force an immediate refresh of IP threat & AI crawler lists
curl -X POST http://127.0.0.1:2019/api/v1/intel/refresh
```

---

## Benchmarks vs Caddy & Nginx

Benchmarked on Apple M4 (10-core, macOS, Go `1.27.0`), terminating TLS 1.3 with the same ECDSA P-256 certificate, 64 concurrent workers, `1 KiB` response payload, and `1024` upstream keep-alive connections across all three proxies (`0` errors on every run):

| Server & Mode | HTTP/1.1 TLS (RPS) | H1 p50 / p99 | HTTP/2 TLS (RPS) | H2 p50 / p99 | Binary Size | RSS (Under Load) |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **85,203** | 0.45 / 6.20 ms | **88,775** | 0.65 / 1.96 ms | 1.5 MB (+libs) | 5.8 MiB |
| **Gymbag `v0.1.0`** (`verify 3`, full WAF active) | **72,517** | 0.74 / 3.14 ms | **66,096** | 0.82 / 3.48 ms | 12.9 MB (static) | 54.3 MiB |
| **Gymbag `v0.1.0`** (`protect off`, pure proxy) | **72,479** | 0.74 / 3.15 ms | **64,048** | 0.83 / 3.94 ms | 12.9 MB (static) | 52.1 MiB |
| **Gymbag `v0.1.0`** (`verify 0`, cleared cookie) | **65,490** | 0.79 / 3.93 ms | **62,108** | 0.84 / 4.27 ms | 12.9 MB (static) | ~54 MiB |
| **Caddy `v2.11.9`** (Go, tuned keep-alive) | **54,741** | 0.90 / 4.90 ms | **39,207** | 1.30 / 6.18 ms | 49.0 MB | 76.8 MiB |

- **+32% faster than Caddy on HTTP/1.1** and **+63% to +69% faster on HTTP/2**, using ~30% less memory and a 3.8x smaller binary — even with Gymbag's full WAF active.
- **85% of C-based Nginx's HTTP/1.1 throughput** with nearly **2x lower p99 tail latency** (`3.14 ms` vs `6.20 ms`).
- **140,680 req/s** edge WAF blocking on `/.env` probes over TLS (`0.35 ms` p50, zero upstream traffic).

See [Benchmarks & Performance](docs/guide/benchmarks.md) for full methodology, Go microbenchmarks (`go test -bench`), and exact configs.

---

## Project Layout

```text
.
├── cmd/gymbag/          # Standalone CLI entrypoint
├── docs/                # Multi-language VitePress docs (EN, 简体中文, 日本語, 한국어, Español)
├── internal/
│   ├── cli/             # Subcommands (init, service, run, reverse-proxy, validate, fmt, version), .env & watcher
│   ├── config/          # Config models, Gymbagfile/Caddyfile & duration/remote-rules parsers, GYMBAG_* env loader
│   ├── host/            # Domain normalization, IPv6 bracket handling, wildcard matching
│   ├── intel/           # O(1) prefix set, auto-updating threat & AI crawler feeds, live IP reputation
│   ├── localpki/        # Two-tier local CA ("Neuro-sama Headquarters Root CA") & /etc/hosts manager
│   ├── proxy/           # Lock-free multi-site router, pooled HTTP & RFC 6455 WebSocket proxies
│   ├── server/          # Fused HTTPS gateway, HTTP->HTTPS/ACME redirector, REST Admin API
│   ├── service/         # System service renderer & manager (systemd, launchd, openrc, supervisor)
│   ├── store/           # StateStore interfaces, 64-shard in-memory store, pure-Go RESP2 Redis client
│   ├── tlsmanager/      # Multi-CA ACME v2 (EAB & ZeroSSL auto-EAB), local PKI, lock-free SNI cert manager
│   └── waf/             # PoW verifier, sharded HMAC clearance cache, Aho-Corasick DFA, remote rules, UI
├── Dockerfile
├── compose.yaml
├── flake.nix
├── Gymbagfile
└── go.mod
```

---

## Star History

<a href="https://www.star-history.com/#Evil-Project/gymbag&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
 </picture>
</a>
