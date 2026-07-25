package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSupportedExchangesIncludesHZTradingAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	(&Server{}).handleGetSupportedExchanges(context)

	var exchanges []SafeExchangeConfig
	if err := json.Unmarshal(recorder.Body.Bytes(), &exchanges); err != nil {
		t.Fatal(err)
	}
	for _, exchange := range exchanges {
		if exchange.ExchangeType == "hz" {
			if exchange.Name != "HZ 交易账户" {
				t.Fatalf("unexpected HZ display name: %q", exchange.Name)
			}
			return
		}
	}
	t.Fatal("HZ exchange type was not returned")
}

func TestNormalizeHZAPIURL(t *testing.T) {
	got, err := normalizeHZAPIURL(" https://trade.kunai.fun/api/v1/ ")
	if err != nil || got != "https://trade.kunai.fun/api/v1" {
		t.Fatalf("unexpected normalized URL %q: %v", got, err)
	}

	for _, value := range []string{
		"http://trade.kunai.fun/api/v1",
		"https://user@trade.kunai.fun/api/v1",
		"https://trade.kunai.fun/api/v1?token=secret",
	} {
		if _, err := normalizeHZAPIURL(value); err == nil {
			t.Fatalf("expected invalid HZ API URL: %q", value)
		}
	}
}
