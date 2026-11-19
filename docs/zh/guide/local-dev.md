# 本地开发与两级本地 PKI

本地使用自定义域名调试 HTTPS 通常需要配置 `mkcert`、手动编辑 `/etc/hosts` 并忍受浏览器证书警告。当站点使用以下本地域名时，Gymbag 会自动完成本地域名解析和本地 X.509 证书签发：

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

## 自动管理 `/etc/hosts`

当 `auto_hosts` 开启时（默认开启），Gymbag 会在启动时检查 `/etc/hosts`（Windows 为 `C:\Windows\System32\drivers\etc\hosts`）。对于尚未解析的具体本地域名，会自动追加带实例 ID 标记的 IPv4 与 IPv6 回环记录：

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

- 现有的 `localhost` 条目保持不变。
- 退出时（`Ctrl+C` / `SIGTERM`），Gymbag 会重新读取 `/etc/hosts` 以保留您在运行期间的其他编辑，并**仅删除**自身添加的 `# gymbag-managed:<instanceID>` 行。
- 若启动时没有修改 `/etc/hosts` 的权限，Gymbag 会输出一条警告日志并**继续正常运行**。如果通过 `dnsmasq` 解析或手动管理 hosts，可设置 `auto_hosts off`（或 `GYMBAG_AUTO_HOSTS=off`）。

## 两级本地证书颁发机构 (PKI)

在 `<cert_dir>/ca/`（默认 `certs/ca/`）下，Gymbag 自动维护一套两级 ECDSA P-256 PKI：

1. **根证书 (Root CA)**：`"Neuro-sama Headquarters Root CA"`（`certs/ca/neuro-sama-root-ca.pem`，有效期 10 年，`MaxPathLen: 1`）
2. **中间证书 (Intermediate CA)**：`"Gymbag & Swarm SSL Local CA"`（`certs/ca/gymbag-swarm-local-ca.pem`，有效期 5 年，`MaxPathLen: 0`）
3. **叶子证书 (Leaf Cert)**：`certs/local-fullchain.pem` + `certs/local-privkey.pem`（90 天有效期，过期前 30 天内自动续期，包含 DNS SAN 以及 `127.0.0.1` 和 `::1` IP SAN）

### 一次性信任 Root CA

将 `certs/ca/neuro-sama-root-ca.pem` 添加到系统受信任根证书存储一次，浏览器和 `curl` 即可全部显示绿色安全锁：

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

::: info Firefox 注意事项
在 macOS 和 Windows 上，在 `about:config` 中开启 `security.enterprise_roots.enabled = true` 后 Firefox 会自动信任系统根证书。在 Linux 上，如果 Firefox 使用独立的 NSS 数据库而非 `p11-kit`，请在 **设置 → 隐私与安全 → 证书 → 查看证书 → 证书颁发机构** 中导入 `certs/ca/neuro-sama-root-ca.pem`。
:::
