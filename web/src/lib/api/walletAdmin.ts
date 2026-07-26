import { API_BASE, handleJSONResponse, getAuthHeaders } from './helpers'

export type WalletLedgerRow = {
  id: number
  user_id: string
  delta: number
  balance_after: number
  reason: string
  ref_strategy_id: string
  created_at: string
}

export type MarketEntitlement = {
  user_id: string
  strategy_id: string
  amount_paid: number
  created_at: string
  /** ISO8601，包月/包周到期时间；过期后仍保留解锁，恢复按轮扣费 */
  subscription_until?: string | null
}

export type MarketSubscriptionAlert = {
  kind: 'expiring_soon' | 'expired_recent' | string
  strategy_id: string
  subscription_until: string
  message_zh: string
  message_en?: string
}

export type AccountLockState = {
  locked: boolean
  kind?: 'balance_debt' | 'market_subscription_expired' | string
  severity?: 'critical' | string
  blocking?: boolean
  balance_usdt?: number
  required_recharge_usdt?: number
  strategy_id?: string
  strategy_name?: string
  subscription_until?: string
  amount_paid?: number
  title_zh?: string
  message_zh?: string
  primary_action_zh?: string
}

export type UserNotificationRow = {
  id: number
  user_id: string
  /** true 表示全站公告，所有人可见 */
  broadcast?: boolean
  title: string
  body: string
  created_at: string
}

export type InvitedUserRow = {
  id: string
  email: string
  display_name: string
  created_at: string
}

export type InviteMePayload = {
  invite_code: string
  invite_link: string
  invited_count: number
  reward_text: string
  invited_users: InvitedUserRow[]
  generated_at: string
}

export async function getInviteMe(): Promise<InviteMePayload> {
  const res = await fetch(`${API_BASE}/invite/me`, {
    headers: getAuthHeaders(),
  })
  return handleJSONResponse(res)
}

/** GET /api/invite/network — 伞下关系树、主站可返佣消费合计、返利 VIP（若已同步） */
export type InviteNetworkNode = {
  id: string
  email: string
  display_name: string
  created_at: string
  balance_usdt: number
  direct_invite_count: number
  personal_consumption_usdt: number
  team_consumption_usdt: number
  rebate_synced: boolean
  rebate_vip_level?: number
  /** 运营起步档：自动升级不会低于该档（0 表示纯按团队业绩） */
  rebate_vip_level_floor?: number
  rebate_vip_level_locked?: boolean
  rebate_is_studio?: boolean
  rebate_team_total?: string
  rebate_balance_usdt?: string
  rebate_recharge_balance_usdt?: string
  /** 运营总账号等：不参与返利统计与发放 */
  rebate_exempt?: boolean
  children: InviteNetworkNode[]
}

export type InviteNetworkPayload = {
  invite_code: string
  invite_link: string
  rebate_metrics_available: boolean
  root: {
    id: string
    email: string
    display_name: string
    direct_invite_count: number
    personal_consumption_usdt: number
    team_consumption_usdt: number
    rebate_synced?: boolean
    rebate_vip_level?: number
    rebate_vip_level_floor?: number
    rebate_vip_level_locked?: boolean
    rebate_is_studio?: boolean
    rebate_team_total?: string
    rebate_balance_usdt?: string
    rebate_recharge_balance_usdt?: string
    rebate_exempt?: boolean
  }
  tree: InviteNetworkNode[]
  stats: { total_descendants: number; max_depth: number }
  generated_at: string
}

export async function getInviteNetwork(): Promise<InviteNetworkPayload> {
  const res = await fetch(`${API_BASE}/invite/network`, {
    headers: getAuthHeaders(),
  })
  return handleJSONResponse(res)
}

export async function getUserNotifications(): Promise<{
  notifications: UserNotificationRow[]
}> {
  const res = await fetch(`${API_BASE}/notifications`, {
    headers: getAuthHeaders(),
  })
  return handleJSONResponse(res)
}

