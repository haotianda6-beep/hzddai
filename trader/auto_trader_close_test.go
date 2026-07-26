package trader

import (
	"sync"
	"testing"
	"time"
)

type closeTrackingTrader struct {
	Trader
	closed chan struct{}
	once   sync.Once
}

func newCloseTrackingTrader() *closeTrackingTrader {
	return &closeTrackingTrader{closed: make(chan struct{})}
}

func (t *closeTrackingTrader) Close() {
	t.once.Do(func() { close(t.closed) })
}

func TestAutoTraderCloseClosesUnderlyingTrader(t *testing.T) {
	underlying := newCloseTrackingTrader()
	autoTrader := &AutoTrader{trader: underlying}

	autoTrader.Close()

	select {
	case <-underlying.closed:
	default:
		t.Fatal("underlying trader was not closed")
	}
}

func TestAutoTraderStopAsyncEventuallyClosesUnderlyingTrader(t *testing.T) {
	underlying := newCloseTrackingTrader()
	autoTrader := &AutoTrader{trader: underlying}

	autoTrader.StopAsync()

	select {
	case <-underlying.closed:
	case <-time.After(time.Second):
		t.Fatal("asynchronous stop did not close the underlying trader")
	}
}
