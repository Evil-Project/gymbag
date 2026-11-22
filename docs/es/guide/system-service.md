# Configuración de Servicio del Sistema

`gymbag service` genera, instala, inicia, detiene, reinicia, consulta el estado y elimina servicios en segundo plano del sistema operativo en cuatro sistemas init:

| Gestor | SO / Init de destino | Ruta predeterminada de la unidad |
| :--- | :--- | :--- |
| `systemd` | Linux (Ubuntu, Debian, Fedora, Arch, RHEL) | `/etc/systemd/system/<name>.service` (o `~/.config/systemd/user/<name>.service` con `--user`) |
| `launchd` | macOS (LaunchDaemons / LaunchAgents) | `/Library/LaunchDaemons/io.gymbag.<name>.plist` (o `~/Library/LaunchAgents/io.gymbag.<name>.plist` con `--user`) |
| `openrc` | Linux (Alpine, Gentoo, Artix) | `/etc/init.d/<name>` |
| `supervisor` | Unix / Contenedores (`supervisord`) | `/etc/supervisor/conf.d/<name>.conf` (o `~/.config/supervisor/conf.d/<name>.conf` con `--user`) |

## Flujo de servicio en dos comandos

Cuando ya tengas un `gymbag.json` o `Gymbagfile`:

```bash
# 1. Registrar el servicio (detecta automáticamente systemd / launchd / openrc / supervisor)
sudo ./gymbag service install --config /etc/gymbag/gymbag.json

# 2. Iniciarlo
sudo ./gymbag service start
```

O genera la configuración e instala el servicio en un solo paso:

```bash
sudo ./gymbag init -d app.example.com -u :3000 -o /etc/gymbag/gymbag.json --service auto --system
sudo ./gymbag service start --system
```

::: tip `--watch` y `CAP_NET_BIND_SERVICE`
- `gymbag service install` activa `--watch=true` por defecto, así que al editar la configuración se recargan rutas, reglas de firewall y certificados SNI sin reiniciar el demonio.
- Las unidades `systemd` de sistema con un `--service-user` sin root y puertos privilegiados (`< 1024`, incluidos `80`, `443`, `22` o `verify_port < 1024`) reciben `AmbientCapabilities=CAP_NET_BIND_SERVICE`, `CapabilityBoundingSet=CAP_NET_BIND_SERVICE` y `NoNewPrivileges=true`.
- Las unidades OpenRC reciben `capabilities="^cap_net_bind_service"` cuando `--service-user` no es root.
:::

## Subcomandos de `service`

```bash
./gymbag service start
./gymbag service stop
./gymbag service restart
./gymbag service status
./gymbag service uninstall
```

### Ámbito de usuario vs sistema

- Sin `sudo` (`euid != 0`), `gymbag service` usa el ámbito `--user` por defecto (`systemd --user`, `~/Library/LaunchAgents`) en `systemd`, `launchd` y `supervisor`.
- Como `root`, usa `--system` por defecto (`/etc/systemd/system`, `/Library/LaunchDaemons`, `/etc/init.d`).
- Pasa `--user` o `--system` para elegir explícitamente.

## Debian / Ubuntu (`.deb` + `systemd`)

Al instalar el paquete `.deb` de release se colocan `/usr/bin/gymbag` (con `cap_net_bind_service=+ep`), `/etc/gymbag/Gymbagfile` (como conffile) y `/lib/systemd/system/gymbag.service` (`DynamicUser=yes`, `StateDirectory=gymbag`, `GYMBAG_CERT_DIR=/var/lib/gymbag/certs`):

```bash
sudo apt install ./gymbag_<version>_linux_amd64.deb
sudo gymbag init -o /etc/gymbag/Gymbagfile --format gymbagfile
sudo systemctl enable --now gymbag
```

## Arch Linux (`pacman` + `systemd`)

Al instalar el paquete `.pkg.tar.zst` (o compilar desde `packaging/arch/PKGBUILD`) se colocan `/usr/bin/gymbag` (con `cap_net_bind_service=+ep`), `/etc/gymbag/Gymbagfile` y `/usr/lib/systemd/system/gymbag.service`:

```bash
sudo gymbag init -o /etc/gymbag/Gymbagfile --format gymbagfile
sudo systemctl enable --now gymbag
```

## Módulo de NixOS (`services.gymbag`)

En NixOS, importa `nixosModules.default` desde el flake:

```nix
{
  inputs.gymbag.url = "github:Evil-Project/gymbag";

  outputs = { nixpkgs, gymbag, ... }: {
    nixosConfigurations.my-server = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        gymbag.nixosModules.default
        {
          services.gymbag = {
            enable = true;
            openFirewall = true;
            configText = ''
              app.example.com {
                  reverse_proxy http://127.0.0.1:3000
                  verify 3
              }
            '';
          };
        }
      ];
    };
  };
}
```

El módulo ejecuta Gymbag con `DynamicUser = true`, `StateDirectory = "gymbag"` y `CAP_NET_BIND_SERVICE` para que los puertos `80` y `443` funcionen sin root.

## Renderizar archivos de unidad entre plataformas (`render`)

`render` imprime la definición de servicio para cualquier sistema init soportado desde cualquier sistema operativo — útil cuando generas una unidad `systemd` de Linux en macOS antes de desplegar:

```bash
./gymbag service render --type systemd --system --service-user gymbag --config /etc/gymbag/gymbag.json
./gymbag service render --type systemd --system -o ./deploy/gymbag.service
```

## Flags de `gymbag service`

| Flag | Descripción |
| :--- | :--- |
| `-t`, `--type <mgr>` | `auto` (por defecto), `systemd`, `launchd`, `openrc` o `supervisor` |
| `-n`, `--name <name>` | Nombre del servicio (por defecto `gymbag`) |
| `--label <id>` | Etiqueta de `launchd` (por defecto `io.gymbag.<name>`) |
| `-c`, `--config <path>` | Ruta a `gymbag.json`, `Gymbagfile` o `Caddyfile` |
| `--bin <path>` | Ruta al binario `gymbag` (por defecto el ejecutable actual) |
| `--workdir <dir>` | Directorio de trabajo (por defecto el directorio del archivo de configuración) |
| `--env-file <path>` | Archivo `.env` opcional (se detecta automáticamente si existe `<workdir>/.env`) |
| `--log-dir <dir>` | Directorio de logs para `launchd`, `openrc` y `supervisor` |
| `-o`, `--output <path>` | Sobrescribir la ruta destino de unidad/plist/script |
| `--service-user <user>` | Usuario del SO para servicio de sistema |
| `--service-group <grp>` | Grupo del SO para servicio de sistema |
| `--user` / `--system` | Forzar ámbito de usuario o de sistema |
| `--watch` | Pasar `--watch` a `gymbag run` (por defecto `true`) |
| `--enable` | Activar el servicio en el arranque durante `install` (por defecto `true`) |
| `--start`, `--now` | Iniciar el servicio justo después de `install` |
| `--no-reload` | Escribir/eliminar el archivo de unidad sin ejecutar `systemctl`/`launchctl`/`rc-update`/`supervisorctl` |
| `-f`, `--force` | Sobrescribir un archivo de servicio existente |
