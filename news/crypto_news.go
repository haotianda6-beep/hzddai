package news

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type Item struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	TitleZH     string     `json:"title_zh"`
	Source      string     `json:"source"`
	URL         string     `json:"url"`
	Summary     string     `json:"summary"`
	SummaryZH   string     `json:"summary_zh"`
	PublishedAt string     `json:"published_at"`
	Tags        []string   `json:"tags"`
	Importance  string     `json:"importance"`
	GoldImpact  GoldImpact `json:"gold_impact"`
}

type Payload struct {
	UpdatedAt  string            `json:"updated_at"`
	Items      []Item            `json:"items"`
	Sources    []SourceStatus    `json:"sources"`
	GoldMarket *GoldMarketStatus `json:"gold_market,omitempty"`
}

type SourceStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type rssFeed struct {
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	GUID        string `xml:"guid"`
}

type source struct {
	Name string
	URL  string
}

var sources = []source{
	{Name: "币圈快讯", URL: "https://news.google.com/rss/search?q=%E5%B8%81%E5%9C%88%20%E5%BF%AB%E8%AE%AF%20OR%20%E5%8A%A0%E5%AF%86%E8%B4%A7%E5%B8%81%20%E5%BF%AB%E8%AE%AF&hl=zh-CN&gl=CN&ceid=CN:zh-Hans"},
	{Name: "比特币快讯", URL: "https://news.google.com/rss/search?q=%E6%AF%94%E7%89%B9%E5%B8%81%20%E5%BF%AB%E8%AE%AF%20OR%20BTC%20%E5%BF%AB%E8%AE%AF&hl=zh-CN&gl=CN&ceid=CN:zh-Hans"},
	{Name: "以太坊快讯", URL: "https://news.google.com/rss/search?q=%E4%BB%A5%E5%A4%AA%E5%9D%8A%20%E5%BF%AB%E8%AE%AF%20OR%20ETH%20%E5%BF%AB%E8%AE%AF&hl=zh-CN&gl=CN&ceid=CN:zh-Hans"},
	{Name: "币安快讯", URL: "https://news.google.com/rss/search?q=%E5%B8%81%E5%AE%89%20%E5%85%AC%E5%91%8A%20OR%20Binance%20%E4%B8%8A%E5%B8%81%20OR%20Binance%20%E5%BF%AB%E8%AE%AF&hl=zh-CN&gl=CN&ceid=CN:zh-Hans"},
	{Name: "ETF监管快讯", URL: "https://news.google.com/rss/search?q=%E6%AF%94%E7%89%B9%E5%B8%81%20ETF%20OR%20%E5%8A%A0%E5%AF%86%20SEC%20OR%20%E5%8A%A0%E5%AF%86%20%E7%9B%91%E7%AE%A1&hl=zh-CN&gl=CN&ceid=CN:zh-Hans"},
	{Name: "链上安全快讯", URL: "https://news.google.com/rss/search?q=%E5%8A%A0%E5%AF%86%E8%B4%A7%E5%B8%81%20%E9%BB%91%E5%AE%A2%20OR%20DeFi%20%E6%94%BB%E5%87%BB%20OR%20%E9%93%BE%E4%B8%8A%20%E5%AE%89%E5%85%A8&hl=zh-CN&gl=CN&ceid=CN:zh-Hans"},
	{Name: "金色财经聚合", URL: "https://news.google.com/rss/search?q=%E9%87%91%E8%89%B2%E8%B4%A2%E7%BB%8F%20%E6%AF%94%E7%89%B9%E5%B8%81%20OR%20%E9%87%91%E8%89%B2%E8%B4%A2%E7%BB%8F%20%E5%8A%A0%E5%AF%86%E8%B4%A7%E5%B8%81&hl=zh-CN&gl=CN&ceid=CN:zh-Hans"},
	{Name: "PANews聚合", URL: "https://news.google.com/rss/search?q=PANews%20%E5%8A%A0%E5%AF%86%20OR%20PANews%20%E6%AF%94%E7%89%B9%E5%B8%81&hl=zh-CN&gl=CN&ceid=CN:zh-Hans"},
	{Name: "黄金市场快讯", URL: googleNewsURL("黄金 OR 现货黄金 OR XAUUSD")},
	{Name: "美联储宏观", URL: googleNewsURL("美联储 OR FOMC OR 美国 CPI OR 美国 PCE OR 美国 非农")},
	{Name: "美元美债", URL: googleNewsURL("美元指数 OR 美债收益率 OR 美国十年期国债收益率")},
	{Name: "地缘风险", URL: googleNewsURL("地缘冲突 OR 战争 OR 停火 OR 制裁 黄金")},
	{Name: "原油能源快讯", URL: googleNewsURL("原油 OR 油价 OR 布伦特 OR WTI OR OPEC OR EIA库存")},
	{Name: "中东能源风险", URL: googleNewsURL("霍尔木兹 OR 伊朗 石油 OR 中东 原油 OR 红海 油轮")},
}

