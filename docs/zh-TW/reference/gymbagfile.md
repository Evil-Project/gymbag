# `Gymbagfile` 與 JSON Schema 參考

預設情況下，Gymbag 會在工作目錄自動搜尋 `./gymbag.json`、`./Gymbagfile` 與 `./Caddyfile`（亦可透過 `--config <path>` 或 `GYMBAG_CONFIG` 指定）。Gymbag 原生支援解析 `gymbag.json`、`Gymbagfile` / `Caddyfile v2`、`nginx.conf` 以及 Apache（`httpd.conf` / `apache2.conf`）。

## 全域設定區塊 `{ ... }`

檔案最上方可選擇性加入 `{ ... }` 區塊，用來設定全伺服器層級的 TLS、ACME、網路介面、威脅／VPN／AI 情資清單、遠端規則、驗證入口與預設 WAF 策略：

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

    # 預設通行 Cookie / IP 有效時間（秒數或時間長度：30m、12h、7d）
    verify_timeout 24h

    # 共用的 SSH 與 Minecraft HTTPS 驗證入口
    # verify_url https://verify.example.com:8443
    # verify_port 8443

    # 選用的 Redis 儲存，用於共用 PoW Nonce、速率限制與 SSH/MC 閘道票證
    # redis redis://127.0.0.1:6379/0

    # IP 威脅情資、VPN/Tor 偵測與 AI 爬蟲 IP 清單
    ip_security on
    ip_security_action verify
    ip_reputation on
    vpn_detection on
    ai_ip_list on
    intel_refresh 6h

    # 共用的遠端防火牆規則（支援 Gymbagfile 語法或 JSON）
    # remote_rules https://rules.example.com/baseline.rules
    # remote_rules_refresh 5m

    # 全域自訂規則（會在遠端規則與站台規則之前執行）
    block ip 198.51.100.99 192.0.2.0/24
}
```

### 全域指令

| 指令 | 說明 | 預設值 |
| :--- | :--- | :--- |
| `email` / `acme_email` | ACME 註冊聯絡信箱（同時用於 ZeroSSL 自動取得 EAB） | — |
| `ssl_provider` / `ca` | `letsencrypt`、`letsencrypt_staging`、`zerossl`、`google`、`ssl.com`、`litessl`、`buypass`、`local` 或自訂 URL | `auto`（`letsencrypt` / `local`） |
| `acme_directory` | 明確指定 RFC 8555 ACME v2 目錄 URL | Let's Encrypt v2 |
| `eab <kid> <hmac>` | External Account Binding 憑證（亦支援 `eab_kid` / `eab_hmac_key`） | — |
| `account_key` | 既有的 ECDSA 或 RSA ACME 帳戶 PEM 檔案路徑 | `<cert_dir>/acme-account.pem` |
| `ssl_key` | 用於簽署 ACME CSR 的既有私鑰路徑 | 自動產生 |
| `tls` | `auto`、`acme`、`local`、`selfsigned`、`file`、`off`、`acme.sh <path>` 或 `<cert.pem> <key.pem>` | `auto` |
| `auto_ssl` | 啟用或停用自動 SSL 申請（`on`/`off`；亦支援 `auto_https on\|off\|disable_redirects`） | `on` |
| `acme_sh_path` | `acme.sh` 主目錄（`~/.acme.sh`）或網域憑證目錄路徑（別名 `acme_sh`） | — |
| `direct_http` | 直接在 `http_port` 上提供網站服務而不重導向至 HTTPS（亦支援 `allow_http` / `http_only`） | `off`（停用 TLS 時為 `on`） |
| `key_password` | 加密 PEM 私鑰的解鎖密碼 | — |
| `cert_dir` | 存放憑證、本機 CA、Secret、情資快取與規則快取的目錄 | `certs` |
| `renew_interval` | 憑證續期檢查間隔（秒數或時間長度如 `12h`） | `43200`（`12h`） |
| `renew_within_days` | 距離到期日少於 N 天時自動續期憑證 | `30` |
| `auto_hosts` | 自動管理 `localhost` / `*.internal` 的 `/etc/hosts` 項目（`on`/`off`） | `on` |
| `hsts` | 傳送 `Strict-Transport-Security` 標頭（`on`/`off`） | 公開 ACME 憑證預設 `on` |
| `bind` / `bind_host` | 監聽 IP 位址 | `127.0.0.1`（本機 HTTP）/ `0.0.0.0`（公開 HTTP、SSH 或 MC） |
| `http_port` / `https_port` | HTTP 重導向 / ACME 連接埠與 HTTPS 閘道連接埠 | `80` / `443` |
| `verify_url` | SSH/Minecraft 驗證入口的公開 HTTPS URL | 伺服器 IP + IP SSL 憑證 |
| `verify_port` | SSH/Minecraft 驗證入口共用的 HTTPS 連接埠 | `8443` |
| `verify_server { ... }` | 選用區塊，支援設定 `url`、`port` 與 `tls <cert> <key>` | — |
| `admin` | 本機管理 REST API 監聽位址（如 `127.0.0.1:2019` 或 `off`） | 停用 |
| `redis` / `redis_url` | Redis 連線 URL（`redis://` 或 `rediss://`） | 記憶體儲存 |
| `secret` | 用於簽署 `waf_clearance` Cookie 的 HMAC 密鑰 | `<cert_dir>/secret` |
| `trusted_proxies` | 解析 `X-Forwarded-For` 時信任的下游代理 IP/CIDR | — |
| `mascot` | 自訂吉祥物檔案、目錄、`http(s)://` URL，或 `mascot { default / pensive / happy / reject }` 區塊（亦支援 `mascot_pensive`、`mascot_happy`、`mascot_reject`） | 內嵌預設圖檔 |
| `ip_security` | 啟用威脅情資 CIDR 清單（`on`/`off`） | `on` |
| `ip_security_action` | 命中威脅情資時的動作（`verify` 或 `block`） | `verify` |
| `ip_reputation` | 啟用即時 Shodan InternetDB 與 StopForumSpam 查詢（當 `vpn_detection` 開啟時亦包含即時 VPN API 查詢） | `on` |
| `vpn_detection` / `vpn_detect` | 在驗證等級 `1` 與 `3` 時將免費 VPN/Proxy/Tor IP 標記為可疑（`on`/`off`） | `on` |
| `ai_ip_list` | 啟用官方 AI 爬蟲 CIDR 清單（`on`/`off`） | `on` |
| `intel_refresh` | 情資清單更新間隔（秒數或時間長度如 `6h`） | `21600`（`6h`） |
| `threat_feed` / `ai_ip_feed` / `vpn_feed` | 額外的自訂 IP 清單 URL（亦支援 `threat_feeds`、`ai_ip_feeds`、`vpn_feeds`） | — |
| `remote_rules` | 由所有站台繼承的 HTTP/HTTPS 遠端防火牆規則 URL | — |
| `remote_rules_refresh` | 遠端規則 URL 的更新間隔（秒數或時間長度；別名 `rules_refresh`） | `300`（`5m`） |
| `rules_file` | 本機 `.rules`/`.json` 規則檔案（`rules_file <path> [dynamic\|static]`） | — |
| `rules_dir` | 本機規則目錄（`rules_dir <path> [dynamic\|static]`） | — |
| `ruleset <name> { ... }` | 具名的本機或遠端規則來源區塊（`source`/`file`/`dir`/`url`、`dynamic on\|off`） | — |
| `rules_watch_interval` | `dynamic` 本機規則檔案與資料夾的輪詢間隔 | `2`（`2s`） |
| `verify_timeout` / `clearance_ttl` | 預設驗證通行有效時間（秒數或時間長度如 `30m`、`12h`、`7d`） | `86400`（`24h`） |
| `authelia <url> [{ ... }]` | 預設 Authelia 前置驗證（Forward Auth）設定，供僅寫 `authelia` 的站台繼承 | — |
| `forward_auth <url> { ... }` | 通用 Caddy v2 前置驗證範本 | — |

