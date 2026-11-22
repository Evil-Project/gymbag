# `Gymbagfile` 与 JSON 配置参考

默认情况下，Gymbag 会在工作目录依次探测 `./gymbag.json`、`./Gymbagfile` 与 `./Caddyfile`（或使用 `--config <path>` / `GYMBAG_CONFIG` 指定）。它原生支持解析 `gymbag.json`、`Gymbagfile` / `Caddyfile v2`、`nginx.conf` 以及 Apache 配置（`httpd.conf` / `apache2.conf`）。

## 全局选项块 `{ ... }`

可选的顶层 `{ ... }` 块用于配置全局 TLS、ACME、网络监听、威胁/VPN/AI 情报源、远程规则、验证门户以及默认 WAF 策略：

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

    # 默认通行 Cookie/IP 有效期（整数秒或时长：30m, 12h, 7d）
    verify_timeout 24h

    # SSH 与 Minecraft 共享 HTTPS 验证门户
    # verify_url https://verify.example.com:8443
    # verify_port 8443

    # 可选 Redis，用于共享 PoW nonce、限流与 SSH/MC 验证凭证
    # redis redis://127.0.0.1:6379/0

    # IP 威胁情报、VPN/Tor 检测与 AI 爬虫 IP 列表
    ip_security on
    ip_security_action verify
    ip_reputation on
    vpn_detection on
    ai_ip_list on
    intel_refresh 6h

    # 共享远程防火墙规则（Gymbagfile 语法或 JSON）
    # remote_rules https://rules.example.com/baseline.rules
    # remote_rules_refresh 5m

    # 先于远程规则与站点规则执行的全局自定义规则
    block ip 198.51.100.99 192.0.2.0/24
}
```

### 全局指令

| 指令 | 说明 | 默认值 |
| :--- | :--- | :--- |
| `email` / `acme_email` | ACME 注册联系邮箱（同时用于 ZeroSSL 自动 EAB） | — |
| `ssl_provider` / `ca` | `letsencrypt`、`letsencrypt_staging`、`zerossl`、`google`、`ssl.com`、`litessl`、`buypass`、`local` 或自定义 URL | `auto`（`letsencrypt` / `local`） |
| `acme_directory` | 显式指定 RFC 8555 ACME v2 目录 URL | Let's Encrypt v2 |
| `eab <kid> <hmac>` | 外部账户绑定凭据（也支持 `eab_kid` / `eab_hmac_key`） | — |
| `account_key` | 既有的 ECDSA 或 RSA ACME 账户 PEM 路径 | `<cert_dir>/acme-account.pem` |
| `ssl_key` | 用于签发 ACME CSR 的既有私钥路径 | 自动生成 |
| `tls` | `auto`、`acme`、`local`、`selfsigned` 或 `<cert.pem> <key.pem>` | `auto` |
| `key_password` | 加密 PEM 私钥的解密口令 | — |
| `cert_dir` | 证书、本地 CA、密钥、情报缓存与规则缓存目录 | `certs` |
| `renew_interval` | 证书续期巡检间隔（整数秒或 `12h` 等时长） | `43200`（`12h`） |
| `renew_within_days` | 距离过期不足 N 天时执行续期 | `30` |
| `auto_hosts` | 管理 `localhost` / `*.internal` 的 `/etc/hosts` 条目（`on`/`off`） | `on` |
| `hsts` | 下发 `Strict-Transport-Security` 响应头（`on`/`off`） | 公网 ACME 证书默认 `on` |
| `bind` / `bind_host` | 监听绑定 IP | `127.0.0.1`（仅本地 HTTP）/ `0.0.0.0`（公网 HTTP、SSH 或 MC） |
| `http_port` / `https_port` | HTTP 重定向/ACME 端口与 HTTPS 网关端口 | `80` / `443` |
| `verify_url` | SSH/Minecraft 验证门户公开 HTTPS URL | 服务器 IP + IP SSL 证书 |
| `verify_port` | SSH/Minecraft 验证门户共享 HTTPS 端口 | `8443` |
| `verify_server { ... }` | 可选块，支持 `url`、`port` 和 `tls <cert> <key>` | — |
| `admin` | 本地 Admin REST API 监听地址（如 `127.0.0.1:2019` 或 `off`） | 关闭 |
| `redis` / `redis_url` | Redis 连接地址（`redis://` 或 `rediss://`） | 内存存储 |
| `secret` | `waf_clearance` Cookie 的 HMAC 密钥 | `<cert_dir>/secret` |
| `trusted_proxies` | 解析 `X-Forwarded-For` 时信任的下游代理 IP/CIDR | — |
| `ip_security` | 启用威胁情报 CIDR 列表（`on`/`off`） | `on` |
| `ip_security_action` | 威胁情报命中动作（`verify` 或 `block`） | `verify` |
| `ip_reputation` | 启用 Shodan InternetDB 与 StopForumSpam 实时查询（开启 `vpn_detection` 时同时包含实时 VPN API 查询） | `on` |
| `vpn_detection` / `vpn_detect` | 在验证级别 `1` 和 `3` 下将免费 VPN/代理/Tor IP 判定为可疑（`on`/`off`） | `on` |
| `ai_ip_list` | 启用官方 AI 爬虫 CIDR 列表（`on`/`off`） | `on` |
| `intel_refresh` | 情报源刷新间隔（整数秒或 `6h` 等时长） | `21600`（`6h`） |
| `threat_feed` / `ai_ip_feed` / `vpn_feed` | 追加自定义 IP 列表 URL（复数 `threat_feeds`、`ai_ip_feeds`、`vpn_feeds` 同样支持） | — |
| `remote_rules` | 所有站点继承的 HTTP/HTTPS 远程防火墙规则 URL | — |
| `remote_rules_refresh` | 远程规则 URL 刷新间隔（秒数或时长；别名 `rules_refresh`） | `300`（`5m`） |
| `rules_file` | 本地 `.rules`/`.json` 规则文件（`rules_file <path> [dynamic\|static]`） | — |
| `rules_dir` | 本地规则目录（`rules_dir <path> [dynamic\|static]`） | — |
| `ruleset <name> { ... }` | 命名本地或远程规则来源块（`source`/`file`/`dir`/`url`，`dynamic on\|off`） | — |
| `rules_watch_interval` | `dynamic` 本地规则文件与目录轮询间隔 | `2`（`2s`） |
| `verify_timeout` / `clearance_ttl` | 默认通行有效期（秒数或 `30m`、`12h`、`7d` 等时长） | `86400`（`24h`） |

