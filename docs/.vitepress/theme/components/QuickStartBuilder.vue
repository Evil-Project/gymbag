<script setup lang="ts">
import { computed, ref } from 'vue'
import { useData } from 'vitepress'

const { lang } = useData()

type LocaleKey = 'en' | 'zh' | 'ja' | 'ko' | 'es'

const activeLocale = computed<LocaleKey>(() => {
  const code = (lang.value || 'en').slice(0, 2).toLowerCase()
  if (code === 'zh' || code === 'ja' || code === 'ko' || code === 'es') {
    return code
  }
  return 'en'
})

const copyByLocale: Record<
  LocaleKey,
  {
    eyebrow: string
    title: string
    copyBtn: string
    copiedBtn: string
    domain: string
    upstream: string
    verifyLevel: string
    verify3: string
    verify2: string
    verify1: string
    verify0: string
    verifyOff: string
    service: string
    serviceNone: string
    serviceAuto: string
    serviceSystemd: string
    serviceLaunchd: string
    serviceOpenrc: string
    serviceSupervisor: string
    sslProvider: string
    acmeEmail: string
    runCommands: string
    step1Gen: string
    step1GenSvc: string
    step2Start: string
    generatedFile: string
  }
> = {
  en: {
    eyebrow: 'Two-Command Quickstart',
    title: 'Configure & Launch Preview',
    copyBtn: 'Copy 2 Commands',
    copiedBtn: 'Copied!',
    domain: 'Domain',
    upstream: 'Upstream URL',
    verifyLevel: 'Verify Level',
    verify3: '3 — suspicious & ai (Recommended)',
    verify2: '2 — only ai',
    verify1: '1 — only suspicious',
    verify0: '0 — always challenge',
    verifyOff: 'off — protect off',
    service: 'System Service',
    serviceNone: 'None (run directly with ./gymbag)',
    serviceAuto: 'auto (detect OS init)',
    serviceSystemd: 'systemd (Linux)',
    serviceLaunchd: 'launchd (macOS)',
    serviceOpenrc: 'OpenRC (Alpine / Gentoo)',
    serviceSupervisor: 'Supervisor',
    sslProvider: 'SSL Provider',
    acmeEmail: 'ACME Email',
    runCommands: 'Run these 2 commands',
    step1Gen: '# 1. Generate config',
    step1GenSvc: '# 1. Generate config & register service',
    step2Start: '# 2. Start service',
    generatedFile: 'Generated Gymbagfile'
  },
  zh: {
    eyebrow: '两条命令快速启动',
    title: '配置生成与启动预览',
    copyBtn: '复制两条命令',
    copiedBtn: '已复制!',
    domain: '域名',
    upstream: '上游地址 (Upstream URL)',
    verifyLevel: '验证级别 (Verify Level)',
    verify3: '3 — 可疑流量与 AI 爬虫（推荐）',
    verify2: '2 — 仅 AI 爬虫',
    verify1: '1 — 仅可疑流量',
    verify0: '0 — 始终验证',
    verifyOff: 'off — 关闭 WAF 防护',
    service: '系统服务',
    serviceNone: '无（直接运行 ./gymbag）',
    serviceAuto: 'auto（自动检测系统 Init）',
    serviceSystemd: 'systemd (Linux)',
    serviceLaunchd: 'launchd (macOS)',
    serviceOpenrc: 'OpenRC (Alpine / Gentoo)',
    serviceSupervisor: 'Supervisor',
    sslProvider: 'SSL 证书提供商',
    acmeEmail: 'ACME 联系邮箱',
    runCommands: '执行这两条命令',
    step1Gen: '# 1. 生成配置文件',
    step1GenSvc: '# 1. 生成配置并注册系统服务',
    step2Start: '# 2. 启动服务',
    generatedFile: '生成的 Gymbagfile'
  },
  ja: {
    eyebrow: '2コマンド クイックスタート',
    title: '設定＆起動プレビュー',
    copyBtn: '2コマンドをコピー',
    copiedBtn: 'コピーしました!',
    domain: 'ドメイン',
    upstream: 'アップストリーム URL',
    verifyLevel: '検証レベル (Verify Level)',
    verify3: '3 — 不審トラフィック & AI（推奨）',
    verify2: '2 — AI クローラーのみ',
    verify1: '1 — 不審トラフィックのみ',
    verify0: '0 — 常にチャレンジ',
    verifyOff: 'off — WAF 保護オフ',
    service: 'システムサービス',
    serviceNone: 'なし（./gymbag を直接起動）',
    serviceAuto: 'auto（OS init 自動検出）',
    serviceSystemd: 'systemd (Linux)',
    serviceLaunchd: 'launchd (macOS)',
    serviceOpenrc: 'OpenRC (Alpine / Gentoo)',
    serviceSupervisor: 'Supervisor',
    sslProvider: 'SSL プロバイダー',
    acmeEmail: 'ACME メールアドレス',
    runCommands: '次の2つのコマンドを実行',
    step1Gen: '# 1. 設定ファイルを生成',
    step1GenSvc: '# 1. 設定生成 & サービス登録',
    step2Start: '# 2. サービスを起動',
    generatedFile: '生成される Gymbagfile'
  },
  ko: {
    eyebrow: '두 줄 명령어 빠른 시작',
    title: '설정 및 실행 미리보기',
    copyBtn: '명령어 2개 복사',
    copiedBtn: '복사됨!',
    domain: '도메인',
    upstream: '업스트림 URL',
    verifyLevel: '검증 레벨 (Verify Level)',
    verify3: '3 — 의심 트래픽 & AI (권장)',
    verify2: '2 — AI 크롤러만',
    verify1: '1 — 의심 트래픽만',
    verify0: '0 — 항상 검증',
    verifyOff: 'off — WAF 보호 끄기',
    service: '시스템 서비스',
    serviceNone: '없음 (./gymbag 직접 실행)',
    serviceAuto: 'auto (OS init 자동 감지)',
    serviceSystemd: 'systemd (Linux)',
    serviceLaunchd: 'launchd (macOS)',
    serviceOpenrc: 'OpenRC (Alpine / Gentoo)',
    serviceSupervisor: 'Supervisor',
    sslProvider: 'SSL 공급자',
    acmeEmail: 'ACME 이메일',
    runCommands: '아래 2개 명령어 실행',
    step1Gen: '# 1. 설정 파일 생성',
    step1GenSvc: '# 1. 설정 생성 및 서비스 등록',
    step2Start: '# 2. 서비스 시작',
    generatedFile: '생성된 Gymbagfile'
  },
  es: {
    eyebrow: 'Inicio Rápido en 2 Comandos',
    title: 'Vista Previa de Configuración',
    copyBtn: 'Copiar 2 Comandos',
    copiedBtn: '¡Copiado!',
    domain: 'Dominio',
    upstream: 'URL Upstream',
    verifyLevel: 'Nivel de Verificación',
    verify3: '3 — sospechoso e IA (Recomendado)',
    verify2: '2 — solo IA',
    verify1: '1 — solo sospechoso',
    verify0: '0 — desafiar siempre',
    verifyOff: 'off — protección desactivada',
    service: 'Servicio del Sistema',
    serviceNone: 'Ninguno (ejecutar ./gymbag directo)',
    serviceAuto: 'auto (detectar init del SO)',
    serviceSystemd: 'systemd (Linux)',
    serviceLaunchd: 'launchd (macOS)',
    serviceOpenrc: 'OpenRC (Alpine / Gentoo)',
    serviceSupervisor: 'Supervisor',
    sslProvider: 'Proveedor SSL',
    acmeEmail: 'Correo ACME',
    runCommands: 'Ejecuta estos 2 comandos',
    step1Gen: '# 1. Generar configuración',
    step1GenSvc: '# 1. Generar config y registrar servicio',
    step2Start: '# 2. Iniciar servicio',
    generatedFile: 'Gymbagfile generado'
  }
}

