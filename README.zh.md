# Gymbag

<p align="center">
  <img src="docs/public/mascot-happy.png" alt="Gymbag mascot" width="140" />
</p>

<p align="center">
  <strong>纯 Go 编写的单二进制 HTTPS 反向代理与工作量证明（PoW）WAF</strong>
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

Gymbag 是一个内置类 Anubis 工作量证明（Proof-of-Work）Web 应用防火墙的单二进制 HTTPS 反向代理。它完全基于纯 Go 标准库开发 —— **零外部 Go 依赖**，所有 HTML 模板、浏览器 SHA-256 计算脚本和看板娘 PNG 素材均通过 `go:embed` 直接编译进可执行文件。

如果你自建了 Web 应用、Gitea/Forgejo 实例、文档站或内部工具，你很可能遇到过与 Anubis 相同的问题：无视 `robots.txt` 的激进大模型爬虫，以及全天候扫描 `/.env` 和 `/wp-admin` 的漏洞探测器。与其在 Nginx/Caddy、独立 PoW Sidecar 容器、fail2ban 和手工整理的 CIDR 列表之间来回拼凑，不如用 Gymbag 将 TLS 终结、多 CA ACME、HTTP/WebSocket 代理、本地与远程防火墙规则、IP 威胁情报和按站点配置的 PoW 策略集成进一个由简洁 `Gymbagfile` 驱动的二进制文件中。

---

## 快速开始（开箱即用的安全防护）

