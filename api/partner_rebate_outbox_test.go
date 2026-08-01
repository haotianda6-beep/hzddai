package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"nofx/store"
)

func TestPartnerRebateOutboxIsAtomicAndIdempotentlyDelivered(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Platform-Secret") != "test-secret" {
			http.Error(w, `{"detail":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		mu.Lock()
		calls[r.URL.Path]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"duplicate":false}`))
	}))
	defer remote.Close()
	t.Setenv("AGENT_REBATE_BASE_URL", remote.URL)
	t.Setenv("AGENT_REBATE_PLATFORM_SECRET", "test-secret")

	st, err := store.New(filepath.Join(t.TempDir(), "outbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	user := &store.User{
		ID: "user-a", Email: "user-a@example.com", PasswordHash: "test", DisplayName: "A",
	}
	if err := st.User().Create(user); err != nil {
		t.Fatal(err)
	}
	s := &Server{store: st}

	deposit := &store.PartnerRebateOutbox{
		EventType: store.PartnerRebateEventDeposit,
		Source:    "finance_confirmed",
	}
	newBalance, depositLedgerID, depositOutboxID, err := s.runPlatformWalletAdjust(
		user.ID, 100, "partner_confirmed:finance", deposit,
	)
	if err != nil || newBalance != 100 || depositLedgerID == 0 || depositOutboxID == 0 {
		t.Fatalf("deposit transaction failed: balance=%v ledger=%d outbox=%d err=%v", newBalance, depositLedgerID, depositOutboxID, err)
	}
	s.dispatchPartnerRebateOutboxID(depositOutboxID)
	row, err := st.PartnerRebateOutbox().Get(depositOutboxID)
	if err != nil || row.Status != "sent" {
		t.Fatalf("deposit outbox not sent: row=%+v err=%v", row, err)
	}

	reversal := &store.PartnerRebateOutbox{
		EventType:              store.PartnerRebateEventReversal,
		OriginalWalletLedgerID: depositLedgerID,
		Source:                 "admin_reversal",
	}
	newBalance, _, reversalOutboxID, err := s.runPlatformWalletAdjust(
		user.ID, -20, "admin_adjust", reversal,
	)
	if err != nil || newBalance != 80 || reversalOutboxID == 0 {
		t.Fatalf("reversal transaction failed: balance=%v outbox=%d err=%v", newBalance, reversalOutboxID, err)
	}
	s.dispatchPartnerRebateOutboxID(reversalOutboxID)

	invalid := &store.PartnerRebateOutbox{
		EventType:              store.PartnerRebateEventReversal,
		OriginalWalletLedgerID: depositLedgerID + 999,
	}
	_, _, _, err = s.runPlatformWalletAdjust(user.ID, -10, "admin_adjust", invalid)
	if !errors.Is(err, store.ErrInvalidPartnerRebateReversal) {
		t.Fatalf("expected invalid reversal error, got %v", err)
	}
	refreshed, err := st.User().GetByID(user.ID)
	if err != nil || refreshed.BalanceUSDT != 80 {
		t.Fatalf("invalid reversal was not rolled back: balance=%v err=%v", refreshed.BalanceUSDT, err)
	}

	mu.Lock()
	defer mu.Unlock()
	if calls["/api/platform/user-sync"] != 1 || calls["/api/platform/deposits"] != 1 || calls["/api/platform/deposit-reversals"] != 1 {
		t.Fatalf("unexpected delivery calls: %+v", calls)
	}
}
