import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import useSWR from 'swr'
import {
  Activity,
  ArrowLeft,
  BadgeCheck,
  Loader2,
  Sparkles,
} from 'lucide-react'
import {
  Area,
  AreaChart,
  Cell,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { useLanguage } from '../contexts/LanguageContext'
import { useAuth } from '../contexts/AuthContext'
import { api } from '../lib/api'
import { toast } from 'sonner'
import { t } from '../i18n/translations'
import { ROUTES } from '../router/paths'
import type { StrategyMarketAccess } from '../types'
import type {
  PublicStrategyDemoAiInsight,
  PublicStrategyDemoTradeHighlight,
  PublicStrategyMarketDetailPayload,
} from '../lib/api/strategies'
import './strategy-market-ai3.css'
import {
  isHistoricalOnlyStrategy,
  isSimulatedPerformance,
  stripStrategyTitleParenthetical,
} from '../lib/strategyMarketDisplay'
import {
  MARKET_SUB_MONTHLY_USDT,
  MARKET_SUB_WEEKLY_TRIAL_USDT,
} from '../lib/strategyMarketPricing'
import { findExistingForkIdForMarketSource } from '../lib/marketStrategyFork'
import { getExchangeIcon } from '../components/common/ExchangeIcons'
import {
  formatUserFacingError,
  throwUserFacingFetchError,
} from '../lib/userFacingFetchError'
import {
  TradeHistorySection,
  MAINSTREAM_CALM_STRATEGY_ID,
  OKX_STABLE_STRATEGY_ID,
  BN_SMART_OPERATION_STRATEGY_ID,
  ULTIMATE_SOL_STRATEGY_ID,
  buildMainstreamCalmPerformance,
  buildOkxStablePerformance,
  buildBnSmartOperationPerformance,
  buildUltimateSolPerformance,
} from '../components/strategy/TradeHistorySection'

/** 与列表页一致的公开策略展示类型（详情接口 strategy 字段） */
interface PublicStrategyDetail {
  id: string
  name: string
  description: string
  creator_display_name?: string
  creator_avatar_url?: string
  market_access?: StrategyMarketAccess
  market_revision?: number
  market_sale_price_usdt?: number
  market_subscription_monthly_only?: boolean
  market_ai_model?: string
  performance_only?: boolean
  performance_source?: string
  performance_disclosure?: string
  realtime_follow_available?: boolean
  config?: { coins?: string[]; symbol?: string; leverage?: number }
  stats?: {
    used_by: number
    running_agents?: number
    total_agents?: number
    return_7d_pct?: number
    cumulative_return_pct?: number
    max_drawdown_pct?: number
    trend?: number[]
    stats_window_days?: number
  }
  updated_at: string
  /** 演示叠加：虚拟成交明细（名称命中静默测试等） */
  demo_trade_highlights?: PublicStrategyDemoTradeHighlight[]
  demo_ai_insights?: PublicStrategyDemoAiInsight[]
}

const CRYPTO_UP = '#00C087'
const CRYPTO_DOWN = '#F6465D'
/** 详情页展示用固定杠杆（产品约定） */
const DISPLAY_LEVERAGE = 20

function monthlySubscriptionPrice(s: PublicStrategyDetail): number {
  return s.market_sale_price_usdt && s.market_sale_price_usdt > 0
    ? s.market_sale_price_usdt
    : MARKET_SUB_MONTHLY_USDT
}
const MARKET_CREATOR_DISGUISES: Record<
  string,
  { name: string; avatar: string }
> = {
  [MAINSTREAM_CALM_STRATEGY_ID]: {
    name: '青衫量化',
    avatar: 'https://api.dicebear.com/7.x/avataaars/svg?seed=qingshan-quant',
  },
  [OKX_STABLE_STRATEGY_ID]: {
    name: '陆沉',
    avatar: 'https://api.dicebear.com/7.x/avataaars/svg?seed=luchen-okx',
  },
  [ULTIMATE_SOL_STRATEGY_ID]: {
    name: '星野SOL',
    avatar: 'https://api.dicebear.com/7.x/avataaars/svg?seed=xingye-sol',
  },
  [BN_SMART_OPERATION_STRATEGY_ID]: {
    name: '云栖量化',
    avatar: 'https://api.dicebear.com/7.x/avataaars/svg?seed=yunqi-quant',
  },
}

/** 与「总盈亏」主数字一致：Space Grotesk + 等宽数字（不用系统等宽体） */
const FONT_STATS = "font-['Space_Grotesk',sans-serif] tabular-nums"
/** 同字体族的标签/说明小字 */
const FONT_STATS_LABEL = "font-['Space_Grotesk',sans-serif]"

function publicStrategyDetailUrl(id: string): string {
  const path = `/api/strategies/public/${encodeURIComponent(id)}`
  if (import.meta.env.PROD) {
    return path
  }
  const b =
    (import.meta.env.VITE_API_BASE as string | undefined)
      ?.trim()
      .replace(/\/$/, '') ?? ''
  return b ? `${b}${path}` : path
}

function marketAccessOf(s: PublicStrategyDetail): StrategyMarketAccess {
  const a = s.market_access
  if (
    a === 'subscription' ||
    a === 'public' ||
    a === 'open_source' ||
    a === 'private'
  )
    return a
  return 'private'
}

function formatReturnPct(v: number | undefined): string {
  if (typeof v !== 'number' || !Number.isFinite(v)) return '—'
  return `${v >= 0 ? '+' : ''}${v.toFixed(2)}%`
}

function formatUsd(v: number | undefined, compact = false): string {
  if (typeof v !== 'number' || !Number.isFinite(v)) return '—'
  const abs = Math.abs(v)
  const sign = v < 0 ? '-' : ''
  if (compact && abs >= 1_000_000)
    return `${sign}$${(abs / 1_000_000).toFixed(2)}M`
  if (compact && abs >= 1_000) return `${sign}$${(abs / 1_000).toFixed(2)}K`
  return `${sign}$${abs.toFixed(2)}`
}

function formatHoldZh(ms: number | undefined): string {
  if (typeof ms !== 'number' || !Number.isFinite(ms) || ms <= 0) return '—'
  const h = ms / 3600000
  const days = Math.floor(h / 24)
  const hrs = Math.floor(h % 24)
  const mins = Math.floor((ms % 3600000) / 60000)
  if (days > 0) return `${days} 天 ${hrs} 小时`
  if (hrs > 0) return `${hrs} 小时 ${mins} 分`
  return `${mins} 分`
}

/** 上架版本号（支持小数，演示叠加时为小数点后 1 位） */
function formatMarketRevision(v: number | undefined): string {
  if (typeof v !== 'number' || !Number.isFinite(v)) return '1.0'
  return (Math.round(v * 10) / 10).toFixed(1)
}

function formatShortDate(dateStr: string | undefined, lang: string) {
  if (!dateStr) return '—'
  const d = new Date(dateStr)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleDateString(lang === 'zh' ? 'zh-CN' : 'en-US', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  })
}

