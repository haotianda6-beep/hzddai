package binance

import (
	"context"
	"fmt"
	"nofx/logger"
	"nofx/trader/types"
	"strconv"
	"strings"
	"time"

	"github.com/adshao/go-binance/v2/futures"
)

// IsUserDataStreamActive 用户数据流已启动（持仓/余额优先读 WS 写入的内存缓存，勿每轮清缓存打 REST）。
func (t *FuturesTrader) IsUserDataStreamActive() bool {
	t.userDataStreamMutex.Lock()
	defer t.userDataStreamMutex.Unlock()
	return t.userDataStreamStarted
}

func (t *FuturesTrader) StartUserDataStream(traderID string) {
	if t.useDemoTrading {
		// go-binance 用户流依赖全局 UseTestnet，与主网混跑会串线；虚拟盘仅用 REST 缓存即可
		logger.Infof("ℹ️ [%s] Binance 虚拟盘跳过 user data stream（REST 轮询持仓）", traderID)
		return
	}
	t.userDataStreamMutex.Lock()
	if t.userDataStreamStarted {
		t.userDataStreamMutex.Unlock()
		return
	}
	t.userDataStreamStarted = true
	t.userDataStreamMutex.Unlock()

	go t.userDataStreamLoop(traderID)
}

func (t *FuturesTrader) userDataStreamLoop(traderID string) {
	for {
		listenKey, err := t.client.NewStartUserStreamService().Do(context.Background())
		if err != nil {
			t.markRateLimited(err)
			logger.Warnf("⚠️ [%s] Binance user stream listenKey failed: %v", traderID, err)
			time.Sleep(30 * time.Second)
			continue
		}

		logger.Infof("🔌 [%s] Binance user data stream connected", traderID)
		doneC, stopC, err := futures.WsUserDataServe(
			listenKey,
			func(event *futures.WsUserDataEvent) {
				t.handleUserDataEvent(event)
			},
			func(err error) {
				logger.Warnf("⚠️ [%s] Binance user data stream error: %v", traderID, err)
			},
		)
		if err != nil {
			logger.Warnf("⚠️ [%s] Binance user data stream start failed: %v", traderID, err)
			_ = t.client.NewCloseUserStreamService().ListenKey(listenKey).Do(context.Background())
			time.Sleep(15 * time.Second)
			continue
		}

		t.userDataStreamMutex.Lock()
		t.userDataStreamStopC = stopC
		t.userDataStreamMutex.Unlock()

		keepAliveTicker := time.NewTicker(25 * time.Minute)
		select {
		case <-doneC:
			keepAliveTicker.Stop()
			_ = t.client.NewCloseUserStreamService().ListenKey(listenKey).Do(context.Background())
			logger.Warnf("⚠️ [%s] Binance user data stream closed; reconnecting", traderID)
			time.Sleep(5 * time.Second)
		case <-keepAliveTicker.C:
			if err := t.client.NewKeepaliveUserStreamService().ListenKey(listenKey).Do(context.Background()); err != nil {
				t.markRateLimited(err)
				logger.Warnf("⚠️ [%s] Binance listenKey keepalive failed: %v", traderID, err)
				close(stopC)
			}
			keepAliveTicker.Stop()
		}
	}
}

func (t *FuturesTrader) handleUserDataEvent(event *futures.WsUserDataEvent) {
	if event == nil {
		return
	}
	switch event.Event {
	case futures.UserDataEventTypeOrderTradeUpdate:
		t.applyOrderTradeUpdate(event.OrderTradeUpdate)
	case futures.UserDataEventTypeAccountUpdate:
		t.applyAccountUpdate(event.AccountUpdate)
	case futures.UserDataEventTypeAlgoUpdate:
		t.applyAlgoUpdate(event.AlgoUpdate)
	case futures.UserDataEventTypeListenKeyExpired:
		t.invalidateRealtimeCaches()
	}
}

