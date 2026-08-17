package store

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// ComkunFollowingStatsRow is the read-only, non-PII snapshot used by the
// admin dashboard and the COMKUN -> BALIB integration.
type ComkunFollowingStatsRow struct {
	StrategyID              string `json:"strategyId"`
	StrategyName            string `json:"strategyName"`
	SourceStrategyID        string `json:"sourceStrategyId"`
	RunningFollowerCount    int    `json:"runningFollowerCount"`
	RunningTraderCount      int    `json:"runningTraderCount"`
	SubscribedFollowerCount int    `json:"subscribedFollowerCount"`
}

// ComkunMasterSourceMapping maps the immutable source strategy ID to the
// stable BALIB/COMKUN master user ID configured by operations. Strategy names
// and display slots are deliberately not involved in this mapping.
func ComkunMasterSourceMapping() (map[string]string, error) {
	raw := strings.TrimSpace(os.Getenv("COMKUN_OBSERVER_LIVE_MASTER_IDS"))
	if raw == "" {
		return nil, fmt.Errorf("COMKUN_OBSERVER_LIVE_MASTER_IDS is not configured")
	}
	mapping := make(map[string]string)
	for _, part := range strings.Split(raw, ",") {
		masterID := strings.TrimSpace(part)
		if masterID == "" {
			return nil, fmt.Errorf("COMKUN_OBSERVER_LIVE_MASTER_IDS contains an empty ID")
		}
		sourceID := HZMasterSourceStrategyID(masterID)
		if _, exists := mapping[sourceID]; exists {
			return nil, fmt.Errorf("duplicate COMKUN master ID")
		}
		mapping[sourceID] = masterID
	}
	if len(mapping) == 0 {
		return nil, fmt.Errorf("COMKUN_OBSERVER_LIVE_MASTER_IDS is empty")
	}
	return mapping, nil
}

type comkunStatsStrategy struct {
	StrategyID   string
	StrategyName string
	SourceID     string
}

// ListComkunFollowingStats counts only active subscriptions and currently
// started traders with an enabled exchange. Running users are deduplicated by
// (listing strategy, final user), while running traders remain a separate
// metric. No credentials or user PII leave this method.
func (s *TraderStore) ListComkunFollowingStats() ([]ComkunFollowingStatsRow, error) {
	mapping, mappingErr := ComkunMasterSourceMapping()
	var strategies []Strategy
	if err := s.db.Find(&strategies).Error; err != nil {
		return nil, err
	}
	listings := make(map[string]comkunStatsStrategy)
	for _, strategy := range strategies {
		var cfg StrategyConfig
		if json.Unmarshal([]byte(strategy.Config), &cfg) != nil || !cfg.ComkunFollowListingTemplate {
			continue
		}
		sourceID := strings.TrimSpace(cfg.ComkunMarketSourceStrategyID)
		if sourceID == "" {
			continue
		}
		if mappingErr == nil {
			if _, ok := mapping[sourceID]; !ok {
				continue
			}
		}
		listings[strategy.ID] = comkunStatsStrategy{
			StrategyID: strategy.ID, StrategyName: strings.TrimSpace(strategy.Name), SourceID: sourceID,
		}
	}

	result := make(map[string]*ComkunFollowingStatsRow, len(listings))
	for id, listing := range listings {
		result[id] = &ComkunFollowingStatsRow{
			StrategyID: id, StrategyName: listing.StrategyName, SourceStrategyID: listing.SourceID,
		}
	}

	now := time.Now().UTC()
	var entitlements []StrategyMarketEntitlement
	if len(listings) > 0 {
		ids := make([]string, 0, len(listings))
		for id := range listings {
			ids = append(ids, id)
		}
		if err := s.db.Where("strategy_id IN ?", ids).
			Where("subscription_until IS NULL OR subscription_until > ?", now).
			Find(&entitlements).Error; err != nil {
			return nil, err
		}
	}
	subscribedByListing := make(map[string]map[string]struct{}, len(listings))
	for _, entitlement := range entitlements {
		row := result[entitlement.StrategyID]
		if row == nil {
			continue
		}
		users := subscribedByListing[entitlement.StrategyID]
		if users == nil {
			users = make(map[string]struct{})
			subscribedByListing[entitlement.StrategyID] = users
		}
		if _, exists := users[entitlement.UserID]; exists {
			continue
		}
		users[entitlement.UserID] = struct{}{}
		row.SubscribedFollowerCount++
	}

	var running []struct {
		UserID     string
		StrategyID string
		Config     string
	}
	if err := s.db.Table("traders AS t").
		Select("t.user_id, t.strategy_id, COALESCE(s.config, '') AS config").
		Joins("INNER JOIN strategies AS s ON s.id = t.strategy_id").
		Joins("INNER JOIN exchanges AS e ON e.id = t.exchange_id").
		Where("t.is_running = ? AND e.enabled = ?", true, true).
		Where("s.config LIKE ? AND s.config LIKE ?", "%comkun_market_follow%", "%comkun_market_source_strategy_id%").
		Find(&running).Error; err != nil {
		return nil, err
	}

	usersByListing := make(map[string]map[string]struct{})
	for _, item := range running {
		var cfg StrategyConfig
		if json.Unmarshal([]byte(item.Config), &cfg) != nil || !cfg.ComkunMarketFollow {
			continue
		}
		sourceID := strings.TrimSpace(cfg.ComkunMarketSourceStrategyID)
		listingID := strings.TrimSpace(cfg.ComkunMarketListingStrategyID)
		listing, ok := listings[listingID]
		if !ok || listing.SourceID != sourceID {
			continue
		}
		var entitlement StrategyMarketEntitlement
		query := s.db.Where("user_id = ? AND strategy_id = ?", item.UserID, listingID).
			Where("subscription_until IS NULL OR subscription_until > ?", now).First(&entitlement)
		if query.Error != nil {
			continue
		}
		row := result[listingID]
		row.RunningTraderCount++
		if usersByListing[listingID] == nil {
			usersByListing[listingID] = make(map[string]struct{})
		}
		usersByListing[listingID][item.UserID] = struct{}{}
	}
	for listingID, users := range usersByListing {
		result[listingID].RunningFollowerCount = len(users)
	}

	rows := make([]ComkunFollowingStatsRow, 0, len(result))
	for _, row := range result {
		if row.RunningFollowerCount > row.SubscribedFollowerCount {
			row.RunningFollowerCount = row.SubscribedFollowerCount
		}
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].StrategyID < rows[j].StrategyID })
	return rows, nil
}

