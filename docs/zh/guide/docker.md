# Docker 与 Docker Compose

每次打标签发布或推送至 `main` 分支，都会在 GitHub Container Registry（`GHCR`）构建多架构（`linux/amd64`、`linux/arm64`）Alpine 镜像：

- **镜像地址**：`ghcr.io/evil-project/gymbag`
- **标签**：`latest`、`<semver>`（如 `0.1.0`）、`<major>.<minor>`、`<major>`、`main` 以及 commit SHA
- **运行用户**：非 root 用户 `gymbag`（UID `10001`），具备 `cap_net_bind_service=+ep`
- **默认暴露端口**：`80`（HTTP 重定向与 ACME HTTP-01）和 `443`（HTTPS 网关）
- **健康检查**：每 30s 探测一次 `127.0.0.1:80` TCP 端口

## 使用 `docker run` 快速启动

### 方案 A：挂载 `Gymbagfile` 或 `gymbag.json`

在当前目录编写 `Gymbagfile`：

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

挂载持久化卷 `/app/certs`，防止重建容器后 ACME 证书、HMAC 密钥、威胁情报缓存及远程规则缓存丢失：

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

### 方案 B：仅用环境变量（不挂载配置文件）

如果不想单独维护配置文件，可以直接传入 `GYMBAG_*` 环境变量：

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

### 方案 C：暴露 SSH、Minecraft 与验证门户

在 Docker 中防护 SSH 或 Minecraft 服务器时，除了业务端口（`22` 或 `25565`），还需映射验证门户端口（`8443`）；如果不想在断开提示里显示容器 IP，传入 `GYMBAG_VERIFY_URL` 指定域名：

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

SSH 也有对应的一组变量（`GYMBAG_SSH_ENABLED`、`GYMBAG_SSH_PORT`、`GYMBAG_SSH_UPSTREAM`、`GYMBAG_SSH_VERIFY_LEVEL`、`GYMBAG_SSH_PROTECT`）。

### 方案 D：启用热重载（`--watch`）

覆盖 `CMD` 传入 `--watch`，修改配置后无需重启容器即可重载路由、防火墙规则与 SNI 证书：

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

::: tip 使用 `--watch` 时请挂载父目录
许多编辑器保存文件时采用原子重命名，会替换宿主机上的 inode。如果只绑定挂载了单个文件（`./Gymbagfile:/app/Gymbagfile:ro`），容器内看到的仍是旧 inode，`--watch` 无法感知变更。请改为挂载整个父目录（`./config:/app/config:ro`）。
:::

## Docker Compose 搭配 Redis

仓库自带的 `compose.yaml` 将 Gymbag 与 `redis:8-alpine` 编排在一起。设置 `GYMBAG_REDIS_URL` 后，Gymbag 会启用纯 Go RESP2 Redis 客户端，使 PoW 质询 nonce、固定窗口限流计数以及 SSH/Minecraft 验证凭证和 IP 放行记录在容器重启或跨实例间保持共享：

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

在 `Gymbagfile` 中将 `reverse_proxy` 指向 Compose 服务名（`http://app:3000`）或宿主机进程（`http://host.docker.internal:3000`）：

```caddyfile
app.example.com {
    reverse_proxy http://app:3000
    verify 3
    verify_timeout 24h
}
```

启动服务或基于本地源码构建镜像：

```bash
docker compose up -d
# 或本地构建：
docker compose up -d --build
```

放在 `compose.yaml` 同级目录的 `.env` 文件会自动覆盖 `Gymbagfile` 中的对应配置：

```dotenv
GYMBAG_ACME_EMAIL=ops@example.com
GYMBAG_SSL_PROVIDER=zerossl
GYMBAG_VERIFY_LEVEL=3
GYMBAG_CLEARANCE_TTL=12h
GYMBAG_REMOTE_RULES=https://rules.example.com/waf.rules
```

## 无配置文件代理多个容器

```yaml
environment:
  GYMBAG_DOMAINS: app.example.com,api.example.com,docs.example.com
  GYMBAG_UPSTREAMS: "app.example.com=http://web:3000;api.example.com=http://api:8080;docs.example.com=http://docs:4000"
  GYMBAG_PROTECT_DOMAINS: app.example.com,docs.example.com
  GYMBAG_VERIFY_LEVEL: "3"
```

## 挂载卷、权限与安全说明

- **持久化 `/app/certs` 卷**：该目录存放 ACME 账户密钥、签发证书、本地 CA 文件、HMAC 通行密钥（`/app/certs/secret`）、威胁/VPN/AI 情报缓存（`/app/certs/intel-cache`）以及远程规则缓存（`/app/certs/rules-cache`）。务必使用命名卷或宿主机目录持久化。
- **宿主机目录挂载权限**：Gymbag 在容器内以 UID/GID `10001` 运行。如果将宿主机目录绑定挂载到 `/app/certs`（`./certs:/app/certs`）而非使用命名卷，需确保该目录对 `10001` 可写：
  ```bash
  mkdir -p ./certs
  sudo chown -R 10001:10001 ./certs
  ```
- **ACME HTTP-01 需要 `80` 端口**：公网 CA 会通过 `80` 端口请求 `/.well-known/acme-challenge/*` 验证域名归属，因此请映射 `80:80` 和 `443:443`。
- **容器内使用本地域名 `*.internal`**：设置 `GYMBAG_AUTO_HOSTS=off`（容器内无法修改宿主机 `/etc/hosts`），按需映射非特权端口（`-p 8080:80 -p 8443:443`），并将卷内的 `/app/certs/ca/neuro-sama-root-ca.pem` 拷出到宿主机导入信任。
- **Admin REST API**：如果在容器内开启 `GYMBAG_ADMIN=0.0.0.0:2019`，宿主机侧请绑定回环地址（`-p 127.0.0.1:2019:2019`），避免将无鉴权的配置修改接口暴露到公网。

## 本地构建镜像

```bash
docker build \
  --build-arg VERSION=0.1.0 \
  -t gymbag:local .
```

`Dockerfile` 基于 `golang:1.27-alpine` 编译出静态 `CGO_ENABLED=0` 二进制文件，再拷贝到包含 `ca-certificates` 和 `libcap` 的 `alpine:3.21` 镜像中。
