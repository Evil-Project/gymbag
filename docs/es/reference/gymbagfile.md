# Referencia de `Gymbagfile` y Esquema JSON

Por defecto, Gymbag busca `./gymbag.json`, `./Gymbagfile` y `./Caddyfile` en el directorio de trabajo (o `--config <path>` / `GYMBAG_CONFIG`). Analiza de forma nativa `gymbag.json`, `Gymbagfile` / `Caddyfile v2`, `nginx.conf` y Apache (`httpd.conf` / `apache2.conf`).

## Bloque de opciones globales `{ ... }`

Un bloque opcional `{ ... }` al principio configura TLS global, ACME, red, feeds de amenazas/VPN/IA, reglas remotas, ajustes del portal de verificación y políticas WAF por defecto:

```caddyfile
{
    email ops@example.com
    ssl_provider letsencrypt
    # eab YOUR_EAB_KID YOUR_EAB_HMAC_KEY
    # account_key certs/acme-account.pem
    # ssl_key certs/domain.key

    bind 0.0.0.0
    http_port 80
    https_port 443
    admin 127.0.0.1:2019
    cert_dir certs
    auto_hosts on
    hsts on

    # Duración por defecto de la cookie de autorización o IP autorizada (segundos o duración: 30m, 12h, 7d)
    verify_timeout 24h

    # Portal HTTPS de verificación compartido para SSH y Minecraft
    # verify_url https://verify.example.com:8443
    # verify_port 8443

    # Almacén Redis opcional para nonces PoW, límites de tasa y tickets SSH/MC compartidos
    # redis redis://127.0.0.1:6379/0

    # Inteligencia de amenazas IP, detección VPN/Tor y feeds de IPs de rastreadores de IA
    ip_security on
    ip_security_action verify
    ip_reputation on
    vpn_detection on
    ai_ip_list on
    intel_refresh 6h

    # Reglas remotas de firewall compartidas (sintaxis Gymbagfile o JSON)
    # remote_rules https://rules.example.com/baseline.rules
    # remote_rules_refresh 5m

    # Reglas personalizadas globales evaluadas antes que las reglas remotas y de cada sitio
    block ip 198.51.100.99 192.0.2.0/24
}
```

### Directivas globales

