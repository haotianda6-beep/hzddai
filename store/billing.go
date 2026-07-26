package store

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WalletLedger 平台余额流水（演示充值 / 策略市场扣款 / 管理员调账）。
// reason=recharge 为站内充值；策略市场周卡体验为 market_subscription_weekly_trial（不参与消费返佣）。
type WalletLedger struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID        string    `gorm:"column:user_id;not null;index:idx_wallet_ledger_user" json:"user_id"`
	Delta         float64   `gorm:"column:delta;not null" json:"delta"`
	BalanceAfter  float64   `gorm:"column:balance_after;not null" json:"balance_after"`
	Reason        string    `gorm:"column:reason;not null;default:''" json:"reason"`
	RefStrategyID string    `gorm:"column:ref_strategy_id;not null;default:''" json:"ref_strategy_id"`
	CreatedAt     time.Time `json:"created_at"`
}

func (WalletLedger) TableName() string { return "wallet_ledgers" }

// StrategyMarketEntitlement 用户已购买的策略市场策略（解锁配置）
type StrategyMarketEntitlement struct {
	UserID     string    `gorm:"column:user_id;primaryKey" json:"user_id"`
	StrategyID string    `gorm:"column:strategy_id;primaryKey" json:"strategy_id"`
	AmountPaid float64   `gorm:"column:amount_paid;not null;default:0" json:"amount_paid"`
	CreatedAt  time.Time `json:"created_at"`
	// SubscriptionUntil 包月/包周有效期内豁免 COMKUN 跟单「同步展示」按轮扣费；过期后仍保留解锁，恢复按轮扣站内余额
	SubscriptionUntil *time.Time `gorm:"column:subscription_until" json:"subscription_until,omitempty"`
}

func (StrategyMarketEntitlement) TableName() string { return "strategy_market_entitlements" }

// BillingStore 钱包流水与策略市场购买记录
type BillingStore struct {
	db *gorm.DB
}

func NewBillingStore(db *gorm.DB) *BillingStore {
	return &BillingStore{db: db}
}

func (s *BillingStore) initTables() error {
	return s.db.AutoMigrate(&WalletLedger{}, &StrategyMarketEntitlement{})
}

func (s *BillingStore) AppendLedger(tx *gorm.DB, userID string, delta, balanceAfter float64, reason, refStrategyID string) (uint64, error) {
	db := tx
	if db == nil {
		db = s.db
	}
	row := &WalletLedger{
		UserID:        userID,
		Delta:         delta,
		BalanceAfter:  balanceAfter,
		Reason:        reason,
		RefStrategyID: refStrategyID,
	}
	if err := db.Create(row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (s *BillingStore) ListLedger(userID string, limit int) ([]WalletLedger, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []WalletLedger
	err := s.db.Where("user_id = ?", userID).Order("id DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

func (s *BillingStore) HasEntitlement(userID, strategyID string) (bool, error) {
	var n int64
	err := s.db.Model(&StrategyMarketEntitlement{}).
		Where("user_id = ? AND strategy_id = ?", userID, strategyID).
		Count(&n).Error
	return n > 0, err
}

func (s *BillingStore) AddEntitlement(tx *gorm.DB, userID, strategyID string, amountPaid float64) error {
	db := tx
	if db == nil {
		db = s.db
	}
	return db.Create(&StrategyMarketEntitlement{
		UserID:     userID,
		StrategyID: strategyID,
		AmountPaid: amountPaid,
	}).Error
}

// GrantEntitlementIfMissing 解锁策略但不设置 subscription_until。
// 用于 0 元订阅 / 按主控广播次数扣费的 COMKUN 跟单策略：只解锁复制权限，不产生包月豁免。
func (s *BillingStore) GrantEntitlementIfMissing(tx *gorm.DB, userID, strategyID string, amountPaid float64) error {
	db := tx
	if db == nil {
		db = s.db
	}
	userID = strings.TrimSpace(userID)
	strategyID = strings.TrimSpace(strategyID)
	if userID == "" || strategyID == "" {
		return nil
	}
	row := &StrategyMarketEntitlement{
		UserID:     userID,
		StrategyID: strategyID,
		AmountPaid: amountPaid,
		CreatedAt:  time.Now().UTC(),
	}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "strategy_id"}},
		DoNothing: true,
	}).Create(row).Error
}

