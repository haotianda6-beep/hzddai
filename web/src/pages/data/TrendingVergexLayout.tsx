import { useMemo, useState } from 'react'
import { ChevronRight, Loader2, RefreshCw, Search } from 'lucide-react'
import type { Language } from '../../i18n/translations'
import type { MarketBoardPayload, MarketCardBlock, MarketTableRow } from '../../types'
import {
  PRIMARY_ORDER,
  type CryptoMetric,
  type PrimaryTab,
  type StockFilter,
  baseSymbol,
  cryptoIconUrl,
  filterStockRows,
  flowDisplay,
  fmtPct,
  fmtPrice,
  splitInflowOutflow,
  stockIconUrl,
} from './trendingVergexHelpers'
import './trending-vergex.css'

const CARD_IDS = ['crypto_vol', 'us_stocks', 'commodities', 'prediction', 'forex'] as const

const t = {
  hotMarkets: '热门市场',
  primary: {
    stocks: '美股',
    indices: '指数',
    commodities: '大宗商品',
    crypto: '加密',
    prediction: '预测',
    forex: '外汇',
  },
  cryptoMetric: {
    'net-flow': '净流入',
    oi: '持仓量',
    depth: '深度',
    rates: '资金费率',
    price: '价格',
  },
  filters: {
    all: '全部',
    mag7: '科技七巨头',
    ai: 'AI',
    platforms: '平台',
    active: '最活跃',
    gainers: '涨幅榜',
    losers: '跌幅榜',
  },
  col: {
    rank: '#',
    name: '名称',
    asset: '资产',
    price: '价格',
    change: '涨跌',
    netFlow: '净流入',
    marketCap: '市值',
    openInterest: '持仓量',
    funding: '资金费率',
  },
  topInflow: '净流入榜',
  topOutflow: '净流出榜',
  search: '搜索…',
  refresh: '刷新',
  loading: '加载中…',
  hydrating: '正在同步其他市场数据…',
  empty: '暂无数据',
  error: '加载失败，请稍后重试',
  viewBoard: '综合看板',
  period: '周期',
  flowNote: '净流入列为 24h 成交额（Binance 公开数据）',
  marketsNav: '市场分类',
  cardTitles: {
    crypto_vol: '加密 AI500',
    us_stocks: '美股精选',
    commodities: '大宗商品精选',
    prediction: '预测市场精选',
    forex: '外汇精选',
  } as Record<string, string>,
}

const STOCK_FILTERS: StockFilter[] = ['all', 'mag7', 'ai', 'platforms', 'active', 'gainers', 'losers']
const CRYPTO_METRICS: CryptoMetric[] = ['net-flow', 'oi', 'depth', 'rates', 'price']

function tabToRowsKey(tab: PrimaryTab): string {
  return tab
}

function AssetIcon({ symbol, tab }: { symbol: string; tab: PrimaryTab }) {
  const src = tab === 'crypto' ? cryptoIconUrl(symbol) : stockIconUrl(symbol)
  const letter = baseSymbol(symbol).slice(0, 2) || '?'
  const [broken, setBroken] = useState(false)
  if (!src || broken) {
    return <span className="vergex-trending__icon-fallback">{letter}</span>
  }
  return (
    <img src={src} alt="" className="vergex-trending__icon" onError={() => setBroken(true)} />
  )
}

function AssetNameCell({ row, tab }: { row: MarketTableRow; tab: PrimaryTab }) {
  const base = baseSymbol(row.symbol)
  const pair =
    tab === 'crypto'
      ? `${base}/${row.symbol.toUpperCase().endsWith('USDT') ? 'USDT' : row.symbol}`
      : row.name
  return (
    <div className="vergex-trending__asset-cell">
      <AssetIcon symbol={row.symbol} tab={tab} />
      <div className="vergex-trending__asset-text">
          <div className="vergex-trending__asset-sym">{base}</div>
        <div className="vergex-trending__asset-pair">{pair}</div>
      </div>
    </div>
  )
}

function hotTitle(card: MarketCardBlock, labels: Record<string, string>) {
  return labels[card.id] || card.title_zh
}

