package api

import (
	"math"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"

	"nofx/store"

	"github.com/gin-gonic/gin"
)

// 策略市场列表/详情演示叠加：匹配名称子串时注入虚拟汇总（环境变量 NOFX_MARKET_DETAIL_DEMO）。
//
// 相关环境变量：
//
//	NOFX_MARKET_DETAIL_DEMO=1
//	NOFX_MARKET_DETAIL_DEMO_NAME_SUBSTR=究极SOL,超级小亮,静默猎手   （逗号分隔）

func marketDetailDemoEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("NOFX_MARKET_DETAIL_DEMO")))
	switch v {
	case "1", "true", "yes", "on", "demo":
		return true
	default:
		return false
	}
}

// marketDetailDemoNameSubstrList 名称子串列表；命中任一即叠加演示数据（多条策略并行，不是互斥替换）
func marketDetailDemoNameSubstrList() []string {
	raw := strings.TrimSpace(os.Getenv("NOFX_MARKET_DETAIL_DEMO_NAME_SUBSTR"))
	if raw == "" {
		return []string{"究极SOL", "超级小亮", "静默猎手", "静默测试", "测试策略勿跟", "测试02", "测试03", "测试04"}
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{"究极SOL", "超级小亮", "静默猎手", "静默测试", "测试策略勿跟", "测试02", "测试03", "测试04"}
	}
	return out
}

// marketDetailDemoNameMatchSummary 供 demo_overlay 元数据展示
func marketDetailDemoNameMatchSummary() string {
	return strings.Join(marketDetailDemoNameSubstrList(), ",")
}

func marketDetailDemoMatchesStrategy(name string) bool {
	n := strings.TrimSpace(name)
	if n == "" {
		return false
	}
	for _, sub := range marketDetailDemoNameSubstrList() {
		if strings.Contains(n, sub) {
			return true
		}
	}
	return false
}

func marketDemoIsSuperXiaoLiang(strategyName string) bool {
	return strings.Contains(strings.TrimSpace(strategyName), "超级小亮")
}

func marketDemoIsSilentHunter(strategyName string) bool {
	return strings.Contains(strings.TrimSpace(strategyName), "静默猎手")
}

func marketDemoIsSilentTest(strategyName string) bool {
	return strings.Contains(strings.TrimSpace(strategyName), "静默测试")
}

func marketDemoIsTestStrategyNoFollow(strategyName string) bool {
	return strings.Contains(strings.TrimSpace(strategyName), "测试策略勿跟")
}

func marketDemoIsTest02(strategyName string) bool {
	n := strings.TrimSpace(strategyName)
	if n == "" {
		return false
	}
	// 避免「测试02（已购）」等副本名称误命中市场演示
	if strings.Contains(n, "已购") {
		return false
	}
	return strings.Contains(n, "测试02")
}

func marketDemoIsTest03(strategyName string) bool {
	n := strings.TrimSpace(strategyName)
	if n == "" {
		return false
	}
	if strings.Contains(n, "已购") {
		return false
	}
	return strings.Contains(n, "测试03")
}

func marketDemoIsTest04(strategyName string) bool {
	n := strings.TrimSpace(strategyName)
	if n == "" {
		return false
	}
	if strings.Contains(n, "已购") {
		return false
	}
	return strings.Contains(n, "测试04")
}

// 演示曲线：约 45 日窗口 + 45 个净值节点（波动更明显）
const demoCurveWindowDays = 45
const demoTrendPoints = 45

// marketDemoAudienceStats 订阅 / 运行中 Agent：按策略名区分
func marketDemoAudienceStats(strategyName string) (subscribers int, runningAgents int, usedBy int) {
	n := strings.TrimSpace(strategyName)
	if strings.Contains(n, "测试04") && !strings.Contains(n, "已购") {
		return 14, 7, 14
	}
	if strings.Contains(n, "测试03") && !strings.Contains(n, "已购") {
		return 9, 5, 9
	}
	if strings.Contains(n, "测试02") && !strings.Contains(n, "已购") {
		return 12, 6, 12
	}
	if strings.Contains(n, "测试策略勿跟") {
		return 18, 9, 18
	}
	if strings.Contains(n, "静默测试") {
		return 35, 19, 35
	}
	if strings.Contains(n, "超级小亮") {
		return 48, 25, 48
	}
	if strings.Contains(n, "静默猎手") {
		return 38, 21, 38
	}
	if strings.Contains(n, "究极SOL") {
		return 82, 41, 82
	}
	return 82, 41, 82
}

// —— 究极 SOL：按导入历史做单重新计算，策略市场管理资产保持原展示值 ——
const (
	demoInitialCap     = 20000.0
	demoReturn7dPct    = 324.14
	demoTotalAUM       = 187725.0
	demoMaxDrawdownPct = 6.03
)

const (
	demoGrossProfit = 97733.0
	demoGrossLoss   = 32905.8
	demoTotalPnL    = demoGrossProfit - demoGrossLoss
	demoTotalFee    = 0.0
)

// —— 超级小亮：与「运行数据」截图一致（金额带分位闭合） ——
const (
	xlInitialCap     = 10000.0
	xlReturnPct      = 352.11   // 展示用收益率 %
	xlTotalPnL       = 35210.73 // ≈ 本金 × 352.11%，与毛利−毛亏闭合
	xlGrossProfit    = 59634.28 // ≈ 截图 59.63K
	xlGrossLoss      = 24423.55 // ≈ 截图 24.42K；毛利−毛亏 = xlTotalPnL
	xlTotalFee       = 0.0      // 截图总费用 0
	xlSharpe         = 3.70
	xlMaxDrawdownPct = 17.85
	xlTotalTrades    = 109
	xlWinTrades      = 58
	xlLossTrades     = 51
	xlWinRatePct     = 53.21
	xlLongTrades     = 54
	xlShortTrades    = 55
	xlAvgHoldMs      = 126000000.0               // (24+11)h → ms，对应「1D 11h」
	xlEndEquity      = xlInitialCap + xlTotalPnL // 列表 total_aum / 曲线终点
)

// —— 静默猎手：与「运行数据」截图一致 ——
const (
	shInitialCap     = 10000.0
	shReturnPct      = 169.36
	shTotalPnL       = 16936.0 // 169.36% × 本金；与毛利−毛亏闭合
	shGrossProfit    = 20310.0
	shGrossLoss      = 3374.0 // 20310 - 3374 = 16936
	shTotalFee       = 0.0
	shSharpe         = 2.99
	shMaxDrawdownPct = 17.58
	shTotalTrades    = 19
	shWinTrades      = 10
	shLossTrades     = 9
	shWinRatePct     = 52.63
	shLongTrades     = 10
	shShortTrades    = 9
	shAvgHoldMs      = float64((24 + 10) * 3600 * 1000) // 1D 10h → 34h
	shEndEquity      = shInitialCap + shTotalPnL
)

