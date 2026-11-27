# 路徑路由、靜態檔案與設定檔匯入

除了 `reverse_proxy` 之外，Gymbag 的站台設定也可以宣告依序比對的**路徑路由（path routes）**，用來提供靜態檔案、固定回應、HTTP 重新導向、修改回應標頭，以及針對特定路徑指派不同的上游伺服器。此外，Gymbag 也能直接讀取並轉換現有的 **nginx**、**Apache** 與 **Caddyfile v2** 設定檔。

## 依序比對的路徑路由

站台內的路由會由上而下依序檢查。第一個符合 `path` 條件的路由會處理該請求；若都不符合，則交由站台預設的 `upstreams` 處理（若未設定預設上游則回傳 `404`）。

WAF 規則、速率限制、威脅情資與工作量證明挑戰一律會在任何路由處理器**之前**執行。

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

### 路由處理器類型

每個路由皆會使用以下其中一種處理器：

- **反向代理（`upstreams`）**：將符合條件的 HTTP 請求與 WebSocket 升級連線轉發至該路由專屬的上游伺服器，並可選擇先剝除路徑前綴（`handle_path` / `strip_prefix`）。
- **靜態檔案伺服器（`root` / `file_server`）**：從 `root` 目錄提供檔案服務，支援 `index` 預設索引檔（預設 `["index.html"]`）、目錄列表（`browse`）以及 SPA `fallback`（`try_files {path} /index.html`）。
- **靜態回應（`respond`）**：不連線至上游伺服器，直接回傳指定的狀態碼與內容。
- **重新導向（`redir`）**：回傳 3xx 重新導向（`301`、`302`、`307`、`308`），支援展開 `{scheme}`、`{host}`、`{uri}`、`{path}` 與 `{query}` 變數。
- **回應標頭（`headers` / `header`）**：設定標頭（`Header-Name value`），或加上 `-` 前綴移除標頭（`-Server`）。

### 路徑比對語法

- `""`、`*` 或 `/*`：比對所有路徑。
- `/exact`：精確路徑比對。
- `/api/*`、`*.php`：萬用字元比對（`*` 可比對包含 `/` 在內的任意字元）。
- `~^/v[0-9]+/`：`~` 之後接 Go RE2 正規表達式。

### `gymbag.json` 與 CLI 中的路由設定

在 `gymbag.json` 中，路由位於站台物件的 `routes` 陣列內：

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

可透過 CLI 或 `PATCH /api/v1/sites/{domain}` 修改路由：

```bash
./gymbag config route list -d app.example.com
./gymbag config route add -d app.example.com --path /static/* --strip-prefix /static --root ./dist --browse
./gymbag config route update -d app.example.com --index 0 --fallback /index.html
./gymbag config route remove -d app.example.com --index 0
```

## Authelia 與轉發驗證（Forward Authentication）

Gymbag 原生支援 **Authelia** 整合、Caddy v2 的 `forward_auth` 區塊，以及 nginx 的 `auth_request` / `auth_request_set` 子請求，並可設定於全域 `{ ... }`、站台層級或 `handle` 路由層級：

```caddyfile
{
    authelia http://127.0.0.1:9091 {
        portal_url https://auth.example.com
    }
}

auth.example.com {
    reverse_proxy http://127.0.0.1:9091
    verify 1
}

app.example.com {
    reverse_proxy http://127.0.0.1:3000
    authelia http://127.0.0.1:9091 {
        portal_url https://auth.example.com
        copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
        except /healthz /public/*
    }
}
```

- **執行順序**：系統會先處理 `/.well-known/acme-challenge/*` 與 `/.waf/*`，接著執行 WAF 規則 / 工作量證明挑戰，只有通過驗證的請求（包含 HTTP 與 WebSocket 升級請求）才會在抵達上游前觸發轉發驗證。
- **防偽造保護**：所有設定的 `copy_headers` 目標標頭（包含 `Src>Dst` 重新對應）都會在發出驗證子請求前、以及注入 Authelia 回傳的已驗證標頭前，先從用戶端傳入的請求中清除。
- **重新導向與 fail-closed 行為**：若 Authelia 回傳 `401` 且未附帶 `Location` 標頭，同時有設定 `portal_url`，則 HTML 瀏覽器的 `GET`/`HEAD` 請求會收到 `302 Found` 並導向至 `<portal_url>?rd=<original_url>`；API/JSON 與 WebSocket 用戶端則直接收到 `401` 狀態碼。若驗證伺服器無法連線，系統會採 fail-closed 原則回傳 `502 Bad Gateway`。

## 匯入 nginx、Apache 與 Caddyfile v2 設定檔

Gymbag 可直接讀取以下格式：
- **Caddyfile v2**：Snippet `(name) { ... }`、`import`（檔案萬用字元與 `{args[n]}`）、具名 `@matchers`（`path`、`path_regexp`、`method`）、`handle`、`handle_path`、`route`、`forward_auth`、`authelia`、`root`、`file_server`、`try_files`、`respond`、`redir` 與 `header`。
- **nginx（`nginx.conf`）**：`http` / `server` / `location` 區塊、`upstream` 叢集、`proxy_pass`、`internal`、`auth_request`、`auth_request_set`（`$upstream_http_*`）、`root` / `alias` / `index` / `autoindex` / `try_files`、`return`、`rewrite ... permanent|redirect`、`add_header`、`ssl_certificate` / `ssl_certificate_key` 與 `include`。
- **Apache（`httpd.conf` / `apache2.conf`）**：`<VirtualHost>`、`ServerName` / `ServerAlias`、`ProxyPass` / `<Proxy balancer://...>`、`DocumentRoot` / `DirectoryIndex`、`Redirect` / `RedirectMatch`、`Header set|unset`、`SSLCertificateFile` / `SSLCertificateKeyFile` 與 `Include` / `IncludeOptional`。

你可以直接以匯入的設定檔啟動，或將其轉換為 `gymbag.json` / `Gymbagfile`：

```bash
./gymbag run -c /etc/nginx/nginx.conf
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```
