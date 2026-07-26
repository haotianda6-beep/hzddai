package trader

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"nofx/logger"
	"nofx/market"
	"nofx/store"
	"nofx/trader/binance"
)

// 默认 1～7 手相对权重（MT5 序列 0.7…11.31 归一化，任意权益按占比拆保证金）
var defaultMartingaleLayerWeights = []float64{
	0.0298, 0.0472, 0.0754, 0.1197, 0.1904, 0.3030, 0.4845,
}

func (at *AutoTrader) IsMartingaleProgramStrategy() bool {
	return at.config.StrategyConfig != nil && store.IsComkunProgramMartingaleStrategy(at.config.StrategyConfig)
}

func resolveMartingaleProgramConfig(cfg *store.StrategyConfig) *store.MartingaleProgramConfig {
	if cfg == nil || cfg.MartingaleProgram == nil {
		return nil
	}
	mp := *cfg.MartingaleProgram
	if strings.TrimSpace(mp.Symbol) == "" {
		mp.Symbol = "XAUUSDT"
	}
	if mp.Leverage < 1 {
		mp.Leverage = 20
	}
	if mp.Leverage > 125 {
		mp.Leverage = 125
	}
	if mp.MaxLayers < 1 {
		mp.MaxLayers = 7
	}
	if mp.MaxLayers > 12 {
		mp.MaxLayers = 12
	}
	if len(mp.LayerWeights) == 0 {
		n := mp.MaxLayers
		if n > len(defaultMartingaleLayerWeights) {
			n = len(defaultMartingaleLayerWeights)
		}
		mp.LayerWeights = append([]float64(nil), defaultMartingaleLayerWeights[:n]...)
	}
	if mp.MarginBudgetPct <= 0 || mp.MarginBudgetPct > 1 {
		mp.MarginBudgetPct = 0.06
	}
	if mp.AddStepPct <= 0 {
		mp.AddStepPct = 0.006
	}
	if mp.BasketTakeProfitROE <= 0 {
		mp.BasketTakeProfitROE = 0.025
	}
	if mp.TrendMinSepPct <= 0 {
		mp.TrendMinSepPct = 0.0008
	}
	if mp.MaxBasketLossROE <= 0 {
		mp.MaxBasketLossROE = 0.12
	}
	if mp.DailyLossLimitPct <= 0 {
		mp.DailyLossLimitPct = 8
	}
	if mp.MinLayerMarginUSDT <= 0 {
		mp.MinLayerMarginUSDT = 0.46
	}
	return &mp
}

