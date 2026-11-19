# 방화벽 규칙, 원격 규칙 & 위협 인텔리전스

## 요청 평가 파이프라인

보호되는 사이트로 들어오는 모든 요청에 대해 Gymbag은 다음 순서로 보안 계층을 평가합니다:

1. **요청 형태 검사** — HTTP 메서드, 최대 URI 길이, 널 바이트 및 `..` / `%2e%2e` 경로 탐색 거부.
2. **ACME HTTP-01 우회** — `/.well-known/acme-challenge/*`는 항상 통과시켜 인증서 발급을 보장합니다.
3. **로컬 사용자 지정 규칙** — 전역 규칙 후 사이트 규칙을 위에서 아래로 평가 (**첫 번째 일치가 우선**).
4. **원격 규칙 (`remote_rules`)** — 구성된 HTTP/HTTPS URL에서 가져와 검증된 규칙을 선언 순으로 평가.
5. **내장 기본 규칙** — 민감한 파일, CMS 공격 프로브, 스캐너, SQLi/XSS 패턴 및 자동화 검증 (`default_rules off` 시 비활성화).
6. **사이트별·IP별 속도 제한** — `rate_limit` 및 `rate_window` 적용.
7. **IP 위협 인텔리전스** — `ip_security_action` (`verify` 기본 또는 `block`) 수행.
8. **Proof-of-Work 검증** — 설정된 `verify` 레벨 (`0`–`3`) 실행.

로컬 사용자 지정 규칙은 원격 규칙, 기본 규칙, 속도 제한 및 위협 피드**보다 먼저** 실행되므로 상단에 `allow` 규칙을 두어 사내 CIDR이나 헬스 체크, 미리보기 쿠키를 안전하게 허용할 수 있습니다.

---

## 한 줄 단축 규칙

구문: `<allow | verify | block> <field> <pattern...>` (`bypass`/`pass`, `challenge`/`pow`, `deny`/`drop`/`reject` 별칭 지원).

| 필드 | 매칭 방식 | 예시 |
| :--- | :--- | :--- |
| `ip` | IPv4, IPv6 또는 CIDR 대역 | `allow ip 127.0.0.1 10.0.0.0/8 2001:db8::/32` |
| `user_agent` | 대소문자 무관 부분 일치, `*` 글롭, `~정규식` | `block user_agent BadBot ~^(?i)python-urllib/` |
| `path` | 정확 일치/접두사, `*` 글롭, `~정규식` | `verify path /api/export* ~^/v[0-9]+/search` |
| `query` | 쿼리 문자열 일치, `*` 글롭, `~정규식` | `block query "union select" ~(?i)<script` |
| `method` | HTTP 메서드 (대소문자 무관) | `verify method POST DELETE` |
| `header` | `<Header-Name> [pattern]` (`""` = 없음/비어 있음, `*` = 존재) | `verify header Accept-Language ""` |
| `cookie` | `<cookie_name> [pattern]` (`""` = 없음/비어 있음, `*` = 존재) | `allow cookie vip_session *` |

---

## 복합 조건 `rule` 블록

단일 필드로 부족할 때는 `rule "name" { ... }`을 사용합니다. **같은 필드**의 여러 값은 **OR**, 같은 `rule` 블록 내의 **서로 다른 필드**는 **AND**로 결합됩니다:

```caddyfile
app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    rule "challenge-auth-mutations" {
        action verify
        method POST
        path /login* /api/auth/* /reset-password*
    }

    rule "block-headless-api-posts" {
        action block
        method POST
        path /api/*
        header Origin ""
    }
}
```

---

## 원격 규칙 (`remote_rules`)

Git 저장소 Raw URL, S3/R2, CDN, 내부 정책 서버 등의 HTTP/HTTPS URL에서 공유 방화벽 규칙을 가져올 수 있습니다. Gymbag은 시작 시 및 `remote_rules_refresh` 간격(기본 `5m` / `300`초)마다 자동 갱신합니다.

### 원격 규칙 설정

전역(모든 사이트 상속) 및 사이트별로 모두 구성할 수 있습니다:

::: code-group
```caddyfile [Gymbagfile]
{
    # 모든 사이트에 적용되는 조직 공용 원격 규칙
    remote_rules https://rules.example.com/org-baseline.rules
    remote_rules_refresh 5m
}

app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    # 로컬 규칙이 항상 원격 규칙보다 우선합니다
    allow ip 10.0.0.0/8

    # 사이트 전용 원격 규칙 (텍스트 또는 JSON)
    remote_rules https://rules.example.com/app-overrides.json
}
```

