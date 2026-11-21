# `Gymbagfile` & JSON スキーマ リファレンス

Gymbag は既定で作業ディレクトリの `./gymbag.json`、`./Gymbagfile`、または `./Caddyfile`（あるいは `--config <path>` / `GYMBAG_CONFIG`）を探索し、`gymbag.json`、`Gymbagfile` / `Caddyfile v2`、`nginx.conf`、Apache（`httpd.conf` / `apache2.conf`）をネイティブにパースします。

## グローバルオプションブロック `{ ... }`

先頭の `{ ... }` ブロックで TLS、ACME、ネットワーク、脅威インテリジェンス、リモートルール、全サイト共通の既定 WAF ポリシーを設定します：

```caddyfile
{
    email ops@example.com
    ssl_provider letsencrypt
    # eab YOUR_EAB_KID YOUR_EAB_HMAC_KEY
    # account_key certs/acme-account.pem
    # ssl_key certs/domain.key

    bind 0.0.0.0
    http_port 80
    https_port 443
    admin 127.0.0.1:2019
    cert_dir certs
    auto_hosts on
    hsts on

    # 認証済みクリアランスの既定タイムアウト（秒数または 30m, 12h, 7d）
    verify_timeout 24h

    # PoW ナンスおよびレート制限共有用 Redis（任意）
    # redis redis://127.0.0.1:6379/0

    # IP 脅威インテリジェンス & AI クローラー IP フィード
    ip_security on
    ip_security_action verify
    ip_reputation on
    ai_ip_list on
    intel_refresh 6h

    # 共有リモートファイアウォールルール（Gymbagfile 構文または JSON）
    # remote_rules https://rules.example.com/baseline.rules
    # remote_rules_refresh 5m

    # 全サイトに継承されるグローバルカスタムルール
    block ip 198.51.100.99 192.0.2.0/24
}
```

### グローバルディレクティブ

| ディレクティブ | 説明 | 既定値 |
| :--- | :--- | :--- |
| `email` / `acme_email` | ACME 登録連絡先メール（ZeroSSL 自動 EAB にも対応） | — |
| `ssl_provider` / `ca` | `letsencrypt`、`letsencrypt_staging`、`zerossl`、`google`、`ssl.com`、`litessl`、`buypass`、`local`、または URL | `auto` (`letsencrypt` / `local`) |
| `acme_directory` | 明示的な RFC 8555 ACME v2 ディレクトリ URL | Let's Encrypt v2 |
| `eab <kid> <hmac>` | EAB 認証情報（`eab_kid` / `eab_hmac_key` も可） | — |
| `account_key` | 既存の ECDSA / RSA ACME アカウント PEM パス | `<cert_dir>/acme-account.pem` |
| `ssl_key` | ACME CSR 用の既存秘密鍵パス | 自動生成 |
| `tls` | `auto`、`acme`、`local`、`selfsigned`、または `<cert.pem> <key.pem>` | `auto` |
| `key_password` | 暗号化 PEM 秘密鍵のパスフレーズ | — |
| `cert_dir` | 証明書・ローカル CA・シークレット・各種キャッシュの保存先 | `certs` |
| `renew_interval` | 証明書更新チェック間隔（秒または `12h` 等） | `43200` (`12h`) |
| `renew_within_days` | 有効期限の何日前に更新するか | `30` |
| `auto_hosts` | `localhost` / `*.internal` の `/etc/hosts` 自動管理 (`on`/`off`) | `on` |
| `hsts` | `Strict-Transport-Security` ヘッダー送信 (`on`/`off`) | 公開 ACME 証明書は `on` |
| `bind` / `bind_host` | 待ち受け IP アドレス | ローカル `127.0.0.1` / 公開 `0.0.0.0` |
| `http_port` / `https_port` | HTTP リダイレクト/ACME ポートと HTTPS ポート | `80` / `443` |
| `admin` | ローカル Admin REST API アドレス（例：`127.0.0.1:2019` または `off`） | 無効 |
| `redis` / `redis_url` | Redis 接続 URL（`redis://[:password@]host:port/db`） | インメモリ |
| `secret` | `waf_clearance` Cookie 用 HMAC シークレット | `<cert_dir>/secret` |
| `trusted_proxies` | `X-Forwarded-For` を信頼するプロキシ IP/CIDR | — |
| `ip_security` | 脅威インテル CIDR フィード有効化 (`on`/`off`) | `on` |
| `ip_security_action` | 脅威一致時のアクション (`verify` または `block`) | `verify` |
| `ip_reputation` | Shodan InternetDB & StopForumSpam ライブ照会有効化 | `on` |
| `ai_ip_list` | 公式 AI クローラー CIDR リスト有効化 | `on` |
| `intel_refresh` | フィード更新間隔（秒または `6h` 等） | `21600` (`6h`) |
| `threat_feed` / `ai_ip_feed` | 追加カスタム IP フィード URL | — |
| `remote_rules` | 全サイトに継承されるリモートファイアウォールルール URL | — |
| `remote_rules_refresh` | リモートルールの更新間隔（秒または `5m`, `1h`；別名 `rules_refresh`） | `300` (`5m`) |
| `rules_file` | 全サイトに継承されるローカル `.rules`/`.json` ルールファイル（`rules_file <path> [dynamic\|static]`） | — |
| `rules_dir` | 全サイトに継承されるローカルルールディレクトリ（`rules_dir <path> [dynamic\|static]`） | — |
| `ruleset <name> { ... }` | 名前付きローカルまたはリモートルールソースブロック（`source`/`file`/`dir`/`url`、`dynamic on\|off`） | — |
| `rules_watch_interval` | `dynamic` なローカルルールファイルおよびディレクトリのポーリング間隔 | `2` (`2s`) |
| `verify_timeout` / `clearance_ttl` | 認証済みクリアランスの既定タイムアウト（秒または `30m`, `12h`, `7d`） | `86400` (`24h`) |

