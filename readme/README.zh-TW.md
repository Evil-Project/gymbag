# Gymbag

<p align="center">
  <img src="../docs/public/mascot-happy.png" alt="Gymbag mascot" width="140" />
</p>

<p align="center">
  <strong>純 Go 打造的單一執行檔 HTTP/HTTPS、SSH 與 Minecraft 工作量證明防護閘道</strong>
</p>

<p align="center">
  <a href="https://github.com/Evil-Project/gymbag/releases"><img src="https://img.shields.io/github/v/release/Evil-Project/gymbag?include_prereleases&label=Release" alt="Release" /></a>
  <a href="https://github.com/Evil-Project/gymbag/pkgs/container/gymbag"><img src="https://img.shields.io/badge/Docker-GHCR-blue?logo=docker" alt="Docker GHCR" /></a>
  <a href="../go.mod"><img src="https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go" alt="Go 1.27+" /></a>
  <a href="../LICENSE"><img src="https://img.shields.io/badge/License-Apache--2.0-green" alt="Apache-2.0 License" /></a>
  <a href="https://evil-project.github.io/gymbag/zh-TW/"><img src="https://img.shields.io/badge/Docs-VitePress-b81d33" alt="Docs" /></a>
</p>

<p align="center">
  <strong>README:</strong>
  <a href="../README.md">English</a> ·
  <a href="README.zh-CN.md">简体中文</a> ·
  <a href="README.zh-TW.md">繁體中文</a> ·
  <a href="README.ja.md">日本語</a> ·
  <a href="README.ko.md">한국어</a> ·
  <a href="README.es.md">Español</a>
  <br />
  <sub>線上文件：<a href="https://evil-project.github.io/gymbag/">EN</a> · <a href="https://evil-project.github.io/gymbag/zh/">ZH</a> · <a href="https://evil-project.github.io/gymbag/zh-TW/">ZH-TW</a> · <a href="https://evil-project.github.io/gymbag/ja/">JA</a> · <a href="https://evil-project.github.io/gymbag/ko/">KO</a> · <a href="https://evil-project.github.io/gymbag/es/">ES</a></sub>
</p>

Gymbag 是一款以純 Go 編寫的單一執行檔 HTTPS 反向代理與工作量證明（Proof-of-Work）WAF（靈感來自 Anubis），同時也能守護 SSH 與 Minecraft 伺服器。它完全只依賴 Go 標準函式庫，零第三方 Go 套件；所有 HTML 頁面、瀏覽器端 SHA-256 求解器與吉祥物圖檔皆透過 `go:embed` 直接編譯進執行檔中。

自建服務、Git 託管平台、SSH 連接埠與 Minecraft 伺服器往往面臨同一批惡意流量：無視 `robots.txt` 的 LLM 爬蟲、瘋狂探測 `/.env` 與 `/wp-admin` 的掃描器、SSH 暴力破解以及登入洪水攻擊。與其把 nginx 或 Caddy、PoW Sidecar、fail2ban 和一堆自訂防火牆腳本拼湊在一起，不如讓 Gymbag 在單一程序內一次搞定 TLS 終止、多 CA ACME、HTTP/WebSocket 代理、SSH/Minecraft TCP 閘道防護、IP 威脅／VPN／AI 情資清單以及防火牆規則，並支援 `gymbag.json` 或類 Caddy 風格的 `Gymbagfile` 設定。

## 快速開始

