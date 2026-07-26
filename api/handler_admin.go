package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"nofx/store"
	"nofx/trader"
)

const (
	adminOverviewLiveConcurrency = 4
	adminOverviewLiveTimeout     = 14 * time.Second
)

type adminBinPos struct {
	TraderID   string  `json:"trader_id"`
	TraderName string  `json:"trader_name"`
	Symbol     string  `json:"symbol"`
	Side       string  `json:"side"`
	Size       float64 `json:"size"`
}

// adminRunningTraderMetric 管理端「运行中交易员」一行
type adminRunningTraderMetric struct {
	TraderID              string      `json:"trader_id"`
	TraderName            string      `json:"trader_name"`
	UserID                string      `json:"user_id"`
	UserEmail             string      `json:"user_email"`
	UserDisplayName       string      `json:"user_display_name"`
	IsRunning             bool        `json:"is_running"`
	TotalEquity           interface{} `json:"total_equity"`
	AvailableBalance      interface{} `json:"available_balance"`
	TotalPnL              interface{} `json:"total_pnl"`
	TotalPnLPct           interface{} `json:"total_pnl_pct"`
	MarginUsed            interface{} `json:"margin_used"`
	MarginUsedPct         interface{} `json:"margin_used_pct"`
	TotalUnrealizedProfit float64     `json:"total_unrealized_profit"`
	PositionCount         int         `json:"position_count"`
	Positions             interface{} `json:"positions"`
	Error                 string      `json:"error,omitempty"`
	SnapshotAt            string      `json:"snapshot_at,omitempty"`
	MetricsSource         string      `json:"metrics_source,omitempty"`
	CheckedAt             string      `json:"checked_at,omitempty"`
}

