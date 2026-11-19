# CLI & `GYMBAG_*` Environment Variables

Precedence order is strictly:

**Config file → `.env` / `GYMBAG_*` environment variables → CLI flags**

Gymbag automatically loads `.env` from the current working directory if present (without overriding already-set OS environment variables).

## CLI Subcommands

```bash
# Generate config (interactive wizard or flag-driven)
gymbag init [flags] [output_path]

# Start server using ./Gymbagfile or ./Caddyfile
gymbag

# Start with explicit config and hot-reload on file changes (polls every 2s)
gymbag run --config Gymbagfile --watch

# Manage OS system services (systemd, launchd, openrc, supervisor)
gymbag service <install|start|stop|restart|status|uninstall|render> [flags]

# Ad-hoc reverse proxy without a config file
gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3 --verify-timeout 12h

# Validate configuration and print resolved per-site policies
gymbag validate --config Gymbagfile

# Format Gymbagfile in place
gymbag fmt --overwrite Gymbagfile

# Print version
gymbag version
```

### Server & Common Flags (`run` / default / `reverse-proxy`)

| Flag | Description |
| :--- | :--- |
| `-c`, `--config <path>` | Path to `Gymbagfile`, `Caddyfile`, or `.json` config |
| `-d`, `--domain <list>` | Override downstream domain(s), comma-separated (`--from` on `reverse-proxy`) |
| `-u`, `--upstream <url>` | Override upstream URL (`http://localhost:3000` or `:3000`; `--to` on `reverse-proxy`) |
| `-l`, `--verify <0-3\|off>` | Override verify level (`0`, `1`, `2`, `3`, or `off`) |
| `--verify-timeout <dur>` | Override verified clearance cookie timeout (e.g. `30m`, `12h`, `7d`, or seconds; alias `--clearance-ttl`) |
| `--remote-rules <url>` | Attach remote firewall rules URL (http/https, repeatable or comma-separated) |
| `--remote-rules-refresh <dur>` | Refresh interval for remote rules (default `5m` / `300s`) |
| `--no-protect` | Disable WAF verification for CLI-configured sites |
| `--ssl-provider <name>` | Override ACME / SSL provider |
| `--admin <addr>` | Enable local REST Admin API (e.g. `127.0.0.1:2019` or `off`) |
| `--http-port <port>` | HTTP redirect & ACME HTTP-01 port (default `80`) |
| `--https-port <port>` | HTTPS gateway port (default `443`) |
| `--watch` | Watch config file and hot-reload routes, rules, and SNI certs (`run` only) |

---

## Environment Variables (`GYMBAG_*`)

| Variable | Description |
| :--- | :--- |
| `GYMBAG_CONFIG` | Path to `Gymbagfile`, `Caddyfile`, or `.json` config |
| `GYMBAG_BIND` | Bind address (`127.0.0.1` for local, `0.0.0.0` when public domains exist) |
| `GYMBAG_HTTP_PORT` / `GYMBAG_HTTPS_PORT` | HTTP and HTTPS listen ports (defaults `80` and `443`) |
| `GYMBAG_DOMAIN` / `GYMBAG_DOMAINS` | Comma-separated downstream domain(s) |
| `GYMBAG_UPSTREAM` | Default upstream target URL |
| `GYMBAG_UPSTREAMS` | Semicolon-separated host map: `app.example.com=http://web:3000;api.example.com=http://api:8080` |
| `GYMBAG_PROTECT` | Enable or disable WAF protection across sites (`on` / `off`) |
| `GYMBAG_PROTECT_DOMAINS` | Comma-separated subset of domains with WAF protection enabled |
| `GYMBAG_VERIFY_LEVEL` | Verify level: `0`, `1`, `2`, or `3` (aliases: `GYMBAG_VERIFY`, `GYMBAG_VERIFY_MODE`, `GYMBAG_MODE`, `GYMBAG_TRIGGER`) |
| `GYMBAG_DEFAULT_RULES` | Enable or disable built-in baseline firewall rules (`on` / `off`) |
| `GYMBAG_REMOTE_RULES` | Comma- or whitespace-separated HTTP/HTTPS remote firewall rules URLs |
| `GYMBAG_REMOTE_RULES_REFRESH` | Remote rules refresh interval in seconds or duration (default `300` / `5m`) |
| `GYMBAG_DIFFICULTY` | SHA-256 PoW difficulty (`1`–`5`, default `4`) |
| `GYMBAG_CHALLENGE_TTL` | Challenge nonce expiry in seconds or duration (default `300` / `5m`) |
| `GYMBAG_CLEARANCE_TTL` | Verified status timeout in seconds or duration like `30m`, `12h`, `7d` (default `86400` / `24h`; aliases: `GYMBAG_VERIFY_TIMEOUT`, `GYMBAG_VERIFIED_TIMEOUT`) |
| `GYMBAG_COOKIE_NAME` / `GYMBAG_COOKIE_SECURE` | Clearance cookie name (`waf_clearance`) and `Secure` flag (`true`) |
| `GYMBAG_RATE_LIMIT` / `GYMBAG_RATE_WINDOW` | Per-IP limit (`240`) and window in seconds or duration (`60` / `1m`) |
| `GYMBAG_TLS_MODE` | `auto`, `acme`, `local`, `selfsigned`, or `file` |
| `GYMBAG_SSL_PROVIDER` | `letsencrypt`, `letsencrypt_staging`, `zerossl`, `google`, `ssl.com`, `litessl`, `buypass`, `local`, or URL |
| `GYMBAG_ACME_EMAIL` | Contact email for ACME registration and ZeroSSL auto-EAB |
| `GYMBAG_ACME_DIRECTORY` | Explicit RFC 8555 ACME v2 directory URL |
| `GYMBAG_EAB_KID` / `GYMBAG_EAB_HMAC_KEY` | External Account Binding Key ID and HMAC key |
| `GYMBAG_ACCOUNT_KEY_FILE` | Path to ECDSA or RSA ACME account key PEM |
| `GYMBAG_CERT_FILE` / `GYMBAG_KEY_FILE` | TLS certificate and private key paths for `file` mode |
| `GYMBAG_KEY_PASSWORD` | Passphrase for encrypted PEM private keys |
| `GYMBAG_CERT_DIR` | Directory for CA certs, ACME state, intel cache, and remote rules cache (default `certs`) |
| `GYMBAG_RENEW_INTERVAL` / `GYMBAG_RENEW_WITHIN_DAYS` | Renewal check interval (`43200`s / `12h`) and threshold (`30` days) |
| `GYMBAG_AUTO_HOSTS` | Automatic `/etc/hosts` management for local domains (`on` / `off`) |
| `GYMBAG_HSTS` | Override `Strict-Transport-Security` header (`on` / `off`) |
| `GYMBAG_IP_SECURITY` / `GYMBAG_IP_SECURITY_ACTION` | Toggle threat feeds (`on`/`off`) and action (`verify` or `block`) |
| `GYMBAG_IP_REPUTATION` | Toggle Shodan InternetDB + StopForumSpam lookups (`on`/`off`) |
| `GYMBAG_AI_IP_LIST` | Toggle official AI crawler IP lists (`on`/`off`) |
| `GYMBAG_INTEL_REFRESH` | Threat & AI feed refresh interval in seconds or duration (default `21600` / `6h`) |
| `GYMBAG_REDIS_URL` | Optional Redis connection URL (`redis://[:password@]host:port/db`) |
| `GYMBAG_SECRET` | HMAC secret for clearance cookies |
| `GYMBAG_ADMIN` | Local Admin REST API listen address (e.g. `127.0.0.1:2019` or `off`) |
