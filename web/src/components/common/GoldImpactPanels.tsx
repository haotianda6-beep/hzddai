import {
  AlertTriangle,
  CheckCircle2,
  CircleDashed,
  Gauge,
  Minus,
  TimerReset,
  TrendingDown,
  TrendingUp,
} from 'lucide-react'
import type { CryptoNewsItem, GoldImpact } from '../../types'

const directionMeta = {
  bullish: {
    label: '黄金利多',
    icon: TrendingUp,
    tone: 'text-emerald-300',
    badge: 'border-emerald-400/25 bg-emerald-400/10 text-emerald-300',
  },
  bearish: {
    label: '黄金利空',
    icon: TrendingDown,
    tone: 'text-rose-300',
    badge: 'border-rose-400/25 bg-rose-400/10 text-rose-300',
  },
  neutral: {
    label: '黄金中性',
    icon: Minus,
    tone: 'text-[#a0a6af]',
    badge: 'border-white/10 bg-white/[0.04] text-[#a0a6af]',
  },
} as const

const riskMeta = {
  none: { label: '无明显影响', tone: 'text-[#7d838d]' },
  low: { label: '低风险', tone: 'text-cyan-300' },
  medium: { label: '中风险', tone: 'text-amber-300' },
  high: { label: '高风险', tone: 'text-orange-300' },
  extreme: { label: '极高风险', tone: 'text-red-300' },
} as const

function getDirectionMeta(direction: GoldImpact['direction']) {
  return directionMeta[direction] ?? directionMeta.neutral
}

function getRiskMeta(level: GoldImpact['risk_level']) {
  return riskMeta[level] ?? riskMeta.none
}

function confirmationLabel(status: GoldImpact['confirmation']) {
  if (status === 'confirmed') return '行情已确认'
  if (status === 'divergent') return '行情背离'
  if (status === 'pending') return '等待市场确认'
  return '无需确认'
}

export function GoldImpactInline({ impact }: { impact?: GoldImpact }) {
  if (!impact) return null
  const direction = getDirectionMeta(impact.direction)
  const risk = getRiskMeta(impact.risk_level)
  const DirectionIcon = direction.icon

  return (
    <div className="mt-3 flex flex-wrap items-center gap-1.5 text-[10px]">
      <span
        className={`inline-flex items-center gap-1 rounded border px-2 py-1 font-bold ${direction.badge}`}
      >
        <DirectionIcon className="h-3 w-3" /> {direction.label}
      </span>
      <span className="rounded border border-white/[0.08] bg-black/20 px-2 py-1 font-mono text-[#a5abb4]">
        风险 <b className={risk.tone}>{impact.risk_score}</b>/100
      </span>
      <span className="inline-flex items-center gap-1 rounded border border-white/[0.08] px-2 py-1 text-[#858b95]">
        <TimerReset className="h-3 w-3" /> {impact.horizon}
      </span>
      <span className="inline-flex items-center gap-1 rounded border border-white/[0.08] px-2 py-1 text-[#858b95]">
        {impact.confirmation === 'confirmed' ? (
          <CheckCircle2 className="h-3 w-3 text-emerald-300" />
        ) : (
          <CircleDashed className="h-3 w-3 text-amber-300" />
        )}
        {confirmationLabel(impact.confirmation)}
      </span>
    </div>
  )
}

