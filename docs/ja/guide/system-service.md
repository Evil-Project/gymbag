# システムサービス設定

`gymbag service` は 4 つの init システムを対象に、OS バックグラウンドサービスの生成・登録・起動・停止・再起動・状態確認・削除を行います。

| マネージャー | 対象 OS / Init | デフォルトのユニットパス |
| :--- | :--- | :--- |
| `systemd` | Linux (Ubuntu, Debian, Fedora, Arch, RHEL) | `/etc/systemd/system/<name>.service`（`--user` 時は `~/.config/systemd/user/<name>.service`） |
| `launchd` | macOS (LaunchDaemons / LaunchAgents) | `/Library/LaunchDaemons/io.gymbag.<name>.plist`（`--user` 時は `~/Library/LaunchAgents/io.gymbag.<name>.plist`） |
| `openrc` | Linux (Alpine, Gentoo, Artix) | `/etc/init.d/<name>` |
| `supervisor` | Unix / コンテナ (`supervisord`) | `/etc/supervisor/conf.d/<name>.conf`（`--user` 時は `~/.config/supervisor/conf.d/<name>.conf`） |

## 2 コマンドでのサービス登録

`gymbag.json` または `Gymbagfile` を用意したら、次の 2 コマンドで登録・起動できます。

```bash
# 1. サービスを登録（systemd / launchd / openrc / supervisor を自動判定）
sudo ./gymbag service install --config /etc/gymbag/gymbag.json

# 2. 起動
sudo ./gymbag service start
```

設定生成とサービス登録を 1 回で済ませることもできます。

```bash
sudo ./gymbag init -d app.example.com -u :3000 -o /etc/gymbag/gymbag.json --service auto --system
sudo ./gymbag service start --system
```

::: tip `--watch` と `CAP_NET_BIND_SERVICE`
- `gymbag service install` はデフォルトで `--watch=true` になるため、設定を編集してもデーモンを再起動せずにルート、ファイアウォールルール、SNI 証明書を再読み込みします。
- システム全体の `systemd` ユニットで非 root の `--service-user` と特権ポート（`< 1024`、`80`・`443`・`22` および `verify_port < 1024` を含む）を使う場合は、`AmbientCapabilities=CAP_NET_BIND_SERVICE`、`CapabilityBoundingSet=CAP_NET_BIND_SERVICE`、`NoNewPrivileges=true` が設定されます。
- 非 root の `--service-user` を使う OpenRC ユニットには `capabilities="^cap_net_bind_service"` が付きます。
:::

## サービス管理サブコマンド

```bash
./gymbag service start
./gymbag service stop
./gymbag service restart
./gymbag service status
./gymbag service uninstall
```

### User スコープと System スコープ

- `sudo` なし（`euid != 0`）で実行すると、`systemd`、`launchd`、`supervisor` ではデフォルトで `--user` スコープ（`systemd --user`、`~/Library/LaunchAgents`）になります。
- `root` で実行すると、デフォルトで `--system` スコープ（`/etc/systemd/system`、`/Library/LaunchDaemons`、`/etc/init.d`）になります。
- `--user` または `--system` を明示すると、どちらかに固定できます。

## Debian / Ubuntu（`.deb` + `systemd`）

リリース版の `.deb` パッケージをインストールすると、`/usr/bin/gymbag`（`cap_net_bind_service=+ep` 付き）、`/etc/gymbag/Gymbagfile`（conffile）、`/lib/systemd/system/gymbag.service`（`DynamicUser=yes`、`StateDirectory=gymbag`、`GYMBAG_CERT_DIR=/var/lib/gymbag/certs`）が配置されます。

```bash
sudo apt install ./gymbag_<version>_linux_amd64.deb
sudo gymbag init -o /etc/gymbag/Gymbagfile --format gymbagfile
sudo systemctl enable --now gymbag
```

## Arch Linux（`pacman` + `systemd`）

`.pkg.tar.zst` パッケージをインストール（または `packaging/arch/PKGBUILD` からビルド）すると、`/usr/bin/gymbag`（`cap_net_bind_service=+ep` 付き）、`/etc/gymbag/Gymbagfile`、`/usr/lib/systemd/system/gymbag.service` が配置されます。

```bash
sudo gymbag init -o /etc/gymbag/Gymbagfile --format gymbagfile
sudo systemctl enable --now gymbag
```

## NixOS モジュール（`services.gymbag`）

NixOS では flake から `nixosModules.default` をインポートできます。

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

このモジュールは `DynamicUser = true`、`StateDirectory = "gymbag"`、`CAP_NET_BIND_SERVICE` で動作するため、root 権限なしでポート `80` と `443` を扱えます。

## クロスプラットフォームでのユニット生成（`render`）

`render` はどの OS 上からでも対応するすべての init システム向けのサービス定義を出力できます。たとえば macOS 上でデプロイ用の Linux `systemd` ユニットを生成する際に便利です。

```bash
./gymbag service render --type systemd --system --service-user gymbag --config /etc/gymbag/gymbag.json
./gymbag service render --type systemd --system -o ./deploy/gymbag.service
```

## `gymbag service` フラグ一覧

| フラグ | 説明 |
| :--- | :--- |
| `-t`, `--type <mgr>` | `auto`（デフォルト）、`systemd`、`launchd`、`openrc`、`supervisor` |
| `-n`, `--name <name>` | サービス名（デフォルト `gymbag`） |
| `--label <id>` | `launchd` 用ラベル（デフォルト `io.gymbag.<name>`） |
| `-c`, `--config <path>` | `gymbag.json`、`Gymbagfile`、または `Caddyfile` のパス |
| `--bin <path>` | `gymbag` バイナリのパス（デフォルトは現在の実行ファイル） |
| `--workdir <dir>` | 作業ディレクトリ（デフォルトは設定ファイルの親ディレクトリ） |
| `--env-file <path>` | 任意の `.env` ファイル（`<workdir>/.env` が存在すれば自動検出） |
| `--log-dir <dir>` | `launchd`、`openrc`、`supervisor` 用のログディレクトリ |
| `-o`, `--output <path>` | 出力先のユニット/plist/スクリプトパスを上書き |
| `--service-user <user>` | システムレベルサービスの実行ユーザー |
| `--service-group <grp>` | システムレベルサービスの実行グループ |
| `--user` / `--system` | ユーザー単位またはシステム全体のスコープを強制 |
| `--watch` | `gymbag run` に `--watch` を付与（デフォルト `true`） |
| `--enable` | `install` 時に自動起動を有効化（デフォルト `true`） |
| `--start`, `--now` | `install` 直後にサービスを起動 |
| `--no-reload` | `systemctl`/`launchctl`/`rc-update`/`supervisorctl` を呼ばずファイルのみ作成/削除 |
| `-f`, `--force` | 既存のサービスファイルを上書き |
