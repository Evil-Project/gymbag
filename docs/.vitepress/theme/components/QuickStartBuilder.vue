<script setup lang="ts">
import { computed, ref } from 'vue'
import { useData } from 'vitepress'

const { lang } = useData()

type LocaleKey = 'en' | 'zh' | 'ja' | 'ko' | 'es'
type ServiceMode = 'http' | 'ssh' | 'minecraft' | 'all'
type ConfigFormat = 'json' | 'gymbagfile'

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
    services: string
    svcHttp: string
    svcSsh: string
    svcMc: string
    svcAll: string
    format: string
    fmtJson: string
    fmtGymbagfile: string
    domain: string
    upstream: string
    sshPort: string
    mcProps: string
    mcPort: string
    verifyUrl: string
    verifyUrlPlaceholder: string
    verifyPort: string
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
    generatedFileJson: string
    generatedFileCaddy: string
  }
> = {
  en: {
    eyebrow: 'Two-Command Quickstart',
    title: 'Command & Config Preview',
    copyBtn: 'Copy 2 Commands',
    copiedBtn: 'Copied!',
    services: 'Services',
    svcHttp: 'HTTP / HTTPS Website',
    svcSsh: 'SSH Server',
    svcMc: 'Minecraft Server',
    svcAll: 'All-in-One (HTTP + SSH + MC)',
    format: 'Config Format',
    fmtJson: 'gymbag.json (default)',
    fmtGymbagfile: 'Gymbagfile (Caddy syntax)',
    domain: 'Domain',
    upstream: 'Upstream URL',
    sshPort: 'SSH Listen Port',
    mcProps: 'server.properties Path',
    mcPort: 'Minecraft Listen Port',
    verifyUrl: 'Verify Portal URL (optional)',
    verifyUrlPlaceholder: 'blank = server IP + IP SSL cert',
    verifyPort: 'Verify Portal Port',
    verifyLevel: 'Verify Level',
    verify3: '3 — suspicious & ai (Default)',
    verify2: '2 — only ai',
    verify1: '1 — only suspicious',
    verify0: '0 — always challenge',
    verifyOff: 'off — protect off',
    service: 'System Service',
    serviceNone: 'None (run directly)',
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
    step2Start: '# 2. Start',
    generatedFileJson: 'Generated gymbag.json',
    generatedFileCaddy: 'Generated Gymbagfile'
  },
  zh: {
    eyebrow: '两条命令快速启动',
    title: '命令与配置预览',
    copyBtn: '复制两条命令',
    copiedBtn: '已复制',
    services: '保护服务',
    svcHttp: 'HTTP / HTTPS 网站',
    svcSsh: 'SSH 服务器',
    svcMc: 'Minecraft 服务器',
    svcAll: '全合一（HTTP + SSH + MC）',
    format: '配置格式',
    fmtJson: 'gymbag.json（默认）',
    fmtGymbagfile: 'Gymbagfile（Caddy 语法）',
    domain: '域名',
    upstream: '上游地址',
    sshPort: 'SSH 监听端口',
    mcProps: 'server.properties 路径',
    mcPort: 'Minecraft 监听端口',
    verifyUrl: '验证页面 URL（可选）',
    verifyUrlPlaceholder: '留空则自动使用服务器 IP + IP 证书',
    verifyPort: '验证页面端口',
    verifyLevel: '验证级别',
    verify3: '3 — 可疑流量与 AI（默认）',
    verify2: '2 — 仅 AI 爬虫',
    verify1: '1 — 仅可疑流量',
    verify0: '0 — 始终验证',
    verifyOff: 'off — 关闭验证',
    service: '系统服务',
    serviceNone: '不注册（直接运行）',
    serviceAuto: 'auto（自动检测）',
    serviceSystemd: 'systemd (Linux)',
    serviceLaunchd: 'launchd (macOS)',
    serviceOpenrc: 'OpenRC (Alpine / Gentoo)',
    serviceSupervisor: 'Supervisor',
    sslProvider: 'SSL 证书提供商',
    acmeEmail: 'ACME 邮箱',
    runCommands: '执行这两条命令',
    step1Gen: '# 1. 生成配置',
    step1GenSvc: '# 1. 生成配置并注册服务',
    step2Start: '# 2. 启动',
    generatedFileJson: '生成的 gymbag.json',
    generatedFileCaddy: '生成的 Gymbagfile'
  },
  ja: {
    eyebrow: '2コマンド クイックスタート',
    title: 'コマンド・設定プレビュー',
    copyBtn: '2コマンドをコピー',
    copiedBtn: 'コピーしました',
    services: '保護対象サービス',
    svcHttp: 'HTTP / HTTPS サイト',
    svcSsh: 'SSH サーバー',
    svcMc: 'Minecraft サーバー',
    svcAll: 'すべて (HTTP + SSH + MC)',
    format: '設定形式',
    fmtJson: 'gymbag.json（既定）',
    fmtGymbagfile: 'Gymbagfile（Caddy 構文）',
    domain: 'ドメイン',
    upstream: 'アップストリーム URL',
    sshPort: 'SSH 待受ポート',
    mcProps: 'server.properties のパス',
    mcPort: 'Minecraft 待受ポート',
    verifyUrl: '検証ポータル URL（任意）',
    verifyUrlPlaceholder: '空欄時はサーバー IP + IP SSL 証明書',
    verifyPort: '検証ポータルポート',
    verifyLevel: '検証レベル',
    verify3: '3 — 不審 & AI（既定）',
    verify2: '2 — AI のみ',
    verify1: '1 — 不審のみ',
    verify0: '0 — 常にチャレンジ',
    verifyOff: 'off — 保護オフ',
    service: 'システムサービス',
    serviceNone: 'なし（直接起動）',
    serviceAuto: 'auto（OS init 自動検出）',
    serviceSystemd: 'systemd (Linux)',
    serviceLaunchd: 'launchd (macOS)',
    serviceOpenrc: 'OpenRC (Alpine / Gentoo)',
    serviceSupervisor: 'Supervisor',
    sslProvider: 'SSL プロバイダー',
    acmeEmail: 'ACME メール',
    runCommands: '次の2コマンドを実行',
    step1Gen: '# 1. 設定を生成',
    step1GenSvc: '# 1. 設定生成 & サービス登録',
    step2Start: '# 2. 起動',
    generatedFileJson: '生成される gymbag.json',
    generatedFileCaddy: '生成される Gymbagfile'
  },
  ko: {
    eyebrow: '두 줄 명령어 빠른 시작',
    title: '명령어 및 설정 미리보기',
    copyBtn: '명령어 2개 복사',
    copiedBtn: '복사됨',
    services: '보호 서비스',
    svcHttp: 'HTTP / HTTPS 웹사이트',
    svcSsh: 'SSH 서버',
    svcMc: 'Minecraft 서버',
    svcAll: '통합 (HTTP + SSH + MC)',
    format: '설정 형식',
    fmtJson: 'gymbag.json (기본값)',
    fmtGymbagfile: 'Gymbagfile (Caddy 문법)',
    domain: '도메인',
    upstream: '업스트림 URL',
    sshPort: 'SSH 수신 포트',
    mcProps: 'server.properties 경로',
    mcPort: 'Minecraft 수신 포트',
    verifyUrl: '검증 포털 URL (선택)',
    verifyUrlPlaceholder: '비워 두면 서버 IP + IP SSL 인증서 사용',
    verifyPort: '검증 포털 포트',
    verifyLevel: '검증 레벨',
    verify3: '3 — 의심 & AI (기본값)',
    verify2: '2 — AI 만',
    verify1: '1 — 의심 트래픽만',
    verify0: '0 — 항상 검증',
    verifyOff: 'off — 보호 끄기',
    service: '시스템 서비스',
    serviceNone: '사용 안 함 (직접 실행)',
    serviceAuto: 'auto (OS init 자동 감지)',
    serviceSystemd: 'systemd (Linux)',
    serviceLaunchd: 'launchd (macOS)',
    serviceOpenrc: 'OpenRC (Alpine / Gentoo)',
    serviceSupervisor: 'Supervisor',
    sslProvider: 'SSL 공급자',
    acmeEmail: 'ACME 이메일',
    runCommands: '아래 2개 명령어 실행',
    step1Gen: '# 1. 설정 생성',
    step1GenSvc: '# 1. 설정 생성 및 서비스 등록',
    step2Start: '# 2. 시작',
    generatedFileJson: '생성된 gymbag.json',
    generatedFileCaddy: '생성된 Gymbagfile'
  },
  es: {
    eyebrow: 'Inicio Rápido en 2 Comandos',
    title: 'Vista Previa de Comandos y Config',
    copyBtn: 'Copiar 2 Comandos',
    copiedBtn: '¡Copiado!',
    services: 'Servicios',
    svcHttp: 'Sitio HTTP / HTTPS',
    svcSsh: 'Servidor SSH',
    svcMc: 'Servidor Minecraft',
    svcAll: 'Todo en Uno (HTTP + SSH + MC)',
    format: 'Formato',
    fmtJson: 'gymbag.json (predeterminado)',
    fmtGymbagfile: 'Gymbagfile (sintaxis Caddy)',
    domain: 'Dominio',
    upstream: 'URL Upstream',
    sshPort: 'Puerto SSH',
    mcProps: 'Ruta a server.properties',
    mcPort: 'Puerto Minecraft',
    verifyUrl: 'URL del Portal (opcional)',
    verifyUrlPlaceholder: 'vacío = IP del servidor + cert SSL de IP',
    verifyPort: 'Puerto del Portal',
    verifyLevel: 'Nivel de Verificación',
    verify3: '3 — sospechoso e IA (Predeterminado)',
    verify2: '2 — solo IA',
    verify1: '1 — solo sospechoso',
    verify0: '0 — desafiar siempre',
    verifyOff: 'off — desactivado',
    service: 'Servicio del Sistema',
    serviceNone: 'Ninguno (ejecutar directo)',
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
    step2Start: '# 2. Iniciar',
    generatedFileJson: 'gymbag.json generado',
    generatedFileCaddy: 'Gymbagfile generado'
  }
}

