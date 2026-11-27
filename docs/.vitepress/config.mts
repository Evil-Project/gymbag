import { defineConfig, type DefaultTheme } from 'vitepress'

function normalizeBase(raw?: string): string {
  const trimmed = (raw ?? '/').trim().replace(/^\/+|\/+$/g, '')
  return trimmed ? `/${trimmed}/` : '/'
}

const base = normalizeBase(process.env.VITEPRESS_BASE)

const enTheme: DefaultTheme.Config = {
  nav: [
    { text: 'Quick Start', link: '/guide/getting-started' },
    { text: 'SSH & Minecraft', link: '/guide/ssh-minecraft' },
    { text: 'Docker', link: '/guide/docker' },
    { text: 'Protection', link: '/guide/verify-levels' },
    { text: 'TLS & Local PKI', link: '/guide/ssl-acme' },
    { text: 'Benchmarks', link: '/guide/benchmarks' },
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
        { text: 'Docker & Docker Compose', link: '/guide/docker' },
        { text: 'System Service Setup', link: '/guide/system-service' },
        { text: 'Local Dev & PKI (.internal)', link: '/guide/local-dev' },
        { text: 'Routes & Config Import', link: '/guide/routes-and-imports' }
      ]
    },
    {
      text: 'WAF & Traffic Defense',
      collapsed: false,
      items: [
        { text: 'Verify Levels & Timeouts', link: '/guide/verify-levels' },
        { text: 'SSH & Minecraft Protection', link: '/guide/ssh-minecraft' },
        { text: 'Firewall & Remote Rules', link: '/guide/firewall-rules' }
      ]
    },
    {
      text: 'Certificates & TLS',
      collapsed: false,
      items: [{ text: 'Multi-CA ACME & Custom Certs', link: '/guide/ssl-acme' }]
    },
    {
      text: 'Performance',
      collapsed: false,
      items: [{ text: 'Benchmarks vs Caddy & Nginx', link: '/guide/benchmarks' }]
    },
    {
      text: 'Reference',
      collapsed: false,
      items: [
        { text: 'Gymbagfile & JSON Schema', link: '/reference/gymbagfile' },
        { text: 'CLI & GYMBAG_* Env Vars', link: '/reference/cli-env' },
        { text: 'REST APIs & Redis', link: '/reference/api' }
      ]
    }
  ],
  editLink: {
    pattern: 'https://github.com/Evil-Project/gymbag/edit/main/docs/:path',
    text: 'Edit this page on GitHub'
  },
  footer: {
    message: 'Pure Go stdlib · Zero external Go dependencies',
    copyright: 'Gymbag — HTTP/HTTPS, SSH & Minecraft PoW Protection Gateway'
  }
}

