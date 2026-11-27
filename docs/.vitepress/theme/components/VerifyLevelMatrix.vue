<script setup lang="ts">
import { computed, ref } from 'vue'
import { useData } from 'vitepress'

const { lang } = useData()

type LocaleKey = 'en' | 'zh' | 'zh-TW' | 'ja' | 'ko' | 'es'
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

const activeLocale = computed<LocaleKey>(() => {
  const raw = (lang.value || 'en').toLowerCase()
  if (raw === 'zh-tw' || raw === 'zh-hant') {
    return 'zh-TW'
  }
  const code = raw.slice(0, 2)
  if (code === 'zh' || code === 'ja' || code === 'ko' || code === 'es') {
    return code
  }
  return 'en'
})

const headersByLocale: Record<LocaleKey, { client: string; action: string; why: string }> = {
  en: { client: 'Client / Traffic', action: 'Action', why: 'Why' },
  zh: { client: '客户端 / 流量类型', action: '处理动作', why: '判定原因' },
  'zh-TW': { client: '用戶端 / 流量類型', action: '處理動作', why: '判定原因' },
  ja: { client: 'クライアント / トラフィック', action: 'アクション', why: '理由' },
  ko: { client: '클라이언트 / 트래픽', action: '동작', why: '판정 이유' },
  es: { client: 'Cliente / Tráfico', action: 'Acción', why: 'Motivo' }
}

