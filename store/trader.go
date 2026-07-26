package store

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// TraderStore trader storage
type TraderStore struct {
	db *gorm.DB
}

// NewTraderStore creates a new trader store
func NewTraderStore(db *gorm.DB) *TraderStore {
	return &TraderStore{db: db}
}

// Trader trader configuration
type Trader struct {
	ID                  string    `gorm:"primaryKey" json:"id"`
	UserID              string    `gorm:"column:user_id;not null;default:default;index" json:"user_id"`
	Name                string    `gorm:"column:name;not null" json:"name"`
	AIModelID           string    `gorm:"column:ai_model_id;not null" json:"ai_model_id"`
	ExchangeID          string    `gorm:"column:exchange_id;not null" json:"exchange_id"`
	StrategyID          string    `gorm:"column:strategy_id;default:''" json:"strategy_id"`
	InitialBalance      float64   `gorm:"column:initial_balance;not null" json:"initial_balance"`
	ScanIntervalMinutes int       `gorm:"column:scan_interval_minutes;default:3" json:"scan_interval_minutes"`
	IsRunning           bool      `gorm:"column:is_running;default:false" json:"is_running"`
	IsCrossMargin       bool      `gorm:"column:is_cross_margin;default:true" json:"is_cross_margin"`
	ShowInCompetition   bool      `gorm:"column:show_in_competition;default:true" json:"show_in_competition"`
	CreatedAt           time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt           time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`

	// Following fields are deprecated, kept for backward compatibility, new traders should use StrategyID
	BTCETHLeverage       int    `gorm:"column:btc_eth_leverage;default:5" json:"btc_eth_leverage,omitempty"`
	AltcoinLeverage      int    `gorm:"column:altcoin_leverage;default:5" json:"altcoin_leverage,omitempty"`
	TradingSymbols       string `gorm:"column:trading_symbols;default:''" json:"trading_symbols,omitempty"`
	UseAI500             bool   `gorm:"column:use_coin_pool;default:false" json:"use_ai500,omitempty"`
	UseOITop             bool   `gorm:"column:use_oi_top;default:false" json:"use_oi_top,omitempty"`
	CustomPrompt         string `gorm:"column:custom_prompt;default:''" json:"custom_prompt,omitempty"`
	OverrideBasePrompt   bool   `gorm:"column:override_base_prompt;default:false" json:"override_base_prompt,omitempty"`
	SystemPromptTemplate string `gorm:"column:system_prompt_template;default:default" json:"system_prompt_template,omitempty"`
}

// TableName returns the table name for Trader
func (Trader) TableName() string {
	return "traders"
}

// TraderFullConfig trader full configuration (includes AI model, exchange and strategy)
type TraderFullConfig struct {
	Trader   *Trader
	AIModel  *AIModel
	Exchange *Exchange
	Strategy *Strategy
}

// RunningStrategyRef 是策略市场统计用的轻量引用：
// 当前正在运行的交易员绑定了哪个策略，以及该策略配置里是否指向某个市场源策略。
type RunningStrategyRef struct {
	StrategyID string
	Config     string
}

// MarketStrategyTraderRef 策略市场统计用：所有交易员及其绑定策略配置。
type MarketStrategyTraderRef struct {
	TraderID       string
	StrategyID     string
	Config         string
	InitialBalance float64
	IsRunning      bool
	CreatedAt      time.Time
	ExchangeType   string `gorm:"column:exchange_type"`
}

type ComkunFollowerTraderRef struct {
	TraderID     string
	TraderName   string
	UserID       string
	StrategyID   string
	StrategyName string
	IsRunning    bool
	ExchangeID   string
	ExchangeType string
}

func (s *TraderStore) initTables() error {
	// For PostgreSQL with existing table, skip AutoMigrate
	if s.db.Dialector.Name() == "postgres" {
		var tableExists int64
		s.db.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'traders'`).Scan(&tableExists)
		if tableExists > 0 {
			return nil
		}
	}
	// Use GORM AutoMigrate
	if err := s.db.AutoMigrate(&Trader{}); err != nil {
		return fmt.Errorf("failed to migrate traders table: %w", err)
	}
	return nil
}

