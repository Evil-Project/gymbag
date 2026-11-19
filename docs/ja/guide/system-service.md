# システムサービス設定

Gymbag には組み込みのサービスマネージャー（`gymbag service`）が同梱されており、以下の環境向けにバックグラウンドサービスの生成・登録・起動・停止・再起動・状態確認・アンインストールを行えます：

| マネージャー | 対象 OS / Init | 既定のユニットパス |
| :--- | :--- | :--- |
| `systemd` | Linux (Ubuntu, Debian, Fedora, Arch, RHEL) | `/etc/systemd/system/<name>.service`（`--user` 時は `~/.config/systemd/user/<name>.service`） |
| `launchd` | macOS (LaunchDaemons / LaunchAgents) | `/Library/LaunchDaemons/io.gymbag.<name>.plist`（`--user` 時は `~/Library/LaunchAgents/io.gymbag.<name>.plist`） |
| `openrc` | Linux (Alpine, Gentoo, Artix) | `/etc/init.d/<name>` |
| `supervisor` | Unix / コンテナ (`supervisord`) | `/etc/supervisor/conf.d/<name>.conf`（`--user` 時は `~/.config/supervisor/conf.d/<name>.conf`） |

---

## 2コマンド サービスワークフロー

`Gymbagfile` を作成済みの場合：

```bash
# 1. システムサービスを登録（systemd / launchd / openrc / supervisor を自動検出）
sudo ./gymbag service install --config /etc/gymbag/Gymbagfile

# 2. サービスを起動
sudo ./gymbag service start
```

設定生成と登録を1コマンドにまとめることもできます：

```bash
sudo ./gymbag init -d app.example.com -u :3000 -o /etc/gymbag/Gymbagfile --service auto --system
sudo ./gymbag service start --system
```

::: tip 自動 `--watch` と `CAP_NET_BIND_SERVICE`
- `gymbag service install` は既定で `gymbag run --config <path> --watch` を設定するため、`Gymbagfile` 編集時にプロセス再起動なしでルート・WAF ポリシー・ルールがホットリロードされます。
- 非 root の `--service-user` と特権ポート（`80`/`443`）でシステム全体 `systemd` ユニットを登録すると、`AmbientCapabilities=CAP_NET_BIND_SERVICE`、`CapabilityBoundingSet=CAP_NET_BIND_SERVICE`、`NoNewPrivileges=true` が自動付与されます。
- OpenRC ユニットでも非 root の `--service-user` 指定時は `capabilities="^cap_net_bind_service"` が自動設定されます。
:::

---

## ライフサイクル管理サブコマンド

```bash
# 起動・停止・再起動・状態確認
./gymbag service start
./gymbag service stop
./gymbag service restart
./gymbag service status

# サービス登録とユニットファイルを削除
./gymbag service uninstall
```

### ユーザー単位 vs システム全体スコープ

- 非 root（`euid != 0`）で実行した場合、`systemd`、`launchd`、`supervisor` は既定で `--user` スコープを使用します。
- `root`（`sudo`）で実行した場合は既定で `--system` スコープを使用します。
- `--user` または `--system` を明示指定して上書きできます。

---

## NixOS 宣言的モジュール (`services.gymbag`)

NixOS では flake から `nixosModules.default` をインポートして宣言的に構成できます：

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

`DynamicUser = true`、`StateDirectory = "gymbag"`、`CAP_NET_BIND_SERVICE` により root 権限なしで安全に `80`/`443` をバインドします。

---

## Arch Linux (`pacman` + `systemd`)

Arch Linux の `.pkg.tar.zst` パッケージ（または `packaging/arch/PKGBUILD`）では `/usr/bin/gymbag`（`cap_net_bind_service=+ep` 済み）、`/etc/gymbag/Gymbagfile`、`/usr/lib/systemd/system/gymbag.service` が導入されます：

```bash
sudo gymbag init -o /etc/gymbag/Gymbagfile
sudo systemctl enable --now gymbag
```

---

## クロスプラットフォームでのユニットファイル生成 (`render`)

任意の OS 上から他 OS 向けのサービス定義ファイルを生成できます（例：macOS 上で Linux 用 `systemd` ユニットを出力）：

```bash
# systemd ユニットを標準出力へプレビュー
./gymbag service render --type systemd --system --service-user gymbag --config /etc/gymbag/Gymbagfile

# ローカル登録せずに指定パスへ出力
./gymbag service render --type systemd --system -o ./deploy/gymbag.service
```

---

## `gymbag service` フラグ一覧

| フラグ | 説明 |
| :--- | :--- |
| `-t`, `--type <mgr>` | `auto`（既定）、`systemd`、`launchd`、`openrc`、`supervisor` |
| `-n`, `--name <name>` | サービス名（既定 `gymbag`） |
| `--label <id>` | `launchd` 用逆引き DNS ラベル（既定 `io.gymbag.<name>`） |
| `-c`, `--config <path>` | `Gymbagfile`、`Caddyfile`、JSON 設定ファイルパス |
| `--bin <path>` | `gymbag` バイナリパス（既定は現在の実行ファイル） |
| `--workdir <dir>` | 作業ディレクトリ（既定は設定ファイルの親ディレクトリ） |
| `--env-file <path>` | 任意の `.env` ファイル（`<workdir>/.env` があれば自動検出） |
| `--log-dir <dir>` | `launchd`、`openrc`、`supervisor` のログ出力先 |
| `-o`, `--output <path>` | 出力先ユニット / plist / スクリプトパスの上書き |
| `--service-user <user>` | システムレベルサービスの実行ユーザー |
| `--service-group <grp>` | システムレベルサービスの実行グループ |
| `--user` / `--system` | ユーザースコープまたはシステムスコープを強制 |
| `--watch` | `gymbag run` に `--watch` を付与（既定 `true`） |
| `--enable` | `install` 時に自動起動を有効化（既定 `true`） |
| `--start`, `--now` | `install` 直後にサービスを起動 |
| `--no-reload` | `systemctl`/`launchctl`/`rc-update`/`supervisorctl` を呼ばずファイルのみ操作 |
| `-f`, `--force` | 既存のユニット / plist / スクリプトを上書き |
