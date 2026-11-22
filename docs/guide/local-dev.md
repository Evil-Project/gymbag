# Local Development & Two-Tier PKI

Running local HTTPS with custom hostnames usually means juggling `mkcert`, editing `/etc/hosts` by hand, and clicking past browser cert warnings. Gymbag handles both the hosts entries and the TLS certificates automatically for any site on:

- `localhost` or `*.localhost`
- `*.internal` (such as `app.internal`, `api.internal`)
- `*.local` or `*.test`

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

## Automatic `/etc/hosts` Management

With `auto_hosts on` (the default), Gymbag checks `/etc/hosts` (`C:\Windows\System32\drivers\etc\hosts` on Windows) at startup. Any local domain that does not resolve there yet gets IPv4 and IPv6 loopback lines tagged with the instance ID:

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

- Existing `localhost` lines stay untouched.
- On `Ctrl+C` or `SIGTERM`, Gymbag re-reads `/etc/hosts` so edits you made while it was running survive, and removes only its `# gymbag-managed:<instanceID>` lines.
- If `/etc/hosts` isn't writable (no root), Gymbag logs a warning and keeps running. Set `auto_hosts off` (or `GYMBAG_AUTO_HOSTS=off`) if you resolve `.internal` through `dnsmasq` or manage the file yourself.

## Two-Tier Local Certificate Authority

Under `<cert_dir>/ca/` (default `certs/ca/`), Gymbag keeps a two-tier ECDSA P-256 CA:

1. **Root CA**: `"Neuro-sama Headquarters Root CA"` (`certs/ca/neuro-sama-root-ca.pem`, 10-year validity, `MaxPathLen: 1`)
2. **Intermediate CA**: `"Gymbag & Swarm SSL Local CA"` (`certs/ca/gymbag-swarm-local-ca.pem`, 5-year validity, `MaxPathLen: 0`)
3. **Leaf Certificate**: `certs/local-fullchain.pem` + `certs/local-privkey.pem` (90-day validity, renewed within 30 days, with DNS SANs plus `127.0.0.1` and `::1`)

### Trusting the Root CA Once

Install `certs/ca/neuro-sama-root-ca.pem` in your OS trust store once and browsers plus `curl` validate every local site:

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

::: info Firefox Note
On macOS and Windows, Firefox reads OS root CAs when `security.enterprise_roots.enabled` is `true` in `about:config`. On Linux, if Firefox uses its own NSS database instead of `p11-kit`, import `certs/ca/neuro-sama-root-ca.pem` once in **Settings -> Privacy & Security -> Certificates -> View Certificates -> Authorities**.
:::
