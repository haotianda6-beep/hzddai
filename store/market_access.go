package store

import "strings"

// 策略市场权限（存库字段 market_access，小写英文）
const (
	MarketAccessOff          = "off"          // 不公开 / 不上架
	MarketAccessPrivate      = "private"      // 上架仅展示，不可复制
	MarketAccessSubscription = "subscription" // 需订阅
	MarketAccessPublic       = "public"       // 公开：可复制完整配置使用
	MarketAccessOpenSource   = "open_source"  // 开源：可复制提示词相关片段
)

// ValidMarketAccess 是否为允许的存库值
func ValidMarketAccess(v string) bool {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case MarketAccessOff, MarketAccessPrivate, MarketAccessSubscription, MarketAccessPublic, MarketAccessOpenSource:
		return true
	default:
		return false
	}
}

// IsListedOnMarket 是否出现在策略市场列表中
func IsListedOnMarket(access string) bool {
	a := strings.TrimSpace(strings.ToLower(access))
	return a != "" && a != MarketAccessOff
}

// SyncListingFlagsFromAccess 根据 market_access 同步旧字段 is_public / config_visible（兼容旧逻辑）
func SyncListingFlagsFromAccess(st *Strategy, access string) {
	a := strings.TrimSpace(strings.ToLower(access))
	if a == "" {
		a = MarketAccessOff
	}
	st.MarketAccess = a
	st.IsPublic = IsListedOnMarket(a)
	// 与策略市场 API 一致：仅「开源」对外展示完整配置；公开 / 需订阅不在市场暴露 JSON
	st.ConfigVisible = a == MarketAccessOpenSource
}

// EffectiveMarketAccess 读取时归一化（兼容未迁移的空字段）
func EffectiveMarketAccess(st *Strategy) string {
	if st == nil {
		return MarketAccessOff
	}
	if v := strings.TrimSpace(strings.ToLower(st.MarketAccess)); v != "" {
		if ValidMarketAccess(v) {
			return v
		}
		return MarketAccessOff
	}
	if st.IsPublic && st.ConfigVisible {
		return MarketAccessPublic
	}
	if st.IsPublic && !st.ConfigVisible {
		return MarketAccessSubscription
	}
	return MarketAccessOff
}

// EffectivePublicListingAccess 策略市场对外的权限档位。
func EffectivePublicListingAccess(st *Strategy) string {
	if st == nil {
		return MarketAccessOff
	}
	return EffectiveMarketAccess(st)
}

// IsVisibleOnPublicMarket 是否应出现在策略市场列表 API 中
func IsVisibleOnPublicMarket(st *Strategy) bool {
	if st == nil || st.IsDefault {
		return false
	}
	if IsListedOnMarket(EffectiveMarketAccess(st)) {
		return true
	}
	return false
}
