# 兩道指令快速上手

Gymbag 僅使用 Go 標準函式庫即可編譯成單一執行檔。HTML 範本、SHA-256 瀏覽器解題腳本與吉祥物 PNG 圖片皆透過 `go:embed` 直接內嵌其中。

## 安裝

每次在 [GitHub Releases](https://github.com/Evil-Project/gymbag/releases) 發布版本標籤，皆會提供 `CGO_ENABLED=0` 靜態執行檔（`linux`、`darwin`、`windows` 的 `amd64`/`arm64`）、Debian/Ubuntu `.deb` 套件、Arch Linux `.pkg.tar.zst` 套件、NixOS bundle 以及多架構 GHCR 容器映像檔（`ghcr.io/evil-project/gymbag`）：

::: code-group
```bash [Debian / Ubuntu (.deb)]
# 安裝釋出的 .deb 套件（會自動設定 cap_net_bind_service 與 systemd 服務檔）
sudo apt install ./gymbag_<version>_linux_amd64.deb

# 產生設定檔並啟用服務
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
# 安裝釋出的 .pkg.tar.zst 套件
sudo pacman -U gymbag-<version>-1-x86_64.pkg.tar.zst

# 或從 packaging/arch 自行建置
git clone https://github.com/Evil-Project/gymbag.git
cd gymbag/packaging/arch
makepkg -si
```

```bash [Nix / NixOS]
nix run github:Evil-Project/gymbag -- --help
nix profile install github:Evil-Project/gymbag

# 靜態編譯的 Linux 釋出執行檔（CGO_ENABLED=0）亦可在 NixOS 上直接執行
```

```bash [從原始碼編譯 (Go)]
git clone https://github.com/Evil-Project/gymbag.git
cd gymbag
go build -o gymbag ./cmd/gymbag
```
:::

關於容器部署、Compose + Redis 架構與磁碟區權限說明，請參閱 [Docker 與 Docker Compose 指南](/zh-TW/guide/docker)。

## 兩道指令立即啟動

`gymbag init` 預設會產生 `gymbag.json`，若加上 `--format gymbagfile`（或 `-o Gymbagfile`）則會產生 `Gymbagfile`。所有產生的設定檔在寫入磁碟前都會先經過驗證。

### HTTP / HTTPS 網站

```bash
# 1. 產生 gymbag.json（加上 --format gymbagfile 可改用 Caddy 語法）
./gymbag init -d app.example.com -u http://localhost:3000

# 2. 啟動 Gymbag（會依序自動載入 ./gymbag.json、./Gymbagfile、./Caddyfile）
./gymbag
```

### SSH 防護

此指令會將 `/etc/ssh/sshd_config` 中的 `sshd` 移至 `20000–59999` 之間的隨機可用連接埠（並保留 `.gymbag.bak` 備份檔），透過 `sshd -t` 驗證設定（若失敗則自動還原），重新載入 `sshd`，並更新主機防火牆（`ufw`、`firewalld`、`nftables` 或 `iptables`+`ip6tables`），讓外部連線只能經由埠號 `22` 上的 Gymbag 進入：

```bash
# 在遠端主機執行時，請務必保持另一個 SSH 終端機連線開啟
sudo ./gymbag init -t ssh
sudo ./gymbag
```

### Minecraft 伺服器防護

設定 `--server-jar` 後，Gymbag 會在啟動防護代理的同時**自動啟動你的 Minecraft 伺服器**，並**自動挑選機器上最合適的 Java 執行環境**（透過檢查 `version.json`、bundler 巢狀 JAR、`patch.properties` 或 `.class` 位元組碼標頭來區分舊版 Java 8 與現代 Java 17/21+ 伺服器，並在關閉時傳送優雅的 `stop` 主控台指令）。Gymbag 也會將 `server.properties` 中的 `server-port` 改為 `20000–59999` 之間的隨機可用連接埠（若設定 `server_jar` 且檔案不存在時會自動建立，並將空白/`0.0.0.0`/`::` 的 `server-ip` 綁定為 `127.0.0.1`），同時原封不動轉發 Server List Ping，並在玩家登入時於 Minecraft 的 **「Connection Lost」** 畫面顯示 HTTPS 驗證連結：

```bash
# 自動挑選 Java + 隨機後端連接埠並啟動 server.jar：
./gymbag init -t minecraft --server-jar /srv/mc/paper.jar --mc-jvm-args "-Xms1G -Xmx4G"
./gymbag

# 或透過 server.properties 保護已執行的伺服器：
./gymbag init -t minecraft --server-properties /path/to/server.properties
./gymbag
```

### 多合一模式（`http,ssh,minecraft`）或互動式精靈

單一處理程序即可同時保護網站、SSH 與 Minecraft：

```bash
# 非互動式組合設定：
sudo ./gymbag init -t http,ssh,minecraft -d app.example.com -u :3000 --server-jar /srv/mc/paper.jar
sudo ./gymbag

# 或執行互動式設定精靈：
./gymbag init
./gymbag
```

> **驗證入口與 IP SSL 憑證：** SSH 與 Minecraft 預設共用埠號 `8443` 的 HTTPS 驗證入口（`--verify-port`）。你可以傳入 `--verify-url https://verify.example.com:8443` 使用自訂網域，或留空以伺服器偵測到的 IP 搭配 RFC 8738 ACME IP 憑證提供服務（若 HTTP-01 無法連線，則自動退回使用 Gymbag 本機 CA 或自簽 IP 憑證）。詳情請參閱 [SSH 與 Minecraft 防護指南](/zh-TW/guide/ssh-minecraft)。

### 本機開發（`*.internal`）

```bash
# 1. 產生 app.internal -> http://localhost:3000（使用本機 PKI）
./gymbag init --preset local

# 2. 以非特權連接埠執行（或使用 sudo 綁定 80/443 並管理 /etc/hosts）
./gymbag --http-port 8080 --https-port 8443
```

### 註冊為系統服務

```bash
./gymbag init -d app.example.com -u :3000 --service auto
./gymbag service start
```

關於 `systemd`、`launchd`、`openrc` 與 `supervisor` 的設定細節，請參閱 [系統服務設定](/zh-TW/guide/system-service)。

## 匯入現有的 nginx / Apache / Caddyfile 設定檔

Gymbag 可直接解析原生的 `nginx.conf`、Apache `httpd.conf` / `apache2.conf` 與 Caddyfile v2 設定檔：

```bash
./gymbag run -c /etc/nginx/nginx.conf
./gymbag config convert -c Caddyfile -o gymbag.json --format json
```

關於路徑路由、靜態檔案、重新導向與轉換細節，請參閱 [路由與設定檔匯入](/zh-TW/guide/routes-and-imports)。

## 臨時反向代理（無須設定檔）

```bash
./gymbag reverse-proxy --from app.internal --to :3000 --verify 3 --verify-timeout 12h
```

## `gymbag init` 參數列表

| 參數 | 說明 |
| :--- | :--- |
| `-o`, `--output <path>` | 輸出路徑（預設為 `gymbag.json`，搭配 `--format gymbagfile` 則為 `Gymbagfile`） |
| `--format <fmt>` | `json`（預設）或 `gymbagfile` |
| `-t`, `--template <list>` | 服務範本：`http`、`ssh`、`minecraft` 或 `all`（可用逗號分隔或重複指定） |
| `--preset <name>` | `standard`、`local`、`public`、`strict`、`ai-only`、`suspicious`、`proxy-only`、`http`、`ssh`、`minecraft` 或 `all` |
| `-d`, `--domain <domains>` | 下游網域（可用逗號分隔） |
| `-u`, `--upstream <url>` | 上游網址（例如 `http://localhost:3000` 或 `:3000`） |
| `--site <domain=url>` | 新增多個站台設定（可重複指定） |
| `--ssh` / `--ssh-port <port>` | 啟用 SSH 防護（預設連接埠 `22`） |
| `--ssh-upstream <addr>` / `--sshd-config <p>` | 指定 `sshd` 目標位址或 `sshd_config` 路徑（預設 `/etc/ssh/sshd_config`） |
| `--minecraft`, `--mc` / `--mc-port <port>` | 啟用 Minecraft 防護（預設連接埠 `25565`） |
| `--mc-upstream <addr>` / `--server-properties <p>` | 指定 Minecraft 後端位址或 `server.properties` 路徑 |
| `--mc-jar`, `--server-jar <path>` | Minecraft 伺服器 `.jar` 路徑（會自動挑選 Java 並啟動伺服器） |
| `--mc-java`, `--java <bin>` | 指定 Minecraft 伺服器使用的 Java 執行檔路徑（預設為自動挑選） |
| `--mc-jvm-args <args>` / `--mc-work-dir <dir>` | 託管 Minecraft 伺服器的 JVM 參數（以空白分隔）與工作目錄 |
| `--verify-url <url>` | 共用的 HTTPS 驗證入口網址（留空則使用伺服器 IP + IP SSL 憑證） |
| `--verify-port <port>` | 共用的 HTTPS 驗證入口連接埠（預設 `8443`） |
| `--no-auto-setup` / `--no-firewall` | 跳過自動修改 `sshd_config`/防火牆或 `server.properties` |
| `-l`, `--verify <0-3\|off>` | 驗證等級（`0`、`1`、`2`、`3` 或 `off`） |
| `--difficulty <1-5>` | SHA-256 工作量證明難度（預設 `4`） |
| `--verify-timeout <dur>` | 通行憑證有效期限（`30m`、`12h`、`7d` 或秒數；別名 `--clearance-ttl`） |
| `--remote-rules <url>` | 遠端防火牆規則網址（可重複指定或以逗號分隔） |
| `--remote-rules-refresh <dur>` | 遠端規則更新週期（預設 `5m`） |
| `--rules-file <path>` / `--rules-dir <path>` | 本機規則檔案或目錄（可重複指定） |
| `--rules-dynamic` / `--rules-watch-interval <dur>` | 監控本機規則來源的變動（預設 `2s`） |
| `--no-protect` / `--no-default-rules` | 停用工作量證明驗證或內建基礎規則 |
| `--ssl-provider <name>` | `letsencrypt`、`zerossl`、`google`、`ssl.com`、`litessl`、`buypass`、`local` 或網址 |
| `--email <addr>` | ACME 聯絡信箱（同時啟用 ZeroSSL 自動 EAB） |
| `--eab-kid` / `--eab-hmac-key` | ACME External Account Binding 憑證 |
| `--http-port` / `--https-port` | 自訂 HTTP/HTTPS 連接埠（預設 `80` / `443`） |
| `--bind <ip>` | 綁定的 IP 位址（省略時自動選擇） |
| `--admin <addr>` | 啟用本機 REST Admin API（例如 `127.0.0.1:2019`） |
| `--cert-dir <dir>` | 存放憑證、本機 CA 與快取的目錄（預設 `certs`） |
| `--redis <url>` | Redis 連線網址，用於共享挑戰 Nonce、速率限制與 SSH/MC 驗證狀態 |
| `--ip-security-action <act>` | 命中威脅情資時的處置動作（`verify` 或 `block`） |
| `--no-auto-hosts` | 停用自動管理本機網域的 `/etc/hosts` |
| `-y`, `--yes` / `-i`, `--interactive` | 略過精靈提示或強制進入互動式精靈 |
| `--dry-run` / `-f`, `--force` | 僅將設定印出至 stdout 不寫入檔案 / 覆寫現有檔案 |
| `--service <type>` | 註冊作業系統服務（`auto`、`systemd`、`launchd`、`openrc`、`supervisor`） |

## 免寫規則即可生效的防護機制

只要寫上單純的 `verify`（預設為等級 `3`：`suspicious & ai`）或 `block`，以下三層防護就會立即啟用：

1. **內建基礎規則（`default_rules on`）**：對 `/.env`、`/.git/*`、`/.aws/*`、`/wp-admin/*`、`sqlmap`/`nikto`/`nuclei` 與 SQLi/XSS 查詢探測直接回傳 `403`，並將 CLI/無頭 HTTP 用戶端導向工作量證明驗證頁。
2. **威脅情資、即時 IP 信譽與 VPN/Tor 偵測**：於啟動時及每 6 小時更新一次 IPsum（level 3+）、Spamhaus DROP v4/v6、Feodo Tracker、X4BNet VPN 清單與 Tor 出口節點，並以 fail-open 逾時機制查詢 Shodan InternetDB、StopForumSpam、`ipquery.io` 與 `ipapi.is`。VPN/代理/Tor IP 在 HTTP、SSH 與 Minecraft 的等級 `1` 與 `3` 皆會被視為可疑流量。
3. **官方 AI 爬蟲 IP 清單**：比對 OpenAI、Anthropic、Perplexity、Google 與 Apple 公開的 CIDR 網段，即使爬蟲偽造 Chrome 的 `User-Agent` 也能準確識別。
