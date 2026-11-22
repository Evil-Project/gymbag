# ACME Multi-CA, Certificados IP y Certificados Propios

Para dominios públicos, `tls auto` (predeterminado) obtiene y renueva certificados mediante RFC 8555 ACME HTTP-01.

## Autoridades de Certificación soportadas

Elige una CA con `ssl_provider` (alias: `acme_provider`, `ca`, `cert_issuer`) o `--ssl-provider`:

| Palabra clave | Autoridad de Certificación | Requisito EAB |
| :--- | :--- | :--- |
| `letsencrypt` (`le`) | Let's Encrypt (Producción) | Ninguno |
| `letsencrypt_staging` | Let's Encrypt (Staging) | Ninguno |
| `zerossl` | ZeroSSL | **Automático** cuando `email` está definido, o explícito `eab <kid> <hmac_key>` |
| `google` (`gts`) | Google Trust Services | Requiere `eab <kid> <hmac_key>` |
| `ssl.com` (`sslcom`, `ssl.com-rsa`) | SSL.com (ECC o RSA) | Requiere `eab <kid> <hmac_key>` |
| `litessl` (`trustasia`) | LiteSSL by TrustAsia | Ninguno para DV estándar |
| `buypass` | Buypass Go SSL | Ninguno |
| `local` | PKI local de dos niveles | CA local (`Neuro-sama Headquarters Root CA`) |
| `https://...` | Cualquier CA RFC 8555 ACME v2 | `eab <kid> <hmac_key>` opcional (p. ej. `step-ca`, Pebble) |

## Ejemplos de proveedores

### ZeroSSL con EAB automático

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

### Google Trust Services / SSL.com (EAB explícito)

Se aceptan claves HMAC en base64 con relleno o base64 URL-safe sin relleno. `eab_hmac_key` aparece oculto en las respuestas de la Admin API.

```caddyfile
{
    email ops@example.com
    ssl_provider google
    eab YOUR_EAB_KEY_ID YOUR_EAB_HMAC_KEY
}
```

### URL de directorio ACME personalizada

```caddyfile
{
    email ops@example.com
    ssl_provider https://ca.internal.corp/acme/acme/directory
}
```

Los certificados ACME se cachean en `<cert_dir>` indexados por proveedor (por ejemplo `certs/acme-zerossl-fullchain.pem`, mientras Let's Encrypt usa `certs/acme-fullchain.pem`), así que cambiar `ssl_provider` solicita el certificado a la nueva CA en vez de reutilizar el anterior.

## Certificados IP RFC 8738 (portal de verificación SSH y Minecraft)

Cuando la protección SSH o Minecraft está activa sin un `verify_url` personalizado, Gymbag sirve el portal HTTPS de verificación en la dirección IP detectada del servidor (puerto por defecto `8443`).

En modo `auto` (`tls auto` con un proveedor distinto de `local`), Gymbag solicita un **certificado IP ACME HTTP-01 RFC 8738** para la IP pública del servidor —pidiendo automáticamente el perfil `shortlived` de la CA si el directorio ACME lo anuncia— y lo guarda en `certs/verify-acme-fullchain.pem` (`verify-acme-<provider>-fullchain.pem` para otras CAs). Si el puerto `80` no está disponible o la CA rechaza identificadores IP, Gymbag recurre a su CA local (`certs/verify-local-fullchain.pem`) o a un certificado IP autofirmado (`certs/verify-selfsigned.pem`).

También puedes asignar un par de certificado dedicado al portal de verificación:

```caddyfile
{
    verify_url https://verify.example.com:8443
    verify_server {
        tls /etc/ssl/verify-fullchain.pem /etc/ssl/verify-privkey.pem
    }
}
```

## Claves y certificados propios

- **Clave de cuenta ACME**: `account_key /etc/gymbag/account.pem` (ECDSA P-256/P-384 o RSA)
- **Clave privada CSR**: `ssl_key /etc/gymbag/domain.key`
- **Certificado y clave PEM (global o SNI por sitio)**: `tls /etc/ssl/corp-fullchain.pem /etc/ssl/corp-privkey.pem`
- **Frase de paso de clave privada cifrada**: `key_password super-secret-passphrase` (o `GYMBAG_KEY_PASSWORD`)

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

## Renovación en segundo plano

Un bucle en segundo plano comprueba los certificados cada `renew_interval` segundos (por defecto `43200`, es decir, 12 horas). Todo certificado al que le queden menos de `renew_within_days` días para caducar (por defecto `30` días), o que se haya reemplazado en disco, se renueva o recarga y se sustituye de forma atómica mediante `atomic.Pointer` para que los flujos HTTP abiertos y las conexiones WebSocket sigan conectados.
