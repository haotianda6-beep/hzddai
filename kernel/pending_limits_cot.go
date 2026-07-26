package kernel

import (
	"fmt"
	"strings"

	"nofx/market"
)

// IsLimitLikePendingOrderType 是否为「限价类」挂单（供提示词、思维链、二次解读一致判断）。
// 含 LIMIT / LIMIT_MAKER，以及币安常见的 STOP_LIMIT、TAKE_PROFIT_LIMIT 等。
func IsLimitLikePendingOrderType(typ string) bool {
	t := strings.ToUpper(strings.TrimSpace(typ))
	switch {
	case t == "LIMIT", t == "LIMIT_MAKER":
		return true
	default:
		return (strings.Contains(t, "STOP") && strings.Contains(t, "LIMIT")) ||
			(strings.Contains(t, "TAKE_PROFIT") && strings.Contains(t, "LIMIT"))
	}
}

// PendingLimitDisplayPrice 展示与名义金额用价格：普通限价用 price；部分条件限价主价位在 stop_price
func PendingLimitDisplayPrice(o PendingOrder) float64 {
	if o.Price > 0 {
		return o.Price
	}
	if IsLimitLikePendingOrderType(o.Type) && o.StopPrice > 0 {
		return o.StopPrice
	}
	return 0
}

// HasLimitStylePending 是否存在限价类挂单（供提示词与思维链增强判断）
func HasLimitStylePending(orders []PendingOrder) bool {
	for _, o := range orders {
		if !IsLimitLikePendingOrderType(o.Type) {
			continue
		}
		px := PendingLimitDisplayPrice(o)
		if strings.TrimSpace(o.Symbol) != "" && px > 0 && o.Quantity > 0 {
			return true
		}
	}
	return false
}

// PendingOrderTriggerPrice 条件单触发价（市价止盈止损、追踪止损等常用 stop_price；数量可能为 0 仍有触发价）
func PendingOrderTriggerPrice(o PendingOrder) float64 {
	if o.StopPrice > 0 {
		return o.StopPrice
	}
	if o.Price > 0 {
		return o.Price
	}
	return 0
}

// HasConditionalTPSLPending 是否存在止盈/止损类条件单（无限价时原先思维链摘要整段被跳过）
func HasConditionalTPSLPending(orders []PendingOrder) bool {
	for _, o := range orders {
		if strings.TrimSpace(o.Symbol) == "" {
			continue
		}
		tr := PendingOrderTriggerPrice(o)
		if tr <= 0 {
			continue
		}
		if IsConditionalTakeProfitOrderType(o.Type) || IsConditionalStopLossOrderType(o.Type) {
			return true
		}
	}
	return false
}

// InferredPositionSideForLimit 从限价单推断持仓方向（与镜像跟单逻辑一致）
func InferredPositionSideForLimit(o PendingOrder) string {
	ps := strings.ToUpper(strings.TrimSpace(o.PositionSide))
	if ps != "" && ps != "BOTH" {
		return ps
	}
	if strings.ToUpper(strings.TrimSpace(o.Side)) == "SELL" {
		return "SHORT"
	}
	return "LONG"
}

// IsConditionalTakeProfitOrderType 交易所条件单中的「止盈」类（含市价止盈、限价止盈等）
func IsConditionalTakeProfitOrderType(typ string) bool {
	t := strings.ToUpper(strings.TrimSpace(typ))
	if t == "" {
		return false
	}
	return strings.Contains(t, "TAKE_PROFIT")
}

// IsConditionalStopLossOrderType 交易所条件单中的「止损」类（含止损、移动止损；排除止盈）
func IsConditionalStopLossOrderType(typ string) bool {
	t := strings.ToUpper(strings.TrimSpace(typ))
	if t == "" || IsConditionalTakeProfitOrderType(typ) {
		return false
	}
	// 各所常见：STOP_MARKET / STOP / STOP_LOSS / TRAILING_STOP_MARKET 等
	return strings.Contains(t, "STOP") || strings.Contains(t, "TRAILING")
}

