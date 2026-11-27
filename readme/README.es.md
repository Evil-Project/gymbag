# Gymbag

<p align="center">
  <img src="../docs/public/mascot-happy.png" alt="Mascota de Gymbag" width="140" />
</p>

<p align="center">
  <strong>Gateway HTTP/HTTPS, SSH y Minecraft con Proof-of-Work en un único binario en Go puro</strong>
</p>

<p align="center">
  <a href="https://github.com/Evil-Project/gymbag/releases"><img src="https://img.shields.io/github/v/release/Evil-Project/gymbag?include_prereleases&label=Release" alt="Release" /></a>
  <a href="https://github.com/Evil-Project/gymbag/pkgs/container/gymbag"><img src="https://img.shields.io/badge/Docker-GHCR-blue?logo=docker" alt="Docker GHCR" /></a>
  <a href="../go.mod"><img src="https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go" alt="Go 1.27+" /></a>
  <a href="../LICENSE"><img src="https://img.shields.io/badge/License-Apache--2.0-green" alt="Apache-2.0 License" /></a>
  <a href="https://evil-project.github.io/gymbag/es/"><img src="https://img.shields.io/badge/Docs-VitePress-b81d33" alt="Docs" /></a>
</p>

<p align="center">
  <strong>README:</strong>
  <a href="../README.md">English</a> ·
  <a href="README.zh.md">简体中文</a> ·
  <a href="README.zh-TW.md">繁體中文</a> ·
  <a href="README.ja.md">日本語</a> ·
  <a href="README.ko.md">한국어</a> ·
  <a href="README.es.md">Español</a>
  <br />
  <sub>Documentación en línea: <a href="https://evil-project.github.io/gymbag/">EN</a> · <a href="https://evil-project.github.io/gymbag/zh/">ZH</a> · <a href="https://evil-project.github.io/gymbag/zh-TW/">ZH-TW</a> · <a href="https://evil-project.github.io/gymbag/ja/">JA</a> · <a href="https://evil-project.github.io/gymbag/ko/">KO</a> · <a href="https://evil-project.github.io/gymbag/es/">ES</a></sub>
</p>

Gymbag es un proxy inverso HTTPS y WAF con Proof-of-Work en un único binario (en la línea de Anubis) que también protege servidores SSH y Minecraft. Solo usa la biblioteca estándar de Go, sin dependencias de módulos externos; las páginas HTML, el resolvedor SHA-256 en el navegador y las imágenes de la mascota van compiladas en el binario mediante `go:embed`.

Las aplicaciones autoalojadas, forjas Git, puertos SSH y servidores de Minecraft reciben el mismo tráfico constante: scrapers de LLM que ignoran `robots.txt`, bots sondeando `/.env` y `/wp-admin`, ataques de fuerza bruta por SSH e inundaciones de login. En lugar de enlazar nginx o Caddy, un sidecar PoW, fail2ban y scripts de firewall a mano, Gymbag resuelve terminación TLS, ACME multi-CA, proxy HTTP/WebSocket, filtrado TCP para SSH/Minecraft, listas de amenazas IP/VPN/IA y reglas de firewall en un solo proceso configurado con `gymbag.json` o un `Gymbagfile` al estilo de Caddy.

## Inicio rápido