function HotCard({
  card,
  lang,
  labels,
}: {
  card: MarketCardBlock
  lang: Language
  labels: Record<string, string>
}) {
  const iconTab: PrimaryTab = card.id === 'crypto_vol' ? 'crypto' : 'stocks'
  return (
    <article className="vergex-trending__hot-card">
      <div className="vergex-trending__hot-card-head">
        <span>{hotTitle(card, labels)}</span>
        <ChevronRight className="h-4 w-4 text-[#8b8f96]" />
      </div>
      {card.items
        .filter((it) => it.symbol && it.symbol !== '—')
        .slice(0, 3)
        .map((it, idx) => (
          <div key={`${card.id}-${idx}`} className="vergex-trending__hot-row">
            <AssetIcon symbol={it.symbol} tab={iconTab} />
            <div className="vergex-trending__hot-mid">
              <div className="vergex-trending__hot-sym">{baseSymbol(it.symbol)}</div>
              <div className="vergex-trending__hot-sub">{it.extra || it.name}</div>
            </div>
            <div className="vergex-trending__hot-right">
              <div className="vergex-trending__hot-price">{fmtPrice(it.price, lang)}</div>
              <div className={it.up ? 'vergex-trending__up' : 'vergex-trending__down'}>
                {fmtPct(it.change_pct)}
              </div>
            </div>
          </div>
        ))}
    </article>
  )
}

