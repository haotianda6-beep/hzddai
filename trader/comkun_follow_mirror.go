package trader

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"nofx/kernel"
	"nofx/logger"
	"nofx/market"
	"nofx/store"
	"nofx/trader/binance"
)

// 镜像跟单共享工具（执行入口见 comkun_follow_mirror_v2.go）。
const (
	// 网页镜像主控广播里连续几轮「已无该腿」即允许被控市价全平。
	mirrorSafetyMasterFlatConfirms = 2
)

func mirrorFlatConfirmRequired(webMirror bool) int {
	if webMirror {
		return mirrorSafetyMasterFlatConfirms
	}
	return 1
}

// mirrorSafetyMarketCooldown 见 comkun_env_timing.go（默认 5s，可用 COMKUN_MIRROR_MARKET_COOLDOWN_SEC 覆盖）

// ComkunMirrorMarginMeta 写入主广播 master_state_json：跟单端按「保证金/主控权益」比例镜像时用主控假定杠杆。
type ComkunMirrorMarginMeta struct {
	MasterMarginLeverage int     `json:"master_margin_leverage,omitempty"`
	MasterMarginUsed     float64 `json:"master_margin_used,omitempty"`
}

// ComkunSymbolVolumeOI 主控广播里按币种的「成交量 + OI」摘要，跟单端扫描区/决策卡展示用。
type ComkunSymbolVolumeOI struct {
	Vol4hCurrent  float64 `json:"vol_4h_current,omitempty"`
	Vol4hAvg      float64 `json:"vol_4h_avg,omitempty"`
	OILatest      float64 `json:"oi_latest,omitempty"`
	OIAvg         float64 `json:"oi_avg,omitempty"`
	PriceChg1hPct float64 `json:"price_chg_1h_pct,omitempty"`
	OITopRank     int     `json:"oi_top_rank,omitempty"`
	OITopDeltaPct float64 `json:"oi_top_delta_pct,omitempty"`
}

func (b *ComkunSymbolVolumeOI) hasDisplaySignal() bool {
	if b == nil {
		return false
	}
	return b.Vol4hCurrent > 1e-12 || b.Vol4hAvg > 1e-12 ||
		b.OILatest > 1e-9 || b.OIAvg > 1e-9 ||
		math.Abs(b.PriceChg1hPct) > 1e-9 ||
		b.OITopRank > 0
}

func appendComkunMasterVolumeOIParts(b *ComkunSymbolVolumeOI) []string {
	if b == nil || !b.hasDisplaySignal() {
		return nil
	}
	var parts []string
	if b.Vol4hCurrent > 1e-12 || b.Vol4hAvg > 1e-12 {
		parts = append(parts, fmt.Sprintf("4h 成交量 当前 %.4f / 均量 %.4f", b.Vol4hCurrent, b.Vol4hAvg))
	}
	if b.OILatest > 1e-9 || b.OIAvg > 1e-9 {
		parts = append(parts, fmt.Sprintf("合约持仓 OI 最新 %.2f / 均值 %.2f", b.OILatest, b.OIAvg))
	}
	if math.Abs(b.PriceChg1hPct) > 1e-9 {
		parts = append(parts, fmt.Sprintf("1h 涨跌 %+0.2f%%", b.PriceChg1hPct))
	}
	if b.OITopRank > 0 {
		parts = append(parts, fmt.Sprintf("OI 增幅榜 #%d（1h 持仓 %+0.2f%%）", b.OITopRank, b.OITopDeltaPct))
	}
	return parts
}

// resolveMirrorVolumeOIBriefForSymbol 合并主控广播 volume_oi_brief、跟单端上下文行情与本机 market 拉取，供研判与执行日志使用。
func resolveMirrorVolumeOIBriefForSymbol(symbol string, masterBrief map[string]*ComkunSymbolVolumeOI, ctx *kernel.Context) *ComkunSymbolVolumeOI {
	norm := market.Normalize(symbol)
	if masterBrief != nil {
		if b := masterBrief[norm]; b != nil && b.hasDisplaySignal() {
			return b
		}
	}
	out := &ComkunSymbolVolumeOI{}
	symU := strings.ToUpper(strings.TrimSpace(symbol))
	if ctx != nil && ctx.MarketDataMap != nil {
		if d := ctx.MarketDataMap[symU]; d != nil {
			if d.LongerTermContext != nil {
				out.Vol4hCurrent = d.LongerTermContext.CurrentVolume
				out.Vol4hAvg = d.LongerTermContext.AverageVolume
			}
			if d.OpenInterest != nil {
				out.OILatest = d.OpenInterest.Latest
				out.OIAvg = d.OpenInterest.Average
			}
			out.PriceChg1hPct = d.PriceChange1h
		}
	}
	if ctx != nil && ctx.OITopDataMap != nil {
		if oi := ctx.OITopDataMap[symU]; oi != nil && oi.Rank > 0 {
			out.OITopRank = oi.Rank
			out.OITopDeltaPct = oi.OIDeltaPercent
		}
	}
	if out.hasDisplaySignal() {
		return out
	}
	out2 := &ComkunSymbolVolumeOI{}
	if d, err := market.Get(norm); err == nil && d != nil {
		if d.LongerTermContext != nil {
			out2.Vol4hCurrent = d.LongerTermContext.CurrentVolume
			out2.Vol4hAvg = d.LongerTermContext.AverageVolume
		}
		if d.OpenInterest != nil {
			out2.OILatest = d.OpenInterest.Latest
			out2.OIAvg = d.OpenInterest.Average
		}
		out2.PriceChg1hPct = d.PriceChange1h
	}
	if ctx != nil && ctx.OITopDataMap != nil {
		if oi := ctx.OITopDataMap[symU]; oi != nil && oi.Rank > 0 {
			out2.OITopRank = oi.Rank
			out2.OITopDeltaPct = oi.OIDeltaPercent
		}
	}
	if out2.hasDisplaySignal() {
		return out2
	}
	return nil
}

// interpretMirrorVolumeOIForZH 根据量能/OI 事实生成简短中文研判（被控挂限价时写入日志与决策说明）。
func interpretMirrorVolumeOIForZH(b *ComkunSymbolVolumeOI) string {
	if b == nil || !b.hasDisplaySignal() {
		return "当前缺少可用的量能或持仓量对比数据，挂单主要依据价位同步与账户比例。"
	}
	var s []string
	if b.Vol4hAvg > 1e-12 && b.Vol4hCurrent > 1e-12 {
		ratio := b.Vol4hCurrent / b.Vol4hAvg
		switch {
		case ratio >= 1.25:
			s = append(s, "4h 成交量明显高于近期均量，短线交投偏活跃，限价成交概率与波动风险都相对更高。")
		case ratio <= 0.8:
			s = append(s, "4h 成交量低于近期均量，交投偏淡，该价位附近挂单可能成交更慢。")
		default:
			s = append(s, "4h 成交量与均量接近，量能中性。")
		}
	}
	if b.OIAvg > 1e-9 && b.OILatest > 1e-9 {
		pct := (b.OILatest - b.OIAvg) / (b.OIAvg + 1e-12) * 100
		switch {
		case pct > 5:
			s = append(s, "持仓量 OI 高于近期均值，资金在该合约上的暴露偏多，多空博弈可能加剧。")
		case pct < -5:
			s = append(s, "持仓量 OI 低于近期均值，资金有离场迹象，趋势延续性需更谨慎看待。")
		default:
			s = append(s, "OI 与均值接近，持仓结构相对稳定。")
		}
	}
	// PriceChange1h 为百分比数值（如 1.2 表示 +1.2%）
	if math.Abs(b.PriceChg1hPct) > 0.5 {
		if b.PriceChg1hPct > 0 {
			s = append(s, "近 1h 价格偏强，若挂卖单或减多需注意是否已接近短线过热区。")
		} else {
			s = append(s, "近 1h 价格偏弱，若挂买单或回补空头需注意下跌惯性。")
		}
	}
	if b.OITopRank > 0 && b.OITopRank <= 30 {
		s = append(s, fmt.Sprintf("该合约出现在 OI 增幅榜前列（#%d），市场关注度较高，限价宜控制仓位与杠杆。", b.OITopRank))
	}
	if len(s) == 0 {
		return "量能与 OI 信号偏中性，结合价位执行挂单。"
	}
	return strings.Join(s, "")
}

