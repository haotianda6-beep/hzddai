import type {
  Strategy,
  StrategyConfig,
} from '../../types'
import { API_BASE, httpClient } from './helpers'

/** GET /api/strategies/public/:id 详情页汇总（无需登录） */
export type PublicStrategyMarketAggregateTrading = {
  stats: {
    total_trades: number
    win_trades: number
    loss_trades: number
    win_rate: number
    profit_factor: number
    sharpe_ratio: number
    total_pnl: number
    total_fee: number
    avg_win: number
    avg_loss: number
    max_drawdown_pct: number
  }
  avg_hold_ms: number
  gross_profit: number
  gross_loss: number
  long_trades: number
  short_trades: number
}

export type PublicStrategyMarketDetailDemoOverlay = {
  enabled?: boolean
  env_key?: string
  name_match_substr?: string
  note_zh?: string
}

/** 策略市场演示叠加：单笔成交展示（静默测试等） */
export type PublicStrategyDemoTradeHighlight = {
  symbol: string
  side: string
  entry_price: number
  exit_price: number
  pnl_usdt: number
  opened_at: string
}

export type PublicStrategyDemoAiInsight = {
  at: string
  content: string
}

export type PublicStrategyMarketTradeHistoryRow = {
  id: string
  symbol: string
  contractLabel: '永续'
  leverage: string
  marginMode: '全仓' | '逐仓'
  direction: '多' | '空'
  status: '已平仓'
  opened: string
  entryPrice: string
  maxOpenInterest: string
  closingPnl: string
  closed: string
  avgClosePrice: string
  closedVol: string
}

export type PublicStrategyMarketDetailPayload = {
  strategy: Record<string, unknown>
  initial_capital: number
  aggregate_trading: PublicStrategyMarketAggregateTrading
  /** 绑定该策略的交易员所用交易所（取首条，大写） */
  exchange_label?: string
  /** live=数据库汇总；demo_overlay=演示叠加（见 demo_overlay） */
  stats_source?: 'live' | 'demo_overlay'
  demo_overlay?: PublicStrategyMarketDetailDemoOverlay
  trade_history?: PublicStrategyMarketTradeHistoryRow[]
}

export const strategyApi = {
  /** 策略市场详情（净值曲线汇总区间见 stats.stats_window_days） */
  async getPublicStrategyMarketDetail(strategyId: string): Promise<PublicStrategyMarketDetailPayload> {
    const result = await httpClient.get<PublicStrategyMarketDetailPayload>(
      `${API_BASE}/strategies/public/${encodeURIComponent(strategyId)}`
    )
    if (!result.success || !result.data) {
      throw new Error(result.message || 'Failed to load strategy detail')
    }
    return result.data
  },

  async getStrategies(): Promise<Strategy[]> {
    const result = await httpClient.get<{ strategies: Strategy[] }>(`${API_BASE}/strategies`)
    if (!result.success) throw new Error('Failed to fetch strategy list')
    const strategies = result.data?.strategies
    return Array.isArray(strategies) ? strategies : []
  },

  async getStrategy(strategyId: string): Promise<Strategy> {
    const result = await httpClient.get<Strategy>(`${API_BASE}/strategies/${strategyId}`)
    if (!result.success) throw new Error('Failed to fetch strategy')
    return result.data!
  },

  async getActiveStrategy(): Promise<Strategy> {
    const result = await httpClient.get<Strategy>(`${API_BASE}/strategies/active`)
    if (!result.success) throw new Error('Failed to fetch active strategy')
    return result.data!
  },

  async getDefaultStrategyConfig(): Promise<StrategyConfig> {
    const result = await httpClient.get<StrategyConfig>(`${API_BASE}/strategies/default-config`)
    if (!result.success) throw new Error('Failed to fetch default strategy config')
    return result.data!
  },

  async createStrategy(data: {
    name: string
    description: string
    config: StrategyConfig
  }): Promise<Strategy> {
    const result = await httpClient.post<Strategy>(`${API_BASE}/strategies`, data)
    if (!result.success) throw new Error('Failed to create strategy')
    return result.data!
  },

  async updateStrategy(
    strategyId: string,
    data: {
      name?: string
      description?: string
      config?: StrategyConfig
    }
  ): Promise<Strategy> {
    const result = await httpClient.put<Strategy>(`${API_BASE}/strategies/${strategyId}`, data)
    if (!result.success) throw new Error('Failed to update strategy')
    return result.data!
  },

  async deleteStrategy(strategyId: string): Promise<void> {
    const result = await httpClient.delete(`${API_BASE}/strategies/${strategyId}`)
    if (!result.success) throw new Error('Failed to delete strategy')
  },

  async activateStrategy(strategyId: string): Promise<Strategy> {
    const result = await httpClient.post<Strategy>(`${API_BASE}/strategies/${strategyId}/activate`)
    if (!result.success) throw new Error('Failed to activate strategy')
    return result.data!
  },

  /**
   * 从源策略复制一条「我的策略」（市场已购 / 公开等权限由后端校验）。
   * 后端返回 { id, message }，不是完整 Strategy 对象。
   */
  async duplicateStrategy(strategyId: string, name: string): Promise<{ id: string }> {
    const result = await httpClient.post<{ id: string; message?: string }>(
      `${API_BASE}/strategies/${encodeURIComponent(strategyId)}/duplicate`,
      { name }
    )
    if (!result.success || !result.data?.id) {
      throw new Error(result.message || '复制策略失败')
    }
    return { id: result.data.id }
  },

  /** 跟单开关上架模板：主控广播 + 绑定本策略的交易员挂单记录（策略构建器看板） */
  async getComkunMasterBoard(strategyId: string): Promise<ComkunMasterBoardPayload> {
    const result = await httpClient.get<ComkunMasterBoardPayload>(
      `${API_BASE}/strategies/${encodeURIComponent(strategyId)}/comkun-master-board`,
      undefined,
      undefined,
      { timeout: 90000, silent: true }
    )
    if (!result.success) throw new Error(result.message || 'Failed to load comkun master board')
    return result.data!
  },
}

