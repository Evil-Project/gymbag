---
layout: home

hero:
  name: Gymbag
  text: Proxy Inverso HTTPS y WAF Proof-of-Work
  tagline: Gateway en un único binario Go puro sin dependencias externas — ACME multi-CA, PKI local de dos niveles, reglas WAF remotas e integradas, inteligencia de amenazas autoactualizable y verificación SHA-256 estilo Anubis.
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
    title: Motor Go Puro de Alto Rendimiento
    details: Pipeline WAF de asignación cero con coincidencia de patrones Aho-Corasick DFA, limitador fragmentado y buffers reutilizables — logrando ~72.5k RPS (+32–69% más rápido que Caddy) y más de 140k RPS bloqueando ataques en el borde.
  - icon: 🛡️
    title: WAF Listo para Usar, PoW (0–3) y Timeouts
    details: Reglas integradas bloquean /.env, /wp-admin, escáneres y sondeos SQLi/XSS, mientras PoW SHA-256 desafía rastreadores de IA con duraciones de verificación configurables (30m, 12h, 7d).
  - icon: 🌐
    title: Reglas Remotas e Inteligencia de Amenazas
    details: Sincroniza reglas de firewall remotas (sintaxis Gymbagfile o JSON) por HTTP/HTTPS con caché ETag, más listas autoactualizables de IPsum, Spamhaus DROP, Feodo y rangos CIDR oficiales de IA.
  - icon: 🔒
    title: ACME Multi-CA + PKI Local
    details: ACME HTTP-01 automático (RFC 8555) con Let's Encrypt, ZeroSSL (auto-EAB), Google, SSL.com, LiteSSL y Buypass — además de CA local *.internal y gestión automática de /etc/hosts.
  - icon: 🐳
    title: Docker y Servicios Nativos del Sistema
    details: Imágenes multiarquitectura en GHCR (linux/amd64 y arm64) con Docker Compose + Redis, o soporte nativo para systemd, launchd, OpenRC, Supervisor, Arch y NixOS.
  - icon: 🔌
    title: API REST de Administración y Recarga en Caliente
    details: Recarga automáticamente Gymbagfile con --watch o ajusta en vivo niveles de verificación, timeouts, reglas locales y URLs remotas por HTTP sin cortar conexiones activas.
---

<QuickStartBuilder />

## Niveles de Verificación Interactivos

Selecciona un nivel abajo para ver cómo Gymbag gestiona navegadores, rastreadores de IA, clientes automatizados, IPs maliciosas y sondeos de vulnerabilidades:

<VerifyLevelMatrix />
