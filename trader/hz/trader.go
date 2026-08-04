package hz

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nofx/trader/types"
)

type Trader struct {
	client                  *client
	crossMargin             atomic.Bool
	streamReady             atomic.Bool
	reconcileHealthy        atomic.Bool
	streamOpeningStopped    atomic.Bool
	accountOpeningStopped   atomic.Bool
	instrumentMu            sync.RWMutex
	instrumentCache         map[string]instrument
	scopeVerified           atomic.Bool
	accountFingerprint      string
	walletFingerprint       string
	positionBookFingerprint string
	intentExecutionMu       sync.Mutex
	intentMu                sync.Mutex
	nextIntent              string
	cancelStream            context.CancelFunc
}

const followerRESTReconcileInterval = 15 * time.Second

func NewTrader(apiURL, apiKey, secret string, crossMargin bool) (*Trader, error) {
	if apiKey == "" || secret == "" {
		return nil, fmt.Errorf("HZ API key and secret are required")
	}
	client, err := newClient(apiURL, apiKey, secret)
	if err != nil {
		return nil, err
	}
	verified, err := verifyCapabilities(client)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	trader := &Trader{
		client: client, cancelStream: cancel,
		accountFingerprint:      verified.AccountFingerprint,
		walletFingerprint:       verified.WalletFingerprint,
		positionBookFingerprint: verified.PositionBookFingerprint,
	}
	trader.crossMargin.Store(crossMargin)
	trader.scopeVerified.Store(true)
	if _, err := trader.instruments(); err != nil {
		cancel()
		return nil, fmt.Errorf("load HZ instruments: %w", err)
	}
	go trader.reconcileLoop(ctx, followerRESTReconcileInterval)
	return trader, nil
}

func (t *Trader) Close() { t.cancelStream() }
func (t *Trader) IsReady() bool {
	return t.reconcileHealthy.Load() &&
		!t.streamOpeningStopped.Load() && !t.accountOpeningStopped.Load()
}

func (t *Trader) GetBalance() (map[string]interface{}, error) {
	var value account
	if err := t.client.do(context.Background(), http.MethodGet, "/account", nil, "", &value); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"total_equity":          number(value.Equity),
		"totalEquity":           number(value.Equity),
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
	if len(positions) == 0 {
		return []map[string]interface{}{}, nil
	}
	instruments, err := t.instruments()
	if err != nil {
		return nil, err
	}
	result := make([]map[string]interface{}, 0, len(positions))
	for _, item := range positions {
		quantity, err := quantityForLots(instruments[item.Instrument], item.Lots)
		if err != nil {
			return nil, fmt.Errorf("invalid HZ position %s: %w", item.PositionID, err)
		}
		if item.Side == "SHORT" {
			quantity = -quantity
		}
		result = append(result, map[string]interface{}{
			"positionId":       item.PositionID,
			"symbol":           item.Instrument,
			"positionAmt":      quantity,
			"entryPrice":       number(item.EntryPrice),
			"markPrice":        number(item.CurrentPrice),
			"unRealizedProfit": number(item.UnrealizedPnL),
			"leverage":         float64(item.Leverage),
			"takeProfit":       pointerNumber(item.TakeProfit),
			"stopLoss":         pointerNumber(item.StopLoss),
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
	if !t.scopeVerified.Load() {
		return nil, fmt.Errorf("HZ AI account scope is not verified")
	}
	if !t.IsReady() {
		return nil, fmt.Errorf("%s", t.openingBlockReason())
	}
	lots, err := t.lotsForQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}
	request := createOrderRequest{
		ClientOrderID: t.popNextIntent(), Instrument: strings.ToUpper(symbol), Side: side, OrderType: "MARKET",
		SizeMode: "LOTS", Size: lots, Leverage: leverage,
		MarginMode: t.marginMode(),
	}
	if request.ClientOrderID == "" {
		return nil, fmt.Errorf("HZ opening requires a fixed execution intent")
	}
	placed, err := t.placeOrder(request)
	if err != nil {
		return nil, err
	}
	return orderResult(placed, quantity), nil
}

func (t *Trader) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	return t.closePosition(symbol, "LONG", quantity)
}

func (t *Trader) CloseShort(symbol string, quantity float64) (map[string]interface{}, error) {
	return t.closePosition(symbol, "SHORT", quantity)
}

func (t *Trader) closePosition(symbol, side string, quantity float64) (map[string]interface{}, error) {
	if !t.scopeVerified.Load() {
		return nil, fmt.Errorf("HZ AI account scope is not verified")
	}
	positions, err := t.positionsFor(symbol, side)
	if err != nil {
		return nil, err
	}
	if len(positions) == 0 {
		return nil, fmt.Errorf("%s position not found for %s", strings.ToLower(side), symbol)
	}
	if len(positions) != 1 {
		return nil, fmt.Errorf("ambiguous HZ %s %s positions: exact position ID is required", symbol, strings.ToLower(side))
	}
	return t.closePositionItem(positions[0], quantity)
}

// ClosePositionByID closes only the server-verified AI position identified by positionID.
func (t *Trader) ClosePositionByID(positionID string, quantity float64) (map[string]interface{}, error) {
	if !t.scopeVerified.Load() {
		return nil, fmt.Errorf("HZ AI account scope is not verified")
	}
	positionID = strings.TrimSpace(positionID)
	if positionID == "" {
		return nil, fmt.Errorf("HZ position ID is required")
	}
	positions, err := t.positions()
	if err != nil {
		return nil, err
	}
	for _, item := range positions {
		if item.PositionID == positionID {
			return t.closePositionItem(item, quantity)
		}
	}
	return nil, fmt.Errorf("HZ position not found")
}

func (t *Trader) closePositionItem(item position, quantity float64) (map[string]interface{}, error) {
	remainingLots := 0.0
	lotPrecision := 0
	if quantity > 0 {
		spec, err := t.instrument(item.Instrument)
		if err != nil {
			return nil, err
		}
		lots, err := t.lotsForQuantityWithSpec(spec, quantity)
		if err != nil {
			return nil, err
		}
		remainingLots = number(lots)
		lotPrecision = spec.LotPrecision
		totalLots := number(item.Lots)
		if remainingLots > totalLots+1e-9 {
			return nil, fmt.Errorf("close quantity exceeds HZ position")
		}
	}
	closeLots := item.Lots
	if quantity > 0 {
		closeLots = strconv.FormatFloat(remainingLots, 'f', lotPrecision, 64)
	}
	intent := t.popNextIntent()
	if intent == "" {
		return nil, fmt.Errorf("HZ close requires a fixed execution intent")
	}
	var last positionAction
	body := map[string]string{"lots": closeLots}
	if err := t.client.doJSON(context.Background(), http.MethodPost,
		"/positions/"+url.PathEscape(item.PositionID)+"/close", body, &last, intent); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"orderId": last.ClosedPosition.PositionID, "symbol": strings.ToUpper(item.Instrument),
		"status": "FILLED", "fillPrice": number(last.ClosedPosition.CurrentPrice),
		"avgPrice": number(last.ClosedPosition.CurrentPrice),
	}, nil
}