func googleNewsURL(query string) string {
	return "https://news.google.com/rss/search?q=" + url.QueryEscape(query) + "&hl=zh-CN&gl=CN&ceid=CN:zh-Hans"
}

var (
	client      = &http.Client{Timeout: 10 * time.Second}
	cacheMu     sync.Mutex
	cached      *Payload
	cachedAt    time.Time
	tagPatterns = map[string][]string{
		"BTC":         {"bitcoin", " btc ", "btc/usd"},
		"ETH":         {"ethereum", " ether ", " eth ", "eth/usd"},
		"SOL":         {"solana", " sol "},
		"ETF":         {"etf"},
		"SEC":         {"sec", "lawsuit", "regulator"},
		"Binance":     {"binance"},
		"Hack":        {"hack", "exploit", "stolen", "breach", "attack"},
		"Liquidation": {"liquidation", "liquidated"},
		"Fed":         {"fed", "fomc", "rate cut", "interest rate", "美联储", "降息", "加息"},
		"Gold":        {"gold", "xau", "黄金"},
		"DXY":         {"dollar index", "dxy", "美元指数", "美元走强", "美元走弱"},
		"Yield":       {"treasury yield", "bond yield", "美债收益率", "国债收益率"},
		"CPI":         {" cpi ", " pce ", "通胀", "消费者价格指数"},
		"NFP":         {"nonfarm", "non-farm", "非农", "就业数据"},
		"GeoRisk":     {"geopolitical", "war", "sanction", "地缘", "战争", "制裁", "停火"},
		"Oil":         {" oil ", "crude", "brent", "wti", "原油", "油价", "布伦特"},
		"OPEC":        {"opec", "欧佩克"},
		"EnergyRisk":  {"energy", "能源", "霍尔木兹", "红海", "油轮", "供应中断"},
	}
	riskWords = []string{"hack", "exploit", "stolen", "breach", "attack", "lawsuit", "sec", "delist", "liquidation", "黑客", "攻击", "漏洞", "被盗", "清算", "诉讼", "下架", "战争", "制裁", "危机", "霍尔木兹", "供应中断"}
	htmlTagRe = regexp.MustCompile(`<[^>]+>`)
	spaceRe   = regexp.MustCompile(`\s+`)
)

