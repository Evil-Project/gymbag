# Path Routes, Static Files & Config Imports

Besides plain `reverse_proxy`, a Gymbag site can contain ordered **path routes** for static file serving, fixed responses, HTTP redirects, response header mutations, and path-specific upstreams. Gymbag also natively reads and converts existing **nginx**, **Apache**, and **Caddyfile v2** configurations.

## Ordered Path Routes

Routes inside a site are evaluated in order. The **first** route whose `path` matches handles the request; requests that match no route fall through to the site's default `upstreams` (or return `404` when no default upstream is configured).

WAF rules, rate limits, threat intelligence, and PoW verification always run **before** any route handler.

```caddyfile
app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    handle_path /static/* {
        root ./dist
        file_server browse
    }

    @health path /healthz /readyz
    respond @health "ok" 200

    redir /old/* https://{host}/new{uri} 301

    handle /api/* {
        reverse_proxy http://localhost:8080
        header X-Served-By gymbag
    }

    try_files {path} /index.html
}
```

### Supported Handlers

Each route uses exactly one handler:

- **Reverse Proxy (`upstreams`)**: Proxies matched requests (and WebSocket upgrades) to route-specific upstreams, optionally stripping a prefix (`handle_path` / `strip_prefix`).
- **Static File Server (`root` / `file_server`)**: Serves files from `root` with configurable `index` files (default `["index.html"]`), directory listings (`browse`), and SPA `fallback` (e.g. `try_files {path} /index.html`).
- **Static Response (`respond`)**: Writes an immediate status code and body directly from Gymbag without hitting an upstream.
- **Redirect (`redir`)**: Returns an HTTP redirect (`301`, `302`, `307`, `308`) with request placeholders: `{scheme}`, `{host}`, `{uri}`, `{path}`, and `{query}`.
- **Response Headers (`headers` / `header`)**: Sets headers (`Header-Name value`) or deletes them by prefixing the key with `-` (`-Server`).

### Path Matchers

- `""`, `*`, or `/*`: Matches every request path.
- `/exact`: Exact path match.
- `/api/*`, `*.php`: Glob wildcard (`*` matches any character sequence including `/`).
- `~^/v[0-9]+/`: Go RE2 regular expression after the leading `~`.

### Managing Routes in `gymbag.json` & CLI

In `gymbag.json`, routes live in each site's `routes` array:

```json
{
  "routes": [
    {"path": "/static/*", "strip_prefix": "/static", "root": "./dist", "browse": true},
    {"path": "/healthz", "respond": {"status": 200, "body": "ok"}},
    {"path": "/old/*", "redirect": {"to": "https://{host}/new{uri}", "code": 301}},
    {"path": "/api/*", "upstreams": ["http://localhost:8080"], "headers": {"X-Served-By": "gymbag"}}
  ]
}
```

Manage routes from the CLI or live via `PATCH /api/v1/sites/{domain}`:

```bash
./gymbag config route list -d app.example.com
./gymbag config route add -d app.example.com --path /static/* --strip-prefix /static --root ./dist --browse
./gymbag config route update -d app.example.com --index 0 --fallback /index.html
./gymbag config route remove -d app.example.com --index 0
```

## Importing nginx, Apache & Caddyfile v2

Gymbag natively parses:
- **Caddyfile v2**: Snippets `(name) { ... }`, `import` (with file globs and `{args[n]}`), named `@matchers` (`path`, `path_regexp`, `method`), `handle`, `handle_path`, `route`, `root`, `file_server`, `try_files`, `respond`, `redir`, and `header`.
- **nginx (`nginx.conf`)**: `http` / `server` / `location` blocks, `upstream` pools, `proxy_pass`, `root` / `alias` / `index` / `autoindex` / `try_files`, `return`, `rewrite ... permanent|redirect`, `add_header`, `ssl_certificate` / `ssl_certificate_key`, and `include`.
- **Apache (`httpd.conf` / `apache2.conf`)**: `<VirtualHost>`, `ServerName` / `ServerAlias`, `ProxyPass` / `<Proxy balancer://...>`, `DocumentRoot` / `DirectoryIndex`, `Redirect` / `RedirectMatch`, `Header set|unset`, `SSLCertificateFile` / `SSLCertificateKeyFile`, and `Include` / `IncludeOptional`.

Run an imported config directly or convert it to `gymbag.json` / `Gymbagfile`:

```bash
# Serve directly from nginx.conf or httpd.conf
./gymbag run -c /etc/nginx/nginx.conf

# Convert into Gymbag's native gymbag.json format
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```
