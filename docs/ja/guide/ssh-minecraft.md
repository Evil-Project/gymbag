# SSH & Minecraft 保護

Gymbag は HTTP/HTTPS だけでなく、OpenSSH と Minecraft Java Edition の手前にも同じ SHA-256 Proof-of-Work チャレンジ、レート制限、IP 脅威フィード、VPN/プロキシ/Tor 検出、カスタムルールを配置できます。

SSH や Minecraft のパケット内で直接 PoW を計算させるのではなく、Gymbag は短く手で入力しやすいワンタイム HTTPS リンク（`/v/<token>`、大文字・小文字を区別せず一致する紛らわしくない 8 文字の小文字トークン）を発行します。SSH ではプロトコルの切断画面に表示し、Minecraft 1.20.5+ では未認証プレイヤーをメモリ上の小さな空ワールド（ディスクには何も保存しません）に入れてチャットでクリック可能なリンクを送り、完了後に `Transfer` で実サーバーへ自動リダイレクトします（旧クライアントは切断画面へフォールバックします）。ユーザーがブラウザでそのリンクを開いてパズルを解くと、Gymbag はその IP を `clearance_ttl`（デフォルト `24h`）の間クリア済みとして登録し、以降の接続は生の TCP リレーでそのまま通します。

## SSH 保護

### 自動セットアップ（`sudo gymbag init -t ssh`）

`sudo ./gymbag init -t ssh` を実行すると、ホスト側の配線まで自動で行います。

1. `20000–59999` の範囲から未使用のループバックポートを 1 つ選びます。
2. `/etc/ssh/sshd_config` のバックアップを `/etc/ssh/sshd_config.gymbag.bak` に保存します（未作成の場合）。
3. 既存のトップレベル `Port` ディレクティブをコメントアウトし、最初の `Match` ブロックより前の `# gymbag-managed:start` / `# gymbag-managed:end` ブロックにランダムポートを書き込みます。
4. `sshd -t -f /etc/ssh/sshd_config` を実行し、検証に失敗した場合は元のファイルを復元して中断します。
5. `sshd` をリロードします（Linux 上の `systemctl`、`service`、または `rc-service` 経由）。
6. 稼働中の Linux ファイアウォール（`ufw`、`firewalld`、`nftables`、または `iptables`+`ip6tables`）を更新し、バックエンドポートへのループバック通信を許可しつつ外部からの直接アクセスを遮断し、SSH 待受ポート（`22`）と検証ポータルポート（`8443`）を開放します。
7. `gymbag.json`（`--format gymbagfile` 指定時は `Gymbagfile`）を書き出します。

::: warning 別の SSH ターミナルを開いたまま作業してください
リモートマシン上で `sshd_config` とファイアウォールルールを変更する間は、既存の root シェルを開いたまま Gymbag を起動し、ポート `22` 経由で新規ログインできることを確認してください。
:::

たとえば `sshd` がすでに `127.0.0.1:2222` で待ち受けていて、`sshd_config` やファイアウォールを変更したくない場合は、`--ssh-upstream 127.0.0.1:2222` や `--no-auto-setup`（ファイアウォール変更のみスキップするなら `--no-firewall`）を指定します。

```bash
sudo ./gymbag init -t ssh --ssh-upstream 127.0.0.1:2222
```

### SSH トラフィックの処理フロー

- **ファストパス**: `protect off` が設定されているか、クライアント IP がすでに有効なクリアランスを持っておりカスタムルールも未設定の場合、Gymbag は SSH バナーを一切読まずに TCP ストリームをリレーします。
- **不審な SSH 接続の検出**（検証レベル `1` および `3`）: クライアントの `SSH-2.0-...` 識別バナーを読み取り、空または `SSH-` 以外のプローブ、既知のスキャナーやスクリプト SSH ライブラリ（`libssh`、`paramiko`、`zgrab`、`masscan`、`nmap`、`asyncssh`、`ssh2go`、`jsch`、`scanner`、`bot`、`python`、`curl`、`ssh-2.0-go`、`hydra`、`medusa`、`patator`）、および脅威フィード・VPN/プロキシ/Tor の IP を検出します。
- **チャレンジ応答**: RFC 4253 `SSH_MSG_DISCONNECT` パケット（reason code `11`）を送り、SSH クライアントの標準出力に検証リンクをそのまま表示します。

## Minecraft Java 保護

### 管理起動（`--server-jar`）と既存サーバー（`--server-properties`）