// handleAdminUsersOverview 管理员：所有用户、余额、交易员数量、币安未平仓摘要
func (s *Server) handleAdminUsersOverview(c *gin.Context) {
	users, err := s.store.User().GetAll()
	if err != nil {
		SafeInternalError(c, "Failed to list users", err)
		return
	}

	out := make([]gin.H, 0, len(users))
	var runPending []struct {
		u  store.User
		tr *store.Trader
	}
	for _, u := range users {
		traders, err := s.store.Trader().List(u.ID)
		if err != nil {
			SafeInternalError(c, "Failed to list traders", err)
			return
		}

		var binanceOpen []adminBinPos
		exchangeSummaries := make([]gin.H, 0)
		traderSummaries := make([]gin.H, 0, len(traders))

		exList, err := s.store.Exchange().List(u.ID)
		if err == nil {
			for _, ex := range exList {
				exchangeSummaries = append(exchangeSummaries, gin.H{
					"id":            ex.ID,
					"exchange_type": ex.ExchangeType,
					"account_name":  ex.AccountName,
					"enabled":       ex.Enabled,
				})
			}
		}

		for _, tr := range traders {
			exMeta, xerr := s.store.Exchange().GetByID(u.ID, tr.ExchangeID)
			if xerr != nil || exMeta == nil {
				traderSummaries = append(traderSummaries, gin.H{
					"id":               tr.ID,
					"name":             tr.Name,
					"exchange_id":      tr.ExchangeID,
					"exchange_type":    "",
					"account_name":     "",
					"enabled":          false,
					"missing_exchange": true,
				})
				continue
			}
			traderSummaries = append(traderSummaries, gin.H{
				"id":            tr.ID,
				"name":          tr.Name,
				"exchange_id":   tr.ExchangeID,
				"exchange_type": exMeta.ExchangeType,
				"account_name":  exMeta.AccountName,
				"enabled":       exMeta.Enabled,
			})
		}

		for _, tr := range traders {
			runPending = append(runPending, struct {
				u  store.User
				tr *store.Trader
			}{u, tr})
			ex, err := s.store.Exchange().GetByID(u.ID, tr.ExchangeID)
			if err != nil || ex == nil {
				continue
			}
			et := strings.ToLower(strings.TrimSpace(ex.ExchangeType))
			if et != "binance" {
				continue
			}
			positions, err := s.store.Position().GetOpenPositions(tr.ID)
			if err != nil {
				continue
			}
			for _, p := range positions {
				binanceOpen = append(binanceOpen, adminBinPos{
					TraderID:   tr.ID,
					TraderName: tr.Name,
					Symbol:     p.Symbol,
					Side:       p.Side,
					Size:       p.Quantity,
				})
			}
		}

		out = append(out, gin.H{
			"id":                     u.ID,
			"email":                  u.Email,
			"display_name":           u.DisplayName,
			"balance_usdt":           u.BalanceUSDT,
			"created_at":             u.CreatedAt,
			"trader_count":           len(traders),
			"exchanges":              exchangeSummaries,
			"traders":                traderSummaries,
			"binance_open_positions": binanceOpen,
		})
	}

	runningMetrics := make([]adminRunningTraderMetric, 0, len(runPending))
	if len(runPending) > 0 {
		ids := make([]string, 0, len(runPending))
		for _, job := range runPending {
			if job.tr.IsRunning {
				ids = append(ids, job.tr.ID)
			}
		}
		var latestEq map[string]*store.EquitySnapshot
		if len(ids) > 0 {
			var eqErr error
			latestEq, eqErr = s.store.Equity().GetLatestByTraderIDs(ids)
			if eqErr != nil {
				SafeInternalError(c, "读取权益快照失败", eqErr)
				return
			}
		}

		// 运行中：并行拉交易所实时（与排行榜快照解耦），失败再退回本机 equity 快照
		liveByID := make(map[string]adminRunningTraderMetric)
		var liveMu sync.Mutex
		sem := make(chan struct{}, adminOverviewLiveConcurrency)
		var wg sync.WaitGroup
		for idx := range runPending {
			job := runPending[idx]
			if !job.tr.IsRunning {
				continue
			}
			wg.Add(1)
			go func(job struct {
				u  store.User
				tr *store.Trader
			}) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				at, err := s.traderManager.GetTrader(job.tr.ID)
				if err != nil {
					return
				}
				done := make(chan struct{})
				var row adminRunningTraderMetric
				var ferr error
				go func() {
					defer close(done)
					row, ferr = adminFetchLiveRunningMetric(at, job)
				}()
				select {
				case <-done:
					if ferr != nil {
						return
					}
					liveMu.Lock()
					liveByID[job.tr.ID] = row
					liveMu.Unlock()
				case <-time.After(adminOverviewLiveTimeout):
					return
				}
			}(job)
		}
		wg.Wait()

		for _, job := range runPending {
			row := adminRunningTraderMetric{
				TraderID:        job.tr.ID,
				TraderName:      job.tr.Name,
				UserID:          job.u.ID,
				UserEmail:       job.u.Email,
				UserDisplayName: job.u.DisplayName,
				IsRunning:       job.tr.IsRunning,
			}
			if !job.tr.IsRunning {
				row.Error = "当前未运行"
				runningMetrics = append(runningMetrics, row)
				continue
			}
			if live, ok := liveByID[job.tr.ID]; ok {
				runningMetrics = append(runningMetrics, live)
				continue
			}
			eq := latestEq[job.tr.ID]
			if eq == nil {
				row.Error = "暂无数据：内存未加载该交易员或交易所请求超时/失败，且无本机权益快照（策略跑过周期后才会有快照）"
				runningMetrics = append(runningMetrics, row)
				continue
			}
			row.MetricsSource = "equity_snapshot"
			row.SnapshotAt = eq.Timestamp.UTC().Format(time.RFC3339)
			row.TotalEquity = eq.TotalEquity
			row.AvailableBalance = eq.Balance
			row.MarginUsedPct = eq.MarginUsedPct
			row.TotalUnrealizedProfit = eq.UnrealizedPnL
			if eq.TotalEquity > 0 && eq.MarginUsedPct >= 0 {
				row.MarginUsed = eq.TotalEquity * eq.MarginUsedPct / 100.0
			}
			initBal := job.tr.InitialBalance
			totalPnL := eq.TotalEquity - initBal
			row.TotalPnL = totalPnL
			if initBal > 1e-9 {
				row.TotalPnLPct = (totalPnL / initBal) * 100.0
			} else {
				row.TotalPnLPct = 0.0
			}
			dbPos, pErr := s.store.Position().GetOpenPositions(job.tr.ID)
			if pErr != nil {
				row.Error = fmt.Sprintf("读取本机持仓: %v", pErr)
				row.PositionCount = eq.PositionCount
				runningMetrics = append(runningMetrics, row)
				continue
			}
			row.PositionCount = len(dbPos)
			posOut := make([]gin.H, 0, len(dbPos))
			for _, p := range dbPos {
				qty := p.Quantity
				if qty < 0 {
					qty = -qty
				}
				posOut = append(posOut, gin.H{
					"symbol":         p.Symbol,
					"side":           p.Side,
					"quantity":       qty,
					"entry_price":    p.EntryPrice,
					"mark_price":     p.EntryPrice,
					"unrealized_pnl": 0.0,
					"leverage":       p.Leverage,
				})
			}
			row.Positions = posOut
			runningMetrics = append(runningMetrics, row)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"users":           out,
		"running_traders": runningMetrics,
		"running_traders_meta": gin.H{
			"source": "exchange_live_preferred",
			"hint":   "运行中优先请求交易所实时账户与持仓（并行限流）；失败或超时时退回本机 trader_equity_snapshots。快照行的持仓来自本机库，若与实盘不一致请点「同步持仓」。",
		},
		"generated_at": time.Now().UTC().Format(time.RFC3339),
	})
}

