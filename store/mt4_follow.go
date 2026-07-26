package store

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const MT4GoldMasterStrategyID = "mt4-ea-gold-master"

const (
	MT4MappingPending    = "pending"
	MT4MappingApplied    = "applied"
	MT4MappingMerged     = "merged"
	MT4MappingSuperseded = "superseded"
	MT4MappingFailed     = "failed"
)

// MT4SignalEvent is embedded in every MT4 master snapshot so ticket history
// remains durable even when followers intentionally coalesce rapid snapshots.
type MT4SignalEvent struct {
	Action          string  `json:"action"`
	Ticket          int64   `json:"ticket"`
	Seq             uint64  `json:"seq,omitempty"`
	Symbol          string  `json:"symbol"`
	ExchangeSymbol  string  `json:"exchange_symbol"`
	Side            string  `json:"side"`
	Volume          float64 `json:"volume"`
	PositionVolume  float64 `json:"position_volume,omitempty"`
	MasterQuantity  float64 `json:"master_quantity"`
	EntryPrice      float64 `json:"entry_price,omitempty"`
	Price           float64 `json:"price,omitempty"`
	ClientTimestamp int64   `json:"client_timestamp,omitempty"`
}

type MT4FollowTicketMapping struct {
	ID                    uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TraderID              string    `gorm:"column:trader_id;not null;uniqueIndex:idx_mt4_mapping_once;index" json:"trader_id"`
	SourceStrategyID      string    `gorm:"column:source_strategy_id;not null;index" json:"source_strategy_id"`
	BroadcastID           uint64    `gorm:"column:broadcast_id;not null;uniqueIndex:idx_mt4_mapping_once;index" json:"broadcast_id"`
	MT4Ticket             int64     `gorm:"column:mt4_ticket;not null;index" json:"mt4_ticket"`
	MT4Seq                uint64    `gorm:"column:mt4_seq;not null;default:0" json:"mt4_seq"`
	Action                string    `gorm:"column:action;not null;index" json:"action"`
	Symbol                string    `gorm:"column:symbol;not null" json:"symbol"`
	Side                  string    `gorm:"column:side;not null" json:"side"`
	MT4Volume             float64   `gorm:"column:mt4_volume;not null;default:0" json:"mt4_volume"`
	MasterQuantity        float64   `gorm:"column:master_quantity;not null;default:0" json:"master_quantity"`
	FollowerQuantity      float64   `gorm:"column:follower_quantity;not null;default:0" json:"follower_quantity"`
	EntryPrice            float64   `gorm:"column:entry_price;not null;default:0" json:"entry_price"`
	EventPrice            float64   `gorm:"column:event_price;not null;default:0" json:"event_price"`
	Status                string    `gorm:"column:status;not null;default:'pending';index" json:"status"`
	MergedIntoBroadcastID uint64    `gorm:"column:merged_into_broadcast_id;not null;default:0;index" json:"merged_into_broadcast_id"`
	ExchangeOrderIDs      string    `gorm:"column:exchange_order_ids;type:text;not null;default:'[]'" json:"exchange_order_ids"`
	Error                 string    `gorm:"column:error;type:text;not null;default:''" json:"error"`
	SignalAt              int64     `gorm:"column:signal_at;not null;default:0" json:"signal_at"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

func (MT4FollowTicketMapping) TableName() string { return "mt4_follow_ticket_mappings" }

type MT4ExecutionFill struct {
	ExchangeOrderID string
	Price           float64
	Quantity        float64
	RealizedPnL     float64
	Commission      float64
	CreatedAt       int64
}

type mt4MappingWire struct {
	Positions []struct {
		Symbol     string  `json:"symbol"`
		Side       string  `json:"side"`
		Quantity   float64 `json:"quantity"`
		EntryPrice float64 `json:"entry_price"`
		MarkPrice  float64 `json:"mark_price"`
		Leverage   int     `json:"leverage"`
	} `json:"positions"`
	MirrorMargin *struct {
		MasterMarginLeverage int     `json:"master_margin_leverage"`
		MasterMarginUsed     float64 `json:"master_margin_used"`
	} `json:"mirror_margin"`
}

func IsMT4GoldMasterStrategyID(strategyID string) bool {
	return strings.EqualFold(strings.TrimSpace(strategyID), MT4GoldMasterStrategyID)
}

var mt4AnalysisEventRE = regexp.MustCompile(`MT4 EA：(OPEN|CLOSE)\s+([^\s]+)\s+ticket=([0-9]+)`)

func DecodeMT4SignalEvent(masterStateJSON, analysis string) (MT4SignalEvent, bool) {
	var wire struct {
		Event *MT4SignalEvent `json:"mt4_event"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(masterStateJSON)), &wire) == nil && wire.Event != nil && wire.Event.Ticket != 0 {
		wire.Event.Action = strings.ToUpper(strings.TrimSpace(wire.Event.Action))
		return *wire.Event, true
	}
	m := mt4AnalysisEventRE.FindStringSubmatch(analysis)
	if len(m) != 4 {
		return MT4SignalEvent{}, false
	}
	ticket, _ := strconv.ParseInt(m[3], 10, 64)
	return MT4SignalEvent{
		Action: strings.ToUpper(m[1]), Ticket: ticket, Symbol: m[2],
		ExchangeSymbol: strings.ToUpper(strings.TrimSpace(m[2])),
	}, ticket != 0
}

