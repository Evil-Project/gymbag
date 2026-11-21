# APIs REST y Redis

## Endpoints Públicos WAF

| Método | Ruta | Descripción |
| :--- | :--- | :--- |
| `GET` | `/.waf/v1/session` | Estado (`cleared`), `protected`, `verify_level`, `verify_mode`, `clearance_ttl`, y `expires_at` / `expires_in` si está verificado |
| `DELETE` | `/.waf/v1/session` | Limpia la cookie de verificación (`204 No Content`) |
| `POST` | `/.waf/v1/challenges` | Emite un desafío PoW SHA-256 de un solo uso (`201 Created`) |
| `POST` | `/.waf/v1/verifications` | Verifica la solución PoW, emite cookie firmada y devuelve `{"ok": true, "expires_in": <ttl>}` (alias `/.waf/verify`) |
| `GET` | `/.waf/assets/{name}` | Sirve activos PNG incrustados (`/.waf/v1/assets/{name}`, caché `ETag`) |

---

## API REST de Administración Local

Actívala con `admin 127.0.0.1:2019`, `--admin 127.0.0.1:2019` o `GYMBAG_ADMIN=127.0.0.1:2019`. Vincula siempre este puerto a loopback o a una red privada.

| Método | Ruta | Descripción |
| :--- | :--- | :--- |
| `GET` | `/api/v1/health` | Estado, `sites_count`, `tls_source`, `threat_entries`, `ai_entries` y `remote_rules` |
| `GET` / `POST` | `/api/v1/tls`, `/api/v1/tls/renew` | Estado TLS y renovación forzada en caliente |
| `GET` / `POST` | `/api/v1/intel`, `/api/v1/intel/refresh` | Estado y refresco inmediato de inteligencia de amenazas |
| `GET` / `POST` | `/api/v1/rules/remote`, `/api/v1/rules/remote/refresh` | Estado de reglas remotas y recarga atómica inmediata |
| `GET` / `PUT` | `/api/v1/config` | Inspeccionar o reemplazar atómicamente la configuración completa |
| `GET` / `POST` | `/api/v1/sites` | Listar o añadir sitios en tiempo de ejecución |
| `GET` / `PUT` / `PATCH` / `DELETE` | `/api/v1/sites/{domain}` | Inspeccionar, reemplazar, parchear en vivo (`verify_level`, `protect`, `difficulty`, `clearance_ttl`, `upstreams`, `rate_limit`, `rules`, `remote_rules`, `routes`, `headers` o `rule_sources`) o eliminar un sitio |
| `GET` / `POST` / `PUT` / `DELETE` | `/api/v1/sites/{domain}/rules[/id]` | Gestionar reglas personalizadas en vivo |

### Ejemplos de Mitigación de Emergencia en Vivo

```bash
# Endurecer app.example.com al nivel de verificación 0 con un timeout de 30 minutos
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"verify_level": 0, "difficulty": 5, "clearance_ttl": 1800}'

# Reemplazar rutas por path, cabeceras de respuesta y fuentes de reglas locales en vivo
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"routes": [{"path": "/static/*", "strip_prefix": "/static", "root": "./dist", "browse": true}], "headers": {"X-Served-By": "gymbag"}, "rule_sources": [{"source": "./rules.d", "dynamic": true}]}'

# Bloquear de inmediato un CIDR abusivo sin editar Gymbagfile
curl -X POST http://127.0.0.1:2019/api/v1/sites/app.example.com/rules \
  -H "Content-Type: application/json" \
  -d '{"name": "drop-abusive-subnet", "action": "block", "ips": ["198.51.100.0/24"]}'

# Forzar una actualización inmediata de reglas remotas y feeds de inteligencia de amenazas
curl -X POST http://127.0.0.1:2019/api/v1/rules/remote/refresh
curl -X POST http://127.0.0.1:2019/api/v1/intel/refresh
```

## Redis Compartido

Gymbag incluye un cliente RESP2 en Go puro. Configura `redis redis://[:password@]host:6379/0` (o `GYMBAG_REDIS_URL`) para compartir nonces PoW y límites de tasa entre réplicas. Consulta la [Guía de Docker](/es/guide/docker) para usar `compose.yaml`.
