package api

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"nofx/logger"
	"nofx/market"
	nofxstore "nofx/store"
	"nofx/trader/gate"
	tradertypes "nofx/trader/types"

	"github.com/gin-gonic/gin"
)

// handleTraderList Trader list
func (s *Server) handleTraderList(c *gin.Context) {
	userID := c.GetString("user_id")
	traders, err := s.store.Trader().List(userID)
	if err != nil {
		SafeInternalError(c, "Failed to get trader list", err)
		return
	}

	result := make([]map[string]interface{}, 0, len(traders))
	for _, trader := range traders {
		// Get real-time running status
		isRunning := trader.IsRunning
		if at, err := s.traderManager.GetTrader(trader.ID); err == nil {
			status := at.GetStatus()
			if running, ok := status["is_running"].(bool); ok {
				isRunning = running
			}
		}

		// Get strategy name if strategy_id is set
		var strategyName string
		if trader.StrategyID != "" {
			if strategy, err := s.store.Strategy().Get(userID, trader.StrategyID); err == nil {
				strategyName = strategy.Name
			}
		}

		// Return complete AIModelID (e.g. "admin_deepseek"), don't truncate
		// Frontend needs complete ID to verify model exists (consistent with handleGetTraderConfig)
		result = append(result, map[string]interface{}{
			"trader_id":           trader.ID,
			"trader_name":         trader.Name,
			"ai_model":            trader.AIModelID, // Use complete ID
			"exchange_id":         trader.ExchangeID,
			"is_running":          isRunning,
			"show_in_competition": trader.ShowInCompetition,
			"initial_balance":     trader.InitialBalance,
			"strategy_id":         trader.StrategyID,
			"strategy_name":       strategyName,
		})
	}

	c.JSON(http.StatusOK, result)
}

// handleGetTraderConfig Get trader detailed configuration
func (s *Server) handleGetTraderConfig(c *gin.Context) {
	userID := c.GetString("user_id")
	traderID := c.Param("id")

	if traderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Trader ID cannot be empty"})
		return
	}

	fullCfg, err := s.store.Trader().GetFullConfig(userID, traderID)
	if err != nil {
		SafeNotFound(c, "Trader config")
		return
	}
	traderConfig := fullCfg.Trader

	// Get real-time running status
	isRunning := traderConfig.IsRunning
	if at, err := s.traderManager.GetTrader(traderID); err == nil {
		status := at.GetStatus()
		if running, ok := status["is_running"].(bool); ok {
			isRunning = running
		}
	}

	// Return complete model ID without conversion, consistent with frontend model list
	aiModelID := traderConfig.AIModelID

	result := map[string]interface{}{
		"trader_id":             traderConfig.ID,
		"trader_name":           traderConfig.Name,
		"ai_model":              aiModelID,
		"exchange_id":           traderConfig.ExchangeID,
		"strategy_id":           traderConfig.StrategyID,
		"initial_balance":       traderConfig.InitialBalance,
		"scan_interval_minutes": traderConfig.ScanIntervalMinutes,
		"btc_eth_leverage":      traderConfig.BTCETHLeverage,
		"altcoin_leverage":      traderConfig.AltcoinLeverage,
		"trading_symbols":       traderConfig.TradingSymbols,
		"custom_prompt":         traderConfig.CustomPrompt,
		"override_base_prompt":  traderConfig.OverrideBasePrompt,
		"is_cross_margin":       traderConfig.IsCrossMargin,
		"use_ai500":             traderConfig.UseAI500,
		"use_oi_top":            traderConfig.UseOITop,
		"is_running":            isRunning,
	}

	c.JSON(http.StatusOK, result)
}

// handleStatus System status
func (s *Server) handleStatus(c *gin.Context) {
	_, traderID, err := s.getTraderFromQuery(c)
	if err != nil {
		SafeBadRequest(c, "Invalid trader ID")
		return
	}

	trader, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		SafeNotFound(c, "Trader")
		return
	}

	status := trader.GetStatus()
	c.JSON(http.StatusOK, status)
}

// handleAccount Account information
func (s *Server) handleAccount(c *gin.Context) {
	_, traderID, err := s.getTraderFromQuery(c)
	if err != nil {
		SafeBadRequest(c, "Invalid trader ID")
		return
	}

	trader, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		SafeNotFound(c, "Trader")
		return
	}

	logger.Infof("📊 Received account info request [%s]", trader.GetName())
	account, err := trader.GetAccountInfo()
	if err != nil {
		SafeInternalError(c, "Get account info", err)
		return
	}

	logger.Infof("✓ Returning account info [%s]: equity=%.2f, available=%.2f, pnl=%.2f (%.2f%%)",
		trader.GetName(),
		account["total_equity"],
		account["available_balance"],
		account["total_pnl"],
		account["total_pnl_pct"])
	if isTraderDashboardDemoTrader(traderID) {
		applySilentTestTraderAccountDemo(account)
	}
	c.JSON(http.StatusOK, account)
}

