# Docker & Docker Compose

Gymbag はすべてのリリースおよび `main` ブランチへのプッシュ時に、GitHub Container Registry (`GHCR`) へ Alpine ベースのマルチアーキテクチャ（`linux/amd64`・`linux/arm64`）公式イメージを公開しています：

- **イメージ**: `ghcr.io/evil-project/gymbag`
- **タグ**: `latest`、`<semver>`（例：`0.1.0`）、`<major>.<minor>`、`<major>`、`main`、コミット SHA
- **実行ユーザー**: 非 root ユーザー `gymbag`（UID `10001`、`cap_net_bind_service=+ep` 付与済み）
- **公開ポート**: `80`（HTTP リダイレクト + ACME HTTP-01）および `443`（HTTPS ゲートウェイ）
- **ヘルスチェック**: 30秒ごとに `127.0.0.1:80` へ TCP 疎通確認

---

## `docker run` でのクイックスタート

### 方法 A：`Gymbagfile` をマウント（推奨）

カレントディレクトリに `Gymbagfile` を作成します：

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

証明書・ローカル CA 鍵・クリアランス署名鍵・脅威インテルキャッシュ・リモートルールキャッシュ用の永続ボリュームを指定して起動します：

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

### 方法 B：設定ファイルなし（環境変数のみ）

設定ファイルをマウントせず、`GYMBAG_*` 環境変数だけでサイトとポリシーを構成できます：

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

### 方法 C：ホットリロードを有効化 (`--watch`)

コンテナの `CMD` を上書きして `--watch` を渡すと、`Gymbagfile` の編集時にコンテナを再起動せずルート・ルール・SNI 証明書を自動再読み込みします：

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

::: tip 単一ファイルバインドマウント時の inode 置換に注意
1ファイル単位でマウント（`./Gymbagfile:/app/Gymbagfile:ro`）すると、アトミックリネームで保存するエディタではホスト側の inode が変わりコンテナから変更が見えなくなることがあります。Docker で `--watch` を使う場合は親ディレクトリごとマウント（`-v "$(pwd)/config:/app/config:ro"`）し、`--config /app/config/Gymbagfile --watch` を指定してください。
:::

---

## Docker Compose + Redis（本番マルチコンテナ構成）

リポジトリ付属の `compose.yaml` は、`redis:8-alpine` とともに Gymbag を起動します。`GYMBAG_REDIS_URL` を設定すると、純 Go 製 RESP2 Redis クライアントにより使い捨て PoW ナンスとレート制限ウィンドウがコンテナ再起動や複数レプリカ間で共有されます：

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

`Gymbagfile` 内では Compose サービス名（`http://app:3000`）またはホストサービス（`http://host.docker.internal:3000`）をそのまま指定できます：

```caddyfile
app.example.com {
    reverse_proxy http://app:3000
    verify 3
    verify_timeout 24h
}
```

起動コマンド：

```bash
# GHCR ビルド済みイメージを使用
docker compose up -d

# ローカルソースからビルドして起動
docker compose up -d --build
```

同じディレクトリの `.env` に書いた `GYMBAG_*` 変数も自動で読み込まれます：

```dotenv
GYMBAG_ACME_EMAIL=ops@example.com
GYMBAG_SSL_PROVIDER=zerossl
GYMBAG_VERIFY_LEVEL=3
GYMBAG_CLEARANCE_TTL=12h
GYMBAG_REMOTE_RULES=https://rules.example.com/waf.rules
```

---

## `Gymbagfile` なしで複数コンテナをプロキシ

`GYMBAG_UPSTREAMS` を使って複数ドメインを異なるコンテナへマッピングできます：

```yaml
environment:
  GYMBAG_DOMAINS: app.example.com,api.example.com,docs.example.com
  GYMBAG_UPSTREAMS: "app.example.com=http://web:3000;api.example.com=http://api:8080;docs.example.com=http://docs:4000"
  GYMBAG_PROTECT_DOMAINS: app.example.com,docs.example.com
  GYMBAG_VERIFY_LEVEL: "3"
```

---

## ボリューム・権限・セキュリティ上の注意

- **永続ボリューム `/app/certs`**：ACME アカウント鍵、発行済み証明書、ローカル PKI CA、自動生成の HMAC シークレット（`/app/certs/secret`）、脅威インテルキャッシュ（`/app/certs/intel-cache`）、リモートルールキャッシュ（`/app/certs/rules-cache`）が保存されます。再作成時の不要な証明書再発行や既存セッション無効化を防ぐため必ず永続化してください。
- **ホストディレクトリをマウントする場合の権限**：コンテナは UID/GID `10001` で動作します。名前付きボリュームではなく `./certs:/app/certs` をマウントする場合は書き込み権限を付与してください：
  ```bash
  mkdir -p ./certs
  sudo chown -R 10001:10001 ./certs
  ```
- **ACME HTTP-01 にはポート `80` / `443` が必要**：公開 ACME CA の検証（`/.well-known/acme-challenge/*`）のため、`80:80` と `443:443` を公開してください。
- **コンテナでの `*.internal` ローカル開発**：コンテナはホストの `/etc/hosts` を編集できないため `GYMBAG_AUTO_HOSTS=off` を設定し、必要に応じて非特権ポート（`-p 8080:80 -p 8443:443`）へマップし、`/app/certs/ca/neuro-sama-root-ca.pem` を取り出してホスト OS で1回だけ信頼登録してください。
- **Admin REST API**：コンテナ内で `GYMBAG_ADMIN=0.0.0.0:2019` を有効化する場合は、必ずホストのループバックのみへ公開（`-p 127.0.0.1:2019:2019`）してください。

---

## イメージをローカルでビルド

```bash
docker build \
  --build-arg VERSION=0.1.0 \
  -t gymbag:local .
```

マルチステージ `Dockerfile` は `golang:1.27-alpine` で静的 `CGO_ENABLED=0` バイナリをビルドし、`ca-certificates` と `libcap` のみを含む軽量な `alpine:3.21` ランタイムイメージを生成します。
