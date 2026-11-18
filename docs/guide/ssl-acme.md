# Multi-CA ACME, Custom Certificates & Renewal

For public domains, `tls auto` (the default) automatically provisions and renews TLS certificates over RFC 8555 ACME HTTP-01.

## Supported Certificate Authorities

Switch Certificate Authorities with `ssl_provider` (aliases: `acme_provider`, `ca`, `cert_issuer`) or `--ssl-provider`:

| Provider Keyword | Certificate Authority | EAB Requirement |
| :--- | :--- | :--- |
| `letsencrypt` (`le`) | Let's Encrypt (Production) | None |
| `letsencrypt_staging` | Let's Encrypt (Staging) | None |
| `zerossl` | ZeroSSL | **Automatic** when `email` is set, or explicit `eab <kid> <hmac_key>` |
| `google` (`gts`) | Google Trust Services | Requires `eab <kid> <hmac_key>` |
| `ssl.com` (`sslcom`, `ssl.com-rsa`) | SSL.com (ECC or RSA) | Requires `eab <kid> <hmac_key>` |
| `litessl` (`trustasia`) | LiteSSL by TrustAsia | None for standard DV |
| `buypass` | Buypass Go SSL | None |
| `local` | Local Two-Tier PKI | Local CA (`Neuro-sama Headquarters Root CA`) |
| `https://...` | Any RFC 8555 ACME v2 CA | Optional `eab <kid> <hmac_key>` (e.g. `step-ca`, Pebble) |

---

## Provider Configuration Examples

### ZeroSSL with Automatic Email EAB

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

### Google Trust Services / SSL.com (Explicit EAB)

Both standard padded base64 and URL-safe unpadded base64 HMAC keys are accepted. `eab_hmac_key` is automatically redacted in Admin API responses.

```caddyfile
{
    email ops@example.com
    ssl_provider google
    eab YOUR_EAB_KEY_ID YOUR_EAB_HMAC_KEY
}
```

### Custom ACME Directory URL

```caddyfile
{
    email ops@example.com
    ssl_provider https://ca.internal.corp/acme/acme/directory
}
```

ACME certificates are cached under `<cert_dir>` scoped by provider name (for example `certs/acme-zerossl-fullchain.pem`), so switching `ssl_provider` immediately provisions a cert from the new CA.

---

## Bring Your Own Keys & Certificates

- **Custom ACME account key**: `account_key /etc/gymbag/account.pem` (ECDSA P-256/P-384 or RSA)
- **Custom CSR private key**: `ssl_key /etc/gymbag/domain.key`
- **Custom PEM cert & key (global or per-site SNI)**: `tls /etc/ssl/corp-fullchain.pem /etc/ssl/corp-privkey.pem`
- **Encrypted private key passphrase**: `key_password super-secret-passphrase` (or `GYMBAG_KEY_PASSWORD`)

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

## Zero-Downtime Background Renewal

Gymbag checks certificates every `renew_interval` seconds (default `43200` = 12 hours). When a certificate is within `renew_within_days` of expiration (default `30` days), it renews or reloads in the background and swaps the active `tls.Certificate` pointer atomically without dropping active HTTP streams or WebSocket connections.
