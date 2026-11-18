import { defineConfig } from 'vitepress'

const base = process.env.VITEPRESS_BASE || '/gymbag/'

export default defineConfig({
  title: 'Gymbag',
  description: 'Single-binary HTTPS Reverse Proxy & Proof-of-Work WAF in pure Go',
  base,
  cleanUrls: true,
  lastUpdated: true,

  markdown: {
    languageAlias: {
      caddyfile: 'nginx',
      gymbagfile: 'nginx'
    }
  },

  head: [
    ['link', { rel: 'icon', type: 'image/png', href: `${base}mascot-happy.png` }],
    ['meta', { name: 'theme-color', content: '#b81d33' }],
    ['meta', { property: 'og:title', content: 'Gymbag — HTTPS Reverse Proxy & Proof-of-Work WAF' }],
    [
      'meta',
      {
        property: 'og:description',
        content:
          'Single-binary pure-Go HTTPS reverse proxy with multi-CA ACME, local PKI, auto-updating threat intelligence, and Anubis-style SHA-256 Proof-of-Work verification.'
      }
    ]
  ],

  themeConfig: {
    logo: '/mascot-happy.png',
    siteTitle: 'Gymbag',

    nav: [
      { text: 'Quick Start', link: '/guide/getting-started' },
      { text: 'Protection', link: '/guide/verify-levels' },
      { text: 'TLS & Local PKI', link: '/guide/ssl-acme' },
      { text: 'System Service', link: '/guide/system-service' },
      {
        text: 'Reference',
        items: [
          { text: 'Gymbagfile & JSON', link: '/reference/gymbagfile' },
          { text: 'CLI & Environment', link: '/reference/cli-env' },
          { text: 'REST APIs & Redis', link: '/reference/api' }
        ]
      }
    ],

    sidebar: [
      {
        text: 'Getting Started',
        collapsed: false,
        items: [
          { text: 'Two-Command Quick Start', link: '/guide/getting-started' },
          { text: 'System Service Setup', link: '/guide/system-service' },
          { text: 'Local Dev & PKI (.internal)', link: '/guide/local-dev' }
        ]
      },
      {
        text: 'WAF & Traffic Defense',
        collapsed: false,
        items: [
          { text: 'Verify Levels (0–3)', link: '/guide/verify-levels' },
          { text: 'Firewall Rules & Threat Intel', link: '/guide/firewall-rules' }
        ]
      },
      {
        text: 'Certificates & TLS',
        collapsed: false,
        items: [
          { text: 'Multi-CA ACME & Custom Certs', link: '/guide/ssl-acme' }
        ]
      },
      {
        text: 'Reference',
        collapsed: false,
        items: [
          { text: 'Gymbagfile & JSON Schema', link: '/reference/gymbagfile' },
          { text: 'CLI & GYMBAG_* Env Vars', link: '/reference/cli-env' },
          { text: 'REST APIs & Docker / Redis', link: '/reference/api' }
        ]
      }
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/Evil-Project/gymbag' }
    ],

    search: {
      provider: 'local'
    },

    editLink: {
      pattern: 'https://github.com/Evil-Project/gymbag/edit/main/docs/:path',
      text: 'Edit this page on GitHub'
    },

    footer: {
      message: 'Pure Go stdlib · Zero external Go dependencies',
      copyright: 'Gymbag — HTTPS Reverse Proxy & Proof-of-Work WAF'
    }
  }
})
