package trader

import "testing"

func TestMT4MirrorUsesFixedInitialBalance(t *testing.T) {
	if got := mirrorFollowerSizingEquity("mt4-ea-gold-master", 5000, 4200); got != 5000 {
		t.Fatalf("MT4 sizing equity=%f, want fixed 5000", got)
	}
	if got := mirrorFollowerSizingEquity("other-strategy", 5000, 4200); got != 4200 {
		t.Fatalf("regular mirror sizing equity=%f, want current 4200", got)
	}
}
