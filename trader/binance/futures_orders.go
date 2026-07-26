package binance

import (
	"context"
	"fmt"
	"math"
	"nofx/logger"
	"nofx/trader/types"
	"strconv"
	"strings"
	"time"

	"github.com/adshao/go-binance/v2/futures"
)

func isBinanceAPIAuthOrWhitelistError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "code=-2015") ||
		strings.Contains(s, "-2015") ||
		strings.Contains(s, "invalid api-key") ||
		strings.Contains(s, "invalid api key") ||
		strings.Contains(s, "permissions for action")
}

// OpenLong opens a long position
func (t *FuturesTrader) OpenLong(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	// First cancel all pending orders for this symbol (clean up old stop-loss and take-profit orders)
	if err := t.CancelAllOrders(symbol); err != nil {
		logger.Infof("  ⚠ Failed to cancel old pending orders (may not have any): %v", err)
	}

	// Set leverage
	if err := t.SetLeverage(symbol, leverage); err != nil {
		return nil, err
	}

	// Note: Margin mode should be set by the caller (AutoTrader) before opening position via SetMarginMode

	// Format quantity to correct precision
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// Check if formatted quantity is 0 (prevent rounding errors)
	quantityFloat, parseErr := strconv.ParseFloat(quantityStr, 64)
	if parseErr != nil || quantityFloat <= 0 {
		return nil, fmt.Errorf("position size too small, rounded to 0 (original: %.8f → formatted: %s). Suggest increasing position amount or selecting a lower-priced coin", quantity, quantityStr)
	}

	// Check minimum notional value (Binance requires at least 10 USDT)
	if err := t.CheckMinNotional(symbol, quantityFloat); err != nil {
		return nil, err
	}

	// Create market buy order (using br ID)
	order, err := t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeBuy).
		PositionSide(futures.PositionSideTypeLong).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		NewClientOrderID(getBrOrderID()).
		Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

	if err != nil {
		return nil, fmt.Errorf("failed to open long position: %w", err)
	}

	logger.Infof("✓ Opened long position successfully: %s quantity: %s", symbol, quantityStr)
	logger.Infof("  Order ID: %d", order.OrderID)

	result := make(map[string]interface{})
	result["orderId"] = order.OrderID
	result["symbol"] = order.Symbol
	result["status"] = order.Status
	return result, nil
}

// OpenShort opens a short position
func (t *FuturesTrader) OpenShort(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	// First cancel all pending orders for this symbol (clean up old stop-loss and take-profit orders)
	if err := t.CancelAllOrders(symbol); err != nil {
		logger.Infof("  ⚠ Failed to cancel old pending orders (may not have any): %v", err)
	}

	// Set leverage
	if err := t.SetLeverage(symbol, leverage); err != nil {
		return nil, err
	}

	// Note: Margin mode should be set by the caller (AutoTrader) before opening position via SetMarginMode

	// Format quantity to correct precision
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// Check if formatted quantity is 0 (prevent rounding errors)
	quantityFloat, parseErr := strconv.ParseFloat(quantityStr, 64)
	if parseErr != nil || quantityFloat <= 0 {
		return nil, fmt.Errorf("position size too small, rounded to 0 (original: %.8f → formatted: %s). Suggest increasing position amount or selecting a lower-priced coin", quantity, quantityStr)
	}

	// Check minimum notional value (Binance requires at least 10 USDT)
	if err := t.CheckMinNotional(symbol, quantityFloat); err != nil {
		return nil, err
	}

	// Create market sell order (using br ID)
	order, err := t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeSell).
		PositionSide(futures.PositionSideTypeShort).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		NewClientOrderID(getBrOrderID()).
		Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

	if err != nil {
		return nil, fmt.Errorf("failed to open short position: %w", err)
	}

	logger.Infof("✓ Opened short position successfully: %s quantity: %s", symbol, quantityStr)
	logger.Infof("  Order ID: %d", order.OrderID)

	result := make(map[string]interface{})
	result["orderId"] = order.OrderID
	result["symbol"] = order.Symbol
	result["status"] = order.Status
	return result, nil
}

