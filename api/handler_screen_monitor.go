package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// handleScreenMonitorBroadcast accepts master state from the OCR screen monitor.
// Authenticated via X-Screen-Monitor-Secret header matching SCREEN_MONITOR_SECRET env.
func (s *Server) handleScreenMonitorBroadcast(c *gin.Context) {
	secret := strings.TrimSpace(c.GetHeader("X-Screen-Monitor-Secret"))
	if secret == "" || secret != strings.TrimSpace(os.Getenv("SCREEN_MONITOR_SECRET")) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

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
