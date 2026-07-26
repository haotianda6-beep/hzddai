package api

import (
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"nofx/kernel"
	"nofx/store"
)

const defaultMT4StrategySourceID = "mt4-ea-gold-master"

var mt4StrategyStateMu sync.Mutex
var mt4StrategyPublishMu sync.Mutex

type mt4StrategySignalReq struct {
	Action           string                 `json:"action"`
	SyncComplete     bool                   `json:"sync_complete"`
	Orders           []mt4StrategySyncOrder `json:"orders,omitempty"`
	Symbol           string                 `json:"symbol"`
	Side             string                 `json:"side"`
	MT4Type          int                    `json:"mt4_type"`
	Volume           float64                `json:"volume"`
	Price            float64                `json:"price"`
	ClosePrice       float64                `json:"close_price"`
	MT4Bid           float64                `json:"mt4_bid"`
	MT4Ask           float64                `json:"mt4_ask"`
	SL               float64                `json:"sl"`
	TP               float64                `json:"tp"`
	MasterEquity     float64                `json:"master_equity"`
	MasterMarginUsed float64                `json:"master_margin_used"`
	Ticket           int64                  `json:"ticket"`
	PositionID       int64                  `json:"position_id"`
	Secret           string                 `json:"secret"`
	ClientTimestamp  int64                  `json:"client_timestamp"`
	Seq              uint64                 `json:"seq"`
}

type mt4StrategySyncOrder struct {
	Ticket  int64   `json:"ticket"`
	Symbol  string  `json:"symbol"`
	Side    string  `json:"side"`
	MT4Type int     `json:"mt4_type"`
	Volume  float64 `json:"volume"`
	Price   float64 `json:"price"`
	SL      float64 `json:"sl"`
	TP      float64 `json:"tp"`
}

type mt4StrategySignalState struct {
	Orders map[string]mt4StrategyTrackedOrder `json:"orders"`
}

type mt4StrategyTrackedOrder struct {
	Ticket          int64   `json:"ticket"`
	Symbol          string  `json:"symbol"`
	ExchangeSymbol  string  `json:"exchange_symbol"`
	Side            string  `json:"side"`
	MT4Type         int     `json:"mt4_type"`
	Volume          float64 `json:"volume"`
	Price           float64 `json:"price"`
	SL              float64 `json:"sl"`
	TP              float64 `json:"tp"`
	MasterEquity    float64 `json:"master_equity"`
	ClientTimestamp int64   `json:"client_timestamp"`
	UpdatedAt       int64   `json:"updated_at"`
}

type mt4StrategyMasterStateWire struct {
	V                int                      `json:"v"`
	Positions        []kernel.PositionInfo    `json:"positions"`
	PendingOrders    []kernel.PendingOrder    `json:"pending_orders"`
	CandidateCoins   []string                 `json:"candidate_coins,omitempty"`
	MirrorMargin     *mt4StrategyMirrorMargin `json:"mirror_margin,omitempty"`
	MT4Event         *store.MT4SignalEvent    `json:"mt4_event,omitempty"`
	MirrorSemanticFP string                   `json:"mirror_semantic_fp,omitempty"`
}

type mt4StrategyMirrorMargin struct {
	MasterMarginLeverage int     `json:"master_margin_leverage,omitempty"`
	MasterMarginUsed     float64 `json:"master_margin_used,omitempty"`
}

