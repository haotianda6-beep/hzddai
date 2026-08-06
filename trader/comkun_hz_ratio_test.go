package trader

import (
	"testing"

	"nofx/store"
)

func TestHZFixedFollowerEquityRatio(t *testing.T) {
	cfg := &store.StrategyConfig{ComkunMirrorFollowerEquityRatio: 0.5}
	got := mirrorFollowerTargetEquity(true, cfg, store.HZMasterSourceStrategyID("master"), 100000, 100000, 100000)
	if got != 50000 {
		t.Fatalf("fixed HZ ratio equity = %v, want 50000", got)
	}
}

func TestHZFollowerEquityRatioDoesNotChangeOtherMirrors(t *testing.T) {
	cfg := &store.StrategyConfig{ComkunMirrorFollowerEquityRatio: 0.2}
	if got := mirrorFollowerTargetEquity(false, cfg, "other", 9000, 10000, 100000); got != 10000 {
		t.Fatalf("non-HZ sizing equity changed = %v, want 10000", got)
	}
	if got := mirrorFollowerTargetEquity(true, &store.StrategyConfig{}, store.HZMasterSourceStrategyID("master"), 9000, 10000, 100000); got != 10000 {
		t.Fatalf("default HZ sizing equity = %v, want 10000", got)
	}
}
