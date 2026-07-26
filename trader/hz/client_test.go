package hz

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestClientSignsExactRequestBytes(t *testing.T) {
	var received map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = map[string]string{
			"path":      r.URL.Path,
			"key":       r.Header.Get("X-HZ-API-KEY"),
			"timestamp": r.Header.Get("X-HZ-TIMESTAMP"),
			"nonce":     r.Header.Get("X-HZ-NONCE"),
			"signature": r.Header.Get("X-HZ-SIGNATURE"),
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"currency": "USD"})
	}))
	defer server.Close()

	client, err := newClient(server.URL+"/api/v1", "test-key", "hz_test_secret_2026")
	if err != nil {
		t.Fatal(err)
	}
	client.now = func() time.Time { return time.UnixMilli(1784956800000) }
	client.nonce = func() string { return "node-go-0001" }
	var response map[string]string
	if err := client.do(context.Background(), http.MethodGet, "/account", nil, "", &response); err != nil {
		t.Fatal(err)
	}
	if received["path"] != "/api/v1/account" || received["key"] != "test-key" {
		t.Fatalf("wrong request: %#v", received)
	}
	if received["signature"] != "9f44febd0fce0fe72dab45b91bacdf77a134ee7f7093e2acf0049e3968d5eaef" {
		t.Fatalf("wrong signature: %#v", received)
	}
}

func TestClientReturnsStableAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"code": "STALE_QUOTE", "message": "行情暂停"},
		})
	}))
	defer server.Close()

	client, _ := newClient(server.URL+"/api/v1", "key", "secret")
	var response map[string]any
	err := client.do(context.Background(), http.MethodGet, "/account", nil, "", &response)
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Code != "STALE_QUOTE" || apiErr.Status != http.StatusConflict {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestIdempotentWriteRetriesTemporaryFailureWithSameKey(t *testing.T) {
	var mu sync.Mutex
	var keys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		keys = append(keys, r.Header.Get("X-HZ-IDEMPOTENCY-KEY"))
		attempt := len(keys)
		mu.Unlock()
		if attempt == 1 {
			http.Error(w, "temporary", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "FILLED"})
	}))
	defer server.Close()

	client, _ := newClient(server.URL+"/api/v1", "key", "secret")
	var response map[string]string
	err := client.doJSON(context.Background(), http.MethodPost, "/orders",
		map[string]string{"symbol": "XAUUSD"}, &response, "fixed-idempotency")
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(keys) != 2 || keys[0] != "fixed-idempotency" || keys[1] != keys[0] {
		t.Fatalf("wrong retry keys: %#v", keys)
	}
}
