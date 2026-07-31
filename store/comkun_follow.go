package store

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ComkunFollowDefaultTokensPerScan 每轮扫描默认消耗的虚拟 comkun token（config 为 0 时使用）
const ComkunFollowDefaultTokensPerScan = int64(50)

// ComkunMasterBroadcast 主账户侧发布的「分析 + 决策」快照，跟单用户按 source_strategy_id 拉取最新一条
type ComkunMasterBroadcast struct {
	ID                  uint64  `gorm:"primaryKey;autoIncrement" json:"id"`
	SourceStrategyID    string  `gorm:"column:source_strategy_id;not null;index" json:"source_strategy_id"`
	MasterAccountEquity float64 `gorm:"column:master_account_equity;not null;default:0" json:"master_account_equity"`
	AnalysisText        string  `gorm:"column:analysis_text;type:text" json:"analysis_text"`
	DecisionJSON        string  `gorm:"column:decision_json;type:text" json:"decision_json"`
	// MasterStateJSON 主控交易所快照：持仓 + 挂单（JSON），供被控「镜像跟单」按比例对齐仓位/限价/止盈止损
	MasterStateJSON string    `gorm:"column:master_state_json;type:text" json:"master_state_json"`
	CreatedAt       time.Time `json:"created_at"`
}

func (ComkunMasterBroadcast) TableName() string { return "comkun_master_broadcasts" }

