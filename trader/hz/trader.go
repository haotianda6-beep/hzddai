package hz

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"nofx/trader/types"
)

type Trader struct {
	client                *client
	crossMargin           atomic.Bool
	streamReady           atomic.Bool
	reconcileHealthy      atomic.Bool
	streamOpeningStopped  atomic.Bool
	accountOpeningStopped atomic.Bool
	cancelStream          context.CancelFunc
}

func NewTrader(apiURL, apiKey, secret string, crossMargin bool) (*Trader, error) {
	if apiKey == "" || secret == "" {
		return nil, fmt.Errorf("HZ API key and secret are required")
	}
	client, err := newClient(apiURL, apiKey, secret)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	trader := &Trader{client: client, cancelStream: cancel}
	trader.crossMargin.Store(crossMargin)
	go trader.runStream(ctx)
	go trader.reconcileLoop(ctx, time.Minute)
	return trader, nil
}

func (t *Trader) Close() { t.cancelStream() }
func (t *Trader) IsReady() bool {
	return t.streamReady.Load() && t.reconcileHealthy.Load() &&
		!t.streamOpeningStopped.Load() && !t.accountOpeningStopped.Load()
}

func (t *Trader) GetBalance() (map[string]interface{}, error) {
	var value account
	if err := t.client.do(context.Background(), http.MethodGet, "/account", nil, "", &value); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"total_equity":          number(value.Equity),
		"totalWalletBalance":    number(value.Balance),
		"availableBalance":      number(value.AvailableMargin),
		"usedMargin":            number(value.UsedMargin),
		"totalUnrealizedProfit": number(value.UnrealizedPnL),
		"marginRatio":           number(value.MarginRatio),
	}, nil
}

func (t *Trader) GetPositions() ([]map[string]interface{}, error) {
	positions, err := t.positions()
	if err != nil {
		return nil, err
	}
	result := make([]map[string]interface{}, 0, len(positions))
	for _, item := range positions {
		result = append(result, map[string]interface{}{
			"positionId":       item.PositionID,
			"symbol":           item.Instrument,
			"positionAmt":      number(item.Lots),
			"entryPrice":       number(item.EntryPrice),
			"markPrice":        number(item.CurrentPrice),
			"unRealizedProfit": number(item.UnrealizedPnL),
			"leverage":         float64(item.Leverage),
			"liquidationPrice": 0.0,
			"side":             strings.ToLower(item.Side),
			"mgnMode":          strings.ToLower(item.MarginMode),
			"createdTime":      unixMillis(item.OpenedAt),
		})
	}
	return result, nil
}

func (t *Trader) OpenLong(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	return t.open(symbol, "LONG", quantity, leverage)
}

func (t *Trader) OpenShort(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	return t.open(symbol, "SHORT", quantity, leverage)
}

func (t *Trader) open(symbol, side string, quantity float64, leverage int) (map[string]interface{}, error) {
	request := createOrderRequest{
		Instrument: strings.ToUpper(symbol), Side: side, OrderType: "MARKET",
		SizeMode: "LOTS", Size: decimal(quantity), Leverage: leverage,
		MarginMode: t.marginMode(),
	}
	placed, err := t.placeOrder(request)
	if err != nil {
		return nil, err
	}
	return orderResult(placed), nil
}

func (t *Trader) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	return t.closePosition(symbol, "LONG", quantity)
}

func (t *Trader) CloseShort(symbol string, quantity float64) (map[string]interface{}, error) {
	return t.closePosition(symbol, "SHORT", quantity)
}

func (t *Trader) closePosition(symbol, side string, quantity float64) (map[string]interface{}, error) {
	positions, err := t.positionsFor(symbol, side)
	if err != nil {
		return nil, err
	}
	if len(positions) == 0 {
		return nil, fmt.Errorf("%s position not found for %s", strings.ToLower(side), symbol)
	}
	remaining := quantity
	var last positionAction
	for _, item := range positions {
		size := number(item.Lots)
		if quantity > 0 && remaining < size {
			size = remaining
		}
		body := map[string]string{"lots": decimal(size)}
		if err := t.client.doJSON(context.Background(), http.MethodPost,
			"/positions/"+url.PathEscape(item.PositionID)+"/close", body, &last, randomToken()); err != nil {
			return nil, err
		}
		if quantity > 0 {
			remaining -= size
			if remaining <= 0 {
				break
			}
		}
	}
	return map[string]interface{}{
		"orderId": last.ClosedPosition.PositionID, "symbol": strings.ToUpper(symbol),
		"status": "FILLED", "fillPrice": number(last.ClosedPosition.CurrentPrice),
		"avgPrice": number(last.ClosedPosition.CurrentPrice),
	}, nil
}