## 站点块指令

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
| `reverse_proxy <urls...>` | 一个或多个上游目标（轮询负载均衡；支持 `:3000` 简写） |
| `verify [0\|1\|2\|3\|off]` | 验证级别（不带参数写 `verify` 等同级别 `3`） |
| `protect <on\|off>` | 开关 WAF 验证 |
| `difficulty <1-5>` | SHA-256 前导十六进制零 PoW 难度 |
| `rate_limit <count> [window]` | 单 IP 限流阈值与可选时间窗（同时支持 `rate_window`） |
| `challenge_ttl <sec\|dur>` | PoW 质询 nonce 过期时间（如 `300` 或 `5m`） |
| `clearance_ttl <sec\|dur>` | 已验证放行有效期（别名：`verify_timeout`、`verified_timeout`、`clearance_timeout`） |
| `cookie_name <name>` | 通行 Cookie 名称 |
| `cookie_secure <bool>` | 通行 Cookie 是否附加 `Secure` 属性 |
| `default_rules <on\|off>` | 开关内置基线规则 |
| `remote_rules <urls...>` | 远程防火墙规则 URL（Gymbagfile 语法或 JSON） |
| `rules_file <path> [dynamic\|static]` | 本地 `.rules` 或 `.json` 文件（可选热重载） |
| `rules_dir <path> [dynamic\|static]` | 包含 `.rules`/`.json` 文件的目录（可选热重载） |
| `ruleset <name> { ... }` | 命名规则来源块（`source`/`file`/`dir`/`url`，`dynamic on\|off`） |
| `handle` / `handle_path` / `route` | 有序路径匹配路由块（`reverse_proxy`、`root` + `file_server`、`respond`、`redir`、`header`） |
| `root` / `file_server` / `try_files` | 静态文件托管、目录浏览（`browse`）与 SPA 回退 |
| `respond` / `redir` / `header` | 固定响应、重定向（`{scheme}`、`{host}`、`{uri}`、`{path}`、`{query}`）与响应头修改 |
| `ai_bots <patterns...>` | 额外识别为 AI 爬虫的 User-Agent 子串 |
| `suspicious_uas <patterns...>` | 额外判定为可疑客户端的 User-Agent 子串 |
| `suspicious_paths <prefixes...>` | 额外判定为可疑探测的路径前缀 |
| `tls <cert> <key>` | 按站点配置的 SNI 证书和私钥路径 |
| `allow \| verify \| block ...` | 单行简写自定义防火墙规则 |
| `rule "name" { ... }` | 多条件自定义防火墙规则块 |

## `ssh` 与 `minecraft` 服务块

顶层 `ssh [port] { ... }` 和 `minecraft [port] { ... }`（别名 `mc`）块用于配置 TCP PoW 拦截：

