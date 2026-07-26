package kernel

import (
	"fmt"
	"nofx/logger"
	"strings"
)

// ============================================================================
// Decision Validation
// ============================================================================

func validateDecisions(decisions []Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int, btcEthPosRatio, altcoinPosRatio float64) error {
	for i := range decisions {
		normalizeDecisionAction(&decisions[i])
		if err := validateDecision(&decisions[i], accountEquity, btcEthLeverage, altcoinLeverage, btcEthPosRatio, altcoinPosRatio); err != nil {
			if canDowngradeDecisionToWait(decisions[i]) {
				logger.Warnf("⚠️  [Decision Fallback] decision #%d %s %s rejected (%v), converted to wait",
					i+1, decisions[i].Symbol, decisions[i].Action, err)
				convertDecisionToWait(&decisions[i], err.Error())
				continue
			}
			return fmt.Errorf("decision #%d validation failed: %w", i+1, err)
		}
	}
	return nil
}

func normalizeDecisionAction(d *Decision) {
	action, ok := canonicalDecisionAction(d.Action, d)
	if ok {
		if d.Action != action {
			logger.Infof("⚠️  [Decision Fallback] normalized action %q -> %q for %s", d.Action, action, d.Symbol)
		}
		d.Action = action
		return
	}

	raw := d.Action
	convertDecisionToWait(d, fmt.Sprintf("invalid action: %s", raw))
	logger.Warnf("⚠️  [Decision Fallback] invalid action %q for %s, converted to wait", raw, d.Symbol)
}

func canonicalDecisionAction(raw string, d *Decision) (string, bool) {
	action := strings.ToLower(strings.TrimSpace(raw))
	action = strings.ReplaceAll(action, "-", "_")
	action = strings.ReplaceAll(action, " ", "_")
	action = strings.ReplaceAll(action, "/", "_")

	switch action {
	case "open_long", "open_short", "close_long", "close_short", "hold", "wait":
		return action, true
	case "openlong":
		return "open_long", true
	case "openshort":
		return "open_short", true
	case "closelong":
		return "close_long", true
	case "closeshort":
		return "close_short", true
	case "open_new", "open", "add_position", "add":
		return inferOpenActionFromStops(d)
	case "long", "buy", "open_buy":
		return "open_long", true
	case "short", "sell", "open_sell":
		return "open_short", true
	case "full_close_long", "partial_close_long", "close_position_long":
		return "close_long", true
	case "full_close_short", "partial_close_short", "close_position_short":
		return "close_short", true
	case "full_close", "partial_close", "close", "":
		return "", false
	default:
		if strings.Contains(action, "open") && strings.Contains(action, "long") {
			return "open_long", true
		}
		if strings.Contains(action, "open") && strings.Contains(action, "short") {
			return "open_short", true
		}
		if strings.Contains(action, "close") && strings.Contains(action, "long") {
			return "close_long", true
		}
		if strings.Contains(action, "close") && strings.Contains(action, "short") {
			return "close_short", true
		}
	}

	return "", false
}

func inferOpenActionFromStops(d *Decision) (string, bool) {
	if d.StopLoss > 0 && d.TakeProfit > 0 {
		if d.StopLoss < d.TakeProfit {
			return "open_long", true
		}
		if d.StopLoss > d.TakeProfit {
			return "open_short", true
		}
	}
	return "", false
}

func canDowngradeDecisionToWait(d Decision) bool {
	return d.Action == "wait" || d.Action == "open_long" || d.Action == "open_short"
}

func convertDecisionToWait(d *Decision, reason string) {
	if strings.TrimSpace(d.Symbol) == "" {
		d.Symbol = "ALL"
	}
	d.Action = "wait"
	d.Leverage = 0
	d.PositionSizeUSD = 0
	d.StopLoss = 0
	d.TakeProfit = 0
	d.RiskUSD = 0
	d.Confidence = 0

	fallbackReason := fmt.Sprintf("Decision was converted to wait because it was not executable safely: %s", reason)
	if strings.TrimSpace(d.Reasoning) == "" {
		d.Reasoning = fallbackReason
		return
	}
	d.Reasoning = strings.TrimSpace(d.Reasoning) + "\n\n" + fallbackReason
}

