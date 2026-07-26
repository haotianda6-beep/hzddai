package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"nofx/logger"
)

func (t *FuturesTrader) resetWSAPIClient() {
	t.wsAPIMu.Lock()
	defer t.wsAPIMu.Unlock()
	if t.wsAPIClient != nil {
		_ = t.wsAPIClient.Close()
		t.wsAPIClient = nil
	}
}

// getOrCreateWSAPIClient 单例：Dial + session.logon；失败时清空以便下次重建。
func (t *FuturesTrader) getOrCreateWSAPIClient(ctx context.Context) (*FuturesWSAPIClient, error) {
	t.wsAPIMu.Lock()
	defer t.wsAPIMu.Unlock()
	if t.wsAPIClient != nil {
		return t.wsAPIClient, nil
	}
	ep := futuresWSAPIMainnet
	if t.useDemoTrading || strings.TrimSpace(os.Getenv("BINANCE_FUTURES_WS_TESTNET")) == "1" {
		ep = futuresWSAPITestnet
	}
	cli := NewFuturesWSAPIClient(t.client.APIKey, t.client.SecretKey, ep, func(c context.Context) (int64, error) {
		srv, err := t.client.NewServerTimeService().Do(c)
		if err != nil {
			return 0, err
		}
		return srv + t.client.TimeOffset, nil
	})
	if err := cli.ensureConn(ctx); err != nil {
		return nil, err
	}
	if _, err := cli.SessionLogon(ctx, fmt.Sprintf("logon-%d", time.Now().UnixNano())); err != nil {
		_ = cli.Close()
		return nil, err
	}
	t.wsAPIClient = cli
	logger.Infof("binance WS-API: session.logon ok endpoint=%s", ep)
	return t.wsAPIClient, nil
}

func wsAPIParseOrderResult(raw []byte) (map[string]interface{}, error) {
	if st, _, err := wsAPIParseStatus(raw); err != nil || st != 200 {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("order response status=%d", st)
	}
	var top map[string]interface{}
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, err
	}
	res, _ := top["result"].(map[string]interface{})
	if res == nil {
		return nil, fmt.Errorf("order response: missing result")
	}
	return res, nil
}

// CloseLongWebSocketAPI 使用 WebSocket API 下市价平多（与 CloseLong REST 语义对齐）。
func (t *FuturesTrader) CloseLongWebSocketAPI(ctx context.Context, symbol string, quantity float64) (map[string]interface{}, error) {
	qty := quantity
	if qty == 0 {
		positions, err := t.GetPositions()
		if err != nil {
			return nil, err
		}
		for _, pos := range positions {
			if pos["symbol"] == symbol && pos["side"] == "long" {
				qty = pos["positionAmt"].(float64)
				break
			}
		}
		if qty == 0 {
			return nil, fmt.Errorf("no long position found for %s", symbol)
		}
	}
	qtyStr, err := t.FormatQuantity(symbol, qty)
	if err != nil {
		return nil, err
	}
	qtyFloat, _ := strconv.ParseFloat(qtyStr, 64)
	if err := t.CheckMinNotional(symbol, qtyFloat); err != nil {
		return nil, err
	}
	cli, err := t.getOrCreateWSAPIClient(ctx)
	if err != nil {
		return nil, err
	}
	id := fmt.Sprintf("cl-%d", time.Now().UnixNano())
	raw, err := cli.OrderPlace(ctx, id, map[string]interface{}{
		"symbol":           symbol,
		"side":             "SELL",
		"positionSide":     "LONG",
		"type":             "MARKET",
		"quantity":         qtyStr,
		"newClientOrderId": getBrOrderID(),
	})
	if err != nil {
		t.resetWSAPIClient()
		return nil, err
	}
	res, err := wsAPIParseOrderResult(raw)
	if err != nil {
		return nil, err
	}
	_ = t.CancelAllOrders(symbol)
	return map[string]interface{}{
		"orderId": res["orderId"],
		"symbol":  symbol,
		"status":  res["status"],
	}, nil
}

