package trader

import (
	"testing"
	"time"

	"nofx/kernel"
)

func TestHZExternalOpenTTLAndCloseOnlyPolicy(t *testing.T) {
	now := time.Date(2026, 8, 1, 1, 0, 0, 0, time.UTC)
	if !hzMirrorDeltaAllowed(1, now.Add(-119*time.Second), now, false) {
		t.Fatal("fresh open should be allowed")
	}
	if hzMirrorDeltaAllowed(1, now.Add(-121*time.Second), now, false) {
		t.Fatal("open older than two minutes must not be replayed")
	}
	if !hzMirrorDeltaAllowed(-1, now.Add(-24*time.Hour), now, true) {
		t.Fatal("reduce/close must always be allowed")
	}
	if hzMirrorDeltaAllowed(1, now, now, true) {
		t.Fatal("close-only must reject risk increase")
	}
}

func TestHZMasterLotsUseDynamicContractAndEquityRatio(t *testing.T) {
	position := kernel.PositionInfo{Symbol: "BTC-PERP", Side: "long", Lots: 2, Quantity: 0}
	got, err := hzScaledPositionQuantity(position, 1000, 250, func(_ string, lots float64) (float64, error) {
		return lots * 0.01, nil
	})
	if err != nil || got != 0.005 {
		t.Fatalf("got=%v err=%v", got, err)
	}
}
