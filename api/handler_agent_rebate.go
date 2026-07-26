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
	ok = base != "" && secret != ""
	return
}

func doAgentRebateRequest(method, pathWithQuery string, body io.Reader) (*http.Response, error) {
	base, secret, ok := agentRebateEnvOK()
	if !ok {
		return nil, fmt.Errorf("agent rebate not configured")
	}
	u := strings.TrimRight(base, "/") + pathWithQuery
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Platform-Secret", secret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 20 * time.Second}
	return client.Do(req)
}

// handleAgentRebateBalance proxies GET /api/platform/user-balance on the agent rebate service (JWT user → external_uid).
func (s *Server) handleAgentRebateBalance(c *gin.Context) {
	_, _, ok := agentRebateEnvOK()
	if !ok {
		c.JSON(http.StatusOK, gin.H{"configured": false})
		return
	}
	userID := c.GetString("user_id")
	q := url.Values{}
	q.Set("platform_user_id", userID)
	resp, err := doAgentRebateRequest(http.MethodGet, "/api/platform/user-balance?"+q.Encode(), nil)
	if err != nil {
		logger.Errorf("agent rebate balance: %v", err)
		SafeInternalError(c, "返利服务请求失败", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		c.JSON(http.StatusOK, gin.H{
			"configured": true,
			"synced":     false,
			"message":    "返利账户尚未同步，请稍后再试或联系管理员",
		})
		return
	}
	if resp.StatusCode >= 400 {
		var m map[string]interface{}
		_ = json.Unmarshal(body, &m)
		detail := "返利服务错误"
		if d, ok := m["detail"].(string); ok && d != "" {
			detail = d
		}
		c.JSON(http.StatusOK, gin.H{
			"configured": true,
			"synced":     false,
			"message":    detail,
		})
		return
	}
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		SafeInternalError(c, "解析返利响应失败", err)
		return
	}
	out := gin.H{
		"configured":            true,
		"synced":                true,
		"rebate_balance_usdt":   data["rebate_balance_usdt"],
		"recharge_balance_usdt": data["recharge_balance_usdt"],
		"rebate_nickname":       data["nickname"],
		"rebate_external_uid":   data["external_uid"],
		"rebate_vip_level":      data["vip_level"],
	}
	if v, ok := data["is_studio"]; ok {
		out["rebate_is_studio"] = v
	}
	if v, ok := data["vip_level_floor"]; ok {
		out["rebate_vip_level_floor"] = v
	}
	if v, ok := data["vip_level_locked"]; ok {
		out["rebate_vip_level_locked"] = v
	}
	if v, ok := data["rebate_exempt"]; ok {
		out["rebate_exempt"] = v
	}
	c.JSON(http.StatusOK, out)
}

type agentRebateTransferBody struct {
	Amount string `json:"amount"`
}

