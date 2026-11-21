# Referencia de CLI y Variables `GYMBAG_*`

Orden de precedencia estricto: **Archivo de configuración → `.env` / Variables `GYMBAG_*` → Flags CLI**

## Subcomandos CLI

```bash
gymbag init [flags] [output_path]
gymbag init --format gymbagfile -o Gymbagfile -d app.example.com -u :3000
gymbag
gymbag run --config Gymbagfile --watch
gymbag service <install|start|stop|restart|status|uninstall|render> [flags]
gymbag config <list|show|add|update|remove|route|set|unset|convert> [flags]
gymbag ruleset <list|add|update|remove|validate|init|rule> [flags]
gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3 --verify-timeout 12h
gymbag validate --config Gymbagfile
gymbag fmt --overwrite Gymbagfile
gymbag version
```

### Flags Comunes (`run` / predeterminado / `reverse-proxy`)

| Flag | Descripción |
| :--- | :--- |
| `-c`, `--config <path>` | Ruta a `gymbag.json`, `Gymbagfile`, `Caddyfile`, `nginx.conf` nativo o Apache `httpd.conf` / `apache2.conf` |
| `-d`, `--domain <list>` / `-u`, `--upstream <url>` | Dominios (`--from`) y upstream (`--to`) |
| `-l`, `--verify <0-3\|off>` / `--no-protect` | Nivel de verificación o desactivar WAF |
| `--verify-timeout <dur>` | Timeout de estado verificado (`30m`, `12h`, `7d` o segundos; alias `--clearance-ttl`) |
| `--remote-rules <url>` / `--remote-rules-refresh <dur>` | URLs de reglas remotas y frecuencia de refresco |
| `--ssl-provider <name>` / `--admin <addr>` | Proveedor ACME y API Admin REST |
| `--http-port` / `--https-port` / `--watch` | Puertos y recarga automática en caliente |

---

## Gestión de Configuración (`gymbag config`)

Todos los subcomandos `config` aceptan `-c`, `--config <path>` (por defecto: el archivo de configuración autodetectado).

```bash
gymbag config list
gymbag config add -d api.example.com -u :8080 -l 1
gymbag config update -d api.example.com -l 3 --verify-timeout 6h
gymbag config remove -d api.example.com
gymbag config set admin 127.0.0.1:2019
gymbag config unset admin
gymbag config route add -d app.example.com --path '/static/*' --strip-prefix /static --root ./dist --browse
gymbag config route list -d app.example.com
gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```

| Subcomando | Descripción |
| :--- | :--- |
| `list` / `show` | Listar sitios / imprimir la configuración formateada |
| `add` | Añadir un sitio: `-d`, `-u`, `-l`, `--difficulty`, `--verify-timeout`, `--no-protect`, `--no-default-rules`, `--cert`, `--key`, `--root`, `--browse`, `--fallback`, `--respond`, `--status`, `--redir`, `--code`, `--header Name=Value\|-Name`, `-f` |
| `update` | Actualizar un sitio: `-d`, `-u`, `-l`, `--difficulty`, `--verify-timeout`, `--protect on\|off`, `--default-rules on\|off`, `--cert`, `--key`, `--clear-tls`, `--header`, `--remove-header` |
| `remove` | Eliminar un sitio (`-d`) |
| `route list\|add\|update\|remove` | Gestionar rutas por path: `-d`, `--path`, `--strip-prefix`, `-u`, `--root`, `--browse`, `--fallback`, `--respond`, `--status`, `--redir`, `--code`, `--header`, `--index` (índice de ruta base 0 para `update` / `remove`) |
| `set <key> <value>` / `unset <key>` | Cambiar o restablecer una opción global (`email`, `ssl_provider`, `admin`, `http_port`, `https_port`, `rules_watch_interval`, `remote_rules_refresh`, …) |
| `convert` | Convertir nginx, Apache, Caddyfile, Gymbagfile o JSON: `-c <in>`, `-o <out>` (stdout si se omite), `--format json\|gymbagfile` (por defecto `json`), `-f` |

## Gestión de Conjuntos de Reglas (`gymbag ruleset`)

```bash
gymbag ruleset init ./rules/custom.rules
gymbag ruleset add --source ./rules --dynamic --name local-dir
gymbag ruleset update local-dir --static
gymbag ruleset remove local-dir
gymbag ruleset validate ./rules
gymbag ruleset rule add --file ./rules/custom.rules --action block --name bad-bot --ua "*BadBot*"
```

