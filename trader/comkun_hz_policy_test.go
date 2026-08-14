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

func TestHZInitialMarginRatioTargets(t *testing.T) {
	price := func(string) (float64, error) { return 50, nil }
	tests := []struct {
		name       string
		masterEq   float64
		followerEq float64
		margin     float64
		positions  bool
		wantQty    float64
		wantErr    bool
	}{
		{name: "100/10 -> 1000/100", masterEq: 100, followerEq: 1000, margin: 10, positions: true, wantQty: 20},
		{name: "same equity", masterEq: 100, followerEq: 100, margin: 10, positions: true, wantQty: 2},
		{name: "reduce to four percent", masterEq: 100, followerEq: 1000, margin: 4, positions: true, wantQty: 8},
		{name: "close", masterEq: 100, followerEq: 1000, positions: false, wantQty: 0},
		{name: "invalid master equity", masterEq: 0, followerEq: 1000, margin: 10, positions: true, wantErr: true},
		{name: "invalid follower equity", masterEq: 100, followerEq: 0, margin: 10, positions: true, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wire := &comkunMasterStateWire{}
			if test.positions {
				wire.Positions = []kernel.PositionInfo{{
					Symbol: "BTC-PERP", Side: "long", Leverage: 10,
					MarginMode: "cross", MarginUsed: test.margin,
				}}
			}
			target, err := buildHZInitialMarginTargetFromWire(wire, test.masterEq, test.followerEq, price)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected fail-closed sizing error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := target[posKey("BTC-PERP", "long")]
			if math.Abs(got-test.wantQty) > 1e-12 {
				t.Fatalf("target quantity=%.12f want=%.12f", got, test.wantQty)
			}
		})
	}
}
