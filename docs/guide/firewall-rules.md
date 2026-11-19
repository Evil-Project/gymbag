# Firewall Rules, Remote Rules & Threat Intelligence

## Request Evaluation Pipeline

For every incoming request on a protected site, Gymbag evaluates security layers in strict order:

1. **Request shape sanity** — HTTP verb check, max URI length, null-byte and `..` / `%2e%2e` path traversal rejection.
2. **ACME HTTP-01 bypass** — `/.well-known/acme-challenge/*` always passes so certificate issuance never breaks.
3. **Local custom rules** — Global rules first, then site-specific rules, top-to-bottom (**first match wins**).
4. **Remote rules (`remote_rules`)** — Validated rules fetched from configured HTTP/HTTPS URLs, evaluated in declaration order.
5. **Built-in baseline rules** — Sensitive files, CMS exploit probes, scanners, SQLi/XSS query patterns, and automation verification (unless `default_rules off`).
6. **Per-site, per-IP rate limiter** — Configurable `rate_limit` and `rate_window`.
7. **IP threat intelligence** — `ip_security_action verify` (default) or `block`.
8. **Proof-of-Work verification** — Configured `verify` level (`0`–`3`).

Because local custom rules run **before** remote rules, baseline rules, rate limiting, and threat feeds, placing a local `allow` rule at the top always cleanly overrides remote lists to whitelist trusted office CIDRs, internal health checks, or preview cookies.

---

## One-Liner Shorthand Rules

Syntax: `<allow | verify | block> <field> <pattern...>` (aliases `bypass`/`pass`, `challenge`/`pow`, and `deny`/`drop`/`reject` also work).

| Field | Matching Semantics | Example |
| :--- | :--- | :--- |
| `ip` | IPv4, IPv6, or CIDR ranges | `allow ip 127.0.0.1 10.0.0.0/8 2001:db8::/32` |
| `user_agent` | Case-insensitive substring, `*` glob, or `~regex` | `block user_agent BadBot ~^(?i)python-urllib/` |
| `path` | Exact/prefix, `*` glob, or `~regex` | `verify path /api/export* ~^/v[0-9]+/search` |
| `query` | Raw/decoded query string match, `*` glob, or `~regex` | `block query "union select" ~(?i)<script` |
| `method` | HTTP verbs (case-insensitive) | `verify method POST DELETE` |
| `header` | `<Header-Name> [pattern]` (`""` = missing/empty, `*` = present) | `verify header Accept-Language ""` |
| `cookie` | `<cookie_name> [pattern]` (`""` = missing/empty, `*` = present) | `allow cookie vip_session *` |

---

## Multi-Condition `rule` Blocks

Use `rule "name" { ... }` when multiple conditions must match together. Multiple values on the **same** field combine with **OR**; different fields inside the same `rule` block combine with **AND**:

```caddyfile
app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    rule "challenge-auth-mutations" {
        action verify
        method POST
        path /login* /api/auth/* /reset-password*
    }

    rule "block-headless-api-posts" {
        action block
        method POST
        path /api/*
        header Origin ""
    }
}
```

---

## Remote Rules (`remote_rules`)

You can pull shared or centrally managed firewall rules from one or more HTTP/HTTPS URLs (such as a Git repository raw URL, S3/R2 bucket, CDN, or internal policy server). Gymbag automatically refreshes remote rules on startup and every `remote_rules_refresh` interval (default `5m` / `300` seconds).

### Configuring Remote Rules

Remote rules can be attached globally (inherited by every site) and/or per site:

::: code-group
```caddyfile [Gymbagfile]
{
    # Shared org-wide rules applied to every site
    remote_rules https://rules.example.com/org-baseline.rules
    remote_rules_refresh 5m
}

app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    # Local rules always evaluate before remote rules
    allow ip 10.0.0.0/8

    # Site-specific remote rules (plain text or JSON)
    remote_rules https://rules.example.com/app-overrides.json
}
```

