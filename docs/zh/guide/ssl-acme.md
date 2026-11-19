# 多 CA ACME、自定义证书与自动续期

对于公网域名，默认模式 `tls auto` 会通过 RFC 8555 ACME HTTP-01 自动签发并续期 TLS 证书。

## 支持的证书颁发机构 (CA)

通过 `ssl_provider`（别名：`acme_provider`、`ca`、`cert_issuer`）或 `--ssl-provider` 切换 CA：

| 提供商关键字 | 证书颁发机构 | EAB 要求 |
| :--- | :--- | :--- |
| `letsencrypt` (`le`) | Let's Encrypt（生产环境） | 无需 |
| `letsencrypt_staging` | Let's Encrypt（测试环境） | 无需 |
| `zerossl` | ZeroSSL | 设置 `email` 后**自动获取**，或显式配置 `eab <kid> <hmac_key>` |
| `google` (`gts`) | Google Trust Services | 需配置 `eab <kid> <hmac_key>` |
| `ssl.com` (`sslcom`, `ssl.com-rsa`) | SSL.com（ECC 或 RSA） | 需配置 `eab <kid> <hmac_key>` |
| `litessl` (`trustasia`) | LiteSSL by TrustAsia | 标准 DV 无需 |
| `buypass` | Buypass Go SSL | 无需 |
| `local` | 本地两级 PKI | 本地 CA (`Neuro-sama Headquarters Root CA`) |
| `https://...` | 任意 RFC 8555 ACME v2 CA | 可选 `eab <kid> <hmac_key>`（如 `step-ca`、Pebble） |

---

## 配置示例

### ZeroSSL（邮箱自动获取 EAB）

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

同时支持标准带填充 Base64 和 URL 安全无填充 Base64 HMAC 密钥。Admin API 响应会自动脱敏隐藏 `eab_hmac_key`。

```caddyfile
{
    email ops@example.com
    ssl_provider google
    eab YOUR_EAB_KEY_ID YOUR_EAB_HMAC_KEY
}
```

### 自定义 ACME Directory URL

```caddyfile
{
    email ops@example.com
    ssl_provider https://ca.internal.corp/acme/acme/directory
}
```

ACME 证书按提供商名称隔离缓存在 `<cert_dir>` 下（如 `certs/acme-zerossl-fullchain.pem`），切换 `ssl_provider` 后会立即向新 CA 申请证书。

---

## 自带私钥与证书

- **自定义 ACME 账号密钥**：`account_key /etc/gymbag/account.pem`（ECDSA P-256/P-384 或 RSA）
- **自定义 CSR 私钥**：`ssl_key /etc/gymbag/domain.key`
- **自定义 PEM 证书与私钥（全局或按站点 SNI）**：`tls /etc/ssl/corp-fullchain.pem /etc/ssl/corp-privkey.pem`
- **加密私钥密码**：`key_password super-secret-passphrase`（或 `GYMBAG_KEY_PASSWORD`）

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

## 零停机后台续期

Gymbag 每隔 `renew_interval`（默认 `43200` 秒 / `12h`）检查一次证书。当证书距离过期不足 `renew_within_days` 天（默认 `30` 天）时，会在后台自动续期或重新加载，并原子替换 `tls.Certificate` 指针，不中断任何 HTTP 流或 WebSocket 连接。
