# CLI & `GYMBAG_*` Environment Variables

Precedence order is:

**Config file -> `.env` / `GYMBAG_*` environment variables -> CLI flags**

Gymbag loads `.env` from the working directory if present without overwriting existing environment variables.

## CLI Subcommands

```bash
# Generate config (interactive wizard or flags); writes gymbag.json by default
gymbag init [flags] [output_path]
gymbag init --format gymbagfile -o Gymbagfile -d app.example.com -u :3000

# Start using ./gymbag.json, ./Gymbagfile, or ./Caddyfile
gymbag

# Start with explicit config and hot-reload on file edits (polls every 2s)
gymbag run --config Gymbagfile --watch

# Manage OS services (systemd, launchd, openrc, supervisor)
gymbag service <install|start|stop|restart|status|uninstall|render> [flags]

# Manage sites, routes, global settings, and config conversions
gymbag config <list|show|add|update|remove|site|route|set|unset|convert> [flags]

# Manage rule files, folders, and inline rules
gymbag ruleset <list|add|update|remove|validate|init|rule> [flags]

# Ad-hoc HTTP/HTTPS reverse proxy without a config file
gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3 --verify-timeout 12h

# Validate config and print resolved service summary
gymbag validate --config Gymbagfile

# Format Gymbagfile in place (4-space indentation)
gymbag fmt --overwrite Gymbagfile

# Print version
gymbag version
```

### Server Flags (`run` / default / `reverse-proxy`)

| Flag | Description |
| :--- | :--- |
| `-c`, `--config <path>` | Path to `gymbag.json`, `Gymbagfile`, `Caddyfile`, `nginx.conf`, or Apache config |
| `-d`, `--domain <list>` | Override downstream domain(s), comma-separated (`--from` on `reverse-proxy`) |
| `-u`, `--upstream <url>` | Override upstream URL (`http://localhost:3000` or `:3000`; `--to` on `reverse-proxy`) |
| `-l`, `--verify <0-3\|off>` | Override verify level (`0`, `1`, `2`, `3`, or `off`; aliases `--verify-level`, `-m`, `--mode`) |
| `--verify-timeout <dur>` | Override clearance lifetime (`30m`, `12h`, `7d`, or seconds; alias `--clearance-ttl`) |
| `--remote-rules <url>` | Attach remote firewall rules URL (repeatable or comma-separated) |
| `--remote-rules-refresh <dur>` | Refresh interval for remote rules (default `5m` / `300s`) |
| `--no-protect` | Disable PoW verification for CLI-configured sites |
| `--ssl-provider <name>` | Override ACME / SSL provider |
| `--admin <addr>` | Enable local REST Admin API (`127.0.0.1:2019` or `off`) |
| `--http-port <port>` | HTTP redirect & ACME HTTP-01 port (default `80`) |
| `--https-port <port>` | HTTPS gateway port (default `443`) |
| `--watch` | Watch config file and hot-reload changes (`run` / default only) |

## Config Management (`gymbag config`)

All `config` subcommands accept `-c`, `--config <path>` (defaults to auto-detected config file).

```bash
gymbag config list
gymbag config add -d api.example.com -u :8080 -l 1
gymbag config update -d api.example.com -l 3 --verify-timeout 6h
gymbag config remove -d api.example.com
gymbag config set admin 127.0.0.1:2019
gymbag config unset admin
gymbag config route add -d app.example.com --path '/static/*' --strip-prefix /static --root ./dist --browse
gymbag config route list -d app.example.com
gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```