func (t *Trader) SetLeverage(_ string, leverage int) error {
	if leverage <= 0 {
		return fmt.Errorf("HZ leverage must be positive")
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
	if !t.scopeVerified.Load() {
		return fmt.Errorf("HZ AI account scope is not verified")
	}
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

// SetNextIntent supplies the stable client/idempotency key for exactly one
// subsequent position-changing call.
func (t *Trader) SetNextIntent(clientOrderID string) {
	t.intentMu.Lock()
	t.nextIntent = strings.TrimSpace(clientOrderID)
	t.intentMu.Unlock()
}

// ExecuteWithIntent serializes the one-shot intent handoff so a failed
// preflight or a concurrent call cannot leak the clientOrderId to another
// order.
func (t *Trader) ExecuteWithIntent(clientOrderID string, execute func() (map[string]interface{}, error)) (map[string]interface{}, error) {
	clientOrderID = strings.TrimSpace(clientOrderID)
	if clientOrderID == "" || execute == nil {
		return nil, fmt.Errorf("HZ execution intent is required")
	}
	t.intentExecutionMu.Lock()
	defer t.intentExecutionMu.Unlock()
	t.SetNextIntent(clientOrderID)
	defer t.clearNextIntent()
	return execute()
}

func (t *Trader) clearNextIntent() {
	t.intentMu.Lock()
	t.nextIntent = ""
	t.intentMu.Unlock()
}

func (t *Trader) popNextIntent() string {
	t.intentMu.Lock()
	defer t.intentMu.Unlock()
	value := t.nextIntent
	t.nextIntent = ""
	return value
}

func (t *Trader) FormatQuantity(symbol string, quantity float64) (string, error) {
	spec, err := t.instrument(symbol)
	if err != nil {
		return "", err
	}
	lots, err := t.lotsForQuantityWithSpec(spec, quantity)
	if err != nil {
		return "", err
	}
	formatted, err := quantityForLots(spec, lots)
	if err != nil {
		return "", err
	}
	return strconv.FormatFloat(formatted, 'f', -1, 64), nil
}

func (t *Trader) GetOrderStatus(_ string, orderID string) (map[string]interface{}, error) {
	var value order
	if err := t.client.do(context.Background(), http.MethodGet,
		"/orders/"+url.PathEscape(orderID), nil, "", &value); err != nil {
		return nil, err
	}
	spec, err := t.instrument(value.Instrument)
	if err != nil {
		return nil, err
	}
	quantity, err := quantityForLots(spec, value.Lots)
	if err != nil {
		return nil, err
	}
	return orderResult(value, quantity), nil
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
	if len(page.Items) == 0 {
		return []types.ClosedPnLRecord{}, nil
	}
	instruments, err := t.instruments()
	if err != nil {
		return nil, err
	}
	result := make([]types.ClosedPnLRecord, 0, len(page.Items))
	for _, item := range page.Items {
		executed, _ := time.Parse(time.RFC3339Nano, item.ExecutedAt)
		if executed.Before(startTime) {
			continue
		}
		quantity, err := quantityForLots(instruments[item.Instrument], item.Lots)
		if err != nil {
			return nil, fmt.Errorf("invalid HZ trade %s: %w", item.TradeID, err)
		}
		result = append(result, types.ClosedPnLRecord{
			Symbol: item.Instrument, Side: strings.ToLower(item.Side), ExitPrice: number(item.Price),
			Quantity: quantity, RealizedPnL: number(item.RealizedPnL),
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
	if len(values) == 0 {
		return []types.OpenOrder{}, nil
	}
	instruments, err := t.instruments()
	if err != nil {
		return nil, err
	}
	result := make([]types.OpenOrder, 0, len(values))
	for _, item := range values {
		if symbol != "" && item.Instrument != strings.ToUpper(symbol) {
			continue
		}
		quantity, err := quantityForLots(instruments[item.Instrument], item.Lots)
		if err != nil {
			return nil, fmt.Errorf("invalid HZ order %s: %w", item.OrderID, err)
		}
		result = append(result, types.OpenOrder{
			OrderID: item.OrderID, Symbol: item.Instrument, Side: orderSide(item.Side),
			PositionSide: item.Side, Type: item.OrderType, Price: pointerNumber(item.LimitPrice),
			StopPrice: pointerNumber(item.TriggerPrice), Quantity: quantity, Status: item.Status,
		})
	}
	return result, nil
}
