# SSH & Minecraft Protection

Beyond HTTP/HTTPS, Gymbag can gate OpenSSH and Minecraft Java Edition behind the same SHA-256 proof-of-work challenge, rate limiter, IP threat feeds, VPN/proxy/Tor detection, and custom rules.

Instead of requiring clients to solve PoW inside SSH or Minecraft packets, Gymbag issues a short, human-typable one-time HTTPS link (`/v/<token>`, 8 unambiguous lowercase characters matched case-insensitively): SSH returns it in the disconnect screen, while Minecraft 1.20.5+ places unverified players into a tiny in-memory empty world (nothing saved to disk), sends the clickable link in chat, and automatically redirects them to the real server via `Transfer` once completed (older clients fall back to the disconnect screen). Once the user opens that link in a browser and solves the puzzle, Gymbag clears their IP for `clearance_ttl` (default `24h`) and subsequent connections pass straight through over a raw TCP relay.

## SSH Protection

### Automated Setup (`sudo gymbag init -t ssh`)

Running `sudo ./gymbag init -t ssh` handles the host wiring automatically:

1. Picks an unused loopback port in `20000–59999`.
2. Saves a backup of `/etc/ssh/sshd_config` to `/etc/ssh/sshd_config.gymbag.bak` (if one does not exist yet).
3. Comments out existing top-level `Port` directives and writes the random port inside a `# gymbag-managed:start` / `# gymbag-managed:end` block before the first `Match` block.
4. Runs `sshd -t -f /etc/ssh/sshd_config`. If validation fails, it restores the original file and aborts.
5. Reloads `sshd` (`systemctl`/`service`/`rc-service` on Linux; `service sshd reload` on FreeBSD/DragonFly; `rcctl reload sshd` on OpenBSD; `/etc/rc.d/sshd reload` on NetBSD).
6. Updates the active Linux (`ufw`, `firewalld`, `nftables`, `iptables`+`ip6tables`) or BSD firewall (`pf` via `gymbag_ssh` anchor in `/etc/pf.conf`, FreeBSD `ipfw` set 29, or NetBSD `npf`) to allow loopback traffic to the backend port, drop external traffic hitting the backend port directly, and keep the SSH listen port (`22`) and verify portal port (`8443`) open.
7. Writes `gymbag.json` (or `Gymbagfile` with `--format gymbagfile`).

::: warning Keep a Second SSH Terminal Open
Whenever `sshd_config` and firewall rules are changed on a remote machine, keep an existing root shell open while you start Gymbag and verify you can log in through port `22`.
:::

To skip modifying `sshd_config` or firewall rules—for example when `sshd` already listens on `127.0.0.1:2222`—pass `--ssh-upstream 127.0.0.1:2222` or `--no-auto-setup` (or `--no-firewall` to only skip firewall changes):

```bash
sudo ./gymbag init -t ssh --ssh-upstream 127.0.0.1:2222
```

### How SSH Traffic Is Handled

- **Fast path**: When `protect off` is set or the client IP already has a valid clearance (and no custom rules are configured), Gymbag relays the TCP stream without reading the SSH banner at all.
- **Suspicious SSH detection** (at verify levels `1` and `3`): Gymbag reads the client's `SSH-2.0-...` identification banner and flags empty/non-`SSH-` probes plus known scanners and scripted SSH libraries (`libssh`, `paramiko`, `zgrab`, `masscan`, `nmap`, `asyncssh`, `ssh2go`, `jsch`, `scanner`, `bot`, `python`, `curl`, `ssh-2.0-go`, `hydra`, `medusa`, `patator`), along with threat-feed IPs and VPN/proxy/Tor IPs.
- **Challenge response**: Sends an RFC 4253 `SSH_MSG_DISCONNECT` packet (reason code `11`) with the verification link right in the SSH client output.

## Minecraft Java Protection

### Managed Startup (`--server-jar`) vs. Existing Server (`--server-properties`)

If you want Gymbag to supervise the Minecraft process itself, pass `--server-jar`:

```bash
./gymbag init -t minecraft --server-jar /srv/mc/paper.jar --mc-jvm-args "-Xms1G -Xmx4G"
./gymbag
```

In managed mode, Gymbag:

1. **Picks a compatible Java runtime automatically**: Inspects `version.json`, bundler nested JARs, `patch.properties`, or `.class` bytecode headers to distinguish legacy Java 8 servers from modern Java 17/21+ servers, and selects a matching `java` binary on Linux, macOS, Windows, or BSD (`/usr/local/openjdk*`, `/usr/local/jdk-*`, `/usr/pkg/java/openjdk*`; override with `--mc-java` / `java_path`).
2. **Wires `server.properties` safely**: Picks an unused loopback port in `20000–59999`, creates `server.properties` with `server-ip=127.0.0.1` if it does not exist yet (or backs up and updates an existing file), and sets `upstream` automatically.
3. **Manages the child process**: Starts the JAR with your `jvm_args` and `server_args`, restarts on crash by default (`auto_restart`), and sends a graceful console `stop` (15s timeout) when Gymbag shuts down.

### Bringing the Minecraft Server Console into Gymbag (`minecraft.console`)

When Gymbag launches the server via `server_jar`, set `console on` (Gymbagfile), `"console": true` (JSON), `--mc-console`, or `GYMBAG_MC_CONSOLE=on` to expose the Minecraft console directly in Gymbag:

```bash
# Enable interactive console during init (interactive wizard also prompts when server_jar is set)
./gymbag init -t minecraft --server-jar /srv/mc/paper.jar --mc-console --admin 127.0.0.1:2019

# Run Gymbag in the foreground with raw Minecraft console stdout/stderr + stdin command input
./gymbag

# Or run Gymbag as a daemon and attach from another shell via Admin API:
./gymbag console                           # Live interactive console stream + command prompt
./gymbag console --cmd "whitelist add Alice"
./gymbag console --no-stream --tail 100
```

