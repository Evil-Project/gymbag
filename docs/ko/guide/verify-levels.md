# 검증 레벨 (`0`–`3`), Proof-of-Work & 타임아웃

단일 Gymbag 프로세스로 수십 개의 도메인을 동시에 호스팅하고 보호할 수 있으며, 멀티 업스트림 라운드 로빈 로드 밸런싱과 전이중 RFC 6455 WebSocket 프록시를 지원합니다. 각 사이트는 `verify <0-3>` 또는 `protect off`로 검증 정책을 개별 구성합니다.

<VerifyLevelMatrix />

## 챌린지 및 차단 상태 화면

Gymbag은 Proof-of-Work 브라우저 챌린지 페이지, SHA-256 솔버 스크립트 및 세 가지 상태의 마스코트 이미지를 Go 바이너리에 직접 내장합니다:

<div class="gb-mascot-strip">
  <div class="gb-mascot-tile">
    <img src="/mascot-pensive.png" alt="PoW 검증 중" />
    <strong>PoW 계산 중</strong>
    <span class="gb-badge challenge">챌린지 발급됨</span>
  </div>
  <div class="gb-mascot-tile">
    <img src="/mascot-happy.png" alt="검증 통과" />
    <strong>통과 승인됨</strong>
    <span class="gb-badge pass">waf_clearance 발급</span>
  </div>
  <div class="gb-mascot-tile">
    <img src="/mascot-reject.png" alt="요청 차단됨" />
    <strong>규칙 / 위협 차단</strong>
    <span class="gb-badge block">403 Forbidden</span>
  </div>
</div>

## 멀티 사이트 구성 예시

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
```

## 사용자 지정 인증 완료 타임아웃 (`verify_timeout` / `clearance_ttl`)

방문자가 SHA-256 Proof-of-Work를 통과하면 Gymbag은 호스트 이름과 클라이언트 IP에 바인딩된 `HttpOnly`, `SameSite=Lax` HMAC 서명 쿠키(기본 이름 `waf_clearance`)를 발급합니다.

기본 유효 시간은 **24시간**(`86400`초)입니다. 전역 또는 사이트별로 정수 초 **또는** 기간 문자열(`30m`, `12h`, `1h30m`, `7d`, `2w`)을 사용해 자유롭게 조정할 수 있습니다:

::: code-group
```caddyfile [Gymbagfile]
{
    # 모든 사이트의 기본 인증 유효 시간
    verify_timeout 12h
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
    # 스테이징 환경용 짧은 30분 타임아웃
    verify_timeout 30m
}

docs.example.com {
    reverse_proxy http://localhost:4000
    verify 2
    # 문서 사이트용 긴 7일 유효 시간 (clearance_ttl 별칭도 가능)
    clearance_ttl 7d
}
```

```bash [CLI 플래그]
# 6시간 인증 타임아웃으로 설정 생성
gymbag init -d app.example.com -u :3000 --verify-timeout 6h

# 실행 시 재정의
gymbag run --config Gymbagfile --verify-timeout 30m
gymbag reverse-proxy --from app.internal --to :3000 --verify 0 --verify-timeout 15m
```

```bash [환경 변수]
# 정수 초 또는 기간 형식(30m, 12h, 7d 등) 모두 지원
GYMBAG_VERIFY_TIMEOUT=12h gymbag
GYMBAG_CLEARANCE_TTL=3600 gymbag
```

```bash [실시간 관리 API]
# 재시작 없이 clearance_ttl(초 단위) 실시간 수정
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/staging.example.com \
  -H "Content-Type: application/json" \
  -d '{"clearance_ttl": 1800}'
```
:::

- `Gymbagfile` 지시어 별칭: `verify_timeout`, `verified_timeout`, `clearance_timeout`, `clearance_ttl`.
- 환경 변수 별칭: `GYMBAG_CLEARANCE_TTL`, `GYMBAG_VERIFY_TIMEOUT`, `GYMBAG_VERIFIED_TIMEOUT`.
- `POST /.waf/v1/verifications`는 `{"ok": true, "expires_in": <초>}`를 반환하고 쿠키 `Max-Age`를 설정합니다.
- `GET /.waf/v1/session`은 인증된 방문자에게 `clearance_ttl`, `expires_at`(RFC 3339 UTC) 및 남은 시간 `expires_in`을 반환합니다.

## 난이도, 챌린지 만료 및 속도 제한

- **`difficulty`** (`1`–`5`, 기본 `4`): SHA-256 해시에 필요한 선행 16진수 0의 개수.
- **`challenge_ttl`** (기본 `300`초 / `5m`): 일회용 PoW 챌린지 수명(초 또는 기간 문자열).
- **`clearance_ttl` / `verify_timeout`** (기본 `86400`초 / `24h`): 인증 완료 쿠키 수명(`30m`, `12h`, `7d` 등).
- **`rate_limit` / `rate_window`** (기본 `60`초당 `240`회): 64-샤드 인메모리 저장소(또는 Redis) 기반의 사이트별·IP별 고정 윈도우 속도 제한.
