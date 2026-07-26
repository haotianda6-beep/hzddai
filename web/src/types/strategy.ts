/** 策略市场权限（与后端 market_access 一致） */
export type StrategyMarketAccess =
  | 'off'
  | 'private'
  | 'subscription'
  | 'public'
  | 'open_source'

// Strategy Studio Types
export interface Strategy {
  id: string
  name: string
  description: string
  is_active: boolean
  is_default: boolean
  /** 策略市场权限：off=不上架，private=仅展示，subscription=需订阅，public=公开可复制配置，open_source=开源可复制提示词 */
  market_access?: StrategyMarketAccess
  /** 曾修改过策略名称：off 时也会在策略市场以「仅展示」出现 */
  show_after_rename?: boolean
  is_public: boolean // 兼容：与后端同步
  config_visible: boolean // 兼容：仅 public 时为 true
  /** 市场复制来源；非开源复制件会锁定内容，只能使用不能编辑 */
  source_strategy_id?: string
  source_market_access?: StrategyMarketAccess
  content_locked?: boolean
  /** 每次「更新到策略市场」后端递增，购买者可对比是否需同步 */
  market_revision?: number
  config: StrategyConfig
  created_at: string
  updated_at: string
}

// 策略使用统计
export interface StrategyStats {
  clone_count: number // 被克隆次数
  active_users: number // 当前使用人数
  top_performers?: StrategyPerformer[] // 收益排行
}

// 策略使用者收益排行
export interface StrategyPerformer {
  user_id: string
  user_name: string // 脱敏后的用户名
  total_pnl_pct: number // 总收益率
  total_pnl: number // 总收益金额
  win_rate: number // 胜率
  trade_count: number // 交易次数
  using_since: string // 使用开始时间
  rank: number // 排名
}

export interface PromptSectionsConfig {
  role_definition?: string
  trading_frequency?: string
  entry_standards?: string
  decision_process?: string
}

export interface StrategyConfig {
  // Strategy type: ai_trading | grid_trading | program_martingale
  strategy_type?: 'ai_trading' | 'grid_trading' | 'program_martingale'
  // Language setting: "zh" for Chinese, "en" for English
  // Determines the language used for data formatting and prompt generation
  language?: 'zh' | 'en'
  coin_source: CoinSourceConfig
  indicators: IndicatorConfig
  custom_prompt?: string
  /** 给 AI 的一整段策略说明（优先于旧的 prompt_sections） */
  strategy_prompt?: string
  /** 上架策略市场时的售卖标价（USDT），仅存于 config JSON */
  market_sale_price_usdt?: number
  /** 客户上架策略市场的管理员审核状态 */
  market_review_status?: 'pending' | 'approved' | 'rejected' | string
  market_review_requested_access?: StrategyMarketAccess
  market_review_requested_at?: string
  market_review_reviewed_at?: string
  market_review_reviewed_by?: string
  /**
   * Comkun 合规跟单：为 true 时绑定此策略的交易员周期不走真实 LLM，
   * 而消费管理员发布的「主广播」+ 每轮扣减虚拟 comkun token（见 /api/comkun/*）
   */
  comkun_market_follow?: boolean
  /** 与主广播 source_strategy_id 一致，通常为市场源策略的 strategies.id */
  comkun_market_source_strategy_id?: string
  /** 每轮扫描消耗的虚拟 token；0 或未设则用后端默认 */
  comkun_follow_tokens_per_scan?: number
  /** 官方上架「跟单开关」模板时为 true；他人从市场复制后自动变为跟单子策略 */
  comkun_follow_listing_template?: boolean
  /** 历史兼容字段，新配置请用 comkun_listing_master_skip_exchange_execution */
  comkun_listing_template_master_allow_execute?: boolean
  /**
   * 为 true 时：主控「上架模板」不向交易所执行 AI 决策（仅分析与广播），避免 AI 平/撤你在所的仓位与挂单。
   * 默认 false（仍按 AI 在交易所执行并存快照到广播）。
   */
  comkun_listing_master_skip_exchange_execution?: boolean
  /**
   * 主控 SOL 人工广播：无 SOLUSDT 持仓时对外决策 JSON 固定为 wait；有持仓时去掉 open_*，仅保留 SOL 相关 hold/wait/close。
   * 同时 AI 用户提示会附加「进场/止盈/止损」解读规则（思维链为主）。
   */
  comkun_listing_master_sol_manual_broadcast_mode?: boolean
  /**
   * 为 true 时：被控按主广播里的交易所快照（持仓+挂单）按比例对齐仓位、限价、止盈止损；
   * 从市场复制官方跟单模板时后端默认 true；需主控为「上架模板」且每轮写入 master_state_json；当前以币安合约 GridTrader 为主。
   */
  comkun_follow_mirror_master_exchange?: boolean
  /** 镜像：主广播里写入的假定杠杆，用于「初始保证金≈名义/杠杆」占主控权益比例；默认 20 */
  comkun_mirror_master_margin_leverage?: number
  /** 镜像：被控还原名义与 SetLeverage 使用的杠杆；默认 20 */
  comkun_mirror_follower_margin_leverage?: number
  /**
   * 已废弃：镜像引擎固定走 reconcileComkunFollowMasterStateV2，保留字段仅兼容旧 JSON。
   * 币安镜像市价路径由部署环境 COMKUN_MIRROR_V2_WS_ORDER=1 等控制（见服务端 comkun_env_timing）。
   * 仅主控上架模板有效；跟单子策略不在此配置。
   */
  comkun_listing_master_mirror_reconcile_engine?: string
  risk_control: RiskControlConfig
  /** 旧版四段拆分；仅当 strategy_prompt 为空时参与合并 */
  prompt_sections?: PromptSectionsConfig
  // Grid trading configuration (only used when strategy_type is 'grid_trading')
  grid_config?: GridStrategyConfig | null
  /** 程序化马丁（COMKUN-AI、无 LLM） */
  martingale_program?: MartingaleProgramConfig | null
}

