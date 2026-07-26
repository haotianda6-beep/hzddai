package news

import (
	"fmt"
	"strings"
)

func applyGoldMarketConfirmation(impact GoldImpact, market *GoldMarketStatus) GoldImpact {
	if impact.Confirmation != "pending" || market == nil || market.Status != "live" || market.XAU == nil || market.Dollar == nil {
		return impact
	}
	xau, dollar, window, ok := confirmationWindow(market)
	if !ok {
		return impact
	}
	baseReason := strings.TrimSuffix(impact.Reason, " 等待同步行情确认。")
	context := fmt.Sprintf(" MT4同步行情%s：黄金%+.3f%%，美元指数%+.3f%%", window, xau, dollar)
	if market.US10Y.ChangeBP != nil {
		context += fmt.Sprintf("；10年期美债最近交易日%+.1fbp", *market.US10Y.ChangeBP)
	}
	context += "。"

	confirmed, divergent := false, false
	switch impact.Direction {
	case "bullish":
		confirmed = xau >= 0.03 && dollar <= -0.015
		divergent = xau <= -0.03 && dollar >= 0.015
	case "bearish":
		confirmed = xau <= -0.03 && dollar >= 0.015
		divergent = xau >= 0.03 && dollar <= -0.015
	case "neutral":
		if xau >= 0.05 && dollar <= -0.02 {
			impact.Direction, confirmed = "bullish", true
		} else if xau <= -0.05 && dollar >= 0.02 {
			impact.Direction, confirmed = "bearish", true
		}
	}

	if confirmed {
		impact.Confirmation = "confirmed"
		impact.RiskScore += 25
		if impact.RiskScore > 100 {
			impact.RiskScore = 100
		}
		impact.RiskLevel = goldRiskLevel(impact.RiskScore)
		impact.Reason = baseReason + context + " 黄金与美元指数同向验证该判断。"
		impact.Drivers = appendGoldDriver(impact.Drivers, "同步行情确认")
	} else if divergent {
		impact.Confirmation = "divergent"
		impact.Reason = baseReason + context + " 实际行情与新闻判断相反，暂不追单。"
	} else {
		impact.Reason = baseReason + context + " 当前波动尚未形成双因子确认。"
	}
	return impact
}

func confirmationWindow(market *GoldMarketStatus) (float64, float64, string, bool) {
	if market.XAU.Change15M != nil && market.Dollar.Change15M != nil {
		return *market.XAU.Change15M, *market.Dollar.Change15M, "15分钟", true
	}
	if market.XAU.Change5M != nil && market.Dollar.Change5M != nil {
		return *market.XAU.Change5M, *market.Dollar.Change5M, "5分钟", true
	}
	return 0, 0, "", false
}

func appendGoldDriver(drivers []string, value string) []string {
	for _, driver := range drivers {
		if driver == value {
			return drivers
		}
	}
	if len(drivers) >= 4 {
		return drivers
	}
	return append(drivers, value)
}
