import type { Statistics } from '../types'

export const HZ_DETAIL_POSITIONS_REFRESH_MS = 5000

export function getTraderDashboardLabels(isHZ: boolean) {
  if (isHZ) {
    return {
      equity: 'BALIB AI 账户权益',
      available: 'BALIB AI 可用余额',
      asset: 'USD',
    }
  }
  return {
    equity: '账户权益',
    available: '可用余额',
    asset: 'USDT',
  }
}

export function getTraderDashboardPolling(isHZ: boolean) {
  return {
    positionsMs: isHZ ? HZ_DETAIL_POSITIONS_REFRESH_MS : 15000,
    accountMs: 15000,
    retryCount: 2,
    marketPricesIndependent: true,
  }
}

export function getWinRateDisplay(
  stats: Pick<Statistics, 'total_trades'> | undefined,
  language: string
) {
  if (!stats || (stats.total_trades ?? 0) <= 0) {
    return { value: language === 'zh' ? '暂无成交' : 'No trades yet', unit: '' }
  }
  return { value: undefined, unit: '%' }
}
