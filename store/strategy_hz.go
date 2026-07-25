package store

import (
	"fmt"
	"strings"
)

var hzInstruments = map[string]bool{
	"XAUUSD": true,
	"XAGUSD": true,
	"WTIUSD": true,
}

// IsHZStrategy identifies the commodity-only strategy used by an HZ trading account.
func IsHZStrategy(config *StrategyConfig) bool {
	if config == nil || config.StrategyType == "grid_trading" ||
		!strings.EqualFold(config.CoinSource.SourceType, "static") ||
		len(config.CoinSource.StaticCoins) == 0 {
		return false
	}
	for _, symbol := range config.CoinSource.StaticCoins {
		if !hzInstruments[strings.ToUpper(strings.TrimSpace(symbol))] {
			return false
		}
	}
	return true
}

// ValidateStrategyExchange keeps HZ commodity strategies isolated from crypto accounts.
func ValidateStrategyExchange(exchangeType string, config *StrategyConfig) error {
	hzStrategy := IsHZStrategy(config)
	hzExchange := strings.EqualFold(exchangeType, "hz")
	if hzStrategy && !hzExchange {
		return fmt.Errorf("黄金、白银、原油策略只能连接 HZ 交易账户")
	}
	if hzExchange && !hzStrategy {
		return fmt.Errorf("HZ 交易账户只能使用包含黄金、白银或原油的静态 AI 策略")
	}
	if hzExchange && (config.RiskControl.BTCETHMaxLeverage < 100 ||
		config.RiskControl.BTCETHMaxLeverage > 2000 ||
		config.RiskControl.AltcoinMaxLeverage < 100 ||
		config.RiskControl.AltcoinMaxLeverage > 2000) {
		return fmt.Errorf("HZ 策略杠杆需要在 100 到 2000 倍之间")
	}
	return nil
}
