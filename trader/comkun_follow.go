package trader

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	"nofx/kernel"
	"nofx/logger"
	"nofx/store"

	"gorm.io/gorm"
)

const (
	comkunFollowDisplayFeeMinUnit  = 489
	comkunFollowDisplayFeeMaxUnit  = 524
	comkunFollowDisplayFeeDivisor  = 10000.0
	comkunFollowDisplayFeeMaxUSDT  = float64(comkunFollowDisplayFeeMaxUnit) / comkunFollowDisplayFeeDivisor
	comkunAIStrategyAnalysisPrompt = "COMKUN_AI_STRATEGY_ANALYSIS"
	comkunAIStrategyNoticePrompt   = "COMKUN_AI_STRATEGY_NOTICE"
)

var errComkunPlatformBalanceInsufficient = errors.New("平台余额不足")
var errComkunMarketSubscriptionExpired = errors.New("策略市场订阅已到期")

// isComkunFollowMirrorTransientError 镜像同步失败是否宜于「下周期重试同一条广播」：
// 多为交易所限频、短暂网络问题；非此类仍按原逻辑静默标记 success，避免永久卡住主控已平仓等状态。
func isComkunFollowMirrorTransientError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "-1003") || strings.Contains(s, "too many requests") {
		return true
	}
	if strings.Contains(s, "code=-1003") {
		return true
	}
	if strings.Contains(s, "statuscode=429") || strings.Contains(s, "status code 429") {
		return true
	}
	if strings.Contains(s, "i/o timeout") || strings.Contains(s, "context deadline exceeded") {
		return true
	}
	if strings.Contains(s, "connection reset") || strings.Contains(s, "connection refused") {
		return true
	}
	if strings.Contains(s, "code=-2015") || strings.Contains(s, "-2015") ||
		strings.Contains(s, "invalid api-key") || strings.Contains(s, "invalid api key") ||
		strings.Contains(s, "permissions for action") {
		return true
	}
	// 镜像市价加仓未就绪 / 下单失败：允许下轮对同一条广播重试（避免已 MarkConsumptionSuccess 后永久不再对齐）
	if strings.Contains(s, "mirror_transient:") {
		return true
	}
	return false
}

func comkunFollowBalanceStopMessage(balance, required float64) string {
	if required <= 0 {
		required = store.ComkunFollowScanFeeMaxUSDTOrDefault()
	}
	return fmt.Sprintf("平台余额不足（当前平台余额 %.4f USDT，不足本轮 AI 分析所需 %.4f USDT）。请先充值平台余额；系统已暂停本 AI 策略，本轮不会执行任何操作。",
		balance, required)
}

func roundComkunFeeUSDT(v float64) float64 {
	if v <= 0 {
		return 0
	}
	return math.Round(v*1_000_000) / 1_000_000
}

func comkunDailyFeeTargetUSDT(userID, traderID, sourceID string, day time.Time) float64 {
	min, max := store.ComkunFollowDailyFeeRangeUSDTOrDefault()
	if max <= min {
		return min
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.Join([]string{
		strings.TrimSpace(userID),
		strings.TrimSpace(traderID),
		strings.TrimSpace(sourceID),
		day.UTC().Format("2006-01-02"),
	}, "|")))
	return min + (float64(h.Sum32()%3001)/3000.0)*(max-min)
}

func comkunFeeFromDailyTarget(target, expected float64) float64 {
	if target <= 0 || expected <= 0 {
		return 0
	}
	return roundComkunFeeUSDT(target / expected)
}

func comkunJitteredFeeFromDailyTarget(target, expected float64, entropy string) float64 {
	if target <= 0 || expected <= 0 {
		return 0
	}
	base := target / expected
	jitter := store.ComkunFollowScanFeeJitterPctOrDefault()
	if jitter <= 0 {
		return roundComkunFeeUSDT(base)
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(entropy))
	unit := float64(h.Sum32()%20001)/10000.0 - 1
	return roundComkunFeeUSDT(base * (1 + unit*jitter))
}

func randomComkunFollowDisplayFeeUSDT() float64 {
	unit := rand.Intn(comkunFollowDisplayFeeMaxUnit-comkunFollowDisplayFeeMinUnit+1) + comkunFollowDisplayFeeMinUnit
	return float64(unit) / comkunFollowDisplayFeeDivisor
}

func randomComkunFollowScanFeeUSDT() float64 {
	base := store.ComkunFollowScanFeeUSDTOrDefault()
	if base <= 0 {
		return 0
	}
	jitter := store.ComkunFollowScanFeeJitterPctOrDefault()
	if jitter <= 0 {
		return roundComkunFeeUSDT(base)
	}
	factor := 1 + (rand.Float64()*2-1)*jitter
	if factor < 0.01 {
		factor = 0.01
	}
	return roundComkunFeeUSDT(base * factor)
}

func randomComkunFollowBlankScanFeeUSDT() float64 {
	base := store.ComkunFollowBlankScanFeeUSDTOrDefault()
	if base <= 0 {
		return 0
	}
	jitter := store.ComkunFollowScanFeeJitterPctOrDefault()
	if jitter <= 0 {
		return roundComkunFeeUSDT(base)
	}
	factor := 1 + (rand.Float64()*2-1)*jitter
	if factor < 0.01 {
		factor = 0.01
	}
	return roundComkunFeeUSDT(base * factor)
}

// comkunAnalysisTextIsBillable distinguishes real master AI analysis from
// mirror-only exchange snapshots. Snapshot heartbeats must not drain follower balance
// or overwrite the last real master analysis shown to followers.
func comkunAnalysisTextIsBillable(analysis string) bool {
	analysis = strings.TrimSpace(analysis)
	if analysis == "" {
		return false
	}
	if strings.Contains(analysis, "交易所快照同步") && strings.Contains(analysis, "未等待 AI 长分析完成") {
		return false
	}
	if strings.Contains(analysis, "主控最新交易所快照已确认无持仓") && strings.Contains(analysis, "平仓同步广播") {
		return false
	}
	if strings.Contains(analysis, "未生成可截取的思维链正文") && strings.Contains(analysis, "交易所持仓与挂单快照") {
		return false
	}
	return true
}

func comkunBroadcastHasBillableAIAnalysis(br *store.ComkunMasterBroadcast) bool {
	if br == nil {
		return false
	}
	return comkunAnalysisTextIsBillable(br.AnalysisText)
}

func (at *AutoTrader) comkunFollowScanFeeForBroadcast(br *store.ComkunMasterBroadcast) float64 {
	sourceID := ""
	if at.config.StrategyConfig != nil {
		sourceID = store.ResolveComkunFollowSourceStrategyID(at.config.StrategyConfig)
	}
	now := time.Now()
	target := comkunDailyFeeTargetUSDT(at.userID, at.id, sourceID, now)
	broadcastID := uint64(0)
	if br != nil {
		broadcastID = br.ID
	}
	entropy := fmt.Sprintf("%s|%s|%s|%s|%d", at.userID, at.id, sourceID, now.UTC().Format("2006-01-02"), broadcastID)
	if !comkunBroadcastHasBillableAIAnalysis(br) {
		return comkunJitteredFeeFromDailyTarget(target, store.ComkunFollowBlankExpectedAnalysesPerDayOrDefault(), entropy)
	}
	return comkunJitteredFeeFromDailyTarget(target, store.ComkunFollowExpectedAnalysesPerDayOrDefault(), entropy)
}

const comkunFollowSourceIdleAfter = 30 * time.Second

func comkunFollowOfflineBillingInterval() time.Duration {
	expected := store.ComkunFollowBlankExpectedAnalysesPerDayOrDefault()
	interval := time.Duration(float64(24*time.Hour) / expected)
	if interval < time.Second {
		interval = time.Second
	}
	if comkunFollowFollowMasterPollInterval > interval {
		return comkunFollowFollowMasterPollInterval
	}
	return interval
}

