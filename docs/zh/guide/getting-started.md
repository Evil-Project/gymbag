# 两条命令快速开始

Gymbag 编译为单个独立二进制文件，零外部 Go 依赖 — 所有 HTML 挑战页面、SHA-256 浏览器求解脚本和吉祥物 PNG 资源均通过 `go:embed` 直接嵌入二进制中。

## 安装方式

### 预编译静态二进制、Docker、Arch Linux 与 NixOS

[GitHub Releases](https://github.com/Evil-Project/gymbag/releases) 上的每个发布标签均提供 `CGO_ENABLED=0` 静态二进制文件（`linux`、`darwin`、`windows` 的 `amd64`/`arm64`）、Arch Linux `.pkg.tar.zst` 安装包（`x86_64` 与 `aarch64`），以及多架构 GHCR 容器镜像（`ghcr.io/evil-project/gymbag`）：

::: code-group
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

# 或通过 PKGBUILD 从源码构建
git clone https://github.com/Evil-Project/gymbag.git
cd gymbag/packaging/arch
makepkg -si
```

```bash [Nix / NixOS]
# 通过 Nix Flakes 直接运行或安装
nix run github:Evil-Project/gymbag -- --help
nix profile install github:Evil-Project/gymbag

# 静态 Linux 发布包 (CGO_ENABLED=0) 也可直接在 NixOS 上运行
```

```bash [Go 源码编译]
git clone https://github.com/Evil-Project/gymbag.git
cd gymbag
go build -o gymbag ./cmd/gymbag
```
:::

容器部署、Compose + Redis 编排及卷权限说明请参阅 [Docker 与 Compose 部署指南](/zh/guide/docker)。

## 两条命令启动

### 方式 A：命令行参数驱动（非交互式）

```bash
# 1. 生成 gymbag.json（添加 --format gymbagfile 可生成 Caddy 风格的 Gymbagfile）
./gymbag init -d app.example.com -u http://localhost:3000

# 2. 启动 Gymbag（依次自动加载 ./gymbag.json、./Gymbagfile、./Caddyfile）
./gymbag
```

### 方式 B：交互式配置向导

不带参数直接运行 `gymbag init`，向导会引导您设置域名、上游 URL、验证级别、SSL 提供商、端口、管理 API 及可选系统服务注册：

```bash
# 1. 交互式生成配置
./gymbag init

# 2. 启动 Gymbag
./gymbag
```

### 方式 C：本地开发预设（`*.internal`）

```bash
# 1. 生成带本地 PKI 的 app.internal -> http://localhost:3000 配置
./gymbag init --preset local

# 2. 启动 Gymbag（非特权端口，或使用 sudo 绑定 80/443 与 /etc/hosts）
./gymbag --http-port 8080 --https-port 8443
```

### 方式 D：注册为系统服务

```bash
# 1. 生成 gymbag.json 并注册 systemd / launchd / openrc / supervisor
./gymbag init -d app.example.com -u :3000 --service auto

# 2. 启动已注册的服务
./gymbag service start
```

详见 [系统服务安装与管理](/zh/guide/system-service)。

---

## 导入现有 nginx / Apache / Caddyfile 配置

Gymbag 原生支持读取 `nginx.conf`、Apache `httpd.conf` / `apache2.conf` 与 Caddyfile v2 文件，您可以直接基于现有配置启动：

```bash
./gymbag run -c /etc/nginx/nginx.conf
./gymbag config convert -c Caddyfile -o gymbag.json --format json
```

有关路径路由、静态文件、重定向与配置转换的详情，请参阅 [路径路由与配置导入](/zh/guide/routes-and-imports)。

---

## 临时反向代理（无需配置文件）

无需在磁盘创建配置文件，即可快速启动带防护的反向代理：

```bash
./gymbag reverse-proxy --from app.internal --to :3000 --verify 3 --verify-timeout 12h
```

---

## 配置生成器参数 (`gymbag init`)

`gymbag init` 在写入磁盘前会自动校验生成的配置。

| 参数 | 说明 |
| :--- | :--- |
| `-o`, `--output <path>` | 输出路径（默认 `gymbag.json`，指定 `--format gymbagfile` 时默认 `Gymbagfile`） |
| `--format <fmt>` | `json`（默认）或 `gymbagfile` |
| `--preset <name>` | `standard`、`local`、`public`、`strict`、`ai-only`、`suspicious` 或 `proxy-only` |
| `-d`, `--domain <domains>` | 下游域名，多个域名以逗号分隔 |
| `-u`, `--upstream <url>` | 上游目标 URL（如 `http://localhost:3000` 或 `:3000`） |
| `--site <domain=url>` | 添加多个站点块（可重复，如 `--site app.example.com=:3000 --site api.example.com=:8080`） |
| `-l`, `--verify <0-3\|off>` | 验证级别（`0`、`1`、`2`、`3` 或 `off`） |
| `--difficulty <1-5>` | SHA-256 PoW 前导十六进制零难度（默认 `4`） |
| `--verify-timeout <dur>` | 自定义已验证状态有效期（如 `30m`、`12h`、`7d` 或秒数；别名 `--clearance-ttl`） |
| `--remote-rules <url>` | 远程防火墙规则 URL（可重复或逗号分隔） |
| `--remote-rules-refresh <dur>` | 远程规则刷新间隔（默认 `5m`） |
| `--rules-file <path>` | 指定独立的本地防火墙规则文件路径（可重复） |
| `--rules-dir <path>` | 指定本地防火墙规则目录（可重复） |
| `--rules-dynamic` | 当 `--rules-file` / `--rules-dir` 文件变更时自动热重载 |
| `--rules-watch-interval <dur>` | 动态本地规则文件/目录的轮询间隔（默认 `2s`） |
| `--no-protect` | 关闭 WAF 验证（`protect off`） |
| `--no-default-rules` | 关闭内置基础防火墙规则（`default_rules off`） |
| `--ssl-provider <name>` | `letsencrypt`、`zerossl`、`google`、`ssl.com`、`litessl`、`buypass`、`local` 或自定义 ACME URL |
| `--email <addr>` | ACME 联系邮箱（同时支持 ZeroSSL 自动 EAB） |
| `--eab-kid` / `--eab-hmac-key` | ACME 外部账号绑定（EAB）凭据 |
| `--http-port` / `--https-port` | 自定义 HTTP / HTTPS 端口（默认 `80` / `443`） |
| `--bind <ip>` | 监听绑定地址（省略时自动选择） |
| `--admin <addr>` | 启用本地 REST 管理 API（如 `127.0.0.1:2019`） |
| `--cert-dir <dir>` | 证书、本地 CA 与缓存目录（默认 `certs`） |
| `--redis <url>` | 可选 Redis URL，用于多实例共享质询与限流状态 |
| `--ip-security-action <act>` | 威胁情报 IP 命中动作（`verify` 或 `block`） |
| `--no-auto-hosts` | 关闭本地域名 `/etc/hosts` 自动管理 |
| `-y`, `--yes` | 跳过交互提示并使用默认值 |
| `-i`, `--interactive` | 即使传入参数也强制运行交互向导 |
| `--dry-run` | 仅将生成的配置输出到标准输出而不写入文件 |
| `-f`, `--force` | 覆盖已存在的配置文件 |
| `--service <type>` | 同时注册系统服务（`auto`、`systemd`、`launchd`、`openrc`、`supervisor`） |

---

## 开箱即用防护包含哪些能力

只需写入 `verify`（即验证级别 `3`：`suspicious & ai`）或 `block`，无需编写任何正则即可立刻启用三层防护：

1. **内置基础防火墙规则 (`default_rules on`)** — 自动拦截 `/.env`、`/.git/*`、`/.aws/*`、`/wp-admin/*`、`sqlmap`/`nikto`/`nuclei` 及 SQLi/XSS 查询探测（`403`），并对 CLI/无头自动化客户端发起 PoW 质询。
2. **自动更新 IP 威胁情报** — 启动时及每 6 小时自动拉取 IPsum（级别 3+）、Spamhaus DROP v4/v6、abuse.ch Feodo Tracker，并结合免密钥 Shodan InternetDB 与 StopForumSpam 实时信誉查询。
3. **官方 AI 爬虫 IP 库** — 追踪 OpenAI、Anthropic、Perplexity、Google 与 Apple 公布的爬虫 CIDR 网段，即使爬虫伪造普通桌面浏览器 User-Agent 也能准确识别。
