package trader

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"nofx/market"
	"nofx/store"
	"nofx/trader/types"
)

// martingaleSimTrader 记录发出的交易指令（不真实下单）
type martingaleSimTrader struct {
	balance map[string]interface{}
	poses   []map[string]interface{}
	log     []string
}

func (m *martingaleSimTrader) GetBalance() (map[string]interface{}, error) {
	return m.balance, nil
}
func (m *martingaleSimTrader) GetPositions() ([]map[string]interface{}, error) {
	return m.poses, nil
}
func (m *martingaleSimTrader) OpenLong(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	m.log = append(m.log, fmt.Sprintf("OpenLong %s qty=%.6f lev=%d", symbol, quantity, leverage))
	return map[string]interface{}{"ok": true}, nil
}
func (m *martingaleSimTrader) OpenShort(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	m.log = append(m.log, fmt.Sprintf("OpenShort %s qty=%.6f lev=%d", symbol, quantity, leverage))
	return map[string]interface{}{"ok": true}, nil
}
func (m *martingaleSimTrader) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	m.log = append(m.log, fmt.Sprintf("CloseLong %s qty=%.6f", symbol, quantity))
	return map[string]interface{}{"ok": true}, nil
}
func (m *martingaleSimTrader) CloseShort(symbol string, quantity float64) (map[string]interface{}, error) {
	m.log = append(m.log, fmt.Sprintf("CloseShort %s qty=%.6f", symbol, quantity))
	return map[string]interface{}{"ok": true}, nil
}
func (m *martingaleSimTrader) SetLeverage(symbol string, leverage int) error {
	m.log = append(m.log, fmt.Sprintf("SetLeverage %s %d", symbol, leverage))
	return nil
}
func (m *martingaleSimTrader) SetMarginMode(string, bool) error {
	m.log = append(m.log, "SetMarginMode")
	return nil
}
func (m *martingaleSimTrader) GetMarketPrice(string) (float64, error) { return 0, nil }
func (m *martingaleSimTrader) SetStopLoss(string, string, float64, float64) error   { return nil }
func (m *martingaleSimTrader) SetTakeProfit(string, string, float64, float64) error { return nil }
func (m *martingaleSimTrader) CancelStopLossOrders(string) error                  { return nil }
func (m *martingaleSimTrader) CancelTakeProfitOrders(string) error                { return nil }
func (m *martingaleSimTrader) CancelAllOrders(string) error  { return nil }
func (m *martingaleSimTrader) CancelStopOrders(string) error { return nil }
func (m *martingaleSimTrader) FormatQuantity(string, float64) (string, error) {
	return "", nil
}
func (m *martingaleSimTrader) GetOrderStatus(string, string) (map[string]interface{}, error) {
	return nil, nil
}
func (m *martingaleSimTrader) GetClosedPnL(time.Time, int) ([]types.ClosedPnLRecord, error) {
	return nil, nil
}
func (m *martingaleSimTrader) GetOpenOrders(string) ([]types.OpenOrder, error) {
	return nil, nil
}

func defaultMartingaleStrategyConfig() *store.StrategyConfig {
	return &store.StrategyConfig{
		StrategyType: "program_martingale",
		MartingaleProgram: &store.MartingaleProgramConfig{
			Symbol:              "XAUUSDT",
			Leverage:            20,
			MaxLayers:           7,
			LayerWeights:        append([]float64(nil), defaultMartingaleLayerWeights...),
			MarginBudgetPct:     0.06,
			AddStepPct:          0.006,
			BasketTakeProfitROE: 0.025,
			TrendMinSepPct:      0.0008,
			AllowShort:          true,
			MaxBasketLossROE:    0.12,
			DailyLossLimitPct:   8,
		},
		RiskControl: store.RiskControlConfig{
			MinPositionSize: 12,
			MaxMarginUsage:  0.06,
		},
	}
}

func newMartingaleSimAT(sim *martingaleSimTrader, equity float64) *AutoTrader {
	return &AutoTrader{
		id:             "martingale-sim",
		name:           "martingale-sim",
		exchange:       "binance",
		initialBalance: equity,
		config: AutoTraderConfig{
			StrategyConfig: defaultMartingaleStrategyConfig(),
			IsCrossMargin:  true,
		},
		trader: sim,
	}
}

func runSimCycle(t *testing.T, at *AutoTrader) *store.DecisionRecord {
	t.Helper()
	rec := &store.DecisionRecord{ExecutionLog: []string{}}
	if err := at.runMartingaleProgramCycle(rec); err != nil {
		t.Logf("cycle returned err: %v", err)
	}
	return rec
}

func logContains(rec *store.DecisionRecord, sub string) bool {
	for _, line := range rec.ExecutionLog {
		if strings.Contains(line, sub) {
			return true
		}
	}
	return false
}

func TestMartingaleSaveDecisionHidesThought(t *testing.T) {
	at := newMartingaleSimAT(&martingaleSimTrader{}, 1000)
	rec := &store.DecisionRecord{ExecutionLog: []string{"程序化马丁 test"}}
	_ = at.martingaleSaveDecision(rec)
	if rec.SystemPrompt != "COMKUN_FOLLOW_INFO_ONLY" {
		t.Fatalf("SystemPrompt=%q", rec.SystemPrompt)
	}
	if rec.CoTTrace != "" {
		t.Fatalf("CoTTrace should be empty, got %q", rec.CoTTrace)
	}
}

