package api

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"nofx/marketdata/observation"
	"nofx/store"

	"github.com/gin-gonic/gin"
)

type observationMarketData struct {
	SlotID           int
	ProfileID        string
	SourceAccountID  string
	DisplayName      string
	Disclosure       string
	StartAt          string
	EndAt            string
	HistorySHA256    string
	CompletedMonths  int
	MonthlyRows      int
	InitialBalance   float64
	CurrentBalance   float64
	CumulativeReturn float64
	Return7DPct      float64
	Rollup           *store.AggregatedTradingRollup
	Trend            []float64
	TradeHistory     []gin.H
}

func buildObservationMarketData(strategyID string, includeHistory bool) (*observationMarketData, error) {
	slot, ok := observation.ByStrategyID(strategyID)
	if !ok {
		return nil, nil
	}
	initial, err := observationNumber(slot.History.InitialBalance)
	if err != nil {
		return nil, err
	}
	current, err := observationNumber(slot.History.CurrentBalance)
	if err != nil {
		return nil, err
	}
	cumulative, err := observationNumber(slot.History.CumulativeReturn)
	if err != nil {
		return nil, err
	}
	maxDrawdown, err := observationNumber(slot.History.MaximumDrawdown)
	if err != nil {
		return nil, err
	}
	endAt, err := time.Parse(time.RFC3339Nano, slot.History.EndAt)
	if err != nil {
		return nil, err
	}

	trades := append([]observation.Trade(nil), slot.History.Trades...)
	sort.Slice(trades, func(i, j int) bool { return trades[i].EntryAt < trades[j].EntryAt })
	rollup := &store.AggregatedTradingRollup{}
	equity := []float64{initial}
	pnls := make([]float64, 0, len(trades))
	windowStart := initial
	cutoff := endAt.AddDate(0, 0, -7)
	var holdTotal float64
	for _, trade := range trades {
		pnl, parseErr := observationNumber(trade.NetPnL)
		if parseErr != nil {
			return nil, parseErr
		}
		fee, parseErr := observationNumber(trade.Fee)
		if parseErr != nil {
			return nil, parseErr
		}
		balance, parseErr := observationNumber(trade.BalanceAfter)
		if parseErr != nil {
			return nil, parseErr
		}
		entry, _ := time.Parse(time.RFC3339Nano, trade.EntryAt)
		exit, _ := time.Parse(time.RFC3339Nano, trade.ExitAt)
		if exit.Before(cutoff) {
			windowStart = balance
		}
		rollup.Stats.TotalTrades++
		rollup.Stats.TotalPnL += pnl
		rollup.Stats.TotalFee += fee
		pnls = append(pnls, pnl)
		if pnl > 0 {
			rollup.Stats.WinTrades++
			rollup.GrossProfit += pnl
		} else if pnl < 0 {
			rollup.Stats.LossTrades++
			rollup.GrossLoss += -pnl
		}
		if trade.Direction == "long" {
			rollup.LongTrades++
		} else {
			rollup.ShortTrades++
		}
		holdTotal += float64(exit.Sub(entry).Milliseconds())
		equity = append(equity, balance)
	}
	if len(trades) > 0 {
		rollup.AvgHoldMs = holdTotal / float64(len(trades))
		rollup.Stats.WinRate = roundMarket2(float64(rollup.Stats.WinTrades) / float64(len(trades)) * 100)
	}
	if rollup.GrossLoss > 0 {
		rollup.Stats.ProfitFactor = roundMarket2(rollup.GrossProfit / rollup.GrossLoss)
	}
	if rollup.Stats.WinTrades > 0 {
		rollup.Stats.AvgWin = roundMarket2(rollup.GrossProfit / float64(rollup.Stats.WinTrades))
	}
	if rollup.Stats.LossTrades > 0 {
		rollup.Stats.AvgLoss = roundMarket2(rollup.GrossLoss / float64(rollup.Stats.LossTrades))
	}
	rollup.Stats.TotalPnL = roundMarket2(rollup.Stats.TotalPnL)
	rollup.Stats.TotalFee = roundMarket2(rollup.Stats.TotalFee)
	rollup.GrossProfit = roundMarket2(rollup.GrossProfit)
	rollup.GrossLoss = roundMarket2(rollup.GrossLoss)
	rollup.Stats.SharpeRatio = sharpeFromScaledPnls(pnls, initial)
	rollup.Stats.MaxDrawdownPct = maxDrawdown

	data := &observationMarketData{
		SlotID: slot.Slot, ProfileID: slot.HistoryProfileID, SourceAccountID: slot.SourceAccountID,
		DisplayName: slot.DisplayName, Disclosure: slot.History.Disclosure,
		StartAt: slot.History.StartAt, EndAt: slot.History.EndAt, HistorySHA256: slot.HistorySHA256,
		CompletedMonths: slot.History.Months, MonthlyRows: len(slot.History.MonthlyResults),
		InitialBalance: initial, CurrentBalance: current, CumulativeReturn: cumulative,
		Rollup: rollup, Trend: equity,
	}
	if windowStart > 0 {
		data.Return7DPct = roundMarket2((current - windowStart) / windowStart * 100)
	}
	if includeHistory {
		data.TradeHistory = make([]gin.H, 0, len(trades))
		for i := len(trades) - 1; i >= 0; i-- {
			data.TradeHistory = append(data.TradeHistory, observationTradeHistoryRow(trades[i]))
		}
	}
	return data, nil
}

