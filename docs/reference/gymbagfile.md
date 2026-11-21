# `Gymbagfile` & JSON Schema Reference

By default, Gymbag probes `./gymbag.json`, `./Gymbagfile`, or `./Caddyfile` in the working directory (or `--config <path>` / `GYMBAG_CONFIG`), and natively parses `gymbag.json`, `Gymbagfile` / `Caddyfile v2`, `nginx.conf`, and Apache (`httpd.conf` / `apache2.conf`).

## Global Options Block `{ ... }`

An optional top-level `{ ... }` block configures server-wide TLS, ACME, networking, threat intelligence, remote rules, and default WAF policies inherited by all sites:

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

    # Default verified clearance timeout (seconds or duration: 30m, 12h, 7d)
    verify_timeout 24h

    # Optional Redis store for shared PoW nonces and rate limits
    # redis redis://127.0.0.1:6379/0

    # IP threat intelligence & AI crawler IP feeds
    ip_security on
    ip_security_action verify
    ip_reputation on
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
| `ssl_provider` / `ca` | `letsencrypt`, `letsencrypt_staging`, `zerossl`, `google`, `ssl.com`, `litessl`, `buypass`, `local`, or custom URL | `auto` (`letsencrypt` / `local`) |
| `acme_directory` | Explicit RFC 8555 ACME v2 directory URL | Let's Encrypt v2 |
| `eab <kid> <hmac>` | External Account Binding credentials (or `eab_kid` / `eab_hmac_key`) | — |
| `account_key` | Path to existing ECDSA or RSA ACME account PEM | `<cert_dir>/acme-account.pem` |
| `ssl_key` | Path to existing private key for ACME CSRs | Auto-generated |
| `tls` | `auto`, `acme`, `local`, `selfsigned`, or `<cert.pem> <key.pem>` | `auto` |
| `key_password` | Passphrase for encrypted PEM private keys | — |
| `cert_dir` | Directory for certs, local CA, secret, intel cache, and remote rules cache | `certs` |
| `renew_interval` | Certificate renewal check interval (seconds or duration like `12h`) | `43200` (`12h`) |
| `renew_within_days` | Renew certificates within N days of expiry | `30` |
| `auto_hosts` | Manage `/etc/hosts` entries for `localhost` / `*.internal` (`on`/`off`) | `on` |
| `hsts` | Send `Strict-Transport-Security` header (`on`/`off`) | `on` for public ACME certs |
| `bind` / `bind_host` | Listen IP address | `127.0.0.1` (local) / `0.0.0.0` (public) |
| `http_port` / `https_port` | HTTP redirect/ACME port & HTTPS gateway port | `80` / `443` |
| `admin` | Local Admin REST API listen address (e.g. `127.0.0.1:2019` or `off`) | disabled |
| `redis` / `redis_url` | Redis connection URL (`redis://[:password@]host:port/db`) | in-memory store |
| `secret` | HMAC secret for `waf_clearance` cookies | `<cert_dir>/secret` |
| `trusted_proxies` | Trusted downstream proxy IPs/CIDRs for `X-Forwarded-For` | — |
| `ip_security` | Enable threat-intel CIDR feeds (`on`/`off`) | `on` |
| `ip_security_action` | Threat-intel match action (`verify` or `block`) | `verify` |
| `ip_reputation` | Enable live Shodan InternetDB & StopForumSpam lookups | `on` |
| `ai_ip_list` | Enable official AI crawler CIDR lists | `on` |
| `intel_refresh` | Feed refresh interval (seconds or duration like `6h`) | `21600` (`6h`) |
| `threat_feed` / `ai_ip_feed` | Additional custom IP feed URLs | — |
| `remote_rules` | One or more HTTP/HTTPS remote firewall rule URLs inherited by all sites | — |
| `remote_rules_refresh` | Refresh interval for remote rule URLs (seconds or duration like `5m`, `1h`; alias `rules_refresh`) | `300` (`5m`) |
| `rules_file` | Local `.rules`/`.json` rule file (`rules_file <path> [dynamic\|static]`) inherited by all sites | — |
| `rules_dir` | Local rules directory (`rules_dir <path> [dynamic\|static]`) inherited by all sites | — |
| `ruleset <name> { ... }` | Named local or remote rule source block (`source`/`file`/`dir`/`url`, `dynamic on\|off`) | — |
| `rules_watch_interval` | Polling interval for `dynamic` local rule files and directories | `2` (`2s`) |
| `verify_timeout` / `clearance_ttl` | Default verified clearance cookie timeout (seconds or duration like `30m`, `12h`, `7d`) | `86400` (`24h`) |

