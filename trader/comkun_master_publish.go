package trader

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"time"

	"nofx/kernel"
	"nofx/logger"
	"nofx/store"
)

// comkunPositionSignature 用于检测主控账户持仓是否在两次拉取之间发生变化（人工在交易所操作等）
func comkunPositionSignature(positions []kernel.PositionInfo) string {
	if len(positions) == 0 {
		return ""
	}
	lines := make([]string, 0, len(positions))
	for _, p := range positions {
		sym := strings.ToUpper(strings.TrimSpace(p.Symbol))
		side := strings.ToLower(strings.TrimSpace(p.Side))
		lines = append(lines, fmt.Sprintf("%s|%s|qty:%.6f|lev:%d|ep:%.6f", sym, side, p.Quantity, p.Leverage, p.EntryPrice))
	}
	sort.Strings(lines)
	return strings.Join(lines, ";")
}

func comkunMasterStateJSONIsEmpty(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	var wire comkunMasterStateWire
	if err := json.Unmarshal([]byte(raw), &wire); err != nil {
		return false
	}
	return len(wire.Positions) == 0 && len(wire.PendingOrders) == 0
}

// comkunMasterWirePositionsEmptyJSON 主广播 JSON 里「持仓列表」是否已全平（不要求挂单为空）。
// 市价平仓后常见：positions 已空但 pending_orders 仍有 TP/SL/计划单 → 若用 IsEmpty 做心跳条件会永远不插新 broadcast，被控无法跟平。
func comkunMasterWirePositionsEmptyJSON(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	var wire comkunMasterStateWire
	if err := json.Unmarshal([]byte(raw), &wire); err != nil {
		return false
	}
	for _, p := range wire.Positions {
		if p.Quantity > 1e-9 {
			return false
		}
	}
	return true
}

func (at *AutoTrader) shouldSkipUnconfirmedEmptyComkunSnapshot(sourceID, currentStateJSON string) bool {
	if !comkunMasterStateJSONIsEmpty(currentStateJSON) {
		if at.comkunEmptySnapshotPending != nil {
			delete(at.comkunEmptySnapshotPending, sourceID)
		}
		return false
	}
	// 原先「首轮空仓需下一扫描再确认」会推迟一整轮主循环/广播；跟单平仓要求尽快对齐，不再推迟。
	_ = sourceID
	return false
}

// comkunMirrorSemanticFingerprintFromMasterStateJSON 提取「镜像跟单有意义」的持仓+挂单+杠杆元数据指纹（忽略标记价/浮动盈亏等噪声）。
// 用于主控广播判重：避免因 JSON 字节完全一致才跳过，或反之仅因展示字段抖动误判为变化。
// ComkunMirrorSemanticFingerprintFromMasterStateWire 与 JSON 版一致：用于主广播写入 mirror_semantic_fp、被控打日志对比，避免将浮盈/标记价噪声误判为「持仓结构变化」。
func ComkunMirrorSemanticFingerprintFromMasterStateWire(wire *comkunMasterStateWire) string {
	if wire == nil {
		return ""
	}
	parts := make([]string, 0, len(wire.Positions)+len(wire.PendingOrders)+1)
	for _, p := range wire.Positions {
		sym := strings.ToUpper(strings.TrimSpace(p.Symbol))
		side := strings.ToLower(strings.TrimSpace(p.Side))
		parts = append(parts, fmt.Sprintf("P|%s|%s|%.6f|%d", sym, side, p.Quantity, p.Leverage))
	}
	for _, o := range wire.PendingOrders {
		sym := strings.ToUpper(strings.TrimSpace(o.Symbol))
		side := strings.ToLower(strings.TrimSpace(o.Side))
		oid := strings.TrimSpace(o.OrderID)
		typ := strings.ToLower(strings.TrimSpace(o.Type))
		parts = append(parts, fmt.Sprintf("O|%s|%s|%s|%s|%.8f|%.8f|%.6f", sym, side, typ, oid, o.Price, o.StopPrice, o.Quantity))
	}
	if wire.MirrorMargin != nil {
		parts = append(parts, fmt.Sprintf("M|lev:%d", wire.MirrorMargin.MasterMarginLeverage))
		if wire.MirrorMargin.MasterMarginUsed > 0 {
			parts = append(parts, fmt.Sprintf("M|used:%.8f", wire.MirrorMargin.MasterMarginUsed))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ";")
}

func comkunMirrorSemanticFingerprintFromMasterStateJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var wire comkunMasterStateWire
	if err := json.Unmarshal([]byte(raw), &wire); err != nil {
		h := fnv.New64a()
		_, _ = h.Write([]byte(raw))
		return fmt.Sprintf("raw:%016x", h.Sum64())
	}
	fp := ComkunMirrorSemanticFingerprintFromMasterStateWire(&wire)
	if fp != "" {
		return fp
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(raw))
	return fmt.Sprintf("raw:%016x", h.Sum64())
}

