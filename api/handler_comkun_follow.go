package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"nofx/kernel"
	"nofx/store"
)

type comkunMasterBroadcastReq struct {
	SourceStrategyID    string            `json:"source_strategy_id"`
	MasterAccountEquity float64           `json:"master_account_equity"`
	AnalysisText        string            `json:"analysis_text"`
	Decisions           []kernel.Decision `json:"decisions"`
	// MasterState 可选：主控交易所快照 JSON（与主控 maybePublish 写入结构一致：v, positions, pending_orders）
	MasterState json.RawMessage `json:"master_state"`
}

// handleAdminComkunMasterBroadcast 管理员：发布主账户「分析 + 决策」快照，所有绑定该 source_strategy_id 的跟单用户拉取
func (s *Server) handleAdminComkunMasterBroadcast(c *gin.Context) {
	var req comkunMasterBroadcastReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sid := strings.TrimSpace(req.SourceStrategyID)
	if sid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_strategy_id required"})
		return
	}
	raw, err := json.Marshal(req.Decisions)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "decisions marshal failed"})
		return
	}
	stateStr := strings.TrimSpace(string(req.MasterState))
	row, err := s.store.ComkunFollow().InsertBroadcast(sid, req.MasterAccountEquity, req.AnalysisText, string(raw), stateStr)
	if err != nil {
		SafeInternalError(c, "insert broadcast failed", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":                    row.ID,
		"source_strategy_id":    row.SourceStrategyID,
		"master_account_equity": row.MasterAccountEquity,
		"created_at":            row.CreatedAt,
	})
}

type comkunTraderTokenCreditReq struct {
	TraderID string `json:"trader_id"`
	Tokens   int64  `json:"tokens"`
	Note     string `json:"note"`
}

// handleAdminComkunTraderTokenCredit 管理员：给某交易员增加旧版「跟单虚拟 token」池（历史兼容）。
func (s *Server) handleAdminComkunTraderTokenCredit(c *gin.Context) {
	var req comkunTraderTokenCreditReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tid := strings.TrimSpace(req.TraderID)
	if tid == "" || req.Tokens <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "trader_id and positive tokens required"})
		return
	}
	if _, err := s.store.Trader().GetByID(tid); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "trader not found"})
		return
	}
	if err := s.store.ComkunFollow().CreditFollowBalance(tid, req.Tokens); err != nil {
		SafeInternalError(c, "credit failed", err)
		return
	}
	bal, _ := s.store.ComkunFollow().GetFollowBalance(tid)
	c.JSON(http.StatusOK, gin.H{"trader_id": tid, "balance_tokens": bal, "credited": req.Tokens})
}

// handleComkunFollowBalance 当前用户某交易员跟单可用余额：统一使用站内账户 USDT 余额。
func (s *Server) handleComkunFollowBalance(c *gin.Context) {
	userID := c.GetString("user_id")
	traderID := strings.TrimSpace(c.Query("trader_id"))
	if traderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "trader_id query required"})
		return
	}
	if _, err := s.store.Trader().GetFullConfig(userID, traderID); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "trader not found or not owned"})
		return
	}
	u, err := s.store.User().GetByID(userID)
	if err != nil {
		SafeInternalError(c, "balance read failed", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"trader_id":          traderID,
		"balance_usdt":       u.BalanceUSDT,
		"billing":            "platform_wallet",
		"scan_fee_usdt":      store.ComkunFollowScanFeeUSDTOrDefault(),
		"scan_fee_min_usdt":  0.0489,
		"scan_fee_max_usdt":  store.ComkunFollowScanFeeUSDTOrDefault(),
		"legacy_token_logic": false,
	})
}
