/** 登录/注册页顶部行情：GET /api/market/board 的 ticker 字段 */

export type BoardQuote = {
  symbol: string
  price?: number | string
  change_pct?: number | string
  up?: boolean
  category?: string
}

export type TickerItem = { sym: string; price: string; chg: string; dir: 'up' | 'down' }

export const DEFAULT_TICKER_ITEMS: TickerItem[] = [
  { sym: 'BTC', price: '—', chg: '+0.00%', dir: 'up' },
  { sym: 'ETH', price: '—', chg: '+0.00%', dir: 'up' },
  { sym: 'SOL', price: '—', chg: '+0.00%', dir: 'down' },
  { sym: 'BNB', price: '—', chg: '+0.00%', dir: 'up' },
  { sym: 'XRP', price: '—', chg: '+0.00%', dir: 'up' },
]

function num(v: unknown): number {
  if (typeof v === 'number' && Number.isFinite(v)) return v
  const n = parseFloat(String(v ?? '').replace(/,/g, ''))
  return Number.isFinite(n) ? n : NaN
}

export function formatTickerPrice(p: number): string {
  if (!Number.isFinite(p) || p <= 0) return '—'
  if (p >= 1000) return p.toLocaleString('en-US', { maximumFractionDigits: 2 })
  if (p >= 1) return p.toLocaleString('en-US', { maximumFractionDigits: 4 })
  return p.toLocaleString('en-US', { maximumFractionDigits: 6 })
}

export function quoteToTickerItem(q: BoardQuote): TickerItem {
  const priceN = num(q.price)
  const chN = num(q.change_pct)
  const sym = String(q.symbol || '')
    .replace(/USDT$/i, '')
    .replace(/^USD/, 'USD')
  const chg = `${chN >= 0 ? '+' : ''}${Number.isFinite(chN) ? chN.toFixed(2) : '0.00'}%`
  const up = q.up !== undefined ? Boolean(q.up) : chN >= 0
  return {
    sym: sym || String(q.symbol),
    price: formatTickerPrice(priceN),
    chg,
    dir: up ? 'up' : 'down',
  }
}

export async function fetchBoardTickerItems(): Promise<TickerItem[]> {
  try {
    const res = await fetch('/api/market/board', { credentials: 'same-origin' })
    if (!res.ok) throw new Error(String(res.status))
    const data = (await res.json()) as { ticker?: BoardQuote[] }
    const raw = Array.isArray(data.ticker) ? data.ticker : []
    const items = raw.slice(0, 28).map(quoteToTickerItem)
    if (items.length > 0) return items
  } catch {
    /* ignore */
  }
  return DEFAULT_TICKER_ITEMS
}
