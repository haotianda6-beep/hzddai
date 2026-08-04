package main

import (
	"fmt"
	"os"
	"strings"

	appcrypto "nofx/crypto"
	"nofx/store"
)

func main() {
	if len(os.Args) != 2 || !strings.Contains(os.Args[1], "comkun-hz-qa") {
		panic("usage: hz_qa_refund_unexecuted QA_DATABASE")
	}
	cryptoService, err := appcrypto.NewCryptoService()
	must(err)
	appcrypto.SetGlobalCryptoService(cryptoService)
	st, err := store.New(os.Args[1])
	must(err)
	var billing []store.MirrorExecutionIntent
	must(st.GormDB().Where("action = ? AND billing_status = ?", "billing", store.MirrorBillingCharged).Find(&billing).Error)
	refunded := 0
	amount := 0.0
	for _, item := range billing {
		var confirmed int64
		must(st.GormDB().Model(&store.MirrorExecutionIntent{}).
			Where("master_event_id = ? AND user_id = ? AND trader_id = ? AND action <> ? AND status = ?",
				item.MasterEventID, item.UserID, item.TraderID, "billing", store.MirrorIntentConfirmed).
			Count(&confirmed).Error)
		if confirmed != 0 {
			continue
		}
		must(st.MirrorExecutionIntent().RefundBilling(item.IntentKey, "qa_reconcile_no_execution"))
		refunded++
		amount += item.BillingAmount
	}
	fmt.Printf("refunded=%d amount=%.6f\n", refunded, amount)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
