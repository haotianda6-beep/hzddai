package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"gorm.io/gorm"
)

// GetComkunFollowListingStrategyID 优先读环境变量，否则读库内种子写入的 system_config（与 COMKUN_FOLLOW_LISTING_STRATEGY_ID 对齐）
func GetComkunFollowListingStrategyID(s *Store) string {
	if v := strings.TrimSpace(os.Getenv("COMKUN_FOLLOW_LISTING_STRATEGY_ID")); v != "" {
		return v
	}
	if s == nil {
		return ""
	}
	v, err := s.GetSystemConfig("comkun_follow_listing_strategy_id")
	if err != nil || v == "" {
		return ""
	}
	return strings.TrimSpace(v)
}

// ApplyComkunFollowChildConfig 从「跟单开关」类市场源复制到用户侧策略时：打开 comkun 跟单并锁定主广播 source 为源策略 id
func ApplyComkunFollowChildConfig(s *Store, source *Strategy, buyerUserID string, cfg *StrategyConfig) {
	if cfg == nil || source == nil {
		return
	}
	envID := GetComkunFollowListingStrategyID(s)
	isSwitch := cfg.ComkunFollowListingTemplate || (envID != "" && strings.TrimSpace(source.ID) == envID)
	if !isSwitch {
		return
	}
	// 作者复制自己的上架模板做二次编辑时，不自动改成跟单子策略
	if strings.TrimSpace(buyerUserID) == strings.TrimSpace(source.UserID) {
		return
	}
	cfg.ComkunMarketFollow = true
	cfg.ComkunMarketSourceStrategyID = strings.TrimSpace(source.ID)
	cfg.ComkunFollowListingTemplate = false
	// 市场复制的跟单子策略：默认镜像跟单（按主控快照同步仓位/限价/止盈止损）
	cfg.ComkunFollowMirrorMasterExchange = true
	if cfg.ComkunMirrorFollowerMarginLeverage < 1 {
		cfg.ComkunMirrorFollowerMarginLeverage = 20
	}
	if cfg.ComkunMirrorMasterMarginLeverage < 1 {
		cfg.ComkunMirrorMasterMarginLeverage = 20
	}
}

func (s *Store) marketStrategyDuplicateAllowed(userID string, st *Strategy) bool {
	if st == nil {
		return false
	}
	if strings.TrimSpace(st.UserID) == strings.TrimSpace(userID) {
		return true
	}
	acc := EffectivePublicListingAccess(st)
	switch acc {
	case MarketAccessPublic, MarketAccessOpenSource:
		return true
	case MarketAccessSubscription, MarketAccessPrivate:
		ok, err := s.Billing().HasEntitlement(userID, st.ID)
		return err == nil && ok
	default:
		return false
	}
}

// DuplicateStrategy 复制策略：支持从本人/默认策略复制，或从已上架市场策略（公开/开源或已购）复制，并套用跟单开关字段
func (s *Store) DuplicateStrategy(userID, sourceID, newID, newName string) error {
	var source *Strategy
	src, err := s.Strategy().Get(userID, sourceID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("failed to get source strategy: %w", err)
		}
		ms, e2 := s.Strategy().GetByIDForMarket(sourceID)
		if e2 != nil {
			return fmt.Errorf("failed to get source strategy: %w", err)
		}
		if !s.marketStrategyDuplicateAllowed(userID, ms) {
			return fmt.Errorf("无权复制该市场策略，请先购买或使用公开/开源策略")
		}
		source = ms
	} else {
		source = src
	}

	cfg, err := source.ParseConfig()
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	if cfg.MarketPerformanceOnly {
		return fmt.Errorf("该策略为历史行情场景展示，尚未绑定实时主控，不能复制为交易策略")
	}
	sourceAccess := EffectivePublicListingAccess(source)
	isMarketCopy := strings.TrimSpace(source.UserID) != strings.TrimSpace(userID)
	contentLocked := isMarketCopy && sourceAccess != MarketAccessOpenSource
	ApplyComkunFollowChildConfig(s, source, userID, cfg)
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	newStrategy := &Strategy{
		ID:                 newID,
		UserID:             userID,
		Name:               newName,
		Description:        "Created based on [" + source.Name + "]",
		IsActive:           false,
		IsDefault:          false,
		Config:             string(raw),
		MarketAccess:       MarketAccessOff,
		ShowAfterRename:    false,
		IsPublic:           false,
		ConfigVisible:      !contentLocked,
		SourceStrategyID:   source.ID,
		SourceMarketAccess: sourceAccess,
		ContentLocked:      contentLocked,
		MarketRevision:     0,
	}
	return s.Strategy().Create(newStrategy)
}
