# `Gymbagfile` & JSON 스키마 레퍼런스

Gymbag은 기본적으로 작업 디렉터리에서 `./gymbag.json`, `./Gymbagfile`, `./Caddyfile`을 차례로 찾습니다(또는 `--config <path>` / `GYMBAG_CONFIG`). `gymbag.json`, `Gymbagfile` / `Caddyfile v2`, `nginx.conf`, Apache (`httpd.conf` / `apache2.conf`)를 그대로 파싱합니다.

## 전역 옵션 블록 `{ ... }`

선택적인 최상위 `{ ... }` 블록에서 서버 전체 TLS, ACME, 네트워크, 위협/VPN/AI 피드, 원격 규칙, 인증 포털 설정, 기본 WAF 정책을 지정합니다.

```caddyfile
{
    email ops@example.com
    ssl_provider letsencrypt
    # eab YOUR_EAB_KID YOUR_EAB_HMAC_KEY
    # account_key certs/acme-account.pem
    # ssl_key certs/domain.key

    bind 0.0.0.0
    http_port 80
    https_port 443
    admin 127.0.0.1:2019
    cert_dir certs
    auto_hosts on
    hsts on

    # 기본 클리어런스 쿠키/IP 타임아웃 (초 또는 기간: 30m, 12h, 7d)
    verify_timeout 24h

    # SSH & Minecraft 공용 HTTPS 인증 포털
    # verify_url https://verify.example.com:8443
    # verify_port 8443

    # 공용 PoW 논스, 속도 제한, SSH/MC 게이트 티켓용 선택적 Redis 저장소
    # redis redis://127.0.0.1:6379/0

    # IP 위협 인텔, VPN/Tor 탐지 & AI 크롤러 IP 피드
    ip_security on
    ip_security_action verify
    ip_reputation on
    vpn_detection on
    ai_ip_list on
    intel_refresh 6h

    # 공용 원격 방화벽 규칙 (Gymbagfile 문법 또는 JSON)
    # remote_rules https://rules.example.com/baseline.rules
    # remote_rules_refresh 5m

    # 원격 및 사이트 규칙보다 먼저 평가되는 전역 사용자 지정 규칙
    block ip 198.51.100.99 192.0.2.0/24
}
```

### 전역 지시어