var phraseZH = []struct {
	en string
	zh string
}{
	{"Bitcoin", "比特币"}, {"bitcoin", "比特币"}, {"BTC", "BTC"},
	{"Ethereum", "以太坊"}, {"ethereum", "以太坊"}, {"Ether", "以太坊"},
	{"Solana", "Solana"}, {"XRP", "XRP"}, {"Dogecoin", "狗狗币"},
	{"crypto", "加密货币"}, {"Crypto", "加密货币"},
	{"cryptocurrency", "加密货币"}, {"digital asset", "数字资产"},
	{"stablecoin", "稳定币"}, {"stablecoins", "稳定币"},
	{"ETF", "ETF"}, {"spot ETF", "现货 ETF"},
	{"trader", "交易员"}, {"traders", "交易员"}, {"investor", "投资者"}, {"investors", "投资者"},
	{"market", "市场"}, {"markets", "市场"}, {"price", "价格"}, {"prices", "价格"},
	{"rally", "上涨"}, {"surge", "飙升"}, {"jumps", "上涨"}, {"jump", "上涨"},
	{"falls", "下跌"}, {"fall", "下跌"}, {"drops", "下跌"}, {"drop", "下跌"},
	{"plunges", "大跌"}, {"plunge", "大跌"}, {"rebounds", "反弹"}, {"rebound", "反弹"},
	{"hits", "触及"}, {"hit", "触及"}, {"record high", "历史新高"}, {"all-time high", "历史新高"},
	{"exchange", "交易所"}, {"exchanges", "交易所"}, {"Binance", "币安"},
	{"Coinbase", "Coinbase"}, {"Kraken", "Kraken"}, {"Robinhood", "Robinhood"},
	{"SEC", "美国 SEC"}, {"Fed", "美联储"}, {"FOMC", "FOMC"},
	{"lawsuit", "诉讼"}, {"regulator", "监管机构"}, {"regulation", "监管"},
	{"hack", "黑客攻击"}, {"exploit", "漏洞攻击"}, {"stolen", "被盗"},
	{"liquidation", "清算"}, {"liquidations", "清算"}, {"delist", "下架"},
	{"mining", "挖矿"}, {"miner", "矿工"}, {"miners", "矿工"},
	{"wallet", "钱包"}, {"blockchain", "区块链"}, {"DeFi", "DeFi"},
	{"token", "代币"}, {"tokens", "代币"}, {"memecoin", "Meme 币"}, {"memecoins", "Meme 币"},
	{"fund", "基金"}, {"funds", "基金"}, {"institutional", "机构"},
	{"launches", "推出"}, {"launch", "推出"}, {"raises", "融资"}, {"raise", "融资"},
	{"announces", "宣布"}, {"announce", "宣布"}, {"report", "报告"}, {"reports", "报告"},
	{"analyst", "分析师"}, {"analysts", "分析师"}, {"prediction", "预测"},
	{"bullish", "看涨"}, {"bearish", "看跌"}, {"volume", "成交量"},
	{"futures", "期货"}, {"options", "期权"}, {"derivatives", "衍生品"},
	{"treasury", "储备"}, {"reserve", "储备"}, {"payment", "支付"}, {"payments", "支付"},
	{"court", "法院"}, {"judge", "法官"}, {"approval", "批准"}, {"approved", "已批准"},
	{"rejects", "拒绝"}, {"rejected", "已拒绝"}, {"ban", "禁令"}, {"bans", "禁止"},
}

func Fetch(ctx context.Context) *Payload {
	cacheMu.Lock()
	if cached != nil && time.Since(cachedAt) < 2*time.Minute {
		cp := clonePayload(cached)
		cacheMu.Unlock()
		return enrichGoldMarket(ctx, cp)
	}
	cacheMu.Unlock()

	type result struct {
		src   source
		items []Item
		err   error
	}
	ch := make(chan result, len(sources))
	var wg sync.WaitGroup
	for _, src := range sources {
		src := src
		wg.Add(1)
		go func() {
			defer wg.Done()
			items, err := fetchRSS(ctx, src)
			ch <- result{src: src, items: items, err: err}
		}()
	}
	wg.Wait()
	close(ch)

	seen := map[string]bool{}
	out := make([]Item, 0, 80)
	status := make([]SourceStatus, 0, len(sources))
	for r := range ch {
		if r.err != nil {
			status = append(status, SourceStatus{Name: r.src.Name, Status: "error", Detail: trim(r.err.Error(), 160)})
			continue
		}
		status = append(status, SourceStatus{Name: r.src.Name, Status: "ok"})
		for _, it := range r.items {
			key := dedupeKey(it)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, it)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].PublishedAt > out[j].PublishedAt
	})
	if len(out) > 80 {
		out = out[:80]
	}
	payload := &Payload{
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Items:     out,
		Sources:   status,
	}
	cacheMu.Lock()
	cached = payload
	cachedAt = time.Now()
	cacheMu.Unlock()
	return enrichGoldMarket(ctx, clonePayload(payload))
}

func clonePayload(payload *Payload) *Payload {
	cp := *payload
	cp.Items = append([]Item(nil), payload.Items...)
	cp.Sources = append([]SourceStatus(nil), payload.Sources...)
	return &cp
}

func enrichGoldMarket(ctx context.Context, payload *Payload) *Payload {
	payload.GoldMarket = CurrentGoldMarket(ctx)
	for i := range payload.Items {
		impact := payload.Items[i].GoldImpact
		impact.Drivers = append([]string(nil), impact.Drivers...)
		payload.Items[i].GoldImpact = applyGoldMarketConfirmation(impact, payload.GoldMarket)
	}
	return payload
}

