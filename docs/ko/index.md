---
layout: home

hero:
  name: Gymbag
  text: HTTPS 리버스 프록시 & Proof-of-Work WAF
  tagline: HTTPS를 종료하고 여러 CA로 ACME를 돌리며 SHA-256 proof-of-work로 스크레이퍼를 막는 Go 바이너리 하나.
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
    title: 빠른 프록시 경로
    details: Aho-Corasick 매칭, 샤드 단위 속도 제한, 재사용 버퍼로 WAF가 길을 막지 않습니다. TLS를 거쳐 약 72.5k RPS, 엣지에서 프로브를 막을 때는 140k+ RPS.
  - icon: 🛡️
    title: 기본으로 켜지는 WAF
    details: /.env, /wp-admin, 스캐너, SQLi/XSS 프로브 규칙이 들어 있습니다. 네 단계 검증 레벨이 SHA-256 챌린지를 받을 대상을 정하고, 통과 후 허용 시간은 30m, 12h, 7d 중에 고를 수 있습니다.
  - icon: 🌐
    title: 원격 규칙과 위협 피드
    details: HTTP로 방화벽 규칙을 받아 ETag로 캐시하고, IPsum, Spamhaus DROP, Feodo, 공개된 AI 크롤러 CIDR 목록을 타이머로 갱신합니다.
  - icon: 🔒
    title: 인증서는 알아서
    details: Let's Encrypt, ZeroSSL, Google, SSL.com, LiteSSL, Buypass를 대상으로 ACME HTTP-01을 돌립니다. *.internal 이름은 자체 CA를 만들고 /etc/hosts 항목까지 써 줍니다.
  - icon: 🐳
    title: 익숙한 방식으로 배포
    details: GHCR 멀티 아키텍처 이미지(amd64, arm64), 공유 상태용 Redis가 들어간 compose, Arch 패키지, systemd·launchd·OpenRC·Supervisor 유닛.
  - icon: 🔌
    title: 실행 중에 설정 변경
    details: --watch로 Gymbagfile을 고치거나 Admin API로 검증 레벨, 타임아웃, 로컬 규칙, 원격 URL을 바꿉니다. 열려 있는 연결은 그대로 둡니다.
---

<QuickStartBuilder />

## 대화형 검증 레벨

아래 버튼을 눌러 일반 브라우저, AI 크롤러, 자동화 도구, 위협 IP, 공격 프로브에 Gymbag이 어떻게 반응하는지 확인해 보세요:

<VerifyLevelMatrix />
