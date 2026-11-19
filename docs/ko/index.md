---
layout: home

hero:
  name: Gymbag
  text: HTTPS 리버스 프록시 & Proof-of-Work WAF
  tagline: 외부 Go 의존성이 전혀 없는 순수 Go 단일 바이너리 게이트웨이 — 멀티 CA ACME, 2단계 로컬 PKI, 원격 및 내장 WAF 규칙, 자동 업데이트 위협 인텔리전스, Anubis 스타일 SHA-256 검증을 하나로 제공합니다.
  image:
    src: /mascot-happy.png
    alt: Gymbag 마스코트
  actions:
    - theme: brand
      text: 두 줄 명령어 빠른 시작
      link: /ko/guide/getting-started
    - theme: alt
      text: Docker 가이드
      link: /ko/guide/docker
    - theme: alt
      text: Caddy / Nginx 벤치마크
      link: /ko/guide/benchmarks

features:
  - icon: ⚡
    title: 고성능 순수 Go 엔진
    details: Aho-Corasick DFA 패턴 매칭, 샤딩된 속도 제한, 풀링된 프록시 버퍼 기반의 제로 할당 WAF 파이프라인으로 전체 WAF 활성화 상태에서도 ~72.5k RPS(Caddy 대비 +32~69% 빠름) 및 140k+ RPS 엣지 차단을 제공합니다.
  - icon: 🛡️
    title: 즉시 사용 가능한 WAF, PoW (0–3) & 커스텀 타임아웃
    details: 내장 규칙이 /.env, /wp-admin, 스캐너, SQLi/XSS 프로브를 차단하며 SHA-256 PoW로 AI 크롤러를 검증합니다. 인증 완료 유효 시간(30m, 12h, 7d 등)도 자유롭게 설정할 수 있습니다.
  - icon: 🌐
    title: 원격 규칙 & 자동 위협 인텔리전스
    details: HTTP/HTTPS로 원격 방화벽 규칙(Gymbagfile 구문 또는 JSON)을 ETag 캐싱과 함께 동기화하며 IPsum, Spamhaus DROP, Feodo, 공식 AI 크롤러 CIDR 목록을 자동 갱신합니다.
  - icon: 🔒
    title: 멀티 CA ACME + 로컬 PKI
    details: Let's Encrypt, ZeroSSL(자동 EAB), Google, SSL.com, LiteSSL, Buypass를 지원하는 RFC 8555 HTTP-01 자동 인증서 발급 및 *.internal 로컬 CA와 /etc/hosts 자동 관리를 제공합니다.
  - icon: 🐳
    title: Docker & 네이티브 시스템 서비스
    details: 멀티 아키텍처 GHCR 컨테이너 이미지(linux/amd64, arm64) 및 Docker Compose + Redis를 제공하며 systemd, launchd, OpenRC, Supervisor, Arch, NixOS를 지원합니다.
  - icon: 🔌
    title: 실시간 REST 관리 API & 핫 리로드
    details: --watch로 Gymbagfile을 자동 리로드하거나 HTTP API를 통해 연결을 끊지 않고 검증 레벨, 타임아웃, 로컬 및 원격 규칙을 실시간으로 수정할 수 있습니다.
---

<QuickStartBuilder />

## 대화형 검증 레벨

아래 버튼을 클릭해 일반 브라우저, AI 크롤러, 자동화 도구, 위협 IP 및 공격 프로브에 대한 Gymbag의 동작을 확인해 보세요:

<VerifyLevelMatrix />