[GitHub Releases](https://github.com/Evil-Project/gymbag/releases) 上提供預先編譯的靜態執行檔（`CGO_ENABLED=0`，涵蓋 Linux、macOS、Windows、FreeBSD、OpenBSD 與 NetBSD 的 `amd64`/`arm64`）、Debian/Ubuntu `.deb` 套件、Arch `.pkg.tar.zst` 套件以及 NixOS tarball/NAR。您也可以直接執行 GHCR 容器映像、使用 Nix flake 或從原始碼建置：

```bash
go build -o gymbag ./cmd/gymbag
```

**Debian / Ubuntu：**
```bash
sudo apt install ./gymbag_<version>_linux_amd64.deb
sudo gymbag init -o /etc/gymbag/Gymbagfile --format gymbagfile
sudo systemctl enable --now gymbag
```

**Arch Linux：**
```bash
sudo pacman -U gymbag-<version>-1-x86_64.pkg.tar.zst
# 或從 packaging/arch 建置：
cd packaging/arch && makepkg -si
```

**Nix / NixOS：**
```bash
nix run github:Evil-Project/gymbag -- --help
nix profile install github:Evil-Project/gymbag
```
靜態 Linux 發行版執行檔無須 `patchelf` 或 `nix-ld` 即可直接在 NixOS 上運行，且 `nixosModules.default` 提供了宣告式的 `services.gymbag` 模組。

只需兩個指令即可上手：先產生設定檔（預設寫入 `gymbag.json`，加上 `--format gymbagfile` 則產生 `Gymbagfile`），接著啟動服務：

```bash
# 保護 HTTP/HTTPS 網站
./gymbag init -d app.example.com -u http://localhost:3000
./gymbag

# 保護 SSH（自動將 sshd 移至隨機連接埠並設定防火牆）
sudo ./gymbag init -t ssh
sudo ./gymbag

# 保護 Minecraft Java 版（自動挑選對應 Java 啟動 server.jar 並隨機化連接埠）
./gymbag init -t minecraft --server-jar /srv/mc/paper.jar --mc-jvm-args "-Xms1G -Xmx4G"
./gymbag

# 保護既有的 Minecraft 伺服器而不交由 Gymbag 啟動（自動隨機化 server.properties 連接埠）
./gymbag init -t minecraft --server-properties /path/to/server.properties
./gymbag

# 在單一程序內同時保護以上三種服務
sudo ./gymbag init -t http,ssh,minecraft -d app.example.com -u :3000 --server-jar /srv/mc/paper.jar
sudo ./gymbag
```

您也可以直接將 Gymbag 指向既有的 **nginx**（`nginx.conf`，包含 `auth_request`）、**Apache**（`httpd.conf` / `apache2.conf`）或 **Caddyfile v2**（`forward_auth` / `authelia`）設定檔，或是將其註冊為系統服務（`systemd`、`launchd`、`openrc`、`supervisor`、`freebsd-rc`、`openbsd-rc` 或 `netbsd-rc`）：

```bash
./gymbag init -d app.example.com -u :3000 --service auto
./gymbag service start
```

若只是想快速進行一次性測試而不建立任何設定檔：

```bash
./gymbag reverse-proxy --from app.internal --to :3000 --verify 3
```

> **1024 以下的特權連接埠：** `:80` 用於處理 HTTP 轉 HTTPS 重導向與 ACME HTTP-01，`:443` 提供 HTTPS 服務，`:22` 則監聽 SSH。在 Linux 上，執行 `sudo setcap 'cap_net_bind_service=+ep' ./gymbag` 即可讓 Gymbag 免去 root 權限綁定這些連接埠（`.deb` 與 Arch 套件已自動為您設定此權限）。若要在無特權的本機環境測試網頁，可使用 `--http-port 8080 --https-port 8443`。

### 不寫任何規則也會自動運作的防護層

您無須先撰寫正規表示式或匯入封鎖清單，就能直接將 Gymbag 放在流量最前線。只要寫上 `verify`（預設為等級 `3`：`suspicious & ai`）或單獨寫 `block`（將威脅情資命中從 PoW 挑戰改為直接回傳 `403`），就會自動啟用三層預設防護：

1. **內建基礎規則（`default_rules on`）**
   - **敏感路徑（`403`）**：`/.env`、`/.env.*`、`/.git`、`/.git/*`、`/.svn/*`、`/.hg/*`、`/.DS_Store`、`/.aws/*`、`/.ssh/*`、`/docker-compose.yml`、`/docker-compose.yaml`、`*.bak`、`*.sql`、`*.sqlite`。
   - **CMS 與框架探測（`403`）**：`/wp-admin`、`/wp-admin/*`、`/wp-login.php`、`/xmlrpc.php`、`/phpmyadmin*`、`/vendor/phpunit*`、`/cgi-bin/*`、`/actuator/env*`、`/actuator/heapdump*`。
   - **弱點掃描器（`403`）**：`sqlmap`、`nikto`、`nuclei`、`masscan`、`zgrab`、`gobuster`、`dirbuster`、`ffuf`、`wfuzz`、`feroxbuster`、`acunetix`、`nessus`、`openvas`。
   - **注入與目錄遍歷查詢（`403`）**：查詢字串中的 `UNION SELECT`、`<script`、`javascript:`、`/etc/passwd`、`information_schema`、`../`、`cmd.exe`、`/bin/sh`，並搭配請求結構檢查以攔截空位元組（null byte）與 `%2e%2e` 遍歷攻擊。
   - **自動化用戶端（PoW 挑戰）**：當站台尚未設定任何自訂規則時，CLI 函式庫（`curl`、`wget`、`python-requests`、`python-httpx`、`go-http-client`、`aiohttp`、`okhttp`、`axios`、`node-fetch`、`scrapy`）、無頭瀏覽器（`headlesschrome`、`playwright`、`puppeteer`、`selenium`、`phantomjs`）以及空白 `User-Agent` 標頭都會被導向工作量證明頁面。
2. **自動更新的 IP 威脅情資與 VPN/Tor 偵測**
   - 啟動時與每 6 小時自動抓取 [IPsum (level 3+)](https://raw.githubusercontent.com/stamparm/ipsum/master/levels/3.txt)、[Spamhaus DROP IPv4](https://www.spamhaus.org/drop/drop_v4.json) / [IPv6](https://www.spamhaus.org/drop/dropv6.txt) 以及 [abuse.ch Feodo Tracker](https://feodotracker.abuse.ch/downloads/ipblocklist.txt)，並透過 `ETag` 與 `If-Modified-Since` 快取於 `<cert_dir>/intel-cache`。
   - 對 **Shodan InternetDB**（`internetdb.shodan.io`）與 **StopForumSpam**（`api.stopforumspam.org`）執行免金鑰的即時信譽查詢，具備 1 小時記憶體快取與 Fail-Open 逾時機制，確保上游服務緩慢時不會誤擋正常訪客。
   - **免費 VPN/Proxy/Tor 偵測（`vpn_detection on`）**：載入自動更新的 X4BNet VPN IPv4/IPv6 清單與 Tor 出口節點清單，並搭配免金鑰即時查詢（`api.ipquery.io` 與 `api.ipapi.is`，快取 1 小時，1.2 秒 Fail-Open 逾時）。命中的 IP 在 HTTP、SSH 與 Minecraft 的驗證等級 `1` 與 `3` 中都會被標記為**可疑（suspicious）**。
3. **官方 AI 爬蟲 IP 清單**
   - 持續追蹤 **OpenAI**（`gptbot`、`searchbot`、`chatgpt-user`）、**Anthropic**（`claude.com/crawling/bots.json`）、**Perplexity**（`perplexitybot`、`perplexity-user`）、**Google**（`user-triggered-agents`、`user-triggered-fetchers-google`）與 **Apple**（`applebot`）公開的 CIDR 清單，即使爬蟲偽造 Chrome `User-Agent` 也能依來源 IP 精準識別。

## SSH 與 Minecraft 防護

當未經驗證的 SSH 或 Minecraft 連線連入 Gymbag 並觸發設定的驗證等級時，Gymbag 不會將其轉發給後端服務或遊戲伺服器，而是核發一個單次使用的 HTTPS 驗證連結（`/v/<token>`）——SSH 會將連結印在斷線訊息中；Minecraft 1.20.5+ 則會讓玩家進入一個純記憶體建構的空世界，在聊天室提供可點擊的驗證連結，並在驗證完成後自動透過 `Transfer` 封包導向真正的伺服器：

- **SSH（預設 `port` `22`）**：`sudo ./gymbag init -t ssh` 會在 `20000–59999` 之間挑選一個未使用的本機連接埠、將 `/etc/ssh/sshd_config` 備份至 `/etc/ssh/sshd_config.gymbag.bak`、在任何 `Match` 區塊之前寫入新的 `Port`、執行 `sshd -t`（若驗證失敗則自動回滾）、重新載入 `sshd`（透過 `systemctl`/`service`/`rc-service`/`rcctl`），並更新系統上的 Linux 或 BSD 防火牆（`ufw`、`firewalld`、`nftables`、`iptables`+`ip6tables`、`pf`、`ipfw` 或 `npf`），確保後端連接埠僅限 loopback 存取，同時保持 `22` 與 `8443` 對外開放。（變更 `sshd_config` 時建議保留第二個 SSH 連線視窗以防萬一，或傳入 `--ssh-upstream 127.0.0.1:2222`、`--no-auto-setup` 或 `--no-firewall` 手動處理。）IP 通過瀏覽器挑戰後，後續連線會直接透過原始 TCP 轉發，完全不干擾 SSH 交握。在等級 `1` 與 `3` 時，格式異常的 Banner 或腳本化 SSH 用戶端（`libssh`、`paramiko`、`zgrab`、`masscan`、`nmap`、`asyncssh`、`ssh2go`、`jsch`、`hydra`、`medusa`、`patator`、`python`、`curl`、`ssh-2.0-go`）會被要求驗證；其餘連線則會在終端機直接看到帶有驗證 URL 的 RFC 4253 `SSH_MSG_DISCONNECT` 訊息。
- **Minecraft Java 版（預設 `port` `25565`）**：`./gymbag init -t minecraft --server-properties ./server.properties` 會將 `server.properties` 備份為 `server.properties.gymbag.bak`、將 `server-port` 改為 `20000–59999` 之間的隨機可用連接埠，並將空白或萬用字元的 `server-ip` 設為 `127.0.0.1`。若加上 `--server-jar /srv/mc/paper.jar`，Gymbag 還會自動挑選合適的 Java 啟動該 JAR；再搭配 `--mc-console`（`console on` / `"console": true`），即可將即時 Minecraft 伺服器主控台整合進 Gymbag 互動式終端機、`gymbag console` 與 `/api/v1/minecraft/console`。除非被規則明確封鎖，Server List Ping（`nextState == 1` 與舊版 `0xFE`）仍會直接穿透，讓玩家在伺服器列表正常看到 MOTD 與線上人數。在 Login 或 Transfer 階段若需要驗證，支援的用戶端（`1.20.5`–`26.1`）會進入一個極小型的純記憶體空世界（不會在磁碟寫入任何檔案），在聊天室收到可點擊的驗證連結，驗證通過後立即透過 `Transfer` 自動導向真正的伺服器（較舊版本的用戶端則會退回顯示 **"Connection Lost"** 斷線對話框）。
- **共用 HTTPS 驗證入口（預設 `:8443`）**：SSH 與 Minecraft 共用同一個 `verify_port`（可透過 `--verify-url https://verify.example.com:8443` 指定自訂網域，或留空以自動使用偵測到的伺服器 IP）。當透過 ACME 使用伺服器 IP 時，Gymbag 會申請 **RFC 8738** IP 憑證（若 CA 支援則優先使用 `shortlived` Profile），若無法通過 HTTP-01 驗證則會自動退回使用 Gymbag 本機 CA 或自簽 IP 憑證。若 `verify_port` 與 `https_port` 相同且已設定 HTTP 網站，驗證入口會共用 `443` 連接埠。

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

完整細節與 `gymbag.json` 範例請參閱 [SSH 與 Minecraft 防護指南](../docs/zh-TW/guide/ssh-minecraft.md)。

## 本機開發（`localhost` 與 `*.internal`）

對於任何位於 `localhost`、`*.localhost`、`*.internal`（如 `app.internal`、`api.internal`）、`*.local` 或 `*.test` 的網站，Gymbag 會自動管理 `/etc/hosts` 與本機 TLS 憑證：

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

### 自動管理 `/etc/hosts` 項目

當 `auto_hosts on`（預設開啟）時，Gymbag 會在啟動時檢查 `/etc/hosts`（Windows 上為 `C:\Windows\System32\drivers\etc\hosts`），並為尚未存在的本機網域補上 IPv4 與 IPv6 loopback 紀錄：

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

既有的 `localhost` 項目不會受到影響。在收到 `Ctrl+C` 或 `SIGTERM` 結束時，Gymbag 會重新讀取該檔案（保留您期間手動編輯的內容），並僅刪除帶有 `# gymbag-managed:<instanceID>` 標記的行。在 macOS/Linux 上寫入 `/etc/hosts` 需要 root 權限；若以一般使用者權限執行，Gymbag 會記錄警告並繼續運行。若您透過 `dnsmasq` 或手動管理 `.internal` 網域，請設定 `auto_hosts off`（`GYMBAG_AUTO_HOSTS=off`）。

### 本機 CA

與其每次重啟都重新產生一次性的自簽憑證，Gymbag 會在 `<cert_dir>/ca/`（預設 `certs/ca/`）下維護一套兩層式 ECDSA P-256 CA 架構：

1. **Root CA**：`"Neuro-sama Headquarters Root CA"`（`certs/ca/neuro-sama-root-ca.pem`，10 年效期，`MaxPathLen: 1`）
2. **Intermediate CA**：`"Gymbag & Swarm SSL Local CA"`（`certs/ca/gymbag-swarm-local-ca.pem`，5 年效期，`MaxPathLen: 0`）
3. **終端憑證（Leaf Certificate）**：`certs/local-fullchain.pem` + `certs/local-privkey.pem`（90 天效期，到期前 30 天自動續期，SAN 包含所有網域以及 `127.0.0.1` 與 `::1`）

只需信任 `"Neuro-sama Headquarters Root CA"` 一次，瀏覽器與 `curl` 便會信任所有本機網站：

```bash
# macOS
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain certs/ca/neuro-sama-root-ca.pem

# Debian / Ubuntu
sudo cp certs/ca/neuro-sama-root-ca.pem /usr/local/share/ca-certificates/neuro-sama-root-ca.crt && sudo update-ca-certificates

# Fedora / RHEL / Arch Linux
sudo trust anchor --store certs/ca/neuro-sama-root-ca.pem
```

> **Firefox：** 請在 macOS/Windows（以及搭配 `p11-kit` 的 Linux）的 `about:config` 中將 `security.enterprise_roots.enabled` 設為 `true`，或在 *設定 -> 隱私權與安全性 -> 憑證 -> 檢視憑證 -> 憑證機構* 中匯入一次 `certs/ca/neuro-sama-root-ca.pem`。

## 多站台與驗證等級（`0`–`3`）

單一程序即可同時處理多個網域，支援輪詢（Round-Robin）上游負載平衡與 RFC 6455 WebSocket 升級。每個站台（以及 SSH/Minecraft 服務）皆可各自選擇驗證等級（`0`–`3`），或使用 `protect off` 關閉 PoW 驗證：

| 等級 | 名稱 | 觸發挑戰對象 | 典型應用場景 |
| :---: | :--- | :--- | :--- |
| `0` | `always` | 所有尚未持有有效通行 Cookie 或已驗證 IP 的訪客 | 私有管理後台、測試環境、L7 洪水攻擊緩解 |
| `1` | `only suspicious` | 威脅情資清單、即時高風險 IP、VPN/Proxy/Tor IP、掃描器、CLI 函式庫、無頭瀏覽器、缺少／偽造標頭、漏洞探測 | 不希望一般瀏覽器訪客看到驗證頁面的公開應用與 API |
| `2` | `only ai` | 官方 AI 爬蟲 IP 網段或帶有 AI 機器人 `User-Agent` / `X-Ai-Agent` / `Sec-Agent` 標頭的請求 | 希望一般人類與 `curl` 都能暢行無阻的文件站、部落格與 Git 平台 |
| `3` | `suspicious & ai` | 結合 `1` 與 `2`（單獨寫 `verify`、SSH 與 Minecraft 的預設值） | 公開正式環境網站 |
| n/a | `protect off` | 不進行 PoW 挑戰；純反向代理（明確定義的自訂規則仍會執行） | 靜態資源主機、Webhook、鏡像站 |

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

解開 SHA-256 挑戰（`difficulty` `1`–`5`，預設 `4`）後，系統會簽發綁定主機與用戶端 IP 的 `HttpOnly`、`SameSite=Lax` HMAC 通行 Cookie（`waf_clearance`，預設有效時間 `24h` / `86400s`）。您可透過 `verify_timeout` / `clearance_ttl`（秒數或時間長度如 `30m`、`12h`、`1h30m`、`7d`、`2w`）、`--verify-timeout`、`GYMBAG_VERIFY_TIMEOUT` 或 `PATCH /api/v1/sites/{domain}` 全域或針對單一站台自訂有效時間。對已通過驗證的訪客，`GET /.waf/v1/session` 會回傳 `clearance_ttl`、`expires_at` 與 `expires_in`。

## 路徑路由、靜態檔案、重導向與設定匯入

站台除了 `reverse_proxy` 外，也能定義依序比對的**路徑路由（Path Routes）**。第一個符合條件的路由會處理該請求；未符合任何路由的路徑則會退回交給站台的預設上游（若未設定預設上游則回傳 `404`）。每個路由皆配置一個處理器：反向代理、靜態檔案伺服器（`root`）、靜態回應（`respond`）或重導向（`redir`）。

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

路徑比對器支援精確路徑（`/exact`）、萬用字元（`/api/*`、`*.php`）或 Go RE2 正規表示式（`~^/v[0-9]+/`）。重導向支援展開 `{scheme}`、`{host}`、`{uri}`、`{path}` 與 `{query}`。靜態路由支援 `index`、`browse` 與 SPA `fallback`。`header` 可設定回應標頭，或在標頭名稱前加上 `-` 來移除標頭。防火牆規則與驗證永遠會在任何路由處理器之前執行。

在 `gymbag.json` 中對應的 `routes` 陣列寫法如下：

```json
{
  "routes": [
    {"path": "/static/*", "strip_prefix": "/static", "root": "./dist", "browse": true},
    {"path": "/old/*", "redirect": {"to": "https://{host}/new{uri}", "code": 301}},
    {"path": "/api/*", "upstreams": ["http://localhost:8080"]}
  ]
}
```

可透過 `gymbag config route list|add|update|remove` 或 `PATCH /api/v1/sites/{domain}` 管理路由。若要整合 **Authelia** / 前置驗證（Forward Auth），只需在全域、站台內或 `handle` 路由中加入 `authelia http://127.0.0.1:9091`（或 Caddy v2 的 `forward_auth`）——WAF 規則與挑戰仍會優先執行，通過驗證的流量在送往上游前會先向 Authelia 進行身分驗證，並注入 `Remote-User`、`Remote-Groups`、`Remote-Name` 與 `Remote-Email` 標頭，同時剝除用戶端偽造的身分標頭。

**匯入既有設定：** `gymbag run -c /etc/nginx/nginx.conf`（或 `httpd.conf` / `apache2.conf`）可直接載入原生的 nginx 或 Apache 設定檔執行；而 `gymbag config convert -c <file> -o gymbag.json --format json` 則支援將 nginx（包含 `auth_request` / `auth_request_set`）、Apache 與 Caddyfile v2（包含 snippets、`import`、`@matchers`、`handle`、`route`、`forward_auth`、`file_server`、`respond`、`redir` 與 `header`）轉換為 Gymbag 設定格式。

## SSL 供應商、自訂金鑰與自動續期

公開網域預設採用 `tls auto`，透過 ACME HTTP-01 向 Let's Encrypt 申請憑證。可透過 `ssl_provider`（別名：`acme_provider`、`ca`、`cert_issuer`）或 `tls <provider>` 切換 CA：

| 供應商關鍵字 | 憑證頒發機構（CA） | EAB 需求 |
| :--- | :--- | :--- |
| `letsencrypt`（`le`） | Let's Encrypt（正式環境） | 無 |
| `letsencrypt_staging` | Let's Encrypt（測試環境） | 無 |
| `zerossl` | ZeroSSL | 提供 `email` 即可自動取得，或明確指定 `eab <kid> <hmac_key>` |
| `google`（`gts`） | Google Trust Services | 需要 `eab <kid> <hmac_key>` |
| `ssl.com`（`sslcom`、`ssl.com-rsa`） | SSL.com（ECC 或 RSA） | 需要 `eab <kid> <hmac_key>` |
| `litessl`（`trustasia`） | LiteSSL by TrustAsia | 標準 DV 憑證無須 EAB |
| `buypass` | Buypass Go SSL | 無 |
| `local` | 本機兩層式 PKI | 本機 CA（`Neuro-sama Headquarters Root CA`） |
| `https://...` | 任何 RFC 8555 ACME v2 CA | 選用 `eab <kid> <hmac_key>`（例如 `step-ca`、Pebble） |

針對 **ZeroSSL**，只要設定 `email` 就會自動取得 EAB 憑證（除非您自行傳入 `eab`）。針對 **Google Trust Services** 與 **SSL.com**，請傳入 `eab YOUR_EAB_KEY_ID YOUR_EAB_HMAC_KEY`（標準補齊的 base64 與未補齊的 base64url 皆可，且 `eab_hmac_key` 在管理 API 輸出中會自動遮罩）。ACME 憑證會依供應商分開快取（例如 `certs/acme-zerossl-fullchain.pem`，Let's Encrypt 則為 `certs/acme-fullchain.pem`），因此切換 `ssl_provider` 後會立即向新的 CA 申請憑證。

您也可以自行提供 PEM 檔案、搭配 `acme.sh`、停用自動 SSL，或直接以 HTTP 提供服務：
- `account_key /etc/gymbag/account.pem`：使用既有的 ECDSA（P-256/P-384）或 RSA ACME 帳戶私鑰。
- `ssl_key /etc/gymbag/domain.key`：使用既有的 ECDSA 或 RSA 私鑰簽署 ACME CSR。
- `tls <cert> <key>`：在全域或個別站台（SNI）使用外部管理的憑證；若私鑰有密碼保護，可搭配 `key_password`（`GYMBAG_KEY_PASSWORD`）。
- `acme_sh_path ~/.acme.sh`（`--acme-sh-path` / `GYMBAG_ACME_SH_PATH`）：直接從 `acme.sh` 主目錄（優先採用 `<domain>_ecc/`、其次 `<domain>/` 或萬用字元目錄）或單一網域憑證目錄載入憑證，支援全域或個別站台設定（`tls acme.sh <path>`），並在續期時自動熱重載。
- `auto_ssl off` / `tls off`（`--no-auto-ssl` / `--http-only`）：停止自動 ACME、本機 CA 或自簽憑證申請。將 `auto_ssl off` 搭配 `acme_sh_path` 或 `tls <cert> <key>` 可僅提供使用者自行管理的憑證；使用 `tls off` 則會完全停用 HTTPS 監聽。
- `direct_http on`（`--direct-http` / `http://example.com` / `GYMBAG_DIRECT_HTTP=on`）：直接在 `http_port` 上以純 HTTP 提供受保護的網站服務（具備完整的 WAF、PoW Cookie、反向代理與 `http://` 重導向改寫功能，且不會強制 `308` 重導向或在 HTTP 回應中送出 HSTS）。

背景排程預設每隔 `renew_interval` 秒（`43200` / 12 小時）檢查一次憑證，並自動續期或重新載入距離到期日不足 `renew_within_days`（`30` 天）的憑證，原子化替換 `tls.Certificate` 而不會中斷既有的 HTTP 串流或 WebSocket 連線。

## 撰寫自訂防火牆規則

受保護的 HTTP 請求會依序經過以下檢查步驟：

1. 請求合法性檢查（HTTP 方法、最大 URI 長度、攔截空位元組與 `..` / `%2e%2e` 遍歷）
2. ACME HTTP-01（`/.well-known/acme-challenge/*`）
3. 本機自訂規則（先執行全域規則，再執行站台規則，由上而下，最先符合者勝出）
4. 遠端規則（`remote_rules`，透過 HTTP/HTTPS 自動更新）
5. 內建基礎規則（除非設定 `default_rules off`）
6. 單一站台單一 IP 速率限制器（`rate_limit` / `rate_window`）
7. IP 威脅情資（`ip_security_action verify` 或 `block`）
8. 工作量證明檢查（`verify` `0`–`3`）

由於自訂規則會在基礎規則、速率限制與威脅情資清單之前執行，因此在站台區塊頂端加上 `allow` 規則，即可乾淨俐落地將辦公室 CIDR、健康檢查端點或預覽 Cookie 列入白名單。

### 簡寫規則

語法：`<allow | verify | block> <field> <pattern...>`（亦支援別名 `bypass`/`pass`、`challenge`/`pow`、`deny`/`drop`/`reject`）：

| 欄位 | 值與比對方式 | 範例 |
| :--- | :--- | :--- |
| `ip` | IPv4、IPv6 或 CIDR | `allow ip 127.0.0.1 10.0.0.0/8 2001:db8::/32` |
| `user_agent` | 不區分大小寫的子字串、`*` 萬用字元或 `~regex` | `block user_agent BadBot ~^(?i)python-urllib/` |
| `path` | 精確/子字串比對、`*` 萬用字元或 `~regex` | `verify path /api/export* ~^/v[0-9]+/search` |
| `query` | 原始/解碼後的查詢字串比對、`*` 萬用字元或 `~regex` | `block query "union select" ~(?i)<script` |
| `method` | HTTP 方法（不區分大小寫） | `verify method POST DELETE` |
| `header` | `<Header-Name> [pattern]`（`""` = 缺少或空白，`*` = 存在） | `verify header Accept-Language ""` |
| `cookie` | `<cookie_name> [pattern]`（`""` = 缺少或空白，`*` = 存在） | `allow cookie vip_session *` |

### 多條件 `rule` 區塊

在 `rule "name" { ... }` 區塊內，同一欄位的複數個值彼此為 OR 關係，不同欄位之間則為 AND 關係：

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

### 遠端規則與動態本機規則檔案

將全域或站台的 `remote_rules` 指向提供 Gymbagfile 規則語法或 JSON（`[ ... ]` 或 `{"rules": [ ... ]}`）的 HTTP/HTTPS URL。通過驗證的內容會快取於 `<cert_dir>/rules-cache`，並搭配 `ETag` / `If-Modified-Since` 進行條件式更新；若遠端回傳格式錯誤的內容，系統會繼續沿用上一份有效規則集。

您也可以透過 `rules_file`、`rules_dir` 或 `ruleset` 區塊（或 `gymbag ruleset add`）載入本機的 `.rules` 或 `.json` 檔案與資料夾。只要將來源標記為 `dynamic`，系統便會每隔 `rules_watch_interval`（預設 `2s`）自動偵測變更並熱重載：

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

`default_rules` 預設為 `on`：當您為站台新增自訂規則時，您的規則會優先執行，而內建的 **block** 規則仍會附掛在後方，避免只寫一條 `verify path` 規則就意外讓 `/.env` 暴露在外。若要完全關閉內建規則，請設定 `default_rules off`。

## CLI、Docker Compose、環境變數與 REST API

### CLI 參考

若工作目錄下存在 `.env`，Gymbag 會先載入它，接著自動搜尋 `./gymbag.json`、`./Gymbagfile` 與 `./Caddyfile`（亦可透過 `-c` 指定 JSON、Gymbagfile、Caddyfile v2、`nginx.conf` 或 Apache 設定檔）：

```bash
# 產生設定檔（預設寫入 gymbag.json，加上 --format gymbagfile 則產生 Gymbagfile）
./gymbag init
./gymbag init -d app.example.com -u http://localhost:3000 -l 3 --rules-dir ./rules.d --rules-dynamic
./gymbag init -t ssh --verify-url https://verify.example.com:8443
./gymbag init -t minecraft --server-properties ./server.properties
./gymbag init -t http,ssh,minecraft -d app.example.com -u :3000 --server-properties ./server.properties
./gymbag init --format gymbagfile -o Gymbagfile -d app.example.com -u :3000

# 管理站台、路由、全域設定與轉換設定檔
./gymbag config list
./gymbag config add -d api.example.com -u :8080 -l 1
./gymbag config update -d api.example.com -l 3 --verify-timeout 6h
./gymbag config route add -d api.example.com --path /static/* --strip-prefix /static --root ./dist
./gymbag config remove -d api.example.com
./gymbag config set admin 127.0.0.1:2019
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json

# 管理規則檔案、資料夾與單條規則
./gymbag ruleset init ./rules/custom.rules
./gymbag ruleset add --source ./rules --dynamic --name local-dir
./gymbag ruleset update local-dir --static
./gymbag ruleset remove local-dir
./gymbag ruleset validate ./rules
./gymbag ruleset rule add --file ./rules/custom.rules --action block --name bad-bot --ua "*BadBot*"

# 執行、監聽、驗證、連接主控台、格式化或管理系統服務
./gymbag
./gymbag run --config gymbag.json --watch
./gymbag console --config gymbag.json
./gymbag service install --config gymbag.json
./gymbag service start
./gymbag service render --type freebsd-rc
./gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3
./gymbag validate --config gymbag.json
./gymbag fmt --overwrite Gymbagfile
./gymbag version
```

### Docker 與 Docker Compose

多架構 Alpine 映像檔（`linux/amd64`、`linux/arm64`）發佈於 `ghcr.io/evil-project/gymbag`，以 UID `10001` 身分執行並具備 `cap_net_bind_service=+ep` 權限。

```bash
# 掛載 Gymbagfile 快速啟動容器
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

`compose.yaml` 將 Gymbag 與 `redis:8-alpine` 配對，透過 Gymbag 純 Go 實作的 RESP2 Redis 用戶端，在容器重啟或多副本間共用 PoW 挑戰 Nonce、固定時間窗速率限制以及 SSH/Minecraft 驗證票證與 IP 通行紀錄（`docker compose up -d --build`）。若要在 Docker 中保護 SSH 或 Minecraft，請一併發佈 `-p 22:22` / `-p 25565:25565` 與 `-p 8443:8443`。關於 Bind-Mount 權限（`chown 10001:10001`）、`--watch` 與純環境變數部署，請參閱 [Docker 與 Docker Compose 指南](../docs/zh-TW/guide/docker.md)。

### 環境變數（`GYMBAG_*`）

套用優先順序為：**設定檔 -> `.env` / `GYMBAG_*` 環境變數 -> CLI 參數**。

| 變數 | 說明 |
| :--- | :--- |
| `GYMBAG_CONFIG` | `gymbag.json`、`Gymbagfile`、`Caddyfile`、`nginx.conf` 或 Apache 設定檔路徑 |
| `GYMBAG_BIND` | 綁定 IP（僅本機 HTTP 時預設 `127.0.0.1`；啟用公開網域、SSH 或 Minecraft 時預設 `0.0.0.0`） |
| `GYMBAG_HTTP_PORT` / `GYMBAG_HTTPS_PORT` | HTTP 與 HTTPS 監聽連接埠（預設 `80` 與 `443`） |
| `GYMBAG_DOMAIN` / `GYMBAG_DOMAINS` | 以逗號分隔的下游網域清單 |
| `GYMBAG_UPSTREAM` | 預設上游目標 URL |
| `GYMBAG_UPSTREAMS` | 以分號分隔的主機對映：`app.example.com=http://web:3000;api.example.com=http://api:8080` |
| `GYMBAG_PROTECT` / `GYMBAG_PROTECT_DOMAINS` | 啟用／停用 WAF 防護（`on`/`off`），或僅對列出之網域啟用防護 |
| `GYMBAG_VERIFY_LEVEL` | 驗證等級 `0`–`3` 或 `off`（別名：`GYMBAG_VERIFY`、`GYMBAG_VERIFY_MODE`、`GYMBAG_MODE`、`GYMBAG_TRIGGER`） |
| `GYMBAG_DEFAULT_RULES` | 內建基礎規則（`on` / `off`，預設 `on`） |
| `GYMBAG_REMOTE_RULES` / `GYMBAG_REMOTE_RULES_REFRESH` | 遠端防火牆規則 URL 與更新間隔（預設 `300`s / `5m`） |
| `GYMBAG_RULES_WATCH_INTERVAL` | `dynamic` 本機規則檔案／資料夾的輪詢間隔（預設 `2`s / `2s`） |
| `GYMBAG_DIFFICULTY` | SHA-256 PoW 難度（`1`–`5`，預設 `4`） |
| `GYMBAG_CHALLENGE_TTL` / `GYMBAG_CLEARANCE_TTL` | 挑戰有效期限（`300`s / `5m`）與驗證通行時間（`86400`s / `24h`；別名：`GYMBAG_VERIFY_TIMEOUT`、`GYMBAG_VERIFIED_TIMEOUT`） |
| `GYMBAG_COOKIE_NAME` / `GYMBAG_COOKIE_SECURE` | 通行 Cookie 名稱（`waf_clearance`）與 `Secure` 旗標（`true`） |
| `GYMBAG_RATE_LIMIT` / `GYMBAG_RATE_WINDOW` | 單一 IP 請求上限（`240`）與時間窗（`60`s） |
| `GYMBAG_TLS_MODE` / `GYMBAG_SSL_PROVIDER` | TLS 模式（`auto`、`acme`、`local`、`selfsigned`、`file`、`off`）與 CA（`letsencrypt`、`letsencrypt_staging`、`zerossl`、`google`、`ssl.com`、`litessl`、`buypass`、`local` 或 URL） |
| `GYMBAG_AUTO_SSL` / `GYMBAG_ACME_SH_PATH` | 切換自動 SSL（`on`/`off`）並設定 `acme.sh` 憑證目錄（別名 `GYMBAG_ACME_SH`） |
| `GYMBAG_DIRECT_HTTP` / `GYMBAG_HTTP_ONLY` | 直接在 HTTP 上提供網站服務（`on`/`off`，別名 `GYMBAG_ALLOW_HTTP`），或完全停用 HTTPS（`on`/`off`） |
| `GYMBAG_ACME_EMAIL` / `GYMBAG_ACME_DIRECTORY` | ACME 聯絡信箱（可啟用 ZeroSSL 自動 EAB）與明確指定的 ACME 目錄 URL |
| `GYMBAG_EAB_KID` / `GYMBAG_EAB_HMAC_KEY` | External Account Binding Key ID 與 base64/base64url 格式的 HMAC 金鑰 |
| `GYMBAG_ACCOUNT_KEY_FILE` | ECDSA/RSA ACME 帳戶私鑰 PEM 路徑 |
| `GYMBAG_CERT_FILE` / `GYMBAG_KEY_FILE` / `GYMBAG_KEY_PASSWORD` | `file` 模式使用的憑證、私鑰與加密私鑰密碼 |
| `GYMBAG_CERT_DIR` | 存放憑證、CA 金鑰、Secret、情資快取與規則快取的目錄（預設 `certs`） |
| `GYMBAG_RENEW_INTERVAL` / `GYMBAG_RENEW_WITHIN_DAYS` | 憑證續期檢查間隔（`43200`s）與到期前續期門檻（`30` 天） |
| `GYMBAG_AUTO_HOSTS` / `GYMBAG_HSTS` | 自動管理 `/etc/hosts`（`on`/`off`）與 `Strict-Transport-Security` 覆寫（`on`/`off`） |
| `GYMBAG_IP_SECURITY` / `GYMBAG_IP_SECURITY_ACTION` | 威脅情資清單（`on`/`off`）與命中動作（`verify` 或 `block`） |
| `GYMBAG_IP_REPUTATION` / `GYMBAG_AI_IP_LIST` | 即時 Shodan InternetDB + StopForumSpam 查詢（`on`/`off`）與官方 AI 爬蟲 CIDR 清單（`on`/`off`） |
| `GYMBAG_VPN_DETECTION` | 在可疑連線檢查中偵測免費 VPN/Proxy/Tor（`on`/`off`，預設 `on`） |
| `GYMBAG_INTEL_REFRESH` | 威脅、VPN 與 AI 情資清單更新間隔（預設 `21600`s / `6h`） |
| `GYMBAG_VERIFY_URL` / `GYMBAG_VERIFY_PORT` | 共用 HTTPS 驗證入口 URL（留空則使用伺服器 IP + IP 憑證）與連接埠（預設 `8443`） |
| `GYMBAG_SSH_ENABLED` / `GYMBAG_SSH_PORT` / `GYMBAG_SSH_UPSTREAM` | 啟用 SSH 防護（預設連接埠 `22`）與後端 `sshd` 目標 |
| `GYMBAG_SSH_VERIFY_LEVEL` / `GYMBAG_SSH_PROTECT` | SSH 驗證等級（`0`–`3` 或 `off`）與防護開關（`on`/`off`） |
| `GYMBAG_MC_ENABLED` / `GYMBAG_MC_PORT` / `GYMBAG_MC_UPSTREAM` | 啟用 Minecraft 防護（預設連接埠 `25565`）與後端伺服器目標（支援 `GYMBAG_MINECRAFT_*` 別名） |
| `GYMBAG_MC_SERVER_JAR` / `GYMBAG_MC_JAVA` | Minecraft `server.jar` 路徑（自動挑選對應 Java 啟動伺服器）與選用的 Java 執行檔覆寫 |
| `GYMBAG_MC_JVM_ARGS` / `GYMBAG_MC_WORK_DIR` / `GYMBAG_MC_AUTO_RESTART` | 以空格分隔的 JVM 參數、Minecraft 工作目錄與異常中止自動重啟開關（`true`/`false`，預設 `true`） |
| `GYMBAG_MC_CONSOLE` | 將託管的 Minecraft 伺服器主控台整合至 Gymbag 前景與管理 API（`on`/`off`，別名 `GYMBAG_MINECRAFT_CONSOLE`） |
| `GYMBAG_MC_VERIFY_LEVEL` / `GYMBAG_MC_PROTECT` | Minecraft 驗證等級（`0`–`3` 或 `off`）與防護開關（`on`/`off`） |
| `GYMBAG_AUTHELIA_URL` / `GYMBAG_AUTHELIA_PORTAL_URL` | 啟用全域 Authelia 前置驗證（`http://127.0.0.1:9091`）與選用的瀏覽器登入重導向 URL（`GYMBAG_AUTHELIA_URI`、`GYMBAG_AUTHELIA_COPY_HEADERS`、`GYMBAG_AUTHELIA_TIMEOUT`、`GYMBAG_AUTHELIA_INSECURE_TLS`、`GYMBAG_AUTHELIA_DOMAINS`、`GYMBAG_FORWARD_AUTH_URL`） |
| `GYMBAG_MASCOT` | 自訂吉祥物檔案、目錄或 `http(s)://` URL（`GYMBAG_MASCOT_PENSIVE`、`GYMBAG_MASCOT_HAPPY`、`GYMBAG_MASCOT_REJECT` 可個別覆蓋各狀態圖片） |
| `GYMBAG_REDIS_URL` / `GYMBAG_SECRET` / `GYMBAG_ADMIN` | Redis URL（`redis://` 或 `rediss://`）、HMAC 密鑰與管理 API 監聽位址（例如 `127.0.0.1:2019` 或 `off`） |

### REST API

**公開 WAF 與驗證入口端點：**

| 方法 | 路徑 | 說明 |
| :--- | :--- | :--- |
| `GET` | `/.waf/v1/session` | 通行狀態（`cleared`）、`protected`、`verify_level`、`verify_mode`、`clearance_ttl`、`expires_at`、`expires_in` |
| `DELETE` | `/.waf/v1/session` | 清除訪客的通行 Cookie（`204 No Content`） |
| `POST` | `/.waf/v1/challenges` | 核發單次使用的 SHA-256 PoW 挑戰（`201 Created`） |
| `POST` | `/.waf/v1/verifications` | 驗證 PoW 解答並設定已簽章的通行 Cookie（支援 `/.waf/verify` 別名） |
| `GET` | `/v/{token}` | 提供 SSH/Minecraft 驗證 PoW 頁面（支援 `/verify/{token}` 別名） |
| `POST` | `/.waf/gate/verify` | 驗證 SSH/Minecraft PoW 解答並寫入 IP 通行紀錄 |
| `GET` | `/.waf/assets/{name}` | 提供內嵌的挑戰頁面 PNG 資產（支援 `/.waf/v1/assets/{name}` 別名，透過 `ETag` 快取） |

**本機管理 API（`admin 127.0.0.1:2019` 或 `--admin`）：**

請務必將管理 API 綁定在 loopback（`127.0.0.1:2019`）或私有介面上——它不需要驗證 Token，且會即時修改執行中的路由與防火牆狀態。

| 方法 | 路徑 | 說明 |
| :--- | :--- | :--- |
| `GET` | `/api/v1/health` | 狀態、`sites_count`、`ssh_enabled`、`minecraft_enabled`、`tls_source`、`threat_entries`、`ai_entries`、`vpn_entries`、`remote_rules` |
| `GET` / `POST` | `/api/v1/tls` / `/api/v1/tls/renew` | 目前 TLS 狀態與 CA 清單；強制憑證續期並進行原子化替換 |
| `GET` / `POST` | `/api/v1/intel` / `/api/v1/intel/refresh` | 各威脅／AI／VPN 情資來源筆數與快取大小；觸發立即更新 |
| `GET` / `POST` | `/api/v1/rules/remote` / `/api/v1/rules/remote/refresh` | 遠端規則筆數與抓取錯誤；強制更新並重載 WAF |
| `GET` / `PUT` | `/api/v1/config` | 匯出遮罩後的設定；驗證並替換執行中的設定 |
| `GET` / `POST` | `/api/v1/sites` | 列出所有網站或在執行期間新增網站 |
| `GET` / `PUT` / `PATCH` / `DELETE` | `/api/v1/sites/{domain}` | 檢視、替換、即時修補（`verify_level`、`protect`、`difficulty`、`clearance_ttl`、`upstreams`、`routes`、`headers`、`auth`、`clear_auth`、`mascot`、`clear_mascot`、`rules`、`remote_rules`、`rule_sources`）或刪除網站 |
| `GET` / `POST` / `PUT` / `DELETE` | `/api/v1/sites/{domain}/rules` 與 `.../{index_or_name}` | 列出、新增、替換或移除網站上的自訂規則 |
| `GET` / `POST` | `/api/v1/minecraft/console` | 啟用 `minecraft.console` 時取得主控台快照（`lines`）或串流（`?stream=1` SSE）輸出並傳送指令（`{"command":"..."}`） |

## 與 Caddy、Nginx 的效能基準測試

測試環境為 Apple M4（10 核心、macOS、Go `1.27.0`），TLS 1.3 使用相同的 ECDSA P-256 憑證、64 個並行 worker、`1 KiB` 回應主體、`1024` 條上游 keep-alive 連線，結果均為 `0` 錯誤：

| 伺服器與模式 | HTTP/1.1 TLS (RPS) | H1 p50 / p99 | HTTP/2 TLS (RPS) | H2 p50 / p99 | 執行檔大小 | RSS（負載下） |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`**（C，`worker_processes auto`） | **85,203** | 0.45 / 6.20 ms | **88,775** | 0.65 / 1.96 ms | 1.5 MB（+libs） | 5.8 MiB |
| **Gymbag `v0.1.0`**（`verify 3`，完整 WAF 啟用） | **72,517** | 0.74 / 3.14 ms | **66,096** | 0.82 / 3.48 ms | 12.9 MB（靜態） | 54.3 MiB |
| **Gymbag `v0.1.0`**（`protect off`，純代理模式） | **72,479** | 0.74 / 3.15 ms | **64,048** | 0.83 / 3.94 ms | 12.9 MB（靜態） | 52.1 MiB |
| **Gymbag `v0.1.0`**（`verify 0`，已持有通行 Cookie） | **65,490** | 0.79 / 3.93 ms | **62,108** | 0.84 / 4.27 ms | 12.9 MB（靜態） | ~54 MiB |
| **Caddy `v2.11.9`**（Go，已調校 keep-alive） | **54,741** | 0.90 / 4.90 ms | **39,207** | 1.30 / 6.18 ms | 49.0 MB | 76.8 MiB |

以 C 語言編寫的 Nginx 在原始吞吐量上依然領先，但 Gymbag 在即時運行完整 WAF 的情況下，於 HTTP/1.1 上仍維持更低的 p99 延遲（`3.14 ms` 對比 `6.20 ms`），並且大幅領先 Caddy（HTTP/1.1 快 `+32%`，HTTP/2 快 `+63%` 至 `+69%`），同時記憶體佔用減少約 30%、執行檔體積縮小 3.8 倍。諸如 `/.env` 等已知漏洞探測請求可在 TLS 邊緣以 **140,680 req/s**（`0.35 ms` p50）直接攔截，完全不會觸及上游伺服器。

測試方法、`go test -bench` 微基準測試與完整設定檔請參閱 [效能基準測試與調校](../docs/zh-TW/guide/benchmarks.md)。

## 專案目錄結構

```text
.
├── cmd/gymbag/          # CLI 進入點
├── docs/                # 多語系 VitePress 文件（EN、简体中文、繁體中文、日本語、한국어、Español）
├── internal/
│   ├── cli/             # 子指令（init、config、ruleset、service、run、reverse-proxy、validate、fmt、version）、.env 與 --watch
│   ├── config/          # 設定型別、Gymbagfile/Caddyfile v2/nginx/Apache 解析器、時間長度與規則來源、GYMBAG_* 環境變數載入器
│   ├── host/            # 網域正規化、IPv6 方括號處理、萬用字元比對
│   ├── intel/           # O(1) 前綴集合、自動更新威脅／VPN／AI 情資清單、即時 IP 與 VPN 信譽查詢
│   ├── localpki/        # 兩層式本機 CA（"Neuro-sama Headquarters Root CA"）與 /etc/hosts 管理員
│   ├── proxy/           # 無鎖多站台與路徑路由器、連線池化 HTTP 與 RFC 6455 WebSocket 代理
│   ├── server/          # 整合式 HTTPS 閘道、HTTP->HTTPS/ACME 重導向器、管理 REST API
│   ├── service/         # 系統服務範本產生與管理（systemd、launchd、openrc、supervisor、freebsd-rc、openbsd-rc、netbsd-rc）
│   ├── setup/           # 自動化 sshd_config/防火牆設定與 Minecraft server.properties 連接埠隨機化
│   ├── store/           # 64 分片記憶體儲存、純 Go RESP2 Redis 用戶端（挑戰、速率限制、閘道票證／通行紀錄）
│   ├── tcpguard/        # SSH 與 Minecraft Java TCP 防護代理、策略引擎、共用 HTTPS 驗證入口
│   ├── tlsmanager/      # 多 CA ACME v2（RFC 8738 IP 憑證、EAB 與 ZeroSSL 自動 EAB）、本機 PKI、原子化 SNI 憑證管理
│   └── waf/             # PoW 驗證器、分片 HMAC 通行快取、Aho-Corasick DFA、遠端／本機規則、內嵌 UI
├── nix/                 # NixOS 模組（services.gymbag）
├── packaging/           # Arch Linux PKGBUILD 與 Debian/Ubuntu .deb 套件腳本／systemd unit
├── Dockerfile
├── compose.yaml
├── flake.nix
├── Gymbagfile
└── go.mod
```

## Star 成長趨勢

<a href="https://www.star-history.com/#Evil-Project/gymbag&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
 </picture>
</a>
