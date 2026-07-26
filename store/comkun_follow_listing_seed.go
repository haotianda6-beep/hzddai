package store

import (
	"encoding/json"
	"strings"

	"nofx/logger"

	"github.com/google/uuid"
)

const (
	comkunFollowListingOwnerEmail          = "haotianda6@gmail.com"
	systemConfigComkunFollowListingSeeded  = "comkun_follow_listing_seeded_strategy_id"
	systemConfigComkunFollowListingStratID = "comkun_follow_listing_strategy_id"
)

// EnsureComkunFollowListingSeed 为指定邮箱用户上架「跟单开关」官方模板策略（仅首次写入 system_config 标记后跳过）
func (s *Store) EnsureComkunFollowListingSeed() error {
	existing, err := s.GetSystemConfig(systemConfigComkunFollowListingSeeded)
	if err != nil {
		return err
	}
	if strings.TrimSpace(existing) != "" {
		return nil
	}
	u, err := s.User().GetByEmail(comkunFollowListingOwnerEmail)
	if err != nil {
		logger.Warnf("comkun follow listing seed: user %s not found, skip: %v", comkunFollowListingOwnerEmail, err)
		return nil
	}
	cfg := GetDefaultStrategyConfig("zh")
	cfg.ComkunFollowListingTemplate = true
	cfg.ComkunMarketFollow = false
	cfg.ComkunMarketSourceStrategyID = ""
	cfg.MarketSalePriceUSDT = 0
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	id := uuid.New().String()
	st := &Strategy{
		ID:              id,
		UserID:          u.ID,
		Name:            "COMKUN 跟单开关（官方）",
		Description:     "策略市场官方跟单开关模板；复制后自动绑定主广播源。",
		IsActive:        false,
		IsDefault:       false,
		Config:          string(raw),
		ShowAfterRename: false,
		MarketRevision:  0,
	}
	SyncListingFlagsFromAccess(st, MarketAccessSubscription)
	if err := s.Strategy().Create(st); err != nil {
		return err
	}
	if err := s.SetSystemConfig(systemConfigComkunFollowListingSeeded, id); err != nil {
		return err
	}
	if err := s.SetSystemConfig(systemConfigComkunFollowListingStratID, id); err != nil {
		return err
	}
	logger.Infof("✅ comkun follow listing strategy seeded id=%s owner=%s", id, comkunFollowListingOwnerEmail)
	return nil
}
