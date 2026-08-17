package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"nofx/store"

	"github.com/gin-gonic/gin"
)

func TestBuildComkunFollowingStatsPayloadDeduplicatesStableKeysAndValidatesCounts(t *testing.T) {
	rows := []store.ComkunFollowingStatsRow{
		{StrategyID: "listing-1", StrategyName: "One", SourceStrategyID: "source-1", RunningFollowerCount: 2, RunningTraderCount: 3, SubscribedFollowerCount: 4},
		{StrategyID: "listing-1", StrategyName: "One", SourceStrategyID: "source-1", RunningFollowerCount: 2, RunningTraderCount: 3, SubscribedFollowerCount: 4},
	}
	raw, payload, err := buildComkunFollowingStatsPayload(rows, map[string]string{"source-1": "master-1"}, time.Date(2026, 8, 17, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Strategies) != 1 || payload.Strategies[0].RunningFollowerCount != 2 || payload.Strategies[0].SubscribedFollowerCount != 4 {
		t.Fatalf("payload=%+v, want one deduplicated strategy", payload)
	}
	if len(raw) == 0 || payload.SourceEventID == "" {
		t.Fatalf("raw/source event missing: raw=%d payload=%+v", len(raw), payload)
	}

	rows[0].RunningFollowerCount = 5
	if _, _, err := buildComkunFollowingStatsPayload(rows[:1], map[string]string{"source-1": "master-1"}, time.Now().UTC()); err == nil {
		t.Fatal("runningFollowerCount > subscribedFollowerCount must fail")
	}
}

func TestComkunStatsDeliveryRetriesWithIdenticalSignedBody(t *testing.T) {
	var mu sync.Mutex
	var calls int
	var bodies [][]byte
	var headers []http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		mu.Lock()
		calls++
		bodies = append(bodies, append([]byte(nil), body...))
		headers = append(headers, r.Header.Clone())
		current := calls
		mu.Unlock()
		if current == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	raw := []byte(`{"contractVersion":"1","source":"comkun"}`)
	delivery := newComkunStatsDelivery(server.Client(), func(time.Duration) {})
	if err := delivery.send(context.Background(), server.URL, "test-secret", "event-1", raw); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 2 || !reflect.DeepEqual(bodies[0], bodies[1]) {
		t.Fatalf("calls=%d bodies_equal=%t", calls, reflect.DeepEqual(bodies[0], bodies[1]))
	}
	for _, h := range headers {
		if h.Get("X-HZ-Event-Id") != "event-1" || h.Get("X-HZ-Event-Signature") == "" || h.Get("X-HZ-Event-Nonce") == "" || len(h.Get("X-HZ-Event-Timestamp")) != 13 {
			t.Fatalf("invalid integration headers: %v", h)
		}
		if got, want := h.Get("X-HZ-Event-Signature"), signComkunStats("test-secret", h.Get("X-HZ-Event-Timestamp"), h.Get("X-HZ-Event-Nonce"), raw); got != want {
			t.Fatalf("signature=%s want=%s", got, want)
		}
	}
	if headers[0].Get("X-HZ-Event-Nonce") != headers[1].Get("X-HZ-Event-Nonce") || headers[0].Get("X-HZ-Event-Signature") != headers[1].Get("X-HZ-Event-Signature") {
		t.Fatal("retry must keep nonce and signature stable")
	}
}

func TestComkunStatsDeliveryDoesNotRetryOrdinary4xx(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	delivery := newComkunStatsDelivery(server.Client(), func(time.Duration) {})
	if err := delivery.send(context.Background(), server.URL, "test-secret", "event-4xx", []byte(`{}`)); err == nil {
		t.Fatal("ordinary 4xx must return an error")
	}
	if calls != 1 {
		t.Fatalf("calls=%d, ordinary 4xx must not retry", calls)
	}
}

func TestComkunStatsPayloadUsesContractJSONNames(t *testing.T) {
	_, payload, err := buildComkunFollowingStatsPayload([]store.ComkunFollowingStatsRow{{
		StrategyID: "strategy-1", StrategyName: "Name", SourceStrategyID: "source-1", RunningFollowerCount: 1, SubscribedFollowerCount: 2,
	}}, map[string]string{"source-1": "master-1"}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"contractVersion", "sourceEventId", "observedAt", "masterUserId", "strategyId", "runningFollowerCount", "subscribedFollowerCount"} {
		if !containsJSONKey(raw, name) {
			t.Fatalf("payload missing contract key %q: %s", name, raw)
		}
	}
}

func TestComkunPublisherFiltersRowsOutsideActiveMasterAllowlist(t *testing.T) {
	rows := []store.ComkunFollowingStatsRow{
		{StrategyID: "active-listing", SourceStrategyID: "source-active"},
		{StrategyID: "inactive-listing", SourceStrategyID: "source-inactive"},
	}
	filtered := filterComkunFollowingStatsRows(rows, map[string]string{"source-active": "master-active"})
	if len(filtered) != 1 || filtered[0].StrategyID != "active-listing" {
		t.Fatalf("filtered=%+v, want only active listing", filtered)
	}
}

func TestScopeComkunFollowingStatsDoesNotExposeOtherMasters(t *testing.T) {
	rows := []store.ComkunFollowingStatsRow{
		{StrategyID: "strategy-1", SourceStrategyID: "source-1"},
		{StrategyID: "strategy-2", SourceStrategyID: "source-2"},
	}
	mapping := map[string]string{"source-1": "master-1", "source-2": "master-2"}
	scoped := scopeComkunFollowingStats(rows, mapping, "master-1", nil)
	if len(scoped) != 1 || scoped[0].StrategyID != "strategy-1" {
		t.Fatalf("master-1 scope=%+v, want only strategy-1", scoped)
	}
	if scoped := scopeComkunFollowingStats(rows, mapping, "ordinary-user", nil); len(scoped) != 0 {
		t.Fatalf("ordinary user scope=%+v, want empty", scoped)
	}
}
func TestComkunStatsAdminMiddlewareBlocksMasterAndAllowsAdmin(t *testing.T) {
	t.Setenv("COMKUN_ADMIN_EMAILS", "admin@example.invalid")
	server := &Server{}
	router := gin.New()
	router.GET("/master", func(c *gin.Context) { c.Set("email", "master@example.invalid") }, server.adminMiddleware(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/admin", func(c *gin.Context) { c.Set("email", "admin@example.invalid") }, server.adminMiddleware(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	masterReq := httptest.NewRequest(http.MethodGet, "/master", nil)
	masterResp := httptest.NewRecorder()
	router.ServeHTTP(masterResp, masterReq)
	if masterResp.Code != http.StatusForbidden {
		t.Fatalf("master status=%d, want 403", masterResp.Code)
	}
	adminReq := httptest.NewRequest(http.MethodGet, "/admin", nil)
	adminResp := httptest.NewRecorder()
	router.ServeHTTP(adminResp, adminReq)
	if adminResp.Code != http.StatusNoContent {
		t.Fatalf("admin status=%d, want 204", adminResp.Code)
	}
}
