# Benchmarks & Performance

Gymbag is engineered to act as a high-throughput HTTPS reverse proxy **without** sacrificing speed when its built-in Proof-of-Work WAF, custom/remote firewall rules, and AI/suspicion detectors are enabled.

## Test Environment & Methodology

All benchmarks were executed on the same local machine under identical conditions, terminating TLS 1.3 with the **exact same ECDSA P-256 certificate** and proxying to the **exact same Go HTTP/1.1 upstream server** returning a `1 KiB` (`1024`-byte) response body:

| Parameter | Value |
| :--- | :--- |
| **Hardware** | Apple M4 (10-core CPU, 16 GiB RAM) |
| **OS / Runtime** | macOS 26.6.1 (`darwin/arm64`), Go `1.27.0` |
| **Proxies Tested** | **Gymbag** `v0.1.0`, **Caddy** `v2.11.9`, **Nginx** `1.31.8` |
| **Concurrency** | `64` concurrent workers (`8` multiplexed TCP connections for HTTP/2) |
| **Window** | `2s` warmup + `8s` measured per scenario (one server active at a time) |
| **Upstream Tuning** | All three proxies configured with `1024` idle upstream keep-alive connections and access logging disabled (`access_log off` on Nginx, `level ERROR` on Caddy) |
| **Direct Upstream Baseline** | `143,761 req/s` (HTTP/1.1 plaintext, `0.342 ms` p50, `1.817 ms` p99, `0` errors) |

---

## HTTPS Reverse Proxy Comparison (HTTP/1.1 Keep-Alive)

64 concurrent persistent TLS 1.3 connections sending realistic browser headers (`User-Agent`, `Accept`, `Accept-Language`, `Sec-Fetch-Mode`):

| Server & Mode | Throughput (RPS) | Avg Latency | p50 | p95 | p99 | RSS Memory | Errors |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **85,203** | 0.748 ms | 0.449 ms | 2.428 ms | 6.201 ms | 5.8 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 3`, full WAF active) | **72,517** | 0.880 ms | 0.743 ms | 1.936 ms | **3.141 ms** | 54.3 MiB | 0 |
| **Gymbag `v0.1.0`** (`protect off`, pure proxy) | **72,479** | 0.880 ms | 0.744 ms | 1.909 ms | **3.154 ms** | 52.1 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 0`, cleared cookie) | **65,490** | 0.974 ms | 0.791 ms | 2.251 ms | 3.933 ms | ~54 MiB | 0 |
| **Caddy `v2.11.9`** (Go, tuned keep-alive) | **54,741** | 1.167 ms | 0.898 ms | 3.062 ms | 4.901 ms | 76.8 MiB | 0 |

---

## HTTPS Reverse Proxy Comparison (HTTP/2)

64 concurrent workers multiplexed across 8 HTTP/2 TLS 1.3 connections:

| Server & Mode | Throughput (RPS) | Avg Latency | p50 | p95 | p99 | RSS Memory | Errors |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **88,775** | 0.717 ms | 0.649 ms | 1.419 ms | 1.960 ms | 5.8 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 3`, full WAF active) | **66,096** | 0.964 ms | 0.821 ms | 1.969 ms | 3.476 ms | 52.5 MiB | 0 |
| **Gymbag `v0.1.0`** (`protect off`, pure proxy) | **64,048** | 0.995 ms | 0.831 ms | 2.085 ms | 3.940 ms | 47.0 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 0`, cleared cookie) | **62,108** | 1.026 ms | 0.838 ms | 2.186 ms | 4.265 ms | ~52 MiB | 0 |
| **Caddy `v2.11.9`** (Go, tuned keep-alive) | **39,207** | 1.628 ms | 1.303 ms | 3.701 ms | 6.183 ms | 70.7 MiB | 0 |

---

## Edge WAF Attack Blocking (`/.env` Exploit Probe)

Measuring malicious requests dropped directly at the Gymbag edge (`403 Forbidden`) before touching the upstream server:

| Scenario | Throughput (RPS) | Avg Latency | p50 | p95 | p99 | Bandwidth | Errors |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Gymbag Edge Block (`/.env`, HTTPS / HTTP/1.1)** | **140,680** | 0.452 ms | 0.348 ms | 1.203 ms | 2.028 ms | 340.6 MiB/s | 0 |

---

## Binary Size & Memory Footprint

