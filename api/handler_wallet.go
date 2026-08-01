package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleGetWallet(c *gin.Context) {
	userID := c.GetString("user_id")
	u, err := s.store.User().GetByID(userID)
	if err != nil {
		SafeInternalError(c, "Failed to load user", err)
		return
	}
	ledgers, err := s.store.Billing().ListLedger(userID, 40)
	if err != nil {
		SafeInternalError(c, "Failed to load ledger", err)
		return
	}
	ents, err := s.store.Billing().ListEntitlements(userID)
	if err != nil {
		SafeInternalError(c, "Failed to load entitlements", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"balance_usdt":               u.BalanceUSDT,
		"ledger":                     ledgers,
		"entitlements":               ents,
		"account_lock":               s.buildAccountLock(userID),
		"market_subscription_alerts": []gin.H{},
		"market_weekly_trial_used":   u.MarketWeeklyTrialUsed,
		"is_admin":                   isAdminEmail(u.Email),
		"is_finance":                 isFinanceEmail(u.Email),
	})
}
