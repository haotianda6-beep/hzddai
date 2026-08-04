package trader

import (
	"testing"

	"nofx/store"
)

func TestHZFixedFollowerEquityRatio(t *testing.T) {
	cfg := &store.StrategyConfig{ComkunMirrorFollowerEquityRatio: 0.6}
	got := mirrorFollowerTargetEquity(true, cfg, store.HZMasterSourceStrategyID("master"), 9000, 10000, 100000)
	if got != 60000 {
		t.Fatalf("fixed HZ ratio equity = %v, want 60000", got)
	}
}

func TestHZFollowerEquityRatioDefaultsToLiveEquity(t *testing.T) {
	cfg := &store.StrategyConfig{}
	got := mirrorFollowerTargetEquity(true, cfg, store.HZMasterSourceStrategyID("master"), 9000, 10000, 100000)
	if got != 10000 {
		t.Fatalf("default HZ sizing equity = %v, want live equity 10000", got)
	}
	got = mirrorFollowerTargetEquity(false, &store.StrategyConfig{ComkunMirrorFollowerEquityRatio: 0.6}, "other", 9000, 10000, 100000)
	if got != 10000 {
		t.Fatalf("non-HZ sizing equity changed = %v, want 10000", got)
	}
}