func (at *AutoTrader) maybePublishComkunMasterFlatCloseBroadcast(ctx *kernel.Context) {
	if at.store == nil || ctx == nil || at.config.StrategyConfig == nil {
		return
	}
	cfg := at.config.StrategyConfig
	if !cfg.ComkunFollowListingTemplate || store.IsComkunMarketFollowStrategy(cfg) {
		return
	}
	sourceID := strings.TrimSpace(at.config.StrategyID)
	if sourceID == "" {
		return
	}
	if len(ctx.Positions) > 0 {
		return
	}
	prev, err := at.store.ComkunFollow().GetLatestBroadcast(sourceID)
	if err != nil || prev == nil || comkunMasterStateJSONIsEmpty(prev.MasterStateJSON) {
		return
	}
	stateJSON := comkunMasterStateJSONFlatFromCtx(ctx, cfg)
	if strings.TrimSpace(stateJSON) == "" {
		return
	}
	eq := ctx.Account.TotalEquity
	analysis := "主控最新交易所快照已确认无持仓；本轮属于平仓同步广播。跟单端应撤销相关残留挂单，并平掉不再受主控持仓保护的仓位。"
	row, err := at.store.ComkunFollow().InsertBroadcast(sourceID, eq, analysis, "[]", stateJSON)
	if err != nil {
		logger.Warnf("comkun master flat close broadcast: insert failed: %v", err)
		return
	}
	logger.Infof("📡 comkun master flat close broadcast published id=%d source=%s equity=%.4f prev_id=%d",
		row.ID, sourceID, eq, prev.ID)
}

