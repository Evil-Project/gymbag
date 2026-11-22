# Niveles de Verificación (`0`–`3`) y Proof-of-Work

Un único proceso de Gymbag puede situarse delante de múltiples sitios web (con balanceo round-robin hacia los upstreams y actualizaciones WebSocket RFC 6455) junto con protección para SSH y Minecraft. Cada sitio y servicio fija su política con `verify <0-3>`, o desactiva los desafíos PoW con `protect off`.

<VerifyLevelMatrix />

## Estados de desafío

Gymbag compila directamente en el binario el HTML del desafío PoW, el resolvedor SHA-256 en el navegador y las tres imágenes de la mascota:

<div class="gb-mascot-strip">
  <div class="gb-mascot-tile">
    <img src="/mascot-pensive.png" alt="Mascota resolviendo el desafío" />
    <strong>Resolviendo PoW</strong>
    <span class="gb-badge challenge">Desafío emitido</span>
  </div>
  <div class="gb-mascot-tile">
    <img src="/mascot-happy.png" alt="Mascota de visitante autorizado" />
    <strong>Acceso concedido</strong>
    <span class="gb-badge pass">waf_clearance establecida</span>
  </div>
  <div class="gb-mascot-tile">
    <img src="/mascot-reject.png" alt="Mascota de petición bloqueada" />
    <strong>Regla / Amenaza bloqueada</strong>
    <span class="gb-badge block">403 Forbidden</span>
  </div>
</div>

## Ejemplo multisitio y multiservicio

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

ssh {
    upstream 127.0.0.1:41822
    verify 3
}
```

En los niveles `1` y `3`, el tráfico sospechoso incluye **IPs de VPN, proxy y nodos de salida de Tor** (`vpn_detection on`) además de feeds de amenazas, coincidencias en vivo con Shodan/StopForumSpam, cabeceras de navegador ausentes, clientes automatizados, sondeos de exploits y clientes SSH/Minecraft automatizados.

## Duración del estado verificado (`verify_timeout` / `clearance_ttl`)

En sitios HTTP/HTTPS, resolver el desafío PoW SHA-256 emite una cookie firmada con HMAC (`waf_clearance` por defecto) con atributos `HttpOnly` y `SameSite=Lax`, vinculada al host y a la IP del cliente. En SSH y Minecraft, resolver el enlace de verificación en el navegador guarda un registro de autorización IP para ese servicio.

La duración predeterminada es de **24 horas** (`86400` segundos). Puedes cambiarla de forma global o por sitio/servicio con segundos enteros o cadenas de duración (`30m`, `12h`, `1h30m`, `7d`, `2w`):

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

```bash [Variables de entorno]
GYMBAG_VERIFY_TIMEOUT=12h gymbag
GYMBAG_CLEARANCE_TTL=3600 gymbag
```

```bash [Admin API en vivo]
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/staging.example.com \
  -H "Content-Type: application/json" \
  -d '{"clearance_ttl": 1800}'
```
:::

- Alias admitidos en `Gymbagfile`: `verify_timeout`, `verified_timeout`, `clearance_timeout`, `clearance_ttl`.
- Alias admitidos en variables de entorno: `GYMBAG_CLEARANCE_TTL`, `GYMBAG_VERIFY_TIMEOUT`, `GYMBAG_VERIFIED_TIMEOUT`.
- `POST /.waf/v1/verifications` devuelve `{"ok": true, "expires_in": <seconds>}` y fija `Max-Age` al TTL configurado.
- `GET /.waf/v1/session` devuelve `clearance_ttl`, además de `expires_at` (RFC 3339 UTC) y `expires_in` para navegadores autorizados.

## Dificultad, ventana de desafío y límites de tasa

- `difficulty` (`1`–`5`, por defecto `4`): número de ceros hexadecimales iniciales exigidos en el hash SHA-256 (`0000...` con dificultad `4`).
- `challenge_ttl` (por defecto `300`s / `5m`): caducidad de los nonces de desafío de un solo uso.
- `clearance_ttl` / `verify_timeout` (por defecto `86400`s / `24h`): tiempo de validez de la cookie de autorización o autorización IP TCP.
- `rate_limit` / `rate_window`: limitador de peticiones por IP con ventana fija. Los sitios usan `240` peticiones cada `60`s por defecto; los bloques `ssh` y `minecraft` usan `60` intentos cada `60`s.
