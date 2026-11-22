# Inicio Rápido en 2 Comandos

Gymbag se compila en un único binario usando solo la biblioteca estándar de Go. Las plantillas HTML, el script SHA-256 para el navegador y los PNG de la mascota van incrustados directamente mediante `go:embed`.

## Instalación

Cada versión etiquetada en [GitHub Releases](https://github.com/Evil-Project/gymbag/releases) publica binarios estáticos `CGO_ENABLED=0` (`linux`, `darwin`, `windows` para `amd64`/`arm64`), paquetes `.deb` para Debian/Ubuntu, paquetes `.pkg.tar.zst` para Arch Linux, bundles de NixOS e imágenes multiarquitectura en GHCR (`ghcr.io/evil-project/gymbag`):

::: code-group
```bash [Debian / Ubuntu (.deb)]
# Instalar un paquete .deb de release (configura cap_net_bind_service y la unidad systemd)
sudo apt install ./gymbag_<version>_linux_amd64.deb

# Generar configuración y activar el servicio
sudo gymbag init -o /etc/gymbag/Gymbagfile --format gymbagfile
sudo systemctl enable --now gymbag
```

```bash [Docker (GHCR)]
docker run -d --name gymbag \
  -p 80:80 -p 443:443 \
  --cap-add NET_BIND_SERVICE \
  -v "$(pwd)/Gymbagfile:/app/Gymbagfile:ro" \
  -v gymbag-certs:/app/certs \
  ghcr.io/evil-project/gymbag:latest
```

```bash [Arch Linux (pacman)]
# Instalar un paquete .pkg.tar.zst de release
sudo pacman -U gymbag-<version>-1-x86_64.pkg.tar.zst

# O compilar desde packaging/arch
git clone https://github.com/Evil-Project/gymbag.git
cd gymbag/packaging/arch
makepkg -si
```

```bash [Nix / NixOS]
nix run github:Evil-Project/gymbag -- --help
nix profile install github:Evil-Project/gymbag

# Los binarios estáticos de Linux (CGO_ENABLED=0) también corren directo en NixOS
```

```bash [Compilar desde fuentes (Go)]
git clone https://github.com/Evil-Project/gymbag.git
cd gymbag
go build -o gymbag ./cmd/gymbag
```
:::

Consulta la [guía de Docker y Docker Compose](/es/guide/docker) para despliegues con contenedores, pilas Compose + Redis y permisos de volúmenes.

## Arranque en dos comandos

`gymbag init` escribe `gymbag.json` por defecto, o `Gymbagfile` si pasas `--format gymbagfile` (o `-o Gymbagfile`). Toda configuración generada se valida antes de escribirse en disco.

### Sitio web HTTP / HTTPS

```bash
# 1. Generar gymbag.json (añade --format gymbagfile para sintaxis Caddy)
./gymbag init -d app.example.com -u http://localhost:3000

# 2. Iniciar Gymbag (carga automáticamente ./gymbag.json, ./Gymbagfile y luego ./Caddyfile)
./gymbag
```

### Protección SSH

Mueve `sshd` a un puerto libre aleatorio en `20000–59999` dentro de `/etc/ssh/sshd_config` (guardando una copia `.gymbag.bak`), valida con `sshd -t` (revirtiendo si algo falla), recarga `sshd` y actualiza el firewall del host (`ufw`, `firewalld`, `nftables` o `iptables`+`ip6tables`) para que el tráfico externo solo pueda entrar a través de Gymbag en el puerto `22`:

```bash
# Deja una segunda terminal SSH abierta mientras ejecutas esto en una máquina remota
sudo ./gymbag init -t ssh
sudo ./gymbag
```

### Protección de servidor Minecraft

Mueve `server-port` a un puerto libre aleatorio en `20000–59999` dentro de `server.properties` (y enlaza `server-ip` vacío/`0.0.0.0`/`::` a `127.0.0.1`), deja funcionando los Server List Pings para que sigan viéndose el MOTD y el número de jugadores, y muestra el enlace HTTPS de verificación en el diálogo **"Connection Lost"** de Minecraft al intentar entrar:

```bash
./gymbag init -t minecraft --server-properties /path/to/server.properties
./gymbag
```

### Todo junto (`http,ssh,minecraft`) o asistente interactivo

Un mismo proceso puede proteger sitios web, SSH y Minecraft a la vez:

```bash
# Configuración combinada no interactiva:
sudo ./gymbag init -t http,ssh,minecraft -d app.example.com -u :3000 --server-properties /path/to/server.properties
sudo ./gymbag

# O ejecuta el asistente interactivo:
./gymbag init
./gymbag
```

> **Portal de verificación y certificados SSL por IP:** SSH y Minecraft comparten un portal HTTPS de verificación en el puerto `8443` por defecto (`--verify-port`). Pasa `--verify-url https://verify.example.com:8443` para usar un dominio propio, o déjalo vacío para servir la comprobación en la IP detectada del servidor usando un certificado IP ACME RFC 8738 (con respaldo a la CA local de Gymbag o a un certificado IP autofirmado cuando HTTP-01 no es accesible). Consulta la [guía de protección de SSH y Minecraft](/es/guide/ssh-minecraft).

### Desarrollo local (`*.internal`)

```bash
# 1. Generar app.internal -> http://localhost:3000 con PKI local
./gymbag init --preset local

# 2. Ejecutar en puertos sin privilegios (o usar sudo para 80/443 + /etc/hosts)
./gymbag --http-port 8080 --https-port 8443
```

### Registrar como servicio del sistema

```bash
./gymbag init -d app.example.com -u :3000 --service auto
./gymbag service start
```

Consulta [Configuración de servicio del sistema](/es/guide/system-service) para `systemd`, `launchd`, `openrc` y `supervisor`.

## Configuraciones existentes de nginx / Apache / Caddyfile

Gymbag lee archivos nativos `nginx.conf`, Apache `httpd.conf` / `apache2.conf` y Caddyfile v2 directamente:

```bash
./gymbag run -c /etc/nginx/nginx.conf
./gymbag config convert -c Caddyfile -o gymbag.json --format json
```

Consulta [Rutas e importación de configuración](/es/guide/routes-and-imports) para detalles sobre rutas por path, archivos estáticos, redirecciones y conversión.

## Proxy inverso ad-hoc (sin archivo de configuración)

```bash
./gymbag reverse-proxy --from app.internal --to :3000 --verify 3 --verify-timeout 12h
```

## Flags de `gymbag init`

| Flag | Descripción |
| :--- | :--- |
| `-o`, `--output <path>` | Ruta de salida (`gymbag.json` por defecto, o `Gymbagfile` con `--format gymbagfile`) |
| `--format <fmt>` | `json` (por defecto) o `gymbagfile` |
| `-t`, `--template <list>` | Plantilla(s) de servicio: `http`, `ssh`, `minecraft` o `all` (separadas por comas o repetibles) |
| `--preset <name>` | `standard`, `local`, `public`, `strict`, `ai-only`, `suspicious`, `proxy-only`, `http`, `ssh`, `minecraft` o `all` |
| `-d`, `--domain <domains>` | Dominio(s) de destino separados por comas |
| `-u`, `--upstream <url>` | URL upstream (`http://localhost:3000` o `:3000`) |
| `--site <domain=url>` | Añadir múltiples sitios (repetible) |
| `--ssh` / `--ssh-port <port>` | Activar protección SSH (puerto por defecto `22`) |
| `--ssh-upstream <addr>` / `--sshd-config <p>` | Destino explícito de `sshd` o ruta a `sshd_config` (por defecto `/etc/ssh/sshd_config`) |
| `--minecraft`, `--mc` / `--mc-port <port>` | Activar protección Minecraft (puerto por defecto `25565`) |
| `--mc-upstream <addr>` / `--server-properties <p>` | Backend explícito de Minecraft o ruta a `server.properties` |
| `--verify-url <url>` | URL del portal HTTPS de verificación compartido (vacío = IP del servidor + cert SSL IP) |
| `--verify-port <port>` | Puerto del portal HTTPS de verificación compartido (por defecto `8443`) |
| `--no-auto-setup` / `--no-firewall` | Omitir ediciones automáticas de `sshd_config`/firewall o `server.properties` |
| `-l`, `--verify <0-3\|off>` | Nivel de verificación (`0`, `1`, `2`, `3` u `off`) |
| `--difficulty <1-5>` | Dificultad SHA-256 PoW (por defecto `4`) |
| `--verify-timeout <dur>` | Duración de la autorización (`30m`, `12h`, `7d` o segundos; alias `--clearance-ttl`) |
| `--remote-rules <url>` | URL de reglas remotas de firewall (repetible o separada por comas) |
| `--remote-rules-refresh <dur>` | Intervalo de refresco para reglas remotas (por defecto `5m`) |
| `--rules-file <path>` / `--rules-dir <path>` | Archivo o directorio local de reglas (repetible) |
| `--rules-dynamic` / `--rules-watch-interval <dur>` | Vigilar cambios en fuentes de reglas locales (por defecto `2s`) |
| `--no-protect` / `--no-default-rules` | Desactivar verificación PoW o reglas base integradas |
| `--ssl-provider <name>` | `letsencrypt`, `zerossl`, `google`, `ssl.com`, `litessl`, `buypass`, `local` o URL |
| `--email <addr>` | Correo de contacto ACME (también habilita auto-EAB de ZeroSSL) |
| `--eab-kid` / `--eab-hmac-key` | Credenciales ACME External Account Binding |
| `--http-port` / `--https-port` | Puertos HTTP/HTTPS personalizados (por defecto `80` / `443`) |
| `--bind <ip>` | Dirección IP de escucha (se elige automáticamente si se omite) |
| `--admin <addr>` | Habilitar Admin REST API local (p. ej. `127.0.0.1:2019`) |
| `--cert-dir <dir>` | Directorio para certificados, CA local y cachés (por defecto `certs`) |
| `--redis <url>` | URL de Redis para estado compartido de desafíos, límites de tasa y puertas SSH/MC |
| `--ip-security-action <act>` | Acción ante coincidencias de inteligencia de amenazas (`verify` o `block`) |
| `--no-auto-hosts` | Desactivar gestión de `/etc/hosts` para dominios locales |
| `-y`, `--yes` / `-i`, `--interactive` | Omitir preguntas del asistente o forzar modo interactivo |
| `--dry-run` / `-f`, `--force` | Imprimir configuración por stdout sin escribir / sobrescribir archivo existente |
| `--service <type>` | Registrar servicio del SO (`auto`, `systemd`, `launchd`, `openrc`, `supervisor`) |

## Qué funciona sin escribir reglas

Con escribir `verify` a secas (fija el nivel de verificación `3`: `suspicious & ai`) o `block`, se activan tres capas de inmediato:

1. **Reglas base integradas (`default_rules on`)**: devuelven `403` ante `/.env`, `/.git/*`, `/.aws/*`, `/wp-admin/*`, `sqlmap`/`nikto`/`nuclei` y consultas de inyección SQLi/XSS, y envían clientes HTTP de consola o headless a la pantalla PoW.
2. **Feeds de amenazas, reputación IP en vivo y detección de VPN/Tor**: actualiza IPsum (nivel 3+), Spamhaus DROP v4/v6, Feodo Tracker, listas VPN de X4BNet y nodos de salida de Tor al iniciar y cada 6 horas, además de consultar Shodan InternetDB, StopForumSpam, `ipquery.io` e `ipapi.is` con timeouts fail-open. Las IPs VPN/proxy/Tor cuentan como sospechosas en los niveles `1` y `3` para HTTP, SSH y Minecraft.
3. **Listas oficiales de IPs de rastreadores de IA**: detecta los bloques CIDR publicados de OpenAI, Anthropic, Perplexity, Google y Apple aunque un bot falsifique un `User-Agent` de Chrome.
