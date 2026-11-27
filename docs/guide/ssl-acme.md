# Multi-CA ACME, IP Certs & Custom Certificates

For public domains, `tls auto` (the default) gets and renews certificates over RFC 8555 ACME HTTP-01.

## Supported Certificate Authorities

Pick a CA with `ssl_provider` (aliases: `acme_provider`, `ca`, `cert_issuer`) or `--ssl-provider`:

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

## Provider Examples

### ZeroSSL with Automatic EAB

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

Both padded base64 and unpadded URL-safe base64 HMAC keys are accepted. `eab_hmac_key` is redacted in Admin API output.

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

ACME certificates are cached under `<cert_dir>` keyed by provider (for example `certs/acme-zerossl-fullchain.pem`, while Let's Encrypt uses `certs/acme-fullchain.pem`), so switching `ssl_provider` requests from the new CA instead of reusing the old one.

## RFC 8738 IP Certificates (SSH & Minecraft Verify Portal)

When SSH or Minecraft protection is active without a custom `verify_url`, Gymbag serves the HTTPS verification portal on the server's detected IP address (default port `8443`).

In `auto` mode (`tls auto` with a non-`local` provider), Gymbag requests an **RFC 8738 ACME IP HTTP-01 certificate** for the public server IP—automatically requesting the CA's `shortlived` profile if the ACME directory advertises one—and caches it at `certs/verify-acme-fullchain.pem` (`verify-acme-<provider>-fullchain.pem` for other CAs). If port `80` is unavailable or the CA rejects IP identifiers, Gymbag falls back to its Local CA (`certs/verify-local-fullchain.pem`) or a self-signed IP cert (`certs/verify-selfsigned.pem`).

You can also assign a dedicated certificate pair to the verification portal:

```caddyfile
{
    verify_url https://verify.example.com:8443
    verify_server {
        tls /etc/ssl/verify-fullchain.pem /etc/ssl/verify-privkey.pem
    }
}
```

## Bring Your Own Keys & Certificates

- **ACME account key**: `account_key /etc/gymbag/account.pem` (ECDSA P-256/P-384 or RSA)
- **CSR private key**: `ssl_key /etc/gymbag/domain.key`
- **PEM cert & key (global or per-site SNI)**: `tls /etc/ssl/corp-fullchain.pem /etc/ssl/corp-privkey.pem`
- **`acme.sh` certificate directory**: `acme_sh_path ~/.acme.sh` (or per-site `acme_sh_path /etc/ssl/acme/corp` / `tls acme.sh <path>`) — automatically resolves `<domain>_ecc/` (ECC preferred), `<domain>/` (RSA), wildcard parent directories, `fullchain.cer`, and direct single-domain directories, and hot-reloads on renewal
- **Encrypted private key passphrase**: `key_password super-secret-passphrase` (or `GYMBAG_KEY_PASSWORD`)

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

## Disabling Auto SSL & Direct HTTP Access

- **Disable automatic SSL (`auto_ssl off` / `tls off`)**: Stops Gymbag from provisioning ACME, Local CA, or self-signed certificates. Pair `auto_ssl off` with `acme_sh_path` or `tls <cert> <key>` to serve HTTPS using only externally managed certs, or use `tls off` (`http_only` / `--http-only`) with no certs to disable the HTTPS listener completely and serve plain HTTP directly.
- **Direct HTTP access (`direct_http on` / `http://domain`)**: Serves sites directly on `http_port` (full WAF, PoW verification without requiring `Secure` cookies over HTTP, reverse proxy, and `http://` `Location` rewrites) instead of issuing a `308` redirect to HTTPS. When HTTPS is also enabled, `/.well-known/acme-challenge/*` continues to work on `http_port` and `Strict-Transport-Security` is only sent on HTTPS responses.

```caddyfile
# Serve plain HTTP only (HTTPS and auto SSL off)
http://app.example.com {
    reverse_proxy http://localhost:3000
    verify 3
}
```

```caddyfile
# Serve both direct HTTP and HTTPS using acme.sh certs (no built-in auto SSL)
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

## Background Renewal

A background loop checks certificates every `renew_interval` seconds (default `43200`, i.e. 12 hours). Any certificate within `renew_within_days` of expiry (default `30` days), or replaced on disk, is renewed or reloaded and swapped atomically via `atomic.Pointer` so open HTTP streams and WebSocket connections stay connected.
