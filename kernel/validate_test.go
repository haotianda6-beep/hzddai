package kernel

import (
	"testing"
)

// TestLeverageFallback tests automatic correction when leverage exceeds limit
func TestLeverageFallback(t *testing.T) {
	tests := []struct {
		name            string
		decision        Decision
		accountEquity   float64
		btcEthLeverage  int
		altcoinLeverage int
		wantLeverage    int // Expected leverage after correction
		wantError       bool
	}{
		{
			name: "Altcoin leverage exceeded - auto-correct to limit",
			decision: Decision{
				Symbol:          "SOLUSDT",
				Action:          "open_long",
				Leverage:        20, // Exceeds limit
				PositionSizeUSD: 100,
				StopLoss:        50,
				TakeProfit:      200,
			},
			accountEquity:   100,
			btcEthLeverage:  10,
			altcoinLeverage: 5, // Limit 5x
			wantLeverage:    5, // Should be corrected to 5
			wantError:       false,
		},
		{
			name: "BTC leverage exceeded - auto-correct to limit",
			decision: Decision{
				Symbol:          "BTCUSDT",
				Action:          "open_long",
				Leverage:        20, // Exceeds limit
				PositionSizeUSD: 1000,
				StopLoss:        90000,
				TakeProfit:      110000,
			},
			accountEquity:   100,
			btcEthLeverage:  10, // Limit 10x
			altcoinLeverage: 5,
			wantLeverage:    10, // Should be corrected to 10
			wantError:       false,
		},
		{
			name: "Leverage within limit - no correction",
			decision: Decision{
				Symbol:          "ETHUSDT",
				Action:          "open_short",
				Leverage:        5, // Not exceeded
				PositionSizeUSD: 500,
				StopLoss:        4000,
				TakeProfit:      3000,
			},
			accountEquity:   100,
			btcEthLeverage:  10,
			altcoinLeverage: 5,
			wantLeverage:    5, // Stays unchanged
			wantError:       false,
		},
		{
			name: "Leverage is 0 - should error",
			decision: Decision{
				Symbol:          "SOLUSDT",
				Action:          "open_long",
				Leverage:        0, // Invalid
				PositionSizeUSD: 100,
				StopLoss:        50,
				TakeProfit:      200,
			},
			accountEquity:   100,
			btcEthLeverage:  10,
			altcoinLeverage: 5,
			wantLeverage:    0,
			wantError:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use default position value ratios for testing (10x for BTC/ETH, 1.5x for altcoins)
			err := validateDecision(&tt.decision, tt.accountEquity, tt.btcEthLeverage, tt.altcoinLeverage, 10.0, 1.5)

			// Check error status
			if (err != nil) != tt.wantError {
				t.Errorf("validateDecision() error = %v, wantError %v", err, tt.wantError)
				return
			}

			// If shouldn't error, check if leverage was correctly corrected
			if !tt.wantError && tt.decision.Leverage != tt.wantLeverage {
				t.Errorf("Leverage not corrected: got %d, want %d", tt.decision.Leverage, tt.wantLeverage)
			}
		})
	}
}

func TestValidateJSONFormatAllowsRangeTextInsideStrings(t *testing.T) {
	jsonContent := `[{"symbol":"BTCUSDT","final_action":"wait","reason_cn":"RSI30~50 只是说明文字，不是数值字段","confidence":null}]`
	if err := validateJSONFormat(jsonContent); err != nil {
		t.Fatalf("validateJSONFormat() should allow ~ inside quoted text, got %v", err)
	}
}

func TestValidateJSONFormatRejectsRangeSymbolOutsideStrings(t *testing.T) {
	jsonContent := `[{"symbol":"BTCUSDT","final_action":"wait","confidence":30~50}]`
	if err := validateJSONFormat(jsonContent); err == nil {
		t.Fatal("validateJSONFormat() should reject ~ outside quoted text")
	}
}

func TestValidateDecisionsDowngradesTinyBTCETHOpenToWait(t *testing.T) {
	decisions := []Decision{
		{
			Symbol:          "ETHUSDT",
			Action:          "open_long",
			Leverage:        20,
			PositionSizeUSD: 30.13,
			StopLoss:        2000,
			TakeProfit:      2200,
			Reasoning:       "Tiny test entry",
		},
	}

	if err := validateDecisions(decisions, 100, 20, 10, 10.0, 1.5); err != nil {
		t.Fatalf("validateDecisions() should downgrade tiny open to wait, got %v", err)
	}
	if decisions[0].Action != "wait" {
		t.Fatalf("tiny open should become wait, got %q", decisions[0].Action)
	}
	if decisions[0].PositionSizeUSD != 0 || decisions[0].Leverage != 0 {
		t.Fatalf("wait fallback should clear executable open fields, got size=%v leverage=%v", decisions[0].PositionSizeUSD, decisions[0].Leverage)
	}
}

func TestValidateDecisionsDowngradesInvalidActionToWait(t *testing.T) {
	decisions := []Decision{
		{
			Symbol:    "BTCUSDT",
			Action:    "",
			Reasoning: "Missing action from model",
		},
	}

	if err := validateDecisions(decisions, 100, 20, 10, 10.0, 1.5); err != nil {
		t.Fatalf("validateDecisions() should downgrade invalid action to wait, got %v", err)
	}
	if decisions[0].Action != "wait" {
		t.Fatalf("invalid action should become wait, got %q", decisions[0].Action)
	}
}

func TestValidateDecisionsNormalizesLegacyOpenNewWhenDirectionIsClear(t *testing.T) {
	decisions := []Decision{
		{
			Symbol:          "ETHUSDT",
			Action:          "OPEN_NEW",
			Leverage:        20,
			PositionSizeUSD: 120,
			StopLoss:        2000,
			TakeProfit:      2200,
			Reasoning:       "Legacy open action with long stop/take-profit relation",
		},
	}

	if err := validateDecisions(decisions, 100, 20, 10, 10.0, 1.5); err != nil {
		t.Fatalf("validateDecisions() should normalize executable OPEN_NEW, got %v", err)
	}
	if decisions[0].Action != "open_long" {
		t.Fatalf("OPEN_NEW with stop_loss < take_profit should become open_long, got %q", decisions[0].Action)
	}
}

// contains checks if string contains substring (helper function)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