// ComkunFollowTraderBalance 跟单虚拟 token 余额（按交易员维度；管理员充值、每轮扫描扣减）
type ComkunFollowTraderBalance struct {
	TraderID      string    `gorm:"column:trader_id;primaryKey" json:"trader_id"`
	BalanceTokens int64     `gorm:"column:balance_tokens;not null;default:0" json:"balance_tokens"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (ComkunFollowTraderBalance) TableName() string { return "comkun_follow_trader_balances" }

type ComkunFollowBroadcastConsumption struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TraderID    string    `gorm:"column:trader_id;not null;uniqueIndex:idx_comkun_consumption_once" json:"trader_id"`
	BroadcastID uint64    `gorm:"column:broadcast_id;not null;uniqueIndex:idx_comkun_consumption_once" json:"broadcast_id"`
	Status      string    `gorm:"column:status;not null;default:'processing'" json:"status"`
	Error       string    `gorm:"column:error;not null;default:''" json:"error"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (ComkunFollowBroadcastConsumption) TableName() string {
	return "comkun_follow_broadcast_consumptions"
}

// ComkunFollowStore 合规跟单 + 虚拟 token
type ComkunFollowStore struct {
	db *gorm.DB
}

func NewComkunFollowStore(db *gorm.DB) *ComkunFollowStore {
	return &ComkunFollowStore{db: db}
}

func (s *ComkunFollowStore) initTables() error {
	return s.db.AutoMigrate(
		&ComkunMasterBroadcast{},
		&ComkunFollowTraderBalance{},
		&ComkunFollowBroadcastConsumption{},
		&MT4FollowTicketMapping{},
	)
}

// InsertBroadcast 写入一条主账户广播（管理员或内部服务调用）
func (s *ComkunFollowStore) InsertBroadcast(sourceStrategyID string, masterEquity float64, analysisText, decisionJSON, masterStateJSON string) (*ComkunMasterBroadcast, error) {
	sourceStrategyID = strings.TrimSpace(sourceStrategyID)
	if sourceStrategyID == "" {
		return nil, fmt.Errorf("source_strategy_id required")
	}
	row := &ComkunMasterBroadcast{
		SourceStrategyID:    sourceStrategyID,
		MasterAccountEquity: masterEquity,
		AnalysisText:        analysisText,
		DecisionJSON:        decisionJSON,
		MasterStateJSON:     strings.TrimSpace(masterStateJSON),
		CreatedAt:           time.Now().UTC(),
	}
	if err := s.db.Create(row).Error; err != nil {
		return nil, err
	}
	NotifyComkunFollowersOfBroadcast(sourceStrategyID)
	return row, nil
}

// GetLatestBroadcast 取该市场源策略下最新一条广播
func (s *ComkunFollowStore) GetLatestBroadcast(sourceStrategyID string) (*ComkunMasterBroadcast, error) {
	sourceStrategyID = strings.TrimSpace(sourceStrategyID)
	if sourceStrategyID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var row ComkunMasterBroadcast
	err := s.db.Where("source_strategy_id = ?", sourceStrategyID).
		Order("id DESC").Limit(1).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *ComkunFollowStore) GetBroadcastByID(id uint64) (*ComkunMasterBroadcast, error) {
	if id == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var row ComkunMasterBroadcast
	if err := s.db.Where("id = ?", id).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *ComkunFollowStore) GetLatestAnalysisBroadcast(sourceStrategyID string) (*ComkunMasterBroadcast, error) {
	sourceStrategyID = strings.TrimSpace(sourceStrategyID)
	if sourceStrategyID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var row ComkunMasterBroadcast
	err := s.db.Where("source_strategy_id = ?", sourceStrategyID).
		Where("TRIM(COALESCE(analysis_text, '')) <> ''").
		Where("analysis_text NOT LIKE ?", "（本轮为交易所快照同步%").
		Order("id DESC").Limit(1).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

const (
	comkunConsumptionStaleProcessing      = 3 * time.Minute
	comkunConsumptionFailedRetryCooldown  = 2 * time.Minute
	comkunConsumptionErrorStartupBaseline = "startup_baseline"
)

// ReclaimStaleConsumptionLocks marks long-stuck processing rows as failed so followers can retry.
func (s *ComkunFollowStore) ReclaimStaleConsumptionLocks(traderID string, maxAge time.Duration) (int64, error) {
	traderID = strings.TrimSpace(traderID)
	if traderID == "" {
		return 0, nil
	}
	if maxAge <= 0 {
		maxAge = comkunConsumptionStaleProcessing
	}
	cutoff := time.Now().UTC().Add(-maxAge)
	var res *gorm.DB
	err := retrySQLiteBusy(s.db, func() error {
		res = s.db.Model(&ComkunFollowBroadcastConsumption{}).
			Where("trader_id = ? AND status = ? AND updated_at < ?", traderID, "processing", cutoff).
			Updates(map[string]interface{}{
				"status":     "failed",
				"error":      "stale_processing_reclaim",
				"updated_at": time.Now().UTC(),
			})
		return res.Error
	})
	if err != nil {
		return 0, err
	}
	return res.RowsAffected, res.Error
}

// TryAcquireConsumptionLock 抢占某被控交易员对某广播的执行权；同一 trader+broadcast 只能有一个执行者。
// failed 状态允许下一轮重试；processing/success 会被跳过。
func (s *ComkunFollowStore) TryAcquireConsumptionLock(traderID string, broadcastID uint64) (bool, error) {
	return s.TryAcquireConsumptionLockWithCooldown(traderID, broadcastID, comkunConsumptionFailedRetryCooldown)
}

// ConsumptionAttemptDue keeps exchange reads out of the hot polling path while
// another attempt is processing or a failed attempt is cooling down.
func (s *ComkunFollowStore) ConsumptionAttemptDue(traderID string, broadcastID uint64) (bool, error) {
	return s.ConsumptionAttemptDueWithCooldown(traderID, broadcastID, comkunConsumptionFailedRetryCooldown)
}

func (s *ComkunFollowStore) ConsumptionAttemptDueWithCooldown(
	traderID string,
	broadcastID uint64,
	retryCooldown time.Duration,
) (bool, error) {
	traderID = strings.TrimSpace(traderID)
	if traderID == "" || broadcastID == 0 {
		return false, fmt.Errorf("trader_id and broadcast_id required")
	}
	var existing ComkunFollowBroadcastConsumption
	err := s.db.Where("trader_id = ? AND broadcast_id = ?", traderID, broadcastID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	sinceUpdate := time.Since(existing.UpdatedAt)
	switch existing.Status {
	case "success":
		return false, nil
	case "processing":
		return existing.UpdatedAt.IsZero() || sinceUpdate >= comkunConsumptionStaleProcessing, nil
	case "failed":
		return retryCooldown <= 0 || existing.UpdatedAt.IsZero() || sinceUpdate >= retryCooldown, nil
	default:
		return true, nil
	}
}

func (s *ComkunFollowStore) TryAcquireConsumptionLockWithCooldown(traderID string, broadcastID uint64, retryCooldown time.Duration) (bool, error) {
	traderID = strings.TrimSpace(traderID)
	if traderID == "" || broadcastID == 0 {
		return false, fmt.Errorf("trader_id and broadcast_id required")
	}
	now := time.Now().UTC()
	row := &ComkunFollowBroadcastConsumption{
		TraderID:    traderID,
		BroadcastID: broadcastID,
		Status:      "processing",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	var res *gorm.DB
	if err := retrySQLiteBusy(s.db, func() error {
		res = s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(row)
		return res.Error
	}); err != nil {
		return false, err
	}
	if res.RowsAffected > 0 {
		return true, nil
	}

	var existing ComkunFollowBroadcastConsumption
	if err := s.db.Where("trader_id = ? AND broadcast_id = ?", traderID, broadcastID).First(&existing).Error; err != nil {
		return false, err
	}
	eligible := existing.Status == "failed" && (retryCooldown <= 0 || existing.UpdatedAt.IsZero() || time.Since(existing.UpdatedAt) >= retryCooldown)
	if existing.Status == "processing" && (existing.UpdatedAt.IsZero() || time.Since(existing.UpdatedAt) >= comkunConsumptionStaleProcessing) {
		eligible = true
	}
	if !eligible {
		return false, nil
	}
	previousStatus := existing.Status
	if err := retrySQLiteBusy(s.db, func() error {
		res = s.db.Model(&ComkunFollowBroadcastConsumption{}).
			Where("id = ? AND status = ? AND updated_at = ?", existing.ID, previousStatus, existing.UpdatedAt).
			Updates(map[string]interface{}{"status": "processing", "error": "", "updated_at": now})
		return res.Error
	}); err != nil {
		return false, err
	}
	return res.RowsAffected == 1, nil
}

// GetMaxSuccessfulConsumptionBroadcastID 已成功消费（含静默跳过落库）的广播 id 最大值，供被控重启时与决策表共同恢复水位线。
func (s *ComkunFollowStore) GetMaxSuccessfulConsumptionBroadcastID(traderID string) uint64 {
	traderID = strings.TrimSpace(traderID)
	if traderID == "" {
		return 0
	}
	var maxID uint64
	err := s.db.Model(&ComkunFollowBroadcastConsumption{}).
		Where("trader_id = ? AND status = ?", traderID, "success").
		Select("COALESCE(MAX(broadcast_id), 0)").
		Scan(&maxID).Error
	if err != nil {
		return 0
	}
	return maxID
}

func (s *ComkunFollowStore) GetConsumptionStatus(traderID string, broadcastID uint64) (string, error) {
	traderID = strings.TrimSpace(traderID)
	if traderID == "" || broadcastID == 0 {
		return "", nil
	}
	var row ComkunFollowBroadcastConsumption
	err := s.db.Where("trader_id = ? AND broadcast_id = ?", traderID, broadcastID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return row.Status, nil
}

func (s *ComkunFollowStore) MarkConsumptionSuccess(traderID string, broadcastID uint64) error {
	return s.MarkConsumptionSuccessWithCheckpoint(traderID, broadcastID, "")
}

func (s *ComkunFollowStore) MarkConsumptionSuccessWithCheckpoint(traderID string, broadcastID uint64, checkpoint string) error {
	if len(checkpoint) > 1000 {
		checkpoint = checkpoint[:1000]
	}
	return retrySQLiteBusy(s.db, func() error {
		res := s.db.Model(&ComkunFollowBroadcastConsumption{}).
			Where("trader_id = ? AND broadcast_id = ?", strings.TrimSpace(traderID), broadcastID).
			Updates(map[string]interface{}{"status": "success", "error": checkpoint, "updated_at": time.Now().UTC()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return fmt.Errorf("consumption checkpoint not found")
		}
		return nil
	})
}

func (s *ComkunFollowStore) GetConsumptionCheckpoint(traderID string, broadcastID uint64) string {
	traderID = strings.TrimSpace(traderID)
	if traderID == "" || broadcastID == 0 {
		return ""
	}
	var checkpoint string
	err := s.db.Model(&ComkunFollowBroadcastConsumption{}).
		Where("trader_id = ? AND broadcast_id = ? AND status = ?", traderID, broadcastID, "success").
		Select("error").Scan(&checkpoint).Error
	if err != nil {
		return ""
	}
	return checkpoint
}

func (s *ComkunFollowStore) MarkConsumptionStartupBaseline(traderID string, broadcastID uint64) error {
	traderID = strings.TrimSpace(traderID)
	if traderID == "" || broadcastID == 0 {
		return nil
	}
	now := time.Now().UTC()
	row := &ComkunFollowBroadcastConsumption{
		TraderID:    traderID,
		BroadcastID: broadcastID,
		Status:      "success",
		Error:       comkunConsumptionErrorStartupBaseline,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return retrySQLiteBusy(s.db, func() error {
		return s.db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "trader_id"}, {Name: "broadcast_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"status":     "success",
				"error":      comkunConsumptionErrorStartupBaseline,
				"updated_at": now,
			}),
		}).Create(row).Error
	})
}

func (s *ComkunFollowStore) IsStartupBaselineConsumption(traderID string, broadcastID uint64) bool {
	traderID = strings.TrimSpace(traderID)
	if traderID == "" || broadcastID == 0 {
		return false
	}
	var n int64
	err := s.db.Model(&ComkunFollowBroadcastConsumption{}).
		Where("trader_id = ? AND broadcast_id = ? AND status = ? AND error = ?",
			traderID, broadcastID, "success", comkunConsumptionErrorStartupBaseline).
		Count(&n).Error
	return err == nil && n > 0
}

func (s *ComkunFollowStore) MarkConsumptionFailed(traderID string, broadcastID uint64, msg string) error {
	if len(msg) > 1000 {
		msg = msg[:1000]
	}
	return retrySQLiteBusy(s.db, func() error {
		return s.db.Model(&ComkunFollowBroadcastConsumption{}).
			Where("trader_id = ? AND broadcast_id = ?", strings.TrimSpace(traderID), broadcastID).
			Updates(map[string]interface{}{"status": "failed", "error": msg, "updated_at": time.Now().UTC()}).Error
	})
}

// LatestFailedConsumptionError 跟单被控端最近一次广播消费失败的原因（用于主控看板状态提示）
func (s *ComkunFollowStore) LatestFailedConsumptionError(traderID string) (msg string, at time.Time, ok bool) {
	traderID = strings.TrimSpace(traderID)
	if traderID == "" {
		return "", time.Time{}, false
	}
	var row ComkunFollowBroadcastConsumption
	err := s.db.Where("trader_id = ? AND status = ? AND COALESCE(error, '') != ''", traderID, "failed").
		Where("LOWER(error) NOT LIKE ?", "%database%locked%").
		Order("updated_at DESC").
		Limit(1).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", time.Time{}, false
		}
		return "", time.Time{}, false
	}
	msg = strings.TrimSpace(row.Error)
	if msg == "" {
		return "", time.Time{}, false
	}
	if len([]rune(msg)) > 200 {
		rs := []rune(msg)
		msg = string(rs[:200]) + "…"
	}
	return msg, row.UpdatedAt, true
}

// ListBroadcasts 主控端历史广播（新在前），用于策略构建器看板
func (s *ComkunFollowStore) ListBroadcasts(sourceStrategyID string, limit int) ([]ComkunMasterBroadcast, error) {
	sourceStrategyID = strings.TrimSpace(sourceStrategyID)
	if sourceStrategyID == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []ComkunMasterBroadcast
	err := s.db.Where("source_strategy_id = ?", sourceStrategyID).
		Order("id DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

// GetFollowBalance 读取跟单虚拟 token
func (s *ComkunFollowStore) GetFollowBalance(traderID string) (int64, error) {
	var row ComkunFollowTraderBalance
	err := s.db.Where("trader_id = ?", traderID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return row.BalanceTokens, nil
}

// CreditFollowBalance 增加虚拟 token（管理员充值）
func (s *ComkunFollowStore) CreditFollowBalance(traderID string, delta int64) error {
	if delta <= 0 {
		return fmt.Errorf("delta must be positive")
	}
	traderID = strings.TrimSpace(traderID)
	if traderID == "" {
		return fmt.Errorf("trader_id required")
	}
	now := time.Now().UTC()
	return s.db.Transaction(func(tx *gorm.DB) error {
		var row ComkunFollowTraderBalance
		err := tx.Where("trader_id = ?", traderID).First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&ComkunFollowTraderBalance{
				TraderID:      traderID,
				BalanceTokens: delta,
				UpdatedAt:     now,
			}).Error
		}
		if err != nil {
			return err
		}
		row.BalanceTokens += delta
		row.UpdatedAt = now
		return tx.Save(&row).Error
	})
}

// DebitFollowBalanceIfEnough 每轮扫描扣减；余额不足返回 ok=false
func (s *ComkunFollowStore) DebitFollowBalanceIfEnough(traderID string, cost int64) (ok bool, remaining int64, err error) {
	if cost <= 0 {
		return false, 0, fmt.Errorf("cost must be positive")
	}
	traderID = strings.TrimSpace(traderID)
	if traderID == "" {
		return false, 0, fmt.Errorf("trader_id required")
	}
	var rem int64
	e := s.db.Transaction(func(tx *gorm.DB) error {
		var row ComkunFollowTraderBalance
		err := tx.Where("trader_id = ?", traderID).First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			rem = 0
			return errComkunInsufficient
		}
		if err != nil {
			return err
		}
		if row.BalanceTokens < cost {
			rem = row.BalanceTokens
			return errComkunInsufficient
		}
		row.BalanceTokens -= cost
		rem = row.BalanceTokens
		row.UpdatedAt = time.Now().UTC()
		return tx.Save(&row).Error
	})
	if errors.Is(e, errComkunInsufficient) {
		return false, rem, nil
	}
	if e != nil {
		return false, 0, e
	}
	return true, rem, nil
}

var errComkunInsufficient = errors.New("comkun insufficient balance")

const bnScreenMirrorMasterStrategyPrefix = "bn-screen-mirror-"
const okxScreenMirrorMasterStrategyPrefix = "okx-screen-mirror-"

func IsOkxScreenMirrorMasterStrategyID(strategyID string) bool {
	return strings.HasPrefix(strings.TrimSpace(strategyID), okxScreenMirrorMasterStrategyPrefix)
}

// IsBnScreenMirrorMasterStrategyID 网页油猴/API 爬虫主控：bn-screen-mirror-* 或 okx-screen-mirror-*，广播走 POST /api/comkun/screen-broadcast。
// 被控通过 comkun_market_source_strategy_id 指向该 ID 时走「仅市价」简化镜像，与 API 主控（究极 SOL 等）全量 v2 区分。
func IsBnScreenMirrorMasterStrategyID(strategyID string) bool {
	s := strings.TrimSpace(strategyID)
	return strings.HasPrefix(s, bnScreenMirrorMasterStrategyPrefix) ||
		strings.HasPrefix(s, okxScreenMirrorMasterStrategyPrefix)
}

// IsComkunMarketFollowStrategy 策略 config 打开「市场合规跟单」开关（被控：消费主广播、走镜像/token 路径）。
// 与 comkun_follow_listing_template（主控发广播）互斥：若 JSON 误将两者同时设为 true，仍按主控处理，
// 否则 runCycle 会走「等主控广播」静默分支且 maybePublish 拒发广播，表现为「主控一次都不扫描」。
func IsComkunMarketFollowStrategy(c *StrategyConfig) bool {
	if c == nil {
		return false
	}
	if c.ComkunFollowListingTemplate {
		return false
	}
	return c.ComkunMarketFollow
}

// ListingTemplateMasterSkipsExchangeExecution 跟单开关「主控」上架模板是否跳过向交易所执行 AI 决策。
// 默认 true（仅分析+广播，如究极 SOL 人工主控）；显式 allow_execute 或 skip_exchange_execution=false 时主控 AI 正常下单并广播。
func ListingTemplateMasterSkipsExchangeExecution(cfg *StrategyConfig) bool {
	if cfg == nil {
		return false
	}
	if !cfg.ComkunFollowListingTemplate || IsComkunMarketFollowStrategy(cfg) {
		return false
	}
	if cfg.ComkunListingTemplateMasterAllowExecute {
		return false
	}
	if !cfg.ComkunListingMasterSkipExchangeExecution {
		return false
	}
	return true
}

// ComkunFollowTokensPerScanOrDefault config 为 0 或未设置时返回默认扣费
func ComkunFollowTokensPerScanOrDefault(c *StrategyConfig) int64 {
	if c == nil || c.ComkunFollowTokensPerScan <= 0 {
		return ComkunFollowDefaultTokensPerScan
	}
	return int64(c.ComkunFollowTokensPerScan)
}

// IsComkunAIModelRow 是否为用户配置中的「COMKUN-AI」占位模型（不含 comkun_proxy 多模型代理）
func IsComkunAIModelRow(m *AIModel) bool {
	if m == nil {
		return false
	}
	return strings.TrimSpace(m.Provider) == "comkun_ai" || strings.TrimSpace(m.Provider) == "ai" || strings.TrimSpace(m.ID) == "comkun_ai"
}

// StrategyRequiresComkunAIModel 跟单/上架模板类策略：前端推荐优先绑定 COMKUN-AI（与 IsComkunMarketFollowStrategy 及主控模板一致）
func StrategyRequiresComkunAIModel(cfg *StrategyConfig) bool {
	if cfg == nil {
		return false
	}
	if cfg.ComkunFollowListingTemplate {
		// 主控 AI 实盘（非仅广播）继续用 DeepSeek/代理等真实模型
		return ListingTemplateMasterSkipsExchangeExecution(cfg)
	}
	if IsComkunProgramMartingaleStrategy(cfg) {
		return true
	}
	return IsComkunMarketFollowStrategy(cfg)
}

// ValidateComkunAIStrategyBinding COMKUN-AI ↔ 策略类型约束。
// 跟单类策略推荐 COMKUN-AI，但允许用户选择其他真实模型；普通策略仍不能使用 COMKUN-AI 占位通道。
func ValidateComkunAIStrategyBinding(model *AIModel, cfg *StrategyConfig) error {
	req := StrategyRequiresComkunAIModel(cfg)
	comkun := IsComkunAIModelRow(model)
	if comkun && !req {
		return fmt.Errorf("COMKUN-AI 仅允许用于合规跟单、官方上架跟单模板或 program_martingale 策略，请更换 AI 模型或调整策略类型")
	}
	return nil
}

const (
	comkunFollowDefaultDailyFeeTargetUSDT          = 6.5
	comkunFollowDailyFeeMinUSDT                    = 5.0
	comkunFollowDailyFeeMaxUSDT                    = 8.0
	comkunFollowDefaultExpectedAnalysesPerDay      = 96.0
	comkunFollowBlankDefaultDailyFeeTargetUSDT     = 6.5
	comkunFollowBlankDefaultExpectedAnalysesPerDay = 6500.0
	comkunFollowDefaultScanFeeJitterPct            = 0.12
)

func comkunFollowFloatEnvOrDefault(name string, fallback float64) float64 {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 {
		return fallback
	}
	return f
}

func ComkunFollowDailyFeeRangeUSDTOrDefault() (float64, float64) {
	min := comkunFollowFloatEnvOrDefault("COMKUN_FOLLOW_DAILY_FEE_MIN_USDT", comkunFollowDailyFeeMinUSDT)
	max := comkunFollowFloatEnvOrDefault("COMKUN_FOLLOW_DAILY_FEE_MAX_USDT", comkunFollowDailyFeeMaxUSDT)
	if min <= 0 {
		min = comkunFollowDailyFeeMinUSDT
	}
	if max < min {
		max = min
	}
	return min, max
}

func ComkunFollowExpectedAnalysesPerDayOrDefault() float64 {
	expected := comkunFollowFloatEnvOrDefault("COMKUN_FOLLOW_EXPECTED_ANALYSES_PER_DAY", comkunFollowDefaultExpectedAnalysesPerDay)
	if expected <= 0 {
		return comkunFollowDefaultExpectedAnalysesPerDay
	}
	return expected
}

func ComkunFollowBlankExpectedAnalysesPerDayOrDefault() float64 {
	expected := comkunFollowFloatEnvOrDefault("COMKUN_FOLLOW_BLANK_EXPECTED_ANALYSES_PER_DAY", comkunFollowBlankDefaultExpectedAnalysesPerDay)
	if expected <= 0 {
		return comkunFollowBlankDefaultExpectedAnalysesPerDay
	}
	return expected
}

// ComkunFollowScanFeeUSDTOrDefault COMKUN-AI 每次消费真实 AI 分析的基准费用。
// 默认按「约 6.5U/天」折算：6.5 / 96（约每 15 分钟一次 AI 分析）。
// 可用 COMKUN_FOLLOW_SCAN_FEE_USDT 直接覆盖单次基准费用；
// 或用 COMKUN_FOLLOW_DAILY_FEE_TARGET_USDT / COMKUN_FOLLOW_EXPECTED_ANALYSES_PER_DAY 调整目标。
func ComkunFollowScanFeeUSDTOrDefault() float64 {
	if v := strings.TrimSpace(os.Getenv("COMKUN_FOLLOW_SCAN_FEE_USDT")); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err == nil && f >= 0 {
			return f
		}
	}
	target := comkunFollowFloatEnvOrDefault("COMKUN_FOLLOW_DAILY_FEE_TARGET_USDT", comkunFollowDefaultDailyFeeTargetUSDT)
	return target / ComkunFollowExpectedAnalysesPerDayOrDefault()
}

