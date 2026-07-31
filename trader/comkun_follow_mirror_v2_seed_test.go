package trader

import (
	"fmt"
	"math"
	"testing"

	"nofx/store"
)

type mirrorTestQuantityFormatter struct{}

func (mirrorTestQuantityFormatter) FormatQuantity(_ string, quantity float64) (string, error) {
	return fmt.Sprintf("%.3f", math.Floor(quantity*1000+1e-9)/1000), nil
}

func TestMirrorStartupBaselineCheckpointRoundTrip(t *testing.T) {
	want := map[string]float64{
		"XAUUSDT|long":  0.081,
		"XAUUSDT|short": 0.02,
	}

	checkpoint := encodeMirrorStartupBaselineCheckpoint(want)
	got, ok := decodeMirrorStartupBaselineCheckpoint(checkpoint)
	if !ok {
		t.Fatalf("expected valid baseline checkpoint, got %q", checkpoint)
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d baseline legs, got %d", len(want), len(got))
	}
	for key, qty := range want {
		if got[key] != qty {
			t.Fatalf("expected %s quantity %.8f, got %.8f", key, qty, got[key])
		}
	}

	if checkpoint := encodeMirrorStartupBaselineCheckpoint(nil); checkpoint != "" {
		t.Fatalf("expected empty checkpoint without active baseline, got %q", checkpoint)
	}
	if _, ok := decodeMirrorStartupBaselineCheckpoint("unrelated"); ok {
		t.Fatal("unrelated consumption metadata must not restore a baseline")
	}
}

func TestMirrorStartupBaselineCheckpointPersistsForOKX(t *testing.T) {
	at := &AutoTrader{
		config: AutoTraderConfig{StrategyConfig: &store.StrategyConfig{
			ComkunMarketFollow:           true,
			ComkunMarketSourceStrategyID: "okx-screen-mirror-02",
		}},
	}
	if !at.shouldPersistMirrorStartupBaseline() {
		t.Fatal("OKX startup baseline must survive trader and backend restarts")
	}
}

func TestMirrorStartupBaselineCheckpointPersistsForHZ(t *testing.T) {
	at := &AutoTrader{
		exchange: "hz",
		config: AutoTraderConfig{StrategyConfig: &store.StrategyConfig{
			ComkunMarketFollow:           true,
			ComkunMarketSourceStrategyID: store.HZMasterSourceStrategyID("master-1"),
		}},
	}
	if !at.shouldPersistMirrorStartupBaseline() {
		t.Fatal("HZ startup baseline must survive restarts so existing master exposure is never chased")
	}
}

func TestMirrorSeedAdjustedTarget(t *testing.T) {
	at := &AutoTrader{
		mirrorSeedBaselineQty: map[string]float64{
			"SOLUSDT|long": 1.0,
		},
	}
	k := "SOLUSDT|long"

	target, _ := at.mirrorSeedAdjustedTarget(k, 1.0)
	if target != 0 {
		t.Fatalf("expected zero target for startup baseline, got %.8f", target)
	}

	target, _ = at.mirrorSeedAdjustedTarget(k, 2.0)
	if target < 0.999 || target > 1.001 {
		t.Fatalf("expected only the new excess above startup baseline, got %.8f", target)
	}

	target, _ = at.mirrorSeedAdjustedTarget(k, 1.01)
	if target != 0 {
		t.Fatalf("expected tiny baseline drift to stay ignored, got %.8f", target)
	}

	at.mirrorSeedBaselineQty = map[string]float64{k: 1.0}
	target, _ = at.mirrorSeedAdjustedTarget(k, 0)
	if target != 0 {
		t.Fatalf("expected master flat target to remain zero, got %.8f", target)
	}
	if _, ok := at.mirrorSeedBaselineQty[k]; ok {
		t.Fatal("baseline should clear when master closes leg")
	}

	at.mirrorSeedBaselineQty = map[string]float64{"ETHUSDT|long": 0.2}
	target, _ = at.mirrorSeedAdjustedTarget("SOLUSDT|long", 1.0)
	if target != 1.0 {
		t.Fatalf("expected full target for legs not in startup baseline, got %.8f", target)
	}
}

