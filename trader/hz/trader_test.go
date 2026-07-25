package hz

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestOpenLongUsesLotsAndReconcilesTimeout(t *testing.T) {
	var clientOrderID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/orders":
			var input createOrderRequest
			_ = json.NewDecoder(r.Body).Decode(&input)
			clientOrderID = input.ClientOrderID
			if input.Side != "LONG" || input.SizeMode != "LOTS" || input.Size != "0.01" || input.Leverage != 500 {
				t.Errorf("wrong order payload: %#v", input)
			}
			time.Sleep(40 * time.Millisecond)
			writeOrder(w, input.ClientOrderID, "FILLED")
		case strings.HasPrefix(r.URL.Path, "/api/v1/orders/by-client-id/"):
			writeOrder(w, clientOrderID, "FILLED")
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
	trader.client.http.Timeout = 10 * time.Millisecond
	trader.streamReady.Store(true)
	result, err := trader.OpenLong("XAUUSD", 0.01, 500)
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
	trader, err := NewTrader(server.URL+"/api/v1", "key", "secret", true)
	if err != nil {
		t.Fatal(err)
	}
	defer trader.Close()
	if _, err := trader.OpenShort("XAUUSD", 0.01, 500); err == nil {
		t.Fatal("expected disconnected stream to block opening")
	}
}

func TestGetPositionsMapsHZSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]position{{
			PositionID: "pos-1", Instrument: "XAUUSD", Side: "LONG", MarginMode: "CROSS",
			Leverage: 500, Lots: "0.010", EntryPrice: "2400.10", CurrentPrice: "2401.20",
			UnrealizedPnL: "1.10", OpenedAt: "2026-07-25T00:00:00Z",
		}})
	}))
	defer server.Close()

	trader, _ := NewTrader(server.URL+"/api/v1", "key", "secret", true)
	defer trader.Close()
	positions, err := trader.GetPositions()
	if err != nil {
		t.Fatal(err)
	}
	if len(positions) != 1 || positions[0]["side"] != "long" || positions[0]["positionAmt"] != 0.01 {
		t.Fatalf("wrong positions: %#v", positions)
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
