# Gymbag

<p align="center">
  <img src="docs/public/mascot-happy.png" alt="Gymbag mascot" width="140" />
</p>

<p align="center">
  <strong>纯 Go 单二进制 HTTP/HTTPS、SSH 与 Minecraft 工作量证明防护网关</strong>
</p>

<p align="center">
  <a href="https://github.com/Evil-Project/gymbag/releases"><img src="https://img.shields.io/github/v/release/Evil-Project/gymbag?include_prereleases&label=Release" alt="Release" /></a>
  <a href="https://github.com/Evil-Project/gymbag/pkgs/container/gymbag"><img src="https://img.shields.io/badge/Docker-GHCR-blue?logo=docker" alt="Docker GHCR" /></a>
  <a href="go.mod"><img src="https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go" alt="Go 1.27+" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache--2.0-green" alt="Apache-2.0 License" /></a>
  <a href="https://evil-project.github.io/gymbag/zh/"><img src="https://img.shields.io/badge/Docs-VitePress-b81d33" alt="Docs" /></a>
</p>

<p align="center">
  <strong>README:</strong>
  <a href="README.md">English</a> ·
  <a href="README.zh.md">简体中文</a> ·
  <a href="README.ja.md">日本語</a> ·
  <a href="README.ko.md">한국어</a> ·
  <a href="README.es.md">Español</a>
  <br />
  <sub>在线文档：<a href="https://evil-project.github.io/gymbag/">EN</a> · <a href="https://evil-project.github.io/gymbag/zh/">ZH</a> · <a href="https://evil-project.github.io/gymbag/ja/">JA</a> · <a href="https://evil-project.github.io/gymbag/ko/">KO</a> · <a href="https://evil-project.github.io/gymbag/es/">ES</a></sub>
</p>

Gymbag 是一个单二进制 HTTPS 反向代理与工作量证明（PoW）WAF（思路类似 Anubis），同时支持防护 SSH 和 Minecraft 服务器。它仅依赖 Go 标准库，没有第三方 Go 模块；HTML 页面、浏览器端 SHA-256 求解脚本以及吉祥物图片都通过 `go:embed` 直接编译进二进制文件。

自建应用、Git 托管站、SSH 端口和 Minecraft 服务器面对的往往是同一批流量：无视 `robots.txt` 的 LLM 爬虫、四处扫 `/.env` 和 `/wp-admin` 的探测脚本、SSH 爆破工具以及登录洪水攻击。与其把 nginx 或 Caddy、独立的 PoW sidecar、fail2ban 和一堆自定义防火墙脚本拼在一起，不如直接用 Gymbag 在单个进程里搞定 TLS 终结、多 CA ACME、HTTP/WebSocket 代理、SSH/Minecraft TCP 拦截、IP 威胁/VPN/AI 列表和防火墙规则，配置使用 `gymbag.json` 或 Caddy 风格的 `Gymbagfile`。

## 快速开始