// CountTradersLinkedToMarketSource 同一主站用户下，策略副本的 source_strategy_id 指向同一市场源 id 的交易员数量（可排除某 trader_id）。
// 与 Billing.HasEntitlement 配合：已购某市场源的用户，该源下仅能有一个交易员绑定副本策略。
func (s *TraderStore) CountTradersLinkedToMarketSource(userID, marketSourceStrategyID, excludeTraderID string) (int64, error) {
	uid := strings.TrimSpace(userID)
	ms := strings.TrimSpace(marketSourceStrategyID)
	if uid == "" || ms == "" {
		return 0, nil
	}
	q := s.db.Table("traders AS t").
		Joins("INNER JOIN strategies AS s ON s.id = t.strategy_id AND s.user_id = t.user_id").
		Where("t.user_id = ? AND TRIM(COALESCE(s.source_strategy_id, '')) = ?", uid, ms)
	if x := strings.TrimSpace(excludeTraderID); x != "" {
		q = q.Where("t.id <> ?", x)
	}
	var n int64
	err := q.Count(&n).Error
	return n, err
}

// Create creates trader
func (s *TraderStore) Create(trader *Trader) error {
	return s.db.Create(trader).Error
}

// List gets user's trader list
func (s *TraderStore) List(userID string) ([]*Trader, error) {
	var traders []*Trader
	err := s.db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&traders).Error
	if err != nil {
		return nil, err
	}
	return traders, nil
}