// —— 静默测试：展示用「从今天 UTC 往前推 30 天」滚动窗口、100 笔、胜率 75.89%、收益率闭合 ——
const (
	stestInitialCap      = 10000.0
	stestReturnPct       = 236.47
	stestTotalPnL        = 23647.0 // 本金 × 236.47%；与毛利−毛亏闭合
	stestGrossProfit     = 27100.0
	stestGrossLoss       = 3453.0 // 27100 − 3453 = 23647
	stestTotalFee        = 418.35
	stestSharpe          = 3.42
	stestMaxDrawdownPct  = 13.65
	stestTotalTrades     = 100
	stestWinTrades       = 76
	stestLossTrades      = 24
	stestWinRatePct      = 75.89
	stestLongTrades      = 54
	stestShortTrades     = 46
	stestAvgHoldMs       = float64((14*3600 + 27*60) * 1000)
	stestEndEquity       = stestInitialCap + stestTotalPnL
	stestCurveWindowDays = 30 // 统计区间长度（自然日）；起点始终为「当前时刻 −30 天」
	stestTrendPoints     = 30
)

// —— 测试策略勿跟（bn-screen-mirror-ec92c8f5）：营销展示指标（收益率与盈亏金额按产品约定可分开展示） ——
const (
	tnfInitialCap      = 100000.0
	tnfReturnPct       = 4560.54
	tnfTotalPnL        = 22802.70
	tnfGrossProfit     = 25549.10 // 61 胜 × avg_win；与毛亏闭合 total PnL
	tnfGrossLoss       = 2746.40  // 20 负 × avg_loss
	tnfTotalFee        = 892.37
	tnfSharpe          = 4.18 // 90d 窗口：由收益率/回撤量级估算的展示用夏普
	tnfMaxDrawdownPct  = 8.52
	tnfTotalTrades     = 81
	tnfWinTrades       = 61
	tnfLossTrades      = 20
	tnfWinRatePct      = 75.31
	tnfLongTrades      = 32 // 32/81≈39.51%，49/81≈60.49%（多空比约 0.653）
	tnfShortTrades     = 49
	tnfAvgHoldMs       = float64((18*3600 + 42*60) * 1000)
	tnfEndEquity       = tnfInitialCap + tnfTotalPnL
	tnfCurveWindowDays = 90
	tnfTrendPoints     = 45
)

// —— 测试02（bn-screen-mirror-test02-5a3b7c1d）：营销展示指标 ——
const (
	t02InitialCap      = 25000.0
	t02ReturnPct       = 70.61
	t02TotalPnL        = 18940.92
	t02GrossProfit     = 23307.72 // 14 胜；毛利−毛亏 = t02TotalPnL
	t02GrossLoss       = 4366.80  // 8 负
	t02TotalFee        = 231.58   // 22 笔 × ~10.5 USDT（25k 本金、合约双边 taker 量级估算）
	t02Sharpe          = 0.61
	t02MaxDrawdownPct  = 14.95
	t02TotalTrades     = 22
	t02WinTrades       = 14
	t02LossTrades      = 8
	t02WinRatePct      = 62.50
	t02LongTrades      = 15 // 15:7≈68.18%（22 笔整数下最接近 7:3）；展示百分比由前端按笔数计算
	t02ShortTrades     = 7
	t02AvgHoldMs       = float64((12*3600 + 18*60) * 1000)
	t02EndEquity       = t02InitialCap + t02TotalPnL
	t02CurveWindowDays = 90
	t02TrendPoints     = 45
)

// —— 测试03（bn-screen-mirror-test03-8c402b25）：营销展示指标 ——
const (
	t03InitialCap      = 5000.0
	t03ReturnPct       = 401.05
	t03TotalPnL        = 15480.36
	t03GrossProfit     = 17415.44 // 28 胜；毛利−毛亏 = t03TotalPnL
	t03GrossLoss       = 1935.08  // 4 负
	t03TotalFee        = 168.35   // 33 笔 × ~5.1 USDT（5k 本金、由 test02 22 笔费用按比例估算）
	t03Sharpe          = 10.14
	t03MaxDrawdownPct  = 10.16
	t03TotalTrades     = 33
	t03WinTrades       = 28
	t03LossTrades      = 4
	t03WinRatePct      = 87.50
	t03LongTrades      = 26 // 26/33≈78.79%（33 笔整数下最接近 8:2 的 79.38%/20.62% 展示）
	t03ShortTrades     = 7
	t03AvgHoldMs       = float64((16*3600 + 35*60) * 1000)
	t03EndEquity       = t03InitialCap + t03TotalPnL
	t03CurveWindowDays = 180
	t03TrendPoints     = 45
)

// —— 测试04（bn-screen-mirror-test04-2560c95f）：营销展示指标 ——
const (
	t04InitialCap      = 35000.0
	t04ReturnPct       = 269.88
	t04TotalPnL        = 80251.64
	t04GrossProfit     = 98742.80 // 147 胜；毛利−毛亏 = t04TotalPnL（与 test02 盈亏比量级闭合）
	t04GrossLoss       = 18491.16 // 33 负
	t04TotalFee        = 1980.45  // 180 笔 × ~11 USDT（35k 本金、合约双边费用估算）
	t04Sharpe          = 4.67
	t04MaxDrawdownPct  = 9.86
	t04TotalTrades     = 180
	t04WinTrades       = 147
	t04LossTrades      = 33
	t04WinRatePct      = 81.67
	t04LongTrades      = 92 // 92/180≈51.11%，88/180≈48.89%（5:5 略偏多，接近 51.37%/48.63%）
	t04ShortTrades     = 88
	t04AvgHoldMs       = float64((14*3600 + 22*60) * 1000)
	t04EndEquity       = t04InitialCap + t04TotalPnL
	t04CurveWindowDays = 90
	t04TrendPoints     = 45
)

// silentTestWindowStartUTC 演示区间起点：当前时间往前推整整 stestCurveWindowDays 天（滚动一月，随请求日变化）
func silentTestWindowStartUTC() time.Time {
	return time.Now().UTC().AddDate(0, 0, -stestCurveWindowDays)
}

// demoMarketRevisionFloat 版本号展示为小数点后 1 位（在整数 revision 基础上 +0.4 再四舍五入）
func demoMarketRevisionFloat(rev uint) float64 {
	br := float64(rev)
	if br < 1 {
		br = 1
	}
	return math.Round((br+0.4)*10) / 10
}

// gaussianValley 在 t∈[0,1] 上生成一段向下凹的「亏损区间」（钟形，center 为谷底大致位置）
func gaussianValley(t, center, width, depth float64) float64 {
	d := (t - center) / width
	return -depth * math.Exp(-d*d)
}

// marketDemoSeedFromString 由 strategy_id + profile 生成可复现种子（各行曲线不同）
func marketDemoSeedFromString(s string) int64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return int64(h & 0x7fffffffffffffff)
}

// marketDemoSeriesMaxDrawdownPct 沿序列的峰值回撤百分比
func marketDemoSeriesMaxDrawdownPct(vals []float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	peak := vals[0]
	maxDD := 0.0
	for _, v := range vals {
		if v > peak {
			peak = v
		}
		if peak > 0 {
			dd := (peak - v) / peak * 100
			if dd > maxDD {
				maxDD = dd
			}
		}
	}
	return maxDD
}

