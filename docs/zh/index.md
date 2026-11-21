---
layout: home

hero:
  name: Gymbag
  text: HTTPS 反向代理与工作量证明 WAF
  tagline: 一个 Go 二进制，接管 HTTPS、跑多个 CA 的 ACME，再用 SHA-256 工作量证明把爬虫挡在门外。
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
    title: 代理路径够快
    details: Aho-Corasick 匹配、分片限流和复用缓冲区让 WAF 不挡道：走 TLS 大约 72.5k RPS，在边缘拦探测请求能到 140k+ RPS。
  - icon: 🛡️
    title: WAF 默认就开着
    details: 自带 /.env、/wp-admin、扫描器和 SQLi/XSS 探测规则。四档验证级别决定谁要过 SHA-256 挑战，通过后的有效期可以设成 30m、12h 或 7d。
  - icon: 🌐
    title: 远程规则与威胁情报
    details: 通过 HTTP 拉取防火墙规则并用 ETag 缓存，按定时器刷新 IPsum、Spamhaus DROP、Feodo 以及公开的 AI 爬虫 CIDR 列表。
  - icon: 🔒
    title: 证书不用操心
    details: 针对 Let's Encrypt、ZeroSSL、Google、SSL.com、LiteSSL、Buypass 跑 ACME HTTP-01。遇到 *.internal 会自己签发一套 CA，并把 /etc/hosts 也写好。
  - icon: 🐳
    title: 照旧方式部署
    details: GHCR 多架构镜像（amd64、arm64）、带 Redis 共享状态的 compose、Arch 软件包，以及 systemd、launchd、OpenRC、Supervisor 服务单元。
  - icon: 🔌
    title: 运行中改配置
    details: 用 --watch 改 Gymbagfile，或在 Admin API 上调整验证级别、超时、本地规则和远程规则 URL，已经连上的连接不会断。
---

<QuickStartBuilder />

## 交互式验证级别预览

点下面的按钮，可以看到 Gymbag 对普通浏览器、AI 爬虫、自动化客户端、威胁 IP 和漏洞探测分别怎么处理：

<VerifyLevelMatrix />
