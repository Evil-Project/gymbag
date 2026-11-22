# ベンチマーク & パフォーマンス

ここでは、WAF・デフォルトブロックルール・AI/不審トラフィック検出を有効にした状態で、プレーンな TLS リバースプロキシと比較してどれだけのオーバーヘッドが生じるかを計測しています。

## テスト環境と計測方法

すべての計測は同一マシン・同一条件で実施し、**同一の ECDSA P-256 証明書**で TLS 1.3 を終端し、`1 KiB`（`1024` バイト）のボディを返す**同一の Go HTTP/1.1 アップストリームサーバー**へプロキシしました。

| 項目 | 値 |
| :--- | :--- |
| **ハードウェア** | Apple M4 (10 コア CPU, 16 GiB RAM) |
| **OS / ランタイム** | macOS 26.6.1 (`darwin/arm64`), Go `1.27.0` |
| **比較対象プロキシ** | **Gymbag** `v0.1.0`、**Caddy** `v2.11.9`、**Nginx** `1.31.8` |
| **同時実行数** | `64` 並列ワーカー（HTTP/2 は `8` 本の多重化 TCP 接続） |
| **計測時間** | 各シナリオ `2s` ウォームアップ + `8s` 本計測（1 サーバーずつ単独起動） |
| **アップストリーム調整** | 3 製品ともアイドルアップストリーム keep-alive を `1024` に設定し、アクセスログを無効化（Nginx は `access_log off`、Caddy は `level ERROR`） |
| **アップストリーム直結基準** | `143,761 req/s`（平文 HTTP/1.1、`0.342 ms` p50、`1.817 ms` p99、`0` エラー） |

## HTTPS リバースプロキシ比較（HTTP/1.1 Keep-Alive）

一般的なブラウザヘッダー（`User-Agent`、`Accept`、`Accept-Language`、`Sec-Fetch-Mode`）を付与した 64 本の持続的 TLS 1.3 接続での計測結果です。

| サーバー＆モード | スループット (RPS) | 平均レイテンシ | p50 | p95 | p99 | RSS メモリ | エラー |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **85,203** | 0.748 ms | 0.449 ms | 2.428 ms | 6.201 ms | 5.8 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 3`, フル WAF 有効) | **72,517** | 0.880 ms | 0.743 ms | 1.936 ms | **3.141 ms** | 54.3 MiB | 0 |
| **Gymbag `v0.1.0`** (`protect off`, 純プロキシ) | **72,479** | 0.880 ms | 0.744 ms | 1.909 ms | **3.154 ms** | 52.1 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 0`, Cookie 認証済) | **65,490** | 0.974 ms | 0.791 ms | 2.251 ms | 3.933 ms | ~54 MiB | 0 |
| **Caddy `v2.11.9`** (Go, keep-alive 調整済) | **54,741** | 1.167 ms | 0.898 ms | 3.062 ms | 4.901 ms | 76.8 MiB | 0 |

## HTTPS リバースプロキシ比較（HTTP/2）

8 本の HTTP/2 TLS 1.3 接続上で 64 ワーカーを多重化した計測結果です。

| サーバー＆モード | スループット (RPS) | 平均レイテンシ | p50 | p95 | p99 | RSS メモリ | エラー |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **88,775** | 0.717 ms | 0.649 ms | 1.419 ms | 1.960 ms | 5.8 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 3`, フル WAF 有効) | **66,096** | 0.964 ms | 0.821 ms | 1.969 ms | 3.476 ms | 52.5 MiB | 0 |
| **Gymbag `v0.1.0`** (`protect off`, 純プロキシ) | **64,048** | 0.995 ms | 0.831 ms | 2.085 ms | 3.940 ms | 47.0 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 0`, Cookie 認証済) | **62,108** | 1.026 ms | 0.838 ms | 2.186 ms | 4.265 ms | ~52 MiB | 0 |
| **Caddy `v2.11.9`** (Go, keep-alive 調整済) | **39,207** | 1.628 ms | 1.303 ms | 3.701 ms | 6.183 ms | 70.7 MiB | 0 |

## エッジ WAF ブロック性能（`/.env` プローブ）

アップストリームにトラフィックを流さず Gymbag のエッジでプローブを拒否（`403 Forbidden`）した際の結果です。

| シナリオ | スループット (RPS) | 平均レイテンシ | p50 | p95 | p99 | 帯域幅 | エラー |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Gymbag エッジブロック (`/.env`, HTTPS / HTTP/1.1)** | **140,680** | 0.452 ms | 0.348 ms | 1.203 ms | 2.028 ms | 340.6 MiB/s | 0 |

