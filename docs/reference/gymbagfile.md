# `Gymbagfile` & JSON Schema Reference

By default, Gymbag probes `./Gymbagfile` or `./Caddyfile` in the working directory (or `--config <path>` / `GYMBAG_CONFIG`), and also accepts `.json` files.

## Global Options Block `{ ... }`

An optional top-level `{ ... }` block configures server-wide TLS, ACME, networking, threat intelligence, and default WAF policies inherited by all sites:

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

    # Optional Redis store for shared PoW nonces and rate limits
    # redis redis://127.0.0.1:6379/0

    # IP threat intelligence & AI crawler IP feeds
    ip_security on
    ip_security_action verify
    ip_reputation on
    ai_ip_list on
    intel_refresh 21600

    # Global custom rules evaluated before site rules
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
| `cert_dir` | Directory for certs, local CA, secret, and intel cache | `certs` |
| `renew_interval` | Certificate renewal check interval (seconds) | `43200` |
| `renew_within_days` | Renew certificates within N days of expiry | `30` |
| `auto_hosts` | Manage `/etc/hosts` entries for `localhost` / `*.internal` (`on`/`off`) | `on` |
| `hsts` | Send `Strict-Transport-Security` header (`on`/`off`) | `on` for public domains |
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
| `intel_refresh` | Feed refresh interval in seconds | `21600` |
| `threat_feed` / `ai_ip_feed` | Additional custom IP feed URLs | — |

---

## Site Block Directives

```caddyfile
app.example.com, www.example.com {
    reverse_proxy http://localhost:3000 http://localhost:3001
    verify 3
    difficulty 4
    rate_limit 240 60
    challenge_ttl 300
    clearance_ttl 86400
    cookie_name waf_clearance
    cookie_secure true
    default_rules on

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
| `rate_limit <count> [window_sec]` | Per-IP rate limit and optional window in seconds (`rate_window` also accepted) |
| `challenge_ttl <sec>` | PoW challenge nonce expiry in seconds |
| `clearance_ttl <sec>` | `waf_clearance` cookie lifetime in seconds |
| `cookie_name <name>` | Clearance cookie name |
| `cookie_secure <bool>` | Whether clearance cookie sets `Secure` |
| `default_rules <on\|off>` | Toggle built-in baseline rules |
| `tls <cert> <key>` | Per-site SNI certificate and private key paths |
| `allow \| verify \| block ...` | Shorthand custom firewall rule |
| `rule "name" { ... }` | Multi-condition custom firewall rule block |

---

## Formatting & Validation

```bash
# Validate syntax & print per-site summary
./gymbag validate --config Gymbagfile

# Format into canonical 4-space indentation in place
./gymbag fmt --overwrite Gymbagfile
```

## JSON Configuration

You can generate a `.json` config directly with:

```bash
./gymbag init --format json -d app.example.com -u :3000
```

All fields map directly to the JSON schema (`sites`, `bind_host`, `http_port`, `https_port`, `admin_addr`, `tls`, `intel`, `redis_url`, `secret`, `trusted_proxies`, `defaults`).