type ComkunFollowingStatsTotals struct {
	RunningFollowerCount    int
	RunningTraderCount      int
	SubscribedFollowerCount int
}

// ComkunFollowingStatsTotals returns platform-wide distinct-user totals for
// the same official listing set used by ListComkunFollowingStats.
func (s *TraderStore) ComkunFollowingStatsTotals(rows []ComkunFollowingStatsRow) (ComkunFollowingStatsTotals, error) {
	ids := make([]string, 0, len(rows))
	listings := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.StrategyID) == "" {
			continue
		}
		ids = append(ids, row.StrategyID)
		listings[row.StrategyID] = struct{}{}
	}
	if len(ids) == 0 {
		return ComkunFollowingStatsTotals{}, nil
	}
	now := time.Now().UTC()
	var entitlements []StrategyMarketEntitlement
	if err := s.db.Where("strategy_id IN ?", ids).Where("subscription_until IS NULL OR subscription_until > ?", now).Find(&entitlements).Error; err != nil {
		return ComkunFollowingStatsTotals{}, err
	}
	subscribedUsers := make(map[string]struct{}, len(entitlements))
	subscribedKeys := make(map[string]struct{}, len(entitlements))
	for _, entitlement := range entitlements {
		subscribedUsers[entitlement.UserID] = struct{}{}
		subscribedKeys[entitlement.UserID+"\x00"+entitlement.StrategyID] = struct{}{}
	}
	var running []struct {
		ID     string
		UserID string
		Config string
	}
	if err := s.db.Table("traders AS t").Select("t.id, t.user_id, COALESCE(s.config, '') AS config").Joins("INNER JOIN strategies AS s ON s.id = t.strategy_id").Joins("INNER JOIN exchanges AS e ON e.id = t.exchange_id").Where("t.is_running = ? AND e.enabled = ?", true, true).Find(&running).Error; err != nil {
		return ComkunFollowingStatsTotals{}, err
	}
	runningUsers := make(map[string]struct{})
	runningTraders := make(map[string]struct{})
	for _, item := range running {
		var cfg StrategyConfig
		if json.Unmarshal([]byte(item.Config), &cfg) != nil || !cfg.ComkunMarketFollow {
			continue
		}
		listingID := strings.TrimSpace(cfg.ComkunMarketListingStrategyID)
		if _, ok := listings[listingID]; !ok {
			continue
		}
		if _, ok := subscribedKeys[item.UserID+"\x00"+listingID]; !ok {
			continue
		}
		runningUsers[item.UserID] = struct{}{}
		runningTraders[item.ID] = struct{}{}
	}
	return ComkunFollowingStatsTotals{RunningFollowerCount: len(runningUsers), RunningTraderCount: len(runningTraders), SubscribedFollowerCount: len(subscribedUsers)}, nil
}