// CloseShortWebSocketAPI WebSocket API 市价平空。
func (t *FuturesTrader) CloseShortWebSocketAPI(ctx context.Context, symbol string, quantity float64) (map[string]interface{}, error) {
	qty := quantity
	if qty == 0 {
		positions, err := t.GetPositions()
		if err != nil {
			return nil, err
		}
		for _, pos := range positions {
			if pos["symbol"] == symbol && pos["side"] == "short" {
				qty = -pos["positionAmt"].(float64)
				break
			}
		}
		if qty == 0 {
			return nil, fmt.Errorf("no short position found for %s", symbol)
		}
	}
	qtyStr, err := t.FormatQuantity(symbol, qty)
	if err != nil {
		return nil, err
	}
	qtyFloat, _ := strconv.ParseFloat(qtyStr, 64)
	if err := t.CheckMinNotional(symbol, qtyFloat); err != nil {
		return nil, err
	}
	cli, err := t.getOrCreateWSAPIClient(ctx)
	if err != nil {
		return nil, err
	}
	id := fmt.Sprintf("cs-%d", time.Now().UnixNano())
	raw, err := cli.OrderPlace(ctx, id, map[string]interface{}{
		"symbol":           symbol,
		"side":             "BUY",
		"positionSide":     "SHORT",
		"type":             "MARKET",
		"quantity":         qtyStr,
		"newClientOrderId": getBrOrderID(),
	})
	if err != nil {
		t.resetWSAPIClient()
		return nil, err
	}
	res, err := wsAPIParseOrderResult(raw)
	if err != nil {
		return nil, err
	}
	_ = t.CancelAllOrders(symbol)
	return map[string]interface{}{
		"orderId": res["orderId"],
		"symbol":  symbol,
		"status":  res["status"],
	}, nil
}

func (t *FuturesTrader) CloseLongPreparedWebSocketAPI(ctx context.Context, symbol string, quantity float64) (map[string]interface{}, error) {
	return t.closePreparedWebSocketAPI(ctx, symbol, "SELL", "LONG", quantity)
}

func (t *FuturesTrader) CloseShortPreparedWebSocketAPI(ctx context.Context, symbol string, quantity float64) (map[string]interface{}, error) {
	return t.closePreparedWebSocketAPI(ctx, symbol, "BUY", "SHORT", quantity)
}

func (t *FuturesTrader) closePreparedWebSocketAPI(ctx context.Context, symbol, side, positionSide string, quantity float64) (map[string]interface{}, error) {
	qtyStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}
	qtyFloat, err := strconv.ParseFloat(qtyStr, 64)
	if err != nil || qtyFloat <= 0 {
		return nil, fmt.Errorf("position size too small after format")
	}
	if err := t.CheckMinNotional(symbol, qtyFloat); err != nil {
		return nil, err
	}
	cli, err := t.getOrCreateWSAPIClient(ctx)
	if err != nil {
		return nil, err
	}
	raw, err := cli.OrderPlace(ctx, fmt.Sprintf("cp-%d", time.Now().UnixNano()), map[string]interface{}{
		"symbol":           symbol,
		"side":             side,
		"positionSide":     positionSide,
		"type":             "MARKET",
		"quantity":         qtyStr,
		"newClientOrderId": getBrOrderID(),
	})
	if err != nil {
		t.resetWSAPIClient()
		return nil, err
	}
	res, err := wsAPIParseOrderResult(raw)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"orderId": res["orderId"], "symbol": symbol, "status": res["status"]}, nil
}

// OpenLongWebSocketAPI WebSocket API 市价开多。
func (t *FuturesTrader) OpenLongWebSocketAPI(ctx context.Context, symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	if err := t.CancelAllOrders(symbol); err != nil {
		logger.Infof("  ⚠ WS OpenLong cancel pending: %v", err)
	}
	if err := t.SetLeverage(symbol, leverage); err != nil {
		return nil, err
	}
	return t.openPositionWebSocketAPI(ctx, symbol, "BUY", "LONG", quantity)
}

func (t *FuturesTrader) OpenLongPreparedWebSocketAPI(ctx context.Context, symbol string, quantity float64) (map[string]interface{}, error) {
	return t.openPositionWebSocketAPI(ctx, symbol, "BUY", "LONG", quantity)
}

// OpenShortWebSocketAPI WebSocket API 市价开空。
func (t *FuturesTrader) OpenShortWebSocketAPI(ctx context.Context, symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	if err := t.CancelAllOrders(symbol); err != nil {
		logger.Infof("  ⚠ WS OpenShort cancel pending: %v", err)
	}
	if err := t.SetLeverage(symbol, leverage); err != nil {
		return nil, err
	}
	return t.openPositionWebSocketAPI(ctx, symbol, "SELL", "SHORT", quantity)
}

