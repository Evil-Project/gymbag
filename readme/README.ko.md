# Gymbag

<p align="center">
  <img src="../docs/public/mascot-happy.png" alt="Gymbag mascot" width="140" />
</p>

<p align="center">
  <strong>순수 Go로 작성된 단일 바이너리 HTTP/HTTPS, SSH &amp; Minecraft 작업 증명(PoW) 게이트웨이</strong>
</p>

<p align="center">
  <a href="https://github.com/Evil-Project/gymbag/releases"><img src="https://img.shields.io/github/v/release/Evil-Project/gymbag?include_prereleases&label=Release" alt="Release" /></a>
  <a href="https://github.com/Evil-Project/gymbag/pkgs/container/gymbag"><img src="https://img.shields.io/badge/Docker-GHCR-blue?logo=docker" alt="Docker GHCR" /></a>
  <a href="../go.mod"><img src="https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go" alt="Go 1.27+" /></a>
  <a href="../LICENSE"><img src="https://img.shields.io/badge/License-Apache--2.0-green" alt="Apache-2.0 License" /></a>
  <a href="https://evil-project.github.io/gymbag/ko/"><img src="https://img.shields.io/badge/Docs-VitePress-b81d33" alt="Docs" /></a>
</p>

<p align="center">
  <strong>README:</strong>
  <a href="../README.md">English</a> ·
  <a href="README.zh-CN.md">简体中文</a> ·
  <a href="README.zh-TW.md">繁體中文</a> ·
  <a href="README.ja.md">日本語</a> ·
  <a href="README.ko.md">한국어</a> ·
  <a href="README.es.md">Español</a>
  <br />
  <sub>온라인 문서: <a href="https://evil-project.github.io/gymbag/">EN</a> · <a href="https://evil-project.github.io/gymbag/zh/">ZH</a> · <a href="https://evil-project.github.io/gymbag/zh-TW/">ZH-TW</a> · <a href="https://evil-project.github.io/gymbag/ja/">JA</a> · <a href="https://evil-project.github.io/gymbag/ko/">KO</a> · <a href="https://evil-project.github.io/gymbag/es/">ES</a></sub>
</p>

Gymbag은 Anubis 계열의 작업 증명(PoW) WAF가 내장된 단일 바이너리 HTTPS 리버스 프록시이자 SSH·Minecraft TCP 보호 게이트웨이입니다. 서드파티 Go 모듈 없이 Go 표준 라이브러리만으로 작성되었고, HTML 페이지, 브라우저 SHA-256 솔버, 마스코트 이미지까지 모두 `go:embed`로 바이너리에 포함됩니다.

셀프 호스팅 앱, Git 포지, SSH 포트, Minecraft 서버를 운영하다 보면 마주치는 트래픽은 비슷합니다. `robots.txt`를 무시하는 LLM 스크래퍼, `/.env`와 `/wp-admin`을 찌르는 봇, SSH 무차별 대입 시도, 로그인 플러딩이 계속 들어옵니다. nginx나 Caddy에 PoW 사이드카, fail2ban, 직접 짠 방화벽 스크립트를 따로 붙여 운영하는 대신, Gymbag은 TLS 종단, 멀티 CA ACME, HTTP/WebSocket 프록시, SSH/Minecraft TCP 게이트, IP 위협·VPN·AI 목록, 방화벽 규칙을 한 프로세스에서 처리하며 `gymbag.json` 또는 Caddy 스타일의 `Gymbagfile`로 설정합니다.

## 빠른 시작