/** 管理员：发布全站公告（所有登录用户可见） */
export async function postAdminBroadcastNotification(
  title: string,
  body: string
): Promise<{ message: string }> {
  const res = await fetch(`${API_BASE}/admin/notifications/broadcast`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({ title, body }),
  })
  return handleJSONResponse(res)
}

export async function postAdminStrategyMarketReview(
  strategyId: string,
  action: 'approve' | 'reject'
): Promise<{
  message: string
  strategy_id: string
  action: string
  market_access: string
}> {
  const res = await fetch(
    `${API_BASE}/admin/strategies/${encodeURIComponent(strategyId)}/market-review`,
    {
      method: 'POST',
      headers: getAuthHeaders(),
      body: JSON.stringify({ action }),
    }
  )
  return handleJSONResponse(res)
}

export async function getWallet(): Promise<{
  balance_usdt: number
  ledger: WalletLedgerRow[]
  entitlements: MarketEntitlement[]
  account_lock?: AccountLockState
  market_subscription_alerts?: MarketSubscriptionAlert[]
  /** 为 true 时不可再购买周卡体验（每账号仅一次） */
  market_weekly_trial_used?: boolean
  is_admin: boolean
  is_finance?: boolean
}> {
  const res = await fetch(`${API_BASE}/wallet`, { headers: getAuthHeaders() })
  return handleJSONResponse(res)
}

/** 邀请返利（Python 返利服务）余额：由后端代理，需配置 AGENT_REBATE_* */
export type AgentRebateBalanceResponse =
  | { configured: false }
  | { configured: true; synced: false; message?: string }
  | {
      configured: true
      synced: true
      rebate_balance_usdt: string
      recharge_balance_usdt: string
      rebate_nickname?: string
      rebate_external_uid?: string
      rebate_vip_level?: number
      rebate_vip_level_floor?: number
      rebate_vip_level_locked?: boolean
      /** 工作室：直推名义分成 +5% */
      rebate_is_studio?: boolean
      /** 为 true 时不展示返佣操作入口（运营账号） */
      rebate_exempt?: boolean
    }

export async function getAgentRebateBalance(): Promise<AgentRebateBalanceResponse> {
  const res = await fetch(`${API_BASE}/user/agent-rebate-balance`, {
    headers: getAuthHeaders(),
  })
  return handleJSONResponse(res)
}

/** 返利余额 → 返利系统内「充值记账」（不可直接等同于站内 AI 余额） */
export async function postAgentRebateTransferToRecharge(
  amount: string
): Promise<{ ok: boolean }> {
  const res = await fetch(
    `${API_BASE}/user/agent-rebate/transfer-to-recharge`,
    {
      method: 'POST',
      headers: getAuthHeaders(),
      body: JSON.stringify({ amount: amount.trim() }),
    }
  )
  return handleJSONResponse(res)
}

/** VIP5 周分红发放明细（返利侧入账到返佣余额后的历史记录） */
export type AgentRebateDividendRow = {
  amount_usdt: string
  week_start: string
  week_end: string
  pool_usdt: string
  weight_team_total?: string
  created_at?: string | null
}

export async function getAgentRebateDividends(): Promise<
  | { configured: false; rows: [] }
  | {
      configured: true
      synced?: boolean
      rows: AgentRebateDividendRow[]
      error?: string
    }
> {
  const res = await fetch(`${API_BASE}/user/agent-rebate-dividends`, {
    headers: getAuthHeaders(),
  })
  return handleJSONResponse(res)
}

export async function postWalletRecharge(
  amountUsdt: number
): Promise<{ balance_usdt: number; message?: string }> {
  const res = await fetch(`${API_BASE}/wallet/recharge`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({ amount_usdt: amountUsdt }),
  })
  return handleJSONResponse(res)
}

export async function postMarketPurchase(
  strategyId: string,
  plan: 'monthly' | 'weekly' | 'free'
): Promise<{
  balance_usdt: number
  strategy_id: string
  paid_usdt: number
  plan?: string
  subscription_until?: string | null
  message?: string
}> {
  const res = await fetch(`${API_BASE}/strategies/market/purchase`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({ strategy_id: strategyId, plan }),
  })
  return handleJSONResponse(res)
}