func (t *FuturesTrader) OpenShortPreparedWebSocketAPI(ctx context.Context, symbol string, quantity float64) (map[string]interface{}, error) {
	return t.openPositionWebSocketAPI(ctx, symbol, "SELL", "SHORT", quantity)
}

func (t *FuturesTrader) openPositionWebSocketAPI(ctx context.Context, symbol, side, positionSide string, quantity float64) (map[string]interface{}, error) {
	qtyStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}
	qtyFloat, parseErr := strconv.ParseFloat(qtyStr, 64)
	if parseErr != nil || qtyFloat <= 0 {
		return nil, fmt.Errorf("position size too small after format")
	}
	if err := t.CheckMinNotional(symbol, qtyFloat); err != nil {
		return nil, err
	}
	cli, err := t.getOrCreateWSAPIClient(ctx)
	if err != nil {
		return nil, err
	}
	id := fmt.Sprintf("op-%d", time.Now().UnixNano())
	raw, err := cli.OrderPlace(ctx, id, map[string]interface{}{
		"symbol":           symbol,
		"side":             side,
		"positionSide":     positionSide,
		"type":             "MARKET",
		"quantity":         qtyStr,
		"newClientOrderId": getBrOrderID(),
	})
	if err != nil {
		t.resetWSAPIClient()
		return nil, err
	}
	res, err := wsAPIParseOrderResult(raw)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"orderId": res["orderId"],
		"symbol":  symbol,
		"status":  res["status"],
	}, nil
}

// SetStopLossWebSocketAPI WS API STOP_MARKET 止损（双向持仓）。
func (t *FuturesTrader) SetStopLossWebSocketAPI(ctx context.Context, symbol, positionSide string, quantity, stopPrice float64) error {
	qtyStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return err
	}
	cli, err := t.getOrCreateWSAPIClient(ctx)
	if err != nil {
		return t.SetStopLoss(symbol, positionSide, quantity, stopPrice)
	}
	ps := strings.ToUpper(strings.TrimSpace(positionSide))
	side := "SELL"
	if ps == "SHORT" {
		side = "BUY"
	}
	id := fmt.Sprintf("sl-%d", time.Now().UnixNano())
	raw, err := cli.OrderPlace(ctx, id, map[string]interface{}{
		"symbol":       symbol,
		"side":         side,
		"positionSide": ps,
		"type":         "STOP_MARKET",
		"stopPrice":    fmt.Sprintf("%.8f", stopPrice),
		"quantity":     qtyStr,
		"workingType":  "CONTRACT_PRICE",

		"newClientOrderId": getBrOrderID(),
	})
	if err != nil {
		t.resetWSAPIClient()
		return t.SetStopLoss(symbol, positionSide, quantity, stopPrice)
	}
	_, err = wsAPIParseOrderResult(raw)
	if err != nil {
		logger.Infof("WS止损 %s 失败(回退REST): %v", symbol, err)
		return t.SetStopLoss(symbol, positionSide, quantity, stopPrice)
	}
	return nil
}

// SetTakeProfitWebSocketAPI WS API TAKE_PROFIT_MARKET 止盈。
func (t *FuturesTrader) SetTakeProfitWebSocketAPI(ctx context.Context, symbol, positionSide string, quantity, takeProfitPrice float64) error {
	qtyStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return err
	}
	cli, err := t.getOrCreateWSAPIClient(ctx)
	if err != nil {
		return t.SetTakeProfit(symbol, positionSide, quantity, takeProfitPrice)
	}
	ps := strings.ToUpper(strings.TrimSpace(positionSide))
	side := "SELL"
	if ps == "SHORT" {
		side = "BUY"
	}
	id := fmt.Sprintf("tp-%d", time.Now().UnixNano())
	raw, err := cli.OrderPlace(ctx, id, map[string]interface{}{
		"symbol":           symbol,
		"side":             side,
		"positionSide":     ps,
		"type":             "TAKE_PROFIT_MARKET",
		"stopPrice":        fmt.Sprintf("%.8f", takeProfitPrice),
		"quantity":         qtyStr,
		"workingType":      "CONTRACT_PRICE",
		"newClientOrderId": getBrOrderID(),
	})
	if err != nil {
		t.resetWSAPIClient()
		return t.SetTakeProfit(symbol, positionSide, quantity, takeProfitPrice)
	}
	_, err = wsAPIParseOrderResult(raw)
	if err != nil {
		logger.Infof("WS止盈 %s 失败(回退REST): %v", symbol, err)
		return t.SetTakeProfit(symbol, positionSide, quantity, takeProfitPrice)
	}
	return nil
}

