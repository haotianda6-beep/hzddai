import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import useSWR from 'swr'
import {
  TrendingUp,
  Shield,
  Zap,
  EyeOff,
  Copy,
  CheckCircle2,
  Layers,
  Target,
  Activity,
  Search,
  ChevronLeft,
  ChevronRight,
  Sparkles,
  User,
  Wallet,
} from 'lucide-react'
import { useLanguage } from '../contexts/LanguageContext'
import { useAuth } from '../contexts/AuthContext'
import { api } from '../lib/api'
import { toast } from 'sonner'
import { t } from '../i18n/translations'
import { ROUTES, strategyMarketDetailPath } from '../router/paths'
import type { StrategyMarketAccess } from '../types'
import { getModelDisplayName } from '../components/trader/model-constants'
import { getModelIcon } from '../components/common/ModelIcons'
import { getExchangeIcon } from '../components/common/ExchangeIcons'
import './strategy-market-ai3.css'
import {
  isHistoricalOnlyStrategy,
  stripStrategyTitleParenthetical,
} from '../lib/strategyMarketDisplay'
import {
  MARKET_SUB_MONTHLY_USDT,
  MARKET_SUB_WEEKLY_TRIAL_USDT,
} from '../lib/strategyMarketPricing'
import { findExistingForkIdForMarketSource } from '../lib/marketStrategyFork'
import {
  formatUserFacingError,
  throwUserFacingFetchError,
} from '../lib/userFacingFetchError'
import {
  MAINSTREAM_CALM_STRATEGY_ID,
  OKX_STABLE_STRATEGY_ID,
  BN_SMART_OPERATION_STRATEGY_ID,
  ULTIMATE_SOL_STRATEGY_ID,
  buildMainstreamCalmPerformance,
  buildOkxStablePerformance,
  buildBnSmartOperationPerformance,
} from '../components/strategy/TradeHistorySection'

/** 公开列表接口：生产环境固定走同源 /api，避免构建时带入 Docker 内部域名导致浏览器请求失败、整页无数据 */
function publicStrategiesListUrl(): string {
  if (import.meta.env.PROD) {
    return '/api/strategies/public'
  }
  const b =
    (import.meta.env.VITE_API_BASE as string | undefined)
      ?.trim()
      .replace(/\/$/, '') ?? ''
  return b ? `${b}/api/strategies/public` : '/api/strategies/public'
}

/** 币圈常见：涨绿跌红 */
const CRYPTO_UP = '#0ECB81'
const CRYPTO_DOWN = '#F6465D'

interface PublicStrategy {
  id: string
  name: string
  description: string
  author_email?: string
  creator_display_name?: string
  creator_avatar_url?: string
  market_access?: StrategyMarketAccess
  is_public: boolean
  config_visible: boolean
  market_revision?: number
  config?: any
  open_source_bundle?: Record<string, unknown>
  market_sale_price_usdt?: number
  market_subscription_monthly_only?: boolean
  market_ai_model?: string
  /** 后端根据绑定交易员推断的交易所类型，如 OKX、BINANCE */
  exchange_type?: string
  performance_only?: boolean
  performance_source?: string
  performance_disclosure?: string
  realtime_follow_available?: boolean
  stats?: {
    used_by: number
    rating: number
    subscribers?: number
    running_agents?: number
    total_agents?: number
    total_aum?: number
    return_7d_pct?: number
    cumulative_return_pct?: number
    max_drawdown_pct?: number
    trend?: number[]
    data_complete?: boolean
  }
  created_at: string
  updated_at: string
}

function marketAccessOf(s: PublicStrategy): StrategyMarketAccess {
  const a = s.market_access
  if (
    a === 'subscription' ||
    a === 'public' ||
    a === 'open_source' ||
    a === 'private'
  )
    return a
  if (s.is_public && s.config_visible) return 'public'
  if (s.is_public) return 'subscription'
  return 'private'
}

function isFreeSubscriptionStrategy(s: PublicStrategy): boolean {
  return (
    marketAccessOf(s) === 'subscription' &&
    typeof s.market_sale_price_usdt === 'number' &&
    s.market_sale_price_usdt <= 0
  )
}

function monthlySubscriptionPrice(s: PublicStrategy): number {
  return s.market_sale_price_usdt && s.market_sale_price_usdt > 0
    ? s.market_sale_price_usdt
    : MARKET_SUB_MONTHLY_USDT
}

const strategyStyles: Record<
  string,
  {
    color: string
    border: string
    glow: string
    shadow: string
    icon: typeof Zap
    bg: string
    badgeClass: string
    topBorder: string
  }
> = {
  scalper: {
    color: 'text-primary',
    border: 'border-primary/30',
    glow: 'shadow-[0_0_20px_rgba(243,255,202,0.12)]',
    shadow: 'hover:shadow-[0_0_24px_rgba(212,255,51,0.15)]',
    bg: 'bg-primary/10',
    icon: Zap,
    badgeClass: 'bg-primary/10 text-primary',
    topBorder: 'border-t-2 border-primary-container',
  },
  swing: {
    color: 'text-cyan-400',
    border: 'border-cyan-400/30',
    glow: 'shadow-[0_0_20px_rgba(34,211,238,0.12)]',
    shadow: 'hover:shadow-[0_0_24px_rgba(34,211,238,0.2)]',
    bg: 'bg-cyan-400/10',
    icon: TrendingUp,
    badgeClass: 'bg-cyan-400/10 text-cyan-300',
    topBorder: 'border-t-2 border-cyan-400/60',
  },
  arbitrage: {
    color: 'text-purple-400',
    border: 'border-purple-400/30',
    glow: 'shadow-[0_0_20px_rgba(192,132,252,0.12)]',
    shadow: 'hover:shadow-[0_0_24px_rgba(192,132,252,0.2)]',
    bg: 'bg-purple-400/10',
    icon: Layers,
    badgeClass: 'bg-purple-400/10 text-purple-300',
    topBorder: 'border-t-2 border-purple-400/50',
  },
  conservative: {
    color: 'text-secondary',
    border: 'border-secondary/30',
    glow: 'shadow-[0_0_20px_rgba(236,232,86,0.12)]',
    shadow: 'hover:shadow-[0_0_24px_rgba(236,232,86,0.18)]',
    bg: 'bg-secondary/10',
    icon: Shield,
    badgeClass: 'bg-secondary/10 text-secondary',
    topBorder: 'border-t-2 border-secondary',
  },
  aggressive: {
    color: 'text-error',
    border: 'border-error/30',
    glow: 'shadow-[0_0_20px_rgba(255,115,81,0.12)]',
    shadow: 'hover:shadow-[0_0_24px_rgba(255,115,81,0.2)]',
    bg: 'bg-error/10',
    icon: Target,
    badgeClass: 'bg-error/10 text-error',
    topBorder: 'border-t-2 border-error/70',
  },
  default: {
    color: 'text-on-surface-variant',
    border: 'border-outline-variant/30',
    glow: '',
    shadow: 'hover:shadow-[0_0_16px_rgba(255,255,255,0.04)]',
    bg: 'bg-surface-container-highest/40',
    icon: Activity,
    badgeClass: 'bg-primary/5 text-on-surface-variant',
    topBorder: 'border-t-2 border-primary-container/40',
  },
}

function getStrategyStyle(name: string) {
  const lower = name.toLowerCase()
  if (lower.includes('scalp')) return strategyStyles.scalper
  if (lower.includes('swing')) return strategyStyles.swing
  if (lower.includes('arb')) return strategyStyles.arbitrage
  if (lower.includes('safe') || lower.includes('conserv'))
    return strategyStyles.conservative
  if (lower.includes('aggress') || lower.includes('high'))
    return strategyStyles.aggressive
  return strategyStyles.default
}

