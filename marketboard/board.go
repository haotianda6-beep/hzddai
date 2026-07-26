// Package marketboard 聚合无需 API Key 的公开行情（Binance、Frankfurter、Stooq、CoinCap），供 /api/market/board 使用。
package marketboard

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	binanceBase     = "https://api.binance.com"
	binanceFapiBase = "https://fapi.binance.com"
	frankfurterBase = "https://api.frankfurter.dev/v1"
	stooqBase       = "https://stooq.com/q/l/"
	coinCapBase     = "https://api.coincap.io/v2"
	polymarketBase  = "https://gamma-api.polymarket.com"
	fearGreedBase   = "https://api.alternative.me"
)

// Quote 单条展示用行情
type Quote struct {
	Symbol    string  `json:"symbol"`
	Name      string  `json:"name"`
	Category  string  `json:"category"`
	Price     float64 `json:"price"`
	ChangePct float64 `json:"change_pct"`
	Volume    float64 `json:"volume,omitempty"`
	Extra     string  `json:"extra,omitempty"`
	Up        bool    `json:"up"`
	Source    string  `json:"source,omitempty"`
}

// TableRow 表格一行
type TableRow struct {
	Rank      int     `json:"rank"`
	Symbol    string  `json:"symbol"`
	Name      string  `json:"name"`
	Price     float64 `json:"price"`
	ChangePct float64 `json:"change_pct"`
	Volume    float64 `json:"volume,omitempty"`
	VolumeStr string  `json:"volume_str"`
	Signal    string  `json:"signal"`
	Up        bool    `json:"up"`
}

// CardBlock 热门市场卡片
type CardBlock struct {
	ID      string  `json:"id"`
	TitleZH string  `json:"title_zh"`
	TitleEN string  `json:"title_en"`
	Items   []Quote `json:"items"`
}

// Metric 顶部关键指标，全部来自免费公开源。
type Metric struct {
	ID       string `json:"id"`
	LabelZH  string `json:"label_zh"`
	LabelEN  string `json:"label_en"`
	Value    string `json:"value"`
	DetailZH string `json:"detail_zh,omitempty"`
	DetailEN string `json:"detail_en,omitempty"`
	Tone     string `json:"tone,omitempty"` // up/down/neutral/warn
}

// SourceStatus 数据源状态，前端用于明确告诉用户哪些数据是真实拉到的。
type SourceStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok/error
	Detail string `json:"detail,omitempty"`
}

// BoardPayload 看板完整数据
type BoardPayload struct {
	UpdatedAt  string                `json:"updated_at"`
	Ticker     []Quote               `json:"ticker"`
	Cards      []CardBlock           `json:"cards"`
	Metrics    []Metric              `json:"metrics"`
	Sources    []SourceStatus        `json:"sources"`
	TableRows  map[string][]TableRow `json:"table_rows"`
	Disclaimer string                `json:"disclaimer"`
}

var httpClient = &http.Client{Timeout: 8 * time.Second}

func getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "hzddai-marketboard/1.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: %d %s", u, resp.StatusCode, string(b[:min(200, len(b))]))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// --- Binance ---

type binance24h struct {
	Symbol             string `json:"symbol"`
	PriceChangePercent string `json:"priceChangePercent"`
	LastPrice          string `json:"lastPrice"`
	QuoteVolume        string `json:"quoteVolume"`
}

type binancePremiumIndex struct {
	Symbol          string `json:"symbol"`
	LastFundingRate string `json:"lastFundingRate"`
	NextFundingTime int64  `json:"nextFundingTime"`
	MarkPrice       string `json:"markPrice"`
}

type binanceOpenInterest struct {
	Symbol       string `json:"symbol"`
	OpenInterest string `json:"openInterest"`
}

func fetchBinance24h(ctx context.Context, symbols []string) ([]binance24h, error) {
	raw, err := json.Marshal(symbols)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("%s/api/v3/ticker/24hr?symbols=%s", binanceBase, url.QueryEscape(string(raw)))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "hzddai-marketboard/1.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("binance: %d %s", resp.StatusCode, string(b[:min(200, len(b))]))
	}
	var rows []binance24h
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func fetchBinanceAll24h(ctx context.Context) ([]binance24h, error) {
	var all []binance24h
	if err := getJSON(ctx, binanceBase+"/api/v3/ticker/24hr", &all); err != nil {
		return nil, err
	}
	return all, nil
}

func pickDefaultBinanceRows(all []binance24h) []binance24h {
	bySym := make(map[string]binance24h, len(all))
	for _, r := range all {
		bySym[r.Symbol] = r
	}
	want := defaultBinanceSymbols()
	out := make([]binance24h, 0, len(want))
	for _, s := range want {
		if r, ok := bySym[s]; ok {
			out = append(out, r)
		}
	}
	if len(out) > 0 {
		return out
	}
	if len(all) > 20 {
		return all[:20]
	}
	return all
}

// topUSDTFromAll 从全市场 24h 列表筛 USDT 并按成交额排序（避免重复请求 Binance）。
func topUSDTFromAll(all []binance24h, limit int) []binance24h {
	if limit <= 0 {
		limit = 50
	}
	type rowVol struct {
		r binance24h
		v float64
	}
	filtered := make([]rowVol, 0, 256)
	for _, r := range all {
		sym := r.Symbol
		if !strings.HasSuffix(sym, "USDT") {
			continue
		}
		upper := strings.ToUpper(sym)
		if strings.Contains(upper, "UPUSDT") || strings.Contains(upper, "DOWNUSDT") ||
			strings.Contains(upper, "BULL") || strings.Contains(upper, "BEAR") {
			continue
		}
		vol, err := strconv.ParseFloat(r.QuoteVolume, 64)
		if err != nil || vol < 5e5 {
			continue
		}
		filtered = append(filtered, rowVol{r, vol})
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].v > filtered[j].v })
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	out := make([]binance24h, len(filtered))
	for i, rv := range filtered {
		out[i] = rv.r
	}
	return out
}

