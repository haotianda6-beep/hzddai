package api

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"nofx/store"

	"github.com/gin-gonic/gin"
)

// 交易员数据看板演示叠加：对指定 trader_id 返回与「静默测试」策略市场一致的虚拟汇总，不落库。
//
// 环境变量：
//
//	NOFX_TRADER_DASHBOARD_DEMO=1
//	NOFX_TRADER_DEMO_IDS=逗号分隔的 trader_id（可选；为空则使用内置默认 ID）
//
// 默认内置包含名称「静默测试」对应示例交易员 ID（可在部署时通过 NOFX_TRADER_DEMO_IDS 覆盖）。

const silentTestDashboardTraderIDDefault = "30702a29_a1eb0d02-21a5-435f-99f4-13dbf8e56cfe_comkun_ai_1778137535"

func traderDashboardDemoEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("NOFX_TRADER_DASHBOARD_DEMO")))
	switch v {
	case "1", "true", "yes", "on", "demo":
		return true
	default:
		return false
	}
}

func traderDashboardDemoIDList() []string {
	raw := strings.TrimSpace(os.Getenv("NOFX_TRADER_DEMO_IDS"))
	if raw == "" {
		return []string{silentTestDashboardTraderIDDefault}
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{silentTestDashboardTraderIDDefault}
	}
	return out
}

func isTraderDashboardDemoTrader(traderID string) bool {
	if !traderDashboardDemoEnabled() {
		return false
	}
	tid := strings.TrimSpace(traderID)
	if tid == "" {
		return false
	}
	for _, id := range traderDashboardDemoIDList() {
		if tid == strings.TrimSpace(id) {
			return true
		}
	}
	return false
}

func applySilentTestTraderAccountDemo(account map[string]interface{}) {
	init := stestInitialCap
	eq := stestEndEquity
	pnl := stestTotalPnL
	pct := stestReturnPct
	account["total_equity"] = eq
	account["wallet_balance"] = math.Round(eq*0.94*100) / 100
	account["unrealized_profit"] = 0.0
	account["available_balance"] = math.Round(eq*0.88*100) / 100
	account["total_pnl"] = pnl
	account["total_pnl_pct"] = pct
	account["initial_balance"] = init
	account["daily_pnl"] = 0.0
	account["position_count"] = 0
	account["margin_used"] = 0.0
	account["margin_used_pct"] = 0.0
}

// buildSilentTestEquityHistoryDemo 与策略市场同一套净值曲线；时间轴为「当前 UTC 往前 30 天」至当前
func buildSilentTestEquityHistoryDemo() []gin.H {
	trend := marketDemoWavyEquityTrendSilentTest("silent-test-dashboard", stestInitialCap, stestEndEquity, stestTrendPoints)
	start := silentTestWindowStartUTC()
	n := len(trend)
	if n == 0 {
		return nil
	}
	span := float64(stestCurveWindowDays) * 24 * 3600
	init := stestInitialCap
	out := make([]gin.H, n)
	for i, eq := range trend {
		var ts time.Time
		if n == 1 {
			ts = start
		} else {
			sec := span * float64(i) / float64(n-1)
			ts = start.Add(time.Duration(sec) * time.Second)
		}
		pnl := eq - init
		pnlPct := 0.0
		if init > 0 {
			pnlPct = (pnl / init) * 100
		}
		out[i] = gin.H{
			"timestamp":         ts.Format("2006-01-02 15:04:05"),
			"total_equity":      eq,
			"available_balance": math.Round(eq*0.91*100) / 100,
			"total_pnl":         pnl,
			"total_pnl_pct":     pnlPct,
			"pnl":               pnl,
			"pnl_pct":           pnlPct,
			"position_count":    0,
			"margin_used_pct":   0.0,
		}
	}
	return out
}

func buildSilentTestTraderStats() *store.TraderStats {
	pf := stestGrossProfit / stestGrossLoss
	return &store.TraderStats{
		TotalTrades:    stestTotalTrades,
		WinTrades:      stestWinTrades,
		LossTrades:     stestLossTrades,
		WinRate:        stestWinRatePct,
		ProfitFactor:   math.Round(pf*100) / 100,
		SharpeRatio:    stestSharpe,
		TotalPnL:       stestTotalPnL,
		TotalFee:       stestTotalFee,
		AvgWin:         stestGrossProfit / float64(stestWinTrades),
		AvgLoss:        stestGrossLoss / float64(stestLossTrades),
		MaxDrawdownPct: stestMaxDrawdownPct,
	}
}

