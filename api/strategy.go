package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"net/http"
	"nofx/kernel"
	"nofx/logger"
	"nofx/market"
	"nofx/mcp"
	_ "nofx/mcp/payment"
	_ "nofx/mcp/provider"
	"nofx/store"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// validateStrategyConfig validates strategy configuration and returns warnings
func validateStrategyConfig(config *store.StrategyConfig) []string {
	var warnings []string

	// Validate NofxOS API key if any NofxOS feature is enabled
	if (config.Indicators.EnableQuantData || config.Indicators.EnableOIRanking ||
		config.Indicators.EnableNetFlowRanking || config.Indicators.EnablePriceRanking) &&
		config.Indicators.NofxOSAPIKey == "" {
		warnings = append(warnings, "NofxOS API key is not configured. NofxOS data sources may not work properly.")
	}

	return warnings
}

// handleEstimateTokens estimates token usage for a strategy config (no auth required, pure computation)
func (s *Server) handleEstimateTokens(c *gin.Context) {
	var req struct {
		Config store.StrategyConfig `json:"config" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	estimate := req.Config.EstimateTokens()
	c.JSON(http.StatusOK, estimate)
}

type publicStrategyMarketStats struct {
	UsedBy         int       `json:"used_by"`
	Rating         float64   `json:"rating"`
	Subscribers    int       `json:"subscribers"`
	RunningAgents  int       `json:"running_agents"`
	TotalAgents    int       `json:"total_agents"`
	TotalAUM       float64   `json:"total_aum"`
	Return7DPct    float64   `json:"return_7d_pct"`
	MaxDrawdownPct float64   `json:"max_drawdown_pct"`
	Trend          []float64 `json:"trend"`
	DataComplete   bool      `json:"data_complete"`
}

const publicStrategiesCacheTTL = 20 * time.Second

var publicStrategiesCache = struct {
	sync.RWMutex
	fetchedAt time.Time
	payload   gin.H
}{}

func getPublicStrategiesCache() (gin.H, bool) {
	publicStrategiesCache.RLock()
	defer publicStrategiesCache.RUnlock()
	if publicStrategiesCache.payload == nil || time.Since(publicStrategiesCache.fetchedAt) > publicStrategiesCacheTTL {
		return nil, false
	}
	return publicStrategiesCache.payload, true
}

func setPublicStrategiesCache(payload gin.H) {
	publicStrategiesCache.Lock()
	defer publicStrategiesCache.Unlock()
	publicStrategiesCache.fetchedAt = time.Now()
	publicStrategiesCache.payload = payload
}

const (
	marketDisplaySubscribersMin   = 7
	marketDisplaySubscribersMax   = 38
	marketDisplayRunningAgentsMin = 5
	marketDisplayRunningAgentsMax = 17
)

const (
	hzStarMarketStrategyID        = "b1d9c5f2-c818-4701-8931-962a73b78274"
	hzStarSourceStrategyID        = "bc34778b-c102-4753-9cd5-5d86654a23a4"
	hzStarMarketDisplayInitial    = 10000.0
	hzStarMarketTradeScale        = 100.0
	hzStarMarketTradeHistoryLimit = 120

	ultimateSolMarketStrategyID        = "cff474d3-2d32-44ed-a2a4-813fd142e373"
	ultimateSolMarketDisplayInitial    = 20000.0
	ultimateSolMarketTradeHistoryLimit = 120
)

type hzStarMarketData struct {
	TraderID     string
	ExchangeType string
	Rollup       *store.AggregatedTradingRollup
	Trend        []float64
	TradeHistory []gin.H
}

type ultimateSolMarketData struct {
	TraderID     string
	ExchangeType string
	Rollup       *store.AggregatedTradingRollup
	Trend        []float64
	TradeHistory []gin.H
}

func roundMarket2(v float64) float64 {
	return math.Round(v*100) / 100
}

func sampleMarketTrend(series []float64, n int) []float64 {
	if len(series) == 0 || n <= 0 {
		return nil
	}
	if len(series) <= n {
		return series
	}
	out := make([]float64, 0, n)
	last := len(series) - 1
	for i := 0; i < n; i++ {
		idx := int(float64(i) * float64(last) / float64(n-1))
		out = append(out, series[idx])
	}
	return out
}

func stableMarketAudienceNumber(seed string, minValue, maxValue int) int {
	if maxValue <= minValue {
		return minValue
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	return minValue + int(h.Sum32()%uint32(maxValue-minValue+1))
}

func stableMarketAudienceSeed(strategyID, strategyName string) string {
	seed := strings.TrimSpace(strategyID)
	if seed == "" {
		seed = strings.TrimSpace(strategyName)
	}
	return seed
}

func applyStableMarketAudienceStats(item gin.H, strategyID, strategyName string) {
	stats, ok := item["stats"].(gin.H)
	if !ok {
		return
	}
	seed := stableMarketAudienceSeed(strategyID, strategyName)
	runningAgents := stableMarketAudienceNumber("running:"+seed, marketDisplayRunningAgentsMin, marketDisplayRunningAgentsMax)
	subscribers := stableMarketAudienceNumber("subscribers:"+seed, marketDisplaySubscribersMin, marketDisplaySubscribersMax)
	if subscribers < runningAgents {
		subscribers = runningAgents
	}
	stats["subscribers"] = subscribers
	stats["running_agents"] = runningAgents
	stats["total_agents"] = runningAgents
	stats["used_by"] = subscribers
}

func sharpeFromScaledPnls(pnls []float64, initial float64) float64 {
	if len(pnls) <= 1 || initial <= 0 {
		return 0
	}
	returns := make([]float64, 0, len(pnls))
	for _, pnl := range pnls {
		returns = append(returns, pnl/initial)
	}
	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= float64(len(returns))
	var variance float64
	for _, r := range returns {
		variance += (r - mean) * (r - mean)
	}
	variance /= float64(len(returns) - 1)
	if variance <= 0 {
		return 0
	}
	return roundMarket2((mean / math.Sqrt(variance)) * math.Sqrt(float64(len(returns))))
}

func maxDrawdownFromEquity(series []float64) float64 {
	if len(series) < 2 {
		return 0
	}
	peak := series[0]
	maxDD := 0.0
	for _, eq := range series {
		if eq > peak {
			peak = eq
		}
		if peak > 0 {
			dd := (peak - eq) / peak * 100
			if dd > maxDD {
				maxDD = dd
			}
		}
	}
	return roundMarket2(maxDD)
}

func hzStarDisplayTime(ms int64) string {
	if ms <= 0 {
		return ""
	}
	cst := time.FixedZone("CST", 8*3600)
	return time.UnixMilli(ms).In(cst).Format("2006-01-02 15:04:05")
}

func hzStarNumberWithCommas(v float64, decimals int) string {
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	s := fmt.Sprintf("%.*f", decimals, v)
	parts := strings.SplitN(s, ".", 2)
	intPart := parts[0]
	for i := len(intPart) - 3; i > 0; i -= 3 {
		intPart = intPart[:i] + "," + intPart[i:]
	}
	if len(parts) == 2 {
		return sign + intPart + "." + parts[1]
	}
	return sign + intPart
}

func hzStarPriceText(price float64) string {
	decimals := 2
	if price > 0 && price < 1 {
		decimals = 4
	}
	return hzStarNumberWithCommas(price, decimals) + " USDT"
}

func hzStarPnlText(pnl float64) string {
	prefix := "+"
	if pnl < 0 {
		prefix = "-"
		pnl = -pnl
	}
	return prefix + hzStarNumberWithCommas(pnl, 2) + " USDT"
}

func hzStarQuantityText(symbol string, qty float64) string {
	unit := strings.TrimSuffix(strings.ToUpper(strings.TrimSpace(symbol)), "USDT")
	if unit == "" {
		unit = strings.ToUpper(strings.TrimSpace(symbol))
	}
	decimals := 3
	if qty >= 1000 {
		decimals = 0
	} else if qty >= 100 {
		decimals = 2
	}
	return hzStarNumberWithCommas(qty, decimals) + " " + unit
}

func hzStarDisplayLeverage(pos *store.TraderPosition, tr *store.Trader) int {
	if pos != nil && pos.Leverage > 1 {
		return pos.Leverage
	}
	if tr != nil {
		sym := strings.ToUpper(strings.TrimSpace(pos.Symbol))
		if sym == "BTCUSDT" || sym == "ETHUSDT" {
			if tr.BTCETHLeverage > 0 {
				return tr.BTCETHLeverage
			}
		}
		if tr.AltcoinLeverage > 0 {
			return tr.AltcoinLeverage
		}
	}
	if pos != nil && pos.Leverage > 0 {
		return pos.Leverage
	}
	return 1
}

func hzStarTradeHistoryRow(pos *store.TraderPosition, tr *store.Trader, idx int) gin.H {
	side := strings.ToUpper(strings.TrimSpace(pos.Side))
	direction := "多"
	if side == "SHORT" {
		direction = "空"
	}
	qty := pos.EntryQuantity
	if qty <= 0 {
		qty = pos.Quantity
	}
	scaledQty := math.Abs(qty) * hzStarMarketTradeScale
	scaledClosedQty := math.Abs(pos.Quantity) * hzStarMarketTradeScale
	marginMode := "全仓"
	if tr != nil && !tr.IsCrossMargin {
		marginMode = "逐仓"
	}
	return gin.H{
		"id":              fmt.Sprintf("hz-star-live-%d-%d", pos.ID, idx),
		"symbol":          strings.ToUpper(strings.TrimSpace(pos.Symbol)),
		"contractLabel":   "永续",
		"leverage":        fmt.Sprintf("%d倍", hzStarDisplayLeverage(pos, tr)),
		"marginMode":      marginMode,
		"direction":       direction,
		"status":          "已平仓",
		"opened":          hzStarDisplayTime(pos.EntryTime),
		"entryPrice":      hzStarPriceText(pos.EntryPrice),
		"maxOpenInterest": hzStarQuantityText(pos.Symbol, scaledQty),
		"closingPnl":      hzStarPnlText(pos.RealizedPnL * hzStarMarketTradeScale),
		"closed":          hzStarDisplayTime(pos.ExitTime),
		"avgClosePrice":   hzStarPriceText(pos.ExitPrice),
		"closedVol":       hzStarQuantityText(pos.Symbol, scaledClosedQty),
	}
}

func hzStarSourceTraderID(refs []store.MarketStrategyTraderRef) string {
	for _, ref := range refs {
		if strings.TrimSpace(ref.TraderID) == "" {
			continue
		}
		if strings.TrimSpace(ref.StrategyID) == hzStarSourceStrategyID {
			return strings.TrimSpace(ref.TraderID)
		}
	}
	return ""
}

func (s *Server) buildHzStarMarketData(refs []store.MarketStrategyTraderRef, includeHistory bool) (*hzStarMarketData, error) {
	traderID := hzStarSourceTraderID(refs)
	if traderID == "" {
		var row struct {
			ID string
		}
		err := s.store.GormDB().Table("traders").
			Select("id").
			Where("strategy_id = ? AND name = ?", hzStarSourceStrategyID, "星星").
			Order("created_at ASC").
			Limit(1).
			Scan(&row).Error
		if err != nil {
			return nil, err
		}
		traderID = strings.TrimSpace(row.ID)
	}
	if traderID == "" {
		return nil, nil
	}
	tr, err := s.store.Trader().GetByID(traderID)
	if err != nil {
		return nil, err
	}
	positions, err := s.store.Position().GetClosedPositions(traderID, 500)
	if err != nil {
		return nil, err
	}
	sort.Slice(positions, func(i, j int) bool {
		return positions[i].ExitTime < positions[j].ExitTime
	})

	rollup := &store.AggregatedTradingRollup{}
	var pnls []float64
	equitySeries := []float64{hzStarMarketDisplayInitial}
	equity := hzStarMarketDisplayInitial
	var holdMsSum float64
	var holdN int
	for _, pos := range positions {
		scaledPnL := pos.RealizedPnL * hzStarMarketTradeScale
		scaledFee := pos.Fee * hzStarMarketTradeScale
		rollup.Stats.TotalTrades++
		rollup.Stats.TotalPnL += scaledPnL
		rollup.Stats.TotalFee += scaledFee
		pnls = append(pnls, scaledPnL)
		if scaledPnL > 0 {
			rollup.Stats.WinTrades++
			rollup.GrossProfit += scaledPnL
		} else if scaledPnL < 0 {
			rollup.Stats.LossTrades++
			rollup.GrossLoss += -scaledPnL
		}
		switch strings.ToUpper(strings.TrimSpace(pos.Side)) {
		case "LONG":
			rollup.LongTrades++
		case "SHORT":
			rollup.ShortTrades++
		}
		if pos.ExitTime > 0 && pos.EntryTime > 0 {
			holdMsSum += float64(pos.ExitTime - pos.EntryTime)
			holdN++
		}
		equity += scaledPnL
		equitySeries = append(equitySeries, roundMarket2(equity))
	}
	if holdN > 0 {
		rollup.AvgHoldMs = holdMsSum / float64(holdN)
	}
	if rollup.Stats.TotalTrades > 0 {
		rollup.Stats.WinRate = roundMarket2(float64(rollup.Stats.WinTrades) / float64(rollup.Stats.TotalTrades) * 100)
	}
	if rollup.GrossLoss > 0 {
		rollup.Stats.ProfitFactor = roundMarket2(rollup.GrossProfit / rollup.GrossLoss)
	} else if rollup.GrossProfit > 0 {
		rollup.Stats.ProfitFactor = 999
	}
	if rollup.Stats.WinTrades > 0 {
		rollup.Stats.AvgWin = roundMarket2(rollup.GrossProfit / float64(rollup.Stats.WinTrades))
	}
	if rollup.Stats.LossTrades > 0 {
		rollup.Stats.AvgLoss = roundMarket2(rollup.GrossLoss / float64(rollup.Stats.LossTrades))
	}
	rollup.Stats.TotalPnL = roundMarket2(rollup.Stats.TotalPnL)
	rollup.Stats.TotalFee = roundMarket2(rollup.Stats.TotalFee)
	rollup.GrossProfit = roundMarket2(rollup.GrossProfit)
	rollup.GrossLoss = roundMarket2(rollup.GrossLoss)
	rollup.Stats.SharpeRatio = sharpeFromScaledPnls(pnls, hzStarMarketDisplayInitial)
	rollup.Stats.MaxDrawdownPct = maxDrawdownFromEquity(equitySeries)

	out := &hzStarMarketData{
		TraderID: traderID,
		Rollup:   rollup,
		Trend:    equitySeries,
	}
	var et string
	if err := s.store.GormDB().Raw(
		`SELECT e.exchange_type FROM traders t LEFT JOIN exchanges e ON e.id = t.exchange_id WHERE t.id = ? LIMIT 1`,
		traderID,
	).Scan(&et).Error; err == nil {
		out.ExchangeType = strings.ToUpper(strings.TrimSpace(et))
	}
	if includeHistory {
		sort.Slice(positions, func(i, j int) bool {
			return positions[i].ExitTime > positions[j].ExitTime
		})
		limit := hzStarMarketTradeHistoryLimit
		if len(positions) < limit {
			limit = len(positions)
		}
		out.TradeHistory = make([]gin.H, 0, limit)
		for i := 0; i < limit; i++ {
			out.TradeHistory = append(out.TradeHistory, hzStarTradeHistoryRow(positions[i], tr, i))
		}
	}
	return out, nil
}

func (s *Server) applyHzStarMarketListOverlay(item gin.H, refs []store.MarketStrategyTraderRef) error {
	data, err := s.buildHzStarMarketData(refs, false)
	if err != nil || data == nil || data.Rollup == nil {
		return err
	}
	stats, ok := item["stats"].(gin.H)
	if !ok {
		return nil
	}
	stats["data_complete"] = true
	stats["total_aum"] = roundMarket2(hzStarMarketDisplayInitial + data.Rollup.Stats.TotalPnL)
	stats["return_7d_pct"] = roundMarket2((data.Rollup.Stats.TotalPnL / hzStarMarketDisplayInitial) * 100)
	stats["max_drawdown_pct"] = data.Rollup.Stats.MaxDrawdownPct
	stats["trend"] = sampleMarketTrend(data.Trend, 18)
	if data.ExchangeType != "" {
		item["exchange_type"] = data.ExchangeType
	}
	return nil
}

func ultimateSolTradeScale(tr *store.Trader) float64 {
	if tr == nil || tr.InitialBalance <= 0 {
		return 1
	}
	scale := ultimateSolMarketDisplayInitial / tr.InitialBalance
	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		return 1
	}
	return scale
}

func ultimateSolTradeHistoryRow(pos *store.TraderPosition, tr *store.Trader, idx int, scale float64) gin.H {
	side := strings.ToUpper(strings.TrimSpace(pos.Side))
	direction := "多"
	if side == "SHORT" {
		direction = "空"
	}
	qty := pos.EntryQuantity
	if qty <= 0 {
		qty = pos.Quantity
	}
	scaledQty := math.Abs(qty) * scale
	scaledClosedQty := math.Abs(pos.Quantity) * scale
	marginMode := "全仓"
	if tr != nil && !tr.IsCrossMargin {
		marginMode = "逐仓"
	}
	return gin.H{
		"id":              fmt.Sprintf("ultimate-sol-live-%d-%d", pos.ID, idx),
		"symbol":          strings.ToUpper(strings.TrimSpace(pos.Symbol)),
		"contractLabel":   "永续",
		"leverage":        fmt.Sprintf("%d倍", hzStarDisplayLeverage(pos, tr)),
		"marginMode":      marginMode,
		"direction":       direction,
		"status":          "已平仓",
		"opened":          hzStarDisplayTime(pos.EntryTime),
		"entryPrice":      hzStarPriceText(pos.EntryPrice),
		"maxOpenInterest": hzStarQuantityText(pos.Symbol, scaledQty),
		"closingPnl":      hzStarPnlText(pos.RealizedPnL * scale),
		"closed":          hzStarDisplayTime(pos.ExitTime),
		"avgClosePrice":   hzStarPriceText(pos.ExitPrice),
		"closedVol":       hzStarQuantityText(pos.Symbol, scaledClosedQty),
	}
}

func (s *Server) ultimateSolSourceTraderID() (string, error) {
	var row struct {
		ID string
	}
	err := s.store.GormDB().Table("traders").
		Select("id").
		Where("strategy_id = ? AND name = ?", ultimateSolMarketStrategyID, "主控sol").
		Order("created_at ASC").
		Limit(1).
		Scan(&row).Error
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(row.ID) != "" {
		return strings.TrimSpace(row.ID), nil
	}
	err = s.store.GormDB().Table("traders").
		Select("id").
		Where("strategy_id = ?", ultimateSolMarketStrategyID).
		Order("created_at ASC").
		Limit(1).
		Scan(&row).Error
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(row.ID), nil
}

func (s *Server) buildUltimateSolMarketData(includeHistory bool) (*ultimateSolMarketData, error) {
	traderID, err := s.ultimateSolSourceTraderID()
	if err != nil {
		return nil, err
	}
	if traderID == "" {
		return nil, nil
	}
	tr, err := s.store.Trader().GetByID(traderID)
	if err != nil {
		return nil, err
	}
	scale := ultimateSolTradeScale(tr)
	positions, err := s.store.Position().GetClosedPositions(traderID, 500)
	if err != nil {
		return nil, err
	}
	sort.Slice(positions, func(i, j int) bool {
		return positions[i].ExitTime < positions[j].ExitTime
	})

	rollup := &store.AggregatedTradingRollup{}
	var pnls []float64
	equitySeries := []float64{ultimateSolMarketDisplayInitial}
	equity := ultimateSolMarketDisplayInitial
	var holdMsSum float64
	var holdN int
	for _, pos := range positions {
		scaledPnL := pos.RealizedPnL * scale
		scaledFee := pos.Fee * scale
		rollup.Stats.TotalTrades++
		rollup.Stats.TotalPnL += scaledPnL
		rollup.Stats.TotalFee += scaledFee
		pnls = append(pnls, scaledPnL)
		if scaledPnL > 0 {
			rollup.Stats.WinTrades++
			rollup.GrossProfit += scaledPnL
		} else if scaledPnL < 0 {
			rollup.Stats.LossTrades++
			rollup.GrossLoss += -scaledPnL
		}
		switch strings.ToUpper(strings.TrimSpace(pos.Side)) {
		case "LONG":
			rollup.LongTrades++
		case "SHORT":
			rollup.ShortTrades++
		}
		if pos.ExitTime > 0 && pos.EntryTime > 0 {
			holdMsSum += float64(pos.ExitTime - pos.EntryTime)
			holdN++
		}
		equity += scaledPnL
		equitySeries = append(equitySeries, roundMarket2(equity))
	}
	if holdN > 0 {
		rollup.AvgHoldMs = holdMsSum / float64(holdN)
	}
	if rollup.Stats.TotalTrades > 0 {
		rollup.Stats.WinRate = roundMarket2(float64(rollup.Stats.WinTrades) / float64(rollup.Stats.TotalTrades) * 100)
	}
	if rollup.GrossLoss > 0 {
		rollup.Stats.ProfitFactor = roundMarket2(rollup.GrossProfit / rollup.GrossLoss)
	} else if rollup.GrossProfit > 0 {
		rollup.Stats.ProfitFactor = 999
	}
	if rollup.Stats.WinTrades > 0 {
		rollup.Stats.AvgWin = roundMarket2(rollup.GrossProfit / float64(rollup.Stats.WinTrades))
	}
	if rollup.Stats.LossTrades > 0 {
		rollup.Stats.AvgLoss = roundMarket2(rollup.GrossLoss / float64(rollup.Stats.LossTrades))
	}
	rollup.Stats.TotalPnL = roundMarket2(rollup.Stats.TotalPnL)
	rollup.Stats.TotalFee = roundMarket2(rollup.Stats.TotalFee)
	rollup.GrossProfit = roundMarket2(rollup.GrossProfit)
	rollup.GrossLoss = roundMarket2(rollup.GrossLoss)
	rollup.Stats.SharpeRatio = sharpeFromScaledPnls(pnls, ultimateSolMarketDisplayInitial)
	rollup.Stats.MaxDrawdownPct = maxDrawdownFromEquity(equitySeries)

	out := &ultimateSolMarketData{
		TraderID: traderID,
		Rollup:   rollup,
		Trend:    equitySeries,
	}
	var et string
	if err := s.store.GormDB().Raw(
		`SELECT e.exchange_type FROM traders t LEFT JOIN exchanges e ON e.id = t.exchange_id WHERE t.id = ? LIMIT 1`,
		traderID,
	).Scan(&et).Error; err == nil {
		out.ExchangeType = strings.ToUpper(strings.TrimSpace(et))
	}
	if includeHistory {
		sort.Slice(positions, func(i, j int) bool {
			return positions[i].ExitTime > positions[j].ExitTime
		})
		limit := ultimateSolMarketTradeHistoryLimit
		if len(positions) < limit {
			limit = len(positions)
		}
		out.TradeHistory = make([]gin.H, 0, limit)
		for i := 0; i < limit; i++ {
			out.TradeHistory = append(out.TradeHistory, ultimateSolTradeHistoryRow(positions[i], tr, i, scale))
		}
	}
	return out, nil
}

func (s *Server) applyUltimateSolMarketListOverlay(item gin.H) error {
	data, err := s.buildUltimateSolMarketData(false)
	if err != nil || data == nil || data.Rollup == nil {
		return err
	}
	stats, ok := item["stats"].(gin.H)
	if !ok {
		return nil
	}
	stats["data_complete"] = true
	stats["total_aum"] = roundMarket2(ultimateSolMarketDisplayInitial + data.Rollup.Stats.TotalPnL)
	stats["return_7d_pct"] = roundMarket2((data.Rollup.Stats.TotalPnL / ultimateSolMarketDisplayInitial) * 100)
	stats["max_drawdown_pct"] = data.Rollup.Stats.MaxDrawdownPct
	stats["trend"] = sampleMarketTrend(data.Trend, 18)
	if data.ExchangeType != "" {
		item["exchange_type"] = data.ExchangeType
	}
	return nil
}

// buildPublicStrategyItem 策略市场列表项：按 market_access 返回不同可见字段
func buildPublicStrategyItem(st *store.Strategy, creator *store.User, stats publicStrategyMarketStats) gin.H {
	access := store.EffectivePublicListingAccess(st)
	var cfg store.StrategyConfig
	_ = json.Unmarshal([]byte(st.Config), &cfg)

	item := gin.H{
		"id":            st.ID,
		"name":          st.Name,
		"description":   st.Description,
		"author_email":  "",
		"market_access": access,
		"is_public":     store.IsListedOnMarket(access),
		// 仅「开源」在策略市场暴露完整配置；「公开 / 需订阅」只展示元数据，复制到本人策略后才可见完整 JSON
		"config_visible":  access == store.MarketAccessOpenSource,
		"market_revision": st.MarketRevision,
		"market_ai_model": strings.TrimSpace(st.MarketAIModel),
		"created_at":      st.CreatedAt,
		"updated_at":      st.UpdatedAt,
		"stats": gin.H{
			"used_by":          stats.UsedBy,
			"rating":           stats.Rating,
			"subscribers":      stats.Subscribers,
			"running_agents":   stats.RunningAgents,
			"total_agents":     stats.TotalAgents,
			"total_aum":        stats.TotalAUM,
			"return_7d_pct":    stats.Return7DPct,
			"max_drawdown_pct": stats.MaxDrawdownPct,
			"trend":            stats.Trend,
			"data_complete":    stats.DataComplete,
		},
	}
	// 兜底：未设置 market_ai_model 时按策略类型给一个稳定展示（不影响实际交易员模型选择）
	if strings.TrimSpace(st.MarketAIModel) == "" {
		if cfg.ComkunFollowListingTemplate || cfg.ComkunMarketFollow {
			item["market_ai_model"] = "comkun_ai"
		} else if strings.TrimSpace(cfg.Language) == "en" {
			item["market_ai_model"] = "claude"
		} else {
			item["market_ai_model"] = "deepseek"
		}
	}
	if creator != nil {
		item["creator_display_name"] = creator.DisplayName
		item["creator_avatar_url"] = creator.AvatarURL
	} else {
		item["creator_display_name"] = ""
		item["creator_avatar_url"] = ""
	}
	if access == store.MarketAccessSubscription && (store.IsComkunMarketFollowStrategy(&cfg) || cfg.ComkunFollowListingTemplate) {
		item["market_sale_price_usdt"] = 0
	} else if cfg.MarketSalePriceUSDT > 0 {
		item["market_sale_price_usdt"] = cfg.MarketSalePriceUSDT
	}
	applyOkxScreenMarketIdentityOverlay(item, st.ID)

	switch access {
	case store.MarketAccessOpenSource:
		item["config"] = cfg
		item["open_source_bundle"] = gin.H{
			"strategy_prompt": cfg.StrategyPrompt,
			"prompt_sections": cfg.PromptSections,
			"language":        cfg.Language,
			"strategy_type":   cfg.StrategyType,
			"custom_prompt":   cfg.CustomPrompt,
		}
	default:
		// private / subscription / public：不返回配置正文（公开与需订阅仅允许用户复制到自己的策略后查看）
	}
	return item
}

func runningAgentCountsByMarketStrategy(refs []store.RunningStrategyRef) map[string]int {
	out := make(map[string]int)
	for _, ref := range refs {
		sid := strings.TrimSpace(ref.StrategyID)
		if sid != "" {
			out[sid]++
		}
		var cfg store.StrategyConfig
		if strings.TrimSpace(ref.Config) != "" && json.Unmarshal([]byte(ref.Config), &cfg) == nil {
			sourceID := strings.TrimSpace(cfg.ComkunMarketSourceStrategyID)
			if cfg.ComkunMarketFollow && sourceID != "" && sourceID != sid {
				out[sourceID]++
			}
		}
	}
	return out
}

func strategyIDForMarketStats(ref store.MarketStrategyTraderRef) string {
	sid := strings.TrimSpace(ref.StrategyID)
	var cfg store.StrategyConfig
	if strings.TrimSpace(ref.Config) != "" && json.Unmarshal([]byte(ref.Config), &cfg) == nil {
		sourceID := strings.TrimSpace(cfg.ComkunMarketSourceStrategyID)
		if cfg.ComkunMarketFollow && sourceID != "" {
			return sourceID
		}
	}
	return sid
}

// primaryMarketExchangeForStrategy 绑定该策略的交易员里，取创建最早的一条所用交易所类型（与详情页 exchange_label 同源逻辑）
func primaryMarketExchangeForStrategy(strategyID string, refs []store.MarketStrategyTraderRef) string {
	type cand struct {
		et string
		ts time.Time
	}
	var list []cand
	for _, ref := range refs {
		if strategyIDForMarketStats(ref) != strategyID {
			continue
		}
		et := strings.TrimSpace(ref.ExchangeType)
		if et == "" {
			continue
		}
		list = append(list, cand{et: et, ts: ref.CreatedAt})
	}
	if len(list) == 0 {
		return ""
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].ts.Before(list[j].ts)
	})
	return list[0].et
}

func buildPublicStrategyStats(
	strategyIDs []string,
	refs []store.MarketStrategyTraderRef,
	subCounts map[string]int,
	latest map[string]*store.EquitySnapshot,
	history map[string][]*store.EquitySnapshot,
) map[string]publicStrategyMarketStats {
	out := make(map[string]publicStrategyMarketStats, len(strategyIDs))
	for _, id := range strategyIDs {
		out[id] = publicStrategyMarketStats{Subscribers: subCounts[id], DataComplete: true}
	}
	groupTraderIDs := map[string][]string{}
	for _, ref := range refs {
		sid := strategyIDForMarketStats(ref)
		if sid == "" {
			continue
		}
		st := out[sid]
		st.TotalAgents++
		if ref.IsRunning {
			st.RunningAgents++
		}
		if snap := latest[ref.TraderID]; snap != nil && snap.TotalEquity > 0 {
			st.TotalAUM += snap.TotalEquity
		} else if ref.InitialBalance > 0 {
			st.TotalAUM += ref.InitialBalance
			st.DataComplete = false
		}
		out[sid] = st
		groupTraderIDs[sid] = append(groupTraderIDs[sid], ref.TraderID)
	}
	for sid, traderIDs := range groupTraderIDs {
		st := out[sid]
		st.UsedBy = st.Subscribers
		if st.TotalAgents > st.UsedBy {
			st.UsedBy = st.TotalAgents
		}
		startSum := 0.0
		endSum := 0.0
		var combined []*store.EquitySnapshot
		for _, tid := range traderIDs {
			points := history[tid]
			if len(points) == 0 {
				st.DataComplete = false
				continue
			}
			first := points[0]
			last := points[len(points)-1]
			if first.TotalEquity > 0 && last.TotalEquity > 0 {
				startSum += first.TotalEquity
				endSum += last.TotalEquity
			}
			combined = append(combined, points...)
			dd := maxDrawdownPct(points)
			if dd > st.MaxDrawdownPct {
				st.MaxDrawdownPct = dd
			}
		}
		if startSum > 0 {
			st.Return7DPct = math.Round(((endSum-startSum)/startSum*100)*100) / 100
		} else {
			st.DataComplete = false
		}
		st.Trend = strategyEquityTrend(traderIDs, combined, 18)
		if len(st.Trend) == 0 && st.TotalAUM > 0 {
			st.Trend = []float64{st.TotalAUM}
		}
		out[sid] = st
	}
	for sid, st := range out {
		if st.UsedBy == 0 {
			st.UsedBy = st.Subscribers
		}
		out[sid] = st
	}
	return out
}

func maxDrawdownPct(points []*store.EquitySnapshot) float64 {
	peak := 0.0
	maxDD := 0.0
	for _, p := range points {
		eq := p.TotalEquity
		if eq <= 0 {
			continue
		}
		if eq > peak {
			peak = eq
		}
		if peak > 0 {
			dd := (peak - eq) / peak * 100
			if dd > maxDD {
				maxDD = dd
			}
		}
	}
	return math.Round(maxDD*100) / 100
}

func strategyEquityTrend(traderIDs []string, points []*store.EquitySnapshot, n int) []float64 {
	if len(points) == 0 || n <= 0 {
		return nil
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Timestamp.Before(points[j].Timestamp) })
	latest := make(map[string]float64, len(traderIDs))
	series := make([]float64, 0, len(points))
	for _, p := range points {
		if p.TotalEquity <= 0 {
			continue
		}
		latest[p.TraderID] = p.TotalEquity
		sum := 0.0
		for _, v := range latest {
			sum += v
		}
		if sum > 0 {
			series = append(series, sum)
		}
	}
	if len(series) <= n {
		return series
	}
	out := make([]float64, 0, n)
	last := len(series) - 1
	for i := 0; i < n; i++ {
		idx := int(float64(i) * float64(last) / float64(n-1))
		out = append(out, series[idx])
	}
	return out
}

// handlePublicStrategies Get public strategies for strategy market (no auth required)
func (s *Server) handlePublicStrategies(c *gin.Context) {
	if payload, ok := getPublicStrategiesCache(); ok {
		c.JSON(http.StatusOK, payload)
		return
	}

	strategies, err := s.store.Strategy().ListPublic()
	if err != nil {
		SafeInternalError(c, "Failed to get public strategies", err)
		return
	}

	marketRefs, err := s.store.Trader().ListMarketStrategyTraderRefs()
	if err != nil {
		SafeInternalError(c, "Failed to load running strategy stats", err)
		return
	}
	traderIDs := make([]string, 0, len(marketRefs))
	for _, ref := range marketRefs {
		if strings.TrimSpace(ref.TraderID) != "" {
			traderIDs = append(traderIDs, ref.TraderID)
		}
	}
	latestEquity, err := s.store.Equity().GetLatestByTraderIDs(traderIDs)
	if err != nil {
		SafeInternalError(c, "Failed to load latest equity stats", err)
		return
	}
	historyEquity, err := s.store.Equity().GetByTraderIDsSince(traderIDs, time.Now().UTC().AddDate(0, 0, -7))
	if err != nil {
		SafeInternalError(c, "Failed to load equity history stats", err)
		return
	}

	seen := make(map[string]struct{})
	var userIDs []string
	for _, st := range strategies {
		if st.UserID == "" {
			continue
		}
		if _, ok := seen[st.UserID]; ok {
			continue
		}
		seen[st.UserID] = struct{}{}
		userIDs = append(userIDs, st.UserID)
	}
	userMap, err := s.store.User().GetMapByIDs(userIDs)
	if err != nil {
		SafeInternalError(c, "Failed to load strategy authors", err)
		return
	}

	result := make([]gin.H, 0, len(strategies))
	strategyIDs := make([]string, 0, len(strategies))
	for _, st := range strategies {
		strategyIDs = append(strategyIDs, st.ID)
	}
	subCounts, err := s.store.Billing().CountEntitlementsByStrategyIDs(strategyIDs)
	if err != nil {
		SafeInternalError(c, "Failed to load subscription stats", err)
		return
	}
	statsByID := buildPublicStrategyStats(strategyIDs, marketRefs, subCounts, latestEquity, historyEquity)
	for _, st := range strategies {
		var creator *store.User
		if u, ok := userMap[st.UserID]; ok {
			cu := u
			creator = &cu
		}
		item := buildPublicStrategyItem(st, creator, statsByID[st.ID])
		if et := primaryMarketExchangeForStrategy(st.ID, marketRefs); et != "" {
			item["exchange_type"] = strings.ToUpper(et)
		}
		if st.ID == hzStarMarketStrategyID {
			if err := s.applyHzStarMarketListOverlay(item, marketRefs); err != nil {
				SafeInternalError(c, "Failed to load HZ star market stats", err)
				return
			}
		}
		if st.ID == ultimateSolMarketStrategyID {
			if err := s.applyUltimateSolMarketListOverlay(item); err != nil {
				SafeInternalError(c, "Failed to load ultimate SOL market stats", err)
				return
			}
		}
		if st.ID == okx01MarketStrategyID {
			applyOkx01MarketListOverlay(item)
		}
		if st.ID == okx02MarketStrategyID {
			applyOkx02MarketListOverlay(item)
		}
		if st.ID == okx03MarketStrategyID {
			applyOkx03MarketListOverlay(item)
		}
		if st.ID == okx04MarketStrategyID {
			applyOkx04MarketListOverlay(item)
		}
		if st.ID == okx05MarketStrategyID {
			applyOkx05MarketListOverlay(item)
		}
		if st.ID == okx06MarketStrategyID {
			applyOkx06MarketListOverlay(item)
		}
		if st.ID == okx07MarketStrategyID {
			applyOkx07MarketListOverlay(item)
		}
		if st.ID == okx08MarketStrategyID {
			applyOkx08MarketListOverlay(item)
		}
		if st.ID != ultimateSolMarketStrategyID && st.ID != okx01MarketStrategyID && st.ID != okx02MarketStrategyID && st.ID != okx03MarketStrategyID && st.ID != okx04MarketStrategyID && st.ID != okx05MarketStrategyID && st.ID != okx06MarketStrategyID && st.ID != okx07MarketStrategyID && st.ID != okx08MarketStrategyID {
			applyPublicStrategyMarketDemoOverlay(item, st.ID, st.Name, st.MarketRevision)
		}
		applyStableMarketAudienceStats(item, st.ID, st.Name)
		applyOkxScreenMarketDisplayExchangeOverlay(item, st.ID)
		result = append(result, item)
	}

	payload := gin.H{
		"strategies": result,
	}
	setPublicStrategiesCache(payload)
	c.JSON(http.StatusOK, payload)
}

// handlePublicStrategyDetail 策略市场详情页：公开上架策略 + 汇总交易员净值与成交统计（无需登录）
func (s *Server) handlePublicStrategyDetail(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		SafeBadRequest(c, "Missing strategy id")
		return
	}

	st, err := s.store.Strategy().GetByIDForMarket(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Strategy not found"})
		return
	}
	marketDetailID := strings.TrimSpace(st.SourceStrategyID)
	if marketDetailID == "" {
		marketDetailID = st.ID
	}

	marketRefs, err := s.store.Trader().ListMarketStrategyTraderRefs()
	if err != nil {
		SafeInternalError(c, "Failed to load trader refs", err)
		return
	}

	traderIDSeen := make(map[string]struct{})
	initialCapitalSum := 0.0
	for _, ref := range marketRefs {
		if strategyIDForMarketStats(ref) != marketDetailID {
			continue
		}
		tid := strings.TrimSpace(ref.TraderID)
		if tid == "" {
			continue
		}
		traderIDSeen[tid] = struct{}{}
		if ref.InitialBalance > 0 {
			initialCapitalSum += ref.InitialBalance
		}
	}
	traderIDs := make([]string, 0, len(traderIDSeen))
	for tid := range traderIDSeen {
		traderIDs = append(traderIDs, tid)
	}
	sort.Strings(traderIDs)

	since := time.Now().UTC().AddDate(0, 0, -90)
	historyEquity, err := s.store.Equity().GetByTraderIDsSince(traderIDs, since)
	if err != nil {
		SafeInternalError(c, "Failed to load equity history", err)
		return
	}
	latestEquity, err := s.store.Equity().GetLatestByTraderIDs(traderIDs)
	if err != nil {
		SafeInternalError(c, "Failed to load latest equity", err)
		return
	}
	subCounts, err := s.store.Billing().CountEntitlementsByStrategyIDs([]string{marketDetailID})
	if err != nil {
		SafeInternalError(c, "Failed to load subscription stats", err)
		return
	}
	statsByID := buildPublicStrategyStats([]string{marketDetailID}, marketRefs, subCounts, latestEquity, historyEquity)
	pub := statsByID[marketDetailID]

	rollup, err := s.store.Position().GetAggregatedTradingStatsForTraders(traderIDs)
	if err != nil {
		SafeInternalError(c, "Failed to load trading rollup", err)
		return
	}

	var creator *store.User
	if strings.TrimSpace(st.UserID) != "" {
		if u, err := s.store.User().GetByID(st.UserID); err == nil {
			creator = u
		}
	}

	item := buildPublicStrategyItem(st, creator, pub)
	if gh, ok := item["stats"].(gin.H); ok {
		gh["stats_window_days"] = 90
	}

	statsSource := "live"
	demoMeta := gin.H{
		"enabled": false,
		"env_key": "NOFX_MARKET_DETAIL_DEMO",
	}
	var tradeHistory []gin.H
	hzStarApplied := false
	hzStarExchangeLabel := ""
	ultimateSolApplied := false
	ultimateSolExchangeLabel := ""
	okx01Applied := false
	okx01ExchangeLabel := ""
	okx02Applied := false
	okx02ExchangeLabel := ""
	okx03Applied := false
	okx03ExchangeLabel := ""
	okx04Applied := false
	okx04ExchangeLabel := ""
	okx05Applied := false
	okx05ExchangeLabel := ""
	okx06Applied := false
	okx06ExchangeLabel := ""
	okx07Applied := false
	okx07ExchangeLabel := ""
	okx08Applied := false
	okx08ExchangeLabel := ""
	if marketDetailID == hzStarMarketStrategyID {
		hzData, hzErr := s.buildHzStarMarketData(marketRefs, true)
		if hzErr != nil {
			SafeInternalError(c, "Failed to load HZ star market detail", hzErr)
			return
		}
		if hzData != nil && hzData.Rollup != nil {
			hzStarApplied = true
			initialCapitalSum = hzStarMarketDisplayInitial
			rollup = hzData.Rollup
			tradeHistory = hzData.TradeHistory
			if gh, ok := item["stats"].(gin.H); ok {
				gh["data_complete"] = true
				gh["return_7d_pct"] = roundMarket2((hzData.Rollup.Stats.TotalPnL / hzStarMarketDisplayInitial) * 100)
				gh["max_drawdown_pct"] = hzData.Rollup.Stats.MaxDrawdownPct
				gh["stats_window_days"] = 90
				gh["trend"] = hzData.Trend
				gh["total_aum"] = roundMarket2(hzStarMarketDisplayInitial + hzData.Rollup.Stats.TotalPnL)
			}
			if hzData.ExchangeType != "" {
				item["exchange_type"] = hzData.ExchangeType
				hzStarExchangeLabel = hzData.ExchangeType
			}
		}
	}
	if marketDetailID == ultimateSolMarketStrategyID {
		solData, solErr := s.buildUltimateSolMarketData(true)
		if solErr != nil {
			SafeInternalError(c, "Failed to load ultimate SOL market detail", solErr)
			return
		}
		if solData != nil && solData.Rollup != nil {
			ultimateSolApplied = true
			initialCapitalSum = ultimateSolMarketDisplayInitial
			rollup = solData.Rollup
			tradeHistory = solData.TradeHistory
			if gh, ok := item["stats"].(gin.H); ok {
				gh["data_complete"] = true
				gh["return_7d_pct"] = roundMarket2((solData.Rollup.Stats.TotalPnL / ultimateSolMarketDisplayInitial) * 100)
				gh["max_drawdown_pct"] = solData.Rollup.Stats.MaxDrawdownPct
				gh["stats_window_days"] = 90
				gh["trend"] = solData.Trend
				gh["total_aum"] = roundMarket2(ultimateSolMarketDisplayInitial + solData.Rollup.Stats.TotalPnL)
			}
			if solData.ExchangeType != "" {
				item["exchange_type"] = solData.ExchangeType
				ultimateSolExchangeLabel = solData.ExchangeType
			}
		}
	}
	if marketDetailID == okx02MarketStrategyID {
		okx02Data := buildOkx02MarketData(true)
		if okx02Data != nil && okx02Data.Rollup != nil {
			okx02Applied = true
			initialCapitalSum = okx02MarketDisplayInitial
			rollup = okx02Data.Rollup
			tradeHistory = okx02Data.TradeHistory
			if gh, ok := item["stats"].(gin.H); ok {
				gh["data_complete"] = true
				gh["return_7d_pct"] = roundMarket2((okx02Data.Rollup.Stats.TotalPnL / okx02MarketDisplayInitial) * 100)
				gh["max_drawdown_pct"] = okx02Data.Rollup.Stats.MaxDrawdownPct
				gh["stats_window_days"] = okx02MarketStatsWindowDays
				gh["trend"] = sampleMarketTrend(okx02Data.Trend, 18)
				gh["total_aum"] = roundMarket2(okx02MarketDisplayInitial + okx02Data.Rollup.Stats.TotalPnL)
			}
			item["exchange_type"] = "OKX"
			okx02ExchangeLabel = "OKX"
		}
	}
	if marketDetailID == okx01MarketStrategyID {
		okx01Data := buildOkx01MarketData(true)
		if okx01Data != nil && okx01Data.Rollup != nil {
			okx01Applied = true
			initialCapitalSum = okx01MarketDisplayInitial
			rollup = okx01Data.Rollup
			tradeHistory = okx01Data.TradeHistory
			if gh, ok := item["stats"].(gin.H); ok {
				gh["data_complete"] = true
				gh["return_7d_pct"] = roundMarket2((okx01Data.Rollup.Stats.TotalPnL / okx01MarketDisplayInitial) * 100)
				gh["max_drawdown_pct"] = okx01Data.Rollup.Stats.MaxDrawdownPct
				gh["stats_window_days"] = okx01MarketStatsWindowDays
				gh["trend"] = sampleMarketTrend(okx01Data.Trend, 18)
				gh["total_aum"] = roundMarket2(okx01MarketDisplayInitial + okx01Data.Rollup.Stats.TotalPnL)
			}
			item["exchange_type"] = "OKX"
			okx01ExchangeLabel = "OKX"
		}
	}
	if marketDetailID == okx03MarketStrategyID {
		okx03Data := buildOkx03MarketData(true)
		if okx03Data != nil && okx03Data.Rollup != nil {
			okx03Applied = true
			initialCapitalSum = okx03MarketDisplayInitial
			rollup = okx03Data.Rollup
			tradeHistory = okx03Data.TradeHistory
			if gh, ok := item["stats"].(gin.H); ok {
				gh["data_complete"] = true
				gh["return_7d_pct"] = roundMarket2((okx03Data.Rollup.Stats.TotalPnL / okx03MarketDisplayInitial) * 100)
				gh["max_drawdown_pct"] = okx03Data.Rollup.Stats.MaxDrawdownPct
				gh["stats_window_days"] = okx03MarketStatsWindowDays
				gh["trend"] = sampleMarketTrend(okx03Data.Trend, 18)
				gh["total_aum"] = roundMarket2(okx03MarketDisplayInitial + okx03Data.Rollup.Stats.TotalPnL)
			}
			item["exchange_type"] = "OKX"
			okx03ExchangeLabel = "OKX"
		}
	}
	if marketDetailID == okx04MarketStrategyID {
		okx04Data := buildOkx04MarketData(true)
		if okx04Data != nil && okx04Data.Rollup != nil {
			okx04Applied = true
			initialCapitalSum = okx04MarketDisplayInitial
			rollup = okx04Data.Rollup
			tradeHistory = okx04Data.TradeHistory
			if gh, ok := item["stats"].(gin.H); ok {
				gh["data_complete"] = true
				gh["return_7d_pct"] = roundMarket2((okx04Data.Rollup.Stats.TotalPnL / okx04MarketDisplayInitial) * 100)
				gh["max_drawdown_pct"] = okx04Data.Rollup.Stats.MaxDrawdownPct
				gh["stats_window_days"] = okx04MarketStatsWindowDays
				gh["trend"] = sampleMarketTrend(okx04Data.Trend, 18)
				gh["total_aum"] = roundMarket2(okx04MarketDisplayInitial + okx04Data.Rollup.Stats.TotalPnL)
			}
			item["exchange_type"] = "OKX"
			okx04ExchangeLabel = "OKX"
		}
	}
	if marketDetailID == okx05MarketStrategyID {
		okx05Data := buildOkx05MarketData(true)
		if okx05Data != nil && okx05Data.Rollup != nil {
			okx05Applied = true
			initialCapitalSum = okx05MarketDisplayInitial
			rollup = okx05Data.Rollup
			tradeHistory = okx05Data.TradeHistory
			if gh, ok := item["stats"].(gin.H); ok {
				gh["data_complete"] = true
				gh["return_7d_pct"] = roundMarket2((okx05Data.Rollup.Stats.TotalPnL / okx05MarketDisplayInitial) * 100)
				gh["max_drawdown_pct"] = okx05Data.Rollup.Stats.MaxDrawdownPct
				gh["stats_window_days"] = okx05MarketStatsWindowDays
				gh["trend"] = sampleMarketTrend(okx05Data.Trend, 18)
				gh["total_aum"] = roundMarket2(okx05MarketDisplayInitial + okx05Data.Rollup.Stats.TotalPnL)
			}
			item["exchange_type"] = "OKX"
			okx05ExchangeLabel = "OKX"
		}
	}
	if marketDetailID == okx06MarketStrategyID {
		okx06Data := buildOkx06MarketData(true)
		if okx06Data != nil && okx06Data.Rollup != nil {
			okx06Applied = true
			initialCapitalSum = okx06MarketDisplayInitial
			rollup = okx06Data.Rollup
			tradeHistory = okx06Data.TradeHistory
			if gh, ok := item["stats"].(gin.H); ok {
				gh["data_complete"] = true
				gh["return_7d_pct"] = roundMarket2((okx06Data.Rollup.Stats.TotalPnL / okx06MarketDisplayInitial) * 100)
				gh["max_drawdown_pct"] = okx06Data.Rollup.Stats.MaxDrawdownPct
				gh["stats_window_days"] = okx06MarketStatsWindowDays
				gh["trend"] = sampleMarketTrend(okx06Data.Trend, 18)
				gh["total_aum"] = roundMarket2(okx06MarketDisplayInitial + okx06Data.Rollup.Stats.TotalPnL)
			}
			item["exchange_type"] = "OKX"
			okx06ExchangeLabel = "OKX"
		}
	}
	if marketDetailID == okx07MarketStrategyID {
		okx07Data := buildOkx07MarketData(true)
		if okx07Data != nil && okx07Data.Rollup != nil {
			okx07Applied = true
			initialCapitalSum = okx07MarketDisplayInitial
			rollup = okx07Data.Rollup
			tradeHistory = okx07Data.TradeHistory
			if gh, ok := item["stats"].(gin.H); ok {
				gh["data_complete"] = true
				gh["return_7d_pct"] = roundMarket2((okx07Data.Rollup.Stats.TotalPnL / okx07MarketDisplayInitial) * 100)
				gh["max_drawdown_pct"] = okx07Data.Rollup.Stats.MaxDrawdownPct
				gh["stats_window_days"] = okx07MarketStatsWindowDays
				gh["trend"] = sampleMarketTrend(okx07Data.Trend, 18)
				gh["total_aum"] = roundMarket2(okx07MarketDisplayInitial + okx07Data.Rollup.Stats.TotalPnL)
			}
			item["exchange_type"] = "OKX"
			okx07ExchangeLabel = "OKX"
		}
	}
	if marketDetailID == okx08MarketStrategyID {
		okx08Data := buildOkx08MarketData(true)
		if okx08Data != nil && okx08Data.Rollup != nil {
			okx08Applied = true
			initialCapitalSum = okx08MarketDisplayInitial
			rollup = okx08Data.Rollup
			tradeHistory = okx08Data.TradeHistory
			if gh, ok := item["stats"].(gin.H); ok {
				gh["data_complete"] = true
				gh["return_7d_pct"] = roundMarket2((okx08Data.Rollup.Stats.TotalPnL / okx08MarketDisplayInitial) * 100)
				gh["max_drawdown_pct"] = okx08Data.Rollup.Stats.MaxDrawdownPct
				gh["stats_window_days"] = okx08MarketStatsWindowDays
				gh["trend"] = sampleMarketTrend(okx08Data.Trend, 18)
				gh["total_aum"] = roundMarket2(okx08MarketDisplayInitial + okx08Data.Rollup.Stats.TotalPnL)
			}
			item["exchange_type"] = "OKX"
			okx08ExchangeLabel = "OKX"
		}
	}
	if !hzStarApplied && !ultimateSolApplied && !okx01Applied && !okx02Applied && !okx03Applied && !okx04Applied && !okx05Applied && !okx06Applied && !okx07Applied && !okx08Applied && applyMarketDetailDemoOverlay(marketDetailID, st.Name, item, &initialCapitalSum, &rollup, st.MarketRevision) {
		statsSource = "demo_overlay"
		demoMeta = gin.H{
			"enabled":           true,
			"env_key":           "NOFX_MARKET_DETAIL_DEMO",
			"name_match_substr": marketDetailDemoNameMatchSummary(),
			"note_zh":           "当前为演示叠加数据。取消或关闭 NOFX_MARKET_DETAIL_DEMO（设为 0）后恢复真实汇总。",
		}
	}

	exchangeLabel := ""
	if len(traderIDs) > 0 {
		var et string
		q := `SELECT e.exchange_type FROM traders t INNER JOIN exchanges e ON e.id = t.exchange_id WHERE t.id IN ? ORDER BY t.created_at ASC LIMIT 1`
		if err := s.store.GormDB().Raw(q, traderIDs).Scan(&et).Error; err == nil && strings.TrimSpace(et) != "" {
			exchangeLabel = strings.ToUpper(strings.TrimSpace(et))
		}
	}
	if exchangeLabel == "" && hzStarExchangeLabel != "" {
		exchangeLabel = hzStarExchangeLabel
	}
	if exchangeLabel == "" && ultimateSolExchangeLabel != "" {
		exchangeLabel = ultimateSolExchangeLabel
	}
	if exchangeLabel == "" && okx01ExchangeLabel != "" {
		exchangeLabel = okx01ExchangeLabel
	}
	if exchangeLabel == "" && okx02ExchangeLabel != "" {
		exchangeLabel = okx02ExchangeLabel
	}
	if exchangeLabel == "" && okx03ExchangeLabel != "" {
		exchangeLabel = okx03ExchangeLabel
	}
	if exchangeLabel == "" && okx04ExchangeLabel != "" {
		exchangeLabel = okx04ExchangeLabel
	}
	if exchangeLabel == "" && okx05ExchangeLabel != "" {
		exchangeLabel = okx05ExchangeLabel
	}
	if exchangeLabel == "" && okx06ExchangeLabel != "" {
		exchangeLabel = okx06ExchangeLabel
	}
	if exchangeLabel == "" && okx07ExchangeLabel != "" {
		exchangeLabel = okx07ExchangeLabel
	}
	if exchangeLabel == "" && okx08ExchangeLabel != "" {
		exchangeLabel = okx08ExchangeLabel
	}
	if exchangeLabel == "" {
		if _, ok := okxScreenMarketIdentities[marketDetailID]; ok {
			exchangeLabel = okxScreenMarketDisplayExchange(marketDetailID)
		}
	}
	applyStableMarketAudienceStats(item, marketDetailID, st.Name)
	if displayExchange := applyOkxScreenMarketDisplayExchangeOverlay(item, marketDetailID); displayExchange != "" {
		exchangeLabel = displayExchange
	}

	c.JSON(http.StatusOK, gin.H{
		"strategy":          item,
		"initial_capital":   initialCapitalSum,
		"aggregate_trading": rollup,
		"exchange_label":    exchangeLabel,
		"trade_history":     tradeHistory,
		"stats_source":      statsSource,
		"demo_overlay":      demoMeta,
	})
}

// handleGetStrategies Get strategy list
func (s *Server) handleGetStrategies(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	strategies, err := s.store.Strategy().List(userID)
	if err != nil {
		SafeInternalError(c, "Failed to get strategy list", err)
		return
	}

	// Convert to frontend format
	result := make([]gin.H, 0, len(strategies))
	for _, st := range strategies {
		var config store.StrategyConfig
		json.Unmarshal([]byte(st.Config), &config)

		result = append(result, gin.H{
			"id":                   st.ID,
			"name":                 st.Name,
			"description":          st.Description,
			"is_active":            st.IsActive,
			"is_default":           st.IsDefault,
			"market_access":        store.EffectiveMarketAccess(st),
			"show_after_rename":    st.ShowAfterRename,
			"is_public":            st.IsPublic,
			"config_visible":       st.ConfigVisible,
			"market_revision":      st.MarketRevision,
			"source_strategy_id":   st.SourceStrategyID,
			"source_market_access": st.SourceMarketAccess,
			"content_locked":       st.ContentLocked,
			"config":               config,
			"created_at":           st.CreatedAt,
			"updated_at":           st.UpdatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"strategies": result,
	})
}

// handleGetStrategy Get single strategy
func (s *Server) handleGetStrategy(c *gin.Context) {
	userID := c.GetString("user_id")
	strategyID := c.Param("id")

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	strategy, err := s.store.Strategy().Get(userID, strategyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Strategy not found"})
		return
	}

	var config store.StrategyConfig
	json.Unmarshal([]byte(strategy.Config), &config)

	c.JSON(http.StatusOK, gin.H{
		"id":                   strategy.ID,
		"name":                 strategy.Name,
		"description":          strategy.Description,
		"is_active":            strategy.IsActive,
		"is_default":           strategy.IsDefault,
		"market_access":        store.EffectiveMarketAccess(strategy),
		"show_after_rename":    strategy.ShowAfterRename,
		"is_public":            strategy.IsPublic,
		"config_visible":       strategy.ConfigVisible,
		"market_revision":      strategy.MarketRevision,
		"source_strategy_id":   strategy.SourceStrategyID,
		"source_market_access": strategy.SourceMarketAccess,
		"content_locked":       strategy.ContentLocked,
		"config":               config,
		"created_at":           strategy.CreatedAt,
		"updated_at":           strategy.UpdatedAt,
	})
}

// handleCreateStrategy Create strategy.
// If "config" is omitted from the request body, the system default config is used automatically.
func (s *Server) handleCreateStrategy(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Name        string                `json:"name" binding:"required"`
		Description string                `json:"description"`
		Lang        string                `json:"lang"`   // "zh" or "en", used when config is omitted
		Config      *store.StrategyConfig `json:"config"` // optional — uses default if omitted
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	// Use default config when none provided
	if req.Config == nil {
		lang := req.Lang
		if lang == "" {
			lang = "zh"
		}
		defaultCfg := store.GetDefaultStrategyConfig(lang)
		req.Config = &defaultCfg
	}

	// Serialize configuration
	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		SafeInternalError(c, "Serialize configuration", err)
		return
	}

	strategy := &store.Strategy{
		ID:              uuid.New().String(),
		UserID:          userID,
		Name:            req.Name,
		Description:     req.Description,
		IsActive:        false,
		IsDefault:       false,
		Config:          string(configJSON),
		MarketAccess:    store.MarketAccessOff,
		ShowAfterRename: false,
		IsPublic:        false,
		ConfigVisible:   true,
	}

	if err := s.store.Strategy().Create(strategy); err != nil {
		SafeInternalError(c, "Failed to create strategy", err)
		return
	}

	// Validate configuration and collect warnings
	warnings := validateStrategyConfig(req.Config)

	response := gin.H{
		"id":      strategy.ID,
		"message": "Strategy created successfully",
	}
	if len(warnings) > 0 {
		response["warnings"] = warnings
	}

	c.JSON(http.StatusOK, response)
}

type strategyPutRequest struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Config        json.RawMessage `json:"config"`
	MarketAccess  string          `json:"market_access"` // off | private | subscription | public | open_source
	IsPublic      bool            `json:"is_public"`     // 兼容旧前端
	ConfigVisible bool            `json:"config_visible"`
	MarketAIModel string          `json:"market_ai_model"` // 策略市场展示用 AI 模型标识（可选）
}

func resolveMarketAccessFromRequest(req *strategyPutRequest) (string, error) {
	raw := strings.TrimSpace(strings.ToLower(req.MarketAccess))
	if raw != "" {
		if !store.ValidMarketAccess(raw) {
			return "", fmt.Errorf("invalid market_access")
		}
		return raw, nil
	}
	if !req.IsPublic {
		return store.MarketAccessOff, nil
	}
	if req.ConfigVisible {
		return store.MarketAccessPublic, nil
	}
	return store.MarketAccessSubscription, nil
}

const (
	marketReviewStatusPending  = "pending"
	marketReviewStatusApproved = "approved"
	marketReviewStatusRejected = "rejected"
	marketReviewMarkerPrefix   = "market_review:"
)

func normalizeMarketReviewStatus(v string) string {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case marketReviewStatusPending:
		return marketReviewStatusPending
	case marketReviewStatusApproved:
		return marketReviewStatusApproved
	case marketReviewStatusRejected:
		return marketReviewStatusRejected
	default:
		return ""
	}
}

func (s *Server) isStrategyMarketAdminUser(userID string) bool {
	u, err := s.store.User().GetByID(userID)
	return err == nil && u != nil && isAdminEmail(u.Email)
}

func (s *Server) applyMarketReviewGate(userID string, existing *store.Strategy, cfg *store.StrategyConfig, requestedAccess string) (actualAccess string, notifyAdmin bool) {
	requestedAccess = strings.TrimSpace(strings.ToLower(requestedAccess))
	if requestedAccess == "" {
		requestedAccess = store.MarketAccessOff
	}
	if !store.IsListedOnMarket(requestedAccess) {
		cfg.MarketReviewStatus = ""
		cfg.MarketReviewRequestedAccess = ""
		cfg.MarketReviewRequestedAt = ""
		cfg.MarketReviewReviewedAt = ""
		cfg.MarketReviewReviewedBy = ""
		return store.MarketAccessOff, false
	}

	if s.isStrategyMarketAdminUser(userID) {
		cfg.MarketReviewStatus = marketReviewStatusApproved
		cfg.MarketReviewRequestedAccess = requestedAccess
		return requestedAccess, false
	}

	var oldCfg store.StrategyConfig
	if existing != nil && strings.TrimSpace(existing.Config) != "" {
		_ = json.Unmarshal([]byte(existing.Config), &oldCfg)
	}
	oldStatus := normalizeMarketReviewStatus(oldCfg.MarketReviewStatus)
	oldRequested := strings.TrimSpace(strings.ToLower(oldCfg.MarketReviewRequestedAccess))
	if existing != nil && store.IsListedOnMarket(store.EffectiveMarketAccess(existing)) &&
		oldStatus == marketReviewStatusApproved && oldRequested == requestedAccess {
		cfg.MarketReviewStatus = marketReviewStatusApproved
		cfg.MarketReviewRequestedAccess = requestedAccess
		return requestedAccess, false
	}

	cfg.MarketReviewStatus = marketReviewStatusPending
	cfg.MarketReviewRequestedAccess = requestedAccess
	cfg.MarketReviewRequestedAt = time.Now().UTC().Format(time.RFC3339)
	cfg.MarketReviewReviewedAt = ""
	cfg.MarketReviewReviewedBy = ""
	return store.MarketAccessOff, oldStatus != marketReviewStatusPending || oldRequested != requestedAccess
}

func (s *Server) notifyAdminsMarketReviewRequest(st *store.Strategy, owner *store.User, requestedAccess string) {
	if st == nil {
		return
	}
	ownerName := ""
	ownerEmail := ""
	if owner != nil {
		ownerName = strings.TrimSpace(owner.DisplayName)
		ownerEmail = strings.TrimSpace(owner.Email)
	}
	if ownerName == "" {
		ownerName = ownerEmail
	}
	if ownerName == "" {
		ownerName = st.UserID
	}
	title := "策略市场上架审核"
	body := fmt.Sprintf(
		"用户：%s\n邮箱：%s\n策略：%s\n策略ID：%s\n申请展示方式：%s\n\n请在本通知里选择同意显示或不同意。\n审核标记：%s%s",
		ownerName,
		ownerEmail,
		st.Name,
		st.ID,
		requestedAccess,
		marketReviewMarkerPrefix,
		st.ID,
	)
	for _, email := range adminEmailList() {
		u, err := s.store.User().GetByEmail(email)
		if err != nil || u == nil {
			continue
		}
		_ = s.store.Notification().Add(u.ID, title, body)
	}
}

// mergeStrategyPut merges req.Config onto existing DB config（与 PUT 相同规则）。
func mergeStrategyPut(existing *store.Strategy, req *strategyPutRequest) (mergedConfig store.StrategyConfig, configJSON string, name, description string, err error) {
	if uerr := json.Unmarshal([]byte(existing.Config), &mergedConfig); uerr != nil {
		mergedConfig = store.StrategyConfig{}
	}
	if len(req.Config) > 0 && string(req.Config) != "null" {
		if err = json.Unmarshal(req.Config, &mergedConfig); err != nil {
			return mergedConfig, "", "", "", err
		}
	}
	name = req.Name
	if name == "" {
		name = existing.Name
	}
	description = req.Description
	if description == "" {
		description = existing.Description
	}
	var raw []byte
	raw, err = json.Marshal(mergedConfig)
	if err != nil {
		return mergedConfig, "", "", "", err
	}
	return mergedConfig, string(raw), name, description, nil
}

// finalizeStrategySaveResponse 写入成功后做 token 校验并返回 JSON；失败时已写入 DB（与旧行为一致）。
func (s *Server) finalizeStrategySaveResponse(c *gin.Context, mergedConfig *store.StrategyConfig, message string) {
	if mergedConfig.StrategyType == "" || mergedConfig.StrategyType == "ai_trading" {
		estimate := mergedConfig.EstimateTokens()
		allExceed := true
		for _, ml := range estimate.ModelLimits {
			if ml.UsagePct <= 100 {
				allExceed = false
				break
			}
		}
		if allExceed && len(estimate.ModelLimits) > 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":          fmt.Sprintf("Estimated %d tokens exceeds all known model context limits. Reduce coins, timeframes, or K-line count.", estimate.Total),
				"token_estimate": estimate,
			})
			return
		}
	}
	warnings := validateStrategyConfig(mergedConfig)
	if strings.TrimSpace(message) == "" {
		message = "Strategy updated successfully"
	}
	response := gin.H{"message": message}
	if len(warnings) > 0 {
		response["warnings"] = warnings
	}
	c.JSON(http.StatusOK, response)
}

// handleUpdateStrategy Update strategy.
// The incoming config is merged with the existing one: top-level sections present in the
// request overwrite the corresponding existing sections; absent sections are preserved.
// This prevents partial updates from zeroing out unmentioned fields.
func (s *Server) handleUpdateStrategy(c *gin.Context) {
	userID := c.GetString("user_id")
	strategyID := c.Param("id")

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	existing, err := s.store.Strategy().Get(userID, strategyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Strategy not found"})
		return
	}
	if existing.IsDefault {
		c.JSON(http.StatusForbidden, gin.H{"error": "Cannot modify system default strategy"})
		return
	}
	if existing.ContentLocked {
		c.JSON(http.StatusForbidden, gin.H{"error": "该作者未公开策略内容，只可使用，不能修改内容"})
		return
	}

	var req strategyPutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	mergedConfig, _, name, description, mergeErr := mergeStrategyPut(existing, &req)
	if mergeErr != nil {
		SafeBadRequest(c, "Invalid config JSON")
		return
	}

	access, accErr := resolveMarketAccessFromRequest(&req)
	if accErr != nil {
		SafeBadRequest(c, accErr.Error())
		return
	}
	// 普通订阅必须标价 >0；COMKUN 跟单/官方跟单模板允许 0 元订阅，后续按主控广播次数扣平台余额。
	if access == store.MarketAccessSubscription && mergedConfig.MarketSalePriceUSDT <= 0 &&
		!mergedConfig.ComkunFollowListingTemplate && !store.IsComkunMarketFollowStrategy(&mergedConfig) {
		SafeBadRequest(c, "Subscription listing requires market_sale_price_usdt > 0 in config")
		return
	}
	actualAccess, notifyAdmin := s.applyMarketReviewGate(userID, existing, &mergedConfig, access)
	configBytes, err := json.Marshal(mergedConfig)
	if err != nil {
		SafeInternalError(c, "Serialize configuration", err)
		return
	}
	configJSON := string(configBytes)

	nameChanged := strings.TrimSpace(name) != strings.TrimSpace(existing.Name)
	showAfterRename := existing.ShowAfterRename || nameChanged
	if actualAccess == store.MarketAccessOff {
		showAfterRename = false
	}

	strategy := &store.Strategy{
		ID:                 strategyID,
		UserID:             userID,
		Name:               name,
		Description:        description,
		Config:             configJSON,
		MarketAccess:       actualAccess,
		ShowAfterRename:    showAfterRename,
		SourceStrategyID:   existing.SourceStrategyID,
		SourceMarketAccess: existing.SourceMarketAccess,
		ContentLocked:      existing.ContentLocked,
	}
	if v := strings.TrimSpace(req.MarketAIModel); v != "" {
		strategy.MarketAIModel = v
	}
	store.SyncListingFlagsFromAccess(strategy, actualAccess)

	if err := s.store.Strategy().Update(strategy); err != nil {
		SafeInternalError(c, "Failed to update strategy", err)
		return
	}

	if notifyAdmin {
		if owner, ownerErr := s.store.User().GetByID(userID); ownerErr == nil {
			s.notifyAdminsMarketReviewRequest(strategy, owner, access)
		} else {
			s.notifyAdminsMarketReviewRequest(strategy, nil, access)
		}
	}
	msg := "Strategy updated successfully"
	if normalizeMarketReviewStatus(mergedConfig.MarketReviewStatus) == marketReviewStatusPending {
		msg = "策略已提交管理员审核，通过后才会显示在策略市场"
	}
	s.finalizeStrategySaveResponse(c, &mergedConfig, msg)
}

// handlePublishMarketUpdate 已上架（is_public）策略：保存当前内容并递增 market_revision，便于市场订阅方检测更新。
func (s *Server) handlePublishMarketUpdate(c *gin.Context) {
	userID := c.GetString("user_id")
	strategyID := c.Param("id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	existing, err := s.store.Strategy().Get(userID, strategyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Strategy not found"})
		return
	}
	if existing.IsDefault {
		c.JSON(http.StatusForbidden, gin.H{"error": "Cannot modify system default strategy"})
		return
	}
	if existing.ContentLocked {
		c.JSON(http.StatusForbidden, gin.H{"error": "该作者未公开策略内容，只可使用，不能修改内容"})
		return
	}
	if !store.IsVisibleOnPublicMarket(existing) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Strategy is not listed on the market. Enable a market listing mode first, then save."})
		return
	}

	var req strategyPutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	mergedConfig, configJSON, name, description, mergeErr := mergeStrategyPut(existing, &req)
	if mergeErr != nil {
		SafeBadRequest(c, "Invalid config JSON")
		return
	}

	access := store.EffectivePublicListingAccess(existing)
	if strings.TrimSpace(req.MarketAccess) != "" {
		var accErr error
		access, accErr = resolveMarketAccessFromRequest(&req)
		if accErr != nil {
			SafeBadRequest(c, accErr.Error())
			return
		}
	}
	if !store.IsListedOnMarket(access) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Market update requires an active listing; use normal save to delist."})
		return
	}
	if access == store.MarketAccessSubscription && mergedConfig.MarketSalePriceUSDT <= 0 &&
		!mergedConfig.ComkunFollowListingTemplate && !store.IsComkunMarketFollowStrategy(&mergedConfig) {
		SafeBadRequest(c, "Subscription listing requires market_sale_price_usdt > 0 in config")
		return
	}

	if mergedConfig.StrategyType == "" || mergedConfig.StrategyType == "ai_trading" {
		estimate := mergedConfig.EstimateTokens()
		allExceed := true
		for _, ml := range estimate.ModelLimits {
			if ml.UsagePct <= 100 {
				allExceed = false
				break
			}
		}
		if allExceed && len(estimate.ModelLimits) > 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":          fmt.Sprintf("Estimated %d tokens exceeds all known model context limits. Reduce coins, timeframes, or K-line count.", estimate.Total),
				"token_estimate": estimate,
			})
			return
		}
	}

	// 市场更新必须保持上架；下架请用普通「保存」
	strategy := &store.Strategy{
		ID:                 strategyID,
		UserID:             userID,
		Name:               name,
		Description:        description,
		Config:             configJSON,
		MarketAccess:       access,
		ShowAfterRename:    existing.ShowAfterRename,
		SourceStrategyID:   existing.SourceStrategyID,
		SourceMarketAccess: existing.SourceMarketAccess,
		ContentLocked:      existing.ContentLocked,
	}
	store.SyncListingFlagsFromAccess(strategy, access)

	if err := s.store.Strategy().UpdateAndIncrementMarketRevision(strategy); err != nil {
		SafeInternalError(c, "Failed to publish market update", err)
		return
	}

	updated, err := s.store.Strategy().Get(userID, strategyID)
	if err != nil {
		SafeInternalError(c, "Failed to read strategy after publish", err)
		return
	}
	warnings := validateStrategyConfig(&mergedConfig)
	resp := gin.H{
		"message":         "Market strategy updated; subscribers can compare market_revision",
		"market_revision": updated.MarketRevision,
		"updated_at":      updated.UpdatedAt,
	}
	if len(warnings) > 0 {
		resp["warnings"] = warnings
	}
	c.JSON(http.StatusOK, resp)
}

// handleDeleteStrategy Delete strategy
func (s *Server) handleDeleteStrategy(c *gin.Context) {
	userID := c.GetString("user_id")
	strategyID := c.Param("id")

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if err := s.store.Strategy().Delete(userID, strategyID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": SanitizeError(err, "Failed to delete strategy")})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Strategy deleted successfully"})
}

// handleActivateStrategy Activate strategy
func (s *Server) handleActivateStrategy(c *gin.Context) {
	userID := c.GetString("user_id")
	strategyID := c.Param("id")

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if err := s.store.Strategy().SetActive(userID, strategyID); err != nil {
		SafeInternalError(c, "Failed to activate strategy", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Strategy activated successfully"})
}

// handleDuplicateStrategy Duplicate strategy
func (s *Server) handleDuplicateStrategy(c *gin.Context) {
	userID := c.GetString("user_id")
	sourceID := c.Param("id")

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	newID := uuid.New().String()
	if err := s.store.DuplicateStrategy(userID, sourceID, newID, req.Name); err != nil {
		if strings.Contains(err.Error(), "无权复制") {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "策略不存在"})
			return
		}
		SafeInternalError(c, "Failed to duplicate strategy", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":      newID,
		"message": "Strategy duplicated successfully",
	})
}

// handleGetActiveStrategy Get currently active strategy
func (s *Server) handleGetActiveStrategy(c *gin.Context) {
	userID := c.GetString("user_id")

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	strategy, err := s.store.Strategy().GetActive(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No active strategy"})
		return
	}

	var config store.StrategyConfig
	json.Unmarshal([]byte(strategy.Config), &config)

	c.JSON(http.StatusOK, gin.H{
		"id":          strategy.ID,
		"name":        strategy.Name,
		"description": strategy.Description,
		"is_active":   strategy.IsActive,
		"is_default":  strategy.IsDefault,
		"config":      config,
		"created_at":  strategy.CreatedAt,
		"updated_at":  strategy.UpdatedAt,
	})
}

// handleGetDefaultStrategyConfig Get default strategy configuration template
func (s *Server) handleGetDefaultStrategyConfig(c *gin.Context) {
	// Get language from query parameter, default to "en"
	lang := c.Query("lang")
	if lang != "zh" {
		lang = "en"
	}

	// Return default configuration with i18n support
	defaultConfig := store.GetDefaultStrategyConfig(lang)
	c.JSON(http.StatusOK, defaultConfig)
}

// handlePreviewPrompt Preview prompt generated by strategy
func (s *Server) handlePreviewPrompt(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Config        store.StrategyConfig `json:"config" binding:"required"`
		AccountEquity float64              `json:"account_equity"`
		PromptVariant string               `json:"prompt_variant"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	// Use default values
	if req.AccountEquity <= 0 {
		req.AccountEquity = 1000.0 // Default simulated account equity
	}
	if req.PromptVariant == "" {
		req.PromptVariant = "balanced"
	}

	// Create strategy engine to build prompt
	engine := kernel.NewStrategyEngine(&req.Config)

	// Build system prompt (using built-in method from strategy engine)
	systemPrompt := engine.BuildSystemPrompt(
		req.AccountEquity,
		req.PromptVariant,
	)

	c.JSON(http.StatusOK, gin.H{
		"system_prompt":  systemPrompt,
		"prompt_variant": req.PromptVariant,
		"config_summary": gin.H{
			"coin_source":      req.Config.CoinSource.SourceType,
			"primary_tf":       req.Config.Indicators.Klines.PrimaryTimeframe,
			"btc_eth_leverage": req.Config.RiskControl.BTCETHMaxLeverage,
			"altcoin_leverage": req.Config.RiskControl.AltcoinMaxLeverage,
			"max_positions":    req.Config.RiskControl.MaxPositions,
		},
	})
}

// handleStrategyTestRun AI test run (does not execute trades, only returns AI analysis results)
func (s *Server) handleStrategyTestRun(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Config        store.StrategyConfig `json:"config" binding:"required"`
		PromptVariant string               `json:"prompt_variant"`
		AIModelID     string               `json:"ai_model_id"`
		RunRealAI     bool                 `json:"run_real_ai"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	if req.PromptVariant == "" {
		req.PromptVariant = "balanced"
	}

	// Create strategy engine to build prompt
	engine := kernel.NewStrategyEngine(&req.Config)

	// Get candidate coins
	candidates, err := engine.GetCandidateCoins()
	if err != nil {
		logger.Errorf("[API Error] Failed to get candidate coins: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":       "Failed to get candidate coins",
			"ai_response": "",
		})
		return
	}

	// Get timeframe configuration
	timeframes := req.Config.Indicators.Klines.SelectedTimeframes
	primaryTimeframe := req.Config.Indicators.Klines.PrimaryTimeframe
	klineCount := req.Config.Indicators.Klines.PrimaryCount

	// If no timeframes selected, use default values
	if len(timeframes) == 0 {
		// Backward compatibility: use primary and longer timeframes
		if primaryTimeframe != "" {
			timeframes = append(timeframes, primaryTimeframe)
		} else {
			timeframes = append(timeframes, "3m")
		}
		if req.Config.Indicators.Klines.LongerTimeframe != "" {
			timeframes = append(timeframes, req.Config.Indicators.Klines.LongerTimeframe)
		}
	}
	if primaryTimeframe == "" {
		primaryTimeframe = timeframes[0]
	}
	if klineCount <= 0 {
		klineCount = 30
	}

	fmt.Printf("📊 Using timeframes: %v, primary: %s, kline count: %d\n", timeframes, primaryTimeframe, klineCount)

	// Get real market data (using multiple timeframes)
	marketDataMap := make(map[string]*market.Data)
	for _, coin := range candidates {
		data, err := market.GetWithTimeframes(coin.Symbol, timeframes, primaryTimeframe, klineCount)
		if err != nil {
			// If getting data for a coin fails, log but continue
			fmt.Printf("⚠️  Failed to get market data for %s: %v\n", coin.Symbol, err)
			continue
		}
		marketDataMap[coin.Symbol] = data
	}

	// Fetch quantitative data for each candidate coin
	symbols := make([]string, 0, len(candidates))
	for _, c := range candidates {
		symbols = append(symbols, c.Symbol)
	}
	quantDataMap := engine.FetchQuantDataBatch(symbols)

	// Fetch OI ranking data (market-wide position changes)
	oiRankingData := engine.FetchOIRankingData()

	// Fetch NetFlow ranking data (market-wide fund flow)
	netFlowRankingData := engine.FetchNetFlowRankingData()

	// Fetch Price ranking data (market-wide gainers/losers)
	priceRankingData := engine.FetchPriceRankingData()

	// Build real context (for generating User Prompt)
	testContext := &kernel.Context{
		CurrentTime:    time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
		RuntimeMinutes: 0,
		CallCount:      1,
		Account: kernel.AccountInfo{
			TotalEquity:      1000.0,
			AvailableBalance: 1000.0,
			UnrealizedPnL:    0,
			TotalPnL:         0,
			TotalPnLPct:      0,
			MarginUsed:       0,
			MarginUsedPct:    0,
			PositionCount:    0,
		},
		Positions:          []kernel.PositionInfo{},
		CandidateCoins:     candidates,
		PromptVariant:      req.PromptVariant,
		MarketDataMap:      marketDataMap,
		QuantDataMap:       quantDataMap,
		OIRankingData:      oiRankingData,
		NetFlowRankingData: netFlowRankingData,
		PriceRankingData:   priceRankingData,
	}

	// Build System Prompt
	systemPrompt := engine.BuildSystemPrompt(1000.0, req.PromptVariant)

	// Build User Prompt (using real market data)
	userPrompt := engine.BuildUserPrompt(testContext)

	// If requesting real AI call
	if req.RunRealAI && req.AIModelID != "" {
		aiResponse, aiErr := s.runRealAITest(userID, req.AIModelID, systemPrompt, userPrompt)
		if aiErr != nil {
			c.JSON(http.StatusOK, gin.H{
				"system_prompt":   systemPrompt,
				"user_prompt":     userPrompt,
				"candidate_count": len(candidates),
				"candidates":      candidates,
				"prompt_variant":  req.PromptVariant,
				"ai_response":     fmt.Sprintf("❌ AI call failed: %s", aiErr.Error()),
				"ai_error":        aiErr.Error(),
				"note":            "AI call error",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"system_prompt":   systemPrompt,
			"user_prompt":     userPrompt,
			"candidate_count": len(candidates),
			"candidates":      candidates,
			"prompt_variant":  req.PromptVariant,
			"ai_response":     aiResponse,
			"note":            "✅ Real AI test run successful",
		})
		return
	}

	// Return result (without actually calling AI, only return built prompt)
	c.JSON(http.StatusOK, gin.H{
		"system_prompt":   systemPrompt,
		"user_prompt":     userPrompt,
		"candidate_count": len(candidates),
		"candidates":      candidates,
		"prompt_variant":  req.PromptVariant,
		"ai_response":     "Please select an AI model and click 'Run Test' to perform real AI analysis.",
		"note":            "AI model not selected or real AI call not enabled",
	})
}

// runRealAITest Execute real AI test call
func (s *Server) runRealAITest(userID, modelID, systemPrompt, userPrompt string) (string, error) {
	// Get AI model configuration
	model, err := s.store.AIModel().Get(userID, modelID)
	if err != nil {
		return "", fmt.Errorf("failed to get AI model: %w", err)
	}

	if !model.Enabled {
		return "", fmt.Errorf("AI model %s is not enabled", model.Name)
	}

	if !isPlatformBilledAIProvider(model.Provider) && model.APIKey == "" {
		return "", fmt.Errorf("AI model %s is missing API Key", model.Name)
	}

	// Create AI client via registry
	provider := model.Provider
	apiKey := string(model.APIKey)

	aiClient := mcp.NewAIClientByProvider(provider)
	if aiClient == nil {
		aiClient = mcp.NewClient()
	}

	// Payment providers ignore custom URL
	switch provider {
	case "claw402", "comkun_proxy", "comkun_ai":
		if provider == "comkun_proxy" || provider == "comkun_ai" {
			apiKey = ""
		}
		aiClient.SetAPIKey(apiKey, "", model.CustomModelName)
	default:
		aiClient.SetAPIKey(apiKey, model.CustomAPIURL, model.CustomModelName)
	}

	// Call AI API
	response, err := aiClient.CallWithMessages(systemPrompt, userPrompt)
	if err != nil {
		return "", fmt.Errorf("AI API call failed: %w", err)
	}

	return response, nil
}
