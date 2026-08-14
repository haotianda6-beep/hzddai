package observation

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	StatusHistoricalSimulation = "historical_simulation"
	ExpectedArtifactSHA256     = "375b879fdb89549a1612a1486d7933098a59fe40f30bf0508679a4b7996e06d3"
)

//go:embed data/comkunai_observation_histories.json
var artifactJSON []byte

type Document struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Source        string              `json:"source"`
	Status        string              `json:"status"`
	ExportedAt    string              `json:"exportedAt"`
	HistorySchema map[string][]string `json:"historySchema"`
	Slots         []Slot              `json:"slots"`
	SHA256        string              `json:"sha256"`
}

type Slot struct {
	Slot                    int     `json:"slot"`
	HistoryProfileID        string  `json:"historyProfileId"`
	DisplayName             string  `json:"displayName"`
	SourceAccountID         string  `json:"sourceAccountId"`
	SourceStrategyID        *string `json:"sourceStrategyId"`
	HistorySHA256           string  `json:"historySha256"`
	History                 History `json:"history"`
	RealtimeFollowAvailable bool    `json:"-"`
}

type History struct {
	ID                   string          `json:"id"`
	Name                 string          `json:"name"`
	StrategyLabel        string          `json:"strategyLabel"`
	InstrumentCount      int             `json:"instrumentCount"`
	Months               int             `json:"months"`
	DailyTradeRange      string          `json:"dailyTradeRange"`
	AverageMonthlyReturn string          `json:"averageMonthlyReturn"`
	CumulativeReturn     string          `json:"cumulativeReturn"`
	MaximumDrawdown      string          `json:"maximumDrawdown"`
	WinRate              string          `json:"winRate"`
	TradeCount           int             `json:"tradeCount"`
	InitialBalance       string          `json:"initialBalance"`
	CurrentBalance       string          `json:"currentBalance"`
	StartAt              string          `json:"startAt"`
	EndAt                string          `json:"endAt"`
	Status               string          `json:"status"`
	Disclosure           string          `json:"disclosure"`
	MonthlyResults       []MonthlyResult `json:"monthlyResults"`
	Trades               []Trade         `json:"trades"`
}

type MonthlyResult struct {
	Month          string `json:"month"`
	OpeningBalance string `json:"openingBalance"`
	ClosingBalance string `json:"closingBalance"`
	NetPnL         string `json:"netPnl"`
	ReturnRate     string `json:"returnRate"`
	TradeCount     int    `json:"tradeCount"`
	Wins           int    `json:"wins"`
	Losses         int    `json:"losses"`
}

type Trade struct {
	ID           string `json:"id"`
	Instrument   string `json:"instrument"`
	Direction    string `json:"direction"`
	Quantity     string `json:"quantity"`
	EntryPrice   string `json:"entryPrice"`
	ExitPrice    string `json:"exitPrice"`
	EntryAt      string `json:"entryAt"`
	ExitAt       string `json:"exitAt"`
	Fee          string `json:"fee"`
	NetPnL       string `json:"netPnl"`
	ReturnRate   string `json:"returnRate"`
	BalanceAfter string `json:"balanceAfter"`
}

type rawDocument struct {
	Document
	Slots []rawSlot `json:"slots"`
}

type rawSlot struct {
	Slot
	History json.RawMessage `json:"history"`
}

var (
	loadOnce sync.Once
	loaded   *Document
	loadErr  error
)

func StrategyID(slot int) string {
	return fmt.Sprintf("comkun-observation-history-%02d", slot)
}

func ArtifactSHA256() string {
	sum := sha256.Sum256(artifactJSON)
	return hex.EncodeToString(sum[:])
}

func Load() (*Document, error) {
	loadOnce.Do(func() { loaded, loadErr = parseAndValidate(artifactJSON) })
	return loaded, loadErr
}

func ByStrategyID(strategyID string) (*Slot, bool) {
	doc, err := Load()
	if err != nil {
		return nil, false
	}
	for i := range doc.Slots {
		if StrategyID(doc.Slots[i].Slot) == strings.TrimSpace(strategyID) {
			return &doc.Slots[i], true
		}
	}
	return nil, false
}