// CloseLong closes a long position
func (t *FuturesTrader) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	// If quantity is 0, get current position quantity
	if quantity == 0 {
		positions, err := t.GetPositions()
		if err != nil {
			return nil, err
		}

		for _, pos := range positions {
			if pos["symbol"] == symbol && pos["side"] == "long" {
				quantity = pos["positionAmt"].(float64)
				break
			}
		}

		if quantity == 0 {
			return nil, fmt.Errorf("no long position found for %s", symbol)
		}
	}

	// Format quantity
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// Create market sell order (close long, using br ID)
	order, err := t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeSell).
		PositionSide(futures.PositionSideTypeLong).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		NewClientOrderID(getBrOrderID()).
		Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

	if err != nil {
		return nil, fmt.Errorf("failed to close long position: %w", err)
	}

	logger.Infof("✓ Closed long position successfully: %s quantity: %s", symbol, quantityStr)

	// After closing position, cancel all pending orders for this symbol (stop-loss and take-profit orders)
	if err := t.CancelAllOrders(symbol); err != nil {
		logger.Infof("  ⚠ Failed to cancel pending orders: %v", err)
	}

	result := make(map[string]interface{})
	result["orderId"] = order.OrderID
	result["symbol"] = order.Symbol
	result["status"] = order.Status
	return result, nil
}

// CloseShort closes a short position
func (t *FuturesTrader) CloseShort(symbol string, quantity float64) (map[string]interface{}, error) {
	// If quantity is 0, get current position quantity
	if quantity == 0 {
		positions, err := t.GetPositions()
		if err != nil {
			return nil, err
		}

		for _, pos := range positions {
			if pos["symbol"] == symbol && pos["side"] == "short" {
				quantity = -pos["positionAmt"].(float64) // Short position quantity is negative, take absolute value
				break
			}
		}

		if quantity == 0 {
			return nil, fmt.Errorf("no short position found for %s", symbol)
		}
	}

	// Format quantity
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// Create market buy order (close short, using br ID)
	order, err := t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeBuy).
		PositionSide(futures.PositionSideTypeShort).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		NewClientOrderID(getBrOrderID()).
		Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

	if err != nil {
		return nil, fmt.Errorf("failed to close short position: %w", err)
	}

	logger.Infof("✓ Closed short position successfully: %s quantity: %s", symbol, quantityStr)

	// After closing position, cancel all pending orders for this symbol (stop-loss and take-profit orders)
	if err := t.CancelAllOrders(symbol); err != nil {
		logger.Infof("  ⚠ Failed to cancel pending orders: %v", err)
	}

	result := make(map[string]interface{})
	result["orderId"] = order.OrderID
	result["symbol"] = order.Symbol
	result["status"] = order.Status
	return result, nil
}

// CancelStopLossOrders cancels only stop-loss orders (doesn't affect take-profit orders)
// Now uses both legacy API and new Algo Order API
func (t *FuturesTrader) CancelStopLossOrders(symbol string) error {
	canceledCount := 0
	var cancelErrors []error

	// 1. Cancel legacy stop-loss orders
	orders, err := t.client.NewListOpenOrdersService().
		Symbol(symbol).
		Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

	if err == nil {
		for _, order := range orders {
			orderType := string(order.Type)

			// Only cancel stop-loss orders (don't cancel take-profit orders)
			// Use string comparison since OrderType constants were removed in v2.8.9
			if orderType == "STOP_MARKET" || orderType == "STOP" {
				_, err := t.client.NewCancelOrderService().
					Symbol(symbol).
					OrderID(order.OrderID).
					Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

				if err != nil {
					errMsg := fmt.Sprintf("Order ID %d: %v", order.OrderID, err)
					cancelErrors = append(cancelErrors, fmt.Errorf("%s", errMsg))
					logger.Infof("  ⚠ Failed to cancel legacy stop-loss order: %s", errMsg)
					continue
				}

				canceledCount++
				logger.Infof("  ✓ Canceled legacy stop-loss order (Order ID: %d, Type: %s, Side: %s)", order.OrderID, orderType, order.PositionSide)
			}
		}
	}

	// 2. Cancel Algo stop-loss orders
	algoOrders, err := t.client.NewListOpenAlgoOrdersService().
		Symbol(symbol).
		Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

	if err == nil {
		for _, algoOrder := range algoOrders {
			slTrig, _ := strconv.ParseFloat(strings.TrimSpace(algoOrder.SlTriggerPrice), 64)
			typ := algoOrder.OrderType
			// 仓位/组合单：止损价在 slTriggerPrice，type 可能不是 STOP_MARKET；勿把纯止盈单误当止损
			isSL := typ == futures.AlgoOrderTypeStopMarket || typ == futures.AlgoOrderTypeStop ||
				(slTrig > 0 && typ != futures.AlgoOrderTypeTakeProfitMarket && typ != futures.AlgoOrderTypeTakeProfit)
			if !isSL {
				continue
			}
			_, err := t.client.NewCancelAlgoOrderService().
				AlgoID(algoOrder.AlgoId).
				Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

			if err != nil {
				errMsg := fmt.Sprintf("Algo ID %d: %v", algoOrder.AlgoId, err)
				cancelErrors = append(cancelErrors, fmt.Errorf("%s", errMsg))
				logger.Infof("  ⚠ Failed to cancel Algo stop-loss order: %s", errMsg)
				continue
			}

			canceledCount++
			logger.Infof("  ✓ Canceled Algo stop-loss order (Algo ID: %d, Type: %s)", algoOrder.AlgoId, algoOrder.OrderType)
		}
	}

	if canceledCount == 0 && len(cancelErrors) == 0 {
		logger.Infof("  ℹ %s has no stop-loss orders to cancel", symbol)
	} else if canceledCount > 0 {
		logger.Infof("  ✓ Canceled %d stop-loss order(s) for %s", canceledCount, symbol)
	}

	// If all cancellations failed, return error
	if len(cancelErrors) > 0 && canceledCount == 0 {
		return fmt.Errorf("failed to cancel stop-loss orders: %v", cancelErrors)
	}

	return nil
}

