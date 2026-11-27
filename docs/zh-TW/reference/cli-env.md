# CLI 與 `GYMBAG_*` 環境變數

設定套用的優先順序為：

**設定檔 -> `.env` / `GYMBAG_*` 環境變數 -> CLI 參數**

若工作目錄下存在 `.env`，Gymbag 會自動載入其中的變數，但不會覆蓋系統中既有的環境變數。

## CLI 子指令

```bash
# 產生設定檔（支援互動式精靈或參數）；預設寫入 gymbag.json
gymbag init [flags] [output_path]
gymbag init --format gymbagfile -o Gymbagfile -d app.example.com -u :3000

# 使用 ./gymbag.json、./Gymbagfile 或 ./Caddyfile 啟動
gymbag

# 指定設定檔啟動，並在檔案變更時自動熱重載（每 2 秒輪詢一次）
gymbag run --config Gymbagfile --watch

# 管理作業系統服務（systemd、launchd、openrc、supervisor、freebsd-rc、openbsd-rc、netbsd-rc）
gymbag service <install|start|stop|restart|status|uninstall|render> [flags]

# 互動式連接或傳送指令至託管的 Minecraft 伺服器主控台
gymbag console [--config gymbag.json] [--admin 127.0.0.1:2019] [--cmd "..."] [--tail 200] [--no-stream]

# 管理網站、路由、全域設定與轉換設定檔格式
gymbag config <list|show|add|update|remove|site|route|set|unset|convert> [flags]

# 管理規則檔案、資料夾與單條內嵌規則
gymbag ruleset <list|add|update|remove|validate|init|rule> [flags]

# 無須設定檔直接啟動臨時 HTTP/HTTPS 反向代理
gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3 --verify-timeout 12h

# 驗證設定檔並印出解析後的服務摘要（包含為 server_jar 選定的 Java 版本）
gymbag validate --config Gymbagfile

# 就地格式化 Gymbagfile（4 空格縮排）
gymbag fmt --overwrite Gymbagfile

# 印出版本資訊
gymbag version
```

### 伺服器參數（`run` / 預設 / `reverse-proxy`）

| 參數 | 說明 |
| :--- | :--- |
| `-c`, `--config <path>` | 指定 `gymbag.json`、`Gymbagfile`、`Caddyfile`、`nginx.conf` 或 Apache 設定檔路徑 |
| `-d`, `--domain <list>` | 覆蓋下游網域，多個網域以逗號分隔（在 `reverse-proxy` 中為 `--from`） |
| `-u`, `--upstream <url>` | 覆蓋上游目標 URL（如 `http://localhost:3000` 或 `:3000`；在 `reverse-proxy` 中為 `--to`） |
| `-l`, `--verify <0-3\|off>` | 覆蓋驗證等級（`0`、`1`、`2`、`3` 或 `off`；別名為 `--verify-level`、`-m`、`--mode`） |
| `--verify-timeout <dur>` | 覆蓋驗證通行有效時間（`30m`、`12h`、`7d` 或秒數；別名為 `--clearance-ttl`） |
| `--remote-rules <url>` | 掛載遠端防火牆規則 URL（可重複指定或以逗號分隔） |
| `--remote-rules-refresh <dur>` | 遠端規則的更新間隔（預設 `5m` / `300s`） |
| `--no-protect` | 停用透過 CLI 設定之網站的 PoW 驗證 |
| `--ssl-provider <name>` | 覆蓋 ACME / SSL 憑證供應商 |
| `--no-auto-ssl` | 停用自動 SSL 憑證申請（ACME、本機 CA、自簽憑證） |
| `--acme-sh-path <path>` | 使用由 `acme.sh` 管理的憑證（別名為 `--acme-sh`） |
| `--cert-file <path>` / `--key-file <path>` | 自訂 TLS 憑證與私鑰 PEM 檔案路徑 |
| `--direct-http` | 直接透過 HTTP 提供網站服務而不重導向至 HTTPS（別名為 `--allow-http`） |
| `--http-only` | 停用 HTTPS 與 Auto-SSL，僅透過 HTTP 提供網站服務 |
| `--mascot <path-or-url>` | 自訂吉祥物圖片檔案、目錄或 `http(s)://` URL（可使用 `--mascot-pensive`、`--mascot-happy`、`--mascot-reject` 個別覆蓋各狀態圖片） |
| `--admin <addr>` | 啟用本機 REST 管理 API（`127.0.0.1:2019` 或 `off`） |
| `--http-port <port>` | HTTP 重導向與 ACME HTTP-01 連接埠（預設 `80`） |
| `--https-port <port>` | HTTPS 閘道連接埠（預設 `443`） |
| `--mc-console` | 將託管的 Minecraft 伺服器主控台整合至 Gymbag 前景與管理 API（別名為 `--minecraft-console`） |
| `--watch` | 監聽設定檔變更並熱重載（僅限 `run` / 預設模式） |

