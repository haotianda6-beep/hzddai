import { useMemo, useState } from 'react'
import useSWR from 'swr'
import {
  Activity,
  AlertTriangle,
  Clock3,
  ExternalLink,
  Filter,
  Loader2,
  LockKeyhole,
  Newspaper,
  Radio,
  RefreshCw,
  Search,
  ShieldCheck,
  Signal,
  X,
} from 'lucide-react'
import { useAuth } from '../contexts/AuthContext'
import { useLanguage } from '../contexts/LanguageContext'
import {
  GoldImpactDetail,
  GoldImpactInline,
  GoldImpactRadar,
} from '../components/common/GoldImpactPanels'
import { GoldMarketOverview } from '../components/common/GoldMarketOverview'
import { dataApi } from '../lib/api/data'
import type { CryptoNewsItem, CryptoNewsPayload } from '../types'

const ALLOWED_EMAIL = 'haotianda6@gmail.com'
const TRACKED_TOPICS = [
  'BTC',
  'ETH',
  'SOL',
  'ETF',
  'SEC',
  'Binance',
  'Hack',
  'Fed',
] as const

type FilterKey = 'all' | 'gold' | 'risk' | 'macro' | 'exchange' | 'onchain'

const FILTERS: Array<{ id: FilterKey; label: string }> = [
  { id: 'all', label: '全部情报' },
  { id: 'gold', label: '黄金影响' },
  { id: 'risk', label: '风险事件' },
  { id: 'macro', label: '宏观监管' },
  { id: 'exchange', label: '交易所动态' },
  { id: 'onchain', label: '链上安全' },
]

function itemText(item: CryptoNewsItem) {
  return `${item.title_zh || item.title} ${item.summary_zh || item.summary} ${(item.tags || []).join(' ')}`.toLowerCase()
}

function matchesFilter(item: CryptoNewsItem, filter: FilterKey) {
  if (filter === 'all') return true
  if (filter === 'gold') return (item.gold_impact?.risk_score ?? 0) >= 25
  if (filter === 'risk') return item.importance === 'risk'
  const text = itemText(item)
  if (filter === 'macro')
    return /\betf\b|\bsec\b|\bfed\b|监管|美联储|利率|政策/.test(text)
  if (filter === 'exchange')
    return /binance|okx|gate|bybit|coinbase|kraken|交易所|上币|下架/.test(text)
  return /hack|exploit|defi|链上|黑客|攻击|漏洞|被盗/.test(text)
}

