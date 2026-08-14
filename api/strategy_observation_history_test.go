package api

import (
	"encoding/json"
	"strings"
	"testing"

	"nofx/marketdata/observation"
	"nofx/store"

	"github.com/gin-gonic/gin"
)

func TestBuildObservationMarketData(t *testing.T) {
	data, err := buildObservationMarketData(observation.StrategyID(1), true)
	if err != nil {
		t.Fatal(err)
	}
	if data == nil || data.Rollup == nil {
		t.Fatal("missing observation market data")
	}
	if data.Rollup.Stats.TotalTrades != 645 || len(data.TradeHistory) != 645 {
		t.Fatalf("trade totals = %d/%d", data.Rollup.Stats.TotalTrades, len(data.TradeHistory))
	}
	if data.InitialBalance != 10000 || data.CurrentBalance != 52947.41 {
		t.Fatalf("balances = %.2f/%.2f", data.InitialBalance, data.CurrentBalance)
	}
	if data.Rollup.Stats.TotalFee <= 0 {
		t.Fatalf("fees were not aggregated: %.2f", data.Rollup.Stats.TotalFee)
	}
	if data.CompletedMonths != 8 || data.MonthlyRows != 9 {
		t.Fatalf("months = %d/%d", data.CompletedMonths, data.MonthlyRows)
	}
	if data.TradeHistory[0]["performance_source"] != observation.StatusHistoricalSimulation {
		t.Fatalf("history disclosure missing: %#v", data.TradeHistory[0])
	}
}

func TestObservationListOverlayIsAuditable(t *testing.T) {
	item := gin.H{"stats": gin.H{}}
	applied, err := applyObservationMarketListOverlay(item, observation.StrategyID(6))
	if err != nil {
		t.Fatal(err)
	}
	if !applied || item["performance_source"] != observation.StatusHistoricalSimulation || item["realtime_follow_available"] != false {
		t.Fatalf("overlay metadata = %#v", item)
	}
	stats := item["stats"].(gin.H)
	if stats["data_complete"] != true || stats["completed_months"] != 3 || stats["monthly_rows"] != 4 {
		t.Fatalf("overlay stats = %#v", stats)
	}
}

func TestPublicItemExposesHistoricalOnlyBoundary(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("zh")
	cfg.MarketPerformanceOnly = true
	cfg.MarketPerformanceSource = observation.StatusHistoricalSimulation
	cfg.MarketPerformanceDisclosure = "历史行情场景数据"
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	item := buildPublicStrategyItem(&store.Strategy{
		ID: observation.StrategyID(1), Name: "稳衡一号", Config: string(raw), MarketAccess: store.MarketAccessPublic,
	}, nil, publicStrategyMarketStats{})
	if item["performance_only"] != true || item["realtime_follow_available"] != false ||
		!strings.Contains(item["performance_disclosure"].(string), "历史行情") {
		t.Fatalf("public item boundary = %#v", item)
	}
	if strings.HasPrefix(observation.StrategyID(1), store.HZMasterSourcePrefix) {
		t.Fatal("history strategy must not be a live HZ master route")
	}
}
