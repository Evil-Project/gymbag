# Docker 与 Docker Compose 部署

Gymbag 在每次发布和推送至 `main` 分支时，均会向 GitHub Container Registry (`GHCR`) 发布基于 Alpine 的多架构（`linux/amd64` 与 `linux/arm64`）官方容器镜像：

- **镜像地址**：`ghcr.io/evil-project/gymbag`
- **标签**：`latest`、`<semver>`（如 `0.1.0`）、`<major>.<minor>`、`<major>`、`main` 及 Git Commit SHA
- **运行用户**：非 root 用户 `gymbag`（UID `10001`），已赋予 `cap_net_bind_service=+ep`
- **暴露端口**：`80`（HTTP 跳转 + ACME HTTP-01）和 `443`（HTTPS 网关）
- **内置健康检查**：每 30 秒通过 TCP 探测 `127.0.0.1:80`

---

## 使用 `docker run` 快速启动

### 方式 A：挂载 `Gymbagfile`（推荐）

在当前目录创建 `Gymbagfile`：

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

挂载持久化卷用于保存证书、本地 CA 密钥、签名密钥、威胁情报缓存及远程规则缓存并启动容器：

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

### 方式 B：零配置文件（纯环境变量模式）

无需挂载配置文件，直接通过 `GYMBAG_*` 环境变量配置站点与防护策略：

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

### 方式 C：开启配置文件热重载 (`--watch`)

覆盖容器 `CMD` 传入 `--watch`，修改挂载的 `Gymbagfile` 时无需重启容器即可自动重载路由、规则与 SNI 证书：

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

::: tip 避免单文件挂载引起的 inode 替换问题
当以单文件形式挂载（`./Gymbagfile:/app/Gymbagfile:ro`）时，部分编辑器通过原子重命名保存会更换宿主机 inode，导致容器内无法感知更新。在 Docker 中使用 `--watch` 时，建议挂载整个配置目录（`-v "$(pwd)/config:/app/config:ro"`）并指定 `--config /app/config/Gymbagfile --watch`。
:::

---

## Docker Compose + Redis（生产多容器编排）

仓库自带开箱即用的 `compose.yaml`，可将 Gymbag 与 `redis:8-alpine` 一同编排运行。配置 `GYMBAG_REDIS_URL` 后，Gymbag 通过内置纯 Go RESP2 客户端与 Redis 通信，实现容器重启或跨多副本间的单次 PoW 质询与限流窗口共享：

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

在 `Gymbagfile` 中直接将 `reverse_proxy` 指向 Compose 服务名（如 `http://app:3000`），或指向宿主机服务（`http://host.docker.internal:3000`）：

```caddyfile
app.example.com {
    reverse_proxy http://app:3000
    verify 3
    verify_timeout 24h
}
```

启动服务（或从源码本地构建）：

```bash
# 使用 GHCR 预构建镜像
docker compose up -d

# 或从本地源码构建镜像
docker compose up -d --build
```

同目录下的 `.env` 文件中的 `GYMBAG_*` 变量会被自动加载：

```dotenv
GYMBAG_ACME_EMAIL=ops@example.com
GYMBAG_SSL_PROVIDER=zerossl
GYMBAG_VERIFY_LEVEL=3
GYMBAG_CLEARANCE_TTL=12h
GYMBAG_REMOTE_RULES=https://rules.example.com/waf.rules
```

---

## 无配置文件代理多个容器

可通过 `GYMBAG_UPSTREAMS` 将多个域名直接映射到不同的后端容器服务：

```yaml
environment:
  GYMBAG_DOMAINS: app.example.com,api.example.com,docs.example.com
  GYMBAG_UPSTREAMS: "app.example.com=http://web:3000;api.example.com=http://api:8080;docs.example.com=http://docs:4000"
  GYMBAG_PROTECT_DOMAINS: app.example.com,docs.example.com
  GYMBAG_VERIFY_LEVEL: "3"
```

---

## 数据卷、权限与安全注意事项

- **持久化 `/app/certs` 卷**：用于存储 ACME 账号密钥、已签发证书、本地 PKI CA、自动生成的 HMAC 签名密钥（`/app/certs/secret`）、威胁情报缓存（`/app/certs/intel-cache`）以及远程规则缓存（`/app/certs/rules-cache`）。请务必持久化该目录，避免容器重建后重复申请证书或导致已签发的访客 Cookie 失效。
- **宿主机目录挂载权限**：容器默认以 UID/GID `10001` 运行。如果将宿主机目录绑定挂载到 `/app/certs`（`./certs:/app/certs`），请确保 UID `10001` 拥有读写权限：
  ```bash
  mkdir -p ./certs
  sudo chown -R 10001:10001 ./certs
  ```
- **ACME HTTP-01 需要开放 80/443 端口**：公共 ACME CA 通过 80 端口验证 `/.well-known/acme-challenge/*`，请直接映射宿主机 `80:80` 与 `443:443`。
- **容器内 `*.internal` 本地开发**：请设置 `GYMBAG_AUTO_HOSTS=off`（容器无法修改宿主机 `/etc/hosts`），按需映射非特权端口（`-p 8080:80 -p 8443:443`），并将卷中的 `/app/certs/ca/neuro-sama-root-ca.pem` 拷贝到宿主机信任一次即可。
- **Admin REST API**：如果在容器内启用 `GYMBAG_ADMIN=0.0.0.0:2019`，请务必仅将端口绑定到宿主机回环地址（`-p 127.0.0.1:2019:2019`），切勿暴露至公网。

---

## 本地构建镜像

```bash
docker build \
  --build-arg VERSION=0.1.0 \
  -t gymbag:local .
```

多阶段 `Dockerfile` 在 `golang:1.27-alpine` 中编译静态 `CGO_ENABLED=0` 二进制文件，并打包进仅包含 `ca-certificates` 和 `libcap` 的轻量级 `alpine:3.21` 镜像。
