# `Gymbagfile` & JSON スキーマ リファレンス

デフォルトでは作業ディレクトリの `./gymbag.json`、`./Gymbagfile`、`./Caddyfile` を順に探します（`--config <path>` や `GYMBAG_CONFIG` での指定も可）。`gymbag.json`、`Gymbagfile` / `Caddyfile v2`、`nginx.conf`、Apache（`httpd.conf` / `apache2.conf`）をそのまま読み込めます。

## グローバルオプションブロック `{ ... }`

先頭の省略可能な `{ ... }` ブロックで、サーバー全体の TLS、ACME、ネットワーク、脅威/VPN/AI フィード、リモートルール、検証ポータル設定、デフォルトの WAF ポリシーを指定します。

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

    # デフォルトのクリアランス Cookie / IP 有効期限（秒または期間表記: 30m, 12h, 7d）
    verify_timeout 24h

    # SSH・Minecraft 用の共有 HTTPS 検証ポータル
    # verify_url https://verify.example.com:8443
    # verify_port 8443

    # 共有 PoW ナンス、レート制限、SSH/MC ゲートチケット用の任意 Redis ストア
    # redis redis://127.0.0.1:6379/0

    # IP 脅威インテル、VPN/Tor 検出、AI クローラー IP フィード
    ip_security on
    ip_security_action verify
    ip_reputation on
    vpn_detection on
    ai_ip_list on
    intel_refresh 6h

    # 共有リモートファイアウォールルール（Gymbagfile 構文または JSON）
    # remote_rules https://rules.example.com/baseline.rules
    # remote_rules_refresh 5m

    # リモートルールやサイトルールより前に評価されるグローバルカスタムルール
    block ip 198.51.100.99 192.0.2.0/24
}
```

### グローバルディレクティブ

| ディレクティブ | 説明 | デフォルト |
| :--- | :--- | :--- |
| `email` / `acme_email` | ACME 登録用の連絡先メール（ZeroSSL 自動 EAB 兼用） | — |
| `ssl_provider` / `ca` | `letsencrypt`、`letsencrypt_staging`、`zerossl`、`google`、`ssl.com`、`litessl`、`buypass`、`local`、または URL | `auto` (`letsencrypt` / `local`) |
| `acme_directory` | 明示的な RFC 8555 ACME v2 ディレクトリ URL | Let's Encrypt v2 |
| `eab <kid> <hmac>` | External Account Binding 資格情報（`eab_kid` / `eab_hmac_key` も可） | — |
| `account_key` | 既存の ECDSA または RSA ACME アカウント PEM パス | `<cert_dir>/acme-account.pem` |
| `ssl_key` | ACME CSR 用の既存秘密鍵パス | 自動生成 |
| `tls` | `auto`、`acme`、`local`、`selfsigned`、または `<cert.pem> <key.pem>` | `auto` |
| `key_password` | 暗号化 PEM 秘密鍵のパスフレーズ | — |
| `cert_dir` | 証明書・ローカル CA・シークレット・インテルキャッシュ・ルールキャッシュの保存ディレクトリ | `certs` |
| `renew_interval` | 証明書更新チェック間隔（秒または `12h` 等の期間表記） | `43200` (`12h`) |
| `renew_within_days` | 有効期限の残り N 日以内で更新 | `30` |
| `auto_hosts` | `localhost` / `*.internal` の `/etc/hosts` 自動管理 (`on`/`off`) | `on` |
| `hsts` | `Strict-Transport-Security` ヘッダー送信 (`on`/`off`) | 公開 ACME 証明書では `on` |
| `bind` / `bind_host` | 待受 IP アドレス | `127.0.0.1`（ローカル HTTP）/ `0.0.0.0`（公開 HTTP、SSH、MC） |
| `http_port` / `https_port` | HTTP リダイレクト/ACME ポートと HTTPS ゲートウェイポート | `80` / `443` |
| `verify_url` | SSH/Minecraft 検証ポータルの公開 HTTPS URL | サーバー IP + IP SSL 証明書 |
| `verify_port` | SSH/Minecraft 検証ポータルの共有 HTTPS ポート | `8443` |
| `verify_server { ... }` | `url`、`port`、`tls <cert> <key>` を設定できる任意ブロック | — |
| `admin` | ローカル Admin REST API の待受アドレス（例: `127.0.0.1:2019` または `off`） | 無効 |
| `redis` / `redis_url` | Redis 接続 URL（`redis://` または `rediss://`） | インメモリストア |
| `secret` | `waf_clearance` Cookie 用 HMAC シークレット | `<cert_dir>/secret` |
| `trusted_proxies` | `X-Forwarded-For` を信頼する下流プロキシの IP/CIDR | — |
| `ip_security` | 脅威インテル CIDR フィードを有効化 (`on`/`off`) | `on` |
| `ip_security_action` | 脅威インテル一致時のアクション (`verify` または `block`) | `verify` |
| `ip_reputation` | Shodan InternetDB & StopForumSpam のライブ照会（`vpn_detection` 有効時はライブ VPN API 照会も含む） | `on` |
| `vpn_detection` / `vpn_detect` | 検証レベル `1` & `3` で無料 VPN/プロキシ/Tor IP を不審判定に含める (`on`/`off`) | `on` |
| `ai_ip_list` | 公式 AI クローラー CIDR リストを有効化 (`on`/`off`) | `on` |
| `intel_refresh` | フィード更新間隔（秒または `6h` 等の期間表記） | `21600` (`6h`) |
| `threat_feed` / `ai_ip_feed` / `vpn_feed` | 追加のカスタム IP フィード URL（`threat_feeds`、`ai_ip_feeds`、`vpn_feeds` も使用可） | — |
| `remote_rules` | 全サイトに継承される HTTP/HTTPS リモートファイアウォールルール URL | — |
| `remote_rules_refresh` | リモートルール URL の更新間隔（秒または期間表記；別名 `rules_refresh`） | `300` (`5m`) |
| `rules_file` | ローカル `.rules`/`.json` ルールファイル（`rules_file <path> [dynamic\|static]`） | — |
| `rules_dir` | ローカルルールディレクトリ（`rules_dir <path> [dynamic\|static]`） | — |
| `ruleset <name> { ... }` | 名前付きのローカルまたはリモートルールソースブロック（`source`/`file`/`dir`/`url`、`dynamic on\|off`） | — |
| `rules_watch_interval` | `dynamic` ローカルルールファイルとフォルダのポーリング間隔 | `2` (`2s`) |
| `verify_timeout` / `clearance_ttl` | デフォルトのクリアランス有効期間（秒または `30m`、`12h`、`7d` 等の期間表記） | `86400` (`24h`) |

