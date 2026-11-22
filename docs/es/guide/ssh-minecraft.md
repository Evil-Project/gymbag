# Protección SSH y Minecraft

Más allá de HTTP/HTTPS, Gymbag puede colocar OpenSSH y Minecraft Java Edition detrás del mismo desafío proof-of-work SHA-256, limitador de tasa, feeds de amenazas IP, detección de VPN/proxy/Tor y reglas personalizadas.

En lugar de exigir que el cliente resuelva el PoW dentro de paquetes SSH o Minecraft, Gymbag rechaza las conexiones sin verificar usando la propia pantalla de desconexión del protocolo e imprime un enlace HTTPS corto de un solo uso fácil de teclear (`/v/<token>`, 8 caracteres en minúsculas sin ambigüedades, insensible a mayúsculas/minúsculas). Cuando el usuario abre ese enlace en un navegador y resuelve el desafío, Gymbag autoriza su IP durante `clearance_ttl` (por defecto `24h`) y las siguientes conexiones pasan directo mediante un relay TCP puro.

## Protección SSH

### Configuración automática (`sudo gymbag init -t ssh`)

Al ejecutar `sudo ./gymbag init -t ssh`, Gymbag deja configurado el host automáticamente:

1. Elige un puerto loopback libre en `20000–59999`.
2. Guarda una copia de seguridad de `/etc/ssh/sshd_config` en `/etc/ssh/sshd_config.gymbag.bak` (si aún no existe).
3. Comenta las directivas `Port` de nivel superior existentes y escribe el puerto aleatorio dentro de un bloque `# gymbag-managed:start` / `# gymbag-managed:end` antes del primer bloque `Match`.
4. Ejecuta `sshd -t -f /etc/ssh/sshd_config`. Si la validación falla, restaura el archivo original y aborta.
5. Recarga `sshd` (mediante `systemctl`, `service` o `rc-service` en Linux).
6. Actualiza el firewall activo de Linux (`ufw`, `firewalld`, `nftables` o `iptables`+`ip6tables`) para permitir tráfico loopback hacia el puerto interno, descartar el tráfico externo que apunte directamente al puerto interno y mantener abiertos el puerto de escucha SSH (`22`) y el puerto del portal de verificación (`8443`).
7. Escribe `gymbag.json` (o `Gymbagfile` con `--format gymbagfile`).

::: warning Mantén abierta una segunda terminal SSH
Siempre que se modifiquen `sshd_config` y las reglas del firewall en una máquina remota, mantén una sesión de root abierta mientras inicias Gymbag y compruebas que puedes iniciar sesión por el puerto `22`.
:::

Para omitir cambios en `sshd_config` o el firewall —por ejemplo cuando `sshd` ya escucha en `127.0.0.1:2222`—, pasa `--ssh-upstream 127.0.0.1:2222` o `--no-auto-setup` (o `--no-firewall` para saltarte solo el firewall):

```bash
sudo ./gymbag init -t ssh --ssh-upstream 127.0.0.1:2222
```

### Cómo se gestiona el tráfico SSH

- **Ruta rápida**: cuando `protect off` está activo o la IP del cliente ya tiene autorización válida (y no hay reglas personalizadas), Gymbag reenvía el flujo TCP sin siquiera leer el banner SSH.
- **Detección de tráfico SSH sospechoso** (en niveles `1` y `3`): Gymbag lee el banner de identificación `SSH-2.0-...` del cliente y marca sondeos vacíos o sin prefijo `SSH-`, además de escáneres conocidos y librerías SSH automatizadas (`libssh`, `paramiko`, `zgrab`, `masscan`, `nmap`, `asyncssh`, `ssh2go`, `jsch`, `scanner`, `bot`, `python`, `curl`, `ssh-2.0-go`, `hydra`, `medusa`, `patator`), junto con IPs de feeds de amenazas e IPs VPN/proxy/Tor.
- **Respuesta de desafío**: envía un paquete RFC 4253 `SSH_MSG_DISCONNECT` (código de razón `11`) con el enlace de verificación directamente en la salida del cliente SSH.

