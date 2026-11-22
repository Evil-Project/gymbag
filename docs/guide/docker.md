# Docker & Docker Compose

Tagged releases and pushes to `main` publish multi-arch (`linux/amd64`, `linux/arm64`) Alpine images on GitHub Container Registry (`GHCR`):

- **Image**: `ghcr.io/evil-project/gymbag`
- **Tags**: `latest`, `<semver>` (e.g. `0.1.0`), `<major>.<minor>`, `<major>`, `main`, and commit SHAs
- **Runtime user**: non-root `gymbag` (UID `10001`) with `cap_net_bind_service=+ep`
- **Ports**: `80` (HTTP redirect + ACME HTTP-01) and `443` (HTTPS gateway) exposed by default
- **Healthcheck**: TCP probe on `127.0.0.1:80` every 30s

## Quick Start with `docker run`

### Option A: Mount a `Gymbagfile` or `gymbag.json`

Write a `Gymbagfile` in your current directory:

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

Start Gymbag with a persistent `/app/certs` volume so ACME certs, secrets, threat-intel caches, and remote-rule caches survive container rebuilds:

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

### Option B: Environment Variables Only

If you don't want to mount a config file, pass `GYMBAG_*` variables directly:

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

### Option C: Exposing SSH, Minecraft & Verify Portal

To protect an SSH server or Minecraft server from Docker, publish the service port (`22` or `25565`) plus the verification portal port (`8443`), and pass `GYMBAG_VERIFY_URL` if you want a custom domain in the disconnect banner instead of the container's IP:

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

The same variables work for SSH (`GYMBAG_SSH_ENABLED`, `GYMBAG_SSH_PORT`, `GYMBAG_SSH_UPSTREAM`, `GYMBAG_SSH_VERIFY_LEVEL`, `GYMBAG_SSH_PROTECT`).

### Option D: Hot Reload (`--watch`)

Override `CMD` to pass `--watch` so editing the config reloads routes, firewall rules, and SNI certificates without restarting:

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

::: tip Mount the Parent Directory for `--watch`
Editors that save files via atomic rename replace the host inode. If you bind-mount a single file (`./Gymbagfile:/app/Gymbagfile:ro`), the container keeps reading the old inode and `--watch` will not pick up edits. Mount the parent directory (`./config:/app/config:ro`) instead.
:::

## Docker Compose with Redis

The repository's `compose.yaml` runs Gymbag next to `redis:8-alpine`. With `GYMBAG_REDIS_URL` set, Gymbag uses its pure-Go RESP2 client so PoW challenge nonces, fixed-window rate limits, and SSH/Minecraft verification tickets and IP clearances persist across restarts and work across multiple containers:

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

In `Gymbagfile`, point `reverse_proxy` at the Compose service (`http://app:3000`) or `http://host.docker.internal:3000` for host-bound apps:

```caddyfile
app.example.com {
    reverse_proxy http://app:3000
    verify 3
    verify_timeout 24h
}
```

Start the stack, or build from the local checkout:

```bash
docker compose up -d
# Or build locally:
docker compose up -d --build
```

A `.env` file sitting next to `compose.yaml` overrides values in `Gymbagfile`:

```dotenv
GYMBAG_ACME_EMAIL=ops@example.com
GYMBAG_SSL_PROVIDER=zerossl
GYMBAG_VERIFY_LEVEL=3
GYMBAG_CLEARANCE_TTL=12h
GYMBAG_REMOTE_RULES=https://rules.example.com/waf.rules
```

## Proxying Multiple Containers Without a Config File

```yaml
environment:
  GYMBAG_DOMAINS: app.example.com,api.example.com,docs.example.com
  GYMBAG_UPSTREAMS: "app.example.com=http://web:3000;api.example.com=http://api:8080;docs.example.com=http://docs:4000"
  GYMBAG_PROTECT_DOMAINS: app.example.com,docs.example.com
  GYMBAG_VERIFY_LEVEL: "3"
```

## Volumes, Permissions & Security Notes

- **Persistent `/app/certs` volume**: Stores ACME account keys, issued certificates, Local CA files, the HMAC clearance secret (`/app/certs/secret`), the threat/VPN/AI intel cache (`/app/certs/intel-cache`), and the remote-rule cache (`/app/certs/rules-cache`). Always back this with a named volume or host directory.
- **Host bind mount permissions**: Gymbag runs as UID/GID `10001` inside the container. If you bind-mount `./certs:/app/certs` instead of using a named volume, make it writable by `10001`:
  ```bash
  mkdir -p ./certs
  sudo chown -R 10001:10001 ./certs
  ```
- **ACME HTTP-01 needs port `80`**: Public CAs validate ownership via `/.well-known/acme-challenge/*` on port `80`, so map `80:80` and `443:443`.
- **Local `*.internal` inside Docker**: Set `GYMBAG_AUTO_HOSTS=off` (the container cannot edit the host's `/etc/hosts`), map unprivileged ports if needed (`-p 8080:80 -p 8443:443`), and copy `/app/certs/ca/neuro-sama-root-ca.pem` out of the volume to trust it on the host.
- **Admin REST API**: If you enable `GYMBAG_ADMIN=0.0.0.0:2019` inside a container, bind the host port to loopback (`-p 127.0.0.1:2019:2019`) so it never exposes unauthenticated mutations to the internet.

## Building the Image Locally

```bash
docker build \
  --build-arg VERSION=0.1.0 \
  -t gymbag:local .
```

The `Dockerfile` builds a static `CGO_ENABLED=0` binary on `golang:1.27-alpine` and copies it into `alpine:3.21` with `ca-certificates` and `libcap`.
