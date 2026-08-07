package api

import (
	"testing"

	"nofx/trader"
)

type intentCloseTrader struct {
	trader.Trader
	intent string
	closed bool
}

func (t *intentCloseTrader) ExecuteWithIntent(intent string, execute func() (map[string]interface{}, error)) (map[string]interface{}, error) {
	t.intent = intent
	return execute()
}

func (t *intentCloseTrader) CloseLong(string, float64) (map[string]interface{}, error) {
	t.closed = true
	return map[string]interface{}{"status": "FILLED"}, nil
}

func TestExecutePositionCloseSuppliesHZIdempotencyIntent(t *testing.T) {
	probe := &intentCloseTrader{}
	result, err := executePositionClose(probe, "NXPCUSDT", "LONG")
	if err != nil || result["status"] != "FILLED" || !probe.closed {
		t.Fatalf("result=%v closed=%v err=%v", result, probe.closed, err)
	}
	if probe.intent == "" {
		t.Fatal("HZ close must receive a stable idempotency intent")
	}
}
