import { describe, expect, it } from 'vitest'
import type { Position } from '../types'
import { HZ_LAST_PRICE_REFRESH_MS, mergeHZLastPrices } from './hzLastPrices'

describe('HZ last-price fast path', () => {
  it('polls inside the 200-250ms budget', () => {
    expect(HZ_LAST_PRICE_REFRESH_MS).toBeGreaterThanOrEqual(200)
    expect(HZ_LAST_PRICE_REFRESH_MS).toBeLessThanOrEqual(250)
  })

  it('updates only last trade fields and keeps mark/PnL snapshot unchanged', () => {
    const position = {
      symbol: 'NXPCUSDT',
      mark_price: 0.2384,
      unrealized_pnl: 1.5,
      last_price: 0.2382,
      last_price_time: 100,
    } as Position
    const merged = mergeHZLastPrices([position], [{
      symbol: 'NXPCUSDT', price: 0.2383, time: 200, source: 'binance_agg_trade_ws',
    }])[0]

    expect(merged.last_price).toBe(0.2383)
    expect(merged.last_price_time).toBe(200)
    expect(merged.mark_price).toBe(0.2384)
    expect(merged.unrealized_pnl).toBe(1.5)
  })
})