[GitHub Releases](https://github.com/Evil-Project/gymbag/releases) 上提供预编译静态二进制文件（`CGO_ENABLED=0`，覆盖 Linux、macOS 和 Windows 的 `amd64`/`arm64`）、Debian/Ubuntu `.deb` 包、Arch `.pkg.tar.zst` 包以及 NixOS tarball/NAR。也可以使用 GHCR 容器镜像、Nix flake，或直接从源码编译：

```bash
go build -o gymbag ./cmd/gymbag
```

**Debian / Ubuntu：**
```bash
sudo apt install ./gymbag_<version>_linux_amd64.deb
sudo gymbag init -o /etc/gymbag/Gymbagfile --format gymbagfile
sudo systemctl enable --now gymbag
```

**Arch Linux：**
```bash
sudo pacman -U gymbag-<version>-1-x86_64.pkg.tar.zst
# 或从 packaging/arch 本地构建：
cd packaging/arch && makepkg -si
```

**Nix / NixOS：**
```bash
nix run github:Evil-Project/gymbag -- --help
nix profile install github:Evil-Project/gymbag
```
静态编译的 Linux 发行二进制无需 `patchelf` 或 `nix-ld` 即可在 NixOS 上直接运行，同时 `nixosModules.default` 提供了声明式的 `services.gymbag` 模块。

只需两条命令即可开始：先生成配置（默认写入 `gymbag.json`，传入 `--format gymbagfile` 则生成 `Gymbagfile`），再启动服务：

```bash
# 防护 HTTP/HTTPS 网站
./gymbag init -d app.example.com -u http://localhost:3000
./gymbag

# 防护 SSH（将 sshd 迁到随机端口并配置主机防火墙）
sudo ./gymbag init -t ssh
sudo ./gymbag

# 防护 Minecraft Java 版（自动选择兼容 Java 启动 server.jar 并随机修改端口）
./gymbag init -t minecraft --server-jar /srv/mc/paper.jar --mc-jvm-args "-Xms1G -Xmx4G"
./gymbag

# 防护既有的 Minecraft 服务器且不托管启动（随机修改 server.properties 中的端口）
./gymbag init -t minecraft --server-properties /path/to/server.properties
./gymbag

# 在同一个进程中同时防护以上三类服务
sudo ./gymbag init -t http,ssh,minecraft -d app.example.com -u :3000 --server-jar /srv/mc/paper.jar
sudo ./gymbag
```

也可以让 Gymbag 直接读取现有的 **nginx**（`nginx.conf`）、**Apache**（`httpd.conf` / `apache2.conf`）或 **Caddyfile v2** 配置，或者将其注册为系统服务（`systemd`、`launchd`、`openrc` 或 `supervisor`）：

```bash
./gymbag init -d app.example.com -u :3000 --service auto
./gymbag service start
```

不写配置文件做临时测试时，可以直接跑 `reverse-proxy`：

```bash
./gymbag reverse-proxy --from app.internal --to :3000 --verify 3
```

> **1024 以下端口权限：** `:80` 用于 HTTP 跳 HTTPS 重定向和 ACME HTTP-01 验证，`:443` 提供 HTTPS 服务，`:22` 监听 SSH。在 Linux 上运行 `sudo setcap 'cap_net_bind_service=+ep' ./gymbag` 后即可用非 root 身份绑定这些端口（`.deb` 和 Arch 软件包已自动设置该能力）。仅做本地无特权 Web 测试时，可改用 `--http-port 8080 --https-port 8443`。

### 不写规则时默认生效的能力

在流量前面挂上 Gymbag 之前，不需要自己写正则或导入黑名单。只要写上不带参数的 `verify`（启用级别 `3`：`suspicious & ai`）或 `block`（将威胁情报命中从质询改为直接返回 `403`），就会自动启用三层防护：

1. **内置基线规则（`default_rules on`）**
   - 敏感路径（`403`）：`/.env`、`/.env.*`、`/.git`、`/.git/*`、`/.svn/*`、`/.hg/*`、`/.DS_Store`、`/.aws/*`、`/.ssh/*`、`/docker-compose.yml`、`/docker-compose.yaml`、`*.bak`、`*.sql`、`*.sqlite`。
   - CMS 与框架探测（`403`）：`/wp-admin`、`/wp-admin/*`、`/wp-login.php`、`/xmlrpc.php`、`/phpmyadmin*`、`/vendor/phpunit*`、`/cgi-bin/*`、`/actuator/env*`、`/actuator/heapdump*`。
   - 漏洞扫描器（`403`）：`sqlmap`、`nikto`、`nuclei`、`masscan`、`zgrab`、`gobuster`、`dirbuster`、`ffuf`、`wfuzz`、`feroxbuster`、`acunetix`、`nessus`、`openvas`。
   - 注入与路径穿越查询（`403`）：拦截查询字符串中的 `UNION SELECT`、`<script`、`javascript:`、`/etc/passwd`、`information_schema`、`../`、`cmd.exe`、`/bin/sh`，同时做请求格式检查，拒绝空字节和 `%2e%2e` 路径穿越。
   - 自动化客户端（PoW 质询）：站点尚未配置自定义规则时，CLI 请求库（`curl`、`wget`、`python-requests`、`python-httpx`、`go-http-client`、`aiohttp`、`okhttp`、`axios`、`node-fetch`、`scrapy`）、无头浏览器（`headlesschrome`、`playwright`、`puppeteer`、`selenium`、`phantomjs`）以及空 `User-Agent` 都会进入工作量证明页面。
2. **自动更新的 IP 威胁情报与 VPN/Tor 检测**
   - 启动时及每 6 小时拉取一次 [IPsum (level 3+)](https://raw.githubusercontent.com/stamparm/ipsum/master/levels/3.txt)、[Spamhaus DROP IPv4](https://www.spamhaus.org/drop/drop_v4.json) / [IPv6](https://www.spamhaus.org/drop/dropv6.txt) 和 [abuse.ch Feodo Tracker](https://feodotracker.abuse.ch/downloads/ipblocklist.txt)，利用 `ETag` 和 `If-Modified-Since` 缓存在 `<cert_dir>/intel-cache`。
   - 接入免密钥的 **Shodan InternetDB**（`internetdb.shodan.io`）和 **StopForumSpam**（`api.stopforumspam.org`）实时信誉查询，带 1 小时内存缓存和故障放行（fail-open）超时，上游响应慢不会卡住正常访客。
   - 免费 VPN/代理/Tor 检测（`vpn_detection on`）：加载自动更新的 X4BNet VPN IPv4/IPv6 列表和 Tor 出口节点列表，并结合免密钥实时查询（`api.ipquery.io` 和 `api.ipapi.is`，1 小时缓存，1.2s 故障放行超时）。命中的 IP 在 HTTP、SSH 和 Minecraft 的验证级别 `1` 和 `3` 下均被判定为 **suspicious**。
3. **官方 AI 爬虫 IP 列表**
   - 追踪 **OpenAI**（`gptbot`、`searchbot`、`chatgpt-user`）、**Anthropic**（`claude.com/crawling/bots.json`）、**Perplexity**（`perplexitybot`、`perplexity-user`）、**Google**（`user-triggered-agents`、`user-triggered-fetchers-google`）和 **Apple**（`applebot`）公开的 CIDR 网段，即便爬虫把 `User-Agent` 伪装成 Chrome，依然能按来源 IP 识别出来。

## SSH 与 Minecraft 防护

当未验证的 SSH 或 Minecraft 连接命中 Gymbag 并触发配置的验证级别时，Gymbag 不会将连接转给后端守护进程或游戏服务端，而是下发一次性 HTTPS 验证链接（`/v/<token>`）——SSH 会将其打印在断开提示中，Minecraft 1.20.5+ 则将玩家送入纯内存空世界并在聊天栏发送可点击链接，验证通过后通过 `Transfer` 自动重定向回真实服务器：

- **SSH（默认 `port` `22`）**：`sudo ./gymbag init -t ssh` 会挑选 `20000–59999` 范围内的空闲本地端口，将 `/etc/ssh/sshd_config` 备份到 `/etc/ssh/sshd_config.gymbag.bak`，把新 `Port` 写入所有 `Match` 块之前，执行 `sshd -t` 校验（失败会自动回滚），重载 `sshd`，并更新当前生效的 Linux 防火墙（`ufw`、`firewalld`、`nftables` 或 `iptables`+`ip6tables`），让后端端口仅允许回环访问，同时保持 `22` 和 `8443` 对外放行。（修改 `sshd_config` 时请保持另一个 SSH 终端不断开；如需手动接管，可传入 `--ssh-upstream 127.0.0.1:2222`、`--no-auto-setup` 或 `--no-firewall`。）IP 在浏览器通过质询后，后续连接直接走原始 TCP 透传，不干扰 SSH 握手。在级别 `1` 和 `3` 下，畸形 banner 或脚本化 SSH 客户端（`libssh`、`paramiko`、`zgrab`、`masscan`、`nmap`、`asyncssh`、`ssh2go`、`jsch`、`hydra`、`medusa`、`patator`、`python`、`curl`、`ssh-2.0-go`）会触发验证，终端内会直接打印带验证 URL 的 RFC 4253 `SSH_MSG_DISCONNECT` 消息。
- **Minecraft Java（默认 `port` `25565`）**：`./gymbag init -t minecraft --server-properties ./server.properties` 会将 `server.properties` 备份为 `server.properties.gymbag.bak`，把 `server-port` 改为 `20000–59999` 间的空闲随机端口，并将空的或通配的 `server-ip` 设为 `127.0.0.1`。服务器列表 Ping（`nextState == 1` 与旧版 `0xFE`）默认直接放行（除非被规则拦截），MOTD 和在线人数正常显示。遇到 Login 或 Transfer 且需要验证时，受支持的客户端（`1.20.5`–`26.1`）会进入一个纯内存的微型空世界（不在磁盘保存任何世界文件），在聊天栏收到可点击的验证链接，并在通过后通过 `Transfer` 自动重定向到真实服务器（旧版客户端回退到 **“连接已丢失（Connection Lost）”** 弹窗）。
- **共享 HTTPS 验证门户（默认 `:8443`）**：SSH 和 Minecraft 共用 `verify_port`（可用 `--verify-url https://verify.example.com:8443` 指定域名，留空则使用检测到的服务器 IP）。使用服务器 IP 走 ACME 时，Gymbag 会申请 **RFC 8738** IP 证书（如果 CA 支持则启用 `shortlived` profile）；若 HTTP-01 不可达，则回退到本地 CA 或自签名 IP 证书。当 `verify_port` 与 `https_port` 一致且配置了 HTTP 站点时，门户会直接复用 `443` 端口。

```caddyfile
{
    verify_url https://verify.example.com:8443
    verify_port 8443
}

ssh {
    port 22
    upstream 127.0.0.1:41822
    verify 3
    rate_limit 60 60s
}

minecraft {
    port 25565
    upstream 127.0.0.1:48291
    server_properties /srv/mc/server.properties
    verify 3
    allow ip 192.168.1.0/24
}
```

完整说明与 `gymbag.json` 示例请参阅 [SSH 与 Minecraft 防护指南](docs/zh/guide/ssh-minecraft.md)。

## 本地开发（`localhost` 与 `*.internal`）

只要站点域名属于 `localhost`、`*.localhost`、`*.internal`（如 `app.internal`、`api.internal`）、`*.local` 或 `*.test`，Gymbag 就会自动接管 `/etc/hosts` 和本地 TLS 证书：

```caddyfile
app.internal {
    reverse_proxy http://localhost:3000
    verify 3
}

api.internal {
    reverse_proxy http://localhost:8080
    protect off
}
```

### 自动管理 `/etc/hosts`

在默认 `auto_hosts on` 下，Gymbag 启动时会检查 `/etc/hosts`（Windows 为 `C:\Windows\System32\drivers\etc\hosts`），为尚未解析的本地域名写入 IPv4 和 IPv6 回环条目：

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

既有的 `localhost` 条目保持不动。收到 `Ctrl+C` 或 `SIGTERM` 时，Gymbag 会重新读取该文件（保留你在此期间做的手动修改），仅删除带自身 `# gymbag-managed:<instanceID>` 标记的行。在 macOS/Linux 上写入 `/etc/hosts` 需要 root 权限；若以普通用户运行，Gymbag 会记录警告并继续运行。如果你通过 `dnsmasq` 或手动维护 `.internal` 解析，设置 `auto_hosts off`（或 `GYMBAG_AUTO_HOSTS=off`）即可关闭。

### 本地 CA

Gymbag 不使用每次重启都作废的临时自签证书，而是在 `<cert_dir>/ca/`（默认为 `certs/ca/`）下维护一套双层 ECDSA P-256 CA：

1. **Root CA**：`"Neuro-sama Headquarters Root CA"`（`certs/ca/neuro-sama-root-ca.pem`，有效期 10 年，`MaxPathLen: 1`）
2. **Intermediate CA**：`"Gymbag & Swarm SSL Local CA"`（`certs/ca/gymbag-swarm-local-ca.pem`，有效期 5 年，`MaxPathLen: 0`）
3. **Leaf Certificate**：`certs/local-fullchain.pem` + `certs/local-privkey.pem`（有效期 90 天，剩余不足 30 天续期，包含 DNS SAN 以及 `127.0.0.1` 和 `::1`）

只需将 `"Neuro-sama Headquarters Root CA"` 导入一次系统信任库，浏览器和 `curl` 就会认可所有本地站点：

```bash
# macOS
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain certs/ca/neuro-sama-root-ca.pem

# Debian / Ubuntu
sudo cp certs/ca/neuro-sama-root-ca.pem /usr/local/share/ca-certificates/neuro-sama-root-ca.crt && sudo update-ca-certificates

# Fedora / RHEL / Arch Linux
sudo trust anchor --store certs/ca/neuro-sama-root-ca.pem
```

> **Firefox 提示：** 在 macOS/Windows（以及启用 `p11-kit` 的 Linux）中，可在 `about:config` 将 `security.enterprise_roots.enabled` 设为 `true`；也可以在 *Settings -> Privacy & Security -> Certificates -> View Certificates -> Authorities* 中手动导入 `certs/ca/neuro-sama-root-ca.pem`。

## 多站点与验证级别（`0`–`3`）

单进程可同时托管多个域名，支持上游轮询负载均衡和 RFC 6455 WebSocket 升级。每个站点（以及 SSH/Minecraft 服务）均可设置验证级别（`0`–`3`），或用 `protect off` 关闭 PoW：

| 级别 | 名称 | 触发质询的对象 | 适用场景 |
| :---: | :--- | :--- | :--- |
| `0` | `always` | 所有未持有有效通行 Cookie 或未放行 IP 的访客 | 私有管理后台、预发环境、L7 洪水攻击缓解 |
| `1` | `only suspicious` | 威胁情报、实时不良信誉 IP、VPN/代理/Tor IP、扫描器、CLI 工具库、无头浏览器、缺失/伪造请求头、漏洞探测 | 正常浏览器不应弹质询的公开应用与 API |
| `2` | `only ai` | 官方 AI 爬虫 IP 段，或带 AI 机器人 `User-Agent` / `X-Ai-Agent` / `Sec-Agent` 请求头的客户端 | 人类和 `curl` 可直接访问的文档、博客和 Git 仓库 |
| `3` | `suspicious & ai` | 组合 `1` 和 `2`（不带参数写 `verify` 以及 SSH、Minecraft 的默认值） | 公开生产站点 |
| n/a | `protect off` | 不触发 PoW 质询，纯反向代理（显式配置的自定义规则仍会执行） | 静态资源站、Webhook、镜像站 |

```caddyfile
app.example.com, www.example.com {
    reverse_proxy http://localhost:3000 http://localhost:3001
    verify 3
    difficulty 4
}

docs.example.com {
    reverse_proxy http://localhost:4000
    verify 2
}

api.example.com {
    reverse_proxy http://localhost:8080
    verify 1
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
    verify_timeout 30m
}

static.example.com {
    reverse_proxy http://localhost:9000
    protect off
}
```

通过 SHA-256 质询（`difficulty` `1`–`5`，默认 `4`）后，系统会下发绑定主机名和客户端 IP 的 `HttpOnly`、`SameSite=Lax` HMAC 通行 Cookie（`waf_clearance`，默认有效期 `24h` / `86400s`）。可通过 `verify_timeout` / `clearance_ttl`（支持整数秒或 `30m`、`12h`、`1h30m`、`7d`、`2w` 等时长格式）、`--verify-timeout`、`GYMBAG_VERIFY_TIMEOUT` 或 `PATCH /api/v1/sites/{domain}` 在全局或单站点覆盖该时长。已验证访客访问 `GET /.waf/v1/session` 会返回 `clearance_ttl`、`expires_at` 和 `expires_in`。

## 路径路由、静态文件、重定向与配置导入

除了 `reverse_proxy`，站点内还可以定义有序的**路径路由**。按从上到下第一条匹配的路径处理请求；未匹配到的路径会回落到站点默认上游（若未配置上游则返回 `404`）。每条路由对应一种处理器：反向代理、静态文件服务（`root`）、固定响应 `respond` 或重定向 `redir`。

```caddyfile
app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    handle_path /static/* {
        root ./dist
        file_server browse
    }
    @health path /healthz /readyz
    respond @health "ok" 200
    redir /old/* https://{host}/new{uri} 301
    handle /api/* {
        reverse_proxy http://localhost:8080
        header X-Served-By gymbag
    }
    try_files {path} /index.html
}
```

路径匹配器支持精确路径（`/exact`）、通配符（`/api/*`、`*.php`）以及 Go RE2 正则（`~^/v[0-9]+/`）。重定向支持展开 `{scheme}`、`{host}`、`{uri}`、`{path}` 和 `{query}`。静态路由支持 `index`、`browse` 以及 SPA `fallback`。`header` 用于设置响应头，前缀 `-` 则删除对应响应头。防火墙规则和 PoW 验证会在所有路由处理器之前执行。

`gymbag.json` 中对应的 `routes` 数组写法如下：

```json
{
  "routes": [
    {"path": "/static/*", "strip_prefix": "/static", "root": "./dist", "browse": true},
    {"path": "/old/*", "redirect": {"to": "https://{host}/new{uri}", "code": 301}},
    {"path": "/api/*", "upstreams": ["http://localhost:8080"]}
  ]
}
```

可使用 `gymbag config route list|add|update|remove` 或 `PATCH /api/v1/sites/{domain}` 管理路由。

**导入既有配置：** `gymbag run -c /etc/nginx/nginx.conf`（或 `httpd.conf` / `apache2.conf`）可直接加载原生 nginx 或 Apache 配置运行；`gymbag config convert -c <file> -o gymbag.json --format json` 支持转换 nginx、Apache 和 Caddyfile v2（包含 snippets、`import`、`@matchers`、`handle`、`route`、`file_server`、`respond`、`redir` 与 `header`）。

## SSL 提供商、自定义密钥与续期

公网域名默认使用 `tls auto`，通过 ACME HTTP-01 向 Let's Encrypt 申请证书。可通过 `ssl_provider`（别名：`acme_provider`、`ca`、`cert_issuer`）或 `tls <provider>` 切换 CA：

| 提供商关键字 | 证书颁发机构 | EAB 要求 |
| :--- | :--- | :--- |
| `letsencrypt` (`le`) | Let's Encrypt（生产环境） | 无 |
| `letsencrypt_staging` | Let's Encrypt（测试环境） | 无 |
| `zerossl` | ZeroSSL | 设置 `email` 后自动获取，或显式配置 `eab <kid> <hmac_key>` |
| `google` (`gts`) | Google Trust Services | 需要 `eab <kid> <hmac_key>` |
| `ssl.com` (`sslcom`, `ssl.com-rsa`) | SSL.com（ECC 或 RSA） | 需要 `eab <kid> <hmac_key>` |
| `litessl` (`trustasia`) | LiteSSL by TrustAsia | 标准 DV 无需 |
| `buypass` | Buypass Go SSL | 无 |
| `local` | 本地双层 PKI | 本地 CA（`Neuro-sama Headquarters Root CA`） |
| `https://...` | 任意 RFC 8555 ACME v2 CA | 可选 `eab <kid> <hmac_key>`（如 `step-ca`、Pebble） |

选择 **ZeroSSL** 并配置 `email` 时，除非手动传入 `eab`，否则 Gymbag 会自动拉取 EAB 凭据。使用 **Google Trust Services** 和 **SSL.com** 时需传入 `eab YOUR_EAB_KEY_ID YOUR_EAB_HMAC_KEY`（支持标准填充 base64 和无填充 base64url，Admin API 输出中会自动脱敏 `eab_hmac_key`）。ACME 证书按提供商分别缓存在磁盘（如 `certs/acme-zerossl-fullchain.pem`，Let's Encrypt 为 `certs/acme-fullchain.pem`），切换 `ssl_provider` 后会立即向新 CA 申请。

也可以传入自有 PEM 文件：
- `account_key /etc/gymbag/account.pem`：既有 ECDSA（P-256/P-384）或 RSA ACME 账户密钥。
- `ssl_key /etc/gymbag/domain.key`：既有 ECDSA 或 RSA 密钥，用于签发 ACME CSR。
- `tls <cert> <key>`：全局或按站点（SNI）使用外部管理的证书；若私钥带密码，配置 `key_password`（`GYMBAG_KEY_PASSWORD`）。

后台任务每隔 `renew_interval` 秒（默认 `43200` / 12h）巡检证书，距过期不足 `renew_within_days`（`30` 天）时自动续期或重载，并原子替换 `tls.Certificate`，不中断既有 HTTP 流或 WebSocket 连接。

## 编写自定义防火墙规则

受保护站点上的 HTTP 请求按以下顺序逐层检查：

1. 请求基础合法性检查（HTTP 方法、最大 URI 长度、拦截空字节和 `..` / `%2e%2e`）
2. ACME HTTP-01（`/.well-known/acme-challenge/*`）
3. 本地自定义规则（先全局规则，再站点规则，从上到下，首条命中生效）
4. 远程规则（`remote_rules`，通过 HTTP/HTTPS 自动刷新）
5. 内置基线规则（除非配置 `default_rules off`）
6. 按站点按 IP 限流器（`rate_limit` / `rate_window`）
7. IP 威胁情报（`ip_security_action verify` 或 `block`）
8. 工作量证明检查（`verify` `0`–`3`）

自定义规则排在基线规则、限流和威胁情报之前，因此在站点块顶部加一条 `allow` 就能直接放行办公网 CIDR、健康检查或带预览 Cookie 的请求。

### 单行简写规则

语法：`<allow | verify | block> <field> <pattern...>`（同时支持别名 `bypass`/`pass`、`challenge`/`pow`、`deny`/`drop`/`reject`）：

| 字段 | 值与匹配方式 | 示例 |
| :--- | :--- | :--- |
| `ip` | IPv4、IPv6 或 CIDR | `allow ip 127.0.0.1 10.0.0.0/8 2001:db8::/32` |
| `user_agent` | 忽略大小写子串、`*` 通配符或 `~regex` | `block user_agent BadBot ~^(?i)python-urllib/` |
| `path` | 精确/子串、`*` 通配符或 `~regex` | `verify path /api/export* ~^/v[0-9]+/search` |
| `query` | 原始/解码查询匹配、`*` 通配符或 `~regex` | `block query "union select" ~(?i)<script` |
| `method` | HTTP 方法（忽略大小写） | `verify method POST DELETE` |
| `header` | `<Header-Name> [pattern]`（`""` = 缺失/为空，`*` = 存在） | `verify header Accept-Language ""` |
| `cookie` | `<cookie_name> [pattern]`（`""` = 缺失/为空，`*` = 存在） | `allow cookie vip_session *` |

### 多条件 `rule` 块

在 `rule "name" { ... }` 块内，同一字段的多个值之间是 OR 关系，不同字段之间是 AND 关系：

```caddyfile
app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    rule "challenge-auth-mutations" {
        action verify
        method POST
        path /login* /api/auth/* /reset-password*
    }

    rule "block-headless-api-posts" {
        action block
        method POST
        path /api/*
        header Origin ""
    }
}
```

### 远程规则与动态本地规则文件

可在全局或按站点配置 `remote_rules`，指向提供 Gymbagfile 规则语法或 JSON（`[ ... ]` 或 `{"rules": [ ... ]}`）的 HTTP/HTTPS URL。校验通过的规则集缓存在 `<cert_dir>/rules-cache` 下，通过 `ETag` / `If-Modified-Since` 进行条件请求；拉取或解析出错时会继续保留上一份可用规则。

本地 `.rules` 或 `.json` 文件与目录也可通过 `rules_file`、`rules_dir` 或 `ruleset` 块（或 `gymbag ruleset add`）加载。加上 `dynamic` 标记后，每 `rules_watch_interval`（默认 `2s`）轮询变更并热重载：

```caddyfile
{
    remote_rules https://rules.example.com/org-baseline.rules
    remote_rules_refresh 5m
    rules_watch_interval 2s
}

app.example.com {
    reverse_proxy http://localhost:3000
    rules_file ./rules/custom.rules dynamic
    rules_dir ./rules.d dynamic
}
```

`default_rules` 默认开启（`on`）：为站点添加自定义规则后，自定义规则会优先执行，内置 **block** 规则仍会保留在后面，避免只写了一条 `verify path` 就意外放开 `/.env`。如需完全关闭内置规则，设置 `default_rules off`。

## CLI、Docker Compose、环境变量与 REST API

### CLI 参考

若工作目录存在 `.env`，Gymbag 会先加载它，再依次探测 `./gymbag.json`、`./Gymbagfile` 和 `./Caddyfile`（也可通过 `-c` 指定 JSON、Gymbagfile、Caddyfile v2、`nginx.conf` 或 Apache 配置）：

```bash
# 生成配置（默认写入 gymbag.json，传 --format gymbagfile 则写入 Gymbagfile）
./gymbag init
./gymbag init -d app.example.com -u http://localhost:3000 -l 3 --rules-dir ./rules.d --rules-dynamic
./gymbag init -t ssh --verify-url https://verify.example.com:8443
./gymbag init -t minecraft --server-properties ./server.properties
./gymbag init -t http,ssh,minecraft -d app.example.com -u :3000 --server-properties ./server.properties
./gymbag init --format gymbagfile -o Gymbagfile -d app.example.com -u :3000

# 管理站点、路由、全局设置并转换配置
./gymbag config list
./gymbag config add -d api.example.com -u :8080 -l 1
./gymbag config update -d api.example.com -l 3 --verify-timeout 6h
./gymbag config route add -d api.example.com --path /static/* --strip-prefix /static --root ./dist
./gymbag config remove -d api.example.com
./gymbag config set admin 127.0.0.1:2019
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json

# 管理规则文件、目录及单条规则
./gymbag ruleset init ./rules/custom.rules
./gymbag ruleset add --source ./rules --dynamic --name local-dir
./gymbag ruleset update local-dir --static
./gymbag ruleset remove local-dir
./gymbag ruleset validate ./rules
./gymbag ruleset rule add --file ./rules/custom.rules --action block --name bad-bot --ua "*BadBot*"

# 运行、监听热重载、校验、格式化及系统服务管理
./gymbag
./gymbag run --config gymbag.json --watch
./gymbag service install --config gymbag.json
./gymbag service start
./gymbag service render --type systemd
./gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3
./gymbag validate --config gymbag.json
./gymbag fmt --overwrite Gymbagfile
./gymbag version
```

### Docker 与 Docker Compose

多架构 Alpine 镜像（`linux/amd64`、`linux/arm64`）发布在 `ghcr.io/evil-project/gymbag`，容器内以 UID `10001` 运行并附带 `cap_net_bind_service=+ep`。

```bash
# 挂载 Gymbagfile 快速运行
docker run -d \
  --name gymbag \
  --restart unless-stopped \
  -p 80:80 -p 443:443 \
  --cap-add NET_BIND_SERVICE \
  --add-host=host.docker.internal:host-gateway \
  -v "$(pwd)/Gymbagfile:/app/Gymbagfile:ro" \
  -v gymbag-certs:/app/certs \
  ghcr.io/evil-project/gymbag:latest
```

`compose.yaml` 将 Gymbag 与 `redis:8-alpine` 组合在一起，通过内置的纯 Go RESP2 Redis 客户端在重启或多副本之间共享 PoW nonce、固定窗口限流计数以及 SSH/Minecraft 验证凭证和 IP 放行记录（`docker compose up -d --build`）。在 Docker 中防护 SSH 或 Minecraft 时，需同时映射 `-p 22:22` / `-p 25565:25565` 和 `-p 8443:8443`。宿主机挂载权限（`chown 10001:10001`）、`--watch` 以及纯环境变量启动方式详见 [Docker 与 Docker Compose 指南](docs/zh/guide/docker.md)。

### 环境变量（`GYMBAG_*`）

优先级顺序：**配置文件 -> `.env` / `GYMBAG_*` 环境变量 -> CLI 参数**。

| 变量 | 说明 |
| :--- | :--- |
| `GYMBAG_CONFIG` | `gymbag.json`、`Gymbagfile`、`Caddyfile`、`nginx.conf` 或 Apache 配置文件路径 |
| `GYMBAG_BIND` | 绑定 IP（仅本地 HTTP 时默认 `127.0.0.1`；启用公网域名、SSH 或 Minecraft 时为 `0.0.0.0`） |
| `GYMBAG_HTTP_PORT` / `GYMBAG_HTTPS_PORT` | HTTP 与 HTTPS 监听端口（默认 `80` 和 `443`） |
| `GYMBAG_DOMAIN` / `GYMBAG_DOMAINS` | 逗号分隔的下游域名 |
| `GYMBAG_UPSTREAM` | 默认上游目标 URL |
| `GYMBAG_UPSTREAMS` | 分号分隔的域名映射：`app.example.com=http://web:3000;api.example.com=http://api:8080` |
| `GYMBAG_PROTECT` / `GYMBAG_PROTECT_DOMAINS` | 启停 WAF 防护（`on`/`off`），或仅对列出的域名启用防护 |
| `GYMBAG_VERIFY_LEVEL` | 验证级别 `0`–`3` 或 `off`（别名：`GYMBAG_VERIFY`、`GYMBAG_VERIFY_MODE`、`GYMBAG_MODE`、`GYMBAG_TRIGGER`） |
| `GYMBAG_DEFAULT_RULES` | 内置基线规则（`on` / `off`，默认 `on`） |
| `GYMBAG_REMOTE_RULES` / `GYMBAG_REMOTE_RULES_REFRESH` | 远程防火墙规则 URL 及刷新间隔（默认 `300`s / `5m`） |
| `GYMBAG_RULES_WATCH_INTERVAL` | `dynamic` 本地规则文件/目录轮询间隔（默认 `2`s / `2s`） |
| `GYMBAG_DIFFICULTY` | SHA-256 PoW 难度（`1`–`5`，默认 `4`） |
| `GYMBAG_CHALLENGE_TTL` / `GYMBAG_CLEARANCE_TTL` | 质询有效期（`300`s / `5m`）与通行有效期（`86400`s / `24h`；别名：`GYMBAG_VERIFY_TIMEOUT`、`GYMBAG_VERIFIED_TIMEOUT`） |
| `GYMBAG_COOKIE_NAME` / `GYMBAG_COOKIE_SECURE` | 通行 Cookie 名称（`waf_clearance`）与 `Secure` 标志（`true`） |
| `GYMBAG_RATE_LIMIT` / `GYMBAG_RATE_WINDOW` | 单 IP 请求上限（`240`）与窗口时长（`60`s） |
| `GYMBAG_TLS_MODE` / `GYMBAG_SSL_PROVIDER` | TLS 模式（`auto`、`acme`、`local`、`selfsigned`、`file`）与 CA（`letsencrypt`、`letsencrypt_staging`、`zerossl`、`google`、`ssl.com`、`litessl`、`buypass`、`local` 或 URL） |
| `GYMBAG_ACME_EMAIL` / `GYMBAG_ACME_DIRECTORY` | ACME 联系邮箱（启用 ZeroSSL 自动 EAB）及显式 ACME 目录 URL |
| `GYMBAG_EAB_KID` / `GYMBAG_EAB_HMAC_KEY` | 外部账户绑定 Key ID 与 base64/base64url HMAC 密钥 |
| `GYMBAG_ACCOUNT_KEY_FILE` | ECDSA/RSA ACME 账户密钥 PEM 路径 |
| `GYMBAG_CERT_FILE` / `GYMBAG_KEY_FILE` / `GYMBAG_KEY_PASSWORD` | `file` 模式下使用的证书、私钥及加密私钥口令 |
| `GYMBAG_CERT_DIR` | 证书、CA 密钥、密钥、情报缓存与规则缓存目录（默认 `certs`） |
| `GYMBAG_RENEW_INTERVAL` / `GYMBAG_RENEW_WITHIN_DAYS` | 续期巡检间隔（`43200`s）与提前续期天数（`30` 天） |
| `GYMBAG_AUTO_HOSTS` / `GYMBAG_HSTS` | 自动维护 `/etc/hosts`（`on`/`off`）与覆盖 `Strict-Transport-Security`（`on`/`off`） |
| `GYMBAG_IP_SECURITY` / `GYMBAG_IP_SECURITY_ACTION` | 威胁情报源开关（`on`/`off`）与命中动作（`verify` 或 `block`） |
| `GYMBAG_IP_REPUTATION` / `GYMBAG_AI_IP_LIST` | Shodan InternetDB + StopForumSpam 实时查询（`on`/`off`）与官方 AI 爬虫 CIDR 列表（`on`/`off`） |
| `GYMBAG_VPN_DETECTION` | 可疑流量判定中启用免费 VPN/代理/Tor 检测（`on`/`off`，默认 `on`） |
| `GYMBAG_INTEL_REFRESH` | 威胁、VPN 与 AI 情报刷新间隔（默认 `21600`s / `6h`） |
| `GYMBAG_VERIFY_URL` / `GYMBAG_VERIFY_PORT` | 共享 HTTPS 验证门户 URL（留空则使用服务器 IP + IP 证书）与端口（默认 `8443`） |
| `GYMBAG_SSH_ENABLED` / `GYMBAG_SSH_PORT` / `GYMBAG_SSH_UPSTREAM` | 启用 SSH 防护（默认端口 `22`）及后端 `sshd` 地址 |
| `GYMBAG_SSH_VERIFY_LEVEL` / `GYMBAG_SSH_PROTECT` | SSH 验证级别（`0`–`3` 或 `off`）与防护开关（`on`/`off`） |
| `GYMBAG_MC_ENABLED` / `GYMBAG_MC_PORT` / `GYMBAG_MC_UPSTREAM` | 启用 Minecraft 防护（默认端口 `25565`）及后端目标（支持 `GYMBAG_MINECRAFT_*` 别名） |
| `GYMBAG_MC_SERVER_JAR` / `GYMBAG_MC_JAVA` | Minecraft `server.jar` 路径（自动选择 Java 并拉起服务端）以及可选的 Java 可执行文件覆盖路径 |
| `GYMBAG_MC_JVM_ARGS` / `GYMBAG_MC_WORK_DIR` / `GYMBAG_MC_AUTO_RESTART` | 空格分隔的 JVM 参数、Minecraft 工作目录以及崩溃自动重启开关（`true`/`false`，默认 `true`） |
| `GYMBAG_MC_VERIFY_LEVEL` / `GYMBAG_MC_PROTECT` | Minecraft 验证级别（`0`–`3` 或 `off`）与防护开关（`on`/`off`） |
| `GYMBAG_REDIS_URL` / `GYMBAG_SECRET` / `GYMBAG_ADMIN` | Redis URL（`redis://` 或 `rediss://`）、HMAC 密钥与 Admin API 监听地址（如 `127.0.0.1:2019` 或 `off`） |

### REST API

**公共 WAF 与验证门户接口：**

| 方法 | 路径 | 说明 |
| :--- | :--- | :--- |
| `GET` | `/.waf/v1/session` | 通行状态（`cleared`）、`protected`、`verify_level`、`verify_mode`、`clearance_ttl`、`expires_at`、`expires_in` |
| `DELETE` | `/.waf/v1/session` | 清除访客通行 Cookie（`204 No Content`） |
| `POST` | `/.waf/v1/challenges` | 签发一次性 SHA-256 PoW 质询（`201 Created`） |
| `POST` | `/.waf/v1/verifications` | 校验 PoW 解并下发签名通行 Cookie（兼容 `/.waf/verify` 别名） |
| `GET` | `/v/{token}` | 返回 SSH/Minecraft 验证 PoW 页面（兼容 `/verify/{token}` 别名） |
| `POST` | `/.waf/gate/verify` | 校验 SSH/Minecraft PoW 解并写入 IP 放行记录 |
| `GET` | `/.waf/assets/{name}` | 提供内嵌质询 PNG 图片资源（支持 `/.waf/v1/assets/{name}` 别名及 `ETag` 缓存） |

**本地管理 API（`admin 127.0.0.1:2019` 或 `--admin`）：**

请务必将 Admin API 绑定在回环地址（`127.0.0.1:2019`）或私有管理网卡上——该接口无需鉴权令牌，且会直接修改运行中的路由和防火墙状态。

| 方法 | 路径 | 说明 |
| :--- | :--- | :--- |
| `GET` | `/api/v1/health` | 状态、`sites_count`、`ssh_enabled`、`minecraft_enabled`、`tls_source`、`threat_entries`、`ai_entries`、`vpn_entries`、`remote_rules` |
| `GET` / `POST` | `/api/v1/tls` / `/api/v1/tls/renew` | 当前 TLS 状态与 CA 目录；强制续期证书并原子切换 |
| `GET` / `POST` | `/api/v1/intel` / `/api/v1/intel/refresh` | 各威胁/AI/VPN 源条目数与缓存大小；立即触发刷新 |
| `GET` / `POST` | `/api/v1/rules/remote` / `/api/v1/rules/remote/refresh` | 远程规则条数与拉取错误；立即刷新并重载 WAF |
| `GET` / `PUT` | `/api/v1/config` | 导出脱敏后的配置；校验并替换运行中配置 |
| `GET` / `POST` | `/api/v1/sites` | 列出站点或运行时新增站点 |
| `GET` / `PUT` / `PATCH` / `DELETE` | `/api/v1/sites/{domain}` | 查看、替换、局部热更新（`verify_level`、`protect`、`difficulty`、`clearance_ttl`、`upstreams`、`routes`、`headers`、`rules`、`remote_rules`、`rule_sources`）或删除站点 |
| `GET` / `POST` / `PUT` / `DELETE` | `/api/v1/sites/{domain}/rules` 与 `.../{index_or_name}` | 列出、追加、替换或删除站点自定义规则 |

## 与 Caddy、Nginx 的性能基准对比

测试环境为 Apple M4（10 核，macOS，Go `1.27.0`），使用同一张 ECDSA P-256 证书终结 TLS 1.3，64 并发，`1 KiB` 响应体，`1024` 上游 keep-alive 连接，全程 `0` 错误：

| 服务器与模式 | HTTP/1.1 TLS (RPS) | H1 p50 / p99 | HTTP/2 TLS (RPS) | H2 p50 / p99 | 二进制体积 | 负载下 RSS |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **85,203** | 0.45 / 6.20 ms | **88,775** | 0.65 / 1.96 ms | 1.5 MB (+libs) | 5.8 MiB |
| **Gymbag `v0.1.0`** (`verify 3`，全量 WAF 开启) | **72,517** | 0.74 / 3.14 ms | **66,096** | 0.82 / 3.48 ms | 12.9 MB (静态) | 54.3 MiB |
| **Gymbag `v0.1.0`** (`protect off`，纯代理) | **72,479** | 0.74 / 3.15 ms | **64,048** | 0.83 / 3.94 ms | 12.9 MB (静态) | 52.1 MiB |
| **Gymbag `v0.1.0`** (`verify 0`，已放行 Cookie) | **65,490** | 0.79 / 3.93 ms | **62,108** | 0.84 / 4.27 ms | 12.9 MB (静态) | ~54 MiB |
| **Caddy `v2.11.9`** (Go，keep-alive 调优) | **54,741** | 0.90 / 4.90 ms | **39,207** | 1.30 / 6.18 ms | 49.0 MB | 76.8 MiB |

C 语言编写的 Nginx 在极限吞吐上仍占优势，但 Gymbag 在在线运行 WAF 的同时，HTTP/1.1 p99 延迟更低（`3.14 ms` vs `6.20 ms`），且吞吐超过 Caddy（HTTP/1.1 高 `+32%`，HTTP/2 高 `+63%` 到 `+69%`），内存少约 30%，二进制体积小 3.8 倍。对于 `/.env` 这类已知探测，Gymbag 在 TLS 边缘以 **140,680 req/s**（`0.35 ms` p50）直接丢弃，不消耗上游资源。

测试方法、`go test -bench` 微基准数据及完整配置请参阅 [基准测试与性能](docs/zh/guide/benchmarks.md)。

## 项目结构

```text
.
├── cmd/gymbag/          # CLI 入口
├── docs/                # 多语言 VitePress 文档（EN, 简体中文, 日本語, 한국어, Español）
├── internal/
│   ├── cli/             # 子命令（init, config, ruleset, service, run, reverse-proxy, validate, fmt, version）、.env 与 --watch
│   ├── config/          # 配置结构、Gymbagfile/Caddyfile v2/nginx/Apache 解析器、时长与规则来源、GYMBAG_* 加载器
│   ├── host/            # 域名归一化、IPv6 方括号、通配符匹配
│   ├── intel/           # O(1) 前缀集合、自动更新威胁/VPN/AI 源、实时 IP 与 VPN 信誉查询
│   ├── localpki/        # 双层本地 CA（"Neuro-sama Headquarters Root CA"）与 /etc/hosts 管理器
│   ├── proxy/           # 无锁多站点与路径路由器、连接池化 HTTP 与 RFC 6455 WebSocket 代理
│   ├── server/          # 融合 HTTPS 网关、HTTP->HTTPS/ACME 重定向器、Admin REST API
│   ├── service/         # 系统服务渲染与管理器（systemd, launchd, openrc, supervisor）
│   ├── setup/           # 自动配置 sshd_config/防火墙与 Minecraft server.properties 端口随机化
│   ├── store/           # 64 分片内存存储、纯 Go RESP2 Redis 客户端（质询、限流、门户凭证/放行记录）
│   ├── tcpguard/        # SSH 与 Minecraft Java TCP 防护代理、策略引擎、共享 HTTPS 验证门户
│   ├── tlsmanager/      # 多 CA ACME v2（RFC 8738 IP 证书、EAB 与 ZeroSSL 自动 EAB）、本地 PKI、原子 SNI 证书管理
│   └── waf/             # PoW 校验器、分片 HMAC 放行缓存、Aho-Corasick DFA、远程/本地规则、内嵌 UI
├── nix/                 # NixOS 模块（services.gymbag）
├── packaging/           # Arch Linux PKGBUILD 与 Debian/Ubuntu .deb 打包脚本/unit
├── Dockerfile
├── compose.yaml
├── flake.nix
├── Gymbagfile
└── go.mod
```

## Star 趋势

<a href="https://www.star-history.com/#Evil-Project/gymbag&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
 </picture>
</a>