// handlePositions Position list
func (s *Server) handlePositions(c *gin.Context) {
	_, traderID, err := s.getTraderFromQuery(c)
	if err != nil {
		SafeBadRequest(c, "Invalid trader ID")
		return
	}

	trader, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		SafeNotFound(c, "Trader")
		return
	}

	positions, err := trader.GetPositions()
	if err != nil {
		SafeInternalError(c, "Get positions", err)
		return
	}

	c.JSON(http.StatusOK, positions)
}

// handlePositionHistory Historical closed positions with statistics
func (s *Server) handlePositionHistory(c *gin.Context) {
	_, traderID, err := s.getTraderFromQuery(c)
	if err != nil {
		SafeBadRequest(c, "Invalid trader ID")
		return
	}
	userID := c.GetString("user_id")

	trader, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		SafeNotFound(c, "Trader")
		return
	}

	// Get optional query parameters
	limitStr := c.DefaultQuery("limit", "100")
	limit := 100
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 500 {
		limit = l
	}

	if isTraderDashboardDemoTrader(traderID) {
		c.JSON(http.StatusOK, buildSilentTestPositionHistoryPayload(traderID, limit))
		return
	}

	// Get store
	store := trader.GetStore()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Store not available"})
		return
	}

	fullCfg, cfgErr := s.store.Trader().GetFullConfig(userID, traderID)
	if cfgErr == nil && fullCfg.Strategy != nil {
		if strategyCfg, parseErr := fullCfg.Strategy.ParseConfig(); parseErr == nil && nofxstore.IsMT4GoldMasterStrategyID(nofxstore.ResolveComkunFollowSourceStrategyID(strategyCfg)) {
			records, historyErr := buildMT4FollowHistoryRecords(s.store.ComkunFollow(), traderID, limit)
			if historyErr == nil {
				c.JSON(http.StatusOK, buildLivePositionHistoryPayload(
					traderID,
					fullCfg.Exchange.ID,
					fullCfg.Exchange.ExchangeType,
					records,
					limit,
				))
				return
			}
			logger.Warnf("MT4 ticket history failed [%s]: %v", trader.GetName(), historyErr)
		}
	}
	if cfgErr == nil && fullCfg.Exchange != nil && strings.EqualFold(fullCfg.Exchange.ExchangeType, "gate") {
		startTime := time.Now().UTC().Add(-30 * 24 * time.Hour)
		records, liveErr := trader.GetUnderlyingTrader().GetClosedPnL(startTime, limit)
		if liveErr == nil {
			if gateTrader, ok := trader.GetUnderlyingTrader().(*gate.GateTrader); ok {
				if trades, tradeErr := gateTrader.GetTrades(startTime, limit); tradeErr == nil {
					records = append(records, buildGateCloseTradeHistoryRecords(
						trades,
						records,
						store,
						trader.GetID(),
					)...)
					records = filterGateAggregateClosedPnL(records)
				} else {
					logger.Warnf("Gate close trade history failed [%s]: %v", trader.GetName(), tradeErr)
				}
			}
			c.JSON(http.StatusOK, buildLivePositionHistoryPayload(
				trader.GetID(),
				fullCfg.Exchange.ID,
				fullCfg.Exchange.ExchangeType,
				records,
				limit,
			))
			return
		}
		logger.Warnf("Gate live position history failed [%s]: %v", trader.GetName(), liveErr)
	}

	// Get closed positions
	positions, err := store.Position().GetClosedPositions(trader.GetID(), limit)
	if err != nil {
		SafeInternalError(c, "Get position history", err)
		return
	}

	// Get statistics
	stats, _ := store.Position().GetFullStats(trader.GetID())

	// Get symbol stats
	symbolStats, _ := store.Position().GetSymbolStats(trader.GetID(), 10)

	// Get direction stats
	directionStats, _ := store.Position().GetDirectionStats(trader.GetID())

	c.JSON(http.StatusOK, gin.H{
		"positions":       positions,
		"stats":           stats,
		"symbol_stats":    symbolStats,
		"direction_stats": directionStats,
	})
}

