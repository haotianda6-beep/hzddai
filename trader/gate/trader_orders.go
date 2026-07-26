package gate

import (
	"fmt"
	"math"
	"nofx/logger"
	"nofx/trader/types"
	"strconv"
	"strings"

	"github.com/antihax/optional"
	"github.com/gateio/gateapi-go/v6"
)

func gateContractSize(quantity float64, contract *gateapi.Contract) (int64, error) {
	if contract == nil || quantity <= 0 || math.IsNaN(quantity) || math.IsInf(quantity, 0) {
		return 0, fmt.Errorf("invalid Gate contract quantity: %.12f", quantity)
	}
	multiplier, err := strconv.ParseFloat(contract.QuantoMultiplier, 64)
	if err != nil || multiplier <= 0 {
		return 0, fmt.Errorf("invalid Gate quanto_multiplier %q", contract.QuantoMultiplier)
	}
	size := int64(math.Round(quantity / multiplier))
	minimum := contract.OrderSizeMin
	if minimum < 1 {
		minimum = 1
	}
	if size < minimum {
		return 0, fmt.Errorf("Gate quantity %.12f is below minimum %d contracts", quantity, minimum)
	}
	if contract.OrderSizeMax > 0 && size > contract.OrderSizeMax {
		return 0, fmt.Errorf("Gate quantity %.12f exceeds maximum %d contracts", quantity, contract.OrderSizeMax)
	}
	return size, nil
}

// SetLeverage sets the leverage for a symbol
func (t *GateTrader) SetLeverage(symbol string, leverage int) error {
	symbol = t.convertSymbol(symbol)

	apiLeverage := fmt.Sprintf("%d", leverage)
	var opts *gateapi.UpdatePositionLeverageOpts
	if t.isCrossMarginSymbol(symbol) {
		apiLeverage = "0"
		opts = &gateapi.UpdatePositionLeverageOpts{
			CrossLeverageLimit: optional.NewString(fmt.Sprintf("%d", leverage)),
		}
	}

	_, _, err := t.client.FuturesApi.UpdatePositionLeverage(t.ctx, "usdt", symbol, apiLeverage, opts)
	if err != nil {
		// Gate.io may return error if leverage is already set
		if strings.Contains(err.Error(), "RISK_LIMIT_EXCEEDED") {
			logger.Warnf("  [Gate] Leverage %d exceeds limit for %s", leverage, symbol)
			return nil
		}
		return fmt.Errorf("failed to set leverage: %w", err)
	}

	logger.Infof("  [Gate] Leverage set to %dx for %s (cross=%v)", leverage, symbol, t.isCrossMarginSymbol(symbol))
	return nil
}

// SetMarginMode sets margin mode (cross or isolated)
func (t *GateTrader) SetMarginMode(symbol string, isCrossMargin bool) error {
	symbol = t.convertSymbol(symbol)
	t.marginModeMutex.Lock()
	if t.marginModeBySymbol == nil {
		t.marginModeBySymbol = make(map[string]bool)
	}
	t.marginModeBySymbol[symbol] = isCrossMargin
	t.marginModeMutex.Unlock()
	logger.Infof("  [Gate] Margin mode set for %s: cross=%v", symbol, isCrossMargin)
	return nil
}

func (t *GateTrader) isCrossMarginSymbol(symbol string) bool {
	symbol = t.convertSymbol(symbol)
	t.marginModeMutex.RLock()
	defer t.marginModeMutex.RUnlock()
	if t.marginModeBySymbol == nil {
		return true
	}
	isCross, ok := t.marginModeBySymbol[symbol]
	if !ok {
		return true
	}
	return isCross
}

