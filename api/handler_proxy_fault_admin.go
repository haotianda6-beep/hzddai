package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// handleAdminOutboundProxyFaults GET /admin/outbound-proxy-faults
func (s *Server) handleAdminOutboundProxyFaults(c *gin.Context) {
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	rows, err := s.store.ProxyFault().ListAdminRecent(limit)
	if err != nil {
		SafeInternalError(c, "list proxy faults", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"events": rows})
}
