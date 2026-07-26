package payment

import "testing"

func TestPlatformAIMarkupMultiplierDefault(t *testing.T) {
	t.Setenv("PLATFORM_AI_MARKUP_MULTIPLIER", "")

	if got := platformAIMarkupMultiplier(); got != 3.0 {
		t.Fatalf("platformAIMarkupMultiplier() = %v, want 3.0", got)
	}
}

func TestPlatformAIMarkupMultiplierEnvOverride(t *testing.T) {
	t.Setenv("PLATFORM_AI_MARKUP_MULTIPLIER", "2.25")

	if got := platformAIMarkupMultiplier(); got != 2.25 {
		t.Fatalf("platformAIMarkupMultiplier() = %v, want 2.25", got)
	}
}

func TestPlatformAIMinChargeUSDTDefault(t *testing.T) {
	t.Setenv("PLATFORM_AI_MIN_CHARGE_USDT", "")

	if got := platformAIMinChargeUSDT(); got != 0.01 {
		t.Fatalf("platformAIMinChargeUSDT() = %v, want 0.01", got)
	}
}
