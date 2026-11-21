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
  <strong>README:</strong>
  <a href="README.md">English</a> ·
  <a href="README.zh.md">简体中文</a> ·
  <a href="README.ja.md">日本語</a> ·
  <a href="README.ko.md">한국어</a> ·
  <a href="README.es.md">Español</a>
  <br />
  <sub>Online Docs: <a href="https://evil-project.github.io/gymbag/">EN</a> · <a href="https://evil-project.github.io/gymbag/zh/">ZH</a> · <a href="https://evil-project.github.io/gymbag/ja/">JA</a> · <a href="https://evil-project.github.io/gymbag/ko/">KO</a> · <a href="https://evil-project.github.io/gymbag/es/">ES</a></sub>
</p>

Gymbag is a single-binary HTTPS reverse proxy with a built-in proof-of-work WAF, in the spirit of Anubis. It uses only the Go standard library, so there are no third-party modules to pull in; templates, the SHA-256 solver script and the mascot images are embedded with `go:embed`.

It exists for the usual mess. Self-hosted apps, Gitea/Forgejo, docs sites and internal tools get hit by LLM scrapers that ignore `robots.txt` and by scanners walking `/.env` and `/wp-admin` all day. The usual fix is nginx or Caddy plus a PoW sidecar, fail2ban and a hand-maintained CIDR list. Gymbag does TLS, multi-CA ACME, HTTP and WebSocket proxying, local and remote firewall rules, IP threat intel and per-site PoW in one binary, driven by a `Gymbagfile`.

---

## Quick start

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

Getting started takes two commands: generate a config, then start.

```bash
# 1. Generate a JSON config (or pass --format gymbagfile for Caddy-style Gymbagfile)
./gymbag init -d app.example.com -u http://localhost:3000

# 2. Start the server (auto-loads ./gymbag.json, ./Gymbagfile, or ./Caddyfile)
./gymbag
```

It also reads an existing **nginx** (`nginx.conf`), **Apache** (`httpd.conf` / `apache2.conf`) or **Caddyfile v2** config, including snippets, `import`, `@matchers`, `handle`, `handle_path`, `route`, `root`, `file_server`, `try_files`, `respond`, `redir` and `header`.

To run it as a service (`systemd`, `launchd`, `openrc` or `supervisor`):

```bash
./gymbag init -d app.example.com -u :3000 --service auto
./gymbag service start
```

For a one-off you can skip the config file:

```bash
./gymbag reverse-proxy --from app.internal --to :3000 --verify 3
```

### What you get with no rules written

Gymbag can face the public internet without a single regex or imported blocklist. A bare `verify` turns on level `3` (`suspicious & ai`), while a bare `block` answers threat-intel IPs with `403 Forbidden` instead of a challenge. Either one enables three layers:

1. **Built-in baseline rules (`default_rules on`)**
   - **Sensitive files (`403`)**: `/.env`, `/.env.*`, `/.git/*`, `/.svn/*`, `/.hg/*`, `/.DS_Store`, `/.aws/*`, `/.ssh/*`, `/docker-compose.yml`, `/docker-compose.yaml`, `*.bak`, `*.sql`, `*.sqlite`.
   - **CMS and framework probes (`403`)**: `/wp-admin/*`, `/wp-login.php`, `/xmlrpc.php`, `/phpmyadmin*`, `/vendor/phpunit*`, `/cgi-bin/*`, `/actuator/env*`, `/actuator/heapdump*`.
   - **Vulnerability scanners (`403`)**: `sqlmap`, `nikto`, `nuclei`, `masscan`, `zgrab`, `gobuster`, `dirbuster`, `ffuf`, `wfuzz`, `feroxbuster`, `acunetix`, `nessus`, `openvas`.
   - **Injection and traversal queries (`403`)**: `UNION SELECT`, `<script`, `javascript:`, `/etc/passwd`, `information_schema`, `../`, `cmd.exe`, `/bin/sh` in query strings, on top of strict request-shape checks for null bytes and `%2e%2e` traversal.
   - **Automation clients (`PoW challenge`)**: while a site has no custom rules, CLI clients (`curl`, `wget`, `python-requests`, `python-httpx`, `go-http-client`, `scrapy`), headless browsers (`headlesschrome`, `playwright`, `puppeteer`, `selenium`) and empty `User-Agent` headers go to the proof-of-work screen.
