# ローカル開発 & 2層ローカル PKI

カスタムホスト名でのローカル HTTPS テストは、`mkcert` の導入や `/etc/hosts` の手動編集、ブラウザの警告画面が煩雑になりがちです。Gymbag は以下のローカルドメインが指定された場合、名前解決とローカル X.509 証明書発行の両方を自動で処理します：

- `localhost` または `*.localhost`
- `*.internal`（例：`app.internal`、`api.internal`）
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

## 自動 `/etc/hosts` 管理

`auto_hosts` が有効（既定）の場合、Gymbag は起動時に `/etc/hosts`（Windows は `C:\Windows\System32\drivers\etc\hosts`）を確認し、未登録の具体的なローカルドメインに対してインスタンス ID タグ付きの IPv4 / IPv6 ループバックエントリを追記します：

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

- 既存の `localhost` エントリには一切触れません。
- 終了時（`Ctrl+C` / `SIGTERM`）には `/etc/hosts` を再読み込みして実行中に行った手動編集を保持し、自身が追加した `# gymbag-managed:<instanceID>` 行**のみ**を削除します。
- `/etc/hosts` への書き込み権限がない場合は警告ログを出して**そのまま正常起動を継続**します。`dnsmasq` 等を利用する場合は `auto_hosts off`（または `GYMBAG_AUTO_HOSTS=off`）を設定してください。

## 2層ローカル認証局 (PKI)

`<cert_dir>/ca/`（既定 `certs/ca/`）配下に、2層構成の ECDSA P-256 PKI を自動構築します：

1. **Root CA**：`"Neuro-sama Headquarters Root CA"`（`certs/ca/neuro-sama-root-ca.pem`、有効期限10年、`MaxPathLen: 1`）
2. **Intermediate CA**：`"Gymbag & Swarm SSL Local CA"`（`certs/ca/gymbag-swarm-local-ca.pem`、有効期限5年、`MaxPathLen: 0`）
3. **Leaf Certificate**：`certs/local-fullchain.pem` + `certs/local-privkey.pem`（有効期限90日、残り30日以内で自動更新、DNS SAN に加え `127.0.0.1` と `::1` の IP SAN を付与）

### Root CA を1度だけ信頼する

`certs/ca/neuro-sama-root-ca.pem` を OS のトラストストアに1回登録するだけで、ブラウザと `curl` で警告なしのグリーンロック表示になります：

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

::: info Firefox での注意
macOS と Windows の Firefox では、`about:config` で `security.enterprise_roots.enabled` を `true` にすると OS のルート証明書を自動信頼します。Linux で Firefox が `p11-kit` ではなく独自の NSS ストアを使う場合は、**設定 → プライバシーとセキュリティ → 証明書 → 証明書を表示 → 認証局** から `certs/ca/neuro-sama-root-ca.pem` をインポートしてください。
:::