// buildComkunVolumeOIBrief 主控发广播前：对持仓+挂单涉及的交易对拉取量能/OI（及可选 OI 榜），写入 master_state_json。
func buildComkunVolumeOIBrief(ctx *kernel.Context, se *kernel.StrategyEngine) map[string]*ComkunSymbolVolumeOI {
	if ctx == nil {
		return nil
	}
	const maxSymbols = 32
	syms := make(map[string]struct{})
	for _, p := range ctx.Positions {
		if s := strings.TrimSpace(p.Symbol); s != "" {
			syms[market.Normalize(s)] = struct{}{}
		}
	}
	for _, o := range ctx.PendingOrders {
		if s := strings.TrimSpace(o.Symbol); s != "" {
			syms[market.Normalize(s)] = struct{}{}
		}
	}
	if len(syms) > maxSymbols {
		keys := make([]string, 0, len(syms))
		for k := range syms {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		trim := make(map[string]struct{})
		for i := 0; i < maxSymbols && i < len(keys); i++ {
			trim[keys[i]] = struct{}{}
		}
		syms = trim
	}
	if len(syms) == 0 {
		return nil
	}
	var oiTop map[string]*kernel.OITopData
	if se != nil {
		oiTop = se.FetchOITopDataMap()
	}
	out := make(map[string]*ComkunSymbolVolumeOI)
	for norm := range syms {
		brief := &ComkunSymbolVolumeOI{}
		if d, err := market.Get(norm); err == nil && d != nil {
			if d.LongerTermContext != nil {
				brief.Vol4hCurrent = d.LongerTermContext.CurrentVolume
				brief.Vol4hAvg = d.LongerTermContext.AverageVolume
			}
			if d.OpenInterest != nil {
				brief.OILatest = d.OpenInterest.Latest
				brief.OIAvg = d.OpenInterest.Average
			}
			brief.PriceChg1hPct = d.PriceChange1h
		} else if err != nil {
			logger.Infof("comkun master state: market.Get %s: %v", norm, err)
		}
		if oiTop != nil {
			if row, ok := oiTop[norm]; ok && row != nil {
				brief.OITopRank = row.Rank
				brief.OITopDeltaPct = row.OIDeltaPercent
			}
		}
		if brief.hasDisplaySignal() {
			out[norm] = brief
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type comkunMasterStateWire struct {
	V                int                              `json:"v"`
	SourceEventID    string                           `json:"source_event_id,omitempty"`
	SourceSequence   int64                            `json:"source_sequence,omitempty"`
	SnapshotSequence int64                            `json:"snapshot_sequence,omitempty"`
	OccurredAt       time.Time                        `json:"occurred_at,omitempty"`
	EventType        string                           `json:"event_type,omitempty"`
	PollingReconcile bool                             `json:"polling_reconcile,omitempty"`
	Positions        []kernel.PositionInfo            `json:"positions"`
	PendingOrders    []kernel.PendingOrder            `json:"pending_orders"`
	CandidateCoins   []string                         `json:"candidate_coins,omitempty"`
	MirrorMargin     *ComkunMirrorMarginMeta          `json:"mirror_margin,omitempty"`
	MT4Event         *store.MT4SignalEvent            `json:"mt4_event,omitempty"`
	VolumeOIBrief    map[string]*ComkunSymbolVolumeOI `json:"volume_oi_brief,omitempty"`
	MirrorSemanticFP string                           `json:"mirror_semantic_fp,omitempty"` // 持仓+挂单+杠杆元数据稳定指纹（不含浮盈等展示噪声）
}

func comkunCandidateSymbolsFromCtx(ctx *kernel.Context) []string {
	if ctx == nil || len(ctx.CandidateCoins) == 0 {
		return nil
	}
	out := make([]string, 0, len(ctx.CandidateCoins))
	seen := map[string]bool{}
	for _, c := range ctx.CandidateCoins {
		s := strings.ToUpper(strings.TrimSpace(c.Symbol))
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func comkunMasterStateJSONFromCtxBasic(ctx *kernel.Context, cfg *store.StrategyConfig) string {
	if ctx == nil {
		return ""
	}
	positions := ctx.Positions
	if positions == nil {
		positions = []kernel.PositionInfo{}
	}
	pending := ctx.PendingOrders
	if pending == nil {
		pending = []kernel.PendingOrder{}
	}
	if cfg != nil && cfg.ComkunFollowListingTemplate && cfg.ComkunListingMasterSolManualBroadcastMode {
		orig := positions
		fltPos := make([]kernel.PositionInfo, 0, len(positions))
		for _, p := range positions {
			if strings.EqualFold(strings.TrimSpace(p.Symbol), "SOLUSDT") {
				fltPos = append(fltPos, p)
			}
		}
		positions = fltPos
		fltPend := make([]kernel.PendingOrder, 0, len(pending))
		for _, o := range pending {
			if strings.EqualFold(strings.TrimSpace(o.Symbol), "SOLUSDT") {
				fltPend = append(fltPend, o)
			}
		}
		pending = fltPend
		var droppedSyms []string
		seen := map[string]bool{}
		for _, p := range orig {
			s := strings.TrimSpace(p.Symbol)
			if s == "" || strings.EqualFold(s, "SOLUSDT") {
				continue
			}
			u := strings.ToUpper(s)
			if seen[u] {
				continue
			}
			seen[u] = true
			droppedSyms = append(droppedSyms, s)
		}
		if len(droppedSyms) > 0 {
			logger.Warnf("comkun master broadcast(basic): 已启用 SOL 手动广播模式，下列合约不会写入主广播（跟单端无法镜像）：%s",
				strings.Join(droppedSyms, ", "))
		}
	}
	positions = kernel.EnrichPositionsSLTPFromPending(positions, pending)
	wire := comkunMasterStateWire{
		V:              1,
		Positions:      positions,
		PendingOrders:  pending,
		CandidateCoins: comkunCandidateSymbolsFromCtx(ctx),
	}
	if cfg != nil {
		ml := store.ComkunMirrorMasterMarginLeverageOrDefault(cfg)
		wire.MirrorMargin = &ComkunMirrorMarginMeta{MasterMarginLeverage: ml}
	}
	wire.MirrorSemanticFP = ComkunMirrorSemanticFingerprintFromMasterStateWire(&wire)
	b, err := json.Marshal(wire)
	if err != nil {
		return ""
	}
	return string(b)
}

func comkunMasterStateJSONFlatFromCtx(ctx *kernel.Context, cfg *store.StrategyConfig) string {
	if ctx == nil {
		return ""
	}
	wire := comkunMasterStateWire{
		V:              1,
		Positions:      []kernel.PositionInfo{},
		PendingOrders:  []kernel.PendingOrder{},
		CandidateCoins: comkunCandidateSymbolsFromCtx(ctx),
	}
	if cfg != nil {
		ml := store.ComkunMirrorMasterMarginLeverageOrDefault(cfg)
		wire.MirrorMargin = &ComkunMirrorMarginMeta{MasterMarginLeverage: ml}
	}
	wire.MirrorSemanticFP = ComkunMirrorSemanticFingerprintFromMasterStateWire(&wire)
	b, err := json.Marshal(wire)
	if err != nil {
		return ""
	}
	return string(b)
}

// comkunMasterStateJSONFromCtx 序列化主控交易所快照（持仓 + 挂单 + 镜像杠杆元数据 + 成交量/OI 摘要），供被控镜像跟单。
func comkunMasterStateJSONFromCtx(ctx *kernel.Context, cfg *store.StrategyConfig, se *kernel.StrategyEngine) string {
	if ctx == nil {
		return ""
	}
	positions := ctx.Positions
	if positions == nil {
		positions = []kernel.PositionInfo{}
	}
	pending := ctx.PendingOrders
	if pending == nil {
		pending = []kernel.PendingOrder{}
	}
	// SOL 专项模板：主广播快照只保留 SOLUSDT，避免历史挂单残留导致跟单端仍看到其它币对。
	if cfg != nil && cfg.ComkunFollowListingTemplate && cfg.ComkunListingMasterSolManualBroadcastMode {
		orig := positions
		fltPos := make([]kernel.PositionInfo, 0, len(positions))
		for _, p := range positions {
			if strings.EqualFold(strings.TrimSpace(p.Symbol), "SOLUSDT") {
				fltPos = append(fltPos, p)
			}
		}
		positions = fltPos
		fltPend := make([]kernel.PendingOrder, 0, len(pending))
		for _, o := range pending {
			if strings.EqualFold(strings.TrimSpace(o.Symbol), "SOLUSDT") {
				fltPend = append(fltPend, o)
			}
		}
		pending = fltPend
		var droppedSyms []string
		seen := map[string]bool{}
		for _, p := range orig {
			s := strings.TrimSpace(p.Symbol)
			if s == "" || strings.EqualFold(s, "SOLUSDT") {
				continue
			}
			u := strings.ToUpper(s)
			if seen[u] {
				continue
			}
			seen[u] = true
			droppedSyms = append(droppedSyms, s)
		}
		if len(droppedSyms) > 0 {
			logger.Warnf("comkun master broadcast: 已启用 SOL 手动广播模式，下列合约不会写入主广播（跟单端无法镜像）：%s",
				strings.Join(droppedSyms, ", "))
		}
	}
	positions = kernel.EnrichPositionsSLTPFromPending(positions, pending)
	wire := comkunMasterStateWire{
		V:              1,
		Positions:      positions,
		PendingOrders:  pending,
		CandidateCoins: comkunCandidateSymbolsFromCtx(ctx),
	}
	// volume_oi_brief 同样受上面「仅 SOL」过滤影响（因为 ctx.PendingOrders/Positions 是本次序列化用的快照）
	if vb := buildComkunVolumeOIBrief(&kernel.Context{Positions: positions, PendingOrders: pending}, se); len(vb) > 0 {
		wire.VolumeOIBrief = vb
	}
	if cfg != nil {
		ml := store.ComkunMirrorMasterMarginLeverageOrDefault(cfg)
		wire.MirrorMargin = &ComkunMirrorMarginMeta{MasterMarginLeverage: ml}
	}
	wire.MirrorSemanticFP = ComkunMirrorSemanticFingerprintFromMasterStateWire(&wire)
	b, err := json.Marshal(wire)
	if err != nil {
		return ""
	}
	return string(b)
}

func resolveMirrorMasterLevFromWire(wire *comkunMasterStateWire) int {
	if wire != nil && wire.MirrorMargin != nil && wire.MirrorMargin.MasterMarginLeverage > 0 {
		v := wire.MirrorMargin.MasterMarginLeverage
		if v > 125 {
			return 125
		}
		return v
	}
	return 20
}

// mirrorResolveMasterLevForSymbol 镜像限价用：优先主控快照里该币对持仓的交易所杠杆，否则用广播 mirror_margin，再默认 20。
func mirrorResolveMasterLevForSymbol(positions []kernel.PositionInfo, sym string, masterLevWire int) int {
	su := strings.ToUpper(strings.TrimSpace(sym))
	for _, p := range positions {
		if strings.ToUpper(strings.TrimSpace(p.Symbol)) != su {
			continue
		}
		if p.Leverage > 0 {
			return p.Leverage
		}
	}
	if masterLevWire > 0 {
		return masterLevWire
	}
	return 20
}

// mirrorEquityMarginPctFromPosition 主控该腿占权益比例（按名义价值 qty×价，非页面 margin_used）。
// 主控仅调杠杆、qty 不变时比例不变，避免被控误减仓。
func mirrorEquityMarginPctFromPosition(mp kernel.PositionInfo, masterEq float64, masterLevFallback int) float64 {
	if masterEq < 1e-9 {
		return 0
	}
	px := mp.MarkPrice
	if px <= 0 {
		px = mp.EntryPrice
	}
	if px <= 0 || math.Abs(mp.Quantity) < 1e-12 {
		return 0
	}
	notional := math.Abs(mp.Quantity) * px
	pct := notional / masterEq
	if pct > 1 {
		return 1
	}
	if pct < 0 {
		return 0
	}
	return pct
}

// mirrorEquityMarginPctFromLimitOrder 主控限价单若成交，对应的初始保证金占主控权益比例；杠杆取该币对主控持仓上的值，从下限价起与主控「保证金占比」对齐。
func mirrorEquityMarginPctFromLimitOrder(o kernel.PendingOrder, positions []kernel.PositionInfo, masterEq float64, masterLevWire int) float64 {
	if masterEq < 1e-9 || o.Quantity <= 0 {
		return 0
	}
	px := kernel.PendingLimitDisplayPrice(o)
	if px <= 0 {
		return 0
	}
	Lm := mirrorResolveMasterLevForSymbol(positions, o.Symbol, masterLevWire)
	if Lm < 1 {
		Lm = 20
	}
	im := o.Quantity * px / float64(Lm)
	if im <= 0 {
		return 0
	}
	pct := im / masterEq
	if pct > 1 {
		return 1
	}
	if pct < 0 {
		return 0
	}
	return pct
}

// mirrorQtyFromEquityMarginPct 被控张数：与主控相同的保证金占权益比 pct，用被控镜像杠杆还原名义。
func mirrorQtyFromEquityMarginPct(pct, followerEq, px float64, followLev int) float64 {
	if pct <= 0 || followerEq < 1e-9 || px <= 0 {
		return 0
	}
	fl := followLev
	if fl < 1 {
		fl = 20
	}
	return pct * followerEq * float64(fl) / px
}

// mirrorScaledLimitQty 主控限价镜像张数：主控侧 (名义/Lm)/主控权益 = 被控侧 (名义/Lf)/被控权益；Lm 优先取主控该币对持仓交易所杠杆。
func mirrorScaledLimitQty(o kernel.PendingOrder, positions []kernel.PositionInfo, masterEq, followerEq float64, masterLevWire, followLev int) float64 {
	if masterEq < 1e-9 || followerEq < 1e-9 {
		return 0
	}
	px := kernel.PendingLimitDisplayPrice(o)
	if px <= 0 || o.Quantity <= 0 {
		return 0
	}
	pct := mirrorEquityMarginPctFromLimitOrder(o, positions, masterEq, masterLevWire)
	return mirrorQtyFromEquityMarginPct(pct, followerEq, px, followLev)
}

// mirrorScaledLimitQtyCapped 在 mirrorScaledLimitQty 基础上增加上限：不超过「主控挂单张数 × (被控权益/主控权益)」。
// 否则主控 equity 急跌时，按保证金占比缩放会把被控限价数量越算越大，挂单冻结保证金持续上升（并非重复开仓，但观感类似「越跟越大」）。
func mirrorScaledLimitQtyCapped(o kernel.PendingOrder, positions []kernel.PositionInfo, masterEq, followerEq float64, masterLevWire, followLev int) float64 {
	q := mirrorScaledLimitQty(o, positions, masterEq, followerEq, masterLevWire, followLev)
	if masterEq < 1e-9 || followerEq < 1e-9 || o.Quantity <= 0 {
		return q
	}
	simple := o.Quantity * (followerEq / masterEq)
	if simple > 0 && q > simple+1e-12 {
		return simple
	}
	return q
}

// mirrorScaledPositionQty 主控持仓镜像张数：与限价同一套「保证金占主控权益」比例，优先用快照里的 MarginUsed。
// buildMirrorMasterTargetFromWire 按主控快照构建被控缩放目标仓位 map（posKey -> qty）。
func buildMirrorMasterTargetFromWire(wire *comkunMasterStateWire, masterEq, followerEq float64) map[string]float64 {
	masterTarget := make(map[string]float64)
	if wire == nil {
		return masterTarget
	}
	masterLev := resolveMirrorMasterLevFromWire(wire)
	fallbackRatio := 1.0
	if masterEq > 1e-9 {
		fallbackRatio = followerEq / masterEq
	}
	if fallbackRatio <= 0 || math.IsNaN(fallbackRatio) || math.IsInf(fallbackRatio, 0) {
		fallbackRatio = 1
	}
	for _, mp := range wire.Positions {
		sym := strings.TrimSpace(mp.Symbol)
		side := strings.ToLower(strings.TrimSpace(mp.Side))
		if sym == "" || (side != "long" && side != "short") {
			continue
		}
		var mq float64
		if masterEq > 1e-9 {
			posLev := mp.Leverage
			if posLev < 1 {
				posLev = masterLev
			}
			mq = mirrorScaledPositionQty(mp, masterEq, followerEq, posLev, posLev)
		}
		if mq < 1e-12 {
			mq = mp.Quantity * fallbackRatio
		}
		if mq < 1e-12 {
			continue
		}
		masterTarget[posKey(sym, side)] += mq
	}
	return masterTarget
}

type hzLotsQuantityConverter func(symbol string, lots float64) (float64, error)

func hzScaledPositionQuantity(mp kernel.PositionInfo, masterEq, followerEq float64, convert hzLotsQuantityConverter) (float64, error) {
	if masterEq <= 0 || followerEq <= 0 || mp.Lots <= 0 || convert == nil {
		return 0, nil
	}
	quantity, err := convert(mp.Symbol, mp.Lots)
	if err != nil {
		return 0, err
	}
	return math.Abs(quantity) * followerEq / masterEq, nil
}

func buildHZMasterTargetFromWire(wire *comkunMasterStateWire, masterEq, followerEq float64, convert hzLotsQuantityConverter) (map[string]float64, error) {
	target := make(map[string]float64)
	if wire == nil {
		return target, nil
	}
	for _, position := range wire.Positions {
		symbol := strings.TrimSpace(position.Symbol)
		side := strings.ToLower(strings.TrimSpace(position.Side))
		if symbol == "" || (side != "long" && side != "short") {
			continue
		}
		quantity, err := hzScaledPositionQuantity(position, masterEq, followerEq, convert)
		if err != nil {
			return nil, err
		}
		if quantity > 0 {
			target[posKey(symbol, side)] += quantity
		}
	}
	return target, nil
}

func hzMirrorDeltaAllowed(delta float64, occurredAt, now time.Time, closeOnly bool) bool {
	if delta <= 0 {
		return true
	}
	if closeOnly || occurredAt.IsZero() {
		return false
	}
	return !occurredAt.Before(now.Add(-2 * time.Minute))
}

func hzMirrorRiskIncreaseExpired(wire *comkunMasterStateWire, now time.Time) bool {
	if wire == nil {
		return false
	}
	eventType := strings.ToUpper(strings.TrimSpace(wire.EventType))
	if eventType != "OPEN" && eventType != "INCREASE" {
		return false
	}
	return !hzMirrorDeltaAllowed(1, wire.OccurredAt, now, false)
}

// buildMT4MasterTargetFromWire preserves the MT4 account risk percentage.
// MT4 cent-account lots are not a safe proxy for used margin, especially for
// martingale books that accumulate many small tickets.
func buildMT4MasterTargetFromWire(wire *comkunMasterStateWire, masterEq, followerEq float64, followerLev int) map[string]float64 {
	if wire == nil || wire.MirrorMargin == nil || wire.MirrorMargin.MasterMarginUsed <= 0 || masterEq <= 0 || followerEq <= 0 {
		return buildMirrorMasterTargetFromWire(wire, masterEq, followerEq)
	}
	marginPct := wire.MirrorMargin.MasterMarginUsed / masterEq
	if marginPct <= 0 {
		return map[string]float64{}
	}
	if marginPct > 1 {
		marginPct = 1
	}
	fl := followerLev
	if fl < 1 {
		fl = resolveMirrorMasterLevFromWire(wire)
	}
	if fl < 1 {
		fl = 20
	}
	type leg struct {
		key, symbol string
		margin      float64
		price       float64
	}
	legs := make([]leg, 0, len(wire.Positions))
	totalSyntheticMargin := 0.0
	for _, position := range wire.Positions {
		symbol := strings.TrimSpace(position.Symbol)
		side := strings.ToLower(strings.TrimSpace(position.Side))
		price := position.MarkPrice
		if price <= 0 {
			price = position.EntryPrice
		}
		lev := position.Leverage
		if lev < 1 {
			lev = resolveMirrorMasterLevFromWire(wire)
		}
		if symbol == "" || (side != "long" && side != "short") || price <= 0 || position.Quantity <= 0 || lev < 1 {
			continue
		}
		margin := math.Abs(position.Quantity) * price / float64(lev)
		if margin <= 0 {
			continue
		}
		legs = append(legs, leg{key: posKey(symbol, side), symbol: symbol, margin: margin, price: price})
		totalSyntheticMargin += margin
	}
	if totalSyntheticMargin <= 0 {
		return buildMirrorMasterTargetFromWire(wire, masterEq, followerEq)
	}
	followerMargin := marginPct * followerEq
	target := make(map[string]float64)
	for _, item := range legs {
		qty := (item.margin / totalSyntheticMargin) * followerMargin * float64(fl) / item.price
		if qty > 0 {
			target[item.key] += qty
		}
	}
	return target
}

// mirrorScaledPositionQty 主控持仓镜像张数：按 qty 占主控权益比例缩放（调杠杆不改变 qty 时不改目标张数）。
func mirrorScaledPositionQty(mp kernel.PositionInfo, masterEq, followerEq float64, masterLev, followLev int) float64 {
	_ = masterLev
	_ = followLev
	if masterEq < 1e-9 || followerEq < 1e-9 || mp.Quantity < 1e-12 {
		return 0
	}
	return math.Abs(mp.Quantity) * followerEq / masterEq
}

// mirrorDisplayLimitPriceClose 判断被控限价与展示价是否同一点位（相对容差）。
func mirrorDisplayLimitPriceClose(a, b float64) bool {
	if a <= 0 || b <= 0 {
		return false
	}
	eps := math.Max(1e-10, math.Max(math.Abs(a), math.Abs(b))*1e-8)
	return math.Abs(a-b) <= eps
}

// followerHasOpenLimitAtDisplayPrice 被控账户在该交易对上是否已有未成交限价，且委托价与镜像展示价一致。
func followerHasOpenLimitAtDisplayPrice(at *AutoTrader, symbol string, displayPrice float64) bool {
	if at == nil || at.trader == nil || displayPrice <= 0 {
		return false
	}
	sym := strings.TrimSpace(symbol)
	if sym == "" {
		return false
	}
	ords, err := at.trader.GetOpenOrders(sym)
	if err != nil || len(ords) == 0 {
		return false
	}
	symU := strings.ToUpper(sym)
	for _, o := range ords {
		if strings.ToUpper(strings.TrimSpace(o.Symbol)) != symU {
			continue
		}
		typ := strings.ToUpper(strings.TrimSpace(o.Type))
		if typ != "LIMIT" && typ != "LIMIT_MAKER" {
			continue
		}
		px := o.Price
		if px <= 0 {
			px = o.StopPrice
		}
		if mirrorDisplayLimitPriceClose(px, displayPrice) {
			return true
		}
	}
	return false
}

// followerCoversAllMirrorDisplayLimits 被控是否已在每条镜像展示决策对应的价位上挂了限价；是则不再写 decision_json，避免每轮扫描重复出卡片。
func followerCoversAllMirrorDisplayLimits(at *AutoTrader, display []kernel.Decision) bool {
	if at == nil || len(display) == 0 {
		return false
	}
	for _, d := range display {
		sym := strings.TrimSpace(d.Symbol)
		px := d.Price
		if sym == "" || px <= 0 {
			continue
		}
		if !followerHasOpenLimitAtDisplayPrice(at, sym, px) {
			return false
		}
	}
	return true
}

// fingerprintMirrorLimitFloat 指纹用稳定小数，避免 JSON 往返 / 交易所字段微抖导致每轮指纹不同。
func fingerprintMirrorLimitFloat(f float64) string {
	s := fmt.Sprintf("%.8f", f)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}

// fingerprintMirrorLimitOrders 与 buildMirrorDisplayDecisionsFromWire 使用同一套挂单筛选与展示价，
// 否则会出现「有展示卡片但指纹永远为空」→ 每轮都写 decision_json → 限价卡片刷屏。
func fingerprintMirrorLimitOrders(wire comkunMasterStateWire) string {
	var lines []string
	for _, o := range wire.PendingOrders {
		if !kernel.IsLimitLikePendingOrderType(o.Type) {
			continue
		}
		px := kernel.PendingLimitDisplayPrice(o)
		sym := strings.TrimSpace(o.Symbol)
		if sym == "" || px <= 0 || o.Quantity <= 0 {
			continue
		}
		ps := mirrorLimitPositionSide(o, wire.Positions)
		act := mapMirrorLimitToDisplayAction(strings.ToUpper(strings.TrimSpace(o.Side)), ps)
		if act == "wait" {
			continue
		}
		typ := strings.ToUpper(strings.TrimSpace(o.Type))
		symU := strings.ToUpper(sym)
		line := fmt.Sprintf("%s|%s|%s|%s|%s|%s",
			symU,
			strings.ToUpper(strings.TrimSpace(o.Side)),
			typ,
			fingerprintMirrorLimitFloat(px),
			fingerprintMirrorLimitFloat(o.Quantity),
			strings.ToUpper(strings.TrimSpace(o.PositionSide)),
		)
		lines = append(lines, line)
	}
	sort.Strings(lines)
	return strings.Join(lines, ";")
}

// stripMirrorCoTBlock 去掉思考链中以 marker 开头的整段（到下一节标题或文末）。
func stripMirrorCoTBlock(s, marker string) string {
	s = strings.TrimSpace(s)
	if s == "" || marker == "" {
		return s
	}
	i := strings.Index(s, marker)
	if i < 0 {
		return s
	}
	rest := s[i+len(marker):]
	next := len(s)
	for _, sep := range []string{"\n\n【", "\n\n##", "\n\n---"} {
		if j := strings.Index(rest, sep); j >= 0 {
			cand := i + len(marker) + j
			if cand < next {
				next = cand
			}
		}
	}
	if next < len(s) {
		return strings.TrimSpace(s[:i] + s[next:])
	}
	return strings.TrimSpace(s[:i])
}

func mirrorNeutralizeExposureWording(s string) string {
	if s == "" {
		return ""
	}
	r := strings.NewReplacer(
		"主控账户", "当前账户",
		"主控权益", "参考权益",
		"主控挂单", "该笔挂单",
		"主控侧", "",
		"主控", "",
		"被控账户", "本账户",
		"被控端", "本端",
		"被控", "本端",
		"广播", "",
		"跟单端简报", "简要结论",
		"跟单端", "",
		"订阅者", "",
		"订阅用户", "",
		"订阅方", "",
		"人工", "",
		"手动", "",
		"镜像同步", "同步",
	)
	return strings.TrimSpace(r.Replace(s))
}

// mirrorLimitActionLabelZH 决策动作用语（用于「我在本价位挂何种限价」叙述，避免「解读他人单」语感）。
func mirrorLimitActionLabelZH(action string) string {
	switch strings.TrimSpace(strings.ToLower(action)) {
	case "open_long":
		return "开多限价"
	case "open_short":
		return "开空限价"
	case "close_long":
		return "平多限价"
	case "close_short":
		return "平空限价"
	default:
		return "限价单"
	}
}

// mirrorLimitIntentPhraseZH 第一人称：说明「我为何在本轮选这个价位挂限价」，不写推断他人意图。
func mirrorLimitIntentPhraseZH(action string) string {
	switch strings.TrimSpace(strings.ToLower(action)) {
	case "open_long":
		return "我选择在相对低位用限价承接或加多，用挂单控制入场成本，避免市价追高。"
	case "open_short":
		return "我选择在压力附近用限价试空或对冲，用价位约束风险，不急于市价追空。"
	case "close_long":
		return "我选择在盈利或压力带用限价分批减多，锁定利润、压低滑点。"
	case "close_short":
		return "我选择在有利价位用限价回补空头，控制成交节奏与滑点。"
	default:
		return "我结合当前持仓与方向，在本价位挂限价以落实本轮计划。"
	}
}

// buildMirrorAutonomousLimitSection 第一人称：先分析再挂单——写「我在该点位挂限价」的执行计划，不写「解读某笔单为何摆在这」。
func buildMirrorAutonomousLimitSection(display []kernel.Decision) string {
	if len(display) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("【本价位限价安排】\n")
	b.WriteString("在完成本轮量能与 OI 研判后，我在下列点位挂限价单（名义与杠杆按本账户规则缩放）：\n")
	for _, d := range display {
		sym := strings.TrimSpace(d.Symbol)
		if sym == "" {
			continue
		}
		phrase := mirrorLimitIntentPhraseZH(d.Action)
		lab := mirrorLimitActionLabelZH(d.Action)
		b.WriteString(fmt.Sprintf("\n• %s：在约 %.4f 挂%s，名义约 %.2f USDT、杠杆 %dx。%s",
			sym, d.Price, lab, d.PositionSizeUSD, d.Leverage, phrase))
		if d.StopLoss > 0 || d.TakeProfit > 0 {
			b.WriteString(fmt.Sprintf(" 同步设置的价位参考：止损约 %.6f、止盈约 %.6f（以交易所实际委托为准）。", d.StopLoss, d.TakeProfit))
		}
	}
	return b.String()
}

func mirrorAnalysisIsDryFallback(s string) bool {
	t := strings.TrimSpace(s)
	if t == "" {
		return true
	}
	return strings.Contains(t, "未生成可截取") ||
		strings.Contains(t, "未附带可展示") ||
		strings.Contains(t, "以下为交易所持仓与挂单快照")
}

func mirrorDirectionTextZH(action string) string {
	switch strings.TrimSpace(strings.ToLower(action)) {
	case "open_long":
		return "偏多承接"
	case "open_short":
		return "偏空试探"
	case "close_long":
		return "多单减仓/止盈"
	case "close_short":
		return "空单回补/止盈"
	default:
		return "等待成交"
	}
}

func buildMirrorAutonomousMarketReport(display []kernel.Decision, volBrief map[string]*ComkunSymbolVolumeOI) string {
	var b strings.Builder
	b.WriteString("# 市场分析报告\n\n")
	b.WriteString("## 一、当前市场结构与执行方向\n\n")
	if len(display) == 0 {
		b.WriteString("本轮没有需要新执行的挂单或仓位变化，交易节奏以等待新信号为主；在没有新的有效价位出现前，不主动追单。\n\n")
	} else {
		for _, d := range display {
			sym := strings.TrimSpace(strings.ToUpper(d.Symbol))
			if sym == "" {
				continue
			}
			dir := mirrorDirectionTextZH(d.Action)
			b.WriteString(fmt.Sprintf("### %s - %s\n\n", sym, dir))
			b.WriteString(fmt.Sprintf("本轮重点观察 %s 的限价成交机会。当前计划不是追市价，而是在 %.4f 附近等待价格回到计划区间后再成交，目的是控制入场成本和滑点。\n\n", sym, d.Price))
			if vb := volBrief[market.Normalize(sym)]; vb != nil && vb.hasDisplaySignal() {
				parts := appendComkunMasterVolumeOIParts(vb)
				if len(parts) > 0 {
					b.WriteString("量能/OI 参考：")
					b.WriteString(strings.Join(parts, " · "))
					if j := strings.TrimSpace(interpretMirrorVolumeOIForZH(vb)); j != "" {
						b.WriteString("。综合判断：")
						b.WriteString(j)
					}
					b.WriteString("\n\n")
				}
			}
		}
	}

	if len(display) > 0 {
		b.WriteString("## 二、限价执行计划\n\n")
		b.WriteString("| 币种 | 方向 | 计划价位 | 名义金额 | 杠杆 | 风控参考 |\n")
		b.WriteString("|---|---|---:|---:|---:|---|\n")
		for _, d := range display {
			sym := strings.TrimSpace(strings.ToUpper(d.Symbol))
			if sym == "" {
				continue
			}
			risk := "以交易所实际委托为准"
			if d.StopLoss > 0 || d.TakeProfit > 0 {
				risk = fmt.Sprintf("止损 %.6f / 止盈 %.6f", d.StopLoss, d.TakeProfit)
			}
			b.WriteString(fmt.Sprintf("| %s | %s | %.4f | %.2f USDT | %dx | %s |\n",
				sym, mirrorLimitActionLabelZH(d.Action), d.Price, d.PositionSizeUSD, d.Leverage, risk))
		}
		b.WriteString("\n")
	}

	b.WriteString("## 三、风险与纪律\n\n")
	b.WriteString("- 未成交前不追价，继续等待计划价位触发。\n")
	b.WriteString("- 如果价格快速脱离计划区间，优先保持挂单纪律，不临时扩大风险。\n")
	b.WriteString("- 成交后以同步后的止损、止盈或后续新信号管理仓位。\n\n")
	b.WriteString("## 四、综合结论\n\n")
	if len(display) > 0 {
		b.WriteString("本轮结论：按计划执行限价挂单，等待市场给出成交位置；不做额外主观加仓。\n")
	} else {
		b.WriteString("本轮结论：暂无新的执行动作，保持等待。\n")
	}
	return strings.TrimSpace(b.String())
}

func buildMirrorCloseExplanationFromLogs(logs []string) string {
	if len(logs) == 0 {
		return ""
	}
	seen := map[string]bool{}
	type closeItem struct {
		Symbol string
		Side   string
	}
	var items []closeItem
	for _, line := range logs {
		line = strings.TrimSpace(line)
		const prefix = "镜像: 主控无 "
		const suffix = "，平掉被控仓位"
		if !strings.HasPrefix(line, prefix) || !strings.Contains(line, suffix) {
			continue
		}
		body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, prefix), suffix))
		parts := strings.Split(body, "|")
		if len(parts) != 2 {
			continue
		}
		sym := strings.ToUpper(strings.TrimSpace(parts[0]))
		side := strings.ToLower(strings.TrimSpace(parts[1]))
		if sym == "" || (side != "long" && side != "short") {
			continue
		}
		key := sym + "|" + side
		if seen[key] {
			continue
		}
		seen[key] = true
		items = append(items, closeItem{Symbol: sym, Side: side})
	}
	if len(items) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## 平仓说明\n\n")
	b.WriteString("本轮最新同步快照中，以下仓位已经不再属于有效持仓计划，因此我执行平仓处理。这个动作不是本端独立开新判断，而是跟随最新状态做风险收口。\n\n")
	for _, item := range items {
		sideText := "多单"
		actionText := "平多"
		if item.Side == "short" {
			sideText = "空单"
			actionText = "平空"
		}
		b.WriteString(fmt.Sprintf("### %s %s\n\n", item.Symbol, actionText))
		b.WriteString(fmt.Sprintf("- **触发原因**：最新同步快照里已经没有 %s 的%s，说明这笔持仓计划已经结束，继续持有会变成无信号暴露。\n", item.Symbol, sideText))
		b.WriteString("- **执行逻辑**：先撤该币种相关挂单/条件单，再按当前交易所仓位执行平仓，避免平仓后遗留止盈止损或反向条件单。\n")
		b.WriteString("- **风险理解**：平仓的核心目的不是追求更高收益，而是释放保证金、停止不再受信号保护的风险敞口。\n")
		b.WriteString("- **后续纪律**：平仓后保持等待，只有后续收到新的有效开仓或挂单快照，才重新进入下一轮执行。\n\n")
	}
	b.WriteString("## 综合结论\n\n")
	b.WriteString("本轮属于跟随最新状态完成风险收口；无新开仓信号时，不额外主动进场。\n")
	return strings.TrimSpace(b.String())
}

// formatMirrorPostAnalysisLimitLine 决策卡 Reasoning 末尾一句：强调「分析后的挂单」，而非解读外部挂单。
func formatMirrorPostAnalysisLimitLine(sym string, px float64, action string) string {
	sym = strings.ToUpper(strings.TrimSpace(sym))
	lab := mirrorLimitActionLabelZH(action)
	return fmt.Sprintf("综合上述判断，本轮在 %s 约 %.4f 挂%s。", sym, px, lab)
}

// buildMirrorVolumeOISectionForThought 成交量与 OI，写入思考过程（不出现「广播/主控」等词）。
func buildMirrorVolumeOISectionForThought(brief map[string]*ComkunSymbolVolumeOI) string {
	if len(brief) == 0 {
		return ""
	}
	keys := make([]string, 0, len(brief))
	for k := range brief {
		if strings.TrimSpace(k) != "" {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("【成交量与 OI】\n")
	b.WriteString("这是我本轮重点参考的量能与持仓变化线索：\n")
	for _, sym := range keys {
		parts := appendComkunMasterVolumeOIParts(brief[sym])
		if len(parts) == 0 {
			continue
		}
		b.WriteString("\n• ")
		b.WriteString(sym)
		b.WriteString("：")
		b.WriteString(strings.Join(parts, " · "))
		if brief[sym] != nil {
			if j := strings.TrimSpace(interpretMirrorVolumeOIForZH(brief[sym])); j != "" {
				b.WriteString(" 研判：")
				b.WriteString(j)
			}
		}
	}
	return b.String()
}

// FinalizeMirrorFollowerCoT 镜像跟单落库前的思考过程：去技术扫描块、去敏措辞，并仅保留「成交量/OI」与简要结论。
func FinalizeMirrorFollowerCoT(rawAnalysis string, volBrief map[string]*ComkunSymbolVolumeOI, display []kernel.Decision) string {
	base := strings.TrimSpace(kernel.ExtractFollowerBroadcastCoT(rawAnalysis))
	if base == "" {
		base = strings.TrimSpace(kernel.FallbackFollowerBriefFromCoT(rawAnalysis))
	}
	base = kernel.StripPendingOrdersScanBlocks(base)
	base = stripMirrorCoTBlock(base, "【挂单与风控解读】")
	base = stripMirrorCoTBlock(base, "【风控与价位理解】")
	base = stripMirrorCoTBlock(base, "[Orders & risk commentary]")
	base = stripMirrorCoTBlock(base, "【跟单信号 · 成交量与 OI 快照】")
	base = strings.TrimSpace(base)
	base = mirrorNeutralizeExposureWording(base)
	dryBase := mirrorAnalysisIsDryFallback(base)
	if dryBase && len(display) > 0 {
		return strings.TrimSpace(buildMirrorAutonomousMarketReport(display, volBrief))
	}
	var chunks []string
	if base != "" && !dryBase {
		chunks = append(chunks, "【本轮判断】\n"+base)
	}
	if vol := strings.TrimSpace(buildMirrorVolumeOISectionForThought(volBrief)); vol != "" {
		chunks = append(chunks, vol)
	}
	if lim := strings.TrimSpace(buildMirrorAutonomousLimitSection(display)); lim != "" {
		chunks = append(chunks, lim)
	}
	out := strings.TrimSpace(strings.Join(chunks, "\n\n"))
	if out == "" {
		out = strings.TrimSpace(kernel.CoTForComkunMasterBroadcast(rawAnalysis))
	}
	if mirrorAnalysisIsDryFallback(out) {
		out = strings.TrimSpace(buildMirrorAutonomousMarketReport(display, volBrief))
	}
	if out == "" {
		out = strings.TrimSpace(buildMirrorAutonomousMarketReport(display, volBrief))
	}
	return out
}

func posKey(sym, side string) string {
	return market.Normalize(strings.TrimSpace(sym)) + "|" + strings.ToLower(strings.TrimSpace(side))
}

func splitPosKey(k string) (sym string, side string, ok bool) {
	i := strings.LastIndex(k, "|")
	if i <= 0 || i >= len(k)-1 {
		return "", "", false
	}
	return k[:i], k[i+1:], true
}

func mapFollowerPositionQuantities(positions []map[string]interface{}) map[string]float64 {
	out := make(map[string]float64)
	for _, p := range positions {
		sym, _ := p["symbol"].(string)
		side, _ := p["side"].(string)
		if ps, ok := p["positionSide"].(string); ok {
			switch strings.ToUpper(strings.TrimSpace(ps)) {
			case "LONG":
				side = "long"
			case "SHORT":
				side = "short"
			}
		}
		if strings.TrimSpace(sym) == "" {
			continue
		}
		side = strings.ToLower(strings.TrimSpace(side))
		var q float64
		if side == "long" || side == "short" {
			if v, ok := p["positionAmt"].(float64); ok {
				q = math.Abs(v)
			}
		}
		if math.Abs(q) < 1e-12 {
			continue
		}
		out[posKey(sym, side)] = q
	}
	return out
}

func masterLeverageFor(positions []kernel.PositionInfo, sym, side string) int {
	su := strings.ToUpper(strings.TrimSpace(sym))
	sd := strings.ToLower(strings.TrimSpace(side))
	for _, p := range positions {
		if strings.ToUpper(strings.TrimSpace(p.Symbol)) == su && strings.ToLower(strings.TrimSpace(p.Side)) == sd {
			if p.Leverage > 0 {
				return p.Leverage
			}
		}
	}
	return 10
}

// mirrorLimitPositionSide 镜像限价：优先用挂单上的 LONG/SHORT；若为 BOTH/空（单向常见），则根据主控持仓判断是平仓侧还是开仓侧。
func mirrorLimitPositionSide(o kernel.PendingOrder, masterPos []kernel.PositionInfo) string {
	raw := strings.ToUpper(strings.TrimSpace(o.PositionSide))
	if raw == "LONG" || raw == "SHORT" {
		return raw
	}
	sym := strings.TrimSpace(o.Symbol)
	sideU := strings.ToUpper(strings.TrimSpace(o.Side))
	var hasLong, hasShort bool
	for _, mp := range masterPos {
		if !strings.EqualFold(strings.TrimSpace(mp.Symbol), sym) {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(mp.Side)) {
		case "long":
			hasLong = true
		case "short":
			hasShort = true
		}
	}
	// 单向常见：仅有多头时卖单多为平多；仅有空头时买单多为平空
	if hasLong && !hasShort && sideU == "SELL" {
		return "LONG"
	}
	if hasShort && !hasLong && sideU == "BUY" {
		return "SHORT"
	}
	if sideU == "SELL" {
		return "SHORT"
	}
	return "LONG"
}

func masterLeverageForSymbol(positions []kernel.PositionInfo, sym string) int {
	su := strings.ToUpper(strings.TrimSpace(sym))
	for _, p := range positions {
		if strings.ToUpper(strings.TrimSpace(p.Symbol)) == su && p.Leverage > 0 {
			return p.Leverage
		}
	}
	return 10
}

// mapMirrorLimitToDisplayAction 将主控限价方向映射为前端决策卡片动作（与镜像 mirrorLimitPositionSide 一致）
func mapMirrorLimitToDisplayAction(sideU, posSide string) string {
	sideU = strings.ToUpper(strings.TrimSpace(sideU))
	ps := strings.ToUpper(strings.TrimSpace(posSide))
	switch {
	case sideU == "BUY" && ps == "LONG":
		return "open_long"
	case sideU == "BUY" && ps == "SHORT":
		return "close_short"
	case sideU == "SELL" && ps == "SHORT":
		return "open_short"
	case sideU == "SELL" && ps == "LONG":
		return "close_long"
	default:
		return "wait"
	}
}

// masterWireConditionalTPSLImpliesSide 快照里条件止盈/止损单通常表示该侧仍有实盘仓位（即使 positions 因 API 缓存尚未写入）。
func masterWireConditionalTPSLImpliesSide(wire *comkunMasterStateWire, sym, side string) bool {
	if wire == nil {
		return false
	}
	su := strings.ToUpper(strings.TrimSpace(sym))
	sd := strings.ToLower(strings.TrimSpace(side))
	for _, o := range wire.PendingOrders {
		if !strings.EqualFold(strings.TrimSpace(o.Symbol), su) {
			continue
		}
		typ := strings.ToUpper(strings.TrimSpace(o.Type))
		if !kernel.IsConditionalTakeProfitOrderType(typ) && !kernel.IsConditionalStopLossOrderType(typ) {
			continue
		}
		if kernel.PendingOrderTriggerPrice(o) <= 0 {
			continue
		}
		ps := strings.ToUpper(strings.TrimSpace(o.PositionSide))
		if ps == "LONG" || ps == "SHORT" {
			return strings.ToLower(ps) == sd
		}
		os := strings.ToUpper(strings.TrimSpace(o.Side))
		if sd == "long" && os == "SELL" {
			return true
		}
		if sd == "short" && os == "BUY" {
			return true
		}
	}
	return false
}

// masterWireGlobalFlatNoPositions 主控快照 positions 是否已全无仓（不看挂单）。
// 主控已全无仓时，不应再因「残留开仓限价」长期阻止被控跟平。
func masterWireGlobalFlatNoPositions(wire *comkunMasterStateWire) bool {
	if wire == nil {
		return true
	}
	for _, mp := range wire.Positions {
		if mp.Quantity > 1e-9 {
			return false
		}
	}
	return true
}

// masterWireSnapshotMayLagPosition 主控 positions 可能滞后但挂单已表明不应按「无仓位」去平被控。
func masterWireSnapshotMayLagPosition(wire *comkunMasterStateWire, sym, side string) bool {
	if masterWireGlobalFlatNoPositions(wire) {
		return false
	}
	if masterWireOpenIntentLimitOnSymbol(wire, sym) {
		return true
	}
	// 快照里该币对已无任何仓位时，仍出现的 TP/SL 多为平仓后的短暂残留，不应挡住被控跟平。
	if masterWireHasAnyPositionOnSymbol(wire, sym) && masterWireConditionalTPSLImpliesSide(wire, sym, side) {
		return true
	}
	return false
}

// masterWireOpenIntentLimitOnSymbol 主控快照里该交易对是否存在「开仓类」限价挂单（非 reduceOnly）。
func masterWireOpenIntentLimitOnSymbol(wire *comkunMasterStateWire, sym string) bool {
	if wire == nil {
		return false
	}
	sym = strings.TrimSpace(sym)
	for _, o := range wire.PendingOrders {
		if !kernel.IsLimitLikePendingOrderType(o.Type) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(o.Symbol), sym) {
			continue
		}
		ps := mirrorLimitPositionSide(o, wire.Positions)
		su := strings.ToUpper(strings.TrimSpace(o.Side))
		if (ps == "LONG" && su == "SELL") || (ps == "SHORT" && su == "BUY") {
			continue
		}
		act := mapMirrorLimitToDisplayAction(su, ps)
		if act == "open_long" || act == "open_short" {
			return true
		}
	}
	return false
}

// masterWirePositionSideQty 主控快照里某交易对某方向的持仓数量之和。
func masterWirePositionSideQty(wire *comkunMasterStateWire, sym, side string) float64 {
	if wire == nil {
		return 0
	}
	sd := strings.ToLower(strings.TrimSpace(side))
	var sum float64
	for _, mp := range wire.Positions {
		if !strings.EqualFold(strings.TrimSpace(mp.Symbol), sym) {
			continue
		}
		if strings.ToLower(strings.TrimSpace(mp.Side)) != sd {
			continue
		}
		sum += mp.Quantity
	}
	return sum
}

const mirrorWebStartupPnLEps = 1e-8

// masterWirePositionUnrealizedPnL 主控快照某腿的网页浮动盈亏（USDT，正=浮盈负=浮亏）。
func masterWirePositionUnrealizedPnL(wire *comkunMasterStateWire, sym, side string) (float64, bool) {
	if wire == nil {
		return 0, false
	}
	sd := strings.ToLower(strings.TrimSpace(side))
	for _, mp := range wire.Positions {
		if !strings.EqualFold(strings.TrimSpace(mp.Symbol), sym) {
			continue
		}
		if strings.ToLower(strings.TrimSpace(mp.Side)) != sd {
			continue
		}
		return mp.UnrealizedPnL, true
	}
	return 0, false
}

// mirrorWebPruneHighPnLSkipKeys 主控已平仓的腿从「浮盈跳过」集合移除。
func (at *AutoTrader) mirrorWebPruneHighPnLSkipKeys(masterTarget map[string]float64) {
	if at == nil || len(at.mirrorWebHighPnLSkipKeys) == 0 {
		return
	}
	at.mirrorSafetyMu.Lock()
	defer at.mirrorSafetyMu.Unlock()
	for k := range at.mirrorWebHighPnLSkipKeys {
		if _, ok := masterTarget[k]; !ok {
			delete(at.mirrorWebHighPnLSkipKeys, k)
		}
	}
}

// mirrorWebShouldSkipHighPnLOpen 网页镜像被控启动对齐时：主控浮盈则不市价追已有仓（静默，等下一新仓）；浮亏或持平可进场。
func (at *AutoTrader) mirrorWebShouldSkipHighPnLOpen(k string, wire *comkunMasterStateWire, webStartupAlign bool, fq float64) bool {
	if at == nil || wire == nil {
		return false
	}
	at.mirrorSafetyMu.Lock()
	if at.mirrorWebHighPnLSkipKeys == nil {
		at.mirrorWebHighPnLSkipKeys = make(map[string]bool)
	}
	if at.mirrorWebHighPnLSkipKeys[k] {
		at.mirrorSafetyMu.Unlock()
		return true
	}
	at.mirrorSafetyMu.Unlock()

	if !webStartupAlign || fq >= qtyEps(0) {
		return false
	}
	sym, side, ok := splitPosKey(k)
	if !ok {
		return false
	}
	upnl, ok2 := masterWirePositionUnrealizedPnL(wire, sym, side)
	if !ok2 || upnl <= mirrorWebStartupPnLEps {
		return false
	}
	at.mirrorSafetyMu.Lock()
	at.mirrorWebHighPnLSkipKeys[k] = true
	at.mirrorSafetyMu.Unlock()
	logger.Infof("[%s] v2 网页镜像启动: 跳过 %s %s 浮盈 %.2f USDT，等待新仓",
		at.name, sym, side, upnl)
	return true
}

// masterWireHasAnyPositionOnSymbol 主控快照里该交易对是否仍有非零仓位（任一侧）。
// 用于区分：① 仓位接口滞后但 TP/SL 仍有效 → 暂缓镜像平被控；② 仓位已全平仅残留条件单/挂单竞态 → 不应无限期挡住被控跟平。
func masterWireHasAnyPositionOnSymbol(wire *comkunMasterStateWire, sym string) bool {
	if wire == nil {
		return false
	}
	su := strings.ToUpper(strings.TrimSpace(sym))
	for _, mp := range wire.Positions {
		if !strings.EqualFold(strings.TrimSpace(mp.Symbol), su) {
			continue
		}
		if mp.Quantity > 1e-12 {
			return true
		}
	}
	return false
}

// formatMirrorScanBriefReasoning 扫描币种行：事实数据 + 简要研判（被控限价决策卡与执行日志共用）。
func formatMirrorScanBriefReasoning(ctx *kernel.Context, symbol string, masterBrief map[string]*ComkunSymbolVolumeOI) string {
	const title = "成交量与 OI 分析"
	resolved := resolveMirrorVolumeOIBriefForSymbol(symbol, masterBrief, ctx)
	parts := appendComkunMasterVolumeOIParts(resolved)
	facts := strings.Join(parts, " · ")
	interp := strings.TrimSpace(interpretMirrorVolumeOIForZH(resolved))
	if facts == "" {
		if interp != "" && !strings.HasPrefix(interp, "当前缺少") {
			return title + "：" + interp
		}
		return title + "：本端暂无量能/OI 明细，结论见上方简报。"
	}
	out := title + " · " + facts
	if interp != "" {
		out += "\n【研判】" + interp
	}
	return out
}

// buildMirrorDisplayDecisionsFromWire 从主控快照中的限价类挂单生成「仅供展示」的决策 JSON，
// 与镜像下单同一套「保证金占主控权益比例」公式。
func buildMirrorDisplayDecisionsFromWire(wire comkunMasterStateWire, masterEq, followerEq float64, masterLev, followLev int, ctx *kernel.Context, masterVolumeOI map[string]*ComkunSymbolVolumeOI) []kernel.Decision {
	pend := wire.PendingOrders
	if len(pend) == 0 {
		return nil
	}
	out := make([]kernel.Decision, 0)
	for _, o := range pend {
		if !kernel.IsLimitLikePendingOrderType(o.Type) {
			continue
		}
		px := kernel.PendingLimitDisplayPrice(o)
		sym := strings.TrimSpace(o.Symbol)
		if sym == "" || px <= 0 || o.Quantity <= 0 {
			continue
		}
		ps := mirrorLimitPositionSide(o, wire.Positions)
		act := mapMirrorLimitToDisplayAction(strings.ToUpper(strings.TrimSpace(o.Side)), ps)
		if act == "wait" {
			continue
		}
		sl, tp := mirrorResolveSLTPFromWire(&wire, sym, strings.ToLower(ps))
		qty := mirrorScaledLimitQtyCapped(o, wire.Positions, masterEq, followerEq, masterLev, followLev)
		if qty <= 0 && masterEq > 1e-9 {
			ratio := followerEq / masterEq
			if ratio > 0 && !math.IsNaN(ratio) && !math.IsInf(ratio, 0) {
				qty = o.Quantity * ratio
			}
		}
		notional := qty * px
		scan := formatMirrorScanBriefReasoning(ctx, sym, masterVolumeOI)
		limitLine := formatMirrorPostAnalysisLimitLine(sym, px, act)
		out = append(out, kernel.Decision{
			Symbol:          sym,
			Action:          act,
			Leverage:        followLev,
			PositionSizeUSD: notional,
			Price:           px,
			StopLoss:        sl,
			TakeProfit:      tp,
			OrderID:         comkunDisplayOrderIDForPending(o, act),
			Reasoning:       strings.TrimSpace(scan + "\n" + limitLine),
		})
	}
	return out
}

func qtyEps(q float64) float64 {
	return math.Max(1e-8, math.Abs(q)*1e-5)
}

// appendMirrorLimitVolumeOIExecutionLog 镜像限价下单前输出量能/OI 事实与研判，便于决策详情里查看。
func appendMirrorLimitVolumeOIExecutionLog(record *store.DecisionRecord, sym string, wire *comkunMasterStateWire, ctx *kernel.Context) {
	if record == nil || wire == nil {
		return
	}
	b := resolveMirrorVolumeOIBriefForSymbol(sym, wire.VolumeOIBrief, ctx)
	parts := appendComkunMasterVolumeOIParts(b)
	facts := strings.Join(parts, " · ")
	interp := strings.TrimSpace(interpretMirrorVolumeOIForZH(b))
	if facts != "" {
		record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("📊 镜像限价 %s 量能/OI：%s", sym, facts))
	}
	if interp != "" {
		record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("📊 镜像限价 %s 研判：%s", sym, interp))
	}
}

// followerAvailableUSDT 从 GetBalance 解析可用 USDT（各所 map 字段一致部分）。
func followerAvailableUSDT(at *AutoTrader) float64 {
	if at == nil || at.trader == nil {
		return 0
	}
	bal, err := at.trader.GetBalance()
	if err != nil || bal == nil {
		return 0
	}
	v, ok := bal["availableBalance"].(float64)
	if !ok || v < 0 {
		return 0
	}
	return v
}

// clampMirrorScaledQtyByAvailable 将按比例缩放后的限价数量压到「可用保证金粗算」上限，减轻币安 -2019。
// 不减仓单（reduceOnly）一般不占新开仓保证金，不压缩。
func clampMirrorScaledQtyByAvailable(at *AutoTrader, symbol string, price, scaledQty float64, leverage int, reduceOnly bool, record *store.DecisionRecord) float64 {
	if reduceOnly || scaledQty <= 0 || price <= 0 {
		return scaledQty
	}
	if leverage < 1 {
		leverage = 1
	}
	avail := followerAvailableUSDT(at)
	if avail < 1e-6 {
		return scaledQty
	}
	// 名义 ≈ qty*price；线性合约初始保证金约≈名义/杠杆。系数留足余量（维持保证金、手续费、步进精度上浮）。
	maxNotional := avail * float64(leverage) * 0.82
	maxQty := maxNotional / price
	if maxQty <= 0 {
		return scaledQty
	}
	if scaledQty <= maxQty+1e-12 {
		return scaledQty
	}
	clamped := maxQty
	if record != nil {
		record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf(
			"订单同步: %s 限价数量 %.6f → %.6f（可用约 %.2f USDT、杠杆 %dx 估算上限，减轻保证金不足）",
			symbol, scaledQty, clamped, avail, leverage))
	}
	return clamped
}

func mirrorMinNotionalUSDT(at *AutoTrader, symbol string) float64 {
	if ft, ok := at.trader.(*binance.FuturesTrader); ok {
		n := ft.GetMinNotional(symbol)
		if n > 0 {
			return n
		}
	}
	return 5
}

// mirrorWebBootstrapOpenQty 网页镜像：缩放目标名义低于交易所最小时，按最小名义推算可下单张数（向上取整到 stepSize，保证格式化后仍 ≥ minNotional）。
func mirrorWebBootstrapOpenQty(at *AutoTrader, symbol string, price, minNotional float64) float64 {
	if at == nil || price <= 1e-12 || minNotional <= 0 {
		return 0
	}
	step := 0.001
	ft, hasFT := at.trader.(*binance.FuturesTrader)
	if hasFT {
		if s := ft.GetLotStepSize(symbol); s > 0 {
			step = s
		}
	}
	raw := minNotional / price
	qty := math.Ceil((raw-1e-12)/step) * step
	if hasFT {
		if s, err := ft.FormatQuantity(symbol, qty); err == nil {
			if q, err := strconv.ParseFloat(s, 64); err == nil && q > 0 {
				qty = q
			}
		}
	}
	for i := 0; i < 32 && qty*price+1e-9 < minNotional; i++ {
		qty += step
		if hasFT {
			if s, err := ft.FormatQuantity(symbol, qty); err == nil {
				if q, err := strconv.ParseFloat(s, 64); err == nil && q > 0 {
					qty = q
				}
			}
		}
	}
	return qty
}

func (at *AutoTrader) mirrorSafetyInitStreakMaps() {
	if at == nil {
		return
	}
	if at.mirrorMasterFlatStreak == nil {
		at.mirrorMasterFlatStreak = make(map[string]int)
	}
	if at.mirrorLastMirrorMarketAt == nil {
		at.mirrorLastMirrorMarketAt = make(map[string]time.Time)
	}
}

func resultOrderIDString(res map[string]interface{}) string {
	if res == nil {
		return "未知"
	}
	for _, key := range []string{"orderId", "order_id", "id"} {
		if v, ok := res[key]; ok && v != nil {
			return fmt.Sprintf("%v", v)
		}
	}
	return "未知"
}

// masterWireSymbolSet 主控快照里出现过的交易对（持仓或任意挂单），用于判断被控残留挂单是否与本轮镜像相关。
func masterWireSymbolSet(wire *comkunMasterStateWire) map[string]struct{} {
	out := make(map[string]struct{})
	if wire == nil {
		return out
	}
	for _, p := range wire.Positions {
		if s := strings.TrimSpace(p.Symbol); s != "" {
			out[s] = struct{}{}
		}
	}
	for _, o := range wire.PendingOrders {
		if s := strings.TrimSpace(o.Symbol); s != "" {
			out[s] = struct{}{}
		}
	}
	return out
}

// mirrorResyncOrderSymbols 本轮需要在被控上「先撤光再按主控重挂」的币对：
// 含主控有持仓、有开仓限价、有止盈/止损/追踪等条件单的币对；并并入「主控快照相关币对」上被控仍有的任意挂单，避免只撤 LIMIT 而残留 TAKE_PROFIT_MARKET 等。
func mirrorResyncOrderSymbols(at *AutoTrader, wire *comkunMasterStateWire) map[string]struct{} {
	resync := make(map[string]struct{})
	add := func(s string) {
		if t := strings.TrimSpace(s); t != "" {
			resync[t] = struct{}{}
		}
	}
	if wire == nil {
		return resync
	}
	for _, mp := range wire.Positions {
		if math.Abs(mp.Quantity) > 1e-12 {
			add(mp.Symbol)
		}
	}
	for _, o := range wire.PendingOrders {
		sym := strings.TrimSpace(o.Symbol)
		if sym == "" {
			continue
		}
		typ := strings.ToUpper(strings.TrimSpace(o.Type))
		if kernel.IsConditionalTakeProfitOrderType(typ) || kernel.IsConditionalStopLossOrderType(typ) {
			if kernel.PendingOrderTriggerPrice(o) > 0 {
				add(sym)
			}
			continue
		}
		if kernel.IsLimitLikePendingOrderType(typ) {
			px := kernel.PendingLimitDisplayPrice(o)
			if px > 0 && o.Quantity > 0 {
				add(sym)
			}
		}
	}
	ms := masterWireSymbolSet(wire)
	if folOrds, err := at.trader.GetOpenOrders(""); err != nil {
		logger.Infof("镜像: 读取被控全账户挂单(并入撤单范围): %v", err)
	} else {
		for _, o := range folOrds {
			s := strings.TrimSpace(o.Symbol)
			if s == "" {
				continue
			}
			if len(ms) == 0 {
				resync[s] = struct{}{}
				continue
			}
			if _, ok := ms[s]; ok {
				resync[s] = struct{}{}
			}
		}
	}
	return resync
}

// mirrorResolveSLTPFromWire 止盈止损触发价：优先主控 pending_orders；缺一侧时用 positions.stop_loss / take_profit
// （主控发广播前经 kernel.EnrichPositionsSLTPFromPending 写入，便于跟单端挂单列表偶发不全时仍能挂 SL/TP）。
func mirrorResolveSLTPFromWire(wire *comkunMasterStateWire, sym string, sideLower string) (sl, tp float64) {
	if wire == nil {
		return 0, 0
	}
	ps := strings.ToUpper(strings.TrimSpace(sideLower))
	sl, tp = kernel.FindSLTPFromPendingOrders(wire.PendingOrders, sym, ps)
	if sl > 0 && tp > 0 {
		return sl, tp
	}
	su := strings.ToUpper(strings.TrimSpace(sym))
	sd := strings.ToLower(strings.TrimSpace(sideLower))
	for _, p := range wire.Positions {
		if strings.ToUpper(strings.TrimSpace(p.Symbol)) != su || strings.ToLower(strings.TrimSpace(p.Side)) != sd {
			continue
		}
		if sl <= 0 && p.StopLoss > 0 {
			sl = p.StopLoss
		}
		if tp <= 0 && p.TakeProfit > 0 {
			tp = p.TakeProfit
		}
		break
	}
	return sl, tp
}