func parseAndValidate(raw []byte) (*Document, error) {
	if ArtifactSHA256() != ExpectedArtifactSHA256 {
		return nil, fmt.Errorf("observation artifact sha256 mismatch")
	}
	var source rawDocument
	if err := json.Unmarshal(raw, &source); err != nil {
		return nil, fmt.Errorf("decode observation artifact: %w", err)
	}
	doc := source.Document
	doc.Slots = make([]Slot, 0, len(source.Slots))
	if doc.SchemaVersion != 1 || doc.Status != StatusHistoricalSimulation || len(source.Slots) != 6 {
		return nil, fmt.Errorf("invalid observation artifact header")
	}
	seenAccounts, seenProfiles, seenTrades := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for i, rawSlot := range source.Slots {
		slot := rawSlot.Slot
		if err := json.Unmarshal(rawSlot.History, &slot.History); err != nil {
			return nil, fmt.Errorf("decode slot %d history: %w", slot.Slot, err)
		}
		if slot.Slot != i+1 || slot.SourceStrategyID != nil || slot.RealtimeFollowAvailable {
			return nil, fmt.Errorf("invalid slot %d mapping", slot.Slot)
		}
		if seenAccounts[slot.SourceAccountID] || seenProfiles[slot.HistoryProfileID] {
			return nil, fmt.Errorf("duplicate slot provenance at %d", slot.Slot)
		}
		seenAccounts[slot.SourceAccountID], seenProfiles[slot.HistoryProfileID] = true, true
		sum := sha256.Sum256(rawSlot.History)
		if hex.EncodeToString(sum[:]) != slot.HistorySHA256 {
			return nil, fmt.Errorf("slot %d history sha256 mismatch", slot.Slot)
		}
		if err := validateHistory(slot, seenTrades); err != nil {
			return nil, err
		}
		doc.Slots = append(doc.Slots, slot)
	}
	return &doc, nil
}

func validateHistory(slot Slot, seenTrades map[string]bool) error {
	h := slot.History
	if h.ID != slot.HistoryProfileID || h.Name != slot.DisplayName || h.Status != StatusHistoricalSimulation ||
		strings.TrimSpace(h.Disclosure) != "历史行情场景数据" || h.TradeCount != len(h.Trades) ||
		len(h.MonthlyResults) != h.Months+1 {
		return fmt.Errorf("slot %d history metadata mismatch", slot.Slot)
	}
	start, startErr := time.Parse(time.RFC3339Nano, h.StartAt)
	end, endErr := time.Parse(time.RFC3339Nano, h.EndAt)
	initial, initialErr := decimal(h.InitialBalance)
	current, currentErr := decimal(h.CurrentBalance)
	if startErr != nil || endErr != nil || !end.After(start) || initialErr != nil || currentErr != nil || initial <= 0 || current <= 0 {
		return fmt.Errorf("slot %d invalid history boundary", slot.Slot)
	}
	trades := append([]Trade(nil), h.Trades...)
	sort.Slice(trades, func(i, j int) bool { return trades[i].EntryAt < trades[j].EntryAt })
	previous := initial
	for _, trade := range trades {
		if seenTrades[trade.ID] {
			return fmt.Errorf("duplicate observation trade %s", trade.ID)
		}
		seenTrades[trade.ID] = true
		entry, e1 := time.Parse(time.RFC3339Nano, trade.EntryAt)
		exit, e2 := time.Parse(time.RFC3339Nano, trade.ExitAt)
		quantity, e3 := decimal(trade.Quantity)
		entryPrice, e4 := decimal(trade.EntryPrice)
		exitPrice, e5 := decimal(trade.ExitPrice)
		fee, e6 := decimal(trade.Fee)
		pnl, e7 := decimal(trade.NetPnL)
		balance, e8 := decimal(trade.BalanceAfter)
		if e1 != nil || e2 != nil || !exit.After(entry) || entry.Before(start) || exit.After(end) ||
			e3 != nil || e4 != nil || e5 != nil || e6 != nil || e7 != nil || e8 != nil ||
			quantity <= 0 || entryPrice <= 0 || exitPrice <= 0 || fee < 0 || balance <= 0 ||
			(trade.Direction != "long" && trade.Direction != "short") || math.Abs((previous+pnl)-balance) > 0.011 {
			return fmt.Errorf("slot %d invalid trade %s", slot.Slot, trade.ID)
		}
		previous = balance
	}
	if math.Abs(previous-current) > 0.011 {
		return fmt.Errorf("slot %d ending balance mismatch", slot.Slot)
	}
	monthlyTrades := 0
	for _, month := range h.MonthlyResults {
		opening, e1 := decimal(month.OpeningBalance)
		closing, e2 := decimal(month.ClosingBalance)
		pnl, e3 := decimal(month.NetPnL)
		if e1 != nil || e2 != nil || e3 != nil || opening <= 0 || closing <= 0 ||
			month.TradeCount != month.Wins+month.Losses || math.Abs((opening+pnl)-closing) > 0.011 {
			return fmt.Errorf("slot %d invalid month %s", slot.Slot, month.Month)
		}
		monthlyTrades += month.TradeCount
	}
	if monthlyTrades != len(h.Trades) {
		return fmt.Errorf("slot %d monthly trade count mismatch", slot.Slot)
	}
	return nil
}

func decimal(raw string) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("invalid decimal")
	}
	return value, nil
}
