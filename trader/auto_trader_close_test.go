package trader

import "testing"

type closeTrackingTrader struct {
	Trader
	closed bool
}

func (t *closeTrackingTrader) Close() { t.closed = true }

func TestAutoTraderCloseClosesUnderlyingTrader(t *testing.T) {
	underlying := &closeTrackingTrader{}
	autoTrader := &AutoTrader{trader: underlying}

	autoTrader.Close()

	if !underlying.closed {
		t.Fatal("underlying trader was not closed")
	}
}
