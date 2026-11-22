# Reglas de Firewall, Reglas Remotas e Inteligencia de Amenazas

## Pipeline de evaluación de peticiones

En los sitios HTTP protegidos, las peticiones pasan por estas capas en orden:

1. **Validación básica de petición**: verificación de método HTTP, longitud máxima de URI y rechazo de bytes nulos y traversal `..` / `%2e%2e`.
2. **Exención ACME HTTP-01**: `/.well-known/acme-challenge/*` siempre pasa para no interrumpir la emisión de certificados.
3. **Reglas personalizadas locales**: primero reglas globales y luego las del sitio, de arriba abajo (gana la primera coincidencia).
4. **Reglas remotas (`remote_rules`)**: reglas validadas descargadas desde URLs HTTP/HTTPS, evaluadas en orden.
5. **Reglas base integradas**: rutas sensibles, sondeos de CMS, escáneres, consultas SQLi/XSS y reglas PoW de automatización (salvo `default_rules off`).
6. **Limitador de tasa por sitio y por IP**: `rate_limit` y `rate_window` (por defecto `240` / `60s`).
7. **Inteligencia de amenazas IP**: `ip_security_action verify` (por defecto) o `block`.
8. **Verificación Proof-of-Work**: nivel `verify` configurado (`0`–`3`), incluida la detección de VPN/proxy/Tor (`1` y `3`) y coincidencia con rastreadores de IA (`2` y `3`).

Como las reglas personalizadas locales se evalúan antes que las reglas remotas, las reglas base, los límites de tasa y los feeds de amenazas, colocar una regla `allow` al principio de un bloque de sitio incluye en lista blanca un CIDR de oficina, un health check o una cookie de vista previa sin rodeos. Las reglas globales dentro de `{ ... }` también se aplican a los servicios `ssh` y `minecraft` (junto con las reglas específicas dentro de `ssh { ... }` y `minecraft { ... }`).

## Reglas abreviadas en una línea

Sintaxis: `<allow | verify | block> <field> <pattern...>` (también funcionan los alias `bypass`/`pass`, `challenge`/`pow` y `deny`/`drop`/`reject`).

| Campo | Semántica de coincidencia | Ejemplo |
| :--- | :--- | :--- |
| `ip` | Direcciones IPv4, IPv6 o rangos CIDR | `allow ip 127.0.0.1 10.0.0.0/8 2001:db8::/32` |
| `user_agent` | Subcadena insensible a mayúsculas, glob `*` o `~regex` | `block user_agent BadBot ~^(?i)python-urllib/` |
| `path` | Exacto/prefijo, glob `*` o `~regex` | `verify path /api/export* ~^/v[0-9]+/search` |
| `query` | Coincidencia en query raw/decodificada, glob `*` o `~regex` | `block query "union select" ~(?i)<script` |
| `method` | Verbos HTTP (insensible a mayúsculas) | `verify method POST DELETE` |
| `header` | `<Header-Name> [pattern]` (`""` = ausente/vacía, `*` = presente) | `verify header Accept-Language ""` |
| `cookie` | `<cookie_name> [pattern]` (`""` = ausente/vacía, `*` = presente) | `allow cookie vip_session *` |

## Bloques `rule` multicondición

Usa `rule "name" { ... }` cuando varias condiciones deban cumplirse juntas. Múltiples valores en un mismo campo se combinan con OR; campos distintos dentro del mismo bloque se combinan con AND:

```caddyfile
app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    rule "challenge-auth-mutations" {
        action verify
        method POST
        path /login* /api/auth/* /reset-password*
    }

    rule "block-headless-api-posts" {
        action block
        method POST
        path /api/*
        header Origin ""
    }
}
```

## Reglas remotas (`remote_rules`)

Puedes obtener conjuntos compartidos de reglas desde una o más URLs HTTP/HTTPS (un archivo raw en Git, bucket de almacenamiento de objetos, CDN o servidor de políticas). Gymbag las descarga al arrancar y cada `remote_rules_refresh` (por defecto `5m` / `300s`).

::: code-group
```caddyfile [Gymbagfile]
{
    remote_rules https://rules.example.com/org-baseline.rules
    remote_rules_refresh 5m
}

app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    allow ip 10.0.0.0/8
    remote_rules https://rules.example.com/app-overrides.json
}
```

```bash [CLI y entorno]
gymbag run --config Gymbagfile \
  --remote-rules https://rules.example.com/baseline.rules \
  --remote-rules-refresh 10m

GYMBAG_REMOTE_RULES="https://rules.example.com/baseline.rules,https://rules.example.com/extra.json" \
GYMBAG_REMOTE_RULES_REFRESH=5m \
gymbag
```

```bash [Admin REST API]
curl http://127.0.0.1:2019/api/v1/rules/remote
curl -X POST http://127.0.0.1:2019/api/v1/rules/remote/refresh

curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"remote_rules": ["https://rules.example.com/emergency.rules"]}'
```
:::

### Formatos soportados de reglas remotas

Gymbag acepta tanto la **sintaxis de reglas de Gymbagfile** como **JSON** (`[ ... ]` o `{"rules": [ ... ]}`):

::: code-group
```caddyfile [Sintaxis Gymbagfile en texto plano]
block ip 198.51.100.0/24 203.0.113.42
block user_agent BadScraper ~^(?i)EvilBot/
verify path /api/export*

rule "challenge-xml-posts" {
    action verify
    method POST
    path /xmlrpc* /api/import*
    header Origin ""
}
```

