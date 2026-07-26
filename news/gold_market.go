package news

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type GoldMarketSnapshotInput struct {
	ClientTimestamp int64   `json:"client_timestamp"`
	XAUSymbol       string  `json:"xau_symbol"`
	XAUBid          float64 `json:"xau_bid"`
	XAUAsk          float64 `json:"xau_ask"`
	DollarSymbol    string  `json:"dollar_symbol"`
	DollarBid       float64 `json:"dollar_bid"`
	DollarAsk       float64 `json:"dollar_ask"`
}

type goldMarketSnapshot struct {
	ObservedAt   time.Time `json:"observed_at"`
	XAUSymbol    string    `json:"xau_symbol"`
	XAUPrice     float64   `json:"xau_price"`
	DollarSymbol string    `json:"dollar_symbol"`
	DollarPrice  float64   `json:"dollar_price"`
}

type GoldMarketQuote struct {
	Symbol    string   `json:"symbol"`
	Price     float64  `json:"price"`
	Change5M  *float64 `json:"change_5m,omitempty"`
	Change15M *float64 `json:"change_15m,omitempty"`
	Change60M *float64 `json:"change_60m,omitempty"`
}

type Treasury10YQuote struct {
	Status   string   `json:"status"`
	Date     string   `json:"date,omitempty"`
	Value    *float64 `json:"value,omitempty"`
	ChangeBP *float64 `json:"change_bp,omitempty"`
	Source   string   `json:"source"`
}

type GoldMarketStatus struct {
	Status    string           `json:"status"`
	UpdatedAt string           `json:"updated_at,omitempty"`
	XAU       *GoldMarketQuote `json:"xau,omitempty"`
	Dollar    *GoldMarketQuote `json:"dollar,omitempty"`
	US10Y     Treasury10YQuote `json:"us10y"`
}

var (
	goldMarketMu        sync.Mutex
	goldMarketLoadedFor string
	goldMarketSnapshots []goldMarketSnapshot
	treasuryMu          sync.Mutex
	treasuryCached      Treasury10YQuote
	treasuryCachedAt    time.Time
	treasuryAttemptedAt time.Time
	treasuryFetching    bool
)

func goldMarketStatePath() string {
	if value := strings.TrimSpace(os.Getenv("GOLD_MARKET_STATE_PATH")); value != "" {
		return value
	}
	return filepath.Join("data", "gold_market_snapshots.json")
}

func RecordGoldMarketSnapshot(input GoldMarketSnapshotInput) error {
	xau, err := quoteMid(input.XAUBid, input.XAUAsk)
	if err != nil || xau < 100 || xau > 10000 {
		return errors.New("invalid XAU quote")
	}
	dollar, err := quoteMid(input.DollarBid, input.DollarAsk)
	if err != nil || dollar < 50 || dollar > 200 {
		return errors.New("invalid dollar index quote")
	}
	now := time.Now().UTC()
	observed := time.Unix(input.ClientTimestamp, 0).UTC()
	if input.ClientTimestamp <= 0 || observed.Before(now.Add(-5*time.Minute)) || observed.After(now.Add(5*time.Minute)) {
		observed = now
	}
	snapshot := goldMarketSnapshot{
		ObservedAt: observed,
		XAUSymbol:  strings.TrimSpace(input.XAUSymbol), XAUPrice: xau,
		DollarSymbol: strings.TrimSpace(input.DollarSymbol), DollarPrice: dollar,
	}
	if snapshot.XAUSymbol == "" {
		snapshot.XAUSymbol = "XAUUSDc"
	}
	if snapshot.DollarSymbol == "" {
		snapshot.DollarSymbol = "DOLLAR"
	}

	goldMarketMu.Lock()
	defer goldMarketMu.Unlock()
	path := goldMarketStatePath()
	loadGoldMarketLocked(path)
	cutoff := now.Add(-26 * time.Hour)
	kept := goldMarketSnapshots[:0]
	for _, current := range goldMarketSnapshots {
		if current.ObservedAt.After(cutoff) {
			kept = append(kept, current)
		}
	}
	goldMarketSnapshots = kept
	last := len(goldMarketSnapshots) - 1
	if last >= 0 && goldMarketSnapshots[last].ObservedAt.Unix()/60 == observed.Unix()/60 {
		goldMarketSnapshots[last] = snapshot
	} else {
		goldMarketSnapshots = append(goldMarketSnapshots, snapshot)
	}
	sort.Slice(goldMarketSnapshots, func(i, j int) bool {
		return goldMarketSnapshots[i].ObservedAt.Before(goldMarketSnapshots[j].ObservedAt)
	})
	return saveGoldMarketLocked(path)
}

func CurrentGoldMarket(ctx context.Context) *GoldMarketStatus {
	goldMarketMu.Lock()
	path := goldMarketStatePath()
	loadGoldMarketLocked(path)
	snapshots := append([]goldMarketSnapshot(nil), goldMarketSnapshots...)
	goldMarketMu.Unlock()

	status := &GoldMarketStatus{Status: "offline", US10Y: fetchTreasury10Y(ctx)}
	if len(snapshots) == 0 {
		return status
	}
	latest := snapshots[len(snapshots)-1]
	status.UpdatedAt = latest.ObservedAt.Format(time.RFC3339)
	status.XAU = buildMarketQuote(snapshots, latest, true)
	status.Dollar = buildMarketQuote(snapshots, latest, false)
	age := time.Since(latest.ObservedAt)
	if age < 0 {
		age = 0
	}
	if age <= 3*time.Minute {
		status.Status = "live"
		if status.XAU.Change5M == nil || status.Dollar.Change5M == nil {
			status.Status = "warming_up"
		}
	}
	return status
}