func buildLivePositionHistoryPayload(traderID, exchangeID, exchangeType string, records []tradertypes.ClosedPnLRecord, limit int) gin.H {
	sort.Slice(records, func(i, j int) bool {
		return records[i].ExitTime.After(records[j].ExitTime)
	})
	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}

	positions := make([]gin.H, 0, len(records))
	totalPnL := 0.0
	totalFee := 0.0
	winTrades := 0
	lossTrades := 0
	totalWin := 0.0
	totalLoss := 0.0

	for i, record := range records {
		side := strings.ToUpper(strings.TrimSpace(record.Side))
		if side == "BUY" {
			side = "LONG"
		} else if side == "SELL" {
			side = "SHORT"
		}

		totalPnL += record.RealizedPnL
		totalFee += record.Fee
		if record.RealizedPnL > 0 {
			winTrades++
			totalWin += record.RealizedPnL
		} else if record.RealizedPnL < 0 {
			lossTrades++
			totalLoss += -record.RealizedPnL
		}

		positions = append(positions, gin.H{
			"id":                   i + 1,
			"trader_id":            traderID,
			"exchange_id":          exchangeID,
			"exchange_type":        exchangeType,
			"exchange_position_id": record.ExchangeID,
			"symbol":               record.Symbol,
			"side":                 side,
			"quantity":             record.Quantity,
			"entry_quantity":       record.Quantity,
			"entry_price":          record.EntryPrice,
			"entry_order_id":       "",
			"entry_time":           record.EntryTime.Format(time.RFC3339),
			"exit_price":           record.ExitPrice,
			"exit_order_id":        record.OrderID,
			"exit_time":            record.ExitTime.Format(time.RFC3339),
			"realized_pnl":         record.RealizedPnL,
			"fee":                  record.Fee,
			"leverage":             record.Leverage,
			"status":               "CLOSED",
			"close_reason":         record.CloseType,
			"created_at":           record.ExitTime.Format(time.RFC3339),
			"updated_at":           record.ExitTime.Format(time.RFC3339),
		})
	}

	winRate := 0.0
	if len(records) > 0 {
		winRate = float64(winTrades) / float64(len(records)) * 100
	}
	profitFactor := 0.0
	if totalLoss > 0 {
		profitFactor = totalWin / totalLoss
	}
	avgWin := 0.0
	if winTrades > 0 {
		avgWin = totalWin / float64(winTrades)
	}
	avgLoss := 0.0
	if lossTrades > 0 {
		avgLoss = totalLoss / float64(lossTrades)
	}

	return gin.H{
		"positions": positions,
		"stats": gin.H{
			"total_trades":     len(records),
			"win_trades":       winTrades,
			"loss_trades":      lossTrades,
			"win_rate":         winRate,
			"profit_factor":    profitFactor,
			"sharpe_ratio":     0,
			"total_pnl":        totalPnL,
			"total_fee":        totalFee,
			"avg_win":          avgWin,
			"avg_loss":         avgLoss,
			"max_drawdown_pct": 0,
		},
		"symbol_stats":    []gin.H{},
		"direction_stats": []gin.H{},
	}
}

func buildGateCloseTradeHistoryRecords(trades []gate.GateTrade, existing []tradertypes.ClosedPnLRecord, st *nofxstore.Store, traderID string) []tradertypes.ClosedPnLRecord {
	out := make([]tradertypes.ClosedPnLRecord, 0)
	all := append([]tradertypes.ClosedPnLRecord{}, existing...)
	for _, trade := range trades {
		if !strings.HasPrefix(trade.OrderAction, "close_") || trade.FillQty <= 0 || trade.FillPrice <= 0 {
			continue
		}
		side := "LONG"
		if strings.Contains(trade.OrderAction, "short") {
			side = "SHORT"
		}
		record := tradertypes.ClosedPnLRecord{
			Symbol:      market.Normalize(strings.ReplaceAll(trade.Symbol, "_", "")),
			Side:        side,
			ExitPrice:   trade.FillPrice,
			Quantity:    trade.FillQty,
			RealizedPnL: trade.ProfitLoss,
			Fee:         trade.Fee,
			Leverage:    1,
			EntryTime:   trade.ExecTime,
			ExitTime:    trade.ExecTime,
			OrderID:     firstNonEmpty(trade.OrderID, trade.TradeID),
			CloseType:   "partial_close",
			ExchangeID:  firstNonEmpty(trade.TradeID, trade.OrderID),
		}
		record.EntryPrice = estimateGateCloseEntryPrice(st, traderID, record.Symbol, side, trade.ExecTime, trade.FillPrice, trade.FillQty, trade.ProfitLoss, trade.Fee)
		if record.EntryPrice <= 0 {
			continue
		}
		if record.RealizedPnL == 0 {
			if side == "SHORT" {
				record.RealizedPnL = (record.EntryPrice - record.ExitPrice) * record.Quantity
			} else {
				record.RealizedPnL = (record.ExitPrice - record.EntryPrice) * record.Quantity
			}
			record.RealizedPnL -= record.Fee
		}
		if hasSimilarClosedRecord(all, record) {
			continue
		}
		out = append(out, record)
		all = append(all, record)
	}
	return out
}