export type FinanceUserLiteRow = {
  id: string
  email: string
  display_name: string
  balance_usdt: number
  created_at: string
}

/** 财务台：用户简表（用于查找客户并入账） */
export async function getFinanceUsersLite(): Promise<{
  users: FinanceUserLiteRow[]
  generated_at: string
}> {
  const res = await fetch(`${API_BASE}/finance/users-lite`, {
    headers: getAuthHeaders(),
  })
  return handleJSONResponse(res)
}

/** 财务台：为客户正数入账（与管理员正数调账同逻辑） */
export async function postFinanceWalletAdjust(
  userId: string,
  deltaUsdt: number,
  note?: string
): Promise<{ user_id: string; balance_usdt: number }> {
  const res = await fetch(
    `${API_BASE}/finance/users/${encodeURIComponent(userId)}/wallet-adjust`,
    {
      method: 'POST',
      headers: getAuthHeaders(),
      body: JSON.stringify({ delta_usdt: deltaUsdt, note: note || '' }),
    }
  )
  return handleJSONResponse(res)
}

export async function getMarketOwnedStrategy(strategyId: string): Promise<{
  id: string
  name: string
  description: string
  market_access: string
  config: unknown
}> {
  const res = await fetch(
    `${API_BASE}/strategies/market/owned/${encodeURIComponent(strategyId)}`,
    {
      headers: getAuthHeaders(),
    }
  )
  return handleJSONResponse(res)
}

export type AdminUserRow = {
  id: string
  email: string
  display_name: string
  balance_usdt: number
  created_at: string
  trader_count: number
  exchanges: {
    id: string
    exchange_type: string
    account_name: string
    enabled: boolean
  }[]
  /** 管理后台代理绑定等：用户下交易员及绑定的交易所 */
  traders?: Array<{
    id: string
    name: string
    exchange_id: string
    exchange_type: string
    account_name: string
    enabled?: boolean
    missing_exchange?: boolean
  }>
  binance_open_positions: {
    trader_id: string
    trader_name: string
    symbol: string
    side: string
    size: number
  }[]
}

export type AdminRunningTraderRow = {
  trader_id: string
  trader_name: string
  user_id: string
  user_email: string
  user_display_name: string
  /** 数据库是否标记为运行中（与管理后台启停一致） */
  is_running?: boolean
  total_equity?: number
  available_balance?: number
  total_pnl?: number
  total_pnl_pct?: number
  /** 已用保证金（USDT，与账户接口一致） */
  margin_used?: number
  margin_used_pct?: number
  total_unrealized_profit: number
  position_count: number
  /** 本机 equity 快照时间（UTC）；快照降级时有值 */
  snapshot_at?: string
  /** exchange_live=刚才请求的交易所实时；equity_snapshot=本机库快照 */
  metrics_source?: string
  /** 交易所实时拉取时间（UTC RFC3339），仅 metrics_source=exchange_live */
  checked_at?: string
  positions?: Array<{
    symbol: string
    side: string
    quantity?: number
    size?: number
    entry_price?: number
    mark_price?: number
    unrealized_pnl?: number
    leverage?: number
  }>
  error?: string
}

export type AdminAIPlatformUsageRow = {
  id: string
  user_id: string
  user_email: string
  user_display_name: string
  trader_id: string
  provider: string
  model: string
  actual_cost_usdc: number
  charged_usdt: number
  markup_multiplier: number
  wallet_balance_before: number
  wallet_balance_after: number
  status: string
  payment_tx_hash: string
  error_message: string
  created_at: string
}

export type AdminBinanceBrokerRebateRow = {
  market: string
  asset: string
  amount: number
  customer_id?: string
  sub_account_id?: string
  symbol?: string
  income_type?: string
  time?: number
  raw?: Record<string, unknown>
}

export async function getAdminBinanceBrokerRebates(): Promise<{
  configured: boolean
  message?: string
  items: AdminBinanceBrokerRebateRow[]
  totals: Record<string, number>
  errors?: string[]
  generated_at?: string
}> {
  const res = await fetch(`${API_BASE}/admin/binance-broker-rebates`, {
    headers: getAuthHeaders(),
  })
  return handleJSONResponse(res)
}

