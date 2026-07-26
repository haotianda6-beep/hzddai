package api

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"nofx/store"
	tradertypes "nofx/trader/types"
)

func buildMT4FollowHistoryRecords(follow *store.ComkunFollowStore, traderID string, limit int) ([]tradertypes.ClosedPnLRecord, error) {
	queryLimit := limit * 8
	if queryLimit < 500 {
		queryLimit = 500
	}
	rows, err := follow.ListMT4TicketMappings(traderID, queryLimit)
	if err != nil {
		return nil, err
	}
	executionRows := make(map[uint64]store.MT4FollowTicketMapping, len(rows))
	for _, row := range rows {
		executionRows[row.BroadcastID] = row
	}
	fillsByExecution := make(map[uint64][]store.MT4ExecutionFill)
	for executionID, row := range executionRows {
		action := strings.ToUpper(row.Action)
		if action != "CLOSE" && action != "REDUCE" {
			continue
		}
		orderIDs := store.DecodeMT4OrderIDs(row.ExchangeOrderIDs)
		if len(orderIDs) == 0 {
			continue
		}
		fills, fillErr := follow.ListMT4ExecutionFills(traderID, orderIDs)
		if fillErr != nil {
			return nil, fillErr
		}
		fillsByExecution[executionID] = fills
	}
	return buildMT4TicketHistoryRecords(rows, fillsByExecution), nil
}

func buildMT4TicketHistoryRecords(rows []store.MT4FollowTicketMapping, fillsByExecution map[uint64][]store.MT4ExecutionFill) []tradertypes.ClosedPnLRecord {
	sort.Slice(rows, func(i, j int) bool { return rows[i].BroadcastID < rows[j].BroadcastID })
	type entry struct {
		price float64
		at    time.Time
	}
	entries := make(map[int64]entry)
	closingQty := make(map[uint64]float64)
	for _, row := range rows {
		action := strings.ToUpper(row.Action)
		if action == "OPEN" && entries[row.MT4Ticket].price == 0 {
			entries[row.MT4Ticket] = entry{price: row.EntryPrice, at: mt4SignalTime(row)}
		}
		if (action == "CLOSE" || action == "REDUCE") && mt4MappingExecuted(row.Status) {
			closingQty[row.EffectiveExecutionBroadcastID()] += row.FollowerQuantity
		}
	}

	records := make([]tradertypes.ClosedPnLRecord, 0)
	for _, row := range rows {
		action := strings.ToUpper(row.Action)
		if (action != "CLOSE" && action != "REDUCE") || !mt4MappingExecuted(row.Status) || row.FollowerQuantity <= 0 {
			continue
		}
		open := entries[row.MT4Ticket]
		if open.price <= 0 {
			open.price = row.EntryPrice
			open.at = mt4SignalTime(row)
		}
		exitPrice := row.EventPrice
		weightedExit := 0.0
		pnl := 0.0
		fee := 0.0
		executionID := row.EffectiveExecutionBroadcastID()
		fills := fillsByExecution[executionID]
		fillQty := 0.0
		for _, fill := range fills {
			if fill.Quantity > 0 && fill.Price > 0 {
				weightedExit += fill.Price * fill.Quantity
				fillQty += fill.Quantity
			}
			pnl += fill.RealizedPnL
			fee += fill.Commission
		}
		if fillQty > 0 {
			exitPrice = weightedExit / fillQty
		}
		groupQty := closingQty[executionID]
		if len(fills) > 0 && groupQty > 0 {
			share := row.FollowerQuantity / groupQty
			pnl *= share
			fee *= share
		} else if open.price > 0 && exitPrice > 0 {
			pnl = (exitPrice - open.price) * row.FollowerQuantity
			if strings.EqualFold(row.Side, "short") {
				pnl = -pnl
			}
		}
		orderIDs := []string{}
		for _, candidate := range rows {
			if candidate.BroadcastID == executionID {
				orderIDs = store.DecodeMT4OrderIDs(candidate.ExchangeOrderIDs)
				break
			}
		}
		closeType := "mt4_ticket"
		if row.Status == store.MT4MappingMerged || row.Status == store.MT4MappingSuperseded {
			closeType = "mt4_ticket_merged"
		}
		records = append(records, tradertypes.ClosedPnLRecord{
			Symbol: row.Symbol, Side: strings.ToUpper(row.Side), EntryPrice: open.price,
			ExitPrice: exitPrice, Quantity: row.FollowerQuantity, RealizedPnL: pnl, Fee: fee,
			Leverage: 20, EntryTime: open.at, ExitTime: mt4SignalTime(row),
			OrderID: strings.Join(orderIDs, ","), CloseType: closeType,
			ExchangeID: fmt.Sprintf("mt4-%d-%d", row.MT4Ticket, row.BroadcastID),
		})
	}
	return records
}

func mt4MappingExecuted(status string) bool {
	return status == store.MT4MappingApplied || status == store.MT4MappingMerged || status == store.MT4MappingSuperseded
}

func mt4SignalTime(row store.MT4FollowTicketMapping) time.Time {
	if row.SignalAt > 0 {
		return time.UnixMilli(row.SignalAt).UTC()
	}
	return row.CreatedAt.UTC()
}
