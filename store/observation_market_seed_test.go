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
	t.Setenv("COMKUN_OBSERVER_MARKET_ENABLED", "true")
	st, err := NewFromGorm(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureObservationMarketSeeds(); err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureObservationMarketSeeds(); err != nil {
		t.Fatalf("second seed failed: %v", err)
	}

	var count int64
	if err := db.Model(&Strategy{}).Where("id LIKE ?", "comkun-observation-history-%").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 6 {
		t.Fatalf("seed count = %d", count)
	}
	for slot := 1; slot <= 6; slot++ {
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
		if !strings.Contains(strategy.Description, "非实盘") {
			t.Fatalf("slot %d disclosure missing", slot)
		}
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
