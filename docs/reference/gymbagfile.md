# `Gymbagfile` & JSON Schema Reference

By default, Gymbag probes `./gymbag.json`, `./Gymbagfile`, and `./Caddyfile` in the working directory (or `--config <path>` / `GYMBAG_CONFIG`). It natively parses `gymbag.json`, `Gymbagfile` / `Caddyfile v2`, `nginx.conf`, and Apache (`httpd.conf` / `apache2.conf`).

## Global Options Block `{ ... }`

An optional top-level `{ ... }` block configures server-wide TLS, ACME, networking, threat/VPN/AI feeds, remote rules, verification portal settings, and default WAF policies:

```caddyfile
{
    email ops@example.com
    ssl_provider letsencrypt
    # eab YOUR_EAB_KID YOUR_EAB_HMAC_KEY
    # account_key certs/acme-account.pem
    # ssl_key certs/domain.key

    bind 0.0.0.0
    http_port 80
    https_port 443
    admin 127.0.0.1:2019
    cert_dir certs
    auto_hosts on
    hsts on

    # Default clearance cookie/IP timeout (seconds or duration: 30m, 12h, 7d)
    verify_timeout 24h

    # Shared HTTPS verification portal for SSH & Minecraft
    # verify_url https://verify.example.com:8443
    # verify_port 8443

    # Optional Redis store for shared PoW nonces, rate limits, and SSH/MC gate tickets
    # redis redis://127.0.0.1:6379/0

    # IP threat intel, VPN/Tor detection & AI crawler IP feeds
    ip_security on
    ip_security_action verify
    ip_reputation on
    vpn_detection on
    ai_ip_list on
    intel_refresh 6h

    # Shared remote firewall rules (Gymbagfile syntax or JSON)
    # remote_rules https://rules.example.com/baseline.rules
    # remote_rules_refresh 5m

    # Global custom rules evaluated before remote & site rules
    block ip 198.51.100.99 192.0.2.0/24
}
```

### Global Directives

| Directive | Description | Default |
| :--- | :--- | :--- |
| `email` / `acme_email` | Contact email for ACME registration (and ZeroSSL auto-EAB) | — |
| `ssl_provider` / `ca` | `letsencrypt`, `letsencrypt_staging`, `zerossl`, `google`, `ssl.com`, `litessl`, `buypass`, `local`, or URL | `auto` (`letsencrypt` / `local`) |
| `acme_directory` | Explicit RFC 8555 ACME v2 directory URL | Let's Encrypt v2 |
| `eab <kid> <hmac>` | External Account Binding credentials (or `eab_kid` / `eab_hmac_key`) | — |
| `account_key` | Existing ECDSA or RSA ACME account PEM path | `<cert_dir>/acme-account.pem` |
| `ssl_key` | Existing private key path for ACME CSRs | Auto-generated |
| `tls` | `auto`, `acme`, `local`, `selfsigned`, or `<cert.pem> <key.pem>` | `auto` |
| `key_password` | Passphrase for encrypted PEM private keys | — |
| `cert_dir` | Directory for certs, local CA, secret, intel cache, and rule cache | `certs` |
| `renew_interval` | Certificate renewal check interval (seconds or duration like `12h`) | `43200` (`12h`) |
| `renew_within_days` | Renew certificates within N days of expiry | `30` |
| `auto_hosts` | Manage `/etc/hosts` entries for `localhost` / `*.internal` (`on`/`off`) | `on` |
| `hsts` | Send `Strict-Transport-Security` header (`on`/`off`) | `on` for public ACME certs |
| `bind` / `bind_host` | Listen IP address | `127.0.0.1` (local HTTP) / `0.0.0.0` (public HTTP, SSH, or MC) |
| `http_port` / `https_port` | HTTP redirect/ACME port & HTTPS gateway port | `80` / `443` |
| `verify_url` | Public HTTPS URL for the SSH/Minecraft verification portal | Server IP + IP SSL cert |
| `verify_port` | Shared HTTPS port for the SSH/Minecraft verification portal | `8443` |
| `verify_server { ... }` | Optional block supporting `url`, `port`, and `tls <cert> <key>` | — |
| `admin` | Local Admin REST API listen address (e.g. `127.0.0.1:2019` or `off`) | disabled |
| `redis` / `redis_url` | Redis connection URL (`redis://` or `rediss://`) | in-memory store |
| `secret` | HMAC secret for `waf_clearance` cookies | `<cert_dir>/secret` |
| `trusted_proxies` | Trusted downstream proxy IPs/CIDRs for `X-Forwarded-For` | — |
| `ip_security` | Enable threat-intel CIDR feeds (`on`/`off`) | `on` |
| `ip_security_action` | Threat-intel match action (`verify` or `block`) | `verify` |
| `ip_reputation` | Enable live Shodan InternetDB & StopForumSpam lookups (plus live VPN API lookups when `vpn_detection` is on) | `on` |
| `vpn_detection` / `vpn_detect` | Flag free VPN/proxy/Tor IPs as suspicious at verify levels `1` & `3` (`on`/`off`) | `on` |
| `ai_ip_list` | Enable official AI crawler CIDR lists (`on`/`off`) | `on` |
| `intel_refresh` | Feed refresh interval (seconds or duration like `6h`) | `21600` (`6h`) |
| `threat_feed` / `ai_ip_feed` / `vpn_feed` | Additional custom IP feed URLs (`threat_feeds`, `ai_ip_feeds`, `vpn_feeds` also work) | — |
| `remote_rules` | HTTP/HTTPS remote firewall rule URLs inherited by all sites | — |
| `remote_rules_refresh` | Refresh interval for remote rule URLs (seconds or duration; alias `rules_refresh`) | `300` (`5m`) |
| `rules_file` | Local `.rules`/`.json` rule file (`rules_file <path> [dynamic\|static]`) | — |
| `rules_dir` | Local rules directory (`rules_dir <path> [dynamic\|static]`) | — |
| `ruleset <name> { ... }` | Named local or remote rule source block (`source`/`file`/`dir`/`url`, `dynamic on\|off`) | — |
| `rules_watch_interval` | Polling interval for `dynamic` local rule files and folders | `2` (`2s`) |
| `verify_timeout` / `clearance_ttl` | Default clearance lifetime (seconds or duration like `30m`, `12h`, `7d`) | `86400` (`24h`) |

