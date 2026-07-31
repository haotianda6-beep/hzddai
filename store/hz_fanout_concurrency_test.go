package store

import (
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"
)

func newHZFanoutTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := InitGorm(filepath.Join(t.TempDir(), "hz-fanout.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&User{}, &WalletLedger{}, &AIPlatformUsageLedger{},
		&MirrorExecutionIntent{}, &ComkunFollowBroadcastConsumption{},
	); err != nil {
		t.Fatal(err)
	}
	st, err := NewFromGorm(db)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestHZThirtyConcurrentBillingReservationsCompleteWithinThreeSeconds(t *testing.T) {
	st := newHZFanoutTestStore(t)
	const followers = 30
	for i := 0; i < followers; i++ {
		userID := fmt.Sprintf("user-%02d", i)
		if err := st.GormDB().Create(&User{
			ID: userID, Email: userID + "@example.test", PasswordHash: "x", BalanceUSDT: 10,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}

	start := make(chan struct{})
	errCh := make(chan error, followers)
	durations := make(chan time.Duration, followers)
	var wg sync.WaitGroup
	for i := 0; i < followers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			began := time.Now()
			userID := fmt.Sprintf("user-%02d", i)
			traderID := fmt.Sprintf("trader-%02d", i)
			exchangeID := fmt.Sprintf("exchange-%02d", i)
			intentKey := fmt.Sprintf("billing-%02d", i)
			if _, err := st.MirrorExecutionIntent().Ensure(MirrorExecutionIntentInput{
				IntentKey: intentKey, MasterEventID: "event-fanout", BroadcastID: 1,
				UserID: userID, TraderID: traderID, ExchangeID: exchangeID,
				Instrument: "__account__", PositionSide: "none", Action: "billing", ClientOrderID: intentKey,
			}); err != nil {
				errCh <- fmt.Errorf("billing intent follower %02d: %w", i, err)
				return
			}
			if _, err := st.MirrorExecutionIntent().ReserveBilling(intentKey, 0.05); err != nil {
				errCh <- fmt.Errorf("reserve follower %02d: %w", i, err)
				return
			}
			openKey := fmt.Sprintf("open-%02d", i)
			if _, err := st.MirrorExecutionIntent().Ensure(MirrorExecutionIntentInput{
				IntentKey: openKey, MasterEventID: "event-fanout", BroadcastID: 1,
				UserID: userID, TraderID: traderID, ExchangeID: exchangeID,
				Instrument: "BTCUSDT", PositionSide: "long", Action: "open", ClientOrderID: openKey,
			}); err != nil {
				errCh <- fmt.Errorf("trade intent follower %02d: %w", i, err)
				return
			}
			if err := st.MirrorExecutionIntent().MarkConfirmed(openKey, fmt.Sprintf("remote-%02d", i)); err != nil {
				errCh <- fmt.Errorf("confirm follower %02d: %w", i, err)
				return
			}
			if err := st.MirrorExecutionIntent().FinalizeBilling(intentKey); err != nil {
				errCh <- fmt.Errorf("finalize follower %02d: %w", i, err)
				return
			}
			durations <- time.Since(began)
		}(i)
	}
	close(start)
	wg.Wait()
	close(errCh)
	close(durations)
	for err := range errCh {
		t.Error(err)
	}
	if t.Failed() {
		return
	}

	latencies := make([]time.Duration, 0, followers)
	for duration := range durations {
		latencies = append(latencies, duration)
	}
	if len(latencies) != followers {
		t.Fatalf("completed=%d want=%d", len(latencies), followers)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p95 := latencies[(followers*95+99)/100-1]
	t.Logf("30-follower store fanout p95=%s max=%s", p95, latencies[len(latencies)-1])
	if p95 > 3*time.Second {
		t.Fatalf("billing fanout p95=%s exceeds 3s", p95)
	}
	var charged int64
	if err := st.GormDB().Model(&MirrorExecutionIntent{}).
		Where("master_event_id = ? AND action = ? AND billing_status = ?", "event-fanout", "billing", MirrorBillingCharged).
		Count(&charged).Error; err != nil || charged != followers {
		t.Fatalf("charged=%d want=%d err=%v", charged, followers, err)
	}
	var confirmed int64
	if err := st.GormDB().Model(&MirrorExecutionIntent{}).
		Where("master_event_id = ? AND action = ? AND status = ?", "event-fanout", "open", MirrorIntentConfirmed).
		Count(&confirmed).Error; err != nil || confirmed != followers {
		t.Fatalf("confirmed=%d want=%d err=%v", confirmed, followers, err)
	}
}

func TestHZThirtyConcurrentReduceCloseConsumptionCheckpointsAreDurable(t *testing.T) {
	st := newHZFanoutTestStore(t)
	for _, broadcastID := range []uint64{4, 5} {
		start := make(chan struct{})
		errCh := make(chan error, 30)
		var wg sync.WaitGroup
		for i := 0; i < 30; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				traderID := fmt.Sprintf("trader-%02d", i)
				acquired, err := st.ComkunFollow().TryAcquireConsumptionLockWithCooldown(traderID, broadcastID, 0)
				if err != nil || !acquired {
					errCh <- fmt.Errorf("acquire broadcast %d follower %02d: acquired=%t err=%w", broadcastID, i, acquired, err)
					return
				}
				if err := st.ComkunFollow().MarkConsumptionSuccess(traderID, broadcastID); err != nil {
					errCh <- fmt.Errorf("checkpoint broadcast %d follower %02d: %w", broadcastID, i, err)
				}
			}(i)
		}
		close(start)
		wg.Wait()
		close(errCh)
		for err := range errCh {
			t.Error(err)
		}
		var succeeded int64
		if err := st.GormDB().Model(&ComkunFollowBroadcastConsumption{}).
			Where("broadcast_id = ? AND status = ?", broadcastID, "success").Count(&succeeded).Error; err != nil || succeeded != 30 {
			t.Errorf("broadcast %d durable checkpoints=%d want=30 err=%v", broadcastID, succeeded, err)
		}
	}
}
