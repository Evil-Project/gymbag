# `Gymbagfile` & JSON 스키마 레퍼런스

Gymbag은 기본적으로 작업 디렉터리의 `./Gymbagfile` 또는 `./Caddyfile`(또는 `--config <path>` / `GYMBAG_CONFIG`)을 읽으며 `.json` 형식도 지원합니다.

## 전역 옵션 블록 `{ ... }`

상단 `{ ... }` 블록에서 서버 전체 TLS, ACME, 네트워크, 위협 인텔리전스, 원격 규칙 및 기본 WAF 정책을 설정합니다:

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

    # 기본 인증 완료 타임아웃 (초 또는 기간: 30m, 12h, 7d)
    verify_timeout 24h

    # PoW 논스 및 속도 제한 공유용 Redis (선택)
    # redis redis://127.0.0.1:6379/0

    # IP 위협 인텔리전스 & AI 크롤러 IP 피드
    ip_security on
    ip_security_action verify
    ip_reputation on
    ai_ip_list on
    intel_refresh 6h

    # 공용 원격 방화벽 규칙 (Gymbagfile 구문 또는 JSON)
    # remote_rules https://rules.example.com/baseline.rules
    # remote_rules_refresh 5m

    # 모든 사이트에 상속되는 전역 사용자 지정 규칙
    block ip 198.51.100.99 192.0.2.0/24
}
```

### 전역 지시어

| 지시어 | 설명 | 기본값 |
| :--- | :--- | :--- |
| `email` / `acme_email` | ACME 등록 이메일 (ZeroSSL 자동 EAB 지원) | — |
| `ssl_provider` / `ca` | `letsencrypt`, `letsencrypt_staging`, `zerossl`, `google`, `ssl.com`, `litessl`, `buypass`, `local`, 또는 URL | `auto` (`letsencrypt` / `local`) |
| `acme_directory` | 명시적 RFC 8555 ACME v2 디렉터리 URL | Let's Encrypt v2 |
| `eab <kid> <hmac>` | EAB 자격 증명 (`eab_kid` / `eab_hmac_key`도 가능) | — |
| `account_key` | 기존 ECDSA 또는 RSA ACME 계정 PEM 경로 | `<cert_dir>/acme-account.pem` |
| `ssl_key` | ACME CSR용 기존 개인 키 경로 | 자동 생성 |
| `tls` | `auto`, `acme`, `local`, `selfsigned`, 또는 `<cert.pem> <key.pem>` | `auto` |
| `key_password` | 암호화된 PEM 개인 키 암호 | — |
| `cert_dir` | 인증서, 로컬 CA, 시크릿, 인텔 및 원격 규칙 캐시 디렉터리 | `certs` |
| `renew_interval` | 인증서 갱신 확인 주기 (초 또는 `12h` 등) | `43200` (`12h`) |
| `renew_within_days` | 만료 N일 전 갱신 시작 | `30` |
| `auto_hosts` | `localhost` / `*.internal`용 `/etc/hosts` 관리 (`on`/`off`) | `on` |
| `hsts` | `Strict-Transport-Security` 헤더 전송 (`on`/`off`) | 공인 ACME 인증서 시 `on` |
| `bind` / `bind_host` | 수신 바인딩 IP 주소 | 로컬 `127.0.0.1` / 공인 `0.0.0.0` |
| `http_port` / `https_port` | HTTP 리디렉션/ACME 포트 및 HTTPS 게이트웨이 포트 | `80` / `443` |
| `admin` | 로컬 Admin REST API 수신 주소 (예: `127.0.0.1:2019` 또는 `off`) | 비활성 |
| `redis` / `redis_url` | Redis 연결 URL (`redis://[:password@]host:port/db`) | 인메모리 저장소 |
| `secret` | `waf_clearance` 쿠키용 HMAC 시크릿 | `<cert_dir>/secret` |
| `trusted_proxies` | `X-Forwarded-For`를 신뢰할 프록시 IP/CIDR | — |
| `ip_security` | 위협 인텔 CIDR 피드 활성화 (`on`/`off`) | `on` |
| `ip_security_action` | 위협 일치 시 동작 (`verify` 또는 `block`) | `verify` |
| `ip_reputation` | Shodan InternetDB & StopForumSpam 실시간 조회 활성화 | `on` |
| `ai_ip_list` | 공식 AI 크롤러 CIDR 목록 활성화 | `on` |
| `intel_refresh` | 피드 갱신 주기 (초 또는 `6h` 등) | `21600` (`6h`) |
| `threat_feed` / `ai_ip_feed` | 추가 사용자 지정 IP 피드 URL | — |
| `remote_rules` | 모든 사이트가 상속하는 원격 방화벽 규칙 URL | — |
| `remote_rules_refresh` | 원격 규칙 갱신 주기 (초 또는 `5m`, `1h`; 별칭 `rules_refresh`) | `300` (`5m`) |
| `verify_timeout` / `clearance_ttl` | 기본 인증 완료 쿠키 타임아웃 (초 또는 `30m`, `12h`, `7d`) | `86400` (`24h`) |