func estimateGateCloseEntryPrice(st *nofxstore.Store, traderID, symbol, side string, closeTime time.Time, exitPrice, quantity, pnl, fee float64) float64 {
	closeMs := closeTime.UTC().UnixMilli()
	if st != nil {
		if positions, err := st.Position().GetClosedPositions(traderID, 200); err == nil {
			bestPrice := 0.0
			bestDistance := int64(math.MaxInt64)
			for _, pos := range positions {
				if !strings.EqualFold(pos.Symbol, symbol) || !strings.EqualFold(pos.Side, side) || pos.EntryPrice <= 0 {
					continue
				}
				distance := absInt64(pos.EntryTime - closeMs)
				if pos.CreatedAt > 0 {
					distance = minInt64(distance, absInt64(pos.CreatedAt-closeMs))
				}
				if distance < bestDistance {
					bestDistance = distance
					bestPrice = pos.EntryPrice
				}
			}
			if bestPrice > 0 {
				return bestPrice
			}
		}
		if orders, err := st.Order().GetTraderOrders(traderID, 200); err == nil {
			openAction := "open_long"
			if side == "SHORT" {
				openAction = "open_short"
			}
			totalQty := 0.0
			totalValue := 0.0
			for _, order := range orders {
				if !strings.EqualFold(order.Symbol, symbol) || order.OrderAction != openAction || order.FilledAt > closeMs {
					continue
				}
				price := order.AvgFillPrice
				if price <= 0 {
					price = order.Price
				}
				qty := order.FilledQuantity
				if qty <= 0 {
					qty = order.Quantity
				}
				if price <= 0 || qty <= 0 {
					continue
				}
				totalQty += qty
				totalValue += qty * price
			}
			if totalQty > 0 {
				return totalValue / totalQty
			}
		}
	}
	if quantity > 0 && pnl != 0 {
		if side == "SHORT" {
			return exitPrice + (pnl+fee)/quantity
		}
		return exitPrice - (pnl+fee)/quantity
	}
	return exitPrice
}

func hasSimilarClosedRecord(records []tradertypes.ClosedPnLRecord, candidate tradertypes.ClosedPnLRecord) bool {
	for _, record := range records {
		if candidate.ExchangeID != "" && (record.ExchangeID == candidate.ExchangeID || record.OrderID == candidate.ExchangeID) {
			return true
		}
		if !strings.EqualFold(record.Symbol, candidate.Symbol) || !strings.EqualFold(record.Side, candidate.Side) {
			continue
		}
		if math.Abs(record.Quantity-candidate.Quantity) > 0.00000001 {
			continue
		}
		if math.Abs(record.ExitTime.Sub(candidate.ExitTime).Seconds()) <= 180 {
			return true
		}
	}
	return false
}

