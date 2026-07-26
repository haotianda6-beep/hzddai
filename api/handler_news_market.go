package api

import (
	"net/http"
	"nofx/news"

	"github.com/gin-gonic/gin"
)

type goldMarketSnapshotRequest struct {
	Secret string `json:"secret"`
	news.GoldMarketSnapshotInput
}

func (s *Server) handleGoldMarketSnapshot(c *gin.Context) {
	var req goldMarketSnapshotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid market snapshot"})
		return
	}
	if !mt4StrategySecretOK(req.Secret) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if err := news.RecordGoldMarketSnapshot(req.GoldMarketSnapshotInput); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
