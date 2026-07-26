import type { Strategy } from '../types/strategy'

/**
 * 从市场源 id 复制出的「我的策略」副本，后端会写入 source_strategy_id = 市场源策略 id。
 * 列表接口按 created_at DESC，传入顺序中第一个匹配项即最新副本。
 */
export function findExistingForkIdForMarketSource(
  marketListingStrategyId: string,
  strategies: Strategy[]
): string | undefined {
  const mid = marketListingStrategyId.trim()
  if (!mid) return undefined
  for (const s of strategies) {
    if ((s.source_strategy_id ?? '').trim() === mid) return s.id
  }
  return undefined
}