func (s *Server) handleMT4StrategySignal(c *gin.Context) {
	var req mt4StrategySignalReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !mt4StrategySecretOK(req.Secret) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	action := strings.ToUpper(strings.TrimSpace(req.Action))
	if action != "OPEN" && action != "CLOSE" && action != "SYNC" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "action must be OPEN, CLOSE or SYNC"})
		return
	}
	if action == "SYNC" {
		s.handleMT4StrategyFullSync(c, req)
		return
	}
	if req.Ticket == 0 {
		req.Ticket = req.PositionID
	}
	if req.Ticket == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ticket required"})
		return
	}
	mt4StrategyPublishMu.Lock()
	defer mt4StrategyPublishMu.Unlock()

	state, changed, activeOrder, previous, err := mt4StrategyApplySignal(req)
	if err != nil {
		SafeInternalError(c, "mt4 signal state update", err)
		return
	}
	if !activeOrder {
		c.JSON(http.StatusOK, gin.H{"status": "ignored_pending_order", "ticket": req.Ticket})
		return
	}
	if !changed {
		c.JSON(http.StatusOK, gin.H{"status": "duplicate_noop", "ticket": req.Ticket})
		return
	}

	masterEq := mt4StrategyMasterEquity(req.MasterEquity)
	masterMarginUsed, _ := s.mt4StrategyEffectiveMarginUsed(state, req.MasterMarginUsed)
	event := mt4StrategyBuildSignalEvent(req, previous, state)
	stateJSON, positionCount, err := mt4StrategyBuildMasterStateJSON(state, masterEq, masterMarginUsed, &event)
	if err != nil {
		SafeInternalError(c, "mt4 master state build", err)
		return
	}
	sourceID := mt4StrategySourceID()
	analysis := fmt.Sprintf("（本轮为交易所快照同步，未等待 AI 长分析完成。）MT4 EA：%s %s ticket=%d，当前持仓=%d。", action, strings.TrimSpace(req.Symbol), req.Ticket, positionCount)
	row, err := s.store.ComkunFollow().InsertBroadcast(sourceID, masterEq, analysis, "[]", string(stateJSON))
	if err != nil {
		SafeInternalError(c, "insert mt4 broadcast", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":                "ok",
		"id":                    row.ID,
		"source_strategy_id":    row.SourceStrategyID,
		"master_account_equity": row.MasterAccountEquity,
		"positions":             positionCount,
		"exchange_symbol":       mt4StrategyNormalizeSymbol(req.Symbol),
		"created_at":            row.CreatedAt,
	})
}

