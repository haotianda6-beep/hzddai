package hz

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestExecuteWithIntentClearsUnusedIntent(t *testing.T) {
	trader := &Trader{}
	_, err := trader.ExecuteWithIntent("comkun-fixed", func() (map[string]interface{}, error) {
		return nil, errors.New("preflight failed")
	})
	if err == nil || trader.popNextIntent() != "" {
		t.Fatalf("intent must be cleared after a failed preflight: %v", err)
	}
}

func TestVerifyCapabilitiesRequiresAIScopeAndSafePermissions(t *testing.T) {
	tests := []struct {
		name        string
		permissions map[string]bool
		scope       string
		wallet      string
		book        string
		orderTypes  []string
		ok          bool
	}{
		{"safe", map[string]bool{"read": true, "trade": true}, "AI", "w", "p", []string{"MARKET"}, true},
		{"withdraw", map[string]bool{"read": true, "trade": true, "withdraw": true}, "AI", "w", "p", []string{"MARKET"}, false},
		{"manual scope", map[string]bool{"read": true, "trade": true}, "MANUAL", "w", "p", []string{"MARKET"}, false},
		{"missing book", map[string]bool{"read": true, "trade": true}, "AI", "w", "", []string{"MARKET"}, false},
		{"limit enabled", map[string]bool{"read": true, "trade": true}, "AI", "w", "p", []string{"MARKET", "LIMIT"}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/capabilities":
					_ = json.NewEncoder(w).Encode(map[string]any{
						"apiVersion": "v1", "permissions": test.permissions,
						"accountScope": test.scope, "walletId": test.wallet,
						"positionBookId": test.book, "orderTypes": test.orderTypes,
					})
				case "/account":
					_ = json.NewEncoder(w).Encode(map[string]any{
						"accountId": "a", "accountScope": test.scope,
						"walletId": test.wallet, "positionBookId": test.book,
					})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			_, err := VerifyCapabilities(server.URL, "key", "secret")
			if (err == nil) != test.ok {
				t.Fatalf("ok=%v err=%v", test.ok, err)
			}
		})
	}
}

func TestDynamicInstrumentPrecision(t *testing.T) {
	trader := &Trader{}
	spec := instrument{Instrument: "BTC-PERP", LotPrecision: 4, MinLots: "0.0025", MaxLots: "10", LotStep: "0.0025", ContractSize: "0.01"}
	lots, err := trader.lotsForQuantityWithSpec(spec, 0.000076)
	if err != nil || lots != "0.0075" {
		t.Fatalf("lots=%q err=%v", lots, err)
	}
	quantity, err := quantityForLots(spec, lots)
	if err != nil || quantity != 0.000075 {
		t.Fatalf("quantity=%v err=%v", quantity, err)
	}
}

func TestFixedIntentClientOrderIDSurvivesRetry(t *testing.T) {
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/instruments":
			_ = json.NewEncoder(w).Encode([]instrument{{Instrument: "BTC-PERP", LotPrecision: 3, MinLots: "0.001", MaxLots: "100", LotStep: "0.001", ContractSize: "1"}})
		case strings.HasPrefix(r.URL.Path, "/api/v1/orders/by-client-id/"):
			if posts.Load() == 0 {
				http.NotFound(w, r)
				return
			}
			writeOrder(w, strings.TrimPrefix(r.URL.Path, "/api/v1/orders/by-client-id/"), "FILLED")
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/orders":
			posts.Add(1)
			var input createOrderRequest
			_ = json.NewDecoder(r.Body).Decode(&input)
			if input.ClientOrderID != "comkun-fixed-intent" {
				t.Errorf("clientOrderId=%q", input.ClientOrderID)
			}
			writeOrder(w, input.ClientOrderID, "FILLED")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	trader := newTestTrader(t, server.URL+"/api/v1", true)
	trader.streamReady.Store(true)
	trader.reconcileHealthy.Store(true)
	trader.SetNextIntent("comkun-fixed-intent")
	if _, err := trader.OpenLong("BTC-PERP", 1, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := trader.LookupOrderByClientID("comkun-fixed-intent"); err != nil {
		t.Fatal(err)
	}
	if posts.Load() != 1 {
		t.Fatalf("posts=%d want 1", posts.Load())
	}
}

func TestCloseRejectsAmbiguousMultiplePositions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/positions" {
			_ = json.NewEncoder(w).Encode([]position{
				{PositionID: "p1", Instrument: "BTC-PERP", Side: "LONG", Lots: "1"},
				{PositionID: "p2", Instrument: "BTC-PERP", Side: "LONG", Lots: "1"},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	trader := newTestTrader(t, server.URL+"/api/v1", true)
	trader.scopeVerified.Store(true)
	if _, err := trader.CloseLong("BTC-PERP", 0); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous close rejection, got %v", err)
	}
}
