package hz

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"nofx/logger"
	"nofx/store"

	"gorm.io/gorm"
)

type hzTradeProjection struct {
	trade
	quantity   float64
	executedAt time.Time
	intent     store.MirrorExecutionIntent
	order      order
}

// SyncOrdersFromHZ projects confirmed mirror executions into the local order,
// fill and position tables used by the trader and strategy history APIs.
func (t *Trader) SyncOrdersFromHZ(traderID, exchangeID, exchangeType string, st *store.Store) error {
	if st == nil {
		return fmt.Errorf("store is nil")
	}
	var page tradePage
	query := url.Values{"limit": []string{"100"}}
	if err := t.client.do(context.Background(), http.MethodGet, "/trades", query, "", &page); err != nil {
		return fmt.Errorf("get HZ trades: %w", err)
	}
	intents, err := st.MirrorExecutionIntent().ListConfirmedForTrader(traderID, 400)
	if err != nil {
		return fmt.Errorf("list HZ execution intents: %w", err)
	}

	tradeIDs := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		tradeIDs = append(tradeIDs, item.TradeID)
	}
	projectedTradeIDs := make(map[string]struct{}, len(tradeIDs))
	if len(tradeIDs) > 0 {
		var values []string
		if err := st.GormDB().Model(&store.TraderFill{}).
			Where("exchange_id = ? AND exchange_trade_id IN ?", exchangeID, tradeIDs).
			Pluck("exchange_trade_id", &values).Error; err != nil {
			return fmt.Errorf("list projected HZ trades: %w", err)
		}
		for _, value := range values {
			projectedTradeIDs[value] = struct{}{}
		}
	}
	intentsByOrderID := make(map[string]store.MirrorExecutionIntent, len(intents))
	intentsByClientID := make(map[string]store.MirrorExecutionIntent, len(intents))
	ordersByOrderID := make(map[string]order, len(intents))
	for _, intent := range intents {
		intentsByClientID[intent.ClientOrderID] = intent
		if strings.TrimSpace(intent.ExchangeOrderID) != "" {
			intentsByOrderID[intent.ExchangeOrderID] = intent
		}
	}

	projections := make([]hzTradeProjection, 0, len(page.Items))
	alreadyProjected := 0
	orderLookupFailed := 0
	unmatchedIntent := 0
	recoveredManualClose := 0
	for _, item := range page.Items {
		if _, ok := projectedTradeIDs[item.TradeID]; ok {
			alreadyProjected++
			continue
		}
		executedAt, err := time.Parse(time.RFC3339Nano, item.ExecutedAt)
		if err != nil {
			return fmt.Errorf("parse HZ trade %s time: %w", item.TradeID, err)
		}
		intent, ok := intentsByOrderID[item.OrderID]
		if !ok {
			var remoteOrder order
			if err := t.client.do(context.Background(), http.MethodGet,
				"/orders/"+url.PathEscape(item.OrderID), nil, "", &remoteOrder); err != nil {
				orderLookupFailed++
				continue
			}
			intent, ok = hzIntentByRemoteClientOrderID(intentsByClientID, remoteOrder.ClientOrderID)
			if !ok {
				intent, ok = hzManualCloseIntent(remoteOrder, traderID, exchangeID, executedAt)
				if !ok {
					unmatchedIntent++
					continue
				}
				recoveredManualClose++
			}
			ordersByOrderID[item.OrderID] = remoteOrder
			if intent.IntentKey != "" && item.OrderID != intent.ExchangeOrderID {
				if err := st.MirrorExecutionIntent().SetExchangeOrderID(intent.IntentKey, item.OrderID); err != nil {
					return fmt.Errorf("repair HZ exchange order id: %w", err)
				}
			}
		}
		spec, err := t.instrument(item.Instrument)
		if err != nil {
			return fmt.Errorf("load HZ instrument %s: %w", item.Instrument, err)
		}
		quantity, err := quantityForLots(spec, item.Lots)
		if err != nil {
			return fmt.Errorf("convert HZ trade %s quantity: %w", item.TradeID, err)
		}
		projections = append(projections, hzTradeProjection{
			trade: item, quantity: quantity, executedAt: executedAt, intent: intent, order: ordersByOrderID[item.OrderID],
		})
	}
	sort.Slice(projections, func(i, j int) bool { return projections[i].executedAt.Before(projections[j].executedAt) })

	orderStore := st.Order()
	created := 0
	missingOpeningIntent := 0
	for _, projection := range projections {
		existingFill, err := orderStore.GetFillByExchangeTradeID(exchangeID, projection.TradeID)
		if err != nil {
			return err
		}
		if existingFill != nil {
			alreadyProjected++
			continue
		}
		positionSide := strings.ToUpper(projection.intent.PositionSide)
		orderAction, err := hzOrderAction(projection.intent.Action, positionSide)
		if err != nil {
			return err
		}
		tradeTime := projection.executedAt.UTC().UnixMilli()
		leverage := projection.order.Leverage
		if leverage <= 0 {
			leverage = 1
		}
		symbol := strings.ToUpper(strings.TrimSpace(projection.Instrument))
		isClose := strings.HasPrefix(orderAction, "close_")
		orderSide, err := hzTradeOrderSide(projection.intent.Action, positionSide)
		if err != nil {
			return err
		}
		entryTime, entryOrderID, hasOpeningIntent := hzOpeningIntent(intents, projection.intent)
		if isClose {
			openPosition, err := st.Position().GetOpenPositionBySymbol(traderID, symbol, positionSide)
			if err != nil {
				return err
			}
			if openPosition == nil && !hasOpeningIntent && !hzConfirmedManualClose(projection.intent) {
				missingOpeningIntent++
				continue
			}
			if openPosition == nil && entryTime == 0 {
				entryTime = tradeTime - 1
			}
		}
		orderRecord := &store.TraderOrder{
			TraderID: traderID, ExchangeID: exchangeID, ExchangeType: exchangeType,
			ExchangeOrderID: projection.OrderID, ClientOrderID: projection.intent.ClientOrderID,
			Symbol: symbol, Side: orderSide, PositionSide: positionSide,
			Type: "MARKET", Quantity: projection.quantity, Price: number(projection.Price),
			Status: "FILLED", FilledQuantity: projection.quantity, AvgFillPrice: number(projection.Price),
			Commission: number(projection.Fee), CommissionAsset: "USDT", Leverage: leverage,
			ReduceOnly: isClose, ClosePosition: strings.EqualFold(projection.intent.Action, "close"), OrderAction: orderAction,
			CreatedAt: tradeTime, UpdatedAt: tradeTime, FilledAt: tradeTime,
		}
		if err := orderStore.CreateOrder(orderRecord); err != nil {
			return fmt.Errorf("create HZ order %s: %w", projection.OrderID, err)
		}
		fill := &store.TraderFill{
			TraderID: traderID, ExchangeID: exchangeID, ExchangeType: exchangeType,
			OrderID: orderRecord.ID, ExchangeOrderID: projection.OrderID, ExchangeTradeID: projection.TradeID,
			Symbol: symbol, Side: orderSide, Price: number(projection.Price),
			Quantity: projection.quantity, QuoteQuantity: number(projection.Price) * projection.quantity,
			Commission: number(projection.Fee), CommissionAsset: "USDT", RealizedPnL: number(projection.RealizedPnL),
			CreatedAt: tradeTime,
		}
		projected := false
		err = st.GormDB().Transaction(func(tx *gorm.DB) error {
			var count int64
			if err := tx.Model(&store.TraderFill{}).
				Where("exchange_id = ? AND exchange_trade_id = ?", exchangeID, projection.TradeID).
				Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return nil
			}
			if err := tx.Create(fill).Error; err != nil {
				return err
			}
			positionStore := store.NewPositionStore(tx)
			positionBuilder := store.NewPositionBuilder(positionStore)
			if isClose {
				openPosition, err := positionStore.GetOpenPositionBySymbol(traderID, symbol, positionSide)
				if err != nil {
					return err
				}
				if openPosition == nil {
					entryQuantity := projection.intent.TargetQuantity - projection.intent.DeltaQuantity
					if entryQuantity <= 0 {
						entryQuantity = projection.quantity
					}
					entryPrice := hzEntryPriceFromClose(positionSide, number(projection.Price), projection.quantity, number(projection.RealizedPnL))
					if err := positionBuilder.ProcessTrade(traderID, exchangeID, exchangeType, symbol, positionSide,
						"open_"+strings.ToLower(positionSide), entryQuantity, entryPrice, 0, 0, entryTime, entryOrderID); err != nil {
						return err
					}
				}
			}
			if err := positionBuilder.ProcessTrade(traderID, exchangeID, exchangeType, symbol, positionSide, orderAction,
				projection.quantity, number(projection.Price), number(projection.Fee), number(projection.RealizedPnL),
				tradeTime, projection.OrderID); err != nil {
				return err
			}
			projected = true
			return nil
		})
		if err != nil {
			return fmt.Errorf("project HZ fill and position from trade %s: %w", projection.TradeID, err)
		}
		if !projected {
			alreadyProjected++
			continue
		}
		created++
	}
	if created > 0 || orderLookupFailed > 0 || unmatchedIntent > 0 || missingOpeningIntent > 0 || recoveredManualClose > 0 {
		logger.Infof("HZ order+position sync: trader=%s received=%d new=%d existing=%d order_lookup_failed=%d unmatched_intent=%d missing_open_intent=%d recovered_manual_close=%d",
			traderID, len(page.Items), created, alreadyProjected, orderLookupFailed, unmatchedIntent, missingOpeningIntent, recoveredManualClose)
	}
	return nil
}