// OpenLong opens a long position
func (t *GateTrader) OpenLong(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	symbol = t.convertSymbol(symbol)

	// Cancel old orders first
	t.CancelAllOrders(symbol)

	// Set leverage
	if err := t.SetLeverage(symbol, leverage); err != nil {
		logger.Warnf("  [Gate] Failed to set leverage: %v", err)
	}

	// Get contract info for size calculation
	contract, err := t.getContract(symbol)
	if err != nil {
		return nil, err
	}

	// Gate uses contract size units (each contract = quanto_multiplier base currency)
	// size = quantity / quanto_multiplier
	size, err := gateContractSize(quantity, contract)
	if err != nil {
		return nil, err
	}

	order := gateapi.FuturesOrder{
		Contract: symbol,
		Size:     size, // Positive for long
		Price:    "0",  // Market order
		Tif:      "ioc",
		Text:     "t-nofx",
	}

	logger.Infof("  [Gate] OpenLong: symbol=%s, size=%d, leverage=%d", symbol, size, leverage)

	result, _, err := t.client.FuturesApi.CreateFuturesOrder(t.ctx, "usdt", order, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open long position: %w", err)
	}

	// Clear cache
	t.clearCache()

	// Parse fill price from result
	fillPrice, _ := strconv.ParseFloat(result.FillPrice, 64)

	logger.Infof("  [Gate] Opened long position: orderId=%d, fillPrice=%.4f", result.Id, fillPrice)

	return map[string]interface{}{
		"orderId":   fmt.Sprintf("%d", result.Id),
		"symbol":    t.revertSymbol(symbol),
		"status":    "FILLED",
		"fillPrice": fillPrice,
		"avgPrice":  fillPrice,
	}, nil
}

// OpenShort opens a short position
func (t *GateTrader) OpenShort(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	symbol = t.convertSymbol(symbol)

	// Cancel old orders first
	t.CancelAllOrders(symbol)

	// Set leverage
	if err := t.SetLeverage(symbol, leverage); err != nil {
		logger.Warnf("  [Gate] Failed to set leverage: %v", err)
	}

	// Get contract info for size calculation
	contract, err := t.getContract(symbol)
	if err != nil {
		return nil, err
	}

	// Gate uses contract size units
	size, err := gateContractSize(quantity, contract)
	if err != nil {
		return nil, err
	}

	order := gateapi.FuturesOrder{
		Contract: symbol,
		Size:     -size, // Negative for short
		Price:    "0",   // Market order
		Tif:      "ioc",
		Text:     "t-nofx",
	}

	logger.Infof("  [Gate] OpenShort: symbol=%s, size=%d, leverage=%d", symbol, -size, leverage)

	result, _, err := t.client.FuturesApi.CreateFuturesOrder(t.ctx, "usdt", order, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open short position: %w", err)
	}

	// Clear cache
	t.clearCache()

	// Parse fill price from result
	fillPrice, _ := strconv.ParseFloat(result.FillPrice, 64)

	logger.Infof("  [Gate] Opened short position: orderId=%d, fillPrice=%.4f", result.Id, fillPrice)

	return map[string]interface{}{
		"orderId":   fmt.Sprintf("%d", result.Id),
		"symbol":    t.revertSymbol(symbol),
		"status":    "FILLED",
		"fillPrice": fillPrice,
		"avgPrice":  fillPrice,
	}, nil
}

// CloseLong closes a long position
func (t *GateTrader) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	symbol = t.convertSymbol(symbol)

	// If quantity is 0, get current position
	if quantity == 0 {
		positions, err := t.GetPositions()
		if err != nil {
			return nil, err
		}
		for _, pos := range positions {
			posSymbol := t.convertSymbol(pos["symbol"].(string))
			if posSymbol == symbol && pos["side"] == "long" {
				quantity = pos["positionAmt"].(float64)
				break
			}
		}
		if quantity == 0 {
			return nil, fmt.Errorf("long position not found for %s", symbol)
		}
	}

	// Get contract info for size calculation
	contract, err := t.getContract(symbol)
	if err != nil {
		return nil, err
	}

	size, err := gateContractSize(quantity, contract)
	if err != nil {
		return nil, err
	}

	// Close long = sell (use ReduceOnly, not Close which requires Size=0)
	order := gateapi.FuturesOrder{
		Contract:   symbol,
		Size:       -size, // Negative to close long
		Price:      "0",
		Tif:        "ioc",
		ReduceOnly: true,
		Text:       "t-nofx-close",
	}

	logger.Infof("  [Gate] CloseLong: symbol=%s, size=%d", symbol, -size)

	result, _, err := t.client.FuturesApi.CreateFuturesOrder(t.ctx, "usdt", order, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to close long position: %w", err)
	}

	// Clear cache
	t.clearCache()

	// Parse fill price from result
	fillPrice, _ := strconv.ParseFloat(result.FillPrice, 64)

	logger.Infof("  [Gate] Closed long position: orderId=%d, fillPrice=%.4f", result.Id, fillPrice)

	return map[string]interface{}{
		"orderId":   fmt.Sprintf("%d", result.Id),
		"symbol":    t.revertSymbol(symbol),
		"status":    "FILLED",
		"fillPrice": fillPrice,
		"avgPrice":  fillPrice,
	}, nil
}

