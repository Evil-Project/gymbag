# Docker & Docker Compose

릴리스 태그와 `main` 브랜치 푸시마다 GitHub Container Registry(`GHCR`)에 멀티 아키텍처(`linux/amd64`, `linux/arm64`) Alpine 이미지가 게시됩니다.

- **이미지**: `ghcr.io/evil-project/gymbag`
- **태그**: `latest`, `<semver>` (예: `0.1.0`), `<major>.<minor>`, `<major>`, `main`, 커밋 SHA
- **실행 사용자**: `cap_net_bind_service=+ep`가 적용된 비루트 `gymbag` (UID `10001`)
- **포트**: 기본 노출 포트는 `80` (HTTP 리다이렉트 + ACME HTTP-01) 및 `443` (HTTPS 게이트웨이)
- **헬스체크**: 30초마다 `127.0.0.1:80` TCP 프로브

## `docker run`으로 빠른 시작

### 옵션 A: `Gymbagfile` 또는 `gymbag.json` 마운트

현재 디렉터리에 `Gymbagfile`을 작성합니다.

```caddyfile
{
    email ops@example.com
    ssl_provider letsencrypt
    verify_timeout 12h
    remote_rules https://rules.example.com/baseline.rules
    remote_rules_refresh 5m
}

app.example.com {
    reverse_proxy http://host.docker.internal:3000
    verify 3
}
```

ACME 인증서, 시크릿, 위협 인텔 캐시, 원격 규칙 캐시가 컨테이너 재빌드 후에도 남도록 `/app/certs` 영구 볼륨을 붙여 실행합니다.

```bash
docker run -d \
  --name gymbag \
  --restart unless-stopped \
  -p 80:80 \
  -p 443:443 \
  --cap-add NET_BIND_SERVICE \
  --add-host=host.docker.internal:host-gateway \
  -v "$(pwd)/Gymbagfile:/app/Gymbagfile:ro" \
  -v gymbag-certs:/app/certs \
  ghcr.io/evil-project/gymbag:latest
```

### 옵션 B: 환경 변수만 사용

설정 파일을 마운트하지 않으려면 `GYMBAG_*` 환경 변수를 직접 전달합니다.

```bash
docker run -d \
  --name gymbag \
  --restart unless-stopped \
  -p 80:80 \
  -p 443:443 \
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

### 옵션 C: SSH, Minecraft 및 인증 포털 노출

Docker에서 SSH 서버나 Minecraft 서버를 보호하려면 해당 서비스 포트(`22` 또는 `25565`)와 함께 인증 포털 포트(`8443`)를 열고, 연결 해제 배너에 컨테이너 IP 대신 사용자 지정 도메인을 띄우려면 `GYMBAG_VERIFY_URL`을 지정하세요.

```bash
docker run -d \
  --name gymbag \
  --restart unless-stopped \
  -p 80:80 -p 443:443 \
  -p 25565:25565 -p 8443:8443 \
  --cap-add NET_BIND_SERVICE \
  --add-host=host.docker.internal:host-gateway \
  -e GYMBAG_MC_ENABLED=true \
  -e GYMBAG_MC_UPSTREAM=host.docker.internal:25566 \
  -e GYMBAG_MC_VERIFY_LEVEL=3 \
  -e GYMBAG_VERIFY_URL=https://verify.example.com:8443 \
  -e GYMBAG_VERIFY_PORT=8443 \
  -e GYMBAG_VPN_DETECTION=on \
  -v gymbag-certs:/app/certs \
  ghcr.io/evil-project/gymbag:latest
```

SSH에도 동일한 형태의 변수(`GYMBAG_SSH_ENABLED`, `GYMBAG_SSH_PORT`, `GYMBAG_SSH_UPSTREAM`, `GYMBAG_SSH_VERIFY_LEVEL`, `GYMBAG_SSH_PROTECT`)를 그대로 씁니다.

### 옵션 D: 핫 리로드 (`--watch`)

`CMD`를 덮어써서 `--watch`를 전달하면 설정 파일 편집 시 컨테이너 재시작 없이 라우트, 방화벽 규칙, SNI 인증서를 다시 불러옵니다.

```bash
docker run -d \
  --name gymbag \
  -p 80:80 -p 443:443 \
  --cap-add NET_BIND_SERVICE \
  -v "$(pwd)/config:/app/config:ro" \
  -v gymbag-certs:/app/certs \
  ghcr.io/evil-project/gymbag:latest \
  run --config /app/config/Gymbagfile --watch