// EnsureMT4TicketMappings records every source event through the latest target.
// Events without their own consumption row were intentionally coalesced into
// the latest snapshot and remain visible as merged rather than disappearing.
func (s *ComkunFollowStore) EnsureMT4TicketMappings(traderID, sourceStrategyID string, throughBroadcastID uint64, followerInitialBalance float64) error {
	traderID = strings.TrimSpace(traderID)
	sourceStrategyID = strings.TrimSpace(sourceStrategyID)
	if traderID == "" || !IsMT4GoldMasterStrategyID(sourceStrategyID) || throughBroadcastID == 0 {
		return nil
	}
	var broadcasts []ComkunMasterBroadcast
	if err := s.db.Where("source_strategy_id = ? AND id <= ?", sourceStrategyID, throughBroadcastID).
		Order("id ASC").Limit(2000).Find(&broadcasts).Error; err != nil {
		return err
	}
	openQtyByTicket := make(map[int64]float64)
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, br := range broadcasts {
			event, ok := DecodeMT4SignalEvent(br.MasterStateJSON, br.AnalysisText)
			if !ok {
				continue
			}
			status := MT4MappingPending
			mergedInto := uint64(0)
			var consumption ComkunFollowBroadcastConsumption
			err := tx.Where("trader_id = ? AND broadcast_id = ?", traderID, br.ID).First(&consumption).Error
			switch {
			case err == nil && consumption.Status == "success":
				status = MT4MappingApplied
			case err == nil && consumption.Status == "failed":
				status = MT4MappingFailed
			case br.ID < throughBroadcastID:
				status = MT4MappingMerged
				mergedInto = throughBroadcastID
			}
			signalAt := event.ClientTimestamp
			if signalAt > 0 && signalAt < 10_000_000_000 {
				signalAt *= 1000
			}
			if signalAt == 0 {
				signalAt = br.CreatedAt.UnixMilli()
			}
			followerQuantity := mt4FollowerQuantityForMapping(br, event, followerInitialBalance, openQtyByTicket)
			if strings.EqualFold(event.Action, "OPEN") && followerQuantity > 0 {
				openQtyByTicket[event.Ticket] = followerQuantity
			}
			row := MT4FollowTicketMapping{
				TraderID: traderID, SourceStrategyID: sourceStrategyID, BroadcastID: br.ID,
				MT4Ticket: event.Ticket, MT4Seq: event.Seq, Action: event.Action,
				Symbol: event.ExchangeSymbol, Side: event.Side, MT4Volume: event.Volume,
				MasterQuantity: event.MasterQuantity, FollowerQuantity: followerQuantity,
				EntryPrice: event.EntryPrice, EventPrice: event.Price, Status: status,
				MergedIntoBroadcastID: mergedInto, ExchangeOrderIDs: "[]", SignalAt: signalAt,
			}
			if row.Symbol == "" {
				row.Symbol = event.Symbol
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "trader_id"}, {Name: "broadcast_id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"mt4_ticket", "mt4_seq", "action", "symbol", "side", "mt4_volume",
					"master_quantity", "follower_quantity", "entry_price", "event_price", "signal_at",
				}),
			}).Create(&row).Error; err != nil {
				return err
			}
			if br.ID < throughBroadcastID {
				if err := tx.Model(&MT4FollowTicketMapping{}).
					Where("trader_id = ? AND broadcast_id = ? AND status IN ?", traderID, br.ID,
						[]string{MT4MappingPending, MT4MappingSuperseded, MT4MappingFailed}).
					Updates(map[string]interface{}{"status": MT4MappingMerged, "merged_into_broadcast_id": throughBroadcastID}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func mt4FollowerQuantityForMapping(br ComkunMasterBroadcast, event MT4SignalEvent, followerInitialBalance float64, openQtyByTicket map[int64]float64) float64 {
	fallback := 0.0
	if br.MasterAccountEquity > 0 && followerInitialBalance > 0 {
		fallback = event.MasterQuantity * followerInitialBalance / br.MasterAccountEquity
	}
	if !strings.EqualFold(event.Action, "OPEN") {
		if qty := openQtyByTicket[event.Ticket]; qty > 0 {
			return qty
		}
		return fallback
	}
	var wire mt4MappingWire
	if json.Unmarshal([]byte(strings.TrimSpace(br.MasterStateJSON)), &wire) != nil ||
		wire.MirrorMargin == nil || wire.MirrorMargin.MasterMarginUsed <= 0 ||
		br.MasterAccountEquity <= 0 || followerInitialBalance <= 0 {
		return fallback
	}
	followLev := wire.MirrorMargin.MasterMarginLeverage
	if followLev < 1 {
		followLev = 20
	}
	totalMargin := 0.0
	matchMargin := 0.0
	eventSymbol := strings.ToUpper(strings.TrimSpace(event.ExchangeSymbol))
	if eventSymbol == "" {
		eventSymbol = strings.ToUpper(strings.TrimSpace(event.Symbol))
	}
	eventSide := strings.ToLower(strings.TrimSpace(event.Side))
	eventPrice := event.EntryPrice
	if eventPrice <= 0 {
		eventPrice = event.Price
	}
	for _, p := range wire.Positions {
		price := p.MarkPrice
		if price <= 0 {
			price = p.EntryPrice
		}
		lev := p.Leverage
		if lev < 1 {
			lev = followLev
		}
		if price <= 0 || p.Quantity <= 0 || lev < 1 {
			continue
		}
		margin := math.Abs(p.Quantity) * price / float64(lev)
		totalMargin += margin
		if strings.EqualFold(strings.TrimSpace(p.Symbol), eventSymbol) &&
			strings.EqualFold(strings.TrimSpace(p.Side), eventSide) &&
			math.Abs(math.Abs(p.Quantity)-math.Abs(event.MasterQuantity)) < 1e-9 &&
			(eventPrice <= 0 || math.Abs(price-eventPrice) <= math.Max(0.01, eventPrice*1e-6)) {
			matchMargin += margin
		}
	}
	if totalMargin <= 0 {
		return fallback
	}
	if matchMargin <= 0 && event.MasterQuantity > 0 && eventPrice > 0 {
		matchMargin = math.Abs(event.MasterQuantity) * eventPrice / float64(followLev)
	}
	if matchMargin <= 0 {
		return fallback
	}
	marginPct := wire.MirrorMargin.MasterMarginUsed / br.MasterAccountEquity
	if marginPct > 1 {
		marginPct = 1
	}
	if marginPct <= 0 {
		return fallback
	}
	return (matchMargin / totalMargin) * marginPct * followerInitialBalance * float64(followLev) / eventPrice
}

func (s *ComkunFollowStore) MarkMT4TicketMappingStatus(traderID string, broadcastID uint64, status, message string) error {
	if len(message) > 1000 {
		message = message[:1000]
	}
	return s.db.Model(&MT4FollowTicketMapping{}).
		Where("trader_id = ? AND broadcast_id = ?", strings.TrimSpace(traderID), broadcastID).
		Updates(map[string]interface{}{"status": status, "error": message, "updated_at": time.Now().UTC()}).Error
}

func (s *ComkunFollowStore) AppendMT4ExchangeOrderID(traderID string, broadcastID uint64, orderID string) error {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return nil
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var row MT4FollowTicketMapping
		if err := tx.Where("trader_id = ? AND broadcast_id = ?", strings.TrimSpace(traderID), broadcastID).First(&row).Error; err != nil {
			return err
		}
		var ids []string
		_ = json.Unmarshal([]byte(row.ExchangeOrderIDs), &ids)
		for _, id := range ids {
			if id == orderID {
				return nil
			}
		}
		ids = append(ids, orderID)
		raw, _ := json.Marshal(ids)
		return tx.Model(&row).Update("exchange_order_ids", string(raw)).Error
	})
}

