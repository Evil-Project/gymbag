# 系統服務設定

`gymbag service` 支援橫跨七種 init 系統（涵蓋 Linux、macOS 與 BSD）的作業系統背景服務產生、安裝、啟動、停止、重啟、狀態查詢與移除功能：

| 服務管理器 | 目標作業系統 / Init | 預設服務設定檔路徑 |
| :--- | :--- | :--- |
| `systemd` | Linux（Ubuntu、Debian、Fedora、Arch、RHEL） | `/etc/systemd/system/<name>.service`（加上 `--user` 則為 `~/.config/systemd/user/<name>.service`） |
| `launchd` | macOS（LaunchDaemons / LaunchAgents） | `/Library/LaunchDaemons/io.gymbag.<name>.plist`（加上 `--user` 則為 `~/Library/LaunchAgents/io.gymbag.<name>.plist`） |
| `openrc` | Linux（Alpine、Gentoo、Artix） | `/etc/init.d/<name>` |
| `supervisor` | Unix / 容器（`supervisord`） | `/etc/supervisor/conf.d/<name>.conf`（加上 `--user` 則為 `~/.config/supervisor/conf.d/<name>.conf`） |
| `freebsd-rc` | FreeBSD / DragonFly BSD（`rc.d` + `sysrc`） | `/usr/local/etc/rc.d/<name>` |
| `openbsd-rc` | OpenBSD（`rc.d` + `rcctl`） | `/etc/rc.d/<name>` |
| `netbsd-rc` | NetBSD（`rc.d` + `/etc/rc.conf`） | `/etc/rc.d/<name>` |

## 兩道指令完成服務設定

準備好 `gymbag.json` 或 `Gymbagfile` 後即可執行：

```bash
# 1. 註冊系統服務（自動偵測 systemd / launchd / openrc / supervisor / BSD rc.d）
sudo ./gymbag service install --config /etc/gymbag/gymbag.json

# 2. 啟動服務
sudo ./gymbag service start
```

也可以在產生設定檔的同時一併安裝系統服務：

```bash
sudo ./gymbag init -d app.example.com -u :3000 -o /etc/gymbag/gymbag.json --service auto --system
sudo ./gymbag service start --system
```

::: tip `--watch` 與 `CAP_NET_BIND_SERVICE`
- `gymbag service install` 預設會啟用 `--watch=true`，因此修改設定檔後不需要重啟背景服務即可自動重載路由、防火牆規則與 SNI 憑證。
- 在系統層級的 `systemd` 服務中，若指定非 root 的 `--service-user` 且有使用特權連接埠（`< 1024`，包含 `80`、`443`、`22` 或 `verify_port < 1024`），系統會自動加上 `AmbientCapabilities=CAP_NET_BIND_SERVICE`、`CapabilityBoundingSet=CAP_NET_BIND_SERVICE` 與 `NoNewPrivileges=true`。
- 若為 OpenRC 服務且 `--service-user` 為非 root 使用者，則會自動設定 `capabilities="^cap_net_bind_service"`。
:::

## 服務子指令

```bash
./gymbag service start
./gymbag service stop
./gymbag service restart
./gymbag service status
./gymbag service uninstall
```

### 使用者層級（User）vs 系統層級（System）

- 未使用 `sudo`（`euid != 0`）執行時，`gymbag service` 在 `systemd`、`launchd` 與 `supervisor` 上預設會採用 `--user` 範圍（`systemd --user`、`~/Library/LaunchAgents`）。
- 以 `root` 身分執行時，預設會採用 `--system` 範圍（`/etc/systemd/system`、`/Library/LaunchDaemons`、`/etc/init.d`、`/usr/local/etc/rc.d`、`/etc/rc.d`）。
- BSD 的 `rc.d` 管理器（`freebsd-rc`、`openbsd-rc`、`netbsd-rc`）僅支援系統層級服務。
- 可透過傳入 `--user` 或 `--system` 明確指定範圍。

## BSD（`FreeBSD`、`OpenBSD`、`NetBSD`、`DragonFly BSD`）

`gymbag service install` 會自動偵測 FreeBSD/DragonFly（`freebsd-rc`）、OpenBSD（`openbsd-rc`）與 NetBSD（`netbsd-rc`）原生的 `rc.d` 管理器；獨立的參考範本亦收錄於 `packaging/bsd/`：

