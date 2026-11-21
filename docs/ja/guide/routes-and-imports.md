# パスルート・静的ファイル・設定インポート

通常の `reverse_proxy` に加え、Gymbag のサイトでは静的ファイル配信、固定レスポンス、HTTP リダイレクト、レスポンスヘッダー変更、パス別のアップストリームを定義できる順序付き**パスルート**を利用できます。また、既存の **nginx**、**Apache**、**Caddyfile v2** 設定をネイティブに読み込み・変換することも可能です。

## 順序付きパスルート

サイト内のルートは定義順に評価されます。`path` に一致した**最初の**ルートがリクエストを処理し、どのルートにも一致しないリクエストはサイトの既定 `upstreams` にフォールバックします（既定アップストリームが未設定の場合は `404` を返します）。

WAF ルール、レート制限、脅威インテリジェンス、PoW 検証は、常にルートハンドラーの**前**に実行されます。

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

### サポートされるハンドラー

各ルートはちょうど1つのハンドラーを使用します：

- **リバースプロキシ (`upstreams`)**：一致したリクエスト（および WebSocket アップグレード）をルート固有のアップストリームへ転送します。必要に応じてプレフィックスを削除できます（`handle_path` / `strip_prefix`）。
- **静的ファイルサーバー (`root` / `file_server`)**：`root` からファイルを配信します。`index` ファイル（既定 `["index.html"]`）、ディレクトリ一覧（`browse`）、SPA 用 `fallback`（例：`try_files {path} /index.html`）を設定できます。
- **静的レスポンス (`respond`)**：アップストリームへ転送せず、Gymbag から直接ステータスコードとボディを返します。
- **リダイレクト (`redir`)**：HTTP リダイレクト（`301`、`302`、`307`、`308`）を返します。プレースホルダー `{scheme}`、`{host}`、`{uri}`、`{path}`、`{query}` を利用できます。
- **レスポンスヘッダー (`headers` / `header`)**：ヘッダーを設定（`Header-Name value`）するか、先頭に `-` を付けて削除（`-Server`）します。

### パスマッチャー

- `""`、`*`、`/*`：すべてのリクエストパスに一致。
- `/exact`：完全一致。
- `/api/*`、`*.php`：グロブワイルドカード（`*` は `/` を含む任意の文字列に一致）。
- `~^/v[0-9]+/`：先頭の `~` 以降を Go RE2 正規表現として評価。

### `gymbag.json` と CLI でのルート管理

`gymbag.json` では、各サイトの `routes` 配列にルートを定義します：

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

CLI または `PATCH /api/v1/sites/{domain}` でルートを管理できます：

```bash
./gymbag config route list -d app.example.com
./gymbag config route add -d app.example.com --path /static/* --strip-prefix /static --root ./dist --browse
./gymbag config route update -d app.example.com --index 0 --fallback /index.html
./gymbag config route remove -d app.example.com --index 0
```

## nginx・Apache・Caddyfile v2 のインポート

Gymbag は以下をネイティブにパースします：
- **Caddyfile v2**：スニペット `(name) { ... }`、`import`（ファイルグロブおよび `{args[n]}` 対応）、名前付き `@matchers`（`path`、`path_regexp`、`method`）、`handle`、`handle_path`、`route`、`root`、`file_server`、`try_files`、`respond`、`redir`、`header`。
- **nginx (`nginx.conf`)**：`http` / `server` / `location` ブロック、`upstream` プール、`proxy_pass`、`root` / `alias` / `index` / `autoindex` / `try_files`、`return`、`rewrite ... permanent|redirect`、`add_header`、`ssl_certificate` / `ssl_certificate_key`、`include`。
- **Apache (`httpd.conf` / `apache2.conf`)**：`<VirtualHost>`、`ServerName` / `ServerAlias`、`ProxyPass` / `<Proxy balancer://...>`、`DocumentRoot` / `DirectoryIndex`、`Redirect` / `RedirectMatch`、`Header set|unset`、`SSLCertificateFile` / `SSLCertificateKeyFile`、`Include` / `IncludeOptional`。

インポートした設定をそのまま起動するか、`gymbag.json` / `Gymbagfile` へ変換できます：

```bash
# nginx.conf や httpd.conf から直接起動
./gymbag run -c /etc/nginx/nginx.conf

# Gymbag ネイティブの gymbag.json 形式へ変換
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```