// CancelTakeProfitOrders cancels only take-profit orders (doesn't affect stop-loss orders)
// Now uses both legacy API and new Algo Order API
func (t *FuturesTrader) CancelTakeProfitOrders(symbol string) error {
	canceledCount := 0
	var cancelErrors []error

	// 1. Cancel legacy take-profit orders
	orders, err := t.client.NewListOpenOrdersService().
		Symbol(symbol).
		Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

	if err == nil {
		for _, order := range orders {
			orderType := string(order.Type)

			// Only cancel take-profit orders (don't cancel stop-loss orders)
			// Use string comparison since OrderType constants were removed in v2.8.9
			if orderType == "TAKE_PROFIT_MARKET" || orderType == "TAKE_PROFIT" {
				_, err := t.client.NewCancelOrderService().
					Symbol(symbol).
					OrderID(order.OrderID).
					Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

				if err != nil {
					errMsg := fmt.Sprintf("Order ID %d: %v", order.OrderID, err)
					cancelErrors = append(cancelErrors, fmt.Errorf("%s", errMsg))
					logger.Infof("  ⚠ Failed to cancel legacy take-profit order: %s", errMsg)
					continue
				}

				canceledCount++
				logger.Infof("  ✓ Canceled legacy take-profit order (Order ID: %d, Type: %s, Side: %s)", order.OrderID, orderType, order.PositionSide)
			}
		}
	}

	// 2. Cancel Algo take-profit orders
	algoOrders, err := t.client.NewListOpenAlgoOrdersService().
		Symbol(symbol).
		Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

	if err == nil {
		for _, algoOrder := range algoOrders {
			tpTrig, _ := strconv.ParseFloat(strings.TrimSpace(algoOrder.TpTriggerPrice), 64)
			typ := algoOrder.OrderType
			isTP := typ == futures.AlgoOrderTypeTakeProfitMarket || typ == futures.AlgoOrderTypeTakeProfit ||
				(tpTrig > 0 && typ != futures.AlgoOrderTypeStopMarket && typ != futures.AlgoOrderTypeStop)
			if !isTP {
				continue
			}
			_, err := t.client.NewCancelAlgoOrderService().
				AlgoID(algoOrder.AlgoId).
				Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

			if err != nil {
				errMsg := fmt.Sprintf("Algo ID %d: %v", algoOrder.AlgoId, err)
				cancelErrors = append(cancelErrors, fmt.Errorf("%s", errMsg))
				logger.Infof("  ⚠ Failed to cancel Algo take-profit order: %s", errMsg)
				continue
			}

			canceledCount++
			logger.Infof("  ✓ Canceled Algo take-profit order (Algo ID: %d, Type: %s)", algoOrder.AlgoId, algoOrder.OrderType)
		}
	}

	if canceledCount == 0 && len(cancelErrors) == 0 {
		logger.Infof("  ℹ %s has no take-profit orders to cancel", symbol)
	} else if canceledCount > 0 {
		logger.Infof("  ✓ Canceled %d take-profit order(s) for %s", canceledCount, symbol)
	}

	// If all cancellations failed, return error
	if len(cancelErrors) > 0 && canceledCount == 0 {
		return fmt.Errorf("failed to cancel take-profit orders: %v", cancelErrors)
	}

	return nil
}

