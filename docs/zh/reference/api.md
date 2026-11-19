# REST API 与 Redis

## 公开 WAF 端点

受保护站点默认提供以下端点：

| 方法 | 路径 | 说明 |
| :--- | :--- | :--- |
| `GET` | `/.waf/v1/session` | 返回当前通行状态 (`cleared`)、`protected`、`verify_level` (`0`–`3`)、`verify_mode`、`clearance_ttl`，以及已验证时的 `expires_at` / `expires_in` |
| `DELETE` | `/.waf/v1/session` | 清除访客的验证 Cookie（`204 No Content`） |
| `POST` | `/.waf/v1/challenges` | 签发单次有效的 SHA-256 PoW 质询（`201 Created`） |
| `POST` | `/.waf/v1/verifications` | 校验 PoW 解并签发签名 Cookie，返回 `{"ok": true, "expires_in": <ttl>}`（兼容 `/.waf/verify`） |
| `GET` | `/.waf/assets/{name}` | 提供内嵌吉祥物 PNG 资源（兼容 `/.waf/v1/assets/{name}`，预缓存并支持 `ETag`） |

---

## 本地 Admin REST API

通过 `admin 127.0.0.1:2019`、`--admin 127.0.0.1:2019` 或 `GYMBAG_ADMIN=127.0.0.1:2019` 启用。

::: warning 务必绑定到回环或私有管理网络
Admin API 无需认证令牌即可在不重启进程的情况下修改实时路由和防火墙规则。请始终绑定到 `127.0.0.1:2019` 或私有 WireGuard/Unix 接口。
:::

| 方法 | 路径 | 说明 |
| :--- | :--- | :--- |
| `GET` | `/api/v1/health` | 网关状态、`sites_count`、`tls_source`、`threat_entries`、`ai_entries` 与已加载 `remote_rules` 数量 |
| `GET` | `/api/v1/tls` | 活跃 TLS 来源、证书路径、SSL 提供商、HSTS 状态及支持的 CA 列表 |
| `POST` | `/api/v1/tls/renew` | 强制立即续期证书并原子热重载 |
| `GET` | `/api/v1/intel` | 各威胁/AI 订阅计数、上次刷新时间与信誉缓存统计 |
| `POST` | `/api/v1/intel/refresh` | 立即触发后台情报源刷新 |
| `GET` | `/api/v1/rules/remote` | 查看所有远程规则 URL、已加载规则数量、缓存状态及错误信息 |
| `POST` | `/api/v1/rules/remote/refresh` | 立即拉取全部远程规则 URL 并原子重载 WAF 引擎 |
| `GET` | `/api/v1/config` | 导出当前运行时配置（自动脱敏 `secret`、`key_password`、`eab_hmac_key`） |
| `PUT` | `/api/v1/config` | 校验并原子替换完整运行时配置 |
| `GET` | `/api/v1/sites` | 列出所有已配置站点及 WAF 策略 |
| `POST` | `/api/v1/sites` | 运行时动态添加新站点（`201 Created`） |
| `GET` | `/api/v1/sites/{domain}` | 查看单个站点配置 |
| `PUT` | `/api/v1/sites/{domain}` | 替换单个站点配置 |
| `PATCH` | `/api/v1/sites/{domain}` | 实时局部更新 `verify_level`、`protect`、`difficulty`、`clearance_ttl`、`upstreams`、`rate_limit`、`rules` 或 `remote_rules` |
| `DELETE` | `/api/v1/sites/{domain}` | 移除站点（`204 No Content`；禁止删除最后一个站点） |
| `GET` | `/api/v1/sites/{domain}/rules` | 列出站点本地自定义规则 |
| `POST` | `/api/v1/sites/{domain}/rules` | 实时追加一条自定义防火墙规则（`201 Created`） |
| `PUT` | `/api/v1/sites/{domain}/rules` | 替换站点全部本地自定义规则 |
| `DELETE` | `/api/v1/sites/{domain}/rules/{index_or_name}` | 按索引或 `name` 删除自定义规则（`204 No Content`） |

### 实时应急防护示例

```bash
# 将 app.example.com 切换为级别 0（全员验证）并设置 30 分钟有效期
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"verify_level": 0, "difficulty": 5, "clearance_ttl": 1800}'

# 无需修改 Gymbagfile 立即封禁恶意网段
curl -X POST http://127.0.0.1:2019/api/v1/sites/app.example.com/rules \
  -H "Content-Type: application/json" \
  -d '{"name": "drop-abusive-subnet", "action": "block", "ips": ["198.51.100.0/24"]}'

# 立即刷新远程防火墙规则与威胁情报
curl -X POST http://127.0.0.1:2019/api/v1/rules/remote/refresh
curl -X POST http://127.0.0.1:2019/api/v1/intel/refresh
```

---

## Redis 共享状态

Gymbag 内置纯 Go RESP2 Redis 客户端，零外部依赖。配置 `redis redis://[:password@]host:6379/0`（或 `GYMBAG_REDIS_URL`）后，单次 PoW 质询与按 IP 限流窗口可在容器重启或多副本间共享。

Docker 与 Compose 完整部署方式请参阅 [Docker 与 Compose 部署指南](/zh/guide/docker)。
