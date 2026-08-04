package trader

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"nofx/kernel"
	"nofx/logger"
	"nofx/store"
	"nofx/trader/binance"
	hzadapter "nofx/trader/hz"
	tradertypes "nofx/trader/types"
)

// ============================================================
// 镜像跟单引擎 — WS API 优先 + REST 回退
//
//  1. 优先走 Binance WebSocket API，失败自动回退 REST
//  2. 支持市价部分平仓
//  3. 内置 WS API 令牌桶限速（默认 40 msg/s）
//
// 安全机制：
//   - 防抖平仓（mirrorSafetyMasterFlatConfirms 连续确认）
//   - 市价全平冷却（mirrorCloseCooldown）
//   - 可用保证金上限（≤82%）
//   - 最小名义值检查
//   - Lag Guard（主控接口滞后检测）
// ============================================================

const (
	mirrorSeedBaselineBlocked                  = 1e300
	mirrorSeedBaselineWaitForFlat              = -1
	mirrorStartupBaselineCheckpointStatePrefix = "startup_baseline_state:"
)

type mirrorPositionCacheInvalidator interface {
	InvalidatePositionsCache()
}

type hzMirrorTrader interface {
	QuantityForLots(symbol string, lots float64) (float64, error)
	ExecuteWithIntent(clientOrderID string, execute func() (map[string]interface{}, error)) (map[string]interface{}, error)
	LookupOrderByClientID(clientOrderID string) (map[string]interface{}, error)
}

type hzPositionIDCloser interface {
	ClosePositionByID(positionID string, quantity float64) (map[string]interface{}, error)
}

func hzMirrorIntentIDs(masterEventID, userID, traderID, instrument, side, action string) (string, string) {
	return hzMirrorIntentIDsScoped(masterEventID, userID, traderID, instrument, side, action, "")
}

func hzMirrorIntentIDsScoped(masterEventID, userID, traderID, instrument, side, action, scope string) (string, string) {
	parts := []string{
		strings.TrimSpace(masterEventID), strings.TrimSpace(userID), strings.TrimSpace(traderID),
		strings.ToUpper(strings.TrimSpace(instrument)), strings.ToLower(strings.TrimSpace(side)), strings.TrimSpace(action),
	}
	if scope = strings.TrimSpace(scope); scope != "" {
		parts = append(parts, scope)
	}
	canonical := strings.Join(parts, "|")
	sum := sha256.Sum256([]byte(canonical))
	key := fmt.Sprintf("%x", sum[:])
	return key, "comkun-" + key[:32]
}

func mirrorResultOrderID(result map[string]interface{}) string {
	if result == nil {
		return ""
	}
	for _, key := range []string{"orderId", "order_id"} {
		if value, ok := result[key].(string); ok {
			return value
		}
	}
	return ""
}

type hzCloseLeg struct {
	positionID string
	quantity   float64
}

func followerRemotePositionsForClose(positions []map[string]interface{}, key string, closeQuantity float64) []hzCloseLeg {
	matching := make([]hzCloseLeg, 0, len(positions))
	for _, position := range positions {
		symbol, _ := position["symbol"].(string)
		side, _ := position["side"].(string)
		quantity, _ := position["positionAmt"].(float64)
		quantity = math.Abs(quantity)
		if posKey(symbol, side) != key || quantity <= 0 {
			continue
		}
		if value, ok := position["positionId"].(string); ok && strings.TrimSpace(value) != "" {
			matching = append(matching, hzCloseLeg{positionID: strings.TrimSpace(value), quantity: quantity})
		}
	}
	for _, leg := range matching {
		if math.Abs(leg.quantity-closeQuantity) <= qtyEps(math.Max(leg.quantity, closeQuantity)) {
			return []hzCloseLeg{{positionID: leg.positionID, quantity: closeQuantity}}
		}
	}
	sort.Slice(matching, func(i, j int) bool { return matching[i].positionID < matching[j].positionID })
	remaining := closeQuantity
	result := make([]hzCloseLeg, 0, len(matching))
	for _, leg := range matching {
		if remaining <= qtyEps(closeQuantity) {
			break
		}
		leg.quantity = math.Min(leg.quantity, remaining)
		result = append(result, leg)
		remaining -= leg.quantity
	}
	if remaining > qtyEps(closeQuantity) {
		return nil
	}
	return result
}

func (at *AutoTrader) executeHZMirrorIntent(
	br *store.ComkunMasterBroadcast,
	wire *comkunMasterStateWire,
	instrument, side, action, remotePositionID, intentScope string,
	targetQuantity, deltaQuantity float64,
	execute func() (map[string]interface{}, error),
) (map[string]interface{}, error) {
	hzTrader, ok := at.trader.(hzMirrorTrader)
	if !ok || at.store == nil || wire == nil || strings.TrimSpace(wire.SourceEventID) == "" {
		return nil, fmt.Errorf("HZ mirror execution intent metadata is incomplete")
	}
	intentKey, clientOrderID := hzMirrorIntentIDsScoped(wire.SourceEventID, at.userID, at.id, instrument, side, action, intentScope)
	requestRaw := fmt.Sprintf("%s|%s|%s|%.12g|%.12g", instrument, side, remotePositionID, targetQuantity, deltaQuantity)
	requestHash := fmt.Sprintf("%x", sha256.Sum256([]byte(requestRaw)))
	intent, err := at.store.MirrorExecutionIntent().Ensure(store.MirrorExecutionIntentInput{
		IntentKey: intentKey, MasterEventID: wire.SourceEventID, BroadcastID: br.ID,
		UserID: at.userID, TraderID: at.id, ExchangeID: at.exchangeID,
		Instrument: strings.ToUpper(instrument), PositionSide: strings.ToLower(side), Action: action,
		RemotePositionID: remotePositionID, TargetQuantity: targetQuantity, DeltaQuantity: deltaQuantity,
		ClientOrderID: clientOrderID, RequestSHA256: requestHash,
	})
	if err != nil {
		return nil, err
	}
	if intent.Status == store.MirrorIntentConfirmed {
		return map[string]interface{}{"orderId": intent.ExchangeOrderID, "status": "RECOVERED"}, nil
	}
	if intent.Status == store.MirrorIntentSubmitted || intent.Status == store.MirrorIntentFailed {
		recovered, lookupErr := hzTrader.LookupOrderByClientID(intent.ClientOrderID)
		if lookupErr == nil {
			_ = at.store.MirrorExecutionIntent().MarkConfirmed(intent.IntentKey, mirrorResultOrderID(recovered))
			return recovered, nil
		}
		if !hzadapter.IsNotFound(lookupErr) {
			return nil, fmt.Errorf("mirror_transient: query original HZ order: %w", lookupErr)
		}
	}
	if err := at.store.MirrorExecutionIntent().MarkSubmitted(intent.IntentKey); err != nil {
		return nil, err
	}
	result, err := hzTrader.ExecuteWithIntent(intent.ClientOrderID, execute)
	if err != nil {
		_ = at.store.MirrorExecutionIntent().MarkFailed(intent.IntentKey, err.Error())
		return nil, err
	}
	if err := at.store.MirrorExecutionIntent().MarkConfirmed(intent.IntentKey, mirrorResultOrderID(result)); err != nil {
		return nil, err
	}
	return result, nil
}

