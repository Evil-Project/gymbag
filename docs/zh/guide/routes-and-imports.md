# 路径路由、静态文件与配置导入

除了基础的 `reverse_proxy`，Gymbag 站点还支持有序的**路径路由（Path Routes）**，用于提供静态文件服务、固定响应、HTTP 重定向、响应头修改以及按路径分流的上游代理。同时，Gymbag 原生支持直接读取和转换现有的 **nginx**、**Apache** 与 **Caddyfile v2** 配置文件。

## 有序路径路由

站点内的路由按声明顺序依次评估。**首个** `path` 匹配的路由将处理该请求；未匹配任何路由的请求会回退到站点的默认 `upstreams`（若未配置默认上游则返回 `404`）。

WAF 规则、限流、威胁情报与 PoW 验证始终在任何路由处理器**之前**执行。

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

### 支持的处理器

每条路由使用以下处理器之一：

- **反向代理 (`upstreams`)**：将匹配的请求（含 WebSocket 升级）代理到路由专属上游，可选剥离路径前缀（`handle_path` / `strip_prefix`）。
- **静态文件服务 (`root` / `file_server`)**：从 `root` 目录提供静态文件，支持可配置的 `index` 文件（默认 `["index.html"]`）、目录浏览（`browse`）及 SPA `fallback`（如 `try_files {path} /index.html`）。
- **固定响应 (`respond`)**：直接由 Gymbag 返回指定状态码与响应体，无需请求上游。
- **重定向 (`redir`)**：返回 HTTP 重定向（`301`、`302`、`307`、`308`），支持请求占位符：`{scheme}`、`{host}`、`{uri}`、`{path}` 与 `{query}`。
- **响应头修改 (`headers` / `header`)**：设置响应头（`Header-Name value`），或通过在键名前加 `-` 来删除响应头（`-Server`）。

### 路径匹配器

- `""`、`*` 或 `/*`：匹配所有请求路径。
- `/exact`：精确路径匹配。
- `/api/*`、`*.php`：通配符匹配（`*` 匹配包括 `/` 在内的任意字符序列）。
- `~^/v[0-9]+/`：开头的 `~` 之后为 Go RE2 正则表达式。

### 在 `gymbag.json` 与 CLI 中管理路由

在 `gymbag.json` 中，路由配置在每个站点的 `routes` 数组中：

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

可通过 CLI 或 `PATCH /api/v1/sites/{domain}` 实时管理路由：

```bash
./gymbag config route list -d app.example.com
./gymbag config route add -d app.example.com --path /static/* --strip-prefix /static --root ./dist --browse
./gymbag config route update -d app.example.com --index 0 --fallback /index.html
./gymbag config route remove -d app.example.com --index 0
```

## 导入 nginx、Apache 与 Caddyfile v2

Gymbag 原生支持解析以下格式：
- **Caddyfile v2**：代码片段 `(name) { ... }`、`import`（支持文件通配符与 `{args[n]}`）、命名匹配器 `@matchers`（`path`、`path_regexp`、`method`）、`handle`、`handle_path`、`route`、`root`、`file_server`、`try_files`、`respond`、`redir` 与 `header`。
- **nginx (`nginx.conf`)**：`http` / `server` / `location` 块、`upstream` 池、`proxy_pass`、`root` / `alias` / `index` / `autoindex` / `try_files`、`return`、`rewrite ... permanent|redirect`、`add_header`、`ssl_certificate` / `ssl_certificate_key` 与 `include`。
- **Apache (`httpd.conf` / `apache2.conf`)**：`<VirtualHost>`、`ServerName` / `ServerAlias`、`ProxyPass` / `<Proxy balancer://...>`、`DocumentRoot` / `DirectoryIndex`、`Redirect` / `RedirectMatch`、`Header set|unset`、`SSLCertificateFile` / `SSLCertificateKeyFile` 与 `Include` / `IncludeOptional`。

可直接运行导入的配置，或将其转换为 `gymbag.json` / `Gymbagfile`：

```bash
# 直接使用 nginx.conf 或 httpd.conf 启动服务
./gymbag run -c /etc/nginx/nginx.conf

# 转换为 Gymbag 原生 gymbag.json 格式
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```
