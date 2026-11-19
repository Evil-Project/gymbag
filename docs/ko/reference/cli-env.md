# CLI & `GYMBAG_*` 환경 변수 레퍼런스

우선순위는 다음과 같습니다:

**설정 파일 → `.env` / `GYMBAG_*` 환경 변수 → CLI 플래그**

현재 작업 디렉터리에 `.env` 파일이 있으면 자동으로 로드합니다(기존 OS 환경 변수는 덮어쓰지 않음).

## CLI 하위 명령어

```bash
# 설정 생성 (대화형 마법사 또는 플래그 기반)
gymbag init [flags] [output_path]

# ./Gymbagfile 또는 ./Caddyfile로 서버 시작
gymbag

# 명시적 설정 및 파일 변경 감지 핫 리로드 (2초 간격)
gymbag run --config Gymbagfile --watch

# OS 시스템 서비스 관리 (systemd, launchd, openrc, supervisor)
gymbag service <install|start|stop|restart|status|uninstall|render> [flags]

# 설정 파일 없는 임시 리버스 프록시
gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3 --verify-timeout 12h

# 설정 검증 및 사이트별 정책 출력
gymbag validate --config Gymbagfile

# Gymbagfile 덮어쓰기 정렬
gymbag fmt --overwrite Gymbagfile

# 버전 출력
gymbag version
```

### 서버 및 공통 플래그 (`run` / 기본 / `reverse-proxy`)

| 플래그 | 설명 |
| :--- | :--- |
| `-c`, `--config <path>` | `Gymbagfile`, `Caddyfile`, `.json` 설정 파일 경로 |
| `-d`, `--domain <list>` | 도메인 목록 (`reverse-proxy`에서는 `--from`) |
| `-u`, `--upstream <url>` | 업스트림 URL (`http://localhost:3000` 또는 `:3000`; `reverse-proxy`에서는 `--to`) |
| `-l`, `--verify <0-3\|off>` | 검증 레벨 (`0`, `1`, `2`, `3` 또는 `off`) |
| `--verify-timeout <dur>` | 인증 완료 쿠키 타임아웃 (`30m`, `12h`, `7d` 또는 초; 별칭 `--clearance-ttl`) |
| `--remote-rules <url>` | 원격 방화벽 규칙 URL (반복 또는 쉼표 구분) |
| `--remote-rules-refresh <dur>` | 원격 규칙 갱신 주기 (기본 `5m` / `300s`) |
| `--no-protect` | WAF 검증 비활성화 |
| `--ssl-provider <name>` | ACME / SSL 공급자 재정의 |
| `--admin <addr>` | 로컬 REST 관리 API 활성화 (예: `127.0.0.1:2019` 또는 `off`) |
| `--http-port <port>` | HTTP 리디렉션 및 ACME 포트 (기본 `80`) |
| `--https-port <port>` | HTTPS 게이트웨이 포트 (기본 `443`) |
| `--watch` | 설정 파일 변경 감시 및 자동 핫 리로드 (`run` 전용) |

---

## 환경 변수 (`GYMBAG_*`)

