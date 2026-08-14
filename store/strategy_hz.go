package store

import (
	"fmt"
	"strings"
)

// IsHZStrategy identifies a strategy fed by the isolated HZ AI master source.
// Instrument validity belongs to B's dynamic /instruments contract.
func IsHZStrategy(config *StrategyConfig) bool {
	return config != nil && config.ComkunMarketFollow &&
		strings.HasPrefix(strings.TrimSpace(config.ComkunMarketSourceStrategyID), HZMasterSourcePrefix)
}

// ValidateStrategyExchange keeps HZ commodity strategies isolated from crypto accounts.
func ValidateStrategyExchange(exchangeType string, config *StrategyConfig) error {
	if config != nil && config.MarketPerformanceOnly {
		return fmt.Errorf("历史行情场景策略仅供业绩展示，尚未绑定实时主控，不能创建交易员")
	}
	hzStrategy := IsHZStrategy(config)
	hzExchange := strings.EqualFold(exchangeType, "hz")
	if hzStrategy && !hzExchange {
		return fmt.Errorf("黄金、白银、原油策略只能连接 HZ 交易账户")
	}
	// HZ instruments and contract rules are validated against B at binding and
	// execution time; keeping a second static allow-list here causes drift.
	return nil
}
