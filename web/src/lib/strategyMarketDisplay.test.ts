import { describe, expect, it } from 'vitest'

import {
  isHistoricalOnlyStrategy,
  isSimulatedPerformance,
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