| Subcomando | Descripción |
| :--- | :--- |
| `list` | Listar fuentes de reglas enlazadas (`-c`, `-d`) |
| `add` | Enlazar un archivo de reglas, carpeta o URL: `--source`, `--name`, `--dynamic` / `--static`, `-d <domain>` o `--global`, `-c` |
| `update` | Modificar un ruleset enlazado: `--target` (o source/name posicional), `--source`, `--name`, `--dynamic` / `--static`, `-d` / `--global`, `-c` |
| `remove` | Desenlazar un ruleset: `--source <source-or-name>`, `-d` / `--global`, `-c` |
| `validate <path>` | Validar un archivo de reglas o directorio de reglas |
| `init <path>` | Crear un archivo de reglas inicial (`-o`, `--format rules\|json`, `-f`) |
| `rule list\|add\|update\|remove` | Gestionar reglas individuales en `--file` (o en línea en la configuración vía `-c` / `-d`): `--name`, `--index`, `--action allow\|verify\|block`, `--ip`, `--ua`, `--path`, `--query`, `--method`, `--header`, `--cookie` |

---

## Variables de Entorno (`GYMBAG_*`)

| Variable | Descripción |
| :--- | :--- |
| `GYMBAG_CONFIG` / `GYMBAG_BIND` / `GYMBAG_HTTP_PORT` / `GYMBAG_HTTPS_PORT` | Ruta de configuración (`gymbag.json`, `Gymbagfile`, `Caddyfile`, `nginx.conf` o Apache `httpd.conf`), IP de escucha y puertos |
| `GYMBAG_DOMAIN` / `GYMBAG_DOMAINS` / `GYMBAG_UPSTREAM` / `GYMBAG_UPSTREAMS` | Dominios y mapeo `host=url;...` |
| `GYMBAG_PROTECT` / `GYMBAG_PROTECT_DOMAINS` / `GYMBAG_VERIFY_LEVEL` / `GYMBAG_DEFAULT_RULES` | Control global de protección, nivel `0`–`3` y reglas base |
| `GYMBAG_REMOTE_RULES` / `GYMBAG_REMOTE_RULES_REFRESH` | URLs remotas de firewall e intervalo (`300`s / `5m`) |
| `GYMBAG_RULES_WATCH_INTERVAL` | Intervalo de sondeo en segundos o duración para archivos/carpetas de reglas locales dinámicas (por defecto `2` / `2s`) |
| `GYMBAG_DIFFICULTY` / `GYMBAG_CHALLENGE_TTL` | Dificultad PoW (`1`–`5`) y expiración del desafío (`300`s / `5m`) |
| `GYMBAG_CLEARANCE_TTL` | Tiempo de validez verificado (`30m`, `12h`, `7d` o segundos; alias `GYMBAG_VERIFY_TIMEOUT`, `GYMBAG_VERIFIED_TIMEOUT`) |
| `GYMBAG_COOKIE_NAME` / `GYMBAG_COOKIE_SECURE` / `GYMBAG_RATE_LIMIT` / `GYMBAG_RATE_WINDOW` | Cookie de autorización y límites de tasa |
| `GYMBAG_TLS_MODE` / `GYMBAG_SSL_PROVIDER` / `GYMBAG_ACME_EMAIL` / `GYMBAG_ACME_DIRECTORY` | Configuración ACME / TLS |
| `GYMBAG_EAB_KID` / `GYMBAG_EAB_HMAC_KEY` / `GYMBAG_ACCOUNT_KEY_FILE` | Credenciales EAB y clave de cuenta ACME |
| `GYMBAG_CERT_FILE` / `GYMBAG_KEY_FILE` / `GYMBAG_KEY_PASSWORD` / `GYMBAG_CERT_DIR` | Certificados PEM y directorio de trabajo (`certs`) |
| `GYMBAG_RENEW_INTERVAL` / `GYMBAG_RENEW_WITHIN_DAYS` / `GYMBAG_AUTO_HOSTS` / `GYMBAG_HSTS` | Renovación TLS, `/etc/hosts` y cabecera HSTS |
| `GYMBAG_IP_SECURITY` / `GYMBAG_IP_SECURITY_ACTION` / `GYMBAG_IP_REPUTATION` / `GYMBAG_AI_IP_LIST` / `GYMBAG_INTEL_REFRESH` | Inteligencia de amenazas y listas de IA |
| `GYMBAG_REDIS_URL` / `GYMBAG_SECRET` / `GYMBAG_ADMIN` | Redis RESP2, secreto HMAC y dirección de API Admin |