// maybePublishComkunMasterBroadcast 主控实盘：策略为「跟单开关上架模板」且未开启 comkun_market_follow 时，
// 在每轮 AI 决策（及可选的实盘执行）完成后，将本轮决策 JSON + 思维链 + 主账户权益写入 comkun_master_broadcasts，
// 供所有 comkun_market_source_strategy_id 指向本策略 ID 的跟单用户消费。
// 无决策但有思维链时也会广播，便于「只分析不下单」模式实时推送行情解读。
func (at *AutoTrader) maybePublishComkunMasterBroadcast(ctx *kernel.Context, decisions []kernel.Decision, record *store.DecisionRecord) {
	if at.store == nil || ctx == nil || at.config.StrategyConfig == nil {
		return
	}
	cfg := at.config.StrategyConfig
	if !cfg.ComkunFollowListingTemplate {
		return
	}
	// 跟单侧（消费广播）打开 comkun_market_follow，不应再当主控发广播
	if store.IsComkunMarketFollowStrategy(cfg) {
		return
	}
	sourceID := strings.TrimSpace(at.config.StrategyID)
	if sourceID == "" {
		return
	}
	analysis := ""
	if record != nil {
		analysis = kernel.CoTForComkunMasterBroadcast(record.CoTTrace)
	}
	eq := ctx.Account.TotalEquity
	if record == nil && len(decisions) == 0 {
		stateJSON := comkunMasterStateJSONFromCtxBasic(ctx, cfg)
		if strings.TrimSpace(stateJSON) == "" {
			logger.Warnf("comkun master broadcast: skip because master_state_json is empty (主控广播必须带交易所快照)")
			return
		}
		prev, perr := at.store.ComkunFollow().GetLatestBroadcast(sourceID)
		dup := perr == nil && prev != nil &&
			comkunMirrorSemanticFingerprintFromMasterStateJSON(prev.MasterStateJSON) == comkunMirrorSemanticFingerprintFromMasterStateJSON(stateJSON) &&
			(at.startTime.IsZero() || !prev.CreatedAt.Before(at.startTime))
		// 持仓已平、指纹仍与上条相同（常见：挂单/TP 未变）：仍须周期性新 broadcast_id，否则被控无法重试平仓。
		flatHeartbeat := dup && comkunMasterWirePositionsEmptyJSON(stateJSON) && time.Since(prev.CreatedAt) >= 5*time.Second
		if dup && !flatHeartbeat {
			logger.Infof("comkun master broadcast: skip duplicate unchanged snapshot source=%s prev_id=%d", sourceID, prev.ID)
			return
		}
		if flatHeartbeat {
			logger.Infof("comkun master broadcast: 空仓快照心跳（持仓已平、距上条 ≥5s、同指纹），仍发布新 broadcast 供被控重试对齐 source=%s prev_id=%d", sourceID, prev.ID)
		}
		row, err := at.store.ComkunFollow().InsertBroadcast(sourceID, eq, "（本轮为交易所快照同步，未等待 AI 长分析完成。）", "[]", stateJSON)
		if err != nil {
			logger.Warnf("comkun master broadcast: insert failed: %v", err)
			return
		}
		logger.Infof("📡 comkun master snapshot broadcast published id=%d source=%s equity=%.4f state_bytes=%d",
			row.ID, sourceID, eq, len(stateJSON))
		return
	}

	stateJSON := comkunMasterStateJSONFromCtx(ctx, cfg, at.strategyEngine)
	if strings.TrimSpace(stateJSON) == "" {
		logger.Warnf("comkun master broadcast: skip because master_state_json is empty (主控广播必须带交易所快照)")
		return
	}
	if at.shouldSkipUnconfirmedEmptyComkunSnapshot(sourceID, stateJSON) {
		logger.Warnf("comkun master broadcast: skip first empty snapshot for source=%s; require next scan confirmation before broadcasting flat state", sourceID)
		return
	}
	if prev2, perr2 := at.store.ComkunFollow().GetLatestBroadcast(sourceID); perr2 == nil && prev2 != nil &&
		comkunMirrorSemanticFingerprintFromMasterStateJSON(prev2.MasterStateJSON) == comkunMirrorSemanticFingerprintFromMasterStateJSON(stateJSON) &&
		(at.startTime.IsZero() || !prev2.CreatedAt.Before(at.startTime)) &&
		time.Since(prev2.CreatedAt) < 2*time.Minute {
		flatHB := comkunMasterWirePositionsEmptyJSON(stateJSON) && time.Since(prev2.CreatedAt) >= 5*time.Second
		if !flatHB {
			logger.Infof("comkun master broadcast: skip duplicate fresh snapshot source=%s prev_id=%d", sourceID, prev2.ID)
			return
		}
		logger.Infof("comkun master broadcast: 空仓快照心跳（AI 路径 2min 判重窗口内，持仓已平、距上条 ≥5s），仍发布 source=%s prev_id=%d", sourceID, prev2.ID)
	}
	// 镜像跟单依赖每轮新广播 id；仅有快照时也插入一条，避免被控因「无新 id」长期静默
	if len(decisions) == 0 && strings.TrimSpace(analysis) == "" {
		analysis = "（本轮主控未生成可截取的思维链正文；以下为交易所持仓与挂单快照，供被控镜像同步。）"
	}
	raw := "[]"
	rawDecisionCount := 0
	// 主控上架模板现在只广播交易所快照；AI 的开平仓动作不作为可执行 decision_json 下发。
	if !store.ListingTemplateMasterSkipsExchangeExecution(cfg) && len(decisions) > 0 {
		b, err := json.Marshal(decisions)
		if err != nil {
			logger.Warnf("comkun master broadcast: marshal decisions: %v", err)
			return
		}
		raw = string(b)
		rawDecisionCount = len(decisions)
	}
	const maxAnalysis = 200_000
	if len(analysis) > maxAnalysis {
		analysis = analysis[:maxAnalysis] + "\n…(truncated)"
	}
	row, err := at.store.ComkunFollow().InsertBroadcast(sourceID, eq, analysis, raw, stateJSON)
	if err != nil {
		logger.Warnf("comkun master broadcast: insert failed: %v", err)
		return
	}
	logger.Infof("📡 comkun master broadcast published id=%d source=%s decisions=%d equity=%.4f state_bytes=%d",
		row.ID, sourceID, rawDecisionCount, eq, len(stateJSON))
}

