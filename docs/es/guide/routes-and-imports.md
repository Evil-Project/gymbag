# Rutas por Path, Archivos Estáticos e Importación de Configuraciones

Junto a `reverse_proxy`, un sitio en Gymbag puede declarar **rutas por path** ordenadas para archivos estáticos, respuestas fijas, redirecciones HTTP, modificación de cabeceras de respuesta y upstreams específicos por ruta. Gymbag también lee y convierte directamente archivos existentes de **nginx**, **Apache** y **Caddyfile v2**.

## Rutas por path ordenadas

Las rutas dentro de un sitio se evalúan de arriba abajo. El primer `path` que coincida atiende la petición; todo lo que no coincida pasa a los `upstreams` predeterminados del sitio (o devuelve `404` si no hay ninguno definido).

Las reglas WAF, los límites de tasa, la inteligencia de amenazas y los desafíos PoW se ejecutan siempre **antes** que cualquier manejador de ruta.

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

### Manejadores de ruta

Cada ruta utiliza un manejador:

- **Proxy inverso (`upstreams`)**: reenvía las peticiones HTTP coincidentes y actualizaciones WebSocket hacia los upstreams propios de la ruta, retirando opcionalmente un prefijo (`handle_path` / `strip_prefix`).
- **Servidor de archivos estáticos (`root` / `file_server`)**: sirve archivos desde `root` con archivos `index` (por defecto `["index.html"]`), listados de directorio (`browse`) y `fallback` para SPA (`try_files {path} /index.html`).
- **Respuesta estática (`respond`)**: devuelve de inmediato un código de estado y cuerpo sin abrir conexión al upstream.
- **Redirección (`redir`)**: envía una redirección 3xx (`301`, `302`, `307`, `308`) expandiendo `{scheme}`, `{host}`, `{uri}`, `{path}` y `{query}`.
- **Cabeceras de respuesta (`headers` / `header`)**: fija cabeceras (`Header-Name value`) o las elimina con prefijo `-` (`-Server`).

### Comparadores de path

- `""`, `*` o `/*`: coincide con cualquier ruta.
- `/exact`: coincidencia exacta de path.
- `/api/*`, `*.php`: comodín glob (`*` coincide con cualquier secuencia de caracteres, incluido `/`).
- `~^/v[0-9]+/`: expresión regular Go RE2 tras `~`.

### Rutas en `gymbag.json` y CLI

En `gymbag.json`, las rutas van en el array `routes` del sitio:

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

Edita rutas desde la CLI o mediante `PATCH /api/v1/sites/{domain}`:

```bash
./gymbag config route list -d app.example.com
./gymbag config route add -d app.example.com --path /static/* --strip-prefix /static --root ./dist --browse
./gymbag config route update -d app.example.com --index 0 --fallback /index.html
./gymbag config route remove -d app.example.com --index 0
```

## Importar nginx, Apache y Caddyfile v2

Gymbag lee directamente estos formatos:
- **Caddyfile v2**: snippets `(name) { ... }`, `import` (globs de archivo y `{args[n]}`), matchers nombrados `@matchers` (`path`, `path_regexp`, `method`), `handle`, `handle_path`, `route`, `root`, `file_server`, `try_files`, `respond`, `redir` y `header`.
- **nginx (`nginx.conf`)**: bloques `http` / `server` / `location`, pools `upstream`, `proxy_pass`, `root` / `alias` / `index` / `autoindex` / `try_files`, `return`, `rewrite ... permanent|redirect`, `add_header`, `ssl_certificate` / `ssl_certificate_key` e `include`.
- **Apache (`httpd.conf` / `apache2.conf`)**: `<VirtualHost>`, `ServerName` / `ServerAlias`, `ProxyPass` / `<Proxy balancer://...>`, `DocumentRoot` / `DirectoryIndex`, `Redirect` / `RedirectMatch`, `Header set|unset`, `SSLCertificateFile` / `SSLCertificateKeyFile` e `Include` / `IncludeOptional`.

Sirve directamente una configuración importada o conviértela a `gymbag.json` / `Gymbagfile`:

```bash
./gymbag run -c /etc/nginx/nginx.conf
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```
