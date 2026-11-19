# ACME Multi-CA, Certificados Propios y Renovación

Para dominios públicos, `tls auto` (predeterminado) aprovisiona y renueva certificados TLS automáticamente por RFC 8555 ACME HTTP-01.

## Autoridades Certificadoras Soportadas

| Proveedor | Autoridad Certificadora | Requisito EAB |
| :--- | :--- | :--- |
| `letsencrypt` (`le`) | Let's Encrypt (Producción) | Ninguno |
| `letsencrypt_staging` | Let's Encrypt (Staging) | Ninguno |
| `zerossl` | ZeroSSL | **Automático** con `email`, o `eab <kid> <hmac_key>` |
| `google` (`gts`) | Google Trust Services | Requiere `eab <kid> <hmac_key>` |
| `ssl.com` (`sslcom`, `ssl.com-rsa`) | SSL.com (ECC o RSA) | Requiere `eab <kid> <hmac_key>` |
| `litessl` (`trustasia`) | LiteSSL by TrustAsia | Ninguno para DV estándar |
| `buypass` | Buypass Go SSL | Ninguno |
| `local` | PKI Local de Dos Niveles | CA Local (`Neuro-sama Headquarters Root CA`) |
| `https://...` | Cualquier CA ACME v2 | Opcional `eab <kid> <hmac_key>` (`step-ca`, Pebble) |

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

## Claves y Certificados Propios + Renovación Sin Cortes

- `account_key /etc/gymbag/account.pem`, `ssl_key /etc/gymbag/domain.key`, `tls /etc/ssl/cert.pem /etc/ssl/key.pem`, `key_password <passphrase>`.
- Gymbag comprueba cada `renew_interval` (`12h` por defecto) y renueva cuando faltan menos de `renew_within_days` (`30` días), intercambiando el certificado en memoria atómicamente sin desconectar flujos HTTP ni WebSockets.