| Subcommand | Description |
| :--- | :--- |
| `list` / `show` | List sites & TCP services / print formatted config (`--format json\|gymbagfile`) |
| `add` | Add a site: `-d`, `-u`, `-l`, `--difficulty`, `--verify-timeout`, `--no-protect`, `--no-default-rules`, `--cert`, `--key`, `--root`, `--browse`, `--fallback`, `--respond`, `--status`, `--redir`, `--code`, `--header Name=Value\|-Name`, `-f` |
| `update` | Update a site: `-d`, `-u`, `-l`, `--difficulty`, `--verify-timeout`, `--protect on\|off`, `--default-rules on\|off`, `--cert`, `--key`, `--clear-tls`, `--header`, `--remove-header` |
| `remove` | Remove a site (`-d`) |
| `route list\|add\|update\|remove` | Manage path routes: `-d`, `--path`, `--strip-prefix`, `-u`, `--root`, `--browse`, `--fallback`, `--respond`, `--status`, `--redir`, `--code`, `--header`, `--index` |
| `set <key> <value>` / `unset <key>` | Set or reset global keys (`email`, `ssl_provider`, `http_port`, `https_port`, `bind`, `admin`, `cert_dir`, `redis`, `ip_security`, `ip_security_action`, `ip_reputation`, `ai_ip_list`, `intel_refresh`, `remote_rules_refresh`, `rules_watch_interval`, `verify_timeout`, `auto_hosts`, `hsts`, `eab_kid`, `eab_hmac_key`) |
| `convert` | Convert nginx, Apache, Caddyfile, Gymbagfile, or JSON: `-c <in>`, `-o <out>`, `--format json\|gymbagfile`, `-f` |

## Ruleset Management (`gymbag ruleset`)

```bash
gymbag ruleset init ./rules/custom.rules
gymbag ruleset add --source ./rules --dynamic --name local-dir
gymbag ruleset update local-dir --static
gymbag ruleset remove local-dir
gymbag ruleset validate ./rules
gymbag ruleset rule add --file ./rules/custom.rules --action block --name bad-bot --ua "*BadBot*"
```

| Subcommand | Description |
| :--- | :--- |
| `list` | List linked rule sources (`-c`, `-d`) |
| `add` | Link a rule file, folder, or URL: `--source`, `--name`, `--dynamic` / `--static`, `-d <domain>` or `--global`, `-c` |
| `update` | Change a linked ruleset: `--target` (or positional source/name), `--source`, `--name`, `--dynamic` / `--static`, `-d` / `--global`, `-c` |
| `remove` | Unlink a ruleset: `--source <source-or-name>`, `-d` / `--global`, `-c` |
| `validate <path>` | Validate a rule file or rules directory |
| `init <path>` | Create a starter ruleset (`-o`, `--format rules\|json`, `-f`) |
| `rule list\|add\|update\|remove` | Manage single rules in `--file` (or inline in config via `-c` / `-d`): `--name`, `--index`, `--action allow\|verify\|block`, `--ip`, `--ua`, `--path`, `--query`, `--method`, `--header`, `--cookie` |

## Environment Variables (`GYMBAG_*`)