## Protección para Minecraft Java

### Arranque gestionado (`--server-jar`) frente a servidor existente (`--server-properties`)

Si quieres que Gymbag supervise el propio proceso de Minecraft, pasa `--server-jar`:

```bash
./gymbag init -t minecraft --server-jar /srv/mc/paper.jar --mc-jvm-args "-Xms1G -Xmx4G"
./gymbag
```

En modo gestionado, Gymbag:

1. **Elige un runtime de Java compatible automáticamente**: inspecciona `version.json`, los JARs anidados del bundler, `patch.properties` o las cabeceras de bytecode `.class` para distinguir servidores Java 8 heredados de servidores Java 17/21+ modernos, y selecciona un binario `java` compatible en el host (puedes anularlo con `--mc-java` / `java_path`).
2. **Configura `server.properties` de forma segura**: elige un puerto loopback libre en `20000–59999`, crea `server.properties` con `server-ip=127.0.0.1` si aún no existe (o respalda y actualiza el archivo existente) y fija `upstream` automáticamente.
3. **Gestiona el proceso hijo**: arranca el JAR con tus `jvm_args` y `server_args`, lo reinicia ante caídas por defecto (`auto_restart`) y envía un `stop` limpio por consola (con timeout de 15s) cuando Gymbag se apaga.

Si ya ejecutas el servidor de Minecraft por separado (systemd, tmux, screen, Pterodactyl), apunta `init` a su `server.properties`:

```bash
./gymbag init -t minecraft --server-properties /srv/mc/server.properties
./gymbag
```

Para un `server.properties` existente, `init`:

1. Elige un puerto libre en `20000–59999` y respalda `server.properties` en `server.properties.gymbag.bak`.
2. Actualiza `server-port=<random>` conservando comentarios y el resto de claves intactas.
3. Reescribe líneas `server-ip` vacías, `0.0.0.0` o `::` a `server-ip=127.0.0.1` para que los jugadores no puedan saltarse Gymbag conectando directo al puerto interno (asegúrate de bloquear también ese puerto en el firewall externo si `server-ip` tenía una dirección no loopback).
4. Escribe el upstream resultante en `gymbag.json` o `Gymbagfile`.

Pasa `--mc-upstream 127.0.0.1:25566` o `--no-auto-setup` si prefieres gestionar `server.properties` por tu cuenta.

### Cómo se gestiona el tráfico de Minecraft

- **Paso directo de Server List Ping**: tanto los handshakes de estado modernos (`nextState == 1`) como los pings heredados de la versión 1.6 (`0xFE`) se reenvían al servidor upstream salvo bloqueo explícito por regla o `ip_security_action block`. El MOTD, el icono y el número de jugadores siguen funcionando con normalidad.
- **Control en Login y Transfer (`nextState == 2` / `3`)**: Gymbag inspecciona el handshake y el nombre de usuario en Login Start. Versiones de protocolo inválidas (`<= 0`), paquetes de login malformados, nombres inválidos (vacíos, de más de 32 caracteres o con caracteres de control), IPs de feeds de amenazas e IPs VPN/proxy/Tor se consideran sospechosos en los niveles `1` y `3`.
- **Respuesta de desafío**: envía un paquete Minecraft Login Disconnect (`0x00`) formateado como JSON Chat Component con una URL de verificación clicable para copiar al portapapeles (`click_event`/`hover_event` en 1.21.5+ y `clickEvent`/`hoverEvent` en 1.21.4 y anteriores) dentro del diálogo **"Connection Lost"** del jugador.

## Ejecutar HTTP, SSH y Minecraft juntos

Un único proceso de Gymbag puede ejecutar los tres listeners a la vez:

```bash
sudo ./gymbag init -t http,ssh,minecraft \
  -d app.example.com -u http://localhost:3000 \
  --server-jar /srv/mc/paper.jar
sudo ./gymbag
```

