# CLI & `GYMBAG_*` 環境変数 リファレンス

設定の優先順位は以下の通りです：

**設定ファイル → `.env` / `GYMBAG_*` 環境変数 → CLI フラグ**

カレントディレクトリに `.env` がある場合は自動で読み込まれます（既存の OS 環境変数は上書きしません）。

## CLI サブコマンド

```bash
# 設定を生成（対話ウィザードまたはフラグ指定；既定では gymbag.json を出力）
gymbag init [flags] [output_path]
gymbag init --format gymbagfile -o Gymbagfile -d app.example.com -u :3000

# ./gymbag.json、./Gymbagfile、./Caddyfile の順で自動読み込みしてサーバーを起動
gymbag

# 設定ファイル指定＆変更監視ホットリロード（2秒ごとにポーリング）
gymbag run --config Gymbagfile --watch

# OS システムサービス管理（systemd, launchd, openrc, supervisor）
gymbag service <install|start|stop|restart|status|uninstall|render> [flags]

# gymbag.json または Gymbagfile のサイト・ルート・グローバル設定を管理
gymbag config <list|show|add|update|remove|route|set|unset|convert> [flags]

# ルールファイル・ルールフォルダ・インラインルールを管理
gymbag ruleset <list|add|update|remove|validate|init|rule> [flags]

# 設定ファイルなしのアドホック リバースプロキシ
gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3 --verify-timeout 12h

# 設定ファイルを検証し解決されたサイト別ポリシーを表示
gymbag validate --config Gymbagfile

# Gymbagfile を上書き整形
gymbag fmt --overwrite Gymbagfile

# バージョンを表示
gymbag version
```

### サーバー＆共通フラグ (`run` / 既定 / `reverse-proxy`)

| フラグ | 説明 |
| :--- | :--- |
| `-c`, `--config <path>` | `gymbag.json`、`Gymbagfile`、`Caddyfile`、ネイティブ `nginx.conf`、または Apache `httpd.conf` / `apache2.conf` のパス |
| `-d`, `--domain <list>` | 下流ドメイン（カンマ区切り；`reverse-proxy` では `--from`） |
| `-u`, `--upstream <url>` | アップストリーム URL（`http://localhost:3000` または `:3000`；`reverse-proxy` では `--to`） |
| `-l`, `--verify <0-3\|off>` | 検証レベル（`0`、`1`、`2`、`3`、または `off`） |
| `--verify-timeout <dur>` | 認証済みクリアランスのタイムアウト（`30m`, `12h`, `7d` または秒数；別名 `--clearance-ttl`） |
| `--remote-rules <url>` | リモートファイアウォールルール URL（複数回またはカンマ区切り） |
| `--remote-rules-refresh <dur>` | リモートルール更新間隔（既定 `5m` / `300s`） |
| `--no-protect` | WAF 検証を無効化 |
| `--ssl-provider <name>` | ACME / SSL プロバイダーを上書き |
| `--admin <addr>` | ローカル REST 管理 API を有効化（例：`127.0.0.1:2019` または `off`） |
| `--http-port <port>` | HTTP リダイレクト＆ACME ポート（既定 `80`） |
| `--https-port <port>` | HTTPS ゲートウェイポート（既定 `443`） |
| `--watch` | 設定ファイルの変更を監視して自動ホットリロード（`run` のみ） |

---

## 設定管理 (`gymbag config`)