const zhTheme: DefaultTheme.Config = {
  nav: [
    { text: '快速开始', link: '/zh/guide/getting-started' },
    { text: 'SSH 与 Minecraft', link: '/zh/guide/ssh-minecraft' },
    { text: 'Docker', link: '/zh/guide/docker' },
    { text: '防护策略', link: '/zh/guide/verify-levels' },
    { text: 'TLS 与本地 PKI', link: '/zh/guide/ssl-acme' },
    { text: '性能基准', link: '/zh/guide/benchmarks' },
    {
      text: '参考文档',
      items: [
        { text: 'Gymbagfile 与 JSON', link: '/zh/reference/gymbagfile' },
        { text: 'CLI 与环境变量', link: '/zh/reference/cli-env' },
        { text: 'REST API 与 Redis', link: '/zh/reference/api' }
      ]
    }
  ],
  sidebar: [
    {
      text: '快速上手',
      collapsed: false,
      items: [
        { text: '两条命令快速开始', link: '/zh/guide/getting-started' },
        { text: 'Docker 与 Compose 部署', link: '/zh/guide/docker' },
        { text: '系统服务安装与管理', link: '/zh/guide/system-service' },
        { text: '本地开发与 PKI (.internal)', link: '/zh/guide/local-dev' },
        { text: '路由与配置导入', link: '/zh/guide/routes-and-imports' }
      ]
    },
    {
      text: 'WAF 与流量防御',
      collapsed: false,
      items: [
        { text: '验证级别与超时配置', link: '/zh/guide/verify-levels' },
        { text: 'SSH 与 Minecraft 防护', link: '/zh/guide/ssh-minecraft' },
        { text: '防火墙、远程规则与威胁情报', link: '/zh/guide/firewall-rules' }
      ]
    },
    {
      text: '证书与 TLS',
      collapsed: false,
      items: [{ text: '多 CA ACME 与自定义证书', link: '/zh/guide/ssl-acme' }]
    },
    {
      text: '性能测试',
      collapsed: false,
      items: [{ text: '与 Caddy / Nginx 基准对比', link: '/zh/guide/benchmarks' }]
    },
    {
      text: '参考手册',
      collapsed: false,
      items: [
        { text: 'Gymbagfile 与 JSON 结构', link: '/zh/reference/gymbagfile' },
        { text: 'CLI 与 GYMBAG_* 环境变量', link: '/zh/reference/cli-env' },
        { text: 'REST API 与 Redis', link: '/zh/reference/api' }
      ]
    }
  ],
  outline: { label: '页面目录' },
  docFooter: { prev: '上一页', next: '下一页' },
  lastUpdated: { text: '最后更新于' },
  darkModeSwitchLabel: '外观主题',
  lightModeSwitchTitle: '切换到浅色模式',
  darkModeSwitchTitle: '切换到深色模式',
  sidebarMenuLabel: '菜单',
  returnToTopLabel: '返回顶部',
  langMenuLabel: '切换语言',
  editLink: {
    pattern: 'https://github.com/Evil-Project/gymbag/edit/main/docs/:path',
    text: '在 GitHub 上编辑此页'
  },
  footer: {
    message: '纯 Go 标准库实现 · 零外部 Go 依赖',
    copyright: 'Gymbag — HTTP/HTTPS、SSH 与 Minecraft 工作量证明防护网关'
  }
}

const zhTWTheme: DefaultTheme.Config = {
  nav: [
    { text: '快速開始', link: '/zh-TW/guide/getting-started' },
    { text: 'SSH 與 Minecraft', link: '/zh-TW/guide/ssh-minecraft' },
    { text: 'Docker', link: '/zh-TW/guide/docker' },
    { text: '防護策略', link: '/zh-TW/guide/verify-levels' },
    { text: 'TLS 與本地 PKI', link: '/zh-TW/guide/ssl-acme' },
    { text: '效能基準', link: '/zh-TW/guide/benchmarks' },
    {
      text: '參考文件',
      items: [
        { text: 'Gymbagfile 與 JSON', link: '/zh-TW/reference/gymbagfile' },
        { text: 'CLI 與環境變數', link: '/zh-TW/reference/cli-env' },
        { text: 'REST API 與 Redis', link: '/zh-TW/reference/api' }
      ]
    }
  ],
  sidebar: [
    {
      text: '快速上手',
      collapsed: false,
      items: [
        { text: '兩行指令快速開始', link: '/zh-TW/guide/getting-started' },
        { text: 'Docker 與 Compose 部署', link: '/zh-TW/guide/docker' },
        { text: '系統服務安裝與管理', link: '/zh-TW/guide/system-service' },
        { text: '本地開發與 PKI (.internal)', link: '/zh-TW/guide/local-dev' },
        { text: '路由與設定檔匯入', link: '/zh-TW/guide/routes-and-imports' }
      ]
    },
    {
      text: 'WAF 與流量防禦',
      collapsed: false,
      items: [
        { text: '驗證等級與逾時設定', link: '/zh-TW/guide/verify-levels' },
        { text: 'SSH 與 Minecraft 防護', link: '/zh-TW/guide/ssh-minecraft' },
        { text: '防火牆、遠端規則與威脅情資', link: '/zh-TW/guide/firewall-rules' }
      ]
    },
    {
      text: '憑證與 TLS',
      collapsed: false,
      items: [{ text: '多 CA ACME 與自訂憑證', link: '/zh-TW/guide/ssl-acme' }]
    },
    {
      text: '效能測試',
      collapsed: false,
      items: [{ text: '與 Caddy / Nginx 基準對比', link: '/zh-TW/guide/benchmarks' }]
    },
    {
      text: '參考手冊',
      collapsed: false,
      items: [
        { text: 'Gymbagfile 與 JSON 結構', link: '/zh-TW/reference/gymbagfile' },
        { text: 'CLI 與 GYMBAG_* 環境變數', link: '/zh-TW/reference/cli-env' },
        { text: 'REST API 與 Redis', link: '/zh-TW/reference/api' }
      ]
    }
  ],
  outline: { label: '本頁目錄' },
  docFooter: { prev: '上一頁', next: '下一頁' },
  lastUpdated: { text: '最後更新於' },
  darkModeSwitchLabel: '外觀主題',
  lightModeSwitchTitle: '切換到淺色模式',
  darkModeSwitchTitle: '切換到深色模式',
  sidebarMenuLabel: '選單',
  returnToTopLabel: '回到頂部',
  langMenuLabel: '切換語言',
  editLink: {
    pattern: 'https://github.com/Evil-Project/gymbag/edit/main/docs/:path',
    text: '在 GitHub 上編輯此頁'
  },
  footer: {
    message: '純 Go 標準函式庫實作 · 零外部 Go 依賴',
    copyright: 'Gymbag — HTTP/HTTPS、SSH 與 Minecraft 工作量證明防護閘道'
  }
}

