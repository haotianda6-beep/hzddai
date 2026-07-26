package api

import (
	"testing"
	"time"

	tradertypes "nofx/trader/types"
)

func TestFilterGateAggregateClosedPnLPrefersDetailedFills(t *testing.T) {
	openTime := time.Date(2026, 7, 8, 7, 45, 42, 0, time.UTC)
	firstClose := time.Date(2026, 7, 8, 8, 20, 12, 0, time.UTC)
	finalClose := time.Date(2026, 7, 8, 14, 20, 42, 0, time.UTC)

	records := []tradertypes.ClosedPnLRecord{
		{
			Symbol:     "BTCUSDT",
			Side:       "SHORT",
			Quantity:   0.0207,
			ExitPrice:  62374.80,
			EntryTime:  openTime,
			ExitTime:   finalClose,
			CloseType:  "position_close",
			ExchangeID: "gate_close_BTC_USDT_SHORT_1783520442_0.0207",
		},
		{
			Symbol:    "BTCUSDT",
			Side:      "SHORT",
			Quantity:  0.0158,
			ExitPrice: 62483.00,
			EntryTime: openTime,
			ExitTime:  firstClose,
			CloseType: "partial_close",
		},
		{
			Symbol:    "BTCUSDT",
			Side:      "SHORT",
			Quantity:  0.0049,
			ExitPrice: 62025.90,
			EntryTime: finalClose.Add(-time.Minute),
			ExitTime:  finalClose,
			CloseType: "partial_close",
		},
	}

	filtered := filterGateAggregateClosedPnL(records)
	if len(filtered) != 2 {
		t.Fatalf("expected aggregate row removed, got %d records", len(filtered))
	}
	for _, record := range filtered {
		if record.CloseType == "position_close" {
			t.Fatalf("aggregate position_close row was not removed: %+v", record)
		}
	}
}