// CloseShort closes a short position
func (t *GateTrader) CloseShort(symbol string, quantity float64) (map[string]interface{}, error) {
	symbol = t.convertSymbol(symbol)

	// If quantity is 0, get current position
	if quantity == 0 {
		positions, err := t.GetPositions()
		if err != nil {
			return nil, err
		}
		for _, pos := range positions {
			posSymbol := t.convertSymbol(pos["symbol"].(string))
			if posSymbol == symbol && pos["side"] == "short" {
				quantity = pos["positionAmt"].(float64)
				break
			}
		}
		if quantity == 0 {
			return nil, fmt.Errorf("short position not found for %s", symbol)
		}
	}

	// Ensure quantity is positive
	if quantity < 0 {
		quantity = -quantity
	}

	// Get contract info for size calculation
	contract, err := t.getContract(symbol)
	if err != nil {
		return nil, err
	}

	size, err := gateContractSize(quantity, contract)
	if err != nil {
		return nil, err
	}

	// Close short = buy (use ReduceOnly, not Close which requires Size=0)
	order := gateapi.FuturesOrder{
		Contract:   symbol,
		Size:       size, // Positive to close short
		Price:      "0",
		Tif:        "ioc",
		ReduceOnly: true,
		Text:       "t-nofx-close",
	}

	logger.Infof("  [Gate] CloseShort: symbol=%s, size=%d", symbol, size)

	result, _, err := t.client.FuturesApi.CreateFuturesOrder(t.ctx, "usdt", order, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to close short position: %w", err)
	}

	// Clear cache
	t.clearCache()

	// Parse fill price from result
	fillPrice, _ := strconv.ParseFloat(result.FillPrice, 64)

	logger.Infof("  [Gate] Closed short position: orderId=%d, fillPrice=%.4f", result.Id, fillPrice)

	return map[string]interface{}{
		"orderId":   fmt.Sprintf("%d", result.Id),
		"symbol":    t.revertSymbol(symbol),
		"status":    "FILLED",
		"fillPrice": fillPrice,
		"avgPrice":  fillPrice,
	}, nil
}

// GetMarketPrice gets the current market price
func (t *GateTrader) GetMarketPrice(symbol string) (float64, error) {
	symbol = t.convertSymbol(symbol)

	opts := &gateapi.ListFuturesTickersOpts{
		Contract: optional.NewString(symbol),
	}

	tickers, _, err := t.client.FuturesApi.ListFuturesTickers(t.ctx, "usdt", opts)
	if err != nil {
		return 0, fmt.Errorf("failed to get market price: %w", err)
	}

	if len(tickers) == 0 {
		return 0, fmt.Errorf("no ticker data for %s", symbol)
	}

	price, _ := strconv.ParseFloat(tickers[0].Last, 64)
	return price, nil
}

// SetStopLoss sets a stop loss order
func (t *GateTrader) SetStopLoss(symbol string, positionSide string, quantity, stopPrice float64) error {
	symbol = t.convertSymbol(symbol)

	contract, err := t.getContract(symbol)
	if err != nil {
		return err
	}

	size, err := gateContractSize(quantity, contract)
	if err != nil {
		return err
	}

	// For long position, stop loss means sell when price drops
	// For short position, stop loss means buy when price rises
	if strings.ToUpper(positionSide) == "LONG" {
		size = -size
	}

	// Use price trigger order
	trigger := gateapi.FuturesPriceTriggeredOrder{
		Initial: gateapi.FuturesInitialOrder{
			Contract:   symbol,
			Size:       size,
			Price:      "0", // Market order
			Tif:        "ioc",
			ReduceOnly: true,
			Close:      true,
		},
		Trigger: gateapi.FuturesPriceTrigger{
			StrategyType: 0, // Close position
			PriceType:    0, // Latest price
			Price:        fmt.Sprintf("%.8f", stopPrice),
			Rule:         1, // Price <= trigger price
		},
	}

	if strings.ToUpper(positionSide) == "SHORT" {
		trigger.Trigger.Rule = 2 // Price >= trigger price for short stop loss
	}

	_, _, err = t.client.FuturesApi.CreatePriceTriggeredOrder(t.ctx, "usdt", trigger)
	if err != nil {
		return fmt.Errorf("failed to set stop loss: %w", err)
	}

	logger.Infof("  [Gate] Stop loss set: %s @ %.4f", symbol, stopPrice)
	return nil
}