```bash [CLI & 환경 변수]
# CLI 플래그 (반복 또는 쉼표 구분)
gymbag run --config Gymbagfile \
  --remote-rules https://rules.example.com/baseline.rules \
  --remote-rules-refresh 10m

# 환경 변수
GYMBAG_REMOTE_RULES="https://rules.example.com/baseline.rules,https://rules.example.com/extra.json" \
GYMBAG_REMOTE_RULES_REFRESH=5m \
gymbag
```

```bash [Admin REST API]
# 동기화 상태, 규칙 수 및 오류 확인
curl http://127.0.0.1:2019/api/v1/rules/remote

# 즉시 원격 규칙 가져오기 및 WAF 엔진 원자적 리로드
curl -X POST http://127.0.0.1:2019/api/v1/rules/remote/refresh

# 특정 사이트의 원격 규칙 URL 실시간 업데이트
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"remote_rules": ["https://rules.example.com/emergency.rules"]}'
```
:::

### 지원되는 원격 규칙 형식

**Gymbagfile 규칙 구문**과 **JSON**을 모두 자동 감지합니다:

::: code-group
```caddyfile [Gymbagfile 텍스트 구문]
# 공유 원격 방화벽 규칙
block ip 198.51.100.0/24 203.0.113.42
block user_agent BadScraper ~^(?i)EvilBot/
verify path /api/export*

rule "challenge-xml-posts" {
    action verify
    method POST
    path /xmlrpc* /api/import*
    header Origin ""
}
```

```json [JSON 객체 (또는 배열)]
{
  "rules": [
    {
      "name": "block-abusive-cidr",
      "action": "block",
      "ips": ["198.51.100.0/24", "203.0.113.42"]
    },
    {
      "name": "verify-exports",
      "action": "verify",
      "methods": ["GET", "POST"],
      "paths": ["/api/export*"]
    }
  ]
}
```
:::

### 캐싱 및 장애 안전 보장

- **조건부 요청**: `If-None-Match` (`ETag`)와 `If-Modified-Since`를 전송해 `304 Not Modified` 응답 시 재컴파일을 건너뜁니다.
- **디스크 캐시**: 검증된 규칙을 `<cert_dir>/rules-cache/`에 저장해 시작 즉시 로드합니다.
- **장애 안전 검증**: 활성화 전 구문과 필드를 엄격히 검증합니다(최대 4 MiB, 15초 타임아웃). 원격 오류나 잘못된 구문이 감지되면 직전 정상 규칙 세트를 유지하고 `GET /api/v1/rules/remote`에 오류를 표시합니다.
- **무중단 교체**: 규칙 변경 시 WAF 엔진(O(1) 경로 맵 및 Aho-Corasick DFA 포함)을 재컴파일하고 활성 연결을 끊지 않고 라우팅 스냅샷을 원자적으로 교체합니다.

---

## 내장 규칙 제어 (`default_rules on|off`)

`default_rules`는 기본값이 `on`입니다:
- 로컬/원격 사용자 지정 규칙이 **없을 때**는 내장 차단 규칙(`/.env`, `/.git/*`, `/wp-admin/*`, `sqlmap`/`nuclei`, SQLi/XSS)과 자동화 검증 규칙(`curl`, `wget`, `python-requests`, `headlesschrome`, `playwright`, 빈 `User-Agent`)이 모두 적용됩니다.
- 사용자 지정 규칙을 **추가하면** 해당 규칙이 먼저 평가된 뒤 내장 **차단** 규칙이 이어서 실행되므로 실수로 `/.env` 등이 노출되지 않습니다.
- 완전한 수동 제어를 원할 경우 `default_rules off`를 지정하세요.

---

## 자동 업데이트 위협 및 AI 크롤러 피드

모든 피드는 시작 시 및 `intel_refresh`(기본 `21600`초 / `6h`)마다 자동 갱신되며 `<cert_dir>/intel-cache`에 `ETag`/`If-Modified-Since` 기반으로 캐시됩니다:

- **위협 피드 (`ip_security on`)**: IPsum level 3+, Spamhaus DROP IPv4/IPv6, abuse.ch Feodo Tracker.
- **실시간 IP 평판 (`ip_reputation on`)**: 키 없이 Shodan InternetDB와 StopForumSpam 조회 (TTL 캐시 및 fail-open 지원).
- **공식 AI 크롤러 IP 대역 (`ai_ip_list on`)**: OpenAI, Anthropic, Perplexity, Google, Apple 공식 CIDR 목록.

```caddyfile
{
    ip_security on
    ip_security_action block   # "verify" (PoW 챌린지) 또는 "block" (403 거부)
    ip_reputation on
    ai_ip_list on
    intel_refresh 6h
}
```
