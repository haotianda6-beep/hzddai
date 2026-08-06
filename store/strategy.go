package store

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Hard limits to prevent token explosion in AI requests
const (
	MaxCandidateCoins = 10
	MaxPositions      = 3
	MaxTimeframes     = 4
	MinKlineCount     = 10
	MaxKlineCount     = 30
)

// ClampLimits enforces product-level limits on strategy config to prevent token overflow.
func (c *StrategyConfig) ClampLimits() {
	// Clamp coin source limits
	// 重要：limit=0 在后端会被当成“默认更大值”（如 AI500 默认 30），会导致候选币暴增。
	// 这里把 0/负数统一归一到产品默认值（3），再做上限裁剪。
	if c.CoinSource.AI500Limit <= 0 {
		c.CoinSource.AI500Limit = 3
	}
	if c.CoinSource.OITopLimit <= 0 {
		c.CoinSource.OITopLimit = 3
	}
	if c.CoinSource.OILowLimit <= 0 {
		c.CoinSource.OILowLimit = 3
	}
	if c.CoinSource.HyperMainLimit <= 0 {
		c.CoinSource.HyperMainLimit = 20
	}
	if c.CoinSource.AI500Limit > MaxCandidateCoins {
		c.CoinSource.AI500Limit = MaxCandidateCoins
	}
	if c.CoinSource.OITopLimit > MaxCandidateCoins {
		c.CoinSource.OITopLimit = MaxCandidateCoins
	}
	if c.CoinSource.OILowLimit > MaxCandidateCoins {
		c.CoinSource.OILowLimit = MaxCandidateCoins
	}

	// Clamp static coins
	if len(c.CoinSource.StaticCoins) > MaxCandidateCoins {
		c.CoinSource.StaticCoins = c.CoinSource.StaticCoins[:MaxCandidateCoins]
	}

	// Clamp kline count
	if c.Indicators.Klines.PrimaryCount < MinKlineCount {
		c.Indicators.Klines.PrimaryCount = MinKlineCount
	}
	if c.Indicators.Klines.PrimaryCount > MaxKlineCount {
		c.Indicators.Klines.PrimaryCount = MaxKlineCount
	}
	if c.Indicators.Klines.LongerCount > MaxKlineCount {
		c.Indicators.Klines.LongerCount = MaxKlineCount
	}

	// Clamp timeframes
	if len(c.Indicators.Klines.SelectedTimeframes) > MaxTimeframes {
		c.Indicators.Klines.SelectedTimeframes = c.Indicators.Klines.SelectedTimeframes[:MaxTimeframes]
	}

	// Clamp max positions
	if c.RiskControl.MaxPositions > MaxPositions {
		c.RiskControl.MaxPositions = MaxPositions
	}

	if c.ComkunMirrorMasterMarginLeverage > 125 {
		c.ComkunMirrorMasterMarginLeverage = 125
	}
	if c.ComkunMirrorFollowerMarginLeverage > 125 {
		c.ComkunMirrorFollowerMarginLeverage = 125
	}

}

// StrategyStore strategy storage
type StrategyStore struct {
	db *gorm.DB
}

