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

	"github.com/gorilla/websocket"
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
	trader.client.http.Timeout = 10 * time.Millisecond
	trader.streamReady.Store(true)
	trader.reconcileHealthy.Store(true)
	result, err := trader.OpenLong("XAUUSD", 1, 500)
	if err != nil {
		t.Fatal(err)
	}
	if result["status"] != "FILLED" || result["orderId"] == "" {
		t.Fatalf("wrong result: %#v", result)
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

func TestPartialCloseConvertsQuantityAndRejectsOverClose(t *testing.T) {
	var mu sync.Mutex
	var closed []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/instruments":
			writeInstruments(w)
		case r.URL.Path == "/api/v1/positions":
			_ = json.NewEncoder(w).Encode([]position{
				{PositionID: "pos-1", Instrument: "XAUUSD", Side: "LONG", Lots: "0.010"},
				{PositionID: "pos-2", Instrument: "XAUUSD", Side: "LONG", Lots: "0.020"},
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

	if _, err := trader.CloseLong("XAUUSD", 1.5); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	got := strings.Join(closed, ",")
	mu.Unlock()
	if got != "0.010,0.005" {
		t.Fatalf("wrong close lots: %s", got)
	}
	if _, err := trader.CloseLong("XAUUSD", 3.1); err == nil {
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
			_ = json.NewEncoder(w).Encode(account{Currency: "USD", Tradable: true})
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

func TestStreamReconcilesBeforeAllowingOpening(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/ws":
			socket, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer socket.Close()
			_ = socket.WriteJSON(streamEnvelope{
				Type: "system.status", Sequence: 1,
				Data: json.RawMessage(`{"marketStatus":"open","apiOpeningStopped":false}`),
			})
			time.Sleep(100 * time.Millisecond)
		case "/api/v1/account":
			_ = json.NewEncoder(w).Encode(account{Currency: "USD", Tradable: true})
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
		t.Fatal("stream never became ready after REST reconciliation")
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
	trader.crossMargin.Store(crossMargin)
	return trader
}
