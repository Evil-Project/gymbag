# CLI 与 `GYMBAG_*` 环境变量参考

配置优先级严格遵循：

**配置文件 → `.env` / `GYMBAG_*` 环境变量 → CLI 命令行参数**

若当前工作目录存在 `.env` 文件，Gymbag 会自动加载它（不会覆盖已设置的操作系统环境变量）。

## CLI 子命令

```bash
# 生成配置文件（交互向导或参数驱动）
gymbag init [flags] [output_path]

# 使用 ./Gymbagfile 或 ./Caddyfile 启动服务
gymbag

# 指定配置文件并开启自动热重载（每 2 秒轮询变化）
gymbag run --config Gymbagfile --watch

# 管理系统服务（systemd、launchd、openrc、supervisor）
gymbag service <install|start|stop|restart|status|uninstall|render> [flags]

# 无配置文件临时反向代理
gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3 --verify-timeout 12h

# 校验配置并打印各站点解析后的策略
gymbag validate --config Gymbagfile

# 就地格式化 Gymbagfile
gymbag fmt --overwrite Gymbagfile

# 查看版本
gymbag version
```

### 常用与服务端参数 (`run` / 默认 / `reverse-proxy`)

| 参数 | 说明 |
| :--- | :--- |
| `-c`, `--config <path>` | `Gymbagfile`、`Caddyfile` 或 `.json` 配置文件路径 |
| `-d`, `--domain <list>` | 覆盖下游域名，多个以逗号分隔（`reverse-proxy` 中为 `--from`） |
| `-u`, `--upstream <url>` | 覆盖上游 URL（`http://localhost:3000` 或 `:3000`；`reverse-proxy` 中为 `--to`） |
| `-l`, `--verify <0-3\|off>` | 覆盖验证级别（`0`、`1`、`2`、`3` 或 `off`） |
| `--verify-timeout <dur>` | 覆盖已验证状态有效期（如 `30m`、`12h`、`7d` 或秒数；别名 `--clearance-ttl`） |
| `--remote-rules <url>` | 挂载远程防火墙规则 URL（http/https，可重复或逗号分隔） |
| `--remote-rules-refresh <dur>` | 远程规则刷新间隔（默认 `5m` / `300s`） |
| `--no-protect` | 关闭 WAF 验证 |
| `--ssl-provider <name>` | 覆盖 ACME / SSL 证书提供商 |
| `--admin <addr>` | 启用本地 REST 管理 API（如 `127.0.0.1:2019` 或 `off`） |
| `--http-port <port>` | HTTP 跳转与 ACME HTTP-01 端口（默认 `80`） |
| `--https-port <port>` | HTTPS 网关端口（默认 `443`） |
| `--watch` | 监听配置文件变更并自动热重载（仅 `run`） |

---

## 环境变量 (`GYMBAG_*`)

