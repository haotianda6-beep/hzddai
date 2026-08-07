package hz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"nofx/logger"
)

const (
	binanceAggTradeStreamURL = "wss://fstream.binance.com/market/ws/%s@aggTrade"
	binanceLastPriceRESTURL  = "https://fapi.binance.com/fapi/v1/ticker/price?symbol=%s"
)

type binanceAggTradeEvent struct {
	EventTime unixMillisValue
	TradeTime unixMillisValue
	Symbol    string
	Price     string
}

func (event *binanceAggTradeEvent) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	tradeTime, err := parseUnixMillis(raw["T"])
	if err != nil {
		return err
	}
	eventTime, err := parseUnixMillis(raw["E"])
	if err != nil {
		eventTime = tradeTime
	}
	if err := json.Unmarshal(raw["s"], &event.Symbol); err != nil {
		return err
	}
	if err := json.Unmarshal(raw["p"], &event.Price); err != nil {
		return err
	}
	event.EventTime = eventTime
	event.TradeTime = tradeTime
	return nil
}

type binanceLastPriceSnapshot struct {
	price      float64
	eventTime  time.Time
	receivedAt time.Time
	source     string
}

type binanceLastPriceFeed struct {
	wsURLTemplate   string
	restURLTemplate string
	httpClient      *http.Client
	mu              sync.RWMutex
	prices          map[string]binanceLastPriceSnapshot
	watched         map[string]struct{}
}

var sharedBinanceLastPrices = newBinanceLastPriceFeed(binanceAggTradeStreamURL, binanceLastPriceRESTURL)

func newBinanceLastPriceFeed(wsURLTemplate, restURLTemplate string) *binanceLastPriceFeed {
	return &binanceLastPriceFeed{
		wsURLTemplate: wsURLTemplate, restURLTemplate: restURLTemplate,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		prices:     make(map[string]binanceLastPriceSnapshot), watched: make(map[string]struct{}),
	}
}

func (f *binanceLastPriceFeed) watch(symbol string) {
	if f == nil || (f.wsURLTemplate == "" && f.restURLTemplate == "") {
		return
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return
	}
	f.mu.Lock()
	if _, ok := f.watched[symbol]; ok {
		f.mu.Unlock()
		return
	}
	f.watched[symbol] = struct{}{}
	f.mu.Unlock()
	go f.run(context.Background(), symbol)
}

func (f *binanceLastPriceFeed) run(ctx context.Context, symbol string) {
	delay := time.Second
	for ctx.Err() == nil {
		if err := f.refreshREST(ctx, symbol); err != nil {
			logger.Warnf("[HZ] Binance last-price REST fallback failed for %s: %v", symbol, err)
		}
		if f.wsURLTemplate == "" {
			return
		}
		connected, err := f.session(ctx, symbol)
		if ctx.Err() != nil {
			return
		}
		if connected {
			delay = time.Second
		} else if delay < 30*time.Second {
			delay *= 2
		}
		logger.Warnf("[HZ] Binance agg-trade stream lost for %s; retrying in %s: %v", symbol, delay, err)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (f *binanceLastPriceFeed) session(ctx context.Context, symbol string) (bool, error) {
	streamURL := fmt.Sprintf(f.wsURLTemplate, strings.ToLower(url.QueryEscape(symbol)))
	dialer := websocket.Dialer{HandshakeTimeout: 8 * time.Second}
	socket, response, err := dialer.DialContext(ctx, streamURL, nil)
	if err != nil {
		if response != nil {
			_ = response.Body.Close()
		}
		return false, err
	}
	defer socket.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = socket.Close()
		case <-done:
		}
	}()

	logger.Infof("[HZ] Binance USD-M agg-trade stream connected for %s", symbol)
	for {
		_ = socket.SetReadDeadline(time.Now().Add(4 * time.Minute))
		var event binanceAggTradeEvent
		if err := socket.ReadJSON(&event); err != nil {
			return true, err
		}
		f.apply(event, time.Now(), "binance_agg_trade_ws")
	}
}

func (f *binanceLastPriceFeed) refreshREST(ctx context.Context, symbol string) error {
	if f.restURLTemplate == "" {
		return nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf(f.restURLTemplate, url.QueryEscape(symbol)), nil)
	if err != nil {
		return err
	}
	response, err := f.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", response.StatusCode)
	}
	var value struct {
		Symbol string `json:"symbol"`
		Price  string `json:"price"`
		Time   int64  `json:"time"`
	}
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		return err
	}
	f.apply(binanceAggTradeEvent{
		EventTime: unixMillisValue(value.Time), TradeTime: unixMillisValue(value.Time),
		Symbol: value.Symbol, Price: value.Price,
	}, time.Now(), "binance_last_price_rest")
	return nil
}

func (f *binanceLastPriceFeed) apply(event binanceAggTradeEvent, receivedAt time.Time, source string) {
	price, err := strconv.ParseFloat(event.Price, 64)
	if f == nil || err != nil || price <= 0 || event.Symbol == "" {
		return
	}
	eventMillis := event.TradeTime
	if eventMillis == 0 {
		eventMillis = event.EventTime
	}
	eventTime := time.UnixMilli(int64(eventMillis))
	symbol := strings.ToUpper(event.Symbol)
	f.mu.Lock()
	current, ok := f.prices[symbol]
	if !ok || eventTime.After(current.eventTime) ||
		(eventTime.Equal(current.eventTime) && source == "binance_agg_trade_ws") {
		f.prices[symbol] = binanceLastPriceSnapshot{
			price: price, eventTime: eventTime, receivedAt: receivedAt, source: source,
		}
	}
	f.mu.Unlock()
}

func (f *binanceLastPriceFeed) latest(symbol string) (binanceLastPriceSnapshot, bool) {
	if f == nil {
		return binanceLastPriceSnapshot{}, false
	}
	f.mu.RLock()
	snapshot, ok := f.prices[strings.ToUpper(symbol)]
	f.mu.RUnlock()
	return snapshot, ok
}
