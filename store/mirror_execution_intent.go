package store

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	MirrorIntentPrepared  = "prepared"
	MirrorIntentSubmitted = "submitted"
	MirrorIntentConfirmed = "confirmed"
	MirrorIntentFailed    = "failed"

	MirrorBillingNone     = "none"
	MirrorBillingPending  = "pending"
	MirrorBillingCharged  = "charged"
	MirrorBillingRefunded = "refunded"
)

type MirrorExecutionIntent struct {
	ID                  string     `gorm:"primaryKey" json:"id"`
	IntentKey           string     `gorm:"not null;uniqueIndex" json:"intent_key"`
	MasterEventID       string     `gorm:"not null;index" json:"master_event_id"`
	BroadcastID         uint64     `gorm:"not null;index" json:"broadcast_id"`
	UserID              string     `gorm:"not null;index" json:"user_id"`
	TraderID            string     `gorm:"not null;index" json:"trader_id"`
	ExchangeID          string     `gorm:"not null;index" json:"exchange_id"`
	Instrument          string     `gorm:"not null" json:"instrument"`
	PositionSide        string     `gorm:"not null" json:"position_side"`
	Action              string     `gorm:"not null" json:"action"`
	RemotePositionID    string     `gorm:"not null;default:''" json:"remote_position_id"`
	TargetQuantity      float64    `gorm:"not null;default:0" json:"target_quantity"`
	DeltaQuantity       float64    `gorm:"not null;default:0" json:"delta_quantity"`
	ClientOrderID       string     `gorm:"not null;uniqueIndex" json:"client_order_id"`
	RequestSHA256       string     `gorm:"not null;default:''" json:"request_sha256"`
	ExchangeOrderID     string     `gorm:"not null;default:''" json:"exchange_order_id"`
	Status              string     `gorm:"not null;default:'prepared';index" json:"status"`
	AttemptCount        int        `gorm:"not null;default:0" json:"attempt_count"`
	BillingStatus       string     `gorm:"not null;default:'none'" json:"billing_status"`
	BillingAmount       float64    `gorm:"not null;default:0" json:"billing_amount"`
	WalletLedgerID      uint64     `gorm:"not null;default:0" json:"wallet_ledger_id"`
	UsageLedgerID       string     `gorm:"not null;default:''" json:"usage_ledger_id"`
	BillingBalanceAfter float64    `gorm:"not null;default:0" json:"billing_balance_after"`
	LastError           string     `gorm:"not null;default:''" json:"last_error"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	ConfirmedAt         *time.Time `json:"confirmed_at,omitempty"`
}

func (MirrorExecutionIntent) TableName() string { return "mirror_execution_intents" }

type MirrorExecutionIntentInput struct {
	IntentKey, MasterEventID, UserID, TraderID, ExchangeID string
	Instrument, PositionSide, Action, RemotePositionID     string
	ClientOrderID, RequestSHA256                           string
	BroadcastID                                            uint64
	TargetQuantity, DeltaQuantity                          float64
}

type MirrorBillingReservation struct {
	Reserved, Insufficient bool
	BalanceAfter           float64
	WalletLedgerID         uint64
	UsageLedgerID          string
}

type MirrorExecutionIntentStore struct {
	db    *gorm.DB
	store *Store
}

// ponytail: SQLite is a single-writer store; serialize only these short wallet
// transactions. Exchange fan-out remains concurrent. Remove when production moves to Postgres.
var mirrorBillingSQLiteMu sync.Mutex

func (s *MirrorExecutionIntentStore) billingTransaction(operation func() error) error {
	if s.db != nil && s.db.Dialector.Name() == "sqlite" {
		mirrorBillingSQLiteMu.Lock()
		defer mirrorBillingSQLiteMu.Unlock()
	}
	return retrySQLiteBusy(s.db, operation)
}

func NewMirrorExecutionIntentStore(db *gorm.DB, st *Store) *MirrorExecutionIntentStore {
	return &MirrorExecutionIntentStore{db: db, store: st}
}

func (s *MirrorExecutionIntentStore) initTables() error {
	return s.db.AutoMigrate(&MirrorExecutionIntent{})
}

func (s *MirrorExecutionIntentStore) Ensure(input MirrorExecutionIntentInput) (*MirrorExecutionIntent, error) {
	input.IntentKey = strings.TrimSpace(input.IntentKey)
	input.ClientOrderID = strings.TrimSpace(input.ClientOrderID)
	if input.IntentKey == "" || input.ClientOrderID == "" || strings.TrimSpace(input.Action) == "" {
		return nil, fmt.Errorf("intent_key, client_order_id and action required")
	}
	now := time.Now().UTC()
	row := &MirrorExecutionIntent{
		ID: uuid.New().String(), IntentKey: input.IntentKey, MasterEventID: input.MasterEventID,
		BroadcastID: input.BroadcastID, UserID: input.UserID, TraderID: input.TraderID,
		ExchangeID: input.ExchangeID, Instrument: input.Instrument, PositionSide: input.PositionSide,
		Action: input.Action, RemotePositionID: input.RemotePositionID,
		TargetQuantity: input.TargetQuantity, DeltaQuantity: input.DeltaQuantity,
		ClientOrderID: input.ClientOrderID, RequestSHA256: input.RequestSHA256,
		Status: MirrorIntentPrepared, BillingStatus: MirrorBillingNone,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := retrySQLiteBusy(s.db, func() error {
		return s.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "intent_key"}}, DoNothing: true}).Create(row).Error
	}); err != nil {
		return nil, err
	}
	var existing MirrorExecutionIntent
	if err := s.db.Where("intent_key = ?", input.IntentKey).First(&existing).Error; err != nil {
		return nil, err
	}
	if existing.ClientOrderID != input.ClientOrderID ||
		(existing.RequestSHA256 != "" && input.RequestSHA256 != "" && existing.RequestSHA256 != input.RequestSHA256) {
		return nil, fmt.Errorf("mirror intent conflict")
	}
	return &existing, nil
}

func (s *MirrorExecutionIntentStore) Get(intentKey string) (*MirrorExecutionIntent, error) {
	var row MirrorExecutionIntent
	if err := s.db.Where("intent_key = ?", intentKey).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// ListConfirmedForTrader returns executable intents in creation order so an
// exchange trade stream can be projected into local orders and positions.
func (s *MirrorExecutionIntentStore) ListConfirmedForTrader(traderID string, limit int) ([]MirrorExecutionIntent, error) {
	if limit <= 0 {
		limit = 200
	}
	var rows []MirrorExecutionIntent
	err := s.db.Where("trader_id = ? AND status = ? AND action IN ?", traderID, MirrorIntentConfirmed,
		[]string{"open", "increase", "reduce", "close"}).
		Order("created_at DESC").Limit(limit).Find(&rows).Error
	for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
		rows[left], rows[right] = rows[right], rows[left]
	}
	return rows, err
}

// SetExchangeOrderID repairs the remote order identifier without changing the
// intent lifecycle or attempt count.
func (s *MirrorExecutionIntentStore) SetExchangeOrderID(intentKey, exchangeOrderID string) error {
	if strings.TrimSpace(exchangeOrderID) == "" {
		return fmt.Errorf("exchange_order_id required")
	}
	return retrySQLiteBusy(s.db, func() error {
		return s.db.Model(&MirrorExecutionIntent{}).Where("intent_key = ?", intentKey).Updates(map[string]interface{}{
			"exchange_order_id": exchangeOrderID, "updated_at": time.Now().UTC(),
		}).Error
	})
}

func (s *MirrorExecutionIntentStore) MarkSubmitted(intentKey string) error {
	return retrySQLiteBusy(s.db, func() error {
		return s.db.Model(&MirrorExecutionIntent{}).Where("intent_key = ?", intentKey).Updates(map[string]interface{}{
			"status": MirrorIntentSubmitted, "attempt_count": gorm.Expr("attempt_count + 1"),
			"last_error": "", "updated_at": time.Now().UTC(),
		}).Error
	})
}

func (s *MirrorExecutionIntentStore) MarkConfirmed(intentKey, exchangeOrderID string) error {
	now := time.Now().UTC()
	return retrySQLiteBusy(s.db, func() error {
		return s.db.Model(&MirrorExecutionIntent{}).Where("intent_key = ?", intentKey).Updates(map[string]interface{}{
			"status": MirrorIntentConfirmed, "exchange_order_id": exchangeOrderID,
			"last_error": "", "confirmed_at": now, "updated_at": now,
		}).Error
	})
}

func (s *MirrorExecutionIntentStore) MarkFailed(intentKey, lastError string) error {
	return retrySQLiteBusy(s.db, func() error {
		return s.db.Model(&MirrorExecutionIntent{}).Where("intent_key = ?", intentKey).Updates(map[string]interface{}{
			"status": MirrorIntentFailed, "last_error": lastError, "updated_at": time.Now().UTC(),
		}).Error
	})
}

func (s *MirrorExecutionIntentStore) ReserveBilling(intentKey string, amount float64) (MirrorBillingReservation, error) {
	if amount <= 0 {
		return MirrorBillingReservation{}, nil
	}
	var result MirrorBillingReservation
	err := s.billingTransaction(func() error {
		result = MirrorBillingReservation{}
		return s.db.Transaction(func(tx *gorm.DB) error {
			var intent MirrorExecutionIntent
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("intent_key = ?", intentKey).First(&intent).Error; err != nil {
				return err
			}
			if intent.BillingStatus == MirrorBillingPending || intent.BillingStatus == MirrorBillingCharged {
				result = MirrorBillingReservation{Reserved: true, BalanceAfter: intent.BillingBalanceAfter,
					WalletLedgerID: intent.WalletLedgerID, UsageLedgerID: intent.UsageLedgerID}
				return nil
			}
			if intent.BillingStatus == MirrorBillingRefunded {
				result = MirrorBillingReservation{BalanceAfter: intent.BillingBalanceAfter,
					WalletLedgerID: intent.WalletLedgerID, UsageLedgerID: intent.UsageLedgerID}
				return nil
			}
			var user User
			if err := tx.Where("id = ?", intent.UserID).First(&user).Error; err != nil {
				return err
			}
			balanceAfter, ok, err := s.store.User().AddBalanceDelta(tx, intent.UserID, -amount)
			if err != nil {
				return err
			}
			if !ok {
				result = MirrorBillingReservation{Insufficient: true, BalanceAfter: user.BalanceUSDT}
				return nil
			}
			ledgerID, err := s.store.Billing().AppendLedger(tx, intent.UserID, -amount, balanceAfter, "comkun_follow_scan", intent.TraderID)
			if err != nil {
				return err
			}
			usageID, err := s.store.AIPlatformUsage().CreatePendingTx(tx, intent.UserID, intent.TraderID,
				"comkun-ai", "comkun-ai", 0, amount, 0, amount, user.BalanceUSDT, balanceAfter)
			if err != nil {
				return err
			}
			if err := tx.Model(&MirrorExecutionIntent{}).Where("id = ?", intent.ID).Updates(map[string]interface{}{
				"billing_status": MirrorBillingPending, "billing_amount": amount,
				"wallet_ledger_id": ledgerID, "usage_ledger_id": usageID,
				"billing_balance_after": balanceAfter, "updated_at": time.Now().UTC(),
			}).Error; err != nil {
				return err
			}
			result = MirrorBillingReservation{Reserved: true, BalanceAfter: balanceAfter, WalletLedgerID: ledgerID, UsageLedgerID: usageID}
			return nil
		})
	})
	return result, err
}

func (s *MirrorExecutionIntentStore) FinalizeBilling(intentKey string) error {
	return s.billingTransaction(func() error {
		return s.db.Transaction(func(tx *gorm.DB) error {
			var intent MirrorExecutionIntent
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("intent_key = ?", intentKey).First(&intent).Error; err != nil {
				return err
			}
			if intent.BillingStatus == MirrorBillingCharged || intent.BillingStatus == MirrorBillingNone {
				return nil
			}
			if intent.BillingStatus != MirrorBillingPending {
				return fmt.Errorf("billing cannot finalize from %s", intent.BillingStatus)
			}
			if err := tx.Model(&AIPlatformUsageLedger{}).Where("id = ?", intent.UsageLedgerID).Updates(map[string]interface{}{
				"status":          AIPlatformUsageStatusSuccess,
				"payment_tx_hash": fmt.Sprintf("wallet_ledger:%d", intent.WalletLedgerID),
				"updated_at":      time.Now().UTC(),
			}).Error; err != nil {
				return err
			}
			return tx.Model(&MirrorExecutionIntent{}).Where("id = ?", intent.ID).Updates(map[string]interface{}{
				"billing_status": MirrorBillingCharged, "updated_at": time.Now().UTC(),
			}).Error
		})
	})
}

func (s *MirrorExecutionIntentStore) RefundBilling(intentKey, reason string) error {
	return s.billingTransaction(func() error {
		return s.db.Transaction(func(tx *gorm.DB) error {
			var intent MirrorExecutionIntent
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("intent_key = ?", intentKey).First(&intent).Error; err != nil {
				return err
			}
			if intent.BillingStatus == MirrorBillingRefunded || intent.BillingStatus == MirrorBillingNone {
				return nil
			}
			if intent.BillingStatus != MirrorBillingPending && intent.BillingStatus != MirrorBillingCharged {
				return fmt.Errorf("billing cannot refund from %s", intent.BillingStatus)
			}
			balanceAfter, _, err := s.store.User().AddBalanceDelta(tx, intent.UserID, intent.BillingAmount)
			if err != nil {
				return err
			}
			if _, err := s.store.Billing().AppendLedger(tx, intent.UserID, intent.BillingAmount, balanceAfter,
				"comkun_follow_scan_refund:"+strings.TrimSpace(reason), intent.TraderID); err != nil {
				return err
			}
			if err := s.store.AIPlatformUsage().MarkRefundedTx(tx, intent.UsageLedgerID, reason, balanceAfter); err != nil {
				return err
			}
			return tx.Model(&MirrorExecutionIntent{}).Where("id = ?", intent.ID).Updates(map[string]interface{}{
				"billing_status": MirrorBillingRefunded, "billing_balance_after": balanceAfter,
				"updated_at": time.Now().UTC(),
			}).Error
		})
	})
}

func (s *MirrorExecutionIntentStore) HasConfirmedOpen(masterEventID, userID, traderID string) (bool, error) {
	var count int64
	err := s.db.Model(&MirrorExecutionIntent{}).
		Where("master_event_id = ? AND user_id = ? AND trader_id = ? AND action = ? AND status = ?",
			masterEventID, userID, traderID, "open", MirrorIntentConfirmed).
		Count(&count).Error
	return count > 0, err
}

func (s *MirrorExecutionIntentStore) CorrectRefundedBillingForConfirmedOpen(intentKey, reason string) (bool, error) {
	corrected := false
	err := s.billingTransaction(func() error {
		return s.db.Transaction(func(tx *gorm.DB) error {
			var intent MirrorExecutionIntent
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("intent_key = ?", intentKey).First(&intent).Error; err != nil {
				return err
			}
			if intent.BillingStatus == MirrorBillingCharged {
				return nil
			}
			if intent.BillingStatus != MirrorBillingRefunded {
				return fmt.Errorf("billing correction requires refunded status, got %s", intent.BillingStatus)
			}
			var confirmed int64
			if err := tx.Model(&MirrorExecutionIntent{}).
				Where("master_event_id = ? AND user_id = ? AND trader_id = ? AND action = ? AND status = ?",
					intent.MasterEventID, intent.UserID, intent.TraderID, "open", MirrorIntentConfirmed).
				Count(&confirmed).Error; err != nil {
				return err
			}
			if confirmed == 0 {
				return fmt.Errorf("billing correction requires a confirmed open intent")
			}
			balanceAfter, ok, err := s.store.User().AddBalanceDelta(tx, intent.UserID, -intent.BillingAmount)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("billing correction balance insufficient")
			}
			ledgerID, err := s.store.Billing().AppendLedger(tx, intent.UserID, -intent.BillingAmount, balanceAfter,
				"comkun_follow_scan_correction:"+strings.TrimSpace(reason), intent.TraderID)
			if err != nil {
				return err
			}
			if err := tx.Model(&AIPlatformUsageLedger{}).Where("id = ?", intent.UsageLedgerID).Updates(map[string]interface{}{
				"status":               AIPlatformUsageStatusSuccess,
				"error_message":        "",
				"wallet_balance_after": balanceAfter,
				"payment_tx_hash":      fmt.Sprintf("wallet_ledger:%d", ledgerID),
				"updated_at":           time.Now().UTC(),
			}).Error; err != nil {
				return err
			}
			if err := tx.Model(&MirrorExecutionIntent{}).Where("id = ?", intent.ID).Updates(map[string]interface{}{
				"billing_status":        MirrorBillingCharged,
				"wallet_ledger_id":      ledgerID,
				"billing_balance_after": balanceAfter,
				"last_error":            "billing_correction:" + strings.TrimSpace(reason),
				"updated_at":            time.Now().UTC(),
			}).Error; err != nil {
				return err
			}
			corrected = true
			return nil
		})
	})
	return corrected, err
}
