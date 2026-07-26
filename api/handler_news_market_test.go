package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHandleGoldMarketSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("MT4_STRATEGY_SIGNAL_SECRET", "test-secret")
	t.Setenv("GOLD_MARKET_STATE_PATH", filepath.Join(t.TempDir(), "market.json"))
	router := gin.New()
	server := &Server{}
	router.POST("/api/news/gold-market-snapshot", server.handleGoldMarketSnapshot)

	body := `{"secret":"test-secret","client_timestamp":0,"xau_symbol":"XAUUSDc","xau_bid":4050.1,"xau_ask":4050.3,"dollar_symbol":"DOLLAR","dollar_bid":99.1,"dollar_ask":99.2}`
	req := httptest.NewRequest(http.MethodPost, "/api/news/gold-market-snapshot", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	bad := strings.Replace(body, "test-secret", "wrong-secret", 1)
	req = httptest.NewRequest(http.MethodPost, "/api/news/gold-market-snapshot", strings.NewReader(bad))
	req.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d body=%s", response.Code, response.Body.String())
	}
}
