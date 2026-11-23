# REST APIs & Redis

## Public WAF Endpoints

Served automatically on protected websites:

| Method | Path | Description |
| :--- | :--- | :--- |
| `GET` | `/.waf/v1/session` | Returns `site`, `protected`, `verify_level` (`0`–`3`), `verify_mode`, `clearance_ttl`, `cleared`, and (when cleared) `expires_at` / `expires_in` |
| `DELETE` | `/.waf/v1/session` | Clears the visitor's clearance cookie (`204 No Content`) |
| `POST` | `/.waf/v1/challenges` | Issues a single-use SHA-256 PoW challenge (`201 Created`) |
| `POST` | `/.waf/v1/verifications` | Verifies a PoW solution, sets the signed clearance cookie, and returns `{"ok": true, "expires_in": <ttl>}` (`/.waf/verify` alias supported) |
| `GET` | `/.waf/assets/{name}` | Serves embedded challenge PNG assets (`/.waf/v1/assets/{name}` alias, cached via `ETag`) |

## Verification Portal Endpoints (SSH & Minecraft)

Served on `verify_port` (default `8443`), or on `https_port` (`443`) when `verify_port` matches `https_port` and HTTP sites are enabled:

| Method | Path | Description |
| :--- | :--- | :--- |
| `GET` | `/v/{token}` | Serves the browser PoW page for an SSH or Minecraft gate ticket (8-char tokens matched case-insensitively; `/verify/{token}` alias supported) |
| `POST` | `/.waf/gate/verify` | Verifies `{token, challenge_id, nonce, verified}` and stores an IP clearance (`{"ok": true, "service": ..., "client_ip": ..., "expires_in": <ttl>}`) |
| `GET` | `/.waf/assets/{name}` | Embedded mascot PNGs (`/.waf/v1/assets/{name}` alias supported) |
| `GET` | `/.well-known/acme-challenge/{token}` | ACME HTTP-01 challenge response for portal domain or RFC 8738 IP certs |

## Local Admin REST API

Enable with `admin 127.0.0.1:2019`, `--admin 127.0.0.1:2019`, or `GYMBAG_ADMIN=127.0.0.1:2019`.

::: warning Bind to Loopback or Private Interfaces Only
The Admin API takes no authentication token and mutates live routing and firewall state without restarting Gymbag. Always bind it to `127.0.0.1:2019` or a private management interface.
:::

