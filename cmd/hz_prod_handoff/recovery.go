package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"nofx/store"
)

func recover(statePath, dbPath, billingBroadcastRaw, snapshotBroadcastRaw string) error {
	if filepath.Clean(dbPath) != productionDB {
		return fmt.Errorf("refusing unexpected database path")
	}
	billingBroadcast, err := strconv.ParseUint(billingBroadcastRaw, 10, 64)
	if err != nil {
		return err
	}
	snapshotBroadcast, err := strconv.ParseUint(snapshotBroadcastRaw, 10, 64)
	if err != nil {
		return err
	}
	var state handoffState
	if err := readJSON(statePath, &state); err != nil {
		return err
	}
	db, err := store.InitGorm(dbPath)
	if err != nil {
		return err
	}
	defer closeDB(db)
	st, err := store.NewFromGorm(db)
	if err != nil {
		return err
	}
	traderIDs := make([]string, 0, len(state.Followers))
	for _, follower := range state.Followers {
		traderIDs = append(traderIDs, follower.TraderID)
	}
	var billingIntents []store.MirrorExecutionIntent
	if err := db.Where("broadcast_id = ? AND action = ? AND trader_id IN ?", billingBroadcast, "billing", traderIDs).Find(&billingIntents).Error; err != nil {
		return err
	}
	if len(billingIntents) != len(state.Followers) {
		return fmt.Errorf("billing intent count mismatch")
	}
	refunded := 0
	for _, intent := range billingIntents {
		if intent.BillingStatus == store.MirrorBillingPending || intent.BillingStatus == store.MirrorBillingCharged {
			if err := st.MirrorExecutionIntent().RefundBilling(intent.IntentKey, "poller_close_only_recovery"); err != nil {
				return err
			}
			refunded++
		}
	}
	var snapshot store.ComkunMasterBroadcast
	if err := db.Where("id = ? AND source_strategy_id = ?", snapshotBroadcast, state.SourceID).First(&snapshot).Error; err != nil {
		return err
	}
	var sourceEvent store.IntegrationMasterEvent
	if err := db.Where("broadcast_id = ?", snapshotBroadcast).First(&sourceEvent).Error; err != nil {
		return err
	}
	var masterState map[string]any
	if err := json.Unmarshal([]byte(snapshot.MasterStateJSON), &masterState); err != nil {
		return err
	}
	recoveryID := fmt.Sprintf("recovery-%x", sha256.Sum256([]byte(sourceEvent.MasterAccountID+"|10|poller-race")))
	now := time.Now().UTC()
	masterState["source_event_id"] = recoveryID
	masterState["source_sequence"] = int64(10)
	masterState["event_type"] = "RECONCILE"
	masterState["occurred_at"] = now.Format(time.RFC3339Nano)
	masterState["polling_reconcile"] = true
	stateRaw, err := json.Marshal(masterState)
	if err != nil {
		return err
	}
	payloadHash := sha256.Sum256(stateRaw)
	result, err := st.IntegrationMasterEvent().Accept(store.IntegrationMasterEventInput{
		Provider: "hz-recovery", MasterAccountID: sourceEvent.MasterAccountID, EventID: recoveryID,
		Sequence: 10, EventType: "RECONCILE", OccurredAt: now, PayloadVersion: 1,
		PayloadSHA256: hex.EncodeToString(payloadHash[:]), PayloadJSON: string(stateRaw),
		AuthNonce: "recovery:" + recoveryID, MasterEquity: snapshot.MasterAccountEquity, MasterStateJSON: string(stateRaw),
	}, true)
	if err != nil {
		return err
	}
	if !result.Accepted && !result.Duplicate {
		return fmt.Errorf("recovery event was not accepted")
	}
	fmt.Printf("recovery_event_sequence=10 refunded=%d broadcast_id=%d\n", refunded, result.BroadcastID)
	return nil
}
