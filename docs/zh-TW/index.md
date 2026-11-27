---
layout: home

hero:
  name: Gymbag
  text: HTTP/HTTPS、SSH 與 Minecraft 工作量證明閘道器
  tagline: 單一 Go 執行檔即可終止 HTTPS、透過瀏覽器工作量證明保護 SSH 與 Minecraft，並封鎖爬蟲與威脅 IP。
  image:
    src: /mascot-happy.png
    alt: Gymbag 吉祥物
  actions:
    - theme: brand
      text: 兩道指令快速上手
      link: /zh-TW/guide/getting-started
    - theme: alt
      text: SSH 與 Minecraft 防護指南
      link: /zh-TW/guide/ssh-minecraft
    - theme: alt
      text: 與 Caddy 及 Nginx 的效能基準測試
      link: /zh-TW/guide/benchmarks

features:
  - icon: ⚡
    title: 高速代理路徑
    details: "透過 Aho-Corasick 規則比對、分片式速率限制器與緩衝區物件池，讓 WAF 不拖慢連線速度：經 TLS 轉發可達約 72.5k RPS，在邊緣端直接攔截探測請求更可達 140k+ RPS。"
  - icon: 🛡️
    title: 同時防護 HTTP、SSH 與 Minecraft
    details: "單一處理程序即可保護網站、WebSocket、OpenSSH 與 Minecraft Java 伺服器。SSH 會在斷線畫面顯示 HTTPS 驗證連結；Minecraft 1.20.5+ 則會先將未驗證玩家傳送至純記憶體空世界，於聊天室提供可點擊連結，驗證完成後自動透過 Transfer 封包導回正式伺服器。"
  - icon: 🌐
    title: 威脅情資、VPN 與 AI 爬蟲清單
    details: "自動更新 IPsum、Spamhaus DROP、Feodo、X4BNet VPN/Tor 清單與官方 AI 爬蟲 CIDR，並支援透過 Shodan InternetDB、StopForumSpam、ipquery.io 與 ipapi.is 進行 fail-open 即時查詢。"
  - icon: 🔒
    title: ACME、IP 憑證與本機 PKI
    details: "支援 Let's Encrypt、ZeroSSL、Google、SSL.com、LiteSSL 與 Buypass 的 HTTP-01 驗證，支援用於驗證入口的 RFC 8738 ACME IP 憑證，並內建 CA 自動簽發 *.internal 本機憑證。"
  - icon: 🐳
    title: 套件與容器開箱即用
    details: "提供靜態執行檔、Debian/Ubuntu .deb 套件、Arch .pkg.tar.zst 套件、NixOS flake、多架構 GHCR 容器映像檔、搭配 Redis 的 Compose 設定，以及 systemd/launchd/OpenRC/Supervisor 服務檔。"
  - icon: 🔌
    title: 執行中動態調整設定
    details: "可自動產生 gymbag.json 或 Gymbagfile，直接讀取 nginx/Apache/Caddyfile v2 設定檔，監控規則目錄變動並即時套用，且提供不中斷連線的即時 Admin REST API。"
---

<QuickStartBuilder />

## 互動式驗證等級

選擇下方的驗證等級，查看 Gymbag 如何處理一般瀏覽器、AI 爬蟲、VPN/Tor IP、自動化用戶端、威脅情資清單與漏洞探測請求：

<VerifyLevelMatrix />
