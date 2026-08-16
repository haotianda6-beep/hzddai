package store

import (
	"strings"
	"testing"
)

func TestValidateStrategyExchangeHZBinding(t *testing.T) {
	hz := &StrategyConfig{
		StrategyType:                 "ai_trading",
		ComkunMarketFollow:           true,
		ComkunMarketSourceStrategyID: HZMasterSourceStrategyID("master-1"),
		CoinSource:                   CoinSourceConfig{SourceType: "static", StaticCoins: []string{"BTC-PERP"}},
	}
	if err := ValidateStrategyExchange("hz", hz); err != nil {
		t.Fatalf("valid HZ binding rejected: %v", err)
	}
	if err := ValidateStrategyExchange("binance", hz); err == nil || !strings.Contains(err.Error(), "BALIB 交易账户") {
		t.Fatalf("HZ strategy should reject crypto exchange: %v", err)
	}

	dynamic := GetDefaultStrategyConfig("zh")
	dynamic.CoinSource = CoinSourceConfig{SourceType: "static", StaticCoins: []string{"EURUSD", "BTC-PERP"}}
	if err := ValidateStrategyExchange("hz", &dynamic); err != nil {
		t.Fatalf("dynamic HZ instruments should be accepted: %v", err)
	}
}

func TestValidateStrategyExchangeHZLeverageComesFromB(t *testing.T) {
	config := &StrategyConfig{
		CoinSource: CoinSourceConfig{SourceType: "static", StaticCoins: []string{"XAUUSD"}},
		RiskControl: RiskControlConfig{
			BTCETHMaxLeverage:  20,
			AltcoinMaxLeverage: 20,
		},
	}
	if err := ValidateStrategyExchange("hz", config); err != nil {
		t.Fatalf("HZ leverage should not use the removed static range: %v", err)
	}
}
