# REST APIs, Docker Compose & Redis

## Public WAF Endpoints

Served automatically on protected sites:

| Method | Path | Description |
| :--- | :--- | :--- |
| `GET` | `/.waf/v1/session` | Returns current clearance status, `protected`, `verify_level` (`0`–`3`), and `verify_mode` |
| `DELETE` | `/.waf/v1/session` | Clears the visitor's clearance cookie (`204 No Content`) |
| `POST` | `/.waf/v1/challenges` | Issues a single-use SHA-256 PoW challenge (`201 Created`) |
| `POST` | `/.waf/v1/verifications` | Verifies a PoW solution and sets the signed clearance cookie (`/.waf/verify` alias supported) |
| `GET` | `/.waf/assets/{name}` | Serves embedded challenge PNG assets (`/.waf/v1/assets/{name}` alias, `ETag` cached) |

---

## Local Admin REST API

Enable via `admin 127.0.0.1:2019`, `--admin 127.0.0.1:2019`, or `GYMBAG_ADMIN=127.0.0.1:2019`.

::: warning Bind to Loopback or Private Management Interfaces
The Admin API requires no authentication token and can mutate live routing and firewall rules without restarting Gymbag. Always bind it to `127.0.0.1:2019` or a private WireGuard/Unix interface.
:::

| Method | Path | Description |
| :--- | :--- | :--- |
| `GET` | `/api/v1/health` | Gateway status, site count, `tls_source`, `threat_entries`, `ai_entries` |
| `GET` | `/api/v1/tls` | Active TLS source, cert path, SSL provider, HSTS status, supported CA catalog |
| `POST` | `/api/v1/tls/renew` | Force immediate certificate renewal and atomic hot-reload |
| `GET` | `/api/v1/intel` | Per-feed threat & AI crawler counts, refresh timestamps, reputation cache stats |
| `POST` | `/api/v1/intel/refresh` | Trigger immediate background feed refresh |
| `GET` | `/api/v1/config` | Dump active runtime config (`secret`, `key_password`, `eab_hmac_key` redacted) |
| `PUT` | `/api/v1/config` | Atomically validate and replace the entire runtime configuration |
| `GET` | `/api/v1/sites` | List all configured websites and WAF policies |
| `POST` | `/api/v1/sites` | Add a new website at runtime (`201 Created`) |
| `GET` | `/api/v1/sites/{domain}` | Inspect a single website's configuration |
| `PUT` | `/api/v1/sites/{domain}` | Replace a website's configuration |
| `PATCH` | `/api/v1/sites/{domain}` | Live-patch `verify_level`, `protect`, `difficulty`, `upstreams`, `rate_limit`, or `rules` |
| `DELETE` | `/api/v1/sites/{domain}` | Remove a website (`204 No Content`; refuses to delete the last remaining site) |
| `GET` | `/api/v1/sites/{domain}/rules` | List custom firewall rules on a website |
| `POST` | `/api/v1/sites/{domain}/rules` | Append a custom firewall rule live (`201 Created`) |
| `PUT` | `/api/v1/sites/{domain}/rules` | Replace all custom firewall rules on a website |
| `DELETE` | `/api/v1/sites/{domain}/rules/{index_or_name}` | Delete a custom rule by numeric index or rule `name` (`204 No Content`) |

### Live Emergency Mitigation Examples

```bash
# Tighten app.example.com to verify level 0 (always challenge) during an L7 spike
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"verify_level": 0, "difficulty": 5}'

# Immediately block an abusive CIDR without editing Gymbagfile
curl -X POST http://127.0.0.1:2019/api/v1/sites/app.example.com/rules \
  -H "Content-Type: application/json" \
  -d '{"name": "drop-abusive-subnet", "action": "block", "ips": ["198.51.100.0/24"]}'

# Force an immediate refresh of IP threat & AI crawler lists
curl -X POST http://127.0.0.1:2019/api/v1/intel/refresh
```

---

## Docker Compose & Redis

The repository's `compose.yaml` runs Gymbag as non-root UID `10001` (with `NET_BIND_SERVICE`) alongside `redis:8-alpine`. Gymbag implements a pure-Go RESP2 Redis client so single-use PoW challenge nonces and fixed-window rate limits can be shared across container restarts or multiple replicas:

```bash
docker compose up -d --build
```

- Mounts `./Gymbagfile` read-only at `/app/Gymbagfile`
- Persists certificates and threat-intel cache in the `gymbag-certs` volume
- Passes `GYMBAG_REDIS_URL=redis://redis:6379/0` automatically
