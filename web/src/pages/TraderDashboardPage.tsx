import { useEffect, useState, useRef, useMemo } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import useSWR from 'swr'
import { mutate } from 'swr'
import { api } from '../lib/api'
import { ChartTabs } from '../components/charts/ChartTabs'
import { DecisionCard } from '../components/trader/DecisionCard'
import { PositionHistory } from '../components/trader/PositionHistory'
import { PunkAvatar, getTraderAvatar } from '../components/common/PunkAvatar'
import { confirmToast, notify } from '../lib/notify'
import { formatPrice, formatQuantity } from '../utils/format'
import { t, type Language } from '../i18n/translations'
import {
    LogOut,
    Loader2,
    Eye,
    EyeOff,
    Copy,
    Check,
    Pencil,
    Trash2,
    Play,
    Square,
    Target,
    TrendingUp,
    Diamond,
    Wallet,
} from 'lucide-react'
import { DeepVoidBackground } from '../components/common/DeepVoidBackground'
import { NofxSelect } from '../components/ui/select'
import { GridRiskPanel } from '../components/strategy/GridRiskPanel'
import { getTraderDashboardLabels, getWinRateDisplay } from '../lib/traderDashboardData'
import { ROUTES } from '../router/paths'
import { SourceUserAccountsPanel, TraderOpenOrdersPanel } from '../components/trader/TraderOpenOrdersPanel'
import { ComkunMasterFollowersSidebar } from '../components/trader/ComkunMasterFollowersSidebar'
import type {
    SystemStatus,
    AccountInfo,
    Position,
    DecisionRecord,
    TraderInfo,
    Exchange,
    Statistics,
    Strategy,
} from '../types'

// --- Helper Functions ---

// Get friendly AI model display name（后端偶发缺字段时不能抛错，否则整页黑屏）
function getModelDisplayName(rawModelId: string | undefined | null): string {
    if (rawModelId == null || rawModelId === '') return '—'
    const modelId = rawModelId.split('_').pop() || rawModelId
    switch (modelId.toLowerCase()) {
        case 'deepseek':
            return 'DeepSeek'
        case 'qwen':
            return 'Qwen'
        case 'claude':
            return 'Claude'
        case 'comkun_ai':
            return 'COMKUN-AI'
        default:
            return modelId.toUpperCase()
    }
}

// Helper function to get exchange display name from exchange ID (UUID)
function getExchangeDisplayNameFromList(
    exchangeId: string | undefined,
    exchanges: Exchange[] | undefined
): string {
    if (!exchangeId) return 'Unknown'
    const exchange = exchanges?.find((e) => e.id === exchangeId)
    if (!exchange) return exchangeId.substring(0, 8).toUpperCase() + '...'
    const typeName =
        (exchange.exchange_type && String(exchange.exchange_type).toUpperCase()) ||
        exchange.name ||
        '—'
    return exchange.account_name
        ? `${typeName} - ${exchange.account_name}`
        : typeName
}

// Helper function to get exchange type from exchange ID (UUID) - for kline charts
function getExchangeTypeFromList(
    exchangeId: string | undefined,
    exchanges: Exchange[] | undefined
): string {
    if (!exchangeId) return 'binance'
    const exchange = exchanges?.find((e) => e.id === exchangeId)
    if (!exchange) return 'binance' // Default to binance for charts
    return exchange.exchange_type?.toLowerCase() || 'binance'
}

// Helper function to check if exchange is a perp-dex type (wallet-based)
function isPerpDexExchange(exchangeType: string | undefined): boolean {
    if (!exchangeType) return false
    const perpDexTypes = ['hyperliquid', 'lighter', 'aster']
    return perpDexTypes.includes(exchangeType.toLowerCase())
}

// Helper function to get wallet address for perp-dex exchanges
function getWalletAddress(exchange: Exchange | undefined): string | undefined {
    if (!exchange) return undefined
    const type = exchange.exchange_type?.toLowerCase()
    switch (type) {
        case 'hyperliquid':
            return exchange.hyperliquidWalletAddr
        case 'lighter':
            return exchange.lighterWalletAddr
        case 'aster':
            return exchange.asterSigner
        default:
            return undefined
    }
}

// Helper function to truncate wallet address for display
function truncateAddress(address: string, startLen = 6, endLen = 4): string {
    if (address.length <= startLen + endLen + 3) return address
    return `${address.slice(0, startLen)}...${address.slice(-endLen)}`
}

// --- Components ---

interface TraderDashboardPageProps {
    selectedTrader?: TraderInfo
    traders?: TraderInfo[]
    tradersError?: Error
    selectedTraderId?: string
    onTraderSelect: (traderId: string) => void
    onNavigateToTraders: () => void
    status?: SystemStatus
    account?: AccountInfo
    accountFailed?: boolean
    onRetryAccount?: () => void
    positions?: Position[]
    positionsFailed?: boolean
    onRetryPositions?: () => void
    decisions?: DecisionRecord[]
    decisionsFailed?: boolean
    statsFailed?: boolean
    onRetryStats?: () => void
    stats?: Statistics
    language: Language
    exchanges?: Exchange[]
}

