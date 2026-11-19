# Benchmarks y Rendimiento (vs Caddy y Nginx)

Gymbag está diseñado como un proxy inverso HTTPS de alto rendimiento sin sacrificar velocidad cuando su WAF Proof-of-Work, reglas locales/remotas y detectores de IA están activos.

## Entorno de Pruebas y Metodología

Todas las pruebas se ejecutaron en la misma máquina en condiciones idénticas, terminando TLS 1.3 con el **mismo certificado ECDSA P-256** y conectando al **mismo upstream Go HTTP/1.1** (`1 KiB` / `1024` bytes):

| Parámetro | Valor |
| :--- | :--- |
| **Hardware** | Apple M4 (CPU 10 núcleos, 16 GiB RAM) |
| **SO / Runtime** | macOS 26.6.1 (`darwin/arm64`), Go `1.27.0` |
| **Servidores Probados** | **Gymbag** `v0.1.0`, **Caddy** `v2.11.9`, **Nginx** `1.31.8` |
| **Concurrencia** | `64` workers simultáneos (`8` conexiones TCP multiplexadas en HTTP/2) |
| **Ventana** | `2s` calentamiento + `8s` medición (un servidor a la vez) |
| **Ajuste Upstream** | `1024` conexiones keep-alive inactivas y logs de acceso desactivados en los tres |
| **Línea Base Directa** | `143,761 req/s` (HTTP/1.1 plano, `0.342 ms` p50, `1.817 ms` p99, `0` errores) |

---

## Comparativa Proxy Inverso HTTPS (HTTP/1.1 Keep-Alive)

| Servidor y Modo | Rendimiento (RPS) | Latencia Media | p50 | p95 | p99 | Memoria RSS | Errores |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **85,203** | 0.748 ms | 0.449 ms | 2.428 ms | 6.201 ms | 5.8 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 3`, WAF completo) | **72,517** | 0.880 ms | 0.743 ms | 1.936 ms | **3.141 ms** | 54.3 MiB | 0 |
| **Gymbag `v0.1.0`** (`protect off`, proxy puro) | **72,479** | 0.880 ms | 0.744 ms | 1.909 ms | **3.154 ms** | 52.1 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 0`, cookie validada) | **65,490** | 0.974 ms | 0.791 ms | 2.251 ms | 3.933 ms | ~54 MiB | 0 |
| **Caddy `v2.11.9`** (Go, keep-alive ajustado) | **54,741** | 1.167 ms | 0.898 ms | 3.062 ms | 4.901 ms | 76.8 MiB | 0 |

---

## Comparativa Proxy Inverso HTTPS (HTTP/2)

| Servidor y Modo | Rendimiento (RPS) | Latencia Media | p50 | p95 | p99 | Memoria RSS | Errores |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **88,775** | 0.717 ms | 0.649 ms | 1.419 ms | 1.960 ms | 5.8 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 3`, WAF completo) | **66,096** | 0.964 ms | 0.821 ms | 1.969 ms | 3.476 ms | 52.5 MiB | 0 |
| **Gymbag `v0.1.0`** (`protect off`, proxy puro) | **64,048** | 0.995 ms | 0.831 ms | 2.085 ms | 3.940 ms | 47.0 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 0`, cookie validada) | **62,108** | 1.026 ms | 0.838 ms | 2.186 ms | 4.265 ms | ~52 MiB | 0 |
| **Caddy `v2.11.9`** (Go, keep-alive ajustado) | **39,207** | 1.628 ms | 1.303 ms | 3.701 ms | 6.183 ms | 70.7 MiB | 0 |

---

## Bloqueo WAF en el Borde (Sondeo `/.env`)

| Escenario | Rendimiento (RPS) | Latencia Media | p50 | p95 | p99 | Ancho de Banda | Errores |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Gymbag Edge Block (`/.env`, HTTPS / HTTP/1.1)** | **140,680** | 0.452 ms | 0.348 ms | 1.203 ms | 2.028 ms | 340.6 MiB/s | 0 |

---

## Tamaño del Binario y Consumo de Memoria

| Servidor | Tamaño Ejecutable | Dependencias Externas | RSS Bajo Carga |
| :--- | ---: | :--- | ---: |
| **Gymbag `v0.1.0`** | **12.9 MB** (estático con UI y PNGs) | Ninguna (`CGO_ENABLED=0`) | 47 – 54 MiB |
| **Caddy `v2.11.9`** | 49.0 MB | Ninguna | 71 – 77 MiB |
| **Nginx `1.31.8`** | 1.5 MB | OpenSSL 4, PCRE2, libc | 5.8 MiB (Master + Workers) |

---

## Conclusiones Clave

1. **Más rápido que Caddy en HTTP/1.1 y HTTP/2**: Gymbag ofrece **+32% más RPS en HTTP/1.1** (`72.5k` vs `54.7k`) y **+63% a +69% más en HTTP/2** (`64.0k–66.1k` vs `39.2k`) con **30% menos memoria** y un binario **3.8x más pequeño**, incluso con el WAF completo activo.
2. **Competitivo frente a Nginx en C**: Alcanza el **85% del rendimiento HTTP/1.1 de Nginx** con **casi 2x menor latencia p99** (`3.14 ms` vs `6.20 ms`) y **74% en HTTP/2**, escrito íntegramente en Go estándar con memoria segura.
3. **Sobrecarga WAF prácticamente nula**: `verify 3` rinde igual que `protect off` (`72,517` vs `72,479` RPS en HTTP/1.1).
4. **Mitigación ultrarrápida en el borde**: Sondeos como `/.env` se bloquean sobre TLS a **140,680 req/s** sin tocar el backend.

---

## Microbenchmarks del Motor WAF (`go test -bench`)

```bash
go test -bench=BenchmarkEngineDecideHTTP -benchmem ./internal/waf
```

| Benchmark | Tiempo / Op | Bytes / Op | Allocs / Op |
| :--- | ---: | ---: | ---: |
| `ProtectOff` | **129.1 ns/op** | 240 B/op | 1 alloc/op |
| `Verify3_BlockDotEnv` | **137.8 ns/op** | 288 B/op | 2 allocs/op |
| `Verify0_ClearedCookie` | **899.8 ns/op** | 320 B/op | 3 allocs/op |
| `Verify3_BrowserPassThrough` | **1,539 ns/op** | 320 B/op | 3 allocs/op |
