# SSH 與 Minecraft 防護

除了 HTTP/HTTPS 之外，Gymbag 也能為 OpenSSH 與 Minecraft Java 版加上同一套 SHA-256 工作量證明挑戰、速率限制器、IP 威脅情資、VPN/代理/Tor 偵測與自訂規則保護。

Gymbag 不會要求用戶端在 SSH 或 Minecraft 封包內直接計算工作量證明，而是發放一個簡短、方便手動輸入的一次性 HTTPS 連結（`/v/<token>`，由 8 個不易混淆的小寫字元組成，比對時不分大小寫）：SSH 會在斷線畫面印出該連結；Minecraft 1.20.5+ 則會把未驗證玩家放進一個純記憶體的微型空世界（不會在磁碟寫入任何世界檔案），於遊戲內聊天室傳送可點擊的連結，並在驗證完成後透過 `Transfer` 封包自動將玩家導回真正的伺服器（舊版用戶端則改以斷線畫面顯示）。使用者只要在瀏覽器開啟連結並完成解題，Gymbag 就會在 `clearance_ttl`（預設 `24h`）期間放行該 IP，後續連線皆會透過原始 TCP 轉發直通後端。

## SSH 防護

### 自動化設定（`sudo gymbag init -t ssh`）

執行 `sudo ./gymbag init -t ssh` 會自動完成主機端的接線設定：

1. 在 `20000–59999` 之間挑選一個未使用的本機迴環（loopback）連接埠。
2. 將 `/etc/ssh/sshd_config` 備份至 `/etc/ssh/sshd_config.gymbag.bak`（若尚未備份）。
3. 註解掉現有的頂層 `Port` 指令，並在第一個 `Match` 區塊之前插入 `# gymbag-managed:start` / `# gymbag-managed:end` 區塊寫入隨機連接埠。
4. 執行 `sshd -t -f /etc/ssh/sshd_config`。若驗證失敗，立即還原原始設定檔並中止。
5. 重新載入 `sshd`（Linux 上使用 `systemctl`/`service`/`rc-service`；FreeBSD/DragonFly 使用 `service sshd reload`；OpenBSD 使用 `rcctl reload sshd`；NetBSD 使用 `/etc/rc.d/sshd reload`）。
6. 更新系統中啟用的 Linux 防火牆（`ufw`、`firewalld`、`nftables` 或 `iptables`+`ip6tables`）或 BSD 防火牆（透過 `/etc/pf.conf` 中的 `gymbag_ssh` anchor 設定 `pf`、FreeBSD `ipfw` set 29 或 NetBSD `npf`），僅允許本機迴環流量存取後端連接埠，阻擋外部直接連入該後端連接埠，並保持 SSH 監聽埠（`22`）與驗證入口埠（`8443`）開放。
7. 寫入 `gymbag.json`（若指定 `--format gymbagfile` 則寫入 `Gymbagfile`）。

::: warning 請保持第二個 SSH 終端機連線開啟
在遠端主機修改 `sshd_config` 與防火牆規則時，請務必保留一個現有的 root Shell 連線開啟，直到啟動 Gymbag 並確認能透過埠號 `22` 正常登入為止。
:::

若不想修改 `sshd_config` 或防火牆規則（例如 `sshd` 已經監聽在 `127.0.0.1:2222`），請傳入 `--ssh-upstream 127.0.0.1:2222` 或 `--no-auto-setup`（若只想略過防火牆修改，可使用 `--no-firewall`）：

```bash
sudo ./gymbag init -t ssh --ssh-upstream 127.0.0.1:2222
```

### SSH 流量處理方式

- **快速路徑**：當設定為 `protect off` 或用戶端 IP 已持有有效通行紀錄（且無自訂規則）時，Gymbag 會直接轉發 TCP 串流，完全不讀取 SSH Banner。
- **可疑 SSH 偵測**（於驗證等級 `1` 與 `3` 啟用）：Gymbag 會讀取用戶端的 `SSH-2.0-...` 識別 Banner，將空白或非 `SSH-` 開頭的探測封包、已知掃描器與腳本化 SSH 函式庫（`libssh`、`paramiko`、`zgrab`、`masscan`、`nmap`、`asyncssh`、`ssh2go`、`jsch`、`scanner`、`bot`、`python`、`curl`、`ssh-2.0-go`、`hydra`、`medusa`、`patator`），連同威脅情資 IP 與 VPN/代理/Tor IP 一併標記為可疑流量。
- **挑戰回應**：傳送 RFC 4253 `SSH_MSG_DISCONNECT` 封包（原因代碼 `11`），直接將驗證連結顯示在 SSH 用戶端的輸出訊息中。

## Minecraft Java 版防護

### 託管啟動（`--server-jar`）vs. 現有伺服器（`--server-properties`）

如果你想讓 Gymbag 直接託管並監督 Minecraft 處理程序，請傳入 `--server-jar`：

```bash
./gymbag init -t minecraft --server-jar /srv/mc/paper.jar --mc-jvm-args "-Xms1G -Xmx4G"
./gymbag
```

