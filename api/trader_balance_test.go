package api

import (
	"errors"
	"testing"
)

func TestResolveTraderInitialBalanceDoesNotFallbackAfterAccountFailure(t *testing.T) {
	requested := 123.0
	got, err := resolveTraderInitialBalance("hz", requested, nil, errors.New("HZ API RATE_LIMITED"))
	if err == nil || got != 0 {
		t.Fatalf("account failure must stay unknown, got=%v err=%v", got, err)
	}
}

func TestResolveTraderInitialBalanceRejectsRealZero(t *testing.T) {
	got, err := resolveTraderInitialBalance("hz", 123, map[string]interface{}{"total_equity": 0.0}, nil)
	if err == nil || got != 0 {
		t.Fatalf("real zero must not use user input, got=%v err=%v", got, err)
	}
}

func TestResolveTraderInitialBalanceUsesRemoteEquity(t *testing.T) {
	got, err := resolveTraderInitialBalance("hz", 123, map[string]interface{}{"total_equity": 5000.0}, nil)
	if err != nil || got != 5000 {
		t.Fatalf("remote equity should be used, got=%v err=%v", got, err)
	}
}
