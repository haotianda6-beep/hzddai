package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// handleGetAICosts returns AI charges for a specific trader
func (s *Server) handleGetAICosts(c *gin.Context) {
	traderID := c.Query("trader_id")
	period := c.DefaultQuery("period", "today")

	if traderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "trader_id is required"})
		return
	}

	charges, total, err := s.store.AICharge().GetCharges(traderID, period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"charges": charges,
		"total":   total,
		"count":   len(charges),
	})
}

// handleGetAICostsSummary returns AI cost summary across all traders
func (s *Server) handleGetAICostsSummary(c *gin.Context) {
	period := c.DefaultQuery("period", "today")

	total, count, byModel := s.store.AICharge().GetSummary(period)

	c.JSON(http.StatusOK, gin.H{
		"total":    total,
		"count":    count,
		"by_model": byModel,
	})
}

// handleGetAIPlatformUsage returns the current user's platform AI billing ledger.
func (s *Server) handleGetAIPlatformUsage(c *gin.Context) {
	userID := strings.TrimSpace(c.GetString("user_id"))
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing user context"})
		return
	}
	limit := 50
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	rows, err := s.store.AIPlatformUsage().ListAdmin(userID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":        rows,
		"generated_at": time.Now().UTC().Format(time.RFC3339),
	})
}
