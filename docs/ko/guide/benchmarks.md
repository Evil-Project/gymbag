# 벤치마크 및 성능

WAF, 기본 차단 규칙, AI/의심 탐지를 켠 상태와 순수 TLS 리버스 프록시로 동작할 때 Gymbag이 추가하는 오버헤드를 측정한 결과입니다.

## 테스트 환경 및 측정 방법

모든 테스트는 같은 머신과 동일 조건에서 **동일한 ECDSA P-256 인증서**로 TLS 1.3을 종단하고, `1 KiB`(`1024`바이트) 본문을 반환하는 **동일한 Go HTTP/1.1 업스트림 서버**로 프록시하며 측정했습니다.

| 항목 | 값 |
| :--- | :--- |
| **하드웨어** | Apple M4 (10코어 CPU, 16 GiB RAM) |
| **OS / 런타임** | macOS 26.6.1 (`darwin/arm64`), Go `1.27.0` |
| **테스트 대상** | **Gymbag** `v0.1.0`, **Caddy** `v2.11.9`, **Nginx** `1.31.8` |
| **동시성** | 동시 워커 `64`개 (HTTP/2는 다중화된 TCP 연결 `8`개) |
| **측정 구간** | 시나리오별 `2s` 워밍업 + `8s` 측정 (한 번에 하나의 서버만 실행) |
| **업스트림 튜닝** | 세 서버 모두 유휴 업스트림 keep-alive 연결 `1024`개 설정 및 액세스 로그 비활성화 (Nginx `access_log off`, Caddy `level ERROR`) |
| **직접 업스트림 기준** | `143,761 req/s` (평문 HTTP/1.1, p50 `0.342 ms`, p99 `1.817 ms`, 오류 `0`) |

## HTTPS 리버스 프록시 비교 (HTTP/1.1 Keep-Alive)

표준 브라우저 헤더(`User-Agent`, `Accept`, `Accept-Language`, `Sec-Fetch-Mode`)를 보내는 64개 동시 지속 TLS 1.3 연결 기준입니다.

| 서버 및 모드 | 처리량 (RPS) | 평균 지연 | p50 | p95 | p99 | RSS 메모리 | 오류 |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **85,203** | 0.748 ms | 0.449 ms | 2.428 ms | 6.201 ms | 5.8 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 3`, 전체 WAF 활성) | **72,517** | 0.880 ms | 0.743 ms | 1.936 ms | **3.141 ms** | 54.3 MiB | 0 |
| **Gymbag `v0.1.0`** (`protect off`, 순수 프록시) | **72,479** | 0.880 ms | 0.744 ms | 1.909 ms | **3.154 ms** | 52.1 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 0`, 쿠키 통과) | **65,490** | 0.974 ms | 0.791 ms | 2.251 ms | 3.933 ms | ~54 MiB | 0 |
| **Caddy `v2.11.9`** (Go, keep-alive 튜닝) | **54,741** | 1.167 ms | 0.898 ms | 3.062 ms | 4.901 ms | 76.8 MiB | 0 |

## HTTPS 리버스 프록시 비교 (HTTP/2)

8개의 HTTP/2 TLS 1.3 연결에 64개 워커를 다중화해 측정했습니다.

