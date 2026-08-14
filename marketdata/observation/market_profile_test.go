package observation

import "testing"

func TestMarketProfilesAreCompleteAndUnique(t *testing.T) {
	names := map[string]bool{}
	creators := map[string]bool{}
	wantPrices := []float64{100, 200, 300, 500, 800, 1000}
	for slot := 1; slot <= 6; slot++ {
		profile, ok := MarketProfileBySlot(slot)
		if !ok {
			t.Fatalf("slot %d missing", slot)
		}
		if profile.Slot != slot || profile.Name == "" || profile.Description == "" ||
			profile.CreatorDisplayName == "" || profile.CreatorAvatarURL == "" {
			t.Fatalf("slot %d incomplete: %+v", slot, profile)
		}
		if profile.MonthlyPriceUSDT != wantPrices[slot-1] {
			t.Fatalf("slot %d price=%.2f want=%.2f", slot, profile.MonthlyPriceUSDT, wantPrices[slot-1])
		}
		if names[profile.Name] || creators[profile.CreatorDisplayName] {
			t.Fatalf("slot %d identity is not unique", slot)
		}
		names[profile.Name], creators[profile.CreatorDisplayName] = true, true
	}
}