// fetchBinanceTopUSDTByVolume 保留兼容：单次拉全市场后筛选。
func fetchBinanceTopUSDTByVolume(ctx context.Context, limit int) ([]binance24h, error) {
	all, err := fetchBinanceAll24h(ctx)
	if err != nil {
		return nil, err
	}
	return topUSDTFromAll(all, limit), nil
}

func fetchBinancePremiumIndex(ctx context.Context) (map[string]binancePremiumIndex, error) {
	var rows []binancePremiumIndex
	if err := getJSON(ctx, binanceFapiBase+"/fapi/v1/premiumIndex", &rows); err != nil {
		return nil, err
	}
	out := make(map[string]binancePremiumIndex, len(rows))
	for _, r := range rows {
		if r.Symbol != "" {
			out[r.Symbol] = r
		}
	}
	return out, nil
}

func fetchBinanceOpenInterestMap(ctx context.Context, symbols []string) map[string]float64 {
	out := make(map[string]float64, len(symbols))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5)
	for _, sym := range symbols {
		sym := sym
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var row binanceOpenInterest
			u := fmt.Sprintf("%s/fapi/v1/openInterest?symbol=%s", binanceFapiBase, url.QueryEscape(sym))
			if err := getJSON(ctx, u, &row); err != nil {
				return
			}
			oi, err := strconv.ParseFloat(row.OpenInterest, 64)
			if err != nil || oi <= 0 {
				return
			}
			mu.Lock()
			out[sym] = oi
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

var binanceDisplayNames = map[string]string{
	"BTCUSDT": "Bitcoin", "ETHUSDT": "Ethereum", "BNBUSDT": "BNB", "SOLUSDT": "Solana",
	"XRPUSDT": "XRP", "DOGEUSDT": "Dogecoin", "ADAUSDT": "Cardano", "AVAXUSDT": "Avalanche",
	"DOTUSDT": "Polkadot", "LINKUSDT": "Chainlink", "LTCUSDT": "Litecoin", "TRXUSDT": "TRON",
	"TONUSDT": "Toncoin", "UNIUSDT": "Uniswap", "ATOMUSDT": "Cosmos", "XLMUSDT": "Stellar",
	"BCHUSDT": "Bitcoin Cash", "ETCUSDT": "Ethereum Classic", "ARBUSDT": "Arbitrum", "OPUSDT": "Optimism",
}

func defaultBinanceSymbols() []string {
	return []string{
		"BTCUSDT", "ETHUSDT", "BNBUSDT", "SOLUSDT", "XRPUSDT", "DOGEUSDT", "ADAUSDT", "AVAXUSDT",
		"DOTUSDT", "LINKUSDT", "LTCUSDT", "TRXUSDT", "TONUSDT", "UNIUSDT", "ATOMUSDT", "XLMUSDT",
		"BCHUSDT", "ETCUSDT", "ARBUSDT", "OPUSDT",
	}
}

// --- Frankfurter ---

type frankLatest struct {
	Amount float64            `json:"amount"`
	Base   string             `json:"base"`
	Date   string             `json:"date"`
	Rates  map[string]float64 `json:"rates"`
}

type frankRange struct {
	Amount    float64                       `json:"amount"`
	Base      string                        `json:"base"`
	StartDate string                        `json:"start_date"`
	EndDate   string                        `json:"end_date"`
	Rates     map[string]map[string]float64 `json:"rates"`
}

func fetchForexQuotes(ctx context.Context) ([]Quote, error) {
	return fetchForexQuotesWithRange(ctx, true)
}

// fetchForexQuotesFast 仅 latest，跳过 7 日区间（看板加速）。
func fetchForexQuotesFast(ctx context.Context) ([]Quote, error) {
	return fetchForexQuotesWithRange(ctx, false)
}

func fetchForexQuotesWithRange(ctx context.Context, withRange bool) ([]Quote, error) {
	targets := []string{"EUR", "GBP", "JPY", "CNY", "CHF", "AUD", "CAD", "HKD", "SGD", "NZD"}
	toParam := strings.Join(targets, ",")
	var latest frankLatest
	if err := getJSON(ctx, frankfurterBase+"/latest?from=USD&to="+toParam, &latest); err != nil {
		return nil, err
	}
	var rg frankRange
	if withRange {
		end := latest.Date
		t0, err := time.Parse("2006-01-02", end)
		if err != nil {
			t0 = time.Now().UTC()
		}
		start := t0.AddDate(0, 0, -7).Format("2006-01-02")
		rangeURL := fmt.Sprintf("%s/%s..%s?from=USD&to=%s", frankfurterBase, start, end, toParam)
		if err := getJSON(ctx, rangeURL, &rg); err != nil {
			rg.Rates = nil
		}
	}

	names := map[string]string{
		"EUR": "欧元", "GBP": "英镑", "JPY": "日元", "CNY": "人民币", "CHF": "瑞郎",
		"AUD": "澳元", "CAD": "加元", "HKD": "港币", "SGD": "新元", "NZD": "纽元",
	}

	out := make([]Quote, 0, len(targets))
	for _, c := range targets {
		rate, ok := latest.Rates[c]
		if !ok || rate <= 0 {
			continue
		}
		// EURUSD、GBPUSD 等：price = 1/rate（rate 为「1 USD 兑多少外币」中的外币数量对 EUR/GBP 即外币/USD）
		// USDJPY、USDCNY 等：price = rate（直接为常见货币对报价）
		jpyLike := c == "JPY" || c == "CNY" || c == "HKD"
		var price float64
		var pairSym string
		if jpyLike {
			pairSym = "USD" + c
			price = rate
		} else {
			pairSym = c + "USD"
			price = 1 / rate
		}
		ch := 0.0
		if rg.Rates != nil {
			var dates []string
			for d := range rg.Rates {
				if _, ok2 := rg.Rates[d][c]; ok2 {
					dates = append(dates, d)
				}
			}
			sort.Strings(dates)
			if len(dates) >= 2 {
				first := rg.Rates[dates[0]][c]
				last := rg.Rates[dates[len(dates)-1]][c]
				if first > 0 && last > 0 {
					var p0, p1 float64
					if jpyLike {
						p0, p1 = first, last
					} else {
						p0, p1 = 1/first, 1/last
					}
					ch = (p1 - p0) / p0 * 100
				}
			}
		}
		nm := names[c]
		if nm == "" {
			nm = c
		}
		out = append(out, Quote{
			Symbol:    pairSym,
			Name:      nm,
			Category:  "forex",
			Price:     math.Round(price*1e5) / 1e5,
			ChangePct: math.Round(ch*100) / 100,
			Up:        ch >= 0,
			Extra:     "ECB / Frankfurter",
		})
	}
	return out, nil
}

// --- Stooq ---

func fetchStooqRow(ctx context.Context, sym string) (open, close, vol float64, ok bool) {
	u := fmt.Sprintf("%s?s=%s&f=sd2t2ohlcv&h&e=csv", stooqBase, url.QueryEscape(sym))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, 0, 0, false
	}
	req.Header.Set("User-Agent", "hzddai-marketboard/1.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, 0, 0, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, 0, false
	}
	r := csv.NewReader(resp.Body)
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil || len(rows) < 2 {
		return 0, 0, 0, false
	}
	last := rows[len(rows)-1]
	if len(last) < 8 {
		return 0, 0, 0, false
	}
	// Stooq f=sd2t2ohlcv 返回列：
	// Symbol, Date, Time, Open, High, Low, Close, Volume
	// 之前误把 High 当 Open、Volume 当 Close，导致涨跌幅被成交量放大成离谱长数字。
	if strings.EqualFold(strings.TrimSpace(last[3]), "N/D") ||
		strings.EqualFold(strings.TrimSpace(last[6]), "N/D") {
		return 0, 0, 0, false
	}
	o, e1 := strconv.ParseFloat(strings.TrimSpace(last[3]), 64)
	cl, e2 := strconv.ParseFloat(strings.TrimSpace(last[6]), 64)
	v, _ := strconv.ParseFloat(strings.TrimSpace(last[7]), 64)
	if e1 != nil || e2 != nil || o <= 0 || cl <= 0 {
		return 0, 0, 0, false
	}
	return o, cl, v, true
}

