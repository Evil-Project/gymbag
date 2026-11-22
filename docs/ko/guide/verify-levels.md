# 검증 레벨 (`0`–`3`) & 작업 증명(PoW)

한 Gymbag 프로세스로 여러 웹사이트(라운드 로빈 업스트림 부하 분산 및 RFC 6455 WebSocket 업그레이드 지원)와 SSH·Minecraft 보호를 함께 운영할 수 있습니다. 각 사이트와 서비스는 `verify <0-3>`으로 검증 정책을 지정하거나 `protect off`로 PoW 챌린지를 끌 수 있습니다.

<VerifyLevelMatrix />

## 챌린지 상태 화면

Gymbag은 PoW 챌린지 HTML, SHA-256 브라우저 솔버, 세 가지 마스코트 이미지를 바이너리 안에 직접 내장합니다.

<div class="gb-mascot-strip">
  <div class="gb-mascot-tile">
    <img src="/mascot-pensive.png" alt="PoW 계산 중인 마스코트" />
    <strong>PoW 계산 중</strong>
    <span class="gb-badge challenge">챌린지 발급됨</span>
  </div>
  <div class="gb-mascot-tile">
    <img src="/mascot-happy.png" alt="인증 통과 마스코트" />
    <strong>인증 완료</strong>
    <span class="gb-badge pass">waf_clearance 발급</span>
  </div>
  <div class="gb-mascot-tile">
    <img src="/mascot-reject.png" alt="요청 차단 마스코트" />
    <strong>규칙 / 위협 차단</strong>
    <span class="gb-badge block">403 Forbidden</span>
  </div>
</div>

## 다중 사이트 및 다중 서비스 예시

```caddyfile
app.example.com, www.example.com {
    reverse_proxy http://localhost:3000 http://localhost:3001
    verify 3
    difficulty 4
    verify_timeout 12h
}

docs.example.com {
    reverse_proxy http://localhost:4000
    verify 2
    verify_timeout 7d
}

api.example.com {
    reverse_proxy http://localhost:8080
    verify 1
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
    verify_timeout 30m
}

static.example.com {
    reverse_proxy http://localhost:9000
    protect off
}

ssh {
    upstream 127.0.0.1:41822
    verify 3
}
```

레벨 `1`과 `3`에서는 위협 피드, 실시간 Shodan/StopForumSpam 조회, 누락된 브라우저 헤더, 자동화 클라이언트, 익스플로잇 프로브, 스크립트 SSH/Minecraft 클라이언트 외에 **VPN, 프록시, Tor 출구 IP**(`vpn_detection on`)도 의심 트래픽에 포함됩니다.

## 인증 유지 시간 (`verify_timeout` / `clearance_ttl`)

HTTP/HTTPS 사이트에서 SHA-256 PoW 챌린지를 통과하면 호스트와 클라이언트 IP에 바인딩된 `HttpOnly`, `SameSite=Lax` HMAC 서명 쿠키(기본 이름 `waf_clearance`)가 발급됩니다. SSH와 Minecraft에서는 브라우저 인증 링크를 통과하면 해당 서비스에 대한 IP 인증 기록이 저장됩니다.

기본 유효 시간은 **24시간**(`86400`초)입니다. 전역 또는 사이트·서비스별로 정수 초나 기간 문자열(`30m`, `12h`, `1h30m`, `7d`, `2w`)로 재정의할 수 있습니다.

::: code-group
```caddyfile [Gymbagfile]
{
    verify_timeout 12h
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
    verify_timeout 30m
}

docs.example.com {
    reverse_proxy http://localhost:4000
    verify 2
    clearance_ttl 7d
}
```

```bash [CLI 플래그]
gymbag init -d app.example.com -u :3000 --verify-timeout 6h
gymbag run --config Gymbagfile --verify-timeout 30m
gymbag reverse-proxy --from app.internal --to :3000 --verify 0 --verify-timeout 15m
```

```bash [환경 변수]
GYMBAG_VERIFY_TIMEOUT=12h gymbag
GYMBAG_CLEARANCE_TTL=3600 gymbag
```

```bash [실시간 Admin API]
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/staging.example.com \
  -H "Content-Type: application/json" \
  -d '{"clearance_ttl": 1800}'
```
:::

- `Gymbagfile` 지시어 별칭: `verify_timeout`, `verified_timeout`, `clearance_timeout`, `clearance_ttl`.
- 환경 변수 별칭: `GYMBAG_CLEARANCE_TTL`, `GYMBAG_VERIFY_TIMEOUT`, `GYMBAG_VERIFIED_TIMEOUT`.
- `POST /.waf/v1/verifications`는 `{"ok": true, "expires_in": <seconds>}`를 반환하고 설정된 TTL로 `Max-Age`를 지정합니다.
- `GET /.waf/v1/session`은 `clearance_ttl`과 함께 인증된 브라우저에 `expires_at`(RFC 3339 UTC) 및 `expires_in`을 반환합니다.

## 난이도, 챌린지 유효 창 및 속도 제한

- `difficulty` (`1`–`5`, 기본 `4`): SHA-256 다이제스트 앞에 와야 하는 16진수 `0`의 개수 (난이도 `4`는 `0000...`).
- `challenge_ttl` (기본 `300`s / `5m`): 일회용 챌린지 논스 만료 시간.
- `clearance_ttl` / `verify_timeout` (기본 `86400`s / `24h`): 클리어런스 쿠키 또는 TCP IP 인증 유지 시간.
- `rate_limit` / `rate_window`: IP별 고정 윈도우 요청 제한. 사이트는 `60`초당 `240`회, `ssh`와 `minecraft` 블록은 `60`초당 `60`회가 기본값입니다.
