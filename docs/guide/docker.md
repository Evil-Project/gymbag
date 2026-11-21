# Docker & Docker Compose

Every release, and every push to `main`, publishes multi-arch (`linux/amd64`, `linux/arm64`) Alpine images to GitHub Container Registry (`GHCR`):

- **Image**: `ghcr.io/evil-project/gymbag`
- **Tags**: `latest`, `<semver>` (e.g. `0.1.0`), `<major>.<minor>`, `<major>`, `main`, and commit SHAs
- **Runtime user**: non-root `gymbag` (UID `10001`) with `cap_net_bind_service=+ep`
- **Ports**: `80` (HTTP redirect + ACME HTTP-01) and `443` (HTTPS gateway)
- **Healthcheck**: TCP probe against `127.0.0.1:80` every 30 seconds

---

## Quick Start with `docker run`

### Option A: Mount a `Gymbagfile` (Recommended)

Create a `Gymbagfile` in your current directory:

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

Then start Gymbag with a persistent volume for certificates, local CA keys, clearance secrets, threat-intel caches, and remote rule caches:

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

If you would rather not mount a config file, set up sites and policies with `GYMBAG_*` variables:

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

### Option C: Hot Reload (`--watch`)

Override the container `CMD` to pass `--watch`. Editing the mounted `Gymbagfile` then reloads routes, firewall rules, and SNI certificates without a restart:

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

::: tip Avoid Inode Replacement with Bind-Mounted Files
When mounting a single file (`./Gymbagfile:/app/Gymbagfile:ro`), editors that write via atomic rename replace the host inode and the container will not see the new file. For `--watch` in Docker, mount the parent directory (`-v "$(pwd)/config:/app/config:ro"`) and pass `--config /app/config/Gymbagfile --watch`.
:::

---

## Docker Compose with Redis (Production Multi-Container Setup)

The repo has a `compose.yaml` that runs Gymbag next to `redis:8-alpine`. Once `GYMBAG_REDIS_URL` is set, Gymbag talks to Redis through its own small pure-Go RESP2 client, so single-use PoW challenges and per-IP rate-limit windows survive container restarts and work across replicas.

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

In `Gymbagfile`, either point `reverse_proxy` at the Compose service name (`http://app:3000`) or use `http://host.docker.internal:3000` for things running on the host itself:

```caddyfile
app.example.com {
    reverse_proxy http://app:3000
    verify 3
    verify_timeout 24h
}
```

Start the stack, or build it from your working copy:

```bash
# Using prebuilt GHCR image
docker compose up -d

# Or building from local source checkout
docker compose up -d --build
```

A `.env` file sitting next to `compose.yaml` gets picked up automatically:

```dotenv
GYMBAG_ACME_EMAIL=ops@example.com
GYMBAG_SSL_PROVIDER=zerossl
GYMBAG_VERIFY_LEVEL=3
GYMBAG_CLEARANCE_TTL=12h
GYMBAG_REMOTE_RULES=https://rules.example.com/waf.rules
```

---

## Proxying Multiple Containers Without a `Gymbagfile`

If you just want domains mapped to services, `GYMBAG_UPSTREAMS` does the whole job:

```yaml
environment:
  GYMBAG_DOMAINS: app.example.com,api.example.com,docs.example.com
  GYMBAG_UPSTREAMS: "app.example.com=http://web:3000;api.example.com=http://api:8080;docs.example.com=http://docs:4000"
  GYMBAG_PROTECT_DOMAINS: app.example.com,docs.example.com
  GYMBAG_VERIFY_LEVEL: "3"
```

---

## Volumes, Permissions & Security Notes

- **Persistent `/app/certs` volume**: Holds ACME account keys, issued certificates, local PKI CA files, the generated HMAC clearance secret (`/app/certs/secret`), the IP threat-intel cache (`/app/certs/intel-cache`), and the remote rules cache (`/app/certs/rules-cache`). Keep it on a named volume or host directory, otherwise every container recreation re-issues certificates and logs visitors out.
- **Host bind mount permissions**: The container runs as UID/GID `10001`. With a host bind mount (`./certs:/app/certs`) instead of a named volume, let that UID write to it:
  ```bash
  mkdir -p ./certs
  sudo chown -R 10001:10001 ./certs
  ```
- **ACME HTTP-01 needs ports `80` and `443`**: Public CAs check ownership on port `80` (`/.well-known/acme-challenge/*`), so map `80:80` and `443:443` straight through.
- **Local `*.internal` development in Docker**: Set `GYMBAG_AUTO_HOSTS=off` (a container cannot edit the host's `/etc/hosts`), map unprivileged ports if you like (`-p 8080:80 -p 8443:443`), and copy `/app/certs/ca/neuro-sama-root-ca.pem` out of the volume so you can trust it on the host.
- **Admin REST API**: If you turn on `GYMBAG_ADMIN=0.0.0.0:2019` in a container, publish it only on host loopback (`-p 127.0.0.1:2019:2019`) so it never faces the internet.

---

## Building the Image Locally

```bash
docker build \
  --build-arg VERSION=0.1.0 \
  -t gymbag:local .
```

The multi-stage `Dockerfile` builds a static `CGO_ENABLED=0` binary on `golang:1.27-alpine` and copies it into a small `alpine:3.21` runtime with `ca-certificates` and `libcap`.
