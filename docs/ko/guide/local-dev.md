# 로컬 개발 & 2계층 PKI

사용자 지정 호스트명으로 로컬 HTTPS를 띄우려면 보통 `mkcert`, 수동 `/etc/hosts` 편집, 브라우저 인증서 경고를 매번 처리해야 합니다. Gymbag은 사이트가 다음 호스트명을 쓸 때 호스트 등록과 TLS 인증서를 자동으로 처리합니다.

- `localhost` 또는 `*.localhost`
- `*.internal` (`app.internal`, `api.internal` 등)
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

## `/etc/hosts` 자동 관리

`auto_hosts on`(기본값)이 켜져 있으면 시작 시 `/etc/hosts`(Windows는 `C:\Windows\System32\drivers\etc\hosts`)를 확인합니다. 아직 등록되지 않은 로컬 도메인에는 인스턴스 ID 태그가 달린 IPv4 및 IPv6 루프백 줄을 추가합니다.

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

- 기존 `localhost` 줄은 건드리지 않습니다.
- `Ctrl+C` 또는 `SIGTERM`으로 종료하면 실행 중에 사용자가 수정한 내용이 남도록 `/etc/hosts`를 다시 읽은 뒤 자신이 쓴 `# gymbag-managed:<instanceID>` 줄만 지웁니다.
- root 권한이 없어 `/etc/hosts`에 쓸 수 없으면 경고를 로그에 남기고 계속 실행됩니다. `.internal`을 `dnsmasq`나 수동 편집으로 관리한다면 `auto_hosts off`(또는 `GYMBAG_AUTO_HOSTS=off`)로 끄세요.

## 2계층 로컬 인증 기관

`<cert_dir>/ca/`(기본 `certs/ca/`) 아래에 2계층 ECDSA P-256 CA를 유지합니다.

1. **루트 CA**: `"Neuro-sama Headquarters Root CA"` (`certs/ca/neuro-sama-root-ca.pem`, 유효기간 10년, `MaxPathLen: 1`)
2. **중간 CA**: `"Gymbag & Swarm SSL Local CA"` (`certs/ca/gymbag-swarm-local-ca.pem`, 유효기간 5년, `MaxPathLen: 0`)
3. **리프 인증서**: `certs/local-fullchain.pem` + `certs/local-privkey.pem` (유효기간 90일, 만료 30일 이내 갱신, DNS SAN과 `127.0.0.1` 및 `::1` 포함)

### 루트 CA 한 번만 신뢰 등록하기

OS 신뢰 저장소에 `certs/ca/neuro-sama-root-ca.pem`을 한 번 설치해 두면 브라우저와 `curl`이 모든 로컬 사이트를 검증합니다.

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
macOS와 Windows에서는 `about:config`의 `security.enterprise_roots.enabled`가 `true`이면 Firefox가 OS 루트 CA를 읽습니다. Linux에서 Firefox가 `p11-kit` 대신 자체 NSS DB를 쓴다면 **설정 -> 개인 정보 및 보안 -> 인증서 -> 인증서 보기 -> 인증 기관**에서 `certs/ca/neuro-sama-root-ca.pem`을 한 번 가져오세요.
:::