// SetTakeProfit sets a take profit order
func (t *GateTrader) SetTakeProfit(symbol string, positionSide string, quantity, takeProfitPrice float64) error {
	symbol = t.convertSymbol(symbol)

	contract, err := t.getContract(symbol)
	if err != nil {
		return err
	}

	size, err := gateContractSize(quantity, contract)
	if err != nil {
		return err
	}

	// For long position, take profit means sell when price rises
	// For short position, take profit means buy when price drops
	if strings.ToUpper(positionSide) == "LONG" {
		size = -size
	}

	trigger := gateapi.FuturesPriceTriggeredOrder{
		Initial: gateapi.FuturesInitialOrder{
			Contract:   symbol,
			Size:       size,
			Price:      "0", // Market order
			Tif:        "ioc",
			ReduceOnly: true,
			Close:      true,
		},
		Trigger: gateapi.FuturesPriceTrigger{
			StrategyType: 0, // Close position
			PriceType:    0, // Latest price
			Price:        fmt.Sprintf("%.8f", takeProfitPrice),
			Rule:         2, // Price >= trigger price for long take profit
		},
	}

	if strings.ToUpper(positionSide) == "SHORT" {
		trigger.Trigger.Rule = 1 // Price <= trigger price for short take profit
	}

	_, _, err = t.client.FuturesApi.CreatePriceTriggeredOrder(t.ctx, "usdt", trigger)
	if err != nil {
		return fmt.Errorf("failed to set take profit: %w", err)
	}

	logger.Infof("  [Gate] Take profit set: %s @ %.4f", symbol, takeProfitPrice)
	return nil
}

// CancelStopLossOrders cancels stop loss orders
func (t *GateTrader) CancelStopLossOrders(symbol string) error {
	return t.cancelTriggerOrders(symbol, "stop_loss")
}

// CancelTakeProfitOrders cancels take profit orders
func (t *GateTrader) CancelTakeProfitOrders(symbol string) error {
	return t.cancelTriggerOrders(symbol, "take_profit")
}

// cancelTriggerOrders cancels trigger orders of a specific type
func (t *GateTrader) cancelTriggerOrders(symbol string, orderType string) error {
	symbol = t.convertSymbol(symbol)

	opts := &gateapi.ListPriceTriggeredOrdersOpts{
		Contract: optional.NewString(symbol),
	}

	orders, _, err := t.client.FuturesApi.ListPriceTriggeredOrders(t.ctx, "usdt", "open", opts)
	if err != nil {
		return err
	}

	for _, order := range orders {
		// Determine if it's stop loss or take profit based on trigger rule and position
		// For simplicity, cancel all matching symbol orders
		_, _, err := t.client.FuturesApi.CancelPriceTriggeredOrder(t.ctx, "usdt", fmt.Sprintf("%d", order.Id))
		if err != nil {
			logger.Warnf("  [Gate] Failed to cancel trigger order %d: %v", order.Id, err)
		}
	}

	return nil
}

// CancelAllOrders cancels all pending orders for a symbol
func (t *GateTrader) CancelAllOrders(symbol string) error {
	symbol = t.convertSymbol(symbol)

	// Cancel regular orders
	_, _, err := t.client.FuturesApi.CancelFuturesOrders(t.ctx, "usdt", symbol, nil)
	if err != nil {
		// Ignore if no orders to cancel
		if !strings.Contains(err.Error(), "ORDER_NOT_FOUND") {
			logger.Warnf("  [Gate] Error canceling orders: %v", err)
		}
	}

	// Cancel trigger orders
	t.cancelTriggerOrders(symbol, "")

	return nil
}

// CancelStopOrders cancels all stop orders (stop loss and take profit)
func (t *GateTrader) CancelStopOrders(symbol string) error {
	t.CancelStopLossOrders(symbol)
	t.CancelTakeProfitOrders(symbol)
	return nil
}

