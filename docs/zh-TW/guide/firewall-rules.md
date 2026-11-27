# 防火牆規則、遠端規則與威脅情資

## 請求評估流程

在受保護的 HTTP 站台上，每個請求會依序經過以下檢查層：

1. **請求格式檢查**：驗證 HTTP 方法、URI 長度上限，並拒絕 Null 位元組與 `..` / `%2e%2e` 路徑遍歷攻擊。
2. **ACME HTTP-01 放行**：`/.well-known/acme-challenge/*` 一律直接放行，確保憑證簽發與續期不會中斷。
3. **本機自訂規則**：先套用全域規則，再套用站台規則，由上而下依序比對（先符合者優先）。
4. **遠端規則（`remote_rules`）**：從 HTTP/HTTPS 網址取得並驗證通過的規則，依序進行評估。
5. **內建基礎規則**：常見敏感路徑、CMS 探測、掃描器、SQLi/XSS 查詢與自動化工具 PoW 規則（除非設定 `default_rules off`）。
6. **單一站台、單一 IP 速率限制器**：`rate_limit` 與 `rate_window`（預設 `240` 次 / `60s`）。
7. **IP 威脅情資**：`ip_security_action verify`（預設）或 `block`。
8. **工作量證明驗證**：依據設定的 `verify` 等級（`0`–`3`）執行，包含 VPN/代理/Tor 偵測（`1` 與 `3`）及 AI 爬蟲比對（`2` 與 `3`）。

由於本機自訂規則會在遠端規則、基礎規則、速率限制與威脅情資之前執行，因此只要在站台區塊最上方加上一條 `allow` 規則，就能直接將辦公室 CIDR、健康檢查或預覽 Cookie 加入白名單，無須額外繞道設定。寫在 `{ ... }` 內的全域規則也會套用至 `ssh` 與 `minecraft` 服務（與寫在 `ssh { ... }` 或 `minecraft { ... }` 內的個別服務規則並存）。

## 單行簡寫規則

語法：`<allow | verify | block> <field> <pattern...>`（別名 `bypass`/`pass`、`challenge`/`pow` 與 `deny`/`drop`/`reject` 皆可使用）。

| 欄位 | 比對語意 | 範例 |
| :--- | :--- | :--- |
| `ip` | IPv4、IPv6 或 CIDR 網段 | `allow ip 127.0.0.1 10.0.0.0/8 2001:db8::/32` |
| `user_agent` | 不分大小寫子字串、`*` 萬用字元或 `~regex` | `block user_agent BadBot ~^(?i)python-urllib/` |
| `path` | 精確/前綴比對、`*` 萬用字元或 `~regex` | `verify path /api/export* ~^/v[0-9]+/search` |
| `query` | 原始/解碼後查詢字串比對、`*` 萬用字元或 `~regex` | `block query "union select" ~(?i)<script` |
| `method` | HTTP 方法（不分大小寫） | `verify method POST DELETE` |
| `header` | `<Header-Name> [pattern]`（`""` 表示缺少或為空，`*` 表示存在即可） | `verify header Accept-Language ""` |
| `cookie` | `<cookie_name> [pattern]`（`""` 表示缺少或為空，`*` 表示存在即可） | `allow cookie vip_session *` |

## 多條件 `rule` 區塊

當需要同時符合多個條件時，請使用 `rule "name" { ... }`。同一欄位的多個值為 OR 關係；同一個區塊內的不同欄位則為 AND 關係：

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

## 遠端規則（`remote_rules`）

你可以從一個或多個 HTTP/HTTPS 網址拉取共用規則集（例如 Git raw 檔案、物件儲存空間、CDN 或策略伺服器）。Gymbag 會在啟動時拉取，並每隔 `remote_rules_refresh`（預設 `5m` / `300s`）更新一次。

::: code-group
```caddyfile [Gymbagfile]
{
    remote_rules https://rules.example.com/org-baseline.rules
    remote_rules_refresh 5m
}

app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    allow ip 10.0.0.0/8
    remote_rules https://rules.example.com/app-overrides.json
}
```

```bash [CLI 與環境變數]
gymbag run --config Gymbagfile \
  --remote-rules https://rules.example.com/baseline.rules \
  --remote-rules-refresh 10m

GYMBAG_REMOTE_RULES="https://rules.example.com/baseline.rules,https://rules.example.com/extra.json" \
GYMBAG_REMOTE_RULES_REFRESH=5m \
gymbag
```

```bash [Admin REST API]
curl http://127.0.0.1:2019/api/v1/rules/remote
curl -X POST http://127.0.0.1:2019/api/v1/rules/remote/refresh

curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"remote_rules": ["https://rules.example.com/emergency.rules"]}'
```
:::

### 支援的遠端規則格式

Gymbag 同時支援 **Gymbagfile 規則語法**與 **JSON**（`[ ... ]` 或 `{"rules": [ ... ]}`）：

::: code-group
```caddyfile [純文字 Gymbagfile 語法]
block ip 198.51.100.0/24 203.0.113.42
block user_agent BadScraper ~^(?i)EvilBot/
verify path /api/export*

rule "challenge-xml-posts" {
    action verify
    method POST
    path /xmlrpc* /api/import*
    header Origin ""
}
```