## 站台區塊指令

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

| 指令 | 說明 |
| :--- | :--- |
| `reverse_proxy <urls...>` | 一個或多個上游目標（採用輪詢負載平衡；支援 `:3000` 簡寫） |
| `verify [0\|1\|2\|3\|off]` | 驗證等級（單獨寫 `verify` 代表啟用等級 `3`） |
| `protect <on\|off>` | 啟用或停用 WAF 驗證 |
| `difficulty <1-5>` | SHA-256 前導十六進位零的 PoW 難度 |
| `rate_limit <count> [window]` | 單一 IP 速率限制與選用時間窗（亦支援 `rate_window`） |
| `challenge_ttl <sec\|dur>` | PoW 挑戰 Nonce 過期時間（例如 `300` 或 `5m`） |
| `clearance_ttl <sec\|dur>` | 驗證通過後的有效時間（別名：`verify_timeout`、`verified_timeout`、`clearance_timeout`） |
| `cookie_name <name>` | 通行 Cookie 名稱 |
| `cookie_secure <bool>` | 通行 Cookie 是否加上 `Secure` 屬性 |
| `default_rules <on\|off>` | 切換內建基礎規則 |
| `remote_rules <urls...>` | 遠端防火牆規則 URL（Gymbagfile 語法或 JSON） |
| `rules_file <path> [dynamic\|static]` | 本機 `.rules` 或 `.json` 檔案（可選擇開啟熱重載） |
| `rules_dir <path> [dynamic\|static]` | 存放 `.rules`/`.json` 檔案的目錄（可選擇開啟熱重載） |
| `ruleset <name> { ... }` | 具名規則來源區塊（`source`/`file`/`dir`/`url`、`dynamic on\|off`） |
| `handle` / `handle_path` / `route` | 依序比對路徑的路由區塊（支援 `reverse_proxy`、`authelia`/`forward_auth`、`root` + `file_server`、`respond`、`redir`、`header`） |
| `authelia [url] [{ ... }]` | 在站台上強制執行 Authelia 前置驗證（單獨寫 `authelia` 會繼承全域 `{ authelia ... }`；支援 `uri`、`portal_url`、`copy_headers`、`path`、`except`、`header`、`timeout`、`insecure_skip_tls`） |
| `forward_auth [matcher] <url> { ... }` | Caddy v2 前置驗證指令（支援 `uri`、`copy_headers`、`header_up`、`transport http { tls_insecure_skip_verify }`） |
| `root` / `file_server` / `try_files` | 靜態檔案服務、目錄瀏覽（`browse`）與 SPA 後備路由 |
| `respond` / `redir` / `header` | 靜態回應、重導向（支援 `{scheme}`、`{host}`、`{uri}`、`{path}`、`{query}`）與標頭修改 |
| `ai_bots <patterns...>` | 額外視為 AI 爬蟲的 `User-Agent` 子字串 |
| `suspicious_uas <patterns...>` | 額外視為可疑的 `User-Agent` 子字串 |
| `suspicious_paths <prefixes...>` | 額外視為可疑探測的路徑前綴 |
| `tls <cert> <key>` | 個別站台的 SNI 憑證與私鑰路徑（或 `tls acme.sh <path>`） |
| `acme_sh_path <path>` | 個別站台的 `acme.sh` 主目錄或網域憑證目錄（別名 `acme_sh`） |
| `mascot <path-or-url>` | 個別站台的自訂吉祥物檔案、目錄、URL 或 `mascot { ... }` 區塊（`mascot_pensive`、`mascot_happy`、`mascot_reject`） |
| `allow \| verify \| block ...` | 簡寫自訂防火牆規則 |
| `rule "name" { ... }` | 多條件自訂防火牆規則區塊 |

