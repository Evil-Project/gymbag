# Local Development & Two-Tier PKI

Testing HTTPS locally with custom hostnames usually means juggling `mkcert`, manual `/etc/hosts` edits, and browser warnings. Gymbag handles both local hostname resolution and local X.509 issuance automatically whenever a site uses:

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

When `auto_hosts` is enabled (the default), Gymbag inspects `/etc/hosts` (or `C:\Windows\System32\drivers\etc\hosts` on Windows) on startup. For any concrete local domain not already present, it appends IPv4 and IPv6 loopback entries tagged with its instance ID:

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

- Existing `localhost` entries are left untouched.
- On shutdown (`Ctrl+C` / `SIGTERM`), Gymbag re-reads `/etc/hosts` to preserve any edits you made while it was running and removes **only** its own `# gymbag-managed:<instanceID>` lines.
- If Gymbag starts without root privileges to write `/etc/hosts`, it logs a warning and **continues running normally**. Set `auto_hosts off` (or `GYMBAG_AUTO_HOSTS=off`) if you resolve `.internal` via `dnsmasq` or manage `/etc/hosts` manually.

## Two-Tier Local Certificate Authority

Inside `<cert_dir>/ca/` (default `certs/ca/`), Gymbag maintains a two-tier ECDSA P-256 PKI:

1. **Root CA**: `"Neuro-sama Headquarters Root CA"` (`certs/ca/neuro-sama-root-ca.pem`, 10-year validity, `MaxPathLen: 1`)
2. **Intermediate CA**: `"Gymbag & Swarm SSL Local CA"` (`certs/ca/gymbag-swarm-local-ca.pem`, 5-year validity, `MaxPathLen: 0`)
3. **Leaf Certificate**: `certs/local-fullchain.pem` + `certs/local-privkey.pem` (90-day validity, auto-renewed within 30 days, with DNS SANs plus `127.0.0.1` and `::1` IP SANs)

### Trusting the Root CA Once

Trust `certs/ca/neuro-sama-root-ca.pem` once in your OS keychain for a clean green lock across browsers and `curl`:

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