すべての `config` サブコマンドは `-c`, `--config <path>` を受け付けます（既定：自動検出された設定ファイル）。

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
| `list` / `show` | サイト一覧を表示 / 整形済み設定を出力 |
| `add` | サイトを追加：`-d`、`-u`、`-l`、`--difficulty`、`--verify-timeout`、`--no-protect`、`--no-default-rules`、`--cert`、`--key`、`--root`、`--browse`、`--fallback`、`--respond`、`--status`、`--redir`、`--code`、`--header Name=Value\|-Name`、`-f` |
| `update` | サイトを更新：`-d`、`-u`、`-l`、`--difficulty`、`--verify-timeout`、`--protect on\|off`、`--default-rules on\|off`、`--cert`、`--key`、`--clear-tls`、`--header`、`--remove-header` |
| `remove` | サイトを削除（`-d`） |
| `route list\|add\|update\|remove` | パスルートを管理：`-d`、`--path`、`--strip-prefix`、`-u`、`--root`、`--browse`、`--fallback`、`--respond`、`--status`、`--redir`、`--code`、`--header`、`--index`（`update` / `remove` 用の 0 始まりルートインデックス） |
| `set <key> <value>` / `unset <key>` | グローバルオプションを変更またはリセット（`email`、`ssl_provider`、`admin`、`http_port`、`https_port`、`rules_watch_interval`、`remote_rules_refresh` など） |
| `convert` | nginx、Apache、Caddyfile、Gymbagfile、または JSON を変換：`-c <in>`、`-o <out>`（省略時は標準出力）、`--format json\|gymbagfile`（既定 `json`）、`-f` |

## ルールセット管理 (`gymbag ruleset`)

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
| `list` | 紐付けられたルールソースを一覧表示（`-c`、`-d`） |
| `add` | ルールファイル・フォルダ・URL を紐付け：`--source`、`--name`、`--dynamic` / `--static`、`-d <domain>` または `--global`、`-c` |
| `update` | 紐付け済みルールセットを変更：`--target`（または位置引数のソース/名前）、`--source`、`--name`、`--dynamic` / `--static`、`-d` / `--global`、`-c` |
| `remove` | ルールセットの紐付けを解除：`--source <source-or-name>`、`-d` / `--global`、`-c` |
| `validate <path>` | ルールファイルまたはルールディレクトリを検証 |
| `init <path>` | スタータールールセットを作成（`-o`、`--format rules\|json`、`-f`） |
| `rule list\|add\|update\|remove` | `--file` 内（または `-c` / `-d` 経由で設定ファイル内のインライン）単体ルールを管理：`--name`、`--index`、`--action allow\|verify\|block`、`--ip`、`--ua`、`--path`、`--query`、`--method`、`--header`、`--cookie` |

---

## 環境変数 (`GYMBAG_*`)

