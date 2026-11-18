# Gymbag

HTTPS reverse proxy with an Anubis-inspired proof-of-work web application firewall.

## Install

```bash
python -m pip install .
```

## Quick Start (`Gymbagfile`)

Configure your downstream domain and upstream URL in `Gymbagfile` using Caddyfile-style syntax:

```caddyfile
example.com {
    reverse_proxy http://localhost:5173
}
```

Then run:

```bash
gymbag
```

Or pass a config file or flags directly:

```bash
gymbag -c Gymbagfile
gymbag --domain example.com --upstream http://localhost:5173
```
