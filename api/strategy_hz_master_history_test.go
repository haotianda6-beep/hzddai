package api

import (
	"testing"
	"time"

	"nofx/store"
)

func TestHZMasterHistoryUsesHighestRatioFollowerWithoutScaling(t *testing.T) {
	strategyID := store.HZMasterSourcePrefix + "test"
	refs := []store.MarketStrategyTraderRef{
		{TraderID: "half", StrategyID: "copy-half", ExchangeType: "hz", CreatedAt: time.Unix(1, 0),
			Config: `{"comkun_market_follow":true,"comkun_market_source_strategy_id":"hz-ai-master:test","comkun_mirror_follower_equity_ratio":0.5}`},
		{TraderID: "full", StrategyID: "copy-full", ExchangeType: "hz", CreatedAt: time.Unix(2, 0),
			Config: `{"comkun_market_follow":true,"comkun_market_source_strategy_id":"hz-ai-master:test","comkun_mirror_follower_equity_ratio":1}`},
	}
	if got := hzMasterHistoryTraderID(strategyID, refs); got != "full" {
		t.Fatalf("representative trader=%q, want full", got)
	}
	row := hzMasterTradeHistoryRow(&store.TraderPosition{
		ID: 7, Symbol: "XAUUSD", Side: "LONG", EntryQuantity: 1,
		EntryPrice: 2400, ExitPrice: 2410, RealizedPnL: 10,
		EntryTime: time.Unix(1, 0).UnixMilli(), ExitTime: time.Unix(2, 0).UnixMilli(),
	}, &store.Trader{IsCrossMargin: true}, 0)
	if got := row["closingPnl"]; got != "+10.00 USDT" {
		t.Fatalf("closingPnl=%v, want unscaled +10.00 USDT", got)
	}
	if got := row["maxOpenInterest"]; got != "1.000 XAUUSD" {
		t.Fatalf("maxOpenInterest=%v, want unscaled 1.000 XAUUSD", got)
	}
}
