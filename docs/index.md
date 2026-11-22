---
layout: home

hero:
  name: Gymbag
  text: HTTP/HTTPS, SSH & Minecraft PoW Gateway
  tagline: A single Go binary that terminates HTTPS, guards SSH and Minecraft with browser proof-of-work, and blocks scrapers and threat IPs.
  image:
    src: /mascot-happy.png
    alt: Gymbag mascot
  actions:
    - theme: brand
      text: Two-Command Quick Start
      link: /guide/getting-started
    - theme: alt
      text: SSH & Minecraft Guide
      link: /guide/ssh-minecraft
    - theme: alt
      text: Benchmarks vs Caddy & Nginx
      link: /guide/benchmarks

features:
  - icon: ⚡
    title: Fast proxy path
    details: "Aho-Corasick rule matching, sharded rate limiters, and pooled buffers keep the WAF out of the way: ~72.5k RPS through TLS, and 140k+ RPS dropping probes at the edge."
  - icon: 🛡️
    title: HTTP, SSH, and Minecraft
    details: "Guards websites, WebSockets, OpenSSH, and Minecraft Java servers from one process. SSH and Minecraft challenges print an HTTPS link straight into the client disconnect screen."
  - icon: 🌐
    title: Threat intel, VPN & AI lists
    details: "Auto-refreshes IPsum, Spamhaus DROP, Feodo, X4BNet VPN/Tor lists, and official AI crawler CIDRs, plus fail-open live lookups via Shodan InternetDB, StopForumSpam, ipquery.io, and ipapi.is."
  - icon: 🔒
    title: ACME, IP certs & Local PKI
    details: "Runs HTTP-01 for Let's Encrypt, ZeroSSL, Google, SSL.com, LiteSSL, and Buypass, supports RFC 8738 ACME IP certs for verify portals, and signs *.internal names with its own CA."
  - icon: 🐳
    title: Packages & containers ready
    details: "Ships static binaries, Debian/Ubuntu .deb packages, Arch .pkg.tar.zst packages, a NixOS flake, multi-arch GHCR images, Redis-backed compose, and systemd/launchd/OpenRC/Supervisor units."
  - icon: 🔌
    title: Change config while it runs
    details: "Generates gymbag.json or Gymbagfile, reads nginx/Apache/Caddyfile v2 configs, watches rule directories for edits, and exposes a live Admin REST API without dropping connections."
---

<QuickStartBuilder />

## Interactive Verify Levels

Pick a verify level below to see how Gymbag treats browsers, AI crawlers, VPN/Tor IPs, automation clients, threat feeds, and exploit probes:

<VerifyLevelMatrix />
