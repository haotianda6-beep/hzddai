package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"nofx/kernel"
	"nofx/store"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	hzMasterEventSecretEnv = "COMKUN_AI_EVENT_SECRET"
	hzMasterEventMaxBody   = 2 << 20
	hzMasterEventWindow    = 5 * time.Minute
)

type hzMasterEventRequest struct {
	EventID         string             `json:"eventId"`
	Sequence        string             `json:"sequence"`
	EventType       string             `json:"eventType"`
	OccurredAt      time.Time          `json:"occurredAt"`
	MasterAccountID string             `json:"masterAccountId"`
	Account         hzMasterAccount    `json:"account"`
	Positions       []hzMasterPosition `json:"positions"`
}

type hzMasterSnapshotResponse struct {
	LastSequence    string             `json:"lastSequence"`
	CapturedAt      time.Time          `json:"capturedAt"`
	MasterAccountID string             `json:"masterAccountId"`
	Account         hzMasterAccount    `json:"account"`
	Positions       []hzMasterPosition `json:"positions"`
}

type hzMasterAccount struct {
	AccountID       string `json:"accountId"`
	AccountScope    string `json:"accountScope"`
	WalletID        string `json:"walletId"`
	PositionBookID  string `json:"positionBookId"`
	Currency        string `json:"currency"`
	Balance         string `json:"balance"`
	Equity          string `json:"equity"`
	AvailableMargin string `json:"availableMargin"`
	UsedMargin      string `json:"usedMargin"`
	MarginRatio     string `json:"marginRatio"`
	UnrealizedPnL   string `json:"unrealizedPnl"`
	Tradable        bool   `json:"tradable"`
}

type hzMasterPosition struct {
	PositionID    string  `json:"positionId"`
	Instrument    string  `json:"instrument"`
	Side          string  `json:"side"`
	MarginMode    string  `json:"marginMode"`
	Leverage      int     `json:"leverage"`
	Lots          string  `json:"lots"`
	EntryPrice    string  `json:"entryPrice"`
	CurrentPrice  string  `json:"currentPrice"`
	PositionValue string  `json:"positionValue"`
	InitialMargin string  `json:"initialMargin"`
	UnrealizedPnL string  `json:"unrealizedPnl"`
	TakeProfit    *string `json:"takeProfit"`
	StopLoss      *string `json:"stopLoss"`
	OpenedAt      string  `json:"openedAt"`
}

type hzMasterState struct {
	Version          int                   `json:"v"`
	SourceEventID    string                `json:"source_event_id"`
	SourceSequence   int64                 `json:"source_sequence"`
	OccurredAt       time.Time             `json:"occurred_at"`
	EventType        string                `json:"event_type"`
	PollingReconcile bool                  `json:"polling_reconcile,omitempty"`
	Positions        []kernel.PositionInfo `json:"positions"`
	PendingOrders    []any                 `json:"pending_orders"`
}

