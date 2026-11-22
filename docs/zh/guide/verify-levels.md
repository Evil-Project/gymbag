# 验证级别（`0`–`3`）与工作量证明

单个 Gymbag 进程可以同时托管多个网站（支持上游轮询负载均衡和 RFC 6455 WebSocket 升级），并兼顾 SSH 与 Minecraft 防护。每个站点和服务都可以通过 `verify <0-3>` 设定策略，或用 `protect off` 关闭 PoW 质询。

<VerifyLevelMatrix />

## 质询状态

Gymbag 把 PoW 质询页面 HTML、SHA-256 浏览器端求解脚本和三张吉祥物图片直接编译在二进制内部：

<div class="gb-mascot-strip">
  <div class="gb-mascot-tile">
    <img src="/mascot-pensive.png" alt="正在求解 PoW" />
    <strong>正在计算 PoW</strong>
    <span class="gb-badge challenge">已下发质询</span>
  </div>
  <div class="gb-mascot-tile">
    <img src="/mascot-happy.png" alt="已放行访客" />
    <strong>验证通过</strong>
    <span class="gb-badge pass">已签发 waf_clearance</span>
  </div>
  <div class="gb-mascot-tile">
    <img src="/mascot-reject.png" alt="已拦截请求" />
    <strong>规则 / 威胁拦截</strong>
    <span class="gb-badge block">403 Forbidden</span>
  </div>
</div>

## 多站点与多服务示例

```caddyfile
app.example.com, www.example.com {
    reverse_proxy http://localhost:3000 http://localhost:3001
    verify 3
    difficulty 4
    verify_timeout 12h
}

docs.example.com {
    reverse_proxy http://localhost:4000
    verify 2
    verify_timeout 7d
}

api.example.com {
    reverse_proxy http://localhost:8080
    verify 1
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
    verify_timeout 30m
}

static.example.com {
    reverse_proxy http://localhost:9000
    protect off
}

ssh {
    upstream 127.0.0.1:41822
    verify 3
}
```

在级别 `1` 和 `3` 下，除了威胁情报命中、Shodan/StopForumSpam 实时不良信誉、缺失浏览器请求头、自动化客户端、漏洞探测以及脚本化 SSH/Minecraft 客户端外，**VPN、代理和 Tor 出口 IP**（`vpn_detection on`）也会被归为可疑流量。

## 已验证状态有效期（`verify_timeout` / `clearance_ttl`）

在 HTTP/HTTPS 站点上，解出 SHA-256 PoW 题目后会收到一个绑定域名与客户端 IP 的 `HttpOnly`、`SameSite=Lax` HMAC 签名 Cookie（默认名为 `waf_clearance`）。在 SSH 和 Minecraft 上，通过浏览器打开验证链接并解出题目后，系统会为对应服务写入一条 IP 放行记录。

默认有效期为 **24 小时**（`86400` 秒）。可在全局或单站点/服务覆盖该时长，支持整数秒或时长字符串（`30m`、`12h`、`1h30m`、`7d`、`2w`）：

::: code-group
```caddyfile [Gymbagfile]
{
    verify_timeout 12h
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
    verify_timeout 30m
}

docs.example.com {
    reverse_proxy http://localhost:4000
    verify 2
    clearance_ttl 7d
}
```

```bash [CLI 参数]
gymbag init -d app.example.com -u :3000 --verify-timeout 6h
gymbag run --config Gymbagfile --verify-timeout 30m
gymbag reverse-proxy --from app.internal --to :3000 --verify 0 --verify-timeout 15m
```

```bash [环境变量]
GYMBAG_VERIFY_TIMEOUT=12h gymbag
GYMBAG_CLEARANCE_TTL=3600 gymbag
```

```bash [实时管理 API]
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/staging.example.com \
  -H "Content-Type: application/json" \
  -d '{"clearance_ttl": 1800}'
```
:::

- `Gymbagfile` 支持的指令别名：`verify_timeout`、`verified_timeout`、`clearance_timeout`、`clearance_ttl`。
- 支持的环境变量别名：`GYMBAG_CLEARANCE_TTL`、`GYMBAG_VERIFY_TIMEOUT`、`GYMBAG_VERIFIED_TIMEOUT`。
- `POST /.waf/v1/verifications` 会返回 `{"ok": true, "expires_in": <seconds>}` 并把 `Max-Age` 设为配置的 TTL。
- `GET /.waf/v1/session` 会在浏览器已放行时返回 `clearance_ttl`，以及 `expires_at`（RFC 3339 UTC）和 `expires_in`。

## 难度、质询窗口与限流

- `difficulty`（`1`–`5`，默认 `4`）：SHA-256 摘要需要匹配的前导十六进制零数量（难度 `4` 即 `0000...`）。
- `challenge_ttl`（默认 `300`s / `5m`）：一次性 PoW 质询 nonce 的过期时间。
- `clearance_ttl` / `verify_timeout`（默认 `86400`s / `24h`）：通行 Cookie 或 TCP IP 放行记录的存活时长。
- `rate_limit` / `rate_window`：按 IP 的固定窗口限流器。HTTP 站点默认为每 `60`s `240` 次请求；`ssh` 和 `minecraft` 块默认为每 `60`s `60` 次尝试。
