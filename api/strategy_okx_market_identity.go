package api

import "github.com/gin-gonic/gin"

type okxScreenMarketIdentity struct {
	Name    string
	Creator string
	Seed    string
}

var okxScreenMarketDisplayExchanges = map[string]string{
	okx01MarketStrategyID: "BINANCE",
	okx02MarketStrategyID: "BYBIT",
	okx03MarketStrategyID: "GATE",
	okx04MarketStrategyID: "BITGET",
	okx05MarketStrategyID: "OKX",
	okx06MarketStrategyID: "BINANCE",
	okx07MarketStrategyID: "BYBIT",
	okx08MarketStrategyID: "GATE",
}

var okxScreenMarketIdentities = map[string]okxScreenMarketIdentity{
	okx01MarketStrategyID: {
		Name:    "大饼杀手",
		Creator: "林知远",
		Seed:    "okx-screen-mirror-01-lin-zhiyuan",
	},
	okx02MarketStrategyID: {
		Name:    "主流货币+生态币",
		Creator: "顾北辰",
		Seed:    "okx-screen-mirror-02-gu-beichen",
	},
	okx03MarketStrategyID: {
		Name:    "生态币轻松拿捏",
		Creator: "沈清禾",
		Seed:    "okx-screen-mirror-03-shen-qinghe",
	},
	okx04MarketStrategyID: {
		Name:    "AI数据500源策略",
		Creator: "陆星野",
		Seed:    "okx-screen-mirror-04-lu-xingye",
	},
	"okx-screen-mirror-05": {
		Name:    "老天会眷顾我的",
		Creator: "周既白",
		Seed:    "okx-screen-mirror-05-zhou-jibai",
	},
	"okx-screen-mirror-06": {
		Name:    "交易宝爸闯荡币圈",
		Creator: "许云深",
		Seed:    "okx-screen-mirror-06-xu-yunshen",
	},
	"okx-screen-mirror-07": {
		Name:    "我们意念合一",
		Creator: "程观澜",
		Seed:    "okx-screen-mirror-07-cheng-guanlan",
	},
	"okx-screen-mirror-08": {
		Name:    "稳健复利投入",
		Creator: "韩予安",
		Seed:    "okx-screen-mirror-08-han-yuan",
	},
}

func applyOkxScreenMarketIdentityOverlay(item gin.H, strategyID string) {
	identity, ok := okxScreenMarketIdentities[strategyID]
	if !ok {
		return
	}
	item["name"] = identity.Name
	item["creator_display_name"] = identity.Creator
	item["creator_avatar_url"] = "https://api.dicebear.com/7.x/avataaars/svg?seed=" + identity.Seed
	applyOkxScreenMarketDisplayExchangeOverlay(item, strategyID)
}

func okxScreenMarketDisplayExchange(strategyID string) string {
	if v, ok := okxScreenMarketDisplayExchanges[strategyID]; ok {
		return v
	}
	return ""
}

func applyOkxScreenMarketDisplayExchangeOverlay(item gin.H, strategyID string) string {
	exchange := okxScreenMarketDisplayExchange(strategyID)
	if exchange == "" {
		return ""
	}
	item["exchange_type"] = exchange
	return exchange
}
