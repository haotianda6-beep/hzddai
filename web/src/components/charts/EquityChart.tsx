import { useState } from 'react'
import {
  ComposedChart,
  Area,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  ReferenceLine,
} from 'recharts'
import useSWR from 'swr'
import { api } from '../../lib/api'
import { useLanguage } from '../../contexts/LanguageContext'
import { useAuth } from '../../contexts/AuthContext'
import { t } from '../../i18n/translations'
import {
  AlertTriangle,
  BarChart3,
  DollarSign,
  Percent,
  TrendingUp as ArrowUp,
  TrendingDown as ArrowDown,
} from 'lucide-react'

interface EquityPoint {
  timestamp: string
  total_equity: number
  pnl: number
  pnl_pct: number
  cycle_number: number
}

/** 同一分钟内只保留最后一个点，作为 1 分钟序列的锚点 */
function bucketLastPerMinute(history: EquityPoint[]): EquityPoint[] {
  if (history.length === 0) return []
  const map = new Map<string, EquityPoint>()
  for (const p of history) {
    const d = new Date(p.timestamp)
    const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
    const prev = map.get(key)
    if (!prev || new Date(p.timestamp).getTime() >= new Date(prev.timestamp).getTime()) {
      map.set(key, p)
    }
  }
  return Array.from(map.values()).sort(
    (a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime(),
  )
}

/** 在相邻分钟锚点之间按分钟线性插值，避免折线「折成直角」；跨度过大时不插值以控点数 */
function interpolateEquityPerMinute(points: EquityPoint[]): EquityPoint[] {
  if (points.length <= 1) return points
  const MAX_GAP_MS = 8 * 60 * 60 * 1000 // 超过 8 小时的大空档不逐分钟填
  const out: EquityPoint[] = [points[0]]
  for (let i = 0; i < points.length - 1; i++) {
    const a = out[out.length - 1]
    const b = points[i + 1]
    const ta = new Date(a.timestamp).getTime()
    const tb = new Date(b.timestamp).getTime()
    if (tb <= ta) {
      out.push(b)
      continue
    }
    const gap = tb - ta
    if (gap <= MAX_GAP_MS && gap > 60 * 1000) {
      let t = ta + 60 * 1000
      while (t < tb) {
        const r = (t - ta) / gap
        out.push({
          timestamp: new Date(t).toISOString(),
          total_equity: a.total_equity + (b.total_equity - a.total_equity) * r,
          pnl: a.pnl + (b.pnl - a.pnl) * r,
          pnl_pct: a.pnl_pct + (b.pnl_pct - a.pnl_pct) * r,
          cycle_number: b.cycle_number ?? a.cycle_number,
        })
        t += 60 * 1000
      }
    }
    out.push(b)
  }
  return out
}

interface EquityChartProps {
  traderId?: string
  embedded?: boolean // 嵌入模式（不显示外层卡片）
}

export function EquityChart({ traderId, embedded = false }: EquityChartProps) {
  const { language } = useLanguage()
  const { user, token } = useAuth()
  const [displayMode, setDisplayMode] = useState<'dollar' | 'percent'>('dollar')

  const { data: history, error, isLoading } = useSWR<EquityPoint[]>(
    user && token && traderId ? `equity-history-${traderId}` : null,
    () => api.getEquityHistory(traderId, true),
    {
      refreshInterval: 60000,
      revalidateOnFocus: false,
      dedupingInterval: 45000,
      errorRetryCount: 8,
      errorRetryInterval: 3000,
    }
  )

  const { data: account } = useSWR(
    user && token && traderId ? `account-${traderId}` : null,
    () => api.getAccount(traderId, true),
    {
      refreshInterval: 15000, // 15秒刷新（配合后端缓存）
      revalidateOnFocus: false,
      dedupingInterval: 10000,
    }
  )

  // Loading state - show skeleton
  if (isLoading) {
    return (
      <div className={embedded ? 'p-6' : 'binance-card p-6'}>
        {!embedded && (
          <h3 className="text-lg font-semibold mb-6" style={{ color: '#EAECEF' }}>
            {t('accountEquityCurve', language)}
          </h3>
        )}
        <div className="animate-pulse">
          <div className="skeleton h-64 w-full rounded"></div>
        </div>
      </div>
    )
  }

  if (error && !history) {
    return (
      <div className={embedded ? 'p-6' : 'binance-card p-6'}>
        <div
          className="flex items-center gap-3 p-4 rounded"
          style={{
            background: 'rgba(246, 70, 93, 0.1)',
            border: '1px solid rgba(246, 70, 93, 0.2)',
          }}
        >
          <AlertTriangle className="w-6 h-6" style={{ color: '#F6465D' }} />
          <div>
            <div className="font-semibold" style={{ color: '#F6465D' }}>
              {t('loadingError', language)}
            </div>
            <div className="text-sm" style={{ color: '#848E9C' }}>
              {language === 'zh' ? '净值数据正在重试，请稍后刷新。' : 'Retrying equity data, please refresh later.'}
            </div>
          </div>
        </div>
      </div>
    )
  }

  // 过滤掉无效数据：total_equity为0或小于1的数据点（API失败导致）
  const validHistory = history?.filter((point) => point.total_equity > 1) || []

  if (!validHistory || validHistory.length === 0) {
    return (
      <div className={embedded ? 'p-6' : 'binance-card p-6'}>
        {!embedded && (
          <h3 className="text-lg font-semibold mb-6" style={{ color: '#EAECEF' }}>
            {t('accountEquityCurve', language)}
          </h3>
        )}
        <div className="text-center py-16" style={{ color: '#848E9C' }}>
          <div className="mb-4 flex justify-center opacity-50">
            <BarChart3 className="w-16 h-16" />
          </div>
          <div className="text-lg font-semibold mb-2">
            {t('noHistoricalData', language)}
          </div>
          <div className="text-sm">{t('dataWillAppear', language)}</div>
        </div>
      </div>
    )
  }

  const perMinute = bucketLastPerMinute(validHistory)
  const smoothSeries = interpolateEquityPerMinute(perMinute)

  // 限制显示最近的数据点（性能优化）
  const MAX_DISPLAY_POINTS = 4000
  const displayHistory =
    smoothSeries.length > MAX_DISPLAY_POINTS ? smoothSeries.slice(-MAX_DISPLAY_POINTS) : smoothSeries

  // 计算初始余额（优先从 account 获取配置的初始余额，备选从历史数据反推）
  const initialBalance =
    account?.initial_balance || // 从交易员配置读取真实初始余额
    (validHistory[0]
      ? validHistory[0].total_equity - validHistory[0].pnl
      : undefined) || // 备选：淨值 - 盈亏
    1000 // 默认值（与创建交易员时的默认配置一致）

  // 转换数据格式
  const chartData = displayHistory.map((point, index) => {
    const pnl = point.total_equity - initialBalance
    const pnlPct = ((pnl / initialBalance) * 100).toFixed(2)
    return {
      time: new Date(point.timestamp).toLocaleTimeString('zh-CN', {
        hour: '2-digit',
        minute: '2-digit',
      }),
      value: displayMode === 'dollar' ? point.total_equity : parseFloat(pnlPct),
      cycle: point.cycle_number ?? index + 1,
      raw_equity: point.total_equity,
      raw_pnl: pnl,
      raw_pnl_pct: parseFloat(pnlPct),
    }
  })

  const currentValue = chartData[chartData.length - 1]
  const isProfit = currentValue.raw_pnl >= 0

  // 计算Y轴范围
  const calculateYDomain = () => {
    if (displayMode === 'percent') {
      // 百分比模式：找到最大最小值，留20%余量
      const values = chartData.map((d) => d.value)
      const minVal = Math.min(...values)
      const maxVal = Math.max(...values)
      const range = Math.max(Math.abs(maxVal), Math.abs(minVal))
      const padding = Math.max(range * 0.2, 1) // 至少留1%余量
      return [Math.floor(minVal - padding), Math.ceil(maxVal + padding)]
    } else {
      // 美元模式：以初始余额为基准，上下留10%余量
      const values = chartData.map((d) => d.value)
      const minVal = Math.min(...values, initialBalance)
      const maxVal = Math.max(...values, initialBalance)
      const range = maxVal - minVal
      const padding = Math.max(range * 0.15, initialBalance * 0.01) // 至少留1%余量
      return [Math.floor(minVal - padding), Math.ceil(maxVal + padding)]
    }
  }

  // 自定义Tooltip - Binance Style
  const CustomTooltip = ({ active, payload }: any) => {
    if (active && payload && payload.length) {
      const data = payload[0].payload
      const cycleLine =
        language === 'zh'
          ? `周期 ${data.cycle != null ? data.cycle : '—'}`
          : `Cycle #${data.cycle != null ? data.cycle : '—'}`
      return (
        <div
          className="rounded p-3 shadow-xl"
          style={{ background: '#1c1c1c', border: '1px solid #2B3139' }}
        >
          <div className="text-xs mb-1" style={{ color: '#848E9C' }}>
            {cycleLine}
          </div>
          <div className="font-bold mono" style={{ color: '#EAECEF' }}>
            {data.raw_equity.toFixed(2)} USDT
          </div>
          <div
            className="text-sm mono font-bold"
            style={{ color: data.raw_pnl >= 0 ? '#0ECB81' : '#F6465D' }}
          >
            {data.raw_pnl >= 0 ? '+' : ''}
            {data.raw_pnl.toFixed(2)} USDT ({data.raw_pnl_pct >= 0 ? '+' : ''}
            {data.raw_pnl_pct}%)
          </div>
        </div>
      )
    }
    return null
  }

  return (
    <div className={embedded ? 'p-3 sm:p-5' : 'binance-card p-3 sm:p-5 animate-fade-in'}>
      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between mb-4">
        <div className="flex-1">
          {!embedded && (
            <h3
              className="text-base sm:text-lg font-bold mb-2"
              style={{ color: '#EAECEF' }}
            >
              {t('accountEquityCurve', language)}
            </h3>
          )}
          <div className="flex flex-col sm:flex-row sm:items-baseline gap-2 sm:gap-4">
            <span
              className="text-2xl sm:text-3xl font-bold mono"
              style={{ color: '#EAECEF' }}
            >
              {account?.total_equity.toFixed(2) || '0.00'}
              <span
                className="text-base sm:text-lg ml-1"
                style={{ color: '#848E9C' }}
              >
                USDT
              </span>
            </span>
            <div className="flex items-center gap-2 flex-wrap">
              <span
                className="text-sm sm:text-lg font-bold mono px-2 sm:px-3 py-1 rounded flex items-center gap-1"
                style={{
                  color: isProfit ? '#0ECB81' : '#F6465D',
                  background: isProfit
                    ? 'rgba(14, 203, 129, 0.1)'
                    : 'rgba(246, 70, 93, 0.1)',
                  border: `1px solid ${
                    isProfit
                      ? 'rgba(14, 203, 129, 0.2)'
                      : 'rgba(246, 70, 93, 0.2)'
                  }`,
                }}
              >
                {isProfit ? (
                  <ArrowUp className="w-4 h-4" />
                ) : (
                  <ArrowDown className="w-4 h-4" />
                )}
                {isProfit ? '+' : ''}
                {currentValue.raw_pnl_pct}%
              </span>
              <span
                className="text-xs sm:text-sm mono"
                style={{ color: '#848E9C' }}
              >
                ({isProfit ? '+' : ''}
                {currentValue.raw_pnl.toFixed(2)} USDT)
              </span>
            </div>
          </div>
        </div>

        {/* Display Mode Toggle */}
        <div
          className="flex gap-0.5 sm:gap-1 rounded p-0.5 sm:p-1 self-start sm:self-auto"
          style={{ background: '#0b0b0b', border: '1px solid #2B3139' }}
        >
          <button
            onClick={() => setDisplayMode('dollar')}
            className="px-3 sm:px-4 py-1.5 sm:py-2 rounded text-xs sm:text-sm font-bold transition-all flex items-center gap-1"
            style={
              displayMode === 'dollar'
                ? {
                    background: '#d4ff33',
                    color: '#000',
                    boxShadow: '0 2px 8px rgba(240, 185, 11, 0.4)',
                  }
                : { background: 'transparent', color: '#848E9C' }
            }
          >
            <DollarSign className="w-4 h-4" /> USDT
          </button>
          <button
            onClick={() => setDisplayMode('percent')}
            className="px-3 sm:px-4 py-1.5 sm:py-2 rounded text-xs sm:text-sm font-bold transition-all flex items-center gap-1"
            style={
              displayMode === 'percent'
                ? {
                    background: '#d4ff33',
                    color: '#000',
                    boxShadow: '0 2px 8px rgba(240, 185, 11, 0.4)',
                  }
                : { background: 'transparent', color: '#848E9C' }
            }
          >
            <Percent className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* Chart */}
      <div
        className="my-2"
        style={{
          borderRadius: '8px',
          overflow: 'hidden',
          position: 'relative',
        }}
      >
        {/* COMKUN watermark */}
        <div
          style={{
            position: 'absolute',
            top: '15px',
            right: '15px',
            fontSize: '20px',
            fontWeight: 'bold',
            color: 'rgba(240, 185, 11, 0.15)',
            zIndex: 10,
            pointerEvents: 'none',
            fontFamily: 'monospace',
          }}
        >
          COMKUN
        </div>
        <ResponsiveContainer width="100%" height={280}>
          <ComposedChart data={chartData} margin={{ top: 10, right: 20, left: 5, bottom: 30 }}>
            <defs>
              <linearGradient id="colorGradient" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="#d4ff33" stopOpacity={0.85} />
                <stop offset="95%" stopColor="#c4cf45" stopOpacity={0.35} />
              </linearGradient>
              <linearGradient id="equityAreaFill" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="#d4ff33" stopOpacity={0.22} />
                <stop offset="100%" stopColor="#d4ff33" stopOpacity={0} />
              </linearGradient>
            </defs>
            <CartesianGrid strokeDasharray="4 6" stroke="#2B3139" strokeOpacity={0.45} vertical={false} />
            <XAxis
              dataKey="time"
              stroke="#5E6673"
              tick={{ fill: '#848E9C', fontSize: 11 }}
              tickLine={{ stroke: '#2B3139' }}
              interval={Math.floor(chartData.length / 10)}
              angle={-15}
              textAnchor="end"
              height={60}
            />
            <YAxis
              stroke="#5E6673"
              tick={{ fill: '#848E9C', fontSize: 12 }}
              tickLine={{ stroke: '#2B3139' }}
              domain={calculateYDomain()}
              tickFormatter={(value) =>
                displayMode === 'dollar' ? `$${value.toFixed(0)}` : `${value}%`
              }
            />
            <Tooltip content={<CustomTooltip />} />
            <ReferenceLine
              y={displayMode === 'dollar' ? initialBalance : 0}
              stroke="#474D57"
              strokeDasharray="3 3"
              label={{
                value:
                  displayMode === 'dollar'
                    ? t('initialBalance', language).split(' ')[0]
                    : '0%',
                fill: '#848E9C',
                fontSize: 12,
              }}
            />
            <Area
              type="natural"
              dataKey="value"
              stroke="none"
              fill="url(#equityAreaFill)"
              isAnimationActive={false}
              connectNulls
            />
            <Line
              type="natural"
              dataKey="value"
              stroke="url(#colorGradient)"
              strokeWidth={2.5}
              dot={false}
              activeDot={{ r: 4, fill: '#d4ff33', stroke: '#1c1c1c', strokeWidth: 1 }}
              isAnimationActive
              animationDuration={520}
              animationEasing="ease-out"
              connectNulls
            />
          </ComposedChart>
        </ResponsiveContainer>
      </div>

      {/* Footer Stats */}
      <div
        className="mt-3 grid grid-cols-2 sm:grid-cols-4 gap-2 sm:gap-3 pt-3"
        style={{ borderTop: '1px solid #2B3139' }}
      >
        <div
          className="p-2 rounded transition-all hover:bg-opacity-50"
          style={{ background: 'rgba(240, 185, 11, 0.05)' }}
        >
          <div
            className="text-xs mb-1 uppercase tracking-wider"
            style={{ color: '#848E9C' }}
          >
            {t('initialBalance', language)}
          </div>
          <div
            className="text-xs sm:text-sm font-bold mono"
            style={{ color: '#EAECEF' }}
          >
            {initialBalance.toFixed(2)} USDT
          </div>
        </div>
        <div
          className="p-2 rounded transition-all hover:bg-opacity-50"
          style={{ background: 'rgba(240, 185, 11, 0.05)' }}
        >
          <div
            className="text-xs mb-1 uppercase tracking-wider"
            style={{ color: '#848E9C' }}
          >
            {t('currentEquity', language)}
          </div>
          <div
            className="text-xs sm:text-sm font-bold mono"
            style={{ color: '#EAECEF' }}
          >
            {currentValue.raw_equity.toFixed(2)} USDT
          </div>
        </div>
        <div
          className="p-2 rounded transition-all hover:bg-opacity-50"
          style={{ background: 'rgba(240, 185, 11, 0.05)' }}
        >
          <div
            className="text-xs mb-1 uppercase tracking-wider"
            style={{ color: '#848E9C' }}
          >
            {t('historicalCycles', language)}
          </div>
          <div
            className="text-xs sm:text-sm font-bold mono"
            style={{ color: '#EAECEF' }}
          >
            {validHistory.length} {t('cycles', language)}
          </div>
        </div>
        <div
          className="p-2 rounded transition-all hover:bg-opacity-50"
          style={{ background: 'rgba(240, 185, 11, 0.05)' }}
        >
          <div
            className="text-xs mb-1 uppercase tracking-wider"
            style={{ color: '#848E9C' }}
          >
            {t('displayRange', language)}
          </div>
          <div
            className="text-xs sm:text-sm font-bold mono"
            style={{ color: '#EAECEF' }}
          >
            {validHistory.length > MAX_DISPLAY_POINTS
              ? `${t('recent', language)} ${MAX_DISPLAY_POINTS}`
              : t('allData', language)}
          </div>
        </div>
      </div>
    </div>
  )
}