func fetchRSS(ctx context.Context, src source) ([]Item, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "COMKUN-AI-news/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 3_000_000))
	if err != nil {
		return nil, err
	}
	var feed rssFeed
	if err := xml.Unmarshal(b, &feed); err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(feed.Channel.Items))
	for _, raw := range feed.Channel.Items {
		title := clean(raw.Title)
		url := strings.TrimSpace(raw.Link)
		if title == "" || url == "" {
			continue
		}
		displaySource := src.Name
		if strings.Contains(src.Name, "快讯") || strings.Contains(src.Name, "聚合") {
			if t, s := splitGoogleNewsTitle(title); t != "" {
				title = t
				if s != "" {
					displaySource = s
				}
			}
		}
		// 只保留中文源/中文标题，避免英文机翻质量差影响阅读。
		if !looksChinese(title) {
			continue
		}
		published := parseTime(raw.PubDate)
		summary := trim(clean(raw.Description), 180)
		titleZH := translateHeadlineZH(title)
		summaryZH := translateHeadlineZH(summary)
		item := Item{
			ID:          hash(title + "|" + url),
			Title:       title,
			TitleZH:     titleZH,
			Source:      displaySource,
			URL:         url,
			Summary:     summary,
			SummaryZH:   summaryZH,
			PublishedAt: published,
			Tags:        tagsFor(title + " " + summary + " " + url),
			Importance:  importanceFor(title + " " + summary),
			GoldImpact:  analyzeGoldImpact(title+" "+titleZH, summary+" "+summaryZH, published),
		}
		out = append(out, item)
	}
	return out, nil
}

func splitGoogleNewsTitle(title string) (string, string) {
	title = strings.TrimSpace(title)
	parts := strings.Split(title, " - ")
	if len(parts) < 2 {
		return title, ""
	}
	source := strings.TrimSpace(parts[len(parts)-1])
	main := strings.TrimSpace(strings.Join(parts[:len(parts)-1], " - "))
	return main, source
}

func clean(s string) string {
	s = html.UnescapeString(s)
	s = htmlTagRe.ReplaceAllString(s, " ")
	s = strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
	return s
}

func parseTime(s string) string {
	s = strings.TrimSpace(s)
	layouts := []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, time.RFC3339}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return time.Now().UTC().Format(time.RFC3339)
}

func tagsFor(s string) []string {
	low := " " + strings.ToLower(s) + " "
	var tags []string
	for tag, pats := range tagPatterns {
		for _, p := range pats {
			if strings.Contains(low, strings.ToLower(p)) {
				tags = append(tags, tag)
				break
			}
		}
	}
	sort.Strings(tags)
	return tags
}

func importanceFor(s string) string {
	low := strings.ToLower(s)
	for _, w := range riskWords {
		if strings.Contains(low, w) {
			return "risk"
		}
	}
	return "normal"
}

func dedupeKey(it Item) string {
	base := strings.ToLower(strings.TrimSpace(it.Title))
	base = strings.TrimPrefix(base, "breaking: ")
	base = spaceRe.ReplaceAllString(base, " ")
	if base == "" {
		return strings.ToLower(strings.TrimSpace(it.URL))
	}
	return base
}

func hash(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func trim(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}

func looksChinese(s string) bool {
	for _, r := range s {
		if r >= '\u4e00' && r <= '\u9fff' {
			return true
		}
	}
	return false
}

func translateHeadlineZH(s string) string {
	s = clean(s)
	if s == "" || looksChinese(s) {
		return s
	}
	out := s
	for _, p := range phraseZH {
		out = strings.ReplaceAll(out, p.en, p.zh)
	}
	out = strings.ReplaceAll(out, " says ", " 表示 ")
	out = strings.ReplaceAll(out, " amid ", "，背景是 ")
	out = strings.ReplaceAll(out, " after ", "，此前 ")
	out = strings.ReplaceAll(out, " as ", "，因 ")
	out = strings.ReplaceAll(out, " with ", "，伴随 ")
	out = strings.ReplaceAll(out, " for ", "，面向 ")
	out = strings.ReplaceAll(out, " on ", "，关于 ")
	out = strings.ReplaceAll(out, " in ", "，在 ")
	out = strings.ReplaceAll(out, " to ", " 至 ")
	out = strings.ReplaceAll(out, " from ", " 来自 ")
	out = strings.ReplaceAll(out, " by ", " 由 ")
	out = strings.ReplaceAll(out, " and ", " 和 ")
	out = strings.ReplaceAll(out, " or ", " 或 ")
	out = strings.TrimSpace(spaceRe.ReplaceAllString(out, " "))
	return "【机翻】" + out
}
