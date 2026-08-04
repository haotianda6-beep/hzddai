package trader

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"nofx/kernel"
	"nofx/store"
	"nofx/trader/hz"
	"nofx/trader/types"
)

type recoveringHZIntentTrader struct {
	failingBalanceTrader
	orders        map[string]map[string]interface{}
	next          string
	positions     []map[string]interface{}
	balance       float64
	contractSize  float64
	opens, closes int
	balanceCalls  int
	positionCalls int
	orderCalls    int
}

func (trader *recoveringHZIntentTrader) QuantityForLots(_ string, lots float64) (float64, error) {
	contractSize := trader.contractSize
	if contractSize == 0 {
		contractSize = 1
	}
	return lots * contractSize, nil
}

func (trader *recoveringHZIntentTrader) FormatQuantity(_ string, quantity float64) (string, error) {
	const step = 0.001
	floored := math.Floor((quantity+step*1e-9)/step) * step
	return strconv.FormatFloat(floored, 'f', 3, 64), nil
}

func (trader *recoveringHZIntentTrader) GetBalance() (map[string]interface{}, error) {
	trader.balanceCalls++
	return map[string]interface{}{"total_equity": trader.balance}, nil
}

func (trader *recoveringHZIntentTrader) GetPositions() ([]map[string]interface{}, error) {
	trader.positionCalls++
	return trader.positions, nil
}