func (s *Server) handleHZMasterEvent(c *gin.Context) {
	secret := strings.TrimSpace(os.Getenv(hzMasterEventSecretEnv))
	if secret == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "integration is not configured"})
		return
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, hzMasterEventMaxBody+1))
	if err != nil || len(raw) == 0 || len(raw) > hzMasterEventMaxBody {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	eventID := strings.TrimSpace(c.GetHeader("X-HZ-Event-Id"))
	timestamp := strings.TrimSpace(c.GetHeader("X-HZ-Event-Timestamp"))
	nonce := strings.TrimSpace(c.GetHeader("X-HZ-Event-Nonce"))
	signature := strings.TrimSpace(c.GetHeader("X-HZ-Event-Signature"))
	if err := verifyHZMasterEventAuth(secret, timestamp, nonce, signature, raw, time.Now()); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid integration authentication"})
		return
	}
	var request hzMasterEventRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event JSON"})
		return
	}
	if eventID == "" || eventID != strings.TrimSpace(request.EventID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "event id header mismatch"})
		return
	}
	if err := request.validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// HZ uses one global BIGSERIAL across masters. Per-master sequences are
	// strictly increasing but can jump when another master publishes between
	// two events, so the HTTP ingress must accept monotonic gaps.
	result, err := s.acceptHZMasterEvent(request, raw, nonce, true, false)
	if errors.Is(err, store.ErrIntegrationNonceReplay) {
		c.JSON(http.StatusConflict, gin.H{"error": "nonce replay"})
		return
	}
	if err != nil {
		SafeInternalError(c, "Accept HZ master event", err)
		return
	}
	if result.Gap {
		c.JSON(http.StatusConflict, gin.H{
			"accepted": false, "code": "SEQUENCE_GAP",
			"expected_next_sequence": result.ExpectedNextSequence,
		})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) acceptHZMasterEvent(request hzMasterEventRequest, raw []byte, nonce string, allowGap, pollingReconcile bool) (store.IntegrationMasterEventResult, error) {
	stateJSON, err := request.masterStateJSON(pollingReconcile)
	if err != nil {
		return store.IntegrationMasterEventResult{}, err
	}
	sequence, _ := strconv.ParseInt(request.Sequence, 10, 64)
	equity, _ := parseHZDecimal(request.Account.Equity)
	payloadHash := sha256.Sum256(raw)
	provider := "hz"
	if pollingReconcile {
		provider = "hz-reconcile"
	}
	result, err := s.store.IntegrationMasterEvent().Accept(store.IntegrationMasterEventInput{
		Provider: provider, MasterAccountID: request.MasterAccountID, EventID: request.EventID,
		Sequence: sequence, EventType: request.EventType, OccurredAt: request.OccurredAt,
		PayloadVersion: 1, PayloadSHA256: hex.EncodeToString(payloadHash[:]),
		PayloadJSON: string(raw), AuthNonce: nonce, MasterEquity: equity, MasterStateJSON: stateJSON,
	}, allowGap)
	if err == nil && !result.Gap && (result.Accepted || result.Duplicate) && s.watchHZMarket != nil {
		s.watchHZMasterPositions(request.Positions)
	}
	return result, err
}

func (s *Server) watchHZMasterPositions(positions []hzMasterPosition) {
	if s.watchHZMarket == nil {
		return
	}
	for _, position := range positions {
		s.watchHZMarket(position.Instrument)
	}
}

func verifyHZMasterEventAuth(secret, timestamp, nonce, signature string, body []byte, now time.Time) error {
	if secret == "" || len(timestamp) != 13 || nonce == "" || signature == "" {
		return fmt.Errorf("missing authentication")
	}
	if _, err := uuid.Parse(nonce); err != nil {
		return fmt.Errorf("invalid nonce")
	}
	milliseconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return err
	}
	requestTime := time.UnixMilli(milliseconds)
	if requestTime.Before(now.Add(-hzMasterEventWindow)) || requestTime.After(now.Add(hzMasterEventWindow)) {
		return fmt.Errorf("timestamp outside allowed window")
	}
	provided, err := hex.DecodeString(signature)
	if err != nil {
		return err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "\n" + nonce + "\n"))
	_, _ = mac.Write(body)
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}

func (request hzMasterEventRequest) validate() error {
	sequence, err := strconv.ParseInt(request.Sequence, 10, 64)
	eventType := strings.ToUpper(strings.TrimSpace(request.EventType))
	validEventTypes := map[string]bool{
		"OPEN": true, "INCREASE": true, "REDUCE": true, "CLOSE": true,
		"PROTECTION": true, "LIQUIDATION": true, "RECONCILE": true,
	}
	if strings.TrimSpace(request.EventID) == "" || err != nil || sequence <= 0 || !validEventTypes[eventType] ||
		request.OccurredAt.IsZero() || strings.TrimSpace(request.MasterAccountID) == "" {
		return fmt.Errorf("invalid master event")
	}
	if err := request.Account.validate(); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(request.Positions))
	for _, position := range request.Positions {
		if err := position.validate(); err != nil {
			return err
		}
		positionID := strings.TrimSpace(position.PositionID)
		if _, duplicate := seen[positionID]; duplicate {
			return fmt.Errorf("duplicate positionId")
		}
		seen[positionID] = struct{}{}
	}
	return nil
}

