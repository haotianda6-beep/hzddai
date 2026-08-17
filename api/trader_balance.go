package api

import (
	"fmt"
	"math"
)

// resolveTraderInitialBalance accepts only a fresh, positive remote equity.
// A failed or missing account response is unknown, never a user-supplied zero.
func resolveTraderInitialBalance(exchangeType string, _ float64, balanceInfo map[string]interface{}, balanceErr error) (float64, error) {
	if balanceErr != nil {
		return 0, fmt.Errorf("%s account balance unavailable: %w", accountAssetForExchange(exchangeType), balanceErr)
	}
	value, found := extractExchangeTotalEquity(balanceInfo)
	if !found {
		return 0, fmt.Errorf("%s account equity was not returned", accountAssetForExchange(exchangeType))
	}
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("%s account equity is zero or invalid", accountAssetForExchange(exchangeType))
	}
	return value, nil
}
