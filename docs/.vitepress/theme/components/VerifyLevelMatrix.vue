<script setup lang="ts">
import { computed, ref } from 'vue'

type LevelKey = '3' | '2' | '1' | '0' | 'off'

interface LevelSpec {
  key: LevelKey
  label: string
  directive: string
  summary: string
  outcomes: {
    visitor: string
    status: 'pass' | 'challenge' | 'block'
    outcomeText: string
    reason: string
  }[]
}

const levels: LevelSpec[] = [
  {
    key: '3',
    label: '3 — suspicious & ai (Default)',
    directive: 'verify 3',
    summary:
      'Challenges both suspicious clients/IPs and AI crawlers while letting normal human browsers pass straight through.',
    outcomes: [
      { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: 'Pass Through', reason: 'Standard browser headers & clean IP' },
      { visitor: 'GPTBot / ClaudeBot / Applebot', status: 'challenge', outcomeText: 'PoW Challenge', reason: 'Matched official AI IP range or AI UA' },
      { visitor: 'curl / python-requests / Headless', status: 'challenge', outcomeText: 'PoW Challenge', reason: 'Built-in automation & header heuristics' },
      { visitor: 'Spamhaus / IPsum Threat IP', status: 'challenge', outcomeText: 'PoW (or 403)', reason: 'Threat-intel action (verify by default, or block)' },
      { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 Blocked', reason: 'Built-in default_rules baseline protection' }
    ]
  },
  {
    key: '2',
    label: '2 — only ai',
    directive: 'verify 2',
    summary:
      'Zero friction for humans and curl/API tooling; only AI crawlers and LLM agents must solve Proof-of-Work.',
    outcomes: [
      { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: 'Pass Through', reason: 'Not classified as an AI crawler' },
      { visitor: 'GPTBot / ClaudeBot / PerplexityBot', status: 'challenge', outcomeText: 'PoW Challenge', reason: 'Matched AI crawler CIDR or User-Agent' },
      { visitor: 'curl / normal API clients', status: 'pass', outcomeText: 'Pass Through', reason: 'Generic CLI clients are allowed at level 2' },
      { visitor: 'Spamhaus / IPsum Threat IP', status: 'challenge', outcomeText: 'PoW (or 403)', reason: 'Global IP threat intelligence runs independently' },
      { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 Blocked', reason: 'Built-in default_rules baseline protection' }
    ]
  },
  {
    key: '1',
    label: '1 — only suspicious',
    directive: 'verify 1',
    summary:
      'Challenges scanners, headless browsers, spoofed headers, and bad IPs while leaving normal browsers untouched.',
    outcomes: [
      { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: 'Pass Through', reason: 'Normal browser fingerprint' },
      { visitor: 'Official AI crawler (clean UA)', status: 'pass', outcomeText: 'Pass Through', reason: 'AI-only triggers are disabled at level 1' },
      { visitor: 'curl / Scrapy / Missing headers', status: 'challenge', outcomeText: 'PoW Challenge', reason: 'Suspicious automation / missing browser headers' },
      { visitor: 'Spamhaus / IPsum Threat IP', status: 'challenge', outcomeText: 'PoW (or 403)', reason: 'Threat-intel match' },
      { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 Blocked', reason: 'Built-in default_rules baseline protection' }
    ]
  },
  {
    key: '0',
    label: '0 — always',
    directive: 'verify 0',
    summary:
      'Requires every visitor without a valid HMAC waf_clearance cookie to solve a SHA-256 Proof-of-Work puzzle.',
    outcomes: [
      { visitor: 'Chrome / Firefox / Safari', status: 'challenge', outcomeText: 'PoW Challenge', reason: 'Every visitor challenged once per clearance TTL' },
      { visitor: 'GPTBot / ClaudeBot / Applebot', status: 'challenge', outcomeText: 'PoW Challenge', reason: 'All traffic challenged' },
      { visitor: 'curl / python-requests', status: 'challenge', outcomeText: 'PoW Challenge', reason: 'All traffic challenged' },
      { visitor: 'Spamhaus / IPsum Threat IP', status: 'challenge', outcomeText: 'PoW (or 403)', reason: 'Threat-intel match' },
      { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 Blocked', reason: 'Built-in default_rules baseline protection' }
    ]
  },
  {
    key: 'off',
    label: 'protect off',
    directive: 'protect off',
    summary:
      'Disables Proof-of-Work challenges for static assets, webhooks, or mirrors while keeping HTTPS & explicit custom rules active.',
    outcomes: [
      { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: 'Pass Through', reason: 'Direct HTTPS & WebSocket reverse proxy' },
      { visitor: 'GPTBot / ClaudeBot', status: 'pass', outcomeText: 'Pass Through', reason: 'WAF verification disabled' },
      { visitor: 'curl / webhook clients', status: 'pass', outcomeText: 'Pass Through', reason: 'WAF verification disabled' },
      { visitor: 'Custom site rules (if defined)', status: 'block', outcomeText: 'Evaluated', reason: 'Explicit custom allow/block rules still apply' },
      { visitor: 'TLS & HTTP->HTTPS redirect', status: 'pass', outcomeText: 'Active', reason: 'ACME & Local PKI remain active' }
    ]
  }
]

const selected = ref<LevelKey>('3')
const current = computed(() => levels.find((l) => l.key === selected.value)!)
</script>

<template>
  <div class="gb-card">
    <div class="gb-pill-row">
      <button
        v-for="lvl in levels"
        :key="lvl.key"
        type="button"
        class="gb-pill"
        :class="{ active: selected === lvl.key }"
        @click="selected = lvl.key"
      >
        {{ lvl.label }}
      </button>
    </div>

    <p class="vm-summary">
      <code>{{ current.directive }}</code> — {{ current.summary }}
    </p>

    <div class="vm-table-wrap">
      <table class="vm-table">
        <thead>
          <tr>
            <th>Client / Traffic</th>
            <th>Action</th>
            <th>Why</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in current.outcomes" :key="row.visitor">
            <td><strong>{{ row.visitor }}</strong></td>
            <td>
              <span class="gb-badge" :class="row.status">{{ row.outcomeText }}</span>
            </td>
            <td>{{ row.reason }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.vm-summary {
  margin: 0 0 1rem;
  color: var(--vp-c-text-2);
}

.vm-table-wrap {
  overflow-x: auto;
}

.vm-table {
  width: 100%;
  border-collapse: collapse;
  margin: 0;
}

.vm-table th,
.vm-table td {
  text-align: left;
  padding: 0.6rem 0.75rem;
  border-bottom: 1px solid var(--vp-c-border);
  font-size: 0.9rem;
}

.vm-table th {
  font-size: 0.78rem;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--vp-c-text-3);
}
</style>
