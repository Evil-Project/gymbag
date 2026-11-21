# Local Development & Two-Tier PKI

Doing HTTPS locally with custom hostnames usually means `mkcert`, hand-edited `/etc/hosts`, and a browser warning or two. Gymbag handles both the hostname and the certificate on its own whenever a site uses:

- `localhost` or `*.localhost`
- `*.internal` (e.g. `app.internal`, `api.internal`)
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

With `auto_hosts` on (the default), Gymbag reads `/etc/hosts` (or `C:\Windows\System32\drivers\etc\hosts` on Windows) at startup. Any local domain that is not there yet gets IPv4 and IPv6 loopback entries tagged with the instance ID:

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

- `localhost` entries are left alone.
- On shutdown (`Ctrl+C` / `SIGTERM`), Gymbag re-reads `/etc/hosts` so edits you made while it ran survive, and removes **only** its own `# gymbag-managed:<instanceID>` lines.
- If Gymbag cannot write `/etc/hosts` (no root), it logs a warning and **keeps running**. Set `auto_hosts off` (or `GYMBAG_AUTO_HOSTS=off`) if you resolve `.internal` through `dnsmasq` or edit `/etc/hosts` yourself.

## Two-Tier Local Certificate Authority

Under `<cert_dir>/ca/` (default `certs/ca/`) Gymbag keeps a two-tier ECDSA P-256 PKI:

1. **Root CA**: `"Neuro-sama Headquarters Root CA"` (`certs/ca/neuro-sama-root-ca.pem`, 10-year validity, `MaxPathLen: 1`)
2. **Intermediate CA**: `"Gymbag & Swarm SSL Local CA"` (`certs/ca/gymbag-swarm-local-ca.pem`, 5-year validity, `MaxPathLen: 0`)
3. **Leaf Certificate**: `certs/local-fullchain.pem` + `certs/local-privkey.pem` (90-day validity, auto-renewed within 30 days, with DNS SANs plus `127.0.0.1` and `::1` IP SANs)

### Trusting the Root CA Once

Add `certs/ca/neuro-sama-root-ca.pem` to your OS keychain once and browsers plus `curl` go green:

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
On macOS and Windows, Firefox trusts OS root CAs when `security.enterprise_roots.enabled` is `true` in `about:config`. On Linux, if Firefox uses its own NSS store instead of `p11-kit`, import `certs/ca/neuro-sama-root-ca.pem` under **Settings → Privacy & Security → Certificates → View Certificates → Authorities**.
:::
