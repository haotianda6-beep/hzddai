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

func TestBinanceAggTradeUsesExactTradePriceAndTime(t *testing.T) {
	var event binanceAggTradeEvent
	payload := `{"e":"aggTrade","E":1786090146004,"s":"NXPCUSDT","p":"0.2382000","P":"9.999","T":1786090145999}`
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		t.Fatal(err)
	}
	if event.Price != "0.2382000" || int64(event.TradeTime) != 1786090145999 {
		t.Fatalf("event=%#v", event)
	}
}

func TestBinanceLastPriceKeepsLastTradeWithoutInventingBid(t *testing.T) {
	feed := newBinanceLastPriceFeed("", "")
	tradeTime := time.Now().Add(-10 * time.Minute)
	feed.apply(binanceAggTradeEvent{
		EventTime: unixMillisValue(tradeTime.UnixMilli()),
		TradeTime: unixMillisValue(tradeTime.UnixMilli()),
		Symbol:    "NXPCUSDT", Price: "0.2378",
	}, tradeTime, "binance_agg_trade_ws")

	snapshot, ok := feed.latest("NXPCUSDT")
	if !ok || snapshot.price != 0.2378 || snapshot.source != "binance_agg_trade_ws" {
		t.Fatalf("snapshot=%#v ok=%v", snapshot, ok)
	}
}

func TestCurrentBinanceLastPricesReadsExistingMemoryOnly(t *testing.T) {
	original := sharedBinanceLastPrices
	feed := newBinanceLastPriceFeed("", "")
	sharedBinanceLastPrices = feed
	defer func() { sharedBinanceLastPrices = original }()
	feed.apply(binanceAggTradeEvent{
		EventTime: 1786090146000, TradeTime: 1786090145999,
		Symbol: "NXPCUSDT", Price: "0.2382",
	}, time.Now(), "binance_agg_trade_ws")

	prices := CurrentBinanceLastPrices([]string{"NXPCUSDT", "UNKNOWN"})
	if len(prices) != 1 || prices[0].Symbol != "NXPCUSDT" || prices[0].Price != 0.2382 ||
		prices[0].Time != 1786090145999 || prices[0].Source != "binance_agg_trade_ws" {
		t.Fatalf("prices=%#v", prices)
	}
}

func TestBinanceLastPriceRESTSeedsButDoesNotOverwriteNewerTrade(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"symbol": "NXPCUSDT", "price": "0.2376", "time": int64(1786090145000),
		})
	}))
	defer server.Close()

	feed := newBinanceLastPriceFeed("", server.URL+"?symbol=%s")
	if err := feed.refreshREST(context.Background(), "NXPCUSDT"); err != nil {
		t.Fatal(err)
	}
	feed.apply(binanceAggTradeEvent{
		EventTime: 1786090146000, TradeTime: 1786090146000,
		Symbol: "NXPCUSDT", Price: "0.2378",
	}, time.Now(), "binance_agg_trade_ws")
	if err := feed.refreshREST(context.Background(), "NXPCUSDT"); err != nil {
		t.Fatal(err)
	}

	snapshot, ok := feed.latest("NXPCUSDT")
	if !ok || snapshot.price != 0.2378 || snapshot.source != "binance_agg_trade_ws" {
		t.Fatalf("snapshot=%#v ok=%v", snapshot, ok)
	}
}

func TestBinanceAggTradeFeedReconnects(t *testing.T) {
	var connections atomic.Int32
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		socket, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connection := connections.Add(1)
		now := time.Now().UnixMilli()
		_ = socket.WriteJSON(map[string]interface{}{
			"E": now, "T": now, "s": "NXPCUSDT", "p": "0.23" + strconv.Itoa(int(connection)),
		})
		_ = socket.Close()
	}))
	defer server.Close()

	feed := newBinanceLastPriceFeed("ws"+strings.TrimPrefix(server.URL, "http")+"/%s", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go feed.run(ctx, "NXPCUSDT")

	deadline := time.Now().Add(4 * time.Second)
	for connections.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := connections.Load(); got < 2 {
		t.Fatalf("connections=%d, want reconnect", got)
	}
	if snapshot, ok := feed.latest("NXPCUSDT"); !ok || snapshot.price != 0.232 {
		t.Fatalf("snapshot=%#v ok=%v, want reconnected price 0.232", snapshot, ok)
	}
}
