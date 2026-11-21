# Gymbag

<p align="center">
  <img src="docs/public/mascot-happy.png" alt="Gymbag mascot" width="140" />
</p>

<p align="center">
  <strong>Proxy inverso HTTPS en un único binario y WAF Proof-of-Work en Go puro</strong>
</p>

<p align="center">
  <a href="https://github.com/Evil-Project/gymbag/releases"><img src="https://img.shields.io/github/v/release/Evil-Project/gymbag?include_prereleases&label=Release" alt="Release" /></a>
  <a href="https://github.com/Evil-Project/gymbag/pkgs/container/gymbag"><img src="https://img.shields.io/badge/Docker-GHCR-blue?logo=docker" alt="Docker GHCR" /></a>
  <a href="go.mod"><img src="https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go" alt="Go 1.27+" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache--2.0-green" alt="Apache-2.0 License" /></a>
  <a href="https://evil-project.github.io/gymbag/es/"><img src="https://img.shields.io/badge/Docs-VitePress-b81d33" alt="Docs" /></a>
</p>

<p align="center">
  <strong>README:</strong>
  <a href="README.md">English</a> ·
  <a href="README.zh.md">简体中文</a> ·
  <a href="README.ja.md">日本語</a> ·
  <a href="README.ko.md">한국어</a> ·
  <a href="README.es.md">Español</a>
  <br />
  <sub>Documentación en línea: <a href="https://evil-project.github.io/gymbag/">EN</a> · <a href="https://evil-project.github.io/gymbag/zh/">ZH</a> · <a href="https://evil-project.github.io/gymbag/ja/">JA</a> · <a href="https://evil-project.github.io/gymbag/ko/">KO</a> · <a href="https://evil-project.github.io/gymbag/es/">ES</a></sub>
</p>

Gymbag es un proxy inverso HTTPS en un único binario con un Firewall de Aplicaciones Web (WAF) integrado basado en Proof-of-Work al estilo de Anubis. Está escrito en Go puro utilizando únicamente la biblioteca estándar — cero dependencias externas de Go, y cada plantilla HTML, script SHA-256 para el navegador e imagen PNG de la mascota se compila directamente en el ejecutable mediante `go:embed`.

Si alojas por tu cuenta aplicaciones web, instancias de Gitea/Forgejo, sitios de documentación o herramientas internas, probablemente te hayas topado con el mismo problema que inspiró a Anubis: rastreadores agresivos de LLM que ignoran `robots.txt` y escáneres de vulnerabilidades golpeando `/.env` y `/wp-admin` las 24 horas del día. En lugar de unir Nginx/Caddy, un contenedor sidecar PoW independiente, fail2ban y listas CIDR mantenidas a mano, Gymbag reúne terminación TLS, ACME multi-CA, proxy HTTP/WebSocket, reglas de firewall locales y remotas, inteligencia de amenazas IP y políticas PoW por sitio en un solo binario configurado con un limpio `Gymbagfile`.

---

## Inicio Rápido (Protección Lista para Usar)

