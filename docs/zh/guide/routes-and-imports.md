# 路径路由、静态文件与配置导入

除了 `reverse_proxy`，Gymbag 站点内还可以声明有序的**路径路由（path routes）**，用于静态文件托管、固定响应、HTTP 重定向、响应头修改以及按路径分流到不同上游。Gymbag 也支持直接读取和转换现有的 **nginx**、**Apache** 与 **Caddyfile v2** 配置文件。

## 有序路径路由

同一站点内的路由按从上到下的顺序检查。第一条匹配 `path` 的路由负责处理请求；未命中任何路由的请求回落到站点默认 `upstreams`（如果未配置默认上游则返回 `404`）。

WAF 规则、限流、威胁情报和 PoW 验证始终在任何路由处理器**之前**执行。

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

### 路由处理器

每条路由使用一种处理器：

- 反向代理（`upstreams`）：将匹配的 HTTP 请求与 WebSocket 升级转发到路由专属上游，可选剥离路径前缀（`handle_path` / `strip_prefix`）。
- 静态文件服务（`root` / `file_server`）：基于 `root` 目录提供静态资源，支持 `index` 索引文件（默认 `["index.html"]`）、目录浏览（`browse`）以及 SPA `fallback`（`try_files {path} /index.html`）。
- 固定响应（`respond`）：直接返回指定状态码和响应体，不请求上游。
- 重定向（`redir`）：返回 3xx 重定向（`301`、`302`、`307`、`308`），支持展开 `{scheme}`、`{host}`、`{uri}`、`{path}` 和 `{query}` 占位符。
- 响应头（`headers` / `header`）：设置响应头（`Header-Name value`），或用 `-` 前缀删除响应头（`-Server`）。

### 路径匹配器

- `""`、`*` 或 `/*`：匹配所有路径。
- `/exact`：精确路径匹配。
- `/api/*`、`*.php`：通配符匹配（`*` 匹配包括 `/` 在内的任意字符）。
- `~^/v[0-9]+/`：`~` 后接 Go RE2 正则表达式。

### `gymbag.json` 与 CLI 中的路由配置

在 `gymbag.json` 中，路由位于站点对象的 `routes` 数组中：

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

可通过 CLI 或 `PATCH /api/v1/sites/{domain}` 修改路由：

```bash
./gymbag config route list -d app.example.com
./gymbag config route add -d app.example.com --path /static/* --strip-prefix /static --root ./dist --browse
./gymbag config route update -d app.example.com --index 0 --fallback /index.html
./gymbag config route remove -d app.example.com --index 0
```

## 导入 nginx、Apache 与 Caddyfile v2

Gymbag 可直接识别以下配置格式：
- Caddyfile v2：片段 `(name) { ... }`、`import`（文件通配与 `{args[n]}`）、命名 `@matchers`（`path`、`path_regexp`、`method`）、`handle`、`handle_path`、`route`、`root`、`file_server`、`try_files`、`respond`、`redir` 与 `header`。
- nginx（`nginx.conf`）：`http` / `server` / `location` 块、`upstream` 节点池、`proxy_pass`、`root` / `alias` / `index` / `autoindex` / `try_files`、`return`、`rewrite ... permanent|redirect`、`add_header`、`ssl_certificate` / `ssl_certificate_key` 以及 `include`。
- Apache（`httpd.conf` / `apache2.conf`）：`<VirtualHost>`、`ServerName` / `ServerAlias`、`ProxyPass` / `<Proxy balancer://...>`、`DocumentRoot` / `DirectoryIndex`、`Redirect` / `RedirectMatch`、`Header set|unset`、`SSLCertificateFile` / `SSLCertificateKeyFile` 以及 `Include` / `IncludeOptional`。

可以直接用原有配置启动，或转换成 `gymbag.json` / `Gymbagfile`：

```bash
./gymbag run -c /etc/nginx/nginx.conf
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```