export function GoldImpactDetail({ impact }: { impact?: GoldImpact }) {
  if (!impact) return null
  const direction = getDirectionMeta(impact.direction)
  const risk = getRiskMeta(impact.risk_level)
  const DirectionIcon = direction.icon

  return (
    <section className="mt-5 overflow-hidden rounded-lg border border-[#d4ff33]/15 bg-[#090b0d]">
      <div className="flex items-center gap-2 border-b border-white/[0.08] px-4 py-3 text-sm font-bold text-white">
        <Gauge className="h-4 w-4 text-[#d4ff33]" /> 黄金影响评估
      </div>
      <div className="grid gap-px bg-white/[0.08] sm:grid-cols-2">
        <div className="bg-[#0d0f12] p-4">
          <p className="text-[10px] text-[#707681]">方向</p>
          <p
            className={`mt-2 inline-flex items-center gap-2 text-sm font-bold ${direction.tone}`}
          >
            <DirectionIcon className="h-4 w-4" /> {direction.label}
          </p>
        </div>
        <div className="bg-[#0d0f12] p-4">
          <p className="text-[10px] text-[#707681]">风险等级</p>
          <p className={`mt-2 font-mono text-sm font-bold ${risk.tone}`}>
            {impact.risk_score}/100 · {risk.label}
          </p>
        </div>
        <div className="bg-[#0d0f12] p-4">
          <p className="text-[10px] text-[#707681]">预计影响周期</p>
          <p className="mt-2 text-sm font-bold text-[#d8dce2]">
            {impact.horizon}
          </p>
        </div>
        <div className="bg-[#0d0f12] p-4">
          <p className="text-[10px] text-[#707681]">市场确认状态</p>
          <p className="mt-2 text-sm font-bold text-amber-300">
            {confirmationLabel(impact.confirmation)}
          </p>
        </div>
      </div>
      <div className="border-t border-white/[0.08] p-4">
        <p className="text-[10px] text-[#707681]">判断原因</p>
        <p className="mt-2 text-xs leading-6 text-[#b9bec7]">{impact.reason}</p>
        {impact.drivers?.length ? (
          <div className="mt-3 flex flex-wrap gap-1.5">
            {impact.drivers.map((driver) => (
              <span
                key={driver}
                className="rounded border border-[#d4ff33]/15 bg-[#d4ff33]/[0.04] px-2 py-1 text-[10px] text-[#b8ca74]"
              >
                {driver}
              </span>
            ))}
          </div>
        ) : null}
      </div>
    </section>
  )
}

interface GoldImpactRadarProps {
  items: CryptoNewsItem[]
  onSelect: (item: CryptoNewsItem) => void
  formatTime: (value: string) => string
}

export function GoldImpactRadar({
  items,
  onSelect,
  formatTime,
}: GoldImpactRadarProps) {
  return (
    <section className="overflow-hidden rounded-lg border border-amber-300/[0.16] bg-[#0d0f12]/95">
      <div className="flex items-center justify-between border-b border-white/[0.08] px-4 py-3">
        <div className="flex items-center gap-2 text-sm font-bold text-white">
          <AlertTriangle className="h-4 w-4 text-amber-300" /> 黄金影响雷达
        </div>
        <span className="font-mono text-[10px] text-amber-300">
          {items.length} SIGNALS
        </span>
      </div>
      <div className="divide-y divide-white/[0.07]">
        {items.slice(0, 6).map((item) => {
          const impact = item.gold_impact!
          const direction = getDirectionMeta(impact.direction)
          const risk = getRiskMeta(impact.risk_level)
          return (
            <button
              key={item.id}
              type="button"
              onClick={() => onSelect(item)}
              className="w-full px-4 py-3 text-left hover:bg-amber-300/[0.035]"
            >
              <div className="mb-1.5 flex items-center justify-between gap-3 text-[10px]">
                <span className={`font-bold ${direction.tone}`}>
                  {direction.label}
                </span>
                <span className={`font-mono font-bold ${risk.tone}`}>
                  {impact.risk_score}/100
                </span>
              </div>
              <p className="line-clamp-2 text-xs font-semibold leading-5 text-[#d8dbe0]">
                {item.title_zh || item.title}
              </p>
              <p className="mt-1 font-mono text-[9px] text-[#686e78]">
                {impact.horizon} · {formatTime(item.published_at)}
              </p>
            </button>
          )
        })}
        {!items.length ? (
          <p className="px-4 py-6 text-center text-xs text-[#6f757f]">
            当前未识别到黄金相关事件
          </p>
        ) : null}
      </div>
    </section>
  )
}