// CancelAllOrders cancels all pending orders for this symbol
// Now uses both legacy API and new Algo Order API
func (t *FuturesTrader) CancelAllOrders(symbol string) error {
	// 1. Cancel all legacy orders
	legacyErr := t.client.NewCancelAllOpenOrdersService().
		Symbol(symbol).
		Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

	if legacyErr != nil {
		logger.Infof("  ⚠ Failed to cancel legacy orders: %v", legacyErr)
	} else {
		logger.Infof("  ✓ Canceled all legacy pending orders for %s", symbol)
	}

	// 2. Cancel all Algo orders
	algoErr := t.client.NewCancelAllAlgoOpenOrdersService().
		Symbol(symbol).
		Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

	if algoErr != nil {
		// Ignore "no algo orders" error
		if !contains(algoErr.Error(), "no algo") && !contains(algoErr.Error(), "No algo") {
			logger.Infof("  ⚠ Failed to cancel Algo orders: %v", algoErr)
		}
	} else {
		logger.Infof("  ✓ Canceled all Algo orders for %s", symbol)
	}

	if isBinanceAPIAuthOrWhitelistError(legacyErr) {
		return fmt.Errorf("cancel legacy orders auth/whitelist error: %w", legacyErr)
	}
	if isBinanceAPIAuthOrWhitelistError(algoErr) {
		return fmt.Errorf("cancel algo orders auth/whitelist error: %w", algoErr)
	}

	return nil
}

// futuresPositionSideFromLimitReq 双向持仓下必须使用挂单自带的 positionSide；仅用 BUY/SELL 推断会把「平多的卖单」误挂成「开空」导致拒单或错单。
func futuresPositionSideFromLimitReq(side, positionSide string) futures.PositionSideType {
	ps := strings.ToUpper(strings.TrimSpace(positionSide))
	switch ps {
	case "LONG":
		return futures.PositionSideTypeLong
	case "SHORT":
		return futures.PositionSideTypeShort
	default:
		if strings.ToUpper(strings.TrimSpace(side)) == "BUY" {
			return futures.PositionSideTypeLong
		}
		return futures.PositionSideTypeShort
	}
}

// PlaceLimitOrder places a limit order for grid trading
// This implements the GridTrader interface for FuturesTrader
func (t *FuturesTrader) PlaceLimitOrder(req *types.LimitOrderRequest) (*types.LimitOrderResult, error) {
	// Format quantity to correct precision
	quantityStr, err := t.FormatQuantity(req.Symbol, req.Quantity)
	if err != nil {
		return nil, fmt.Errorf("failed to format quantity: %w", err)
	}

	// Format price to correct precision
	priceStr, err := t.FormatPrice(req.Symbol, req.Price)
	if err != nil {
		return nil, fmt.Errorf("failed to format price: %w", err)
	}

	qtyFloat, errQty := strconv.ParseFloat(quantityStr, 64)
	if errQty != nil || qtyFloat <= 0 {
		return nil, fmt.Errorf("quantity after precision rounding is invalid: %q (raw %.12f)", quantityStr, req.Quantity)
	}
	if req.Price > 0 {
		notional := qtyFloat * req.Price
		minN := t.GetMinNotional(req.Symbol)
		if notional+1e-9 < minN {
			return nil, fmt.Errorf("limit notional %.4f USDT below minimum %.2f USDT (qty=%s price=%s)", notional, minN, quantityStr, priceStr)
		}
	}

	sideStr := strings.ToUpper(strings.TrimSpace(req.Side))
	if sideStr != "BUY" && sideStr != "SELL" {
		return nil, fmt.Errorf("invalid side %q", req.Side)
	}
	var side futures.SideType
	if sideStr == "BUY" {
		side = futures.SideTypeBuy
	} else {
		side = futures.SideTypeSell
	}
	positionSide := futuresPositionSideFromLimitReq(sideStr, req.PositionSide)

	// Set leverage if specified
	if req.Leverage > 0 {
		if err := t.SetLeverage(req.Symbol, req.Leverage); err != nil {
			logger.Warnf("Failed to set leverage: %v", err)
		}
	}

	tif := futures.TimeInForceTypeGTC
	if req.PostOnly {
		tif = futures.TimeInForceTypeGTX // Post-only / LIMIT_MAKER
	}

	// Build order service with broker-tagged client order ID (返佣归因)
	orderService := t.client.NewCreateOrderService().
		Symbol(req.Symbol).
		Side(side).
		PositionSide(positionSide).
		Type(futures.OrderTypeLimit).
		TimeInForce(tif).
		Quantity(quantityStr).
		Price(priceStr).
		NewClientOrderID(getBrOrderID())
	if req.ReduceOnly {
		orderService = orderService.ReduceOnly(true)
	}

	// Execute order
	order, err := orderService.Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))
	if err != nil {
		return nil, fmt.Errorf("failed to place limit order: %w", err)
	}

	logger.Infof("✓ [Grid] Placed limit order: %s %s pos=%s reduceOnly=%v postOnly=%v @ %s, qty=%s, orderID=%d",
		req.Symbol, req.Side, positionSide, req.ReduceOnly, req.PostOnly, priceStr, quantityStr, order.OrderID)

	return &types.LimitOrderResult{
		OrderID:      fmt.Sprintf("%d", order.OrderID),
		ClientID:     order.ClientOrderID,
		Symbol:       order.Symbol,
		Side:         string(order.Side),
		PositionSide: string(order.PositionSide),
		Price:        req.Price,
		Quantity:     req.Quantity,
		Status:       string(order.Status),
	}, nil
}

