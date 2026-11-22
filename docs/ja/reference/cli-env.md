# CLI & `GYMBAG_*` 環境変数

設定の優先順位は次の通りです。

**設定ファイル -> `.env` / `GYMBAG_*` 環境変数 -> CLI フラグ**

作業ディレクトリに `.env` がある場合、既存の環境変数を上書きせずに読み込みます。

## CLI サブコマンド

```bash
# 設定を生成（対話ウィザードまたはフラグ；デフォルトは gymbag.json）
gymbag init [flags] [output_path]
gymbag init --format gymbagfile -o Gymbagfile -d app.example.com -u :3000

# ./gymbag.json、./Gymbagfile、./Caddyfile を使って起動
gymbag

# 明示的に設定を指定し、編集時にホットリロードして起動（2秒間隔でポーリング）
gymbag run --config Gymbagfile --watch

# OS サービスを管理 (systemd, launchd, openrc, supervisor)
gymbag service <install|start|stop|restart|status|uninstall|render> [flags]

# サイト、ルート、グローバル設定、設定変換を管理
gymbag config <list|show|add|update|remove|site|route|set|unset|convert> [flags]

# ルールファイル、フォルダ、インラインルールを管理
gymbag ruleset <list|add|update|remove|validate|init|rule> [flags]

# 設定ファイルなしの単発 HTTP/HTTPS リバースプロキシ
gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3 --verify-timeout 12h

# 設定を検証し解決済みのサービスサマリーを出力（`server_jar` 用に選択された Java も表示）
gymbag validate --config Gymbagfile

# Gymbagfile を上書き整形（4スペースインデント）
gymbag fmt --overwrite Gymbagfile

# バージョンを表示
gymbag version
```

### サーバーフラグ（`run` / デフォルト / `reverse-proxy`）

| フラグ | 説明 |
| :--- | :--- |
| `-c`, `--config <path>` | `gymbag.json`、`Gymbagfile`、`Caddyfile`、`nginx.conf`、または Apache 設定ファイルのパス |
| `-d`, `--domain <list>` | 下流ドメインをカンマ区切りで上書き（`reverse-proxy` では `--from`） |
| `-u`, `--upstream <url>` | アップストリーム URL を上書き（`http://localhost:3000` または `:3000`；`reverse-proxy` では `--to`） |
| `-l`, `--verify <0-3\|off>` | 検証レベルを上書き（`0`、`1`、`2`、`3`、または `off`；別名 `--verify-level`, `-m`, `--mode`） |
| `--verify-timeout <dur>` | クリアランス有効期限を上書き（`30m`、`12h`、`7d`、または秒数；別名 `--clearance-ttl`） |
| `--remote-rules <url>` | リモートファイアウォールルール URL を指定（繰り返し・カンマ区切り可） |
| `--remote-rules-refresh <dur>` | リモートルールの更新間隔（デフォルト `5m` / `300s`） |
| `--no-protect` | CLI で指定したサイトの PoW 検証を無効化 |
| `--ssl-provider <name>` | ACME / SSL プロバイダを上書き |
| `--admin <addr>` | ローカル REST Admin API を有効化（`127.0.0.1:2019` または `off`） |
| `--http-port <port>` | HTTP リダイレクト＆ACME HTTP-01 ポート（デフォルト `80`） |
| `--https-port <port>` | HTTPS ゲートウェイポート（デフォルト `443`） |
| `--watch` | 設定ファイルの変更を監視してホットリロード（`run` / デフォルトのみ） |

## 設定管理（`gymbag config`）

すべての `config` サブコマンドは `-c`、`--config <path>` を受け付けます（省略時は自動検出された設定ファイル）。

```bash
gymbag config list
gymbag config add -d api.example.com -u :8080 -l 1
gymbag config update -d api.example.com -l 3 --verify-timeout 6h
gymbag config remove -d api.example.com
gymbag config set admin 127.0.0.1:2019
gymbag config unset admin
gymbag config route add -d app.example.com --path '/static/*' --strip-prefix /static --root ./dist --browse
gymbag config route list -d app.example.com
gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```

