package api

import (
	"net/http"
	"nofx/news"
	"strings"

	"github.com/gin-gonic/gin"
)

const newsMonitorAllowedEmail = "haotianda6@gmail.com"

func canAccessNewsMonitor(email string) bool {
	return strings.EqualFold(strings.TrimSpace(email), newsMonitorAllowedEmail)
}

func (s *Server) handleCryptoNews(c *gin.Context) {
	if !canAccessNewsMonitor(c.GetString("email")) {
		c.JSON(http.StatusForbidden, gin.H{"error": "新闻信息监控正在开发中，后续上线"})
		return
	}
	payload := news.Fetch(c.Request.Context())
	c.JSON(http.StatusOK, payload)
}
