# Docker 與 Docker Compose

每次發布版本標籤或推送至 `main` 分支時，GitHub Container Registry（`GHCR`）皆會自動建置並發布多架構（`linux/amd64`、`linux/arm64`）的 Alpine 容器映像檔：

- **映像檔**：`ghcr.io/evil-project/gymbag`
- **標籤**：`latest`、`<semver>`（例如 `0.1.0`）、`<major>.<minor>`、`<major>`、`main` 與 Commit SHA
- **執行使用者**：非 root 使用者 `gymbag`（UID `10001`），具備 `cap_net_bind_service=+ep` 權限
- **預設開放連接埠**：`80`（HTTP 重新導向 + ACME HTTP-01）與 `443`（HTTPS 閘道器）
- **健康檢查**：每 30 秒對 `127.0.0.1:80` 執行一次 TCP 探測

## 使用 `docker run` 快速啟動

### 方案 A：掛載 `Gymbagfile` 或 `gymbag.json`

在目前目錄下建立一份 `Gymbagfile`：

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

啟動 Gymbag 並掛載持久化磁碟區 `/app/certs`，確保 ACME 憑證、金鑰、威脅情資快取與遠端規則快取在容器重建後依然保留：

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

### 方案 B：純環境變數啟動

若不想掛載設定檔，可直接傳入 `GYMBAG_*` 環境變數：

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

### 方案 C：開放 SSH、Minecraft 與驗證入口

若要透過 Docker 保護 SSH 或 Minecraft 伺服器，請發布對應的服務連接埠（`22` 或 `25565`）與驗證入口連接埠（`8443`）；若希望斷線提示畫面顯示自訂網域而非容器 IP，請傳入 `GYMBAG_VERIFY_URL`：

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

SSH 亦支援相同的環境變數（`GYMBAG_SSH_ENABLED`、`GYMBAG_SSH_PORT`、`GYMBAG_SSH_UPSTREAM`、`GYMBAG_SSH_VERIFY_LEVEL`、`GYMBAG_SSH_PROTECT`）。

### 方案 D：熱重載（`--watch`）

覆寫 `CMD` 並傳入 `--watch`，即可在修改設定檔後不需重啟容器便即時重載路由、防火牆規則與 SNI 憑證：

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

::: tip 使用 `--watch` 時請掛載上層目錄
許多編輯器存檔時會採用原子重新命名（atomic rename），這會替換主機端檔案的 inode。若只單獨綁定掛載單一檔案（`./Gymbagfile:/app/Gymbagfile:ro`），容器內會繼續讀取舊的 inode，導致 `--watch` 無法偵測到修改。請改為掛載上層目錄（`./config:/app/config:ro`）。
:::

## 搭配 Redis 使用 Docker Compose

專案內附的 `compose.yaml` 會同時啟動 Gymbag 與 `redis:8-alpine`。設定 `GYMBAG_REDIS_URL` 後，Gymbag 會透過純 Go 實作的 RESP2 用戶端將 PoW 挑戰 Nonce、固定視窗速率限制、SSH/Minecraft 驗證票證與 IP 通行紀錄存入 Redis，即使容器重啟或跨多個實例部署也能共用狀態：

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

在 `Gymbagfile` 中，將 `reverse_proxy` 指向 Compose 服務名稱（`http://app:3000`），或指向主機服務（`http://host.docker.internal:3000`）：

```caddyfile
app.example.com {
    reverse_proxy http://app:3000
    verify 3
    verify_timeout 24h
}
```

啟動服務堆疊，或從本地原始碼直接建置：

```bash
docker compose up -d
# 或於本地建置並啟動：
docker compose up -d --build
```

放在 `compose.yaml` 同層目錄的 `.env` 檔案會覆寫 `Gymbagfile` 中的對應設定：

```dotenv
GYMBAG_ACME_EMAIL=ops@example.com
GYMBAG_SSL_PROVIDER=zerossl
GYMBAG_VERIFY_LEVEL=3
GYMBAG_CLEARANCE_TTL=12h
GYMBAG_REMOTE_RULES=https://rules.example.com/waf.rules
```

## 不使用設定檔直接代理多個容器

```yaml
environment:
  GYMBAG_DOMAINS: app.example.com,api.example.com,docs.example.com
  GYMBAG_UPSTREAMS: "app.example.com=http://web:3000;api.example.com=http://api:8080;docs.example.com=http://docs:4000"
  GYMBAG_PROTECT_DOMAINS: app.example.com,docs.example.com
  GYMBAG_VERIFY_LEVEL: "3"
```

## 磁碟區、權限與安全性注意事項

- **持久化 `/app/certs` 磁碟區**：此目錄存放 ACME 帳號金鑰、已簽發憑證、本機 CA 檔案、HMAC 通行簽章金鑰（`/app/certs/secret`）、威脅/VPN/AI 情資快取（`/app/certs/intel-cache`）與遠端規則快取（`/app/certs/rules-cache`）。請務必使用具名磁碟區或主機目錄加以保留。
- **主機綁定掛載權限**：Gymbag 在容器內以 UID/GID `10001` 執行。若採用 `./certs:/app/certs` 綁定掛載而非具名磁碟區，請確保該目錄對 `10001` 具有寫入權限：
  ```bash
  mkdir -p ./certs
  sudo chown -R 10001:10001 ./certs
  ```
- **ACME HTTP-01 需要連接埠 `80`**：公開 CA 會透過連接埠 `80` 的 `/.well-known/acme-challenge/*` 驗證網域所有權，因此請務必映射 `80:80` 與 `443:443`。
- **在 Docker 內使用本機 `*.internal`**：請設定 `GYMBAG_AUTO_HOSTS=off`（容器無法修改主機的 `/etc/hosts`），視需求映射非特權連接埠（`-p 8080:80 -p 8443:443`），並將 `/app/certs/ca/neuro-sama-root-ca.pem` 複製到主機端加入信任。
- **Admin REST API**：若在容器內啟用 `GYMBAG_ADMIN=0.0.0.0:2019`，請務必將主機端連接埠綁定於本機迴環介面（`-p 127.0.0.1:2019:2019`），避免未經驗證的設定變更介面暴露到網際網路。

## 本地建置映像檔

```bash
docker build \
  --build-arg VERSION=0.1.0 \
  -t gymbag:local .
```

`Dockerfile` 會在 `golang:1.27-alpine` 中編譯出 `CGO_ENABLED=0` 的靜態執行檔，再複製到包含 `ca-certificates` 與 `libcap` 的 `alpine:3.21` 映像檔中。