func adminFetchLiveRunningMetric(at *trader.AutoTrader, job struct {
	u  store.User
	tr *store.Trader
}) (adminRunningTraderMetric, error) {
	acc, err := at.GetAccountInfo()
	if err != nil {
		return adminRunningTraderMetric{}, err
	}
	positions, err := at.GetPositions()
	if err != nil {
		return adminRunningTraderMetric{}, err
	}
	row := adminRunningTraderMetric{
		TraderID:        job.tr.ID,
		TraderName:      job.tr.Name,
		UserID:          job.u.ID,
		UserEmail:       job.u.Email,
		UserDisplayName: job.u.DisplayName,
		IsRunning:       true,
		MetricsSource:   "exchange_live",
		CheckedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	if v, ok := acc["total_equity"].(float64); ok {
		row.TotalEquity = v
	}
	if v, ok := acc["available_balance"].(float64); ok {
		row.AvailableBalance = v
	}
	if v, ok := acc["total_pnl"].(float64); ok {
		row.TotalPnL = v
	}
	if v, ok := acc["total_pnl_pct"].(float64); ok {
		row.TotalPnLPct = v
	}
	if v, ok := acc["margin_used"].(float64); ok {
		row.MarginUsed = v
	}
	if v, ok := acc["margin_used_pct"].(float64); ok {
		row.MarginUsedPct = v
	}
	if v, ok := acc["unrealized_profit"].(float64); ok {
		row.TotalUnrealizedProfit = v
	}
	row.PositionCount = len(positions)
	posOut := make([]gin.H, 0, len(positions))
	for _, p := range positions {
		sym, _ := p["symbol"].(string)
		side, _ := p["side"].(string)
		qty := adminFloatFromIface(p["quantity"])
		ep := adminFloatFromIface(p["entry_price"])
		mp := adminFloatFromIface(p["mark_price"])
		upnl := adminFloatFromIface(p["unrealized_pnl"])
		lev := adminIntFromIface(p["leverage"])
		posOut = append(posOut, gin.H{
			"symbol":         sym,
			"side":           side,
			"quantity":       qty,
			"entry_price":    ep,
			"mark_price":     mp,
			"unrealized_pnl": upnl,
			"leverage":       lev,
		})
	}
	row.Positions = posOut
	return row, nil
}

func adminFloatFromIface(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	default:
		return 0
	}
}

