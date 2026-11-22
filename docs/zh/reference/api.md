# REST API 与 Redis

## 公共 WAF 接口

自动挂载在所有受保护网站下：

| 方法 | 路径 | 说明 |
| :--- | :--- | :--- |
| `GET` | `/.waf/v1/session` | 返回 `site`、`protected`、`verify_level`（`0`–`3`）、`verify_mode`、`clearance_ttl`、`cleared`，已放行时额外返回 `expires_at` / `expires_in` |
| `DELETE` | `/.waf/v1/session` | 清除访客的通行 Cookie（`204 No Content`） |
| `POST` | `/.waf/v1/challenges` | 签发一次性 SHA-256 PoW 质询（`201 Created`） |
| `POST` | `/.waf/v1/verifications` | 校验 PoW 解，签发签名通行 Cookie 并返回 `{"ok": true, "expires_in": <ttl>}`（支持 `/.waf/verify` 别名） |
| `GET` | `/.waf/assets/{name}` | 提供内嵌质询页 PNG 资源（支持 `/.waf/v1/assets/{name}` 别名及 `ETag` 缓存） |

## 验证门户接口（SSH 与 Minecraft）

监听在 `verify_port`（默认 `8443`）上；如果配置了 HTTP 站点且 `verify_port` 等于 `https_port`（`443`），则直接挂载到主 HTTPS 监听器：

| 方法 | 路径 | 说明 |
| :--- | :--- | :--- |
| `GET` | `/v/{token}` | 为 SSH 或 Minecraft 验证凭证返回浏览器 PoW 页面（8 位验证令牌匹配时不区分大小写；支持 `/verify/{token}` 别名） |
| `POST` | `/.waf/gate/verify` | 校验 `{token, challenge_id, nonce, verified}` 并写入 IP 放行记录（`{"ok": true, "service": ..., "client_ip": ..., "expires_in": <ttl>}`） |
| `GET` | `/.waf/assets/{name}` | 内嵌吉祥物 PNG 图片（支持 `/.waf/v1/assets/{name}` 别名） |
| `GET` | `/.well-known/acme-challenge/{token}` | 用于门户域名或 RFC 8738 IP 证书签发的 ACME HTTP-01 验证响应 |

## 本地管理 REST API

通过 `admin 127.0.0.1:2019`、`--admin 127.0.0.1:2019` 或 `GYMBAG_ADMIN=127.0.0.1:2019` 启用。

::: warning 仅绑定回环或私有网卡
Admin API 不设鉴权令牌，可以直接在不重启进程的情况下修改路由与防火墙状态。务必将其绑定在 `127.0.0.1:2019` 或私有管理网卡上。
:::

| 方法 | 路径 | 说明 |
| :--- | :--- | :--- |
| `GET` | `/api/v1/health` | 状态、`sites_count`、`ssh_enabled`、`minecraft_enabled`、`tls_source`、`threat_entries`、`ai_entries`、`vpn_entries`、已加载 `remote_rules` 数量，以及在配置 `server_jar` 时返回 `minecraft_server` 状态（`running`、`pid`、`java_major`、`required_java`、`mc_version`、`restarts`、`eula_blocked`、`last_error`） |
| `GET` | `/api/v1/tls` | 当前 TLS 来源、证书路径、SSL 提供商、HSTS 状态及支持的 CA 列表 |
| `POST` | `/api/v1/tls/renew` | 强制立即执行证书续期并原子重载 |
| `GET` | `/api/v1/intel` | 威胁、AI 爬虫及 VPN/Tor 条目数（`vpn_detection_enabled`、`vpn_entries`、`vpn_cache_size`、`reputation_cache_size`）、刷新时间戳与各源状态（`threat`、`ai`、`vpn`） |
| `POST` | `/api/v1/intel/refresh` | 立即刷新全部威胁、VPN 与 AI 列表 |
| `GET` | `/api/v1/rules/remote` | 远程规则 URL、已加载规则数、缓存状态及最近错误 |
| `POST` | `/api/v1/rules/remote/refresh` | 拉取远程规则 URL 并原子重载 WAF 引擎 |
| `GET` | `/api/v1/config` | 导出当前运行中配置（脱敏 `secret`、`key_password` 和 `eab_hmac_key`） |
| `PUT` | `/api/v1/config` | 校验并整份替换当前运行时配置 |
| `GET` | `/api/v1/sites` | 列出已配置网站及其 WAF 策略 |
| `POST` | `/api/v1/sites` | 运行时新增网站（`201 Created`） |
| `GET` | `/api/v1/sites/{domain}` | 查看单个网站配置 |
| `PUT` | `/api/v1/sites/{domain}` | 整体替换单个网站配置 |
| `PATCH` | `/api/v1/sites/{domain}` | 局部热更新 `verify_level`、`protect`、`difficulty`、`clearance_ttl`、`upstreams`、`rate_limit`、`rules`、`remote_rules`、`routes`、`headers` 或 `rule_sources` |
| `DELETE` | `/api/v1/sites/{domain}` | 删除网站（`204 No Content`；当 SSH 与 Minecraft 均未启用时，删除最后一个剩余站点会返回 `409 Conflict`） |
| `GET` | `/api/v1/sites/{domain}/rules` | 列出网站上的本地自定义规则 |
| `POST` | `/api/v1/sites/{domain}/rules` | 追加一条自定义规则（`201 Created`） |
| `PUT` | `/api/v1/sites/{domain}/rules` | 整体替换网站的全部自定义规则 |
| `DELETE` | `/api/v1/sites/{domain}/rules/{index_or_name}` | 按数字索引或规则 `name` 删除自定义规则（`204 No Content`） |

### 线上实时调整示例

```bash
# 将 app.example.com 收紧至 verify level 0，通行窗口设为 30 分钟
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"verify_level": 0, "difficulty": 5, "clearance_ttl": 1800}'

# 热更新路径路由、响应头与本地规则来源
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"routes": [{"path": "/static/*", "strip_prefix": "/static", "root": "./dist", "browse": true}], "headers": {"X-Served-By": "gymbag"}, "rule_sources": [{"source": "./rules.d", "dynamic": true}]}'

# 不改 Gymbagfile 直接封禁恶意网段
curl -X POST http://127.0.0.1:2019/api/v1/sites/app.example.com/rules \
  -H "Content-Type: application/json" \
  -d '{"name": "drop-abusive-subnet", "action": "block", "ips": ["198.51.100.0/24"]}'

# 刷新远程规则与威胁/VPN/AI 列表
curl -X POST http://127.0.0.1:2019/api/v1/rules/remote/refresh
curl -X POST http://127.0.0.1:2019/api/v1/intel/refresh
```

## Redis 共享状态

Gymbag 内置纯 Go 实现的 RESP2 Redis 客户端，支持 `redis://` 与 `rediss://` URL。配置 `redis`（或 `GYMBAG_REDIS_URL`）后，以下状态会共享到 Redis：

- 一次性 HTTP 与门户 PoW 质询（`gymbag:challenge:<id>`）
- 固定窗口按 IP 限流计数（`gymbag:rate:<key>`）
- SSH 与 Minecraft 浏览器验证凭证（`gymbag:ticket:<token>`）及 IP 放行记录（`gymbag:clearance:<service>:<ip>`）

如果启动时 Redis 不可达，Gymbag 会输出警告并回退到自带的 64 分片内存存储。立即可用的 Compose 编排见 [Docker 与 Docker Compose 指南](/zh/guide/docker)。
