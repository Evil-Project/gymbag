# 2コマンド クイックスタート

Gymbag は外部 Go 依存ゼロの単一バイナリとしてコンパイルされます — すべての HTML テンプレート、SHA-256 ブラウザソルバースクリプト、マスコット PNG 画像が `go:embed` によりバイナリへ直接埋め込まれています。

## インストール

### ビルド済み静的バイナリ・Docker・Arch Linux・NixOS

[GitHub Releases](https://github.com/Evil-Project/gymbag/releases) の各リリースでは、`CGO_ENABLED=0` 静的バイナリ（`linux`、`darwin`、`windows` の `amd64`/`arm64`）、Arch Linux 用 `.pkg.tar.zst` パッケージ（`x86_64`・`aarch64`）、マルチアーキテクチャ GHCR コンテナイメージ（`ghcr.io/evil-project/gymbag`）を公開しています：

::: code-group
```bash [Docker (GHCR)]
docker run -d --name gymbag \
  -p 80:80 -p 443:443 \
  --cap-add NET_BIND_SERVICE \
  -v "$(pwd)/Gymbagfile:/app/Gymbagfile:ro" \
  -v gymbag-certs:/app/certs \
  ghcr.io/evil-project/gymbag:latest
```

```bash [Arch Linux (pacman)]
# リリース版の .pkg.tar.zst をインストール
sudo pacman -U gymbag-<version>-1-x86_64.pkg.tar.zst

# または PKGBUILD からソースビルド
git clone https://github.com/Evil-Project/gymbag.git
cd gymbag/packaging/arch
makepkg -si
```

```bash [Nix / NixOS]
# Nix Flakes 経由で直接実行またはインストール
nix run github:Evil-Project/gymbag -- --help
nix profile install github:Evil-Project/gymbag

# 静的 Linux リリースバイナリ (CGO_ENABLED=0) は NixOS 上でそのまま動作します
```

```bash [Go からソースビルド]
git clone https://github.com/Evil-Project/gymbag.git
cd gymbag
go build -o gymbag ./cmd/gymbag
```
:::

コンテナ運用や Compose + Redis、ボリューム権限の詳細は [Docker & Docker Compose ガイド](/ja/guide/docker) を参照してください。

## 2コマンドで起動

### 方法 A：フラグ指定（非対話モード）

```bash
# 1. gymbag.json を生成（Caddy 風にしたい場合は --format gymbagfile を指定）
./gymbag init -d app.example.com -u http://localhost:3000

# 2. Gymbag を起動（./gymbag.json、./Gymbagfile、./Caddyfile の順に自動読み込み）
./gymbag
```

### 方法 B：対話型セットアップウィザード

フラグなしで `gymbag init` を実行すると、ドメイン、アップストリーム URL、検証レベル、SSL プロバイダー、ポート、管理 API、システムサービス登録を対話的に設定できます：

```bash
# 1. 対話型設定ジェネレーター
./gymbag init

# 2. Gymbag を起動
./gymbag
```

### 方法 C：ローカル開発プリセット（`*.internal`）

```bash
# 1. ローカル PKI 付きの app.internal -> http://localhost:3000 設定を生成
./gymbag init --preset local

# 2. Gymbag を起動（非特権ポート指定、または 80/443 と /etc/hosts 用に sudo）
./gymbag --http-port 8080 --https-port 8443
```

### 方法 D：システムサービスとして登録

```bash
# 1. gymbag.json を生成し systemd / launchd / openrc / supervisor に登録
./gymbag init -d app.example.com -u :3000 --service auto

# 2. 登録したサービスを起動
./gymbag service start
```

詳細は [システムサービス設定](/ja/guide/system-service) をご覧ください。

---

## 既存の nginx / Apache / Caddyfile 設定

Gymbag は `nginx.conf`、Apache の `httpd.conf` / `apache2.conf`、Caddyfile v2 ファイルをネイティブに読み込めるため、既存の設定からそのまま移行できます：

```bash
./gymbag run -c /etc/nginx/nginx.conf
./gymbag config convert -c Caddyfile -o gymbag.json --format json
```

パスルート、静的ファイル、リダイレクト、変換の詳細は [パスルートと設定インポート](/ja/guide/routes-and-imports) を参照してください。

---

## アドホック リバースプロキシ（設定ファイル不要）

ディスクに設定ファイルを作成せず即座にテストできます：

```bash
./gymbag reverse-proxy --from app.internal --to :3000 --verify 3 --verify-timeout 12h
```

---

## 設定ジェネレーター オプション (`gymbag init`)

`gymbag init` はディスクへ書き込む前に生成設定を自動検証します。

| フラグ | 説明 |
| :--- | :--- |
| `-o`, `--output <path>` | 出力先パス（既定は `gymbag.json`、`--format gymbagfile` 時は `Gymbagfile`） |
| `--format <fmt>` | `json`（既定）または `gymbagfile`（Caddy 風） |
| `--preset <name>` | `standard`、`local`、`public`、`strict`、`ai-only`、`suspicious`、`proxy-only` |
| `-d`, `--domain <domains>` | 公開ドメイン（カンマ区切り） |
| `-u`, `--upstream <url>` | アップストリーム URL（例：`http://localhost:3000` や `:3000`） |
| `--site <domain=url>` | 複数サイトブロックを追加（複数指定可、例：`--site app.example.com=:3000 --site api.example.com=:8080`） |
| `-l`, `--verify <0-3\|off>` | 検証レベル（`0`、`1`、`2`、`3`、または `off`） |
| `--difficulty <1-5>` | SHA-256 PoW の先頭ゼロ桁数難易度（既定 `4`） |
| `--verify-timeout <dur>` | 認証済み有効期限（例：`30m`、`12h`、`7d`、または秒数；別名 `--clearance-ttl`） |
| `--remote-rules <url>` | リモートファイアウォールルール URL（複数またはカンマ区切り） |
| `--remote-rules-refresh <dur>` | リモートルールの更新間隔（既定 `5m`） |
| `--rules-file <path>` | 専用ファイアウォールルールファイルのパス（複数指定可） |
| `--rules-dir <path>` | ファイアウォールルールファイルのディレクトリ（複数指定可） |
| `--rules-dynamic` | `--rules-file` / `--rules-dir` の変更時に自動リロード |
| `--rules-watch-interval <dur>` | 動的ローカルルールファイル/フォルダのポーリング間隔（既定 `2s`） |
| `--no-protect` | WAF 検証を無効化（`protect off`） |
| `--no-default-rules` | 組み込みベースラインルールを無効化（`default_rules off`） |
| `--ssl-provider <name>` | `letsencrypt`、`zerossl`、`google`、`ssl.com`、`litessl`、`buypass`、`local`、または URL |
| `--email <addr>` | ACME 連絡先メールアドレス（ZeroSSL 自動 EAB にも対応） |
| `--eab-kid` / `--eab-hmac-key` | ACME External Account Binding 認証情報 |
| `--http-port` / `--https-port` | HTTP / HTTPS ポート（既定 `80` / `443`） |
| `--bind <ip>` | バインドアドレス（省略時は自動選択） |
| `--admin <addr>` | ローカル REST 管理 API の待ち受けアドレス（例：`127.0.0.1:2019`） |
| `--cert-dir <dir>` | 証明書・ローカル CA・各種キャッシュの保存先（既定 `certs`） |
| `--redis <url>` | チャレンジおよびレート制限共有用の Redis URL |
| `--ip-security-action <act>` | 脅威インテル一致時のアクション（`verify` または `block`） |
| `--no-auto-hosts` | ローカルドメインの `/etc/hosts` 自動管理を無効化 |
| `-y`, `--yes` | 対話プロンプトをスキップし既定値を採用 |
| `-i`, `--interactive` | フラグ指定時でも対話ウィザードを強制起動 |
| `--dry-run` | ファイルに書かず標準出力へ生成結果を出力 |
| `-f`, `--force` | 既存の設定ファイルを上書き |
| `--service <type>` | システムサービスを同時登録（`auto`、`systemd`、`launchd`、`openrc`、`supervisor`） |

---

## 既存設定のインポートと設定管理

Gymbag は既存の **nginx**（`nginx.conf`）、**Apache**（`httpd.conf` / `apache2.conf`）、**Caddyfile v2** 設定をネイティブに読み込めます（スニペット、`import`、`@matchers`、`handle`、`handle_path`、`route`、`root`、`file_server`、`try_files`、`respond`、`redir`、`header` に対応）。`gymbag run -c /etc/nginx/nginx.conf` でそのまま起動するか、ネイティブ形式へ変換します：

```bash
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```

サイト、パスルート、グローバル設定は `gymbag config list|add|update|remove|set|route` で管理できます。詳細は [パスルートと設定インポート](/ja/guide/routes-and-imports) と [CLI & 環境変数](/ja/reference/cli-env) を参照してください。

---

## 箱出し（Open-Box）保護に含まれる3層防御

`verify`（検証レベル `3`：`suspicious & ai`）または `block` を指定するだけで、正規表現を書かずに3層の防御が有効になります：

1. **ベースライン ファイアウォールルール (`default_rules on`)** — `/.env`、`/.git/*`、`/.aws/*`、`/wp-admin/*`、`sqlmap`/`nikto`/`nuclei`、SQLi/XSS クエリプローブを `403` で遮断し、CLI やヘッドレス自動化クライアントに PoW チャレンジを課します。
2. **自動更新 IP 脅威インテリジェンス** — 起動時と6時間ごとに IPsum（レベル 3+）、Spamhaus DROP v4/v6、abuse.ch Feodo Tracker を取得し、API キー不要の Shodan InternetDB と StopForumSpam によるリアルタイム評価を併用します。
3. **公式 AI クローラー IP リスト** — OpenAI、Anthropic、Perplexity、Google、Apple の公開 CIDR 範囲を追跡し、デスクトップブラウザの User-Agent を偽装するクローラーも検出します。
