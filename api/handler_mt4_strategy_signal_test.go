package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"nofx/kernel"
)

func TestMT4StrategySignalBuildsScaledSnapshot(t *testing.T) {
	t.Setenv("MT4_STRATEGY_STATE_PATH", filepath.Join(t.TempDir(), "mt4_state.json"))
	t.Setenv("MT4_STRATEGY_GOLD_SYMBOL", "PAXGUSDT")
	t.Setenv("MT4_STRATEGY_LEVERAGE", "20")

	openA := mt4StrategySignalReq{
		Action:       "OPEN",
		Symbol:       "XAUUSD",
		Side:         "BUY",
		MT4Type:      0,
		Volume:       0.05,
		Price:        2400,
		MasterEquity: 10000,
		Ticket:       101,
	}
	state, changed, active, previous, err := mt4StrategyApplySignal(openA)
	if err != nil || !changed || !active {
		t.Fatalf("open A changed=%v active=%v err=%v", changed, active, err)
	}
	if previous != nil {
		t.Fatalf("new open previous=%+v, want nil", previous)
	}
	openB := openA
	openB.Volume = 0.01
	openB.Ticket = 102
	state, changed, active, _, err = mt4StrategyApplySignal(openB)
	if err != nil || !changed || !active {
		t.Fatalf("open B changed=%v active=%v err=%v", changed, active, err)
	}

	event := mt4StrategyBuildSignalEvent(openB, nil, state)
	raw, count, err := mt4StrategyBuildMasterStateJSON(state, 10000, 0, &event)
	if err != nil {
		t.Fatal(err)
	}
	var wire mt4StrategyMasterStateWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if count != 2 || len(wire.Positions) != 2 {
		t.Fatalf("positions count=%d len=%d", count, len(wire.Positions))
	}
	if wire.Positions[0].Symbol != "PAXGUSDT" || wire.Positions[0].Quantity != 5 {
		t.Fatalf("first position = %#v", wire.Positions[0])
	}
	if wire.Positions[1].Quantity != 1 {
		t.Fatalf("second quantity = %f", wire.Positions[1].Quantity)
	}
	if wire.MT4Event == nil || wire.MT4Event.Action != "OPEN" || wire.MT4Event.Ticket != 102 || wire.MT4Event.MasterQuantity != 1 {
		t.Fatalf("mt4 event = %#v", wire.MT4Event)
	}

	closeA := openA
	closeA.Action = "CLOSE"
	closeA.ClosePrice = 2412.5
	state, changed, active, previous, err = mt4StrategyApplySignal(closeA)
	if err != nil || !changed || !active {
		t.Fatalf("close A changed=%v active=%v err=%v", changed, active, err)
	}
	closeEvent := mt4StrategyBuildSignalEvent(closeA, previous, state)
	raw, count, err = mt4StrategyBuildMasterStateJSON(state, 10000, 0, &closeEvent)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("after close count=%d raw=%s", count, raw)
	}
	if closeEvent.Action != "CLOSE" || closeEvent.MasterQuantity != 5 || closeEvent.Price != 2412.5 {
		t.Fatalf("close event=%+v", closeEvent)
	}
	if _, err := os.Stat(os.Getenv("MT4_STRATEGY_STATE_PATH")); err != nil {
		t.Fatal(err)
	}
}

func TestMT4StrategySignalIgnoresPendingOrders(t *testing.T) {
	t.Setenv("MT4_STRATEGY_STATE_PATH", filepath.Join(t.TempDir(), "mt4_state.json"))
	_, changed, active, _, err := mt4StrategyApplySignal(mt4StrategySignalReq{
		Action:  "OPEN",
		Symbol:  "XAUUSD",
		Side:    "BUY_LIMIT",
		MT4Type: 2,
		Volume:  0.01,
		Ticket:  201,
	})
	if err != nil || changed || active {
		t.Fatalf("pending changed=%v active=%v err=%v", changed, active, err)
	}
}

func TestMT4StrategyClassifiesPartialCloseAsReduce(t *testing.T) {
	t.Setenv("MT4_STRATEGY_STATE_PATH", filepath.Join(t.TempDir(), "mt4_state.json"))
	open := mt4StrategySignalReq{Action: "OPEN", Symbol: "XAUUSD", MT4Type: 0, Volume: 0.05, Price: 2400, Ticket: 301}
	_, _, _, _, err := mt4StrategyApplySignal(open)
	if err != nil {
		t.Fatal(err)
	}
	open.Volume = 0.02
	open.MT4Bid = 2410
	state, changed, active, previous, err := mt4StrategyApplySignal(open)
	if err != nil || !changed || !active || previous == nil {
		t.Fatalf("partial close changed=%v active=%v previous=%+v err=%v", changed, active, previous, err)
	}
	event := mt4StrategyBuildSignalEvent(open, previous, state)
	if event.Action != "REDUCE" || !almostEqual(event.Volume, 0.03, 1e-9) || !almostEqual(event.MasterQuantity, 3, 1e-9) || event.PositionVolume != 0.02 {
		t.Fatalf("reduce event=%+v", event)
	}
}