## 設定管理（`gymbag config`）

所有 `config` 子指令皆支援 `-c`、`--config <path>`（預設自動偵測目前目錄下的設定檔）。

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

| 子指令 | 說明 |
| :--- | :--- |
| `list` / `show` | 列出網站與 TCP 服務 / 印出格式化後的設定（`--format json\|gymbagfile`） |
| `add` | 新增網站：`-d`、`-u`、`-l`、`--difficulty`、`--verify-timeout`、`--authelia`、`--authelia-uri`、`--authelia-portal`、`--no-protect`、`--no-default-rules`、`--cert`、`--key`、`--root`、`--browse`、`--fallback`、`--respond`、`--status`、`--redir`、`--code`、`--header Name=Value\|-Name`、`-f` |
| `update` | 更新網站：`-d`、`-u`、`-l`、`--difficulty`、`--verify-timeout`、`--authelia`、`--authelia-uri`、`--authelia-portal`、`--clear-authelia`、`--protect on\|off`、`--default-rules on\|off`、`--cert`、`--key`、`--clear-tls`、`--header`、`--remove-header` |
| `remove` | 移除網站（`-d`） |
| `route list\|add\|update\|remove` | 管理路徑路由：`-d`、`--path`、`--strip-prefix`、`-u`、`--root`、`--browse`、`--fallback`、`--respond`、`--status`、`--redir`、`--code`、`--header`、`--index` |
| `set <key> <value>` / `unset <key>` | 設定或重設全域鍵值（`email`、`ssl_provider`、`http_port`、`https_port`、`bind`、`admin`、`cert_dir`、`redis`、`ip_security`、`ip_security_action`、`ip_reputation`、`ai_ip_list`、`intel_refresh`、`remote_rules_refresh`、`rules_watch_interval`、`verify_timeout`、`auto_hosts`、`hsts`、`eab_kid`、`eab_hmac_key`、`authelia`、`authelia_uri`、`authelia_portal`、`mc_console`） |
| `convert` | 轉換 nginx、Apache、Caddyfile、Gymbagfile 或 JSON：`-c <in>`、`-o <out>`、`--format json\|gymbagfile`、`-f` |

## 規則集管理（`gymbag ruleset`）

```bash
gymbag ruleset init ./rules/custom.rules
gymbag ruleset add --source ./rules --dynamic --name local-dir
gymbag ruleset update local-dir --static
gymbag ruleset remove local-dir
gymbag ruleset validate ./rules
gymbag ruleset rule add --file ./rules/custom.rules --action block --name bad-bot --ua "*BadBot*"
```

| 子指令 | 說明 |
| :--- | :--- |
| `list` | 列出已連結的規則來源（`-c`、`-d`） |
| `add` | 連結規則檔案、資料夾或 URL：`--source`、`--name`、`--dynamic` / `--static`、`-d <domain>` 或 `--global`、`-c` |
| `update` | 修改已連結的規則集：`--target`（或位置參數的來源／名稱）、`--source`、`--name`、`--dynamic` / `--static`、`-d` / `--global`、`-c` |
| `remove` | 取消連結規則集：`--source <source-or-name>`、`-d` / `--global`、`-c` |
| `validate <path>` | 驗證規則檔案或規則目錄 |
| `init <path>` | 建立範本規則集（`-o`、`--format rules\|json`、`-f`） |
| `rule list\|add\|update\|remove` | 管理 `--file` 中的單條規則（或透過 `-c` / `-d` 管理設定檔內的內嵌規則）：`--name`、`--index`、`--action allow\|verify\|block`、`--ip`、`--ua`、`--path`、`--query`、`--method`、`--header`、`--cookie` |

## 環境變數（`GYMBAG_*`）

