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
	okx06MarketStrategyID      = "okx-screen-mirror-06"
	okx06MarketOKXUniqueName   = ""
	okx06MarketDisplayInitial  = 25000.0
	okx06MarketHistoryLimit    = 300
	okx06MarketStatsWindowDays = 90
)

type okx06HistoryRow = okxMarketHistoryRow

//go:embed data/okx06_history.json
var okx06HistoryJSON []byte

func loadOkx06HistoryRows() []okx06HistoryRow {
	return loadOkxMarketHistoryRows(okx06MarketStrategyID, okxMarketHistoryUniqueName(okx06MarketStrategyID, okx06MarketOKXUniqueName), nil)
}

type okx06MarketData struct {
	Rollup       *store.AggregatedTradingRollup
	Trend        []float64
	TradeHistory []gin.H
}

func buildOkx06MarketData(includeHistory bool) *okx06MarketData {
	rows := loadOkx06HistoryRows()
	if len(rows) == 0 {
		return nil
	}
	sourceRows := append([]okx06HistoryRow(nil), rows...)
	sort.Slice(rows, func(i, j int) bool {
		sortI := okx06HistorySortTime(rows[i])
		sortJ := okx06HistorySortTime(rows[j])
		if !sortI.Equal(sortJ) {
			return sortI.Before(sortJ)
		}
		return okx06HistoryTime(rows[i].Opened).Before(okx06HistoryTime(rows[j].Opened))
	})

	rollup := &store.AggregatedTradingRollup{}
	var pnls []float64
	equitySeries := []float64{okx06MarketDisplayInitial}
	equity := okx06MarketDisplayInitial
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
		opened := okx06HistoryTime(row.Opened)
		closed := okx06HistoryTime(row.Closed)
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
	rollup.Stats.SharpeRatio = sharpeFromScaledPnls(pnls, okx06MarketDisplayInitial)
	rollup.Stats.MaxDrawdownPct = roundMarket2(maxDD)

	out := &okx06MarketData{Rollup: rollup, Trend: equitySeries}
	if includeHistory {
		limit := okx06MarketHistoryLimit
		if len(sourceRows) < limit {
			limit = len(sourceRows)
		}
		out.TradeHistory = make([]gin.H, 0, limit)
		for i := 0; i < limit; i++ {
			out.TradeHistory = append(out.TradeHistory, okx06TradeHistoryRow(sourceRows[i], i))
		}
	}
	return out
}

func okx06HistoryTime(s string) time.Time {
	loc := time.FixedZone("CST", 8*3600)
	t, err := time.ParseInLocation("2006/01/02 15:04:05", strings.TrimSpace(s), loc)
	if err != nil {
		return time.Time{}
	}
	return t
}

func okx06HistorySortTime(row okx06HistoryRow) time.Time {
	if closed := okx06HistoryTime(row.Closed); !closed.IsZero() {
		return closed
	}
	return okx06HistoryTime(row.Opened)
}

func okx06DisplayTime(s string) string {
	t := okx06HistoryTime(s)
	if t.IsZero() {
		return strings.TrimSpace(s)
	}
	return t.Format("2006-01-02 15:04:05")
}

func okx06TradeHistoryRow(row okx06HistoryRow, idx int) gin.H {
	lev := row.Leverage
	if lev <= 0 {
		lev = 1
	}
	return gin.H{
		"id":              fmt.Sprintf("okx06-shot-%d", idx),
		"symbol":          strings.ToUpper(strings.TrimSpace(row.Symbol)),
		"contractLabel":   "永续",
		"leverage":        fmt.Sprintf("%d倍", lev),
		"marginMode":      row.MarginMode,
		"direction":       row.Direction,
		"status":          "已平仓",
		"opened":          okx06DisplayTime(row.Opened),
		"entryPrice":      row.EntryText,
		"maxOpenInterest": row.ClosedVolText,
		"closingPnl":      hzStarPnlText(row.PnL),
		"closed":          okx06DisplayTime(row.Closed),
		"avgClosePrice":   row.ExitText,
		"closedVol":       row.ClosedVolText,
	}
}

func applyOkx06MarketListOverlay(item gin.H) {
	data := buildOkx06MarketData(false)
	if data == nil || data.Rollup == nil {
		applyEmptyOkxMarketListOverlay(item, okx06MarketStatsWindowDays)
		return
	}
	stats, ok := item["stats"].(gin.H)
	if !ok {
		return
	}
	stats["data_complete"] = true
	stats["total_aum"] = roundMarket2(okx06MarketDisplayInitial + data.Rollup.Stats.TotalPnL)
	stats["return_7d_pct"] = roundMarket2((data.Rollup.Stats.TotalPnL / okx06MarketDisplayInitial) * 100)
	stats["max_drawdown_pct"] = data.Rollup.Stats.MaxDrawdownPct
	stats["stats_window_days"] = okx06MarketStatsWindowDays
	stats["trend"] = sampleMarketTrend(data.Trend, 18)
	item["exchange_type"] = "OKX"
}