```json [JSON 物件]
{
  "rules": [
    {
      "name": "block-abusive-cidr",
      "action": "block",
      "ips": ["198.51.100.0/24", "203.0.113.42"]
    },
    {
      "name": "verify-exports",
      "action": "verify",
      "methods": ["GET", "POST"],
      "paths": ["/api/export*"]
    }
  ]
}
```
:::

### 快取與驗證機制

- 請求時會帶上 `If-None-Match`（`ETag`）與 `If-Modified-Since`；收到 `304 Not Modified` 時會直接跳過重新編譯。
- 已驗證的內容會快取於 `<cert_dir>/rules-cache/`，啟動時在第一次網路抓取完成前即可預先載入。
- 每份遠端內容在切換前都會經過驗證（上限 4 MiB、逾時 15 秒）。若上游回傳語法錯誤或 HTTP 錯誤，系統會繼續沿用舊版規則集，並將錯誤資訊顯示於 `GET /api/v1/rules/remote`。
- 規則更新採原子化切換，不會中斷現有連線。

## 本機規則檔、目錄與熱重載

你也可以將規則獨立存放在 `.rules` / `.json` 檔案或目錄中，並標記為 `dynamic` 以監控磁碟變動並自動重新載入：

::: code-group
```caddyfile [Gymbagfile]
{
    rules_watch_interval 2s
}

app.example.com {
    reverse_proxy http://localhost:3000
    rules_file ./rules/custom.rules dynamic
    rules_dir ./rules.d dynamic
    ruleset static-corp {
        source ./rules/corp.json
        dynamic off
    }
}
```

```json [gymbag.json]
{
  "sites": [
    {
      "domain": "app.example.com",
      "upstreams": ["http://localhost:3000"],
      "waf": {
        "rule_sources": [
          {"source": "./rules.d", "dynamic": true, "name": "shared"}
        ]
      }
    }
  ]
}
```

```bash [CLI]
gymbag init -d app.example.com -u :3000 \
  --rules-file ./rules/custom.rules --rules-dir ./rules.d \
  --rules-dynamic --rules-watch-interval 2s

gymbag ruleset init ./rules/custom.rules
gymbag ruleset add --source ./rules --dynamic --name local-dir
gymbag ruleset update local-dir --static
gymbag ruleset validate ./rules
gymbag ruleset rule add --file ./rules/custom.rules --action block --name bad-bot --ua "*BadBot*"
gymbag ruleset remove local-dir
```
:::

輪詢週期由 `rules_watch_interval` 控制（預設 `2s`）。`rule_sources` 亦可透過 `PATCH /api/v1/sites/{domain}` 即時修改。完整的 `ruleset` 指令清單請參閱 [CLI 與環境變數](/zh-TW/reference/cli-env)。

## 內建規則（`default_rules on|off`）

`default_rules` 預設為 `on`：
- 當站台**沒有**任何本機或遠端自訂規則時，Gymbag 會同時啟用內建封鎖規則（`/.env`、`/.git/*`、`/wp-admin/*`、`sqlmap`/`nuclei`、SQLi/XSS 查詢）與自動化工具 PoW 規則（`curl`、`wget`、`python-requests`、`headlesschrome`、`playwright`、空的 `User-Agent`）。
- 一旦你為站台新增了自訂規則，你的規則會優先執行，而內建**封鎖**規則仍會接續在後執行，確保你不會因為只加了一條 `verify path` 規則就意外暴露 `/.env`。
- 若要完全停用內建規則，可在站台或全域設定 `default_rules off`。

## 威脅情資、VPN/Tor 偵測與 AI 爬蟲清單

情資清單會在啟動時與每隔 `intel_refresh`（預設 `21600` 秒 / `6h`）自動更新，並快取於 `<cert_dir>/intel-cache`：

- **威脅情資清單（`ip_security on`）**：IPsum level 3+、Spamhaus DROP IPv4 & IPv6、abuse.ch Feodo Tracker。
- **即時 IP 信譽查詢（`ip_reputation on`）**：透過 Shodan InternetDB 與 StopForumSpam 進行無金鑰查詢，具備 1 小時 TTL 快取與 fail-open 逾時保護。
- **VPN / 代理 / Tor 偵測（`vpn_detection on`）**：自動更新 X4BNet VPN IPv4/IPv6 清單與 Tor 出口節點清單，並搭配 `api.ipquery.io` 與 `api.ipapi.is` 的無金鑰即時檢查（1.2 秒逾時、singleflight 去重、1 小時快取、fail-open）。被標記的 IP 在 HTTP、SSH 與 Minecraft 的驗證等級 `1` 與 `3` 中會被視為**可疑流量**。
- **官方 AI 爬蟲 CIDR（`ai_ip_list on`）**：OpenAI（`gptbot`、`searchbot`、`chatgpt-user`）、Anthropic（`claude.com/crawling/bots.json`）、Perplexity、Google 與 Apple（`applebot`）。

```caddyfile
{
    ip_security on
    ip_security_action block   # "verify"（PoW 挑戰）或 "block"（403 Forbidden）
    ip_reputation on
    vpn_detection on
    ai_ip_list on
    intel_refresh 6h
    # vpn_feed https://rules.example.com/extra-vpn-cidrs.txt
}
```
