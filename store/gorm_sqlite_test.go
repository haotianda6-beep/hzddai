package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteGormBusyTimeoutAppliesToEveryConnection(t *testing.T) {
	db, err := InitGorm(filepath.Join(t.TempDir(), "busy-timeout.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conns := make([]interface{ Close() error }, 0, 3)
	for i := 0; i < 3; i++ {
		conn, err := sqlDB.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, conn)
		var timeout int
		if err := conn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&timeout); err != nil {
			t.Fatal(err)
		}
		if timeout != 10000 {
			t.Fatalf("connection %d busy_timeout=%d, want 10000", i+1, timeout)
		}
	}
	for _, conn := range conns {
		_ = conn.Close()
	}
}

func TestGetLatestRecordsHidesInternalSQLiteLockErrors(t *testing.T) {
	db, err := InitGorm(filepath.Join(t.TempDir(), "decisions.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	decisions := NewDecisionStore(db)
	if err := decisions.initTables(); err != nil {
		t.Fatal(err)
	}
	for _, record := range []*DecisionRecord{
		{TraderID: "trader-1", ErrorMessage: "AI 策略状态更新失败: database is locked"},
		{TraderID: "trader-1", Success: true, ExecutionLog: []string{"AI 策略状态已更新。"}},
	} {
		if err := decisions.LogDecision(record); err != nil {
			t.Fatal(err)
		}
	}

	records, err := decisions.GetLatestRecords("trader-1", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || !records[0].Success {
		t.Fatalf("records=%+v, want only the customer-visible successful record", records)
	}
	if msg, _, ok := decisions.LatestNonEmptyErrorMessage("trader-1"); ok {
		t.Fatalf("internal SQLite lock error leaked into status hint: %q", msg)
	}

	if err := db.AutoMigrate(&ComkunFollowBroadcastConsumption{}); err != nil {
		t.Fatal(err)
	}
	lockedConsumption := &ComkunFollowBroadcastConsumption{
		TraderID: "trader-1", BroadcastID: 1, Status: "failed", Error: "database is locked",
	}
	if err := db.Create(lockedConsumption).Error; err != nil {
		t.Fatal(err)
	}
	if msg, _, ok := NewComkunFollowStore(db).LatestFailedConsumptionError("trader-1"); ok {
		t.Fatalf("internal SQLite lock error leaked from consumption status: %q", msg)
	}
}