func adminIntFromIface(v interface{}) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	default:
		return 0
	}
}

// runPlatformWalletAdjust 站内余额调账：写 wallet_ledgers，返回新余额与 ledger id（事务内完成）。
func (s *Server) runPlatformWalletAdjust(targetID string, delta float64, reason string) (newBal float64, ledgerID uint64, err error) {
	err = s.store.Transaction(func(tx *gorm.DB) error {
		bal, ok, e := s.store.User().AddBalanceDelta(tx, targetID, delta)
		if e != nil {
			return e
		}
		if !ok {
			return errMarketInsufficientBalance
		}
		newBal = bal
		row := &store.WalletLedger{
			UserID:        targetID,
			Delta:         delta,
			BalanceAfter:  newBal,
			Reason:        reason,
			RefStrategyID: "",
		}
		if e := tx.Create(row).Error; e != nil {
			return e
		}
		ledgerID = row.ID
		return nil
	})
	return newBal, ledgerID, err
}

func (s *Server) handleAdminUserWalletAdjust(c *gin.Context) {
	targetID := c.Param("id")
	if targetID == "" {
		SafeBadRequest(c, "missing user id")
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
	if req.DeltaUSDT == 0 || req.DeltaUSDT < -1_000_000 || req.DeltaUSDT > 1_000_000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "delta_usdt 不能为 0，且绝对值不超过 1e6"})
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

	adminNote := strings.TrimSpace(req.Note)
	if len(adminNote) > 200 {
		adminNote = adminNote[:200]
	}
	reason := "admin_adjust"
	if adminNote != "" {
		reason = "admin_adjust:" + adminNote
	}

	newBal, ledgerID, err := s.runPlatformWalletAdjust(targetID, req.DeltaUSDT, reason)
	if err != nil {
		if err == errMarketInsufficientBalance {
			c.JSON(http.StatusPaymentRequired, gin.H{"error": "调账后余额不能为负"})
			return
		}
		SafeInternalError(c, "调账失败", err)
		return
	}
	// 正数调账：与站内充值一致，仅同步返利侧充值记账余额，不触发消费返佣
	if req.DeltaUSDT > 0 {
		s.NotifyAgentRebateAfterWalletRecharge(targetID, req.DeltaUSDT, ledgerID)
	}

	title := "余额变动"
	body := fmt.Sprintf("管理员已调整您的站内余额 %+.2f USDT。当前余额 %.2f USDT。", req.DeltaUSDT, newBal)
	if adminNote != "" {
		body += fmt.Sprintf(" 备注：%s", adminNote)
	}
	if req.DeltaUSDT > 0 {
		title = "入账通知"
	}
	_ = s.store.Notification().Add(targetID, title, body)

	c.JSON(http.StatusOK, gin.H{"user_id": targetID, "balance_usdt": newBal})
}

func (s *Server) handleAdminAIPlatformUsage(c *gin.Context) {
	userID := strings.TrimSpace(c.Query("user_id"))
	limit := 200
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			limit = n
		}
	}
	rows, err := s.store.AIPlatformUsage().ListAdmin(userID, limit)
	if err != nil {
		SafeInternalError(c, "Failed to list AI platform usage", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":        rows,
		"generated_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// handleAdminUserDetail 可选：单用户详情（与 overview 重复时可不用；保留占位扩展）
func (s *Server) handleAdminUserDetail(c *gin.Context) {
	id := c.Param("id")
	u, err := s.store.User().GetByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
			return
		}
		SafeInternalError(c, "Failed to load user", err)
		return
	}
	ledgers, _ := s.store.Billing().ListLedger(id, 100)
	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"id":           u.ID,
			"email":        u.Email,
			"display_name": u.DisplayName,
			"balance_usdt": u.BalanceUSDT,
			"created_at":   u.CreatedAt,
		},
		"ledger": ledgers,
	})
}

