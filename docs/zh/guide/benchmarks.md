# 性能基准测试（对比 Caddy 与 Nginx）

这组数据测试了 Gymbag 在启用 WAF、默认拦截规则和 AI/可疑流量检测的情况下，相对于纯 TLS 反向代理额外增加了多少开销。

## 测试环境与方法

所有测试都在同一台机器的相同参数下完成：使用**同一张 ECDSA P-256 证书**终结 TLS 1.3，后端代理到**同一个返回 `1 KiB`（`1024` 字节）响应体的 Go HTTP/1.1 上游服务**：

| 参数 | 配置 |
| :--- | :--- |
| **硬件** | Apple M4（10 核 CPU，16 GiB 内存） |
| **系统 / 运行时** | macOS 26.6.1（`darwin/arm64`），Go `1.27.0` |
| **参测代理** | **Gymbag** `v0.1.0`、**Caddy** `v2.11.9`、**Nginx** `1.31.8` |
| **并发** | `64` 个并发协程（HTTP/2 使用 `8` 条多路复用 TCP 连接） |
| **测试窗口** | 每个场景 `2s` 预热 + `8s` 正式采集（同一时间仅运行一个服务） |
| **上游连接池调优** | 三款代理均设为 `1024` 个空闲上游长连接并关闭访问日志（Nginx `access_log off`，Caddy `level ERROR`） |
| **直连上游基准** | `143,761 req/s`（明文 HTTP/1.1，`0.342 ms` p50，`1.817 ms` p99，`0` 错误） |

## HTTPS 反向代理对比（HTTP/1.1 长连接）

64 条并发 TLS 1.3 长连接，发送常规浏览器请求头（`User-Agent`、`Accept`、`Accept-Language`、`Sec-Fetch-Mode`）：

| 服务与模式 | 吞吐量 (RPS) | 平均延迟 | p50 | p95 | p99 | RSS 内存 | 错误数 |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **85,203** | 0.748 ms | 0.449 ms | 2.428 ms | 6.201 ms | 5.8 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 3`，全量 WAF 开启) | **72,517** | 0.880 ms | 0.743 ms | 1.936 ms | **3.141 ms** | 54.3 MiB | 0 |
| **Gymbag `v0.1.0`** (`protect off`，纯代理) | **72,479** | 0.880 ms | 0.744 ms | 1.909 ms | **3.154 ms** | 52.1 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 0`，已放行 Cookie) | **65,490** | 0.974 ms | 0.791 ms | 2.251 ms | 3.933 ms | ~54 MiB | 0 |
| **Caddy `v2.11.9`** (Go，keep-alive 调优) | **54,741** | 1.167 ms | 0.898 ms | 3.062 ms | 4.901 ms | 76.8 MiB | 0 |

## HTTPS 反向代理对比（HTTP/2）

64 个并发协程复用到 8 条 HTTP/2 TLS 1.3 连接：

| 服务与模式 | 吞吐量 (RPS) | 平均延迟 | p50 | p95 | p99 | RSS 内存 | 错误数 |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **88,775** | 0.717 ms | 0.649 ms | 1.419 ms | 1.960 ms | 5.8 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 3`，全量 WAF 开启) | **66,096** | 0.964 ms | 0.821 ms | 1.969 ms | 3.476 ms | 52.5 MiB | 0 |
| **Gymbag `v0.1.0`** (`protect off`，纯代理) | **64,048** | 0.995 ms | 0.831 ms | 2.085 ms | 3.940 ms | 47.0 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 0`，已放行 Cookie) | **62,108** | 1.026 ms | 0.838 ms | 2.186 ms | 4.265 ms | ~52 MiB | 0 |
| **Caddy `v2.11.9`** (Go，keep-alive 调优) | **39,207** | 1.628 ms | 1.303 ms | 3.701 ms | 6.183 ms | 70.7 MiB | 0 |

## 边缘 WAF 拦截（`/.env` 探测）

测量恶意探测直接在 Gymbag 边缘返回 `403 Forbidden`、完全不穿透到上游时的吞吐：

| 场景 | 吞吐量 (RPS) | 平均延迟 | p50 | p95 | p99 | 带宽 | 错误数 |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Gymbag 边缘拦截（`/.env`, HTTPS / HTTP/1.1）** | **140,680** | 0.452 ms | 0.348 ms | 1.203 ms | 2.028 ms | 340.6 MiB/s | 0 |

