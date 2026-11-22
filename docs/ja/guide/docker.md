# Docker & Docker Compose

タグ付きリリースおよび `main` ブランチへのプッシュごとに、マルチアーキテクチャ（`linux/amd64`、`linux/arm64`）の Alpine イメージを GitHub Container Registry (`GHCR`) へ公開しています。

- **イメージ**: `ghcr.io/evil-project/gymbag`
- **タグ**: `latest`、`<semver>`（例: `0.1.0`）、`<major>.<minor>`、`<major>`、`main`、コミット SHA
- **実行ユーザー**: 非 root ユーザー `gymbag`（UID `10001`）、`cap_net_bind_service=+ep` 付与済み
- **ポート**: デフォルトで `80`（HTTP リダイレクト + ACME HTTP-01）と `443`（HTTPS ゲートウェイ）を公開
- **ヘルスチェック**: 30 秒ごとに `127.0.0.1:80` へ TCP プローブを実行

## `docker run` クイックスタート

### パターン A: `Gymbagfile` または `gymbag.json` をマウント

カレントディレクトリに `Gymbagfile` を用意します。

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

ACME 証明書、シークレット、脅威インテルキャッシュ、リモートルールキャッシュがコンテナ再作成後も残るよう、`/app/certs` を永続ボリュームとしてマウントして起動します。

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

### パターン B: 環境変数のみ

設定ファイルをマウントしない場合は `GYMBAG_*` 変数を直接渡します。

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

### パターン C: SSH・Minecraft・検証ポータルの公開

Docker から SSH サーバーや Minecraft サーバーを保護するには、サービスポート（`22` または `25565`）に加えて検証ポータルのポート（`8443`）を公開します。切断バナー内にコンテナ IP ではなく独自ドメインを表示したい場合は `GYMBAG_VERIFY_URL` を指定してください。

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

SSH でも同じパターンの変数（`GYMBAG_SSH_ENABLED`、`GYMBAG_SSH_PORT`、`GYMBAG_SSH_UPSTREAM`、`GYMBAG_SSH_VERIFY_LEVEL`、`GYMBAG_SSH_PROTECT`）を使えます。

### パターン D: ホットリロード（`--watch`）

`CMD` を上書きして `--watch` を渡すと、設定を編集した際に再起動なしでルート、ファイアウォールルール、SNI 証明書を再読み込みします。

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

::: tip `--watch` を使うときは親ディレクトリをマウントする
アトミックリネームでファイルを保存するエディタではホスト側の inode が差し替わります。単一ファイル（`./Gymbagfile:/app/Gymbagfile:ro`）でバインドマウントするとコンテナ側が古い inode を読み続けるため、`--watch` が変更を検知できません。代わりに親ディレクトリ（`./config:/app/config:ro`）をマウントしてください。
:::

## Redis を使った Docker Compose

リポジトリの `compose.yaml` は Gymbag と `redis:8-alpine` を並列で起動します。`GYMBAG_REDIS_URL` を指定すると純 Go 製 RESP2 クライアントが使われ、PoW チャレンジナンス、固定ウィンドウのレート制限、SSH/Minecraft の検証チケットと IP クリアランスが再起動や複数コンテナ間で共有されます。

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

`Gymbagfile` 内では Compose のサービス名（`http://app:3000`）またはホスト上のアプリ（`http://host.docker.internal:3000`）を指定します。

```caddyfile
app.example.com {
    reverse_proxy http://app:3000
    verify 3
    verify_timeout 24h
}
```

起動、またはローカルソースからのビルドを実行します。

```bash
docker compose up -d
# ローカルでビルドする場合:
docker compose up -d --build
```

`compose.yaml` と同じ階層に `.env` を置くと `Gymbagfile` の値を上書きできます。

```dotenv
GYMBAG_ACME_EMAIL=ops@example.com
GYMBAG_SSL_PROVIDER=zerossl
GYMBAG_VERIFY_LEVEL=3
GYMBAG_CLEARANCE_TTL=12h
GYMBAG_REMOTE_RULES=https://rules.example.com/waf.rules
```

## 設定ファイルなしで複数コンテナをプロキシ

```yaml
environment:
  GYMBAG_DOMAINS: app.example.com,api.example.com,docs.example.com
  GYMBAG_UPSTREAMS: "app.example.com=http://web:3000;api.example.com=http://api:8080;docs.example.com=http://docs:4000"
  GYMBAG_PROTECT_DOMAINS: app.example.com,docs.example.com
  GYMBAG_VERIFY_LEVEL: "3"
```

## ボリューム・権限・セキュリティ上の注意

- **永続ボリューム `/app/certs`**: ACME アカウント鍵、発行済み証明書、ローカル CA ファイル、HMAC クリアランスシークレット（`/app/certs/secret`）、脅威/VPN/AI インテルキャッシュ（`/app/certs/intel-cache`）、リモートルールキャッシュ（`/app/certs/rules-cache`）が格納されます。必ず名前付きボリュームまたはホストディレクトリで永続化してください。
- **ホストバインドマウントの権限**: コンテナ内の Gymbag は UID/GID `10001` で動作します。名前付きボリュームではなく `./certs:/app/certs` をバインドマウントする場合は、`10001` から書き込めるよう権限を変更してください。
  ```bash
  mkdir -p ./certs
  sudo chown -R 10001:10001 ./certs
  ```
- **ACME HTTP-01 にはポート `80` が必要**: 公開 CA はポート `80` 上の `/.well-known/acme-challenge/*` でドメイン所有権を確認するため、`80:80` と `443:443` をマッピングしてください。
- **Docker 内でのローカル `*.internal`**: コンテナからホストの `/etc/hosts` は書き換えられないため `GYMBAG_AUTO_HOSTS=off` を指定し、必要に応じて非特権ポート（`-p 8080:80 -p 8443:443`）へマップした上で、ボリュームから `/app/certs/ca/neuro-sama-root-ca.pem` を取り出してホスト側で信頼登録してください。
- **Admin REST API**: コンテナ内で `GYMBAG_ADMIN=0.0.0.0:2019` を有効にする場合は、未認証の変更操作をインターネットへ晒さないようホスト側のポートをループバック（`-p 127.0.0.1:2019:2019`）に限定してください。

## イメージのローカルビルド

```bash
docker build \
  --build-arg VERSION=0.1.0 \
  -t gymbag:local .
```

`Dockerfile` は `golang:1.27-alpine` 上で静的な `CGO_ENABLED=0` バイナリをビルドし、`ca-certificates` と `libcap` を含む `alpine:3.21` へコピーします。
