package api

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"nofx/logger"
	"nofx/trader"
)

func normalizeAdminFlattenSymbol(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	if !strings.HasSuffix(s, "USDT") && !strings.HasSuffix(s, "BUSD") && !strings.HasSuffix(s, "USDC") {
		s = s + "USDT"
	}
	return s
}

func positionAmtFromInterface(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err != nil {
			return 0
		}
		return f
	default:
		return 0
	}
}

// handleAdminFlattenAllComkunFollowTraders 管理员：对绑定「市场合规跟单」策略的全部交易员在交易所提交市价平仓（顺序执行，避免并发打爆 API）。
// JSON body 可选：{ "symbol": "SOL" | "SOLUSDT" }，留空则平掉全部非零持仓。
func (s *Server) handleAdminFlattenAllComkunFollowTraders(c *gin.Context) {
	var req struct {
		Symbol string `json:"symbol"`
	}
	_ = c.ShouldBindJSON(&req)

	onlySym := normalizeAdminFlattenSymbol(req.Symbol)

	refs, err := s.store.Trader().ListAllComkunMarketFollowTraderRefs()
	if err != nil {
		SafeInternalError(c, "列出合规跟单交易员失败", err)
		return
	}

	logger.Infof("管理员一键全平合规跟单持仓: symbol_filter=%q 交易员数=%d", onlySym, len(refs))

	results := make([]gin.H, 0, len(refs))
	okCount := 0
	failCount := 0
	for _, ref := range refs {
		r := s.adminFlattenOneFollowTrader(ref.UserID, ref.TraderID, ref.TraderName, onlySym)
		results = append(results, r)
		if b, _ := r["ok"].(bool); b {
			okCount++
		} else {
			failCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "已按顺序处理（详见 results）",
		"symbol_filter": onlySym,
		"trader_count":  len(refs),
		"ok_count":      okCount,
		"fail_count":    failCount,
		"results":       results,
	})
}

func (s *Server) adminFlattenOneFollowTrader(userID, traderID, traderName, onlySymbol string) gin.H {
	fullConfig, err := s.store.Trader().GetFullConfig(userID, traderID)
	if err != nil {
		return gin.H{"trader_id": traderID, "trader_name": traderName, "user_id": userID, "ok": false, "error": fmt.Sprintf("读取配置: %v", err)}
	}
	ex := fullConfig.Exchange
	if ex == nil || !ex.Enabled {
		return gin.H{"trader_id": traderID, "trader_name": traderName, "user_id": userID, "ok": false, "error": "交易所未启用或未配置"}
	}
	tempTrader, err := buildExchangeProbeTrader(ex, userID)
	if err != nil {
		return gin.H{"trader_id": traderID, "trader_name": traderName, "user_id": userID, "exchange_type": ex.ExchangeType, "ok": false, "error": err.Error()}
	}
	return s.adminFlattenPositionsWithTrader(tempTrader, ex.ExchangeType, traderID, traderName, userID, onlySymbol)
}

func (s *Server) adminFlattenPositionsWithTrader(tempTrader trader.Trader, exchangeType, traderID, traderName, userID, onlySymbol string) gin.H {
	positions, err := tempTrader.GetPositions()
	if err != nil {
		return gin.H{"trader_id": traderID, "trader_name": traderName, "user_id": userID, "exchange_type": exchangeType, "ok": false, "error": fmt.Sprintf("查询持仓: %v", err)}
	}

	closed := make([]gin.H, 0)
	hasErr := false
	for _, pos := range positions {
		sym, _ := pos["symbol"].(string)
		sym = strings.ToUpper(strings.TrimSpace(sym))
		sideRaw, _ := pos["side"].(string)
		side := strings.ToLower(strings.TrimSpace(sideRaw))
		amt := positionAmtFromInterface(pos["positionAmt"])
		if sym == "" || math.Abs(amt) < 1e-12 {
			continue
		}
		if onlySymbol != "" && sym != onlySymbol {
			continue
		}
		if side != "long" && side != "short" {
			if amt > 0 {
				side = "long"
			} else if amt < 0 {
				side = "short"
			}
		}
		if side != "long" && side != "short" {
			hasErr = true
			closed = append(closed, gin.H{"symbol": sym, "error": "无法识别持仓方向: " + sideRaw})
			continue
		}

		_ = tempTrader.CancelAllOrders(sym)
		var cerr error
		if side == "long" {
			_, cerr = tempTrader.CloseLong(sym, 0)
		} else {
			_, cerr = tempTrader.CloseShort(sym, 0)
		}
		if cerr != nil {
			hasErr = true
			closed = append(closed, gin.H{"symbol": sym, "side": side, "error": cerr.Error()})
			logger.Infof("管理员全平失败 trader=%s %s %s: %v", traderID, sym, side, cerr)
		} else {
			closed = append(closed, gin.H{"symbol": sym, "side": side, "ok": true})
			logger.Infof("管理员全平已提交 trader=%s %s %s", traderID, sym, side)
		}
	}

	return gin.H{
		"trader_id":     traderID,
		"trader_name":   traderName,
		"user_id":       userID,
		"exchange_type": exchangeType,
		"ok":            !hasErr,
		"closed":        closed,
	}
}
