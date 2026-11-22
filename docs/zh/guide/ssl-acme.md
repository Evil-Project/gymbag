# 多 CA ACME、IP 证书与自定义证书

对于公网域名，默认的 `tls auto` 会通过 RFC 8555 ACME HTTP-01 自动申请和续期证书。

## 支持的证书颁发机构（CA）

使用 `ssl_provider`（别名：`acme_provider`、`ca`、`cert_issuer`）或 `--ssl-provider` 指定 CA：

| 提供商关键字 | 证书颁发机构 | EAB 要求 |
| :--- | :--- | :--- |
| `letsencrypt` (`le`) | Let's Encrypt（生产环境） | 无 |
| `letsencrypt_staging` | Let's Encrypt（测试环境） | 无 |
| `zerossl` | ZeroSSL | 配置 `email` 后**自动获取**，或显式提供 `eab <kid> <hmac_key>` |
| `google` (`gts`) | Google Trust Services | 需要 `eab <kid> <hmac_key>` |
| `ssl.com` (`sslcom`, `ssl.com-rsa`) | SSL.com（ECC 或 RSA） | 需要 `eab <kid> <hmac_key>` |
| `litessl` (`trustasia`) | LiteSSL by TrustAsia | 标准 DV 无需 |
| `buypass` | Buypass Go SSL | 无 |
| `local` | 本地双层 PKI | 本地 CA（`Neuro-sama Headquarters Root CA`） |
| `https://...` | 任意 RFC 8555 ACME v2 CA | 可选 `eab <kid> <hmac_key>`（如 `step-ca`、Pebble） |

## 各提供商配置示例

### ZeroSSL（自动获取 EAB）

```caddyfile
{
    email ops@example.com
    ssl_provider zerossl
}

example.com {
    reverse_proxy http://localhost:3000
    verify 3
}
```

### Google Trust Services / SSL.com（显式 EAB）

标准填充 base64 与无填充 URL-safe base64 格式的 HMAC 密钥均可使用。Admin API 返回时会自动脱敏 `eab_hmac_key`。

```caddyfile
{
    email ops@example.com
    ssl_provider google
    eab YOUR_EAB_KEY_ID YOUR_EAB_HMAC_KEY
}
```

### 自定义 ACME 目录 URL

```caddyfile
{
    email ops@example.com
    ssl_provider https://ca.internal.corp/acme/acme/directory
}
```

ACME 证书按提供商分别保存在 `<cert_dir>` 下（例如 `certs/acme-zerossl-fullchain.pem`，Let's Encrypt 则为 `certs/acme-fullchain.pem`），因此更换 `ssl_provider` 后会直接向新的 CA 申请，不会沿用旧提供商的缓存证书。

## RFC 8738 IP 证书（SSH 与 Minecraft 验证门户）

当启用了 SSH 或 Minecraft 防护且未配置自定义 `verify_url` 时，Gymbag 会在探测到的服务器 IP 地址上提供 HTTPS 验证门户（默认端口 `8443`）。

在 `auto` 模式下（`tls auto` 搭配非 `local` 提供商），Gymbag 会为公网服务器 IP 申请 **RFC 8738 ACME IP HTTP-01 证书**——若 ACME 目录声明了 `shortlived` profile 则自动使用——并缓存在 `certs/verify-acme-fullchain.pem`（其他 CA 为 `verify-acme-<provider>-fullchain.pem`）。如果 `80` 端口不可达或 CA 拒绝 IP 标识符，Gymbag 会回退到本地 CA（`certs/verify-local-fullchain.pem`）或自签名 IP 证书（`certs/verify-selfsigned.pem`）。

也可以为验证门户指定独立证书：

```caddyfile
{
    verify_url https://verify.example.com:8443
    verify_server {
        tls /etc/ssl/verify-fullchain.pem /etc/ssl/verify-privkey.pem
    }
}
```

## 自有密钥与证书

- ACME 账户密钥：`account_key /etc/gymbag/account.pem`（ECDSA P-256/P-384 或 RSA）
- CSR 私钥：`ssl_key /etc/gymbag/domain.key`
- PEM 证书与私钥（全局或按站点 SNI）：`tls /etc/ssl/corp-fullchain.pem /etc/ssl/corp-privkey.pem`
- 加密私钥口令：`key_password super-secret-passphrase`（或 `GYMBAG_KEY_PASSWORD`）

```caddyfile
{
    key_password super-secret-passphrase
}

corp.example.com {
    reverse_proxy http://localhost:8080
    tls /etc/ssl/corp-fullchain.pem /etc/ssl/corp-privkey.pem
    verify 1
}
```

## 后台自动续期

后台协程每隔 `renew_interval` 秒（默认 `43200`，即 12 小时）巡检一次证书。距过期不足 `renew_within_days` 天（默认 `30` 天）或在磁盘上被替换的证书都会自动续期或重载，并通过 `atomic.Pointer` 原子切换，已建立的 HTTP 流和 WebSocket 连接不会断开。