## Site Block Directives

```caddyfile
app.example.com, www.example.com {
    reverse_proxy http://localhost:3000 http://localhost:3001
    verify 3
    difficulty 4
    rate_limit 240 60s
    challenge_ttl 5m
    verify_timeout 12h
    cookie_name waf_clearance
    cookie_secure true
    default_rules on
    remote_rules https://rules.example.com/app.rules

    allow ip 10.0.0.0/8
    block path /internal-only*
}
```

| Directive | Description |
| :--- | :--- |
| `reverse_proxy <urls...>` | One or more upstream targets (round-robin load balanced; supports `:3000` shorthand) |
| `verify [0\|1\|2\|3\|off]` | Verification level (bare `verify` enables level `3`) |
| `protect <on\|off>` | Enable or disable WAF verification |
| `difficulty <1-5>` | SHA-256 leading-hex-zero PoW difficulty |
| `rate_limit <count> [window]` | Per-IP rate limit and optional window (`rate_window` also accepted) |
| `challenge_ttl <sec\|dur>` | PoW challenge nonce expiry (e.g. `300` or `5m`) |
| `clearance_ttl <sec\|dur>` | Verified clearance lifetime (aliases: `verify_timeout`, `verified_timeout`, `clearance_timeout`) |
| `cookie_name <name>` | Clearance cookie name |
| `cookie_secure <bool>` | Whether clearance cookie sets `Secure` |
| `default_rules <on\|off>` | Toggle built-in baseline rules |
| `remote_rules <urls...>` | Remote firewall rules URLs (Gymbagfile syntax or JSON) |
| `rules_file <path> [dynamic\|static]` | Local `.rules` or `.json` file (optionally hot-reloaded) |
| `rules_dir <path> [dynamic\|static]` | Directory of `.rules`/`.json` files (optionally hot-reloaded) |
| `ruleset <name> { ... }` | Named rule source block (`source`/`file`/`dir`/`url`, `dynamic on\|off`) |
| `handle` / `handle_path` / `route` | Ordered path-matched route blocks (`reverse_proxy`, `root` + `file_server`, `respond`, `redir`, `header`) |
| `root` / `file_server` / `try_files` | Static file serving, directory listings (`browse`), and SPA fallback |
| `respond` / `redir` / `header` | Static responses, redirects (`{scheme}`, `{host}`, `{uri}`, `{path}`, `{query}`), and header edits |
| `ai_bots <patterns...>` | Extra User-Agent substrings treated as AI crawlers |
| `suspicious_uas <patterns...>` | Extra User-Agent substrings treated as suspicious |
| `suspicious_paths <prefixes...>` | Extra path prefixes treated as suspicious probes |
| `tls <cert> <key>` | Per-site SNI certificate and private key paths |
| `allow \| verify \| block ...` | Shorthand custom firewall rule |
| `rule "name" { ... }` | Multi-condition custom firewall rule block |

