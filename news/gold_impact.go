package news

import (
	"strings"
	"time"
)

// GoldImpact is a news-event assessment enriched by synchronized market data.
type GoldImpact struct {
	Direction    string   `json:"direction"`
	RiskScore    int      `json:"risk_score"`
	RiskLevel    string   `json:"risk_level"`
	Horizon      string   `json:"horizon"`
	Confirmation string   `json:"confirmation"`
	Reason       string   `json:"reason"`
	Drivers      []string `json:"drivers"`
}

func analyzeGoldImpact(title, summary, publishedAt string) GoldImpact {
	text := strings.ToLower(title + " " + summary)
	drivers := make([]string, 0, 4)
	addDriver := func(driver string) {
		for _, current := range drivers {
			if current == driver {
				return
			}
		}
		if len(drivers) < 4 {
			drivers = append(drivers, driver)
		}
	}

	relevance := 0
	macroEvent := false
	geopolitical := false
	structural := false
	longTermOutlook := false
	switch {
	case containsAny(text, "黄金", "现货金", "xau", " gold ", "金价"):
		relevance = 30
		addDriver("黄金直接相关")
	case containsAny(text, "美联储", "fomc", "降息", "加息", "利率决议", "rate cut", "rate hike"):
		relevance = 26
		addDriver("美联储与利率")
	case containsAny(text, "美元指数", "dxy", "美元走强", "美元走弱", "美债收益率", "国债收益率", "treasury yield"):
		relevance = 25
		addDriver("美元与美债收益率")
	case containsAny(text, "cpi", "pce", "非农", "通胀", "就业数据", "消费者价格指数"):
		relevance = 22
		addDriver("美国宏观数据")
	case containsAny(text, "原油", "油价", "布伦特", "wti", "opec", "欧佩克", "eia库存", "霍尔木兹", "红海", "crude", " oil ", "brent"):
		relevance = 22
		addDriver("原油与通胀预期")
	case containsAny(text, "地缘", "战争", "袭击", "空袭", "制裁", "停火", "geopolitical", "war", "sanction"):
		relevance = 22
		addDriver("地缘与避险情绪")
	case containsAny(text, "央行购金", "央行增持黄金", "黄金etf", "黄金 etf", "gold etf"):
		relevance = 24
		addDriver("央行或ETF资金流")
	}

	if relevance == 0 {
		return GoldImpact{
			Direction:    "neutral",
			RiskLevel:    "none",
			Horizon:      "影响有限",
			Confirmation: "not_required",
			Reason:       "与黄金核心驱动关联较弱，暂不构成黄金交易信号。",
		}
	}

	if containsAny(text, "美联储", "fomc", "cpi", "pce", "非农", "通胀", "利率决议", "就业数据") {
		macroEvent = true
		addDriver("宏观事件")
	}
	if containsAny(text, "原油", "油价", "布伦特", "wti", "opec", "欧佩克", "eia库存", "crude", " oil ", "brent") {
		macroEvent = true
		addDriver("能源价格")
	}
	if containsAny(text, "地缘", "战争", "袭击", "空袭", "制裁", "停火", "geopolitical", "war", "sanction") {
		geopolitical = true
		addDriver("地缘风险")
	}
	if containsAny(text, "霍尔木兹", "红海", "油轮", "伊朗", "中东", "供应中断") {
		geopolitical = true
		addDriver("能源供应风险")
	}
	if containsAny(text, "央行购金", "央行增持黄金", "央行售金", "黄金etf", "黄金 etf", "gold etf") {
		structural = true
		addDriver("中长期资金流")
	}
	if containsAny(text, "明年", "未来一年", "中长期", "长期展望", "next year", "long-term") {
		longTermOutlook = true
		addDriver("中长期展望")
	}

	bullish, bearish := 0, 0
	if containsAny(text, "降息", "下调利率", "鸽派", "宽松", "加息预期降温", "rate cut", "dovish") {
		bullish += 4
		addDriver("利率预期下行")
	}
	if containsAny(text, "美元走弱", "美元下跌", "美元指数下跌", "dollar weak", "dollar falls") {
		bullish += 4
		addDriver("美元走弱")
	}
	if containsAny(text, "收益率下降", "收益率下跌", "收益率走低", "yield falls", "yield drops") {
		bullish += 4
		addDriver("美债收益率下行")
	}
	if containsAny(text, "战争升级", "冲突升级", "发动袭击", "空袭", "制裁升级", "避险升温", "危机", "war escalates") {
		bullish += 3
		addDriver("避险需求上升")
	}
	if containsAny(text, "央行购金", "央行增持黄金", "黄金etf流入", "黄金 etf流入", "gold etf inflow") {
		bullish += 3
		addDriver("黄金资金流入")
	}
	if containsAny(text, "黄金上涨", "金价上涨", "金价走高", "黄金牛市", "重返牛市", "看涨黄金", "金价创新高", "金价创历史新高", "gold rally", "gold rises") {
		bullish += 3
		addDriver("黄金价格趋势向上")
	}

	if containsAny(text, "宣布加息", "上调利率", "鹰派", "维持高利率", "加息预期升温", "rate hike", "hawkish", "higher for longer") {
		bearish += 4
		addDriver("利率预期上行")
	}
	if containsAny(text, "美元走强", "美元上涨", "美元指数上涨", "dollar strong", "dollar rises") {
		bearish += 4
		addDriver("美元走强")
	}
	if containsAny(text, "收益率上升", "收益率上涨", "收益率走高", "yield rises", "yield jumps") {
		bearish += 4
		addDriver("美债收益率上行")
	}
	if containsAny(text, "油价上涨", "原油上涨", "原油大涨", "油价大涨", "油价飙升", "布伦特上涨", "wti上涨", "crude rises", "oil jumps", "oil rises") {
		bearish += 3
		addDriver("油价推升通胀预期")
	}
	if containsAny(text, "强劲非农", "非农超预期", "就业强劲", "停火", "冲突缓和", "风险偏好回升") {
		bearish += 3
		addDriver("避险或降息预期减弱")
	}
	if containsAny(text, "央行售金", "央行减持黄金", "黄金etf流出", "黄金 etf流出", "gold etf outflow") {
		bearish += 3
		addDriver("黄金资金流出")
	}
	if containsAny(text, "黄金下跌", "金价下跌", "金价走低", "黄金回落", "金价回落", "黄金熊市", "看跌黄金", "gold falls") {
		bearish += 3
		addDriver("黄金价格趋势向下")
	}

	direction := "neutral"
	if bullish > bearish+1 {
		direction = "bullish"
	} else if bearish > bullish+1 {
		direction = "bearish"
	}

	shock := 0
	if containsAny(text, "意外", "突发", "紧急", "远超预期", "大幅", "unexpected", "emergency", "surprise") {
		shock = 18
	} else if containsAny(text, "高于预期", "低于预期", "超预期", "不及预期", "above expectations", "below expectations") {
		shock = 15
	} else if containsAny(text, "公布", "决议", "会议纪要", "发布数据") {
		shock = 8
	}

	directionPoints := max(bullish, bearish) * 2
	if directionPoints > 15 {
		directionPoints = 15
	}
	if direction == "neutral" && bullish > 0 && bearish > 0 {
		directionPoints = 8
	}

	durationPoints := 4
	if macroEvent {
		durationPoints = 6
	}
	if geopolitical {
		durationPoints = 8
	}
	if structural {
		durationPoints = 10
	} else if longTermOutlook {
		durationPoints = 8
	}

	score := relevance + shock + directionPoints + durationPoints + recencyPoints(publishedAt)
	// Without synchronized XAU/DXY/yield quotes, the missing 25 confirmation
	// points prevent the heuristic from claiming an extreme-risk signal.
	if score > 75 {
		score = 75
	}

	impact := GoldImpact{
		Direction:    direction,
		RiskScore:    score,
		RiskLevel:    goldRiskLevel(score),
		Horizon:      goldImpactHorizon(macroEvent, geopolitical, structural, longTermOutlook),
		Confirmation: "pending",
		Drivers:      drivers,
	}
	switch direction {
	case "bullish":
		impact.Reason = "事件偏向降低黄金持有机会成本或抬升避险需求。"
	case "bearish":
		impact.Reason = "事件偏向抬升美元/美债收益率或削弱避险需求。"
	default:
		impact.Reason = "利多与利空线索不足或相互冲突，当前方向不明确。"
	}
	impact.Reason += " 等待同步行情确认。"
	return impact
}

func containsAny(text string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func recencyPoints(value string) int {
	published, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return 0
	}
	age := time.Since(published)
	if age < 0 {
		age = 0
	}
	switch {
	case age <= 6*time.Hour:
		return 10
	case age <= 24*time.Hour:
		return 7
	case age <= 72*time.Hour:
		return 4
	default:
		return 1
	}
}

func goldRiskLevel(score int) string {
	switch {
	case score >= 85:
		return "extreme"
	case score >= 70:
		return "high"
	case score >= 50:
		return "medium"
	case score >= 25:
		return "low"
	default:
		return "none"
	}
}

func goldImpactHorizon(macroEvent, geopolitical, structural, longTermOutlook bool) string {
	switch {
	case structural:
		return "1–12周"
	case longTermOutlook:
		return "1–12个月"
	case geopolitical:
		return "数小时–3日"
	case macroEvent:
		return "5分钟–1个交易日"
	default:
		return "1–5日"
	}
}