func observationNumber(raw string) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("invalid observation decimal")
	}
	return value, nil
}

func observationDisplayTime(raw string) string {
	value, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return strings.TrimSpace(raw)
	}
	return value.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05")
}

func observationTradeHistoryRow(trade observation.Trade) gin.H {
	direction := "多"
	if trade.Direction == "short" {
		direction = "空"
	}
	unit := strings.TrimSuffix(strings.ToUpper(trade.Instrument), "USDT")
	pnl, _ := observationNumber(trade.NetPnL)
	return gin.H{
		"id": "observation-" + trade.ID, "symbol": strings.ToUpper(trade.Instrument),
		"contractLabel": "策略成交", "leverage": "—", "marginMode": "—", "direction": direction,
		"status": "已完成", "opened": observationDisplayTime(trade.EntryAt),
		"entryPrice": trade.EntryPrice + " USDT", "maxOpenInterest": trade.Quantity + " " + unit,
		"closingPnl": hzStarPnlText(pnl), "closed": observationDisplayTime(trade.ExitAt),
		"avgClosePrice": trade.ExitPrice + " USDT", "closedVol": trade.Quantity + " " + unit,
		"performance_source": observation.StatusHistoricalSimulation,
	}
}

func applyObservationMetadata(item gin.H, data *observationMarketData) {
	item["exchange_type"] = "BALIB"
	item["performance_source"] = observation.StatusHistoricalSimulation
	item["performance_disclosure"] = "模拟业绩"
	if profile, ok := observation.MarketProfileBySlot(data.SlotID); ok {
		item["creator_display_name"] = profile.CreatorDisplayName
		item["creator_avatar_url"] = profile.CreatorAvatarURL
		item["market_sale_price_usdt"] = profile.MonthlyPriceUSDT
		item["market_subscription_monthly_only"] = true
	}
	item["history_slot"] = data.SlotID
	item["history_profile_id"] = data.ProfileID
	item["history_source_account_id"] = data.SourceAccountID
	item["history_artifact_sha256"] = observation.ExpectedArtifactSHA256
	item["history_sha256"] = data.HistorySHA256
	item["performance_start_at"] = data.StartAt
	item["performance_end_at"] = data.EndAt
	if stats, ok := item["stats"].(gin.H); ok {
		stats["data_complete"] = true
		stats["total_aum"] = data.CurrentBalance
		stats["return_7d_pct"] = data.Return7DPct
		stats["cumulative_return_pct"] = data.CumulativeReturn
		stats["max_drawdown_pct"] = data.Rollup.Stats.MaxDrawdownPct
		stats["trend"] = data.Trend
		stats["completed_months"] = data.CompletedMonths
		stats["monthly_rows"] = data.MonthlyRows
		stats["history_trade_count"] = data.Rollup.Stats.TotalTrades
		start, startErr := time.Parse(time.RFC3339Nano, data.StartAt)
		end, endErr := time.Parse(time.RFC3339Nano, data.EndAt)
		if startErr == nil && endErr == nil && end.After(start) {
			stats["stats_window_days"] = int(math.Ceil(end.Sub(start).Hours() / 24))
		}
	}
}

func applyObservationMarketListOverlay(item gin.H, strategyID string) (bool, error) {
	data, err := buildObservationMarketData(strategyID, false)
	if err != nil || data == nil {
		return false, err
	}
	applyObservationMetadata(item, data)
	return true, nil
}
