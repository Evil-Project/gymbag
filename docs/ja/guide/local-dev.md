# ローカル開発 & 2層 PKI

独自のホスト名でローカル HTTPS を動かす場合、通常は `mkcert` の設定や `/etc/hosts` の手書き、ブラウザの証明書警告への対処が必要になります。Gymbag は次のドメインが設定に含まれると、hosts エントリと TLS 証明書の両方を自動で管理します。

- `localhost` または `*.localhost`
- `*.internal`（`app.internal`、`api.internal` など）
- `*.local` または `*.test`

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

## `/etc/hosts` の自動管理

`auto_hosts on`（デフォルト）が有効な場合、Gymbag は起動時に `/etc/hosts`（Windows では `C:\Windows\System32\drivers\etc\hosts`）を確認し、まだ名前解決できないローカルドメインに対してインスタンス ID タグ付きの IPv4・IPv6 ループバック行を追記します。

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

- 既存の `localhost` 行には手を加えません。
- `Ctrl+C` または `SIGTERM` で終了すると `/etc/hosts` を再読み込みするため、起動中に手動で追記した内容も消えずに残り、`# gymbag-managed:<instanceID>` の行だけが削除されます。
- root 権限がなく `/etc/hosts` に書き込めない場合は警告ログを出して起動を続けます。`.internal` を `dnsmasq` や手動で解決する場合は `auto_hosts off`（または `GYMBAG_AUTO_HOSTS=off`）を設定してください。

## 2 階層ローカル認証局 (PKI)

`<cert_dir>/ca/`（デフォルト `certs/ca/`）配下に 2 階層の ECDSA P-256 CA を保持します。

1. **Root CA**: `"Neuro-sama Headquarters Root CA"`（`certs/ca/neuro-sama-root-ca.pem`、有効期限 10 年、`MaxPathLen: 1`）
2. **Intermediate CA**: `"Gymbag & Swarm SSL Local CA"`（`certs/ca/gymbag-swarm-local-ca.pem`、有効期限 5 年、`MaxPathLen: 0`）
3. **Leaf 証明書**: `certs/local-fullchain.pem` + `certs/local-privkey.pem`（有効期限 90 日、残り 30 日以内で更新、DNS SAN に加え `127.0.0.1` と `::1` を付与）

### Root CA を一度だけ信頼ストアに登録する

`certs/ca/neuro-sama-root-ca.pem` を OS のトラストストアに登録しておけば、ブラウザも `curl` もすべてのローカルサイトを検証します。

::: code-group
```bash [macOS]
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain \
  certs/ca/neuro-sama-root-ca.pem
```

```bash [Debian / Ubuntu]
sudo cp certs/ca/neuro-sama-root-ca.pem /usr/local/share/ca-certificates/neuro-sama-root-ca.crt
sudo update-ca-certificates
```

```bash [Fedora / RHEL / Arch]
sudo trust anchor --store certs/ca/neuro-sama-root-ca.pem
```
:::

::: info Firefox の場合
macOS と Windows では `about:config` の `security.enterprise_roots.enabled` を `true` にすると OS のルート CA を読み込みます。Linux で `p11-kit` ではなく独自の NSS DB を使っている場合は、**設定 -> プライバシーとセキュリティ -> 証明書 -> 証明書を表示 -> 認証局** から `certs/ca/neuro-sama-root-ca.pem` を一度インポートしてください。
:::