// CancelOrder cancels a specific order by ID
// This implements the GridTrader interface for FuturesTrader
func (t *FuturesTrader) CancelOrder(symbol, orderID string) error {
	// Parse order ID to int64
	orderIDInt, err := strconv.ParseInt(orderID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid order ID: %w", err)
	}

	_, err = t.client.NewCancelOrderService().
		Symbol(symbol).
		OrderID(orderIDInt).
		Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

	if err != nil {
		return fmt.Errorf("failed to cancel order: %w", err)
	}

	logger.Infof("✓ [Grid] Cancelled order: %s/%s", symbol, orderID)
	return nil
}

// GetOrderBook gets the order book for a symbol
// This implements the GridTrader interface for FuturesTrader
func (t *FuturesTrader) GetOrderBook(symbol string, depth int) (bids, asks [][]float64, err error) {
	book, err := t.client.NewDepthService().
		Symbol(symbol).
		Limit(depth).
		Do(context.Background())

	if err != nil {
		return nil, nil, fmt.Errorf("failed to get order book: %w", err)
	}

	// Convert bids
	bids = make([][]float64, len(book.Bids))
	for i, bid := range book.Bids {
		price, _ := strconv.ParseFloat(bid.Price, 64)
		qty, _ := strconv.ParseFloat(bid.Quantity, 64)
		bids[i] = []float64{price, qty}
	}

	// Convert asks
	asks = make([][]float64, len(book.Asks))
	for i, ask := range book.Asks {
		price, _ := strconv.ParseFloat(ask.Price, 64)
		qty, _ := strconv.ParseFloat(ask.Quantity, 64)
		asks[i] = []float64{price, qty}
	}

	return bids, asks, nil
}

// CancelStopOrders cancels take-profit/stop-loss orders for this symbol (used to adjust TP/SL positions)
// Now uses both legacy API and new Algo Order API (Binance migrated stop orders to Algo system)
func (t *FuturesTrader) CancelStopOrders(symbol string) error {
	canceledCount := 0

	// 1. Cancel legacy stop orders (for backward compatibility)
	orders, err := t.client.NewListOpenOrdersService().
		Symbol(symbol).
		Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

	if err == nil {
		for _, order := range orders {
			orderType := string(order.Type)

			// Only cancel stop-loss and take-profit orders
			// Use string comparison since OrderType constants were removed in v2.8.9
			if orderType == "STOP_MARKET" ||
				orderType == "TAKE_PROFIT_MARKET" ||
				orderType == "STOP" ||
				orderType == "TAKE_PROFIT" {

				_, err := t.client.NewCancelOrderService().
					Symbol(symbol).
					OrderID(order.OrderID).
					Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

				if err != nil {
					logger.Infof("  ⚠ Failed to cancel legacy order %d: %v", order.OrderID, err)
					continue
				}

				canceledCount++
				logger.Infof("  ✓ Canceled legacy stop order for %s (Order ID: %d, Type: %s)",
					symbol, order.OrderID, orderType)
			}
		}
	}

	// 2. Cancel Algo orders (new API)
	err = t.client.NewCancelAllAlgoOpenOrdersService().
		Symbol(symbol).
		Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

	if err != nil {
		// Ignore "no algo orders" error
		if !contains(err.Error(), "no algo") && !contains(err.Error(), "No algo") {
			logger.Infof("  ⚠ Failed to cancel Algo orders: %v", err)
		}
	} else {
		logger.Infof("  ✓ Canceled all Algo orders for %s", symbol)
		canceledCount++
	}

	if canceledCount == 0 {
		logger.Infof("  ℹ %s has no take-profit/stop-loss orders to cancel", symbol)
	}

	return nil
}

