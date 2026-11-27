# CLI 与 `GYMBAG_*` 环境变量

参数覆盖优先级顺序为：

**配置文件 -> `.env` / `GYMBAG_*` 环境变量 -> CLI 命令行参数**

如果当前工作目录存在 `.env` 文件，Gymbag 会自动加载其中的变量，但不会覆盖进程中已有的环境变量。

## CLI 子命令

```bash
# 生成配置（交互式向导或传参）；默认写入 gymbag.json
gymbag init [flags] [output_path]
gymbag init --format gymbagfile -o Gymbagfile -d app.example.com -u :3000

# 使用当前目录下的 ./gymbag.json、./Gymbagfile 或 ./Caddyfile 启动
gymbag

# 指定配置文件启动并监听改动热重载（每 2s 轮询一次）
gymbag run --config Gymbagfile --watch

# 管理系统服务（systemd、launchd、openrc、supervisor）
gymbag service <install|start|stop|restart|status|uninstall|render> [flags]

# 管理站点、路由、全局设置及配置格式转换
gymbag config <list|show|add|update|remove|site|route|set|unset|convert> [flags]

# 管理规则文件、规则目录与内联规则
gymbag ruleset <list|add|update|remove|validate|init|rule> [flags]

# 无需配置文件启动临时 HTTP/HTTPS 反向代理
gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3 --verify-timeout 12h

# 校验配置并打印解析后的服务摘要（包含为 server_jar 选定的 Java）
gymbag validate --config Gymbagfile

# 原地格式化 Gymbagfile（4 空格缩进）
gymbag fmt --overwrite Gymbagfile

# 输出版本号
gymbag version
```

### 服务端启动参数（`run` / 默认命令 / `reverse-proxy`）

| 参数 | 说明 |
| :--- | :--- |
| `-c`, `--config <path>` | `gymbag.json`、`Gymbagfile`、`Caddyfile`、`nginx.conf` 或 Apache 配置文件路径 |
| `-d`, `--domain <list>` | 覆盖下游域名，多个用逗号分隔（`reverse-proxy` 上为 `--from`） |
| `-u`, `--upstream <url>` | 覆盖上游地址（`http://localhost:3000` 或 `:3000`；`reverse-proxy` 上为 `--to`） |
| `-l`, `--verify <0-3\|off>` | 覆盖验证级别（`0`、`1`、`2`、`3` 或 `off`；别名 `--verify-level`、`-m`、`--mode`） |
| `--verify-timeout <dur>` | 覆盖通行有效期（`30m`、`12h`、`7d` 或秒数；别名 `--clearance-ttl`） |
| `--remote-rules <url>` | 附加远程防火墙规则 URL（可重复或逗号分隔） |
| `--remote-rules-refresh <dur>` | 远程规则刷新间隔（默认 `5m` / `300s`） |
| `--no-protect` | 关闭 CLI 指定站点的 PoW 验证 |
| `--ssl-provider <name>` | 覆盖 ACME / SSL 提供商 |
| `--no-auto-ssl` | 关闭自动 SSL 签发（ACME、本地 CA、自签名） |
| `--acme-sh-path <path>` | 使用 `acme.sh` 管理的证书（别名 `--acme-sh`） |
| `--cert-file <path>` / `--key-file <path>` | 自定义 TLS 证书与私钥 PEM 文件路径 |
| `--direct-http` | 直接通过 HTTP 提供网站服务，不重定向到 HTTPS（别名 `--allow-http`） |
| `--http-only` | 关闭 HTTPS 与自动 SSL，仅通过 HTTP 提供网站服务 |
| `--mascot <path-or-url>` | 自定义吉祥物图片文件、目录或 `http(s)://` URL（支持 `--mascot-pensive`、`--mascot-happy`、`--mascot-reject` 按状态分别覆盖） |
| `--admin <addr>` | 启用本地 REST Admin API（`127.0.0.1:2019` 或 `off`） |
| `--http-port <port>` | HTTP 重定向与 ACME HTTP-01 端口（默认 `80`） |
| `--https-port <port>` | HTTPS 网关端口（默认 `443`） |
| `--watch` | 监听配置文件修改并热重载（仅限 `run` / 默认命令） |

## 配置管理（`gymbag config`）

