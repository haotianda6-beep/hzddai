package store

import "testing"

func TestIsBnScreenMirrorMasterStrategyID(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"bn-screen-mirror-ec92c8f5", true},
		{"  bn-screen-mirror-abc  ", true},
		{"okx-screen-mirror-01", true},
		{"okx-screen-mirror-test", true},
		{"cff474d3-1111-2222-3333-444444444444", false},
		{"comkun_follow_listing", false},
		{"", false},
		{"bn-screen-mirror", false},
	}
	for _, tc := range cases {
		got := IsBnScreenMirrorMasterStrategyID(tc.id)
		if got != tc.want {
			t.Errorf("IsBnScreenMirrorMasterStrategyID(%q) = %v, want %v", tc.id, got, tc.want)
		}
	}
}

func TestIsOkxScreenMirrorMasterStrategyID(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"okx-screen-mirror-01", true},
		{" okx-screen-mirror-test ", true},
		{"bn-screen-mirror-abc", false},
		{"okx-screen-mirror", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsOkxScreenMirrorMasterStrategyID(tc.id); got != tc.want {
			t.Errorf("IsOkxScreenMirrorMasterStrategyID(%q) = %v, want %v", tc.id, got, tc.want)
		}
	}
}

func TestListingTemplateMasterSkipsExchangeExecution(t *testing.T) {
	manualMaster := &StrategyConfig{
		ComkunFollowListingTemplate:              true,
		ComkunListingMasterSkipExchangeExecution: true,
	}
	if !ListingTemplateMasterSkipsExchangeExecution(manualMaster) {
		t.Fatal("manual listing master should skip exchange execution")
	}
	if !StrategyRequiresComkunAIModel(manualMaster) {
		t.Fatal("manual listing master should require COMKUN-AI placeholder")
	}

	aiMaster := &StrategyConfig{
		ComkunFollowListingTemplate:              true,
		ComkunListingMasterSkipExchangeExecution: false,
	}
	if ListingTemplateMasterSkipsExchangeExecution(aiMaster) {
		t.Fatal("AI listing master with skip=false should execute on exchange")
	}
	if StrategyRequiresComkunAIModel(aiMaster) {
		t.Fatal("AI listing master should not require COMKUN-AI placeholder")
	}

	follower := &StrategyConfig{
		ComkunMarketFollow:           true,
		ComkunMarketSourceStrategyID: "master-id",
	}
	if ListingTemplateMasterSkipsExchangeExecution(follower) {
		t.Fatal("follower should not be treated as listing master")
	}
	if !StrategyRequiresComkunAIModel(follower) {
		t.Fatal("follower should require COMKUN-AI")
	}
}

func TestValidateComkunAIStrategyBindingAllowsDirectModelsForPreferredStrategies(t *testing.T) {
	follower := &StrategyConfig{
		ComkunMarketFollow:           true,
		ComkunMarketSourceStrategyID: "master-id",
	}
	deepseek := &AIModel{ID: "user_deepseek", Provider: "deepseek", Name: "DeepSeek"}
	if err := ValidateComkunAIStrategyBinding(deepseek, follower); err != nil {
		t.Fatalf("direct model should be allowed for COMKUN-preferred strategy: %v", err)
	}

	comkun := &AIModel{ID: "comkun_ai", Provider: "comkun_ai", Name: "COMKUN-AI"}
	if err := ValidateComkunAIStrategyBinding(comkun, follower); err != nil {
		t.Fatalf("COMKUN-AI should remain allowed for COMKUN-preferred strategy: %v", err)
	}
	if err := ValidateComkunAIStrategyBinding(comkun, &StrategyConfig{}); err == nil {
		t.Fatal("COMKUN-AI placeholder should still be blocked for ordinary strategies")
	}
}