const t = computed(() => copyByLocale[activeLocale.value])

const serviceMode = ref<ServiceMode>('http')
const cfgFormat = ref<ConfigFormat>('json')
const domain = ref('app.example.com')
const upstream = ref('http://localhost:3000')
const sshPort = ref('22')
const mcProps = ref('./server.properties')
const mcPort = ref('25565')
const verifyUrl = ref('')
const verifyPort = ref('8443')
const verify = ref<'0' | '1' | '2' | '3' | 'off'>('3')
const sslProvider = ref('letsencrypt')
const email = ref('ops@example.com')
const serviceMgr = ref<'none' | 'auto' | 'systemd' | 'launchd' | 'openrc' | 'supervisor'>('none')
const copied = ref(false)

const useHTTP = computed(() => serviceMode.value === 'http' || serviceMode.value === 'all')
const useSSH = computed(() => serviceMode.value === 'ssh' || serviceMode.value === 'all')
const useMC = computed(() => serviceMode.value === 'minecraft' || serviceMode.value === 'all')

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

const needsSudo = computed(() => useSSH.value)

const initCommand = computed(() => {
  const parts = needsSudo.value ? ['sudo', './gymbag', 'init'] : ['./gymbag', 'init']

  if (serviceMode.value === 'ssh') {
    parts.push('-t', 'ssh')
  } else if (serviceMode.value === 'minecraft') {
    parts.push('-t', 'minecraft')
  } else if (serviceMode.value === 'all') {
    parts.push('-t', 'http,ssh,minecraft')
  }

  if (useHTTP.value) {
    const d = domain.value.trim() || 'localhost'
    const u = upstream.value.trim() || 'http://localhost:3000'
    parts.push('-d', d, '-u', u)
  }

  if (useSSH.value && sshPort.value.trim() && sshPort.value.trim() !== '22') {
    parts.push('--ssh-port', sshPort.value.trim())
  }

  if (useMC.value) {
    if (mcProps.value.trim()) {
      parts.push('--server-properties', mcProps.value.trim())
    }
    if (mcPort.value.trim() && mcPort.value.trim() !== '25565') {
      parts.push('--mc-port', mcPort.value.trim())
    }
  }

  if (useSSH.value || useMC.value) {
    if (verifyUrl.value.trim()) {
      parts.push('--verify-url', verifyUrl.value.trim())
    }
    if (verifyPort.value.trim() && verifyPort.value.trim() !== '8443') {
      parts.push('--verify-port', verifyPort.value.trim())
    }
  }

  if (verify.value === 'off') {
    parts.push('--no-protect')
  } else if (verify.value !== '3') {
    parts.push('-l', verify.value)
  }

  if (useHTTP.value && !isLocalDomain.value) {
    if (sslProvider.value !== 'letsencrypt') {
      parts.push('--ssl-provider', sslProvider.value)
    }
    if (email.value.trim()) {
      parts.push('--email', email.value.trim())
    }
  }

  if (cfgFormat.value === 'gymbagfile') {
    parts.push('--format', 'gymbagfile')
  }

  if (serviceMgr.value !== 'none') {
    parts.push('--service', serviceMgr.value)
  }

  return parts.join(' ')
})

