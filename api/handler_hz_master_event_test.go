package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"nofx/store"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newHZMasterEventTestServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&store.IntegrationMasterEvent{}, &store.ComkunMasterBroadcast{}); err != nil {
		t.Fatal(err)
	}
	st, err := store.NewFromGorm(db)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{store: st}, st
}

func TestHZMasterEventEndpointReplayAndSequenceGap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(hzMasterEventSecretEnv, "test-secret")
	server, st := newHZMasterEventTestServer(t)
	router := gin.New()
	router.POST("/api/integrations/hz/v1/master-events", server.handleHZMasterEvent)

	now := time.Now().UTC().Truncate(time.Millisecond)
	body := hzMasterEventBody(t, now, "event-1", 1)
	first := sendHZMasterEvent(t, router, body, "11111111-1111-4111-8111-111111111111", now)
	if first.Code != http.StatusOK {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}
	replay := sendHZMasterEvent(t, router, body, "11111111-1111-4111-8111-111111111111", now)
	if replay.Code != http.StatusOK {
		t.Fatalf("replay status=%d body=%s", replay.Code, replay.Body.String())
	}
	var replayResult store.IntegrationMasterEventResult
	if err := json.Unmarshal(replay.Body.Bytes(), &replayResult); err != nil || !replayResult.Duplicate {
		t.Fatalf("replay=%+v err=%v", replayResult, err)
	}

	gapBody := hzMasterEventBody(t, now, "event-3", 3)
	gap := sendHZMasterEvent(t, router, gapBody, "33333333-3333-4333-8333-333333333333", now)
	if gap.Code != http.StatusConflict || !bytes.Contains(gap.Body.Bytes(), []byte(`"expected_next_sequence":2`)) {
		t.Fatalf("gap status=%d body=%s", gap.Code, gap.Body.String())
	}

	var broadcasts []store.ComkunMasterBroadcast
	if err := st.GormDB().Find(&broadcasts).Error; err != nil || len(broadcasts) != 1 {
		t.Fatalf("broadcasts=%d err=%v", len(broadcasts), err)
	}
	if broadcasts[0].AnalysisText != "AI策略执行" || broadcasts[0].SourceStrategyID == "hz-ai-master:master-human-account" ||
		bytes.Contains([]byte(broadcasts[0].MasterStateJSON), []byte("master-human-account")) {
		t.Fatalf("public mirror state leaked master identity: %+v", broadcasts[0])
	}
}

