package api

import "testing"

func TestBuildTeamDetailsProtectsCyclesOrphansAndPaginates(t *testing.T) {
	snapshot := buildTeamDetails([]partnerTeamRecord{
		{ID: "branch-a", Name: "A", ParentID: "root", Role: "branch"},
		{ID: "member-1", Name: "M1", ParentID: "branch-a", Role: "retail"},
		{ID: "member-2", Name: "M2", ParentID: "member-1", Role: "retail"},
		{ID: "orphan", Name: "Orphan", ParentID: "missing", Role: "retail"},
		{ID: "cycle-a", Name: "Cycle A", ParentID: "cycle-b", Role: "retail"},
		{ID: "cycle-b", Name: "Cycle B", ParentID: "cycle-a", Role: "retail"},
	}, nil)
	if len(snapshot.Branches) != 1 || snapshot.Branches[0].ID != "branch-a" {
		t.Fatalf("branches=%+v", snapshot.Branches)
	}
	if snapshot.Branches[0].UmbrellaUserCount != 2 || snapshot.Branches[0].DirectUserCount != 1 {
		t.Fatalf("branch counts=%+v, want direct=1 umbrella=2", snapshot.Branches[0])
	}
	if snapshot.RelationshipIssues["orphan"] != "orphan" || snapshot.RelationshipIssues["cycle-a"] != "cycle" {
		t.Fatalf("relationship issues=%v", snapshot.RelationshipIssues)
	}

	page := paginateTeamMembers(snapshot.Members, "branch-a", "m", 1, 1)
	if page.Total != 2 || len(page.Items) != 1 || page.Page != 1 || page.PageSize != 1 {
		t.Fatalf("page=%+v, want total=2 one item", page)
	}
}
