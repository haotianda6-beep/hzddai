package store

import (
	"strings"
	"testing"
)

func TestValidateStrategyExchangeHZBinding(t *testing.T) {
	hz := &StrategyConfig{
		StrategyType: "ai_trading",
		CoinSource: CoinSourceConfig{
			SourceType:  "static",
			StaticCoins: []string{"XAUUSD", "XAGUSD", "WTIUSD"},
		},
		RiskControl: RiskControlConfig{
			BTCETHMaxLeverage:  500,
			AltcoinMaxLeverage: 500,
		},
	}
	if err := ValidateStrategyExchange("hz", hz); err != nil {
		t.Fatalf("valid HZ binding rejected: %v", err)
	}
	if err := ValidateStrategyExchange("binance", hz); err == nil || !strings.Contains(err.Error(), "HZ 交易账户") {
		t.Fatalf("HZ strategy should reject crypto exchange: %v", err)
	}

	crypto := GetDefaultStrategyConfig("zh")
	if err := ValidateStrategyExchange("hz", &crypto); err == nil || !strings.Contains(err.Error(), "黄金、白银或原油") {
		t.Fatalf("crypto strategy should reject HZ exchange: %v", err)
	}
}

func TestValidateStrategyExchangeHZLeverage(t *testing.T) {
	config := &StrategyConfig{
		CoinSource: CoinSourceConfig{SourceType: "static", StaticCoins: []string{"XAUUSD"}},
		RiskControl: RiskControlConfig{
			BTCETHMaxLeverage:  20,
			AltcoinMaxLeverage: 20,
		},
	}
	if err := ValidateStrategyExchange("hz", config); err == nil || !strings.Contains(err.Error(), "100 到 2000") {
		t.Fatalf("invalid HZ leverage should be rejected: %v", err)
	}
}