func hzConfirmedManualClose(intent store.MirrorExecutionIntent) bool {
	return intent.IntentKey == "" && strings.EqualFold(intent.Action, "close") &&
		strings.HasPrefix(intent.ClientOrderID, "api-close-") && strings.TrimSpace(intent.ExchangeOrderID) != ""
}

func hzManualCloseIntent(remoteOrder order, traderID, exchangeID string, executedAt time.Time) (store.MirrorExecutionIntent, bool) {
	if !strings.EqualFold(remoteOrder.Status, "FILLED") {
		return store.MirrorExecutionIntent{}, false
	}
	parts := strings.Split(strings.TrimSpace(remoteOrder.ClientOrderID), ":")
	clientOrderID := parts[len(parts)-1]
	const prefix = "api-close-"
	if !strings.HasPrefix(clientOrderID, prefix) {
		return store.MirrorExecutionIntent{}, false
	}
	if _, err := uuid.Parse(strings.TrimPrefix(clientOrderID, prefix)); err != nil {
		return store.MirrorExecutionIntent{}, false
	}
	positionSide := strings.ToLower(strings.TrimSpace(remoteOrder.Side))
	if positionSide != "long" && positionSide != "short" {
		return store.MirrorExecutionIntent{}, false
	}
	instrument := strings.ToUpper(strings.TrimSpace(remoteOrder.Instrument))
	if instrument == "" || strings.TrimSpace(remoteOrder.OrderID) == "" {
		return store.MirrorExecutionIntent{}, false
	}
	return store.MirrorExecutionIntent{
		TraderID: traderID, ExchangeID: exchangeID, Instrument: instrument,
		PositionSide: positionSide, Action: "close", ClientOrderID: clientOrderID,
		ExchangeOrderID: remoteOrder.OrderID, CreatedAt: executedAt,
	}, true
}

