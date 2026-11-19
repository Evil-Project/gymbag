# Docker y Docker Compose

Gymbag publica imágenes oficiales basadas en Alpine multiarquitectura (`linux/amd64` y `linux/arm64`) en GitHub Container Registry (`GHCR`) en cada release y push a `main`:

- **Imagen**: `ghcr.io/evil-project/gymbag`
- **Etiquetas**: `latest`, `<semver>` (p. ej. `0.1.0`), `<major>.<minor>`, `<major>`, `main` y SHA de commit
- **Usuario**: `gymbag` sin privilegios root (UID `10001`) con `cap_net_bind_service=+ep`
- **Puertos expuestos**: `80` (redirección HTTP + ACME HTTP-01) y `443` (gateway HTTPS)
- **Healthcheck integrado**: Sonda TCP a `127.0.0.1:80` cada 30 segundos

---

## Inicio Rápido con `docker run`

### Opción A: Montar un `Gymbagfile` (Recomendado)

Crea un archivo `Gymbagfile` en tu directorio actual:

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

Inicia Gymbag con un volumen persistente para certificados, claves CA locales, secreto HMAC, caché de amenazas y caché de reglas remotas:

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

### Opción B: Cero Configuración mediante Variables de Entorno

Ejecuta Gymbag sin montar ningún archivo configurando todo con variables `GYMBAG_*`:

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

### Opción C: Activar Recarga en Caliente (`--watch`)

Sobrescribe el `CMD` para pasar `--watch` y recargar rutas, reglas y certificados SNI al editar el `Gymbagfile` sin reiniciar el contenedor:

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

::: tip Evita problemas de reemplazo de inodos con archivos montados
Al montar un único archivo (`./Gymbagfile:/app/Gymbagfile:ro`), los editores que guardan mediante renombrado atómico cambian el inodo del host y el contenedor no ve el cambio. Para usar `--watch` en Docker, monta el directorio padre (`-v "$(pwd)/config:/app/config:ro"`) y pasa `--config /app/config/Gymbagfile --watch`.
:::

---

## Docker Compose con Redis (Despliegue en Producción)

El archivo `compose.yaml` del repositorio ejecuta Gymbag junto a `redis:8-alpine`. Con `GYMBAG_REDIS_URL`, el cliente RESP2 en Go puro comparte los nonces PoW de un solo uso y los límites de tasa entre reinicios o múltiples réplicas:

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

Dentro de `Gymbagfile`, apunta `reverse_proxy` al nombre del servicio (`http://app:3000`) o al host (`http://host.docker.internal:3000`):

```caddyfile
app.example.com {
    reverse_proxy http://app:3000
    verify 3
    verify_timeout 24h
}
```

Inicia la pila:

```bash
# Usando imagen oficial de GHCR
docker compose up -d

# O compilando localmente desde el repositorio
docker compose up -d --build
```

Las variables `GYMBAG_*` en un archivo `.env` junto a `compose.yaml` se aplican automáticamente:

```dotenv
GYMBAG_ACME_EMAIL=ops@example.com
GYMBAG_SSL_PROVIDER=zerossl
GYMBAG_VERIFY_LEVEL=3
GYMBAG_CLEARANCE_TTL=12h
GYMBAG_REMOTE_RULES=https://rules.example.com/waf.rules
```

---

## Proxy a Múltiples Contenedores sin `Gymbagfile`

```yaml
environment:
  GYMBAG_DOMAINS: app.example.com,api.example.com,docs.example.com
  GYMBAG_UPSTREAMS: "app.example.com=http://web:3000;api.example.com=http://api:8080;docs.example.com=http://docs:4000"
  GYMBAG_PROTECT_DOMAINS: app.example.com,docs.example.com
  GYMBAG_VERIFY_LEVEL: "3"
```

---

## Volúmenes, Permisos y Seguridad

- **Volumen persistente `/app/certs`**: Almacena claves de cuenta ACME, certificados emitidos, CA local, secreto HMAC (`/app/certs/secret`), caché de amenazas (`/app/certs/intel-cache`) y caché de reglas remotas (`/app/certs/rules-cache`). Móntalo siempre de forma persistente.
- **Permisos en montajes del host**: El contenedor corre como UID/GID `10001`. Si montas una carpeta local (`./certs:/app/certs`), asigna permisos de escritura:
  ```bash
  mkdir -p ./certs
  sudo chown -R 10001:10001 ./certs
  ```
- **ACME HTTP-01 requiere los puertos `80` y `443`**: Mapea `80:80` y `443:443` para validaciones ACME públicas.
- **Desarrollo local `*.internal` en Docker**: Configura `GYMBAG_AUTO_HOSTS=off` y extrae `/app/certs/ca/neuro-sama-root-ca.pem` para confiarlo en tu sistema operativo.
- **API REST de Administración**: Si activas `GYMBAG_ADMIN=0.0.0.0:2019` en el contenedor, expón el puerto únicamente en loopback (`-p 127.0.0.1:2019:2019`).

---

## Compilar la Imagen Localmente

```bash
docker build \
  --build-arg VERSION=0.1.0 \
  -t gymbag:local .
```