| Directiva | Descripción | Por defecto |
| :--- | :--- | :--- |
| `email` / `acme_email` | Correo de contacto para registro ACME (y auto-EAB de ZeroSSL) | — |
| `ssl_provider` / `ca` | `letsencrypt`, `letsencrypt_staging`, `zerossl`, `google`, `ssl.com`, `litessl`, `buypass`, `local` o URL | `auto` (`letsencrypt` / `local`) |
| `acme_directory` | URL explícita de directorio RFC 8555 ACME v2 | Let's Encrypt v2 |
| `eab <kid> <hmac>` | Credenciales External Account Binding (o `eab_kid` / `eab_hmac_key`) | — |
| `account_key` | Ruta PEM a clave de cuenta ACME ECDSA o RSA existente | `<cert_dir>/acme-account.pem` |
| `ssl_key` | Ruta a clave privada existente para CSRs ACME | Autogenerada |
| `tls` | `auto`, `acme`, `local`, `selfsigned`, `file`, `off`, `acme.sh <path>` o `<cert.pem> <key.pem>` | `auto` |
| `auto_ssl` | Activar o desactivar el aprovisionamiento automático de SSL (`on`/`off`; también acepta `auto_https on\|off\|disable_redirects`) | `on` |
| `acme_sh_path` | Ruta al directorio principal de `acme.sh` (`~/.acme.sh`) o al directorio de certificados del dominio (alias `acme_sh`) | — |
| `direct_http` | Servir sitios directamente en `http_port` sin redirección a HTTPS (también acepta `allow_http` / `http_only`) | `off` (`on` cuando TLS está desactivado) |
| `key_password` | Frase de paso para claves privadas PEM cifradas | — |
| `cert_dir` | Directorio para certificados, CA local, secreto, caché de inteligencia y caché de reglas | `certs` |
| `renew_interval` | Intervalo de comprobación de renovación de certificados (segundos o duración como `12h`) | `43200` (`12h`) |
| `renew_within_days` | Renovar certificados cuando queden N días para caducar | `30` |
| `auto_hosts` | Gestionar entradas `/etc/hosts` para `localhost` / `*.internal` (`on`/`off`) | `on` |
| `hsts` | Enviar cabecera `Strict-Transport-Security` (`on`/`off`) | `on` para certs ACME públicos |
| `bind` / `bind_host` | Dirección IP de escucha | `127.0.0.1` (HTTP local) / `0.0.0.0` (HTTP público, SSH o MC) |
| `http_port` / `https_port` | Puerto de redirección HTTP/ACME y puerto de pasarela HTTPS | `80` / `443` |
| `verify_url` | URL HTTPS pública para el portal de verificación de SSH/Minecraft | IP del servidor + cert SSL IP |
| `verify_port` | Puerto HTTPS compartido para el portal de verificación de SSH/Minecraft | `8443` |
| `verify_server { ... }` | Bloque opcional con `url`, `port` y `tls <cert> <key>` | — |
| `admin` | Dirección de escucha de la Admin REST API local (p. ej. `127.0.0.1:2019` u `off`) | desactivada |
| `redis` / `redis_url` | URL de conexión Redis (`redis://` o `rediss://`) | almacén en memoria |
| `secret` | Secreto HMAC para cookies `waf_clearance` | `<cert_dir>/secret` |
| `trusted_proxies` | IPs/CIDRs de proxies intermedios de confianza para `X-Forwarded-For` | — |
| `mascot` | Archivo de imagen, directorio, URL `http(s)://` o bloque `mascot { default / pensive / happy / reject }` personalizado (también acepta `mascot_pensive`, `mascot_happy`, `mascot_reject`) | Predeterminada integrada |
| `ip_security` | Activar feeds CIDR de inteligencia de amenazas (`on`/`off`) | `on` |
| `ip_security_action` | Acción ante coincidencia de amenazas (`verify` o `block`) | `verify` |
| `ip_reputation` | Activar consultas en vivo a Shodan InternetDB y StopForumSpam (y APIs VPN en vivo si `vpn_detection` está activo) | `on` |
| `vpn_detection` / `vpn_detect` | Marcar IPs gratuitas de VPN/proxy/Tor como sospechosas en niveles `1` y `3` (`on`/`off`) | `on` |
| `ai_ip_list` | Activar listas oficiales CIDR de rastreadores de IA (`on`/`off`) | `on` |
| `intel_refresh` | Intervalo de refresco de feeds (segundos o duración como `6h`) | `21600` (`6h`) |
| `threat_feed` / `ai_ip_feed` / `vpn_feed` | URLs adicionales de feeds IP propios (`threat_feeds`, `ai_ip_feeds`, `vpn_feeds` también valen) | — |
| `remote_rules` | URLs HTTP/HTTPS de reglas remotas de firewall heredadas por todos los sitios | — |
| `remote_rules_refresh` | Intervalo de refresco para URLs de reglas remotas (segundos o duración; alias `rules_refresh`) | `300` (`5m`) |
| `rules_file` | Archivo local de reglas `.rules`/`.json` (`rules_file <path> [dynamic\|static]`) | — |
| `rules_dir` | Directorio local de reglas (`rules_dir <path> [dynamic\|static]`) | — |
| `ruleset <name> { ... }` | Bloque con nombre de fuente de reglas local o remota (`source`/`file`/`dir`/`url`, `dynamic on\|off`) | — |
| `rules_watch_interval` | Intervalo de sondeo para archivos y carpetas locales `dynamic` | `2` (`2s`) |
| `verify_timeout` / `clearance_ttl` | Duración por defecto de la autorización (segundos o duración como `30m`, `12h`, `7d`) | `86400` (`24h`) |

## Directivas de bloque de sitio

```caddyfile
app.example.com, www.example.com {
    reverse_proxy http://localhost:3000 http://localhost:3001
    verify 3
    difficulty 4
    rate_limit 240 60s
    challenge_ttl 5m
    verify_timeout 12h
    cookie_name waf_clearance
    cookie_secure true
    default_rules on
    remote_rules https://rules.example.com/app.rules

    allow ip 10.0.0.0/8
    block path /internal-only*
}
```