```caddyfile
ssh {
    port 22
    upstream 127.0.0.1:41822
    verify 3
    difficulty 4
    rate_limit 60 60s
    clearance_ttl 24h
    allow ip 10.0.0.0/8
}

minecraft {
    port 25565
    upstream 127.0.0.1:48291
    server_properties /srv/mc/server.properties
    verify 3
    rate_limit 60 60s
}
```

| 指令 | 说明 | 默认值 |
| :--- | :--- | :--- |
| `enabled <bool>` | 开关该服务块而不删除配置 | `true` |
| `port <port>` | 对外监听端口（也可直接写成 `ssh 2222 { ... }`） | `22`（SSH）/ `25565`（Minecraft） |
| `upstream <host:port>` | 后端 TCP 地址（也支持 `reverse_proxy` / `proxy`，如 `:2222`、`tcp://127.0.0.1:25566`） | —（必填） |
| `server_properties <path>` | `server.properties` 文件路径（仅 `minecraft` 块支持） | — |
| `verify [0\|1\|2\|3\|off]` | 验证级别（不带参数写 `verify` 为 `3`） | `3` |
| `protect <on\|off>` | 开关该服务的 PoW 验证 | `on` |
| `difficulty <1-5>` | PoW 前导十六进制零难度 | `4` |
| `rate_limit <count> [window]` | 单 IP 连接速率限制与窗口 | `60` / `60s` |
| `challenge_ttl <sec\|dur>` | 验证凭证过期时间 | `300`（`5m`） |
| `clearance_ttl <sec\|dur>` | IP 放行存活时间（也支持 `verify_timeout`） | `86400`（`24h`） |
| `verify_url` / `verify_port` | 覆盖断开提示中打印的验证链接地址或端口 | 全局 `verify_url` / `verify_port` |
| `allow \| verify \| block ...` / `rule` | 匹配 IP/CIDR 或客户端 banner/用户名（`user_agent`）的自定义规则 | — |

## 时长语法

时间类指令（`clearance_ttl` / `verify_timeout`、`challenge_ttl`、`rate_window`、`intel_refresh`、`remote_rules_refresh`、`rules_watch_interval` 以及 `renew_interval`）支持以下格式：
- 纯整数秒：`300`、`3600`、`86400`
- Go 时长格式：`45s`、`30m`、`12h`、`1h30m`
- 天与周简写：`1d`、`7d`、`1d12h`、`2w`

## 格式化与校验

```bash
./gymbag validate --config Gymbagfile
./gymbag fmt --overwrite Gymbagfile
```

## JSON 配置（`gymbag.json`）

`gymbag init` 默认生成精简的 `gymbag.json`，省略未启用的服务和默认值（传 `--format gymbagfile` 可切换为 Caddy 风格）：

```bash
./gymbag init -d app.example.com -u :3000 --verify-timeout 12h --rules-dir ./rules.d --rules-dynamic
```

顶层 JSON 字段（解析时强制开启 `DisallowUnknownFields`）：
- `sites`：`{domain, upstreams, routes, headers, protect, waf, cert_file, key_file}` 对象数组，其中 `waf` 包含 `{verify_level, difficulty, challenge_ttl, clearance_ttl, cookie_name, cookie_secure, rate_limit, rate_window, default_rules, rules, remote_rules, rule_sources, custom_ai_bots, custom_suspicious_uas, custom_suspicious_paths}`。
- `ssh`：`{enabled, port, upstream, protect, verify_level, difficulty, challenge_ttl, clearance_ttl, rate_limit, rate_window, verify_url, verify_port, rules}`。
- `minecraft`：`{enabled, port, upstream, server_properties, protect, verify_level, difficulty, challenge_ttl, clearance_ttl, rate_limit, rate_window, verify_url, verify_port, rules}`。
- `verify`：`{url, port, cert_file, key_file}`。
- `bind_host`、`http_port`、`https_port`、`admin_addr`、`remote_rules_refresh`、`rules_watch_interval`、`redis_url`、`secret`、`trusted_proxies`。
- `tls`：`{mode, provider, cert_file, key_file, key_password, account_key_file, eab_kid, eab_hmac_key, auto_hosts, cert_dir, acme_email, acme_directory, renew_interval, renew_within_days, hsts}`。
- `intel`：`{ip_security, ip_security_action, live_reputation, ai_ip_list, vpn_detection, refresh_interval, threat_feeds, ai_ip_feeds, vpn_feeds}`。
- `defaults`：全局共享的基础 `waf` 对象。

也可以将 `nginx.conf`、Apache `httpd.conf`/`apache2.conf` 或 `Caddyfile` 转换为 `gymbag.json`：

```bash
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```
