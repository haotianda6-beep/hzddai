import { describe, expect, it } from 'vitest'

import {
  isHistoricalOnlyStrategy,
  isSimulatedPerformance,
  recentTradeReturnSeries,
} from './strategyMarketDisplay'

describe('isHistoricalOnlyStrategy', () => {
  it('recognizes historical profiles and keeps normal public strategies actionable', () => {
    expect(isHistoricalOnlyStrategy({ performance_only: true })).toBe(true)
    expect(
      isHistoricalOnlyStrategy({
        performance_source: 'historical_simulation',
        realtime_follow_available: true,
      })
    ).toBe(false)
    expect(isHistoricalOnlyStrategy({ realtime_follow_available: false })).toBe(
      true
    )
    expect(isHistoricalOnlyStrategy({ market_access: 'public' })).toBe(false)
  })

  it('keeps simulation provenance independent from live availability', () => {
    expect(
      isSimulatedPerformance({ performance_source: 'historical_simulation' })
    ).toBe(true)
    expect(isSimulatedPerformance({ realtime_follow_available: true })).toBe(
      false
    )
  })
})

describe('recentTradeReturnSeries', () => {
  it('converts the latest 61 balances into 60 real per-trade returns', () => {
    const balances = Array.from({ length: 70 }, (_, index) => 100 + index)
    const returns = recentTradeReturnSeries(balances)

    expect(returns).toHaveLength(60)
    expect(returns[0]).toBeCloseTo((110 - 109) / 109)
    expect(returns.at(-1)).toBeCloseTo((169 - 168) / 168)
  })

  it('preserves losses and ignores invalid balances', () => {
    expect(recentTradeReturnSeries([100, Number.NaN, 110, 99])).toEqual([
      0.1,
      -0.1,
    ])
  })
})
