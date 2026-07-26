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
	okx02MarketStrategyID      = "okx-screen-mirror-02"
	okx02MarketOKXUniqueName   = "0870A246F3749D76"
	okx02MarketDisplayInitial  = 25000.0
	okx02MarketHistoryLimit    = 300
	okx02MarketStatsWindowDays = 90
)

type okx02HistoryRow = okxMarketHistoryRow

//go:embed data/okx02_history.json
var okx02HistoryJSON []byte

func loadOkx02HistoryRows() []okx02HistoryRow {
	return loadOkxMarketHistoryRows(okx02MarketStrategyID, okx02MarketOKXUniqueName, okx02HistoryJSON)
}

type okx02MarketData struct {
	Rollup       *store.AggregatedTradingRollup
	Trend        []float64
	TradeHistory []gin.H
}

func buildOkx02MarketData(includeHistory bool) *okx02MarketData {
	rows := loadOkx02HistoryRows()
	if len(rows) == 0 {
		return nil
	}
	sort.Slice(rows, func(i, j int) bool {
		closedI := okx02HistoryTime(rows[i].Closed)
		closedJ := okx02HistoryTime(rows[j].Closed)
		if !closedI.Equal(closedJ) {
			return closedI.Before(closedJ)
		}
		return okx02HistoryTime(rows[i].Opened).Before(okx02HistoryTime(rows[j].Opened))
	})

	rollup := &store.AggregatedTradingRollup{}
	var pnls []float64
	equitySeries := []float64{okx02MarketDisplayInitial}
	equity := okx02MarketDisplayInitial
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
		opened := okx02HistoryTime(row.Opened)
		closed := okx02HistoryTime(row.Closed)
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
	rollup.Stats.SharpeRatio = sharpeFromScaledPnls(pnls, okx02MarketDisplayInitial)
	rollup.Stats.MaxDrawdownPct = roundMarket2(maxDD)

	out := &okx02MarketData{Rollup: rollup, Trend: equitySeries}
	if includeHistory {
		sort.Slice(rows, func(i, j int) bool {
			closedI := okx02HistoryTime(rows[i].Closed)
			closedJ := okx02HistoryTime(rows[j].Closed)
			if !closedI.Equal(closedJ) {
				return closedI.After(closedJ)
			}
			return okx02HistoryTime(rows[i].Opened).After(okx02HistoryTime(rows[j].Opened))
		})
		limit := okx02MarketHistoryLimit
		if len(rows) < limit {
			limit = len(rows)
		}
		out.TradeHistory = make([]gin.H, 0, limit)
		for i := 0; i < limit; i++ {
			out.TradeHistory = append(out.TradeHistory, okx02TradeHistoryRow(rows[i], i))
		}
	}
	return out
}

func okx02HistoryTime(s string) time.Time {
	loc := time.FixedZone("CST", 8*3600)
	t, err := time.ParseInLocation("2006/01/02 15:04:05", strings.TrimSpace(s), loc)
	if err != nil {
		return time.Time{}
	}
	return t
}

func okx02DisplayTime(s string) string {
	t := okx02HistoryTime(s)
	if t.IsZero() {
		return strings.TrimSpace(s)
	}
	return t.Format("2006-01-02 15:04:05")
}

func okx02TradeHistoryRow(row okx02HistoryRow, idx int) gin.H {
	lev := row.Leverage
	if lev <= 0 {
		lev = 1
	}
	return gin.H{
		"id":              fmt.Sprintf("okx02-shot-%d", idx),
		"symbol":          strings.ToUpper(strings.TrimSpace(row.Symbol)),
		"contractLabel":   "永续",
		"leverage":        fmt.Sprintf("%d倍", lev),
		"marginMode":      row.MarginMode,
		"direction":       row.Direction,
		"status":          "已平仓",
		"opened":          okx02DisplayTime(row.Opened),
		"entryPrice":      row.EntryText,
		"maxOpenInterest": row.ClosedVolText,
		"closingPnl":      hzStarPnlText(row.PnL),
		"closed":          okx02DisplayTime(row.Closed),
		"avgClosePrice":   row.ExitText,
		"closedVol":       row.ClosedVolText,
	}
}

func applyOkx02MarketListOverlay(item gin.H) {
	data := buildOkx02MarketData(false)
	if data == nil || data.Rollup == nil {
		return
	}
	stats, ok := item["stats"].(gin.H)
	if !ok {
		return
	}
	stats["data_complete"] = true
	stats["total_aum"] = roundMarket2(okx02MarketDisplayInitial + data.Rollup.Stats.TotalPnL)
	stats["return_7d_pct"] = roundMarket2((data.Rollup.Stats.TotalPnL / okx02MarketDisplayInitial) * 100)
	stats["max_drawdown_pct"] = data.Rollup.Stats.MaxDrawdownPct
	stats["stats_window_days"] = okx02MarketStatsWindowDays
	stats["trend"] = sampleMarketTrend(data.Trend, 18)
	item["exchange_type"] = "OKX"
}
