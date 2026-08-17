package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestListComkunFollowingStatsUsesDistinctRunningUsersAndActiveEntitlements(t *testing.T) {
	db, err := InitGorm(filepath.Join(t.TempDir(), "follow-stats.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&User{}, &Strategy{}, &Trader{}, &Exchange{}, &StrategyMarketEntitlement{},
	); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	activeUntil := now.Add(24 * time.Hour)
	expiredUntil := now.Add(-time.Hour)
	sourceOne := HZMasterSourceStrategyID("master-1")
	sourceTwo := HZMasterSourceStrategyID("master-2")
	strategies := []Strategy{
		{ID: "listing-1", Name: "Strategy One", Config: `{"comkun_follow_listing_template":true,"comkun_market_source_strategy_id":"` + sourceOne + `"}`},
		{ID: "listing-2", Name: "Strategy Two", Config: `{"comkun_follow_listing_template":true,"comkun_market_source_strategy_id":"` + sourceTwo + `"}`},
		{ID: "copy-1-a", Name: "Copy One A", Config: `{"comkun_market_follow":true,"comkun_market_source_strategy_id":"` + sourceOne + `","comkun_market_listing_strategy_id":"listing-1"}`},
		{ID: "copy-1-b", Name: "Copy One B", Config: `{"comkun_market_follow":true,"comkun_market_source_strategy_id":"` + sourceOne + `","comkun_market_listing_strategy_id":"listing-1"}`},
		{ID: "copy-2", Name: "Copy Two", Config: `{"comkun_market_follow":true,"comkun_market_source_strategy_id":"` + sourceTwo + `","comkun_market_listing_strategy_id":"listing-2"}`},
	}
	if err := db.Create(&strategies).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]User{
		{ID: "u1", Email: "u1@example.invalid"},
		{ID: "u2", Email: "u2@example.invalid"},
		{ID: "u3", Email: "u3@example.invalid"},
		{ID: "u4", Email: "u4@example.invalid"},
		{ID: "u5", Email: "u5@example.invalid"},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]Exchange{
		{ID: "ex-enabled", UserID: "u1", ExchangeType: "hz", Enabled: true},
		{ID: "ex-disabled", UserID: "u3", ExchangeType: "hz", Enabled: false},
		{ID: "ex-enabled-2", UserID: "u5", ExchangeType: "hz", Enabled: true},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]Trader{
		{ID: "t1", UserID: "u1", StrategyID: "copy-1-a", ExchangeID: "ex-enabled", IsRunning: true},
		{ID: "t2", UserID: "u1", StrategyID: "copy-1-b", ExchangeID: "ex-enabled", IsRunning: true},
		{ID: "t3", UserID: "u2", StrategyID: "copy-1-a", ExchangeID: "ex-enabled", IsRunning: true},
		{ID: "t4", UserID: "u3", StrategyID: "copy-1-a", ExchangeID: "ex-enabled", IsRunning: false},
		{ID: "t5", UserID: "u3", StrategyID: "copy-1-b", ExchangeID: "ex-disabled", IsRunning: true},
		{ID: "t6", UserID: "u4", StrategyID: "copy-1-a", ExchangeID: "ex-enabled", IsRunning: true},
		{ID: "t7", UserID: "u5", StrategyID: "copy-2", ExchangeID: "ex-enabled-2", IsRunning: true},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]StrategyMarketEntitlement{
		{UserID: "u1", StrategyID: "listing-1", SubscriptionUntil: &activeUntil},
		{UserID: "u2", StrategyID: "listing-1", SubscriptionUntil: &activeUntil},
		{UserID: "u3", StrategyID: "listing-1", SubscriptionUntil: &activeUntil},
		{UserID: "u4", StrategyID: "listing-1", SubscriptionUntil: &expiredUntil},
		{UserID: "u5", StrategyID: "listing-2", SubscriptionUntil: nil},
	}).Error; err != nil {
		t.Fatal(err)
	}

	rows, err := NewTraderStore(db).ListComkunFollowingStats()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%+v, want two listing strategies", rows)
	}
	if rows[0].StrategyID != "listing-1" || rows[0].RunningFollowerCount != 2 || rows[0].RunningTraderCount != 3 || rows[0].SubscribedFollowerCount != 3 {
		t.Fatalf("listing-1 stats=%+v, want users=2 traders=3 subscribed=3", rows[0])
	}
	if rows[1].StrategyID != "listing-2" || rows[1].RunningFollowerCount != 1 || rows[1].RunningTraderCount != 1 || rows[1].SubscribedFollowerCount != 1 {
		t.Fatalf("listing-2 stats=%+v, want users=1 traders=1 subscribed=1", rows[1])
	}
	totals, err := NewTraderStore(db).ComkunFollowingStatsTotals(rows)
	if err != nil {
		t.Fatal(err)
	}
	if totals.RunningFollowerCount != 3 || totals.RunningTraderCount != 4 || totals.SubscribedFollowerCount != 4 {
		t.Fatalf("totals=%+v, want users=3 traders=4 subscribed=4", totals)
	}
}

func TestComkunMasterSourceMappingUsesStableConfiguredIDs(t *testing.T) {
	t.Setenv("COMKUN_OBSERVER_LIVE_MASTER_IDS", "master-1,master-2")
	mapping, err := ComkunMasterSourceMapping()
	if err != nil {
		t.Fatal(err)
	}
	if mapping[HZMasterSourceStrategyID("master-1")] != "master-1" || mapping[HZMasterSourceStrategyID("master-2")] != "master-2" {
		t.Fatalf("mapping=%v, want stable source-to-master mapping", mapping)
	}
}

func TestComkunActiveMasterSourceMappingFailsClosedAndKeepsFullMappingSeparate(t *testing.T) {
	t.Setenv("COMKUN_OBSERVER_LIVE_MASTER_IDS", "master-1,master-2")
	t.Setenv("COMKUN_OBSERVER_PUBLISH_MASTER_IDS", "master-2")
	active, err := ComkunActiveMasterSourceMapping()
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[HZMasterSourceStrategyID("master-2")] != "master-2" {
		t.Fatalf("active=%v, want only master-2", active)
	}
	full, err := ComkunMasterSourceMapping()
	if err != nil {
		t.Fatal(err)
	}
	if len(full) != 2 {
		t.Fatalf("full=%v, want both stable masters", full)
	}

	t.Setenv("COMKUN_OBSERVER_PUBLISH_MASTER_IDS", "")
	if _, err := ComkunActiveMasterSourceMapping(); err == nil {
		t.Fatal("missing publisher allowlist must fail closed")
	}
	t.Setenv("COMKUN_OBSERVER_PUBLISH_MASTER_IDS", "unconfigured-master")
	if _, err := ComkunActiveMasterSourceMapping(); err == nil {
		t.Fatal("publisher allowlist outside stable mapping must fail closed")
	}
}
