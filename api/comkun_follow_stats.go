package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"nofx/store"

	"github.com/google/uuid"
)

type comkunFollowingStatsStrategy struct {
	MasterUserID            string `json:"masterUserId"`
	StrategyID              string `json:"strategyId"`
	StrategyName            string `json:"strategyName,omitempty"`
	RunningFollowerCount    int    `json:"runningFollowerCount"`
	SubscribedFollowerCount int    `json:"subscribedFollowerCount"`
}

type comkunFollowingStatsPayload struct {
	ContractVersion string                         `json:"contractVersion"`
	Source          string                         `json:"source"`
	SourceEventID   string                         `json:"sourceEventId"`
	ObservedAt      string                         `json:"observedAt"`
	Strategies      []comkunFollowingStatsStrategy `json:"strategies"`
}

func filterComkunFollowingStatsRows(rows []store.ComkunFollowingStatsRow, mapping map[string]string) []store.ComkunFollowingStatsRow {
	filtered := make([]store.ComkunFollowingStatsRow, 0, len(rows))
	for _, row := range rows {
		if _, ok := mapping[strings.TrimSpace(row.SourceStrategyID)]; ok {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func buildComkunFollowingStatsPayload(rows []store.ComkunFollowingStatsRow, mapping map[string]string, observedAt time.Time) ([]byte, comkunFollowingStatsPayload, error) {
	if len(rows) > 1000 {
		return nil, comkunFollowingStatsPayload{}, fmt.Errorf("too many strategies")
	}
	observedAt = observedAt.UTC()
	byKey := make(map[string]comkunFollowingStatsStrategy, len(rows))
	for _, row := range rows {
		sourceID := strings.TrimSpace(row.SourceStrategyID)
		strategyID := strings.TrimSpace(row.StrategyID)
		masterID := strings.TrimSpace(mapping[sourceID])
		if sourceID == "" || strategyID == "" || masterID == "" {
			return nil, comkunFollowingStatsPayload{}, fmt.Errorf("stable master/strategy ID missing")
		}
		if row.RunningFollowerCount < 0 || row.SubscribedFollowerCount < 0 || row.RunningTraderCount < 0 {
			return nil, comkunFollowingStatsPayload{}, fmt.Errorf("counts must be non-negative")
		}
		if row.RunningFollowerCount > row.SubscribedFollowerCount {
			return nil, comkunFollowingStatsPayload{}, fmt.Errorf("running count exceeds subscribed count")
		}
		item := comkunFollowingStatsStrategy{
			MasterUserID: masterID, StrategyID: strategyID, StrategyName: strings.TrimSpace(row.StrategyName),
			RunningFollowerCount: row.RunningFollowerCount, SubscribedFollowerCount: row.SubscribedFollowerCount,
		}
		key := masterID + "\x00" + strategyID
		if previous, exists := byKey[key]; exists {
			if previous.RunningFollowerCount != item.RunningFollowerCount || previous.SubscribedFollowerCount != item.SubscribedFollowerCount {
				return nil, comkunFollowingStatsPayload{}, fmt.Errorf("conflicting strategy snapshot")
			}
			continue
		}
		byKey[key] = item
	}
	entries := make([]comkunFollowingStatsStrategy, 0, len(byKey))
	for _, item := range byKey {
		entries = append(entries, item)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].MasterUserID != entries[j].MasterUserID {
			return entries[i].MasterUserID < entries[j].MasterUserID
		}
		return entries[i].StrategyID < entries[j].StrategyID
	})
	canonical, err := json.Marshal(entries)
	if err != nil {
		return nil, comkunFollowingStatsPayload{}, err
	}
	eventHash := sha256.Sum256(append([]byte(observedAt.Format(time.RFC3339Nano)), canonical...))
	payload := comkunFollowingStatsPayload{
		ContractVersion: "1", Source: "comkun",
		SourceEventID: "comkun-follow-stats-" + hex.EncodeToString(eventHash[:12]),
		ObservedAt:    observedAt.Format(time.RFC3339Nano), Strategies: entries,
	}
	raw, err := json.Marshal(payload)
	return raw, payload, err
}

func containsJSONKey(raw []byte, key string) bool {
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	if _, ok := value[key]; ok {
		return true
	}
	var nested struct {
		Strategies []map[string]json.RawMessage `json:"strategies"`
	}
	if json.Unmarshal(raw, &nested) != nil {
		return false
	}
	for _, item := range nested.Strategies {
		if _, ok := item[key]; ok {
			return true
		}
	}
	return false
}

func signComkunStats(secret, timestamp, nonce string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write([]byte(nonce))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

type comkunStatsDelivery struct {
	client *http.Client
	sleep  func(time.Duration)
}

func newComkunStatsDelivery(client *http.Client, sleep func(time.Duration)) *comkunStatsDelivery {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	if sleep == nil {
		sleep = time.Sleep
	}
	return &comkunStatsDelivery{client: client, sleep: sleep}
}

func (d *comkunStatsDelivery) send(ctx context.Context, target, secret, eventID string, body []byte) error {
	target = strings.TrimSpace(target)
	secret = strings.TrimSpace(secret)
	eventID = strings.TrimSpace(eventID)
	if target == "" || secret == "" || eventID == "" || len(body) == 0 {
		return fmt.Errorf("stats delivery target, secret, event ID and body are required")
	}
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	nonce := uuid.NewString()
	signature := signComkunStats(secret, timestamp, nonce, body)
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-HZ-Event-Id", eventID)
		req.Header.Set("X-HZ-Event-Timestamp", timestamp)
		req.Header.Set("X-HZ-Event-Nonce", nonce)
		req.Header.Set("X-HZ-Event-Signature", signature)
		resp, err := d.client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
			lastErr = fmt.Errorf("stats delivery status=%d", resp.StatusCode)
			if resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
				return lastErr
			}
		} else {
			lastErr = err
		}
		if attempt == 4 {
			break
		}
		d.sleep(time.Duration(1<<attempt) * time.Second)
	}
	return lastErr
}
