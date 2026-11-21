# 두 줄 명령어 빠른 시작

Gymbag은 외부 Go 의존성이 전혀 없는 단일 독립 바이너리로 컴파일됩니다 — 모든 HTML 템플릿, SHA-256 브라우저 솔버 스크립트, 마스코트 PNG가 `go:embed`로 바이너리에 직접 포함됩니다.

## 설치 방법

### 사전 빌드 정적 바이너리, Docker, Arch Linux & NixOS

[GitHub Releases](https://github.com/Evil-Project/gymbag/releases)의 각 릴리스 태그마다 `CGO_ENABLED=0` 정적 바이너리(`linux`, `darwin`, `windows`용 `amd64`/`arm64`), Arch Linux `.pkg.tar.zst` 패키지(`x86_64`, `aarch64`), 멀티 아키텍처 GHCR 컨테이너 이미지(`ghcr.io/evil-project/gymbag`)가 제공됩니다:

::: code-group
```bash [Docker (GHCR)]
docker run -d --name gymbag \
  -p 80:80 -p 443:443 \
  --cap-add NET_BIND_SERVICE \
  -v "$(pwd)/Gymbagfile:/app/Gymbagfile:ro" \
  -v gymbag-certs:/app/certs \
  ghcr.io/evil-project/gymbag:latest
```

```bash [Arch Linux (pacman)]
# 릴리스 .pkg.tar.zst 패키지 설치
sudo pacman -U gymbag-<version>-1-x86_64.pkg.tar.zst

# 또는 PKGBUILD로 소스 빌드
git clone https://github.com/Evil-Project/gymbag.git
cd gymbag/packaging/arch
makepkg -si
```

```bash [Nix / NixOS]
# Nix Flakes로 바로 실행 또는 설치
nix run github:Evil-Project/gymbag -- --help
nix profile install github:Evil-Project/gymbag

# 정적 Linux 릴리스 바이너리(CGO_ENABLED=0)는 NixOS에서도 바로 실행됩니다
```

```bash [Go 소스 빌드]
git clone https://github.com/Evil-Project/gymbag.git
cd gymbag
go build -o gymbag ./cmd/gymbag
```
:::

컨테이너 배포, Compose + Redis 및 볼륨 권한 설정은 [Docker & Docker Compose 가이드](/ko/guide/docker)를 참고하세요.

## 두 줄 명령어로 시작하기

### 옵션 A: 플래그 기반 (비대화형)

```bash
# 1. gymbag.json 생성 (Caddy 스타일 Gymbagfile을 원하면 --format gymbagfile 추가)
./gymbag init -d app.example.com -u http://localhost:3000

# 2. Gymbag 실행 (./gymbag.json, ./Gymbagfile, ./Caddyfile 순으로 자동 로드)
./gymbag
```

### 옵션 B: 대화형 설정 마법사

플래그 없이 `gymbag init`을 실행하면 도메인, 업스트림 URL, 검증 레벨, SSL 공급자, 포트, 관리 API 및 시스템 서비스 등록을 단계별로 안내합니다:

```bash
# 1. 대화형 설정 생성기
./gymbag init

# 2. Gymbag 실행
./gymbag
```

### 옵션 C: 로컬 개발 프리셋 (`*.internal`)

```bash
# 1. 로컬 PKI가 포함된 app.internal -> http://localhost:3000 생성
./gymbag init --preset local

# 2. Gymbag 실행 (비특권 포트 사용 또는 80/443 및 /etc/hosts용 sudo)
./gymbag --http-port 8080 --https-port 8443
```

### 옵션 D: 시스템 서비스 등록

```bash
# 1. gymbag.json 생성 및 systemd / launchd / openrc / supervisor 등록
./gymbag init -d app.example.com -u :3000 --service auto

# 2. 등록된 서비스 시작
./gymbag service start
```

자세한 내용은 [시스템 서비스 설정](/ko/guide/system-service)을 참고하세요.

---

## 기존 nginx / Apache / Caddyfile 설정

Gymbag은 `nginx.conf`, Apache `httpd.conf` / `apache2.conf`, Caddyfile v2 파일을 기본으로 읽을 수 있어 기존 설정에서 바로 시작할 수 있습니다:

```bash
./gymbag run -c /etc/nginx/nginx.conf
./gymbag config convert -c Caddyfile -o gymbag.json --format json
```

경로 라우트, 정적 파일, 리디렉션 및 변환에 대한 자세한 내용은 [경로 라우트 및 설정 가져오기](/ko/guide/routes-and-imports)를 참고하세요.

---

## 임시 리버스 프록시 (설정 파일 없음)

디스크에 파일을 만들지 않고 즉시 테스트할 수 있습니다:

```bash
./gymbag reverse-proxy --from app.internal --to :3000 --verify 3 --verify-timeout 12h
```

---

## 설정 생성기 옵션 (`gymbag init`)

`gymbag init`은 디스크에 쓰기 전에 생성된 설정을 자동으로 검증합니다.

| 플래그 | 설명 |
| :--- | :--- |
| `-o`, `--output <path>` | 출력 경로 (기본값 `gymbag.json`, `--format gymbagfile` 사용 시 `Gymbagfile`) |
| `--format <fmt>` | `json` (기본값) 또는 `gymbagfile` |
| `--preset <name>` | `standard`, `local`, `public`, `strict`, `ai-only`, `suspicious`, `proxy-only` |
| `-d`, `--domain <domains>` | 도메인 (쉼표로 구분) |
| `-u`, `--upstream <url>` | 업스트림 URL (`http://localhost:3000` 또는 `:3000`) |
| `--site <domain=url>` | 여러 사이트 블록 추가 (반복 사용 가능, 예: `--site app.example.com=:3000 --site api.example.com=:8080`) |
| `-l`, `--verify <0-3\|off>` | 검증 레벨 (`0`, `1`, `2`, `3` 또는 `off`) |
| `--difficulty <1-5>` | SHA-256 PoW 선행 0 개수 난이도 (기본 `4`) |
| `--verify-timeout <dur>` | 인증 완료 유효 시간 (`30m`, `12h`, `7d` 또는 초; 별칭 `--clearance-ttl`) |
| `--remote-rules <url>` | 원격 방화벽 규칙 URL (반복 또는 쉼표 구분) |
| `--remote-rules-refresh <dur>` | 원격 규칙 갱신 주기 (기본 `5m`) |
| `--rules-file <path>` | 전용 방화벽 규칙 파일 경로 (반복 사용 가능) |
| `--rules-dir <path>` | 방화벽 규칙 파일이 포함된 디렉터리 (반복 사용 가능) |
| `--rules-dynamic` | 파일 변경 시 `--rules-file` / `--rules-dir` 자동 리로드 |
| `--rules-watch-interval <dur>` | 동적 로컬 규칙 파일/폴더 폴링 주기 (기본 `2s`) |
| `--no-protect` | WAF 검증 비활성화 (`protect off`) |
| `--no-default-rules` | 내장 기본 방화벽 규칙 비활성화 (`default_rules off`) |
| `--ssl-provider <name>` | `letsencrypt`, `zerossl`, `google`, `ssl.com`, `litessl`, `buypass`, `local`, 또는 사용자 지정 URL |
| `--email <addr>` | ACME 연락처 이메일 (ZeroSSL 자동 EAB 지원) |
| `--eab-kid` / `--eab-hmac-key` | ACME External Account Binding 자격 증명 |
| `--http-port` / `--https-port` | HTTP / HTTPS 포트 (기본 `80` / `443`) |
| `--bind <ip>` | 바인딩 IP 주소 (생략 시 자동 선택) |
| `--admin <addr>` | 로컬 REST 관리 API 주소 (예: `127.0.0.1:2019`) |
| `--cert-dir <dir>` | 인증서, 로컬 CA, 캐시 저장 디렉터리 (기본 `certs`) |
| `--redis <url>` | 챌린지 및 속도 제한 공유용 Redis URL |
| `--ip-security-action <act>` | 위협 인텔 일치 시 동작 (`verify` 또는 `block`) |
| `--no-auto-hosts` | 로컬 도메인 `/etc/hosts` 자동 관리 비활성화 |
| `-y`, `--yes` | 대화형 프롬프트 건너뛰기 |
| `-i`, `--interactive` | 플래그가 있어도 대화형 마법사 실행 |
| `--dry-run` | 파일에 쓰지 않고 표준 출력으로 출력 |
| `-f`, `--force` | 기존 설정 파일 덮어쓰기 |
| `--service <type>` | 시스템 서비스 동시 등록 (`auto`, `systemd`, `launchd`, `openrc`, `supervisor`) |

---

## 기본 제공 3계층 보안

`verify` (레벨 `3`: `suspicious & ai`) 또는 `block`만 지정하면 정규식을 하나도 쓰지 않고도 3계층 보호가 즉시 작동합니다:

1. **기본 방화벽 규칙 (`default_rules on`)** — `/.env`, `/.git/*`, `/.aws/*`, `/wp-admin/*`, `sqlmap`/`nikto`/`nuclei`, SQLi/XSS 쿼리 프로브를 `403`으로 차단하고 CLI/헤드리스 자동화 클라이언트에 PoW 챌린지를 요구합니다.
2. **자동 업데이트 IP 위협 인텔리전스** — 시작 시 및 6시간마다 IPsum(레벨 3+), Spamhaus DROP v4/v6, abuse.ch Feodo Tracker를 가져오며 키가 필요 없는 Shodan InternetDB 및 StopForumSpam 실시간 조회를 수행합니다.
3. **공식 AI 크롤러 IP 목록** — OpenAI, Anthropic, Perplexity, Google, Apple의 공개 CIDR 대역을 추적하여 데스크톱 브라우저 User-Agent를 위조한 크롤러도 정확히 탐지합니다.
