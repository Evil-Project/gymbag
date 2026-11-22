# CLI & `GYMBAG_*` 환경 변수

적용 우선순위는 다음과 같습니다.

**설정 파일 -> `.env` / `GYMBAG_*` 환경 변수 -> CLI 플래그**

작업 디렉터리에 `.env` 파일이 있으면 기존 환경 변수를 덮어쓰지 않고 자동으로 불러옵니다.

## CLI 하위 명령어

```bash
# 설정 생성 (대화형 마법사 또는 플래그; 기본값으로 gymbag.json 생성)
gymbag init [flags] [output_path]
gymbag init --format gymbagfile -o Gymbagfile -d app.example.com -u :3000

# ./gymbag.json, ./Gymbagfile, ./Caddyfile을 사용해 시작
gymbag

# 명시적 설정 파일로 시작 및 편집 시 핫 리로드 (2s 주기 폴링)
gymbag run --config Gymbagfile --watch

# OS 서비스 관리 (systemd, launchd, openrc, supervisor)
gymbag service <install|start|stop|restart|status|uninstall|render> [flags]

# 사이트, 라우트, 전역 설정 관리 및 설정 파일 변환
gymbag config <list|show|add|update|remove|site|route|set|unset|convert> [flags]

# 규칙 파일, 폴더, 인라인 규칙 관리
gymbag ruleset <list|add|update|remove|validate|init|rule> [flags]

# 설정 파일 없는 임시 HTTP/HTTPS 리버스 프록시
gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3 --verify-timeout 12h

# 설정 검증 및 해석된 서비스 요약 출력
gymbag validate --config Gymbagfile

# Gymbagfile 제자리 포맷팅 (4칸 들여쓰기)
gymbag fmt --overwrite Gymbagfile

# 버전 출력
gymbag version
```

### 서버 플래그 (`run` / 기본 실행 / `reverse-proxy`)

| 플래그 | 설명 |
| :--- | :--- |
| `-c`, `--config <path>` | `gymbag.json`, `Gymbagfile`, `Caddyfile`, `nginx.conf`, Apache 설정 경로 |
| `-d`, `--domain <list>` | 도메인 목록 재정의 (쉼표 구분; `reverse-proxy`에서는 `--from`) |
| `-u`, `--upstream <url>` | 업스트림 URL 재정의 (`http://localhost:3000` 또는 `:3000`; `reverse-proxy`에서는 `--to`) |
| `-l`, `--verify <0-3\|off>` | 검증 레벨 재정의 (`0`, `1`, `2`, `3`, 또는 `off`; 별칭 `--verify-level`, `-m`, `--mode`) |
| `--verify-timeout <dur>` | 클리어런스 유지 시간 재정의 (`30m`, `12h`, `7d`, 또는 초; 별칭 `--clearance-ttl`) |
| `--remote-rules <url>` | 원격 방화벽 규칙 URL 연결 (반복 또는 쉼표 구분) |
| `--remote-rules-refresh <dur>` | 원격 규칙 새로고침 주기 (기본 `5m` / `300s`) |
| `--no-protect` | CLI로 설정한 사이트의 PoW 검증 비활성화 |
| `--ssl-provider <name>` | ACME / SSL 공급자 재정의 |
| `--admin <addr>` | 로컬 REST Admin API 활성화 (`127.0.0.1:2019` 또는 `off`) |
| `--http-port <port>` | HTTP 리다이렉트 & ACME HTTP-01 포트 (기본 `80`) |
| `--https-port <port>` | HTTPS 게이트웨이 포트 (기본 `443`) |
| `--watch` | 설정 파일 감시 및 변경 시 핫 리로드 (`run` / 기본 실행 전용) |

## 설정 관리 (`gymbag config`)

모든 `config` 하위 명령어는 `-c`, `--config <path>`를 받습니다(기본값은 자동 감지된 설정 파일).

```bash
gymbag config list
gymbag config add -d api.example.com -u :8080 -l 1
gymbag config update -d api.example.com -l 3 --verify-timeout 6h
gymbag config remove -d api.example.com
gymbag config set admin 127.0.0.1:2019
gymbag config unset admin
gymbag config route add -d app.example.com --path '/static/*' --strip-prefix /static --root ./dist --browse
gymbag config route list -d app.example.com
gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```

