# 性能基准测试（对比 Caddy 与 Nginx）

Gymbag 的设计目标是在启用内置 Proof-of-Work WAF、自定义/远程防火墙规则以及 AI/可疑流量检测的同时，依然保持极高的 HTTPS 反向代理吞吐性能。

## 测试环境与方法

所有基准测试均在同一台机器的相同条件下进行：使用**完全相同的 ECDSA P-256 证书**终止 TLS 1.3，并反向代理到**同一个返回 `1 KiB`（`1024` 字节）响应体的 Go HTTP/1.1 上游服务器**：

| 参数 | 配置详情 |
| :--- | :--- |
| **硬件环境** | Apple M4（10 核 CPU，16 GiB 内存） |
| **系统 / 运行时** | macOS 26.6.1 (`darwin/arm64`)，Go `1.27.0` |
| **参测服务** | **Gymbag** `v0.1.0`、**Caddy** `v2.11.9`、**Nginx** `1.31.8` |
| **并发模型** | `64` 个并发工作协程（HTTP/2 模式使用 `8` 条多路复用 TCP 连接） |
| **测试窗口** | 每次测试 `2s` 预热 + `8s` 正式采集（同一时间仅运行一个代理进程） |
| **上游连接池调优** | 三款代理均配置 `1024` 上游空闲长连接并关闭访问日志（Nginx `access_log off`，Caddy `level ERROR`） |
| **直连上游基准** | `143,761 req/s`（明文 HTTP/1.1，`0.342 ms` p50，`1.817 ms` p99，`0` 错误） |

---

## HTTPS 反向代理对比（HTTP/1.1 长连接）

64 条并发 TLS 1.3 长连接，模拟真实浏览器请求头（`User-Agent`、`Accept`、`Accept-Language`、`Sec-Fetch-Mode`）：

| 服务与模式 | 吞吐量 (RPS) | 平均延迟 | p50 | p95 | p99 | 常驻内存 (RSS) | 错误数 |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **85,203** | 0.748 ms | 0.449 ms | 2.428 ms | 6.201 ms | 5.8 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 3`, 开启完整 WAF) | **72,517** | 0.880 ms | 0.743 ms | 1.936 ms | **3.141 ms** | 54.3 MiB | 0 |
| **Gymbag `v0.1.0`** (`protect off`, 纯净代理) | **72,479** | 0.880 ms | 0.744 ms | 1.909 ms | **3.154 ms** | 52.1 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 0`, 已验证 Cookie) | **65,490** | 0.974 ms | 0.791 ms | 2.251 ms | 3.933 ms | ~54 MiB | 0 |
| **Caddy `v2.11.9`** (Go, 调优长连接池) | **54,741** | 1.167 ms | 0.898 ms | 3.062 ms | 4.901 ms | 76.8 MiB | 0 |

---

## HTTPS 反向代理对比（HTTP/2）

64 个并发协程复用到 8 条 HTTP/2 TLS 1.3 连接：

| 服务与模式 | 吞吐量 (RPS) | 平均延迟 | p50 | p95 | p99 | 常驻内存 (RSS) | 错误数 |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **88,775** | 0.717 ms | 0.649 ms | 1.419 ms | 1.960 ms | 5.8 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 3`, 开启完整 WAF) | **66,096** | 0.964 ms | 0.821 ms | 1.969 ms | 3.476 ms | 52.5 MiB | 0 |
| **Gymbag `v0.1.0`** (`protect off`, 纯净代理) | **64,048** | 0.995 ms | 0.831 ms | 2.085 ms | 3.940 ms | 47.0 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 0`, 已验证 Cookie) | **62,108** | 1.026 ms | 0.838 ms | 2.186 ms | 4.265 ms | ~52 MiB | 0 |
| **Caddy `v2.11.9`** (Go, 调优长连接池) | **39,207** | 1.628 ms | 1.303 ms | 3.701 ms | 6.183 ms | 70.7 MiB | 0 |

---

## 边缘 WAF 攻击拦截（`/.env` 漏洞探测）

测试恶意探测请求直接在 Gymbag 边缘被拦截（`403 Forbidden`）而不触及上游服务时的吞吐：

| 场景 | 吞吐量 (RPS) | 平均延迟 | p50 | p95 | p99 | 响应带宽 | 错误数 |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Gymbag 边缘拦截 (`/.env`, HTTPS / HTTP/1.1)** | **140,680** | 0.452 ms | 0.348 ms | 1.203 ms | 2.028 ms | 340.6 MiB/s | 0 |

---

## 二进制体积与内存占用