## サイトブロックのディレクティブ

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
| `reverse_proxy <urls...>` | 1 つ以上のアップストリーム宛先（ラウンドロビン負荷分散、`:3000` 省略形に対応） |
| `verify [0\|1\|2\|3\|off]` | 検証レベル（引数なしの `verify` はレベル `3`） |
| `protect <on\|off>` | WAF 検証の有効/無効 |
| `difficulty <1-5>` | SHA-256 先頭 16 進ゼロ数の PoW 難易度 |
| `rate_limit <count> [window]` | IP ごとのレート制限と任意のウィンドウ（`rate_window` も使用可） |
| `challenge_ttl <sec\|dur>` | PoW チャレンジナンスの有効期限（例: `300` や `5m`） |
| `clearance_ttl <sec\|dur>` | 認証済みクリアランス有効期間（別名: `verify_timeout`, `verified_timeout`, `clearance_timeout`） |
| `cookie_name <name>` | クリアランス Cookie 名 |
| `cookie_secure <bool>` | クリアランス Cookie に `Secure` を付与するか |
| `default_rules <on\|off>` | 組み込みベースラインルールの切替 |
| `remote_rules <urls...>` | リモートファイアウォールルール URL（Gymbagfile 構文または JSON） |
| `rules_file <path> [dynamic\|static]` | ローカル `.rules` または `.json` ファイル（任意でホットリロード） |
| `rules_dir <path> [dynamic\|static]` | `.rules`/`.json` ファイルを含むディレクトリ（任意でホットリロード） |
| `ruleset <name> { ... }` | 名前付きルールソースブロック（`source`/`file`/`dir`/`url`、`dynamic on\|off`） |
| `handle` / `handle_path` / `route` | 順序付きパスマッチルートブロック（`reverse_proxy`、`root` + `file_server`、`respond`、`redir`、`header`） |
| `root` / `file_server` / `try_files` | 静的ファイル配信、ディレクトリ一覧（`browse`）、SPA フォールバック |
| `respond` / `redir` / `header` | 固定レスポンス、リダイレクト（`{scheme}`, `{host}`, `{uri}`, `{path}`, `{query}`）、ヘッダー編集 |
| `ai_bots <patterns...>` | AI クローラーとみなす追加の User-Agent 部分文字列 |
| `suspicious_uas <patterns...>` | 不審とみなす追加の User-Agent 部分文字列 |
| `suspicious_paths <prefixes...>` | 不審プローブとみなす追加のパスプレフィックス |
| `tls <cert> <key>` | サイトごとの SNI 証明書および秘密鍵のパス |
| `allow \| verify \| block ...` | 短縮カスタムファイアウォールルール |
| `rule "name" { ... }` | 複数条件カスタムファイアウォールルールブロック |

