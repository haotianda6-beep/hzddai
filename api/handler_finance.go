package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"nofx/store"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// handleFinanceUsersLite 财务：用户简表（仅 id/邮箱/昵称/余额），用于为客户入账时查找账号。
func (s *Server) handleFinanceUsersLite(c *gin.Context) {
	users, err := s.store.User().GetAll()
	if err != nil {
		SafeInternalError(c, "Failed to list users", err)
		return
	}
	sort.Slice(users, func(i, j int) bool {
		return users[i].CreatedAt.After(users[j].CreatedAt)
	})
	out := make([]gin.H, 0, len(users))
	for _, u := range users {
		out = append(out, gin.H{
			"id":           u.ID,
			"email":        u.Email,
			"display_name": u.DisplayName,
			"balance_usdt": u.BalanceUSDT,
			"created_at":   u.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"users":        out,
		"generated_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// handleFinanceUserWalletAdjust 财务：仅允许对客户做正数入账（与管理员正数调账一致：返利回调、系统通知）。
func (s *Server) handleFinanceUserWalletAdjust(c *gin.Context) {
	actorID := strings.TrimSpace(c.GetString("user_id"))
	targetID := strings.TrimSpace(c.Param("id"))
	if targetID == "" {
		SafeBadRequest(c, "missing user id")
		return
	}
	if actorID != "" && targetID == actorID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不能给自己入账，请使用其他管理员处理"})
		return
	}
	var req struct {
		DeltaUSDT float64 `json:"delta_usdt" binding:"required"`
		Note      string  `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "delta_usdt 必填")
		return
	}
	if req.DeltaUSDT <= 0 || req.DeltaUSDT > 1_000_000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "财务入账仅支持正数 delta_usdt，且单笔不超过 1e6 USDT"})
		return
	}
	if _, err := s.store.User().GetByID(targetID); err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
			return
		}
		SafeInternalError(c, "Failed to load user", err)
		return
	}
	note := strings.TrimSpace(req.Note)
	if len(note) > 200 {
		note = note[:200]
	}
	reason := "finance_adjust"
	if note != "" {
		reason = "finance_adjust:" + note
	}
	reason = "partner_confirmed:finance"
	rebateEvent := &store.PartnerRebateOutbox{
		EventType: store.PartnerRebateEventDeposit,
		Source:    "finance_confirmed",
		Note:      note,
	}

	newBal, ledgerID, outboxID, err := s.runPlatformWalletAdjust(targetID, req.DeltaUSDT, reason, rebateEvent)
	if err != nil {
		if err == errMarketInsufficientBalance {
			c.JSON(http.StatusPaymentRequired, gin.H{"error": "调账后余额不能为负"})
			return
		}
		SafeInternalError(c, "入账失败", err)
		return
	}
	go s.dispatchPartnerRebateOutboxID(outboxID)

	title := "入账通知"
	body := fmt.Sprintf("财务已为您入账站内余额 +%.2f USDT。当前余额 %.2f USDT。", req.DeltaUSDT, newBal)
	if note != "" {
		body += fmt.Sprintf(" 备注：%s", note)
	}
	_ = s.store.Notification().Add(targetID, title, body)

	c.JSON(http.StatusOK, gin.H{"user_id": targetID, "balance_usdt": newBal, "ledger_id": ledgerID})
}