export type AdminInvitePartner = {
  id: string
  email: string
  display_name: string
  invite_code: string
  invite_link: string
  customer_count: number
  customers: Array<{
    id: string
    email: string
    display_name: string
    balance_usdt: number
    created_at: string
  }>
}

export async function getAdminInvitesOverview(): Promise<{
  partners: AdminInvitePartner[]
  generated_at: string
}> {
  const res = await fetch(`${API_BASE}/admin/invites-overview`, {
    headers: getAuthHeaders(),
  })
  return handleJSONResponse(res)
}

export async function getAdminUsersOverview(): Promise<{
  users: AdminUserRow[]
  running_traders: AdminRunningTraderRow[]
  /** 运行中列表数据来源说明（如本地库快照） */
  running_traders_meta?: { source?: string; hint?: string }
  generated_at: string
}> {
  const res = await fetch(`${API_BASE}/admin/users-overview`, {
    headers: getAuthHeaders(),
  })
  return handleJSONResponse(res)
}

export async function getAdminAIPlatformUsage(
  userId?: string
): Promise<{ items: AdminAIPlatformUsageRow[]; generated_at: string }> {
  const q = userId ? `?user_id=${encodeURIComponent(userId)}` : ''
  const res = await fetch(`${API_BASE}/admin/ai-platform-usage${q}`, {
    headers: getAuthHeaders(),
  })
  return handleJSONResponse(res)
}

export async function getAIPlatformUsage(
  limit = 50
): Promise<{ items: AdminAIPlatformUsageRow[]; generated_at: string }> {
  const q = `?limit=${encodeURIComponent(String(limit))}`
  const res = await fetch(`${API_BASE}/ai-platform-usage${q}`, {
    headers: getAuthHeaders(),
  })
  return handleJSONResponse(res)
}

export async function postAdminWalletAdjust(
  userId: string,
  deltaUsdt: number,
  note?: string
): Promise<{ user_id: string; balance_usdt: number }> {
  const res = await fetch(
    `${API_BASE}/admin/users/${encodeURIComponent(userId)}/wallet-adjust`,
    {
      method: 'POST',
      headers: getAuthHeaders(),
      body: JSON.stringify({ delta_usdt: deltaUsdt, note: note || '' }),
    }
  )
  return handleJSONResponse(res)
}

/** 管理员：返利侧设置 VIP、锁定等级、工作室（须服务器配置 AGENT_REBATE_*） */
export async function postAdminRebateSetUserAttrs(payload: {
  user_id: string
  vip_level?: number
  vip_level_locked?: boolean
  is_studio?: boolean
}): Promise<Record<string, unknown>> {
  const res = await fetch(`${API_BASE}/admin/rebate/set-user-attrs`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(payload),
  })
  return handleJSONResponse(res)
}

/** 管理员：对全部「市场合规跟单」交易员在交易所市价平仓（可选仅某一合约） */
/** 管理员：代用户启动交易员 */
export async function postAdminTraderStart(
  traderId: string
): Promise<{ message?: string }> {
  const res = await fetch(
    `${API_BASE}/admin/traders/${encodeURIComponent(traderId)}/start`,
    {
      method: 'POST',
      headers: getAuthHeaders(),
    }
  )
  return handleJSONResponse(res)
}

/** 管理员：代用户停止交易员 */
export async function postAdminTraderStop(
  traderId: string
): Promise<{ message?: string; warning?: string }> {
  const res = await fetch(
    `${API_BASE}/admin/traders/${encodeURIComponent(traderId)}/stop`,
    {
      method: 'POST',
      headers: getAuthHeaders(),
    }
  )
  return handleJSONResponse(res)
}

/** 管理员：按交易所当前持仓重写本机 OPEN 记录（消除幽灵持仓） */
export async function postAdminTraderSyncPositionsFromExchange(
  traderId: string
): Promise<{ message?: string }> {
  const res = await fetch(
    `${API_BASE}/admin/traders/${encodeURIComponent(traderId)}/sync-positions-from-exchange`,
    {
      method: 'POST',
      headers: getAuthHeaders(),
    }
  )
  return handleJSONResponse(res)
}

