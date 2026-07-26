package trader

import (
	"strings"

	"nofx/kernel"
	"nofx/store"
)

const comkunSolManualWaitReason = "【SOL 专项扫描】当前未检测到 SOLUSDT 合约持仓：本轮仅做 SOL 行情与趋势扫描，不自动进场。"

func hasSOLUSDTPosition(ctx *kernel.Context) bool {
	if ctx == nil {
		return false
	}
	for _, p := range ctx.Positions {
		if strings.EqualFold(strings.TrimSpace(p.Symbol), "SOLUSDT") && p.Quantity > 1e-12 {
			return true
		}
	}
	return false
}

// applyComkunListingSolManualBroadcastMode 主控「SOL + 人工开仓」广播闸门：在写入 comkun 广播前规范化决策 JSON。
func applyComkunListingSolManualBroadcastMode(cfg *store.StrategyConfig, ctx *kernel.Context, in []kernel.Decision) []kernel.Decision {
	if cfg == nil || !cfg.ComkunFollowListingTemplate || !cfg.ComkunListingMasterSolManualBroadcastMode {
		return in
	}
	if store.IsComkunMarketFollowStrategy(cfg) {
		return in
	}
	if !hasSOLUSDTPosition(ctx) {
		return []kernel.Decision{{
			Symbol:     "SOLUSDT",
			Action:     "wait",
			Confidence: 100,
			Reasoning:  comkunSolManualWaitReason,
		}}
	}
	out := make([]kernel.Decision, 0, len(in))
	for _, d := range in {
		a := strings.ToLower(strings.TrimSpace(d.Action))
		if a == "open_long" || a == "open_short" {
			continue
		}
		sym := strings.TrimSpace(d.Symbol)
		if sym != "" && !strings.EqualFold(sym, "SOLUSDT") {
			continue
		}
		out = append(out, d)
	}
	return out
}
