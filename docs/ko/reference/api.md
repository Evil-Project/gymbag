# REST API & Redis

## 공개 WAF 엔드포인트

보호되는 웹사이트에서 자동으로 제공됩니다.

| 메서드 | 경로 | 설명 |
| :--- | :--- | :--- |
| `GET` | `/.waf/v1/session` | `site`, `protected`, `verify_level` (`0`–`3`), `verify_mode`, `clearance_ttl`, `cleared`, 그리고 인증된 경우 `expires_at` / `expires_in` 반환 |
| `DELETE` | `/.waf/v1/session` | 방문자의 클리어런스 쿠키 제거 (`204 No Content`) |
| `POST` | `/.waf/v1/challenges` | 일회용 SHA-256 PoW 챌린지 발급 (`201 Created`) |
| `POST` | `/.waf/v1/verifications` | PoW 풀이 검증, 서명된 클리어런스 쿠키 발급 및 `{"ok": true, "expires_in": <ttl>}` 반환 (`/.waf/verify` 별칭 지원) |
| `GET` | `/.waf/assets/{name}` | 내장 챌린지 PNG 에셋 제공 (`/.waf/v1/assets/{name}` 별칭, `ETag` 캐싱) |

## 인증 포털 엔드포인트 (SSH & Minecraft)

`verify_port`(기본 `8443`)에서 제공되며, `verify_port`가 `https_port`와 같고 HTTP 사이트가 활성화된 경우에는 `https_port`(`443`)에서 함께 제공됩니다.

| 메서드 | 경로 | 설명 |
| :--- | :--- | :--- |
| `GET` | `/v/{token}` | SSH 또는 Minecraft 게이트 티켓용 브라우저 PoW 페이지 제공 (8자리 토큰 대소문자 미구분 매칭; `/verify/{token}` 별칭 지원) |
| `POST` | `/.waf/gate/verify` | `{token, challenge_id, nonce, verified}`를 검증하고 IP 인증 기록 저장 (`{"ok": true, "service": ..., "client_ip": ..., "expires_in": <ttl>}`) |
| `GET` | `/.waf/assets/{name}` | 내장 마스코트 PNG 제공 (`/.waf/v1/assets/{name}` 별칭 지원) |
| `GET` | `/.well-known/acme-challenge/{token}` | 포털 도메인 또는 RFC 8738 IP 인증서용 ACME HTTP-01 챌린지 응답 |

## 로컬 Admin REST API

`admin 127.0.0.1:2019`, `--admin 127.0.0.1:2019`, 또는 `GYMBAG_ADMIN=127.0.0.1:2019`로 활성화합니다.

::: warning 반드시 루프백 또는 사설 인터페이스에만 바인딩하세요
Admin API는 인증 토큰이 없으며 Gymbag 재시작 없이 실시간 라우팅과 방화벽 상태를 변경합니다. 항상 `127.0.0.1:2019`나 사설 관리 인터페이스에 바인딩하세요.
:::

