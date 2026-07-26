package trader

import (
	"encoding/json"
	"fmt"
	"strings"

	"nofx/kernel"
	"nofx/logger"
	"nofx/store"
)

// enrichDecisionCoTWithPendingLimits 在思维链前附加「挂单扫描摘要」，并在主 AI 之后追加一段简短二次解读（限价意图 + 止盈止损逻辑）
func (at *AutoTrader) enrichDecisionCoTWithPendingLimits(ctx *kernel.Context, record *store.DecisionRecord) {
	if ctx == nil || record == nil {
		return
	}
	lang := kernel.LangEnglish
	if at.strategyEngine != nil {
		lang = at.strategyEngine.GetLanguage()
	}
	summary := kernel.FormatPendingLimitOrdersCoTSummary(ctx, lang)
	if summary == "" {
		return
	}
	existing := strings.TrimSpace(record.CoTTrace)
	if existing == "" {
		record.CoTTrace = summary
	} else {
		record.CoTTrace = summary + "\n\n" + existing
	}

	if at.mcpClient == nil {
		return
	}

	type limSnap struct {
		Symbol       string  `json:"symbol"`
		Side         string  `json:"side"`
		PositionSide string  `json:"position_side"`
		Type         string  `json:"type"`
		Price        float64 `json:"price"`
		Quantity     float64 `json:"quantity"`
		NotionalUSDT float64 `json:"notional_usdt"`
		Leverage     int     `json:"leverage_ref"`
		StopLoss     float64 `json:"inferred_stop_loss"`
		TakeProfit   float64 `json:"inferred_take_profit"`
		LastPrice    float64 `json:"last_price_ref"`
	}
	var snaps []limSnap
	for _, o := range ctx.PendingOrders {
		if !kernel.IsLimitLikePendingOrderType(o.Type) {
			continue
		}
		px := kernel.PendingLimitDisplayPrice(o)
		if px <= 0 || o.Quantity <= 0 {
			continue
		}
		sym := strings.TrimSpace(o.Symbol)
		ps := kernel.InferredPositionSideForLimit(o)
		sl, tp := kernel.FindSLTPFromPendingOrders(ctx.PendingOrders, sym, ps)
		lev := kernel.LeverageForPendingSymbol(ctx, sym)
		last := 0.0
		if ctx.MarketDataMap != nil {
			if d := ctx.MarketDataMap[strings.ToUpper(sym)]; d != nil {
				last = d.CurrentPrice
			}
		}
		snaps = append(snaps, limSnap{
			Symbol: sym, Side: o.Side, PositionSide: ps, Type: o.Type,
			Price: px, Quantity: o.Quantity, NotionalUSDT: px * o.Quantity,
			Leverage: lev, StopLoss: sl, TakeProfit: tp, LastPrice: last,
		})
	}
	// 仅有止盈止损市价单、无限价时：仍给二次解读模型一份 JSON（否则 snaps 为空直接 return）
	for _, o := range ctx.PendingOrders {
		if !kernel.IsConditionalTakeProfitOrderType(o.Type) && !kernel.IsConditionalStopLossOrderType(o.Type) {
			continue
		}
		trig := o.StopPrice
		if trig <= 0 {
			trig = o.Price
		}
		if trig <= 0 {
			continue
		}
		sym := strings.TrimSpace(o.Symbol)
		ps := strings.ToUpper(strings.TrimSpace(o.PositionSide))
		if ps == "" || ps == "BOTH" {
			ps = kernel.InferredPositionSideForLimit(o)
		}
		sl, tp := kernel.FindSLTPFromPendingOrders(ctx.PendingOrders, sym, ps)
		lev := kernel.LeverageForPendingSymbol(ctx, sym)
		last := 0.0
		if ctx.MarketDataMap != nil {
			if d := ctx.MarketDataMap[strings.ToUpper(sym)]; d != nil {
				last = d.CurrentPrice
			}
		}
		snaps = append(snaps, limSnap{
			Symbol: sym, Side: o.Side, PositionSide: ps, Type: o.Type,
			Price: trig, Quantity: o.Quantity, NotionalUSDT: 0,
			Leverage: lev, StopLoss: sl, TakeProfit: tp, LastPrice: last,
		})
	}
	if len(snaps) == 0 {
		return
	}
	payload, _ := json.MarshalIndent(snaps, "", "  ")
	var sys, user string
	if lang == kernel.LangChinese {
		sys = "你是加密货币 U 本位合约交易助理。只根据给定 JSON 与数字推理，勿编造新闻或内幕。回答使用简体中文，总字数控制在 400 字以内。"
		user = fmt.Sprintf(
			"下列为当前账户在交易所的挂单摘要（JSON；可能含限价、或仅含止盈/止损条件单的触发价）：\n%s\n\n请用连贯段落回答（勿重复逐字段朗读）：\n"+
				"1) 若有限价：相对参考现价的位置与可能意图（不确定请写不确定）；\n"+
				"2) 若有触发价（止盈/止损条件单）：与现价、持仓风险边界的关系。\n"+
				"段落开头请写标题一行：【风控与价位理解】",
			string(payload),
		)
	} else {
		sys = "You are a crypto perpetual futures assistant. Reason only from the JSON. No rumors. English, max ~250 words."
		user = fmt.Sprintf(
			"Open orders snapshot (JSON; may include limits and/or TP/SL triggers only):\n%s\n\nAnswer in prose (do not reread every field). Start with one title line: [Orders & risk commentary]\n"+
				"1) If limits exist: vs ref price; 2) If triggers exist: risk vs open positions.",
			string(payload),
		)
	}
	out, err := at.mcpClient.CallWithMessages(sys, user)
	if err != nil {
		logger.Infof("pending limit rationale LLM: %v", err)
		return
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return
	}
	record.CoTTrace = strings.TrimRight(record.CoTTrace, "\n") + "\n\n" + out
}