func buildSilentTestPositionHistoryPayload(traderID string, limit int) gin.H {
	hl := silentTestTradeHighlights()
	if limit > 0 && len(hl) > limit {
		hl = hl[:limit]
	}
	positions := make([]gin.H, 0, len(hl))
	for i, row := range hl {
		sym, _ := row["symbol"].(string)
		sideRaw, _ := row["side"].(string)
		entry, _ := row["entry_price"].(float64)
		exit, _ := row["exit_price"].(float64)
		pnl, _ := row["pnl_usdt"].(float64)
		opened, _ := row["opened_at"].(string)
		closedStr, _ := row["closed_at"].(string)
		side := "LONG"
		if strings.EqualFold(sideRaw, "short") {
			side = "SHORT"
		}
		tOpen, err := time.Parse(time.RFC3339, opened)
		if err != nil {
			tOpen = time.Now().UTC()
		}
		tClose := tOpen.Add(55 * time.Minute)
		if strings.TrimSpace(closedStr) != "" {
			if tc, err2 := time.Parse(time.RFC3339, closedStr); err2 == nil {
				tClose = tc
			}
		}
		qty := math.Abs(pnl) / math.Max(1e-9, math.Abs(exit-entry))
		if qty < 1e-8 || math.IsNaN(qty) {
			qty = 1200 / math.Max(entry, 1e-6)
		}
		fee := math.Round((5+math.Mod(float64(i*11), 40))*100) / 10000
		positions = append(positions, gin.H{
			"id":              i + 1,
			"trader_id":       traderID,
			"exchange_id":     "demo",
			"exchange_type":   "binance",
			"symbol":          sym,
			"side":            side,
			"quantity":        math.Round(qty*1e6) / 1e6,
			"entry_quantity":  math.Round(qty*1e6) / 1e6,
			"entry_price":     entry,
			"entry_order_id":  "demo-e-" + strconv.Itoa(i),
			"entry_time":      tOpen.Format(time.RFC3339),
			"exit_price":      exit,
			"exit_order_id":   "demo-x-" + strconv.Itoa(i),
			"exit_time":       tClose.Format(time.RFC3339),
			"realized_pnl":    math.Round(pnl*100) / 100,
			"fee":             fee,
			"leverage":        20,
			"status":          "CLOSED",
			"close_reason":    "demo",
			"created_at":      tOpen.Format(time.RFC3339),
			"updated_at":      tClose.Format(time.RFC3339),
		})
	}
	stats := buildSilentTestTraderStats()
	symbolStats := []gin.H{
		{"symbol": "BTCUSDT", "total_trades": 20, "win_trades": 15, "win_rate": 75.89, "total_pnl": 6200, "avg_pnl": 310.0, "avg_hold_mins": 118},
		{"symbol": "ETHUSDT", "total_trades": 16, "win_trades": 12, "win_rate": 75.0, "total_pnl": 5100, "avg_pnl": 318.75, "avg_hold_mins": 105},
		{"symbol": "SOLUSDT", "total_trades": 14, "win_trades": 11, "win_rate": 78.57, "total_pnl": 4800, "avg_pnl": 342.86, "avg_hold_mins": 96},
	}
	dirStats := []gin.H{
		{"side": "LONG", "trade_count": stestLongTrades, "win_rate": 75.89, "total_pnl": stestTotalPnL * 0.58, "avg_pnl": stestTotalPnL * 0.58 / float64(stestLongTrades)},
		{"side": "SHORT", "trade_count": stestShortTrades, "win_rate": 75.89, "total_pnl": stestTotalPnL * 0.42, "avg_pnl": stestTotalPnL * 0.42 / float64(stestShortTrades)},
	}
	return gin.H{
		"positions":       positions,
		"stats":           stats,
		"symbol_stats":    symbolStats,
		"direction_stats": dirStats,
	}
}

