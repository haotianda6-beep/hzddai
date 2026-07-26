package api

import (
	"path/filepath"
	"testing"
)

func TestRememberOkxMarketHistoryUniqueNameFromRelay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "okx_unique_names.json")
	t.Setenv("OKX_MARKET_HISTORY_UNIQUE_FILE", path)

	rememberOkxMarketHistoryUniqueNameFromRelay(18770, []byte(`{"unique_name":"ABC1234567890123"}`))

	got := okxMarketHistoryUniqueName(okx06MarketStrategyID, "")
	if got != "ABC1234567890123" {
		t.Fatalf("expected persisted unique name, got %q", got)
	}
}