| 变量名 | 说明 |
| :--- | :--- |
| `GYMBAG_CONFIG` | `Gymbagfile`、`Caddyfile` 或 `.json` 配置文件路径 |
| `GYMBAG_BIND` | 绑定监听地址（本地域名默认 `127.0.0.1`，公网域名默认 `0.0.0.0`） |
| `GYMBAG_HTTP_PORT` / `GYMBAG_HTTPS_PORT` | HTTP 与 HTTPS 监听端口（默认 `80` 与 `443`） |
| `GYMBAG_DOMAIN` / `GYMBAG_DOMAINS` | 逗号分隔的下游域名 |
| `GYMBAG_UPSTREAM` | 默认上游目标 URL |
| `GYMBAG_UPSTREAMS` | 分号分隔的域名映射：`app.example.com=http://web:3000;api.example.com=http://api:8080` |
| `GYMBAG_PROTECT` | 全局开关 WAF 防护（`on` / `off`） |
| `GYMBAG_PROTECT_DOMAINS` | 逗号分隔的启用 WAF 防护的域名子集 |
| `GYMBAG_VERIFY_LEVEL` | 验证级别：`0`、`1`、`2` 或 `3`（别名：`GYMBAG_VERIFY`、`GYMBAG_VERIFY_MODE`、`GYMBAG_MODE`、`GYMBAG_TRIGGER`） |
| `GYMBAG_DEFAULT_RULES` | 开关内置基础防火墙规则（`on` / `off`） |
| `GYMBAG_REMOTE_RULES` | 逗号或空格分隔的 HTTP/HTTPS 远程规则 URL 列表 |
| `GYMBAG_REMOTE_RULES_REFRESH` | 远程规则刷新间隔，支持秒数或时长（默认 `300` / `5m`） |
| `GYMBAG_DIFFICULTY` | SHA-256 PoW 难度（`1`–`5`，默认 `4`） |
| `GYMBAG_CHALLENGE_TTL` | 质询过期时间（秒数或时长，默认 `300` / `5m`） |
| `GYMBAG_CLEARANCE_TTL` | 已验证状态有效期（秒数或如 `30m`、`12h`、`7d`，默认 `86400` / `24h`；别名：`GYMBAG_VERIFY_TIMEOUT`、`GYMBAG_VERIFIED_TIMEOUT`） |
| `GYMBAG_COOKIE_NAME` / `GYMBAG_COOKIE_SECURE` | Cookie 名称（`waf_clearance`）与 `Secure` 标志（`true`） |
| `GYMBAG_RATE_LIMIT` / `GYMBAG_RATE_WINDOW` | 单 IP 限流次数（`240`）与窗口（`60` 秒 / `1m`） |
| `GYMBAG_TLS_MODE` | `auto`、`acme`、`local`、`selfsigned` 或 `file` |
| `GYMBAG_SSL_PROVIDER` | `letsencrypt`、`letsencrypt_staging`、`zerossl`、`google`、`ssl.com`、`litessl`、`buypass`、`local` 或 URL |
| `GYMBAG_ACME_EMAIL` | ACME 注册邮箱及 ZeroSSL 自动 EAB 邮箱 |
| `GYMBAG_ACME_DIRECTORY` | 显式 RFC 8555 ACME v2 目录 URL |
| `GYMBAG_EAB_KID` / `GYMBAG_EAB_HMAC_KEY` | EAB Key ID 与 HMAC 密钥 |
| `GYMBAG_ACCOUNT_KEY_FILE` | ACME 账号私钥 PEM 路径 |
| `GYMBAG_CERT_FILE` / `GYMBAG_KEY_FILE` | `file` 模式下的证书与私钥路径 |
| `GYMBAG_KEY_PASSWORD` | 加密 PEM 私钥密码 |
| `GYMBAG_CERT_DIR` | CA 证书、ACME 状态、情报缓存与远程规则缓存目录（默认 `certs`） |
| `GYMBAG_RENEW_INTERVAL` / `GYMBAG_RENEW_WITHIN_DAYS` | 续期检查间隔（`43200`s / `12h`）与提前天数（`30` 天） |
| `GYMBAG_AUTO_HOSTS` | 本地域名 `/etc/hosts` 自动管理（`on` / `off`） |
| `GYMBAG_HSTS` | 覆盖 `Strict-Transport-Security` 响应头（`on` / `off`） |
| `GYMBAG_IP_SECURITY` / `GYMBAG_IP_SECURITY_ACTION` | 开关威胁情报订阅（`on`/`off`）及动作（`verify` 或 `block`） |
| `GYMBAG_IP_REPUTATION` | 开关 Shodan InternetDB + StopForumSpam 查询（`on`/`off`） |
| `GYMBAG_AI_IP_LIST` | 开关官方 AI 爬虫 IP 列表（`on`/`off`） |
| `GYMBAG_INTEL_REFRESH` | 情报刷新间隔（秒数或时长，默认 `21600` / `6h`） |
| `GYMBAG_REDIS_URL` | 可选 Redis 连接地址（`redis://[:password@]host:port/db`） |
| `GYMBAG_SECRET` | 通行 Cookie 的 HMAC 签名密钥 |
| `GYMBAG_ADMIN` | 本地 Admin REST API 监听地址（如 `127.0.0.1:2019` 或 `off`） |