// appendFuturesOpenAlgoOrders 合并 /fapi/v1/openAlgoOrders 结果。
// 币安常把「仓位止损/止盈」写在 slTriggerPrice、tpTriggerPrice，而 triggerPrice 为空；这里拆成独立虚拟行便于展示与镜像解析。
// seen 的 key 形如 "123_main" / "123_sl" / "123_tp" 防止重复。
func appendFuturesOpenAlgoOrders(dst *[]types.OpenOrder, seen map[string]struct{}, algos []futures.GetAlgoOrderResp) {
	parseF := func(s string) float64 {
		s = strings.TrimSpace(s)
		if s == "" {
			return 0
		}
		v, _ := strconv.ParseFloat(s, 64)
		return v
	}
	priceNearlyEqual := func(a, b float64) bool {
		if a <= 0 || b <= 0 {
			return false
		}
		scale := math.Max(math.Abs(a), math.Abs(b))
		return math.Abs(a-b) <= 1e-8*math.Max(1, scale)
	}

	for _, algoOrder := range algos {
		if algoOrder.AlgoId == 0 {
			continue
		}
		oid := strconv.FormatInt(algoOrder.AlgoId, 10)
		st := strings.TrimSpace(algoOrder.AlgoStatus)
		if st == "" {
			st = "NEW"
		}
		price, _ := strconv.ParseFloat(algoOrder.Price, 64)
		quantity, _ := strconv.ParseFloat(algoOrder.Quantity, 64)
		mainTrig := parseF(algoOrder.TriggerPrice)
		// 仓位/组合：触发价有时只在 slPrice、tpPrice，而 slTriggerPrice/tpTriggerPrice 为 0
		slTrig := parseF(algoOrder.SlTriggerPrice)
		if slTrig <= 0 {
			slTrig = parseF(algoOrder.SlPrice)
		}
		tpTrig := parseF(algoOrder.TpTriggerPrice)
		if tpTrig <= 0 {
			tpTrig = parseF(algoOrder.TpPrice)
		}
		typU := strings.ToUpper(strings.TrimSpace(string(algoOrder.OrderType)))

		push := func(dedupeKey, orderID string, trig float64, typStr string) {
			if trig <= 0 {
				return
			}
			if _, ok := seen[dedupeKey]; ok {
				return
			}
			seen[dedupeKey] = struct{}{}
			*dst = append(*dst, types.OpenOrder{
				OrderID:      orderID,
				Symbol:       algoOrder.Symbol,
				Side:         string(algoOrder.Side),
				PositionSide: string(algoOrder.PositionSide),
				Type:         typStr,
				Price:        price,
				StopPrice:    trig,
				Quantity:     quantity,
				Status:       st,
			})
		}

		if slTrig > 0 {
			push(oid+"_sl", oid+"_sl", slTrig, "STOP_MARKET")
		}
		if tpTrig > 0 {
			push(oid+"_tp", oid+"_tp", tpTrig, "TAKE_PROFIT_MARKET")
		}

		if mainTrig <= 0 {
			continue
		}
		mainKey := oid + "_main"
		if _, ok := seen[mainKey]; ok {
			continue
		}
		skipMain := false
		if slTrig > 0 && (typU == "STOP_MARKET" || typU == "STOP") && priceNearlyEqual(mainTrig, slTrig) {
			skipMain = true
		}
		if tpTrig > 0 && strings.Contains(typU, "TAKE_PROFIT") && priceNearlyEqual(mainTrig, tpTrig) {
			skipMain = true
		}
		if skipMain {
			seen[mainKey] = struct{}{}
			continue
		}
		seen[mainKey] = struct{}{}
		*dst = append(*dst, types.OpenOrder{
			OrderID:      oid,
			Symbol:       algoOrder.Symbol,
			Side:         string(algoOrder.Side),
			PositionSide: string(algoOrder.PositionSide),
			Type:         string(algoOrder.OrderType),
			Price:        price,
			StopPrice:    mainTrig,
			Quantity:     quantity,
			Status:       st,
		})
	}
}

