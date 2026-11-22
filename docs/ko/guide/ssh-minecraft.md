# SSH & Minecraft 보호

Gymbag은 HTTP/HTTPS뿐만 아니라 OpenSSH와 Minecraft Java Edition 앞단에서도 SHA-256 작업 증명(PoW) 챌린지, 속도 제한, IP 위협 피드, VPN/프록시/Tor 탐지, 사용자 지정 규칙을 동일하게 적용합니다.

SSH나 Minecraft 패킷 내부에서 PoW를 계산하게 하는 대신, 미인증 연결을 해당 프로토콜의 연결 해제 메시지로 거부하면서 사람이 직접 입력하기 쉬운 짧은 일회용 HTTPS 링크(`/v/<token>`, 대소문자를 구분하지 않고 매칭되는 혼동 없는 소문자 8자리)를 출력합니다. 사용자가 브라우저에서 해당 링크를 열어 퍼즐을 풀면 Gymbag이 `clearance_ttl`(기본 `24h`) 동안 해당 IP를 인증 처리하며, 이후 연결은 원시 TCP 릴레이로 바로 통과합니다.

## SSH 보호

### 자동 설정 (`sudo gymbag init -t ssh`)

`sudo ./gymbag init -t ssh`를 실행하면 호스트 구성을 자동으로 처리합니다.

1. `20000–59999` 사이에서 사용 중이 아닌 루프백 포트를 하나 선택합니다.
2. `/etc/ssh/sshd_config` 백업이 없으면 `/etc/ssh/sshd_config.gymbag.bak`으로 저장합니다.
3. 기존 최상위 `Port` 지시어를 주석 처리하고, 첫 번째 `Match` 블록 앞에 `# gymbag-managed:start` / `# gymbag-managed:end` 블록으로 새 포트를 기록합니다.
4. `sshd -t -f /etc/ssh/sshd_config`를 실행합니다. 검증에 실패하면 원본 파일을 복원하고 중단합니다.
5. `sshd`를 리로드합니다(Linux에서 `systemctl`, `service`, 또는 `rc-service` 사용).
6. 활성 Linux 방화벽(`ufw`, `firewalld`, `nftables`, 또는 `iptables`+`ip6tables`)을 갱신해 백엔드 포트로 가는 루프백 트래픽만 허용하고 외부에서 백엔드 포트로 직접 들어오는 트래픽은 차단하며, SSH 수신 포트(`22`)와 인증 포털 포트(`8443`)는 열어 둡니다.
7. `gymbag.json`을 작성합니다(`--format gymbagfile` 지정 시 `Gymbagfile`).

::: warning 두 번째 SSH 터미널을 열어 두세요
원격 머신에서 `sshd_config`와 방화벽 규칙을 바꿀 때는 기존 root 셸을 하나 열어 둔 상태에서 Gymbag을 시작하고 포트 `22`로 정상 로그인되는지 먼저 확인하세요.
:::

`sshd`가 이미 `127.0.0.1:2222`에서 대기 중이거나 `sshd_config`·방화벽 수정을 건너뛰려면 `--ssh-upstream 127.0.0.1:2222` 또는 `--no-auto-setup`을 전달하세요(방화벽 변경만 건너뛰려면 `--no-firewall`).

```bash
sudo ./gymbag init -t ssh --ssh-upstream 127.0.0.1:2222
```

### SSH 트래픽 처리 방식

- **빠른 경로**: `protect off` 상태이거나 클라이언트 IP에 유효한 인증 기록이 있고 사용자 지정 규칙이 없다면, Gymbag은 SSH 배너를 읽지 않고 TCP 스트림을 바로 릴레이합니다.
- **의심 SSH 탐지** (검증 레벨 `1`과 `3`): 클라이언트의 `SSH-2.0-...` 식별 배너를 검사해 빈 배너나 `SSH-`로 시작하지 않는 프로브, 알려진 스캐너 및 스크립트 SSH 라이브러리(`libssh`, `paramiko`, `zgrab`, `masscan`, `nmap`, `asyncssh`, `ssh2go`, `jsch`, `scanner`, `bot`, `python`, `curl`, `ssh-2.0-go`, `hydra`, `medusa`, `patator`), 위협 피드 IP, VPN/프록시/Tor IP를 잡아냅니다.
- **챌린지 응답**: RFC 4253 `SSH_MSG_DISCONNECT` 패킷(사유 코드 `11`)에 인증 링크를 담아 SSH 클라이언트 터미널에 바로 출력합니다.

