import { useMemo, useState } from 'react'
import useSWR from 'swr'
import {
  Activity,
  Database,
  Loader2,
  RefreshCw,
  Search,
  ShieldCheck,
  Sparkles,
  Zap,
} from 'lucide-react'
import { dataApi } from '../../lib/api/data'
import type { Language } from '../../i18n/translations'
import type { MarketBoardPayload, MarketTableRow } from '../../types'
import { TrendingVergexLayout } from './TrendingVergexLayout'

const fontStack =
  "Inter, 'Space Grotesk', ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
const monoStack =
  "'Space Grotesk', 'SFMono-Regular', Consolas, ui-monospace, monospace"

const zh = {
  viewTrending: '趋势',
  viewBoard: '综合看板',
  hotMarkets: '热门市场',
  gainers: '涨幅榜',
  losers: '跌幅榜',
  title: '行情看板',
  titleTrending: '趋势',
  subtitle: '热门市场 · 多品类报价',
  subtitleTrending: '热门市场 · 多品类实时公开行情',
  updated: '更新于',
  refresh: '刷新',
  loading: '正在拉取公开行情…',
  error: '加载失败，请稍后重试',
  search: '搜索代码或名称…',
  tabs: {
    crypto: '加密货币',
    derivatives: '合约数据',
    forex: '外汇',
    stocks: '美股',
    indices: '指数',
    commodities: '大宗商品',
    prediction: '预测市场',
  },
  stockFilters: {
    all: '全部',
    tech: '科技股',
    ai: 'AI 链',
    active: '最活跃',
    mag7: 'Mag 7',
    gainers: '涨幅',
    losers: '跌幅',
  },
  trendingTabs: {
    stocks: '美股',
    indices: '指数',
    commodities: '大宗',
    crypto: '加密',
    prediction: '预测',
    forex: '外汇',
  },
  tableCols: ['#', '资产', '最新价', '涨跌幅', '成交额', '信号'],
  changeHint:
    '涨跌：加密/合约为 24h；外汇约 7 日；股/指/大宗为延迟快照开收估算；预测市场展示价格与成交额',
  signal: '信号',
}

const en = {
  viewTrending: 'Trending',
  viewBoard: 'Full board',
  hotMarkets: 'Hot Markets',
  gainers: 'Top gainers',
  losers: 'Top losers',
  title: 'Market Board',
  titleTrending: 'Trending',
  subtitle: 'Popular markets · multi-asset quotes',
  subtitleTrending: 'Hot markets · live public quotes',
  updated: 'Updated',
  refresh: 'Refresh',
  loading: 'Loading public quotes…',
  error: 'Failed to load. Retry later.',
  search: 'Search symbol or name…',
  tabs: {
    crypto: 'Crypto',
    derivatives: 'Derivatives',
    forex: 'Forex',
    stocks: 'US Stocks',
    indices: 'Indices',
    commodities: 'Commodities',
    prediction: 'Prediction',
  },
  stockFilters: {
    all: 'All',
    tech: 'Tech',
    ai: 'AI names',
    active: 'Most active',
    mag7: 'Mag 7',
    gainers: 'Gainers',
    losers: 'Losers',
  },
  trendingTabs: {
    stocks: 'US Stocks',
    indices: 'Indices',
    commodities: 'Commodities',
    crypto: 'Crypto',
    prediction: 'Prediction',
    forex: 'Forex',
  },
  tableCols: ['#', 'Asset', 'Last', 'Change', 'Volume', 'Signal'],
  changeHint:
    'Change: crypto/derivatives 24h; forex ~7d; stocks/indices/commodities delayed session approx.; prediction uses market price/volume.',
  signal: 'Signal',
}

type TabKey = keyof typeof zh.tabs
type ViewMode = 'trending' | 'board'
type RowFilter =
  | 'all'
  | 'tech'
  | 'ai'
  | 'active'
  | 'mag7'
  | 'gainers'
  | 'losers'

function fmtPrice(n: number, lang: Language): string {
  const loc = lang === 'zh' ? 'zh-CN' : 'en-US'
  if (!Number.isFinite(n) || n === 0) return '—'
  if (Math.abs(n) >= 1000)
    return n.toLocaleString(loc, { maximumFractionDigits: 2 })
  if (Math.abs(n) >= 1)
    return n.toLocaleString(loc, {
      minimumFractionDigits: 2,
      maximumFractionDigits: 4,
    })
  return n.toLocaleString(loc, { maximumFractionDigits: 6 })
}

