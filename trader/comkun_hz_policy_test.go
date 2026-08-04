package trader

import (
	"math"
	"testing"
	"time"

	"nofx/kernel"
)

func TestHZExternalFloorsFinalTargetBeforeReduce(t *testing.T) {
	tests := []struct {
		name                   string
		current, rawTarget     float64
		wantTarget, wantReduce float64
	}{
		{"10k", 0.030, 0.020004, 0.020, 0.010},
		{"5k", 0.015, 0.010002, 0.010, 0.005},
		{"1k", 0.003, 0.0020004, 0.002, 0.001},
		{"exact step", 0.030, 0.020, 0.020, 0.010},
		{"close", 0.003, 0, 0, 0.003},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target, err := floorHZMirrorTarget(mirrorTestQuantityFormatter{}, "BTCUSDT", test.rawTarget)
			if err != nil {
				t.Fatal(err)
			}
			reduce := test.current - target
			if math.Abs(target-test.wantTarget) > 1e-12 || math.Abs(reduce-test.wantReduce) > 1e-12 {
				t.Fatalf("target=%.12f reduce=%.12f, want target=%.12f reduce=%.12f", target, reduce, test.wantTarget, test.wantReduce)
			}
		})
	}
}

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

func TestHZFreshPollingReconcileCanRepairMissedOpen(t *testing.T) {
	now := time.Now()
	wire := &comkunMasterStateWire{PollingReconcile: true, OccurredAt: now.Add(-30 * time.Second)}
	if !hzMirrorDeltaAllowed(1, wire.OccurredAt, now, hzMirrorEventCloseOnly(false, wire)) {
		t.Fatal("fresh polling reconcile must repair a missed OPEN within the two-minute TTL")
	}
	if hzMirrorDeltaAllowed(1, wire.OccurredAt, now, hzMirrorEventCloseOnly(true, wire)) {
		t.Fatal("insufficient platform balance must remain close-only")
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
