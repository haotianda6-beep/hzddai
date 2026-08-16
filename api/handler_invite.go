package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func inviteLinkForRequest(c *gin.Context, code string) string {
	scheme := "https"
	if c.Request.TLS == nil {
		scheme = "http"
	}
	if forwarded := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); forwarded != "" {
		scheme = strings.Split(forwarded, ",")[0]
	}
	return scheme + "://" + c.Request.Host + "/register?invite=" + code
}

func (s *Server) handleInviteMe(c *gin.Context) {
	userID := c.GetString("user_id")
	user, err := s.store.User().EnsureProfileDefaults(userID)
	if err != nil {
		SafeInternalError(c, "Failed to load invite profile", err)
		return
	}
	invited, err := s.store.User().ListInvitedUsers(userID)
	if err != nil {
		SafeInternalError(c, "Failed to list invited users", err)
		return
	}
	rows := make([]gin.H, 0, len(invited))
	for _, row := range invited {
		rows = append(rows, gin.H{
			"id":           row.ID,
			"email":        row.Email,
			"display_name": row.DisplayName,
			"created_at":   row.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"invite_code":   user.InviteCode,
		"invite_link":   inviteLinkForRequest(c, user.InviteCode),
		"invited_count": len(rows),
		"reward_text":   "邀请关系用于工作室和分公司的返佣归属。",
		"invited_users": rows,
		"generated_at":  time.Now().UTC().Format(time.RFC3339),
	})
}
