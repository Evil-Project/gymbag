---
layout: home

hero:
  name: Gymbag
  text: Proxy Inverso HTTPS y WAF Proof-of-Work
  tagline: Un único binario Go que termina HTTPS, emite certificados ACME con varias CA y frena a los scrapers con un desafío proof-of-work SHA-256.
  image:
    src: /mascot-happy.png
    alt: Mascota de Gymbag
  actions:
    - theme: brand
      text: Inicio Rápido en 2 Comandos
      link: /es/guide/getting-started
    - theme: alt
      text: Guía de Docker
      link: /es/guide/docker
    - theme: alt
      text: Benchmarks vs Caddy y Nginx
      link: /es/guide/benchmarks

features:
  - icon: ⚡
    title: Ruta de proxy rápida
    details: La coincidencia Aho-Corasick, los limitadores por shard y los buffers reutilizables dejan el WAF fuera del camino, con unos 72.5k RPS pasando por TLS y más de 140k RPS bloqueando sondeos en el borde.
  - icon: 🛡️
    title: WAF activo por defecto
    details: Trae reglas para /.env, /wp-admin, escáneres y sondeos SQLi/XSS. Cuatro niveles de verificación deciden a quién se le pide el desafío SHA-256, y el permiso puede durar 30m, 12h o 7d.
  - icon: 🌐
    title: Reglas remotas y feeds de amenazas
    details: Descarga reglas de firewall por HTTP con caché ETag, y refresca con temporizador IPsum, Spamhaus DROP, Feodo y las listas CIDR publicadas de rastreadores de IA.
  - icon: 🔒
    title: Certificados sin complicaciones
    details: ACME HTTP-01 contra Let's Encrypt, ZeroSSL, Google, SSL.com, LiteSSL o Buypass. Para nombres *.internal monta su propia CA y escribe las entradas de /etc/hosts por ti.
  - icon: 🐳
    title: Despliega como ya despliegas
    details: Imágenes multiarquitectura en GHCR (amd64 y arm64), un compose con Redis para estado compartido, paquetes para Arch y unidades para systemd, launchd, OpenRC y Supervisor.
  - icon: 🔌
    title: Cambia la configuración en caliente
    details: Edita el Gymbagfile con --watch, o ajusta niveles de verificación, timeouts, reglas locales y URLs remotas desde la Admin API. Las conexiones activas siguen en pie.
---

<QuickStartBuilder />

## Niveles de Verificación Interactivos

Elige un nivel abajo y verás cómo Gymbag trata a navegadores, rastreadores de IA, clientes automatizados, IPs maliciosas y sondeos de vulnerabilidades:

<VerifyLevelMatrix />