// ComkunFollowBlankScanFeeUSDTOrDefault 0 元订阅按主控空白/快照 AI 输出次数计费。
// 默认按「约 6.5U/天」折算：6.5 / 6500，匹配现网究极 SOL 空白快照频率。
func ComkunFollowBlankScanFeeUSDTOrDefault() float64 {
	if v := strings.TrimSpace(os.Getenv("COMKUN_FOLLOW_BLANK_SCAN_FEE_USDT")); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err == nil && f >= 0 {
			return f
		}
	}
	target := comkunFollowFloatEnvOrDefault("COMKUN_FOLLOW_BLANK_DAILY_FEE_TARGET_USDT", comkunFollowBlankDefaultDailyFeeTargetUSDT)
	return target / ComkunFollowBlankExpectedAnalysesPerDayOrDefault()
}

// ComkunFollowScanFeeJitterPctOrDefault 单次费用随机浮动比例，默认上下浮动 12%。
func ComkunFollowScanFeeJitterPctOrDefault() float64 {
	jitter := comkunFollowFloatEnvOrDefault("COMKUN_FOLLOW_SCAN_FEE_JITTER_PCT", comkunFollowDefaultScanFeeJitterPct)
	if jitter > 0.5 {
		return 0.5
	}
	return jitter
}

