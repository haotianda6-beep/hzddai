package main

import (
	"flag"
	"fmt"
	"os"

	"nofx/store"
)

const correctionReason = "sequence3_confirmed_open"

func main() {
	dbPath := flag.String("db", "", "SQLite database path")
	broadcastID := flag.Uint64("broadcast-id", 0, "exact HZ broadcast ID")
	expected := flag.Int("expected", 0, "required correction target count")
	apply := flag.Bool("apply", false, "apply the idempotent correction")
	flag.Parse()
	if *dbPath == "" || *broadcastID == 0 || *expected <= 0 {
		fmt.Fprintln(os.Stderr, "db, broadcast-id and positive expected are required")
		os.Exit(2)
	}

	db, err := store.InitGorm(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open database: category=%s\n", store.StoreErrorCategory(err))
		os.Exit(1)
	}
	st, err := store.NewFromGorm(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "initialize store: category=%s\n", store.StoreErrorCategory(err))
		os.Exit(1)
	}

	var keys []string
	err = db.Table("mirror_execution_intents AS billing").
		Where("billing.broadcast_id = ? AND billing.action = ?", *broadcastID, "billing").
		Where("(billing.billing_status = ? OR billing.last_error = ?)",
			store.MirrorBillingRefunded, "billing_correction:"+correctionReason).
		Where(`EXISTS (
			SELECT 1 FROM mirror_execution_intents trade
			WHERE trade.master_event_id = billing.master_event_id
			  AND trade.user_id = billing.user_id
			  AND trade.trader_id = billing.trader_id
			  AND trade.action = 'open'
			  AND trade.status = 'confirmed'
		)`).
		Order("billing.intent_key").Pluck("billing.intent_key", &keys).Error
	if err != nil {
		fmt.Fprintf(os.Stderr, "select corrections: category=%s\n", store.StoreErrorCategory(err))
		os.Exit(1)
	}
	if len(keys) != *expected {
		fmt.Fprintf(os.Stderr, "target_count=%d expected=%d; refusing\n", len(keys), *expected)
		os.Exit(1)
	}
	if !*apply {
		fmt.Printf("target_count=%d apply=false\n", len(keys))
		return
	}

	corrected := 0
	for _, key := range keys {
		changed, err := st.MirrorExecutionIntent().CorrectRefundedBillingForConfirmedOpen(key, correctionReason)
		if err != nil {
			fmt.Fprintf(os.Stderr, "correction failed: category=%s corrected=%d\n", store.StoreErrorCategory(err), corrected)
			os.Exit(1)
		}
		if changed {
			corrected++
		}
	}
	fmt.Printf("target_count=%d corrected=%d apply=true\n", len(keys), corrected)
}