// PlaceLimitOrderWebSocketAPI 使用 WebSocket API 下限价单（用于 v2 跟单，绕开 REST 权重与签名延迟）。
func (t *FuturesTrader) PlaceLimitOrderWebSocketAPI(ctx context.Context, symbol, side, positionSide string, price, quantity float64, leverage int, reduceOnly, postOnly bool) (map[string]interface{}, error) {
	if err := t.SetMarginMode(symbol, true); err != nil {
		logger.Infof("  WS limit SetMarginMode %s: %v", symbol, err)
	}
	if leverage > 0 {
		if err := t.SetLeverage(symbol, leverage); err != nil {
			return nil, err
		}
	}
	qtyStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}
	qtyFloat, _ := strconv.ParseFloat(qtyStr, 64)
	if qtyFloat <= 0 {
		return nil, fmt.Errorf("limit qty too small after format: %s", qtyStr)
	}
	if err := t.CheckMinNotional(symbol, qtyFloat); err != nil {
		return nil, err
	}
	priceStr, _ := t.FormatPrice(symbol, price)

	cli, err := t.getOrCreateWSAPIClient(ctx)
	if err != nil {
		return nil, err
	}

	sideU := strings.ToUpper(strings.TrimSpace(side))
	ps := strings.ToUpper(strings.TrimSpace(positionSide))
	orderType := "LIMIT"
	tif := "GTC"
	if postOnly {
		tif = "GTX"
	}
	id := fmt.Sprintf("lm-%d", time.Now().UnixNano())

	params := map[string]interface{}{
		"symbol":           symbol,
		"side":             sideU,
		"positionSide":     ps,
		"type":             orderType,
		"timeInForce":      tif,
		"quantity":         qtyStr,
		"price":            priceStr,
		"newClientOrderId": getBrOrderID(),
	}
	if reduceOnly {
		params["reduceOnly"] = true
	}

	raw, err := cli.OrderPlace(ctx, id, params)
	if err != nil {
		t.resetWSAPIClient()
		return nil, err
	}
	res, err := wsAPIParseOrderResult(raw)
	if err != nil {
		return nil, err
	}
	logger.Infof("✓ WS limit: %s %s pos=%s @%s qty=%s reduceOnly=%v orderID=%v",
		symbol, sideU, ps, priceStr, qtyStr, reduceOnly, res["orderId"])
	return res, nil
}

// CancelOrderWebSocketAPI 通过 WS API 撤单（按 orderId 或 origClientOrderId）。
func (t *FuturesTrader) CancelOrderWebSocketAPI(ctx context.Context, symbol, orderID string, origClientOrderID string) error {
	cli, err := t.getOrCreateWSAPIClient(ctx)
	if err != nil {
		return err
	}
	params := map[string]interface{}{
		"symbol": symbol,
	}
	if strings.TrimSpace(orderID) != "" {
		params["orderId"] = orderID
	} else if strings.TrimSpace(origClientOrderID) != "" {
		params["origClientOrderId"] = origClientOrderID
	} else {
		return fmt.Errorf("cancel order requires orderId or origClientOrderId")
	}
	id := fmt.Sprintf("co-%d", time.Now().UnixNano())
	raw, err := cli.OrderCancel(ctx, id, params)
	if err != nil {
		t.resetWSAPIClient()
		return err
	}
	st, emsg, perr := wsAPIParseStatus(raw)
	if perr != nil {
		return perr
	}
	if st != 200 {
		return fmt.Errorf("cancel order ws status=%d msg=%s", st, emsg)
	}
	return nil
}