| Server | Executable Size | External Runtime Deps | Steady-State RSS (Under Load) |
| :--- | ---: | :--- | ---: |
| **Gymbag `v0.1.0`** | **12.9 MB** (static, embedded UI & assets) | None (`CGO_ENABLED=0`) | 47 – 54 MiB |
| **Caddy `v2.11.9`** | 49.0 MB | None | 71 – 77 MiB |
| **Nginx `1.31.8`** | 1.5 MB | OpenSSL 4, PCRE2, libc | 5.8 MiB (master + workers) |

---

## Key Takeaways

1. **Faster than Caddy across both HTTP/1.1 and HTTP/2**: Gymbag delivers **+32% higher RPS on HTTP/1.1** (`72.5k` vs `54.7k`) and **+63% to +69% higher RPS on HTTP/2** (`64.0k–66.1k` vs `39.2k`) with **30% lower memory usage** and a **3.8x smaller binary** — even with Gymbag's full WAF active.
2. **Competitive with C-based Nginx**: Gymbag achieves **85% of Nginx's HTTP/1.1 throughput** with **nearly 2x lower p99 tail latency** (`3.14 ms` vs `6.20 ms`) and **74% of Nginx's HTTP/2 throughput**, while written in memory-safe pure Go stdlib with built-in PoW WAF, multi-CA ACME, local PKI, and threat intelligence.
3. **Negligible WAF inspection overhead**: Enabling `verify 3` (request-shape checks, built-in block rules, sharded rate limiting, and AI/suspicion detection) stays within measurement noise of `protect off` (`72.5k` vs `72.5k` RPS on HTTP/1.1).
4. **High-speed edge mitigation**: Malicious probes like `/.env`, `/wp-admin`, and vulnerability scanners are rejected at **140,680 req/s over TLS**, shielding upstream applications completely.

---

## WAF Engine Microbenchmarks (`go test -bench`)

You can verify WAF rule-engine performance locally at any time with:

```bash
go test -bench=BenchmarkEngineDecideHTTP -benchmem ./internal/waf
```

Measured on Apple M4 (`darwin/arm64`, Go `1.27.0`):

| Benchmark | Time / Request | Bytes / Op | Allocs / Op |
| :--- | ---: | ---: | ---: |
| `ProtectOff` (shape check + ACME bypass) | **129.1 ns/op** | 240 B/op | 1 alloc/op |
| `Verify3_BlockDotEnv` (default block rule hit) | **137.8 ns/op** | 288 B/op | 2 allocs/op |
| `Verify0_ClearedCookie` (cached HMAC clearance) | **899.8 ns/op** | 320 B/op | 3 allocs/op |
| `Verify3_BrowserPassThrough` (full inspection + DFA) | **1,539 ns/op** | 320 B/op | 3 allocs/op |

---

## Architecture Optimizations Under the Hood

Gymbag achieves these numbers without any external Go dependencies through focused hot-path engineering:

- **Fused zero-clone HTTPS pipeline**: Resolves host route, client IP, HSTS, and WebSocket upgrade state once per request without `r.WithContext` request cloning or intermediate header/cookie map allocations.
- **Zero-allocation Aho-Corasick DFA**: Built-in AI bot signatures, suspicious client signatures, query attack tokens, and custom `user_agent` rules compile into pre-completed case-insensitive ASCII DFAs (`O(len(input))`, single pass, zero lowercasing allocations).
- **Exact-map & fast-glob path matching**: Simple globs (`prefix*`, `*suffix`, `*sub*`) and exact paths avoid regex evaluation entirely; query unescaping is skipped when queries are empty or contain no `%`/`+`.
- **64-shard rate limiter & clearance token cache**: Eliminates cross-core mutex contention on per-IP rate limits and caches verified `(token, host, IP) -> expiresAt` entries across 64 cache-line-padded shards (still strictly enforcing expiry timestamps).
- **Pre-parsed upstreams & pooled proxy buffers**: Upstreams are parsed once at route load, response copy buffers (`32 KiB`) are recycled via `sync.Pool`, upstream transport pools `1024` idle connections per host with `DisableCompression: true`, and per-chunk flushing is reserved strictly for streaming responses (`SSE` / unknown `Content-Length`).
- **Pre-rendered static WAF responses & lock-free SNI**: Embedded mascot assets, ETags, rejection HTML pages, and split challenge templates are pre-computed at startup; TLS SNI certificates are served lock-free via `atomic.Pointer`.

---

## Reference Configs Used in This Benchmark

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