| 하위 명령어 | 설명 |
| :--- | :--- |
| `list` / `show` | 사이트 및 TCP 서비스 나열 / 포맷된 설정 출력 (`--format json\|gymbagfile`) |
| `add` | 사이트 추가: `-d`, `-u`, `-l`, `--difficulty`, `--verify-timeout`, `--no-protect`, `--no-default-rules`, `--cert`, `--key`, `--root`, `--browse`, `--fallback`, `--respond`, `--status`, `--redir`, `--code`, `--header Name=Value\|-Name`, `-f` |
| `update` | 사이트 수정: `-d`, `-u`, `-l`, `--difficulty`, `--verify-timeout`, `--protect on\|off`, `--default-rules on\|off`, `--cert`, `--key`, `--clear-tls`, `--header`, `--remove-header` |
| `remove` | 사이트 제거 (`-d`) |
| `route list\|add\|update\|remove` | 경로 라우트 관리: `-d`, `--path`, `--strip-prefix`, `-u`, `--root`, `--browse`, `--fallback`, `--respond`, `--status`, `--redir`, `--code`, `--header`, `--index` |
| `set <key> <value>` / `unset <key>` | 전역 키 설정 또는 초기화 (`email`, `ssl_provider`, `http_port`, `https_port`, `bind`, `admin`, `cert_dir`, `redis`, `ip_security`, `ip_security_action`, `ip_reputation`, `ai_ip_list`, `intel_refresh`, `remote_rules_refresh`, `rules_watch_interval`, `verify_timeout`, `auto_hosts`, `hsts`, `eab_kid`, `eab_hmac_key`) |
| `convert` | nginx, Apache, Caddyfile, Gymbagfile, JSON 상호 변환: `-c <in>`, `-o <out>`, `--format json\|gymbagfile`, `-f` |

## 규칙 세트 관리 (`gymbag ruleset`)

```bash
gymbag ruleset init ./rules/custom.rules
gymbag ruleset add --source ./rules --dynamic --name local-dir
gymbag ruleset update local-dir --static
gymbag ruleset remove local-dir
gymbag ruleset validate ./rules
gymbag ruleset rule add --file ./rules/custom.rules --action block --name bad-bot --ua "*BadBot*"
```

| 하위 명령어 | 설명 |
| :--- | :--- |
| `list` | 연결된 규칙 소스 목록 조회 (`-c`, `-d`) |
| `add` | 규칙 파일, 폴더, URL 연결: `--source`, `--name`, `--dynamic` / `--static`, `-d <domain>` 또는 `--global`, `-c` |
| `update` | 연결된 규칙 세트 수정: `--target` (또는 위치 인자 source/name), `--source`, `--name`, `--dynamic` / `--static`, `-d` / `--global`, `-c` |
| `remove` | 규칙 세트 연결 해제: `--source <source-or-name>`, `-d` / `--global`, `-c` |
| `validate <path>` | 규칙 파일 또는 규칙 디렉터리 검증 |
| `init <path>` | 시작용 규칙 세트 파일 생성 (`-o`, `--format rules\|json`, `-f`) |
| `rule list\|add\|update\|remove` | `--file` 내 단일 규칙 관리 (또는 `-c` / `-d`로 설정 내 인라인 관리): `--name`, `--index`, `--action allow\|verify\|block`, `--ip`, `--ua`, `--path`, `--query`, `--method`, `--header`, `--cookie` |

## 환경 변수 (`GYMBAG_*`)

