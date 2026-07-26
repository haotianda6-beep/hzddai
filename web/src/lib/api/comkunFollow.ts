import { API_BASE, getAuthHeaders, handleJSONResponse } from './helpers'

/** 合规跟单：按交易员 ID 查询站内账户余额，扣费统一走平台余额。 */
export async function getComkunFollowBalance(traderId: string): Promise<{
  trader_id: string
  balance_usdt: number
  scan_fee_usdt: number
  scan_fee_min_usdt?: number
  scan_fee_max_usdt?: number
  billing: 'platform_wallet'
}> {
  const res = await fetch(
    `${API_BASE}/comkun/follow-balance?trader_id=${encodeURIComponent(traderId)}`,
    { headers: getAuthHeaders() }
  )
  return handleJSONResponse(res)
}
