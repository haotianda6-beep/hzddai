package market

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetWithTimeframesUsesHZCandles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/candles" || r.URL.Query().Get("instrument") != "XAUUSD" {
			http.NotFound(w, r)
			return
		}
		base := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
		candles := make([]map[string]string, 30)
		for i := range candles {
			open := base.Add(time.Duration(i) * 5 * time.Minute)
			candles[i] = map[string]string{
				"instrument": "XAUUSD", "interval": "5m",
				"open": "2400", "high": "2402", "low": "2399", "close": "2401", "volume": "10",
				"openTime": open.Format(time.RFC3339), "closeTime": open.Add(5 * time.Minute).Format(time.RFC3339),
			}
		}
		_ = json.NewEncoder(w).Encode(candles)
	}))
	defer server.Close()

	if err := ConfigureHZ(server.URL + "/api/v1"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ConfigureHZ("") })
	data, err := GetWithTimeframes("XAUUSD", []string{"5m"}, "5m", 20)
	if err != nil {
		t.Fatal(err)
	}
	if data.Symbol != "XAUUSD" || data.CurrentPrice != 2401 || len(data.TimeframeData["5m"].Klines) != 20 {
		t.Fatalf("wrong HZ market data: %#v", data)
	}
}