| 서버 및 모드 | 처리량 (RPS) | 평균 지연 | p50 | p95 | p99 | RSS 메모리 | 오류 |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **88,775** | 0.717 ms | 0.649 ms | 1.419 ms | 1.960 ms | 5.8 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 3`, 전체 WAF 활성) | **66,096** | 0.964 ms | 0.821 ms | 1.969 ms | 3.476 ms | 52.5 MiB | 0 |
| **Gymbag `v0.1.0`** (`protect off`, 순수 프록시) | **64,048** | 0.995 ms | 0.831 ms | 2.085 ms | 3.940 ms | 47.0 MiB | 0 |
| **Gymbag `v0.1.0`** (`verify 0`, 쿠키 통과) | **62,108** | 1.026 ms | 0.838 ms | 2.186 ms | 4.265 ms | ~52 MiB | 0 |
| **Caddy `v2.11.9`** (Go, keep-alive 튜닝) | **39,207** | 1.628 ms | 1.303 ms | 3.701 ms | 6.183 ms | 70.7 MiB | 0 |

## 엣지 WAF 차단 (`/.env` 프로브)

업스트림 트래픽 없이 Gymbag 엣지에서 프로브를 즉시 차단(`403 Forbidden`)할 때의 측정치입니다.

| 시나리오 | 처리량 (RPS) | 평균 지연 | p50 | p95 | p99 | 대역폭 | 오류 |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Gymbag 엣지 차단 (`/.env`, HTTPS / HTTP/1.1)** | **140,680** | 0.452 ms | 0.348 ms | 1.203 ms | 2.028 ms | 340.6 MiB/s | 0 |

## 바이너리 크기 및 메모리 사용량

| 서버 | 실행 파일 크기 | 외부 런타임 의존성 | 정상 상태 RSS (부하 시) |
| :--- | ---: | :--- | ---: |
| **Gymbag `v0.1.0`** | **12.9 MB** (정적, 내장 UI 및 에셋 포함) | 없음 (`CGO_ENABLED=0`) | 47 – 54 MiB |
| **Caddy `v2.11.9`** | 49.0 MB | 없음 | 71 – 77 MiB |
| **Nginx `1.31.8`** | 1.5 MB | OpenSSL 4, PCRE2, libc | 5.8 MiB (master + workers) |

## 요약

1. **Caddy 대비**: Gymbag은 WAF를 실행하면서도 **HTTP/1.1에서 약 32%(`72.5k` vs `54.7k` RPS)**, **HTTP/2에서 63–69%(`64.0k–66.1k` vs `39.2k` RPS)** 더 빠르며 메모리는 약 30% 적게, 바이너리는 3.8배 작게 유지합니다.
2. **Nginx 대비**: C로 작성된 Nginx가 처리량(`85.2k` / `88.8k` RPS)과 작은 RSS(`5.8 MiB`)에서는 앞섭니다. Gymbag은 **Nginx HTTP/1.1 처리량의 85%**를 내면서 HTTP/1.1 p99 지연(`3.14 ms` vs `6.20 ms`)은 더 낮고, 동일 바이너리 안에서 ACME, WAF, 위협 피드를 함께 처리합니다.
3. **WAF 오버헤드**: `verify 3`(형태 검사, 기본 규칙, 샤드 속도 제한, AI/의심 DFA 매칭)은 HTTP/1.1 기준 `protect off`(`72.5k` vs `72.5k` RPS)와 실행 간 오차 범위 안에 있습니다.
4. **엣지 차단**: `/.env`, `/wp-admin` 같은 프로브는 백엔드에 닿기 전에 TLS 엣지에서 **140,680 req/s**로 거절됩니다.

## WAF 마이크로벤치마크 (`go test -bench`)

로컬에서 WAF 엔진 벤치마크를 실행하려면 다음 명령을 씁니다.

```bash
go test -bench=BenchmarkEngineDecideHTTP -benchmem ./internal/waf
```

Apple M4(`darwin/arm64`, Go `1.27.0`) 측정 결과입니다.

| 벤치마크 | 시간 / 요청 | 바이트 / Op | 할당 횟수 / Op |
| :--- | ---: | ---: | ---: |
| `ProtectOff` (형태 검사 + ACME 우회) | **129.1 ns/op** | 240 B/op | 1 alloc/op |
| `Verify3_BlockDotEnv` (기본 차단 규칙 일치) | **137.8 ns/op** | 288 B/op | 2 allocs/op |
| `Verify0_ClearedCookie` (캐시된 HMAC 클리어런스) | **899.8 ns/op** | 320 B/op | 3 allocs/op |
| `Verify3_BrowserPassThrough` (전체 검사 + DFA) | **1,539 ns/op** | 320 B/op | 3 allocs/op |

## 핫 패스를 가볍게 유지하는 구조

- **단일 패스 HTTPS 핸들러**: 요청당 라우트, 클라이언트 IP, HSTS, WebSocket 업그레이드 상태를 한 번만 해석해 `r.WithContext` 복제나 중간 맵 할당을 만들지 않습니다.
- **ASCII Aho-Corasick DFA**: 내장 AI 봇 시그니처, 의심 클라이언트 토큰, 쿼리 공격 패턴, 사용자 지정 `user_agent` 규칙을 대소문자 무관 DFA(`O(len(input))`)로 컴파일해 요청마다 소문자로 변환하지 않습니다.
- **정확 일치 맵 & 단순 글롭 매칭**: 정확한 경로와 단순 글롭(`prefix*`, `*suffix`, `*sub*`)은 정규식 엔진을 건너뛰며, `%`나 `+`가 없으면 쿼리 언이스케이프도 건너뜁니다.
- **64-샤드 속도 제한기 & 클리어런스 캐시**: 요청 카운터와 인증된 `(token, host, IP)` 캐시 조회 시 코어 간 락 경합을 피합니다.
- **사전 파싱된 업스트림 & 버퍼 풀**: 설정 로드 시 업스트림을 미리 파싱하고 `32 KiB` 프록시 복사 버퍼를 `sync.Pool`에서 재사용합니다. 트랜스포트는 호스트당 `1024`개 유휴 연결(`DisableCompression: true`)을 풀링하며, 스트리밍 플러시는 SSE나 `Content-Length`를 알 수 없는 경우에만 켭니다.
- **사전 렌더링 응답 & 원자적 SNI**: 챌린지 HTML 템플릿, 거부 페이지, 에셋 ETag, SNI 인증서 맵은 `atomic.Pointer`로 교체합니다.

## 이 벤치마크에 사용한 참조 설정

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