// handleAgentRebateTransferToRecharge proxies POST rebate-to-recharge for current user.
func (s *Server) handleAgentRebateTransferToRecharge(c *gin.Context) {
	_, _, envOk := agentRebateEnvOK()
	if !envOk {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "未配置返利服务"})
		return
	}
	var reqBody agentRebateTransferBody
	if err := c.ShouldBindJSON(&reqBody); err != nil || strings.TrimSpace(reqBody.Amount) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供 amount"})
		return
	}
	userID := c.GetString("user_id")
	q := url.Values{}
	q.Set("platform_user_id", userID)
	q.Set("amount", strings.TrimSpace(reqBody.Amount))
	resp, err := doAgentRebateRequest(http.MethodPost, "/api/platform/rebate-to-recharge?"+q.Encode(), nil)
	if err != nil {
		logger.Errorf("agent rebate transfer: %v", err)
		SafeInternalError(c, "返利服务请求失败", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		var m map[string]interface{}
		_ = json.Unmarshal(body, &m)
		detail := string(body)
		if d, ok := m["detail"].(string); ok && d != "" {
			detail = d
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": detail})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func nickFromUser(u *store.User) string {
	if u == nil {
		return ""
	}
	n := strings.TrimSpace(u.DisplayName)
	if n == "" {
		n, _, _ = strings.Cut(u.Email, "@")
	}
	return n
}

// syncInviteChainToAgentRebate 自叶子向根收集邀请链后，从根到叶依次同步，保证多级邀请下返利侧上级均已存在。
func (s *Server) syncInviteChainToAgentRebate(leafUserID string) {
	_, _, ok := agentRebateEnvOK()
	if !ok {
		return
	}
	const maxDepth = 96
	seen := make(map[string]bool)
	var ascending []string
	cur := leafUserID
	for i := 0; i < maxDepth; i++ {
		if cur == "" || seen[cur] {
			break
		}
		seen[cur] = true
		u, err := s.store.User().GetByID(cur)
		if err != nil {
			logger.Warnf("agent rebate chain: load user %s: %v", cur, err)
			break
		}
		ascending = append(ascending, cur)
		p := strings.TrimSpace(u.InvitedByUserID)
		if p == "" {
			break
		}
		cur = p
	}
	for i := len(ascending) - 1; i >= 0; i-- {
		id := ascending[i]
		u, err := s.store.User().GetByID(id)
		if err != nil {
			continue
		}
		nick := nickFromUser(u)
		parent := strings.TrimSpace(u.InvitedByUserID)
		s.notifyAgentRebateUserSync(id, nick, parent)
	}
}

// notifyAgentRebateUserSync 将单个主站用户写入返利库（邀请链），与 POST /api/platform/user-sync 一致。
func (s *Server) notifyAgentRebateUserSync(platformUID, nickname, parentPlatformUID string) {
	_, _, ok := agentRebateEnvOK()
	if !ok {
		return
	}
	body := map[string]string{
		"external_uid": platformUID,
		"nickname":     nickname,
	}
	if strings.TrimSpace(parentPlatformUID) != "" {
		body["parent_external_uid"] = strings.TrimSpace(parentPlatformUID)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return
	}
	resp, err := doAgentRebateRequest(http.MethodPost, "/api/platform/user-sync", bytes.NewReader(raw))
	if err != nil {
		logger.Warnf("agent rebate user-sync request: %v", err)
		return
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		logger.Warnf("agent rebate user-sync failed: status=%d body=%s", resp.StatusCode, string(b))
	}
}

// 须低于返利子系统 max_platform_user_sync_batch（默认 2000），留余量。
const maxAgentRebateUserSyncChunk = 1500

// syncInviteNetworkBatchToAgentRebate 将根用户与全部伞下一次同步到返利库（服务端拓扑排序），
// 避免伞下仅在主站存在、从未触发单笔同步时，users-metrics 长期 unsynced、邀请页充值/VIP 不显示。
func (s *Server) syncInviteNetworkBatchToAgentRebate(root *store.User, descendants []store.User) {
	_, _, ok := agentRebateEnvOK()
	if !ok || root == nil {
		return
	}
	type syncItem struct {
		ExternalUID       string  `json:"external_uid"`
		Nickname          string  `json:"nickname"`
		ParentExternalUID *string `json:"parent_external_uid,omitempty"`
	}
	add := func(id, nick, parent string) syncItem {
		it := syncItem{ExternalUID: strings.TrimSpace(id), Nickname: strings.TrimSpace(nick)}
		p := strings.TrimSpace(parent)
		if p != "" {
			it.ParentExternalUID = &p
		}
		return it
	}
	items := make([]syncItem, 0, 1+len(descendants))
	items = append(items, add(root.ID, nickFromUser(root), root.InvitedByUserID))
	for i := range descendants {
		du := &descendants[i]
		items = append(items, add(du.ID, nickFromUser(du), du.InvitedByUserID))
	}
	for start := 0; start < len(items); start += maxAgentRebateUserSyncChunk {
		end := start + maxAgentRebateUserSyncChunk
		if end > len(items) {
			end = len(items)
		}
		chunk := items[start:end]
		body := map[string][]syncItem{"users": chunk}
		raw, err := json.Marshal(body)
		if err != nil {
			logger.Warnf("agent rebate batch user-sync marshal: %v", err)
			return
		}
		resp, err := doAgentRebateRequest(http.MethodPost, "/api/platform/user-sync", bytes.NewReader(raw))
		if err != nil {
			logger.Warnf("agent rebate batch user-sync request: %v", err)
			return
		}
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode >= 400 {
			logger.Warnf("agent rebate batch user-sync failed: status=%d body=%s", resp.StatusCode, string(b))
		}
	}
}

func (s *Server) postAgentRebatePlatformRecharge(platformUID string, amount float64, note, externalRef string) int {
	q := url.Values{}
	q.Set("platform_user_id", platformUID)
	q.Set("amount", fmt.Sprintf("%.8f", amount))
	if note != "" {
		q.Set("note", note)
	}
	if strings.TrimSpace(externalRef) != "" {
		q.Set("external_ref", strings.TrimSpace(externalRef))
	}
	resp, err := doAgentRebateRequest(http.MethodPost, "/api/platform/recharge?"+q.Encode(), nil)
	if err != nil {
		logger.Warnf("agent rebate platform recharge: %v", err)
		return -1
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func (s *Server) postAgentRebatePlatformConsume(platformUID string, amount float64, note, externalRef string) int {
	q := url.Values{}
	q.Set("platform_user_id", platformUID)
	q.Set("amount", fmt.Sprintf("%.8f", amount))
	if note != "" {
		q.Set("note", note)
	}
	if strings.TrimSpace(externalRef) != "" {
		q.Set("external_ref", strings.TrimSpace(externalRef))
	}
	resp, err := doAgentRebateRequest(http.MethodPost, "/api/platform/consume?"+q.Encode(), nil)
	if err != nil {
		logger.Warnf("agent rebate platform consume: %v", err)
		return -1
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// NotifyAgentRebateAfterWalletRecharge 主站站内余额增加（充值/正数调账/财务入账）后调用：返利侧仅增加「充值记账余额」，不触发返佣。
func (s *Server) NotifyAgentRebateAfterWalletRecharge(userID string, amount float64, walletLedgerID uint64) {
	if _, _, ok := agentRebateEnvOK(); !ok {
		return
	}
	extRef := ""
	if walletLedgerID > 0 {
		extRef = "hzddai_wallet_ledger:" + strconv.FormatUint(walletLedgerID, 10)
	}
	code := s.postAgentRebatePlatformRecharge(userID, amount, "hzddai_wallet_recharge", extRef)
	if code == http.StatusNotFound {
		s.syncInviteChainToAgentRebate(userID)
		code = s.postAgentRebatePlatformRecharge(userID, amount, "hzddai_wallet_recharge", extRef)
	}
	if code != http.StatusOK && code >= 0 {
		logger.Warnf("agent rebate platform recharge status=%d user=%s", code, userID)
	}
}

// NotifyAgentRebateAfterWalletSpend 主站可返佣消费落账后调用：返利侧按消费额记业绩并分佣。ledgerID 用于幂等键。
func (s *Server) NotifyAgentRebateAfterWalletSpend(userID string, spendUSDT float64, walletLedgerID uint64, ledgerReason string) {
	if _, _, ok := agentRebateEnvOK(); !ok {
		return
	}
	if spendUSDT <= 0 || walletLedgerID == 0 || !store.IsAgentRebateEligibleSpendReason(ledgerReason) {
		return
	}
	extRef := "hzddai_wallet_spend:" + strconv.FormatUint(walletLedgerID, 10)
	note := "hzddai_spend:" + ledgerReason
	code := s.postAgentRebatePlatformConsume(userID, spendUSDT, note, extRef)
	if code == http.StatusNotFound {
		s.syncInviteChainToAgentRebate(userID)
		code = s.postAgentRebatePlatformConsume(userID, spendUSDT, note, extRef)
	}
	if code != http.StatusOK && code >= 0 {
		logger.Warnf("agent rebate platform consume status=%d user=%s", code, userID)
	}
}

// handleAgentRebateDividends 代理 GET /api/platform/user-dividends，展示 VIP5 周分红历史。
func (s *Server) handleAgentRebateDividends(c *gin.Context) {
	_, _, ok := agentRebateEnvOK()
	if !ok {
		c.JSON(http.StatusOK, gin.H{"configured": false, "rows": []interface{}{}})
		return
	}
	userID := c.GetString("user_id")
	q := url.Values{}
	q.Set("platform_user_id", userID)
	q.Set("limit", "40")
	resp, err := doAgentRebateRequest(http.MethodGet, "/api/platform/user-dividends?"+q.Encode(), nil)
	if err != nil {
		logger.Errorf("agent rebate dividends: %v", err)
		SafeInternalError(c, "返利服务请求失败", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusOK, gin.H{"configured": true, "rows": []interface{}{}, "error": string(body)})
		return
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		SafeInternalError(c, "解析返利分红响应失败", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"configured": true,
		"synced":     parsed["synced"],
		"rows":       parsed["rows"],
	})
}
