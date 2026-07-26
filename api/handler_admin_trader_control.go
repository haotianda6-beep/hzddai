package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// handleAdminStartTrader 管理员代对应用户启动交易员 POST /admin/traders/:id/start
func (s *Server) handleAdminStartTrader(c *gin.Context) {
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
	s.handleStartTraderForUser(c, tr.UserID, traderID)
}

// handleAdminStopTrader 管理员代对应用户停止交易员 POST /admin/traders/:id/stop
func (s *Server) handleAdminStopTrader(c *gin.Context) {
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
	s.executeStopTrader(c, tr.UserID, traderID, true)
}
