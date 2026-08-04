package main

import (
	"testing"

	"nofx/trader/types"
)

func TestTradeEvidenceCountsUniqueOrders(t *testing.T) {
	got := tradeEvidence([]types.ClosedPnLRecord{
		{OrderID: "open", Quantity: 0.01},
		{OrderID: "close", Quantity: 0.01, RealizedPnL: 1.25},
		{OrderID: "close", Quantity: 0.005, RealizedPnL: 0.5},
	})
	want := "trades=3 orders=2 quantity=0.025000 realized_pnl=1.750000"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
