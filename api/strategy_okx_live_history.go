package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"nofx/logger"
)

const (
	okxMarketHistoryCacheTTL = 10 * time.Minute
	okxMarketHistoryPageSize = 200
	okxMarketHistoryMaxPages = 30
)

type okxMarketHistoryRow struct {
	Symbol        string
	Leverage      int
	MarginMode    string
	Direction     string
	Opened        string
	Closed        string
	EntryText     string
	ExitText      string
	PnL           float64
	ClosedVolText string
}

type okxMarketHistoryCacheEntry struct {
	rows       []okxMarketHistoryRow
	fetchedAt  time.Time
	refreshing bool
}

var okxMarketHistoryCache = struct {
	sync.Mutex
	entries map[string]okxMarketHistoryCacheEntry
}{
	entries: make(map[string]okxMarketHistoryCacheEntry),
}

var fetchOkxPublicPositionHistoryFn = fetchOkxPublicPositionHistory

type okxPublicPositionHistoryResponse struct {
	Code string                         `json:"code"`
	Data []okxPublicPositionHistoryItem `json:"data"`
	Msg  string                         `json:"msg"`
}

type okxPublicPositionHistoryItem struct {
	ID          string `json:"id"`
	TradeItemID string `json:"tradeItemId"`
	InstID      string `json:"instId"`
	Lever       string `json:"lever"`
	MgnMode     string `json:"mgnMode"`
	PosSide     string `json:"posSide"`
	Side        string `json:"side"`
	OpenTime    string `json:"openTime"`
	UTime       string `json:"uTime"`
	OpenAvgPx   string `json:"openAvgPx"`
	CloseAvgPx  string `json:"closeAvgPx"`
	PnL         string `json:"pnl"`
	SubPos      string `json:"subPos"`
	ContractVal string `json:"contractVal"`
	Multiplier  string `json:"multiplier"`
}

func loadOkxMarketHistoryRows(strategyID, uniqueName string, fallbackJSON []byte) []okxMarketHistoryRow {
	fallback := loadEmbeddedOkxMarketHistoryRows(fallbackJSON)
	if strings.TrimSpace(uniqueName) == "" {
		return fallback
	}

	cacheKey := strategyID + ":" + uniqueName
	okxMarketHistoryCache.Lock()
	cached, hasCached := okxMarketHistoryCache.entries[cacheKey]
	if hasCached && time.Since(cached.fetchedAt) < okxMarketHistoryCacheTTL {
		rows := cloneOkxMarketHistoryRows(cached.rows)
		okxMarketHistoryCache.Unlock()
		return rows
	}
	if hasCached && len(cached.rows) > 0 {
		rows := cloneOkxMarketHistoryRows(cached.rows)
		if !cached.refreshing {
			cached.refreshing = true
			okxMarketHistoryCache.entries[cacheKey] = cached
			go refreshOkxMarketHistoryRows(cacheKey, uniqueName)
		}
		okxMarketHistoryCache.Unlock()
		return rows
	}
	if len(fallback) == 0 {
		okxMarketHistoryCache.entries[cacheKey] = okxMarketHistoryCacheEntry{refreshing: true}
		okxMarketHistoryCache.Unlock()
		rows, err := fetchOkxPublicPositionHistoryFn(uniqueName)
		now := time.Now()
		okxMarketHistoryCache.Lock()
		if err == nil && len(rows) > 0 {
			okxMarketHistoryCache.entries[cacheKey] = okxMarketHistoryCacheEntry{
				rows:      cloneOkxMarketHistoryRows(rows),
				fetchedAt: now,
			}
			okxMarketHistoryCache.Unlock()
			return rows
		}
		okxMarketHistoryCache.entries[cacheKey] = okxMarketHistoryCacheEntry{fetchedAt: now}
		okxMarketHistoryCache.Unlock()
		if err != nil {
			logger.Warnf("OKX market history initial fetch failed key=%s: %v", cacheKey, err)
		}
		return nil
	}
	okxMarketHistoryCache.entries[cacheKey] = okxMarketHistoryCacheEntry{
		rows:       cloneOkxMarketHistoryRows(fallback),
		refreshing: true,
	}
	okxMarketHistoryCache.Unlock()
	go refreshOkxMarketHistoryRows(cacheKey, uniqueName)
	return fallback
}

