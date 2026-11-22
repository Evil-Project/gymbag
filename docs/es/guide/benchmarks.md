# Benchmarks y Rendimiento

Estas cifras miden cuánta sobrecarga añade Gymbag con el WAF, las reglas base de bloqueo y la detección de IA/tráfico sospechoso activados frente a operar como un proxy inverso TLS plano.

## Entorno de pruebas y metodología

Todas las pruebas se ejecutaron en la misma máquina bajo ajustes idénticos, terminando TLS 1.3 con el **mismo certificado ECDSA P-256** y haciendo proxy hacia el **mismo servidor upstream HTTP/1.1 en Go** que devuelve un cuerpo de `1 KiB` (`1024` bytes):

| Parámetro | Valor |
| :--- | :--- |
| **Hardware** | Apple M4 (CPU de 10 núcleos, 16 GiB RAM) |
| **SO / Runtime** | macOS 26.6.1 (`darwin/arm64`), Go `1.27.0` |
| **Proxies probados** | **Gymbag** `v0.1.0`, **Caddy** `v2.11.9`, **Nginx** `1.31.8` |
| **Concurrencia** | `64` workers concurrentes (`8` conexiones TCP multiplexadas para HTTP/2) |
| **Ventana** | `2s` de calentamiento + `8s` de medición por escenario (un servidor activo cada vez) |
| **Ajuste del upstream** | Los tres configurados con `1024` conexiones keep-alive inactivas hacia el upstream y logs de acceso desactivados (`access_log off` en Nginx, `level ERROR` en Caddy) |
| **Línea base directa al upstream** | `143,761 req/s` (HTTP/1.1 texto plano, `0.342 ms` p50, `1.817 ms` p99, `0` errores) |

## Comparativa de proxy inverso HTTPS (HTTP/1.1 Keep-Alive)

64 conexiones TLS 1.3 persistentes y concurrentes enviando cabeceras estándar de navegador (`User-Agent`, `Accept`, `Accept-Language`, `Sec-Fetch-Mode`):

| Servidor y modo | Caudal (RPS) | Latencia media | p50 | p95 | p99 | Memoria RSS | Errores |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **85,203** | 0.748 ms | 0.449 ms | 2.428 ms | 6.201 ms | 5.8 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 3`, WAF completo activo) | **72,517** | 0.880 ms | 0.743 ms | 1.936 ms | **3.141 ms** | 54.3 MiB | 0 |
| **Gymbag `v0.1.0`** (`protect off`, proxy puro) | **72,479** | 0.880 ms | 0.744 ms | 1.909 ms | **3.154 ms** | 52.1 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 0`, cookie válida) | **65,490** | 0.974 ms | 0.791 ms | 2.251 ms | 3.933 ms | ~54 MiB | 0 |
| **Caddy `v2.11.9`** (Go, keep-alive ajustado) | **54,741** | 1.167 ms | 0.898 ms | 3.062 ms | 4.901 ms | 76.8 MiB | 0 |

## Comparativa de proxy inverso HTTPS (HTTP/2)

64 workers concurrentes multiplexados sobre 8 conexiones HTTP/2 TLS 1.3:

| Servidor y modo | Caudal (RPS) | Latencia media | p50 | p95 | p99 | Memoria RSS | Errores |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **88,775** | 0.717 ms | 0.649 ms | 1.419 ms | 1.960 ms | 5.8 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 3`, WAF completo activo) | **66,096** | 0.964 ms | 0.821 ms | 1.969 ms | 3.476 ms | 52.5 MiB | 0 |
| **Gymbag `v0.1.0`** (`protect off`, proxy puro) | **64,048** | 0.995 ms | 0.831 ms | 2.085 ms | 3.940 ms | 47.0 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 0`, cookie válida) | **62,108** | 1.026 ms | 0.838 ms | 2.186 ms | 4.265 ms | ~52 MiB | 0 |
| **Caddy `v2.11.9`** (Go, keep-alive ajustado) | **39,207** | 1.628 ms | 1.303 ms | 3.701 ms | 6.183 ms | 70.7 MiB | 0 |

## Bloqueo WAF en el borde (sondeo `/.env`)

Medición de sondeos rechazados en el borde de Gymbag (`403 Forbidden`) sin tráfico hacia el upstream:

| Escenario | Caudal (RPS) | Latencia media | p50 | p95 | p99 | Ancho de banda | Errores |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Bloqueo en el borde de Gymbag (`/.env`, HTTPS / HTTP/1.1)** | **140,680** | 0.452 ms | 0.348 ms | 1.203 ms | 2.028 ms | 340.6 MiB/s | 0 |

## Tamaño del binario y consumo de memoria

| Servidor | Tamaño del ejecutable | Dependencias externas en tiempo de ejecución | RSS estacionario (bajo carga) |
| :--- | ---: | :--- | ---: |
| **Gymbag `v0.1.0`** | **12.9 MB** (estático, UI y recursos incrustados) | Ninguna (`CGO_ENABLED=0`) | 47 – 54 MiB |
| **Caddy `v2.11.9`** | 49.0 MB | Ninguna | 71 – 77 MiB |
| **Nginx `1.31.8`** | 1.5 MB | OpenSSL 4, PCRE2, libc | 5.8 MiB (master + workers) |

## Resumen