// ClosePartialLongWebSocketAPI WS API 市价部分平多（quantity>0 平指定张数）。
func (t *FuturesTrader) ClosePartialLongWebSocketAPI(ctx context.Context, symbol string, quantity float64) (map[string]interface{}, error) {
	qtyStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}
	qtyFloat, _ := strconv.ParseFloat(qtyStr, 64)
	if qtyFloat <= 0 {
		return nil, fmt.Errorf("close partial long qty too small: %s", qtyStr)
	}
	if err := t.CheckMinNotional(symbol, qtyFloat); err != nil {
		return nil, err
	}
	cli, err := t.getOrCreateWSAPIClient(ctx)
	if err != nil {
		return nil, err
	}
	id := fmt.Sprintf("cpl-%d", time.Now().UnixNano())
	raw, err := cli.OrderPlace(ctx, id, map[string]interface{}{
		"symbol":           symbol,
		"side":             "SELL",
		"positionSide":     "LONG",
		"type":             "MARKET",
		"quantity":         qtyStr,
		"reduceOnly":       true,
		"newClientOrderId": getBrOrderID(),
	})
	if err != nil {
		t.resetWSAPIClient()
		return nil, err
	}
	res, err := wsAPIParseOrderResult(raw)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"orderId": res["orderId"],
		"symbol":  symbol,
		"status":  res["status"],
	}, nil
}

// ClosePartialShortWebSocketAPI WS API 市价部分平空（quantity>0 平指定张数）。
func (t *FuturesTrader) ClosePartialShortWebSocketAPI(ctx context.Context, symbol string, quantity float64) (map[string]interface{}, error) {
	qtyStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}
	qtyFloat, _ := strconv.ParseFloat(qtyStr, 64)
	if qtyFloat <= 0 {
		return nil, fmt.Errorf("close partial short qty too small: %s", qtyStr)
	}
	if err := t.CheckMinNotional(symbol, qtyFloat); err != nil {
		return nil, err
	}
	cli, err := t.getOrCreateWSAPIClient(ctx)
	if err != nil {
		return nil, err
	}
	id := fmt.Sprintf("cps-%d", time.Now().UnixNano())
	raw, err := cli.OrderPlace(ctx, id, map[string]interface{}{
		"symbol":           symbol,
		"side":             "BUY",
		"positionSide":     "SHORT",
		"type":             "MARKET",
		"quantity":         qtyStr,
		"reduceOnly":       true,
		"newClientOrderId": getBrOrderID(),
	})
	if err != nil {
		t.resetWSAPIClient()
		return nil, err
	}
	res, err := wsAPIParseOrderResult(raw)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"orderId": res["orderId"],
		"symbol":  symbol,
		"status":  res["status"],
	}, nil
}

// CancelStopLossOrdersWebSocketAPI 通过 WS API 撤销止损单（对指定币种的所有止损挂单）。
// Binance WebSocket API 的 order.cancel 目前按单笔 orderId 撤；全币种撤仍用 REST CancelAllOrders（消息量少，REST 权重可接受）。
// 本方法仅用于 v2 需要 WS 撤单个止损单的场景。
func (t *FuturesTrader) CancelStopLossOrdersWebSocketAPI(ctx context.Context, symbol string) error {
	// 先获取当前挂单缓存，找出止损单
	ords, err := t.GetOpenOrders(symbol)
	if err != nil {
		return err
	}
	for _, o := range ords {
		if strings.EqualFold(strings.TrimSpace(o.Symbol), symbol) {
			typ := strings.ToUpper(strings.TrimSpace(o.Type))
			if typ == "STOP_MARKET" || typ == "STOP" {
				if err := t.CancelOrderWebSocketAPI(ctx, symbol, o.OrderID, ""); err != nil {
					logger.Warnf("WS cancel stop-loss %s orderID=%s: %v", symbol, o.OrderID, err)
				}
			}
		}
	}
	return nil
}

// CancelTakeProfitOrdersWebSocketAPI 通过 WS API 撤销止盈单。
func (t *FuturesTrader) CancelTakeProfitOrdersWebSocketAPI(ctx context.Context, symbol string) error {
	ords, err := t.GetOpenOrders(symbol)
	if err != nil {
		return err
	}
	for _, o := range ords {
		if strings.EqualFold(strings.TrimSpace(o.Symbol), symbol) {
			typ := strings.ToUpper(strings.TrimSpace(o.Type))
			if typ == "TAKE_PROFIT_MARKET" || typ == "TAKE_PROFIT" {
				if err := t.CancelOrderWebSocketAPI(ctx, symbol, o.OrderID, ""); err != nil {
					logger.Warnf("WS cancel take-profit %s orderID=%s: %v", symbol, o.OrderID, err)
				}
			}
		}
	}
	return nil
}