## バイナリサイズとメモリ使用量

| サーバー | 実行ファイルサイズ | 外部ランタイム依存 | 定常時 RSS（負荷時） |
| :--- | ---: | :--- | ---: |
| **Gymbag `v0.1.0`** | **12.9 MB**（UI・アセット埋め込み済み静的バイナリ） | なし (`CGO_ENABLED=0`) | 47 – 54 MiB |
| **Caddy `v2.11.9`** | 49.0 MB | なし | 71 – 77 MiB |
| **Nginx `1.31.8`** | 1.5 MB | OpenSSL 4, PCRE2, libc | 5.8 MiB（master + workers） |

## まとめ

1. **Caddy との比較**: WAF を動かした状態で、Gymbag は Caddy より **HTTP/1.1 で約 32% 高速**（`72.5k` vs `54.7k` RPS）、**HTTP/2 で 63–69% 高速**（`64.0k–66.1k` vs `39.2k` RPS）で、メモリは約 30% 少なく、バイナリサイズも 3.8 倍コンパクトです。
2. **Nginx との比較**: スループット（`85.2k` / `88.8k` RPS）と RSS（`5.8 MiB`）では C 製の Nginx がリードしていますが、Gymbag は ACME・WAF・脅威フィードを単一バイナリで終端しながら **Nginx の HTTP/1.1 スループットの 85%** に達し、HTTP/1.1 の p99 レイテンシでは Nginx を下回っています（`3.14 ms` vs `6.20 ms`）。
3. **WAF オーバーヘッド**: `verify 3`（形式検証・デフォルトルール・シャード化レート制限・AI/不審トラフィック用 DFA マッチング）の負荷は、HTTP/1.1 では計測誤差の範囲内に収まっています（`72.5k` vs `72.5k` RPS）。
4. **エッジでの遮断**: `/.env` や `/wp-admin` へのプローブは、バックエンドへ触れる前に TLS 終端エッジで **140,680 req/s** でドロップします。

## WAF マイクロベンチマーク（`go test -bench`）

ローカル環境で WAF エンジンのベンチマークを実行するには次のようにします。

```bash
go test -bench=BenchmarkEngineDecideHTTP -benchmem ./internal/waf
```

Apple M4（`darwin/arm64`、Go `1.27.0`）での計測結果:

| ベンチマーク | 時間 / リクエスト | バイト / Op | アロケーション / Op |
| :--- | ---: | ---: | ---: |
| `ProtectOff`（形式チェック + ACME バイパス） | **129.1 ns/op** | 240 B/op | 1 alloc/op |
| `Verify3_BlockDotEnv`（デフォルトブロックルール一致） | **137.8 ns/op** | 288 B/op | 2 allocs/op |
| `Verify0_ClearedCookie`（キャッシュ済み HMAC クリアランス） | **899.8 ns/op** | 320 B/op | 3 allocs/op |
| `Verify3_BrowserPassThrough`（全検査 + DFA） | **1,539 ns/op** | 320 B/op | 3 allocs/op |

## ホットパスを小さく保つ工夫

- **シングルパス HTTPS ハンドラ**: `r.WithContext` による複製や中間マップの割り当てを行わず、リクエストごとに 1 パスでルート・クライアント IP・HSTS・WebSocket アップグレード状態を解決します。
- **ASCII Aho-Corasick DFA**: 組み込みの AI ボット署名、不審なクライアントトークン、クエリ攻撃パターン、カスタム `user_agent` ルールを大文字小文字を区別しない DFA にコンパイルします（`O(len(input))`、リクエストごとの小文字変換なし）。
- **完全一致マップと単純グロブ判定**: 完全一致パスや単純なグロブ（`prefix*`、`*suffix`、`*sub*`）では正規表現評価をスキップし、`%` や `+` がなければクエリのアンエスケープも行いません。
- **64 シャード化レート制限＆クリアランスキャッシュ**: レートカウンタと認証済み `(token, host, IP)` キャッシュ参照におけるコア間のロック競合を回避します。
- **事前パース済みアップストリームとバッファプール**: 設定読み込み時にアップストリーム URL をパースし、`32 KiB` のプロキシコピーバッファを `sync.Pool` で再利用します。トランスポートはホストあたり `1024` 本のアイドル接続をプールし（`DisableCompression: true`）、ストリーミングのフラッシュは SSE または `Content-Length` 不明時のみ行います。
- **事前レンダリング応答とアトミック SNI**: チャレンジ HTML テンプレート、拒否ページ、アセット ETag、SNI 証明書マップを `atomic.Pointer` で差し替えます。

## ベンチマークで使用したリファレンス設定

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