所有 `config` 子命令均支持 `-c`, `--config <path>`（默认使用自动探测到的配置文件）。

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
| `list` / `show` | 列出站点与 TCP 服务 / 打印格式化后的配置（`--format json\|gymbagfile`） |
| `add` | 新增站点：`-d`、`-u`、`-l`、`--difficulty`、`--verify-timeout`、`--no-protect`、`--no-default-rules`、`--cert`、`--key`、`--root`、`--browse`、`--fallback`、`--respond`、`--status`、`--redir`、`--code`、`--header Name=Value\|-Name`、`-f` |
| `update` | 更新站点：`-d`、`-u`、`-l`、`--difficulty`、`--verify-timeout`、`--protect on\|off`、`--default-rules on\|off`、`--cert`、`--key`、`--clear-tls`、`--header`、`--remove-header` |
| `remove` | 移除站点（`-d`） |
| `route list\|add\|update\|remove` | 管理路径路由：`-d`、`--path`、`--strip-prefix`、`-u`、`--root`、`--browse`、`--fallback`、`--respond`、`--status`、`--redir`、`--code`、`--header`、`--index` |
| `set <key> <value>` / `unset <key>` | 设置或重置全局键（`email`、`ssl_provider`、`http_port`、`https_port`、`bind`、`admin`、`cert_dir`、`redis`、`ip_security`、`ip_security_action`、`ip_reputation`、`ai_ip_list`、`intel_refresh`、`remote_rules_refresh`、`rules_watch_interval`、`verify_timeout`、`auto_hosts`、`hsts`、`eab_kid`、`eab_hmac_key`） |
| `convert` | 转换 nginx、Apache、Caddyfile、Gymbagfile 或 JSON：`-c <in>`、`-o <out>`、`--format json\|gymbagfile`、`-f` |

## 规则集管理（`gymbag ruleset`）

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
| `list` | 列出已关联的规则来源（`-c`、`-d`） |
| `add` | 关联规则文件、目录或 URL：`--source`、`--name`、`--dynamic` / `--static`、`-d <domain>` 或 `--global`、`-c` |
| `update` | 更新已关联规则集：`--target`（或位置参数 source/name）、`--source`、`--name`、`--dynamic` / `--static`、`-d` / `--global`、`-c` |
| `remove` | 取消关联规则集：`--source <source-or-name>`、`-d` / `--global`、`-c` |
| `validate <path>` | 校验单个规则文件或规则目录 |
| `init <path>` | 创建规则集模板（`-o`、`--format rules\|json`、`-f`） |
| `rule list\|add\|update\|remove` | 管理 `--file` 中（或通过 `-c` / `-d` 内联在配置中）的单条规则：`--name`、`--index`、`--action allow\|verify\|block`、`--ip`、`--ua`、`--path`、`--query`、`--method`、`--header`、`--cookie` |

## 环境变量（`GYMBAG_*`）