## `ssh` および `minecraft` サービスブロック

トップレベルの `ssh [port] { ... }` および `minecraft [port] { ... }`（別名 `mc`）ブロックで TCP PoW ゲーティングを構成します。

```caddyfile
ssh {
    port 22
    upstream 127.0.0.1:41822
    verify 3
    difficulty 4
    rate_limit 60 60s
    clearance_ttl 24h
    allow ip 10.0.0.0/8
}

minecraft {
    port 25565
    upstream 127.0.0.1:48291
    server_jar /srv/mc/paper.jar
    jvm_args -Xms1G -Xmx4G
    server_properties /srv/mc/server.properties
    verify 3
    rate_limit 60 60s
}
```

| ディレクティブ | 説明 | デフォルト |
| :--- | :--- | :--- |
| `enabled <bool>` | ブロックを削除せずに有効/無効を切替 | `true` |
| `port <port>` | 公開待受ポート（`ssh 2222 { ... }` のようにも記述可） | `22` (SSH) / `25565` (Minecraft) |
| `upstream <host:port>` | バックエンド TCP 宛先（`reverse_proxy` / `proxy` も使用可、例: `:2222`、`tcp://127.0.0.1:25566`） | —（`server_jar` 指定時は `127.0.0.1:25566`） |
| `server_properties <path>` | `server.properties` のパス（`minecraft` ブロック専用） | — |
| `server_jar <path>` / `jar` | Gymbag とあわせて自動起動する Minecraft `.jar` のパス（`minecraft` ブロック専用） | — |
| `java_path <bin>` / `java` | 明示的な Java バイナリの上書き（`minecraft` ブロック専用） | 自動選択 |
| `jvm_args <flags...>` | `-jar` の前に渡す JVM 引数（`minecraft` ブロック専用） | — |
| `server_args <args...>` | JAR パスの後に渡す追加引数（`minecraft` ブロック専用） | — |
| `work_dir <dir>` | 管理対象 Minecraft プロセスの作業ディレクトリ（`minecraft` ブロック専用） | JAR のディレクトリ |
| `auto_restart <on\|off>` | Minecraft サーバーが予期せず終了した場合に自動再起動する（`minecraft` ブロック専用） | `on` |
| `verify [0\|1\|2\|3\|off]` | 検証レベル（引数なし `verify` は `3`） | `3` |
| `protect <on\|off>` | このサービスでの PoW 検証を切替 | `on` |
| `difficulty <1-5>` | PoW の先頭 16 進ゼロ数難易度 | `4` |
| `rate_limit <count> [window]` | IP ごとの接続レート制限回数とウィンドウ | `60` / `60s` |
| `challenge_ttl <sec\|dur>` | チャレンジトークンの有効期限 | `300` (`5m`) |
| `clearance_ttl <sec\|dur>` | クリア済み IP の有効期間（`verify_timeout` も使用可） | `86400` (`24h`) |
| `verify_url` / `verify_port` | 切断メッセージに表示する検証リンクのサービス別上書き | グローバル `verify_url` / `verify_port` |
| `allow \| verify \| block ...` / `rule` | IP/CIDR またはクライアントバナー/ユーザー名（`user_agent`）に一致するカスタムルール | — |