func quoteMid(bid, ask float64) (float64, error) {
	if bid <= 0 || ask <= 0 || ask < bid {
		return 0, errors.New("invalid bid/ask")
	}
	return (bid + ask) / 2, nil
}

func buildMarketQuote(snapshots []goldMarketSnapshot, latest goldMarketSnapshot, xau bool) *GoldMarketQuote {
	symbol, price := latest.DollarSymbol, latest.DollarPrice
	if xau {
		symbol, price = latest.XAUSymbol, latest.XAUPrice
	}
	return &GoldMarketQuote{
		Symbol: symbol, Price: price,
		Change5M:  marketChange(snapshots, latest, 5*time.Minute, xau),
		Change15M: marketChange(snapshots, latest, 15*time.Minute, xau),
		Change60M: marketChange(snapshots, latest, 60*time.Minute, xau),
	}
}

func marketChange(snapshots []goldMarketSnapshot, latest goldMarketSnapshot, window time.Duration, xau bool) *float64 {
	target := latest.ObservedAt.Add(-window)
	index := sort.Search(len(snapshots), func(i int) bool {
		return snapshots[i].ObservedAt.After(target)
	}) - 1
	if index < 0 {
		return nil
	}
	base, current := snapshots[index].DollarPrice, latest.DollarPrice
	if xau {
		base, current = snapshots[index].XAUPrice, latest.XAUPrice
	}
	if base <= 0 {
		return nil
	}
	value := (current/base - 1) * 100
	return &value
}

func loadGoldMarketLocked(path string) {
	if goldMarketLoadedFor == path {
		return
	}
	goldMarketLoadedFor = path
	goldMarketSnapshots = nil
	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &goldMarketSnapshots)
	}
	sort.Slice(goldMarketSnapshots, func(i, j int) bool {
		return goldMarketSnapshots[i].ObservedAt.Before(goldMarketSnapshots[j].ObservedAt)
	})
}

func saveGoldMarketLocked(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(goldMarketSnapshots)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func fetchTreasury10Y(_ context.Context) Treasury10YQuote {
	treasuryMu.Lock()
	if !treasuryCachedAt.IsZero() && time.Since(treasuryCachedAt) < 6*time.Hour {
		quote := treasuryCached
		treasuryMu.Unlock()
		return quote
	}
	if treasuryFetching || (!treasuryAttemptedAt.IsZero() && time.Since(treasuryAttemptedAt) < 5*time.Minute) {
		quote := treasuryFallbackLocked()
		treasuryMu.Unlock()
		return quote
	}
	treasuryFetching = true
	treasuryAttemptedAt = time.Now()
	quote := treasuryFallbackLocked()
	treasuryMu.Unlock()
	go refreshTreasury10Y()
	return quote
}

func refreshTreasury10Y() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	quote, err := requestTreasury10Y(ctx)
	treasuryMu.Lock()
	defer treasuryMu.Unlock()
	treasuryFetching = false
	if err == nil {
		treasuryCached, treasuryCachedAt = quote, time.Now()
	}
}

func requestTreasury10Y(ctx context.Context) (Treasury10YQuote, error) {
	url := fmt.Sprintf("https://home.treasury.gov/resource-center/data-chart-center/interest-rates/pages/xml?data=daily_treasury_yield_curve&field_tdr_date_value=%d", time.Now().UTC().Year())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Treasury10YQuote{}, err
	}
	requestClient := &http.Client{Timeout: 20 * time.Second}
	resp, err := requestClient.Do(req)
	if err != nil {
		return Treasury10YQuote{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Treasury10YQuote{}, fmt.Errorf("treasury HTTP %d", resp.StatusCode)
	}
	points, err := parseTreasury10Y(io.LimitReader(resp.Body, 4_000_000))
	if err != nil || len(points) == 0 {
		return Treasury10YQuote{}, errors.New("treasury 10Y data unavailable")
	}
	latest := points[len(points)-1]
	quote := Treasury10YQuote{Status: "daily", Date: latest.date, Value: &latest.value, Source: "U.S. Treasury"}
	if len(points) > 1 {
		change := (latest.value - points[len(points)-2].value) * 100
		quote.ChangeBP = &change
	}
	return quote, nil
}

func treasuryFallbackLocked() Treasury10YQuote {
	if treasuryCached.Value != nil {
		return treasuryCached
	}
	return Treasury10YQuote{Status: "offline", Source: "U.S. Treasury"}
}

type treasuryPoint struct {
	date  string
	value float64
}

func parseTreasury10Y(reader io.Reader) ([]treasuryPoint, error) {
	decoder := xml.NewDecoder(reader)
	var points []treasuryPoint
	var date, value string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch item := token.(type) {
		case xml.StartElement:
			if item.Name.Local == "NEW_DATE" || item.Name.Local == "BC_10YEAR" {
				var text string
				if err := decoder.DecodeElement(&text, &item); err != nil {
					return nil, err
				}
				if item.Name.Local == "NEW_DATE" {
					date = strings.TrimSpace(text)
				} else {
					value = strings.TrimSpace(text)
				}
			}
		case xml.EndElement:
			if item.Name.Local == "entry" {
				if date != "" && value != "" {
					parsed, err := strconv.ParseFloat(value, 64)
					if err == nil {
						points = append(points, treasuryPoint{date: date, value: parsed})
					}
				}
				date, value = "", ""
			}
		}
	}
	sort.Slice(points, func(i, j int) bool { return points[i].date < points[j].date })
	return points, nil
}
