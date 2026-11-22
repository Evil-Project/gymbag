---
layout: home

hero:
  name: Gymbag
  text: HTTP/HTTPS、SSH 与 Minecraft PoW 网关
  tagline: 单个 Go 二进制文件同时处理 HTTPS 终结、用浏览器工作量证明防护 SSH 与 Minecraft，并拦截爬虫和威胁 IP。
  image:
    src: /mascot-happy.png
    alt: Gymbag 吉祥物
  actions:
    - theme: brand
      text: 两条命令快速开始
      link: /zh/guide/getting-started
    - theme: alt
      text: SSH 与 Minecraft 指南
      link: /zh/guide/ssh-minecraft
    - theme: alt
      text: 与 Caddy / Nginx 基准对比
      link: /zh/guide/benchmarks

features:
  - icon: ⚡
    title: 代理路径够快
    details: Aho-Corasick 规则匹配、分片限流和缓冲池让 WAF 不拖慢代理：走 TLS 约 72.5k RPS，边缘直接丢弃探测请求可达 140k+ RPS。
  - icon: 🛡️
    title: HTTP、SSH 与 Minecraft
    details: 单进程同时防护网站、WebSocket、OpenSSH 和 Minecraft Java 服务器。SSH 与 Minecraft 的质询会直接把 HTTPS 验证链接打在客户端断开界面里。
  - icon: 🌐
    title: 威胁情报、VPN 与 AI 列表
    details: 自动刷新 IPsum、Spamhaus DROP、Feodo、X4BNet VPN/Tor 列表和官方 AI 爬虫 CIDR，并接入 Shodan InternetDB、StopForumSpam、ipquery.io 与 ipapi.is 故障放行实时查询。
  - icon: 🔒
    title: ACME、IP 证书与本地 PKI
    details: 支持 Let's Encrypt、ZeroSSL、Google、SSL.com、LiteSSL 与 Buypass 的 HTTP-01，为验证门户申请 RFC 8738 ACME IP 证书，并用自有 CA 签发 *.internal 域名。
  - icon: 🐳
    title: 软件包与容器就绪
    details: 提供静态二进制、Debian/Ubuntu .deb、Arch .pkg.tar.zst、NixOS flake、多架构 GHCR 镜像、带 Redis 的 Compose，以及 systemd/launchd/OpenRC/Supervisor 服务单元。
  - icon: 🔌
    title: 运行时修改配置
    details: 可生成 gymbag.json 或 Gymbagfile，直接读取 nginx/Apache/Caddyfile v2 配置，监听规则目录变更并热加载，还能通过 Admin REST API 不断流调整策略。
---

<QuickStartBuilder />

## 交互式验证级别

在下方选择验证级别，查看 Gymbag 对普通浏览器、AI 爬虫、VPN/Tor IP、自动化客户端、威胁情报源和漏洞探测的处理方式：

<VerifyLevelMatrix />
