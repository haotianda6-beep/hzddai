package api

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"nofx/logger"
)

const hzPersistedMarketWatchInterval = 30 * time.Second

func (s *Server) startPersistedHZMasterMarketWatch() {
	masterAccountID := strings.TrimSpace(os.Getenv(hzMasterPollAccountIDEnv))
	if masterAccountID == "" || s.store == nil || s.watchHZMarket == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.hzMarketWatchCancel = cancel
	go func() {
		for {
			if err := s.renewPersistedHZMasterMarket(masterAccountID); err != nil {
				logger.Warnf("HZ persisted market watch renewal failed: %v", err)
			}
			timer := time.NewTimer(hzPersistedMarketWatchInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
}

func (s *Server) renewPersistedHZMasterMarket(masterAccountID string) error {
	raw, err := s.store.IntegrationMasterEvent().GetLatestMasterState(masterAccountID)
	if err != nil {
		return err
	}
	if raw == "" {
		return nil
	}
	var state struct {
		Positions []struct {
			Symbol string `json:"symbol"`
		} `json:"positions"`
	}
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return err
	}
	for _, position := range state.Positions {
		s.watchHZMarket(position.Symbol)
	}
	return nil
}
