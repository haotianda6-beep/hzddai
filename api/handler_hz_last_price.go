package api

import (
	"net/http"
	"strings"

	"nofx/trader/hz"

	"github.com/gin-gonic/gin"
)

const maxLastPriceSymbols = 20

func (s *Server) handleHZLastPrices(c *gin.Context) {
	rawSymbols := strings.Split(c.Query("symbols"), ",")
	symbols := make([]string, 0, len(rawSymbols))
	seen := make(map[string]struct{}, len(rawSymbols))
	for _, symbol := range rawSymbols {
		symbol = strings.ToUpper(strings.TrimSpace(symbol))
		if !isMarketSymbol(symbol) {
			continue
		}
		if _, ok := seen[symbol]; ok {
			continue
		}
		seen[symbol] = struct{}{}
		symbols = append(symbols, symbol)
		if len(symbols) == maxLastPriceSymbols {
			break
		}
	}
	c.JSON(http.StatusOK, hz.CurrentBinanceLastPrices(symbols))
}

func isMarketSymbol(symbol string) bool {
	if len(symbol) < 3 || len(symbol) > 20 {
		return false
	}
	for _, char := range symbol {
		if (char < 'A' || char > 'Z') && (char < '0' || char > '9') {
			return false
		}
	}
	return true
}
