package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"nofx/store"
	tradertypes "nofx/trader/types"
)

// 单跟单用户拉交易所持仓/挂单的上限；串行叠加曾导致整接口 >2 分钟、前端 30s 超时后侧边栏一直「加载中」
const comkunFollowerBoardExchangeTimeout = 6 * time.Second

type comkunMasterBoardBroadcast struct {
	ID                  uint64  `json:"id"`
	SourceStrategyID    string  `json:"source_strategy_id"`
	MasterAccountEquity float64 `json:"master_account_equity"`
	CreatedAt           string  `json:"created_at"`
	AnalysisPreview     string  `json:"analysis_preview"`
	DecisionCount       int     `json:"decision_count"`
	MasterState         json.RawMessage `json:"master_state,omitempty"`
}

type screenMonitorPosition struct {
	Symbol           string  `json:"symbol"`
	Side             string  `json:"side"`
	EntryPrice       float64 `json:"entry_price"`
	Quantity         float64 `json:"quantity"`
	Leverage         int     `json:"leverage"`
	MarginUsed       float64 `json:"margin_used"`
	MarkPrice        float64 `json:"mark_price,omitempty"`
	UnrealizedPnl    float64 `json:"unrealized_pnl,omitempty"`
	UnrealizedPnlPct float64 `json:"unrealized_pnl_pct,omitempty"`
	UpdateTime       int64   `json:"update_time"`
}

type screenMonitorMasterState struct {
	Version   int                     `json:"v"`
	Positions []screenMonitorPosition `json:"positions"`
}

type comkunFollowerBoardRow struct {
	TraderID        string                       `json:"trader_id"`
	TraderName      string                       `json:"trader_name"`
	UserID          string                       `json:"user_id"`
	UserLabel       string                       `json:"user_label"`
	UserEmailMasked string                       `json:"user_email_masked"`
	StrategyID      string                       `json:"strategy_id"`
	StrategyName    string                       `json:"strategy_name"`
	IsRunning       bool                         `json:"is_running"`
	ExchangeType    string                       `json:"exchange_type"`
	StatusHint      string                       `json:"status_hint"`
	Positions       []comkunFollowerBoardPosition `json:"positions"`
	Orders          []comkunFollowerBoardOrder    `json:"orders"`
}

type comkunFollowerBoardPosition struct {
	ID          string  `json:"id"`
	TraderID    string  `json:"trader_id"`
	Symbol      string  `json:"symbol"`
	Side        string  `json:"side"`
	Quantity    float64 `json:"quantity"`
	EntryPrice  float64 `json:"entry_price"`
	MarkPrice   float64 `json:"mark_price,omitempty"`
	Leverage    int     `json:"leverage"`
	Status      string  `json:"status"`
	UnrealizedPnl float64 `json:"unrealized_pnl,omitempty"`
}

type comkunFollowerBoardOrder struct {
	ID            string  `json:"id"`
	TraderID      string  `json:"trader_id"`
	Symbol        string  `json:"symbol"`
	Side          string  `json:"side"`
	PositionSide  string  `json:"position_side"`
	Type          string  `json:"type"`
	Status        string  `json:"status"`
	Quantity      float64 `json:"quantity"`
	Price         float64 `json:"price"`
	StopPrice     float64 `json:"stop_price"`
	FilledQuantity float64 `json:"filled_quantity,omitempty"`
	AvgFillPrice   float64 `json:"avg_fill_price,omitempty"`
	Leverage      int     `json:"leverage,omitempty"`
	TakeProfitPrice float64 `json:"take_profit_price,omitempty"`
	StopLossPrice   float64 `json:"stop_loss_price,omitempty"`
	EstimatedMargin float64 `json:"estimated_margin,omitempty"`
}

func asBoardString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asBoardFloat(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	default:
		return 0
	}
}

func asBoardInt(v interface{}) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case float32:
		return int(x)
	default:
		return 0
	}
}

// boardBracketTakeProfitStopLoss 从订单类型解析止盈/止损展示价（与币安_algo 拆条 id 后缀 _tp/_sl 对齐）。
func boardBracketTakeProfitStopLoss(orderID, typ string, stopPrice, price float64) (takeProfit, stopLoss float64) {
	id := strings.TrimSpace(orderID)
	trig := stopPrice
	if trig <= 0 {
		trig = price
	}
	if trig <= 0 {
		return 0, 0
	}
	if strings.HasSuffix(id, "_tp") {
		return trig, 0
	}
	if strings.HasSuffix(id, "_sl") {
		return 0, trig
	}
	u := strings.ToUpper(strings.TrimSpace(typ))
	switch {
	case strings.Contains(u, "TAKE_PROFIT"):
		return trig, 0
	case strings.Contains(u, "TRAILING_STOP"):
		// TRAILING_STOP_MARKET：归入止损展示价
		return 0, trig
	case strings.Contains(u, "STOP_LOSS"):
		return 0, trig
	case strings.Contains(u, "STOP_MARKET"):
		// 币安 U 本位常见止损市价（类型名不含 STOP_LOSS）；否则 board 上「止损」一直为 —
		return 0, trig
	case u == "STOP":
		// 少量接口仍返回 legacy STOP
		return 0, trig
	}
	return 0, 0
}

