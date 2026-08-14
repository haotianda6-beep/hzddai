package observation

import (
	"testing"
)

func TestLoadVerifiedArtifact(t *testing.T) {
	doc, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if doc.Status != StatusHistoricalSimulation {
		t.Fatalf("status = %q", doc.Status)
	}
	if len(doc.Slots) != 6 {
		t.Fatalf("slots = %d", len(doc.Slots))
	}

	wantTrades := []int{645, 568, 561, 471, 415, 269}
	seenStrategies := map[string]bool{}
	for i, slot := range doc.Slots {
		if slot.Slot != i+1 {
			t.Fatalf("slot[%d] = %d", i, slot.Slot)
		}
		if slot.SourceStrategyID != nil {
			t.Fatalf("slot %d unexpectedly has source strategy", slot.Slot)
		}
		if len(slot.History.Trades) != wantTrades[i] {
			t.Fatalf("slot %d trades = %d", slot.Slot, len(slot.History.Trades))
		}
		id := StrategyID(slot.Slot)
		if seenStrategies[id] {
			t.Fatalf("duplicate strategy id %q", id)
		}
		seenStrategies[id] = true
	}
}

func TestValidateRejectsHistoryAccountAsLiveMaster(t *testing.T) {
	doc, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for _, slot := range doc.Slots {
		if slot.RealtimeFollowAvailable {
			t.Fatalf("slot %d must remain history-only", slot.Slot)
		}
		if slot.SourceAccountID == "" {
			t.Fatalf("slot %d missing provenance account", slot.Slot)
		}
	}
}

func TestValidateArtifactHash(t *testing.T) {
	if got := ArtifactSHA256(); got != ExpectedArtifactSHA256 {
		t.Fatalf("artifact sha256 = %s", got)
	}
}