| Directiva | Descripción |
| :--- | :--- |
| `reverse_proxy <urls...>` | Uno o más destinos upstream (balanceo round-robin; admite forma abreviada `:3000`) |
| `verify [0\|1\|2\|3\|off]` | Nivel de verificación (`verify` sin argumentos activa el nivel `3`) |
| `protect <on\|off>` | Activa o desactiva la verificación WAF |
| `difficulty <1-5>` | Dificultad SHA-256 PoW en ceros hexadecimales iniciales |
| `rate_limit <count> [window]` | Límite de tasa por IP y ventana opcional (también acepta `rate_window`) |
| `challenge_ttl <sec\|dur>` | Caducidad del nonce de desafío PoW (p. ej. `300` o `5m`) |
| `clearance_ttl <sec\|dur>` | Duración de la autorización verificada (alias: `verify_timeout`, `verified_timeout`, `clearance_timeout`) |
| `cookie_name <name>` | Nombre de la cookie de autorización |
| `cookie_secure <bool>` | Si la cookie de autorización incluye el atributo `Secure` |
| `default_rules <on\|off>` | Activar o desactivar las reglas base integradas |
| `remote_rules <urls...>` | URLs de reglas remotas de firewall (sintaxis Gymbagfile o JSON) |
| `rules_file <path> [dynamic\|static]` | Archivo local `.rules` o `.json` (con recarga en caliente opcional) |
| `rules_dir <path> [dynamic\|static]` | Directorio de archivos `.rules`/`.json` (con recarga en caliente opcional) |
| `ruleset <name> { ... }` | Bloque con nombre de fuente de reglas (`source`/`file`/`dir`/`url`, `dynamic on\|off`) |
| `handle` / `handle_path` / `route` | Bloques ordenados de ruta por path (`reverse_proxy`, `root` + `file_server`, `respond`, `redir`, `header`) |
| `root` / `file_server` / `try_files` | Servicio de archivos estáticos, listados de directorio (`browse`) y fallback SPA |
| `respond` / `redir` / `header` | Respuestas estáticas, redirecciones (`{scheme}`, `{host}`, `{uri}`, `{path}`, `{query}`) y edición de cabeceras |
| `ai_bots <patterns...>` | Subcadenas adicionales de User-Agent tratadas como rastreadores de IA |
| `suspicious_uas <patterns...>` | Subcadenas adicionales de User-Agent tratadas como sospechosas |
| `suspicious_paths <prefixes...>` | Prefijos adicionales de path tratados como sondeos sospechosos |
| `tls <cert> <key>` | Rutas de certificado SNI y clave privada por sitio (o `tls acme.sh <path>`) |
| `acme_sh_path <path>` | Directorio principal de `acme.sh` o directorio de certificados del dominio por sitio (alias `acme_sh`) |
| `mascot <path-or-url>` | Archivo, directorio, URL o bloque `mascot { ... }` de mascota personalizada por sitio (`mascot_pensive`, `mascot_happy`, `mascot_reject`) |
| `allow \| verify \| block ...` | Regla de firewall personalizada abreviada |
| `rule "name" { ... }` | Bloque de regla de firewall multicondición |

## Bloques de servicio `ssh` y `minecraft`

Los bloques de nivel superior `ssh [port] { ... }` y `minecraft [port] { ... }` (alias `mc`) configuran el filtrado TCP con PoW:

```caddyfile
ssh {
    port 22
    upstream 127.0.0.1:41822
    verify 3
    difficulty 4
    rate_limit 60 60s
    clearance_ttl 24h
    allow ip 10.0.0.0/8
}

minecraft {
    port 25565
    upstream 127.0.0.1:48291
    server_jar /srv/mc/paper.jar
    jvm_args -Xms1G -Xmx4G
    server_properties /srv/mc/server.properties
    verify 3
    rate_limit 60 60s
}
```