## Minecraft Java 보호

### 관리형 시작 (`--server-jar`) vs. 기존 서버 (`--server-properties`)

Gymbag이 Minecraft 프로세스를 직접 관리하도록 하려면 `--server-jar`를 전달하세요.

```bash
./gymbag init -t minecraft --server-jar /srv/mc/paper.jar --mc-jvm-args "-Xms1G -Xmx4G"
./gymbag
```

관리형 모드에서 Gymbag은 다음을 수행합니다.

1. **호환되는 Java 런타임 자동 선택**: `version.json`, 번들러 중첩 JAR, `patch.properties`, `.class` 바이트코드 헤더를 검사해 레거시 Java 8 서버와 최신 Java 17/21+ 서버를 구분하고 호스트에서 일치하는 `java` 바이너리를 선택합니다(`--mc-java` / `java_path`로 재정의 가능).
2. **안전한 `server.properties` 구성**: `20000–59999` 사이의 빈 루프백 포트를 고르고, `server.properties`가 아직 없으면 `server-ip=127.0.0.1`로 새로 생성하며(기존 파일이 있으면 백업 후 갱신), `upstream`을 자동으로 설정합니다.
3. **자식 프로세스 관리**: `jvm_args`와 `server_args`를 적용해 JAR을 실행하고, 비정상 종료 시 기본 설정(`auto_restart`)에 따라 재시작하며, Gymbag 종료 시 콘솔 `stop`(15s 타임아웃)으로 안전하게 종료합니다.

이미 별도로(systemd, tmux, screen, Pterodactyl 등) Minecraft 서버를 실행하고 있다면 대신 `server.properties`를 지정해 `init`을 실행하세요.

```bash
./gymbag init -t minecraft --server-properties /srv/mc/server.properties
./gymbag
```

기존 `server.properties`에 대해 `init`은 다음을 수행합니다.

1. `20000–59999` 사이의 빈 포트를 고르고 `server.properties`를 `server.properties.gymbag.bak`으로 백업합니다.
2. 주석과 다른 설정은 유지한 채 `server-port=<random>`을 갱신합니다.
3. 비어 있거나 `0.0.0.0` 또는 `::`로 된 `server-ip` 줄을 `server-ip=127.0.0.1`로 바꿔 플레이어가 백엔드 포트로 직접 우회하지 못하게 합니다(`server-ip`가 비루프백 주소였다면 외부 방화벽에서도 해당 포트를 막아 두세요).
4. `gymbag.json` 또는 `Gymbagfile`에 업스트림을 기록합니다.

`server.properties`를 직접 관리하려면 `--mc-upstream 127.0.0.1:25566` 또는 `--no-auto-setup`을 전달하세요.

### Minecraft 트래픽 처리 방식

- **서버 목록 핑 통과**: 최신 상태 핸드셰이크(`nextState == 1`)와 레거시 1.6 핑(`0xFE`)은 규칙이나 `ip_security_action block`에 걸리지 않는 한 업스트림 서버로 전달됩니다. 따라서 MOTD, 아이콘, 접속자 수 표시는 평소처럼 동작합니다.
- **로그인 및 전송 게이트 (`nextState == 2` / `3`)**: 핸드셰이크와 Login Start 사용자 이름을 검사합니다. 잘못된 프로토콜 버전(`<= 0`), 기형적인 로그인 패킷, 잘못된 사용자 이름(빈 값, 32자 초과, 제어 문자 포함), 위협 피드 IP, VPN/프록시/Tor IP는 레벨 `1`과 `3`에서 의심 대상으로 처리됩니다.
- **챌린지 응답**: JSON Chat Component 형식의 Minecraft Login Disconnect 패킷(`0x00`)을 반환해 플레이어의 **"Connection Lost"** 대화 상자에 클릭해 복사할 수 있는 인증 URL(1.21.5+에서는 `click_event`/`hover_event`, 1.21.4 이하에서는 `clickEvent`/`hoverEvent`)을 표시합니다.

## HTTP, SSH, Minecraft 함께 실행하기