func (at *AutoTrader) comkunFollowSourceNeedsScheduledBilling(br *store.ComkunMasterBroadcast, now time.Time) bool {
	if br == nil || br.CreatedAt.IsZero() {
		return !at.startTime.IsZero() && now.Sub(at.startTime) >= comkunFollowSourceIdleAfter
	}
	if now.Sub(br.CreatedAt) < comkunFollowSourceIdleAfter {
		return false
	}
	// 网页镜像绝不消费断流后的旧仓位；其他跟单源先执行尚未消费的新信号，再进入定时计费。
	return at.comkunFollowSourceIsBnScreenMirror() || at.comkunFollowBroadcastAlreadyConsumed(br.ID)
}

func (at *AutoTrader) comkunFollowOfflineChargeDue(now time.Time) bool {
	interval := comkunFollowOfflineBillingInterval()
	if at.lastComkunOfflineChargeAt.IsZero() {
		at.lastComkunOfflineChargeAt = now
		return false
	}
	if now.Sub(at.lastComkunOfflineChargeAt) < interval {
		return false
	}
	at.lastComkunOfflineChargeAt = now
	return true
}

func (at *AutoTrader) comkunFollowOfflineFee(sourceID string, now time.Time) float64 {
	interval := comkunFollowOfflineBillingInterval()
	expected := float64(24*time.Hour) / float64(interval)
	target := comkunDailyFeeTargetUSDT(at.userID, at.id, sourceID, now)
	slot := now.UnixNano() / int64(interval)
	entropy := fmt.Sprintf("offline|%s|%s|%s|%s|%d", at.userID, at.id, sourceID, now.UTC().Format("2006-01-02"), slot)
	return comkunJitteredFeeFromDailyTarget(target, expected, entropy)
}

func (at *AutoTrader) chargeComkunFollowScanUsage(feeUSDT float64) (balanceAfter float64, spendLedgerID uint64, err error) {
	if at.store == nil || at.userID == "" || feeUSDT <= 0 {
		return 0, 0, nil
	}
	err = at.store.Transaction(func(tx *gorm.DB) error {
		balanceBefore := 0.0
		if u, e := at.store.User().GetByID(at.userID); e == nil && u != nil {
			balanceBefore = u.BalanceUSDT
		}
		bal, e := at.store.User().AddBalanceDeltaAllowNegative(tx, at.userID, -feeUSDT)
		if e != nil {
			return e
		}
		balanceAfter = bal
		lid, e := at.store.Billing().AppendLedger(tx, at.userID, -feeUSDT, balanceAfter, "comkun_follow_scan", at.id)
		if e != nil {
			return e
		}
		spendLedgerID = lid
		_, e = at.store.AIPlatformUsage().CreateSuccessTx(
			tx,
			at.userID,
			at.id,
			"comkun-ai",
			"comkun-ai",
			0,
			feeUSDT,
			0,
			feeUSDT,
			balanceBefore,
			balanceAfter,
			fmt.Sprintf("wallet_ledger:%d", lid),
		)
		return e
	})
	return balanceAfter, spendLedgerID, err
}

func (at *AutoTrader) maybeChargeComkunFollowOffline(sourceID string, now time.Time) error {
	if !at.comkunFollowOfflineChargeDue(now) || at.store == nil || at.userID == "" {
		return nil
	}
	feeUSDT := at.comkunFollowOfflineFee(sourceID, now)
	if feeUSDT <= 0 {
		return nil
	}
	u, err := at.store.User().GetByID(at.userID)
	if err != nil {
		logger.Warnf("[%s] comkun 跟单空闲计费：读取平台余额失败: %v", at.name, err)
		return nil
	}
	if u.BalanceUSDT < -1e-9 {
		at.stopComkunFollowForInsufficientBalance(comkunFollowBalanceStopMessage(u.BalanceUSDT, feeUSDT))
		return nil
	}
	balanceAfter, ledgerID, err := at.chargeComkunFollowScanUsage(feeUSDT)
	if err != nil {
		logger.Warnf("[%s] comkun 跟单空闲计费失败: %v", at.name, err)
		return nil
	}
	if ledgerID > 0 {
		store.DispatchAgentRebateSpendIfEligible(at.userID, feeUSDT, ledgerID, "comkun_follow_scan")
	}
	if balanceAfter < -1e-9 {
		at.stopComkunFollowForInsufficientBalance(comkunFollowBalanceStopMessage(balanceAfter, feeUSDT))
	}
	return nil
}

// comkunFollowScanFeeWaived kept for old call sites; market follow subscriptions are no longer package-waived.
func (at *AutoTrader) comkunFollowScanFeeWaived() bool {
	return false
}

func (at *AutoTrader) comkunFollowSourceSubscriptionExpired(sourceID string) bool {
	return false
}

func scaleComkunDecisions(decisions []kernel.Decision, equityRatio float64) []kernel.Decision {
	out := make([]kernel.Decision, len(decisions))
	copy(out, decisions)
	if equityRatio <= 0 || len(out) == 0 {
		return out
	}
	for i := range out {
		if out[i].PositionSizeUSD > 0 {
			out[i].PositionSizeUSD = out[i].PositionSizeUSD * equityRatio
		}
	}
	return out
}

