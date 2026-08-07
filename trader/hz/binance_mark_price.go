package hz

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"nofx/logger"
)

const (
	binanceMarkPriceStreamURL = "wss://fstream.binance.com/market/ws/!markPrice@arr@1s"
	binanceMarkPriceMaxAge    = 3 * time.Second
)

type binanceMarkPriceEvent struct {
	EventTime unixMillisValue `json:"E"`
	Symbol    string          `json:"s"`
	MarkPrice string          `json:"p"`
}

type unixMillisValue int64

func (event *binanceMarkPriceEvent) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	eventTime, err := parseUnixMillis(raw["E"])
	if err != nil {
		eventTime, err = parseUnixMillis(raw["T"])
	}
	if err != nil {
		return err
	}
	var symbol, markPrice string
	if err := json.Unmarshal(raw["s"], &symbol); err != nil {
		return err
	}
	if err := json.Unmarshal(raw["p"], &markPrice); err != nil {
		return err
	}
	event.EventTime = eventTime
	event.Symbol = symbol
	event.MarkPrice = markPrice
	return nil
}

func parseUnixMillis(data json.RawMessage) (unixMillisValue, error) {
	milliseconds, err := strconv.ParseInt(strings.Trim(string(data), `"`), 10, 64)
	return unixMillisValue(milliseconds), err
}

type binanceMarkPriceSnapshot struct {
	price      float64
	eventTime  time.Time
	receivedAt time.Time
}

type binanceMarkPriceFeed struct {
	url       string
	mu        sync.RWMutex
	prices    map[string]binanceMarkPriceSnapshot
	startOnce sync.Once
}

var sharedBinanceMarkPrices = newBinanceMarkPriceFeed(binanceMarkPriceStreamURL)

func newBinanceMarkPriceFeed(url string) *binanceMarkPriceFeed {
	return &binanceMarkPriceFeed{url: url, prices: make(map[string]binanceMarkPriceSnapshot)}
}

func (f *binanceMarkPriceFeed) start() {
	if f == nil || f.url == "" {
		return
	}
	f.startOnce.Do(func() { go f.run(context.Background()) })
}

func (f *binanceMarkPriceFeed) run(ctx context.Context) {
	delay := time.Second
	for ctx.Err() == nil {
		connected, err := f.session(ctx)
		if ctx.Err() != nil {
			return
		}
		if connected {
			delay = time.Second
		} else if delay < 30*time.Second {
			delay *= 2
		}
		logger.Warnf("[HZ] Binance mark-price stream lost; retrying in %s: %v", delay, err)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (f *binanceMarkPriceFeed) session(ctx context.Context) (bool, error) {
	dialer := websocket.Dialer{HandshakeTimeout: 8 * time.Second}
	socket, response, err := dialer.DialContext(ctx, f.url, nil)
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

	logger.Infof("[HZ] Binance USD-M mark-price stream connected")
	for {
		_ = socket.SetReadDeadline(time.Now().Add(5 * time.Second))
		var rawEvents []json.RawMessage
		if err := socket.ReadJSON(&rawEvents); err != nil {
			return true, err
		}
		events := make([]binanceMarkPriceEvent, 0, len(rawEvents))
		invalid := 0
		for _, raw := range rawEvents {
			var event binanceMarkPriceEvent
			if err := json.Unmarshal(raw, &event); err != nil {
				invalid++
				continue
			}
			events = append(events, event)
		}
		if invalid > 0 {
			logger.Warnf("[HZ] Binance mark-price frame skipped %d invalid event(s)", invalid)
		}
		f.apply(events, time.Now())
	}
}

func (f *binanceMarkPriceFeed) apply(events []binanceMarkPriceEvent, receivedAt time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, event := range events {
		price, err := strconv.ParseFloat(event.MarkPrice, 64)
		if err != nil || price <= 0 || event.Symbol == "" {
			continue
		}
		eventTime := time.UnixMilli(int64(event.EventTime))
		f.prices[strings.ToUpper(event.Symbol)] = binanceMarkPriceSnapshot{
			price: price, eventTime: eventTime, receivedAt: receivedAt,
		}
	}
}

func (f *binanceMarkPriceFeed) latest(symbol string, now time.Time) (binanceMarkPriceSnapshot, bool) {
	if f == nil {
		return binanceMarkPriceSnapshot{}, false
	}
	f.mu.RLock()
	snapshot, ok := f.prices[strings.ToUpper(symbol)]
	f.mu.RUnlock()
	if !ok || now.Sub(snapshot.receivedAt) > binanceMarkPriceMaxAge {
		return binanceMarkPriceSnapshot{}, false
	}
	return snapshot, true
}
