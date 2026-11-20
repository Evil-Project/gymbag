# Gymbag

<p align="center">
  <img src="docs/public/mascot-happy.png" alt="Gymbag mascot" width="140" />
</p>

<p align="center">
  <strong>純 Go 製シングルバイナリ HTTPS リバースプロキシ &amp; Proof-of-Work WAF</strong>
</p>

<p align="center">
  <a href="https://github.com/Evil-Project/gymbag/releases"><img src="https://img.shields.io/github/v/release/Evil-Project/gymbag?include_prereleases&label=Release" alt="Release" /></a>
  <a href="https://github.com/Evil-Project/gymbag/pkgs/container/gymbag"><img src="https://img.shields.io/badge/Docker-GHCR-blue?logo=docker" alt="Docker GHCR" /></a>
  <a href="go.mod"><img src="https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go" alt="Go 1.27+" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache--2.0-green" alt="Apache-2.0 License" /></a>
  <a href="https://evil-project.github.io/gymbag/ja/"><img src="https://img.shields.io/badge/Docs-VitePress-b81d33" alt="Docs" /></a>
</p>

<p align="center">
  <strong>README:</strong>
  <a href="README.md">English</a> ·
  <a href="README.zh.md">简体中文</a> ·
  <a href="README.ja.md">日本語</a> ·
  <a href="README.ko.md">한국어</a> ·
  <a href="README.es.md">Español</a>
  <br />
  <sub>オンラインドキュメント: <a href="https://evil-project.github.io/gymbag/">EN</a> · <a href="https://evil-project.github.io/gymbag/zh/">ZH</a> · <a href="https://evil-project.github.io/gymbag/ja/">JA</a> · <a href="https://evil-project.github.io/gymbag/ko/">KO</a> · <a href="https://evil-project.github.io/gymbag/es/">ES</a></sub>
</p>

Gymbag は、Anubis スタイルの Proof-of-Work（作業証明）Web アプリケーションファイアウォールを内蔵した、純 Go 製のシングルバイナリ HTTPS リバースプロキシです。標準ライブラリのみを使用して実装されており、外部 Go 依存はゼロ。すべての HTML テンプレート、ブラウザ用 SHA-256 ソルバースクリプト、マスコット PNG 画像まで `go:embed` によって実行ファイルに直接組み込まれています。

Web アプリや Gitea/Forgejo、ドキュメントサイト、社内ツールなどをセルフホストしているなら、Anubis が生まれたのと同じ課題に直面したことがあるはずです。`robots.txt` を無視する攻撃的な LLM クローラーや、`/.env` や `/wp-admin` を24時間スキャンし続ける脆弱性スキャナーです。Nginx/Caddy に独立した PoW サイドカーコンテナ、fail2ban、手作業の CIDR リストを組み合わせる代わりに、Gymbag は TLS 終端、マルチ CA ACME、HTTP/WebSocket プロキシ、ローカル＆リモートファイアウォールルール、IP 脅威インテリジェンス、サイトごとの PoW ポリシーをシンプルな `Gymbagfile` で設定できる 1 つのバイナリに統合します。

---

## クイックスタート（すぐに使える多層防御）