const t = computed(() => copyByLocale[activeLocale.value])

const domain = ref('app.example.com')
const upstream = ref('http://localhost:3000')
const verify = ref<'0' | '1' | '2' | '3' | 'off'>('3')
const sslProvider = ref('letsencrypt')
const email = ref('ops@example.com')
const serviceMgr = ref<'none' | 'auto' | 'systemd' | 'launchd' | 'openrc' | 'supervisor'>('none')
const copied = ref(false)

const isLocalDomain = computed(() => {
  const d = domain.value.trim().toLowerCase()
  return (
    d === 'localhost' ||
    d.endsWith('.localhost') ||
    d.endsWith('.internal') ||
    d.endsWith('.local') ||
    d.endsWith('.test')
  )
})

const initCommand = computed(() => {
  const parts = ['./gymbag', 'init']
  const d = domain.value.trim() || 'localhost'
  const u = upstream.value.trim() || 'http://localhost:3000'
  parts.push('-d', d, '-u', u)

  if (verify.value === 'off') {
    parts.push('--no-protect')
  } else if (verify.value !== '3') {
    parts.push('-l', verify.value)
  }

  if (!isLocalDomain.value) {
    if (sslProvider.value !== 'letsencrypt') {
      parts.push('--ssl-provider', sslProvider.value)
    }
    if (email.value.trim()) {
      parts.push('--email', email.value.trim())
    }
  }

  if (serviceMgr.value !== 'none') {
    parts.push('--service', serviceMgr.value)
  }

  return parts.join(' ')
})

