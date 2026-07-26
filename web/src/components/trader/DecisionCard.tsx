import { useMemo, useState, useEffect, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import {
  Lock,
  Sparkles,
  CheckCircle2,
  MessageCircle,
  Copy,
  Maximize2,
  Minimize2,
  ChevronDown,
} from 'lucide-react'
import type { DecisionRecord, DecisionAction } from '../../types'
import { t, type Language } from '../../i18n/translations'
import { notify } from '../../lib/notify'

interface DecisionCardProps {
  decision: DecisionRecord
  language: Language
  onSymbolClick?: (symbol: string) => void
}

/** 萤火黄绿 */
const FIREFLY = '#d4ff33'
const FIREFLY_BG = 'rgba(212, 255, 51, 0.12)'
const LEVERAGE_BLUE = '#60a5fa'
const CONFIDENCE_SKY = '#93c5fd'
const FOLLOW_THOUGHT_HIDDEN_ZH = '该策略作者暂未公开思考过程'
const FOLLOW_THOUGHT_HIDDEN_EN = 'The strategy author has not made the thought process public.'
const DAXINGXING_MASTER_SOURCE_STRATEGY_ID = 'bc34778b-c102-4753-9cd5-5d86654a23a4'

const ACTION_STYLE: Record<
  string,
  { color: string; bg: string; labelZh: string; labelEn: string }
> = {
  open_long: { color: '#0ECB81', bg: 'rgba(14, 203, 129, 0.18)', labelZh: '开多', labelEn: 'LONG' },
  open_short: { color: '#F6465D', bg: 'rgba(246, 70, 93, 0.18)', labelZh: '开空', labelEn: 'SHORT' },
  close_long: { color: FIREFLY, bg: FIREFLY_BG, labelZh: '平多', labelEn: 'CLOSE' },
  close_short: { color: FIREFLY, bg: FIREFLY_BG, labelZh: '平空', labelEn: 'CLOSE' },
  hold: { color: '#848E9C', bg: 'rgba(132, 142, 156, 0.2)', labelZh: '观望', labelEn: 'HOLD' },
  wait: { color: '#EAECEF', bg: 'rgba(55, 60, 68, 0.95)', labelZh: '等待', labelEn: 'WAIT' },
}

function getBaseAsset(symbol: string): string {
  return symbol
    .replace(/USDT$/i, '')
    .replace(/USDC$/i, '')
    .replace(/BUSD$/i, '')
    .replace(/PERP$/i, '')
    .replace(/_\w+$/i, '') // e.g. BTCUSDT_SPOT
}

function formatPrice(price: number | undefined): string {
  if (!price || price === 0) return '-'
  if (price >= 1000) return price.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })
  if (price >= 1) return price.toFixed(4)
  return price.toFixed(6)
}

function formatPriceWithUnit(price: number | undefined, language: Language): string {
  const p = formatPrice(price)
  if (p === '-') return '-'
  return language === 'zh' ? `${p} 美元` : p
}

function formatRelativeTime(iso: string, language: Language): string {
  const ms = Date.now() - new Date(iso).getTime()
  const sec = Math.max(0, Math.floor(ms / 1000))
  if (language === 'zh') {
    if (sec < 60) return '刚刚'
    if (sec < 3600) return `${Math.floor(sec / 60)} 分钟前`
    if (sec < 86400) return `${Math.floor(sec / 3600)} 小时前`
    if (sec < 86400 * 7) return `${Math.floor(sec / 86400)} 天前`
    return `${Math.floor(sec / 86400)} 天前`
  }
  if (sec < 60) return 'just now'
  if (sec < 3600) return `${Math.floor(sec / 60)}m ago`
  if (sec < 86400) return `${Math.floor(sec / 3600)}h ago`
  if (sec < 86400 * 7) return `${Math.floor(sec / 86400)}d ago`
  return `${Math.floor(sec / 86400)}d ago`
}

/**
 * 数据库里 `decisions` 常为空（例如未执行到下单循环），但 `decision_json` 里仍有 AI 输出的计划。
 * 解析后用于展示币对、状态、置信度等。
 */
function decisionsFromDecisionJson(decision: DecisionRecord): DecisionAction[] {
  const raw = decision.decision_json?.trim()
  if (!raw) return []
  try {
    const arr = JSON.parse(raw) as Record<string, unknown>[]
    if (!Array.isArray(arr)) return []
    return arr.map((item) => {
      const qtyRaw = item.quantity ?? item.position_size_usd
      const qty =
        typeof qtyRaw === 'number' && !Number.isNaN(qtyRaw) ? qtyRaw : Number(qtyRaw) || 0
      const lev = item.leverage
      const leverage =
        typeof lev === 'number' && !Number.isNaN(lev) ? Math.max(1, Math.round(lev)) : 1
      const sl = item.stop_loss
      const tp = item.take_profit
      const conf = item.confidence
      return {
        action: String(item.action ?? 'wait'),
        symbol: String(item.symbol ?? ''),
        quantity: qty,
        leverage,
        price: typeof item.price === 'number' ? item.price : Number(item.price) || 0,
        stop_loss: typeof sl === 'number' && !Number.isNaN(sl) ? sl : undefined,
        take_profit: typeof tp === 'number' && !Number.isNaN(tp) ? tp : undefined,
        confidence: typeof conf === 'number' && !Number.isNaN(conf) ? conf : Number(conf) || 0,
        reasoning: String(item.reasoning ?? ''),
        order_id: String(item.order_id ?? ''),
        timestamp: decision.timestamp,
        success: decision.success,
        error: '',
      } as DecisionAction
    })
  } catch {
    return []
  }
}

function getEffectiveDecisions(decision: DecisionRecord): DecisionAction[] {
  if (decision.decisions && decision.decisions.length > 0) {
    return decision.decisions
  }
  return decisionsFromDecisionJson(decision)
}