```bash [CLI & Environment]
# CLI flags (repeatable or comma-separated)
gymbag run --config Gymbagfile \
  --remote-rules https://rules.example.com/baseline.rules \
  --remote-rules-refresh 10m

# Environment variables
GYMBAG_REMOTE_RULES="https://rules.example.com/baseline.rules,https://rules.example.com/extra.json" \
GYMBAG_REMOTE_RULES_REFRESH=5m \
gymbag
```

```bash [Admin REST API]
# Inspect remote rule sync status, rule counts, and any fetch errors
curl http://127.0.0.1:2019/api/v1/rules/remote

# Force an immediate refresh and atomic hot-reload
curl -X POST http://127.0.0.1:2019/api/v1/rules/remote/refresh

# Attach or update remote rule URLs on a single site live
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"remote_rules": ["https://rules.example.com/emergency.rules"]}'
```
:::

### Supported Remote Rule Formats

Gymbag auto-detects both **Gymbagfile rule syntax** and **JSON**:

::: code-group
```caddyfile [Plain-Text Gymbagfile Syntax]
# Shared remote firewall rules
block ip 198.51.100.0/24 203.0.113.42
block user_agent BadScraper ~^(?i)EvilBot/
verify path /api/export*

rule "challenge-xml-posts" {
    action verify
    method POST
    path /xmlrpc* /api/import*
    header Origin ""
}
```

```json [JSON Object (or JSON Array)]
{
  "rules": [
    {
      "name": "block-abusive-cidr",
      "action": "block",
      "ips": ["198.51.100.0/24", "203.0.113.42"]
    },
    {
      "name": "verify-exports",
      "action": "verify",
      "methods": ["GET", "POST"],
      "paths": ["/api/export*"]
    }
  ]
}
```
:::

### Caching & Fail-Safe Guarantees

- **Conditional fetching**: Sends `If-None-Match` (`ETag`) and `If-Modified-Since` headers; `304 Not Modified` responses skip recompilation completely.
- **Disk cache**: Validated payloads are cached under `<cert_dir>/rules-cache/` and warm-loaded immediately on startup before the first network fetch finishes.
- **Fail-safe validation**: Every fetched file is strictly validated before activation (4 MiB cap, 15s timeout). If a remote host returns an HTTP error or malformed rule syntax, Gymbag retains the last known good ruleset and surfaces the error under `GET /api/v1/rules/remote`.
- **Zero-downtime swap**: When rules change, Gymbag recompiles the site's WAF engine (including O(1) path maps and Aho-Corasick DFAs) and atomically swaps the routing snapshot without dropping in-flight connections.

---

## Built-In Rules (`default_rules on|off`)

`default_rules` is `on` by default:
- With **no** local or remote custom rules on a site, Gymbag enforces both built-in block rules (`/.env`, `/.git/*`, `/wp-admin/*`, `sqlmap`/`nuclei`, SQLi/XSS queries) and automation verify rules (`curl`, `wget`, `python-requests`, `headlesschrome`, `playwright`, empty `User-Agent`).
- When you **add** local or remote custom rules, your rules evaluate first and built-in **block** presets remain appended afterward so adding one `verify path` rule doesn't accidentally expose `/.env`.
- Set `default_rules off` (site-wide or globally) for full manual control.

---

## Auto-Updating Threat & AI Crawler Feeds

All feeds refresh automatically on startup and every `intel_refresh` seconds/duration (default `21600`s / `6h`), cached under `<cert_dir>/intel-cache` with conditional `ETag` / `If-Modified-Since` requests:

- **Threat feeds (`ip_security on`)**: IPsum level 3+, Spamhaus DROP IPv4 & IPv6, abuse.ch Feodo Tracker.
- **Live IP reputation (`ip_reputation on`)**: Keyless lookups against Shodan InternetDB and StopForumSpam with in-memory TTL caching and fail-open timeouts.
- **Official AI crawler IP ranges (`ai_ip_list on`)**: OpenAI (`gptbot`, `searchbot`, `chatgpt-user`), Anthropic (`claude.com/crawling/bots.json`), Perplexity, Google, and Apple (`applebot`).

```caddyfile
{
    ip_security on
    ip_security_action block   # "verify" (PoW challenge) or "block" (403 Forbidden)
    ip_reputation on
    ai_ip_list on
    intel_refresh 6h
}
```