// marketDemoScaleToEndpoints 从 initial 锚点缩放至 end，比全段线性归一化更能保留中段回撤
func marketDemoScaleToEndpoints(out []float64, initial, end float64) {
	if len(out) < 2 {
		return
	}
	span := end - initial
	denom := out[len(out)-1] - initial
	if math.Abs(denom) < 1e-9 {
		denom = span
		if math.Abs(denom) < 1e-9 {
			denom = 1
		}
	}
	k := span / denom
	for i := 1; i < len(out)-1; i++ {
		out[i] = initial + (out[i]-initial)*k
	}
	out[0] = initial
	out[len(out)-1] = end
}

// marketDemoSeededEquityTrend 不规则净值走势：种子决定形态，终点对齐 initial+PnL，回撤贴近 maxDrawdownPct
func marketDemoSeededEquityTrend(seedKey string, initial, end float64, n int, maxDrawdownPct float64) []float64 {
	if n < 2 {
		n = demoTrendPoints
	}
	if maxDrawdownPct < 0.5 {
		maxDrawdownPct = 10
	}
	if initial <= 0 {
		initial = 1
	}
	if end <= 0 {
		end = initial * 1.05
	}
	rng := rand.New(rand.NewSource(marketDemoSeedFromString(seedKey)))
	span := end - initial
	targetDD := maxDrawdownPct / 100.0
	vol := 0.55 + rng.Float64()*0.9

	stepCap := math.Max(span*0.12, initial*0.04)
	deltas := make([]float64, n-1)
	sum := 0.0
	for i := range deltas {
		mag := span / float64(n-1) * (0.35 + rng.Float64()*1.25) * vol
		if rng.Float64() < 0.42 {
			mag = -mag * (0.55 + rng.Float64()*0.75)
		}
		if i%5 == 0 && rng.Float64() < 0.22 {
			mag *= -(1.2 + rng.Float64()*0.5)
		}
		if mag > stepCap {
			mag = stepCap
		}
		if mag < -stepCap {
			mag = -stepCap
		}
		deltas[i] = mag
		sum += mag
	}
	if math.Abs(sum) < 1e-9 {
		deltas[0] = span
		sum = span
	}
	scale := span / sum
	for i := range deltas {
		deltas[i] *= scale
	}

	out := make([]float64, n)
	out[0] = initial
	for i := 1; i < n; i++ {
		out[i] = out[i-1] + deltas[i-1]
	}

	numDips := 2 + rng.Intn(2)
	for d := 0; d < numDips; d++ {
		idx := 4 + rng.Intn(n-8)
		peak := out[0]
		for i := 1; i < idx; i++ {
			if out[i] > peak {
				peak = out[i]
			}
		}
		want := peak * (1.0 - targetDD*(0.88+rng.Float64()*0.12))
		if out[idx] > want {
			out[idx] = want
			for k := 1; k <= 2; k++ {
				if idx-k >= 0 {
					blend := 0.3 + 0.12*float64(k)
					out[idx-k] = out[idx-k]*(1-blend) + want*blend
				}
				if idx+k < n {
					blend := 0.3 + 0.12*float64(k)
					out[idx+k] = out[idx+k]*(1-blend) + want*blend
				}
			}
		}
	}
	marketDemoScaleToEndpoints(out, initial, end)

	for attempt := 0; attempt < 3; attempt++ {
		if dd := marketDemoSeriesMaxDrawdownPct(out); dd >= maxDrawdownPct*0.88 {
			break
		}
		troughIdx := 1
		bestDD := 0.0
		runPeak := out[0]
		for i, v := range out {
			if v > runPeak {
				runPeak = v
			}
			if runPeak > 0 {
				dd := (runPeak - v) / runPeak
				if dd > bestDD {
					bestDD = dd
					troughIdx = i
				}
			}
		}
		if runPeak <= 0 {
			break
		}
		want := runPeak * (1.0 - targetDD)
		if out[troughIdx] > want {
			out[troughIdx] = want
			for k := 1; k <= 2; k++ {
				if troughIdx-k > 0 {
					blend := 0.28 + 0.12*float64(k)
					out[troughIdx-k] = out[troughIdx-k]*(1-blend) + want*blend
				}
				if troughIdx+k < n-1 {
					blend := 0.28 + 0.12*float64(k)
					out[troughIdx+k] = out[troughIdx+k]*(1-blend) + want*blend
				}
			}
		}
		marketDemoScaleToEndpoints(out, initial, end)
	}
	floor := initial * (1.0 - targetDD*1.15)
	for i := 1; i < n-1; i++ {
		if out[i] < floor {
			out[i] = floor + rng.Float64()*initial*0.006
		}
	}
	marketDemoScaleToEndpoints(out, initial, end)

	for i := range out {
		jitter := (rng.Float64()*0.97 - 0.41) * 0.01 * math.Max(1, math.Abs(out[i])*0.002)
		out[i] = math.Round((out[i]+jitter)*100) / 100
	}
	out[0] = math.Round(initial*100) / 100
	out[n-1] = math.Round(end*100) / 100
	return out
}

func marketDemoTrendSeed(profile, strategyID string) string {
	p := strings.TrimSpace(profile)
	sid := strings.TrimSpace(strategyID)
	if sid == "" {
		return "demo-trend:" + p
	}
	return "demo-trend:" + p + "|" + sid
}

// marketDemoWavyEquityTrend 曲折净值序列（究极SOL 等）
func marketDemoWavyEquityTrend(strategyID string, initial, end float64, n int) []float64 {
	if n < 2 {
		n = demoTrendPoints
	}
	return marketDemoSeededEquityTrend(marketDemoTrendSeed("jijis-sol", strategyID), initial, end, n, demoMaxDrawdownPct)
}

func marketDemoWavyEquityTrendXiaoLiang(strategyID string, initial, end float64, n int) []float64 {
	return marketDemoSeededEquityTrend(marketDemoTrendSeed("super-xiaoliang", strategyID), initial, end, n, xlMaxDrawdownPct)
}

func marketDemoWavyEquityTrendSilentHunter(strategyID string, initial, end float64, n int) []float64 {
	return marketDemoSeededEquityTrend(marketDemoTrendSeed("silent-hunter", strategyID), initial, end, n, shMaxDrawdownPct)
}

func marketDemoWavyEquityTrendSilentTest(strategyID string, initial, end float64, n int) []float64 {
	if n < 2 {
		n = stestTrendPoints
	}
	return marketDemoSeededEquityTrend(marketDemoTrendSeed("silent-test", strategyID), initial, end, n, stestMaxDrawdownPct)
}

func marketDemoWavyEquityTrendTestStrategyNoFollow(strategyID string, initial, end float64, n int) []float64 {
	if n < 2 {
		n = tnfTrendPoints
	}
	return marketDemoSeededEquityTrend(marketDemoTrendSeed("ec92-no-follow", strategyID), initial, end, n, tnfMaxDrawdownPct)
}