## 期間表記の構文

時間を指定するディレクティブ（`clearance_ttl` / `verify_timeout`、`challenge_ttl`、`rate_window`、`intel_refresh`、`remote_rules_refresh`、`rules_watch_interval`、`renew_interval`）では次の表記を使えます。
- 整数の秒数: `300`、`3600`、`86400`
- Go duration 形式: `45s`、`30m`、`12h`、`1h30m`
- 日・週の省略記法: `1d`、`7d`、`1d12h`、`2w`

## 整形と検証

```bash
./gymbag validate --config Gymbagfile
./gymbag fmt --overwrite Gymbagfile
```

## JSON 設定（`gymbag.json`）

`gymbag init` はデフォルトで未使用サービスやデフォルト値を省いた簡潔な `gymbag.json` を出力します（`--format gymbagfile` で Caddy 構文に切り替えられます）。

```bash
./gymbag init -d app.example.com -u :3000 --verify-timeout 12h --rules-dir ./rules.d --rules-dynamic
```

トップレベル JSON キー（`DisallowUnknownFields` が適用されます）:
- `sites`: `{domain, upstreams, routes, headers, protect, waf, cert_file, key_file}` の配列。`waf` には `{verify_level, difficulty, challenge_ttl, clearance_ttl, cookie_name, cookie_secure, rate_limit, rate_window, default_rules, rules, remote_rules, rule_sources, custom_ai_bots, custom_suspicious_uas, custom_suspicious_paths}` を含みます。
- `ssh`: `{enabled, port, upstream, protect, verify_level, difficulty, challenge_ttl, clearance_ttl, rate_limit, rate_window, verify_url, verify_port, rules}`
- `minecraft`: `{enabled, port, upstream, server_properties, server_jar, java_path, jvm_args, server_args, work_dir, auto_restart, protect, verify_level, difficulty, challenge_ttl, clearance_ttl, rate_limit, rate_window, verify_url, verify_port, rules}`
- `verify`: `{url, port, cert_file, key_file}`
- `bind_host`、`http_port`、`https_port`、`admin_addr`、`remote_rules_refresh`、`rules_watch_interval`、`redis_url`、`secret`、`trusted_proxies`
- `tls`: `{mode, provider, cert_file, key_file, key_password, account_key_file, eab_kid, eab_hmac_key, auto_hosts, cert_dir, acme_email, acme_directory, renew_interval, renew_within_days, hsts}`
- `intel`: `{ip_security, ip_security_action, live_reputation, ai_ip_list, vpn_detection, refresh_interval, threat_feeds, ai_ip_feeds, vpn_feeds}`
- `defaults`: 共有ベースとなる `waf` オブジェクト

`nginx.conf`、Apache `httpd.conf`/`apache2.conf`、`Caddyfile` から `gymbag.json` への変換も行えます。

```bash
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```
