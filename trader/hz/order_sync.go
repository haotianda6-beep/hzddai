package hz

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

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

	tradeOrderIDs := make(map[string]struct{}, len(page.Items))
	for _, item := range page.Items {
		tradeOrderIDs[item.OrderID] = struct{}{}
	}
	intentsByOrderID := make(map[string]store.MirrorExecutionIntent, len(intents))
	ordersByOrderID := make(map[string]order, len(intents))
	for _, intent := range intents {
		if _, ok := tradeOrderIDs[intent.ExchangeOrderID]; ok {
			intentsByOrderID[intent.ExchangeOrderID] = intent
			continue
		}
		legacyCloseID := strings.TrimSpace(intent.ExchangeOrderID) == "" ||
			(strings.EqualFold(intent.ExchangeOrderID, intent.RemotePositionID) && strings.TrimSpace(intent.RemotePositionID) != "")
		if !legacyCloseID || (!strings.EqualFold(intent.Action, "close") && !strings.EqualFold(intent.Action, "reduce")) {
			continue
		}
		remoteOrder, lookupErr := t.orderByClientID(intent.ClientOrderID)
		if lookupErr != nil || remoteOrder.OrderID == "" {
			continue
		}
		intentsByOrderID[remoteOrder.OrderID] = intent
		ordersByOrderID[remoteOrder.OrderID] = remoteOrder
		if remoteOrder.OrderID != intent.ExchangeOrderID {
			if err := st.MirrorExecutionIntent().SetExchangeOrderID(intent.IntentKey, remoteOrder.OrderID); err != nil {
				return fmt.Errorf("repair HZ exchange order id: %w", err)
			}
		}
	}

	projections := make([]hzTradeProjection, 0, len(page.Items))
	for _, item := range page.Items {
		intent, ok := intentsByOrderID[item.OrderID]
		if !ok {
			continue
		}
		spec, err := t.instrument(item.Instrument)
		if err != nil {
			return fmt.Errorf("load HZ instrument %s: %w", item.Instrument, err)
		}
		quantity, err := quantityForLots(spec, item.Lots)
		if err != nil {
			return fmt.Errorf("convert HZ trade %s quantity: %w", item.TradeID, err)
		}
		executedAt, err := time.Parse(time.RFC3339Nano, item.ExecutedAt)
		if err != nil {
			return fmt.Errorf("parse HZ trade %s time: %w", item.TradeID, err)
		}
		projections = append(projections, hzTradeProjection{
			trade: item, quantity: quantity, executedAt: executedAt, intent: intent, order: ordersByOrderID[item.OrderID],
		})
	}
	sort.Slice(projections, func(i, j int) bool { return projections[i].executedAt.Before(projections[j].executedAt) })

	orderStore := st.Order()
	created := 0
	for _, projection := range projections {
		existingFill, err := orderStore.GetFillByExchangeTradeID(exchangeID, projection.TradeID)
		if err != nil {
			return err
		}
		if existingFill != nil {
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
		orderRecord := &store.TraderOrder{
			TraderID: traderID, ExchangeID: exchangeID, ExchangeType: exchangeType,
			ExchangeOrderID: projection.OrderID, ClientOrderID: projection.intent.ClientOrderID,
			Symbol: symbol, Side: strings.ToUpper(projection.Side), PositionSide: positionSide,
			Type: "MARKET", Quantity: projection.quantity, Price: number(projection.Price),
			Status: "FILLED", FilledQuantity: projection.quantity, AvgFillPrice: number(projection.Price),
			Commission: number(projection.Fee), CommissionAsset: "USDT", Leverage: leverage,
			ReduceOnly: isClose, ClosePosition: projection.intent.Action == "close", OrderAction: orderAction,
			CreatedAt: tradeTime, UpdatedAt: tradeTime, FilledAt: tradeTime,
		}
		if err := orderStore.CreateOrder(orderRecord); err != nil {
			return fmt.Errorf("create HZ order %s: %w", projection.OrderID, err)
		}
		fill := &store.TraderFill{
			TraderID: traderID, ExchangeID: exchangeID, ExchangeType: exchangeType,
			OrderID: orderRecord.ID, ExchangeOrderID: projection.OrderID, ExchangeTradeID: projection.TradeID,
			Symbol: symbol, Side: strings.ToUpper(projection.Side), Price: number(projection.Price),
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
			positionBuilder := store.NewPositionBuilder(store.NewPositionStore(tx))
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
			continue
		}
		created++
	}
	if created > 0 {
		logger.Infof("✅ HZ order+position sync: trader=%s new_trades=%d", traderID, created)
	}
	return nil
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