func parseWSFloat(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

func (t *FuturesTrader) applyOrderTradeUpdate(o futures.WsOrderTradeUpdate) {
	id := fmt.Sprintf("%d", o.ID)
	ord := types.OpenOrder{
		OrderID:      id,
		Symbol:       o.Symbol,
		Side:         string(o.Side),
		PositionSide: string(o.PositionSide),
		Type:         string(o.Type),
		Price:        parseWSFloat(o.OriginalPrice),
		StopPrice:    parseWSFloat(o.StopPrice),
		Quantity:     parseWSFloat(o.OriginalQty) - parseWSFloat(o.AccumulatedFilledQty),
		Status:       string(o.Status),
	}
	if ord.Quantity <= 0 {
		ord.Quantity = parseWSFloat(o.OriginalQty)
	}
	t.updateOpenOrderCache(ord)
	// 不在此处清空持仓/余额缓存，避免每笔成交都触发 REST；仓位与权益以 ACCOUNT_UPDATE 推送为准。
}

func (t *FuturesTrader) applyAlgoUpdate(o futures.WsAlgoUpdate) {
	id := strings.TrimSpace(o.OrderID)
	if id == "" {
		id = strings.TrimSpace(o.ClientAlgoID)
	}
	ord := types.OpenOrder{
		OrderID:      id,
		Symbol:       o.Symbol,
		Side:         string(o.Side),
		PositionSide: string(o.PositionSide),
		Type:         string(o.OrderType),
		Price:        parseWSFloat(o.OrderPrice),
		StopPrice:    parseWSFloat(o.TriggerPrice),
		Quantity:     parseWSFloat(o.Quantity),
		Status:       o.AlgoStatus,
	}
	t.updateOpenOrderCache(ord)
}

func (t *FuturesTrader) applyAccountUpdate(a futures.WsAccountUpdate) {
	var positions []map[string]interface{}
	var unrealSum float64
	for _, p := range a.Positions {
		amt := parseWSFloat(p.Amount)
		if amt == 0 {
			continue
		}
		side := strings.ToLower(string(p.Side))
		if side == "" || side == "both" {
			if amt > 0 {
				side = "long"
			} else {
				side = "short"
			}
		}
		if amt < 0 {
			amt = -amt
		}
		up := parseWSFloat(p.UnrealizedPnL)
		unrealSum += up
		positions = append(positions, map[string]interface{}{
			"symbol":           p.Symbol,
			"side":             side,
			"positionAmt":      amt,
			"entryPrice":       parseWSFloat(p.EntryPrice),
			"markPrice":        parseWSFloat(p.MarkPrice),
			"unRealizedProfit": up,
			"leverage":         float64(0),
			"liquidationPrice": float64(0),
		})
	}
	t.positionsCacheMutex.Lock()
	t.cachedPositions = positions
	t.positionsCacheTime = time.Now()
	t.positionsCacheMutex.Unlock()

	// 从推送合并 USDT 钱包字段，减少后续 GetBalance → /fapi/v2/account 调用
	t.balanceCacheMutex.Lock()
	merged := make(map[string]interface{})
	if t.cachedBalance != nil {
		for k, v := range t.cachedBalance {
			merged[k] = v
		}
	}
	for _, b := range a.Balances {
		if strings.EqualFold(strings.TrimSpace(b.Asset), "USDT") {
			wb := parseWSFloat(b.Balance)
			cw := parseWSFloat(b.CrossWalletBalance)
			if wb > 1e-12 {
				merged["totalWalletBalance"] = wb
			}
			if cw > 1e-12 {
				merged["availableBalance"] = cw
			} else if wb > 1e-12 {
				merged["availableBalance"] = wb
			}
			break
		}
	}
	merged["totalUnrealizedProfit"] = unrealSum
	if len(merged) > 0 {
		t.cachedBalance = merged
		t.balanceCacheTime = time.Now()
	}
	t.balanceCacheMutex.Unlock()

	t.fireComkunMasterSnapshotNotify()
}

// SetComkunMasterMirrorSnapshotNotify 主控 COMKUN 用：币安用户流刷新持仓缓存后触发（须在 goroutine 内极轻，勿阻塞 WS）。
func (t *FuturesTrader) SetComkunMasterMirrorSnapshotNotify(fn func()) {
	t.comkunMasterSnapshotNotifyMu.Lock()
	defer t.comkunMasterSnapshotNotifyMu.Unlock()
	t.comkunMasterSnapshotNotify = fn
}

// ClearComkunMasterMirrorSnapshotNotify 停止主控监控时解除回调，避免悬空闭包。
func (t *FuturesTrader) ClearComkunMasterMirrorSnapshotNotify() {
	t.comkunMasterSnapshotNotifyMu.Lock()
	defer t.comkunMasterSnapshotNotifyMu.Unlock()
	t.comkunMasterSnapshotNotify = nil
}

func (t *FuturesTrader) fireComkunMasterSnapshotNotify() {
	t.comkunMasterSnapshotNotifyMu.Lock()
	fn := t.comkunMasterSnapshotNotify
	t.comkunMasterSnapshotNotifyMu.Unlock()
	if fn != nil {
		fn()
	}
}

func (t *FuturesTrader) updateOpenOrderCache(order types.OpenOrder) {
	orderID := strings.TrimSpace(order.OrderID)
	if orderID == "" {
		return
	}
	status := strings.ToUpper(strings.TrimSpace(order.Status))
	t.openOrdersCacheMutex.Lock()
	defer t.openOrdersCacheMutex.Unlock()

	next := make([]types.OpenOrder, 0, len(t.cachedOpenOrders)+1)
	replaced := false
	for _, existing := range t.cachedOpenOrders {
		if strings.TrimSpace(existing.OrderID) == orderID {
			replaced = true
			if status == "NEW" || status == "PARTIALLY_FILLED" || status == "ACCEPTED" {
				next = append(next, order)
			}
			continue
		}
		next = append(next, existing)
	}
	if !replaced && (status == "NEW" || status == "PARTIALLY_FILLED" || status == "ACCEPTED") {
		next = append(next, order)
	}
	t.cachedOpenOrders = next
	t.openOrdersCacheTime = time.Now()
}