func marketDemoWavyEquityTrendTest02(strategyID string, initial, end float64, n int) []float64 {
	if n < 2 {
		n = t02TrendPoints
	}
	return marketDemoSeededEquityTrend(marketDemoTrendSeed("test02", strategyID), initial, end, n, t02MaxDrawdownPct)
}

func marketDemoWavyEquityTrendTest03(strategyID string, initial, end float64, n int) []float64 {
	if n < 2 {
		n = t03TrendPoints
	}
	return marketDemoSeededEquityTrend(marketDemoTrendSeed("test03", strategyID), initial, end, n, t03MaxDrawdownPct)
}

func marketDemoWavyEquityTrendTest04(strategyID string, initial, end float64, n int) []float64 {
	if n < 2 {
		n = t04TrendPoints
	}
	return marketDemoSeededEquityTrend(marketDemoTrendSeed("test04", strategyID), initial, end, n, t04MaxDrawdownPct)
}

// marketDetailDemoRollup 究极 SOL 详情 aggregate_trading
func marketDetailDemoRollup() *store.AggregatedTradingRollup {
	const (
		totalTrades    = 87
		winRatePct     = 73.56
		maxDrawdownPct = demoMaxDrawdownPct
		winTrades      = 64
		lossTrades     = 23
		longTrades     = 51
		shortTrades    = 36
		avgHoldMs      = 34751873.563
	)

	totalPnL := demoTotalPnL
	grossLoss := demoGrossLoss
	grossProfit := demoGrossProfit
	pf := grossProfit / grossLoss
	out := &store.AggregatedTradingRollup{
		AvgHoldMs:   avgHoldMs,
		GrossProfit: grossProfit,
		GrossLoss:   grossLoss,
		LongTrades:  longTrades,
		ShortTrades: shortTrades,
	}
	out.Stats.TotalTrades = totalTrades
	out.Stats.WinTrades = winTrades
	out.Stats.LossTrades = lossTrades
	out.Stats.WinRate = winRatePct
	out.Stats.ProfitFactor = math.Round(pf*100) / 100
	out.Stats.SharpeRatio = 4.04
	out.Stats.TotalPnL = totalPnL
	out.Stats.TotalFee = demoTotalFee
	if winTrades > 0 {
		out.Stats.AvgWin = grossProfit / float64(winTrades)
	}
	if lossTrades > 0 {
		out.Stats.AvgLoss = grossLoss / float64(lossTrades)
	}
	out.Stats.MaxDrawdownPct = maxDrawdownPct
	return out
}

// marketDetailDemoRollupSuperXiaoLiang 超级小亮：与截图运行数据一致
func marketDetailDemoRollupSuperXiaoLiang() *store.AggregatedTradingRollup {
	pf := xlGrossProfit / xlGrossLoss
	out := &store.AggregatedTradingRollup{
		AvgHoldMs:   xlAvgHoldMs,
		GrossProfit: xlGrossProfit,
		GrossLoss:   xlGrossLoss,
		LongTrades:  xlLongTrades,
		ShortTrades: xlShortTrades,
	}
	out.Stats.TotalTrades = xlTotalTrades
	out.Stats.WinTrades = xlWinTrades
	out.Stats.LossTrades = xlLossTrades
	out.Stats.WinRate = xlWinRatePct
	out.Stats.ProfitFactor = math.Round(pf*100) / 100
	out.Stats.SharpeRatio = xlSharpe
	out.Stats.TotalPnL = xlTotalPnL
	out.Stats.TotalFee = xlTotalFee
	if xlWinTrades > 0 {
		out.Stats.AvgWin = xlGrossProfit / float64(xlWinTrades)
	}
	if xlLossTrades > 0 {
		out.Stats.AvgLoss = xlGrossLoss / float64(xlLossTrades)
	}
	out.Stats.MaxDrawdownPct = xlMaxDrawdownPct
	return out
}

// marketDetailDemoRollupSilentHunter 静默猎手
func marketDetailDemoRollupSilentHunter() *store.AggregatedTradingRollup {
	pf := shGrossProfit / shGrossLoss
	out := &store.AggregatedTradingRollup{
		AvgHoldMs:   shAvgHoldMs,
		GrossProfit: shGrossProfit,
		GrossLoss:   shGrossLoss,
		LongTrades:  shLongTrades,
		ShortTrades: shShortTrades,
	}
	out.Stats.TotalTrades = shTotalTrades
	out.Stats.WinTrades = shWinTrades
	out.Stats.LossTrades = shLossTrades
	out.Stats.WinRate = shWinRatePct
	out.Stats.ProfitFactor = math.Round(pf*100) / 100
	out.Stats.SharpeRatio = shSharpe
	out.Stats.TotalPnL = shTotalPnL
	out.Stats.TotalFee = shTotalFee
	if shWinTrades > 0 {
		out.Stats.AvgWin = shGrossProfit / float64(shWinTrades)
	}
	if shLossTrades > 0 {
		out.Stats.AvgLoss = shGrossLoss / float64(shLossTrades)
	}
	out.Stats.MaxDrawdownPct = shMaxDrawdownPct
	return out
}

// marketDetailDemoRollupTest03 测试03
func marketDetailDemoRollupTest03() *store.AggregatedTradingRollup {
	pf := t03GrossProfit / t03GrossLoss
	out := &store.AggregatedTradingRollup{
		AvgHoldMs:   t03AvgHoldMs,
		GrossProfit: t03GrossProfit,
		GrossLoss:   t03GrossLoss,
		LongTrades:  t03LongTrades,
		ShortTrades: t03ShortTrades,
	}
	out.Stats.TotalTrades = t03TotalTrades
	out.Stats.WinTrades = t03WinTrades
	out.Stats.LossTrades = t03LossTrades
	out.Stats.WinRate = t03WinRatePct
	out.Stats.ProfitFactor = math.Round(pf*100) / 100
	out.Stats.SharpeRatio = t03Sharpe
	out.Stats.TotalPnL = t03TotalPnL
	out.Stats.TotalFee = t03TotalFee
	if t03WinTrades > 0 {
		out.Stats.AvgWin = t03GrossProfit / float64(t03WinTrades)
	}
	if t03LossTrades > 0 {
		out.Stats.AvgLoss = t03GrossLoss / float64(t03LossTrades)
	}
	out.Stats.MaxDrawdownPct = t03MaxDrawdownPct
	return out
}

// marketDetailDemoRollupTest04 测试04
func marketDetailDemoRollupTest04() *store.AggregatedTradingRollup {
	pf := t04GrossProfit / t04GrossLoss
	out := &store.AggregatedTradingRollup{
		AvgHoldMs:   t04AvgHoldMs,
		GrossProfit: t04GrossProfit,
		GrossLoss:   t04GrossLoss,
		LongTrades:  t04LongTrades,
		ShortTrades: t04ShortTrades,
	}
	out.Stats.TotalTrades = t04TotalTrades
	out.Stats.WinTrades = t04WinTrades
	out.Stats.LossTrades = t04LossTrades
	out.Stats.WinRate = t04WinRatePct
	out.Stats.ProfitFactor = math.Round(pf*100) / 100
	out.Stats.SharpeRatio = t04Sharpe
	out.Stats.TotalPnL = t04TotalPnL
	out.Stats.TotalFee = t04TotalFee
	if t04WinTrades > 0 {
		out.Stats.AvgWin = t04GrossProfit / float64(t04WinTrades)
	}
	if t04LossTrades > 0 {
		out.Stats.AvgLoss = t04GrossLoss / float64(t04LossTrades)
	}
	out.Stats.MaxDrawdownPct = t04MaxDrawdownPct
	return out
}

