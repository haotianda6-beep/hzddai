import { describe, expect, it } from 'vitest'

import { isHistoricalOnlyStrategy } from './strategyMarketDisplay'

describe('isHistoricalOnlyStrategy', () => {
  it('recognizes historical profiles and keeps normal public strategies actionable', () => {
    expect(isHistoricalOnlyStrategy({ performance_only: true })).toBe(true)
    expect(
      isHistoricalOnlyStrategy({ performance_source: 'historical_simulation' })
    ).toBe(true)
    expect(isHistoricalOnlyStrategy({ realtime_follow_available: false })).toBe(
      true
    )
    expect(isHistoricalOnlyStrategy({ market_access: 'public' })).toBe(false)
  })
})
