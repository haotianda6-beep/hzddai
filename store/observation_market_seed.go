package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"nofx/logger"
	"nofx/marketdata/observation"

	"gorm.io/gorm"
)

const observationMarketEnabledEnv = "COMKUN_OBSERVER_MARKET_ENABLED"

func observationMarketEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(observationMarketEnabledEnv))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// EnsureObservationMarketSeeds publishes the six verified historical-scenario
// profiles. It never creates exchanges, traders, credentials, or live routes.
func (s *Store) EnsureObservationMarketSeeds() error {
	if !observationMarketEnabled() {
		return nil
	}
	doc, err := observation.Load()
	if err != nil {
		return err
	}
	owner, err := s.User().GetByEmail(comkunFollowListingOwnerEmail)
	if err != nil {
		logger.Warnf("observation market seed: owner not found, skip: %v", err)
		return nil
	}
	now := time.Now().UTC()
	err = s.gdb.Transaction(func(tx *gorm.DB) error {
		for _, slot := range doc.Slots {
			cfg := GetDefaultStrategyConfig("zh")
			cfg.MarketPerformanceOnly = true
			cfg.MarketPerformanceSource = observation.StatusHistoricalSimulation
			cfg.MarketPerformanceDisclosure = slot.History.Disclosure
			cfg.MarketRealtimeFollowAvailable = false
			cfg.ComkunMarketFollow = false
			cfg.ComkunFollowListingTemplate = false
			cfg.ComkunMarketSourceStrategyID = ""
			raw, err := json.Marshal(cfg)
			if err != nil {
				return err
			}
			strategy := &Strategy{
				ID: observation.StrategyID(slot.Slot), UserID: owner.ID,
				Name: slot.DisplayName,
				Description: fmt.Sprintf(
					"%s；%d 个完整月份 + 当月进行中，共 %d 笔。历史行情场景数据，非实盘已实现收益；目前不支持实时跟单。",
					slot.History.StrategyLabel, slot.History.Months, slot.History.TradeCount,
				),
				IsActive: false, IsDefault: false, Config: string(raw),
				MarketAIModel: "comkun_ai", MarketRevision: 1, ContentLocked: true,
				CreatedAt: now, UpdatedAt: now,
			}
			SyncListingFlagsFromAccess(strategy, MarketAccessPublic)
			var existing Strategy
			findErr := tx.Where("id = ?", strategy.ID).First(&existing).Error
			switch {
			case errors.Is(findErr, gorm.ErrRecordNotFound):
				if err := tx.Create(strategy).Error; err != nil {
					return err
				}
			case findErr != nil:
				return findErr
			default:
				strategy.CreatedAt = existing.CreatedAt
				if err := tx.Model(&existing).Updates(map[string]any{
					"user_id": owner.ID, "name": strategy.Name, "description": strategy.Description,
					"is_active": false, "is_default": false, "is_public": strategy.IsPublic,
					"config_visible": strategy.ConfigVisible, "market_access": strategy.MarketAccess,
					"market_ai_model": strategy.MarketAIModel, "config": strategy.Config,
					"market_revision": strategy.MarketRevision, "source_strategy_id": "",
					"source_market_access": "", "content_locked": true, "updated_at": now,
				}).Error; err != nil {
					return err
				}
			}
			// Strategy.ConfigVisible has a legacy database default of true. Force the
			// zero value after first insert so public historical profiles never expose
			// their internal config.
			if err := tx.Model(&Strategy{}).Where("id = ?", strategy.ID).
				Update("config_visible", false).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		logger.Infof("✅ observation market strategies ready count=%d source=%s", len(doc.Slots), doc.Status)
	}
	return err
}