func filterGateAggregateClosedPnL(records []tradertypes.ClosedPnLRecord) []tradertypes.ClosedPnLRecord {
	if len(records) < 3 {
		return records
	}
	filtered := make([]tradertypes.ClosedPnLRecord, 0, len(records))
	for i, record := range records {
		if !strings.EqualFold(record.CloseType, "position_close") || !gateAggregateCoveredByDetails(records, i, record) {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func gateAggregateCoveredByDetails(records []tradertypes.ClosedPnLRecord, aggregateIndex int, aggregate tradertypes.ClosedPnLRecord) bool {
	start := aggregate.EntryTime.Add(-5 * time.Minute)
	if aggregate.EntryTime.IsZero() || !aggregate.EntryTime.Before(aggregate.ExitTime) {
		start = aggregate.ExitTime.Add(-24 * time.Hour)
	}
	end := aggregate.ExitTime.Add(5 * time.Minute)

	count := 0
	totalQty := 0.0
	totalExitValue := 0.0
	for i, record := range records {
		if i == aggregateIndex || strings.EqualFold(record.CloseType, "position_close") {
			continue
		}
		if !strings.EqualFold(record.Symbol, aggregate.Symbol) || !strings.EqualFold(record.Side, aggregate.Side) {
			continue
		}
		if record.Quantity <= 0 || record.ExitPrice <= 0 || record.ExitTime.Before(start) || record.ExitTime.After(end) {
			continue
		}
		count++
		totalQty += record.Quantity
		totalExitValue += record.Quantity * record.ExitPrice
	}
	if count < 2 || !closeEnough(totalQty, aggregate.Quantity, math.Max(0.00000001, aggregate.Quantity*0.0001)) {
		return false
	}
	weightedExit := totalExitValue / totalQty
	return closeEnough(weightedExit, aggregate.ExitPrice, math.Max(0.01, aggregate.ExitPrice*0.0005))
}

func closeEnough(a, b, tolerance float64) bool {
	return math.Abs(a-b) <= tolerance
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// handleTrades Historical trades list
func (s *Server) handleTrades(c *gin.Context) {
	_, traderID, err := s.getTraderFromQuery(c)
	if err != nil {
		SafeBadRequest(c, "Invalid trader ID")
		return
	}

	trader, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		SafeNotFound(c, "Trader")
		return
	}

	// Get optional query parameters
	symbol := c.Query("symbol")
	limitStr := c.DefaultQuery("limit", "100")
	limit := 100
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}

	// Normalize symbol (add USDT suffix if not present)
	if symbol != "" {
		symbol = market.Normalize(symbol)
	}

	// Get trades from store
	store := trader.GetStore()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Store not available"})
		return
	}

	allTrades, err := store.Position().GetRecentTrades(trader.GetID(), limit)
	if err != nil {
		SafeInternalError(c, "Get trades", err)
		return
	}

	// Filter by symbol if specified
	if symbol != "" {
		var result []interface{}
		for _, trade := range allTrades {
			if trade.Symbol == symbol {
				result = append(result, trade)
			}
		}
		c.JSON(http.StatusOK, result)
		return
	}

	c.JSON(http.StatusOK, allTrades)
}

// handleOrders Order list (all orders including open, close, stop loss, take profit, etc.)
func (s *Server) handleOrders(c *gin.Context) {
	_, traderID, err := s.getTraderFromQuery(c)
	if err != nil {
		SafeBadRequest(c, "Invalid trader ID")
		return
	}

	trader, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		SafeNotFound(c, "Trader")
		return
	}

	// Get optional query parameters
	symbol := c.Query("symbol")
	statusFilter := c.Query("status") // NEW, FILLED, CANCELED, etc.
	limitStr := c.DefaultQuery("limit", "100")
	limit := 100
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}

	// Normalize symbol (add USDT suffix if not present)
	if symbol != "" {
		symbol = market.Normalize(symbol)
	}

	// Get orders from store
	store := trader.GetStore()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Store not available"})
		return
	}

	// Get orders with filters applied at database level
	orders, err := store.Order().GetTraderOrdersFiltered(trader.GetID(), symbol, statusFilter, limit)
	if err != nil {
		SafeInternalError(c, "Get orders", err)
		return
	}

	c.JSON(http.StatusOK, orders)
}

// handleOrderFills Order fill details (all fills for a specific order)
func (s *Server) handleOrderFills(c *gin.Context) {
	orderIDStr := c.Param("id")
	orderID, err := strconv.ParseInt(orderIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid order ID"})
		return
	}

	_, traderID, err := s.getTraderFromQuery(c)
	if err != nil {
		SafeBadRequest(c, "Invalid trader ID")
		return
	}

	trader, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		SafeNotFound(c, "Trader")
		return
	}

	store := trader.GetStore()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Store not available"})
		return
	}

	// Get fills for this order
	fills, err := store.Order().GetOrderFills(orderID)
	if err != nil {
		SafeInternalError(c, "Get order fills", err)
		return
	}

	c.JSON(http.StatusOK, fills)
}

// handleOpenOrders Get open orders (pending SL/TP) from exchange
func (s *Server) handleOpenOrders(c *gin.Context) {
	_, traderID, err := s.getTraderFromQuery(c)
	if err != nil {
		SafeBadRequest(c, "Invalid trader ID")
		return
	}

	trader, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		SafeNotFound(c, "Trader")
		return
	}

	// symbol 可选：不传或空表示拉取**当前账户全部交易对**未成交委托（含限价、止盈止损条件单等），供数据看板展示
	symbol := strings.TrimSpace(c.Query("symbol"))
	if symbol != "" {
		symbol = market.Normalize(symbol)
	}

	openOrders, err := trader.GetOpenOrders(symbol)
	if err != nil {
		SafeInternalError(c, "Get open orders", err)
		return
	}

	c.JSON(http.StatusOK, openOrders)
}