Gymbag 自身に Minecraft プロセスを監視・起動させたい場合は、`--server-jar` を指定します。

```bash
./gymbag init -t minecraft --server-jar /srv/mc/paper.jar --mc-jvm-args "-Xms1G -Xmx4G"
./gymbag
```

管理起動モードでは、Gymbag が以下を行います。

1. **互換 Java ランタイムの自動選択**: `version.json`、バンドラー内のネストされた JAR、`patch.properties`、`.class` バイトコードヘッダを検査して、レガシーな Java 8 サーバーとモダンな Java 17/21+ サーバーを見分け、ホスト上の一致する `java` バイナリを選択します（`--mc-java` / `java_path` で上書き可能）。
2. **`server.properties` の安全な配線**: `20000–59999` の未使用ループバックポートを 1 つ選び、`server.properties` がまだ存在しなければ `server-ip=127.0.0.1` 付きで新規作成し（既存ファイルがある場合はバックアップを取って更新）、`upstream` を自動設定します。
3. **子プロセスの管理**: 指定した `jvm_args` と `server_args` を付けて JAR を起動し、クラッシュ時はデフォルトで自動再起動し（`auto_restart`）、Gymbag 終了時にはコンソールへ `stop` コマンドを送って安全に停止します（タイムアウト 15 秒）。

すでに Minecraft サーバーを別で稼働させている場合（systemd、tmux、screen、Pterodactyl など）は、代わりに `server.properties` を指定して `init` を実行します。

```bash
./gymbag init -t minecraft --server-properties /srv/mc/server.properties
./gymbag
```

既存の `server.properties` に対して、`init` は次を実行します。

1. `20000–59999` から未使用ポートを選び、`server.properties` を `server.properties.gymbag.bak` にバックアップします。
2. コメントや他の設定行を残したまま `server-port=<random>` を更新します。
3. 空欄、`0.0.0.0`、または `::` になっている `server-ip` 行を `server-ip=127.0.0.1` に書き換え、プレイヤーがバックエンドポートへ直接迂回接続できないようにします（ループバック以外のアドレスを設定していた場合は、外部ファイアウォールでもそのポートを閉じてください）。
4. 生成したアップストリーム設定を `gymbag.json` または `Gymbagfile` に保存します。

`server.properties` を手動管理したい場合は `--mc-upstream 127.0.0.1:25566` または `--no-auto-setup` を渡してください。

### Minecraft トラフィックの処理フロー

- **Server List Ping の透過**: 新形式のステータスハンドシェイク（`nextState == 1`）も 1.6 の旧形式 Ping（`0xFE`）も、カスタムルールや `ip_security_action block` で遮断されない限りアップストリームへ転送します。MOTD、サーバーアイコン、プレイヤー数表示はそのまま動作します。
- **Login & Transfer のゲーティング（`nextState == 2` / `3`）**: ハンドシェイクと Login Start のユーザー名を検査します。不正なプロトコルバージョン（`<= 0`）、破損したログインパケット、不正なユーザー名（空文字、32 文字超、制御文字を含むもの）、脅威フィード IP、VPN/プロキシ/Tor IP はレベル `1` および `3` で不審（suspicious）として扱われます。
- **メモリ上の空ワールドでの検証と自動転送（1.20.5+）**: 対応するモダンな Java クライアント（`1.20.5`–`26.1`）では、未認証のプレイヤーを純粋なメモリ上の小さな空ワールド（ワールドファイルは一切ディスクに保存されません）の小さな石の足場へスポーンさせ、ゲーム内チャットにクリック可能な検証リンクを直接送信します。プレイヤーがブラウザでチェックを完了すると、Gymbag は Minecraft の `Transfer` パケットを送信して実サーバーへ自動リダイレクトします（`accepts-transfers=false` のアップストリームでも受け付けられるよう、`nextState=3` を `2` へ透過的に書き換えます）。
- **切断画面フォールバック（旧バージョンまたはカスタムクライアント）**: `1.20.5` 未満のクライアント、未知の将来プロトコル、または組み込み Known Packs を拒否するクライアントには、**「Connection Lost」**ダイアログ内にクリックでクリップボードへコピーできる検証 URL（1.21.5+ では `click_event`/`hover_event`、1.21.4 以前では `clickEvent`/`hoverEvent`）を含めた Minecraft Login/Configuration Disconnect パケットを返します。

## HTTP・SSH・Minecraft の同時運用

