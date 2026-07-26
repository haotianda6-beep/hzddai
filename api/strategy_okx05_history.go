package api

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"
	"time"

	"nofx/store"

	"github.com/gin-gonic/gin"
)

const (
	okx05MarketStrategyID      = "okx-screen-mirror-05"
	okx05MarketOKXUniqueName   = ""
	okx05MarketDisplayInitial  = 25000.0
	okx05MarketHistoryLimit    = 300
	okx05MarketStatsWindowDays = 90
)

type okx05HistoryRow = okxMarketHistoryRow

//go:embed data/okx05_history.json
var okx05HistoryJSON []byte

func loadOkx05HistoryRows() []okx05HistoryRow {
	return loadOkxMarketHistoryRows(okx05MarketStrategyID, okxMarketHistoryUniqueName(okx05MarketStrategyID, okx05MarketOKXUniqueName), nil)
}

type okx05MarketData struct {
	Rollup       *store.AggregatedTradingRollup
	Trend        []float64
	TradeHistory []gin.H
}

func buildOkx05MarketData(includeHistory bool) *okx05MarketData {
	rows := loadOkx05HistoryRows()
	if len(rows) == 0 {
		return nil
	}
	sourceRows := append([]okx05HistoryRow(nil), rows...)
	sort.Slice(rows, func(i, j int) bool {
		sortI := okx05HistorySortTime(rows[i])
		sortJ := okx05HistorySortTime(rows[j])
		if !sortI.Equal(sortJ) {
			return sortI.Before(sortJ)
		}
		return okx05HistoryTime(rows[i].Opened).Before(okx05HistoryTime(rows[j].Opened))
	})

	rollup := &store.AggregatedTradingRollup{}
	var pnls []float64
	equitySeries := []float64{okx05MarketDisplayInitial}
	equity := okx05MarketDisplayInitial
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
		opened := okx05HistoryTime(row.Opened)
		closed := okx05HistoryTime(row.Closed)
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
	rollup.Stats.SharpeRatio = sharpeFromScaledPnls(pnls, okx05MarketDisplayInitial)
	rollup.Stats.MaxDrawdownPct = roundMarket2(maxDD)

	out := &okx05MarketData{Rollup: rollup, Trend: equitySeries}
	if includeHistory {
		limit := okx05MarketHistoryLimit
		if len(sourceRows) < limit {
			limit = len(sourceRows)
		}
		out.TradeHistory = make([]gin.H, 0, limit)
		for i := 0; i < limit; i++ {
			out.TradeHistory = append(out.TradeHistory, okx05TradeHistoryRow(sourceRows[i], i))
		}
	}
	return out
}

func okx05HistoryTime(s string) time.Time {
	loc := time.FixedZone("CST", 8*3600)
	t, err := time.ParseInLocation("2006/01/02 15:04:05", strings.TrimSpace(s), loc)
	if err != nil {
		return time.Time{}
	}
	return t
}

func okx05HistorySortTime(row okx05HistoryRow) time.Time {
	if closed := okx05HistoryTime(row.Closed); !closed.IsZero() {
		return closed
	}
	return okx05HistoryTime(row.Opened)
}

func okx05DisplayTime(s string) string {
	t := okx05HistoryTime(s)
	if t.IsZero() {
		return strings.TrimSpace(s)
	}
	return t.Format("2006-01-02 15:04:05")
}

func okx05TradeHistoryRow(row okx05HistoryRow, idx int) gin.H {
	lev := row.Leverage
	if lev <= 0 {
		lev = 1
	}
	return gin.H{
		"id":              fmt.Sprintf("okx05-shot-%d", idx),
		"symbol":          strings.ToUpper(strings.TrimSpace(row.Symbol)),
		"contractLabel":   "永续",
		"leverage":        fmt.Sprintf("%d倍", lev),
		"marginMode":      row.MarginMode,
		"direction":       row.Direction,
		"status":          "已平仓",
		"opened":          okx05DisplayTime(row.Opened),
		"entryPrice":      row.EntryText,
		"maxOpenInterest": row.ClosedVolText,
		"closingPnl":      hzStarPnlText(row.PnL),
		"closed":          okx05DisplayTime(row.Closed),
		"avgClosePrice":   row.ExitText,
		"closedVol":       row.ClosedVolText,
	}
}

func applyOkx05MarketListOverlay(item gin.H) {
	data := buildOkx05MarketData(false)
	if data == nil || data.Rollup == nil {
		applyEmptyOkxMarketListOverlay(item, okx05MarketStatsWindowDays)
		return
	}
	stats, ok := item["stats"].(gin.H)
	if !ok {
		return
	}
	stats["data_complete"] = true
	stats["total_aum"] = roundMarket2(okx05MarketDisplayInitial + data.Rollup.Stats.TotalPnL)
	stats["return_7d_pct"] = roundMarket2((data.Rollup.Stats.TotalPnL / okx05MarketDisplayInitial) * 100)
	stats["max_drawdown_pct"] = data.Rollup.Stats.MaxDrawdownPct
	stats["stats_window_days"] = okx05MarketStatsWindowDays
	stats["trend"] = sampleMarketTrend(data.Trend, 18)
	item["exchange_type"] = "OKX"
}
