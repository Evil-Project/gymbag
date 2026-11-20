# Gymbag

<p align="center">
  <img src="docs/public/mascot-happy.png" alt="Gymbag mascot" width="140" />
</p>

<p align="center">
  <strong>순수 Go로 작성된 단일 바이너리 HTTPS 리버스 프록시 &amp; 작업 증명(PoW) WAF</strong>
</p>

<p align="center">
  <a href="https://github.com/Evil-Project/gymbag/releases"><img src="https://img.shields.io/github/v/release/Evil-Project/gymbag?include_prereleases&label=Release" alt="Release" /></a>
  <a href="https://github.com/Evil-Project/gymbag/pkgs/container/gymbag"><img src="https://img.shields.io/badge/Docker-GHCR-blue?logo=docker" alt="Docker GHCR" /></a>
  <a href="go.mod"><img src="https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go" alt="Go 1.27+" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache--2.0-green" alt="Apache-2.0 License" /></a>
  <a href="https://evil-project.github.io/gymbag/ko/"><img src="https://img.shields.io/badge/Docs-VitePress-b81d33" alt="Docs" /></a>
</p>

<p align="center">
  <strong>README:</strong>
  <a href="README.md">English</a> ·
  <a href="README.zh.md">简体中文</a> ·
  <a href="README.ja.md">日本語</a> ·
  <a href="README.ko.md">한국어</a> ·
  <a href="README.es.md">Español</a>
  <br />
  <sub>온라인 문서: <a href="https://evil-project.github.io/gymbag/">EN</a> · <a href="https://evil-project.github.io/gymbag/zh/">ZH</a> · <a href="https://evil-project.github.io/gymbag/ja/">JA</a> · <a href="https://evil-project.github.io/gymbag/ko/">KO</a> · <a href="https://evil-project.github.io/gymbag/es/">ES</a></sub>
</p>

Gymbag은 Anubis 스타일의 작업 증명(Proof-of-Work) 웹 애플리케이션 방화벽이 내장된 단일 바이너리 HTTPS 리버스 프록시입니다. 외부 Go 의존성이 전혀 없는 순수 Go 표준 라이브러리만으로 작성되었으며, 모든 HTML 템플릿, 브라우저 SHA-256 솔버 스크립트, 마스코트 PNG까지 `go:embed`를 통해 실행 파일에 직접 컴파일됩니다.

웹 앱, Gitea/Forgejo 인스턴스, 문서 사이트 또는 내부 도구를 셀프 호스팅한다면 Anubis가 해결하고자 했던 문제와 똑같은 상황을 겪어보셨을 것입니다. `robots.txt`를 무시하는 공격적인 LLM 스크래퍼와 24시간 내내 `/.env` 및 `/wp-admin`을 두드리는 취약점 스캐너가 바로 그것입니다. Nginx/Caddy에 별도의 PoW 사이드카 컨테이너, fail2ban, 수동 CIDR 목록을 덕지덕지 붙이는 대신, Gymbag은 TLS 종단, 멀티 CA ACME, HTTP/WebSocket 프록시, 로컬 및 원격 방화벽 규칙, IP 위협 인텔리전스, 사이트별 PoW 정책을 깔끔한 `Gymbagfile`로 설정하는 하나의 바이너리에 모두 담았습니다.

---

## 빠른 시작 (즉시 보호 기능)

