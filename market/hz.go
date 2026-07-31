package market

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var hzMarket struct {
	sync.RWMutex
	baseURL     *url.URL
	client      *http.Client
	instruments map[string]struct{}
}

type hzCandle struct {
	Open      string `json:"open"`
	High      string `json:"high"`
	Low       string `json:"low"`
	Close     string `json:"close"`
	Volume    string `json:"volume"`
	OpenTime  string `json:"openTime"`
	CloseTime string `json:"closeTime"`
}

func ConfigureHZ(apiURL string) error {
	hzMarket.Lock()
	defer hzMarket.Unlock()
	if strings.TrimSpace(apiURL) == "" {
		hzMarket.baseURL = nil
		hzMarket.client = nil
		return nil
	}
	base, err := url.Parse(strings.TrimRight(strings.TrimSpace(apiURL), "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return fmt.Errorf("invalid HZ market API URL")
	}
	hzMarket.baseURL = base
	hzMarket.client = &http.Client{Timeout: 10 * time.Second}
	return nil
}

func IsHZSymbol(symbol string) bool {
	hzMarket.RLock()
	defer hzMarket.RUnlock()
	_, ok := hzMarket.instruments[strings.ToUpper(strings.TrimSpace(symbol))]
	return ok
}

// SetHZInstruments replaces the HZ market routing allow-list with the
// server-authoritative instrument set returned by B.
func SetHZInstruments(instruments []string) {
	next := make(map[string]struct{}, len(instruments))
	for _, instrument := range instruments {
		if name := strings.ToUpper(strings.TrimSpace(instrument)); name != "" {
			next[name] = struct{}{}
		}
	}
	hzMarket.Lock()
	hzMarket.instruments = next
	hzMarket.Unlock()
}

func hzConfigured(symbol string) bool {
	hzMarket.RLock()
	defer hzMarket.RUnlock()
	return IsHZSymbol(symbol) && hzMarket.baseURL != nil
}

func getWithHZTimeframes(symbol string, timeframes []string, primary string, count int) (*Data, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if len(timeframes) == 0 {
		return nil, fmt.Errorf("at least one timeframe is required")
	}
	if primary == "" && len(timeframes) > 0 {
		primary = timeframes[0]
	}
	hasPrimary := false
	for _, timeframe := range timeframes {
		hasPrimary = hasPrimary || timeframe == primary
	}
	if !hasPrimary {
		timeframes = append([]string{primary}, timeframes...)
	}

	series := make(map[string]*TimeframeSeriesData, len(timeframes))
	var primaryKlines []Kline
	for _, timeframe := range timeframes {
		klines, err := getKlinesFromHZ(symbol, timeframe, 200)
		if err != nil {
			return nil, fmt.Errorf("HZ %s %s K线读取失败: %w", symbol, timeframe, err)
		}
		series[timeframe] = calculateTimeframeSeries(klines, timeframe, count)
		if timeframe == primary {
			primaryKlines = klines
		}
	}
	if len(primaryKlines) == 0 {
		return nil, fmt.Errorf("HZ 主周期 %s K线为空", primary)
	}
	if isStaleData(primaryKlines, symbol) {
		return nil, fmt.Errorf("%s 行情已过期", symbol)
	}
	return &Data{
		Symbol: symbol, CurrentPrice: primaryKlines[len(primaryKlines)-1].Close,
		PriceChange1h: calculatePriceChangeByBars(primaryKlines, primary, 60),
		PriceChange4h: calculatePriceChangeByBars(primaryKlines, primary, 240),
		CurrentEMA20:  calculateEMA(primaryKlines, 20),
		CurrentMACD:   calculateMACD(primaryKlines), CurrentRSI7: calculateRSI(primaryKlines, 7),
		OpenInterest: &OIData{}, TimeframeData: series,
	}, nil
}

func getKlinesFromHZ(symbol, timeframe string, limit int) ([]Kline, error) {
	hzMarket.RLock()
	if hzMarket.baseURL == nil || hzMarket.client == nil {
		hzMarket.RUnlock()
		return nil, fmt.Errorf("HZ market API is not configured")
	}
	endpoint := *hzMarket.baseURL
	client := hzMarket.client
	hzMarket.RUnlock()

	apiTimeframe := timeframe
	if apiTimeframe == "3m" {
		apiTimeframe = "5m"
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/candles"
	query := endpoint.Query()
	query.Set("instrument", strings.ToUpper(symbol))
	query.Set("interval", apiTimeframe)
	query.Set("limit", strconv.Itoa(limit))
	endpoint.RawQuery = query.Encode()

	response, err := client.Get(endpoint.String())
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var wire []hzCandle
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&wire); err != nil {
		return nil, err
	}
	result := make([]Kline, 0, len(wire))
	for _, item := range wire {
		openTime, openErr := time.Parse(time.RFC3339Nano, item.OpenTime)
		closeTime, closeErr := time.Parse(time.RFC3339Nano, item.CloseTime)
		open, openValueErr := strconv.ParseFloat(item.Open, 64)
		high, highErr := strconv.ParseFloat(item.High, 64)
		low, lowErr := strconv.ParseFloat(item.Low, 64)
		closePrice, closeValueErr := strconv.ParseFloat(item.Close, 64)
		volume, volumeErr := strconv.ParseFloat(item.Volume, 64)
		if openErr != nil || closeErr != nil || openValueErr != nil || highErr != nil ||
			lowErr != nil || closeValueErr != nil || volumeErr != nil {
			return nil, fmt.Errorf("HZ returned an invalid candle")
		}
		result = append(result, Kline{
			OpenTime: openTime.UnixMilli(), CloseTime: closeTime.UnixMilli(),
			Open: open, High: high, Low: low, Close: closePrice, Volume: volume,
		})
	}
	return result, nil
}
