# APIs REST y Redis

## Endpoints públicos del WAF

Servidos automáticamente en sitios web protegidos:

| Método | Ruta | Descripción |
| :--- | :--- | :--- |
| `GET` | `/.waf/v1/session` | Devuelve `site`, `protected`, `verify_level` (`0`–`3`), `verify_mode`, `clearance_ttl`, `cleared` y (cuando está autorizado) `expires_at` / `expires_in` |
| `DELETE` | `/.waf/v1/session` | Limpia la cookie de autorización del visitante (`204 No Content`) |
| `POST` | `/.waf/v1/challenges` | Emite un desafío PoW SHA-256 de un solo uso (`201 Created`) |
| `POST` | `/.waf/v1/verifications` | Verifica una solución PoW, fija la cookie firmada y devuelve `{"ok": true, "expires_in": <ttl>}` (admite el alias `/.waf/verify`) |
| `GET` | `/.waf/assets/{name}` | Sirve recursos PNG incrustados de desafío (alias `/.waf/v1/assets/{name}`, con caché `ETag`) |

## Endpoints del portal de verificación (SSH y Minecraft)

Servidos en `verify_port` (por defecto `8443`), o en `https_port` (`443`) cuando `verify_port` coincide con `https_port` y hay sitios HTTP activos:

| Método | Ruta | Descripción |
| :--- | :--- | :--- |
| `GET` | `/v/{token}` | Sirve la página PoW en el navegador para un ticket de puerta SSH o Minecraft (tokens de 8 caracteres insensibles a mayúsculas/minúsculas; admite el alias `/verify/{token}`) |
| `POST` | `/.waf/gate/verify` | Verifica `{token, challenge_id, nonce, verified}` y guarda una autorización IP (`{"ok": true, "service": ..., "client_ip": ..., "expires_in": <ttl>}`) |
| `GET` | `/.waf/assets/{name}` | PNGs de la mascota incrustados (admite el alias `/.waf/v1/assets/{name}`) |
| `GET` | `/.well-known/acme-challenge/{token}` | Respuesta de desafío ACME HTTP-01 para el dominio del portal o certificados IP RFC 8738 |

## Admin REST API local

Se habilita con `admin 127.0.0.1:2019`, `--admin 127.0.0.1:2019` o `GYMBAG_ADMIN=127.0.0.1:2019`.

::: warning Enlaza solo a loopback o interfaces privadas
La Admin API no solicita token de autenticación y modifica el enrutamiento en vivo y el estado del firewall sin reiniciar Gymbag. Enlázala siempre a `127.0.0.1:2019` o a una interfaz privada de gestión.
:::