const jaTheme: DefaultTheme.Config = {
  nav: [
    { text: 'クイックスタート', link: '/ja/guide/getting-started' },
    { text: 'SSH & Minecraft', link: '/ja/guide/ssh-minecraft' },
    { text: 'Docker', link: '/ja/guide/docker' },
    { text: 'WAF 保護', link: '/ja/guide/verify-levels' },
    { text: 'TLS & ローカル PKI', link: '/ja/guide/ssl-acme' },
    { text: 'ベンチマーク', link: '/ja/guide/benchmarks' },
    {
      text: 'リファレンス',
      items: [
        { text: 'Gymbagfile & JSON', link: '/ja/reference/gymbagfile' },
        { text: 'CLI & 環境変数', link: '/ja/reference/cli-env' },
        { text: 'REST API & Redis', link: '/ja/reference/api' }
      ]
    }
  ],
  sidebar: [
    {
      text: 'はじめに',
      collapsed: false,
      items: [
        { text: '2コマンド クイックスタート', link: '/ja/guide/getting-started' },
        { text: 'Docker & Docker Compose', link: '/ja/guide/docker' },
        { text: 'システムサービス設定', link: '/ja/guide/system-service' },
        { text: 'ローカル開発 & PKI (.internal)', link: '/ja/guide/local-dev' },
        { text: 'ルーティングと設定インポート', link: '/ja/guide/routes-and-imports' }
      ]
    },
    {
      text: 'WAF & トラフィック防御',
      collapsed: false,
      items: [
        { text: '検証レベルとタイムアウト', link: '/ja/guide/verify-levels' },
        { text: 'SSH & Minecraft 保護', link: '/ja/guide/ssh-minecraft' },
        { text: 'ファイアウォール & リモートルール', link: '/ja/guide/firewall-rules' }
      ]
    },
    {
      text: '証明書 & TLS',
      collapsed: false,
      items: [{ text: 'マルチ CA ACME & カスタム証明書', link: '/ja/guide/ssl-acme' }]
    },
    {
      text: 'パフォーマンス',
      collapsed: false,
      items: [{ text: 'Caddy / Nginx とのベンチマーク', link: '/ja/guide/benchmarks' }]
    },
    {
      text: 'リファレンス',
      collapsed: false,
      items: [
        { text: 'Gymbagfile & JSON スキーマ', link: '/ja/reference/gymbagfile' },
        { text: 'CLI & GYMBAG_* 環境変数', link: '/ja/reference/cli-env' },
        { text: 'REST API & Redis', link: '/ja/reference/api' }
      ]
    }
  ],
  outline: { label: '目次' },
  docFooter: { prev: '前のページ', next: '次のページ' },
  lastUpdated: { text: '最終更新' },
  darkModeSwitchLabel: '外観',
  lightModeSwitchTitle: 'ライトモードに切り替え',
  darkModeSwitchTitle: 'ダークモードに切り替え',
  sidebarMenuLabel: 'メニュー',
  returnToTopLabel: 'トップへ戻る',
  langMenuLabel: '言語を選択',
  editLink: {
    pattern: 'https://github.com/Evil-Project/gymbag/edit/main/docs/:path',
    text: 'GitHub でこのページを編集'
  },
  footer: {
    message: '純粋な Go 標準ライブラリ · 外部 Go 依存ゼロ',
    copyright: 'Gymbag — HTTP/HTTPS・SSH・Minecraft 対応 PoW 防御ゲートウェイ'
  }
}