function hashId(id: string): number {
  let h = 0
  for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) | 0
  return Math.abs(h)
}

function MiniSparkPath(id: string, up: boolean): string {
  const h = hashId(id)
  const pts: string[] = []
  let y = 18 + (h % 8)
  for (let x = 0; x <= 100; x += 10) {
    const wobble = ((h >> (x / 10)) % 7) - 3
    if (up) y = Math.max(4, Math.min(26, y - wobble + 1))
    else y = Math.max(4, Math.min(26, y + wobble - 1))
    pts.push(`${x},${y}`)
  }
  return `M${pts.join(' L')}`
}

function MiniSparkPathReal(
  values: number[] | undefined,
  fallbackId: string,
  up: boolean
): string {
  const clean = (values ?? []).filter((v) => Number.isFinite(v) && v > 0)
  if (clean.length < 2) return MiniSparkPath(fallbackId, up)
  const min = Math.min(...clean)
  const max = Math.max(...clean)
  const span = Math.max(1e-9, max - min)
  const last = clean.length - 1
  const pts = clean.map((v, i) => {
    const x = last === 0 ? 0 : (i / last) * 100
    const y = 26 - ((v - min) / span) * 22
    return `${x.toFixed(1)},${y.toFixed(1)}`
  })
  return `M${pts.join(' L')}`
}

function formatShortDate(dateStr: string, lang: string) {
  const d = new Date(dateStr)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleDateString(
    lang === 'zh' ? 'zh-CN' : lang === 'id' ? 'id-ID' : 'en-US',
    {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    }
  )
}

const PAGE_SIZE = 10
const HIDDEN_MARKET_STRATEGY_IDS = new Set([
  'bn-screen-mirror-ec92c8f5',
  'bn-screen-mirror-test04-2560c95f',
  'mt5-xau-martingale-bn-draft-v1',
])
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

function formatReturnPct(v: number | undefined): string {
  if (typeof v !== 'number' || !Number.isFinite(v)) return '—'
  return `${v >= 0 ? '+' : ''}${v.toFixed(2)}%`
}

function isReturnValueUp(v: number | undefined): boolean {
  return typeof v === 'number' && Number.isFinite(v) ? v >= 0 : true
}

/** 列表页单独展示的 7 日收益（详情页仍用接口原始 stats）；匹配策略名称子串 */
function marketListDisplayReturn7dPct(
  strategyName: string,
  raw: number | undefined
): number | undefined {
  const n = strategyName.trim()
  if (n.includes('只做主流币,冷静处理行情')) return raw
  if (n.includes('静默测试')) return 236.47
  if (n.includes('静默猎手')) return 18.53
  if (n.includes('小亮')) return 20.54
  return raw
}

function formatAum(v: number | undefined): string {
  if (typeof v !== 'number' || !Number.isFinite(v) || v <= 0) return '—'
  if (v >= 1_000_000) return `${(v / 1_000_000).toFixed(2)}M`
  if (v >= 1_000) return `${(v / 1_000).toFixed(2)}K`
  return v.toFixed(2)
}

function formatDrawdown(v: number | undefined): string {
  if (typeof v !== 'number' || !Number.isFinite(v)) return '—'
  return `${v.toFixed(2)}%`
}

/** 策略市场上架版本：支持接口返回小数（演示叠加），固定展示小数点后 1 位 */
function formatMarketRevision(v: number | undefined): string {
  if (typeof v !== 'number' || !Number.isFinite(v)) return '1.0'
  return (Math.round(v * 10) / 10).toFixed(1)
}

/** 策略市场列表：交易所类型 → 展示名（与部署向导模板一致） */
function exchangeDisplayName(exchangeType: string): string {
  const k = exchangeType.trim().toLowerCase()
  const map: Record<string, string> = {
    binance: 'Binance',
    bybit: 'Bybit',
    okx: 'OKX',
    bitget: 'Bitget',
    gate: 'Gate.io',
    kucoin: 'KuCoin',
    hyperliquid: 'Hyperliquid',
    aster: 'Aster',
    lighter: 'Lighter',
    indodax: 'Indodax',
  }
  return map[k] ?? exchangeType.toUpperCase()
}

function creatorDisplayName(s: PublicStrategy): string {
  const disguised = MARKET_CREATOR_DISGUISES[s.id]?.name
  if (disguised) return disguised
  const n = s.creator_display_name?.trim()
  if (n) return n
  const e = s.author_email?.split('@')[0]
  return e || '—'
}

function creatorAvatar(s: PublicStrategy): string | undefined {
  const disguised = MARKET_CREATOR_DISGUISES[s.id]?.avatar
  if (disguised) return disguised
  const u = s.creator_avatar_url?.trim()
  return u || undefined
}

function applyTradeHistoryMarketOverrides(s: PublicStrategy): PublicStrategy {
  const performance =
    s.id === MAINSTREAM_CALM_STRATEGY_ID
      ? buildMainstreamCalmPerformance(s.id)
      : s.id === OKX_STABLE_STRATEGY_ID
        ? buildOkxStablePerformance(s.id)
        : s.id === BN_SMART_OPERATION_STRATEGY_ID
          ? buildBnSmartOperationPerformance(s.id)
          : undefined
  if (!performance) return s

  const isBnSmartOperation = s.id === BN_SMART_OPERATION_STRATEGY_ID

  return {
    ...s,
    name: isBnSmartOperation ? '全智能操作,解放双手' : s.name,
    market_revision:
      s.id === MAINSTREAM_CALM_STRATEGY_ID
        ? 1.3
        : isBnSmartOperation
          ? 1.4
          : s.market_revision,
    stats: {
      ...s.stats,
      used_by: s.stats?.used_by ?? 0,
      rating: s.stats?.rating ?? 0,
      total_aum:
        s.id === OKX_STABLE_STRATEGY_ID
          ? 769054
          : isBnSmartOperation
            ? performance.initialCapital + performance.aggregate.stats.total_pnl
            : s.stats?.total_aum,
      return_7d_pct: performance.returnPct,
      max_drawdown_pct: -Math.abs(performance.aggregate.stats.max_drawdown_pct),
      trend: performance.trend,
    },
  }
}