func TestMT4StrategyUsesReportedMasterEquity(t *testing.T) {
	t.Setenv("MT4_STRATEGY_MASTER_EQUITY", "")
	if got := mt4StrategyMasterEquity(101.56); got != 101.56 {
		t.Fatalf("reported master equity = %f, want 101.56", got)
	}
	t.Setenv("MT4_STRATEGY_MASTER_EQUITY", "20000")
	if got := mt4StrategyMasterEquity(101.56); got != 20000 {
		t.Fatalf("env master equity baseline = %f, want 20000", got)
	}
}

func TestMT4StrategyEmbedsActualMarginUsed(t *testing.T) {
	state := mt4StrategySignalState{Orders: map[string]mt4StrategyTrackedOrder{
		"1": {Ticket: 1, ExchangeSymbol: "XAUUSDT", Side: "long", Volume: 0.01, Price: 4000},
		"2": {Ticket: 2, ExchangeSymbol: "XAUUSDT", Side: "long", Volume: 0.02, Price: 4000},
	}}
	raw, _, err := mt4StrategyBuildMasterStateJSON(state, 100, 0.8, nil)
	if err != nil {
		t.Fatal(err)
	}
	var wire mt4StrategyMasterStateWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.MirrorMargin == nil || wire.MirrorMargin.MasterMarginUsed != 0.8 {
		t.Fatalf("mirror margin = %#v", wire.MirrorMargin)
	}
	var total float64
	for _, position := range wire.Positions {
		total += position.MarginUsed
	}
	if !almostEqual(total, 0.8, 1e-9) {
		t.Fatalf("allocated margin = %.8f, want 0.8", total)
	}
}

func TestMT4StrategyFullSyncReplacesStaleTickets(t *testing.T) {
	t.Setenv("MT4_STRATEGY_STATE_PATH", filepath.Join(t.TempDir(), "mt4_state.json"))
	for _, req := range []mt4StrategySignalReq{
		{Action: "OPEN", Symbol: "XAUUSDc", MT4Type: 0, Volume: 0.04, Price: 4061, Ticket: 1001},
		{Action: "OPEN", Symbol: "XAUUSDc", MT4Type: 1, Volume: 0.09, Price: 4059, Ticket: 1002},
	} {
		if _, _, _, _, err := mt4StrategyApplySignal(req); err != nil {
			t.Fatal(err)
		}
	}

	req := mt4StrategySignalReq{
		Action:       "SYNC",
		SyncComplete: true,
		MasterEquity: 96.3,
		Orders: []mt4StrategySyncOrder{
			{Ticket: 2001, Symbol: "XAUUSDc", MT4Type: 1, Volume: 0.01, Price: 4055},
			{Ticket: 2002, Symbol: "XAUUSDc", MT4Type: 1, Volume: 0.01, Price: 4054},
		},
	}
	state, changed, err := mt4StrategyApplyFullSync(req)
	if err != nil || !changed {
		t.Fatalf("full sync changed=%v err=%v", changed, err)
	}
	if len(state.Orders) != 2 || state.Orders["2001"].Side != "short" || state.Orders["2002"].Side != "short" {
		t.Fatalf("full sync state=%+v", state.Orders)
	}
	if _, ok := state.Orders["1001"]; ok {
		t.Fatal("stale long ticket was not removed")
	}
	if _, changed, err := mt4StrategyApplyFullSync(req); err != nil || changed {
		t.Fatalf("identical sync changed=%v err=%v", changed, err)
	}
}

func TestMT4StrategyFullSyncOnlyClearsOnCompleteSnapshot(t *testing.T) {
	t.Setenv("MT4_STRATEGY_STATE_PATH", filepath.Join(t.TempDir(), "mt4_state.json"))
	open := mt4StrategySignalReq{Action: "OPEN", Symbol: "XAUUSDc", MT4Type: 0, Volume: 0.01, Price: 4050, Ticket: 3001}
	if _, _, _, _, err := mt4StrategyApplySignal(open); err != nil {
		t.Fatal(err)
	}

	if _, _, err := mt4StrategyApplyFullSync(mt4StrategySignalReq{Action: "SYNC"}); err == nil {
		t.Fatal("incomplete sync must be rejected")
	}
	state, err := mt4StrategyLoadState()
	if err != nil || len(state.Orders) != 1 {
		t.Fatalf("incomplete sync modified state=%+v err=%v", state.Orders, err)
	}

	state, changed, err := mt4StrategyApplyFullSync(mt4StrategySignalReq{Action: "SYNC", SyncComplete: true})
	if err != nil || !changed || len(state.Orders) != 0 {
		t.Fatalf("complete empty sync state=%+v changed=%v err=%v", state.Orders, changed, err)
	}
}

func TestMT4StrategyEstimatesGrossMarginForHedgedBook(t *testing.T) {
	previous := mt4StrategyMasterStateWire{
		Positions: []kernel.PositionInfo{{Quantity: 8}},
		MirrorMargin: &mt4StrategyMirrorMargin{
			MasterMarginUsed: 0.32,
		},
	}
	current := mt4StrategySignalState{Orders: map[string]mt4StrategyTrackedOrder{
		"1": {Volume: 0.08},
		"2": {Volume: 0.08},
	}}
	if got := mt4StrategyEstimateMarginFromWire(current, previous); !almostEqual(got, 0.64, 1e-9) {
		t.Fatalf("estimated gross margin = %.8f, want 0.64", got)
	}
}
