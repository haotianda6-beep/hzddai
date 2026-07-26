package api

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// adminEmailList 管理员邮箱白名单；可用环境变量 COMKUN_ADMIN_EMAILS 逗号分隔覆盖
func adminEmailList() []string {
	raw := strings.TrimSpace(os.Getenv("COMKUN_ADMIN_EMAILS"))
	if raw == "" {
		return []string{"haotianda6@gmail.com"}
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{"haotianda6@gmail.com"}
	}
	return out
}

func isAdminEmail(email string) bool {
	em := strings.TrimSpace(strings.ToLower(email))
	for _, a := range adminEmailList() {
		if em == a {
			return true
		}
	}
	return false
}

// financeEmailList 财务账号邮箱：可为其他用户做「正数入账」（与管理员调账同逻辑，流水 reason 为 finance_adjust*）。
// 环境变量 COMKUN_FINANCE_EMAILS 逗号分隔；未配置时默认包含运营指定财务账号。
func financeEmailList() []string {
	raw := strings.TrimSpace(os.Getenv("COMKUN_FINANCE_EMAILS"))
	if raw == "" {
		return []string{"1013018910@qq.com"}
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{"1013018910@qq.com"}
	}
	return out
}

func isFinanceEmail(email string) bool {
	em := strings.TrimSpace(strings.ToLower(email))
	for _, a := range financeEmailList() {
		if em == a {
			return true
		}
	}
	return false
}

func (s *Server) adminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isAdminEmail(c.GetString("email")) {
			c.JSON(http.StatusForbidden, gin.H{"error": "需要管理员权限"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func (s *Server) financeMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isFinanceEmail(c.GetString("email")) {
			c.JSON(http.StatusForbidden, gin.H{"error": "需要财务权限"})
			c.Abort()
			return
		}
		c.Next()
	}
}
