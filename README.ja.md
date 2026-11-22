# Gymbag

<p align="center">
  <img src="docs/public/mascot-happy.png" alt="Gymbag mascot" width="140" />
</p>

<p align="center">
  <strong>純 Go 製シングルバイナリ HTTP/HTTPS・SSH・Minecraft 対応 Proof-of-Work ゲートウェイ</strong>
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

Gymbag は、Anubis の発想を取り入れた Proof-of-Work (PoW) WAF を備え、SSH と Minecraft サーバーも保護できる単一バイナリの HTTPS リバースプロキシです。Go 標準ライブラリのみで実装しており、外部 Go モジュールへの依存はありません。HTML ページ、ブラウザ用 SHA-256 ソルバー、マスコット画像も `go:embed` でバイナリに組み込まれています。

セルフホストのアプリや Git フォージ、SSH ポート、Minecraft サーバーがさらされるトラフィックはどこも似たようなものです。`robots.txt` を無視する LLM スクレイパー、`/.env` や `/wp-admin` を探るボット、SSH ブルートフォース、ログインフラッド。nginx や Caddy に PoW 用サイドカー、fail2ban、独自のファイアウォールスクリプトを組み合わせる代わりに、Gymbag は TLS 終端、マルチ CA ACME、HTTP/WebSocket プロキシ、SSH/Minecraft TCP ゲーティング、IP 脅威・VPN・AI リスト、ファイアウォールルールを 1 プロセスにまとめています。設定は `gymbag.json` または Caddy 風の `Gymbagfile` で記述できます。

## クイックスタート

