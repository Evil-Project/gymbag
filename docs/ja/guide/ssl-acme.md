# マルチ CA ACME・カスタム証明書・自動更新

公開ドメインでは、既定の `tls auto` が RFC 8555 ACME HTTP-01 を通じて自動的に TLS 証明書を取得・更新します。

## 対応する認証局 (CA)

`ssl_provider`（別名：`acme_provider`、`ca`、`cert_issuer`）または `--ssl-provider` で CA を切り替えられます：

| プロバイダー名 | 認証局 | EAB 要件 |
| :--- | :--- | :--- |
| `letsencrypt` (`le`) | Let's Encrypt (本番) | 不要 |
| `letsencrypt_staging` | Let's Encrypt (ステージング) | 不要 |
| `zerossl` | ZeroSSL | `email` 設定時は**自動取得**、または `eab <kid> <hmac_key>` |
| `google` (`gts`) | Google Trust Services | `eab <kid> <hmac_key>` が必須 |
| `ssl.com` (`sslcom`, `ssl.com-rsa`) | SSL.com (ECC または RSA) | `eab <kid> <hmac_key>` が必須 |
| `litessl` (`trustasia`) | LiteSSL by TrustAsia | 標準 DV は不要 |
| `buypass` | Buypass Go SSL | 不要 |
| `local` | ローカル2層 PKI | ローカル CA (`Neuro-sama Headquarters Root CA`) |
| `https://...` | 任意の RFC 8555 ACME v2 CA | 任意の `eab <kid> <hmac_key>`（`step-ca`、Pebble 等） |

---

## 設定例

### ZeroSSL（メールによる自動 EAB）

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

### Google Trust Services / SSL.com（明示的 EAB）

標準 Base64 および URL-safe Base64 の両方に対応しており、Admin API 出力では `eab_hmac_key` が自動的にマスクされます。

```caddyfile
{
    email ops@example.com
    ssl_provider google
    eab YOUR_EAB_KEY_ID YOUR_EAB_HMAC_KEY
}
```

### カスタム ACME Directory URL

```caddyfile
{
    email ops@example.com
    ssl_provider https://ca.internal.corp/acme/acme/directory
}
```

ACME 証明書はプロバイダー名ごとに `<cert_dir>` 配下に保存されるため（例：`certs/acme-zerossl-fullchain.pem`）、`ssl_provider` を切り替えると即座に新しい CA から証明書を取得します。

---

## 独自鍵・証明書の持ち込み

- **カスタム ACME アカウント鍵**：`account_key /etc/gymbag/account.pem`（ECDSA P-256/P-384 または RSA）
- **カスタム CSR 秘密鍵**：`ssl_key /etc/gymbag/domain.key`
- **カスタム PEM 証明書＆秘密鍵（グローバルまたはサイト別 SNI）**：`tls /etc/ssl/corp-fullchain.pem /etc/ssl/corp-privkey.pem`
- **パスフレーズ付き秘密鍵**：`key_password super-secret-passphrase`（または `GYMBAG_KEY_PASSWORD`）

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

## 無停止バックグラウンド更新

Gymbag は `renew_interval` 秒ごと（既定 `43200` 秒 / `12h`）に証明書を確認し、有効期限まで `renew_within_days` 日以内（既定 `30` 日）になるとバックグラウンドで更新または再読み込みを行い、`tls.Certificate` ポインタをアトミックに差し替えます。進行中の HTTP ストリームや WebSocket 接続は一切切断されません。