## 二进制体积与内存占用

| 服务 | 可执行文件体积 | 外部运行时依赖 | 稳态 RSS 内存（负载下） |
| :--- | ---: | :--- | ---: |
| **Gymbag `v0.1.0`** | **12.9 MB**（静态编译，含内嵌 UI 与图片） | 无（`CGO_ENABLED=0`） | 47 – 54 MiB |
| **Caddy `v2.11.9`** | 49.0 MB | 无 | 71 – 77 MiB |
| **Nginx `1.31.8`** | 1.5 MB | OpenSSL 4、PCRE2、libc | 5.8 MiB（master + workers） |

## 结论

1. **对比 Caddy**：在开启 WAF 的状态下，Gymbag 在 HTTP/1.1 上快约 **32%**（`72.5k` vs `54.7k` RPS），在 HTTP/2 上快 **63–69%**（`64.0k–66.1k` vs `39.2k` RPS），同时内存少约 30%，二进制小 3.8 倍。
2. **对比 Nginx**：C 语言编写的 Nginx 仍保有最高吞吐（`85.2k` / `88.8k` RPS）和极小内存占用（`5.8 MiB`）。Gymbag 跑到了 **Nginx HTTP/1.1 吞吐的 85%**，且 HTTP/1.1 p99 延迟更低（`3.14 ms` vs `6.20 ms`），同时在同一个二进制里承担 ACME、WAF 和威胁情报检查。
3. **WAF 开销**：`verify 3`（请求格式检查、默认拦截规则、分片限流以及 AI/可疑流量 DFA 匹配）与 `protect off` 相比差距在测试波动范围内（HTTP/1.1 为 `72.5k` vs `72.5k` RPS）。
4. **边缘拦截**：`/.env`、`/wp-admin` 等探测请求在 TLS 边缘即可达到 **140,680 req/s** 丢弃速率，不会到达业务后端。

## WAF 微基准测试（`go test -bench`）

在本地运行以下命令复测 WAF 引擎耗时：

```bash
go test -bench=BenchmarkEngineDecideHTTP -benchmem ./internal/waf
```

Apple M4（`darwin/arm64`，Go `1.27.0`）实测结果：

| 基准项 | 耗时 / 请求 | 内存 / 次 | 分配次数 / 次 |
| :--- | ---: | ---: | ---: |
| `ProtectOff`（格式检查 + ACME 放行） | **129.1 ns/op** | 240 B/op | 1 alloc/op |
| `Verify3_BlockDotEnv`（命中默认拦截规则） | **137.8 ns/op** | 288 B/op | 2 allocs/op |
| `Verify0_ClearedCookie`（命中分片 HMAC 缓存） | **899.8 ns/op** | 320 B/op | 3 allocs/op |
| `Verify3_BrowserPassThrough`（完整检查 + DFA） | **1,539 ns/op** | 320 B/op | 3 allocs/op |

## 热路径保持精简的设计

- **单遍 HTTPS 处理器**：单次解析路由、客户端 IP、HSTS 和 WebSocket 升级状态，不做 `r.WithContext` 克隆，也不生成中间态 map。
- **ASCII Aho-Corasick DFA**：内置 AI 爬虫签名、可疑客户端标记、查询攻击特征与自定义 `user_agent` 规则统一编译成大小写不敏感 DFA（`O(len(input))`，无需每请求转小写）。
- **精确哈希与简单通配符匹配**：精确路径和简单通配符（`prefix*`、`*suffix`、`*sub*`）绕过正则引擎；查询串中没有 `%` 或 `+` 时直接跳过 URL 解码。
- **64 分片限流器与放行缓存**：避免限流计数和已验证 `(token, host, IP)` 缓存查找时的跨核锁竞争。
- **预解析上游与缓冲池**：加载配置时即完成上游 URL 解析；`32 KiB` 拷贝缓冲区从 `sync.Pool` 获取；Transport 为每台主机池化 `1024` 个空闲连接（`DisableCompression: true`）；仅在 SSE 或未知 `Content-Length` 时触发流式 Flush。
- **预渲染响应与原子 SNI**：质询页 HTML 模板、拒绝页面、静态资源 ETag 以及 SNI 证书映射均通过 `atomic.Pointer` 替换。

## 本次测试使用的参考配置

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