// GetOpenOrders gets all open/pending orders for a symbol.
// 若 symbol 为空字符串，则按 Binance 合约接口返回**当前账户全部交易对**的挂单（含不在策略候选币里的手工限价单）。
// 说明：App 里「下单时带的止盈止损」与「持仓区点的仓位止盈止损」在 U 本位上多数都会落成 STOP/TP 条件单；
// 新单走 /fapi/v1/openAlgoOrders（可能带 slTriggerPrice/tpTriggerPrice）；老单或部分类型仍在 /fapi/v1/openOrders。二者这里都会合并。
// 不传 openAlgoOrders 的 algoType 参数，避免误过滤；统一账户/仅网页内部状态且 API 不返回的，本接口无法读取。
func (t *FuturesTrader) GetOpenOrders(symbol string) ([]types.OpenOrder, error) {
	var result []types.OpenOrder
	sym := strings.TrimSpace(symbol)
	allSymbols := sym == ""

	if allSymbols {
		t.openOrdersCacheMutex.RLock()
		if t.cachedOpenOrders != nil && time.Since(t.openOrdersCacheTime) < 20*time.Second {
			cached := append([]types.OpenOrder(nil), t.cachedOpenOrders...)
			t.openOrdersCacheMutex.RUnlock()
			logger.Infof("✓ Using cached all-account open orders (%d)", len(cached))
			return cached, nil
		}
		if t.isRateLimited() && t.cachedOpenOrders != nil {
			cached := append([]types.OpenOrder(nil), t.cachedOpenOrders...)
			t.openOrdersCacheMutex.RUnlock()
			logger.Infof("✓ Using cached open orders during Binance rate-limit cooldown")
			return cached, nil
		}
		t.openOrdersCacheMutex.RUnlock()
	}

	// 1. Get legacy open orders
	svc := t.client.NewListOpenOrdersService()
	if !allSymbols {
		svc = svc.Symbol(sym)
	}
	orders, err := svc.Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))
	if err != nil {
		t.markRateLimited(err)
		if allSymbols && t.cachedOpenOrders != nil {
			t.openOrdersCacheMutex.RLock()
			cached := append([]types.OpenOrder(nil), t.cachedOpenOrders...)
			t.openOrdersCacheMutex.RUnlock()
			return cached, nil
		}
		if allSymbols {
			logger.Infof("  ⚠ 币安 U 本位全账户挂单(openOrders)失败：请确认 API 为**合约(U本位)**密钥、含读取权限，并核对 IP 白名单。错误: %v", err)
		}
		return nil, fmt.Errorf("failed to get open orders: %w", err)
	}

	for _, order := range orders {
		price, _ := strconv.ParseFloat(order.Price, 64)
		stopPrice, _ := strconv.ParseFloat(order.StopPrice, 64)
		typU := strings.ToUpper(string(order.Type))
		// 移动止损：文档说明 stopPrice 可忽略，实际用 activatePrice
		if typU == "TRAILING_STOP_MARKET" {
			if ap, _ := strconv.ParseFloat(strings.TrimSpace(order.ActivatePrice), 64); ap > 0 {
				stopPrice = ap
			}
		}
		origQty, _ := strconv.ParseFloat(order.OrigQuantity, 64)
		executedQty, _ := strconv.ParseFloat(order.ExecutedQuantity, 64)
		quantity := origQty - executedQty
		if quantity <= 0 && origQty > 0 {
			// 部分接口字段未填 executed 时仍用原始委托量
			quantity = origQty
		}

		result = append(result, types.OpenOrder{
			OrderID:      fmt.Sprintf("%d", order.OrderID),
			Symbol:       order.Symbol,
			Side:         string(order.Side),
			PositionSide: string(order.PositionSide),
			Type:         string(order.Type),
			Price:        price,
			StopPrice:    stopPrice,
			Quantity:     quantity,
			Status:       string(order.Status),
		})
	}

	// 2. Algo 条件单（止盈止损等）。全账户时优先不传 symbol 一次拉齐，避免「仅有 Algo 单、无普通限价」时漏扫。
	if allSymbols {
		logger.Infof("  ℹ 币安 U 本位全账户: openOrders 普通挂单 %d 笔", len(orders))
		algoSeen := make(map[string]struct{})
		algoOrdersAll, err2 := t.client.NewListOpenAlgoOrdersService().
			Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))
		if err2 != nil {
			t.markRateLimited(err2)
			logger.Infof("  ⚠ 币安全账户 openAlgoOrders 失败(条件单可能不完整): %v — 将按已有普通挂单的交易对逐个重试", err2)
			queried := make(map[string]struct{})
			for _, o := range result {
				s := strings.TrimSpace(o.Symbol)
				if s == "" {
					continue
				}
				if _, ok := queried[s]; ok {
					continue
				}
				queried[s] = struct{}{}
				bySym, err3 := t.client.NewListOpenAlgoOrdersService().
					Symbol(s).
					Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))
				if err3 != nil {
					continue
				}
				appendFuturesOpenAlgoOrders(&result, algoSeen, bySym)
			}
		} else {
			appendFuturesOpenAlgoOrders(&result, algoSeen, algoOrdersAll)
			logger.Infof("  ℹ 币安全账户 openAlgoOrders: 条件/算法单原始 %d 笔（含 sl/tp 拆条后可能更多）", len(algoOrdersAll))
		}
		t.openOrdersCacheMutex.Lock()
		t.cachedOpenOrders = append([]types.OpenOrder(nil), result...)
		t.openOrdersCacheTime = time.Now()
		t.openOrdersCacheMutex.Unlock()
		return result, nil
	}

	algoSeen := make(map[string]struct{})
	algoOrders, err := t.client.NewListOpenAlgoOrdersService().
		Symbol(sym).
		Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

	if err != nil {
		t.markRateLimited(err)
		logger.Infof("  ⚠ 币安 %s openAlgoOrders 失败: %v（仅返回普通 openOrders）", sym, err)
	} else {
		appendFuturesOpenAlgoOrders(&result, algoSeen, algoOrders)
	}

	return result, nil
}