func hzIntentByRemoteClientOrderID(intents map[string]store.MirrorExecutionIntent, remoteClientOrderID string) (store.MirrorExecutionIntent, bool) {
	remoteClientOrderID = strings.TrimSpace(remoteClientOrderID)
	if intent, ok := intents[remoteClientOrderID]; ok {
		return intent, true
	}
	var matched store.MirrorExecutionIntent
	found := false
	for clientOrderID, intent := range intents {
		if clientOrderID != "" && strings.HasSuffix(remoteClientOrderID, ":"+clientOrderID) {
			if found {
				return store.MirrorExecutionIntent{}, false
			}
			matched, found = intent, true
		}
	}
	return matched, found
}

func hzTradeOrderSide(action, positionSide string) (string, error) {
	positionSide = strings.ToLower(strings.TrimSpace(positionSide))
	isClose := strings.EqualFold(action, "reduce") || strings.EqualFold(action, "close")
	if !isClose && !strings.EqualFold(action, "open") && !strings.EqualFold(action, "increase") {
		return "", fmt.Errorf("invalid HZ mirror action %q", action)
	}
	switch positionSide {
	case "long":
		if isClose {
			return "SELL", nil
		}
		return "BUY", nil
	case "short":
		if isClose {
			return "BUY", nil
		}
		return "SELL", nil
	default:
		return "", fmt.Errorf("invalid HZ position side %q", positionSide)
	}
}

