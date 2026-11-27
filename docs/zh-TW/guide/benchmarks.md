# 效能基準測試與表現

以下數據實測 Gymbag 在開啟 WAF、預設封鎖規則與 AI/可疑行為偵測的情況下，相較於單純作為 TLS 反向代理會增加多少額外負擔。

## 測試環境與方法

所有測試皆在同一台機器、相同參數下執行，使用**同一張 ECDSA P-256 憑證**終止 TLS 1.3，並代理至**同一個 Go HTTP/1.1 上游伺服器**（回傳 `1 KiB` / `1024` 位元組的內容）：

| 參數 | 數值 |
| :--- | :--- |
| **硬體** | Apple M4（10 核心 CPU、16 GiB 記憶體） |
| **作業系統 / 執行環境** | macOS 26.6.1（`darwin/arm64`）、Go `1.27.0` |
| **受測代理伺服器** | **Gymbag** `v0.1.0`、**Caddy** `v2.11.9`、**Nginx** `1.31.8` |
| **並行數** | `64` 個並行 worker（HTTP/2 使用 `8` 條多工 TCP 連線） |
| **測試區間** | 每個情境皆先進行 `2s` 預熱，再測量 `8s`（每次僅啟動單一伺服器） |
| **上游連線調校** | 三者皆設定 `1024` 個閒置上游 keep-alive 連線並關閉存取紀錄（Nginx 為 `access_log off`，Caddy 為 `level ERROR`） |
| **直接存取上游基準** | `143,761 req/s`（HTTP/1.1 明文、`0.342 ms` p50、`1.817 ms` p99、`0` 錯誤） |

## HTTPS 反向代理比較（HTTP/1.1 Keep-Alive）

64 條並行持久 TLS 1.3 連線，傳送標準瀏覽器標頭（`User-Agent`、`Accept`、`Accept-Language`、`Sec-Fetch-Mode`）：

| 伺服器與模式 | 吞吐量 (RPS) | 平均延遲 | p50 | p95 | p99 | RSS 記憶體 | 錯誤數 |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`**（C，`worker_processes auto`） | **85,203** | 0.748 ms | 0.449 ms | 2.428 ms | 6.201 ms | 5.8 MiB | 0 |
| **Gymbag `v0.1.0`**（`verify 3`，完整 WAF 啟用） | **72,517** | 0.880 ms | 0.743 ms | 1.936 ms | **3.141 ms** | 54.3 MiB | 0 |
| **Gymbag `v0.1.0`**（`protect off`，純代理模式） | **72,479** | 0.880 ms | 0.744 ms | 1.909 ms | **3.154 ms** | 52.1 MiB | 0 |
| **Gymbag `v0.1.0`**（`verify 0`，已持有通行 Cookie） | **65,490** | 0.974 ms | 0.791 ms | 2.251 ms | 3.933 ms | ~54 MiB | 0 |
| **Caddy `v2.11.9`**（Go，調校 keep-alive） | **54,741** | 1.167 ms | 0.898 ms | 3.062 ms | 4.901 ms | 76.8 MiB | 0 |

## HTTPS 反向代理比較（HTTP/2）

64 個並行 worker 透過 8 條 HTTP/2 TLS 1.3 連線進行多工傳輸：

| 伺服器與模式 | 吞吐量 (RPS) | 平均延遲 | p50 | p95 | p99 | RSS 記憶體 | 錯誤數 |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`**（C，`worker_processes auto`） | **88,775** | 0.717 ms | 0.649 ms | 1.419 ms | 1.960 ms | 5.8 MiB | 0 |
| **Gymbag `v0.1.0`**（`verify 3`，完整 WAF 啟用） | **66,096** | 0.964 ms | 0.821 ms | 1.969 ms | 3.476 ms | 52.5 MiB | 0 |
| **Gymbag `v0.1.0`**（`protect off`，純代理模式） | **64,048** | 0.995 ms | 0.831 ms | 2.085 ms | 3.940 ms | 47.0 MiB | 0 |
| **Gymbag `v0.1.0`**（`verify 0`，已持有通行 Cookie） | **62,108** | 1.026 ms | 0.838 ms | 2.186 ms | 4.265 ms | ~52 MiB | 0 |
| **Caddy `v2.11.9`**（Go，調校 keep-alive） | **39,207** | 1.628 ms | 1.303 ms | 3.701 ms | 6.183 ms | 70.7 MiB | 0 |

## 邊緣端 WAF 封鎖效能（`/.env` 探測請求）

測量在 Gymbag 邊緣端直接拒絕惡意探測（回傳 `403 Forbidden`）、完全不觸及上游伺服器的效能表現：

| 測試情境 | 吞吐量 (RPS) | 平均延遲 | p50 | p95 | p99 | 頻寬 | 錯誤數 |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Gymbag 邊緣封鎖（`/.env`，HTTPS / HTTP/1.1）** | **140,680** | 0.452 ms | 0.348 ms | 1.203 ms | 2.028 ms | 340.6 MiB/s | 0 |

