package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"nofx/logger"
	"nofx/store"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleAdminComkunFollowingStats(c *gin.Context) {
	rows, totals, err := s.comkunFollowingStatsSnapshot()
	if err != nil {
		SafeInternalError(c, "跟单统计读取失败", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"observed_at": time.Now().UTC(),
		"platform": gin.H{
			"running_follower_count":    totals.RunningFollowerCount,
			"running_trader_count":      totals.RunningTraderCount,
			"subscribed_follower_count": totals.SubscribedFollowerCount,
		},
		"strategies": rows,
	})
}

func (s *Server) handleComkunFollowingStats(c *gin.Context) {
	rows, _, err := s.comkunFollowingStatsSnapshot()
	if err != nil {
		SafeInternalError(c, "跟单统计读取失败", err)
		return
	}
	userID := strings.TrimSpace(c.GetString("user_id"))
	mapping, _ := store.ComkunMasterSourceMapping()
	owned := make(map[string]struct{})
	if strategies, err := s.store.Strategy().List(userID); err == nil {
		for _, strategy := range strategies {
			var cfg store.StrategyConfig
			if strategy != nil && jsonUnmarshalStrategyConfig(strategy.Config, &cfg) == nil && cfg.ComkunFollowListingTemplate {
				owned[strategy.ID] = struct{}{}
			}
		}
	}
	scoped := scopeComkunFollowingStats(rows, mapping, userID, owned)
	if len(scoped) == 0 && !isAdminEmail(c.GetString("email")) {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权查看其他主控的跟单统计"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"observed_at": time.Now().UTC(), "strategies": scoped})
}

func jsonUnmarshalStrategyConfig(raw string, cfg *store.StrategyConfig) error {
	return json.Unmarshal([]byte(raw), cfg)
}

func (s *Server) comkunFollowingStatsSnapshot() ([]store.ComkunFollowingStatsRow, store.ComkunFollowingStatsTotals, error) {
	rows, err := s.store.Trader().ListComkunFollowingStats()
	if err != nil {
		return nil, store.ComkunFollowingStatsTotals{}, err
	}
	totals, err := s.store.Trader().ComkunFollowingStatsTotals(rows)
	return rows, totals, err
}

func (s *Server) requestComkunFollowStatsPush() {
	if s.comkunFollowStatsTrigger == nil {
		return
	}
	select {
	case s.comkunFollowStatsTrigger <- struct{}{}:
	default:
	}
}

func (s *Server) startComkunFollowStatsPublisher() {
	target := strings.TrimSpace(os.Getenv("COMKUN_FOLLOW_STATS_TARGET_URL"))
	secret := strings.TrimSpace(os.Getenv("COMKUN_AI_EVENT_SECRET"))
	if target == "" || secret == "" {
		return
	}
	interval := 300
	if raw, err := strconv.Atoi(strings.TrimSpace(os.Getenv("COMKUN_FOLLOW_STATS_INTERVAL_SECONDS"))); err == nil && raw > 0 {
		interval = raw
	}
	if interval < 60 {
		interval = 60
	}
	s.comkunFollowStatsTrigger = make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	s.comkunFollowStatsCancel = cancel
	delivery := newComkunStatsDelivery(&http.Client{Timeout: 20 * time.Second}, nil)
	go func() {
		ticker := time.NewTicker(time.Duration(interval) * time.Second)
		defer ticker.Stop()
		publish := func() {
			rows, err := s.store.Trader().ListComkunFollowingStats()
			if err != nil {
				logger.Warnf("comkun follow stats query failed: %v", err)
				return
			}
			mapping, err := store.ComkunMasterSourceMapping()
			if err != nil {
				logger.Warnf("comkun follow stats stable mapping unavailable")
				return
			}
			raw, payload, err := buildComkunFollowingStatsPayload(rows, mapping, time.Now().UTC())
			if err != nil {
				logger.Warnf("comkun follow stats payload invalid: %v", err)
				return
			}
			if err := delivery.send(ctx, target, secret, payload.SourceEventID, raw); err != nil {
				logger.Warnf("comkun follow stats delivery failed: %v", err)
			}
		}
		publish()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				publish()
			case <-s.comkunFollowStatsTrigger:
				publish()
			}
		}
	}()
}
