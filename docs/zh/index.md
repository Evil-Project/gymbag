---
layout: home

hero:
  name: Gymbag
  text: HTTPS 反向代理与工作量证明 WAF
  tagline: 零外部依赖的纯 Go 单二进制网关 — 集成多 CA 自动 ACME、两级本地 PKI、远程与内置 WAF 规则、自动更新威胁情报及 Anubis 风格 SHA-256 验证。
  image:
    src: /mascot-happy.png
    alt: Gymbag 吉祥物
  actions:
    - theme: brand
      text: 两条命令快速开始
      link: /zh/guide/getting-started
    - theme: alt
      text: Docker 部署指南
      link: /zh/guide/docker
    - theme: alt
      text: 与 Caddy / Nginx 性能对比
      link: /zh/guide/benchmarks

features:
  - icon: ⚡
    title: 高吞吐纯 Go 引擎
    details: 零分配 WAF 流水线，结合 Aho-Corasick DFA 模式匹配、分片限流与缓冲池技术 — 在开启完整 WAF 时仍可提供约 72.5k RPS（比 Caddy 快 32–69%）及 140k+ RPS 边缘拦截能力。
  - icon: 🛡️
    title: 开箱即用 WAF、PoW (0–3) 与自定义超时
    details: 内置规则拦截 /.env、/wp-admin、扫描器及 SQLi/XSS 探测；SHA-256 PoW 拦截 AI 爬虫，并支持自定义验证通过有效期（如 30m、12h、7d）。
  - icon: 🌐
    title: 远程规则与自动威胁情报
    details: 支持通过 HTTP/HTTPS 拉取并热更新远程防火墙规则（Gymbagfile 语法或 JSON），并自动拉取 IPsum、Spamhaus DROP、Feodo 及官方 AI 爬虫 CIDR 列表。
  - icon: 🔒
    title: 多 CA ACME 与本地 PKI
    details: 支持 Let's Encrypt、ZeroSSL（自动 EAB）、Google、SSL.com、LiteSSL 与 Buypass 的 RFC 8555 HTTP-01 自动签发，以及 *.internal 本地 CA 与自动 /etc/hosts 管理。
  - icon: 🐳
    title: Docker 与原生系统服务
    details: 提供多架构 GHCR 容器镜像（linux/amd64 与 arm64）和 Docker Compose + Redis 编排，并原生支持 systemd、launchd、OpenRC、Supervisor、Arch 与 NixOS。
  - icon: 🔌
    title: 实时 REST 管理 API 与热重载
    details: 使用 --watch 监控 Gymbagfile 自动热重载，或通过 HTTP API 实时调整验证级别、超时时间、本地规则与远程规则 URL，全程不中断现存连接。
---

<QuickStartBuilder />

## 交互式验证级别预览

点击下方不同的验证级别，查看 Gymbag 如何处理普通浏览器、AI 爬虫、自动化客户端、威胁情报 IP 以及漏洞探测请求：

<VerifyLevelMatrix />
