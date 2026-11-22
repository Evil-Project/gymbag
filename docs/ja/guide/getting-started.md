# 2コマンド クイックスタート

Gymbag は Go 標準ライブラリのみを使って単一バイナリとしてビルドできます。HTML テンプレート、ブラウザ用 SHA-256 ソルバースクリプト、マスコット PNG 画像も `go:embed` でバイナリ内に直接埋め込まれています。

## インストール

[GitHub Releases](https://github.com/Evil-Project/gymbag/releases) のタグ付きリリースごとに、`CGO_ENABLED=0` の静的バイナリ（`linux`、`darwin`、`windows` の `amd64`/`arm64`）、Debian/Ubuntu 用 `.deb` パッケージ、Arch Linux 用 `.pkg.tar.zst` パッケージ、NixOS バンドル、マルチアーキテクチャ GHCR コンテナイメージ（`ghcr.io/evil-project/gymbag`）を公開しています。

::: code-group
```bash [Debian / Ubuntu (.deb)]
# リリース版 .deb をインストール（cap_net_bind_service と systemd ユニットも設定）
sudo apt install ./gymbag_<version>_linux_amd64.deb

# 設定を生成してサービスを有効化
sudo gymbag init -o /etc/gymbag/Gymbagfile --format gymbagfile
sudo systemctl enable --now gymbag
```

```bash [Docker (GHCR)]
docker run -d --name gymbag \
  -p 80:80 -p 443:443 \
  --cap-add NET_BIND_SERVICE \
  -v "$(pwd)/Gymbagfile:/app/Gymbagfile:ro" \
  -v gymbag-certs:/app/certs \
  ghcr.io/evil-project/gymbag:latest
```

```bash [Arch Linux (pacman)]
# リリース版 .pkg.tar.zst をインストール
sudo pacman -U gymbag-<version>-1-x86_64.pkg.tar.zst

# または packaging/arch からビルド
git clone https://github.com/Evil-Project/gymbag.git
cd gymbag/packaging/arch
makepkg -si
```

```bash [Nix / NixOS]
nix run github:Evil-Project/gymbag -- --help
nix profile install github:Evil-Project/gymbag

# 静的 Linux リリースバイナリ (CGO_ENABLED=0) も NixOS 上でそのまま動作
```

```bash [ソースからビルド (Go)]
git clone https://github.com/Evil-Project/gymbag.git
cd gymbag
go build -o gymbag ./cmd/gymbag
```
:::

コンテナ運用や Compose + Redis 構成、ボリュームの権限については [Docker & Docker Compose ガイド](/ja/guide/docker) を参照してください。

## 2 コマンドで起動

`gymbag init` はデフォルトで `gymbag.json` を生成します。`--format gymbagfile`（または `-o Gymbagfile`）を付けると `Gymbagfile` を出力します。生成された設定はファイルへ書き込む前に必ず検証されます。

### HTTP / HTTPS Web サイト

```bash
# 1. gymbag.json を生成（Caddy 構文にする場合は --format gymbagfile を追加）
./gymbag init -d app.example.com -u http://localhost:3000

# 2. Gymbag を起動（./gymbag.json、./Gymbagfile、./Caddyfile の順に自動読込）
./gymbag
```

### SSH 保護

`/etc/ssh/sshd_config` 内の `sshd` を `20000–59999` の未使用ポートへ移動し（`.gymbag.bak` にバックアップを保存）、`sshd -t` で構文を検証して（失敗時は自動復元）、`sshd` をリロードした上でホストのファイアウォール（`ufw`、`firewalld`、`nftables`、`iptables`+`ip6tables`）を更新し、外部からポート `22` の Gymbag 経由でのみ接続できるようにします。

```bash
# リモートホストで実行する際は別の SSH ターミナルを開いたままにしてください
sudo ./gymbag init -t ssh
sudo ./gymbag
```

### Minecraft サーバー保護

`--server-jar` を指定すると、Gymbag が保護プロキシとあわせて **Minecraft サーバーを自動起動**し、マシン上から**最適な互換 Java ランタイムを自動選択**します（`version.json`、バンドラー内のネストされた JAR、`patch.properties`、`.class` バイトコードヘッダを検査して Java 8 のレガシーサーバーと Java 17/21+ のサーバーを判別し、終了時にはコンソールへ `stop` を送って安全に停止します）。また `server.properties` 内の `server-port` を `20000–59999` の未使用ポートへ移動し（`server_jar` 指定時にファイルがなければ自動作成し、空欄・`0.0.0.0`・`::` の `server-ip` は `127.0.0.1` にバインド）、Server List Ping はそのまま通しつつ、ログイン時には Minecraft の **「Connection Lost」**ダイアログに HTTPS 検証リンクを表示します。

```bash
# 自動選択した Java で server.jar を自動起動 + バックエンドポートをランダム化:
./gymbag init -t minecraft --server-jar /srv/mc/paper.jar --mc-jvm-args "-Xms1G -Xmx4G"
./gymbag

# すでに稼働中のサーバーを server.properties 経由で保護する場合:
./gymbag init -t minecraft --server-properties /path/to/server.properties
./gymbag
```

### 全部入り（`http,ssh,minecraft`）と対話ウィザード

1 つのプロセスで Web サイト、SSH、Minecraft を同時に保護できます。

```bash
# 非対話での統合設定:
sudo ./gymbag init -t http,ssh,minecraft -d app.example.com -u :3000 --server-jar /srv/mc/paper.jar
sudo ./gymbag

# 対話ウィザードを起動する場合:
./gymbag init
./gymbag
```

> **検証ポータルと IP SSL 証明書:** SSH と Minecraft はデフォルトでポート `8443`（`--verify-port`）の HTTPS 検証ポータルを共有します。独自ドメインを使う場合は `--verify-url https://verify.example.com:8443` を指定してください。空欄にすると自動検出したサーバー IP 上で RFC 8738 ACME IP 証明書（HTTP-01 に到達できない場合はローカル CA または自己署名 IP 証明書へフォールバック）を使って配信します。詳しくは [SSH & Minecraft 保護ガイド](/ja/guide/ssh-minecraft) を参照してください。

### ローカル開発（`*.internal`）

```bash
# 1. ローカル PKI を使った app.internal -> http://localhost:3000 を生成
./gymbag init --preset local

# 2. 非特権ポートで起動（80/443 と /etc/hosts 管理を使う場合は sudo で起動）
./gymbag --http-port 8080 --https-port 8443
```

### システムサービスとして登録

```bash
./gymbag init -d app.example.com -u :3000 --service auto
./gymbag service start
```

`systemd`、`launchd`、`openrc`、`supervisor` の詳細は [システムサービス設定](/ja/guide/system-service) を参照してください。

## 既存の nginx / Apache / Caddyfile 設定

Gymbag は `nginx.conf`、Apache の `httpd.conf` / `apache2.conf`、Caddyfile v2 をそのまま読み込めます。

```bash
./gymbag run -c /etc/nginx/nginx.conf
./gymbag config convert -c Caddyfile -o gymbag.json --format json
```

パスルート、静的ファイル配信、リダイレクト、変換仕様は [パスルートと設定インポート](/ja/guide/routes-and-imports) を参照してください。

## 設定ファイルなしの単発リバースプロキシ

```bash
./gymbag reverse-proxy --from app.internal --to :3000 --verify 3 --verify-timeout 12h
```

## `gymbag init` フラグ一覧

| フラグ | 説明 |
| :--- | :--- |
| `-o`, `--output <path>` | 出力先パス（デフォルト `gymbag.json`、`--format gymbagfile` 時は `Gymbagfile`） |
| `--format <fmt>` | `json`（デフォルト）または `gymbagfile` |
| `-t`, `--template <list>` | サービステンプレート: `http`、`ssh`、`minecraft`、または `all`（カンマ区切りまたは複数回指定） |
| `--preset <name>` | `standard`、`local`、`public`、`strict`、`ai-only`、`suspicious`、`proxy-only`、`http`、`ssh`、`minecraft`、または `all` |
| `-d`, `--domain <domains>` | 下流ドメイン（カンマ区切り） |
| `-u`, `--upstream <url>` | アップストリーム URL（`http://localhost:3000` または `:3000`） |
| `--site <domain=url>` | 複数サイトのエントリを追加（繰り返し指定可） |
| `--ssh` / `--ssh-port <port>` | SSH 保護を有効化（デフォルトポート `22`） |
| `--ssh-upstream <addr>` / `--sshd-config <p>` | 明示的な `sshd` 宛先、または `sshd_config` のパス（デフォルト `/etc/ssh/sshd_config`） |
| `--minecraft`, `--mc` / `--mc-port <port>` | Minecraft 保護を有効化（デフォルトポート `25565`） |
| `--mc-upstream <addr>` / `--server-properties <p>` | 明示的な Minecraft バックエンド、または `server.properties` のパス |
| `--mc-jar`, `--server-jar <path>` | Minecraft サーバー `.jar` のパス（自動選択した Java でサーバーを自動起動） |
| `--mc-java`, `--java <bin>` | Minecraft サーバー用の明示的な Java バイナリパス（デフォルトは自動選択） |
| `--mc-jvm-args <args>` / `--mc-work-dir <dir>` | 管理対象 Minecraft サーバー用のスペース区切り JVM フラグと作業ディレクトリ |
| `--verify-url <url>` | 共有 HTTPS 検証ポータルの URL（空欄 = サーバー IP + IP SSL 証明書） |
| `--verify-port <port>` | 共有 HTTPS 検証ポータルのポート（デフォルト `8443`） |
| `--no-auto-setup` / `--no-firewall` | `sshd_config`/ファイアウォールや `server.properties` の自動書き換えをスキップ |
| `-l`, `--verify <0-3\|off>` | 検証レベル（`0`、`1`、`2`、`3`、または `off`） |
| `--difficulty <1-5>` | SHA-256 PoW の難易度（デフォルト `4`） |
| `--verify-timeout <dur>` | クリアランス有効期限（`30m`、`12h`、`7d`、または秒数；別名 `--clearance-ttl`） |
| `--remote-rules <url>` | リモートファイアウォールルールの URL（繰り返し・カンマ区切り可） |
| `--remote-rules-refresh <dur>` | リモートルールの更新間隔（デフォルト `5m`） |
| `--rules-file <path>` / `--rules-dir <path>` | ローカルルールファイルまたはディレクトリ（繰り返し指定可） |
| `--rules-dynamic` / `--rules-watch-interval <dur>` | ローカルルールソースの変更を監視（デフォルト `2s`） |
| `--no-protect` / `--no-default-rules` | PoW 検証または組み込みベースラインルールを無効化 |
| `--ssl-provider <name>` | `letsencrypt`、`zerossl`、`google`、`ssl.com`、`litessl`、`buypass`、`local`、または URL |
| `--email <addr>` | ACME 連絡先メール（ZeroSSL 自動 EAB 兼用） |
| `--eab-kid` / `--eab-hmac-key` | ACME External Account Binding 資格情報 |
| `--http-port` / `--https-port` | カスタム HTTP/HTTPS ポート（デフォルト `80` / `443`） |
| `--bind <ip>` | バインド IP アドレス（省略時は自動選択） |
| `--admin <addr>` | ローカル REST Admin API を有効化（例: `127.0.0.1:2019`） |
| `--cert-dir <dir>` | 証明書・ローカル CA・キャッシュ保存先（デフォルト `certs`） |
| `--redis <url>` | チャレンジ・レート制限・SSH/MC ゲート状態を共有する Redis URL |
| `--ip-security-action <act>` | 脅威インテル一致時のアクション（`verify` または `block`） |
| `--no-auto-hosts` | ローカルドメインの `/etc/hosts` 管理を無効化 |
| `-y`, `--yes` / `-i`, `--interactive` | ウィザードの確認をスキップ / 対話ウィザードを強制起動 |
| `--dry-run` / `-f`, `--force` | ファイルに書かず標準出力へ表示 / 既存ファイルを上書き |
| `--service <type>` | OS サービスを登録（`auto`、`systemd`、`launchd`、`openrc`、`supervisor`） |

## ルールを書かずに動く標準防御

引数なしの `verify`（検証レベル `3`: `suspicious & ai`）または `block` を書くだけで、次の 3 層が有効になります。

1. **組み込みベースラインルール（`default_rules on`）**: `/.env`、`/.git/*`、`/.aws/*`、`/wp-admin/*`、`sqlmap`/`nikto`/`nuclei`、SQLi/XSS クエリプローブに `403` を返し、CLI クライアントやヘッドレスブラウザを PoW 画面へ送ります。
2. **脅威フィード・ライブ IP レピュテーション・VPN/Tor 検出**: 起動時と 6 時間ごとに IPsum (level 3+)、Spamhaus DROP v4/v6、Feodo Tracker、X4BNet VPN リスト、Tor 出口ノードを更新し、Shodan InternetDB、StopForumSpam、`ipquery.io`、`ipapi.is` へフェイルオープンタイムアウト付きで照会します。VPN/プロキシ/Tor の IP は HTTP・SSH・Minecraft のレベル `1` と `3` で不審（suspicious）として扱われます。
3. **公式 AI クローラー IP リスト**: OpenAI、Anthropic、Perplexity、Google、Apple の公開 CIDR ブロックと照合するため、Chrome の `User-Agent` を偽装するボットも捕捉します。