func hzEntryPriceFromClose(positionSide string, exitPrice, quantity, realizedPnL float64) float64 {
	if exitPrice <= 0 || quantity <= 0 {
		return exitPrice
	}
	entryPrice := exitPrice - realizedPnL/quantity
	if strings.EqualFold(positionSide, "short") {
		entryPrice = exitPrice + realizedPnL/quantity
	}
	if entryPrice <= 0 {
		return exitPrice
	}
	return entryPrice
}

func hzOpeningIntent(intents []store.MirrorExecutionIntent, closeIntent store.MirrorExecutionIntent) (int64, string, bool) {
	var best *store.MirrorExecutionIntent
	for idx := range intents {
		candidate := &intents[idx]
		if candidate.TraderID != closeIntent.TraderID ||
			!strings.EqualFold(candidate.Instrument, closeIntent.Instrument) ||
			!strings.EqualFold(candidate.PositionSide, closeIntent.PositionSide) ||
			candidate.CreatedAt.After(closeIntent.CreatedAt) ||
			!strings.EqualFold(candidate.Action, "open") {
			continue
		}
		if best == nil || candidate.CreatedAt.After(best.CreatedAt) {
			best = candidate
		}
	}
	if best == nil || strings.TrimSpace(best.ExchangeOrderID) == "" {
		return 0, "", false
	}
	return best.CreatedAt.UTC().UnixMilli(), best.ExchangeOrderID, true
}

func hzOrderAction(action, positionSide string) (string, error) {
	suffix := strings.ToLower(positionSide)
	if suffix != "long" && suffix != "short" {
		return "", fmt.Errorf("invalid HZ position side %q", positionSide)
	}
	switch strings.ToLower(action) {
	case "open", "increase":
		return "open_" + suffix, nil
	case "reduce", "close":
		return "close_" + suffix, nil
	default:
		return "", fmt.Errorf("invalid HZ mirror action %q", action)
	}
}

// StartOrderSync runs an initial recovery pass, then keeps local history in sync.
func (t *Trader) StartOrderSync(traderID, exchangeID, exchangeType string, st *store.Store, interval time.Duration) {
	syncOnce := func() {
		if err := t.SyncOrdersFromHZ(traderID, exchangeID, exchangeType, st); err != nil {
			logger.Infof("⚠️ HZ order+position sync failed: %v", err)
		}
	}
	syncOnce()
	ctx := t.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				syncOnce()
			}
		}
	}()
	logger.Infof("🔄 HZ order+position sync started (interval: %s)", interval)
}
