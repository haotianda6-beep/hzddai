import { Activity } from 'lucide-react'
import type { GoldMarketQuote, GoldMarketStatus } from '../../types'

function formatChange(value?: number, suffix = '%') {
  if (value === undefined || Number.isNaN(value)) return '--'
  return `${value >= 0 ? '+' : ''}${value.toFixed(suffix === '%' ? 3 : 1)}${suffix}`
}

function changeTone(value?: number) {
  if (value === undefined || Math.abs(value) < 0.0001) return 'text-[#777d87]'
  return value > 0 ? 'text-emerald-300' : 'text-rose-300'
}

function QuoteWindows({ quote }: { quote?: GoldMarketQuote }) {
  return (
    <div className="mt-3 grid grid-cols-3 gap-2 font-mono text-[9px]">
      {[
        ['5M', quote?.change_5m],
        ['15M', quote?.change_15m],
        ['60M', quote?.change_60m],
      ].map(([label, value]) => (
        <div key={String(label)}>
          <p className="text-[#606671]">{label}</p>
          <p
            className={`mt-1 font-bold ${changeTone(value as number | undefined)}`}
          >
            {formatChange(value as number | undefined)}
          </p>
        </div>
      ))}
    </div>
  )
}

export function GoldMarketOverview({ market }: { market?: GoldMarketStatus }) {
  const status = market?.status ?? 'offline'
  const statusLabel =
    status === 'live' ? '实时' : status === 'warming_up' ? '预热中' : '离线'
  const statusTone =
    status === 'live'
      ? 'text-emerald-300'
      : status === 'warming_up'
        ? 'text-amber-300'
        : 'text-rose-300'

  return (
    <section className="overflow-hidden rounded-lg border border-cyan-300/[0.16] bg-[#0d0f12]/95">
      <div className="flex items-center justify-between border-b border-white/[0.08] px-4 py-3">
        <div className="flex items-center gap-2 text-sm font-bold text-white">
          <Activity className="h-4 w-4 text-cyan-300" /> 黄金联动行情
        </div>
        <span className={`font-mono text-[10px] font-bold ${statusTone}`}>
          {statusLabel}
        </span>
      </div>
      <div className="grid grid-cols-2 gap-px bg-white/[0.08]">
        {[market?.xau, market?.dollar].map((quote, index) => (
          <div key={quote?.symbol || index} className="bg-[#0d0f12] p-4">
            <p className="font-mono text-[10px] text-[#747a84]">
              {quote?.symbol || (index === 0 ? 'XAUUSDc' : 'DOLLAR')}
            </p>
            <p className="mt-2 font-mono text-lg font-bold text-white">
              {quote?.price?.toFixed(index === 0 ? 2 : 3) || '--'}
            </p>
            <QuoteWindows quote={quote} />
          </div>
        ))}
      </div>
      <div className="flex items-end justify-between gap-4 border-t border-white/[0.08] px-4 py-3">
        <div>
          <p className="text-[10px] text-[#747a84]">美国 10 年期国债收益率</p>
          <p className="mt-1 font-mono text-sm font-bold text-white">
            {market?.us10y?.value !== undefined
              ? `${market.us10y.value.toFixed(2)}%`
              : '--'}
          </p>
        </div>
        <div className="text-right">
          <p
            className={`font-mono text-xs font-bold ${changeTone(market?.us10y?.change_bp)}`}
          >
            {formatChange(market?.us10y?.change_bp, 'bp')}
          </p>
          <p className="mt-1 text-[9px] text-[#606671]">美国财政部 · 日频</p>
        </div>
      </div>
      {market?.updated_at ? (
        <p className="border-t border-white/[0.06] px-4 py-2 font-mono text-[9px] text-[#5f6570]">
          MT4 最近上报 {new Date(market.updated_at).toLocaleString('zh-CN')}
        </p>
      ) : null}
    </section>
  )
}
