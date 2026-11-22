# 멀티 CA ACME, IP 인증서 & 사용자 지정 인증서

공용 도메인은 기본 설정인 `tls auto`를 통해 RFC 8555 ACME HTTP-01 방식으로 인증서를 발급받고 자동 갱신합니다.

## 지원하는 인증 기관 (CA)

`ssl_provider`(별칭: `acme_provider`, `ca`, `cert_issuer`) 또는 `--ssl-provider`로 CA를 선택합니다.

| 공급자 키워드 | 인증 기관 (CA) | EAB 요구사항 |
| :--- | :--- | :--- |
| `letsencrypt` (`le`) | Let's Encrypt (운영) | 없음 |
| `letsencrypt_staging` | Let's Encrypt (스테이징) | 없음 |
| `zerossl` | ZeroSSL | `email` 설정 시 **자동**, 또는 명시적 `eab <kid> <hmac_key>` |
| `google` (`gts`) | Google Trust Services | `eab <kid> <hmac_key>` 필요 |
| `ssl.com` (`sslcom`, `ssl.com-rsa`) | SSL.com (ECC 또는 RSA) | `eab <kid> <hmac_key>` 필요 |
| `litessl` (`trustasia`) | LiteSSL by TrustAsia | 표준 DV는 없음 |
| `buypass` | Buypass Go SSL | 없음 |
| `local` | 로컬 2계층 PKI | 로컬 CA (`Neuro-sama Headquarters Root CA`) |
| `https://...` | 임의의 RFC 8555 ACME v2 CA | 선택적 `eab <kid> <hmac_key>` (`step-ca`, Pebble 등) |

## 공급자별 설정 예시

### ZeroSSL (자동 EAB)

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

패딩된 base64와 패딩 없는 URL-safe base64 HMAC 키를 모두 받습니다. Admin API 응답에서는 `eab_hmac_key`가 마스킹됩니다.

```caddyfile
{
    email ops@example.com
    ssl_provider google
    eab YOUR_EAB_KEY_ID YOUR_EAB_HMAC_KEY
}
```

### 사용자 지정 ACME 디렉터리 URL

```caddyfile
{
    email ops@example.com
    ssl_provider https://ca.internal.corp/acme/acme/directory
}
```

ACME 인증서는 `<cert_dir>` 아래에 공급자별로 구분해 저장되므로(예: `certs/acme-zerossl-fullchain.pem`, Let's Encrypt는 `certs/acme-fullchain.pem`), `ssl_provider`를 바꾸면 기존 CA 인증서를 재사용하지 않고 새 CA에 즉시 요청합니다.

## RFC 8738 IP 인증서 (SSH & Minecraft 인증 포털)

사용자 지정 `verify_url` 없이 SSH 또는 Minecraft 보호를 켜면, Gymbag은 감지된 서버 IP 주소로 HTTPS 인증 포털(기본 포트 `8443`)을 엽니다.

`auto` 모드(`local`이 아닌 공급자 + `tls auto`)에서 Gymbag은 공인 서버 IP에 대해 **RFC 8738 ACME IP HTTP-01 인증서**를 요청하며(ACME 디렉터리가 `shortlived` 프로필을 제공하면 자동 선택), `certs/verify-acme-fullchain.pem`(다른 CA는 `verify-acme-<provider>-fullchain.pem`)에 캐싱합니다. 포트 `80`에 닿지 못하거나 CA가 IP 식별자를 거절하면 로컬 CA(`certs/verify-local-fullchain.pem`) 또는 자체 서명 IP 인증서(`certs/verify-selfsigned.pem`)로 폴백합니다.

인증 포털에 별도 인증서 쌍을 직접 지정할 수도 있습니다.

```caddyfile
{
    verify_url https://verify.example.com:8443
    verify_server {
        tls /etc/ssl/verify-fullchain.pem /etc/ssl/verify-privkey.pem
    }
}
```

## 기존 키 및 인증서 사용

- **ACME 계정 키**: `account_key /etc/gymbag/account.pem` (ECDSA P-256/P-384 또는 RSA)
- **CSR 개인 키**: `ssl_key /etc/gymbag/domain.key`
- **PEM 인증서 및 키 (전역 또는 사이트별 SNI)**: `tls /etc/ssl/corp-fullchain.pem /etc/ssl/corp-privkey.pem`
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

## 백그라운드 자동 갱신

백그라운드 루프가 `renew_interval`초(기본 `43200`, 즉 12시간)마다 인증서를 점검합니다. 만료가 `renew_within_days`(기본 `30`일) 이내로 남았거나 디스크 파일이 교체되면 갱신 또는 재로딩한 뒤 `atomic.Pointer`로 원자적으로 교체해 열린 HTTP 스트림과 WebSocket 연결을 끊지 않습니다.