// EnrichPositionsSLTPFromPending 将「挂单中能解析到的」止盈/止损触发价写回各条持仓，便于写入主广播 positions；
// 被控镜像时除读 pending_orders 外可再读 positions.stop_loss / take_profit 作兜底。
func EnrichPositionsSLTPFromPending(positions []PositionInfo, pending []PendingOrder) []PositionInfo {
	if len(positions) == 0 || len(pending) == 0 {
		return positions
	}
	out := make([]PositionInfo, len(positions))
	copy(out, positions)
	for i := range out {
		side := strings.ToLower(strings.TrimSpace(out[i].Side))
		if side != "long" && side != "short" {
			continue
		}
		ps := strings.ToUpper(side)
		sl, tp := FindSLTPFromPendingOrders(pending, out[i].Symbol, ps)
		if sl > 0 {
			out[i].StopLoss = sl
		}
		if tp > 0 {
			out[i].TakeProfit = tp
		}
	}
	return out
}

// FindSLTPFromPendingOrders 从挂单列表解析某方向仓位的止盈、止损触发价（对齐币安/OKX/Bybit 等常见类型字符串）
func FindSLTPFromPendingOrders(pending []PendingOrder, symbol, posSide string) (stopLoss, takeProfit float64) {
	su := strings.ToUpper(strings.TrimSpace(symbol))
	ps := strings.ToUpper(strings.TrimSpace(posSide))
	for _, o := range pending {
		if strings.ToUpper(strings.TrimSpace(o.Symbol)) != su {
			continue
		}
		typ := strings.ToUpper(strings.TrimSpace(o.Type))
		os := strings.ToUpper(strings.TrimSpace(o.Side))
		ops := strings.ToUpper(strings.TrimSpace(o.PositionSide))
		if ops != "" && ops != "BOTH" && ops != ps {
			continue
		}
		trigger := PendingOrderTriggerPrice(o)
		if trigger <= 0 {
			continue
		}
		if ps == "LONG" {
			if os == "SELL" && IsConditionalStopLossOrderType(typ) {
				stopLoss = trigger
			}
			if os == "SELL" && IsConditionalTakeProfitOrderType(typ) {
				takeProfit = trigger
			}
		} else if ps == "SHORT" {
			if os == "BUY" && IsConditionalStopLossOrderType(typ) {
				stopLoss = trigger
			}
			if os == "BUY" && IsConditionalTakeProfitOrderType(typ) {
				takeProfit = trigger
			}
		}
	}
	return stopLoss, takeProfit
}

func isBTCETHSymbol(sym string) bool {
	u := strings.ToUpper(strings.TrimSpace(sym))
	u = strings.TrimSuffix(u, "USDT")
	u = strings.TrimSuffix(u, "USDC")
	return strings.HasPrefix(u, "BTC") || strings.HasPrefix(u, "ETH")
}

// LeverageForPendingSymbol 挂单展示用杠杆：优先同币持仓，否则策略默认 BTC/ETH 或山寨杠杆
func LeverageForPendingSymbol(ctx *Context, sym string) int {
	if ctx == nil {
		return 10
	}
	su := strings.ToUpper(strings.TrimSpace(sym))
	for _, p := range ctx.Positions {
		if strings.ToUpper(strings.TrimSpace(p.Symbol)) == su && p.Leverage > 0 {
			return p.Leverage
		}
	}
	if isBTCETHSymbol(sym) && ctx.BTCETHLeverage > 0 {
		return ctx.BTCETHLeverage
	}
	if ctx.AltcoinLeverage > 0 {
		return ctx.AltcoinLeverage
	}
	return 10
}

func currentPriceForSymbol(ctx *Context, sym string) float64 {
	if ctx == nil || ctx.MarketDataMap == nil {
		return 0
	}
	raw := strings.ToUpper(strings.TrimSpace(sym))
	if d, ok := ctx.MarketDataMap[raw]; ok && d != nil && d.CurrentPrice > 0 {
		return d.CurrentPrice
	}
	norm := market.Normalize(sym)
	if d, ok := ctx.MarketDataMap[norm]; ok && d != nil && d.CurrentPrice > 0 {
		return d.CurrentPrice
	}
	for k, d := range ctx.MarketDataMap {
		if d == nil || d.CurrentPrice <= 0 {
			continue
		}
		if strings.EqualFold(k, raw) || market.Normalize(k) == norm {
			return d.CurrentPrice
		}
	}
	return 0
}

