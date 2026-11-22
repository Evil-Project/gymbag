---
layout: home

hero:
  name: Gymbag
  text: Gateway PoW para HTTP/HTTPS, SSH y Minecraft
  tagline: Un único binario en Go que termina HTTPS, protege SSH y Minecraft con proof-of-work en el navegador y bloquea scrapers e IPs maliciosas.
  image:
    src: /mascot-happy.png
    alt: Mascota de Gymbag
  actions:
    - theme: brand
      text: Inicio Rápido en 2 Comandos
      link: /es/guide/getting-started
    - theme: alt
      text: Guía de SSH y Minecraft
      link: /es/guide/ssh-minecraft
    - theme: alt
      text: Benchmarks vs Caddy y Nginx
      link: /es/guide/benchmarks

features:
  - icon: ⚡
    title: Ruta de proxy rápida
    details: "La coincidencia Aho-Corasick, los limitadores por shard y los buffers reutilizables dejan el WAF fuera del camino: ~72.5k RPS a través de TLS y más de 140k RPS descartando sondeos en el borde."
  - icon: 🛡️
    title: HTTP, SSH y Minecraft
    details: "Protege sitios web, WebSockets, OpenSSH y servidores Minecraft Java desde un mismo proceso. Los desafíos de SSH y Minecraft imprimen un enlace HTTPS directamente en la pantalla de desconexión del cliente."
  - icon: 🌐
    title: Inteligencia de amenazas, VPN y listas de IA
    details: "Actualiza automáticamente IPsum, Spamhaus DROP, Feodo, listas VPN/Tor de X4BNet y CIDRs oficiales de rastreadores de IA, además de consultas fail-open en vivo con Shodan InternetDB, StopForumSpam, ipquery.io e ipapi.is."
  - icon: 🔒
    title: ACME, certs IP y PKI local
    details: "Ejecuta HTTP-01 para Let's Encrypt, ZeroSSL, Google, SSL.com, LiteSSL y Buypass, soporta certificados IP ACME RFC 8738 para portales de verificación y firma nombres *.internal con su propia CA."
  - icon: 🐳
    title: Paquetes y contenedores listos
    details: "Publica binarios estáticos, paquetes .deb para Debian/Ubuntu, .pkg.tar.zst para Arch, un flake de NixOS, imágenes multiarquitectura en GHCR, compose con Redis y unidades para systemd/launchd/OpenRC/Supervisor."
  - icon: 🔌
    title: Cambia la configuración en marcha
    details: "Genera gymbag.json o Gymbagfile, lee configuraciones de nginx/Apache/Caddyfile v2, vigila ediciones en directorios de reglas y expone una Admin REST API en vivo sin cortar conexiones."
---

<QuickStartBuilder />

## Niveles de Verificación Interactivos

Elige abajo un nivel de verificación para ver cómo trata Gymbag a navegadores, rastreadores de IA, IPs VPN/Tor, clientes automatizados, feeds de amenazas y sondeos de exploits:

<VerifyLevelMatrix />
