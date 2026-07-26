package api

import (
	"github.com/gin-gonic/gin"
)

func emptyAccountLock() gin.H {
	return gin.H{"locked": false}
}

func (s *Server) buildAccountLock(userID string) gin.H {
	return emptyAccountLock()
}

func (s *Server) accountLockMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}
