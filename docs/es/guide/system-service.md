# Configuración del Servicio del Sistema

Gymbag incluye un gestor de servicios integrado (`gymbag service`) que genera, registra, inicia, detiene, reinicia, inspecciona y desinstala servicios en segundo plano en:

| Gestor | SO / Init | Ruta de Unidad por Defecto |
| :--- | :--- | :--- |
| `systemd` | Linux (Ubuntu, Debian, Fedora, Arch, RHEL) | `/etc/systemd/system/<name>.service` (o `~/.config/systemd/user/<name>.service` con `--user`) |
| `launchd` | macOS (LaunchDaemons / LaunchAgents) | `/Library/LaunchDaemons/io.gymbag.<name>.plist` (o `~/Library/LaunchAgents/io.gymbag.<name>.plist` con `--user`) |
| `openrc` | Linux (Alpine, Gentoo, Artix) | `/etc/init.d/<name>` |
| `supervisor` | Unix / Contenedores (`supervisord`) | `/etc/supervisor/conf.d/<name>.conf` (o `~/.config/supervisor/conf.d/<name>.conf` con `--user`) |

---

## Flujo en Dos Comandos

```bash
# 1. Registrar el servicio (autodetecta systemd / launchd / openrc / supervisor)
sudo ./gymbag service install --config /etc/gymbag/Gymbagfile

# 2. Iniciar el servicio
sudo ./gymbag service start
```

O combinar creación de configuración y registro en el paso 1:

```bash
sudo ./gymbag init -d app.example.com -u :3000 -o /etc/gymbag/Gymbagfile --service auto --system
sudo ./gymbag service start --system
```

::: tip `--watch` Automático y `CAP_NET_BIND_SERVICE`
- Por defecto, `gymbag service install` configura `gymbag run --config <path> --watch`, recargando rutas y reglas automáticamente al editar `Gymbagfile`.
- Con `--service-user` no-root en `systemd` y puertos `80`/`443`, Gymbag añade automáticamente `AmbientCapabilities=CAP_NET_BIND_SERVICE`, `CapabilityBoundingSet=CAP_NET_BIND_SERVICE` y `NoNewPrivileges=true`.
- Las unidades OpenRC añaden `capabilities="^cap_net_bind_service"`.
:::

---

## Subcomandos del Ciclo de Vida

```bash
./gymbag service start
./gymbag service stop
./gymbag service restart
./gymbag service status
./gymbag service uninstall
```

---

## Módulo Declarativo de NixOS (`services.gymbag`)

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

---

## Arch Linux (`pacman` + `systemd`)

```bash
sudo gymbag init -o /etc/gymbag/Gymbagfile
sudo systemctl enable --now gymbag
```

---

## Previsualizar Archivos de Unidad (`render`)

```bash
./gymbag service render --type systemd --system --service-user gymbag --config /etc/gymbag/Gymbagfile
./gymbag service render --type systemd --system -o ./deploy/gymbag.service
```

---

## Referencia de Flags de `gymbag service`

| Flag | Descripción |
| :--- | :--- |
| `-t`, `--type <mgr>` | `auto` (predeterminado), `systemd`, `launchd`, `openrc` o `supervisor` |
| `-n`, `--name <name>` | Nombre del servicio (predeterminado `gymbag`) |
| `--label <id>` | Etiqueta DNS inversa para `launchd` (predeterminado `io.gymbag.<name>`) |
| `-c`, `--config <path>` | Ruta a `Gymbagfile`, `Caddyfile` o JSON |
| `--bin <path>` | Ruta al binario `gymbag` |
| `--workdir <dir>` | Directorio de trabajo del servicio |
| `--env-file <path>` | Archivo `.env` opcional |
| `--log-dir <dir>` | Directorio de logs para `launchd`, `openrc` y `supervisor` |
| `-o`, `--output <path>` | Sobrescribir ruta del archivo generado |
| `--service-user <user>` | Usuario OS para el servicio del sistema |
| `--service-group <grp>` | Grupo OS para el servicio del sistema |
| `--user` / `--system` | Forzar alcance de usuario o sistema |
| `--watch` | Pasar `--watch` a `gymbag run` (predeterminado `true`) |
| `--enable` | Habilitar al arranque durante `install` (predeterminado `true`) |
| `--start`, `--now` | Iniciar inmediatamente tras `install` |
| `--no-reload` | No invocar `systemctl`/`launchctl`/`rc-update`/`supervisorctl` |
| `-f`, `--force` | Sobrescribir archivo de unidad existente |