[GitHub Releases](https://github.com/Evil-Project/gymbag/releases)에서 정적 빌드 바이너리(Linux/macOS/Windows 및 Arch Linux용 `.pkg.tar.zst`)를 다운로드하거나, Nix Flakes 또는 소스에서 직접 빌드하세요:

```bash
go build -o gymbag ./cmd/gymbag
```

**Arch Linux:**
```bash
# 미리 빌드된 릴리스 패키지(.pkg.tar.zst) 설치
sudo pacman -U gymbag-0.1.0-1-x86_64.pkg.tar.zst
# 또는 makepkg로 로컬 빌드
cd packaging/arch && makepkg -si
```

**Nix / NixOS:**
```bash
# flake로 실행 또는 설치
nix run github:Evil-Project/gymbag -- --help
nix profile install github:Evil-Project/gymbag
```
정적 링크된 `CGO_ENABLED=0` Linux 릴리스 바이너리는 NixOS에서도 `patchelf`나 `nix-ld` 없이 바로 실행되며, `nixosModules.default`는 선언형 `services.gymbag` 설정을 제공합니다.

**단 두 줄의 명령어**로 `Gymbagfile`을 생성하고 서버를 시작할 수 있습니다:

```bash
# 1. 설정 생성 (대화형 마법사 또는 플래그를 통한 한 줄 설정)
./gymbag init -d app.example.com -u http://localhost:3000

# 2. 서버 시작
./gymbag
```

또는 시스템 서비스(`systemd`, `launchd`, `openrc`, `supervisor`)로 등록하여 실행할 수 있습니다:

```bash
./gymbag init -d app.example.com -u :3000 --service auto
./gymbag service start
```

설정 파일 없이 즉석에서 리버스 프록시를 실행할 수도 있습니다:

```bash
./gymbag reverse-proxy --from app.internal --to :3000 --verify 3
```

### "즉시 보호(Open-Box)"가 실제로 의미하는 것

Gymbag을 공용 인터넷에 올리기 전에 정규표현식을 한 줄 쓰거나 차단 목록을 수동으로 가져올 필요가 없습니다. `verify`(검증 레벨 `3`: `suspicious & ai` 활성화) 또는 `block`(위협 인텔리전스 일치 IP에 챌린지 대신 즉시 `403 Forbidden` 반환)만 선언하면 다음 세 가지 계층이 즉시 작동합니다:

1. **내장 기본 방화벽 규칙 (`default_rules on`)**
   - **민감한 파일 차단 (`403`)**: `/.env`, `/.env.*`, `/.git/*`, `/.svn/*`, `/.hg/*`, `/.DS_Store`, `/.aws/*`, `/.ssh/*`, `/docker-compose.yml`, `/docker-compose.yaml`, `*.bak`, `*.sql`, `*.sqlite` 요청을 차단합니다.
   - **CMS 및 프레임워크 취약점 탐색 차단 (`403`)**: `/wp-admin/*`, `/wp-login.php`, `/xmlrpc.php`, `/phpmyadmin*`, `/vendor/phpunit*`, `/cgi-bin/*`, `/actuator/env*`, `/actuator/heapdump*` 요청을 드롭합니다.
   - **취약점 스캐너 차단 (`403`)**: `sqlmap`, `nikto`, `nuclei`, `masscan`, `zgrab`, `gobuster`, `dirbuster`, `ffuf`, `wfuzz`, `feroxbuster`, `acunetix`, `nessus`, `openvas`를 거부합니다.
   - **인젝션 및 경로 탐색 차단 (`403`)**: 쿼리 문자열 내 `UNION SELECT`, `<script`, `javascript:`, `/etc/passwd`, `information_schema`, `../`, `cmd.exe`, `/bin/sh`를 탐지합니다(널 바이트 및 `%2e%2e` 경로 탐색에 대한 엄격한 요청 형태 검사 포함).
   - **자동화 클라이언트 검증 (`PoW 챌린지`)**: 사용자 지정 규칙이 아직 없는 경우 CLI HTTP 클라이언트(`curl`, `wget`, `python-requests`, `python-httpx`, `go-http-client`, `scrapy`), 헤드리스 브라우저(`headlesschrome`, `playwright`, `puppeteer`, `selenium`) 및 빈 `User-Agent`는 작업 증명 화면으로 이동합니다.
2. **자동 업데이트 IP 위협 인텔리전스**
   - 시작 시 및 6시간마다 백그라운드에서 [IPsum (level 3+)](https://raw.githubusercontent.com/stamparm/ipsum/master/levels/3.txt), [Spamhaus DROP IPv4](https://www.spamhaus.org/drop/drop_v4.json) & [IPv6](https://www.spamhaus.org/drop/dropv6.txt), [abuse.ch Feodo Tracker](https://feodotracker.abuse.ch/downloads/ipblocklist.txt)를 가져와 `ETag` / `If-Modified-Since` 조건부 헤더로 `<cert_dir>/intel-cache`에 캐싱합니다.
   - **Shodan InternetDB**(`internetdb.shodan.io`) 및 **StopForumSpam**(`api.stopforumspam.org`)에 대한 키 없는 실시간 IP 평판 조회를 인메모리 TTL 캐시와 fail-open 타임아웃으로 수행하여 외부 장애가 트래픽을 막지 않도록 합니다.
3. **공식 AI 크롤러 IP 대역 추적**
   - **OpenAI**(`gptbot`, `searchbot`, `chatgpt-user`), **Anthropic**(`claude.com/crawling/bots.json`), **Perplexity**(`perplexitybot`, `perplexity-user`), **Google**(`user-triggered-agents`, `user-triggered-fetchers-google`), **Apple**(`applebot`)의 공식 CIDR 목록을 추적합니다. 크롤러가 일반 데스크톱 Chrome `User-Agent`로 위장하더라도 출발지 IP로 AI 대역을 식별합니다.

> **80 및 443 포트 운영 참고:** 기본적으로 Gymbag은 `:80`(HTTP→HTTPS 리다이렉트 + ACME HTTP-01 응답기)과 `:443`(HTTPS 게이트웨이)에 바인딩됩니다. Linux에서는 root로 실행하지 않도록 바이너리에 `CAP_NET_BIND_SERVICE` 권한을 부여하세요:
> ```bash
> sudo setcap 'cap_net_bind_service=+ep' ./gymbag
> ```
> capabilities나 `sudo` 없이 로컬에서 비권한 테스트를 하려면 `--http-port 8080 --https-port 8443`을 전달하세요(또는 `Gymbagfile`의 `http_port` / `https_port` 설정).

---

## 로컬 개발 (`localhost` & `*.internal`)

사용자 지정 호스트명으로 로컬 HTTPS를 테스트하려면 보통 `mkcert`, 수동 `/etc/hosts` 수정, 자체 서명 인증서 경고와 씨름해야 합니다. Gymbag은 사이트가 `localhost`, `*.localhost`, `*.internal`(예: `app.internal`, `api.internal`), `*.local`, `*.test`를 사용할 때 호스트명 해석과 로컬 X.509 인증서 발급을 기본으로 자동 처리합니다:

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

### `/etc/hosts` 자동 등록 및 정리

`auto_hosts`가 활성화되어 있으면(기본값) Gymbag은 시작 시 `/etc/hosts`(Windows의 경우 `C:\Windows\System32\drivers\etc\hosts`)를 확인합니다. `/etc/hosts`에 아직 등록되지 않은 구체적인 로컬 도메인(`app.internal`, `api.internal` 등)에 대해 인스턴스 ID가 태그된 IPv4 및 IPv6 루프백 항목을 추가합니다:

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

기존 `localhost` 줄은 건드리지 않습니다. Gymbag을 종료(`Ctrl+C` / `SIGTERM`)하면 실행 중에 사용자가 수정한 내용을 보존하기 위해 `/etc/hosts`를 다시 읽은 뒤 자신이 추가한 `# gymbag-managed:<instanceID>` 줄**만** 제거합니다.

- **macOS & Linux 권한:** `/etc/hosts` 수정에는 root 권한이 필요합니다. `/etc/hosts` 쓰기 권한 없이 일반 사용자로 `./gymbag`를 시작하면 명확한 경고를 기록하고 **중단 없이 정상 계속 실행**됩니다. `/etc/hosts` 자동 관리(및 `80`/`443` 포트)가 필요할 때는 `sudo ./gymbag`를 사용하거나, `dnsmasq` 등을 이용 중이라면 `auto_hosts off`(또는 `GYMBAG_AUTO_HOSTS=off`)로 설정하세요.

### 2계층 로컬 PKI 및 루트 CA 신뢰 설정

브라우저가 신뢰 캐싱을 거부하는 일회용 자체 서명 리프 인증서 대신, Gymbag은 `<cert_dir>/ca/`(기본 `certs/ca/`) 내부에 2계층 ECDSA P-256 인증 기관을 유지합니다:

1. **루트 CA**: **`"Neuro-sama Headquarters Root CA"`** (`certs/ca/neuro-sama-root-ca.pem`, 유효기간 10년, `MaxPathLen: 1`)
2. **중간 CA**: **`"Gymbag & Swarm SSL Local CA"`** (`certs/ca/gymbag-swarm-local-ca.pem`, 유효기간 5년, `"Neuro-sama Headquarters Root CA"`로 서명, `MaxPathLen: 0`)
3. **리프 인증서**: `certs/local-fullchain.pem` + `certs/local-privkey.pem` (유효기간 90일, 만료 30일 전 자동 갱신, DNS SAN 및 `127.0.0.1` / `::1` IP SAN 포함)

OS 신뢰 저장소에 **`"Neuro-sama Headquarters Root CA"`**를 한 번만 등록하면 Gymbag이 제공하는 모든 `https://localhost` 및 `https://*.internal` 도메인이 브라우저와 `curl`에서 신뢰된 인증서로 표시됩니다:

**macOS (시스템 키체인):**
```bash
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain certs/ca/neuro-sama-root-ca.pem
```

**Debian / Ubuntu:**
```bash
sudo cp certs/ca/neuro-sama-root-ca.pem /usr/local/share/ca-certificates/neuro-sama-root-ca.crt && sudo update-ca-certificates
```

**Fedora / RHEL / Arch Linux:**
```bash
sudo trust anchor --store certs/ca/neuro-sama-root-ca.pem
```

> **Firefox 참고:** macOS 및 Windows에서는 `about:config`에서 `security.enterprise_roots.enabled`가 `true`이면 Firefox가 OS 루트를 자동으로 신뢰합니다. Linux에서 Firefox가 p11-kit 대신 자체 NSS DB를 사용하는 경우 `security.enterprise_roots.enabled`를 켜거나 *설정 -> 개인 정보 및 보안 -> 인증서 -> 인증서 보기 -> 인증 기관*에서 `certs/ca/neuro-sama-root-ca.pem`을 한 번 가져오세요.

---

## 다중 웹사이트 보호 및 검증 레벨 (`0`–`3`)

단일 Gymbag 프로세스로 수십 개의 도메인을 동시에 호스팅하고 리버스 프록시하며, 멀티 업스트림 라운드 로빈 로드 밸런싱과 전이중 RFC 6455 WebSocket 업그레이드를 처리할 수 있습니다. 각 사이트는 `verify <0-3>`으로 자체 검증 정책을 선택합니다(`protect off`로 WAF 비활성화 가능):

| 레벨 | 이름 | 챌린지 대상 | 추천 용도 |
| :---: | :--- | :--- | :--- |
| `0` | `always` | 유효한 HMAC 클리어런스 쿠키가 없는 모든 방문자. | 비공개 도구, 스테이징 서버 또는 활성 L7 플러딩 공격을 받는 사이트. |
| `1` | `only suspicious` | 위협 피드/저평판 IP, 스캐너, CLI HTTP 라이브러리, 헤드리스 브라우저, 누락/스푸핑된 브라우저 헤더 및 익스플로잇 프로브. | 일반 브라우저 방문자에게 챌린지 화면을 보여주고 싶지 않은 공개 웹 앱 및 API. |
| `2` | `only ai` | **공식 AI 크롤러 IP 대역** *또는* AI 봇 `User-Agent`/에이전트 헤더와 일치하는 AI 크롤러 및 LLM 에이전트. | 사람과 `curl` 사용자에게는 마찰 없이 열려 있으면서도 LLM 스크래퍼는 차단/제한하려는 문서, 블로그, Git 포지. |
| `3` | `suspicious & ai` | `1`과 `2`를 결합: 의심스러운 클라이언트/IP와 AI 크롤러 모두에게 챌린지를 요구하며 정상적인 사람 브라우저는 바로 통과. | 공개 운영 웹사이트의 권장 기본값(인자 없이 `verify`만 써도 활성화됨). |
| — | `protect off` | 작업 증명 챌린지를 비활성화하고 순수 HTTPS & WebSocket 리버스 프록시로 동작(사이트에 명시된 사용자 지정 규칙은 여전히 실행됨). | 정적 에셋 호스트, 웹훅 및 공개 패키지 미러. |

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
}

static.example.com {
    reverse_proxy http://localhost:9000
    protect off
}
```

브라우저가 SHA-256 작업 증명 챌린지(`difficulty` `1`–`5`, 기본 `4`)를 해결하면, Gymbag은 호스트와 클라이언트 IP에 바인딩된 `HttpOnly`, `SameSite=Lax` HMAC 서명 클리어런스 쿠키(`waf_clearance`, 기본 TTL `24h` / `86400s`)를 발급합니다.

### 사용자 지정 검증 유지 시간 (`verify_timeout` / `clearance_ttl`)

방문자의 인증 유지 시간을 전역 또는 사이트별로 정수 초 **또는** 사람이 읽기 쉬운 기간 문자열(`30m`, `12h`, `1h30m`, `7d`, `2w`)로 지정할 수 있습니다:

```caddyfile
{
    verify_timeout 12h     # 전역 기본값 (별칭: verified_timeout, clearance_ttl)
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
    verify_timeout 30m     # 사이트별 재정의
}
```

`gymbag init`, `gymbag run`, `gymbag reverse-proxy`에 `--verify-timeout 6h`(또는 `--clearance-ttl 6h`)를 전달하거나, `GYMBAG_VERIFY_TIMEOUT` / `GYMBAG_CLEARANCE_TTL` 환경 변수를 설정하거나, `PATCH /api/v1/sites/{domain}`으로 `clearance_ttl`을 실시간 패치할 수도 있습니다. 인증된 방문자는 `GET /.waf/v1/session`에서 `clearance_ttl`, `expires_at`, 남은 `expires_in` 초를 확인할 수 있습니다.

---

## SSL 공급자 전환, 사용자 지정 키 및 자동 갱신

공용 도메인의 경우 `tls auto`(기본값)가 Let's Encrypt를 통해 RFC 8555 ACME HTTP-01 인증서를 자동 발급합니다. `ssl_provider` 지시어(별칭: `acme_provider`, `ca`, `cert_issuer`) 또는 `tls <provider>` 한 줄로 인증 기관(CA)을 전환할 수 있습니다:

| 공급자 키워드 | 인증 기관 (CA) | EAB 요구사항 |
| :--- | :--- | :--- |
| `letsencrypt` (`le`) | Let's Encrypt (운영) | 없음 |
| `letsencrypt_staging` | Let's Encrypt (스테이징) | 없음 |
| `zerossl` | ZeroSSL | `email` 설정 시 **자동 발급**, 또는 명시적 `eab <kid> <hmac_key>` |
| `google` (`gts`) | Google Trust Services | `eab <kid> <hmac_key>` 필요 |
| `ssl.com` (`sslcom`, `ssl.com-rsa`) | SSL.com (ECC 또는 RSA) | `eab <kid> <hmac_key>` 필요 |
| `litessl` (`trustasia`) | LiteSSL by TrustAsia | 표준 DV는 불필요 (`acme_directory`로 URL 재정의 가능) |
| `buypass` | Buypass Go SSL | 없음 |
| `local` | 로컬 2계층 PKI | 로컬 CA (`Neuro-sama Headquarters Root CA` -> `Gymbag & Swarm SSL Local CA`) |
| `https://...` | 모든 RFC 8555 ACME v2 CA | 선택적 `eab <kid> <hmac_key>` (Smallstep `step-ca`, Pebble 등) |

### ACME CA 전환 및 External Account Binding (EAB)

**ZeroSSL 무설정 이메일 자동 EAB:**
`zerossl`을 선택하고 `email`을 설정하면, 명시적 EAB 키를 제공하지 않았을 때 Gymbag이 자동으로 ZeroSSL EAB 엔드포인트(`https://api.zerossl.com/acme/eab-credentials-email`)와 통신하여 자격 증명을 교환합니다:

```caddyfile
{
    email ops@example.com
    ssl_provider zerossl
}

example.com {
    reverse_proxy http://localhost:3000
    verify 3
}
```

**Google Trust Services, SSL.com 또는 명시적 EAB 자격 증명:**
Key ID와 HMAC 키를 전달합니다(패딩 없는 URL-safe base64와 표준 패딩 base64 모두 지원되며 Admin API 출력에서는 `eab_hmac_key`가 자동 마스킹됩니다):

```caddyfile
{
    email ops@example.com
    ssl_provider google
    eab YOUR_EAB_KEY_ID YOUR_EAB_HMAC_KEY
}
```

**LiteSSL by TrustAsia 또는 사용자 지정 ACME 디렉토리:**

```caddyfile
{
    email ops@example.com
    ssl_provider litessl
    # 또는 임의의 ACME v2 디렉토리 URL을 직접 지정:
    # acme_directory https://ca.internal.corp/acme/acme/directory
}
```

Gymbag은 디스크의 ACME 인증서를 공급자 이름별로 격리 저장합니다(예: `certs/acme-zerossl-fullchain.pem`, `certs/acme-google-fullchain.pem`, Let's Encrypt는 `certs/acme-fullchain.pem` 유지). `ssl_provider`를 변경하고 재시작하거나 리로드하면 이전 공급자의 남은 인증서를 쓰지 않고 즉시 새 CA에 인증서를 요청합니다.

### 기존 키 및 인증서 사용 (BYOK)

- **사용자 지정 ACME 계정 키 (`account_key`)**: 기본적으로 `certs/acme-account.pem`에 ECDSA P-256 계정 키를 생성해 재사용합니다. 조직에 이미 등록된 ECDSA(P-256/P-384) 또는 RSA ACME 계정 키가 있다면 `account_key /etc/gymbag/account.pem`(또는 `GYMBAG_ACCOUNT_KEY_FILE`)으로 지정하세요.
- **사용자 지정 CSR 개인 키 (`ssl_key`)**: `ssl_key /etc/gymbag/domain.key`를 사용하면 새 키를 생성하는 대신 보유한 ECDSA 또는 RSA 개인 키로 ACME CSR에 서명합니다.
- **사용자 지정 TLS 인증서 및 키 (`tls <cert> <key>`)**: 상용 인증서, Cloudflare Origin 인증서 또는 외부에서 관리하는 PEM 파일을 전역 또는 사이트별(SNI 라우팅)로 지정할 수 있습니다. 암호화된 개인 키는 `key_password`(또는 `GYMBAG_KEY_PASSWORD`)로 복호화합니다:

```caddyfile
{
    key_password super-secret-passphrase
}

corp.example.com {
    reverse_proxy http://localhost:8080
    tls /etc/ssl/corp-fullchain.pem /etc/ssl/corp-privkey.pem
    verify 1
}
```

### 무중단 백그라운드 자동 갱신

Gymbag은 `renew_interval`초(기본 `43200` = 12시간)마다 백그라운드 갱신 루프를 실행합니다. ACME 인증서, 로컬 PKI 리프 인증서 또는 파일 기반 인증서의 만료가 `renew_within_days`일(기본 `30`일) 이내로 다가오면 백그라운드에서 갱신/재로딩한 뒤 활성 `tls.Certificate` 포인터를 원자적으로 교체합니다. 진행 중인 HTTP 스트림과 WebSocket 터널은 단 하나의 TCP 연결도 끊기지 않습니다.

---

## 사용자 지정 방화벽 규칙 작성

보호된 사이트로 들어오는 모든 요청은 예측 가능한 순서대로 계층별 평가를 거칩니다:

1. **요청 형식 유효성 검사** (HTTP 메서드 검증, 최대 URI 길이, 널 바이트 및 `..` 경로 탐색 차단)
2. **ACME HTTP-01 우회** (`/.well-known/acme-challenge/*`)
3. **로컬 사용자 지정 규칙** (전역 규칙 우선 후 사이트 규칙, 위에서 아래로 — 첫 번째 일치 항목 적용)
4. **원격 방화벽 규칙 (`remote_rules`)** (Gymbagfile 문법 또는 JSON 형식의 자동 갱신 HTTP/HTTPS 규칙 세트)
5. **내장 기본 보안 규칙** (`default_rules off`가 아닌 경우)
6. **사이트별·IP별 속도 제한기** (`rate_limit` / `rate_window`)
7. **IP 위협 인텔리전스** (`ip_security_action verify` 또는 `block`)
8. **작업 증명(PoW) 검증** (`verify` 레벨 `0`–`3`)

사용자 지정 규칙은 내장 기본 규칙, 속도 제한기, 위협 피드**보다 먼저** 실행되므로 사이트 블록 상단에 `allow` 규칙을 두어 신뢰하는 사무실 CIDR, 내부 헬스 체크 또는 미리보기 쿠키를 깔끔하게 화이트리스트 처리할 수 있습니다.

### 한 줄 단축 규칙

문법: `<allow | verify | block> <field> <pattern...>` (`bypass`/`pass`, `challenge`/`pow`, `deny`/`drop` 별칭도 지원).

| 필드 | 허용 값 및 패턴 매칭 | 예시 |
| :--- | :--- | :--- |
| `ip` | IPv4, IPv6 또는 CIDR 대역 (공백 또는 쉼표 구분) | `allow ip 127.0.0.1 10.0.0.0/8 2001:db8::/32` |
| `user_agent` | 대소문자 구분 없는 부분 문자열, `*` 와일드카드 또는 `~regex` | `block user_agent BadBot ~^(?i)python-urllib/` |
| `path` | 정확/부분 일치, `*` 와일드카드 또는 `~regex` | `verify path /api/export* ~^/v[0-9]+/search` |
| `query` | 원시 쿼리 문자열 일치, `*` 와일드카드 또는 `~regex` | `block query "union select" ~(?i)<script` |
| `method` | HTTP 메서드 (대소문자 무관) | `verify method POST DELETE` |
| `header` | `<Header-Name> [pattern]` (`""` = 누락/비어 있음, `*` = 존재함) | `verify header Accept-Language ""` |
| `cookie` | `<cookie_name> [pattern]` (`""` = 누락/비어 있음, `*` = 존재함) | `allow cookie vip_session *` |

### 다중 조건 `rule` 블록

단일 필드로 충분하지 않을 때는 `rule "name" { ... }` 블록을 사용하세요. **같은 필드**의 여러 값은 **OR** 조건으로, 같은 블록 내의 **서로 다른 필드**는 **AND** 조건으로 결합됩니다:

```caddyfile
app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    # 로그인 또는 비밀번호 재설정 엔드포인트로의 POST 요청에 PoW 요구
    rule "challenge-auth-mutations" {
        action verify
        method POST
        path /login* /api/auth/* /reset-password*
    }

    # Origin 헤더가 없는 외부 /api/* POST 요청 차단
    rule "block-headless-api-posts" {
        action block
        method POST
        path /api/*
        header Origin ""
    }
}
```

### 원격 방화벽 규칙 (`remote_rules`)

전역 또는 사이트별로 하나 이상의 HTTP/HTTPS URL에서 공유 방화벽 규칙을 가져옵니다. **Gymbagfile 규칙 문법**(`allow`/`verify`/`block` 단축 규칙 및 `rule "name" { ... }` 블록)과 **JSON**(`[ ... ]` 또는 `{"rules": [ ... ]}`)을 모두 지원하며, `ETag` / `If-Modified-Since` 조건부 요청으로 검증된 페이로드를 `<cert_dir>/rules-cache`에 캐싱하고, 원격 소스에 연결할 수 없거나 형식이 잘못된 경우 마지막으로 정상 동작한 규칙 세트를 안전하게 유지합니다:

```caddyfile
{
    remote_rules https://rules.example.com/org-baseline.rules
    remote_rules_refresh 5m
}

app.example.com {
    reverse_proxy http://localhost:3000
    verify 3
    allow ip 10.0.0.0/8                              # 로컬 규칙이 항상 먼저 실행됨
    remote_rules https://rules.example.com/app.json  # 사이트 전용 원격 규칙
}
```

`GET /api/v1/rules/remote`와 `POST /api/v1/rules/remote/refresh`로 상태를 조회하거나 즉시 새로고침할 수 있으며 `--remote-rules` / `GYMBAG_REMOTE_RULES`로도 전달할 수 있습니다.

### 내장 규칙 제어 (`default_rules on|off`)

`default_rules`의 기본값은 `on`입니다:
- 보호된 사이트에 사용자 지정 규칙이 **없으면**, 내장 차단 프리셋(`/.env`, `/wp-admin`, 스캐너, SQLi/XSS 쿼리)과 내장 자동화 검증 프리셋이 모두 적용됩니다.
- 사용자 지정 규칙을 추가하면 해당 규칙이 먼저 실행되고 내장 **차단** 프리셋은 그 뒤에 그대로 유지되므로, `verify path` 규칙 하나를 추가했다고 해서 실수로 `/.git`이나 `/.env`가 노출되지 않습니다.
- 사이트별 또는 전역에서 내장 규칙을 완전히 끄고 수동 제어하려면 `default_rules off`(또는 `GYMBAG_DEFAULT_RULES=off`)를 설정하세요.

---

## CLI, Docker Compose, 환경 변수 & REST API

### CLI 레퍼런스

작업 디렉토리에 `.env`가 있으면 자동으로 로드하며, 플래그로 재정의하지 않으면 `./Gymbagfile` 또는 `./Caddyfile`을 사용합니다:

```bash
# 대화형 마법사 또는 플래그로 Gymbagfile 생성
./gymbag init
./gymbag init -d app.example.com -u http://localhost:3000 -l 3

# ./Gymbagfile 또는 ./Caddyfile로 서버 시작
./gymbag

# 명시적 설정 파일 지정 및 저장 시 핫 리로드 (2초마다 폴링)
./gymbag run --config Gymbagfile --watch

# 시스템 서비스 등록 및 관리 (systemd, launchd, openrc, supervisor)
./gymbag service install --config Gymbagfile
./gymbag service start
./gymbag service status
./gymbag service render --type systemd   # 모든 OS에서 systemd 유닛 미리보기

# 설정 파일 없는 즉석 리버스 프록시
./gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3

# 문법 검증 및 사이트별 보안 요약 출력
./gymbag validate --config Gymbagfile

# Gymbagfile을 표준 4칸 들여쓰기로 덮어쓰기 포맷팅
./gymbag fmt --overwrite Gymbagfile

# 버전 출력
./gymbag version
```

| 플래그 | 설명 |
| :--- | :--- |
| `-c`, `--config <path>` | `Gymbagfile`, `Caddyfile` 또는 JSON 설정 파일 경로 |
| `-d`, `--domain <list>` | 도메인 재정의 (쉼표 구분) |
| `-u`, `--upstream <url>` | 업스트림 URL 재정의 (예: `http://localhost:3000` 또는 `:3000`) |
| `-l`, `--verify <0-3>` | 검증 레벨 재정의 (`0`, `1`, `2`, `3` 또는 `off`) |
| `--verify-timeout <dur>` | 인증 유지 시간 재정의 (예: `30m`, `12h`, `7d` 또는 초; 별칭 `--clearance-ttl`) |
| `--remote-rules <url>` | 원격 방화벽 규칙 URL 연결 (http/https, 반복 지정 또는 쉼표 구분 가능) |
| `--remote-rules-refresh <dur>` | 원격 규칙 새로고침 주기 (기본 `5m` / `300s`) |
| `--no-protect` | CLI로 설정한 사이트의 WAF 검증 비활성화 |
| `--ssl-provider <name>` | ACME / SSL 공급자 재정의 (예: `letsencrypt`, `zerossl`, `google`, `litessl`, `local`) |
| `--admin <addr>` | 로컬 REST Admin API 활성화 (예: `127.0.0.1:2019` 또는 `off`) |
| `--http-port <port>` | HTTP 리다이렉트 및 ACME HTTP-01 포트 (기본 `80`) |
| `--https-port <port>` | HTTPS 게이트웨이 포트 (기본 `443`) |
| `--watch` | 설정 파일 변경 감지 및 라우트, 규칙, SNI 인증서 핫 리로드 |

### Docker & Docker Compose

멀티 아키텍처(`linux/amd64`, `linux/arm64`) 공식 Alpine 컨테이너 이미지가 `cap_net_bind_service=+ep`가 부여된 비루트 UID `10001` 구성으로 GitHub Container Registry(`ghcr.io/evil-project/gymbag`)에 게시됩니다.

**`Gymbagfile`을 마운트한 빠른 `docker run`:**

```bash
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

**환경 변수만 사용하는 무설정 `docker run`:**

```bash
docker run -d \
  --name gymbag \
  -p 80:80 -p 443:443 \
  --cap-add NET_BIND_SERVICE \
  --add-host=host.docker.internal:host-gateway \
  -e GYMBAG_DOMAIN=app.example.com \
  -e GYMBAG_UPSTREAM=http://host.docker.internal:3000 \
  -e GYMBAG_ACME_EMAIL=ops@example.com \
  -e GYMBAG_VERIFY_LEVEL=3 \
  -e GYMBAG_VERIFY_TIMEOUT=12h \
  -e GYMBAG_REMOTE_RULES=https://rules.example.com/baseline.rules \
  -v gymbag-certs:/app/certs \
  ghcr.io/evil-project/gymbag:latest
```

**Redis와 함께 실행하는 Docker Compose:**

포함된 `compose.yaml`은 Gymbag과 `redis:8-alpine`을 함께 실행하며, 순수 Go RESP2 Redis 클라이언트를 사용하여 일회용 PoW 챌린지 논스(nonce)와 고정 윈도우 속도 제한을 컨테이너 재시작이나 여러 게이트웨이 복제본 간에 공유합니다:

```bash
docker compose up -d --build
```

설정은 `./Gymbagfile`(`/app/Gymbagfile`에 읽기 전용 마운트)에서 불러오며, 인증서, 시크릿, 위협 인텔리전스 캐시, 원격 규칙 캐시는 `gymbag-certs` 명명된 볼륨에 영구 저장됩니다. 로컬 `.env` 파일의 `GYMBAG_*` 변수는 파일 설정을 깔끔하게 재정의합니다:

```dotenv
GYMBAG_ACME_EMAIL=ops@example.com
GYMBAG_SSL_PROVIDER=zerossl
GYMBAG_VERIFY_LEVEL=3
GYMBAG_CLEARANCE_TTL=12h
GYMBAG_REMOTE_RULES=https://rules.example.com/baseline.rules
```

바인드 마운트 권한(`chown 10001:10001`), `--watch` 핫 리로드, 다중 컨테이너 프록시, 로컬 빌드에 대해서는 [Docker & Docker Compose 가이드](docs/ko/guide/docker.md)를 참고하세요.

### 환경 변수 (`GYMBAG_*`)

우선순위는 **설정 파일 -> `GYMBAG_*` 환경 변수 -> CLI 플래그**입니다.

| 변수 | 설명 |
| :--- | :--- |
| `GYMBAG_CONFIG` | `Gymbagfile`, `Caddyfile` 또는 `.json` 설정 파일 경로 |
| `GYMBAG_BIND` | 바인드 주소 (로컬 도메인만 있을 때 `127.0.0.1`, 공용 도메인이 있으면 `0.0.0.0`) |
| `GYMBAG_HTTP_PORT` / `GYMBAG_HTTPS_PORT` | HTTP 및 HTTPS 수신 포트 (기본 `80` 및 `443`) |
| `GYMBAG_DOMAIN` / `GYMBAG_DOMAINS` | 쉼표로 구분된 도메인 목록 |
| `GYMBAG_UPSTREAM` | 기본 업스트림 대상 URL |
| `GYMBAG_UPSTREAMS` | 세미콜론으로 구분된 호스트 매핑: `app.example.com=http://web:3000;api.example.com=http://api:8080` |
| `GYMBAG_PROTECT` | 사이트 전체 WAF 보호 활성화/비활성화 (`on` / `off`) |
| `GYMBAG_PROTECT_DOMAINS` | WAF 보호를 활성화할 도메인 부분 집합 (쉼표 구분) |
| `GYMBAG_VERIFY_LEVEL` | 검증 레벨: `0` (`always`), `1` (`only suspicious`), `2` (`only ai`), `3` (`suspicious & ai`) |
| `GYMBAG_DEFAULT_RULES` | 내장 기본 방화벽 규칙 활성화/비활성화 (`on` / `off`, 기본 `on`) |
| `GYMBAG_REMOTE_RULES` | 쉼표 또는 공백으로 구분된 HTTP/HTTPS 원격 방화벽 규칙 URL |
| `GYMBAG_REMOTE_RULES_REFRESH` | 원격 규칙 새로고침 간격(초 또는 기간 문자열, 기본 `300` / `5m`) |
| `GYMBAG_DIFFICULTY` | SHA-256 PoW 선행 16진수 0 난이도 (`1`–`5`, 기본 `4`) |
| `GYMBAG_CHALLENGE_TTL` / `GYMBAG_CLEARANCE_TTL` | 챌린지 만료(`300`s / `5m`) 및 검증 유지 시간(`86400`s / `24h`; 별칭 `GYMBAG_VERIFY_TIMEOUT`, `GYMBAG_VERIFIED_TIMEOUT`; `30m`, `12h`, `7d` 지원) |
| `GYMBAG_COOKIE_NAME` / `GYMBAG_COOKIE_SECURE` | 클리어런스 쿠키 이름(`waf_clearance`) 및 `Secure` 플래그(`true`) |
| `GYMBAG_RATE_LIMIT` / `GYMBAG_RATE_WINDOW` | IP당 요청 제한(`240`) 및 윈도우(초, `60`) |
| `GYMBAG_TLS_MODE` | `auto`, `acme`, `local`, `selfsigned` 또는 `file` |
| `GYMBAG_SSL_PROVIDER` | `letsencrypt`, `letsencrypt_staging`, `zerossl`, `google`, `ssl.com`, `litessl`, `buypass`, `local` 또는 URL |
| `GYMBAG_ACME_EMAIL` | ACME 등록용 이메일 (ZeroSSL 자동 EAB에도 사용) |
| `GYMBAG_ACME_DIRECTORY` | 명시적 RFC 8555 ACME v2 디렉토리 URL |
| `GYMBAG_EAB_KID` / `GYMBAG_EAB_HMAC_KEY` | EAB Key ID 및 base64/base64url HMAC 키 |
| `GYMBAG_ACCOUNT_KEY_FILE` | 사용자 제공 ECDSA 또는 RSA ACME 계정 키 PEM 경로 |
| `GYMBAG_CERT_FILE` / `GYMBAG_KEY_FILE` | `file` 모드용 TLS 인증서 및 개인 키 경로 |
| `GYMBAG_KEY_PASSWORD` | 암호화된 PEM 개인 키의 암호 |
| `GYMBAG_CERT_DIR` | CA 인증서, ACME 상태 및 인텔 캐시 디렉토리 (기본 `certs`) |
| `GYMBAG_RENEW_INTERVAL` / `GYMBAG_RENEW_WITHIN_DAYS` | 갱신 확인 간격(초, `43200`) 및 갱신 임계 일수(`30`) |
| `GYMBAG_AUTO_HOSTS` | `localhost` & `*.internal`용 `/etc/hosts` 자동 관리 (`on` / `off`, 기본 `on`) |
| `GYMBAG_HSTS` | `Strict-Transport-Security` 헤더 재정의 (`on` / `off`) |
| `GYMBAG_IP_SECURITY` / `GYMBAG_IP_SECURITY_ACTION` | 위협 피드 전환(`on`/`off`) 및 동작(`verify` 또는 `block`) |
| `GYMBAG_IP_REPUTATION` | 실시간 Shodan InternetDB + StopForumSpam 조회 전환 (`on`/`off`) |
| `GYMBAG_AI_IP_LIST` | 공식 OpenAI, Anthropic, Perplexity, Google & Applebot IP 목록 전환 (`on`/`off`) |
| `GYMBAG_INTEL_REFRESH` | 위협 및 AI 피드 새로고침 간격(초, 기본 `21600`) |
| `GYMBAG_REDIS_URL` | 선택적 Redis 연결 URL (`redis://[:password@]host:port/db`) |
| `GYMBAG_SECRET` | 클리어런스 쿠키용 HMAC 시크릿 (미설정 시 `<cert_dir>/secret`에 자동 생성) |
| `GYMBAG_ADMIN` | 로컬 Admin REST API 수신 주소 (예: `127.0.0.1:2019` 또는 `off`) |

### REST API

#### 공개 WAF 엔드포인트 (보호된 사이트에서 제공)

| 메서드 | 경로 | 설명 |
| :--- | :--- | :--- |
| `GET` | `/.waf/v1/session` | 현재 사이트 클리어런스 상태, `protected`, `verify_level`(`0`–`3`), `verify_mode`, `clearance_ttl` 및 (클리어 시) `expires_at` / `expires_in` 반환 |
| `DELETE` | `/.waf/v1/session` | 방문자의 클리어런스 쿠키 삭제 (`204 No Content`) |
| `POST` | `/.waf/v1/challenges` | 일회용 SHA-256 PoW 챌린지 발급 (`201 Created`) |
| `POST` | `/.waf/v1/verifications` | PoW 풀이 검증, 서명된 클리어런스 쿠키 설정 및 `expires_in` 반환 (`/.waf/verify` 별칭 지원) |
| `GET` | `/.waf/assets/{name}` | 내장 PNG 챌린지 에셋 제공 (`/.waf/v1/assets/{name}` 별칭 지원, `ETag` 캐싱) |

#### 로컬 관리 API (`admin 127.0.0.1:2019` 또는 `--admin`으로 활성화)

관리 API는 루프백(`127.0.0.1:2019`) 또는 Unix/WireGuard 관리 인터페이스에 바인딩하세요. 인증 토큰 없이도 서버를 재시작하지 않고 실시간 라우팅과 방화벽 규칙을 변경할 수 있습니다.

| 메서드 | 경로 | 설명 |
| :--- | :--- | :--- |
| `GET` | `/api/v1/health` | 게이트웨이 상태, 사이트 수, 활성 `tls_source`, 로드된 `threat_entries`, `ai_entries`, `remote_rules` |
| `GET` | `/api/v1/tls` | 활성 TLS 소스, 인증서 경로, 설정된 SSL 공급자, HSTS 상태 및 지원 CA 카탈로그 |
| `POST` | `/api/v1/tls/renew` | 즉시 인증서 갱신 및 원자적 핫 리로드 강제 실행 |
| `GET` | `/api/v1/intel` | 피드별 위협 & AI 크롤러 IP 수, 마지막 새로고침 타임스탬프, 평판 캐시 통계 |
| `POST` | `/api/v1/intel/refresh` | 백그라운드 피드 즉시 새로고침 및 갱신된 개수 반환 |
| `GET` | `/api/v1/rules/remote` | URL별 원격 방화벽 규칙 수, 캐시 상태, 새로고침 간격 및 마지막 가져오기 오류 |
| `POST` | `/api/v1/rules/remote/refresh` | 모든 원격 규칙 URL 즉시 가져오기 & WAF 엔진 원자적 리로드 |
| `GET` | `/api/v1/config` | 활성 런타임 설정 출력 (`secret`, `key_password`, `eab_hmac_key` 마스킹됨) |
| `PUT` | `/api/v1/config` | 전체 런타임 설정을 원자적으로 검증 및 교체 |
| `GET` | `/api/v1/sites` | 설정된 모든 웹사이트 및 WAF 정책 목록 조회 |
| `POST` | `/api/v1/sites` | 런타임 중 새 웹사이트 추가 (`201 Created`) |
| `GET` | `/api/v1/sites/{domain}` | 단일 웹사이트 설정 상세 조회 |
| `PUT` | `/api/v1/sites/{domain}` | 단일 웹사이트 설정 전체 교체 |
| `PATCH` | `/api/v1/sites/{domain}` | `verify_level`, `protect`, `difficulty`, `clearance_ttl`, `upstreams`, `rate_limit`, `rules`, `remote_rules` 실시간 패치 |
| `DELETE` | `/api/v1/sites/{domain}` | 웹사이트 제거 (`204 No Content`; 마지막 남은 1개 사이트는 삭제 거부) |
| `GET` | `/api/v1/sites/{domain}/rules` | 웹사이트의 사용자 지정 방화벽 규칙 조회 |
| `POST` | `/api/v1/sites/{domain}/rules` | 사용자 지정 방화벽 규칙 실시간 추가 (`201 Created`) |
| `PUT` | `/api/v1/sites/{domain}/rules` | 웹사이트의 모든 사용자 지정 방화벽 규칙 교체 |
| `DELETE` | `/api/v1/sites/{domain}/rules/{index_or_name}` | 숫자 인덱스 또는 규칙 `name`으로 사용자 지정 규칙 삭제 (`204 No Content`) |

**실시간 긴급 완화 예시:**

```bash
# 트래픽 급증 시 app.example.com을 verify level 0(전체 챌린지)으로 즉시 강화
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"verify_level": 0, "difficulty": 5}'

# Gymbagfile 수정 없이 악성 CIDR 대역 즉시 차단
curl -X POST http://127.0.0.1:2019/api/v1/sites/app.example.com/rules \
  -H "Content-Type: application/json" \
  -d '{"name": "drop-abusive-subnet", "action": "block", "ips": ["198.51.100.0/24"]}'

# IP 위협 및 AI 크롤러 목록 즉시 강제 새로고침
curl -X POST http://127.0.0.1:2019/api/v1/intel/refresh
```

---

## Caddy & Nginx와의 벤치마크 비교

Apple M4(10코어, macOS, Go `1.27.0`)에서 세 프록시 모두 동일한 ECDSA P-256 인증서로 TLS 1.3을 종단하고, 64개 동시 워커, `1 KiB` 응답 페이로드, `1024` 업스트림 keep-alive 연결 조건으로 벤치마크했습니다(모든 실행 오류 `0`건):

| 서버 & 모드 | HTTP/1.1 TLS (RPS) | H1 p50 / p99 | HTTP/2 TLS (RPS) | H2 p50 / p99 | 바이너리 크기 | 부하 시 RSS |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **85,203** | 0.45 / 6.20 ms | **88,775** | 0.65 / 1.96 ms | 1.5 MB (+libs) | 5.8 MiB |
| **Gymbag `v0.1.0`** (`verify 3`, 전체 WAF 활성) | **72,517** | 0.74 / 3.14 ms | **66,096** | 0.82 / 3.48 ms | 12.9 MB (정적) | 54.3 MiB |
| **Gymbag `v0.1.0`** (`protect off`, 순수 프록시) | **72,479** | 0.74 / 3.15 ms | **64,048** | 0.83 / 3.94 ms | 12.9 MB (정적) | 52.1 MiB |
| **Gymbag `v0.1.0`** (`verify 0`, 쿠키 통과) | **65,490** | 0.79 / 3.93 ms | **62,108** | 0.84 / 4.27 ms | 12.9 MB (정적) | ~54 MiB |
| **Caddy `v2.11.9`** (Go, keep-alive 튜닝) | **54,741** | 0.90 / 4.90 ms | **39,207** | 1.30 / 6.18 ms | 49.0 MB | 76.8 MiB |

- **HTTP/1.1에서 Caddy보다 +32%, HTTP/2에서 +63% ~ +69% 더 빠르며**, 메모리는 약 30% 적게 사용하고 바이너리 크기는 3.8배 더 작습니다 — 심지어 Gymbag의 전체 WAF가 켜진 상태에서도 마찬가지입니다.
- **C 기반 Nginx HTTP/1.1 처리량의 85%**를 달성하면서 p99 테일 레이턴시는 거의 **2배 더 낮습니다**(`3.14 ms` vs `6.20 ms`).
- TLS 상의 `/.env` 프로브에 대해 **140,680 req/s** 엣지 WAF 차단 성능(`0.35 ms` p50, 업스트림 트래픽 0).

전체 측정 방법론, Go 마이크로벤치마크(`go test -bench`) 및 정확한 설정은 [벤치마크 및 성능](docs/ko/guide/benchmarks.md)을 참조하세요.

---

## 프로젝트 구조

```text
.
├── cmd/gymbag/          # 독립 실행형 CLI 진입점
├── docs/                # 다국어 VitePress 문서 (EN, 简体中文, 日本語, 한국어, Español)
├── internal/
│   ├── cli/             # 하위 명령어 (init, service, run, reverse-proxy, validate, fmt, version), .env & 감시자
│   ├── config/          # 설정 모델, Gymbagfile/Caddyfile 및 기간/원격 규칙 파서, GYMBAG_* 로더
│   ├── host/            # 도메인 정규화, IPv6 대괄호 처리, 와일드카드 매칭
│   ├── intel/           # O(1) 프리픽스 집합, 자동 갱신 위협 & AI 크롤러 피드, 실시간 IP 평판
│   ├── localpki/        # 2계층 로컬 CA ("Neuro-sama Headquarters Root CA") & /etc/hosts 관리자
│   ├── proxy/           # 락프리 멀티 사이트 라우터, 풀링된 HTTP & RFC 6455 WebSocket 프록시
│   ├── server/          # 통합 HTTPS 게이트웨이, HTTP->HTTPS/ACME 리다이렉터, REST Admin API
│   ├── service/         # 시스템 서비스 렌더러 & 관리자 (systemd, launchd, openrc, supervisor)
│   ├── store/           # StateStore 인터페이스, 64-샤드 인메모리 스토어, 순수 Go RESP2 Redis 클라이언트
│   ├── tlsmanager/      # 멀티 CA ACME v2 (EAB & ZeroSSL 자동 EAB), 로컬 PKI, 락프리 SNI 인증서 관리
│   └── waf/             # PoW 검증기, 샤드화된 HMAC 클리어런스 캐시, Aho-Corasick DFA, 원격 규칙, UI
├── Dockerfile
├── compose.yaml
├── flake.nix
├── Gymbagfile
└── go.mod
```

---

## Star 히스토리

<a href="https://www.star-history.com/#Evil-Project/gymbag&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
 </picture>
</a>