[GitHub Releases](https://github.com/Evil-Project/gymbag/releases)에서 정적 바이너리(Linux, macOS, Windows용 `CGO_ENABLED=0` `amd64`/`arm64`), Debian/Ubuntu `.deb` 패키지, Arch `.pkg.tar.zst` 패키지, NixOS 타볼/NAR를 배포합니다. GHCR 컨테이너 이미지나 Nix flake를 쓰거나 소스에서 직접 빌드할 수도 있습니다.

```bash
go build -o gymbag ./cmd/gymbag
```

**Debian / Ubuntu:**
```bash
sudo apt install ./gymbag_<version>_linux_amd64.deb
sudo gymbag init -o /etc/gymbag/Gymbagfile --format gymbagfile
sudo systemctl enable --now gymbag
```

**Arch Linux:**
```bash
sudo pacman -U gymbag-<version>-1-x86_64.pkg.tar.zst
# 또는 packaging/arch에서 직접 빌드:
cd packaging/arch && makepkg -si
```

**Nix / NixOS:**
```bash
nix run github:Evil-Project/gymbag -- --help
nix profile install github:Evil-Project/gymbag
```
정적 Linux 릴리스 바이너리는 NixOS에서도 `patchelf`나 `nix-ld` 없이 그대로 실행되며, `nixosModules.default`를 통해 선언형 `services.gymbag` 모듈을 사용할 수 있습니다.

두 줄이면 설정을 만들고(기본 `gymbag.json`, `--format gymbagfile` 지정 시 `Gymbagfile`) 바로 실행할 수 있습니다.

```bash
# HTTP/HTTPS 웹사이트 보호
./gymbag init -d app.example.com -u http://localhost:3000
./gymbag

# SSH 보호 (sshd를 임의 포트로 옮기고 방화벽 규칙을 자동 설정)
sudo ./gymbag init -t ssh
sudo ./gymbag

# Minecraft Java Edition 보호 (자동 선택된 Java로 server.jar 자동 시작 + 포트 무작위화)
./gymbag init -t minecraft --server-jar /srv/mc/paper.jar --mc-jvm-args "-Xms1G -Xmx4G"
./gymbag

# 관리형 시작 없이 기존 Minecraft 서버 보호 (server.properties 포트 무작위화)
./gymbag init -t minecraft --server-properties /path/to/server.properties
./gymbag

# 한 프로세스에서 세 가지 모두 보호
sudo ./gymbag init -t http,ssh,minecraft -d app.example.com -u :3000 --server-jar /srv/mc/paper.jar
sudo ./gymbag
```

기존 **nginx**(`nginx.conf`), **Apache**(`httpd.conf` / `apache2.conf`), **Caddyfile v2** 설정을 그대로 지정하거나 시스템 서비스(`systemd`, `launchd`, `openrc`, `supervisor`)로 등록할 수도 있습니다.

```bash
./gymbag init -d app.example.com -u :3000 --service auto
./gymbag service start
```

설정 파일을 만들지 않고 바로 테스트할 때는 `reverse-proxy`를 씁니다.

```bash
./gymbag reverse-proxy --from app.internal --to :3000 --verify 3
```

> **1024 미만 포트 참고:** `:80`은 HTTP→HTTPS 리다이렉트 및 ACME HTTP-01을, `:443`은 HTTPS를, `:22`는 SSH를 수신합니다. Linux에서 `sudo setcap 'cap_net_bind_service=+ep' ./gymbag`를 실행하면 root 권한 없이도 이 포트들에 바인딩할 수 있습니다(`.deb`와 Arch 패키지에는 이 capability가 기본 적용됩니다). 로컬에서 비권한 포트로 웹 테스트를 하려면 `--http-port 8080 --https-port 8443`을 전달하세요.

### 규칙을 작성하지 않아도 동작하는 기본 계층

트래픽 앞에 Gymbag을 두기 전에 정규식이나 차단 목록을 미리 작성할 필요는 없습니다. `verify`(레벨 `3`: `suspicious & ai`)나 `block`(위협 인텔 일치 시 챌린지 대신 `403` 반환)만 써 두면 다음 세 계층이 바로 동작합니다.

1. **내장 기본 규칙 (`default_rules on`)**
   - **민감한 경로 (`403`)**: `/.env`, `/.env.*`, `/.git`, `/.git/*`, `/.svn/*`, `/.hg/*`, `/.DS_Store`, `/.aws/*`, `/.ssh/*`, `/docker-compose.yml`, `/docker-compose.yaml`, `*.bak`, `*.sql`, `*.sqlite`
   - **CMS 및 프레임워크 탐색 (`403`)**: `/wp-admin`, `/wp-admin/*`, `/wp-login.php`, `/xmlrpc.php`, `/phpmyadmin*`, `/vendor/phpunit*`, `/cgi-bin/*`, `/actuator/env*`, `/actuator/heapdump*`
   - **취약점 스캐너 (`403`)**: `sqlmap`, `nikto`, `nuclei`, `masscan`, `zgrab`, `gobuster`, `dirbuster`, `ffuf`, `wfuzz`, `feroxbuster`, `acunetix`, `nessus`, `openvas`
   - **인젝션 및 경로 탐색 쿼리 (`403`)**: 쿼리 문자열 내 `UNION SELECT`, `<script`, `javascript:`, `/etc/passwd`, `information_schema`, `../`, `cmd.exe`, `/bin/sh` 차단, 널 바이트 및 `%2e%2e` 탐색에 대한 요청 형태 검사
   - **자동화 클라이언트 (PoW 챌린지)**: 사이트에 사용자 정의 규칙이 아직 없으면 CLI 라이브러리(`curl`, `wget`, `python-requests`, `python-httpx`, `go-http-client`, `aiohttp`, `okhttp`, `axios`, `node-fetch`, `scrapy`), 헤드리스 브라우저(`headlesschrome`, `playwright`, `puppeteer`, `selenium`, `phantomjs`), 빈 `User-Agent` 헤더를 작업 증명 페이지로 보냅니다.
2. **자동 갱신 IP 위협 인텔 및 VPN/Tor 탐지**
   - 시작 시 및 6시간마다 [IPsum (level 3+)](https://raw.githubusercontent.com/stamparm/ipsum/master/levels/3.txt), [Spamhaus DROP IPv4](https://www.spamhaus.org/drop/drop_v4.json) / [IPv6](https://www.spamhaus.org/drop/dropv6.txt), [abuse.ch Feodo Tracker](https://feodotracker.abuse.ch/downloads/ipblocklist.txt)를 받아 `ETag` 및 `If-Modified-Since` 헤더와 함께 `<cert_dir>/intel-cache`에 캐싱합니다.
   - **Shodan InternetDB**(`internetdb.shodan.io`)와 **StopForumSpam**(`api.stopforumspam.org`)에 API 키 없이 실시간 평판 조회를 수행하며, 1시간 인메모리 캐시와 fail-open 타임아웃을 적용해 외부 지연이 방문자를 막지 않도록 합니다.
   - **무료 VPN/프록시/Tor 탐지 (`vpn_detection on`)**: 자동 갱신되는 X4BNet VPN IPv4/IPv6 목록과 Tor 출구 노드 목록을 불러오고 `api.ipquery.io`, `api.ipapi.is`에 키 없는 실시간 조회(1시간 캐시, 1.2s fail-open 타임아웃)를 함께 수행합니다. 일치하는 IP는 HTTP, SSH, Minecraft 전역에서 검증 레벨 `1`과 `3`의 **의심(suspicious)** 대상으로 분류됩니다.
3. **공식 AI 크롤러 IP 목록**
   - **OpenAI**(`gptbot`, `searchbot`, `chatgpt-user`), **Anthropic**(`claude.com/crawling/bots.json`), **Perplexity**(`perplexitybot`, `perplexity-user`), **Google**(`user-triggered-agents`, `user-triggered-fetchers-google`), **Apple**(`applebot`)의 공개 CIDR 목록을 추적해 Chrome `User-Agent`로 위장한 크롤러도 출발지 IP로 잡아냅니다.

## SSH 및 Minecraft 보호

미인증 SSH나 Minecraft 연결이 Gymbag에 들어와 설정된 검증 레벨에 걸리면, Gymbag은 백엔드 데몬이나 게임 서버로 넘기지 않고 일회용 HTTPS 인증 링크(`/v/<token>`)를 발급합니다(SSH는 연결 해제 메시지 안에 출력하며, Minecraft 1.20.5+는 메모리 내 빈 월드 채팅으로 전송한 뒤 완료 시 `Transfer`로 실제 서버에 자동 리디렉션합니다).

- **SSH (기본 `port` `22`)**: `sudo ./gymbag init -t ssh`는 `20000–59999` 사이의 빈 로컬 포트를 고른 뒤 `/etc/ssh/sshd_config`를 `/etc/ssh/sshd_config.gymbag.bak`으로 백업하고, 첫 `Match` 블록 앞에 새 `Port`를 기록해 `sshd -t`로 검증합니다(실패 시 자동 롤백). 이어서 `sshd`를 리로드하고 활성 Linux 방화벽(`ufw`, `firewalld`, `nftables`, 또는 `iptables`+`ip6tables`)을 갱신해 백엔드 포트는 루프백에서만 접근 가능하게 막고 `22`와 `8443`만 열어 둡니다. (`sshd_config` 수정 중에는 두 번째 SSH 셸을 열어 두세요. 직접 관리하려면 `--ssh-upstream 127.0.0.1:2222`, `--no-auto-setup`, 또는 `--no-firewall`을 전달하면 됩니다.) 브라우저 챌린지를 통과한 IP는 이후 SSH 핸드셰이크를 건드리지 않고 원시 TCP 릴레이로 바로 통과합니다. 레벨 `1`과 `3`에서는 잘못된 배너나 스크립트 기반 SSH 클라이언트(`libssh`, `paramiko`, `zgrab`, `masscan`, `nmap`, `asyncssh`, `ssh2go`, `jsch`, `hydra`, `medusa`, `patator`, `python`, `curl`, `ssh-2.0-go`)에 챌린지를 띄우며, 터미널에는 RFC 4253 `SSH_MSG_DISCONNECT`와 함께 인증 URL이 표시됩니다.
- **Minecraft Java (기본 `port` `25565`)**: `./gymbag init -t minecraft --server-properties ./server.properties`는 `server.properties`를 `server.properties.gymbag.bak`으로 백업하고, `server-port`를 `20000–59999`의 빈 포트로 옮기며, 비어 있거나 와일드카드인 `server-ip`를 `127.0.0.1`로 바꿉니다. 서버 목록 핑(`nextState == 1` 및 레거시 `0xFE`)은 규칙에 의해 차단되지 않는 한 그대로 통과하므로 MOTD와 접속자 수 표시는 정상 동작합니다. 로그인이나 전송 시 인증이 필요하면 지원 클라이언트(`1.20.5`–`26.1`)는 순수 메모리 기반의 작은 빈 월드로 들어가(디스크에 아무것도 저장하지 않음) 채팅에서 클릭 가능한 인증 링크를 받고, 인증이 완료되면 `Transfer`를 통해 실제 서버로 자동 리디렉션됩니다(구버전은 **"Connection Lost"** 창으로 폴백).
- **공용 HTTPS 인증 포털 (기본 `:8443`)**: SSH와 Minecraft는 `verify_port`를 공유합니다(자체 도메인을 쓰려면 `--verify-url https://verify.example.com:8443`을 지정하고, 비워 두면 감지된 서버 IP를 사용합니다). ACME에서 서버 IP를 사용할 때는 CA가 `shortlived` 프로필을 제공하면 이를 사용해 **RFC 8738** IP 인증서를 요청하고, HTTP-01에 닿지 못하면 Gymbag 로컬 CA나 자체 서명 IP 인증서로 폴백합니다. `verify_port`가 `https_port`와 같고 HTTP 사이트가 설정되어 있으면 포털은 `443` 포트를 함께 씁니다.

```caddyfile
{
    verify_url https://verify.example.com:8443
    verify_port 8443
}

ssh {
    port 22
    upstream 127.0.0.1:41822
    verify 3
    rate_limit 60 60s
}

minecraft {
    port 25565
    upstream 127.0.0.1:48291
    server_properties /srv/mc/server.properties
    verify 3
    allow ip 192.168.1.0/24
}
```

자세한 설정과 `gymbag.json` 예시는 [SSH & Minecraft 보호 가이드](../docs/ko/guide/ssh-minecraft.md)를 참고하세요.

## 로컬 개발 (`localhost` 및 `*.internal`)

`localhost`, `*.localhost`, `*.internal`(`app.internal`, `api.internal`), `*.local`, `*.test` 도메인을 쓰는 사이트는 Gymbag이 `/etc/hosts`와 로컬 TLS 인증서를 자동으로 관리합니다.

```caddyfile
app.internal {
    reverse_proxy http://localhost:3000
    verify 3
}

api.internal {
    reverse_proxy http://localhost:8080
    protect off
}
```

### `/etc/hosts` 자동 등록

`auto_hosts on`(기본값)이 켜져 있으면 시작 시 `/etc/hosts`(Windows는 `C:\Windows\System32\drivers\etc\hosts`)를 확인해 아직 등록되지 않은 로컬 도메인에 IPv4/IPv6 루프백 항목을 추가합니다.

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

기존 `localhost` 줄은 건드리지 않습니다. `Ctrl+C` 또는 `SIGTERM`으로 종료할 때는 파일을 다시 읽어 실행 도중 사용자가 수정한 내용을 유지한 채 자신의 `# gymbag-managed:<instanceID>` 줄만 지웁니다. macOS/Linux에서는 `/etc/hosts` 쓰기에 root 권한이 필요하며, 일반 권한으로 실행하면 경고를 남기고 그대로 계속 실행됩니다. `.internal`을 `dnsmasq`나 수동 편집으로 처리한다면 `auto_hosts off`(`GYMBAG_AUTO_HOSTS=off`)로 꺼 두세요.

### 로컬 CA

매 재시작마다 임시 자체 서명 인증서를 버리는 대신, Gymbag은 `<cert_dir>/ca/`(기본 `certs/ca/`) 아래에 2계층 ECDSA P-256 CA를 유지합니다.

1. **루트 CA**: `"Neuro-sama Headquarters Root CA"` (`certs/ca/neuro-sama-root-ca.pem`, 유효기간 10년, `MaxPathLen: 1`)
2. **중간 CA**: `"Gymbag & Swarm SSL Local CA"` (`certs/ca/gymbag-swarm-local-ca.pem`, 유효기간 5년, `MaxPathLen: 0`)
3. **리프 인증서**: `certs/local-fullchain.pem` + `certs/local-privkey.pem` (유효기간 90일, 만료 30일 이내 갱신, DNS SAN과 `127.0.0.1` / `::1` 포함)

`"Neuro-sama Headquarters Root CA"`를 한 번만 신뢰 저장소에 등록하면 브라우저와 `curl`이 모든 로컬 사이트를 신뢰합니다.

```bash
# macOS
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain certs/ca/neuro-sama-root-ca.pem

# Debian / Ubuntu
sudo cp certs/ca/neuro-sama-root-ca.pem /usr/local/share/ca-certificates/neuro-sama-root-ca.crt && sudo update-ca-certificates

# Fedora / RHEL / Arch Linux
sudo trust anchor --store certs/ca/neuro-sama-root-ca.pem
```

> **Firefox:** macOS/Windows(및 `p11-kit`을 쓰는 Linux)에서는 `about:config`의 `security.enterprise_roots.enabled`를 `true`로 설정하거나, *설정 -> 개인 정보 및 보안 -> 인증서 -> 인증서 보기 -> 인증 기관*에서 `certs/ca/neuro-sama-root-ca.pem`을 한 번 가져오세요.

## 다중 사이트 및 검증 레벨 (`0`–`3`)

한 프로세스에서 라운드 로빈 업스트림 부하 분산과 RFC 6455 WebSocket 업그레이드를 지원하며 여러 도메인을 처리합니다. 사이트(및 SSH/Minecraft 서비스)마다 검증 레벨(`0`–`3`)을 고르거나 `protect off`로 PoW를 끌 수 있습니다.

| 레벨 | 이름 | 챌린지 대상 | 주요 용도 |
| :---: | :--- | :--- | :--- |
| `0` | `always` | 유효한 클리어런스 쿠키나 인증된 IP가 없는 모든 요청 | 내부 관리 도구, 스테이징 앱, L7 플러딩 대응 |
| `1` | `only suspicious` | 위협 피드, 실시간 저평판 IP, VPN/프록시/Tor IP, 스캐너, CLI 라이브러리, 헤드리스 브라우저, 헤더 누락/위장, 익스플로잇 프로브 | 일반 브라우저에 챌린지를 띄우지 않으려는 공개 앱 및 API |
| `2` | `only ai` | 공식 AI 크롤러 IP 대역 또는 AI 봇 `User-Agent` / `X-Ai-Agent` / `Sec-Agent` 헤더 | 사람과 `curl`은 통과시키고 AI 스크래퍼만 막으려는 문서, 블로그, Git 포지 |
| `3` | `suspicious & ai` | `1`과 `2`를 결합 (인자 없는 `verify`, SSH, Minecraft의 기본값) | 일반 공개 운영 웹사이트 |
| — | `protect off` | PoW 챌린지 없이 순수 프록시로 동작 (명시된 사용자 지정 규칙은 실행됨) | 정적 에셋 호스트, 웹훅, 미러 |

```caddyfile
app.example.com, www.example.com {
    reverse_proxy http://localhost:3000 http://localhost:3001
    verify 3
    difficulty 4
}

docs.example.com {
    reverse_proxy http://localhost:4000
    verify 2
}

api.example.com {
    reverse_proxy http://localhost:8080
    verify 1
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
    verify_timeout 30m
}

static.example.com {
    reverse_proxy http://localhost:9000
    protect off
}
```

SHA-256 챌린지(`difficulty` `1`–`5`, 기본 `4`)를 통과하면 호스트와 클라이언트 IP에 바인딩된 `HttpOnly`, `SameSite=Lax` HMAC 클리어런스 쿠키(`waf_clearance`, 기본 `24h` / `86400s`)가 발급됩니다. 전역 또는 사이트별 유지 시간은 `verify_timeout` / `clearance_ttl`(초 또는 `30m`, `12h`, `1h30m`, `7d`, `2w` 형식), `--verify-timeout`, `GYMBAG_VERIFY_TIMEOUT`, `PATCH /api/v1/sites/{domain}`으로 바꿀 수 있습니다. 인증을 통과한 방문자는 `GET /.waf/v1/session`에서 `clearance_ttl`, `expires_at`, `expires_in`을 조회할 수 있습니다.

## 경로 라우트, 정적 파일, 리다이렉트 및 설정 가져오기

사이트 블록 안에서 `reverse_proxy`와 함께 순서가 있는 **경로 라우트**를 정의할 수 있습니다. 첫 번째로 일치하는 라우트가 요청을 처리하고, 일치하는 라우트가 없으면 사이트 기본 업스트림으로 넘어갑니다(기본 업스트림이 없으면 `404`). 각 라우트는 리버스 프록시, 정적 파일 서버(`root`), 정적 `respond`, `redir` 중 하나의 핸들러를 갖습니다.

```caddyfile
app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    handle_path /static/* {
        root ./dist
        file_server browse
    }
    @health path /healthz /readyz
    respond @health "ok" 200
    redir /old/* https://{host}/new{uri} 301
    handle /api/* {
        reverse_proxy http://localhost:8080
        header X-Served-By gymbag
    }
    try_files {path} /index.html
}
```

경로 매처는 정확한 경로(`/exact`), 글롭(`/api/*`, `*.php`), Go RE2 정규식(`~^/v[0-9]+/`)을 받습니다. 리다이렉트는 `{scheme}`, `{host}`, `{uri}`, `{path}`, `{query}` 치환자를 확장합니다. 정적 라우트는 `index`, `browse`, SPA `fallback`을 지원하며, `header`는 응답 헤더를 설정하거나 앞에 `-`를 붙여 제거합니다. 방화벽 규칙과 검증은 모든 라우트 핸들러보다 먼저 실행됩니다.

`gymbag.json`에서 동일한 `routes` 배열은 다음과 같습니다.

```json
{
  "routes": [
    {"path": "/static/*", "strip_prefix": "/static", "root": "./dist", "browse": true},
    {"path": "/old/*", "redirect": {"to": "https://{host}/new{uri}", "code": 301}},
    {"path": "/api/*", "upstreams": ["http://localhost:8080"]}
  ]
}
```

라우트는 `gymbag config route list|add|update|remove` 또는 `PATCH /api/v1/sites/{domain}`으로 관리합니다.

**기존 설정 가져오기:** `gymbag run -c /etc/nginx/nginx.conf`(또는 `httpd.conf` / `apache2.conf`)로 네이티브 nginx나 Apache 설정을 바로 실행할 수 있고, `gymbag config convert -c <file> -o gymbag.json --format json`으로 nginx, Apache, Caddyfile v2(스니펫, `import`, `@matchers`, `handle`, `route`, `file_server`, `respond`, `redir`, `header` 포함)를 변환할 수 있습니다.

## SSL 공급자, 사용자 지정 키 및 갱신

공용 도메인은 기본값인 `tls auto`로 Let's Encrypt ACME HTTP-01 인증서를 받습니다. CA 전환은 `ssl_provider`(별칭: `acme_provider`, `ca`, `cert_issuer`) 또는 `tls <provider>`로 지정합니다.

| 공급자 키워드 | 인증 기관 (CA) | EAB 요구사항 |
| :--- | :--- | :--- |
| `letsencrypt` (`le`) | Let's Encrypt (운영) | 없음 |
| `letsencrypt_staging` | Let's Encrypt (스테이징) | 없음 |
| `zerossl` | ZeroSSL | `email` 설정 시 자동 발급, 또는 `eab <kid> <hmac_key>` |
| `google` (`gts`) | Google Trust Services | `eab <kid> <hmac_key>` 필요 |
| `ssl.com` (`sslcom`, `ssl.com-rsa`) | SSL.com (ECC 또는 RSA) | `eab <kid> <hmac_key>` 필요 |
| `litessl` (`trustasia`) | LiteSSL by TrustAsia | 표준 DV는 불필요 |
| `buypass` | Buypass Go SSL | 없음 |
| `local` | 로컬 2계층 PKI | 로컬 CA (`Neuro-sama Headquarters Root CA`) |
| `https://...` | 임의의 RFC 8555 ACME v2 CA | 선택적 `eab <kid> <hmac_key>` (예: `step-ca`, Pebble) |

**ZeroSSL**은 `eab`를 직접 주지 않고 `email`만 설정해도 EAB 자격 증명을 자동으로 받아옵니다. **Google Trust Services**와 **SSL.com**은 `eab YOUR_EAB_KEY_ID YOUR_EAB_HMAC_KEY`를 전달하면 되며(패딩 base64와 비패딩 base64url 모두 지원), Admin API 출력에서는 `eab_hmac_key`가 마스킹됩니다. ACME 인증서는 공급자별로 캐시되므로(`certs/acme-zerossl-fullchain.pem`, Let's Encrypt는 `certs/acme-fullchain.pem`), `ssl_provider`를 바꾸면 새 CA에서 즉시 인증서를 요청합니다.

직접 보유한 PEM 파일을 사용하거나 `acme.sh` 연동, 자동 SSL 비활성화, 또는 HTTP 직접 제공도 가능합니다.
- `account_key /etc/gymbag/account.pem`: 기존 ECDSA(P-256/P-384) 또는 RSA ACME 계정 키
- `ssl_key /etc/gymbag/domain.key`: ACME CSR 서명용 기존 ECDSA 또는 RSA 개인 키
- `tls <cert> <key>`: 전역 또는 사이트별(SNI) 외부 관리 인증서, 암호화된 키는 `key_password`(`GYMBAG_KEY_PASSWORD`) 사용
- `acme_sh_path ~/.acme.sh`(`--acme-sh-path` / `GYMBAG_ACME_SH_PATH`): `acme.sh` 홈 디렉터리(`<domain>_ecc/` 우선, `<domain>/`, 와일드카드 디렉터리 지원) 또는 단일 도메인 디렉터리에서 인증서를 직접 불러오며, 전역 또는 사이트별(`tls acme.sh <path>`)로 적용되고 갱신 시 자동으로 핫 리로드됩니다.
- `auto_ssl off` / `tls off`(`--no-auto-ssl` / `--http-only`): ACME, 로컬 CA, 자체 서명 인증서 자동 발급을 중단합니다. `auto_ssl off`를 `acme_sh_path` 또는 `tls <cert> <key>`와 함께 사용하면 사용자 관리 인증서만 제공하며, `tls off`는 HTTPS 리스너를 완전히 비활성화합니다.
- `direct_http on`(`--direct-http` / `http://example.com` / `GYMBAG_DIRECT_HTTP=on`): 보호된 사이트를 `http_port`의 일반 HTTP로 직접 제공합니다(`308` 리다이렉트를 강제하거나 HTTP에서 HSTS를 전송하지 않고 전체 WAF, PoW 쿠키, 프록시 및 `http://` 리다이렉트 재작성 동작).

백그라운드 태스크가 `renew_interval`초(기본 `43200` / 12시간)마다 인증서를 확인해 `renew_within_days`(`30`일) 이내에 만료되는 인증서를 갱신하거나 다시 불러오며, 활성 HTTP 스트림이나 WebSocket 연결을 끊지 않고 `tls.Certificate`를 원자적으로 교체합니다.

## 사용자 지정 방화벽 규칙 작성

보호된 HTTP 요청은 다음 순서대로 검사를 거칩니다.

1. 요청 형식 검사 (HTTP 메서드, 최대 URI 길이, 널 바이트 및 `..` / `%2e%2e` 경로 탐색 거부)
2. ACME HTTP-01 (`/.well-known/acme-challenge/*`)
3. 로컬 사용자 지정 규칙 (전역 규칙 후 사이트 규칙, 위에서 아래로 첫 일치 적용)
4. 원격 규칙 (`remote_rules`, HTTP/HTTPS로 자동 갱신)
5. 내장 기본 규칙 (`default_rules off`가 아닌 경우)
6. 사이트별·IP별 속도 제한 (`rate_limit` / `rate_window`)
7. IP 위협 인텔리전스 (`ip_security_action verify` 또는 `block`)
8. 작업 증명 검사 (`verify` `0`–`3`)

사용자 지정 규칙이 기본 규칙, 속도 제한, 위협 피드보다 먼저 실행되므로 사이트 블록 상단에 `allow`를 두면 사내 CIDR, 헬스 체크, 미리보기 쿠키를 간단히 예외 처리할 수 있습니다.

### 단축 규칙

문법: `<allow | verify | block> <field> <pattern...>` (`bypass`/`pass`, `challenge`/`pow`, `deny`/`drop`/`reject` 별칭도 지원)

| 필드 | 값 및 매칭 방식 | 예시 |
| :--- | :--- | :--- |
| `ip` | IPv4, IPv6 또는 CIDR | `allow ip 127.0.0.1 10.0.0.0/8 2001:db8::/32` |
| `user_agent` | 대소문자 무관 부분 문자열, `*` 글롭 또는 `~regex` | `block user_agent BadBot ~^(?i)python-urllib/` |
| `path` | 정확/부분 문자열, `*` 글롭 또는 `~regex` | `verify path /api/export* ~^/v[0-9]+/search` |
| `query` | 원시/디코딩 쿼리 일치, `*` 글롭 또는 `~regex` | `block query "union select" ~(?i)<script` |
| `method` | HTTP 메서드 (대소문자 무관) | `verify method POST DELETE` |
| `header` | `<Header-Name> [pattern]` (`""` = 누락/비어 있음, `*` = 존재) | `verify header Accept-Language ""` |
| `cookie` | `<cookie_name> [pattern]` (`""` = 누락/비어 있음, `*` = 존재) | `allow cookie vip_session *` |

### 다중 조건 `rule` 블록

`rule "name" { ... }` 안에서는 같은 필드의 여러 값이 OR로, 서로 다른 필드가 AND로 결합됩니다.

```caddyfile
app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    rule "challenge-auth-mutations" {
        action verify
        method POST
        path /login* /api/auth/* /reset-password*
    }

    rule "block-headless-api-posts" {
        action block
        method POST
        path /api/*
        header Origin ""
    }
}
```

### 원격 규칙 및 동적 로컬 규칙 파일

`remote_rules`에 Gymbagfile 규칙 문법이나 JSON(`[ ... ]` 또는 `{"rules": [ ... ]}`)을 응답하는 HTTP/HTTPS URL을 전역 또는 사이트별로 지정할 수 있습니다. 검증을 통과한 페이로드는 `<cert_dir>/rules-cache`에 `ETag` / `If-Modified-Since` 조건부 요청으로 캐시되며, 형식이 잘못된 업데이트가 와도 직전 규칙 세트를 유지합니다.

로컬 `.rules` 또는 `.json` 파일과 폴더도 `rules_file`, `rules_dir`, `ruleset` 블록(또는 `gymbag ruleset add`)으로 불러올 수 있습니다. `dynamic`을 붙이면 `rules_watch_interval`(기본 `2s`)마다 변경 사항을 감지해 핫 리로드합니다.

```caddyfile
{
    remote_rules https://rules.example.com/org-baseline.rules
    remote_rules_refresh 5m
    rules_watch_interval 2s
}

app.example.com {
    reverse_proxy http://localhost:3000
    rules_file ./rules/custom.rules dynamic
    rules_dir ./rules.d dynamic
}
```

`default_rules`의 기본값은 `on`입니다. 사이트에 사용자 지정 규칙을 추가해도 사용자 규칙 뒤에 내장 **block** 규칙이 계속 붙으므로 `verify path` 규칙 하나를 추가했다고 `/.env`가 열리지 않습니다. 내장 규칙을 완전히 끄려면 `default_rules off`를 지정하세요.

## CLI, Docker Compose, 환경 변수 및 REST API

### CLI 레퍼런스

Gymbag은 작업 디렉터리에 `.env`가 있으면 먼저 읽은 뒤 `./gymbag.json`, `./Gymbagfile`, `./Caddyfile`을 차례로 찾습니다(`-c`로 JSON, Gymbagfile, Caddyfile v2, `nginx.conf`, Apache 설정 지정 가능).

```bash
# 설정 생성 (기본값 gymbag.json, --format gymbagfile 지정 시 Gymbagfile)
./gymbag init
./gymbag init -d app.example.com -u http://localhost:3000 -l 3 --rules-dir ./rules.d --rules-dynamic
./gymbag init -t ssh --verify-url https://verify.example.com:8443
./gymbag init -t minecraft --server-properties ./server.properties
./gymbag init -t http,ssh,minecraft -d app.example.com -u :3000 --server-properties ./server.properties
./gymbag init --format gymbagfile -o Gymbagfile -d app.example.com -u :3000

# 사이트, 라우트, 전역 설정 관리 및 설정 변환
./gymbag config list
./gymbag config add -d api.example.com -u :8080 -l 1
./gymbag config update -d api.example.com -l 3 --verify-timeout 6h
./gymbag config route add -d api.example.com --path /static/* --strip-prefix /static --root ./dist
./gymbag config remove -d api.example.com
./gymbag config set admin 127.0.0.1:2019
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json

# 규칙 파일, 폴더, 개별 규칙 관리
./gymbag ruleset init ./rules/custom.rules
./gymbag ruleset add --source ./rules --dynamic --name local-dir
./gymbag ruleset update local-dir --static
./gymbag ruleset remove local-dir
./gymbag ruleset validate ./rules
./gymbag ruleset rule add --file ./rules/custom.rules --action block --name bad-bot --ua "*BadBot*"

# 실행, 감시, 검증, 포맷팅, OS 서비스 관리
./gymbag
./gymbag run --config gymbag.json --watch
./gymbag service install --config gymbag.json
./gymbag service start
./gymbag service render --type systemd
./gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3
./gymbag validate --config gymbag.json
./gymbag fmt --overwrite Gymbagfile
./gymbag version
```

### Docker 및 Docker Compose

`ghcr.io/evil-project/gymbag`에서 `cap_net_bind_service=+ep`가 부여된 UID `10001` 기반 멀티 아키텍처 Alpine 이미지(`linux/amd64`, `linux/arm64`)를 제공합니다.

```bash
# Gymbagfile 마운트로 빠른 docker run 실행
docker run -d \
  --name gymbag \
  --restart unless-stopped \
  -p 80:80 -p 443:443 \
  --cap-add NET_BIND_SERVICE \
  --add-host=host.docker.internal:host-gateway \
  -v "$(pwd)/Gymbagfile:/app/Gymbagfile:ro" \
  -v gymbag-certs:/app/certs \
  ghcr.io/evil-project/gymbag:latest
```

`compose.yaml`은 Gymbag과 `redis:8-alpine`을 함께 띄우고 순수 Go RESP2 Redis 클라이언트로 PoW 챌린지 논스, 고정 윈도우 속도 제한, SSH/Minecraft 인증 티켓과 IP 인증 기록을 재시작 및 레플리카 간에 공유합니다(`docker compose up -d --build`). Docker에서 SSH나 Minecraft를 보호할 때는 `-p 22:22` / `-p 25565:25565`와 `-p 8443:8443`도 함께 여세요. 바인드 마운트 권한(`chown 10001:10001`), `--watch`, 환경 변수 전용 구성은 [Docker & Docker Compose 가이드](../docs/ko/guide/docker.md)를 참고하세요.

### 환경 변수 (`GYMBAG_*`)

적용 우선순위는 **설정 파일 -> `.env` / `GYMBAG_*` 환경 변수 -> CLI 플래그**입니다.

| 변수 | 설명 |
| :--- | :--- |
| `GYMBAG_CONFIG` | `gymbag.json`, `Gymbagfile`, `Caddyfile`, `nginx.conf`, Apache 설정 파일 경로 |
| `GYMBAG_BIND` | 바인드 IP (로컬 전용 HTTP는 `127.0.0.1`, 공용 도메인·SSH·Minecraft 활성 시 `0.0.0.0`) |
| `GYMBAG_HTTP_PORT` / `GYMBAG_HTTPS_PORT` | HTTP 및 HTTPS 수신 포트 (기본 `80` 및 `443`) |
| `GYMBAG_DOMAIN` / `GYMBAG_DOMAINS` | 쉼표로 구분된 도메인 목록 |
| `GYMBAG_UPSTREAM` | 기본 업스트림 대상 URL |
| `GYMBAG_UPSTREAMS` | 세미콜론 구분 호스트 맵 (`app.example.com=http://web:3000;api.example.com=http://api:8080`) |
| `GYMBAG_PROTECT` / `GYMBAG_PROTECT_DOMAINS` | WAF 보호 켜기/끄기(`on`/`off`), 또는 보호할 도메인 목록 제한 |
| `GYMBAG_VERIFY_LEVEL` | 검증 레벨 `0`–`3` 또는 `off` (별칭: `GYMBAG_VERIFY`, `GYMBAG_VERIFY_MODE`, `GYMBAG_MODE`, `GYMBAG_TRIGGER`) |
| `GYMBAG_DEFAULT_RULES` | 내장 기본 규칙 (`on` / `off`, 기본 `on`) |
| `GYMBAG_REMOTE_RULES` / `GYMBAG_REMOTE_RULES_REFRESH` | 원격 방화벽 규칙 URL 및 새로고침 주기 (기본 `300`s / `5m`) |
| `GYMBAG_RULES_WATCH_INTERVAL` | `dynamic` 로컬 규칙 파일/폴더 폴링 주기 (기본 `2`s / `2s`) |
| `GYMBAG_DIFFICULTY` | SHA-256 PoW 난이도 (`1`–`5`, 기본 `4`) |
| `GYMBAG_CHALLENGE_TTL` / `GYMBAG_CLEARANCE_TTL` | 챌린지 만료(`300`s / `5m`) 및 인증 유지 시간(`86400`s / `24h`; 별칭: `GYMBAG_VERIFY_TIMEOUT`, `GYMBAG_VERIFIED_TIMEOUT`) |
| `GYMBAG_COOKIE_NAME` / `GYMBAG_COOKIE_SECURE` | 클리어런스 쿠키 이름(`waf_clearance`) 및 `Secure` 플래그(`true`) |
| `GYMBAG_RATE_LIMIT` / `GYMBAG_RATE_WINDOW` | IP당 요청 제한(`240`) 및 윈도우(`60`s) |
| `GYMBAG_TLS_MODE` / `GYMBAG_SSL_PROVIDER` | TLS 모드(`auto`, `acme`, `local`, `selfsigned`, `file`, `off`) 및 CA(`letsencrypt`, `letsencrypt_staging`, `zerossl`, `google`, `ssl.com`, `litessl`, `buypass`, `local`, 또는 URL) |
| `GYMBAG_AUTO_SSL` / `GYMBAG_ACME_SH_PATH` | 자동 SSL 토글(`on`/`off`) 및 `acme.sh` 인증서 디렉터리 설정(`GYMBAG_ACME_SH` 별칭) |
| `GYMBAG_DIRECT_HTTP` / `GYMBAG_HTTP_ONLY` | HTTP에서 직접 사이트 제공(`on`/`off`, 별칭 `GYMBAG_ALLOW_HTTP`) 또는 HTTPS 완전 비활성화(`on`/`off`) |
| `GYMBAG_ACME_EMAIL` / `GYMBAG_ACME_DIRECTORY` | ACME 연락처 이메일(ZeroSSL 자동 EAB 활성화) 및 명시적 ACME 디렉터리 URL |
| `GYMBAG_EAB_KID` / `GYMBAG_EAB_HMAC_KEY` | External Account Binding Key ID 및 base64/base64url HMAC 키 |
| `GYMBAG_ACCOUNT_KEY_FILE` | ECDSA/RSA ACME 계정 키 PEM 경로 |
| `GYMBAG_CERT_FILE` / `GYMBAG_KEY_FILE` / `GYMBAG_KEY_PASSWORD` | `file` 모드용 인증서, 키, 암호화된 키 암호 |
| `GYMBAG_CERT_DIR` | 인증서, CA 키, 시크릿, 인텔 캐시, 규칙 캐시 디렉터리 (기본 `certs`) |
| `GYMBAG_RENEW_INTERVAL` / `GYMBAG_RENEW_WITHIN_DAYS` | 갱신 확인 주기(`43200`s) 및 임계 일수(`30`일) |
| `GYMBAG_AUTO_HOSTS` / `GYMBAG_HSTS` | `/etc/hosts` 자동 관리(`on`/`off`) 및 `Strict-Transport-Security` 재정의(`on`/`off`) |
| `GYMBAG_IP_SECURITY` / `GYMBAG_IP_SECURITY_ACTION` | 위협 피드(`on`/`off`) 및 일치 동작(`verify` 또는 `block`) |
| `GYMBAG_IP_REPUTATION` / `GYMBAG_AI_IP_LIST` | Shodan InternetDB + StopForumSpam 실시간 조회(`on`/`off`) 및 공식 AI 크롤러 CIDR 목록(`on`/`off`) |
| `GYMBAG_VPN_DETECTION` | 의심 판정 시 무료 VPN/프록시/Tor 탐지(`on`/`off`, 기본 `on`) |
| `GYMBAG_INTEL_REFRESH` | 위협, VPN, AI 피드 갱신 주기 (기본 `21600`s / `6h`) |
| `GYMBAG_VERIFY_URL` / `GYMBAG_VERIFY_PORT` | 공용 HTTPS 인증 포털 URL(빈 값 = 서버 IP + IP 인증서) 및 포트(기본 `8443`) |
| `GYMBAG_SSH_ENABLED` / `GYMBAG_SSH_PORT` / `GYMBAG_SSH_UPSTREAM` | SSH 보호 활성화(기본 포트 `22`) 및 백엔드 `sshd` 대상 |
| `GYMBAG_SSH_VERIFY_LEVEL` / `GYMBAG_SSH_PROTECT` | SSH 검증 레벨(`0`–`3` 또는 `off`) 및 보호 토글(`on`/`off`) |
| `GYMBAG_MC_ENABLED` / `GYMBAG_MC_PORT` / `GYMBAG_MC_UPSTREAM` | Minecraft 보호 활성화(기본 포트 `25565`) 및 백엔드 대상(`GYMBAG_MINECRAFT_*` 별칭 지원) |
| `GYMBAG_MC_SERVER_JAR` / `GYMBAG_MC_JAVA` | Minecraft `server.jar` 경로(자동 선택된 Java로 서버 자동 시작) 및 선택적 Java 바이너리 재정의 |
| `GYMBAG_MC_JVM_ARGS` / `GYMBAG_MC_WORK_DIR` / `GYMBAG_MC_AUTO_RESTART` | 공백으로 구분된 JVM 플래그, Minecraft 작업 디렉터리, 비정상 종료 시 자동 재시작 토글(`true`/`false`, 기본 `true`) |
| `GYMBAG_MC_VERIFY_LEVEL` / `GYMBAG_MC_PROTECT` | Minecraft 검증 레벨(`0`–`3` 또는 `off`) 및 보호 토글(`on`/`off`) |
| `GYMBAG_MASCOT` | 사용자 지정 마스코트 파일, 디렉터리 또는 `http(s)://` URL(상태별 재정의: `GYMBAG_MASCOT_PENSIVE`, `GYMBAG_MASCOT_HAPPY`, `GYMBAG_MASCOT_REJECT`) |
| `GYMBAG_REDIS_URL` / `GYMBAG_SECRET` / `GYMBAG_ADMIN` | Redis URL(`redis://` 또는 `rediss://`), HMAC 시크릿, Admin API 수신 주소(예: `127.0.0.1:2019` 또는 `off`) |

### REST API

**공개 WAF 및 인증 포털 엔드포인트:**

| 메서드 | 경로 | 설명 |
| :--- | :--- | :--- |
| `GET` | `/.waf/v1/session` | 인증 상태(`cleared`), `protected`, `verify_level`, `verify_mode`, `clearance_ttl`, `expires_at`, `expires_in` |
| `DELETE` | `/.waf/v1/session` | 방문자의 클리어런스 쿠키 제거 (`204 No Content`) |
| `POST` | `/.waf/v1/challenges` | 일회용 SHA-256 PoW 챌린지 발급 (`201 Created`) |
| `POST` | `/.waf/v1/verifications` | PoW 풀이 검증 및 서명된 클리어런스 쿠키 설정 (`/.waf/verify` 별칭 지원) |
| `GET` | `/v/{token}` | SSH/Minecraft 인증용 PoW 페이지 제공 (`/verify/{token}` 별칭 지원) |
| `POST` | `/.waf/gate/verify` | SSH/Minecraft PoW 풀이 검증 및 IP 인증 기록 저장 |
| `GET` | `/.waf/assets/{name}` | 내장 챌린지 PNG 에셋 제공 (`/.waf/v1/assets/{name}` 별칭, `ETag` 캐시) |

**로컬 Admin API (`admin 127.0.0.1:2019` 또는 `--admin`):**

Admin API는 인증 토큰이 없으며 실시간 라우팅과 방화벽 상태를 변경하므로 반드시 루프백(`127.0.0.1:2019`)이나 사설 인터페이스에만 바인딩하세요.

| 메서드 | 경로 | 설명 |
| :--- | :--- | :--- |
| `GET` | `/api/v1/health` | 상태, `sites_count`, `ssh_enabled`, `minecraft_enabled`, `tls_source`, `threat_entries`, `ai_entries`, `vpn_entries`, `remote_rules` |
| `GET` / `POST` | `/api/v1/tls` / `/api/v1/tls/renew` | 활성 TLS 상태 및 CA 카탈로그, 강제 인증서 갱신 및 원자적 교체 |
| `GET` / `POST` | `/api/v1/intel` / `/api/v1/intel/refresh` | 피드별 위협/AI/VPN 수 및 캐시 크기, 즉시 새로고침 실행 |
| `GET` / `POST` | `/api/v1/rules/remote` / `/api/v1/rules/remote/refresh` | 원격 규칙 수 및 가져오기 오류 조회, 강제 새로고침 및 WAF 리로드 |
| `GET` / `PUT` | `/api/v1/config` | 마스킹된 설정 덤프, 검증 후 활성 런타임 설정 교체 |
| `GET` / `POST` | `/api/v1/sites` | 사이트 목록 조회 및 런타임 사이트 추가 |
| `GET` / `PUT` / `PATCH` / `DELETE` | `/api/v1/sites/{domain}` | 사이트 조회, 교체, 실시간 패치(`verify_level`, `protect`, `difficulty`, `clearance_ttl`, `upstreams`, `routes`, `headers`, `mascot`, `clear_mascot`, `rules`, `remote_rules`, `rule_sources`), 삭제 |
| `GET` / `POST` / `PUT` / `DELETE` | `/api/v1/sites/{domain}/rules` 및 `.../{index_or_name}` | 사이트 사용자 지정 규칙 조회, 추가, 교체, 삭제 |

## Caddy 및 Nginx 대비 벤치마크

Apple M4(10코어, macOS, Go `1.27.0`)에서 동일한 ECDSA P-256 인증서로 TLS 1.3을 종단하고, 동시 워커 64개, `1 KiB` 응답 본문, `1024` 업스트림 keep-alive 연결 조건으로 측정했습니다(오류 `0`건).

| 서버 및 모드 | HTTP/1.1 TLS (RPS) | H1 p50 / p99 | HTTP/2 TLS (RPS) | H2 p50 / p99 | 바이너리 크기 | 부하 시 RSS |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **85,203** | 0.45 / 6.20 ms | **88,775** | 0.65 / 1.96 ms | 1.5 MB (+libs) | 5.8 MiB |
| **Gymbag `v0.1.0`** (`verify 3`, 전체 WAF 활성) | **72,517** | 0.74 / 3.14 ms | **66,096** | 0.82 / 3.48 ms | 12.9 MB (정적) | 54.3 MiB |
| **Gymbag `v0.1.0`** (`protect off`, 순수 프록시) | **72,479** | 0.74 / 3.15 ms | **64,048** | 0.83 / 3.94 ms | 12.9 MB (정적) | 52.1 MiB |
| **Gymbag `v0.1.0`** (`verify 0`, 쿠키 통과) | **65,490** | 0.79 / 3.93 ms | **62,108** | 0.84 / 4.27 ms | 12.9 MB (정적) | ~54 MiB |
| **Caddy `v2.11.9`** (Go, keep-alive 튜닝) | **54,741** | 0.90 / 4.90 ms | **39,207** | 1.30 / 6.18 ms | 49.0 MB | 76.8 MiB |

단순 처리량은 C 기반 Nginx가 가장 높지만, Gymbag은 인라인 WAF를 돌리면서도 HTTP/1.1 p99 지연을 더 낮게(`3.14 ms` vs `6.20 ms`) 유지하며 Caddy보다 HTTP/1.1에서 `+32%`, HTTP/2에서 `+63%`~`+69%` 빠르고 메모리는 약 30% 적게, 바이너리는 3.8배 작게 유지합니다. `/.env` 같은 알려진 익스플로잇 프로브는 TLS 엣지에서 **140,680 req/s**(`0.35 ms` p50)로 업스트림에 닿기 전에 드롭됩니다.

측정 방법론, `go test -bench` 마이크로벤치마크, 정확한 설정 파일은 [벤치마크 및 성능](../docs/ko/guide/benchmarks.md)에 정리되어 있습니다.

## 프로젝트 구조

```text
.
├── cmd/gymbag/          # CLI 진입점
├── docs/                # 다국어 VitePress 문서 (EN, 简体中文, 日本語, 한국어, Español)
├── internal/
│   ├── cli/             # 하위 명령어 (init, config, ruleset, service, run, reverse-proxy, validate, fmt, version), .env & --watch
│   ├── config/          # 설정 타입, Gymbagfile/Caddyfile v2/nginx/Apache 파서, 기간 & 규칙 소스, GYMBAG_* 환경 변수 로더
│   ├── host/            # 도메인 정규화, IPv6 대괄호 처리, 와일드카드 매칭
│   ├── intel/           # O(1) 프리픽스 집합, 자동 갱신 위협/VPN/AI 피드, 실시간 IP 및 VPN 평판
│   ├── localpki/        # 2계층 로컬 CA ("Neuro-sama Headquarters Root CA") & /etc/hosts 관리자
│   ├── proxy/           # 락프리 다중 사이트 & 경로 라우터, 풀링된 HTTP & RFC 6455 WebSocket 프록시
│   ├── server/          # 통합 HTTPS 게이트웨이, HTTP->HTTPS/ACME 리다이렉터, Admin REST API
│   ├── service/         # 시스템 서비스 렌더러 & 관리자 (systemd, launchd, openrc, supervisor)
│   ├── setup/           # 자동 sshd_config/방화벽 설정 및 Minecraft server.properties 포트 무작위화
│   ├── store/           # 64-샤드 인메모리 스토어, 순수 Go RESP2 Redis 클라이언트 (챌린지, 속도 제한, 게이트 티켓/클리어런스)
│   ├── tcpguard/        # SSH & Minecraft Java TCP 보호 프록시, 정책 엔진, 공용 HTTPS 인증 포털
│   ├── tlsmanager/      # 멀티 CA ACME v2 (RFC 8738 IP 인증서, EAB & ZeroSSL 자동 EAB), 로컬 PKI, 원자적 SNI 인증서 관리자
│   └── waf/             # PoW 검증기, 샤딩된 HMAC 클리어런스 캐시, Aho-Corasick DFA, 원격/로컬 규칙, 내장 UI
├── nix/                 # NixOS 모듈 (services.gymbag)
├── packaging/           # Arch Linux PKGBUILD & Debian/Ubuntu .deb 패키지 스크립트/유닛
├── Dockerfile
├── compose.yaml
├── flake.nix
├── Gymbagfile
└── go.mod
```

## Star 히스토리

<a href="https://www.star-history.com/#Evil-Project/gymbag&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
 </picture>
</a>
