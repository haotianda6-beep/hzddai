package binance

import (
	"context"
	"fmt"
	"math"
	"nofx/logger"
	"strconv"
	"strings"
	"time"

	"github.com/adshao/go-binance/v2/futures"
)

// positionsCacheTTL 持仓 REST 缓存时长（可与余额分开）；测试环境 cache 全为 0 时返回 0 表示禁用缓存命中。
func (t *FuturesTrader) positionsCacheTTL() time.Duration {
	if t.positionsCacheDuration > 0 {
		return t.positionsCacheDuration
	}
	if t.cacheDuration > 0 {
		return t.cacheDuration
	}
	return 0
}

func isTooManyRequestsBinance(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "-1003") || strings.Contains(strings.ToLower(s), "too many requests")
}

// InvalidatePositionsCache 清空持仓缓存；主控写跟单广播前调用，避免限价刚成交后 15s 内快照仍无仓位导致被控误平。
func (t *FuturesTrader) InvalidatePositionsCache() {
	t.positionsCacheMutex.Lock()
	t.cachedPositions = nil
	t.positionsCacheTime = time.Time{}
	t.positionsCacheMutex.Unlock()
}

// GetPositions gets all positions (with cache)
func (t *FuturesTrader) GetPositions() ([]map[string]interface{}, error) {
	posTTL := t.positionsCacheTTL()
	// First check if cache is valid
	t.positionsCacheMutex.RLock()
	if posTTL > 0 && t.cachedPositions != nil && time.Since(t.positionsCacheTime) < posTTL {
		cacheAge := time.Since(t.positionsCacheTime)
		t.positionsCacheMutex.RUnlock()
		logger.Infof("✓ Using cached position information (cache age: %.1f seconds ago)", cacheAge.Seconds())
		return t.cachedPositions, nil
	}
	t.positionsCacheMutex.RUnlock()

	// Cache expired or doesn't exist, call API
	if t.isRateLimited() && t.cachedPositions != nil {
		logger.Infof("✓ Using cached positions during Binance rate-limit cooldown")
		return t.cachedPositions, nil
	}
	logger.Infof("🔄 Cache expired, calling Binance API to get position information...")
	var positions []*futures.PositionRisk
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			wait := time.Duration(400+attempt*500) * time.Millisecond
			logger.Warnf("⚠️ Binance positionRisk congested, retry in %v (attempt %d/3)", wait, attempt+1)
			time.Sleep(wait)
		}
		positions, err = t.client.NewGetPositionRiskService().Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))
		if err == nil {
			break
		}
		t.markRateLimited(err)
		if !isTooManyRequestsBinance(err) {
			break
		}
	}
	if err != nil {
		if t.cachedPositions != nil {
			logger.Warnf("⚠️ Binance positions API failed, returning stale cache: %v", err)
			return t.cachedPositions, nil
		}
		return nil, fmt.Errorf("failed to get positions: %w", err)
	}

	var result []map[string]interface{}
	for _, pos := range positions {
		posAmt, _ := strconv.ParseFloat(pos.PositionAmt, 64)
		if posAmt == 0 {
			continue // Skip positions with zero amount
		}

		posMap := make(map[string]interface{})
		posMap["symbol"] = pos.Symbol
		posMap["positionAmt"], _ = strconv.ParseFloat(pos.PositionAmt, 64)
		posMap["entryPrice"], _ = strconv.ParseFloat(pos.EntryPrice, 64)
		posMap["markPrice"], _ = strconv.ParseFloat(pos.MarkPrice, 64)
		posMap["unRealizedProfit"], _ = strconv.ParseFloat(pos.UnRealizedProfit, 64)
		posMap["leverage"], _ = strconv.ParseFloat(pos.Leverage, 64)
		posMap["liquidationPrice"], _ = strconv.ParseFloat(pos.LiquidationPrice, 64)
		// Note: Binance SDK doesn't expose updateTime field, will fallback to local tracking

		// 双向持仓：positionAmt 多为正数，必须用 positionSide 区分多空；单向持仓仍可用正负号。
		ps := strings.ToUpper(strings.TrimSpace(pos.PositionSide))
		switch ps {
		case "LONG":
			posMap["side"] = "long"
			posMap["positionAmt"] = math.Abs(posAmt)
		case "SHORT":
			posMap["side"] = "short"
			posMap["positionAmt"] = -math.Abs(posAmt)
		default:
			if posAmt > 0 {
				posMap["side"] = "long"
			} else {
				posMap["side"] = "short"
			}
		}
		posMap["positionSide"] = strings.TrimSpace(pos.PositionSide)

		result = append(result, posMap)
	}

	// Update cache
	t.positionsCacheMutex.Lock()
	t.cachedPositions = result
	t.positionsCacheTime = time.Now()
	t.positionsCacheMutex.Unlock()

	return result, nil
}