1. **Frente a Caddy**: Gymbag es alrededor de un **32% más rápido en HTTP/1.1** (`72.5k` frente a `54.7k` RPS) y un **63–69% más rápido en HTTP/2** (`64.0k–66.1k` frente a `39.2k` RPS), con ~30% menos de memoria y un binario 3.8 veces más pequeño manteniendo el WAF activo.
2. **Frente a Nginx**: Nginx en C conserva el liderazgo de caudal (`85.2k` / `88.8k` RPS) y una huella RSS mínima (`5.8 MiB`). Gymbag alcanza el **85% del caudal HTTP/1.1 de Nginx** con menor latencia p99 en HTTP/1.1 (`3.14 ms` frente a `6.20 ms`) mientras resuelve ACME, WAF y feeds de amenazas en el mismo binario.
3. **Sobrecarga del WAF**: `verify 3` (validación básica, reglas predeterminadas, limitación de tasa con shards y coincidencia DFA para IA/sospechosos) se mide dentro del margen de variación entre ejecuciones respecto a `protect off` (`72.5k` frente a `72.5k` RPS en HTTP/1.1).
4. **Descartes en el borde**: sondeos como `/.env` y `/wp-admin` se rechazan a **140,680 req/s** sobre TLS antes de tocar el backend.

## Microbenchmarks del WAF (`go test -bench`)

Ejecuta los benchmarks del motor WAF localmente con:

```bash
go test -bench=BenchmarkEngineDecideHTTP -benchmem ./internal/waf
```

Medido en Apple M4 (`darwin/arm64`, Go `1.27.0`):

| Benchmark | Tiempo / petición | Bytes / op | Asigns / op |
| :--- | ---: | ---: | ---: |
| `ProtectOff` (validación básica + bypass ACME) | **129.1 ns/op** | 240 B/op | 1 alloc/op |
| `Verify3_BlockDotEnv` (acierto en regla base de bloqueo) | **137.8 ns/op** | 288 B/op | 2 allocs/op |
| `Verify0_ClearedCookie` (autorización HMAC en caché) | **899.8 ns/op** | 320 B/op | 3 allocs/op |
| `Verify3_BrowserPassThrough` (inspección completa + DFA) | **1,539 ns/op** | 320 B/op | 3 allocs/op |

## Qué mantiene ligera la ruta crítica

- **Manejador HTTPS en una sola pasada**: resuelve ruta, IP del cliente, HSTS y actualización WebSocket una vez por petición sin clonar contextos vía `r.WithContext` ni asignar mapas intermedios.
- **DFA Aho-Corasick ASCII**: las firmas integradas de bots de IA, tokens de clientes sospechosos, patrones de ataque en query y reglas `user_agent` personalizadas se compilan en DFAs insensibles a mayúsculas (`O(len(input))`, sin pasar a minúsculas en cada petición).
- **Mapas exactos y globs simples**: las rutas exactas y los globs simples (`prefix*`, `*suffix`, `*sub*`) evitan evaluar expresiones regulares, y se omite decodificar la query cuando no contiene `%` ni `+`.
- **Limitador de tasa y caché de autorizaciones con 64 shards**: reduce la contención de bloqueos entre núcleos en contadores de tasa y búsquedas en caché de `(token, host, IP)`.
- **Upstreams preanalizados y buffers reutilizados**: los upstreams se analizan al cargar la configuración, los buffers de copia de `32 KiB` salen de `sync.Pool`, el transporte mantiene `1024` conexiones inactivas por host (`DisableCompression: true`) y el vaciado de streaming solo se activa para SSE o `Content-Length` desconocido.
- **Respuestas prerenderizadas y SNI atómico**: las plantillas HTML de desafío, páginas de rechazo, ETags de recursos y mapas de certificados SNI se intercambian mediante `atomic.Pointer`.

## Configuraciones de referencia usadas en este benchmark

::: code-group
```caddyfile [Gymbag (verify 3)]
{
    bind 127.0.0.1
    http_port 19081
    https_port 19446
    tls bench-cert.pem bench-key.pem
    auto_hosts off
    ip_security off
    ip_reputation off
    ai_ip_list off
}

localhost {
    reverse_proxy http://127.0.0.1:19080
    verify 3
    rate_limit 100000000 60
    verify_timeout 24h
}
```

```caddyfile [Caddy v2.11.9]
{
    admin off
    auto_https off
    log {
        level ERROR
    }
}

https://localhost:19444 {
    bind 127.0.0.1
    tls bench-cert.pem bench-key.pem
    reverse_proxy 127.0.0.1:19080 {
        transport http {
            keepalive_idle_conns 4096
            keepalive_idle_conns_per_host 1024
        }
    }
}
```

```nginx [Nginx 1.31.8]
worker_processes auto;
events { worker_connections 4096; }

http {
    access_log off;
    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_requests 100000;

    upstream backend {
        server 127.0.0.1:19080;
        keepalive 1024;
    }

    server {
        listen 127.0.0.1:19443 ssl;
        http2 on;
        server_name localhost;
        ssl_certificate     bench-cert.pem;
        ssl_certificate_key bench-key.pem;
        ssl_protocols       TLSv1.2 TLSv1.3;
        ssl_session_cache   shared:SSL:10m;

        location / {
            proxy_pass http://backend;
            proxy_http_version 1.1;
            proxy_set_header Connection "";
            proxy_set_header Host $host;
            proxy_set_header X-Forwarded-For $remote_addr;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-Proto https;
        }
    }
}
```
:::
