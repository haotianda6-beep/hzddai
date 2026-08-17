package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *Server) partnerTeamRecords() ([]partnerTeamRecord, error) {
	s.teamDetailsCacheMu.Lock()
	if time.Since(s.teamDetailsCacheAt) < 15*time.Second && s.teamDetailsCache != nil {
		records := append([]partnerTeamRecord(nil), s.teamDetailsCache...)
		s.teamDetailsCacheMu.Unlock()
		return records, nil
	}
	s.teamDetailsCacheMu.Unlock()

	resp, err := doAgentRebateRequest(http.MethodGet, "/api/admin/dashboard?limit=1000", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("partner dashboard status=%d", resp.StatusCode)
	}
	var payload struct {
		Users []partnerTeamRecord `json:"users"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("partner dashboard response invalid")
	}
	records := make([]partnerTeamRecord, 0, len(payload.Users))
	for _, record := range payload.Users {
		record.ID = strings.TrimSpace(record.ID)
		if record.ID != "" {
			records = append(records, record)
		}
	}
	s.teamDetailsCacheMu.Lock()
	s.teamDetailsCache = append([]partnerTeamRecord(nil), records...)
	s.teamDetailsCacheAt = time.Now()
	s.teamDetailsCacheMu.Unlock()
	return records, nil
}

func (s *Server) localTeamUsers() (map[string]partnerLocalUser, error) {
	users, err := s.store.User().GetAll()
	if err != nil {
		return nil, err
	}
	local := make(map[string]partnerLocalUser, len(users))
	for _, user := range users {
		status := "registered"
		if user.LastLoginAt != nil {
			status = "active"
		}
		local[user.ID] = partnerLocalUser{RegisteredAt: user.CreatedAt, Activated: strings.TrimSpace(user.PasswordHash) != "", Status: status}
	}
	return local, nil
}

func (s *Server) handleAdminTeamDetails(c *gin.Context) {
	records, err := s.partnerTeamRecords()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "团队服务暂时不可用"})
		return
	}
	local, err := s.localTeamUsers()
	if err != nil {
		SafeInternalError(c, "团队用户状态读取失败", err)
		return
	}
	snapshot := buildTeamDetails(records, local)
	role := strings.ToLower(strings.TrimSpace(c.Query("role")))
	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	filtered := make([]teamMember, 0, len(snapshot.Members))
	for _, member := range snapshot.Members {
		if role != "" && strings.ToLower(member.Role) != role {
			continue
		}
		if status != "" && strings.ToLower(member.Status) != status {
			continue
		}
		filtered = append(filtered, member)
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	members := paginateTeamMembers(filtered, c.Query("branch_id"), c.Query("q"), page, pageSize)
	c.JSON(http.StatusOK, gin.H{
		"branches":            snapshot.Branches,
		"members":             members,
		"relationship_issues": snapshot.RelationshipIssues,
	})
}