func boardEstimatedMargin(notionalQty, refPrice float64, leverage int) float64 {
	if notionalQty <= 0 || refPrice <= 0 {
		return 0
	}
	lev := leverage
	if lev <= 0 {
		lev = 1
	}
	return notionalQty * refPrice / float64(lev)
}

func followerBoardLeverageBySymbol(positions []comkunFollowerBoardPosition) map[string]int {
	m := map[string]int{}
	for _, p := range positions {
		sym := strings.TrimSpace(p.Symbol)
		if sym == "" || p.Leverage <= 0 {
			continue
		}
		if _, ok := m[sym]; !ok {
			m[sym] = p.Leverage
		}
	}
	return m
}

type followerBoardSnapshot struct {
	positions      []comkunFollowerBoardPosition
	orders         []comkunFollowerBoardOrder
	exchangeOK     bool
}

func buildFollowerBoardSnapshot(s *Server, ref store.ComkunFollowerTraderRef) followerBoardSnapshot {
	var snap followerBoardSnapshot
	if memTrader, mErr := s.traderManager.GetTrader(ref.TraderID); mErr == nil && memTrader != nil {
		ex := memTrader.GetUnderlyingTrader()
		if exPositions, pErr := ex.GetPositions(); pErr == nil {
			snap.positions = exchangePositionsForBoard(ref.TraderID, exPositions)
		}
		if exOrders, oErr := ex.GetOpenOrders(""); oErr == nil {
			snap.exchangeOK = true
			levBySymbol := followerBoardLeverageBySymbol(snap.positions)
			snap.orders = exchangeOrdersForBoard(ref.TraderID, exOrders, levBySymbol)
		}
	}
	if snap.positions == nil {
		pos, pErr := s.store.Position().GetOpenPositions(ref.TraderID)
		if pErr != nil {
			return snap
		}
		snap.positions = dbPositionsForBoard(ref.TraderID, pos)
	}
	if !snap.exchangeOK {
		dbOpenOrders, oErr := s.store.Order().GetTraderOrdersFiltered(ref.TraderID, "", "NEW", 80)
		if oErr != nil {
			return snap
		}
		partialOrders, poErr := s.store.Order().GetTraderOrdersFiltered(ref.TraderID, "", "PARTIALLY_FILLED", 80)
		if poErr != nil {
			return snap
		}
		dbOpenOrders = append(dbOpenOrders, partialOrders...)
		snap.orders = dbOrdersForBoard(ref.TraderID, dbOpenOrders)
	}
	return snap
}

func buildFollowerBoardSnapshotWithTimeout(s *Server, ref store.ComkunFollowerTraderRef) followerBoardSnapshot {
	ch := make(chan followerBoardSnapshot, 1)
	go func() {
		ch <- buildFollowerBoardSnapshot(s, ref)
	}()
	select {
	case snap := <-ch:
		return snap
	case <-time.After(comkunFollowerBoardExchangeTimeout):
		snap := followerBoardSnapshot{}
		if pos, pErr := s.store.Position().GetOpenPositions(ref.TraderID); pErr == nil {
			snap.positions = dbPositionsForBoard(ref.TraderID, pos)
		}
		if dbOpenOrders, oErr := s.store.Order().GetTraderOrdersFiltered(ref.TraderID, "", "NEW", 80); oErr == nil {
			if partialOrders, poErr := s.store.Order().GetTraderOrdersFiltered(ref.TraderID, "", "PARTIALLY_FILLED", 80); poErr == nil {
				dbOpenOrders = append(dbOpenOrders, partialOrders...)
			}
			snap.orders = dbOrdersForBoard(ref.TraderID, dbOpenOrders)
		}
		return snap
	}
}

func followerBoardStatusHint(s *Server, traderID string) string {
	cMsg, cAt, cOk := s.store.ComkunFollow().LatestFailedConsumptionError(traderID)
	dMsg, dAt, dOk := s.store.Decision().LatestNonEmptyErrorMessage(traderID)
	if !cOk && !dOk {
		return ""
	}
	if cOk && (!dOk || cAt.After(dAt)) {
		return cMsg
	}
	if dOk {
		return dMsg
	}
	return cMsg
}