## `ssh` 與 `minecraft` 服務區塊

頂層的 `ssh [port] { ... }` 與 `minecraft [port] { ... }`（別名 `mc`）區塊用來設定 TCP PoW 閘道防護：

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

| 指令 | 說明 | 預設值 |
| :--- | :--- | :--- |
| `enabled <bool>` | 啟用或停用該區塊而無須刪除設定 | `true` |
| `port <port>` | 對外監聽連接埠（亦可直接寫作 `ssh 2222 { ... }`） | `22`（SSH）/ `25565`（Minecraft） |
| `upstream <host:port>` | 後端 TCP 目標（亦支援 `reverse_proxy` / `proxy`，例如 `:2222`、`tcp://127.0.0.1:25566`） | —（設定 `server_jar` 時為 `127.0.0.1:25566`） |
| `server_properties <path>` | `server.properties` 檔案路徑（僅限 `minecraft` 區塊） | — |
| `server_jar <path>` / `jar` | 要隨 Gymbag 一同自動啟動的 Minecraft `.jar` 路徑（僅限 `minecraft` 區塊） | — |
| `java_path <bin>` / `java` | 明確指定 Java 執行檔路徑（僅限 `minecraft` 區塊） | 自動選擇 |
| `jvm_args <flags...>` | 傳遞在 `-jar` 之前的 JVM 參數（僅限 `minecraft` 區塊） | — |
| `server_args <args...>` | 傳遞在 JAR 路徑之後的額外參數（僅限 `minecraft` 區塊） | — |
| `work_dir <dir>` | 託管 Minecraft 行程的工作目錄（僅限 `minecraft` 區塊） | JAR 所在目錄 |
| `auto_restart <on\|off>` | Minecraft 伺服器非預期結束時自動重啟（僅限 `minecraft` 區塊） | `on` |
| `console <on\|off>` | 將託管的 Minecraft 伺服器主控台整合至 Gymbag 前景、`gymbag console` 與 `/api/v1/minecraft/console`（別名 `mc_console`；僅限 `minecraft` 區塊） | `off` |
| `verify [0\|1\|2\|3\|off]` | 驗證等級（單獨寫 `verify` 會設為 `3`） | `3` |
| `protect <on\|off>` | 切換此服務的 PoW 驗證開關 | `on` |
| `difficulty <1-5>` | 前導十六進位零的 PoW 難度 | `4` |
| `rate_limit <count> [window]` | 單一 IP 連線速率限制與時間窗 | `60` / `60s` |
| `challenge_ttl <sec\|dur>` | 挑戰 Token 過期時間 | `300`（`5m`） |
| `clearance_ttl <sec\|dur>` | 已通過驗證的 IP 有效時間（亦支援 `verify_timeout`） | `86400`（`24h`） |
| `verify_url` / `verify_port` | 個別服務覆蓋斷線訊息中所印出的驗證連結 | 全域 `verify_url` / `verify_port` |
| `allow \| verify \| block ...` / `rule` | 比對 IP/CIDR 或用戶端識別字串／玩家名稱（`user_agent`）的自訂規則 | — |

