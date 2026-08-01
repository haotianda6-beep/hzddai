package store

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	PartnerRebateEventDeposit  = "deposit"
	PartnerRebateEventReversal = "reversal"
)

var ErrInvalidPartnerRebateReversal = errors.New("invalid partner rebate reversal")

// PartnerRebateOutbox durably bridges main-wallet transactions to the rebate service.
type PartnerRebateOutbox struct {
	ID                     uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	EventType              string     `gorm:"not null;index:idx_partner_rebate_due,priority:1" json:"event_type"`
	UserID                 string     `gorm:"not null;index" json:"user_id"`
	AmountUSDT             float64    `gorm:"not null" json:"amount_usdt"`
	WalletLedgerID         uint64     `gorm:"not null;uniqueIndex" json:"wallet_ledger_id"`
	OriginalWalletLedgerID uint64     `gorm:"not null;default:0;index" json:"original_wallet_ledger_id"`
	Source                 string     `gorm:"not null;default:''" json:"source"`
	Note                   string     `gorm:"not null;default:''" json:"note"`
	Status                 string     `gorm:"not null;default:'pending';index:idx_partner_rebate_due,priority:1" json:"status"`
	Attempts               int        `gorm:"not null;default:0" json:"attempts"`
	LastError              string     `gorm:"not null;default:''" json:"last_error"`
	NextAttemptAt          time.Time  `gorm:"not null;index:idx_partner_rebate_due,priority:2" json:"next_attempt_at"`
	SentAt                 *time.Time `json:"sent_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

func (PartnerRebateOutbox) TableName() string { return "partner_rebate_outbox" }

type PartnerRebateOutboxStore struct{ db *gorm.DB }

func NewPartnerRebateOutboxStore(db *gorm.DB) *PartnerRebateOutboxStore {
	return &PartnerRebateOutboxStore{db: db}
}

func (s *PartnerRebateOutboxStore) initTables() error {
	return s.db.AutoMigrate(&PartnerRebateOutbox{})
}

func (s *PartnerRebateOutboxStore) Enqueue(tx *gorm.DB, event *PartnerRebateOutbox) error {
	if tx == nil {
		tx = s.db
	}
	if event == nil || event.WalletLedgerID == 0 || event.AmountUSDT <= 0 {
		return errors.New("invalid partner rebate outbox event")
	}
	if event.EventType == PartnerRebateEventReversal {
		var original WalletLedger
		err := tx.Where("id = ? AND user_id = ?", event.OriginalWalletLedgerID, event.UserID).First(&original).Error
		if err != nil || original.Delta <= 0 || !strings.HasPrefix(original.Reason, "partner_confirmed:") {
			return fmt.Errorf("%w: 原流水不是该用户的新体系确认充值", ErrInvalidPartnerRebateReversal)
		}
		var reversed float64
		if err := tx.Model(&PartnerRebateOutbox{}).
			Where("event_type = ? AND original_wallet_ledger_id = ?", PartnerRebateEventReversal, original.ID).
			Select("COALESCE(SUM(amount_usdt), 0)").Scan(&reversed).Error; err != nil {
			return err
		}
		if event.AmountUSDT > original.Delta-reversed+1e-8 {
			return fmt.Errorf("%w: 累计冲正金额超过原确认充值", ErrInvalidPartnerRebateReversal)
		}
	}
	event.Status = "pending"
	event.NextAttemptAt = time.Now().UTC()
	return tx.Create(event).Error
}

func (s *PartnerRebateOutboxStore) Get(id uint64) (*PartnerRebateOutbox, error) {
	var row PartnerRebateOutbox
	err := s.db.First(&row, id).Error
	return &row, err
}

func (s *PartnerRebateOutboxStore) ListDue(limit int) ([]PartnerRebateOutbox, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []PartnerRebateOutbox
	err := s.db.Where("status = ? AND next_attempt_at <= ?", "pending", time.Now().UTC()).
		Order("id ASC").Limit(limit).Find(&rows).Error
	return rows, err
}

func (s *PartnerRebateOutboxStore) MarkSent(id uint64) error {
	now := time.Now().UTC()
	return s.db.Model(&PartnerRebateOutbox{}).Where("id = ? AND status = ?", id, "pending").
		Updates(map[string]interface{}{"status": "sent", "sent_at": now, "last_error": ""}).Error
}

func (s *PartnerRebateOutboxStore) MarkFailed(row *PartnerRebateOutbox, deliveryErr error) error {
	if row == nil {
		return nil
	}
	attempts := row.Attempts + 1
	delay := time.Duration(math.Min(300, math.Pow(2, math.Min(float64(attempts), 8)))) * time.Second
	detail := strings.TrimSpace(deliveryErr.Error())
	if len(detail) > 500 {
		detail = detail[:500]
	}
	return s.db.Model(&PartnerRebateOutbox{}).Where("id = ? AND status = ?", row.ID, "pending").
		Updates(map[string]interface{}{
			"attempts":        attempts,
			"last_error":      detail,
			"next_attempt_at": time.Now().UTC().Add(delay),
		}).Error
}
