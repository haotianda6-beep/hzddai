package trader

import (
	"fmt"
	"testing"
	"time"

	"nofx/store"
)

func TestComkunDailyFeeTargetRangeAndStability(t *testing.T) {
	day := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	a := comkunDailyFeeTargetUSDT("user-a", "trader-a", "source-a", day)
	b := comkunDailyFeeTargetUSDT("user-a", "trader-a", "source-a", day)
	if a != b {
		t.Fatalf("daily target should be stable, got %.8f and %.8f", a, b)
	}
	if a < 5 || a > 8 {
		t.Fatalf("daily target %.8f outside 5-8 USDT range", a)
	}
}

func TestComkunFeeFromDailyTarget(t *testing.T) {
	got := comkunFeeFromDailyTarget(6.5, 100)
	if got != 0.065 {
		t.Fatalf("fee = %.8f, want 0.065", got)
	}
}

func TestComkunJitteredFeeVariesWithinConfiguredRange(t *testing.T) {
	t.Setenv("COMKUN_FOLLOW_SCAN_FEE_JITTER_PCT", "0.12")
	seen := make(map[float64]bool)
	for i := 1; i <= 20; i++ {
		fee := comkunJitteredFeeFromDailyTarget(6.5, 6500, fmt.Sprintf("broadcast-%d", i))
		if fee < 0.00088 || fee > 0.00112 {
			t.Fatalf("fee %.8f outside expected jitter range", fee)
		}
		seen[fee] = true
	}
	if len(seen) < 2 {
		t.Fatalf("expected varying fees, got %v", seen)
	}
}

func TestComkunSourceIdleAndOfflineCadence(t *testing.T) {
	t.Setenv("COMKUN_FOLLOW_BLANK_EXPECTED_ANALYSES_PER_DAY", "17280")
	oldInterval := comkunFollowFollowMasterPollInterval
	comkunFollowFollowMasterPollInterval = 5 * time.Second
	defer func() { comkunFollowFollowMasterPollInterval = oldInterval }()

	started := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	at := &AutoTrader{
		startTime: started,
		config: AutoTraderConfig{StrategyConfig: &store.StrategyConfig{
			ComkunMarketFollow:           true,
			ComkunMarketSourceStrategyID: "okx-screen-mirror-02",
		}},
	}
	if at.comkunFollowSourceNeedsScheduledBilling(nil, started.Add(29*time.Second)) {
		t.Fatal("feed should not be offline before grace period")
	}
	if !at.comkunFollowSourceNeedsScheduledBilling(nil, started.Add(30*time.Second)) {
		t.Fatal("feed should be offline after grace period")
	}

	now := started.Add(30 * time.Second)
	if at.comkunFollowOfflineChargeDue(now) {
		t.Fatal("first offline check should initialize cadence without charging")
	}
	if at.comkunFollowOfflineChargeDue(now.Add(4 * time.Second)) {
		t.Fatal("offline charge should wait for the full interval")
	}
	if !at.comkunFollowOfflineChargeDue(now.Add(5 * time.Second)) {
		t.Fatal("offline charge should be due after one interval")
	}
}

func TestComkunOkxScreenMirrorBroadcastFreshnessIsShortLived(t *testing.T) {
	at := &AutoTrader{
		config: AutoTraderConfig{StrategyConfig: &store.StrategyConfig{
			ComkunMarketFollow:           true,
			ComkunMarketSourceStrategyID: "okx-screen-mirror-01",
		}},
	}
	if !at.comkunFollowBroadcastIsFresh(&store.ComkunMasterBroadcast{ID: 1, CreatedAt: time.Now().Add(-time.Minute)}) {
		t.Fatal("fresh OKX mirror broadcast should be consumed")
	}
	if at.comkunFollowBroadcastIsFresh(&store.ComkunMasterBroadcast{ID: 2, CreatedAt: time.Now().Add(-3 * time.Minute)}) {
		t.Fatal("stale OKX mirror broadcast should be skipped")
	}
}

func TestComkunBinanceScreenMirrorKeepsLongerFreshness(t *testing.T) {
	at := &AutoTrader{
		config: AutoTraderConfig{StrategyConfig: &store.StrategyConfig{
			ComkunMarketFollow:           true,
			ComkunMarketSourceStrategyID: "bn-screen-mirror-test",
		}},
	}
	if !at.comkunFollowBroadcastIsFresh(&store.ComkunMasterBroadcast{ID: 1, CreatedAt: time.Now().Add(-3 * time.Hour)}) {
		t.Fatal("Binance mirror broadcast should keep the longer freshness window")
	}
}

func TestComkunScheduledBillingIsIndependentPerRunningTrader(t *testing.T) {
	t.Setenv("COMKUN_FOLLOW_BLANK_EXPECTED_ANALYSES_PER_DAY", "17280")
	oldInterval := comkunFollowFollowMasterPollInterval
	comkunFollowFollowMasterPollInterval = 5 * time.Second
	defer func() { comkunFollowFollowMasterPollInterval = oldInterval }()

	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	br := &store.ComkunMasterBroadcast{ID: 42, CreatedAt: now.Add(-time.Minute)}
	newTrader := func(id string) *AutoTrader {
		return &AutoTrader{
			id:        id,
			userID:    "same-user",
			startTime: now.Add(-time.Hour),
			config: AutoTraderConfig{StrategyConfig: &store.StrategyConfig{
				ComkunMarketFollow:           true,
				ComkunMarketSourceStrategyID: "mt4-ea-gold-master",
			}},
			lastComkunConsumedBroadcastID: br.ID,
		}
	}
	a := newTrader("trader-a")
	b := newTrader("trader-b")
	if !a.comkunFollowSourceNeedsScheduledBilling(br, now) || !b.comkunFollowSourceNeedsScheduledBilling(br, now) {
		t.Fatal("each trader should enter scheduled billing when its source is idle")
	}
	if a.comkunFollowOfflineChargeDue(now) || b.comkunFollowOfflineChargeDue(now) {
		t.Fatal("each trader should initialize its own cadence without an immediate charge")
	}
	if !a.comkunFollowOfflineChargeDue(now.Add(5*time.Second)) || !b.comkunFollowOfflineChargeDue(now.Add(5*time.Second)) {
		t.Fatal("each trader should become independently billable after one interval")
	}
	if comkunDailyFeeTargetUSDT(a.userID, a.id, "mt4-ea-gold-master", now) ==
		comkunDailyFeeTargetUSDT(b.userID, b.id, "mt4-ea-gold-master", now) {
		t.Fatal("daily fee target must be keyed by trader id, not only by user")
	}
}
