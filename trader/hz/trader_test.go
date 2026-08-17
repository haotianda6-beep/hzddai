package hz

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestOpenLongUsesLotsAndReconcilesTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/instruments":
			_ = json.NewEncoder(w).Encode([]instrument{{
				Instrument: "XAUUSD", LotPrecision: 3, MinLots: "0.001",
				MaxLots: "100", LotStep: "0.001", ContractSize: "100",
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/orders":
			var input createOrderRequest
			_ = json.NewDecoder(r.Body).Decode(&input)
			if input.Side != "LONG" || input.SizeMode != "LOTS" || input.Size != "0.010" || input.Leverage != 500 {
				t.Errorf("wrong order payload: %#v", input)
			}
			time.Sleep(40 * time.Millisecond)
			writeOrder(w, input.ClientOrderID, "FILLED")
		case strings.HasPrefix(r.URL.Path, "/api/v1/orders/by-client-id/"):
			writeOrder(w, strings.TrimPrefix(r.URL.Path, "/api/v1/orders/by-client-id/"), "FILLED")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	trader := newTestTrader(t, server.URL+"/api/v1", true)
	lastPrices := newBinanceLastPriceFeed("ws://127.0.0.1:1/%s", "")
	lastPrices.watchTTL = time.Minute
	trader.lastPrices = lastPrices
	trader.markPrices = newBinanceMarkPriceFeed("")
	trader.client.http.Timeout = 10 * time.Millisecond
	trader.streamReady.Store(true)
	trader.reconcileHealthy.Store(true)
	trader.SetNextIntent("comkun-open-test")
	result, err := trader.OpenLong("XAUUSD", 1, 500)
	if err != nil {
		t.Fatal(err)
	}
	if result["status"] != "FILLED" || result["orderId"] == "" {
		t.Fatalf("wrong result: %#v", result)
	}
	if !lastPrices.isWatched("XAUUSD", time.Now()) {
		t.Fatal("market order must watch its symbol before a position query")
	}
}

func TestOpenIsBlockedWhileStreamIsDisconnected(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	trader := newTestTrader(t, server.URL+"/api/v1", true)
	if _, err := trader.OpenShort("XAUUSD", 0.01, 500); err == nil ||
		!strings.Contains(err.Error(), "已停止新开仓") {
		t.Fatalf("expected Chinese disconnected guard, got %v", err)
	}
}

func TestGetPositionsMapsHZSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/instruments" {
			writeInstruments(w)
			return
		}
		_ = json.NewEncoder(w).Encode([]position{
			{
				PositionID: "pos-1", Instrument: "XAUUSD", Side: "LONG", MarginMode: "CROSS",
				Leverage: 500, Lots: "0.010", EntryPrice: "2400.10", CurrentPrice: "2401.20",
				UnrealizedPnL: "1.10", OpenedAt: "2026-07-25T00:00:00Z",
			},
			{
				PositionID: "pos-2", Instrument: "XAGUSD", Side: "SHORT", MarginMode: "ISOLATED",
				Leverage: 200, Lots: "0.002", EntryPrice: "30", CurrentPrice: "29.9",
				UnrealizedPnL: "1", OpenedAt: "2026-07-25T00:00:00Z",
			},
		})
	}))
	defer server.Close()

	trader := newTestTrader(t, server.URL+"/api/v1", true)
	positions, err := trader.GetPositions()
	if err != nil {
		t.Fatal(err)
	}
	if len(positions) != 2 || positions[0]["side"] != "long" || positions[0]["positionAmt"] != 1.0 ||
		positions[0]["leverage"] != float64(500) || positions[0]["liquidationPrice"] != 0.0 ||
		positions[1]["side"] != "short" || positions[1]["positionAmt"] != -10.0 {
		t.Fatalf("wrong positions: %#v", positions)
	}
}