func refreshOkxMarketHistoryRows(cacheKey, uniqueName string) {
	rows, err := fetchOkxPublicPositionHistoryFn(uniqueName)
	now := time.Now()
	okxMarketHistoryCache.Lock()
	defer okxMarketHistoryCache.Unlock()
	cached := okxMarketHistoryCache.entries[cacheKey]
	if err == nil && len(rows) > 0 {
		okxMarketHistoryCache.entries[cacheKey] = okxMarketHistoryCacheEntry{
			rows:      rows,
			fetchedAt: now,
		}
		return
	}
	cached.refreshing = false
	cached.fetchedAt = now
	okxMarketHistoryCache.entries[cacheKey] = cached
	if err != nil {
		logger.Warnf("OKX market history refresh failed key=%s: %v", cacheKey, err)
	}
}

func loadEmbeddedOkxMarketHistoryRows(raw []byte) []okxMarketHistoryRow {
	var rows []okxMarketHistoryRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil
	}
	return rows
}

func cloneOkxMarketHistoryRows(rows []okxMarketHistoryRow) []okxMarketHistoryRow {
	if len(rows) == 0 {
		return nil
	}
	out := make([]okxMarketHistoryRow, len(rows))
	copy(out, rows)
	return out
}

func fetchOkxPublicPositionHistory(uniqueName string) ([]okxMarketHistoryRow, error) {
	client := &http.Client{Timeout: 18 * time.Second}
	seen := make(map[string]struct{})
	rows := make([]okxMarketHistoryRow, 0, okxMarketHistoryPageSize)
	after := ""

	for page := 0; page < okxMarketHistoryMaxPages; page++ {
		req, err := http.NewRequest(http.MethodGet, "https://www.okx.com/priapi/v5/ecotrade/public/position-history", nil)
		if err != nil {
			return nil, err
		}
		q := req.URL.Query()
		q.Set("uniqueName", uniqueName)
		q.Set("instType", "SWAP")
		q.Set("size", strconv.Itoa(okxMarketHistoryPageSize))
		if after != "" {
			q.Set("after", after)
		}
		req.URL.RawQuery = q.Encode()
		req.Header.Set("User-Agent", "Mozilla/5.0")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("App-Type", "web")
		req.Header.Set("x-locale", "zh_CN")
		req.Header.Set("Referer", "https://www.okx.com/zh-hans/copy-trading/account/"+uniqueName+"?tab=trade")

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		var payload okxPublicPositionHistoryResponse
		err = json.NewDecoder(resp.Body).Decode(&payload)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 || payload.Code != "0" {
			return nil, fmt.Errorf("okx history status=%d code=%s msg=%s", resp.StatusCode, payload.Code, payload.Msg)
		}
		if len(payload.Data) == 0 {
			break
		}

		newRows := 0
		for _, item := range payload.Data {
			id := strings.TrimSpace(item.ID)
			if id == "" {
				id = strings.TrimSpace(item.TradeItemID)
			}
			if id != "" {
				if _, ok := seen[id]; ok {
					continue
				}
				seen[id] = struct{}{}
			}
			row, ok := okxPublicPositionToHistoryRow(item)
			if !ok {
				continue
			}
			rows = append(rows, row)
			newRows++
		}
		last := payload.Data[len(payload.Data)-1]
		after = strings.TrimSpace(last.ID)
		if after == "" {
			after = strings.TrimSpace(last.TradeItemID)
		}
		if len(payload.Data) < okxMarketHistoryPageSize || newRows == 0 || after == "" {
			break
		}
	}
	return rows, nil
}

