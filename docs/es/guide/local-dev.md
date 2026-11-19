# Desarrollo Local y PKI de Dos Niveles

Gymbag gestiona automáticamente la resolución de nombres local y la emisión de certificados X.509 locales cuando un sitio utiliza:

- `localhost` o `*.localhost`
- `*.internal` (p. ej. `app.internal`, `api.internal`)
- `*.local` o `*.test`

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

## Gestión Automática de `/etc/hosts`

Cuando `auto_hosts` está activado (por defecto), Gymbag añade entradas de loopback IPv4 e IPv6 etiquetadas con su ID de instancia al iniciar, y las elimina limpiamente al detenerse (`Ctrl+C` / `SIGTERM`) preservando cualquier otra edición en `/etc/hosts`:

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

Si se ejecuta sin permisos de root sobre `/etc/hosts`, registra una advertencia y continúa funcionando normalmente. Usa `auto_hosts off` (o `GYMBAG_AUTO_HOSTS=off`) si prefieres gestionarlo manualmente o con `dnsmasq`.

## Autoridad Certificadora Local de Dos Niveles

Dentro de `<cert_dir>/ca/` (por defecto `certs/ca/`), Gymbag mantiene una PKI ECDSA P-256:

1. **CA Raíz**: `"Neuro-sama Headquarters Root CA"` (`certs/ca/neuro-sama-root-ca.pem`, 10 años, `MaxPathLen: 1`)
2. **CA Intermedia**: `"Gymbag & Swarm SSL Local CA"` (`certs/ca/gymbag-swarm-local-ca.pem`, 5 años, `MaxPathLen: 0`)
3. **Certificado Hoja**: `certs/local-fullchain.pem` + `certs/local-privkey.pem` (90 días, renovación automática a los 30 días, con SANs DNS + `127.0.0.1` y `::1`)

### Confiar en la CA Raíz una Sola Vez

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

::: info Nota para Firefox
En macOS y Windows, activa `security.enterprise_roots.enabled = true` en `about:config`. En Linux, si Firefox usa su propio almacén NSS, importa `certs/ca/neuro-sama-root-ca.pem` en **Ajustes → Privacidad y Seguridad → Certificados → Ver Certificados → Autoridades**.
:::
