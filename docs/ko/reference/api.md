# REST API & Redis

## 공개 WAF 엔드포인트

보호되는 사이트에서 자동으로 제공됩니다:

| 메서드 | 경로 | 설명 |
| :--- | :--- | :--- |
| `GET` | `/.waf/v1/session` | 클리어런스 상태(`cleared`), `protected`, `verify_level` (`0`–`3`), `verify_mode`, `clearance_ttl`, 인증된 경우 `expires_at` / `expires_in` 반환 |
| `DELETE` | `/.waf/v1/session` | 방문자의 클리어런스 쿠키 삭제 (`204 No Content`) |
| `POST` | `/.waf/v1/challenges` | 일회용 SHA-256 PoW 챌린지 발급 (`201 Created`) |
| `POST` | `/.waf/v1/verifications` | PoW 풀이 검증, 서명된 쿠키 발급 및 `{"ok": true, "expires_in": <ttl>}` 반환 (`/.waf/verify` 별칭 지원) |
| `GET` | `/.waf/assets/{name}` | 내장 마스코트 PNG 에셋 제공 (`/.waf/v1/assets/{name}` 별칭, `ETag` 캐시) |

---

## 로컬 Admin REST API

`admin 127.0.0.1:2019`, `--admin 127.0.0.1:2019`, 또는 `GYMBAG_ADMIN=127.0.0.1:2019`로 활성화합니다.

::: warning 반드시 루프백 또는 사설 관리 인터페이스에 바인딩하세요
Admin API는 인증 토큰 없이 실시간 라우팅과 방화벽 규칙을 변경할 수 있습니다. 항상 `127.0.0.1:2019` 또는 사설 WireGuard/Unix 인터페이스에 바인딩하세요.
:::

| 메서드 | 경로 | 설명 |
| :--- | :--- | :--- |
| `GET` | `/api/v1/health` | 상태, `sites_count`, `tls_source`, `threat_entries`, `ai_entries`, 로드된 `remote_rules` 수 |
| `GET` | `/api/v1/tls` | 활성 TLS 소스, 인증서 경로, SSL 공급자, HSTS 상태, 지원 CA 목록 |
| `POST` | `/api/v1/tls/renew` | 즉시 인증서 갱신 및 원자적 핫 리로드 강제 실행 |
| `GET` | `/api/v1/intel` | 피드별 위협 및 AI 크롤러 수, 마지막 갱신 시각, 평판 캐시 통계 |
| `POST` | `/api/v1/intel/refresh` | 백그라운드 피드 즉시 갱신 트리거 |
| `GET` | `/api/v1/rules/remote` | 원격 규칙 URL별 로드된 규칙 수, 캐시 상태 및 오류 조회 |
| `POST` | `/api/v1/rules/remote/refresh` | 모든 원격 규칙 URL 즉시 가져오기 및 WAF 엔진 원자적 리로드 |
| `GET` | `/api/v1/config` | 활성 런타임 설정 조회 (`secret`, `key_password`, `eab_hmac_key` 마스킹) |
| `PUT` | `/api/v1/config` | 전체 런타임 설정 검증 후 원자적 교체 |
| `GET` | `/api/v1/sites` | 모든 웹사이트 및 WAF 정책 목록 |
| `POST` | `/api/v1/sites` | 런타임에 새 웹사이트 추가 (`201 Created`) |
| `GET` | `/api/v1/sites/{domain}` | 단일 웹사이트 설정 조회 |
| `PUT` | `/api/v1/sites/{domain}` | 단일 웹사이트 설정 교체 |
| `PATCH` | `/api/v1/sites/{domain}` | `verify_level`, `protect`, `difficulty`, `clearance_ttl`, `upstreams`, `rate_limit`, `rules`, `remote_rules` 부분 수정 |
| `DELETE` | `/api/v1/sites/{domain}` | 웹사이트 제거 (`204 No Content`; 마지막 사이트는 삭제 불가) |
| `GET` | `/api/v1/sites/{domain}/rules` | 사이트의 로컬 사용자 지정 규칙 목록 |
| `POST` | `/api/v1/sites/{domain}/rules` | 사용자 지정 방화벽 규칙 실시간 추가 (`201 Created`) |
| `PUT` | `/api/v1/sites/{domain}/rules` | 사이트의 모든 사용자 지정 방화벽 규칙 교체 |
| `DELETE` | `/api/v1/sites/{domain}/rules/{index_or_name}` | 인덱스 또는 `name`으로 규칙 삭제 (`204 No Content`) |

### 실시간 긴급 완화 예시

```bash
# app.example.com을 레벨 0(전체 검증) 및 30분 타임아웃으로 강화
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"verify_level": 0, "difficulty": 5, "clearance_ttl": 1800}'

# Gymbagfile 수정 없이 악성 CIDR 즉시 차단
curl -X POST http://127.0.0.1:2019/api/v1/sites/app.example.com/rules \
  -H "Content-Type: application/json" \
  -d '{"name": "drop-abusive-subnet", "action": "block", "ips": ["198.51.100.0/24"]}'

# 원격 규칙 및 위협 인텔 즉시 갱신
curl -X POST http://127.0.0.1:2019/api/v1/rules/remote/refresh
curl -X POST http://127.0.0.1:2019/api/v1/intel/refresh
```

---

## Redis 공유 상태

Gymbag은 외부 의존성이 없는 순수 Go RESP2 Redis 클라이언트를 내장하고 있습니다. `redis redis://[:password@]host:6379/0`(또는 `GYMBAG_REDIS_URL`)을 설정하면 컨테이너 재시작이나 다중 게이트웨이 레플리카 간에 일회용 PoW 챌린지와 IP별 속도 제한 창을 공유할 수 있습니다.

Docker 및 Docker Compose 배포 방법은 [Docker & Docker Compose 가이드](/ko/guide/docker)를 참고하세요.