---

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
| `protect <on\|off>` | Enable or disable WAF verification for the site |
| `difficulty <1-5>` | SHA-256 leading-hex-zero PoW difficulty |
| `rate_limit <count> [window]` | Per-IP rate limit and optional window in seconds or duration (`rate_window` also accepted) |
| `challenge_ttl <sec\|dur>` | PoW challenge nonce expiry (e.g. `300` or `5m`) |
| `clearance_ttl <sec\|dur>` | Verified clearance cookie lifetime (e.g. `86400`, `30m`, `12h`, `7d`; aliases: `verify_timeout`, `verified_timeout`, `clearance_timeout`) |
| `cookie_name <name>` | Clearance cookie name |
| `cookie_secure <bool>` | Whether clearance cookie sets `Secure` |
| `default_rules <on\|off>` | Toggle built-in baseline rules |
| `remote_rules <urls...>` | One or more remote firewall rules URLs (Gymbagfile syntax or JSON; aliases: `remote_rule`, `rules_url`, `rule_url`, `remote_rules_url`) |
| `rules_file <path> [dynamic\|static]` | Attach a local `.rules` or `.json` file (optionally hot-reloaded on disk changes) |
| `rules_dir <path> [dynamic\|static]` | Attach all `.rules`/`.json` files in a directory (optionally hot-reloaded) |
| `ruleset <name> { ... }` | Named rule source block (`source`/`file`/`dir`/`url`, `dynamic on\|off`) |
| `handle` / `handle_path` / `route` | Ordered path-matched route blocks (`reverse_proxy`, `root` + `file_server`, `respond`, `redir`, `header`) |
| `root` / `file_server` / `try_files` | Static file serving, directory browsing (`browse`), and SPA fallback |
| `respond` / `redir` / `header` | Fixed responses, HTTP redirects (`{scheme}`, `{host}`, `{uri}`, `{path}`, `{query}`), and response header mutations |
| `ai_bots <patterns...>` | Additional User-Agent substrings treated as AI crawlers |
| `suspicious_uas <patterns...>` | Additional User-Agent substrings treated as suspicious clients |
| `suspicious_paths <prefixes...>` | Additional path prefixes treated as suspicious probes |
| `tls <cert> <key>` | Per-site SNI certificate and private key paths |
| `allow \| verify \| block ...` | Shorthand custom firewall rule |
| `rule "name" { ... }` | Multi-condition custom firewall rule block |

---

## Duration Syntax

Time directives (`clearance_ttl` / `verify_timeout`, `challenge_ttl`, `rate_window`, `intel_refresh`, `remote_rules_refresh`, and `renew_interval`) accept:
- **Plain integer seconds**: `300`, `3600`, `86400`
- **Go durations**: `45s`, `30m`, `12h`, `1h30m`
- **Day & week shorthand**: `1d`, `7d`, `1d12h`, `2w`

---

## Formatting & Validation

```bash
# Validate syntax & print per-site summary (including clearance_ttl and remote rules)
./gymbag validate --config Gymbagfile

# Format into canonical 4-space indentation in place
./gymbag fmt --overwrite Gymbagfile
```

## JSON Configuration

`gymbag init` generates `gymbag.json` by default (use `--format gymbagfile` for Caddy-style syntax):

```bash
./gymbag init -d app.example.com -u :3000 --verify-timeout 12h --rules-dir ./rules.d --rules-dynamic
```

All fields map directly to the JSON schema (`sites`, `bind_host`, `http_port`, `https_port`, `admin_addr`, `tls`, `intel`, `remote_rules_refresh`, `rules_watch_interval`, `redis_url`, `secret`, `trusted_proxies`, `defaults`), with per-site `routes` (`path`, `strip_prefix`, `upstreams`, `root`, `index`, `browse`, `fallback`, `respond`, `redirect`, `headers`), `headers`, `waf.clearance_ttl` (integer seconds), `waf.remote_rules` (`[]string`), and `waf.rule_sources` (`[{"source", "dynamic", "name"}]`).

You can also convert an existing `nginx.conf`, Apache `httpd.conf`/`apache2.conf`, or `Caddyfile` into `gymbag.json`:

```bash
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```
