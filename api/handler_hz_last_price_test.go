package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHZLastPriceHandlerNeedsNoStoreOrTraderLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet,
		"/api/market/last-prices?symbols=NXPCUSDT,NXPCUSDT,../../secret", nil)

	(&Server{}).handleHZLastPrices(context)

	if response.Code != http.StatusOK || response.Body.String() != "[]" {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestMarketSymbolValidation(t *testing.T) {
	if !isMarketSymbol("NXPCUSDT") || isMarketSymbol("../../secret") || isMarketSymbol("") {
		t.Fatal("market symbol validation mismatch")
	}
}