| サブコマンド | 説明 |
| :--- | :--- |
| `list` / `show` | サイト＆TCP サービス一覧を表示 / 整形済み設定を出力（`--format json\|gymbagfile`） |
| `add` | サイトを追加: `-d`, `-u`, `-l`, `--difficulty`, `--verify-timeout`, `--no-protect`, `--no-default-rules`, `--cert`, `--key`, `--root`, `--browse`, `--fallback`, `--respond`, `--status`, `--redir`, `--code`, `--header Name=Value\|-Name`, `-f` |
| `update` | サイトを更新: `-d`, `-u`, `-l`, `--difficulty`, `--verify-timeout`, `--protect on\|off`, `--default-rules on\|off`, `--cert`, `--key`, `--clear-tls`, `--header`, `--remove-header` |
| `remove` | サイトを削除（`-d`） |
| `route list\|add\|update\|remove` | パスルートを管理: `-d`, `--path`, `--strip-prefix`, `-u`, `--root`, `--browse`, `--fallback`, `--respond`, `--status`, `--redir`, `--code`, `--header`, `--index` |
| `set <key> <value>` / `unset <key>` | グローバルキーを設定またはリセット（`email`, `ssl_provider`, `http_port`, `https_port`, `bind`, `admin`, `cert_dir`, `redis`, `ip_security`, `ip_security_action`, `ip_reputation`, `ai_ip_list`, `intel_refresh`, `remote_rules_refresh`, `rules_watch_interval`, `verify_timeout`, `auto_hosts`, `hsts`, `eab_kid`, `eab_hmac_key`） |
| `convert` | nginx、Apache、Caddyfile、Gymbagfile、または JSON を変換: `-c <in>`, `-o <out>`, `--format json\|gymbagfile`, `-f` |

## ルールセット管理（`gymbag ruleset`）

```bash
gymbag ruleset init ./rules/custom.rules
gymbag ruleset add --source ./rules --dynamic --name local-dir
gymbag ruleset update local-dir --static
gymbag ruleset remove local-dir
gymbag ruleset validate ./rules
gymbag ruleset rule add --file ./rules/custom.rules --action block --name bad-bot --ua "*BadBot*"
```

| サブコマンド | 説明 |
| :--- | :--- |
| `list` | 紐付けられたルールソースを一覧表示（`-c`, `-d`） |
| `add` | ルールファイル・フォルダ・URL を紐付け: `--source`, `--name`, `--dynamic` / `--static`, `-d <domain>` または `--global`, `-c` |
| `update` | 紐付け済みルールセットを変更: `--target`（または位置引数のソース/名前）, `--source`, `--name`, `--dynamic` / `--static`, `-d` / `--global`, `-c` |
| `remove` | ルールセットの紐付けを解除: `--source <source-or-name>`, `-d` / `--global`, `-c` |
| `validate <path>` | ルールファイルまたはルールディレクトリを検証 |
| `init <path>` | スタータールールセットを作成（`-o`, `--format rules\|json`, `-f`） |
| `rule list\|add\|update\|remove` | `--file` 内（または `-c` / `-d` 経由で設定ファイル内のインライン）の単体ルールを管理: `--name`, `--index`, `--action allow\|verify\|block`, `--ip`, `--ua`, `--path`, `--query`, `--method`, `--header`, `--cookie` |

## 環境変数（`GYMBAG_*`）