在託管模式下，Gymbag 會執行以下動作：

1. **自動挑選相容的 Java 執行環境**：檢查 `version.json`、bundler 巢狀 JAR、`patch.properties` 或 `.class` 位元組碼標頭，區分傳統 Java 8 與現代 Java 17/21+ 伺服器，並自動選用 Linux、macOS、Windows 或 BSD 上對應的 `java` 執行檔（`/usr/local/openjdk*`、`/usr/local/jdk-*`、`/usr/pkg/java/openjdk*`；可透過 `--mc-java` / `java_path` 手動指定）。
2. **安全設定 `server.properties`**：在 `20000–59999` 之間挑選未使用的本機迴環連接埠；若 `server.properties` 尚未存在則自動建立並設為 `server-ip=127.0.0.1`（若已存在則先備份再更新），並自動設定 `upstream`。
3. **管理子處理程序**：搭配你指定的 `jvm_args` 與 `server_args` 啟動 JAR，預設在當機時自動重啟（`auto_restart`），並在 Gymbag 關閉時傳送優雅的 `stop` 主控台指令（逾時 15 秒）。

### 在 Gymbag 中整合 Minecraft 伺服器主控台（`minecraft.console`）

透過 `server_jar` 啟動伺服器時，只要設定 `console on`（Gymbagfile）、`"console": true`（JSON）、`--mc-console` 或 `GYMBAG_MC_CONSOLE=on`，就能直接在 Gymbag 中操作 Minecraft 主控台：

```bash
# 在 init 時啟用互動式主控台（設定 server_jar 時互動式精靈也會詢問）
./gymbag init -t minecraft --server-jar /srv/mc/paper.jar --mc-console --admin 127.0.0.1:2019

# 在前景執行 Gymbag，直接查看 Minecraft stdout/stderr 並由 stdin 輸入指令
./gymbag

# 或以背景服務執行 Gymbag，從另一個終端機透過 Admin API 連線操作：
./gymbag console                           # 即時互動式主控台串流 + 指令提示列
./gymbag console --cmd "whitelist add Alice"
./gymbag console --no-stream --tail 100
```

- 在主控台內輸入 `stop`（或 `/stop`）會乾淨地停止 Minecraft 子處理程序，不會觸發 `auto_restart` 重啟迴圈；輸入 `start`（或對 `/api/v1/minecraft/console` 發送 POST `{"command":"start"}`）即可在不重啟 Gymbag 的情況下重新啟動伺服器。
- 當 `console` 為 `false`（預設值）時，`/api/v1/minecraft/console` 會回傳 `403 console_disabled`。

若你已經透過其他方式（systemd、tmux、screen、Pterodactyl）執行 Minecraft 伺服器，只需將 `init` 指向其 `server.properties` 即可：

```bash
./gymbag init -t minecraft --server-properties /srv/mc/server.properties
./gymbag
```

針對現有的 `server.properties`，`init` 會：

1. 挑選 `20000–59999` 之間未使用的連接埠，並將 `server.properties` 備份為 `server.properties.gymbag.bak`。
2. 更新 `server-port=<random>`，同時保留所有註解與其他設定鍵。
3. 將空白、`0.0.0.0` 或 `::` 的 `server-ip` 改寫為 `server-ip=127.0.0.1`，防止玩家直接繞過 Gymbag 連線至後端連接埠（若原本 `server-ip` 設為非本機迴環位址，請務必確認外部防火牆也封鎖了該連接埠）。
4. 將上游設定寫入 `gymbag.json` 或 `Gymbagfile`。

若你偏好自行管理 `server.properties`，請傳入 `--mc-upstream 127.0.0.1:25566` 或 `--no-auto-setup`。

### Minecraft 流量處理方式

- **Server List Ping 透傳**：無論是現代狀態交握（`nextState == 1`）還是舊版 1.6 Ping（`0xFE`），除非被規則或 `ip_security_action block` 硬性封鎖，否則都會直接轉發給上游伺服器。伺服器的 MOTD、圖示與線上人數顯示皆可正常運作。
- **登入與 Transfer 閘門（`nextState == 2` / `3`）**：Gymbag 會檢查交握與 Login Start 的玩家名稱。無效的協定版本（`<= 0`）、格式異常的登入封包、無效的玩家名稱（空白、超過 32 字元或包含控制字元）、威脅情資 IP 與 VPN/代理/Tor IP 在等級 `1` 與 `3` 皆會被視為可疑連線。
- **純記憶體空世界驗證與自動傳送（1.20.5+）**：對於支援的現代 Java 版用戶端（`1.20.5`–`26.1`），Gymbag 會先將未驗證的玩家傳送至一個極小的純記憶體空世界（磁碟上不會建立任何世界檔案），站在一個小型石頭出生平台上，並直接在遊戲內聊天室發送可點擊的驗證連結。玩家在瀏覽器完成驗證後，Gymbag 會發送 Minecraft `Transfer` 封包自動將玩家導向真正的伺服器（並透明地將 `nextState=3` 改寫為 `2`，確保即使上游設定 `accepts-transfers=false` 也能正常接受連線）。
- **斷線提示備援（舊版或自訂用戶端）**：低於 `1.20.5` 的用戶端、未知的未來協定版本，或拒絕內建 known packs 的用戶端，會在 **「Connection Lost」** 對話框中收到 Minecraft Login/Configuration Disconnect 封包，顯示可點擊複製至剪貼簿的驗證網址（1.21.5+ 使用 `click_event`/`hover_event`，1.21.4 及更舊版本使用 `clickEvent`/`hoverEvent`）。

