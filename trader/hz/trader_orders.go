package hz

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"nofx/trader/types"
)

func (t *Trader) PlaceLimitOrder(request *types.LimitOrderRequest) (*types.LimitOrderResult, error) {
	if request.PostOnly || request.ReduceOnly {
		return nil, fmt.Errorf("HZ API does not support Post Only or reduce-only limit orders")
	}
	side := strings.ToUpper(request.PositionSide)
	if side == "" {
		if strings.EqualFold(request.Side, "BUY") {
			side = "LONG"
		} else {
			side = "SHORT"
		}
	}
	price := decimal(request.Price)
	placed, err := t.placeOrder(createOrderRequest{
		ClientOrderID: request.ClientID, Instrument: strings.ToUpper(request.Symbol),
		Side: side, OrderType: "LIMIT", SizeMode: "LOTS", Size: decimal(request.Quantity),
		Leverage: request.Leverage, MarginMode: t.marginMode(), LimitPrice: &price,
	})
	if err != nil {
		return nil, err
	}
	return &types.LimitOrderResult{
		OrderID: placed.OrderID, ClientID: placed.ClientOrderID, Symbol: placed.Instrument,
		Side: request.Side, PositionSide: placed.Side, Price: request.Price,
		Quantity: request.Quantity, Status: placed.Status,
	}, nil
}

func (t *Trader) CancelOrder(_ string, orderID string) error {
	var value order
	return t.client.do(context.Background(), http.MethodDelete,
		"/orders/"+url.PathEscape(orderID), nil, randomToken(), &value)
}

func (t *Trader) GetOrderBook(symbol string, _ int) ([][]float64, [][]float64, error) {
	var value quote
	query := url.Values{"instrument": []string{strings.ToUpper(symbol)}}
	if err := t.client.do(context.Background(), http.MethodGet, "/ticker", query, "", &value); err != nil {
		return nil, nil, err
	}
	return [][]float64{{number(value.Bid), 0}}, [][]float64{{number(value.Ask), 0}}, nil
}

func (t *Trader) Reconcile() error {
	if _, err := t.GetBalance(); err != nil {
		return err
	}
	if _, err := t.GetPositions(); err != nil {
		return err
	}
	_, err := t.GetOpenOrders("")
	return err
}

func (t *Trader) placeOrder(request createOrderRequest) (order, error) {
	if !t.IsReady() {
		return order{}, fmt.Errorf("HZ real-time connection is not ready; opening is blocked")
	}
	if err := t.SetLeverage(request.Instrument, request.Leverage); err != nil {
		return order{}, err
	}
	if request.ClientOrderID == "" {
		request.ClientOrderID = "comkun-" + randomToken()
	}
	var placed order
	err := t.client.doJSON(context.Background(), http.MethodPost, "/orders",
		request, &placed, request.ClientOrderID)
	if err == nil {
		return placed, nil
	}
	apiErr, isAPIError := err.(*APIError)
	if isAPIError && apiErr.Status < http.StatusInternalServerError {
		return order{}, err
	}
	for _, wait := range []time.Duration{0, 100 * time.Millisecond, 300 * time.Millisecond} {
		if wait > 0 {
			time.Sleep(wait)
		}
		if reconciled, lookupErr := t.orderByClientID(request.ClientOrderID); lookupErr == nil {
			return reconciled, nil
		}
	}
	return order{}, err
}

func (t *Trader) orderByClientID(clientOrderID string) (order, error) {
	var value order
	err := t.client.do(context.Background(), http.MethodGet,
		"/orders/by-client-id/"+url.PathEscape(clientOrderID), nil, "", &value)
	return value, err
}

func (t *Trader) positions() ([]position, error) {
	var values []position
	err := t.client.do(context.Background(), http.MethodGet, "/positions", nil, "", &values)
	return values, err
}

func (t *Trader) positionsFor(symbol, side string) ([]position, error) {
	values, err := t.positions()
	if err != nil {
		return nil, err
	}
	symbol = strings.ToUpper(symbol)
	side = strings.ToUpper(side)
	result := make([]position, 0)
	for _, item := range values {
		if item.Instrument == symbol && item.Side == side {
			result = append(result, item)
		}
	}
	return result, nil
}

func (t *Trader) setProtection(symbol, side, field, value string) error {
	positions, err := t.positionsFor(symbol, side)
	if err != nil {
		return err
	}
	if len(positions) == 0 {
		return fmt.Errorf("position not found for %s %s", symbol, side)
	}
	body := map[string]any{field: value}
	var updated position
	return t.client.doJSON(context.Background(), http.MethodPatch,
		"/positions/"+url.PathEscape(positions[0].PositionID)+"/protection",
		body, &updated, randomToken())
}

func (t *Trader) clearProtection(symbol, field string) error {
	positions, err := t.positions()
	if err != nil {
		return err
	}
	for _, item := range positions {
		if item.Instrument != strings.ToUpper(symbol) {
			continue
		}
		body := map[string]any{}
		if field == "stopLoss" || field == "both" {
			body["stopLoss"] = nil
		}
		if field == "takeProfit" || field == "both" {
			body["takeProfit"] = nil
		}
		var updated position
		if err := t.client.doJSON(context.Background(), http.MethodPatch,
			"/positions/"+url.PathEscape(item.PositionID)+"/protection",
			body, &updated, randomToken()); err != nil {
			return err
		}
	}
	return nil
}

func (t *Trader) marginMode() string {
	if t.crossMargin.Load() {
		return "CROSS"
	}
	return "ISOLATED"
}

func orderResult(value order) map[string]interface{} {
	return map[string]interface{}{
		"orderId": value.OrderID, "clientOrderId": value.ClientOrderID,
		"symbol": value.Instrument, "status": value.Status,
		"avgPrice": 0.0, "executedQty": number(value.Lots), "commission": 0.0,
	}
}

func orderSide(side string) string {
	if side == "LONG" {
		return "BUY"
	}
	return "SELL"
}

func number(value string) float64 {
	result, _ := strconv.ParseFloat(value, 64)
	return result
}

func pointerNumber(value *string) float64 {
	if value == nil {
		return 0
	}
	return number(*value)
}

func decimal(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func unixMillis(value string) int64 {
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed.UnixMilli()
}