const startCommand = computed(() => {
  if (serviceMgr.value === 'none') {
    return './gymbag'
  }
  return './gymbag service start'
})

const generatedGymbagfile = computed(() => {
  const lines: string[] = []
  const globals: string[] = []

  if (!isLocalDomain.value) {
    if (email.value.trim()) {
      globals.push(`    email ${email.value.trim()}`)
    }
    if (sslProvider.value !== 'letsencrypt') {
      globals.push(`    ssl_provider ${sslProvider.value}`)
    }
  }

  if (globals.length > 0) {
    lines.push('{')
    lines.push(...globals)
    lines.push('}')
    lines.push('')
  }

  const d = domain.value.trim() || 'localhost'
  const u = upstream.value.trim() || 'http://localhost:3000'
  lines.push(`${d} {`)
  lines.push(`    reverse_proxy ${u}`)
  if (verify.value === 'off') {
    lines.push('    protect off')
  } else if (verify.value !== '0') {
    lines.push(`    verify ${verify.value}`)
  }
  lines.push('}')
  return lines.join('\n')
})

async function copyCommands() {
  const text = `${initCommand.value}\n${startCommand.value}`
  try {
    await navigator.clipboard.writeText(text)
    copied.value = true
    setTimeout(() => {
      copied.value = false
    }, 1800)
  } catch {
    copied.value = false
  }
}
</script>