const levelsByLocale: Record<LocaleKey, LevelSpec[]> = {
  en: [
    {
      key: '3',
      label: '3 — suspicious & ai (Default)',
      directive: 'verify 3',
      summary:
        'Challenges suspicious clients/IPs (including VPN/proxy/Tor IPs) and AI crawlers, while regular browsers pass right through.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: 'Pass Through', reason: 'Normal browser headers and clean IP' },
        { visitor: 'GPTBot / ClaudeBot / Applebot', status: 'challenge', outcomeText: 'PoW Challenge', reason: 'Matched official AI CIDR list or AI UA' },
        { visitor: 'VPN / Proxy / Tor IP', status: 'challenge', outcomeText: 'PoW Challenge', reason: 'Flagged by X4BNet/Tor lists or live ipquery.io / ipapi.is check (HTTP, SSH, Minecraft)' },
        { visitor: 'curl / python-requests / Headless', status: 'challenge', outcomeText: 'PoW Challenge', reason: 'Automation UA, missing headers, or scripted SSH/MC client' },
        { visitor: 'Spamhaus / IPsum Threat IP', status: 'challenge', outcomeText: 'PoW (or 403)', reason: 'Threat feed hit (verify by default, or 403 when ip_security_action is block)' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 Blocked', reason: 'Built-in default_rules baseline rule' }
      ]
    },
    {
      key: '2',
      label: '2 — only ai',
      directive: 'verify 2',
      summary:
        'Lets humans, curl, and VPN users pass; only AI crawlers and LLM agents get a PoW challenge.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: 'Pass Through', reason: 'Not an AI crawler' },
        { visitor: 'GPTBot / ClaudeBot / PerplexityBot', status: 'challenge', outcomeText: 'PoW Challenge', reason: 'Matched AI crawler CIDR or User-Agent' },
        { visitor: 'VPN / Proxy / Tor IP', status: 'pass', outcomeText: 'Pass Through', reason: 'VPN/suspicious checks are off at level 2' },
        { visitor: 'curl / normal API clients', status: 'pass', outcomeText: 'Pass Through', reason: 'Generic CLI clients pass at level 2' },
        { visitor: 'Spamhaus / IPsum Threat IP', status: 'challenge', outcomeText: 'PoW (or 403)', reason: 'Global IP threat intel runs on all protected sites' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 Blocked', reason: 'Built-in default_rules baseline rule' }
      ]
    },
    {
      key: '1',
      label: '1 — only suspicious',
      directive: 'verify 1',
      summary:
        'Challenges scanners, headless browsers, spoofed headers, VPN/proxy/Tor IPs, and bad IPs while leaving normal browsers alone.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: 'Pass Through', reason: 'Normal browser headers and clean IP' },
        { visitor: 'Official AI crawler (clean UA)', status: 'pass', outcomeText: 'Pass Through', reason: 'AI-only triggers are off at level 1' },
        { visitor: 'VPN / Proxy / Tor IP', status: 'challenge', outcomeText: 'PoW Challenge', reason: 'Flagged by X4BNet/Tor lists or live ipquery.io / ipapi.is check (HTTP, SSH, Minecraft)' },
        { visitor: 'curl / Scrapy / Missing headers', status: 'challenge', outcomeText: 'PoW Challenge', reason: 'Automation tool, missing browser headers, or suspicious SSH/MC client' },
        { visitor: 'Spamhaus / IPsum Threat IP', status: 'challenge', outcomeText: 'PoW (or 403)', reason: 'Threat-intel match' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 Blocked', reason: 'Built-in default_rules baseline rule' }
      ]
    },
    {
      key: '0',
      label: '0 — always',
      directive: 'verify 0',
      summary:
        'Requires every visitor or TCP IP without an active clearance to solve a SHA-256 PoW challenge once.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'challenge', outcomeText: 'PoW Challenge', reason: 'Everyone is challenged once per clearance TTL' },
        { visitor: 'GPTBot / ClaudeBot / Applebot', status: 'challenge', outcomeText: 'PoW Challenge', reason: 'All traffic challenged' },
        { visitor: 'VPN / Proxy / Tor IP', status: 'challenge', outcomeText: 'PoW Challenge', reason: 'All traffic challenged' },
        { visitor: 'curl / python-requests', status: 'challenge', outcomeText: 'PoW Challenge', reason: 'All traffic challenged' },
        { visitor: 'Spamhaus / IPsum Threat IP', status: 'challenge', outcomeText: 'PoW (or 403)', reason: 'Threat-intel match' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 Blocked', reason: 'Built-in default_rules baseline rule' }
      ]
    },
    {
      key: 'off',
      label: 'protect off',
      directive: 'protect off',
      summary:
        'Turns off PoW challenges for static assets, webhooks, or mirrors while keeping HTTPS and any explicit custom rules active.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: 'Pass Through', reason: 'Plain HTTPS and WebSocket reverse proxy' },
        { visitor: 'GPTBot / ClaudeBot', status: 'pass', outcomeText: 'Pass Through', reason: 'WAF verification off' },
        { visitor: 'VPN / Proxy / Tor IP', status: 'pass', outcomeText: 'Pass Through', reason: 'WAF verification off' },
        { visitor: 'curl / webhook clients', status: 'pass', outcomeText: 'Pass Through', reason: 'WAF verification off' },
        { visitor: 'Custom site rules (if defined)', status: 'block', outcomeText: 'Evaluated', reason: 'Explicit allow/block rules still run' },
        { visitor: 'TLS & HTTP->HTTPS redirect', status: 'pass', outcomeText: 'Active', reason: 'ACME and Local PKI stay on' }
      ]
    }
  ],
  zh: [
    {
      key: '3',
      label: '3 — 可疑流量与 AI（默认）',
      directive: 'verify 3',
      summary: '质询可疑客户端/IP（含 VPN/代理/Tor IP）和 AI 爬虫，普通浏览器直接放行。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '直接放行', reason: '正常浏览器请求头且 IP 干净' },
        { visitor: 'GPTBot / ClaudeBot / Applebot', status: 'challenge', outcomeText: 'PoW 质询', reason: '命中官方 AI IP 段或 AI UA' },
        { visitor: 'VPN / 代理 / Tor 出口 IP', status: 'challenge', outcomeText: 'PoW 质询', reason: '命中 X4BNet/Tor 列表或 ipquery.io / ipapi.is 实时检测（HTTP、SSH、Minecraft 均生效）' },
        { visitor: 'curl / python-requests / 无头浏览器', status: 'challenge', outcomeText: 'PoW 质询', reason: '自动化工具、缺失请求头或可疑 SSH/MC 客户端' },
        { visitor: 'Spamhaus / IPsum 威胁 IP', status: 'challenge', outcomeText: 'PoW（或 403）', reason: '命中威胁情报（默认 verify，设为 block 则 403）' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 拦截', reason: '内置 default_rules 基础规则' }
      ]
    },
    {
      key: '2',
      label: '2 — 仅 AI 爬虫',
      directive: 'verify 2',
      summary: '放行普通访客、curl 和 VPN 用户；只让 AI 爬虫和 LLM Agent 过工作量证明。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '直接放行', reason: '不属于 AI 爬虫' },
        { visitor: 'GPTBot / ClaudeBot / PerplexityBot', status: 'challenge', outcomeText: 'PoW 质询', reason: '命中 AI 爬虫 CIDR 或 User-Agent' },
        { visitor: 'VPN / 代理 / Tor 出口 IP', status: 'pass', outcomeText: '直接放行', reason: '级别 2 不触发可疑/VPN 检查' },
        { visitor: 'curl / 常规 API 客户端', status: 'pass', outcomeText: '直接放行', reason: '级别 2 放行常规命令行客户端' },
        { visitor: 'Spamhaus / IPsum 威胁 IP', status: 'challenge', outcomeText: 'PoW（或 403）', reason: '全局威胁情报仍会生效' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 拦截', reason: '内置 default_rules 基础规则' }
      ]
    },
    {
      key: '1',
      label: '1 — 仅可疑流量',
      directive: 'verify 1',
      summary: '只对扫描器、无头浏览器、伪造请求头、VPN/代理/Tor IP 和恶意 IP 发起质询。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '直接放行', reason: '正常浏览器特征与干净 IP' },
        { visitor: '官方 AI 爬虫（常规 UA）', status: 'pass', outcomeText: '直接放行', reason: '级别 1 不启用纯 AI 触发器' },
        { visitor: 'VPN / 代理 / Tor 出口 IP', status: 'challenge', outcomeText: 'PoW 质询', reason: '命中 X4BNet/Tor 列表或 ipquery.io / ipapi.is 实时检测（HTTP、SSH、Minecraft 均生效）' },
        { visitor: 'curl / Scrapy / 缺失请求头', status: 'challenge', outcomeText: 'PoW 质询', reason: '自动化工具、缺少浏览器请求头或可疑 SSH/MC 客户端' },
        { visitor: 'Spamhaus / IPsum 威胁 IP', status: 'challenge', outcomeText: 'PoW（或 403）', reason: '命中威胁情报库' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 拦截', reason: '内置 default_rules 基础规则' }
      ]
    },
    {
      key: '0',
      label: '0 — 始终验证',
      directive: 'verify 0',
      summary: '所有没有有效放行凭证的访客或 TCP 客户端都要先过一遍 SHA-256 工作量证明。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'challenge', outcomeText: 'PoW 质询', reason: '每个访客在有效期内验证一次' },
        { visitor: 'GPTBot / ClaudeBot / Applebot', status: 'challenge', outcomeText: 'PoW 质询', reason: '全部流量均需验证' },
        { visitor: 'VPN / 代理 / Tor 出口 IP', status: 'challenge', outcomeText: 'PoW 质询', reason: '全部流量均需验证' },
        { visitor: 'curl / python-requests', status: 'challenge', outcomeText: 'PoW 质询', reason: '全部流量均需验证' },
        { visitor: 'Spamhaus / IPsum 威胁 IP', status: 'challenge', outcomeText: 'PoW（或 403）', reason: '命中威胁情报库' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 拦截', reason: '内置 default_rules 基础规则' }
      ]
    },
    {
      key: 'off',
      label: 'protect off',
      directive: 'protect off',
      summary: '为静态资源、Webhook 或镜像站关闭 PoW 质询，但保留 HTTPS 与你写下的自定义规则。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '直接放行', reason: '纯 HTTPS 与 WebSocket 反向代理' },
        { visitor: 'GPTBot / ClaudeBot', status: 'pass', outcomeText: '直接放行', reason: 'WAF 验证已关闭' },
        { visitor: 'VPN / 代理 / Tor 出口 IP', status: 'pass', outcomeText: '直接放行', reason: 'WAF 验证已关闭' },
        { visitor: 'curl / Webhook 客户端', status: 'pass', outcomeText: '直接放行', reason: 'WAF 验证已关闭' },
        { visitor: '站点自定义规则（若已配置）', status: 'block', outcomeText: '照常执行', reason: '显式 allow/block 规则仍生效' },
        { visitor: 'TLS 与 HTTP->HTTPS 跳转', status: 'pass', outcomeText: '已启用', reason: 'ACME 与本地 PKI 照常工作' }
      ]
    }
  ],
  'zh-TW': [
    {
      key: '3',
      label: '3 — 可疑流量與 AI（預設）',
      directive: 'verify 3',
      summary: '對可疑用戶端/IP（含 VPN/代理/Tor IP）與 AI 爬蟲發起 PoW 挑戰，一般瀏覽器直接放行。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '直接放行', reason: '正常的瀏覽器標頭與乾淨 IP' },
        { visitor: 'GPTBot / ClaudeBot / Applebot', status: 'challenge', outcomeText: 'PoW 挑戰', reason: '符合官方 AI IP 網段或 AI UA' },
        { visitor: 'VPN / 代理 / Tor 出口 IP', status: 'challenge', outcomeText: 'PoW 挑戰', reason: '命中 X4BNet/Tor 清單或 ipquery.io / ipapi.is 即時偵測（HTTP、SSH、Minecraft 皆適用）' },
        { visitor: 'curl / python-requests / 無頭瀏覽器', status: 'challenge', outcomeText: 'PoW 挑戰', reason: '自動化工具、缺少標頭或可疑 SSH/MC 用戶端' },
        { visitor: 'Spamhaus / IPsum 威脅 IP', status: 'challenge', outcomeText: 'PoW（或 403）', reason: '命中威脅情資（預設 verify，設為 block 則 403）' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 攔截', reason: '內建 default_rules 基礎規則' }
      ]
    },
    {
      key: '2',
      label: '2 — 僅 AI 爬蟲',
      directive: 'verify 2',
      summary: '放行一般訪客、curl 與 VPN 使用者；僅對 AI 爬蟲和 LLM Agent 要求工作量證明。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '直接放行', reason: '非 AI 爬蟲' },
        { visitor: 'GPTBot / ClaudeBot / PerplexityBot', status: 'challenge', outcomeText: 'PoW 挑戰', reason: '符合 AI 爬蟲 CIDR 或 User-Agent' },
        { visitor: 'VPN / 代理 / Tor 出口 IP', status: 'pass', outcomeText: '直接放行', reason: '等級 2 不觸發可疑/VPN 檢查' },
        { visitor: 'curl / 一般 API 用戶端', status: 'pass', outcomeText: '直接放行', reason: '等級 2 放行一般命令列用戶端' },
        { visitor: 'Spamhaus / IPsum 威脅 IP', status: 'challenge', outcomeText: 'PoW（或 403）', reason: '全域威脅情資仍會生效' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 攔截', reason: '內建 default_rules 基礎規則' }
      ]
    },
    {
      key: '1',
      label: '1 — 僅可疑流量',
      directive: 'verify 1',
      summary: '僅對掃描器、無頭瀏覽器、偽造標頭、VPN/代理/Tor IP 與惡意 IP 發起挑戰。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '直接放行', reason: '正常的瀏覽器特徵與乾淨 IP' },
        { visitor: '官方 AI 爬蟲（正常 UA）', status: 'pass', outcomeText: '直接放行', reason: '等級 1 不啟用純 AI 觸發器' },
        { visitor: 'VPN / 代理 / Tor 出口 IP', status: 'challenge', outcomeText: 'PoW 挑戰', reason: '命中 X4BNet/Tor 清單或 ipquery.io / ipapi.is 即時偵測（HTTP、SSH、Minecraft 皆適用）' },
        { visitor: 'curl / Scrapy / 缺少標頭', status: 'challenge', outcomeText: 'PoW 挑戰', reason: '自動化工具、缺少瀏覽器標頭或可疑 SSH/MC 用戶端' },
        { visitor: 'Spamhaus / IPsum 威脅 IP', status: 'challenge', outcomeText: 'PoW（或 403）', reason: '命中威脅情資庫' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 攔截', reason: '內建 default_rules 基礎規則' }
      ]
    },
    {
      key: '0',
      label: '0 — 一律驗證',
      directive: 'verify 0',
      summary: '所有沒有有效放行憑證的訪客或 TCP 用戶端，都必須先通過一次 SHA-256 工作量證明。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'challenge', outcomeText: 'PoW 挑戰', reason: '每位訪客在有效期內驗證一次' },
        { visitor: 'GPTBot / ClaudeBot / Applebot', status: 'challenge', outcomeText: 'PoW 挑戰', reason: '所有流量皆須驗證' },
        { visitor: 'VPN / 代理 / Tor 出口 IP', status: 'challenge', outcomeText: 'PoW 挑戰', reason: '所有流量皆須驗證' },
        { visitor: 'curl / python-requests', status: 'challenge', outcomeText: 'PoW 挑戰', reason: '所有流量皆須驗證' },
        { visitor: 'Spamhaus / IPsum 威脅 IP', status: 'challenge', outcomeText: 'PoW（或 403）', reason: '命中威脅情資庫' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 攔截', reason: '內建 default_rules 基礎規則' }
      ]
    },
    {
      key: 'off',
      label: 'protect off',
      directive: 'protect off',
      summary: '針對靜態資源、Webhook 或鏡像站關閉 PoW 挑戰，但保留 HTTPS 與你設定的自訂規則。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '直接放行', reason: '純 HTTPS 與 WebSocket 反向代理' },
        { visitor: 'GPTBot / ClaudeBot', status: 'pass', outcomeText: '直接放行', reason: 'WAF 驗證已關閉' },
        { visitor: 'VPN / 代理 / Tor 出口 IP', status: 'pass', outcomeText: '直接放行', reason: 'WAF 驗證已關閉' },
        { visitor: 'curl / Webhook 用戶端', status: 'pass', outcomeText: '直接放行', reason: 'WAF 驗證已關閉' },
        { visitor: '站台自訂規則（若有設定）', status: 'block', outcomeText: '照常執行', reason: '明確的 allow/block 規則仍會生效' },
        { visitor: 'TLS 與 HTTP->HTTPS 跳轉', status: 'pass', outcomeText: '已啟用', reason: 'ACME 與本地 PKI 照常運作' }
      ]
    }
  ],
  ja: [
    {
      key: '3',
      label: '3 — 不審 & AI（既定）',
      directive: 'verify 3',
      summary: '不審なクライアント/IP（VPN・プロキシ・Tor IP を含む）と AI クローラーに PoW を要求し、普通のブラウザはそのまま通します。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '通過', reason: '通常のブラウザヘッダー & クリーンな IP' },
        { visitor: 'GPTBot / ClaudeBot / Applebot', status: 'challenge', outcomeText: 'PoW チャレンジ', reason: '公式 AI CIDR または AI UA に一致' },
        { visitor: 'VPN / プロキシ / Tor 出口 IP', status: 'challenge', outcomeText: 'PoW チャレンジ', reason: 'X4BNet/Tor リストまたは ipquery.io / ipapi.is で検出（HTTP・SSH・Minecraft 共通）' },
        { visitor: 'curl / python-requests / Headless', status: 'challenge', outcomeText: 'PoW チャレンジ', reason: '自動化ツール・ヘッダー欠落・不審な SSH/MC クライアント' },
        { visitor: 'Spamhaus / IPsum 脅威 IP', status: 'challenge', outcomeText: 'PoW (または 403)', reason: '脅威フィード一致（既定 verify、block 設定時は 403）' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 ブロック', reason: '組み込み default_rules ルール' }
      ]
    },
    {
      key: '2',
      label: '2 — AI のみ',
      directive: 'verify 2',
      summary: '一般ブラウザ・curl・VPN ユーザーはそのまま通過し、AI クローラーと LLM エージェントだけに PoW を課します。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '通過', reason: 'AI クローラーではない' },
        { visitor: 'GPTBot / ClaudeBot / PerplexityBot', status: 'challenge', outcomeText: 'PoW チャレンジ', reason: 'AI クローラー CIDR または User-Agent に一致' },
        { visitor: 'VPN / プロキシ / Tor 出口 IP', status: 'pass', outcomeText: '通過', reason: 'レベル 2 では VPN / 不審判定を行いません' },
        { visitor: 'curl / 通常 API クライアント', status: 'pass', outcomeText: '通過', reason: 'レベル 2 では汎用 CLI クライアントを許可' },
        { visitor: 'Spamhaus / IPsum 脅威 IP', status: 'challenge', outcomeText: 'PoW (または 403)', reason: 'グローバル脅威インテルは別途評価' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 ブロック', reason: '組み込み default_rules ルール' }
      ]
    },
    {
      key: '1',
      label: '1 — 不審のみ',
      directive: 'verify 1',
      summary: 'スキャナー、ヘッドレスブラウザ、偽装ヘッダー、VPN/プロキシ/Tor IP、悪性 IP のみに PoW を出します。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '通過', reason: '通常のブラウザヘッダーとクリーンな IP' },
        { visitor: '公式 AI クローラー（通常 UA）', status: 'pass', outcomeText: '通過', reason: 'レベル 1 では AI 専用判定は無効' },
        { visitor: 'VPN / プロキシ / Tor 出口 IP', status: 'challenge', outcomeText: 'PoW チャレンジ', reason: 'X4BNet/Tor リストまたは ipquery.io / ipapi.is で検出（HTTP・SSH・Minecraft 共通）' },
        { visitor: 'curl / Scrapy / ヘッダー欠落', status: 'challenge', outcomeText: 'PoW チャレンジ', reason: '自動化・ヘッダー不足・不審な SSH/MC クライアント' },
        { visitor: 'Spamhaus / IPsum 脅威 IP', status: 'challenge', outcomeText: 'PoW (または 403)', reason: '脅威フィード一致' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 ブロック', reason: '組み込み default_rules ルール' }
      ]
    },
    {
      key: '0',
      label: '0 — 常に検証',
      directive: 'verify 0',
      summary: 'クリアランスのない全訪問者・TCP クライアントに SHA-256 PoW を 1 回求めます。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'challenge', outcomeText: 'PoW チャレンジ', reason: 'クリアランス有効期間ごとに 1 回検証' },
        { visitor: 'GPTBot / ClaudeBot / Applebot', status: 'challenge', outcomeText: 'PoW チャレンジ', reason: '全トラフィック対象' },
        { visitor: 'VPN / プロキシ / Tor 出口 IP', status: 'challenge', outcomeText: 'PoW チャレンジ', reason: '全トラフィック対象' },
        { visitor: 'curl / python-requests', status: 'challenge', outcomeText: 'PoW チャレンジ', reason: '全トラフィック対象' },
        { visitor: 'Spamhaus / IPsum 脅威 IP', status: 'challenge', outcomeText: 'PoW (または 403)', reason: '脅威フィード一致' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 ブロック', reason: '組み込み default_rules ルール' }
      ]
    },
    {
      key: 'off',
      label: 'protect off',
      directive: 'protect off',
      summary: '静的ファイル・Webhook・ミラー向けに PoW を無効化し、HTTPS とカスタムルールだけ動かします。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '通過', reason: '通常の HTTPS & WebSocket プロキシ' },
        { visitor: 'GPTBot / ClaudeBot', status: 'pass', outcomeText: '通過', reason: 'WAF 検証オフ' },
        { visitor: 'VPN / プロキシ / Tor 出口 IP', status: 'pass', outcomeText: '通過', reason: 'WAF 検証オフ' },
        { visitor: 'curl / Webhook クライアント', status: 'pass', outcomeText: '通過', reason: 'WAF 検証オフ' },
        { visitor: 'カスタムルール（設定時）', status: 'block', outcomeText: '評価実行', reason: '明示的な allow/block ルールは有効' },
        { visitor: 'TLS & HTTP->HTTPS リダイレクト', status: 'pass', outcomeText: '有効', reason: 'ACME とローカル PKI は有効' }
      ]
    }
  ],
  ko: [
    {
      key: '3',
      label: '3 — 의심 & AI (기본값)',
      directive: 'verify 3',
      summary: '의심스러운 클라이언트/IP(VPN·프록시·Tor IP 포함)와 AI 크롤러에는 PoW를 요구하고 일반 브라우저는 통과시킵니다.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '통과', reason: '표준 브라우저 헤더 및 정상 IP' },
        { visitor: 'GPTBot / ClaudeBot / Applebot', status: 'challenge', outcomeText: 'PoW 챌린지', reason: '공식 AI IP 대역 또는 AI UA 일치' },
        { visitor: 'VPN / 프록시 / Tor 출구 IP', status: 'challenge', outcomeText: 'PoW 챌린지', reason: 'X4BNet/Tor 목록 또는 ipquery.io / ipapi.is 실시간 조회에서 탐지 (HTTP, SSH, Minecraft 공통)' },
        { visitor: 'curl / python-requests / Headless', status: 'challenge', outcomeText: 'PoW 챌린지', reason: '자동화 도구, 헤더 누락 또는 의심 SSH/MC 클라이언트' },
        { visitor: 'Spamhaus / IPsum 위협 IP', status: 'challenge', outcomeText: 'PoW (또는 403)', reason: '위협 피드 일치 (기본 verify, block 설정 시 403)' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 차단', reason: '내장 default_rules 기본 규칙' }
      ]
    },
    {
      key: '2',
      label: '2 — AI 만',
      directive: 'verify 2',
      summary: '일반 사용자·curl·VPN 접속은 통과시키고 AI 크롤러와 LLM 에이전트만 PoW를 풉니다.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '통과', reason: 'AI 크롤러 아님' },
        { visitor: 'GPTBot / ClaudeBot / PerplexityBot', status: 'challenge', outcomeText: 'PoW 챌린지', reason: 'AI 크롤러 CIDR 또는 User-Agent 일치' },
        { visitor: 'VPN / 프록시 / Tor 출구 IP', status: 'pass', outcomeText: '통과', reason: '레벨 2에서는 VPN/의심 검사를 건너뜁니다' },
        { visitor: 'curl / 일반 API 클라이언트', status: 'pass', outcomeText: '통과', reason: '레벨 2에서는 일반 CLI 클라이언트 허용' },
        { visitor: 'Spamhaus / IPsum 위협 IP', status: 'challenge', outcomeText: 'PoW (또는 403)', reason: '전역 IP 위협 인텔은 독립 동작' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 차단', reason: '내장 default_rules 기본 규칙' }
      ]
    },
    {
      key: '1',
      label: '1 — 의심 트래픽만',
      directive: 'verify 1',
      summary: '스캐너, 헤드리스 브라우저, 위조된 헤더, VPN/프록시/Tor IP, 악성 IP에만 챌린지를 겁니다.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '통과', reason: '정상 브라우저 지문과 깨끗한 IP' },
        { visitor: '공식 AI 크롤러 (정상 UA)', status: 'pass', outcomeText: '통과', reason: '레벨 1에서는 AI 전용 트리거 비활성화' },
        { visitor: 'VPN / 프록시 / Tor 출구 IP', status: 'challenge', outcomeText: 'PoW 챌린지', reason: 'X4BNet/Tor 목록 또는 ipquery.io / ipapi.is 실시간 조회에서 탐지 (HTTP, SSH, Minecraft 공통)' },
        { visitor: 'curl / Scrapy / 헤더 누락', status: 'challenge', outcomeText: 'PoW 챌린지', reason: '자동화 도구, 브라우저 헤더 누락 또는 의심 SSH/MC 클라이언트' },
        { visitor: 'Spamhaus / IPsum 위협 IP', status: 'challenge', outcomeText: 'PoW (또는 403)', reason: '위협 피드 일치' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 차단', reason: '내장 default_rules 기본 규칙' }
      ]
    },
    {
      key: '0',
      label: '0 — 항상 검증',
      directive: 'verify 0',
      summary: '유효한 통과 기록이 없는 모든 방문자와 TCP 클라이언트에 SHA-256 PoW를 요구합니다.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'challenge', outcomeText: 'PoW 챌린지', reason: '클리어런스 TTL마다 한 번 검증' },
        { visitor: 'GPTBot / ClaudeBot / Applebot', status: 'challenge', outcomeText: 'PoW 챌린지', reason: '모든 트래픽 검증' },
        { visitor: 'VPN / 프록시 / Tor 출구 IP', status: 'challenge', outcomeText: 'PoW 챌린지', reason: '모든 트래픽 검증' },
        { visitor: 'curl / python-requests', status: 'challenge', outcomeText: 'PoW 챌린지', reason: '모든 트래픽 검증' },
        { visitor: 'Spamhaus / IPsum 위협 IP', status: 'challenge', outcomeText: 'PoW (또는 403)', reason: '위협 피드 일치' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 차단', reason: '내장 default_rules 기본 규칙' }
      ]
    },
    {
      key: 'off',
      label: 'protect off',
      directive: 'protect off',
      summary: '정적 파일, 웹훅, 미러를 위해 PoW 챌린지를 끄고 HTTPS와 사용자 규칙만 유지합니다.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '통과', reason: '일반 HTTPS & WebSocket 프록시' },
        { visitor: 'GPTBot / ClaudeBot', status: 'pass', outcomeText: '통과', reason: 'WAF 검증 비활성화' },
        { visitor: 'VPN / 프록시 / Tor 출구 IP', status: 'pass', outcomeText: '통과', reason: 'WAF 검증 비활성화' },
        { visitor: 'curl / 웹훅 클라이언트', status: 'pass', outcomeText: '통과', reason: 'WAF 검증 비활성화' },
        { visitor: '사용자 지정 규칙 (설정 시)', status: 'block', outcomeText: '평가됨', reason: '명시적 allow/block 규칙은 계속 적용' },
        { visitor: 'TLS & HTTP->HTTPS 리디렉션', status: 'pass', outcomeText: '활성', reason: 'ACME 및 로컬 PKI 유지' }
      ]
    }
  ],
  es: [
    {
      key: '3',
      label: '3 — sospechoso e IA (Predeterminado)',
      directive: 'verify 3',
      summary:
        'Desafía a clientes/IPs sospechosos (incluidas IPs de VPN/proxy/Tor) y rastreadores de IA; los navegadores normales pasan directo.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: 'Pasa directo', reason: 'Cabeceras de navegador normales e IP limpia' },
        { visitor: 'GPTBot / ClaudeBot / Applebot', status: 'challenge', outcomeText: 'Desafío PoW', reason: 'Coincide con rango IP o UA oficial de IA' },
        { visitor: 'IP de VPN / Proxy / Tor', status: 'challenge', outcomeText: 'Desafío PoW', reason: 'Detectada en listas X4BNet/Tor o consulta ipquery.io / ipapi.is (HTTP, SSH y Minecraft)' },
        { visitor: 'curl / python-requests / Headless', status: 'challenge', outcomeText: 'Desafío PoW', reason: 'Automatización, cabeceras faltantes o cliente SSH/MC sospechoso' },
        { visitor: 'IP de amenaza Spamhaus / IPsum', status: 'challenge', outcomeText: 'PoW (o 403)', reason: 'Coincidencia en listas de amenazas (verify o block)' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 Bloqueado', reason: 'Reglas base integradas default_rules' }
      ]
    },
    {
      key: '2',
      label: '2 — solo IA',
      directive: 'verify 2',
      summary:
        'Deja pasar a navegadores, curl y usuarios de VPN; solo los rastreadores de IA y agentes LLM resuelven el PoW.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: 'Pasa directo', reason: 'No es un rastreador de IA' },
        { visitor: 'GPTBot / ClaudeBot / PerplexityBot', status: 'challenge', outcomeText: 'Desafío PoW', reason: 'Coincide con CIDR o User-Agent de IA' },
        { visitor: 'IP de VPN / Proxy / Tor', status: 'pass', outcomeText: 'Pasa directo', reason: 'Las comprobaciones de VPN/sospecha están apagadas en nivel 2' },
        { visitor: 'curl / clientes API normales', status: 'pass', outcomeText: 'Pasa directo', reason: 'Clientes CLI genéricos permitidos en nivel 2' },
        { visitor: 'IP de amenaza Spamhaus / IPsum', status: 'challenge', outcomeText: 'PoW (o 403)', reason: 'La inteligencia de IPs sigue activa' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 Bloqueado', reason: 'Reglas base integradas default_rules' }
      ]
    },
    {
      key: '1',
      label: '1 — solo sospechoso',
      directive: 'verify 1',
      summary:
        'Desafía escáneres, navegadores headless, cabeceras falsificadas, IPs de VPN/proxy/Tor e IPs maliciosas sin molestar a navegadores normales.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: 'Pasa directo', reason: 'Huella normal de navegador' },
        { visitor: 'Rastreador IA oficial (UA limpio)', status: 'pass', outcomeText: 'Pasa directo', reason: 'Reglas exclusivas de IA desactivadas en nivel 1' },
        { visitor: 'IP de VPN / Proxy / Tor', status: 'challenge', outcomeText: 'Desafío PoW', reason: 'Detectada en listas X4BNet/Tor o consulta ipquery.io / ipapi.is (HTTP, SSH y Minecraft)' },
        { visitor: 'curl / Scrapy / Sin cabeceras', status: 'challenge', outcomeText: 'Desafío PoW', reason: 'Automatización, falta de cabeceras o cliente SSH/MC sospechoso' },
        { visitor: 'IP de amenaza Spamhaus / IPsum', status: 'challenge', outcomeText: 'PoW (o 403)', reason: 'Coincidencia en listas de amenazas' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 Bloqueado', reason: 'Reglas base integradas default_rules' }
      ]
    },
    {
      key: '0',
      label: '0 — siempre',
      directive: 'verify 0',
      summary:
        'Exige resolver un desafío SHA-256 PoW a cualquier visitante o IP TCP sin autorización activa.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'challenge', outcomeText: 'Desafío PoW', reason: 'Cada visitante se verifica una vez por TTL' },
        { visitor: 'GPTBot / ClaudeBot / Applebot', status: 'challenge', outcomeText: 'Desafío PoW', reason: 'Todo el tráfico es desafiado' },
        { visitor: 'IP de VPN / Proxy / Tor', status: 'challenge', outcomeText: 'Desafío PoW', reason: 'Todo el tráfico es desafiado' },
        { visitor: 'curl / python-requests', status: 'challenge', outcomeText: 'Desafío PoW', reason: 'Todo el tráfico es desafiado' },
        { visitor: 'IP de amenaza Spamhaus / IPsum', status: 'challenge', outcomeText: 'PoW (o 403)', reason: 'Coincidencia en listas de amenazas' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 Bloqueado', reason: 'Reglas base integradas default_rules' }
      ]
    },
    {
      key: 'off',
      label: 'protect off',
      directive: 'protect off',
      summary:
        'Desactiva los desafíos PoW para estáticos, webhooks o mirrors manteniendo activos HTTPS y reglas personalizadas.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: 'Pasa directo', reason: 'Proxy inverso HTTPS y WebSocket directo' },
        { visitor: 'GPTBot / ClaudeBot', status: 'pass', outcomeText: 'Pasa directo', reason: 'Verificación WAF desactivada' },
        { visitor: 'IP de VPN / Proxy / Tor', status: 'pass', outcomeText: 'Pasa directo', reason: 'Verificación WAF desactivada' },
        { visitor: 'curl / clientes webhook', status: 'pass', outcomeText: 'Pasa directo', reason: 'Verificación WAF desactivada' },
        { visitor: 'Reglas personalizadas del sitio', status: 'block', outcomeText: 'Evaluado', reason: 'Las reglas explícitas allow/block siguen activas' },
        { visitor: 'TLS y redirección HTTP->HTTPS', status: 'pass', outcomeText: 'Activo', reason: 'ACME y PKI local siguen activos' }
      ]
    }
  ]
}

const headers = computed(() => headersByLocale[activeLocale.value])
const levels = computed(() => levelsByLocale[activeLocale.value])
const selected = ref<LevelKey>('3')
const current = computed(() => levels.value.find((l) => l.key === selected.value)!)
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
            <th>{{ headers.client }}</th>
            <th>{{ headers.action }}</th>
            <th>{{ headers.why }}</th>
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
