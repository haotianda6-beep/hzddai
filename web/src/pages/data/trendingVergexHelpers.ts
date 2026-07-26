import type { Language } from '../../i18n/translations'
import type { MarketTableRow } from '../../types'

export type PrimaryTab = 'stocks' | 'indices' | 'commodities' | 'crypto' | 'prediction' | 'forex'
export type CryptoMetric = 'net-flow' | 'oi' | 'depth' | 'rates' | 'price'
export type StockFilter = 'all' | 'mag7' | 'ai' | 'platforms' | 'active' | 'gainers' | 'losers'

export const PRIMARY_ORDER: PrimaryTab[] = [
  'stocks',
  'indices',
  'commodities',
  'crypto',
  'prediction',
  'forex',
]

const MAG7 = new Set(
  ['AAPL.US', 'MSFT.US', 'GOOGL.US', 'AMZN.US', 'META.US', 'NVDA.US', 'TSLA.US'].map((s) =>
    s.toUpperCase(),
  ),
)

export function fmtPrice(n: number, lang: Language): string {
  const loc = lang === 'zh' ? 'zh-CN' : 'en-US'
  if (!Number.isFinite(n) || n === 0) return '—'
  if (Math.abs(n) >= 1000) return n.toLocaleString(loc, { maximumFractionDigits: 2 })
  if (Math.abs(n) >= 1) return n.toLocaleString(loc, { minimumFractionDigits: 2, maximumFractionDigits: 4 })
  return n.toLocaleString(loc, { maximumFractionDigits: 6 })
}

export function fmtPct(n: number): string {
  if (!Number.isFinite(n)) return '—'
  return n >= 0 ? `+${n.toFixed(2)}%` : `${n.toFixed(2)}%`
}

export function baseSymbol(sym: string): string {
  return sym.replace(/USDT$/i, '').replace(/\.US$/i, '').toUpperCase()
}

export function cryptoIconUrl(sym: string): string {
  const b = baseSymbol(sym).toLowerCase()
  if (!b || b === '—') return ''
  return `https://cdn.jsdelivr.net/gh/spothq/cryptocurrency-icons@master/32/color/${b}.png`
}

export function stockIconUrl(sym: string): string {
  const t = sym.replace(/\.US$/i, '').toUpperCase()
  if (!t) return ''
  return `https://financialmodelingprep.com/image-stock/${t}.png`
}

export function filterStockRows(rows: MarketTableRow[], f: StockFilter): MarketTableRow[] {
  let list = [...rows]
  if (f === 'mag7') list = list.filter((r) => MAG7.has(r.symbol.toUpperCase()))
  if (f === 'ai') list = list.filter((r) => /nvda|meta|googl|msft|amd|pltr|intc/i.test(r.symbol))
  if (f === 'gainers') list = list.filter((r) => r.change_pct > 0).sort((a, b) => b.change_pct - a.change_pct)
  else if (f === 'losers') list = list.filter((r) => r.change_pct < 0).sort((a, b) => a.change_pct - b.change_pct)
  else if (f === 'active') list = list.sort((a, b) => (b.volume ?? 0) - (a.volume ?? 0))
  return list.map((r, i) => ({ ...r, rank: i + 1 }))
}

export function splitInflowOutflow(rows: MarketTableRow[], limit = 20) {
  const up = rows
    .filter((r) => r.change_pct > 0)
    .sort((a, b) => (b.volume ?? 0) - (a.volume ?? 0))
    .slice(0, limit)
    .map((r, i) => ({ ...r, rank: i + 1 }))
  const down = rows
    .filter((r) => r.change_pct < 0)
    .sort((a, b) => (b.volume ?? 0) - (a.volume ?? 0))
    .slice(0, limit)
    .map((r, i) => ({ ...r, rank: i + 1 }))
  return { up, down }
}

export function flowDisplay(r: MarketTableRow, positive: boolean): string {
  const v = r.volume_str && r.volume_str !== '—' ? r.volume_str : '—'
  if (v === '—') return '—'
  return positive ? `+${v}` : `-${v.replace(/^\$/, '$')}`
}
