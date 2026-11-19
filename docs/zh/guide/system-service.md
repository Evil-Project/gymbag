# 系统服务安装与管理

Gymbag 内置系统服务管理器（`gymbag service`），支持以下环境的服务生成、注册、启停、重启、状态查看与卸载：

| 管理器 | 目标操作系统 / Init 系统 | 默认单元文件路径 |
| :--- | :--- | :--- |
| `systemd` | Linux（Ubuntu、Debian、Fedora、Arch、RHEL） | `/etc/systemd/system/<name>.service`（`--user` 时为 `~/.config/systemd/user/<name>.service`） |
| `launchd` | macOS（LaunchDaemons / LaunchAgents） | `/Library/LaunchDaemons/io.gymbag.<name>.plist`（`--user` 时为 `~/Library/LaunchAgents/io.gymbag.<name>.plist`） |
| `openrc` | Linux（Alpine、Gentoo、Artix） | `/etc/init.d/<name>` |
| `supervisor` | Unix / 容器（`supervisord`） | `/etc/supervisor/conf.d/<name>.conf`（`--user` 时为 `~/.config/supervisor/conf.d/<name>.conf`） |

---

## 两条命令注册并启动服务

生成 `Gymbagfile` 后：

```bash
# 1. 注册系统服务（自动检测 systemd / launchd / openrc / supervisor）
sudo ./gymbag service install --config /etc/gymbag/Gymbagfile

# 2. 启动服务
sudo ./gymbag service start
```

也可以将配置生成与服务注册合并为第一步：

```bash
sudo ./gymbag init -d app.example.com -u :3000 -o /etc/gymbag/Gymbagfile --service auto --system
sudo ./gymbag service start --system
```

::: tip 自动开启 `--watch` 与 `CAP_NET_BIND_SERVICE`
- `gymbag service install` 默认配置 `gymbag run --config <path> --watch`，修改 `Gymbagfile` 后会自动热重载路由、WAF 策略与规则，无需重启进程。
- 使用非 root `--service-user` 注册系统级 `systemd` 服务且监听 `80`/`443` 端口时，Gymbag 会自动添加 `AmbientCapabilities=CAP_NET_BIND_SERVICE`、`CapabilityBoundingSet=CAP_NET_BIND_SERVICE` 与 `NoNewPrivileges=true`。
- OpenRC 单元在指定非 root `--service-user` 时也会自动添加 `capabilities="^cap_net_bind_service"`。
:::

---

## 服务生命周期子命令

```bash
# 启动、停止、重启或查看状态
./gymbag service start
./gymbag service stop
./gymbag service restart
./gymbag service status

# 卸载并删除服务单元文件
./gymbag service uninstall
```

### 用户级与系统级作用域

- 非 root 运行时（`euid != 0`），`systemd`、`launchd` 和 `supervisor` 默认使用 `--user` 作用域。
- 以 `root`（`sudo`）运行时默认使用 `--system` 作用域。
- 也可显式传入 `--user` 或 `--system` 覆盖默认作用域。

---

## NixOS 声明式模块 (`services.gymbag`)

在 NixOS 中，可通过 flake 导入 `nixosModules.default`：

```nix
{
  inputs.gymbag.url = "github:Evil-Project/gymbag";

  outputs = { nixpkgs, gymbag, ... }: {
    nixosConfigurations.my-server = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        gymbag.nixosModules.default
        {
          services.gymbag = {
            enable = true;
            openFirewall = true;
            configText = ''
              app.example.com {
                  reverse_proxy http://127.0.0.1:3000
                  verify 3
              }
            '';
          };
        }
      ];
    };
  };
}
```

该模块使用 `DynamicUser = true`、`StateDirectory = "gymbag"` 与 `CAP_NET_BIND_SERVICE` 安全绑定 `80`/`443` 端口。

---

## Arch Linux (`pacman` + `systemd`)

通过 Arch Linux `.pkg.tar.zst` 安装包（或 `packaging/arch/PKGBUILD`）安装后，默认包含 `/usr/bin/gymbag`（已赋 `cap_net_bind_service=+ep`）、`/etc/gymbag/Gymbagfile` 与 `/usr/lib/systemd/system/gymbag.service`：

```bash
sudo gymbag init -o /etc/gymbag/Gymbagfile
sudo systemctl enable --now gymbag
```

---

## 跨平台预览或生成单元文件 (`render`)

可在任意系统上生成任何目标平台的服务配置文件（例如在 macOS 上生成 Linux `systemd` 单元文件）：

```bash
# 输出 systemd 单元到标准输出
./gymbag service render --type systemd --system --service-user gymbag --config /etc/gymbag/Gymbagfile

# 导出到指定文件而不注册到本地
./gymbag service render --type systemd --system -o ./deploy/gymbag.service
```

---

## `gymbag service` 参数参考

| 参数 | 说明 |
| :--- | :--- |
| `-t`, `--type <mgr>` | `auto`（默认）、`systemd`、`launchd`、`openrc` 或 `supervisor` |
| `-n`, `--name <name>` | 服务名称（默认 `gymbag`） |
| `--label <id>` | `launchd` 反向 DNS 标签（默认 `io.gymbag.<name>`） |
| `-c`, `--config <path>` | `Gymbagfile`、`Caddyfile` 或 JSON 配置文件路径 |
| `--bin <path>` | `gymbag` 二进制路径（默认当前可执行文件） |
| `--workdir <dir>` | 服务工作目录（默认配置文件所在目录） |
| `--env-file <path>` | 可选 `.env` 文件（若 `<workdir>/.env` 存在则自动加载） |
| `--log-dir <dir>` | `launchd`、`openrc` 与 `supervisor` 的日志目录 |
| `-o`, `--output <path>` | 覆盖目标单元 / plist / 脚本输出路径 |
| `--service-user <user>` | 系统级服务运行用户 |
| `--service-group <grp>` | 系统级服务运行用户组 |
| `--user` / `--system` | 强制使用用户级或系统级作用域 |
| `--watch` | 向 `gymbag run` 传递 `--watch`（默认 `true`） |
| `--enable` | `install` 时设置开机自启（默认 `true`） |
| `--start`, `--now` | `install` 后立即启动服务 |
| `--no-reload` | 仅读写单元文件而不调用 `systemctl`/`launchctl`/`rc-update`/`supervisorctl` |
| `-f`, `--force` | 覆盖已存在的单元文件 |
