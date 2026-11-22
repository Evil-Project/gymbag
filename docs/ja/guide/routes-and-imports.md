# パスルート・静的ファイル・設定インポート

Gymbag のサイトでは `reverse_proxy` に並べて、静的ファイル配信、固定レスポンス、HTTP リダイレクト、レスポンスヘッダー編集、パス別アップストリームを行う順序付きの**パスルート**を定義できます。また、既存の **nginx**、**Apache**、**Caddyfile v2** 設定ファイルを直接読み込んで変換することもできます。

## 順序付きパスルート

サイト内のルートは上から順に評価され、最初に一致した `path` がリクエストを処理します。どのルートにも一致しなかった場合はサイトのデフォルト `upstreams` へフォールバックします（未設定の場合は `404`）。

WAF ルール、レート制限、脅威インテル、PoW チャレンジは常にどのルートハンドラよりも**前**に実行されます。

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

### ルートハンドラ

各ルートには次のうち 1 つのハンドラを指定します。

- **リバースプロキシ (`upstreams`)**: 一致した HTTP リクエストと WebSocket アップグレードをルート固有のアップストリームへ転送します。必要に応じてプレフィックス除去（`handle_path` / `strip_prefix`）も行えます。
- **静的ファイルサーバー (`root` / `file_server`)**: `root` からファイルを配信します。`index` ファイル（デフォルト `["index.html"]`）、ディレクトリ一覧（`browse`）、SPA 用 `fallback`（`try_files {path} /index.html`）に対応しています。
- **固定レスポンス (`respond`)**: アップストリームに接続せず、指定したステータスコードとボディを即座に返します。
- **リダイレクト (`redir`)**: 3xx リダイレクト（`301`、`302`、`307`、`308`）を返します。`{scheme}`、`{host}`、`{uri}`、`{path}`、`{query}` の展開に対応しています。
- **レスポンスヘッダー (`headers` / `header`)**: ヘッダーを設定（`Header-Name value`）するか、先頭に `-` を付けて削除（`-Server`）します。

### パスマッチャー

- `""`、`*`、`/*`: すべてのパスに一致。
- `/exact`: 完全一致。
- `/api/*`、`*.php`: グロブワイルドカード（`*` は `/` を含む任意の文字列に一致）。
- `~^/v[0-9]+/`: `~` に続けて Go RE2 正規表現を指定。

### `gymbag.json` と CLI でのルート管理

`gymbag.json` ではサイトの `routes` 配列に記述します。

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

ルートの編集は CLI または `PATCH /api/v1/sites/{domain}` で行えます。

```bash
./gymbag config route list -d app.example.com
./gymbag config route add -d app.example.com --path /static/* --strip-prefix /static --root ./dist --browse
./gymbag config route update -d app.example.com --index 0 --fallback /index.html
./gymbag config route remove -d app.example.com --index 0
```

## nginx・Apache・Caddyfile v2 のインポート

Gymbag は次の形式を直接読み込めます。
- **Caddyfile v2**: スニペット `(name) { ... }`、`import`（ファイルグロブと `{args[n]}`）、名前付き `@matchers`（`path`、`path_regexp`、`method`）、`handle`、`handle_path`、`route`、`root`、`file_server`、`try_files`、`respond`、`redir`、`header`。
- **nginx (`nginx.conf`)**: `http` / `server` / `location` ブロック、`upstream` プール、`proxy_pass`、`root` / `alias` / `index` / `autoindex` / `try_files`、`return`、`rewrite ... permanent|redirect`、`add_header`、`ssl_certificate` / `ssl_certificate_key`、`include`。
- **Apache (`httpd.conf` / `apache2.conf`)**: `<VirtualHost>`、`ServerName` / `ServerAlias`、`ProxyPass` / `<Proxy balancer://...>`、`DocumentRoot` / `DirectoryIndex`、`Redirect` / `RedirectMatch`、`Header set|unset`、`SSLCertificateFile` / `SSLCertificateKeyFile`、`Include` / `IncludeOptional`。

読み込んだ設定でそのまま起動することも、`gymbag.json` / `Gymbagfile` へ変換することもできます。

```bash
./gymbag run -c /etc/nginx/nginx.conf
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```
