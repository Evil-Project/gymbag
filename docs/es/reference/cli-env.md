# Referencia de CLI y Variables `GYMBAG_*`

Orden de precedencia estricto: **Archivo de configuración → `.env` / Variables `GYMBAG_*` → Flags CLI**

## Subcomandos CLI

```bash
gymbag init [flags] [output_path]
gymbag
gymbag run --config Gymbagfile --watch
gymbag service <install|start|stop|restart|status|uninstall|render> [flags]
gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3 --verify-timeout 12h
gymbag validate --config Gymbagfile
gymbag fmt --overwrite Gymbagfile
gymbag version
```

### Flags Comunes (`run` / predeterminado / `reverse-proxy`)

| Flag | Descripción |
| :--- | :--- |
| `-c`, `--config <path>` | Ruta a `Gymbagfile`, `Caddyfile` o `.json` |
| `-d`, `--domain <list>` / `-u`, `--upstream <url>` | Dominios (`--from`) y upstream (`--to`) |
| `-l`, `--verify <0-3\|off>` / `--no-protect` | Nivel de verificación o desactivar WAF |
| `--verify-timeout <dur>` | Timeout de estado verificado (`30m`, `12h`, `7d` o segundos; alias `--clearance-ttl`) |
| `--remote-rules <url>` / `--remote-rules-refresh <dur>` | URLs de reglas remotas y frecuencia de refresco |
| `--ssl-provider <name>` / `--admin <addr>` | Proveedor ACME y API Admin REST |
| `--http-port` / `--https-port` / `--watch` | Puertos y recarga automática en caliente |

---

## Variables de Entorno (`GYMBAG_*`)

| Variable | Descripción |
| :--- | :--- |
| `GYMBAG_CONFIG` / `GYMBAG_BIND` / `GYMBAG_HTTP_PORT` / `GYMBAG_HTTPS_PORT` | Ruta de configuración, IP de escucha y puertos |
| `GYMBAG_DOMAIN` / `GYMBAG_DOMAINS` / `GYMBAG_UPSTREAM` / `GYMBAG_UPSTREAMS` | Dominios y mapeo `host=url;...` |
| `GYMBAG_PROTECT` / `GYMBAG_PROTECT_DOMAINS` / `GYMBAG_VERIFY_LEVEL` / `GYMBAG_DEFAULT_RULES` | Control global de protección, nivel `0`–`3` y reglas base |
| `GYMBAG_REMOTE_RULES` / `GYMBAG_REMOTE_RULES_REFRESH` | URLs remotas de firewall e intervalo (`300`s / `5m`) |
| `GYMBAG_DIFFICULTY` / `GYMBAG_CHALLENGE_TTL` | Dificultad PoW (`1`–`5`) y expiración del desafío (`300`s / `5m`) |
| `GYMBAG_CLEARANCE_TTL` | Tiempo de validez verificado (`30m`, `12h`, `7d` o segundos; alias `GYMBAG_VERIFY_TIMEOUT`, `GYMBAG_VERIFIED_TIMEOUT`) |
| `GYMBAG_COOKIE_NAME` / `GYMBAG_COOKIE_SECURE` / `GYMBAG_RATE_LIMIT` / `GYMBAG_RATE_WINDOW` | Cookie de autorización y límites de tasa |
| `GYMBAG_TLS_MODE` / `GYMBAG_SSL_PROVIDER` / `GYMBAG_ACME_EMAIL` / `GYMBAG_ACME_DIRECTORY` | Configuración ACME / TLS |
| `GYMBAG_EAB_KID` / `GYMBAG_EAB_HMAC_KEY` / `GYMBAG_ACCOUNT_KEY_FILE` | Credenciales EAB y clave de cuenta ACME |
| `GYMBAG_CERT_FILE` / `GYMBAG_KEY_FILE` / `GYMBAG_KEY_PASSWORD` / `GYMBAG_CERT_DIR` | Certificados PEM y directorio de trabajo (`certs`) |
| `GYMBAG_RENEW_INTERVAL` / `GYMBAG_RENEW_WITHIN_DAYS` / `GYMBAG_AUTO_HOSTS` / `GYMBAG_HSTS` | Renovación TLS, `/etc/hosts` y cabecera HSTS |
| `GYMBAG_IP_SECURITY` / `GYMBAG_IP_SECURITY_ACTION` / `GYMBAG_IP_REPUTATION` / `GYMBAG_AI_IP_LIST` / `GYMBAG_INTEL_REFRESH` | Inteligencia de amenazas y listas de IA |
| `GYMBAG_REDIS_URL` / `GYMBAG_SECRET` / `GYMBAG_ADMIN` | Redis RESP2, secreto HMAC y dirección de API Admin |
