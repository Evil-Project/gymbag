# Desarrollo Local y PKI de Dos Niveles

Ejecutar HTTPS en local con nombres de host propios suele obligar a pelearse con `mkcert`, editar `/etc/hosts` a mano y aceptar avisos de certificados en el navegador. Gymbag gestiona tanto las entradas de hosts como los certificados TLS automáticamente para cualquier sitio en:

- `localhost` o `*.localhost`
- `*.internal` (como `app.internal`, `api.internal`)
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

## Gestión automática de `/etc/hosts`

Con `auto_hosts on` (valor predeterminado), Gymbag revisa `/etc/hosts` (`C:\Windows\System32\drivers\etc\hosts` en Windows) al iniciar. Todo dominio local que aún no resuelva allí recibe líneas loopback IPv4 e IPv6 etiquetadas con el ID de la instancia:

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

- Las líneas previas de `localhost` quedan intactas.
- Al recibir `Ctrl+C` o `SIGTERM`, Gymbag vuelve a leer `/etc/hosts` para conservar ediciones manuales hechas mientras corría, y borra únicamente sus líneas `# gymbag-managed:<instanceID>`.
- Si `/etc/hosts` no es escribible (sin root), Gymbag registra una advertencia y sigue ejecutándose. Fija `auto_hosts off` (o `GYMBAG_AUTO_HOSTS=off`) si resuelves `.internal` mediante `dnsmasq` o gestionas el archivo a mano.

## Autoridad de Certificación local de dos niveles

Bajo `<cert_dir>/ca/` (por defecto `certs/ca/`), Gymbag mantiene una CA ECDSA P-256 de dos niveles:

1. **Root CA**: `"Neuro-sama Headquarters Root CA"` (`certs/ca/neuro-sama-root-ca.pem`, 10 años de validez, `MaxPathLen: 1`)
2. **Intermediate CA**: `"Gymbag & Swarm SSL Local CA"` (`certs/ca/gymbag-swarm-local-ca.pem`, 5 años de validez, `MaxPathLen: 0`)
3. **Certificado hoja (Leaf)**: `certs/local-fullchain.pem` + `certs/local-privkey.pem` (90 días de validez, renovado cuando faltan menos de 30 días, con SANs DNS más `127.0.0.1` y `::1`)

### Confiar una vez en la Root CA

Instala `certs/ca/neuro-sama-root-ca.pem` en el almacén de confianza de tu sistema operativo una sola vez para que navegadores y `curl` validen todos los sitios locales:

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
En macOS y Windows, Firefox lee las CAs raíz del sistema operativo cuando `security.enterprise_roots.enabled` es `true` en `about:config`. En Linux, si Firefox usa su propia base de datos NSS en lugar de `p11-kit`, importa `certs/ca/neuro-sama-root-ca.pem` una vez en **Ajustes -> Privacidad y seguridad -> Certificados -> Ver certificados -> Autoridades**.
:::