// Strategy strategy configuration
type Strategy struct {
	ID            string `gorm:"primaryKey" json:"id"`
	UserID        string `gorm:"column:user_id;not null;default:'';index" json:"user_id"`
	Name          string `gorm:"not null" json:"name"`
	Description   string `gorm:"default:''" json:"description"`
	IsActive      bool   `gorm:"column:is_active;default:false;index" json:"is_active"`
	IsDefault     bool   `gorm:"column:is_default;default:false" json:"is_default"`
	IsPublic      bool   `gorm:"column:is_public;default:false;index" json:"is_public"`         // 兼容旧逻辑：与 market_access 同步
	ConfigVisible bool   `gorm:"column:config_visible;default:true" json:"config_visible"`      // 兼容：仅 public 时为 true
	MarketAccess  string `gorm:"column:market_access;default:'off';index" json:"market_access"` // off/private/subscription/public/open_source
	// 策略市场展示用：该策略「建议/使用」的 AI 模型标识（仅用于市场卡片展示，不影响交易员实际模型选择）
	MarketAIModel string `gorm:"column:market_ai_model;default:'';index" json:"market_ai_model"`
	// 用户曾修改过策略名称：允许在 market_access=off 时仍出现在策略市场（按仅展示处理）
	ShowAfterRename    bool      `gorm:"column:show_after_rename;default:false;index" json:"show_after_rename"`
	SourceStrategyID   string    `gorm:"column:source_strategy_id;default:'';index" json:"source_strategy_id,omitempty"`
	SourceMarketAccess string    `gorm:"column:source_market_access;default:''" json:"source_market_access,omitempty"`
	ContentLocked      bool      `gorm:"column:content_locked;default:false" json:"content_locked,omitempty"`
	Config             string    `gorm:"not null;default:'{}'" json:"config"`
	MarketRevision     uint      `gorm:"column:market_revision;default:0" json:"market_revision"` // 每次「更新到策略市场」递增，便于订阅方对比版本
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (Strategy) TableName() string { return "strategies" }

// StrategyConfig strategy configuration details (JSON structure)
type StrategyConfig struct {
	// Strategy type: "ai_trading" (default), "grid_trading", or "program_martingale"
	StrategyType string `json:"strategy_type,omitempty"`

	// language setting: "zh" for Chinese, "en" for English
	// This determines the language used for data formatting and prompt generation
	Language string `json:"language,omitempty"`
	// coin source configuration
	CoinSource CoinSourceConfig `json:"coin_source"`
	// quantitative data configuration
	Indicators IndicatorConfig `json:"indicators"`
	// custom prompt (appended at the end)
	CustomPrompt string `json:"custom_prompt,omitempty"`
	// single user-authored strategy narrative (preferred over legacy prompt_sections)
	StrategyPrompt string `json:"strategy_prompt,omitempty"`
	// risk control configuration
	RiskControl RiskControlConfig `json:"risk_control"`
	// legacy editable sections (ignored when strategy_prompt is non-empty)
	PromptSections PromptSectionsConfig `json:"prompt_sections,omitempty"`

	// Grid trading configuration (only used when StrategyType == "grid_trading")
	GridConfig *GridStrategyConfig `json:"grid_config,omitempty"`

	// 程序化马丁（COMKUN-AI 占位、不走 LLM；StrategyType=program_martingale）
	MartingaleProgram *MartingaleProgramConfig `json:"martingale_program,omitempty"`

	// Strategy market listing price in USDT (optional, frontend-only contract until market checkout wired)
	MarketSalePriceUSDT float64 `json:"market_sale_price_usdt,omitempty"`
	// Strategy market admin review state for customer-created listings.
	MarketReviewStatus          string `json:"market_review_status,omitempty"`           // pending/approved/rejected
	MarketReviewRequestedAccess string `json:"market_review_requested_access,omitempty"` // requested market_access while pending
	MarketReviewRequestedAt     string `json:"market_review_requested_at,omitempty"`
	MarketReviewReviewedAt      string `json:"market_review_reviewed_at,omitempty"`
	MarketReviewReviewedBy      string `json:"market_review_reviewed_by,omitempty"`

	// Comkun 合规跟单：为 true 时交易员周期不走真实 LLM 下单，而消费主账户广播 + 虚拟 comkun token
	ComkunMarketFollow bool `json:"comkun_market_follow,omitempty"`
	// 对应策略市场「源策略」的 strategies.id（主账户发布广播时使用同一 ID）
	ComkunMarketSourceStrategyID string `json:"comkun_market_source_strategy_id,omitempty"`
	// 每轮扫描消耗的虚拟 token，0 表示使用后端默认 ComkunFollowDefaultTokensPerScan
	ComkunFollowTokensPerScan int `json:"comkun_follow_tokens_per_scan,omitempty"`
	// 上架模板：为 true 表示「策略市场跟单开关」官方策略；他人从市场复制时自动打开 comkun_market_follow
	ComkunFollowListingTemplate bool `json:"comkun_follow_listing_template,omitempty"`
	// 历史兼容：曾用「是否允许主控执行」表达；新逻辑请用 comkun_listing_master_skip_exchange_execution。
	ComkunListingTemplateMasterAllowExecute bool `json:"comkun_listing_template_master_allow_execute,omitempty"`
	// 为 true 时：主控「上架模板」不向交易所执行 AI 决策（仅分析与广播），避免 AI 平掉/撤掉你在交易所的挂单与仓位。
	ComkunListingMasterSkipExchangeExecution bool `json:"comkun_listing_master_skip_exchange_execution,omitempty"`
	// 为 true 时（仅主控上架模板、且非 comkun_market_follow）：广播前强制「无 SOLUSDT 持仓 → 决策仅 wait」；
	// 有持仓则去掉 open_long/open_short，并仅保留 SOLUSDT 相关决策；同时在 AI 用户提示末尾附加 SOL 人工解读规则。
	ComkunListingMasterSolManualBroadcastMode bool `json:"comkun_listing_master_sol_manual_broadcast_mode,omitempty"`
	// 已废弃：镜像引擎固定 v2；保留 JSON 字段仅兼容旧数据。
	ComkunListingMasterMirrorReconcileEngine string `json:"comkun_listing_master_mirror_reconcile_engine,omitempty"`
	// 历史字段：跟单子策略在广播含 master_state_json 时由后端自动镜像，不再依赖本开关；保留 JSON 兼容旧数据。
	ComkunFollowMirrorMasterExchange bool `json:"comkun_follow_mirror_master_exchange,omitempty"`
	// 镜像跟单：用「初始保证金 ≈ 名义/杠杆」占主控权益的比例同步到被控。主控侧写入广播 mirror_margin；默认 20。
	ComkunMirrorMasterMarginLeverage int `json:"comkun_mirror_master_margin_leverage,omitempty"`
	// 被控镜像还原名义时使用的杠杆（SetLeverage 与数量公式）；默认 20。跟单子策略可在 JSON 中单独改。
	ComkunMirrorFollowerMarginLeverage int `json:"comkun_mirror_follower_margin_leverage,omitempty"`
	// HZ 固定跟单比例；0 表示继续按 follower/master 实时权益比例计算。
	ComkunMirrorFollowerEquityRatio float64 `json:"comkun_mirror_follower_equity_ratio,omitempty"`
}

// MartingaleProgramConfig 程序化马丁：大趋势开仓 + 1～N 层按权益比例补仓（非网格）。
type MartingaleProgramConfig struct {
	Symbol              string    `json:"symbol"`
	Leverage            int       `json:"leverage"`
	MaxLayers           int       `json:"max_layers"`
	LayerWeights        []float64 `json:"layer_weights,omitempty"`
	MarginBudgetPct     float64   `json:"margin_budget_pct"`
	AddStepPct          float64   `json:"add_step_pct"`
	BasketTakeProfitROE float64   `json:"basket_take_profit_roe"`
	TrendMinSepPct      float64   `json:"trend_min_sep_pct"`
	AllowShort          bool      `json:"allow_short"`
	// 整篮浮亏/已用保证金 ≤ -该值则止损全平（如 0.12 = -12% ROE）
	MaxBasketLossROE float64 `json:"max_basket_loss_roe,omitempty"`
	// 当日净值回撤 % 达限则暂停开新仓；有仓可继续风控平仓
	DailyLossLimitPct float64 `json:"daily_loss_limit_pct,omitempty"`
	// true：保证金预算按可用余额而非净值
	BudgetUseAvailableOnly bool `json:"budget_use_available_only,omitempty"`
	// 单层下单初始保证金下限（币安 20x XAU 约 0.46U）；按保证金校验，非名义 12U
	MinLayerMarginUSDT float64 `json:"min_layer_margin_usdt,omitempty"`
}

// IsComkunProgramMartingaleStrategy 使用 COMKUN-AI 程序化执行（无真实 LLM）。
func IsComkunProgramMartingaleStrategy(c *StrategyConfig) bool {
	return c != nil && strings.TrimSpace(c.StrategyType) == "program_martingale" && c.MartingaleProgram != nil
}

// GridStrategyConfig grid trading specific configuration
type GridStrategyConfig struct {
	// Trading pair (e.g., "BTCUSDT")
	Symbol string `json:"symbol"`
	// Number of grid levels (5-50)
	GridCount int `json:"grid_count"`
	// Total investment in USDT
	TotalInvestment float64 `json:"total_investment"`
	// Leverage (1-20)
	Leverage int `json:"leverage"`
	// Upper price boundary (0 = auto-calculate from ATR)
	UpperPrice float64 `json:"upper_price"`
	// Lower price boundary (0 = auto-calculate from ATR)
	LowerPrice float64 `json:"lower_price"`
	// Use ATR to auto-calculate bounds
	UseATRBounds bool `json:"use_atr_bounds"`
	// ATR multiplier for bound calculation (default 2.0)
	ATRMultiplier float64 `json:"atr_multiplier"`
	// Position distribution: "uniform" | "gaussian" | "pyramid"
	Distribution string `json:"distribution"`
	// Maximum drawdown percentage before emergency exit
	MaxDrawdownPct float64 `json:"max_drawdown_pct"`
	// Stop loss percentage per position
	StopLossPct float64 `json:"stop_loss_pct"`
	// Daily loss limit percentage
	DailyLossLimitPct float64 `json:"daily_loss_limit_pct"`
	// Use maker-only orders for lower fees
	UseMakerOnly bool `json:"use_maker_only"`
	// Enable automatic grid direction adjustment based on box breakouts
	EnableDirectionAdjust bool `json:"enable_direction_adjust"`
	// Direction bias ratio for long_bias/short_bias modes (default 0.7 = 70%/30%)
	DirectionBiasRatio float64 `json:"direction_bias_ratio"`
}

// PromptSectionsConfig legacy editable sections of System Prompt (superseded by StrategyPrompt)
type PromptSectionsConfig struct {
	// role definition (title + description)
	RoleDefinition string `json:"role_definition,omitempty"`
	// trading frequency awareness
	TradingFrequency string `json:"trading_frequency,omitempty"`
	// entry standards
	EntryStandards string `json:"entry_standards,omitempty"`
	// decision process
	DecisionProcess string `json:"decision_process,omitempty"`
}

// MergedStrategyNarrative returns one user-authored narrative: strategy_prompt if set,
// otherwise non-empty legacy prompt_sections joined in order.
func (c *StrategyConfig) MergedStrategyNarrative() string {
	if c == nil {
		return ""
	}
	s := strings.TrimSpace(c.StrategyPrompt)
	if s != "" {
		return s
	}
	ps := c.PromptSections
	parts := []string{
		strings.TrimSpace(ps.RoleDefinition),
		strings.TrimSpace(ps.TradingFrequency),
		strings.TrimSpace(ps.EntryStandards),
		strings.TrimSpace(ps.DecisionProcess),
	}
	var b strings.Builder
	first := true
	for _, p := range parts {
		if p == "" {
			continue
		}
		if !first {
			b.WriteString("\n\n")
		}
		first = false
		b.WriteString(p)
	}
	return b.String()
}

// CoinSourceConfig coin source configuration
type CoinSourceConfig struct {
	// source type: "static" | "ai500" | "oi_top" | "oi_low" | "mixed"
	SourceType string `json:"source_type"`
	// static coin list (used when source_type = "static")
	StaticCoins []string `json:"static_coins,omitempty"`
	// excluded coins list (filtered out from all sources)
	ExcludedCoins []string `json:"excluded_coins,omitempty"`
	// whether to use AI500 coin pool
	UseAI500 bool `json:"use_ai500"`
	// AI500 coin pool maximum count
	AI500Limit int `json:"ai500_limit,omitempty"`
	// whether to use OI Top (OI increase ranking, suitable for long positions)
	UseOITop bool `json:"use_oi_top"`
	// OI Top maximum count
	OITopLimit int `json:"oi_top_limit,omitempty"`
	// whether to use OI Low (OI decrease ranking, suitable for short positions)
	UseOILow bool `json:"use_oi_low"`
	// OI Low maximum count
	OILowLimit int `json:"oi_low_limit,omitempty"`
	// whether to use Hyperliquid All coins (all available perp pairs)
	UseHyperAll bool `json:"use_hyper_all"`
	// whether to use Hyperliquid Main coins (top N by 24h volume)
	UseHyperMain bool `json:"use_hyper_main"`
	// Hyperliquid Main maximum count (default 20)
	HyperMainLimit int `json:"hyper_main_limit,omitempty"`
	// Note: API URLs are now built automatically using NofxOSAPIKey from IndicatorConfig
}

// IndicatorConfig indicator configuration
type IndicatorConfig struct {
	// K-line configuration
	Klines KlineConfig `json:"klines"`
	// raw kline data (OHLCV) - always enabled, required for AI analysis
	EnableRawKlines bool `json:"enable_raw_klines"`
	// technical indicator switches
	EnableEMA         bool `json:"enable_ema"`
	EnableMACD        bool `json:"enable_macd"`
	EnableRSI         bool `json:"enable_rsi"`
	EnableATR         bool `json:"enable_atr"`
	EnableBOLL        bool `json:"enable_boll"` // Bollinger Bands
	EnableVolume      bool `json:"enable_volume"`
	EnableOI          bool `json:"enable_oi"`           // open interest
	EnableFundingRate bool `json:"enable_funding_rate"` // funding rate
	// EMA period configuration
	EMAPeriods []int `json:"ema_periods,omitempty"` // default [20, 50]
	// RSI period configuration
	RSIPeriods []int `json:"rsi_periods,omitempty"` // default [7, 14]
	// ATR period configuration
	ATRPeriods []int `json:"atr_periods,omitempty"` // default [14]
	// BOLL period configuration (period, standard deviation multiplier is fixed at 2)
	BOLLPeriods []int `json:"boll_periods,omitempty"` // default [20] - can select multiple timeframes
	// external data sources
	ExternalDataSources []ExternalDataSource `json:"external_data_sources,omitempty"`

	// ========== NofxOS Unified API Configuration ==========
	// Unified API Key for all NofxOS data sources
	NofxOSAPIKey string `json:"nofxos_api_key,omitempty"`

	// quantitative data sources (capital flow, position changes, price changes)
	EnableQuantData    bool `json:"enable_quant_data"`    // whether to enable quantitative data
	EnableQuantOI      bool `json:"enable_quant_oi"`      // whether to show OI data
	EnableQuantNetflow bool `json:"enable_quant_netflow"` // whether to show Netflow data

	// OI ranking data (market-wide open interest increase/decrease rankings)
	EnableOIRanking   bool   `json:"enable_oi_ranking"`             // whether to enable OI ranking data
	OIRankingDuration string `json:"oi_ranking_duration,omitempty"` // duration: 1h, 4h, 24h
	OIRankingLimit    int    `json:"oi_ranking_limit,omitempty"`    // number of entries (default 10)

	// NetFlow ranking data (market-wide fund flow rankings - institution/personal)
	EnableNetFlowRanking   bool   `json:"enable_netflow_ranking"`             // whether to enable NetFlow ranking data
	NetFlowRankingDuration string `json:"netflow_ranking_duration,omitempty"` // duration: 1h, 4h, 24h
	NetFlowRankingLimit    int    `json:"netflow_ranking_limit,omitempty"`    // number of entries (default 10)

	// Price ranking data (market-wide gainers/losers)
	EnablePriceRanking   bool   `json:"enable_price_ranking"`             // whether to enable price ranking data
	PriceRankingDuration string `json:"price_ranking_duration,omitempty"` // durations: "1h" or "1h,4h,24h"
	PriceRankingLimit    int    `json:"price_ranking_limit,omitempty"`    // number of entries per ranking (default 10)
}

// KlineConfig K-line configuration
type KlineConfig struct {
	// primary timeframe: "1m", "3m", "5m", "15m", "1h", "4h"
	PrimaryTimeframe string `json:"primary_timeframe"`
	// primary timeframe K-line count
	PrimaryCount int `json:"primary_count"`
	// longer timeframe
	LongerTimeframe string `json:"longer_timeframe,omitempty"`
	// longer timeframe K-line count
	LongerCount int `json:"longer_count,omitempty"`
	// whether to enable multi-timeframe analysis
	EnableMultiTimeframe bool `json:"enable_multi_timeframe"`
	// selected timeframe list (new: supports multi-timeframe selection)
	SelectedTimeframes []string `json:"selected_timeframes,omitempty"`
}

// ExternalDataSource external data source configuration
type ExternalDataSource struct {
	Name        string            `json:"name"`   // data source name
	Type        string            `json:"type"`   // type: "api" | "webhook"
	URL         string            `json:"url"`    // API URL
	Method      string            `json:"method"` // HTTP method
	Headers     map[string]string `json:"headers,omitempty"`
	DataPath    string            `json:"data_path,omitempty"`    // JSON data path
	RefreshSecs int               `json:"refresh_secs,omitempty"` // refresh interval (seconds)
}

// RiskControlConfig risk control configuration
type RiskControlConfig struct {
	// Max number of coins held simultaneously (CODE ENFORCED)
	MaxPositions int `json:"max_positions"`

	// BTC/ETH exchange leverage for opening positions (AI guided)
	BTCETHMaxLeverage int `json:"btc_eth_max_leverage"`
	// Altcoin exchange leverage for opening positions (AI guided)
	AltcoinMaxLeverage int `json:"altcoin_max_leverage"`

	// BTC/ETH single position max value = equity × this ratio (CODE ENFORCED, default: 5)
	BTCETHMaxPositionValueRatio float64 `json:"btc_eth_max_position_value_ratio"`
	// Altcoin single position max value = equity × this ratio (CODE ENFORCED, default: 1)
	AltcoinMaxPositionValueRatio float64 `json:"altcoin_max_position_value_ratio"`

	// Max margin utilization (e.g. 0.9 = 90%) (CODE ENFORCED)
	MaxMarginUsage float64 `json:"max_margin_usage"`
	// Min position size in USDT (CODE ENFORCED)
	MinPositionSize float64 `json:"min_position_size"`

	// Min take_profit / stop_loss ratio (AI guided)
	MinRiskRewardRatio float64 `json:"min_risk_reward_ratio"`
	// Min AI confidence to open position (AI guided)
	MinConfidence int `json:"min_confidence"`
}

// NewStrategyStore creates a new StrategyStore
func NewStrategyStore(db *gorm.DB) *StrategyStore {
	return &StrategyStore{db: db}
}

func (s *StrategyStore) initTables() error {
	if err := s.db.AutoMigrate(&Strategy{}); err != nil {
		return err
	}
	// 从旧字段回填 market_access（顺序重要：先处理上架，再把仍为空置为 off）
	_ = s.db.Exec(`
		UPDATE strategies SET market_access = ?
		WHERE (market_access IS NULL OR TRIM(market_access) = '' OR market_access = ?)
		  AND is_public = 1 AND config_visible = 1
	`, MarketAccessPublic, MarketAccessOff).Error
	_ = s.db.Exec(`
		UPDATE strategies SET market_access = ?
		WHERE (market_access IS NULL OR TRIM(market_access) = '' OR market_access = ?)
		  AND is_public = 1 AND config_visible = 0
	`, MarketAccessSubscription, MarketAccessOff).Error
	_ = s.db.Exec(`
		UPDATE strategies SET market_access = ?
		WHERE market_access IS NULL OR TRIM(market_access) = '' OR market_access = ?
	`, MarketAccessOff, MarketAccessOff).Error
	return nil
}

func (s *StrategyStore) initDefaultData() error {
	// No longer pre-populate strategies - create on demand when user configures
	return nil
}

// GetDefaultStrategyConfig returns the default strategy configuration for the given language
func GetDefaultStrategyConfig(lang string) StrategyConfig {
	// Normalize language to "zh" or "en"
	normalizedLang := "en"
	if lang == "zh" {
		normalizedLang = "zh"
	}

	config := StrategyConfig{
		Language: normalizedLang,
		CoinSource: CoinSourceConfig{
			SourceType: "ai500",
			UseAI500:   true,
			AI500Limit: 3,
			UseOITop:   false,
			OITopLimit: 3,
			UseOILow:   false,
			OILowLimit: 3,
		},
		Indicators: IndicatorConfig{
			Klines: KlineConfig{
				PrimaryTimeframe:     "5m",
				PrimaryCount:         20,
				LongerTimeframe:      "4h",
				LongerCount:          10,
				EnableMultiTimeframe: true,
				SelectedTimeframes:   []string{"5m", "15m", "1h"},
			},
			EnableRawKlines:   true, // Required - raw OHLCV data for AI analysis
			EnableEMA:         false,
			EnableMACD:        false,
			EnableRSI:         false,
			EnableATR:         false,
			EnableBOLL:        false,
			EnableVolume:      true,
			EnableOI:          true,
			EnableFundingRate: true,
			EMAPeriods:        []int{20, 50},
			RSIPeriods:        []int{7, 14},
			ATRPeriods:        []int{14},
			BOLLPeriods:       []int{20},
			// NofxOS key：公共 key 已废弃；平台模式默认走 claw402-data 网关，因此这里不再默认填充。
			NofxOSAPIKey: "",
			// Quant data
			EnableQuantData:    true,
			EnableQuantOI:      true,
			EnableQuantNetflow: true,
			// OI ranking data
			EnableOIRanking:   true,
			OIRankingDuration: "1h",
			OIRankingLimit:    10,
			// NetFlow ranking data
			EnableNetFlowRanking:   true,
			NetFlowRankingDuration: "1h",
			NetFlowRankingLimit:    10,
			// Price ranking data
			EnablePriceRanking:   true,
			PriceRankingDuration: "1h,4h,24h",
			PriceRankingLimit:    10,
		},
		RiskControl: RiskControlConfig{
			MaxPositions:                 3,   // Max 3 coins simultaneously (CODE ENFORCED)
			BTCETHMaxLeverage:            5,   // BTC/ETH exchange leverage (AI guided)
			AltcoinMaxLeverage:           5,   // Altcoin exchange leverage (AI guided)
			BTCETHMaxPositionValueRatio:  5.0, // BTC/ETH: max position = 5x equity (CODE ENFORCED)
			AltcoinMaxPositionValueRatio: 1.0, // Altcoin: max position = 1x equity (CODE ENFORCED)
			MaxMarginUsage:               0.9, // Max 90% margin usage (CODE ENFORCED)
			MinPositionSize:              12,  // Min 12 USDT per position (CODE ENFORCED)
			MinRiskRewardRatio:           3.0, // Min 3:1 profit/loss ratio (AI guided)
			MinConfidence:                75,  // Min 75% confidence (AI guided)
		},
		ComkunMirrorMasterMarginLeverage:   20,
		ComkunMirrorFollowerMarginLeverage: 20,
	}

	if lang == "zh" {
		config.StrategyPrompt = `# 你是一个专业的加密货币交易AI

你的任务是根据提供的市场数据做出交易决策。你是一个经验丰富的量化交易员，擅长技术分析和风险管理。

# ⏱️ 交易频率意识

- 优秀交易员：每天2-4笔 ≈ 每小时0.1-0.2笔
- 每小时超过2笔 = 过度交易
- 单笔持仓时间 ≥ 30-60分钟
如果你发现自己每个周期都在交易 → 标准太低；如果持仓不到30分钟就平仓 → 太冲动。

# 🎯 入场标准（严格）

只在多个信号共振时入场。自由使用任何有效的分析方法，避免单一指标、信号矛盾、横盘震荡、或平仓后立即重新开仓等低质量行为。

# 📋 决策流程

1. 检查持仓 → 是否止盈/止损
2. 扫描候选币种 + 多时间框架 → 是否存在强信号
3. 先写思维链，再输出结构化JSON`
	} else {
		config.StrategyPrompt = `# You are a professional cryptocurrency trading AI

Your task is to make trading decisions based on the provided market data. You are an experienced quantitative trader skilled in technical analysis and risk management.

# ⏱️ Trading Frequency Awareness

- Excellent trader: 2-4 trades per day ≈ 0.1-0.2 trades per hour
- >2 trades per hour = overtrading
- Single position holding time ≥ 30-60 minutes
If you find yourself trading every cycle → standards are too low; if closing positions in <30 minutes → too impulsive.

# 🎯 Entry Standards (Strict)

Only enter positions when multiple signals resonate. Freely use any effective analysis methods, avoid low-quality behaviors such as single indicators, contradictory signals, sideways oscillation, or immediately restarting after closing positions.

# 📋 Decision Process

1. Check positions → whether to take profit/stop loss
2. Scan candidate coins + multi-timeframe → whether strong signals exist
3. Write chain of thought first, then output structured JSON`
	}

	return config
}

// Create create a strategy
func (s *StrategyStore) Create(strategy *Strategy) error {
	return s.db.Create(strategy).Error
}

// Update update a strategy
func (s *StrategyStore) Update(strategy *Strategy) error {
	return s.db.Model(&Strategy{}).
		Where("id = ? AND user_id = ?", strategy.ID, strategy.UserID).
		Updates(map[string]interface{}{
			"name":                 strategy.Name,
			"description":          strategy.Description,
			"config":               strategy.Config,
			"is_public":            strategy.IsPublic,
			"config_visible":       strategy.ConfigVisible,
			"market_access":        strategy.MarketAccess,
			"show_after_rename":    strategy.ShowAfterRename,
			"source_strategy_id":   strategy.SourceStrategyID,
			"source_market_access": strategy.SourceMarketAccess,
			"content_locked":       strategy.ContentLocked,
			"updated_at":           time.Now().UTC(),
		}).Error
}

// UpdateAndIncrementMarketRevision 与 Update 相同，但额外将 market_revision +1（用于已上架策略的「市场更新」）
func (s *StrategyStore) UpdateAndIncrementMarketRevision(strategy *Strategy) error {
	return s.db.Model(&Strategy{}).
		Where("id = ? AND user_id = ?", strategy.ID, strategy.UserID).
		Updates(map[string]interface{}{
			"name":                 strategy.Name,
			"description":          strategy.Description,
			"config":               strategy.Config,
			"is_public":            strategy.IsPublic,
			"config_visible":       strategy.ConfigVisible,
			"market_access":        strategy.MarketAccess,
			"show_after_rename":    strategy.ShowAfterRename,
			"source_strategy_id":   strategy.SourceStrategyID,
			"source_market_access": strategy.SourceMarketAccess,
			"content_locked":       strategy.ContentLocked,
			"updated_at":           time.Now().UTC(),
			"market_revision":      gorm.Expr("COALESCE(market_revision, 0) + 1"),
		}).Error
}

// Delete delete a strategy
func (s *StrategyStore) Delete(userID, id string) error {
	// do not allow deleting system default strategy
	var st Strategy
	if err := s.db.Where("id = ?", id).First(&st).Error; err == nil {
		if st.IsDefault {
			return fmt.Errorf("cannot delete system default strategy")
		}
		if st.IsActive {
			return fmt.Errorf("cannot delete active strategy")
		}
	}

	// Check if any trader references this strategy
	var count int64
	if err := s.db.Model(&Trader{}).
		Where("user_id = ? AND strategy_id = ?", userID, id).
		Count(&count).Error; err == nil && count > 0 {
		return fmt.Errorf("cannot delete strategy in use by %d trader(s) - reassign those traders first", count)
	}

	return s.db.Where("id = ? AND user_id = ?", id, userID).Delete(&Strategy{}).Error
}

// List get user's strategy list
func (s *StrategyStore) List(userID string) ([]*Strategy, error) {
	var strategies []*Strategy
	err := s.db.Where("user_id = ? OR is_default = ?", userID, true).
		Order("is_default DESC, created_at DESC").
		Find(&strategies).Error
	if err != nil {
		return nil, err
	}
	return strategies, nil
}

// GetByIDForMarket 按 ID 加载「已上架策略市场」的策略（不限所属用户，用于购买校验）
func (s *StrategyStore) GetByIDForMarket(id string) (*Strategy, error) {
	var st Strategy
	if err := s.db.Where("id = ?", id).First(&st).Error; err != nil {
		return nil, err
	}
	if !IsVisibleOnPublicMarket(&st) {
		return nil, gorm.ErrRecordNotFound
	}
	return &st, nil
}

// GetByIDAny 按策略 ID 加载（仅服务内部使用，如 comkun 跟单读取源策略主控配置）。不校验调用者身份、不要求已上架市场。
func (s *StrategyStore) GetByIDAny(id string) (*Strategy, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var st Strategy
	if err := s.db.Where("id = ?", id).First(&st).Error; err != nil {
		return nil, err
	}
	return &st, nil
}

// ListPublic get all strategies listed on the strategy market
func (s *StrategyStore) ListPublic() ([]*Strategy, error) {
	var strategies []*Strategy
	err := s.db.Where(
		"market_access IN ? AND is_default = ?",
		[]string{
			MarketAccessPrivate,
			MarketAccessSubscription,
			MarketAccessPublic,
			MarketAccessOpenSource,
		},
		false,
	).Order("created_at DESC").
		Find(&strategies).Error
	if err != nil {
		return nil, err
	}
	return strategies, nil
}

// Get get a single strategy
func (s *StrategyStore) Get(userID, id string) (*Strategy, error) {
	var st Strategy
	err := s.db.Where("id = ? AND (user_id = ? OR is_default = ?)", id, userID, true).
		First(&st).Error
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// GetActive get user's currently active strategy
func (s *StrategyStore) GetActive(userID string) (*Strategy, error) {
	var st Strategy
	err := s.db.Where("user_id = ? AND is_active = ?", userID, true).First(&st).Error
	if err == gorm.ErrRecordNotFound {
		// no active strategy, return system default strategy
		return s.GetDefault()
	}
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// GetDefault get system default strategy
func (s *StrategyStore) GetDefault() (*Strategy, error) {
	var st Strategy
	err := s.db.Where("is_default = ?", true).First(&st).Error
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// SetActive set active strategy (will first deactivate other strategies)
func (s *StrategyStore) SetActive(userID, strategyID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// first deactivate all strategies for the user
		if err := tx.Model(&Strategy{}).Where("user_id = ?", userID).
			Update("is_active", false).Error; err != nil {
			return err
		}

		// activate specified strategy
		return tx.Model(&Strategy{}).
			Where("id = ? AND (user_id = ? OR is_default = ?)", strategyID, userID, true).
			Update("is_active", true).Error
	})
}

// ParseConfig parse strategy configuration JSON
func (s *Strategy) ParseConfig() (*StrategyConfig, error) {
	var config StrategyConfig
	if err := json.Unmarshal([]byte(s.Config), &config); err != nil {
		return nil, fmt.Errorf("failed to parse strategy configuration: %w", err)
	}
	return &config, nil
}

// SetConfig set strategy configuration
func (s *Strategy) SetConfig(config *StrategyConfig) error {
	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to serialize strategy configuration: %w", err)
	}
	s.Config = string(data)
	return nil
}

// ============================================================================
// Token Estimation
// ============================================================================

// TokenEstimate holds the result of token estimation
type TokenEstimate struct {
	Total       int            `json:"total"`
	Breakdown   TokenBreakdown `json:"breakdown"`
	ModelLimits []ModelLimit   `json:"model_limits"`
	Suggestions []string       `json:"suggestions"`
}

// TokenBreakdown shows estimated tokens per component
type TokenBreakdown struct {
	SystemPrompt  int `json:"system_prompt"`
	MarketData    int `json:"market_data"`
	RankingData   int `json:"ranking_data"`
	QuantData     int `json:"quant_data"`
	FixedOverhead int `json:"fixed_overhead"`
}

// ModelLimit shows token usage against a specific model's context limit
type ModelLimit struct {
	Name         string `json:"name"`
	ContextLimit int    `json:"context_limit"`
	UsagePct     int    `json:"usage_pct"`
	Level        string `json:"level"` // "ok" | "warning" | "danger"
}

// Context window sizes (tokens) for each model family
const (
	contextLimitDeepSeek = 131_072   // 128K
	contextLimitOpenAI   = 128_000   // 128K
	contextLimitClaude   = 200_000   // 200K
	contextLimitQwen     = 131_072   // 128K
	contextLimitGemini   = 1_000_000 // 1M
	contextLimitGrok     = 131_072   // 128K
	contextLimitKimi     = 131_072   // 128K
	contextLimitMinimax  = 1_000_000 // 1M
)

// ModelContextLimits maps provider names to their context window sizes (in tokens)
var ModelContextLimits = map[string]int{
	"deepseek": contextLimitDeepSeek,
	"openai":   contextLimitOpenAI,
	"claude":   contextLimitClaude,
	"qwen":     contextLimitQwen,
	"gemini":   contextLimitGemini,
	"grok":     contextLimitGrok,
	"kimi":     contextLimitKimi,
	"minimax":  contextLimitMinimax,
	// comkun_ai 为平台跟单占位通道，真实推理可走多路由；若此处填过小（如 8k），
	// 策略实验室会取全表最小值作进度条分母，导致「预估预算」恒满，与 AI_MAX_TOKENS 无关。
	"comkun_ai": contextLimitDeepSeek,
}

// GetContextLimit returns the context limit for a given provider
func GetContextLimit(provider string) int {
	if limit, ok := ModelContextLimits[provider]; ok {
		return limit
	}
	return contextLimitDeepSeek // safe default
}

// GetContextLimitForClient returns context limit for a provider+model pair.
// For claw402, the underlying model is inferred from the model name prefix.
func GetContextLimitForClient(provider, model string) int {
	if provider == "claw402" {
		switch {
		case strings.HasPrefix(model, "claude"):
			return ModelContextLimits["claude"]
		case strings.HasPrefix(model, "gpt"), strings.HasPrefix(model, "o1"), strings.HasPrefix(model, "o3"):
			return ModelContextLimits["openai"]
		case strings.HasPrefix(model, "gemini"):
			return ModelContextLimits["gemini"]
		case strings.HasPrefix(model, "grok"):
			return ModelContextLimits["grok"]
		case strings.HasPrefix(model, "kimi"):
			return ModelContextLimits["kimi"]
		case strings.HasPrefix(model, "qwen"):
			return ModelContextLimits["qwen"]
		case strings.HasPrefix(model, "minimax"):
			return ModelContextLimits["minimax"]
		case strings.HasPrefix(model, "deepseek"):
			return ModelContextLimits["deepseek"]
		default:
			return ModelContextLimits["deepseek"]
		}
	}
	return GetContextLimit(provider)
}

// EstimateTokens estimates the total token count for a strategy configuration.
// This is a pure computation based on config fields — no network calls.
func (c *StrategyConfig) EstimateTokens() TokenEstimate {
	breakdown := TokenBreakdown{}

	// --- System Prompt ---
	// Base system prompt: schema + role + rules + output format
	baseChars := 4000 // English default
	if c.Language == "zh" {
		baseChars = 3000
	}
	baseChars += len(c.MergedStrategyNarrative())
	baseChars += len(c.CustomPrompt)

	if c.Language == "zh" {
		breakdown.SystemPrompt = baseChars / 2 // CJK: ~2 chars per token
	} else {
		breakdown.SystemPrompt = baseChars / 4 // English: ~4 chars per token
	}

	// --- Fixed Overhead ---
	// Time, BTC price, account info, section headers
	breakdown.FixedOverhead = 800 / 4 // ~200 tokens

	// --- Market Data ---
	numCoins := c.getEffectiveCoinCount()
	numTimeframes := c.getEffectiveTimeframeCount()
	klineCount := c.Indicators.Klines.PrimaryCount
	if klineCount <= 0 {
		klineCount = 20
	}

	// Per coin per timeframe: kline OHLCV rows
	charsPerCoinTF := klineCount * 80 // each OHLCV line ~80 chars

	// Add enabled indicator overhead per timeframe
	indicatorCharsPerLine := 0
	if c.Indicators.EnableEMA {
		indicatorCharsPerLine += 20 // EMA values appended
	}
	if c.Indicators.EnableMACD {
		indicatorCharsPerLine += 30
	}
	if c.Indicators.EnableRSI {
		indicatorCharsPerLine += 15
	}
	if c.Indicators.EnableATR {
		indicatorCharsPerLine += 15
	}
	if c.Indicators.EnableBOLL {
		indicatorCharsPerLine += 25
	}
	if c.Indicators.EnableVolume {
		indicatorCharsPerLine += 10
	}
	charsPerCoinTF += klineCount * indicatorCharsPerLine

	totalMarketChars := numCoins * numTimeframes * charsPerCoinTF

	// OI + Funding per coin
	if c.Indicators.EnableOI || c.Indicators.EnableFundingRate {
		totalMarketChars += numCoins * 100
	}

	breakdown.MarketData = totalMarketChars / 4 // numeric data: ~4 chars per token

	// --- Quant Data ---
	if c.Indicators.EnableQuantData {
		quantCharsPerCoin := 0
		if c.Indicators.EnableQuantOI {
			quantCharsPerCoin += 300
		}
		if c.Indicators.EnableQuantNetflow {
			quantCharsPerCoin += 300
		}
		breakdown.QuantData = (numCoins * quantCharsPerCoin) / 4
	}

	// --- Ranking Data ---
	rankingChars := 0
	if c.Indicators.EnableOIRanking {
		limit := c.Indicators.OIRankingLimit
		if limit <= 0 {
			limit = 10
		}
		rankingChars += limit * 60
	}
	if c.Indicators.EnableNetFlowRanking {
		limit := c.Indicators.NetFlowRankingLimit
		if limit <= 0 {
			limit = 10
		}
		rankingChars += limit * 80
	}
	if c.Indicators.EnablePriceRanking {
		limit := c.Indicators.PriceRankingLimit
		if limit <= 0 {
			limit = 10
		}
		// Count durations (comma-separated)
		numDurations := 1
		if c.Indicators.PriceRankingDuration != "" {
			numDurations = len(strings.Split(c.Indicators.PriceRankingDuration, ","))
		}
		rankingChars += limit * numDurations * 40
	}
	breakdown.RankingData = rankingChars / 4

	// --- Total with 15% safety margin ---
	subtotal := breakdown.SystemPrompt + breakdown.MarketData + breakdown.RankingData + breakdown.QuantData + breakdown.FixedOverhead
	total := subtotal * 115 / 100

	// --- Model limits ---
	modelLimits := make([]ModelLimit, 0, len(ModelContextLimits))
	for name, limit := range ModelContextLimits {
		pct := total * 100 / limit
		level := "ok"
		if pct >= 100 {
			level = "danger"
		} else if pct >= 80 {
			level = "warning"
		}
		modelLimits = append(modelLimits, ModelLimit{
			Name:         name,
			ContextLimit: limit,
			UsagePct:     pct,
			Level:        level,
		})
	}

	// Sort by usage_pct desc, then name asc for deterministic order
	sort.Slice(modelLimits, func(i, j int) bool {
		if modelLimits[i].UsagePct != modelLimits[j].UsagePct {
			return modelLimits[i].UsagePct > modelLimits[j].UsagePct
		}
		return modelLimits[i].Name < modelLimits[j].Name
	})

	// --- Suggestions ---
	var suggestions []string
	// Find the strictest model (smallest context)
	minLimit := 0
	for _, limit := range ModelContextLimits {
		if minLimit == 0 || limit < minLimit {
			minLimit = limit
		}
	}
	if minLimit > 0 && total > minLimit {
		if numTimeframes > 1 {
			savedPerTF := (numCoins * klineCount * (80 + indicatorCharsPerLine)) / 4 * 115 / 100
			suggestions = append(suggestions, fmt.Sprintf("Reduce 1 timeframe to save ~%d tokens", savedPerTF))
		}
		if numCoins > 1 {
			savedPerCoin := (numTimeframes * klineCount * (80 + indicatorCharsPerLine)) / 4 * 115 / 100
			suggestions = append(suggestions, fmt.Sprintf("Reduce 1 coin to save ~%d tokens", savedPerCoin))
		}
		if klineCount > 15 {
			suggestions = append(suggestions, "Reduce K-line count to 15 to save tokens")
		}
	}

	return TokenEstimate{
		Total:       total,
		Breakdown:   breakdown,
		ModelLimits: modelLimits,
		Suggestions: suggestions,
	}
}

// getEffectiveCoinCount returns the estimated number of coins that will be analyzed
func (c *StrategyConfig) getEffectiveCoinCount() int {
	count := 0
	switch c.CoinSource.SourceType {
	case "static":
		count = len(c.CoinSource.StaticCoins)
	case "ai500":
		count = c.CoinSource.AI500Limit
	case "oi_top":
		count = c.CoinSource.OITopLimit
	case "oi_low":
		count = c.CoinSource.OILowLimit
	case "mixed":
		if c.CoinSource.UseAI500 {
			count += c.CoinSource.AI500Limit
		}
		if c.CoinSource.UseOITop {
			count += c.CoinSource.OITopLimit
		}
		if c.CoinSource.UseOILow {
			count += c.CoinSource.OILowLimit
		}
	default:
		count = c.CoinSource.AI500Limit
	}
	if count <= 0 {
		count = 3
	}
	return count
}

// getEffectiveTimeframeCount returns the number of timeframes that will be used
func (c *StrategyConfig) getEffectiveTimeframeCount() int {
	if len(c.Indicators.Klines.SelectedTimeframes) > 0 {
		return len(c.Indicators.Klines.SelectedTimeframes)
	}
	count := 1
	if c.Indicators.Klines.LongerTimeframe != "" {
		count++
	}
	return count
}
