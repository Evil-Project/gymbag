# Docker & Docker Compose

Gymbag은 모든 릴리스 및 `main` 브랜치 푸시마다 GitHub Container Registry(`GHCR`)에 Alpine 기반 멀티 아키텍처(`linux/amd64`, `linux/arm64`) 공식 이미지를 게시합니다:

- **이미지**: `ghcr.io/evil-project/gymbag`
- **태그**: `latest`, `<semver>` (예: `0.1.0`), `<major>.<minor>`, `<major>`, `main`, 커밋 SHA
- **실행 사용자**: 비루트 사용자 `gymbag` (UID `10001`, `cap_net_bind_service=+ep` 부여)
- **노출 포트**: `80` (HTTP 리디렉션 + ACME HTTP-01) 및 `443` (HTTPS 게이트웨이)
- **헬스체크**: 30초마다 `127.0.0.1:80` TCP 상태 점검

---

## `docker run` 빠른 시작

### 옵션 A: `Gymbagfile` 마운트 (권장)

현재 디렉터리에 `Gymbagfile`을 생성합니다:

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

인증서, 로컬 CA 키, 서명 비밀 키, 위협 인텔 캐시, 원격 규칙 캐시용 영구 볼륨과 함께 실행합니다:

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

### 옵션 B: 설정 파일 없이 환경 변수만 사용

설정 파일을 마운트하지 않고 `GYMBAG_*` 환경 변수만으로 구성할 수 있습니다:

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

### 옵션 C: 핫 리로드 활성화 (`--watch`)

컨테이너 `CMD`에 `--watch`를 전달하면 컨테이너 재시작 없이 변경된 `Gymbagfile`을 자동 반영합니다:

```bash
docker run -d \
  --name gymbag \
  -p 80:80 -p 443:443 \
  --cap-add NET_BIND_SERVICE \
  -v "$(pwd)/Gymbagfile:/app/Gymbagfile:ro" \
  -v gymbag-certs:/app/certs \
  ghcr.io/evil-project/gymbag:latest \
  run --config /app/Gymbagfile --watch
```

::: tip 단일 파일 바인드 마운트 시 inode 교체 주의
단일 파일만 마운트(`./Gymbagfile:/app/Gymbagfile:ro`)한 경우, 일부 에디터가 원자적 이름 바꾸기로 저장하면 호스트 inode가 바뀌어 컨테이너가 변경을 감지하지 못할 수 있습니다. Docker에서 `--watch`를 사용할 때는 디렉터리 단위로 마운트(`-v "$(pwd)/config:/app/config:ro"`)하고 `--config /app/config/Gymbagfile --watch`를 사용하세요.
:::

---

## Docker Compose + Redis (프로덕션 멀티 컨테이너)

저장소에 포함된 `compose.yaml`은 `redis:8-alpine`과 함께 Gymbag을 실행합니다. `GYMBAG_REDIS_URL`이 설정되면 내장 순수 Go RESP2 Redis 클라이언트가 재시작 및 다중 레플리카 간 일회용 PoW 챌린지와 속도 제한 상태를 공유합니다:

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

`Gymbagfile` 내부에서는 Compose 서비스 이름(`http://app:3000`) 또는 호스트 주소(`http://host.docker.internal:3000`)로 연결합니다:

```caddyfile
app.example.com {
    reverse_proxy http://app:3000
    verify 3
    verify_timeout 24h
}
```

실행 명령:

```bash
# GHCR 사전 빌드 이미지 사용
docker compose up -d

# 로컬 소스에서 빌드 후 실행
docker compose up -d --build
```

같은 디렉터리의 `.env` 파일에 정의된 `GYMBAG_*` 변수도 자동으로 적용됩니다:

```dotenv
GYMBAG_ACME_EMAIL=ops@example.com
GYMBAG_SSL_PROVIDER=zerossl
GYMBAG_VERIFY_LEVEL=3
GYMBAG_CLEARANCE_TTL=12h
GYMBAG_REMOTE_RULES=https://rules.example.com/waf.rules
```

---

## `Gymbagfile` 없이 여러 컨테이너 프록시

`GYMBAG_UPSTREAMS`로 여러 도메인을 서로 다른 컨테이너에 매핑할 수 있습니다:

```yaml
environment:
  GYMBAG_DOMAINS: app.example.com,api.example.com,docs.example.com
  GYMBAG_UPSTREAMS: "app.example.com=http://web:3000;api.example.com=http://api:8080;docs.example.com=http://docs:4000"
  GYMBAG_PROTECT_DOMAINS: app.example.com,docs.example.com
  GYMBAG_VERIFY_LEVEL: "3"
```

---

## 볼륨, 권한 및 보안 참고사항

- **영구 `/app/certs` 볼륨**: ACME 계정 키, 발급된 인증서, 로컬 PKI CA, 자동 생성 HMAC 시크릿(`/app/certs/secret`), 위협 인텔 캐시(`/app/certs/intel-cache`), 원격 규칙 캐시(`/app/certs/rules-cache`)가 저장됩니다. 컨테이너 재생성 시 중복 인증서 발급이나 기존 세션 무효화를 막기 위해 반드시 영구 볼륨을 마운트하세요.
- **호스트 디렉터리 마운트 권한**: 컨테이너는 UID/GID `10001`로 실행됩니다. `./certs:/app/certs`처럼 호스트 디렉터리를 마운트할 경우 UID `10001`에 쓰기 권한을 부여하세요:
  ```bash
  mkdir -p ./certs
  sudo chown -R 10001:10001 ./certs
  ```
- **ACME HTTP-01을 위한 `80`/`443` 포트**: 공인 ACME CA 검증(`/.well-known/acme-challenge/*`)을 위해 호스트 `80:80`과 `443:443`을 매핑해야 합니다.
- **Docker에서 `*.internal` 로컬 개발**: 컨테이너는 호스트 `/etc/hosts`를 수정할 수 없으므로 `GYMBAG_AUTO_HOSTS=off`를 설정하고 필요시 비특권 포트(`-p 8080:80 -p 8443:443`)를 사용하며 `/app/certs/ca/neuro-sama-root-ca.pem`을 꺼내 호스트 OS에 한 번 신뢰 등록하세요.
- **Admin REST API**: 컨테이너에서 `GYMBAG_ADMIN=0.0.0.0:2019`를 활성화한 경우 반드시 호스트 루프백에만 바인딩(`-p 127.0.0.1:2019:2019`)하세요.

---

## 로컬 이미지 빌드

```bash
docker build \
  --build-arg VERSION=0.1.0 \
  -t gymbag:local .
```

멀티 스테이지 `Dockerfile`은 `golang:1.27-alpine`에서 정적 `CGO_ENABLED=0` 바이너리를 컴파일하고 `ca-certificates`와 `libcap`만 포함된 경량 `alpine:3.21` 런타임 이미지를 생성합니다.
