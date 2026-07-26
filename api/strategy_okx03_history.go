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
	okx03MarketStrategyID      = "okx-screen-mirror-03"
	okx03MarketOKXUniqueName   = "769075957323167141"
	okx03MarketDisplayInitial  = 5000.0
	okx03MarketHistoryLimit    = 500
	okx03MarketStatsWindowDays = 90
)

type okx03HistoryRow = okxMarketHistoryRow

//go:embed data/okx03_history.json
var okx03HistoryJSON []byte

func loadOkx03HistoryRows() []okx03HistoryRow {
	return loadOkxMarketHistoryRows(okx03MarketStrategyID, okx03MarketOKXUniqueName, okx03HistoryJSON)
}

type okx03MarketData struct {
	Rollup       *store.AggregatedTradingRollup
	Trend        []float64
	TradeHistory []gin.H
}

func buildOkx03MarketData(includeHistory bool) *okx03MarketData {
	rows := loadOkx03HistoryRows()
	if len(rows) == 0 {
		return nil
	}
	sort.Slice(rows, func(i, j int) bool {
		sortI := okx03HistorySortTime(rows[i])
		sortJ := okx03HistorySortTime(rows[j])
		if !sortI.Equal(sortJ) {
			return sortI.Before(sortJ)
		}
		return okx03HistoryTime(rows[i].Opened).Before(okx03HistoryTime(rows[j].Opened))
	})

	rollup := &store.AggregatedTradingRollup{}
	var pnls []float64
	equitySeries := []float64{okx03MarketDisplayInitial}
	equity := okx03MarketDisplayInitial
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
		opened := okx03HistoryTime(row.Opened)
		closed := okx03HistoryTime(row.Closed)
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
	rollup.Stats.SharpeRatio = sharpeFromScaledPnls(pnls, okx03MarketDisplayInitial)
	rollup.Stats.MaxDrawdownPct = roundMarket2(maxDD)

	out := &okx03MarketData{Rollup: rollup, Trend: equitySeries}
	if includeHistory {
		sort.Slice(rows, func(i, j int) bool {
			sortI := okx03HistorySortTime(rows[i])
			sortJ := okx03HistorySortTime(rows[j])
			if !sortI.Equal(sortJ) {
				return sortI.After(sortJ)
			}
			return okx03HistoryTime(rows[i].Opened).After(okx03HistoryTime(rows[j].Opened))
		})
		limit := okx03MarketHistoryLimit
		if len(rows) < limit {
			limit = len(rows)
		}
		out.TradeHistory = make([]gin.H, 0, limit)
		for i := 0; i < limit; i++ {
			out.TradeHistory = append(out.TradeHistory, okx03TradeHistoryRow(rows[i], i))
		}
	}
	return out
}

func okx03HistoryTime(s string) time.Time {
	loc := time.FixedZone("CST", 8*3600)
	t, err := time.ParseInLocation("2006/01/02 15:04:05", strings.TrimSpace(s), loc)
	if err != nil {
		return time.Time{}
	}
	return t
}

func okx03HistorySortTime(row okx03HistoryRow) time.Time {
	if closed := okx03HistoryTime(row.Closed); !closed.IsZero() {
		return closed
	}
	return okx03HistoryTime(row.Opened)
}

func okx03DisplayTime(s string) string {
	t := okx03HistoryTime(s)
	if t.IsZero() {
		return strings.TrimSpace(s)
	}
	return t.Format("2006-01-02 15:04:05")
}

func okx03TradeHistoryRow(row okx03HistoryRow, idx int) gin.H {
	lev := row.Leverage
	if lev <= 0 {
		lev = 1
	}
	return gin.H{
		"id":              fmt.Sprintf("okx03-shot-%d", idx),
		"symbol":          strings.ToUpper(strings.TrimSpace(row.Symbol)),
		"contractLabel":   "永续",
		"leverage":        fmt.Sprintf("%d倍", lev),
		"marginMode":      row.MarginMode,
		"direction":       row.Direction,
		"status":          "已平仓",
		"opened":          okx03DisplayTime(row.Opened),
		"entryPrice":      row.EntryText,
		"maxOpenInterest": row.ClosedVolText,
		"closingPnl":      hzStarPnlText(row.PnL),
		"closed":          okx03DisplayTime(row.Closed),
		"avgClosePrice":   row.ExitText,
		"closedVol":       row.ClosedVolText,
	}
}

func applyOkx03MarketListOverlay(item gin.H) {
	data := buildOkx03MarketData(false)
	if data == nil || data.Rollup == nil {
		return
	}
	stats, ok := item["stats"].(gin.H)
	if !ok {
		return
	}
	stats["data_complete"] = true
	stats["total_aum"] = roundMarket2(okx03MarketDisplayInitial + data.Rollup.Stats.TotalPnL)
	stats["return_7d_pct"] = roundMarket2((data.Rollup.Stats.TotalPnL / okx03MarketDisplayInitial) * 100)
	stats["max_drawdown_pct"] = data.Rollup.Stats.MaxDrawdownPct
	stats["stats_window_days"] = okx03MarketStatsWindowDays
	stats["trend"] = sampleMarketTrend(data.Trend, 18)
	item["exchange_type"] = "OKX"
}
