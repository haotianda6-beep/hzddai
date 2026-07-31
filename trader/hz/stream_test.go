package hz

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestCloseDisconnectsActiveStream(t *testing.T) {
	connected := make(chan struct{})
	disconnected := make(chan struct{})
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/capabilities":
			writeCapabilities(w)
		case "/api/v1/instruments":
			writeInstruments(w)
		case "/api/v1/ws":
			socket, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			close(connected)
			_, _, _ = socket.ReadMessage()
			_ = socket.Close()
			close(disconnected)
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
	select {
	case <-connected:
	case <-time.After(time.Second):
		t.Fatal("stream did not connect")
	}
	trader.Close()
	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("Close did not disconnect the active stream")
	}
}

func TestMarketPauseBlocksOpeningButAllowsClose(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/capabilities":
			writeCapabilities(w)
		case r.URL.Path == "/api/v1/instruments":
			writeInstruments(w)
		case r.URL.Path == "/api/v1/ws":
			socket, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			_ = socket.WriteJSON(streamEnvelope{
				Type: "system.status", Sequence: 1,
				Data: json.RawMessage(`{"marketStatus":"PAUSED","apiOpeningStopped":false}`),
			})
			_, _, _ = socket.ReadMessage()
			_ = socket.Close()
		case r.URL.Path == "/api/v1/account":
			_ = json.NewEncoder(w).Encode(account{Currency: "USD", Tradable: true})
		case r.URL.Path == "/api/v1/positions":
			_ = json.NewEncoder(w).Encode([]position{{
				PositionID: "pos-1", Instrument: "XAUUSD", Side: "LONG", Lots: "0.010",
			}})
		case r.URL.Path == "/api/v1/orders/open":
			_, _ = w.Write([]byte("[]"))
		case r.URL.Path == "/api/v1/positions/pos-1/close":
			_ = json.NewEncoder(w).Encode(positionAction{ClosedPosition: position{
				PositionID: "pos-1", Instrument: "XAUUSD", CurrentPrice: "2400",
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
	for (!trader.streamReady.Load() || !trader.streamOpeningStopped.Load()) &&
		time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if trader.IsReady() {
		t.Fatal("paused market should block opening")
	}
	if _, err := trader.OpenLong("XAUUSD", 1, 500); err == nil {
		t.Fatal("opening succeeded while market was paused")
	}
	trader.SetNextIntent("comkun-paused-close")
	if _, err := trader.CloseLong("XAUUSD", 0); err != nil {
		t.Fatalf("closing should remain available while market is paused: %v", err)
	}
}
