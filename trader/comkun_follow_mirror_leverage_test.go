package trader

import (
	"math"
	"testing"

	"nofx/kernel"
)

func TestMirrorScaledPositionQtyIgnoresLeverageOnlyChange(t *testing.T) {
	mp := kernel.PositionInfo{
		Symbol:     "BTCUSDT",
		Side:       "long",
		Quantity:   0.663,
		EntryPrice: 76716.4,
		MarkPrice:  76935.4,
		MarginUsed: 5096,
		Leverage:   10,
	}
	masterEq := 10000.0
	followerEq := 1000.0

	q10 := mirrorScaledPositionQty(mp, masterEq, followerEq, 10, 20)
	mp.Leverage = 20
	mp.MarginUsed = 2548
	q20 := mirrorScaledPositionQty(mp, masterEq, followerEq, 20, 20)

	if math.Abs(q10-q20) > 1e-12 {
		t.Fatalf("leverage-only change must not change target qty: 10x=%.8f 20x=%.8f", q10, q20)
	}
	want := 0.663 * followerEq / masterEq
	if math.Abs(q10-want) > 1e-12 {
		t.Fatalf("target qty = masterQty*followerEq/masterEq: got %.8f want %.8f", q10, want)
	}
}

func TestMirrorScaledPositionQtyScalesWithQuantityChange(t *testing.T) {
	masterEq := 10000.0
	followerEq := 1000.0
	base := kernel.PositionInfo{
		Symbol: "ETHUSDT", Side: "long", Quantity: 15.597, Leverage: 10,
		MarkPrice: 2100, MarginUsed: 3288,
	}
	mp2 := base
	mp2.Quantity = 23.456

	q1 := mirrorScaledPositionQty(base, masterEq, followerEq, 10, 20)
	q2 := mirrorScaledPositionQty(mp2, masterEq, followerEq, 10, 20)
	ratio := q2 / q1
	wantRatio := 23.456 / 15.597
	if math.Abs(ratio-wantRatio) > 1e-6 {
		t.Fatalf("qty ratio mismatch: got %.6f want %.6f", ratio, wantRatio)
	}
}

func TestMT4TargetUsesActualMasterMargin(t *testing.T) {
	wire := &comkunMasterStateWire{
		Positions: []kernel.PositionInfo{{
			Symbol: "XAUUSDT", Side: "long", Quantity: 33, MarkPrice: 4000, Leverage: 20,
		}},
		MirrorMargin: &ComkunMirrorMarginMeta{MasterMarginUsed: 0.8, MasterMarginLeverage: 20},
	}
	target := buildMT4MasterTargetFromWire(wire, 100, 3000, 20)
	want := 0.8 / 100 * 3000 * 20 / 4000
	if math.Abs(target["XAUUSDT|long"]-want) > 1e-12 {
		t.Fatalf("MT4 target = %.12f, want %.12f", target["XAUUSDT|long"], want)
	}
}
