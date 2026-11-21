# Referencia de `Gymbagfile` y Esquema JSON

Por defecto, Gymbag busca `./gymbag.json`, `./Gymbagfile` o `./Caddyfile` en el directorio de trabajo (o `--config <path>` / `GYMBAG_CONFIG`), y analiza de forma nativa `gymbag.json`, `Gymbagfile` / `Caddyfile v2`, `nginx.conf` y Apache (`httpd.conf` / `apache2.conf`).

## Bloque Global `{ ... }`

```caddyfile
{
    email ops@example.com
    ssl_provider letsencrypt
    bind 0.0.0.0
    http_port 80
    https_port 443
    admin 127.0.0.1:2019
    cert_dir certs
    auto_hosts on
    hsts on

    verify_timeout 24h
    # redis redis://127.0.0.1:6379/0

    ip_security on
    ip_security_action verify
    ip_reputation on
    ai_ip_list on
    intel_refresh 6h

    # remote_rules https://rules.example.com/baseline.rules
    # remote_rules_refresh 5m

    block ip 198.51.100.99 192.0.2.0/24
}
```

### Directivas Globales Principales

| Directiva | Descripción | Por Defecto |
| :--- | :--- | :--- |
| `email` / `ssl_provider` | Correo ACME y CA (`letsencrypt`, `zerossl`, `google`, `ssl.com`, `litessl`, `buypass`, `local` o URL) | `auto` |
| `eab <kid> <hmac>` / `account_key` / `ssl_key` | Credenciales EAB y rutas de claves PEM personalizadas | — |
| `tls` / `key_password` | Modo TLS (`auto`, `acme`, `local`, `selfsigned`, `<cert> <key>`) y contraseña | `auto` |
| `cert_dir` / `renew_interval` / `renew_within_days` | Directorio de certificados/cachés y ventana de renovación | `certs` / `12h` / `30` |
| `auto_hosts` / `hsts` | Gestión de `/etc/hosts` y cabecera HSTS | `on` / `on` (ACME) |
| `bind` / `http_port` / `https_port` / `admin` | Dirección IP, puertos y API Admin REST | `80` / `443` |
| `redis` / `secret` / `trusted_proxies` | Almacén Redis RESP2, secreto HMAC y proxies confiables | memoria / `<cert_dir>/secret` |
| `ip_security` / `ip_security_action` / `ip_reputation` / `ai_ip_list` / `intel_refresh` | Feeds de amenazas, reputación en vivo y listas de IA | `on` / `verify` / `6h` |
| `remote_rules` / `remote_rules_refresh` | URLs de reglas remotas e intervalo de actualización | `300` (`5m`) |
| `rules_file` | Archivo local de reglas `.rules`/`.json` (`rules_file <path> [dynamic\|static]`) heredado por todos los sitios | — |
| `rules_dir` | Directorio local de reglas (`rules_dir <path> [dynamic\|static]`) heredado por todos los sitios | — |
| `ruleset <name> { ... }` | Bloque de fuente de reglas local o remota con nombre (`source`/`file`/`dir`/`url`, `dynamic on\|off`) | — |
| `rules_watch_interval` | Intervalo de sondeo para archivos y directorios de reglas locales `dynamic` | `2` (`2s`) |
| `verify_timeout` / `clearance_ttl` | Timeout predeterminado del estado verificado (`30m`, `12h`, `7d` o segundos) | `86400` (`24h`) |

---

## Directivas de Bloque de Sitio

| Directiva | Descripción |
| :--- | :--- |
| `reverse_proxy <urls...>` | Uno o más upstreams (round-robin; soporta `:3000`) |
| `verify [0\|1\|2\|3\|off]` / `protect <on\|off>` | Nivel de verificación y activación WAF |
| `difficulty <1-5>` | Dificultad PoW SHA-256 |
| `rate_limit <count> [window]` | Límite por IP y ventana (`rate_window` también válido) |
| `challenge_ttl <sec\|dur>` | Expiración del desafío PoW (`300` o `5m`) |
| `clearance_ttl <sec\|dur>` | Tiempo de validez verificado (`86400`, `30m`, `12h`, `7d`; alias `verify_timeout`, `verified_timeout`, `clearance_timeout`) |
| `cookie_name` / `cookie_secure` / `default_rules` | Nombre de cookie, bandera `Secure` y reglas base integradas |
| `remote_rules <urls...>` | URLs de reglas remotas del sitio (alias `remote_rule`, `rules_url`, `rule_url`, `remote_rules_url`) |
| `rules_file <path> [dynamic\|static]` | Adjunta un archivo local `.rules` o `.json` (opcionalmente recargado en caliente ante cambios en disco) |
| `rules_dir <path> [dynamic\|static]` | Adjunta todos los archivos `.rules`/`.json` de un directorio (opcionalmente recargados en caliente) |
| `ruleset <name> { ... }` | Bloque de fuente de reglas con nombre (`source`/`file`/`dir`/`url`, `dynamic on\|off`) |
| `handle` / `handle_path` / `route` | Bloques de rutas por path ordenadas (`reverse_proxy`, `root` + `file_server`, `respond`, `redir`, `header`) |
| `root` / `file_server` / `try_files` | Servicio de archivos estáticos, listado de directorios (`browse`) y fallback para SPA |
| `respond` / `redir` / `header` | Respuestas fijas, redirecciones HTTP (`{scheme}`, `{host}`, `{uri}`, `{path}`, `{query}`) y modificaciones de cabeceras de respuesta |
| `tls <cert> <key>` | Certificados SNI por sitio |
| `allow \| verify \| block ...` / `rule "name" { ... }` | Reglas de firewall personalizadas |

## Sintaxis de Duración y Configuración JSON

Acepta segundos enteros (`3600`), duraciones Go (`30m`, `12h`, `1h30m`) y días/semanas (`7d`, `2w`).

`gymbag init` genera `gymbag.json` por defecto (usa `--format gymbagfile` para sintaxis estilo Caddy):

```bash
./gymbag init -d app.example.com -u :3000 --verify-timeout 12h --rules-dir ./rules.d --rules-dynamic
```

Todos los campos se corresponden directamente con el esquema JSON (`sites`, `bind_host`, `http_port`, `https_port`, `admin_addr`, `tls`, `intel`, `remote_rules_refresh`, `rules_watch_interval`, `redis_url`, `secret`, `trusted_proxies`, `defaults`), con `routes` por sitio (`path`, `strip_prefix`, `upstreams`, `root`, `index`, `browse`, `fallback`, `respond`, `redirect`, `headers`), `headers`, `waf.clearance_ttl` (segundos enteros), `waf.remote_rules` (`[]string`) y `waf.rule_sources` (`[{"source", "dynamic", "name"}]`).

También puedes convertir un archivo existente `nginx.conf`, Apache `httpd.conf`/`apache2.conf` o `Caddyfile` a `gymbag.json`:

```bash
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json
```
