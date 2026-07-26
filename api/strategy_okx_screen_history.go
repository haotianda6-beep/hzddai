package api

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"nofx/store"

	"github.com/gin-gonic/gin"
)

const (
	okx07MarketStrategyID      = "okx-screen-mirror-07"
	okx07MarketOKXUniqueName   = ""
	okx07MarketDisplayInitial  = 25000.0
	okx07MarketHistoryLimit    = 300
	okx07MarketStatsWindowDays = 90

	okx08MarketStrategyID      = "okx-screen-mirror-08"
	okx08MarketOKXUniqueName   = ""
	okx08MarketDisplayInitial  = 25000.0
	okx08MarketHistoryLimit    = 300
	okx08MarketStatsWindowDays = 90
)

type okxScreenMarketHistoryConfig struct {
	Prefix          string
	StrategyID      string
	UniqueName      string
	FallbackJSON    []byte
	DisplayInitial  float64
	HistoryLimit    int
	StatsWindowDays int
}

type okxScreenMarketData struct {
	Rollup       *store.AggregatedTradingRollup
	Trend        []float64
	TradeHistory []gin.H
}

//go:embed data/okx07_history.json
var okx07HistoryJSON []byte

//go:embed data/okx08_history.json
var okx08HistoryJSON []byte

var okxMarketUniqueNameMu sync.Mutex

func okxMarketHistoryUniqueName(strategyID, fallback string) string {
	suffix := strings.TrimPrefix(strategyID, "okx-screen-mirror-")
	keys := []string{}
	if suffix != strategyID && suffix != "" {
		keys = append(keys,
			"OKX_MARKET_HISTORY_UNIQUE_"+suffix,
			"OKX_MARKET_HISTORY_UNIQUE_"+strings.TrimLeft(suffix, "0"),
		)
	}
	keys = append(keys, "OKX_MARKET_HISTORY_UNIQUE_"+strings.ToUpper(strings.ReplaceAll(strategyID, "-", "_")))
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	if v := readOkxMarketHistoryUniqueName(strategyID, suffix); v != "" {
		return v
	}
	return fallback
}

func okxMarketHistoryUniqueNamePath() string {
	if v := strings.TrimSpace(os.Getenv("OKX_MARKET_HISTORY_UNIQUE_FILE")); v != "" {
		return v
	}
	return filepath.Join("data", "okx_market_unique_names.json")
}

func readOkxMarketHistoryUniqueName(strategyID, suffix string) string {
	okxMarketUniqueNameMu.Lock()
	defer okxMarketUniqueNameMu.Unlock()

	raw, err := os.ReadFile(okxMarketHistoryUniqueNamePath())
	if err != nil {
		return ""
	}
	var values map[string]string
	if json.Unmarshal(raw, &values) != nil {
		return ""
	}
	for _, key := range []string{strategyID, suffix, strings.TrimLeft(suffix, "0")} {
		if v := normalizeOkxMarketUniqueName(values[key]); v != "" {
			return v
		}
	}
	return ""
}

