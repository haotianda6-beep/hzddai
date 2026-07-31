package store

import (
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newIntegrationTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&IntegrationMasterEvent{}, &MirrorExecutionIntent{}, &ComkunMasterBroadcast{},
		&User{}, &WalletLedger{}, &AIPlatformUsageLedger{},
	); err != nil {
		t.Fatal(err)
	}
	st, err := NewFromGorm(db)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func integrationInput(eventID, nonce string, sequence int64) IntegrationMasterEventInput {
	return IntegrationMasterEventInput{
		Provider: "hz", MasterAccountID: "master-1", EventID: eventID,
		Sequence: sequence, EventType: "position.snapshot", AuthNonce: nonce,
		OccurredAt:     time.Date(2026, 8, 1, 0, 0, int(sequence), 0, time.UTC),
		PayloadVersion: 1, PayloadSHA256: fmt.Sprintf("sha-%d", sequence),
		PayloadJSON: `{}`, MasterEquity: 1000,
		MasterStateJSON: `{"v":1,"positions":[],"pending_orders":[]}`,
	}
}

func TestIntegrationMasterEventReplayGapAndPollingRecovery(t *testing.T) {
	st := newIntegrationTestStore(t)
	first, err := st.IntegrationMasterEvent().Accept(integrationInput("event-10", "nonce-10", 10), false)
	if err != nil || !first.Accepted || first.BroadcastID == 0 {
		t.Fatalf("first=%+v err=%v", first, err)
	}

	replay, err := st.IntegrationMasterEvent().Accept(integrationInput("event-10", "nonce-10b", 10), false)
	if err != nil || !replay.Duplicate || replay.BroadcastID != first.BroadcastID {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}

	gap, err := st.IntegrationMasterEvent().Accept(integrationInput("event-12", "nonce-12", 12), false)
	if err != nil || !gap.Gap || gap.ExpectedNextSequence != 11 {
		t.Fatalf("gap=%+v err=%v", gap, err)
	}

	recovered, err := st.IntegrationMasterEvent().Accept(integrationInput("event-12", "nonce-poll-12", 12), true)
	if err != nil || !recovered.Accepted || recovered.ExpectedNextSequence != 13 {
		t.Fatalf("recovered=%+v err=%v", recovered, err)
	}
	next, err := st.IntegrationMasterEvent().Accept(integrationInput("event-13", "nonce-13", 13), false)
	if err != nil || !next.Accepted || next.ExpectedNextSequence != 14 {
		t.Fatalf("next=%+v err=%v", next, err)
	}

	var broadcasts int64
	if err := st.GormDB().Model(&ComkunMasterBroadcast{}).Count(&broadcasts).Error; err != nil || broadcasts != 3 {
		t.Fatalf("broadcasts=%d err=%v", broadcasts, err)
	}
}

func TestIntegrationMasterEventFanoutThirtyFollowers(t *testing.T) {
	st := newIntegrationTestStore(t)
	const followers = 30
	channels := make([]chan struct{}, 0, followers)
	cleanups := make([]func(), 0, followers)
	for i := 0; i < followers; i++ {
		ch := make(chan struct{}, 1)
		channels = append(channels, ch)
		cleanups = append(cleanups, RegisterComkunFollowWake(HZMasterSourceStrategyID("master-1"), ch))
	}
	defer func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}()
	if result, err := st.IntegrationMasterEvent().Accept(integrationInput("event-1", "nonce-1", 1), false); err != nil || !result.Accepted {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	for i, ch := range channels {
		select {
		case <-ch:
		default:
			t.Fatalf("follower %d was not woken", i)
		}
	}
}