2. **Self-updating IP threat intelligence**
   - [IPsum (level 3+)](https://raw.githubusercontent.com/stamparm/ipsum/master/levels/3.txt), [Spamhaus DROP IPv4](https://www.spamhaus.org/drop/drop_v4.json) and [IPv6](https://www.spamhaus.org/drop/dropv6.txt), and [abuse.ch Feodo Tracker](https://feodotracker.abuse.ch/downloads/ipblocklist.txt) are fetched at startup and every 6 hours, cached in `<cert_dir>/intel-cache` with conditional `ETag` / `If-Modified-Since` requests.
   - Keyless live reputation lookups against **Shodan InternetDB** (`internetdb.shodan.io`) and **StopForumSpam** (`api.stopforumspam.org`) with in-memory TTL caching and fail-open timeouts, so an upstream outage never stalls traffic.
3. **Official AI crawler IP tracking**
   - Published CIDR lists from **OpenAI** (`gptbot`, `searchbot`, `chatgpt-user`), **Anthropic** (`claude.com/crawling/bots.json`), **Perplexity** (`perplexitybot`, `perplexity-user`), **Google** (`user-triggered-agents`, `user-triggered-fetchers-google`) and **Apple** (`applebot`). A crawler that spoofs a desktop Chrome `User-Agent` still matches by source IP.

> **Ports 80 and 443:** `:80` handles the HTTP-to-HTTPS redirect and ACME HTTP-01, `:443` the HTTPS gateway. On Linux, grant the binary `CAP_NET_BIND_SERVICE` and it never needs root:
> ```bash
> sudo setcap 'cap_net_bind_service=+ep' ./gymbag
> ```
> For unprivileged local testing, pass `--http-port 8080 --https-port 8443` (or `http_port` / `https_port` in `Gymbagfile`).

---

## Local development (`localhost` and `*.internal`)

Local HTTPS on a custom hostname usually means mkcert, hand-edited `/etc/hosts` and a browser warning every day. Gymbag handles the hosts entry and the certificate for any site on `localhost`, `*.localhost`, `*.internal` (`app.internal`, `api.internal`), `*.local` or `*.test`:

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

### `/etc/hosts` is handled for you

With `auto_hosts` on (the default) Gymbag reads `/etc/hosts` at startup (`C:\Windows\System32\drivers\etc\hosts` on Windows). A concrete local domain such as `app.internal` or `api.internal` that doesn't already resolve there gets IPv4 and IPv6 loopback lines tagged with the instance ID:

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

Existing `localhost` lines are untouched. On shutdown (`Ctrl+C` / `SIGTERM`) Gymbag re-reads the file, so your own edits survive, and removes only its `# gymbag-managed:<instanceID>` lines.

- **macOS and Linux permissions:** `/etc/hosts` needs root. Run as a normal user, Gymbag logs a warning and keeps going. Use `sudo ./gymbag` for automatic hosts handling (and ports `80`/`443`), or set `auto_hosts off` (`GYMBAG_AUTO_HOSTS=off`) if you resolve `.internal` through `dnsmasq` or manage the file yourself.

### Local CA

Self-signed leaves are thrown away on every run, so the browser never learns to trust them. Instead Gymbag keeps a two-tier ECDSA P-256 CA under `<cert_dir>/ca/` (default `certs/ca/`):

1. **Root CA**: **`"Neuro-sama Headquarters Root CA"`** (`certs/ca/neuro-sama-root-ca.pem`, 10-year validity, `MaxPathLen: 1`)
2. **Intermediate CA**: **`"Gymbag & Swarm SSL Local CA"`** (`certs/ca/gymbag-swarm-local-ca.pem`, 5-year validity, signed by `"Neuro-sama Headquarters Root CA"`, `MaxPathLen: 0`)
3. **Leaf Certificate**: `certs/local-fullchain.pem` + `certs/local-privkey.pem` (90-day validity, auto-renewed within 30 days, serving the leaf + intermediate chain with DNS SANs and automatic `127.0.0.1` / `::1` IP SANs)

Trust **`"Neuro-sama Headquarters Root CA"`** once and every `https://localhost` and `https://*.internal` site validates in browsers and `curl`:

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

> **Firefox:** on macOS and Windows it picks up OS roots when `security.enterprise_roots.enabled` is `true` in `about:config`. On Linux, if Firefox uses its own NSS store rather than p11-kit, set that flag or import `certs/ca/neuro-sama-root-ca.pem` once under *Settings -> Privacy & Security -> Certificates -> View Certificates -> Authorities*.

---

## Multiple sites and verify levels (`0`–`3`)

One process proxies dozens of domains, round-robin across upstreams, with full-duplex RFC 6455 WebSocket upgrades. Each site sets its own policy with `verify <0-3>`, or disables challenges with `protect off`:

| Level | Name | Who gets challenged | Best fit |
| :---: | :--- | :--- | :--- |
| `0` | `always` | Every visitor without a valid HMAC clearance cookie. | Private tools, staging servers, or sites under active L7 flood. |
| `1` | `only suspicious` | Threat-feed / bad-reputation IPs, scanners, CLI HTTP libraries, headless browsers, missing/spoofed browser headers, and exploit probes. | Public web apps and APIs where normal browsers shouldn't see a challenge screen. |
| `2` | `only ai` | AI crawlers and LLM agents matched by **official AI crawler IP ranges** *or* AI bot `User-Agent` / agent headers. | Docs, blogs, and Git forges that want zero friction for humans and `curl` users, while taxing LLM scrapers. |
| `3` | `suspicious & ai` | Combines `1` and `2`: challenges both suspicious clients/IPs and AI crawlers; normal human browsers pass straight through. | Recommended default for public production websites (also enabled by bare `verify`). |
| n/a | `protect off` | No proof-of-work challenges; a plain HTTPS and WebSocket reverse proxy. Explicit custom rules on the site still run. | Static asset hosts, webhooks, public package mirrors. |

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

When a browser solves the SHA-256 proof-of-work challenge (`difficulty` `1`–`5`, default `4`), Gymbag sets an `HttpOnly`, `SameSite=Lax` clearance cookie signed with HMAC (`waf_clearance`, TTL `24h` / `86400s` by default). The cookie is bound to both host and client IP.

### How long clearance lasts (`verify_timeout` / `clearance_ttl`)

Set it globally or per site, in seconds or as a duration (`30m`, `12h`, `1h30m`, `7d`, `2w`):

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

The same value is also `--verify-timeout 6h` (or `--clearance-ttl 6h`) on `gymbag init`, `gymbag run` and `gymbag reverse-proxy`, `GYMBAG_VERIFY_TIMEOUT` / `GYMBAG_CLEARANCE_TTL`, or live through `PATCH /api/v1/sites/{domain}`. `GET /.waf/v1/session` reports `clearance_ttl`, `expires_at` and remaining `expires_in` seconds for cleared visitors.

---

## Path routes, static files, redirects and config imports

A site can also define ordered **path routes** next to plain `reverse_proxy`. The first match wins; with no match the request falls through to the site's default upstreams, or `404` if there are none. Each route has exactly one handler: reverse proxy, static file server (`root`), fixed `respond` or `redir`.

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
    try_files {path} /index.html          # SPA fallback
}
```

A matcher is an exact path (`/exact`), a glob (`/api/*`, `*.php`) or a Go RE2 regex after `~` (`~^/v[0-9]+/`). Redirect targets expand `{scheme}`, `{host}`, `{uri}`, `{path}` and `{query}`. Static routes accept `index`, `browse` (directory listing) and `fallback` (`/index.html` for SPAs). A route `header` sets a header, or removes it when the key starts with `-`. WAF rules and verification still run before any route handler.

The same thing as a `routes` array in `gymbag.json`:

```json
{"routes": [
  {"path": "/static/*", "strip_prefix": "/static", "root": "./dist", "browse": true},
  {"path": "/old/*", "redirect": {"to": "https://{host}/new{uri}", "code": 301}},
  {"path": "/api/*", "upstreams": ["http://localhost:8080"]}
]}
```

Manage them with `gymbag config route list|add|update|remove`, or live via `PATCH /api/v1/sites/{domain}` (`routes`, `headers`, `rule_sources`).

**Importing existing configs:** `gymbag run -c /etc/nginx/nginx.conf` (or `httpd.conf` / `apache2.conf`) serves a native nginx or Apache config as-is. To convert instead, `gymbag config convert -c <file> -o gymbag.json --format json` handles nginx, Apache and Caddyfile v2, including snippets, `import`, `@matchers`, `handle` and `route`.

---

## SSL providers, custom keys and renewal

For public domains, `tls auto` (the default) gets certificates from Let's Encrypt over RFC 8555 ACME HTTP-01. To use another CA, set `ssl_provider` (aliases: `acme_provider`, `ca`, `cert_issuer`) or `tls <provider>`:

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

### Other CAs and External Account Binding (EAB)

**ZeroSSL, no EAB setup needed:**
Pick `zerossl` and set `email`, and Gymbag exchanges the address with ZeroSSL's EAB endpoint (`https://api.zerossl.com/acme/eab-credentials-email`) unless you've supplied EAB keys yourself:

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

**Google Trust Services, SSL.com, or explicit EAB:**
Pass the Key ID and HMAC key. Both URL-safe unpadded base64 and standard padded base64 work, and `eab_hmac_key` is masked in Admin API output:

```caddyfile
{
    email ops@example.com
    ssl_provider google
    eab YOUR_EAB_KEY_ID YOUR_EAB_HMAC_KEY
}
```

**LiteSSL (TrustAsia) or a custom ACME directory:**

```caddyfile
{
    email ops@example.com
    ssl_provider litessl
    # Or point directly at any ACME v2 directory URL:
    # acme_directory https://ca.internal.corp/acme/acme/directory
}
```

Cached ACME certs are stored per provider on disk, e.g. `certs/acme-zerossl-fullchain.pem` and `certs/acme-google-fullchain.pem`, with `certs/acme-fullchain.pem` left for Let's Encrypt. Change `ssl_provider` and restart or reload, and Gymbag requests from the new CA right away instead of serving the old issuer's cert.

### Your own keys and certificates

- **ACME account key (`account_key`)**: Gymbag creates and reuses an ECDSA P-256 key at `certs/acme-account.pem`. If you already have a registered ECDSA (P-256/P-384) or RSA account key, point at it with `account_key /etc/gymbag/account.pem` (or `GYMBAG_ACCOUNT_KEY_FILE`).
- **CSR private key (`ssl_key`)**: `ssl_key /etc/gymbag/domain.key` signs ACME CSRs with your own ECDSA or RSA key instead of a generated one.
- **Full certificate and key (`tls <cert> <key>`)**: use commercial, Cloudflare Origin or otherwise externally managed PEM files, globally or per site (matched by SNI). Encrypted keys work via `key_password` (or `GYMBAG_KEY_PASSWORD`):

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

### Renewal without downtime

A background loop runs every `renew_interval` seconds (default `43200`, i.e. 12 hours). When an ACME cert, local PKI leaf or file-backed cert comes within `renew_within_days` of expiry (default `30` days), Gymbag renews or reloads it and swaps the active `tls.Certificate` atomically. Open HTTP streams and WebSocket tunnels stay connected.

---

## Writing your own firewall rules

On a protected site every request walks through the layers in a fixed order:

1. **Request shape sanity** (HTTP verb check, max URI length, null-byte and `..` traversal rejection)
2. **ACME HTTP-01 bypass** (`/.well-known/acme-challenge/*`)
3. **Your local custom rules** (global rules first, then site rules, top to bottom; first match wins)
4. **Remote rules (`remote_rules`)** (auto-refreshed HTTP/HTTPS rulesets in Gymbagfile syntax or JSON)
5. **Built-in baseline security rules** (unless `default_rules off` is set)
6. **Per-site per-IP rate limiter** (`rate_limit` / `rate_window`)
7. **IP threat intelligence** (`ip_security_action verify` or `block`)
8. **Proof-of-work verification** (`verify` level `0`–`3`)

Your custom rules run before the built-in baseline rules, the rate limiter and the threat feeds, so an `allow` at the top of a site block is enough to whitelist an office CIDR, an internal health check or a preview cookie.

### One-line shorthand rules

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

### Multi-condition `rule` blocks

When one field isn't specific enough, use `rule "name" { ... }`. Values on the same field are OR'd; different fields in the same block are AND'd:

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

### Remote rules (`remote_rules`)

Shared rulesets can live at one or more HTTP/HTTPS URLs, globally or per site. Gymbag reads both **Gymbagfile syntax** (`allow`/`verify`/`block` one-liners and `rule "name" { ... }` blocks) and **JSON** (`[ ... ]` or `{"rules": [ ... ]}`). Validated payloads are cached under `<cert_dir>/rules-cache` with conditional `ETag` / `If-Modified-Since` requests, and if a source is unreachable or malformed the last good ruleset stays active:

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

Check status or force a refresh with `GET /api/v1/rules/remote` and `POST /api/v1/rules/remote/refresh`, or set `--remote-rules` / `GYMBAG_REMOTE_RULES`.

### Local rule files, folders and hot reload

Rules can live in a `.rules`/`.json` file or a whole directory, referenced by path or linked with `gymbag ruleset add`. Mark a source `dynamic` and it is watched and reloaded on change, no restart needed. Invalid syntax keeps the previous ruleset in place:

```caddyfile
{
    rules_watch_interval 2s
}

app.example.com {
    reverse_proxy http://localhost:3000
    rules_file ./rules/custom.rules dynamic
    rules_dir ./rules.d dynamic
    ruleset static-corp {
        source ./rules/corp.json
        dynamic off
    }
}
```

In `gymbag.json` that's `"rule_sources": [{"source": "./rules.d", "dynamic": true, "name": "shared"}]`, or pass `--rules-file`, `--rules-dir`, `--rules-dynamic` and `--rules-watch-interval` to `gymbag init`.

### Built-in rules (`default_rules on|off`)

`default_rules` is `on` by default:
- With **no** custom rules on a site, Gymbag enforces both the built-in block presets (`/.env`, `/wp-admin`, scanners, SQLi/XSS queries) and the automation verify presets.
- Once you add your own rules, they run first and the built-in **block** presets stay appended, so adding one `verify path` rule doesn't expose `/.git` or `/.env`.
- For full manual control, set `default_rules off` (or `GYMBAG_DEFAULT_RULES=off`) on a site or globally.

---

## CLI, Docker Compose, environment variables and REST APIs

### CLI reference

Gymbag loads `.env` from the working directory if there is one, then picks up `./gymbag.json`, `./Gymbagfile` or `./Caddyfile` unless a flag overrides it. A native `nginx.conf` or Apache `httpd.conf`/`apache2.conf` can also be passed straight in with `-c`:

```bash
# Generate gymbag.json (default) or Gymbagfile via interactive wizard or flags
./gymbag init
./gymbag init -d app.example.com -u http://localhost:3000 -l 3 --rules-dir ./rules.d --rules-dynamic
./gymbag init --format gymbagfile -o Gymbagfile -d app.example.com -u :3000

# Manage sites, routes, and global settings in gymbag.json or Gymbagfile
./gymbag config list
./gymbag config add -d api.example.com -u :8080 -l 1
./gymbag config update -d api.example.com -l 3 --verify-timeout 6h
./gymbag config route add -d api.example.com --path /static/* --strip-prefix /static --root ./dist
./gymbag config remove -d api.example.com
./gymbag config set admin 127.0.0.1:2019
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json

# Manage dedicated rule files, rules folders, and inline rules
./gymbag ruleset init ./rules/custom.rules
./gymbag ruleset add --source ./rules --dynamic --name local-dir
./gymbag ruleset update local-dir --static
./gymbag ruleset remove local-dir
./gymbag ruleset validate ./rules
./gymbag ruleset rule add --file ./rules/custom.rules --action block --name bad-bot --ua "*BadBot*"

# Start server using ./gymbag.json, ./Gymbagfile, or ./Caddyfile
./gymbag

# Start with an explicit config file (JSON, Gymbagfile, Caddyfile v2, nginx, Apache) and hot-reload on save
./gymbag run --config gymbag.json --watch

# Register & manage a system service (systemd, launchd, openrc, supervisor)
./gymbag service install --config gymbag.json
./gymbag service start
./gymbag service status
./gymbag service render --type systemd   # Preview a systemd unit on any OS

# Ad-hoc reverse proxy without a config file
./gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3

# Validate config + local rule sources and print the resolved per-site security summary
./gymbag validate --config gymbag.json

# Format a Gymbagfile into canonical 4-space layout in place
./gymbag fmt --overwrite Gymbagfile

# Print version
./gymbag version
```

| Flag | Description |
| :--- | :--- |
| `-c`, `--config <path>` | Path to `gymbag.json`, `Gymbagfile`, `Caddyfile`, `nginx.conf`, or Apache config file |
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

### Docker and Docker Compose

Official multi-arch Alpine images (`linux/amd64`, `linux/arm64`) live on GitHub Container Registry (`ghcr.io/evil-project/gymbag`), running as non-root UID `10001` with `cap_net_bind_service=+ep`.

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

### Environment variables (`GYMBAG_*`)

Precedence is **config file -> `GYMBAG_*` environment variables -> CLI flags**.

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
| `GYMBAG_RULES_WATCH_INTERVAL` | Polling interval in seconds or duration for dynamic local rule files/folders (default `2` / `2s`) |
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

#### Public WAF endpoints (served on protected sites)

| Method | Path | Description |
| :--- | :--- | :--- |
| `GET` | `/.waf/v1/session` | Returns current site clearance status, `protected`, `verify_level` (`0`–`3`), `verify_mode`, `clearance_ttl`, and (when cleared) `expires_at` / `expires_in` |
| `DELETE` | `/.waf/v1/session` | Clears the visitor's clearance cookie (`204 No Content`) |
| `POST` | `/.waf/v1/challenges` | Issues a single-use SHA-256 PoW challenge (`201 Created`) |
| `POST` | `/.waf/v1/verifications` | Verifies a PoW solution, sets the signed clearance cookie, and returns `expires_in` (`/.waf/verify` alias supported) |
| `GET` | `/.waf/assets/{name}` | Serves embedded PNG challenge assets (`/.waf/v1/assets/{name}` alias, cached via `ETag`) |

#### Local Admin API (`admin 127.0.0.1:2019` or `--admin`)

Bind the Admin API to loopback (`127.0.0.1:2019`) or a Unix/WireGuard management interface. It needs no auth token, and can change live routing and firewall rules without a restart.

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

## Benchmarks against Caddy and Nginx

Apple M4 (10-core, macOS, Go `1.27.0`), TLS 1.3 with the same ECDSA P-256 certificate, 64 concurrent workers, `1 KiB` response payload, `1024` upstream keep-alive connections, `0` errors on every run:

| Server & Mode | HTTP/1.1 TLS (RPS) | H1 p50 / p99 | HTTP/2 TLS (RPS) | H2 p50 / p99 | Binary Size | RSS (Under Load) |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **85,203** | 0.45 / 6.20 ms | **88,775** | 0.65 / 1.96 ms | 1.5 MB (+libs) | 5.8 MiB |
| **Gymbag `v0.1.0`** (`verify 3`, full WAF active) | **72,517** | 0.74 / 3.14 ms | **66,096** | 0.82 / 3.48 ms | 12.9 MB (static) | 54.3 MiB |
| **Gymbag `v0.1.0`** (`protect off`, pure proxy) | **72,479** | 0.74 / 3.15 ms | **64,048** | 0.83 / 3.94 ms | 12.9 MB (static) | 52.1 MiB |
| **Gymbag `v0.1.0`** (`verify 0`, cleared cookie) | **65,490** | 0.79 / 3.93 ms | **62,108** | 0.84 / 4.27 ms | 12.9 MB (static) | ~54 MiB |
| **Caddy `v2.11.9`** (Go, tuned keep-alive) | **54,741** | 0.90 / 4.90 ms | **39,207** | 1.30 / 6.18 ms | 49.0 MB | 76.8 MiB |

- **+32% over Caddy on HTTP/1.1** and **+63% to +69% on HTTP/2**, with about 30% less memory and a 3.8x smaller binary, full WAF included.
- **85% of C-based Nginx's HTTP/1.1 throughput** with nearly **half the p99 tail** (`3.14 ms` vs `6.20 ms`).
- **140,680 req/s** blocking `/.env` probes at the edge over TLS (`0.35 ms` p50, no upstream traffic).

Full methodology, Go microbenchmarks (`go test -bench`) and exact configs are in [Benchmarks & Performance](docs/guide/benchmarks.md).

---

## Project layout

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

## Star history

<a href="https://www.star-history.com/#Evil-Project/gymbag&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
 </picture>
</a>
