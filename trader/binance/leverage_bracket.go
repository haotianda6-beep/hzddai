package binance

import (
	"context"
	"strings"
	"time"

	"github.com/adshao/go-binance/v2/futures"
)

type leverageBracketCacheEntry struct {
	brackets  []futures.Bracket
	expiresAt time.Time
}

func leverageForTargetNotional(requested int, notional float64, brackets []futures.Bracket) int {
	if requested < 1 {
		requested = 1
	}
	allowed := requested
	for i, bracket := range brackets {
		matches := notional >= bracket.NotionalFloor && (bracket.NotionalCap <= 0 || notional < bracket.NotionalCap)
		if matches || i == len(brackets)-1 && notional >= bracket.NotionalFloor {
			if bracket.InitialLeverage > 0 && bracket.InitialLeverage < allowed {
				allowed = bracket.InitialLeverage
			}
			break
		}
	}
	return allowed
}

func isMaximumPositionAtLeverageError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "-2027") || strings.Contains(s, "maximum allowable position")
}

func IsMaximumPositionAtLeverageError(err error) bool {
	return isMaximumPositionAtLeverageError(err)
}

func (t *FuturesTrader) RecommendedLeverage(symbol string, requested int, targetNotional float64) (int, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	now := time.Now()
	t.settingsMu.RLock()
	cached, ok := t.leverageBracketCache[symbol]
	t.settingsMu.RUnlock()
	if ok && now.Before(cached.expiresAt) {
		return leverageForTargetNotional(requested, targetNotional, cached.brackets), nil
	}
	rows, err := t.client.NewGetLeverageBracketService().Symbol(symbol).
		Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))
	if err != nil {
		return requested, err
	}
	var brackets []futures.Bracket
	if len(rows) > 0 {
		brackets = rows[0].Brackets
	}
	t.settingsMu.Lock()
	if t.leverageBracketCache == nil {
		t.leverageBracketCache = make(map[string]leverageBracketCacheEntry)
	}
	t.leverageBracketCache[symbol] = leverageBracketCacheEntry{brackets: brackets, expiresAt: now.Add(5 * time.Minute)}
	t.settingsMu.Unlock()
	return leverageForTargetNotional(requested, targetNotional, brackets), nil
}
