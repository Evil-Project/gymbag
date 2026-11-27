# 多 CA ACME、IP 憑證與自訂憑證

針對公開網域，`tls auto`（預設值）會透過 RFC 8555 ACME HTTP-01 自動申請並續期憑證。

## 支援的憑證機構（CA）

使用 `ssl_provider`（別名：`acme_provider`、`ca`、`cert_issuer`）或 `--ssl-provider` 指定 CA：

| 提供者關鍵字 | 憑證機構 | EAB 需求 |
| :--- | :--- | :--- |
| `letsencrypt`（`le`） | Let's Encrypt（正式環境） | 無 |
| `letsencrypt_staging` | Let's Encrypt（測試環境） | 無 |
| `zerossl` | ZeroSSL | 設定 `email` 時**自動申請**，或手動指定 `eab <kid> <hmac_key>` |
| `google`（`gts`） | Google Trust Services | 需提供 `eab <kid> <hmac_key>` |
| `ssl.com`（`sslcom`、`ssl.com-rsa`） | SSL.com（ECC 或 RSA） | 需提供 `eab <kid> <hmac_key>` |
| `litessl`（`trustasia`） | LiteSSL by TrustAsia | 標準 DV 憑證無需 EAB |
| `buypass` | Buypass Go SSL | 無 |
| `local` | 本機雙層 PKI | 本機 CA（`Neuro-sama Headquarters Root CA`） |
| `https://...` | 任何 RFC 8555 ACME v2 CA | 選填 `eab <kid> <hmac_key>`（例如 `step-ca`、Pebble） |

## 設定範例

### ZeroSSL（自動取得 EAB）

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

### Google Trust Services / SSL.com（手動指定 EAB）

支援標準補齊（padded）的 base64 與無補齊的 URL-safe base64 HMAC 金鑰。Admin API 輸出時會自動遮罩 `eab_hmac_key`。

```caddyfile
{
    email ops@example.com
    ssl_provider google
    eab YOUR_EAB_KEY_ID YOUR_EAB_HMAC_KEY
}
```

### 自訂 ACME Directory 網址

```caddyfile
{
    email ops@example.com
    ssl_provider https://ca.internal.corp/acme/acme/directory
}
```

ACME 憑證會依提供者名稱分開快取於 `<cert_dir>`（例如 `certs/acme-zerossl-fullchain.pem`，而 Let's Encrypt 為 `certs/acme-fullchain.pem`），因此切換 `ssl_provider` 時會直接向新的 CA 重新申請，不會誤用舊憑證。

## RFC 8738 IP 憑證（SSH 與 Minecraft 驗證入口）

當啟用 SSH 或 Minecraft 防護且未設定自訂 `verify_url` 時，Gymbag 會在伺服器偵測到的 IP 位址上啟動 HTTPS 驗證入口（預設連接埠 `8443`）。

在 `auto` 模式下（`tls auto` 搭配非 `local` 提供者），Gymbag 會為伺服器的公開 IP 申請 **RFC 8738 ACME IP HTTP-01 憑證**（若 ACME 目錄支援 `shortlived` 設定檔會自動選用），並快取至 `certs/verify-acme-fullchain.pem`（其他 CA 則為 `verify-acme-<provider>-fullchain.pem`）。若埠號 `80` 無法連線或該 CA 拒絕 IP 識別符，Gymbag 會退回使用本機 CA（`certs/verify-local-fullchain.pem`）或自簽 IP 憑證（`certs/verify-selfsigned.pem`）。

你也可以為驗證入口指定專屬的憑證與私鑰：

```caddyfile
{
    verify_url https://verify.example.com:8443
    verify_server {
        tls /etc/ssl/verify-fullchain.pem /etc/ssl/verify-privkey.pem
    }
}
```

## 自備金鑰與憑證

- **ACME 帳號金鑰**：`account_key /etc/gymbag/account.pem`（支援 ECDSA P-256/P-384 或 RSA）
- **CSR 私鑰**：`ssl_key /etc/gymbag/domain.key`
- **PEM 憑證與私鑰（全域或單一站台 SNI）**：`tls /etc/ssl/corp-fullchain.pem /etc/ssl/corp-privkey.pem`
- **`acme.sh` 憑證目錄**：`acme_sh_path ~/.acme.sh`（或單一站台 `acme_sh_path /etc/ssl/acme/corp` / `tls acme.sh <path>`）——會自動解析 `<domain>_ecc/`（優先使用 ECC）、`<domain>/`（RSA）、萬用字元上層目錄、`fullchain.cer` 與直接的單網域目錄，並在續期後自動熱重載
- **加密私鑰密碼**：`key_password super-secret-passphrase`（或環境變數 `GYMBAG_KEY_PASSWORD`）

```caddyfile
{
    auto_ssl off
    acme_sh_path /root/.acme.sh
    key_password super-secret-passphrase
}

corp.example.com {
    reverse_proxy http://localhost:8080
    tls /etc/ssl/corp-fullchain.pem /etc/ssl/corp-privkey.pem
    verify 1
}
```

## 停用自動 SSL 與直接 HTTP 存取

- **停用自動 SSL（`auto_ssl off` / `tls off`）**：停止 Gymbag 自動申請 ACME、本機 CA 或自簽憑證。將 `auto_ssl off` 搭配 `acme_sh_path` 或 `tls <cert> <key>` 使用，即可只用外部管理的憑證提供 HTTPS；若使用 `tls off`（`http_only` / `--http-only`）且不提供任何憑證，則會完全關閉 HTTPS 監聽器並直接提供純 HTTP 服務。
- **直接 HTTP 存取（`direct_http on` / `http://domain`）**：讓站台直接在 `http_port` 上提供完整服務（包含完整 WAF、不需 HTTPS `Secure` Cookie 即可使用的工作量證明驗證、反向代理與 `http://` 的 `Location` 改寫），而不發送 `308` 重新導向至 HTTPS。當同時啟用 HTTPS 時，`/.well-known/acme-challenge/*` 仍會在 `http_port` 上正常運作，且 `Strict-Transport-Security` 只會在 HTTPS 回應中送出。

```caddyfile
# 僅提供純 HTTP 服務（關閉 HTTPS 與自動 SSL）
http://app.example.com {
    reverse_proxy http://localhost:3000
    verify 3
}
```

```caddyfile
# 使用 acme.sh 憑證同時提供直接 HTTP 與 HTTPS 服務（停用內建自動 SSL）
{
    auto_ssl off
    acme_sh_path /root/.acme.sh
    direct_http on
}

app.example.com {
    reverse_proxy http://localhost:3000
    verify 3
}
```

## 背景自動續期

背景排程會每隔 `renew_interval` 秒（預設 `43200`，即 12 小時）檢查一次憑證。任何距離到期日不足 `renew_within_days`（預設 `30` 天）或在磁碟上被替換的憑證，都會自動續期或重新載入，並透過 `atomic.Pointer` 進行無鎖原子切換，讓進行中的 HTTP 串流與 WebSocket 連線保持不斷線。
