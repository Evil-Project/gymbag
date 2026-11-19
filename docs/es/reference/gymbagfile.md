# Referencia de `Gymbagfile` y Esquema JSON

Por defecto, Gymbag busca `./Gymbagfile` o `./Caddyfile` (o `--config <path>` / `GYMBAG_CONFIG`), y también acepta archivos `.json`.

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
| `tls <cert> <key>` | Certificados SNI por sitio |
| `allow \| verify \| block ...` / `rule "name" { ... }` | Reglas de firewall personalizadas |

## Sintaxis de Duración y JSON

Acepta segundos enteros (`3600`), duraciones Go (`30m`, `12h`, `1h30m`) y días/semanas (`7d`, `2w`). Usa `gymbag init --format json` para exportar el esquema JSON completo.