```bash
# FreeBSD / DragonFly BSD
sudo ./gymbag service install --config /usr/local/etc/gymbag/gymbag.json --start
# 會安裝 /usr/local/etc/rc.d/gymbag，執行 sysrc（或寫入 /etc/rc.conf）並執行 service gymbag onestart

# OpenBSD
doas ./gymbag service install --config /etc/gymbag/gymbag.json --start
# 會安裝 /etc/rc.d/gymbag，透過 rcctl enable/set flags 啟用並透過 rcctl start 啟動

# NetBSD
sudo ./gymbag service install --config /usr/pkg/etc/gymbag/gymbag.json --start
# 會安裝 /etc/rc.d/gymbag，於 /etc/rc.conf 寫入 gymbag=YES 並執行 /etc/rc.d/gymbag start
```

> **在 FreeBSD 上不以 root 權限綁定特權連接埠：** 可透過 `sysctl net.inet.ip.portrange.reservedhigh=0` 開放非 root 綁定低連接埠，或搭配專用服務帳號與 pf 重新導向。

## Debian / Ubuntu（`.deb` + `systemd`）

安裝釋出的 `.deb` 套件會自動配置 `/usr/bin/gymbag`（帶有 `cap_net_bind_service=+ep`）、`/etc/gymbag/Gymbagfile`（設為 conffile）以及 `/lib/systemd/system/gymbag.service`（已啟用 `DynamicUser=yes`、`StateDirectory=gymbag` 與 `GYMBAG_CERT_DIR=/var/lib/gymbag/certs`）：

```bash
sudo apt install ./gymbag_<version>_linux_amd64.deb
sudo gymbag init -o /etc/gymbag/Gymbagfile --format gymbagfile
sudo systemctl enable --now gymbag
```

## Arch Linux（`pacman` + `systemd`）

安裝 `.pkg.tar.zst` 套件（或從 `packaging/arch/PKGBUILD` 建置）會自動配置 `/usr/bin/gymbag`（帶有 `cap_net_bind_service=+ep`）、`/etc/gymbag/Gymbagfile` 與 `/usr/lib/systemd/system/gymbag.service`：

```bash
sudo gymbag init -o /etc/gymbag/Gymbagfile --format gymbagfile
sudo systemctl enable --now gymbag
```

## NixOS 模組（`services.gymbag`）

在 NixOS 上，可從 flake 匯入 `nixosModules.default`：

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

此模組會以 `DynamicUser = true`、`StateDirectory = "gymbag"` 與 `CAP_NET_BIND_SERVICE` 執行 Gymbag，無需 root 即可使用 `80` 與 `443` 連接埠。

## 跨平台產生服務設定檔（`render`）

`render` 可在任何作業系統上輸出任意支援 init 系統的服務定義檔——例如在 macOS 上先產生好 Linux `systemd` 單元檔再進行部署：

```bash
./gymbag service render --type systemd --system --service-user gymbag --config /etc/gymbag/gymbag.json
./gymbag service render --type systemd --system -o ./deploy/gymbag.service
```

## `gymbag service` 參數列表

| 參數 | 說明 |
| :--- | :--- |
| `-t`, `--type <mgr>` | `auto`（預設）、`systemd`、`launchd`、`openrc`、`supervisor`、`freebsd-rc`、`openbsd-rc` 或 `netbsd-rc` |
| `-n`, `--name <name>` | 服務名稱（預設 `gymbag`） |
| `--label <id>` | `launchd` 標籤（預設 `io.gymbag.<name>`） |
| `-c`, `--config <path>` | `gymbag.json`、`Gymbagfile` 或 `Caddyfile` 路徑 |
| `--bin <path>` | `gymbag` 執行檔路徑（預設為目前執行中的檔案） |
| `--workdir <dir>` | 工作目錄（預設為設定檔所在目錄） |
| `--env-file <path>` | 選用的 `.env` 檔案（若 `<workdir>/.env` 存在會自動偵測） |
| `--log-dir <dir>` | `launchd`、`openrc`、`supervisor`、`freebsd-rc`、`openbsd-rc` 與 `netbsd-rc` 的日誌目錄 |
| `-o`, `--output <path>` | 覆寫輸出的 unit/plist/script 目標路徑 |
| `--service-user <user>` | 系統層級服務的執行使用者 |
| `--service-group <grp>` | 系統層級服務的執行群組 |
| `--user` / `--system` | 強制指定使用者層級或系統層級範圍 |
| `--watch` | 傳遞 `--watch` 給 `gymbag run`（預設 `true`） |
| `--enable` | 在 `install` 時設定開機自動啟動（預設 `true`） |
| `--start`, `--now` | `install` 完成後立即啟動服務 |
| `--no-reload` | 僅寫入或移除設定檔，不執行服務管理器的重載或啟用指令 |
| `-f`, `--force` | 覆寫既有的服務設定檔 |
