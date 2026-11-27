---
layout: home

hero:
  name: Gymbag
  text: HTTP/HTTPS, SSH & Minecraft PoW 게이트웨이
  tagline: HTTPS를 종단하고, SSH와 Minecraft를 브라우저 작업 증명(PoW)으로 보호하며, 스크래퍼와 위협 IP를 차단하는 단일 Go 바이너리.
  image:
    src: /mascot-happy.png
    alt: Gymbag 마스코트
  actions:
    - theme: brand
      text: 두 줄 명령어 빠른 시작
      link: /ko/guide/getting-started
    - theme: alt
      text: SSH & Minecraft 가이드
      link: /ko/guide/ssh-minecraft
    - theme: alt
      text: Caddy / Nginx 벤치마크
      link: /ko/guide/benchmarks

features:
  - icon: ⚡
    title: 빠른 프록시 경로
    details: "Aho-Corasick 규칙 매칭, 샤드 단위 속도 제한, 버퍼 풀링으로 WAF 부하를 줄였습니다. TLS 경유 시 약 72.5k RPS, 엣지 프로브 차단 시 140k+ RPS를 처리합니다."
  - icon: 🛡️
    title: HTTP, SSH, Minecraft 통합 보호
    details: "웹사이트, WebSocket, OpenSSH, Minecraft Java 서버를 한 프로세스에서 보호합니다. SSH는 연결 해제 화면에 HTTPS 링크를 출력하고, Minecraft 1.20.5+는 미인증 플레이어를 메모리 내 빈 월드로 보내 채팅에 클릭 가능한 링크를 제공한 뒤 완료 시 Transfer로 자동 리디렉션합니다."
  - icon: 🌐
    title: 위협 인텔, VPN 및 AI 목록
    details: "IPsum, Spamhaus DROP, Feodo, X4BNet VPN/Tor 목록, 공식 AI 크롤러 CIDR을 자동 갱신하고 Shodan InternetDB, StopForumSpam, ipquery.io, ipapi.is로 fail-open 실시간 조회를 수행합니다."
  - icon: 🔒
    title: ACME, IP 인증서 & 로컬 PKI
    details: "Let's Encrypt, ZeroSSL, Google, SSL.com, LiteSSL, Buypass HTTP-01을 지원하고 인증 포털용 RFC 8738 ACME IP 인증서를 발급하며 *.internal 이름은 자체 CA로 서명합니다."
  - icon: 🐳
    title: 패키지 및 컨테이너 지원
    details: "정적 바이너리, Debian/Ubuntu .deb, Arch .pkg.tar.zst, NixOS flake, 멀티 아키텍처 GHCR 이미지, Redis 기반 Compose, systemd·launchd·OpenRC·Supervisor 유닛을 제공합니다."
  - icon: 🔌
    title: 실행 중 설정 변경
    details: "gymbag.json이나 Gymbagfile을 생성하고 nginx/Apache/Caddyfile v2 설정을 그대로 읽으며, 규칙 디렉터리 감시와 Admin REST API로 연결을 끊지 않고 설정을 바꿉니다."
---

<QuickStartBuilder />

## 대화형 검증 레벨

아래에서 검증 레벨을 선택해 일반 브라우저, AI 크롤러, VPN/Tor IP, 자동화 클라이언트, 위협 피드, 익스플로잇 프로브를 Gymbag이 어떻게 처리하는지 확인해 보세요.

<VerifyLevelMatrix />
