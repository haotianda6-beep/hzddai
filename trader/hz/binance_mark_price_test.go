package hz

import (
	"context"
	"encoding/json"
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

func TestBinanceMarkPriceEventAcceptsStringTimestamp(t *testing.T) {
	var events []binanceMarkPriceEvent
	if err := json.Unmarshal([]byte(`[{"E":"1786089000123","s":"NXPCUSDT","p":"0.2384"}]`), &events); err != nil {
		t.Fatalf("decode Binance event with string timestamp: %v", err)
	}
	feed := newBinanceMarkPriceFeed("")
	feed.apply(events, time.UnixMilli(1786089000200))
	if snapshot, ok := feed.latest("NXPCUSDT", time.UnixMilli(1786089000200)); !ok || snapshot.eventTime.UnixMilli() != 1786089000123 {
		t.Fatalf("snapshot=%#v ok=%v", snapshot, ok)
	}
}

func TestBinanceMarkPriceNamedEventUsesTransactionTime(t *testing.T) {
	var events []binanceMarkPriceEvent
	payload := `[{"E":"markPriceUpdate","T":1786089000456,"s":"NXPCUSDT","p":"0.2385"}]`
	if err := json.Unmarshal([]byte(payload), &events); err != nil {
		t.Fatalf("decode Binance named event: %v", err)
	}
	if got := int64(events[0].EventTime); got != 1786089000456 {
		t.Fatalf("event time=%d, want transaction time", got)
	}
}

func TestBinanceMarkPriceEventUsesExactCaseSensitiveFields(t *testing.T) {
	var events []binanceMarkPriceEvent
	payload := `[{"e":"markPriceUpdate","E":1786088785000,"s":"NXPCUSDT","p":"0.23790000","P":"0.23888838","T":1786089600000}]`
	if err := json.Unmarshal([]byte(payload), &events); err != nil {
		t.Fatal(err)
	}
	if got := int64(events[0].EventTime); got != 1786088785000 {
		t.Fatalf("event time=%d, want exact uppercase E", got)
	}
	if got := events[0].MarkPrice; got != "0.23790000" {
		t.Fatalf("mark price=%s, want exact lowercase p", got)
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
			EventTime: unixMillisValue(time.Now().UnixMilli()), Symbol: "NXPCUSDT",
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