## 時間長度語法

時間相關指令（`clearance_ttl` / `verify_timeout`、`challenge_ttl`、`rate_window`、`intel_refresh`、`remote_rules_refresh`、`rules_watch_interval` 與 `renew_interval`）支援以下格式：
- 純整數秒數：`300`、`3600`、`86400`
- Go 時間長度：`45s`、`30m`、`12h`、`1h30m`
- 天與週簡寫：`1d`、`7d`、`1d12h`、`2w`

## 格式化與驗證

```bash
./gymbag validate --config Gymbagfile
./gymbag fmt --overwrite Gymbagfile
```

## JSON 設定格式（`gymbag.json`）

`gymbag init` 預設會產生精簡的 `gymbag.json`，省略未使用的服務與預設值（加上 `--format gymbagfile` 則改用 Caddy 語法）：

```bash
./gymbag init -d app.example.com -u :3000 --verify-timeout 12h --rules-dir ./rules.d --rules-dynamic
```

頂層 JSON 鍵值（嚴格執行 `DisallowUnknownFields`，不允許未知欄位）：
- `sites`：`{domain, upstreams, routes, headers, auth, protect, mascot, waf, cert_file, key_file, acme_sh_path}` 陣列，其中 `mascot` 為路徑/URL 字串或 `{default, pensive, happy, reject}`，`waf` 包含 `{verify_level, difficulty, challenge_ttl, clearance_ttl, cookie_name, cookie_secure, rate_limit, rate_window, default_rules, rules, remote_rules, rule_sources, custom_ai_bots, custom_suspicious_uas, custom_suspicious_paths}`，`auth` 包含 `{provider, url, uri, portal_url, copy_headers, headers, paths, except_paths, timeout, insecure_skip_tls}`。
- `authelia`：選用的全域前置驗證物件，結構與站台 `auth` 相同。
- `ssh`：`{enabled, port, upstream, protect, verify_level, difficulty, challenge_ttl, clearance_ttl, rate_limit, rate_window, verify_url, verify_port, rules}`。
- `minecraft`：`{enabled, port, upstream, server_properties, server_jar, java_path, jvm_args, server_args, work_dir, auto_restart, console, protect, verify_level, difficulty, challenge_ttl, clearance_ttl, rate_limit, rate_window, verify_url, verify_port, rules}`。
- `verify`：`{url, port, cert_file, key_file}`。
- `bind_host`、`http_port`、`https_port`、`direct_http`、`admin_addr`、`remote_rules_refresh`、`rules_watch_interval`、`redis_url`、`secret`、`trusted_proxies`、`mascot`。
- `tls`：`{mode, auto_ssl, provider, cert_file, key_file, acme_sh_path, key_password, account_key_file, eab_kid, eab_hmac_key, auto_hosts, cert_dir, acme_email, acme_directory, renew_interval, renew_within_days, hsts}`。
- `intel`：`{ip_security, ip_security_action, live_reputation, ai_ip_list, vpn_detection, refresh_interval, threat_feeds, ai_ip_feeds, vpn_feeds}`。
- `defaults`：共用的基礎 `waf` 物件。

您也可以將 `nginx.conf`、Apache `httpd.conf`/`apache2.conf` 或 `Caddyfile` 轉換為 `gymbag.json`：

```bash
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```