| 変数名 | 説明 |
| :--- | :--- |
| `GYMBAG_CONFIG` | `gymbag.json`、`Gymbagfile`、`Caddyfile`、`nginx.conf`、または Apache `httpd.conf` のパス |
| `GYMBAG_BIND` | バインドアドレス（ローカルは `127.0.0.1`、公開ドメインありは `0.0.0.0`） |
| `GYMBAG_HTTP_PORT` / `GYMBAG_HTTPS_PORT` | HTTP / HTTPS 待ち受けポート（既定 `80` / `443`） |
| `GYMBAG_DOMAIN` / `GYMBAG_DOMAINS` | カンマ区切りの下流ドメイン |
| `GYMBAG_UPSTREAM` | 既定のアップストリーム URL |
| `GYMBAG_UPSTREAMS` | セミコロン区切りのホストマップ：`app.example.com=http://web:3000;api.example.com=http://api:8080` |
| `GYMBAG_PROTECT` | 全サイト WAF 保護のオン/オフ（`on` / `off`） |
| `GYMBAG_PROTECT_DOMAINS` | WAF 保護を有効にするドメインのカンマ区切り一覧 |
| `GYMBAG_VERIFY_LEVEL` | 検証レベル：`0`、`1`、`2`、`3`（別名：`GYMBAG_VERIFY`, `GYMBAG_VERIFY_MODE`, `GYMBAG_MODE`, `GYMBAG_TRIGGER`） |
| `GYMBAG_DEFAULT_RULES` | 組み込みベースラインルールの有効/無効（`on` / `off`） |
| `GYMBAG_REMOTE_RULES` | カンマまたは空白区切りのリモートファイアウォールルール URL |
| `GYMBAG_RULES_WATCH_INTERVAL` | 動的ローカルルールファイル/フォルダのポーリング間隔（秒または期間；既定 `2` / `2s`） |
| `GYMBAG_REMOTE_RULES_REFRESH` | リモートルールの更新間隔（秒または期間；既定 `300` / `5m`） |
| `GYMBAG_DIFFICULTY` | SHA-256 PoW 難易度（`1`–`5`、既定 `4`） |
| `GYMBAG_CHALLENGE_TTL` | チャレンジ有効期限（秒または期間；既定 `300` / `5m`） |
| `GYMBAG_CLEARANCE_TTL` | 認証済みステータス有効期限（秒または `30m`, `12h`, `7d`；既定 `86400` / `24h`；別名：`GYMBAG_VERIFY_TIMEOUT`, `GYMBAG_VERIFIED_TIMEOUT`） |
| `GYMBAG_COOKIE_NAME` / `GYMBAG_COOKIE_SECURE` | Cookie 名（`waf_clearance`）と `Secure` 属性（`true`） |
| `GYMBAG_RATE_LIMIT` / `GYMBAG_RATE_WINDOW` | IP 別上限（`240`）とウィンドウ（`60` 秒 / `1m`） |
| `GYMBAG_TLS_MODE` | `auto`、`acme`、`local`、`selfsigned`、`file` |
| `GYMBAG_SSL_PROVIDER` | `letsencrypt`、`letsencrypt_staging`、`zerossl`、`google`、`ssl.com`、`litessl`、`buypass`、`local`、URL |
| `GYMBAG_ACME_EMAIL` | ACME 連絡先メール（ZeroSSL 自動 EAB 兼用） |
| `GYMBAG_ACME_DIRECTORY` | 明示的な RFC 8555 ACME v2 ディレクトリ URL |
| `GYMBAG_EAB_KID` / `GYMBAG_EAB_HMAC_KEY` | EAB Key ID と HMAC キー |
| `GYMBAG_ACCOUNT_KEY_FILE` | ACME アカウント秘密鍵 PEM パス |
| `GYMBAG_CERT_FILE` / `GYMBAG_KEY_FILE` | `file` モード用 TLS 証明書＆秘密鍵パス |
| `GYMBAG_KEY_PASSWORD` | 暗号化 PEM 秘密鍵のパスフレーズ |
| `GYMBAG_CERT_DIR` | CA 証明書・ACME 状態・インテル/リモートルールキャッシュ保存先（既定 `certs`） |
| `GYMBAG_RENEW_INTERVAL` / `GYMBAG_RENEW_WITHIN_DAYS` | 更新チェック間隔（`43200`s / `12h`）と更新閾値（`30` 日） |
| `GYMBAG_AUTO_HOSTS` | ローカルドメイン用 `/etc/hosts` 自動管理（`on` / `off`） |
| `GYMBAG_HSTS` | `Strict-Transport-Security` ヘッダー上書き（`on` / `off`） |
| `GYMBAG_IP_SECURITY` / `GYMBAG_IP_SECURITY_ACTION` | 脅威フィード切替（`on`/`off`）とアクション（`verify` / `block`） |
| `GYMBAG_IP_REPUTATION` | Shodan InternetDB + StopForumSpam 照会切替（`on`/`off`） |
| `GYMBAG_AI_IP_LIST` | 公式 AI クローラー IP リスト切替（`on`/`off`） |
| `GYMBAG_INTEL_REFRESH` | 脅威＆AI フィード更新間隔（秒または期間；既定 `21600` / `6h`） |
| `GYMBAG_REDIS_URL` | Redis 接続 URL（`redis://[:password@]host:port/db`） |
| `GYMBAG_SECRET` | クリアランス Cookie 用 HMAC シークレット |
| `GYMBAG_ADMIN` | ローカル Admin REST API 待ち受けアドレス（例：`127.0.0.1:2019` または `off`） |