func (s *Server) handleMT4StrategyFullSync(c *gin.Context, req mt4StrategySignalReq) {
	mt4StrategyPublishMu.Lock()
	defer mt4StrategyPublishMu.Unlock()

	state, changed, err := mt4StrategyApplyFullSync(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	masterMarginUsed, latestHasMargin := s.mt4StrategyEffectiveMarginUsed(state, req.MasterMarginUsed)
	if !changed && (masterMarginUsed <= 0 || latestHasMargin) {
		c.JSON(http.StatusOK, gin.H{"status": "sync_noop", "positions": len(state.Orders)})
		return
	}

	masterEq := mt4StrategyMasterEquity(req.MasterEquity)
	stateJSON, positionCount, err := mt4StrategyBuildMasterStateJSON(state, masterEq, masterMarginUsed, nil)
	if err != nil {
		SafeInternalError(c, "mt4 full sync state build", err)
		return
	}
	analysis := fmt.Sprintf("（本轮为交易所快照同步，未等待 AI 长分析完成。）MT4 EA：FULL_SYNC，当前持仓=%d。", positionCount)
	row, err := s.store.ComkunFollow().InsertBroadcast(mt4StrategySourceID(), masterEq, analysis, "[]", string(stateJSON))
	if err != nil {
		SafeInternalError(c, "insert mt4 full sync broadcast", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":                "ok",
		"id":                    row.ID,
		"source_strategy_id":    row.SourceStrategyID,
		"master_account_equity": row.MasterAccountEquity,
		"positions":             positionCount,
		"created_at":            row.CreatedAt,
	})
}

func (s *Server) mt4StrategyEffectiveMarginUsed(state mt4StrategySignalState, reported float64) (float64, bool) {
	rows, err := s.store.ComkunFollow().ListBroadcasts(mt4StrategySourceID(), 20)
	if err != nil || len(rows) == 0 {
		return reported, false
	}
	latestHasMargin := false
	effective := reported
	for i, row := range rows {
		var wire mt4StrategyMasterStateWire
		if json.Unmarshal([]byte(strings.TrimSpace(row.MasterStateJSON)), &wire) != nil {
			continue
		}
		if i == 0 {
			latestHasMargin = wire.MirrorMargin != nil && wire.MirrorMargin.MasterMarginUsed > 0
		}
		if estimate := mt4StrategyEstimateMarginFromWire(state, wire); estimate > effective {
			effective = estimate
		}
		if effective > reported {
			break
		}
	}
	return effective, latestHasMargin
}

func mt4StrategyEstimateMarginFromWire(state mt4StrategySignalState, previous mt4StrategyMasterStateWire) float64 {
	if previous.MirrorMargin == nil || previous.MirrorMargin.MasterMarginUsed <= 0 {
		return 0
	}
	previousQuantity := 0.0
	for _, position := range previous.Positions {
		previousQuantity += math.Abs(position.Quantity)
	}
	currentQuantity := 0.0
	for _, order := range state.Orders {
		currentQuantity += math.Abs(order.Volume) * 100
	}
	if previousQuantity <= 0 || currentQuantity <= 0 {
		return 0
	}
	return previous.MirrorMargin.MasterMarginUsed * currentQuantity / previousQuantity
}

func mt4StrategySecretOK(got string) bool {
	got = strings.TrimSpace(got)
	expected := strings.TrimSpace(os.Getenv("MT4_STRATEGY_SIGNAL_SECRET"))
	if expected == "" {
		return got != ""
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

func mt4StrategyApplySignal(req mt4StrategySignalReq) (mt4StrategySignalState, bool, bool, *mt4StrategyTrackedOrder, error) {
	if req.MT4Type != 0 && req.MT4Type != 1 {
		return mt4StrategySignalState{}, false, false, nil, nil
	}
	mt4StrategyStateMu.Lock()
	defer mt4StrategyStateMu.Unlock()

	state, err := mt4StrategyLoadState()
	if err != nil {
		return state, false, true, nil, err
	}
	key := strconv.FormatInt(req.Ticket, 10)
	action := strings.ToUpper(strings.TrimSpace(req.Action))
	if action == "CLOSE" {
		cur, ok := state.Orders[key]
		if !ok {
			return state, false, true, nil, nil
		}
		delete(state.Orders, key)
		return state, true, true, &cur, mt4StrategySaveState(state)
	}

	next := mt4StrategyTrackedOrder{
		Ticket:          req.Ticket,
		Symbol:          strings.TrimSpace(req.Symbol),
		ExchangeSymbol:  mt4StrategyNormalizeSymbol(req.Symbol),
		Side:            mt4StrategySide(req),
		MT4Type:         req.MT4Type,
		Volume:          math.Abs(req.Volume),
		Price:           mt4StrategyPrice(req),
		SL:              req.SL,
		TP:              req.TP,
		MasterEquity:    mt4StrategyMasterEquity(req.MasterEquity),
		ClientTimestamp: req.ClientTimestamp,
		UpdatedAt:       time.Now().Unix(),
	}
	if next.Volume <= 0 || next.ExchangeSymbol == "" || next.Side == "" {
		return state, false, true, nil, fmt.Errorf("invalid active order: symbol=%q side=%q volume=%f", next.ExchangeSymbol, next.Side, next.Volume)
	}
	var previous *mt4StrategyTrackedOrder
	if cur, ok := state.Orders[key]; ok {
		previous = &cur
		if mt4StrategyTrackedEqual(cur, next) {
			return state, false, true, previous, nil
		}
	}
	state.Orders[key] = next
	return state, true, true, previous, mt4StrategySaveState(state)
}

func mt4StrategyApplyFullSync(req mt4StrategySignalReq) (mt4StrategySignalState, bool, error) {
	if !req.SyncComplete {
		return mt4StrategySignalState{}, false, fmt.Errorf("complete MT4 snapshot required")
	}
	if len(req.Orders) > 200 {
		return mt4StrategySignalState{}, false, fmt.Errorf("too many MT4 orders")
	}

	now := time.Now().Unix()
	next := mt4StrategySignalState{Orders: make(map[string]mt4StrategyTrackedOrder, len(req.Orders))}
	for _, order := range req.Orders {
		if order.Ticket == 0 {
			return mt4StrategySignalState{}, false, fmt.Errorf("sync order ticket required")
		}
		key := strconv.FormatInt(order.Ticket, 10)
		if _, exists := next.Orders[key]; exists {
			return mt4StrategySignalState{}, false, fmt.Errorf("duplicate sync ticket %d", order.Ticket)
		}
		itemReq := mt4StrategySignalReq{
			Symbol: order.Symbol, Side: order.Side, MT4Type: order.MT4Type,
			Volume: order.Volume, Price: order.Price, SL: order.SL, TP: order.TP,
		}
		tracked := mt4StrategyTrackedOrder{
			Ticket: order.Ticket, Symbol: strings.TrimSpace(order.Symbol),
			ExchangeSymbol: mt4StrategyNormalizeSymbol(order.Symbol), Side: mt4StrategySide(itemReq),
			MT4Type: order.MT4Type, Volume: math.Abs(order.Volume), Price: order.Price,
			SL: order.SL, TP: order.TP, MasterEquity: mt4StrategyMasterEquity(req.MasterEquity),
			ClientTimestamp: req.ClientTimestamp, UpdatedAt: now,
		}
		if tracked.MT4Type != 0 && tracked.MT4Type != 1 {
			return mt4StrategySignalState{}, false, fmt.Errorf("sync ticket %d is not a market order", order.Ticket)
		}
		if tracked.Volume <= 0 || tracked.ExchangeSymbol == "" || tracked.Side == "" || tracked.Price <= 0 {
			return mt4StrategySignalState{}, false, fmt.Errorf("invalid sync ticket %d", order.Ticket)
		}
		next.Orders[key] = tracked
	}

	mt4StrategyStateMu.Lock()
	defer mt4StrategyStateMu.Unlock()
	current, err := mt4StrategyLoadState()
	if err != nil {
		return current, false, err
	}
	if mt4StrategySyncStateEqual(current, next) {
		return current, false, nil
	}
	return next, true, mt4StrategySaveState(next)
}

func mt4StrategySyncStateEqual(a, b mt4StrategySignalState) bool {
	if len(a.Orders) != len(b.Orders) {
		return false
	}
	for key, left := range a.Orders {
		right, ok := b.Orders[key]
		if !ok || left.Ticket != right.Ticket || left.Symbol != right.Symbol ||
			left.ExchangeSymbol != right.ExchangeSymbol || left.Side != right.Side || left.MT4Type != right.MT4Type ||
			!almostEqual(left.Volume, right.Volume, 1e-8) || !almostEqual(left.Price, right.Price, 1e-8) ||
			!almostEqual(left.SL, right.SL, 1e-8) || !almostEqual(left.TP, right.TP, 1e-8) {
			return false
		}
	}
	return true
}

func mt4StrategyBuildSignalEvent(req mt4StrategySignalReq, previous *mt4StrategyTrackedOrder, state mt4StrategySignalState) store.MT4SignalEvent {
	action := strings.ToUpper(strings.TrimSpace(req.Action))
	event := store.MT4SignalEvent{
		Action:          action,
		Ticket:          req.Ticket,
		Seq:             req.Seq,
		Symbol:          strings.TrimSpace(req.Symbol),
		ExchangeSymbol:  mt4StrategyNormalizeSymbol(req.Symbol),
		Side:            mt4StrategySide(req),
		ClientTimestamp: req.ClientTimestamp,
	}
	if previous != nil {
		event.Symbol = previous.Symbol
		event.ExchangeSymbol = previous.ExchangeSymbol
		event.Side = previous.Side
		event.EntryPrice = previous.Price
	}
	if action == "CLOSE" {
		if previous != nil {
			event.Volume = previous.Volume
			event.MasterQuantity = previous.Volume * 100
		}
		event.Price = mt4StrategyClosePrice(req)
		return event
	}

	current := state.Orders[strconv.FormatInt(req.Ticket, 10)]
	event.PositionVolume = current.Volume
	event.EntryPrice = current.Price
	event.Price = mt4StrategyPrice(req)
	if previous == nil {
		event.Action = "OPEN"
		event.Volume = current.Volume
	} else {
		delta := current.Volume - previous.Volume
		switch {
		case delta > 1e-8:
			event.Action = "INCREASE"
			event.Volume = delta
		case delta < -1e-8:
			event.Action = "REDUCE"
			event.Volume = -delta
		default:
			event.Action = "UPDATE"
		}
	}
	event.MasterQuantity = event.Volume * 100
	return event
}

func mt4StrategyBuildMasterStateJSON(state mt4StrategySignalState, masterEq, masterMarginUsed float64, event *store.MT4SignalEvent) ([]byte, int, error) {
	keys := make([]string, 0, len(state.Orders))
	for k := range state.Orders {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	lev := mt4StrategyLeverage()
	positions := make([]kernel.PositionInfo, 0, len(keys))
	type positionMargin struct {
		index int
		value float64
	}
	syntheticMargins := make([]positionMargin, 0, len(keys))
	syntheticTotal := 0.0
	for _, k := range keys {
		o := state.Orders[k]
		if o.Volume <= 0 || o.ExchangeSymbol == "" || o.Side == "" {
			continue
		}
		qty := o.Volume * 100
		price := o.Price
		margin := 0.0
		if price > 0 && lev > 0 {
			margin = qty * price / float64(lev)
		}
		positions = append(positions, kernel.PositionInfo{
			Symbol:     o.ExchangeSymbol,
			Side:       o.Side,
			EntryPrice: price,
			MarkPrice:  price,
			Quantity:   qty,
			Leverage:   lev,
			MarginUsed: margin,
			UpdateTime: time.Now().UnixMilli(),
			StopLoss:   o.SL,
			TakeProfit: o.TP,
		})
		syntheticMargins = append(syntheticMargins, positionMargin{index: len(positions) - 1, value: margin})
		syntheticTotal += margin
	}
	// AccountMargin is authoritative for MT4, especially on cent accounts and
	// martingale books where lots do not represent actual risk consistently.
	if masterMarginUsed > 0 && syntheticTotal > 0 {
		for _, item := range syntheticMargins {
			positions[item.index].MarginUsed = masterMarginUsed * item.value / syntheticTotal
		}
	}
	wire := mt4StrategyMasterStateWire{
		V:              2,
		Positions:      positions,
		PendingOrders:  []kernel.PendingOrder{},
		CandidateCoins: mt4StrategyCandidateCoins(positions),
		MirrorMargin:   &mt4StrategyMirrorMargin{MasterMarginLeverage: lev, MasterMarginUsed: masterMarginUsed},
		MT4Event:       event,
	}
	wire.MirrorSemanticFP = mt4StrategyFingerprint(wire, masterEq)
	raw, err := json.Marshal(wire)
	return raw, len(positions), err
}

func mt4StrategyLoadState() (mt4StrategySignalState, error) {
	state := mt4StrategySignalState{Orders: map[string]mt4StrategyTrackedOrder{}}
	raw, err := os.ReadFile(mt4StrategyStatePath())
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return state, nil
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return state, err
	}
	if state.Orders == nil {
		state.Orders = map[string]mt4StrategyTrackedOrder{}
	}
	return state, nil
}

func mt4StrategySaveState(state mt4StrategySignalState) error {
	path := mt4StrategyStatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func mt4StrategyStatePath() string {
	if p := strings.TrimSpace(os.Getenv("MT4_STRATEGY_STATE_PATH")); p != "" {
		return p
	}
	return filepath.Join("data", "mt4_strategy_signal_state.json")
}

func mt4StrategySourceID() string {
	if v := strings.TrimSpace(os.Getenv("MT4_STRATEGY_SOURCE_ID")); v != "" {
		return v
	}
	return defaultMT4StrategySourceID
}

func mt4StrategyMasterEquity(v float64) float64 {
	if env, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv("MT4_STRATEGY_MASTER_EQUITY")), 64); err == nil && env > 0 {
		return env
	}
	if v > 0 {
		return v
	}
	return 10000
}

func mt4StrategyLeverage() int {
	if env, err := strconv.Atoi(strings.TrimSpace(os.Getenv("MT4_STRATEGY_LEVERAGE"))); err == nil && env > 0 {
		return env
	}
	return 20
}

func mt4StrategyNormalizeSymbol(raw string) string {
	orig := strings.ToUpper(strings.TrimSpace(raw))
	clean := strings.NewReplacer("_", "", "-", "", "/", "", ".", "").Replace(orig)
	if mapped := mt4StrategySymbolMap()[orig]; mapped != "" {
		return mapped
	}
	if mapped := mt4StrategySymbolMap()[clean]; mapped != "" {
		return mapped
	}
	if strings.Contains(clean, "XAU") || strings.Contains(clean, "GOLD") {
		if gold := strings.TrimSpace(os.Getenv("MT4_STRATEGY_GOLD_SYMBOL")); gold != "" {
			return strings.ToUpper(gold)
		}
		return "XAUUSDT"
	}
	if strings.HasSuffix(clean, "USDT") {
		return clean
	}
	if strings.HasSuffix(clean, "USD") {
		return strings.TrimSuffix(clean, "USD") + "USDT"
	}
	return clean + "USDT"
}

func mt4StrategySymbolMap() map[string]string {
	out := map[string]string{}
	for _, pair := range strings.Split(os.Getenv("MT4_STRATEGY_SYMBOL_MAP"), ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.FieldsFunc(pair, func(r rune) bool { return r == '=' || r == ':' })
		if len(parts) != 2 {
			continue
		}
		k := strings.ToUpper(strings.TrimSpace(parts[0]))
		v := strings.ToUpper(strings.TrimSpace(parts[1]))
		if k != "" && v != "" {
			out[k] = v
		}
	}
	return out
}

func mt4StrategySide(req mt4StrategySignalReq) string {
	switch req.MT4Type {
	case 0:
		return "long"
	case 1:
		return "short"
	}
	side := strings.ToUpper(strings.TrimSpace(req.Side))
	if strings.Contains(side, "BUY") {
		return "long"
	}
	if strings.Contains(side, "SELL") {
		return "short"
	}
	return ""
}

func mt4StrategyPrice(req mt4StrategySignalReq) float64 {
	if req.Price > 0 {
		return req.Price
	}
	if req.MT4Bid > 0 && req.MT4Ask > 0 {
		return (req.MT4Bid + req.MT4Ask) / 2
	}
	if req.MT4Bid > 0 {
		return req.MT4Bid
	}
	return req.MT4Ask
}

func mt4StrategyClosePrice(req mt4StrategySignalReq) float64 {
	if req.ClosePrice > 0 {
		return req.ClosePrice
	}
	if req.MT4Bid > 0 && req.MT4Ask > 0 {
		return (req.MT4Bid + req.MT4Ask) / 2
	}
	if req.MT4Bid > 0 {
		return req.MT4Bid
	}
	if req.MT4Ask > 0 {
		return req.MT4Ask
	}
	return req.Price
}

func mt4StrategyCandidateCoins(positions []kernel.PositionInfo) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(positions))
	for _, p := range positions {
		s := strings.TrimSpace(p.Symbol)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func mt4StrategyFingerprint(wire mt4StrategyMasterStateWire, masterEq float64) string {
	parts := make([]string, 0, len(wire.Positions)+1)
	parts = append(parts, fmt.Sprintf("eq:%.4f|lev:%d", masterEq, mt4StrategyLeverage()))
	for _, p := range wire.Positions {
		parts = append(parts, fmt.Sprintf("%s|%s|%.8f|%.8f|%.8f|%.8f|%d",
			p.Symbol, p.Side, p.Quantity, p.EntryPrice, p.StopLoss, p.TakeProfit, p.Leverage))
	}
	sort.Strings(parts)
	sum := md5.Sum([]byte(strings.Join(parts, ";")))
	return hex.EncodeToString(sum[:])[:16]
}

func mt4StrategyTrackedEqual(a, b mt4StrategyTrackedOrder) bool {
	return a.Ticket == b.Ticket &&
		a.Symbol == b.Symbol &&
		a.ExchangeSymbol == b.ExchangeSymbol &&
		a.Side == b.Side &&
		a.MT4Type == b.MT4Type &&
		almostEqual(a.Volume, b.Volume, 1e-8) &&
		almostEqual(a.Price, b.Price, 1e-8) &&
		almostEqual(a.SL, b.SL, 1e-8) &&
		almostEqual(a.TP, b.TP, 1e-8) &&
		almostEqual(a.MasterEquity, b.MasterEquity, 1e-2)
}

func almostEqual(a, b, eps float64) bool {
	return math.Abs(a-b) <= eps
}
