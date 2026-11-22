# Docker y Docker Compose

Las versiones etiquetadas y los envíos a `main` publican imágenes Alpine multiarquitectura (`linux/amd64`, `linux/arm64`) en GitHub Container Registry (`GHCR`):

- **Imagen**: `ghcr.io/evil-project/gymbag`
- **Etiquetas**: `latest`, `<semver>` (p. ej. `0.1.0`), `<major>.<minor>`, `<major>`, `main` y SHAs de commit
- **Usuario de ejecución**: usuario sin privilegios `gymbag` (UID `10001`) con `cap_net_bind_service=+ep`
- **Puertos**: `80` (redirección HTTP + ACME HTTP-01) y `443` (pasarela HTTPS) expuestos por defecto
- **Healthcheck**: comprobación TCP en `127.0.0.1:80` cada 30s

## Inicio rápido con `docker run`

### Opción A: montar un `Gymbagfile` o `gymbag.json`

Crea un `Gymbagfile` en tu directorio actual:

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

Inicia Gymbag con un volumen `/app/certs` persistente para conservar certificados ACME, secretos, cachés de inteligencia de amenazas y cachés de reglas remotas entre recreaciones del contenedor:

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

### Opción B: solo variables de entorno

Si prefieres no montar un archivo de configuración, pasa variables `GYMBAG_*` directamente:

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

### Opción C: exponer SSH, Minecraft y el portal de verificación

Para proteger un servidor SSH o de Minecraft desde Docker, publica el puerto del servicio (`22` o `25565`) junto con el puerto del portal de verificación (`8443`), y pasa `GYMBAG_VERIFY_URL` si quieres usar un dominio propio en el mensaje de desconexión en lugar de la IP del contenedor:

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

Las mismas variables existen para SSH (`GYMBAG_SSH_ENABLED`, `GYMBAG_SSH_PORT`, `GYMBAG_SSH_UPSTREAM`, `GYMBAG_SSH_VERIFY_LEVEL`, `GYMBAG_SSH_PROTECT`).

### Opción D: recarga en caliente (`--watch`)

Sobrescribe `CMD` para pasar `--watch` y que al editar la configuración se recarguen rutas, reglas de firewall y certificados SNI sin reiniciar:

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

::: tip Monta el directorio padre para `--watch`
Los editores que guardan archivos mediante renombrado atómico sustituyen el inodo en el host. Si montas un único archivo (`./Gymbagfile:/app/Gymbagfile:ro`), el contenedor seguirá leyendo el inodo antiguo y `--watch` no detectará los cambios. Monta el directorio padre (`./config:/app/config:ro`) en su lugar.
:::

## Docker Compose con Redis

El archivo `compose.yaml` del repositorio levanta Gymbag junto a `redis:8-alpine`. Con `GYMBAG_REDIS_URL` configurado, Gymbag utiliza su cliente Redis RESP2 en Go puro para que los nonces de desafíos PoW, los límites de tasa de ventana fija y los tickets de verificación y autorizaciones IP de SSH/Minecraft persistan entre reinicios y funcionen entre varios contenedores:

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

En `Gymbagfile`, apunta `reverse_proxy` al servicio de Compose (`http://app:3000`) o a `http://host.docker.internal:3000` si la aplicación corre en el host:

```caddyfile
app.example.com {
    reverse_proxy http://app:3000
    verify 3
    verify_timeout 24h
}
```

Inicia la pila, o compila desde la copia local:

```bash
docker compose up -d
# O compilar localmente:
docker compose up -d --build
```

Un archivo `.env` junto a `compose.yaml` sobrescribe los valores de `Gymbagfile`:

```dotenv
GYMBAG_ACME_EMAIL=ops@example.com
GYMBAG_SSL_PROVIDER=zerossl
GYMBAG_VERIFY_LEVEL=3
GYMBAG_CLEARANCE_TTL=12h
GYMBAG_REMOTE_RULES=https://rules.example.com/waf.rules
```

## Proxy de múltiples contenedores sin archivo de configuración

```yaml
environment:
  GYMBAG_DOMAINS: app.example.com,api.example.com,docs.example.com
  GYMBAG_UPSTREAMS: "app.example.com=http://web:3000;api.example.com=http://api:8080;docs.example.com=http://docs:4000"
  GYMBAG_PROTECT_DOMAINS: app.example.com,docs.example.com
  GYMBAG_VERIFY_LEVEL: "3"
```

## Volúmenes, permisos y notas de seguridad

- **Volumen `/app/certs` persistente**: guarda claves de cuenta ACME, certificados emitidos, archivos de la CA local, el secreto HMAC (`/app/certs/secret`), la caché de inteligencia de amenazas/VPN/IA (`/app/certs/intel-cache`) y la caché de reglas remotas (`/app/certs/rules-cache`). Respáldalo siempre con un volumen nombrado o un directorio en el host.
- **Permisos en montajes bind del host**: Gymbag corre como UID/GID `10001` dentro del contenedor. Si montas `./certs:/app/certs` en lugar de usar un volumen nombrado, hazlo escribible para `10001`:
  ```bash
  mkdir -p ./certs
  sudo chown -R 10001:10001 ./certs
  ```
- **ACME HTTP-01 necesita el puerto `80`**: las CAs públicas validan la propiedad vía `/.well-known/acme-challenge/*` en el puerto `80`, así que mapea `80:80` y `443:443`.
- **`*.internal` local dentro de Docker**: fija `GYMBAG_AUTO_HOSTS=off` (el contenedor no puede editar `/etc/hosts` del host), mapea puertos sin privilegios si hace falta (`-p 8080:80 -p 8443:443`) y copia `/app/certs/ca/neuro-sama-root-ca.pem` del volumen para confiar en él en el host.
- **Admin REST API**: si habilitas `GYMBAG_ADMIN=0.0.0.0:2019` dentro de un contenedor, enlaza el puerto del host a loopback (`-p 127.0.0.1:2019:2019`) para no exponer cambios sin autenticar a internet.

## Compilar la imagen localmente

```bash
docker build \
  --build-arg VERSION=0.1.0 \
  -t gymbag:local .
```

El `Dockerfile` compila un binario estático `CGO_ENABLED=0` sobre `golang:1.27-alpine` y lo copia a `alpine:3.21` con `ca-certificates` y `libcap`.