// marketDetailDemoRollupTest02 测试02
func marketDetailDemoRollupTest02() *store.AggregatedTradingRollup {
	pf := t02GrossProfit / t02GrossLoss
	out := &store.AggregatedTradingRollup{
		AvgHoldMs:   t02AvgHoldMs,
		GrossProfit: t02GrossProfit,
		GrossLoss:   t02GrossLoss,
		LongTrades:  t02LongTrades,
		ShortTrades: t02ShortTrades,
	}
	out.Stats.TotalTrades = t02TotalTrades
	out.Stats.WinTrades = t02WinTrades
	out.Stats.LossTrades = t02LossTrades
	out.Stats.WinRate = t02WinRatePct
	out.Stats.ProfitFactor = math.Round(pf*100) / 100
	out.Stats.SharpeRatio = t02Sharpe
	out.Stats.TotalPnL = t02TotalPnL
	out.Stats.TotalFee = t02TotalFee
	if t02WinTrades > 0 {
		out.Stats.AvgWin = t02GrossProfit / float64(t02WinTrades)
	}
	if t02LossTrades > 0 {
		out.Stats.AvgLoss = t02GrossLoss / float64(t02LossTrades)
	}
	out.Stats.MaxDrawdownPct = t02MaxDrawdownPct
	return out
}

// marketDetailDemoRollupTestStrategyNoFollow 测试策略勿跟
func marketDetailDemoRollupTestStrategyNoFollow() *store.AggregatedTradingRollup {
	pf := tnfGrossProfit / tnfGrossLoss
	out := &store.AggregatedTradingRollup{
		AvgHoldMs:   tnfAvgHoldMs,
		GrossProfit: tnfGrossProfit,
		GrossLoss:   tnfGrossLoss,
		LongTrades:  tnfLongTrades,
		ShortTrades: tnfShortTrades,
	}
	out.Stats.TotalTrades = tnfTotalTrades
	out.Stats.WinTrades = tnfWinTrades
	out.Stats.LossTrades = tnfLossTrades
	out.Stats.WinRate = tnfWinRatePct
	out.Stats.ProfitFactor = math.Round(pf*100) / 100
	out.Stats.SharpeRatio = tnfSharpe
	out.Stats.TotalPnL = tnfTotalPnL
	out.Stats.TotalFee = tnfTotalFee
	if tnfWinTrades > 0 {
		out.Stats.AvgWin = tnfGrossProfit / float64(tnfWinTrades)
	}
	if tnfLossTrades > 0 {
		out.Stats.AvgLoss = tnfGrossLoss / float64(tnfLossTrades)
	}
	out.Stats.MaxDrawdownPct = tnfMaxDrawdownPct
	return out
}

// marketDetailDemoRollupSilentTest 静默测试
func marketDetailDemoRollupSilentTest() *store.AggregatedTradingRollup {
	pf := stestGrossProfit / stestGrossLoss
	out := &store.AggregatedTradingRollup{
		AvgHoldMs:   stestAvgHoldMs,
		GrossProfit: stestGrossProfit,
		GrossLoss:   stestGrossLoss,
		LongTrades:  stestLongTrades,
		ShortTrades: stestShortTrades,
	}
	out.Stats.TotalTrades = stestTotalTrades
	out.Stats.WinTrades = stestWinTrades
	out.Stats.LossTrades = stestLossTrades
	out.Stats.WinRate = stestWinRatePct
	out.Stats.ProfitFactor = math.Round(pf*100) / 100
	out.Stats.SharpeRatio = stestSharpe
	out.Stats.TotalPnL = stestTotalPnL
	out.Stats.TotalFee = stestTotalFee
	if stestWinTrades > 0 {
		out.Stats.AvgWin = stestGrossProfit / float64(stestWinTrades)
	}
	if stestLossTrades > 0 {
		out.Stats.AvgLoss = stestGrossLoss / float64(stestLossTrades)
	}
	out.Stats.MaxDrawdownPct = stestMaxDrawdownPct
	return out
}

func silentTestDistributeTotal(total float64, n int, seed int64) []float64 {
	if n <= 0 || total <= 0 {
		return nil
	}
	rng := rand.New(rand.NewSource(seed))
	out := make([]float64, n)
	var sumWeights float64
	weights := make([]float64, n)
	for i := 0; i < n; i++ {
		w := 0.82 + rng.Float64()*0.36
		weights[i] = w
		sumWeights += w
	}
	scale := total / sumWeights
	var running float64
	for i := 0; i < n; i++ {
		out[i] = math.Round(weights[i]*scale*100) / 100
		running += out[i]
	}
	if n > 0 {
		out[n-1] = math.Round((out[n-1]+(total-running))*100) / 100
	}
	return out
}

type silentRefPx struct {
	Symbol string
	Ref    float64
}

// 演示锚定价：对齐 Binance 现货公开市价量级（更新日期见 git / 部署时可改）。
// silentPriceAt 会在此基础上做小幅时序波动，避免出现「9 万 BTC、150 SOL」等脱离实盘区间的价位。
var silentTestSymbolRefs = []silentRefPx{
	{"BTCUSDT", 81076},
	{"ETHUSDT", 2334},
	{"SOLUSDT", 95.33},
	{"BNBUSDT", 653.71},
	{"XRPUSDT", 1.4515},
	{"DOGEUSDT", 0.11009},
	{"ADAUSDT", 0.2802},
	{"AVAXUSDT", 10.14},
	{"LINKUSDT", 10.59},
	{"DOTUSDT", 1.366},
}

func silentDemoBase(sym string) float64 {
	for _, r := range silentTestSymbolRefs {
		if r.Symbol == sym {
			return r.Ref
		}
	}
	return 1
}

func silentHash(sym string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(sym); i++ {
		h ^= uint32(sym[i])
		h *= 16777619
	}
	return h
}

// silentPriceAt 演示用「参考价」：随时间平滑波动，模拟近一两周日内振幅量级（不含极端跳空）
func silentPriceAt(sym string, t time.Time) float64 {
	base := silentDemoBase(sym)
	seed := float64(silentHash(sym)%2347) / 2347.0
	u := float64(t.UTC().Unix())
	h := math.Sin(u/3600.0*(2*math.Pi/19.0) + seed*6.283)
	d := math.Sin(u/86400.0*(2*math.Pi/6.2) + seed*3.7)
	m := math.Sin(u/480.0*(2*math.Pi/13.0) + seed*1.9)
	ampH := 0.0016
	ampD := 0.009
	ampM := 0.00035
	v := base * (1 + ampH*h + ampD*0.45*d + ampM*m)
	return math.Round(v*1e6) / 1e6
}

