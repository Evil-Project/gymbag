# Rutas por Path, Archivos Estáticos e Importación de Configuración

Además de `reverse_proxy` simple, un sitio de Gymbag puede contener **rutas por path** ordenadas para servir archivos estáticos, respuestas fijas, redirecciones HTTP, modificaciones de cabeceras de respuesta y upstreams específicos por ruta. Gymbag también lee y convierte de forma nativa configuraciones existentes de **nginx**, **Apache** y **Caddyfile v2**.

## Rutas por Path Ordenadas

Las rutas dentro de un sitio se evalúan en orden. La **primera** ruta cuyo `path` coincida gestiona la petición; las peticiones que no coincidan con ninguna ruta pasan a los `upstreams` predeterminados del sitio (o devuelven `404` cuando no hay un upstream predeterminado configurado).

Las reglas WAF, los límites de tasa, la inteligencia de amenazas y la verificación PoW siempre se ejecutan **antes** que cualquier manejador de ruta.

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

### Manejadores Soportados

Cada ruta utiliza exactamente un manejador:

- **Proxy Inverso (`upstreams`)**: Envía las peticiones coincidentes (y actualizaciones WebSocket) a upstreams específicos de la ruta, eliminando opcionalmente un prefijo (`handle_path` / `strip_prefix`).
- **Servidor de Archivos Estáticos (`root` / `file_server`)**: Sirve archivos desde `root` con archivos `index` configurables (por defecto `["index.html"]`), listado de directorios (`browse`) y `fallback` para SPA (p. ej. `try_files {path} /index.html`).
- **Respuesta Estática (`respond`)**: Escribe un código de estado y cuerpo inmediatos directamente desde Gymbag sin pasar por un upstream.
- **Redirección (`redir`)**: Devuelve una redirección HTTP (`301`, `302`, `307`, `308`) con marcadores de petición: `{scheme}`, `{host}`, `{uri}`, `{path}` y `{query}`.
- **Cabeceras de Respuesta (`headers` / `header`)**: Establece cabeceras (`Header-Name value`) o las elimina anteponiendo `-` a la clave (`-Server`).

### Coincidencia de Paths

- `""`, `*` o `/*`: Coincide con cualquier ruta de petición.
- `/exact`: Coincidencia exacta de path.
- `/api/*`, `*.php`: Comodín glob (`*` coincide con cualquier secuencia de caracteres, incluyendo `/`).
- `~^/v[0-9]+/`: Expresión regular Go RE2 después del `~` inicial.

### Gestión de Rutas en `gymbag.json` y CLI

En `gymbag.json`, las rutas se definen en el arreglo `routes` de cada sitio:

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

Gestiona las rutas desde la CLI o en vivo mediante `PATCH /api/v1/sites/{domain}`:

```bash
./gymbag config route list -d app.example.com
./gymbag config route add -d app.example.com --path /static/* --strip-prefix /static --root ./dist --browse
./gymbag config route update -d app.example.com --index 0 --fallback /index.html
./gymbag config route remove -d app.example.com --index 0
```

## Importación de nginx, Apache y Caddyfile v2

Gymbag analiza de forma nativa:
- **Caddyfile v2**: Snippets `(name) { ... }`, `import` (con globs de archivos y `{args[n]}`), `@matchers` con nombre (`path`, `path_regexp`, `method`), `handle`, `handle_path`, `route`, `root`, `file_server`, `try_files`, `respond`, `redir` y `header`.
- **nginx (`nginx.conf`)**: Bloques `http` / `server` / `location`, pools `upstream`, `proxy_pass`, `root` / `alias` / `index` / `autoindex` / `try_files`, `return`, `rewrite ... permanent|redirect`, `add_header`, `ssl_certificate` / `ssl_certificate_key` e `include`.
- **Apache (`httpd.conf` / `apache2.conf`)**: `<VirtualHost>`, `ServerName` / `ServerAlias`, `ProxyPass` / `<Proxy balancer://...>`, `DocumentRoot` / `DirectoryIndex`, `Redirect` / `RedirectMatch`, `Header set|unset`, `SSLCertificateFile` / `SSLCertificateKeyFile` e `Include` / `IncludeOptional`.

Ejecuta una configuración importada directamente o conviértela a `gymbag.json` / `Gymbagfile`:

```bash
# Servir directamente desde nginx.conf o httpd.conf
./gymbag run -c /etc/nginx/nginx.conf

# Convertir al formato nativo gymbag.json de Gymbag
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```