| 변수명 | 설명 |
| :--- | :--- |
| `GYMBAG_CONFIG` | `Gymbagfile`, `Caddyfile`, `.json` 설정 경로 |
| `GYMBAG_BIND` | 바인딩 주소 (로컬 `127.0.0.1`, 공인 도메인 포함 시 `0.0.0.0`) |
| `GYMBAG_HTTP_PORT` / `GYMBAG_HTTPS_PORT` | HTTP / HTTPS 수신 포트 (기본 `80`, `443`) |
| `GYMBAG_DOMAIN` / `GYMBAG_DOMAINS` | 쉼표로 구분된 도메인 목록 |
| `GYMBAG_UPSTREAM` | 기본 업스트림 URL |
| `GYMBAG_UPSTREAMS` | 세미콜론 구분 호스트 매핑: `app.example.com=http://web:3000;api.example.com=http://api:8080` |
| `GYMBAG_PROTECT` | 전체 사이트 WAF 보호 활성화/비활성화 (`on` / `off`) |
| `GYMBAG_PROTECT_DOMAINS` | WAF 보호를 활성화할 도메인 목록 |
| `GYMBAG_VERIFY_LEVEL` | 검증 레벨: `0`–`3` (별칭: `GYMBAG_VERIFY`, `GYMBAG_VERIFY_MODE`, `GYMBAG_MODE`, `GYMBAG_TRIGGER`) |
| `GYMBAG_DEFAULT_RULES` | 내장 기본 방화벽 규칙 활성화/비활성화 (`on` / `off`) |
| `GYMBAG_REMOTE_RULES` | 쉼표/공백 구분 HTTP/HTTPS 원격 방화벽 규칙 URL |
| `GYMBAG_REMOTE_RULES_REFRESH` | 원격 규칙 갱신 주기 (초 또는 기간; 기본 `300` / `5m`) |
| `GYMBAG_DIFFICULTY` | SHA-256 PoW 난이도 (`1`–`5`, 기본 `4`) |
| `GYMBAG_CHALLENGE_TTL` | 챌린지 논스 만료 시간 (초 또는 기간; 기본 `300` / `5m`) |
| `GYMBAG_CLEARANCE_TTL` | 인증 완료 상태 타임아웃 (`30m`, `12h`, `7d` 또는 초; 기본 `86400` / `24h`; 별칭: `GYMBAG_VERIFY_TIMEOUT`, `GYMBAG_VERIFIED_TIMEOUT`) |
| `GYMBAG_COOKIE_NAME` / `GYMBAG_COOKIE_SECURE` | 쿠키 이름 (`waf_clearance`) 및 `Secure` 플래그 (`true`) |
| `GYMBAG_RATE_LIMIT` / `GYMBAG_RATE_WINDOW` | IP별 제한 (`240`) 및 윈도우 (`60`초 / `1m`) |
| `GYMBAG_TLS_MODE` | `auto`, `acme`, `local`, `selfsigned`, `file` |
| `GYMBAG_SSL_PROVIDER` | `letsencrypt`, `letsencrypt_staging`, `zerossl`, `google`, `ssl.com`, `litessl`, `buypass`, `local`, 또는 URL |
| `GYMBAG_ACME_EMAIL` | ACME 등록 이메일 (ZeroSSL 자동 EAB 겸용) |
| `GYMBAG_ACME_DIRECTORY` | 명시적 RFC 8555 ACME v2 디렉터리 URL |
| `GYMBAG_EAB_KID` / `GYMBAG_EAB_HMAC_KEY` | EAB Key ID 및 HMAC 키 |
| `GYMBAG_ACCOUNT_KEY_FILE` | ECDSA/RSA ACME 계정 키 PEM 경로 |
| `GYMBAG_CERT_FILE` / `GYMBAG_KEY_FILE` | `file` 모드용 인증서 및 개인 키 경로 |
| `GYMBAG_KEY_PASSWORD` | 암호화된 PEM 개인 키 암호 |
| `GYMBAG_CERT_DIR` | CA 인증서, ACME 상태, 인텔 및 원격 규칙 캐시 디렉터리 (기본 `certs`) |
| `GYMBAG_RENEW_INTERVAL` / `GYMBAG_RENEW_WITHIN_DAYS` | 갱신 확인 주기 (`43200`초 / `12h`) 및 기준 일수 (`30`일) |
| `GYMBAG_AUTO_HOSTS` | 로컬 도메인용 `/etc/hosts` 자동 관리 (`on` / `off`) |
| `GYMBAG_HSTS` | `Strict-Transport-Security` 헤더 재정의 (`on` / `off`) |
| `GYMBAG_IP_SECURITY` / `GYMBAG_IP_SECURITY_ACTION` | 위협 피드 토글 (`on`/`off`) 및 동작 (`verify` / `block`) |
| `GYMBAG_IP_REPUTATION` | Shodan InternetDB + StopForumSpam 조회 토글 (`on`/`off`) |
| `GYMBAG_AI_IP_LIST` | 공식 AI 크롤러 IP 목록 토글 (`on`/`off`) |
| `GYMBAG_INTEL_REFRESH` | 위협/AI 피드 갱신 주기 (초 또는 기간; 기본 `21600` / `6h`) |
| `GYMBAG_REDIS_URL` | Redis 연결 URL (`redis://[:password@]host:port/db`) |
| `GYMBAG_SECRET` | 클리어런스 쿠키용 HMAC 시크릿 |
| `GYMBAG_ADMIN` | 로컬 Admin REST API 수신 주소 (`127.0.0.1:2019` 또는 `off`) |
