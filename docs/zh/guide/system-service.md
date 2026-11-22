# 系统服务安装与管理

`gymbag service` 支持在四种初始化系统上生成、安装、启停、重启、查看状态和卸载操作系统后台服务：

| 管理器 | 目标系统 / Init | 默认单元文件路径 |
| :--- | :--- | :--- |
| `systemd` | Linux（Ubuntu、Debian、Fedora、Arch、RHEL） | `/etc/systemd/system/<name>.service`（传入 `--user` 时为 `~/.config/systemd/user/<name>.service`） |
| `launchd` | macOS（LaunchDaemons / LaunchAgents） | `/Library/LaunchDaemons/io.gymbag.<name>.plist`（传入 `--user` 时为 `~/Library/LaunchAgents/io.gymbag.<name>.plist`） |
| `openrc` | Linux（Alpine、Gentoo、Artix） | `/etc/init.d/<name>` |
| `supervisor` | Unix / 容器（`supervisord`） | `/etc/supervisor/conf.d/<name>.conf`（传入 `--user` 时为 `~/.config/supervisor/conf.d/<name>.conf`） |

## 两条命令注册并启动服务

准备好 `gymbag.json` 或 `Gymbagfile` 后：

```bash
# 1. 注册服务（自动识别 systemd / launchd / openrc / supervisor）
sudo ./gymbag service install --config /etc/gymbag/gymbag.json

# 2. 启动服务
sudo ./gymbag service start
```

也可以在生成配置的同时完成注册：

```bash
sudo ./gymbag init -d app.example.com -u :3000 -o /etc/gymbag/gymbag.json --service auto --system
sudo ./gymbag service start --system
```

::: tip `--watch` 与 `CAP_NET_BIND_SERVICE`
- `gymbag service install` 默认启用 `--watch=true`，修改配置文件会自动重载路由、防火墙规则和 SNI 证书，无需重启进程。
- 当系统级 `systemd` unit 指定了非 root 的 `--service-user` 且监听特权端口（`< 1024`，包括 `80`、`443`、`22` 或 `< 1024` 的 `verify_port`）时，会自动生成 `AmbientCapabilities=CAP_NET_BIND_SERVICE`、`CapabilityBoundingSet=CAP_NET_BIND_SERVICE` 和 `NoNewPrivileges=true`。
- 在 OpenRC 下使用非 root `--service-user` 时，会写入 `capabilities="^cap_net_bind_service"`。
:::

## 服务子命令

```bash
./gymbag service start
./gymbag service stop
./gymbag service restart
./gymbag service status
./gymbag service uninstall
```

### 用户级与系统级作用域

- 不带 `sudo`（`euid != 0`）运行时，`gymbag service` 在 `systemd`、`launchd` 和 `supervisor` 下默认使用 `--user` 作用域（`systemd --user`、`~/Library/LaunchAgents`）。
- 以 `root` 运行时默认使用 `--system` 作用域（`/etc/systemd/system`、`/Library/LaunchDaemons`、`/etc/init.d`）。
- 可显式传入 `--user` 或 `--system` 指定作用域。

## Debian / Ubuntu（`.deb` + `systemd`）

安装发行版 `.deb` 包会自动部署 `/usr/bin/gymbag`（附带 `cap_net_bind_service=+ep`）、`/etc/gymbag/Gymbagfile`（标记为 conffile）以及 `/lib/systemd/system/gymbag.service`（配置了 `DynamicUser=yes`、`StateDirectory=gymbag`、`GYMBAG_CERT_DIR=/var/lib/gymbag/certs`）：

```bash
sudo apt install ./gymbag_<version>_linux_amd64.deb
sudo gymbag init -o /etc/gymbag/Gymbagfile --format gymbagfile
sudo systemctl enable --now gymbag
```

## Arch Linux（`pacman` + `systemd`）

安装 `.pkg.tar.zst` 包（或通过 `packaging/arch/PKGBUILD` 构建）会部署 `/usr/bin/gymbag`（附带 `cap_net_bind_service=+ep`）、`/etc/gymbag/Gymbagfile` 以及 `/usr/lib/systemd/system/gymbag.service`：

```bash
sudo gymbag init -o /etc/gymbag/Gymbagfile --format gymbagfile
sudo systemctl enable --now gymbag
```

## NixOS 模块（`services.gymbag`）

在 NixOS 上直接从 flake 引入 `nixosModules.default`：

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

该模块默认启用 `DynamicUser = true`、`StateDirectory = "gymbag"` 与 `CAP_NET_BIND_SERVICE`，无需 root 即可监听 `80` 和 `443` 端口。

## 跨平台生成服务单元文件（`render`）

`render` 可以在任意操作系统上输出指定 init 系统的服务定义文件，适合在 macOS 开发机上直接生成 Linux `systemd` unit 文件用于部署：

```bash
./gymbag service render --type systemd --system --service-user gymbag --config /etc/gymbag/gymbag.json
./gymbag service render --type systemd --system -o ./deploy/gymbag.service
```

## `gymbag service` 参数

| 参数 | 说明 |
| :--- | :--- |
| `-t`, `--type <mgr>` | `auto`（默认）、`systemd`、`launchd`、`openrc` 或 `supervisor` |
| `-n`, `--name <name>` | 服务名称（默认 `gymbag`） |
| `--label <id>` | `launchd` 标签（默认 `io.gymbag.<name>`） |
| `-c`, `--config <path>` | `gymbag.json`、`Gymbagfile` 或 `Caddyfile` 路径 |
| `--bin <path>` | `gymbag` 可执行文件路径（默认为当前可执行文件） |
| `--workdir <dir>` | 工作目录（默认为配置文件所在目录） |
| `--env-file <path>` | 可选 `.env` 文件（若 `<workdir>/.env` 存在则自动加载） |
| `--log-dir <dir>` | `launchd`、`openrc` 与 `supervisor` 使用的日志目录 |
| `-o`, `--output <path>` | 覆盖生成 unit/plist/script 的输出路径 |
| `--service-user <user>` | 系统级服务运行用户 |
| `--service-group <grp>` | 系统级服务运行用户组 |
| `--user` / `--system` | 强制使用用户级或系统级作用域 |
| `--watch` | 向 `gymbag run` 传递 `--watch`（默认 `true`） |
| `--enable` | `install` 时设置开机自启（默认 `true`） |
| `--start`, `--now` | `install` 完成后立即启动服务 |
| `--no-reload` | 仅写/删 unit 文件，不调用 `systemctl`/`launchctl`/`rc-update`/`supervisorctl` |
| `-f`, `--force` | 覆盖已存在的服务文件 |
