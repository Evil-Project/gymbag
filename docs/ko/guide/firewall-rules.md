# 방화벽 규칙, 원격 규칙 & 위협 인텔리전스

## 요청 평가 파이프라인

보호된 HTTP 사이트에 들어오는 요청은 다음 순서로 각 단계를 거칩니다.

1. **요청 형식 검사**: HTTP 메서드 검증, 최대 URI 길이, 널 바이트 및 `..` / `%2e%2e` 경로 탐색 거부.
2. **ACME HTTP-01 우회**: 인증서 발급이 막히지 않도록 `/.well-known/acme-challenge/*`는 항상 통과시킵니다.
3. **로컬 사용자 지정 규칙**: 전역 규칙 다음 사이트 규칙을 위에서 아래로 평가하며 첫 일치 항목이 적용됩니다.
4. **원격 규칙 (`remote_rules`)**: HTTP/HTTPS URL에서 가져와 검증한 규칙을 순서대로 평가합니다.
5. **내장 기본 규칙**: 민감한 경로, CMS 프로브, 스캐너, SQLi/XSS 쿼리, 자동화 PoW 규칙 (`default_rules off` 시 비활성화).
6. **사이트별·IP별 속도 제한기**: `rate_limit` 및 `rate_window` (기본 `240` / `60s`).
7. **IP 위협 인텔리전스**: `ip_security_action verify`(기본값) 또는 `block`.
8. **작업 증명(PoW) 검증**: 설정된 `verify` 레벨(`0`–`3`)을 적용하며 VPN/프록시/Tor 탐지(`1`, `3`) 및 AI 크롤러 매칭(`2`, `3`)이 포함됩니다.

로컬 사용자 지정 규칙이 원격 규칙, 기본 규칙, 속도 제한, 위협 피드보다 먼저 실행되므로 사이트 블록 맨 위에 `allow` 규칙을 두면 사내 CIDR, 헬스 체크, 미리보기 쿠키를 바로 화이트리스트로 뺄 수 있습니다. `{ ... }` 안에 적은 전역 규칙은 `ssh`와 `minecraft` 서비스에서도(`ssh { ... }` 및 `minecraft { ... }` 내부의 서비스별 규칙과 함께) 실행됩니다.

## 한 줄 단축 규칙

문법: `<allow | verify | block> <field> <pattern...>` (`bypass`/`pass`, `challenge`/`pow`, `deny`/`drop`/`reject` 별칭도 지원)

| 필드 | 매칭 규칙 | 예시 |
| :--- | :--- | :--- |
| `ip` | IPv4, IPv6 또는 CIDR 대역 | `allow ip 127.0.0.1 10.0.0.0/8 2001:db8::/32` |
| `user_agent` | 대소문자 무관 부분 문자열, `*` 글롭 또는 `~regex` | `block user_agent BadBot ~^(?i)python-urllib/` |
| `path` | 정확/접두사 일치, `*` 글롭 또는 `~regex` | `verify path /api/export* ~^/v[0-9]+/search` |
| `query` | 원시/디코딩 쿼리 일치, `*` 글롭 또는 `~regex` | `block query "union select" ~(?i)<script` |
| `method` | HTTP 메서드 (대소문자 무관) | `verify method POST DELETE` |
| `header` | `<Header-Name> [pattern]` (`""` = 누락/비어 있음, `*` = 존재) | `verify header Accept-Language ""` |
| `cookie` | `<cookie_name> [pattern]` (`""` = 누락/비어 있음, `*` = 존재) | `allow cookie vip_session *` |

## 다중 조건 `rule` 블록

여러 조건이 동시에 맞아야 할 때는 `rule "name" { ... }` 블록을 씁니다. 같은 필드에 여러 값을 적으면 OR로 묶이고, 한 블록 안의 서로 다른 필드는 AND로 결합됩니다.

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

## 원격 규칙 (`remote_rules`)

하나 이상의 HTTP/HTTPS URL(Git raw 파일, 오브젝트 스토리지 버킷, CDN, 사내 정책 서버 등)에서 공용 규칙 세트를 가져올 수 있습니다. Gymbag은 시작 시 및 `remote_rules_refresh`(기본 `5m` / `300s`)마다 이를 새로 고칩니다.

::: code-group
```caddyfile [Gymbagfile]
{
    remote_rules https://rules.example.com/org-baseline.rules
    remote_rules_refresh 5m
}

app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    allow ip 10.0.0.0/8
    remote_rules https://rules.example.com/app-overrides.json
}
```

```bash [CLI & 환경 변수]
gymbag run --config Gymbagfile \
  --remote-rules https://rules.example.com/baseline.rules \
  --remote-rules-refresh 10m

GYMBAG_REMOTE_RULES="https://rules.example.com/baseline.rules,https://rules.example.com/extra.json" \
GYMBAG_REMOTE_RULES_REFRESH=5m \
gymbag
```

```bash [Admin REST API]
curl http://127.0.0.1:2019/api/v1/rules/remote
curl -X POST http://127.0.0.1:2019/api/v1/rules/remote/refresh

curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"remote_rules": ["https://rules.example.com/emergency.rules"]}'
```
:::

### 지원하는 원격 규칙 형식

**Gymbagfile 규칙 문법**과 **JSON**(`[ ... ]` 또는 `{"rules": [ ... ]}`)을 모두 지원합니다.