| Variable | Description |
| :--- | :--- |
| `GYMBAG_CONFIG` | Path to `gymbag.json`, `Gymbagfile`, `Caddyfile`, `nginx.conf`, or Apache config |
| `GYMBAG_BIND` | Bind address (`127.0.0.1` for local-only HTTP, `0.0.0.0` when public domains, SSH, or Minecraft are active) |
| `GYMBAG_HTTP_PORT` / `GYMBAG_HTTPS_PORT` | HTTP and HTTPS listen ports (defaults `80` and `443`) |
| `GYMBAG_DOMAIN` / `GYMBAG_DOMAINS` | Comma-separated downstream domain(s) |
| `GYMBAG_UPSTREAM` | Default upstream target URL |
| `GYMBAG_UPSTREAMS` | Semicolon-separated host map: `app.example.com=http://web:3000;api.example.com=http://api:8080` |
| `GYMBAG_PROTECT` / `GYMBAG_PROTECT_DOMAINS` | Toggle WAF protection across sites (`on`/`off`), or restrict protection to listed domains |
| `GYMBAG_VERIFY_LEVEL` | Verify level: `0`, `1`, `2`, `3`, or `off` (aliases: `GYMBAG_VERIFY`, `GYMBAG_VERIFY_MODE`, `GYMBAG_MODE`, `GYMBAG_TRIGGER`) |
| `GYMBAG_DEFAULT_RULES` | Built-in baseline rules (`on` / `off`) |
| `GYMBAG_REMOTE_RULES` / `GYMBAG_REMOTE_RULES_REFRESH` | Remote rule URLs and refresh interval (default `300`s / `5m`) |
| `GYMBAG_RULES_WATCH_INTERVAL` | Polling interval for `dynamic` local rule sources (default `2`s / `2s`) |
| `GYMBAG_DIFFICULTY` | SHA-256 PoW difficulty (`1`–`5`, default `4`) |
| `GYMBAG_CHALLENGE_TTL` / `GYMBAG_CLEARANCE_TTL` | Challenge expiry (`300`s / `5m`) and clearance lifetime (`86400`s / `24h`; aliases: `GYMBAG_VERIFY_TIMEOUT`, `GYMBAG_VERIFIED_TIMEOUT`) |
| `GYMBAG_COOKIE_NAME` / `GYMBAG_COOKIE_SECURE` | Clearance cookie name (`waf_clearance`) and `Secure` flag (`true`) |
| `GYMBAG_RATE_LIMIT` / `GYMBAG_RATE_WINDOW` | Per-IP request limit (`240`) and window (`60`s / `1m`) |
| `GYMBAG_TLS_MODE` / `GYMBAG_SSL_PROVIDER` | TLS mode (`auto`, `acme`, `local`, `selfsigned`, `file`) and CA provider or URL |
| `GYMBAG_ACME_EMAIL` / `GYMBAG_ACME_DIRECTORY` | ACME contact email (and ZeroSSL auto-EAB) and explicit ACME directory URL |
| `GYMBAG_EAB_KID` / `GYMBAG_EAB_HMAC_KEY` | External Account Binding Key ID and HMAC key |
| `GYMBAG_ACCOUNT_KEY_FILE` | Path to ECDSA or RSA ACME account key PEM |
| `GYMBAG_CERT_FILE` / `GYMBAG_KEY_FILE` / `GYMBAG_KEY_PASSWORD` | Cert, key, and key passphrase for `file` mode |
| `GYMBAG_CERT_DIR` | Directory for certs, CA keys, secret, intel cache, and rule cache (default `certs`) |
| `GYMBAG_RENEW_INTERVAL` / `GYMBAG_RENEW_WITHIN_DAYS` | Renewal check interval (`43200`s / `12h`) and threshold (`30` days) |
| `GYMBAG_AUTO_HOSTS` / `GYMBAG_HSTS` | Automatic `/etc/hosts` management (`on`/`off`) and `Strict-Transport-Security` override (`on`/`off`) |
| `GYMBAG_IP_SECURITY` / `GYMBAG_IP_SECURITY_ACTION` | Toggle threat feeds (`on`/`off`) and action (`verify` or `block`) |
| `GYMBAG_IP_REPUTATION` / `GYMBAG_AI_IP_LIST` | Toggle Shodan InternetDB + StopForumSpam lookups (`on`/`off`) and official AI crawler lists (`on`/`off`) |
| `GYMBAG_VPN_DETECTION` | Toggle free VPN/proxy/Tor detection in suspicious checks (`on`/`off`, default `on`) |
| `GYMBAG_INTEL_REFRESH` | Threat, VPN, and AI feed refresh interval (default `21600`s / `6h`) |
| `GYMBAG_VERIFY_URL` / `GYMBAG_VERIFY_PORT` | Shared HTTPS verify portal URL (blank = server IP + IP cert) and port (default `8443`) |
| `GYMBAG_SSH_ENABLED` / `GYMBAG_SSH_PORT` / `GYMBAG_SSH_UPSTREAM` | Enable SSH protection (default `22`) and set backend `sshd` target |
| `GYMBAG_SSH_VERIFY_LEVEL` / `GYMBAG_SSH_PROTECT` | SSH verify level (`0`–`3` or `off`) and protection toggle (`on`/`off`) |
| `GYMBAG_MC_ENABLED` / `GYMBAG_MC_PORT` / `GYMBAG_MC_UPSTREAM` | Enable Minecraft protection (default `25565`) and set backend target (`GYMBAG_MINECRAFT_*` aliases supported) |
| `GYMBAG_MC_VERIFY_LEVEL` / `GYMBAG_MC_PROTECT` | Minecraft verify level (`0`–`3` or `off`) and protection toggle (`on`/`off`) |
| `GYMBAG_REDIS_URL` | Redis URL (`redis://` or `rediss://`) |
| `GYMBAG_SECRET` / `GYMBAG_ADMIN` | HMAC clearance secret and Admin REST API listen address (`127.0.0.1:2019` or `off`) |