var stooqDefs = []struct {
	Sym, Cat, NameZH, NameEN string
}{
	{"aapl.us", "stock", "苹果", "Apple"},
	{"msft.us", "stock", "微软", "Microsoft"},
	{"nvda.us", "stock", "英伟达", "NVIDIA"},
	{"googl.us", "stock", "谷歌 A", "Alphabet"},
	{"amzn.us", "stock", "亚马逊", "Amazon"},
	{"meta.us", "stock", "Meta", "Meta"},
	{"tsm.us", "stock", "台积电 ADR", "TSMC"},
	{"^spx", "index", "标普 500", "S&P 500"},
	{"^ndx", "index", "纳斯达克 100", "Nasdaq 100"},
	{"^dji", "index", "道琼斯", "Dow Jones"},
	{"^hsi", "index", "恒生指数", "Hang Seng"},
	{"xauusd", "commodity", "现货黄金", "Gold Spot"},
	{"xagusd", "commodity", "现货白银", "Silver Spot"},
	{"cl.f", "commodity", "WTI 原油", "WTI Crude"},
	{"ng.f", "commodity", "天然气", "Natural Gas"},
}

func fetchStooqBatch(ctx context.Context) []Quote {
	var mu sync.Mutex
	out := make([]Quote, 0, len(stooqDefs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, def := range stooqDefs {
		def := def
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			o, cl, vol, ok := fetchStooqRow(ctx, def.Sym)
			if !ok {
				return
			}
			ch := (cl - o) / o * 100
			q := Quote{
				Symbol:    strings.ToUpper(strings.TrimPrefix(def.Sym, "^")),
				Name:      def.NameZH,
				Category:  def.Cat,
				Price:     math.Round(cl*1e4) / 1e4,
				ChangePct: math.Round(ch*100) / 100,
				Volume:    vol,
				Up:        ch >= 0,
				Extra:     "Stooq 延迟",
			}
			if def.Cat == "stock" {
				q.Name = def.NameEN
				q.Extra = "美股延迟 · Stooq"
			}
			mu.Lock()
			out = append(out, q)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

// --- CoinCap ---

type coinCapResp struct {
	Data []struct {
		ID              string `json:"id"`
		Symbol          string `json:"symbol"`
		Name            string `json:"name"`
		PriceUsd        string `json:"priceUsd"`
		ChangePercent24 string `json:"changePercent24Hr"`
		MarketCapUsd    string `json:"marketCapUsd"`
		Rank            string `json:"rank"`
	} `json:"data"`
}

func fetchCoinCapTop(ctx context.Context, limit int) ([]Quote, error) {
	u := fmt.Sprintf("%s/assets?limit=%d", coinCapBase, limit)
	var raw coinCapResp
	if err := getJSON(ctx, u, &raw); err != nil {
		return nil, err
	}
	out := make([]Quote, 0, len(raw.Data))
	for _, d := range raw.Data {
		price, _ := strconv.ParseFloat(d.PriceUsd, 64)
		ch, _ := strconv.ParseFloat(d.ChangePercent24, 64)
		mc, _ := strconv.ParseFloat(d.MarketCapUsd, 64)
		extra := ""
		if mc > 0 {
			extra = fmt.Sprintf("市值约 $%.1fB", mc/1e9)
		}
		out = append(out, Quote{
			Symbol:    strings.ToUpper(d.Symbol) + "USDT",
			Name:      d.Name,
			Category:  "crypto",
			Price:     price,
			ChangePct: math.Round(ch*100) / 100,
			Up:        ch >= 0,
			Extra:     extra,
		})
	}
	return out, nil
}

// --- Alternative Fear & Greed ---

type fearGreedResp struct {
	Data []struct {
		Value               string `json:"value"`
		ValueClassification string `json:"value_classification"`
		Timestamp           string `json:"timestamp"`
	} `json:"data"`
}

func fetchFearGreed(ctx context.Context) (Metric, error) {
	var raw fearGreedResp
	if err := getJSON(ctx, fearGreedBase+"/fng/?limit=1", &raw); err != nil {
		return Metric{}, err
	}
	if len(raw.Data) == 0 {
		return Metric{}, fmt.Errorf("fear greed empty")
	}
	v, _ := strconv.Atoi(strings.TrimSpace(raw.Data[0].Value))
	class := strings.TrimSpace(raw.Data[0].ValueClassification)
	tone := "neutral"
	if v >= 65 {
		tone = "up"
	} else if v <= 35 {
		tone = "warn"
	}
	return Metric{
		ID:       "fear_greed",
		LabelZH:  "加密恐惧贪婪",
		LabelEN:  "Crypto Fear & Greed",
		Value:    fmt.Sprintf("%d", v),
		DetailZH: class + " · Alternative.me",
		DetailEN: class + " · Alternative.me",
		Tone:     tone,
	}, nil
}

// --- Polymarket ---

type polyMarket struct {
	Question      string          `json:"question"`
	Slug          string          `json:"slug"`
	Volume        any             `json:"volume"`
	Volume24hr    any             `json:"volume24hr"`
	Liquidity     any             `json:"liquidity"`
	OutcomePrices json.RawMessage `json:"outcomePrices"`
	Outcomes      json.RawMessage `json:"outcomes"`
}

func anyFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	default:
		return 0
	}
}

func parseStringArray(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var arr []string
	if json.Unmarshal(raw, &arr) == nil {
		return arr
	}
	var encoded string
	if json.Unmarshal(raw, &encoded) == nil {
		_ = json.Unmarshal([]byte(encoded), &arr)
	}
	return arr
}

func fetchPolymarket(ctx context.Context) ([]TableRow, []Quote, error) {
	u := polymarketBase + "/markets?active=true&closed=false&order=volume_24hr&ascending=false&limit=12"
	var raw []polyMarket
	if err := getJSON(ctx, u, &raw); err != nil {
		return nil, nil, err
	}
	rows := make([]TableRow, 0, len(raw))
	quotes := make([]Quote, 0, 3)
	for _, m := range raw {
		q := strings.TrimSpace(m.Question)
		if q == "" {
			continue
		}
		vol24 := anyFloat(m.Volume24hr)
		if vol24 <= 0 {
			vol24 = anyFloat(m.Volume)
		}
		liq := anyFloat(m.Liquidity)
		prices := parseStringArray(m.OutcomePrices)
		outs := parseStringArray(m.Outcomes)
		price := 0.0
		if len(prices) > 0 {
			price, _ = strconv.ParseFloat(prices[0], 64)
		}
		label := "预测市场"
		if len(outs) > 0 && strings.TrimSpace(outs[0]) != "" {
			label = "Yes: " + strings.TrimSpace(outs[0])
		}
		row := TableRow{
			Rank:      len(rows) + 1,
			Symbol:    "POLY",
			Name:      q,
			Price:     math.Round(price*10000) / 10000,
			ChangePct: 0,
			Volume:    vol24,
			VolumeStr: formatVol(vol24),
			Signal:    fmt.Sprintf("流动性 %s", formatVol(liq)),
			Up:        true,
		}
		rows = append(rows, row)
		if len(quotes) < 3 {
			quotes = append(quotes, Quote{
				Symbol: "POLY", Name: q, Category: "prediction", Price: row.Price, ChangePct: 0,
				Volume: vol24, Extra: label + " · " + formatVol(vol24) + " 24h", Up: true, Source: "Polymarket",
			})
		}
	}
	return rows, quotes, nil
}

// buildQuickPayload 趋势页首屏：仅 crypto 表 + ticker + 加密热门卡（跳过 Stooq/Poly 等慢源）。
func buildQuickPayload(payload *BoardPayload, binRows, topUSDTRows []binance24h, forex []Quote, binErr error) *BoardPayload {
	bySym := map[string]binance24h{}
	for _, r := range binRows {
		bySym[r.Symbol] = r
	}
	addTicker := func(q Quote) {
		payload.Ticker = append(payload.Ticker, q)
	}
	for _, sym := range []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT", "XRPUSDT"} {
		if r, ok := bySym[sym]; ok {
			last, _ := strconv.ParseFloat(r.LastPrice, 64)
			ch, _ := strconv.ParseFloat(r.PriceChangePercent, 64)
			name := binanceDisplayNames[sym]
			if name == "" {
				name = sym
			}
			addTicker(Quote{Symbol: sym, Name: name, Category: "crypto", Price: last, ChangePct: ch, Up: ch >= 0})
		}
	}
	for _, q := range forex {
		if len(payload.Ticker) >= 14 {
			break
		}
		addTicker(q)
	}

	type pair struct {
		q binance24h
		v float64
	}
	src := binRows
	if len(topUSDTRows) > 0 {
		src = topUSDTRows
	}
	var pairs []pair
	for _, r := range src {
		vol, _ := strconv.ParseFloat(r.QuoteVolume, 64)
		pairs = append(pairs, pair{r, vol})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].v > pairs[j].v })
	cryptoTop3 := make([]Quote, 0, 3)
	for i := 0; i < len(pairs) && len(cryptoTop3) < 3; i++ {
		r := pairs[i].q
		last, _ := strconv.ParseFloat(r.LastPrice, 64)
		ch, _ := strconv.ParseFloat(r.PriceChangePercent, 64)
		name := binanceDisplayNames[r.Symbol]
		if name == "" {
			name = r.Symbol
		}
		cryptoTop3 = append(cryptoTop3, Quote{
			Symbol: r.Symbol, Name: name, Category: "crypto", Price: last, ChangePct: ch, Up: ch >= 0,
			Volume: pairs[i].v, Extra: fmt.Sprintf("24h 额 $%.2fB", pairs[i].v/1e9),
		})
	}

	payload.Cards = []CardBlock{
		{ID: "crypto_vol", TitleZH: "加密货币（24h 成交额 Top）", TitleEN: "Crypto (24h volume)", Items: padQuotes(cryptoTop3, 3)},
		{ID: "us_stocks", TitleZH: "美股与 AI 链（延迟）", TitleEN: "US / AI stocks (delayed)", Items: padQuotes(nil, 3)},
		{ID: "commodities", TitleZH: "大宗商品", TitleEN: "Commodities", Items: padQuotes(nil, 3)},
		{ID: "prediction", TitleZH: "预测市场（Polymarket）", TitleEN: "Prediction markets", Items: padQuotes(nil, 3)},
		{ID: "forex", TitleZH: "外汇", TitleEN: "Forex", Items: padQuotes(forex, 3)},
	}

	cryptoRows := src
	payload.TableRows["crypto"] = binanceToTableRows(cryptoRows)
	payload.TableRows["derivatives"] = []TableRow{}
	payload.TableRows["forex"] = quotesToTableRows(forex, 1)
	payload.TableRows["stocks"] = []TableRow{}
	payload.TableRows["indices"] = []TableRow{}
	payload.TableRows["commodities"] = []TableRow{}
	payload.TableRows["prediction"] = []TableRow{}

	payload.Sources = []SourceStatus{
		{Name: "Binance Spot 24h", Status: statusFromErr(binErr), Detail: errText(binErr)},
		{Name: "Frankfurter ECB FX", Status: statusFromBool(len(forex) > 0), Detail: ""},
	}
	return payload
}