export function TraderDashboardPage({
    selectedTrader,
    status,
    account,
    accountFailed,
    onRetryAccount,
    positions,
    positionsFailed,
    onRetryPositions,
    decisions,
    decisionsFailed,
    statsFailed,
    onRetryStats,
    language,
    traders,
    tradersError,
    selectedTraderId,
    onTraderSelect,
    onNavigateToTraders,
    exchanges,
    stats,
}: TraderDashboardPageProps) {
    const navigate = useNavigate()
    const [dataTab, setDataTab] = useState<'positions' | 'history' | 'orders'>('positions')
    const [runBusy, setRunBusy] = useState(false)
    const [deleteBusy, setDeleteBusy] = useState(false)
    const [closingPosition, setClosingPosition] = useState<string | null>(null)
    const [selectedChartSymbol, setSelectedChartSymbol] = useState<string | undefined>(undefined)
    const [chartUpdateKey, setChartUpdateKey] = useState<number>(0)
    const chartSectionRef = useRef<HTMLDivElement>(null)
    const [showWalletAddress, setShowWalletAddress] = useState<boolean>(false)
    const [copiedAddress, setCopiedAddress] = useState<boolean>(false)

    // Current positions pagination
    const [positionsPageSize, setPositionsPageSize] = useState<number>(20)
    const [positionsCurrentPage, setPositionsCurrentPage] = useState<number>(1)

    // Calculate paginated positions
    const totalPositions = positions?.length || 0
    const totalPositionPages = Math.ceil(totalPositions / positionsPageSize)
    const paginatedPositions = positions?.slice(
        (positionsCurrentPage - 1) * positionsPageSize,
        positionsCurrentPage * positionsPageSize
    ) || []

    // Reset page when positions change
    useEffect(() => {
        setPositionsCurrentPage(1)
    }, [selectedTraderId, positionsPageSize])

    // Auto-set chart symbol for grid trading
    useEffect(() => {
        if (status?.strategy_type === 'grid_trading' && status?.grid_symbol) {
            setSelectedChartSymbol(status.grid_symbol)
        }
    }, [status?.strategy_type, status?.grid_symbol])

    // Get current exchange info for perp-dex wallet display
    const currentExchange = exchanges?.find(
        (e) => e.id === selectedTrader?.exchange_id
    )
    const walletAddress = getWalletAddress(currentExchange)
    const isPerpDex = isPerpDexExchange(currentExchange?.exchange_type)
    const isHZTrader = currentExchange?.exchange_type?.toLowerCase() === 'hz'
    const dashboardLabels = getTraderDashboardLabels(isHZTrader)

    const strategyIdForTrader = selectedTrader?.strategy_id?.trim() || ''
    const { data: boundStrategy } = useSWR<Strategy>(
        strategyIdForTrader ? ['dashboard-strategy', strategyIdForTrader] : null,
        () => api.getStrategy(strategyIdForTrader),
        { revalidateOnFocus: false, dedupingInterval: 60_000 }
    )
    const isComkunMasterListing = boundStrategy?.config?.comkun_follow_listing_template === true

    // Copy wallet address to clipboard
    const handleCopyAddress = async () => {
        if (!walletAddress) return
        try {
            await navigator.clipboard.writeText(walletAddress)
            setCopiedAddress(true)
            setTimeout(() => setCopiedAddress(false), 2000)
        } catch (err) {
            console.error('Failed to copy address:', err)
        }
    }

    const winRatePct = useMemo(() => {
        if (!stats) return undefined
        if (stats.total_trades != null && stats.total_trades > 0 && stats.win_rate != null) {
            return stats.win_rate
        }
        return undefined
    }, [stats])
    const winRateEmpty = getWinRateDisplay(stats, language)

    const ordersTabLabel = language === 'zh' ? '订单与挂单' : 'Orders & pending'
    const historyTabLabel = language === 'zh' ? '成交与历史' : 'Trade History'

    // Handle symbol click from Decision Card
    const handleSymbolClick = (symbol: string) => {
        // Set the selected symbol
        setSelectedChartSymbol(symbol)
        // Scroll to chart section
        setTimeout(() => {
            chartSectionRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
        }, 100)
    }

    // Close position handler
    const handleClosePosition = async (symbol: string, side: string) => {
        if (!selectedTraderId) return

        const sideLabel = side === 'LONG' ? 'LONG' : 'SHORT'
        const confirmMsg = t('traderDashboard.confirmClosePosition', language, { symbol, side: sideLabel })

        const confirmed = await confirmToast(confirmMsg, {
            title: t('traderDashboard.confirmClose', language),
            okText: t('traderDashboard.confirm', language),
            cancelText: t('traderDashboard.cancel', language),
        })

        if (!confirmed) return

        setClosingPosition(symbol)
        try {
            await api.closePosition(selectedTraderId, symbol, side)
            notify.success(t('traderDashboard.positionClosed', language))
            // Use SWR mutate to refresh data instead of reloading page
            await Promise.all([
                mutate(`positions-${selectedTraderId}`),
                mutate(`account-${selectedTraderId}`),
            ])
        } catch (err: unknown) {
            const errorMsg =
                err instanceof Error
                    ? err.message
                    : t('traderDashboard.closeFailed', language)
            notify.error(errorMsg)
        } finally {
            setClosingPosition(null)
        }
    }

    const handleToggleRun = async () => {
        if (!selectedTraderId) return
        setRunBusy(true)
        try {
            if (status?.is_running) {
                // 先乐观改 UI：后端 StopAsync 不阻塞长 AI，界面立刻显示已停
                void mutate(
                    `status-${selectedTraderId}`,
                    (prev: SystemStatus | undefined) =>
                        prev ? { ...prev, is_running: false } : prev,
                    { revalidate: false }
                )
                await api.stopTrader(selectedTraderId)
                notify.success(language === 'zh' ? '已停止' : 'Stopped')
            } else {
                await api.startTrader(selectedTraderId)
                notify.success(language === 'zh' ? '已启动' : 'Started')
            }
            await Promise.all([
                mutate(`status-${selectedTraderId}`),
                mutate(`account-${selectedTraderId}`),
                mutate(`positions-${selectedTraderId}`),
                mutate('traders-dashboard'),
            ])
        } catch (err: unknown) {
            notify.error(err instanceof Error ? err.message : '操作失败')
            void mutate(`status-${selectedTraderId}`)
        } finally {
            setRunBusy(false)
        }
    }

    const handleDeleteTrader = async () => {
        if (!selectedTraderId) return
        const ok = await confirmToast(
            language === 'zh'
                ? '确定删除该交易员？此操作不可恢复。'
                : 'Delete this trader? This cannot be undone.',
            {
                title: language === 'zh' ? '删除交易员' : 'Delete trader',
                okText: language === 'zh' ? '删除' : 'Delete',
                cancelText: language === 'zh' ? '取消' : 'Cancel',
            },
        )
        if (!ok) return
        setDeleteBusy(true)
        try {
            await api.deleteTrader(selectedTraderId)
            await mutate('traders-dashboard')
            notify.success(language === 'zh' ? '已删除' : 'Deleted')
            navigate(ROUTES.dashboard, { replace: true })
        } catch (err: unknown) {
            notify.error(err instanceof Error ? err.message : '删除失败')
        } finally {
            setDeleteBusy(false)
        }
    }

    // If API failed with error, show empty state (likely backend not running)
    if (tradersError) {
        return (
            <div className="flex items-center justify-center min-h-[60vh] relative z-10">
                <div className="text-center max-w-md mx-auto px-6">
                    <div
                        className="w-24 h-24 mx-auto mb-6 rounded-full flex items-center justify-center nofx-glass"
                        style={{
                            background: 'rgba(240, 185, 11, 0.1)',
                            borderColor: 'rgba(240, 185, 11, 0.3)',
                        }}
                    >
                        <svg
                            className="w-12 h-12 text-nofx-gold"
                            fill="none"
                            viewBox="0 0 24 24"
                            stroke="currentColor"
                        >
                            <path
                                strokeLinecap="round"
                                strokeLinejoin="round"
                                strokeWidth={2}
                                d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
                            />
                        </svg>
                    </div>
                    <h2 className="text-2xl font-bold mb-3 text-nofx-text-main">
                        {t('traderDashboard.connectionFailed', language)}
                    </h2>
                    <p className="text-base mb-6 text-nofx-text-muted">
                        {t('traderDashboard.connectionFailedDesc', language)}
                    </p>
                    <button
                        onClick={() => window.location.reload()}
                        className="px-6 py-3 rounded-lg font-semibold transition-all hover:scale-105 active:scale-95 nofx-glass border border-nofx-gold/30 text-nofx-gold hover:bg-nofx-gold/10"
                    >
                        {t('traderDashboard.retry', language)}
                    </button>
                </div>
            </div>
        )
    }

    // If traders is loaded and empty, show empty state
    if (traders && traders.length === 0) {
        return (
            <div className="flex items-center justify-center min-h-[60vh] relative z-10">
                <div className="text-center max-w-md mx-auto px-6">
                    <div
                        className="w-24 h-24 mx-auto mb-6 rounded-full flex items-center justify-center nofx-glass"
                        style={{
                            background: 'rgba(240, 185, 11, 0.1)',
                            borderColor: 'rgba(240, 185, 11, 0.3)',
                        }}
                    >
                        <svg
                            className="w-12 h-12 text-nofx-gold"
                            fill="none"
                            viewBox="0 0 24 24"
                            stroke="currentColor"
                        >
                            <path
                                strokeLinecap="round"
                                strokeLinejoin="round"
                                strokeWidth={2}
                                d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"
                            />
                        </svg>
                    </div>
                    <h2 className="text-2xl font-bold mb-3 text-nofx-text-main">
                        {t('dashboardEmptyTitle', language)}
                    </h2>
                    <p className="text-base mb-6 text-nofx-text-muted">
                        {t('dashboardEmptyDescription', language)}
                    </p>
                    <button
                        onClick={onNavigateToTraders}
                        className="px-6 py-3 rounded-lg font-semibold transition-all hover:scale-105 active:scale-95 nofx-glass border border-nofx-gold/30 text-nofx-gold hover:bg-nofx-gold/10"
                    >
                        {t('goToTradersPage', language)}
                    </button>
                </div>
            </div>
        )
    }

    // If traders is still loading or selectedTrader is not ready, show skeleton
    if (!selectedTrader) {
        return (
            <div className="relative z-10 w-full min-w-0 space-y-4 px-3 pt-0 md:px-5 lg:px-6 xl:px-8">
                <div className="grid w-full min-w-0 grid-cols-1 content-start items-start gap-y-4 lg:grid-cols-[minmax(0,6fr)_minmax(0,4fr)] lg:items-start lg:gap-x-4 lg:gap-y-0 xl:gap-x-5">
                    <div className="order-1 flex w-full flex-col gap-5 lg:order-none lg:col-start-1 lg:row-start-1">
                        <div className="nofx-glass animate-pulse rounded-2xl p-3 md:p-4">
                            <div className="mb-3 h-8 w-48 rounded bg-nofx-bg/50"></div>
                            <div className="flex gap-4">
                                <div className="h-4 w-32 rounded bg-nofx-bg/50"></div>
                                <div className="h-4 w-24 rounded bg-nofx-bg/50"></div>
                            </div>
                        </div>
                        <div className="space-y-3">
                            <div className="grid grid-cols-2 gap-2 md:grid-cols-4 md:gap-3">
                                {[1, 2, 3, 4].map((i) => (
                                    <div key={i} className="nofx-glass animate-pulse rounded-xl p-5">
                                        <div className="mb-3 h-4 w-24 rounded bg-nofx-bg/50"></div>
                                        <div className="h-8 w-32 rounded bg-nofx-bg/50"></div>
                                    </div>
                                ))}
                            </div>
                            <div className="nofx-glass animate-pulse rounded-2xl p-6">
                                <div className="mb-4 h-6 w-40 rounded bg-nofx-bg/50"></div>
                                <div className="h-64 w-full rounded bg-nofx-bg/50"></div>
                            </div>
                        </div>
                    </div>
                    <div className="order-2 hidden h-[min(420px,55vh)] animate-pulse self-start rounded-2xl bg-nofx-bg/40 lg:order-none lg:col-start-2 lg:row-start-1 lg:block"></div>
                </div>
            </div>
        )
    }

    return (
        <DeepVoidBackground className="min-h-screen pb-8" disableAnimation>
            <div className="relative z-10 w-full min-w-0 px-3 pt-0 md:px-5 lg:px-6 xl:px-8">
                {/* 主区：左 60% / 右 40；显式 col 防止列错位；aside 内勿用 flex-1 撑高（否则右侧会出现大块空白） */}
                <div className="mb-4 grid w-full min-w-0 grid-cols-1 content-start items-start gap-y-4 lg:grid-cols-[minmax(0,6fr)_minmax(0,4fr)] lg:items-start lg:gap-x-4 lg:gap-y-0 xl:gap-x-5">
                    <div className="order-1 flex min-w-0 w-full flex-col gap-5 lg:order-none lg:col-start-1 lg:row-start-1 lg:min-h-0">
                        {/* 多交易员时：并列看板卡片；当前交易员不显示「看板」按钮，仅高亮 */}
                        {traders && traders.length > 1 && (
                            <div className="w-full shrink-0 rounded-xl border border-[#2b3139]/60 bg-nofx-bg-tertiary/50 p-2.5 shadow-inner md:p-3">
                                <div className="mb-2 text-[10px] font-semibold uppercase tracking-wide text-[#5e6673]">
                                    {language === 'zh' ? '交易员看板' : 'Traders'}
                                </div>
                                <div className="flex flex-wrap gap-2">
                                    {traders.map((tr) => {
                                        const active = tr.trader_id === selectedTraderId
                                        return (
                                            <div
                                                key={tr.trader_id}
                                                className={`flex min-w-0 max-w-full flex-[1_1_10rem] items-center gap-2 rounded-lg border px-2 py-1.5 sm:flex-[1_1_11rem] md:max-w-[14rem] ${
                                                    active
                                                        ? 'border-[#d4ff33]/45 bg-[#d4ff33]/10 shadow-[0_0_12px_rgba(212,255,51,0.12)]'
                                                        : 'border-[#2b3139] bg-nofx-bg-secondary/80'
                                                }`}
                                            >
                                                <PunkAvatar
                                                    seed={getTraderAvatar(tr.trader_id, tr.trader_name)}
                                                    size={28}
                                                    className={`shrink-0 rounded-md border ${
                                                        active ? 'border-[#d4ff33]/40' : 'border-[#2b3139]'
                                                    }`}
                                                />
                                                <span className="min-w-0 flex-1 truncate text-xs font-medium text-[#EAECEF]">
                                                    {tr.trader_name}
                                                </span>
                                                {active ? (
                                                    <span className="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-bold text-[#d4ff33]">
                                                        {language === 'zh' ? '当前' : 'Active'}
                                                    </span>
                                                ) : (
                                                    <button
                                                        type="button"
                                                        onClick={() => onTraderSelect(tr.trader_id)}
                                                        className="shrink-0 rounded-md border border-[#2b3139] bg-nofx-bg-tertiary px-2 py-1 text-[10px] font-bold text-[#d4ff33] transition-colors hover:border-[#d4ff33]/40 hover:bg-[#d4ff33]/10"
                                                    >
                                                        {language === 'zh' ? '看板' : 'View'}
                                                    </button>
                                                )}
                                            </div>
                                        )
                                    })}
                                </div>
                            </div>
                        )}
                        {/* 交易员顶栏 — 独立板块，与下方四宫格用 gap 拉开 */}
                        <div className="w-full shrink-0 self-start rounded-2xl border-0 bg-nofx-bg-secondary/95 p-3 shadow-[0_0_32px_rgba(0,0,0,0.45)] md:p-4">
                    <div className="flex flex-col gap-2.5 lg:flex-row lg:items-center lg:justify-between">
                        <div className="flex min-w-0 flex-1 items-start gap-3">
                            <div className="relative shrink-0">
                                <PunkAvatar
                                    seed={getTraderAvatar(
                                        selectedTrader.trader_id,
                                        selectedTrader.trader_name
                                    )}
                                    size={40}
                                    className="rounded-lg border border-[#d4ff33]/35 shadow-[0_0_12px_rgba(212,255,51,0.15)]"
                                />
                                <div
                                    className={`absolute -bottom-0.5 -right-0.5 h-2.5 w-2.5 rounded-full border border-nofx-bg-tertiary ${status?.is_running ? 'bg-[#0ECB81]' : 'bg-[#5e6673]'}`}
                                    title={status?.is_running ? 'running' : 'stopped'}
                                />
                            </div>
                            <div className="min-w-0 flex-1">
                                <div className="flex flex-wrap items-center gap-1.5">
                                    <h1 className="max-w-[min(100%,14rem)] truncate font-['Space_Grotesk',sans-serif] text-lg font-bold tracking-tight text-[#EAECEF] sm:max-w-[20rem] sm:text-xl">
                                        {selectedTrader.trader_name}
                                    </h1>
                                    <Link
                                        to={ROUTES.traders}
                                        className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-[#2b3139] text-[#848E9C] transition-colors hover:border-[#d4ff33]/40 hover:text-[#d4ff33]"
                                        title={language === 'zh' ? '编辑配置' : 'Edit'}
                                    >
                                        <Pencil className="h-3.5 w-3.5" aria-hidden />
                                    </Link>
                                    <span
                                        className={`rounded px-1.5 py-0.5 text-[10px] font-bold ${status?.is_running ? 'border border-[#d4ff33]/35 bg-[#d4ff33]/10 text-[#d4ff33]' : 'bg-white/10 text-[#848E9C]'}`}
                                    >
                                        {status?.is_running
                                            ? language === 'zh'
                                                ? '运行中'
                                                : 'On'
                                            : language === 'zh'
                                              ? '停止'
                                              : 'Off'}
                                    </span>
                                </div>
                                <p className="mt-0.5 text-[10px] leading-snug text-[#5e6673]">
                                    ID {selectedTrader.trader_id.slice(0, 8)}…
                                    {account && !accountFailed ? (
                                        <>
                                            {' · '}
                                            <span className="text-[#848E9C]">
                                                {language === 'zh' ? '净值' : 'Eq'}
                                            </span>{' '}
                                            <span className="font-mono tabular-nums text-[#b7bdc6]">
                                                {account.total_equity?.toFixed(2)}
                                            </span>
                                            <span className="text-[#5e6673]"> {dashboardLabels.asset}</span>
                                        </>
                                    ) : null}
                                    {status ? (
                                        <>
                                            {' · '}
                                            <span className="text-[#848E9C]">
                                                {language === 'zh' ? '周期' : 'Cy'}
                                            </span>{' '}
                                            <span className="text-[#b7bdc6]">{status.call_count}</span>
                                            {' · '}
                                            <span className="text-[#848E9C]">
                                                {language === 'zh' ? '运行' : 'Run'}
                                            </span>{' '}
                                            <span className="text-[#b7bdc6]">{status.runtime_minutes}m</span>
                                        </>
                                    ) : null}
                                </p>
                                <p className="mt-1 line-clamp-2 text-[10px] leading-relaxed text-[#848E9C]">
                                    <span className="text-[#5e6673]">AI</span>{' '}
                                    {getModelDisplayName(selectedTrader.ai_model)}
                                    <span className="mx-1 text-[#2b3139]">|</span>
                                    <span className="text-[#5e6673]">
                                        {language === 'zh' ? '所' : 'Ex'}
                                    </span>{' '}
                                    <span className="text-[#b7bdc6]">
                                        {getExchangeDisplayNameFromList(
                                            selectedTrader.exchange_id,
                                            exchanges,
                                        )}
                                    </span>
                                    <span className="mx-1 text-[#2b3139]">|</span>
                                    <span className="text-[#5e6673]">
                                        {language === 'zh' ? '策' : 'St'}
                                    </span>{' '}
                                    <span className="text-[#d4ff33]/90">
                                        {selectedTrader.strategy_name ||
                                            (language === 'zh' ? '未绑定' : '—')}
                                    </span>
                                </p>
                            </div>
                        </div>
                        <div className="flex w-full shrink-0 flex-wrap items-center justify-end gap-2 sm:w-auto">
                            {exchanges && isPerpDex && (
                                <div className="flex max-w-full flex-1 items-center gap-1 rounded-lg border border-[#2b3139] bg-nofx-bg-tertiary/60 px-2 py-1 sm:max-w-[min(100%,11rem)] sm:flex-none">
                                    {walletAddress ? (
                                        <>
                                            <span className="truncate font-mono text-[10px] text-[#b7bdc6]">
                                                {showWalletAddress
                                                    ? walletAddress
                                                    : truncateAddress(walletAddress)}
                                            </span>
                                            <button
                                                type="button"
                                                onClick={() => setShowWalletAddress(!showWalletAddress)}
                                                className="shrink-0 rounded p-0.5 hover:bg-white/10"
                                                title={
                                                    showWalletAddress
                                                        ? t('traderDashboard.hideAddress', language)
                                                        : t('traderDashboard.showFullAddress', language)
                                                }
                                            >
                                                {showWalletAddress ? (
                                                    <EyeOff className="h-3 w-3 text-[#848E9C]" />
                                                ) : (
                                                    <Eye className="h-3 w-3 text-[#848E9C]" />
                                                )}
                                            </button>
                                            <button
                                                type="button"
                                                onClick={handleCopyAddress}
                                                className="shrink-0 rounded p-0.5 hover:bg-white/10"
                                                title={t('traderDashboard.copyAddress', language)}
                                            >
                                                {copiedAddress ? (
                                                    <Check className="h-3 w-3 text-[#0ECB81]" />
                                                ) : (
                                                    <Copy className="h-3 w-3 text-[#848E9C]" />
                                                )}
                                            </button>
                                        </>
                                    ) : (
                                        <span className="text-[10px] text-[#848E9C]">
                                            {t('traderDashboard.noAddressConfigured', language)}
                                        </span>
                                    )}
                                </div>
                            )}
                            <button
                                type="button"
                                disabled={runBusy || !selectedTraderId}
                                onClick={() => void handleToggleRun()}
                                className="inline-flex flex-1 items-center justify-center gap-1.5 rounded-lg bg-[#d4ff33] px-3 py-2 text-xs font-bold text-black shadow-[0_0_16px_rgba(212,255,51,0.2)] transition-opacity hover:opacity-90 disabled:opacity-50 sm:flex-none sm:text-sm"
                            >
                                {runBusy ? (
                                    <Loader2 className="h-3.5 w-3.5 animate-spin shrink-0" aria-hidden />
                                ) : status?.is_running ? (
                                    <Square className="h-3.5 w-3.5 shrink-0 fill-current" aria-hidden />
                                ) : (
                                    <Play className="h-3.5 w-3.5 shrink-0" aria-hidden />
                                )}
                                {runBusy
                                    ? language === 'zh'
                                        ? '处理中'
                                        : 'Wait'
                                    : status?.is_running
                                      ? language === 'zh'
                                          ? '停止'
                                          : 'Stop'
                                      : language === 'zh'
                                        ? '启动'
                                        : 'Start'}
                            </button>
                            <button
                                type="button"
                                disabled={deleteBusy || !selectedTraderId}
                                onClick={() => void handleDeleteTrader()}
                                className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-[#F6465D]/35 text-[#F6465D] transition-colors hover:bg-[#F6465D]/10 disabled:opacity-40"
                                title={language === 'zh' ? '删除交易员' : 'Delete trader'}
                            >
                                {deleteBusy ? (
                                    <Loader2 className="h-4 w-4 animate-spin" aria-hidden />
                                ) : (
                                    <Trash2 className="h-4 w-4" aria-hidden />
                                )}
                            </button>
                        </div>
                    </div>
                </div>

                        {/* 数据区：四宫格 + 图表 + 持仓（单独一块，不与交易员卡片合并） */}
                        <div className="min-w-0 space-y-4">
                        {/* 指标四宫格 — 左侧主栏 */}
                        <div className="grid grid-cols-2 gap-2.5 md:grid-cols-4 md:gap-3">
                            <StatCard
                                title={language === 'zh' ? '胜率' : 'Win Rate'}
                                value={
                                    winRatePct === undefined
                                        ? stats ? (winRateEmpty.value ?? '--') : '--'
                                        : `${winRatePct.toFixed(1)}`
                                }
                                unit={winRatePct === undefined ? winRateEmpty.unit : '%'}
                                subtitle={
                                    stats && stats.total_trades != null && stats.total_trades > 0
                                        ? language === 'zh'
                                            ? `盈利 ${stats.win_trades ?? 0} / 共 ${stats.total_trades} 笔（已平仓）`
                                            : `${stats.win_trades ?? 0} wins / ${stats.total_trades} closed`
                                        : language === 'zh'
                                          ? '暂无已平仓成交'
                                          : 'No closed trades yet'
                                }
                                iconKind="cycle"
                                loading={false}
                            />
                            <StatCard
                                title={t('totalPnL', language)}
                                value={
                                    accountFailed && !account
                                        ? '--'
                                        : `${account?.total_pnl !== undefined && account.total_pnl >= 0 ? '+' : ''}${account?.total_pnl?.toFixed(2) ?? '--'}`
                                }
                                unit={dashboardLabels.asset}
                                change={account ? account.total_pnl_pct || 0 : undefined}
                                positive={(account?.total_pnl ?? 0) >= 0}
                                iconKind="pnl"
                                loading={!account && !accountFailed}
                            />
                            <StatCard
                                title={language === 'zh' ? '保证金占用' : 'Margin Used'}
                                value={
                                    accountFailed && !account
                                        ? '--'
                                        : `${account?.margin_used_pct?.toFixed(1) ?? '--'}`
                                }
                                unit="%"
                                subtitle={
                                    language === 'zh'
                                        ? `持仓 ${account?.position_count ?? '--'}`
                                        : `${account?.position_count ?? '--'} open`
                                }
                                iconKind="margin"
                                loading={!account && !accountFailed}
                            />
                            <StatCard
                                title={dashboardLabels.available}
                                value={accountFailed && !account ? '--' : `${account?.available_balance?.toFixed(2) ?? '--'}`}
                                unit={dashboardLabels.asset}
                                subtitle={
                                    accountFailed && !account
                                        ? '--'
                                        : `${account?.available_balance != null && account?.total_equity ? ((account.available_balance / account.total_equity) * 100).toFixed(1) : '--'}% ${t('free', language)}`
                                }
                                iconKind="balance"
                                loading={!account && !accountFailed}
                            />
                        </div>

                        {accountFailed && !account && (
                            <div className="rounded-lg border border-[#F6465D]/30 bg-[#F6465D]/10 px-3 py-2 text-xs text-[#F6465D]">
                                <span>{language === 'zh' ? '账户数据读取失败，请重试' : 'Account data failed to load; retry.'}</span>
                                <button type="button" onClick={() => onRetryAccount?.()} className="ml-2 underline">{language === 'zh' ? '重试' : 'Retry'}</button>
                            </div>
                        )}

                        {statsFailed && (
                            <div className="rounded-lg border border-[#F6465D]/30 bg-[#F6465D]/10 px-3 py-2 text-xs text-[#F6465D]">
                                <span>{stats ? (language === 'zh' ? '统计数据刷新失败，当前显示上次结果' : 'Statistics refresh failed; showing the last result.') : (language === 'zh' ? '统计数据读取失败，请重试' : 'Statistics failed to load; retry.')}</span>
                                <button type="button" onClick={() => onRetryStats?.()} className="ml-2 underline">{language === 'zh' ? '重试' : 'Retry'}</button>
                            </div>
                        )}

                        {status?.strategy_type === 'grid_trading' && selectedTraderId && (
                            <div className="animate-slide-in" style={{ animationDelay: '0.05s' }}>
                                <GridRiskPanel
                                    traderId={selectedTraderId}
                                    language={language}
                                    refreshInterval={5000}
                                />
                            </div>
                        )}

                        <div
                            ref={chartSectionRef}
                            className="chart-container scroll-mt-28 rounded-xl border border-[#2b3139] bg-nofx-bg-secondary/85 p-1 shadow-[0_0_24px_rgba(0,0,0,0.35)] md:p-1.5"
                        >
                            <ChartTabs
                                isHZ={isHZTrader}
                                traderId={selectedTrader.trader_id}
                                selectedSymbol={selectedChartSymbol}
                                updateKey={chartUpdateKey}
                                exchangeId={getExchangeTypeFromList(
                                    selectedTrader.exchange_id,
                                    exchanges
                                )}
                            />
                        </div>

                        <div className="overflow-hidden rounded-xl border border-[#2b3139] bg-nofx-bg-secondary/95 shadow-[0_0_24px_rgba(0,0,0,0.3)]">
                            <div className="flex flex-wrap gap-1 border-b border-[#2b3139] bg-nofx-bg-tertiary/70 p-1.5">
                                {(
                                    [
                                        { id: 'positions' as const, label: t('currentPositions', language) },
                                        { id: 'history' as const, label: historyTabLabel },
                                        { id: 'orders' as const, label: ordersTabLabel },
                                    ] as const
                                ).map(({ id, label }) => (
                                    <button
                                        key={id}
                                        type="button"
                                        onClick={() => setDataTab(id)}
                                        className={`rounded-lg px-3 py-2 text-left text-xs font-bold transition-colors md:text-sm ${
                                            dataTab === id
                                                ? 'bg-[#d4ff33]/14 text-[#d4ff33] ring-1 ring-[#d4ff33]/35'
                                                : 'text-[#848E9C] hover:bg-white/[0.06] hover:text-[#EAECEF]'
                                        }`}
                                    >
                                        {label}
                                    </button>
                                ))}
                            </div>
                            <div className="relative p-4 md:p-5">
                                {dataTab === 'positions' && (
                        <div className="group relative overflow-hidden rounded-xl bg-nofx-bg-tertiary/35 p-2 md:p-4">
                            <div className="pointer-events-none absolute right-0 top-0 p-3 opacity-[0.07]">
                                <div className="h-24 w-24 rounded-full bg-[#d4ff33] blur-3xl" />
                            </div>
                            <div className="relative z-10 mb-3 flex items-center justify-between">
                                <h2 className="flex items-center gap-2 text-base font-bold uppercase tracking-wide text-[#EAECEF] md:text-lg">
                                    <span className="text-[#d4ff33]">◈</span> {t('currentPositions', language)}
                                </h2>
                                {positions && positions.length > 0 && (
                                    <div className="rounded-md border border-[#d4ff33]/30 bg-[#d4ff33]/10 px-2 py-1 font-mono text-xs font-semibold text-[#d4ff33]">
                                        {positions.length} {t('active', language)}
                                    </div>
                                )}
                            </div>
                            {positions && positions.length > 0 ? (
                                <div>
                                    <div className="space-y-2 md:hidden">
                                        {paginatedPositions.map((pos, i) => {
                                            const pnl = pos.unrealized_pnl ?? 0
                                            const side = String(pos.side ?? '').toUpperCase()
                                            return (
                                                <button
                                                    key={`${pos.symbol}-${i}`}
                                                    type="button"
                                                    className="w-full rounded-lg border border-[#2b3139] bg-black/25 p-3 text-left transition-colors hover:border-[#d4ff33]/30"
                                                    onClick={() => {
                                                        setSelectedChartSymbol(pos.symbol)
                                                        setChartUpdateKey(Date.now())
                                                        chartSectionRef.current?.scrollIntoView({
                                                            behavior: 'smooth',
                                                            block: 'start',
                                                        })
                                                    }}
                                                >
                                                    <div className="flex items-start justify-between gap-3">
                                                        <div className="min-w-0">
                                                            <div className="truncate font-mono text-sm font-bold text-[#EAECEF]">
                                                                {pos.symbol}
                                                            </div>
                                                            <div className="mt-1 flex flex-wrap items-center gap-2 text-[11px] text-[#848E9C]">
                                                                <span
                                                                    className={`rounded px-1.5 py-0.5 text-[10px] font-bold uppercase ${
                                                                        pos.side === 'long'
                                                                            ? 'bg-nofx-green/10 text-nofx-green'
                                                                            : 'bg-nofx-red/10 text-nofx-red'
                                                                    }`}
                                                                >
                                                                    {t(pos.side === 'long' ? 'long' : 'short', language)}
                                                                </span>
                                                                <span>{pos.leverage ?? 0}x</span>
                                                                <span>{formatQuantity(pos.quantity)}</span>
                                                            </div>
                                                        </div>
                                                        <div className="shrink-0 text-right">
                                                            <div
                                                                className={`font-mono text-base font-bold ${
                                                                    pnl >= 0 ? 'text-nofx-green' : 'text-nofx-red'
                                                                }`}
                                                            >
                                                                {pnl >= 0 ? '+' : ''}
                                                                {pnl.toFixed(2)}
                                                            </div>
                                                            <button
                                                                type="button"
                                                                onClick={(e) => {
                                                                    e.stopPropagation()
                                                                    if (!side) return
                                                                    handleClosePosition(pos.symbol, side)
                                                                }}
                                                                disabled={closingPosition === pos.symbol || !pos.side}
                                                                className="mt-2 inline-flex h-8 items-center justify-center rounded-md border border-[#F6465D]/35 px-2 text-[11px] text-[#F6465D] disabled:opacity-40"
                                                            >
                                                                {closingPosition === pos.symbol ? (
                                                                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                                                                ) : (
                                                                    t('traderDashboard.closePosition', language)
                                                                )}
                                                            </button>
                                                        </div>
                                                    </div>
                                                    <div className="mt-3 grid grid-cols-2 gap-2 text-[11px]">
                                                        <div>
                                                            <div className="text-[#5e6673]">{t('traderDashboard.entry', language)}</div>
                                                            <div className="font-mono text-[#EAECEF]">{formatPrice(pos.entry_price)}</div>
                                                        </div>
                                                        <div>
                                                            <div className="text-[#5e6673]">{t('traderDashboard.mark', language)}</div>
                                                            <div className="font-mono text-[#EAECEF]">{formatPrice(pos.mark_price)}</div>
                                                        </div>
                                                        <div>
                                                            <div className="text-[#5e6673]">{t('traderDashboard.last', language)}</div>
                                                            <div className="font-mono text-[#EAECEF]">{formatPrice(pos.last_price)}</div>
                                                        </div>
                                                        <div>
                                                            <div className="text-[#5e6673]">{t('traderDashboard.value', language)}</div>
                                                            <div className="font-mono text-[#EAECEF]">
                                                                {((pos.quantity ?? 0) * (pos.mark_price ?? 0)).toFixed(2)}
                                                            </div>
                                                        </div>
                                                        <div>
                                                            <div className="text-[#5e6673]">{t('traderDashboard.liq', language)}</div>
                                                            <div className="font-mono text-[#EAECEF]">{formatPrice(pos.liquidation_price)}</div>
                                                        </div>
                                                    </div>
                                                </button>
                                            )
                                        })}
                                    </div>
                                    <div className="hidden overflow-x-auto md:block">
                                        <table className="w-full text-xs">
                                            <thead className="text-left border-b border-white/5">
                                                <tr>
                                                    <th className="px-1 pb-3 font-semibold text-nofx-text-muted whitespace-nowrap text-left">{t('symbol', language)}</th>
                                                    <th className="px-1 pb-3 font-semibold text-nofx-text-muted whitespace-nowrap text-center">{t('side', language)}</th>
                                                    <th className="px-1 pb-3 font-semibold text-nofx-text-muted whitespace-nowrap text-center">{t('traderDashboard.action', language)}</th>
                                                    <th className="px-1 pb-3 font-semibold text-nofx-text-muted whitespace-nowrap text-right hidden md:table-cell" title={t('entryPrice', language)}>{t('traderDashboard.entry', language)}</th>
                                                    <th className="px-1 pb-3 font-semibold text-nofx-text-muted whitespace-nowrap text-right hidden md:table-cell" title={t('markPrice', language)}>{t('traderDashboard.mark', language)}</th>
                                                    <th className="px-1 pb-3 font-semibold text-nofx-text-muted whitespace-nowrap text-right hidden md:table-cell">{t('traderDashboard.last', language)}</th>
                                                    <th className="px-1 pb-3 font-semibold text-nofx-text-muted whitespace-nowrap text-right" title={t('quantity', language)}>{t('traderDashboard.qty', language)}</th>
                                                    <th className="px-1 pb-3 font-semibold text-nofx-text-muted whitespace-nowrap text-right hidden md:table-cell" title={t('positionValue', language)}>{t('traderDashboard.value', language)}</th>
                                                    <th className="px-1 pb-3 font-semibold text-nofx-text-muted whitespace-nowrap text-center hidden md:table-cell" title={t('leverage', language)}>{t('traderDashboard.lev', language)}</th>
                                                    <th className="px-1 pb-3 font-semibold text-nofx-text-muted whitespace-nowrap text-right" title={t('unrealizedPnL', language)}>{t('traderDashboard.uPnL', language)}</th>
                                                    <th className="px-1 pb-3 font-semibold text-nofx-text-muted whitespace-nowrap text-right hidden md:table-cell" title={t('liqPrice', language)}>{t('traderDashboard.liq', language)}</th>
                                                </tr>
                                            </thead>
                                            <tbody>
                                                {paginatedPositions.map((pos, i) => (
                                                    <tr
                                                        key={i}
                                                        className="border-b border-white/5 last:border-0 transition-all hover:bg-white/5 cursor-pointer group/row"
                                                        onClick={() => {
                                                            setSelectedChartSymbol(pos.symbol)
                                                            setChartUpdateKey(Date.now())
                                                            if (chartSectionRef.current) {
                                                                chartSectionRef.current.scrollIntoView({
                                                                    behavior: 'smooth',
                                                                    block: 'start',
                                                                })
                                                            }
                                                        }}
                                                    >
                                                        <td className="px-1 py-3 font-mono font-semibold whitespace-nowrap text-left text-nofx-text-main group-hover/row:text-white transition-colors">
                                                            {pos.symbol}
                                                        </td>
                                                        <td className="px-1 py-3 whitespace-nowrap text-center">
                                                            <span
                                                                className={`px-1.5 py-0.5 rounded text-[10px] font-bold uppercase tracking-wider ${pos.side === 'long' ? 'bg-nofx-green/10 text-nofx-green shadow-[0_0_8px_rgba(14,203,129,0.2)]' : 'bg-nofx-red/10 text-nofx-red shadow-[0_0_8px_rgba(246,70,93,0.2)]'}`}
                                                            >
                                                                {t(pos.side === 'long' ? 'long' : 'short', language)}
                                                            </span>
                                                        </td>
                                                        <td className="px-1 py-3 whitespace-nowrap text-center">
                                                            <button
                                                                type="button"
                                                                onClick={(e) => {
                                                                    e.stopPropagation()
                                                                    const s = String(pos.side ?? '').toUpperCase()
                                                                    if (!s) return
                                                                    handleClosePosition(pos.symbol, s)
                                                                }}
                                                                disabled={closingPosition === pos.symbol || !pos.side}
                                                                className="mx-auto inline-flex h-7 w-7 items-center justify-center rounded-md border border-[#2b3139] text-[#848E9C] transition-colors hover:border-[#F6465D]/40 hover:text-[#F6465D] disabled:cursor-not-allowed disabled:opacity-40"
                                                                title={t('traderDashboard.closePosition', language)}
                                                            >
                                                                {closingPosition === pos.symbol ? (
                                                                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                                                                ) : (
                                                                    <LogOut className="h-3.5 w-3.5" />
                                                                )}
                                                            </button>
                                                        </td>
                                                        <td className="px-1 py-3 font-mono whitespace-nowrap text-right text-nofx-text-main hidden md:table-cell">{formatPrice(pos.entry_price)}</td>
                                                        <td className="px-1 py-3 font-mono whitespace-nowrap text-right text-nofx-text-main hidden md:table-cell">{formatPrice(pos.mark_price)}</td>
                                                        <td className="px-1 py-3 font-mono whitespace-nowrap text-right text-nofx-text-main hidden md:table-cell" title={pos.last_price_time ? new Date(pos.last_price_time).toLocaleString() : undefined}>{formatPrice(pos.last_price)}</td>
                                                        <td className="px-1 py-3 font-mono whitespace-nowrap text-right text-nofx-text-main">{formatQuantity(pos.quantity)}</td>
                                                        <td className="px-1 py-3 font-mono font-bold whitespace-nowrap text-right text-nofx-text-main hidden md:table-cell">
                                                            {((pos.quantity ?? 0) * (pos.mark_price ?? 0)).toFixed(2)}
                                                        </td>
                                                        <td className="hidden px-1 py-3 text-center font-mono whitespace-nowrap text-[#d4ff33] md:table-cell">
                                                            {pos.leverage ?? 0}x
                                                        </td>
                                                        <td className="px-1 py-3 font-mono whitespace-nowrap text-right">
                                                            <span
                                                                className={`font-bold ${(pos.unrealized_pnl ?? 0) >= 0 ? 'text-nofx-green shadow-nofx-green' : 'text-nofx-red shadow-nofx-red'}`}
                                                                style={{
                                                                    textShadow:
                                                                        (pos.unrealized_pnl ?? 0) >= 0
                                                                            ? '0 0 10px rgba(14,203,129,0.3)'
                                                                            : '0 0 10px rgba(246,70,93,0.3)',
                                                                }}
                                                            >
                                                                {(pos.unrealized_pnl ?? 0) >= 0 ? '+' : ''}
                                                                {(pos.unrealized_pnl ?? 0).toFixed(2)}
                                                            </span>
                                                        </td>
                                                        <td className="px-1 py-3 font-mono whitespace-nowrap text-right text-nofx-text-muted hidden md:table-cell">{formatPrice(pos.liquidation_price)}</td>
                                                    </tr>
                                                ))}
                                            </tbody>
                                        </table>
                                    </div>
                                    {/* Pagination footer */}
                                    {totalPositions > 10 && (
                                        <div className="flex flex-wrap items-center justify-between gap-3 pt-4 mt-4 text-xs border-t border-white/5 text-nofx-text-muted">
                                            <span>
                                                {t('traderDashboard.showingPositions', language, { shown: paginatedPositions.length, total: totalPositions })}
                                            </span>
                                            <div className="flex items-center gap-3">
                                                <div className="flex items-center gap-2">
                                                    <span>{t('traderDashboard.perPage', language)}:</span>
                                                    <NofxSelect
                                                        value={positionsPageSize}
                                                        onChange={(val) => setPositionsPageSize(Number(val))}
                                                        options={[{ value: 20, label: '20' }, { value: 50, label: '50' }, { value: 100, label: '100' }]}
                                                        className="bg-black/40 border border-white/10 rounded px-2 py-1 text-xs text-nofx-text-main transition-colors"
                                                    />
                                                </div>
                                                {totalPositionPages > 1 && (
                                                    <div className="flex items-center gap-1">
                                                        {['«', '‹', `${positionsCurrentPage} / ${totalPositionPages}`, '›', '»'].map((label, idx) => {
                                                            const isText = idx === 2;
                                                            const isFirst = idx === 0;
                                                            const isPrev = idx === 1;
                                                            const isNext = idx === 3;
                                                            const isLast = idx === 4;
                                                            if (isText) return <span key={idx} className="px-3 text-nofx-text-main">{label}</span>;

                                                            let onClick = () => { };
                                                            let disabled = false;

                                                            if (isFirst) { onClick = () => setPositionsCurrentPage(1); disabled = positionsCurrentPage === 1; }
                                                            if (isPrev) { onClick = () => setPositionsCurrentPage(p => Math.max(1, p - 1)); disabled = positionsCurrentPage === 1; }
                                                            if (isNext) { onClick = () => setPositionsCurrentPage(p => Math.min(totalPositionPages, p + 1)); disabled = positionsCurrentPage === totalPositionPages; }
                                                            if (isLast) { onClick = () => setPositionsCurrentPage(totalPositionPages); disabled = positionsCurrentPage === totalPositionPages; }

                                                            return (
                                                                <button
                                                                    key={idx}
                                                                    onClick={onClick}
                                                                    disabled={disabled}
                                                                    className={`px-2 py-1 rounded transition-colors ${disabled ? 'opacity-30 cursor-not-allowed' : 'hover:bg-white/10 text-nofx-text-main bg-white/5'}`}
                                                                >
                                                                    {label}
                                                                </button>
                                                            )
                                                        })}
                                                    </div>
                                                )}
                                            </div>
                                        </div>
                                    )}
                                </div>
                            ) : positionsFailed ? (
                                <div className="text-center py-16 text-nofx-text-muted opacity-60">
                                    <div className="text-4xl mb-4">⚠️</div>
                                    <div className="text-lg font-semibold mb-2">{t('traderDashboard.positionsFetchFailed', language)}</div>
                                    <button type="button" onClick={() => onRetryPositions?.()} className="mt-2 underline">{language === 'zh' ? '重试' : 'Retry'}</button>
                                </div>
                            ) : (
                                <div className="text-center py-16 text-nofx-text-muted opacity-60">
                                    <div className="text-6xl mb-4 opacity-50 grayscale">📊</div>
                                    <div className="text-lg font-semibold mb-2">{t('noPositions', language)}</div>
                                    <div className="text-sm">{t('noActivePositions', language)}</div>
                                </div>
                            )}
                        </div>
                                )}
                                {dataTab === 'history' && selectedTraderId && (
                                    <div className="min-h-[240px] rounded-xl border border-[#2b3139]/70 bg-nofx-bg-tertiary/40 p-2">
                                        <PositionHistory traderId={selectedTraderId} />
                                    </div>
                                )}
                                {dataTab === 'orders' && selectedTraderId && (
                                    <div className="min-h-[240px] rounded-xl border border-[#2b3139]/70 bg-nofx-bg-tertiary/40 p-3 md:p-4">
                                        <TraderOpenOrdersPanel
                                            traderId={selectedTraderId}
                                            language={language}
                                            refreshMs={15000}
                                        />
                                        <SourceUserAccountsPanel
                                            strategyId={selectedTrader?.strategy_id}
                                            language={language}
                                        />
                                    </div>
                                )}
                            </div>
                        </div>
                        </div>
                    </div>

                    {/* 主控上架模板：右侧同时展示 AI 决策 + 跟单客户看板 */}
                    <div className="order-2 flex min-h-0 min-w-0 w-full flex-col gap-3 self-start lg:order-none lg:col-start-2 lg:row-start-1 lg:w-auto lg:self-start">
                    <aside className="flex min-h-[min(34vh,20rem)] min-w-0 w-full max-h-[min(62vh,38rem)] flex-none flex-col overflow-hidden rounded-xl border-0 bg-nofx-bg-secondary/95 p-3 shadow-[0_0_24px_rgba(0,0,0,0.35)] md:p-4 lg:min-h-[min(38vh,24rem)]">
                        <div className="mb-2 flex shrink-0 items-center gap-2 border-b border-[#2b3139]/80 pb-2">
                            <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-[#c4cf45]/40 bg-[#c4cf45]/12 text-base shadow-[0_0_14px_rgba(212,255,51,0.22)]">
                                🧠
                            </div>
                            <div className="min-w-0 flex-1">
                                <h2 className="font-['Space_Grotesk',sans-serif] text-lg font-bold text-[#EAECEF]">
                                    {language === 'zh' ? 'AI 分析' : 'AI Analysis'}
                                </h2>
                                <p className="text-[11px] text-[#848E9C]">
                                    {language === 'zh' ? '最近决策' : 'Recent decisions'}
                                </p>
                                {decisions && decisions.length > 0 && (
                                    <div className="mt-0.5 text-[10px] text-[#5e6673]">
                                        {t('lastCycles', language, { count: decisions.length })}
                                    </div>
                                )}
                            </div>
                        </div>

                        <div className="custom-scrollbar mt-0 min-h-0 flex-1 space-y-3 overflow-y-auto overscroll-contain pr-1 pt-0">
                            {decisions && decisions.length > 0 ? (
                                decisions.map((decision, i) => (
                                    <DecisionCard key={i} decision={decision} language={language} onSymbolClick={handleSymbolClick} />
                                ))
                            ) : decisionsFailed ? (
                                <div className="py-16 text-center text-nofx-text-muted opacity-60">
                                    <div className="text-4xl mb-4">⚠️</div>
                                    <div className="text-lg font-semibold mb-2">{t('traderDashboard.decisionsFetchFailed', language)}</div>
                                </div>
                            ) : (
                                <div className="py-16 text-center text-nofx-text-muted opacity-60">
                                    <div className="text-6xl mb-4 opacity-30 grayscale">🧠</div>
                                    <div className="text-lg font-semibold mb-2 text-nofx-text-main">
                                        {t('noDecisionsYet', language)}
                                    </div>
                                    <div className="text-sm">
                                        {t('aiDecisionsWillAppear', language)}
                                    </div>
                                </div>
                            )}
                        </div>
                    </aside>
                    {isComkunMasterListing && strategyIdForTrader ? (
                        <ComkunMasterFollowersSidebar
                            strategyId={strategyIdForTrader}
                            language={language}
                            embedded
                        />
                    ) : null}
                    </div>
                </div>
            </div>
        </DeepVoidBackground>
    )
}

type DashboardStatIconKind = 'cycle' | 'pnl' | 'margin' | 'balance'

function DashboardStatIcon({
    kind,
    className,
    strokeWidth,
}: {
    kind: DashboardStatIconKind
    className?: string
    strokeWidth?: number
}) {
    const props = { className, strokeWidth, 'aria-hidden': true as const }
    switch (kind) {
        case 'cycle':
            return <Target {...props} />
        case 'pnl':
            return <TrendingUp {...props} />
        case 'margin':
            return <Diamond {...props} />
        case 'balance':
            return <Wallet {...props} />
        default:
            return null
    }
}

/** 仪表盘四宫格：对齐参考图（左上标题、右上淡图标、大号主值+灰色单位、底部辅文） */
function StatCard({
    title,
    value,
    unit,
    change,
    positive,
    subtitle,
    iconKind,
    loading,
}: {
    title: string
    value: string
    unit?: string
    change?: number
    positive?: boolean
    subtitle?: string
    iconKind: DashboardStatIconKind
    loading?: boolean
}) {
    return (
        <div
            className="relative flex min-h-[118px] flex-col rounded-[10px] border border-[#2B3139] bg-[#1c1c1c] p-3.5 md:min-h-[124px] md:p-4"
            style={{ boxShadow: '0 1px 0 rgba(255,255,255,0.03) inset' }}
        >
            <div className="flex items-start justify-between gap-2">
                <span className="text-[11px] font-medium leading-tight text-[#848E9C] md:text-xs">
                    {title}
                </span>
                <DashboardStatIcon
                    kind={iconKind}
                    className="pointer-events-none h-[18px] w-[18px] shrink-0 text-[#EAECEF]/[0.08] md:h-5 md:w-5"
                    strokeWidth={1.15}
                />
            </div>

            {loading ? (
                <div className="mt-4 flex flex-1 flex-col justify-end gap-2">
                    <div className="h-8 w-28 animate-pulse rounded bg-white/[0.06]" />
                    <div className="h-3 w-20 animate-pulse rounded bg-white/[0.06]" />
                </div>
            ) : (
                <div className="mt-3 flex min-h-0 flex-1 flex-col justify-end">
                    <div className="flex flex-wrap items-baseline gap-x-1.5 gap-y-0.5">
                        <span className="text-[21px] font-bold leading-none tracking-tight text-[#EAECEF] tabular-nums md:text-[22px]">
                            {value}
                        </span>
                        {unit ? (
                            <span className="text-[11px] font-medium uppercase tracking-wide text-[#5e6673] md:text-xs">
                                {unit}
                            </span>
                        ) : null}
                    </div>

                    {change !== undefined ? (
                        <div
                            className={`mt-2 flex items-center gap-1 text-xs font-semibold tabular-nums md:text-[13px] ${
                                positive ? 'text-[#0ECB81]' : 'text-[#F6465D]'
                            }`}
                        >
                            <span aria-hidden>{positive ? '▲' : '▼'}</span>
                            <span>
                                {positive ? '+' : ''}
                                {change.toFixed(2)}%
                            </span>
                        </div>
                    ) : null}

                    {subtitle ? (
                        <div
                            className={`font-mono text-[11px] tabular-nums text-[#848E9C] md:text-xs ${
                                change !== undefined ? 'mt-1' : 'mt-2'
                            }`}
                        >
                            {subtitle}
                        </div>
                    ) : null}
                </div>
            )}
        </div>
    )
}