function fmtPct(n: number): string {
  if (!Number.isFinite(n)) return '—'
  const s = n >= 0 ? `+${n.toFixed(2)}%` : `${n.toFixed(2)}%`
  return s
}

function toneClass(tone?: string): string {
  if (tone === 'up')
    return 'border-emerald-400/25 bg-emerald-500/10 text-emerald-200'
  if (tone === 'down') return 'border-red-400/25 bg-red-500/10 text-red-200'
  if (tone === 'warn')
    return 'border-amber-300/25 bg-amber-400/10 text-amber-100'
  return 'border-[#d4ff33]/20 bg-[#d4ff33]/[0.06] text-[#eaff7a]'
}

function sourceLabel(status: string | undefined, language: Language): string {
  if (language === 'zh') return status === 'ok' ? '正常' : '异常'
  return status === 'ok' ? 'OK' : 'Issue'
}

const techSymbols = new Set(
  [
    'AAPL.US',
    'MSFT.US',
    'NVDA.US',
    'GOOGL.US',
    'AMZN.US',
    'META.US',
    'TSM.US',
  ].map((s) => s.toUpperCase())
)

export function MarketBoardView({ language }: { language: Language }) {
  const t = language === 'zh' ? zh : en
  const [viewMode, setViewMode] = useState<ViewMode>('trending')
  const [tab, setTab] = useState<TabKey>('crypto')
  const [rowFilter, setRowFilter] = useState<RowFilter>('all')
  const [q, setQ] = useState('')

  const { data: quickData } = useSWR<MarketBoardPayload>(
    viewMode === 'trending' ? 'market-board-quick' : null,
    () => dataApi.getMarketBoard({ silent: true, quick: true }),
    { revalidateOnFocus: false, dedupingInterval: 15_000 }
  )

  const {
    data: fullData,
    error,
    isLoading,
    isValidating,
    mutate,
  } = useSWR<MarketBoardPayload>(
    'market-board',
    () => dataApi.getMarketBoard({ silent: true, quick: false }),
    {
      refreshInterval: 30_000,
      revalidateOnFocus: true,
      keepPreviousData: true,
      dedupingInterval: 10_000,
    }
  )

  const data = fullData ?? quickData

  const activeTab = tab

  const rows = useMemo(() => {
    if (!data?.table_rows) return []
    const raw = data.table_rows[activeTab] ?? []
    let list = raw
    const needle = q.trim().toLowerCase()
    if (needle) {
      list = list.filter(
        (r) =>
          r.symbol.toLowerCase().includes(needle) ||
          r.name.toLowerCase().includes(needle)
      )
    }
    if (viewMode === 'board' && tab === 'stocks') {
      if (rowFilter === 'tech') {
        list = list.filter((r) => techSymbols.has(r.symbol.toUpperCase()))
      }
      if (rowFilter === 'ai') {
        list = list.filter((r) => /nvda|meta|googl|msft/i.test(r.symbol))
      }
      if (rowFilter === 'active') {
        list = [...list].sort((a, b) => (b.volume ?? 0) - (a.volume ?? 0))
      }
    }
    return list
  }, [data, activeTab, tab, q, rowFilter, viewMode])

  if (viewMode === 'trending') {
    return (
      <TrendingVergexLayout
        language={language}
        data={data}
        error={error}
        isLoading={isLoading && !data}
        isHydrating={!fullData && !!quickData}
        isValidating={isValidating}
        onRefresh={() => void mutate()}
        onSwitchBoard={() => setViewMode('board')}
      />
    )
  }

  return (
    <div
      className="relative min-h-[calc(100vh-4rem)] w-full max-w-[1600px] overflow-hidden px-3 pb-10 pt-4 text-[#F4F6F8] md:px-6"
      style={{ fontFamily: fontStack }}
    >
      <div className="pointer-events-none absolute left-1/2 top-0 h-72 w-[42rem] -translate-x-1/2 rounded-full bg-[#d4ff33]/[0.07] blur-3xl" />
      <div className="relative mb-6 overflow-hidden rounded-[28px] border border-[#d4ff33]/18 bg-[radial-gradient(circle_at_top_left,rgba(212,255,51,0.12),transparent_34%),linear-gradient(135deg,rgba(10,13,10,0.98),rgba(3,5,7,0.99))] p-5 shadow-[0_24px_80px_rgba(0,0,0,0.58)] md:p-7">
        <div className="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
          <div>
            <div className="mb-3 inline-flex items-center gap-2 rounded-full border border-[#d4ff33]/20 bg-[#d4ff33]/10 px-3 py-1 text-[11px] font-bold text-[#d4ff33]">
              <Sparkles className="h-3.5 w-3.5" />
              {language === 'zh' ? '免费公开真实行情' : 'Free public live data'}
            </div>
            <div className="mb-4 inline-flex rounded-xl border border-[#1f2933] bg-[#0B0F14] p-1">
              <button
                type="button"
                onClick={() => setViewMode('trending')}
                className={`rounded-lg px-3 py-1.5 text-xs font-bold transition-colors ${
                  false
                    ? 'bg-nofx-gold text-black'
                    : 'text-[#A7B0BC] hover:text-[#F8FAFC]'
                }`}
              >
                {t.viewTrending}
              </button>
              <button
                type="button"
                onClick={() => setViewMode('board')}
                className={`rounded-lg px-3 py-1.5 text-xs font-bold transition-colors ${
                  viewMode === 'board'
                    ? 'bg-nofx-gold text-black'
                    : 'text-[#A7B0BC] hover:text-[#F8FAFC]'
                }`}
              >
                {t.viewBoard}
              </button>
            </div>
            <h1
              className="text-3xl font-black tracking-tight text-[#F8FAFC] md:text-5xl"
              style={{ fontFamily: monoStack }}
            >
              {t.title}
            </h1>
            <p className="mt-2 max-w-2xl text-sm font-medium leading-relaxed text-[#A7B0BC]">
              {language === 'zh'
                ? '聚合加密现货、合约资金费率/OI、外汇、美股、指数、大宗商品与预测市场。只接入当前可免费访问的公开接口。'
                : 'Aggregates crypto spot, futures funding/OI, FX, US stocks, indices, commodities and prediction markets from free public APIs.'}
            </p>
            {data?.updated_at ? (
              <p
                className="mt-3 text-[11px] font-medium text-[#7F8A99]"
                style={{ fontFamily: monoStack }}
              >
                {t.updated}:{' '}
                {new Date(data.updated_at).toLocaleString(
                  language === 'zh' ? 'zh-CN' : 'en-US'
                )}
              </p>
            ) : null}
          </div>
          <button
            type="button"
            onClick={() => void mutate()}
            disabled={isValidating}
            className="inline-flex w-full items-center justify-center gap-2 self-start rounded-2xl border border-[#d4ff33]/25 bg-[#d4ff33]/10 px-4 py-2.5 text-xs font-bold text-[#d4ff33] transition-colors hover:bg-[#d4ff33]/15 disabled:opacity-50 sm:w-auto"
          >
            {isValidating ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <RefreshCw className="h-4 w-4" />
            )}
            {t.refresh}
          </button>
        </div>
      </div>

      {isLoading && !data ? (
        <div className="flex flex-col items-center justify-center gap-3 py-24 font-medium text-[#A7B0BC]">
          <Loader2 className="h-10 w-10 animate-spin text-[#d4ff33]" />
          <p>{t.loading}</p>
        </div>
      ) : error ? (
        <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-8 text-center text-red-300">
          {t.error}
        </div>
      ) : data ? (
        <>
          {viewMode === 'board' && data.metrics && data.metrics.length > 0 ? (
            <div className="mb-5 grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-5">
              {data.metrics.map((m) => (
                <div
                  key={m.id}
                  className={`rounded-2xl border p-4 shadow-[0_12px_36px_rgba(0,0,0,0.22)] ${toneClass(m.tone)}`}
                >
                  <div className="mb-3 flex items-center justify-between gap-2">
                    <span className="text-[11px] font-bold uppercase tracking-wide opacity-75">
                      {language === 'zh' ? m.label_zh : m.label_en}
                    </span>
                    <Activity className="h-4 w-4 opacity-70" />
                  </div>
                  <div className="font-['Space_Grotesk',monospace] text-2xl font-black tabular-nums">
                    {m.value}
                  </div>
                  <div className="mt-2 text-[11px] leading-relaxed opacity-75">
                    {language === 'zh' ? m.detail_zh : m.detail_en}
                  </div>
                </div>
              ))}
            </div>
          ) : null}

          {viewMode === 'board' && data.sources && data.sources.length > 0 ? (
            <div className="mb-5 rounded-2xl border border-[#1f2933] bg-[#05070A]/80 p-3">
              <div className="mb-2 flex items-center gap-2 text-[11px] font-bold uppercase tracking-wide text-[#8D98A8]">
                <Database className="h-3.5 w-3.5 text-[#d4ff33]" />
                {language === 'zh' ? '真实数据源状态' : 'Real data sources'}
              </div>
              <div className="flex flex-wrap gap-2">
                {data.sources.map((s) => (
                  <span
                    key={s.name}
                    title={s.detail || s.name}
                    className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] ${
                      s.status === 'ok'
                        ? 'border-emerald-400/20 bg-emerald-500/10 text-emerald-300'
                        : 'border-amber-300/20 bg-amber-400/10 text-amber-200'
                    }`}
                  >
                    <ShieldCheck className="h-3 w-3" />
                    {s.name}
                    <span className="opacity-70">
                      · {sourceLabel(s.status, language)}
                    </span>
                  </span>
                ))}
              </div>
            </div>
          ) : null}

          {/* 顶部滚动行情 */}
          <div className="mb-6 overflow-hidden rounded-2xl border border-[#d4ff33]/15 bg-gradient-to-b from-[#070A0D] to-[#030506] shadow-inner">
            <div className="flex gap-0 overflow-x-auto pb-1 pt-1 [scrollbar-width:thin]">
              {data.ticker.map((item, i) => (
                <div
                  key={`${item.category}-${item.symbol}-${i}`}
                  className="flex min-w-[9.5rem] shrink-0 flex-col border-r border-[#2b3139]/80 px-3 py-2 last:border-r-0"
                >
                  <span className="text-[10px] font-bold uppercase tracking-wider text-[#9AA5B5]">
                    {item.symbol}
                  </span>
                  <span className="truncate text-[11px] font-medium text-[#B5BFCC]">
                    {item.name}
                  </span>
                  <span
                    className="mt-0.5 text-sm font-black text-[#F8FAFC]"
                    style={{ fontFamily: monoStack }}
                  >
                    {fmtPrice(item.price, language)}
                  </span>
                  <span
                    className={`font-mono text-xs font-semibold ${item.up ? 'text-emerald-400' : 'text-red-400'}`}
                  >
                    {fmtPct(item.change_pct)}
                  </span>
                </div>
              ))}
            </div>
          </div>
          <div className="mb-8 grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
            {data.cards.map((card) => (
              <div
                key={card.id}
                className="group rounded-2xl border border-[#1f2933] bg-[linear-gradient(180deg,rgba(8,11,10,0.98),rgba(4,6,8,0.99))] p-4 shadow-[0_0_22px_rgba(0,0,0,0.48)] transition-colors hover:border-[#d4ff33]/25"
              >
                <div className="mb-3 flex items-center gap-2 border-b border-[#2b3139]/80 pb-2">
                  <Zap className="h-4 w-4 text-[#d4ff33] opacity-80" />
                  <h3 className="text-sm font-extrabold text-[#F4F6F8]">
                    {language === 'zh' ? card.title_zh : card.title_en}
                  </h3>
                </div>
                <ul className="space-y-3">
                  {card.items.map((it, idx) => (
                    <li
                      key={`${card.id}-${idx}-${it.symbol}`}
                      className="flex items-start justify-between gap-2 text-sm"
                    >
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-2">
                          <span
                            className="truncate text-xs font-black text-[#F8FAFC]"
                            style={{ fontFamily: monoStack }}
                          >
                            {it.symbol}
                          </span>
                          <span className="truncate text-[11px] font-medium text-[#A7B0BC]">
                            {it.name}
                          </span>
                        </div>
                        {it.extra ? (
                          <p className="mt-0.5 truncate text-[10px] font-medium text-[#7F8A99]">
                            {it.extra}
                          </p>
                        ) : null}
                      </div>
                      <div className="shrink-0 text-right">
                        <div
                          className="text-sm font-bold text-[#F8FAFC]"
                          style={{ fontFamily: monoStack }}
                        >
                          {fmtPrice(it.price, language)}
                        </div>
                        <div
                          className={`font-mono text-xs ${it.up ? 'text-emerald-400' : 'text-red-400'}`}
                        >
                          {fmtPct(it.change_pct)}
                        </div>
                      </div>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>

          {/* 表格区 */}
          <div className="rounded-2xl border border-[#1f2933] bg-[#05070A]/90 p-3 shadow-[0_20px_60px_rgba(0,0,0,0.38)] md:p-4">
            <div className="mb-4 flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div className="flex flex-nowrap gap-1.5 overflow-x-auto [scrollbar-width:thin]">
                {(Object.keys(t.tabs) as TabKey[]).map((k) => (
                  <button
                    key={k}
                    type="button"
                    onClick={() => setTab(k)}
                    className={`shrink-0 rounded-lg px-3 py-1.5 text-xs font-semibold transition-colors ${
                      tab === k
                        ? 'bg-[#d4ff33] text-black'
                        : 'bg-[#0B0F14] text-[#A7B0BC] hover:text-[#F8FAFC]'
                    }`}
                  >
                    {t.tabs[k]}
                  </button>
                ))}
              </div>
              <div className="relative w-full max-w-md flex-1">
                <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-[#8D98A8]" />
                <input
                  value={q}
                  onChange={(e) => setQ(e.target.value)}
                  placeholder={t.search}
                  className="w-full rounded-lg border border-[#1f2933] bg-[#020304] py-2 pl-9 pr-3 text-sm font-medium text-[#F4F6F8] placeholder:text-[#7F8A99] focus:border-[#d4ff33]/40 focus:outline-none"
                />
              </div>
            </div>

            {tab === 'stocks' ? (
              <div className="mb-3 flex flex-nowrap gap-1.5 overflow-x-auto [scrollbar-width:thin]">
                {(['all', 'tech', 'ai', 'active'] as const).map((fk) => (
                  <button
                    key={fk}
                    type="button"
                    onClick={() => setRowFilter(fk)}
                    className={`shrink-0 rounded-md px-2.5 py-1 text-[11px] font-medium ${
                      rowFilter === fk
                        ? 'bg-white/10 text-[#d4ff33]'
                        : 'text-[#8D98A8] hover:text-[#D5DBE4]'
                    }`}
                  >
                    {t.stockFilters[fk]}
                  </button>
                ))}
              </div>
            ) : null}

            <p className="mb-2 text-[10px] font-medium text-[#7F8A99]">
              {t.changeHint}
            </p>

            <div className="overflow-x-auto rounded-lg border border-[#1f2933]">
              <table className="w-full min-w-[640px] border-collapse text-left text-sm">
                <thead>
                  <tr className="border-b border-[#1f2933] bg-[#080B10] text-[11px] uppercase tracking-wide text-[#9AA5B5]">
                    {t.tableCols.map((c) => (
                      <th key={c} className="px-3 py-2.5 font-semibold">
                        {c}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {rows.length === 0 ? (
                    <tr>
                      <td
                        colSpan={t.tableCols.length}
                        className="px-3 py-10 text-center text-zinc-500"
                      >
                        {language === 'zh' ? '无匹配数据' : 'No rows'}
                      </td>
                    </tr>
                  ) : (
                    rows.map((r: MarketTableRow) => (
                      <tr
                        key={`${r.symbol}-${r.rank}`}
                        className="border-b border-[#1f2933]/70 hover:bg-white/[0.04]"
                      >
                        <td
                          className="px-3 py-2.5 text-xs font-semibold text-[#8D98A8]"
                          style={{ fontFamily: monoStack }}
                        >
                          {r.rank}
                        </td>
                        <td className="px-3 py-2.5">
                          <div
                            className="text-xs font-black text-[#F8FAFC]"
                            style={{ fontFamily: monoStack }}
                          >
                            {r.symbol}
                          </div>
                          <div className="text-[11px] font-medium text-[#A7B0BC]">
                            {r.name}
                          </div>
                        </td>
                        <td
                          className="px-3 py-2.5 text-sm font-bold text-[#F8FAFC]"
                          style={{ fontFamily: monoStack }}
                        >
                          {fmtPrice(r.price, language)}
                        </td>
                        <td
                          className={`px-3 py-2.5 font-mono text-sm font-semibold ${r.up ? 'text-emerald-400' : 'text-red-400'}`}
                        >
                          {fmtPct(r.change_pct)}
                        </td>
                        <td
                          className="px-3 py-2.5 text-xs font-semibold text-[#B5BFCC]"
                          style={{ fontFamily: monoStack }}
                        >
                          {r.volume_str}
                        </td>
                        <td className="px-3 py-2.5 text-xs font-medium text-[#8D98A8]">
                          {r.signal}
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>

          <p className="mt-6 text-center text-[11px] font-medium leading-relaxed text-[#7F8A99]">
            {data.disclaimer}
          </p>
        </>
      ) : null}
    </div>
  )
}
