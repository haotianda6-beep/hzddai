package api

import (
	"testing"
	"time"

	"nofx/store"
)

func TestBuildMT4TicketHistorySplitsAggregateFillByTicket(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	rows := []store.MT4FollowTicketMapping{
		{BroadcastID: 1, MT4Ticket: 101, Action: "OPEN", Side: "long", Symbol: "XAUUSDT", FollowerQuantity: 0.5, EntryPrice: 2400, SignalAt: now.UnixMilli(), Status: store.MT4MappingMerged},
		{BroadcastID: 2, MT4Ticket: 102, Action: "OPEN", Side: "long", Symbol: "XAUUSDT", FollowerQuantity: 1, EntryPrice: 2405, SignalAt: now.Add(time.Second).UnixMilli(), Status: store.MT4MappingMerged},
		{BroadcastID: 3, MT4Ticket: 101, Action: "CLOSE", Side: "long", Symbol: "XAUUSDT", FollowerQuantity: 0.5, EntryPrice: 2400, EventPrice: 2410, SignalAt: now.Add(2 * time.Second).UnixMilli(), Status: store.MT4MappingMerged, MergedIntoBroadcastID: 4},
		{BroadcastID: 4, MT4Ticket: 102, Action: "CLOSE", Side: "long", Symbol: "XAUUSDT", FollowerQuantity: 1, EntryPrice: 2405, EventPrice: 2410, SignalAt: now.Add(3 * time.Second).UnixMilli(), Status: store.MT4MappingApplied, ExchangeOrderIDs: `["9001"]`},
	}
	fills := map[uint64][]store.MT4ExecutionFill{4: {{ExchangeOrderID: "9001", Price: 2410, Quantity: 1.5, RealizedPnL: 15, Commission: 1.5}}}
	records := buildMT4TicketHistoryRecords(rows, fills)
	if len(records) != 2 {
		t.Fatalf("records=%d, want 2", len(records))
	}
	if records[0].RealizedPnL != 5 || records[0].Fee != 0.5 || records[0].OrderID != "9001" {
		t.Fatalf("ticket 101 record=%+v", records[0])
	}
	if records[1].RealizedPnL != 10 || records[1].Fee != 1 || records[1].ExitPrice != 2410 {
		t.Fatalf("ticket 102 record=%+v", records[1])
	}
}