| 메서드 | 경로 | 설명 |
| :--- | :--- | :--- |
| `GET` | `/api/v1/health` | 상태, `sites_count`, `ssh_enabled`, `minecraft_enabled`, `tls_source`, `threat_entries`, `ai_entries`, `vpn_entries`, `remote_rules` 수, 그리고 `server_jar` 설정 시 `minecraft_server` 상태 (`running`, `pid`, `java_major`, `required_java`, `mc_version`, `restarts`, `eula_blocked`, `last_error`) |
| `GET` | `/api/v1/tls` | 활성 TLS 소스, 인증서 경로, SSL 공급자, HSTS 상태, 지원 CA 카탈로그 |
| `POST` | `/api/v1/tls/renew` | 즉시 인증서 갱신 및 원자적 리로드 강제 실행 |
| `GET` | `/api/v1/intel` | 위협·AI 크롤러·VPN/Tor 피드 수 (`vpn_detection_enabled`, `vpn_entries`, `vpn_cache_size`, `reputation_cache_size`), 갱신 타임스탬프, 피드별 상태 (`threat`, `ai`, `vpn`) |
| `POST` | `/api/v1/intel/refresh` | 모든 위협, VPN, AI 피드 즉시 새로고침 |
| `GET` | `/api/v1/rules/remote` | 원격 규칙 URL, 로드된 규칙 수, 캐시 상태, 마지막 오류 |
| `POST` | `/api/v1/rules/remote/refresh` | 원격 규칙 URL 가져오기 및 WAF 엔진 원자적 리로드 |
| `GET` | `/api/v1/config` | 활성 런타임 설정 출력 (`secret`, `key_password`, `eab_hmac_key` 마스킹) |
| `PUT` | `/api/v1/config` | 활성 런타임 설정 검증 후 교체 |
| `GET` | `/api/v1/sites` | 설정된 웹사이트 및 WAF 정책 나열 |
| `POST` | `/api/v1/sites` | 런타임에 웹사이트 추가 (`201 Created`) |
| `GET` | `/api/v1/sites/{domain}` | 단일 웹사이트 설정 조회 |
| `PUT` | `/api/v1/sites/{domain}` | 웹사이트 설정 교체 |
| `PATCH` | `/api/v1/sites/{domain}` | `verify_level`, `protect`, `difficulty`, `clearance_ttl`, `upstreams`, `rate_limit`, `rules`, `remote_rules`, `routes`, `headers`, `mascot`, `clear_mascot`, `rule_sources` 실시간 패치 |
| `DELETE` | `/api/v1/sites/{domain}` | 웹사이트 삭제 (`204 No Content`; SSH와 Minecraft가 모두 꺼진 상태에서 마지막 남은 사이트를 삭제하려 하면 `409 Conflict` 반환) |
| `GET` | `/api/v1/sites/{domain}/rules` | 웹사이트의 로컬 사용자 지정 규칙 나열 |
| `POST` | `/api/v1/sites/{domain}/rules` | 사용자 지정 규칙 추가 (`201 Created`) |
| `PUT` | `/api/v1/sites/{domain}/rules` | 웹사이트의 모든 사용자 지정 규칙 교체 |
| `DELETE` | `/api/v1/sites/{domain}/rules/{index_or_name}` | 숫자 인덱스 또는 `name`으로 사용자 지정 규칙 삭제 (`204 No Content`) |

### 실시간 완화 예시

```bash
# app.example.com을 verify level 0과 30분 클리어런스 유지 시간으로 강화
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"verify_level": 0, "difficulty": 5, "clearance_ttl": 1800}'

# 경로 라우트, 응답 헤더, 로컬 규칙 소스를 실시간 패치
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"routes": [{"path": "/static/*", "strip_prefix": "/static", "root": "./dist", "browse": true}], "headers": {"X-Served-By": "gymbag"}, "rule_sources": [{"source": "./rules.d", "dynamic": true}]}'

# Gymbagfile을 고치지 않고 악성 CIDR 차단
curl -X POST http://127.0.0.1:2019/api/v1/sites/app.example.com/rules \
  -H "Content-Type: application/json" \
  -d '{"name": "drop-abusive-subnet", "action": "block", "ips": ["198.51.100.0/24"]}'

# 원격 규칙과 위협/VPN/AI 피드 새로고침
curl -X POST http://127.0.0.1:2019/api/v1/rules/remote/refresh
curl -X POST http://127.0.0.1:2019/api/v1/intel/refresh
```

## Redis 공유 상태

Gymbag은 `redis://` 및 `rediss://` URL을 지원하는 순수 Go RESP2 Redis 클라이언트를 내장하고 있습니다. `redis`(또는 `GYMBAG_REDIS_URL`)에 Redis 인스턴스를 지정하면 다음 상태를 공유할 수 있습니다.

- 일회용 HTTP 및 게이트 PoW 챌린지 (`gymbag:challenge:<id>`)
- 고정 윈도우 IP별 속도 제한 (`gymbag:rate:<key>`)
- SSH 및 Minecraft 브라우저 인증 티켓 (`gymbag:ticket:<token>`)과 IP 인증 기록 (`gymbag:clearance:<service>:<ip>`)

시작 시 Redis에 연결할 수 없으면 경고를 로그에 남기고 64-샤드 인메모리 스토어로 폴백합니다. 바로 띄울 수 있는 Compose 스택은 [Docker & Docker Compose 가이드](/ko/guide/docker)를 참고하세요.