func maskEmailForBoard(email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return ""
	}
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		if len([]rune(email)) <= 3 {
			return "***"
		}
		rs := []rune(email)
		return string(rs[:2]) + "***"
	}
	name := []rune(parts[0])
	prefix := "*"
	if len(name) > 0 {
		prefix = string(name[:1])
	}
	return prefix + "***@" + parts[1]
}

func dbPositionsForBoard(traderID string, rows []*store.TraderPosition) []comkunFollowerBoardPosition {
	out := make([]comkunFollowerBoardPosition, 0, len(rows))
	for _, p := range rows {
		out = append(out, comkunFollowerBoardPosition{
			ID:            fmt.Sprintf("%d", p.ID),
			TraderID:      traderID,
			Symbol:        p.Symbol,
			Side:          p.Side,
			Quantity:      p.Quantity,
			EntryPrice:    p.EntryPrice,
			Leverage:      p.Leverage,
			Status:        p.Status,
			UnrealizedPnl: 0,
		})
	}
	return out
}

func dbOrdersForBoard(traderID string, rows []*store.TraderOrder) []comkunFollowerBoardOrder {
	out := make([]comkunFollowerBoardOrder, 0, len(rows))
	for _, o := range rows {
		tp, sl := boardBracketTakeProfitStopLoss(o.ExchangeOrderID, o.Type, o.StopPrice, o.Price)
		refPx := o.Price
		if refPx <= 0 {
			refPx = o.StopPrice
		}
		if refPx <= 0 {
			refPx = o.AvgFillPrice
		}
		em := boardEstimatedMargin(o.Quantity, refPx, o.Leverage)
		out = append(out, comkunFollowerBoardOrder{
			ID:             o.ExchangeOrderID,
			TraderID:       traderID,
			Symbol:         o.Symbol,
			Side:           o.Side,
			PositionSide:   o.PositionSide,
			Type:           o.Type,
			Status:         o.Status,
			Quantity:       o.Quantity,
			Price:          o.Price,
			StopPrice:      o.StopPrice,
			FilledQuantity: o.FilledQuantity,
			AvgFillPrice:   o.AvgFillPrice,
			Leverage:       o.Leverage,
			TakeProfitPrice: tp,
			StopLossPrice:   sl,
			EstimatedMargin: em,
		})
	}
	return out
}

func exchangePositionsForBoard(traderID string, rows []map[string]interface{}) []comkunFollowerBoardPosition {
	out := make([]comkunFollowerBoardPosition, 0, len(rows))
	for i, p := range rows {
		qty := asBoardFloat(p["positionAmt"])
		if qty < 0 {
			qty = -qty
		}
		if qty == 0 {
			continue
		}
		out = append(out, comkunFollowerBoardPosition{
			ID:            fmt.Sprintf("%s-%s-%d", strings.TrimSpace(asBoardString(p["symbol"])), strings.TrimSpace(asBoardString(p["side"])), i),
			TraderID:      traderID,
			Symbol:        asBoardString(p["symbol"]),
			Side:          asBoardString(p["side"]),
			Quantity:      qty,
			EntryPrice:    asBoardFloat(p["entryPrice"]),
			MarkPrice:     asBoardFloat(p["markPrice"]),
			Leverage:      asBoardInt(p["leverage"]),
			Status:        "OPEN",
			UnrealizedPnl: asBoardFloat(p["unRealizedProfit"]),
		})
	}
	return out
}

func exchangeOrdersForBoard(traderID string, rows []tradertypes.OpenOrder, levBySymbol map[string]int) []comkunFollowerBoardOrder {
	out := make([]comkunFollowerBoardOrder, 0, len(rows))
	for _, o := range rows {
		lev := 0
		if levBySymbol != nil {
			lev = levBySymbol[strings.TrimSpace(o.Symbol)]
		}
		tp, sl := boardBracketTakeProfitStopLoss(o.OrderID, o.Type, o.StopPrice, o.Price)
		refPx := o.Price
		if refPx <= 0 {
			refPx = o.StopPrice
		}
		em := boardEstimatedMargin(o.Quantity, refPx, lev)
		out = append(out, comkunFollowerBoardOrder{
			ID:              o.OrderID,
			TraderID:        traderID,
			Symbol:          o.Symbol,
			Side:            o.Side,
			PositionSide:    o.PositionSide,
			Type:            o.Type,
			Status:          o.Status,
			Quantity:        o.Quantity,
			Price:           o.Price,
			StopPrice:       o.StopPrice,
			Leverage:        lev,
			TakeProfitPrice: tp,
			StopLossPrice:   sl,
			EstimatedMargin: em,
		})
	}
	return out
}

