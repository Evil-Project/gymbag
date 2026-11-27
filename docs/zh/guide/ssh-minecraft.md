# SSH 与 Minecraft 防护

除了 HTTP/HTTPS，Gymbag 还可以把 OpenSSH 和 Minecraft Java 版服务器放到同一套 SHA-256 工作量证明质询、限流器、IP 威胁情报、VPN/代理/Tor 检测和自定义规则后面。

Gymbag 不要求客户端在 SSH 或 Minecraft 协议数据包里直接计算 PoW，而是在未通过校验时下发简短、便于手动输入的一次性 HTTPS 链接（`/v/<token>`，8 位不易混淆的小写字符，匹配时不区分大小写）：SSH 会将其打印在断开提示界面里，而 Minecraft 1.20.5+ 会把未验证玩家送入纯内存微型空世界（不在磁盘保存任何文件），在聊天栏发送可点击链接，并在验证完成后通过 `Transfer` 自动重定向回真实服务器（旧版客户端回退到断开提示界面）。用户在浏览器打开该链接并解出题目后，Gymbag 会为该 IP 记录有效期为 `clearance_ttl`（默认 `24h`）的放行状态，后续连接直接通过原始 TCP 转发直连后端。

## SSH 防护

### 自动配置（`sudo gymbag init -t ssh`）

运行 `sudo ./gymbag init -t ssh` 会自动完成主机上的端口和防火墙调整：

1. 在 `20000–59999` 范围内选择一个未占用的回环端口。
2. 将 `/etc/ssh/sshd_config` 备份至 `/etc/ssh/sshd_config.gymbag.bak`（如果尚未备份）。
3. 注释掉顶层现有的 `Port` 指令，并在首个 `Match` 块之前写入包含随机端口的 `# gymbag-managed:start` / `# gymbag-managed:end` 块。
4. 执行 `sshd -t -f /etc/ssh/sshd_config`。若校验失败，立即恢复原文件并退出。
5. 重载 `sshd`（在 Linux 上依次尝试 `systemctl`、`service` 或 `rc-service`）。
6. 更新当前启用的 Linux 防火墙（`ufw`、`firewalld`、`nftables` 或 `iptables`+`ip6tables`），仅允许回环流量访问后端端口，丢弃外部直接打到后端端口的流量，并保持 SSH 监听端口（`22`）和验证门户端口（`8443`）开放。
7. 写入 `gymbag.json`（传入 `--format gymbagfile` 则写入 `Gymbagfile`）。

::: warning 保留第二个 SSH 终端连接
在远程服务器上修改 `sshd_config` 和防火墙规则时，请务必保留一个已登录的 root shell 不要关闭，直到你启动 Gymbag 并确认能正常通过 `22` 端口登录为止。
:::

如果不想修改 `sshd_config` 或防火墙规则（例如 `sshd` 已经手动监听在 `127.0.0.1:2222`），可传入 `--ssh-upstream 127.0.0.1:2222` 或 `--no-auto-setup`（仅跳过防火墙变更则传 `--no-firewall`）：

```bash
sudo ./gymbag init -t ssh --ssh-upstream 127.0.0.1:2222
```

### SSH 流量处理流程

- **快速路径**：当设置了 `protect off` 或客户端 IP 已持有有效放行记录（且没有配置自定义规则）时，Gymbag 直接透传 TCP 流，连 SSH banner 都不读取。
- **可疑 SSH 检测**（验证级别 `1` 和 `3`）：Gymbag 会读取客户端发送的 `SSH-2.0-...` 标识 banner，识别空探测或非 `SSH-` 开头的连接，以及常见扫描器和脚本化 SSH 库（`libssh`、`paramiko`、`zgrab`、`masscan`、`nmap`、`asyncssh`、`ssh2go`、`jsch`、`scanner`、`bot`、`python`、`curl`、`ssh-2.0-go`、`hydra`、`medusa`、`patator`），同时结合威胁情报 IP 与 VPN/代理/Tor IP 进行判定。
- **质询响应**：返回 RFC 4253 `SSH_MSG_DISCONNECT` 数据包（reason code `11`），把验证链接直接打印在用户的 SSH 客户端输出中。

## Minecraft Java 防护

### 托管启动（`--server-jar`）与既有服务端（`--server-properties`）

如果希望由 Gymbag 直接托管 Minecraft 进程，可传入 `--server-jar`：

```bash
./gymbag init -t minecraft --server-jar /srv/mc/paper.jar --mc-jvm-args "-Xms1G -Xmx4G"
./gymbag
```

在托管模式下，Gymbag 会：

1. **自动选择兼容的 Java 运行时**：通过检查 `version.json`、bundler 嵌套 JAR、`patch.properties` 或 `.class` 字节码版本头来区分传统 Java 8 服务端与现代 Java 17/21+ 服务端，并在主机上挑选匹配的 `java` 可执行文件（可通过 `--mc-java` / `java_path` 覆盖）。
2. **安全接管 `server.properties`**：在 `20000–59999` 范围内挑选未占用的回环端口；若 `server.properties` 尚不存在则自动创建并写入 `server-ip=127.0.0.1`（若已存在则先备份再更新），并自动写入 `upstream`。
3. **管理子进程生命周期**：使用你配置的 `jvm_args` 和 `server_args` 启动 JAR；默认在崩溃时自动重启（`auto_restart`），并在 Gymbag 关闭时向控制台发送优雅 `stop` 指令（15 秒超时）。

