import { describe, expect, it } from 'vitest'
import {
  getTraderDashboardLabels,
  getTraderDashboardPolling,
  getWinRateDisplay,
} from './traderDashboardData'

describe('trader detail data states', () => {
  it('names HZ funds as BALIB AI funds instead of contract balance', () => {
    expect(getTraderDashboardLabels(true)).toEqual({
      equity: 'BALIB AI 账户权益',
      available: 'BALIB AI 可用余额',
      asset: 'USD',
    })
  })

  it('shows an explicit empty trade state for zero trades', () => {
    expect(getWinRateDisplay({ total_trades: 0 }, 'zh')).toEqual({
      value: '暂无成交',
      unit: '',
    })
  })

  it('bounds HZ detail polling and keeps market prices separate', () => {
    expect(getTraderDashboardPolling(true)).toMatchObject({
      positionsMs: 5000,
      accountMs: 15000,
      retryCount: 2,
      marketPricesIndependent: true,
    })
  })
})
