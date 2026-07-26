package api

import (
	"testing"
	"time"
)

func resetOkxMarketHistoryCacheForTest() {
	okxMarketHistoryCache.Lock()
	defer okxMarketHistoryCache.Unlock()
	okxMarketHistoryCache.entries = make(map[string]okxMarketHistoryCacheEntry)
}

func TestLoadOkxMarketHistoryRowsReturnsFallbackBeforeRefresh(t *testing.T) {
	resetOkxMarketHistoryCacheForTest()
	oldFetch := fetchOkxPublicPositionHistoryFn
	defer func() {
		fetchOkxPublicPositionHistoryFn = oldFetch
		resetOkxMarketHistoryCacheForTest()
	}()

	fetchStarted := make(chan struct{}, 1)
	releaseFetch := make(chan struct{})
	fetchOkxPublicPositionHistoryFn = func(uniqueName string) ([]okxMarketHistoryRow, error) {
		fetchStarted <- struct{}{}
		<-releaseFetch
		return []okxMarketHistoryRow{{Symbol: "ETHUSDT", Opened: "2026/06/29 10:00:00", Closed: "2026/06/29 10:15:00"}}, nil
	}

	fallback := []byte(`[{"Symbol":"BTCUSDT","Opened":"2026/06/28 10:00:00","Closed":"2026/06/28 10:15:00"}]`)
	started := time.Now()
	rows := loadOkxMarketHistoryRows("strategy-fast", "unique-fast", fallback)
	if elapsed := time.Since(started); elapsed > 50*time.Millisecond {
		t.Fatalf("load blocked on background refresh for %s", elapsed)
	}
	if len(rows) != 1 || rows[0].Symbol != "BTCUSDT" {
		t.Fatalf("expected immediate fallback row, got %#v", rows)
	}
	select {
	case <-fetchStarted:
	case <-time.After(time.Second):
		t.Fatal("background refresh did not start")
	}
	close(releaseFetch)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		rows = loadOkxMarketHistoryRows("strategy-fast", "unique-fast", fallback)
		if len(rows) == 1 && rows[0].Symbol == "ETHUSDT" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("refreshed rows did not become available, got %#v", rows)
}