En [GitHub Releases](https://github.com/Evil-Project/gymbag/releases) se publican binarios estáticos precompilados (`CGO_ENABLED=0` para Linux, macOS y Windows en `amd64`/`arm64`), paquetes `.deb` para Debian/Ubuntu, `.pkg.tar.zst` para Arch y archivos/NAR para NixOS. También puedes usar la imagen en GHCR, el flake de Nix o compilar desde las fuentes:

```bash
go build -o gymbag ./cmd/gymbag
```

**Debian / Ubuntu:**
```bash
sudo apt install ./gymbag_<version>_linux_amd64.deb
sudo gymbag init -o /etc/gymbag/Gymbagfile --format gymbagfile
sudo systemctl enable --now gymbag
```

**Arch Linux:**
```bash
sudo pacman -U gymbag-<version>-1-x86_64.pkg.tar.zst
# O compilar desde packaging/arch:
cd packaging/arch && makepkg -si
```

**Nix / NixOS:**
```bash
nix run github:Evil-Project/gymbag -- --help
nix profile install github:Evil-Project/gymbag
```
Los binarios estáticos de Linux también funcionan directamente en NixOS sin `patchelf` ni `nix-ld`, y `nixosModules.default` expone el módulo declarativo `services.gymbag`.

Ponerlo en marcha lleva dos comandos: genera la configuración (por defecto escribe `gymbag.json`, o `Gymbagfile` con `--format gymbagfile`) y arranca:

```bash
# Proteger un sitio web HTTP/HTTPS
./gymbag init -d app.example.com -u http://localhost:3000
./gymbag

# Proteger SSH (mueve sshd a un puerto aleatorio y configura el firewall)
sudo ./gymbag init -t ssh
sudo ./gymbag

# Proteger Minecraft Java Edition (arranca automáticamente server.jar con Java autoseleccionado y puerto aleatorio)
./gymbag init -t minecraft --server-jar /srv/mc/paper.jar --mc-jvm-args "-Xms1G -Xmx4G"
./gymbag

# Proteger un servidor Minecraft existente sin arranque gestionado (asigna un server-port aleatorio en server.properties)
./gymbag init -t minecraft --server-properties /path/to/server.properties
./gymbag

# Proteger los tres desde un mismo proceso
sudo ./gymbag init -t http,ssh,minecraft -d app.example.com -u :3000 --server-jar /srv/mc/paper.jar
sudo ./gymbag
```

También puedes apuntar Gymbag directamente a una configuración existente de **nginx** (`nginx.conf`), **Apache** (`httpd.conf` / `apache2.conf`) o **Caddyfile v2**, o registrarlo como servicio del sistema (`systemd`, `launchd`, `openrc` o `supervisor`):

```bash
./gymbag init -d app.example.com -u :3000 --service auto
./gymbag service start
```

Para pruebas rápidas sin crear archivo de configuración:

```bash
./gymbag reverse-proxy --from app.internal --to :3000 --verify 3
```

> **Puertos por debajo de 1024:** `:80` gestiona las redirecciones HTTP a HTTPS y ACME HTTP-01, `:443` sirve HTTPS y `:22` escucha para SSH. En Linux, `sudo setcap 'cap_net_bind_service=+ep' ./gymbag` permite abrir esos puertos sin ejecutar como root (los paquetes `.deb` y de Arch ya aplican esta capacidad). Para pruebas locales sin privilegios, usa `--http-port 8080 --https-port 8443`.

### Qué se ejecuta sin escribir reglas

No hace falta redactar expresiones regulares ni importar listas antes de poner Gymbag delante del tráfico. Con escribir `verify` a secas (fija el nivel `3`: `suspicious & ai`) o `block` (cambia las coincidencias de inteligencia de amenazas de desafío a `403`), se activan tres capas predeterminadas:

1. **Reglas base integradas (`default_rules on`)**
   - **Rutas sensibles (`403`)**: `/.env`, `/.env.*`, `/.git`, `/.git/*`, `/.svn/*`, `/.hg/*`, `/.DS_Store`, `/.aws/*`, `/.ssh/*`, `/docker-compose.yml`, `/docker-compose.yaml`, `*.bak`, `*.sql`, `*.sqlite`.
   - **Sondeos a CMS y frameworks (`403`)**: `/wp-admin`, `/wp-admin/*`, `/wp-login.php`, `/xmlrpc.php`, `/phpmyadmin*`, `/vendor/phpunit*`, `/cgi-bin/*`, `/actuator/env*`, `/actuator/heapdump*`.
   - **Escáneres de vulnerabilidades (`403`)**: `sqlmap`, `nikto`, `nuclei`, `masscan`, `zgrab`, `gobuster`, `dirbuster`, `ffuf`, `wfuzz`, `feroxbuster`, `acunetix`, `nessus`, `openvas`.
   - **Consultas de inyección y traversal (`403`)**: `UNION SELECT`, `<script`, `javascript:`, `/etc/passwd`, `information_schema`, `../`, `cmd.exe`, `/bin/sh` en la cadena de consulta, además de validación de forma que rechaza bytes nulos y traversal `%2e%2e`.
   - **Clientes automatizados (desafío PoW)**: cuando un sitio aún no tiene reglas propias, los clientes de consola (`curl`, `wget`, `python-requests`, `python-httpx`, `go-http-client`, `aiohttp`, `okhttp`, `axios`, `node-fetch`, `scrapy`), navegadores headless (`headlesschrome`, `playwright`, `puppeteer`, `selenium`, `phantomjs`) y cabeceras `User-Agent` vacías pasan a la pantalla Proof-of-Work.
2. **Inteligencia de amenazas IP autoactualizable y detección de VPN/Tor**
   - Descarga [IPsum (level 3+)](https://raw.githubusercontent.com/stamparm/ipsum/master/levels/3.txt), [Spamhaus DROP IPv4](https://www.spamhaus.org/drop/drop_v4.json) / [IPv6](https://www.spamhaus.org/drop/dropv6.txt) y [abuse.ch Feodo Tracker](https://feodotracker.abuse.ch/downloads/ipblocklist.txt) al arrancar y cada 6 horas, cacheado en `<cert_dir>/intel-cache` con `ETag` e `If-Modified-Since`.
   - Consulta en vivo y sin clave API contra **Shodan InternetDB** (`internetdb.shodan.io`) y **StopForumSpam** (`api.stopforumspam.org`) con caché en memoria de 1 hora y timeouts fail-open para que una lentitud externa no bloquee a los visitantes.
   - **Detección gratuita de VPN/proxy/Tor (`vpn_detection on`)**: carga las listas autoactualizables de VPN IPv4/IPv6 de X4BNet y la lista de nodos de salida de Tor junto con consultas en vivo sin clave (`api.ipquery.io` y `api.ipapi.is`, caché de 1 hora, timeout fail-open de 1.2s). Las IPs coincidentes quedan marcadas como **sospechosas** en los niveles `1` y `3` para HTTP, SSH y Minecraft.
3. **Listas oficiales de IPs de rastreadores de IA**
   - Mantiene los rangos CIDR publicados por **OpenAI** (`gptbot`, `searchbot`, `chatgpt-user`), **Anthropic** (`claude.com/crawling/bots.json`), **Perplexity** (`perplexitybot`, `perplexity-user`), **Google** (`user-triggered-agents`, `user-triggered-fetchers-google`) y **Apple** (`applebot`), detectando rastreadores por su IP de origen incluso si falsifican un `User-Agent` de Chrome.

## Protección de SSH y Minecraft

Cuando una conexión SSH o Minecraft sin verificar llega a Gymbag y dispara el nivel de verificación configurado, Gymbag no la reenvía al demonio ni al servidor de juego. En su lugar, emite un enlace HTTPS de un solo uso (`/v/<token>`): en el mensaje de desconexión de SSH, o en el chat de un mundo vacío en memoria para Minecraft 1.20.5+ con redirección automática por `Transfer` al completarlo:

- **SSH (`port` `22` por defecto)**: `sudo ./gymbag init -t ssh` elige un puerto local libre en `20000–59999`, guarda una copia de `/etc/ssh/sshd_config` en `/etc/ssh/sshd_config.gymbag.bak`, escribe el nuevo `Port` antes de cualquier bloque `Match`, ejecuta `sshd -t` (revirtiendo automáticamente si falla la validación), recarga `sshd` y actualiza el firewall Linux activo (`ufw`, `firewalld`, `nftables` o `iptables`+`ip6tables`) para que el puerto interno solo sea accesible desde loopback mientras `22` y `8443` permanecen abiertos. (Deja una segunda sesión SSH abierta mientras modificas `sshd_config`, o pasa `--ssh-upstream 127.0.0.1:2222`, `--no-auto-setup` o `--no-firewall` para encargarte manualmente.) Una vez que una IP supera el desafío en el navegador, las conexiones pasan directo por relay TCP puro sin tocar el handshake SSH. En los niveles `1` y `3`, los banners malformados o clientes SSH automatizados (`libssh`, `paramiko`, `zgrab`, `masscan`, `nmap`, `asyncssh`, `ssh2go`, `jsch`, `hydra`, `medusa`, `patator`, `python`, `curl`, `ssh-2.0-go`) reciben desafío; el resto ve el mensaje RFC 4253 `SSH_MSG_DISCONNECT` con la URL de verificación directamente en la terminal.
- **Minecraft Java (`port` `25565` por defecto)**: `./gymbag init -t minecraft --server-properties ./server.properties` respalda `server.properties` en `server.properties.gymbag.bak`, mueve `server-port` a un puerto libre aleatorio en `20000–59999` y fija `server-ip` vacío o comodín a `127.0.0.1`. Los Server List Pings (`nextState == 1` y el `0xFE` heredado) siguen pasando salvo bloqueo explícito por regla, así que los jugadores siguen viendo el MOTD y el conteo de jugadores. En Login o Transfer, si hace falta verificar, los clientes compatibles (`1.20.5`–`26.1`) entran en un pequeño mundo vacío puramente en memoria (no se guarda nada en disco), reciben un enlace de verificación clicable en el chat y son redirigidos automáticamente al servidor real mediante `Transfer` al verificarse (los clientes anteriores recurren al diálogo **"Connection Lost"**).
- **Portal HTTPS de verificación compartido (por defecto `:8443`)**: tanto SSH como Minecraft comparten `verify_port` (pasa `--verify-url https://verify.example.com:8443` para usar un dominio propio, o déjalo vacío para usar la IP detectada del servidor). Al servir por IP con ACME, Gymbag solicita un certificado IP **RFC 8738** (usando el perfil `shortlived` de la CA si está disponible) y recurre a la CA local de Gymbag o a un certificado IP autofirmado si HTTP-01 no es accesible. Si `verify_port` coincide con `https_port` y hay sitios HTTP configurados, el portal comparte el puerto `443`.

```caddyfile
{
    verify_url https://verify.example.com:8443
    verify_port 8443
}

ssh {
    port 22
    upstream 127.0.0.1:41822
    verify 3
    rate_limit 60 60s
}

minecraft {
    port 25565
    upstream 127.0.0.1:48291
    server_properties /srv/mc/server.properties
    verify 3
    allow ip 192.168.1.0/24
}
```

Consulta la [guía de protección de SSH y Minecraft](../docs/es/guide/ssh-minecraft.md) para ver todos los detalles y ejemplos en `gymbag.json`.

## Desarrollo local (`localhost` y `*.internal`)

Gymbag gestiona `/etc/hosts` y los certificados TLS locales de forma automática para cualquier sitio en `localhost`, `*.localhost`, `*.internal` (`app.internal`, `api.internal`), `*.local` o `*.test`:

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

### Entradas automáticas en `/etc/hosts`

Con `auto_hosts on` (valor predeterminado), Gymbag revisa `/etc/hosts` (`C:\Windows\System32\drivers\etc\hosts` en Windows) al iniciar y añade entradas loopback IPv4 e IPv6 para los dominios locales que aún no figuren ahí:

```text
127.0.0.1 app.internal # gymbag-managed:9f3a1c2b
::1 app.internal # gymbag-managed:9f3a1c2b
```

Las entradas previas de `localhost` no se tocan. Al recibir `Ctrl+C` o `SIGTERM`, Gymbag vuelve a leer el archivo (conservando los cambios que hayas hecho entre tanto) y elimina solo sus líneas `# gymbag-managed:<instanceID>`. Escribir en `/etc/hosts` requiere privilegios de root en macOS/Linux; si se ejecuta sin ellos, Gymbag registra una advertencia y continúa. Desactívalo con `auto_hosts off` (`GYMBAG_AUTO_HOSTS=off`) si resuelves `.internal` con `dnsmasq` o a mano.

### CA local

En lugar de desechar certificados autofirmados efímeros en cada reinicio, Gymbag mantiene una CA ECDSA P-256 de dos niveles en `<cert_dir>/ca/` (por defecto `certs/ca/`):

1. **Root CA**: `"Neuro-sama Headquarters Root CA"` (`certs/ca/neuro-sama-root-ca.pem`, 10 años de validez, `MaxPathLen: 1`)
2. **Intermediate CA**: `"Gymbag & Swarm SSL Local CA"` (`certs/ca/gymbag-swarm-local-ca.pem`, 5 años de validez, `MaxPathLen: 0`)
3. **Certificado hoja (Leaf)**: `certs/local-fullchain.pem` + `certs/local-privkey.pem` (90 días de validez, renovado cuando faltan menos de 30 días, con SANs DNS además de `127.0.0.1` y `::1`)

Confía una sola vez en `"Neuro-sama Headquarters Root CA"` y tanto navegadores como `curl` aceptarán todos los sitios locales:

```bash
# macOS
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain certs/ca/neuro-sama-root-ca.pem

# Debian / Ubuntu
sudo cp certs/ca/neuro-sama-root-ca.pem /usr/local/share/ca-certificates/neuro-sama-root-ca.crt && sudo update-ca-certificates

# Fedora / RHEL / Arch Linux
sudo trust anchor --store certs/ca/neuro-sama-root-ca.pem
```

> **Firefox:** Activa `security.enterprise_roots.enabled` en `about:config` en macOS/Windows (y Linux con `p11-kit`), o importa `certs/ca/neuro-sama-root-ca.pem` una vez en *Ajustes -> Privacidad y seguridad -> Certificados -> Ver certificados -> Autoridades*.

## Múltiples sitios y niveles de verificación (`0`–`3`)

Un mismo proceso sirve varios dominios con balanceo round-robin entre upstreams y actualizaciones WebSocket RFC 6455. Cada sitio (y servicio SSH/Minecraft) elige un nivel de verificación (`0`–`3`) o desactiva el PoW con `protect off`:

| Nivel | Nombre | Quién recibe el desafío | Uso habitual |
| :---: | :--- | :--- | :--- |
| `0` | `always` | Cualquier cliente sin cookie válida o IP autorizada | Paneles internos, entornos de staging o mitigación de inundaciones L7 |
| `1` | `only suspicious` | Feeds de amenazas, mala reputación en vivo, IPs VPN/proxy/Tor, escáneres, clientes CLI, navegadores headless, cabeceras ausentes o falsificadas, sondeos de exploits | Aplicaciones públicas y APIs donde los navegadores normales no deben ver desafíos |
| `2` | `only ai` | Rangos IP oficiales de rastreadores de IA o cabeceras/`User-Agent` (`X-Ai-Agent`, `Sec-Agent`) de bots de IA | Documentación, blogs y forjas Git donde humanos y `curl` pasan directo |
| `3` | `suspicious & ai` | Combina `1` y `2` (valor por defecto para `verify` sin argumentos, SSH y Minecraft) | Sitios web públicos en producción |
| n/a | `protect off` | Sin desafíos PoW; proxy directo (las reglas personalizadas explícitas siguen ejecutándose) | Servidores de archivos estáticos, webhooks, mirrors |

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
    verify_timeout 30m
}

static.example.com {
    reverse_proxy http://localhost:9000
    protect off
}
```

Resolver el desafío SHA-256 (`difficulty` `1`–`5`, por defecto `4`) establece una cookie HMAC `HttpOnly`, `SameSite=Lax` (`waf_clearance`, por defecto `24h` / `86400s`) vinculada al host y a la IP del cliente. Ajusta su duración globalmente o por sitio con `verify_timeout` / `clearance_ttl` (segundos o duraciones como `30m`, `12h`, `1h30m`, `7d`, `2w`), `--verify-timeout`, `GYMBAG_VERIFY_TIMEOUT` o `PATCH /api/v1/sites/{domain}`. `GET /.waf/v1/session` devuelve `clearance_ttl`, `expires_at` y `expires_in` para visitantes autorizados.

## Rutas por path, archivos estáticos, redirecciones e importación de configuraciones

Los sitios pueden declarar **rutas por path** ordenadas junto a `reverse_proxy`. La primera ruta que coincide atiende la petición; las rutas sin coincidencia pasan a los upstreams predeterminados del sitio (o devuelven `404` si no hay ninguno). Cada ruta tiene un único manejador: proxy inverso, servidor de archivos estáticos (`root`), respuesta fija `respond` o redirección `redir`.

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
    try_files {path} /index.html
}
```

Los comparadores de ruta admiten rutas exactas (`/exact`), globs (`/api/*`, `*.php`) o expresiones regulares Go RE2 (`~^/v[0-9]+/`). Las redirecciones expanden `{scheme}`, `{host}`, `{uri}`, `{path}` y `{query}`. Las rutas estáticas soportan `index`, `browse` y `fallback` para SPA. `header` fija una cabecera de respuesta o elimina una con prefijo `-`. Las reglas de firewall y la verificación se ejecutan antes que cualquier manejador de ruta.

El array `routes` equivalente en `gymbag.json`:

```json
{
  "routes": [
    {"path": "/static/*", "strip_prefix": "/static", "root": "./dist", "browse": true},
    {"path": "/old/*", "redirect": {"to": "https://{host}/new{uri}", "code": 301}},
    {"path": "/api/*", "upstreams": ["http://localhost:8080"]}
  ]
}
```

Gestiona las rutas con `gymbag config route list|add|update|remove` o `PATCH /api/v1/sites/{domain}`.

**Importar configuraciones existentes:** `gymbag run -c /etc/nginx/nginx.conf` (o `httpd.conf` / `apache2.conf`) ejecuta configuraciones nativas de nginx o Apache directamente, y `gymbag config convert -c <file> -o gymbag.json --format json` convierte nginx, Apache y Caddyfile v2 (incluidos snippets, `import`, `@matchers`, `handle`, `route`, `file_server`, `respond`, `redir` y `header`).

## Proveedores SSL, claves propias y renovación

Los dominios públicos usan `tls auto` por defecto, que obtiene certificados de Let's Encrypt mediante ACME HTTP-01. Cambia de CA con `ssl_provider` (alias: `acme_provider`, `ca`, `cert_issuer`) o `tls <provider>`:

| Palabra clave | Autoridad de Certificación | Requisito EAB |
| :--- | :--- | :--- |
| `letsencrypt` (`le`) | Let's Encrypt (Producción) | Ninguno |
| `letsencrypt_staging` | Let's Encrypt (Staging) | Ninguno |
| `zerossl` | ZeroSSL | Automático vía `email`, o explícito `eab <kid> <hmac_key>` |
| `google` (`gts`) | Google Trust Services | Requiere `eab <kid> <hmac_key>` |
| `ssl.com` (`sslcom`, `ssl.com-rsa`) | SSL.com (ECC o RSA) | Requiere `eab <kid> <hmac_key>` |
| `litessl` (`trustasia`) | LiteSSL by TrustAsia | Ninguno para DV estándar |
| `buypass` | Buypass Go SSL | Ninguno |
| `local` | PKI local de dos niveles | CA local (`Neuro-sama Headquarters Root CA`) |
| `https://...` | Cualquier CA RFC 8555 ACME v2 | `eab <kid> <hmac_key>` opcional (p. ej. `step-ca`, Pebble) |

Para **ZeroSSL**, definir `email` solicita las credenciales EAB automáticamente salvo que pases `eab` de forma explícita. Para **Google Trust Services** y **SSL.com**, pasa `eab YOUR_EAB_KEY_ID YOUR_EAB_HMAC_KEY` (funcionan tanto base64 con relleno como base64url sin relleno, y `eab_hmac_key` se oculta en la salida de la Admin API). Los certificados ACME se cachean por proveedor (`certs/acme-zerossl-fullchain.pem`, `certs/acme-fullchain.pem` para Let's Encrypt), así que cambiar `ssl_provider` pide el certificado a la nueva CA de inmediato.

También puedes usar tus propios archivos PEM, utilizar `acme.sh`, desactivar auto-SSL o servir HTTP directamente:
- `account_key /etc/gymbag/account.pem`: clave de cuenta ACME ECDSA (P-256/P-384) o RSA existente.
- `ssl_key /etc/gymbag/domain.key`: clave ECDSA o RSA existente para firmar los CSR de ACME.
- `tls <cert> <key>`: certificados gestionados externamente a nivel global o por sitio (SNI), con `key_password` (`GYMBAG_KEY_PASSWORD`) si la clave privada está cifrada.
- `acme_sh_path ~/.acme.sh` (`--acme-sh-path` / `GYMBAG_ACME_SH_PATH`): carga certificados directamente desde un directorio raíz de `acme.sh` (preferencia por `<domain>_ecc/`, `<domain>/` o directorios wildcard) o un directorio de un solo dominio, a nivel global o por sitio (`tls acme.sh <path>`), con recarga en caliente automática al renovar.
- `auto_ssl off` / `tls off` (`--no-auto-ssl` / `--http-only`): detiene la emisión automática de certificados ACME, CA local o autofirmados. Combina `auto_ssl off` con `acme_sh_path` o `tls <cert> <key>` para servir únicamente certificados gestionados por el usuario, o usa `tls off` para desactivar por completo el listener HTTPS.
- `direct_http on` (`--direct-http` / `http://example.com` / `GYMBAG_DIRECT_HTTP=on`): sirve sitios protegidos directamente por HTTP plano en `http_port` (WAF completo, cookies PoW, proxy y reescritura de redirecciones `http://`, sin forzar redirección `308` ni emitir HSTS sobre HTTP).

Una tarea en segundo plano revisa los certificados cada `renew_interval` segundos (`43200` / 12h por defecto) y renueva o recarga los que estén dentro de `renew_within_days` (`30` días), sustituyendo el `tls.Certificate` atómicamente sin cortar flujos HTTP ni WebSockets activos.

## Reglas de firewall personalizadas

Las peticiones HTTP protegidas pasan por estas comprobaciones en orden:

1. Validación básica de la petición (verbo HTTP, longitud máxima de URI, rechazo de bytes nulos y `..` / `%2e%2e`)
2. ACME HTTP-01 (`/.well-known/acme-challenge/*`)
3. Reglas personalizadas locales (reglas globales y luego reglas del sitio, de arriba abajo; gana la primera coincidencia)
4. Reglas remotas (`remote_rules`, actualizadas automáticamente por HTTP/HTTPS)
5. Reglas base integradas (salvo `default_rules off`)
6. Limitador de tasa por sitio e IP (`rate_limit` / `rate_window`)
7. Inteligencia de amenazas IP (`ip_security_action verify` o `block`)
8. Comprobación Proof-of-Work (`verify` `0`–`3`)

Como las reglas personalizadas se evalúan antes que las reglas base, los límites de tasa y las listas de amenazas, poner un `allow` al principio de un bloque de sitio autoriza limpiamente un CIDR de oficina, un health check o una cookie de vista previa.

### Reglas abreviadas

Sintaxis: `<allow | verify | block> <field> <pattern...>` (también se aceptan los alias `bypass`/`pass`, `challenge`/`pow`, `deny`/`drop`/`reject`):

| Campo | Valores y coincidencia | Ejemplo |
| :--- | :--- | :--- |
| `ip` | IPv4, IPv6 o CIDRs | `allow ip 127.0.0.1 10.0.0.0/8 2001:db8::/32` |
| `user_agent` | Subcadena insensible a mayúsculas, glob `*` o `~regex` | `block user_agent BadBot ~^(?i)python-urllib/` |
| `path` | Exacto/subcadena, glob `*` o `~regex` | `verify path /api/export* ~^/v[0-9]+/search` |
| `query` | Coincidencia en query raw/decodificada, glob `*` o `~regex` | `block query "union select" ~(?i)<script` |
| `method` | Métodos HTTP (insensible a mayúsculas) | `verify method POST DELETE` |
| `header` | `<Header-Name> [pattern]` (`""` = ausente/vacío, `*` = presente) | `verify header Accept-Language ""` |
| `cookie` | `<cookie_name> [pattern]` (`""` = ausente/vacío, `*` = presente) | `allow cookie vip_session *` |

### Bloques `rule` multicondición

Dentro de `rule "name" { ... }`, varios valores en el mismo campo se combinan con OR y campos distintos se combinan con AND:

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

### Reglas remotas y archivos de reglas locales dinámicos

Apunta `remote_rules` de forma global o por sitio a URLs HTTP/HTTPS que devuelvan reglas en sintaxis Gymbagfile o JSON (`[ ... ]` o `{"rules": [ ... ]}`). Los payloads validados se cachean en `<cert_dir>/rules-cache` con peticiones condicionales `ETag` / `If-Modified-Since`, y las respuestas malformadas mantienen activo el conjunto anterior.

También puedes cargar archivos `.rules` o `.json` y carpetas locales mediante `rules_file`, `rules_dir` o bloques `ruleset` (o `gymbag ruleset add`). Marca una fuente como `dynamic` para vigilar y recargar cambios en caliente cada `rules_watch_interval` (`2s` por defecto):

```caddyfile
{
    remote_rules https://rules.example.com/org-baseline.rules
    remote_rules_refresh 5m
    rules_watch_interval 2s
}

app.example.com {
    reverse_proxy http://localhost:3000
    rules_file ./rules/custom.rules dynamic
    rules_dir ./rules.d dynamic
}
```

`default_rules` permanece en `on` por defecto: al añadir reglas personalizadas a un sitio, tus reglas se ejecutan primero y las reglas base de **bloqueo** siguen aplicándose después para que añadir un `verify path` no deje expuesto `/.env` por descuido. Fija `default_rules off` para desactivar las reglas integradas por completo.

## CLI, Docker Compose, variables de entorno y APIs REST

### Referencia de CLI

Gymbag carga `.env` del directorio de trabajo si existe y luego busca `./gymbag.json`, `./Gymbagfile` y `./Caddyfile` (o `-c` con JSON, Gymbagfile, Caddyfile v2, `nginx.conf` o configuraciones de Apache):

```bash
# Generación de configuración (escribe gymbag.json por defecto, o Gymbagfile con --format gymbagfile)
./gymbag init
./gymbag init -d app.example.com -u http://localhost:3000 -l 3 --rules-dir ./rules.d --rules-dynamic
./gymbag init -t ssh --verify-url https://verify.example.com:8443
./gymbag init -t minecraft --server-properties ./server.properties
./gymbag init -t http,ssh,minecraft -d app.example.com -u :3000 --server-properties ./server.properties
./gymbag init --format gymbagfile -o Gymbagfile -d app.example.com -u :3000

# Gestionar sitios, rutas, ajustes globales y convertir configuraciones
./gymbag config list
./gymbag config add -d api.example.com -u :8080 -l 1
./gymbag config update -d api.example.com -l 3 --verify-timeout 6h
./gymbag config route add -d api.example.com --path /static/* --strip-prefix /static --root ./dist
./gymbag config remove -d api.example.com
./gymbag config set admin 127.0.0.1:2019
./gymbag config convert -c /etc/nginx/nginx.conf -o gymbag.json --format json

# Gestionar archivos de reglas, carpetas y reglas individuales
./gymbag ruleset init ./rules/custom.rules
./gymbag ruleset add --source ./rules --dynamic --name local-dir
./gymbag ruleset update local-dir --static
./gymbag ruleset remove local-dir
./gymbag ruleset validate ./rules
./gymbag ruleset rule add --file ./rules/custom.rules --action block --name bad-bot --ua "*BadBot*"

# Ejecutar, vigilar cambios, validar, formatear o gestionar servicios del SO
./gymbag
./gymbag run --config gymbag.json --watch
./gymbag service install --config gymbag.json
./gymbag service start
./gymbag service render --type systemd
./gymbag reverse-proxy --from app.internal --to http://localhost:5173 --verify 3
./gymbag validate --config gymbag.json
./gymbag fmt --overwrite Gymbagfile
./gymbag version
```

### Docker y Docker Compose

En `ghcr.io/evil-project/gymbag` se publican imágenes Alpine multiarquitectura (`linux/amd64`, `linux/arm64`) ejecutándose como UID `10001` con `cap_net_bind_service=+ep`.

```bash
# Ejecución rápida con docker run montando un Gymbagfile
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

`compose.yaml` empareja Gymbag con `redis:8-alpine` usando el cliente Redis RESP2 en Go puro para compartir los nonces PoW, los límites de tasa de ventana fija y los tickets de verificación y autorizaciones IP de SSH/Minecraft entre reinicios o réplicas (`docker compose up -d --build`). Al proteger SSH o Minecraft desde Docker, publica también `-p 22:22` / `-p 25565:25565` y `-p 8443:8443`. Consulta la [Guía de Docker y Docker Compose](../docs/es/guide/docker.md) para ver permisos en montajes bind (`chown 10001:10001`), `--watch` y configuraciones solo por variables de entorno.

### Variables de entorno (`GYMBAG_*`)

El orden de precedencia es **archivo de configuración -> `.env` / variables de entorno `GYMBAG_*` -> flags CLI**.

| Variable | Descripción |
| :--- | :--- |
| `GYMBAG_CONFIG` | Ruta a `gymbag.json`, `Gymbagfile`, `Caddyfile`, `nginx.conf` o configuración de Apache |
| `GYMBAG_BIND` | IP de escucha (`127.0.0.1` para HTTP solo local, `0.0.0.0` cuando hay dominios públicos, SSH o Minecraft activos) |
| `GYMBAG_HTTP_PORT` / `GYMBAG_HTTPS_PORT` | Puertos de escucha HTTP y HTTPS (por defecto `80` y `443`) |
| `GYMBAG_DOMAIN` / `GYMBAG_DOMAINS` | Dominio(s) de destino separados por comas |
| `GYMBAG_UPSTREAM` | URL upstream por defecto |
| `GYMBAG_UPSTREAMS` | Mapa por host separado por punto y coma: `app.example.com=http://web:3000;api.example.com=http://api:8080` |
| `GYMBAG_PROTECT` / `GYMBAG_PROTECT_DOMAINS` | Activa/desactiva la protección WAF (`on`/`off`), o restringe la protección a los dominios indicados |
| `GYMBAG_VERIFY_LEVEL` | Nivel de verificación `0`–`3` u `off` (alias: `GYMBAG_VERIFY`, `GYMBAG_VERIFY_MODE`, `GYMBAG_MODE`, `GYMBAG_TRIGGER`) |
| `GYMBAG_DEFAULT_RULES` | Reglas base integradas (`on` / `off`, por defecto `on`) |
| `GYMBAG_REMOTE_RULES` / `GYMBAG_REMOTE_RULES_REFRESH` | URLs de reglas remotas e intervalo de refresco (por defecto `300`s / `5m`) |
| `GYMBAG_RULES_WATCH_INTERVAL` | Intervalo de sondeo para archivos/carpetas de reglas locales `dynamic` (por defecto `2`s / `2s`) |
| `GYMBAG_DIFFICULTY` | Dificultad SHA-256 PoW (`1`–`5`, por defecto `4`) |
| `GYMBAG_CHALLENGE_TTL` / `GYMBAG_CLEARANCE_TTL` | Caducidad del desafío (`300`s / `5m`) y duración de la autorización (`86400`s / `24h`; alias: `GYMBAG_VERIFY_TIMEOUT`, `GYMBAG_VERIFIED_TIMEOUT`) |
| `GYMBAG_COOKIE_NAME` / `GYMBAG_COOKIE_SECURE` | Nombre de la cookie de autorización (`waf_clearance`) y flag `Secure` (`true`) |
| `GYMBAG_RATE_LIMIT` / `GYMBAG_RATE_WINDOW` | Límite de peticiones por IP (`240`) y ventana (`60`s) |
| `GYMBAG_TLS_MODE` / `GYMBAG_SSL_PROVIDER` | Modo TLS (`auto`, `acme`, `local`, `selfsigned`, `file`, `off`) y CA (`letsencrypt`, `letsencrypt_staging`, `zerossl`, `google`, `ssl.com`, `litessl`, `buypass`, `local` o URL) |
| `GYMBAG_AUTO_SSL` / `GYMBAG_ACME_SH_PATH` | Activa/desactiva SSL automático (`on`/`off`) y define el directorio de certificados de `acme.sh` (alias `GYMBAG_ACME_SH`) |
| `GYMBAG_DIRECT_HTTP` / `GYMBAG_HTTP_ONLY` | Sirve sitios directamente por HTTP (`on`/`off`, alias `GYMBAG_ALLOW_HTTP`) o desactiva HTTPS por completo (`on`/`off`) |
| `GYMBAG_ACME_EMAIL` / `GYMBAG_ACME_DIRECTORY` | Correo de contacto ACME (activa auto-EAB de ZeroSSL) y URL explícita del directorio ACME |
| `GYMBAG_EAB_KID` / `GYMBAG_EAB_HMAC_KEY` | Key ID de External Account Binding y clave HMAC en base64/base64url |
| `GYMBAG_ACCOUNT_KEY_FILE` | Ruta al archivo PEM de la clave de cuenta ACME ECDSA/RSA |
| `GYMBAG_CERT_FILE` / `GYMBAG_KEY_FILE` / `GYMBAG_KEY_PASSWORD` | Certificado, clave y frase de paso para claves cifradas en modo `file` |
| `GYMBAG_CERT_DIR` | Directorio para certificados, claves de CA, secreto, caché de inteligencia y caché de reglas (por defecto `certs`) |
| `GYMBAG_RENEW_INTERVAL` / `GYMBAG_RENEW_WITHIN_DAYS` | Intervalo de comprobación de renovación (`43200`s) y umbral (`30` días) |
| `GYMBAG_AUTO_HOSTS` / `GYMBAG_HSTS` | Gestión automática de `/etc/hosts` (`on`/`off`) y anulación de `Strict-Transport-Security` (`on`/`off`) |
| `GYMBAG_IP_SECURITY` / `GYMBAG_IP_SECURITY_ACTION` | Listas de amenazas (`on`/`off`) y acción al coincidir (`verify` o `block`) |
| `GYMBAG_IP_REPUTATION` / `GYMBAG_AI_IP_LIST` | Consultas en vivo a Shodan InternetDB + StopForumSpam (`on`/`off`) y listas oficiales de CIDR de rastreadores de IA (`on`/`off`) |
| `GYMBAG_VPN_DETECTION` | Detección gratuita de VPN/proxy/Tor en comprobaciones sospechosas (`on`/`off`, por defecto `on`) |
| `GYMBAG_INTEL_REFRESH` | Intervalo de refresco de feeds de amenazas, VPN e IA (por defecto `21600`s / `6h`) |
| `GYMBAG_VERIFY_URL` / `GYMBAG_VERIFY_PORT` | URL del portal HTTPS de verificación compartido (vacío = IP del servidor + cert IP) y puerto (por defecto `8443`) |
| `GYMBAG_SSH_ENABLED` / `GYMBAG_SSH_PORT` / `GYMBAG_SSH_UPSTREAM` | Activa la protección SSH (puerto por defecto `22`) y fija el destino `sshd` |
| `GYMBAG_SSH_VERIFY_LEVEL` / `GYMBAG_SSH_PROTECT` | Nivel de verificación SSH (`0`–`3` u `off`) e interruptor de protección (`on`/`off`) |
| `GYMBAG_MC_ENABLED` / `GYMBAG_MC_PORT` / `GYMBAG_MC_UPSTREAM` | Activa la protección Minecraft (puerto por defecto `25565`) y fija el servidor destino (admite alias `GYMBAG_MINECRAFT_*`) |
| `GYMBAG_MC_SERVER_JAR` / `GYMBAG_MC_JAVA` | Ruta a `server.jar` de Minecraft (arranca el servidor con Java autoseleccionado) y binario Java opcional |
| `GYMBAG_MC_JVM_ARGS` / `GYMBAG_MC_WORK_DIR` / `GYMBAG_MC_AUTO_RESTART` | Flags JVM separados por espacios, directorio de trabajo de Minecraft y reinicio automático ante caídas (`true`/`false`, por defecto `true`) |
| `GYMBAG_MC_VERIFY_LEVEL` / `GYMBAG_MC_PROTECT` | Nivel de verificación Minecraft (`0`–`3` u `off`) e interruptor de protección (`on`/`off`) |
| `GYMBAG_MASCOT` | Archivo, directorio o URL `http(s)://` de mascota personalizada (anulaciones por estado: `GYMBAG_MASCOT_PENSIVE`, `GYMBAG_MASCOT_HAPPY`, `GYMBAG_MASCOT_REJECT`) |
| `GYMBAG_REDIS_URL` / `GYMBAG_SECRET` / `GYMBAG_ADMIN` | URL de Redis (`redis://` o `rediss://`), secreto HMAC y dirección de escucha de la Admin API (p. ej. `127.0.0.1:2019` u `off`) |

### APIs REST

**Endpoints públicos del WAF y del portal de verificación:**

| Método | Ruta | Descripción |
| :--- | :--- | :--- |
| `GET` | `/.waf/v1/session` | Estado de autorización (`cleared`), `protected`, `verify_level`, `verify_mode`, `clearance_ttl`, `expires_at`, `expires_in` |
| `DELETE` | `/.waf/v1/session` | Limpia la cookie de autorización del visitante (`204 No Content`) |
| `POST` | `/.waf/v1/challenges` | Emite un desafío SHA-256 PoW de un solo uso (`201 Created`) |
| `POST` | `/.waf/v1/verifications` | Verifica la solución PoW y fija la cookie firmada (admite el alias `/.waf/verify`) |
| `GET` | `/v/{token}` | Sirve la página PoW de verificación para SSH/Minecraft (admite el alias `/verify/{token}`) |
| `POST` | `/.waf/gate/verify` | Verifica la solución PoW de SSH/Minecraft y guarda la autorización IP |
| `GET` | `/.waf/assets/{name}` | Sirve los PNGs de desafío incrustados (alias `/.waf/v1/assets/{name}`, con caché `ETag`) |

**Admin API local (`admin 127.0.0.1:2019` o `--admin`):**

Mantén la Admin API enlazada a loopback (`127.0.0.1:2019`) o a una interfaz privada: no exige token de autenticación y modifica en vivo el enrutamiento y el estado del firewall.

| Método | Ruta | Descripción |
| :--- | :--- | :--- |
| `GET` | `/api/v1/health` | Estado, `sites_count`, `ssh_enabled`, `minecraft_enabled`, `tls_source`, `threat_entries`, `ai_entries`, `vpn_entries`, `remote_rules` |
| `GET` / `POST` | `/api/v1/tls` / `/api/v1/tls/renew` | Estado TLS activo y catálogo de CAs; fuerza renovación de certificado e intercambio atómico |
| `GET` / `POST` | `/api/v1/intel` / `/api/v1/intel/refresh` | Conteos por feed de amenazas/IA/VPN y tamaño de caché; fuerza actualización inmediata |
| `GET` / `POST` | `/api/v1/rules/remote` / `/api/v1/rules/remote/refresh` | Conteos de reglas remotas y errores de descarga; fuerza refresco y recarga del WAF |
| `GET` / `PUT` | `/api/v1/config` | Vuelca la configuración con secretos ocultos; valida y reemplaza la configuración activa |
| `GET` / `POST` | `/api/v1/sites` | Lista sitios o añade un sitio en tiempo de ejecución |
| `GET` / `PUT` / `PATCH` / `DELETE` | `/api/v1/sites/{domain}` | Inspecciona, reemplaza, parchea en vivo (`verify_level`, `protect`, `difficulty`, `clearance_ttl`, `upstreams`, `routes`, `headers`, `mascot`, `clear_mascot`, `rules`, `remote_rules`, `rule_sources`) o elimina un sitio |
| `GET` / `POST` / `PUT` / `DELETE` | `/api/v1/sites/{domain}/rules` y `.../{index_or_name}` | Lista, añade, reemplaza o elimina reglas personalizadas de un sitio |

## Benchmarks frente a Caddy y Nginx

Medido en Apple M4 (10 núcleos, macOS, Go `1.27.0`), TLS 1.3 con el mismo certificado ECDSA P-256, 64 workers concurrentes, cuerpo de respuesta de `1 KiB`, `1024` conexiones keep-alive al upstream y `0` errores:

| Servidor y modo | HTTP/1.1 TLS (RPS) | H1 p50 / p99 | HTTP/2 TLS (RPS) | H2 p50 / p99 | Tamaño binario | RSS (bajo carga) |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| **Nginx `1.31.8`** (C, `worker_processes auto`) | **85,203** | 0.45 / 6.20 ms | **88,775** | 0.65 / 1.96 ms | 1.5 MB (+libs) | 5.8 MiB |
| **Gymbag `v0.1.0`** (`verify 3`, WAF completo activo) | **72,517** | 0.74 / 3.14 ms | **66,096** | 0.82 / 3.48 ms | 12.9 MB (estático) | 54.3 MiB |
| **Gymbag `v0.1.0`** (`protect off`, proxy puro) | **72,479** | 0.74 / 3.15 ms | **64,048** | 0.83 / 3.94 ms | 12.9 MB (estático) | 52.1 MiB |
| **Gymbag `v0.1.0`** (`verify 0`, cookie válida) | **65,490** | 0.79 / 3.93 ms | **62,108** | 0.84 / 4.27 ms | 12.9 MB (estático) | ~54 MiB |
| **Caddy `v2.11.9`** (Go, keep-alive ajustado) | **54,741** | 0.90 / 4.90 ms | **39,207** | 1.30 / 6.18 ms | 49.0 MB | 76.8 MiB |

Nginx en C sigue siendo más rápido en caudal bruto, aunque Gymbag mantiene menor latencia p99 (`3.14 ms` frente a `6.20 ms`) en HTTP/1.1 con el WAF ejecutándose en línea, y supera a Caddy (`+32%` en HTTP/1.1, `+63%` a `+69%` en HTTP/2) con alrededor de un 30% menos de memoria y un binario 3.8 veces más pequeño. Sondeos de exploits conocidos como `/.env` se descartan en el borde sobre TLS a **140,680 req/s** (`0.35 ms` p50) sin llegar al upstream.

La metodología, los microbenchmarks `go test -bench` y las configuraciones exactas están en [Benchmarks y rendimiento](../docs/es/guide/benchmarks.md).

## Estructura del proyecto

```text
.
├── cmd/gymbag/          # Punto de entrada CLI
├── docs/                # Documentación VitePress multi-idioma (EN, 简体中文, 日本語, 한국어, Español)
├── internal/
│   ├── cli/             # Subcomandos (init, config, ruleset, service, run, reverse-proxy, validate, fmt, version), .env y --watch
│   ├── config/          # Tipos de config, parsers de Gymbagfile/Caddyfile v2/nginx/Apache, duraciones y fuentes de reglas, cargador GYMBAG_*
│   ├── host/            # Normalización de dominios, corchetes IPv6, coincidencia con comodines
│   ├── intel/           # Conjunto de prefijos O(1), feeds autoactualizables de amenazas/VPN/IA, reputación IP en vivo
│   ├── localpki/        # CA local de dos niveles ("Neuro-sama Headquarters Root CA") y gestor de /etc/hosts
│   ├── proxy/           # Enrutador multisitio y por path sin bloqueos, proxies HTTP con pool y WebSocket RFC 6455
│   ├── server/          # Pasarela HTTPS fusionada, redirector HTTP->HTTPS/ACME, API Admin REST
│   ├── service/         # Generador y gestor de servicios del sistema (systemd, launchd, openrc, supervisor)
│   ├── setup/           # Configuración automática de sshd_config/firewall y puerto aleatorio de server.properties en Minecraft
│   ├── store/           # Almacén en memoria de 64 shards, cliente Redis RESP2 en Go puro (desafíos, límites de tasa, tickets/autorizaciones)
│   ├── tcpguard/        # Proxies de protección TCP para SSH y Minecraft Java, motor de políticas, portal HTTPS de verificación compartido
│   ├── tlsmanager/      # ACME v2 multi-CA (certs IP RFC 8738, EAB y ZeroSSL auto-EAB), PKI local, gestor SNI atómico
│   └── waf/             # Verificador PoW, caché HMAC con shards, DFA Aho-Corasick, reglas remotas/locales, UI incrustada
├── nix/                 # Módulo NixOS (services.gymbag)
├── packaging/           # Arch Linux PKGBUILD y scripts/unidad .deb para Debian/Ubuntu
├── Dockerfile
├── compose.yaml
├── flake.nix
├── Gymbagfile
└── go.mod
```

## Historial de estrellas

<a href="https://www.star-history.com/#Evil-Project/gymbag&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=Evil-Project/gymbag&type=Date" />
 </picture>
</a>
