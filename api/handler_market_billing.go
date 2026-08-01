package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"nofx/store"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 非跟单策略仍保留旧套餐；跟单策略统一 0U 解锁，运行中按主控 AI 广播次数扣平台余额。
const (
	marketSubMonthlyUSDT  = 600.0
	marketSubWeeklyUSDT   = 40.0 // 周卡体验价（全账号仅一次，流水 reason 单独，不参与返利/分账）
	marketSubMonthlyLabel = "monthly"
	marketSubWeeklyLabel  = "weekly"
)

func marketPlanPriceAndDuration(plan string) (price float64, extend time.Duration, ledgerReason string, ok bool) {
	switch strings.ToLower(strings.TrimSpace(plan)) {
	case marketSubMonthlyLabel:
		return marketSubMonthlyUSDT, 30 * 24 * time.Hour, "market_subscription_monthly", true
	case marketSubWeeklyLabel:
		return marketSubWeeklyUSDT, 7 * 24 * time.Hour, "market_subscription_weekly_trial", true
	default:
		return 0, 0, "", false
	}
}

func isFreeComkunMarketSubscription(access string, cfg *store.StrategyConfig) bool {
	if cfg == nil || access != store.MarketAccessSubscription {
		return false
	}
	return store.IsComkunMarketFollowStrategy(cfg) || cfg.ComkunFollowListingTemplate
}

// handleMarketStrategyPurchase 解锁策略市场订阅；跟单策略不再卖包月，后续按主控 AI 广播次数扣平台余额。
func (s *Server) handleMarketStrategyPurchase(c *gin.Context) {
	userID := c.GetString("user_id")
	var req struct {
		StrategyID string `json:"strategy_id" binding:"required"`
		Plan       string `json:"plan"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "strategy_id 必填")
		return
	}

	st, err := s.store.Strategy().GetByIDForMarket(req.StrategyID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "策略不存在或未上架"})
			return
		}
		SafeInternalError(c, "Failed to load strategy", err)
		return
	}
	if st.UserID == userID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无需购买自己的策略"})
		return
	}

	access := store.EffectivePublicListingAccess(st)
	if access != store.MarketAccessSubscription && access != store.MarketAccessPrivate {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该策略为公开或开源，无需购买"})
		return
	}

	var cfg store.StrategyConfig
	_ = json.Unmarshal([]byte(st.Config), &cfg)
	if isFreeComkunMarketSubscription(access, &cfg) {
		var newBal float64
		if err := s.store.Transaction(func(tx *gorm.DB) error {
			if e := s.store.Billing().GrantEntitlementIfMissing(tx, userID, st.ID, 0); e != nil {
				return e
			}
			if u, e := s.store.User().GetByID(userID); e == nil && u != nil {
				newBal = u.BalanceUSDT
			}
			return nil
		}); err != nil {
			SafeInternalError(c, "免费订阅失败", err)
			return
		}
		if newBal == 0 {
			if u, e := s.store.User().GetByID(userID); e == nil && u != nil {
				newBal = u.BalanceUSDT
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"balance_usdt":       newBal,
			"strategy_id":        st.ID,
			"paid_usdt":          0,
			"plan":               "free",
			"subscription_until": nil,
			"message":            "订阅成功：跟单策略 0U 解锁，运行后按主控 AI 分析/广播次数扣除平台余额",
		})
		return
	}

	price, extend, ledgerReason, ok := marketPlanPriceAndDuration(req.Plan)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plan 须为 monthly（月卡 600U·30 天）或 weekly（周卡体验 40U·7 天，每账号仅一次）"})
		return
	}

	isWeekly := strings.EqualFold(strings.TrimSpace(req.Plan), marketSubWeeklyLabel)

	var newBal float64
	err = s.store.Transaction(func(tx *gorm.DB) error {
		if isWeekly {
			claimed, e := s.store.User().TryClaimMarketWeeklyTrial(tx, userID)
			if e != nil {
				return e
			}
			if !claimed {
				return errMarketWeeklyTrialAlreadyUsed
			}
		}
		bal, okBal, e := s.store.User().AddBalanceDelta(tx, userID, -price)
		if e != nil {
			return e
		}
		if !okBal {
			return errMarketInsufficientBalance
		}
		newBal = bal
		_, e = s.store.Billing().AppendLedger(tx, userID, -price, newBal, ledgerReason, st.ID)
		if e != nil {
			return e
		}
		return s.store.Billing().ExtendMarketSubscription(tx, userID, st.ID, price, extend)
	})
	if err != nil {
		if err == errMarketWeeklyTrialAlreadyUsed {
			c.JSON(http.StatusBadRequest, gin.H{"error": "周卡体验每个账号仅可购买一次，请选择月卡续订"})
			return
		}
		if err == errMarketInsufficientBalance {
			c.JSON(http.StatusPaymentRequired, gin.H{"error": "余额不足，请先充值"})
			return
		}
		SafeInternalError(c, "购买失败", err)
		return
	}

	if u, e := s.store.User().GetByID(userID); e == nil && u != nil {
		newBal = u.BalanceUSDT
	}

	var subUntil *time.Time
	if row, e := s.store.Billing().GetEntitlement(userID, st.ID); e == nil && row != nil {
		subUntil = row.SubscriptionUntil
	}

	msg := "订阅成功：有效期内同步策略不再按轮扣除站内余额"
	c.JSON(http.StatusOK, gin.H{
		"balance_usdt":       newBal,
		"strategy_id":        st.ID,
		"paid_usdt":          price,
		"plan":               strings.ToLower(strings.TrimSpace(req.Plan)),
		"subscription_until": subUntil,
		"message":            msg,
	})
}

var errMarketInsufficientBalance = errors.New("insufficient balance for market purchase")

var errMarketWeeklyTrialAlreadyUsed = errors.New("market weekly trial already used for this account")

func (s *Server) handleMarketEntitlements(c *gin.Context) {
	userID := c.GetString("user_id")
	ents, err := s.store.Billing().ListEntitlements(userID)
	if err != nil {
		SafeInternalError(c, "Failed to list entitlements", err)
		return
	}
	ids := make([]string, 0, len(ents))
	for _, e := range ents {
		ids = append(ids, e.StrategyID)
	}
	c.JSON(http.StatusOK, gin.H{"strategy_ids": ids, "items": ents})
}

// handleMarketOwnedStrategy 已购买或本人发布的策略：返回完整配置（用于复制到策略构建器）
func (s *Server) handleMarketOwnedStrategy(c *gin.Context) {
	userID := c.GetString("user_id")
	strategyID := c.Param("id")
	if strategyID == "" {
		SafeBadRequest(c, "missing strategy id")
		return
	}

	st, err := s.store.Strategy().GetByIDForMarket(strategyID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "策略不存在或未上架"})
			return
		}
		SafeInternalError(c, "Failed to load strategy", err)
		return
	}

	allowed := st.UserID == userID
	if !allowed {
		ok, e := s.store.Billing().HasEntitlement(userID, strategyID)
		if e != nil {
			SafeInternalError(c, "Failed to check entitlement", e)
			return
		}
		allowed = ok
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "请先购买该策略"})
		return
	}

	var cfg store.StrategyConfig
	if err := json.Unmarshal([]byte(st.Config), &cfg); err != nil {
		SafeInternalError(c, "Invalid strategy config", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":            st.ID,
		"name":          st.Name,
		"description":   st.Description,
		"market_access": store.EffectivePublicListingAccess(st),
		"config":        cfg,
	})
}
