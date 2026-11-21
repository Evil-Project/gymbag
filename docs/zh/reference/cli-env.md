# CLI 与 `GYMBAG_*` 环境变量参考

配置优先级严格遵循：

**配置文件 → `.env` / `GYMBAG_*` 环境变量 → CLI 命令行参数**

若当前工作目录存在 `.env` 文件，Gymbag 会自动加载它（不会覆盖已设置的操作系统环境变量）。

## CLI 子命令

```bash
# 生成配置文件（交互向导或参数驱动）；默认写入 gymbag.json
gymbag init [flags] [output_path]
gymbag init --format gymbagfile -o Gymbagfile -d app.example.com -u :3000

# 按顺序使用 ./gymbag.json、./Gymbagfile 或 ./Caddyfile 启动服务
gymbag

# 指定配置文件并开启自动热重载（每 2 秒轮询变化）
gymbag run --config Gymbagfile --watch

# 管理系统服务（systemd、launchd、openrc、supervisor）
gymbag service <install|start|stop|restart|status|uninstall|render> [flags]

# 管理 gymbag.json 或 Gymbagfile 中的站点、路由与全局设置
gymbag config <list|show|add|update|remove|route|set|unset|convert> [flags]

# 管理规则文件、规则目录与内联规则
gymbag ruleset <list|add|update|remove|validate|init|rule> [flags]

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
| `-c`, `--config <path>` | `gymbag.json`、`Gymbagfile`、`Caddyfile`、原生 `nginx.conf` 或 Apache `httpd.conf` / `apache2.conf` 配置文件路径 |
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

## 配置管理 (`gymbag config`)

所有 `config` 子命令均支持 `-c`、`--config <path>`（默认使用自动探测到的配置文件）。

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

| 子命令 | 说明 |
| :--- | :--- |
| `list` / `show` | 列出站点 / 打印格式化后的配置 |
| `add` | 添加站点：`-d`、`-u`、`-l`、`--difficulty`、`--verify-timeout`、`--no-protect`、`--no-default-rules`、`--cert`、`--key`、`--root`、`--browse`、`--fallback`、`--respond`、`--status`、`--redir`、`--code`、`--header Name=Value\|-Name`、`-f` |
| `update` | 更新站点：`-d`、`-u`、`-l`、`--difficulty`、`--verify-timeout`、`--protect on\|off`、`--default-rules on\|off`、`--cert`、`--key`、`--clear-tls`、`--header`、`--remove-header` |
| `remove` | 移除站点（`-d`） |
| `route list\|add\|update\|remove` | 管理路径路由：`-d`、`--path`、`--strip-prefix`、`-u`、`--root`、`--browse`、`--fallback`、`--respond`、`--status`、`--redir`、`--code`、`--header`、`--index`（用于 `update` / `remove` 的从 0 开始的路由索引） |
| `set <key> <value>` / `unset <key>` | 修改或重置全局选项（`email`、`ssl_provider`、`admin`、`http_port`、`https_port`、`rules_watch_interval`、`remote_rules_refresh` 等） |
| `convert` | 转换 nginx、Apache、Caddyfile、Gymbagfile 或 JSON：`-c <in>`、`-o <out>`（省略则输出到标准输出）、`--format json\|gymbagfile`（默认 `json`）、`-f` |

## 规则集管理 (`gymbag ruleset`)

```bash
gymbag ruleset init ./rules/custom.rules
gymbag ruleset add --source ./rules --dynamic --name local-dir
gymbag ruleset update local-dir --static
gymbag ruleset remove local-dir
gymbag ruleset validate ./rules
gymbag ruleset rule add --file ./rules/custom.rules --action block --name bad-bot --ua "*BadBot*"
```

| 子命令 | 说明 |
| :--- | :--- |
| `list` | 列出已关联的规则源（`-c`、`-d`） |
| `add` | 关联规则文件、目录或 URL：`--source`、`--name`、`--dynamic` / `--static`、`-d <domain>` 或 `--global`、`-c` |
| `update` | 修改已关联的规则集：`--target`（或位置参数 source/name）、`--source`、`--name`、`--dynamic` / `--static`、`-d` / `--global`、`-c` |
| `remove` | 取消关联规则集：`--source <source-or-name>`、`-d` / `--global`、`-c` |
| `validate <path>` | 校验规则文件或规则目录 |
| `init <path>` | 创建初始规则集（`-o`、`--format rules\|json`、`-f`） |
| `rule list\|add\|update\|remove` | 管理 `--file` 中（或通过 `-c` / `-d` 内联在配置中）的单条规则：`--name`、`--index`、`--action allow\|verify\|block`、`--ip`、`--ua`、`--path`、`--query`、`--method`、`--header`、`--cookie` |

---

## 环境变量 (`GYMBAG_*`)

| 变量名 | 说明 |
| :--- | :--- |
| `GYMBAG_CONFIG` | `gymbag.json`、`Gymbagfile`、`Caddyfile`、`nginx.conf` 或 Apache `httpd.conf` 配置文件路径 |
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
| `GYMBAG_RULES_WATCH_INTERVAL` | 动态本地规则文件/目录的轮询间隔，支持秒数或时长（默认 `2` / `2s`） |
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
