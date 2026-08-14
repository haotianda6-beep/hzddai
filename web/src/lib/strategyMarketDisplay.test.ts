import { describe, expect, it } from 'vitest'

import {
  isHistoricalOnlyStrategy,
  isSimulatedPerformance,
  recentBalanceSeries,
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

describe('recentBalanceSeries', () => {
  it('keeps the latest 40 real cumulative balances', () => {
    const balances = Array.from({ length: 70 }, (_, index) => 100 + index)
    const recent = recentBalanceSeries(balances)

    expect(recent).toHaveLength(40)
    expect(recent[0]).toBe(130)
    expect(recent.at(-1)).toBe(169)
  })

  it('preserves drawdowns and ignores invalid balances', () => {
    expect(recentBalanceSeries([100, Number.NaN, 110, 99])).toEqual([
      100, 110, 99,
    ])
  })
})
