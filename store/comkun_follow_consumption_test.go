package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestConsumptionAttemptDueHonorsStateAndCooldown(t *testing.T) {
	db, err := InitGorm(filepath.Join(t.TempDir(), "consumption-due.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&ComkunFollowBroadcastConsumption{}); err != nil {
		t.Fatal(err)
	}
	store := NewComkunFollowStore(db)
	now := time.Now().UTC()
	rows := []ComkunFollowBroadcastConsumption{
		{TraderID: "success", BroadcastID: 1, Status: "success", CreatedAt: now, UpdatedAt: now},
		{TraderID: "failed-recent", BroadcastID: 2, Status: "failed", CreatedAt: now, UpdatedAt: now},
		{TraderID: "failed-old", BroadcastID: 3, Status: "failed", CreatedAt: now.Add(-3 * time.Minute), UpdatedAt: now.Add(-3 * time.Minute)},
		{TraderID: "processing-recent", BroadcastID: 4, Status: "processing", CreatedAt: now, UpdatedAt: now},
		{TraderID: "processing-old", BroadcastID: 5, Status: "processing", CreatedAt: now.Add(-4 * time.Minute), UpdatedAt: now.Add(-4 * time.Minute)},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		traderID    string
		broadcastID uint64
		want        bool
	}{
		{name: "new", traderID: "new", broadcastID: 6, want: true},
		{name: "success", traderID: "success", broadcastID: 1, want: false},
		{name: "failed recent", traderID: "failed-recent", broadcastID: 2, want: false},
		{name: "failed old", traderID: "failed-old", broadcastID: 3, want: true},
		{name: "processing recent", traderID: "processing-recent", broadcastID: 4, want: false},
		{name: "processing old", traderID: "processing-old", broadcastID: 5, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.ConsumptionAttemptDue(tt.traderID, tt.broadcastID)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("due=%t, want %t", got, tt.want)
			}
		})
	}
}