function FlowPanel({
  title,
  titleClass,
  rows,
  tab,
  lang,
  positive,
}: {
  title: string
  titleClass: string
  rows: MarketTableRow[]
  tab: PrimaryTab
  lang: Language
  positive: boolean
}) {
  return (
    <div>
      <h3 className={`vergex-trending__panel-title ${titleClass}`}>{title}</h3>
      <div className="vergex-trending__table-wrap">
        <table className="vergex-trending__table">
          <thead>
            <tr>
              <th>{t.col.rank}</th>
              <th>{t.col.name}</th>
              <th>{t.col.price}</th>
              <th>{t.col.change}</th>
              <th>{t.col.netFlow}</th>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 ? (
              <tr>
                <td colSpan={5} style={{ textAlign: 'center', color: '#8b8f96', padding: '1.5rem' }}>
                  {t.empty}
                </td>
              </tr>
            ) : (
              rows.map((r) => (
                <tr key={`${title}-${r.symbol}`}>
                  <td>
                    <span className="vergex-trending__rank">{r.rank}</span>
                  </td>
                  <td>
                    <AssetNameCell row={r} tab={tab} />
                  </td>
                  <td>{fmtPrice(r.price, lang)}</td>
                  <td className={r.up ? 'vergex-trending__up' : 'vergex-trending__down'}>
                    {fmtPct(r.change_pct)}
                  </td>
                  <td className={positive ? 'vergex-trending__up' : 'vergex-trending__down'}>
                    {flowDisplay(r, positive)}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function PriceTable({
  rows,
  tab,
  lang,
  cols,
}: {
  rows: MarketTableRow[]
  tab: PrimaryTab
  lang: Language
  cols: { price: string; change: string; extra?: string }
}) {
  const colSpan = tab === 'stocks' ? 3 : cols.extra ? 5 : 4
  return (
    <div className="vergex-trending__table-wrap">
      <table className="vergex-trending__table">
        <thead>
          <tr>
            <th>{t.col.rank}</th>
            <th>{tab === 'stocks' ? t.col.asset : t.col.name}</th>
            {tab === 'stocks' ? (
              <th>{t.col.marketCap}</th>
            ) : (
              <>
                <th>{cols.price}</th>
                <th>{cols.change}</th>
                {cols.extra ? <th>{cols.extra}</th> : null}
              </>
            )}
          </tr>
        </thead>
        <tbody>
          {rows.length === 0 ? (
            <tr>
              <td colSpan={colSpan} style={{ textAlign: 'center', color: '#8b8f96', padding: '1.5rem' }}>
                {t.empty}
              </td>
            </tr>
          ) : (
            rows.map((r) => (
              <tr key={r.symbol}>
                <td>
                  <span className="vergex-trending__rank">{r.rank}</span>
                </td>
                <td>
                  <AssetNameCell row={r} tab={tab} />
                </td>
                {tab === 'stocks' ? (
                  <td style={{ color: '#8b8f96' }}>—</td>
                ) : (
                  <>
                    <td>{fmtPrice(r.price, lang)}</td>
                    <td className={r.up ? 'vergex-trending__up' : 'vergex-trending__down'}>
                      {fmtPct(r.change_pct)}
                    </td>
                    {cols.extra ? <td style={{ color: '#8b8f96' }}>{r.signal || r.volume_str || '—'}</td> : null}
                  </>
                )}
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  )
}

function TrendingSkeleton() {
  return (
    <div className="vergex-trending vergex-trending--skeleton">
      <div className="vergex-trending__skel-ticker" />
      <section className="vergex-trending__section">
        <div className="vergex-trending__skel-title" />
        <div className="vergex-trending__skel-hot-row" />
        <div className="vergex-trending__skel-tabs" />
        <div className="vergex-trending__skel-table" />
      </section>
    </div>
  )
}

export interface TrendingVergexLayoutProps {
  language: Language
  data?: MarketBoardPayload
  error?: unknown
  isLoading: boolean
  isHydrating?: boolean
  isValidating: boolean
  onRefresh: () => void
  onSwitchBoard: () => void
}

export function TrendingVergexLayout({
  language,
  data,
  error,
  isLoading,
  isHydrating = false,
  isValidating,
  onRefresh,
  onSwitchBoard,
}: TrendingVergexLayoutProps) {
  const [primary, setPrimary] = useState<PrimaryTab>('crypto')
  const [cryptoMetric, setCryptoMetric] = useState<CryptoMetric>('net-flow')
  const [stockFilter, setStockFilter] = useState<StockFilter>('all')
  const [q, setQ] = useState('')

  const hotCards = useMemo(() => {
    if (!data?.cards) return []
    const byId = Object.fromEntries(data.cards.map((c) => [c.id, c]))
    return CARD_IDS.map((id) => byId[id]).filter(Boolean) as MarketCardBlock[]
  }, [data])

  const baseRows = useMemo(() => {
    if (!data?.table_rows) return []
    if (primary === 'crypto') {
      if (cryptoMetric === 'oi' || cryptoMetric === 'rates' || cryptoMetric === 'depth') {
        return data.table_rows.derivatives ?? []
      }
      return data.table_rows.crypto ?? []
    }
    return data.table_rows[tabToRowsKey(primary)] ?? []
  }, [data, primary, cryptoMetric])

  const filteredRows = useMemo(() => {
    let list = primary === 'stocks' ? filterStockRows(baseRows, stockFilter) : [...baseRows]
    const needle = q.trim().toLowerCase()
    if (needle) {
      list = list.filter(
        (r) => r.symbol.toLowerCase().includes(needle) || r.name.toLowerCase().includes(needle),
      )
    }
    return list.map((r, i) => ({ ...r, rank: i + 1 }))
  }, [baseRows, primary, stockFilter, q])

  const { up: inflowRows, down: outflowRows } = useMemo(
    () => splitInflowOutflow(filteredRows, 20),
    [filteredRows],
  )

  const tickerDup = data?.ticker ? [...data.ticker, ...data.ticker] : []

  if (isLoading && !data) {
    return <TrendingSkeleton />
  }

  if (error && !data) {
    return (
      <div className="vergex-trending">
        <div className="vergex-trending__error">{t.error}</div>
      </div>
    )
  }

  if (!data) return null

  return (
    <div className="vergex-trending">
      <div className="vergex-trending__mode-bar">
        <button
          type="button"
          onClick={onSwitchBoard}
          className="vergex-trending__filter-pill vergex-trending__filter-pill--active"
        >
          {t.viewBoard}
        </button>
        <button
          type="button"
          onClick={onRefresh}
          disabled={isValidating}
          className="vergex-trending__filter-pill"
          style={{ marginLeft: '0.5rem' }}
        >
          {isValidating ? (
            <Loader2 className="inline h-3.5 w-3.5 animate-spin" />
          ) : (
            <RefreshCw className="inline h-3.5 w-3.5" />
          )}
          <span className="ml-1">{t.refresh}</span>
        </button>
      </div>

      {tickerDup.length > 0 ? (
        <div className="vergex-trending__ticker-wrap">
          <div className="vergex-trending__ticker-track">
            {tickerDup.map((item, i) => (
              <div key={`${item.symbol}-${i}`} className="vergex-trending__ticker-item">
                <span className="vergex-trending__ticker-sym">{item.symbol}</span>
                <span className="vergex-trending__ticker-price">{fmtPrice(item.price, language)}</span>
                <span className={item.up ? 'vergex-trending__up' : 'vergex-trending__down'}>
                  {fmtPct(item.change_pct)}
                </span>
              </div>
            ))}
          </div>
        </div>
      ) : null}

      <section className="vergex-trending__section">
        <h2 className="vergex-trending__hot-title">{t.hotMarkets}</h2>
        <div className="vergex-trending__hot-scroll">
          {hotCards.map((card) => (
            <HotCard key={card.id} card={card} lang={language} labels={t.cardTitles} />
          ))}
        </div>

        <nav className="vergex-trending__primary-tabs" aria-label={t.marketsNav}>
          {PRIMARY_ORDER.map((k) => (
            <button
              key={k}
              type="button"
              className={`vergex-trending__primary-tab ${primary === k ? 'vergex-trending__primary-tab--active' : ''}`}
              onClick={() => {
                setPrimary(k)
                setStockFilter('all')
                if (k === 'crypto') setCryptoMetric('net-flow')
              }}
            >
              {t.primary[k]}
            </button>
          ))}
        </nav>

        <div className="vergex-trending__sub-row">
          {primary === 'crypto' ? (
            <>
              {CRYPTO_METRICS.map((m) => (
                <button
                  key={m}
                  type="button"
                  className={`vergex-trending__metric-pill ${cryptoMetric === m ? 'vergex-trending__metric-pill--active' : ''}`}
                  onClick={() => setCryptoMetric(m)}
                >
                  {t.cryptoMetric[m]}
                </button>
              ))}
              <select className="vergex-trending__time-select" defaultValue="24h" aria-label={t.period}>
                <option value="24h">24h</option>
              </select>
            </>
          ) : primary === 'stocks' ? (
            STOCK_FILTERS.map((f) => (
              <button
                key={f}
                type="button"
                className={`vergex-trending__filter-pill ${stockFilter === f ? 'vergex-trending__filter-pill--active' : ''}`}
                onClick={() => setStockFilter(f)}
              >
                {t.filters[f]}
              </button>
            ))
          ) : null}

          <div className="vergex-trending__search-wrap" style={{ marginLeft: primary === 'crypto' || primary === 'stocks' ? undefined : 'auto' }}>
            <Search
              className="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2"
              style={{ color: '#8b8f96' }}
            />
            <input
              className="vergex-trending__search"
              placeholder={t.search}
              value={q}
              onChange={(e) => setQ(e.target.value)}
            />
          </div>
        </div>

        {primary === 'crypto' && cryptoMetric === 'net-flow' ? (
          <>
            <p style={{ fontSize: '0.6875rem', color: '#8b8f96', margin: '0 0 1rem' }}>{t.flowNote}</p>
            {isHydrating ? (
              <p className="vergex-trending__hydrate-hint">{t.hydrating}</p>
            ) : null}
            <div className="vergex-trending__dual">
              <FlowPanel
                title={t.topInflow}
                titleClass="vergex-trending__panel-title--up"
                rows={inflowRows}
                tab="crypto"
                lang={language}
                positive
              />
              <FlowPanel
                title={t.topOutflow}
                titleClass="vergex-trending__panel-title--down"
                rows={outflowRows}
                tab="crypto"
                lang={language}
                positive={false}
              />
            </div>
          </>
        ) : (
          <PriceTable
            rows={filteredRows}
            tab={primary}
            lang={language}
            cols={{
              price: t.col.price,
              change: t.col.change,
              extra:
                primary === 'crypto' && (cryptoMetric === 'oi' || cryptoMetric === 'rates')
                  ? cryptoMetric === 'oi'
                    ? t.col.openInterest
                    : t.col.funding
                  : undefined,
            }}
          />
        )}
      </section>

      {data.disclaimer ? <p className="vergex-trending__disclaimer">{data.disclaimer}</p> : null}
    </div>
  )
}