ビルド済みの静的バイナリ（Linux・macOS・Windows の `amd64`/`arm64` 向け `CGO_ENABLED=0`）、Debian/Ubuntu 用 `.deb` パッケージ、Arch Linux 用 `.pkg.tar.zst` パッケージ、NixOS 用 tarball/NAR を [GitHub Releases](https://github.com/Evil-Project/gymbag/releases) で公開しています。GHCR のコンテナイメージや Nix flake を使うか、ソースから直接ビルドすることもできます。

```bash
go build -o gymbag ./cmd/gymbag
```

**Debian / Ubuntu:**
```bash
sudo apt install ./gymbag_<version>_linux_amd64.deb
sudo gymbag init -o /etc/gymbag/Gymbagfile --format gymbagfile
sudo systemctl enable --now gymbag
```

**Arch Linux:**
```bash
sudo pacman -U gymbag-<version>-1-x86_64.pkg.tar.zst
# または packaging/arch からビルド:
cd packaging/arch && makepkg -si
```

**Nix / NixOS:**
```bash
nix run github:Evil-Project/gymbag -- --help
nix profile install github:Evil-Project/gymbag
```
静的リンクされた Linux リリースバイナリは NixOS 上で `patchelf` や `nix-ld` なしにそのまま動作し、`nixosModules.default` で宣言的な `services.gymbag` モジュールも利用できます。

起動までは 2 コマンドです。設定ファイルを生成し（デフォルトは `gymbag.json`、`--format gymbagfile` で `Gymbagfile` を出力）、起動します。

```bash
# HTTP/HTTPS サイトを保護
./gymbag init -d app.example.com -u http://localhost:3000
./gymbag

# SSH を保護（sshd をランダムポートへ移動しファイアウォールを設定）
sudo ./gymbag init -t ssh
sudo ./gymbag

# Minecraft Java Edition を保護（自動選択した Java で server.jar を自動起動 + ポートをランダム化）
./gymbag init -t minecraft --server-jar /srv/mc/paper.jar --mc-jvm-args "-Xms1G -Xmx4G"
./gymbag

# 管理起動を使わず既存の Minecraft サーバーを保護（server.properties のポートをランダム化）
./gymbag init -t minecraft --server-properties /path/to/server.properties
./gymbag

# 3 つすべてを 1 プロセスで保護
sudo ./gymbag init -t http,ssh,minecraft -d app.example.com -u :3000 --server-jar /srv/mc/paper.jar
sudo ./gymbag
```

既存の **nginx**（`nginx.conf`）、**Apache**（`httpd.conf` / `apache2.conf`）、**Caddyfile v2** の設定ファイルをそのまま読み込むことも、システムサービス（`systemd`、`launchd`、`openrc`、`supervisor`）として登録することもできます。

```bash
./gymbag init -d app.example.com -u :3000 --service auto
./gymbag service start
```

設定ファイルを書かずに単発でプロキシを試したい場合は次のように実行します。

```bash
./gymbag reverse-proxy --from app.internal --to :3000 --verify 3
```

> **1024 番未満のポート:** `:80` は HTTP→HTTPS リダイレクトと ACME HTTP-01、`:443` は HTTPS、`:22` は SSH で待ち受けます。Linux では `sudo setcap 'cap_net_bind_service=+ep' ./gymbag` を付与すると root 権限なしでバインドできます（`.deb` パッケージと Arch パッケージでは自動で付与されます）。非特権ユーザーでローカル Web テストを行う場合は `--http-port 8080 --https-port 8443` を指定してください。

### ルールを書かずに動く標準防御

正規表現を書いたりブロックリストを取り込んだりしなくても、サイトの前に置くだけで保護が働きます。引数なしの `verify`（レベル `3`: `suspicious & ai`）または `block`（脅威インテル一致時にチャレンジではなく `403` を返却）を指定すると、次の 3 層が有効になります。

1. **組み込みベースラインルール（`default_rules on`）**
   - **機密パス（`403`）**: `/.env`、`/.env.*`、`/.git`、`/.git/*`、`/.svn/*`、`/.hg/*`、`/.DS_Store`、`/.aws/*`、`/.ssh/*`、`/docker-compose.yml`、`/docker-compose.yaml`、`*.bak`、`*.sql`、`*.sqlite`。
   - **CMS・フレームワーク探査（`403`）**: `/wp-admin`、`/wp-admin/*`、`/wp-login.php`、`/xmlrpc.php`、`/phpmyadmin*`、`/vendor/phpunit*`、`/cgi-bin/*`、`/actuator/env*`、`/actuator/heapdump*`。
   - **脆弱性スキャナー（`403`）**: `sqlmap`、`nikto`、`nuclei`、`masscan`、`zgrab`、`gobuster`、`dirbuster`、`ffuf`、`wfuzz`、`feroxbuster`、`acunetix`、`nessus`、`openvas`。
   - **インジェクション・パストラバーサル（`403`）**: クエリ文字列内の `UNION SELECT`、`<script`、`javascript:`、`/etc/passwd`、`information_schema`、`../`、`cmd.exe`、`/bin/sh`。さらに NULL バイトや `%2e%2e` トラバーサルをリクエスト形状チェックで拒否します。
   - **自動化クライアント（PoW チャレンジ）**: カスタムルールが未設定のサイトでは、CLI ライブラリ（`curl`、`wget`、`python-requests`、`python-httpx`、`go-http-client`、`aiohttp`、`okhttp`、`axios`、`node-fetch`、`scrapy`）、ヘッドレスブラウザ（`headlesschrome`、`playwright`、`puppeteer`、`selenium`、`phantomjs`）、空の `User-Agent` を PoW 検証ページへ誘導します。
2. **自動更新 IP 脅威インテルと VPN/Tor 検出**
   - 起動時と 6 時間ごとに [IPsum (level 3+)](https://raw.githubusercontent.com/stamparm/ipsum/master/levels/3.txt)、[Spamhaus DROP IPv4](https://www.spamhaus.org/drop/drop_v4.json) / [IPv6](https://www.spamhaus.org/drop/dropv6.txt)、[abuse.ch Feodo Tracker](https://feodotracker.abuse.ch/downloads/ipblocklist.txt) を取得し、`ETag` と `If-Modified-Since` を使って `<cert_dir>/intel-cache` にキャッシュします。
   - **Shodan InternetDB**（`internetdb.shodan.io`）と **StopForumSpam**（`api.stopforumspam.org`）へキー不要でライブ照会を行います。インメモリキャッシュは 1 時間有効で、上流が遅い場合はフェイルオープンで通過させるため通常の訪問者を止めません。
   - **無料 VPN/プロキシ/Tor 検出（`vpn_detection on`）**: 自動更新される X4BNet VPN IPv4/IPv6 リストと Tor 一括出口ノードリストに加え、`api.ipquery.io` と `api.ipapi.is` へのキー不要なライブ照会（キャッシュ 1 時間、1.2 秒フェイルオープン）を行います。一致した IP は HTTP・SSH・Minecraft の検証レベル `1` および `3` で **suspicious** 扱いになります。
3. **公式 AI クローラー IP リスト**
   - **OpenAI**（`gptbot`、`searchbot`、`chatgpt-user`）、**Anthropic**（`claude.com/crawling/bots.json`）、**Perplexity**（`perplexitybot`、`perplexity-user`）、**Google**（`user-triggered-agents`、`user-triggered-fetchers-google`）、**Apple**（`applebot`）が公開している CIDR リストを追跡し、`User-Agent` を Chrome に偽装していても送信元 IP から判定します。

## SSH・Minecraft の保護

未認証の SSH または Minecraft 接続が Gymbag に届き、設定された検証レベルに該当すると、Gymbag はバックエンドのデーモンやゲームサーバーには転送せず、プロトコル自身の切断メッセージ内にワンタイム HTTPS リンク（`/v/<token>`）を含めて返します。

- **SSH（デフォルト `port` `22`）**: `sudo ./gymbag init -t ssh` を実行すると、`20000–59999` の範囲から未使用のローカルポートを選び、`/etc/ssh/sshd_config` を `/etc/ssh/sshd_config.gymbag.bak` へバックアップした上で、すべての `Match` ブロックより前に新しい `Port` を書き込みます。続けて `sshd -t` で検証し（失敗時は自動ロールバック）、`sshd` をリロードして稼働中の Linux ファイアウォール（`ufw`、`firewalld`、`nftables`、`iptables`+`ip6tables`）を更新し、バックエンドポートへの外部アクセスを遮断して `22` と `8443` を開放します。（`sshd_config` 変更中は別の SSH セッションを開いたまま作業してください。手動で設定したい場合は `--ssh-upstream 127.0.0.1:2222`、`--no-auto-setup`、`--no-firewall` を使います。）ブラウザでチャレンジを完了した IP からの接続は、SSH ハンドシェイクに触れず生の TCP リレーでそのまま通過します。レベル `1` と `3` では、不正なバナーやスクリプト化された SSH クライアント（`libssh`、`paramiko`、`zgrab`、`masscan`、`nmap`、`asyncssh`、`ssh2go`、`jsch`、`hydra`、`medusa`、`patator`、`python`、`curl`、`ssh-2.0-go`）がチャレンジ対象となり、それ以外のクライアントは RFC 4253 `SSH_MSG_DISCONNECT` 経由でターミナル上に検証 URL を受け取ります。
- **Minecraft Java（デフォルト `port` `25565`）**: `./gymbag init -t minecraft --server-properties ./server.properties` は `server.properties` を `server.properties.gymbag.bak` にバックアップし、`server-port` を `20000–59999` の空きポートへ移動して、空またはワイルドカードの `server-ip` を `127.0.0.1` に設定します。Server List Ping（`nextState == 1` および旧形式 `0xFE`）はルールで遮断されない限りそのまま通すため、MOTD やプレイヤー数は通常どおり表示されます。Login または Transfer 時に検証が必要な場合は、Minecraft Login Disconnect パケット（`0x00`）を返し、**「Connection Lost」**ダイアログ内に検証リンクを表示します。
- **共有 HTTPS 検証ポータル（デフォルト `:8443`）**: SSH と Minecraft は `verify_port` を共有します（独自ドメインを使う場合は `--verify-url https://verify.example.com:8443` を指定し、空欄のままにすると自動検出したサーバー IP が使われます）。ACME でサーバー IP を使う場合、Gymbag は **RFC 8738** IP 証明書（CA が提供していれば `shortlived` プロファイル）を要求し、HTTP-01 が到達できない環境ではローカル CA または自己署名 IP 証明書へフォールバックします。`verify_port` が `https_port` と一致し HTTP サイトが有効な場合は、ポータルもポート `443` を共有します。

```caddyfile
{
    verify_url https://verify.example.com:8443
    verify_port 8443
}

ssh {
    port 22
    upstream 127.0.0.1:41822
    verify 3
    rate_limit 60 60s
}

minecraft {
    port 25565
    upstream 127.0.0.1:48291
    server_properties /srv/mc/server.properties
    verify 3
    allow ip 192.168.1.0/24
}
```

設定例（`gymbag.json` を含む）と動作仕様の詳細は [SSH・Minecraft 保護ガイド](docs/ja/guide/ssh-minecraft.md) を参照してください。

## ローカル開発（`localhost` と `*.internal`）

`localhost`、`*.localhost`、`*.internal`（`app.internal`、`api.internal`）、`*.local`、`*.test` を使うサイトでは、Gymbag が `/etc/hosts` とローカル TLS 証明書を自動で管理します。

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

### `/etc/hosts` の自動管理

`auto_hosts on`（デフォルト）が有効な場合、Gymbag は起動時に `/etc/hosts`（Windows では `C:\Windows\System32\drivers\etc\hosts`）を確認し、未登録のローカルドメインに対して IPv4・IPv6 のループバックエントリを追記します。

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

既存の `localhost` 行には手を加えません。`Ctrl+C` または `SIGTERM` で終了する際、ファイルを再読み込みして実行中の手動編集を残したまま `# gymbag-managed:<instanceID>` 行だけを削除します。macOS/Linux で `/etc/hosts` を書き換えるには root 権限が必要ですが、非特権ユーザーで起動した場合は警告ログを出してそのまま起動を続けます。`.internal` の名前解決を `dnsmasq` や手動管理に任せる場合は `auto_hosts off`（`GYMBAG_AUTO_HOSTS=off`）を設定してください。

### ローカル CA

再起動のたびに使い捨ての自己署名証明書を作り直す代わりに、Gymbag は `<cert_dir>/ca/`（デフォルト `certs/ca/`）に 2 階層の ECDSA P-256 CA を維持します。

1. **Root CA**: `"Neuro-sama Headquarters Root CA"`（`certs/ca/neuro-sama-root-ca.pem`、有効期限 10 年、`MaxPathLen: 1`）
2. **Intermediate CA**: `"Gymbag & Swarm SSL Local CA"`（`certs/ca/gymbag-swarm-local-ca.pem`、有効期限 5 年、`MaxPathLen: 0`）
3. **Leaf 証明書**: `certs/local-fullchain.pem` + `certs/local-privkey.pem`（有効期限 90 日、残り 30 日以内で更新、DNS SAN と `127.0.0.1`・`::1` を付与）

`"Neuro-sama Headquarters Root CA"` を一度信頼ストアに登録すれば、ブラウザも `curl` もすべてのローカルサイトを検証済みとして受け入れます。

```bash
# macOS
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain certs/ca/neuro-sama-root-ca.pem

# Debian / Ubuntu
sudo cp certs/ca/neuro-sama-root-ca.pem /usr/local/share/ca-certificates/neuro-sama-root-ca.crt && sudo update-ca-certificates

# Fedora / RHEL / Arch Linux
sudo trust anchor --store certs/ca/neuro-sama-root-ca.pem
```

> **Firefox:** macOS/Windows（および `p11-kit` 対応の Linux）では `about:config` で `security.enterprise_roots.enabled` を `true` にするか、*設定 -> プライバシーとセキュリティ -> 証明書 -> 証明書を表示 -> 認証局* から `certs/ca/neuro-sama-root-ca.pem` をインポートしてください。

## 複数サイトの管理と検証レベル（`0`–`3`）

1 つのプロセスで複数ドメインをホストし、ラウンドロビンのアップストリーム負荷分散と RFC 6455 WebSocket アップグレードを処理できます。サイトごと（および SSH/Minecraft サービスごと）に検証レベル（`0`–`3`）または PoW 無効化（`protect off`）を指定します。

| レベル | 名称 | チャレンジ対象 | 主な用途 |
| :---: | :--- | :--- | :--- |
| `0` | `always` | 有効なクリアランス Cookie または認証済み IP を持たない全員 | 管理ツール、ステージング環境、L7 フラッド対策 |
| `1` | `only suspicious` | 脅威フィード、ライブ低評価 IP、VPN/プロキシ/Tor IP、スキャナー、CLI ライブラリ、ヘッドレスブラウザ、ヘッダー欠落/偽装、攻撃プローブ | 通常のブラウザにチャレンジを見せない公開アプリや API |
| `2` | `only ai` | 公式 AI クローラー IP 範囲、または AI ボットの `User-Agent` / `X-Ai-Agent` / `Sec-Agent` ヘッダー | 人間と `curl` は素通ししたいドキュメント・ブログ・Git フォージ |
| `3` | `suspicious & ai` | `1` と `2` の組み合わせ（引数なし `verify`、SSH、Minecraft のデフォルト） | 公開本番 Web サイト |
| — | `protect off` | PoW チャレンジなしのプロキシ動作（明示的なカスタムルールは評価） | 静的アセット配信、Webhook、ミラー |

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
    verify_timeout 30m
}

static.example.com {
    reverse_proxy http://localhost:9000
    protect off
}
```

SHA-256 チャレンジ（`difficulty` `1`–`5`、デフォルト `4`）を解くと、ホストとクライアント IP に紐付いた `HttpOnly`・`SameSite=Lax` の HMAC クリアランス Cookie（`waf_clearance`、デフォルト `24h` / `86400s`）が発行されます。有効期限は `verify_timeout` / `clearance_ttl`（秒数、または `30m`、`12h`、`1h30m`、`7d`、`2w` などの期間表記）、`--verify-timeout`、`GYMBAG_VERIFY_TIMEOUT`、`PATCH /api/v1/sites/{domain}` でグローバルまたはサイト単位に変更できます。認証済み訪問者が `GET /.waf/v1/session` を叩くと `clearance_ttl`、`expires_at`、`expires_in` が返ります。

## パスルート・静的ファイル・リダイレクト・設定インポート

サイト内には `reverse_proxy` と並べて順序付きの**パスルート**を書けます。最初に一致したルートがリクエストを処理し、どれにも一致しない場合はサイトのデフォルトアップストリームへ渡ります（未設定なら `404`）。各ルートが持つハンドラは、リバースプロキシ、静的ファイルサーバー（`root`）、固定 `respond`、`redir` のいずれか 1 つです。

```caddyfile
app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    handle_path /static/* {
        root ./dist
        file_server browse
    }
    @health path /healthz /readyz
    respond @health "ok" 200
    redir /old/* https://{host}/new{uri} 301
    handle /api/* {
        reverse_proxy http://localhost:8080
        header X-Served-By gymbag
    }
    try_files {path} /index.html
}
```

パスマッチャーには完全一致（`/exact`）、グロブ（`/api/*`、`*.php`）、Go RE2 正規表現（`~^/v[0-9]+/`）を指定できます。リダイレクト先は `{scheme}`、`{host}`、`{uri}`、`{path}`、`{query}` を展開します。静的ルートは `index`、`browse`、SPA `fallback` に対応し、`header` はレスポンスヘッダーの追加または `-` プレフィックスでの削除を行います。なお、ファイアウォールルールと検証は常にルートハンドラより先に実行されます。

`gymbag.json` では同等の定義を `routes` 配列に書きます。

```json
{
  "routes": [
    {"path": "/static/*", "strip_prefix": "/static", "root": "./dist", "browse": true},
    {"path": "/old/*", "redirect": {"to": "https://{host}/new{uri}", "code": 301}},
    {"path": "/api/*", "upstreams": ["http://localhost:8080"]}
  ]
}
```

ルートの編集には `gymbag config route list|add|update|remove` または `PATCH /api/v1/sites/{domain}` を使います。

**既存設定のインポート:** `gymbag run -c /etc/nginx/nginx.conf`（または `httpd.conf` / `apache2.conf`）で nginx・Apache 設定をそのまま実行でき、`gymbag config convert -c <file> -o gymbag.json --format json` で nginx、Apache、Caddyfile v2（スニペット、`import`、`@matchers`、`handle`、`route`、`file_server`、`respond`、`redir`、`header` 対応）を変換できます。

## SSL プロバイダ・独自鍵・証明書更新

公開ドメインのデフォルトは `tls auto` で、ACME HTTP-01 経由で Let's Encrypt から証明書を取得します。CA を変更するには `ssl_provider`（別名: `acme_provider`、`ca`、`cert_issuer`）または `tls <provider>` を指定します。

| プロバイダキーワード | 認証局 (CA) | EAB 要件 |
| :--- | :--- | :--- |
| `letsencrypt` (`le`) | Let's Encrypt (本番) | なし |
| `letsencrypt_staging` | Let's Encrypt (ステージング) | なし |
| `zerossl` | ZeroSSL | `email` 設定時は自動取得、または `eab <kid> <hmac_key>` |
| `google` (`gts`) | Google Trust Services | `eab <kid> <hmac_key>` が必須 |
| `ssl.com` (`sslcom`, `ssl.com-rsa`) | SSL.com (ECC または RSA) | `eab <kid> <hmac_key>` が必須 |
| `litessl` (`trustasia`) | LiteSSL by TrustAsia | 標準 DV では不要 |
| `buypass` | Buypass Go SSL | なし |
| `local` | ローカル 2 階層 PKI | ローカル CA (`Neuro-sama Headquarters Root CA`) |
| `https://...` | 任意の RFC 8555 ACME v2 CA | 任意の `eab <kid> <hmac_key>`（`step-ca`、Pebble 等） |

**ZeroSSL** では `eab` を明示しない限り、`email` を設定するだけで EAB 資格情報を自動取得します。**Google Trust Services** と **SSL.com** では `eab YOUR_EAB_KEY_ID YOUR_EAB_HMAC_KEY` を渡してください（パディング付き Base64 とパディングなし Base64URL の両方に対応し、`eab_hmac_key` は Admin API の出力でマスクされます）。ACME 証明書はプロバイダごとに保存されるため（ZeroSSL なら `certs/acme-zerossl-fullchain.pem`、Let's Encrypt なら `certs/acme-fullchain.pem`）、`ssl_provider` を切り替えると即座に新しい CA から取得し直します。

既存の PEM ファイルも持ち込めます。
- `account_key /etc/gymbag/account.pem`: 既存の ECDSA（P-256/P-384）または RSA ACME アカウント鍵。
- `ssl_key /etc/gymbag/domain.key`: ACME CSR 署名用の既存 ECDSA/RSA 秘密鍵。
- `tls <cert> <key>`: 外部管理の証明書をグローバルまたはサイト単位（SNI）で指定。暗号化秘密鍵には `key_password`（`GYMBAG_KEY_PASSWORD`）を使います。

バックグラウンドタスクが `renew_interval` 秒（デフォルト `43200` = 12 時間）ごとに証明書を監視し、有効期限まで `renew_within_days` 日（デフォルト `30` 日）を切ると更新または再読み込みを行い、稼働中の HTTP ストリームや WebSocket を切らずに `tls.Certificate` をアトミックに差し替えます。

## カスタムファイアウォールルール

保護対象の HTTP リクエストは次の順で評価されます。

1. リクエスト形式チェック（HTTP メソッド、最大 URI 長、NULL バイトおよび `..` / `%2e%2e` の拒否）
2. ACME HTTP-01（`/.well-known/acme-challenge/*`）
3. ローカルカスタムルール（グローバルルール → サイトルールを上から評価、最初の一致で決定）
4. リモートルール（`remote_rules`、HTTP/HTTPS 経由で自動更新）
5. 組み込みベースラインルール（`default_rules off` でない限り有効）
6. サイト単位・IP 単位のレートリミッター（`rate_limit` / `rate_window`）
7. IP 脅威インテリジェンス（`ip_security_action verify` または `block`）
8. Proof-of-Work チェック（`verify` `0`–`3`）

カスタムルールはベースラインルール・レート制限・脅威フィードより前に実行されるため、サイトブロックの先頭に `allow` を置くだけでオフィスの CIDR やヘルスチェック、プレビュー用 Cookie を確実に許可できます。

### 短縮ルール構文

構文: `<allow | verify | block> <field> <pattern...>`（別名 `bypass`/`pass`、`challenge`/`pow`、`deny`/`drop`/`reject` も使用可）

| フィールド | 値と一致ルール | 例 |
| :--- | :--- | :--- |
| `ip` | IPv4、IPv6、CIDR | `allow ip 127.0.0.1 10.0.0.0/8 2001:db8::/32` |
| `user_agent` | 大文字小文字を区別しない部分一致、`*` グロブ、`~regex` | `block user_agent BadBot ~^(?i)python-urllib/` |
| `path` | 完全一致/部分一致、`*` グロブ、`~regex` | `verify path /api/export* ~^/v[0-9]+/search` |
| `query` | 生/デコード済みクエリ一致、`*` グロブ、`~regex` | `block query "union select" ~(?i)<script` |
| `method` | HTTP メソッド（大文字小文字不問） | `verify method POST DELETE` |
| `header` | `<Header-Name> [pattern]`（`""` = 欠落または空、`*` = 存在） | `verify header Accept-Language ""` |
| `cookie` | `<cookie_name> [pattern]`（`""` = 欠落または空、`*` = 存在） | `allow cookie vip_session *` |

### 複数条件の `rule` ブロック

`rule "name" { ... }` 内では、同じフィールドに並べた複数値が OR、異なるフィールド同士が AND で評価されます。

```caddyfile
app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    rule "challenge-auth-mutations" {
        action verify
        method POST
        path /login* /api/auth/* /reset-password*
    }

    rule "block-headless-api-posts" {
        action block
        method POST
        path /api/*
        header Origin ""
    }
}
```

### リモートルールと動的ローカルルールファイル

`remote_rules` をグローバルまたはサイト単位で指定すると、Gymbagfile ルール構文または JSON（`[ ... ]` もしくは `{"rules": [ ... ]}`）を配信する HTTP/HTTPS URL からルールを読み込みます。検証済みペイロードは `ETag` / `If-Modified-Since` 付きで `<cert_dir>/rules-cache` にキャッシュされ、不正な更新があっても直前の正常なルールセットが維持されます。

ローカルの `.rules`・`.json` ファイルやフォルダも `rules_file`、`rules_dir`、`ruleset` ブロック（または `gymbag ruleset add`）で読み込めます。ソースに `dynamic` を指定すると `rules_watch_interval`（デフォルト `2s`）ごとに変更を監視してホットリロードします。

```caddyfile
{
    remote_rules https://rules.example.com/org-baseline.rules
    remote_rules_refresh 5m
    rules_watch_interval 2s
}

app.example.com {
    reverse_proxy http://localhost:3000
    rules_file ./rules/custom.rules dynamic
    rules_dir ./rules.d dynamic
}
```

`default_rules` のデフォルトは `on` です。サイトにカスタムルールを追加すると、ユーザー定義ルールが先に評価され、組み込みの **block** ルールはその後に残ります。`verify path` を 1 つ追加しただけで `/.env` が無防備になることはありません。組み込みルールを完全に切る場合は `default_rules off` を指定してください。

## CLI・Docker Compose・環境変数・REST API

### CLI リファレンス

作業ディレクトリに `.env` があれば読み込み、続けて `./gymbag.json`、`./Gymbagfile`、`./Caddyfile` を順に探します（`-c` で JSON、Gymbagfile、Caddyfile v2、`nginx.conf`、Apache 設定ファイルを明示指定することもできます）。

```bash
# 設定ファイルの生成（デフォルトは gymbag.json、--format gymbagfile で Gymbagfile）
./gymbag init
./gymbag init -d app.example.com -u http://localhost:3000 -l 3 --rules-dir ./rules.d --rules-dynamic
./gymbag init -t ssh --verify-url https://verify.example.com:8443
./gymbag init -t minecraft --server-properties ./server.properties
./gymbag init -t http,ssh,minecraft -d app.example.com -u :3000 --server-properties ./server.properties
./gymbag init --format gymbagfile -o Gymbagfile -d app.example.com -u :3000

# サイト・ルート・グローバル設定の管理と形式変換
./gymbag config list
./gymbag config add -d api.example.com -u :8080 -l 1
./gymbag config update -d api.example.com -l 3 --verify-timeout 6h
./gymbag config route add -d api.example.com --path /static/* --strip-prefix /static --root ./dist
./gymbag config remove -d api.example.com
./gymbag config set admin 127.0.0.1:2019
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json

# ルールファイル・フォルダ・個別ルールの管理
./gymbag ruleset init ./rules/custom.rules
./gymbag ruleset add --source ./rules --dynamic --name local-dir
./gymbag ruleset update local-dir --static
./gymbag ruleset remove local-dir
./gymbag ruleset validate ./rules
./gymbag ruleset rule add --file ./rules/custom.rules --action block --name bad-bot --ua "*BadBot*"

# 起動・監視・検証・整形・OS サービス管理
./gymbag
./gymbag run --config gymbag.json --watch
./gymbag service install --config gymbag.json
./gymbag service start
./gymbag service render --type systemd
./gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3
./gymbag validate --config gymbag.json
./gymbag fmt --overwrite Gymbagfile
./gymbag version
```

### Docker と Docker Compose

`ghcr.io/evil-project/gymbag` にマルチアーキテクチャの Alpine イメージ（`linux/amd64`、`linux/arm64`）を公開しています。UID `10001`・`cap_net_bind_service=+ep` で動作します。

```bash
# Gymbagfile をマウントした docker run 例
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

`compose.yaml` は Gymbag と `redis:8-alpine` を組み合わせて起動し、純 Go 製 RESP2 Redis クライアント経由で PoW ナンス、固定ウィンドウのレート制限、SSH/Minecraft 検証チケット、IP クリアランスをコンテナ再起動や複数レプリカ間で共有します（`docker compose up -d --build`）。Docker から SSH や Minecraft を保護する場合は `-p 22:22` / `-p 25565:25565` と `-p 8443:8443` も公開してください。バインドマウントの権限設定（`chown 10001:10001`）、`--watch`、環境変数のみの構成については [Docker & Docker Compose ガイド](docs/ja/guide/docker.md) を参照してください。

### 環境変数（`GYMBAG_*`）

優先順位は **設定ファイル -> `.env` / `GYMBAG_*` 環境変数 -> CLI フラグ** です。

| 変数 | 説明 |
| :--- | :--- |
| `GYMBAG_CONFIG` | `gymbag.json`、`Gymbagfile`、`Caddyfile`、`nginx.conf`、または Apache 設定ファイルのパス |
| `GYMBAG_BIND` | バインド IP（ローカル HTTP のみなら `127.0.0.1`、公開ドメイン・SSH・Minecraft が有効なら `0.0.0.0`） |
| `GYMBAG_HTTP_PORT` / `GYMBAG_HTTPS_PORT` | HTTP と HTTPS の待受ポート（デフォルト `80` / `443`） |
| `GYMBAG_DOMAIN` / `GYMBAG_DOMAINS` | カンマ区切りの下流ドメイン |
| `GYMBAG_UPSTREAM` | デフォルトのアップストリーム URL |
| `GYMBAG_UPSTREAMS` | セミコロン区切りのホストマップ: `app.example.com=http://web:3000;api.example.com=http://api:8080` |
| `GYMBAG_PROTECT` / `GYMBAG_PROTECT_DOMAINS` | WAF 保護の有効/無効（`on`/`off`）、または指定ドメインのみ保護 |
| `GYMBAG_VERIFY_LEVEL` | 検証レベル `0`–`3` または `off`（別名: `GYMBAG_VERIFY`, `GYMBAG_VERIFY_MODE`, `GYMBAG_MODE`, `GYMBAG_TRIGGER`） |
| `GYMBAG_DEFAULT_RULES` | 組み込みベースラインルール（`on` / `off`、デフォルト `on`） |
| `GYMBAG_REMOTE_RULES` / `GYMBAG_REMOTE_RULES_REFRESH` | リモートファイアウォールルール URL と更新間隔（デフォルト `300`s / `5m`） |
| `GYMBAG_RULES_WATCH_INTERVAL` | `dynamic` ローカルルールファイル/フォルダのポーリング間隔（デフォルト `2`s / `2s`） |
| `GYMBAG_DIFFICULTY` | SHA-256 PoW 難易度（`1`–`5`、デフォルト `4`） |
| `GYMBAG_CHALLENGE_TTL` / `GYMBAG_CLEARANCE_TTL` | チャレンジ有効期限（`300`s / `5m`）とクリアランス有効期限（`86400`s / `24h`；別名: `GYMBAG_VERIFY_TIMEOUT`, `GYMBAG_VERIFIED_TIMEOUT`） |
| `GYMBAG_COOKIE_NAME` / `GYMBAG_COOKIE_SECURE` | クリアランス Cookie 名（`waf_clearance`）と `Secure` フラグ（`true`） |
| `GYMBAG_RATE_LIMIT` / `GYMBAG_RATE_WINDOW` | IP あたりのリクエスト上限（`240`）とウィンドウ（`60`s） |
| `GYMBAG_TLS_MODE` / `GYMBAG_SSL_PROVIDER` | TLS モード（`auto`, `acme`, `local`, `selfsigned`, `file`）と CA（`letsencrypt`, `letsencrypt_staging`, `zerossl`, `google`, `ssl.com`, `litessl`, `buypass`, `local`, または URL） |
| `GYMBAG_ACME_EMAIL` / `GYMBAG_ACME_DIRECTORY` | ACME 連絡先メール（ZeroSSL 自動 EAB 兼用）と明示的 ACME ディレクトリ URL |
| `GYMBAG_EAB_KID` / `GYMBAG_EAB_HMAC_KEY` | EAB Key ID と Base64/Base64URL HMAC 鍵 |
| `GYMBAG_ACCOUNT_KEY_FILE` | ECDSA/RSA ACME アカウント鍵 PEM のパス |
| `GYMBAG_CERT_FILE` / `GYMBAG_KEY_FILE` / `GYMBAG_KEY_PASSWORD` | `file` モード用の証明書・秘密鍵・暗号化鍵パスフレーズ |
| `GYMBAG_CERT_DIR` | 証明書・CA 鍵・シークレット・インテルキャッシュ・ルールキャッシュの保存ディレクトリ（デフォルト `certs`） |
| `GYMBAG_RENEW_INTERVAL` / `GYMBAG_RENEW_WITHIN_DAYS` | 更新チェック間隔（`43200`s）と更新しきい値（`30` 日） |
| `GYMBAG_AUTO_HOSTS` / `GYMBAG_HSTS` | `/etc/hosts` 自動管理（`on`/`off`）と `Strict-Transport-Security` 上書き（`on`/`off`） |
| `GYMBAG_IP_SECURITY` / `GYMBAG_IP_SECURITY_ACTION` | 脅威フィード（`on`/`off`）と一致時アクション（`verify` または `block`） |
| `GYMBAG_IP_REPUTATION` / `GYMBAG_AI_IP_LIST` | Shodan InternetDB + StopForumSpam ライブ照会（`on`/`off`）と公式 AI クローラー CIDR リスト（`on`/`off`） |
| `GYMBAG_VPN_DETECTION` | 不審判定での無料 VPN/プロキシ/Tor 検出（`on`/`off`、デフォルト `on`） |
| `GYMBAG_INTEL_REFRESH` | 脅威・VPN・AI フィードの更新間隔（デフォルト `21600`s / `6h`） |
| `GYMBAG_VERIFY_URL` / `GYMBAG_VERIFY_PORT` | 共有 HTTPS 検証ポータル URL（空欄 = サーバー IP + IP 証明書）とポート（デフォルト `8443`） |
| `GYMBAG_SSH_ENABLED` / `GYMBAG_SSH_PORT` / `GYMBAG_SSH_UPSTREAM` | SSH 保護の有効化（デフォルトポート `22`）とバックエンド `sshd` 宛先 |
| `GYMBAG_SSH_VERIFY_LEVEL` / `GYMBAG_SSH_PROTECT` | SSH 検証レベル（`0`–`3` または `off`）と保護の有効/無効（`on`/`off`） |
| `GYMBAG_MC_ENABLED` / `GYMBAG_MC_PORT` / `GYMBAG_MC_UPSTREAM` | Minecraft 保護の有効化（デフォルトポート `25565`）とバックエンド宛先（`GYMBAG_MINECRAFT_*` 別名あり） |
| `GYMBAG_MC_SERVER_JAR` / `GYMBAG_MC_JAVA` | Minecraft `server.jar` のパス（自動選択した Java でサーバーを自動起動）と任意の Java バイナリ上書き |
| `GYMBAG_MC_JVM_ARGS` / `GYMBAG_MC_WORK_DIR` / `GYMBAG_MC_AUTO_RESTART` | スペース区切りの JVM フラグ、Minecraft 作業ディレクトリ、クラッシュ時自動再起動の有効/無効（`true`/`false`、デフォルト `true`） |
| `GYMBAG_MC_VERIFY_LEVEL` / `GYMBAG_MC_PROTECT` | Minecraft 検証レベル（`0`–`3` または `off`）と保護の有効/無効（`on`/`off`） |
| `GYMBAG_REDIS_URL` / `GYMBAG_SECRET` / `GYMBAG_ADMIN` | Redis URL（`redis://` または `rediss://`）、HMAC シークレット、Admin API 待受アドレス（`127.0.0.1:2019` や `off`） |

### REST API

**公開 WAF・検証ポータルエンドポイント:**

| メソッド | パス | 説明 |
| :--- | :--- | :--- |
| `GET` | `/.waf/v1/session` | クリアランス状態 (`cleared`)、`protected`、`verify_level`、`verify_mode`、`clearance_ttl`、`expires_at`、`expires_in` |
| `DELETE` | `/.waf/v1/session` | 訪問者のクリアランス Cookie を削除（`204 No Content`） |
| `POST` | `/.waf/v1/challenges` | 使い捨て SHA-256 PoW チャレンジを発行（`201 Created`） |
| `POST` | `/.waf/v1/verifications` | PoW 解を検証し署名 Cookie を設定（`/.waf/verify` 別名対応） |
| `GET` | `/v/{token}` | SSH/Minecraft 検証用の PoW ページを配信（`/verify/{token}` 別名対応） |
| `POST` | `/.waf/gate/verify` | SSH/Minecraft PoW 解を検証し IP クリアランスを保存 |
| `GET` | `/.waf/assets/{name}` | 埋め込みチャレンジ PNG アセットを配信（`/.waf/v1/assets/{name}` 別名、`ETag` キャッシュ付き） |

**ローカル Admin API (`admin 127.0.0.1:2019` または `--admin`):**

Admin API は認証トークンなしで稼働中のルーティングやファイアウォール状態を変更するため、必ずループバック（`127.0.0.1:2019`）かプライベートネットワークにバインドしてください。

| メソッド | パス | 説明 |
| :--- | :--- | :--- |
| `GET` | `/api/v1/health` | ステータス、`sites_count`、`ssh_enabled`、`minecraft_enabled`、`tls_source`、`threat_entries`、`ai_entries`、`vpn_entries`、`remote_rules` |
| `GET` / `POST` | `/api/v1/tls` / `/api/v1/tls/renew` | 有効な TLS 状態と CA 一覧／証明書の即時更新とアトミック差し替え |
| `GET` / `POST` | `/api/v1/intel` / `/api/v1/intel/refresh` | フィード別 threat/AI/VPN 件数とキャッシュサイズ／即時更新実行 |
| `GET` / `POST` | `/api/v1/rules/remote` / `/api/v1/rules/remote/refresh` | リモートルール件数と取得エラー／即時更新と WAF リロード |
| `GET` / `PUT` | `/api/v1/config` | マスク済み設定の出力／検証して稼働中ランタイム設定を置換 |
| `GET` / `POST` | `/api/v1/sites` | サイト一覧／実行時サイト追加 |
| `GET` / `PUT` / `PATCH` / `DELETE` | `/api/v1/sites/{domain}` | 取得・置換・ライブパッチ（`verify_level`、`protect`、`difficulty`、`clearance_ttl`、`upstreams`、`routes`、`headers`、`rules`、`remote_rules`、`rule_sources`）・削除 |
| `GET` / `POST` / `PUT` / `DELETE` | `/api/v1/sites/{domain}/rules` & `.../{index_or_name}` | サイトのカスタムルール一覧・末尾追加・置換・削除 |

## Caddy・Nginx とのベンチマーク比較

Apple M4（10 コア、macOS、Go `1.27.0`）環境、同一 ECDSA P-256 証明書による TLS 1.3、64 並列ワーカー、`1 KiB` レスポンスボディ、アップストリーム keep-alive `1024` 接続、エラー `0` で計測しました。

| サーバー＆モード | HTTP/1.1 TLS (RPS) | H1 p50 / p99 | HTTP/2 TLS (RPS) | H2 p50 / p99 | バイナリサイズ | 負荷時 RSS |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **85,203** | 0.45 / 6.20 ms | **88,775** | 0.65 / 1.96 ms | 1.5 MB (+libs) | 5.8 MiB |
| **Gymbag `v0.1.0`** (`verify 3`, フル WAF 有効) | **72,517** | 0.74 / 3.14 ms | **66,096** | 0.82 / 3.48 ms | 12.9 MB (静的) | 54.3 MiB |
| **Gymbag `v0.1.0`** (`protect off`, 純プロキシ) | **72,479** | 0.74 / 3.15 ms | **64,048** | 0.83 / 3.94 ms | 12.9 MB (静的) | 52.1 MiB |
| **Gymbag `v0.1.0`** (`verify 0`, Cookie 認証済) | **65,490** | 0.79 / 3.93 ms | **62,108** | 0.84 / 4.27 ms | 12.9 MB (静的) | ~54 MiB |
| **Caddy `v2.11.9`** (Go, keep-alive 調整済) | **54,741** | 0.90 / 4.90 ms | **39,207** | 1.30 / 6.18 ms | 49.0 MB | 76.8 MiB |

生のスループットでは C 製の Nginx が上回りますが、Gymbag は WAF をインラインで動かしながら HTTP/1.1 の p99 レイテンシで Nginx を下回り（`3.14 ms` vs `6.20 ms`）、Caddy に対しては HTTP/1.1 で `+32%`、HTTP/2 で `+63%`〜`+69%` 高速、メモリは約 30% 少なく、バイナリサイズは 3.8 倍小さくなっています。`/.env` のような既知の攻撃プローブは TLS 終端エッジで **140,680 req/s**（p50 `0.35 ms`）で落とすため、アップストリームへ届きません。

計測手法、`go test -bench` マイクロベンチマーク、検証に使用した設定ファイルは [ベンチマークとパフォーマンス](docs/ja/guide/benchmarks.md) にまとめています。

## プロジェクト構成

```text
.
├── cmd/gymbag/          # CLI エントリポイント
├── docs/                # 多言語 VitePress ドキュメント（EN, 简体中文, 日本語, 한국어, Español）
├── internal/
│   ├── cli/             # サブコマンド (init, config, ruleset, service, run, reverse-proxy, validate, fmt, version)、.env ＆ --watch
│   ├── config/          # 設定モデル、Gymbagfile/Caddyfile v2/nginx/Apache パーサー、期間＆ルールソース、GYMBAG_* ローダー
│   ├── host/            # ドメイン正規化、IPv6 ブラケット処理、ワイルドカード一致
│   ├── intel/           # O(1) プレフィックスセット、自動更新脅威/VPN/AI フィード、ライブ IP＆VPN レピュテーション
│   ├── localpki/        # 2 階層ローカル CA ("Neuro-sama Headquarters Root CA") ＆ /etc/hosts マネージャー
│   ├── proxy/           # ロックフリーマルチサイト＆パスルーター、プーリング HTTP ＆ RFC 6455 WebSocket プロキシ
│   ├── server/          # 統合 HTTPS ゲートウェイ、HTTP->HTTPS/ACME リダイレクター、Admin REST API
│   ├── service/         # システムサービスレンダラー＆マネージャー (systemd, launchd, openrc, supervisor)
│   ├── setup/           # sshd_config / ファイアウォール自動設定と Minecraft server.properties ポートランダム化
│   ├── store/           # 64 シャードインメモリストア、純 Go RESP2 Redis クライアント (チャレンジ、レート制限、ゲートチケット/クリアランス)
│   ├── tcpguard/        # SSH ＆ Minecraft Java TCP 保護プロキシ、ポリシーエンジン、共有 HTTPS 検証ポータル
│   ├── tlsmanager/      # マルチ CA ACME v2 (RFC 8738 IP 証明書、EAB ＆ ZeroSSL 自動 EAB)、ローカル PKI、アトミック SNI 証明書管理
│   └── waf/             # PoW 検証、シャード化 HMAC クリアランスキャッシュ、Aho-Corasick DFA、リモート/ローカルルール、埋め込み UI
├── nix/                 # NixOS モジュール (services.gymbag)
├── packaging/           # Arch Linux PKGBUILD ＆ Debian/Ubuntu .deb メンテナスクリプト/ユニット
├── Dockerfile
├── compose.yaml
├── flake.nix
├── Gymbagfile
└── go.mod
```

## Star 履歴

<a href="https://www.star-history.com/#Evil-Project/gymbag&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
 </picture>
</a>
