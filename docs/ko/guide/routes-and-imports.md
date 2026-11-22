# 경로 라우트, 정적 파일 & 설정 가져오기

Gymbag 사이트는 `reverse_proxy`와 함께 정적 파일, 고정 응답, HTTP 리다이렉트, 응답 헤더 수정, 경로별 업스트림을 위한 순서 기반 **경로 라우트**를 선언할 수 있습니다. 또한 기존 **nginx**, **Apache**, **Caddyfile v2** 파일을 직접 읽고 변환합니다.

## 순서 기반 경로 라우트

사이트 내부의 라우트는 위에서 아래로 검사합니다. 가장 먼저 일치하는 `path`가 요청을 처리하고, 일치하는 항목이 없으면 사이트의 기본 `upstreams`로 넘어갑니다(설정된 업스트림이 없으면 `404` 반환).

WAF 규칙, 속도 제한, 위협 인텔, PoW 챌린지는 항상 모든 라우트 핸들러보다 **먼저** 실행됩니다.

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

### 라우트 핸들러

각 라우트는 하나의 핸들러를 사용합니다.

- **리버스 프록시 (`upstreams`)**: 일치하는 HTTP 요청과 WebSocket 업그레이드를 라우트 전용 업스트림으로 전달하며, 선택적으로 접두사를 제거합니다(`handle_path` / `strip_prefix`).
- **정적 파일 서버 (`root` / `file_server`)**: `root`에서 파일을 제공하며 `index` 파일(기본 `["index.html"]`), 디렉터리 목록(`browse`), SPA `fallback`(`try_files {path} /index.html`)을 지원합니다.
- **정적 응답 (`respond`)**: 업스트림에 연결하지 않고 상태 코드와 본문을 바로 반환합니다.
- **리다이렉트 (`redir`)**: `{scheme}`, `{host}`, `{uri}`, `{path}`, `{query}` 치환자를 확장해 3xx 리다이렉트(`301`, `302`, `307`, `308`)를 보냅니다.
- **응답 헤더 (`headers` / `header`)**: 헤더를 설정(`Header-Name value`)하거나 `-` 접두사로 제거(`-Server`)합니다.

### 경로 매처

- `""`, `*`, `/*`: 모든 경로와 일치합니다.
- `/exact`: 정확한 경로 일치입니다.
- `/api/*`, `*.php`: 글롭 와일드카드 (`*`는 `/`를 포함한 임의의 문자와 일치합니다).
- `~^/v[0-9]+/`: `~` 뒤에 오는 Go RE2 정규식입니다.

### `gymbag.json` 및 CLI의 라우트

`gymbag.json`에서는 사이트의 `routes` 배열에 라우트가 들어갑니다.

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

CLI 또는 `PATCH /api/v1/sites/{domain}`으로 라우트를 수정할 수 있습니다.

```bash
./gymbag config route list -d app.example.com
./gymbag config route add -d app.example.com --path /static/* --strip-prefix /static --root ./dist --browse
./gymbag config route update -d app.example.com --index 0 --fallback /index.html
./gymbag config route remove -d app.example.com --index 0
```

## nginx, Apache & Caddyfile v2 가져오기

Gymbag은 다음 형식을 직접 파싱합니다.
- **Caddyfile v2**: 스니펫 `(name) { ... }`, `import`(파일 글롭 및 `{args[n]}`), 이름 있는 `@matchers`(`path`, `path_regexp`, `method`), `handle`, `handle_path`, `route`, `root`, `file_server`, `try_files`, `respond`, `redir`, `header`.
- **nginx (`nginx.conf`)**: `http` / `server` / `location` 블록, `upstream` 풀, `proxy_pass`, `root` / `alias` / `index` / `autoindex` / `try_files`, `return`, `rewrite ... permanent|redirect`, `add_header`, `ssl_certificate` / `ssl_certificate_key`, `include`.
- **Apache (`httpd.conf` / `apache2.conf`)**: `<VirtualHost>`, `ServerName` / `ServerAlias`, `ProxyPass` / `<Proxy balancer://...>`, `DocumentRoot` / `DirectoryIndex`, `Redirect` / `RedirectMatch`, `Header set|unset`, `SSLCertificateFile` / `SSLCertificateKeyFile`, `Include` / `IncludeOptional`.

가져온 설정으로 바로 실행하거나 `gymbag.json` / `Gymbagfile`로 변환할 수 있습니다.

```bash
./gymbag run -c /etc/nginx/nginx.conf
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```
