package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"nofx/logger"
	"nofx/store"

	"github.com/gin-gonic/gin"
)

func agentRebateEnvOK() (base, secret string, ok bool) {
	base = strings.TrimSpace(os.Getenv("AGENT_REBATE_BASE_URL"))
	secret = strings.TrimSpace(os.Getenv("AGENT_REBATE_PLATFORM_SECRET"))
	return base, secret, base != "" && secret != ""
}

func doAgentRebateRequest(method, path string, body io.Reader) (*http.Response, error) {
	base, secret, ok := agentRebateEnvOK()
	if !ok {
		return nil, fmt.Errorf("partner rebate not configured")
	}
	req, err := http.NewRequest(method, strings.TrimRight(base, "/")+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Platform-Secret", secret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return (&http.Client{Timeout: 20 * time.Second}).Do(req)
}

func proxyAgentRebate(c *gin.Context, method, path string, payload interface{}) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			SafeInternalError(c, "返佣请求编码失败", err)
			return
		}
		body = bytes.NewReader(raw)
	}
	resp, err := doAgentRebateRequest(method, path, body)
	if err != nil {
		SafeInternalError(c, "返佣服务请求失败", err)
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		SafeInternalError(c, "返佣服务响应无效", err)
		return
	}
	c.JSON(resp.StatusCode, out)
}

func (s *Server) handlePartnerDashboard(c *gin.Context) {
	userID := c.GetString("user_id")
	q := url.Values{"platform_user_id": {userID}, "limit": {"300"}}
	path := "/api/platform/dashboard?" + q.Encode()
	resp, err := doAgentRebateRequest(http.MethodGet, path, nil)
	if err == nil && resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		s.syncInviteChainToAgentRebate(userID)
		proxyAgentRebate(c, http.MethodGet, path, nil)
		return
	}
	if err != nil {
		SafeInternalError(c, "返佣服务请求失败", err)
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out interface{}
	if json.Unmarshal(raw, &out) != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "返佣服务响应无效"})
		return
	}
	c.JSON(resp.StatusCode, out)
}