| 지시어 | 설명 | 기본값 |
| :--- | :--- | :--- |
| `email` / `acme_email` | ACME 등록 연락처 이메일 (및 ZeroSSL 자동 EAB) | — |
| `ssl_provider` / `ca` | `letsencrypt`, `letsencrypt_staging`, `zerossl`, `google`, `ssl.com`, `litessl`, `buypass`, `local`, 또는 URL | `auto` (`letsencrypt` / `local`) |
| `acme_directory` | 명시적 RFC 8555 ACME v2 디렉터리 URL | Let's Encrypt v2 |
| `eab <kid> <hmac>` | External Account Binding 자격 증명 (`eab_kid` / `eab_hmac_key`도 지원) | — |
| `account_key` | 기존 ECDSA 또는 RSA ACME 계정 PEM 경로 | `<cert_dir>/acme-account.pem` |
| `ssl_key` | ACME CSR용 기존 개인 키 경로 | 자동 생성 |
| `tls` | `auto`, `acme`, `local`, `selfsigned`, 또는 `<cert.pem> <key.pem>` | `auto` |
| `key_password` | 암호화된 PEM 개인 키 암호 | — |
| `cert_dir` | 인증서, 로컬 CA, 시크릿, 인텔 캐시, 규칙 캐시 디렉터리 | `certs` |
| `renew_interval` | 인증서 갱신 확인 주기 (초 또는 `12h` 같은 기간 문자열) | `43200` (`12h`) |
| `renew_within_days` | 만료 N일 이내에 인증서 갱신 | `30` |
| `auto_hosts` | `localhost` / `*.internal`용 `/etc/hosts` 항목 관리 (`on`/`off`) | `on` |
| `hsts` | `Strict-Transport-Security` 헤더 전송 (`on`/`off`) | 공인 ACME 인증서 시 `on` |
| `bind` / `bind_host` | 수신 바인드 IP 주소 | `127.0.0.1` (로컬 HTTP) / `0.0.0.0` (공인 HTTP, SSH, MC) |
| `http_port` / `https_port` | HTTP 리다이렉트/ACME 포트 및 HTTPS 게이트웨이 포트 | `80` / `443` |
| `verify_url` | SSH/Minecraft 인증 포털의 공인 HTTPS URL | 서버 IP + IP SSL 인증서 |
| `verify_port` | SSH/Minecraft 인증 포털 공용 HTTPS 포트 | `8443` |
| `verify_server { ... }` | `url`, `port`, `tls <cert> <key>`를 지원하는 선택적 블록 | — |
| `admin` | 로컬 Admin REST API 수신 주소 (예: `127.0.0.1:2019` 또는 `off`) | 비활성화 |
| `redis` / `redis_url` | Redis 연결 URL (`redis://` 또는 `rediss://`) | 인메모리 스토어 |
| `secret` | `waf_clearance` 쿠키용 HMAC 시크릿 | `<cert_dir>/secret` |
| `trusted_proxies` | `X-Forwarded-For`를 신뢰할 다운스트림 프록시 IP/CIDR | — |
| `ip_security` | 위협 인텔 CIDR 피드 활성화 (`on`/`off`) | `on` |
| `ip_security_action` | 위협 인텔 일치 시 동작 (`verify` 또는 `block`) | `verify` |
| `ip_reputation` | 실시간 Shodan InternetDB & StopForumSpam 조회 활성화 (`vpn_detection` 활성 시 실시간 VPN API 조회 포함) | `on` |
| `vpn_detection` / `vpn_detect` | 검증 레벨 `1` 및 `3`에서 무료 VPN/프록시/Tor IP를 의심 대상으로 표시 (`on`/`off`) | `on` |
| `ai_ip_list` | 공식 AI 크롤러 CIDR 목록 활성화 (`on`/`off`) | `on` |
| `intel_refresh` | 피드 갱신 주기 (초 또는 `6h` 같은 기간 문자열) | `21600` (`6h`) |
| `threat_feed` / `ai_ip_feed` / `vpn_feed` | 추가 사용자 지정 IP 피드 URL (`threat_feeds`, `ai_ip_feeds`, `vpn_feeds`도 지원) | — |
| `remote_rules` | 모든 사이트가 상속하는 HTTP/HTTPS 원격 방화벽 규칙 URL | — |
| `remote_rules_refresh` | 원격 규칙 URL 새로고침 주기 (초 또는 기간; 별칭 `rules_refresh`) | `300` (`5m`) |
| `rules_file` | 로컬 `.rules`/`.json` 규칙 파일 (`rules_file <path> [dynamic\|static]`) | — |
| `rules_dir` | 로컬 규칙 디렉터리 (`rules_dir <path> [dynamic\|static]`) | — |
| `ruleset <name> { ... }` | 이름 지정 로컬 또는 원격 규칙 소스 블록 (`source`/`file`/`dir`/`url`, `dynamic on\|off`) | — |
| `rules_watch_interval` | `dynamic` 로컬 규칙 파일 및 폴더 폴링 주기 | `2` (`2s`) |
| `verify_timeout` / `clearance_ttl` | 기본 클리어런스 유지 시간 (초 또는 `30m`, `12h`, `7d` 같은 기간 문자열) | `86400` (`24h`) |

## 사이트 블록 지시어

```caddyfile
app.example.com, www.example.com {
    reverse_proxy http://localhost:3000 http://localhost:3001
    verify 3
    difficulty 4
    rate_limit 240 60s
    challenge_ttl 5m
    verify_timeout 12h
    cookie_name waf_clearance
    cookie_secure true
    default_rules on
    remote_rules https://rules.example.com/app.rules

    allow ip 10.0.0.0/8
    block path /internal-only*
}
```

