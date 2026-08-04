package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const HZMasterSourcePrefix = "hz-ai-master:"

var ErrIntegrationNonceReplay = errors.New("integration nonce replay")

type IntegrationMasterEvent struct {
	ID               string    `gorm:"primaryKey" json:"id"`
	Provider         string    `gorm:"not null;uniqueIndex:idx_integration_event_id,priority:1;uniqueIndex:idx_integration_event_sequence,priority:1" json:"provider"`
	MasterAccountID  string    `gorm:"not null;index;uniqueIndex:idx_integration_event_sequence,priority:2" json:"master_account_id"`
	ProducerEventID  string    `gorm:"not null;uniqueIndex:idx_integration_event_id,priority:2" json:"producer_event_id"`
	ProducerSequence int64     `gorm:"not null;uniqueIndex:idx_integration_event_sequence,priority:3" json:"producer_sequence"`
	EventType        string    `gorm:"not null" json:"event_type"`
	OccurredAt       time.Time `gorm:"not null" json:"occurred_at"`
	ReceivedAt       time.Time `gorm:"not null" json:"received_at"`
	PayloadVersion   int       `gorm:"not null" json:"payload_version"`
	PayloadSHA256    string    `gorm:"not null" json:"payload_sha256"`
	PayloadJSON      string    `gorm:"type:text;not null" json:"-"`
	AuthNonce        string    `gorm:"not null;uniqueIndex" json:"-"`
	IngestStatus     string    `gorm:"not null" json:"ingest_status"`
	BroadcastID      uint64    `gorm:"not null;index" json:"broadcast_id"`
	SourceStrategyID string    `gorm:"not null;index" json:"source_strategy_id"`
	LastError        string    `gorm:"not null;default:''" json:"last_error"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (IntegrationMasterEvent) TableName() string { return "integration_master_events" }

type IntegrationMasterEventInput struct {
	Provider        string
	MasterAccountID string
	EventID         string
	Sequence        int64
	EventType       string
	OccurredAt      time.Time
	PayloadVersion  int
	PayloadSHA256   string
	PayloadJSON     string
	AuthNonce       string
	MasterEquity    float64
	MasterStateJSON string
}

type IntegrationMasterEventResult struct {
	Accepted             bool   `json:"accepted"`
	Duplicate            bool   `json:"duplicate"`
	Gap                  bool   `json:"gap"`
	EventID              string `json:"event_id"`
	BroadcastID          uint64 `json:"broadcast_id,omitempty"`
	SourceStrategyID     string `json:"source_strategy_id,omitempty"`
	ExpectedNextSequence int64  `json:"expected_next_sequence"`
}

type IntegrationMasterEventStore struct {
	db *gorm.DB
}

func NewIntegrationMasterEventStore(db *gorm.DB) *IntegrationMasterEventStore {
	return &IntegrationMasterEventStore{db: db}
}

func (s *IntegrationMasterEventStore) initTables() error {
	return s.db.AutoMigrate(&IntegrationMasterEvent{})
}

func HZMasterSourceStrategyID(masterAccountID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(masterAccountID)))
	return HZMasterSourcePrefix + hex.EncodeToString(sum[:12])
}

func (s *IntegrationMasterEventStore) Accept(input IntegrationMasterEventInput, allowGap bool) (IntegrationMasterEventResult, error) {
	input.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
	input.MasterAccountID = strings.TrimSpace(input.MasterAccountID)
	input.EventID = strings.TrimSpace(input.EventID)
	input.EventType = strings.TrimSpace(input.EventType)
	input.AuthNonce = strings.TrimSpace(input.AuthNonce)
	if input.Provider == "" || input.MasterAccountID == "" || input.EventID == "" || input.Sequence <= 0 ||
		input.EventType == "" || input.OccurredAt.IsZero() || input.PayloadVersion <= 0 ||
		input.PayloadSHA256 == "" || input.PayloadJSON == "" || input.AuthNonce == "" || input.MasterStateJSON == "" {
		return IntegrationMasterEventResult{}, fmt.Errorf("invalid integration master event")
	}

	sourceID := HZMasterSourceStrategyID(input.MasterAccountID)
	result := IntegrationMasterEventResult{EventID: input.EventID, SourceStrategyID: sourceID}
	accepted := false
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var existing IntegrationMasterEvent
		err := tx.Where("provider = ? AND (producer_event_id = ? OR (master_account_id = ? AND producer_sequence = ?))",
			input.Provider, input.EventID, input.MasterAccountID, input.Sequence).First(&existing).Error
		if err == nil {
			result.Duplicate = true
			result.BroadcastID = existing.BroadcastID
			result.ExpectedNextSequence = existing.ProducerSequence + 1
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if input.Provider == "hz-reconcile" {
			var pushed IntegrationMasterEvent
			pushErr := tx.Where("provider = ? AND master_account_id = ? AND producer_sequence = ?",
				"hz", input.MasterAccountID, input.Sequence).First(&pushed).Error
			if pushErr == nil {
				result.Duplicate = true
				result.BroadcastID = pushed.BroadcastID
				result.ExpectedNextSequence = pushed.ProducerSequence + 1
				return nil
			}
			if !errors.Is(pushErr, gorm.ErrRecordNotFound) {
				return pushErr
			}
		}
		var nonceCount int64
		if err := tx.Model(&IntegrationMasterEvent{}).Where("auth_nonce = ?", input.AuthNonce).Count(&nonceCount).Error; err != nil {
			return err
		}
		if nonceCount > 0 {
			return ErrIntegrationNonceReplay
		}

		var latest int64
		if err := tx.Model(&IntegrationMasterEvent{}).
			Where("provider = ? AND master_account_id = ?", input.Provider, input.MasterAccountID).
			Select("COALESCE(MAX(producer_sequence),0)").Scan(&latest).Error; err != nil {
			return err
		}
		if latest > 0 && input.Sequence <= latest {
			result.Duplicate = true
			result.ExpectedNextSequence = latest + 1
			return nil
		}
		if latest > 0 && input.Sequence != latest+1 && !allowGap {
			result.Gap = true
			result.ExpectedNextSequence = latest + 1
			return nil
		}

		now := time.Now().UTC()
		if input.Provider == "hz" {
			var reconciled IntegrationMasterEvent
			reconcileErr := tx.Where("provider = ? AND master_account_id = ? AND producer_sequence = ?",
				"hz-reconcile", input.MasterAccountID, input.Sequence).First(&reconciled).Error
			if reconcileErr == nil {
				if err := tx.Model(&ComkunMasterBroadcast{}).Where("id = ?", reconciled.BroadcastID).Updates(map[string]any{
					"master_account_equity": input.MasterEquity,
					"master_state_json":     input.MasterStateJSON,
				}).Error; err != nil {
					return err
				}
				row := &IntegrationMasterEvent{
					ID: uuid.New().String(), Provider: input.Provider, MasterAccountID: input.MasterAccountID,
					ProducerEventID: input.EventID, ProducerSequence: input.Sequence, EventType: input.EventType,
					OccurredAt: input.OccurredAt.UTC(), ReceivedAt: now, PayloadVersion: input.PayloadVersion,
					PayloadSHA256: input.PayloadSHA256, PayloadJSON: input.PayloadJSON, AuthNonce: input.AuthNonce,
					IngestStatus: "promoted_reconcile", BroadcastID: reconciled.BroadcastID, SourceStrategyID: sourceID,
					CreatedAt: now, UpdatedAt: now,
				}
				if err := tx.Create(row).Error; err != nil {
					return err
				}
				accepted = true
				result.Accepted = true
				result.BroadcastID = reconciled.BroadcastID
				result.ExpectedNextSequence = input.Sequence + 1
				return nil
			}
			if !errors.Is(reconcileErr, gorm.ErrRecordNotFound) {
				return reconcileErr
			}
		}
		broadcast := &ComkunMasterBroadcast{
			SourceStrategyID: sourceID, MasterAccountEquity: input.MasterEquity,
			AnalysisText: "AI策略执行", DecisionJSON: "[]",
			MasterStateJSON: input.MasterStateJSON, CreatedAt: now,
		}
		if err := tx.Create(broadcast).Error; err != nil {
			return err
		}
		status := "published"
		if latest > 0 && input.Sequence != latest+1 {
			status = "reconciled_gap"
		}
		row := &IntegrationMasterEvent{
			ID: uuid.New().String(), Provider: input.Provider, MasterAccountID: input.MasterAccountID,
			ProducerEventID: input.EventID, ProducerSequence: input.Sequence, EventType: input.EventType,
			OccurredAt: input.OccurredAt.UTC(), ReceivedAt: now, PayloadVersion: input.PayloadVersion,
			PayloadSHA256: input.PayloadSHA256, PayloadJSON: input.PayloadJSON, AuthNonce: input.AuthNonce,
			IngestStatus: status, BroadcastID: broadcast.ID, SourceStrategyID: sourceID,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		accepted = true
		result.Accepted = true
		result.BroadcastID = broadcast.ID
		result.ExpectedNextSequence = input.Sequence + 1
		return nil
	})
	if err != nil {
		// A concurrent delivery may win the unique event/sequence insert after
		// this transaction's initial lookup. In that case the protocol still
		// requires a successful duplicate acknowledgement.
		var existing IntegrationMasterEvent
		lookupErr := s.db.Where("provider = ? AND (producer_event_id = ? OR (master_account_id = ? AND producer_sequence = ?))",
			input.Provider, input.EventID, input.MasterAccountID, input.Sequence).First(&existing).Error
		if lookupErr == nil {
			return IntegrationMasterEventResult{
				Duplicate: true, EventID: input.EventID, BroadcastID: existing.BroadcastID,
				SourceStrategyID: existing.SourceStrategyID, ExpectedNextSequence: existing.ProducerSequence + 1,
			}, nil
		}
		return IntegrationMasterEventResult{}, err
	}
	if accepted {
		NotifyComkunFollowersOfBroadcast(sourceID)
	}
	return result, nil
}

func (s *IntegrationMasterEventStore) GetByBroadcastID(broadcastID uint64) (*IntegrationMasterEvent, error) {
	var row IntegrationMasterEvent
	if err := s.db.Where("broadcast_id = ?", broadcastID).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *IntegrationMasterEventStore) GetLatestSequence(provider, masterAccountID string) (int64, error) {
	var latest int64
	err := s.db.Model(&IntegrationMasterEvent{}).
		Where("provider = ? AND master_account_id = ?", strings.ToLower(strings.TrimSpace(provider)), strings.TrimSpace(masterAccountID)).
		Select("COALESCE(MAX(producer_sequence),0)").Scan(&latest).Error
	return latest, err
}
