# 防火墙规则、远程规则与威胁情报

## 请求评估流水线

对于受保护站点上的每一个请求，Gymbag 按严格顺序依次评估各安全层：

1. **请求形状校验** — 检查 HTTP 方法、最大 URI 长度、空字节及 `..` / `%2e%2e` 路径穿越。
2. **ACME HTTP-01 放行** — `/.well-known/acme-challenge/*` 始终放行，确保证书签发不受影响。
3. **本地自定义规则** — 先执行全局规则，再执行站点规则，从上到下**首次命中即生效**。
4. **远程规则 (`remote_rules`)** — 从配置的 HTTP/HTTPS URL 拉取并校验通过的规则，按声明顺序评估。
5. **内置基础规则** — 拦截敏感文件、CMS 漏洞探测、扫描器、SQLi/XSS 查询，并质询自动化客户端（除非设置 `default_rules off`）。
6. **按站点、按 IP 限流** — 可配置 `rate_limit` 与 `rate_window`。
7. **IP 威胁情报** — 执行 `ip_security_action`（默认 `verify`，可设为 `block`）。
8. **PoW 验证** — 根据配置的 `verify` 级别（`0`–`3`）执行质询或放行。

由于本地自定义规则优先级**高于**远程规则、内置规则、限流和威胁情报库，在顶部添加 `allow` 规则即可 cleanly 放行办公网段、内部健康检查或预览 Cookie。

---

## 单行简写规则

语法：`<allow | verify | block> <field> <pattern...>`（同时支持 `bypass`/`pass`、`challenge`/`pow` 与 `deny`/`drop`/`reject` 别名）。

| 字段 | 匹配语义 | 示例 |
| :--- | :--- | :--- |
| `ip` | IPv4、IPv6 或 CIDR 网段 | `allow ip 127.0.0.1 10.0.0.0/8 2001:db8::/32` |
| `user_agent` | 不区分大小写子串、`*` 通配符或 `~正则` | `block user_agent BadBot ~^(?i)python-urllib/` |
| `path` | 精确/目录前缀、`*` 通配符或 `~正则` | `verify path /api/export* ~^/v[0-9]+/search` |
| `query` | 原始/解码后查询串子串、`*` 通配符或 `~正则` | `block query "union select" ~(?i)<script` |
| `method` | HTTP 请求方法（不区分大小写） | `verify method POST DELETE` |
| `header` | `<Header-Name> [pattern]`（`""` = 缺失/空，`*` = 存在） | `verify header Accept-Language ""` |
| `cookie` | `<cookie_name> [pattern]`（`""` = 缺失/空，`*` = 存在） | `allow cookie vip_session *` |

---

## 多条件组合 `rule` 块

当单个字段不足以描述条件时，可使用 `rule "name" { ... }`。**同一字段**的多个值按 **OR** 组合；同一 `rule` 块内**不同字段**按 **AND** 组合：

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

---

## 远程规则 (`remote_rules`)

您可以从一个或多个 HTTP/HTTPS URL（如 Git 仓库 Raw 地址、S3/R2 存储桶、CDN 或内部策略服务器）拉取共享或集中管理的防火墙规则。Gymbag 会在启动时及每隔 `remote_rules_refresh`（默认 `5m` / `300` 秒）自动后台更新。

### 配置远程规则

支持在全局块（所有站点继承）或单个站点块中配置：

::: code-group
```caddyfile [Gymbagfile]
{
    # 全局共享规则，应用到所有站点
    remote_rules https://rules.example.com/org-baseline.rules
    remote_rules_refresh 5m
}

app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    # 本地规则永远优先于远程规则
    allow ip 10.0.0.0/8

    # 站点专用远程规则（纯文本或 JSON）
    remote_rules https://rules.example.com/app-overrides.json
}
```

```bash [CLI 与环境变量]
# CLI 参数（可重复或逗号分隔）
gymbag run --config Gymbagfile \
  --remote-rules https://rules.example.com/baseline.rules \
  --remote-rules-refresh 10m

# 环境变量
GYMBAG_REMOTE_RULES="https://rules.example.com/baseline.rules,https://rules.example.com/extra.json" \
GYMBAG_REMOTE_RULES_REFRESH=5m \
gymbag
```

```bash [Admin REST API]
# 查看远程规则同步状态、数量与错误信息
curl http://127.0.0.1:2019/api/v1/rules/remote

# 立即触发全量拉取并热重载 WAF 引擎
curl -X POST http://127.0.0.1:2019/api/v1/rules/remote/refresh

# 在线为单个站点绑定或更新远程规则 URL
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"remote_rules": ["https://rules.example.com/emergency.rules"]}'
```
:::

