# REST API 與 Redis

## 公開 WAF 端點

在受保護的網站上會自動提供以下端點：

| 方法 | 路徑 | 說明 |
| :--- | :--- | :--- |
| `GET` | `/.waf/v1/session` | 回傳 `site`、`protected`、`verify_level`（`0`–`3`）、`verify_mode`、`clearance_ttl`、`cleared`，以及（若已通過驗證）`expires_at` / `expires_in` |
| `DELETE` | `/.waf/v1/session` | 清除訪客的驗證通行 Cookie（`204 No Content`） |
| `POST` | `/.waf/v1/challenges` | 核發單次使用的 SHA-256 工作量證明（PoW）挑戰（`201 Created`） |
| `POST` | `/.waf/v1/verifications` | 驗證 PoW 解答、設定已簽章的通行 Cookie，並回傳 `{"ok": true, "expires_in": <ttl>}`（支援 `/.waf/verify` 別名） |
| `GET` | `/.waf/assets/{name}` | 提供內嵌的挑戰頁面 PNG 資產（支援 `/.waf/v1/assets/{name}` 別名，並透過 `ETag` 快取） |

## 驗證入口端點（SSH 與 Minecraft）

預設在 `verify_port`（`8443`）上提供服務；當 `verify_port` 與 `https_port` 相同且已啟用 HTTP 網站時，則會共用 `https_port`（`443`）：

| 方法 | 路徑 | 說明 |
| :--- | :--- | :--- |
| `GET` | `/v/{token}` | 為 SSH 或 Minecraft 閘道票證提供瀏覽器 PoW 驗證頁面（8 字元 Token 不區分大小寫；支援 `/verify/{token}` 別名） |
| `POST` | `/.waf/gate/verify` | 驗證 `{token, challenge_id, nonce, verified}` 並寫入 IP 通行紀錄（`{"ok": true, "service": ..., "client_ip": ..., "expires_in": <ttl>}`） |
| `GET` | `/.waf/assets/{name}` | 內嵌的吉祥物 PNG 圖檔（支援 `/.waf/v1/assets/{name}` 別名） |
| `GET` | `/.well-known/acme-challenge/{token}` | 回應驗證入口網域或 RFC 8738 IP 憑證的 ACME HTTP-01 挑戰 |

## 本機管理 REST API

可透過 `admin 127.0.0.1:2019`、`--admin 127.0.0.1:2019` 或環境變數 `GYMBAG_ADMIN=127.0.0.1:2019` 啟用。

::: warning 僅限綁定 Loopback 或私有網路介面
管理 API 不需要驗證 Token，且無須重啟 Gymbag 即可即時修改執行中的路由與防火牆狀態。請務必僅將其綁定至 `127.0.0.1:2019` 或內部管理介面。
:::