func TestMirrorExecutionIntentAndBillingAreIdempotent(t *testing.T) {
	st := newIntegrationTestStore(t)
	if err := st.GormDB().Create(&User{ID: "user-1", Email: "user@example.test", PasswordHash: "x", BalanceUSDT: 1}).Error; err != nil {
		t.Fatal(err)
	}
	input := MirrorExecutionIntentInput{
		IntentKey: "intent-fixed", MasterEventID: "event-1", BroadcastID: 1,
		UserID: "user-1", TraderID: "trader-1", ExchangeID: "exchange-1",
		Instrument: "BTCUSDT", PositionSide: "long", Action: "open",
		ClientOrderID: "comkun-fixed", RequestSHA256: "request-hash",
	}
	one, err := st.MirrorExecutionIntent().Ensure(input)
	if err != nil {
		t.Fatal(err)
	}
	two, err := st.MirrorExecutionIntent().Ensure(input)
	if err != nil || one.ID != two.ID || one.ClientOrderID != two.ClientOrderID {
		t.Fatalf("one=%+v two=%+v err=%v", one, two, err)
	}

	reservation, err := st.MirrorExecutionIntent().ReserveBilling("intent-fixed", 0.25)
	if err != nil || !reservation.Reserved || reservation.BalanceAfter != 0.75 {
		t.Fatalf("reservation=%+v err=%v", reservation, err)
	}
	again, err := st.MirrorExecutionIntent().ReserveBilling("intent-fixed", 0.25)
	if err != nil || again.BalanceAfter != 0.75 {
		t.Fatalf("again=%+v err=%v", again, err)
	}
	if err := st.MirrorExecutionIntent().RefundBilling("intent-fixed", "exchange_failed"); err != nil {
		t.Fatal(err)
	}
	if err := st.MirrorExecutionIntent().RefundBilling("intent-fixed", "duplicate_refund"); err != nil {
		t.Fatal(err)
	}
	afterRefund, err := st.MirrorExecutionIntent().ReserveBilling("intent-fixed", 0.25)
	if err != nil || afterRefund.Reserved {
		t.Fatalf("refunded reservation=%+v err=%v", afterRefund, err)
	}
	user, _ := st.User().GetByID("user-1")
	if user.BalanceUSDT != 1 {
		t.Fatalf("balance=%v want 1", user.BalanceUSDT)
	}
	var usage AIPlatformUsageLedger
	if err := st.GormDB().First(&usage).Error; err != nil || usage.Status != AIPlatformUsageStatusRefunded {
		t.Fatalf("usage=%+v err=%v", usage, err)
	}
	var net float64
	if err := st.GormDB().Model(&WalletLedger{}).Select("COALESCE(SUM(delta),0)").Scan(&net).Error; err != nil || net != 0 {
		t.Fatalf("ledger net=%v err=%v", net, err)
	}
}

func TestCorrectRefundedBillingForConfirmedOpenIsIdempotent(t *testing.T) {
	st := newIntegrationTestStore(t)
	if err := st.GormDB().Create(&User{ID: "user-correction", Email: "correction@example.test", PasswordHash: "x", BalanceUSDT: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := st.MirrorExecutionIntent().Ensure(MirrorExecutionIntentInput{
		IntentKey: "billing-correction", MasterEventID: "event-correction", BroadcastID: 3,
		UserID: "user-correction", TraderID: "trader-correction", ExchangeID: "exchange-correction",
		Instrument: "__account__", PositionSide: "none", Action: "billing", ClientOrderID: "billing-correction",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.MirrorExecutionIntent().ReserveBilling("billing-correction", 0.25); err != nil {
		t.Fatal(err)
	}
	if _, err := st.MirrorExecutionIntent().Ensure(MirrorExecutionIntentInput{
		IntentKey: "open-correction", MasterEventID: "event-correction", BroadcastID: 3,
		UserID: "user-correction", TraderID: "trader-correction", ExchangeID: "exchange-correction",
		Instrument: "BTCUSDT", PositionSide: "long", Action: "open", ClientOrderID: "open-correction",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.MirrorExecutionIntent().MarkConfirmed("open-correction", "remote-order"); err != nil {
		t.Fatal(err)
	}
	if err := st.MirrorExecutionIntent().RefundBilling("billing-correction", "open_ttl_expired"); err != nil {
		t.Fatal(err)
	}
	corrected, err := st.MirrorExecutionIntent().CorrectRefundedBillingForConfirmedOpen("billing-correction", "sequence3_confirmed_open")
	if err != nil || !corrected {
		t.Fatalf("corrected=%t err=%v", corrected, err)
	}
	corrected, err = st.MirrorExecutionIntent().CorrectRefundedBillingForConfirmedOpen("billing-correction", "duplicate")
	if err != nil || corrected {
		t.Fatalf("duplicate corrected=%t err=%v", corrected, err)
	}
	user, err := st.User().GetByID("user-correction")
	if err != nil || user.BalanceUSDT != 0.75 {
		t.Fatalf("user=%+v err=%v", user, err)
	}
	var net float64
	if err := st.GormDB().Model(&WalletLedger{}).Select("COALESCE(SUM(delta),0)").Scan(&net).Error; err != nil || net != -0.25 {
		t.Fatalf("ledger net=%v err=%v", net, err)
	}
	var usage AIPlatformUsageLedger
	if err := st.GormDB().First(&usage).Error; err != nil || usage.Status != AIPlatformUsageStatusSuccess {
		t.Fatalf("usage=%+v err=%v", usage, err)
	}
}

func TestInsufficientBillingDoesNotDebit(t *testing.T) {
	st := newIntegrationTestStore(t)
	if err := st.GormDB().Create(&User{ID: "user-1", Email: "low@example.test", PasswordHash: "x", BalanceUSDT: 0.1}).Error; err != nil {
		t.Fatal(err)
	}
	_, err := st.MirrorExecutionIntent().Ensure(MirrorExecutionIntentInput{
		IntentKey: "billing-low", UserID: "user-1", TraderID: "trader-1",
		Action: "billing", ClientOrderID: "billing-low",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := st.MirrorExecutionIntent().ReserveBilling("billing-low", 0.25)
	if err != nil || !result.Insufficient || result.Reserved {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	user, _ := st.User().GetByID("user-1")
	if user.BalanceUSDT != 0.1 {
		t.Fatalf("balance changed: %v", user.BalanceUSDT)
	}
}