export function StrategyMarketPage() {
  const navigate = useNavigate()
  const { language } = useLanguage()
  const { token, applyUserProfile } = useAuth()
  const [searchQuery, setSearchQuery] = useState('')
  const [listMode, setListMode] = useState<'trend' | 'recent' | 'yield'>(
    'trend'
  )
  /** 权限分类筛选 */
  const [permFilter, setPermFilter] = useState<
    'all' | 'subscription' | 'public' | 'open_source'
  >('all')
  const [page, setPage] = useState(1)
  const [purchaseStrategy, setPurchaseStrategy] =
    useState<PublicStrategy | null>(null)
  const [purchasePlan, setPurchasePlan] = useState<'monthly' | 'weekly'>(
    'monthly'
  )
  const [purchaseBusy, setPurchaseBusy] = useState(false)

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
    if (purchaseStrategy) setPurchasePlan('monthly')
  }, [purchaseStrategy])

  useEffect(() => {
    if (
      purchaseStrategy &&
      walletData?.market_weekly_trial_used &&
      purchasePlan === 'weekly'
    ) {
      setPurchasePlan('monthly')
    }
  }, [purchaseStrategy, walletData?.market_weekly_trial_used, purchasePlan])

  const tr = (key: string) => t(`strategyMarket.${key}`, language)

  const {
    data: strategies,
    error: strategiesError,
    isLoading,
    mutate: refetchStrategies,
  } = useSWR<PublicStrategy[]>(
    publicStrategiesListUrl(),
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
      const data = await response.json()
      return data.strategies || []
    },
    {
      refreshInterval: 60000,
      revalidateOnFocus: false,
      dedupingInterval: 30000,
      keepPreviousData: true,
    }
  )

  const filtered = useMemo(() => {
    if (!strategies) return []
    let list = strategies
      .filter((s) => !HIDDEN_MARKET_STRATEGY_IDS.has(s.id))
      .map(applyTradeHistoryMarketOverrides)
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase()
      list = list.filter(
        (s) =>
          s.name.toLowerCase().includes(q) ||
          (s.description || '').toLowerCase().includes(q)
      )
    }
    if (permFilter !== 'all') {
      list = list.filter((s) => marketAccessOf(s) === permFilter)
    }

    const copy = [...list]
    if (listMode === 'recent') {
      copy.sort(
        (a, b) =>
          new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime()
      )
    } else if (listMode === 'yield') {
      copy.sort((a, b) => {
        const ua =
          a.stats?.cumulative_return_pct ?? a.stats?.return_7d_pct ?? -999999
        const ub =
          b.stats?.cumulative_return_pct ?? b.stats?.return_7d_pct ?? -999999
        return ub - ua
      })
    }
    return copy
  }, [strategies, searchQuery, listMode, permFilter])

  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE))
  const pageSafe = Math.min(page, totalPages)
  const pageSlice = useMemo(() => {
    const start = (pageSafe - 1) * PAGE_SIZE
    return filtered.slice(start, start + PAGE_SIZE)
  }, [filtered, pageSafe])

  const pageButtonRange = useMemo(() => {
    const maxBtns = 5
    let end = Math.min(totalPages, pageSafe + 2)
    let start = Math.max(1, end - maxBtns + 1)
    end = Math.min(totalPages, start + maxBtns - 1)
    start = Math.max(1, end - maxBtns + 1)
    const arr: number[] = []
    for (let n = start; n <= end; n++) arr.push(n)
    return arr
  }, [pageSafe, totalPages])

  const featured = useMemo(() => filtered.slice(0, 3), [filtered])

  /**
   * 从策略市场复制一条「我的策略」并打开构建器。
   * - 已购 / 仅展示类已购权益：可拉 owned 接口写剪贴板
   * - 公开：不在市场暴露配置，不写剪贴板
   * - 开源：列表里已有完整 config，可顺带复制到剪贴板
   */
  const duplicateMarketStrategyToMine = async (
    strategy: PublicStrategy,
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
    const dupName = `${strategy.name}${suffix}`.slice(0, 200)

    let existingId: string | undefined
    try {
      const mine = await api.getStrategies()
      existingId = findExistingForkIdForMarketSource(strategy.id, mine)
    } catch {
      /* 列表拉取失败时不阻断复制 */
    }

    const finishOpenFork = async (targetId: string, reused: boolean) => {
      try {
        if (kind === 'open_source' && strategy.config) {
          await navigator.clipboard.writeText(
            JSON.stringify(strategy.config, null, 2)
          )
        } else if (kind === 'purchase' || kind === 'sub_owned') {
          const data = await api.getMarketOwnedStrategy(strategy.id)
          await navigator.clipboard.writeText(
            JSON.stringify(data.config, null, 2)
          )
        }
      } catch {
        /* 剪贴板失败不阻断 */
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

    const { id } = await api.duplicateStrategy(strategy.id, dupName)
    await finishOpenFork(id, false)
  }

  const handleActionClick = (strategy: PublicStrategy) => {
    if (isHistoricalOnlyStrategy(strategy)) {
      toast.info(
        language === 'zh'
          ? '该策略尚未绑定实时主控，暂不支持跟单'
          : 'This strategy is not connected to a live master yet'
      )
      return
    }
    const acc = marketAccessOf(strategy)
    if (acc === 'private' || acc === 'subscription') {
      if (!token) {
        toast.error(language === 'zh' ? '请先登录' : 'Please sign in')
        navigate(ROUTES.login)
        return
      }
      if (entitlementIds.has(strategy.id)) {
        void (async () => {
          try {
            await duplicateMarketStrategyToMine(strategy, 'sub_owned')
          } catch (e) {
            toast.error(e instanceof Error ? e.message : tr('copyFailed'))
          }
        })()
        return
      }
      setPurchaseStrategy(strategy)
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
            strategy,
            acc === 'open_source' ? 'open_source' : 'public'
          )
        } catch (e) {
          toast.error(e instanceof Error ? e.message : tr('copyFailed'))
        }
      })()
    }
  }

  const submitPurchase = async () => {
    if (!purchaseStrategy) return
    setPurchaseBusy(true)
    try {
      const res = await api.postMarketPurchase(
        purchaseStrategy.id,
        isFreeSubscriptionStrategy(purchaseStrategy) ? 'free' : purchasePlan
      )
      toast.success(
        res.message || (language === 'zh' ? '购买成功' : 'Purchased')
      )
      await mutateWallet()
      applyUserProfile({ balance_usdt: res.balance_usdt })
      try {
        await duplicateMarketStrategyToMine(purchaseStrategy, 'purchase')
      } catch (dupErr) {
        toast.error(tr('toastPurchasedDupFailed'))
        console.warn(dupErr)
      }
      setPurchaseStrategy(null)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '购买失败')
    } finally {
      setPurchaseBusy(false)
    }
  }

  const agentCount = (s: PublicStrategy) => s.stats?.used_by ?? 0
  const renderActionButton = (s: PublicStrategy, fullWidth = false) => {
    const acc = marketAccessOf(s)
    const owned = entitlementIds.has(s.id)
    const paid = acc === 'subscription' || acc === 'private'
    const historicalOnly = isHistoricalOnlyStrategy(s)

    let label: string
    let icon: JSX.Element
    let btnClass: string

    if (historicalOnly) {
      label = language === 'zh' ? '仅历史展示' : 'Historical only'
      icon = <EyeOff className="h-3.5 w-3.5" aria-hidden />
      btnClass =
        'cursor-not-allowed border border-amber-500/30 bg-amber-500/10 text-amber-200 opacity-90'
    } else if (acc === 'public' || acc === 'open_source') {
      label = tr('actionAddToStrategies')
      icon = <Copy className="h-3.5 w-3.5" aria-hidden />
      btnClass =
        'bg-surface-container-highest text-primary hover:bg-primary-container hover:text-on-primary-container'
    } else if (paid && owned) {
      label = language === 'zh' ? '已购' : 'Purchased'
      icon = <CheckCircle2 className="h-3.5 w-3.5" aria-hidden />
      btnClass =
        'border border-emerald-400/35 bg-[#0ECB81] text-black shadow-[0_0_18px_rgba(14,203,129,0.35)] hover:brightness-110 active:brightness-95'
    } else if (paid && !owned) {
      label = language === 'zh' ? '订阅' : 'Subscribe'
      icon = <Sparkles className="h-3.5 w-3.5" aria-hidden />
      btnClass =
        'market-detail-cta-gold border border-[#c9e820]/45 text-black shadow-[0_0_18px_rgba(212,255,51,0.28)] hover:brightness-105 active:brightness-95'
    } else {
      label = tr('actionViewOnly')
      icon = <EyeOff className="h-3.5 w-3.5 opacity-80" aria-hidden />
      btnClass =
        'border border-outline-variant/25 bg-surface-container-high text-on-surface-variant hover:border-outline-variant/40 hover:text-on-surface'
    }

    return (
      <button
        type="button"
        disabled={historicalOnly}
        onClick={() => handleActionClick(s)}
        title={
          historicalOnly
            ? language === 'zh'
              ? '尚未绑定实时主控，暂不支持跟单'
              : 'Live master route is unavailable'
            : undefined
        }
        className={`inline-flex h-11 items-center justify-center rounded-lg px-3 text-xs font-bold transition-all ${fullWidth ? 'w-full' : ''} ${btnClass}`}
      >
        <span className="inline-flex items-center gap-1">
          {icon}
          {label}
        </span>
      </button>
    )
  }

  return (
    <div className="strategy-market-ai3 min-h-screen bg-surface pb-14 text-on-surface selection:bg-primary/30 selection:text-on-primary-container font-[Inter,system-ui,sans-serif]">
      {/* 顶栏由全局 AppChrome + HeaderBar 提供，此处不再重复固定导航 */}
      <main className="min-h-screen pt-0">
        <div
          className="fixed inset-0 -z-10 ai3-noise bg-cover bg-center"
          aria-hidden
        />

        <div className="px-4 pb-8 pt-6 sm:px-8 sm:pt-8">
          <div className="mb-8 flex flex-col gap-4 text-left sm:mb-10">
            <div className="w-full max-w-3xl">
              <h1 className="font-['Space_Grotesk',sans-serif] text-3xl font-bold tracking-tight text-on-surface sm:text-4xl">
                {tr('title')}
              </h1>
              <p className="mt-2 font-light text-on-surface-variant">
                {tr('descriptionLum')}
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-3">
              <div className="flex items-center rounded-full border border-outline-variant/10 bg-surface-container-high p-1">
                {(['trend', 'recent', 'yield'] as const).map((m) => (
                  <button
                    key={m}
                    type="button"
                    onClick={() => {
                      setListMode(m)
                      setPage(1)
                    }}
                    className={`rounded-full px-3 py-1.5 text-xs font-bold uppercase transition-colors sm:px-4 ${
                      listMode === m
                        ? 'bg-primary-container text-on-primary-container'
                        : 'text-on-surface-variant hover:text-on-surface'
                    }`}
                  >
                    {m === 'trend'
                      ? tr('filterTrend')
                      : m === 'recent'
                        ? tr('filterRecent')
                        : tr('filterHighYield')}
                  </button>
                ))}
              </div>
            </div>
            <div className="flex flex-col gap-3 rounded-xl border border-outline-variant/15 bg-surface-container-low/60 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
              <div className="flex flex-wrap items-center gap-3 text-sm">
                <Wallet
                  className="h-4 w-4 shrink-0 text-primary-container"
                  aria-hidden
                />
                <span className="text-on-surface-variant">
                  {language === 'zh'
                    ? '站内余额（购买订阅/仅展示策略）'
                    : 'Platform balance'}
                </span>
                <span className="font-['Space_Grotesk',sans-serif] text-lg font-bold tabular-nums text-[#d4ff33]">
                  {(walletData?.balance_usdt ?? 0).toFixed(2)} USDT
                </span>
              </div>
              {token ? (
                <button
                  type="button"
                  onClick={() => navigate(ROUTES.recharge)}
                  className="rounded-lg bg-primary-container px-4 py-2 text-xs font-bold text-on-primary-container transition-opacity hover:opacity-95"
                >
                  {language === 'zh' ? '去充值' : 'Recharge'}
                </button>
              ) : (
                <span className="text-xs text-on-surface-variant">
                  {language === 'zh'
                    ? '登录后可充值与购买'
                    : 'Sign in to recharge'}
                </span>
              )}
            </div>
          </div>

          {token &&
            (walletData?.market_subscription_alerts?.length ?? 0) > 0 && (
              <div className="mb-6 space-y-2">
                {(walletData?.market_subscription_alerts ?? []).map((a, i) => (
                  <div
                    key={`${a.strategy_id}-${a.kind}-${i}`}
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

          {strategiesError && (
            <div className="mb-6 rounded-xl border border-red-500/35 bg-red-950/50 px-4 py-4 text-left text-sm text-red-100">
              <p className="font-bold">
                {language === 'zh'
                  ? '策略列表加载失败'
                  : 'Failed to load market strategies'}
              </p>
              <p className="mt-2 text-sm text-red-200/90">
                {formatUserFacingError(
                  strategiesError,
                  language === 'zh' ? 'zh' : 'en'
                )}
              </p>
              <button
                type="button"
                onClick={() => void refetchStrategies()}
                className="mt-3 rounded-lg bg-red-500/25 px-4 py-2 text-xs font-bold text-red-50 hover:bg-red-500/35"
              >
                {language === 'zh' ? '重试' : 'Retry'}
              </button>
            </div>
          )}

          {isLoading && strategies === undefined && !strategiesError && (
            <div className="flex flex-col items-center justify-center py-24">
              <Sparkles className="h-10 w-10 animate-pulse text-primary-container" />
              <p className="mt-4 text-sm text-on-surface-variant">
                {tr('loading')}
              </p>
            </div>
          )}

          {!isLoading &&
            !strategiesError &&
            strategies !== undefined &&
            strategies.length === 0 && (
              <div className="rounded-xl border border-outline-variant/15 bg-surface-container-low py-24 text-center">
                <Activity className="mx-auto mb-3 h-12 w-12 text-on-surface-variant/40" />
                <h3 className="font-['Space_Grotesk',sans-serif] text-lg font-bold text-on-surface">
                  {tr('noStrategies')}
                </h3>
                <p className="mt-2 text-sm text-on-surface-variant">
                  {tr('noStrategiesDesc')}
                </p>
              </div>
            )}

          {!isLoading &&
            !strategiesError &&
            strategies !== undefined &&
            strategies.length > 0 &&
            filtered.length === 0 && (
              <div className="rounded-xl border border-amber-500/25 bg-amber-950/20 px-4 py-8 text-center text-sm text-amber-100">
                {language === 'zh'
                  ? '当前筛选或搜索条件下没有策略，请清空搜索或把权限筛选项改为「全部」。'
                  : 'No strategies match the current filters. Reset search or set access filter to “All”.'}
              </div>
            )}

          {!isLoading && filtered.length > 0 && (
            <>
              <div className="mb-10 grid grid-cols-1 gap-6 md:grid-cols-3">
                {featured.map((s, idx) => {
                  const st = getStrategyStyle(s.name)
                  const cname = creatorDisplayName(s)
                  const cav = creatorAvatar(s)
                  const modelId = s.market_ai_model || ''
                  const modelName = getModelDisplayName(modelId)
                  const modelIcon = getModelIcon(modelId, {
                    width: 16,
                    height: 16,
                    className: 'opacity-90',
                  })
                  const exchangeSlug = (s.exchange_type ?? '')
                    .trim()
                    .toLowerCase()
                  const retValue = marketListDisplayReturn7dPct(
                    s.name,
                    s.stats?.cumulative_return_pct ?? s.stats?.return_7d_pct
                  )
                  const ret = formatReturnPct(retValue)
                  const up = isReturnValueUp(retValue)
                  const subs = s.stats?.subscribers ?? 0
                  const rankLabel =
                    language === 'zh'
                      ? `第 ${idx + 1} 位`
                      : language === 'id'
                        ? `#${idx + 1}`
                        : `#${idx + 1}`
                  const stratBadge =
                    language === 'zh'
                      ? '策略'
                      : language === 'id'
                        ? 'Strategi'
                        : 'Strategy'
                  const to = strategyMarketDetailPath(s.id)
                  return (
                    <Link
                      key={`feat-${s.id}`}
                      to={to}
                      onClick={(e) => {
                        if (!token) {
                          e.preventDefault()
                          navigate(ROUTES.login)
                        }
                      }}
                      className={`group relative block overflow-hidden rounded-xl bg-nofx-bg-secondary p-5 text-left no-underline transition-all hover:bg-nofx-bg-tertiary hover:ring-1 hover:ring-primary/30 focus-visible:outline focus-visible:ring-2 focus-visible:ring-primary ${st.topBorder} ${st.shadow} text-[#eaecef] visited:text-[#eaecef]`}
                    >
                      <div className="mb-1 flex items-center justify-between gap-2 text-[10px] font-bold uppercase tracking-wider text-zinc-500">
                        <span>{rankLabel}</span>
                      </div>
                      <div className="relative mb-4 flex items-start justify-between gap-3">
                        <span className="shrink-0 rounded-md bg-[#2b3139] px-2 py-1 text-[10px] font-bold tracking-wide text-zinc-200">
                          {stratBadge}
                        </span>
                        <div className="relative flex shrink-0 items-end gap-2">
                          <svg
                            className="pointer-events-none h-9 w-[4.5rem] opacity-35"
                            viewBox="0 0 100 30"
                            aria-hidden
                          >
                            <path
                              d={MiniSparkPathReal(s.stats?.trend, s.id, up)}
                              fill="none"
                              stroke={up ? CRYPTO_UP : CRYPTO_DOWN}
                              strokeWidth="2"
                            />
                          </svg>
                          <span
                            className="font-['Space_Grotesk',sans-serif] text-lg font-bold tabular-nums"
                            style={{ color: up ? CRYPTO_UP : CRYPTO_DOWN }}
                          >
                            {ret}
                          </span>
                        </div>
                      </div>
                      <h3 className="mb-3 font-['Space_Grotesk',sans-serif] text-2xl font-bold leading-tight text-white sm:text-[1.65rem]">
                        {stripStrategyTitleParenthetical(s.name)}
                      </h3>
                      <div className="mb-4 border-b border-zinc-600/60" />
                      <div className="space-y-2.5 text-sm text-zinc-400">
                        <div className="flex flex-wrap items-center gap-2">
                          <User
                            className="h-4 w-4 shrink-0 text-zinc-500"
                            aria-hidden
                          />
                          {cav ? (
                            <img
                              src={cav}
                              alt=""
                              className="h-7 w-7 shrink-0 rounded-full border border-zinc-600 object-cover"
                            />
                          ) : (
                            <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-zinc-700 text-xs font-bold text-zinc-300">
                              {cname[0] ?? '?'}
                            </div>
                          )}
                          <span className="max-w-[10rem] truncate font-medium text-zinc-200">
                            {cname}
                          </span>
                          <span className="text-zinc-600">·</span>
                          <span className="text-zinc-400">
                            {subs} {tr('subsLabel')}
                          </span>
                        </div>
                        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
                          <span className="inline-flex items-center gap-2">
                            <span
                              className="h-4 w-4 shrink-0 text-center text-zinc-500"
                              aria-hidden
                            >
                              {modelIcon || (
                                <span className="text-[11px] font-bold">
                                  AI
                                </span>
                              )}
                            </span>
                            <span className="text-zinc-300">
                              {language === 'zh' ? 'AI 模型' : 'AI'}：
                              {modelName}
                            </span>
                          </span>
                          {exchangeSlug ? (
                            <span className="inline-flex items-center gap-2">
                              <span className="text-zinc-600" aria-hidden>
                                ·
                              </span>
                              <span
                                className="flex shrink-0 items-center"
                                aria-hidden
                              >
                                {getExchangeIcon(exchangeSlug, {
                                  width: 16,
                                  height: 16,
                                  className: 'opacity-90',
                                })}
                              </span>
                              <span className="text-zinc-300">
                                {language === 'zh' ? '交易所' : 'Exchange'}：
                                {exchangeDisplayName(exchangeSlug)}
                              </span>
                            </span>
                          ) : null}
                        </div>
                        <div className="flex items-center gap-2">
                          <Wallet
                            className="h-4 w-4 shrink-0 text-zinc-500"
                            aria-hidden
                          />
                          <span className="tabular-nums text-zinc-300">
                            {formatAum(s.stats?.total_aum)} USDT
                          </span>
                        </div>
                      </div>
                    </Link>
                  )
                })}
              </div>

              <section className="signal-glow overflow-hidden rounded-xl border border-outline-variant/10 bg-surface-container-low">
                <div className="flex flex-col gap-4 border-b border-outline-variant/5 px-4 py-4 sm:flex-row sm:items-center sm:justify-between sm:px-6">
                  <h2 className="shrink-0 font-['Space_Grotesk',sans-serif] text-lg font-bold text-on-surface">
                    {tr('tableTitle')}
                  </h2>
                  <div className="flex min-w-0 flex-1 flex-col gap-3 sm:flex-row sm:items-center sm:justify-end">
                    <div className="relative w-full min-w-0 sm:max-w-xs md:max-w-md">
                      <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-on-surface-variant" />
                      <input
                        type="search"
                        value={searchQuery}
                        onChange={(e) => {
                          setSearchQuery(e.target.value)
                          setPage(1)
                        }}
                        placeholder={tr('topSearchPlaceholder')}
                        className="w-full rounded-lg border-none bg-surface-container-highest py-2 pl-10 pr-3 text-sm text-on-surface placeholder:text-on-surface-variant/60 focus:outline-none focus:ring-1 focus:ring-primary"
                      />
                    </div>
                    <label className="flex w-full shrink-0 flex-col gap-1 sm:w-auto sm:min-w-[11rem]">
                      <span className="text-[10px] font-bold uppercase tracking-wider text-on-surface-variant">
                        {tr('permFilterLabel')}
                      </span>
                      <select
                        value={permFilter}
                        onChange={(e) => {
                          setPermFilter(
                            e.target.value as
                              | 'all'
                              | 'subscription'
                              | 'public'
                              | 'open_source'
                          )
                          setPage(1)
                        }}
                        className="w-full cursor-pointer rounded-lg border border-outline-variant/20 bg-surface-container-highest py-2 pl-3 pr-8 text-sm text-on-surface focus:outline-none focus:ring-1 focus:ring-primary"
                      >
                        <option value="all">{tr('permFilterAll')}</option>
                        <option value="subscription">
                          {tr('permFilterSubscription')}
                        </option>
                        <option value="public">{tr('permFilterPublic')}</option>
                        <option value="open_source">
                          {tr('permFilterOpenSource')}
                        </option>
                      </select>
                    </label>
                  </div>
                </div>
                <div className="divide-y divide-outline-variant/10 md:hidden">
                  {pageSlice.map((s, rowIdx) => {
                    const st = getStrategyStyle(s.name)
                    const Icon = st.icon
                    const cname = creatorDisplayName(s)
                    const cav = creatorAvatar(s)
                    const modelId = s.market_ai_model || ''
                    const modelName = getModelDisplayName(modelId)
                    const modelIcon = getModelIcon(modelId, {
                      width: 14,
                      height: 14,
                      className: 'opacity-90',
                    })
                    const exchangeSlug = (s.exchange_type ?? '')
                      .trim()
                      .toLowerCase()
                    const retValue = marketListDisplayReturn7dPct(
                      s.name,
                      s.stats?.cumulative_return_pct ?? s.stats?.return_7d_pct
                    )
                    const ret = formatReturnPct(retValue)
                    const up = isReturnValueUp(retValue)
                    const agents = s.stats?.running_agents ?? agentCount(s)
                    const seq = (pageSafe - 1) * PAGE_SIZE + rowIdx + 1
                    const acc = marketAccessOf(s)
                    const accessLabel =
                      acc === 'subscription'
                        ? tr('badgeAccessSubscription')
                        : acc === 'public'
                          ? tr('badgeAccessPublic')
                          : acc === 'open_source'
                            ? tr('badgeAccessOpenSource')
                            : tr('badgeAccessPrivate')
                    const accessClass =
                      acc === 'subscription'
                        ? 'border-amber-500/30 bg-amber-500/10 text-amber-300'
                        : acc === 'public'
                          ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-300'
                          : acc === 'open_source'
                            ? 'border-sky-500/30 bg-sky-500/10 text-sky-300'
                            : 'border-outline-variant/25 bg-surface-container-highest text-on-surface-variant'

                    return (
                      <article key={`mobile-${s.id}`} className="px-4 py-4">
                        <div className="mb-3 flex items-start gap-3">
                          <div
                            className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-lg ${st.bg} ${st.color}`}
                          >
                            <Icon className="h-5 w-5" />
                          </div>
                          <div className="min-w-0 flex-1">
                            <div className="mb-1 flex items-center gap-2 text-[10px] font-bold uppercase tracking-wider text-on-surface-variant/70">
                              <span>#{seq}</span>
                              <span
                                className={`rounded-full border px-2 py-0.5 ${accessClass}`}
                              >
                                {accessLabel}
                              </span>
                            </div>
                            <Link
                              to={strategyMarketDetailPath(s.id)}
                              className="line-clamp-2 text-base font-bold leading-snug text-on-surface hover:text-primary"
                            >
                              {stripStrategyTitleParenthetical(s.name)}
                            </Link>
                            <div className="mt-2 flex min-w-0 items-center gap-2 text-xs text-on-surface-variant">
                              {cav ? (
                                <img
                                  src={cav}
                                  alt=""
                                  className="h-6 w-6 shrink-0 rounded-full object-cover ring-1 ring-outline-variant/30"
                                />
                              ) : (
                                <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-surface-container-highest text-[10px] font-bold">
                                  {cname[0] ?? '?'}
                                </div>
                              )}
                              <span className="truncate">{cname}</span>
                            </div>
                          </div>
                        </div>

                        <div className="grid grid-cols-2 gap-2 rounded-xl bg-surface-container-high/45 p-3 text-xs">
                          <div>
                            <div className="text-on-surface-variant">
                              {tr('colReturn7d')}
                            </div>
                            <div
                              className="mt-1 font-['Space_Grotesk',sans-serif] text-base font-bold tabular-nums"
                              style={{ color: up ? CRYPTO_UP : CRYPTO_DOWN }}
                            >
                              {ret}
                            </div>
                          </div>
                          <div>
                            <div className="text-on-surface-variant">
                              {tr('colAum')}
                            </div>
                            <div className="mt-1 truncate font-['Space_Grotesk',sans-serif] text-sm font-semibold tabular-nums text-on-surface">
                              {formatAum(s.stats?.total_aum)} USDT
                            </div>
                          </div>
                          <div>
                            <div className="text-on-surface-variant">
                              {tr('colSubs')}
                            </div>
                            <div className="mt-1 font-['Space_Grotesk',sans-serif] text-sm font-semibold tabular-nums text-on-surface">
                              {s.stats?.subscribers ?? 0}
                            </div>
                          </div>
                          <div>
                            <div className="text-on-surface-variant">
                              {tr('colAgentsTitle')}
                            </div>
                            <div className="mt-1 inline-flex items-center gap-1.5 font-['Space_Grotesk',sans-serif] text-sm font-semibold tabular-nums text-on-surface">
                              <span
                                className={`h-1.5 w-1.5 rounded-full ${
                                  agents > 0
                                    ? 'bg-[#0ECB81]'
                                    : 'bg-on-surface-variant/40'
                                }`}
                              />
                              {agents}
                            </div>
                          </div>
                        </div>

                        <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-on-surface-variant/85">
                          <span className="inline-flex min-w-0 items-center gap-1.5">
                            <span aria-hidden>
                              {modelIcon || (
                                <span className="text-[10px] font-bold">
                                  AI
                                </span>
                              )}
                            </span>
                            <span className="truncate">AI：{modelName}</span>
                          </span>
                          {exchangeSlug ? (
                            <span className="inline-flex min-w-0 items-center gap-1.5">
                              <span
                                className="flex shrink-0 items-center"
                                aria-hidden
                              >
                                {getExchangeIcon(exchangeSlug, {
                                  width: 14,
                                  height: 14,
                                  className: 'opacity-90',
                                })}
                              </span>
                              <span className="truncate">
                                {language === 'zh' ? '交易所' : 'Exchange'}：
                                {exchangeDisplayName(exchangeSlug)}
                              </span>
                            </span>
                          ) : null}
                        </div>

                        <div className="mt-4">
                          {renderActionButton(s, true)}
                        </div>
                      </article>
                    )
                  })}
                </div>

                <div className="hidden overflow-x-auto md:block">
                  <table className="w-full min-w-[960px] border-collapse text-left">
                    <thead>
                      <tr className="text-[11px] font-bold uppercase tracking-wider text-on-surface-variant">
                        <th className="w-12 px-2 py-3 text-center sm:px-3">
                          {tr('colIndex')}
                        </th>
                        <th className="px-4 py-3 sm:px-6">
                          {tr('colNameAuthor')}
                        </th>
                        <th className="min-w-[8.5rem] whitespace-nowrap px-4 py-3">
                          {tr('colPermission')}
                        </th>
                        <th className="min-w-[9rem] whitespace-nowrap px-4 py-3">
                          {tr('colAum')}
                        </th>
                        <th className="px-3 py-3">{tr('colReturn7d')}</th>
                        <th className="px-3 py-3 text-center">
                          {tr('colDrawdown')}
                        </th>
                        <th className="px-3 py-3">{tr('colSubs')}</th>
                        <th className="px-3 py-3 normal-case">
                          <span className="block leading-tight">
                            {tr('colAgentsTitle')}
                          </span>
                          <span className="mt-0.5 block text-[9px] font-normal tracking-normal text-on-surface-variant">
                            {tr('colAgentsSub')}
                          </span>
                        </th>
                        <th className="px-3 py-3">{tr('colRevision')}</th>
                        <th className="px-3 py-3">{tr('colPublished')}</th>
                        <th className="px-3 py-3">{tr('colTrend')}</th>
                        <th className="px-4 py-3 text-right sm:px-6">
                          {tr('colAction')}
                        </th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-outline-variant/5">
                      {pageSlice.map((s, rowIdx) => {
                        const st = getStrategyStyle(s.name)
                        const Icon = st.icon
                        const cname = creatorDisplayName(s)
                        const cav = creatorAvatar(s)
                        const modelId = s.market_ai_model || ''
                        const modelName = getModelDisplayName(modelId)
                        const modelIcon = getModelIcon(modelId, {
                          width: 14,
                          height: 14,
                          className: 'opacity-90',
                        })
                        const exchangeSlug = (s.exchange_type ?? '')
                          .trim()
                          .toLowerCase()
                        const retValue = marketListDisplayReturn7dPct(
                          s.name,
                          s.stats?.cumulative_return_pct ??
                            s.stats?.return_7d_pct
                        )
                        const ret = formatReturnPct(retValue)
                        const up = isReturnValueUp(retValue)
                        const agents = s.stats?.running_agents ?? agentCount(s)
                        const seq = (pageSafe - 1) * PAGE_SIZE + rowIdx + 1
                        return (
                          <tr
                            key={s.id}
                            className="group transition-colors hover:bg-surface-container-high/80"
                          >
                            <td className="px-2 py-4 text-center font-mono text-xs tabular-nums text-on-surface-variant sm:px-3">
                              {seq}
                            </td>
                            <td className="px-4 py-4 sm:px-6">
                              <div className="flex items-center gap-3">
                                <div
                                  className={`flex h-10 w-10 shrink-0 items-center justify-center rounded ${st.bg} ${st.color}`}
                                >
                                  <Icon className="h-5 w-5" />
                                </div>
                                <div className="min-w-0">
                                  <Link
                                    to={strategyMarketDetailPath(s.id)}
                                    onClick={(e) => e.stopPropagation()}
                                    className="block truncate text-lg font-bold text-on-surface hover:text-primary"
                                  >
                                    {stripStrategyTitleParenthetical(s.name)}
                                  </Link>
                                  <div className="mt-1 flex items-center gap-2">
                                    {cav ? (
                                      <img
                                        src={cav}
                                        alt=""
                                        className="h-6 w-6 shrink-0 rounded-full object-cover ring-1 ring-outline-variant/30"
                                      />
                                    ) : (
                                      <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-surface-container-highest text-[10px] font-bold text-on-surface-variant">
                                        {cname[0] ?? '?'}
                                      </div>
                                    )}
                                    <span className="truncate text-xs text-on-surface-variant">
                                      {cname}
                                    </span>
                                  </div>
                                  <div className="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[11px] text-on-surface-variant/80">
                                    <span className="inline-flex min-w-0 items-center gap-1.5">
                                      <span aria-hidden>
                                        {modelIcon || (
                                          <span className="text-[10px] font-bold">
                                            AI
                                          </span>
                                        )}
                                      </span>
                                      <span className="truncate">
                                        {language === 'zh' ? 'AI' : 'AI'}：
                                        {modelName}
                                      </span>
                                    </span>
                                    {exchangeSlug ? (
                                      <span className="inline-flex items-center gap-1.5">
                                        <span
                                          className="text-on-surface-variant/45"
                                          aria-hidden
                                        >
                                          ·
                                        </span>
                                        <span
                                          className="flex shrink-0 items-center"
                                          aria-hidden
                                        >
                                          {getExchangeIcon(exchangeSlug, {
                                            width: 14,
                                            height: 14,
                                            className: 'opacity-90',
                                          })}
                                        </span>
                                        <span className="truncate">
                                          {language === 'zh'
                                            ? '交易所'
                                            : 'Exchange'}
                                          ：{exchangeDisplayName(exchangeSlug)}
                                        </span>
                                      </span>
                                    ) : null}
                                  </div>
                                </div>
                              </div>
                            </td>
                            <td className="min-w-[8.5rem] whitespace-nowrap px-4 py-4 align-middle pr-5">
                              {(() => {
                                const acc = marketAccessOf(s)
                                const badgeBase =
                                  'inline-flex shrink-0 whitespace-nowrap rounded-full px-2.5 py-1 text-[10px] leading-tight'
                                if (acc === 'subscription') {
                                  return (
                                    <span
                                      className={`${badgeBase} border border-amber-500/30 bg-amber-500/10 text-amber-300`}
                                    >
                                      {tr('badgeAccessSubscription')}
                                    </span>
                                  )
                                }
                                if (acc === 'public') {
                                  return (
                                    <span
                                      className={`${badgeBase} border border-emerald-500/30 bg-emerald-500/10 text-emerald-300`}
                                    >
                                      {tr('badgeAccessPublic')}
                                    </span>
                                  )
                                }
                                if (acc === 'open_source') {
                                  return (
                                    <span
                                      className={`${badgeBase} border border-sky-500/30 bg-sky-500/10 text-sky-300`}
                                    >
                                      {tr('badgeAccessOpenSource')}
                                    </span>
                                  )
                                }
                                return (
                                  <span
                                    className={`${badgeBase} border border-outline-variant/25 bg-surface-container-highest text-on-surface-variant`}
                                  >
                                    {tr('badgeAccessPrivate')}
                                  </span>
                                )
                              })()}
                            </td>
                            <td className="min-w-[9rem] whitespace-nowrap pl-2 pr-4 font-['Space_Grotesk',sans-serif] text-sm font-medium tabular-nums text-on-surface/90">
                              <span className="inline-block whitespace-nowrap">
                                {formatAum(s.stats?.total_aum)} USDT
                              </span>
                            </td>
                            <td
                              className="px-3 py-4 font-['Space_Grotesk',sans-serif] text-sm font-bold tabular-nums"
                              style={{ color: up ? CRYPTO_UP : CRYPTO_DOWN }}
                            >
                              {ret}
                            </td>
                            <td className="px-3 py-4 text-center font-['Space_Grotesk',sans-serif] text-sm text-on-surface-variant">
                              {formatDrawdown(s.stats?.max_drawdown_pct)}
                            </td>
                            <td className="px-3 py-4 text-sm text-on-surface">
                              {s.stats?.subscribers ?? 0}
                            </td>
                            <td className="px-3 py-4">
                              <div className="flex flex-col items-start gap-0.5">
                                <span
                                  className={`h-1.5 w-1.5 shrink-0 rounded-full ${
                                    agents > 0
                                      ? 'animate-pulse bg-[#0ECB81]'
                                      : 'bg-on-surface-variant/40'
                                  }`}
                                />
                                <span className="font-['Space_Grotesk',sans-serif] text-sm font-bold tabular-nums text-on-surface">
                                  {agents}
                                </span>
                              </div>
                            </td>
                            <td className="px-3 py-4 font-mono text-xs tabular-nums text-primary">
                              {formatMarketRevision(s.market_revision)}
                            </td>
                            <td className="px-3 py-4 text-xs text-on-surface-variant">
                              {formatShortDate(s.updated_at, language)}
                            </td>
                            <td className="px-3 py-4">
                              <svg
                                className="h-8 w-24"
                                viewBox="0 0 100 30"
                                aria-hidden
                              >
                                <path
                                  d={MiniSparkPathReal(
                                    s.stats?.trend,
                                    s.id,
                                    up
                                  )}
                                  fill="none"
                                  stroke={up ? CRYPTO_UP : CRYPTO_DOWN}
                                  strokeWidth="2"
                                />
                              </svg>
                            </td>
                            <td className="px-4 py-4 text-right sm:px-6">
                              {renderActionButton(s)}
                            </td>
                          </tr>
                        )
                      })}
                    </tbody>
                  </table>
                </div>
                <div className="flex flex-col items-center justify-between gap-3 border-t border-outline-variant/5 bg-surface-container px-4 py-3 text-xs text-on-surface-variant sm:flex-row sm:px-6">
                  <span>
                    {tr('paginationShow')
                      .replace(
                        '{{from}}',
                        String((pageSafe - 1) * PAGE_SIZE + 1)
                      )
                      .replace(
                        '{{to}}',
                        String(Math.min(pageSafe * PAGE_SIZE, filtered.length))
                      )
                      .replace('{{total}}', String(filtered.length))}
                  </span>
                  <div className="flex items-center gap-1">
                    <button
                      type="button"
                      disabled={pageSafe <= 1}
                      onClick={() => setPage((p) => Math.max(1, p - 1))}
                      className="flex h-8 w-8 items-center justify-center rounded bg-surface-container-high transition-colors hover:bg-surface-container-highest disabled:opacity-40"
                    >
                      <ChevronLeft className="h-4 w-4" />
                    </button>
                    {pageButtonRange.map((n) => (
                      <button
                        key={n}
                        type="button"
                        onClick={() => setPage(n)}
                        className={`flex h-8 w-8 items-center justify-center rounded text-xs font-bold transition-colors ${
                          pageSafe === n
                            ? 'bg-primary-container text-on-primary-container'
                            : 'bg-surface-container-high hover:bg-surface-container-highest'
                        }`}
                      >
                        {n}
                      </button>
                    ))}
                    {totalPages >
                      pageButtonRange[pageButtonRange.length - 1]! && (
                      <span className="px-1">…</span>
                    )}
                    <button
                      type="button"
                      disabled={pageSafe >= totalPages}
                      onClick={() =>
                        setPage((p) => Math.min(totalPages, p + 1))
                      }
                      className="flex h-8 w-8 items-center justify-center rounded bg-surface-container-high transition-colors hover:bg-surface-container-highest disabled:opacity-40"
                    >
                      <ChevronRight className="h-4 w-4" />
                    </button>
                  </div>
                </div>
              </section>
            </>
          )}
        </div>

        {purchaseStrategy && (
          <div className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/70 p-4 sm:items-center">
            <div className="max-h-[calc(100vh-2rem)] w-full max-w-lg overflow-y-auto rounded-2xl border border-outline-variant/20 bg-surface-container-high p-4 shadow-2xl sm:p-6">
              <h2 className="text-lg font-bold text-on-surface">
                {stripStrategyTitleParenthetical(purchaseStrategy.name)}
              </h2>
              {isFreeSubscriptionStrategy(purchaseStrategy) ? (
                <div className="mt-4 rounded-xl border border-[#d4ff33]/35 bg-primary-container/15 p-4">
                  <div className="text-xs font-bold uppercase tracking-wider text-on-surface-variant">
                    {language === 'zh' ? '免费订阅' : 'Free subscription'}
                  </div>
                  <div className="mt-1 font-['Space_Grotesk',sans-serif] text-2xl font-bold tabular-nums text-[#d4ff33]">
                    0 USDT
                  </div>
                  <p className="mt-2 text-xs leading-relaxed text-on-surface-variant">
                    {language === 'zh'
                      ? '订阅本身免费。创建 COMKUN-AI 交易员后，每消费一次 AI 分析，按次从平台余额扣费。'
                      : 'The subscription is free. COMKUN-AI followers are billed from platform balance per master AI analysis broadcast consumed.'}
                  </p>
                </div>
              ) : (
                <>
                  <p className="mt-2 text-sm text-on-surface-variant">
                    {purchaseStrategy.market_subscription_monthly_only
                      ? language === 'zh'
                        ? '月费订阅有效期为 30 天，到期后需续订才可继续跟单。'
                        : 'The monthly subscription is valid for 30 days and must be renewed to keep copy trading.'
                      : language === 'zh'
                        ? '选择套餐：订阅期内使用同步策略不再按轮扣除站内余额。'
                        : 'Pick a plan. Sync usage fees are waived while the subscription is active.'}
                  </p>
                  <div
                    className={`mt-4 grid grid-cols-1 gap-3 ${
                      purchaseStrategy.market_subscription_monthly_only
                        ? ''
                        : 'sm:grid-cols-2'
                    }`}
                  >
                    <button
                      type="button"
                      onClick={() => setPurchasePlan('monthly')}
                      className={`rounded-xl border-2 p-4 text-left transition-all ${
                        purchasePlan === 'monthly'
                          ? 'border-[#d4ff33] bg-primary-container/15 ring-1 ring-[#d4ff33]/40'
                          : 'border-outline-variant/30 bg-surface-container-low hover:border-outline-variant/50'
                      }`}
                    >
                      <div className="text-xs font-bold uppercase tracking-wider text-on-surface-variant">
                        {language === 'zh' ? '月卡 · 30 天' : 'Monthly · 30d'}
                      </div>
                      <div className="mt-1 font-['Space_Grotesk',sans-serif] text-2xl font-bold tabular-nums text-[#d4ff33]">
                        {monthlySubscriptionPrice(purchaseStrategy)} USDT
                      </div>
                      <p className="mt-2 text-[11px] text-on-surface-variant">
                        {language === 'zh'
                          ? '有效期 30 天'
                          : 'Valid for 30 days'}
                      </p>
                    </button>
                    {!purchaseStrategy.market_subscription_monthly_only && (
                      <button
                        type="button"
                        disabled={Boolean(walletData?.market_weekly_trial_used)}
                        onClick={() => {
                          if (!walletData?.market_weekly_trial_used)
                            setPurchasePlan('weekly')
                        }}
                        className={`rounded-xl border-2 p-4 text-left transition-all ${
                          purchasePlan === 'weekly'
                            ? 'border-[#d4ff33] bg-primary-container/15 ring-1 ring-[#d4ff33]/40'
                            : 'border-outline-variant/30 bg-surface-container-low hover:border-outline-variant/50'
                        } ${walletData?.market_weekly_trial_used ? 'cursor-not-allowed opacity-50' : ''}`}
                      >
                        <div className="text-xs font-bold uppercase tracking-wider text-on-surface-variant">
                          {language === 'zh'
                            ? '周卡体验 · 7 天'
                            : 'Weekly trial · 7d'}
                        </div>
                        <div className="mt-1 font-['Space_Grotesk',sans-serif] text-2xl font-bold tabular-nums text-[#d4ff33]">
                          {MARKET_SUB_WEEKLY_TRIAL_USDT} USDT
                        </div>
                        {walletData?.market_weekly_trial_used ? (
                          <p className="mt-2 text-[11px] text-on-surface-variant">
                            {language === 'zh'
                              ? '本账号已使用过唯一一次体验'
                              : 'Trial already used on this account'}
                          </p>
                        ) : (
                          <p className="mt-2 text-[11px] text-on-surface-variant">
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
              <p className="mt-4 text-xs text-on-surface-variant">
                {language === 'zh' ? '当前余额：' : 'Your balance: '}
                <span className="tabular-nums font-semibold text-on-surface">
                  {(walletData?.balance_usdt ?? 0).toFixed(2)} USDT
                </span>
              </p>
              <div className="mt-6 flex justify-end gap-2">
                <button
                  type="button"
                  onClick={() => setPurchaseStrategy(null)}
                  className="rounded-lg px-4 py-2 text-sm text-on-surface-variant hover:bg-surface-container-low"
                >
                  {language === 'zh' ? '取消' : 'Cancel'}
                </button>
                <button
                  type="button"
                  disabled={purchaseBusy}
                  onClick={() => void submitPurchase()}
                  className="market-detail-cta-gold rounded-lg px-4 py-2 text-sm font-bold text-black disabled:opacity-50"
                >
                  {purchaseBusy
                    ? '…'
                    : isFreeSubscriptionStrategy(purchaseStrategy)
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
      </main>

      <div className="fixed bottom-0 left-0 right-0 z-40 flex h-10 items-center justify-between border-t border-outline-variant/10 bg-surface-container-low px-4 text-[10px] font-bold uppercase tracking-widest text-on-surface-variant sm:px-6">
        <div className="flex flex-wrap items-center gap-4">
          <span className="flex items-center gap-2">
            <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-primary-container" />
            {tr('footerStatus')}
          </span>
          <span className="hidden items-center gap-2 text-primary sm:flex">
            <Shield className="h-3 w-3" />
            {tr('footerRisk')}
          </span>
        </div>
        <div className="hidden items-center gap-4 sm:flex">
          <span className="text-primary">{tr('footerBtc')}</span>
        </div>
      </div>
    </div>
  )
}

export default StrategyMarketPage