---

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
| `reverse_proxy <urls...>` | 하나 이상의 업스트림 대상 (라운드 로빈, `:3000` 단축 표기 지원) |
| `verify [0\|1\|2\|3\|off]` | 검증 레벨 (단독 `verify`는 레벨 `3`) |
| `protect <on\|off>` | 사이트 WAF 검증 활성화/비활성화 |
| `difficulty <1-5>` | SHA-256 PoW 선행 16진수 0 난이도 |
| `rate_limit <count> [window]` | IP별 속도 제한 및 창 길이(초 또는 기간; `rate_window`도 지원) |
| `challenge_ttl <sec\|dur>` | PoW 챌린지 논스 만료 시간 (`300` 또는 `5m` 등) |
| `clearance_ttl <sec\|dur>` | 인증 완료 쿠키 유효 시간 (`86400`, `30m`, `12h`, `7d`; 별칭 `verify_timeout`, `verified_timeout`, `clearance_timeout`) |
| `cookie_name <name>` | 클리어런스 쿠키 이름 |
| `cookie_secure <bool>` | `Secure` 플래그 설정 여부 |
| `default_rules <on\|off>` | 내장 기본 규칙 토글 |
| `remote_rules <urls...>` | 원격 방화벽 규칙 URL (별칭 `remote_rule`, `rules_url`, `rule_url`, `remote_rules_url`) |
| `ai_bots <patterns...>` | AI 크롤러로 취급할 추가 User-Agent 패턴 |
| `suspicious_uas <patterns...>` | 의심 클라이언트로 취급할 추가 User-Agent 패턴 |
| `suspicious_paths <prefixes...>` | 의심 프로브로 취급할 추가 경로 접두사 |
| `tls <cert> <key>` | 사이트별 SNI 인증서 및 개인 키 경로 |
| `allow \| verify \| block ...` | 한 줄 단축 방화벽 규칙 |
| `rule "name" { ... }` | 복합 조건 방화벽 규칙 블록 |

---

## 기간 구문

시간 지시어(`clearance_ttl` / `verify_timeout`, `challenge_ttl`, `rate_window`, `intel_refresh`, `remote_rules_refresh`, `renew_interval`)는 다음 형식을 지원합니다:
- **정수 초**: `300`, `3600`, `86400`
- **Go 기간**: `45s`, `30m`, `12h`, `1h30m`
- **일/주 단위**: `1d`, `7d`, `1d12h`, `2w`

---

## 서식 지정 및 검증

```bash
# 구문 검증 및 사이트별 요약(clearance_ttl 및 원격 규칙 수 포함) 출력
./gymbag validate --config Gymbagfile

# 표준 4칸 들여쓰기로 덮어쓰기 정렬
./gymbag fmt --overwrite Gymbagfile
```

## JSON 구성

```bash
./gymbag init --format json -d app.example.com -u :3000 --verify-timeout 12h
```

모든 필드는 JSON 스키마(`sites`, `bind_host`, `http_port`, `https_port`, `admin_addr`, `tls`, `intel`, `remote_rules_refresh`, `redis_url`, `secret`, `trusted_proxies`, `defaults`)에 1:1 대응하며 사이트별 `waf.clearance_ttl`(초) 및 `waf.remote_rules`(`[]string`)를 포함합니다.
