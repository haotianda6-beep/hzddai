package hz

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

func TestGetBalanceRejectsMissingAccountFieldsInsteadOfReturningZeros(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/account" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"accountId": "account"})
	}))
	defer server.Close()

	trader := newTestTrader(t, server.URL+"/api/v1", true)
	if balance, err := trader.GetBalance(); err == nil || balance != nil {
		t.Fatalf("missing account values must be an error, balance=%#v err=%v", balance, err)
	}
}

func TestGetBalancePreservesLegitimateZeroValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/account" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accountId": "account", "accountScope": "AI", "walletId": "wallet", "positionBookId": "book",
			"currency": "USD", "balance": "0", "equity": "0", "availableMargin": "0",
			"usedMargin": "0", "marginRatio": "0", "unrealizedPnl": "0", "tradable": true,
		})
	}))
	defer server.Close()

	trader := newTestTrader(t, server.URL+"/api/v1", true)
	balance, err := trader.GetBalance()
	if err != nil {
		t.Fatal(err)
	}
	if balance["total_equity"] != float64(0) || balance["availableBalance"] != float64(0) {
		t.Fatalf("legitimate zero must remain a numeric zero: %#v", balance)
	}
}

func TestConcurrentGetBalanceUsesOneAccountRequest(t *testing.T) {
	var accountRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/account" {
			http.NotFound(w, r)
			return
		}
		accountRequests.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accountId": "account", "accountScope": "AI", "walletId": "wallet", "positionBookId": "book",
			"currency": "USD", "balance": "5000", "equity": "5000", "availableMargin": "5000",
			"usedMargin": "0", "marginRatio": "0", "unrealizedPnl": "0", "tradable": true,
		})
	}))
	defer server.Close()

	first := newTestTrader(t, server.URL+"/api/v1", true)
	second := newTestTrader(t, server.URL+"/api/v1", true)
	var wg sync.WaitGroup
	for _, trader := range []*Trader{first, second} {
		wg.Add(1)
		go func(trader *Trader) {
			defer wg.Done()
			if _, err := trader.GetBalance(); err != nil {
				t.Errorf("GetBalance: %v", err)
			}
		}(trader)
	}
	wg.Wait()
	if got := accountRequests.Load(); got != 1 {
		t.Fatalf("account requests=%d, want one shared in-flight request", got)
	}
}

func TestGET429RetriesWithRetryAfterAndStopsAtBound(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/account" {
			http.NotFound(w, r)
			return
		}
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{"code": "RATE_LIMITED", "message": "slow down"},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accountId": "account", "accountScope": "AI", "walletId": "wallet", "positionBookId": "book",
			"currency": "USD", "balance": "5000", "equity": "5000", "availableMargin": "5000",
			"usedMargin": "0", "marginRatio": "0", "unrealizedPnl": "0", "tradable": true,
		})
	}))
	defer server.Close()

	client, err := newClient(server.URL+"/api/v1", "key", "secret")
	if err != nil {
		t.Fatal(err)
	}
	var account account
	if err := client.do(context.Background(), http.MethodGet, "/account", nil, "", &account); err != nil {
		t.Fatal(err)
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("GET attempts=%d, want one bounded retry", got)
	}
}
func TestGET429RetryCountIsBounded(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "RATE_LIMITED", "message": "slow down"}})
	}))
	defer server.Close()

	client, err := newClient(server.URL+"/api/v1", "key", "secret")
	if err != nil {
		t.Fatal(err)
	}
	var account account
	if err := client.do(context.Background(), http.MethodGet, "/account", nil, "", &account); err == nil {
		t.Fatal("repeated 429 responses must remain an error")
	}
	if got := attempts.Load(); got != 3 {
		t.Fatalf("GET attempts=%d, want initial request plus two bounded retries", got)
	}
}
