package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"nofx/logger"

	"github.com/gin-gonic/gin"
)

type adminRebateSetUserBody struct {
	UserID           string `json:"user_id"`
	VIPLevel         *int   `json:"vip_level"`
	VIPLevelLocked   *bool  `json:"vip_level_locked"`
	IsStudio         *bool  `json:"is_studio"`
}

// POST /admin/rebate/set-user-attrs — 代理返利服务：设置 VIP、锁定等级、工作室身份（须 AGENT_REBATE_*）
func (s *Server) handleAdminRebateSetUserAttrs(c *gin.Context) {
	_, _, ok := agentRebateEnvOK()
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "未配置返利服务"})
		return
	}
	var body adminRebateSetUserBody
	if err := c.ShouldBindJSON(&body); err != nil || body.UserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供 user_id"})
		return
	}
	payload := map[string]interface{}{
		"platform_user_id": body.UserID,
	}
	if body.VIPLevel != nil {
		payload["vip_level"] = *body.VIPLevel
	}
	if body.VIPLevelLocked != nil {
		payload["vip_level_locked"] = *body.VIPLevelLocked
	}
	if body.IsStudio != nil {
		payload["is_studio"] = *body.IsStudio
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数序列化失败"})
		return
	}
	resp, err := doAgentRebateRequest(http.MethodPost, "/api/platform/set-user-attrs", bytes.NewReader(raw))
	if err != nil {
		logger.Errorf("admin rebate set-user-attrs: %v", err)
		SafeInternalError(c, "返利服务请求失败", err)
		return
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	c.Data(resp.StatusCode, "application/json", b)
}