한 Gymbag 프로세스에서 세 리스너를 동시에 실행할 수 있습니다.

```bash
sudo ./gymbag init -t http,ssh,minecraft \
  -d app.example.com -u http://localhost:3000 \
  --server-jar /srv/mc/paper.jar
sudo ./gymbag
```

`Gymbagfile` 형식:

```caddyfile
{
    email ops@example.com
    verify_url https://verify.example.com:8443
    verify_port 8443
}

app.example.com {
    reverse_proxy http://localhost:3000
    verify 3
}

ssh {
    port 22
    upstream 127.0.0.1:41822
    verify 3
    difficulty 4
    rate_limit 60 60s
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

동일한 `gymbag.json` 구성:

```json
{
  "sites": [
    {
      "domain": "app.example.com",
      "upstreams": ["http://localhost:3000"],
      "waf": {"verify_level": 3}
    }
  ],
  "ssh": {
    "port": 22,
    "upstream": "127.0.0.1:41822",
    "verify_level": 3,
    "rules": [
      {"action": "allow", "ips": ["10.0.0.0/8"]}
    ]
  },
  "minecraft": {
    "port": 25565,
    "upstream": "127.0.0.1:48291",
    "server_jar": "/srv/mc/paper.jar",
    "jvm_args": ["-Xms1G", "-Xmx4G"],
    "server_properties": "/srv/mc/server.properties",
    "verify_level": 3
  },
  "verify": {
    "url": "https://verify.example.com:8443",
    "port": 8443
  },
  "tls": {
    "acme_email": "ops@example.com"
  }
}
```

## 공용 HTTPS 인증 포털 및 IP SSL 인증서

기본적으로 SSH와 Minecraft는 포트 `8443`(`verify_port` / `--verify-port` / `GYMBAG_VERIFY_PORT`)의 HTTPS 리스너를 함께 사용합니다.

- **포트 `443` 공유**: 이미 HTTP 사이트가 활성화되어 있고 `verify_port 443`(`https_port`와 동일)으로 설정하면, Gymbag은 두 번째 포트를 열지 않고 기본 HTTPS 리스너에서 `/v/*`, `/verify/*`, `/.waf/gate/verify`를 제공합니다.
- **사용자 지정 도메인 vs 자동 서버 IP**: DNS 호스트명을 쓰려면 `verify_url https://verify.example.com:8443`을 지정합니다. `verify_url`을 비워 두면 로컬 인터페이스에서 서버의 공인 IP를 찾습니다(찾지 못하면 1.5s 타임아웃으로 `https://api.ipify.org` 폴백).
- **IP SSL 인증서 (RFC 8738)**: ACME 환경에서 IP로 포털을 제공할 때 Gymbag은 RFC 8738 IP HTTP-01 인증서를 요청하며(CA가 `shortlived` 프로필을 알리면 자동 선택) `certs/verify-acme-fullchain.pem`(LE 외 CA는 `verify-acme-<provider>-fullchain.pem`)에 캐싱합니다. 포트 `80`에 도달할 수 없거나 CA가 IP 발급을 거부하면 로컬 CA(`certs/verify-local-fullchain.pem`) 또는 자체 서명 IP 인증서(`certs/verify-selfsigned.pem`)로 폴백하며, `verify_server { tls <cert> <key> }`로 직접 인증서 파일을 지정할 수도 있습니다.

## 기본값, 속도 제한 및 Redis

- `ssh`와 `minecraft` 블록의 기본 `verify` 레벨은 `3`(`suspicious & ai`)입니다.
- `ssh`와 `minecraft`의 기본 `rate_limit`은 `60`초당 `60`회 시도입니다(웹사이트 기본값은 `60`초당 `240`회).
- `{ ... }` 안에 정의한 전역 사용자 지정 규칙은 SSH와 Minecraft 서비스에서도 서비스별 규칙보다 먼저 평가됩니다(IP/CIDR 및 클라이언트 배너/사용자 이름을 `user_agent`로 매칭).
- `redis` / `GYMBAG_REDIS_URL`이 설정되면 인증 티켓(`gymbag:ticket:<token>`)과 인증된 IP(`gymbag:clearance:<service>:<ip>`)가 Redis에 저장되므로 재시작 후에도 유지되고 여러 인스턴스에서 공유됩니다.