// formatPendingConditionalTPSLSummaryZH 仅止盈/止损条件单（无限价时补一段，避免「扫描不到」）
func formatPendingConditionalTPSLSummaryZH(ctx *Context) string {
	var b strings.Builder
	b.WriteString("【止盈止损条件单（交易所）】\n")
	b.WriteString("以下为未成交的市价类止盈/止损或算法条件单（触发价来自交易所接口；数量可能为 0 表示全仓平仓触发）。\n\n")
	n := 0
	for _, o := range ctx.PendingOrders {
		if !IsConditionalTakeProfitOrderType(o.Type) && !IsConditionalStopLossOrderType(o.Type) {
			continue
		}
		tr := PendingOrderTriggerPrice(o)
		if tr <= 0 {
			continue
		}
		n++
		sym := strings.TrimSpace(o.Symbol)
		last := currentPriceForSymbol(ctx, sym)
		kind := "止损"
		if IsConditionalTakeProfitOrderType(o.Type) {
			kind = "止盈"
		}
		b.WriteString(fmt.Sprintf("%d) %s | %s | 方向 %s | 持仓侧 %s | 类型 %s | 触发价 %.8f | 数量 %.8f | 状态 %s | id:%s\n",
			n, sym, kind, strings.ToUpper(strings.TrimSpace(o.Side)), strings.TrimSpace(o.PositionSide), strings.TrimSpace(o.Type), tr, o.Quantity, o.Status, o.OrderID))
		if last > 0 {
			b.WriteString(fmt.Sprintf("   现价参考 %.8f（触发价与现价相对距离约 %.3f%%）\n", last, (tr-last)/last*100))
		}
		b.WriteString("\n")
	}
	if n == 0 {
		return ""
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatPendingConditionalTPSLSummaryEN(ctx *Context) string {
	var b strings.Builder
	b.WriteString("[TP / SL conditional orders (exchange)]\n")
	b.WriteString("Pending stop/take-profit style algo orders. Trigger from exchange; qty may be 0 for close-position triggers.\n\n")
	n := 0
	for _, o := range ctx.PendingOrders {
		if !IsConditionalTakeProfitOrderType(o.Type) && !IsConditionalStopLossOrderType(o.Type) {
			continue
		}
		tr := PendingOrderTriggerPrice(o)
		if tr <= 0 {
			continue
		}
		n++
		sym := strings.TrimSpace(o.Symbol)
		last := currentPriceForSymbol(ctx, sym)
		kind := "SL"
		if IsConditionalTakeProfitOrderType(o.Type) {
			kind = "TP"
		}
		b.WriteString(fmt.Sprintf("%d) %s | %s | side %s | posSide %s | type %s | trigger %.8f | qty %.8f | status %s | id:%s\n",
			n, sym, kind, strings.ToUpper(strings.TrimSpace(o.Side)), strings.TrimSpace(o.PositionSide), strings.TrimSpace(o.Type), tr, o.Quantity, o.Status, o.OrderID))
		if last > 0 {
			b.WriteString(fmt.Sprintf("   ref %.8f (~%.3f%% from trigger)\n", last, (tr-last)/last*100))
		}
		b.WriteString("\n")
	}
	if n == 0 {
		return ""
	}
	return strings.TrimRight(b.String(), "\n")
}

// FormatPendingLimitOrdersCoTSummary 生成写入思维链顶部的「挂单扫描摘要」（限价 + 推断；并补充「仅有止盈止损条件单」段落）
func FormatPendingLimitOrdersCoTSummary(ctx *Context, lang Language) string {
	if ctx == nil || len(ctx.PendingOrders) == 0 {
		return ""
	}
	var chunks []string

	if HasLimitStylePending(ctx.PendingOrders) {
		var b strings.Builder
		if lang == LangChinese {
			b.WriteString("【挂单扫描摘要】\n")
			b.WriteString("以下为交易所在本轮扫描中读到的未成交限价单及同交易对关联止盈/止损（从挂单列表推断）。\n\n")
		} else {
			b.WriteString("[Pending limit orders scan]\n")
			b.WriteString("Open LIMIT / LIMIT_MAKER orders and inferred TP/SL for the same symbol from the exchange snapshot.\n\n")
		}
		idx := 0
		for _, o := range ctx.PendingOrders {
			if !IsLimitLikePendingOrderType(o.Type) {
				continue
			}
			px := PendingLimitDisplayPrice(o)
			if px <= 0 || o.Quantity <= 0 {
				continue
			}
			idx++
			sym := strings.TrimSpace(o.Symbol)
			ps := InferredPositionSideForLimit(o)
			lev := LeverageForPendingSymbol(ctx, sym)
			notional := px * o.Quantity
			marginEst := 0.0
			if lev > 0 {
				marginEst = notional / float64(lev)
			}
			sl, tp := FindSLTPFromPendingOrders(ctx.PendingOrders, sym, ps)
			last := currentPriceForSymbol(ctx, sym)
			if lang == LangChinese {
				b.WriteString(fmt.Sprintf("%d) %s | 方向 %s | 持仓侧 %s | 类型 %s | 限价/主价 %.8f | 数量 %.8f | 名义约 %.4f USDT | 杠杆 %dx | 估算保证金约 %.4f USDT\n",
					idx, sym, strings.ToUpper(strings.TrimSpace(o.Side)), ps, strings.TrimSpace(o.Type), px, o.Quantity, notional, lev, marginEst))
				if last > 0 {
					b.WriteString(fmt.Sprintf("   现价参考 %.8f（与限价距离约 %.3f%%）\n", last, (px-last)/last*100))
				}
				if sl > 0 || tp > 0 {
					b.WriteString(fmt.Sprintf("   同向推断：止损触发价 %.8f | 止盈触发价 %.8f（来自当前挂单中的条件单）\n", sl, tp))
				} else {
					b.WriteString("   同向推断：未在挂单列表中匹配到典型止盈/止损条件单（可能尚未挂或类型不同）。\n")
				}
				b.WriteString("\n")
			} else {
				b.WriteString(fmt.Sprintf("%d) %s | side %s | positionSide %s | type %s | limit/ref %.8f | qty %.8f | notional ~%.4f USDT | lev %dx | est.margin ~%.4f USDT\n",
					idx, sym, strings.ToUpper(strings.TrimSpace(o.Side)), ps, strings.TrimSpace(o.Type), px, o.Quantity, notional, lev, marginEst))
				if last > 0 {
					b.WriteString(fmt.Sprintf("   ref price %.8f (~%.3f%% from limit)\n", last, (px-last)/last*100))
				}
				if sl > 0 || tp > 0 {
					b.WriteString(fmt.Sprintf("   inferred SL %.8f | TP %.8f (from pending conditional orders)\n", sl, tp))
				} else {
					b.WriteString("   inferred: no typical TP/SL conditional orders matched in the pending list.\n")
				}
				b.WriteString("\n")
			}
		}
		if idx > 0 {
			chunks = append(chunks, strings.TrimRight(b.String(), "\n"))
		}
	}

	// 仅有止盈止损、没有限价时：原先整段摘要为空，主控像「没扫到」——单独补一段
	if HasConditionalTPSLPending(ctx.PendingOrders) {
		var tpsl string
		if lang == LangChinese {
			tpsl = formatPendingConditionalTPSLSummaryZH(ctx)
		} else {
			tpsl = formatPendingConditionalTPSLSummaryEN(ctx)
		}
		if tpsl != "" {
			chunks = append(chunks, tpsl)
		}
	}

	if len(chunks) == 0 {
		return ""
	}
	return strings.Join(chunks, "\n\n")
}