// HasScanFeeWaiver 策略市场源策略是否在包月/包周有效期内（豁免跟单轮询站内扣费）
func (s *BillingStore) HasScanFeeWaiver(userID, strategyID string) (bool, error) {
	strategyID = strings.TrimSpace(strategyID)
	userID = strings.TrimSpace(userID)
	if strategyID == "" || userID == "" {
		return false, nil
	}
	var row StrategyMarketEntitlement
	err := s.db.Where("user_id = ? AND strategy_id = ?", userID, strategyID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if row.SubscriptionUntil == nil {
		return false, nil
	}
	return row.SubscriptionUntil.After(time.Now().UTC()), nil
}

// ExtendMarketSubscription 扣款成功后调用：新购或续费延长 subscription_until（从当前到期日与「现在」中取较晚者为起点）
func (s *BillingStore) ExtendMarketSubscription(tx *gorm.DB, userID, strategyID string, addPaid float64, extend time.Duration) error {
	db := tx
	if db == nil {
		db = s.db
	}
	userID = strings.TrimSpace(userID)
	strategyID = strings.TrimSpace(strategyID)
	base := time.Now().UTC()
	var row StrategyMarketEntitlement
	err := db.Where("user_id = ? AND strategy_id = ?", userID, strategyID).First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil && row.SubscriptionUntil != nil && row.SubscriptionUntil.After(base) {
		base = row.SubscriptionUntil.UTC()
	}
	until := base.Add(extend)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&StrategyMarketEntitlement{
			UserID:            userID,
			StrategyID:        strategyID,
			AmountPaid:        addPaid,
			CreatedAt:         time.Now().UTC(),
			SubscriptionUntil: &until,
		}).Error
	}
	newPaid := row.AmountPaid + addPaid
	return db.Model(&StrategyMarketEntitlement{}).
		Where("user_id = ? AND strategy_id = ?", userID, strategyID).
		Updates(map[string]interface{}{
			"subscription_until": until,
			"amount_paid":        newPaid,
		}).Error
}

func (s *BillingStore) ListEntitlements(userID string) ([]StrategyMarketEntitlement, error) {
	var rows []StrategyMarketEntitlement
	err := s.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&rows).Error
	return rows, err
}

func (s *BillingStore) GetEntitlement(userID, strategyID string) (*StrategyMarketEntitlement, error) {
	var row StrategyMarketEntitlement
	err := s.db.Where("user_id = ? AND strategy_id = ?", userID, strategyID).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// ComkunSourceSubscriptionExpired 用户对该源策略有过包月/包周且 subscription_until 已过期。
// 跟单侧：订阅期内豁免平台余额；到期后不再自动按轮从余额扣费，而是停止交易员（续订后可再启动）。
func (s *BillingStore) ComkunSourceSubscriptionExpired(userID, sourceStrategyID string) (bool, error) {
	userID = strings.TrimSpace(userID)
	sourceStrategyID = strings.TrimSpace(sourceStrategyID)
	if userID == "" || sourceStrategyID == "" {
		return false, nil
	}
	row, err := s.GetEntitlement(userID, sourceStrategyID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if row.SubscriptionUntil == nil {
		return false, nil
	}
	return !row.SubscriptionUntil.UTC().After(time.Now().UTC()), nil
}

func (s *BillingStore) CountEntitlementsByStrategyIDs(strategyIDs []string) (map[string]int, error) {
	out := make(map[string]int)
	clean := make([]string, 0, len(strategyIDs))
	seen := map[string]bool{}
	for _, id := range strategyIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		clean = append(clean, id)
	}
	if len(clean) == 0 {
		return out, nil
	}
	var rows []struct {
		StrategyID string
		Count      int
	}
	err := s.db.Model(&StrategyMarketEntitlement{}).
		Select("strategy_id, COUNT(*) AS count").
		Where("strategy_id IN ?", clean).
		Group("strategy_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.StrategyID] = row.Count
	}
	return out, nil
}

// SumRechargeByUserIDs 统计每位用户 wallet_ledgers 中 reason=recharge 的 delta 合计（主站演示充值）。
func (s *BillingStore) SumRechargeByUserIDs(userIDs []string) (map[string]float64, error) {
	out := make(map[string]float64)
	clean := make([]string, 0, len(userIDs))
	seen := map[string]bool{}
	for _, id := range userIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		clean = append(clean, id)
	}
	if len(clean) == 0 {
		return out, nil
	}
	var rows []struct {
		UserID string  `gorm:"column:user_id"`
		Total  float64 `gorm:"column:total"`
	}
	err := s.db.Model(&WalletLedger{}).
		Select("user_id, COALESCE(SUM(delta), 0) AS total").
		Where("user_id IN ? AND reason = ?", clean, "recharge").
		Group("user_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.UserID] = row.Total
	}
	return out, nil
}

// SumRebateEligibleSpendByUserIDs 统计每位用户「可参与返佣的消费」：白名单 reason 的支出（-delta）之和。
func (s *BillingStore) SumRebateEligibleSpendByUserIDs(userIDs []string) (map[string]float64, error) {
	out := make(map[string]float64)
	clean := make([]string, 0, len(userIDs))
	seen := map[string]bool{}
	for _, id := range userIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		clean = append(clean, id)
	}
	if len(clean) == 0 {
		return out, nil
	}
	reasons := make([]string, 0, len(agentRebateSpendReasons))
	for r := range agentRebateSpendReasons {
		reasons = append(reasons, r)
	}
	var rows []struct {
		UserID string  `gorm:"column:user_id"`
		Total  float64 `gorm:"column:total"`
	}
	err := s.db.Model(&WalletLedger{}).
		Select("user_id, COALESCE(SUM(-delta), 0) AS total").
		Where("user_id IN ? AND delta < 0 AND reason IN ?", clean, reasons).
		Group("user_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.Total > 0 {
			out[row.UserID] = row.Total
		}
	}
	return out, nil
}