const koTheme: DefaultTheme.Config = {
  nav: [
    { text: '빠른 시작', link: '/ko/guide/getting-started' },
    { text: 'SSH & Minecraft', link: '/ko/guide/ssh-minecraft' },
    { text: 'Docker', link: '/ko/guide/docker' },
    { text: 'WAF 보호', link: '/ko/guide/verify-levels' },
    { text: 'TLS & 로컬 PKI', link: '/ko/guide/ssl-acme' },
    { text: '벤치마크', link: '/ko/guide/benchmarks' },
    {
      text: '레퍼런스',
      items: [
        { text: 'Gymbagfile & JSON', link: '/ko/reference/gymbagfile' },
        { text: 'CLI & 환경 변수', link: '/ko/reference/cli-env' },
        { text: 'REST API & Redis', link: '/ko/reference/api' }
      ]
    }
  ],
  sidebar: [
    {
      text: '시작하기',
      collapsed: false,
      items: [
        { text: '두 줄 명령어 빠른 시작', link: '/ko/guide/getting-started' },
        { text: 'Docker & Docker Compose', link: '/ko/guide/docker' },
        { text: '시스템 서비스 설정', link: '/ko/guide/system-service' },
        { text: '로컬 개발 & PKI (.internal)', link: '/ko/guide/local-dev' },
        { text: '라우트 및 설정 가져오기', link: '/ko/guide/routes-and-imports' }
      ]
    },
    {
      text: 'WAF & 트래픽 방어',
      collapsed: false,
      items: [
        { text: '검증 레벨 & 사용자 지정 타임아웃', link: '/ko/guide/verify-levels' },
        { text: 'SSH & Minecraft 보호', link: '/ko/guide/ssh-minecraft' },
        { text: '방화벽 & 원격 규칙', link: '/ko/guide/firewall-rules' }
      ]
    },
    {
      text: '인증서 & TLS',
      collapsed: false,
      items: [{ text: '멀티 CA ACME & 사용자 인증서', link: '/ko/guide/ssl-acme' }]
    },
    {
      text: '성능',
      collapsed: false,
      items: [{ text: 'Caddy / Nginx 벤치마크 비교', link: '/ko/guide/benchmarks' }]
    },
    {
      text: '레퍼런스',
      collapsed: false,
      items: [
        { text: 'Gymbagfile & JSON 스키마', link: '/ko/reference/gymbagfile' },
        { text: 'CLI & GYMBAG_* 환경 변수', link: '/ko/reference/cli-env' },
        { text: 'REST API & Redis', link: '/ko/reference/api' }
      ]
    }
  ],
  outline: { label: '목차' },
  docFooter: { prev: '이전 페이지', next: '다음 페이지' },
  lastUpdated: { text: '최종 수정' },
  darkModeSwitchLabel: '테마',
  lightModeSwitchTitle: '라이트 모드로 전환',
  darkModeSwitchTitle: '다크 모드로 전환',
  sidebarMenuLabel: '메뉴',
  returnToTopLabel: '맨 위로',
  langMenuLabel: '언어 선택',
  editLink: {
    pattern: 'https://github.com/Evil-Project/gymbag/edit/main/docs/:path',
    text: 'GitHub에서 이 페이지 편집'
  },
  footer: {
    message: '순수 Go 표준 라이브러리 · 외부 Go 의존성 제로',
    copyright: 'Gymbag — HTTP/HTTPS, SSH & Minecraft PoW 보호 게이트웨이'
  }
}

