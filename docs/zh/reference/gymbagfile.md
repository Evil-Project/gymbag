# `Gymbagfile` 与 JSON 配置参考

默认情况下，Gymbag 会自动探测当前目录的 `./gymbag.json`、`./Gymbagfile` 或 `./Caddyfile`（也可通过 `--config <path>` 或 `GYMBAG_CONFIG` 指定），并原生解析 `gymbag.json`、`Gymbagfile` / `Caddyfile v2`、`nginx.conf` 以及 Apache（`httpd.conf` / `apache2.conf`）配置。

## 全局配置块 `{ ... }`

可选的顶层 `{ ... }` 块用于配置全局 TLS、ACME、网络监听、威胁情报、远程规则以及所有站点默认继承的 WAF 策略：

```caddyfile
{
    email ops@example.com
    ssl_provider letsencrypt
    # eab YOUR_EAB_KID YOUR_EAB_HMAC_KEY
    # account_key certs/acme-account.pem
    # ssl_key certs/domain.key

    bind 0.0.0.0
    http_port 80
    https_port 443
    admin 127.0.0.1:2019
    cert_dir certs
    auto_hosts on
    hsts on

    # 默认已验证状态超时（支持秒数或时长：30m、12h、7d）
    verify_timeout 24h

    # 可选 Redis，用于共享 PoW 质询与限流状态
    # redis redis://127.0.0.1:6379/0

    # IP 威胁情报与 AI 爬虫 IP 列表
    ip_security on
    ip_security_action verify
    ip_reputation on
    ai_ip_list on
    intel_refresh 6h

    # 全局远程防火墙规则（Gymbagfile 语法或 JSON）
    # remote_rules https://rules.example.com/baseline.rules
    # remote_rules_refresh 5m

    # 全局自定义规则（优先于远程规则与站点规则）
    block ip 198.51.100.99 192.0.2.0/24
}
```

### 全局指令

| 指令 | 说明 | 默认值 |
| :--- | :--- | :--- |
| `email` / `acme_email` | ACME 注册联系邮箱（同时支持 ZeroSSL 自动 EAB） | — |
| `ssl_provider` / `ca` | `letsencrypt`、`letsencrypt_staging`、`zerossl`、`google`、`ssl.com`、`litessl`、`buypass`、`local` 或自定义 URL | `auto` (`letsencrypt` / `local`) |
| `acme_directory` | 显式指定 RFC 8555 ACME v2 目录 URL | Let's Encrypt v2 |
| `eab <kid> <hmac>` | 外部账号绑定凭据（也可分开写 `eab_kid` / `eab_hmac_key`） | — |
| `account_key` | 已有 ECDSA 或 RSA ACME 账号 PEM 路径 | `<cert_dir>/acme-account.pem` |
| `ssl_key` | 已有 ACME CSR 私钥路径 | 自动生成 |
| `tls` | `auto`、`acme`、`local`、`selfsigned` 或 `<cert.pem> <key.pem>` | `auto` |
| `key_password` | 加密 PEM 私钥密码 | — |
| `cert_dir` | 证书、本地 CA、签名密钥、情报缓存与远程规则缓存目录 | `certs` |
| `renew_interval` | 证书续期检查间隔（秒数或时长如 `12h`） | `43200` (`12h`) |
| `renew_within_days` | 距离到期多少天内触发自动续期 | `30` |
| `auto_hosts` | 是否为 `localhost` / `*.internal` 自动管理 `/etc/hosts`（`on`/`off`） | `on` |
| `hsts` | 是否发送 `Strict-Transport-Security` 头（`on`/`off`） | 公网 ACME 证书默认 `on` |
| `bind` / `bind_host` | 监听绑定 IP 地址 | 本地 `127.0.0.1` / 公网 `0.0.0.0` |
| `http_port` / `https_port` | HTTP 跳转/ACME 端口与 HTTPS 网关端口 | `80` / `443` |
| `admin` | 本地 Admin REST API 监听地址（如 `127.0.0.1:2019` 或 `off`） | 关闭 |
| `redis` / `redis_url` | Redis 连接地址（`redis://[:password@]host:port/db`） | 内存存储 |
| `secret` | `waf_clearance` Cookie HMAC 签名密钥 | `<cert_dir>/secret` |
| `trusted_proxies` | 受信任的下游反代 IP/CIDR，用于解析 `X-Forwarded-For` | — |
| `ip_security` | 开启威胁情报 CIDR 列表（`on`/`off`） | `on` |
| `ip_security_action` | 命中威胁情报时的动作（`verify` 或 `block`） | `verify` |
| `ip_reputation` | 开启 Shodan InternetDB 与 StopForumSpam 实时信誉查询 | `on` |
| `ai_ip_list` | 开启官方 AI 爬虫 CIDR 列表 | `on` |
| `intel_refresh` | 情报刷新间隔（秒数或时长如 `6h`） | `21600` (`6h`) |
| `threat_feed` / `ai_ip_feed` | 追加自定义威胁或 AI IP 订阅 URL | — |
| `remote_rules` | 所有站点继承的远程防火墙规则 URL（支持一个或多个） | — |
| `remote_rules_refresh` | 远程规则刷新间隔（秒数或时长如 `5m`、`1h`；别名 `rules_refresh`） | `300` (`5m`) |
| `rules_file` | 所有站点继承的本地 `.rules`/`.json` 规则文件（`rules_file <path> [dynamic\|static]`） | — |
| `rules_dir` | 所有站点继承的本地规则目录（`rules_dir <path> [dynamic\|static]`） | — |
| `ruleset <name> { ... }` | 命名本地或远程规则源块（`source`/`file`/`dir`/`url`、`dynamic on\|off`） | — |
| `rules_watch_interval` | `dynamic` 本地规则文件与目录的轮询间隔 | `2` (`2s`) |
| `verify_timeout` / `clearance_ttl` | 默认已验证状态有效期（秒数或时长如 `30m`、`12h`、`7d`） | `86400` (`24h`) |