| 方法 | 路徑 | 說明 |
| :--- | :--- | :--- |
| `GET` | `/api/v1/health` | 狀態、`sites_count`、`ssh_enabled`、`minecraft_enabled`、`tls_source`、`threat_entries`、`ai_entries`、`vpn_entries`、`remote_rules` 數量，以及在設定 `server_jar` 時回傳 `minecraft_server` 狀態（`running`、`pid`、`console_enabled`、`java_major`、`required_java`、`mc_version`、`restarts`、`eula_blocked`、`last_error`） |
| `GET` | `/api/v1/tls` | 目前啟用的 TLS 來源、憑證路徑、SSL 供應商、HSTS 狀態與支援的 CA 清單 |
| `POST` | `/api/v1/tls/renew` | 強制立即續期憑證並進行原子化熱載入 |
| `GET` | `/api/v1/intel` | 威脅情資、AI 爬蟲與 VPN/Tor 清單筆數（`vpn_detection_enabled`、`vpn_entries`、`vpn_cache_size`、`reputation_cache_size`）、更新時間戳記，以及各情資來源狀態（`threat`、`ai`、`vpn`） |
| `POST` | `/api/v1/intel/refresh` | 立即更新所有威脅情資、VPN 與 AI 清單 |
| `GET` | `/api/v1/rules/remote` | 遠端規則 URL、已載入規則數、快取狀態與最近一次錯誤訊息 |
| `POST` | `/api/v1/rules/remote/refresh` | 重新抓取遠端規則 URL 並原子化重載 WAF 引擎 |
| `GET` | `/api/v1/config` | 匯出目前執行中的設定（`secret`、`key_password` 與 `eab_hmac_key` 會自動遮罩） |
| `PUT` | `/api/v1/config` | 驗證並替換目前執行中的設定 |
| `GET` | `/api/v1/sites` | 列出已設定的網站與 WAF 策略 |
| `POST` | `/api/v1/sites` | 在執行期間新增網站（`201 Created`） |
| `GET` | `/api/v1/sites/{domain}` | 檢視單一網站的設定 |
| `PUT` | `/api/v1/sites/{domain}` | 替換單一網站的設定 |
| `PATCH` | `/api/v1/sites/{domain}` | 即時修補 `verify_level`、`protect`、`difficulty`、`clearance_ttl`、`upstreams`、`rate_limit`、`rules`、`remote_rules`、`routes`、`headers`、`auth`、`clear_auth`、`mascot`、`clear_mascot` 或 `rule_sources` |
| `DELETE` | `/api/v1/sites/{domain}` | 移除網站（`204 No Content`；若在未啟用 SSH 與 Minecraft 時嘗試刪除最後一個網站，將回傳 `409 Conflict`） |
| `GET` | `/api/v1/sites/{domain}/rules` | 列出網站上的本機自訂規則 |
| `POST` | `/api/v1/sites/{domain}/rules` | 新增一條自訂規則（`201 Created`） |
| `PUT` | `/api/v1/sites/{domain}/rules` | 替換網站上的所有自訂規則 |
| `DELETE` | `/api/v1/sites/{domain}/rules/{index_or_name}` | 依數字索引或 `name` 刪除自訂規則（`204 No Content`） |
| `GET` | `/api/v1/minecraft/console` | 回傳 JSON 主控台快照（`running`、`pid`、`console_enabled`、`lines`）或即時 SSE 串流（`?stream=1` / `Accept: text/event-stream`），支援 `?since=<seq>`（`Last-Event-ID`）與 `?limit=<n>`；未開啟時回傳 `403 console_disabled` |
| `POST` | `/api/v1/minecraft/console` | 對託管的 Minecraft 伺服器主控台執行 `{"command":"..."}`（手動 `stop` 暫停後支援傳入 `"start"`）；未開啟時回傳 `403 console_disabled`，未執行時回傳 `409 not_running` |

### 即時緩解操作範例

```bash
# 將 app.example.com 收緊為驗證等級 0，並設定 30 分鐘通行時間
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"verify_level": 0, "difficulty": 5, "clearance_ttl": 1800}'

# 即時更新路徑路由、回應標頭與本機規則來源
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"routes": [{"path": "/static/*", "strip_prefix": "/static", "root": "./dist", "browse": true}], "headers": {"X-Served-By": "gymbag"}, "rule_sources": [{"source": "./rules.d", "dynamic": true}]}'

# 無須編輯 Gymbagfile 直接封鎖濫用網段
curl -X POST http://127.0.0.1:2019/api/v1/sites/app.example.com/rules \
  -H "Content-Type: application/json" \
  -d '{"name": "drop-abusive-subnet", "action": "block", "ips": ["198.51.100.0/24"]}'

# 立即更新遠端規則與威脅／VPN／AI 情資清單
curl -X POST http://127.0.0.1:2019/api/v1/rules/remote/refresh
curl -X POST http://127.0.0.1:2019/api/v1/intel/refresh

# 連接／傳送指令至託管的 Minecraft 伺服器主控台（需開啟 minecraft.console: true）
curl -N http://127.0.0.1:2019/api/v1/minecraft/console?stream=1
curl -X POST http://127.0.0.1:2019/api/v1/minecraft/console \
  -H "Content-Type: application/json" \
  -d '{"command": "say Maintenance in 5 minutes"}'
```

## Redis 共用狀態

Gymbag 內建純 Go 實作的 RESP2 Redis 用戶端，支援 `redis://` 與 `rediss://` URL。只要將 `redis`（或 `GYMBAG_REDIS_URL`）指向您的 Redis 執行個體，即可在多執行個體間共用以下狀態：

- 單次使用的 HTTP 與閘道 PoW 挑戰（`gymbag:challenge:<id>`）
- 固定時間窗的單一 IP 速率限制（`gymbag:rate:<key>`）
- SSH 與 Minecraft 的瀏覽器驗證票證（`gymbag:ticket:<token>`）及 IP 通行紀錄（`gymbag:clearance:<service>:<ip>`）

若啟動時無法連線至 Redis，Gymbag 會記錄警告並自動退回使用 64 分片的記憶體儲存（In-Memory Store）。完整的開箱即用堆疊設定請參閱 [Docker 與 Docker Compose 指南](/zh-TW/guide/docker)。
