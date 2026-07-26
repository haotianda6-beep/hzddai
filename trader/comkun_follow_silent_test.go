package trader

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"nofx/store"
	"nofx/trader/types"
)

type failingBalanceTrader struct{}

func (f failingBalanceTrader) GetBalance() (map[string]interface{}, error) {
	return nil, fmt.Errorf("balance timeout")
}
func (f failingBalanceTrader) GetPositions() ([]map[string]interface{}, error) { return nil, nil }
func (f failingBalanceTrader) OpenLong(string, float64, int) (map[string]interface{}, error) {
	return nil, nil
}
func (f failingBalanceTrader) OpenShort(string, float64, int) (map[string]interface{}, error) {
	return nil, nil
}
func (f failingBalanceTrader) CloseLong(string, float64) (map[string]interface{}, error) {
	return nil, nil
}
func (f failingBalanceTrader) CloseShort(string, float64) (map[string]interface{}, error) {
	return nil, nil
}
func (f failingBalanceTrader) SetLeverage(string, int) error          { return nil }
func (f failingBalanceTrader) SetMarginMode(string, bool) error       { return nil }
func (f failingBalanceTrader) GetMarketPrice(string) (float64, error) { return 0, nil }
func (f failingBalanceTrader) SetStopLoss(string, string, float64, float64) error {
	return nil
}
func (f failingBalanceTrader) SetTakeProfit(string, string, float64, float64) error {
	return nil
}
func (f failingBalanceTrader) CancelStopLossOrders(string) error   { return nil }
func (f failingBalanceTrader) CancelTakeProfitOrders(string) error { return nil }
func (f failingBalanceTrader) CancelAllOrders(string) error        { return nil }
func (f failingBalanceTrader) CancelStopOrders(string) error       { return nil }
func (f failingBalanceTrader) FormatQuantity(string, float64) (string, error) {
	return "", nil
}
func (f failingBalanceTrader) GetOrderStatus(string, string) (map[string]interface{}, error) {
	return nil, nil
}
func (f failingBalanceTrader) GetClosedPnL(time.Time, int) ([]types.ClosedPnLRecord, error) {
	return nil, nil
}
func (f failingBalanceTrader) GetOpenOrders(string) ([]types.OpenOrder, error) {
	return nil, nil
}

func TestComkunFollowSnapshotErrorDoesNotCreateVisibleDecision(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "follow-silent.db"))
	if err != nil {
		t.Fatal(err)
	}

	at := &AutoTrader{
		id:     "follow-trader",
		name:   "follow-trader",
		trader: failingBalanceTrader{},
		store:  st,
		userID: "user-1",
		config: AutoTraderConfig{
			StrategyConfig: &store.StrategyConfig{ComkunMarketFollow: true},
		},
	}
	at.isRunning = true

	if err := at.runCycle(); err != nil {
		t.Fatalf("runCycle returned error: %v", err)
	}
	records, err := st.Decision().GetLatestRecords(at.id, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("expected no customer-visible decision records, got %+v", records)
	}
}
