# Verify Levels (`0`–`3`) & Proof-of-Work

One Gymbag process can front a pile of domains at once, round-robin across several upstreams, and pass WebSocket upgrades (RFC 6455) straight through. Each site picks its own policy with `verify <0-3>`, or opts out with `protect off`.

<VerifyLevelMatrix />

## Challenge States

Gymbag embeds its Proof-of-Work browser challenge page, SHA-256 solver script, and three mascot states directly into the Go binary:

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

## Multi-Site Example

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
```

## Custom Verified Status Timeout (`verify_timeout` / `clearance_ttl`)

Once a visitor completes the SHA-256 Proof-of-Work challenge, Gymbag issues an `HttpOnly`, `SameSite=Lax` HMAC-signed clearance cookie (`waf_clearance` by default) bound to both the site's hostname and the client IP.

By default, the verified status lasts **24 hours** (`86400` seconds). You can customize it globally or per site using integer seconds **or** human-readable duration strings (`30m`, `12h`, `1h30m`, `7d`, `2w`):

::: code-group
```caddyfile [Gymbagfile]
{
    # Global default verified timeout for all sites
    verify_timeout 12h
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
    # Short 30-minute clearance window for staging
    verify_timeout 30m
}

docs.example.com {
    reverse_proxy http://localhost:4000
    verify 2
    # Long 7-day clearance window for docs (clearance_ttl alias also works)
    clearance_ttl 7d
}
```

```bash [CLI Flags]
# Generate config with a 6-hour verified timeout
gymbag init -d app.example.com -u :3000 --verify-timeout 6h

# Or override at runtime
gymbag run --config Gymbagfile --verify-timeout 30m
gymbag reverse-proxy --from app.internal --to :3000 --verify 0 --verify-timeout 15m
```

```bash [Environment Variables]
# Both aliases accept integer seconds or durations (e.g. 30m, 12h, 7d)
GYMBAG_VERIFY_TIMEOUT=12h gymbag
GYMBAG_CLEARANCE_TTL=3600 gymbag
```

```bash [Live Admin API]
# Update clearance_ttl (in seconds) without restarting
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/staging.example.com \
  -H "Content-Type: application/json" \
  -d '{"clearance_ttl": 1800}'
```
:::

- Accepted directive aliases in `Gymbagfile`: `verify_timeout`, `verified_timeout`, `clearance_timeout`, and `clearance_ttl`.
- Accepted environment variable aliases: `GYMBAG_CLEARANCE_TTL`, `GYMBAG_VERIFY_TIMEOUT`, and `GYMBAG_VERIFIED_TIMEOUT`.
- `POST /.waf/v1/verifications` returns `{"ok": true, "expires_in": <seconds>}` and sets the cookie's `Max-Age` to the site's configured timeout.
- `GET /.waf/v1/session` returns `clearance_ttl` plus `expires_at` (RFC 3339 UTC) and `expires_in` (remaining seconds) whenever the visitor holds an active clearance cookie.

## Difficulty, Challenge Window & Rate Limits

- **`difficulty`** (`1`–`5`, default `4`): Number of leading hex zeros required in the SHA-256 hash (`0000...` at difficulty `4`).
- **`challenge_ttl`** (default `300` seconds / `5m`): Lifetime of a single-use PoW challenge nonce. Accepts seconds or durations (e.g. `5m`).
- **`clearance_ttl` / `verify_timeout`** (default `86400` seconds / `24h`): Lifetime of the verified status cookie. Accepts seconds or durations (e.g. `30m`, `12h`, `7d`).
- **`rate_limit` / `rate_window`** (default `240` requests per `60` seconds): Per-site, per-IP fixed-window rate limiter backed by a 64-shard in-memory store (or Redis).
