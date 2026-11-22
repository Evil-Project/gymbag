# 두 줄 명령어 빠른 시작

Gymbag은 외부 의존성 없이 Go 표준 라이브러리만으로 단일 바이너리로 컴파일됩니다. HTML 템플릿, SHA-256 브라우저 솔버 스크립트, 마스코트 PNG도 `go:embed`로 바이너리에 직접 포함됩니다.

## 설치

[GitHub Releases](https://github.com/Evil-Project/gymbag/releases)의 각 릴리스 태그마다 정적 `CGO_ENABLED=0` 바이너리(`linux`, `darwin`, `windows`용 `amd64`/`arm64`), Debian/Ubuntu `.deb` 패키지, Arch Linux `.pkg.tar.zst` 패키지, NixOS 번들, 멀티 아키텍처 GHCR 컨테이너 이미지(`ghcr.io/evil-project/gymbag`)가 올라옵니다.

::: code-group
```bash [Debian / Ubuntu (.deb)]
# 릴리스 .deb 패키지 설치 (cap_net_bind_service 및 systemd 유닛 자동 설정)
sudo apt install ./gymbag_<version>_linux_amd64.deb

# 설정 생성 후 서비스 활성화
sudo gymbag init -o /etc/gymbag/Gymbagfile --format gymbagfile
sudo systemctl enable --now gymbag
```

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

# 또는 packaging/arch에서 직접 빌드
git clone https://github.com/Evil-Project/gymbag.git
cd gymbag/packaging/arch
makepkg -si
```

```bash [Nix / NixOS]
nix run github:Evil-Project/gymbag -- --help
nix profile install github:Evil-Project/gymbag

# 정적 Linux 릴리스 바이너리(CGO_ENABLED=0)는 NixOS에서도 바로 실행됩니다
```

```bash [소스 빌드 (Go)]
git clone https://github.com/Evil-Project/gymbag.git
cd gymbag
go build -o gymbag ./cmd/gymbag
```
:::

컨테이너 배포, Compose + Redis 스택, 볼륨 권한 설정은 [Docker & Docker Compose 가이드](/ko/guide/docker)를 참고하세요.

## 두 줄로 시작하기

`gymbag init`은 기본적으로 `gymbag.json`을 생성하며, `--format gymbagfile`(또는 `-o Gymbagfile`)을 주면 `Gymbagfile`을 만듭니다. 생성된 모든 설정은 디스크에 기록하기 전에 자동으로 검증합니다.

### HTTP / HTTPS 웹사이트

```bash
# 1. gymbag.json 생성 (Caddy 문법을 쓰려면 --format gymbagfile 추가)
./gymbag init -d app.example.com -u http://localhost:3000

# 2. Gymbag 시작 (./gymbag.json, ./Gymbagfile, ./Caddyfile 순으로 자동 탐색)
./gymbag
```

### SSH 보호

`/etc/ssh/sshd_config`에서 `sshd` 포트를 `20000–59999` 사이의 빈 포트로 옮기고(`.gymbag.bak` 백업 생성), `sshd -t`로 검증한 뒤(실패 시 롤백) `sshd`를 리로드합니다. 이어서 호스트 방화벽(`ufw`, `firewalld`, `nftables`, 또는 `iptables`+`ip6tables`)을 갱신해 외부 트래픽이 포트 `22`의 Gymbag을 통해서만 들어오도록 설정합니다.

```bash
# 원격 호스트에서 실행할 때는 두 번째 SSH 터미널을 열어 둔 채 진행하세요
sudo ./gymbag init -t ssh
sudo ./gymbag
```

### Minecraft 서버 보호

`server.properties`의 `server-port`를 `20000–59999` 사이의 빈 포트로 옮기고 비어 있거나 `0.0.0.0`/`::`인 `server-ip`를 `127.0.0.1`로 바꿉니다. 서버 목록 핑은 그대로 동작해 MOTD와 접속자 수가 표시되며, 로그인 시에는 Minecraft **"Connection Lost"** 대화 상자에 HTTPS 인증 링크를 띄웁니다.

```bash
./gymbag init -t minecraft --server-properties /path/to/server.properties
./gymbag
```

### 통합 보호 (`http,ssh,minecraft`) 또는 대화형 마법사

한 프로세스로 웹사이트, SSH, Minecraft를 함께 보호할 수 있습니다.

```bash
# 비대화형 통합 설정:
sudo ./gymbag init -t http,ssh,minecraft -d app.example.com -u :3000 --server-properties /path/to/server.properties
sudo ./gymbag

# 또는 대화형 마법사 실행:
./gymbag init
./gymbag
```

> **인증 포털 및 IP SSL 인증서:** SSH와 Minecraft는 기본적으로 `8443` 포트의 HTTPS 인증 포털(`--verify-port`)을 함께 사용합니다. 사용자 지정 도메인을 쓰려면 `--verify-url https://verify.example.com:8443`을 지정하고, 비워 두면 감지된 서버 IP에서 RFC 8738 ACME IP 인증서(HTTP-01 도달 불가 시 Gymbag 로컬 CA 또는 자체 서명 IP 인증서로 폴백)로 인증 페이지를 제공합니다. 자세한 내용은 [SSH & Minecraft 보호 가이드](/ko/guide/ssh-minecraft)를 참고하세요.

### 로컬 개발 (`*.internal`)

```bash
# 1. 로컬 PKI가 적용된 app.internal -> http://localhost:3000 생성
./gymbag init --preset local

# 2. 비권한 포트로 실행 (80/443 포트 및 /etc/hosts 자동 관리에는 sudo 사용)
./gymbag --http-port 8080 --https-port 8443
```

### 시스템 서비스로 등록

```bash
./gymbag init -d app.example.com -u :3000 --service auto
./gymbag service start
```

`systemd`, `launchd`, `openrc`, `supervisor` 설정 방법은 [시스템 서비스 설정](/ko/guide/system-service)을 참고하세요.

## 기존 nginx / Apache / Caddyfile 설정

Gymbag은 네이티브 `nginx.conf`, Apache `httpd.conf` / `apache2.conf`, Caddyfile v2 파일을 직접 파싱합니다.

```bash
./gymbag run -c /etc/nginx/nginx.conf
./gymbag config convert -c Caddyfile -o gymbag.json --format json
```

경로 라우트, 정적 파일, 리다이렉트, 변환 상세 내용은 [경로 라우트 및 설정 가져오기](/ko/guide/routes-and-imports)를 참고하세요.

## 설정 파일 없는 임시 리버스 프록시

```bash
./gymbag reverse-proxy --from app.internal --to :3000 --verify 3 --verify-timeout 12h
```

## `gymbag init` 플래그

| 플래그 | 설명 |
| :--- | :--- |
| `-o`, `--output <path>` | 출력 경로 (기본 `gymbag.json`, `--format gymbagfile` 시 `Gymbagfile`) |
| `--format <fmt>` | `json` (기본값) 또는 `gymbagfile` |
| `-t`, `--template <list>` | 서비스 템플릿: `http`, `ssh`, `minecraft`, 또는 `all` (쉼표 구분 또는 반복 가능) |
| `--preset <name>` | `standard`, `local`, `public`, `strict`, `ai-only`, `suspicious`, `proxy-only`, `http`, `ssh`, `minecraft`, 또는 `all` |
| `-d`, `--domain <domains>` | 도메인 목록 (쉼표 구분) |
| `-u`, `--upstream <url>` | 업스트림 URL (`http://localhost:3000` 또는 `:3000`) |
| `--site <domain=url>` | 여러 사이트 항목 추가 (반복 가능) |
| `--ssh` / `--ssh-port <port>` | SSH 보호 활성화 (기본 포트 `22`) |
| `--ssh-upstream <addr>` / `--sshd-config <p>` | `sshd` 대상 주소 또는 `sshd_config` 경로 지정 (기본 `/etc/ssh/sshd_config`) |
| `--minecraft`, `--mc` / `--mc-port <port>` | Minecraft 보호 활성화 (기본 포트 `25565`) |
| `--mc-upstream <addr>` / `--server-properties <p>` | Minecraft 백엔드 주소 또는 `server.properties` 경로 지정 |
| `--verify-url <url>` | 공용 HTTPS 인증 포털 URL (빈 값 = 서버 IP + IP SSL 인증서) |
| `--verify-port <port>` | 공용 HTTPS 인증 포털 포트 (기본 `8443`) |
| `--no-auto-setup` / `--no-firewall` | 자동 `sshd_config`/방화벽 또는 `server.properties` 수정 건너뛰기 |
| `-l`, `--verify <0-3\|off>` | 검증 레벨 (`0`, `1`, `2`, `3`, 또는 `off`) |
| `--difficulty <1-5>` | SHA-256 PoW 난이도 (기본 `4`) |
| `--verify-timeout <dur>` | 인증 유지 시간 (`30m`, `12h`, `7d`, 또는 초; 별칭 `--clearance-ttl`) |
| `--remote-rules <url>` | 원격 방화벽 규칙 URL (반복 또는 쉼표 구분) |
| `--remote-rules-refresh <dur>` | 원격 규칙 새로고침 주기 (기본 `5m`) |
| `--rules-file <path>` / `--rules-dir <path>` | 로컬 규칙 파일 또는 디렉터리 (반복 가능) |
| `--rules-dynamic` / `--rules-watch-interval <dur>` | 로컬 규칙 소스 변경 감시 (기본 `2s`) |
| `--no-protect` / `--no-default-rules` | PoW 검증 또는 내장 기본 규칙 비활성화 |
| `--ssl-provider <name>` | `letsencrypt`, `zerossl`, `google`, `ssl.com`, `litessl`, `buypass`, `local`, 또는 URL |
| `--email <addr>` | ACME 연락처 이메일 (ZeroSSL 자동 EAB에도 사용) |
| `--eab-kid` / `--eab-hmac-key` | ACME External Account Binding 자격 증명 |
| `--http-port` / `--https-port` | HTTP/HTTPS 포트 지정 (기본 `80` / `443`) |
| `--bind <ip>` | 바인드 IP 주소 (생략 시 자동 선택) |
| `--admin <addr>` | 로컬 REST Admin API 활성화 (예: `127.0.0.1:2019`) |
| `--cert-dir <dir>` | 인증서, 로컬 CA, 캐시 저장 디렉터리 (기본 `certs`) |
| `--redis <url>` | 챌린지, 속도 제한, SSH/MC 게이트 상태 공유용 Redis URL |
| `--ip-security-action <act>` | 위협 인텔 일치 시 동작 (`verify` 또는 `block`) |
| `--no-auto-hosts` | 로컬 도메인 `/etc/hosts` 관리 비활성화 |
| `-y`, `--yes` / `-i`, `--interactive` | 마법사 프롬프트 건너뛰기 또는 대화형 마법사 강제 실행 |
| `--dry-run` / `-f`, `--force` | 파일에 쓰지 않고 stdout 출력 / 기존 파일 덮어쓰기 |
| `--service <type>` | OS 서비스 등록 (`auto`, `systemd`, `launchd`, `openrc`, `supervisor`) |

## 규칙 없이 동작하는 기본 방어

인자 없이 `verify`(레벨 `3`: `suspicious & ai`)나 `block`만 선언해도 세 계층이 바로 동작합니다.

1. **내장 기본 규칙 (`default_rules on`)**: `/.env`, `/.git/*`, `/.aws/*`, `/wp-admin/*`, `sqlmap`/`nikto`/`nuclei`, SQLi/XSS 쿼리 프로브에 `403`을 반환하고, CLI 및 헤드리스 HTTP 클라이언트는 PoW 화면으로 보냅니다.
2. **위협 피드, 실시간 IP 평판, VPN/Tor 탐지**: 시작 시 및 6시간마다 IPsum(level 3+), Spamhaus DROP v4/v6, Feodo Tracker, X4BNet VPN 목록, Tor 출구 노드를 갱신하고 Shodan InternetDB, StopForumSpam, `ipquery.io`, `ipapi.is`를 fail-open 타임아웃으로 조회합니다. VPN/프록시/Tor IP는 HTTP, SSH, Minecraft 전반에서 레벨 `1`과 `3`의 의심 대상으로 분류됩니다.
3. **공식 AI 크롤러 IP 목록**: OpenAI, Anthropic, Perplexity, Google, Apple의 공개 CIDR 대역을 대조해 Chrome `User-Agent`로 위장한 봇도 식별합니다.
