# Inicio Rápido en 2 Comandos

Gymbag se compila en un único binario independiente sin dependencias externas de Go — todas las plantillas HTML, el script SHA-256 del navegador y las imágenes PNG de la mascota están incrustados mediante `go:embed`.

## Instalación

### Binarios Estáticos, Docker, Arch Linux y NixOS

Cada etiqueta de lanzamiento en [GitHub Releases](https://github.com/Evil-Project/gymbag/releases) publica binarios estáticos `CGO_ENABLED=0` (`linux`, `darwin`, `windows` para `amd64`/`arm64`), paquetes `.pkg.tar.zst` de Arch Linux (`x86_64` y `aarch64`), e imágenes de contenedor multiarquitectura en GHCR (`ghcr.io/evil-project/gymbag`):

::: code-group
```bash [Docker (GHCR)]
docker run -d --name gymbag \
  -p 80:80 -p 443:443 \
  --cap-add NET_BIND_SERVICE \
  -v "$(pwd)/Gymbagfile:/app/Gymbagfile:ro" \
  -v gymbag-certs:/app/certs \
  ghcr.io/evil-project/gymbag:latest
```

```bash [Arch Linux (pacman)]
# Instalar el paquete binario .pkg.tar.zst
sudo pacman -U gymbag-<version>-1-x86_64.pkg.tar.zst

# O compilar desde el código fuente con PKGBUILD
git clone https://github.com/Evil-Project/gymbag.git
cd gymbag/packaging/arch
makepkg -si
```

```bash [Nix / NixOS]
# Ejecutar o instalar directamente vía Nix Flakes
nix run github:Evil-Project/gymbag -- --help
nix profile install github:Evil-Project/gymbag

# Los binarios estáticos Linux (CGO_ENABLED=0) también funcionan directo en NixOS
```

```bash [Compilar desde Código Fuente (Go)]
git clone https://github.com/Evil-Project/gymbag.git
cd gymbag
go build -o gymbag ./cmd/gymbag
```
:::

Consulta la [Guía de Docker y Docker Compose](/es/guide/docker) para despliegues con contenedores, Redis y permisos de volúmenes.

## Iniciar en Dos Comandos

### Opción A: Por Flags (No interactivo)

```bash
# 1. Generar gymbag.json (añade --format gymbagfile para un Gymbagfile estilo Caddy)
./gymbag init -d app.example.com -u http://localhost:3000

# 2. Iniciar Gymbag (carga automáticamente ./gymbag.json, ./Gymbagfile y luego ./Caddyfile)
./gymbag
```

### Opción B: Asistente Interactivo

Ejecuta `gymbag init` sin flags para configurar de forma guiada dominios, URL upstream, nivel de verificación, proveedor SSL, puertos, API de administración y registro opcional como servicio:

```bash
# 1. Generador interactivo
./gymbag init

# 2. Iniciar Gymbag
./gymbag
```

### Opción C: Preajuste de Desarrollo Local (`*.internal`)

```bash
# 1. Generar app.internal -> http://localhost:3000 con PKI local
./gymbag init --preset local

# 2. Iniciar Gymbag (puertos no privilegiados o sudo para 80/443 + /etc/hosts)
./gymbag --http-port 8080 --https-port 8443
```

### Opción D: Registrar como Servicio del Sistema

```bash
# 1. Generar gymbag.json y registrar systemd / launchd / openrc / supervisor
./gymbag init -d app.example.com -u :3000 --service auto

# 2. Iniciar el servicio registrado
./gymbag service start
```

Consulta [Servicio del Sistema](/es/guide/system-service) para más detalles.

---

## Configuraciones Existentes de nginx / Apache / Caddyfile

Gymbag lee de forma nativa archivos `nginx.conf`, Apache `httpd.conf` / `apache2.conf` y Caddyfile v2, por lo que puedes partir de una configuración existente:

```bash
./gymbag run -c /etc/nginx/nginx.conf
./gymbag config convert -c Caddyfile -o gymbag.json --format json
```

Consulta [Rutas e Importación de Configuración](/es/guide/routes-and-imports) para detalles sobre rutas por path, archivos estáticos, redirecciones y conversión.

---

## Proxy Inverso Ad-Hoc (Sin Archivo de Configuración)

Para pruebas rápidas sin crear ningún archivo en disco:

```bash
./gymbag reverse-proxy --from app.internal --to :3000 --verify 3 --verify-timeout 12h
```

---

## Opciones del Generador (`gymbag init`)

`gymbag init` valida toda configuración generada antes de escribirla en disco.

| Flag | Descripción |
| :--- | :--- |
| `-o`, `--output <path>` | Ruta de salida (`gymbag.json` por defecto, o `Gymbagfile` con `--format gymbagfile`) |
| `--format <fmt>` | `json` (predeterminado) o `gymbagfile` |
| `--preset <name>` | `standard`, `local`, `public`, `strict`, `ai-only`, `suspicious`, o `proxy-only` |
| `-d`, `--domain <domains>` | Dominio(s) públicos, separados por comas |
| `-u`, `--upstream <url>` | URL upstream (p. ej. `http://localhost:3000` o `:3000`) |
| `--site <domain=url>` | Añadir múltiples bloques de sitios (repetible) |
| `-l`, `--verify <0-3\|off>` | Nivel de verificación (`0`, `1`, `2`, `3` u `off`) |
| `--difficulty <1-5>` | Dificultad PoW SHA-256 (ceros hexadecimales iniciales, por defecto `4`) |
| `--verify-timeout <dur>` | Tiempo de validez de verificación (ej. `30m`, `12h`, `7d` o segundos; alias `--clearance-ttl`) |
| `--remote-rules <url>` | URL de reglas remotas de firewall (repetible o separada por comas) |
| `--remote-rules-refresh <dur>` | Intervalo de refresco de reglas remotas (por defecto `5m`) |
| `--rules-file <path>` | Ruta a archivo dedicado de reglas de firewall (repetible) |
| `--rules-dir <path>` | Directorio de archivos de reglas de firewall (repetible) |
| `--rules-dynamic` | Recargar automáticamente `--rules-file` / `--rules-dir` cuando cambian los archivos |
| `--rules-watch-interval <dur>` | Intervalo de sondeo para archivos/carpetas de reglas locales dinámicas (por defecto `2s`) |
| `--no-protect` | Desactivar verificación WAF (`protect off`) |
| `--no-default-rules` | Desactivar reglas base integradas (`default_rules off`) |
| `--ssl-provider <name>` | `letsencrypt`, `zerossl`, `google`, `ssl.com`, `litessl`, `buypass`, `local` o URL personalizada |
| `--email <addr>` | Correo de contacto ACME (habilita auto-EAB en ZeroSSL) |
| `--eab-kid` / `--eab-hmac-key` | Credenciales ACME External Account Binding |
| `--http-port` / `--https-port` | Puertos HTTP/HTTPS personalizados (por defecto `80` / `443`) |
| `--bind <ip>` | Dirección de escucha (autoseleccionada si se omite) |
| `--admin <addr>` | Activar API REST local (ej. `127.0.0.1:2019`) |
| `--cert-dir <dir>` | Directorio para certificados, CA local y cachés (por defecto `certs`) |
| `--redis <url>` | URL opcional de Redis para compartir desafíos y límites de tasa |
| `--ip-security-action <act>` | Acción para IPs maliciosas (`verify` o `block`) |
| `--no-auto-hosts` | Desactivar gestión automática de `/etc/hosts` |
| `-y`, `--yes` | Omitir preguntas interactivas y usar valores predeterminados |
| `-i`, `--interactive` | Forzar el asistente interactivo aunque haya flags |
| `--dry-run` | Imprimir la configuración generada sin escribir archivo |
| `-f`, `--force` | Sobrescribir un archivo de configuración existente |
| `--service <type>` | Registrar servicio del sistema (`auto`, `systemd`, `launchd`, `openrc`, `supervisor`) |

---

## Qué Incluye la Protección Lista para Usar

Escribir `verify` (nivel `3`: `suspicious & ai`) o `block` activa de inmediato tres capas de protección sin escribir expresiones regulares:

1. **Reglas base integradas (`default_rules on`)** — Bloquea `/.env`, `/.git/*`, `/.aws/*`, `/wp-admin/*`, `sqlmap`/`nikto`/`nuclei` y sondeos SQLi/XSS (`403`), y desafía a clientes CLI/headless.
2. **Inteligencia de amenazas IP autoactualizable** — Descarga IPsum (nivel 3+), Spamhaus DROP v4/v6 y abuse.ch Feodo Tracker al inicio y cada 6 horas, más consultas en vivo a Shodan InternetDB y StopForumSpam sin claves API.
3. **Listas oficiales de IPs de rastreadores de IA** — Rastrea rangos CIDR publicados de OpenAI, Anthropic, Perplexity, Google y Apple, detectando bots incluso si falsifican el User-Agent.