| 変数 | 説明 |
| :--- | :--- |
| `GYMBAG_CONFIG` | `gymbag.json`、`Gymbagfile`、`Caddyfile`、`nginx.conf`、または Apache 設定ファイルのパス |
| `GYMBAG_BIND` | バインドアドレス（ローカル HTTP のみなら `127.0.0.1`、公開ドメイン・SSH・Minecraft が有効なら `0.0.0.0`） |
| `GYMBAG_HTTP_PORT` / `GYMBAG_HTTPS_PORT` | HTTP と HTTPS の待受ポート（デフォルト `80` と `443`） |
| `GYMBAG_DOMAIN` / `GYMBAG_DOMAINS` | カンマ区切りの下流ドメイン |
| `GYMBAG_UPSTREAM` | デフォルトのアップストリーム URL |
| `GYMBAG_UPSTREAMS` | セミコロン区切りのホストマップ: `app.example.com=http://web:3000;api.example.com=http://api:8080` |
| `GYMBAG_PROTECT` / `GYMBAG_PROTECT_DOMAINS` | 全サイトで WAF 保護を切替（`on`/`off`）、または保護対象を指定ドメインに限定 |
| `GYMBAG_VERIFY_LEVEL` | 検証レベル: `0`、`1`、`2`、`3`、または `off`（別名: `GYMBAG_VERIFY`, `GYMBAG_VERIFY_MODE`, `GYMBAG_MODE`, `GYMBAG_TRIGGER`） |
| `GYMBAG_DEFAULT_RULES` | 組み込みベースラインルール（`on` / `off`） |
| `GYMBAG_REMOTE_RULES` / `GYMBAG_REMOTE_RULES_REFRESH` | リモートルール URL と更新間隔（デフォルト `300`s / `5m`） |
| `GYMBAG_RULES_WATCH_INTERVAL` | `dynamic` ローカルルールソースのポーリング間隔（デフォルト `2`s / `2s`） |
| `GYMBAG_DIFFICULTY` | SHA-256 PoW 難易度（`1`–`5`、デフォルト `4`） |
| `GYMBAG_CHALLENGE_TTL` / `GYMBAG_CLEARANCE_TTL` | チャレンジ有効期限（`300`s / `5m`）とクリアランス有効期間（`86400`s / `24h`；別名: `GYMBAG_VERIFY_TIMEOUT`, `GYMBAG_VERIFIED_TIMEOUT`） |
| `GYMBAG_COOKIE_NAME` / `GYMBAG_COOKIE_SECURE` | クリアランス Cookie 名（`waf_clearance`）と `Secure` フラグ（`true`） |
| `GYMBAG_RATE_LIMIT` / `GYMBAG_RATE_WINDOW` | IP あたりのリクエスト上限（`240`）とウィンドウ（`60`s / `1m`） |
| `GYMBAG_TLS_MODE` / `GYMBAG_SSL_PROVIDER` | TLS モード（`auto`, `acme`, `local`, `selfsigned`, `file`）と CA プロバイダまたは URL |
| `GYMBAG_ACME_EMAIL` / `GYMBAG_ACME_DIRECTORY` | ACME 連絡先メール（ZeroSSL 自動 EAB 兼用）と明示的な ACME ディレクトリ URL |
| `GYMBAG_EAB_KID` / `GYMBAG_EAB_HMAC_KEY` | External Account Binding Key ID と HMAC 鍵 |
| `GYMBAG_ACCOUNT_KEY_FILE` | ECDSA または RSA ACME アカウント鍵 PEM のパス |
| `GYMBAG_CERT_FILE` / `GYMBAG_KEY_FILE` / `GYMBAG_KEY_PASSWORD` | `file` モード用の証明書、秘密鍵、秘密鍵パスフレーズ |
| `GYMBAG_CERT_DIR` | 証明書、CA 鍵、シークレット、インテルキャッシュ、ルールキャッシュのディレクトリ（デフォルト `certs`） |
| `GYMBAG_RENEW_INTERVAL` / `GYMBAG_RENEW_WITHIN_DAYS` | 更新チェック間隔（`43200`s / `12h`）と更新しきい値（`30` 日） |
| `GYMBAG_AUTO_HOSTS` / `GYMBAG_HSTS` | `/etc/hosts` 自動管理（`on`/`off`）と `Strict-Transport-Security` 上書き（`on`/`off`） |
| `GYMBAG_IP_SECURITY` / `GYMBAG_IP_SECURITY_ACTION` | 脅威フィードの切替（`on`/`off`）とアクション（`verify` または `block`） |
| `GYMBAG_IP_REPUTATION` / `GYMBAG_AI_IP_LIST` | Shodan InternetDB + StopForumSpam 照会（`on`/`off`）と公式 AI クローラーリスト（`on`/`off`） |
| `GYMBAG_VPN_DETECTION` | 不審判定での無料 VPN/プロキシ/Tor 検出を切替（`on`/`off`、デフォルト `on`） |
| `GYMBAG_INTEL_REFRESH` | 脅威・VPN・AI フィードの更新間隔（デフォルト `21600`s / `6h`） |
| `GYMBAG_VERIFY_URL` / `GYMBAG_VERIFY_PORT` | 共有 HTTPS 検証ポータルの URL（空欄 = サーバー IP + IP 証明書）とポート（デフォルト `8443`） |
| `GYMBAG_SSH_ENABLED` / `GYMBAG_SSH_PORT` / `GYMBAG_SSH_UPSTREAM` | SSH 保護を有効化（デフォルト `22`）してバックエンド `sshd` 宛先を設定 |
| `GYMBAG_SSH_VERIFY_LEVEL` / `GYMBAG_SSH_PROTECT` | SSH 検証レベル（`0`–`3` または `off`）と保護の切替（`on`/`off`） |
| `GYMBAG_MC_ENABLED` / `GYMBAG_MC_PORT` / `GYMBAG_MC_UPSTREAM` | Minecraft 保護を有効化（デフォルト `25565`）してバックエンド宛先を設定（`GYMBAG_MINECRAFT_*` 別名も可） |
| `GYMBAG_MC_SERVER_JAR` / `GYMBAG_MC_JAVA` | Minecraft `server.jar` のパス（自動選択した Java でサーバーを自動起動；別名 `GYMBAG_MC_JAR`）と Java バイナリ上書き（別名 `GYMBAG_MC_JAVA_PATH`） |
| `GYMBAG_MC_JVM_ARGS` / `GYMBAG_MC_WORK_DIR` / `GYMBAG_MC_AUTO_RESTART` | スペース区切りの JVM フラグ、Minecraft 作業ディレクトリ、クラッシュ時自動再起動の切替（`true`/`false`、デフォルト `true`） |
| `GYMBAG_MC_VERIFY_LEVEL` / `GYMBAG_MC_PROTECT` | Minecraft 検証レベル（`0`–`3` または `off`）と保護の切替（`on`/`off`） |
| `GYMBAG_REDIS_URL` | Redis URL（`redis://` または `rediss://`） |
| `GYMBAG_SECRET` / `GYMBAG_ADMIN` | HMAC クリアランスシークレットと Admin REST API 待受アドレス（`127.0.0.1:2019` または `off`） |
