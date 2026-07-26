package api

import (
	"net/http"
	"strings"

	"nofx/logger"
	"nofx/store"

	"github.com/gin-gonic/gin"
)

type proxyPoolImportRequest struct {
	Lines string `json:"lines"` // 多行文本，每行一条
}

// handleAdminOutboundProxyPoolList GET /admin/outbound-proxy-pool
func (s *Server) handleAdminOutboundProxyPoolList(c *gin.Context) {
	rows, err := s.store.ProxyPool().ListAdmin()
	if err != nil {
		SafeInternalError(c, "Failed to list proxy pool", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"entries": rows})
}

// handleAdminOutboundProxyPoolImport POST /admin/outbound-proxy-pool/import
func (s *Server) handleAdminOutboundProxyPoolImport(c *gin.Context) {
	var req proxyPoolImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	raw := strings.Split(strings.ReplaceAll(req.Lines, "\r\n", "\n"), "\n")
	added, skipped, errs := s.store.ProxyPool().ImportLines(raw)
	logger.Infof("📥 Admin proxy pool import: added=%d skipped=%d", added, skipped)
	c.JSON(http.StatusOK, gin.H{
		"added":   added,
		"skipped": skipped,
		"errors":  errs,
	})
}

// handleAdminOutboundProxyPoolDelete DELETE /admin/outbound-proxy-pool/:id
func (s *Server) handleAdminOutboundProxyPoolDelete(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing id"})
		return
	}
	if err := s.store.ProxyPool().DeleteUnassigned(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// handleAdminOutboundProxyPoolRelease POST /admin/outbound-proxy-pool/:id/release
func (s *Server) handleAdminOutboundProxyPoolRelease(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing id"})
		return
	}
	var row store.OutboundProxyPool
	if err := s.store.GormDB().Where("id = ?", id).First(&row).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	exID := strings.TrimSpace(row.AssignedExchangeID)
	if exID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该条目未分配"})
		return
	}
	uid := strings.TrimSpace(row.AssignedUserID)
	if err := s.store.ProxyPool().ReleaseByExchangeID(exID); err != nil {
		SafeInternalError(c, "release pool", err)
		return
	}
	ex, err := s.store.Exchange().GetByID(uid, exID)
	if err == nil && ex != nil {
		if uerr := s.store.Exchange().Update(uid, exID, ex.Enabled, "", "", "", ex.Testnet,
			ex.APIURL,
			ex.HyperliquidWalletAddr, ex.HyperliquidUnifiedAcct,
			ex.AsterUser, ex.AsterSigner, "", ex.LighterWalletAddr, "", "", ex.LighterAPIKeyIndex,
			nil, true); uerr != nil {
			logger.Warnf("admin release proxy: clear exchange outbound failed: %v", uerr)
		}
	}
	s.exchangeAccountStateCache.Invalidate(uid)
	if list, err := s.store.Trader().ListByExchangeID(uid, exID); err == nil {
		for _, t := range list {
			if t != nil && strings.TrimSpace(t.ID) != "" {
				s.traderManager.RemoveTrader(t.ID)
			}
		}
	} else {
		logger.Warnf("admin proxy release: list traders for exchange %s: %v", exID, err)
	}
	_ = s.traderManager.LoadUserTradersFromStore(s.store, uid)
	c.JSON(http.StatusOK, gin.H{"message": "released"})
}

type proxyPoolAssignRequest struct {
	UserID     string `json:"user_id"`
	ExchangeID string `json:"exchange_id"`
}

// handleAdminOutboundProxyPoolAssign POST /admin/outbound-proxy-pool/:id/assign
func (s *Server) handleAdminOutboundProxyPoolAssign(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing id"})
		return
	}
	var req proxyPoolAssignRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.UserID) == "" || strings.TrimSpace(req.ExchangeID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "需要 JSON：user_id、exchange_id"})
		return
	}
	if err := s.store.ProxyPool().AssignToExchange(id, req.UserID, req.ExchangeID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	s.exchangeAccountStateCache.Invalidate(strings.TrimSpace(req.UserID))
	// 与 handleUpdateExchanges 一致：内存里已加载的 AutoTrader 不会重读 DB，必须 Remove 再 Load，新 SOCKS5 才会进币安 HTTP 客户端
	uid := strings.TrimSpace(req.UserID)
	eid := strings.TrimSpace(req.ExchangeID)
	if list, err := s.store.Trader().ListByExchangeID(uid, eid); err == nil {
		for _, t := range list {
			if t != nil && strings.TrimSpace(t.ID) != "" {
				s.traderManager.RemoveTrader(t.ID)
			}
		}
	} else {
		logger.Warnf("admin proxy assign: list traders for exchange %s: %v", eid, err)
	}
	_ = s.traderManager.LoadUserTradersFromStore(s.store, uid)
	c.JSON(http.StatusOK, gin.H{"message": "assigned"})
}
