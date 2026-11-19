# 验证级别 (`0`–`3`)、工作量证明与自定义超时

单个 Gymbag 进程可同时托管并保护数十个域名，支持多上游轮询负载均衡与全双工 RFC 6455 WebSocket 代理。每个站点均可通过 `verify <0-3>` 或 `protect off` 独立配置验证策略。

<VerifyLevelMatrix />

## 质询与拦截状态页

Gymbag 将 PoW 浏览器挑战页面、SHA-256 求解脚本与三态吉祥物直接嵌入 Go 二进制文件中：

<div class="gb-mascot-strip">
  <div class="gb-mascot-tile">
    <img src="/mascot-pensive.png" alt="正在求解 PoW" />
    <strong>正在计算 PoW</strong>
    <span class="gb-badge challenge">已下发质询</span>
  </div>
  <div class="gb-mascot-tile">
    <img src="/mascot-happy.png" alt="验证通过" />
    <strong>验证通过</strong>
    <span class="gb-badge pass">已签发 waf_clearance</span>
  </div>
  <div class="gb-mascot-tile">
    <img src="/mascot-reject.png" alt="请求被拦截" />
    <strong>规则 / 威胁拦截</strong>
    <span class="gb-badge block">403 Forbidden</span>
  </div>
</div>

## 多站点配置示例

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
```

## 自定义已验证状态有效期 (`verify_timeout` / `clearance_ttl`)

访客完成 SHA-256 工作量证明后，Gymbag 会签发一个绑定域名与客户端 IP 的 `HttpOnly`、`SameSite=Lax` HMAC 签名 Cookie（默认名称为 `waf_clearance`）。

默认验证有效期为 **24 小时**（`86400` 秒）。您可以全局或按站点自定义有效期，支持整数秒**或**可读时长格式（如 `30m`、`12h`、`1h30m`、`7d`、`2w`）：

::: code-group
```caddyfile [Gymbagfile]
{
    # 为所有站点设置默认已验证有效期
    verify_timeout 12h
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
    # 测试环境设置较短的 30 分钟有效期
    verify_timeout 30m
}

docs.example.com {
    reverse_proxy http://localhost:4000
    verify 2
    # 文档站设置较长的 7 天有效期（clearance_ttl 别名同样可用）
    clearance_ttl 7d
}
```

```bash [CLI 参数]
# 生成带 6 小时验证有效期的配置
gymbag init -d app.example.com -u :3000 --verify-timeout 6h

# 运行时覆盖有效期
gymbag run --config Gymbagfile --verify-timeout 30m
gymbag reverse-proxy --from app.internal --to :3000 --verify 0 --verify-timeout 15m
```

```bash [环境变量]
# 支持整数秒或时长格式（如 30m、12h、7d）
GYMBAG_VERIFY_TIMEOUT=12h gymbag
GYMBAG_CLEARANCE_TTL=3600 gymbag
```

```bash [实时管理 API]
# 无需重启即可动态修改站点 clearance_ttl（单位：秒）
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/staging.example.com \
  -H "Content-Type: application/json" \
  -d '{"clearance_ttl": 1800}'
```
:::

- `Gymbagfile` 支持的指令别名：`verify_timeout`、`verified_timeout`、`clearance_timeout` 和 `clearance_ttl`。
- 支持的环境变量别名：`GYMBAG_CLEARANCE_TTL`、`GYMBAG_VERIFY_TIMEOUT` 和 `GYMBAG_VERIFIED_TIMEOUT`。
- `POST /.waf/v1/verifications` 会返回 `{"ok": true, "expires_in": <秒数>}` 并将 Cookie `Max-Age` 设为对应有效期。
- `GET /.waf/v1/session` 会在访客已通过验证时返回 `clearance_ttl`、`expires_at`（RFC 3339 UTC）与剩余 `expires_in` 秒数。

## 难度、质询窗口与限流

- **`difficulty`**（`1`–`5`，默认 `4`）：SHA-256 哈希所需的前导十六进制零个数（难度 `4` 即 `0000...`）。
- **`challenge_ttl`**（默认 `300` 秒 / `5m`）：单次 PoW 质询的有效期，支持秒数或时长字符串。
- **`clearance_ttl` / `verify_timeout`**（默认 `86400` 秒 / `24h`）：已验证 Cookie 的有效期，支持秒数或时长字符串（如 `30m`、`12h`、`7d`）。
- **`rate_limit` / `rate_window`**（默认每 `60` 秒 `240` 次请求）：基于 64 分片内存存储（或 Redis）的按站点、按 IP 固定窗口限流器。
