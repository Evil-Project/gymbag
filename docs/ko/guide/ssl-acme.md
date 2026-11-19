# 멀티 CA ACME, 사용자 지정 인증서 & 자동 갱신

공인 도메인에서는 기본값인 `tls auto`가 RFC 8555 ACME HTTP-01을 통해 TLS 인증서를 자동으로 발급하고 갱신합니다.

## 지원되는 인증 기관 (CA)

`ssl_provider` (별칭: `acme_provider`, `ca`, `cert_issuer`) 또는 `--ssl-provider`로 CA를 전환할 수 있습니다:

| 공급자 키워드 | 인증 기관 | EAB 요구 사항 |
| :--- | :--- | :--- |
| `letsencrypt` (`le`) | Let's Encrypt (운영) | 불필요 |
| `letsencrypt_staging` | Let's Encrypt (스테이징) | 불필요 |
| `zerossl` | ZeroSSL | `email` 설정 시 **자동 발급**, 또는 `eab <kid> <hmac_key>` |
| `google` (`gts`) | Google Trust Services | `eab <kid> <hmac_key>` 필수 |
| `ssl.com` (`sslcom`, `ssl.com-rsa`) | SSL.com (ECC 또는 RSA) | `eab <kid> <hmac_key>` 필수 |
| `litessl` (`trustasia`) | LiteSSL by TrustAsia | 표준 DV 불필요 |
| `buypass` | Buypass Go SSL | 불필요 |
| `local` | 로컬 2단계 PKI | 로컬 CA (`Neuro-sama Headquarters Root CA`) |
| `https://...` | 임의의 RFC 8555 ACME v2 CA | 선택적 `eab <kid> <hmac_key>` (`step-ca`, Pebble 등) |

---

## 구성 예시

### ZeroSSL (이메일 기반 자동 EAB)

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

### Google Trust Services / SSL.com (명시적 EAB)

표준 패딩 Base64와 URL-safe 비패딩 Base64 HMAC 키 모두 지원하며 Admin API 응답에서는 `eab_hmac_key`가 자동 마스킹됩니다.

```caddyfile
{
    email ops@example.com
    ssl_provider google
    eab YOUR_EAB_KEY_ID YOUR_EAB_HMAC_KEY
}
```

### 사용자 지정 ACME Directory URL

```caddyfile
{
    email ops@example.com
    ssl_provider https://ca.internal.corp/acme/acme/directory
}
```

ACME 인증서는 공급자 이름별로 `<cert_dir>`에 분리 캐시되므로(예: `certs/acme-zerossl-fullchain.pem`), `ssl_provider`를 변경하면 즉시 새 CA에서 인증서를 발급받습니다.

---

## 자체 키 및 인증서 사용

- **사용자 지정 ACME 계정 키**: `account_key /etc/gymbag/account.pem` (ECDSA P-256/P-384 또는 RSA)
- **사용자 지정 CSR 개인 키**: `ssl_key /etc/gymbag/domain.key`
- **사용자 지정 PEM 인증서 및 키 (전역 또는 사이트별 SNI)**: `tls /etc/ssl/corp-fullchain.pem /etc/ssl/corp-privkey.pem`
- **암호화된 개인 키 암호**: `key_password super-secret-passphrase` (또는 `GYMBAG_KEY_PASSWORD`)

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

## 무중단 백그라운드 갱신

Gymbag은 `renew_interval`초(기본 `43200`초 / `12h`)마다 인증서를 점검하며, 만료 `renew_within_days`일(기본 `30`일) 이내에 도달하면 백그라운드에서 갱신 또는 다시 로드해 활성 `tls.Certificate` 포인터를 원자적으로 교체합니다. 진행 중인 HTTP 스트림이나 WebSocket 연결은 끊어지지 않습니다.