| 지시어 | 설명 |
| :--- | :--- |
| `reverse_proxy <urls...>` | 하나 이상의 업스트림 대상 (라운드 로빈 부하 분산; `:3000` 단축 표기 지원) |
| `verify [0\|1\|2\|3\|off]` | 검증 레벨 (단독 `verify`는 레벨 `3`) |
| `protect <on\|off>` | WAF 검증 활성화/비활성화 |
| `difficulty <1-5>` | SHA-256 선행 16진수 `0` PoW 난이도 |
| `rate_limit <count> [window]` | IP별 속도 제한 및 선택적 윈도우 (`rate_window`도 허용) |
| `challenge_ttl <sec\|dur>` | PoW 챌린지 논스 만료 시간 (예: `300` 또는 `5m`) |
| `clearance_ttl <sec\|dur>` | 인증 유지 시간 (별칭: `verify_timeout`, `verified_timeout`, `clearance_timeout`) |
| `cookie_name <name>` | 클리어런스 쿠키 이름 |
| `cookie_secure <bool>` | 클리어런스 쿠키의 `Secure` 속성 설정 여부 |
| `default_rules <on\|off>` | 내장 기본 규칙 토글 |
| `remote_rules <urls...>` | 원격 방화벽 규칙 URL (Gymbagfile 문법 또는 JSON) |
| `rules_file <path> [dynamic\|static]` | 로컬 `.rules` 또는 `.json` 파일 (선택적 핫 리로드) |
| `rules_dir <path> [dynamic\|static]` | `.rules`/`.json` 파일 디렉터리 (선택적 핫 리로드) |
| `ruleset <name> { ... }` | 이름 지정 규칙 소스 블록 (`source`/`file`/`dir`/`url`, `dynamic on\|off`) |
| `handle` / `handle_path` / `route` | 순서 기반 경로 매칭 라우트 블록 (`reverse_proxy`, `root` + `file_server`, `respond`, `redir`, `header`) |
| `root` / `file_server` / `try_files` | 정적 파일 제공, 디렉터리 목록(`browse`), SPA 폴백 |
| `respond` / `redir` / `header` | 정적 응답, 리다이렉트(`{scheme}`, `{host}`, `{uri}`, `{path}`, `{query}`), 헤더 수정 |
| `ai_bots <patterns...>` | AI 크롤러로 취급할 추가 User-Agent 부분 문자열 |
| `suspicious_uas <patterns...>` | 의심 대상으로 취급할 추가 User-Agent 부분 문자열 |
| `suspicious_paths <prefixes...>` | 의심 프로브로 취급할 추가 경로 접두사 |
| `tls <cert> <key>` | 사이트별 SNI 인증서 및 개인 키 경로 |
| `allow \| verify \| block ...` | 한 줄 단축 사용자 지정 방화벽 규칙 |
| `rule "name" { ... }` | 다중 조건 사용자 지정 방화벽 규칙 블록 |

## `ssh` 및 `minecraft` 서비스 블록

최상위 `ssh [port] { ... }` 및 `minecraft [port] { ... }`(별칭 `mc`) 블록으로 TCP PoW 게이트를 설정합니다.

```caddyfile
ssh {
    port 22
    upstream 127.0.0.1:41822
    verify 3
    difficulty 4
    rate_limit 60 60s
    clearance_ttl 24h
    allow ip 10.0.0.0/8
}

minecraft {
    port 25565
    upstream 127.0.0.1:48291
    server_jar /srv/mc/paper.jar
    jvm_args -Xms1G -Xmx4G
    server_properties /srv/mc/server.properties
    verify 3
    rate_limit 60 60s
}
```

