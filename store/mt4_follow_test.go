package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestMT4TicketMappingsPreserveMergedFastCloses(t *testing.T) {
	db, err := InitGorm(filepath.Join(t.TempDir(), "mt4-follow.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	follow := NewComkunFollowStore(db)
	if err := follow.initTables(); err != nil {
		t.Fatal(err)
	}

	var broadcasts []*ComkunMasterBroadcast
	for i, ticket := range []int64{101, 102, 103} {
		event := MT4SignalEvent{
			Action: "CLOSE", Ticket: ticket, Symbol: "XAUUSD", ExchangeSymbol: "XAUUSDT",
			Side: "long", Volume: 0.01, MasterQuantity: 1, Price: 4050 + float64(i),
		}
		state, _ := json.Marshal(map[string]interface{}{
			"v": 2, "positions": []interface{}{}, "pending_orders": []interface{}{}, "mt4_event": event,
		})
		row, insertErr := follow.InsertBroadcast("mt4-ea-gold-master", 10000, "mt4", "[]", string(state))
		if insertErr != nil {
			t.Fatal(insertErr)
		}
		broadcasts = append(broadcasts, row)
	}

	if err := follow.EnsureMT4TicketMappings("trader-1", "mt4-ea-gold-master", broadcasts[2].ID, 5000); err != nil {
		t.Fatal(err)
	}
	rows, err := follow.ListMT4TicketMappings("trader-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("mapping count=%d, want 3", len(rows))
	}
	byBroadcast := make(map[uint64]MT4FollowTicketMapping, len(rows))
	for _, row := range rows {
		byBroadcast[row.BroadcastID] = row
		if row.FollowerQuantity != 0.5 {
			t.Fatalf("broadcast %d follower qty=%f, want 0.5", row.BroadcastID, row.FollowerQuantity)
		}
	}
	for _, row := range broadcasts[:2] {
		got := byBroadcast[row.ID]
		if got.Status != MT4MappingMerged || got.MergedIntoBroadcastID != broadcasts[2].ID {
			t.Fatalf("broadcast %d = status %q merged_into %d", row.ID, got.Status, got.MergedIntoBroadcastID)
		}
	}
	if got := byBroadcast[broadcasts[2].ID]; got.Status != MT4MappingPending {
		t.Fatalf("latest status=%q, want pending", got.Status)
	}

	if err := follow.MarkMT4TicketMappingStatus("trader-1", broadcasts[2].ID, MT4MappingApplied, ""); err != nil {
		t.Fatal(err)
	}
	if err := follow.AppendMT4ExchangeOrderID("trader-1", broadcasts[2].ID, "9001"); err != nil {
		t.Fatal(err)
	}
	rows, err = follow.ListMT4TicketMappings("trader-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	got := rows[0]
	if got.BroadcastID != broadcasts[2].ID || got.Status != MT4MappingApplied || got.ExchangeOrderIDs != `["9001"]` {
		t.Fatalf("applied mapping=%+v", got)
	}
}

func TestMT4TicketMappingUsesActualMarginSizing(t *testing.T) {
	db, err := InitGorm(filepath.Join(t.TempDir(), "mt4-follow-margin.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	follow := NewComkunFollowStore(db)
	if err := follow.initTables(); err != nil {
		t.Fatal(err)
	}

	event := MT4SignalEvent{
		Action: "OPEN", Ticket: 201, Symbol: "XAUUSDc", ExchangeSymbol: "XAUUSDT",
		Side: "short", Volume: 0.01, MasterQuantity: 1, EntryPrice: 4000, Price: 4000,
	}
	state, _ := json.Marshal(map[string]interface{}{
		"v": 2,
		"positions": []map[string]interface{}{{
			"symbol": "XAUUSDT", "side": "short", "quantity": 1.0,
			"entry_price": 4000.0, "mark_price": 4000.0, "leverage": 20,
		}},
		"mirror_margin": map[string]interface{}{
			"master_margin_used": 1.0, "master_margin_leverage": 20,
		},
		"mt4_event": event,
	})
	br, err := follow.InsertBroadcast("mt4-ea-gold-master", 100, "mt4", "[]", string(state))
	if err != nil {
		t.Fatal(err)
	}
	if err := follow.EnsureMT4TicketMappings("trader-1", "mt4-ea-gold-master", br.ID, 5000); err != nil {
		t.Fatal(err)
	}
	rows, err := follow.ListMT4TicketMappings("trader-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("mapping count=%d, want 1", len(rows))
	}
	if got, want := rows[0].FollowerQuantity, 0.25; got != want {
		t.Fatalf("follower qty=%f, want %f", got, want)
	}
}