En formato `Gymbagfile`:

```caddyfile
{
    email ops@example.com
    verify_url https://verify.example.com:8443
    verify_port 8443
}

app.example.com {
    reverse_proxy http://localhost:3000
    verify 3
}

ssh {
    port 22
    upstream 127.0.0.1:41822
    verify 3
    difficulty 4
    rate_limit 60 60s
    allow ip 10.0.0.0/8
}

minecraft {
    port 25565
    upstream 127.0.0.1:48291
    server_jar /srv/mc/paper.jar
    jvm_args -Xms1G -Xmx4G
    server_properties /srv/mc/server.properties
    verify 3
    rate_limit 60 60s
}
```

Y el archivo equivalente `gymbag.json`:

```json
{
  "sites": [
    {
      "domain": "app.example.com",
      "upstreams": ["http://localhost:3000"],
      "waf": {"verify_level": 3}
    }
  ],
  "ssh": {
    "port": 22,
    "upstream": "127.0.0.1:41822",
    "verify_level": 3,
    "rules": [
      {"action": "allow", "ips": ["10.0.0.0/8"]}
    ]
  },
  "minecraft": {
    "port": 25565,
    "upstream": "127.0.0.1:48291",
    "server_jar": "/srv/mc/paper.jar",
    "jvm_args": ["-Xms1G", "-Xmx4G"],
    "server_properties": "/srv/mc/server.properties",
    "verify_level": 3
  },
  "verify": {
    "url": "https://verify.example.com:8443",
    "port": 8443
  },
  "tls": {
    "acme_email": "ops@example.com"
  }
}
```

## Portal HTTPS compartido y certificados SSL por IP

Por defecto, SSH y Minecraft comparten un listener HTTPS en el puerto `8443` (`verify_port` / `--verify-port` / `GYMBAG_VERIFY_PORT`).

- **Compartir el puerto `443`**: si ya tienes sitios HTTP activos y configuras `verify_port 443` (igual que `https_port`), Gymbag sirve `/v/*`, `/verify/*` y `/.waf/gate/verify` en el listener HTTPS principal sin abrir un segundo puerto.
- **Dominio propio frente a IP automática del servidor**: define `verify_url https://verify.example.com:8443` para usar un nombre DNS. Si `verify_url` queda vacío, Gymbag toma la IP pública del servidor de las interfaces locales (con respaldo en `https://api.ipify.org` y un timeout de 1.5s).
- **Certificados SSL por IP (RFC 8738)**: al servir el portal por IP con ACME, Gymbag solicita un certificado IP ACME HTTP-01 RFC 8738 (eligiendo automáticamente el perfil `shortlived` de la CA si lo ofrece) y lo cachea en `certs/verify-acme-fullchain.pem` (`verify-acme-<provider>-fullchain.pem` para otras CAs). Si el puerto `80` no está accesible o la CA no emite por IP, recurre a la CA local de Gymbag (`certs/verify-local-fullchain.pem`) o a un certificado IP autofirmado (`certs/verify-selfsigned.pem`), o puedes entregar tus propios archivos con `verify_server { tls <cert> <key> }`.

## Valores predeterminados, límites de tasa y Redis

- El nivel `verify` por defecto para bloques SSH y Minecraft es `3` (`suspicious & ai`).
- El `rate_limit` por defecto para SSH y Minecraft es `60` intentos cada `60` segundos (los sitios usan `240` cada `60`s).
- Las reglas personalizadas globales definidas dentro de `{ ... }` también se ejecutan antes que las reglas de cada servicio en SSH y Minecraft (comparando IP/CIDR y el banner del cliente o nombre de usuario contra `user_agent`).
- Cuando `redis` / `GYMBAG_REDIS_URL` está configurado, los tickets de verificación (`gymbag:ticket:<token>`) y las IPs autorizadas (`gymbag:clearance:<service>:<ip>`) se almacenan en Redis para que las autorizaciones sobrevivan a reinicios y funcionen entre instancias.