| 服务 | 可执行文件体积 | 外部运行时依赖 | 压测稳态内存 (RSS) |
| :--- | ---: | :--- | ---: |
| **Gymbag `v0.1.0`** | **12.9 MB**（静态编译，含内嵌 UI 与图片） | 无 (`CGO_ENABLED=0`) | 47 – 54 MiB |
| **Caddy `v2.11.9`** | 49.0 MB | 无 | 71 – 77 MiB |
| **Nginx `1.31.8`** | 1.5 MB | OpenSSL 4、PCRE2、libc | 5.8 MiB（Master + Workers） |

---

## 核心结论

1. **HTTP/1.1 与 HTTP/2 均全面领先 Caddy**：在开启完整 WAF 的情况下，Gymbag 的 HTTP/1.1 吞吐量比 Caddy **高出 32%**（`72.5k` vs `54.7k`），HTTP/2 吞吐量 **高出 63%–69%**（`64.0k–66.1k` vs `39.2k`），同时内存占用降低 **30%**，二进制体积小 **3.8 倍**。
2. **逼近纯 C 编写的 Nginx**：在内存安全的纯 Go 标准库实现下，Gymbag 达到 Nginx HTTP/1.1 吞吐的 **85%**，且 p99 尾延迟比 Nginx **低近 2 倍**（`3.14 ms` vs `6.20 ms`），HTTP/2 达到 Nginx 的 **74%**，同时内置了 PoW WAF、多 CA ACME、本地 PKI 与威胁情报。
3. **WAF 检测开销几乎可忽略**：启用 `verify 3`（请求形状检查、内置拦截规则、分片限流及 AI/可疑识别）与 `protect off` 纯反向代理模式吞吐基本持平（HTTP/1.1 分别为 `72,517` 与 `72,479` RPS）。
4. **高速边缘防护**：对 `/.env`、`/wp-admin` 等恶意探测可在 TLS 边缘以 **140,680 req/s** 直接拦截，彻底保护后端服务。

---

## WAF 引擎微基准测试 (`go test -bench`)

您可随时在本地运行以下命令验证 WAF 规则引擎耗时：

```bash
go test -bench=BenchmarkEngineDecideHTTP -benchmem ./internal/waf
```

Apple M4（`darwin/arm64`，Go `1.27.0`）实测结果：

| 基准项 | 单次耗时 | 内存分配 | 分配次数 |
| :--- | ---: | ---: | ---: |
| `ProtectOff`（形状检查 + ACME 放行） | **129.1 ns/op** | 240 B/op | 1 alloc/op |
| `Verify3_BlockDotEnv`（命中默认拦截规则） | **137.8 ns/op** | 288 B/op | 2 allocs/op |
| `Verify0_ClearedCookie`（命中分片 HMAC 缓存） | **899.8 ns/op** | 320 B/op | 3 allocs/op |
| `Verify3_BrowserPassThrough`（完整检查 + DFA 匹配） | **1,539 ns/op** | 320 B/op | 3 allocs/op |

---

## 底层架构优化

Gymbag 在不引入任何第三方 Go 依赖的前提下，通过以下热路径工程优化实现高性能：

- **融合式零克隆 HTTPS 流水线**：单次解析站点路由、客户端 IP、HSTS 与 WebSocket 状态，避免 `r.WithContext` 请求克隆以及中间态 Header/Cookie Map 分配。
- **零分配 Aho-Corasick DFA**：将内置 AI 爬虫特征、可疑 UA、查询注入特征及自定义 `user_agent` 规则预编译为完备的忽略大小写 ASCII DFA（`O(len(input))` 单遍扫描，无需转小写分配）。
- **O(1) 精确路径哈希与快速通配符**：精确路径使用哈希表、`prefix*` / `*suffix` / `*sub*` 走快速路径绕过正则引擎；空查询或无 `%`/`+` 时跳过 URL 解码。
- **64 分片限流器与验证缓存**：通过 64 个缓存行对齐分片消除多核锁竞争，在严格校验过期时间的同时缓存已通过验证的 `(token, host, IP)` 结果。
- **预解析上游与代理缓冲池**：路由重载时预解析上游 URL，使用 `sync.Pool` 复用 32 KiB 拷贝缓冲区，上游连接池预设 `1024` 单主机空闲连接，并仅对 SSE/未知长度流式响应逐块 Flush。
- **静态资源预渲染与无锁 SNI**：内嵌图片 ETag、拦截 HTML 与拆分后的质询模板均在启动时预计算，TLS SNI 证书通过 `atomic.Pointer` 无锁读取。
