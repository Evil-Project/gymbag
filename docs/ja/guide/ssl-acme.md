# マルチ CA ACME・IP 証明書・独自証明書

公開ドメインでは、デフォルトの `tls auto` が RFC 8555 ACME HTTP-01 を使って証明書の取得と更新を自動で行います。

## 対応する認証局 (CA)

`ssl_provider`（別名: `acme_provider`、`ca`、`cert_issuer`）または `--ssl-provider` で CA を切り替えます。

| プロバイダキーワード | 認証局 (CA) | EAB 要件 |
| :--- | :--- | :--- |
| `letsencrypt` (`le`) | Let's Encrypt (本番) | なし |
| `letsencrypt_staging` | Let's Encrypt (ステージング) | なし |
| `zerossl` | ZeroSSL | `email` 設定時は**自動取得**、または明示的な `eab <kid> <hmac_key>` |
| `google` (`gts`) | Google Trust Services | `eab <kid> <hmac_key>` が必須 |
| `ssl.com` (`sslcom`, `ssl.com-rsa`) | SSL.com (ECC または RSA) | `eab <kid> <hmac_key>` が必須 |
| `litessl` (`trustasia`) | LiteSSL by TrustAsia | 標準 DV では不要 |
| `buypass` | Buypass Go SSL | なし |
| `local` | ローカル 2 階層 PKI | ローカル CA (`Neuro-sama Headquarters Root CA`) |
| `https://...` | 任意の RFC 8555 ACME v2 CA | 任意の `eab <kid> <hmac_key>`（`step-ca`、Pebble 等） |

## プロバイダ別設定例

### ZeroSSL（自動 EAB）

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

パディング付き Base64 とパディングなし URL-safe Base64 の HMAC キーの両方を受け付けます。`eab_hmac_key` は Admin API の出力でマスクされます。

```caddyfile
{
    email ops@example.com
    ssl_provider google
    eab YOUR_EAB_KEY_ID YOUR_EAB_HMAC_KEY
}
```

### カスタム ACME ディレクトリ URL

```caddyfile
{
    email ops@example.com
    ssl_provider https://ca.internal.corp/acme/acme/directory
}
```

ACME 証明書はプロバイダをキーにして `<cert_dir>` 配下にキャッシュされます（たとえば ZeroSSL は `certs/acme-zerossl-fullchain.pem`、Let's Encrypt は `certs/acme-fullchain.pem`）。そのため `ssl_provider` を切り替えても古い証明書を使い回さず、新しい CA へリクエストを発行します。

## RFC 8738 IP 証明書（SSH & Minecraft 検証ポータル）

`verify_url` を指定せずに SSH または Minecraft の保護を有効にした場合、Gymbag は自動検出したサーバーの IP アドレス上（デフォルトポート `8443`）で HTTPS 検証ポータルを提供します。

`auto` モード（`tls auto` かつ `local` 以外のプロバイダ）では、パブリックサーバー IP 向けに **RFC 8738 ACME IP HTTP-01 証明書**を要求し（ACME ディレクトリが `shortlived` プロファイルを案内していれば自動選択）、`certs/verify-acme-fullchain.pem`（他 CA では `verify-acme-<provider>-fullchain.pem`）にキャッシュします。ポート `80` に到達できない場合や CA が IP 識別子を発行しない場合は、ローカル CA（`certs/verify-local-fullchain.pem`）または自己署名 IP 証明書（`certs/verify-selfsigned.pem`）へフォールバックします。

検証ポータルに専用の証明書ペアを割り当てることもできます。

```caddyfile
{
    verify_url https://verify.example.com:8443
    verify_server {
        tls /etc/ssl/verify-fullchain.pem /etc/ssl/verify-privkey.pem
    }
}
```

## 独自鍵と証明書の持ち込み

- **ACME アカウント鍵**: `account_key /etc/gymbag/account.pem`（ECDSA P-256/P-384 または RSA）
- **CSR 秘密鍵**: `ssl_key /etc/gymbag/domain.key`
- **PEM 証明書＆秘密鍵（グローバルまたはサイト別 SNI）**: `tls /etc/ssl/corp-fullchain.pem /etc/ssl/corp-privkey.pem`
- **暗号化秘密鍵のパスフレーズ**: `key_password super-secret-passphrase`（または `GYMBAG_KEY_PASSWORD`）

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

## バックグラウンド自動更新

バックグラウンドループが `renew_interval` 秒（デフォルト `43200` = 12 時間）ごとに証明書を確認します。有効期限まで `renew_within_days` 日（デフォルト `30` 日）以内の証明書、またはディスク上で置き換えられた証明書は自動で更新・再読み込みされ、`atomic.Pointer` 経由でアトミックに差し替えられるため、開いている HTTP ストリームや WebSocket 接続が途切れません。