## `ssh` and `minecraft` Service Blocks

Top-level `ssh [port] { ... }` and `minecraft [port] { ... }` (alias `mc`) blocks configure TCP PoW gating:

```caddyfile
ssh {
    port 22
    upstream 127.0.0.1:41822
    verify 3
    difficulty 4
    rate_limit 60 60s
    clearance_ttl 24h
    allow ip 10.0.0.0/8
}

minecraft {
    port 25565
    upstream 127.0.0.1:48291
    server_properties /srv/mc/server.properties
    verify 3
    rate_limit 60 60s
}
```

| Directive | Description | Default |
| :--- | :--- | :--- |
| `enabled <bool>` | Enable or disable the block without removing it | `true` |
| `port <port>` | Public listen port (can also be written as `ssh 2222 { ... }`) | `22` (SSH) / `25565` (Minecraft) |
| `upstream <host:port>` | Backend TCP target (`reverse_proxy` / `proxy` also accepted, e.g. `:2222`, `tcp://127.0.0.1:25566`) | — (required) |
| `server_properties <path>` | Path to `server.properties` (`minecraft` block only) | — |
| `verify [0\|1\|2\|3\|off]` | Verification level (bare `verify` sets `3`) | `3` |
| `protect <on\|off>` | Toggle PoW verification for this service | `on` |
| `difficulty <1-5>` | PoW leading-hex-zero difficulty | `4` |
| `rate_limit <count> [window]` | Per-IP connection rate limit and window | `60` / `60s` |
| `challenge_ttl <sec\|dur>` | Challenge token expiry | `300` (`5m`) |
| `clearance_ttl <sec\|dur>` | Cleared IP lifetime (`verify_timeout` also accepted) | `86400` (`24h`) |
| `verify_url` / `verify_port` | Per-service override for the verification link printed in disconnect messages | Global `verify_url` / `verify_port` |
| `allow \| verify \| block ...` / `rule` | Custom rules matching IP/CIDR or client banner/username (`user_agent`) | — |

## Duration Syntax

Time directives (`clearance_ttl` / `verify_timeout`, `challenge_ttl`, `rate_window`, `intel_refresh`, `remote_rules_refresh`, `rules_watch_interval`, and `renew_interval`) accept:
- Plain integer seconds: `300`, `3600`, `86400`
- Go durations: `45s`, `30m`, `12h`, `1h30m`
- Day & week shorthand: `1d`, `7d`, `1d12h`, `2w`

## Formatting & Validation

```bash
./gymbag validate --config Gymbagfile
./gymbag fmt --overwrite Gymbagfile
```

## JSON Configuration (`gymbag.json`)

`gymbag init` generates concise `gymbag.json` by default, omitting unused services and default values (`--format gymbagfile` switches to Caddy syntax):

```bash
./gymbag init -d app.example.com -u :3000 --verify-timeout 12h --rules-dir ./rules.d --rules-dynamic
```

Top-level JSON keys (`DisallowUnknownFields` is enforced):
- `sites`: array of `{domain, upstreams, routes, headers, protect, waf, cert_file, key_file}`, where `waf` holds `{verify_level, difficulty, challenge_ttl, clearance_ttl, cookie_name, cookie_secure, rate_limit, rate_window, default_rules, rules, remote_rules, rule_sources, custom_ai_bots, custom_suspicious_uas, custom_suspicious_paths}`.
- `ssh`: `{enabled, port, upstream, protect, verify_level, difficulty, challenge_ttl, clearance_ttl, rate_limit, rate_window, verify_url, verify_port, rules}`.
- `minecraft`: `{enabled, port, upstream, server_properties, protect, verify_level, difficulty, challenge_ttl, clearance_ttl, rate_limit, rate_window, verify_url, verify_port, rules}`.
- `verify`: `{url, port, cert_file, key_file}`.
- `bind_host`, `http_port`, `https_port`, `admin_addr`, `remote_rules_refresh`, `rules_watch_interval`, `redis_url`, `secret`, `trusted_proxies`.
- `tls`: `{mode, provider, cert_file, key_file, key_password, account_key_file, eab_kid, eab_hmac_key, auto_hosts, cert_dir, acme_email, acme_directory, renew_interval, renew_within_days, hsts}`.
- `intel`: `{ip_security, ip_security_action, live_reputation, ai_ip_list, vpn_detection, refresh_interval, threat_feeds, ai_ip_feeds, vpn_feeds}`.
- `defaults`: shared base `waf` object.

You can also convert `nginx.conf`, Apache `httpd.conf`/`apache2.conf`, or `Caddyfile` to `gymbag.json`:

```bash
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```
