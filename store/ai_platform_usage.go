package store

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	AIPlatformUsageStatusPending  = "pending"
	AIPlatformUsageStatusSuccess  = "success"
	AIPlatformUsageStatusRefunded = "refunded"
	AIPlatformUsageStatusFailed   = "failed"
)

// AIPlatformUsageLedger 平台统一 AI 调用账单：真实 claw402 成本、平台扣费、用户余额变化。
type AIPlatformUsageLedger struct {
	ID                  string    `gorm:"primaryKey" json:"id"`
	UserID              string    `gorm:"column:user_id;not null;index:idx_ai_platform_usage_user" json:"user_id"`
	TraderID            string    `gorm:"column:trader_id;not null;index:idx_ai_platform_usage_trader" json:"trader_id"`
	Provider            string    `gorm:"column:provider;not null;default:''" json:"provider"`
	Model               string    `gorm:"column:model;not null;default:''" json:"model"`
	ActualCostUSDC      float64   `gorm:"column:actual_cost_usdc;not null;default:0" json:"actual_cost_usdc"`
	ChargedUSDT         float64   `gorm:"column:charged_usdt;not null;default:0" json:"charged_usdt"`
	MarkupMultiplier    float64   `gorm:"column:markup_multiplier;not null;default:0" json:"markup_multiplier"`
	MinChargeUSDT       float64   `gorm:"column:min_charge_usdt;not null;default:0" json:"min_charge_usdt"`
	WalletBalanceBefore float64   `gorm:"column:wallet_balance_before;not null;default:0" json:"wallet_balance_before"`
	WalletBalanceAfter  float64   `gorm:"column:wallet_balance_after;not null;default:0" json:"wallet_balance_after"`
	Status              string    `gorm:"column:status;not null;default:'pending';index:idx_ai_platform_usage_status" json:"status"`
	PaymentTxHash       string    `gorm:"column:payment_tx_hash;not null;default:''" json:"payment_tx_hash"`
	ErrorMessage        string    `gorm:"column:error_message;not null;default:''" json:"error_message"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (AIPlatformUsageLedger) TableName() string { return "ai_platform_usage_ledger" }

type AIPlatformUsageStore struct {
	db *gorm.DB
}

func NewAIPlatformUsageStore(db *gorm.DB) *AIPlatformUsageStore {
	return &AIPlatformUsageStore{db: db}
}

func (s *AIPlatformUsageStore) initTables() error {
	return s.db.AutoMigrate(&AIPlatformUsageLedger{})
}

type AIPlatformUsageAdminRow struct {
	ID                  string    `json:"id"`
	UserID              string    `json:"user_id"`
	UserEmail           string    `json:"user_email"`
	UserDisplayName     string    `json:"user_display_name"`
	TraderID            string    `json:"trader_id"`
	Provider            string    `json:"provider"`
	Model               string    `json:"model"`
	ActualCostUSDC      float64   `json:"actual_cost_usdc"`
	ChargedUSDT         float64   `json:"charged_usdt"`
	MarkupMultiplier    float64   `json:"markup_multiplier"`
	WalletBalanceBefore float64   `json:"wallet_balance_before"`
	WalletBalanceAfter  float64   `json:"wallet_balance_after"`
	Status              string    `json:"status"`
	PaymentTxHash       string    `json:"payment_tx_hash"`
	ErrorMessage        string    `json:"error_message"`
	CreatedAt           time.Time `json:"created_at"`
}

func (s *AIPlatformUsageStore) ListAdmin(userID string, limit int) ([]AIPlatformUsageAdminRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	q := s.db.Table("ai_platform_usage_ledger AS l").
		Select(`l.id, l.user_id, COALESCE(u.email,'') AS user_email, COALESCE(u.display_name,'') AS user_display_name,
			l.trader_id, l.provider, l.model, l.actual_cost_usdc, l.charged_usdt, l.markup_multiplier,
			l.wallet_balance_before, l.wallet_balance_after, l.status, l.payment_tx_hash, l.error_message, l.created_at`).
		Joins("LEFT JOIN users u ON u.id = l.user_id").
		Order("l.created_at DESC").
		Limit(limit)
	if userID != "" {
		q = q.Where("l.user_id = ?", userID)
	}
	var rows []AIPlatformUsageAdminRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *AIPlatformUsageStore) CreatePendingTx(
	tx *gorm.DB,
	userID, traderID, provider, model string,
	actualCostUSDC, chargedUSDT, markupMultiplier, minChargeUSDT, before, after float64,
) (string, error) {
	if tx == nil {
		tx = s.db
	}
	if userID == "" {
		return "", fmt.Errorf("user_id required")
	}
	now := time.Now().UTC()
	row := &AIPlatformUsageLedger{
		ID:                  uuid.New().String(),
		UserID:              userID,
		TraderID:            traderID,
		Provider:            provider,
		Model:               model,
		ActualCostUSDC:      actualCostUSDC,
		ChargedUSDT:         chargedUSDT,
		MarkupMultiplier:    markupMultiplier,
		MinChargeUSDT:       minChargeUSDT,
		WalletBalanceBefore: before,
		WalletBalanceAfter:  after,
		Status:              AIPlatformUsageStatusPending,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := tx.Create(row).Error; err != nil {
		return "", err
	}
	return row.ID, nil
}

func (s *AIPlatformUsageStore) CreateSuccessTx(
	tx *gorm.DB,
	userID, traderID, provider, model string,
	actualCostUSDC, chargedUSDT, markupMultiplier, minChargeUSDT, before, after float64,
	paymentRef string,
) (string, error) {
	if tx == nil {
		tx = s.db
	}
	if userID == "" {
		return "", fmt.Errorf("user_id required")
	}
	now := time.Now().UTC()
	row := &AIPlatformUsageLedger{
		ID:                  uuid.New().String(),
		UserID:              userID,
		TraderID:            traderID,
		Provider:            provider,
		Model:               model,
		ActualCostUSDC:      actualCostUSDC,
		ChargedUSDT:         chargedUSDT,
		MarkupMultiplier:    markupMultiplier,
		MinChargeUSDT:       minChargeUSDT,
		WalletBalanceBefore: before,
		WalletBalanceAfter:  after,
		Status:              AIPlatformUsageStatusSuccess,
		PaymentTxHash:       paymentRef,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := tx.Create(row).Error; err != nil {
		return "", err
	}
	return row.ID, nil
}

func (s *AIPlatformUsageStore) MarkSuccess(id, txHash string) error {
	if id == "" {
		return nil
	}
	return s.db.Model(&AIPlatformUsageLedger{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":          AIPlatformUsageStatusSuccess,
			"payment_tx_hash": txHash,
			"updated_at":      time.Now().UTC(),
		}).Error
}

func (s *AIPlatformUsageStore) MarkFailed(id string, errMsg string) error {
	if id == "" {
		return nil
	}
	return s.db.Model(&AIPlatformUsageLedger{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":        AIPlatformUsageStatusFailed,
			"error_message": errMsg,
			"updated_at":    time.Now().UTC(),
		}).Error
}

func (s *AIPlatformUsageStore) MarkRefundedTx(tx *gorm.DB, id, errMsg string, balanceAfter float64) error {
	if id == "" {
		return nil
	}
	if tx == nil {
		tx = s.db
	}
	return tx.Model(&AIPlatformUsageLedger{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":               AIPlatformUsageStatusRefunded,
			"error_message":        errMsg,
			"wallet_balance_after": balanceAfter,
			"updated_at":           time.Now().UTC(),
		}).Error
}