- Typing `stop` (or `/stop`) inside the console stops the Minecraft subprocess cleanly without triggering `auto_restart` crash loops; type `start` (or POST `{"command":"start"}` to `/api/v1/minecraft/console`) to relaunch it without restarting Gymbag.
- When `console` is `false` (default), `/api/v1/minecraft/console` returns `403 console_disabled`.

If you already run the Minecraft server separately (systemd, tmux, screen, Pterodactyl), point `init` at its `server.properties` instead:

```bash
./gymbag init -t minecraft --server-properties /srv/mc/server.properties
./gymbag
```

For an existing `server.properties`, `init`:

1. Picks an unused port in `20000–59999` and backs up `server.properties` to `server.properties.gymbag.bak`.
2. Updates `server-port=<random>` while keeping comments and other keys intact.
3. Rewrites empty, `0.0.0.0`, or `::` `server-ip` lines to `server-ip=127.0.0.1` so players cannot bypass Gymbag directly on the backend port (make sure external firewall rules also block that port if `server-ip` was set to a non-loopback address).
4. Writes the upstream into `gymbag.json` or `Gymbagfile`.

Pass `--mc-upstream 127.0.0.1:25566` or `--no-auto-setup` if you prefer to manage `server.properties` yourself.

### How Minecraft Traffic Is Handled

- **Server List Ping passthrough**: Both modern status handshakes (`nextState == 1`) and legacy 1.6 pings (`0xFE`) are forwarded to the upstream server unless hard-blocked by a rule or `ip_security_action block`. Your server's MOTD, icon, and player count keep working normally.
- **Login & Transfer gating (`nextState == 2` / `3`)**: Gymbag inspects the handshake and Login Start username. Invalid protocol versions (`<= 0`), malformed login packets, invalid usernames (empty, over 32 characters, or containing control characters), threat-feed IPs, and VPN/proxy/Tor IPs count as suspicious at levels `1` and `3`.
- **In-memory empty-world challenge & automatic transfer (1.20.5+)**: For supported modern Java clients (`1.20.5`–`26.1`), Gymbag drops unverified players into a tiny, purely in-memory empty world (no world files are ever saved to disk) with a small stone spawn platform and sends a clickable verification link directly in in-game chat. Once the player solves the check in their browser, Gymbag sends a Minecraft `Transfer` packet that automatically redirects them into the real server (transparently rewriting `nextState=3` to `2` so upstreams with `accepts-transfers=false` still accept them).
- **Disconnect fallback (older or custom clients)**: Clients older than `1.20.5`, unknown future protocol versions, or clients that decline built-in known packs receive a Minecraft Login/Configuration Disconnect packet displaying the clickable copy-to-clipboard verification URL (`click_event`/`hover_event` on 1.21.5+ and `clickEvent`/`hoverEvent` on 1.21.4 and older) inside the **"Connection Lost"** dialog.

## Running HTTP, SSH, and Minecraft Together

One Gymbag process can run all three listeners at once:

```bash
sudo ./gymbag init -t http,ssh,minecraft \
  -d app.example.com -u http://localhost:3000 \
  --server-jar /srv/mc/paper.jar
sudo ./gymbag
```

In `Gymbagfile` format:

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
    console on
    jvm_args -Xms1G -Xmx4G
    server_properties /srv/mc/server.properties
    verify 3
    rate_limit 60 60s
}
```

And the equivalent `gymbag.json`:

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
    "console": true,
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

## Shared HTTPS Verification Portal & IP SSL Certs

By default, SSH and Minecraft share an HTTPS listener on port `8443` (`verify_port` / `--verify-port` / `GYMBAG_VERIFY_PORT`).

- **Sharing port `443`**: If you already have HTTP sites enabled and set `verify_port 443` (matching `https_port`), Gymbag serves `/v/*`, `/verify/*`, and `/.waf/gate/verify` on the main HTTPS listener instead of opening a second port.
- **Custom domain vs automatic server IP**: Set `verify_url https://verify.example.com:8443` to use a DNS hostname. If `verify_url` is blank, Gymbag picks the server's public IP from local interfaces (falling back to `https://api.ipify.org` with a 1.5s timeout).
- **IP SSL certificates (RFC 8738)**: When serving the portal by IP over ACME, Gymbag requests an RFC 8738 IP HTTP-01 certificate (automatically selecting the CA's `shortlived` profile if offered) and caches it at `certs/verify-acme-fullchain.pem` (`verify-acme-<provider>-fullchain.pem` for non-LE CAs). If port `80` isn't reachable or ACME IP issuance isn't available, it falls back to Gymbag's Local CA (`certs/verify-local-fullchain.pem`) or a self-signed IP cert (`certs/verify-selfsigned.pem`), or you can supply your own files with `verify_server { tls <cert> <key> }`.

## Defaults, Rate Limits & Redis

- Default `verify` level for SSH and Minecraft blocks is `3` (`suspicious & ai`).
- Default `rate_limit` for SSH and Minecraft is `60` attempts per `60` seconds (sites default to `240` per `60`s).
- Global custom rules defined inside `{ ... }` also run before per-service rules on SSH and Minecraft (matching IP/CIDR and client banner/username against `user_agent`).
- When `redis` / `GYMBAG_REDIS_URL` is configured, verification tickets (`gymbag:ticket:<token>`) and cleared IPs (`gymbag:clearance:<service>:<ip>`) are stored in Redis so clearances survive restarts and work across instances.