// FetchBoard 拉取看板；quick 仅 Binance+外汇快路径。全量结果带 45s 内存缓存与 stale 后台刷新。
func FetchBoard(ctx context.Context, quick bool) *BoardPayload {
	if quick {
		if p := getQuickCached(); p != nil {
			return p
		}
		p := buildBoardPayload(ctx, true)
		setQuickCached(p)
		return p
	}
	if p, _ := getFullCached(false); p != nil {
		return p
	}
	if p, _ := getFullCached(true); p != nil {
		refreshFullAsync()
		return p
	}
	p := buildBoardPayload(ctx, false)
	setFullCached(p)
	return p
}

// buildBoardPayload 拉取并合并所有免费源（部分失败仍返回其余数据）。
func buildBoardPayload(ctx context.Context, quick bool) *BoardPayload {
	payload := &BoardPayload{
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
		TableRows:  make(map[string][]TableRow),
		Disclaimer: "行情来自免费公开接口：Binance 现货/合约、Frankfurter(ECB)、Stooq 延迟快照、CoinCap、Alternative.me、Polymarket。不同市场存在延迟与口径差异，不构成投资建议。",
	}

	timeout := 18 * time.Second
	if quick {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var br struct {
		rows []binance24h
		err  error
	}
	var allBin []binance24h
	var allBinErr error
	var topUSDTRows []binance24h
	var premium map[string]binancePremiumIndex
	var premiumErr error
	var fear Metric
	var fearErr error
	var polyRows []TableRow
	var polyQuotes []Quote
	var polyErr error
	var forex []Quote
	var forexErr error
	var stooqQ []Quote
	var oiMap map[string]float64
	var ccTop3 []Quote
	var ccTop25 []Quote
	var ccErr error

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		allBin, allBinErr = fetchBinanceAll24h(ctx)
		if allBinErr == nil {
			br.rows = pickDefaultBinanceRows(allBin)
			topUSDTRows = topUSDTFromAll(allBin, 60)
		} else {
			br.err = allBinErr
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if quick {
			forex, forexErr = fetchForexQuotesFast(ctx)
		} else {
			forex, forexErr = fetchForexQuotes(ctx)
		}
		if forexErr != nil {
			forex = nil
		}
	}()

	if !quick {
		wg.Add(6)
		go func() {
			defer wg.Done()
			stooqQ = fetchStooqBatch(ctx)
		}()
		go func() {
			defer wg.Done()
			premium, premiumErr = fetchBinancePremiumIndex(ctx)
		}()
		go func() {
			defer wg.Done()
			fear, fearErr = fetchFearGreed(ctx)
		}()
		go func() {
			defer wg.Done()
			polyRows, polyQuotes, polyErr = fetchPolymarket(ctx)
		}()
		go func() {
			defer wg.Done()
			oiMap = fetchBinanceOpenInterestMap(ctx, defaultBinanceSymbols()[:8])
		}()
		go func() {
			defer wg.Done()
			if cc, err := fetchCoinCapTop(ctx, 25); err == nil {
				ccTop25 = cc
				if len(cc) > 3 {
					ccTop3 = cc[:3]
				} else {
					ccTop3 = cc
				}
			} else {
				ccErr = err
			}
		}()
	}

	wg.Wait()

	var binRows []binance24h
	if br.err == nil {
		binRows = br.rows
	}
	if len(binRows) == 0 && len(ccTop25) > 0 {
		for _, q := range ccTop25 {
			binRows = append(binRows, binance24h{
				Symbol:             q.Symbol,
				LastPrice:          strconv.FormatFloat(q.Price, 'f', 8, 64),
				PriceChangePercent: strconv.FormatFloat(q.ChangePct, 'f', 4, 64),
				QuoteVolume:        strconv.FormatFloat(q.Volume, 'f', 2, 64),
			})
		}
	}

	if quick {
		return buildQuickPayload(payload, binRows, topUSDTRows, forex, br.err)
	}

	payload.Sources = []SourceStatus{
		{Name: "Binance Spot 24h", Status: statusFromErr(br.err), Detail: errText(br.err)},
		{Name: "Binance Futures Premium/OI", Status: statusFromErr(premiumErr), Detail: errText(premiumErr)},
		{Name: "Frankfurter ECB FX", Status: statusFromBool(len(forex) > 0), Detail: ""},
		{Name: "Stooq delayed markets", Status: statusFromBool(len(stooqQ) > 0), Detail: ""},
		{Name: "Alternative.me Fear & Greed", Status: statusFromErr(fearErr), Detail: errText(fearErr)},
		{Name: "Polymarket Gamma", Status: statusFromErr(polyErr), Detail: errText(polyErr)},
	}

	bySym := map[string]binance24h{}
	for _, r := range binRows {
		bySym[r.Symbol] = r
	}

	// --- Ticker：混合顶部滚动 ---
	tickerSet := map[string]bool{}
	addTicker := func(q Quote) {
		k := q.Category + ":" + q.Symbol
		if tickerSet[k] {
			return
		}
		tickerSet[k] = true
		payload.Ticker = append(payload.Ticker, q)
	}
	for _, sym := range []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT", "XRPUSDT"} {
		if r, ok := bySym[sym]; ok {
			last, _ := strconv.ParseFloat(r.LastPrice, 64)
			ch, _ := strconv.ParseFloat(r.PriceChangePercent, 64)
			name := binanceDisplayNames[sym]
			if name == "" {
				name = sym
			}
			addTicker(Quote{Symbol: sym, Name: name, Category: "crypto", Price: last, ChangePct: ch, Up: ch >= 0})
		}
	}
	for _, q := range forex {
		if len(payload.Ticker) >= 18 {
			break
		}
		addTicker(q)
	}
	for _, q := range stooqQ {
		if len(payload.Ticker) >= 26 {
			break
		}
		if q.Category == "index" || q.Category == "commodity" {
			addTicker(q)
		}
	}
	for _, q := range polyQuotes {
		if len(payload.Ticker) >= 30 {
			break
		}
		addTicker(q)
	}

	// --- 卡片 1：加密货币（Binance 按 24h 成交额取前 3）---
	type pair struct {
		q binance24h
		v float64
	}
	var pairs []pair
	for _, r := range binRows {
		vol, _ := strconv.ParseFloat(r.QuoteVolume, 64)
		pairs = append(pairs, pair{r, vol})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].v > pairs[j].v })
	cryptoTop3 := make([]Quote, 0, 3)
	for i := 0; i < len(pairs) && len(cryptoTop3) < 3; i++ {
		r := pairs[i].q
		last, _ := strconv.ParseFloat(r.LastPrice, 64)
		ch, _ := strconv.ParseFloat(r.PriceChangePercent, 64)
		name := binanceDisplayNames[r.Symbol]
		if name == "" {
			name = r.Symbol
		}
		vol := pairs[i].v
		cryptoTop3 = append(cryptoTop3, Quote{
			Symbol: r.Symbol, Name: name, Category: "crypto", Price: last, ChangePct: ch, Up: ch >= 0,
			Volume: vol,
			Extra:  fmt.Sprintf("24h 额 $%.2fB", vol/1e9),
		})
	}

	// --- 卡片 2：美股 Top3 成交额（从 stooq 股票子集）---
	stStock := filterQuotes(stooqQ, "stock")
	sort.Slice(stStock, func(i, j int) bool { return stStock[i].Volume > stStock[j].Volume })
	if len(stStock) > 3 {
		stStock = stStock[:3]
	}

	// --- 卡片 3：指数 ---
	stIdx := filterQuotes(stooqQ, "index")

	// --- 卡片 4：大宗 ---
	stCom := filterQuotes(stooqQ, "commodity")

	// --- 卡片 5：CoinCap 参考排名（非 AI500，仅免费展示）---
	cardCoinCap := CardBlock{
		ID: "crypto_mcap", TitleZH: "加密货币（市值参考）", TitleEN: "Crypto by market cap",
		Items: ccTop3,
	}
	if len(cardCoinCap.Items) == 0 {
		cardCoinCap.Items = cryptoTop3
	}
	_ = ccErr

	// --- 卡片 6：外汇 Top3 波动 ---
	fxSorted := append([]Quote(nil), forex...)
	sort.Slice(fxSorted, func(i, j int) bool {
		ai := math.Abs(fxSorted[i].ChangePct)
		aj := math.Abs(fxSorted[j].ChangePct)
		return ai > aj
	})
	fxTop3 := fxSorted
	if len(fxTop3) > 3 {
		fxTop3 = fxTop3[:3]
	}

	// --- 合约资金费率 + OI（Binance 免费公开）---
	if oiMap == nil {
		oiMap = make(map[string]float64)
	}
	derivRows := derivativesRows(binRows, premium, oiMap)
	derivTop := derivativesQuotes(derivRows, 3)
	if len(derivTop) == 0 {
		derivTop = cryptoTop3
	}

	polyCardItems := polyQuotes
	if len(polyCardItems) == 0 {
		polyCardItems = []Quote{{Symbol: "POLY", Name: "暂无可用预测市场数据", Category: "prediction", Up: true, Source: "Polymarket"}}
	}

	payload.Cards = []CardBlock{
		{ID: "crypto_vol", TitleZH: "加密货币（24h 成交额 Top）", TitleEN: "Crypto (24h volume)", Items: cryptoTop3},
		{ID: "derivatives", TitleZH: "合约资金费率 / OI", TitleEN: "Funding / Open Interest", Items: padQuotes(derivTop, 3)},
		{ID: "us_stocks", TitleZH: "美股与 AI 链（延迟）", TitleEN: "US / AI stocks (delayed)", Items: padQuotes(stStock, 3)},
		{ID: "indices", TitleZH: "指数", TitleEN: "Indices", Items: padQuotes(stIdx, 3)},
		{ID: "commodities", TitleZH: "大宗商品", TitleEN: "Commodities", Items: padQuotes(stCom, 3)},
		{ID: "forex", TitleZH: "外汇", TitleEN: "Forex", Items: padQuotes(fxTop3, 3)},
		cardCoinCap,
		{ID: "prediction", TitleZH: "预测市场（Polymarket）", TitleEN: "Prediction markets", Items: padQuotes(polyCardItems, 3)},
	}

	// --- 表格 ---
	cryptoRows := binRows
	if len(topUSDTRows) > 0 {
		cryptoRows = topUSDTRows
	}
	payload.TableRows["crypto"] = binanceToTableRows(cryptoRows)
	payload.TableRows["derivatives"] = derivRows
	payload.TableRows["forex"] = quotesToTableRows(forex, 1)
	payload.TableRows["stocks"] = quotesToTableRows(filterQuotes(stooqQ, "stock"), 1)
	payload.TableRows["indices"] = quotesToTableRows(filterQuotes(stooqQ, "index"), 1)
	payload.TableRows["commodities"] = quotesToTableRows(filterQuotes(stooqQ, "commodity"), 1)
	payload.TableRows["prediction"] = polyRows
	payload.Metrics = buildMetrics(binRows, derivRows, forex, stooqQ, fear)

	return payload
}