[GitHub Releases](https://github.com/Evil-Project/gymbag/releases) からビルド済み静的バイナリ（Linux/macOS/Windows、および Arch Linux 用 `.pkg.tar.zst`）をダウンロードするか、Nix Flakes またはソースからビルドしてください：

```bash
go build -o gymbag ./cmd/gymbag
```

**Arch Linux:**
```bash
# ビルド済みパッケージ（.pkg.tar.zst）をインストール
sudo pacman -U gymbag-0.1.0-1-x86_64.pkg.tar.zst
# または makepkg でローカルビルド
cd packaging/arch && makepkg -si
```

**Nix / NixOS:**
```bash
# flake から実行またはインストール
nix run github:Evil-Project/gymbag -- --help
nix profile install github:Evil-Project/gymbag
```
静的リンクされた `CGO_ENABLED=0` の Linux リリースバイナリは NixOS 上でも `patchelf` や `nix-ld` なしでそのまま動作し、`nixosModules.default` による宣言的な `services.gymbag` 設定も提供されています。

**わずか 2 コマンド**で `Gymbagfile` を生成してサーバーを起動できます：

```bash
# 1. 設定を生成（対話ウィザード、またはフラグ指定でワンライナー作成）
./gymbag init -d app.example.com -u http://localhost:3000

# 2. サーバーを起動
./gymbag
```

システムサービス（`systemd`、`launchd`、`openrc`、`supervisor`）として登録して起動することも可能です：

```bash
./gymbag init -d app.example.com -u :3000 --service auto
./gymbag service start
```

設定ファイルを使わずにアドホックなプロキシを起動することもできます：

```bash
./gymbag reverse-proxy --from app.internal --to :3000 --verify 3
```

### 「すぐに使える（Open-Box）」保護の仕組み

Gymbag をインターネットに公開する前に、正規表現を 1 行書いたりブロックリストを手動インポートしたりする必要はありません。`verify`（検証レベル `3`：`suspicious & ai`）または `block`（脅威インテリジェンス一致 IP をチャレンジせず即 `403 Forbidden`）を指定するだけで、次の 3 層が即座に有効になります：

1. **組み込みベースラインファイアウォールルール（`default_rules on`）**
   - **機密ファイル保護（`403`）**: `/.env`、`/.env.*`、`/.git/*`、`/.svn/*`、`/.hg/*`、`/.DS_Store`、`/.aws/*`、`/.ssh/*`、`/docker-compose.yml`、`/docker-compose.yaml`、`*.bak`、`*.sql`、`*.sqlite` へのアクセスを遮断します。
   - **CMS・フレームワーク攻撃探査（`403`）**: `/wp-admin/*`、`/wp-login.php`、`/xmlrpc.php`、`/phpmyadmin*`、`/vendor/phpunit*`、`/cgi-bin/*`、`/actuator/env*`、`/actuator/heapdump*` をドロップします。
   - **脆弱性スキャナー遮断（`403`）**: `sqlmap`、`nikto`、`nuclei`、`masscan`、`zgrab`、`gobuster`、`dirbuster`、`ffuf`、`wfuzz`、`feroxbuster`、`acunetix`、`nessus`、`openvas` を拒否します。
   - **インジェクション＆トラバーサル検知（`403`）**: クエリ文字列内の `UNION SELECT`、`<script`、`javascript:`、`/etc/passwd`、`information_schema`、`../`、`cmd.exe`、`/bin/sh` を検知します（NULL バイトや `%2e%2e` パストラバーサルに対する厳格なリクエスト形式チェックも併用）。
   - **自動化クライアント向けチャレンジ（`PoW`）**: カスタムルールを定義していない場合、CLI HTTP クライアント（`curl`、`wget`、`python-requests`、`python-httpx`、`go-http-client`、`scrapy`）、ヘッドレスブラウザ（`headlesschrome`、`playwright`、`puppeteer`、`selenium`）、および空の `User-Agent` は Proof-of-Work 画面へ誘導されます。
2. **自動更新 IP 脅威インテリジェンス**
   - 起動時およびバックグラウンドで 6 時間ごとに [IPsum (level 3+)](https://raw.githubusercontent.com/stamparm/ipsum/master/levels/3.txt)、[Spamhaus DROP IPv4](https://www.spamhaus.org/drop/drop_v4.json) & [IPv6](https://www.spamhaus.org/drop/dropv6.txt)、[abuse.ch Feodo Tracker](https://feodotracker.abuse.ch/downloads/ipblocklist.txt) を取得し、`ETag` / `If-Modified-Since` を使って `<cert_dir>/intel-cache` にキャッシュします。
   - **Shodan InternetDB**（`internetdb.shodan.io`）および **StopForumSpam**（`api.stopforumspam.org`）に対するキー不要のリアルタイム IP レピュテーション照会を、インメモリ TTL キャッシュとフェイルオープンタイムアウト付きで実行します。
3. **公式 AI クローラー IP レンジ追跡**
   - **OpenAI**（`gptbot`、`searchbot`、`chatgpt-user`）、**Anthropic**（`claude.com/crawling/bots.json`）、**Perplexity**（`perplexitybot`、`perplexity-user`）、**Google**（`user-triggered-agents`、`user-triggered-fetchers-google`）、**Apple**（`applebot`）の公開 CIDR リストを追跡します。`User-Agent` を通常のデスクトップ Chrome に偽装しても、送信元 IP から確実に判別します。

> **ポート 80 と 443 に関する運用メモ：** デフォルトでは `:80`（HTTP→HTTPS リダイレクト + ACME HTTP-01 応答）と `:443`（HTTPS ゲートウェイ）をリッスンします。Linux ではバイナリに `CAP_NET_BIND_SERVICE` を付与することで root 権限なしで起動できます：
> ```bash
> sudo setcap 'cap_net_bind_service=+ep' ./gymbag
> ```
> 非特権環境でのローカルテストでは `--http-port 8080 --https-port 8443`（または `Gymbagfile` で `http_port` / `https_port`）を指定してください。

---

## ローカル開発（`localhost` & `*.internal`）

カスタムホスト名でのローカル HTTPS テストでは、通常 `mkcert` や手動での `/etc/hosts` 編集、自己署名証明書の警告に悩まされます。Gymbag は `localhost`、`*.localhost`、`*.internal`（例：`app.internal` や `api.internal`）、`*.local`、`*.test` を使うサイトに対して、名前解決とローカル X.509 証明書発行を自動で処理します：

```caddyfile
app.internal {
    reverse_proxy http://localhost:3000
    verify 3
}

api.internal {
    reverse_proxy http://localhost:8080
    protect off
}
```

### `/etc/hosts` の自動登録とクリーンアップ

`auto_hosts` が有効（デフォルト）な場合、Gymbag は起動時に `/etc/hosts`（Windows では `C:\Windows\System32\drivers\etc\hosts`）を確認します。未解決のローカルドメイン（`app.internal` など）が見つかると、インスタンス ID タグ付きの IPv4 / IPv6 ループバックエントリを追記します：

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

既存の `localhost` 行には触れません。Gymbag を終了（`Ctrl+C` / `SIGTERM`）すると、実行中の手動編集を保持したまま `/etc/hosts` を再読み込みし、自身の `# gymbag-managed:<instanceID>` 行**のみ**を削除します。

- **macOS & Linux の権限について：** `/etc/hosts` の更新には root 権限が必要です。書き込み権限のない一般ユーザーで `./gymbag` を起動した場合、明確な警告ログを出力して**クラッシュせずそのまま稼働を続けます**。`/etc/hosts` の自動管理（および 80/443 ポート利用）を行いたい場合は `sudo ./gymbag` で実行するか、`dnsmasq` 等を使う場合は `auto_hosts off`（または `GYMBAG_AUTO_HOSTS=off`）を設定してください。

### 2 階層ローカル PKI と Root CA の信頼設定

ブラウザが信頼キャッシュを拒否する使い捨ての自己署名リーフ証明書ではなく、Gymbag は `<cert_dir>/ca/`（デフォルト `certs/ca/`）内に 2 階層の ECDSA P-256 認証局を構築します：

1. **Root CA**: **`"Neuro-sama Headquarters Root CA"`**（`certs/ca/neuro-sama-root-ca.pem`、有効期限 10 年、`MaxPathLen: 1`）
2. **Intermediate CA**: **`"Gymbag & Swarm SSL Local CA"`**（`certs/ca/gymbag-swarm-local-ca.pem`、有効期限 5 年、`"Neuro-sama Headquarters Root CA"` による署名、`MaxPathLen: 0`）
3. **Leaf 証明書**: `certs/local-fullchain.pem` + `certs/local-privkey.pem`（有効期限 90 日、残り 30 日以内で自動更新、DNS SAN および `127.0.0.1` / `::1` の IP SAN 付き）

OS の信頼ストアで **`"Neuro-sama Headquarters Root CA"`** を一度信頼するだけで、Gymbag が配信するすべての `https://localhost` および `https://*.internal` がブラウザや `curl` で正常に検証されます：

**macOS（システムキーチェーン）：**
```bash
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain certs/ca/neuro-sama-root-ca.pem
```

**Debian / Ubuntu:**
```bash
sudo cp certs/ca/neuro-sama-root-ca.pem /usr/local/share/ca-certificates/neuro-sama-root-ca.crt && sudo update-ca-certificates
```

**Fedora / RHEL / Arch Linux:**
```bash
sudo trust anchor --store certs/ca/neuro-sama-root-ca.pem
```

> **Firefox に関する注意：** macOS および Windows では、`about:config` で `security.enterprise_roots.enabled` を `true` にすると OS のルート証明書を自動信頼します。Linux で Firefox が p11-kit ではなく独自の NSS DB を使っている場合は、`security.enterprise_roots.enabled` を有効にするか、*設定 -> プライバシーとセキュリティ -> 証明書 -> 証明書を表示 -> 認証局* から `certs/ca/neuro-sama-root-ca.pem` を一度インポートしてください。

---

## 複数サイトの保護と検証レベル（`0`–`3`）

単一の Gymbag プロセスで数十ものドメインを同時にホストし、複数アップストリームへのラウンドロビン負荷分散や全二重 RFC 6455 WebSocket アップグレードを行えます。各サイトで `verify <0-3>`（または WAF 無効化の `protect off`）を指定します：

| レベル | 名称 | チャレンジ対象 | 最適な用途 |
| :---: | :--- | :--- | :--- |
| `0` | `always` | 有効な HMAC クリアランス Cookie を持たないすべての訪問者。 | 社内専用ツール、ステージング環境、L7 フラッド攻撃を受けているサイト。 |
| `1` | `only suspicious` | 脅威リスト/低評価 IP、スキャナー、CLI HTTP ライブラリ、ヘッドレスブラウザ、不審/偽装ヘッダー、脆弱性プローブ。 | 一般ブラウザにチャレンジ画面を見せたくない公開 Web アプリや API。 |
| `2` | `only ai` | **公式 AI クローラー IP 範囲** *または* AI ボットの `User-Agent`/エージェントヘッダーに一致する AI クローラー・LLM エージェント。 | 人間や `curl` ユーザーにはストレスなく公開しつつ、LLM スクレイパーに負荷を課したいドキュメントやブログ、Git フォージ。 |
| `3` | `suspicious & ai` | `1` と `2` の組み合わせ：不審クライアント/IP と AI クローラーの両方にチャレンジし、通常の人間のブラウザはそのまま通過。 | 公開本番サイトでの推奨デフォルト（引数なしの `verify` でも有効）。 |
| — | `protect off` | Proof-of-Work チャレンジを無効化し、純粋な HTTPS ＆ WebSocket リバースプロキシとして動作（サイトにカスタムルールが定義されている場合は評価されます）。 | 静的アセット配信、Webhook、公開パッケージミラー。 |

```caddyfile
app.example.com, www.example.com {
    reverse_proxy http://localhost:3000 http://localhost:3001
    verify 3
    difficulty 4
}

docs.example.com {
    reverse_proxy http://localhost:4000
    verify 2
}

api.example.com {
    reverse_proxy http://localhost:8080
    verify 1
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
}

static.example.com {
    reverse_proxy http://localhost:9000
    protect off
}
```

ブラウザが SHA-256 Proof-of-Work（`difficulty` `1`–`5`、デフォルト `4`）を解くと、Gymbag はホスト名とクライアント IP の両方にバインドされた `HttpOnly`・`SameSite=Lax` の HMAC 署名済みクリアランス Cookie（`waf_clearance`、デフォルト有効期間 `24h` / `86400s`）を発行します。

### 認証済みステータスのカスタム有効期限（`verify_timeout` / `clearance_ttl`）

訪問者の認証済み状態の有効期限を、グローバルまたはサイト単位で整数秒**または**人間が読みやすい期間表記（`30m`、`12h`、`1h30m`、`7d`、`2w`）でカスタマイズできます：

```caddyfile
{
    verify_timeout 12h     # グローバルデフォルト（エイリアス：verified_timeout, clearance_ttl）
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
    verify_timeout 30m     # サイト単位の上書き
}
```

`gymbag init` / `gymbag run` / `gymbag reverse-proxy` に `--verify-timeout 6h`（または `--clearance-ttl 6h`）を渡したり、`GYMBAG_VERIFY_TIMEOUT` / `GYMBAG_CLEARANCE_TTL` を設定したり、`PATCH /api/v1/sites/{domain}` で `clearance_ttl` をライブ更新することも可能です。認証済みユーザーが `GET /.waf/v1/session` にアクセスすると、`clearance_ttl`、`expires_at`、残り秒数 `expires_in` が返されます。

---

## SSL プロバイダの切り替え、カスタム鍵、自動更新

公開ドメインでは、デフォルトの `tls auto` が RFC 8555 ACME HTTP-01 を通じて Let's Encrypt から自動取得を行います。`ssl_provider` ディレクティブ（エイリアス：`acme_provider`、`ca`、`cert_issuer`）または `tls <provider>` で CA を切り替えられます：

| プロバイダキーワード | 認証局（CA） | EAB 要件 |
| :--- | :--- | :--- |
| `letsencrypt` (`le`) | Let's Encrypt (本番) | 不要 |
| `letsencrypt_staging` | Let's Encrypt (ステージング) | 不要 |
| `zerossl` | ZeroSSL | `email` 設定時は**自動取得**、または明示的な `eab <kid> <hmac_key>` |
| `google` (`gts`) | Google Trust Services | `eab <kid> <hmac_key>` が必要 |
| `ssl.com` (`sslcom`, `ssl.com-rsa`) | SSL.com (ECC または RSA) | `eab <kid> <hmac_key>` が必要 |
| `litessl` (`trustasia`) | LiteSSL by TrustAsia | 標準 DV は不要（または `acme_directory` で URL 上書き） |
| `buypass` | Buypass Go SSL | 不要 |
| `local` | ローカル 2 階層 PKI | ローカル CA（`Neuro-sama Headquarters Root CA` -> `Gymbag & Swarm SSL Local CA`） |
| `https://...` | 任意の RFC 8555 ACME v2 CA | 任意の `eab <kid> <hmac_key>`（Smallstep `step-ca`、Pebble 等） |

### ACME CA の切り替えと External Account Binding (EAB)

**ZeroSSL でのワンクリック自動メール EAB：**
`zerossl` を選択して `email` を設定すると、明示的な EAB キーを指定していない場合に ZeroSSL の EAB エンドポイント（`https://api.zerossl.com/acme/eab-credentials-email`）から自動で認証情報を取得します：

```caddyfile
{
    email ops@example.com
    ssl_provider zerossl
}

example.com {
    reverse_proxy http://localhost:3000
    verify 3
}
```

**Google Trust Services、SSL.com、または明示的な EAB 指定：**
Key ID と HMAC キーを指定します（パディングなし URL-safe Base64 と標準パディング Base64 の両方に対応し、Admin API の出力では `eab_hmac_key` が自動的にマスクされます）：

```caddyfile
{
    email ops@example.com
    ssl_provider google
    eab YOUR_EAB_KEY_ID YOUR_EAB_HMAC_KEY
}
```

**LiteSSL by TrustAsia またはカスタム ACME ディレクトリ：**

```caddyfile
{
    email ops@example.com
    ssl_provider litessl
    # または任意の ACME v2 ディレクトリ URL を直接指定：
    # acme_directory https://ca.internal.corp/acme/acme/directory
}
```

Gymbag はディスク上の ACME 証明書をプロバイダ名ごとに分離保存します（例：`certs/acme-zerossl-fullchain.pem`、`certs/acme-google-fullchain.pem`、Let's Encrypt は `certs/acme-fullchain.pem`）。`ssl_provider` を変更して再起動またはリロードすると、古い証明書を使い回さず新しい CA へ即座に証明書を要求します。

### 自社鍵や既存証明書の持ち込み

- **カスタム ACME アカウント鍵 (`account_key`)**: デフォルトでは `certs/acme-account.pem` に ECDSA P-256 アカウント鍵を生成・再利用します。すでに登録済みの ECDSA（P-256/P-384）または RSA アカウント鍵がある場合は `account_key /etc/gymbag/account.pem`（または `GYMBAG_ACCOUNT_KEY_FILE`）を指定してください。
- **カスタム CSR 秘密鍵 (`ssl_key`)**: `ssl_key /etc/gymbag/domain.key` を指定すると、新規生成ではなくご自身の ECDSA / RSA 秘密鍵で ACME CSR に署名します。
- **カスタム TLS 証明書＆秘密鍵 (`tls <cert> <key>`)**: 商用証明書、Cloudflare Origin 証明書、外部管理の PEM ファイルをグローバルまたはサイト単位（SNI 振り分け）で指定できます。暗号化秘密鍵には `key_password`（または `GYMBAG_KEY_PASSWORD`）が使用できます：

```caddyfile
{
    key_password super-secret-passphrase
}

corp.example.com {
    reverse_proxy http://localhost:8080
    tls /etc/ssl/corp-fullchain.pem /etc/ssl/corp-privkey.pem
    verify 1
}
```

### 無停止バックグラウンド自動更新

Gymbag は `renew_interval` 秒（デフォルト `43200` = 12 時間）ごとにバックグラウンド更新ループを実行します。ACME 証明書、ローカル PKI リーフ証明書、またはファイル指定証明書の有効期限が `renew_within_days` 日（デフォルト `30` 日）以内になると、バックグラウンドで更新または再読み込みを行い、稼働中の `tls.Certificate` ポインタをアトミックに差し替えます。進行中の HTTP ストリームや WebSocket 接続は一切切断されません。

---

## カスタムファイアウォールルールの作成

保護対象サイトへの受信リクエストは、予測可能な以下の順序で評価されます：

1. **リクエスト形式サニティチェック**（HTTP メソッド検証、URI 最大長、NULL バイトおよび `..` トラバーサル拒否）
2. **ACME HTTP-01 バイパス**（`/.well-known/acme-challenge/*`）
3. **ローカルカスタムルール**（グローバルルール -> サイトルール、上から順に評価・最初の一致で決定）
4. **リモートファイアウォールルール (`remote_rules`)**（Gymbagfile 構文または JSON の HTTP/HTTPS ルールセットを自動更新）
5. **組み込みベースラインルール**（`default_rules off` で無効化されていない限り実行）
6. **サイト単位・IP 単位のレートリミッター**（`rate_limit` / `rate_window`）
7. **IP 脅威インテリジェンス**（`ip_security_action verify` または `block`）
8. **Proof-of-Work 検証**（`verify` レベル `0`–`3`）

カスタムルールは組み込みルール・レートリミッター・脅威フィードよりも**前**に評価されるため、サイトブロックの先頭に `allow` ルールを書くだけで、信頼できるオフィスの CIDR や内部ヘルスチェック、プレビュー用 Cookie を確実にホワイトリスト化できます。

### ワンライナー短縮ルール

構文：`<allow | verify | block> <field> <pattern...>`（`bypass`/`pass`、`challenge`/`pow`、`deny`/`drop` のエイリアスも使用可能）。

| フィールド | 受け付ける値とマッチング方式 | 例 |
| :--- | :--- | :--- |
| `ip` | IPv4、IPv6、または CIDR レンジ（スペースまたはカンマ区切り） | `allow ip 127.0.0.1 10.0.0.0/8 2001:db8::/32` |
| `user_agent` | 大文字小文字を区別しない部分一致、`*` ワイルドカード、または `~regex` | `block user_agent BadBot ~^(?i)python-urllib/` |
| `path` | 完全/部分一致、`*` ワイルドカード、または `~regex` | `verify path /api/export* ~^/v[0-9]+/search` |
| `query` | 生クエリ文字列一致、`*` ワイルドカード、または `~regex` | `block query "union select" ~(?i)<script` |
| `method` | HTTP メソッド（大文字小文字不問） | `verify method POST DELETE` |
| `header` | `<Header-Name> [pattern]`（`""` = 未設定または空、`*` = 存在すれば一致） | `verify header Accept-Language ""` |
| `cookie` | `<cookie_name> [pattern]`（`""` = 未設定または空、`*` = 存在すれば一致） | `allow cookie vip_session *` |

### 複数条件の `rule` ブロック

単一フィールドでは絞り込めない場合は `rule "name" { ... }` ブロックを使用します。**同一フィールド内**の複数パターンは **OR** 条件、同一ブロック内の**異なるフィールド同士**は **AND** 条件になります：

```caddyfile
app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    # ログインやパスワードリセットへの POST リクエストに PoW を要求
    rule "challenge-auth-mutations" {
        action verify
        method POST
        path /login* /api/auth/* /reset-password*
    }

    # Origin ヘッダーが欠落した /api/* への POST リクエストをブロック
    rule "block-headless-api-posts" {
        action block
        method POST
        path /api/*
        header Origin ""
    }
}
```

### リモートファイアウォールルール（`remote_rules`）

グローバルまたはサイト単位で 1 つ以上の HTTP/HTTPS URL から共有ファイアウォールルールを取得します。**Gymbagfile ルール構文**（`allow`/`verify`/`block` ワンライナーおよび `rule "name" { ... }` ブロック）と **JSON**（`[ ... ]` または `{"rules": [ ... ]}`）の両方に対応しており、条件付き `ETag` / `If-Modified-Since` リクエストで検証済みペイロードを `<cert_dir>/rules-cache` にキャッシュします。取得失敗や不正データ時には直前の正常なルールセットを安全に維持します：

```caddyfile
{
    remote_rules https://rules.example.com/org-baseline.rules
    remote_rules_refresh 5m
}

app.example.com {
    reverse_proxy http://localhost:3000
    verify 3
    allow ip 10.0.0.0/8                              # ローカルルールが常に優先
    remote_rules https://rules.example.com/app.json  # サイト固有のリモートルール
}
```

`GET /api/v1/rules/remote` で状態確認、`POST /api/v1/rules/remote/refresh` で即時更新が可能なほか、`--remote-rules` / `GYMBAG_REMOTE_RULES` でも指定できます。

### 組み込みルールの制御（`default_rules on|off`）

`default_rules` はデフォルトで `on` です：
- 保護対象サイトにカスタムルールが**無い**場合、組み込みのブロックプリセット（`/.env`、`/wp-admin`、スキャナー、SQLi/XSS クエリ）と自動化検証プリセットの両方が適用されます。
- カスタムルールを追加すると、ユーザー定義ルールが先に評価され、組み込みの**ブロック**プリセットはその後に残るため、`verify path` ルールを 1 つ追加しただけで `/.git` や `/.env` が無防備になることはありません。
- サイト単位またはグローバルで組み込みルールを完全に無効化し手動制御したい場合は `default_rules off`（または `GYMBAG_DEFAULT_RULES=off`）を設定してください。

---

## CLI・Docker Compose・環境変数・REST API

### CLI リファレンス

カレントディレクトリに `.env` があれば自動で読み込み、フラグで上書きされない限り `./Gymbagfile` または `./Caddyfile` を参照します：

```bash
# 対話ウィザードまたはフラグから Gymbagfile を生成
./gymbag init
./gymbag init -d app.example.com -u http://localhost:3000 -l 3

# ./Gymbagfile または ./Caddyfile でサーバーを起動
./gymbag

# 設定ファイルを指定し、保存時にホットリロードして起動（2秒間隔で監視）
./gymbag run --config Gymbagfile --watch

# システムサービスの登録と管理（systemd, launchd, openrc, supervisor）
./gymbag service install --config Gymbagfile
./gymbag service start
./gymbag service status
./gymbag service render --type systemd   # 任意の OS 上で systemd ユニット定義をプレビュー

# 設定ファイル不要のアドホックなリバースプロキシ
./gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3

# 構文検証とサイト別セキュリティ設定サマリーの表示
./gymbag validate --config Gymbagfile

# Gymbagfile を標準的な 4 スペースインデントに整形して上書き保存
./gymbag fmt --overwrite Gymbagfile

# バージョン出力
./gymbag version
```

| フラグ | 説明 |
| :--- | :--- |
| `-c`, `--config <path>` | `Gymbagfile`、`Caddyfile`、または JSON 設定ファイルのパス |
| `-d`, `--domain <list>` | ドメインを上書き（カンマ区切り） |
| `-u`, `--upstream <url>` | アップストリーム URL を上書き（例：`http://localhost:3000` または `:3000`） |
| `-l`, `--verify <0-3>` | 検証レベルを上書き（`0`、`1`、`2`、`3`、または `off`） |
| `--verify-timeout <dur>` | 認証済み有効期限を上書き（例：`30m`、`12h`、`7d`、秒数；エイリアス `--clearance-ttl`） |
| `--remote-rules <url>` | リモートファイアウォールルール URL を指定（http/https、複数指定またはカンマ区切り可） |
| `--remote-rules-refresh <dur>` | リモートルールの更新間隔（デフォルト `5m` / `300s`） |
| `--no-protect` | CLI 指定サイトの WAF 検証を無効化 |
| `--ssl-provider <name>` | ACME / SSL プロバイダを上書き（`letsencrypt`、`zerossl`、`google`、`litessl`、`local` 等） |
| `--admin <addr>` | ローカル REST Admin API を有効化（例：`127.0.0.1:2019` または `off`） |
| `--http-port <port>` | HTTP リダイレクト＆ACME HTTP-01 用ポート（デフォルト `80`） |
| `--https-port <port>` | HTTPS ゲートウェイポート（デフォルト `443`） |
| `--watch` | 設定ファイルの変更を監視し、ルート・ルール・SNI 証明書をホットリロード |

### Docker & Docker Compose

マルチアーキテクチャ（`linux/amd64`、`linux/arm64`）対応の公式 Alpine コンテナイメージを GitHub Container Registry（`ghcr.io/evil-project/gymbag`）で公開しています。非 root ユーザー UID `10001` かつ `cap_net_bind_service=+ep` 付与済みです。

**`Gymbagfile` をマウントしたクイック `docker run`：**

```bash
docker run -d \
  --name gymbag \
  --restart unless-stopped \
  -p 80:80 -p 443:443 \
  --cap-add NET_BIND_SERVICE \
  --add-host=host.docker.internal:host-gateway \
  -v "$(pwd)/Gymbagfile:/app/Gymbagfile:ro" \
  -v gymbag-certs:/app/certs \
  ghcr.io/evil-project/gymbag:latest
```

**環境変数のみの設定ファイルレス `docker run`：**

```bash
docker run -d \
  --name gymbag \
  -p 80:80 -p 443:443 \
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

**Redis 連携の Docker Compose：**

同梱の `compose.yaml` は Gymbag と `redis:8-alpine` を並べて起動し、純 Go 製 RESP2 Redis クライアントによってワンタイム PoW ナンスおよび固定ウィンドウのレート制限状態をコンテナ再起動や複数レプリカ間で共有します：

```bash
docker compose up -d --build
```

設定は `./Gymbagfile`（`/app/Gymbagfile` へ読み取り専用マウント）から読み込まれ、証明書・シークレット・脅威インテリジェンスキャッシュ・リモートルールキャッシュは名前付きボリューム `gymbag-certs` に永続化されます。ローカルの `.env` に記載した `GYMBAG_*` 変数はファイル設定をきれいに上書きします：

```dotenv
GYMBAG_ACME_EMAIL=ops@example.com
GYMBAG_SSL_PROVIDER=zerossl
GYMBAG_VERIFY_LEVEL=3
GYMBAG_CLEARANCE_TTL=12h
GYMBAG_REMOTE_RULES=https://rules.example.com/baseline.rules
```

バインドマウントの権限設定（`chown 10001:10001`）、`--watch` ホットリロード、マルチコンテナ構成、ローカルビルドの詳細は [Docker & Docker Compose ガイド](docs/ja/guide/docker.md) をご覧ください。

### 環境変数（`GYMBAG_*`）

優先順位は **設定ファイル -> `GYMBAG_*` 環境変数 -> CLI フラグ** です。

| 変数 | 説明 |
| :--- | :--- |
| `GYMBAG_CONFIG` | `Gymbagfile`、`Caddyfile`、または `.json` 設定ファイルのパス |
| `GYMBAG_BIND` | バインドアドレス（ローカルドメインのみの場合は `127.0.0.1`、公開ドメインがある場合は `0.0.0.0`） |
| `GYMBAG_HTTP_PORT` / `GYMBAG_HTTPS_PORT` | HTTP と HTTPS の待受ポート（デフォルト `80` と `443`） |
| `GYMBAG_DOMAIN` / `GYMBAG_DOMAINS` | カンマ区切りのドメイン一覧 |
| `GYMBAG_UPSTREAM` | デフォルトのアップストリーム URL |
| `GYMBAG_UPSTREAMS` | セミコロン区切りのホスト別マップ：`app.example.com=http://web:3000;api.example.com=http://api:8080` |
| `GYMBAG_PROTECT` | 全サイト共通の WAF 保護有効/無効（`on` / `off`） |
| `GYMBAG_PROTECT_DOMAINS` | WAF を有効化するドメインの部分集合（カンマ区切り） |
| `GYMBAG_VERIFY_LEVEL` | 検証レベル：`0` (`always`), `1` (`only suspicious`), `2` (`only ai`), `3` (`suspicious & ai`) |
| `GYMBAG_DEFAULT_RULES` | 組み込みベースラインルールの有効/無効（`on` / `off`、デフォルト `on`） |
| `GYMBAG_REMOTE_RULES` | カンマまたは空白区切りの HTTP/HTTPS リモートルール URL |
| `GYMBAG_REMOTE_RULES_REFRESH` | リモートルールの更新間隔（秒数または期間表記、デフォルト `300` / `5m`） |
| `GYMBAG_DIFFICULTY` | SHA-256 PoW 先頭ゼロ桁数難易度（`1`–`5`、デフォルト `4`） |
| `GYMBAG_CHALLENGE_TTL` / `GYMBAG_CLEARANCE_TTL` | チャレンジ有効期限（`300`s / `5m`）と認証済み期限（`86400`s / `24h`；エイリアス `GYMBAG_VERIFY_TIMEOUT`, `GYMBAG_VERIFIED_TIMEOUT`；`30m`, `12h`, `7d` 等に対応） |
| `GYMBAG_COOKIE_NAME` / `GYMBAG_COOKIE_SECURE` | クリアランス Cookie 名（`waf_clearance`）および `Secure` フラグ（`true`） |
| `GYMBAG_RATE_LIMIT` / `GYMBAG_RATE_WINDOW` | IP あたりのリクエスト上限（`240`）とウィンドウ秒数（`60`） |
| `GYMBAG_TLS_MODE` | `auto`, `acme`, `local`, `selfsigned`, `file` |
| `GYMBAG_SSL_PROVIDER` | `letsencrypt`, `letsencrypt_staging`, `zerossl`, `google`, `ssl.com`, `litessl`, `buypass`, `local`, または URL |
| `GYMBAG_ACME_EMAIL` | ACME アカウント登録用メール（ZeroSSL 自動 EAB にも使用） |
| `GYMBAG_ACME_DIRECTORY` | 明示的な RFC 8555 ACME v2 ディレクトリ URL |
| `GYMBAG_EAB_KID` / `GYMBAG_EAB_HMAC_KEY` | EAB Key ID と base64/base64url HMAC 鍵 |
| `GYMBAG_ACCOUNT_KEY_FILE` | ユーザー指定の ECDSA / RSA ACME アカウント鍵 PEM パス |
| `GYMBAG_CERT_FILE` / `GYMBAG_KEY_FILE` | `file` モード用 TLS 証明書と秘密鍵パス |
| `GYMBAG_KEY_PASSWORD` | 暗号化 PEM 秘密鍵のパスフレーズ |
| `GYMBAG_CERT_DIR` | CA 証明書、ACME 状態、インテリジェンスキャッシュの保存ディレクトリ（デフォルト `certs`） |
| `GYMBAG_RENEW_INTERVAL` / `GYMBAG_RENEW_WITHIN_DAYS` | 更新チェック間隔秒数（`43200`）と更新しきい値日数（`30`） |
| `GYMBAG_AUTO_HOSTS` | `localhost` & `*.internal` 用の `/etc/hosts` 自動管理（`on` / `off`、デフォルト `on`） |
| `GYMBAG_HSTS` | `Strict-Transport-Security` ヘッダーの上書き（`on` / `off`） |
| `GYMBAG_IP_SECURITY` / `GYMBAG_IP_SECURITY_ACTION` | 脅威フィードの有効/無効（`on`/`off`）とアクション（`verify` または `block`） |
| `GYMBAG_IP_REPUTATION` | リアルタイム Shodan InternetDB + StopForumSpam 照会の有効/無効（`on`/`off`） |
| `GYMBAG_AI_IP_LIST` | 公式 OpenAI / Anthropic / Perplexity / Google / Applebot IP リストの有効/無効（`on`/`off`） |
| `GYMBAG_INTEL_REFRESH` | 脅威・AI フィードの更新間隔秒数（デフォルト `21600`） |
| `GYMBAG_REDIS_URL` | オプションの Redis 接続 URL（`redis://[:password@]host:port/db`） |
| `GYMBAG_SECRET` | クリアランス Cookie 用 HMAC シークレット（未設定時は `<cert_dir>/secret` に自動生成） |
| `GYMBAG_ADMIN` | ローカル Admin REST API リッスンアドレス（例：`127.0.0.1:2019` または `off`） |

### REST API

#### 公開 WAF エンドポイント（保護対象サイト上で提供）

| メソッド | パス | 説明 |
| :--- | :--- | :--- |
| `GET` | `/.waf/v1/session` | 現在のサイトのクリアランス状態、`protected`、`verify_level`（`0`–`3`）、`verify_mode`、`clearance_ttl`、（認証済みの場合）`expires_at` / `expires_in` を返却 |
| `DELETE` | `/.waf/v1/session` | 訪問者のクリアランス Cookie を削除（`204 No Content`） |
| `POST` | `/.waf/v1/challenges` | 使い捨て SHA-256 PoW チャレンジを発行（`201 Created`） |
| `POST` | `/.waf/v1/verifications` | PoW 解答を検証して署名 Cookie を設定し、`expires_in` を返却（`/.waf/verify` エイリアス対応） |
| `GET` | `/.waf/assets/{name}` | 埋め込み PNG チャレンジアセットを配信（`/.waf/v1/assets/{name}` エイリアス対応、`ETag` キャッシュ付き） |

#### ローカル管理 API（`admin 127.0.0.1:2019` または `--admin` で有効化）

管理 API はループバック（`127.0.0.1:2019`）または管理用 Unix/WireGuard インターフェースにバインドしてください。認証トークンなしでサーバーを再起動せずにライブのルーティングやファイアウォールルールを変更できます。

| メソッド | パス | 説明 |
| :--- | :--- | :--- |
| `GET` | `/api/v1/health` | ゲートウェイ状態、サイト数、アクティブな `tls_source`、ロード済み `threat_entries`・`ai_entries`・`remote_rules` |
| `GET` | `/api/v1/tls` | アクティブな TLS ソース、証明書パス、SSL プロバイダ、HSTS 状態、対応 CA カタログ |
| `POST` | `/api/v1/tls/renew` | 即時証明書更新とアトミックなホットリロードを強制実行 |
| `GET` | `/api/v1/intel` | フィードごとの脅威・AI クローラー IP 数、最終更新日時、レピュテーションキャッシュ統計 |
| `POST` | `/api/v1/intel/refresh` | バックグラウンドフィード更新を即時実行し最新カウントを返却 |
| `GET` | `/api/v1/rules/remote` | URL ごとのリモートルール数、キャッシュ状態、更新間隔、直近のフェッチエラー |
| `POST` | `/api/v1/rules/remote/refresh` | 全リモートルール URL の即時フェッチ＆WAF エンジンのアトミックリロードを実行 |
| `GET` | `/api/v1/config` | 現在のランタイム設定を取得（`secret`、`key_password`、`eab_hmac_key` はマスク） |
| `PUT` | `/api/v1/config` | ランタイム設定全体をアトミックに検証・置換 |
| `GET` | `/api/v1/sites` | 設定済み全サイトと WAF ポリシーを一覧取得 |
| `POST` | `/api/v1/sites` | 実行時に新しいサイトを追加（`201 Created`） |
| `GET` | `/api/v1/sites/{domain}` | 単一サイトの設定詳細を取得 |
| `PUT` | `/api/v1/sites/{domain}` | 単一サイトの設定を置換 |
| `PATCH` | `/api/v1/sites/{domain}` | `verify_level`、`protect`、`difficulty`、`clearance_ttl`、`upstreams`、`rate_limit`、`rules`、`remote_rules` をライブパッチ |
| `DELETE` | `/api/v1/sites/{domain}` | サイトを削除（`204 No Content`；最後の 1 サイトは削除不可） |
| `GET` | `/api/v1/sites/{domain}/rules` | サイトのカスタムファイアウォールルール一覧 |
| `POST` | `/api/v1/sites/{domain}/rules` | カスタムファイアウォールルールをライブ追加（`201 Created`） |
| `PUT` | `/api/v1/sites/{domain}/rules` | サイトの全カスタムファイアウォールルールを置換 |
| `DELETE` | `/api/v1/sites/{domain}/rules/{index_or_name}` | インデックス番号またはルール `name` でカスタムルールを削除（`204 No Content`） |

**緊急時のライブ緩和コマンド例：**

```bash
# トラフィックスパイク中に app.example.com を verify level 0（全リクエスト検証）へ引き上げ
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"verify_level": 0, "difficulty": 5}'

# Gymbagfile を編集せず即座に攻撃元 CIDR をブロック
curl -X POST http://127.0.0.1:2019/api/v1/sites/app.example.com/rules \
  -H "Content-Type: application/json" \
  -d '{"name": "drop-abusive-subnet", "action": "block", "ips": ["198.51.100.0/24"]}'

# IP 脅威・AI クローラーリストの即時更新を強制実行
curl -X POST http://127.0.0.1:2019/api/v1/intel/refresh
```

---

## Caddy・Nginx とのベンチマーク比較

Apple M4（10 コア、macOS、Go `1.27.0`）環境で、3 製品すべて同一の ECDSA P-256 証明書による TLS 1.3 終端、64 並列ワーカー、`1 KiB` レスポンス、`1024` アップストリーム keep-alive 接続で測定（全試行 `0` エラー）：

| サーバー＆モード | HTTP/1.1 TLS (RPS) | H1 p50 / p99 | HTTP/2 TLS (RPS) | H2 p50 / p99 | バイナリサイズ | 負荷時 RSS |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **85,203** | 0.45 / 6.20 ms | **88,775** | 0.65 / 1.96 ms | 1.5 MB (+libs) | 5.8 MiB |
| **Gymbag `v0.1.0`** (`verify 3`, フル WAF 有効) | **72,517** | 0.74 / 3.14 ms | **66,096** | 0.82 / 3.48 ms | 12.9 MB (静的) | 54.3 MiB |
| **Gymbag `v0.1.0`** (`protect off`, 純プロキシ) | **72,479** | 0.74 / 3.15 ms | **64,048** | 0.83 / 3.94 ms | 12.9 MB (静的) | 52.1 MiB |
| **Gymbag `v0.1.0`** (`verify 0`, Cookie 認証済) | **65,490** | 0.79 / 3.93 ms | **62,108** | 0.84 / 4.27 ms | 12.9 MB (静的) | ~54 MiB |
| **Caddy `v2.11.9`** (Go, keep-alive 調整済) | **54,741** | 0.90 / 4.90 ms | **39,207** | 1.30 / 6.18 ms | 49.0 MB | 76.8 MiB |

- **Caddy 比で HTTP/1.1 は +32%、HTTP/2 は +63%〜+69% 高速**でありながら、メモリ消費は約 30% 少なくバイナリサイズも 3.8 倍コンパクト（しかも Gymbag のフル WAF 有効状態での数値）。
- **C 言語製 Nginx の HTTP/1.1 スループットの 85%** を発揮しつつ、p99 テールレイテンシは約 **2 倍低遅延**（`3.14 ms` vs `6.20 ms`）。
- TLS 上の `/.env` スキャンに対して **140,680 req/s** のエッジ WAF 遮断性能（p50 `0.35 ms`、アップストリーム負荷ゼロ）。

詳細な測定手法、Go マイクロベンチマーク（`go test -bench`）、使用設定ファイルは [ベンチマークとパフォーマンス](docs/ja/guide/benchmarks.md) を参照してください。

---

## プロジェクト構成

```text
.
├── cmd/gymbag/          # スタンドアロン CLI エントリポイント
├── docs/                # 多言語 VitePress ドキュメント（EN, 简体中文, 日本語, 한국어, Español）
├── internal/
│   ├── cli/             # サブコマンド (init, service, run, reverse-proxy, validate, fmt, version)、.env ＆ウォッチャー
│   ├── config/          # 設定モデル、Gymbagfile/Caddyfile・期間・リモートルールパーサー、GYMBAG_* ローダー
│   ├── host/            # ドメイン正規化、IPv6 ブラケット処理、ワイルドカード一致
│   ├── intel/           # O(1) プレフィックスセット、自動更新脅威＆AI クローラーフィード、リアルタイム IP レピュテーション
│   ├── localpki/        # 2 階層ローカル CA ("Neuro-sama Headquarters Root CA") ＆ /etc/hosts マネージャー
│   ├── proxy/           # ロックフリーマルチサイトルーター、プーリング HTTP ＆ RFC 6455 WebSocket プロキシ
│   ├── server/          # 統合 HTTPS ゲートウェイ、HTTP->HTTPS/ACME リダイレクター、REST Admin API
│   ├── service/         # システムサービス定義レンダラー＆マネージャー (systemd, launchd, openrc, supervisor)
│   ├── store/           # StateStore インターフェース、64 シャードインメモリストア、純 Go RESP2 Redis クライアント
│   ├── tlsmanager/      # マルチ CA ACME v2 (EAB ＆ ZeroSSL 自動 EAB)、ローカル PKI、ロックフリー SNI 証明書管理
│   └── waf/             # PoW 検証、シャード化 HMAC クリアランスキャッシュ、Aho-Corasick DFA、リモートルール、UI
├── Dockerfile
├── compose.yaml
├── flake.nix
├── Gymbagfile
└── go.mod
```

---

## Star 履歴

<a href="https://www.star-history.com/#Evil-Project/gymbag&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
 </picture>
</a>