func (s *Server) handlePartnerWithdrawal(c *gin.Context) {
	var req struct {
		AmountUSDT string `json:"amount_usdt" binding:"required"`
		Address    string `json:"address" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "amount_usdt和address必填")
		return
	}
	proxyAgentRebate(c, http.MethodPost, "/api/platform/withdrawals", gin.H{
		"platform_user_id": c.GetString("user_id"),
		"amount_usdt":      req.AmountUSDT,
		"network":          "TRC20",
		"address":          strings.TrimSpace(req.Address),
	})
}

func (s *Server) handlePartnerStudioRequest(c *gin.Context) {
	var req struct {
		CandidateUserID string `json:"candidate_user_id" binding:"required"`
		Note            string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "candidate_user_id必填")
		return
	}
	proxyAgentRebate(c, http.MethodPost, "/api/platform/studio-requests", gin.H{
		"branch_platform_user_id":    c.GetString("user_id"),
		"candidate_platform_user_id": strings.TrimSpace(req.CandidateUserID),
		"note":                       strings.TrimSpace(req.Note),
	})
}

func (s *Server) handleAdminPartnerDashboard(c *gin.Context) {
	proxyAgentRebate(c, http.MethodGet, "/api/admin/dashboard?limit=500", nil)
}

func (s *Server) handleAdminPartnerRole(c *gin.Context) {
	var req struct {
		UserID string `json:"user_id" binding:"required"`
		Role   string `json:"role" binding:"required"`
		Note   string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "user_id和role必填")
		return
	}
	proxyAgentRebate(c, http.MethodPost, "/api/admin/roles", gin.H{
		"platform_user_id": strings.TrimSpace(req.UserID),
		"role":             strings.ToLower(strings.TrimSpace(req.Role)),
		"actor":            c.GetString("user_id"),
		"note":             strings.TrimSpace(req.Note),
	})
}

func (s *Server) handleAdminPartnerStudioReview(c *gin.Context) {
	s.proxyPartnerReview(c, "/api/admin/studio-requests/"+c.Param("id")+"/review")
}

func (s *Server) handleAdminPartnerWithdrawalReview(c *gin.Context) {
	s.proxyPartnerReview(c, "/api/admin/withdrawals/"+c.Param("id")+"/review")
}

func (s *Server) proxyPartnerReview(c *gin.Context, path string) {
	var req struct {
		Action string `json:"action" binding:"required"`
		Note   string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "action必填")
		return
	}
	proxyAgentRebate(c, http.MethodPost, path, gin.H{
		"action": strings.ToLower(strings.TrimSpace(req.Action)),
		"actor":  c.GetString("user_id"),
		"note":   strings.TrimSpace(req.Note),
	})
}

type partnerSyncItem struct {
	ExternalUID      string `json:"external_uid"`
	Nickname         string `json:"nickname"`
	ParentExternalID string `json:"parent_external_uid,omitempty"`
}

func partnerNickname(user *store.User) string {
	if name := strings.TrimSpace(user.DisplayName); name != "" {
		return name
	}
	if at := strings.Index(user.Email, "@"); at > 0 {
		return user.Email[:at]
	}
	return user.Email
}

func (s *Server) syncInviteChainToAgentRebate(leafUserID string) bool {
	chain := make([]*store.User, 0, 8)
	seen := map[string]bool{}
	for id := strings.TrimSpace(leafUserID); id != "" && !seen[id]; {
		seen[id] = true
		user, err := s.store.User().GetByID(id)
		if err != nil {
			logger.Warnf("partner rebate sync user %s: %v", id, err)
			return false
		}
		chain = append(chain, user)
		id = strings.TrimSpace(user.InvitedByUserID)
	}
	items := make([]partnerSyncItem, 0, len(chain))
	for i := len(chain) - 1; i >= 0; i-- {
		u := chain[i]
		items = append(items, partnerSyncItem{u.ID, partnerNickname(u), strings.TrimSpace(u.InvitedByUserID)})
	}
	return s.postPartnerSync(items)
}

func (s *Server) postPartnerSync(items []partnerSyncItem) bool {
	raw, _ := json.Marshal(gin.H{"users": items})
	resp, err := doAgentRebateRequest(http.MethodPost, "/api/platform/user-sync", bytes.NewReader(raw))
	if err != nil {
		logger.Warnf("partner rebate user sync: %v", err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		logger.Warnf("partner rebate user sync status=%d body=%s", resp.StatusCode, body)
		return false
	}
	return true
}

func (s *Server) postAgentRebatePlatformConsume(userID string, amount float64, reason string, walletLedgerID uint64) int {
	q := url.Values{}
	q.Set("platform_user_id", userID)
	q.Set("amount", strconv.FormatFloat(amount, 'f', -1, 64))
	q.Set("external_ref", "hzddai_wallet_spend:"+strconv.FormatUint(walletLedgerID, 10))
	q.Set("note", "hzddai_spend:"+reason)
	resp, err := doAgentRebateRequest(http.MethodPost, "/api/platform/consume?"+q.Encode(), nil)
	if err != nil {
		logger.Warnf("partner rebate consume: %v", err)
		return -1
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// NotifyAgentRebateAfterWalletSpend reports only committed eligible wallet spends.
func (s *Server) NotifyAgentRebateAfterWalletSpend(userID string, spendUSDT float64, walletLedgerID uint64, reason string) {
	if _, _, ok := agentRebateEnvOK(); !ok || spendUSDT <= 0 || walletLedgerID == 0 || !store.IsAgentRebateEligibleSpendReason(reason) {
		return
	}
	code := s.postAgentRebatePlatformConsume(userID, spendUSDT, reason, walletLedgerID)
	if code == http.StatusNotFound && s.syncInviteChainToAgentRebate(userID) {
		code = s.postAgentRebatePlatformConsume(userID, spendUSDT, reason, walletLedgerID)
	}
	if code != http.StatusOK && code >= 0 {
		logger.Warnf("partner rebate consume status=%d user=%s", code, userID)
	}
}