func padQuotes(q []Quote, n int) []Quote {
	if len(q) >= n {
		return q[:n]
	}
	for len(q) < n {
		q = append(q, Quote{Symbol: "—", Name: "暂无", Category: "empty", Up: true})
	}
	return q
}

func statusFromErr(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}

func statusFromBool(ok bool) string {
	if ok {
		return "ok"
	}
	return "error"
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > 160 {
		return s[:160]
	}
	return s
}

func filterQuotes(in []Quote, cat string) []Quote {
	var out []Quote
	for _, q := range in {
		if q.Category == cat {
			out = append(out, q)
		}
	}
	return out
}

func quotesToTableRows(qs []Quote, startRank int) []TableRow {
	out := make([]TableRow, 0, len(qs))
	for i, q := range qs {
		out = append(out, TableRow{
			Rank:      startRank + i,
			Symbol:    q.Symbol,
			Name:      q.Name,
			Price:     q.Price,
			ChangePct: q.ChangePct,
			Volume:    q.Volume,
			VolumeStr: formatVol(q.Volume),
			Signal:    "—",
			Up:        q.Up,
		})
	}
	return out
}

func binanceToTableRows(rows []binance24h) []TableRow {
	type rowVol struct {
		r binance24h
		v float64
	}
	var list []rowVol
	for _, r := range rows {
		v, _ := strconv.ParseFloat(r.QuoteVolume, 64)
		list = append(list, rowVol{r, v})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v })
	out := make([]TableRow, 0, len(list))
	for i, rv := range list {
		r := rv.r
		last, _ := strconv.ParseFloat(r.LastPrice, 64)
		ch, _ := strconv.ParseFloat(r.PriceChangePercent, 64)
		name := binanceDisplayNames[r.Symbol]
		if name == "" {
			name = r.Symbol
		}
		signal := "中性"
		if ch >= 3 {
			signal = "24h 强势"
		} else if ch <= -3 {
			signal = "24h 弱势"
		}
		out = append(out, TableRow{
			Rank:      i + 1,
			Symbol:    r.Symbol,
			Name:      name,
			Price:     last,
			ChangePct: math.Round(ch*100) / 100,
			Volume:    rv.v,
			VolumeStr: formatVol(rv.v),
			Signal:    signal,
			Up:        ch >= 0,
		})
	}
	return out
}