// SetStopLoss sets stop-loss order using new Algo Order API
// Binance has migrated stop orders to Algo Order system (error -4120 STOP_ORDER_SWITCH_ALGO)
func (t *FuturesTrader) SetStopLoss(symbol string, positionSide string, quantity, stopPrice float64) error {
	var side futures.SideType
	var posSide futures.PositionSideType

	if positionSide == "LONG" {
		side = futures.SideTypeSell
		posSide = futures.PositionSideTypeLong
	} else {
		side = futures.SideTypeBuy
		posSide = futures.PositionSideTypeShort
	}

	// Use new Algo Order API
	_, err := t.client.NewCreateAlgoOrderService().
		Symbol(symbol).
		Side(side).
		PositionSide(posSide).
		Type(futures.AlgoOrderTypeStopMarket).
		TriggerPrice(fmt.Sprintf("%.8f", stopPrice)).
		WorkingType(futures.WorkingTypeContractPrice).
		ClosePosition(true).
		ClientAlgoId(getBrOrderID()).
		Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

	if err != nil {
		return fmt.Errorf("failed to set stop-loss: %w", err)
	}

	logger.Infof("  Stop-loss price set (Algo Order): %.4f", stopPrice)
	return nil
}

// SetTakeProfit sets take-profit order using new Algo Order API
// Binance has migrated stop orders to Algo Order system (error -4120 STOP_ORDER_SWITCH_ALGO)
func (t *FuturesTrader) SetTakeProfit(symbol string, positionSide string, quantity, takeProfitPrice float64) error {
	var side futures.SideType
	var posSide futures.PositionSideType

	if positionSide == "LONG" {
		side = futures.SideTypeSell
		posSide = futures.PositionSideTypeLong
	} else {
		side = futures.SideTypeBuy
		posSide = futures.PositionSideTypeShort
	}

	// Use new Algo Order API
	_, err := t.client.NewCreateAlgoOrderService().
		Symbol(symbol).
		Side(side).
		PositionSide(posSide).
		Type(futures.AlgoOrderTypeTakeProfitMarket).
		TriggerPrice(fmt.Sprintf("%.8f", takeProfitPrice)).
		WorkingType(futures.WorkingTypeContractPrice).
		ClosePosition(true).
		ClientAlgoId(getBrOrderID()).
		Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

	if err != nil {
		return fmt.Errorf("failed to set take-profit: %w", err)
	}

	logger.Infof("  Take-profit price set (Algo Order): %.4f", takeProfitPrice)
	return nil
}

// GetOrderStatus gets order status
func (t *FuturesTrader) GetOrderStatus(symbol string, orderID string) (map[string]interface{}, error) {
	// Convert orderID to int64
	orderIDInt, err := strconv.ParseInt(orderID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid order ID: %s", orderID)
	}

	order, err := t.client.NewGetOrderService().
		Symbol(symbol).
		OrderID(orderIDInt).
		Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))
	if err != nil {
		return nil, fmt.Errorf("failed to get order status: %w", err)
	}

	// Parse execution price
	avgPrice, _ := strconv.ParseFloat(order.AvgPrice, 64)
	executedQty, _ := strconv.ParseFloat(order.ExecutedQuantity, 64)

	result := map[string]interface{}{
		"orderId":     order.OrderID,
		"symbol":      order.Symbol,
		"status":      string(order.Status),
		"avgPrice":    avgPrice,
		"executedQty": executedQty,
		"side":        string(order.Side),
		"type":        string(order.Type),
		"time":        order.Time,
		"updateTime":  order.UpdateTime,
	}

	// Binance futures commission fee needs to be obtained through GetUserTrades, not retrieved here for now
	// Can be obtained later through WebSocket or separate query
	result["commission"] = 0.0

	return result, nil
}