func silentMaxRelMove(sym string) float64 {
	switch {
	case strings.HasPrefix(sym, "BTC"):
		return 0.026
	case strings.HasPrefix(sym, "ETH"):
		return 0.03
	case strings.HasPrefix(sym, "SOL"), strings.HasPrefix(sym, "BNB"):
		return 0.038
	case strings.HasPrefix(sym, "XRP"), strings.HasPrefix(sym, "ADA"), strings.HasPrefix(sym, "DOGE"):
		return 0.055
	default:
		return 0.048
	}
}

type silentStagedTrade struct {
	tOpen  time.Time
	tClose time.Time
	symbol string
	side   string  // long | short
	target float64 // 含符号：盈利为正，亏损为负
}

// silentTestTradeHighlights 生成 100 笔演示成交：毛利/毛亏拆分闭合；开平仓时间为虚拟时刻，
// 价格由 silentPriceAt(币种,时刻) 给出，价差限制在日内合理区间，再用仓位数量对齐目标盈亏。
func silentTestTradeHighlights() []gin.H {
	wins := silentTestDistributeTotal(stestGrossProfit, stestWinTrades, 9001)
	lossAmts := silentTestDistributeTotal(stestGrossLoss, stestLossTrades, 9002)
	rng := rand.New(rand.NewSource(20260511))
	base := silentTestWindowStartUTC()
	nowCutoff := time.Now().UTC()
	windowSec := int64(stestCurveWindowDays * 24 * 3600)

	staged := make([]silentStagedTrade, 0, stestTotalTrades)

	for i := 0; i < stestWinTrades; i++ {
		off := int64((i+1)*733 + rng.Intn(4200))
		tOpen := base.Add(time.Duration(off%windowSec) * time.Second)
		if tOpen.After(nowCutoff) {
			tOpen = nowCutoff.Add(-time.Duration(20+i*3) * time.Minute)
		}
		holdMin := int32(38 + (i*17)%185)
		tClose := tOpen.Add(time.Duration(holdMin) * time.Minute)
		if tClose.After(nowCutoff) {
			tClose = nowCutoff
		}
		if !tClose.After(tOpen) {
			tClose = tOpen.Add(25 * time.Minute)
			if tClose.After(nowCutoff) {
				tClose = nowCutoff
			}
		}
		sym := silentTestSymbolRefs[i%len(silentTestSymbolRefs)].Symbol
		side := "long"
		if i >= stestLongTrades {
			side = "short"
		}
		staged = append(staged, silentStagedTrade{tOpen: tOpen, tClose: tClose, symbol: sym, side: side, target: wins[i]})
	}
	for i := 0; i < stestLossTrades; i++ {
		off := int64((i+5)*911 + rng.Intn(5100))
		tOpen := base.Add(time.Duration(off%windowSec) * time.Second)
		if tOpen.After(nowCutoff) {
			tOpen = nowCutoff.Add(-time.Duration(35+i*5) * time.Minute)
		}
		holdMin := int32(32 + (i*23)%160)
		tClose := tOpen.Add(time.Duration(holdMin) * time.Minute)
		if tClose.After(nowCutoff) {
			tClose = nowCutoff
		}
		if !tClose.After(tOpen) {
			tClose = tOpen.Add(22 * time.Minute)
			if tClose.After(nowCutoff) {
				tClose = nowCutoff
			}
		}
		sym := silentTestSymbolRefs[(i+3)%len(silentTestSymbolRefs)].Symbol
		side := "long"
		if i%2 == 1 {
			side = "short"
		}
		staged = append(staged, silentStagedTrade{tOpen: tOpen, tClose: tClose, symbol: sym, side: side, target: -lossAmts[i]})
	}

	sort.Slice(staged, func(i, j int) bool { return staged[i].tClose.After(staged[j].tClose) })

	out := make([]gin.H, 0, len(staged))
	for idx := range staged {
		st := &staged[idx]
		entry := silentPriceAt(st.symbol, st.tOpen)
		cap := silentMaxRelMove(st.symbol)
		exitNat := silentPriceAt(st.symbol, st.tClose)
		rNat := 0.0
		if entry > 0 {
			rNat = (exitNat - entry) / entry
		}
		r := rNat
		target := st.target
		if st.side == "long" {
			if target > 0 {
				if r <= 0 {
					r = 0.0022 + rng.Float64()*math.Min(cap-0.004, 0.018)
				}
			} else {
				if r >= 0 {
					r = -0.0022 - rng.Float64()*math.Min(cap-0.004, 0.020)
				}
			}
		} else {
			if target > 0 {
				if r >= 0 {
					r = -0.0022 - rng.Float64()*math.Min(cap-0.004, 0.018)
				}
			} else {
				if r <= 0 {
					r = 0.0022 + rng.Float64()*math.Min(cap-0.004, 0.020)
				}
			}
		}
		if r > cap {
			r = cap
		}
		if r < -cap {
			r = -cap
		}
		exit := entry * (1 + r)
		denom := exit - entry
		if st.side == "short" {
			denom = entry - exit
		}
		if math.Abs(denom) < entry*1e-6 {
			if st.side == "long" {
				exit = entry * (1 + math.Copysign(0.0035, target))
			} else {
				exit = entry * (1 - math.Copysign(0.0035, target))
			}
			denom = exit - entry
			if st.side == "short" {
				denom = entry - exit
			}
		}
		qty := target / denom
		actPnL := qty * (exit - entry)
		if st.side == "short" {
			actPnL = qty * (entry - exit)
		}
		out = append(out, gin.H{
			"symbol":      st.symbol,
			"side":        st.side,
			"entry_price": math.Round(entry*1e4) / 1e4,
			"exit_price":  math.Round(exit*1e4) / 1e4,
			"pnl_usdt":    math.Round(actPnL*100) / 100,
			"quantity":    math.Round(math.Abs(qty)*1e6) / 1e6, // 展示用绝对数量
			"opened_at":   st.tOpen.Format(time.RFC3339),
			"closed_at":   st.tClose.Format(time.RFC3339),
		})
	}

	var sum float64
	for _, row := range out {
		if v, ok := row["pnl_usdt"].(float64); ok {
			sum += v
		}
	}
	drift := stestTotalPnL - sum
	if len(out) > 0 && math.Abs(drift) > 0.02 {
		last := out[0]
		if pv, ok := last["pnl_usdt"].(float64); ok {
			last["pnl_usdt"] = math.Round((pv+drift)*100) / 100
		}
	}

	return out
}

