# 本地开发与双层 PKI

本地用自定义域名跑 HTTPS 时，通常要折腾 `mkcert`、手动改 `/etc/hosts`，还得反复点掉浏览器的证书警告。只要站点使用下面这些本地域名，Gymbag 就会自动接管 hosts 记录和本地 TLS 证书：

- `localhost` 或 `*.localhost`
- `*.internal`（如 `app.internal`、`api.internal`）
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

## 自动维护 `/etc/hosts`

在默认的 `auto_hosts on` 下，Gymbag 启动时会检查 `/etc/hosts`（Windows 为 `C:\Windows\System32\drivers\etc\hosts`）。对尚未出现在其中的本地域名，自动追加带实例 ID 标记的 IPv4 与 IPv6 回环记录：

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

- 已有的 `localhost` 行保持不变。
- 收到 `Ctrl+C` 或 `SIGTERM` 退出时，Gymbag 会重新读取 `/etc/hosts`（保留你在运行期间手动做的修改），只清理带有自身 `# gymbag-managed:<instanceID>` 标记的行。
- 如果当前进程没有 `/etc/hosts` 写入权限（非 root 运行），Gymbag 会打印一条警告然后继续启动。若你已经通过 `dnsmasq` 解析 `.internal` 或自行管理 hosts，设置 `auto_hosts off`（或 `GYMBAG_AUTO_HOSTS=off`）即可。

## 双层本地证书颁发机构（CA）

Gymbag 会在 `<cert_dir>/ca/`（默认为 `certs/ca/`）下维护一套双层 ECDSA P-256 CA：

1. **根证书（Root CA）**：`"Neuro-sama Headquarters Root CA"`（`certs/ca/neuro-sama-root-ca.pem`，有效期 10 年，`MaxPathLen: 1`）
2. **中间证书（Intermediate CA）**：`"Gymbag & Swarm SSL Local CA"`（`certs/ca/gymbag-swarm-local-ca.pem`，有效期 5 年，`MaxPathLen: 0`）
3. **叶子证书（Leaf Certificate）**：`certs/local-fullchain.pem` + `certs/local-privkey.pem`（有效期 90 天，剩余不足 30 天续期，包含 DNS SAN 以及 `127.0.0.1` 和 `::1`）

### 一次性信任根证书

在操作系统信任库安装一次 `certs/ca/neuro-sama-root-ca.pem`，之后浏览器和 `curl` 都会认可所有本地站点：

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

::: info Firefox 说明
在 macOS 和 Windows 上，只要在 `about:config` 里把 `security.enterprise_roots.enabled` 设为 `true`，Firefox 就会读取系统根证书。在 Linux 上，如果 Firefox 使用独立的 NSS 数据库而非 `p11-kit`，请在 **Settings -> Privacy & Security -> Certificates -> View Certificates -> Authorities** 中手动导入一次 `certs/ca/neuro-sama-root-ca.pem`。
:::