func derivativesRows(rows []binance24h, premium map[string]binancePremiumIndex, oiMap map[string]float64) []TableRow {
	bySym := make(map[string]binance24h, len(rows))
	for _, r := range rows {
		bySym[r.Symbol] = r
	}
	symbols := defaultBinanceSymbols()
	out := make([]TableRow, 0, len(symbols))
	for _, sym := range symbols {
		p, ok := premium[sym]
		if !ok {
			continue
		}
		funding, _ := strconv.ParseFloat(p.LastFundingRate, 64)
		mark, _ := strconv.ParseFloat(p.MarkPrice, 64)
		ch := 0.0
		if spot, ok := bySym[sym]; ok {
			ch, _ = strconv.ParseFloat(spot.PriceChangePercent, 64)
		}
		oi := oiMap[sym]
		notional := oi * mark
		signal := "资金费率中性"
		switch {
		case funding >= 0.0005:
			signal = "多头拥挤"
		case funding <= -0.0005:
			signal = "空头拥挤"
		case math.Abs(funding) >= 0.0002:
			signal = "轻微偏离"
		}
		out = append(out, TableRow{
			Rank:      len(out) + 1,
			Symbol:    sym,
			Name:      binanceDisplayNames[sym],
			Price:     mark,
			ChangePct: math.Round(ch*100) / 100,
			Volume:    notional,
			VolumeStr: formatVol(notional),
			Signal:    fmt.Sprintf("%s · 费率 %.4f%%", signal, funding*100),
			Up:        funding >= 0,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Volume > out[j].Volume })
	for i := range out {
		out[i].Rank = i + 1
	}
	return out
}

func derivativesQuotes(rows []TableRow, n int) []Quote {
	out := make([]Quote, 0, n)
	for _, r := range rows {
		if len(out) >= n {
			break
		}
		out = append(out, Quote{
			Symbol: r.Symbol, Name: r.Name, Category: "derivatives", Price: r.Price, ChangePct: r.ChangePct,
			Volume: r.Volume, Extra: r.Signal + " · OI " + r.VolumeStr, Up: r.Up, Source: "Binance Futures",
		})
	}
	return out
}

func buildMetrics(binRows []binance24h, derivRows []TableRow, forex []Quote, stooq []Quote, fear Metric) []Metric {
	var out []Metric
	up, total := 0, 0
	for _, r := range binRows {
		ch, _ := strconv.ParseFloat(r.PriceChangePercent, 64)
		total++
		if ch >= 0 {
			up++
		}
	}
	if total > 0 {
		pct := float64(up) / float64(total) * 100
		tone := "neutral"
		if pct >= 60 {
			tone = "up"
		} else if pct <= 40 {
			tone = "down"
		}
		out = append(out, Metric{
			ID: "crypto_breadth", LabelZH: "加密上涨占比", LabelEN: "Crypto breadth",
			Value: fmt.Sprintf("%.0f%%", pct), DetailZH: fmt.Sprintf("%d/%d 个观察币种上涨", up, total),
			DetailEN: fmt.Sprintf("%d/%d tracked coins up", up, total), Tone: tone,
		})
	}
	if len(derivRows) > 0 {
		out = append(out, Metric{
			ID: "top_oi", LabelZH: "最大合约 OI", LabelEN: "Largest futures OI",
			Value: derivRows[0].Symbol, DetailZH: derivRows[0].VolumeStr + " · " + derivRows[0].Signal,
			DetailEN: derivRows[0].VolumeStr + " · " + derivRows[0].Signal, Tone: "neutral",
		})
	}
	if fear.ID != "" {
		out = append(out, fear)
	}
	fxVol := maxAbsQuote(forex)
	if fxVol.Symbol != "" {
		out = append(out, Metric{
			ID: "fx_move", LabelZH: "外汇最大波动", LabelEN: "Largest FX move",
			Value: fxVol.Symbol, DetailZH: fmt.Sprintf("%+.2f%% · Frankfurter", fxVol.ChangePct),
			DetailEN: fmt.Sprintf("%+.2f%% · Frankfurter", fxVol.ChangePct), Tone: toneFromChange(fxVol.ChangePct),
		})
	}
	macro := maxAbsQuote(stooq)
	if macro.Symbol != "" {
		out = append(out, Metric{
			ID: "macro_move", LabelZH: "跨市场最大波动", LabelEN: "Largest cross-market move",
			Value: macro.Symbol, DetailZH: fmt.Sprintf("%+.2f%% · %s", macro.ChangePct, macro.Extra),
			DetailEN: fmt.Sprintf("%+.2f%% · %s", macro.ChangePct, macro.Extra), Tone: toneFromChange(macro.ChangePct),
		})
	}
	return out
}

func maxAbsQuote(qs []Quote) Quote {
	var best Quote
	bestAbs := -1.0
	for _, q := range qs {
		a := math.Abs(q.ChangePct)
		if q.Symbol != "" && a > bestAbs {
			bestAbs = a
			best = q
		}
	}
	return best
}

func toneFromChange(ch float64) string {
	if ch > 0 {
		return "up"
	}
	if ch < 0 {
		return "down"
	}
	return "neutral"
}

func formatVol(v float64) string {
	if v <= 0 {
		return "—"
	}
	if v >= 1e9 {
		return fmt.Sprintf("$%.2fB", v/1e9)
	}
	if v >= 1e6 {
		return fmt.Sprintf("$%.2fM", v/1e6)
	}
	return fmt.Sprintf("$%.0f", v)
}