func (s *ComkunFollowStore) ListMT4TicketMappings(traderID string, limit int) ([]MT4FollowTicketMapping, error) {
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	var rows []MT4FollowTicketMapping
	err := s.db.Where("trader_id = ?", strings.TrimSpace(traderID)).Order("broadcast_id DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

func (s *ComkunFollowStore) ListMT4ExecutionFills(traderID string, orderIDs []string) ([]MT4ExecutionFill, error) {
	if len(orderIDs) == 0 {
		return nil, nil
	}
	var rows []MT4ExecutionFill
	err := s.db.Table("trader_fills").
		Select("exchange_order_id, price, quantity, realized_pnl, commission, created_at").
		Where("trader_id = ? AND exchange_order_id IN ?", strings.TrimSpace(traderID), orderIDs).
		Order("created_at ASC").Scan(&rows).Error
	return rows, err
}

func DecodeMT4OrderIDs(raw string) []string {
	var ids []string
	_ = json.Unmarshal([]byte(strings.TrimSpace(raw)), &ids)
	return ids
}

func (m MT4FollowTicketMapping) EffectiveExecutionBroadcastID() uint64 {
	if m.MergedIntoBroadcastID > 0 {
		return m.MergedIntoBroadcastID
	}
	return m.BroadcastID
}

func (m MT4FollowTicketMapping) Validate() error {
	if m.TraderID == "" || m.BroadcastID == 0 || m.MT4Ticket == 0 {
		return fmt.Errorf("invalid MT4 mapping")
	}
	return nil
}
