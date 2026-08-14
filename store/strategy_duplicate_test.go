package store

import "testing"

func TestApplyComkunFollowChildConfigSeparatesListingFromLiveRoute(t *testing.T) {
	source := &Strategy{ID: "market-slot-1", UserID: "owner"}
	cfg := &StrategyConfig{
		ComkunFollowListingTemplate:  true,
		ComkunMarketSourceStrategyID: HZMasterSourceStrategyID("live-master-1"),
	}
	ApplyComkunFollowChildConfig(nil, source, "buyer", cfg)
	if !cfg.ComkunMarketFollow || cfg.ComkunFollowListingTemplate {
		t.Fatalf("child follow flags invalid: %+v", cfg)
	}
	if cfg.ComkunMarketListingStrategyID != source.ID {
		t.Fatalf("listing id=%q want=%q", cfg.ComkunMarketListingStrategyID, source.ID)
	}
	if cfg.ComkunMarketSourceStrategyID != HZMasterSourceStrategyID("live-master-1") {
		t.Fatalf("live route was overwritten: %q", cfg.ComkunMarketSourceStrategyID)
	}
}