func (trader *recoveringHZIntentTrader) GetOpenOrders(string) ([]types.OpenOrder, error) {
	trader.orderCalls++
	return nil, nil
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

func (trader *recoveringHZIntentTrader) ClosePositionByID(positionID string, quantity float64) (map[string]interface{}, error) {
	for i, position := range trader.positions {
		if position["positionId"] != positionID {
			continue
		}
		current, _ := position["positionAmt"].(float64)
		if quantity == 0 || quantity+qtyEps(quantity) >= math.Abs(current) {
			trader.positions = append(trader.positions[:i], trader.positions[i+1:]...)
		} else if current < 0 {
			trader.positions[i]["positionAmt"] = current + quantity
		} else {
			trader.positions[i]["positionAmt"] = current - quantity
		}
		trader.closes++
		return map[string]interface{}{"orderId": "close-" + positionID}, nil
	}
	return nil, fmt.Errorf("position not found")
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

	underlying.positions = []map[string]interface{}{
		{"positionId": "follower-position-a", "symbol": "BTC-PERP", "side": "long", "positionAmt": 0.10},
		{"positionId": "follower-position-b", "symbol": "BTC-PERP", "side": "long", "positionAmt": 0.15},
	}
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
	if underlying.closes != 2 || len(underlying.positions) != 0 {
		t.Fatalf("close-only must close every exact position, closes=%d positions=%v", underlying.closes, underlying.positions)
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
		&store.ComkunMasterBroadcast{ID: 9}, wire, "BTC-PERP", "long", "open", "", "", 1, 1,
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

func TestHZThirtyFollowersBackOffBeforeExchangeSnapshot(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "hz-idle-backoff.db"))
	if err != nil {
		t.Fatal(err)
	}
	sourceID := store.HZMasterSourceStrategyID("master-idle")
	wire := comkunMasterStateWire{
		V: 1, SourceEventID: "event-failed", SourceSequence: 1,
		OccurredAt: time.Now(), EventType: "OPEN",
		Positions: []kernel.PositionInfo{{
			PositionID: "master-position", Symbol: "BTCUSDT", Side: "long", Lots: 0.03, Leverage: 100,
		}},
		PendingOrders: []kernel.PendingOrder{},
	}
	raw, _ := json.Marshal(wire)
	broadcast, err := st.ComkunFollow().InsertBroadcast(sourceID, 10_000, "AI策略执行", "[]", string(raw))
	if err != nil {
		t.Fatal(err)
	}

	var balanceCalls, positionCalls, orderCalls int
	for i := 0; i < 30; i++ {
		traderID := fmt.Sprintf("trader-%02d", i)
		acquired, err := st.ComkunFollow().TryAcquireConsumptionLock(traderID, broadcast.ID)
		if err != nil || !acquired {
			t.Fatalf("seed failed consumption %d: acquired=%t err=%v", i, acquired, err)
		}
		if err := st.ComkunFollow().MarkConsumptionFailed(traderID, broadcast.ID, "temporary"); err != nil {
			t.Fatal(err)
		}
		underlying := &recoveringHZIntentTrader{balance: 10_000, contractSize: 1}
		at := &AutoTrader{
			id: traderID, name: traderID, userID: fmt.Sprintf("user-%02d", i),
			exchangeID: fmt.Sprintf("exchange-%02d", i), exchange: "hz",
			initialBalance: 10_000, store: st, trader: underlying, isRunning: true,
			config: AutoTraderConfig{StrategyConfig: &store.StrategyConfig{
				ComkunMarketFollow: true, ComkunMarketSourceStrategyID: sourceID,
			}},
		}
		if err := at.runCycle(); err != nil {
			t.Fatalf("follower %d runCycle: %v", i, err)
		}
		balanceCalls += underlying.balanceCalls
		positionCalls += underlying.positionCalls
		orderCalls += underlying.orderCalls
	}
	if balanceCalls+positionCalls+orderCalls != 0 {
		t.Fatalf("cooldown made B calls: balance=%d positions=%d orders=%d", balanceCalls, positionCalls, orderCalls)
	}
}

func TestHZExpiredOpenRefundsBillingWithoutTradeIntent(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "hz-expired-refund.db"))
	if err != nil {
		t.Fatal(err)
	}
	const userID = "user-expired"
	const traderID = "trader-expired"
	if err := st.GormDB().Create(&store.User{
		ID: userID, Email: "expired@example.test", PasswordHash: "x", BalanceUSDT: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	sourceID := store.HZMasterSourceStrategyID("master-expired")
	wire := comkunMasterStateWire{
		V: 1, SourceEventID: "event-expired", SourceSequence: 1,
		OccurredAt: time.Now().Add(-3 * time.Minute), EventType: "OPEN",
		Positions: []kernel.PositionInfo{{
			PositionID: "master-position", Symbol: "BTCUSDT", Side: "long", Lots: 0.03, Leverage: 100,
		}},
		PendingOrders: []kernel.PendingOrder{},
	}
	raw, _ := json.Marshal(wire)
	broadcast, err := st.ComkunFollow().InsertBroadcast(sourceID, 10_000, "AI策略执行", "[]", string(raw))
	if err != nil {
		t.Fatal(err)
	}
	underlying := &recoveringHZIntentTrader{balance: 1_000, contractSize: 1}
	at := &AutoTrader{
		id: traderID, name: traderID, userID: userID, exchangeID: "exchange-expired", exchange: "hz",
		initialBalance: 1_000, store: st, trader: underlying, isRunning: true,
		config: AutoTraderConfig{StrategyConfig: &store.StrategyConfig{
			ComkunMarketFollow: true, ComkunMarketSourceStrategyID: sourceID,
		}},
	}
	ctx := &kernel.Context{Account: kernel.AccountInfo{TotalEquity: 1_000}}
	if err := at.runComkunFollowCycle(ctx, &store.DecisionRecord{ExecutionLog: []string{}}); err != nil {
		t.Fatal(err)
	}
	if underlying.opens != 0 {
		t.Fatalf("expired OPEN submitted %d order(s)", underlying.opens)
	}
	billingKey, _ := hzMirrorIntentIDs(wire.SourceEventID, userID, traderID, "__account__", "none", "billing")
	intent, err := st.MirrorExecutionIntent().Get(billingKey)
	if err != nil || intent.BillingStatus != store.MirrorBillingRefunded {
		t.Fatalf("billing intent=%+v err=%v", intent, err)
	}
	user, err := st.User().GetByID(userID)
	if err != nil || user.BalanceUSDT != 1 {
		t.Fatalf("user=%+v err=%v", user, err)
	}
	if status, err := st.ComkunFollow().GetConsumptionStatus(traderID, broadcast.ID); err != nil || status != "success" {
		t.Fatalf("consumption status=%q err=%v", status, err)
	}
}

func TestHZExpiredOpenWithConfirmedTradeFinalizesBilling(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "hz-expired-confirmed.db"))
	if err != nil {
		t.Fatal(err)
	}
	const userID = "user-confirmed"
	const traderID = "trader-confirmed"
	if err := st.GormDB().Create(&store.User{
		ID: userID, Email: "confirmed@example.test", PasswordHash: "x", BalanceUSDT: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	sourceID := store.HZMasterSourceStrategyID("master-confirmed")
	wire := comkunMasterStateWire{
		V: 1, SourceEventID: "event-confirmed", SourceSequence: 3,
		OccurredAt: time.Now().Add(-3 * time.Minute), EventType: "OPEN",
		Positions: []kernel.PositionInfo{{
			PositionID: "master-position", Symbol: "BTCUSDT", Side: "long", Lots: 0.03, Leverage: 100,
		}},
		PendingOrders: []kernel.PendingOrder{},
	}
	raw, _ := json.Marshal(wire)
	broadcast, err := st.ComkunFollow().InsertBroadcast(sourceID, 1_000, "AI策略执行", "[]", string(raw))
	if err != nil {
		t.Fatal(err)
	}
	underlying := &recoveringHZIntentTrader{
		balance: 1_000, contractSize: 1,
		positions: []map[string]interface{}{{
			"positionId": "follower-position", "symbol": "BTCUSDT", "side": "long", "positionAmt": 0.03,
		}},
	}
	at := &AutoTrader{
		id: traderID, name: traderID, userID: userID, exchangeID: "exchange-confirmed", exchange: "hz",
		initialBalance: 1_000, store: st, trader: underlying, isRunning: true,
		config: AutoTraderConfig{StrategyConfig: &store.StrategyConfig{
			ComkunMarketFollow: true, ComkunMarketSourceStrategyID: sourceID,
		}},
	}
	fee := at.comkunFollowScanFeeForBroadcast(broadcast)
	billingKey, billingClientID := hzMirrorIntentIDs(wire.SourceEventID, userID, traderID, "__account__", "none", "billing")
	requestHash := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%.12g", fee))))
	if _, err := st.MirrorExecutionIntent().Ensure(store.MirrorExecutionIntentInput{
		IntentKey: billingKey, MasterEventID: wire.SourceEventID, BroadcastID: broadcast.ID,
		UserID: userID, TraderID: traderID, ExchangeID: "exchange-confirmed",
		Instrument: "__account__", PositionSide: "none", Action: "billing",
		ClientOrderID: billingClientID, RequestSHA256: requestHash,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.MirrorExecutionIntent().ReserveBilling(billingKey, fee); err != nil {
		t.Fatal(err)
	}
	openKey, openClientID := hzMirrorIntentIDs(wire.SourceEventID, userID, traderID, "BTCUSDT", "long", "open")
	if _, err := st.MirrorExecutionIntent().Ensure(store.MirrorExecutionIntentInput{
		IntentKey: openKey, MasterEventID: wire.SourceEventID, BroadcastID: broadcast.ID,
		UserID: userID, TraderID: traderID, ExchangeID: "exchange-confirmed",
		Instrument: "BTCUSDT", PositionSide: "long", Action: "open", ClientOrderID: openClientID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.MirrorExecutionIntent().MarkConfirmed(openKey, "remote-order"); err != nil {
		t.Fatal(err)
	}

	ctx := &kernel.Context{Account: kernel.AccountInfo{TotalEquity: 1_000}}
	if err := at.runComkunFollowCycle(ctx, &store.DecisionRecord{ExecutionLog: []string{}}); err != nil {
		t.Fatal(err)
	}
	billing, err := st.MirrorExecutionIntent().Get(billingKey)
	if err != nil || billing.BillingStatus != store.MirrorBillingCharged {
		t.Fatalf("billing=%+v err=%v, confirmed trade must stay charged", billing, err)
	}
	user, err := st.User().GetByID(userID)
	if err != nil || user.BalanceUSDT != 1-fee {
		t.Fatalf("balance=%v fee=%v err=%v", user.BalanceUSDT, fee, err)
	}
}

func TestComkunCheckpointFailureDoesNotAdvanceMemoryWatermark(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "checkpoint-failure.db"))
	if err != nil {
		t.Fatal(err)
	}
	const traderID = "checkpoint-trader"
	const broadcastID = uint64(5)
	acquired, err := st.ComkunFollow().TryAcquireConsumptionLockWithCooldown(traderID, broadcastID, 0)
	if err != nil || !acquired {
		t.Fatalf("acquired=%t err=%v", acquired, err)
	}
	sqlDB, err := st.GormDB().DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	at := &AutoTrader{id: traderID, name: traderID, store: st}
	if err := at.persistComkunConsumptionSuccess(broadcastID); err == nil {
		t.Fatal("closed database unexpectedly persisted checkpoint")
	}
	if at.comkunFollowBroadcastAlreadyConsumed(broadcastID) {
		t.Fatal("failed durable checkpoint advanced the memory watermark")
	}
}

func TestHZFollowFallbackPollBacksOffToFifteenSeconds(t *testing.T) {
	at := &AutoTrader{
		exchange: "hz",
		config: AutoTraderConfig{StrategyConfig: &store.StrategyConfig{
			ComkunMarketFollow:           true,
			ComkunMarketSourceStrategyID: store.HZMasterSourceStrategyID("master-backoff"),
		}},
	}
	if got := at.comkunFollowPollWaitAfterCycle(time.Second); got != 15*time.Second {
		t.Fatalf("HZ fallback poll=%s, want 15s", got)
	}
}