| Método | Ruta | Descripción |
| :--- | :--- | :--- |
| `GET` | `/api/v1/health` | Estado, `sites_count`, `ssh_enabled`, `minecraft_enabled`, `tls_source`, `threat_entries`, `ai_entries`, `vpn_entries`, conteo de `remote_rules` cargadas y estado de `minecraft_server` (`running`, `pid`, `java_major`, `required_java`, `mc_version`, `restarts`, `eula_blocked`, `last_error`) cuando `server_jar` está configurado |
| `GET` | `/api/v1/tls` | Origen TLS activo, ruta de certificado, proveedor SSL, estado HSTS y catálogo de CAs soportadas |
| `POST` | `/api/v1/tls/renew` | Fuerza renovación inmediata de certificado y recarga atómica |
| `GET` | `/api/v1/intel` | Conteos de feeds de amenazas, rastreadores de IA y VPN/Tor (`vpn_detection_enabled`, `vpn_entries`, `vpn_cache_size`, `reputation_cache_size`), fechas de actualización y estado por feed (`threat`, `ai`, `vpn`) |
| `POST` | `/api/v1/intel/refresh` | Refresca de inmediato todos los feeds de amenazas, VPN e IA |
| `GET` | `/api/v1/rules/remote` | URLs de reglas remotas, conteos de reglas cargadas, estado de caché y últimos errores |
| `POST` | `/api/v1/rules/remote/refresh` | Descarga las URLs de reglas remotas y recarga el motor WAF atómicamente |
| `GET` | `/api/v1/config` | Vuelca la configuración activa en ejecución (`secret`, `key_password` y `eab_hmac_key` ocultados) |
| `PUT` | `/api/v1/config` | Valida y reemplaza la configuración activa en ejecución |
| `GET` | `/api/v1/sites` | Lista sitios web configurados y políticas WAF |
| `POST` | `/api/v1/sites` | Añade un sitio web en tiempo de ejecución (`201 Created`) |
| `GET` | `/api/v1/sites/{domain}` | Inspecciona la configuración de un sitio web |
| `PUT` | `/api/v1/sites/{domain}` | Reemplaza la configuración de un sitio web |
| `PATCH` | `/api/v1/sites/{domain}` | Parchea en vivo `verify_level`, `protect`, `difficulty`, `clearance_ttl`, `upstreams`, `rate_limit`, `rules`, `remote_rules`, `routes`, `headers` o `rule_sources` |
| `DELETE` | `/api/v1/sites/{domain}` | Elimina un sitio web (`204 No Content`; devuelve `409 Conflict` en el último sitio restante cuando ni SSH ni Minecraft están activos) |
| `GET` | `/api/v1/sites/{domain}/rules` | Lista reglas personalizadas locales de un sitio web |
| `POST` | `/api/v1/sites/{domain}/rules` | Añade una regla personalizada (`201 Created`) |
| `PUT` | `/api/v1/sites/{domain}/rules` | Reemplaza todas las reglas personalizadas de un sitio web |
| `DELETE` | `/api/v1/sites/{domain}/rules/{index_or_name}` | Elimina una regla personalizada por índice numérico o por `name` (`204 No Content`) |

### Ejemplos de mitigación en vivo

```bash
# Endurecer app.example.com a verify level 0 con una ventana de autorización de 30 minutos
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"verify_level": 0, "difficulty": 5, "clearance_ttl": 1800}'

# Parchear rutas por path, cabeceras de respuesta y fuentes de reglas locales en vivo
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"routes": [{"path": "/static/*", "strip_prefix": "/static", "root": "./dist", "browse": true}], "headers": {"X-Served-By": "gymbag"}, "rule_sources": [{"source": "./rules.d", "dynamic": true}]}'

# Bloquear un CIDR abusivo sin editar Gymbagfile
curl -X POST http://127.0.0.1:2019/api/v1/sites/app.example.com/rules \
  -H "Content-Type: application/json" \
  -d '{"name": "drop-abusive-subnet", "action": "block", "ips": ["198.51.100.0/24"]}'

# Refrescar reglas remotas y feeds de amenazas/VPN/IA
curl -X POST http://127.0.0.1:2019/api/v1/rules/remote/refresh
curl -X POST http://127.0.0.1:2019/api/v1/intel/refresh
```

## Estado compartido en Redis

Gymbag incluye un cliente Redis RESP2 en Go puro compatible con URLs `redis://` y `rediss://`. Apunta `redis` (o `GYMBAG_REDIS_URL`) a tu instancia de Redis para compartir:

- Desafíos PoW HTTP y de puerta de un solo uso (`gymbag:challenge:<id>`)
- Límites de tasa por IP con ventana fija (`gymbag:rate:<key>`)
- Tickets de verificación en navegador para SSH y Minecraft (`gymbag:ticket:<token>`) y autorizaciones IP (`gymbag:clearance:<service>:<ip>`)

Si Redis no es accesible al arrancar, Gymbag registra una advertencia y recurre a su almacén en memoria de 64 shards. Consulta la [guía de Docker y Docker Compose](/es/guide/docker) para levantar una pila Compose lista para ejecutarse.
