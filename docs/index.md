---
layout: home

hero:
  name: Gymbag
  text: HTTPS Reverse Proxy & Proof-of-Work WAF
  tagline: Single-binary pure Go gateway with zero external dependencies — multi-CA ACME, two-tier local PKI, auto-updating threat feeds, and Anubis-style SHA-256 verification.
  image:
    src: /mascot-happy.png
    alt: Gymbag mascot
  actions:
    - theme: brand
      text: Two-Command Quick Start
      link: /guide/getting-started
    - theme: alt
      text: System Service Setup
      link: /guide/system-service
    - theme: alt
      text: Gymbagfile Reference
      link: /reference/gymbagfile

features:
  - icon: ⚡
    title: Two-Command Setup
    details: Run gymbag init to generate a validated Gymbagfile (interactively or via flags), then run gymbag to start serving HTTPS immediately.
  - icon: 🛡️
    title: Open-Box WAF & PoW (0–3)
    details: Built-in rules block /.env, /wp-admin, scanners, and SQLi/XSS probes, while SHA-256 Proof-of-Work challenges AI crawlers and suspicious automation.
  - icon: 🌐
    title: Multi-CA ACME + Local PKI
    details: Automatic RFC 8555 ACME HTTP-01 across Let's Encrypt, ZeroSSL (auto-EAB), Google, SSL.com, LiteSSL, and Buypass — plus local *.internal CA and /etc/hosts management.
  - icon: 📡
    title: Auto-Updating Threat Intel
    details: Pulls IPsum, Spamhaus DROP v4/v6, Feodo Tracker, official AI crawler CIDRs (OpenAI, Anthropic, Perplexity, Google, Apple), plus Shodan InternetDB & StopForumSpam lookups.
  - icon: ⚙️
    title: Native System Services
    details: Register and control Gymbag with systemd (system or --user with CAP_NET_BIND_SERVICE), macOS launchd, OpenRC, or Supervisor via gymbag service.
  - icon: 🔌
    title: Live REST Admin API & Redis
    details: Hot-reload Gymbagfile with --watch, live-patch rules and verify levels over HTTP, and share PoW nonces and rate limits across replicas with built-in RESP2 Redis support.
---

<QuickStartBuilder />

## Interactive Verify Levels

Click a verify level below to see how Gymbag handles browsers, AI crawlers, automation clients, threat-feed IPs, and exploit probes:

<VerifyLevelMatrix />
