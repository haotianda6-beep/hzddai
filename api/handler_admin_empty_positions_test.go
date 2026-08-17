package api

import (
	"encoding/json"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestEmptyAdminBinancePositionsEncodeAsArray(t *testing.T) {
	raw, err := json.Marshal(gin.H{
		"binance_open_positions": make([]adminBinPos, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"binance_open_positions":[]}` {
		t.Fatalf("empty positions must be [], got %s", raw)
	}
}
