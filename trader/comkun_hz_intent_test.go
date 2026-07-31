package trader

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"nofx/kernel"
	"nofx/store"
	"nofx/trader/hz"
)

type recoveringHZIntentTrader struct {
	failingBalanceTrader
	orders        map[string]map[string]interface{}
	next          string
	positions     []map[string]interface{}
	balance       float64
	contractSize  float64
	opens, closes int
}

func (trader *recoveringHZIntentTrader) QuantityForLots(_ string, lots float64) (float64, error) {
	contractSize := trader.contractSize
	if contractSize == 0 {
		contractSize = 1
	}
	return lots * contractSize, nil
}

func (trader *recoveringHZIntentTrader) GetBalance() (map[string]interface{}, error) {
	return map[string]interface{}{"total_equity": trader.balance}, nil
}

func (trader *recoveringHZIntentTrader) GetPositions() ([]map[string]interface{}, error) {
	return trader.positions, nil
}

func (trader *recoveringHZIntentTrader) OpenLong(string, float64, int) (map[string]interface{}, error) {
	trader.opens++
	return map[string]interface{}{"orderId": "open-long"}, nil
}

func (trader *recoveringHZIntentTrader) OpenShort(string, float64, int) (map[string]interface{}, error) {
	trader.opens++
	return map[string]interface{}{"orderId": "open-short"}, nil
}

func (trader *recoveringHZIntentTrader) CloseLong(string, float64) (map[string]interface{}, error) {
	trader.closes++
	trader.positions = nil
	return map[string]interface{}{"orderId": "close-long"}, nil
}

func (trader *recoveringHZIntentTrader) CloseShort(string, float64) (map[string]interface{}, error) {
	trader.closes++
	trader.positions = nil
	return map[string]interface{}{"orderId": "close-short"}, nil
}

func (trader *recoveringHZIntentTrader) ExecuteWithIntent(clientOrderID string, execute func() (map[string]interface{}, error)) (map[string]interface{}, error) {
	trader.next = clientOrderID
	defer func() { trader.next = "" }()
	return execute()
}

func TestHZMirrorCloseOnlyBlocksOpenButAlwaysCloses(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "hz-close-only.db"))
	if err != nil {
		t.Fatal(err)
	}
	underlying := &recoveringHZIntentTrader{balance: 250, contractSize: 1}
	at := &AutoTrader{
		id: "trader-1", userID: "user-1", exchangeID: "exchange-1", exchange: "hz",
		initialBalance: 250, store: st, trader: underlying,
		config: AutoTraderConfig{StrategyConfig: &store.StrategyConfig{
			ComkunMarketFollow: true, ComkunMarketSourceStrategyID: store.HZMasterSourceStrategyID("master-1"),
		}},
	}
	ctx := &kernel.Context{Account: kernel.AccountInfo{TotalEquity: 250}}
	openWire := comkunMasterStateWire{
		V: 1, SourceEventID: "event-open", SourceSequence: 1, OccurredAt: time.Now(), EventType: "OPEN",
		Positions:     []kernel.PositionInfo{{PositionID: "master-position", Symbol: "BTC-PERP", Side: "long", Lots: 1, Leverage: 10}},
		PendingOrders: []kernel.PendingOrder{},
	}
	openRaw, _ := json.Marshal(openWire)
	if err := at.reconcileComkunFollowMasterStateV2(ctx, &store.ComkunMasterBroadcast{
		ID: 1, MasterAccountEquity: 1000, MasterStateJSON: string(openRaw),
	}, &store.DecisionRecord{ExecutionLog: []string{}}, false, true); err != nil {
		t.Fatal(err)
	}
	if underlying.opens != 0 {
		t.Fatalf("close-only opened %d positions", underlying.opens)
	}

	underlying.positions = []map[string]interface{}{{
		"positionId": "follower-position", "symbol": "BTC-PERP", "side": "long", "positionAmt": 0.25,
	}}
	closeWire := comkunMasterStateWire{
		V: 1, SourceEventID: "event-close", SourceSequence: 2, OccurredAt: time.Now(), EventType: "CLOSE",
		Positions: []kernel.PositionInfo{}, PendingOrders: []kernel.PendingOrder{},
	}
	closeRaw, _ := json.Marshal(closeWire)
	if err := at.reconcileComkunFollowMasterStateV2(ctx, &store.ComkunMasterBroadcast{
		ID: 2, MasterAccountEquity: 1000, MasterStateJSON: string(closeRaw),
	}, &store.DecisionRecord{ExecutionLog: []string{}}, false, true); err != nil {
		t.Fatal(err)
	}
	if underlying.closes != 1 {
		t.Fatalf("close-only must close existing risk, closes=%d", underlying.closes)
	}
}

