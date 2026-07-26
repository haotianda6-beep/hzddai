package binance

import (
	"testing"

	binancefutures "github.com/adshao/go-binance/v2/futures"
)

func TestLeverageForTargetNotional(t *testing.T) {
	brackets := []binancefutures.Bracket{
		{NotionalFloor: 0, NotionalCap: 10_000, InitialLeverage: 20},
		{NotionalFloor: 10_000, NotionalCap: 50_000, InitialLeverage: 10},
		{NotionalFloor: 50_000, NotionalCap: 250_000, InitialLeverage: 5},
	}
	for _, tc := range []struct {
		name      string
		requested int
		notional  float64
		want      int
	}{
		{name: "keep requested leverage inside first bracket", requested: 20, notional: 9_000, want: 20},
		{name: "lower leverage for larger XAU position", requested: 20, notional: 17_440, want: 10},
		{name: "never raise user leverage", requested: 8, notional: 17_440, want: 8},
		{name: "use deepest matching bracket", requested: 20, notional: 80_000, want: 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := leverageForTargetNotional(tc.requested, tc.notional, brackets); got != tc.want {
				t.Fatalf("leverage=%d, want %d", got, tc.want)
			}
		})
	}
}

func TestBinanceMaximumPositionError(t *testing.T) {
	if !isMaximumPositionAtLeverageError(assertError("code=-2027 Exceeded the maximum allowable position at current leverage")) {
		t.Fatal("expected -2027 to be recognized")
	}
}

type assertError string

func (e assertError) Error() string { return string(e) }