func buildSilentTestStatistics() *store.Statistics {
	const tc = 10000
	const ok = 7589
	return &store.Statistics{
		TotalCycles:         tc,
		SuccessfulCycles:    ok,
		FailedCycles:        tc - ok,
		TotalOpenPositions:  0,
		TotalClosePositions: stestTotalTrades,
		TotalTrades:         stestTotalTrades,
		WinTrades:           stestWinTrades,
		LossTrades:          stestLossTrades,
		WinRate:             stestWinRatePct,
	}
}

func buildSilentTestDecisionRecords(traderID string) []*store.DecisionRecord {
	rows := silentTestAIInsights()
	out := make([]*store.DecisionRecord, 0, len(rows))
	for i, row := range rows {
		atStr, _ := row["at"].(string)
		content, _ := row["content"].(string)
		ts, err := time.Parse(time.RFC3339, atStr)
		if err != nil {
			ts = time.Now().UTC()
		}
		cycle := i + 1
		cot := fmt.Sprintf(`【扫描结论 · 周期 #%d】%s（UTC）

【BTCUSDT】4H 仍在箱体上半区震荡；15m 缩量回踩未破坏昨日低点。**倾向**：轻仓顺势，止损置于箱体下沿外侧约 0.35%%。
【ETHUSDT】ETH/BTC 走平，跟随 BTC；资金费率中性，拥挤度不高。
【SOLUSDT】波动率略高于均值，权重略低于 BTC/ETH；短线避免追涨杀跌。

成交量与 OI：主流合约持仓变动温和，未见极端单边押注；短线以区间思路为主，突破后再评估加仓。

【本轮策略小结】%s

【纪律】单笔风险 ≤ 净值 1.2%%；宏观数据窗口前后优先减仓而非加仓。`, cycle, ts.Format("01-02 15:04"), content)

		decisions := []store.DecisionAction{
			{
				Action:     "wait",
				Symbol:     "BTCUSDT",
				Leverage:   20,
				Confidence: 72,
				Reasoning:  "BTC：箱体未有效突破前不追涨；回踩均线且量能配合再考虑加仓。",
				OrderID:    int64(cycle*100 + 1),
				Timestamp:  ts,
				Success:    true,
			},
			{
				Action:     "wait",
				Symbol:     "ETHUSDT",
				Leverage:   20,
				Confidence: 68,
				Reasoning:  "ETH：与大盘高相关，当前以跟随为主，避免单独押注方向。",
				OrderID:    int64(cycle*100 + 2),
				Timestamp:  ts,
				Success:    true,
			},
			{
				Action:     "wait",
				Symbol:     "SOLUSDT",
				Leverage:   15,
				Confidence: 65,
				Reasoning:  "SOL：波动放大时缩小下单阶梯，收紧止损防止噪声扫损。",
				OrderID:    int64(cycle*100 + 3),
				Timestamp:  ts,
				Success:    true,
			},
		}

		out = append(out, &store.DecisionRecord{
			ID:                  int64(i + 1),
			TraderID:            traderID,
			CycleNumber:         cycle,
			Timestamp:           ts,
			SystemPrompt:        "",
			InputPrompt:         fmt.Sprintf("演示周期 #%d · 市场快照（虚拟）已就绪。", cycle),
			CoTTrace:            cot,
			DecisionJSON:        `{"demo":true,"intent":"observe"}`,
			RawResponse:         `{"demo":true,"risk_level":"normal","notes":"演示数据，非实盘推理"}`,
			Success:             true,
			CandidateCoins:      []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT"},
			ExecutionLog:        []string{fmt.Sprintf("周期 #%d：已完成候选币种扫描、费率与 OI 复核（演示数据）。", cycle)},
			Decisions:           decisions,
			AIRequestDurationMs: 820 + int64(i*17)%380,
			AccountState: store.AccountSnapshot{
				TotalBalance:          stestEndEquity * 0.95,
				AvailableBalance:      stestEndEquity * 0.88,
				TotalUnrealizedProfit: 0,
				PositionCount:         0,
				MarginUsedPct:         0,
				InitialBalance:        stestInitialCap,
			},
		})
	}
	return out
}
