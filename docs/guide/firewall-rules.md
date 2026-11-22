# Firewall Rules, Remote Rules & Threat Intelligence

## Request Evaluation Pipeline

On protected HTTP sites, requests walk through these layers in order:

1. **Request shape sanity**: HTTP method validation, URI length limits, null-byte and `..` / `%2e%2e` traversal rejection.
2. **ACME HTTP-01 bypass**: `/.well-known/acme-challenge/*` always passes so certificate issuance never breaks.
3. **Local custom rules**: Global rules first, then site rules, top to bottom (first match wins).
4. **Remote rules (`remote_rules`)**: Validated rules fetched from HTTP/HTTPS URLs, evaluated in order.
5. **Built-in baseline rules**: Sensitive paths, CMS probes, scanners, SQLi/XSS queries, and automation PoW rules (unless `default_rules off`).
6. **Per-site, per-IP rate limiter**: `rate_limit` and `rate_window` (default `240` / `60s`).
7. **IP threat intelligence**: `ip_security_action verify` (default) or `block`.
8. **Proof-of-Work verification**: Configured `verify` level (`0`–`3`), including VPN/proxy/Tor detection (`1` & `3`) and AI crawler matching (`2` & `3`).

Because local custom rules run before remote rules, baseline rules, rate limits, and threat feeds, putting an `allow` rule at the top of a site block whitelists an office CIDR, health check, or preview cookie without extra workarounds. Global rules inside `{ ... }` also run on `ssh` and `minecraft` services (alongside per-service rules inside `ssh { ... }` and `minecraft { ... }`).

## One-Liner Shorthand Rules

Syntax: `<allow | verify | block> <field> <pattern...>` (aliases `bypass`/`pass`, `challenge`/`pow`, and `deny`/`drop`/`reject` also work).

| Field | Matching Semantics | Example |
| :--- | :--- | :--- |
| `ip` | IPv4, IPv6, or CIDR ranges | `allow ip 127.0.0.1 10.0.0.0/8 2001:db8::/32` |
| `user_agent` | Case-insensitive substring, `*` glob, or `~regex` | `block user_agent BadBot ~^(?i)python-urllib/` |
| `path` | Exact/prefix, `*` glob, or `~regex` | `verify path /api/export* ~^/v[0-9]+/search` |
| `query` | Raw/decoded query match, `*` glob, or `~regex` | `block query "union select" ~(?i)<script` |
| `method` | HTTP verbs (case-insensitive) | `verify method POST DELETE` |
| `header` | `<Header-Name> [pattern]` (`""` = missing/empty, `*` = present) | `verify header Accept-Language ""` |
| `cookie` | `<cookie_name> [pattern]` (`""` = missing/empty, `*` = present) | `allow cookie vip_session *` |

## Multi-Condition `rule` Blocks

Use `rule "name" { ... }` when conditions need to match together. Multiple values on the same field are OR'd; different fields inside one block are AND'd:

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

## Remote Rules (`remote_rules`)

You can pull shared rulesets from one or more HTTP/HTTPS URLs (a Git raw file, object storage bucket, CDN, or policy server). Gymbag fetches them on startup and every `remote_rules_refresh` (default `5m` / `300s`).

::: code-group
```caddyfile [Gymbagfile]
{
    remote_rules https://rules.example.com/org-baseline.rules
    remote_rules_refresh 5m
}

app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    allow ip 10.0.0.0/8
    remote_rules https://rules.example.com/app-overrides.json
}
```

```bash [CLI & Environment]
gymbag run --config Gymbagfile \
  --remote-rules https://rules.example.com/baseline.rules \
  --remote-rules-refresh 10m

GYMBAG_REMOTE_RULES="https://rules.example.com/baseline.rules,https://rules.example.com/extra.json" \
GYMBAG_REMOTE_RULES_REFRESH=5m \
gymbag
```

```bash [Admin REST API]
curl http://127.0.0.1:2019/api/v1/rules/remote
curl -X POST http://127.0.0.1:2019/api/v1/rules/remote/refresh

curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"remote_rules": ["https://rules.example.com/emergency.rules"]}'
```
:::

### Supported Remote Rule Formats

Gymbag accepts both **Gymbagfile rule syntax** and **JSON** (`[ ... ]` or `{"rules": [ ... ]}`):

::: code-group
```caddyfile [Plain-Text Gymbagfile Syntax]
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

```json [JSON Object]
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