func (account hzMasterAccount) validate() error {
	equity, equityErr := parseHZDecimal(account.Equity)
	if strings.TrimSpace(account.AccountID) == "" || !strings.EqualFold(strings.TrimSpace(account.AccountScope), "AI") ||
		!strings.HasPrefix(strings.TrimSpace(account.WalletID), "ai:") ||
		!strings.HasPrefix(strings.TrimSpace(account.PositionBookID), "ai:") ||
		strings.TrimSpace(account.Currency) == "" || equityErr != nil || equity <= 0 {
		return fmt.Errorf("invalid AI account snapshot")
	}
	for _, value := range []string{account.Balance, account.AvailableMargin, account.UsedMargin, account.MarginRatio, account.UnrealizedPnL} {
		if _, err := parseHZDecimal(value); err != nil {
			return fmt.Errorf("invalid AI account decimal")
		}
	}
	return nil
}

func (position hzMasterPosition) validate() error {
	lots, lotsErr := parseHZDecimal(position.Lots)
	positionValue, valueErr := parseHZDecimal(position.PositionValue)
	initialMargin, marginErr := parseHZDecimal(position.InitialMargin)
	currentPrice, priceErr := parseHZDecimal(position.CurrentPrice)
	side := strings.ToUpper(strings.TrimSpace(position.Side))
	marginMode := strings.ToUpper(strings.TrimSpace(position.MarginMode))
	if strings.TrimSpace(position.PositionID) == "" || strings.TrimSpace(position.Instrument) == "" ||
		(side != "LONG" && side != "SHORT") || position.Leverage <= 0 || lotsErr != nil || lots <= 0 ||
		valueErr != nil || positionValue < 0 || marginErr != nil || initialMargin <= 0 ||
		priceErr != nil || currentPrice <= 0 || (marginMode != "CROSS" && marginMode != "ISOLATED") {
		return fmt.Errorf("invalid master position")
	}
	return nil
}

func parseHZDecimal(value string) (float64, error) {
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, fmt.Errorf("invalid decimal")
	}
	return number, nil
}

func (request hzMasterEventRequest) masterStateJSON(pollingReconcile bool) (string, error) {
	sequence, _ := strconv.ParseInt(request.Sequence, 10, 64)
	positions := make([]kernel.PositionInfo, 0, len(request.Positions))
	for _, item := range request.Positions {
		lots, _ := parseHZDecimal(item.Lots)
		positionValue, _ := parseHZDecimal(item.PositionValue)
		initialMargin, _ := parseHZDecimal(item.InitialMargin)
		entryPrice, _ := parseHZDecimal(item.EntryPrice)
		currentPrice, _ := parseHZDecimal(item.CurrentPrice)
		unrealizedPnL, _ := parseHZDecimal(item.UnrealizedPnL)
		positions = append(positions, kernel.PositionInfo{
			PositionID: strings.TrimSpace(item.PositionID), Symbol: strings.ToUpper(strings.TrimSpace(item.Instrument)),
			Side: strings.ToLower(strings.TrimSpace(item.Side)), MarginMode: strings.ToLower(strings.TrimSpace(item.MarginMode)),
			Lots: lots, Leverage: item.Leverage, PositionValue: positionValue, MarginUsed: initialMargin,
			EntryPrice: entryPrice, MarkPrice: currentPrice, UnrealizedPnL: unrealizedPnL,
		})
	}
	raw, err := json.Marshal(hzMasterState{
		Version: 1, SourceEventID: strings.TrimSpace(request.EventID), SourceSequence: sequence,
		OccurredAt: request.OccurredAt.UTC(), EventType: strings.ToUpper(strings.TrimSpace(request.EventType)),
		PollingReconcile: pollingReconcile,
		Positions:        positions, PendingOrders: []any{},
	})
	return string(raw), err
}
