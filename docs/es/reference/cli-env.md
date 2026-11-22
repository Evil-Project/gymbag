# CLI y Variables de Entorno `GYMBAG_*`

El orden de precedencia es:

**Archivo de configuración -> `.env` / variables de entorno `GYMBAG_*` -> flags CLI**

Gymbag carga `.env` desde el directorio de trabajo si existe sin sobrescribir variables de entorno ya definidas.

## Subcomandos CLI

```bash
# Generar configuración (asistente interactivo o flags); escribe gymbag.json por defecto
gymbag init [flags] [output_path]
gymbag init --format gymbagfile -o Gymbagfile -d app.example.com -u :3000

# Iniciar usando ./gymbag.json, ./Gymbagfile o ./Caddyfile
gymbag

# Iniciar con archivo explícito y recarga en caliente al editar (sondea cada 2s)
gymbag run --config Gymbagfile --watch

# Gestionar servicios del SO (systemd, launchd, openrc, supervisor)
gymbag service <install|start|stop|restart|status|uninstall|render> [flags]

# Gestionar sitios, rutas, ajustes globales y conversiones de configuración
gymbag config <list|show|add|update|remove|site|route|set|unset|convert> [flags]

# Gestionar archivos de reglas, carpetas y reglas individuales
gymbag ruleset <list|add|update|remove|validate|init|rule> [flags]

# Proxy inverso HTTP/HTTPS ad-hoc sin archivo de configuración
gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3 --verify-timeout 12h

# Validar la configuración e imprimir el resumen de servicios resuelto
gymbag validate --config Gymbagfile

# Formatear Gymbagfile en su lugar (sangría de 4 espacios)
gymbag fmt --overwrite Gymbagfile

# Imprimir versión
gymbag version
```

### Flags del servidor (`run` / por defecto / `reverse-proxy`)

| Flag | Descripción |
| :--- | :--- |
| `-c`, `--config <path>` | Ruta a `gymbag.json`, `Gymbagfile`, `Caddyfile`, `nginx.conf` o configuración de Apache |
| `-d`, `--domain <list>` | Sobrescribir dominio(s) de destino, separados por comas (`--from` en `reverse-proxy`) |
| `-u`, `--upstream <url>` | Sobrescribir URL upstream (`http://localhost:3000` o `:3000`; `--to` en `reverse-proxy`) |
| `-l`, `--verify <0-3\|off>` | Sobrescribir nivel de verificación (`0`, `1`, `2`, `3` u `off`; alias `--verify-level`, `-m`, `--mode`) |
| `--verify-timeout <dur>` | Sobrescribir duración de autorización (`30m`, `12h`, `7d` o segundos; alias `--clearance-ttl`) |
| `--remote-rules <url>` | Adjuntar URL de reglas remotas de firewall (repetible o separada por comas) |
| `--remote-rules-refresh <dur>` | Intervalo de refresco para reglas remotas (por defecto `5m` / `300s`) |
| `--no-protect` | Desactivar verificación PoW en sitios configurados por CLI |
| `--ssl-provider <name>` | Sobrescribir proveedor ACME / SSL |
| `--admin <addr>` | Habilitar Admin REST API local (`127.0.0.1:2019` u `off`) |
| `--http-port <port>` | Puerto de redirección HTTP y ACME HTTP-01 (por defecto `80`) |
| `--https-port <port>` | Puerto de pasarela HTTPS (por defecto `443`) |
| `--watch` | Vigilar archivo de configuración y recargar cambios en caliente (solo `run` / por defecto) |

## Gestión de configuración (`gymbag config`)

