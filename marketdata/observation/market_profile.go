package observation

import "strings"

// MarketProfile contains the product-facing identity for one verified history
// slot. Historical provenance remains in History; live routing is supplied
// separately through COMKUN_OBSERVER_LIVE_MASTER_IDS.
type MarketProfile struct {
	Slot               int
	Name               string
	Description        string
	CreatorDisplayName string
	CreatorAvatarURL   string
	MonthlyPriceUSDT   float64
}

var marketProfiles = []MarketProfile{
	{1, "稳曜量化", "多资产稳健配置，重视回撤控制与持续性。", "林澈", "https://api.dicebear.com/7.x/avataaars/svg?seed=lin-che-balib", 100},
	{2, "潮生计划", "顺应市场节奏切换仓位，兼顾趋势与风险预算。", "周屿", "https://api.dicebear.com/7.x/avataaars/svg?seed=zhou-yu-balib", 200},
	{3, "弧光趋势", "捕捉多品种趋势机会，以纪律化规则控制波动。", "顾川", "https://api.dicebear.com/7.x/avataaars/svg?seed=gu-chuan-balib", 300},
	{4, "星轨资产", "基于跨品种轮动与分散配置管理组合风险。", "沈星河", "https://api.dicebear.com/7.x/avataaars/svg?seed=shen-xinghe-balib", 500},
	{5, "峻峰资本", "聚焦强势行情与风险收益比，追求稳健复利。", "江峻", "https://api.dicebear.com/7.x/avataaars/svg?seed=jiang-jun-balib", 800},
	{6, "脉冲矩阵", "以高响应信号捕捉短周期机会并动态管理敞口。", "陆遥", "https://api.dicebear.com/7.x/avataaars/svg?seed=lu-yao-balib", 1000},
}

func MarketProfileBySlot(slot int) (MarketProfile, bool) {
	if slot < 1 || slot > len(marketProfiles) {
		return MarketProfile{}, false
	}
	return marketProfiles[slot-1], true
}

func MarketProfileByStrategyID(strategyID string) (MarketProfile, bool) {
	slot, ok := ByStrategyID(strings.TrimSpace(strategyID))
	if !ok {
		return MarketProfile{}, false
	}
	return MarketProfileBySlot(slot.Slot)
}