### 支持的远程规则格式

Gymbag 自动识别**纯文本 Gymbagfile 规则语法**与 **JSON**：

::: code-group
```caddyfile [纯文本 Gymbagfile 语法]
# 共享远程防火墙规则
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

```json [JSON 对象（或 JSON 数组）]
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

### 缓存与故障安全保障

- **条件请求**：自动发送 `If-None-Match` (`ETag`) 与 `If-Modified-Since` 请求头；返回 `304 Not Modified` 时跳过重新编译。
- **磁盘缓存**：已验证的规则缓存在 `<cert_dir>/rules-cache/` 中，冷启动时在首次网络请求完成前即可立即预加载。
- **故障安全校验**：每个远程文件激活前均经过严格语法和字段校验（单文件上限 4 MiB，15 秒超时）。若远端返回 HTTP 错误或语法有误，Gymbag 会保留上一份有效的规则集，并在 `GET /api/v1/rules/remote` 中展示具体错误。
- **零停机原子切换**：当规则内容发生变化时，Gymbag 会在后台重新编译 WAF 引擎（含 O(1) 路径表与 Aho-Corasick DFA）并原子替换路由快照，不中断现存连接。

---

## 本地规则文件、规则目录与动态热重载

您可以将规则存放在独立的 `.rules` / `.json` 文件或整个规则目录中并按路径引用。将规则源标记为 `dynamic`，即可在磁盘文件变更时自动监听并热重载，无需重启服务（若文件语法有误，Gymbag 会保留上一份有效的规则集）。本地规则与内联规则一同在远程规则之前评估。

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
      "domains": ["app.example.com"],
      "rule_sources": [
        {"source": "./rules.d", "dynamic": true, "name": "shared"}
      ]
    }
  ]
}
```

```bash [CLI]
# 生成并关联本地规则源的配置
gymbag init -d app.example.com -u :3000 \
  --rules-file ./rules/custom.rules --rules-dir ./rules.d \
  --rules-dynamic --rules-watch-interval 2s

# 管理规则文件与关联规则源
gymbag ruleset init ./rules/custom.rules
gymbag ruleset add --source ./rules --dynamic --name local-dir
gymbag ruleset update local-dir --static
gymbag ruleset validate ./rules
gymbag ruleset rule add --file ./rules/custom.rules --action block --name bad-bot --ua "*BadBot*"
gymbag ruleset remove local-dir
```
:::

轮询间隔由 `rules_watch_interval` 控制（默认 `2s`；可通过 `GYMBAG_RULES_WATCH_INTERVAL` 或 `gymbag config set rules_watch_interval 2s` 设置）。`rule_sources` 条目也可通过 `PATCH /api/v1/sites/{domain}` 实时更新。所有 `ruleset` 子命令请参阅 [CLI 与环境变量](/zh/reference/cli-env)。

---

## 内置规则开关 (`default_rules on|off`)

`default_rules` 默认为 `on`：
- 当站点**未配置**本地或远程自定义规则时，Gymbag 同时启用内置拦截规则（`/.env`、`/.git/*`、`/wp-admin/*`、`sqlmap`/`nuclei`、SQLi/XSS 查询）和自动化质询规则（`curl`、`wget`、`python-requests`、`headlesschrome`、`playwright`、空 `User-Agent`）。
- 当您**添加了**本地或远程自定义规则时，您的规则优先执行，内置**拦截**规则仍会追加在后，避免因添加一条 `verify path` 规则而意外暴露 `/.env`。
- 若需要完全手动控制，可对站点或全局设置 `default_rules off`。

---

## 自动更新威胁与 AI 爬虫情报源

所有情报源在启动时及每隔 `intel_refresh`（默认 `21600` 秒 / `6h`）自动更新，并采用 `ETag` / `If-Modified-Since` 条件请求缓存至 `<cert_dir>/intel-cache`：

- **威胁情报源 (`ip_security on`)**：IPsum level 3+、Spamhaus DROP IPv4/IPv6、abuse.ch Feodo Tracker。
- **实时 IP 信誉 (`ip_reputation on`)**：免密钥查询 Shodan InternetDB 与 StopForumSpam，配备内存 TTL 缓存和超时快速降级（fail-open）。
- **官方 AI 爬虫 IP 段 (`ai_ip_list on`)**：OpenAI（`gptbot`、`searchbot`、`chatgpt-user`）、Anthropic（`claude.com/crawling/bots.json`）、Perplexity、Google 与 Apple（`applebot`）。

```caddyfile
{
    ip_security on
    ip_security_action block   # "verify" (PoW 质询) 或 "block" (403 拦截)
    ip_reputation on
    ai_ip_list on
    intel_refresh 6h
}
```