<template>
  <div class="gb-card qb-wrap">
    <div class="qb-header">
      <div>
        <div class="qb-eyebrow">{{ t.eyebrow }}</div>
        <h3 class="qb-title">{{ t.title }}</h3>
      </div>
      <button class="gb-pill" type="button" @click="copyCommands">
        {{ copied ? t.copiedBtn : t.copyBtn }}
      </button>
    </div>

    <div class="qb-grid">
      <label class="qb-field">
        <span>{{ t.domain }}</span>
        <input v-model="domain" type="text" placeholder="app.example.com" />
      </label>

      <label class="qb-field">
        <span>{{ t.upstream }}</span>
        <input v-model="upstream" type="text" placeholder="http://localhost:3000" />
      </label>

      <label class="qb-field">
        <span>{{ t.verifyLevel }}</span>
        <select v-model="verify">
          <option value="3">{{ t.verify3 }}</option>
          <option value="2">{{ t.verify2 }}</option>
          <option value="1">{{ t.verify1 }}</option>
          <option value="0">{{ t.verify0 }}</option>
          <option value="off">{{ t.verifyOff }}</option>
        </select>
      </label>

      <label class="qb-field">
        <span>{{ t.service }}</span>
        <select v-model="serviceMgr">
          <option value="none">{{ t.serviceNone }}</option>
          <option value="auto">{{ t.serviceAuto }}</option>
          <option value="systemd">{{ t.serviceSystemd }}</option>
          <option value="launchd">{{ t.serviceLaunchd }}</option>
          <option value="openrc">{{ t.serviceOpenrc }}</option>
          <option value="supervisor">{{ t.serviceSupervisor }}</option>
        </select>
      </label>

      <template v-if="!isLocalDomain">
        <label class="qb-field">
          <span>{{ t.sslProvider }}</span>
          <select v-model="sslProvider">
            <option value="letsencrypt">letsencrypt</option>
            <option value="zerossl">zerossl (auto-EAB)</option>
            <option value="google">google</option>
            <option value="ssl.com">ssl.com</option>
            <option value="litessl">litessl</option>
            <option value="buypass">buypass</option>
            <option value="local">local</option>
          </select>
        </label>

        <label class="qb-field">
          <span>{{ t.acmeEmail }}</span>
          <input v-model="email" type="email" placeholder="ops@example.com" />
        </label>
      </template>
    </div>

    <div class="qb-outputs">
      <div class="qb-pane">
        <div class="qb-pane-label">{{ t.runCommands }}</div>
        <pre class="qb-pre"><code><span class="qb-step">{{ serviceMgr !== 'none' ? t.step1GenSvc : t.step1Gen }}</span>
{{ initCommand }}

<span class="qb-step">{{ t.step2Start }}</span>
{{ startCommand }}</code></pre>
      </div>

      <div class="qb-pane">
        <div class="qb-pane-label">{{ t.generatedFile }}</div>
        <pre class="qb-pre"><code>{{ generatedGymbagfile }}</code></pre>
      </div>
    </div>
  </div>
</template>

<style scoped>
.qb-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 1rem;
  margin-bottom: 1rem;
}

.qb-eyebrow {
  font-size: 0.75rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--vp-c-brand-1);
}

.qb-title {
  margin: 0.15rem 0 0;
  font-size: 1.15rem;
}

.qb-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(210px, 1fr));
  gap: 0.85rem;
  margin-bottom: 1rem;
}

.qb-field {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  font-size: 0.82rem;
  font-weight: 600;
  color: var(--vp-c-text-2);
}

.qb-field input,
.qb-field select {
  width: 100%;
  padding: 0.52rem 0.7rem;
  border-radius: 10px;
  border: 1px solid var(--vp-c-border);
  background: var(--vp-c-bg);
  color: var(--vp-c-text-1);
  font-size: 0.88rem;
  font-family: var(--vp-font-family-mono);
}

.qb-field input:focus,
.qb-field select:focus {
  outline: none;
  border-color: var(--vp-c-brand-1);
}

.qb-outputs {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 0.85rem;
}

.qb-pane {
  background: var(--vp-code-block-bg);
  border: 1px solid var(--vp-c-border);
  border-radius: 12px;
  padding: 0.8rem 1rem;
  overflow-x: auto;
}

.qb-pane-label {
  font-size: 0.75rem;
  font-weight: 700;
  color: var(--vp-c-text-3);
  text-transform: uppercase;
  letter-spacing: 0.06em;
  margin-bottom: 0.45rem;
}

.qb-pre {
  margin: 0;
  font-size: 0.84rem;
  line-height: 1.55;
  white-space: pre-wrap;
  word-break: break-word;
}

.qb-step {
  color: var(--vp-c-text-3);
}
</style>