export type ScreenMonitorPosition = {
  symbol: string
  side: string
  entry_price: number
  quantity: number
  leverage: number
  margin_used: number
  mark_price?: number
  unrealized_pnl?: number
  unrealized_pnl_pct?: number
  update_time: number
}

export type ScreenInstruction = {
  type: string
  symbol: string
  side?: string
  entry_price?: number
  quantity?: number
  added_quantity?: number
  reduced_quantity?: number
  remaining_quantity?: number
  leverage?: number
  margin_used?: number
  prev_margin?: number
  margin_change?: number
  prev_side?: string
  timestamp?: number
  reason?: string
}

export type ScreenMonitorMasterState = {
  v: number
  positions: ScreenMonitorPosition[]
  instructions?: ScreenInstruction[]
}

export type ComkunMasterBoardBroadcast = {
  id: number
  source_strategy_id: string
  master_account_equity: number
  created_at: string
  analysis_preview: string
  decision_count: number
  master_state?: ScreenMonitorMasterState
}

export type ComkunMasterBoardOrder = {
  id: number | string
  trader_id: string
  symbol: string
  side: string
  position_side: string
  type: string
  status: string
  quantity: number
  price: number
  stop_price: number
  filled_quantity: number
  avg_fill_price: number
  leverage: number
  order_action: string
  created_at: number
  updated_at: number
  /** 服务端从类型/触发价推导的止盈价（有条件单或 TAKE_PROFIT 类时） */
  take_profit_price?: number
  /** 服务端推导的止损价（TRAILING_STOP、STOP_LOSS 或 _sl 拆条等） */
  stop_loss_price?: number
  /** qty×参考价/杠杆（无杠杆时按 1x），约数 */
  estimated_margin?: number
}

export type ComkunMasterBoardPosition = {
  id: number | string
  trader_id: string
  symbol: string
  side: string
  quantity: number
  entry_price: number
  /** 服务端来自交易所快照的标记价，缺省不传 */
  mark_price?: number
  leverage: number
  status: string
  unrealized_pnl?: number
  created_at: number
  updated_at: number
}

export type ComkunMasterBoardFollower = {
  trader_id: string
  trader_name: string
  user_id: string
  user_label: string
  user_email_masked: string
  strategy_id: string
  /** 跟单用户侧策略名称（便于识别如「大黑猎手」） */
  strategy_name?: string
  is_running: boolean
  exchange_type: string
  /** 消费失败或最近决策报错摘要（主控看板） */
  status_hint?: string
  positions: ComkunMasterBoardPosition[]
  orders: ComkunMasterBoardOrder[]
}

export type ComkunMasterBoardPayload = {
  strategy_id: string
  master_trader_id: string
  master_trader_name: string
  broadcasts: ComkunMasterBoardBroadcast[]
  orders: ComkunMasterBoardOrder[]
  followers: ComkunMasterBoardFollower[]
}
