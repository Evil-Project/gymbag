# 驗證等級（`0`–`3`）與工作量證明

單一 Gymbag 處理程序可以同時代理多個網站（支援輪詢式上游負載平衡與 RFC 6455 WebSocket 升級），並同時提供 SSH 與 Minecraft 防護。每個站台與服務都能透過 `verify <0-3>` 設定策略，或使用 `protect off` 關閉工作量證明挑戰。

<VerifyLevelMatrix />

## 驗證挑戰狀態

Gymbag 將工作量證明挑戰的 HTML 頁面、SHA-256 瀏覽器解題腳本與三張吉祥物圖片直接內嵌在執行檔中：

<div class="gb-mascot-strip">
  <div class="gb-mascot-tile">
    <img src="/mascot-pensive.png" alt="進行工作量證明驗證的吉祥物" />
    <strong>計算工作量證明</strong>
    <span class="gb-badge challenge">已發送驗證挑戰</span>
  </div>
  <div class="gb-mascot-tile">
    <img src="/mascot-happy.png" alt="通過驗證的訪客吉祥物" />
    <strong>已取得通行資格</strong>
    <span class="gb-badge pass">已設定 waf_clearance</span>
  </div>
  <div class="gb-mascot-tile">
    <img src="/mascot-reject.png" alt="遭到封鎖的請求吉祥物" />
    <strong>規則 / 威脅封鎖</strong>
    <span class="gb-badge block">403 Forbidden</span>
  </div>
</div>

## 多站台與多服務設定範例

```caddyfile
app.example.com, www.example.com {
    reverse_proxy http://localhost:3000 http://localhost:3001
    verify 3
    difficulty 4
    verify_timeout 12h
}

docs.example.com {
    reverse_proxy http://localhost:4000
    verify 2
    verify_timeout 7d
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

ssh {
    upstream 127.0.0.1:41822
    verify 3
}
```

在等級 `1` 與 `3` 中，可疑流量除了包含威脅情資清單、即時 Shodan/StopForumSpam 命中結果、缺少瀏覽器標頭、自動化用戶端、漏洞探測與腳本化 SSH/Minecraft 用戶端之外，也包含 **VPN、代理伺服器與 Tor 出口 IP**（`vpn_detection on`）。

## 已驗證狀態逾時時間（`verify_timeout` / `clearance_ttl`）

在 HTTP/HTTPS 站台上，解開 SHA-256 工作量證明挑戰後會簽發一張綁定主機名稱與用戶端 IP 的 `HttpOnly`、`SameSite=Lax` HMAC 簽章 Cookie（預設名稱為 `waf_clearance`）。在 SSH 與 Minecraft 上，完成瀏覽器驗證連結後則會為該服務寫入一筆 IP 通行紀錄。

預設有效期限為 **24 小時**（`86400` 秒）。可在全域或針對個別站台/服務以整數秒數或時間長度字串（`30m`、`12h`、`1h30m`、`7d`、`2w`）進行覆寫：

::: code-group
```caddyfile [Gymbagfile]
{
    verify_timeout 12h
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
    verify_timeout 30m
}

docs.example.com {
    reverse_proxy http://localhost:4000
    verify 2
    clearance_ttl 7d
}
```

```bash [CLI 參數]
gymbag init -d app.example.com -u :3000 --verify-timeout 6h
gymbag run --config Gymbagfile --verify-timeout 30m
gymbag reverse-proxy --from app.internal --to :3000 --verify 0 --verify-timeout 15m
```

```bash [環境變數]
GYMBAG_VERIFY_TIMEOUT=12h gymbag
GYMBAG_CLEARANCE_TTL=3600 gymbag
```

```bash [即時 Admin API]
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/staging.example.com \
  -H "Content-Type: application/json" \
  -d '{"clearance_ttl": 1800}'
```
:::

- `Gymbagfile` 支援的指令別名：`verify_timeout`、`verified_timeout`、`clearance_timeout`、`clearance_ttl`。
- 支援的環境變數別名：`GYMBAG_CLEARANCE_TTL`、`GYMBAG_VERIFY_TIMEOUT`、`GYMBAG_VERIFIED_TIMEOUT`。
- `POST /.waf/v1/verifications` 會回傳 `{"ok": true, "expires_in": <seconds>}`，並將 Cookie 的 `Max-Age` 設為設定的 TTL。
- `GET /.waf/v1/session` 會回傳 `clearance_ttl`，對於已通過驗證的瀏覽器還會額外回傳 `expires_at`（RFC 3339 UTC）與 `expires_in`。

## 難度、挑戰視窗與速率限制

- `difficulty`（`1`–`5`，預設 `4`）：SHA-256 雜湊摘要開頭必須包含的十六進位 `0` 個數（難度 `4` 即要求 `0000...`）。
- `challenge_ttl`（預設 `300` 秒 / `5m`）：單次使用挑戰 Nonce 的有效期限。
- `clearance_ttl` / `verify_timeout`（預設 `86400` 秒 / `24h`）：通行 Cookie 或 TCP IP 通行紀錄的有效期限。
- `rate_limit` / `rate_window`：針對單一 IP 的固定時間視窗請求限制器。網站預設為每 `60` 秒 `240` 次請求；`ssh` 與 `minecraft` 區塊預設為每 `60` 秒 `60` 次嘗試。
