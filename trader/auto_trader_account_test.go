package trader

import "testing"

type authoritativeAccountTrader struct{ Trader }

func (authoritativeAccountTrader) GetBalance() (map[string]interface{}, error) {
	return map[string]interface{}{
		"totalWalletBalance":    1000.0,
		"totalEquity":           1100.0,
		"totalUnrealizedProfit": 100.0,
		"availableBalance":      880.0,
		"usedMargin":            220.0,
	}, nil
}

func (authoritativeAccountTrader) GetPositions() ([]map[string]interface{}, error) {
	return []map[string]interface{}{}, nil
}

func TestAutoTraderAccountUsesExchangeUsedMargin(t *testing.T) {
	autoTrader := &AutoTrader{
		trader:         authoritativeAccountTrader{},
		initialBalance: 1000,
	}

	account, err := autoTrader.GetAccountInfo()
	if err != nil {
		t.Fatal(err)
	}
	if got := account["margin_used"]; got != 220.0 {
		t.Fatalf("margin_used=%v, want exchange value 220", got)
	}
	if got := account["margin_used_pct"]; got != 20.0 {
		t.Fatalf("margin_used_pct=%v, want 20", got)
	}
}