你可以从 [GitHub Releases](https://github.com/Evil-Project/gymbag/releases) 下载预编译静态二进制文件（支持 Linux/macOS/Windows，以及 Arch Linux 的 `.pkg.tar.zst`）、通过 Nix Flakes 安装，或者从源码编译：

```bash
go build -o gymbag ./cmd/gymbag
```

**Arch Linux:**
```bash
# 安装预编译发行包（.pkg.tar.zst）
sudo pacman -U gymbag-0.1.0-1-x86_64.pkg.tar.zst
# 或使用 makepkg 本地构建
cd packaging/arch && makepkg -si
```

**Nix / NixOS:**
```bash
# 通过 flake 运行或安装
nix run github:Evil-Project/gymbag -- --help
nix profile install github:Evil-Project/gymbag
```
静态编译的 `CGO_ENABLED=0` Linux 发行版无需 `patchelf` 或 `nix-ld` 即可在 NixOS 上直接运行，`nixosModules.default` 也提供了声明式的 `services.gymbag` 配置。

**两行命令**即可生成 `Gymbagfile` 并启动服务：

```bash
# 1. 生成配置（交互式向导，或通过参数一键完成配置）
./gymbag init -d app.example.com -u http://localhost:3000

# 2. 启动服务器
./gymbag
```

或者注册为系统服务（`systemd`、`launchd`、`openrc` 或 `supervisor`）并启动：

```bash
./gymbag init -d app.example.com -u :3000 --service auto
./gymbag service start
```

也可以跳过配置文件直接启动临时反向代理：

```bash
./gymbag reverse-proxy --from app.internal --to :3000 --verify 3
```

### “开箱即用”在实战中意味着什么

将 Gymbag 部署到公网前，你不需要手写一条正则表达式或手动导入任何黑名单。只要写上 `verify`（启用验证等级 `3`：`suspicious & ai`）或 `block`（对威胁情报命中 IP 直接返回 `403 Forbidden` 而非发起质询），即可立即激活三层防护：

1. **内置基线防火墙规则（`default_rules on`）**
   - **敏感文件防护（`403`）**：拦截对 `/.env`、`/.env.*`、`/.git/*`、`/.svn/*`、`/.hg/*`、`/.DS_Store`、`/.aws/*`、`/.ssh/*`、`/docker-compose.yml`、`/docker-compose.yaml`、`*.bak`、`*.sql` 和 `*.sqlite` 的请求。
   - **CMS 与框架漏洞探测（`403`）**：直接丢弃 `/wp-admin/*`、`/wp-login.php`、`/xmlrpc.php`、`/phpmyadmin*`、`/vendor/phpunit*`、`/cgi-bin/*`、`/actuator/env*` 和 `/actuator/heapdump*`。
   - **漏洞扫描器拦截（`403`）**：拒绝 `sqlmap`、`nikto`、`nuclei`、`masscan`、`zgrab`、`gobuster`、`dirbuster`、`ffuf`、`wfuzz`、`feroxbuster`、`acunetix`、`nessus` 和 `openvas`。
   - **注入与路径穿越检测（`403`）**：拦截查询字符串中的 `UNION SELECT`、`<script`、`javascript:`、`/etc/passwd`、`information_schema`、`../`、`cmd.exe` 和 `/bin/sh`（同时执行严格的请求格式校验，拒绝空字节和 `%2e%2e` 路径穿越）。
   - **自动化客户端质询（`PoW 质询`）**：当你尚未自定义规则时，CLI HTTP 客户端（`curl`、`wget`、`python-requests`、`python-httpx`、`go-http-client`、`scrapy`）、无头浏览器（`headlesschrome`、`playwright`、`puppeteer`、`selenium`）以及缺失 `User-Agent` 的请求都会进入工作量证明页面。
2. **自动更新的 IP 威胁情报**
   - 启动时及后台每 6 小时自动拉取 [IPsum (level 3+)](https://raw.githubusercontent.com/stamparm/ipsum/master/levels/3.txt)、[Spamhaus DROP IPv4](https://www.spamhaus.org/drop/drop_v4.json) & [IPv6](https://www.spamhaus.org/drop/dropv6.txt) 以及 [abuse.ch Feodo Tracker](https://feodotracker.abuse.ch/downloads/ipblocklist.txt)，并使用条件请求头 `ETag` / `If-Modified-Since` 将数据缓存在 `<cert_dir>/intel-cache`。
   - 集成免密钥的实时 IP 信誉查询（**Shodan InternetDB** `internetdb.shodan.io` 与 **StopForumSpam** `api.stopforumspam.org`），带内存 TTL 缓存与故障放行（fail-open）超时控制，确保上游接口故障不会阻塞正常流量。
3. **官方 AI 爬虫 IP 段追踪**
   - 实时追踪 **OpenAI**（`gptbot`、`searchbot`、`chatgpt-user`）、**Anthropic**（`claude.com/crawling/bots.json`）、**Perplexity**（`perplexitybot`、`perplexity-user`）、**Google**（`user-triggered-agents`、`user-triggered-fetchers-google`）和 **Apple**（`applebot`）公开的 CIDR 列表。即使爬虫将 `User-Agent` 伪装成桌面版 Chrome，其来源 IP 依然会被精准识别。

> **关于 80 与 443 端口的运维提示：** Gymbag 默认监听 `:80`（HTTP 到 HTTPS 重定向 + ACME HTTP-01 验证响应）与 `:443`（HTTPS 网关）。在 Linux 上可授予二进制文件 `CAP_NET_BIND_SERVICE` 权限，从而避免以 root 身份运行：
> ```bash
> sudo setcap 'cap_net_bind_service=+ep' ./gymbag
> ```
> 在本地无特权环境测试且不使用 capabilities 或 `sudo` 时，可传入 `--http-port 8080 --https-port 8443`（或在 `Gymbagfile` 中设置 `http_port` / `https_port`）。

---

## 本地开发（`localhost` 与 `*.internal`）

使用自定义域名进行本地 HTTPS 测试时，通常需要折腾 `mkcert`、手动编辑 `/etc/hosts` 以及忽略自签名证书警告。只要站点使用 `localhost`、`*.localhost`、`*.internal`（如 `app.internal` 或 `api.internal`）、`*.local` 或 `*.test`，Gymbag 即可开箱自动处理域名解析与本地 X.509 证书签发：

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

### 自动管理与清理 `/etc/hosts`

当启用 `auto_hosts`（默认开启）时，Gymbag 在启动时会检查 `/etc/hosts`（Windows 下为 `C:\Windows\System32\drivers\etc\hosts`）。对于任何尚未在 `/etc/hosts` 中解析的具体本地域名（如 `app.internal` 或 `api.internal`），Gymbag 会自动追加带实例 ID 标记的 IPv4 与 IPv6 回环记录：

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

已存在的 `localhost` 条目会保持原样。当你停止 Gymbag（`Ctrl+C` / `SIGTERM`）时，它会重新读取 `/etc/hosts`（保留你在运行期间做的其他修改），并**仅移除**属于当前实例的 `# gymbag-managed:<instanceID>` 记录。

- **macOS 与 Linux 权限说明：** 修改 `/etc/hosts` 需要 root 权限。如果你以普通用户身份运行 `./gymbag` 且没有 `/etc/hosts` 写权限，Gymbag 会输出清晰的警告日志并**继续正常运行** —— 不会崩溃。需要自动管理 `/etc/hosts`（以及绑定 `80`/`443` 端口）时请使用 `sudo ./gymbag`；如果你使用 `dnsmasq` 解析 `.internal` 或自行维护 `/etc/hosts`，可以设置 `auto_hosts off`（或 `GYMBAG_AUTO_HOSTS=off`）。

### 双层本地 PKI 与信任根证书（Root CA）

不同于浏览器拒绝缓存信任的临时自签名叶子证书，Gymbag 在 `<cert_dir>/ca/`（默认为 `certs/ca/`）下维护了一套完整的双层 ECDSA P-256 证书颁发机构（CA）：

1. **根证书（Root CA）**：**`"Neuro-sama Headquarters Root CA"`**（`certs/ca/neuro-sama-root-ca.pem`，有效期 10 年，`MaxPathLen: 1`）
2. **中间证书（Intermediate CA）**：**`"Gymbag & Swarm SSL Local CA"`**（`certs/ca/gymbag-swarm-local-ca.pem`，有效期 5 年，由 `"Neuro-sama Headquarters Root CA"` 签发，`MaxPathLen: 0`）
3. **叶子证书（Leaf Certificate）**：`certs/local-fullchain.pem` + `certs/local-privkey.pem`（有效期 90 天，剩余不足 30 天时自动续期，包含完整证书链、DNS SAN 以及 `127.0.0.1` / `::1` IP SAN）

只需在操作系统信任库中信任一次 **`"Neuro-sama Headquarters Root CA"`**，Gymbag 提供的所有 `https://localhost` 与 `https://*.internal` 域名都会在浏览器和 `curl` 中显示绿色安全锁：

**macOS（系统钥匙串）：**
```bash
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain certs/ca/neuro-sama-root-ca.pem
```

**Debian / Ubuntu:**
```bash
sudo cp certs/ca/neuro-sama-root-ca.pem /usr/local/share/ca-certificates/neuro-sama-root-ca.crt && sudo update-ca-certificates
```

**Fedora / RHEL / Arch Linux:**
```bash
sudo trust anchor --store certs/ca/neuro-sama-root-ca.pem
```

> **Firefox 提示：** 在 macOS 和 Windows 上，只要在 `about:config` 中将 `security.enterprise_roots.enabled` 设为 `true`，Firefox 就会自动信任系统根证书。在 Linux 上，如果 Firefox 使用独立的 NSS 数据库而非 p11-kit，可以开启 `security.enterprise_roots.enabled` 或在 *设置 -> 隐私与安全 -> 证书 -> 查看证书 -> 证书机构* 中手动导入一次 `certs/ca/neuro-sama-root-ca.pem`。

---

## 保护多个站点与验证等级（`0`–`3`）

单个 Gymbag 进程可同时托管和反向代理数十个域名，支持多上游轮询负载均衡以及全双工 RFC 6455 WebSocket 升级。每个站点都可以通过 `verify <0-3>` 独立选择验证策略（或通过 `protect off` 关闭 WAF 质询）：

| 等级 | 名称 | 触发质询的对象 | 适用场景 |
| :---: | :--- | :--- | :--- |
| `0` | `always` | 所有未携带有效 HMAC 通行 Cookie 的访客。 | 私有工具、测试环境或正在遭受 L7 洪水攻击的站点。 |
| `1` | `only suspicious` | 威胁情报/不良信誉 IP、扫描器、CLI HTTP 库、无头浏览器、缺失或伪造浏览器请求头以及漏洞探测请求。 | 希望普通浏览器完全无感知访问的公网 Web 应用与 API。 |
| `2` | `only ai` | 通过**官方 AI 爬虫 IP 段** *或* AI 机器人 `User-Agent`/代理请求头识别出的 AI 爬虫与大模型 Agent。 | 希望对人类和 `curl` 用户零摩擦开放，同时拦截/消耗 LLM 抓取器的文档、博客与代码托管站。 |
| `3` | `suspicious & ai` | 结合等级 `1` 与 `2`：同时质询可疑客户端/IP 与 AI 爬虫；正常人类浏览器直接放行。 | 推荐作为公网生产站点的默认配置（单独写 `verify` 即启用此等级）。 |
| — | `protect off` | 关闭工作量证明质询，仅作为纯 HTTPS 与 WebSocket 反向代理运行（若配置了站点自定义规则，仍会执行）。 | 静态资源托管、Webhook 回调与公共软件包镜像源。 |

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
}

static.example.com {
    reverse_proxy http://localhost:9000
    protect off
}
```

一旦浏览器解出 SHA-256 工作量证明（`difficulty` 可选 `1`–`5`，默认 `4`），Gymbag 会下发一个绑定了主机名和客户端 IP 的 `HttpOnly`、`SameSite=Lax` HMAC 签名通行 Cookie（`waf_clearance`，默认有效期 `24h` / `86400s`）。

### 自定义已验证状态有效期（`verify_timeout` / `clearance_ttl`）

支持在全局或单站点级别通过整数秒**或**人类可读时长（`30m`、`12h`、`1h30m`、`7d`、`2w`）自定义访客验证通过后的有效时间：

```caddyfile
{
    verify_timeout 12h     # 全局默认（别名：verified_timeout, clearance_ttl）
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
    verify_timeout 30m     # 单站点覆盖
}
```

你也可以向 `gymbag init`、`gymbag run` 或 `gymbag reverse-proxy` 传递 `--verify-timeout 6h`（或 `--clearance-ttl 6h`），设置环境变量 `GYMBAG_VERIFY_TIMEOUT` / `GYMBAG_CLEARANCE_TTL`，或者通过 `PATCH /api/v1/sites/{domain}` 动态热更新 `clearance_ttl`。已通过验证的访客请求 `GET /.waf/v1/session` 将返回 `clearance_ttl`、`expires_at` 以及剩余 `expires_in` 秒数。

---

## 切换 SSL 证书提供商、自定义密钥与自动续期

对于公网域名，默认的 `tls auto` 会通过 RFC 8555 ACME HTTP-01 自动向 Let's Encrypt 申请证书。你可以通过一条 `ssl_provider` 指令（别名：`acme_provider`、`ca`、`cert_issuer`）或 `tls <provider>` 轻松切换 CA：

| 提供商关键字 | 证书颁发机构（CA） | EAB 要求 |
| :--- | :--- | :--- |
| `letsencrypt` (`le`) | Let's Encrypt (生产环境) | 无需 |
| `letsencrypt_staging` | Let's Encrypt (测试环境) | 无需 |
| `zerossl` | ZeroSSL | 设置 `email` 后**自动获取**，或显式配置 `eab <kid> <hmac_key>` |
| `google` (`gts`) | Google Trust Services | 需要 `eab <kid> <hmac_key>` |
| `ssl.com` (`sslcom`, `ssl.com-rsa`) | SSL.com (ECC 或 RSA) | 需要 `eab <kid> <hmac_key>` |
| `litessl` (`trustasia`) | LiteSSL by TrustAsia | 标准 DV 无需（或通过 `acme_directory` 覆盖目录地址） |
| `buypass` | Buypass Go SSL | 无需 |
| `local` | 本地双层 PKI | 本地 CA（`Neuro-sama Headquarters Root CA` -> `Gymbag & Swarm SSL Local CA`） |
| `https://...` | 任意 RFC 8555 ACME v2 CA | 可选 `eab <kid> <hmac_key>`（如 Smallstep `step-ca`、Pebble） |

### 切换 ACME CA 与外部账户绑定（EAB）

**ZeroSSL 零配置邮箱自动获取 EAB：**
当你选择 `zerossl` 并填写了 `email` 时，如果未显式提供 EAB 密钥，Gymbag 会自动请求 ZeroSSL 的 EAB 接口（`https://api.zerossl.com/acme/eab-credentials-email`）换取凭据：

```caddyfile
{
    email ops@example.com
    ssl_provider zerossl
}

example.com {
    reverse_proxy http://localhost:3000
    verify 3
}
```

**Google Trust Services、SSL.com 或显式 EAB 凭据：**
传入你的 Key ID 与 HMAC 密钥（同时支持无填充 URL-safe Base64 与标准填充 Base64，且 Admin API 返回时会自动脱敏 `eab_hmac_key`）：

```caddyfile
{
    email ops@example.com
    ssl_provider google
    eab YOUR_EAB_KEY_ID YOUR_EAB_HMAC_KEY
}
```

**LiteSSL by TrustAsia 或自定义 ACME 目录 URL：**

```caddyfile
{
    email ops@example.com
    ssl_provider litessl
    # 或直接指向任意 ACME v2 目录 URL：
    # acme_directory https://ca.internal.corp/acme/acme/directory
}
```

Gymbag 会按提供商名称分别在磁盘中缓存 ACME 证书（例如 `certs/acme-zerossl-fullchain.pem`、`certs/acme-google-fullchain.pem`，Let's Encrypt 则使用 `certs/acme-fullchain.pem`）。一旦更改 `ssl_provider` 并重启或重载，Gymbag 会立即向新的 CA 申请证书，而不会继续复用旧提供商遗留的证书。

### 使用自有密钥或证书

- **自定义 ACME 账户密钥（`account_key`）**：默认情况下 Gymbag 会在 `certs/acme-account.pem` 创建并复用 ECDSA P-256 账户密钥。如果你的组织已有注册好的 ECDSA（P-256/P-384）或 RSA ACME 账户密钥，可通过 `account_key /etc/gymbag/account.pem`（或 `GYMBAG_ACCOUNT_KEY_FILE`）指定。
- **自定义 CSR 私钥（`ssl_key`）**：使用 `ssl_key /etc/gymbag/domain.key` 可使用你自己的 ECDSA 或 RSA 私钥签发 ACME CSR，而不是每次生成新密钥。
- **自定义 TLS 证书与私钥（`tls <cert> <key>`）**：支持全局或按站点（基于 SNI 路由）提供商业证书、Cloudflare Origin 证书或外部管理的 PEM 文件；若私钥已加密，可通过 `key_password`（或 `GYMBAG_KEY_PASSWORD`）解密：

```caddyfile
{
    key_password super-secret-passphrase
}

corp.example.com {
    reverse_proxy http://localhost:8080
    tls /etc/ssl/corp-fullchain.pem /etc/ssl/corp-privkey.pem
    verify 1
}
```

### 零停机后台自动续期

Gymbag 每隔 `renew_interval` 秒（默认 `43200` = 12 小时）运行一次后台续期巡检。只要 ACME 证书、本地 PKI 叶子证书或文件证书距离过期不足 `renew_within_days` 天（默认 `30` 天），Gymbag 就会在后台完成续期或重新加载，并原子切换活动 `tls.Certificate` 指针。正在进行的 HTTP 流与 WebSocket 长连接不会出现任何 TCP 断连。

---

## 编写自定义防火墙规则

对于受保护站点上的每一个入站请求，Gymbag 按严格可预期的顺序逐层评估：

1. **请求格式基础校验**（HTTP 方法合法性、最大 URI 长度、空字节与 `..` 路径穿越拦截）
2. **ACME HTTP-01 放行**（`/.well-known/acme-challenge/*`）
3. **本地自定义规则**（先全局规则，后站点规则，自上而下 —— 首条命中生效）
4. **远程防火墙规则（`remote_rules`）**（支持自动刷新的 HTTP/HTTPS 规则集，兼容 Gymbagfile 语法与 JSON）
5. **内置基线安全规则**（除非显式设置了 `default_rules off`）
6. **按站点按 IP 速率限制**（`rate_limit` / `rate_window`）
7. **IP 威胁情报拦截/质询**（`ip_security_action verify` 或 `block`）
8. **工作量证明验证**（`verify` 等级 `0`–`3`）

由于你的自定义规则在内置基线规则、限流器和威胁情报**之前**执行，在站点配置顶部添加一条 `allow` 规则即可干净利落地放行受信任办公网 CIDR、内部健康检查或携带预览 Cookie 的请求。

### 单行简写规则

语法：`<allow | verify | block> <field> <pattern...>`（同时支持别名 `bypass`/`pass`、`challenge`/`pow` 以及 `deny`/`drop`）。

| 字段 | 支持的值与模式匹配 | 示例 |
| :--- | :--- | :--- |
| `ip` | IPv4、IPv6 或 CIDR 网段（空格或逗号分隔） | `allow ip 127.0.0.1 10.0.0.0/8 2001:db8::/32` |
| `user_agent` | 忽略大小写子串、`*` 通配符或 `~regex` 正则 | `block user_agent BadBot ~^(?i)python-urllib/` |
| `path` | 精确/子串匹配、`*` 通配符或 `~regex` 正则 | `verify path /api/export* ~^/v[0-9]+/search` |
| `query` | 原始查询字符串匹配、`*` 通配符或 `~regex` 正则 | `block query "union select" ~(?i)<script` |
| `method` | HTTP 请求方法（忽略大小写） | `verify method POST DELETE` |
| `header` | `<Header-Name> [pattern]`（`""` = 缺失或为空，`*` = 存在即可） | `verify header Accept-Language ""` |
| `cookie` | `<cookie_name> [pattern]`（`""` = 缺失或为空，`*` = 存在即可） | `allow cookie vip_session *` |

### 多条件 `rule` 规则块

当单个字段不足以精确描述条件时，可以使用 `rule "name" { ... }` 块。**同一字段**的多个匹配值之间为 **OR** 逻辑；同一个规则块内的**不同字段**之间为 **AND** 逻辑：

```caddyfile
app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    # 对命中登录或重置密码接口的 POST 请求强制执行 PoW 验证
    rule "challenge-auth-mutations" {
        action verify
        method POST
        path /login* /api/auth/* /reset-password*
    }

    # 拦截缺少 Origin 请求头的外部 /api/* POST 请求
    rule "block-headless-api-posts" {
        action block
        method POST
        path /api/*
        header Origin ""
    }
}
```

### 远程防火墙规则（`remote_rules`）

支持在全局或单站点配置中从一个或多个 HTTP/HTTPS URL 自动拉取共享防火墙规则。Gymbag 同时支持 **Gymbagfile 规则语法**（`allow`/`verify`/`block` 单行规则与 `rule "name" { ... }` 规则块）以及 **JSON 格式**（`[ ... ]` 或 `{"rules": [ ... ]}`），通过 `ETag` / `If-Modified-Since` 条件请求将已校验的规则集缓存在 `<cert_dir>/rules-cache` 中；当远程源不可达或格式异常时，会自动保留上一次可用的有效规则集：

```caddyfile
{
    remote_rules https://rules.example.com/org-baseline.rules
    remote_rules_refresh 5m
}

app.example.com {
    reverse_proxy http://localhost:3000
    verify 3
    allow ip 10.0.0.0/8                              # 本地规则始终优先执行
    remote_rules https://rules.example.com/app.json  # 站点专属远程规则
}
```

你可以通过 `GET /api/v1/rules/remote` 查看状态、通过 `POST /api/v1/rules/remote/refresh` 立即触发刷新，或者使用 `--remote-rules` 与 `GYMBAG_REMOTE_RULES`。

### 控制内置规则（`default_rules on|off`）

`default_rules` 默认为 `on`：
- 当受保护站点**没有**配置自定义规则时，Gymbag 会同时启用内置拦截预设（`/.env`、`/wp-admin`、扫描器、SQLi/XSS 查询）和内置自动化客户端质询预设。
- 当你添加了自定义规则后，你的规则会优先执行，同时内置**拦截预设**仍会追加在其后，避免仅因为加了一条 `verify path` 规则而意外放行 `/.git` 或 `/.env`。
- 如果你希望在单个站点或全局彻底接管、完全禁用内置规则，可设置 `default_rules off`（或 `GYMBAG_DEFAULT_RULES=off`）。

---

## CLI、Docker Compose、环境变量与 REST API

### CLI 参考

若当前工作目录存在 `.env`，Gymbag 会自动加载，然后解析 `./Gymbagfile` 或 `./Caddyfile`（除非被命令行参数覆盖）：

```bash
# 通过交互式向导或参数生成 Gymbagfile
./gymbag init
./gymbag init -d app.example.com -u http://localhost:3000 -l 3

# 使用 ./Gymbagfile 或 ./Caddyfile 启动服务器
./gymbag

# 指定配置文件启动并监听文件改动自动热重载（每 2 秒轮询一次）
./gymbag run --config Gymbagfile --watch

# 注册并管理系统服务（systemd、launchd、openrc、supervisor）
./gymbag service install --config Gymbagfile
./gymbag service start
./gymbag service status
./gymbag service render --type systemd   # 在任意系统上预览 systemd unit 文件

# 无需配置文件直接启动临时反向代理
./gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3

# 校验语法并打印各站点的最终安全策略摘要
./gymbag validate --config Gymbagfile

# 原地格式化 Gymbagfile 为规范的 4 空格缩进
./gymbag fmt --overwrite Gymbagfile

# 打印版本信息
./gymbag version
```

| 参数 | 说明 |
| :--- | :--- |
| `-c`, `--config <path>` | 指定 `Gymbagfile`、`Caddyfile` 或 JSON 配置文件路径 |
| `-d`, `--domain <list>` | 覆盖下游域名，多个域名以逗号分隔 |
| `-u`, `--upstream <url>` | 覆盖上游目标 URL（如 `http://localhost:3000` 或 `:3000`） |
| `-l`, `--verify <0-3>` | 覆盖验证等级（`0`、`1`、`2`、`3` 或 `off`） |
| `--verify-timeout <dur>` | 覆盖已验证通行状态的过期时间（如 `30m`、`12h`、`7d` 或秒数；别名 `--clearance-ttl`） |
| `--remote-rules <url>` | 附加远程防火墙规则 URL（支持 http/https，可重复传参或逗号分隔） |
| `--remote-rules-refresh <dur>` | 远程规则刷新间隔（默认 `5m` / `300s`） |
| `--no-protect` | 为 CLI 配置的站点禁用 WAF 验证 |
| `--ssl-provider <name>` | 覆盖 ACME / SSL 提供商（如 `letsencrypt`、`zerossl`、`google`、`litessl`、`local`） |
| `--admin <addr>` | 启用本地 REST Admin API（如 `127.0.0.1:2019` 或 `off`） |
| `--http-port <port>` | HTTP 重定向与 ACME HTTP-01 监听端口（默认 `80`） |
| `--https-port <port>` | HTTPS 网关监听端口（默认 `443`） |
| `--watch` | 监听配置文件改动，自动热重载站点路由、规则与 SNI 证书 |

### Docker 与 Docker Compose

官方多架构（`linux/amd64`、`linux/arm64`）Alpine 容器镜像已发布至 GitHub Container Registry（`ghcr.io/evil-project/gymbag`），以非 root 用户 UID `10001` 运行并带有 `cap_net_bind_service=+ep`。

**挂载 `Gymbagfile` 快速执行 `docker run`：**

```bash
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

**仅使用环境变量的零配置文件 `docker run`：**

```bash
docker run -d \
  --name gymbag \
  -p 80:80 -p 443:443 \
  --cap-add NET_BIND_SERVICE \
  --add-host=host.docker.internal:host-gateway \
  -e GYMBAG_DOMAIN=app.example.com \
  -e GYMBAG_UPSTREAM=http://host.docker.internal:3000 \
  -e GYMBAG_ACME_EMAIL=ops@example.com \
  -e GYMBAG_VERIFY_LEVEL=3 \
  -e GYMBAG_VERIFY_TIMEOUT=12h \
  -e GYMBAG_REMOTE_RULES=https://rules.example.com/baseline.rules \
  -v gymbag-certs:/app/certs \
  ghcr.io/evil-project/gymbag:latest
```

**搭配 Redis 的 Docker Compose：**

仓库自带的 `compose.yaml` 会将 Gymbag 与 `redis:8-alpine` 一同启动，并利用 Gymbag 内置的纯 Go RESP2 Redis 客户端在容器重启或多实例副本之间共享一次性 PoW Nonce 与固定窗口限流状态：

```bash
docker compose up -d --build
```

配置默认读取 `./Gymbagfile`（只读挂载到 `/app/Gymbagfile`），证书、HMAC 密钥、威胁情报缓存及远程规则缓存均持久化在命名卷 `gymbag-certs` 中。本地 `.env` 文件中的任意 `GYMBAG_*` 变量都会干净地覆盖配置文件设置：

```dotenv
GYMBAG_ACME_EMAIL=ops@example.com
GYMBAG_SSL_PROVIDER=zerossl
GYMBAG_VERIFY_LEVEL=3
GYMBAG_CLEARANCE_TTL=12h
GYMBAG_REMOTE_RULES=https://rules.example.com/baseline.rules
```

完整部署指南（宿主机目录挂载权限 `chown 10001:10001`、`--watch` 热重载、多容器代理与本地构建）请参阅 [Docker 与 Docker Compose 指南](docs/zh/guide/docker.md)。

### 环境变量（`GYMBAG_*`）

优先级顺序为：**配置文件 -> `GYMBAG_*` 环境变量 -> CLI 命令行参数**。

| 变量 | 说明 |
| :--- | :--- |
| `GYMBAG_CONFIG` | `Gymbagfile`、`Caddyfile` 或 `.json` 配置文件路径 |
| `GYMBAG_BIND` | 绑定地址（仅本地域名时默认为 `127.0.0.1`，存在公网域名时为 `0.0.0.0`） |
| `GYMBAG_HTTP_PORT` / `GYMBAG_HTTPS_PORT` | HTTP 与 HTTPS 监听端口（默认 `80` 与 `443`） |
| `GYMBAG_DOMAIN` / `GYMBAG_DOMAINS` | 逗号分隔的下游域名列表 |
| `GYMBAG_UPSTREAM` | 默认上游目标 URL |
| `GYMBAG_UPSTREAMS` | 分号分隔的站点上游映射：`app.example.com=http://web:3000;api.example.com=http://api:8080` |
| `GYMBAG_PROTECT` | 全局开启或关闭站点 WAF 防护（`on` / `off`） |
| `GYMBAG_PROTECT_DOMAINS` | 启用 WAF 防护的站点域名子集（逗号分隔） |
| `GYMBAG_VERIFY_LEVEL` | 验证等级：`0`（`always`）、`1`（`only suspicious`）、`2`（`only ai`）、`3`（`suspicious & ai`） |
| `GYMBAG_DEFAULT_RULES` | 开启或关闭内置基线防火墙规则（`on` / `off`，默认 `on`） |
| `GYMBAG_REMOTE_RULES` | 逗号或空格分隔的 HTTP/HTTPS 远程防火墙规则 URL |
| `GYMBAG_REMOTE_RULES_REFRESH` | 远程规则刷新间隔（秒数或时长，默认 `300` / `5m`） |
| `GYMBAG_DIFFICULTY` | SHA-256 PoW 前导零位数难度（`1`–`5`，默认 `4`） |
| `GYMBAG_CHALLENGE_TTL` / `GYMBAG_CLEARANCE_TTL` | 质询有效期（`300`s / `5m`）与已验证状态超时（`86400`s / `24h`；别名 `GYMBAG_VERIFY_TIMEOUT`、`GYMBAG_VERIFIED_TIMEOUT`；支持 `30m`、`12h`、`7d` 等格式） |
| `GYMBAG_COOKIE_NAME` / `GYMBAG_COOKIE_SECURE` | 通行 Cookie 名称（`waf_clearance`）与 `Secure` 标志（`true`） |
| `GYMBAG_RATE_LIMIT` / `GYMBAG_RATE_WINDOW` | 单 IP 请求上限（`240`）与统计窗口秒数（`60`） |
| `GYMBAG_TLS_MODE` | `auto`、`acme`、`local`、`selfsigned` 或 `file` |
| `GYMBAG_SSL_PROVIDER` | `letsencrypt`、`letsencrypt_staging`、`zerossl`、`google`、`ssl.com`、`litessl`、`buypass`、`local` 或自定义 URL |
| `GYMBAG_ACME_EMAIL` | ACME 注册联系邮箱（同时用于 ZeroSSL 自动 EAB） |
| `GYMBAG_ACME_DIRECTORY` | 显式指定 RFC 8555 ACME v2 目录 URL |
| `GYMBAG_EAB_KID` / `GYMBAG_EAB_HMAC_KEY` | EAB Key ID 与 Base64/Base64URL HMAC 密钥 |
| `GYMBAG_ACCOUNT_KEY_FILE` | 用户提供的 ECDSA 或 RSA ACME 账户私钥 PEM 路径 |
| `GYMBAG_CERT_FILE` / `GYMBAG_KEY_FILE` | `file` 模式下使用的 TLS 证书与私钥路径 |
| `GYMBAG_KEY_PASSWORD` | 加密 PEM 私钥的解密口令 |
| `GYMBAG_CERT_DIR` | 存放 CA 证书、ACME 状态与情报缓存的目录（默认 `certs`） |
| `GYMBAG_RENEW_INTERVAL` / `GYMBAG_RENEW_WITHIN_DAYS` | 续期巡检间隔秒数（`43200`）与提前续期阈值天数（`30`） |
| `GYMBAG_AUTO_HOSTS` | 自动管理 `localhost` 与 `*.internal` 的 `/etc/hosts`（`on` / `off`，默认 `on`） |
| `GYMBAG_HSTS` | 覆盖 `Strict-Transport-Security` 响应头（`on` / `off`） |
| `GYMBAG_IP_SECURITY` / `GYMBAG_IP_SECURITY_ACTION` | 威胁情报开关（`on`/`off`）与命中动作（`verify` 或 `block`） |
| `GYMBAG_IP_REPUTATION` | 实时 Shodan InternetDB + StopForumSpam 查询开关（`on`/`off`） |
| `GYMBAG_AI_IP_LIST` | 官方 OpenAI、Anthropic、Perplexity、Google 与 Applebot IP 列表开关（`on`/`off`） |
| `GYMBAG_INTEL_REFRESH` | 威胁情报与 AI IP 列表刷新间隔秒数（默认 `21600`） |
| `GYMBAG_REDIS_URL` | 可选的 Redis 连接地址（`redis://[:password@]host:port/db`） |
| `GYMBAG_SECRET` | 通行 Cookie 签名 HMAC 密钥（未设置时自动生成于 `<cert_dir>/secret`） |
| `GYMBAG_ADMIN` | 本地 Admin REST API 监听地址（如 `127.0.0.1:2019` 或 `off`） |

### REST API

#### 公共 WAF 接口（暴露在受保护站点下）

| 方法 | 路径 | 说明 |
| :--- | :--- | :--- |
| `GET` | `/.waf/v1/session` | 返回当前站点验证状态、`protected`、`verify_level`（`0`–`3`）、`verify_mode`、`clearance_ttl`，以及（已验证时）`expires_at` / `expires_in` |
| `DELETE` | `/.waf/v1/session` | 清除访客当前的通行 Cookie（`204 No Content`） |
| `POST` | `/.waf/v1/challenges` | 签发一次性 SHA-256 PoW 质询（`201 Created`） |
| `POST` | `/.waf/v1/verifications` | 校验 PoW 解、签发签名通行 Cookie 并返回 `expires_in`（兼容 `/.waf/verify` 别名） |
| `GET` | `/.waf/assets/{name}` | 提供内嵌 PNG 质询页素材（支持 `/.waf/v1/assets/{name}` 别名及 `ETag` 缓存） |

#### 本地管理 API（通过 `admin 127.0.0.1:2019` 或 `--admin` 启用）

请将管理 API 绑定至回环地址（`127.0.0.1:2019`）或 Unix/WireGuard 管理网卡 —— 它无需认证令牌即可在不重启服务的情况下实时修改路由与防火墙规则。

| 方法 | 路径 | 说明 |
| :--- | :--- | :--- |
| `GET` | `/api/v1/health` | 网关健康状态、站点数量、活动 `tls_source`、已加载的 `threat_entries`、`ai_entries` 与 `remote_rules` |
| `GET` | `/api/v1/tls` | 当前活动 TLS 来源、证书路径、配置的 SSL 提供商、HSTS 状态及支持的 CA 目录 |
| `POST` | `/api/v1/tls/renew` | 强制立即执行证书续期并原子热重载 |
| `GET` | `/api/v1/intel` | 各威胁情报与 AI 爬虫源计数、上次刷新时间戳与信誉缓存统计 |
| `POST` | `/api/v1/intel/refresh` | 立即触发后台情报源刷新并返回最新计数 |
| `GET` | `/api/v1/rules/remote` | 按 URL 列出远程防火墙规则数量、缓存状态、刷新间隔及最近拉取错误 |
| `POST` | `/api/v1/rules/remote/refresh` | 立即拉取全部远程规则并原子重载 WAF 引擎 |
| `GET` | `/api/v1/config` | 导出当前活动运行时配置（自动脱敏 `secret`、`key_password` 和 `eab_hmac_key`） |
| `PUT` | `/api/v1/config` | 原子校验并替换整份运行时配置 |
| `GET` | `/api/v1/sites` | 列出所有已配置站点及其 WAF 策略 |
| `POST` | `/api/v1/sites` | 运行时动态新增站点（`201 Created`） |
| `GET` | `/api/v1/sites/{domain}` | 查看单个站点的详细配置 |
| `PUT` | `/api/v1/sites/{domain}` | 整体替换单个站点配置 |
| `PATCH` | `/api/v1/sites/{domain}` | 实时热修补 `verify_level`、`protect`、`difficulty`、`clearance_ttl`、`upstreams`、`rate_limit`、`rules` 或 `remote_rules` |
| `DELETE` | `/api/v1/sites/{domain}` | 移除站点（`204 No Content`；拒绝删除最后一个剩余站点） |
| `GET` | `/api/v1/sites/{domain}/rules` | 列出站点的自定义防火墙规则 |
| `POST` | `/api/v1/sites/{domain}/rules` | 在线追加一条自定义防火墙规则（`201 Created`） |
| `PUT` | `/api/v1/sites/{domain}/rules` | 整体替换站点的全部自定义防火墙规则 |
| `DELETE` | `/api/v1/sites/{domain}/rules/{index_or_name}` | 按数字索引或规则 `name` 删除自定义规则（`204 No Content`） |

**紧急热防护示例：**

```bash
# 遭遇异常流量高峰时，立即将 app.example.com 收紧为 verify level 0（全员质询）
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"verify_level": 0, "difficulty": 5}'

# 无需编辑 Gymbagfile，立即拉黑恶意 CIDR 网段
curl -X POST http://127.0.0.1:2019/api/v1/sites/app.example.com/rules \
  -H "Content-Type: application/json" \
  -d '{"name": "drop-abusive-subnet", "action": "block", "ips": ["198.51.100.0/24"]}'

# 强制立即刷新 IP 威胁情报与 AI 爬虫列表
curl -X POST http://127.0.0.1:2019/api/v1/intel/refresh
```

---

## 与 Caddy、Nginx 的性能基准对比

测试环境为 Apple M4（10 核，macOS，Go `1.27.0`），三款代理统一使用同一张 ECDSA P-256 证书终结 TLS 1.3，64 并发压测、`1 KiB` 响应体与 `1024` 上游 keep-alive 连接（全部压测均实现 `0` 错误）：

| 服务器与模式 | HTTP/1.1 TLS (RPS) | H1 p50 / p99 | HTTP/2 TLS (RPS) | H2 p50 / p99 | 二进制体积 | 负载下 RSS 内存 |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **85,203** | 0.45 / 6.20 ms | **88,775** | 0.65 / 1.96 ms | 1.5 MB (+libs) | 5.8 MiB |
| **Gymbag `v0.1.0`** (`verify 3`，全量 WAF 开启) | **72,517** | 0.74 / 3.14 ms | **66,096** | 0.82 / 3.48 ms | 12.9 MB (静态) | 54.3 MiB |
| **Gymbag `v0.1.0`** (`protect off`，纯反代) | **72,479** | 0.74 / 3.15 ms | **64,048** | 0.83 / 3.94 ms | 12.9 MB (静态) | 52.1 MiB |
| **Gymbag `v0.1.0`** (`verify 0`，已通过 Cookie) | **65,490** | 0.79 / 3.93 ms | **62,108** | 0.84 / 4.27 ms | 12.9 MB (静态) | ~54 MiB |
| **Caddy `v2.11.9`** (Go，keep-alive 调优) | **54,741** | 0.90 / 4.90 ms | **39,207** | 1.30 / 6.18 ms | 49.0 MB | 76.8 MiB |

- **比 Caddy 快 +32%（HTTP/1.1）与 +63% 至 +69%（HTTP/2）**，同时内存占用少约 30%，二进制体积缩小 3.8 倍 —— 而且这是在 **Gymbag 开启完整 WAF 检测** 的前提下。
- **达到 C 语言 Nginx HTTP/1.1 吞吐量的 85%**，且 p99 尾延迟降低近 **2 倍**（`3.14 ms` vs `6.20 ms`）。
- **140,680 req/s** 边缘 WAF 直接拦截能力（基于 TLS 下的 `/.env` 探测，`0.35 ms` p50，上游零流量穿透）。

完整测试方法论、Go 微基准测试（`go test -bench`）与确切配置请参阅 [基准测试与性能优化](docs/zh/guide/benchmarks.md)。

---

## 项目结构

```text
.
├── cmd/gymbag/          # 独立 CLI 入口
├── docs/                # 多语言 VitePress 文档（EN, 简体中文, 日本語, 한국어, Español）
├── internal/
│   ├── cli/             # 子命令（init, service, run, reverse-proxy, validate, fmt, version）、.env 与热加载监听器
│   ├── config/          # 配置模型、Gymbagfile/Caddyfile 与时长/远程规则解析器、GYMBAG_* 环境变量加载器
│   ├── host/            # 域名归一化、IPv6 方括号处理、通配符匹配
│   ├── intel/           # O(1) 前缀集合、自动更新威胁情报与 AI 爬虫列表、实时 IP 信誉查询
│   ├── localpki/        # 双层本地 CA（"Neuro-sama Headquarters Root CA"）与 /etc/hosts 管理器
│   ├── proxy/           # 无锁多站点路由器、池化 HTTP 与 RFC 6455 WebSocket 代理
│   ├── server/          # 融合型 HTTPS 网关、HTTP->HTTPS/ACME 重定向器、REST 管理 API
│   ├── service/         # 系统服务模板渲染与管理器（systemd, launchd, openrc, supervisor）
│   ├── store/           # StateStore 接口、64 分片内存存储、纯 Go RESP2 Redis 客户端
│   ├── tlsmanager/      # 多 CA ACME v2（支持 EAB 与 ZeroSSL 自动 EAB）、本地 PKI、无锁 SNI 证书管理器
│   └── waf/             # PoW 校验器、分片 HMAC 通行缓存、Aho-Corasick DFA、远程规则、挑战页 UI
├── Dockerfile
├── compose.yaml
├── flake.nix
├── Gymbagfile
└── go.mod
```

---

## Star 趋势

<a href="https://www.star-history.com/#Evil-Project/gymbag&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
 </picture>
</a>
