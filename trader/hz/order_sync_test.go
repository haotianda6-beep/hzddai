package hz

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"nofx/store"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestHZTradeOrderSideUsesIntentAction(t *testing.T) {
	tests := []struct {
		action, positionSide, want string
	}{
		{"open", "long", "BUY"},
		{"increase", "long", "BUY"},
		{"reduce", "long", "SELL"},
		{"close", "long", "SELL"},
		{"open", "short", "SELL"},
		{"close", "short", "BUY"},
	}
	for _, test := range tests {
		got, err := hzTradeOrderSide(test.action, test.positionSide)
		if err != nil {
			t.Fatalf("%s %s: %v", test.action, test.positionSide, err)
		}
		if got != test.want {
			t.Fatalf("%s %s side=%s, want %s", test.action, test.positionSide, got, test.want)
		}
	}
}

func TestHZIntentByRemoteClientOrderIDRejectsAmbiguousSuffix(t *testing.T) {
	intents := map[string]store.MirrorExecutionIntent{
		"close-1":       {IntentKey: "short"},
		"scope:close-1": {IntentKey: "long"},
	}
	if _, ok := hzIntentByRemoteClientOrderID(intents, "api:key:scope:close-1"); ok {
		t.Fatal("ambiguous remote clientOrderId must not select an intent")
	}
}

func TestHZOpeningIntentDoesNotFabricateMissingOpen(t *testing.T) {
	closeIntent := store.MirrorExecutionIntent{
		TraderID: "trader-1", Instrument: "XAUUSD", PositionSide: "long", Action: "close",
		CreatedAt: time.Unix(2, 0),
	}
	if _, _, ok := hzOpeningIntent([]store.MirrorExecutionIntent{closeIntent}, closeIntent); ok {
		t.Fatal("close without a confirmed opening intent must not synthesize a position")
	}
}

func TestSyncOrdersFromHZProjectsCloseHistoryIdempotently(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:hz-order-sync?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&store.MirrorExecutionIntent{}, &store.TraderOrder{}, &store.TraderFill{}, &store.TraderPosition{},
	); err != nil {
		t.Fatal(err)
	}
	st, err := store.NewFromGorm(db)
	if err != nil {
		t.Fatal(err)
	}

	for _, input := range []store.MirrorExecutionIntentInput{
		{
			IntentKey: "intent-open", MasterEventID: "event-open", BroadcastID: 1,
			UserID: "user-1", TraderID: "trader-1", ExchangeID: "exchange-1",
			Instrument: "XAUUSD", PositionSide: "long", Action: "open",
			ClientOrderID: "client-open", RequestSHA256: "open-hash",
		},
		{
			IntentKey: "intent-close", MasterEventID: "event-close", BroadcastID: 2,
			UserID: "user-1", TraderID: "trader-1", ExchangeID: "exchange-1",
			Instrument: "XAUUSD", PositionSide: "long", Action: "close",
			RemotePositionID: "position-1", ClientOrderID: "client-close", RequestSHA256: "close-hash",
		},
	} {
		intent, ensureErr := st.MirrorExecutionIntent().Ensure(input)
		if ensureErr != nil {
			t.Fatal(ensureErr)
		}
		exchangeOrderID := "order-open"
		if input.Action == "close" {
			exchangeOrderID = "position-1" // legacy close response returned the position id
		}
		if err := st.MirrorExecutionIntent().MarkConfirmed(intent.IntentKey, exchangeOrderID); err != nil {
			t.Fatal(err)
		}
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/trades":
			_ = json.NewEncoder(w).Encode(tradePage{Items: []trade{
				{
					TradeID: "trade-close", OrderID: "order-close", Instrument: "XAUUSD", Side: "LONG",
					Lots: "0.010", Price: "2410", Fee: "0.10", RealizedPnL: "10",
					ExecutedAt: "2026-08-07T03:42:00Z",
				},
			}})
		case "/api/v1/orders/order-close":
			_ = json.NewEncoder(w).Encode(order{
				OrderID: "order-close", ClientOrderID: "api:api-key-1:client-close", Instrument: "XAUUSD",
				Side: "LONG", OrderType: "MARKET", Leverage: 1000, Lots: "0.010", Status: "FILLED",
			})
		default:
			http.NotFound(w, r)
		}
	})
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder.Result(), nil
	})}
	baseURL, _ := url.Parse("https://hz.test/api/v1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hzTrader := &Trader{
		client: &client{baseURL: baseURL, http: httpClient, now: time.Now, nonce: randomToken}, ctx: ctx,
		instrumentCache: map[string]instrument{
			"XAUUSD": {Instrument: "XAUUSD", ContractSize: "100", LotStep: "0.001", LotPrecision: 3},
		},
	}

	// StartOrderSync must recover history immediately; the explicit second pass
	// simulates the 3-second poll replaying the same remote trades.
	hzTrader.StartOrderSync("trader-1", "exchange-1", "hz", st, time.Hour)
	cancel()
	if err := hzTrader.SyncOrdersFromHZ("trader-1", "exchange-1", "hz", st); err != nil {
		t.Fatalf("duplicate sync: %v", err)
	}

	var orders, fills int64
	if err := db.Model(&store.TraderOrder{}).Count(&orders).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&store.TraderFill{}).Count(&fills).Error; err != nil {
		t.Fatal(err)
	}
	if orders != 1 || fills != 1 {
		t.Fatalf("orders=%d fills=%d, want 1/1 after duplicate sync", orders, fills)
	}
	positions, err := st.Position().GetClosedPositions("trader-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(positions) != 1 {
		t.Fatalf("closed positions=%d, want 1", len(positions))
	}
	position := positions[0]
	if position.Side != "LONG" || position.EntryPrice != 2400 || position.ExitPrice != 2410 || position.RealizedPnL != 10 {
		t.Fatalf("closed position=%+v", position)
	}
	closeIntent, err := st.MirrorExecutionIntent().Get("intent-close")
	if err != nil {
		t.Fatal(err)
	}
	if closeIntent.ExchangeOrderID != "order-close" {
		t.Fatalf("close exchange_order_id=%q, want repaired order-close", closeIntent.ExchangeOrderID)
	}
}
