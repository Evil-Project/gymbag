# Verify Levels (`0`–`3`) & Proof-of-Work

One Gymbag process can front multiple websites (with round-robin upstream load balancing and RFC 6455 WebSocket upgrades) alongside SSH and Minecraft protection. Each site and service sets its policy with `verify <0-3>`, or turns off PoW challenges with `protect off`.

<VerifyLevelMatrix />

## Challenge States

Gymbag embeds the PoW challenge HTML, SHA-256 browser solver, and three mascot images directly in the binary:

<div class="gb-mascot-strip">
  <div class="gb-mascot-tile">
    <img src="/mascot-pensive.png" alt="Verifying challenge mascot" />
    <strong>Solving PoW</strong>
    <span class="gb-badge challenge">Challenge Issued</span>
  </div>
  <div class="gb-mascot-tile">
    <img src="/mascot-happy.png" alt="Cleared visitor mascot" />
    <strong>Clearance Granted</strong>
    <span class="gb-badge pass">waf_clearance Set</span>
  </div>
  <div class="gb-mascot-tile">
    <img src="/mascot-reject.png" alt="Blocked request mascot" />
    <strong>Rule / Threat Block</strong>
    <span class="gb-badge block">403 Forbidden</span>
  </div>
</div>

## Multi-Site & Multi-Service Example

```caddyfile
app.example.com, www.example.com {
    reverse_proxy http://localhost:3000 http://localhost:3001
    verify 3
    difficulty 4
    verify_timeout 12h
}

docs.example.com {
    reverse_proxy http://localhost:4000
    verify 2
    verify_timeout 7d
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

ssh {
    upstream 127.0.0.1:41822
    verify 3
}
```

At levels `1` and `3`, suspicious traffic includes **VPN, proxy, and Tor exit IPs** (`vpn_detection on`) in addition to threat feeds, live Shodan/StopForumSpam hits, missing browser headers, automation clients, exploit probes, and scripted SSH/Minecraft clients.

## Verified Status Timeout (`verify_timeout` / `clearance_ttl`)

On HTTP/HTTPS sites, solving the SHA-256 PoW challenge issues an `HttpOnly`, `SameSite=Lax` HMAC-signed cookie (`waf_clearance` by default) bound to host and client IP. On SSH and Minecraft, solving the browser verification link stores an IP clearance record for that service.

The default lifetime is **24 hours** (`86400` seconds). Override it globally or per site/service with integer seconds or duration strings (`30m`, `12h`, `1h30m`, `7d`, `2w`):

::: code-group
```caddyfile [Gymbagfile]
{
    verify_timeout 12h
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
    verify_timeout 30m
}

docs.example.com {
    reverse_proxy http://localhost:4000
    verify 2
    clearance_ttl 7d
}
```

```bash [CLI Flags]
gymbag init -d app.example.com -u :3000 --verify-timeout 6h
gymbag run --config Gymbagfile --verify-timeout 30m
gymbag reverse-proxy --from app.internal --to :3000 --verify 0 --verify-timeout 15m
```

```bash [Environment Variables]
GYMBAG_VERIFY_TIMEOUT=12h gymbag
GYMBAG_CLEARANCE_TTL=3600 gymbag
```

```bash [Live Admin API]
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/staging.example.com \
  -H "Content-Type: application/json" \
  -d '{"clearance_ttl": 1800}'
```
:::

- Accepted directive aliases in `Gymbagfile`: `verify_timeout`, `verified_timeout`, `clearance_timeout`, `clearance_ttl`.
- Accepted env var aliases: `GYMBAG_CLEARANCE_TTL`, `GYMBAG_VERIFY_TIMEOUT`, `GYMBAG_VERIFIED_TIMEOUT`.
- `POST /.waf/v1/verifications` returns `{"ok": true, "expires_in": <seconds>}` and sets `Max-Age` to the configured TTL.
- `GET /.waf/v1/session` returns `clearance_ttl`, plus `expires_at` (RFC 3339 UTC) and `expires_in` for cleared browsers.

## Difficulty, Challenge Window, and Rate Limits

- `difficulty` (`1`–`5`, default `4`): Number of leading hex zeros required in the SHA-256 digest (`0000...` at difficulty `4`).
- `challenge_ttl` (default `300`s / `5m`): Expiry for single-use challenge nonces.
- `clearance_ttl` / `verify_timeout` (default `86400`s / `24h`): Lifetime of the clearance cookie or TCP IP clearance.
- `rate_limit` / `rate_window`: Fixed-window per-IP request limiter. Sites default to `240` requests per `60`s; `ssh` and `minecraft` blocks default to `60` attempts per `60`s.
