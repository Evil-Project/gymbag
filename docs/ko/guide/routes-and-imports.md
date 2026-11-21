# 경로 라우트, 정적 파일 및 설정 가져오기

Gymbag 사이트는 일반 `reverse_proxy` 외에도 정적 파일 제공, 고정 응답, HTTP 리디렉션, 응답 헤더 변경, 경로별 업스트림을 위한 순서 기반 **경로 라우트(path routes)**를 포함할 수 있습니다. 또한 Gymbag은 기존 **nginx**, **Apache**, **Caddyfile v2** 설정을 기본으로 읽고 변환할 수 있습니다.

## 순서 기반 경로 라우트

사이트 내부의 라우트는 선언된 순서대로 평가됩니다. 요청과 `path`가 일치하는 **첫 번째** 라우트가 요청을 처리하며, 어떤 라우트와도 일치하지 않는 요청은 사이트의 기본 `upstreams`로 전달됩니다(기본 업스트림이 설정되지 않은 경우 `404` 반환).

WAF 규칙, 속도 제한, 위협 인텔리전스 및 PoW 검증은 항상 모든 라우트 핸들러**보다 먼저** 실행됩니다.

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

### 지원되는 핸들러

각 라우트는 정확히 하나의 핸들러를 사용합니다:

- **리버스 프록시 (`upstreams`)**: 일치하는 요청(및 WebSocket 업그레이드)을 라우트별 업스트림으로 프록시하며, 선택적으로 접두사를 제거할 수 있습니다(`handle_path` / `strip_prefix`).
- **정적 파일 서버 (`root` / `file_server`)**: 설정 가능한 `index` 파일(기본 `["index.html"]`), 디렉터리 목록(`browse`), SPA `fallback`(예: `try_files {path} /index.html`)과 함께 `root`에서 파일을 제공합니다.
- **정적 응답 (`respond`)**: 업스트림을 거치지 않고 Gymbag에서 직접 상태 코드와 본문을 즉시 반환합니다.
- **리디렉션 (`redir`)**: 요청 플레이스홀더(`{scheme}`, `{host}`, `{uri}`, `{path}`, `{query}`)를 포함한 HTTP 리디렉션(`301`, `302`, `307`, `308`)을 반환합니다.
- **응답 헤더 (`headers` / `header`)**: 헤더를 설정(`Header-Name value`)하거나 키 앞에 `-`를 붙여 삭제(`-Server`)합니다.

### 경로 매처

- `""`, `*`, 또는 `/*`: 모든 요청 경로와 일치합니다.
- `/exact`: 정확히 일치하는 경로입니다.
- `/api/*`, `*.php`: 글롭 와일드카드(`*`는 `/`를 포함한 임의의 문자열과 일치).
- `~^/v[0-9]+/`: 선행 `~` 뒤에 오는 Go RE2 정규식입니다.

### `gymbag.json` 및 CLI에서 라우트 관리

`gymbag.json`에서는 각 사이트의 `routes` 배열에 라우트가 위치합니다:

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

CLI를 사용하거나 `PATCH /api/v1/sites/{domain}`을 통해 실시간으로 라우트를 관리할 수 있습니다:

```bash
./gymbag config route list -d app.example.com
./gymbag config route add -d app.example.com --path /static/* --strip-prefix /static --root ./dist --browse
./gymbag config route update -d app.example.com --index 0 --fallback /index.html
./gymbag config route remove -d app.example.com --index 0
```

## nginx, Apache 및 Caddyfile v2 가져오기

Gymbag은 다음 형식을 기본으로 파싱합니다:
- **Caddyfile v2**: 스니펫 `(name) { ... }`, `import`(파일 글롭 및 `{args[n]}` 지원), 이름 지정 `@matchers`(`path`, `path_regexp`, `method`), `handle`, `handle_path`, `route`, `root`, `file_server`, `try_files`, `respond`, `redir`, `header`.
- **nginx (`nginx.conf`)**: `http` / `server` / `location` 블록, `upstream` 풀, `proxy_pass`, `root` / `alias` / `index` / `autoindex` / `try_files`, `return`, `rewrite ... permanent|redirect`, `add_header`, `ssl_certificate` / `ssl_certificate_key`, `include`.
- **Apache (`httpd.conf` / `apache2.conf`)**: `<VirtualHost>`, `ServerName` / `ServerAlias`, `ProxyPass` / `<Proxy balancer://...>`, `DocumentRoot` / `DirectoryIndex`, `Redirect` / `RedirectMatch`, `Header set|unset`, `SSLCertificateFile` / `SSLCertificateKeyFile`, `Include` / `IncludeOptional`.

가져온 설정으로 바로 실행하거나 `gymbag.json` / `Gymbagfile`로 변환할 수 있습니다:

```bash
# nginx.conf 또는 httpd.conf에서 직접 실행
./gymbag run -c /etc/nginx/nginx.conf

# Gymbag 기본 gymbag.json 형식으로 변환
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```