Descarga un binario estático precompilado desde [GitHub Releases](https://github.com/Evil-Project/gymbag/releases) (Linux/macOS/Windows, además de `.pkg.tar.zst` para Arch Linux), instálalo mediante Nix Flakes o compila desde el código fuente:

```bash
go build -o gymbag ./cmd/gymbag
```

**Arch Linux:**
```bash
# Instalar el paquete precompilado (.pkg.tar.zst)
sudo pacman -U gymbag-0.1.0-1-x86_64.pkg.tar.zst
# O compilar localmente con makepkg
cd packaging/arch && makepkg -si
```

**Nix / NixOS:**
```bash
# Ejecutar o instalar mediante flake
nix run github:Evil-Project/gymbag -- --help
nix profile install github:Evil-Project/gymbag
```
Los binarios estáticos de Linux con `CGO_ENABLED=0` también se ejecutan directamente en NixOS sin `patchelf` ni `nix-ld`, y `nixosModules.default` ofrece configuración declarativa con `services.gymbag`.

Comienza en **dos comandos** — genera `gymbag.json` e inicia el servidor:

```bash
# 1. Generar una configuración JSON (o pasa --format gymbagfile para un Gymbagfile estilo Caddy)
./gymbag init -d app.example.com -u http://localhost:3000

# 2. Iniciar el servidor (carga automáticamente ./gymbag.json, ./Gymbagfile o ./Caddyfile)
./gymbag
```

Gymbag también lee de forma nativa configuraciones existentes de **nginx** (`nginx.conf`), **Apache** (`httpd.conf` / `apache2.conf`) y **Caddyfile v2** (incluyendo snippets, `import`, `@matchers`, `handle`, `handle_path`, `route`, `root`, `file_server`, `try_files`, `respond`, `redir` y `header`).

O regístralo como servicio del sistema (`systemd`, `launchd`, `openrc` o `supervisor`) e inícialo:

```bash
./gymbag init -d app.example.com -u :3000 --service auto
./gymbag service start
```

También puedes omitir el archivo de configuración por completo para un proxy rápido ad-hoc:

```bash
./gymbag reverse-proxy --from app.internal --to :3000 --verify 3
```

### Qué Significa "Open-Box" en la Práctica

No necesitas escribir ni una sola expresión regular ni importar listas de bloqueo antes de exponer Gymbag a internet. Al escribir simplemente `verify` (que activa el nivel `3`: `suspicious & ai`) o `block` (que rechaza directamente con `403 Forbidden` a las IP detectadas en inteligencia de amenazas en lugar de desafiarlas), se activan de inmediato tres capas de protección:

1. **Reglas base integradas (`default_rules on`)**
   - **Archivos sensibles (`403`)**: Bloquea solicitudes a `/.env`, `/.env.*`, `/.git/*`, `/.svn/*`, `/.hg/*`, `/.DS_Store`, `/.aws/*`, `/.ssh/*`, `/docker-compose.yml`, `/docker-compose.yaml`, `*.bak`, `*.sql` y `*.sqlite`.
   - **Sondeos de exploits en CMS y frameworks (`403`)**: Descarta `/wp-admin/*`, `/wp-login.php`, `/xmlrpc.php`, `/phpmyadmin*`, `/vendor/phpunit*`, `/cgi-bin/*`, `/actuator/env*` y `/actuator/heapdump*`.
   - **Escáneres de vulnerabilidades (`403`)**: Rechaza `sqlmap`, `nikto`, `nuclei`, `masscan`, `zgrab`, `gobuster`, `dirbuster`, `ffuf`, `wfuzz`, `feroxbuster`, `acunetix`, `nessus` y `openvas`.
   - **Consultas de inyección y traversal (`403`)**: Detecta `UNION SELECT`, `<script`, `javascript:`, `/etc/passwd`, `information_schema`, `../`, `cmd.exe` y `/bin/sh` en las cadenas de consulta (además de comprobaciones estrictas contra bytes nulos y traversal `%2e%2e`).
   - **Clientes automatizados (`desafío PoW`)**: Cuando aún no has definido reglas personalizadas, los clientes HTTP de consola (`curl`, `wget`, `python-requests`, `python-httpx`, `go-http-client`, `scrapy`), navegadores headless (`headlesschrome`, `playwright`, `puppeteer`, `selenium`) y cabeceras `User-Agent` vacías pasan a la pantalla Proof-of-Work.
2. **Inteligencia de amenazas IP autoactualizable**
   - Descarga [IPsum (level 3+)](https://raw.githubusercontent.com/stamparm/ipsum/master/levels/3.txt), [Spamhaus DROP IPv4](https://www.spamhaus.org/drop/drop_v4.json) y [IPv6](https://www.spamhaus.org/drop/dropv6.txt), y [abuse.ch Feodo Tracker](https://feodotracker.abuse.ch/downloads/ipblocklist.txt) al iniciar y cada 6 horas en segundo plano, guardando en caché en `<cert_dir>/intel-cache` con cabeceras condicionales `ETag` / `If-Modified-Since`.
   - Ejecuta consultas en vivo sin clave contra **Shodan InternetDB** (`internetdb.shodan.io`) y **StopForumSpam** (`api.stopforumspam.org`) con caché TTL en memoria y timeouts fail-open para que una caída externa nunca bloquee tu tráfico.
3. **Seguimiento oficial de IPs de rastreadores de IA**
   - Sigue los rangos CIDR publicados por **OpenAI** (`gptbot`, `searchbot`, `chatgpt-user`), **Anthropic** (`claude.com/crawling/bots.json`), **Perplexity** (`perplexitybot`, `perplexity-user`), **Google** (`user-triggered-agents`, `user-triggered-fetchers-google`) y **Apple** (`applebot`). Incluso si un rastreador falsifica un `User-Agent` de Chrome de escritorio, su IP de origen sigue coincidiendo con el rango de IA.

> **Nota operativa sobre los puertos 80 y 443:** Por defecto Gymbag escucha en `:80` (redirección HTTP→HTTPS + respondedor ACME HTTP-01) y `:443` (pasarela HTTPS). En Linux, concede `CAP_NET_BIND_SERVICE` al binario para no tener que ejecutarlo como root:
> ```bash
> sudo setcap 'cap_net_bind_service=+ep' ./gymbag
> ```
> Para pruebas locales sin privilegios ni `sudo`, pasa `--http-port 8080 --https-port 8443` (o usa `http_port` / `https_port` en `Gymbagfile`).

---

## Desarrollo Local (`localhost` y `*.internal`)

Probar HTTPS en local con nombres de host personalizados suele implicar lidiar con `mkcert`, editar `/etc/hosts` a mano y aceptar advertencias de certificados autofirmados. Gymbag gestiona automáticamente tanto la resolución de nombres como la emisión local X.509 siempre que un sitio use `localhost`, `*.localhost`, `*.internal` (como `app.internal` o `api.internal`), `*.local` o `*.test`:

```caddyfile
app.internal {
    reverse_proxy http://localhost:3000
    verify 3
}

api.internal {
    reverse_proxy http://localhost:8080
    protect off
}
```

### Configuración y Limpieza Automática de `/etc/hosts`

Cuando `auto_hosts` está activado (por defecto), Gymbag inspecciona `/etc/hosts` al arrancar (`C:\Windows\System32\drivers\etc\hosts` en Windows). Para cualquier dominio local concreto como `app.internal` o `api.internal` que aún no resuelva en `/etc/hosts`, Gymbag añade entradas de loopback IPv4 e IPv6 etiquetadas con el ID de su instancia:

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

Las líneas existentes de `localhost` quedan intactas. Al detener Gymbag (`Ctrl+C` / `SIGTERM`), vuelve a leer `/etc/hosts` para conservar cualquier edición manual realizada mientras se ejecutaba y elimina **únicamente** sus propias líneas `# gymbag-managed:<instanceID>`.

- **Permisos en macOS y Linux:** Escribir en `/etc/hosts` requiere privilegios de root. Si ejecutas `./gymbag` como usuario normal sin permiso de escritura en `/etc/hosts`, Gymbag registra una advertencia clara y **sigue funcionando con normalidad** — no fallará. Usa `sudo ./gymbag` cuando quieras gestión automática de `/etc/hosts` (y puertos `80`/`443`), o establece `auto_hosts off` (o `GYMBAG_AUTO_HOSTS=off`) si resuelves `.internal` con `dnsmasq` o gestionas `/etc/hosts` manualmente.

### PKI Local de Dos Niveles y Confianza en la CA Raíz

En lugar de generar certificados hoja autofirmados desechables cuya confianza los navegadores no guardan, Gymbag mantiene una Autoridad de Certificación ECDSA P-256 de dos niveles dentro de `<cert_dir>/ca/` (por defecto `certs/ca/`):

1. **Root CA**: **`"Neuro-sama Headquarters Root CA"`** (`certs/ca/neuro-sama-root-ca.pem`, validez de 10 años, `MaxPathLen: 1`)
2. **Intermediate CA**: **`"Gymbag & Swarm SSL Local CA"`** (`certs/ca/gymbag-swarm-local-ca.pem`, validez de 5 años, firmada por `"Neuro-sama Headquarters Root CA"`, `MaxPathLen: 0`)
3. **Certificado Hoja (Leaf)**: `certs/local-fullchain.pem` + `certs/local-privkey.pem` (validez de 90 días, autorrenovado a los 30 días de caducar, sirviendo la cadena hoja + intermedia con SANs DNS y SANs IP automáticos `127.0.0.1` / `::1`)

Confía en **`"Neuro-sama Headquarters Root CA"`** una sola vez en el almacén de confianza de tu sistema y cada dominio `https://localhost` y `https://*.internal` servido por Gymbag mostrará el candado verde en navegadores y `curl`:

**macOS (Llavero del sistema):**
```bash
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain certs/ca/neuro-sama-root-ca.pem
```

**Debian / Ubuntu:**
```bash
sudo cp certs/ca/neuro-sama-root-ca.pem /usr/local/share/ca-certificates/neuro-sama-root-ca.crt && sudo update-ca-certificates
```

**Fedora / RHEL / Arch Linux:**
```bash
sudo trust anchor --store certs/ca/neuro-sama-root-ca.pem
```

> **Nota para Firefox:** En macOS y Windows, Firefox confía automáticamente en las raíces del SO cuando `security.enterprise_roots.enabled` es `true` en `about:config`. En Linux, si Firefox usa su propia base de datos NSS en lugar de p11-kit, activa `security.enterprise_roots.enabled` o importa una vez `certs/ca/neuro-sama-root-ca.pem` en *Ajustes -> Privacidad y seguridad -> Certificados -> Ver certificados -> Autoridades*.

---

## Protección de Múltiples Sitios Web y Niveles de Verificación (`0`–`3`)

Un solo proceso de Gymbag puede alojar y hacer proxy inverso de decenas de dominios a la vez, incluyendo balanceo round-robin con múltiples upstreams y actualizaciones WebSocket RFC 6455 full-duplex. Cada sitio elige su propia política mediante `verify <0-3>` (o desactiva los desafíos WAF con `protect off`):

| Nivel | Nombre | Quién Recibe el Desafío | Mejor Uso |
| :---: | :--- | :--- | :--- |
| `0` | `always` | Todo visitante sin una cookie de autorización HMAC válida. | Herramientas privadas, entornos de staging o sitios bajo ataque L7 activo. |
| `1` | `only suspicious` | IPs en feeds de amenazas/mala reputación, escáneres, clientes CLI, navegadores headless, cabeceras ausentes o falsificadas y sondeos de exploits. | Aplicaciones web públicas y APIs donde los navegadores normales no deben ver pantallas de verificación. |
| `2` | `only ai` | Rastreadores de IA y agentes LLM detectados por **rangos IP oficiales de crawlers de IA** *o* cabeceras/`User-Agent` de bots de IA. | Documentación, blogs y forjas Git que buscan cero fricción para humanos y usuarios de `curl` mientras frenan a scrapers de LLM. |
| `3` | `suspicious & ai` | Combina `1` y `2`: desafía tanto a clientes/IPs sospechosos como a rastreadores de IA; los navegadores humanos pasan directo. | Valor predeterminado recomendado para producción pública (también activado con `verify` sin argumentos). |
| — | `protect off` | Desactiva los desafíos Proof-of-Work; actúa como proxy inverso HTTPS y WebSocket puro (las reglas personalizadas explícitas del sitio siguen ejecutándose si existen). | Servidores de archivos estáticos, webhooks y mirrors públicos de paquetes. |

```caddyfile
app.example.com, www.example.com {
    reverse_proxy http://localhost:3000 http://localhost:3001
    verify 3
    difficulty 4
}

docs.example.com {
    reverse_proxy http://localhost:4000
    verify 2
}

api.example.com {
    reverse_proxy http://localhost:8080
    verify 1
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
}

static.example.com {
    reverse_proxy http://localhost:9000
    protect off
}
```

Una vez que el navegador resuelve un desafío Proof-of-Work SHA-256 (`difficulty` `1`–`5`, por defecto `4`), Gymbag establece una cookie firmada con HMAC (`waf_clearance`, `HttpOnly`, `SameSite=Lax`, TTL por defecto `24h` / `86400s`) vinculada tanto al host como a la IP del cliente.

### Duración Personalizada del Estado Verificado (`verify_timeout` / `clearance_ttl`)

Personaliza cuánto tiempo permanece verificado un visitante de forma global o por sitio usando segundos enteros **o** duraciones legibles (`30m`, `12h`, `1h30m`, `7d`, `2w`):

```caddyfile
{
    verify_timeout 12h     # Predeterminado global (alias: verified_timeout, clearance_ttl)
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
    verify_timeout 30m     # Anulación por sitio
}
```

También puedes pasar `--verify-timeout 6h` (o `--clearance-ttl 6h`) a `gymbag init`, `gymbag run` o `gymbag reverse-proxy`, establecer `GYMBAG_VERIFY_TIMEOUT` / `GYMBAG_CLEARANCE_TTL`, o actualizar `clearance_ttl` en caliente mediante `PATCH /api/v1/sites/{domain}`. `GET /.waf/v1/session` devuelve `clearance_ttl`, `expires_at` y los segundos restantes en `expires_in` para visitantes verificados.

---

## Rutas por Path, Archivos Estáticos, Redirecciones e Importación de Configuraciones

Además de un simple `reverse_proxy`, un sitio puede contener **rutas por path** ordenadas. La primera ruta cuyo path coincida atiende la solicitud; lo que no coincida con ninguna ruta pasa a los upstreams predeterminados del sitio (o `404` si no hay). Cada ruta tiene exactamente un handler: proxy inverso, servidor de archivos estáticos (`root`), `respond` fijo o `redir`.

```caddyfile
app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    handle_path /static/* {
        root ./dist
        file_server browse
    }
    @health path /healthz /readyz
    respond @health "ok" 200
    redir /old/* https://{host}/new{uri} 301
    handle /api/* {
        reverse_proxy http://localhost:8080
        header X-Served-By gymbag
    }
    try_files {path} /index.html          # Fallback SPA
}
```

Los matchers de path son un path exacto (`/exact`), un glob (`/api/*`, `*.php`) o una regex Go RE2 con prefijo `~` (`~^/v[0-9]+/`). Los destinos de redirección admiten `{scheme}`, `{host}`, `{uri}`, `{path}` y `{query}`. Las rutas estáticas admiten `index`, `browse` (listados de directorio) y `fallback` (p. ej. `/index.html` para SPA); los `headers` de ruta establecen una cabecera, o la eliminan cuando la clave lleva el prefijo `-`. Las reglas WAF y la verificación se ejecutan siempre antes de cualquier handler de ruta.

En `gymbag.json` lo mismo es un array `routes`:

```json
{"routes": [
  {"path": "/static/*", "strip_prefix": "/static", "root": "./dist", "browse": true},
  {"path": "/old/*", "redirect": {"to": "https://{host}/new{uri}", "code": 301}},
  {"path": "/api/*", "upstreams": ["http://localhost:8080"]}
]}
```

Gestiona las rutas desde la CLI con `gymbag config route list|add|update|remove`, o en vivo mediante `PATCH /api/v1/sites/{domain}` (`routes`, `headers`, `rule_sources`).

**Importación de configuraciones existentes:** `gymbag run -c /etc/nginx/nginx.conf` (o `httpd.conf` / `apache2.conf`) sirve directamente una configuración nativa de nginx/Apache, y `gymbag config convert -c <file> -o gymbag.json --format json` convierte nginx, Apache o Caddyfile v2 (snippets, `import`, `@matchers`, `handle`, `route`) al formato nativo de Gymbag.

---

## Cambio de Proveedores SSL, Claves Propias y Renovación Automática

Para dominios públicos, `tls auto` (predeterminado) obtiene certificados automáticamente mediante RFC 8555 ACME HTTP-01 usando Let's Encrypt. Puedes cambiar de Autoridad de Certificación con una sola directiva `ssl_provider` (alias: `acme_provider`, `ca`, `cert_issuer`) o `tls <provider>`:

| Palabra Clave | Autoridad de Certificación | Requisito EAB |
| :--- | :--- | :--- |
| `letsencrypt` (`le`) | Let's Encrypt (Producción) | Ninguno |
| `letsencrypt_staging` | Let's Encrypt (Staging) | Ninguno |
| `zerossl` | ZeroSSL | **Automático** cuando `email` está configurado, o explícito `eab <kid> <hmac_key>` |
| `google` (`gts`) | Google Trust Services | Requiere `eab <kid> <hmac_key>` |
| `ssl.com` (`sslcom`, `ssl.com-rsa`) | SSL.com (ECC o RSA) | Requiere `eab <kid> <hmac_key>` |
| `litessl` (`trustasia`) | LiteSSL by TrustAsia | Ninguno para DV estándar (o sobrescribe la URL con `acme_directory`) |
| `buypass` | Buypass Go SSL | Ninguno |
| `local` | PKI Local de Dos Niveles | CA local (`Neuro-sama Headquarters Root CA` -> `Gymbag & Swarm SSL Local CA`) |
| `https://...` | Cualquier CA RFC 8555 ACME v2 | `eab <kid> <hmac_key>` opcional (p. ej. Smallstep `step-ca`, Pebble) |

### Cambio de CAs ACME y External Account Binding (EAB)

**ZeroSSL con EAB automático por correo electrónico:**
Al seleccionar `zerossl` y configurar `email`, Gymbag intercambia automáticamente tu correo con el endpoint EAB de ZeroSSL (`https://api.zerossl.com/acme/eab-credentials-email`) si no proporcionaste claves EAB explícitas:

```caddyfile
{
    email ops@example.com
    ssl_provider zerossl
}

example.com {
    reverse_proxy http://localhost:3000
    verify 3
}
```

**Google Trust Services, SSL.com o credenciales EAB explícitas:**
Pasa tu Key ID y clave HMAC (se aceptan tanto base64 URL-safe sin relleno como base64 estándar con relleno, y `eab_hmac_key` se oculta automáticamente en la salida de la API Admin):

```caddyfile
{
    email ops@example.com
    ssl_provider google
    eab YOUR_EAB_KEY_ID YOUR_EAB_HMAC_KEY
}
```

**LiteSSL by TrustAsia o un directorio ACME personalizado:**

```caddyfile
{
    email ops@example.com
    ssl_provider litessl
    # O apunta directamente a cualquier URL de directorio ACME v2:
    # acme_directory https://ca.internal.corp/acme/acme/directory
}
```

Gymbag guarda los certificados ACME en disco separados por proveedor (por ejemplo `certs/acme-zerossl-fullchain.pem` y `certs/acme-google-fullchain.pem`, manteniendo `certs/acme-fullchain.pem` para Let's Encrypt). Si cambias `ssl_provider` y reinicias o recargas, Gymbag solicita inmediatamente un certificado a la nueva CA en vez de servir uno antiguo del emisor anterior.

### Uso de Claves o Certificados Propios

- **Clave de cuenta ACME personalizada (`account_key`)**: Por defecto Gymbag crea y reutiliza una clave ECDSA P-256 en `certs/acme-account.pem`. Si tu organización ya tiene una clave ACME ECDSA (P-256/P-384) o RSA registrada, apúntala con `account_key /etc/gymbag/account.pem` (o `GYMBAG_ACCOUNT_KEY_FILE`).
- **Clave privada CSR personalizada (`ssl_key`)**: Usa `ssl_key /etc/gymbag/domain.key` para firmar los CSR de ACME con tu propia clave privada ECDSA o RSA en lugar de generar una nueva.
- **Certificado TLS y clave propios (`tls <cert> <key>`)**: Aporta archivos PEM comerciales, Cloudflare Origin o gestionados externamente de forma global o por sitio (enrutados por SNI). Las claves privadas cifradas son compatibles con `key_password` (o `GYMBAG_KEY_PASSWORD`):

```caddyfile
{
    key_password super-secret-passphrase
}

corp.example.com {
    reverse_proxy http://localhost:8080
    tls /etc/ssl/corp-fullchain.pem /etc/ssl/corp-privkey.pem
    verify 1
}
```

### Renovación en Segundo Plano Sin Tiempo de Inactividad

Gymbag ejecuta un bucle de renovación en segundo plano cada `renew_interval` segundos (por defecto `43200` = 12 horas). Siempre que un certificado ACME, hoja PKI local o de archivo esté a menos de `renew_within_days` días de caducar (por defecto `30` días), Gymbag lo renueva o recarga en segundo plano e intercambia el puntero `tls.Certificate` activo de forma atómica. Las conexiones HTTP en curso y los túneles WebSocket permanecen activos sin perder ni una conexión TCP.

---

## Escribir Tus Propias Reglas de Firewall

Para cada solicitud entrante en un sitio protegido, Gymbag evalúa las capas en un orden predecible:

1. **Comprobación de forma de solicitud** (verificación de verbo HTTP, longitud máxima de URI, rechazo de bytes nulos y traversal `..`)
2. **Exención ACME HTTP-01** (`/.well-known/acme-challenge/*`)
3. **Tus reglas personalizadas locales** (primero globales, luego del sitio, de arriba abajo — gana la primera coincidencia)
4. **Reglas remotas (`remote_rules`)** (conjuntos de reglas HTTP/HTTPS autoactualizados en sintaxis Gymbagfile o JSON)
5. **Reglas base de seguridad integradas** (salvo que `default_rules off` esté activo)
6. **Limitador de tasa por sitio y por IP** (`rate_limit` / `rate_window`)
7. **Inteligencia de amenazas IP** (`ip_security_action verify` o `block`)
8. **Verificación Proof-of-Work** (`verify` nivel `0`–`3`)

Como tus reglas personalizadas se ejecutan **antes** que las reglas base integradas, el limitador de tasa y los feeds de amenazas, colocar una regla `allow` al inicio de un bloque de sitio incluye en lista blanca de forma limpia CIDRs de oficina de confianza, health checks internos o cookies de vista previa.

### Reglas Cortas en Una Línea

Sintaxis: `<allow | verify | block> <campo> <patrón...>` (también funcionan los alias `bypass`/`pass`, `challenge`/`pow` y `deny`/`drop`).

| Campo | Valores Aceptados y Coincidencia de Patrones | Ejemplo |
| :--- | :--- | :--- |
| `ip` | IPv4, IPv6 o rangos CIDR (separados por espacio o coma) | `allow ip 127.0.0.1 10.0.0.0/8 2001:db8::/32` |
| `user_agent` | Subcadena sin distinción de mayúsculas, comodín `*` o `~regex` | `block user_agent BadBot ~^(?i)python-urllib/` |
| `path` | Exacto/subcadena, comodín `*` o `~regex` | `verify path /api/export* ~^/v[0-9]+/search` |
| `query` | Coincidencia de cadena query sin procesar, `*` o `~regex` | `block query "union select" ~(?i)<script` |
| `method` | Verbos HTTP (insensible a mayúsculas) | `verify method POST DELETE` |
| `header` | `<Header-Name> [patrón]` (`""` = ausente/vacío, `*` = presente) | `verify header Accept-Language ""` |
| `cookie` | `<cookie_name> [patrón]` (`""` = ausente/vacía, `*` = presente) | `allow cookie vip_session *` |

### Bloques `rule` Multicondición

Cuando un solo campo no basta, utiliza un bloque `rule "nombre" { ... }`. Varios valores en el **mismo** campo coinciden con **OR**; distintos campos dentro del mismo bloque coinciden con **AND**:

```caddyfile
app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    # Exigir PoW en peticiones POST a rutas de login o recuperación de contraseña
    rule "challenge-auth-mutations" {
        action verify
        method POST
        path /login* /api/auth/* /reset-password*
    }

    # Bloquear peticiones POST a /api/* sin cabecera Origin
    rule "block-headless-api-posts" {
        action block
        method POST
        path /api/*
        header Origin ""
    }
}
```

### Reglas Remotas de Firewall (`remote_rules`)

Obtén reglas compartidas desde una o más URLs HTTP/HTTPS de forma global o por sitio. Gymbag soporta tanto la **sintaxis de reglas de Gymbagfile** (líneas `allow`/`verify`/`block` y bloques `rule "name" { ... }`) como **JSON** (`[ ... ]` o `{"rules": [ ... ]}`), cachea cargas validadas en `<cert_dir>/rules-cache` mediante peticiones condicionales `ETag` / `If-Modified-Since` y conserva el último conjunto válido si una fuente remota deja de estar disponible o llega malformada:

```caddyfile
{
    remote_rules https://rules.example.com/org-baseline.rules
    remote_rules_refresh 5m
}

app.example.com {
    reverse_proxy http://localhost:3000
    verify 3
    allow ip 10.0.0.0/8                              # Las reglas locales siempre van primero
    remote_rules https://rules.example.com/app.json  # Reglas remotas específicas del sitio
}
```

Inspecciona o fuerza una actualización inmediata con `GET /api/v1/rules/remote` y `POST /api/v1/rules/remote/refresh`, o mediante `--remote-rules` / `GYMBAG_REMOTE_RULES`.

### Archivos de Reglas Locales, Carpetas de Reglas y Recarga Dinámica en Caliente

Guarda las reglas en un archivo `.rules`/`.json` dedicado o en un directorio completo de reglas y referéncialos por ruta (o enlázalos con `gymbag ruleset add`). Marca una fuente como `dynamic` para vigilarla y recargar en caliente los cambios en disco sin reiniciar el servidor (si un archivo tiene sintaxis inválida, Gymbag conserva el último conjunto válido):

```caddyfile
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

En `gymbag.json`, configura `"rule_sources": [{"source": "./rules.d", "dynamic": true, "name": "shared"}]` o pasa `--rules-file`, `--rules-dir`, `--rules-dynamic` y `--rules-watch-interval` a `gymbag init`.

### Control de Reglas Integradas (`default_rules on|off`)

`default_rules` está activado (`on`) por defecto:
- Cuando un sitio protegido **no tiene** reglas personalizadas, Gymbag aplica tanto los preajustes de bloqueo integrados (`/.env`, `/wp-admin`, escáneres, SQLi/XSS) como los preajustes de verificación para clientes automatizados.
- Cuando añades tus propias reglas personalizadas, tus reglas se ejecutan primero y los preajustes de **bloqueo** integrados permanecen detrás para que no expongas accidentalmente `/.git` o `/.env` solo por añadir una regla `verify path`.
- Si deseas control manual total sin reglas integradas en un sitio (o globalmente), establece `default_rules off` (o `GYMBAG_DEFAULT_RULES=off`).

---

## CLI, Docker Compose, Variables de Entorno y APIs REST

### Referencia de CLI

Gymbag carga automáticamente `.env` del directorio de trabajo si existe, y luego resuelve `./gymbag.json`, `./Gymbagfile` o `./Caddyfile` salvo que se indique otra ruta por flags (también puedes pasar directamente un `nginx.conf` o un `httpd.conf`/`apache2.conf` de Apache nativos con `-c`):

```bash
# Generar gymbag.json (por defecto) o Gymbagfile con asistente interactivo o flags
./gymbag init
./gymbag init -d app.example.com -u http://localhost:3000 -l 3 --rules-dir ./rules.d --rules-dynamic
./gymbag init --format gymbagfile -o Gymbagfile -d app.example.com -u :3000

# Gestionar sitios, rutas y ajustes globales en gymbag.json o Gymbagfile
./gymbag config list
./gymbag config add -d api.example.com -u :8080 -l 1
./gymbag config update -d api.example.com -l 3 --verify-timeout 6h
./gymbag config route add -d api.example.com --path /static/* --strip-prefix /static --root ./dist
./gymbag config remove -d api.example.com
./gymbag config set admin 127.0.0.1:2019
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json

# Gestionar archivos de reglas dedicados, carpetas de reglas y reglas en línea
./gymbag ruleset init ./rules/custom.rules
./gymbag ruleset add --source ./rules --dynamic --name local-dir
./gymbag ruleset update local-dir --static
./gymbag ruleset remove local-dir
./gymbag ruleset validate ./rules
./gymbag ruleset rule add --file ./rules/custom.rules --action block --name bad-bot --ua "*BadBot*"

# Iniciar el servidor usando ./gymbag.json, ./Gymbagfile o ./Caddyfile
./gymbag

# Iniciar con archivo explícito (JSON, Gymbagfile, Caddyfile v2, nginx, Apache) y recarga en caliente al guardar
./gymbag run --config gymbag.json --watch

# Registrar y gestionar como servicio del sistema (systemd, launchd, openrc, supervisor)
./gymbag service install --config gymbag.json
./gymbag service start
./gymbag service status
./gymbag service render --type systemd   # Previsualizar una unidad systemd en cualquier SO

# Proxy inverso ad-hoc sin archivo de configuración
./gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3

# Validar la configuración y las fuentes de reglas locales e imprimir el resumen de seguridad resuelto por sitio
./gymbag validate --config gymbag.json

# Formatear Gymbagfile en su lugar con sangría canónica de 4 espacios
./gymbag fmt --overwrite Gymbagfile

# Imprimir versión
./gymbag version
```

| Flag | Descripción |
| :--- | :--- |
| `-c`, `--config <path>` | Ruta a `gymbag.json`, `Gymbagfile`, `Caddyfile`, `nginx.conf` o archivo de configuración de Apache |
| `-d`, `--domain <list>` | Sobrescribir dominio(s), separados por comas |
| `-u`, `--upstream <url>` | Sobrescribir URL upstream (p. ej. `http://localhost:3000` o `:3000`) |
| `-l`, `--verify <0-3>` | Sobrescribir nivel de verificación (`0`, `1`, `2`, `3` o `off`) |
| `--verify-timeout <dur>` | Sobrescribir duración de estado verificado (p. ej. `30m`, `12h`, `7d` o segundos; alias `--clearance-ttl`) |
| `--remote-rules <url>` | Adjuntar URL de reglas remotas de firewall (http/https, repetible o separado por comas) |
| `--remote-rules-refresh <dur>` | Intervalo de actualización para reglas remotas (por defecto `5m` / `300s`) |
| `--no-protect` | Desactivar verificación WAF para sitios configurados por CLI |
| `--ssl-provider <name>` | Sobrescribir proveedor ACME / SSL (p. ej. `letsencrypt`, `zerossl`, `google`, `litessl`, `local`) |
| `--admin <addr>` | Habilitar API Admin REST local (p. ej. `127.0.0.1:2019` u `off`) |
| `--http-port <port>` | Puerto de redirección HTTP y ACME HTTP-01 (por defecto `80`) |
| `--https-port <port>` | Puerto de pasarela HTTPS (por defecto `443`) |
| `--watch` | Vigilar cambios en el archivo de configuración y recargar rutas, reglas y certs SNI |

### Docker y Docker Compose

Se publican imágenes Alpine oficiales multiarquitectura (`linux/amd64`, `linux/arm64`) en GitHub Container Registry (`ghcr.io/evil-project/gymbag`) como usuario no-root UID `10001` con `cap_net_bind_service=+ep`.

**`docker run` rápido montando un `Gymbagfile`:**

```bash
docker run -d \
  --name gymbag \
  --restart unless-stopped \
  -p 80:80 -p 443:443 \
  --cap-add NET_BIND_SERVICE \
  --add-host=host.docker.internal:host-gateway \
  -v "$(pwd)/Gymbagfile:/app/Gymbagfile:ro" \
  -v gymbag-certs:/app/certs \
  ghcr.io/evil-project/gymbag:latest
```

**`docker run` sin archivo de configuración usando variables de entorno:**

```bash
docker run -d \
  --name gymbag \
  -p 80:80 -p 443:443 \
  --cap-add NET_BIND_SERVICE \
  --add-host=host.docker.internal:host-gateway \
  -e GYMBAG_DOMAIN=app.example.com \
  -e GYMBAG_UPSTREAM=http://host.docker.internal:3000 \
  -e GYMBAG_ACME_EMAIL=ops@example.com \
  -e GYMBAG_VERIFY_LEVEL=3 \
  -e GYMBAG_VERIFY_TIMEOUT=12h \
  -e GYMBAG_REMOTE_RULES=https://rules.example.com/baseline.rules \
  -v gymbag-certs:/app/certs \
  ghcr.io/evil-project/gymbag:latest
```

**Docker Compose con Redis:**

El archivo `compose.yaml` incluido ejecuta Gymbag junto a `redis:8-alpine` usando el cliente Redis RESP2 en Go puro de Gymbag para compartir nonces de un solo uso PoW y límites de tasa de ventana fija entre reinicios o múltiples réplicas:

```bash
docker compose up -d --build
```

La configuración se lee desde `./Gymbagfile` (montado solo lectura en `/app/Gymbagfile`), y los certificados, secretos, cachés de inteligencia de amenazas y cachés de reglas remotas persisten en el volumen `gymbag-certs`. Cualquier variable `GYMBAG_*` en un archivo `.env` local sobrescribe limpiamente la configuración del archivo:

```dotenv
GYMBAG_ACME_EMAIL=ops@example.com
GYMBAG_SSL_PROVIDER=zerossl
GYMBAG_VERIFY_LEVEL=3
GYMBAG_CLEARANCE_TTL=12h
GYMBAG_REMOTE_RULES=https://rules.example.com/baseline.rules
```

Consulta la [Guía completa de Docker y Docker Compose](docs/es/guide/docker.md) para permisos en montajes bind (`chown 10001:10001`), recarga en caliente con `--watch`, proxy multicontenedor y compilación local.

### Variables de Entorno (`GYMBAG_*`)

El orden de prioridad es **Archivo de configuración -> Variables de entorno `GYMBAG_*` -> Flags de CLI**.

| Variable | Descripción |
| :--- | :--- |
| `GYMBAG_CONFIG` | Ruta a `Gymbagfile`, `Caddyfile` o archivo `.json` |
| `GYMBAG_BIND` | Dirección de enlace (`127.0.0.1` para dominios locales, `0.0.0.0` cuando existen dominios públicos) |
| `GYMBAG_HTTP_PORT` / `GYMBAG_HTTPS_PORT` | Puertos de escucha HTTP y HTTPS (por defecto `80` y `443`) |
| `GYMBAG_DOMAIN` / `GYMBAG_DOMAINS` | Dominio(s) downstream separados por comas |
| `GYMBAG_UPSTREAM` | URL upstream de destino por defecto |
| `GYMBAG_UPSTREAMS` | Mapa de hosts separado por punto y coma: `app.example.com=http://web:3000;api.example.com=http://api:8080` |
| `GYMBAG_PROTECT` | Activa o desactiva la protección WAF en todos los sitios (`on` / `off`) |
| `GYMBAG_PROTECT_DOMAINS` | Subconjunto de dominios con protección WAF activada (separados por comas) |
| `GYMBAG_VERIFY_LEVEL` | Nivel de verificación: `0` (`always`), `1` (`only suspicious`), `2` (`only ai`), `3` (`suspicious & ai`) |
| `GYMBAG_DEFAULT_RULES` | Activa o desactiva las reglas base integradas (`on` / `off`, por defecto `on`) |
| `GYMBAG_REMOTE_RULES` | URLs HTTP/HTTPS de reglas remotas de firewall separadas por comas o espacios |
| `GYMBAG_REMOTE_RULES_REFRESH` | Intervalo de actualización de reglas remotas en segundos o duración (por defecto `300` / `5m`) |
| `GYMBAG_RULES_WATCH_INTERVAL` | Intervalo de sondeo en segundos o duración para archivos/carpetas de reglas locales dinámicos (por defecto `2` / `2s`) |
| `GYMBAG_DIFFICULTY` | Dificultad de ceros hexadecimales iniciales SHA-256 PoW (`1`–`5`, por defecto `4`) |
| `GYMBAG_CHALLENGE_TTL` / `GYMBAG_CLEARANCE_TTL` | Expiración del desafío (`300`s / `5m`) y duración del estado verificado (`86400`s / `24h`; alias `GYMBAG_VERIFY_TIMEOUT`, `GYMBAG_VERIFIED_TIMEOUT`; acepta `30m`, `12h`, `7d`) |
| `GYMBAG_COOKIE_NAME` / `GYMBAG_COOKIE_SECURE` | Nombre de cookie de autorización (`waf_clearance`) y flag `Secure` (`true`) |
| `GYMBAG_RATE_LIMIT` / `GYMBAG_RATE_WINDOW` | Límite de peticiones por IP (`240`) y ventana en segundos (`60`) |
| `GYMBAG_TLS_MODE` | `auto`, `acme`, `local`, `selfsigned` o `file` |
| `GYMBAG_SSL_PROVIDER` | `letsencrypt`, `letsencrypt_staging`, `zerossl`, `google`, `ssl.com`, `litessl`, `buypass`, `local` o URL |
| `GYMBAG_ACME_EMAIL` | Correo de contacto para registro ACME (y EAB automático de ZeroSSL) |
| `GYMBAG_ACME_DIRECTORY` | URL explícita del directorio RFC 8555 ACME v2 |
| `GYMBAG_EAB_KID` / `GYMBAG_EAB_HMAC_KEY` | Key ID de EAB y clave HMAC en base64/base64url |
| `GYMBAG_ACCOUNT_KEY_FILE` | Ruta a clave de cuenta ACME ECDSA o RSA en formato PEM |
| `GYMBAG_CERT_FILE` / `GYMBAG_KEY_FILE` | Rutas del certificado TLS y clave privada para modo `file` |
| `GYMBAG_KEY_PASSWORD` | Contraseña para claves privadas PEM cifradas |
| `GYMBAG_CERT_DIR` | Directorio para certificados CA, estado ACME y caché de inteligencia (por defecto `certs`) |
| `GYMBAG_RENEW_INTERVAL` / `GYMBAG_RENEW_WITHIN_DAYS` | Intervalo de comprobación de renovación en segundos (`43200`) y umbral en días (`30`) |
| `GYMBAG_AUTO_HOSTS` | Gestión automática de `/etc/hosts` para `localhost` y `*.internal` (`on` / `off`, por defecto `on`) |
| `GYMBAG_HSTS` | Sobrescribir cabecera `Strict-Transport-Security` (`on` / `off`) |
| `GYMBAG_IP_SECURITY` / `GYMBAG_IP_SECURITY_ACTION` | Alternar feeds de amenazas (`on`/`off`) y acción (`verify` o `block`) |
| `GYMBAG_IP_REPUTATION` | Alternar consultas en vivo Shodan InternetDB + StopForumSpam (`on`/`off`) |
| `GYMBAG_AI_IP_LIST` | Alternar listas oficiales de IPs de OpenAI, Anthropic, Perplexity, Google y Applebot (`on`/`off`) |
| `GYMBAG_INTEL_REFRESH` | Intervalo de actualización de feeds de amenazas e IA en segundos (por defecto `21600`) |
| `GYMBAG_REDIS_URL` | URL opcional de conexión Redis (`redis://[:password@]host:port/db`) |
| `GYMBAG_SECRET` | Secreto HMAC para cookies de autorización (autogenerado en `<cert_dir>/secret` si no se define) |
| `GYMBAG_ADMIN` | Dirección de escucha de la API REST Admin local (p. ej. `127.0.0.1:2019` u `off`) |

### APIs REST

#### Endpoints Públicos WAF (Servidos en Sitios Protegidos)

| Método | Ruta | Descripción |
| :--- | :--- | :--- |
| `GET` | `/.waf/v1/session` | Devuelve el estado actual de autorización del sitio, `protected`, `verify_level` (`0`–`3`), `verify_mode`, `clearance_ttl` y (cuando está verificado) `expires_at` / `expires_in` |
| `DELETE` | `/.waf/v1/session` | Limpia la cookie de autorización del visitante (`204 No Content`) |
| `POST` | `/.waf/v1/challenges` | Emite un desafío PoW SHA-256 de un solo uso (`201 Created`) |
| `POST` | `/.waf/v1/verifications` | Verifica la solución PoW, establece la cookie firmada y devuelve `expires_in` (soporta alias `/.waf/verify`) |
| `GET` | `/.waf/assets/{name}` | Sirve recursos PNG integrados de la página de desafío (alias `/.waf/v1/assets/{name}`, con caché `ETag`) |

#### API Admin Local (Activada mediante `admin 127.0.0.1:2019` o `--admin`)

Enlaza la API Admin a loopback (`127.0.0.1:2019`) o a una interfaz de gestión Unix/WireGuard — no requiere token de autenticación y puede modificar rutas y reglas de firewall en vivo sin reiniciar el servidor.

| Método | Ruta | Descripción |
| :--- | :--- | :--- |
| `GET` | `/api/v1/health` | Estado de la pasarela, número de sitios, `tls_source` activo, `threat_entries`, `ai_entries` y `remote_rules` cargados |
| `GET` | `/api/v1/tls` | Origen TLS activo, ruta del certificado, proveedor SSL configurado, estado HSTS y catálogo de CAs soportadas |
| `POST` | `/api/v1/tls/renew` | Fuerza una renovación inmediata de certificado y recarga atómica en caliente |
| `GET` | `/api/v1/intel` | Conteos por feed de amenazas e IPs de IA, marcas de última actualización y estadísticas de caché de reputación |
| `POST` | `/api/v1/intel/refresh` | Dispara una actualización inmediata en segundo plano de los feeds y devuelve conteos actualizados |
| `GET` | `/api/v1/rules/remote` | Conteos de reglas remotas por URL, estado de caché, intervalo de refresco y últimos errores |
| `POST` | `/api/v1/rules/remote/refresh` | Descarga inmediata y recarga atómica del motor WAF para todas las URLs de reglas remotas |
| `GET` | `/api/v1/config` | Volcado de la configuración activa en ejecución (`secret`, `key_password` y `eab_hmac_key` ocultados) |
| `PUT` | `/api/v1/config` | Valida y reemplaza atómicamente toda la configuración en ejecución |
| `GET` | `/api/v1/sites` | Lista todos los sitios web configurados y sus políticas WAF |
| `POST` | `/api/v1/sites` | Añade un nuevo sitio web en tiempo de ejecución (`201 Created`) |
| `GET` | `/api/v1/sites/{domain}` | Inspecciona la configuración de un único sitio web |
| `PUT` | `/api/v1/sites/{domain}` | Reemplaza la configuración de un sitio web |
| `PATCH` | `/api/v1/sites/{domain}` | Parchea en vivo `verify_level`, `protect`, `difficulty`, `clearance_ttl`, `upstreams`, `rate_limit`, `rules`, `remote_rules`, `routes`, `headers` o `rule_sources` |
| `DELETE` | `/api/v1/sites/{domain}` | Elimina un sitio web (`204 No Content`; rechaza borrar el último sitio restante) |
| `GET` | `/api/v1/sites/{domain}/rules` | Lista reglas de firewall personalizadas en un sitio web |
| `POST` | `/api/v1/sites/{domain}/rules` | Añade una regla de firewall personalizada en vivo (`201 Created`) |
| `PUT` | `/api/v1/sites/{domain}/rules` | Reemplaza todas las reglas de firewall personalizadas de un sitio web |
| `DELETE` | `/api/v1/sites/{domain}/rules/{index_or_name}` | Elimina una regla personalizada por índice numérico o por `name` (`204 No Content`) |

**Ejemplos de mitigación de emergencia en vivo:**

```bash
# Endurecer app.example.com a verify level 0 (desafiar siempre) durante un pico de tráfico
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"verify_level": 0, "difficulty": 5}'

# Bloquear de inmediato un CIDR abusivo sin editar Gymbagfile
curl -X POST http://127.0.0.1:2019/api/v1/sites/app.example.com/rules \
  -H "Content-Type: application/json" \
  -d '{"name": "drop-abusive-subnet", "action": "block", "ips": ["198.51.100.0/24"]}'

# Forzar actualización inmediata de listas de amenazas IP y rastreadores de IA
curl -X POST http://127.0.0.1:2019/api/v1/intel/refresh
```

---

## Benchmarks vs Caddy y Nginx

Probado en Apple M4 (10 núcleos, macOS, Go `1.27.0`), terminando TLS 1.3 con el mismo certificado ECDSA P-256, 64 workers concurrentes, respuesta de `1 KiB` y `1024` conexiones keep-alive al upstream en los tres proxies (`0` errores en todas las pruebas):

| Servidor y Modo | HTTP/1.1 TLS (RPS) | H1 p50 / p99 | HTTP/2 TLS (RPS) | H2 p50 / p99 | Tamaño Binario | RSS (Bajo Carga) |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **85,203** | 0.45 / 6.20 ms | **88,775** | 0.65 / 1.96 ms | 1.5 MB (+libs) | 5.8 MiB |
| **Gymbag `v0.1.0`** (`verify 3`, WAF completo) | **72,517** | 0.74 / 3.14 ms | **66,096** | 0.82 / 3.48 ms | 12.9 MB (estático) | 54.3 MiB |
| **Gymbag `v0.1.0`** (`protect off`, proxy puro) | **72,479** | 0.74 / 3.15 ms | **64,048** | 0.83 / 3.94 ms | 12.9 MB (estático) | 52.1 MiB |
| **Gymbag `v0.1.0`** (`verify 0`, cookie válida) | **65,490** | 0.79 / 3.93 ms | **62,108** | 0.84 / 4.27 ms | 12.9 MB (estático) | ~54 MiB |
| **Caddy `v2.11.9`** (Go, keep-alive ajustado) | **54,741** | 0.90 / 4.90 ms | **39,207** | 1.30 / 6.18 ms | 49.0 MB | 76.8 MiB |

- **+32% más rápido que Caddy en HTTP/1.1** y **+63% a +69% más rápido en HTTP/2**, usando ~30% menos memoria y un binario 3.8 veces más pequeño — incluso con el WAF completo de Gymbag activo.
- **85% del rendimiento de Nginx en C sobre HTTP/1.1** con una latencia de cola p99 casi **2 veces menor** (`3.14 ms` vs `6.20 ms`).
- **140,680 req/s** de bloqueo WAF en el borde ante sondeos `/.env` sobre TLS (`0.35 ms` p50, cero tráfico al upstream).

Consulta [Benchmarks y Rendimiento](docs/es/guide/benchmarks.md) para ver la metodología completa, microbenchmarks en Go (`go test -bench`) y configuraciones exactas.

---

## Estructura del Proyecto

```text
.
├── cmd/gymbag/          # Punto de entrada CLI independiente
├── docs/                # Documentación VitePress multi-idioma (EN, 简体中文, 日本語, 한국어, Español)
├── internal/
│   ├── cli/             # Subcomandos (init, service, run, reverse-proxy, validate, fmt, version), .env y watcher
│   ├── config/          # Modelos de config, parsers de Gymbagfile/Caddyfile y duración/reglas remotas, cargador GYMBAG_*
│   ├── host/            # Normalización de dominios, corchetes IPv6, coincidencia con comodines
│   ├── intel/           # Conjunto de prefijos O(1), feeds autoactualizables de amenazas/IA, reputación IP en vivo
│   ├── localpki/        # CA local de dos niveles ("Neuro-sama Headquarters Root CA") y gestor de /etc/hosts
│   ├── proxy/           # Enrutador multisitio sin bloqueos, proxies HTTP con pool y WebSocket RFC 6455
│   ├── server/          # Pasarela HTTPS fusionada, redirector HTTP->HTTPS/ACME, API Admin REST
│   ├── service/         # Generador y gestor de servicios del sistema (systemd, launchd, openrc, supervisor)
│   ├── store/           # Interfaces StateStore, almacén en memoria de 64 shards, cliente Redis RESP2 en Go puro
│   ├── tlsmanager/      # ACME v2 multi-CA (EAB y ZeroSSL auto-EAB), PKI local, gestor SNI sin bloqueos
│   └── waf/             # Verificador PoW, caché HMAC sharded, DFA Aho-Corasick, reglas remotas, UI
├── Dockerfile
├── compose.yaml
├── flake.nix
├── Gymbagfile
└── go.mod
```

---

## Historial de Estrellas

<a href="https://www.star-history.com/#Evil-Project/gymbag&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
 </picture>
</a>