func okxPublicPositionToHistoryRow(item okxPublicPositionHistoryItem) (okxMarketHistoryRow, bool) {
	symbol, base := okxInstIDToSymbol(item.InstID)
	if symbol == "" {
		return okxMarketHistoryRow{}, false
	}
	opened := okxMarketHistoryMS(item.OpenTime)
	if opened == "" {
		return okxMarketHistoryRow{}, false
	}
	closed := okxMarketHistoryMS(item.UTime)
	if closed == "" {
		closed = "--"
	}
	return okxMarketHistoryRow{
		Symbol:        symbol,
		Leverage:      okxMarketInt(item.Lever, 1),
		MarginMode:    okxMarketMarginMode(item.MgnMode),
		Direction:     okxMarketDirection(item.PosSide, item.Side),
		Opened:        opened,
		Closed:        closed,
		EntryText:     okxMarketPriceText(symbol, item.OpenAvgPx),
		ExitText:      okxMarketPriceText(symbol, item.CloseAvgPx),
		PnL:           okxMarketRound(okxMarketFloat(item.PnL), 2),
		ClosedVolText: okxMarketVolumeText(base, item.SubPos, item.ContractVal, item.Multiplier),
	}, true
}

func okxInstIDToSymbol(instID string) (string, string) {
	parts := strings.Split(strings.ToUpper(strings.TrimSpace(instID)), "-")
	if len(parts) < 2 {
		return "", ""
	}
	base := parts[0]
	return base + parts[1], base
}

func okxMarketHistoryMS(raw string) string {
	ms := okxMarketFloat(raw)
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(int64(ms)).In(time.FixedZone("CST", 8*3600)).Format("2006/01/02 15:04:05")
}

func okxMarketMarginMode(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), "cross") {
		return "全仓"
	}
	return "逐仓"
}

func okxMarketDirection(posSide, side string) string {
	raw := strings.ToLower(strings.TrimSpace(posSide))
	if raw == "" {
		raw = strings.ToLower(strings.TrimSpace(side))
	}
	if raw == "short" || raw == "sell" {
		return "空"
	}
	return "多"
}

func okxMarketInt(raw string, fallback int) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func okxMarketFloat(raw string) float64 {
	n, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(raw), ",", ""), 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0
	}
	return n
}

func okxMarketRound(v float64, places int) float64 {
	pow := math.Pow10(places)
	return math.Round(v*pow) / pow
}

func okxMarketPriceText(symbol, raw string) string {
	v := okxMarketFloat(raw)
	abs := math.Abs(v)
	places := 2
	switch {
	case symbol == "BTCUSDT":
		places = 1
	case abs >= 100:
		places = 2
	case abs >= 1:
		places = 3
	case abs >= 0.1:
		places = 4
	default:
		places = 5
	}
	return okxFormatMarketNumber(v, places, false) + " USDT"
}

func okxMarketVolumeText(base, subPosRaw, contractValRaw, multiplierRaw string) string {
	subPos := math.Abs(okxMarketFloat(subPosRaw))
	contractVal := okxMarketFloat(contractValRaw)
	if contractVal == 0 {
		contractVal = 1
	}
	multiplier := okxMarketFloat(multiplierRaw)
	if multiplier == 0 {
		multiplier = 1
	}
	volume := subPos * contractVal * multiplier

	switch base {
	case "BTC":
		return okxFormatMarketNumber(volume, 4, false) + " " + base
	case "ETH", "SNDK":
		return okxFormatMarketNumber(volume, 3, false) + " " + base
	default:
		return okxFormatMarketNumber(volume, 4, true) + " " + base
	}
}

func okxFormatMarketNumber(v float64, places int, trim bool) string {
	text := fmt.Sprintf("%.*f", places, okxMarketRound(v, places))
	if trim && strings.Contains(text, ".") {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}
	parts := strings.SplitN(text, ".", 2)
	intPart := parts[0]
	sign := ""
	if strings.HasPrefix(intPart, "-") {
		sign = "-"
		intPart = strings.TrimPrefix(intPart, "-")
	}
	for i := len(intPart) - 3; i > 0; i -= 3 {
		intPart = intPart[:i] + "," + intPart[i:]
	}
	if len(parts) == 2 {
		return sign + intPart + "." + parts[1]
	}
	return sign + intPart
}
