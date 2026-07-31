package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"nofx/logger"
	"nofx/trader/hz"
)

const (
	hzMasterPollAPIURLEnv    = "HZ_MASTER_POLL_API_URL"
	hzMasterPollAPIKeyEnv    = "HZ_MASTER_POLL_API_KEY"
	hzMasterPollAPISecretEnv = "HZ_MASTER_POLL_API_SECRET"
	hzMasterPollAccountIDEnv = "HZ_MASTER_POLL_ACCOUNT_ID"
	hzMasterPollIntervalEnv  = "HZ_MASTER_POLL_INTERVAL_SECONDS"
)

type hzMasterPollConfig struct {
	APIURL, APIKey, Secret, MasterAccountID string
	Interval                                time.Duration
}

func hzMasterPollConfigFromEnv() (hzMasterPollConfig, bool) {
	config := hzMasterPollConfig{
		APIURL:          strings.TrimSpace(os.Getenv(hzMasterPollAPIURLEnv)),
		APIKey:          strings.TrimSpace(os.Getenv(hzMasterPollAPIKeyEnv)),
		Secret:          strings.TrimSpace(os.Getenv(hzMasterPollAPISecretEnv)),
		MasterAccountID: strings.TrimSpace(os.Getenv(hzMasterPollAccountIDEnv)),
		Interval:        30 * time.Second,
	}
	if seconds, err := strconv.Atoi(strings.TrimSpace(os.Getenv(hzMasterPollIntervalEnv))); err == nil && seconds >= 5 {
		config.Interval = time.Duration(seconds) * time.Second
	}
	return config, config.APIURL != "" && config.APIKey != "" && config.Secret != "" && config.MasterAccountID != ""
}

func (s *Server) startHZMasterEventPoller() {
	config, enabled := hzMasterPollConfigFromEnv()
	if !enabled || s.store == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.hzMasterPollCancel = cancel
	go func() {
		if err := s.pollHZMasterEventsOnce(ctx, config); err != nil {
			logger.Warnf("HZ master event reconciliation failed: %v", err)
		}
		ticker := time.NewTicker(config.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.pollHZMasterEventsOnce(ctx, config); err != nil {
					logger.Warnf("HZ master event reconciliation failed: %v", err)
				}
			}
		}
	}()
}

func (s *Server) pollHZMasterEventsOnce(ctx context.Context, config hzMasterPollConfig) error {
	latest, err := s.store.IntegrationMasterEvent().GetLatestSequence("hz", config.MasterAccountID)
	if err != nil {
		return err
	}
	raw, err := hz.FetchMasterSnapshot(ctx, config.APIURL, config.APIKey, config.Secret)
	if err != nil {
		return err
	}
	var snapshot hzMasterSnapshotResponse
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return fmt.Errorf("decode polled HZ master snapshot: %w", err)
	}
	sequence, err := strconv.ParseInt(snapshot.LastSequence, 10, 64)
	if err != nil || sequence < 0 {
		return fmt.Errorf("invalid HZ master snapshot sequence")
	}
	if sequence <= latest {
		return nil
	}
	if strings.TrimSpace(snapshot.MasterAccountID) != config.MasterAccountID {
		return fmt.Errorf("polled HZ snapshot master account mismatch")
	}
	request := hzMasterEventRequest{
		EventID:  fmt.Sprintf("reconcile-%x", sha256.Sum256([]byte(snapshot.MasterAccountID+"|"+snapshot.LastSequence))),
		Sequence: snapshot.LastSequence, EventType: "RECONCILE", OccurredAt: snapshot.CapturedAt,
		MasterAccountID: snapshot.MasterAccountID, Account: snapshot.Account, Positions: snapshot.Positions,
	}
	if err := request.validate(); err != nil {
		return err
	}
	eventRaw, err := json.Marshal(request)
	if err != nil {
		return err
	}
	nonce := fmt.Sprintf("poll:%s:%s", request.EventID, request.Sequence)
	if _, err := s.acceptHZMasterEvent(request, eventRaw, nonce, true, true); err != nil {
		return err
	}
	return nil
}
