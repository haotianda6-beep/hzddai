/**
 * 与 agent_rebate_backend/app/vip_constants.py 中 VIP_THRESHOLDS 的 extra 一致。
 * - extra：在「基础直推 15%」之上的等级额外奖励（百分数）。
 * - tier 合计：返利执行直推比例 = 15% + extra（即 tier_total_percent，不含工作室）。
 * - 工作室：直推名义再 +5%（STUDIO_EXTRA_PERCENT，见 commission.py）。
 * - VIP0 实发直推另可在返利服务按「本人充值」封顶，此处仅展示名义比例。
 */
export const REBATE_BASE_DIRECT_PERCENT = 15

/** 工作室直推额外加成（百分点），与后端 STUDIO_EXTRA_PERCENT 一致 */
export const REBATE_STUDIO_EXTRA_PERCENT = 5

/** 等级额外奖励（%），对应后端 Decimal extra */
export const REBATE_VIP_EXTRA_PERCENT: Record<number, number> = {
  1: 12,
  2: 20,
  3: 26,
  4: 31,
  5: 35,
}

/** 某 VIP 等级的「额外」部分（VIP0 为 0） */
export function rebateVipExtraPercent(vipLevel: number): number {
  if (vipLevel <= 0) return 0
  return REBATE_VIP_EXTRA_PERCENT[vipLevel] ?? 0
}

/** 返利侧直推合计比例（整数百分比），与 tier_total_percent×100 一致（不含工作室加成） */
export function rebateVipTierTotalPercent(vipLevel: number): number {
  if (vipLevel <= 0) return REBATE_BASE_DIRECT_PERCENT
  const extra = REBATE_VIP_EXTRA_PERCENT[vipLevel]
  if (extra === undefined) return REBATE_BASE_DIRECT_PERCENT
  return REBATE_BASE_DIRECT_PERCENT + extra
}

/** 含工作室时的直推名义合计（展示用） */
export function rebateVipTierTotalPercentWithStudio(vipLevel: number): number {
  return rebateVipTierTotalPercent(vipLevel) + REBATE_STUDIO_EXTRA_PERCENT
}