### Caching & Validation

- Sends `If-None-Match` (`ETag`) and `If-Modified-Since`; `304 Not Modified` responses skip recompiling the ruleset.
- Caches validated payloads under `<cert_dir>/rules-cache/` and warm-loads them on startup before the first fetch finishes.
- Every payload is validated (4 MiB limit, 15s timeout) before swapping. If an upstream returns invalid syntax or an HTTP error, the previous ruleset stays active and the error shows up in `GET /api/v1/rules/remote`.
- Rule updates swap atomically without dropping active connections.

## Local Rule Files, Folders & Hot-Reload

Keep rules in standalone `.rules` / `.json` files or directories. Mark a source `dynamic` to watch disk changes and reload automatically:

::: code-group
```caddyfile [Gymbagfile]
{
    rules_watch_interval 2s
}

app.example.com {
    reverse_proxy http://localhost:3000
    rules_file ./rules/custom.rules dynamic
    rules_dir ./rules.d dynamic
    ruleset static-corp {
        source ./rules/corp.json
        dynamic off
    }
}
```

```json [gymbag.json]
{
  "sites": [
    {
      "domain": "app.example.com",
      "upstreams": ["http://localhost:3000"],
      "waf": {
        "rule_sources": [
          {"source": "./rules.d", "dynamic": true, "name": "shared"}
        ]
      }
    }
  ]
}
```

```bash [CLI]
gymbag init -d app.example.com -u :3000 \
  --rules-file ./rules/custom.rules --rules-dir ./rules.d \
  --rules-dynamic --rules-watch-interval 2s

gymbag ruleset init ./rules/custom.rules
gymbag ruleset add --source ./rules --dynamic --name local-dir
gymbag ruleset update local-dir --static
gymbag ruleset validate ./rules
gymbag ruleset rule add --file ./rules/custom.rules --action block --name bad-bot --ua "*BadBot*"
gymbag ruleset remove local-dir
```
:::

Polling uses `rules_watch_interval` (default `2s`). `rule_sources` can also be patched via `PATCH /api/v1/sites/{domain}`. See [CLI & Environment](/reference/cli-env) for the full `ruleset` command list.

## Built-In Rules (`default_rules on|off`)

`default_rules` is `on` by default:
- When a site has **no** local or remote custom rules, Gymbag runs both built-in block rules (`/.env`, `/.git/*`, `/wp-admin/*`, `sqlmap`/`nuclei`, SQLi/XSS queries) and automation PoW rules (`curl`, `wget`, `python-requests`, `headlesschrome`, `playwright`, empty `User-Agent`).
- Once you add custom rules to a site, your rules run first and built-in **block** rules stay appended afterward so adding one `verify path` rule doesn't expose `/.env`.
- Set `default_rules off` on a site or globally to disable built-in rules completely.

## Threat Feeds, VPN/Tor Detection & AI Crawler Lists

Feeds refresh on startup and every `intel_refresh` (default `21600`s / `6h`), cached under `<cert_dir>/intel-cache`:

- **Threat feeds (`ip_security on`)**: IPsum level 3+, Spamhaus DROP IPv4 & IPv6, abuse.ch Feodo Tracker.
- **Live IP reputation (`ip_reputation on`)**: Keyless lookups via Shodan InternetDB and StopForumSpam with 1-hour TTL caching and fail-open timeouts.
- **VPN / proxy / Tor detection (`vpn_detection on`)**: Auto-updating X4BNet VPN IPv4/IPv6 lists and Tor bulk exit list, plus keyless live checks against `api.ipquery.io` and `api.ipapi.is` (1.2s timeout, singleflight deduplication, 1-hour cache, fail-open). Flagged IPs count as **suspicious** at verify levels `1` and `3` for HTTP, SSH, and Minecraft.
- **Official AI crawler CIDRs (`ai_ip_list on`)**: OpenAI (`gptbot`, `searchbot`, `chatgpt-user`), Anthropic (`claude.com/crawling/bots.json`), Perplexity, Google, and Apple (`applebot`).

```caddyfile
{
    ip_security on
    ip_security_action block   # "verify" (PoW challenge) or "block" (403 Forbidden)
    ip_reputation on
    vpn_detection on
    ai_ip_list on
    intel_refresh 6h
    # vpn_feed https://rules.example.com/extra-vpn-cidrs.txt
}
```
