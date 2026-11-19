# 로컬 개발 & 2단계 로컬 PKI

사용자 지정 호스트 이름으로 로컬 HTTPS를 테스트할 때는 보통 `mkcert`, 수동 `/etc/hosts` 편집, 브라우저 보안 경고를 다뤄야 합니다. Gymbag은 사이트가 다음 로컬 도메인을 사용할 경우 로컬 이름 해석과 X.509 인증서 발급을 모두 자동으로 처리합니다:

- `localhost` 또는 `*.localhost`
- `*.internal` (예: `app.internal`, `api.internal`)
- `*.local` 또는 `*.test`

```caddyfile
app.internal {
    reverse_proxy http://localhost:3000
    verify 3
}

api.internal {
    reverse_proxy http://localhost:8080
    protect off
}
```

## 자동 `/etc/hosts` 관리

`auto_hosts`가 활성화되어 있으면(기본값) Gymbag은 시작 시 `/etc/hosts`(Windows는 `C:\Windows\System32\drivers\etc\hosts`)를 검사하여 등록되지 않은 로컬 도메인에 대해 인스턴스 ID 태그가 붙은 IPv4 및 IPv6 루프백 항목을 추가합니다:

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

- 기존 `localhost` 항목은 건드리지 않습니다.
- 종료 시(`Ctrl+C` / `SIGTERM`) `/etc/hosts`를 다시 읽어 실행 중 사용자가 수정한 내용을 보존하고, 자신이 추가한 `# gymbag-managed:<instanceID>` 줄**만** 깔끔하게 제거합니다.
- 쓰기 권한이 없는 경우 경고를 기록하고 **정상적으로 계속 실행**됩니다. `dnsmasq` 등을 사용하는 경우 `auto_hosts off`(또는 `GYMBAG_AUTO_HOSTS=off`)를 설정하세요.

## 2단계 로컬 인증 기관 (PKI)

`<cert_dir>/ca/`(기본 `certs/ca/`) 내부에서 2단계 ECDSA P-256 PKI를 유지 관리합니다:

1. **Root CA**: `"Neuro-sama Headquarters Root CA"` (`certs/ca/neuro-sama-root-ca.pem`, 10년 유효, `MaxPathLen: 1`)
2. **Intermediate CA**: `"Gymbag & Swarm SSL Local CA"` (`certs/ca/gymbag-swarm-local-ca.pem`, 5년 유효, `MaxPathLen: 0`)
3. **Leaf Certificate**: `certs/local-fullchain.pem` + `certs/local-privkey.pem` (90일 유효, 만료 30일 전 자동 갱신, DNS SAN과 `127.0.0.1` / `::1` IP SAN 포함)

### Root CA 한 번만 신뢰 등록하기

`certs/ca/neuro-sama-root-ca.pem`을 OS 신뢰 저장소에 한 번만 등록하면 브라우저와 `curl` 모두에서 깨끗한 초록색 자물쇠가 표시됩니다:

::: code-group
```bash [macOS]
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain \
  certs/ca/neuro-sama-root-ca.pem
```

```bash [Debian / Ubuntu]
sudo cp certs/ca/neuro-sama-root-ca.pem /usr/local/share/ca-certificates/neuro-sama-root-ca.crt
sudo update-ca-certificates
```

```bash [Fedora / RHEL / Arch]
sudo trust anchor --store certs/ca/neuro-sama-root-ca.pem
```
:::

::: info Firefox 참고사항
macOS 및 Windows Firefox에서는 `about:config`에서 `security.enterprise_roots.enabled`를 `true`로 설정하면 OS 루트 인증서를 자동으로 신뢰합니다. Linux에서 Firefox가 자체 NSS 저장소를 쓰는 경우 **설정 → 개인정보 및 보안 → 인증서 → 인증서 보기 → 인증 기관**에서 `certs/ca/neuro-sama-root-ca.pem`을 가져오세요.
:::