export async function postAdminFlattenAllComkunFollowTraders(
  symbol?: string
): Promise<{
  message: string
  symbol_filter: string
  trader_count: number
  ok_count: number
  fail_count: number
  results: Array<{
    trader_id: string
    trader_name?: string
    user_id: string
    exchange_type?: string
    ok: boolean
    error?: string
    closed?: Array<{
      symbol?: string
      side?: string
      ok?: boolean
      error?: string
    }>
  }>
}> {
  const body: { symbol?: string } = {}
  const s = symbol?.trim()
  if (s) body.symbol = s
  const res = await fetch(
    `${API_BASE}/admin/comkun/flatten-all-follow-traders`,
    {
      method: 'POST',
      headers: getAuthHeaders(),
      body: JSON.stringify(body),
    }
  )
  return handleJSONResponse(res)
}

/** 管理员：SOCKS5 代理池条目 */
export type AdminOutboundProxyPoolRow = {
  id: string
  display_host: string
  expires_at?: string | null
  assigned_user_id: string
  assigned_exchange_id: string
  assigned_at?: string | null
  created_at: string
  assigned_user_email: string
  assigned_user_display_name: string
  assigned_exchange_account_name: string
  seconds_until_expiry?: number
}

export async function getAdminOutboundProxyPool(): Promise<{
  entries: AdminOutboundProxyPoolRow[]
}> {
  const res = await fetch(`${API_BASE}/admin/outbound-proxy-pool`, {
    headers: getAuthHeaders(),
  })
  return handleJSONResponse(res)
}

/** 管理员：出口代理 REST 失败告警 */
export type AdminOutboundProxyFaultRow = {
  id: number
  user_id: string
  trader_id: string
  exchange_id: string
  proxy_redacted: string
  display_host: string
  error_type: string
  last_error: string
  hit_count: number
  first_at: string
  last_at: string
  user_email: string
  user_display_name: string
  trader_name: string
}

export async function getAdminOutboundProxyFaults(limit = 50): Promise<{
  events: AdminOutboundProxyFaultRow[]
}> {
  const res = await fetch(
    `${API_BASE}/admin/outbound-proxy-faults?limit=${limit}`,
    {
      headers: getAuthHeaders(),
    }
  )
  return handleJSONResponse(res)
}

export async function postAdminOutboundProxyPoolImport(lines: string): Promise<{
  added: number
  skipped: number
  errors: string[]
}> {
  const res = await fetch(`${API_BASE}/admin/outbound-proxy-pool/import`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({ lines }),
  })
  return handleJSONResponse(res)
}

export async function deleteAdminOutboundProxyPoolEntry(
  id: string
): Promise<{ message: string }> {
  const res = await fetch(
    `${API_BASE}/admin/outbound-proxy-pool/${encodeURIComponent(id)}`,
    {
      method: 'DELETE',
      headers: getAuthHeaders(),
    }
  )
  return handleJSONResponse(res)
}

export async function postAdminOutboundProxyPoolRelease(
  id: string
): Promise<{ message: string }> {
  const res = await fetch(
    `${API_BASE}/admin/outbound-proxy-pool/${encodeURIComponent(id)}/release`,
    {
      method: 'POST',
      headers: getAuthHeaders(),
    }
  )
  return handleJSONResponse(res)
}

/** 将未分配的池条目绑定到指定用户的交易所，并写入出站代理 URL */
export async function postAdminOutboundProxyPoolAssign(
  id: string,
  body: { user_id: string; exchange_id: string }
): Promise<{ message: string }> {
  const res = await fetch(
    `${API_BASE}/admin/outbound-proxy-pool/${encodeURIComponent(id)}/assign`,
    {
      method: 'POST',
      headers: getAuthHeaders(),
      body: JSON.stringify(body),
    }
  )
  return handleJSONResponse(res)
}