## 執行檔大小與記憶體佔用

| 伺服器 | 執行檔大小 | 外部執行期相依性 | 穩定狀態 RSS（負載下） |
| :--- | ---: | :--- | ---: |
| **Gymbag `v0.1.0`** | **12.9 MB**（靜態編譯，內嵌 UI 與靜態資源） | 無（`CGO_ENABLED=0`） | 47 – 54 MiB |
| **Caddy `v2.11.9`** | 49.0 MB | 無 | 71 – 77 MiB |
| **Nginx `1.31.8`** | 1.5 MB | OpenSSL 4、PCRE2、libc | 5.8 MiB（master + workers） |

## 總結

1. **對比 Caddy**：Gymbag 在啟用 WAF 的情況下，HTTP/1.1 吞吐量約快 **32%**（`72.5k` vs `54.7k` RPS），HTTP/2 更是快上 **63–69%**（`64.0k–66.1k` vs `39.2k` RPS），同時記憶體佔用減少約 30%，執行檔體積縮小 3.8 倍。
2. **對比 Nginx**：以 C 語言撰寫的 Nginx 在吞吐量（`85.2k` / `88.8k` RPS）與極低 RSS 佔用（`5.8 MiB`）上仍保持領先。不過 Gymbag 在同一支執行檔內整合了 ACME、WAF 與威脅情資清單的同時，仍達到 **Nginx HTTP/1.1 吞吐量的 85%**，且 HTTP/1.1 的 p99 延遲更低（`3.14 ms` vs `6.20 ms`）。
3. **WAF 額外負擔**：`verify 3`（請求格式檢查、預設規則、分片速率限制與 AI/可疑特徵 DFA 比對）的表現幾乎與 `protect off` 在多次測試的誤差範圍內無異（HTTP/1.1 皆為 `72.5k` RPS）。
4. **邊緣攔截**：像 `/.env` 與 `/wp-admin` 這類探測請求在抵達後端之前，就能在 TLS 層以 **140,680 req/s** 的速度直接擋下。

## WAF 微基準測試（`go test -bench`）

可在本地端執行以下指令測試 WAF 引擎效能：

```bash
go test -bench=BenchmarkEngineDecideHTTP -benchmem ./internal/waf
```

於 Apple M4（`darwin/arm64`、Go `1.27.0`）實測結果：

| 基準測試項目 | 耗時 / 請求 | 記憶體配置 / 次 | 配置次數 / 次 |
| :--- | ---: | ---: | ---: |
| `ProtectOff`（請求格式檢查 + ACME 放行） | **129.1 ns/op** | 240 B/op | 1 alloc/op |
| `Verify3_BlockDotEnv`（命中預設封鎖規則） | **137.8 ns/op** | 288 B/op | 2 allocs/op |
| `Verify0_ClearedCookie`（已快取的 HMAC 通行驗證） | **899.8 ns/op** | 320 B/op | 3 allocs/op |
| `Verify3_BrowserPassThrough`（完整檢查 + DFA 比對） | **1,539 ns/op** | 320 B/op | 3 allocs/op |

## 熱路徑保持輕量的關鍵設計

- **單次掃描 HTTPS 處理器**：每個請求僅解析一次路由、用戶端 IP、HSTS 與 WebSocket 升級狀態，不透過 `r.WithContext` 複製上下文，也避免中間 Map 記憶體配置。
- **ASCII Aho-Corasick DFA**：內建 AI 機器人特徵、可疑用戶端字串、查詢攻擊模式與自訂 `user_agent` 規則皆編譯為不分大小寫的 DFA（時間複雜度 `O(len(input))`，無須針對每個請求轉換小寫）。
- **精確比對 Map 與簡易萬用字元**：完全符合的路徑與簡易萬用字元（`prefix*`、`*suffix`、`*sub*`）會跳過正規表達式引擎；若查詢字串不含 `%` 或 `+`，也會略過 URL 解碼。
- **64 分片速率限制器與通行快取**：減少多核心環境下計數器與已驗證 `(token, host, IP)` 快取查詢的鎖競爭。
- **預先解析上游與緩衝區物件池**：上游位址於載入設定時預先解析，`32 KiB` 代理複製緩衝區取自 `sync.Pool`，Transport 為每台主機保留 `1024` 個閒置連線（`DisableCompression: true`），且僅在 SSE 或未知 `Content-Length` 時啟用串流 Flush。
- **預渲染回應與原子化 SNI**：驗證挑戰 HTML 範本、封鎖頁面、靜態資源 ETag 與 SNI 憑證對照表皆透過 `atomic.Pointer` 進行無鎖切換。

## 本次基準測試使用的參考設定檔

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