// publishComkunManualPositionRationale 主控模板「仅分析」模式下，若本轮前后持仓快照不一致，则再发一条广播：
// 由 AI 用自然语言解读可能的人工下单逻辑（订阅方仍须自行决策，本系统不代下单）。
func (at *AutoTrader) publishComkunManualPositionRationale(ctxBefore, ctxAfter *kernel.Context) {
	if at.store == nil || ctxBefore == nil || ctxAfter == nil || at.config.StrategyConfig == nil {
		return
	}
	cfg := at.config.StrategyConfig
	if !store.ListingTemplateMasterSkipsExchangeExecution(cfg) || !cfg.ComkunFollowListingTemplate {
		return
	}
	if store.IsComkunMarketFollowStrategy(cfg) {
		return
	}
	sourceID := strings.TrimSpace(at.config.StrategyID)
	if sourceID == "" {
		return
	}
	beforeJSON, _ := json.MarshalIndent(ctxBefore.Positions, "", "  ")
	afterJSON, _ := json.MarshalIndent(ctxAfter.Positions, "", "  ")
	userPrompt := fmt.Sprintf(
		"【本轮扫描开始时账户持仓 JSON】\n%s\n\n【本轮扫描结束时账户持仓 JSON】\n%s\n\n请根据两次快照的差异，用简体中文写一段给「订阅该主控策略信号的用户」的说明。",
		string(beforeJSON), string(afterJSON),
	)
	system := `你是跟单产品里的「交易逻辑讲解员」。主控账户在交易所的下单/平仓由人工完成，系统只观察到持仓变化。
要求：用简体中文；结构清晰（可含：可能意图、风险与盈亏考量、关键价位与逻辑链条）；只根据数据中可见的仓位与价格推断，不要编造未出现的成交；不要使用「必须跟单」「建议立即开仓」等命令式话术；结尾提醒：订阅者需自行判断与承担风险。
最后必须单独输出一节，标题严格为「## 跟单端简报」：用不超过 500 字概括给订阅者的行情要点与风险；不要写镜像同步、执行日志、JSON。`
	text, err := at.mcpClient.CallWithMessages(system, userPrompt)
	if err != nil {
		logger.Warnf("comkun manual position rationale: AI failed: %v", err)
		return
	}
	if at.store != nil {
		if chargeErr := at.store.AICharge().Record(at.id, at.aiModel, at.config.AIModel); chargeErr != nil {
			logger.Warnf("comkun manual position rationale: AI charge record failed: %v", chargeErr)
		}
	}
	rawText := "[主控实盘仓位变化 · AI解读]\n" + strings.TrimSpace(text)
	analysis := kernel.ExtractFollowerBroadcastCoT(rawText)
	if analysis == "" {
		analysis = kernel.FallbackFollowerBriefFromCoT(rawText)
	}
	const maxAnalysis = 200_000
	if len(analysis) > maxAnalysis {
		analysis = analysis[:maxAnalysis] + "\n…(truncated)"
	}
	eq := ctxAfter.Account.TotalEquity
	stateJSON := comkunMasterStateJSONFromCtx(ctxAfter, cfg, at.strategyEngine)
	if strings.TrimSpace(stateJSON) == "" {
		logger.Warnf("comkun manual position rationale: skip broadcast because master_state_json is empty (主控广播必须带交易所快照)")
		return
	}
	if at.shouldSkipUnconfirmedEmptyComkunSnapshot(sourceID, stateJSON) {
		logger.Warnf("comkun manual position rationale: skip first empty snapshot for source=%s; require next scan confirmation", sourceID)
		return
	}
	row, err := at.store.ComkunFollow().InsertBroadcast(sourceID, eq, analysis, "[]", stateJSON)
	if err != nil {
		logger.Warnf("comkun manual position rationale: insert broadcast failed: %v", err)
		return
	}
	logger.Infof("📡 comkun manual position rationale broadcast id=%d source=%s equity=%.4f", row.ID, sourceID, eq)
}