// FormatQuantity formats quantity to correct precision
func (t *GateTrader) FormatQuantity(symbol string, quantity float64) (string, error) {
	contract, err := t.getContract(symbol)
	if err != nil {
		return fmt.Sprintf("%.4f", quantity), nil
	}

	// Gate uses quanto_multiplier for contract size
	if size, sizeErr := gateContractSize(quantity, contract); sizeErr == nil {
		return strconv.FormatInt(size, 10), nil
	}

	return fmt.Sprintf("%.4f", quantity), nil
}

// GetOrderStatus gets the status of an order
func (t *GateTrader) GetOrderStatus(symbol string, orderID string) (map[string]interface{}, error) {
	symbol = t.convertSymbol(symbol)

	order, _, err := t.client.FuturesApi.GetFuturesOrder(t.ctx, "usdt", orderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get order status: %w", err)
	}

	fillPrice, _ := strconv.ParseFloat(order.FillPrice, 64)
	tkFee, _ := strconv.ParseFloat(order.Tkfr, 64)
	mkFee, _ := strconv.ParseFloat(order.Mkfr, 64)
	totalFee := tkFee + mkFee

	// Get quanto_multiplier to convert contracts to actual quantity
	quantoMultiplier := 1.0
	contract, contractErr := t.getContract(symbol)
	if contractErr == nil && contract != nil {
		qm, _ := strconv.ParseFloat(contract.QuantoMultiplier, 64)
		if qm > 0 {
			quantoMultiplier = qm
		}
	}

	// Map status
	status := "NEW"
	switch order.Status {
	case "finished":
		if order.FinishAs == "filled" {
			status = "FILLED"
		} else if order.FinishAs == "cancelled" {
			status = "CANCELED"
		} else {
			status = "CLOSED"
		}
	case "open":
		status = "NEW"
	}

	side := "BUY"
	if order.Size < 0 {
		side = "SELL"
	}

	// Convert contract count to actual token quantity
	executedQty := math.Abs(float64(order.Size-order.Left)) * quantoMultiplier

	return map[string]interface{}{
		"orderId":     orderID,
		"symbol":      t.revertSymbol(symbol),
		"status":      status,
		"avgPrice":    fillPrice,
		"executedQty": executedQty,
		"side":        side,
		"type":        order.Tif,
		"time":        int64(order.CreateTime * 1000),
		"updateTime":  int64(order.FinishTime * 1000),
		"commission":  totalFee,
	}, nil
}

// GetOpenOrders gets open/pending orders
func (t *GateTrader) GetOpenOrders(symbol string) ([]types.OpenOrder, error) {
	symbol = t.convertSymbol(symbol)

	opts := &gateapi.ListFuturesOrdersOpts{
		Contract: optional.NewString(symbol),
	}

	orders, _, err := t.client.FuturesApi.ListFuturesOrders(t.ctx, "usdt", "open", opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get open orders: %w", err)
	}

	// Get quanto_multiplier to convert contracts to actual quantity
	quantoMultiplier := 1.0
	contract, err := t.getContract(symbol)
	if err == nil && contract != nil {
		qm, _ := strconv.ParseFloat(contract.QuantoMultiplier, 64)
		if qm > 0 {
			quantoMultiplier = qm
		}
	}

	var result []types.OpenOrder
	for _, order := range orders {
		price, _ := strconv.ParseFloat(order.Price, 64)

		side := "BUY"
		if order.Size < 0 {
			side = "SELL"
		}
		positionSide := "LONG"
		if side == "SELL" {
			positionSide = "SHORT"
		}
		if order.ReduceOnly || order.IsReduceOnly {
			if side == "SELL" {
				positionSide = "LONG"
			} else {
				positionSide = "SHORT"
			}
		}

		// Convert contract count to actual token quantity
		quantity := math.Abs(float64(order.Size)) * quantoMultiplier

		result = append(result, types.OpenOrder{
			OrderID:      fmt.Sprintf("%d", order.Id),
			Symbol:       t.revertSymbol(order.Contract),
			Side:         side,
			PositionSide: positionSide,
			Type:         "LIMIT",
			Price:        price,
			Quantity:     quantity,
			Status:       "NEW",
		})
	}

	// Also get trigger orders
	triggerOpts := &gateapi.ListPriceTriggeredOrdersOpts{
		Contract: optional.NewString(symbol),
	}

	triggerOrders, _, err := t.client.FuturesApi.ListPriceTriggeredOrders(t.ctx, "usdt", "open", triggerOpts)
	if err == nil {
		for _, order := range triggerOrders {
			triggerPrice, _ := strconv.ParseFloat(order.Trigger.Price, 64)

			side := "BUY"
			if order.Initial.Size < 0 {
				side = "SELL"
			}
			positionSide := "LONG"
			if side == "SELL" {
				positionSide = "SHORT"
			}
			if order.Initial.ReduceOnly || order.Initial.IsReduceOnly {
				if side == "SELL" {
					positionSide = "LONG"
				} else {
					positionSide = "SHORT"
				}
			}

			orderType := "STOP_MARKET"
			if order.Trigger.Rule == 2 {
				orderType = "TAKE_PROFIT_MARKET"
			}

			// Convert contract count to actual token quantity
			quantity := math.Abs(float64(order.Initial.Size)) * quantoMultiplier

			result = append(result, types.OpenOrder{
				OrderID:      fmt.Sprintf("%d", order.Id),
				Symbol:       t.revertSymbol(order.Initial.Contract),
				Side:         side,
				PositionSide: positionSide,
				Type:         orderType,
				StopPrice:    triggerPrice,
				Quantity:     quantity,
				Status:       "NEW",
			})
		}
	}

	return result, nil
}

