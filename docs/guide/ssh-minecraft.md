# SSH & Minecraft Protection

Beyond HTTP/HTTPS, Gymbag can gate OpenSSH and Minecraft Java Edition behind the same SHA-256 proof-of-work challenge, rate limiter, IP threat feeds, VPN/proxy/Tor detection, and custom rules.

Instead of requiring clients to solve PoW inside SSH or Minecraft packets, Gymbag rejects unverified connections using the protocol's own disconnect screen and prints a one-time HTTPS link (`/v/<token>`). Once the user opens that link in a browser and solves the puzzle, Gymbag clears their IP for `clearance_ttl` (default `24h`) and subsequent connections pass straight through over a raw TCP relay.

## SSH Protection

### Automated Setup (`sudo gymbag init -t ssh`)

Running `sudo ./gymbag init -t ssh` handles the host wiring automatically:

1. Picks an unused loopback port in `20000–59999`.
2. Saves a backup of `/etc/ssh/sshd_config` to `/etc/ssh/sshd_config.gymbag.bak` (if one does not exist yet).
3. Comments out existing top-level `Port` directives and writes the random port inside a `# gymbag-managed:start` / `# gymbag-managed:end` block before the first `Match` block.
4. Runs `sshd -t -f /etc/ssh/sshd_config`. If validation fails, it restores the original file and aborts.
5. Reloads `sshd` (via `systemctl`, `service`, or `rc-service` on Linux).
6. Updates the active Linux firewall (`ufw`, `firewalld`, `nftables`, or `iptables`+`ip6tables`) to allow loopback traffic to the backend port, drop external traffic hitting the backend port directly, and keep the SSH listen port (`22`) and verify portal port (`8443`) open.
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

### Automated Setup (`gymbag init -t minecraft`)

Point `init` at your server's `server.properties`:

```bash
./gymbag init -t minecraft --server-properties /srv/mc/server.properties
./gymbag
```

1. Picks an unused port in `20000–59999` and backs up `server.properties` to `server.properties.gymbag.bak`.
2. Updates `server-port=<random>` while keeping comments and other keys intact.
3. Rewrites empty, `0.0.0.0`, or `::` `server-ip` lines to `server-ip=127.0.0.1` so players cannot bypass Gymbag directly on the backend port (make sure external firewall rules also block that port if `server-ip` was set to a non-loopback address).
4. Writes the upstream into `gymbag.json` or `Gymbagfile`.

Pass `--mc-upstream 127.0.0.1:25566` or `--no-auto-setup` if you prefer to manage `server.properties` yourself.

### How Minecraft Traffic Is Handled

- **Server List Ping passthrough**: Both modern status handshakes (`nextState == 1`) and legacy 1.6 pings (`0xFE`) are forwarded to the upstream server unless hard-blocked by a rule or `ip_security_action block`. Your server's MOTD, icon, and player count keep working normally.
- **Login & Transfer gating (`nextState == 2` / `3`)**: Gymbag inspects the handshake and Login Start username. Invalid protocol versions (`<= 0`), malformed login packets, invalid usernames (empty, over 32 characters, or containing control characters), threat-feed IPs, and VPN/proxy/Tor IPs count as suspicious at levels `1` and `3`.
- **Challenge response**: Sends a Minecraft Login Disconnect packet (`0x00`) formatted as a JSON Chat Component so the clickable/readable verification URL shows up in the player's **"Connection Lost"** dialog.

## Running HTTP, SSH, and Minecraft Together

One Gymbag process can run all three listeners at once:

```bash
sudo ./gymbag init -t http,ssh,minecraft \
  -d app.example.com -u http://localhost:3000 \
  --server-properties /srv/mc/server.properties
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