func rememberOkxMarketHistoryUniqueNameFromRelay(port int, body []byte) {
	strategyID := okxMarketStrategyIDForPort(port)
	if strategyID == "" {
		return
	}
	var payload struct {
		UniqueName string `json:"unique_name"`
		UniqueCode string `json:"uniqueCode"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return
	}
	uniqueName := normalizeOkxMarketUniqueName(payload.UniqueName)
	if uniqueName == "" {
		uniqueName = normalizeOkxMarketUniqueName(payload.UniqueCode)
	}
	if uniqueName == "" {
		return
	}

	suffix := strings.TrimPrefix(strategyID, "okx-screen-mirror-")
	path := okxMarketHistoryUniqueNamePath()
	okxMarketUniqueNameMu.Lock()
	defer okxMarketUniqueNameMu.Unlock()

	values := map[string]string{}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &values)
	}
	values[strategyID] = uniqueName
	if suffix != strategyID && suffix != "" {
		values[suffix] = uniqueName
		values[strings.TrimLeft(suffix, "0")] = uniqueName
	}
	raw, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, raw, 0644) == nil {
		_ = os.Rename(tmp, path)
	}
}

func normalizeOkxMarketUniqueName(raw string) string {
	v := strings.TrimSpace(raw)
	if len(v) < 6 || len(v) > 64 {
		return ""
	}
	for _, ch := range v {
		if (ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || ch == '_' || ch == '-' {
			continue
		}
		return ""
	}
	return v
}

func okxMarketStrategyIDForPort(port int) string {
	switch port {
	case 18765:
		return okx01MarketStrategyID
	case 18766:
		return okx02MarketStrategyID
	case 18767:
		return okx03MarketStrategyID
	case 18768:
		return okx04MarketStrategyID
	case 18769:
		return okx05MarketStrategyID
	case 18770:
		return okx06MarketStrategyID
	case 18771:
		return okx07MarketStrategyID
	case 18772:
		return okx08MarketStrategyID
	default:
		return ""
	}
}

func okx07MarketHistoryConfig() okxScreenMarketHistoryConfig {
	return okxScreenMarketHistoryConfig{
		Prefix:          "okx07",
		StrategyID:      okx07MarketStrategyID,
		UniqueName:      okxMarketHistoryUniqueName(okx07MarketStrategyID, okx07MarketOKXUniqueName),
		FallbackJSON:    okx07HistoryJSON,
		DisplayInitial:  okx07MarketDisplayInitial,
		HistoryLimit:    okx07MarketHistoryLimit,
		StatsWindowDays: okx07MarketStatsWindowDays,
	}
}

func okx08MarketHistoryConfig() okxScreenMarketHistoryConfig {
	return okxScreenMarketHistoryConfig{
		Prefix:          "okx08",
		StrategyID:      okx08MarketStrategyID,
		UniqueName:      okxMarketHistoryUniqueName(okx08MarketStrategyID, okx08MarketOKXUniqueName),
		FallbackJSON:    okx08HistoryJSON,
		DisplayInitial:  okx08MarketDisplayInitial,
		HistoryLimit:    okx08MarketHistoryLimit,
		StatsWindowDays: okx08MarketStatsWindowDays,
	}
}

func buildOkx07MarketData(includeHistory bool) *okxScreenMarketData {
	return buildOkxScreenMarketData(okx07MarketHistoryConfig(), includeHistory)
}

func buildOkx08MarketData(includeHistory bool) *okxScreenMarketData {
	return buildOkxScreenMarketData(okx08MarketHistoryConfig(), includeHistory)
}

func buildOkxScreenMarketData(cfg okxScreenMarketHistoryConfig, includeHistory bool) *okxScreenMarketData {
	rows := loadOkxMarketHistoryRows(cfg.StrategyID, cfg.UniqueName, cfg.FallbackJSON)
	if len(rows) == 0 {
		out := &okxScreenMarketData{
			Rollup: &store.AggregatedTradingRollup{},
			Trend:  []float64{cfg.DisplayInitial},
		}
		if includeHistory {
			out.TradeHistory = []gin.H{}
		}
		return out
	}
	sort.Slice(rows, func(i, j int) bool {
		sortI := okxScreenHistorySortTime(rows[i])
		sortJ := okxScreenHistorySortTime(rows[j])
		if !sortI.Equal(sortJ) {
			return sortI.Before(sortJ)
		}
		return okxScreenHistoryTime(rows[i].Opened).Before(okxScreenHistoryTime(rows[j].Opened))
	})

	rollup := &store.AggregatedTradingRollup{}
	var pnls []float64
	equitySeries := []float64{cfg.DisplayInitial}
	equity := cfg.DisplayInitial
	peak := equity
	maxDD := 0.0
	var holdMsSum float64
	var holdN int
	for _, row := range rows {
		pnl := row.PnL
		rollup.Stats.TotalTrades++
		rollup.Stats.TotalPnL += pnl
		pnls = append(pnls, pnl)
		if pnl > 0 {
			rollup.Stats.WinTrades++
			rollup.GrossProfit += pnl
		} else if pnl < 0 {
			rollup.Stats.LossTrades++
			rollup.GrossLoss += -pnl
		}
		if row.Direction == "多" {
			rollup.LongTrades++
		} else {
			rollup.ShortTrades++
		}
		opened := okxScreenHistoryTime(row.Opened)
		closed := okxScreenHistoryTime(row.Closed)
		if !opened.IsZero() && !closed.IsZero() && closed.After(opened) {
			holdMsSum += float64(closed.Sub(opened).Milliseconds())
			holdN++
		}
		equity += pnl
		if equity > peak {
			peak = equity
		}
		if peak > 0 {
			dd := (peak - equity) / peak * 100
			if dd > maxDD {
				maxDD = dd
			}
		}
		equitySeries = append(equitySeries, roundMarket2(equity))
	}
	if holdN > 0 {
		rollup.AvgHoldMs = holdMsSum / float64(holdN)
	}
	if rollup.Stats.TotalTrades > 0 {
		rollup.Stats.WinRate = roundMarket2(float64(rollup.Stats.WinTrades) / float64(rollup.Stats.TotalTrades) * 100)
	}
	if rollup.GrossLoss > 0 {
		rollup.Stats.ProfitFactor = roundMarket2(rollup.GrossProfit / rollup.GrossLoss)
	} else if rollup.GrossProfit > 0 {
		rollup.Stats.ProfitFactor = 999
	}
	if rollup.Stats.WinTrades > 0 {
		rollup.Stats.AvgWin = roundMarket2(rollup.GrossProfit / float64(rollup.Stats.WinTrades))
	}
	if rollup.Stats.LossTrades > 0 {
		rollup.Stats.AvgLoss = roundMarket2(rollup.GrossLoss / float64(rollup.Stats.LossTrades))
	}
	rollup.Stats.TotalPnL = roundMarket2(rollup.Stats.TotalPnL)
	rollup.GrossProfit = roundMarket2(rollup.GrossProfit)
	rollup.GrossLoss = roundMarket2(rollup.GrossLoss)
	rollup.Stats.SharpeRatio = sharpeFromScaledPnls(pnls, cfg.DisplayInitial)
	rollup.Stats.MaxDrawdownPct = roundMarket2(maxDD)

	out := &okxScreenMarketData{Rollup: rollup, Trend: equitySeries}
	if includeHistory {
		sort.Slice(rows, func(i, j int) bool {
			sortI := okxScreenHistorySortTime(rows[i])
			sortJ := okxScreenHistorySortTime(rows[j])
			if !sortI.Equal(sortJ) {
				return sortI.After(sortJ)
			}
			return okxScreenHistoryTime(rows[i].Opened).After(okxScreenHistoryTime(rows[j].Opened))
		})
		limit := cfg.HistoryLimit
		if len(rows) < limit {
			limit = len(rows)
		}
		out.TradeHistory = make([]gin.H, 0, limit)
		for i := 0; i < limit; i++ {
			out.TradeHistory = append(out.TradeHistory, okxScreenTradeHistoryRow(cfg.Prefix, rows[i], i))
		}
	}
	return out
}

func okxScreenHistoryTime(s string) time.Time {
	loc := time.FixedZone("CST", 8*3600)
	t, err := time.ParseInLocation("2006/01/02 15:04:05", strings.TrimSpace(s), loc)
	if err != nil {
		return time.Time{}
	}
	return t
}

func okxScreenHistorySortTime(row okxMarketHistoryRow) time.Time {
	if closed := okxScreenHistoryTime(row.Closed); !closed.IsZero() {
		return closed
	}
	return okxScreenHistoryTime(row.Opened)
}

func okxScreenDisplayTime(s string) string {
	t := okxScreenHistoryTime(s)
	if t.IsZero() {
		return strings.TrimSpace(s)
	}
	return t.Format("2006-01-02 15:04:05")
}

func okxScreenTradeHistoryRow(prefix string, row okxMarketHistoryRow, idx int) gin.H {
	lev := row.Leverage
	if lev <= 0 {
		lev = 1
	}
	return gin.H{
		"id":              fmt.Sprintf("%s-shot-%d", prefix, idx),
		"symbol":          strings.ToUpper(strings.TrimSpace(row.Symbol)),
		"contractLabel":   "永续",
		"leverage":        fmt.Sprintf("%d倍", lev),
		"marginMode":      row.MarginMode,
		"direction":       row.Direction,
		"status":          "已平仓",
		"opened":          okxScreenDisplayTime(row.Opened),
		"entryPrice":      row.EntryText,
		"maxOpenInterest": row.ClosedVolText,
		"closingPnl":      hzStarPnlText(row.PnL),
		"closed":          okxScreenDisplayTime(row.Closed),
		"avgClosePrice":   row.ExitText,
		"closedVol":       row.ClosedVolText,
	}
}

func applyOkxScreenMarketListOverlay(item gin.H, cfg okxScreenMarketHistoryConfig) {
	data := buildOkxScreenMarketData(cfg, false)
	if data == nil || data.Rollup == nil {
		applyEmptyOkxMarketListOverlay(item, cfg.StatsWindowDays)
		return
	}
	stats, ok := item["stats"].(gin.H)
	if !ok {
		return
	}
	stats["data_complete"] = true
	stats["total_aum"] = roundMarket2(cfg.DisplayInitial + data.Rollup.Stats.TotalPnL)
	stats["return_7d_pct"] = roundMarket2((data.Rollup.Stats.TotalPnL / cfg.DisplayInitial) * 100)
	stats["max_drawdown_pct"] = data.Rollup.Stats.MaxDrawdownPct
	stats["stats_window_days"] = cfg.StatsWindowDays
	stats["trend"] = sampleMarketTrend(data.Trend, 18)
	item["exchange_type"] = "OKX"
}

func applyOkx07MarketListOverlay(item gin.H) {
	applyOkxScreenMarketListOverlay(item, okx07MarketHistoryConfig())
}

func applyOkx08MarketListOverlay(item gin.H) {
	applyOkxScreenMarketListOverlay(item, okx08MarketHistoryConfig())
}

func applyEmptyOkxMarketListOverlay(item gin.H, statsWindowDays int) {
	stats, ok := item["stats"].(gin.H)
	if !ok {
		return
	}
	stats["data_complete"] = false
	stats["total_aum"] = 0
	stats["return_7d_pct"] = 0
	stats["max_drawdown_pct"] = 0
	stats["stats_window_days"] = statsWindowDays
	stats["trend"] = []float64{}
	item["exchange_type"] = "OKX"
}
