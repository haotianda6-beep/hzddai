package api

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"nofx/store"

	"github.com/gin-gonic/gin"
)

// hzMasterHistoryTraderID uses the highest configured HZ follower ratio as
// the representative strategy history, avoiding duplicate rows from followers.
func hzMasterHistoryTraderID(strategyID string, refs []store.MarketStrategyTraderRef) string {
	bestID := ""
	bestRatio := -1.0
	var bestCreated int64
	for _, ref := range refs {
		if !strings.EqualFold(strings.TrimSpace(ref.ExchangeType), "hz") ||
			strategyIDForMarketStats(ref) != strategyID || strings.TrimSpace(ref.TraderID) == "" {
			continue
		}
		var cfg store.StrategyConfig
		_ = json.Unmarshal([]byte(ref.Config), &cfg)
		ratio := cfg.ComkunMirrorFollowerEquityRatio
		created := ref.CreatedAt.UnixNano()
		if ratio > bestRatio || (ratio == bestRatio && (bestID == "" || created < bestCreated)) {
			bestID, bestRatio, bestCreated = strings.TrimSpace(ref.TraderID), ratio, created
		}
	}
	return bestID
}

func hzMasterTradeHistoryRow(pos *store.TraderPosition, tr *store.Trader, idx int) gin.H {
	direction := "多"
	if strings.EqualFold(strings.TrimSpace(pos.Side), "short") {
		direction = "空"
	}
	quantity := pos.EntryQuantity
	if quantity <= 0 {
		quantity = pos.Quantity
	}
	marginMode := "全仓"
	if tr != nil && !tr.IsCrossMargin {
		marginMode = "逐仓"
	}
	return gin.H{
		"id":              fmt.Sprintf("hz-master-live-%d-%d", pos.ID, idx),
		"symbol":          strings.ToUpper(strings.TrimSpace(pos.Symbol)),
		"contractLabel":   "永续",
		"leverage":        fmt.Sprintf("%d倍", hzStarDisplayLeverage(pos, tr)),
		"marginMode":      marginMode,
		"direction":       direction,
		"status":          "已平仓",
		"opened":          hzStarDisplayTime(pos.EntryTime),
		"entryPrice":      hzStarPriceText(pos.EntryPrice),
		"maxOpenInterest": hzStarQuantityText(pos.Symbol, math.Abs(quantity)),
		"closingPnl":      hzStarPnlText(pos.RealizedPnL),
		"closed":          hzStarDisplayTime(pos.ExitTime),
		"avgClosePrice":   hzStarPriceText(pos.ExitPrice),
		"closedVol":       hzStarQuantityText(pos.Symbol, math.Abs(quantity)),
	}
}

func (s *Server) buildHZMasterTradeHistory(strategyID string, refs []store.MarketStrategyTraderRef) ([]gin.H, error) {
	traderID := hzMasterHistoryTraderID(strategyID, refs)
	if traderID == "" {
		return nil, nil
	}
	tr, err := s.store.Trader().GetByID(traderID)
	if err != nil {
		return nil, err
	}
	positions, err := s.store.Position().GetClosedPositions(traderID, hzStarMarketTradeHistoryLimit)
	if err != nil {
		return nil, err
	}
	sort.Slice(positions, func(i, j int) bool { return positions[i].ExitTime > positions[j].ExitTime })
	rows := make([]gin.H, 0, len(positions))
	for idx, position := range positions {
		rows = append(rows, hzMasterTradeHistoryRow(position, tr, idx))
	}
	return rows, nil
}