---

## サイトブロック ディレクティブ

```caddyfile
app.example.com, www.example.com {
    reverse_proxy http://localhost:3000 http://localhost:3001
    verify 3
    difficulty 4
    rate_limit 240 60s
    challenge_ttl 5m
    verify_timeout 12h
    cookie_name waf_clearance
    cookie_secure true
    default_rules on
    remote_rules https://rules.example.com/app.rules

    allow ip 10.0.0.0/8
    block path /internal-only*
}
```

| ディレクティブ | 説明 |
| :--- | :--- |
| `reverse_proxy <urls...>` | 1つ以上のアップストリーム（ラウンドロビン、`:3000` 省略形対応） |
| `verify [0\|1\|2\|3\|off]` | 検証レベル（引数なし `verify` はレベル `3`） |
| `protect <on\|off>` | サイトの WAF 検証を有効/無効化 |
| `difficulty <1-5>` | SHA-256 PoW 先頭ゼロ桁数難易度 |
| `rate_limit <count> [window]` | IP 別レート制限回数とウィンドウ（秒または期間；`rate_window` も可） |
| `challenge_ttl <sec\|dur>` | PoW チャレンジナンス有効期限（例：`300`, `5m`） |
| `clearance_ttl <sec\|dur>` | 認証済み Cookie 有効期限（例：`86400`, `30m`, `12h`, `7d`；別名 `verify_timeout`, `verified_timeout`, `clearance_timeout`） |
| `cookie_name <name>` | クリアランス Cookie 名 |
| `cookie_secure <bool>` | `Secure` 属性を付与するか |
| `default_rules <on\|off>` | 組み込みベースラインルールの有効/無効 |
| `remote_rules <urls...>` | リモートファイアウォールルール URL（Gymbagfile 構文または JSON；別名 `remote_rule`, `rules_url`, `rule_url`, `remote_rules_url`） |
| `rules_file <path> [dynamic\|static]` | ローカルの `.rules` または `.json` ファイルを紐付け（ディスク変更時のホットリロード対応） |
| `rules_dir <path> [dynamic\|static]` | ディレクトリ内のすべての `.rules`/`.json` ファイルを紐付け（ホットリロード対応） |
| `ruleset <name> { ... }` | 名前付きルールソースブロック（`source`/`file`/`dir`/`url`、`dynamic on\|off`） |
| `handle` / `handle_path` / `route` | 順序付きパスマッチルートブロック（`reverse_proxy`、`root` + `file_server`、`respond`、`redir`、`header`） |
| `root` / `file_server` / `try_files` | 静的ファイル配信、ディレクトリ一覧表示（`browse`）、SPA フォールバック |
| `respond` / `redir` / `header` | 固定レスポンス、HTTP リダイレクト（`{scheme}`、`{host}`、`{uri}`、`{path}`、`{query}`）、レスポンスヘッダー変更 |
| `ai_bots <patterns...>` | AI クローラーとみなす追加 User-Agent パターン |
| `suspicious_uas <patterns...>` | 不審クライアントとみなす追加 User-Agent パターン |
| `suspicious_paths <prefixes...>` | 不審プローブとみなす追加パスプレフィックス |
| `tls <cert> <key>` | サイト固有 SNI 証明書＆秘密鍵パス |
| `allow \| verify \| block ...` | 1行ショートハンドルール |
| `rule "name" { ... }` | 複数条件カスタムルールブロック |

---

## 期間表記構文

すべての時間ディレクティブ（`clearance_ttl` / `verify_timeout`、`challenge_ttl`、`rate_window`、`intel_refresh`、`remote_rules_refresh`、`renew_interval`）で以下が利用できます：
- **整数秒**: `300`, `3600`, `86400`
- **Go duration**: `45s`, `30m`, `12h`, `1h30m`
- **日・週単位**: `1d`, `7d`, `1d12h`, `2w`

---

## 整形と検証

```bash
# 構文を検証しサイト別ポリシー要約（clearance_ttl・リモートルール数含む）を表示
./gymbag validate --config Gymbagfile

# 標準の4スペースインデントで上書き整形
./gymbag fmt --overwrite Gymbagfile
```

## JSON 設定

`gymbag init` は既定で `gymbag.json` を生成します（Caddy 風の構文にする場合は `--format gymbagfile` を指定してください）：

```bash
./gymbag init -d app.example.com -u :3000 --verify-timeout 12h --rules-dir ./rules.d --rules-dynamic
```

すべてのフィールドは JSON スキーマ（`sites`、`bind_host`、`http_port`、`https_port`、`admin_addr`、`tls`、`intel`、`remote_rules_refresh`、`rules_watch_interval`、`redis_url`、`secret`、`trusted_proxies`、`defaults`）にそのまま対応し、サイト内には `routes`（`path`、`strip_prefix`、`upstreams`、`root`、`index`、`browse`、`fallback`、`respond`、`redirect`、`headers`）、`headers`、`waf.clearance_ttl`（整数秒）、`waf.remote_rules`（`[]string`）、`waf.rule_sources`（`[{"source", "dynamic", "name"}]`）を含みます。

既存の `nginx.conf`、Apache の `httpd.conf`/`apache2.conf`、または `Caddyfile` を `gymbag.json` に変換することもできます：

```bash
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```
