import { useState, useEffect, useMemo } from 'react'
import { api } from '../../lib/api'
import { useLanguage } from '../../contexts/LanguageContext'
import { t } from '../../i18n/translations'
import { formatPrice, formatQuantity } from '../../utils/format'
import { NofxSelect } from '../ui/select'
import type { HistoricalPosition } from '../../types'

interface PositionHistoryProps {
  traderId: string
}

// Format number with proper decimals (for large numbers)
function formatNumber(value: number, decimals: number = 2): string {
  if (Math.abs(value) >= 1000000) {
    return (value / 1000000).toFixed(2) + 'M'
  }
  if (Math.abs(value) >= 1000) {
    return (value / 1000).toFixed(2) + 'K'
  }
  return value.toFixed(decimals)
}

// Format duration from minutes
function formatDuration(minutes: number): string {
  if (!minutes || minutes <= 0) return '-'
  if (minutes < 60) return `${minutes.toFixed(0)}m`
  if (minutes < 1440) return `${(minutes / 60).toFixed(1)}h`
  return `${(minutes / 1440).toFixed(1)}d`
}

// Format date
function formatDate(dateStr: string): string {
  if (!dateStr) return '-'
  const date = new Date(dateStr)
  if (isNaN(date.getTime())) return '-'
  return date.toLocaleDateString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function getPositionView(position: HistoricalPosition) {
  const side = position.side || ''
  const isLong = side.toUpperCase() === 'LONG'
  const realizedPnl = position.realized_pnl || 0
  const isProfitable = realizedPnl >= 0
  const sideColor = isLong ? '#0ECB81' : '#F6465D'
  const pnlColor = isProfitable ? '#0ECB81' : '#F6465D'

  // Calculate holding time
  const entryTime = position.entry_time ? new Date(position.entry_time).getTime() : 0
  const exitTime = position.exit_time ? new Date(position.exit_time).getTime() : 0
  const holdingMinutes = entryTime && exitTime && exitTime > entryTime ? (exitTime - entryTime) / 60000 : 0

  // Calculate PnL percentage based on entry price
  const entryPrice = position.entry_price || 0
  const exitPrice = position.exit_price || 0
  let pnlPct = 0
  if (entryPrice > 0) {
    if (isLong) {
      pnlPct = ((exitPrice - entryPrice) / entryPrice) * 100
    } else {
      pnlPct = ((entryPrice - exitPrice) / entryPrice) * 100
    }
  }

  // Use entry_quantity for display (original position size)
  const displayQty = position.entry_quantity || position.quantity || 0

  return {
    side,
    isLong,
    sideColor,
    pnlColor,
    isProfitable,
    realizedPnl,
    entryPrice,
    exitPrice,
    displayQty,
    pnlPct,
    holdingMinutes,
  }
}

function PositionCard({ position, language }: { position: HistoricalPosition; language: 'zh' | 'en' | 'id' }) {
  const v = getPositionView(position)

  return (
    <div className="rounded-lg border border-[#2B3139] bg-black/25 p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className="truncate font-mono text-sm font-semibold" style={{ color: '#EAECEF' }}>
              {(position.symbol || '').replace('USDT', '')}
            </span>
            <span
              className="rounded px-2 py-0.5 text-[10px] font-semibold uppercase"
              style={{
                background: `${v.sideColor}22`,
                color: v.sideColor,
                border: `1px solid ${v.sideColor}44`,
              }}
            >
              {v.side}
            </span>
          </div>
          <div className="mt-1 text-[11px]" style={{ color: '#848E9C' }}>
            {formatDate(position.exit_time)} · {formatDuration(v.holdingMinutes)}
          </div>
        </div>
        <div className="shrink-0 text-right">
          <div className="font-mono text-base font-semibold" style={{ color: v.pnlColor }}>
            {v.isProfitable ? '+' : ''}
            {formatNumber(v.realizedPnl)}
          </div>
          <div className="text-xs" style={{ color: v.pnlColor }}>
            {v.pnlPct >= 0 ? '+' : ''}
            {v.pnlPct.toFixed(2)}%
          </div>
        </div>
      </div>

      <div className="mt-3 grid grid-cols-2 gap-2 text-[11px]">
        <div>
          <div style={{ color: '#5e6673' }}>{t('positionHistory.entry', language)}</div>
          <div className="font-mono" style={{ color: '#EAECEF' }}>{formatPrice(v.entryPrice)}</div>
        </div>
        <div>
          <div style={{ color: '#5e6673' }}>{t('positionHistory.exit', language)}</div>
          <div className="font-mono" style={{ color: '#EAECEF' }}>{formatPrice(v.exitPrice)}</div>
        </div>
        <div>
          <div style={{ color: '#5e6673' }}>{t('positionHistory.qty', language)}</div>
          <div className="font-mono" style={{ color: '#848E9C' }}>{formatQuantity(v.displayQty)}</div>
        </div>
        <div>
          <div style={{ color: '#5e6673' }}>{t('positionHistory.value', language)}</div>
          <div className="font-mono" style={{ color: '#EAECEF' }}>{formatNumber(v.entryPrice * v.displayQty)}</div>
        </div>
      </div>
    </div>
  )
}

// Position Row Component
function PositionRow({ position }: { position: HistoricalPosition }) {
  const v = getPositionView(position)

  return (
    <tr
      className="transition-all duration-200 hover:bg-white/5"
      style={{ borderBottom: '1px solid #2B3139' }}
    >
      {/* Symbol */}
      <td className="py-3 px-4">
        <div className="flex items-center gap-2">
          <span className="font-mono font-semibold" style={{ color: '#EAECEF' }}>
            {(position.symbol || '').replace('USDT', '')}
          </span>
          <span
            className="px-2 py-0.5 rounded text-xs font-semibold uppercase"
            style={{
              background: `${v.sideColor}22`,
              color: v.sideColor,
              border: `1px solid ${v.sideColor}44`,
            }}
          >
            {v.side}
          </span>
        </div>
      </td>

      {/* Entry Price */}
      <td className="py-3 px-4 text-right font-mono" style={{ color: '#EAECEF' }}>
        {formatPrice(v.entryPrice)}
      </td>

      {/* Exit Price */}
      <td className="py-3 px-4 text-right font-mono" style={{ color: '#EAECEF' }}>
        {formatPrice(v.exitPrice)}
      </td>

      {/* Quantity */}
      <td className="py-3 px-4 text-right font-mono" style={{ color: '#848E9C' }}>
        {formatQuantity(v.displayQty)}
      </td>

      {/* Position Value (Entry Price * Quantity) */}
      <td className="py-3 px-4 text-right font-mono" style={{ color: '#EAECEF' }}>
        {formatNumber(v.entryPrice * v.displayQty)}
      </td>

      {/* P&L */}
      <td className="py-3 px-4 text-right">
        <div className="font-mono font-semibold" style={{ color: v.pnlColor }}>
          {v.isProfitable ? '+' : ''}
          {formatNumber(v.realizedPnl)}
        </div>
        <div className="text-xs" style={{ color: v.pnlColor }}>
          {v.pnlPct >= 0 ? '+' : ''}
          {v.pnlPct.toFixed(2)}%
        </div>
      </td>

      {/* Fee - show more precision for small fees */}
      <td className="py-3 px-4 text-right font-mono text-xs" style={{ color: '#848E9C' }}>
        -{((position.fee || 0) < 0.01 && (position.fee || 0) > 0)
          ? (position.fee || 0).toFixed(4)
          : (position.fee || 0).toFixed(2)}
      </td>

      {/* Duration */}
      <td className="py-3 px-4 text-center text-sm" style={{ color: '#848E9C' }}>
        {formatDuration(v.holdingMinutes)}
      </td>

      {/* Exit Time */}
      <td className="py-3 px-4 text-right text-xs" style={{ color: '#848E9C' }}>
        {formatDate(position.exit_time)}
      </td>
    </tr>
  )
}

export function PositionHistory({ traderId }: PositionHistoryProps) {
  const { language } = useLanguage()
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [positions, setPositions] = useState<HistoricalPosition[]>([])

  // Pagination state
  const [pageSize, setPageSize] = useState<number>(20)
  const [currentPage, setCurrentPage] = useState<number>(1)

  // Filter state
  const [filterSymbol, setFilterSymbol] = useState<string>('all')
  const [filterSide, setFilterSide] = useState<string>('all')
  const [sortBy, setSortBy] = useState<'time' | 'pnl' | 'pnl_pct'>('time')
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc')

  useEffect(() => {
    const fetchData = async () => {
      try {
        setLoading(true)
        setError(null)
        // Fetch more data than needed to support filtering, but respect pageSize for initial load
        const data = await api.getPositionHistory(
          traderId,
          Math.max(200, pageSize * 5),
          true
        )
        setPositions(data.positions || [])
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load history')
      } finally {
        setLoading(false)
      }
    }

    if (traderId) {
      fetchData()
    }
  }, [traderId, pageSize])

  // Get unique symbols for filter
  const uniqueSymbols = useMemo(() => {
    const symbols = new Set(positions.map((p) => p.symbol))
    return Array.from(symbols).sort()
  }, [positions])

  // Filtered and sorted positions (before pagination)
  const filteredAndSortedPositions = useMemo(() => {
    let result = [...positions]

    // Apply filters
    if (filterSymbol !== 'all') {
      result = result.filter((p) => p.symbol === filterSymbol)
    }
    if (filterSide !== 'all') {
      result = result.filter(
        (p) => (p.side || '').toUpperCase() === filterSide.toUpperCase()
      )
    }

    // Apply sorting
    result.sort((a, b) => {
      let comparison = 0
      switch (sortBy) {
        case 'time':
          comparison =
            new Date(a.exit_time || 0).getTime() - new Date(b.exit_time || 0).getTime()
          break
        case 'pnl':
          comparison = (a.realized_pnl || 0) - (b.realized_pnl || 0)
          break
        case 'pnl_pct': {
          const aPrice = a.entry_price || 1
          const bPrice = b.entry_price || 1
          const aPct = ((a.exit_price || 0) - aPrice) / aPrice * 100
          const bPct = ((b.exit_price || 0) - bPrice) / bPrice * 100
          comparison = aPct - bPct
          break
        }
      }
      return sortOrder === 'desc' ? -comparison : comparison
    })

    return result
  }, [positions, filterSymbol, filterSide, sortBy, sortOrder])

  // Pagination calculations
  const totalFilteredCount = filteredAndSortedPositions.length
  const totalPages = Math.ceil(totalFilteredCount / pageSize)

  // Reset to page 1 when filters change
  useEffect(() => {
    setCurrentPage(1)
  }, [filterSymbol, filterSide, sortBy, sortOrder, pageSize])

  // Paginated positions (for display)
  const paginatedPositions = useMemo(() => {
    const startIndex = (currentPage - 1) * pageSize
    return filteredAndSortedPositions.slice(startIndex, startIndex + pageSize)
  }, [filteredAndSortedPositions, currentPage, pageSize])

  // For backwards compatibility, keep filteredPositions as the paginated result
  const filteredPositions = paginatedPositions

  if (loading) {
    return (
      <div
        className="flex items-center justify-center p-12"
        style={{ color: '#848E9C' }}
      >
        <div className="animate-spin mr-3">
          <svg className="w-6 h-6" fill="none" viewBox="0 0 24 24">
            <circle
              className="opacity-25"
              cx="12"
              cy="12"
              r="10"
              stroke="currentColor"
              strokeWidth="4"
            />
            <path
              className="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
            />
          </svg>
        </div>
        {t('positionHistory.loading', language)}
      </div>
    )
  }

  if (error) {
    return (
      <div
        className="rounded-lg p-6 text-center"
        style={{
          background: 'rgba(246, 70, 93, 0.1)',
          border: '1px solid rgba(246, 70, 93, 0.3)',
          color: '#F6465D',
        }}
      >
        {error}
      </div>
    )
  }

  if (positions.length === 0) {
    return (
      <div
        className="rounded-lg p-12 text-center"
        style={{
          background: 'linear-gradient(135deg, #1c1c1c 0%, #131313 100%)',
          border: '1px solid #2B3139',
        }}
      >
        <div className="text-4xl mb-4">📊</div>
        <div className="text-lg font-semibold mb-2" style={{ color: '#EAECEF' }}>
          {t('positionHistory.noHistory', language)}
        </div>
        <div style={{ color: '#848E9C' }}>
          {t('positionHistory.noHistoryDesc', language)}
        </div>
      </div>
    )
  }

  return (
    <div
      className="rounded-lg overflow-hidden"
      style={{
        background: 'linear-gradient(135deg, #1c1c1c 0%, #131313 100%)',
        border: '1px solid #2B3139',
      }}
    >
      {/* Filters */}
      <div
        className="flex flex-col items-stretch gap-3 p-3 sm:flex-row sm:flex-wrap sm:items-center sm:gap-4 sm:p-4"
        style={{ borderBottom: '1px solid #2B3139' }}
      >
          <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:gap-2">
            <span className="text-sm" style={{ color: '#848E9C' }}>
              {t('positionHistory.symbol', language)}:
            </span>
            <NofxSelect
              value={filterSymbol}
              onChange={(val) => setFilterSymbol(val)}
              options={[
                { value: 'all', label: t('positionHistory.allSymbols', language) },
                ...uniqueSymbols.map(s => ({ value: s, label: (s || '').replace('USDT', '') }))
              ]}
              className="w-full rounded px-3 py-1.5 text-sm sm:w-auto"
              style={{
                background: '#0b0b0b',
                border: '1px solid #2B3139',
                color: '#EAECEF',
              }}
            />
          </div>

          <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:gap-2">
            <span className="text-sm" style={{ color: '#848E9C' }}>
              {t('positionHistory.side', language)}:
            </span>
            <div className="flex overflow-hidden rounded" style={{ border: '1px solid #2B3139' }}>
              {['all', 'LONG', 'SHORT'].map((side) => (
                <button
                  key={side}
                  onClick={() => setFilterSide(side)}
                  className="flex-1 px-3 py-1.5 text-sm capitalize transition-colors sm:flex-none"
                  style={{
                    background: filterSide === side ? '#2B3139' : 'transparent',
                    color: filterSide === side ? '#EAECEF' : '#848E9C',
                  }}
                >
                  {side === 'all' ? t('positionHistory.all', language) : side}
                </button>
              ))}
            </div>
          </div>

          <div className="flex flex-col gap-1 sm:ml-auto sm:flex-row sm:items-center sm:gap-2">
            <span className="text-sm" style={{ color: '#848E9C' }}>
              {t('positionHistory.sort', language)}:
            </span>
            <NofxSelect
              value={`${sortBy}-${sortOrder}`}
              onChange={(val) => {
                const [by, order] = val.split('-') as ['time' | 'pnl' | 'pnl_pct', 'asc' | 'desc']
                setSortBy(by)
                setSortOrder(order)
              }}
              options={[
                { value: 'time-desc', label: t('positionHistory.latestFirst', language) },
                { value: 'time-asc', label: t('positionHistory.oldestFirst', language) },
                { value: 'pnl-desc', label: t('positionHistory.highestPnL', language) },
                { value: 'pnl-asc', label: t('positionHistory.lowestPnL', language) },
              ]}
              className="w-full rounded px-3 py-1.5 text-sm sm:w-auto"
              style={{
                background: '#0b0b0b',
                border: '1px solid #2B3139',
                color: '#EAECEF',
              }}
            />
          </div>
        </div>

        <div className="space-y-2 p-3 md:hidden">
          {filteredPositions.map((position) => (
            <PositionCard key={position.id} position={position} language={language} />
          ))}
        </div>

        {/* Table */}
        <div className="hidden overflow-x-auto md:block">
          <table className="w-full">
            <thead>
              <tr style={{ background: '#0b0b0b' }}>
                <th
                  className="py-3 px-4 text-left text-xs font-semibold uppercase tracking-wider"
                  style={{ color: '#848E9C' }}
                >
                  {t('positionHistory.symbol', language)}
                </th>
                <th
                  className="py-3 px-4 text-right text-xs font-semibold uppercase tracking-wider"
                  style={{ color: '#848E9C' }}
                >
                  {t('positionHistory.entry', language)}
                </th>
                <th
                  className="py-3 px-4 text-right text-xs font-semibold uppercase tracking-wider"
                  style={{ color: '#848E9C' }}
                >
                  {t('positionHistory.exit', language)}
                </th>
                <th
                  className="py-3 px-4 text-right text-xs font-semibold uppercase tracking-wider"
                  style={{ color: '#848E9C' }}
                >
                  {t('positionHistory.qty', language)}
                </th>
                <th
                  className="py-3 px-4 text-right text-xs font-semibold uppercase tracking-wider"
                  style={{ color: '#848E9C' }}
                >
                  {t('positionHistory.value', language)}
                </th>
                <th
                  className="py-3 px-4 text-right text-xs font-semibold uppercase tracking-wider"
                  style={{ color: '#848E9C' }}
                >
                  {t('positionHistory.pnl', language)}
                </th>
                <th
                  className="py-3 px-4 text-right text-xs font-semibold uppercase tracking-wider"
                  style={{ color: '#848E9C' }}
                >
                  {t('positionHistory.fee', language)}
                </th>
                <th
                  className="py-3 px-4 text-center text-xs font-semibold uppercase tracking-wider"
                  style={{ color: '#848E9C' }}
                >
                  {t('positionHistory.duration', language)}
                </th>
                <th
                  className="py-3 px-4 text-right text-xs font-semibold uppercase tracking-wider"
                  style={{ color: '#848E9C' }}
                >
                  {t('positionHistory.closedAt', language)}
                </th>
              </tr>
            </thead>
            <tbody>
              {filteredPositions.map((position) => (
                <PositionRow key={position.id} position={position} />
              ))}
            </tbody>
          </table>
        </div>

        {/* Footer with Pagination */}
        <div
          className="flex flex-col items-stretch justify-between gap-4 p-3 text-sm sm:flex-row sm:items-center sm:p-4"
          style={{ borderTop: '1px solid #2B3139', color: '#848E9C' }}
        >
          {/* Left: Count info */}
          <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:gap-4">
            <span>
              {t('positionHistory.showingPositions', language, { count: totalFilteredCount, total: positions.length })}
            </span>
            {totalFilteredCount > 0 && (
              <span>
                {t('positionHistory.totalPnL', language)}:{' '}
                <span
                  style={{
                    color:
                      filteredAndSortedPositions.reduce((sum, p) => sum + (p.realized_pnl || 0), 0) >= 0
                        ? '#0ECB81'
                        : '#F6465D',
                  }}
                >
                  {filteredAndSortedPositions.reduce((sum, p) => sum + (p.realized_pnl || 0), 0) >= 0
                    ? '+'
                    : ''}
                  {formatNumber(
                    filteredAndSortedPositions.reduce((sum, p) => sum + (p.realized_pnl || 0), 0)
                  )}
                </span>
              </span>
            )}
          </div>

          {/* Right: Pagination controls */}
          <div className="flex flex-wrap items-center gap-3">
            {/* Page size selector */}
            <div className="flex items-center gap-2">
              <span className="text-xs" style={{ color: '#848E9C' }}>
                {language === 'zh' ? '每页' : 'Per page'}:
              </span>
              <NofxSelect
                value={pageSize}
                onChange={(val) => setPageSize(Number(val))}
                options={[
                  { value: 20, label: '20' },
                  { value: 50, label: '50' },
                  { value: 100, label: '100' },
                ]}
                className="rounded px-2 py-1 text-sm"
                style={{
                  background: '#0b0b0b',
                  border: '1px solid #2B3139',
                  color: '#EAECEF',
                }}
              />
            </div>

            {/* Page navigation */}
            {totalPages > 1 && (
              <div className="flex items-center gap-1">
                <button
                  onClick={() => setCurrentPage(1)}
                  disabled={currentPage === 1}
                  className="px-2 py-1 rounded text-xs transition-colors disabled:opacity-30"
                  style={{
                    background: currentPage === 1 ? 'transparent' : '#2B3139',
                    color: '#EAECEF',
                  }}
                >
                  «
                </button>
                <button
                  onClick={() => setCurrentPage((p) => Math.max(1, p - 1))}
                  disabled={currentPage === 1}
                  className="px-2 py-1 rounded text-xs transition-colors disabled:opacity-30"
                  style={{
                    background: currentPage === 1 ? 'transparent' : '#2B3139',
                    color: '#EAECEF',
                  }}
                >
                  ‹
                </button>
                <span className="px-3 text-xs" style={{ color: '#EAECEF' }}>
                  {currentPage} / {totalPages}
                </span>
                <button
                  onClick={() => setCurrentPage((p) => Math.min(totalPages, p + 1))}
                  disabled={currentPage === totalPages}
                  className="px-2 py-1 rounded text-xs transition-colors disabled:opacity-30"
                  style={{
                    background: currentPage === totalPages ? 'transparent' : '#2B3139',
                    color: '#EAECEF',
                  }}
                >
                  ›
                </button>
                <button
                  onClick={() => setCurrentPage(totalPages)}
                  disabled={currentPage === totalPages}
                  className="px-2 py-1 rounded text-xs transition-colors disabled:opacity-30"
                  style={{
                    background: currentPage === totalPages ? 'transparent' : '#2B3139',
                    color: '#EAECEF',
                  }}
                >
                  »
                </button>
              </div>
            )}
          </div>
        </div>
      </div>
  )
}