// SetMarginMode sets margin mode
func (t *FuturesTrader) SetMarginMode(symbol string, isCrossMargin bool) error {
	t.settingsMu.RLock()
	cachedMode, cached := t.marginModeBySymbol[symbol]
	t.settingsMu.RUnlock()
	if cached && cachedMode == isCrossMargin {
		return nil
	}
	cacheMode := func() {
		t.settingsMu.Lock()
		if t.marginModeBySymbol == nil {
			t.marginModeBySymbol = make(map[string]bool)
		}
		t.marginModeBySymbol[symbol] = isCrossMargin
		t.settingsMu.Unlock()
	}
	var marginType futures.MarginType
	if isCrossMargin {
		marginType = futures.MarginTypeCrossed
	} else {
		marginType = futures.MarginTypeIsolated
	}

	// Try to set margin mode
	err := t.client.NewChangeMarginTypeService().
		Symbol(symbol).
		MarginType(marginType).
		Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

	marginModeStr := "Cross Margin"
	if !isCrossMargin {
		marginModeStr = "Isolated Margin"
	}

	if err != nil {
		// If error message contains "No need to change", margin mode is already set to target value
		if contains(err.Error(), "No need to change margin type") {
			cacheMode()
			logger.Infof("  ✓ %s margin mode is already %s", symbol, marginModeStr)
			return nil
		}
		// If there is an open position, margin mode cannot be changed, but this doesn't affect trading
		if contains(err.Error(), "Margin type cannot be changed if there exists position") {
			cacheMode()
			logger.Infof("  ⚠️ %s has open positions, cannot change margin mode, continuing with current mode", symbol)
			return nil
		}
		// Detect Multi-Assets mode (error code -4168)
		if contains(err.Error(), "Multi-Assets mode") || contains(err.Error(), "-4168") || contains(err.Error(), "4168") {
			cacheMode()
			logger.Infof("  ⚠️ %s detected Multi-Assets mode, forcing Cross Margin mode", symbol)
			logger.Infof("  💡 Tip: To use Isolated Margin mode, please disable Multi-Assets mode in Binance")
			return nil
		}
		// Detect Unified Account API (Portfolio Margin)
		if contains(err.Error(), "unified") || contains(err.Error(), "portfolio") || contains(err.Error(), "Portfolio") {
			logger.Infof("  ❌ %s detected Unified Account API, unable to trade futures", symbol)
			return fmt.Errorf("please use 'Spot & Futures Trading' API permission, do not use 'Unified Account API'")
		}
		if isBinanceAPIAuthOrWhitelistError(err) {
			return fmt.Errorf("set margin mode auth/whitelist error: %w", err)
		}
		logger.Infof("  ⚠️ Failed to set margin mode: %v", err)
		// Don't return error, let trading continue
		return nil
	}

	logger.Infof("  ✓ %s margin mode set to %s", symbol, marginModeStr)
	cacheMode()
	return nil
}

// SetLeverage sets leverage (with smart detection and cooldown period)
func (t *FuturesTrader) SetLeverage(symbol string, leverage int) error {
	t.settingsMu.RLock()
	cachedLeverage := t.leverageBySymbol[symbol]
	t.settingsMu.RUnlock()
	if cachedLeverage == leverage && leverage > 0 {
		return nil
	}
	cacheLeverage := func() {
		t.settingsMu.Lock()
		if t.leverageBySymbol == nil {
			t.leverageBySymbol = make(map[string]int)
		}
		t.leverageBySymbol[symbol] = leverage
		t.settingsMu.Unlock()
	}
	// First try to get current leverage (from position information)
	currentLeverage := 0
	positions, err := t.GetPositions()
	if err == nil {
		for _, pos := range positions {
			if pos["symbol"] == symbol {
				if lev, ok := pos["leverage"].(float64); ok {
					currentLeverage = int(lev)
					break
				}
			}
		}
	}

	// If current leverage is already the target leverage, skip
	if currentLeverage == leverage && currentLeverage > 0 {
		cacheLeverage()
		logger.Infof("  ✓ %s leverage is already %dx, no need to change", symbol, leverage)
		return nil
	}

	// Change leverage
	_, err = t.client.NewChangeLeverageService().
		Symbol(symbol).
		Leverage(leverage).
		Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

	if err != nil {
		// If error message contains "No need to change", leverage is already the target value
		if contains(err.Error(), "No need to change") {
			cacheLeverage()
			logger.Infof("  ✓ %s leverage is already %dx", symbol, leverage)
			return nil
		}
		return fmt.Errorf("failed to set leverage: %w", err)
	}

	logger.Infof("  ✓ %s leverage changed to %dx", symbol, leverage)
	cacheLeverage()

	return nil
}