::: code-group
```caddyfile [일반 텍스트 Gymbagfile 문법]
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

```json [JSON 객체]
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

### 캐싱 및 검증

- `If-None-Match`(`ETag`)와 `If-Modified-Since`를 보내 `304 Not Modified` 응답 시 규칙 재컴파일을 건너뜁니다.
- 검증된 페이로드를 `<cert_dir>/rules-cache/`에 캐시해 두고, 시작 시 첫 번째 원격 요청이 끝나기 전에 디스크 캐시를 먼저 불러옵니다.
- 교체 전 페이로드 전체를 검증합니다(최대 4 MiB, 15s 타임아웃). 업스트림이 문법 오류나 HTTP 오류를 반환하면 기존 규칙 세트를 유지하고 `GET /api/v1/rules/remote`에 오류를 표시합니다.
- 규칙 변경 사항은 활성 연결을 끊지 않고 원자적으로 교체됩니다.

## 로컬 규칙 파일, 폴더 및 핫 리로드

별도의 `.rules` / `.json` 파일이나 디렉터리에 규칙을 관리하고, 소스를 `dynamic`으로 지정해 디스크 변경을 감지해 자동 리로드할 수 있습니다.

::: code-group
```caddyfile [Gymbagfile]
{
    rules_watch_interval 2s
}

app.example.com {
    reverse_proxy http://localhost:3000
    rules_file ./rules/custom.rules dynamic
    rules_dir ./rules.d dynamic
    ruleset static-corp {
        source ./rules/corp.json
        dynamic off
    }
}
```

```json [gymbag.json]
{
  "sites": [
    {
      "domain": "app.example.com",
      "upstreams": ["http://localhost:3000"],
      "waf": {
        "rule_sources": [
          {"source": "./rules.d", "dynamic": true, "name": "shared"}
        ]
      }
    }
  ]
}
```

```bash [CLI]
gymbag init -d app.example.com -u :3000 \
  --rules-file ./rules/custom.rules --rules-dir ./rules.d \
  --rules-dynamic --rules-watch-interval 2s

gymbag ruleset init ./rules/custom.rules
gymbag ruleset add --source ./rules --dynamic --name local-dir
gymbag ruleset update local-dir --static
gymbag ruleset validate ./rules
gymbag ruleset rule add --file ./rules/custom.rules --action block --name bad-bot --ua "*BadBot*"
gymbag ruleset remove local-dir
```
:::

폴링 주기는 `rules_watch_interval`(기본 `2s`)입니다. `rule_sources`는 `PATCH /api/v1/sites/{domain}`으로도 수정할 수 있습니다. `ruleset` 명령 목록은 [CLI & 환경 변수](/ko/reference/cli-env)를 참고하세요.

## 내장 규칙 (`default_rules on|off`)

`default_rules`의 기본값은 `on`입니다.
- 사이트에 로컬·원격 사용자 지정 규칙이 **없을 때**는 내장 차단 규칙(`/.env`, `/.git/*`, `/wp-admin/*`, `sqlmap`/`nuclei`, SQLi/XSS 쿼리)과 자동화 PoW 규칙(`curl`, `wget`, `python-requests`, `headlesschrome`, `playwright`, 빈 `User-Agent`)을 모두 실행합니다.
- 사이트에 사용자 지정 규칙을 추가하면 사용자 규칙이 먼저 실행되고 내장 **차단** 규칙이 그 뒤에 계속 붙으므로, `verify path` 규칙을 하나 추가했다고 `/.env`가 열리지 않습니다.
- 내장 규칙을 완전히 끄려면 사이트 또는 전역 설정에 `default_rules off`를 지정하세요.

## 위협 피드, VPN/Tor 탐지 및 AI 크롤러 목록

피드는 시작 시 및 `intel_refresh`(기본 `21600`s / `6h`)마다 갱신되며 `<cert_dir>/intel-cache`에 캐싱됩니다.

- **위협 피드 (`ip_security on`)**: IPsum level 3+, Spamhaus DROP IPv4 & IPv6, abuse.ch Feodo Tracker.
- **실시간 IP 평판 (`ip_reputation on`)**: Shodan InternetDB와 StopForumSpam을 API 키 없이 조회합니다(1시간 TTL 캐시 및 fail-open 타임아웃).
- **VPN / 프록시 / Tor 탐지 (`vpn_detection on`)**: 자동 갱신되는 X4BNet VPN IPv4/IPv6 목록과 Tor 출구 노드 목록에 더해 `api.ipquery.io` 및 `api.ipapi.is`에 키 없는 실시간 조회(1.2s 타임아웃, singleflight 중복 제거, 1시간 캐시, fail-open)를 수행합니다. 플래그가 찍힌 IP는 HTTP, SSH, Minecraft 모두에서 검증 레벨 `1`과 `3`의 **의심(suspicious)** 대상으로 처리됩니다.
- **공식 AI 크롤러 CIDR (`ai_ip_list on`)**: OpenAI(`gptbot`, `searchbot`, `chatgpt-user`), Anthropic(`claude.com/crawling/bots.json`), Perplexity, Google, Apple(`applebot`).

```caddyfile
{
    ip_security on
    ip_security_action block   # "verify" (PoW 챌린지) 또는 "block" (403 Forbidden)
    ip_reputation on
    vpn_detection on
    ai_ip_list on
    intel_refresh 6h
    # vpn_feed https://rules.example.com/extra-vpn-cidrs.txt
}
```
