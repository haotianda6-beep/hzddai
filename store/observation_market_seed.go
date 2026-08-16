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

const (
	observationMarketEnabledEnv        = "COMKUN_OBSERVER_MARKET_ENABLED"
	observationLiveMasterIDsEnv        = "COMKUN_OBSERVER_LIVE_MASTER_IDS"
	observationExpectedLiveMasterCount = 3
)

func observationMarketEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(observationMarketEnabledEnv))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func observationLiveMasterSourceIDs() ([]string, bool) {
	parts := strings.Split(strings.TrimSpace(os.Getenv(observationLiveMasterIDsEnv)), ",")
	if len(parts) != observationExpectedLiveMasterCount {
		return nil, false
	}
	sources := make([]string, 0, observationExpectedLiveMasterCount)
	seen := make(map[string]bool, observationExpectedLiveMasterCount)
	for _, raw := range parts {
		masterID := strings.TrimSpace(raw)
		if masterID == "" || seen[masterID] {
			return nil, false
		}
		seen[masterID] = true
		sources = append(sources, HZMasterSourceStrategyID(masterID))
	}
	return sources, true
}

// EnsureObservationMarketSeeds publishes the three approved historical-scenario
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
	liveSources, liveReady := observationLiveMasterSourceIDs()
	err = s.gdb.Transaction(func(tx *gorm.DB) error {
		for _, slot := range doc.Slots {
			if slot.Slot > observationExpectedLiveMasterCount {
				continue
			}
			profile, ok := observation.MarketProfileBySlot(slot.Slot)
			if !ok {
				return fmt.Errorf("observation market profile missing slot=%d", slot.Slot)
			}
			cfg := GetDefaultStrategyConfig("zh")
			cfg.MarketSalePriceUSDT = profile.MonthlyPriceUSDT
			cfg.MarketSubscriptionMonthlyOnly = true
			cfg.MarketPerformanceOnly = !liveReady
			cfg.MarketPerformanceSource = observation.StatusHistoricalSimulation
			cfg.MarketPerformanceDisclosure = "模拟业绩"
			cfg.MarketRealtimeFollowAvailable = liveReady
			cfg.ComkunMarketFollow = false
			cfg.ComkunFollowListingTemplate = liveReady
			cfg.ComkunMarketSourceStrategyID = ""
			cfg.ComkunMarketListingStrategyID = ""
			if liveReady {
				cfg.ComkunMarketSourceStrategyID = liveSources[slot.Slot-1]
				cfg.ComkunFollowMirrorMasterExchange = true
			}
			raw, err := json.Marshal(cfg)
			if err != nil {
				return err
			}
			strategy := &Strategy{
				ID: observation.StrategyID(slot.Slot), UserID: owner.ID,
				Name: profile.Name, Description: profile.Description,
				IsActive: false, IsDefault: false, Config: string(raw),
				MarketAIModel: "comkun_ai", MarketRevision: 2, ContentLocked: true,
				CreatedAt: now, UpdatedAt: now,
			}
			access := MarketAccessPublic
			if liveReady {
				access = MarketAccessSubscription
			}
			SyncListingFlagsFromAccess(strategy, access)
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
		logger.Infof("✅ observation market strategies ready count=%d live_routes=%t source=%s", observationExpectedLiveMasterCount, liveReady, doc.Status)
	}
	return err
}
