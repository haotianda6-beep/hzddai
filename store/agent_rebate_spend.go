package store

import "strings"

var agentRebateSpendReasons = map[string]struct{}{
	"market_subscription_monthly": {},
	"comkun_follow_scan":          {},
	"ai_platform_call":            {},
}

func IsAgentRebateEligibleSpendReason(reason string) bool {
	reason = strings.TrimSpace(reason)
	_, ok := agentRebateSpendReasons[reason]
	return ok
}

var agentRebateSpendCallback func(userID string, spendUSDT float64, walletLedgerID uint64, ledgerReason string)

func SetAgentRebateSpendCallback(fn func(userID string, spendUSDT float64, walletLedgerID uint64, ledgerReason string)) {
	agentRebateSpendCallback = fn
}

func DispatchAgentRebateSpendIfEligible(userID string, spendUSDT float64, walletLedgerID uint64, ledgerReason string) {
	if agentRebateSpendCallback == nil || spendUSDT <= 0 || walletLedgerID == 0 || !IsAgentRebateEligibleSpendReason(ledgerReason) {
		return
	}
	agentRebateSpendCallback(userID, spendUSDT, walletLedgerID, ledgerReason)
}