const esTheme: DefaultTheme.Config = {
  nav: [
    { text: 'Inicio Rápido', link: '/es/guide/getting-started' },
    { text: 'SSH y Minecraft', link: '/es/guide/ssh-minecraft' },
    { text: 'Docker', link: '/es/guide/docker' },
    { text: 'Protección', link: '/es/guide/verify-levels' },
    { text: 'TLS y PKI Local', link: '/es/guide/ssl-acme' },
    { text: 'Benchmarks', link: '/es/guide/benchmarks' },
    {
      text: 'Referencia',
      items: [
        { text: 'Gymbagfile y JSON', link: '/es/reference/gymbagfile' },
        { text: 'CLI y Entorno', link: '/es/reference/cli-env' },
        { text: 'APIs REST y Redis', link: '/es/reference/api' }
      ]
    }
  ],
  sidebar: [
    {
      text: 'Primeros Pasos',
      collapsed: false,
      items: [
        { text: 'Inicio Rápido en 2 Comandos', link: '/es/guide/getting-started' },
        { text: 'Docker y Docker Compose', link: '/es/guide/docker' },
        { text: 'Servicio del Sistema', link: '/es/guide/system-service' },
        { text: 'Desarrollo Local y PKI (.internal)', link: '/es/guide/local-dev' },
        { text: 'Rutas e importación de configuración', link: '/es/guide/routes-and-imports' }
      ]
    },
    {
      text: 'WAF y Defensa de Tráfico',
      collapsed: false,
      items: [
        { text: 'Niveles de Verificación y Timeouts', link: '/es/guide/verify-levels' },
        { text: 'Protección SSH y Minecraft', link: '/es/guide/ssh-minecraft' },
        { text: 'Firewall y Reglas Remotas', link: '/es/guide/firewall-rules' }
      ]
    },
    {
      text: 'Certificados y TLS',
      collapsed: false,
      items: [{ text: 'ACME Multi-CA y Certificados Propios', link: '/es/guide/ssl-acme' }]
    },
    {
      text: 'Rendimiento',
      collapsed: false,
      items: [{ text: 'Benchmarks vs Caddy y Nginx', link: '/es/guide/benchmarks' }]
    },
    {
      text: 'Referencia',
      collapsed: false,
      items: [
        { text: 'Gymbagfile y Esquema JSON', link: '/es/reference/gymbagfile' },
        { text: 'CLI y Variables GYMBAG_*', link: '/es/reference/cli-env' },
        { text: 'APIs REST y Redis', link: '/es/reference/api' }
      ]
    }
  ],
  outline: { label: 'En esta página' },
  docFooter: { prev: 'Página anterior', next: 'Página siguiente' },
  lastUpdated: { text: 'Última actualización' },
  darkModeSwitchLabel: 'Apariencia',
  lightModeSwitchTitle: 'Cambiar a modo claro',
  darkModeSwitchTitle: 'Cambiar a modo oscuro',
  sidebarMenuLabel: 'Menú',
  returnToTopLabel: 'Volver arriba',
  langMenuLabel: 'Cambiar idioma',
  editLink: {
    pattern: 'https://github.com/Evil-Project/gymbag/edit/main/docs/:path',
    text: 'Editar esta página en GitHub'
  },
  footer: {
    message: 'Biblioteca estándar de Go puro · Cero dependencias externas',
    copyright: 'Gymbag — Proxy Inverso HTTPS, SSH y Minecraft con WAF Proof-of-Work'
  }
}

