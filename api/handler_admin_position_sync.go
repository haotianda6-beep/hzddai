package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	nfxtrader "nofx/trader"
)

// handleAdminSyncTraderPositionsFromExchange 管理员：按交易所当前持仓重写本机 OPEN 记录（清除「幽灵持仓」）。
// POST /admin/traders/:id/sync-positions-from-exchange
func (s *Server) handleAdminSyncTraderPositionsFromExchange(c *gin.Context) {
	traderID := c.Param("id")
	tr, err := s.store.Trader().GetByID(traderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Trader not found"})
			return
		}
		SafeInternalError(c, "Failed to load trader", err)
		return
	}

	fullCfg, err := s.store.Trader().GetFullConfig(tr.UserID, traderID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Trader config not found"})
		return
	}
	ex := fullCfg.Exchange
	if ex == nil || !ex.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "交易所未配置或未启用"})
		return
	}

	tempTrader, err := buildExchangeProbeTrader(ex, tr.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := nfxtrader.CreatePositionSnapshot(traderID, ex.ID, ex.ExchangeType, tempTrader, s.store); err != nil {
		SafeInternalError(c, "同步持仓失败", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "已按交易所实况重写本机开仓记录（若无实盘持仓则库内 OPEN 会清空）",
	})
}
