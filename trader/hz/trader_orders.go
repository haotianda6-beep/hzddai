package hz

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"nofx/trader/types"
)

func (t *Trader) PlaceLimitOrder(request *types.LimitOrderRequest) (*types.LimitOrderResult, error) {
	return nil, fmt.Errorf("HZ AI scope supports MARKET orders only")
}

func (t *Trader) CancelOrder(_ string, orderID string) error {
	if !t.scopeVerified.Load() {
		return fmt.Errorf("HZ AI account scope is not verified")
	}
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

func (t *Trader) Reconcile() (err error) {
	defer func() { t.reconcileHealthy.Store(err == nil) }()
	var snapshot account
	if err = t.client.do(context.Background(), http.MethodGet, "/account", nil, "", &snapshot); err != nil {
		return err
	}
	t.accountOpeningStopped.Store(!snapshot.Tradable)
	positions, err := t.positions()
	if err != nil {
		return err
	}
	if len(positions) > 0 && t.markPrices != nil {
		t.markPrices.start()
	}
	_, err = t.GetOpenOrders("")
	return err
}

func (t *Trader) placeOrder(request createOrderRequest) (order, error) {
	if !t.scopeVerified.Load() {
		return order{}, fmt.Errorf("HZ AI account scope is not verified")
	}
	if !t.IsReady() {
		return order{}, fmt.Errorf("%s", t.openingBlockReason())
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

func (t *Trader) openingBlockReason() string {
	switch {
	case !t.reconcileHealthy.Load():
		return "HZ 账户对账失败，已停止新开仓"
	case t.streamOpeningStopped.Load():
		return "HZ 平台已停止 API 新开仓"
	case t.accountOpeningStopped.Load():
		return "HZ 账户当前不可交易，已停止新开仓"
	default:
		return "HZ 平台已停止 API 新开仓"
	}
}

func (t *Trader) reconcileOnce() {
	_ = t.Reconcile()
}

func (t *Trader) reconcileLoop(ctx context.Context, interval time.Duration) {
	t.reconcileOnce()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.reconcileOnce()
		}
	}
}

func (t *Trader) orderByClientID(clientOrderID string) (order, error) {
	var value order
	err := t.client.do(context.Background(), http.MethodGet,
		"/orders/by-client-id/"+url.PathEscape(clientOrderID), nil, "", &value)
	return value, err
}

// LookupOrderByClientID recovers an order accepted by B before an A process
// interruption. Callers must reuse the same clientOrderId and never mint a new
// identifier for the same execution intent.
func (t *Trader) LookupOrderByClientID(clientOrderID string) (map[string]interface{}, error) {
	if !t.scopeVerified.Load() {
		return nil, fmt.Errorf("HZ AI account scope is not verified")
	}
	value, err := t.orderByClientID(clientOrderID)
	if err != nil {
		return nil, err
	}
	return orderResult(value, 0), nil
}

func IsNotFound(err error) bool {
	apiErr, ok := err.(*APIError)
	return ok && apiErr.Status == http.StatusNotFound
}

func (t *Trader) positions() ([]position, error) {
	var values []position
	if err := t.client.do(context.Background(), http.MethodGet, "/positions", nil, "", &values); err != nil {
		return nil, err
	}
	t.positionMu.Lock()
	t.positionCache = append(t.positionCache[:0], values...)
	t.positionCacheReady = true
	t.positionMu.Unlock()
	return values, nil
}

func (t *Trader) positionSnapshot() ([]position, error) {
	t.positionMu.RLock()
	if t.positionCacheReady {
		values := append([]position(nil), t.positionCache...)
		t.positionMu.RUnlock()
		return values, nil
	}
	t.positionMu.RUnlock()
	return t.positions()
}

func (t *Trader) invalidatePositionCache() {
	t.positionMu.Lock()
	t.positionCacheReady = false
	t.positionMu.Unlock()
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
	if !t.scopeVerified.Load() {
		return fmt.Errorf("HZ AI account scope is not verified")
	}
	positions, err := t.positionsFor(symbol, side)
	if err != nil {
		return err
	}
	if len(positions) == 0 {
		return fmt.Errorf("position not found for %s %s", symbol, side)
	}
	if len(positions) != 1 {
		return fmt.Errorf("ambiguous HZ %s %s positions: exact position ID is required", symbol, strings.ToLower(side))
	}
	body := map[string]any{field: value}
	var updated position
	return t.client.doJSON(context.Background(), http.MethodPatch,
		"/positions/"+url.PathEscape(positions[0].PositionID)+"/protection",
		body, &updated, randomToken())
}

func (t *Trader) clearProtection(symbol, field string) error {
	if !t.scopeVerified.Load() {
		return fmt.Errorf("HZ AI account scope is not verified")
	}
	positions, err := t.positions()
	if err != nil {
		return err
	}
	filtered := make([]position, 0, 1)
	for _, item := range positions {
		if item.Instrument != strings.ToUpper(symbol) {
			continue
		}
		filtered = append(filtered, item)
	}
	if len(filtered) == 0 {
		return nil
	}
	if len(filtered) != 1 {
		return fmt.Errorf("ambiguous HZ %s positions: exact position ID is required", symbol)
	}
	body := map[string]any{}
	if field == "stopLoss" || field == "both" {
		body["stopLoss"] = nil
	}
	if field == "takeProfit" || field == "both" {
		body["takeProfit"] = nil
	}
	var updated position
	return t.client.doJSON(context.Background(), http.MethodPatch,
		"/positions/"+url.PathEscape(filtered[0].PositionID)+"/protection",
		body, &updated, randomToken())
}

func (t *Trader) marginMode() string {
	if t.crossMargin.Load() {
		return "CROSS"
	}
	return "ISOLATED"
}

func orderResult(value order, quantity float64) map[string]interface{} {
	return map[string]interface{}{
		"orderId": value.OrderID, "clientOrderId": value.ClientOrderID,
		"symbol": value.Instrument, "status": value.Status,
		"avgPrice": 0.0, "executedQty": quantity, "commission": 0.0,
	}
}

func (t *Trader) instruments() (map[string]instrument, error) {
	t.instrumentMu.RLock()
	cached := t.instrumentCache
	t.instrumentMu.RUnlock()
	if cached != nil {
		return cached, nil
	}
	var values []instrument
	if err := t.client.do(context.Background(), http.MethodGet, "/instruments", nil, "", &values); err != nil {
		return nil, err
	}
	result := make(map[string]instrument, len(values))
	for _, item := range values {
		result[item.Instrument] = item
	}
	t.instrumentMu.Lock()
	if t.instrumentCache == nil {
		t.instrumentCache = result
	}
	cached = t.instrumentCache
	t.instrumentMu.Unlock()
	return cached, nil
}

func (t *Trader) instrument(symbol string) (instrument, error) {
	values, err := t.instruments()
	if err != nil {
		return instrument{}, err
	}
	value, ok := values[strings.ToUpper(symbol)]
	if !ok {
		return instrument{}, fmt.Errorf("HZ instrument not found: %s", symbol)
	}
	return value, nil
}

func (t *Trader) lotsForQuantity(symbol string, quantity float64) (string, error) {
	spec, err := t.instrument(symbol)
	if err != nil {
		return "", err
	}
	return t.lotsForQuantityWithSpec(spec, quantity)
}

func (t *Trader) lotsForQuantityWithSpec(spec instrument, quantity float64) (string, error) {
	contractSize := number(spec.ContractSize)
	minLots := number(spec.MinLots)
	maxLots := number(spec.MaxLots)
	if !finite(quantity) || quantity <= 0 || !finite(contractSize) || contractSize <= 0 ||
		!finite(minLots) || minLots < 0 || !finite(maxLots) || maxLots < 0 {
		return "", fmt.Errorf("invalid HZ quantity for %s", spec.Instrument)
	}
	step := number(spec.LotStep)
	if !finite(step) || step <= 0 {
		step = math.Pow10(-spec.LotPrecision)
	}
	lots := math.Floor((quantity/contractSize+step*1e-9)/step) * step
	formatted := strconv.FormatFloat(lots, 'f', spec.LotPrecision, 64)
	if lots < minLots || (maxLots > 0 && lots > maxLots) {
		return "", fmt.Errorf("HZ quantity for %s is outside the allowed lot range", spec.Instrument)
	}
	return formatted, nil
}

func quantityForLots(spec instrument, lots string) (float64, error) {
	contractSize := number(spec.ContractSize)
	lotValue := number(lots)
	if !finite(contractSize) || contractSize <= 0 || !finite(lotValue) || lotValue < 0 {
		return 0, fmt.Errorf("invalid HZ instrument quantity")
	}
	return lotValue * contractSize, nil
}

// QuantityForLots converts B's canonical lot count using the cached dynamic
// contract specification for the instrument.
func (t *Trader) QuantityForLots(symbol string, lots float64) (float64, error) {
	if !finite(lots) || lots <= 0 {
		return 0, fmt.Errorf("invalid HZ lots for %s", symbol)
	}
	spec, err := t.instrument(symbol)
	if err != nil {
		return 0, err
	}
	return quantityForLots(spec, strconv.FormatFloat(lots, 'f', -1, 64))
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
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
