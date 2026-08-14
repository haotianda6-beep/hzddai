package trader

import (
	"testing"

	"nofx/store"
)

func TestHZSizingUsesCurrentFollowerEquity(t *testing.T) {
	got := mirrorFollowerTargetEquity(store.HZMasterSourceStrategyID("master"), 100000, 100000)
	if got != 100000 {
		t.Fatalf("HZ sizing equity = %v, want current follower equity 100000", got)
	}
}

func TestHZFollowerEquityRatioDoesNotChangeOtherMirrors(t *testing.T) {
	if got := mirrorFollowerTargetEquity("other", 9000, 10000); got != 10000 {
		t.Fatalf("non-HZ sizing equity changed = %v, want 10000", got)
	}
	if got := mirrorFollowerTargetEquity(store.HZMasterSourceStrategyID("master"), 9000, 10000); got != 10000 {
		t.Fatalf("default HZ sizing equity = %v, want 10000", got)
	}
}