| 변수 | 설명 |
| :--- | :--- |
| `GYMBAG_CONFIG` | `gymbag.json`, `Gymbagfile`, `Caddyfile`, `nginx.conf`, Apache 설정 파일 경로 |
| `GYMBAG_BIND` | 바인드 주소 (로컬 전용 HTTP는 `127.0.0.1`, 공용 도메인·SSH·Minecraft 활성 시 `0.0.0.0`) |
| `GYMBAG_HTTP_PORT` / `GYMBAG_HTTPS_PORT` | HTTP 및 HTTPS 수신 포트 (기본 `80` 및 `443`) |
| `GYMBAG_DOMAIN` / `GYMBAG_DOMAINS` | 쉼표로 구분된 도메인 목록 |
| `GYMBAG_UPSTREAM` | 기본 업스트림 대상 URL |
| `GYMBAG_UPSTREAMS` | 세미콜론 구분 호스트 맵: `app.example.com=http://web:3000;api.example.com=http://api:8080` |
| `GYMBAG_PROTECT` / `GYMBAG_PROTECT_DOMAINS` | 전체 사이트 WAF 보호 토글 (`on`/`off`), 또는 지정한 도메인으로 보호 제한 |
| `GYMBAG_VERIFY_LEVEL` | 검증 레벨: `0`, `1`, `2`, `3`, 또는 `off` (별칭: `GYMBAG_VERIFY`, `GYMBAG_VERIFY_MODE`, `GYMBAG_MODE`, `GYMBAG_TRIGGER`) |
| `GYMBAG_DEFAULT_RULES` | 내장 기본 규칙 (`on` / `off`) |
| `GYMBAG_REMOTE_RULES` / `GYMBAG_REMOTE_RULES_REFRESH` | 원격 규칙 URL 및 새로고침 주기 (기본 `300`s / `5m`) |
| `GYMBAG_RULES_WATCH_INTERVAL` | `dynamic` 로컬 규칙 소스 폴링 주기 (기본 `2`s / `2s`) |
| `GYMBAG_DIFFICULTY` | SHA-256 PoW 난이도 (`1`–`5`, 기본 `4`) |
| `GYMBAG_CHALLENGE_TTL` / `GYMBAG_CLEARANCE_TTL` | 챌린지 만료 (`300`s / `5m`) 및 클리어런스 유지 시간 (`86400`s / `24h`; 별칭: `GYMBAG_VERIFY_TIMEOUT`, `GYMBAG_VERIFIED_TIMEOUT`) |
| `GYMBAG_COOKIE_NAME` / `GYMBAG_COOKIE_SECURE` | 클리어런스 쿠키 이름 (`waf_clearance`) 및 `Secure` 플래그 (`true`) |
| `GYMBAG_RATE_LIMIT` / `GYMBAG_RATE_WINDOW` | IP당 요청 제한 (`240`) 및 윈도우 (`60`s / `1m`) |
| `GYMBAG_TLS_MODE` / `GYMBAG_SSL_PROVIDER` | TLS 모드 (`auto`, `acme`, `local`, `selfsigned`, `file`) 및 CA 공급자 또는 URL |
| `GYMBAG_ACME_EMAIL` / `GYMBAG_ACME_DIRECTORY` | ACME 연락처 이메일 (및 ZeroSSL 자동 EAB)과 명시적 ACME 디렉터리 URL |
| `GYMBAG_EAB_KID` / `GYMBAG_EAB_HMAC_KEY` | External Account Binding Key ID 및 HMAC 키 |
| `GYMBAG_ACCOUNT_KEY_FILE` | ECDSA 또는 RSA ACME 계정 키 PEM 경로 |
| `GYMBAG_CERT_FILE` / `GYMBAG_KEY_FILE` / `GYMBAG_KEY_PASSWORD` | `file` 모드용 인증서, 키, 개인 키 암호 |
| `GYMBAG_CERT_DIR` | 인증서, CA 키, 시크릿, 인텔 캐시, 규칙 캐시 디렉터리 (기본 `certs`) |
| `GYMBAG_RENEW_INTERVAL` / `GYMBAG_RENEW_WITHIN_DAYS` | 갱신 확인 주기 (`43200`s / `12h`) 및 임계 일수 (`30`일) |
| `GYMBAG_AUTO_HOSTS` / `GYMBAG_HSTS` | `/etc/hosts` 자동 관리 (`on`/`off`) 및 `Strict-Transport-Security` 재정의 (`on`/`off`) |
| `GYMBAG_IP_SECURITY` / `GYMBAG_IP_SECURITY_ACTION` | 위협 피드 토글 (`on`/`off`) 및 일치 동작 (`verify` 또는 `block`) |
| `GYMBAG_IP_REPUTATION` / `GYMBAG_AI_IP_LIST` | Shodan InternetDB + StopForumSpam 조회 토글 (`on`/`off`) 및 공식 AI 크롤러 목록 (`on`/`off`) |
| `GYMBAG_VPN_DETECTION` | 의심 판정 시 무료 VPN/프록시/Tor 탐지 토글 (`on`/`off`, 기본 `on`) |
| `GYMBAG_INTEL_REFRESH` | 위협, VPN, AI 피드 갱신 주기 (기본 `21600`s / `6h`) |
| `GYMBAG_VERIFY_URL` / `GYMBAG_VERIFY_PORT` | 공용 HTTPS 인증 포털 URL (빈 값 = 서버 IP + IP 인증서) 및 포트 (기본 `8443`) |
| `GYMBAG_SSH_ENABLED` / `GYMBAG_SSH_PORT` / `GYMBAG_SSH_UPSTREAM` | SSH 보호 활성화 (기본 `22`) 및 백엔드 `sshd` 대상 설정 |
| `GYMBAG_SSH_VERIFY_LEVEL` / `GYMBAG_SSH_PROTECT` | SSH 검증 레벨 (`0`–`3` 또는 `off`) 및 보호 토글 (`on`/`off`) |
| `GYMBAG_MC_ENABLED` / `GYMBAG_MC_PORT` / `GYMBAG_MC_UPSTREAM` | Minecraft 보호 활성화 (기본 `25565`) 및 백엔드 대상 설정 (`GYMBAG_MINECRAFT_*` 별칭 지원) |
| `GYMBAG_MC_VERIFY_LEVEL` / `GYMBAG_MC_PROTECT` | Minecraft 검증 레벨 (`0`–`3` 또는 `off`) 및 보호 토글 (`on`/`off`) |
| `GYMBAG_REDIS_URL` | Redis URL (`redis://` 또는 `rediss://`) |
| `GYMBAG_SECRET` / `GYMBAG_ADMIN` | HMAC 클리어런스 시크릿 및 Admin REST API 수신 주소 (`127.0.0.1:2019` 또는 `off`) |
