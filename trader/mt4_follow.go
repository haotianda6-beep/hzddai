package trader

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"nofx/store"
	"nofx/trader/binance"
)

var errMT4BroadcastSuperseded = errors.New("mt4 broadcast superseded by newer target")

func mirrorFollowerSizingEquity(sourceStrategyID string, initialBalance, currentEquity float64) float64 {
	if store.IsMT4GoldMasterStrategyID(sourceStrategyID) && initialBalance > 0 {
		return initialBalance
	}
	return currentEquity
}

func mt4WireHasActualMargin(wire *comkunMasterStateWire) bool {
	return wire != nil && wire.MirrorMargin != nil && wire.MirrorMargin.MasterMarginUsed > 0
}

func openMT4BinancePosition(ctx context.Context, ft *binance.FuturesTrader, symbol, side string, quantity float64, requestedLeverage int, targetNotional float64) (map[string]interface{}, int, error) {
	leverage := requestedLeverage
	if recommended, err := ft.RecommendedLeverage(symbol, requestedLeverage, targetNotional); err == nil && recommended > 0 {
		leverage = recommended
	}
	candidates := []int{leverage, 15, 10, 5, 3, 2, 1}
	seen := map[int]bool{}
	var lastErr error
	for _, candidate := range candidates {
		if candidate < 1 || candidate > leverage || seen[candidate] {
			continue
		}
		seen[candidate] = true
		if err := ft.SetLeverage(symbol, candidate); err != nil {
			lastErr = err
			continue
		}
		var result map[string]interface{}
		var err error
		if side == "long" {
			result, err = ft.OpenLongPreparedWebSocketAPI(ctx, symbol, quantity)
		} else {
			result, err = ft.OpenShortPreparedWebSocketAPI(ctx, symbol, quantity)
		}
		if err == nil {
			return result, candidate, nil
		}
		lastErr = err
		if !binance.IsMaximumPositionAtLeverageError(err) {
			return nil, candidate, err
		}
	}
	return nil, leverage, lastErr
}

func (at *AutoTrader) comkunFollowSourceIsMT4Gold() bool {
	if at.config.StrategyConfig == nil {
		return false
	}
	return store.IsMT4GoldMasterStrategyID(store.ResolveComkunFollowSourceStrategyID(at.config.StrategyConfig))
}

func (at *AutoTrader) recordMT4MirrorOrder(broadcastID uint64, result map[string]interface{}) {
	if !at.comkunFollowSourceIsMT4Gold() || at.store == nil || broadcastID == 0 || result == nil {
		return
	}
	var raw interface{}
	for _, key := range []string{"orderId", "order_id", "id"} {
		if value, ok := result[key]; ok {
			raw = value
			break
		}
	}
	if raw == nil {
		return
	}
	orderID := strings.TrimSpace(fmt.Sprint(raw))
	orderID = strings.TrimSuffix(orderID, ".0")
	_ = at.store.ComkunFollow().AppendMT4ExchangeOrderID(at.id, broadcastID, orderID)
}

func (at *AutoTrader) mt4BroadcastSuperseded(broadcastID uint64) bool {
	if !at.comkunFollowSourceIsMT4Gold() || at.store == nil || broadcastID == 0 {
		return false
	}
	latest, err := at.store.ComkunFollow().GetLatestBroadcast(store.MT4GoldMasterStrategyID)
	return err == nil && latest != nil && latest.ID > broadcastID
}
