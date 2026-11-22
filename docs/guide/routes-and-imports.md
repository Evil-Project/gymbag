# Path Routes, Static Files & Config Imports

Next to `reverse_proxy`, a Gymbag site can declare ordered **path routes** for static files, fixed responses, HTTP redirects, response header edits, and path-specific upstreams. Gymbag also reads and converts existing **nginx**, **Apache**, and **Caddyfile v2** files directly.

## Ordered Path Routes

Routes inside a site are checked top to bottom. The first matching `path` handles the request; anything unmatched falls through to the site's default `upstreams` (or returns `404` when none are set).

WAF rules, rate limits, threat intel, and PoW challenges always run **before** any route handler.

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

### Route Handlers

Each route uses one handler:

- **Reverse Proxy (`upstreams`)**: Forwards matched HTTP requests and WebSocket upgrades to route-specific upstreams, optionally stripping a prefix (`handle_path` / `strip_prefix`).
- **Static File Server (`root` / `file_server`)**: Serves files from `root` with `index` files (default `["index.html"]`), directory listings (`browse`), and SPA `fallback` (`try_files {path} /index.html`).
- **Static Response (`respond`)**: Writes an immediate status code and body without dialing an upstream.
- **Redirect (`redir`)**: Sends a 3xx redirect (`301`, `302`, `307`, `308`) expanding `{scheme}`, `{host}`, `{uri}`, `{path}`, and `{query}`.
- **Response Headers (`headers` / `header`)**: Sets headers (`Header-Name value`) or removes them with a `-` prefix (`-Server`).

### Path Matchers

- `""`, `*`, or `/*`: Matches every path.
- `/exact`: Exact path match.
- `/api/*`, `*.php`: Glob wildcard (`*` matches any characters, including `/`).
- `~^/v[0-9]+/`: Go RE2 regex after `~`.

### Routes in `gymbag.json` & CLI

In `gymbag.json`, routes sit in a site's `routes` array:

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

Edit routes from the CLI or via `PATCH /api/v1/sites/{domain}`:

```bash
./gymbag config route list -d app.example.com
./gymbag config route add -d app.example.com --path /static/* --strip-prefix /static --root ./dist --browse
./gymbag config route update -d app.example.com --index 0 --fallback /index.html
./gymbag config route remove -d app.example.com --index 0
```

## Importing nginx, Apache & Caddyfile v2

Gymbag reads these formats directly:
- **Caddyfile v2**: Snippets `(name) { ... }`, `import` (file globs and `{args[n]}`), named `@matchers` (`path`, `path_regexp`, `method`), `handle`, `handle_path`, `route`, `root`, `file_server`, `try_files`, `respond`, `redir`, and `header`.
- **nginx (`nginx.conf`)**: `http` / `server` / `location` blocks, `upstream` pools, `proxy_pass`, `root` / `alias` / `index` / `autoindex` / `try_files`, `return`, `rewrite ... permanent|redirect`, `add_header`, `ssl_certificate` / `ssl_certificate_key`, and `include`.
- **Apache (`httpd.conf` / `apache2.conf`)**: `<VirtualHost>`, `ServerName` / `ServerAlias`, `ProxyPass` / `<Proxy balancer://...>`, `DocumentRoot` / `DirectoryIndex`, `Redirect` / `RedirectMatch`, `Header set|unset`, `SSLCertificateFile` / `SSLCertificateKeyFile`, and `Include` / `IncludeOptional`.

Serve an imported config straight away or convert it into `gymbag.json` / `Gymbagfile`:

```bash
./gymbag run -c /etc/nginx/nginx.conf
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```
