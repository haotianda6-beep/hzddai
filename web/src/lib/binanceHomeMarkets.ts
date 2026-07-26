/** 首页行情：币安 24h 全市场 ticker，与静态 crypto-home.html 逻辑一致 */

export type HomeAssetAccent = 'btc' | 'eth' | 'sol' | 'bnb' | 'gen'

export interface HomeAssetCard {
  sym: string
  name: string
  price: string
  chg: string
  dir: 'up' | 'down'
  icon: string
  accent: HomeAssetAccent
}

export interface HomeTickerItem {
  sym: string
  price: string
  chg: string
  dir: 'up' | 'down'
}

const BINANCE_24H_URL = 'https://api.binance.com/api/v3/ticker/24hr'

const FIXED_USDT = ['BTCUSDT', 'ETHUSDT', 'SOLUSDT', 'BNBUSDT'] as const

const FIXED_META: Record<
  (typeof FIXED_USDT)[number],
  Pick<HomeAssetCard, 'sym' | 'name' | 'icon' | 'accent'>
> = {
  BTCUSDT: { sym: 'BTC', name: 'Bitcoin', icon: '₿', accent: 'btc' },
  ETHUSDT: { sym: 'ETH', name: 'Ethereum', icon: 'Ξ', accent: 'eth' },
  SOLUSDT: { sym: 'SOL', name: 'Solana', icon: 'S', accent: 'sol' },
  BNBUSDT: { sym: 'BNB', name: 'BNB', icon: 'B', accent: 'bnb' },
}

function fmtUsd(n: string) {
  const x = parseFloat(n)
  if (!Number.isFinite(x)) return '—'
  if (x >= 1000) {
    return '$' + x.toLocaleString('en-US', { maximumFractionDigits: 0 })
  }
  if (x >= 1) {
    return (
      '$' +
      x.toLocaleString('en-US', {
        minimumFractionDigits: 2,
        maximumFractionDigits: 2,
      })
    )
  }
  return '$' + x.toLocaleString('en-US', { maximumFractionDigits: 6 })
}

function pctDir(p: string): { text: string; dir: 'up' | 'down' } {
  const v = parseFloat(p)
  if (!Number.isFinite(v)) return { text: '0.00%', dir: 'up' }
  const sign = v > 0 ? '+' : ''
  return { text: `${sign}${v.toFixed(2)}%`, dir: v >= 0 ? 'up' : 'down' }
}

function isLeveragedOrDust(sym: string, quoteVol: string) {
  if (!sym.endsWith('USDT') || sym === 'USDTUSDT') return true
  if (/UP\d*USDT$/i.test(sym) || /DOWN\d*USDT$/i.test(sym)) return true
  if (
    /BULL|BEAR|USDCUSDT|FDUSDUSDT|TUSDUSDT|USDPUSDT|PAXGUSDT/.test(sym)
  ) {
    return true
  }
  const qv = parseFloat(quoteVol)
  if (!Number.isFinite(qv) || qv < 5e6) return true
  return false
}

function cardFromRow(
  t: {
    symbol: string
    lastPrice: string
    priceChangePercent: string
  },
  meta: (typeof FIXED_META)[keyof typeof FIXED_META] | null
): HomeAssetCard {
  const { text, dir } = pctDir(t.priceChangePercent)
  const base = meta ? meta.sym : t.symbol.replace(/USDT$/i, '')
  return {
    sym: base,
    name: meta ? meta.name : `${base} / USDT`,
    price: fmtUsd(t.lastPrice),
    chg: text,
    dir,
    icon: meta ? meta.icon : base.slice(0, 2).toUpperCase(),
    accent: meta ? meta.accent : 'gen',
  }
}

function tickerFromRow(t: {
  symbol: string
  lastPrice: string
  priceChangePercent: string
}): HomeTickerItem {
  const base = t.symbol.replace(/USDT$/i, '')
  const { text, dir } = pctDir(t.priceChangePercent)
  const price = fmtUsd(t.lastPrice).replace('$', '')
  return { sym: base, price, chg: text.replace('%', ''), dir }
}

/** 一次请求：8 张热门卡 + 顶部滚动行情用列表 */
export async function fetchHomeMarketPageData(): Promise<{
  cards: HomeAssetCard[]
  ticker: HomeTickerItem[]
}> {
  const r = await fetch(BINANCE_24H_URL)
  if (!r.ok) throw new Error(`HTTP ${r.status}`)
  const all = (await r.json()) as Array<{
    symbol: string
    lastPrice: string
    priceChangePercent: string
    quoteVolume: string
  }>
  if (!Array.isArray(all)) throw new Error('格式异常')

  const bySym = Object.fromEntries(all.map((x) => [x.symbol, x]))

  const fixedRows = FIXED_USDT.map((s) => {
    const t = bySym[s]
    if (!t) return null
    return cardFromRow(t, FIXED_META[s])
  }).filter(Boolean) as HomeAssetCard[]

  const fixedSet = new Set<string>(FIXED_USDT)
  const pool = all.filter(
    (x) => !fixedSet.has(x.symbol) && !isLeveragedOrDust(x.symbol, x.quoteVolume)
  )

  const byGain = [...pool].sort(
    (a, b) =>
      parseFloat(b.priceChangePercent) - parseFloat(a.priceChangePercent)
  )
  const top2Gain = byGain.slice(0, 2).map((t) => cardFromRow(t, null))

  const byLoss = [...pool].sort(
    (a, b) =>
      parseFloat(a.priceChangePercent) - parseFloat(b.priceChangePercent)
  )
  const top2Loss = byLoss.slice(0, 2).map((t) => cardFromRow(t, null))

  const cards = [...fixedRows, ...top2Gain, ...top2Loss]

  const forScroll = [...pool]
    .sort((a, b) => parseFloat(b.quoteVolume) - parseFloat(a.quoteVolume))
    .slice(0, 24)
    .map(tickerFromRow)
  const ticker =
    forScroll.length > 0
      ? forScroll
      : (FIXED_USDT.map((s) => bySym[s]).filter(Boolean) as typeof all).map(
          tickerFromRow
        )

  return { cards, ticker }
}

export async function fetchHomeMarketCards(): Promise<HomeAssetCard[]> {
  const { cards } = await fetchHomeMarketPageData()
  return cards
}