| 變數 | 說明 |
| :--- | :--- |
| `GYMBAG_CONFIG` | `gymbag.json`、`Gymbagfile`、`Caddyfile`、`nginx.conf` 或 Apache 設定檔路徑 |
| `GYMBAG_BIND` | 綁定 IP 位址（僅本機 HTTP 時預設 `127.0.0.1`；啟用公開網域、SSH 或 Minecraft 時預設 `0.0.0.0`） |
| `GYMBAG_HTTP_PORT` / `GYMBAG_HTTPS_PORT` | HTTP 與 HTTPS 監聽連接埠（預設 `80` 與 `443`） |
| `GYMBAG_DOMAIN` / `GYMBAG_DOMAINS` | 以逗號分隔的下游網域清單 |
| `GYMBAG_UPSTREAM` | 預設上游目標 URL |
| `GYMBAG_UPSTREAMS` | 以分號分隔的主機對映：`app.example.com=http://web:3000;api.example.com=http://api:8080` |
| `GYMBAG_PROTECT` / `GYMBAG_PROTECT_DOMAINS` | 全域切換 WAF 防護（`on`/`off`），或僅對指定網域啟用防護 |
| `GYMBAG_VERIFY_LEVEL` | 驗證等級：`0`、`1`、`2`、`3` 或 `off`（別名：`GYMBAG_VERIFY`、`GYMBAG_VERIFY_MODE`、`GYMBAG_MODE`、`GYMBAG_TRIGGER`） |
| `GYMBAG_DEFAULT_RULES` | 內建基礎規則（`on` / `off`） |
| `GYMBAG_REMOTE_RULES` / `GYMBAG_REMOTE_RULES_REFRESH` | 遠端規則 URL 與更新間隔（預設 `300`s / `5m`） |
| `GYMBAG_RULES_WATCH_INTERVAL` | `dynamic` 本機規則來源的輪詢間隔（預設 `2`s / `2s`） |
| `GYMBAG_DIFFICULTY` | SHA-256 PoW 難度（`1`–`5`，預設 `4`） |
| `GYMBAG_CHALLENGE_TTL` / `GYMBAG_CLEARANCE_TTL` | 挑戰有效期限（`300`s / `5m`）與驗證通行時間（`86400`s / `24h`；別名：`GYMBAG_VERIFY_TIMEOUT`、`GYMBAG_VERIFIED_TIMEOUT`） |
| `GYMBAG_COOKIE_NAME` / `GYMBAG_COOKIE_SECURE` | 通行 Cookie 名稱（`waf_clearance`）與 `Secure` 旗標（`true`） |
| `GYMBAG_RATE_LIMIT` / `GYMBAG_RATE_WINDOW` | 單一 IP 請求上限（`240`）與時間窗（`60`s / `1m`） |
| `GYMBAG_TLS_MODE` / `GYMBAG_SSL_PROVIDER` | TLS 模式（`auto`、`acme`、`local`、`selfsigned`、`file`、`off`）與 CA 供應商或 URL |
| `GYMBAG_AUTO_SSL` / `GYMBAG_ACME_SH_PATH` | 切換自動 SSL 申請（`on`/`off`）並設定 `acme.sh` 目錄（別名 `GYMBAG_ACME_SH`） |
| `GYMBAG_DIRECT_HTTP` / `GYMBAG_HTTP_ONLY` | 在 HTTPS 之外同時提供直接 HTTP 服務（`on`/`off`，別名 `GYMBAG_ALLOW_HTTP`），或完全停用 HTTPS（`on`/`off`） |
| `GYMBAG_ACME_EMAIL` / `GYMBAG_ACME_DIRECTORY` | ACME 聯絡信箱（可啟用 ZeroSSL 自動 EAB）與明確指定的 ACME 目錄 URL |
| `GYMBAG_EAB_KID` / `GYMBAG_EAB_HMAC_KEY` | External Account Binding（EAB）Key ID 與 HMAC 金鑰 |
| `GYMBAG_ACCOUNT_KEY_FILE` | ECDSA 或 RSA ACME 帳戶私鑰 PEM 路徑 |
| `GYMBAG_CERT_FILE` / `GYMBAG_KEY_FILE` / `GYMBAG_KEY_PASSWORD` | `file` 模式使用的憑證、私鑰與私鑰密碼 |
| `GYMBAG_CERT_DIR` | 存放憑證、CA 金鑰、Secret、情資快取與規則快取的目錄（預設 `certs`） |
| `GYMBAG_RENEW_INTERVAL` / `GYMBAG_RENEW_WITHIN_DAYS` | 憑證續期檢查間隔（`43200`s / `12h`）與到期前續期天數門檻（`30` 天） |
| `GYMBAG_AUTO_HOSTS` / `GYMBAG_HSTS` | 自動管理 `/etc/hosts`（`on`/`off`）與 `Strict-Transport-Security` 覆寫（`on`/`off`） |
| `GYMBAG_IP_SECURITY` / `GYMBAG_IP_SECURITY_ACTION` | 切換威脅情資清單（`on`/`off`）與觸發動作（`verify` 或 `block`） |
| `GYMBAG_IP_REPUTATION` / `GYMBAG_AI_IP_LIST` | 切換 Shodan InternetDB + StopForumSpam 即時信譽查詢（`on`/`off`）與官方 AI 爬蟲清單（`on`/`off`） |
| `GYMBAG_VPN_DETECTION` | 在可疑連線檢查中切換免費 VPN/Proxy/Tor 偵測（`on`/`off`，預設 `on`） |
| `GYMBAG_INTEL_REFRESH` | 威脅、VPN 與 AI 情資清單的更新間隔（預設 `21600`s / `6h`） |
| `GYMBAG_VERIFY_URL` / `GYMBAG_VERIFY_PORT` | 共用 HTTPS 驗證入口 URL（留空則使用伺服器 IP + IP 憑證）與連接埠（預設 `8443`） |
| `GYMBAG_SSH_ENABLED` / `GYMBAG_SSH_PORT` / `GYMBAG_SSH_UPSTREAM` | 啟用 SSH 防護（預設 `22`）並設定後端 `sshd` 目標 |
| `GYMBAG_SSH_VERIFY_LEVEL` / `GYMBAG_SSH_PROTECT` | SSH 驗證等級（`0`–`3` 或 `off`）與防護開關（`on`/`off`） |
| `GYMBAG_MC_ENABLED` / `GYMBAG_MC_PORT` / `GYMBAG_MC_UPSTREAM` | 啟用 Minecraft 防護（預設 `25565`）並設定後端目標（支援 `GYMBAG_MINECRAFT_*` 別名） |
| `GYMBAG_MC_SERVER_JAR` / `GYMBAG_MC_JAVA` | Minecraft `server.jar` 路徑（會自動挑選對應 Java 並啟動伺服器；別名 `GYMBAG_MC_JAR`）與 Java 執行檔覆寫（別名 `GYMBAG_MC_JAVA_PATH`） |
| `GYMBAG_MC_JVM_ARGS` / `GYMBAG_MC_WORK_DIR` / `GYMBAG_MC_AUTO_RESTART` | 以空格分隔的 JVM 參數、Minecraft 工作目錄，以及異常中止時自動重啟開關（`true`/`false`，預設 `true`） |
| `GYMBAG_MC_CONSOLE` | 將託管的 Minecraft 伺服器主控台整合至 Gymbag 前景與管理 API（`on`/`off`，別名 `GYMBAG_MINECRAFT_CONSOLE`） |
| `GYMBAG_MC_VERIFY_LEVEL` / `GYMBAG_MC_PROTECT` | Minecraft 驗證等級（`0`–`3` 或 `off`）與防護開關（`on`/`off`） |
| `GYMBAG_AUTHELIA_URL` / `GYMBAG_AUTHELIA_PORTAL_URL` | 全域 Authelia 前置驗證（Forward Auth）URL 與登入入口重導向 URL（支援 `GYMBAG_AUTHELIA_URI`、`GYMBAG_AUTHELIA_COPY_HEADERS`、`GYMBAG_AUTHELIA_TIMEOUT`、`GYMBAG_AUTHELIA_INSECURE_TLS`、`GYMBAG_AUTHELIA_DOMAINS`、`GYMBAG_FORWARD_AUTH_URL`） |
| `GYMBAG_MASCOT` | 全域吉祥物圖片路徑、目錄或 `http(s)://` URL（可使用 `GYMBAG_MASCOT_PENSIVE`、`GYMBAG_MASCOT_HAPPY`、`GYMBAG_MASCOT_REJECT` 個別覆蓋各狀態圖片） |
| `GYMBAG_REDIS_URL` | Redis 連線 URL（`redis://` 或 `rediss://`） |
| `GYMBAG_SECRET` / `GYMBAG_ADMIN` | HMAC 通行憑證密鑰與管理 REST API 監聽位址（`127.0.0.1:2019` 或 `off`） |
