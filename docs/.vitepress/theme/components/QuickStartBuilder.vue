<script setup lang="ts">
import { computed, ref } from 'vue'

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
        <div class="qb-eyebrow">Two-Command Quickstart</div>
        <h3 class="qb-title">Configure &amp; Launch Preview</h3>
      </div>
      <button class="gb-pill" type="button" @click="copyCommands">
        {{ copied ? 'Copied!' : 'Copy 2 Commands' }}
      </button>
    </div>

    <div class="qb-grid">
      <label class="qb-field">
        <span>Domain</span>
        <input v-model="domain" type="text" placeholder="app.example.com" />
      </label>

      <label class="qb-field">
        <span>Upstream URL</span>
        <input v-model="upstream" type="text" placeholder="http://localhost:3000" />
      </label>

      <label class="qb-field">
        <span>Verify Level</span>
        <select v-model="verify">
          <option value="3">3 — suspicious &amp; ai (Recommended)</option>
          <option value="2">2 — only ai</option>
          <option value="1">1 — only suspicious</option>
          <option value="0">0 — always challenge</option>
          <option value="off">off — protect off</option>
        </select>
      </label>

      <label class="qb-field">
        <span>System Service</span>
        <select v-model="serviceMgr">
          <option value="none">None (run directly with ./gymbag)</option>
          <option value="auto">auto (detect OS init)</option>
          <option value="systemd">systemd (Linux)</option>
          <option value="launchd">launchd (macOS)</option>
          <option value="openrc">OpenRC (Alpine / Gentoo)</option>
          <option value="supervisor">Supervisor</option>
        </select>
      </label>

      <template v-if="!isLocalDomain">
        <label class="qb-field">
          <span>SSL Provider</span>
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
          <span>ACME Email</span>
          <input v-model="email" type="email" placeholder="ops@example.com" />
        </label>
      </template>
    </div>

    <div class="qb-outputs">
      <div class="qb-pane">
        <div class="qb-pane-label">Run these 2 commands</div>
        <pre class="qb-pre"><code><span class="qb-step"># 1. Generate config{{ serviceMgr !== 'none' ? ' &amp; register service' : '' }}</span>
{{ initCommand }}

<span class="qb-step"># 2. Start service</span>
{{ startCommand }}</code></pre>
      </div>

      <div class="qb-pane">
        <div class="qb-pane-label">Generated Gymbagfile</div>
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
