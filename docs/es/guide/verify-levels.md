# Niveles de Verificación (`0`–`3`), Proof-of-Work y Timeouts

Un único proceso de Gymbag puede alojar y proteger decenas de dominios simultáneamente con balanceo round-robin multi-upstream y túneles WebSocket RFC 6455 full-duplex.

<VerifyLevelMatrix />

## Estados de Desafío y Bloqueo

<div class="gb-mascot-strip">
  <div class="gb-mascot-tile">
    <img src="/mascot-pensive.png" alt="Resolviendo PoW" />
    <strong>Resolviendo PoW</strong>
    <span class="gb-badge challenge">Desafío Emitido</span>
  </div>
  <div class="gb-mascot-tile">
    <img src="/mascot-happy.png" alt="Visitante verificado" />
    <strong>Acceso Concedido</strong>
    <span class="gb-badge pass">waf_clearance Emitida</span>
  </div>
  <div class="gb-mascot-tile">
    <img src="/mascot-reject.png" alt="Solicitud bloqueada" />
    <strong>Regla / Amenaza Bloqueada</strong>
    <span class="gb-badge block">403 Forbidden</span>
  </div>
</div>

## Ejemplo Multisitio

```caddyfile
app.example.com, www.example.com {
    reverse_proxy http://localhost:3000 http://localhost:3001
    verify 3
    difficulty 4
    verify_timeout 12h
}

docs.example.com {
    reverse_proxy http://localhost:4000
    verify 2
    verify_timeout 7d
}

api.example.com {
    reverse_proxy http://localhost:8080
    verify 1
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
    verify_timeout 30m
}

static.example.com {
    reverse_proxy http://localhost:9000
    protect off
}
```

## Timeout Personalizado de Estado Verificado (`verify_timeout` / `clearance_ttl`)

Cuando un visitante resuelve el desafío SHA-256 PoW, Gymbag emite una cookie firmada HMAC `HttpOnly` y `SameSite=Lax` (`waf_clearance` por defecto) vinculada al dominio y la IP del cliente.

Por defecto dura **24 horas** (`86400` segundos). Puedes personalizarlo globalmente o por sitio usando segundos enteros **o** cadenas de duración (`30m`, `12h`, `1h30m`, `7d`, `2w`):

::: code-group
```caddyfile [Gymbagfile]
{
    verify_timeout 12h
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
    verify_timeout 30m
}

docs.example.com {
    reverse_proxy http://localhost:4000
    verify 2
    clearance_ttl 7d
}
```

```bash [Flags CLI]
gymbag init -d app.example.com -u :3000 --verify-timeout 6h
gymbag run --config Gymbagfile --verify-timeout 30m
gymbag reverse-proxy --from app.internal --to :3000 --verify 0 --verify-timeout 15m
```

```bash [Variables de Entorno]
GYMBAG_VERIFY_TIMEOUT=12h gymbag
GYMBAG_CLEARANCE_TTL=3600 gymbag
```

```bash [API REST en Vivo]
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/staging.example.com \
  -H "Content-Type: application/json" \
  -d '{"clearance_ttl": 1800}'
```
:::

- Alias en `Gymbagfile`: `verify_timeout`, `verified_timeout`, `clearance_timeout`, `clearance_ttl`.
- Alias en variables de entorno: `GYMBAG_CLEARANCE_TTL`, `GYMBAG_VERIFY_TIMEOUT`, `GYMBAG_VERIFIED_TIMEOUT`.
- `POST /.waf/v1/verifications` devuelve `{"ok": true, "expires_in": <segundos>}` y configura el `Max-Age` de la cookie.
- `GET /.waf/v1/session` devuelve `clearance_ttl`, `expires_at` (RFC 3339 UTC) y `expires_in` para visitantes verificados.

## Dificultad, Ventana de Desafío y Límites de Tasa

- **`difficulty`** (`1`–`5`, defecto `4`): Número de ceros hexadecimales iniciales requeridos en el hash SHA-256.
- **`challenge_ttl`** (defecto `300`s / `5m`): Vida útil del nonce de desafío único.
- **`clearance_ttl` / `verify_timeout`** (defecto `86400`s / `24h`): Duración de la cookie de estado verificado (`30m`, `12h`, `7d`).
- **`rate_limit` / `rate_window`** (defecto `240` peticiones cada `60`s): Limitador por sitio e IP con almacén de 64 fragmentos (o Redis).