func comkunSymbolsFromDecisionJSON(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" || raw == "null" {
		return nil
	}
	var rows []struct {
		Symbol string `json:"symbol"`
	}
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil
	}
	out := make([]string, 0, len(rows))
	seen := map[string]bool{}
	for _, r := range rows {
		s := strings.ToUpper(strings.TrimSpace(r.Symbol))
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func comkunDecisionJSONHasRows(raw string) bool {
	return len(comkunSymbolsFromDecisionJSON(raw)) > 0
}

func comkunSetRecordCandidates(record *store.DecisionRecord, candidates []string) bool {
	if record == nil || len(candidates) == 0 {
		return false
	}
	out := make([]string, 0, len(candidates))
	seen := map[string]bool{}
	for _, c := range candidates {
		s := strings.ToUpper(strings.TrimSpace(c))
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	if len(out) == 0 {
		return false
	}
	record.CandidateCoins = out
	return true
}

func fillComkunRecordCandidateCoins(record *store.DecisionRecord, wire *comkunMasterStateWire, br *store.ComkunMasterBroadcast) {
	if record == nil {
		return
	}
	if wire != nil && comkunSetRecordCandidates(record, wire.CandidateCoins) {
		return
	}
	if br != nil && comkunSetRecordCandidates(record, comkunSymbolsFromDecisionJSON(br.DecisionJSON)) {
		return
	}
	if wire == nil {
		return
	}
	var fallback []string
	seen := map[string]bool{}
	add := func(sym string) {
		s := strings.ToUpper(strings.TrimSpace(sym))
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		fallback = append(fallback, s)
	}
	for _, mp := range wire.Positions {
		add(mp.Symbol)
	}
	for _, o := range wire.PendingOrders {
		add(o.Symbol)
	}
	_ = comkunSetRecordCandidates(record, fallback)
}

func comkunPublicDecisionJSON(br *store.ComkunMasterBroadcast, fallback []kernel.Decision) string {
	if br != nil && comkunDecisionJSONHasRows(br.DecisionJSON) {
		return strings.TrimSpace(br.DecisionJSON)
	}
	if len(fallback) == 0 {
		return `[]`
	}
	if b, err := json.MarshalIndent(fallback, "", "  "); err == nil {
		return string(b)
	}
	return `[]`
}

func prepareComkunPublicAIRecord(record *store.DecisionRecord, ctx *kernel.Context, sourceID string, br *store.ComkunMasterBroadcast, initialBalance float64) {
	if record == nil {
		return
	}
	record.Success = true
	record.SystemPrompt = comkunAIStrategyAnalysisPrompt
	record.InputPrompt = ""
	record.CoTTrace = sanitizeComkunFollowDisplayAnalysis(br.AnalysisText)
	record.ExecutionLog = []string{"AI 分析完成，策略状态已更新。"}
	record.ErrorMessage = ""
	record.AccountState = accountSnapshotFromCtx(ctx, initialBalance)
	fillComkunRecordMeta(record, ctx, sourceID, br.ID, br)
}

func (at *AutoTrader) comkunFollowBroadcastAlreadyConsumed(broadcastID uint64) bool {
	if broadcastID == 0 {
		return false
	}
	at.lastComkunConsumedBroadcastMu.Lock()
	defer at.lastComkunConsumedBroadcastMu.Unlock()
	return at.lastComkunConsumedBroadcastID == broadcastID
}

func (at *AutoTrader) markComkunBroadcastConsumed(broadcastID uint64) {
	if broadcastID == 0 {
		return
	}
	at.lastComkunConsumedBroadcastMu.Lock()
	at.lastComkunConsumedBroadcastID = broadcastID
	at.lastComkunConsumedBroadcastMu.Unlock()
}

const comkunFollowMaxBroadcastAge = 5 * time.Minute
const okxScreenMirrorMaxBroadcastAge = 2 * time.Minute

func (at *AutoTrader) comkunFollowSourceIsBnScreenMirror() bool {
	if at.config.StrategyConfig == nil {
		return false
	}
	sid := strings.TrimSpace(store.ResolveComkunFollowSourceStrategyID(at.config.StrategyConfig))
	return store.IsBnScreenMirrorMasterStrategyID(sid)
}

func (at *AutoTrader) comkunFollowSourceIsOkxScreenMirror() bool {
	if at.config.StrategyConfig == nil {
		return false
	}
	sid := strings.TrimSpace(store.ResolveComkunFollowSourceStrategyID(at.config.StrategyConfig))
	return store.IsOkxScreenMirrorMasterStrategyID(sid)
}

func (at *AutoTrader) comkunFollowSourceIsHZExternal() bool {
	if at.config.StrategyConfig == nil || !strings.EqualFold(at.exchange, "hz") {
		return false
	}
	sid := strings.TrimSpace(store.ResolveComkunFollowSourceStrategyID(at.config.StrategyConfig))
	return strings.HasPrefix(sid, store.HZMasterSourcePrefix)
}

func (at *AutoTrader) comkunFollowBroadcastIsFresh(br *store.ComkunMasterBroadcast) bool {
	if br == nil || br.ID == 0 {
		return false
	}
	// HZ events must always reach reconciliation: stale risk-increasing deltas
	// are rejected there, while reductions and full closes never expire.
	if at.comkunFollowSourceIsHZExternal() {
		return true
	}
	// 网页镜像：DOM 广播可能因浏览器标签休眠间歇停顿；最新快照仍代表当前网页仓位。
	// 不因进程重启 startTime 或 5 分钟 TTL 拒绝消费，否则冷启动/标签挂起后无法全仓对齐。
	if at.comkunFollowSourceIsOkxScreenMirror() {
		return time.Since(br.CreatedAt) <= okxScreenMirrorMaxBroadcastAge
	}
	if at.comkunFollowSourceIsBnScreenMirror() || at.comkunFollowSourceIsMT4Gold() {
		const webMirrorMaxBroadcastAge = 24 * time.Hour
		return time.Since(br.CreatedAt) <= webMirrorMaxBroadcastAge
	}
	if time.Since(br.CreatedAt) > comkunFollowMaxBroadcastAge {
		return false
	}
	if !at.startTime.IsZero() && br.CreatedAt.Before(at.startTime.Add(-2*time.Second)) {
		return false
	}
	return true
}

// comkunFollowAdvanceWatermarkOnStaleBroadcast 网页镜像拒绝过期快照时勿抬高水位线，以便浏览器恢复后仍能消费最新快照。
func (at *AutoTrader) comkunFollowAdvanceWatermarkOnStaleBroadcast() bool {
	return !at.comkunFollowSourceIsBnScreenMirror() && !at.comkunFollowSourceIsMT4Gold()
}

func (at *AutoTrader) comkunDisplayInterval() time.Duration {
	d := at.config.ScanInterval
	if d <= 0 {
		return 3 * time.Minute
	}
	return d
}

func (at *AutoTrader) comkunDisplayDue() bool {
	if at.lastComkunDisplayAt.IsZero() {
		return true
	}
	return time.Since(at.lastComkunDisplayAt) >= at.comkunDisplayInterval()
}

func fingerprintComkunDisplayDecisions(decisions []kernel.Decision) string {
	if len(decisions) == 0 {
		return ""
	}
	lines := make([]string, 0, len(decisions))
	for _, d := range decisions {
		id := strings.TrimSpace(d.OrderID)
		if id == "" {
			id = fmt.Sprintf("%s|%s|%.8f|%.8f|%.8f|%d",
				strings.ToUpper(strings.TrimSpace(d.Symbol)),
				strings.ToLower(strings.TrimSpace(d.Action)),
				d.Price,
				d.StopLoss,
				d.TakeProfit,
				d.Leverage,
			)
		}
		lines = append(lines, id)
	}
	sort.Strings(lines)
	return strings.Join(lines, ";")
}

func comkunDisplayOrderIDForPending(o kernel.PendingOrder, action string) string {
	if id := strings.TrimSpace(o.OrderID); id != "" {
		return "LMT-" + id
	}
	px := kernel.PendingLimitDisplayPrice(o)
	return fmt.Sprintf("LMT-%s-%s-%s-%s-%.8f",
		strings.ToUpper(strings.TrimSpace(o.Symbol)),
		strings.ToLower(strings.TrimSpace(action)),
		strings.ToUpper(strings.TrimSpace(o.Side)),
		strings.ToUpper(strings.TrimSpace(o.PositionSide)),
		px,
	)
}

func comkunDisplayOrderIDForTPSL(symbol, side string, sl, tp float64) string {
	return fmt.Sprintf("TPSL-%s-%s-%.8f-%.8f",
		strings.ToUpper(strings.TrimSpace(symbol)),
		strings.ToLower(strings.TrimSpace(side)),
		sl,
		tp,
	)
}

func sanitizeComkunFollowDisplayAnalysis(analysis string) string {
	analysis = strings.TrimSpace(analysis)
	if analysis == "" {
		return ""
	}
	return strings.TrimSpace(stripComkunPositionManagementAssessment(analysis))
}

func compactComkunAnalysisHeading(line string) string {
	t := strings.TrimSpace(line)
	t = strings.TrimLeft(t, "#* \t")
	replacer := strings.NewReplacer(
		" ", "", "\t", "", ".", "", "。", "", "、", "", ":", "", "：", "",
		"-", "", "—", "", "_", "", "[", "", "]", "", "【", "", "】", "",
		"（", "", "）", "", "(", "", ")", "",
	)
	return replacer.Replace(t)
}

func isComkunPositionManagementHeading(line string) bool {
	c := compactComkunAnalysisHeading(line)
	return strings.Contains(c, "5仓位管理评估") || strings.Contains(c, "五仓位管理评估")
}

func isComkunAnalysisSectionHeading(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" {
		return false
	}
	c := compactComkunAnalysisHeading(t)
	if c == "" {
		return false
	}
	if strings.HasPrefix(t, "【") && strings.Contains(t, "】") {
		return true
	}
	r := []rune(c)[0]
	return (r >= '1' && r <= '9') || r == '一' || r == '二' || r == '三' || r == '四' || r == '五' || r == '六'
}

func stripComkunPositionManagementAssessment(analysis string) string {
	lines := strings.Split(strings.TrimSpace(analysis), "\n")
	out := make([]string, 0, len(lines))
	skipping := false
	for _, line := range lines {
		if isComkunPositionManagementHeading(line) {
			skipping = true
			continue
		}
		if skipping {
			if isComkunAnalysisSectionHeading(line) {
				skipping = false
			} else {
				continue
			}
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func (at *AutoTrader) queueComkunDisplayAnalysis(analysis string) {
	rawAnalysis := strings.TrimSpace(analysis)
	analysis = strings.TrimSpace(analysis)
	if analysis == "" {
		return
	}
	billable := comkunAnalysisTextIsBillable(rawAnalysis)
	if !billable {
		at.pendingComkunDisplayMu.Lock()
		hasRealPending := comkunAnalysisTextIsBillable(at.pendingComkunDisplayCoT)
		at.pendingComkunDisplayMu.Unlock()
		if hasRealPending {
			return
		}
		// Snapshot heartbeats should not become the visible AI analysis when a
		// real master analysis can be loaded from broadcast history.
		return
	}
	analysis = sanitizeComkunFollowDisplayAnalysis(analysis)
	if analysis == "" {
		return
	}
	at.pendingComkunDisplayMu.Lock()
	if !billable && comkunAnalysisTextIsBillable(at.pendingComkunDisplayCoT) {
		at.pendingComkunDisplayMu.Unlock()
		return
	}
	at.pendingComkunDisplayCoT = analysis
	at.pendingComkunDisplayMu.Unlock()
}

func (at *AutoTrader) resolveComkunFollowDisplayAnalysis(sourceID string, br *store.ComkunMasterBroadcast, preferred string) string {
	preferred = sanitizeComkunFollowDisplayAnalysis(preferred)
	if comkunAnalysisTextIsBillable(preferred) {
		return preferred
	}
	if br != nil && comkunAnalysisTextIsBillable(br.AnalysisText) {
		if text := sanitizeComkunFollowDisplayAnalysis(br.AnalysisText); text != "" {
			return text
		}
	}
	if at.store != nil && strings.TrimSpace(sourceID) != "" {
		if latest, err := at.store.ComkunFollow().GetLatestAnalysisBroadcast(sourceID); err == nil && latest != nil {
			if text := sanitizeComkunFollowDisplayAnalysis(latest.AnalysisText); text != "" {
				return text
			}
		}
	}
	return preferred
}

func (at *AutoTrader) queueComkunDisplayDecisions(decisions []kernel.Decision) {
	fp := fingerprintComkunDisplayDecisions(decisions)
	if fp == "" {
		return
	}
	at.pendingComkunDisplayMu.Lock()
	defer at.pendingComkunDisplayMu.Unlock()
	if at.shownComkunDisplayFP == nil {
		at.shownComkunDisplayFP = make(map[string]bool)
	}
	unique := make([]kernel.Decision, 0, len(decisions))
	seen := make(map[string]bool)
	for _, d := range decisions {
		id := fingerprintComkunDisplayDecisions([]kernel.Decision{d})
		if id == "" || seen[id] || at.shownComkunDisplayFP[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, d)
	}
	fp = fingerprintComkunDisplayDecisions(unique)
	if fp == "" {
		at.pendingComkunDisplayDecisions = nil
		at.pendingComkunDisplayFP = ""
		return
	}
	// 展示窗口未到时，只保留最新一轮源快照中的仍有效订单，避免已撤旧单在下一次展示时冒出来。
	at.pendingComkunDisplayDecisions = unique
	at.pendingComkunDisplayFP = fp
}

func buildComkunTPSLDisplayDecisionsFromWire(wire comkunMasterStateWire, masterEq, followerEq float64, masterLev, followLev int) []kernel.Decision {
	var out []kernel.Decision
	for _, p := range wire.Positions {
		sym := strings.TrimSpace(p.Symbol)
		side := strings.ToLower(strings.TrimSpace(p.Side))
		if sym == "" || p.Quantity <= 0 || (side != "long" && side != "short") {
			continue
		}
		sl, tp := mirrorResolveSLTPFromWire(&wire, sym, side)
		if sl <= 0 && tp <= 0 {
			continue
		}
		qty := mirrorScaledPositionQty(p, masterEq, followerEq, masterLev, followLev)
		px := p.MarkPrice
		if px <= 0 {
			px = p.EntryPrice
		}
		notional := qty * px
		if notional <= 0 {
			notional = p.Quantity * px
		}
		action := "open_long"
		if side == "short" {
			action = "open_short"
		}
		out = append(out, kernel.Decision{
			Symbol:          sym,
			Action:          action,
			Leverage:        followLev,
			PositionSizeUSD: notional,
			Price:           px,
			StopLoss:        sl,
			TakeProfit:      tp,
			OrderID:         comkunDisplayOrderIDForTPSL(sym, side, sl, tp),
		})
	}
	return out
}

func (at *AutoTrader) popPendingComkunDisplayDecisions() ([]kernel.Decision, string) {
	at.pendingComkunDisplayMu.Lock()
	defer at.pendingComkunDisplayMu.Unlock()
	decisions := at.pendingComkunDisplayDecisions
	fp := at.pendingComkunDisplayFP
	at.pendingComkunDisplayDecisions = nil
	at.pendingComkunDisplayFP = ""
	if fp != "" {
		if at.shownComkunDisplayFP == nil {
			at.shownComkunDisplayFP = make(map[string]bool)
		}
		for _, part := range strings.Split(fp, "\n") {
			if strings.TrimSpace(part) != "" {
				at.shownComkunDisplayFP[part] = true
			}
		}
	}
	return decisions, fp
}

func (at *AutoTrader) popPendingComkunDisplayCoT() string {
	at.pendingComkunDisplayMu.Lock()
	defer at.pendingComkunDisplayMu.Unlock()
	cot := at.pendingComkunDisplayCoT
	at.pendingComkunDisplayCoT = ""
	return cot
}

func (at *AutoTrader) chargeComkunDisplayFee() (float64, float64, bool, error) {
	if at.comkunFollowScanFeeWaived() {
		if at.store == nil || at.userID == "" {
			return 0, 0, false, nil
		}
		u, err := at.store.User().GetByID(at.userID)
		if err != nil {
			return 0, 0, false, err
		}
		return u.BalanceUSDT, 0, true, nil
	}
	if at.config.StrategyConfig != nil && store.IsComkunMarketFollowStrategy(at.config.StrategyConfig) {
		if at.store == nil || at.userID == "" {
			return 0, 0, false, nil
		}
		u, err := at.store.User().GetByID(at.userID)
		if err != nil {
			return 0, 0, false, err
		}
		return u.BalanceUSDT, 0, true, nil
	}
	if at.store == nil || at.userID == "" {
		return 0, 0, false, nil
	}
	feeUSDT := randomComkunFollowDisplayFeeUSDT()
	var balanceAfter float64
	var spendLedgerID uint64
	err := at.store.Transaction(func(tx *gorm.DB) error {
		bal, ok, err := at.store.User().AddBalanceDelta(tx, at.userID, -feeUSDT)
		if err != nil {
			return err
		}
		if !ok {
			balanceAfter = bal
			return fmt.Errorf("%w", errComkunPlatformBalanceInsufficient)
		}
		balanceAfter = bal
		lid, e := at.store.Billing().AppendLedger(tx, at.userID, -feeUSDT, balanceAfter, "comkun_display_cycle", at.id)
		if e != nil {
			return e
		}
		spendLedgerID = lid
		return nil
	})
	if err != nil {
		logger.Warnf("[%s] COMKUN display fee skipped: %v", at.name, err)
		return balanceAfter, feeUSDT, false, err
	}
	if feeUSDT > 1e-9 && spendLedgerID > 0 {
		store.DispatchAgentRebateSpendIfEligible(at.userID, feeUSDT, spendLedgerID, "comkun_display_cycle")
	}
	return balanceAfter, feeUSDT, true, nil
}

func (at *AutoTrader) stopComkunFollowForInsufficientBalance(message string) {
	logger.Warnf("[%s] %s", at.name, message)
	at.signalStop()
	if at.store != nil && at.userID != "" && at.id != "" {
		if err := at.store.Trader().UpdateStatus(at.userID, at.id, false); err != nil {
			logger.Warnf("[%s] comkun 跟单余额不足后更新停止状态失败: %v", at.name, err)
		}
	}
}

func (at *AutoTrader) saveComkunBalanceStopDecision(ctx *kernel.Context, sourceID string, br *store.ComkunMasterBroadcast, record *store.DecisionRecord, balance, requiredFee float64) error {
	if record == nil {
		record = &store.DecisionRecord{ExecutionLog: []string{}}
	}
	msg := comkunFollowBalanceStopMessage(balance, requiredFee)
	at.stopComkunFollowForInsufficientBalance(msg)
	record.Success = false
	record.SystemPrompt = comkunAIStrategyNoticePrompt
	record.InputPrompt = fmt.Sprintf(
		"platform_balance=%.6f required_fee_usdt=%.6f action=pause_ai_strategy",
		balance, requiredFee,
	)
	record.CoTTrace = msg
	record.DecisionJSON = `[]`
	record.ErrorMessage = msg
	record.ExecutionLog = append(record.ExecutionLog, msg)
	record.AccountState = accountSnapshotFromCtx(ctx, at.initialBalance)
	if br != nil {
		fillComkunRecordMeta(record, ctx, sourceID, br.ID, br)
	} else {
		fillComkunRecordMeta(record, ctx, sourceID, 0, nil)
	}
	at.lastComkunDisplayAt = time.Now()
	return at.saveDecision(record)
}

func comkunMarketSubscriptionExpiredMessage() string {
	return "策略市场包月/包周已到期，本 AI 策略已按订阅周期自动停止。请在策略市场续订该策略后，再点击启动；若不再续订，可改用其他策略。"
}

func (at *AutoTrader) saveComkunSubscriptionExpiredDecision(ctx *kernel.Context, sourceID string, br *store.ComkunMasterBroadcast, record *store.DecisionRecord) error {
	if record == nil {
		record = &store.DecisionRecord{ExecutionLog: []string{}}
	}
	msg := comkunMarketSubscriptionExpiredMessage()
	at.stopComkunFollowForInsufficientBalance(msg)
	record.Success = false
	record.SystemPrompt = comkunAIStrategyNoticePrompt
	record.InputPrompt = fmt.Sprintf(
		"action=pause_ai_strategy reason=market_subscription_expired",
	)
	record.CoTTrace = msg
	record.ErrorMessage = msg
	record.DecisionJSON = `[]`
	record.ExecutionLog = append(record.ExecutionLog, msg)
	record.AccountState = accountSnapshotFromCtx(ctx, at.initialBalance)
	if br != nil {
		fillComkunRecordMeta(record, ctx, sourceID, br.ID, br)
	} else {
		fillComkunRecordMeta(record, ctx, sourceID, 0, nil)
	}
	at.lastComkunDisplayAt = time.Now()
	return at.saveDecision(record)
}

func (at *AutoTrader) requireComkunFollowPlatformBalance(ctx *kernel.Context, sourceID string, br *store.ComkunMasterBroadcast, record *store.DecisionRecord, requiredFee float64) (bool, error) {
	if at.store == nil || at.userID == "" {
		return true, nil
	}
	if requiredFee <= 0 {
		return true, nil
	}
	u, err := at.store.User().GetByID(at.userID)
	if err != nil {
		record.Success = false
		record.ErrorMessage = fmt.Sprintf("读取平台余额失败: %v", err)
		record.AccountState = accountSnapshotFromCtx(ctx, at.initialBalance)
		fillComkunRecordMeta(record, ctx, sourceID, br.ID, br)
		return false, at.saveComkunFollowDecision(record, br)
	}
	if u.BalanceUSDT < -1e-9 {
		return false, at.saveComkunBalanceStopDecision(ctx, sourceID, br, record, u.BalanceUSDT, requiredFee)
	}
	return true, nil
}

func (at *AutoTrader) maybeEmitComkunScheduledDisplay(ctx *kernel.Context, sourceID string, br *store.ComkunMasterBroadcast) error {
	if !at.comkunDisplayDue() {
		return nil
	}
	balanceAfter, feeUSDT, ok, chargeErr := at.chargeComkunDisplayFee()
	if !ok {
		if errors.Is(chargeErr, errComkunMarketSubscriptionExpired) {
			rec := &store.DecisionRecord{ExecutionLog: []string{}}
			return at.saveComkunSubscriptionExpiredDecision(ctx, sourceID, br, rec)
		}
		if errors.Is(chargeErr, errComkunPlatformBalanceInsufficient) {
			return at.saveComkunBalanceStopDecision(ctx, sourceID, br, nil, balanceAfter, feeUSDT)
		}
		at.lastComkunDisplayAt = time.Now()
		return nil
	}
	decisions, fp := at.popPendingComkunDisplayDecisions()
	cot := at.popPendingComkunDisplayCoT()
	cot = at.resolveComkunFollowDisplayAnalysis(sourceID, br, cot)
	decisionJSON := `[]`
	if len(decisions) > 0 {
		if b, err := json.MarshalIndent(decisions, "", "  "); err == nil {
			decisionJSON = string(b)
		}
	}
	record := &store.DecisionRecord{
		Success:      true,
		SystemPrompt: comkunAIStrategyAnalysisPrompt,
		InputPrompt:  "",
		CoTTrace:     cot,
		DecisionJSON: decisionJSON,
		AccountState: accountSnapshotFromCtx(ctx, at.initialBalance),
	}
	_ = fp
	if br != nil {
		fillComkunRecordMeta(record, ctx, sourceID, br.ID, br)
	} else {
		fillComkunRecordMeta(record, ctx, sourceID, 0, nil)
	}
	for _, d := range decisions {
		s := strings.TrimSpace(strings.ToUpper(d.Symbol))
		if s != "" {
			record.CandidateCoins = append(record.CandidateCoins, s)
		}
	}
	at.callCount++
	cotIsRealAnalysis := comkunAnalysisTextIsBillable(cot)
	if feeUSDT < 1e-9 && at.comkunFollowScanFeeWaived() {
		record.ExecutionLog = []string{"AI 分析完成，策略状态已更新。"}
	} else if feeUSDT < 1e-9 && br != nil && comkunBroadcastHasBillableAIAnalysis(br) {
		record.ExecutionLog = []string{"AI 分析完成，策略状态已更新。"}
	} else if feeUSDT < 1e-9 && cotIsRealAnalysis {
		record.ExecutionLog = []string{"AI 分析完成，策略状态已更新。"}
	} else if feeUSDT < 1e-9 && br != nil {
		record.ExecutionLog = []string{"AI 策略状态已更新。"}
	} else if feeUSDT < 1e-9 {
		record.ExecutionLog = []string{"AI 策略状态已更新。"}
	} else {
		record.ExecutionLog = []string{fmt.Sprintf("本轮 AI 分析已扣除 %.4f USDT 平台余额", feeUSDT)}
	}
	at.lastComkunDisplayAt = time.Now()
	return at.saveDecision(record)
}

// seedComkunFollowBroadcastWatermarkOnStart 新建/首次启动被控时把当前已有广播全部视为历史。
// 进程/容器重启若已恢复过消费水位线，不能重新把当前主控仓位设为基线，否则会错过本应同步的启动后新仓。
// 同时记录主控当时已有仓位为启动基线；后续只跟主控启动之后新增的差额，不追历史仓。
func (at *AutoTrader) seedComkunFollowBroadcastWatermarkOnStart() {
	if at.store == nil || at.config.StrategyConfig == nil || !store.IsComkunMarketFollowStrategy(at.config.StrategyConfig) {
		return
	}
	at.lastComkunConsumedBroadcastMu.Lock()
	if at.lastComkunConsumedBroadcastID > 0 {
		at.lastComkunConsumedBroadcastMu.Unlock()
		return
	}
	at.lastComkunConsumedBroadcastMu.Unlock()

	sourceID := strings.TrimSpace(store.ResolveComkunFollowSourceStrategyID(at.config.StrategyConfig))
	if sourceID == "" {
		return
	}
	br, err := at.store.ComkunFollow().GetLatestBroadcast(sourceID)
	if err != nil || br == nil || br.ID == 0 {
		return
	}
	if at.comkunFollowSourceIsOkxScreenMirror() {
		logger.Infof("📡 [%s] OKX 网页镜像新被控：不回放历史广播，等待新鲜快照后再对齐当前持仓 latest_broadcast_id=%d",
			at.name, br.ID)
		return
	}
	at.seedMirrorBaselineFromMasterBroadcast(br)
	at.lastComkunConsumedBroadcastMu.Lock()
	prev := at.lastComkunConsumedBroadcastID
	if br.ID > at.lastComkunConsumedBroadcastID {
		at.lastComkunConsumedBroadcastID = br.ID
		logger.Infof("📡 [%s] comkun 被控启动水位线：忽略历史广播 broadcast_id=%d created_at=%s，只等待主控启动后的新广播",
			at.name, br.ID, br.CreatedAt.UTC().Format(time.RFC3339))
		// 方案A：**所有** comkun_market_follow 被控（不限 SOL/究极模板）；仅从「从未消费过广播」的新被控启用。
		if mirrorSchemeAEnabled && prev == 0 {
			at.mirrorSchemeAPostSeedCyclesRemaining = 1
			logger.Infof("📡 [%s] comkun 方案A（全策略跟单）：下一条新广播的首轮镜像将跳过止盈/止损同步（仍同步限价与平仓安全逻辑）；可用 COMKUN_MIRROR_SCHEME_A=1 开启",
				at.name)
		}
	}
	at.lastComkunConsumedBroadcastMu.Unlock()
	if prev == 0 {
		if err := at.store.ComkunFollow().MarkConsumptionStartupBaseline(at.id, br.ID); err != nil {
			logger.Warnf("[%s] comkun 启动基线持久化失败 broadcast_id=%d: %v", at.name, br.ID, err)
		}
	}
}

// restoreComkunFollowConsumedBroadcastID 被控 Run 启动时从 DB 恢复已消费的主控广播 id。
// 进程重启后内存会清零，若不恢复会把「最新一条」旧广播再跑一遍，出现主控没扫、被控却多一条思维链的现象。
func (at *AutoTrader) restoreComkunFollowConsumedBroadcastID() {
	if at.store == nil || at.config.StrategyConfig == nil || !store.IsComkunMarketFollowStrategy(at.config.StrategyConfig) {
		return
	}
	var id uint64
	if at.comkunFollowSourceIsBnScreenMirror() {
		// 展示扣费决策也会写入 broadcast_id，不能当作镜像已消费水位线。
		id = at.store.ComkunFollow().GetMaxSuccessfulConsumptionBroadcastID(at.id)
	} else {
		id = at.store.Decision().GetLatestSuccessfulComkunFollowBroadcastID(at.id)
		if c := at.store.ComkunFollow().GetMaxSuccessfulConsumptionBroadcastID(at.id); c > id {
			id = c
		}
	}
	if id == 0 {
		return
	}
	if at.store.ComkunFollow().IsStartupBaselineConsumption(at.id, id) {
		if br, err := at.store.ComkunFollow().GetBroadcastByID(id); err == nil && br != nil {
			at.seedMirrorBaselineFromMasterBroadcast(br)
		}
	} else if baseline, ok := decodeMirrorStartupBaselineCheckpoint(
		at.store.ComkunFollow().GetConsumptionCheckpoint(at.id, id)); ok {
		at.mirrorSeedBaselineQty = baseline
		logger.Infof("[%s] v2 startup baseline restored from checkpoint: %d legs", at.name, len(baseline))
	}
	at.lastComkunConsumedBroadcastMu.Lock()
	defer at.lastComkunConsumedBroadcastMu.Unlock()
	if at.lastComkunConsumedBroadcastID >= id {
		return
	}
	at.lastComkunConsumedBroadcastID = id
	logger.Infof("📡 [%s] 已从历史决策恢复 comkun 已消费广播 broadcast_id=%d（主控未发新广播时本轮将静默跳过）", at.name, id)
}

func (at *AutoTrader) markComkunConsumptionSuccess(broadcastID uint64) error {
	checkpoint := ""
	if at.shouldPersistMirrorStartupBaseline() {
		checkpoint = encodeMirrorStartupBaselineCheckpoint(at.mirrorSeedBaselineQty)
	}
	return at.store.ComkunFollow().MarkConsumptionSuccessWithCheckpoint(at.id, broadcastID, checkpoint)
}

func (at *AutoTrader) shouldPersistMirrorStartupBaseline() bool {
	return at.comkunFollowSourceIsMT4Gold() || at.comkunFollowSourceIsOkxScreenMirror() || at.comkunFollowSourceIsHZExternal()
}

// saveComkunFollowDecision 落库后若本轮成功则记录已消费的主控广播 id，避免同一条广播重复扣费/重复打交易所
func (at *AutoTrader) saveComkunFollowDecision(record *store.DecisionRecord, br *store.ComkunMasterBroadcast) error {
	err := at.saveDecision(record)
	if err == nil && record != nil && record.Success && br != nil {
		at.markComkunBroadcastConsumed(br.ID)
	}
	return err
}

// runComkunFollowCycle 合规跟单：消费主账户广播的 analysis + master_state_json，按平台余额计费后镜像同步。
func (at *AutoTrader) runComkunFollowCycle(ctx *kernel.Context, record *store.DecisionRecord) error {
	cfg := at.config.StrategyConfig
	var br *store.ComkunMasterBroadcast

	if at.store == nil {
		record.Success = false
		record.SystemPrompt = comkunAIStrategyNoticePrompt
		record.ErrorMessage = "服务端存储暂不可用，本轮 AI 策略暂停。"
		record.AccountState = accountSnapshotFromCtx(ctx, at.initialBalance)
		return at.saveComkunFollowDecision(record, br)
	}

	sourceID := store.ResolveComkunFollowSourceStrategyID(cfg)
	if sourceID == "" {
		record.Success = false
		record.SystemPrompt = comkunAIStrategyNoticePrompt
		record.ErrorMessage = "AI 策略配置未完成，请检查策略来源设置。"
		record.AccountState = accountSnapshotFromCtx(ctx, at.initialBalance)
		return at.saveComkunFollowDecision(record, br)
	}

	var err error
	br, err = at.store.ComkunFollow().GetLatestBroadcast(sourceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		record.Success = false
		record.SystemPrompt = comkunAIStrategyNoticePrompt
		record.ErrorMessage = fmt.Sprintf("读取 AI 分析失败: %v", err)
		record.AccountState = accountSnapshotFromCtx(ctx, at.initialBalance)
		return at.saveComkunFollowDecision(record, br)
	}
	if br == nil {
		return nil
	}
	if !at.comkunFollowBroadcastIsFresh(br) {
		if at.comkunFollowAdvanceWatermarkOnStaleBroadcast() {
			at.markComkunBroadcastConsumed(br.ID)
		}
		logger.Infof("[%s] comkun 跟单：拒绝消费历史/过期广播 broadcast_id=%d created_at=%s（未扣费、未同步交易所）",
			at.name, br.ID, br.CreatedAt.UTC().Format(time.RFC3339))
		return nil
	}
	if at.comkunFollowSourceSubscriptionExpired(sourceID) {
		return at.saveComkunSubscriptionExpiredDecision(ctx, sourceID, br, record)
	}
	mt4Follow := store.IsMT4GoldMasterStrategyID(sourceID)
	hzExternal := at.comkunFollowSourceIsHZExternal()
	if mt4Follow {
		if err := at.store.ComkunFollow().EnsureMT4TicketMappings(at.id, sourceID, br.ID, at.initialBalance); err != nil {
			record.Success = false
			record.SystemPrompt = comkunAIStrategyNoticePrompt
			record.ErrorMessage = fmt.Sprintf("MT4 跟单状态更新失败: %v", err)
			record.AccountState = accountSnapshotFromCtx(ctx, at.initialBalance)
			fillComkunRecordMeta(record, ctx, sourceID, br.ID, br)
			return at.saveComkunFollowDecision(record, br)
		}
	}

	scanFeeUSDT := at.comkunFollowScanFeeForBroadcast(br)
	if !hzExternal {
		balanceOK, balanceErr := at.requireComkunFollowPlatformBalance(ctx, sourceID, br, record, scanFeeUSDT)
		if !balanceOK {
			return balanceErr
		}
	}

	var acquired bool
	var lockErr error
	if mt4Follow {
		acquired, lockErr = at.store.ComkunFollow().TryAcquireConsumptionLockWithCooldown(at.id, br.ID, 500*time.Millisecond)
	} else {
		acquired, lockErr = at.store.ComkunFollow().TryAcquireConsumptionLock(at.id, br.ID)
	}
	if lockErr != nil {
		record.Success = false
		record.SystemPrompt = comkunAIStrategyNoticePrompt
		record.ErrorMessage = fmt.Sprintf("AI 策略状态更新失败: %v", lockErr)
		record.AccountState = accountSnapshotFromCtx(ctx, at.initialBalance)
		fillComkunRecordMeta(record, ctx, sourceID, br.ID, br)
		return at.saveComkunFollowDecision(record, br)
	}
	if !acquired {
		if st, _ := at.store.ComkunFollow().GetConsumptionStatus(at.id, br.ID); st == "success" {
			at.markComkunBroadcastConsumed(br.ID)
		}
		return nil
	}

	platformBalanceAfter := 0.0
	feeReserved := false
	closeOnly := false
	billingIntentKey := ""
	var scanSpendLedgerID uint64
	if hzExternal && scanFeeUSDT > 0 {
		var stateWire comkunMasterStateWire
		if err := json.Unmarshal([]byte(strings.TrimSpace(br.MasterStateJSON)), &stateWire); err != nil || strings.TrimSpace(stateWire.SourceEventID) == "" {
			_ = at.store.ComkunFollow().MarkConsumptionFailed(at.id, br.ID, "HZ billing event metadata missing")
			return nil
		}
		billingIntentKey, billingClientID := hzMirrorIntentIDs(stateWire.SourceEventID, at.userID, at.id, "__account__", "none", "billing")
		requestHash := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%.12g", scanFeeUSDT))))
		if _, err := at.store.MirrorExecutionIntent().Ensure(store.MirrorExecutionIntentInput{
			IntentKey: billingIntentKey, MasterEventID: stateWire.SourceEventID, BroadcastID: br.ID,
			UserID: at.userID, TraderID: at.id, ExchangeID: at.exchangeID,
			Instrument: "__account__", PositionSide: "none", Action: "billing",
			ClientOrderID: billingClientID, RequestSHA256: requestHash,
		}); err != nil {
			_ = at.store.ComkunFollow().MarkConsumptionFailed(at.id, br.ID, "HZ billing intent failed")
			return nil
		}
		reservation, err := at.store.MirrorExecutionIntent().ReserveBilling(billingIntentKey, scanFeeUSDT)
		if err != nil {
			_ = at.store.ComkunFollow().MarkConsumptionFailed(at.id, br.ID, "HZ billing reservation failed")
			return nil
		}
		platformBalanceAfter = reservation.BalanceAfter
		if reservation.Insufficient {
			closeOnly = true
		} else {
			feeReserved = reservation.Reserved
			scanSpendLedgerID = reservation.WalletLedgerID
		}
	} else if scanFeeUSDT > 0 {
		var chargeErr error
		platformBalanceAfter, scanSpendLedgerID, chargeErr = at.chargeComkunFollowScanUsage(scanFeeUSDT)
		if chargeErr != nil {
			_ = at.store.ComkunFollow().MarkConsumptionFailed(at.id, br.ID, chargeErr.Error())
			if mt4Follow {
				_ = at.store.ComkunFollow().MarkMT4TicketMappingStatus(at.id, br.ID, store.MT4MappingFailed, chargeErr.Error())
			}
			record.Success = false
			record.SystemPrompt = comkunAIStrategyNoticePrompt
			record.ErrorMessage = fmt.Sprintf("%v；本轮 AI 策略暂停。", chargeErr)
			record.AccountState = accountSnapshotFromCtx(ctx, at.initialBalance)
			fillComkunRecordMeta(record, ctx, sourceID, br.ID, br)
			return at.saveComkunFollowDecision(record, br)
		}
		feeReserved = true
		if scanSpendLedgerID > 0 {
			store.DispatchAgentRebateSpendIfEligible(at.userID, scanFeeUSDT, scanSpendLedgerID, "comkun_follow_scan")
		}
		if platformBalanceAfter < -1e-9 {
			_ = at.store.ComkunFollow().MarkConsumptionFailed(at.id, br.ID, "platform balance debt after scan fee")
			if mt4Follow {
				_ = at.store.ComkunFollow().MarkMT4TicketMappingStatus(at.id, br.ID, store.MT4MappingFailed, "platform balance debt after scan fee")
			}
			return at.saveComkunBalanceStopDecision(ctx, sourceID, br, record, platformBalanceAfter, scanFeeUSDT)
		}
	} else if u, err := at.store.User().GetByID(at.userID); err == nil && u != nil {
		platformBalanceAfter = u.BalanceUSDT
	}
	refundFee := func(reason string) {
		if !feeReserved || scanFeeUSDT <= 0 {
			return
		}
		if hzExternal {
			if err := at.store.MirrorExecutionIntent().RefundBilling(billingIntentKey, reason); err != nil {
				logger.Warnf("[%s] HZ billing refund failed broadcast_id=%d: %v", at.name, br.ID, err)
			}
			return
		}
		_ = at.store.Transaction(func(tx *gorm.DB) error {
			bal, _, err := at.store.User().AddBalanceDelta(tx, at.userID, scanFeeUSDT)
			if err != nil {
				return err
			}
			platformBalanceAfter = bal
			_, e := at.store.Billing().AppendLedger(tx, at.userID, scanFeeUSDT, platformBalanceAfter, "comkun_follow_scan_refund:"+reason, at.id)
			return e
		})
	}

	ratio := 1.0
	if br.MasterAccountEquity > 1e-9 {
		ratio = ctx.Account.TotalEquity / br.MasterAccountEquity
	}

	stateTrim := strings.TrimSpace(br.MasterStateJSON)
	// 合规跟单：广播里含主控交易所快照则一律按快照镜像（仓位/限价/止盈止损）。
	// 安全模式也必须走镜像（只可在 reconcile 内限制加仓），禁止改走下方「执行 AI 决策 JSON」否则主控未实盘平仓时被控会误平。
	if stateTrim != "" {
		var stateWire comkunMasterStateWire
		_ = json.Unmarshal([]byte(stateTrim), &stateWire)
		ml := resolveMirrorMasterLevFromWire(&stateWire)
		fl := store.ComkunMirrorFollowerMarginLeverageOrDefault(at.config.StrategyConfig)
		fillMirrorRecordSymbols := func() {
			fillComkunRecordCandidateCoins(record, &stateWire, br)
		}
		if at.safeMode {
			record.ExecutionLog = append(record.ExecutionLog,
				"🛡️ 安全模式：仍按主控交易所快照镜像同步；不执行广播 decision_json（避免与主控实盘不一致）")
		}
		at.isRunningMutex.RLock()
		runMirror := at.isRunning
		at.isRunningMutex.RUnlock()
		if !runMirror {
			refundFee("stopped")
			_ = at.store.ComkunFollow().MarkConsumptionFailed(at.id, br.ID, "trader stopped before mirror execution")
			if mt4Follow {
				_ = at.store.ComkunFollow().MarkMT4TicketMappingStatus(at.id, br.ID, store.MT4MappingFailed, "trader stopped before mirror execution")
			}
			record.Success = false
			record.SystemPrompt = comkunAIStrategyNoticePrompt
			record.InputPrompt = ""
			record.ErrorMessage = "网络异常波动，未从 AI 返回结果，请等待下一周期。"
			record.ExecutionLog = []string{"AI 策略状态暂未更新。"}
			record.AccountState = accountSnapshotFromCtx(ctx, at.initialBalance)
			fillComkunRecordMeta(record, ctx, sourceID, br.ID, br)
			return at.saveComkunFollowDecision(record, br)
		}
		schemeASkipTPSL := mirrorSchemeAEnabled && at.mirrorSchemeAPostSeedCyclesRemaining > 0
		if at.comkunFollowSourceIsBnScreenMirror() {
			schemeASkipTPSL = true
		}
		err := at.reconcileComkunFollowMasterStateV2(ctx, br, record, schemeASkipTPSL, closeOnly || stateWire.PollingReconcile)
		if err != nil {
			if mt4Follow && errors.Is(err, errMT4BroadcastSuperseded) {
				_ = at.markComkunConsumptionSuccess(br.ID)
				_ = at.store.ComkunFollow().MarkMT4TicketMappingStatus(at.id, br.ID, store.MT4MappingSuperseded, err.Error())
				at.markComkunBroadcastConsumed(br.ID)
				return nil
			}
			refundFee("silent_mirror")
			if isComkunFollowMirrorTransientError(err) {
				// 限频/短暂网络：标记 failed（非 success），下轮 TryAcquire 可再次执行；不推进内存水位，避免丢失本条广播。
				msg := err.Error()
				if len(msg) > 800 {
					msg = msg[:800]
				}
				_ = at.store.ComkunFollow().MarkConsumptionFailed(at.id, br.ID, "transient_retry:"+msg)
				if mt4Follow {
					_ = at.store.ComkunFollow().MarkMT4TicketMappingStatus(at.id, br.ID, store.MT4MappingFailed, msg)
				}
				logger.Warnf("[%s] comkun 镜像同步瞬时失败，下轮将重试广播 broadcast_id=%d: %v", at.name, br.ID, err)
				return nil
			}
			// 非瞬时错误：仍标记广播已消费避免永久重试同一条；客户侧不落展示卡，内部细节只进服务日志。
			em := strings.TrimSpace(err.Error())
			if len(em) > 1000 {
				em = em[:1000]
			}
			_ = at.markComkunConsumptionSuccess(br.ID)
			if mt4Follow {
				_ = at.store.ComkunFollow().MarkMT4TicketMappingStatus(at.id, br.ID, store.MT4MappingFailed, em)
			}
			at.markComkunBroadcastConsumed(br.ID)
			logger.Warnf("[%s] comkun 内部同步失败，已静默跳过客户展示 broadcast_id=%d: %s", at.name, br.ID, em)
			return nil
		}
		if schemeASkipTPSL && at.mirrorSchemeAPostSeedCyclesRemaining > 0 {
			at.mirrorSchemeAPostSeedCyclesRemaining--
			logger.Infof("[%s] comkun 方案A 首轮镜像已完成，后续广播将同步止盈止损（若仍有剩余计数异常请检查配置）", at.name)
		}
		if hzExternal && feeReserved {
			if err := at.store.MirrorExecutionIntent().FinalizeBilling(billingIntentKey); err != nil {
				_ = at.store.ComkunFollow().MarkConsumptionFailed(at.id, br.ID, "HZ billing finalization retry")
				return nil
			}
			if scanSpendLedgerID > 0 {
				store.DispatchAgentRebateSpendIfEligible(at.userID, scanFeeUSDT, scanSpendLedgerID, "comkun_follow_scan")
			}
		}
		_ = platformBalanceAfter
		_ = ratio
		record.Success = true
		fillMirrorRecordSymbols()
		mirrorDisplay := buildMirrorDisplayDecisionsFromWire(stateWire, br.MasterAccountEquity, ctx.Account.TotalEquity, ml, fl, ctx, stateWire.VolumeOIBrief)
		if len(mirrorDisplay) == 0 {
			mirrorDisplay = buildComkunTPSLDisplayDecisionsFromWire(stateWire, br.MasterAccountEquity, ctx.Account.TotalEquity, ml, fl)
		}
		limitFP := fingerprintMirrorLimitOrders(stateWire)
		at.lastComkunMirrorLimitFPMu.Lock()
		prevFP := at.lastComkunMirrorLimitFP
		sameLimits := limitFP != "" && limitFP == prevFP
		if !sameLimits {
			at.lastComkunMirrorLimitFP = limitFP
		}
		at.lastComkunMirrorLimitFPMu.Unlock()
		_ = sameLimits
		fillComkunRecordMeta(record, ctx, sourceID, br.ID, br)
		record.AccountState = accountSnapshotFromCtx(ctx, at.initialBalance)

		if comkunBroadcastHasBillableAIAnalysis(br) {
			prepareComkunPublicAIRecord(record, ctx, sourceID, br, at.initialBalance)
			fillMirrorRecordSymbols()
			record.DecisionJSON = comkunPublicDecisionJSON(br, mirrorDisplay)
			if saveErr := at.saveDecision(record); saveErr != nil {
				logger.Warnf("[%s] comkun 主控 AI 分析同步成功但落库失败 broadcast_id=%d: %v（仍将标记广播已消费）",
					at.name, br.ID, saveErr)
			}
		} else {
			logger.Infof("[%s] comkun v2 静默同步快照 broadcast_id=%d（非主控 AI 分析，不生成跟单展示卡片）",
				at.name, br.ID)
		}
		_ = at.markComkunConsumptionSuccess(br.ID)
		if mt4Follow {
			_ = at.store.ComkunFollow().MarkMT4TicketMappingStatus(at.id, br.ID, store.MT4MappingApplied, "")
		}
		at.markComkunBroadcastConsumed(br.ID)
		return nil
	}

	raw := strings.TrimSpace(br.DecisionJSON)
	if raw == "" || raw == "[]" {
		if comkunBroadcastHasBillableAIAnalysis(br) {
			prepareComkunPublicAIRecord(record, ctx, sourceID, br, at.initialBalance)
			record.DecisionJSON = `[]`
			if saveErr := at.saveDecision(record); saveErr != nil {
				logger.Warnf("[%s] comkun 主控 AI 分析落库失败 broadcast_id=%d: %v（仍将标记广播已消费）", at.name, br.ID, saveErr)
			}
		}
		_ = at.markComkunConsumptionSuccess(br.ID)
		at.markComkunBroadcastConsumed(br.ID)
		return nil
	}

	// 合规跟单已统一为「仅认主控广播里的交易所快照 master_state_json」；不再执行 decision_json。
	// 否则主控仅广播 AI 的平仓/开仓字面条、但主控自己未实盘时，被控会误操作。
	if comkunBroadcastHasBillableAIAnalysis(br) {
		prepareComkunPublicAIRecord(record, ctx, sourceID, br, at.initialBalance)
		record.DecisionJSON = comkunPublicDecisionJSON(br, nil)
		fillComkunRecordCandidateCoins(record, nil, br)
		_ = ratio
		_ = scanFeeUSDT
		_ = platformBalanceAfter
		if saveErr := at.saveDecision(record); saveErr != nil {
			logger.Warnf("[%s] comkun 主控 AI 分析落库失败 broadcast_id=%d: %v（仍将标记广播已消费）", at.name, br.ID, saveErr)
		}
	}
	_ = at.markComkunConsumptionSuccess(br.ID)
	at.markComkunBroadcastConsumed(br.ID)
	return nil
}

func accountSnapshotFromCtx(ctx *kernel.Context, initial float64) store.AccountSnapshot {
	if ctx == nil {
		return store.AccountSnapshot{InitialBalance: initial}
	}
	return store.AccountSnapshot{
		TotalBalance:          ctx.Account.TotalEquity,
		AvailableBalance:      ctx.Account.AvailableBalance,
		TotalUnrealizedProfit: ctx.Account.UnrealizedPnL,
		PositionCount:         ctx.Account.PositionCount,
		MarginUsedPct:         ctx.Account.MarginUsedPct,
		InitialBalance:        initial,
	}
}

func fillComkunRecordMeta(record *store.DecisionRecord, ctx *kernel.Context, sourceID string, broadcastID uint64, br *store.ComkunMasterBroadcast) {
	if record == nil {
		return
	}
	// RawResponse 是客户页面可能拿到的轻量元信息；避免暴露内部同步/广播/跟单链路。
	_ = sourceID
	meta := map[string]interface{}{
		"kind": "ai_strategy_analysis",
	}
	if broadcastID > 0 {
		meta["analysis_id"] = broadcastID
	}
	if br != nil {
		meta["analysis_created_at"] = br.CreatedAt.UTC().Format(time.RFC3339)
	}
	if ctx != nil {
		meta["account_equity"] = ctx.Account.TotalEquity
	}
	b, _ := json.Marshal(meta)
	record.RawResponse = string(b)
}
