package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleListUserNotifications(c *gin.Context) {
	userID := c.GetString("user_id")
	rows, err := s.store.Notification().List(userID, 50)
	if err != nil {
		SafeInternalError(c, "Failed to load notifications", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"notifications": rows})
}