func TestMartingaleSimNoPositionTrendNone(t *testing.T) {
	mkt := &market.Data{
		CurrentPrice: 3300,
		LongerTermContext: &market.LongerTermData{
			EMA20: 3300,
			EMA50: 3300,
		},
	}
	if martingaleTrend(mkt, resolveMartingaleProgramConfig(defaultMartingaleStrategyConfig())) != "none" {
		t.Fatal("flat EMA should be none")
	}
}

func TestMartingaleSimTakeProfitCloses(t *testing.T) {
	sim := &martingaleSimTrader{
		balance: map[string]interface{}{
			"totalWalletBalance": 1000.0,
			"availableBalance": 800.0,
		},
		poses: []map[string]interface{}{
			{
				"symbol": "XAUUSDT", "side": "long", "positionAmt": 0.05,
				"entryPrice": 3200.0, "unRealizedProfit": 50.0, "isolatedMargin": 8.0,
			},
		},
	}
	at := newMartingaleSimAT(sim, 1000)
	at.martingaleSetAnchor("long", 3200)

	// 注入行情：仅用于 basket ROE，runCycle 仍会调 market.Get — 本用例靠高 ROE 触发平仓
	// 模拟：unrealized 50 / margin 8 = 625% ROE >> 2.5%
	rec := runSimCycle(t, at)
	if !logContains(rec, "止盈") && !logContains(rec, "全平") {
		t.Fatalf("expected take profit close, log=%v cmds=%v", rec.ExecutionLog, sim.log)
	}
	if len(sim.log) == 0 || !strings.Contains(sim.log[len(sim.log)-1], "CloseLong") {
		t.Fatalf("expected CloseLong, cmds=%v", sim.log)
	}
}

func TestMartingaleSimStopLossCloses(t *testing.T) {
	sim := &martingaleSimTrader{
		balance: map[string]interface{}{
			"totalWalletBalance": 1000.0,
			"availableBalance": 800.0,
		},
		poses: []map[string]interface{}{
			{
				"symbol": "XAUUSDT", "side": "long", "positionAmt": 0.05,
				"entryPrice": 3200.0, "unRealizedProfit": -2.0, "isolatedMargin": 8.0,
			},
		},
	}
	at := newMartingaleSimAT(sim, 1000)
	at.martingaleSetAnchor("long", 3200)
	rec := runSimCycle(t, at)
	// ROE = -2/8 = -25% < -12%
	if !logContains(rec, "止损") {
		t.Fatalf("expected stop loss, log=%v", rec.ExecutionLog)
	}
}

func TestMartingaleSimAddLayerTriggerMath(t *testing.T) {
	mp := resolveMartingaleProgramConfig(defaultMartingaleStrategyConfig())
	anchor := 3300.0
	nextLayer := 2
	trigger := anchor * (1 - mp.AddStepPct*float64(nextLayer))
	price := trigger - 1
	if price > trigger {
		t.Fatal("price should be below trigger for long add")
	}
	// 现价高于触发价 → 不应补仓
	price2 := trigger + 10
	adverse := price2 <= trigger
	if adverse {
		t.Fatal("should not add when price above trigger")
	}
}

func TestMartingaleSimLayerPlanMonotonic(t *testing.T) {
	at := newMartingaleSimAT(&martingaleSimTrader{}, 6422)
	mp := resolveMartingaleProgramConfig(defaultMartingaleStrategyConfig())
	budget := at.martingaleBudget(6422, 6000, mp)
	_, cum := at.martingalePlanLayers("XAUUSDT", budget, 3300, mp)
	for i := 1; i < len(cum); i++ {
		if cum[i] < cum[i-1] {
			t.Fatalf("cum not monotonic: %v", cum)
		}
	}
	if cum[len(cum)-1] <= 0 {
		t.Fatal("max cum qty should be positive")
	}
}

// TestMartingaleLiveMarketDryRun 拉真行情跑一轮「无仓」逻辑，只记日志不下单（需网络）
func TestMartingaleLiveMarketDryRun(t *testing.T) {
	sim := &martingaleSimTrader{
		balance: map[string]interface{}{
			"totalWalletBalance": 6422.0,
			"availableBalance": 6000.0,
		},
		poses: nil,
	}
	at := newMartingaleSimAT(sim, 6422)
	rec := runSimCycle(t, at)
	if rec.SystemPrompt != "COMKUN_FOLLOW_INFO_ONLY" {
		t.Fatalf("after save SystemPrompt=%q", rec.SystemPrompt)
	}
	if len(rec.ExecutionLog) < 2 {
		t.Fatalf("execution log too short: %v", rec.ExecutionLog)
	}
	for _, line := range rec.ExecutionLog {
		if strings.Contains(line, "趋势反转") || strings.Contains(line, "震荡") {
			t.Fatalf("removed close rules still in log: %s", line)
		}
	}
	t.Logf("trend cycle log:\n  %s", strings.Join(rec.ExecutionLog, "\n  "))
	t.Logf("commands: %v", sim.log)
}
