<script setup lang="ts">
import { computed, ref } from 'vue'
import { useData } from 'vitepress'

const { lang } = useData()

type LocaleKey = 'en' | 'zh' | 'ja' | 'ko' | 'es'
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
  const code = (lang.value || 'en').slice(0, 2).toLowerCase()
  if (code === 'zh' || code === 'ja' || code === 'ko' || code === 'es') {
    return code
  }
  return 'en'
})

const headersByLocale: Record<LocaleKey, { client: string; action: string; why: string }> = {
  en: { client: 'Client / Traffic', action: 'Action', why: 'Why' },
  zh: { client: '客户端 / 流量类型', action: '处理动作', why: '判定原因' },
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
  ],
  zh: [
    {
      key: '3',
      label: '3 — 可疑流量与 AI（默认）',
      directive: 'verify 3',
      summary: '同时拦截/质询可疑客户端 IP 与 AI 爬虫，普通人类浏览器直接放行。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '直接放行', reason: '标准浏览器请求头且 IP 信誉良好' },
        { visitor: 'GPTBot / ClaudeBot / Applebot', status: 'challenge', outcomeText: 'PoW 质询', reason: '命中官方 AI IP 段或 AI User-Agent' },
        { visitor: 'curl / python-requests / 无头浏览器', status: 'challenge', outcomeText: 'PoW 质询', reason: '内置自动化客户端与请求头启发式检测' },
        { visitor: 'Spamhaus / IPsum 威胁 IP', status: 'challenge', outcomeText: 'PoW（或 403）', reason: '威胁情报动作（默认 verify，可设为 block）' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 拦截', reason: '内置 default_rules 基础防护规则' }
      ]
    },
    {
      key: '2',
      label: '2 — 仅 AI 爬虫',
      directive: 'verify 2',
      summary: '人类访客与 curl/API 工具零阻力通过；仅 AI 爬虫与 LLM Agent 需要完成 PoW 计算。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '直接放行', reason: '未归类为 AI 爬虫' },
        { visitor: 'GPTBot / ClaudeBot / PerplexityBot', status: 'challenge', outcomeText: 'PoW 质询', reason: '命中 AI 爬虫 CIDR 或 User-Agent' },
        { visitor: 'curl / 常规 API 客户端', status: 'pass', outcomeText: '直接放行', reason: '级别 2 放行常规 CLI 客户端' },
        { visitor: 'Spamhaus / IPsum 威胁 IP', status: 'challenge', outcomeText: 'PoW（或 403）', reason: '全局 IP 威胁情报独立运行' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 拦截', reason: '内置 default_rules 基础防护规则' }
      ]
    },
    {
      key: '1',
      label: '1 — 仅可疑流量',
      directive: 'verify 1',
      summary: '仅质询扫描器、无头浏览器、伪造请求头及恶意 IP，正常浏览器不受影响。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '直接放行', reason: '正常浏览器指纹' },
        { visitor: '官方 AI 爬虫（常规 UA）', status: 'pass', outcomeText: '直接放行', reason: '级别 1 不触发纯 AI 规则' },
        { visitor: 'curl / Scrapy / 缺失请求头', status: 'challenge', outcomeText: 'PoW 质询', reason: '可疑自动化或缺失浏览器请求头' },
        { visitor: 'Spamhaus / IPsum 威胁 IP', status: 'challenge', outcomeText: 'PoW（或 403）', reason: '命中威胁情报库' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 拦截', reason: '内置 default_rules 基础防护规则' }
      ]
    },
    {
      key: '0',
      label: '0 — 始终验证',
      directive: 'verify 0',
      summary: '要求所有未持有有效 HMAC waf_clearance Cookie 的访客先完成 SHA-256 PoW 验证。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'challenge', outcomeText: 'PoW 质询', reason: '每个访客在有效期内需完成一次验证' },
        { visitor: 'GPTBot / ClaudeBot / Applebot', status: 'challenge', outcomeText: 'PoW 质询', reason: '全部流量均需验证' },
        { visitor: 'curl / python-requests', status: 'challenge', outcomeText: 'PoW 质询', reason: '全部流量均需验证' },
        { visitor: 'Spamhaus / IPsum 威胁 IP', status: 'challenge', outcomeText: 'PoW（或 403）', reason: '命中威胁情报库' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 拦截', reason: '内置 default_rules 基础防护规则' }
      ]
    },
    {
      key: 'off',
      label: 'protect off',
      directive: 'protect off',
      summary: '为静态资源、Webhook 或镜像站关闭 PoW 质询，同时保持 HTTPS 与显式自定义规则生效。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '直接放行', reason: '直通 HTTPS 与 WebSocket 反向代理' },
        { visitor: 'GPTBot / ClaudeBot', status: 'pass', outcomeText: '直接放行', reason: '已关闭 WAF 验证' },
        { visitor: 'curl / Webhook 客户端', status: 'pass', outcomeText: '直接放行', reason: '已关闭 WAF 验证' },
        { visitor: '站点自定义规则（若已配置）', status: 'block', outcomeText: '正常执行', reason: '显式 allow/block 自定义规则仍生效' },
        { visitor: 'TLS 与 HTTP->HTTPS 跳转', status: 'pass', outcomeText: '已启用', reason: 'ACME 与本地 PKI 持续有效' }
      ]
    }
  ],
  ja: [
    {
      key: '3',
      label: '3 — 不審 & AI（デフォルト）',
      directive: 'verify 3',
      summary: '不審なクライアント/IP と AI クローラーの両方にチャレンジを課し、通常のブラウザはそのまま通過させます。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '通過', reason: '標準ブラウザヘッダー & クリーンな IP' },
        { visitor: 'GPTBot / ClaudeBot / Applebot', status: 'challenge', outcomeText: 'PoW チャレンジ', reason: '公式 AI IP 範囲または AI UA に一致' },
        { visitor: 'curl / python-requests / Headless', status: 'challenge', outcomeText: 'PoW チャレンジ', reason: '組み込みの自動化・ヘッダー検知' },
        { visitor: 'Spamhaus / IPsum 脅威 IP', status: 'challenge', outcomeText: 'PoW (または 403)', reason: '脅威インテルアクション（既定 verify / block）' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 ブロック', reason: '組み込み default_rules ベースライン保護' }
      ]
    },
    {
      key: '2',
      label: '2 — AI のみ',
      directive: 'verify 2',
      summary: '人間や curl/API ツールには負荷をかけず、AI クローラーと LLM エージェントのみに PoW を要求します。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '通過', reason: 'AI クローラーではない' },
        { visitor: 'GPTBot / ClaudeBot / PerplexityBot', status: 'challenge', outcomeText: 'PoW チャレンジ', reason: 'AI クローラー CIDR または User-Agent に一致' },
        { visitor: 'curl / 通常 API クライアント', status: 'pass', outcomeText: '通過', reason: 'レベル 2 では汎用 CLI クライアントを許可' },
        { visitor: 'Spamhaus / IPsum 脅威 IP', status: 'challenge', outcomeText: 'PoW (または 403)', reason: 'グローバル IP 脅威インテルは独立動作' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 ブロック', reason: '組み込み default_rules ベースライン保護' }
      ]
    },
    {
      key: '1',
      label: '1 — 不審のみ',
      directive: 'verify 1',
      summary: 'スキャナー、ヘッドレスブラウザ、偽装ヘッダー、悪性 IP のみにチャレンジし、通常ブラウザはそのまま通します。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '通過', reason: '通常のブラウザフィンガープリント' },
        { visitor: '公式 AI クローラー（通常 UA）', status: 'pass', outcomeText: '通過', reason: 'レベル 1 では AI 専用トリガーは無効' },
        { visitor: 'curl / Scrapy / ヘッダー欠落', status: 'challenge', outcomeText: 'PoW チャレンジ', reason: '不審な自動化 / ブラウザヘッダー欠如' },
        { visitor: 'Spamhaus / IPsum 脅威 IP', status: 'challenge', outcomeText: 'PoW (または 403)', reason: '脅威インテル一致' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 ブロック', reason: '組み込み default_rules ベースライン保護' }
      ]
    },
    {
      key: '0',
      label: '0 — 常に検証',
      directive: 'verify 0',
      summary: '有効な HMAC waf_clearance Cookie を持たないすべての訪問者に SHA-256 PoW の解決を求めます。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'challenge', outcomeText: 'PoW チャレンジ', reason: 'クリアランス有効期間ごとに1回検証' },
        { visitor: 'GPTBot / ClaudeBot / Applebot', status: 'challenge', outcomeText: 'PoW チャレンジ', reason: 'すべてのトラフィックを検証' },
        { visitor: 'curl / python-requests', status: 'challenge', outcomeText: 'PoW チャレンジ', reason: 'すべてのトラフィックを検証' },
        { visitor: 'Spamhaus / IPsum 脅威 IP', status: 'challenge', outcomeText: 'PoW (または 403)', reason: '脅威インテル一致' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 ブロック', reason: '組み込み default_rules ベースライン保護' }
      ]
    },
    {
      key: 'off',
      label: 'protect off',
      directive: 'protect off',
      summary: '静的アセット・Webhook・ミラー向けに PoW を無効化し、HTTPS と明示的なカスタムルールのみ維持します。',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '通過', reason: 'ダイレクト HTTPS & WebSocket リバースプロキシ' },
        { visitor: 'GPTBot / ClaudeBot', status: 'pass', outcomeText: '通過', reason: 'WAF 検証無効' },
        { visitor: 'curl / Webhook クライアント', status: 'pass', outcomeText: '通過', reason: 'WAF 検証無効' },
        { visitor: 'カスタムルール（設定時）', status: 'block', outcomeText: '評価実行', reason: '明示的な allow/block ルールは引き続き適用' },
        { visitor: 'TLS & HTTP->HTTPS リダイレクト', status: 'pass', outcomeText: '有効', reason: 'ACME とローカル PKI は有効のまま' }
      ]
    }
  ],
  ko: [
    {
      key: '3',
      label: '3 — 의심 & AI (기본값)',
      directive: 'verify 3',
      summary: '의심스러운 클라이언트/IP와 AI 크롤러 모두에 챌린지를 적용하고 일반 브라우저는 그대로 통과시킵니다.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '통과', reason: '표준 브라우저 헤더 및 정상 IP' },
        { visitor: 'GPTBot / ClaudeBot / Applebot', status: 'challenge', outcomeText: 'PoW 챌린지', reason: '공식 AI IP 대역 또는 AI UA 일치' },
        { visitor: 'curl / python-requests / Headless', status: 'challenge', outcomeText: 'PoW 챌린지', reason: '내장 자동화 및 헤더 휴리스틱' },
        { visitor: 'Spamhaus / IPsum 위협 IP', status: 'challenge', outcomeText: 'PoW (또는 403)', reason: '위협 인텔 액션(기본 verify, block 가능)' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 차단', reason: '내장 default_rules 기본 보안 규칙' }
      ]
    },
    {
      key: '2',
      label: '2 — AI 만',
      directive: 'verify 2',
      summary: '일반 사용자와 curl/API 도구에는 마찰 없이 통과시키고 AI 크롤러와 LLM 에이전트만 PoW를 풀도록 합니다.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '통과', reason: 'AI 크롤러로 분류되지 않음' },
        { visitor: 'GPTBot / ClaudeBot / PerplexityBot', status: 'challenge', outcomeText: 'PoW 챌린지', reason: 'AI 크롤러 CIDR 또는 User-Agent 일치' },
        { visitor: 'curl / 일반 API 클라이언트', status: 'pass', outcomeText: '통과', reason: '레벨 2에서는 일반 CLI 클라이언트 허용' },
        { visitor: 'Spamhaus / IPsum 위협 IP', status: 'challenge', outcomeText: 'PoW (또는 403)', reason: '글로벌 IP 위협 인텔 독립 실행' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 차단', reason: '내장 default_rules 기본 보안 규칙' }
      ]
    },
    {
      key: '1',
      label: '1 — 의심 트래픽만',
      directive: 'verify 1',
      summary: '스캐너, 헤드리스 브라우저, 위조된 헤더 및 악성 IP만 검증하고 일반 브라우저는 그대로 통과시킵니다.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '통과', reason: '정상 브라우저 지문' },
        { visitor: '공식 AI 크롤러 (정상 UA)', status: 'pass', outcomeText: '통과', reason: '레벨 1에서는 AI 전용 트리거 비활성화' },
        { visitor: 'curl / Scrapy / 헤더 누락', status: 'challenge', outcomeText: 'PoW 챌린지', reason: '의심 자동화 / 브라우저 헤더 누락' },
        { visitor: 'Spamhaus / IPsum 위협 IP', status: 'challenge', outcomeText: 'PoW (또는 403)', reason: '위협 인텔 일치' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 차단', reason: '내장 default_rules 기본 보안 규칙' }
      ]
    },
    {
      key: '0',
      label: '0 — 항상 검증',
      directive: 'verify 0',
      summary: '유효한 HMAC waf_clearance 쿠키가 없는 모든 방문자에게 SHA-256 PoW 퍼즐 풀이를 요구합니다.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'challenge', outcomeText: 'PoW 챌린지', reason: '클리어런스 TTL마다 한 번 검증' },
        { visitor: 'GPTBot / ClaudeBot / Applebot', status: 'challenge', outcomeText: 'PoW 챌린지', reason: '모든 트래픽 검증' },
        { visitor: 'curl / python-requests', status: 'challenge', outcomeText: 'PoW 챌린지', reason: '모든 트래픽 검증' },
        { visitor: 'Spamhaus / IPsum 위협 IP', status: 'challenge', outcomeText: 'PoW (또는 403)', reason: '위협 인텔 일치' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 차단', reason: '내장 default_rules 기본 보안 규칙' }
      ]
    },
    {
      key: 'off',
      label: 'protect off',
      directive: 'protect off',
      summary: '정적 자산, 웹훅, 미러를 위해 PoW 챌린지를 끄고 HTTPS 및 명시적 사용자 지정 규칙만 유지합니다.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: '통과', reason: '직접 HTTPS & WebSocket 리버스 프록시' },
        { visitor: 'GPTBot / ClaudeBot', status: 'pass', outcomeText: '통과', reason: 'WAF 검증 비활성화' },
        { visitor: 'curl / 웹훅 클라이언트', status: 'pass', outcomeText: '통과', reason: 'WAF 검증 비활성화' },
        { visitor: '사용자 지정 규칙(설정 시)', status: 'block', outcomeText: '평가됨', reason: '명시적 allow/block 규칙은 계속 적용' },
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
        'Desafía tanto a clientes/IPs sospechosos como a rastreadores de IA mientras deja pasar directamente a los navegadores humanos.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: 'Pasa directo', reason: 'Cabeceras de navegador estándar e IP limpia' },
        { visitor: 'GPTBot / ClaudeBot / Applebot', status: 'challenge', outcomeText: 'Desafío PoW', reason: 'Coincide con rango IP o UA oficial de IA' },
        { visitor: 'curl / python-requests / Headless', status: 'challenge', outcomeText: 'Desafío PoW', reason: 'Heurísticas integradas de automatización' },
        { visitor: 'IP de amenaza Spamhaus / IPsum', status: 'challenge', outcomeText: 'PoW (o 403)', reason: 'Acción de inteligencia de amenazas (verify o block)' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 Bloqueado', reason: 'Reglas base integradas default_rules' }
      ]
    },
    {
      key: '2',
      label: '2 — solo IA',
      directive: 'verify 2',
      summary:
        'Cero fricción para humanos y herramientas curl/API; solo los rastreadores de IA y agentes LLM deben resolver PoW.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: 'Pasa directo', reason: 'No clasificado como rastreador de IA' },
        { visitor: 'GPTBot / ClaudeBot / PerplexityBot', status: 'challenge', outcomeText: 'Desafío PoW', reason: 'Coincide con CIDR o User-Agent de IA' },
        { visitor: 'curl / clientes API normales', status: 'pass', outcomeText: 'Pasa directo', reason: 'Clientes CLI genéricos permitidos en nivel 2' },
        { visitor: 'IP de amenaza Spamhaus / IPsum', status: 'challenge', outcomeText: 'PoW (o 403)', reason: 'La inteligencia de IPs opera de forma independiente' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 Bloqueado', reason: 'Reglas base integradas default_rules' }
      ]
    },
    {
      key: '1',
      label: '1 — solo sospechoso',
      directive: 'verify 1',
      summary:
        'Desafía escáneres, navegadores headless, cabeceras falsificadas e IPs maliciosas sin afectar a navegadores normales.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: 'Pasa directo', reason: 'Huella normal de navegador' },
        { visitor: 'Rastreador IA oficial (UA limpio)', status: 'pass', outcomeText: 'Pasa directo', reason: 'Disparadores exclusivos de IA desactivados en nivel 1' },
        { visitor: 'curl / Scrapy / Sin cabeceras', status: 'challenge', outcomeText: 'Desafío PoW', reason: 'Automatización sospechosa / faltan cabeceras' },
        { visitor: 'IP de amenaza Spamhaus / IPsum', status: 'challenge', outcomeText: 'PoW (o 403)', reason: 'Coincidencia en listas de amenazas' },
        { visitor: 'GET /.env, /wp-admin, sqlmap', status: 'block', outcomeText: '403 Bloqueado', reason: 'Reglas base integradas default_rules' }
      ]
    },
    {
      key: '0',
      label: '0 — siempre',
      directive: 'verify 0',
      summary:
        'Exige a cada visitante sin una cookie HMAC waf_clearance válida resolver un acertijo SHA-256 PoW.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'challenge', outcomeText: 'Desafío PoW', reason: 'Cada visitante se verifica una vez por TTL' },
        { visitor: 'GPTBot / ClaudeBot / Applebot', status: 'challenge', outcomeText: 'Desafío PoW', reason: 'Todo el tráfico es desafiado' },
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
        'Desactiva los desafíos PoW para estáticos, webhooks o mirrors manteniendo activos HTTPS y reglas explícitas.',
      outcomes: [
        { visitor: 'Chrome / Firefox / Safari', status: 'pass', outcomeText: 'Pasa directo', reason: 'Proxy inverso directo HTTPS y WebSocket' },
        { visitor: 'GPTBot / ClaudeBot', status: 'pass', outcomeText: 'Pasa directo', reason: 'Verificación WAF desactivada' },
        { visitor: 'curl / clientes webhook', status: 'pass', outcomeText: 'Pasa directo', reason: 'Verificación WAF desactivada' },
        { visitor: 'Reglas personalizadas del sitio', status: 'block', outcomeText: 'Evaluado', reason: 'Reglas explícitas allow/block siguen activas' },
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
