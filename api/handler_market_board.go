package api

import (
	"net/http"
	"nofx/marketboard"

	"github.com/gin-gonic/gin"
)

// handleMarketBoard GET /api/market/board — 聚合免费公开行情（无需登录）
// Query: quick=1 仅 Binance+外汇快路径，供趋势页首屏；全量带 45s 服务端缓存。
func (s *Server) handleMarketBoard(c *gin.Context) {
	ctx := c.Request.Context()
	quick := c.Query("quick") == "1" || c.Query("quick") == "true"
	payload := marketboard.FetchBoard(ctx, quick)
	if quick {
		c.Header("Cache-Control", "public, max-age=15, stale-while-revalidate=30")
	} else {
		c.Header("Cache-Control", "public, max-age=30, stale-while-revalidate=60")
	}
	c.JSON(http.StatusOK, payload)
}