```

::: tip `--watch` 사용 시 상위 디렉터리를 마운트하세요
원자적 rename으로 파일을 저장하는 에디터는 호스트 inode를 새로 만듭니다. 단일 파일만 바인드 마운트(`./Gymbagfile:/app/Gymbagfile:ro`)하면 컨테이너가 예전 inode를 계속 읽으므로 `--watch`가 수정 사항을 못 봅니다. 대신 상위 디렉터리(`./config:/app/config:ro`)를 마운트하세요.
:::

## Redis와 함께 사용하는 Docker Compose

저장소의 `compose.yaml`은 Gymbag 옆에 `redis:8-alpine`을 띄웁니다. `GYMBAG_REDIS_URL`을 설정하면 순수 Go RESP2 클라이언트를 통해 PoW 챌린지 논스, 고정 윈도우 속도 제한, SSH/Minecraft 인증 티켓과 IP 인증 기록을 재시작 및 여러 컨테이너 간에 유지·공유합니다.

```yaml
services:
  gymbag:
    image: ghcr.io/evil-project/gymbag:latest
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    environment:
      GYMBAG_BIND: 0.0.0.0
      GYMBAG_CONFIG: /app/Gymbagfile
      GYMBAG_REDIS_URL: redis://redis:6379/0
      GYMBAG_ACME_EMAIL: ${GYMBAG_ACME_EMAIL:-}
      GYMBAG_SSL_PROVIDER: ${GYMBAG_SSL_PROVIDER:-}
      GYMBAG_VERIFY_LEVEL: ${GYMBAG_VERIFY_LEVEL:-}
      GYMBAG_CLEARANCE_TTL: ${GYMBAG_CLEARANCE_TTL:-}
      GYMBAG_REMOTE_RULES: ${GYMBAG_REMOTE_RULES:-}
      GYMBAG_REMOTE_RULES_REFRESH: ${GYMBAG_REMOTE_RULES_REFRESH:-}
    volumes:
      - ./Gymbagfile:/app/Gymbagfile:ro
      - gymbag-certs:/app/certs
    extra_hosts:
      - "host.docker.internal:host-gateway"
    cap_add:
      - NET_BIND_SERVICE
    depends_on:
      redis:
        condition: service_healthy

  app:
    image: your-org/your-web-app:latest
    restart: unless-stopped
    expose:
      - "3000"

  redis:
    image: redis:8-alpine
    restart: unless-stopped
    command: ["redis-server", "--appendonly", "yes"]
    volumes:
      - gymbag-redis:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5

volumes:
  gymbag-certs:
  gymbag-redis:
```

`Gymbagfile`에서는 Compose 서비스(`http://app:3000`)나 호스트에 바인딩된 앱(`http://host.docker.internal:3000`)으로 `reverse_proxy`를 지정합니다.

```caddyfile
app.example.com {
    reverse_proxy http://app:3000
    verify 3
    verify_timeout 24h
}
```

스택을 시작하거나 로컬 체크아웃에서 직접 빌드해 띄웁니다.

```bash
docker compose up -d
# 또는 로컬 빌드:
docker compose up -d --build
```

`compose.yaml` 옆에 `.env` 파일을 두면 `Gymbagfile` 값을 덮어씁니다.

```dotenv
GYMBAG_ACME_EMAIL=ops@example.com
GYMBAG_SSL_PROVIDER=zerossl
GYMBAG_VERIFY_LEVEL=3
GYMBAG_CLEARANCE_TTL=12h
GYMBAG_REMOTE_RULES=https://rules.example.com/waf.rules
```

## 설정 파일 없이 여러 컨테이너 프록시하기

```yaml
environment:
  GYMBAG_DOMAINS: app.example.com,api.example.com,docs.example.com
  GYMBAG_UPSTREAMS: "app.example.com=http://web:3000;api.example.com=http://api:8080;docs.example.com=http://docs:4000"
  GYMBAG_PROTECT_DOMAINS: app.example.com,docs.example.com
  GYMBAG_VERIFY_LEVEL: "3"
```

## 볼륨, 권한 및 보안 참고사항

- **영구 `/app/certs` 볼륨**: ACME 계정 키, 발급된 인증서, 로컬 CA 파일, HMAC 클리어런스 시크릿(`/app/certs/secret`), 위협/VPN/AI 인텔 캐시(`/app/certs/intel-cache`), 원격 규칙 캐시(`/app/certs/rules-cache`)를 저장합니다. 항상 명명된 볼륨이나 호스트 디렉터리로 유지하세요.
- **호스트 바인드 마운트 권한**: Gymbag은 컨테이너 내부에서 UID/GID `10001`로 실행됩니다. 명명된 볼륨 대신 `./certs:/app/certs`를 바인드 마운트하려면 `10001`이 쓸 수 있도록 권한을 맞춰 주세요.
  ```bash
  mkdir -p ./certs
  sudo chown -R 10001:10001 ./certs
  ```
- **ACME HTTP-01을 위해 `80` 포트 필요**: 공인 CA는 `80` 포트의 `/.well-known/acme-challenge/*`로 소유권을 검증하므로 `80:80`과 `443:443`을 매핑해야 합니다.
- **Docker 내부 로컬 `*.internal`**: 컨테이너는 호스트 `/etc/hosts`를 수정할 수 없으므로 `GYMBAG_AUTO_HOSTS=off`를 설정하고, 필요하면 비권한 포트(`-p 8080:80 -p 8443:443`)를 매핑한 뒤 볼륨에서 `/app/certs/ca/neuro-sama-root-ca.pem`을 꺼내 호스트에서 신뢰 등록하세요.
- **Admin REST API**: 컨테이너에서 `GYMBAG_ADMIN=0.0.0.0:2019`를 연 경우 호스트 포트는 반드시 루프백(`-p 127.0.0.1:2019:2019`)으로 묶어 인증 없는 상태 변경이 인터넷에 노출되지 않도록 하세요.

## 로컬에서 이미지 빌드하기

```bash
docker build \
  --build-arg VERSION=0.1.0 \
  -t gymbag:local .
```

`Dockerfile`은 `golang:1.27-alpine`에서 정적 `CGO_ENABLED=0` 바이너리를 빌드한 뒤 `ca-certificates`와 `libcap`이 포함된 `alpine:3.21` 이미지로 복사합니다.
