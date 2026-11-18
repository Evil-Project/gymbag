# Verify Levels (`0`–`3`) & Proof-of-Work

A single Gymbag process can serve and protect dozens of domains simultaneously with round-robin multi-upstream load balancing and full-duplex RFC 6455 WebSocket proxying. Each site configures its own verification policy via `verify <0-3>` or `protect off`.

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

## Difficulty & Clearance Cookies

- **`difficulty`** (`1`–`5`, default `4`): Number of leading hex zeros required in the SHA-256 hash (`0000...` at difficulty `4`).
- **`challenge_ttl`** (default `300` seconds): Lifetime of a single-use PoW challenge nonce.
- **`clearance_ttl`** (default `86400` seconds = 24 hours): Lifetime of the `HttpOnly`, `SameSite=Lax` HMAC-signed `waf_clearance` cookie, bound to both host and client IP.
- **`rate_limit` / `rate_window`** (default `240` requests per `60` seconds): Per-site, per-IP fixed-window rate limiter.