func validateDecision(d *Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int, btcEthPosRatio, altcoinPosRatio float64) error {
	validActions := map[string]bool{
		"open_long":   true,
		"open_short":  true,
		"close_long":  true,
		"close_short": true,
		"hold":        true,
		"wait":        true,
	}

	if !validActions[d.Action] {
		return fmt.Errorf("invalid action: %s", d.Action)
	}

	if d.Action == "open_long" || d.Action == "open_short" {
		maxLeverage := altcoinLeverage
		posRatio := altcoinPosRatio
		maxPositionValue := accountEquity * posRatio
		if d.Symbol == "BTCUSDT" || d.Symbol == "ETHUSDT" {
			maxLeverage = btcEthLeverage
			posRatio = btcEthPosRatio
			maxPositionValue = accountEquity * posRatio
		}

		if d.Leverage <= 0 {
			return fmt.Errorf("leverage must be greater than 0: %d", d.Leverage)
		}
		if d.Leverage > maxLeverage {
			logger.Infof("⚠️  [Leverage Fallback] %s leverage exceeded (%dx > %dx), auto-adjusting to limit %dx",
				d.Symbol, d.Leverage, maxLeverage, maxLeverage)
			d.Leverage = maxLeverage
		}
		if d.PositionSizeUSD <= 0 {
			return fmt.Errorf("position size must be greater than 0: %.2f", d.PositionSizeUSD)
		}

		const minPositionSizeGeneral = 12.0
		const minPositionSizeBTCETH = 60.0

		if d.Symbol == "BTCUSDT" || d.Symbol == "ETHUSDT" {
			if d.PositionSizeUSD < minPositionSizeBTCETH {
				return fmt.Errorf("%s opening amount too small (%.2f USDT), must be ≥%.2f USDT", d.Symbol, d.PositionSizeUSD, minPositionSizeBTCETH)
			}
		} else {
			if d.PositionSizeUSD < minPositionSizeGeneral {
				return fmt.Errorf("opening amount too small (%.2f USDT), must be ≥%.2f USDT", d.PositionSizeUSD, minPositionSizeGeneral)
			}
		}

		tolerance := maxPositionValue * 0.01
		if d.PositionSizeUSD > maxPositionValue+tolerance {
			if d.Symbol == "BTCUSDT" || d.Symbol == "ETHUSDT" {
				return fmt.Errorf("BTC/ETH single coin position value cannot exceed %.0f USDT (%.1fx account equity), actual: %.0f", maxPositionValue, posRatio, d.PositionSizeUSD)
			} else {
				return fmt.Errorf("altcoin single coin position value cannot exceed %.0f USDT (%.1fx account equity), actual: %.0f", maxPositionValue, posRatio, d.PositionSizeUSD)
			}
		}
		if d.StopLoss <= 0 || d.TakeProfit <= 0 {
			return fmt.Errorf("stop loss and take profit must be greater than 0")
		}

		if d.Action == "open_long" {
			if d.StopLoss >= d.TakeProfit {
				return fmt.Errorf("for long positions, stop loss price must be less than take profit price")
			}
		} else {
			if d.StopLoss <= d.TakeProfit {
				return fmt.Errorf("for short positions, stop loss price must be greater than take profit price")
			}
		}

		var entryPrice float64
		if d.Action == "open_long" {
			entryPrice = d.StopLoss + (d.TakeProfit-d.StopLoss)*0.2
		} else {
			entryPrice = d.StopLoss - (d.StopLoss-d.TakeProfit)*0.2
		}

		var riskPercent, rewardPercent, riskRewardRatio float64
		if d.Action == "open_long" {
			riskPercent = (entryPrice - d.StopLoss) / entryPrice * 100
			rewardPercent = (d.TakeProfit - entryPrice) / entryPrice * 100
			if riskPercent > 0 {
				riskRewardRatio = rewardPercent / riskPercent
			}
		} else {
			riskPercent = (d.StopLoss - entryPrice) / entryPrice * 100
			rewardPercent = (entryPrice - d.TakeProfit) / entryPrice * 100
			if riskPercent > 0 {
				riskRewardRatio = rewardPercent / riskPercent
			}
		}

		if riskRewardRatio < 3.0 {
			return fmt.Errorf("risk/reward ratio too low (%.2f:1), must be ≥3.0:1 [risk: %.2f%% reward: %.2f%%] [stop loss: %.2f take profit: %.2f]",
				riskRewardRatio, riskPercent, rewardPercent, d.StopLoss, d.TakeProfit)
		}
	}

	return nil
}