| 变量 | 说明 |
| :--- | :--- |
| `GYMBAG_CONFIG` | `gymbag.json`、`Gymbagfile`、`Caddyfile`、`nginx.conf` 或 Apache 配置文件路径 |
| `GYMBAG_BIND` | 绑定地址（仅本地 HTTP 为 `127.0.0.1`，存在公网域名、SSH 或 Minecraft 时为 `0.0.0.0`） |
| `GYMBAG_HTTP_PORT` / `GYMBAG_HTTPS_PORT` | HTTP 与 HTTPS 监听端口（默认 `80` 和 `443`） |
| `GYMBAG_DOMAIN` / `GYMBAG_DOMAINS` | 逗号分隔的下游域名 |
| `GYMBAG_UPSTREAM` | 默认上游目标 URL |
| `GYMBAG_UPSTREAMS` | 分号分隔的站点上游映射：`app.example.com=http://web:3000;api.example.com=http://api:8080` |
| `GYMBAG_PROTECT` / `GYMBAG_PROTECT_DOMAINS` | 全局开关 WAF 防护（`on`/`off`），或仅对指定域名开启防护 |
| `GYMBAG_VERIFY_LEVEL` | 验证级别：`0`、`1`、`2`、`3` 或 `off`（别名：`GYMBAG_VERIFY`、`GYMBAG_VERIFY_MODE`、`GYMBAG_MODE`、`GYMBAG_TRIGGER`） |
| `GYMBAG_DEFAULT_RULES` | 内置基线规则（`on` / `off`） |
| `GYMBAG_REMOTE_RULES` / `GYMBAG_REMOTE_RULES_REFRESH` | 远程规则 URL 与刷新间隔（默认 `300`s / `5m`） |
| `GYMBAG_RULES_WATCH_INTERVAL` | `dynamic` 本地规则来源轮询间隔（默认 `2`s / `2s`） |
| `GYMBAG_DIFFICULTY` | SHA-256 PoW 难度（`1`–`5`，默认 `4`） |
| `GYMBAG_CHALLENGE_TTL` / `GYMBAG_CLEARANCE_TTL` | 质询有效期（`300`s / `5m`）与放行有效期（`86400`s / `24h`；别名：`GYMBAG_VERIFY_TIMEOUT`、`GYMBAG_VERIFIED_TIMEOUT`） |
| `GYMBAG_COOKIE_NAME` / `GYMBAG_COOKIE_SECURE` | 通行 Cookie 名称（`waf_clearance`）与 `Secure` 标志（`true`） |
| `GYMBAG_RATE_LIMIT` / `GYMBAG_RATE_WINDOW` | 单 IP 请求上限（`240`）与窗口时长（`60`s / `1m`） |
| `GYMBAG_TLS_MODE` / `GYMBAG_SSL_PROVIDER` | TLS 模式（`auto`、`acme`、`local`、`selfsigned`、`file`、`off`）与 CA 提供商或 URL |
| `GYMBAG_AUTO_SSL` / `GYMBAG_ACME_SH_PATH` | 开关自动 SSL 签发（`on`/`off`）并设置 `acme.sh` 目录（别名 `GYMBAG_ACME_SH`） |
| `GYMBAG_DIRECT_HTTP` / `GYMBAG_HTTP_ONLY` | 在保留 HTTPS 的同时提供直接 HTTP 访问（`on`/`off`，别名 `GYMBAG_ALLOW_HTTP`），或完全关闭 HTTPS（`on`/`off`） |
| `GYMBAG_ACME_EMAIL` / `GYMBAG_ACME_DIRECTORY` | ACME 联系邮箱（含 ZeroSSL 自动 EAB）及显式 ACME 目录 URL |
| `GYMBAG_EAB_KID` / `GYMBAG_EAB_HMAC_KEY` | 外部账户绑定 Key ID 与 HMAC 密钥 |
| `GYMBAG_ACCOUNT_KEY_FILE` | ECDSA 或 RSA ACME 账户密钥 PEM 路径 |
| `GYMBAG_CERT_FILE` / `GYMBAG_KEY_FILE` / `GYMBAG_KEY_PASSWORD` | `file` 模式下使用的证书、私钥及口令 |
| `GYMBAG_CERT_DIR` | 证书、CA 密钥、密钥、情报缓存与规则缓存目录（默认 `certs`） |
| `GYMBAG_RENEW_INTERVAL` / `GYMBAG_RENEW_WITHIN_DAYS` | 续期巡检间隔（`43200`s / `12h`）与提前续期阈值（`30` 天） |
| `GYMBAG_AUTO_HOSTS` / `GYMBAG_HSTS` | 自动维护 `/etc/hosts`（`on`/`off`）与覆盖 `Strict-Transport-Security`（`on`/`off`） |
| `GYMBAG_IP_SECURITY` / `GYMBAG_IP_SECURITY_ACTION` | 威胁情报源开关（`on`/`off`）与动作（`verify` 或 `block`） |
| `GYMBAG_IP_REPUTATION` / `GYMBAG_AI_IP_LIST` | Shodan InternetDB + StopForumSpam 实时查询（`on`/`off`）与官方 AI 爬虫列表（`on`/`off`） |
| `GYMBAG_VPN_DETECTION` | 可疑流量判定中启用免费 VPN/代理/Tor 检测（`on`/`off`，默认 `on`） |
| `GYMBAG_INTEL_REFRESH` | 威胁、VPN 与 AI 列表刷新间隔（默认 `21600`s / `6h`） |
| `GYMBAG_VERIFY_URL` / `GYMBAG_VERIFY_PORT` | 共享 HTTPS 验证门户 URL（留空则使用服务器 IP + IP 证书）与端口（默认 `8443`） |
| `GYMBAG_SSH_ENABLED` / `GYMBAG_SSH_PORT` / `GYMBAG_SSH_UPSTREAM` | 启用 SSH 防护（默认 `22`）及后端 `sshd` 地址 |
| `GYMBAG_SSH_VERIFY_LEVEL` / `GYMBAG_SSH_PROTECT` | SSH 验证级别（`0`–`3` 或 `off`）与防护开关（`on`/`off`） |
| `GYMBAG_MC_ENABLED` / `GYMBAG_MC_PORT` / `GYMBAG_MC_UPSTREAM` | 启用 Minecraft 防护（默认 `25565`）及后端目标（支持 `GYMBAG_MINECRAFT_*` 别名） |
| `GYMBAG_MC_SERVER_JAR` / `GYMBAG_MC_JAVA` | Minecraft `server.jar` 路径（自动选择 Java 并拉起服务端；别名 `GYMBAG_MC_JAR`）以及 Java 可执行文件覆盖路径（别名 `GYMBAG_MC_JAVA_PATH`） |
| `GYMBAG_MC_JVM_ARGS` / `GYMBAG_MC_WORK_DIR` / `GYMBAG_MC_AUTO_RESTART` | 空格分隔的 JVM 参数、Minecraft 工作目录以及崩溃自动重启开关（`true`/`false`，默认 `true`） |
| `GYMBAG_MC_VERIFY_LEVEL` / `GYMBAG_MC_PROTECT` | Minecraft 验证级别（`0`–`3` 或 `off`）与防护开关（`on`/`off`） |
| `GYMBAG_REDIS_URL` | Redis 连接地址（`redis://` 或 `rediss://`） |
| `GYMBAG_MASCOT` | 全局吉祥物图片路径、目录或 `http(s)://` URL（支持 `GYMBAG_MASCOT_PENSIVE`、`GYMBAG_MASCOT_HAPPY`、`GYMBAG_MASCOT_REJECT` 按状态分别覆盖） |
| `GYMBAG_SECRET` / `GYMBAG_ADMIN` | HMAC 通行签名密钥与 Admin REST API 监听地址（`127.0.0.1:2019` 或 `off`） |