// silentTestAIInsights 固定 50 条中文「思考过程」展示（与演示区间对应）
func silentTestAIInsights() []gin.H {
	lines := []string{
		"盘前：检查 BTC 4H 结构，上沿未有效突破，偏震荡，小仓试多。",
		"ETH 资金费率略正，短线不宜激进追多，等回踩均线再考虑。",
		"SOL 波动率抬升，止损收紧到 1.2 倍 ATR。",
		"大盘同步性一般，单币独走概率高，降低相关品种总暴露。",
		"凌晨流动性薄，减少新开仓，仅管理已有头寸。",
		"RSI 小时级别进入中性区，不抢方向，等 K 线确认。",
		"BNB 与大盘 Beta 偏高，作为对冲端权重略降。",
		"发现 DOGE 成交量异常，疑似消息驱动，避免短线反复止损。",
		"关注宏观日历：数据公布前半小时降杠杆。",
		"多周期 MA 纠缠，趋势策略信号弱，改区间思路。",
		"XRP 突破后回踩确认，若收回颈线下则放弃。",
		"AVAX 链上活动指标平稳，无额外加分。",
		"LINK 与大盘背离，仅作辅助观察，不单独押注。",
		"DOT 持仓量下降，追多风险上升。",
		"ADA 低波动，适合小网格，不宜大仓。",
		"账户回撤临近阈值，暂停新开多单。",
		"盈亏比不达标信号一律过滤。",
		"BTC 15m 出现放量阴线，短线多单减仓一半。",
		"ETH/BTC 比值走强，资源略向山寨倾斜但仍控总风险。",
		"SOL 永续基差扩大，期现套利资金涌入，短线冲高不贪尾。",
		"震荡市里分批止盈优于一次性清仓。",
		"周末盘口变薄，止损宽度自适应放宽少许避免噪声扫损。",
		"跨交易所价差收窄，套利冲动不大。",
		"消息面真伪未核实前不加仓。",
		"胜率≠期望值，坚持只做正期望样本。",
		"连续盈利后强制冷却一小时，防上头。",
		"波动率模型给出高风险标记，总名义减半。",
		"空头结构在 4H 未确认，暂不以空为主。",
		"多仓浮盈达到阶段目标，提保本。",
		"关键位假突破后迅速收回，假信号概率高。",
		"链上大额转入交易所，偏短期抛压，注意保护利润。",
		"稳定币流动净入，环境偏暖，但需防一手利好兑现回落。",
		"持仓相关性过高，砍掉相关性最差的一笔。",
		"止损触发后复盘：入场逻辑仍成立则等小级别复位再接。",
		"日内已实现波动低于均值，减少刷单频率。",
		"订单簿买单堆积上移，短线支撑有效，持有观察。",
		"期货溢价回落，情绪降温，适合减仓而非追涨。",
		"指标超买但趋势仍在，采用移动止盈而非逆势摸顶。",
		"模拟极端行情下的爆仓价距离，确保安全边际。",
		"交易日志：本周最大回撤来源已标注为『追涨杀跌』，下周规避。",
		"AI 自检：本次扫描未发现高置信度共振信号，保持轻仓。",
		"BINANCE 深度足够，滑点预期低，可执行计划单。",
		"小币流动性一般，仓位按成交额占比硬约束。",
		"情绪指数极端时反向减仓，不反向建仓。",
		"多策略并行时，对同一指数只保留主策略一条腿。",
		"夜盘低能量，若 2 根 K 无进展则平仓观望。",
		"链上巨鲸地址近期安静，不押注方向性大新闻。",
		"回测过拟合警告：近期参数敏感，减少参数微调。",
		"将部分利润转为稳定币，降低心理账户波动。",
		"最后检查：总杠杆、单币权重、事件风险，三宝确认后收工。",
	}
	base := silentTestWindowStartUTC()
	now := time.Now().UTC()
	span := now.Sub(base)
	if span <= 0 {
		span = time.Hour
	}
	out := make([]gin.H, 0, 50)
	n := len(lines)
	for i, text := range lines {
		var at time.Time
		if n <= 1 {
			at = base
		} else {
			at = base.Add(time.Duration(float64(span) * float64(i+1) / float64(n+1)))
		}
		if at.After(now) {
			at = now.Add(-time.Duration(n-i) * time.Minute)
		}
		out = append(out, gin.H{
			"at":      at.Format(time.RFC3339),
			"content": text,
		})
	}
	return out
}

func marketDetailDemoRollupFor(strategyName string) *store.AggregatedTradingRollup {
	if marketDemoIsTest04(strategyName) {
		return marketDetailDemoRollupTest04()
	}
	if marketDemoIsTest03(strategyName) {
		return marketDetailDemoRollupTest03()
	}
	if marketDemoIsTest02(strategyName) {
		return marketDetailDemoRollupTest02()
	}
	if marketDemoIsTestStrategyNoFollow(strategyName) {
		return marketDetailDemoRollupTestStrategyNoFollow()
	}
	if marketDemoIsSilentTest(strategyName) {
		return marketDetailDemoRollupSilentTest()
	}
	if marketDemoIsSuperXiaoLiang(strategyName) {
		return marketDetailDemoRollupSuperXiaoLiang()
	}
	if marketDemoIsSilentHunter(strategyName) {
		return marketDetailDemoRollupSilentHunter()
	}
	return marketDetailDemoRollup()
}

// applyPublicStrategyMarketDemoOverlay 策略市场列表项：与详情同一套虚拟曲线与汇总字段
func applyPublicStrategyMarketDemoOverlay(item gin.H, strategyID, strategyName string, marketRevision uint) {
	if !marketDetailDemoEnabled() || !marketDetailDemoMatchesStrategy(strategyName) {
		return
	}
	item["market_revision"] = demoMarketRevisionFloat(marketRevision)
	stats, ok := item["stats"].(gin.H)
	if !ok {
		return
	}
	subs, agents, used := marketDemoAudienceStats(strategyName)
	stats["subscribers"] = subs
	stats["running_agents"] = agents
	stats["total_agents"] = agents
	stats["used_by"] = used
	stats["data_complete"] = true

	if marketDemoIsTest04(strategyName) {
		stats["return_7d_pct"] = t04ReturnPct
		stats["max_drawdown_pct"] = t04MaxDrawdownPct
		stats["trend"] = marketDemoWavyEquityTrendTest04(strategyID, t04InitialCap, t04EndEquity, t04TrendPoints)
		stats["total_aum"] = t04EndEquity
		return
	}

	if marketDemoIsTest03(strategyName) {
		stats["return_7d_pct"] = t03ReturnPct
		stats["max_drawdown_pct"] = t03MaxDrawdownPct
		stats["trend"] = marketDemoWavyEquityTrendTest03(strategyID, t03InitialCap, t03EndEquity, t03TrendPoints)
		stats["total_aum"] = t03EndEquity
		return
	}

	if marketDemoIsTest02(strategyName) {
		stats["return_7d_pct"] = t02ReturnPct
		stats["max_drawdown_pct"] = t02MaxDrawdownPct
		stats["trend"] = marketDemoWavyEquityTrendTest02(strategyID, t02InitialCap, t02EndEquity, t02TrendPoints)
		stats["total_aum"] = t02EndEquity
		return
	}

	if marketDemoIsTestStrategyNoFollow(strategyName) {
		stats["return_7d_pct"] = tnfReturnPct
		stats["max_drawdown_pct"] = tnfMaxDrawdownPct
		stats["trend"] = marketDemoWavyEquityTrendTestStrategyNoFollow(strategyID, tnfInitialCap, tnfEndEquity, tnfTrendPoints)
		stats["total_aum"] = tnfEndEquity
		return
	}

	if marketDemoIsSilentTest(strategyName) {
		stats["return_7d_pct"] = stestReturnPct
		stats["max_drawdown_pct"] = stestMaxDrawdownPct
		stats["trend"] = marketDemoWavyEquityTrendSilentTest(strategyID, stestInitialCap, stestEndEquity, stestTrendPoints)
		stats["total_aum"] = stestEndEquity
		return
	}

	if marketDemoIsSuperXiaoLiang(strategyName) {
		stats["return_7d_pct"] = xlReturnPct
		stats["max_drawdown_pct"] = xlMaxDrawdownPct
		stats["trend"] = marketDemoWavyEquityTrendXiaoLiang(strategyID, xlInitialCap, xlEndEquity, demoTrendPoints)
		stats["total_aum"] = xlEndEquity
		return
	}

	if marketDemoIsSilentHunter(strategyName) {
		stats["return_7d_pct"] = shReturnPct
		stats["max_drawdown_pct"] = shMaxDrawdownPct
		stats["trend"] = marketDemoWavyEquityTrendSilentHunter(strategyID, shInitialCap, shEndEquity, demoTrendPoints)
		stats["total_aum"] = shEndEquity
		return
	}

	endEq := demoInitialCap + demoTotalPnL
	stats["return_7d_pct"] = demoReturn7dPct
	stats["max_drawdown_pct"] = demoMaxDrawdownPct
	stats["trend"] = marketDemoWavyEquityTrend(strategyID, demoInitialCap, endEq, demoTrendPoints)
	stats["total_aum"] = demoTotalAUM
}