func (at *AutoTrader) executeHZMirrorCloseIntents(
	br *store.ComkunMasterBroadcast,
	wire *comkunMasterStateWire,
	positions []map[string]interface{},
	key, instrument, side, action string,
	targetQuantity, closeQuantity float64,
) (map[string]interface{}, error) {
	hzTrader, ok := at.trader.(hzPositionIDCloser)
	if !ok {
		return nil, fmt.Errorf("HZ trader does not support exact position close")
	}
	legs := followerRemotePositionsForClose(positions, key, closeQuantity)
	if len(legs) == 0 {
		return nil, fmt.Errorf("mirror_transient: exact HZ position IDs for %s are unavailable", action)
	}
	var result map[string]interface{}
	multiLeg := len(legs) > 1
	for _, leg := range legs {
		intentScope := ""
		if multiLeg {
			intentScope = leg.positionID
		}
		var err error
		result, err = at.executeHZMirrorIntent(br, wire, instrument, side, action, leg.positionID, intentScope, targetQuantity, -leg.quantity, func() (map[string]interface{}, error) {
			return hzTrader.ClosePositionByID(leg.positionID, leg.quantity)
		})
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func mirrorInvalidatePositionsCache(t tradertypes.Trader) {
	if v, ok := t.(mirrorPositionCacheInvalidator); ok {
		v.InvalidatePositionsCache()
	}
}

func encodeMirrorStartupBaselineCheckpoint(baseline map[string]float64) string {
	if len(baseline) == 0 {
		return ""
	}
	raw, err := json.Marshal(baseline)
	if err != nil {
		return ""
	}
	return mirrorStartupBaselineCheckpointStatePrefix + string(raw)
}

func decodeMirrorStartupBaselineCheckpoint(checkpoint string) (map[string]float64, bool) {
	raw := strings.TrimPrefix(strings.TrimSpace(checkpoint), mirrorStartupBaselineCheckpointStatePrefix)
	if raw == strings.TrimSpace(checkpoint) || raw == "" {
		return nil, false
	}
	var baseline map[string]float64
	if err := json.Unmarshal([]byte(raw), &baseline); err != nil || len(baseline) == 0 {
		return nil, false
	}
	return baseline, true
}

func mirrorIsBinanceAPIAuthOrWhitelistError(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.Contains(s, "code=-2015") ||
		strings.Contains(s, "-2015") ||
		strings.Contains(s, "invalid api-key") ||
		strings.Contains(s, "invalid api key") ||
		strings.Contains(s, "permissions for action")
}

// seedMirrorBaselineFromMasterBroadcast 被控启动抬高水位线时，记录主控当时已有仓位（缩放后）。
// 仅用于冷启动不追老仓；运行中主控从 0→新开/补仓不在此基线内，应正常跟市价。
func (at *AutoTrader) seedMirrorBaselineFromMasterBroadcast(br *store.ComkunMasterBroadcast) {
	if br == nil {
		return
	}
	var wire comkunMasterStateWire
	if err := json.Unmarshal([]byte(strings.TrimSpace(br.MasterStateJSON)), &wire); err != nil {
		return
	}
	if at.comkunFollowSourceIsMT4Gold() && !mt4WireHasActualMargin(&wire) {
		logger.Warnf("[%s] v2 MT4 startup baseline deferred: master_margin_used missing", at.name)
		return
	}
	masterEq := br.MasterAccountEquity
	followerEq := at.initialBalance
	if at.trader != nil && !at.comkunFollowSourceIsMT4Gold() {
		if bal, err := at.trader.GetBalance(); err == nil && bal != nil {
			if v, ok := bal["totalWalletBalance"].(float64); ok && v > 0 {
				followerEq = v
			} else if v, ok := bal["total_equity"].(float64); ok && v > 0 {
				followerEq = v
			}
		}
	}
	if followerEq < 0 {
		followerEq = 0
	}
	target := buildMirrorMasterTargetFromWire(&wire, masterEq, followerEq)
	if at.comkunFollowSourceIsHZExternal() {
		if hzTrader, ok := at.trader.(hzMirrorTrader); ok {
			if hzTarget, err := buildHZMasterTargetFromWire(&wire, masterEq, followerEq, hzTrader.QuantityForLots); err == nil {
				target = hzTarget
			}
		}
	}
	if at.comkunFollowSourceIsMT4Gold() {
		followLev := store.ComkunMirrorFollowerMarginLeverageOrDefault(at.config.StrategyConfig)
		target = buildMT4MasterTargetFromWire(&wire, masterEq, followerEq, followLev)
	}
	if len(target) == 0 {
		return
	}
	at.mirrorSeedBaselineQty = target
	logger.Infof("[%s] v2 startup baseline: %d legs (skip chasing master positions present at follower start only)", at.name, len(target))
}

func (at *AutoTrader) mirrorSeedAdjustedTarget(k string, mq float64) (float64, string) {
	if at.mirrorSeedBaselineQty == nil {
		return mq, ""
	}
	baseline, ok := at.mirrorSeedBaselineQty[k]
	if !ok {
		return mq, ""
	}
	if baseline >= mirrorSeedBaselineBlocked/2 {
		return 0, fmt.Sprintf("v2 open blocked: %s (unrecoverable API error)", k)
	}
	if baseline == mirrorSeedBaselineWaitForFlat {
		return 0, fmt.Sprintf("v2 startup wait-flat: %s current master leg is ignored until it closes", k)
	}
	eps := qtyEps(math.Max(mq, baseline))
	if mq < eps {
		delete(at.mirrorSeedBaselineQty, k)
		return mq, ""
	}
	// Cold start must not chase the master position that already existed when the
	// follower started. Only the part above that startup baseline is a new target.
	tolerance := math.Max(eps, baseline*0.02)
	excess := mq - baseline
	if excess <= tolerance {
		return 0, fmt.Sprintf("v2 startup baseline: %s target=%.6f at follower start, not chasing", k, baseline)
	}
	return excess, fmt.Sprintf("v2 startup baseline: %s baseline=%.6f current=%.6f, only following new delta=%.6f", k, baseline, mq, excess)
}

func (at *AutoTrader) mirrorSeedClearMissingMasterTargets(masterTarget map[string]float64) []string {
	if at.mirrorSeedBaselineQty == nil {
		return nil
	}
	var cleared []string
	for k, baseline := range at.mirrorSeedBaselineQty {
		if baseline >= mirrorSeedBaselineBlocked/2 {
			continue
		}
		if _, ok := masterTarget[k]; ok {
			continue
		}
		delete(at.mirrorSeedBaselineQty, k)
		cleared = append(cleared, k)
	}
	return cleared
}

type mirrorQuantityFormatter interface {
	FormatQuantity(symbol string, quantity float64) (string, error)
}

func floorHZMirrorTarget(formatter mirrorQuantityFormatter, symbol string, target float64) (float64, error) {
	if target <= 0 {
		return 0, nil
	}
	formatted, err := formatter.FormatQuantity(symbol, target)
	if err != nil {
		return 0, err
	}
	floored, err := strconv.ParseFloat(formatted, 64)
	if err != nil || math.IsNaN(floored) || math.IsInf(floored, 0) || floored < 0 {
		return 0, fmt.Errorf("invalid HZ floored target %q", formatted)
	}
	return floored, nil
}

func mirrorFollowerTargetEquity(hzExternal bool, cfg *store.StrategyConfig, sourceID string, initialBalance, currentEquity, masterEquity float64) float64 {
	followerEquity := mirrorFollowerSizingEquity(sourceID, initialBalance, currentEquity)
	if hzExternal && cfg != nil && cfg.ComkunMirrorFollowerEquityRatio > 0 {
		return masterEquity * cfg.ComkunMirrorFollowerEquityRatio
	}
	return followerEquity
}

func mirrorQuantitiesAligned(formatter mirrorQuantityFormatter, key string, target, follower float64) bool {
	if formatter != nil {
		if sym, _, ok := splitPosKey(key); ok {
			targetStr, targetErr := formatter.FormatQuantity(sym, math.Abs(target))
			followerStr, followerErr := formatter.FormatQuantity(sym, math.Abs(follower))
			if targetErr == nil && followerErr == nil {
				return targetStr == followerStr
			}
		}
	}
	return math.Abs(follower-target) <= qtyEps(math.Max(math.Abs(follower), math.Abs(target)))
}

func mirrorMarketTargetMismatch(masterTarget, follower map[string]float64, formatter mirrorQuantityFormatter) error {
	for k, fq := range follower {
		mq := masterTarget[k]
		if !mirrorQuantitiesAligned(formatter, k, mq, fq) {
			return fmt.Errorf("%s target=%.8f follower=%.8f", k, mq, fq)
		}
	}
	for k, mq := range masterTarget {
		fq := follower[k]
		if !mirrorQuantitiesAligned(formatter, k, mq, fq) {
			return fmt.Errorf("%s target=%.8f follower=%.8f", k, mq, fq)
		}
	}
	return nil
}

func (at *AutoTrader) mirrorSeedBlockOpenKey(k string) {
	if at.mirrorSeedBaselineQty == nil {
		at.mirrorSeedBaselineQty = make(map[string]float64)
	}
	at.mirrorSeedBaselineQty[k] = mirrorSeedBaselineBlocked
}

// reconcileComkunFollowMasterStateV2 按主控快照对齐被控（WS API 优先 + REST 回退）。
func (at *AutoTrader) reconcileComkunFollowMasterStateV2(ctx *kernel.Context, br *store.ComkunMasterBroadcast, record *store.DecisionRecord, schemeASkipTPSL, closeOnly bool) error {
	var wire comkunMasterStateWire
	if err := json.Unmarshal([]byte(strings.TrimSpace(br.MasterStateJSON)), &wire); err != nil {
		return fmt.Errorf("v2 解析主控状态 JSON: %w", err)
	}
	webMirror := at.comkunFollowSourceIsBnScreenMirror()
	mt4Mirror := at.comkunFollowSourceIsMT4Gold()
	hzExternal := at.comkunFollowSourceIsHZExternal()
	marketOnlyMirror := webMirror || mt4Mirror || hzExternal
	if mt4Mirror && !mt4WireHasActualMargin(&wire) {
		if record != nil {
			record.ExecutionLog = append(record.ExecutionLog,
				"v2 MT4: 广播缺少主控实际保证金，跳过本条旧格式广播，等待新版 EA 事件")
		}
		return nil
	}
	if record != nil {
		mode := "api_full_v2"
		if webMirror {
			mode = "web_screen_market_only"
		} else if mt4Mirror {
			mode = "mt4_latest_target_market_only"
		} else if hzExternal {
			mode = "hz_ai_scope_market_only"
		}
		record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
			"v2 镜像协议(%s): broadcast_id=%d mirror_semantic_fp=%s scheme_a_skip_tpsl=%v",
			mode, br.ID, strings.TrimSpace(wire.MirrorSemanticFP), schemeASkipTPSL))
	}

	// Binance uses WS API fast path; other exchanges use the generic Trader path.
	ft, _ := at.trader.(*binance.FuturesTrader)

	masterEq := br.MasterAccountEquity
	sourceID := store.ResolveComkunFollowSourceStrategyID(at.config.StrategyConfig)
	followerEq := mirrorFollowerTargetEquity(hzExternal, at.config.StrategyConfig, sourceID, at.initialBalance, ctx.Account.TotalEquity, masterEq)
	if followerEq < 0 {
		followerEq = 0
	}
	masterLev := resolveMirrorMasterLevFromWire(&wire)
	fl := store.ComkunMirrorFollowerMarginLeverageOrDefault(at.config.StrategyConfig)

	// Per-symbol leverage: follower uses same leverage as master per position
	masterLevBySymbol := make(map[string]int)
	for _, mp := range wire.Positions {
		sym := strings.TrimSpace(mp.Symbol)
		if sym != "" && mp.Leverage > 0 {
			masterLevBySymbol[sym] = mp.Leverage
		}
	}

	fallbackRatio := 1.0
	if masterEq > 1e-9 {
		fallbackRatio = followerEq / masterEq
	}
	if fallbackRatio <= 0 || math.IsNaN(fallbackRatio) || math.IsInf(fallbackRatio, 0) {
		fallbackRatio = 1
	}

	// ---- Phase 1: 构建主控目标 ----
	masterTarget := buildMirrorMasterTargetFromWire(&wire, masterEq, followerEq)
	if hzExternal {
		hzTrader, ok := at.trader.(hzMirrorTrader)
		if !ok {
			return fmt.Errorf("HZ trader does not expose dynamic lot conversion")
		}
		hzTarget, hzErr := buildHZMasterTargetFromWire(&wire, masterEq, followerEq, hzTrader.QuantityForLots)
		if hzErr != nil {
			return fmt.Errorf("HZ dynamic contract conversion: %w", hzErr)
		}
		for key, rawTarget := range hzTarget {
			symbol, _, ok := splitPosKey(key)
			if !ok {
				continue
			}
			flooredTarget, floorErr := floorHZMirrorTarget(at.trader, symbol, rawTarget)
			if floorErr != nil {
				return fmt.Errorf("HZ target quantity floor: %w", floorErr)
			}
			if flooredTarget == 0 {
				delete(hzTarget, key)
				continue
			}
			hzTarget[key] = flooredTarget
		}
		masterTarget = hzTarget
	}
	if mt4Mirror {
		masterTarget = buildMT4MasterTargetFromWire(&wire, masterEq, followerEq, fl)
	}
	if cleared := at.mirrorSeedClearMissingMasterTargets(masterTarget); len(cleared) > 0 && record != nil {
		for _, k := range cleared {
			record.ExecutionLog = append(record.ExecutionLog,
				fmt.Sprintf("v2 startup baseline cleared: %s master leg is flat", k))
		}
	}

	var syncErrs []string
	mirrorMarketNeedRetry := false
	mirrorCloseNeedRetry := false

	// Ensure follower leverage matches master per-symbol leverage.
	seen := map[string]bool{}
	for _, mp := range wire.Positions {
		sym := strings.TrimSpace(mp.Symbol)
		if sym != "" && !seen[sym] {
			seen[sym] = true
			lev := masterLevBySymbol[sym]
			if lev < 1 {
				lev = masterLev
			}
			if err := at.trader.SetMarginMode(sym, at.config.IsCrossMargin); err != nil {
				logger.Infof("v2 镜像: SetMarginMode %s: %v", sym, err)
			}
			if !mt4Mirror {
				if err := at.trader.SetLeverage(sym, lev); err != nil {
					logger.Infof("v2 镜像: SetLeverage %s -> %dx: %v", sym, lev, err)
				}
			}
		}
	}

	// ---- Phase 2: 读取被控持仓 ----
	if mt4Mirror {
		mirrorInvalidatePositionsCache(at.trader)
	}
	positions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("v2 读取被控持仓: %w", err)
	}
	foll := mapFollowerPositionQuantities(positions)
	if mt4Mirror && at.mt4BroadcastSuperseded(br.ID) {
		return errMT4BroadcastSuperseded
	}

	// 主控仍显示某腿 → 清空无仓连击
	at.mirrorSafetyInitStreakMaps()
	at.mirrorSafetyMu.Lock()
	for k := range foll {
		if _, ok := masterTarget[k]; ok {
			delete(at.mirrorMasterFlatStreak, k)
		}
	}
	at.mirrorSafetyMu.Unlock()

	// ---- Phase 3: 安全平仓（主控已无该腿） ----
	marketCtx := context.Background()
	var marketCancel context.CancelFunc
	if marketOnlyMirror {
		marketCtx, marketCancel = context.WithTimeout(context.Background(), 45*time.Second)
		defer marketCancel()
	}
	for k := range foll {
		if _, ok := masterTarget[k]; ok {
			continue
		}
		sym, side, ok2 := splitPosKey(k)
		if !ok2 {
			continue
		}

		// Lag Guard：检查是否为接口滞后
		if masterWireSnapshotMayLagPosition(&wire, sym, side) {
			record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
				"v2 订单同步: %s 暂不处理，等待交易所状态确认", k))
			continue
		}

		at.mirrorSafetyMu.Lock()
		at.mirrorMasterFlatStreak[k]++
		streak := at.mirrorMasterFlatStreak[k]
		lastAt, hadLast := at.mirrorLastMirrorMarketAt[sym]
		at.mirrorSafetyMu.Unlock()

		requiredFlatConfirms := mirrorFlatConfirmRequired(webMirror)
		if streak < requiredFlatConfirms {
			record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
				"v2 订单同步: %s 安全模式：主控无此持仓连击 %d/%d，暂缓市价平仓", k, streak, requiredFlatConfirms))
			continue
		}
		if !mt4Mirror && !hzExternal && hadLast && time.Since(lastAt) < mirrorCloseCooldown {
			record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
				"v2 订单同步: %s 该合约距上次镜像市价全平不足 %.0f 秒，暂缓全平", sym, mirrorCloseCooldown.Seconds()))
			continue
		}

		record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("v2 订单同步: %s 执行全平（安全确认通过）", k))
		if mt4Mirror && at.mt4BroadcastSuperseded(br.ID) {
			return errMT4BroadcastSuperseded
		}

		// 先撤该币对全部挂单（止盈止损/限价）
		if !mt4Mirror && !hzExternal {
			if err := at.trader.CancelAllOrders(sym); err != nil {
				record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("⚠ v2 平仓前撤单 %s: %v", sym, err))
			}
		}

		// WS API 优先，失败回退 REST
		var closeErr error
		var closeResult map[string]interface{}
		if ft != nil {
			_ = v2WSBucket.wait(marketCtx)
			if mt4Mirror && side == "long" {
				closeResult, closeErr = ft.CloseLongPreparedWebSocketAPI(marketCtx, sym, foll[k])
			} else if mt4Mirror {
				closeResult, closeErr = ft.CloseShortPreparedWebSocketAPI(marketCtx, sym, foll[k])
			} else if side == "long" {
				closeResult, closeErr = ft.CloseLongWebSocketAPI(marketCtx, sym, 0)
			} else {
				closeResult, closeErr = ft.CloseShortWebSocketAPI(marketCtx, sym, 0)
			}
			if closeErr != nil {
				logger.Infof("v2 平仓 %s WS API 失败(回退REST): %v", k, closeErr)
				if side == "long" {
					closeResult, closeErr = at.trader.CloseLong(sym, 0)
				} else {
					closeResult, closeErr = at.trader.CloseShort(sym, 0)
				}
			}
		} else if hzExternal {
			closeResult, closeErr = at.executeHZMirrorCloseIntents(br, &wire, positions, k, sym, side, "close", 0, foll[k])
		} else {
			if side == "long" {
				closeResult, closeErr = at.trader.CloseLong(sym, 0)
			} else {
				closeResult, closeErr = at.trader.CloseShort(sym, 0)
			}
		}
		if closeErr != nil {
			msg := fmt.Sprintf("❌ v2 平仓失败 %s: %v", k, closeErr)
			es := strings.ToLower(closeErr.Error())
			if strings.Contains(es, "2019") || strings.Contains(es, "margin is insufficient") || strings.Contains(es, "position size too small") || strings.Contains(es, "notional") || strings.Contains(es, "2010") || mirrorIsBinanceAPIAuthOrWhitelistError(es) || strings.Contains(es, "1121") || strings.Contains(es, "invalid symbol") {
				if mirrorIsBinanceAPIAuthOrWhitelistError(es) {
					msg += "（API/IP白名单/权限错误，本轮保留重试）"
					record.ExecutionLog = append(record.ExecutionLog, msg)
					mirrorCloseNeedRetry = true
					continue
				}
				if strings.Contains(es, "1121") || strings.Contains(es, "invalid symbol") {
					msg += "（不可恢复，重置跟踪不再重试）"
				} else {
					msg += "（暂时性账户问题，本轮跳过）"
				}
				record.ExecutionLog = append(record.ExecutionLog, msg)
				at.mirrorSafetyMu.Lock()
				delete(at.mirrorMasterFlatStreak, k)
				at.mirrorSafetyMu.Unlock()
				continue
			}
			record.ExecutionLog = append(record.ExecutionLog, msg)
			mirrorCloseNeedRetry = true
		} else {
			at.recordMT4MirrorOrder(br.ID, closeResult)
			at.mirrorSafetyMu.Lock()
			at.mirrorLastMirrorMarketAt[sym] = time.Now()
			delete(at.mirrorMasterFlatStreak, k)
			at.mirrorSafetyMu.Unlock()
			if mt4Mirror && at.mt4BroadcastSuperseded(br.ID) {
				return errMT4BroadcastSuperseded
			}
		}
	}

	// 重拉持仓
	if mt4Mirror {
		mirrorInvalidatePositionsCache(at.trader)
	}
	positions, err = at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("v2 刷新被控持仓: %w", err)
	}
	foll = mapFollowerPositionQuantities(positions)
	if mt4Mirror && mirrorCloseNeedRetry {
		return fmt.Errorf("mirror_transient: v2 MT4 市价镜像旧方向平仓未完成，暂不继续开新方向")
	}

	// ---- Phase 4: 仓位差异调整（加仓 + 部分平仓） ----
	webStartupAlign := webMirror && at.mirrorWebStartupAlignPending
	if webStartupAlign && record != nil {
		record.ExecutionLog = append(record.ExecutionLog,
			"v2 网页镜像启动对齐: 首轮 Phase4 按目标仓位 mq 检查最小名义（差额过小仍可对齐；主控浮盈跳过追仓，浮亏可进场）")
	}
	at.mirrorWebPruneHighPnLSkipKeys(masterTarget)
	baselineOnlyKeys := make(map[string]bool)
	for k, mq := range masterTarget {
		fq := foll[k]
		if adjustedTarget, msg := at.mirrorSeedAdjustedTarget(k, mq); adjustedTarget != mq {
			if record != nil && msg != "" {
				record.ExecutionLog = append(record.ExecutionLog, msg)
			}
			if adjustedTarget <= qtyEps(math.Max(mq, 1)) {
				baselineOnlyKeys[k] = true
				// Startup baseline means "do not chase historical master exposure";
				// it must also avoid closing an existing same-side follower position.
				continue
			}
			mq = adjustedTarget
		}
		delta := mq - fq

		if math.Abs(delta) < qtyEps(mq) {
			continue
		}
		sym, side, ok2 := splitPosKey(k)
		if !ok2 {
			continue
		}
		if hzExternal && !hzMirrorDeltaAllowed(delta, wire.OccurredAt, time.Now(), closeOnly) {
			if record != nil {
				reason := "余额不足，close-only 禁止增加风险"
				if !closeOnly {
					reason = "开仓事件超过 2 分钟，不再补开"
				}
				record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("HZ %s %s: %s", sym, side, reason))
			}
			continue
		}
		diffQty := math.Abs(delta)
		underTargetStartup := webStartupAlign && delta > 0 && fq+qtyEps(mq) < mq
		baseQty := math.Max(math.Abs(mq), math.Abs(fq))
		if baseQty > 0 && !underTargetStartup && !mt4Mirror && !hzExternal {
			diffPct := diffQty / baseQty
			if diffPct < 0.05 {
				record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
					"v2 订单同步: %s %s 差异 %.2f%%<5%%，视为已对齐", sym, side, diffPct*100))
				continue
			}
		}

		if !hzExternal {
			if price, perr := at.trader.GetMarketPrice(sym); perr == nil && price > 0 {
				minN := mirrorMinNotionalUSDT(at, sym)
				notionalForMin := diffQty * price
				if underTargetStartup {
					notionalForMin = mq * price
				}
				missingLeg := fq+qtyEps(mq) < mq
				if webMirror && missingLeg && delta > 0 {
					notionalForMin = mq * price
				}
				skipBelowMin := notionalForMin+1e-9 < minN
				if skipBelowMin && webMirror && delta > 0 && (underTargetStartup || missingLeg) {
					bootQty := mirrorWebBootstrapOpenQty(at, sym, price, minN)
					if bootQty > fq && bootQty*price+1e-9 >= minN {
						delta = bootQty - fq
						diffQty = math.Abs(delta)
						notionalForMin = diffQty * price
						skipBelowMin = notionalForMin+1e-9 < minN
						if record != nil && !skipBelowMin {
							record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
								"v2 网页镜像: %s %s 缩放目标名义 %.4f USDT < 最小 %.2f，改用最小可下单 qty=%.6f（约 %.2f USDT）",
								sym, side, mq*price, minN, bootQty, bootQty*price))
						}
					}
				}
				if skipBelowMin {
					if underTargetStartup {
						record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
							"v2 网页镜像启动对齐: %s %s 目标名义 %.4f USDT < 最小 %.2f，跳过", sym, side, notionalForMin, minN))
					} else {
						record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
							"v2 订单同步: %s %s 差额名义 %.4f USDT < 最小 %.2f，跟单账户太小自动跳过", sym, side, notionalForMin, minN))
					}
					continue
				}
			}
		}

		if err := at.trader.SetMarginMode(sym, at.config.IsCrossMargin); err != nil {
			logger.Infof("v2 镜像: SetMarginMode %s: %v", sym, err)
		}

		if delta > 0 {
			if mt4Mirror && at.mt4BroadcastSuperseded(br.ID) {
				return errMT4BroadcastSuperseded
			}
			if webMirror && at.mirrorWebShouldSkipHighPnLOpen(k, &wire, webStartupAlign, fq) {
				continue
			}
			// ---- 加仓：WS API 优先，失败回退 REST ----
			posLev := masterLevBySymbol[sym]
			if posLev < 1 {
				posLev = fl
			}
			addQty := delta
			price, _ := at.trader.GetMarketPrice(sym)
			if !mt4Mirror {
				addQty = clampMirrorScaledQtyByAvailable(at, sym, 0, addQty, posLev, false, record)
				addQty = clampMirrorScaledQtyByAvailable(at, sym, price, addQty, posLev, false, record)
			}
			if addQty < 1e-12 {
				record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
					"v2 订单同步: %s %s 可用保证金不足，跳过（跟单账户太小）", sym, side))
				continue
			}

			var openErr error
			var openResult map[string]interface{}
			if ft != nil {
				_ = v2WSBucket.wait(marketCtx)
				if mt4Mirror {
					openResult, posLev, openErr = openMT4BinancePosition(marketCtx, ft, sym, side, addQty, posLev, mq*price)
				} else if side == "long" {
					openResult, openErr = ft.OpenLongWebSocketAPI(marketCtx, sym, addQty, posLev)
				} else {
					openResult, openErr = ft.OpenShortWebSocketAPI(marketCtx, sym, addQty, posLev)
				}
				if openErr != nil && !binance.IsMaximumPositionAtLeverageError(openErr) {
					logger.Infof("v2 开仓 %s %s WS API 失败(回退REST): %v", k, side, openErr)
					if side == "long" {
						openResult, openErr = at.trader.OpenLong(sym, addQty, posLev)
					} else {
						openResult, openErr = at.trader.OpenShort(sym, addQty, posLev)
					}
				}
			} else if hzExternal {
				action := "increase"
				if fq <= qtyEps(mq) {
					action = "open"
				}
				openResult, openErr = at.executeHZMirrorIntent(br, &wire, sym, side, action, "", "", mq, addQty, func() (map[string]interface{}, error) {
					if side == "long" {
						return at.trader.OpenLong(sym, addQty, posLev)
					}
					return at.trader.OpenShort(sym, addQty, posLev)
				})
			} else {
				if side == "long" {
					openResult, openErr = at.trader.OpenLong(sym, addQty, posLev)
				} else {
					openResult, openErr = at.trader.OpenShort(sym, addQty, posLev)
				}
			}
			if openErr != nil {
				msg := fmt.Sprintf("❌ v2 市价加%s %s qty=%.6f: %v", side, sym, addQty, openErr)
				es := strings.ToLower(openErr.Error())
				if strings.Contains(es, "2019") || strings.Contains(es, "margin is insufficient") || strings.Contains(es, "position size too small") || strings.Contains(es, "notional") || strings.Contains(es, "2010") || mirrorIsBinanceAPIAuthOrWhitelistError(es) || strings.Contains(es, "1121") || strings.Contains(es, "invalid symbol") {
					if mt4Mirror {
						msg += "（MT4 固定比例不可缩仓，本轮保留重试）"
						record.ExecutionLog = append(record.ExecutionLog, msg)
						mirrorMarketNeedRetry = true
						continue
					}
					if mirrorIsBinanceAPIAuthOrWhitelistError(es) {
						msg += "（API/IP白名单/权限错误，本轮保留重试）"
						record.ExecutionLog = append(record.ExecutionLog, msg)
						mirrorMarketNeedRetry = true
						continue
					}
					if strings.Contains(es, "1121") || strings.Contains(es, "invalid symbol") {
						msg += "（不可恢复，加入黑名单不再重试）"
					} else {
						msg += "（账户 USDT 可用不足，本轮跳过）"
					}
					record.ExecutionLog = append(record.ExecutionLog, msg)
					if strings.Contains(es, "1121") || strings.Contains(es, "invalid symbol") {
						at.mirrorSeedBlockOpenKey(k)
					}
					continue
				}
				record.ExecutionLog = append(record.ExecutionLog, msg)
				mirrorMarketNeedRetry = true
			} else {
				at.recordMT4MirrorOrder(br.ID, openResult)
				record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("✓ v2 市价加%s %s qty=%.6f", side, sym, addQty))
				// position now tracked via follower position map (foll)
				if mt4Mirror && at.mt4BroadcastSuperseded(br.ID) {
					return errMT4BroadcastSuperseded
				}
			}
		} else if delta < 0 {
			if mt4Mirror && at.mt4BroadcastSuperseded(br.ID) {
				return errMT4BroadcastSuperseded
			}
			// ---- 减仓：WS API 部分平仓优先，失败回退 REST ----
			closeQty := math.Abs(delta)
			if !hzExternal {
				if price, perr := at.trader.GetMarketPrice(sym); perr == nil && price > 0 {
					minN := mirrorMinNotionalUSDT(at, sym)
					if closeQty*price+1e-9 < minN {
						record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
							"v2 订单同步: %s %s 需减仓名义 %.4f USDT < 最小 %.2f，跳过部分平仓", sym, side, closeQty*price, minN))
						continue
					}
				}
			}

			var closeErr error
			var closeResult map[string]interface{}
			if ft != nil && v2WSPartialCloseEnabled {
				_ = v2WSBucket.wait(marketCtx)
				if side == "long" {
					closeResult, closeErr = ft.ClosePartialLongWebSocketAPI(marketCtx, sym, closeQty)
				} else {
					closeResult, closeErr = ft.ClosePartialShortWebSocketAPI(marketCtx, sym, closeQty)
				}
				if closeErr != nil {
					logger.Infof("v2 部分平仓 %s %s WS API 失败(回退REST): %v", k, side, closeErr)
					if side == "long" {
						closeResult, closeErr = at.trader.CloseLong(sym, closeQty)
					} else {
						closeResult, closeErr = at.trader.CloseShort(sym, closeQty)
					}
				}
			} else if hzExternal {
				closeResult, closeErr = at.executeHZMirrorCloseIntents(br, &wire, positions, k, sym, side, "reduce", mq, closeQty)
			} else {
				if side == "long" {
					closeResult, closeErr = at.trader.CloseLong(sym, closeQty)
				} else {
					closeResult, closeErr = at.trader.CloseShort(sym, closeQty)
				}
			}
			if closeErr != nil {
				msg := fmt.Sprintf("❌ v2 部分平%s %s qty=%.6f: %v", side, sym, closeQty, closeErr)
				es := strings.ToLower(closeErr.Error())
				if strings.Contains(es, "2019") || strings.Contains(es, "margin is insufficient") || strings.Contains(es, "position size too small") || strings.Contains(es, "notional") || strings.Contains(es, "2010") || mirrorIsBinanceAPIAuthOrWhitelistError(es) || strings.Contains(es, "1121") || strings.Contains(es, "invalid symbol") {
					if mirrorIsBinanceAPIAuthOrWhitelistError(es) {
						msg += "（API/IP白名单/权限错误，本轮保留重试）"
						record.ExecutionLog = append(record.ExecutionLog, msg)
						mirrorMarketNeedRetry = true
						continue
					}
					if strings.Contains(es, "1121") || strings.Contains(es, "invalid symbol") {
						msg += "（不可恢复，加入黑名单不再重试）"
					} else {
						msg += "（暂时性账户问题，本轮跳过）"
					}
					record.ExecutionLog = append(record.ExecutionLog, msg)
					if strings.Contains(es, "1121") || strings.Contains(es, "invalid symbol") {
						at.mirrorSeedBlockOpenKey(k)
					}
					continue
				}
				record.ExecutionLog = append(record.ExecutionLog, msg)
				mirrorMarketNeedRetry = true
			} else {
				at.recordMT4MirrorOrder(br.ID, closeResult)
				record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("✓ v2 部分平%s %s qty=%.6f", side, sym, closeQty))
				// position now tracked via follower position map (foll)
				if mt4Mirror && at.mt4BroadcastSuperseded(br.ID) {
					return errMT4BroadcastSuperseded
				}
			}
		}
	}
	if webStartupAlign {
		at.mirrorWebStartupAlignPending = false
		if record != nil {
			record.ExecutionLog = append(record.ExecutionLog,
				"v2 网页镜像启动对齐: 首轮 Phase4 已完成（后续按常规差额同步）")
		}
	}

	// 重拉持仓
	if mt4Mirror {
		mirrorInvalidatePositionsCache(at.trader)
	}
	positions, err = at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("v2 刷新被控持仓(限价前): %w", err)
	}
	foll = mapFollowerPositionQuantities(positions)

	if marketOnlyMirror {
		if mt4Mirror {
			if err := mirrorMarketTargetMismatch(masterTarget, foll, at.trader); err != nil {
				return fmt.Errorf("mirror_transient: v2 MT4 市价镜像最终持仓未对齐（%w）", err)
			}
		}
		if mirrorMarketNeedRetry || mirrorCloseNeedRetry {
			return fmt.Errorf("mirror_transient: v2 网页镜像市价同步未完成（加仓/全平/部分平仓重试中）")
		}
		if record != nil {
			record.ExecutionLog = append(record.ExecutionLog,
				"v2 市价镜像: 当前目标仓位同步完成，跳过限价/止盈止损")
		}
		return nil
	}

	// ---- Phase 5: 限价单同步（API 主控） ----
	// 先拉取被控现有挂单，避免反复撤了又挂（order flicker）
	existingOrders, _ := at.trader.GetOpenOrders("")
	type orderFingerprint struct {
		symbol, side, positionSide string
		price                      float64
	}
	existingByFP := make(map[orderFingerprint]bool)
	for _, eo := range existingOrders {
		fp := orderFingerprint{
			symbol:       eo.Symbol,
			side:         eo.Side,
			positionSide: eo.PositionSide,
			price:        eo.Price,
		}
		existingByFP[fp] = true
	}

	// 预扫描：判断哪些目标限价单已存在（避免反复撤挂 flicker）
	type pendingTarget struct {
		o                    kernel.PendingOrder
		sym, sideU, ps       string
		px, scaledQty        float64
		reduceOnly, postOnly bool
		alreadyExists        bool
		limLev               int
	}
	var limitTargets []pendingTarget
	symsNeedCancel := make(map[string]bool)

	for _, o := range wire.PendingOrders {
		if !kernel.IsLimitLikePendingOrderType(o.Type) {
			continue
		}
		px := kernel.PendingLimitDisplayPrice(o)
		if px <= 0 || o.Quantity <= 0 {
			continue
		}
		sym := strings.TrimSpace(o.Symbol)
		var scaledQty float64
		limLev := masterLevBySymbol[strings.TrimSpace(o.Symbol)]
		if limLev < 1 {
			limLev = fl
		}
		if masterEq > 1e-9 {
			scaledQty = mirrorScaledLimitQtyCapped(o, wire.Positions, masterEq, followerEq, masterLev, limLev)
		}
		if scaledQty < 1e-8 {
			scaledQty = o.Quantity * fallbackRatio
		}
		if scaledQty < 1e-8 {
			continue
		}

		ps := mirrorLimitPositionSide(o, wire.Positions)
		sideU := strings.ToUpper(strings.TrimSpace(o.Side))
		if baselineOnlyKeys[posKey(sym, strings.ToLower(ps))] {
			record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
				"v2 启动基线: 跳过历史仓位挂单 %s %s @%.6f", sym, sideU, px))
			continue
		}
		reduceOnly := (ps == "LONG" && sideU == "SELL") || (ps == "SHORT" && sideU == "BUY")
		typ := strings.ToUpper(strings.TrimSpace(o.Type))
		if reduceOnly && (kernel.IsConditionalTakeProfitOrderType(typ) || kernel.IsConditionalStopLossOrderType(typ)) {
			continue
		}

		scaledQty = clampMirrorScaledQtyByAvailable(at, sym, px, scaledQty, limLev, reduceOnly, record)
		minN := mirrorMinNotionalUSDT(at, sym)
		if scaledQty*px+1e-9 < minN {
			record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
				"⚠ v2 限价跳过 %s %s：名义 %.4f USDT < 最小 %.2f", sym, sideU, scaledQty*px, minN))
			continue
		}
		postOnly := typ == "LIMIT_MAKER" || strings.Contains(typ, "LIMIT_MAKER")

		targetFP := orderFingerprint{symbol: sym, side: sideU, positionSide: ps, price: px}
		exists := existingByFP[targetFP]

		limitTargets = append(limitTargets, pendingTarget{
			o: o, sym: sym, sideU: sideU, ps: ps,
			px: px, scaledQty: scaledQty,
			reduceOnly: reduceOnly, postOnly: postOnly,
			alreadyExists: exists,
			limLev:        limLev,
		})
		if !exists {
			symsNeedCancel[sym] = true
		}
	}

	// 只撤需要变动的币对（已有完全匹配限价单的币对跳过，避免 flicker）
	resyncSyms := mirrorResyncOrderSymbols(at, &wire)
	cancelled := 0
	for sym := range resyncSyms {
		// 判断该币对是否需要撤单：有目标限价单不匹配 或 无目标限价单（需清理残留）
		hasTargets := false
		allMatch := true
		for _, tl := range limitTargets {
			if tl.sym == sym {
				hasTargets = true
				if !tl.alreadyExists {
					allMatch = false
				}
			}
		}
		needCancel := !hasTargets || !allMatch
		if !needCancel {
			continue
		}
		if err := at.trader.CancelAllOrders(sym); err != nil {
			record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("⚠ v2 撤全部挂单 %s: %v", sym, err))
		} else {
			cancelled++
		}
		time.Sleep(mirrorCancelSymbolSleep)
	}
	if cancelled > 0 {
		record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("✓ v2 已在 %d 个币对上撤销全部挂单", cancelled))
	}

	// 调仓后重拉余额缓存
	if ft != nil {
		ft.InvalidateBalanceCache()
	}

	// 只挂不存在的限价单
	for _, tl := range limitTargets {
		if tl.alreadyExists {
			record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("✓ v2 限价跳过(已存在) %s %s @%.6f", tl.sym, tl.sideU, tl.px))
			continue
		}

		if err := at.trader.SetMarginMode(tl.sym, at.config.IsCrossMargin); err != nil {
			logger.Infof("v2 镜像: SetMarginMode %s: %v", tl.sym, err)
		}

		appendMirrorLimitVolumeOIExecutionLog(record, tl.sym, &wire, ctx)

		// WS API 优先，失败回退 REST (via GridTrader)
		var limitErr error
		if ft != nil && v2WSOrderCancelEnabled {
			_ = v2WSBucket.wait(marketCtx)
			_, limitErr = ft.PlaceLimitOrderWebSocketAPI(marketCtx, tl.sym, tl.sideU, tl.ps, tl.px, tl.scaledQty, tl.limLev, tl.reduceOnly, tl.postOnly)
		}
		if limitErr != nil || ft == nil || !v2WSOrderCancelEnabled {
			if limitErr != nil {
				logger.Infof("v2 限价 %s WS API 失败(回退REST): %v", tl.sym, limitErr)
			}
			// REST 回退
			if gt, ok := at.trader.(tradertypes.GridTrader); ok {
				req := &tradertypes.LimitOrderRequest{
					Symbol:       tl.sym,
					Side:         tl.sideU,
					PositionSide: tl.ps,
					Price:        tl.px,
					Quantity:     tl.scaledQty,
					Leverage:     tl.limLev,
					ReduceOnly:   tl.reduceOnly,
					PostOnly:     tl.postOnly,
				}
				_, limitErr = gt.PlaceLimitOrder(req)
			} else {
				limitErr = fmt.Errorf("exchange %s does not support limit order sync", at.exchange)
			}
		}
		if limitErr != nil {
			msg := fmt.Sprintf("❌ v2 限价失败 %s %s @%.6f qty=%.6f: %v", tl.sym, tl.sideU, tl.px, tl.scaledQty, limitErr)
			es := strings.ToLower(limitErr.Error())
			if strings.Contains(es, "2019") || strings.Contains(es, "margin is insufficient") || strings.Contains(es, "position size too small") || strings.Contains(es, "notional") || strings.Contains(es, "2010") || strings.Contains(es, "2015") || strings.Contains(es, "invalid api-key") || strings.Contains(es, "1121") || strings.Contains(es, "invalid symbol") {
				if strings.Contains(es, "2015") || strings.Contains(es, "invalid api-key") || strings.Contains(es, "1121") || strings.Contains(es, "invalid symbol") {
					msg += "（不可恢复错误）"
				} else {
					msg += "（账户 USDT 可用不足）"
				}
			}
			record.ExecutionLog = append(record.ExecutionLog, msg)
			syncErrs = append(syncErrs, msg)
		} else {
			record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("✓ v2 限价 %s %s @%.6f qty=%.6f", tl.sym, tl.sideU, tl.px, tl.scaledQty))
		}
		time.Sleep(v2WSLimitSleep)
	}
	// ---- Phase 6: 止盈止损同步 ----
	if schemeASkipTPSL {
		record.ExecutionLog = append(record.ExecutionLog,
			"v2 订单同步: 方案A — 本轮跳过止盈/止损镜像（仅限新被控首轮快照）")
	}
	if !schemeASkipTPSL {
		positions, err = at.trader.GetPositions()
		if err != nil {
			return fmt.Errorf("v2 刷新被控持仓(止盈止损前): %w", err)
		}
		foll = mapFollowerPositionQuantities(positions)
		for k, fq := range foll {
			if fq < qtyEps(1) {
				continue
			}
			sym, side, ok2 := splitPosKey(k)
			if !ok2 {
				continue
			}
			if baselineOnlyKeys[k] {
				record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
					"v2 启动基线: %s %s 属于启动前历史仓位，跳过止盈止损同步", sym, side))
				continue
			}
			ps := strings.ToUpper(side)
			sl, tp := mirrorResolveSLTPFromWire(&wire, sym, side)
			if sl <= 0 && tp <= 0 {
				continue
			}
			if sl > 0 {
				_ = at.trader.CancelStopLossOrders(sym)
				// WS API 优先，失败回退 REST
				var slErr error
				if ft != nil {
					_ = v2WSBucket.wait(marketCtx)
					slErr = ft.SetStopLossWebSocketAPI(marketCtx, sym, ps, fq, sl)
					if slErr != nil {
						logger.Infof("v2 止损 %s WS API 失败(回退REST): %v", sym, slErr)
						slErr = at.trader.SetStopLoss(sym, ps, fq, sl)
					}
				} else {
					slErr = at.trader.SetStopLoss(sym, ps, fq, sl)
				}
				if slErr != nil {
					msg := fmt.Sprintf("⚠ v2 止损设置失败 %s %s @%.6f: %v", sym, ps, sl, slErr)
					record.ExecutionLog = append(record.ExecutionLog, msg)
					syncErrs = append(syncErrs, msg)
				} else {
					record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("✓ v2 止损 %s %s qty=%.6f @%.6f", sym, ps, fq, sl))
				}
				time.Sleep(v2WSTPSLSleep)
			}
			if tp > 0 {
				_ = at.trader.CancelTakeProfitOrders(sym)
				// WS API 优先，失败回退 REST
				var tpErr error
				if ft != nil {
					_ = v2WSBucket.wait(marketCtx)
					tpErr = ft.SetTakeProfitWebSocketAPI(marketCtx, sym, ps, fq, tp)
					if tpErr != nil {
						logger.Infof("v2 止盈 %s WS API 失败(回退REST): %v", sym, tpErr)
						tpErr = at.trader.SetTakeProfit(sym, ps, fq, tp)
					}
				} else {
					tpErr = at.trader.SetTakeProfit(sym, ps, fq, tp)
				}
				if tpErr != nil {
					msg := fmt.Sprintf("⚠ v2 止盈设置失败 %s %s @%.6f: %v", sym, ps, tp, tpErr)
					record.ExecutionLog = append(record.ExecutionLog, msg)
					syncErrs = append(syncErrs, msg)
				} else {
					record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("✓ v2 止盈 %s %s qty=%.6f @%.6f", sym, ps, fq, tp))
				}
				time.Sleep(v2WSTPSLSleep)
			}
		}
	}

	// ---- Phase 7: 汇总结果 ----
	if mirrorMarketNeedRetry || mirrorCloseNeedRetry {
		return fmt.Errorf("mirror_transient: v2 镜像市价同步未完成（加仓/全平/部分平仓重试中）")
	}
	if len(syncErrs) > 0 {
		return fmt.Errorf("v2 订单同步存在未完成项：%s", strings.Join(syncErrs, "；"))
	}
	return nil
}