func TestHZFirstSubscriptionSeedsCurrentSequenceWithoutChasingPosition(t *testing.T) {
	underlying := &recoveringHZIntentTrader{balance: 250, contractSize: 0.01}
	at := &AutoTrader{
		exchange: "hz", initialBalance: 250, trader: underlying,
		config: AutoTraderConfig{StrategyConfig: &store.StrategyConfig{
			ComkunMarketFollow: true, ComkunMarketSourceStrategyID: store.HZMasterSourceStrategyID("master-1"),
		}},
	}
	wire := comkunMasterStateWire{
		V: 1, SourceEventID: "event-baseline", SourceSequence: 42,
		Positions:     []kernel.PositionInfo{{Symbol: "BTC-PERP", Side: "long", Lots: 2, Leverage: 10}},
		PendingOrders: []kernel.PendingOrder{},
	}
	raw, _ := json.Marshal(wire)
	at.seedMirrorBaselineFromMasterBroadcast(&store.ComkunMasterBroadcast{
		ID: 42, MasterAccountEquity: 1000, MasterStateJSON: string(raw),
	})
	key := posKey("BTC-PERP", "long")
	if got := at.mirrorSeedBaselineQty[key]; got != 0.005 {
		t.Fatalf("baseline quantity=%v want 0.005", got)
	}
	if target, _ := at.mirrorSeedAdjustedTarget(key, 0.005); target != 0 {
		t.Fatalf("first subscription chased historical master position: target=%v", target)
	}
}

func (trader *recoveringHZIntentTrader) LookupOrderByClientID(clientOrderID string) (map[string]interface{}, error) {
	if order := trader.orders[clientOrderID]; order != nil {
		return order, nil
	}
	return nil, &hz.APIError{Code: "NOT_FOUND", Message: "not found", Status: 404}
}

func TestHZMirrorIntentRecoversAcceptedOrderAcrossRestart(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "hz-intent.db"))
	if err != nil {
		t.Fatal(err)
	}
	wire := &comkunMasterStateWire{SourceEventID: "event-crash", SourceSequence: 7}
	intentKey, clientOrderID := hzMirrorIntentIDs(wire.SourceEventID, "user-1", "trader-1", "BTC-PERP", "long", "open")
	if _, err := st.MirrorExecutionIntent().Ensure(store.MirrorExecutionIntentInput{
		IntentKey: intentKey, MasterEventID: wire.SourceEventID, BroadcastID: 9,
		UserID: "user-1", TraderID: "trader-1", ExchangeID: "exchange-1",
		Instrument: "BTC-PERP", PositionSide: "long", Action: "open",
		TargetQuantity: 1, DeltaQuantity: 1, ClientOrderID: clientOrderID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.MirrorExecutionIntent().MarkSubmitted(intentKey); err != nil {
		t.Fatal(err)
	}
	underlying := &recoveringHZIntentTrader{orders: map[string]map[string]interface{}{
		clientOrderID: {"orderId": "remote-order-1", "status": "FILLED"},
	}}
	restarted := &AutoTrader{
		id: "trader-1", userID: "user-1", exchangeID: "exchange-1", exchange: "hz",
		store: st, trader: underlying,
	}
	executions := 0
	result, err := restarted.executeHZMirrorIntent(
		&store.ComkunMasterBroadcast{ID: 9}, wire, "BTC-PERP", "long", "open", "", 1, 1,
		func() (map[string]interface{}, error) {
			executions++
			return map[string]interface{}{"orderId": "duplicate"}, nil
		},
	)
	if err != nil || result["orderId"] != "remote-order-1" {
		t.Fatalf("result=%v err=%v", result, err)
	}
	if executions != 0 || underlying.next != "" {
		t.Fatalf("a recovered order must not be submitted again: executions=%d next=%q", executions, underlying.next)
	}
	intent, err := st.MirrorExecutionIntent().Get(intentKey)
	if err != nil || intent.Status != store.MirrorIntentConfirmed || intent.ExchangeOrderID != "remote-order-1" {
		t.Fatalf("intent=%+v err=%v", intent, err)
	}
}