// applyMarketDetailDemoOverlay 详情页：净值汇总 + stats + 版本号小数
func applyMarketDetailDemoOverlay(strategyID, strategyName string, item gin.H, initialCapital *float64, rollup **store.AggregatedTradingRollup, marketRevision uint) bool {
	if !marketDetailDemoEnabled() || !marketDetailDemoMatchesStrategy(strategyName) {
		return false
	}

	item["market_revision"] = demoMarketRevisionFloat(marketRevision)
	*rollup = marketDetailDemoRollupFor(strategyName)
	switch {
	case marketDemoIsTest04(strategyName):
		*initialCapital = t04InitialCap
	case marketDemoIsTest03(strategyName):
		*initialCapital = t03InitialCap
	case marketDemoIsTest02(strategyName):
		*initialCapital = t02InitialCap
	case marketDemoIsTestStrategyNoFollow(strategyName):
		*initialCapital = tnfInitialCap
	case marketDemoIsSilentTest(strategyName):
		*initialCapital = stestInitialCap
	case marketDemoIsSuperXiaoLiang(strategyName):
		*initialCapital = xlInitialCap
	case marketDemoIsSilentHunter(strategyName):
		*initialCapital = shInitialCap
	default:
		*initialCapital = demoInitialCap
	}

	if gh, ok := item["stats"].(gin.H); ok {
		subs, agents, used := marketDemoAudienceStats(strategyName)
		gh["subscribers"] = subs
		gh["running_agents"] = agents
		gh["total_agents"] = agents
		gh["used_by"] = used

		if marketDemoIsTest04(strategyName) {
			gh["return_7d_pct"] = t04ReturnPct
			gh["max_drawdown_pct"] = t04MaxDrawdownPct
			gh["stats_window_days"] = t04CurveWindowDays
			gh["trend"] = marketDemoWavyEquityTrendTest04(strategyID, t04InitialCap, t04EndEquity, t04TrendPoints)
			gh["total_aum"] = t04EndEquity
			return true
		}

		if marketDemoIsTest03(strategyName) {
			gh["return_7d_pct"] = t03ReturnPct
			gh["max_drawdown_pct"] = t03MaxDrawdownPct
			gh["stats_window_days"] = t03CurveWindowDays
			gh["trend"] = marketDemoWavyEquityTrendTest03(strategyID, t03InitialCap, t03EndEquity, t03TrendPoints)
			gh["total_aum"] = t03EndEquity
			return true
		}

		if marketDemoIsTest02(strategyName) {
			gh["return_7d_pct"] = t02ReturnPct
			gh["max_drawdown_pct"] = t02MaxDrawdownPct
			gh["stats_window_days"] = t02CurveWindowDays
			gh["trend"] = marketDemoWavyEquityTrendTest02(strategyID, t02InitialCap, t02EndEquity, t02TrendPoints)
			gh["total_aum"] = t02EndEquity
			return true
		}

		if marketDemoIsTestStrategyNoFollow(strategyName) {
			gh["return_7d_pct"] = tnfReturnPct
			gh["max_drawdown_pct"] = tnfMaxDrawdownPct
			gh["stats_window_days"] = tnfCurveWindowDays
			gh["trend"] = marketDemoWavyEquityTrendTestStrategyNoFollow(strategyID, tnfInitialCap, tnfEndEquity, tnfTrendPoints)
			gh["total_aum"] = tnfEndEquity
			return true
		}

		if marketDemoIsSilentTest(strategyName) {
			gh["return_7d_pct"] = stestReturnPct
			gh["max_drawdown_pct"] = stestMaxDrawdownPct
			gh["stats_window_days"] = stestCurveWindowDays
			gh["trend"] = marketDemoWavyEquityTrendSilentTest(strategyID, stestInitialCap, stestEndEquity, stestTrendPoints)
			gh["total_aum"] = stestEndEquity
			item["demo_trade_highlights"] = silentTestTradeHighlights()
			item["demo_ai_insights"] = silentTestAIInsights()
			return true
		}

		if marketDemoIsSuperXiaoLiang(strategyName) {
			gh["return_7d_pct"] = xlReturnPct
			gh["max_drawdown_pct"] = xlMaxDrawdownPct
			gh["stats_window_days"] = demoCurveWindowDays
			gh["trend"] = marketDemoWavyEquityTrendXiaoLiang(strategyID, xlInitialCap, xlEndEquity, demoTrendPoints)
			gh["total_aum"] = xlEndEquity
			return true
		}

		if marketDemoIsSilentHunter(strategyName) {
			gh["return_7d_pct"] = shReturnPct
			gh["max_drawdown_pct"] = shMaxDrawdownPct
			gh["stats_window_days"] = demoCurveWindowDays
			gh["trend"] = marketDemoWavyEquityTrendSilentHunter(strategyID, shInitialCap, shEndEquity, demoTrendPoints)
			gh["total_aum"] = shEndEquity
			return true
		}

		endEq := demoInitialCap + demoTotalPnL
		gh["return_7d_pct"] = demoReturn7dPct
		gh["max_drawdown_pct"] = demoMaxDrawdownPct
		gh["stats_window_days"] = demoCurveWindowDays
		gh["trend"] = marketDemoWavyEquityTrend(strategyID, demoInitialCap, endEq, demoTrendPoints)
		gh["total_aum"] = demoTotalAUM
	}

	return true
}
