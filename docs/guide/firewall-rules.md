# Firewall Rules & Threat Intelligence

## Request Evaluation Pipeline

For every incoming request on a protected site, Gymbag evaluates security layers in strict order:

1. **Request shape sanity** — HTTP verb check, max URI length, null-byte and `..` / `%2e%2e` path traversal rejection.
2. **ACME HTTP-01 bypass** — `/.well-known/acme-challenge/*` always passes so certificate issuance never breaks.
3. **Custom rules** — Global rules first, then site-specific rules, top-to-bottom (**first match wins**).
4. **Built-in baseline rules** — Sensitive files, CMS exploit probes, scanners, SQLi/XSS query patterns, and automation verification (unless `default_rules off`).
5. **Per-site, per-IP rate limiter** — Configurable `rate_limit` and `rate_window`.
6. **IP threat intelligence** — `ip_security_action verify` (default) or `block`.
7. **Proof-of-Work verification** — Configured `verify` level (`0`–`3`).

Because custom rules run **before** baseline rules, rate limiting, and threat feeds, placing an `allow` rule at the top cleanly whitelists trusted office CIDRs, internal health checks, or preview cookies.

---

## One-Liner Shorthand Rules

Syntax: `<allow | verify | block> <field> <pattern...>` (aliases `bypass`/`pass`, `challenge`/`pow`, and `deny`/`drop`/`reject` also work).

| Field | Matching Semantics | Example |
| :--- | :--- | :--- |
| `ip` | IPv4, IPv6, or CIDR ranges | `allow ip 127.0.0.1 10.0.0.0/8 2001:db8::/32` |
| `user_agent` | Case-insensitive substring, `*` glob, or `~regex` | `block user_agent BadBot ~^(?i)python-urllib/` |
| `path` | Exact/substring, `*` glob, or `~regex` | `verify path /api/export* ~^/v[0-9]+/search` |
| `query` | Raw query string match, `*` glob, or `~regex` | `block query "union select" ~(?i)<script` |
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

## Built-In Rules (`default_rules on|off`)

`default_rules` is `on` by default:
- With **no** custom rules on a site, Gymbag enforces both built-in block rules (`/.env`, `/.git/*`, `/wp-admin/*`, `sqlmap`/`nuclei`, SQLi/XSS queries) and automation verify rules (`curl`, `wget`, `python-requests`, `headlesschrome`, `playwright`, empty `User-Agent`).
- When you **add** custom rules, your rules evaluate first and built-in **block** presets remain appended afterward so adding one `verify path` rule doesn't accidentally expose `/.env`.
- Set `default_rules off` (site-wide or globally) for full manual control.

---

## Auto-Updating Threat & AI Crawler Feeds

All feeds refresh automatically on startup and every `intel_refresh` seconds (default `21600` = 6 hours), cached under `<cert_dir>/intel-cache` with conditional `ETag` / `If-Modified-Since` requests:

- **Threat feeds (`ip_security on`)**: IPsum level 3+, Spamhaus DROP IPv4 & IPv6, abuse.ch Feodo Tracker.
- **Live IP reputation (`ip_reputation on`)**: Keyless lookups against Shodan InternetDB and StopForumSpam with in-memory TTL caching and fail-open timeouts.
- **Official AI crawler IP ranges (`ai_ip_list on`)**: OpenAI (`gptbot`, `searchbot`, `chatgpt-user`), Anthropic (`claude.com/crawling/bots.json`), Perplexity, Google, and Apple (`applebot`).

```caddyfile
{
    ip_security on
    ip_security_action block   # "verify" (PoW challenge) or "block" (403 Forbidden)
    ip_reputation on
    ai_ip_list on
    intel_refresh 21600
}
```