/** 公开/开源视为免费，可直接「进入策略」 */
function isFreeMarketAccess(s: PublicStrategyDetail): boolean {
  const acc = marketAccessOf(s)
  return acc === 'public' || acc === 'open_source'
}

function isFreeSubscriptionStrategy(s: PublicStrategyDetail): boolean {
  return (
    marketAccessOf(s) === 'subscription' &&
    typeof s.market_sale_price_usdt === 'number' &&
    s.market_sale_price_usdt <= 0
  )
}

/** 需付费上架且当前用户未购权益 → 金色「购买策略」 */
function needsPurchaseButton(
  s: PublicStrategyDetail,
  entitled: boolean
): boolean {
  if (isFreeMarketAccess(s)) return false
  return !entitled
}

export function StrategyMarketDetailPage() {
  const { strategyId } = useParams<{ strategyId: string }>()
  const id = strategyId?.trim() ?? ''
  const navigate = useNavigate()
  const { language } = useLanguage()
  const { token, applyUserProfile } = useAuth()
  const [purchaseBusy, setPurchaseBusy] = useState(false)
  const [purchaseOpen, setPurchaseOpen] = useState(false)
  const [purchasePlan, setPurchasePlan] = useState<'monthly' | 'weekly'>(
    'monthly'
  )

  const tr = (key: string) => t(`strategyMarket.${key}`, language)

  const { data: walletData, mutate: mutateWallet } = useSWR(
    token ? 'platform-wallet' : null,
    () => api.getWallet(),
    { refreshInterval: 45000, revalidateOnFocus: true }
  )

  const entitlementIds = useMemo(
    () => new Set((walletData?.entitlements ?? []).map((e) => e.strategy_id)),
    [walletData]
  )

  useEffect(() => {
    if (!walletData) return
    applyUserProfile({
      balance_usdt: walletData.balance_usdt,
      is_admin: walletData.is_admin,
      is_finance: walletData.is_finance,
    })
  }, [walletData, applyUserProfile])

  useEffect(() => {
    if (purchaseOpen) setPurchasePlan('monthly')
  }, [purchaseOpen])

  useEffect(() => {
    if (
      purchaseOpen &&
      walletData?.market_weekly_trial_used &&
      purchasePlan === 'weekly'
    ) {
      setPurchasePlan('monthly')
    }
  }, [purchaseOpen, walletData?.market_weekly_trial_used, purchasePlan])

  const {
    data: payload,
    error,
    isLoading,
    mutate,
  } = useSWR<PublicStrategyMarketDetailPayload>(
    id ? publicStrategyDetailUrl(id) : null,
    async (url: string) => {
      const response = await fetch(url, { credentials: 'same-origin' })
      if (!response.ok) {
        const body = await response.text().catch(() => '')
        throwUserFacingFetchError(
          response.status,
          body,
          language === 'zh' ? 'zh' : 'en'
        )
      }
      return response.json() as Promise<PublicStrategyMarketDetailPayload>
    },
    {
      refreshInterval: 60000,
      revalidateOnFocus: false,
      dedupingInterval: 30000,
      keepPreviousData: true,
    }
  )

  const strategy = payload?.strategy as PublicStrategyDetail | undefined
  const historicalOnly = isHistoricalOnlyStrategy(strategy)
  const simulatedPerformance = isSimulatedPerformance(strategy)
  const rawAgg = payload?.aggregate_trading
  const rawInitialCapital = payload?.initial_capital ?? 0
  const historyPerformance = useMemo(
    () =>
      buildMainstreamCalmPerformance(id) ??
      buildOkxStablePerformance(id) ??
      buildBnSmartOperationPerformance(id) ??
      buildUltimateSolPerformance(id),
    [id]
  )
  const agg = historyPerformance?.aggregate ?? rawAgg
  const initialCapital = historyPerformance?.initialCapital ?? rawInitialCapital
  const stats = agg?.stats
  const exchangeLabel = (payload?.exchange_label ?? '').trim()
  const statsSource = payload?.stats_source

  const duplicateMarketStrategyToMine = async (
    s: PublicStrategyDetail,
    kind: 'purchase' | 'public' | 'open_source' | 'sub_owned'
  ) => {
    const suffix =
      kind === 'public'
        ? language === 'zh'
          ? '（公开）'
          : ' (public)'
        : kind === 'open_source'
          ? language === 'zh'
            ? '（开源）'
            : ' (open source)'
          : language === 'zh'
            ? '（已购）'
            : ' (purchased)'
    const dupName = `${s.name}${suffix}`.slice(0, 200)

    let existingId: string | undefined
    try {
      const mine = await api.getStrategies()
      existingId = findExistingForkIdForMarketSource(s.id, mine)
    } catch {
      /* 列表拉取失败时不阻断复制 */
    }

    const finishOpenFork = async (targetId: string, reused: boolean) => {
      try {
        if (kind === 'open_source' && s.config) {
          await navigator.clipboard.writeText(JSON.stringify(s.config, null, 2))
        } else if (kind === 'purchase' || kind === 'sub_owned') {
          const data = await api.getMarketOwnedStrategy(s.id)
          await navigator.clipboard.writeText(
            JSON.stringify(data.config, null, 2)
          )
        }
      } catch {
        /* ignore */
      }
      toast.success(
        tr(reused ? 'toastOpenExistingFork' : 'toastAddedToStrategies')
      )
      navigate(`${ROUTES.strategy}?id=${encodeURIComponent(targetId)}`)
    }

    if (existingId) {
      await finishOpenFork(existingId, true)
      return
    }

    const { id: newId } = await api.duplicateStrategy(s.id, dupName)
    await finishOpenFork(newId, false)
  }

  /** 进入策略：已购/免费用户复制到「我的策略」并打开构建器 */
  const handleEnterStrategy = (s: PublicStrategyDetail) => {
    if (isHistoricalOnlyStrategy(s)) {
      toast.info(
        language === 'zh'
          ? '该策略尚未绑定实时主控，暂不支持跟单'
          : 'This strategy is not connected to a live master yet'
      )
      return
    }
    const acc = marketAccessOf(s)
    if (acc === 'private' || acc === 'subscription') {
      if (!token) {
        toast.error(language === 'zh' ? '请先登录' : 'Please sign in')
        navigate(ROUTES.login)
        return
      }
      if (entitlementIds.has(s.id)) {
        void (async () => {
          try {
            await duplicateMarketStrategyToMine(s, 'sub_owned')
          } catch (e) {
            toast.error(e instanceof Error ? e.message : tr('copyFailed'))
          }
        })()
        return
      }
      setPurchaseOpen(true)
      return
    }
    if (acc === 'public' || acc === 'open_source') {
      if (!token) {
        toast.error(language === 'zh' ? '请先登录' : 'Please sign in')
        navigate(ROUTES.login)
        return
      }
      void (async () => {
        try {
          await duplicateMarketStrategyToMine(
            s,
            acc === 'open_source' ? 'open_source' : 'public'
          )
        } catch (e) {
          toast.error(e instanceof Error ? e.message : tr('copyFailed'))
        }
      })()
    }
  }

  const handlePrimaryCta = (s: PublicStrategyDetail) => {
    if (needsPurchaseButton(s, entitlementIds.has(s.id))) {
      if (!token) {
        toast.error(language === 'zh' ? '请先登录' : 'Please sign in')
        navigate(ROUTES.login)
        return
      }
      setPurchaseOpen(true)
      return
    }
    handleEnterStrategy(s)
  }

  const submitPurchase = async () => {
    if (!strategy) return
    setPurchaseBusy(true)
    try {
      const res = await api.postMarketPurchase(
        strategy.id,
        isFreeSubscriptionStrategy(strategy) ? 'free' : purchasePlan
      )
      toast.success(
        res.message || (language === 'zh' ? '购买成功' : 'Purchased')
      )
      await mutateWallet()
      applyUserProfile({ balance_usdt: res.balance_usdt })
      try {
        await duplicateMarketStrategyToMine(strategy, 'purchase')
      } catch (dupErr) {
        toast.error(tr('toastPurchasedDupFailed'))
        console.warn(dupErr)
      }
      setPurchaseOpen(false)
      void mutate()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '购买失败')
    } finally {
      setPurchaseBusy(false)
    }
  }

  const trend = historyPerformance?.trend ?? strategy?.stats?.trend ?? []
  const windowDays = strategy?.stats?.stats_window_days ?? 90
  const retVal = simulatedPerformance
    ? (strategy?.stats?.cumulative_return_pct ?? strategy?.stats?.return_7d_pct)
    : (historyPerformance?.returnPct ?? strategy?.stats?.return_7d_pct)
  const up =
    typeof retVal === 'number' && Number.isFinite(retVal) ? retVal >= 0 : true
  const disguisedCreator = strategy
    ? MARKET_CREATOR_DISGUISES[strategy.id]
    : undefined
  const displayCreatorName =
    disguisedCreator?.name ?? strategy?.creator_display_name ?? '—'
  const displayCreatorAvatar =
    disguisedCreator?.avatar ?? strategy?.creator_avatar_url
  const displayMarketRevision =
    strategy?.id === BN_SMART_OPERATION_STRATEGY_ID
      ? 1.4
      : strategy?.market_revision

  const equityChartData = useMemo(
    () => trend.map((v, i) => ({ i: i + 1, v })),
    [trend]
  )

  /** 盈亏两行进度条：按盈利/亏损金额占比分配宽度 */
  const profitLossShare = useMemo(() => {
    const gp = Math.max(0, agg?.gross_profit ?? 0)
    const gl = Math.max(0, agg?.gross_loss ?? 0)
    const sum = gp + gl
    if (sum <= 1e-9) return { profitPct: 0, lossPct: 0 }
    return { profitPct: (gp / sum) * 100, lossPct: (gl / sum) * 100 }
  }, [agg])

  const longShortPie = useMemo(() => {
    const lo = agg?.long_trades ?? 0
    const sh = agg?.short_trades ?? 0
    const t = lo + sh
    if (t <= 0) {
      return [
        { name: language === 'zh' ? '多' : 'Long', value: 1, fill: CRYPTO_UP },
        {
          name: language === 'zh' ? '空' : 'Short',
          value: 1,
          fill: CRYPTO_DOWN,
        },
      ]
    }
    return [
      { name: language === 'zh' ? '多' : 'Long', value: lo, fill: CRYPTO_UP },
      {
        name: language === 'zh' ? '空' : 'Short',
        value: sh,
        fill: CRYPTO_DOWN,
      },
    ]
  }, [agg, language])

  const longPct = useMemo(() => {
    const lo = agg?.long_trades ?? 0
    const sh = agg?.short_trades ?? 0
    const t = lo + sh
    if (t <= 0) return 0
    return Math.round((lo / t) * 10000) / 100
  }, [agg])

  const shortPct = useMemo(() => {
    const lo = agg?.long_trades ?? 0
    const sh = agg?.short_trades ?? 0
    const t = lo + sh
    if (t <= 0) return 0
    return Math.round((sh / t) * 10000) / 100
  }, [agg])

  if (!id) {
    return (
      <div className="strategy-market-ai3 min-h-screen bg-black pb-16 pt-8 text-white">
        <div className="px-4">
          <p className="text-zinc-400">
            {language === 'zh' ? '缺少策略 ID' : 'Missing strategy id'}
          </p>
          <Link
            to={ROUTES.strategyMarket}
            className="mt-4 inline-block text-primary"
          >
            ← {language === 'zh' ? '返回策略市场' : 'Back'}
          </Link>
        </div>
      </div>
    )
  }

  return (
    <div className="strategy-market-ai3 min-h-screen bg-black pb-24 font-[Inter,system-ui,sans-serif] text-[#eaecef] selection:bg-primary/30">
      <main className="mx-auto max-w-6xl px-3 pb-10 pt-6 sm:px-6">
        <div className="mb-6 flex flex-wrap items-center gap-3">
          <Link
            to={ROUTES.strategyMarket}
            className="inline-flex items-center gap-2 text-sm text-zinc-400 transition-colors hover:text-white"
          >
            <ArrowLeft className="h-4 w-4" />
            {language === 'zh' ? '返回策略市场' : 'Back to market'}
          </Link>
        </div>

        {isLoading && !payload && (
          <div className="flex flex-col items-center justify-center py-32">
            <Loader2 className="h-10 w-10 animate-spin text-primary" />
            <p className="mt-4 text-sm text-zinc-500">{tr('loading')}</p>
          </div>
        )}

        {error && (
          <div className="rounded-xl border border-red-500/30 bg-red-950/40 px-4 py-4 text-sm text-red-100">
            <p className="font-bold">
              {language === 'zh' ? '加载失败' : 'Failed to load'}
            </p>
            <p className="mt-2">
              {formatUserFacingError(error, language === 'zh' ? 'zh' : 'en')}
            </p>
            <button
              type="button"
              onClick={() => void mutate()}
              className="mt-3 rounded-lg bg-red-500/20 px-3 py-1 text-xs font-bold"
            >
              {language === 'zh' ? '重试' : 'Retry'}
            </button>
          </div>
        )}

        {strategy && (
          <>
            <header className="mb-8 flex flex-col gap-5 border-b border-zinc-800/80 pb-8 lg:flex-row lg:items-start lg:justify-between">
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-3">
                  <h1 className="break-words font-['Space_Grotesk',sans-serif] text-2xl font-bold tracking-tight text-white sm:text-3xl">
                    {stripStrategyTitleParenthetical(strategy.name)}
                  </h1>
                  <span className="shrink-0 rounded-md bg-zinc-800 px-2 py-0.5 text-xs font-bold text-zinc-300">
                    V{formatMarketRevision(displayMarketRevision)}
                  </span>
                </div>
                <div className="mt-4 flex flex-wrap items-center gap-3 text-sm">
                  {displayCreatorAvatar ? (
                    <img
                      src={displayCreatorAvatar}
                      alt=""
                      className="h-9 w-9 rounded-full border border-zinc-700 object-cover"
                    />
                  ) : (
                    <div className="flex h-9 w-9 items-center justify-center rounded-full bg-zinc-800 text-sm font-bold">
                      {(displayCreatorName || '?')[0]}
                    </div>
                  )}
                  <span className="font-medium text-zinc-200">
                    {displayCreatorName}
                  </span>
                  <BadgeCheck className="h-4 w-4 text-sky-400" aria-hidden />
                  <span className="text-zinc-500">·</span>
                  <span className="text-zinc-400">
                    {language === 'zh' ? '运行中 Agent' : 'Running'}:{' '}
                    <span className="tabular-nums text-[#0ECB81]">
                      {strategy.stats?.running_agents ??
                        strategy.stats?.used_by ??
                        0}
                    </span>
                  </span>
                  <span className="text-zinc-500">·</span>
                  <span className="text-zinc-500">
                    {language === 'zh' ? '更新' : 'Updated'}{' '}
                    {formatShortDate(strategy.updated_at, language)}
                  </span>
                </div>
              </div>
              {strategy ? (
                historicalOnly ? (
                  <button
                    type="button"
                    disabled
                    className="inline-flex h-11 w-full cursor-not-allowed items-center justify-center gap-2 rounded-xl border border-amber-500/35 bg-amber-500/10 px-6 text-sm font-bold text-amber-100 opacity-90 sm:w-auto"
                  >
                    <Activity className="h-4 w-4" aria-hidden />
                    {language === 'zh' ? '仅历史展示' : 'Historical only'}
                  </button>
                ) : needsPurchaseButton(
                    strategy,
                    entitlementIds.has(strategy.id)
                  ) ? (
                  <button
                    type="button"
                    onClick={() => handlePrimaryCta(strategy)}
                    className="market-detail-cta-gold inline-flex h-11 w-full shrink-0 items-center justify-center gap-2 rounded-xl px-6 text-sm font-bold text-black transition-transform hover:scale-[1.02] active:scale-[0.98] sm:w-auto"
                  >
                    {language === 'zh' ? '订阅' : 'Subscribe'}
                  </button>
                ) : (
                  <button
                    type="button"
                    onClick={() => handlePrimaryCta(strategy)}
                    className="market-detail-cta-firefly inline-flex h-11 w-full shrink-0 items-center justify-center gap-2 rounded-xl px-6 text-sm font-bold text-emerald-50 shadow-[0_0_28px_rgba(52,211,153,0.38),0_0_56px_rgba(34,211,238,0.12)] transition-transform hover:scale-[1.02] active:scale-[0.98] sm:w-auto"
                  >
                    <Sparkles
                      className="h-4 w-4 text-emerald-200"
                      aria-hidden
                    />
                    {language === 'zh' ? '进入策略' : 'Open strategy'}
                  </button>
                )
              ) : null}
            </header>

            {token &&
              (walletData?.market_subscription_alerts ?? []).filter(
                (a) => a.strategy_id === strategy.id
              ).length > 0 && (
                <div className="mb-6 space-y-2">
                  {(walletData?.market_subscription_alerts ?? [])
                    .filter((a) => a.strategy_id === strategy.id)
                    .map((a, i) => (
                      <div
                        key={`${a.kind}-${i}`}
                        className={`rounded-xl border px-4 py-3 text-sm ${
                          a.kind === 'expiring_soon'
                            ? 'border-amber-500/45 bg-amber-950/35 text-amber-50'
                            : 'border-zinc-600/55 bg-zinc-900/55 text-zinc-200'
                        }`}
                      >
                        {language === 'zh'
                          ? a.message_zh
                          : (a.message_en ?? a.message_zh)}
                      </div>
                    ))}
                </div>
              )}

            <section
              className={`mb-10 flex flex-wrap items-center gap-x-5 gap-y-2 rounded-2xl border border-zinc-800/80 px-4 py-3 text-sm ${FONT_STATS_LABEL}`}
              style={{ backgroundColor: '#121212' }}
            >
              <span className="text-zinc-500">
                {language === 'zh' ? '交易所' : 'Exchange'}
              </span>
              <span className={`font-medium text-zinc-100 ${FONT_STATS}`}>
                <span className="inline-flex items-center gap-2">
                  {exchangeLabel
                    ? getExchangeIcon(exchangeLabel, { width: 20, height: 20 })
                    : null}
                  {exchangeLabel || '—'}
                </span>
              </span>
              <span className="hidden text-zinc-700 sm:inline">|</span>
              <span className="text-zinc-500">
                {language === 'zh' ? '杠杆' : 'Leverage'}
              </span>
              <span className={`font-medium text-zinc-100 ${FONT_STATS}`}>
                {historicalOnly
                  ? '—'
                  : exchangeLabel.toUpperCase() === 'BALIB'
                    ? language === 'zh'
                      ? '动态'
                      : 'Dynamic'
                    : `${DISPLAY_LEVERAGE}x`}
              </span>
              <span className="hidden text-zinc-700 sm:inline">|</span>
              <span className="font-medium text-zinc-200">
                {simulatedPerformance
                  ? language === 'zh'
                    ? '模拟业绩'
                    : 'Simulated performance'
                  : language === 'zh'
                    ? '使用 AI 为 COMKUN-AI'
                    : 'AI: COMKUN-AI'}
              </span>
            </section>

            <section
              className={`mb-4 flex flex-wrap items-center gap-2 text-lg font-bold text-white ${FONT_STATS_LABEL}`}
            >
              <Activity className="h-5 w-5 text-primary" />
              {language === 'zh' ? '运行数据' : 'Performance'}
              <span className="text-sm font-normal text-zinc-400">
                （{language === 'zh' ? '近' : 'Last'} {windowDays}{' '}
                {language === 'zh' ? '日统计区间' : 'd window'}）
              </span>
            </section>

            <div className="grid grid-cols-1 gap-4 lg:grid-cols-12">
              {/* 总盈亏 + 曲线 */}
              <div
                className="flex flex-col rounded-2xl border border-zinc-800/80 p-4 sm:p-5 lg:col-span-5"
                style={{ backgroundColor: '#121212' }}
              >
                <p
                  className={`text-sm font-bold uppercase tracking-wider text-zinc-400 ${FONT_STATS_LABEL}`}
                >
                  {language === 'zh'
                    ? '总盈亏（汇总净值变动）'
                    : 'Total return'}
                </p>
                <p
                  className={`mt-2 text-2xl font-bold sm:text-3xl ${FONT_STATS}`}
                  style={{ color: up ? CRYPTO_UP : CRYPTO_DOWN }}
                >
                  {formatReturnPct(retVal)}{' '}
                  <span
                    className={`text-lg font-semibold text-zinc-300 ${FONT_STATS}`}
                  >
                    {formatUsd(stats?.total_pnl, true)}
                  </span>
                </p>
                <div className="mt-4 h-44 w-full">
                  {equityChartData.length >= 2 ? (
                    <ResponsiveContainer width="100%" height="100%">
                      <AreaChart data={equityChartData}>
                        <defs>
                          <linearGradient
                            id="eqFill"
                            x1="0"
                            y1="0"
                            x2="0"
                            y2="1"
                          >
                            <stop
                              offset="0%"
                              stopColor={up ? CRYPTO_UP : CRYPTO_DOWN}
                              stopOpacity={0.35}
                            />
                            <stop
                              offset="100%"
                              stopColor={up ? CRYPTO_UP : CRYPTO_DOWN}
                              stopOpacity={0}
                            />
                          </linearGradient>
                        </defs>
                        <XAxis dataKey="i" hide />
                        <YAxis hide domain={['auto', 'auto']} />
                        <Tooltip
                          contentStyle={{
                            background: '#1a1a1a',
                            border: '1px solid #333',
                          }}
                          labelStyle={{ display: 'none' }}
                          formatter={(v: number) => [v.toFixed(2), 'Σ净值']}
                        />
                        <Area
                          type="linear"
                          dataKey="v"
                          stroke={up ? CRYPTO_UP : CRYPTO_DOWN}
                          strokeWidth={2}
                          fill="url(#eqFill)"
                        />
                      </AreaChart>
                    </ResponsiveContainer>
                  ) : (
                    <div
                      className={`flex h-full items-center justify-center text-sm text-zinc-500 ${FONT_STATS_LABEL}`}
                    >
                      {language === 'zh'
                        ? '暂无足够曲线数据'
                        : 'Not enough history'}
                    </div>
                  )}
                </div>
                <dl className="mt-4 grid grid-cols-1 gap-3 border-t border-zinc-800/80 pt-4 sm:grid-cols-3">
                  <div>
                    <dt className={`text-sm text-zinc-400 ${FONT_STATS_LABEL}`}>
                      {language === 'zh' ? '初始资金（合计）' : 'Initial'}
                    </dt>
                    <dd
                      className={`mt-1 text-base text-zinc-100 ${FONT_STATS}`}
                    >
                      {initialCapital > 0 ? formatUsd(initialCapital) : '—'}
                    </dd>
                  </div>
                  <div>
                    <dt className={`text-sm text-zinc-400 ${FONT_STATS_LABEL}`}>
                      {language === 'zh' ? '夏普比率' : 'Sharpe'}
                    </dt>
                    <dd
                      className={`mt-1 text-base text-zinc-100 ${FONT_STATS}`}
                    >
                      {stats &&
                      stats.total_trades > 1 &&
                      typeof stats.sharpe_ratio === 'number'
                        ? stats.sharpe_ratio < 0 &&
                          statsSource === 'demo_overlay'
                          ? '—'
                          : stats.sharpe_ratio.toFixed(2)
                        : '—'}
                    </dd>
                  </div>
                  <div>
                    <dt className={`text-sm text-zinc-400 ${FONT_STATS_LABEL}`}>
                      {language === 'zh' ? '最大回撤' : 'Max DD'}
                    </dt>
                    <dd
                      className={`mt-1 text-base text-[#F6465D] ${FONT_STATS}`}
                    >
                      {(() => {
                        const raw =
                          historyPerformance?.aggregate.stats
                            .max_drawdown_pct ??
                          strategy.stats?.max_drawdown_pct ??
                          stats?.max_drawdown_pct
                        if (typeof raw !== 'number' || !Number.isFinite(raw))
                          return '—'
                        const neg = -Math.abs(raw)
                        return `${neg.toFixed(2)}%`
                      })()}
                    </dd>
                  </div>
                </dl>
              </div>

              <div className="flex flex-col gap-4 lg:col-span-4">
                <div
                  className="rounded-2xl border border-zinc-800/80 p-5"
                  style={{ backgroundColor: '#121212' }}
                >
                  <p
                    className={`text-sm font-bold uppercase tracking-wider text-zinc-400 ${FONT_STATS_LABEL}`}
                  >
                    {language === 'zh' ? '总交易次数' : 'Trades'}
                  </p>
                  <p
                    className={`mt-2 text-3xl font-bold text-white ${FONT_STATS}`}
                  >
                    {stats?.total_trades ?? 0}
                  </p>
                  <div className="mt-4 grid grid-cols-2 gap-4">
                    <div>
                      <p
                        className={`text-sm text-zinc-400 ${FONT_STATS_LABEL}`}
                      >
                        {language === 'zh' ? '总费用' : 'Fees'}
                      </p>
                      <p
                        className={`mt-1 text-base text-zinc-100 ${FONT_STATS}`}
                      >
                        {formatUsd(stats?.total_fee)}
                      </p>
                    </div>
                    <div>
                      <p
                        className={`text-sm text-zinc-400 ${FONT_STATS_LABEL}`}
                      >
                        {language === 'zh' ? '平均持仓' : 'Avg hold'}
                      </p>
                      <p
                        className={`mt-1 text-base text-zinc-100 ${FONT_STATS}`}
                      >
                        {formatHoldZh(agg?.avg_hold_ms)}
                      </p>
                    </div>
                  </div>
                </div>

                <div
                  className="flex flex-1 flex-col rounded-2xl border border-zinc-800/80 p-5"
                  style={{ backgroundColor: '#121212' }}
                >
                  <p
                    className={`text-sm font-bold uppercase tracking-wider text-zinc-400 ${FONT_STATS_LABEL}`}
                  >
                    {language === 'zh' ? '盈亏比（盈利因子）' : 'Profit factor'}
                  </p>
                  <p
                    className={`mt-2 text-2xl font-bold text-white ${FONT_STATS}`}
                  >
                    {stats && stats.profit_factor > 0
                      ? stats.profit_factor.toFixed(2)
                      : '—'}
                  </p>
                  <div className="mt-4 flex flex-col gap-4">
                    <div className="flex items-center gap-3">
                      <div className="flex w-[7.5rem] shrink-0 flex-col gap-0.5 sm:w-[9.5rem]">
                        <span
                          className={`text-sm text-zinc-400 ${FONT_STATS_LABEL}`}
                        >
                          {language === 'zh' ? '盈利' : 'Profit'}
                        </span>
                        <span
                          className={`text-base ${FONT_STATS}`}
                          style={{ color: CRYPTO_UP }}
                        >
                          {formatUsd(agg?.gross_profit)}
                        </span>
                      </div>
                      <div className="h-3 min-w-0 flex-1 overflow-hidden rounded-full bg-zinc-800">
                        <div
                          className="h-full rounded-full transition-all"
                          style={{
                            width: `${profitLossShare.profitPct}%`,
                            backgroundColor: CRYPTO_UP,
                          }}
                        />
                      </div>
                    </div>
                    <div className="flex items-center gap-3">
                      <div className="flex w-[7.5rem] shrink-0 flex-col gap-0.5 sm:w-[9.5rem]">
                        <span
                          className={`text-sm text-zinc-400 ${FONT_STATS_LABEL}`}
                        >
                          {language === 'zh' ? '亏损' : 'Loss'}
                        </span>
                        <span
                          className={`text-base ${FONT_STATS}`}
                          style={{ color: CRYPTO_DOWN }}
                        >
                          {formatUsd(agg?.gross_loss)}
                        </span>
                      </div>
                      <div className="h-3 min-w-0 flex-1 overflow-hidden rounded-full bg-zinc-800">
                        <div
                          className="h-full rounded-full transition-all"
                          style={{
                            width: `${profitLossShare.lossPct}%`,
                            backgroundColor: CRYPTO_DOWN,
                          }}
                        />
                      </div>
                    </div>
                  </div>
                </div>
              </div>

              <div className="flex flex-col gap-4 lg:col-span-3">
                <div
                  className="rounded-2xl border border-zinc-800/80 p-5"
                  style={{ backgroundColor: '#121212' }}
                >
                  <p
                    className={`text-sm font-bold uppercase tracking-wider text-zinc-400 ${FONT_STATS_LABEL}`}
                  >
                    {language === 'zh' ? '胜率' : 'Win rate'}
                  </p>
                  <p
                    className={`mt-2 text-3xl font-bold text-white ${FONT_STATS}`}
                  >
                    {stats && stats.total_trades > 0
                      ? `${stats.win_rate.toFixed(2)}%`
                      : '—'}
                  </p>
                  <div className="mt-3 flex h-5 w-full overflow-hidden rounded-full bg-zinc-800">
                    <div
                      className="h-full shrink-0 rounded-l-full transition-all"
                      style={{
                        width: `${
                          stats && stats.total_trades > 0
                            ? (stats.win_trades / stats.total_trades) * 100
                            : 0
                        }%`,
                        backgroundColor: CRYPTO_UP,
                      }}
                    />
                    <div
                      className="h-full shrink-0 rounded-r-full transition-all"
                      style={{
                        width: `${
                          stats && stats.total_trades > 0
                            ? (stats.loss_trades / stats.total_trades) * 100
                            : 0
                        }%`,
                        backgroundColor: CRYPTO_DOWN,
                      }}
                    />
                  </div>
                  <div
                    className={`mt-2 flex justify-between text-base font-semibold ${FONT_STATS}`}
                  >
                    <span style={{ color: CRYPTO_UP }}>
                      {stats?.win_trades ?? 0} {language === 'zh' ? '胜' : 'W'}
                    </span>
                    <span style={{ color: CRYPTO_DOWN }}>
                      {stats?.loss_trades ?? 0} {language === 'zh' ? '负' : 'L'}
                    </span>
                  </div>
                </div>

                <div
                  className="flex flex-1 flex-col rounded-2xl border border-zinc-800/80 p-5"
                  style={{ backgroundColor: '#121212' }}
                >
                  <p
                    className={`text-sm font-bold uppercase tracking-wider text-zinc-400 ${FONT_STATS_LABEL}`}
                  >
                    {language === 'zh' ? '多空比' : 'Long / Short'}
                  </p>
                  {/* 上：多空百分比 | 中：饼图 | 下：笔数，分层留白避免挤在一起 */}
                  <div className="mt-5 flex flex-col items-stretch">
                    <div
                      className={`flex items-baseline justify-between gap-3 px-1 text-base font-bold leading-snug sm:gap-6 sm:text-lg ${FONT_STATS}`}
                    >
                      <span style={{ color: CRYPTO_UP }}>
                        {language === 'zh' ? '多' : 'L'} {longPct.toFixed(2)}%
                      </span>
                      <span
                        className="text-right"
                        style={{ color: CRYPTO_DOWN }}
                      >
                        {language === 'zh' ? '空' : 'S'} {shortPct.toFixed(2)}%
                      </span>
                    </div>
                    <div className="mx-auto my-5 h-[120px] w-[120px] shrink-0 sm:my-6 sm:h-[132px] sm:w-[132px] [&_.recharts-layer]:outline-none">
                      <ResponsiveContainer width="100%" height="100%">
                        <PieChart>
                          <Pie
                            data={longShortPie}
                            dataKey="value"
                            cx="50%"
                            cy="50%"
                            innerRadius={40}
                            outerRadius={54}
                            paddingAngle={0}
                            stroke="none"
                            strokeWidth={0}
                          >
                            {longShortPie.map((entry, index) => (
                              <Cell
                                key={`cell-${index}`}
                                fill={entry.fill}
                                stroke="none"
                                strokeWidth={0}
                              />
                            ))}
                          </Pie>
                          <Tooltip />
                        </PieChart>
                      </ResponsiveContainer>
                    </div>
                    <div
                      className={`flex items-start justify-between gap-3 border-t border-zinc-800/60 px-1 pt-4 text-sm text-zinc-200 sm:gap-6 sm:text-base ${FONT_STATS}`}
                    >
                      <span>
                        {agg?.long_trades ?? 0}{' '}
                        {language === 'zh' ? '笔交易' : 'trades'}
                      </span>
                      <span className="text-right">
                        {agg?.short_trades ?? 0}{' '}
                        {language === 'zh' ? '笔交易' : 'trades'}
                      </span>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <TradeHistorySection
              strategyId={id}
              rows={payload?.trade_history}
            />

            {Array.isArray(strategy.demo_trade_highlights) &&
              strategy.demo_trade_highlights.length > 0 && (
                <section className="mt-10">
                  <h2
                    className={`mb-4 flex flex-wrap items-center gap-2 text-lg font-bold text-white ${FONT_STATS_LABEL}`}
                  >
                    <Sparkles className="h-5 w-5 text-primary" />
                    {language === 'zh'
                      ? '演示成交明细（近一月样本）'
                      : 'Demo trades (sample)'}
                  </h2>
                  <div className="space-y-3 md:hidden">
                    {strategy.demo_trade_highlights.map((row, idx) => {
                      const upRow = row.pnl_usdt >= 0
                      return (
                        <article
                          key={`mobile-${row.symbol}-${idx}`}
                          className="rounded-2xl border border-zinc-800/80 p-4 text-sm"
                          style={{ backgroundColor: '#121212' }}
                        >
                          <div className="flex items-start justify-between gap-3">
                            <div>
                              <div className="font-bold text-zinc-100">
                                {row.symbol}
                              </div>
                              <div className="mt-1 text-xs text-zinc-500">
                                {formatShortDate(row.opened_at, language)}{' '}
                                {new Date(row.opened_at).toLocaleTimeString(
                                  language === 'zh' ? 'zh-CN' : 'en-US',
                                  {
                                    hour: '2-digit',
                                    minute: '2-digit',
                                  }
                                )}
                              </div>
                            </div>
                            <span
                              className={`rounded-full px-2 py-1 text-xs font-bold ${
                                row.side === 'short'
                                  ? 'bg-red-500/10 text-red-300'
                                  : 'bg-emerald-500/10 text-emerald-300'
                              }`}
                            >
                              {row.side === 'short'
                                ? language === 'zh'
                                  ? '空'
                                  : 'Short'
                                : language === 'zh'
                                  ? '多'
                                  : 'Long'}
                            </span>
                          </div>
                          <div className="mt-3 grid grid-cols-2 gap-2 rounded-xl bg-zinc-900/70 p-3">
                            <div>
                              <div className="text-xs text-zinc-500">
                                {language === 'zh' ? '开仓价' : 'Entry'}
                              </div>
                              <div
                                className={`mt-1 tabular-nums text-zinc-200 ${FONT_STATS}`}
                              >
                                {row.entry_price.toFixed(4)}
                              </div>
                            </div>
                            <div>
                              <div className="text-xs text-zinc-500">
                                {language === 'zh' ? '平仓价' : 'Exit'}
                              </div>
                              <div
                                className={`mt-1 tabular-nums text-zinc-200 ${FONT_STATS}`}
                              >
                                {row.exit_price.toFixed(4)}
                              </div>
                            </div>
                            <div className="col-span-2">
                              <div className="text-xs text-zinc-500">
                                {language === 'zh' ? '盈亏 (USDT)' : 'PnL'}
                              </div>
                              <div
                                className={`mt-1 text-lg font-bold tabular-nums ${FONT_STATS}`}
                                style={{
                                  color: upRow ? CRYPTO_UP : CRYPTO_DOWN,
                                }}
                              >
                                {formatUsd(row.pnl_usdt)}
                              </div>
                            </div>
                          </div>
                        </article>
                      )
                    })}
                  </div>
                  <div
                    className="hidden max-h-[28rem] overflow-auto rounded-2xl border border-zinc-800/80 md:block"
                    style={{ backgroundColor: '#121212' }}
                  >
                    <table className="w-full min-w-[720px] border-collapse text-sm">
                      <thead className="sticky top-0 z-[1] bg-[#1a1a1a] text-left text-xs font-bold uppercase tracking-wide text-zinc-500">
                        <tr>
                          <th className="px-4 py-3">
                            {language === 'zh' ? '开仓时间' : 'Opened'}
                          </th>
                          <th className="px-4 py-3">
                            {language === 'zh' ? '币种' : 'Symbol'}
                          </th>
                          <th className="px-4 py-3">
                            {language === 'zh' ? '方向' : 'Side'}
                          </th>
                          <th className="px-4 py-3">
                            {language === 'zh' ? '开仓价' : 'Entry'}
                          </th>
                          <th className="px-4 py-3">
                            {language === 'zh' ? '平仓价' : 'Exit'}
                          </th>
                          <th className="px-4 py-3 text-right">
                            {language === 'zh' ? '盈亏 (USDT)' : 'PnL'}
                          </th>
                        </tr>
                      </thead>
                      <tbody className={`text-zinc-200 ${FONT_STATS}`}>
                        {strategy.demo_trade_highlights.map((row, idx) => {
                          const upRow = row.pnl_usdt >= 0
                          return (
                            <tr
                              key={`${row.symbol}-${idx}`}
                              className="border-t border-zinc-800/60"
                            >
                              <td className="whitespace-nowrap px-4 py-2.5 text-zinc-400">
                                {formatShortDate(row.opened_at, language)}{' '}
                                <span className="tabular-nums text-zinc-500">
                                  {new Date(row.opened_at).toLocaleTimeString(
                                    language === 'zh' ? 'zh-CN' : 'en-US',
                                    {
                                      hour: '2-digit',
                                      minute: '2-digit',
                                    }
                                  )}
                                </span>
                              </td>
                              <td className="px-4 py-2.5 font-medium">
                                {row.symbol}
                              </td>
                              <td className="px-4 py-2.5">
                                {row.side === 'short'
                                  ? language === 'zh'
                                    ? '空'
                                    : 'Short'
                                  : language === 'zh'
                                    ? '多'
                                    : 'Long'}
                              </td>
                              <td className="tabular-nums px-4 py-2.5">
                                {row.entry_price.toFixed(4)}
                              </td>
                              <td className="tabular-nums px-4 py-2.5">
                                {row.exit_price.toFixed(4)}
                              </td>
                              <td
                                className={`tabular-nums px-4 py-2.5 text-right font-semibold`}
                                style={{
                                  color: upRow ? CRYPTO_UP : CRYPTO_DOWN,
                                }}
                              >
                                {formatUsd(row.pnl_usdt)}
                              </td>
                            </tr>
                          )
                        })}
                      </tbody>
                    </table>
                  </div>
                </section>
              )}

            {Array.isArray(strategy.demo_ai_insights) &&
              strategy.demo_ai_insights.length > 0 && (
                <section className="mt-10">
                  <h2
                    className={`mb-4 flex flex-wrap items-center gap-2 text-lg font-bold text-white ${FONT_STATS_LABEL}`}
                  >
                    <Activity className="h-5 w-5 text-primary" />
                    {language === 'zh'
                      ? 'AI 思考记录（演示）'
                      : 'AI notes (demo)'}
                  </h2>
                  <ul className="space-y-3">
                    {strategy.demo_ai_insights.map((ins, i) => (
                      <li
                        key={`ai-${i}`}
                        className="rounded-xl border border-zinc-800/80 px-4 py-3 text-sm leading-relaxed text-zinc-300"
                        style={{ backgroundColor: '#121212' }}
                      >
                        <span className="block text-xs text-zinc-500">
                          {formatShortDate(ins.at, language)}{' '}
                          {new Date(ins.at).toLocaleTimeString(
                            language === 'zh' ? 'zh-CN' : 'en-US',
                            {
                              hour: '2-digit',
                              minute: '2-digit',
                            }
                          )}
                        </span>
                        <span className="mt-1 block text-zinc-200">
                          {ins.content}
                        </span>
                      </li>
                    ))}
                  </ul>
                </section>
              )}
          </>
        )}
      </main>

      {purchaseOpen && strategy && (
        <div className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/70 p-4 sm:items-center">
          <div className="max-h-[calc(100vh-2rem)] w-full max-w-lg overflow-y-auto rounded-2xl border border-zinc-700/60 bg-[#121212] p-4 shadow-2xl sm:p-6">
            <h2 className="text-lg font-bold text-white">
              {stripStrategyTitleParenthetical(strategy.name)}
            </h2>
            {isFreeSubscriptionStrategy(strategy) ? (
              <div className="mt-4 rounded-xl border border-[#d4ff33]/35 bg-[#d4ff33]/10 p-4">
                <div className="text-xs font-bold uppercase tracking-wider text-zinc-500">
                  {language === 'zh' ? '免费订阅' : 'Free subscription'}
                </div>
                <div className="mt-1 font-['Space_Grotesk',sans-serif] text-2xl font-bold tabular-nums text-[#d4ff33]">
                  0 USDT
                </div>
                <p className="mt-2 text-xs leading-relaxed text-zinc-400">
                  {language === 'zh'
                    ? '订阅本身免费。创建 COMKUN-AI 交易员后，每消费一次 AI 分析，按次从平台余额扣费。'
                    : 'The subscription is free. COMKUN-AI followers are billed from platform balance per master AI analysis broadcast consumed.'}
                </p>
              </div>
            ) : (
              <>
                <p className="mt-2 text-sm text-zinc-400">
                  {strategy.market_subscription_monthly_only
                    ? language === 'zh'
                      ? '月费订阅有效期为 30 天，到期后需续订才可继续跟单。'
                      : 'The monthly subscription is valid for 30 days and must be renewed to keep copy trading.'
                    : language === 'zh'
                      ? '选择套餐：订阅有效期内免除同步使用费。'
                      : 'Pick a plan. Sync usage fees are waived while active.'}
                </p>
                <div
                  className={`mt-4 grid grid-cols-1 gap-3 ${
                    strategy.market_subscription_monthly_only
                      ? ''
                      : 'sm:grid-cols-2'
                  }`}
                >
                  <button
                    type="button"
                    onClick={() => setPurchasePlan('monthly')}
                    className={`rounded-xl border-2 p-4 text-left transition-all ${
                      purchasePlan === 'monthly'
                        ? 'border-[#d4ff33] bg-[#d4ff33]/10 ring-1 ring-[#d4ff33]/35'
                        : 'border-zinc-700 bg-zinc-900/80 hover:border-zinc-600'
                    }`}
                  >
                    <div className="text-xs font-bold uppercase tracking-wider text-zinc-500">
                      {language === 'zh' ? '月卡 · 30 天' : 'Monthly · 30d'}
                    </div>
                    <div className="mt-1 font-['Space_Grotesk',sans-serif] text-2xl font-bold tabular-nums text-[#d4ff33]">
                      {monthlySubscriptionPrice(strategy)} USDT
                    </div>
                  </button>
                  {!strategy.market_subscription_monthly_only && (
                    <button
                      type="button"
                      disabled={Boolean(walletData?.market_weekly_trial_used)}
                      onClick={() => {
                        if (!walletData?.market_weekly_trial_used)
                          setPurchasePlan('weekly')
                      }}
                      className={`rounded-xl border-2 p-4 text-left transition-all ${
                        purchasePlan === 'weekly'
                          ? 'border-[#d4ff33] bg-[#d4ff33]/10 ring-1 ring-[#d4ff33]/35'
                          : 'border-zinc-700 bg-zinc-900/80 hover:border-zinc-600'
                      } ${walletData?.market_weekly_trial_used ? 'cursor-not-allowed opacity-50' : ''}`}
                    >
                      <div className="text-xs font-bold uppercase tracking-wider text-zinc-500">
                        {language === 'zh'
                          ? '周卡体验 · 7 天'
                          : 'Weekly trial · 7d'}
                      </div>
                      <div className="mt-1 font-['Space_Grotesk',sans-serif] text-2xl font-bold tabular-nums text-[#d4ff33]">
                        {MARKET_SUB_WEEKLY_TRIAL_USDT} USDT
                      </div>
                      {walletData?.market_weekly_trial_used ? (
                        <p className="mt-2 text-[11px] text-zinc-500">
                          {language === 'zh'
                            ? '本账号已使用过唯一一次体验'
                            : 'Trial already used on this account'}
                        </p>
                      ) : (
                        <p className="mt-2 text-[11px] text-zinc-500">
                          {language === 'zh'
                            ? '每账号仅一次，不计入充值返利统计'
                            : 'One per account; excluded from rebate stats'}
                        </p>
                      )}
                    </button>
                  )}
                </div>
              </>
            )}
            <p className="mt-4 text-xs text-zinc-500">
              {language === 'zh' ? '当前余额：' : 'Balance: '}
              <span className="tabular-nums font-semibold text-zinc-200">
                {(walletData?.balance_usdt ?? 0).toFixed(2)} USDT
              </span>
            </p>
            <div className="mt-6 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
              <button
                type="button"
                onClick={() => setPurchaseOpen(false)}
                className="h-11 rounded-lg px-4 text-sm text-zinc-400 hover:bg-zinc-800"
              >
                {language === 'zh' ? '取消' : 'Cancel'}
              </button>
              <button
                type="button"
                disabled={purchaseBusy}
                onClick={() => void submitPurchase()}
                className="market-detail-cta-gold h-11 rounded-lg px-4 text-sm font-bold text-black disabled:opacity-50"
              >
                {purchaseBusy
                  ? '…'
                  : isFreeSubscriptionStrategy(strategy)
                    ? language === 'zh'
                      ? '确认免费订阅'
                      : 'Subscribe free'
                    : language === 'zh'
                      ? '确认支付'
                      : 'Pay'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export default StrategyMarketDetailPage