---

## 站点配置块

```caddyfile
app.example.com, www.example.com {
    reverse_proxy http://localhost:3000 http://localhost:3001
    verify 3
    difficulty 4
    rate_limit 240 60s
    challenge_ttl 5m
    verify_timeout 12h
    cookie_name waf_clearance
    cookie_secure true
    default_rules on
    remote_rules https://rules.example.com/app.rules

    allow ip 10.0.0.0/8
    block path /internal-only*
}
```

| 指令 | 说明 |
| :--- | :--- |
| `reverse_proxy <urls...>` | 一个或多个上游目标（轮询负载均衡，支持 `:3000` 简写） |
| `verify [0\|1\|2\|3\|off]` | 验证级别（仅写 `verify` 等价于级别 `3`） |
| `protect <on\|off>` | 开启或关闭站点 WAF 验证 |
| `difficulty <1-5>` | SHA-256 PoW 前导十六进制零难度 |
| `rate_limit <count> [window]` | 按 IP 限流次数与可选时间窗口（秒或时长，`rate_window` 同样可用） |
| `challenge_ttl <sec\|dur>` | PoW 质询过期时间（如 `300` 或 `5m`） |
| `clearance_ttl <sec\|dur>` | 已验证 Cookie 有效期（如 `86400`、`30m`、`12h`、`7d`；别名 `verify_timeout`、`verified_timeout`、`clearance_timeout`） |
| `cookie_name <name>` | 验证 Cookie 名称 |
| `cookie_secure <bool>` | Cookie 是否设置 `Secure` 属性 |
| `default_rules <on\|off>` | 开关内置基础规则 |
| `remote_rules <urls...>` | 一个或多个远程规则 URL（Gymbagfile 语法或 JSON；别名 `remote_rule`、`rules_url`、`rule_url`、`remote_rules_url`） |
| `rules_file <path> [dynamic\|static]` | 挂载本地 `.rules` 或 `.json` 规则文件（可选在磁盘变更时热重载） |
| `rules_dir <path> [dynamic\|static]` | 挂载目录中的所有 `.rules`/`.json` 规则文件（可选热重载） |
| `ruleset <name> { ... }` | 命名规则源块（`source`/`file`/`dir`/`url`、`dynamic on\|off`） |
| `handle` / `handle_path` / `route` | 有序的路径匹配路由块（`reverse_proxy`、`root` + `file_server`、`respond`、`redir`、`header`） |
| `root` / `file_server` / `try_files` | 静态文件服务、目录浏览（`browse`）与 SPA 回退 |
| `respond` / `redir` / `header` | 固定响应、HTTP 重定向（`{scheme}`、`{host}`、`{uri}`、`{path}`、`{query}`）与响应头修改 |
| `ai_bots <patterns...>` | 额外识别为 AI 爬虫的 User-Agent 子串 |
| `suspicious_uas <patterns...>` | 额外识别为可疑客户端的 User-Agent 子串 |
| `suspicious_paths <prefixes...>` | 额外识别为可疑探测的路径前缀 |
| `tls <cert> <key>` | 站点专属 SNI 证书与私钥路径 |
| `allow \| verify \| block ...` | 单行简写自定义防火墙规则 |
| `rule "name" { ... }` | 多条件组合防火墙规则块 |

---

## 时长语法

所有时间指令（`clearance_ttl` / `verify_timeout`、`challenge_ttl`、`rate_window`、`intel_refresh`、`remote_rules_refresh`、`renew_interval`）均支持：
- **纯整数秒**：`300`、`3600`、`86400`
- **Go 时长格式**：`45s`、`30m`、`12h`、`1h30m`
- **天 / 周简写**：`1d`、`7d`、`1d12h`、`2w`

---

## 格式化与校验

```bash
# 校验配置语法并打印各站点策略摘要（含 clearance_ttl 与远程规则数）
./gymbag validate --config Gymbagfile

# 就地格式化为标准 4 空格缩进
./gymbag fmt --overwrite Gymbagfile
```

## JSON 配置

`gymbag init` 默认生成 `gymbag.json`（使用 `--format gymbagfile` 可生成 Caddy 风格语法）：

```bash
./gymbag init -d app.example.com -u :3000 --verify-timeout 12h --rules-dir ./rules.d --rules-dynamic
```

所有字段与 JSON Schema 一一对应（`sites`、`bind_host`、`http_port`、`https_port`、`admin_addr`、`tls`、`intel`、`remote_rules_refresh`、`rules_watch_interval`、`redis_url`、`secret`、`trusted_proxies`、`defaults`），每个站点内包含 `routes`（`path`、`strip_prefix`、`upstreams`、`root`、`index`、`browse`、`fallback`、`respond`、`redirect`、`headers`）、`headers`、`waf.clearance_ttl`（整数秒）、`waf.remote_rules`（`[]string`）以及 `waf.rule_sources`（`[{"source", "dynamic", "name"}]`）。

您也可以将现有的 `nginx.conf`、Apache `httpd.conf`/`apache2.conf` 或 `Caddyfile` 转换为 `gymbag.json`：

```bash
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```