如果服务器已经由 systemd、tmux、screen 或 Pterodactyl 等方式单独运行，只需将 `init` 指向其 `server.properties`：

```bash
./gymbag init -t minecraft --server-properties /srv/mc/server.properties
./gymbag
```

针对既有的 `server.properties`，`init` 会：

1. 在 `20000–59999` 范围内挑选空闲端口，并将 `server.properties` 备份为 `server.properties.gymbag.bak`。
2. 更新 `server-port=<random>`，同时保留原有注释和其他配置项。
3. 把空值、`0.0.0.0` 或 `::` 的 `server-ip` 改写为 `server-ip=127.0.0.1`，防止玩家绕过 Gymbag 直连后端端口（如果之前绑定了非回环地址，请同时确保外部防火墙屏蔽了该端口）。
4. 将生成的上游地址写入 `gymbag.json` 或 `Gymbagfile`。

如需自行管理 `server.properties`，可传入 `--mc-upstream 127.0.0.1:25566` 或 `--no-auto-setup`。

### Minecraft 流量处理流程

- **服务器列表 Ping 透传**：新版状态握手（`nextState == 1`）和旧版 1.6 Ping（`0xFE`）默认都会转发给上游服务器（除非被规则或 `ip_security_action block` 拦截），因此服务器的 MOTD、图标和在线人数都能正常显示。
- **登录与转移拦截（`nextState == 2` / `3`）**：Gymbag 会检查握手包与 Login Start 用户名。无效协议号（`<= 0`）、畸形登录包、非法用户名（为空、超过 32 个字符或包含控制字符）、威胁情报命中 IP 以及 VPN/代理/Tor IP 在级别 `1` 和 `3` 下均判定为可疑。
- **纯内存空世界质询与自动转移（1.20.5+）**：对于受支持的现代 Java 客户端（`1.20.5`–`26.1`），Gymbag 会将未验证玩家放入纯内存的微型空世界（不在磁盘保存任何世界文件），脚下有一块小型石头平台，并在游戏内聊天栏直接发送可点击的验证链接。玩家在浏览器完成验证后，Gymbag 会发送 Minecraft `Transfer` 数据包将其自动重定向到真实服务器（并透明地将 `nextState=3` 改写为 `2`，使启用 `accepts-transfers=false` 的上游也能正常放行）。
- **断开界面回退（旧版或自定义客户端）**：低于 `1.20.5` 的客户端、未知的未来协议版本或拒绝内置 Known Packs 的客户端，会收到包含可点击复制到剪贴板验证 URL（1.21.5+ 使用 `click_event`/`hover_event`，1.21.4 及更早版本使用 `clickEvent`/`hoverEvent`）的 Minecraft Login/Configuration Disconnect 数据包，展示在 **“连接已丢失（Connection Lost）”** 弹窗中。

## 同时运行 HTTP、SSH 和 Minecraft

单个 Gymbag 进程可以同时监听这三类流量：

```bash
sudo ./gymbag init -t http,ssh,minecraft \
  -d app.example.com -u http://localhost:3000 \
  --server-jar /srv/mc/paper.jar
sudo ./gymbag
```

`Gymbagfile` 写法：

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

对应的 `gymbag.json`：

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

## 共享 HTTPS 验证门户与 IP SSL 证书

默认情况下，SSH 和 Minecraft 共用一个监听在 `8443` 端口上的 HTTPS 服务（`verify_port` / `--verify-port` / `GYMBAG_VERIFY_PORT`）。

- **复用 `443` 端口**：如果你已经配置了 HTTP 站点并且设置 `verify_port 443`（与 `https_port` 一致），Gymbag 会直接在主 HTTPS 监听器上提供 `/v/*`、`/verify/*` 和 `/.waf/gate/verify`，不再额外开启端口。
- **自定义域名 vs 自动探测服务器 IP**：设置 `verify_url https://verify.example.com:8443` 可绑定 DNS 域名。若 `verify_url` 留空，Gymbag 会从本机网卡获取公网 IP（若无公网地址则以 1.5s 超时回退请求 `https://api.ipify.org`）。
- **IP SSL 证书（RFC 8738）**：通过 IP 走 ACME 提供验证门户时，Gymbag 会向 CA 申请 RFC 8738 IP HTTP-01 证书（若 ACME 目录支持则自动请求 `shortlived` profile），并缓存在 `certs/verify-acme-fullchain.pem`（非 Let's Encrypt CA 为 `verify-acme-<provider>-fullchain.pem`）。若 `80` 端口不可达或 CA 不支持 IP 签发，则回退到 Gymbag 本地 CA（`certs/verify-local-fullchain.pem`）或自签名 IP 证书（`certs/verify-selfsigned.pem`）；也可以用 `verify_server { tls <cert> <key> }` 指定自有证书。

## 默认值、限流与 Redis

- `ssh` 与 `minecraft` 块的默认 `verify` 级别为 `3`（`suspicious & ai`）。
- `ssh` 与 `minecraft` 的默认 `rate_limit` 为每 `60` 秒 `60` 次连接（HTTP 站点默认为每 `60` 秒 `240` 次）。
- `{ ... }` 中定义的全局自定义规则会先于服务内规则在 SSH 和 Minecraft 上执行（支持按 IP/CIDR 匹配，并可用 `user_agent` 匹配客户端 banner 或用户名）。
- 配置 `redis` / `GYMBAG_REDIS_URL` 后，验证凭证（`gymbag:ticket:<token>`）与放行 IP（`gymbag:clearance:<service>:<ip>`）会写入 Redis，重启不丢失且支持多实例共享。
