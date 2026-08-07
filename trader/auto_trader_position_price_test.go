package trader

import "testing"

type positionPriceTrader struct{ Trader }

func (positionPriceTrader) GetPositions() ([]map[string]interface{}, error) {
	return []map[string]interface{}{{
		"symbol": "NXPCUSDT", "side": "long", "entryPrice": 0.2,
		"markPrice": 0.23, "positionAmt": 10.0, "unRealizedProfit": 0.3,
		"liquidationPrice": 0.0, "leverage": 10.0,
		"lastPrice": 0.225, "lastPriceTime": int64(1786090145999),
		"lastPriceSource": "binance_agg_trade_ws",
	}}, nil
}

func TestAutoTraderPositionsExposeLastTradeWithoutChangingMarkOrPnL(t *testing.T) {
	autoTrader := &AutoTrader{trader: positionPriceTrader{}}
	positions, err := autoTrader.GetPositions()
	if err != nil {
		t.Fatal(err)
	}
	position := positions[0]
	if position["last_price"] != 0.225 || position["last_price_time"] != int64(1786090145999) ||
		position["last_price_source"] != "binance_agg_trade_ws" {
		t.Fatalf("last trade fields=%#v", position)
	}
	if position["mark_price"] != 0.23 || position["unrealized_pnl"] != 0.3 {
		t.Fatalf("mark/PnL changed: %#v", position)
	}
}