export interface MartingaleProgramConfig {
  symbol: string
  leverage: number
  max_layers: number
  /** 1～N 层保证金占比，和为 1；空则用 MT5 默认 7 层权重 */
  layer_weights?: number[]
  /** 全部层合计占用净值保证金比例，如 0.06 = 6% */
  margin_budget_pct: number
  /** 每层逆势间距（相对首仓价），如 0.006 = 0.6% */
  add_step_pct: number
  /** basket 浮盈/已用保证金 ≥ 该值则全平 */
  basket_take_profit_roe: number
  /** 4h EMA20/50 最小分离度才认定趋势 */
  trend_min_sep_pct: number
  allow_short: boolean
  max_basket_loss_roe?: number
  daily_loss_limit_pct?: number
  budget_use_available_only?: boolean
  /** 币安 20x 单层初始保证金下限，默认 0.46U */
  min_layer_margin_usdt?: number
}

export const PROGRAM_MARTINGALE_STRATEGY_ID = 'mt5-xau-martingale-bn-draft-v1'

/** 构建器显示程序马丁：类型为 program_martingale 或白名单草稿 */
export function isProgramMartingaleStrategyStudioStrategy(
  strategyId: string | undefined | null,
  strategyType?: string | null
): boolean {
  return (
    strategyType === 'program_martingale' ||
    strategyId === PROGRAM_MARTINGALE_STRATEGY_ID
  )
}

export const defaultMartingaleProgramConfig: MartingaleProgramConfig = {
  symbol: 'XAUUSDT',
  leverage: 20,
  max_layers: 7,
  layer_weights: [0.0298, 0.0472, 0.0754, 0.1197, 0.1904, 0.303, 0.4845],
  margin_budget_pct: 0.06,
  add_step_pct: 0.006,
  basket_take_profit_roe: 0.025,
  trend_min_sep_pct: 0.0008,
  allow_short: true,
  max_basket_loss_roe: 0.12,
  daily_loss_limit_pct: 8,
  budget_use_available_only: false,
  min_layer_margin_usdt: 0.46,
}

