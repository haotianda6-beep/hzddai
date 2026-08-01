package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"nofx/logger"
	"nofx/store"
)

const partnerRebateRetryInterval = 15 * time.Second

func (s *Server) startPartnerRebateOutboxWorker() {
	if s.store == nil {
		return
	}
	if _, _, ok := agentRebateEnvOK(); !ok {
		logger.Warn("partner rebate outbox worker disabled: service is not configured")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.partnerRebateCancel = cancel
	go func() {
		s.dispatchDuePartnerRebateEvents()
		ticker := time.NewTicker(partnerRebateRetryInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.dispatchDuePartnerRebateEvents()
			}
		}
	}()
}

func (s *Server) dispatchDuePartnerRebateEvents() {
	rows, err := s.store.PartnerRebateOutbox().ListDue(50)
	if err != nil {
		logger.Errorf("list partner rebate outbox: %v", err)
		return
	}
	for i := range rows {
		s.dispatchPartnerRebateEvent(&rows[i])
	}
}

func (s *Server) dispatchPartnerRebateOutboxID(id uint64) {
	if id == 0 {
		return
	}
	row, err := s.store.PartnerRebateOutbox().Get(id)
	if err != nil {
		logger.Errorf("load partner rebate outbox id=%d: %v", id, err)
		return
	}
	s.dispatchPartnerRebateEvent(row)
}

func (s *Server) dispatchPartnerRebateEvent(row *store.PartnerRebateOutbox) {
	if row == nil || row.Status != "pending" {
		return
	}
	if err := s.deliverPartnerRebateEvent(row); err != nil {
		logger.Errorf("deliver partner rebate outbox id=%d type=%s: %v", row.ID, row.EventType, err)
		if markErr := s.store.PartnerRebateOutbox().MarkFailed(row, err); markErr != nil {
			logger.Errorf("mark partner rebate outbox failed id=%d: %v", row.ID, markErr)
		}
		return
	}
	if err := s.store.PartnerRebateOutbox().MarkSent(row.ID); err != nil {
		logger.Errorf("mark partner rebate outbox sent id=%d: %v", row.ID, err)
	}
}

func (s *Server) deliverPartnerRebateEvent(row *store.PartnerRebateOutbox) error {
	path := ""
	payload := map[string]string{
		"amount_usdt": strconv.FormatFloat(row.AmountUSDT, 'f', -1, 64),
	}
	switch row.EventType {
	case store.PartnerRebateEventDeposit:
		if !s.syncInviteChainToAgentRebate(row.UserID) {
			return fmt.Errorf("sync invite chain failed")
		}
		path = "/api/platform/deposits"
		payload["platform_user_id"] = row.UserID
		payload["external_ref"] = fmt.Sprintf("hzddai_wallet_ledger:%d", row.WalletLedgerID)
		payload["source"] = row.Source
		payload["note"] = "主站确认充值"
	case store.PartnerRebateEventReversal:
		path = "/api/platform/deposit-reversals"
		payload["original_external_ref"] = fmt.Sprintf("hzddai_wallet_ledger:%d", row.OriginalWalletLedgerID)
		payload["external_ref"] = fmt.Sprintf("hzddai_wallet_reversal:%d", row.WalletLedgerID)
		payload["note"] = row.Note
	default:
		return fmt.Errorf("unsupported partner rebate event type %q", row.EventType)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := doAgentRebateRequest(http.MethodPost, path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("rebate service HTTP %d: %s", resp.StatusCode, body)
	}
	return nil
}