function formatTime(value: string, locale: string) {
  if (!value) return '时间未知'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '时间未知'
  return date.toLocaleString(locale, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function DevelopmentView() {
  return (
    <div className="min-h-[calc(100vh-64px)] bg-[#07080a] px-3 py-6 text-[#e8e8ec] sm:px-6 sm:py-10">
      <div className="mx-auto max-w-[1280px] overflow-hidden rounded-lg border border-white/10 bg-[#101114] shadow-[0_24px_90px_rgba(0,0,0,0.45)]">
        <div className="border-b border-white/[0.08] px-5 py-10 sm:px-10 sm:py-14">
          <div className="mb-5 inline-flex items-center gap-2 rounded-full border border-[#d4ff33]/25 bg-[#d4ff33]/10 px-3 py-1 text-xs font-bold text-[#d4ff33]">
            <LockKeyhole className="h-3.5 w-3.5" /> 内测模块
          </div>
          <h1 className="text-3xl font-black text-white sm:text-5xl">
            新闻信息监控
          </h1>
          <p className="mt-4 max-w-2xl text-sm leading-7 text-[#9d9daa]">
            正在开发中，后续上线。该板块将提供多源新闻聚合、重要事件识别与风险信息监控。
          </p>
        </div>
        <div className="grid gap-px bg-white/[0.08] sm:grid-cols-3">
          {[
            ['多源信息聚合', '自动汇总市场、监管与交易所动态'],
            ['风险事件识别', '识别安全、清算与政策风险信号'],
            ['主题持续监控', '按币种和事件类型追踪最新变化'],
          ].map(([title, description]) => (
            <div key={title} className="bg-[#101114] px-5 py-6">
              <Signal className="mb-4 h-5 w-5 text-[#d4ff33]" />
              <h2 className="text-sm font-bold text-white">{title}</h2>
              <p className="mt-2 text-xs leading-6 text-[#81818d]">
                {description}
              </p>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

export function CryptoNewsPage() {
  const { user } = useAuth()
  const { language } = useLanguage()
  const [selected, setSelected] = useState<CryptoNewsItem | null>(null)
  const [activeFilter, setActiveFilter] = useState<FilterKey>('all')
  const [query, setQuery] = useState('')
  const canUseInternalPage = user?.email?.trim().toLowerCase() === ALLOWED_EMAIL
  const locale = language === 'zh' ? 'zh-CN' : 'en-US'
  const { data, error, isLoading, isValidating, mutate } =
    useSWR<CryptoNewsPayload>(
      canUseInternalPage ? 'news-monitor-page' : null,
      () => dataApi.getCryptoNews(true),
      {
        refreshInterval: 60_000,
        revalidateOnFocus: true,
        keepPreviousData: true,
      }
    )

  const items = useMemo(
    () =>
      (data?.items ?? []).filter(
        (item) => item && (item.title_zh || item.title) && item.url
      ),
    [data?.items]
  )
  const sources = useMemo(
    () => (data?.sources ?? []).filter((source) => source?.name),
    [data?.sources]
  )
  const visibleItems = useMemo(() => {
    const needle = query.trim().toLowerCase()
    return items.filter(
      (item) =>
        matchesFilter(item, activeFilter) &&
        (!needle || itemText(item).includes(needle))
    )
  }, [activeFilter, items, query])
  const riskItems = useMemo(
    () => items.filter((item) => item.importance === 'risk'),
    [items]
  )
  const goldImpactItems = useMemo(
    () =>
      items
        .filter((item) => (item.gold_impact?.risk_score ?? 0) >= 25)
        .sort(
          (a, b) =>
            (b.gold_impact?.risk_score ?? 0) - (a.gold_impact?.risk_score ?? 0)
        ),
    [items]
  )
  const highGoldRiskCount = goldImpactItems.filter(
    (item) => (item.gold_impact?.risk_score ?? 0) >= 70
  ).length
  const healthySources = sources.filter(
    (source) => source.status === 'ok'
  ).length
  const topicCounts = useMemo(
    () =>
      Object.fromEntries(
        TRACKED_TOPICS.map((topic) => [
          topic,
          items.filter((item) => itemText(item).includes(topic.toLowerCase()))
            .length,
        ])
      ),
    [items]
  ) as Record<(typeof TRACKED_TOPICS)[number], number>

  if (!canUseInternalPage) return <DevelopmentView />

  return (
    <div className="min-h-[calc(100vh-64px)] bg-[#07080a] px-3 py-5 text-[#f0f1f3] sm:px-5 sm:py-7">
      <div className="pointer-events-none fixed inset-0 bg-[linear-gradient(rgba(212,255,51,0.025)_1px,transparent_1px),linear-gradient(90deg,rgba(212,255,51,0.025)_1px,transparent_1px)] bg-[size:48px_48px]" />
      <div className="relative mx-auto max-w-[1480px]">
        <header className="mb-5 border-b border-white/10 pb-5">
          <div className="flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
            <div>
              <div className="mb-3 flex flex-wrap items-center gap-2 text-[11px] font-bold uppercase tracking-[0.14em] text-[#d4ff33]">
                <Radio className="h-4 w-4" /> Intelligence Monitor
                <span className="inline-flex items-center gap-1.5 rounded-full border border-emerald-400/20 bg-emerald-400/[0.08] px-2 py-1 normal-case tracking-normal text-emerald-300">
                  <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-emerald-300" />{' '}
                  实时运行
                </span>
              </div>
              <h1 className="text-3xl font-black text-white sm:text-4xl">
                新闻信息监控
              </h1>
              <p className="mt-2 max-w-2xl text-sm leading-6 text-[#8f949e]">
                聚合市场、宏观、交易所与链上信息，自动去重并突出高风险事件。
              </p>
            </div>
            <div className="grid grid-cols-2 gap-px overflow-hidden rounded-lg border border-white/10 bg-white/10 sm:grid-cols-4">
              {[
                ['情报总数', items.length, 'text-white'],
                ['黄金相关', goldImpactItems.length, 'text-amber-300'],
                ['高风险信号', highGoldRiskCount, 'text-red-300'],
                [
                  '在线信源',
                  `${healthySources}/${sources.length || 0}`,
                  'text-emerald-300',
                ],
              ].map(([label, value, tone]) => (
                <div
                  key={String(label)}
                  className="min-w-[112px] bg-[#101216] px-4 py-3"
                >
                  <p className="text-[10px] text-[#737984]">{label}</p>
                  <p className={`mt-1 font-mono text-lg font-bold ${tone}`}>
                    {value}
                  </p>
                </div>
              ))}
            </div>
          </div>
        </header>

        <div className="mb-4 flex flex-col gap-3 rounded-lg border border-white/10 bg-[#0d0f12]/95 p-3 lg:flex-row lg:items-center">
          <div className="relative min-w-0 flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[#737984]" />
            <input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="搜索币种、机构、事件或关键词"
              className="h-10 w-full rounded-md border border-white/10 bg-black/25 pl-10 pr-3 text-sm text-white outline-none transition-colors placeholder:text-[#5f646e] focus:border-[#d4ff33]/45"
            />
          </div>
          <div className="flex min-w-0 gap-1 overflow-x-auto pb-1 lg:pb-0">
            {FILTERS.map((filter) => (
              <button
                key={filter.id}
                type="button"
                onClick={() => setActiveFilter(filter.id)}
                className={`h-10 shrink-0 rounded-md px-3 text-xs font-bold transition-colors ${
                  activeFilter === filter.id
                    ? 'bg-[#d4ff33] text-black'
                    : 'border border-white/10 bg-white/[0.03] text-[#989da7] hover:border-[#d4ff33]/30 hover:text-white'
                }`}
              >
                {filter.label}
              </button>
            ))}
          </div>
          <button
            type="button"
            onClick={() => void mutate()}
            disabled={isValidating}
            title="立即刷新"
            className="inline-flex h-10 shrink-0 items-center justify-center gap-2 rounded-md border border-[#d4ff33]/25 bg-[#d4ff33]/[0.08] px-3 text-xs font-bold text-[#d4ff33] hover:bg-[#d4ff33]/[0.12] disabled:opacity-50"
          >
            {isValidating ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <RefreshCw className="h-4 w-4" />
            )}
            刷新
          </button>
        </div>

        <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_340px]">
          <section className="min-w-0 overflow-hidden rounded-lg border border-white/10 bg-[#0d0f12]/95">
            <div className="flex items-center justify-between border-b border-white/[0.08] px-4 py-3">
              <div className="flex items-center gap-2">
                <Newspaper className="h-4 w-4 text-[#d4ff33]" />
                <h2 className="text-sm font-bold text-white">实时信息流</h2>
              </div>
              <span className="font-mono text-[11px] text-[#727883]">
                {visibleItems.length} 条
              </span>
            </div>

            {isLoading && !data ? (
              <div className="flex min-h-[420px] items-center justify-center text-sm text-[#8f949e]">
                <Loader2 className="mr-2 h-5 w-5 animate-spin text-[#d4ff33]" />{' '}
                正在连接信息源
              </div>
            ) : error ? (
              <div className="m-4 rounded-md border border-red-500/25 bg-red-500/[0.08] px-4 py-10 text-center text-sm text-red-300">
                新闻信息加载失败，请稍后重试
              </div>
            ) : visibleItems.length ? (
              <div className="divide-y divide-white/[0.07]">
                {visibleItems.map((item) => (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => setSelected(item)}
                    className="group grid w-full gap-3 px-4 py-4 text-left transition-colors hover:bg-white/[0.025] sm:grid-cols-[116px_minmax(0,1fr)_auto] sm:items-start"
                  >
                    <div className="flex items-center gap-2 sm:block">
                      <span
                        className={`inline-flex rounded px-2 py-1 text-[10px] font-bold ${item.importance === 'risk' ? 'bg-red-500/[0.12] text-red-300' : 'bg-cyan-400/10 text-cyan-300'}`}
                      >
                        {item.source || 'News'}
                      </span>
                      <p className="mt-0 font-mono text-[10px] text-[#656b75] sm:mt-2">
                        {formatTime(item.published_at, locale)}
                      </p>
                    </div>
                    <div className="min-w-0">
                      <h3 className="text-sm font-bold leading-6 text-[#edf0f3] transition-colors group-hover:text-[#d4ff33]">
                        {item.title_zh || item.title}
                      </h3>
                      {item.summary_zh || item.summary ? (
                        <p className="mt-1 line-clamp-2 text-xs leading-5 text-[#858b95]">
                          {item.summary_zh || item.summary}
                        </p>
                      ) : null}
                      <div className="mt-2 flex flex-wrap gap-1.5">
                        {(item.tags ?? []).slice(0, 5).map((tag) => (
                          <span
                            key={tag}
                            className="rounded border border-white/[0.08] px-1.5 py-0.5 font-mono text-[9px] text-[#777d87]"
                          >
                            {tag}
                          </span>
                        ))}
                      </div>
                      <GoldImpactInline impact={item.gold_impact} />
                    </div>
                    <ExternalLink className="hidden h-4 w-4 text-[#555b65] transition-colors group-hover:text-[#d4ff33] sm:block" />
                  </button>
                ))}
              </div>
            ) : (
              <div className="flex min-h-[320px] flex-col items-center justify-center px-4 text-center text-sm text-[#7f858f]">
                <Filter className="mb-3 h-6 w-6 text-[#d4ff33]/70" />{' '}
                当前筛选条件下暂无信息
              </div>
            )}
          </section>

          <aside className="space-y-4">
            <GoldMarketOverview market={data?.gold_market} />
            <GoldImpactRadar
              items={goldImpactItems}
              onSelect={setSelected}
              formatTime={(value) => formatTime(value, locale)}
            />
            <section className="overflow-hidden rounded-lg border border-red-400/[0.15] bg-[#0d0f12]/95">
              <div className="flex items-center justify-between border-b border-white/[0.08] px-4 py-3">
                <div className="flex items-center gap-2 text-sm font-bold text-white">
                  <AlertTriangle className="h-4 w-4 text-red-300" /> 风险雷达
                </div>
                <span className="font-mono text-[10px] text-red-300">
                  {riskItems.length} ACTIVE
                </span>
              </div>
              <div className="divide-y divide-white/[0.07]">
                {riskItems.slice(0, 5).map((item) => (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => setSelected(item)}
                    className="w-full px-4 py-3 text-left hover:bg-red-500/[0.035]"
                  >
                    <p className="line-clamp-2 text-xs font-semibold leading-5 text-[#d8dbe0]">
                      {item.title_zh || item.title}
                    </p>
                    <p className="mt-1 font-mono text-[9px] text-[#686e78]">
                      {item.source} · {formatTime(item.published_at, locale)}
                    </p>
                  </button>
                ))}
                {!riskItems.length ? (
                  <p className="px-4 py-6 text-center text-xs text-[#6f757f]">
                    当前未识别到高风险事件
                  </p>
                ) : null}
              </div>
            </section>

            <section className="rounded-lg border border-white/10 bg-[#0d0f12]/95 p-4">
              <div className="mb-4 flex items-center gap-2 text-sm font-bold text-white">
                <Activity className="h-4 w-4 text-cyan-300" /> 监控主题
              </div>
              <div className="grid grid-cols-2 gap-2">
                {TRACKED_TOPICS.map((topic) => (
                  <button
                    key={topic}
                    type="button"
                    onClick={() => {
                      setQuery(topic)
                      setActiveFilter('all')
                    }}
                    className="flex items-center justify-between rounded-md border border-white/[0.08] bg-white/[0.025] px-3 py-2 text-left hover:border-cyan-300/25"
                  >
                    <span className="font-mono text-[11px] text-[#a7acb5]">
                      {topic}
                    </span>
                    <span className="font-mono text-xs font-bold text-cyan-300">
                      {topicCounts[topic]}
                    </span>
                  </button>
                ))}
              </div>
            </section>

            <section className="rounded-lg border border-white/10 bg-[#0d0f12]/95 p-4">
              <div className="mb-3 flex items-center justify-between">
                <div className="flex items-center gap-2 text-sm font-bold text-white">
                  <ShieldCheck className="h-4 w-4 text-emerald-300" /> 信源状态
                </div>
                <span className="font-mono text-[9px] text-[#656b75]">
                  AUTO CHECK
                </span>
              </div>
              <div className="space-y-2">
                {sources.map((source) => (
                  <div
                    key={source.name}
                    title={source.detail || source.name}
                    className="flex items-center justify-between gap-3 text-xs"
                  >
                    <span className="truncate text-[#8f959f]">
                      {source.name}
                    </span>
                    <span
                      className={`shrink-0 font-mono text-[9px] ${source.status === 'ok' ? 'text-emerald-300' : 'text-amber-300'}`}
                    >
                      {source.status === 'ok' ? 'ONLINE' : 'DELAYED'}
                    </span>
                  </div>
                ))}
              </div>
              {data?.updated_at ? (
                <div className="mt-4 flex items-center gap-1.5 border-t border-white/[0.08] pt-3 font-mono text-[9px] text-[#626873]">
                  <Clock3 className="h-3 w-3" /> 最近更新{' '}
                  {formatTime(data.updated_at, locale)}
                </div>
              ) : null}
            </section>
          </aside>
        </div>
      </div>

      {selected ? (
        <div className="fixed inset-0 z-[80] flex items-start justify-center overflow-y-auto bg-black/80 p-3 backdrop-blur-sm sm:items-center sm:p-4">
          <div className="max-h-[88vh] w-full max-w-2xl overflow-y-auto rounded-lg border border-white/[0.12] bg-[#0d0f12] shadow-2xl">
            <div className="flex items-start justify-between gap-4 border-b border-white/[0.08] p-4 sm:p-5">
              <div className="min-w-0">
                <div className="mb-2 flex flex-wrap items-center gap-2">
                  <span
                    className={`rounded px-2 py-1 text-[10px] font-bold ${selected.importance === 'risk' ? 'bg-red-500/[0.12] text-red-300' : 'bg-cyan-400/10 text-cyan-300'}`}
                  >
                    {selected.source || 'News'}
                  </span>
                  <span className="font-mono text-[10px] text-[#686e78]">
                    {formatTime(selected.published_at, locale)}
                  </span>
                </div>
                <h2 className="text-lg font-bold leading-7 text-white sm:text-xl">
                  {selected.title_zh || selected.title}
                </h2>
              </div>
              <button
                type="button"
                onClick={() => setSelected(null)}
                title="关闭"
                className="shrink-0 rounded-md p-2 text-[#777d87] hover:bg-white/[0.08] hover:text-white"
              >
                <X className="h-5 w-5" />
              </button>
            </div>
            <div className="p-4 sm:p-5">
              <p className="text-sm leading-7 text-[#c5c9d0]">
                {selected.summary_zh ||
                  selected.summary ||
                  '该信息源未提供摘要，请查看原文了解完整内容。'}
              </p>
              <GoldImpactDetail impact={selected.gold_impact} />
              <div className="mt-4 flex flex-wrap gap-1.5">
                {(selected.tags ?? []).map((tag) => (
                  <span
                    key={tag}
                    className="rounded border border-white/10 px-2 py-1 font-mono text-[10px] text-[#8c929c]"
                  >
                    {tag}
                  </span>
                ))}
              </div>
              <div className="mt-6 flex justify-end">
                <a
                  href={selected.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-2 rounded-md bg-[#d4ff33] px-4 py-2 text-sm font-bold text-black hover:bg-[#e0ff67]"
                >
                  查看原始信息 <ExternalLink className="h-4 w-4" />
                </a>
              </div>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  )
}

export default CryptoNewsPage