## 同時執行 HTTP、SSH 與 Minecraft

單一 Gymbag 處理程序即可同時啟動三種服務的監聽器：

```bash
sudo ./gymbag init -t http,ssh,minecraft \
  -d app.example.com -u http://localhost:3000 \
  --server-jar /srv/mc/paper.jar
sudo ./gymbag
```

`Gymbagfile` 格式範例：

```caddyfile
{
    email ops@example.com
    verify_url https://verify.example.com:8443
    verify_port 8443
}

app.example.com {
    reverse_proxy http://localhost:3000
    verify 3
}

ssh {
    port 22
    upstream 127.0.0.1:41822
    verify 3
    difficulty 4
    rate_limit 60 60s
    allow ip 10.0.0.0/8
}

minecraft {
    port 25565
    upstream 127.0.0.1:48291
    server_jar /srv/mc/paper.jar
    console on
    jvm_args -Xms1G -Xmx4G
    server_properties /srv/mc/server.properties
    verify 3
    rate_limit 60 60s
}
```

對應的 `gymbag.json` 格式：

```json
{
  "sites": [
    {
      "domain": "app.example.com",
      "upstreams": ["http://localhost:3000"],
      "waf": {"verify_level": 3}
    }
  ],
  "ssh": {
    "port": 22,
    "upstream": "127.0.0.1:41822",
    "verify_level": 3,
    "rules": [
      {"action": "allow", "ips": ["10.0.0.0/8"]}
    ]
  },
  "minecraft": {
    "port": 25565,
    "upstream": "127.0.0.1:48291",
    "server_jar": "/srv/mc/paper.jar",
    "console": true,
    "jvm_args": ["-Xms1G", "-Xmx4G"],
    "server_properties": "/srv/mc/server.properties",
    "verify_level": 3
  },
  "verify": {
    "url": "https://verify.example.com:8443",
    "port": 8443
  },
  "tls": {
    "acme_email": "ops@example.com"
  }
}
```

## 共用 HTTPS 驗證入口與 IP SSL 憑證

SSH 與 Minecraft 預設共用埠號 `8443` 上的 HTTPS 監聽器（`verify_port` / `--verify-port` / `GYMBAG_VERIFY_PORT`）。

- **共用埠號 `443`**：若你已啟用 HTTP 站台並將 `verify_port` 設為 `443`（與 `https_port` 相同），Gymbag 會直接透過主要的 HTTPS 監聽器提供 `/v/*`、`/verify/*` 與 `/.waf/gate/verify` 服務，不會另外開啟第二個連接埠。
- **自訂網域 vs 自動偵測伺服器 IP**：設定 `verify_url https://verify.example.com:8443` 可指定 DNS 主機名稱。若 `verify_url` 留空，Gymbag 會從本機網路介面抓取伺服器的公開 IP（若無則透過 `https://api.ipify.org` 查詢，逾時 1.5 秒）。
- **IP SSL 憑證（RFC 8738）**：透過 IP 提供 ACME HTTPS 驗證入口時，Gymbag 會申請 RFC 8738 IP HTTP-01 憑證（若 CA 提供 `shortlived` 設定檔會自動選用），並快取於 `certs/verify-acme-fullchain.pem`（非 Let's Encrypt 的 CA 則為 `verify-acme-<provider>-fullchain.pem`）。若埠號 `80` 無法連線或 CA 不支援 IP 憑證，系統會退回使用 Gymbag 本機 CA（`certs/verify-local-fullchain.pem`）或自簽 IP 憑證（`certs/verify-selfsigned.pem`）；你也可透過 `verify_server { tls <cert> <key> }` 指定自己的憑證檔案。

## 預設值、速率限制與 Redis

- `ssh` 與 `minecraft` 區塊的預設 `verify` 等級為 `3`（`suspicious & ai`）。
- `ssh` 與 `minecraft` 的預設 `rate_limit` 為每 `60` 秒 `60` 次嘗試（網站預設為每 `60` 秒 `240` 次）。
- 寫在 `{ ... }` 內的全域自訂規則在 SSH 與 Minecraft 上同樣會優先於個別服務規則執行（比對 IP/CIDR，並將用戶端 Banner/玩家名稱比對 `user_agent` 規則）。
- 設定 `redis` / `GYMBAG_REDIS_URL` 後，驗證票證（`gymbag:ticket:<token>`）與已放行 IP（`gymbag:clearance:<service>:<ip>`）會儲存於 Redis，即使服務重啟或跨多個實例也能保留通行狀態。