Todos los subcomandos de `config` aceptan `-c`, `--config <path>` (por defecto usa el archivo autodetectado).

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
| `list` / `show` | Lista sitios y servicios TCP / imprime configuración formateada (`--format json\|gymbagfile`) |
| `add` | Añade un sitio: `-d`, `-u`, `-l`, `--difficulty`, `--verify-timeout`, `--no-protect`, `--no-default-rules`, `--cert`, `--key`, `--root`, `--browse`, `--fallback`, `--respond`, `--status`, `--redir`, `--code`, `--header Name=Value\|-Name`, `-f` |
| `update` | Actualiza un sitio: `-d`, `-u`, `-l`, `--difficulty`, `--verify-timeout`, `--protect on\|off`, `--default-rules on\|off`, `--cert`, `--key`, `--clear-tls`, `--header`, `--remove-header` |
| `remove` | Elimina un sitio (`-d`) |
| `route list\|add\|update\|remove` | Gestiona rutas por path: `-d`, `--path`, `--strip-prefix`, `-u`, `--root`, `--browse`, `--fallback`, `--respond`, `--status`, `--redir`, `--code`, `--header`, `--index` |
| `set <key> <value>` / `unset <key>` | Fija o restablece claves globales (`email`, `ssl_provider`, `http_port`, `https_port`, `bind`, `admin`, `cert_dir`, `redis`, `ip_security`, `ip_security_action`, `ip_reputation`, `ai_ip_list`, `intel_refresh`, `remote_rules_refresh`, `rules_watch_interval`, `verify_timeout`, `auto_hosts`, `hsts`, `eab_kid`, `eab_hmac_key`) |
| `convert` | Convierte nginx, Apache, Caddyfile, Gymbagfile o JSON: `-c <in>`, `-o <out>`, `--format json\|gymbagfile`, `-f` |

## Gestión de conjuntos de reglas (`gymbag ruleset`)

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
| `list` | Lista fuentes de reglas enlazadas (`-c`, `-d`) |
| `add` | Enlaza un archivo, carpeta o URL de reglas: `--source`, `--name`, `--dynamic` / `--static`, `-d <domain>` o `--global`, `-c` |
| `update` | Modifica un ruleset enlazado: `--target` (o source/name posicional), `--source`, `--name`, `--dynamic` / `--static`, `-d` / `--global`, `-c` |
| `remove` | Desenlaza un ruleset: `--source <source-or-name>`, `-d` / `--global`, `-c` |
| `validate <path>` | Valida un archivo o directorio de reglas |
| `init <path>` | Crea un conjunto de reglas inicial (`-o`, `--format rules\|json`, `-f`) |
| `rule list\|add\|update\|remove` | Gestiona reglas individuales en `--file` (o en línea en la configuración mediante `-c` / `-d`): `--name`, `--index`, `--action allow\|verify\|block`, `--ip`, `--ua`, `--path`, `--query`, `--method`, `--header`, `--cookie` |

## Variables de entorno (`GYMBAG_*`)

