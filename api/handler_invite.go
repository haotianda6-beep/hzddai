package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"nofx/store"
)

func inviteLinkForRequest(c *gin.Context, code string) string {
	scheme := "https"
	if c.Request.TLS == nil {
		scheme = "http"
	}
	if xf := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); xf != "" {
		scheme = strings.Split(xf, ",")[0]
	}
	host := c.Request.Host
	return scheme + "://" + host + "/register?invite=" + code
}

func (s *Server) handleInviteMe(c *gin.Context) {
	userID := c.GetString("user_id")
	u, err := s.store.User().EnsureProfileDefaults(userID)
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
	for _, it := range invited {
		rows = append(rows, gin.H{
			"id":           it.ID,
			"email":        it.Email,
			"display_name": it.DisplayName,
			"created_at":   it.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"invite_code":   u.InviteCode,
		"invite_link":   inviteLinkForRequest(c, u.InviteCode),
		"invited_count": len(rows),
		"reward_text":   "邀请奖励将按运营规则人工结算；本页实时记录你邀请来的全部用户。",
		"invited_users": rows,
		"generated_at":  time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleAdminInviteOverview(c *gin.Context) {
	users, err := s.store.User().GetAll()
	if err != nil {
		SafeInternalError(c, "Failed to list users", err)
		return
	}
	byInviter := map[string][]gin.H{}
	for _, u := range users {
		if strings.TrimSpace(u.InvitedByUserID) == "" {
			continue
		}
		byInviter[u.InvitedByUserID] = append(byInviter[u.InvitedByUserID], gin.H{
			"id":           u.ID,
			"email":        u.Email,
			"display_name": u.DisplayName,
			"balance_usdt": u.BalanceUSDT,
			"created_at":   u.CreatedAt,
		})
	}
	partners := make([]gin.H, 0)
	for _, u := range users {
		customers := byInviter[u.ID]
		if len(customers) == 0 {
			continue
		}
		partners = append(partners, gin.H{
			"id":             u.ID,
			"email":          u.Email,
			"display_name":   u.DisplayName,
			"invite_code":    u.InviteCode,
			"invite_link":    inviteLinkForRequest(c, u.InviteCode),
			"customer_count": len(customers),
			"customers":      customers,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"partners":     partners,
		"generated_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// 邀请伞下单次展开人数上限，避免超大团队拖垮查询。
const maxInviteNetworkNodes = 5000

type rebateBatchRow struct {
	PlatformUserID        string `json:"platform_user_id"`
	Synced                bool   `json:"synced"`
	VIPLevel              int    `json:"vip_level"`
	VIPLevelFloor         int    `json:"vip_level_floor"`
	VIPLevelLocked        bool   `json:"vip_level_locked"`
	IsStudio              bool   `json:"is_studio"`
	TeamTotalRecharge          string `json:"team_total_recharge"`
	RebateBalanceUSDT          string `json:"rebate_balance_usdt"`
	RechargeBalanceUSDT        string `json:"recharge_balance_usdt"`
	LifetimeConsumptionUSDT    string `json:"lifetime_consumption_usdt"`
	RebateExempt               bool   `json:"rebate_exempt"`
}

func parseRebateAmountUSDT(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// effectiveInviteConsumptionUSDT：邀请树「本人消费」展示用。主站统计可返佣消费流水（白名单 reason 的支出）；
// 若返利子系统已同步：取 max(主站合计, 返利侧累计消费业绩)。
func effectiveInviteConsumptionUSDT(allIDs []string, main map[string]float64, rebate map[string]rebateBatchRow) map[string]float64 {
	out := make(map[string]float64, len(allIDs))
	for _, id := range allIDs {
		out[id] = main[id]
	}
	for id, row := range rebate {
		if !row.Synced {
			continue
		}
		lifetime := parseRebateAmountUSDT(row.LifetimeConsumptionUSDT)
		if lifetime > out[id] {
			out[id] = lifetime
		}
	}
	return out
}

type rebateBatchResp struct {
	OK    bool             `json:"ok"`
	Users []rebateBatchRow `json:"users"`
}

func fetchAgentRebateMetricsBulk(platformUserIDs []string) map[string]rebateBatchRow {
	out := make(map[string]rebateBatchRow)
	if len(platformUserIDs) == 0 {
		return out
	}
	body := map[string][]string{"platform_user_ids": platformUserIDs}
	raw, err := json.Marshal(body)
	if err != nil {
		return out
	}
	resp, err := doAgentRebateRequest("POST", "/api/platform/users-metrics", bytes.NewReader(raw))
	if err != nil || resp == nil {
		return out
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out
	}
	var parsed rebateBatchResp
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return out
	}
	for _, u := range parsed.Users {
		out[u.PlatformUserID] = u
	}
	return out
}

func inviteTeamConsumptionMemo(uid string, children map[string][]store.User, personal map[string]float64, memo map[string]float64) float64 {
	if v, ok := memo[uid]; ok {
		return v
	}
	sum := personal[uid]
	for _, ch := range children[uid] {
		sum += inviteTeamConsumptionMemo(ch.ID, children, personal, memo)
	}
	memo[uid] = sum
	return sum
}

func maxInviteDepth(rootID string, children map[string][]store.User) int {
	var dfs func(string) int
	dfs = func(id string) int {
		kids := children[id]
		if len(kids) == 0 {
			return 0
		}
		mx := 0
		for _, ch := range kids {
			sub := dfs(ch.ID)
			if sub > mx {
				mx = sub
			}
		}
		return mx + 1
	}
	return dfs(rootID)
}

func buildInviteTreeNodes(
	parentID string,
	children map[string][]store.User,
	personalConsumption map[string]float64,
	directCount map[string]int64,
	rebate map[string]rebateBatchRow,
	memo map[string]float64,
) []gin.H {
	kids := children[parentID]
	out := make([]gin.H, 0, len(kids))
	for _, ch := range kids {
		uid := ch.ID
		teamU := inviteTeamConsumptionMemo(uid, children, personalConsumption, memo)
		row := rebate[uid]
		sub := buildInviteTreeNodes(uid, children, personalConsumption, directCount, rebate, memo)
		node := gin.H{
			"id":                       uid,
			"email":                    ch.Email,
			"display_name":             ch.DisplayName,
			"created_at":               ch.CreatedAt,
			"balance_usdt":             ch.BalanceUSDT,
			"direct_invite_count":      directCount[uid],
			"personal_consumption_usdt": personalConsumption[uid],
			"team_consumption_usdt":    teamU,
			"rebate_synced":            row.Synced,
			"children":                 sub,
		}
		if row.Synced {
			node["rebate_vip_level"] = row.VIPLevel
			node["rebate_vip_level_floor"] = row.VIPLevelFloor
			node["rebate_vip_level_locked"] = row.VIPLevelLocked
			node["rebate_is_studio"] = row.IsStudio
			node["rebate_team_total"] = row.TeamTotalRecharge
			node["rebate_balance_usdt"] = row.RebateBalanceUSDT
			node["rebate_recharge_balance_usdt"] = row.RechargeBalanceUSDT
			node["rebate_exempt"] = row.RebateExempt
		}
		out = append(out, node)
	}
	return out
}

// GET /api/invite/network — 当前用户邀请伞下全量关系树、主站可返佣消费合计、（若已配置）返利侧 VIP/团队数据。
func (s *Server) handleInviteNetwork(c *gin.Context) {
	userID := c.GetString("user_id")
	u, err := s.store.User().EnsureProfileDefaults(userID)
	if err != nil {
		SafeInternalError(c, "Failed to load invite profile", err)
		return
	}
	desc, err := s.store.User().ListInviteDescendants(userID)
	if err != nil {
		SafeInternalError(c, "Failed to list invite network", err)
		return
	}
	if len(desc) > maxInviteNetworkNodes {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "伞下人数超过 " + strconv.Itoa(maxInviteNetworkNodes) + "，暂不支持在此页一次性展开，请联系管理员处理。",
		})
		return
	}

	// 先整树同步到返利子系统，再拉 metrics，减少「主站有邀请关系但返利侧未建用户」导致的空白数据。
	s.syncInviteNetworkBatchToAgentRebate(u, desc)

	allIDs := make([]string, 0, len(desc)+1)
	allIDs = append(allIDs, userID)
	for _, du := range desc {
		allIDs = append(allIDs, du.ID)
	}

	spendMap, err := s.store.Billing().SumRebateEligibleSpendByUserIDs(allIDs)
	if err != nil {
		SafeInternalError(c, "Failed to sum eligible spend", err)
		return
	}
	directCount, err := s.store.User().CountDirectInvitesByInviterIDs(allIDs)
	if err != nil {
		SafeInternalError(c, "Failed to count direct invites", err)
		return
	}

	rebateMap := fetchAgentRebateMetricsBulk(allIDs)

	children := make(map[string][]store.User)
	for _, du := range desc {
		p := strings.TrimSpace(du.InvitedByUserID)
		if p == "" {
			continue
		}
		children[p] = append(children[p], du)
	}
	effectivePersonal := effectiveInviteConsumptionUSDT(allIDs, spendMap, rebateMap)
	memo := make(map[string]float64)
	tree := buildInviteTreeNodes(userID, children, effectivePersonal, directCount, rebateMap, memo)

	rootTeam := inviteTeamConsumptionMemo(userID, children, effectivePersonal, memo)
	rootSummary := gin.H{
		"id":                        u.ID,
		"email":                     u.Email,
		"display_name":              u.DisplayName,
		"direct_invite_count":       directCount[userID],
		"personal_consumption_usdt": effectivePersonal[userID],
		"team_consumption_usdt":     rootTeam,
	}
	if r, ok := rebateMap[userID]; ok && r.Synced {
		rootSummary["rebate_synced"] = true
		rootSummary["rebate_vip_level"] = r.VIPLevel
		rootSummary["rebate_vip_level_floor"] = r.VIPLevelFloor
		rootSummary["rebate_vip_level_locked"] = r.VIPLevelLocked
		rootSummary["rebate_is_studio"] = r.IsStudio
		rootSummary["rebate_team_total"] = r.TeamTotalRecharge
		rootSummary["rebate_balance_usdt"] = r.RebateBalanceUSDT
		rootSummary["rebate_recharge_balance_usdt"] = r.RechargeBalanceUSDT
		rootSummary["rebate_exempt"] = r.RebateExempt
	} else {
		rootSummary["rebate_synced"] = false
	}

	_, _, rebateOK := agentRebateEnvOK()

	c.JSON(http.StatusOK, gin.H{
		"invite_code":              u.InviteCode,
		"invite_link":              inviteLinkForRequest(c, u.InviteCode),
		"rebate_metrics_available": rebateOK,
		"root":                     rootSummary,
		"tree":                     tree,
		"stats": gin.H{
			"total_descendants": len(desc),
			"max_depth":         maxInviteDepth(userID, children),
		},
		"generated_at": time.Now().UTC().Format(time.RFC3339),
	})
}