func TestHZMasterEventEndpointRejectsExpiredSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(hzMasterEventSecretEnv, "test-secret")
	server, _ := newHZMasterEventTestServer(t)
	router := gin.New()
	router.POST("/api/integrations/hz/v1/master-events", server.handleHZMasterEvent)
	old := time.Now().Add(-10 * time.Minute)
	response := sendHZMasterEvent(t, router, hzMasterEventBody(t, old, "event-old", 1), "99999999-9999-4999-8999-999999999999", old)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestHZMasterEventPollingRepairsPushGap(t *testing.T) {
	server, st := newHZMasterEventTestServer(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	firstRaw := hzMasterEventBody(t, now, "event-1", 1)
	var first hzMasterEventRequest
	if err := json.Unmarshal(firstRaw, &first); err != nil {
		t.Fatal(err)
	}
	if result, err := server.acceptHZMasterEvent(first, firstRaw, "push-1", false, false); err != nil || !result.Accepted {
		t.Fatalf("first=%+v err=%v", result, err)
	}

	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/master-snapshot" || request.Header.Get("X-HZ-Signature") == "" ||
			request.Header.Get("X-HZ-API-KEY") != "poll-key" {
			http.Error(w, "bad poll request", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(hzMasterSnapshotResponse{
			LastSequence: "3", CapturedAt: now, MasterAccountID: "master-human-account",
			Account: hzMasterTestAccount(), Positions: hzMasterTestPositions(),
		})
	}))
	defer remote.Close()
	config := hzMasterPollConfig{
		APIURL: remote.URL + "/api/v1", APIKey: "poll-key", Secret: "poll-secret",
		MasterAccountID: "master-human-account", Interval: time.Minute,
	}
	if err := server.pollHZMasterEventsOnce(t.Context(), config); err != nil {
		t.Fatal(err)
	}
	latest, err := st.IntegrationMasterEvent().GetLatestSequence("hz", "master-human-account")
	if err != nil || latest != 3 {
		t.Fatalf("latest=%d err=%v", latest, err)
	}
	var count int64
	if err := st.GormDB().Model(&store.ComkunMasterBroadcast{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("broadcast count=%d err=%v", count, err)
	}
	var latestBroadcast store.ComkunMasterBroadcast
	if err := st.GormDB().Order("id DESC").First(&latestBroadcast).Error; err != nil ||
		!bytes.Contains([]byte(latestBroadcast.MasterStateJSON), []byte(`"polling_reconcile":true`)) {
		t.Fatalf("poll reconciliation must be close-only: state=%s err=%v", latestBroadcast.MasterStateJSON, err)
	}
}

func TestHZMasterEventPollingAcceptsEmptySequenceZero(t *testing.T) {
	server, st := newHZMasterEventTestServer(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(hzMasterSnapshotResponse{
			LastSequence: "0", CapturedAt: now, MasterAccountID: "master-human-account",
			Account: hzMasterTestAccount(), Positions: []hzMasterPosition{},
		})
	}))
	defer remote.Close()
	config := hzMasterPollConfig{
		APIURL: remote.URL + "/api/v1", APIKey: "poll-key", Secret: "poll-secret",
		MasterAccountID: "master-human-account", Interval: time.Minute,
	}
	if err := server.pollHZMasterEventsOnce(t.Context(), config); err != nil {
		t.Fatal(err)
	}
	var events, broadcasts int64
	if err := st.GormDB().Model(&store.IntegrationMasterEvent{}).Count(&events).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.GormDB().Model(&store.ComkunMasterBroadcast{}).Count(&broadcasts).Error; err != nil {
		t.Fatal(err)
	}
	if events != 0 || broadcasts != 0 {
		t.Fatalf("events=%d broadcasts=%d", events, broadcasts)
	}
}

func hzMasterEventBody(t *testing.T, occurredAt time.Time, eventID string, sequence int64) []byte {
	t.Helper()
	value := hzMasterEventRequest{
		EventID: eventID, Sequence: strconv.FormatInt(sequence, 10), EventType: "RECONCILE",
		OccurredAt: occurredAt, MasterAccountID: "master-human-account",
		Account: hzMasterTestAccount(), Positions: hzMasterTestPositions(),
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func hzMasterTestAccount() hzMasterAccount {
	return hzMasterAccount{
		AccountID: "master-human-account", AccountScope: "AI", WalletID: "ai:wallet-1",
		PositionBookID: "ai:book-1", Currency: "USD", Balance: "1000", Equity: "1000",
		AvailableMargin: "750", UsedMargin: "250", MarginRatio: "0.25", UnrealizedPnL: "0", Tradable: true,
	}
}

func hzMasterTestPositions() []hzMasterPosition {
	return []hzMasterPosition{{
		PositionID: "position-1", Instrument: "BTC-PERP", Side: "LONG", MarginMode: "CROSS",
		Leverage: 10, Lots: "0.25", PositionValue: "250", EntryPrice: "1000", CurrentPrice: "1000",
	}}
}

func sendHZMasterEvent(t *testing.T, handler http.Handler, body []byte, nonce string, timestamp time.Time) *httptest.ResponseRecorder {
	t.Helper()
	var payload hzMasterEventRequest
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	timestampValue := strconv.FormatInt(timestamp.UnixMilli(), 10)
	mac := hmac.New(sha256.New, []byte("test-secret"))
	_, _ = mac.Write([]byte(timestampValue + "\n" + nonce + "\n"))
	_, _ = mac.Write(body)
	request := httptest.NewRequest(http.MethodPost, "/api/integrations/hz/v1/master-events", bytes.NewReader(body))
	request.Header.Set("X-HZ-Event-Id", payload.EventID)
	request.Header.Set("X-HZ-Event-Timestamp", timestampValue)
	request.Header.Set("X-HZ-Event-Nonce", nonce)
	request.Header.Set("X-HZ-Event-Signature", hex.EncodeToString(mac.Sum(nil)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
