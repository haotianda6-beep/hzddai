package store

import "strings"

// 与主站钱包扣款 reason 一致：仅这些视为「可参与返佣的消费」（周卡体验单独 reason，不在此列表）。
var agentRebateSpendReasons = map[string]struct{}{
	"market_subscription_monthly": {},
	"comkun_display_cycle":        {},
	"comkun_follow_scan":          {},
	"ai_platform_call":            {},
}

// IsAgentRebateEligibleSpendReason 是否应对该笔流水触发返利子系统「消费返佣」回调。
func IsAgentRebateEligibleSpendReason(reason string) bool {
	reason = strings.TrimSpace(reason)
	if reason == "" || reason == "market_subscription_weekly_trial" {
		return false
	}
	_, ok := agentRebateSpendReasons[reason]
	return ok
}

var agentRebateSpendCallback func(userID string, spendUSDT float64, walletLedgerID uint64, ledgerReason string)

// SetAgentRebateSpendCallback 由 API 服务启动时注册：在站内可返佣消费落账成功后异步通知返利子系统。
func SetAgentRebateSpendCallback(fn func(userID string, spendUSDT float64, walletLedgerID uint64, ledgerReason string)) {
	agentRebateSpendCallback = fn
}

// DispatchAgentRebateSpendIfEligible 在钱包扣款流水已提交后调用；金额须为本次实际消费 USDT（正数）。
func DispatchAgentRebateSpendIfEligible(userID string, spendUSDT float64, walletLedgerID uint64, ledgerReason string) {
	if agentRebateSpendCallback == nil || spendUSDT <= 0 || walletLedgerID == 0 {
		return
	}
	if !IsAgentRebateEligibleSpendReason(ledgerReason) {
		return
	}
	agentRebateSpendCallback(userID, spendUSDT, walletLedgerID, ledgerReason)
}