| Variable | Descripción |
| :--- | :--- |
| `GYMBAG_CONFIG` | Ruta a `gymbag.json`, `Gymbagfile`, `Caddyfile`, `nginx.conf` o configuración de Apache |
| `GYMBAG_BIND` | Dirección de escucha (`127.0.0.1` para HTTP solo local, `0.0.0.0` cuando hay dominios públicos, SSH o Minecraft activos) |
| `GYMBAG_HTTP_PORT` / `GYMBAG_HTTPS_PORT` | Puertos de escucha HTTP y HTTPS (por defecto `80` y `443`) |
| `GYMBAG_DOMAIN` / `GYMBAG_DOMAINS` | Dominio(s) de destino separados por comas |
| `GYMBAG_UPSTREAM` | URL upstream de destino por defecto |
| `GYMBAG_UPSTREAMS` | Mapa por host separado por punto y coma: `app.example.com=http://web:3000;api.example.com=http://api:8080` |
| `GYMBAG_PROTECT` / `GYMBAG_PROTECT_DOMAINS` | Activa/desactiva protección WAF entre sitios (`on`/`off`), o restringe protección a los dominios listados |
| `GYMBAG_VERIFY_LEVEL` | Nivel de verificación: `0`, `1`, `2`, `3` u `off` (alias: `GYMBAG_VERIFY`, `GYMBAG_VERIFY_MODE`, `GYMBAG_MODE`, `GYMBAG_TRIGGER`) |
| `GYMBAG_DEFAULT_RULES` | Reglas base integradas (`on` / `off`) |
| `GYMBAG_REMOTE_RULES` / `GYMBAG_REMOTE_RULES_REFRESH` | URLs de reglas remotas e intervalo de refresco (por defecto `300`s / `5m`) |
| `GYMBAG_RULES_WATCH_INTERVAL` | Intervalo de sondeo para fuentes de reglas locales `dynamic` (por defecto `2`s / `2s`) |
| `GYMBAG_DIFFICULTY` | Dificultad SHA-256 PoW (`1`–`5`, por defecto `4`) |
| `GYMBAG_CHALLENGE_TTL` / `GYMBAG_CLEARANCE_TTL` | Caducidad del desafío (`300`s / `5m`) y duración de la autorización (`86400`s / `24h`; alias: `GYMBAG_VERIFY_TIMEOUT`, `GYMBAG_VERIFIED_TIMEOUT`) |
| `GYMBAG_COOKIE_NAME` / `GYMBAG_COOKIE_SECURE` | Nombre de cookie de autorización (`waf_clearance`) y flag `Secure` (`true`) |
| `GYMBAG_RATE_LIMIT` / `GYMBAG_RATE_WINDOW` | Límite de peticiones por IP (`240`) y ventana (`60`s / `1m`) |
| `GYMBAG_TLS_MODE` / `GYMBAG_SSL_PROVIDER` | Modo TLS (`auto`, `acme`, `local`, `selfsigned`, `file`) y proveedor CA o URL |
| `GYMBAG_ACME_EMAIL` / `GYMBAG_ACME_DIRECTORY` | Correo de contacto ACME (y auto-EAB de ZeroSSL) y URL explícita del directorio ACME |
| `GYMBAG_EAB_KID` / `GYMBAG_EAB_HMAC_KEY` | Key ID de External Account Binding y clave HMAC |
| `GYMBAG_ACCOUNT_KEY_FILE` | Ruta al archivo PEM de clave de cuenta ACME ECDSA o RSA |
| `GYMBAG_CERT_FILE` / `GYMBAG_KEY_FILE` / `GYMBAG_KEY_PASSWORD` | Certificado, clave y frase de paso para modo `file` |
| `GYMBAG_CERT_DIR` | Directorio para certificados, claves de CA, secreto, caché de inteligencia y caché de reglas (por defecto `certs`) |
| `GYMBAG_RENEW_INTERVAL` / `GYMBAG_RENEW_WITHIN_DAYS` | Intervalo de comprobación de renovación (`43200`s / `12h`) y umbral (`30` días) |
| `GYMBAG_AUTO_HOSTS` / `GYMBAG_HSTS` | Gestión automática de `/etc/hosts` (`on`/`off`) y anulación de `Strict-Transport-Security` (`on`/`off`) |
| `GYMBAG_IP_SECURITY` / `GYMBAG_IP_SECURITY_ACTION` | Activa/desactiva feeds de amenazas (`on`/`off`) y acción (`verify` o `block`) |
| `GYMBAG_IP_REPUTATION` / `GYMBAG_AI_IP_LIST` | Activa/desactiva consultas Shodan InternetDB + StopForumSpam (`on`/`off`) y listas oficiales de rastreadores de IA (`on`/`off`) |
| `GYMBAG_VPN_DETECTION` | Activa/desactiva detección gratuita de VPN/proxy/Tor en comprobaciones sospechosas (`on`/`off`, por defecto `on`) |
| `GYMBAG_INTEL_REFRESH` | Intervalo de refresco de feeds de amenazas, VPN e IA (por defecto `21600`s / `6h`) |
| `GYMBAG_VERIFY_URL` / `GYMBAG_VERIFY_PORT` | URL del portal HTTPS de verificación compartido (vacío = IP del servidor + cert IP) y puerto (por defecto `8443`) |
| `GYMBAG_SSH_ENABLED` / `GYMBAG_SSH_PORT` / `GYMBAG_SSH_UPSTREAM` | Activa protección SSH (por defecto `22`) y fija destino backend `sshd` |
| `GYMBAG_SSH_VERIFY_LEVEL` / `GYMBAG_SSH_PROTECT` | Nivel de verificación SSH (`0`–`3` u `off`) e interruptor de protección (`on`/`off`) |
| `GYMBAG_MC_ENABLED` / `GYMBAG_MC_PORT` / `GYMBAG_MC_UPSTREAM` | Activa protección Minecraft (por defecto `25565`) y fija destino backend (admite alias `GYMBAG_MINECRAFT_*`) |
| `GYMBAG_MC_VERIFY_LEVEL` / `GYMBAG_MC_PROTECT` | Nivel de verificación Minecraft (`0`–`3` u `off`) e interruptor de protección (`on`/`off`) |
| `GYMBAG_REDIS_URL` | URL de Redis (`redis://` o `rediss://`) |
| `GYMBAG_SECRET` / `GYMBAG_ADMIN` | Secreto HMAC de autorización y dirección de escucha de la Admin REST API (`127.0.0.1:2019` u `off`) |
