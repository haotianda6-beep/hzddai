package hz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestBinanceMarkPriceStreamUsesMarketRoute(t *testing.T) {
	if !strings.Contains(binanceMarkPriceStreamURL, "fstream.binance.com/market/ws/") {
		t.Fatalf("mark-price URL must use Binance market route: %s", binanceMarkPriceStreamURL)
	}
	if !strings.HasSuffix(binanceMarkPriceStreamURL, "!markPrice@arr@1s") {
		t.Fatalf("mark-price URL must request 1s updates: %s", binanceMarkPriceStreamURL)
	}
}

func TestBinanceMarkPriceFeedReconnects(t *testing.T) {
	var connections atomic.Int32
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		socket, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connection := connections.Add(1)
		_ = socket.WriteJSON([]binanceMarkPriceEvent{{
			EventTime: time.Now().UnixMilli(), Symbol: "NXPCUSDT",
			MarkPrice: "0.23" + strconv.Itoa(int(connection)),
		}})
		_ = socket.Close()
	}))
	defer server.Close()

	feed := newBinanceMarkPriceFeed("ws" + strings.TrimPrefix(server.URL, "http"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go feed.run(ctx)

	deadline := time.Now().Add(4 * time.Second)
	for connections.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := connections.Load(); got < 2 {
		t.Fatalf("connections=%d, want reconnect", got)
	}
	if snapshot, ok := feed.latest("NXPCUSDT", time.Now()); !ok || snapshot.price != 0.232 {
		t.Fatalf("latest=%#v ok=%v, want reconnected price 0.232", snapshot, ok)
	}
}