// handleAdminBroadcastNotification 管理员：发布全站公告（所有登录用户在通知列表中可见）
func (s *Server) handleAdminBroadcastNotification(c *gin.Context) {
	var req struct {
		Title string `json:"title" binding:"required"`
		Body  string `json:"body" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "title 与 body 必填")
		return
	}
	title := strings.TrimSpace(req.Title)
	body := strings.TrimSpace(req.Body)
	if title == "" || body == "" {
		SafeBadRequest(c, "title 与 body 不能为空")
		return
	}
	if err := s.store.Notification().AddBroadcast(title, body); err != nil {
		SafeBadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "公告已发布"})
}

// handleAdminStrategyMarketReview 管理员：审核客户策略市场展示申请。
func (s *Server) handleAdminStrategyMarketReview(c *gin.Context) {
	strategyID := strings.TrimSpace(c.Param("id"))
	if strategyID == "" {
		SafeBadRequest(c, "strategy id required")
		return
	}
	var req struct {
		Action string `json:"action" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "action required")
		return
	}
	action := strings.TrimSpace(strings.ToLower(req.Action))
	if action != "approve" && action != "reject" {
		SafeBadRequest(c, "action must be approve or reject")
		return
	}

	st, err := s.store.Strategy().GetByIDAny(strategyID)
	if err != nil || st == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "策略不存在"})
		return
	}
	var cfg store.StrategyConfig
	if strings.TrimSpace(st.Config) != "" {
		_ = json.Unmarshal([]byte(st.Config), &cfg)
	}
	requestedAccess := strings.TrimSpace(strings.ToLower(cfg.MarketReviewRequestedAccess))
	if !store.IsListedOnMarket(requestedAccess) {
		SafeBadRequest(c, "该策略没有待审核的市场展示申请")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	cfg.MarketReviewReviewedAt = now
	cfg.MarketReviewReviewedBy = c.GetString("user_id")
	actualAccess := store.MarketAccessOff
	ownerTitle := "策略市场上架未通过"
	ownerBody := fmt.Sprintf("你的策略「%s」暂未通过管理员审核，当前不会显示在策略市场。", st.Name)
	if action == "approve" {
		cfg.MarketReviewStatus = marketReviewStatusApproved
		actualAccess = requestedAccess
		ownerTitle = "策略市场上架已通过"
		ownerBody = fmt.Sprintf("你的策略「%s」已通过管理员审核，现在会按申请方式显示在策略市场。", st.Name)
	} else {
		cfg.MarketReviewStatus = marketReviewStatusRejected
	}

	raw, err := json.Marshal(cfg)
	if err != nil {
		SafeInternalError(c, "Serialize strategy review config", err)
		return
	}
	next := &store.Strategy{
		ID:                 st.ID,
		UserID:             st.UserID,
		Name:               st.Name,
		Description:        st.Description,
		Config:             string(raw),
		MarketAccess:       actualAccess,
		ShowAfterRename:    false,
		SourceStrategyID:   st.SourceStrategyID,
		SourceMarketAccess: st.SourceMarketAccess,
		ContentLocked:      st.ContentLocked,
	}
	store.SyncListingFlagsFromAccess(next, actualAccess)
	if err := s.store.Strategy().Update(next); err != nil {
		SafeInternalError(c, "Update strategy market review", err)
		return
	}
	_ = s.store.Notification().Add(st.UserID, ownerTitle, ownerBody)

	c.JSON(http.StatusOK, gin.H{
		"message":       "审核已处理",
		"strategy_id":   st.ID,
		"action":        action,
		"market_access": actualAccess,
	})
}
