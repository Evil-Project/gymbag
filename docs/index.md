---
layout: home

hero:
  name: Gymbag
  text: HTTPS Reverse Proxy & Proof-of-Work WAF
  tagline: A single Go binary that terminates HTTPS, runs ACME for several CAs, and blocks scrapers with a SHA-256 proof-of-work challenge.
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
    title: Fast proxy path
    details: "Aho-Corasick rule matching, sharded rate limiters and pooled buffers keep the WAF out of the way: about 72.5k RPS through TLS, and 140k+ RPS when blocking probes at the edge."
  - icon: 🛡️
    title: WAF on by default
    details: Ships with rules for /.env, /wp-admin, scanners and SQLi/XSS probes. Four verify levels decide who gets a SHA-256 challenge, and clearance can last 30m, 12h or 7d.
  - icon: 🌐
    title: Remote rules and threat feeds
    details: Pull firewall rules over HTTP with ETag caching, and refresh IPsum, Spamhaus DROP, Feodo and the published AI crawler CIDR lists on a timer.
  - icon: 🔒
    title: Certificates without the fuss
    details: ACME HTTP-01 against Let's Encrypt, ZeroSSL, Google, SSL.com, LiteSSL or Buypass. For *.internal names it mints its own CA and writes the /etc/hosts entries for you.
  - icon: 🐳
    title: Deploy how you already deploy
    details: Multi-arch images on GHCR (amd64 and arm64), a Compose file with Redis for shared state, packages for Arch, and service units for systemd, launchd, OpenRC and Supervisor.
  - icon: 🔌
    title: Change config while it runs
    details: Edit the Gymbagfile with --watch, or patch verify levels, timeouts, local rules and remote rule URLs through the Admin API. Active connections stay up.
---

<QuickStartBuilder />

## Interactive Verify Levels

Click a verify level below to see how Gymbag handles browsers, AI crawlers, automation clients, threat-feed IPs, and exploit probes:

<VerifyLevelMatrix />
