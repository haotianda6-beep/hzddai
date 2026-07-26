package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"nofx/store"
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

// handlePostWalletRecharge 平台站内余额充值（演示：非链上支付，用于策略市场消费）
func (s *Server) handlePostWalletRecharge(c *gin.Context) {
	userID := c.GetString("user_id")
	var req struct {
		AmountUSDT float64 `json:"amount_usdt" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "amount_usdt 必填")
		return
	}
	if req.AmountUSDT < 1 || req.AmountUSDT > 1_000_000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "单次充值金额需在 1～1,000,000 USDT（站内）之间"})
		return
	}
	var newBal float64
	var ledgerID uint64
	err := s.store.Transaction(func(tx *gorm.DB) error {
		bal, ok, e := s.store.User().AddBalanceDelta(tx, userID, req.AmountUSDT)
		if e != nil {
			return e
		}
		if !ok {
			return errors.New("balance update failed")
		}
		newBal = bal
		row := &store.WalletLedger{
			UserID:        userID,
			Delta:         req.AmountUSDT,
			BalanceAfter:  newBal,
			Reason:        "recharge",
			RefStrategyID: "",
		}
		if e := tx.Create(row).Error; e != nil {
			return e
		}
		ledgerID = row.ID
		return nil
	})
	if err != nil {
		SafeInternalError(c, "充值失败", err)
		return
	}
	// 返利侧：仅同步「充值记账余额」，不触发返佣；返佣在站内实际消费后由 NotifyAgentRebateAfterWalletSpend 触发
	s.NotifyAgentRebateAfterWalletRecharge(userID, req.AmountUSDT, ledgerID)

	_ = s.store.Notification().Add(
		userID,
		"充值成功",
		fmt.Sprintf("您已成功充值 %.2f USDT（站内）。当前余额 %.2f USDT。", req.AmountUSDT, newBal),
	)
	c.JSON(http.StatusOK, gin.H{
		"balance_usdt": newBal,
		"message":      "充值成功",
	})
}
