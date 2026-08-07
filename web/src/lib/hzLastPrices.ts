import type { HZLastPrice, Position } from '../types'

export const HZ_LAST_PRICE_REFRESH_MS = 200

export function mergeHZLastPrices(
  positions: Position[] | undefined,
  prices: HZLastPrice[] | undefined
): Position[] | undefined {
  if (!positions?.length || !prices?.length) return positions
  const bySymbol = new Map(prices.map((price) => [price.symbol, price]))
  return positions.map((position) => {
    const price = bySymbol.get(position.symbol)
    if (!price || (position.last_price_time ?? 0) >= price.time) return position
    return {
      ...position,
      last_price: price.price,
      last_price_time: price.time,
      last_price_source: price.source,
    }
  })
}