| Directiva | Descripción | Por defecto |
| :--- | :--- | :--- |
| `enabled <bool>` | Activar o desactivar el bloque sin borrarlo | `true` |
| `port <port>` | Puerto público de escucha (también puede escribirse `ssh 2222 { ... }`) | `22` (SSH) / `25565` (Minecraft) |
| `upstream <host:port>` | Destino TCP backend (también acepta `reverse_proxy` / `proxy`, p. ej. `:2222`, `tcp://127.0.0.1:25566`) | — (`127.0.0.1:25566` cuando se define `server_jar`) |
| `server_properties <path>` | Ruta a `server.properties` (solo bloque `minecraft`) | — |
| `server_jar <path>` / `jar` | Ruta al `.jar` de Minecraft para arrancarlo automáticamente junto a Gymbag (solo bloque `minecraft`) | — |
| `java_path <bin>` / `java` | Anulación explícita del binario de Java (solo bloque `minecraft`) | Autoseleccionado |
| `jvm_args <flags...>` | Argumentos JVM pasados antes de `-jar` (solo bloque `minecraft`) | — |
| `server_args <args...>` | Argumentos extra pasados después de la ruta del JAR (solo bloque `minecraft`) | — |
| `work_dir <dir>` | Directorio de trabajo para el proceso de Minecraft gestionado (solo bloque `minecraft`) | Directorio del JAR |
| `auto_restart <on\|off>` | Reiniciar automáticamente el servidor de Minecraft si termina de forma inesperada (solo bloque `minecraft`) | `on` |
| `verify [0\|1\|2\|3\|off]` | Nivel de verificación (`verify` sin argumentos fija `3`) | `3` |
| `protect <on\|off>` | Activar o desactivar la verificación PoW en este servicio | `on` |
| `difficulty <1-5>` | Dificultad PoW en ceros hexadecimales iniciales | `4` |
| `rate_limit <count> [window]` | Límite de conexiones por IP y ventana | `60` / `60s` |
| `challenge_ttl <sec\|dur>` | Caducidad del token de desafío | `300` (`5m`) |
| `clearance_ttl <sec\|dur>` | Duración de la IP autorizada (también acepta `verify_timeout`) | `86400` (`24h`) |
| `verify_url` / `verify_port` | Anulación por servicio del enlace de verificación mostrado al desconectar | `verify_url` / `verify_port` globales |
| `allow \| verify \| block ...` / `rule` | Reglas personalizadas que comparan IP/CIDR o banner/usuario del cliente (`user_agent`) | — |

## Sintaxis de duración

Las directivas temporales (`clearance_ttl` / `verify_timeout`, `challenge_ttl`, `rate_window`, `intel_refresh`, `remote_rules_refresh`, `rules_watch_interval` y `renew_interval`) aceptan:
- Segundos enteros: `300`, `3600`, `86400`
- Duraciones Go: `45s`, `30m`, `12h`, `1h30m`
- Abreviaturas de días y semanas: `1d`, `7d`, `1d12h`, `2w`

## Formateo y validación

```bash
./gymbag validate --config Gymbagfile
./gymbag fmt --overwrite Gymbagfile
```

## Configuración JSON (`gymbag.json`)

`gymbag init` genera por defecto un `gymbag.json` conciso, omitiendo servicios sin uso y valores por defecto (`--format gymbagfile` cambia a sintaxis Caddy):

```bash
./gymbag init -d app.example.com -u :3000 --verify-timeout 12h --rules-dir ./rules.d --rules-dynamic
```

Claves JSON de nivel superior (se aplica `DisallowUnknownFields`):
- `sites`: array de `{domain, upstreams, routes, headers, protect, mascot, waf, cert_file, key_file, acme_sh_path}`, donde `mascot` es una cadena de ruta/URL o `{default, pensive, happy, reject}`, y `waf` contiene `{verify_level, difficulty, challenge_ttl, clearance_ttl, cookie_name, cookie_secure, rate_limit, rate_window, default_rules, rules, remote_rules, rule_sources, custom_ai_bots, custom_suspicious_uas, custom_suspicious_paths}`.
- `ssh`: `{enabled, port, upstream, protect, verify_level, difficulty, challenge_ttl, clearance_ttl, rate_limit, rate_window, verify_url, verify_port, rules}`.
- `minecraft`: `{enabled, port, upstream, server_properties, server_jar, java_path, jvm_args, server_args, work_dir, auto_restart, protect, verify_level, difficulty, challenge_ttl, clearance_ttl, rate_limit, rate_window, verify_url, verify_port, rules}`.
- `verify`: `{url, port, cert_file, key_file}`.
- `bind_host`, `http_port`, `https_port`, `direct_http`, `admin_addr`, `remote_rules_refresh`, `rules_watch_interval`, `redis_url`, `secret`, `trusted_proxies`, `mascot`.
- `tls`: `{mode, auto_ssl, provider, cert_file, key_file, acme_sh_path, key_password, account_key_file, eab_kid, eab_hmac_key, auto_hosts, cert_dir, acme_email, acme_directory, renew_interval, renew_within_days, hsts}`.
- `intel`: `{ip_security, ip_security_action, live_reputation, ai_ip_list, vpn_detection, refresh_interval, threat_feeds, ai_ip_feeds, vpn_feeds}`.
- `defaults`: objeto `waf` base compartido.

También puedes convertir `nginx.conf`, Apache `httpd.conf`/`apache2.conf` o `Caddyfile` a `gymbag.json`:

```bash
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```
