package store

import (
	"encoding/json"
	"strings"
	"testing"

	"nofx/marketdata/observation"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEnsureObservationMarketSeeds(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:observation-seed?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&User{}, &Strategy{}); err != nil {
		t.Fatal(err)
	}
	owner := User{ID: "owner", Email: comkunFollowListingOwnerEmail, DisplayName: "COMKUNAI"}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMKUN_OBSERVER_MARKET_ENABLED", "false")
	t.Setenv(observationLiveMasterIDsEnv, "")
	st, err := NewFromGorm(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMKUN_OBSERVER_MARKET_ENABLED", "true")
	if err := st.EnsureObservationMarketSeeds(); err != nil {
		t.Fatal(err)
	}

	var count int64
	if err := db.Model(&Strategy{}).Where("id LIKE ?", "comkun-observation-history-%").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("seed count = %d", count)
	}
	for slot := 1; slot <= 3; slot++ {
		var strategy Strategy
		if err := db.First(&strategy, "id = ?", observation.StrategyID(slot)).Error; err != nil {
			t.Fatal(err)
		}
		if strategy.MarketAccess != MarketAccessPublic || !strategy.IsPublic || strategy.ConfigVisible {
			t.Fatalf("slot %d listing flags are invalid", slot)
		}
		var cfg StrategyConfig
		if err := json.Unmarshal([]byte(strategy.Config), &cfg); err != nil {
			t.Fatal(err)
		}
		if cfg.ComkunMarketFollow || cfg.ComkunFollowListingTemplate || !cfg.MarketPerformanceOnly ||
			cfg.MarketPerformanceSource != observation.StatusHistoricalSimulation || cfg.MarketRealtimeFollowAvailable {
			t.Fatalf("slot %d execution boundary is invalid", slot)
		}
		profile, _ := observation.MarketProfileBySlot(slot)
		if strategy.Name != profile.Name || strategy.Description != profile.Description ||
			cfg.MarketSalePriceUSDT != profile.MonthlyPriceUSDT || !cfg.MarketSubscriptionMonthlyOnly {
			t.Fatalf("slot %d market profile mismatch", slot)
		}
	}
	if err := st.EnsureObservationMarketSeeds(); err != nil {
		t.Fatalf("second seed failed: %v", err)
	}
}

func TestEnsureObservationMarketSeedsEnablesThreeIndependentLiveRoutes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:observation-live-seed?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&User{}, &Strategy{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&User{ID: "owner-live", Email: comkunFollowListingOwnerEmail}).Error; err != nil {
		t.Fatal(err)
	}
	t.Setenv(observationMarketEnabledEnv, "false")
	st, err := NewFromGorm(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(observationMarketEnabledEnv, "true")
	t.Setenv(observationLiveMasterIDsEnv, "master-1,master-2,master-3")
	if err := st.EnsureObservationMarketSeeds(); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for slot := 1; slot <= 3; slot++ {
		var strategy Strategy
		if err := db.First(&strategy, "id = ?", observation.StrategyID(slot)).Error; err != nil {
			t.Fatal(err)
		}
		var cfg StrategyConfig
		if err := json.Unmarshal([]byte(strategy.Config), &cfg); err != nil {
			t.Fatal(err)
		}
		wantSource := HZMasterSourceStrategyID("master-" + string(rune('0'+slot)))
		if strategy.MarketAccess != MarketAccessSubscription || !strategy.IsPublic || strategy.ConfigVisible ||
			cfg.MarketPerformanceOnly || !cfg.MarketRealtimeFollowAvailable || !cfg.ComkunFollowListingTemplate ||
			cfg.ComkunMarketFollow || cfg.ComkunMarketSourceStrategyID != wantSource {
			t.Fatalf("slot %d live route invalid: access=%s cfg=%+v", slot, strategy.MarketAccess, cfg)
		}
		if seen[cfg.ComkunMarketSourceStrategyID] {
			t.Fatalf("slot %d shares a live route", slot)
		}
		seen[cfg.ComkunMarketSourceStrategyID] = true
	}
}

func TestHistoricalProfileCannotBindTrader(t *testing.T) {
	cfg := GetDefaultStrategyConfig("zh")
	cfg.MarketPerformanceOnly = true
	cfg.MarketPerformanceSource = observation.StatusHistoricalSimulation
	if err := ValidateStrategyExchange("hz", &cfg); err == nil || !strings.Contains(err.Error(), "历史行情场景") {
		t.Fatalf("ValidateStrategyExchange() error = %v", err)
	}
}