function isComkunFollowRecord(decision: DecisionRecord): boolean {
  const systemPrompt = String(decision.system_prompt || '').trim()
  return (
    systemPrompt === 'COMKUN_AI_STRATEGY_ANALYSIS' ||
    systemPrompt === 'COMKUN_AI_STRATEGY_NOTICE' ||
    systemPrompt === 'COMKUN_MIRROR_MASTER_AI' ||
    systemPrompt === 'COMKUN_MASTER_AI_NO_STATE' ||
    systemPrompt === 'COMKUN_MIRROR_FAILED_NON_TRANSIENT' ||
    systemPrompt === 'COMKUN_FOLLOW_BALANCE_STOP' ||
    systemPrompt === 'COMKUN_FOLLOW_SUBSCRIPTION_EXPIRED' ||
    systemPrompt === 'COMKUN_MIRROR_FOLLOW' ||
    systemPrompt === 'COMKUN_COMPLIANT_FOLLOW_MODE' ||
    systemPrompt === 'COMKUN_FOLLOW_INFO_ONLY'
  )
}

function isPublicComkunAIAnalysisRecord(decision: DecisionRecord): boolean {
  const systemPrompt = String(decision.system_prompt || '').trim()
  return (
    systemPrompt === 'COMKUN_AI_STRATEGY_ANALYSIS' ||
    systemPrompt === 'COMKUN_MIRROR_MASTER_AI' ||
    systemPrompt === 'COMKUN_MASTER_AI_NO_STATE'
  )
}

function decisionRawMeta(decision: DecisionRecord): Record<string, unknown> {
  const raw = String(decision.raw_response || '').trim()
  if (!raw) return {}
  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>
    return parsed && typeof parsed === 'object' ? parsed : {}
  } catch {
    return {}
  }
}

function isDaxingxingComkunFollowRecord(decision: DecisionRecord): boolean {
  const sourceID = String(decisionRawMeta(decision).source_strategy_id || '').trim()
  return (
    isPublicComkunAIAnalysisRecord(decision) &&
    (sourceID === DAXINGXING_MASTER_SOURCE_STRATEGY_ID ||
      String(decisionRawMeta(decision).kind || '') === 'ai_strategy_analysis')
  )
}

function stripSectionByHeading(text: string, headingPattern: RegExp): string {
  const lines = String(text || '').split(/\r?\n/)
  const out: string[] = []
  let skipping = false
  for (const line of lines) {
    const trimmed = line.trim()
    const isHeading = /^#{1,6}\s+/.test(trimmed)
    if (headingPattern.test(trimmed)) {
      skipping = true
      continue
    }
    if (skipping && isHeading) {
      skipping = false
    }
    if (!skipping) out.push(line)
  }
  return out.join('\n').trim()
}