func TestGetPositionsUsesFreshBinanceMarkForPriceAndPnL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/instruments":
			_ = json.NewEncoder(w).Encode([]instrument{{
				Instrument: "NXPCUSDT", LotPrecision: 2, MinLots: "0.01",
				MaxLots: "100", LotStep: "0.01", ContractSize: "1",
			}})
		case "/api/v1/positions":
			_ = json.NewEncoder(w).Encode([]position{{
				PositionID: "pos-live", Instrument: "NXPCUSDT", Side: "LONG",
				MarginMode: "CROSS", Leverage: 10, Lots: "10", EntryPrice: "0.2000",
				CurrentPrice: "0.2100", UnrealizedPnL: "0.1000",
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	trader := newTestTrader(t, server.URL+"/api/v1", true)
	feed := newBinanceMarkPriceFeed("")
	receivedAt := time.Now()
	feed.apply([]binanceMarkPriceEvent{{
		EventTime: unixMillisValue(receivedAt.Add(-100 * time.Millisecond).UnixMilli()),
		Symbol:    "NXPCUSDT", MarkPrice: "0.2300",
	}}, receivedAt)
	trader.markPrices = feed
	lastTrades := newBinanceLastPriceFeed("", "")
	lastTrades.apply(binanceAggTradeEvent{
		EventTime: unixMillisValue(receivedAt.Add(-90 * time.Millisecond).UnixMilli()),
		TradeTime: unixMillisValue(receivedAt.Add(-100 * time.Millisecond).UnixMilli()),
		Symbol:    "NXPCUSDT", Price: "0.2250",
	}, receivedAt, "binance_agg_trade_ws")
	trader.lastPrices = lastTrades

	positions, err := trader.GetPositions()
	if err != nil {
		t.Fatal(err)
	}
	if len(positions) != 1 {
		t.Fatalf("positions=%d, want 1", len(positions))
	}
	if got := positions[0]["markPrice"]; got != 0.23 {
		t.Fatalf("markPrice=%v, want 0.23", got)
	}
	if got := positions[0]["unRealizedProfit"]; math.Abs(got.(float64)-0.3) > 1e-9 {
		t.Fatalf("unRealizedProfit=%v, want 0.3", got)
	}
	if got := positions[0]["markPriceSource"]; got != "binance_mark_ws" {
		t.Fatalf("markPriceSource=%v, want binance_mark_ws", got)
	}
	if got := positions[0]["lastPrice"]; got != 0.225 {
		t.Fatalf("lastPrice=%v, want 0.225", got)
	}
	if got := positions[0]["lastPriceTime"]; got != int64(lastTrades.prices["NXPCUSDT"].eventTime.UnixMilli()) {
		t.Fatalf("lastPriceTime=%v", got)
	}
	if got := positions[0]["lastPriceSource"]; got != "binance_agg_trade_ws" {
		t.Fatalf("lastPriceSource=%v, want binance_agg_trade_ws", got)
	}
}

func TestStaleBinanceMarkFallsBackToHZPositionSnapshot(t *testing.T) {
	feed := newBinanceMarkPriceFeed("")
	receivedAt := time.Now().Add(-binanceMarkPriceMaxAge - time.Second)
	feed.apply([]binanceMarkPriceEvent{{
		EventTime: unixMillisValue(receivedAt.UnixMilli()), Symbol: "NXPCUSDT", MarkPrice: "0.2300",
	}}, receivedAt)
	if _, ok := feed.latest("NXPCUSDT", time.Now()); ok {
		t.Fatal("stale Binance mark must not override the HZ REST fallback")
	}
}

func TestGetPositionsCachesHZStructureBetweenReconciliations(t *testing.T) {
	var positionRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/instruments":
			writeInstruments(w)
		case "/api/v1/positions":
			positionRequests.Add(1)
			_ = json.NewEncoder(w).Encode([]position{{
				PositionID: "pos-cache", Instrument: "XAUUSD", Side: "LONG", Lots: "0.010",
				EntryPrice: "2400", CurrentPrice: "2401", UnrealizedPnL: "1",
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	trader := newTestTrader(t, server.URL+"/api/v1", true)
	if _, err := trader.GetPositions(); err != nil {
		t.Fatal(err)
	}
	if _, err := trader.GetPositions(); err != nil {
		t.Fatal(err)
	}
	if got := positionRequests.Load(); got != 1 {
		t.Fatalf("HZ position requests=%d, want one cached structure fetch", got)
	}
}

func TestCommodityQuantityLotConversion(t *testing.T) {
	tests := []struct {
		symbol       string
		contractSize string
		quantity     float64
		wantLots     string
	}{
		{"XAUUSD", "100", 1, "0.010"},
		{"XAGUSD", "5000", 5, "0.001"},
		{"WTIUSD", "1000", 1, "0.001"},
	}
	trader := &Trader{}
	for _, test := range tests {
		t.Run(test.symbol, func(t *testing.T) {
			spec := instrument{
				Instrument: test.symbol, LotPrecision: 3, MinLots: "0.001",
				MaxLots: "100", LotStep: "0.001", ContractSize: test.contractSize,
			}
			lots, err := trader.lotsForQuantityWithSpec(spec, test.quantity)
			if err != nil || lots != test.wantLots {
				t.Fatalf("lots=%q err=%v", lots, err)
			}
			quantity, err := quantityForLots(spec, lots)
			if err != nil || quantity != test.quantity {
				t.Fatalf("quantity=%v err=%v", quantity, err)
			}
		})
	}
	spec := instrument{
		Instrument: "XAUUSD", LotPrecision: 3, MinLots: "0.001",
		MaxLots: "100", LotStep: "0.001", ContractSize: "100",
	}
	if _, err := trader.lotsForQuantityWithSpec(spec, math.NaN()); err == nil {
		t.Fatal("NaN quantity should be rejected")
	}
}

func TestBTCQuantityFloorsToLotStepWithoutExactStepDrift(t *testing.T) {
	spec := instrument{
		Instrument: "BTCUSDT", LotPrecision: 3, MinLots: "0.001",
		MaxLots: "100", LotStep: "0.001", ContractSize: "1",
	}
	tests := []struct {
		quantity float64
		wantLots string
	}{
		{0.020004, "0.020"},
		{0.010002, "0.010"},
		{0.0020004, "0.002"},
		{0.020, "0.020"},
	}
	for _, test := range tests {
		lots, err := (&Trader{}).lotsForQuantityWithSpec(spec, test.quantity)
		if err != nil || lots != test.wantLots {
			t.Fatalf("quantity=%.12f lots=%q err=%v, want %q", test.quantity, lots, err, test.wantLots)
		}
	}
}

func TestPartialCloseConvertsQuantityAndRejectsOverClose(t *testing.T) {
	var mu sync.Mutex
	var closed []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/instruments":
			writeInstruments(w)
		case r.URL.Path == "/api/v1/positions":
			_ = json.NewEncoder(w).Encode([]position{
				{PositionID: "pos-1", Instrument: "XAUUSD", Side: "LONG", Lots: "0.020"},
			})
		case strings.HasSuffix(r.URL.Path, "/close"):
			var input map[string]string
			_ = json.NewDecoder(r.Body).Decode(&input)
			mu.Lock()
			closed = append(closed, input["lots"])
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(positionAction{ClosedPosition: position{
				PositionID: strings.Split(r.URL.Path, "/")[4], CurrentPrice: "2400",
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	trader := newTestTrader(t, server.URL+"/api/v1", true)

	trader.SetNextIntent("comkun-close-1")
	if _, err := trader.CloseLong("XAUUSD", 1.5); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	got := strings.Join(closed, ",")
	mu.Unlock()
	if got != "0.015" {
		t.Fatalf("wrong close lots: %s", got)
	}
	trader.SetNextIntent("comkun-close-2")
	if _, err := trader.CloseLong("XAUUSD", 2.1); err == nil {
		t.Fatal("expected over-close to be rejected")
	}
}

func TestReadModelsExposeCommodityQuantity(t *testing.T) {
	var instrumentCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/instruments":
			instrumentCalls.Add(1)
			writeInstruments(w)
		case r.URL.Path == "/api/v1/orders/order-1":
			_ = json.NewEncoder(w).Encode(order{
				OrderID: "order-1", Instrument: "XAGUSD", Lots: "0.002", Status: "FILLED",
			})
		case r.URL.Path == "/api/v1/orders/open":
			_ = json.NewEncoder(w).Encode([]order{{
				OrderID: "order-2", Instrument: "WTIUSD", Lots: "0.003", Status: "PENDING",
			}})
		case r.URL.Path == "/api/v1/trades":
			_ = json.NewEncoder(w).Encode(tradePage{Items: []trade{{
				TradeID: "trade-1", Instrument: "XAUUSD", Side: "LONG", Lots: "0.010",
				Price: "2400", ExecutedAt: "2026-07-25T00:00:00Z",
			}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	trader := newTestTrader(t, server.URL+"/api/v1", true)

	status, err := trader.GetOrderStatus("XAGUSD", "order-1")
	if err != nil || status["executedQty"] != 10.0 {
		t.Fatalf("wrong status: %#v err=%v", status, err)
	}
	orders, err := trader.GetOpenOrders("")
	if err != nil || len(orders) != 1 || orders[0].Quantity != 3.0 {
		t.Fatalf("wrong orders: %#v err=%v", orders, err)
	}
	trades, err := trader.GetClosedPnL(time.Unix(0, 0), 10)
	if err != nil || len(trades) != 1 || trades[0].Quantity != 1.0 {
		t.Fatalf("wrong trades: %#v err=%v", trades, err)
	}
	if instrumentCalls.Load() != 1 {
		t.Fatalf("instrument rules should be cached, calls=%d", instrumentCalls.Load())
	}
}

func TestReconcileFailureBlocksOpening(t *testing.T) {
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, `{"code":"TEMPORARY","message":"暂不可用"}`, http.StatusServiceUnavailable)
			return
		}
		switch r.URL.Path {
		case "/api/v1/account":
			_ = json.NewEncoder(w).Encode(testScopeAccount())
		case "/api/v1/positions", "/api/v1/orders/open":
			_, _ = w.Write([]byte("[]"))
		case "/api/v1/instruments":
			_ = json.NewEncoder(w).Encode([]instrument{{
				Instrument: "XAUUSD", LotPrecision: 3, MinLots: "0.001",
				MaxLots: "100", LotStep: "0.001", ContractSize: "100",
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := newClient(server.URL+"/api/v1", "key", "secret")
	if err != nil {
		t.Fatal(err)
	}
	trader := &Trader{client: client}
	if err := trader.Reconcile(); err != nil {
		t.Fatal(err)
	}
	trader.streamReady.Store(true)
	if !trader.IsReady() {
		t.Fatal("successful reconciliation should permit opening")
	}
	fail.Store(true)
	trader.reconcileOnce()
	if trader.IsReady() {
		t.Fatal("failed reconciliation should block opening")
	}
}

func TestNewTraderUsesRESTReconciliationWithoutWebSocket(t *testing.T) {
	if followerRESTReconcileInterval != 15*time.Second {
		t.Fatalf("REST reconcile interval=%s, want 15s", followerRESTReconcileInterval)
	}
	var wsRequests atomic.Int32
	var accountRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/capabilities":
			writeCapabilities(w)
		case "/api/v1/instruments":
			writeInstruments(w)
		case "/api/v1/account":
			accountRequests.Add(1)
			_ = json.NewEncoder(w).Encode(testScopeAccount())
		case "/api/v1/positions", "/api/v1/orders/open":
			_, _ = w.Write([]byte("[]"))
		case "/api/v1/ws":
			wsRequests.Add(1)
			http.Error(w, "websocket forbidden", http.StatusServiceUnavailable)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	trader, err := NewTrader(server.URL+"/api/v1", "key", "secret", true)
	if err != nil {
		t.Fatal(err)
	}
	defer trader.Close()
	deadline := time.Now().Add(time.Second)
	for !trader.IsReady() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !trader.IsReady() {
		t.Fatal("initial REST reconciliation did not make trader ready")
	}
	if got := wsRequests.Load(); got != 0 {
		t.Fatalf("HZ follower opened %d websocket request(s)", got)
	}
	if got := accountRequests.Load(); got != 1 {
		t.Fatalf("account requests=%d, want one shared account request for capability check and REST reconciliation", got)
	}
}

func TestRESTReconcilesBeforeAllowingOpening(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/capabilities":
			writeCapabilities(w)
		case "/api/v1/account":
			_ = json.NewEncoder(w).Encode(testScopeAccount())
		case "/api/v1/positions", "/api/v1/orders/open":
			_, _ = w.Write([]byte("[]"))
		case "/api/v1/instruments":
			_ = json.NewEncoder(w).Encode([]instrument{{
				Instrument: "XAUUSD", LotPrecision: 3, MinLots: "0.001",
				MaxLots: "100", LotStep: "0.001", ContractSize: "100",
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	trader, err := NewTrader(server.URL+"/api/v1", "key", "secret", true)
	if err != nil {
		t.Fatal(err)
	}
	defer trader.Close()
	deadline := time.Now().Add(time.Second)
	for !trader.IsReady() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !trader.IsReady() {
		t.Fatal("trader never became ready after REST reconciliation")
	}
}

func writeOrder(w http.ResponseWriter, clientID, status string) {
	_ = json.NewEncoder(w).Encode(order{
		OrderID: "order-1", ClientOrderID: clientID, Instrument: "XAUUSD",
		Side: "LONG", OrderType: "MARKET", MarginMode: "CROSS", Leverage: 500,
		Lots: "0.010", Status: status, CreatedAt: "2026-07-25T00:00:00Z",
		UpdatedAt: "2026-07-25T00:00:00Z",
	})
}

func writeInstruments(w http.ResponseWriter) {
	contracts := map[string]int{"XAUUSD": 100, "XAGUSD": 5000, "WTIUSD": 1000}
	values := make([]instrument, 0, len(contracts))
	for symbol, contractSize := range contracts {
		values = append(values, instrument{
			Instrument: symbol, LotPrecision: 3, MinLots: "0.001",
			MaxLots: "100", LotStep: "0.001", ContractSize: strconv.Itoa(contractSize),
		})
	}
	_ = json.NewEncoder(w).Encode(values)
}

func newTestTrader(t *testing.T, apiURL string, crossMargin bool) *Trader {
	t.Helper()
	client, err := newClient(apiURL, "key", "secret")
	if err != nil {
		t.Fatal(err)
	}
	trader := &Trader{client: client}
	trader.scopeVerified.Store(true)
	trader.crossMargin.Store(crossMargin)
	return trader
}

func writeCapabilities(w http.ResponseWriter) {
	_ = json.NewEncoder(w).Encode(Capabilities{
		APIVersion: "v1", Permissions: permissionSet{Read: true, Trade: true}, AccountScope: "AI",
		WalletID: "wallet", PositionBookID: "book", OrderTypes: []string{"MARKET"},
	})
}

func testScopeAccount() account {
	return account{
		AccountID: "account", AccountScope: "AI", WalletID: "wallet", PositionBookID: "book",
		Currency: "USD", Balance: "0", Equity: "0", AvailableMargin: "0", UsedMargin: "0", MarginRatio: "0", UnrealizedPnL: "0", Tradable: true,
	}
}