// Grid trading specific configuration
export interface GridStrategyConfig {
  // Trading pair (e.g., "BTCUSDT")
  symbol: string
  // Number of grid levels (5-50)
  grid_count: number
  // Total investment in USDT
  total_investment: number
  // Leverage (1-20)
  leverage: number
  // Upper price boundary (0 = auto-calculate from ATR)
  upper_price: number
  // Lower price boundary (0 = auto-calculate from ATR)
  lower_price: number
  // Use ATR to auto-calculate bounds
  use_atr_bounds: boolean
  // ATR multiplier for bound calculation (default 2.0)
  atr_multiplier: number
  // Position distribution: "uniform" | "gaussian" | "pyramid"
  distribution: 'uniform' | 'gaussian' | 'pyramid'
  // Maximum drawdown percentage before emergency exit
  max_drawdown_pct: number
  // Stop loss percentage per position
  stop_loss_pct: number
  // Daily loss limit percentage
  daily_loss_limit_pct: number
  // Use maker-only orders for lower fees
  use_maker_only: boolean
  // Enable automatic grid direction adjustment based on box breakouts
  enable_direction_adjust?: boolean
  // Direction bias ratio for long_bias/short_bias modes (default 0.7 = 70%/30%)
  direction_bias_ratio?: number
}

export interface CoinSourceConfig {
  source_type: 'static' | 'ai500' | 'oi_top' | 'oi_low' | 'mixed'
  static_coins?: string[]
  excluded_coins?: string[] // 排除的币种列表
  use_ai500: boolean
  ai500_limit?: number
  use_oi_top: boolean
  oi_top_limit?: number
  use_oi_low: boolean
  oi_low_limit?: number
  // Note: API URLs are now built automatically using nofxos_api_key from IndicatorConfig
}

export interface IndicatorConfig {
  klines: KlineConfig
  // Raw OHLCV kline data - required for AI analysis
  enable_raw_klines: boolean
  // Technical indicators (optional)
  enable_ema: boolean
  enable_macd: boolean
  enable_rsi: boolean
  enable_atr: boolean
  enable_boll: boolean
  enable_volume: boolean
  enable_oi: boolean
  enable_funding_rate: boolean
  ema_periods?: number[]
  rsi_periods?: number[]
  atr_periods?: number[]
  boll_periods?: number[]
  external_data_sources?: ExternalDataSource[]

  // ========== NofxOS 数据源统一配置 ==========
  // Unified NofxOS API Key - used for all NofxOS data sources
  nofxos_api_key?: string

  // 量化数据源（资金流向、持仓变化、价格变化）
  enable_quant_data?: boolean
  enable_quant_oi?: boolean
  enable_quant_netflow?: boolean

  // OI 排行数据（市场持仓量增减排行）
  enable_oi_ranking?: boolean
  oi_ranking_duration?: string // "1h", "4h", "24h"
  oi_ranking_limit?: number

  // NetFlow 排行数据（机构/散户资金流向排行）
  enable_netflow_ranking?: boolean
  netflow_ranking_duration?: string // "1h", "4h", "24h"
  netflow_ranking_limit?: number

  // Price 排行数据（涨跌幅排行）
  enable_price_ranking?: boolean
  price_ranking_duration?: string // "1h", "4h", "24h" or "1h,4h,24h"
  price_ranking_limit?: number
}

export interface KlineConfig {
  primary_timeframe: string
  primary_count: number
  longer_timeframe?: string
  longer_count?: number
  enable_multi_timeframe: boolean
  // 新增：支持选择多个时间周期
  selected_timeframes?: string[]
}

export interface ExternalDataSource {
  name: string
  type: 'api' | 'webhook'
  url: string
  method: string
  headers?: Record<string, string>
  data_path?: string
  refresh_secs?: number
}

export interface RiskControlConfig {
  // Max number of coins held simultaneously (CODE ENFORCED)
  max_positions: number

  // Trading Leverage - exchange leverage for opening positions (AI guided)
  btc_eth_max_leverage: number // BTC/ETH max exchange leverage
  altcoin_max_leverage: number // Altcoin max exchange leverage

  // Position Value Ratio - single position notional value / account equity (CODE ENFORCED)
  // Max position value = equity × this ratio
  btc_eth_max_position_value_ratio?: number // default: 5 (BTC/ETH max position = 5x equity)
  altcoin_max_position_value_ratio?: number // default: 1 (Altcoin max position = 1x equity)

  // Risk Parameters
  max_margin_usage: number // Max margin utilization, e.g. 0.9 = 90% (CODE ENFORCED)
  min_position_size: number // Min position size in USDT (CODE ENFORCED)
  min_risk_reward_ratio: number // Min take_profit / stop_loss ratio (AI guided)
  min_confidence: number // Min AI confidence to open position (AI guided)
}