export default defineConfig({
  title: 'Gymbag',
  description: 'Single-binary HTTP/HTTPS, SSH & Minecraft PoW Protection Gateway in pure Go',
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
    ['meta', { property: 'og:title', content: 'Gymbag — HTTP/HTTPS, SSH & Minecraft PoW Gateway' }],
    [
      'meta',
      {
        property: 'og:description',
        content:
          'Single-binary pure-Go HTTPS reverse proxy and SSH/Minecraft TCP guard with multi-CA ACME, local PKI, threat/VPN/AI IP intel, and SHA-256 Proof-of-Work verification.'
      }
    ]
  ],

  locales: {
    root: {
      label: 'English',
      lang: 'en-US',
      themeConfig: enTheme
    },
    zh: {
      label: '简体中文',
      lang: 'zh-CN',
      link: '/zh/',
      title: 'Gymbag',
      description: '纯 Go 单二进制 HTTP/HTTPS、SSH 与 Minecraft 工作量证明防护网关',
      themeConfig: zhTheme
    },
    'zh-TW': {
      label: '繁體中文',
      lang: 'zh-TW',
      link: '/zh-TW/',
      title: 'Gymbag',
      description: '純 Go 單一執行檔 HTTP/HTTPS、SSH 與 Minecraft 工作量證明防護閘道',
      themeConfig: zhTWTheme
    },
    ja: {
      label: '日本語',
      lang: 'ja-JP',
      link: '/ja/',
      title: 'Gymbag',
      description: '純粋な Go 単一バイナリの HTTP/HTTPS・SSH・Minecraft 対応 PoW 防御ゲートウェイ',
      themeConfig: jaTheme
    },
    ko: {
      label: '한국어',
      lang: 'ko-KR',
      link: '/ko/',
      title: 'Gymbag',
      description: '순수 Go 단일 바이너리 HTTP/HTTPS, SSH 및 Minecraft PoW 보호 게이트웨이',
      themeConfig: koTheme
    },
    es: {
      label: 'Español',
      lang: 'es-ES',
      link: '/es/',
      title: 'Gymbag',
      description: 'Gateway único en Go puro con PoW para HTTP/HTTPS, SSH y Minecraft',
      themeConfig: esTheme
    }
  },

  themeConfig: {
    logo: '/mascot-happy.png',
    siteTitle: 'Gymbag',

    socialLinks: [{ icon: 'github', link: 'https://github.com/Evil-Project/gymbag' }],

    search: {
      provider: 'local',
      options: {
        locales: {
          zh: {
            translations: {
              button: { buttonText: '搜索文档', buttonAriaLabel: '搜索文档' },
              modal: {
                noResultsText: '无法找到相关结果',
                resetButtonTitle: '清除查询条件',
                footer: { selectText: '选择', navigateText: '切换', closeText: '关闭' }
              }
            }
          },
          'zh-TW': {
            translations: {
              button: { buttonText: '搜尋文件', buttonAriaLabel: '搜尋文件' },
              modal: {
                noResultsText: '找不到相關結果',
                resetButtonTitle: '清除搜尋條件',
                footer: { selectText: '選擇', navigateText: '切換', closeText: '關閉' }
              }
            }
          },
          ja: {
            translations: {
              button: { buttonText: '検索', buttonAriaLabel: 'ドキュメントを検索' },
              modal: {
                noResultsText: '結果が見つかりませんでした',
                resetButtonTitle: '検索条件をリセット',
                footer: { selectText: '選択', navigateText: '移動', closeText: '閉じる' }
              }
            }
          },
          ko: {
            translations: {
              button: { buttonText: '검색', buttonAriaLabel: '문서 검색' },
              modal: {
                noResultsText: '검색 결과가 없습니다',
                resetButtonTitle: '검색어 초기화',
                footer: { selectText: '선택', navigateText: '이동', closeText: '닫기' }
              }
            }
          },
          es: {
            translations: {
              button: { buttonText: 'Buscar', buttonAriaLabel: 'Buscar en la documentación' },
              modal: {
                noResultsText: 'No se encontraron resultados',
                resetButtonTitle: 'Limpiar búsqueda',
                footer: { selectText: 'Seleccionar', navigateText: 'Navegar', closeText: 'Cerrar' }
              }
            }
          }
        }
      }
    }
  }
})
