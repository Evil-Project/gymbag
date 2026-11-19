---
layout: home

hero:
  name: Gymbag
  text: HTTPS Reverse Proxy & Proof-of-Work WAF
  tagline: High-throughput, single-binary pure Go gateway with zero external dependencies — multi-CA ACME, two-tier local PKI, remote & built-in WAF rules, auto-updating threat feeds, and Anubis-style SHA-256 verification.
  image:
    src: /mascot-happy.png
    alt: Gymbag mascot
  actions:
    - theme: brand
      text: Two-Command Quick Start
      link: /guide/getting-started
    - theme: alt
      text: Docker Guide
      link: /guide/docker
    - theme: alt
      text: Benchmarks vs Caddy & Nginx
      link: /guide/benchmarks

features:
  - icon: ⚡
    title: High-Throughput Pure Go Engine
    details: Zero-allocation WAF pipeline with Aho-Corasick DFA pattern matching, sharded rate limiting, and pooled proxy buffers — delivering ~72.5k RPS (+32–69% faster than Caddy) and 140k+ RPS edge blocking.
  - icon: 🛡️
    title: Open-Box WAF, PoW (0–3) & Custom Timeouts
    details: Built-in rules block /.env, /wp-admin, scanners, and SQLi/XSS probes, while SHA-256 PoW challenges AI crawlers with configurable verified timeouts (e.g. 30m, 12h, 7d).
  - icon: 🌐
    title: Remote Rules & Threat Intelligence
    details: Sync remote firewall rules (Gymbagfile or JSON) over HTTP/HTTPS with ETag caching, plus auto-updating IPsum, Spamhaus DROP, Feodo, and official AI crawler CIDR feeds.
  - icon: 🔒
    title: Multi-CA ACME + Local PKI
    details: Automatic RFC 8555 ACME HTTP-01 across Let's Encrypt, ZeroSSL (auto-EAB), Google, SSL.com, LiteSSL, and Buypass — plus local *.internal CA and /etc/hosts management.
  - icon: 🐳
    title: Docker & Native System Services
    details: Multi-arch GHCR container images (linux/amd64 & arm64) with Docker Compose + Redis, or native systemd, macOS launchd, OpenRC, Supervisor, Arch, and NixOS support.
  - icon: 🔌
    title: Live REST Admin API & Hot Reload
    details: Hot-reload Gymbagfile with --watch, live-patch verify levels, timeouts, local rules, and remote rule URLs over HTTP without dropping active connections.
---

<QuickStartBuilder />

## Interactive Verify Levels

Click a verify level below to see how Gymbag handles browsers, AI crawlers, automation clients, threat-feed IPs, and exploit probes:

<VerifyLevelMatrix />