func ComkunFollowScanFeeMaxUSDTOrDefault() float64 {
	_, max := ComkunFollowDailyFeeRangeUSDTOrDefault()
	if max > 0 {
		return max / ComkunFollowExpectedAnalysesPerDayOrDefault()
	}
	return ComkunFollowScanFeeUSDTOrDefault() * (1 + ComkunFollowScanFeeJitterPctOrDefault())
}

// ResolveComkunFollowSourceStrategyID 策略内配置的源策略 ID，缺省时读环境变量 COMKUN_FOLLOW_DEFAULT_SOURCE_STRATEGY_ID
func ResolveComkunFollowSourceStrategyID(c *StrategyConfig) string {
	if c != nil {
		if s := strings.TrimSpace(c.ComkunMarketSourceStrategyID); s != "" {
			return s
		}
	}
	return strings.TrimSpace(os.Getenv("COMKUN_FOLLOW_DEFAULT_SOURCE_STRATEGY_ID"))
}

// ComkunMirrorMasterMarginLeverageOrDefault 镜像保证金比例公式里主控假定杠杆；未配置或非法时 20。
func ComkunMirrorMasterMarginLeverageOrDefault(c *StrategyConfig) int {
	if c == nil || c.ComkunMirrorMasterMarginLeverage < 1 {
		return 20
	}
	if c.ComkunMirrorMasterMarginLeverage > 125 {
		return 125
	}
	return c.ComkunMirrorMasterMarginLeverage
}

// ComkunMirrorFollowerMarginLeverageOrDefault 被控镜像 SetLeverage 与名义还原用杠杆；未配置或非法时 20。
func ComkunMirrorFollowerMarginLeverageOrDefault(c *StrategyConfig) int {
	if c == nil || c.ComkunMirrorFollowerMarginLeverage < 1 {
		return 20
	}
	if c.ComkunMirrorFollowerMarginLeverage > 125 {
		return 125
	}
	return c.ComkunMirrorFollowerMarginLeverage
}
