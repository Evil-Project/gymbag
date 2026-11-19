# Reglas de Firewall, Reglas Remotas e Inteligencia de Amenazas

## Pipeline de Evaluación de Peticiones

Para cada petición en un sitio protegido, Gymbag evalúa las capas en orden estricto:

1. **Sanidad de petición** — Método HTTP, longitud máxima de URI, bytes nulos y path traversal (`..` / `%2e%2e`).
2. **Bypass ACME HTTP-01** — `/.well-known/acme-challenge/*` siempre pasa para garantizar la emisión de certificados.
3. **Reglas personalizadas locales** — Globales primero, luego las del sitio, de arriba a abajo (**la primera coincidencia gana**).
4. **Reglas remotas (`remote_rules`)** — Reglas validadas descargadas desde URLs HTTP/HTTPS en orden de declaración.
5. **Reglas base integradas** — Archivos sensibles, sondeos CMS, escáneres, patrones SQLi/XSS y verificación de automatización (salvo `default_rules off`).
6. **Limitador de tasa por sitio e IP** — `rate_limit` y `rate_window`.
7. **Inteligencia de amenazas IP** — `ip_security_action` (`verify` o `block`).
8. **Verificación Proof-of-Work** — Nivel `verify` configurado (`0`–`3`).

---

## Reglas en Una Línea

Sintaxis: `<allow | verify | block> <campo> <patrón...>` (alias `bypass`/`pass`, `challenge`/`pow`, `deny`/`drop`/`reject`).

| Campo | Semántica | Ejemplo |
| :--- | :--- | :--- |
| `ip` | IPv4, IPv6 o rangos CIDR | `allow ip 127.0.0.1 10.0.0.0/8 2001:db8::/32` |
| `user_agent` | Subcadena sin distinción de mayúsculas, `*` glob o `~regex` | `block user_agent BadBot ~^(?i)python-urllib/` |
| `path` | Exacto/prefijo, `*` glob o `~regex` | `verify path /api/export* ~^/v[0-9]+/search` |
| `query` | Cadena de consulta, `*` glob o `~regex` | `block query "union select" ~(?i)<script` |
| `method` | Verbos HTTP | `verify method POST DELETE` |
| `header` | `<Header-Name> [patrón]` (`""` = ausente/vacío, `*` = presente) | `verify header Accept-Language ""` |
| `cookie` | `<cookie_name> [patrón]` (`""` = ausente/vacío, `*` = presente) | `allow cookie vip_session *` |

---

## Bloques `rule` Multicondición

Múltiples valores en el **mismo** campo se combinan con **OR**; campos distintos dentro del mismo bloque `rule` se combinan con **AND**:

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

---

## Reglas Remotas (`remote_rules`)

Sincroniza reglas de firewall compartidas desde una o más URLs HTTP/HTTPS (Git raw, S3/R2, CDN o servidor interno). Se actualizan al iniciar y cada `remote_rules_refresh` (por defecto `5m` / `300`s).

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

```bash [CLI y Entorno]
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
```
:::

### Formatos Soportados y Garantías Fail-Safe

- **Detección automática de formato**: Sintaxis de reglas `Gymbagfile` o JSON (`[ ... ]` o `{"rules": [...]}`).
- **Peticiones condicionales y caché en disco**: Usa `ETag` / `If-Modified-Since` y guarda caché bajo `<cert_dir>/rules-cache/` para arranque instantáneo.
- **Seguridad ante fallos**: Valida sintaxis (límite 4 MiB, timeout 15s) antes de activar; ante errores conserva la última lista válida y recarga el motor WAF atómicamente sin cortar conexiones.

---

## Reglas Integradas (`default_rules on|off`) y Feeds de Amenazas

- `default_rules` está en `on` por defecto (si añades reglas locales o remotas, las reglas base de bloqueo siguen protegiendo `/.env`, `/.git`, `/wp-admin`, escáneres e inyecciones).
- Feeds autoactualizables cada `intel_refresh` (`6h` por defecto): **IPsum level 3+**, **Spamhaus DROP v4/v6**, **abuse.ch Feodo Tracker**, **Shodan InternetDB**, **StopForumSpam** y rangos oficiales de **OpenAI, Anthropic, Perplexity, Google y Apple**.
