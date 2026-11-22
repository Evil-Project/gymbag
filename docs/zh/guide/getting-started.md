# 两条命令快速开始

Gymbag 仅使用 Go 标准库编译为单个二进制文件。HTML 模板、浏览器端 SHA-256 求解脚本以及吉祥物 PNG 图片都通过 `go:embed` 直接嵌入二进制内部。

## 安装

[GitHub Releases](https://github.com/Evil-Project/gymbag/releases) 上的每次版本发布都会附带 `CGO_ENABLED=0` 静态二进制（`linux`、`darwin`、`windows` 的 `amd64`/`arm64`）、Debian/Ubuntu `.deb` 包、Arch Linux `.pkg.tar.zst` 包、NixOS 发布包以及多架构 GHCR 容器镜像（`ghcr.io/evil-project/gymbag`）：

::: code-group
```bash [Debian / Ubuntu (.deb)]
# 安装发行版 .deb 包（会自动设置 cap_net_bind_service 并配置 systemd unit）
sudo apt install ./gymbag_<version>_linux_amd64.deb

# 生成配置并启用系统服务
sudo gymbag init -o /etc/gymbag/Gymbagfile --format gymbagfile
sudo systemctl enable --now gymbag
```

```bash [Docker (GHCR)]
docker run -d --name gymbag \
  -p 80:80 -p 443:443 \
  --cap-add NET_BIND_SERVICE \
  -v "$(pwd)/Gymbagfile:/app/Gymbagfile:ro" \
  -v gymbag-certs:/app/certs \
  ghcr.io/evil-project/gymbag:latest
```

```bash [Arch Linux (pacman)]
# 安装预编译 .pkg.tar.zst 包
sudo pacman -U gymbag-<version>-1-x86_64.pkg.tar.zst

# 或从 packaging/arch 目录构建
git clone https://github.com/Evil-Project/gymbag.git
cd gymbag/packaging/arch
makepkg -si
```

```bash [Nix / NixOS]
nix run github:Evil-Project/gymbag -- --help
nix profile install github:Evil-Project/gymbag

# 静态 Linux 二进制（CGO_ENABLED=0）也可以直接在 NixOS 上运行
```

```bash [源码编译 (Go)]
git clone https://github.com/Evil-Project/gymbag.git
cd gymbag
go build -o gymbag ./cmd/gymbag
```
:::

容器部署、Compose + Redis 编排以及挂载卷权限说明请参阅 [Docker 与 Docker Compose 指南](/zh/guide/docker)。

## 两条命令启动

`gymbag init` 默认写入 `gymbag.json`，传入 `--format gymbagfile`（或 `-o Gymbagfile`）则写入 `Gymbagfile`。生成的配置会在落盘前自动校验。

### HTTP / HTTPS 网站

```bash
# 1. 生成 gymbag.json（加上 --format gymbagfile 可生成 Caddy 风格配置）
./gymbag init -d app.example.com -u http://localhost:3000

# 2. 启动 Gymbag（依次探测 ./gymbag.json、./Gymbagfile、./Caddyfile）
./gymbag
```

### SSH 防护

自动将 `/etc/ssh/sshd_config` 中的 `sshd` 改到 `20000–59999` 范围内的空闲随机端口（保留 `.gymbag.bak` 备份），执行 `sshd -t` 校验（失败自动回滚），重载 `sshd`，并更新主机防火墙（`ufw`、`firewalld`、`nftables` 或 `iptables`+`ip6tables`），让外部流量只能通过 Gymbag 监听的 `22` 端口进入：

```bash
# 在远程主机上执行时，请保持第二个 SSH 终端处于连接状态
sudo ./gymbag init -t ssh
sudo ./gymbag
```

### Minecraft 服务器防护

传入 `--server-jar` 后，Gymbag 会随防护代理**自动拉起你的 Minecraft 服务端**，并**自动选择本机最兼容的 Java 运行时**（通过检查 `version.json`、bundler 嵌套 JAR、`patch.properties` 或 `.class` 字节码版本头区分 Java 8 旧版服务端与 Java 17/21+ 服务端，并在关闭时向控制台发送优雅 `stop` 指令）。同时 Gymbag 会把 `server.properties` 中的 `server-port` 改为 `20000–59999` 间的空闲随机端口（配置了 `server_jar` 且文件不存在时自动创建，并将空值、`0.0.0.0` 或 `::` 的 `server-ip` 绑定为 `127.0.0.1`），服务器列表 Ping 保持放行，玩家登录时会在 Minecraft **“连接已丢失（Connection Lost）”** 弹窗里看到 HTTPS 验证链接：

```bash
# 自动选择 Java 启动 server.jar + 随机后端端口：
./gymbag init -t minecraft --server-jar /srv/mc/paper.jar --mc-jvm-args "-Xms1G -Xmx4G"
./gymbag

# 或仅通过 server.properties 防护已在运行的服务器：
./gymbag init -t minecraft --server-properties /path/to/server.properties
./gymbag
```

### 一体化防护（`http,ssh,minecraft`）或交互式向导

单个进程可以同时防护网站、SSH 和 Minecraft：

```bash
# 非交互式生成组合配置：
sudo ./gymbag init -t http,ssh,minecraft -d app.example.com -u :3000 --server-jar /srv/mc/paper.jar
sudo ./gymbag

# 或运行交互式向导：
./gymbag init
./gymbag
```

> **验证门户与 IP SSL 证书：** SSH 和 Minecraft 默认共用 `8443` 端口上的 HTTPS 验证门户（`--verify-port`）。传入 `--verify-url https://verify.example.com:8443` 可使用自定义域名；留空则使用检测到的服务器 IP 并申请 RFC 8738 ACME IP 证书（当 HTTP-01 不可达时回退到 Gymbag 本地 CA 或自签名 IP 证书）。详见 [SSH 与 Minecraft 防护指南](/zh/guide/ssh-minecraft)。

### 本地开发（`*.internal`）

```bash
# 1. 生成使用本地 PKI 的 app.internal -> http://localhost:3000 配置
./gymbag init --preset local

# 2. 在非特权端口运行（或用 sudo 监听 80/443 并自动写 /etc/hosts）
./gymbag --http-port 8080 --https-port 8443
```

### 注册为系统服务

```bash
./gymbag init -d app.example.com -u :3000 --service auto
./gymbag service start
```

`systemd`、`launchd`、`openrc` 与 `supervisor` 的配置方法见 [系统服务配置](/zh/guide/system-service)。

## 使用现有 nginx / Apache / Caddyfile 配置

Gymbag 可直接解析原生 `nginx.conf`、Apache `httpd.conf` / `apache2.conf` 以及 Caddyfile v2 文件：

```bash
./gymbag run -c /etc/nginx/nginx.conf
./gymbag config convert -c Caddyfile -o gymbag.json --format json
```

路径路由、静态文件、重定向与配置转换细节见 [路径路由与配置导入](/zh/guide/routes-and-imports)。

## 临时反向代理（无配置文件）

```bash
./gymbag reverse-proxy --from app.internal --to :3000 --verify 3 --verify-timeout 12h
```

## `gymbag init` 参数

| 参数 | 说明 |
| :--- | :--- |
| `-o`, `--output <path>` | 输出路径（默认 `gymbag.json`，指定 `--format gymbagfile` 时为 `Gymbagfile`） |
| `--format <fmt>` | `json`（默认）或 `gymbagfile` |
| `-t`, `--template <list>` | 服务模板：`http`、`ssh`、`minecraft` 或 `all`（逗号分隔或重复传参） |
| `--preset <name>` | `standard`、`local`、`public`、`strict`、`ai-only`、`suspicious`、`proxy-only`、`http`、`ssh`、`minecraft` 或 `all` |
| `-d`, `--domain <domains>` | 下游域名，多个以逗号分隔 |
| `-u`, `--upstream <url>` | 上游地址（`http://localhost:3000` 或 `:3000`） |
| `--site <domain=url>` | 添加多个站点条目（可重复传参） |
| `--ssh` / `--ssh-port <port>` | 启用 SSH 防护（默认端口 `22`） |
| `--ssh-upstream <addr>` / `--sshd-config <p>` | 显式指定 `sshd` 目标或 `sshd_config` 路径（默认 `/etc/ssh/sshd_config`） |
| `--minecraft`, `--mc` / `--mc-port <port>` | 启用 Minecraft 防护（默认端口 `25565`） |
| `--mc-upstream <addr>` / `--server-properties <p>` | 显式指定 Minecraft 后端或 `server.properties` 路径 |
| `--mc-jar`, `--server-jar <path>` | Minecraft 服务端 `.jar` 路径（自动选择 Java 并拉起服务端） |
| `--mc-java`, `--java <bin>` | 显式指定 Minecraft 服务端使用的 Java 可执行文件路径（默认自动选择） |
| `--mc-jvm-args <args>` / `--mc-work-dir <dir>` | 托管 Minecraft 服务端使用的空格分隔 JVM 参数与工作目录 |
| `--verify-url <url>` | 共享 HTTPS 验证门户 URL（留空则使用服务器 IP + IP SSL 证书） |
| `--verify-port <port>` | 共享 HTTPS 验证门户端口（默认 `8443`） |
| `--no-auto-setup` / `--no-firewall` | 跳过自动修改 `sshd_config`/防火墙或 `server.properties` |
| `-l`, `--verify <0-3\|off>` | 验证级别（`0`、`1`、`2`、`3` 或 `off`） |
| `--difficulty <1-5>` | SHA-256 PoW 难度（默认 `4`） |
| `--verify-timeout <dur>` | 通行有效期（`30m`、`12h`、`7d` 或秒数；别名 `--clearance-ttl`） |
| `--remote-rules <url>` | 远程防火墙规则 URL（可重复或逗号分隔） |
| `--remote-rules-refresh <dur>` | 远程规则刷新间隔（默认 `5m`） |
| `--rules-file <path>` / `--rules-dir <path>` | 本地规则文件或目录（可重复传参） |
| `--rules-dynamic` / `--rules-watch-interval <dur>` | 监听本地规则变更并热重载（默认 `2s`） |
| `--no-protect` / `--no-default-rules` | 关闭 PoW 验证或内置基线规则 |
| `--ssl-provider <name>` | `letsencrypt`、`zerossl`、`google`、`ssl.com`、`litessl`、`buypass`、`local` 或 URL |
| `--email <addr>` | ACME 联系邮箱（同时用于 ZeroSSL 自动 EAB） |
| `--eab-kid` / `--eab-hmac-key` | ACME 外部账户绑定凭据 |
| `--http-port` / `--https-port` | 自定义 HTTP/HTTPS 端口（默认 `80` / `443`） |
| `--bind <ip>` | 监听绑定 IP（省略时自动判断） |
| `--admin <addr>` | 启用本地 REST Admin API（如 `127.0.0.1:2019`） |
| `--cert-dir <dir>` | 证书、本地 CA 与缓存目录（默认 `certs`） |
| `--redis <url>` | Redis URL，用于共享质询、限流与 SSH/MC 验证状态 |
| `--ip-security-action <act>` | 威胁情报命中动作（`verify` 或 `block`） |
| `--no-auto-hosts` | 关闭本地域名 `/etc/hosts` 自动管理 |
| `-y`, `--yes` / `-i`, `--interactive` | 跳过向导提示或强制启用交互式向导 |
| `--dry-run` / `-f`, `--force` | 仅输出配置到 stdout 而不写盘 / 覆盖已有文件 |
| `--service <type>` | 注册系统服务（`auto`、`systemd`、`launchd`、`openrc`、`supervisor`） |

## 不写规则时默认生效的能力

写上 `verify`（启用级别 `3`：`suspicious & ai`）或 `block` 后，无需编写规则即可直接启用三层防护：

1. **内置基线规则（`default_rules on`）**：对 `/.env`、`/.git/*`、`/.aws/*`、`/wp-admin/*`、`sqlmap`/`nikto`/`nuclei` 及 SQLi/XSS 查询探测返回 `403`，并将 CLI 与无头浏览器 HTTP 客户端送入 PoW 验证页。
2. **威胁情报、实时 IP 信誉与 VPN/Tor 检测**：在启动时和每 6 小时刷新 IPsum（level 3+）、Spamhaus DROP v4/v6、Feodo Tracker、X4BNet VPN 列表和 Tor 出口节点，并通过 Shodan InternetDB、StopForumSpam、`ipquery.io` 与 `ipapi.is` 执行故障放行实时查询。VPN/代理/Tor IP 在 HTTP、SSH 和 Minecraft 的级别 `1` 与 `3` 中都会判定为可疑流量。
3. **官方 AI 爬虫 IP 列表**：匹配 OpenAI、Anthropic、Perplexity、Google 与 Apple 公布的 CIDR 网段，即便爬虫将 `User-Agent` 伪装为 Chrome 也能识别。