// GetMarketPrice gets market price
func (t *FuturesTrader) GetMarketPrice(symbol string) (float64, error) {
	prices, err := t.client.NewListPricesService().Symbol(symbol).Do(context.Background())
	if err != nil {
		return 0, fmt.Errorf("failed to get price: %w", err)
	}

	if len(prices) == 0 {
		return 0, fmt.Errorf("price not found")
	}

	price, err := strconv.ParseFloat(prices[0].Price, 64)
	if err != nil {
		return 0, err
	}

	return price, nil
}

// CalculatePositionSize calculates position size
func (t *FuturesTrader) CalculatePositionSize(balance, riskPercent, price float64, leverage int) float64 {
	riskAmount := balance * (riskPercent / 100.0)
	positionValue := riskAmount * float64(leverage)
	quantity := positionValue / price
	return quantity
}

// GetMinNotional gets minimum notional value (Binance requirement)
func (t *FuturesTrader) GetMinNotional(symbol string) float64 {
	// Use conservative default value of 10 USDT to ensure order passes exchange validation
	return 10.0
}

// CheckMinNotional checks if order meets minimum notional value requirement
func (t *FuturesTrader) CheckMinNotional(symbol string, quantity float64) error {
	price, err := t.GetMarketPrice(symbol)
	if err != nil {
		return fmt.Errorf("failed to get market price: %w", err)
	}

	notionalValue := quantity * price
	minNotional := t.GetMinNotional(symbol)

	if notionalValue < minNotional {
		return fmt.Errorf(
			"order amount %.2f USDT is below minimum requirement %.2f USDT (quantity: %.4f, price: %.4f)",
			notionalValue, minNotional, quantity, price,
		)
	}

	return nil
}

// GetSymbolPrecision gets the quantity precision for a trading pair
func (t *FuturesTrader) GetSymbolPrecision(symbol string) (int, error) {
	exchangeInfo, err := t.client.NewExchangeInfoService().Do(context.Background())
	if err != nil {
		return 0, fmt.Errorf("failed to get trading rules: %w", err)
	}

	for _, s := range exchangeInfo.Symbols {
		if s.Symbol == symbol {
			// Get precision from LOT_SIZE filter
			for _, filter := range s.Filters {
				if filter["filterType"] == "LOT_SIZE" {
					stepSize := filter["stepSize"].(string)
					precision := calculatePrecision(stepSize)
					logger.Infof("  %s quantity precision: %d (stepSize: %s)", symbol, precision, stepSize)
					return precision, nil
				}
			}
		}
	}

	logger.Infof("  ⚠ %s precision information not found, using default precision 3", symbol)
	return 3, nil // Default precision is 3
}

// GetLotStepSize returns LOT_SIZE stepSize for quantity increments.
func (t *FuturesTrader) GetLotStepSize(symbol string) float64 {
	exchangeInfo, err := t.client.NewExchangeInfoService().Do(context.Background())
	if err != nil {
		return 0.001
	}
	for _, s := range exchangeInfo.Symbols {
		if s.Symbol != symbol {
			continue
		}
		for _, filter := range s.Filters {
			if filter["filterType"] != "LOT_SIZE" {
				continue
			}
			stepSize, ok := filter["stepSize"].(string)
			if !ok || stepSize == "" {
				break
			}
			step, err := strconv.ParseFloat(stepSize, 64)
			if err != nil || step <= 0 {
				break
			}
			return step
		}
	}
	return 0.001
}

// FormatQuantity formats quantity to correct precision
func (t *FuturesTrader) FormatQuantity(symbol string, quantity float64) (string, error) {
	precision, err := t.GetSymbolPrecision(symbol)
	if err != nil {
		// If retrieval fails, use default format
		return fmt.Sprintf("%.3f", quantity), nil
	}

	format := fmt.Sprintf("%%.%df", precision)
	return fmt.Sprintf(format, quantity), nil
}

// GetSymbolPricePrecision gets the price precision for a trading pair
func (t *FuturesTrader) GetSymbolPricePrecision(symbol string) (int, error) {
	exchangeInfo, err := t.client.NewExchangeInfoService().Do(context.Background())
	if err != nil {
		return 0, fmt.Errorf("failed to get trading rules: %w", err)
	}

	for _, s := range exchangeInfo.Symbols {
		if s.Symbol == symbol {
			// Get precision from PRICE_FILTER filter
			for _, filter := range s.Filters {
				if filter["filterType"] == "PRICE_FILTER" {
					tickSize := filter["tickSize"].(string)
					precision := calculatePrecision(tickSize)
					return precision, nil
				}
			}
		}
	}

	// Default to 2 decimal places for price
	return 2, nil
}

// FormatPrice formats price to correct precision
func (t *FuturesTrader) FormatPrice(symbol string, price float64) (string, error) {
	precision, err := t.GetSymbolPricePrecision(symbol)
	if err != nil {
		// If retrieval fails, use default format
		return fmt.Sprintf("%.2f", price), nil
	}

	format := fmt.Sprintf("%%.%df", precision)
	return fmt.Sprintf(format, price), nil
}