| 지시어 | 설명 | 기본값 |
| :--- | :--- | :--- |
| `enabled <bool>` | 블록을 지우지 않고 서비스 켜기/끄기 | `true` |
| `port <port>` | 공개 수신 포트 (`ssh 2222 { ... }` 형태로도 작성 가능) | `22` (SSH) / `25565` (Minecraft) |
| `upstream <host:port>` | 백엔드 TCP 대상 (`reverse_proxy` / `proxy`도 허용, 예: `:2222`, `tcp://127.0.0.1:25566`) | — (`server_jar` 설정 시 `127.0.0.1:25566`) |
| `server_properties <path>` | `server.properties` 경로 (`minecraft` 블록 전용) | — |
| `server_jar <path>` / `jar` | Gymbag과 함께 자동 시작할 Minecraft `.jar` 경로 (`minecraft` 블록 전용) | — |
| `java_path <bin>` / `java` | 명시적 Java 바이너리 재정의 (`minecraft` 블록 전용) | 자동 선택 |
| `jvm_args <flags...>` | `-jar` 앞에 전달할 JVM 인수 (`minecraft` 블록 전용) | — |
| `server_args <args...>` | JAR 경로 뒤에 전달할 추가 인수 (`minecraft` 블록 전용) | — |
| `work_dir <dir>` | 관리형 Minecraft 프로세스의 작업 디렉터리 (`minecraft` 블록 전용) | JAR 디렉터리 |
| `auto_restart <on\|off>` | Minecraft 서버가 예기치 않게 종료될 때 자동 재시작 (`minecraft` 블록 전용) | `on` |
| `verify [0\|1\|2\|3\|off]` | 검증 레벨 (단독 `verify`는 `3`) | `3` |
| `protect <on\|off>` | 이 서비스의 PoW 검증 토글 | `on` |
| `difficulty <1-5>` | PoW 선행 16진수 `0` 난이도 | `4` |
| `rate_limit <count> [window]` | IP별 연결 속도 제한 및 윈도우 | `60` / `60s` |
| `challenge_ttl <sec\|dur>` | 챌린지 토큰 만료 시간 | `300` (`5m`) |
| `clearance_ttl <sec\|dur>` | 인증된 IP 유지 시간 (`verify_timeout`도 허용) | `86400` (`24h`) |
| `verify_url` / `verify_port` | 연결 해제 메시지에 출력할 인증 링크의 서비스별 재정의 | 전역 `verify_url` / `verify_port` |
| `allow \| verify \| block ...` / `rule` | IP/CIDR 또는 클라이언트 배너/사용자 이름(`user_agent`)에 일치하는 사용자 지정 규칙 | — |

## 기간 구문

시간 지시어(`clearance_ttl` / `verify_timeout`, `challenge_ttl`, `rate_window`, `intel_refresh`, `remote_rules_refresh`, `rules_watch_interval`, `renew_interval`)는 다음 형식을 지원합니다.
- 정수 초: `300`, `3600`, `86400`
- Go 기간 문자열: `45s`, `30m`, `12h`, `1h30m`
- 일 및 주 단축 표기: `1d`, `7d`, `1d12h`, `2w`

## 포맷팅 및 검증

```bash
./gymbag validate --config Gymbagfile
./gymbag fmt --overwrite Gymbagfile
```

## JSON 설정 (`gymbag.json`)

`gymbag init`은 기본적으로 사용하지 않는 서비스와 기본값을 제외한 간결한 `gymbag.json`을 생성합니다(`--format gymbagfile`은 Caddy 문법으로 출력).

```bash
./gymbag init -d app.example.com -u :3000 --verify-timeout 12h --rules-dir ./rules.d --rules-dynamic
```

최상위 JSON 키(`DisallowUnknownFields` 적용):
- `sites`: `{domain, upstreams, routes, headers, protect, waf, cert_file, key_file}` 배열. 여기서 `waf`는 `{verify_level, difficulty, challenge_ttl, clearance_ttl, cookie_name, cookie_secure, rate_limit, rate_window, default_rules, rules, remote_rules, rule_sources, custom_ai_bots, custom_suspicious_uas, custom_suspicious_paths}`를 담습니다.
- `ssh`: `{enabled, port, upstream, protect, verify_level, difficulty, challenge_ttl, clearance_ttl, rate_limit, rate_window, verify_url, verify_port, rules}`.
- `minecraft`: `{enabled, port, upstream, server_properties, server_jar, java_path, jvm_args, server_args, work_dir, auto_restart, protect, verify_level, difficulty, challenge_ttl, clearance_ttl, rate_limit, rate_window, verify_url, verify_port, rules}`.
- `verify`: `{url, port, cert_file, key_file}`.
- `bind_host`, `http_port`, `https_port`, `admin_addr`, `remote_rules_refresh`, `rules_watch_interval`, `redis_url`, `secret`, `trusted_proxies`.
- `tls`: `{mode, provider, cert_file, key_file, key_password, account_key_file, eab_kid, eab_hmac_key, auto_hosts, cert_dir, acme_email, acme_directory, renew_interval, renew_within_days, hsts}`.
- `intel`: `{ip_security, ip_security_action, live_reputation, ai_ip_list, vpn_detection, refresh_interval, threat_feeds, ai_ip_feeds, vpn_feeds}`.
- `defaults`: 공용 기본 `waf` 객체.

`nginx.conf`, Apache `httpd.conf`/`apache2.conf`, `Caddyfile`을 `gymbag.json`으로 변환할 수도 있습니다.

```bash
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```