function filterDaxingxingMasterThought(text: string): string {
  let cleaned = String(text || '').trim()
  if (!cleaned) return ''
  if (/^\s*\[?主控实盘仓位变化\s*[·\-—]?\s*AI\s*解读\]?/i.test(cleaned)) {
    return ''
  }
  cleaned = stripSectionByHeading(cleaned, /跟单端简报|(?:^|[#\s、.．0-9一二三四五六七八九十]+)仓位管理评估/i)
  return cleaned
    .replace(/^\s*\[?主控实盘仓位变化\s*[·\-—]?\s*AI\s*解读\]?\s*/gim, '')
    .replace(/^.*主控最新交易所快照已确认无持仓.*$/gim, '')
    .replace(/^.*交易所快照同步.*未等待 AI 长分析完成.*$/gim, '')
    .trim()
}

function isComkunBalanceStopRecord(decision: DecisionRecord): boolean {
  return String(decision.system_prompt || '').trim() === 'COMKUN_FOLLOW_BALANCE_STOP'
}

function sanitizeInternalSyncText(text: string): string {
  return String(text || '')
    .replace(/comkun\s*跟单[:：]?\s*/gi, '')
    .replace(/COMKUN[-_ ]?AI[-_ ]?STRATEGY[-_ ]?(ANALYSIS|NOTICE)/gi, '')
    .replace(/COMKUN[-_ ]?MIRROR[-_ ]?MASTER[-_ ]?AI/gi, '')
    .replace(/COMKUN[-_ ]?MASTER[-_ ]?AI[-_ ]?NO[-_ ]?STATE/gi, '')
    .replace(/COMKUN[-_ ]?MIRROR[-_ ]?FAILED[-_ ]?NON[-_ ]?TRANSIENT/gi, '')
    .replace(/COMKUN[-_ ]?MIRROR[-_ ]?FOLLOW/gi, '')
    .replace(/镜像同步/g, '策略状态')
    .replace(/镜像限价/g, '限价订单')
    .replace(/镜像/g, '策略状态')
    .replace(/主控/g, '')
    .replace(/被控/g, '')
    .replace(/广播/g, '')
    .replace(/跟单/g, '')
    .trim()
}

/** 币种图标：底层大字占位 + 多 CDN 尝试，避免外网图全挂时「像没图标」 */
function CryptoPairIcon({ symbol, size = 28 }: { symbol: string; size?: number }) {
  const baseLower = getBaseAsset(symbol).toLowerCase() || '?'
  const label = getBaseAsset(symbol).toUpperCase().slice(0, 2) || '?'
  const urls = useMemo(
    () => [
      `https://assets.coincap.io/assets/icons/${baseLower}@2x.png`,
      `https://raw.githubusercontent.com/spothq/cryptocurrency-icons/master/128/color/${baseLower}.png`,
    ],
    [baseLower],
  )
  const [urlIdx, setUrlIdx] = useState(0)
  const hasNext = urlIdx < urls.length

  return (
    <div
      className="relative flex shrink-0 items-center justify-center overflow-hidden rounded-full bg-gradient-to-br from-[#3d4450] to-[#1a1d24] font-bold uppercase tracking-tight text-[#c4cf45] ring-1 ring-white/20"
      style={{
        width: size,
        height: size,
        fontSize: Math.max(11, Math.round(size * 0.38)),
      }}
    >
      <span className="relative z-0 select-none opacity-90" aria-hidden>
        {label}
      </span>
      {hasNext ? (
        <img
          src={urls[urlIdx]}
          alt=""
          className="absolute inset-0 z-10 h-full w-full object-cover"
          onError={() => setUrlIdx((i) => i + 1)}
        />
      ) : null}
    </div>
  )
}

/** 本周期真实扫描/决策涉及的币种：只信结构化字段，不再从长文里硬抠币种 */
function scanSymbolsForCycle(decision: DecisionRecord, actions: DecisionAction[]): string[] {
  const set = new Set<string>()
  for (const c of decision.candidate_coins || []) {
    const s = String(c || '').trim().toUpperCase()
    if (s) set.add(s)
  }
  for (const d of actions) {
    const s = String(d.symbol || '').trim().toUpperCase()
    if (s) set.add(s)
  }
  return [...set]
}

function normalizeSymbolKey(s: string): string {
  return s.trim().toUpperCase().replace(/\s+/g, '')
}

function briefSnippet(text: string, maxLen: number): string {
  const t = text.replace(/\s+/g, ' ').trim()
  if (!t.length) return ''
  if (t.length <= maxLen) return t
  return `${t.slice(0, maxLen)}…`
}

/** 从思考链中提取「成交量与 OI」相关一行（排除挂单字段技术长文） */
function extractVolumeOILineFromCot(cot: string): string {
  const raw = cot?.trim() || ''
  if (!raw) return ''
  const lines = raw.split(/\n+/)
  for (const line of lines) {
    const t = line.replace(/\s+/g, ' ').trim()
    if (!t) continue
    if (/成交量\s*与\s*OI|成交量和\s*OI|4h\s*成交量|合约持仓\s*OI|OI\s*增幅榜/i.test(t)) {
      if (/方向\s+[A-Z]{3,4}\s*\||类型\s+LIMIT|挂单列表|持仓侧/i.test(t)) continue
      return briefSnippet(t, 90)
    }
  }
  const m = raw.match(/成交量\s*与\s*OI\s*分析[^\n]*/i)
  if (m) return briefSnippet(m[0].replace(/\s+/g, ' ').trim(), 90)
  return ''
}

/** 从思考链里抠一段包含该交易对的文字，当作「简略说明」兜底 */
function cotSnippetForSymbol(cot: string, sym: string): string {
  if (!cot || !sym) return ''
  const upper = cot.toUpperCase()
  const key = sym.toUpperCase()
  const idx = upper.indexOf(key)
  if (idx < 0) return ''
  const start = Math.max(0, idx - 20)
  return briefSnippet(cot.slice(start, idx + 120).replace(/\s+/g, ' ').trim(), 90)
}

interface SymbolScanRow {
  symbol: string
  confidencePct: number | null
  brief: string
  actionHint: string
}

function actionSetToHint(acts: Set<string>, language: Language): string {
  if (acts.size === 0) return '—'
  const order = ['open_long', 'open_short', 'close_long', 'close_short', 'hold', 'wait']
  for (const key of order) {
    if (acts.has(key)) {
      const cfg = ACTION_STYLE[key]
      if (cfg) return language === 'zh' ? cfg.labelZh : cfg.labelEn
    }
  }
  const first = [...acts][0]
  const cfg = ACTION_STYLE[first] || ACTION_STYLE.wait
  return language === 'zh' ? cfg.labelZh : cfg.labelEn
}

function buildSymbolScanRows(
  scanSymbols: string[],
  actions: DecisionAction[],
  cot: string,
  language: Language,
  systemPrompt?: string,
): SymbolScanRow[] {
  const map = new Map<string, { maxConf: number; reasons: string[]; acts: Set<string> }>()
  for (const a of actions) {
    const k = normalizeSymbolKey(a.symbol || '')
    if (!k) continue
    let ent = map.get(k)
    if (!ent) {
      ent = { maxConf: 0, reasons: [], acts: new Set() }
      map.set(k, ent)
    }
    ent.acts.add(a.action)
    const c =
      a.confidence !== undefined && a.confidence !== null && !Number.isNaN(Number(a.confidence))
        ? Number(a.confidence)
        : 0
    if (c > 0 && c > ent.maxConf) ent.maxConf = c
    if (a.reasoning?.trim()) ent.reasons.push(a.reasoning.trim())
  }

  const rows: SymbolScanRow[] = []
  for (const rawSym of scanSymbols) {
    const sym = rawSym.trim().toUpperCase()
    if (!sym) continue
    const k = normalizeSymbolKey(sym)
    const ent = map.get(k)
    let brief = ''
    if (systemPrompt === 'COMKUN_MIRROR_FOLLOW') {
      const volLines = (ent?.reasons || []).filter((r) => /成交量\s*与\s*OI|成交量和\s*OI/i.test(r))
      if (volLines.length) brief = briefSnippet(volLines.join(' · '), 90)
      if (!brief) brief = extractVolumeOILineFromCot(cot)
      if (!brief) {
        brief = language === 'zh' ? '成交量与 OI 分析：见下方思考过程。' : 'Volume & OI: see thought below.'
      }
    } else {
      if (ent?.reasons.length) brief = briefSnippet(ent.reasons.join(' · '), 90)
      if (!brief) brief = cotSnippetForSymbol(cot, sym)
      if (!brief) {
        brief =
          language === 'zh'
            ? '本轮暂无单独说明，详情见下方思考过程。'
            : 'No short note for this symbol; see thought below.'
      }
    }
    const confPct = ent && ent.maxConf > 0 ? Math.round(ent.maxConf) : null
    rows.push({
      symbol: sym,
      confidencePct: confPct,
      brief,
      actionHint: ent ? actionSetToHint(ent.acts, language) : '—',
    })
  }
  return rows
}

function isMarkdownTableSeparator(line: string): boolean {
  return /^\s*\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)+\|?\s*$/.test(line)
}

function splitMarkdownTableRow(line: string): string[] {
  const trimmed = line.trim().replace(/^\|/, '').replace(/\|$/, '')
  return trimmed.split('|').map((cell) => cell.trim())
}

function renderInlineMarkdown(text: string): ReactNode[] {
  const out: ReactNode[] = []
  const re = /(\*\*[^*]+\*\*)/g
  let last = 0
  let m: RegExpExecArray | null
  while ((m = re.exec(text)) !== null) {
    if (m.index > last) out.push(text.slice(last, m.index))
    const value = m[1].slice(2, -2)
    out.push(
      <strong key={`${m.index}-${value}`} className="font-semibold text-white">
        {value}
      </strong>,
    )
    last = m.index + m[1].length
  }
  if (last < text.length) out.push(text.slice(last))
  return out
}

function ThoughtMarkdown({ body }: { body: string }) {
  const blocks = useMemo(() => {
    const lines = body.replace(/\r\n/g, '\n').split('\n')
    const nodes: ReactNode[] = []
    let i = 0

    while (i < lines.length) {
      const line = lines[i]
      const trimmed = line.trim()
      if (!trimmed) {
        i += 1
        continue
      }

      if (trimmed.startsWith('```')) {
        const code: string[] = []
        i += 1
        while (i < lines.length && !lines[i].trim().startsWith('```')) {
          code.push(lines[i])
          i += 1
        }
        if (i < lines.length) i += 1
        nodes.push(
          <pre
            key={`code-${i}`}
            className="my-3 overflow-x-auto rounded-lg border border-white/10 bg-black/40 p-3 font-mono text-[11px] leading-relaxed text-white"
          >
            {code.join('\n')}
          </pre>,
        )
        continue
      }

      if (trimmed.includes('|') && i + 1 < lines.length && isMarkdownTableSeparator(lines[i + 1])) {
        const headers = splitMarkdownTableRow(trimmed)
        const rows: string[][] = []
        i += 2
        while (i < lines.length && lines[i].trim().includes('|')) {
          rows.push(splitMarkdownTableRow(lines[i]))
          i += 1
        }
        nodes.push(
          <div key={`table-${i}`} className="my-3 overflow-x-auto rounded-lg border border-white/10">
            <table className="min-w-full border-collapse text-left text-[11px] text-white md:text-xs">
              <thead className="bg-white/[0.06]">
                <tr>
                  {headers.map((h, idx) => (
                    <th key={`${h}-${idx}`} className="border-b border-white/10 px-3 py-2 font-semibold text-[#d4ff33]">
                      {renderInlineMarkdown(h)}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {rows.map((row, rIdx) => (
                  <tr key={rIdx} className="border-t border-white/10 odd:bg-white/[0.025]">
                    {headers.map((_, cIdx) => (
                      <td key={cIdx} className="px-3 py-2 align-top text-white">
                        {renderInlineMarkdown(row[cIdx] || '')}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>,
        )
        continue
      }

      const heading = trimmed.match(/^(#{1,4})\s+(.+)$/)
      if (heading) {
        const level = heading[1].length
        const cls =
          level === 1
            ? 'mt-1 mb-3 text-base font-bold text-white md:text-lg'
            : level === 2
              ? 'mt-4 mb-2 text-sm font-bold text-[#d4ff33] md:text-base'
              : 'mt-3 mb-1.5 text-xs font-semibold text-white md:text-sm'
        nodes.push(
          <div key={`h-${i}`} className={cls}>
            {renderInlineMarkdown(heading[2])}
          </div>,
        )
        i += 1
        continue
      }

      if (/^---+$/.test(trimmed)) {
        nodes.push(<div key={`hr-${i}`} className="my-3 h-px bg-white/10" />)
        i += 1
        continue
      }

      if (/^(\d+\.|[-*])\s+/.test(trimmed)) {
        const items: string[] = []
        while (i < lines.length && /^(\d+\.|[-*])\s+/.test(lines[i].trim())) {
          items.push(lines[i].trim().replace(/^(\d+\.|[-*])\s+/, ''))
          i += 1
        }
        nodes.push(
          <ul key={`list-${i}`} className="my-2 space-y-1 pl-4 text-[12px] leading-relaxed text-white md:text-[13px]">
            {items.map((item, idx) => (
              <li key={idx} className="list-disc">
                {renderInlineMarkdown(item)}
              </li>
            ))}
          </ul>,
        )
        continue
      }

      const paragraph: string[] = []
      while (
        i < lines.length &&
        lines[i].trim() &&
        !lines[i].trim().startsWith('```') &&
        !lines[i].trim().match(/^(#{1,4})\s+/) &&
        !(lines[i].trim().includes('|') && i + 1 < lines.length && isMarkdownTableSeparator(lines[i + 1])) &&
        !/^(\d+\.|[-*])\s+/.test(lines[i].trim()) &&
        !/^---+$/.test(lines[i].trim())
      ) {
        paragraph.push(lines[i].trim())
        i += 1
      }
      if (paragraph.length === 0) {
        paragraph.push(trimmed)
        i += 1
      }
      nodes.push(
        <p key={`p-${i}`} className="my-2 text-[12px] leading-relaxed text-white md:text-[13px]">
          {renderInlineMarkdown(paragraph.join(' '))}
        </p>,
      )
    }

    return nodes
  }, [body])

  return <div className="space-y-1">{blocks}</div>
}

/** 仅全屏：遮罩层查看思考过程 */
function DecisionThoughtFullscreenModal({
  open,
  onClose,
  title,
  body,
  language,
}: {
  open: boolean
  onClose: () => void
  title: string
  body: string
  language: Language
}) {
  useEffect(() => {
    if (!open) {
      return
    }
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => {
      document.body.style.overflow = prev
      window.removeEventListener('keydown', onKey)
    }
  }, [open, onClose])

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(body)
      notify.success(language === 'zh' ? '思考过程已复制' : 'Copied')
    } catch {
      notify.error(language === 'zh' ? '复制失败' : 'Copy failed')
    }
  }

  if (!open) return null

  return createPortal(
    <div className="fixed inset-0 z-[220] flex flex-col bg-nofx-bg-tertiary" role="presentation"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div
        className="flex h-full min-h-0 w-full flex-col"
        role="dialog"
        aria-modal="true"
        aria-labelledby="decision-thought-title"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex shrink-0 items-center justify-between gap-2 border-b border-[#2b3139] bg-nofx-bg-secondary/95 px-3 py-2.5 sm:px-4">
          <h3
            id="decision-thought-title"
            className="min-w-0 truncate text-sm font-bold text-[#EAECEF] sm:text-base"
          >
            {title}
          </h3>
          <div className="flex shrink-0 flex-wrap items-center justify-end gap-1.5">
            <button
              type="button"
              className="inline-flex items-center gap-1 rounded-md border border-[#2b3139] px-2 py-1 text-xs text-[#d4ff33] hover:bg-white/[0.06]"
              onClick={() => void handleCopy()}
            >
              <Copy className="h-3.5 w-3.5 shrink-0" aria-hidden />
              {language === 'zh' ? '复制' : 'Copy'}
            </button>
            <button
              type="button"
              className="inline-flex items-center gap-1 rounded-md border border-[#2b3139] px-2 py-1 text-xs text-[#b7bdc6] hover:bg-white/[0.06]"
              onClick={onClose}
              title={language === 'zh' ? '退出全屏' : 'Exit fullscreen'}
            >
              <Minimize2 className="h-3.5 w-3.5 shrink-0" aria-hidden />
              {language === 'zh' ? '退出全屏' : 'Exit'}
            </button>
          </div>
        </div>
        <div className="custom-scrollbar min-h-0 flex-1 overflow-y-auto p-3 sm:p-4">
          <ThoughtMarkdown body={body} />
        </div>
      </div>
    </div>,
    document.body,
  )
}

function riskRewardText(action: DecisionAction): string {
  if (!action.stop_loss || !action.take_profit || !action.price) return '-'
  const slDist = Math.abs(action.price - action.stop_loss)
  const tpDist = Math.abs(action.take_profit - action.price)
  if (slDist <= 0) return '-'
  const ratio = tpDist / slDist
  return `1 : ${ratio.toFixed(1)}`
}

function ActionCard({
  action,
  language,
  onSymbolClick,
  isMirrorFollow,
}: {
  action: DecisionAction
  language: Language
  onSymbolClick?: (symbol: string) => void
  isMirrorFollow?: boolean
}) {
  const cfg = ACTION_STYLE[action.action] || ACTION_STYLE.wait
  const isOpen = action.action.startsWith('open')
  const isClose = action.action.startsWith('close')
  const isTrade = isOpen || isClose
  /** 非开/平：等待、观望等，不展示数量，布局同「等待」卡片 */
  const isWaitLike = !isTrade
  const label = language === 'zh' ? cfg.labelZh : cfg.labelEn
  const hasConfidence = action.confidence !== undefined && action.confidence > 0
  const showReasoning = Boolean(action.reasoning && !isMirrorFollow)
  const actionErrorText =
    isMirrorFollow && action.error
      ? language === 'zh'
        ? '本轮策略状态未更新，请稍后查看账户状态。'
        : 'Strategy state was not updated. Please check the account status later.'
      : action.error
  const isMirrorLimitEntry = Boolean(isMirrorFollow && isOpen && action.price && action.price > 0)
  const orderLabel = String(action.order_id || '').trim()
  const pendingTpSlText =
    language === 'zh'
      ? '限价单\n待入场后拉出止盈止损'
      : 'Limit order\nTP/SL after entry'
  const renderRiskPrice = (price: number | undefined, colorClass: string) => {
    if (price && price > 0) {
      return formatPriceWithUnit(price, language)
    }
    if (isMirrorLimitEntry) {
      return (
        <span className="whitespace-pre-line font-sans text-[10px] leading-snug text-[#d4ff33]">
          {pendingTpSlText}
        </span>
      )
    }
    return <span className={colorClass}>-</span>
  }

  if (isWaitLike) {
    const hasSym = Boolean(action.symbol?.trim())
    const symDisplay = hasSym ? action.symbol!.trim() : language === 'zh' ? '未指定交易对' : '—'
    const iconSym = hasSym ? action.symbol!.trim() : 'MISSINGUSDT'
    return (
      <div
        className="rounded-lg border border-[#2b3139] bg-nofx-bg-tertiary/85 p-3 transition-colors hover:border-[#3d4450]"
        style={{ boxShadow: `inset 0 0 0 1px ${cfg.color}12` }}
      >
        <div className="flex items-start justify-between gap-2">
          <div className="flex min-w-0 flex-1 items-start gap-2">
            <CryptoPairIcon symbol={iconSym} size={28} />
            <div className="min-w-0 flex-1">
              <button
                type="button"
                className="block truncate text-left font-mono text-sm font-bold tracking-tight text-[#EAECEF] hover:text-[#d4ff33]"
                onClick={() => hasSym && onSymbolClick?.(action.symbol)}
              >
                {symDisplay}
              </button>
              {hasConfidence && (
                <div className="mt-0.5 text-[10px] tabular-nums" style={{ color: CONFIDENCE_SKY }}>
                  {language === 'zh' ? '置信度' : 'Conf'}: {action.confidence!.toFixed(0)}%
                </div>
              )}
            </div>
          </div>
          <span
            className="shrink-0 rounded-md px-2.5 py-1 text-xs font-medium"
            style={{ background: cfg.bg, color: cfg.color, border: `1px solid ${cfg.color}40` }}
          >
            {label}
          </span>
        </div>
        {showReasoning ? (
          <div className="mt-2 flex gap-1.5 text-[11px] leading-relaxed text-[#848E9C]">
            <MessageCircle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[#5e6673]" aria-hidden />
            <span>{action.reasoning}</span>
          </div>
        ) : null}
        {actionErrorText ? (
          <div className="mt-2 rounded border border-[#F6465D]/35 bg-[#F6465D]/10 px-2 py-1.5 text-[11px] text-[#F6465D]">
            {actionErrorText}
          </div>
        ) : null}
      </div>
    )
  }

  // 开仓 / 平仓：参考图三
  const showOk = action.success === true
  const qtyStr =
    action.quantity !== undefined && action.quantity !== null
      ? typeof action.quantity === 'number'
        ? action.quantity.toFixed(2)
        : String(action.quantity)
      : '—'

  return (
    <div
      className="rounded-lg border border-[#2b3139] bg-nofx-bg-tertiary/85 p-3 transition-colors hover:border-[#3d4450]"
      style={{ boxShadow: `inset 0 0 0 1px ${cfg.color}18` }}
    >
      <div className="flex items-center justify-between gap-2">
        <div className="flex min-w-0 flex-1 items-center gap-2">
          <CryptoPairIcon symbol={action.symbol} size={32} />
          <button
            type="button"
            className="truncate text-left font-mono text-base font-bold tracking-tight text-[#EAECEF] hover:text-[#d4ff33]"
            onClick={() => onSymbolClick?.(action.symbol)}
          >
            {action.symbol}
          </button>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          {showOk ? <CheckCircle2 className="h-4 w-4 shrink-0 text-[#0ECB81]" aria-hidden /> : null}
          <span
            className="rounded-full px-2.5 py-1 text-xs font-bold"
            style={{ background: cfg.bg, color: cfg.color, border: `1px solid ${cfg.color}55` }}
          >
            {label}
          </span>
        </div>
      </div>

      <div className="mt-1.5 flex flex-wrap items-baseline gap-x-4 gap-y-1 text-[11px]">
        {isMirrorFollow && orderLabel ? (
          <div>
            <span className="text-[#5e6673]">{language === 'zh' ? '编号' : 'ID'}:</span>{' '}
            <span className="font-mono tabular-nums text-[#d4ff33]">{orderLabel}</span>
          </div>
        ) : null}
        <div>
          <span className="text-[#5e6673]">{language === 'zh' ? '数量' : 'Qty'}:</span>{' '}
          <span className="font-mono tabular-nums text-[#EAECEF]">{isTrade ? qtyStr : '—'}</span>
        </div>
        {hasConfidence ? (
          <div>
            <span className="text-[#5e6673]">{language === 'zh' ? '置信度' : 'Conf'}:</span>{' '}
            <span className="font-mono font-semibold tabular-nums" style={{ color: CONFIDENCE_SKY }}>
              {action.confidence!.toFixed(0)}%
            </span>
          </div>
        ) : null}
      </div>

      <div className="mt-3 grid grid-cols-2 gap-3 text-[10px] leading-tight md:grid-cols-4">
        <div>
          <div className="text-[#5e6673]">{language === 'zh' ? '杠杆' : 'Lev'}</div>
          <div className="mt-0.5 font-mono text-sm font-semibold tabular-nums" style={{ color: LEVERAGE_BLUE }}>
            {action.leverage ? `${action.leverage}x` : '-'}
          </div>
        </div>
        <div>
          <div className="font-medium text-[#F6465D]/95">{language === 'zh' ? '止损' : 'SL'}</div>
          <div className="mt-0.5 font-mono text-sm font-semibold tabular-nums text-[#F6465D]">
            {renderRiskPrice(action.stop_loss, 'text-[#F6465D]')}
          </div>
        </div>
        <div>
          <div className="font-medium text-[#0ECB81]/95">{language === 'zh' ? '止盈' : 'TP'}</div>
          <div className="mt-0.5 font-mono text-sm font-semibold tabular-nums text-[#0ECB81]">
            {renderRiskPrice(action.take_profit, 'text-[#0ECB81]')}
          </div>
        </div>
        <div>
          <div className="text-[#5e6673]">{language === 'zh' ? '风险收益比' : 'R:R'}</div>
          <div className="mt-0.5 font-mono text-sm font-semibold tabular-nums text-[#EAECEF]">
            {riskRewardText(action)}
          </div>
        </div>
      </div>

      {showReasoning ? (
        <div className="mt-3 flex gap-1.5 border-t border-[#2b3139]/90 pt-2.5 text-[11px] leading-relaxed text-[#848E9C]">
          <MessageCircle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[#5e6673]" aria-hidden />
          <span>{action.reasoning}</span>
        </div>
      ) : null}

      {actionErrorText ? (
        <div className="mt-2 rounded border border-[#F6465D]/35 bg-[#F6465D]/10 px-2 py-1.5 text-[11px] text-[#F6465D]">
          {actionErrorText}
        </div>
      ) : null}
    </div>
  )
}

export function DecisionCard({ decision, language, onSymbolClick }: DecisionCardProps) {
  const [showThought, setShowThought] = useState(false)
  const [thoughtFullscreen, setThoughtFullscreen] = useState(false)
  const [showPromptDetail, setShowPromptDetail] = useState(false)

  const copyToClipboard = async (text: string, label: string) => {
    try {
      await navigator.clipboard.writeText(text)
      notify.success(language === 'zh' ? `${label} 已复制` : `${label} copied`)
    } catch {
      notify.error(language === 'zh' ? '复制失败' : 'Copy failed')
    }
  }

  /** 跟单端默认不公开策略作者思考过程；大星星策略单独展示主控 AI 文本。 */
  const isFollowRecord = isComkunFollowRecord(decision)
  const showDaxingxingMasterThought = isDaxingxingComkunFollowRecord(decision)
  const hideFollowThought = isFollowRecord && !showDaxingxingMasterThought
  const hideInternalSyncError =
    isFollowRecord ||
    /comkun|镜像|主控|被控|广播|跟单/i.test(
      `${decision.system_prompt || ''} ${decision.raw_response || ''} ${decision.error_message || ''}`,
    )
  const promptLocked = isFollowRecord
  const isBalanceStop = isComkunBalanceStopRecord(decision)
  const balanceStopMessage =
    (decision.error_message && decision.error_message.trim()) ||
    (decision.cot_trace && decision.cot_trace.trim()) ||
    (language === 'zh'
      ? '平台余额不足，请先充值平台余额；系统已停止本智能体，本轮不会执行任何操作。'
      : 'Platform balance is insufficient. Please recharge; this agent has been stopped.')
  const thoughtBody = hideFollowThought
    ? language === 'zh'
      ? FOLLOW_THOUGHT_HIDDEN_ZH
      : FOLLOW_THOUGHT_HIDDEN_EN
    : showDaxingxingMasterThought
      ? filterDaxingxingMasterThought((decision.cot_trace && decision.cot_trace.trim()) || '')
      : (decision.cot_trace && decision.cot_trace.trim()) || ''
  const hasThoughtContent = thoughtBody.length > 0

  const cycleLabel = language === 'zh' ? '周期' : t('cycle', language)
  const effectiveDecisions = useMemo(() => getEffectiveDecisions(decision), [decision])
  const scanSymbols = useMemo(
    () => scanSymbolsForCycle(decision, effectiveDecisions),
    [decision, effectiveDecisions],
  )
  const displayScanSymbols = useMemo(() => {
    if (scanSymbols.length > 0) return scanSymbols
    const set = new Set<string>()
    for (const a of effectiveDecisions) {
      const s = String(a.symbol || '').trim().toUpperCase()
      if (s) set.add(s)
    }
    return [...set]
  }, [scanSymbols, effectiveDecisions])
  const symbolScanRows = useMemo(
    () =>
      buildSymbolScanRows(
        displayScanSymbols,
        effectiveDecisions,
        decision.cot_trace || '',
        language,
        decision.system_prompt,
      ),
    [displayScanSymbols, effectiveDecisions, decision.cot_trace, language, decision.system_prompt],
  )
  const tradeActions = useMemo(
    () =>
      effectiveDecisions.filter(
        (a) => a.action.startsWith('open') || a.action.startsWith('close'),
      ),
    [effectiveDecisions],
  )

  return (
    <div className="rounded-xl border border-[#2b3139] bg-nofx-bg-secondary p-3 shadow-[0_4px_20px_rgba(0,0,0,0.35)] md:p-4">
      {/* 顶：一条元信息（中文为主，不展示执行日志里的英文） */}
      <div className="mb-3 border-b border-[#2b3139]/80 pb-2.5">
        <p className="text-[11px] leading-relaxed text-[#848E9C]">
          <span className="text-[#c4cf45]">{formatRelativeTime(decision.timestamp, language)}</span>
          <span className="mx-1.5 text-[#3d4450]">|</span>
          <span className="font-mono font-semibold text-[#EAECEF]">
            {cycleLabel}#{decision.cycle_number}
          </span>
        </p>
      </div>

      {!hideFollowThought ? (
        <div className="mb-3 rounded-lg border border-[#2b3139]/70 bg-nofx-bg-tertiary/70 px-2.5 py-2">
          <div className="mb-2 text-[10px] font-semibold uppercase tracking-wide text-[#5e6673]">
            {language === 'zh' ? '扫描币种' : 'Scanned symbols'}
          </div>
          {isBalanceStop ? (
            <div className="rounded-lg border border-[#F6465D]/45 bg-[#F6465D]/10 px-3 py-3">
              <p className="text-base font-bold leading-relaxed text-[#F6465D] md:text-lg">
                {balanceStopMessage}
              </p>
            </div>
          ) : symbolScanRows.length > 0 ? (
            <ul className="space-y-2.5">
              {symbolScanRows.map((row) => (
                <li
                  key={row.symbol}
                  className="rounded-md border border-[#2b3139]/60 bg-nofx-bg-secondary/80 p-2.5 transition-colors hover:border-[#3d4450]"
                >
                  <div className="flex items-start gap-2">
                    <button
                      type="button"
                      onClick={() => onSymbolClick?.(row.symbol)}
                      className="shrink-0 rounded-md ring-offset-2 ring-offset-nofx-bg-secondary hover:ring-1 hover:ring-[#d4ff33]/40"
                      title={language === 'zh' ? '在图表中查看' : 'View on chart'}
                    >
                      <CryptoPairIcon symbol={row.symbol} size={32} />
                    </button>
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
                        <button
                          type="button"
                          onClick={() => onSymbolClick?.(row.symbol)}
                          className="font-mono text-sm font-bold tracking-tight text-[#EAECEF] hover:text-[#d4ff33]"
                        >
                          {row.symbol}
                        </button>
                        <span
                          className="rounded px-1.5 py-0.5 text-[10px] font-medium text-[#848E9C]"
                          style={{ background: FIREFLY_BG, color: FIREFLY }}
                        >
                          {row.actionHint}
                        </span>
                        <span className="text-[10px] tabular-nums text-[#5e6673]">
                          {language === 'zh' ? '置信度' : 'Conf'}{' '}
                          <span className="font-semibold" style={{ color: CONFIDENCE_SKY }}>
                            {row.confidencePct != null ? `${row.confidencePct}%` : '—'}
                          </span>
                        </span>
                      </div>
                      <p className="mt-1.5 text-[11px] leading-relaxed text-white">{row.brief}</p>
                    </div>
                  </div>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-[11px] leading-relaxed text-[#5e6673]">
              {language === 'zh'
                ? '未识别到本轮扫描币种（候选列表与决策均为空）。'
                : 'No scanned symbols for this cycle.'}
            </p>
          )}
        </div>
      ) : null}

      {tradeActions.length > 0 ? (
        <div className="space-y-2.5">
          <div className="text-[10px] font-semibold uppercase tracking-wide text-[#5e6673]">
            {decision.system_prompt === 'COMKUN_MIRROR_FOLLOW'
              ? language === 'zh'
                ? '限价 / 止盈止损快照（杠杆 · 名义金额 · 方向）'
                : 'Limit & TP/SL snapshot (lev · notional · side)'
              : language === 'zh'
                ? '执行动作（开/平仓）'
                : 'Executed open / close'}
          </div>
          {tradeActions.map((action, index) => (
            <ActionCard
              key={`${action.symbol}-${action.action}-${index}`}
              action={action}
              language={language}
              onSymbolClick={onSymbolClick}
              isMirrorFollow={decision.system_prompt === 'COMKUN_MIRROR_FOLLOW'}
            />
          ))}
        </div>
      ) : null}

      {/* 工具条紧贴列表下方；从「思考过程」往下展开，不把标题顶到卡片最底 */}
      <div className="mt-3 overflow-hidden rounded-lg border border-[#2b3139] bg-nofx-bg-secondary/90">
        <div className="flex flex-wrap items-center gap-2 border-b border-[#2b3139]/70 bg-nofx-bg-tertiary/50 px-2 py-2">
          <button
            type="button"
            onClick={() => setShowThought(!showThought)}
            className="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-xs font-semibold transition-colors hover:bg-white/[0.06]"
            style={{ color: FIREFLY }}
          >
            <Sparkles className="h-3.5 w-3.5 shrink-0" aria-hidden />
            {language === 'zh' ? '思考过程' : 'Thought'}
            <ChevronDown
              className={`h-3.5 w-3.5 shrink-0 text-[#848E9C] transition-transform ${showThought ? 'rotate-180' : ''}`}
              aria-hidden
            />
          </button>
          <span className="text-[#3d4450]" aria-hidden>
            |
          </span>
          <button
            type="button"
            disabled={promptLocked}
            onClick={() => {
              if (!promptLocked) setShowPromptDetail(!showPromptDetail)
            }}
            className={`inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-45 ${
              promptLocked
                ? 'text-[#5e6673]'
                : showPromptDetail
                  ? 'text-[#b7bdc6] hover:bg-white/[0.06]'
                  : 'text-[#848E9C] hover:bg-white/[0.06]'
            }`}
            title={promptLocked ? (language === 'zh' ? '该策略未公开提示词' : 'Prompt is private') : undefined}
          >
            <Lock className="h-3 w-3 shrink-0" aria-hidden />
            {language === 'zh' ? '提示词' : 'Prompt'}
          </button>
        </div>

        {showThought ? (
          <div className="border-t border-[#2b3139]/60 bg-nofx-bg-tertiary/90 p-3">
            <div className="group relative rounded-lg border-2 border-[#d4ff33]/45 bg-nofx-bg-secondary shadow-[inset_0_0_0_1px_rgba(212,255,51,0.08)]">
              <div className="absolute right-2 top-2 z-10 flex items-center gap-0.5">
                <button
                  type="button"
                  disabled={!hasThoughtContent}
                  className="rounded-md p-1.5 text-[#9ca3af] transition-colors hover:bg-white/[0.08] hover:text-[#d4ff33] disabled:pointer-events-none disabled:opacity-35"
                  title={language === 'zh' ? '复制' : 'Copy'}
                  onClick={() => void copyToClipboard(thoughtBody, language === 'zh' ? '思考过程' : 'Thought')}
                >
                  <Copy className="h-4 w-4" aria-hidden />
                </button>
                <button
                  type="button"
                  disabled={!hasThoughtContent}
                  className="rounded-md p-1.5 text-[#9ca3af] transition-colors hover:bg-white/[0.08] hover:text-[#d4ff33] disabled:pointer-events-none disabled:opacity-35"
                  title={language === 'zh' ? '全屏' : 'Fullscreen'}
                  onClick={() => setThoughtFullscreen(true)}
                >
                  <Maximize2 className="h-4 w-4" aria-hidden />
                </button>
              </div>
              <div className="flex gap-3 p-3 pr-14">
                <div className="flex shrink-0 flex-col items-center pt-1">
                  <span
                    className="h-1.5 w-1.5 rounded-full bg-violet-500 shadow-[0_0_10px_rgba(139,92,246,0.65)]"
                    aria-hidden
                  />
                  <Sparkles className="mt-2 h-4 w-4 shrink-0 text-[#d4ff33]" aria-hidden />
                </div>
                <div className="min-w-0 flex-1">
                  <div className="text-sm font-bold text-[#EAECEF]">
                    {language === 'zh' ? '思考过程' : 'Thought process'}
                  </div>
                  <div className="mt-0.5 text-[11px] text-[#9ca3af]">
                    <span className="text-[#a78bfa]">{language === 'zh' ? 'AI 分析' : 'AI'}</span>
                    <span className="mx-1.5 text-[#3d4450]">·</span>
                    <span>{formatRelativeTime(decision.timestamp, language)}</span>
                    <span className="mx-1.5 text-[#3d4450]">·</span>
                    <span className="font-mono text-[#848E9C]">
                      {language === 'zh' ? '周期' : cycleLabel}#{decision.cycle_number}
                    </span>
                  </div>
                  <div className="custom-scrollbar mt-2 max-h-64 overflow-y-auto rounded-md border border-[#2b3139]/60 bg-black/25 p-2.5">
                    {hasThoughtContent ? (
                      <ThoughtMarkdown body={thoughtBody} />
                    ) : (
                      <p className="text-[12px] leading-relaxed text-[#5e6673] md:text-[13px]">
                        {language === 'zh'
                          ? '本轮未产生思考链记录（模型未输出或尚未落库）。'
                          : 'No chain-of-thought was recorded for this cycle.'}
                      </p>
                    )}
                  </div>
                </div>
              </div>
            </div>
          </div>
        ) : null}

        {!promptLocked && showPromptDetail && (decision.system_prompt || decision.input_prompt) ? (
          <div className="space-y-2 border-t border-[#2b3139]/60 bg-nofx-bg-tertiary/80 p-3 text-[10px]">
            {decision.system_prompt ? (
              <div>
                <div className="mb-1 flex items-center justify-between text-[#a78bfa]">
                  <span className="font-semibold">{language === 'zh' ? '系统提示' : 'System'}</span>
                  <button
                    type="button"
                    className="rounded border border-[#2b3139] px-1.5 py-0.5 text-[#848E9C] hover:border-[#d4ff33]/35"
                    onClick={() => void copyToClipboard(decision.system_prompt, language === 'zh' ? '系统提示' : 'System')}
                  >
                    {language === 'zh' ? '复制' : 'Copy'}
                  </button>
                </div>
                <pre className="whitespace-pre-wrap break-words rounded border border-[#2b3139] bg-black/30 p-2 text-[11px] leading-relaxed text-[#b7bdc6]">
                  {decision.system_prompt}
                </pre>
              </div>
            ) : null}
            {decision.input_prompt ? (
              <div>
                <div className="mb-1 flex items-center justify-between text-[#60a5fa]">
                  <span className="font-semibold">{language === 'zh' ? '输入内容' : 'Input'}</span>
                  <button
                    type="button"
                    className="rounded border border-[#2b3139] px-1.5 py-0.5 text-[#848E9C] hover:border-[#d4ff33]/35"
                    onClick={() => void copyToClipboard(decision.input_prompt, language === 'zh' ? '输入内容' : 'Input')}
                  >
                    {language === 'zh' ? '复制' : 'Copy'}
                  </button>
                </div>
                <pre className="whitespace-pre-wrap break-words rounded border border-[#2b3139] bg-black/30 p-2 text-[11px] leading-relaxed text-[#b7bdc6]">
                  {decision.input_prompt}
                </pre>
              </div>
            ) : null}
          </div>
        ) : null}
      </div>

      {decision.error_message && !decision.success && !isBalanceStop && (
        <div className="mt-2 rounded-lg border border-[#F6465D]/40 bg-[#F6465D]/10 p-2 text-xs text-[#F6465D]">
          {hideInternalSyncError
            ? language === 'zh'
              ? sanitizeInternalSyncText(decision.error_message) || '本轮策略状态未更新，请稍后查看账户状态。'
              : 'Strategy state was not updated. Please check the account status later.'
            : sanitizeInternalSyncText(decision.error_message)}
        </div>
      )}

      <DecisionThoughtFullscreenModal
        open={thoughtFullscreen}
        onClose={() => setThoughtFullscreen(false)}
        title={
          language === 'zh'
            ? `周期 #${decision.cycle_number} · 思考过程`
            : `Cycle #${decision.cycle_number} · Thought`
        }
        body={thoughtBody}
        language={language}
      />
    </div>
  )
}