```json [Objeto JSON]
{
  "rules": [
    {
      "name": "block-abusive-cidr",
      "action": "block",
      "ips": ["198.51.100.0/24", "203.0.113.42"]
    },
    {
      "name": "verify-exports",
      "action": "verify",
      "methods": ["GET", "POST"],
      "paths": ["/api/export*"]
    }
  ]
}
```
:::

### Caché y validación

- Envía `If-None-Match` (`ETag`) e `If-Modified-Since`; las respuestas `304 Not Modified` evitan recompilar el conjunto de reglas.
- Cachea los payloads validados en `<cert_dir>/rules-cache/` y los precarga al iniciar antes de que termine la primera descarga.
- Cada payload se valida (límite de 4 MiB, timeout de 15s) antes del cambio. Si un origen devuelve sintaxis inválida o un error HTTP, el conjunto anterior sigue activo y el error aparece en `GET /api/v1/rules/remote`.
- Las actualizaciones de reglas se aplican de forma atómica sin cortar conexiones activas.

## Archivos de reglas locales, carpetas y recarga en caliente

Guarda reglas en archivos `.rules` / `.json` independientes o en directorios. Marca una fuente como `dynamic` para vigilar cambios en disco y recargar automáticamente:

::: code-group
```caddyfile [Gymbagfile]
{
    rules_watch_interval 2s
}

app.example.com {
    reverse_proxy http://localhost:3000
    rules_file ./rules/custom.rules dynamic
    rules_dir ./rules.d dynamic
    ruleset static-corp {
        source ./rules/corp.json
        dynamic off
    }
}
```

```json [gymbag.json]
{
  "sites": [
    {
      "domain": "app.example.com",
      "upstreams": ["http://localhost:3000"],
      "waf": {
        "rule_sources": [
          {"source": "./rules.d", "dynamic": true, "name": "shared"}
        ]
      }
    }
  ]
}
```

```bash [CLI]
gymbag init -d app.example.com -u :3000 \
  --rules-file ./rules/custom.rules --rules-dir ./rules.d \
  --rules-dynamic --rules-watch-interval 2s

gymbag ruleset init ./rules/custom.rules
gymbag ruleset add --source ./rules --dynamic --name local-dir
gymbag ruleset update local-dir --static
gymbag ruleset validate ./rules
gymbag ruleset rule add --file ./rules/custom.rules --action block --name bad-bot --ua "*BadBot*"
gymbag ruleset remove local-dir
```
:::

El sondeo usa `rules_watch_interval` (por defecto `2s`). `rule_sources` también puede parchearse mediante `PATCH /api/v1/sites/{domain}`. Consulta [CLI y variables de entorno](/es/reference/cli-env) para la lista completa de subcomandos de `ruleset`.

## Reglas integradas (`default_rules on|off`)

`default_rules` está en `on` por defecto:
- Cuando un sitio **no** tiene reglas personalizadas locales ni remotas, Gymbag aplica tanto las reglas de bloqueo integradas (`/.env`, `/.git/*`, `/wp-admin/*`, `sqlmap`/`nuclei`, consultas SQLi/XSS) como las reglas PoW de automatización (`curl`, `wget`, `python-requests`, `headlesschrome`, `playwright`, `User-Agent` vacío).
- En cuanto añades reglas personalizadas a un sitio, tus reglas corren primero y las reglas base de **bloqueo** siguen añadiéndose después para que agregar un `verify path` no deje expuesto `/.env`.
- Fija `default_rules off` en un sitio o globalmente para desactivar por completo las reglas integradas.

## Feeds de amenazas, detección de VPN/Tor y listas de rastreadores de IA

Los feeds se refrescan al iniciar y cada `intel_refresh` (por defecto `21600`s / `6h`), cacheados en `<cert_dir>/intel-cache`:

- **Feeds de amenazas (`ip_security on`)**: IPsum nivel 3+, Spamhaus DROP IPv4 e IPv6, abuse.ch Feodo Tracker.
- **Reputación IP en vivo (`ip_reputation on`)**: consultas sin clave vía Shodan InternetDB y StopForumSpam con caché TTL de 1 hora y timeouts fail-open.
- **Detección de VPN / proxy / Tor (`vpn_detection on`)**: listas autoactualizables de VPN IPv4/IPv6 de X4BNet y lista masiva de nodos de salida de Tor, además de consultas en vivo sin clave contra `api.ipquery.io` y `api.ipapi.is` (timeout de 1.2s, deduplicación singleflight, caché de 1 hora, fail-open). Las IPs detectadas cuentan como **sospechosas** en los niveles `1` y `3` para HTTP, SSH y Minecraft.
- **CIDRs oficiales de rastreadores de IA (`ai_ip_list on`)**: OpenAI (`gptbot`, `searchbot`, `chatgpt-user`), Anthropic (`claude.com/crawling/bots.json`), Perplexity, Google y Apple (`applebot`).

```caddyfile
{
    ip_security on
    ip_security_action block   # "verify" (desafío PoW) o "block" (403 Forbidden)
    ip_reputation on
    vpn_detection on
    ai_ip_list on
    intel_refresh 6h
    # vpn_feed https://rules.example.com/extra-vpn-cidrs.txt
}
```