func normalizeMartingaleWeights(w []float64, maxLayers int) []float64 {
	if maxLayers < 1 {
		maxLayers = 7
	}
	out := make([]float64, maxLayers)
	for i := 0; i < maxLayers; i++ {
		if i < len(w) && w[i] > 0 {
			out[i] = w[i]
		} else if i < len(defaultMartingaleLayerWeights) {
			out[i] = defaultMartingaleLayerWeights[i]
		} else {
			out[i] = defaultMartingaleLayerWeights[len(defaultMartingaleLayerWeights)-1]
		}
	}
	sum := 0.0
	for _, v := range out {
		sum += v
	}
	if sum <= 0 {
		copy(out, defaultMartingaleLayerWeights[:maxLayers])
		sum = 0
		for _, v := range out {
			sum += v
		}
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

// martingaleOrderMarginUSDT 单笔初始保证金 ≈ 名义/杠杆
func martingaleOrderMarginUSDT(qty, price float64, leverage int) float64 {
	if qty <= 0 || price <= 0 || leverage < 1 {
		return 0
	}
	return qty * price / float64(leverage)
}

func (at *AutoTrader) martingaleMinLayerMarginUSDT(mp *store.MartingaleProgramConfig) float64 {
	if mp != nil && mp.MinLayerMarginUSDT > 0 {
		return mp.MinLayerMarginUSDT
	}
	return 0.46
}

func martingaleLayerQtys(budget, price float64, mp *store.MartingaleProgramConfig) []float64 {
	weights := normalizeMartingaleWeights(mp.LayerWeights, mp.MaxLayers)
	lev := float64(mp.Leverage)
	qtys := make([]float64, len(weights))
	for i, w := range weights {
		margin := budget * w
		if price > 0 && lev > 0 {
			qtys[i] = margin * lev / price
		}
	}
	return qtys
}

func martingaleCumulativeQtys(layerQtys []float64) []float64 {
	cum := make([]float64, len(layerQtys))
	s := 0.0
	for i, q := range layerQtys {
		s += q
		cum[i] = s
	}
	return cum
}

func martingaleTrend(mkt *market.Data, mp *store.MartingaleProgramConfig) string {
	if mkt == nil || mkt.LongerTermContext == nil {
		return "none"
	}
	lt := mkt.LongerTermContext
	if lt.EMA20 <= 0 || lt.EMA50 <= 0 {
		return "none"
	}
	sep := (lt.EMA20 - lt.EMA50) / lt.EMA50
	if sep >= mp.TrendMinSepPct {
		return "long"
	}
	if sep <= -mp.TrendMinSepPct {
		if mp.AllowShort {
			return "short"
		}
		return "none"
	}
	return "none"
}

type martingalePositionView struct {
	side       string
	qty        float64
	entry      float64
	unrealized float64
	margin     float64
}

func (at *AutoTrader) findMartingalePosition(symbol string) (*martingalePositionView, error) {
	positions, err := at.trader.GetPositions()
	if err != nil {
		return nil, err
	}
	sym := market.Normalize(symbol)
	for _, p := range positions {
		s, _ := p["symbol"].(string)
		if market.Normalize(s) != sym {
			continue
		}
		side, _ := p["side"].(string)
		amt, _ := p["positionAmt"].(float64)
		if amt == 0 {
			if a2, ok := p["quantity"].(float64); ok {
				amt = a2
			}
		}
		if math.Abs(amt) < 1e-12 {
			continue
		}
		entry, _ := p["entryPrice"].(float64)
		upnl, _ := p["unRealizedProfit"].(float64)
		margin, _ := p["isolatedMargin"].(float64)
		if margin <= 0 {
			if m2, ok := p["margin"].(float64); ok {
				margin = m2
			}
		}
		if strings.EqualFold(side, "long") || amt > 0 {
			return &martingalePositionView{side: "long", qty: math.Abs(amt), entry: entry, unrealized: upnl, margin: margin}, nil
		}
		return &martingalePositionView{side: "short", qty: math.Abs(amt), entry: entry, unrealized: upnl, margin: margin}, nil
	}
	return nil, nil
}

func (at *AutoTrader) martingaleFormatQty(symbol string, qty float64) float64 {
	if qty <= 0 {
		return 0
	}
	if ft, ok := at.trader.(*binance.FuturesTrader); ok {
		if s, err := ft.FormatQuantity(symbol, qty); err == nil {
			if q, err := parseFloat64(s); err == nil && q > 0 {
				return q
			}
		}
	}
	return qty
}

func parseFloat64(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

func (at *AutoTrader) martingaleClearAnchor() {
	at.martingaleMu.Lock()
	at.martingaleAnchorPrice = 0
	at.martingaleSide = ""
	at.martingaleMu.Unlock()
	at.martingaleSavePersistedState()
}

func (at *AutoTrader) martingaleSetAnchor(side string, price float64) {
	at.martingaleMu.Lock()
	at.martingaleSide = side
	at.martingaleAnchorPrice = price
	at.martingaleMu.Unlock()
	at.martingaleSavePersistedState()
}

func (at *AutoTrader) martingaleBudget(equity, avail float64, mp *store.MartingaleProgramConfig) float64 {
	base := equity
	if mp.BudgetUseAvailableOnly && avail > 0 {
		base = avail
	}
	budget := base * mp.MarginBudgetPct
	if at.config.StrategyConfig != nil {
		rc := at.config.StrategyConfig.RiskControl
		if rc.MaxMarginUsage > 0 && rc.MaxMarginUsage <= 1 {
			cap := equity * rc.MaxMarginUsage
			if budget > cap {
				budget = cap
			}
		}
	}
	return budget
}

func (at *AutoTrader) martingalePlanLayers(sym string, budget, price float64, mp *store.MartingaleProgramConfig) (layerQtys, cumQtys []float64) {
	raw := martingaleLayerQtys(budget, price, mp)
	layerQtys = make([]float64, len(raw))
	cumQtys = make([]float64, len(raw))
	prev := 0.0
	rawCum := 0.0
	for i, r := range raw {
		rawCum += r
		target := at.martingaleFormatQty(sym, rawCum)
		layerQtys[i] = target - prev
		if layerQtys[i] < 0 {
			layerQtys[i] = 0
		}
		cumQtys[i] = target
		prev = target
	}
	return layerQtys, cumQtys
}

func (at *AutoTrader) martingaleBasketROE(pos *martingalePositionView, price float64, leverage int) float64 {
	if pos == nil {
		return 0
	}
	usedMargin := pos.margin
	if usedMargin <= 0 && pos.qty > 0 && price > 0 && leverage > 0 {
		usedMargin = pos.qty * price / float64(leverage)
	}
	if usedMargin <= 0 {
		return 0
	}
	return pos.unrealized / usedMargin
}

func (at *AutoTrader) martingaleDailyLossCheck(equity float64, mp *store.MartingaleProgramConfig) (exceeded bool, lossPct float64) {
	now := time.Now()
	at.martingaleMu.Lock()
	if at.martingaleDayReset.IsZero() ||
		now.YearDay() != at.martingaleDayReset.YearDay() ||
		now.Year() != at.martingaleDayReset.Year() {
		at.martingaleDayStartEquity = equity
		at.martingaleDayReset = now
		at.martingaleDailyPaused = false
		at.martingaleMu.Unlock()
		at.martingaleSavePersistedState()
	} else {
		at.martingaleMu.Unlock()
	}
	if mp.DailyLossLimitPct <= 0 {
		return false, 0
	}
	at.martingaleMu.Lock()
	start := at.martingaleDayStartEquity
	at.martingaleMu.Unlock()
	if start <= 0 {
		return false, 0
	}
	if equity >= start {
		return false, 0
	}
	lossPct = (start - equity) / start * 100
	return lossPct >= mp.DailyLossLimitPct, lossPct
}

func (at *AutoTrader) martingaleIsDailyPaused() bool {
	at.martingaleMu.Lock()
	defer at.martingaleMu.Unlock()
	return at.martingaleDailyPaused
}

func (at *AutoTrader) martingaleSetDailyPaused(v bool) {
	at.martingaleMu.Lock()
	at.martingaleDailyPaused = v
	at.martingaleMu.Unlock()
	at.martingaleSavePersistedState()
}

func (at *AutoTrader) martingaleCloseAll(sym string, pos *martingalePositionView, record *store.DecisionRecord, reason string) error {
	var closeErr error
	if pos.side == "long" {
		_, closeErr = at.trader.CloseLong(sym, 0)
	} else {
		_, closeErr = at.trader.CloseShort(sym, 0)
	}
	at.martingaleClearAnchor()
	if closeErr != nil {
		record.ExecutionLog = append(record.ExecutionLog, "❌ "+reason+" 平仓失败: "+closeErr.Error())
		record.Success = false
		record.ErrorMessage = closeErr.Error()
	} else {
		record.ExecutionLog = append(record.ExecutionLog, "✓ "+reason+"，已全平")
		record.Success = true
	}
	at.martingaleSaveDecision(record)
	return closeErr
}

// martingaleSaveDecision 看板与跟单一致：不公开思考过程（仅内部 execution_log 留档）
func (at *AutoTrader) martingaleSaveDecision(record *store.DecisionRecord) error {
	if record != nil {
		record.SystemPrompt = "COMKUN_FOLLOW_INFO_ONLY"
		record.CoTTrace = ""
		record.RawResponse = ""
	}
	return at.saveDecision(record)
}

func (at *AutoTrader) martingaleGetAnchor() (side string, price float64) {
	at.martingaleMu.Lock()
	defer at.martingaleMu.Unlock()
	return at.martingaleSide, at.martingaleAnchorPrice
}

func inferMartingaleLayer(absQty float64, cumQtys []float64) int {
	layer := -1
	for i, c := range cumQtys {
		if absQty+1e-9 >= c {
			layer = i
		}
	}
	return layer
}

func (at *AutoTrader) runMartingaleProgramCycle(record *store.DecisionRecord) error {
	cfg := at.config.StrategyConfig
	mp := resolveMartingaleProgramConfig(cfg)
	if mp == nil {
		record.Success = false
		record.ErrorMessage = "martingale_program 配置缺失"
		at.martingaleSaveDecision(record)
		return fmt.Errorf("martingale_program config missing")
	}

	sym := market.Normalize(mp.Symbol)
	at.martingaleLoadPersistedState()
	record.ExecutionLog = append(record.ExecutionLog, "程序化马丁(COMKUN-AI·无LLM) 周期开始")

	bal, err := at.trader.GetBalance()
	if err != nil {
		record.Success = false
		record.ErrorMessage = err.Error()
		at.martingaleSaveDecision(record)
		return err
	}
	equity := extractFloat(bal, "totalWalletBalance", "totalEquity", "equity")
	if equity <= 0 {
		equity = extractFloat(bal, "availableBalance")
	}
	if equity <= 0 {
		record.Success = false
		record.ErrorMessage = "无法读取账户净值"
		at.martingaleSaveDecision(record)
		return fmt.Errorf("equity unavailable")
	}
	avail := extractFloat(bal, "availableBalance")
	record.AccountState = store.AccountSnapshot{
		TotalBalance:     equity,
		AvailableBalance: avail,
		InitialBalance:   at.initialBalance,
	}

	exchange := strings.TrimSpace(at.exchange)
	if exchange == "" {
		exchange = "binance"
	}
	mkt, err := market.GetWithExchange(sym, exchange)
	if err != nil {
		record.Success = false
		record.ErrorMessage = err.Error()
		at.martingaleSaveDecision(record)
		return err
	}
	price := mkt.CurrentPrice
	trend := martingaleTrend(mkt, mp)
	budget := at.martingaleBudget(equity, avail, mp)
	_, cumQtys := at.martingalePlanLayers(sym, budget, price, mp)

	dailyExceeded, dailyLossPct := at.martingaleDailyLossCheck(equity, mp)
	if dailyExceeded {
		at.martingaleSetDailyPaused(true)
		record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
			"日亏熔断 %.2f%% >= %.2f%%，暂停开新仓", dailyLossPct, mp.DailyLossLimitPct))
	}

	record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
		"大趋势(4h EMA20/50): %s | 现价 %.2f | 净值 %.2f | 马丁预算 %.2f", trend, price, equity, budget))

	pos, err := at.findMartingalePosition(sym)
	if err != nil {
		record.Success = false
		record.ErrorMessage = err.Error()
		at.martingaleSaveDecision(record)
		return err
	}

	// 无仓：按大趋势开第 1 层
	if pos == nil {
		at.martingaleClearAnchor()
		if at.martingaleIsDailyPaused() {
			record.ExecutionLog = append(record.ExecutionLog, "日亏熔断中，不开新仓")
			record.Success = true
			at.martingaleSaveDecision(record)
			return nil
		}
		if trend == "none" {
			record.ExecutionLog = append(record.ExecutionLog, "无明确大趋势，本轮不开仓")
			record.Success = true
			at.martingaleSaveDecision(record)
			return nil
		}
		minMargin := at.martingaleMinLayerMarginUSDT(mp)
		layerQtys, _ := at.martingalePlanLayers(sym, budget, price, mp)
		q0 := layerQtys[0]
		m0 := martingaleOrderMarginUSDT(q0, price, mp.Leverage)
		if m0 < minMargin {
			record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
				"首层保证金 %.2f < %.2fU（20x 名义≈%.2fU），跳过", m0, minMargin, m0*float64(mp.Leverage)))
			record.Success = true
			at.martingaleSaveDecision(record)
			return nil
		}
		if err := at.trader.SetLeverage(sym, mp.Leverage); err != nil {
			logger.Infof("[%s] martingale SetLeverage: %v", at.name, err)
		}
		_ = at.trader.SetMarginMode(sym, at.config.IsCrossMargin)
		var openErr error
		if trend == "long" {
			_, openErr = at.trader.OpenLong(sym, q0, mp.Leverage)
		} else {
			_, openErr = at.trader.OpenShort(sym, q0, mp.Leverage)
		}
		if openErr != nil {
			record.ExecutionLog = append(record.ExecutionLog, "❌ 首层开仓失败: "+openErr.Error())
			record.Success = false
			record.ErrorMessage = openErr.Error()
		} else {
			anchorPrice := price
			if p2, _ := at.findMartingalePosition(sym); p2 != nil && p2.entry > 0 {
				anchorPrice = p2.entry
			}
			at.martingaleSetAnchor(trend, anchorPrice)
			record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
				"✓ 首层 %s %s qty=%.6f anchor=%.2f (层1/%.0f)", trend, sym, q0, anchorPrice, float64(len(layerQtys))))
			record.Success = true
		}
		at.martingaleSaveDecision(record)
		return openErr
	}

	// 有仓：风控平仓优先
	anchorSide, anchorPrice := at.martingaleGetAnchor()
	if anchorSide == "" {
		anchorSide = pos.side
		anchorPrice = pos.entry
		at.martingaleSetAnchor(anchorSide, anchorPrice)
	}

	basketROE := at.martingaleBasketROE(pos, price, mp.Leverage)
	if mp.MaxBasketLossROE > 0 && basketROE <= -mp.MaxBasketLossROE {
		return at.martingaleCloseAll(sym, pos, record, fmt.Sprintf(
			"止损 ROE %.2f%% <= -%.2f%%", basketROE*100, mp.MaxBasketLossROE*100))
	}

	if basketROE >= mp.BasketTakeProfitROE {
		return at.martingaleCloseAll(sym, pos, record, fmt.Sprintf(
			"basket 止盈 ROE %.2f%% >= %.2f%%", basketROE*100, mp.BasketTakeProfitROE*100))
	}

	layer := inferMartingaleLayer(pos.qty, cumQtys)
	record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
		"持仓 %s qty=%.6f 层位=%d/%d anchor=%.2f", pos.side, pos.qty, layer+1, len(cumQtys), anchorPrice))

	if layer >= len(cumQtys)-1 {
		record.ExecutionLog = append(record.ExecutionLog, "已达最大层，本轮仅持仓")
		record.Success = true
		at.martingaleSaveDecision(record)
		return nil
	}

	nextLayer := layer + 1
	targetCum := cumQtys[nextLayer]
	addQty := targetCum - pos.qty
	if addQty <= 0 {
		record.ExecutionLog = append(record.ExecutionLog, "已满足目标层仓位，无需补仓")
		record.Success = true
		at.martingaleSaveDecision(record)
		return nil
	}
	addQty = at.martingaleFormatQty(sym, addQty)
	minMargin := at.martingaleMinLayerMarginUSDT(mp)
	addM := martingaleOrderMarginUSDT(addQty, price, mp.Leverage)
	if addM < minMargin {
		record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
			"补仓保证金 %.2f < %.2fU（名义≈%.2fU），跳过", addM, minMargin, addM*float64(mp.Leverage)))
		record.Success = true
		at.martingaleSaveDecision(record)
		return nil
	}

	// 逆势达到间距才补下一层
	adverse := false
	if pos.side == "long" {
		trigger := anchorPrice * (1 - mp.AddStepPct*float64(nextLayer))
		adverse = price <= trigger
		record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
			"多单补仓触发价 %.2f (现价 %.2f)", trigger, price))
	} else {
		trigger := anchorPrice * (1 + mp.AddStepPct*float64(nextLayer))
		adverse = price >= trigger
		record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
			"空单补仓触发价 %.2f (现价 %.2f)", trigger, price))
	}
	if !adverse {
		record.ExecutionLog = append(record.ExecutionLog, "未达补仓间距，持有")
		record.Success = true
		at.martingaleSaveDecision(record)
		return nil
	}

	if err := at.trader.SetLeverage(sym, mp.Leverage); err != nil {
		logger.Infof("[%s] martingale SetLeverage: %v", at.name, err)
	}
	var addErr error
	if pos.side == "long" {
		_, addErr = at.trader.OpenLong(sym, addQty, mp.Leverage)
	} else {
		_, addErr = at.trader.OpenShort(sym, addQty, mp.Leverage)
	}
	if addErr != nil {
		record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
			"❌ 第%d层补仓失败: %v", nextLayer+1, addErr))
		record.Success = false
		record.ErrorMessage = addErr.Error()
	} else {
		record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
			"✓ 第%d层补仓 %s +%.6f (累计目标 %.6f)", nextLayer+1, pos.side, addQty, targetCum))
		record.Success = true
	}
	at.martingaleSaveDecision(record)
	return addErr
}

func extractFloat(m map[string]interface{}, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			switch x := v.(type) {
			case float64:
				return x
			case float32:
				return float64(x)
			case int:
				return float64(x)
			case int64:
				return float64(x)
			case string:
				f, _ := strconv.ParseFloat(x, 64)
				return f
			}
		}
	}
	return 0
}