const startCommand = computed(() => {
  const prefix = needsSudo.value ? 'sudo ' : ''
  if (serviceMgr.value === 'none') {
    return `${prefix}./gymbag`
  }
  return `${prefix}./gymbag service start`
})

const generatedPreview = computed(() => {
  const d = domain.value.trim() || 'localhost'
  const u = upstream.value.trim() || 'http://localhost:3000'
  const vNum = verify.value === 'off' ? null : Number(verify.value)
  const sp = Number(sshPort.value.trim()) || 22
  const mp = Number(mcPort.value.trim()) || 25565
  const vp = Number(verifyPort.value.trim()) || 8443
  const vUrl = verifyUrl.value.trim()

  if (cfgFormat.value === 'json') {
    const out: Record<string, unknown> = {}

    if (useHTTP.value) {
      const site: Record<string, unknown> = {
        domain: d,
        upstreams: [u]
      }
      if (verify.value === 'off') {
        site.protect = false
      } else {
        site.waf = { verify_level: vNum }
      }
      out.sites = [site]
    }

    if (useSSH.value) {
      const sshObj: Record<string, unknown> = {
        port: sp,
        upstream: '127.0.0.1:41822'
      }
      if (verify.value === 'off') {
        sshObj.protect = false
      } else {
        sshObj.verify_level = vNum
      }
      out.ssh = sshObj
    }

    if (useMC.value) {
      const mcObj: Record<string, unknown> = {
        port: mp,
        upstream: '127.0.0.1:48291'
      }
      if (mcProps.value.trim()) {
        mcObj.server_properties = mcProps.value.trim()
      }
      if (verify.value === 'off') {
        mcObj.protect = false
      } else {
        mcObj.verify_level = vNum
      }
      out.minecraft = mcObj
    }

    if ((useSSH.value || useMC.value) && (vUrl || vp !== 8443)) {
      const verifyObj: Record<string, unknown> = {}
      if (vUrl) verifyObj.url = vUrl
      if (vp !== 8443) verifyObj.port = vp
      out.verify = verifyObj
    }

    if (useHTTP.value && !isLocalDomain.value) {
      const tlsObj: Record<string, unknown> = {}
      if (sslProvider.value !== 'letsencrypt') tlsObj.provider = sslProvider.value
      if (email.value.trim()) tlsObj.acme_email = email.value.trim()
      if (Object.keys(tlsObj).length > 0) out.tls = tlsObj
    }

    return JSON.stringify(out, null, 2)
  }

  const lines: string[] = []
  const globals: string[] = []

  if (useHTTP.value && !isLocalDomain.value) {
    if (email.value.trim()) globals.push(`    email ${email.value.trim()}`)
    if (sslProvider.value !== 'letsencrypt') globals.push(`    ssl_provider ${sslProvider.value}`)
  }
  if (useSSH.value || useMC.value) {
    if (vUrl) globals.push(`    verify_url ${vUrl}`)
    if (vp !== 8443) globals.push(`    verify_port ${vp}`)
  }

  if (globals.length > 0) {
    lines.push('{', ...globals, '}', '')
  }

  if (useHTTP.value) {
    lines.push(`${d} {`, `    reverse_proxy ${u}`)
    if (verify.value === 'off') {
      lines.push('    protect off')
    } else if (verify.value !== '0') {
      lines.push(`    verify ${verify.value}`)
    }
    lines.push('}')
  }

  if (useSSH.value) {
    if (lines.length > 0) lines.push('')
    lines.push('ssh {')
    if (sp !== 22) lines.push(`    port ${sp}`)
    lines.push('    upstream 127.0.0.1:41822')
    if (verify.value === 'off') {
      lines.push('    protect off')
    } else {
      lines.push(`    verify ${verify.value}`)
    }
    lines.push('}')
  }

  if (useMC.value) {
    if (lines.length > 0) lines.push('')
    lines.push('minecraft {')
    if (mp !== 25565) lines.push(`    port ${mp}`)
    lines.push('    upstream 127.0.0.1:48291')
    if (mcProps.value.trim()) lines.push(`    server_properties ${mcProps.value.trim()}`)
    if (verify.value === 'off') {
      lines.push('    protect off')
    } else {
      lines.push(`    verify ${verify.value}`)
    }
    lines.push('}')
  }

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
        <span>{{ t.services }}</span>
        <select v-model="serviceMode">
          <option value="http">{{ t.svcHttp }}</option>
          <option value="ssh">{{ t.svcSsh }}</option>
          <option value="minecraft">{{ t.svcMc }}</option>
          <option value="all">{{ t.svcAll }}</option>
        </select>
      </label>

      <label class="qb-field">
        <span>{{ t.format }}</span>
        <select v-model="cfgFormat">
          <option value="json">{{ t.fmtJson }}</option>
          <option value="gymbagfile">{{ t.fmtGymbagfile }}</option>
        </select>
      </label>

      <template v-if="useHTTP">
        <label class="qb-field">
          <span>{{ t.domain }}</span>
          <input v-model="domain" type="text" placeholder="app.example.com" />
        </label>

        <label class="qb-field">
          <span>{{ t.upstream }}</span>
          <input v-model="upstream" type="text" placeholder="http://localhost:3000" />
        </label>
      </template>

      <template v-if="useSSH">
        <label class="qb-field">
          <span>{{ t.sshPort }}</span>
          <input v-model="sshPort" type="text" placeholder="22" />
        </label>
      </template>

      <template v-if="useMC">
        <label class="qb-field">
          <span>{{ t.mcProps }}</span>
          <input v-model="mcProps" type="text" placeholder="./server.properties" />
        </label>

        <label class="qb-field">
          <span>{{ t.mcPort }}</span>
          <input v-model="mcPort" type="text" placeholder="25565" />
        </label>
      </template>

      <template v-if="useSSH || useMC">
        <label class="qb-field">
          <span>{{ t.verifyUrl }}</span>
          <input v-model="verifyUrl" type="text" :placeholder="t.verifyUrlPlaceholder" />
        </label>

        <label class="qb-field">
          <span>{{ t.verifyPort }}</span>
          <input v-model="verifyPort" type="text" placeholder="8443" />
        </label>
      </template>

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

      <template v-if="useHTTP && !isLocalDomain">
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
        <div class="qb-pane-label">{{ cfgFormat === 'json' ? t.generatedFileJson : t.generatedFileCaddy }}</div>
        <pre class="qb-pre"><code>{{ generatedPreview }}</code></pre>
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