// PlaceLimitOrder places a Gate futures limit order for grid/mirror sync.
func (t *GateTrader) PlaceLimitOrder(req *types.LimitOrderRequest) (*types.LimitOrderResult, error) {
	symbol := t.convertSymbol(req.Symbol)
	if req.Leverage > 0 {
		if err := t.SetLeverage(symbol, req.Leverage); err != nil {
			logger.Warnf("  [Gate] Failed to set leverage before limit order: %v", err)
		}
	}
	contract, err := t.getContract(symbol)
	if err != nil {
		return nil, err
	}
	size, err := gateContractSize(req.Quantity, contract)
	if err != nil {
		return nil, err
	}
	side := strings.ToUpper(strings.TrimSpace(req.Side))
	if side == "SELL" {
		size = -size
	}
	tif := "gtc"
	if req.PostOnly {
		tif = "poc"
	}
	order := gateapi.FuturesOrder{
		Contract:   symbol,
		Size:       size,
		Price:      fmt.Sprintf("%.8f", req.Price),
		Tif:        tif,
		ReduceOnly: req.ReduceOnly,
		Text:       "t-nofx-limit",
	}
	placed, _, err := t.client.FuturesApi.CreateFuturesOrder(t.ctx, "usdt", order, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to place limit order: %w", err)
	}
	t.clearCache()
	return &types.LimitOrderResult{
		OrderID:      fmt.Sprintf("%d", placed.Id),
		ClientID:     req.ClientID,
		Symbol:       req.Symbol,
		Side:         side,
		PositionSide: req.PositionSide,
		Price:        req.Price,
		Quantity:     req.Quantity,
		Status:       "NEW",
	}, nil
}

// CancelOrder cancels a specific Gate futures order.
func (t *GateTrader) CancelOrder(symbol, orderID string) error {
	_, _, err := t.client.FuturesApi.CancelFuturesOrder(t.ctx, "usdt", orderID, nil)
	if err != nil {
		return fmt.Errorf("failed to cancel order: %w", err)
	}
	t.clearCache()
	return nil
}

// GetOrderBook gets Gate futures order book levels.
func (t *GateTrader) GetOrderBook(symbol string, depth int) (bids, asks [][]float64, err error) {
	symbol = t.convertSymbol(symbol)
	if depth <= 0 {
		depth = 25
	}
	book, _, err := t.client.FuturesApi.ListFuturesOrderBook(t.ctx, "usdt", symbol, &gateapi.ListFuturesOrderBookOpts{
		Limit: optional.NewInt32(int32(depth)),
	})
	if err != nil {
		return nil, nil, err
	}
	parse := func(items []gateapi.FuturesOrderBookItem) [][]float64 {
		out := make([][]float64, 0, len(items))
		for _, item := range items {
			price, _ := strconv.ParseFloat(item.P, 64)
			if price > 0 {
				out = append(out, []float64{price, math.Abs(float64(item.S))})
			}
		}
		return out
	}
	return parse(book.Bids), parse(book.Asks), nil
}
