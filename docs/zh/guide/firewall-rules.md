# 防火墙规则、远程规则与威胁情报

## 请求检查流水线

受保护的 HTTP 站点会按固定顺序依次执行下列检查：

1. **请求格式检查**：校验 HTTP 方法、限制最大 URI 长度，拦截空字节和 `..` / `%2e%2e` 路径穿越。
2. **ACME HTTP-01 放行**：`/.well-known/acme-challenge/*` 始终放行，保证证书签发和续期不受干扰。
3. **本地自定义规则**：先全局规则，再站点规则，自上而下按首条命中生效。
4. **远程规则（`remote_rules`）**：从 HTTP/HTTPS URL 拉取并校验过的规则按顺序执行。
5. **内置基线规则**：敏感路径、CMS 探测、扫描器、SQLi/XSS 查询以及自动化客户端 PoW 规则（除非设置 `default_rules off`）。
6. **按站点按 IP 限流器**：`rate_limit` 与 `rate_window`（默认每 `60s` `240` 次）。
7. **IP 威胁情报**：`ip_security_action verify`（默认）或 `block`。
8. **工作量证明校验**：按配置的 `verify` 级别（`0`–`3`）判定，包含 VPN/代理/Tor 检测（`1` 与 `3`）和 AI 爬虫匹配（`2` 与 `3`）。

由于本地自定义规则排在远程规则、基线规则、限流器和威胁情报之前，把 `allow` 放在站点块顶部就能直接放行办公网 CIDR、健康检查或带预览 Cookie 的请求，不需要额外绕弯。写在 `{ ... }` 里的全局规则同样会在 `ssh` 和 `minecraft` 服务上生效（与 `ssh { ... }`、`minecraft { ... }` 内的单服务规则一起执行）。

## 单行简写规则

语法：`<allow | verify | block> <field> <pattern...>`（同时支持别名 `bypass`/`pass`、`challenge`/`pow` 以及 `deny`/`drop`/`reject`）。

| 字段 | 匹配语义 | 示例 |
| :--- | :--- | :--- |
| `ip` | IPv4、IPv6 或 CIDR 网段 | `allow ip 127.0.0.1 10.0.0.0/8 2001:db8::/32` |
| `user_agent` | 忽略大小写子串、`*` 通配符或 `~regex` 正则 | `block user_agent BadBot ~^(?i)python-urllib/` |
| `path` | 精确/前缀、`*` 通配符或 `~regex` 正则 | `verify path /api/export* ~^/v[0-9]+/search` |
| `query` | 原始/解码后查询串匹配、`*` 通配符或 `~regex` 正则 | `block query "union select" ~(?i)<script` |
| `method` | HTTP 请求方法（忽略大小写） | `verify method POST DELETE` |
| `header` | `<Header-Name> [pattern]`（`""` = 缺失/为空，`*` = 存在） | `verify header Accept-Language ""` |
| `cookie` | `<cookie_name> [pattern]`（`""` = 缺失/为空，`*` = 存在） | `allow cookie vip_session *` |

## 多条件 `rule` 块

当多个条件需要组合判断时使用 `rule "name" { ... }`。同一字段上的多个值之间是 OR 关系，同一个规则块内不同字段之间是 AND 关系：

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

## 远程规则（`remote_rules`）

可以从一个或多个 HTTP/HTTPS URL（Git raw 文件、对象存储、CDN 或策略下发服务）拉取共享规则集。Gymbag 在启动时拉取一次，之后每隔 `remote_rules_refresh`（默认 `5m` / `300s`）刷新一次：

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

```bash [CLI 与环境变量]
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

### 支持的远程规则格式

同时兼容 **Gymbagfile 规则语法**与 **JSON**（`[ ... ]` 或 `{"rules": [ ... ]}`）：

::: code-group
```caddyfile [纯文本 Gymbagfile 语法]
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

```json [JSON 对象]
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

### 缓存与校验

- 请求携带 `If-None-Match`（`ETag`）和 `If-Modified-Since`；收到 `304 Not Modified` 时跳过重新编译规则集。
- 已校验的规则载荷缓存在 `<cert_dir>/rules-cache/`，启动时不等第一次网络请求完成就会先热加载本地缓存。
- 每次更新都会先完成校验（4 MiB 大小限制，15s 超时）再切换。若上游返回语法错误或 HTTP 错误，原规则集继续生效，错误信息会显示在 `GET /api/v1/rules/remote` 中。
- 规则更新采用原子切换，不断开既有连接。

## 本地规则文件、目录与热重载

可将规则拆分到独立的 `.rules` / `.json` 文件或目录中。给来源加上 `dynamic` 标记后，系统会监听磁盘变更并自动重载：

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

轮询间隔由 `rules_watch_interval` 控制（默认 `2s`）。`rule_sources` 也可通过 `PATCH /api/v1/sites/{domain}` 动态修改。完整 `ruleset` 子命令列表见 [CLI 与环境变量](/zh/reference/cli-env)。

## 内置规则（`default_rules on|off`）

`default_rules` 默认为 `on`：
- 当站点**没有**配置任何本地或远程自定义规则时，同时启用内置拦截规则（`/.env`、`/.git/*`、`/wp-admin/*`、`sqlmap`/`nuclei`、SQLi/XSS 查询）和自动化客户端 PoW 规则（`curl`、`wget`、`python-requests`、`headlesschrome`、`playwright`、空 `User-Agent`）。
- 一旦站点配置了自定义规则，自定义规则优先执行，内置 **block** 规则仍保留在后面，防止只加了一条 `verify path` 就把 `/.env` 暴露出去。
- 在站点或全局设置 `default_rules off` 可彻底禁用内置规则。

## 威胁情报源、VPN/Tor 检测与 AI 爬虫列表

情报源在启动时和每 `intel_refresh`（默认 `21600`s / `6h`）刷新一次，缓存在 `<cert_dir>/intel-cache`：

- 威胁情报源（`ip_security on`）：IPsum level 3+、Spamhaus DROP IPv4 & IPv6、abuse.ch Feodo Tracker。
- 实时 IP 信誉（`ip_reputation on`）：免密钥查询 Shodan InternetDB 和 StopForumSpam，带 1 小时 TTL 缓存和故障放行超时。
- VPN / 代理 / Tor 检测（`vpn_detection on`）：自动更新的 X4BNet VPN IPv4/IPv6 列表和 Tor 出口节点列表，外加对 `api.ipquery.io` 和 `api.ipapi.is` 的免密钥实时查询（1.2s 超时、singleflight 去重、1 小时缓存、故障放行）。在 HTTP、SSH 和 Minecraft 的验证级别 `1` 和 `3` 中，命中 IP 都会判定为 **suspicious**。
- 官方 AI 爬虫 CIDR（`ai_ip_list on`）：OpenAI（`gptbot`、`searchbot`、`chatgpt-user`）、Anthropic（`claude.com/crawling/bots.json`）、Perplexity、Google 与 Apple（`applebot`）。

```caddyfile
{
    ip_security on
    ip_security_action block   # "verify" (PoW 质询) 或 "block" (403 Forbidden)
    ip_reputation on
    vpn_detection on
    ai_ip_list on
    intel_refresh 6h
    # vpn_feed https://rules.example.com/extra-vpn-cidrs.txt
}
```