func TestMirrorSeedClearMissingMasterTargetsAllowsReopen(t *testing.T) {
	k := "SOLUSDT|short"
	at := &AutoTrader{
		mirrorSeedBaselineQty: map[string]float64{
			k:               10,
			"ETHUSDT|long":  mirrorSeedBaselineBlocked,
			"BTCUSDT|short": 2,
		},
	}

	cleared := at.mirrorSeedClearMissingMasterTargets(map[string]float64{
		"BTCUSDT|short": 2,
	})
	if len(cleared) != 1 || cleared[0] != k {
		t.Fatalf("expected to clear only missing normal baseline %s, got %#v", k, cleared)
	}
	if _, ok := at.mirrorSeedBaselineQty[k]; ok {
		t.Fatal("missing master leg baseline should be cleared after master flat")
	}
	if _, ok := at.mirrorSeedBaselineQty["ETHUSDT|long"]; !ok {
		t.Fatal("blocked API-error baseline should not be cleared by flat snapshot")
	}

	target, _ := at.mirrorSeedAdjustedTarget(k, 10)
	if target != 10 {
		t.Fatalf("expected full target after same-side reopen, got %.8f", target)
	}
}

func TestMirrorSeedWaitFlatBlocksCurrentAddsThenAllowsNextOpen(t *testing.T) {
	k := "BTCUSDT|short"
	at := &AutoTrader{
		mirrorSeedBaselineQty: map[string]float64{
			k: mirrorSeedBaselineWaitForFlat,
		},
	}

	for _, masterTarget := range []float64{1, 2} {
		target, _ := at.mirrorSeedAdjustedTarget(k, masterTarget)
		if target != 0 {
			t.Fatalf("expected current leg and its adds to stay blocked, got %.8f", target)
		}
	}

	cleared := at.mirrorSeedClearMissingMasterTargets(map[string]float64{})
	if len(cleared) != 1 || cleared[0] != k {
		t.Fatalf("expected wait-flat baseline to clear when master closes, got %#v", cleared)
	}

	target, _ := at.mirrorSeedAdjustedTarget(k, 1)
	if target != 1 {
		t.Fatalf("expected the next master open to be followed in full, got %.8f", target)
	}
}

func TestMirrorFlatConfirmRequired(t *testing.T) {
	if got := mirrorFlatConfirmRequired(false); got != 1 {
		t.Fatalf("api/mt4 mirror close should act on first flat snapshot, got %d", got)
	}
	if got := mirrorFlatConfirmRequired(true); got != mirrorSafetyMasterFlatConfirms {
		t.Fatalf("web mirror close should keep debounce confirms, got %d", got)
	}
}

func TestMirrorMarketTargetMismatchDetectsExtraLeg(t *testing.T) {
	err := mirrorMarketTargetMismatch(map[string]float64{}, map[string]float64{
		"XAUUSDT|long": 0.062,
	}, nil)
	if err == nil {
		t.Fatal("expected mismatch when follower still has a leg after master is flat")
	}
}

func TestMirrorMarketTargetMismatchAcceptsAlignedLeg(t *testing.T) {
	err := mirrorMarketTargetMismatch(
		map[string]float64{"XAUUSDT|short": 0.051},
		map[string]float64{"XAUUSDT|short": 0.051},
		nil,
	)
	if err != nil {
		t.Fatalf("expected aligned target, got %v", err)
	}
}

func TestMirrorMarketTargetMismatchUsesExchangeQuantityFormat(t *testing.T) {
	err := mirrorMarketTargetMismatch(
		map[string]float64{"XAUUSDT|short": 0.09246672},
		map[string]float64{"XAUUSDT|short": 0.092},
		mirrorTestQuantityFormatter{},
	)
	if err != nil {
		t.Fatalf("expected formatted quantities to align, got %v", err)
	}
}
