package api

import (
	"sort"
	"strings"
	"time"
)

type partnerTeamRecord struct {
	ID       string `json:"platform_user_id"`
	Name     string `json:"nickname"`
	ParentID string `json:"parent_platform_user_id"`
	Role     string `json:"role"`
}

type partnerLocalUser struct {
	RegisteredAt time.Time
	Activated    bool
	Status       string
}

type teamBranch struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Status            string `json:"status"`
	ParentID          string `json:"parentId,omitempty"`
	DirectUserCount   int    `json:"directUserCount"`
	UmbrellaUserCount int    `json:"umbrellaUserCount"`
}

type teamMember struct {
	ID           string    `json:"id"`
	Name         string    `json:"name,omitempty"`
	BranchID     string    `json:"branchId,omitempty"`
	ParentID     string    `json:"parentId,omitempty"`
	Role         string    `json:"role"`
	Level        int       `json:"level"`
	Status       string    `json:"status"`
	RegisteredAt time.Time `json:"registeredAt,omitempty"`
	Activated    bool      `json:"activated"`
}

type teamMembersPage struct {
	Items    []teamMember `json:"items"`
	Total    int          `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"pageSize"`
}

type teamDetailsSnapshot struct {
	Branches           []teamBranch      `json:"branches"`
	Members            []teamMember      `json:"members"`
	RelationshipIssues map[string]string `json:"relationshipIssues,omitempty"`
}

func buildTeamDetails(records []partnerTeamRecord, local map[string]partnerLocalUser) teamDetailsSnapshot {
	byID := make(map[string]partnerTeamRecord, len(records))
	for _, record := range records {
		record.ID = strings.TrimSpace(record.ID)
		if record.ID == "" {
			continue
		}
		record.ParentID = strings.TrimSpace(record.ParentID)
		record.Role = strings.ToLower(strings.TrimSpace(record.Role))
		record.Name = strings.TrimSpace(record.Name)
		byID[record.ID] = record
	}
	issues := make(map[string]string)
	branchFor := make(map[string]string, len(byID))
	levelFor := make(map[string]int, len(byID))
	for id := range byID {
		current := id
		seen := make(map[string]int)
		path := make([]string, 0, 8)
		for current != "" {
			if index, repeated := seen[current]; repeated {
				for _, cycleID := range path[index:] {
					issues[cycleID] = "cycle"
				}
				break
			}
			seen[current] = len(path)
			path = append(path, current)
			record, exists := byID[current]
			if !exists {
				issues[id] = "orphan"
				break
			}
			if record.Role == "branch" {
				branchFor[id] = current
				levelFor[id] = len(path) - 1
				break
			}
			parent := record.ParentID
			if parent == "" {
				break
			}
			if _, exists := byID[parent]; !exists {
				issues[id] = "orphan"
				break
			}
			current = parent
		}
	}

	branches := make([]teamBranch, 0)
	for id, record := range byID {
		if record.Role != "branch" {
			continue
		}
		status := "active"
		if user, ok := local[id]; ok && strings.TrimSpace(user.Status) != "" {
			status = user.Status
		}
		branch := teamBranch{ID: id, Name: record.Name, Status: status, ParentID: record.ParentID}
		for memberID, branchID := range branchFor {
			if branchID != id || memberID == id {
				continue
			}
			branch.UmbrellaUserCount++
			if byID[memberID].ParentID == id {
				branch.DirectUserCount++
			}
		}
		branches = append(branches, branch)
	}
	sort.Slice(branches, func(i, j int) bool { return branches[i].ID < branches[j].ID })

	members := make([]teamMember, 0, len(byID))
	for id, record := range byID {
		if record.Role == "branch" {
			continue
		}
		member := teamMember{ID: id, Name: record.Name, BranchID: branchFor[id], ParentID: record.ParentID, Role: record.Role, Level: levelFor[id]}
		member.Status = "partner_only"
		if user, ok := local[id]; ok {
			member.Status = user.Status
			member.RegisteredAt = user.RegisteredAt
			member.Activated = user.Activated
		}
		members = append(members, member)
	}
	sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
	return teamDetailsSnapshot{Branches: branches, Members: members, RelationshipIssues: issues}
}

func paginateTeamMembers(members []teamMember, branchID, query string, page, pageSize int) teamMembersPage {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	query = strings.ToLower(strings.TrimSpace(query))
	filtered := make([]teamMember, 0, len(members))
	for _, member := range members {
		if branchID != "" && member.BranchID != branchID {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(member.ID), query) && !strings.Contains(strings.ToLower(member.Name), query) {
			continue
		}
		filtered = append(filtered, member)
	}
	start := (page - 1) * pageSize
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + pageSize
	if end > len(filtered) {
		end = len(filtered)
	}
	return teamMembersPage{Items: filtered[start:end], Total: len(filtered), Page: page, PageSize: pageSize}
}
