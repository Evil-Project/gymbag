# 本機開發與雙層 PKI 架構

在本地開發環境使用自訂主機名稱的 HTTPS，通常得折騰 `mkcert`、手動編輯 `/etc/hosts`，還得一直點掉瀏覽器的憑證警告。只要網域屬於以下類型，Gymbag 就會自動幫你搞定 hosts 紀錄與 TLS 憑證：

- `localhost` 或 `*.localhost`
- `*.internal`（例如 `app.internal`、`api.internal`）
- `*.local` 或 `*.test`

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

## 自動管理 `/etc/hosts`

在 `auto_hosts on`（預設啟用）的狀態下，Gymbag 啟動時會檢查 `/etc/hosts`（Windows 為 `C:\Windows\System32\drivers\etc\hosts`）。任何尚未解析的本機網域都會自動寫入帶有執行實例 ID 標記的 IPv4 與 IPv6 本機迴環紀錄：

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

- 原有的 `localhost` 紀錄不會被更動。
- 按下 `Ctrl+C` 或收到 `SIGTERM` 時，Gymbag 會重新讀取 `/etc/hosts`，保留你在程式執行期間手動做的其他修改，僅移除屬於自己的 `# gymbag-managed:<instanceID>` 紀錄。
- 若因權限不足（非 root）而無法寫入 `/etc/hosts`，Gymbag 會印出警告並繼續執行。若你透過 `dnsmasq` 解析 `.internal` 或習慣自行管理檔案，請設定 `auto_hosts off`（或 `GYMBAG_AUTO_HOSTS=off`）。

## 雙層本機憑證機構（CA）

在 `<cert_dir>/ca/`（預設為 `certs/ca/`）目錄下，Gymbag 會維護一套雙層 ECDSA P-256 CA：

1. **根憑證（Root CA）**：`"Neuro-sama Headquarters Root CA"`（`certs/ca/neuro-sama-root-ca.pem`，效期 10 年，`MaxPathLen: 1`）
2. **中繼憑證（Intermediate CA）**：`"Gymbag & Swarm SSL Local CA"`（`certs/ca/gymbag-swarm-local-ca.pem`，效期 5 年，`MaxPathLen: 0`）
3. **終端憑證（Leaf Certificate）**：`certs/local-fullchain.pem` + `certs/local-privkey.pem`（效期 90 天，到期前 30 天內自動續期，SAN 包含所有 DNS 名稱以及 `127.0.0.1` 與 `::1`）

### 只需信任一次根憑證

將 `certs/ca/neuro-sama-root-ca.pem` 安裝到作業系統的信任憑證庫一次之後，瀏覽器與 `curl` 就能驗證所有本機站台：

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

::: info Firefox 注意事項
在 macOS 與 Windows 上，只要在 `about:config` 將 `security.enterprise_roots.enabled` 設為 `true`，Firefox 便會讀取作業系統的根憑證。在 Linux 上，若 Firefox 使用獨立的 NSS 資料庫而非 `p11-kit`，請至 **設定 -> 隱私權與安全性 -> 憑證 -> 檢視憑證 -> 憑證機構** 手動匯入一次 `certs/ca/neuro-sama-root-ca.pem`。
:::