// handleGetStrategyComkunMasterBoard 跟单开关「主控」策略：拉取主广播历史 + 绑定本策略的交易员挂单记录（用于构建器看板）
func (s *Server) handleGetStrategyComkunMasterBoard(c *gin.Context) {
	userID := c.GetString("user_id")
	strategyID := strings.TrimSpace(c.Param("id"))
	if strategyID == "" {
		SafeBadRequest(c, "strategy id required")
		return
	}
	st, err := s.store.Strategy().Get(userID, strategyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "策略不存在或无权查看"})
		return
	}
	cfg, err := st.ParseConfig()
	if err != nil {
		SafeBadRequest(c, "策略配置解析失败")
		return
	}
	if !cfg.ComkunFollowListingTemplate {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅「跟单开关」上架模板策略可使用主控数据看板"})
		return
	}

	rawRows, err := s.store.ComkunFollow().ListBroadcasts(strategyID, 80)
	if err != nil {
		SafeInternalError(c, "list broadcasts failed", err)
		return
	}
	broadcasts := make([]comkunMasterBoardBroadcast, 0, len(rawRows))
	for _, br := range rawRows {
		preview := strings.TrimSpace(br.AnalysisText)
		if len([]rune(preview)) > 220 {
			rs := []rune(preview)
			preview = string(rs[:220]) + "…"
		}
		dc := 0
		if raw := strings.TrimSpace(br.DecisionJSON); raw != "" && raw != "[]" {
			var arr []json.RawMessage
			if json.Unmarshal([]byte(raw), &arr) == nil {
				dc = len(arr)
			}
		}
		var masterState json.RawMessage
		if msj := strings.TrimSpace(br.MasterStateJSON); msj != "" && msj != "{}" {
			masterState = json.RawMessage(msj)
		}
		broadcasts = append(broadcasts, comkunMasterBoardBroadcast{
			ID:                  br.ID,
			SourceStrategyID:    br.SourceStrategyID,
			MasterAccountEquity: br.MasterAccountEquity,
			CreatedAt:           br.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
			AnalysisPreview:     preview,
			DecisionCount:       dc,
			MasterState:         masterState,
		})
	}

	traders, err := s.store.Trader().List(userID)
	if err != nil {
		SafeInternalError(c, "list traders failed", err)
		return
	}
	var masterTraderID, masterTraderName string
	for _, t := range traders {
		if strings.TrimSpace(t.StrategyID) == strategyID {
			masterTraderID = t.ID
			masterTraderName = t.Name
			break
		}
	}

	var orders []*store.TraderOrder
	if masterTraderID != "" {
		orders, err = s.store.Order().GetTraderOrders(masterTraderID, 120)
		if err != nil {
			SafeInternalError(c, "list orders failed", err)
			return
		}
	}

	followerRefs, err := s.store.Trader().ListComkunFollowersBySourceStrategyID(strategyID)
	if err != nil {
		SafeInternalError(c, "list user accounts failed", err)
		return
	}
	userIDs := make([]string, 0, len(followerRefs))
	seenUsers := map[string]bool{}
	for _, ref := range followerRefs {
		if strings.TrimSpace(ref.UserID) == "" || seenUsers[ref.UserID] {
			continue
		}
		seenUsers[ref.UserID] = true
		userIDs = append(userIDs, ref.UserID)
	}
	userMap, err := s.store.User().GetMapByIDs(userIDs)
	if err != nil {
		SafeInternalError(c, "list user labels failed", err)
		return
	}
	followers := make([]comkunFollowerBoardRow, len(followerRefs))
	var wg sync.WaitGroup
	for i, ref := range followerRefs {
		wg.Add(1)
		go func(i int, ref store.ComkunFollowerTraderRef) {
			defer wg.Done()
			snap := buildFollowerBoardSnapshotWithTimeout(s, ref)
			userLabel := ""
			userEmailMasked := ""
			if u, ok := userMap[ref.UserID]; ok {
				userLabel = strings.TrimSpace(u.DisplayName)
				if userLabel == "" {
					userLabel = maskEmailForBoard(u.Email)
				}
				userEmailMasked = maskEmailForBoard(u.Email)
			}
			followers[i] = comkunFollowerBoardRow{
				TraderID:        ref.TraderID,
				TraderName:      ref.TraderName,
				UserID:          ref.UserID,
				UserLabel:       userLabel,
				UserEmailMasked: userEmailMasked,
				StrategyID:      ref.StrategyID,
				StrategyName:    ref.StrategyName,
				IsRunning:       ref.IsRunning,
				ExchangeType:    ref.ExchangeType,
				StatusHint:      followerBoardStatusHint(s, ref.TraderID),
				Positions:       snap.positions,
				Orders:          snap.orders,
			}
		}(i, ref)
	}
	wg.Wait()

	c.JSON(http.StatusOK, gin.H{
		"strategy_id":        strategyID,
		"master_trader_id":   masterTraderID,
		"master_trader_name": masterTraderName,
		"broadcasts":         broadcasts,
		"orders":             orders,
		"followers":          followers,
	})
}