| Method | Path | Description |
| :--- | :--- | :--- |
| `GET` | `/api/v1/health` | Status, `sites_count`, `ssh_enabled`, `minecraft_enabled`, `tls_source`, `threat_entries`, `ai_entries`, `vpn_entries`, `remote_rules` count, and `minecraft_server` status (`running`, `pid`, `console_enabled`, `java_major`, `required_java`, `mc_version`, `restarts`, `eula_blocked`, `last_error`) when `server_jar` is configured |
| `GET` | `/api/v1/tls` | Active TLS source, cert path, SSL provider, HSTS state, and supported CA catalog |
| `POST` | `/api/v1/tls/renew` | Force immediate certificate renewal and atomic reload |
| `GET` | `/api/v1/intel` | Threat, AI crawler, and VPN/Tor feed counts (`vpn_detection_enabled`, `vpn_entries`, `vpn_cache_size`, `reputation_cache_size`), refresh timestamps, and per-feed status (`threat`, `ai`, `vpn`) |
| `POST` | `/api/v1/intel/refresh` | Refresh all threat, VPN, and AI feeds immediately |
| `GET` | `/api/v1/rules/remote` | Remote rule URLs, loaded rule counts, cache status, and last errors |
| `POST` | `/api/v1/rules/remote/refresh` | Fetch remote rule URLs and reload the WAF engine atomically |
| `GET` | `/api/v1/config` | Dump active runtime config (`secret`, `key_password`, and `eab_hmac_key` redacted) |
| `PUT` | `/api/v1/config` | Validate and replace the active runtime configuration |
| `GET` | `/api/v1/sites` | List configured websites and WAF policies |
| `POST` | `/api/v1/sites` | Add a website at runtime (`201 Created`) |
| `GET` | `/api/v1/sites/{domain}` | Inspect one website's configuration |
| `PUT` | `/api/v1/sites/{domain}` | Replace a website's configuration |
| `PATCH` | `/api/v1/sites/{domain}` | Live-patch `verify_level`, `protect`, `difficulty`, `clearance_ttl`, `upstreams`, `rate_limit`, `rules`, `remote_rules`, `routes`, `headers`, `auth`, `clear_auth`, or `rule_sources` |
| `DELETE` | `/api/v1/sites/{domain}` | Remove a website (`204 No Content`; returns `409 Conflict` on the last remaining site when neither SSH nor Minecraft is active) |
| `GET` | `/api/v1/sites/{domain}/rules` | List local custom rules on a website |
| `POST` | `/api/v1/sites/{domain}/rules` | Append a custom rule (`201 Created`) |
| `PUT` | `/api/v1/sites/{domain}/rules` | Replace all custom rules on a website |
| `DELETE` | `/api/v1/sites/{domain}/rules/{index_or_name}` | Delete a custom rule by numeric index or `name` (`204 No Content`) |
| `GET` | `/api/v1/minecraft/console` | Return JSON console snapshot (`running`, `pid`, `console_enabled`, `lines`) or live SSE stream (`?stream=1` / `Accept: text/event-stream`), supporting `?since=<seq>` (`Last-Event-ID`) and `?limit=<n>`; returns `403 console_disabled` when off |
| `POST` | `/api/v1/minecraft/console` | Execute `{"command":"..."}` on the managed Minecraft server console (supports `"start"` when paused after a manual `stop`); returns `403 console_disabled` when off or `409 not_running` when stopped |

### Live Mitigation Examples

```bash
# Tighten app.example.com to verify level 0 with a 30-minute clearance window
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"verify_level": 0, "difficulty": 5, "clearance_ttl": 1800}'

# Patch path routes, response headers, and local rule sources live
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"routes": [{"path": "/static/*", "strip_prefix": "/static", "root": "./dist", "browse": true}], "headers": {"X-Served-By": "gymbag"}, "rule_sources": [{"source": "./rules.d", "dynamic": true}]}'

# Block an abusive CIDR without editing Gymbagfile
curl -X POST http://127.0.0.1:2019/api/v1/sites/app.example.com/rules \
  -H "Content-Type: application/json" \
  -d '{"name": "drop-abusive-subnet", "action": "block", "ips": ["198.51.100.0/24"]}'

# Refresh remote rules and threat/VPN/AI feeds
curl -X POST http://127.0.0.1:2019/api/v1/rules/remote/refresh
curl -X POST http://127.0.0.1:2019/api/v1/intel/refresh

# Attach / send a command to the managed Minecraft server console (requires minecraft.console: true)
curl -N http://127.0.0.1:2019/api/v1/minecraft/console?stream=1
curl -X POST http://127.0.0.1:2019/api/v1/minecraft/console \
  -H "Content-Type: application/json" \
  -d '{"command": "say Maintenance in 5 minutes"}'
```

## Redis Shared State

Gymbag includes a pure-Go RESP2 Redis client supporting `redis://` and `rediss://` URLs. Point `redis` (or `GYMBAG_REDIS_URL`) at your Redis instance to share:

- Single-use HTTP and gate PoW challenges (`gymbag:challenge:<id>`)
- Fixed-window per-IP rate limits (`gymbag:rate:<key>`)
- SSH and Minecraft browser verification tickets (`gymbag:ticket:<token>`) and IP clearances (`gymbag:clearance:<service>:<ip>`)

If Redis is unreachable at startup, Gymbag logs a warning and falls back to its 64-shard in-memory store. See the [Docker & Docker Compose guide](/guide/docker) for a ready-to-run Compose stack.