func (t *Trader) SetLeverage(_ string, leverage int) error {
	if leverage < 100 || leverage > 2000 {
		return fmt.Errorf("HZ leverage must be between 100 and 2000")
	}
	return nil
}

func (t *Trader) SetMarginMode(_ string, isCrossMargin bool) error {
	t.crossMargin.Store(isCrossMargin)
	return nil
}

func (t *Trader) GetMarketPrice(symbol string) (float64, error) {
	var value quote
	query := url.Values{"instrument": []string{strings.ToUpper(symbol)}}
	if err := t.client.do(context.Background(), http.MethodGet, "/ticker", query, "", &value); err != nil {
		return 0, err
	}
	if value.SourceStatus != "TRADING" {
		return 0, fmt.Errorf("HZ quote is %s", value.SourceStatus)
	}
	return number(value.Reference), nil
}

func (t *Trader) SetStopLoss(symbol, side string, _ float64, price float64) error {
	return t.setProtection(symbol, side, "stopLoss", decimal(price))
}

func (t *Trader) SetTakeProfit(symbol, side string, _ float64, price float64) error {
	return t.setProtection(symbol, side, "takeProfit", decimal(price))
}

func (t *Trader) CancelStopLossOrders(symbol string) error {
	return t.clearProtection(symbol, "stopLoss")
}

func (t *Trader) CancelTakeProfitOrders(symbol string) error {
	return t.clearProtection(symbol, "takeProfit")
}

func (t *Trader) CancelStopOrders(symbol string) error {
	return t.clearProtection(symbol, "both")
}

func (t *Trader) CancelAllOrders(symbol string) error {
	orders, err := t.GetOpenOrders(symbol)
	if err != nil {
		return err
	}
	for _, item := range orders {
		if err := t.CancelOrder(symbol, item.OrderID); err != nil {
			return err
		}
	}
	return nil
}

func (t *Trader) FormatQuantity(symbol string, quantity float64) (string, error) {
	var instruments []instrument
	if err := t.client.do(context.Background(), http.MethodGet, "/instruments", nil, "", &instruments); err != nil {
		return "", err
	}
	for _, item := range instruments {
		if item.Instrument == strings.ToUpper(symbol) {
			return strconv.FormatFloat(quantity, 'f', item.LotPrecision, 64), nil
		}
	}
	return "", fmt.Errorf("HZ instrument not found: %s", symbol)
}

func (t *Trader) GetOrderStatus(_ string, orderID string) (map[string]interface{}, error) {
	var value order
	if err := t.client.do(context.Background(), http.MethodGet,
		"/orders/"+url.PathEscape(orderID), nil, "", &value); err != nil {
		return nil, err
	}
	return orderResult(value), nil
}

func (t *Trader) GetClosedPnL(startTime time.Time, limit int) ([]types.ClosedPnLRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var page tradePage
	query := url.Values{"limit": []string{strconv.Itoa(limit)}}
	if err := t.client.do(context.Background(), http.MethodGet, "/trades", query, "", &page); err != nil {
		return nil, err
	}
	result := make([]types.ClosedPnLRecord, 0, len(page.Items))
	for _, item := range page.Items {
		executed, _ := time.Parse(time.RFC3339Nano, item.ExecutedAt)
		if executed.Before(startTime) {
			continue
		}
		result = append(result, types.ClosedPnLRecord{
			Symbol: item.Instrument, Side: strings.ToLower(item.Side), ExitPrice: number(item.Price),
			Quantity: number(item.Lots), RealizedPnL: number(item.RealizedPnL),
			Fee: number(item.Fee), ExitTime: executed, OrderID: item.OrderID,
			CloseType: "unknown",
		})
	}
	return result, nil
}

func (t *Trader) GetOpenOrders(symbol string) ([]types.OpenOrder, error) {
	var values []order
	if err := t.client.do(context.Background(), http.MethodGet, "/orders/open", nil, "", &values); err != nil {
		return nil, err
	}
	result := make([]types.OpenOrder, 0, len(values))
	for _, item := range values {
		if symbol != "" && item.Instrument != strings.ToUpper(symbol) {
			continue
		}
		result = append(result, types.OpenOrder{
			OrderID: item.OrderID, Symbol: item.Instrument, Side: orderSide(item.Side),
			PositionSide: item.Side, Type: item.OrderType, Price: pointerNumber(item.LimitPrice),
			StopPrice: pointerNumber(item.TriggerPrice), Quantity: number(item.Lots), Status: item.Status,
		})
	}
	return result, nil
}