// UpdateStatus updates trader running status
func (s *TraderStore) UpdateStatus(userID, id string, isRunning bool) error {
	return s.db.Model(&Trader{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("is_running", isRunning).Error
}

// UpdateShowInCompetition updates trader competition visibility
func (s *TraderStore) UpdateShowInCompetition(userID, id string, showInCompetition bool) error {
	return s.db.Model(&Trader{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("show_in_competition", showInCompetition).Error
}

// Update updates trader configuration
func (s *TraderStore) Update(trader *Trader) error {
	fmt.Printf("📝 TraderStore.Update: ID=%s, Name=%s, AIModelID=%s, StrategyID=%s\n",
		trader.ID, trader.Name, trader.AIModelID, trader.StrategyID)

	updates := map[string]interface{}{
		"name":                trader.Name,
		"ai_model_id":         trader.AIModelID,
		"exchange_id":         trader.ExchangeID,
		"strategy_id":         trader.StrategyID,
		"is_cross_margin":     trader.IsCrossMargin,
		"show_in_competition": trader.ShowInCompetition,
	}

	// Only update these if > 0
	if trader.InitialBalance > 0 {
		updates["initial_balance"] = trader.InitialBalance
	}
	if trader.ScanIntervalMinutes > 0 {
		updates["scan_interval_minutes"] = trader.ScanIntervalMinutes
		fmt.Printf("📊 TraderStore.Update: scan_interval_minutes=%d will be saved\n", trader.ScanIntervalMinutes)
	} else {
		fmt.Printf("⚠️ TraderStore.Update: scan_interval_minutes=%d (<=0, NOT updating)\n", trader.ScanIntervalMinutes)
	}

	return s.db.Model(&Trader{}).
		Where("id = ? AND user_id = ?", trader.ID, trader.UserID).
		Updates(updates).Error
}

// UpdateInitialBalance updates initial balance
func (s *TraderStore) UpdateInitialBalance(userID, id string, newBalance float64) error {
	return s.db.Model(&Trader{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("initial_balance", newBalance).Error
}

// UpdateCustomPrompt updates custom prompt
func (s *TraderStore) UpdateCustomPrompt(userID, id string, customPrompt string, overrideBase bool) error {
	return s.db.Model(&Trader{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(map[string]interface{}{
			"custom_prompt":        customPrompt,
			"override_base_prompt": overrideBase,
		}).Error
}

// Delete deletes trader and associated data
func (s *TraderStore) Delete(userID, id string) error {
	// Delete associated equity snapshots first
	s.db.Where("trader_id = ?", id).Delete(&EquitySnapshot{})

	// Delete the trader
	return s.db.Where("id = ? AND user_id = ?", id, userID).Delete(&Trader{}).Error
}

// GetFullConfig gets trader full configuration
func (s *TraderStore) GetFullConfig(userID, traderID string) (*TraderFullConfig, error) {
	var trader Trader
	err := s.db.Where("id = ? AND user_id = ?", traderID, userID).First(&trader).Error
	if err != nil {
		return nil, err
	}

	// Get AI model
	var aiModel AIModel
	err = s.db.Where("id = ? AND user_id = ?", trader.AIModelID, userID).First(&aiModel).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get AI model: %w", err)
	}

	// Get exchange
	var exchange Exchange
	err = s.db.Where("id = ? AND user_id = ?", trader.ExchangeID, userID).First(&exchange).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get exchange: %w", err)
	}

	// Load associated strategy
	var strategy *Strategy
	if trader.StrategyID != "" {
		strategy, _ = s.getStrategyByID(userID, trader.StrategyID)
	}
	// If no associated strategy, get user's active strategy or default strategy
	if strategy == nil {
		strategy, _ = s.getActiveOrDefaultStrategy(userID)
	}

	return &TraderFullConfig{
		Trader:   &trader,
		AIModel:  &aiModel,
		Exchange: &exchange,
		Strategy: strategy,
	}, nil
}

// getStrategyByID internal method: gets strategy by ID
func (s *TraderStore) getStrategyByID(userID, strategyID string) (*Strategy, error) {
	var strategy Strategy
	err := s.db.Where("id = ? AND (user_id = ? OR is_default = ?)", strategyID, userID, true).
		First(&strategy).Error
	if err != nil {
		return nil, err
	}
	return &strategy, nil
}

// getActiveOrDefaultStrategy internal method: gets user's active strategy or system default strategy
func (s *TraderStore) getActiveOrDefaultStrategy(userID string) (*Strategy, error) {
	var strategy Strategy

	// First try to get user's active strategy
	err := s.db.Where("user_id = ? AND is_active = ?", userID, true).First(&strategy).Error
	if err == nil {
		return &strategy, nil
	}

	// Fallback to system default strategy
	err = s.db.Where("is_default = ?", true).First(&strategy).Error
	if err != nil {
		return nil, err
	}
	return &strategy, nil
}

// GetByID gets a trader by ID without requiring userID (for public APIs)
func (s *TraderStore) GetByID(traderID string) (*Trader, error) {
	var trader Trader
	err := s.db.Where("id = ?", traderID).First(&trader).Error
	if err != nil {
		return nil, err
	}
	return &trader, nil
}

// ListAll gets all traders
func (s *TraderStore) ListAll() ([]*Trader, error) {
	var traders []*Trader
	err := s.db.Order("created_at DESC").Find(&traders).Error
	if err != nil {
		return nil, err
	}
	return traders, nil
}

// ListByExchangeID gets traders that use a specific exchange
func (s *TraderStore) ListByExchangeID(userID, exchangeID string) ([]*Trader, error) {
	var traders []*Trader
	err := s.db.Where("user_id = ? AND exchange_id = ?", userID, exchangeID).Find(&traders).Error
	if err != nil {
		return nil, err
	}
	return traders, nil
}

// ListByAIModelID gets traders that use a specific AI model
func (s *TraderStore) ListByAIModelID(userID, aiModelID string) ([]*Trader, error) {
	var traders []*Trader
	err := s.db.Where("user_id = ? AND ai_model_id = ?", userID, aiModelID).Find(&traders).Error
	if err != nil {
		return nil, err
	}
	return traders, nil
}

func (s *TraderStore) ListRunningStrategyRefs() ([]RunningStrategyRef, error) {
	var refs []RunningStrategyRef
	err := s.db.Table("traders AS t").
		Select("t.strategy_id AS strategy_id, COALESCE(s.config, '') AS config").
		Joins("LEFT JOIN strategies AS s ON s.id = t.strategy_id").
		Where("t.is_running = ?", true).
		Scan(&refs).Error
	return refs, err
}

func (s *TraderStore) ListMarketStrategyTraderRefs() ([]MarketStrategyTraderRef, error) {
	var refs []MarketStrategyTraderRef
	err := s.db.Table("traders AS t").
		Select(`t.id AS trader_id, t.strategy_id AS strategy_id, COALESCE(s.config, '') AS config,
			t.initial_balance AS initial_balance, t.is_running AS is_running, t.created_at AS created_at,
			COALESCE(e.exchange_type, '') AS exchange_type`).
		Joins("LEFT JOIN strategies AS s ON s.id = t.strategy_id").
		Joins("LEFT JOIN exchanges AS e ON e.id = t.exchange_id").
		Scan(&refs).Error
	return refs, err
}

func (s *TraderStore) ListComkunFollowersBySourceStrategyID(sourceID string) ([]ComkunFollowerTraderRef, error) {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" {
		return nil, nil
	}
	var rows []struct {
		TraderID     string
		TraderName   string
		UserID       string
		StrategyID   string
		StrategyName string
		IsRunning    bool
		ExchangeID   string
		ExchangeType string
		Config       string
	}
	configNeedle := `%"comkun_market_source_strategy_id":"` + sourceID + `"%`
	if err := s.db.Table("traders AS t").
		Select(`t.id AS trader_id, t.name AS trader_name, t.user_id, t.strategy_id, t.is_running, t.exchange_id, COALESCE(e.type, '') AS exchange_type, COALESCE(s.config, '') AS config, COALESCE(s.name, '') AS strategy_name`).
		Joins("LEFT JOIN strategies AS s ON s.id = t.strategy_id").
		Joins("LEFT JOIN exchanges AS e ON e.id = t.exchange_id").
		Where("s.config LIKE ? AND s.config LIKE ?", configNeedle, `%"comkun_market_follow":true%`).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ComkunFollowerTraderRef, 0)
	for _, row := range rows {
		var cfg StrategyConfig
		if strings.TrimSpace(row.Config) == "" || json.Unmarshal([]byte(row.Config), &cfg) != nil {
			continue
		}
		if !cfg.ComkunMarketFollow || strings.TrimSpace(cfg.ComkunMarketSourceStrategyID) != sourceID {
			continue
		}
		out = append(out, ComkunFollowerTraderRef{
			TraderID:     row.TraderID,
			TraderName:   row.TraderName,
			UserID:       row.UserID,
			StrategyID:   row.StrategyID,
			StrategyName: strings.TrimSpace(row.StrategyName),
			IsRunning:    row.IsRunning,
			ExchangeID:   row.ExchangeID,
			ExchangeType: row.ExchangeType,
		})
	}
	return out, nil
}

// ListAllComkunMarketFollowTraderRefs 枚举策略为「市场合规跟单」（被控端）的全部交易员，用于管理员一键平仓等。
func (s *TraderStore) ListAllComkunMarketFollowTraderRefs() ([]ComkunFollowerTraderRef, error) {
	var rows []struct {
		TraderID     string
		TraderName   string
		UserID       string
		StrategyID   string
		StrategyName string
		IsRunning    bool
		ExchangeID   string
		ExchangeType string
		Config       string
	}
	if err := s.db.Table("traders AS t").
		Select(`t.id AS trader_id, t.name AS trader_name, t.user_id, t.strategy_id, t.is_running, t.exchange_id, COALESCE(e.type, '') AS exchange_type, COALESCE(s.config, '') AS config, COALESCE(s.name, '') AS strategy_name`).
		Joins("LEFT JOIN strategies AS s ON s.id = t.strategy_id").
		Joins("LEFT JOIN exchanges AS e ON e.id = t.exchange_id").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ComkunFollowerTraderRef, 0)
	for _, row := range rows {
		var cfg StrategyConfig
		if strings.TrimSpace(row.Config) == "" || json.Unmarshal([]byte(row.Config), &cfg) != nil {
			continue
		}
		if !IsComkunMarketFollowStrategy(&cfg) {
			continue
		}
		out = append(out, ComkunFollowerTraderRef{
			TraderID:     row.TraderID,
			TraderName:   row.TraderName,
			UserID:       row.UserID,
			StrategyID:   row.StrategyID,
			StrategyName: strings.TrimSpace(row.StrategyName),
			IsRunning:    row.IsRunning,
			ExchangeID:   row.ExchangeID,
			ExchangeType: row.ExchangeType,
		})
	}
	return out, nil
}