1 つの Gymbag プロセスで 3 つのリスナーを同時に動かせる設定例です。

```bash
sudo ./gymbag init -t http,ssh,minecraft \
  -d app.example.com -u http://localhost:3000 \
  --server-jar /srv/mc/paper.jar
sudo ./gymbag
```

`Gymbagfile` 形式での記述例:

```caddyfile
{
    email ops@example.com
    verify_url https://verify.example.com:8443
    verify_port 8443
}

app.example.com {
    reverse_proxy http://localhost:3000
    verify 3
}

ssh {
    port 22
    upstream 127.0.0.1:41822
    verify 3
    difficulty 4
    rate_limit 60 60s
    allow ip 10.0.0.0/8
}

minecraft {
    port 25565
    upstream 127.0.0.1:48291
    server_jar /srv/mc/paper.jar
    jvm_args -Xms1G -Xmx4G
    server_properties /srv/mc/server.properties
    verify 3
    rate_limit 60 60s
}
```

同等の `gymbag.json`:

```json
{
  "sites": [
    {
      "domain": "app.example.com",
      "upstreams": ["http://localhost:3000"],
      "waf": {"verify_level": 3}
    }
  ],
  "ssh": {
    "port": 22,
    "upstream": "127.0.0.1:41822",
    "verify_level": 3,
    "rules": [
      {"action": "allow", "ips": ["10.0.0.0/8"]}
    ]
  },
  "minecraft": {
    "port": 25565,
    "upstream": "127.0.0.1:48291",
    "server_jar": "/srv/mc/paper.jar",
    "jvm_args": ["-Xms1G", "-Xmx4G"],
    "server_properties": "/srv/mc/server.properties",
    "verify_level": 3
  },
  "verify": {
    "url": "https://verify.example.com:8443",
    "port": 8443
  },
  "tls": {
    "acme_email": "ops@example.com"
  }
}
```

## 共有 HTTPS 検証ポータルと IP SSL 証明書

デフォルトでは、SSH と Minecraft はポート `8443`（`verify_port` / `--verify-port` / `GYMBAG_VERIFY_PORT`）上の HTTPS リスナーを共有します。

- **ポート `443` の共有**: HTTP サイトがすでに有効な状態で `verify_port 443`（`https_port` と同じ値）を設定すると、Gymbag は別ポートを開かずメインの HTTPS リスナー上で `/v/*`、`/verify/*`、`/.waf/gate/verify` を提供します。
- **独自ドメイン vs サーバー IP 自動検出**: DNS ホスト名を使う場合は `verify_url https://verify.example.com:8443` を設定します。`verify_url` が空欄の場合、Gymbag はローカルネットワークインターフェースからサーバーのパブリック IP を選びます（見つからない場合は 1.5 秒のタイムアウト付きで `https://api.ipify.org` を参照）。
- **IP SSL 証明書 (RFC 8738)**: ACME 経由で IP アドレスのままポータルを配信する場合、Gymbag は RFC 8738 IP HTTP-01 証明書を要求し（CA のディレクトリが対応していれば `shortlived` プロファイルを自動選択）、`certs/verify-acme-fullchain.pem`（Let's Encrypt 以外の CA では `verify-acme-<provider>-fullchain.pem`）にキャッシュします。ポート `80` が使えない、あるいは CA が IP 識別子を発行しない場合は、ローカル CA（`certs/verify-local-fullchain.pem`）または自己署名 IP 証明書（`certs/verify-selfsigned.pem`）へフォールバックします。`verify_server { tls <cert> <key> }` で手動指定の証明書ペアを渡すこともできます。

## デフォルト値・レート制限・Redis

- `ssh` と `minecraft` ブロックのデフォルト `verify` レベルは `3`（`suspicious & ai`）です。
- SSH と Minecraft のデフォルト `rate_limit` は `60` 秒あたり `60` 回です（Web サイトのデフォルトは `60` 秒あたり `240` 回）。
- `{ ... }` 内に定義したグローバルカスタムルールは、`ssh` と `minecraft` でも各サービス固有ルールより先に評価されます（IP/CIDR と、クライアントバナー・ユーザー名に対する `user_agent` マッチングに対応）。
- `redis` / `GYMBAG_REDIS_URL` を設定すると、検証チケット（`gymbag:ticket:<token>`）とクリア済み IP（`gymbag:clearance:<service>:<ip>`）が Redis に保存され、再起動や複数インスタンス間でもクリア状態が維持されます。
