package store

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"nofx/crypto"
	"nofx/logger"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// OutboundProxyPool 管理员维护的 SOCKS5/HTTP 出口池；与交易所账户一对一绑定，不重复分配。
type OutboundProxyPool struct {
	ID                 string                 `gorm:"primaryKey" json:"id"`
	DisplayHost        string                 `gorm:"column:display_host;not null;default:''" json:"display_host"`
	ProxyURL           crypto.EncryptedString `gorm:"column:proxy_url;not null" json:"-"`
	ExpiresAt          *time.Time             `gorm:"column:expires_at" json:"expires_at"`
	AssignedUserID     string                 `gorm:"column:assigned_user_id;index" json:"assigned_user_id"`
	AssignedExchangeID string                 `gorm:"column:assigned_exchange_id;index" json:"assigned_exchange_id"`
	AssignedAt         *time.Time             `gorm:"column:assigned_at" json:"assigned_at"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
}

func (OutboundProxyPool) TableName() string { return "outbound_proxy_pool" }

// ProxyPoolStore 出口代理池
type ProxyPoolStore struct {
	db *gorm.DB
}

func NewProxyPoolStore(db *gorm.DB) *ProxyPoolStore {
	return &ProxyPoolStore{db: db}
}

func (s *ProxyPoolStore) initTables() error {
	return s.db.AutoMigrate(&OutboundProxyPool{})
}

// ErrProxyPoolExhausted 无可用未分配代理
var ErrProxyPoolExhausted = errors.New("outbound proxy pool exhausted: no unassigned entry")

// ImportLines 批量导入；重复 display_host+proxy 指纹由管理员避免；解析失败行计入 skipped
func (s *ProxyPoolStore) ImportLines(lines []string) (added int, skipped int, errs []string) {
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		socksURL, displayHost, exp, err := ParseProxyImportLine(line)
		if err != nil {
			skipped++
			errs = append(errs, fmt.Sprintf("%s: %v", truncateForErr(line, 40), err))
			continue
		}
		if displayHost == "" {
			displayHost = "—"
		}
		row := OutboundProxyPool{
			ID:          uuid.New().String(),
			DisplayHost: displayHost,
			ProxyURL:    crypto.EncryptedString(socksURL),
			ExpiresAt:   exp,
		}
		if err := s.db.Create(&row).Error; err != nil {
			skipped++
			errs = append(errs, fmt.Sprintf("%s: %v", displayHost, err))
			continue
		}
		added++
	}
	return
}

func classifyProxyHealthError(err error) string {
	if err == nil {
		return "other"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "timeout"), strings.Contains(msg, "i/o timeout"):
		return "timeout"
	case strings.Contains(msg, "refused"), strings.Contains(msg, "unreachable"):
		return "refused"
	default:
		return "other"
	}
}

func truncateForErr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// AdminProxyPoolRow 管理端列表（嵌入 OutboundProxyPool 的基础字段）
type AdminProxyPoolRow struct {
	OutboundProxyPool
	AssignedUserEmail       string `json:"assigned_user_email"`
	AssignedUserDisplayName string `json:"assigned_user_display_name"`
	AssignedExchangeName    string `json:"assigned_exchange_account_name"`
	SecondsUntilExpiry      *int64 `json:"seconds_until_expiry,omitempty"`
}

// ListAdmin 全部池条目 + 分配信息
func (s *ProxyPoolStore) ListAdmin() ([]AdminProxyPoolRow, error) {
	var rows []OutboundProxyPool
	if err := s.db.Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]AdminProxyPoolRow, 0, len(rows))
	now := time.Now()
	for _, r := range rows {
		item := AdminProxyPoolRow{OutboundProxyPool: r}
		if r.ExpiresAt != nil {
			sec := int64(r.ExpiresAt.Sub(now).Seconds())
			if sec < 0 {
				sec = 0
			}
			item.SecondsUntilExpiry = &sec
		}
		if r.AssignedUserID != "" {
			var u User
			if err := s.db.Where("id = ?", r.AssignedUserID).First(&u).Error; err == nil {
				item.AssignedUserEmail = u.Email
				item.AssignedUserDisplayName = u.DisplayName
			}
		}
		if r.AssignedExchangeID != "" {
			var ex Exchange
			if err := s.db.Select("account_name").Where("id = ?", r.AssignedExchangeID).First(&ex).Error; err == nil {
				item.AssignedExchangeName = ex.AccountName
			}
		}
		out = append(out, item)
	}
	return out, nil
}

// DeleteUnassigned 仅未分配可删
func (s *ProxyPoolStore) DeleteUnassigned(id string) error {
	result := s.db.Where("id = ? AND (COALESCE(assigned_exchange_id, '') = '')", id).Delete(&OutboundProxyPool{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("未找到可删除的未分配记录或 id 无效")
	}
	return nil
}

// ReleaseByExchangeID 解绑某交易所占用的池条目（清空分配字段）
func (s *ProxyPoolStore) ReleaseByExchangeID(exchangeID string) error {
	exchangeID = strings.TrimSpace(exchangeID)
	if exchangeID == "" {
		return nil
	}
	res := s.db.Model(&OutboundProxyPool{}).
		Where("assigned_exchange_id = ?", exchangeID).
		Updates(map[string]interface{}{
			"assigned_user_id":     "",
			"assigned_exchange_id": "",
			"assigned_at":          nil,
			"updated_at":           time.Now().UTC(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		logger.Infof("🔓 代理池：已释放交易所绑定 exchange_id=%s", exchangeID)
	}
	return nil
}

// ClaimFirstUnassignedForExchange 保留旧调用方；新建/启动 CEX 时应传入 exchangeType，允许不同平台复用同一出口。
func (s *ProxyPoolStore) ClaimFirstUnassignedForExchange(tx *gorm.DB, userID, exchangeID string) (poolID string, plainProxyURL string, err error) {
	return s.ClaimProxyForExchange(tx, userID, exchangeID, "")
}

// ClaimProxyForExchange 为同一用户分配出口：不同交易所平台可复用同一 IP，同一平台必须取新 IP。
// 复用时复制一条池记录，保留每个交易所独立的过期校验和释放能力。
func (s *ProxyPoolStore) ClaimProxyForExchange(tx *gorm.DB, userID, exchangeID, exchangeType string) (poolID string, plainProxyURL string, err error) {
	userID = strings.TrimSpace(userID)
	exchangeID = strings.TrimSpace(exchangeID)
	exchangeType = strings.TrimSpace(strings.ToLower(exchangeType))
	if userID == "" || exchangeID == "" {
		return "", "", fmt.Errorf("userID/exchangeID 为空")
	}
	now := time.Now().UTC()
	if exchangeType != "" {
		var reusable OutboundProxyPool
		err := tx.Table("outbound_proxy_pool AS p").
			Select("p.*").
			Joins("JOIN exchanges e ON e.id = p.assigned_exchange_id").
			Where("p.assigned_user_id = ? AND p.assigned_exchange_id <> ''", userID).
			Where("e.user_id = ? AND LOWER(e.exchange_type) <> ?", userID, exchangeType).
			Where("p.expires_at IS NULL OR p.expires_at > ?", now).
			Order("p.created_at ASC").First(&reusable).Error
		if err == nil {
			plain := strings.TrimSpace(string(reusable.ProxyURL))
			if plain != "" {
				clone := OutboundProxyPool{
					ID:                 uuid.New().String(),
					DisplayHost:        reusable.DisplayHost,
					ProxyURL:           crypto.EncryptedString(plain),
					ExpiresAt:          reusable.ExpiresAt,
					AssignedUserID:     userID,
					AssignedExchangeID: exchangeID,
					AssignedAt:         &now,
					CreatedAt:          now,
					UpdatedAt:          now,
				}
				if err := tx.Create(&clone).Error; err != nil {
					return "", "", err
				}
				logger.Infof("♻️ 代理池：复用同一出口 host=%s，原平台与新平台不同 user=%s exchange_id=%s", clone.DisplayHost, userID, exchangeID)
				return clone.ID, plain, nil
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", err
		}
	}

	skipped := make([]string, 0, 4)
	faultStore := NewOutboundProxyFaultStore(s.db)
	for attempt := 0; attempt < 10; attempt++ {
		var cand OutboundProxyPool
		q := tx.Where("COALESCE(assigned_exchange_id, '') = ''")
		q = q.Where("expires_at IS NULL OR expires_at > ?", now)
		if len(skipped) > 0 {
			q = q.Where("id NOT IN ?", skipped)
		}
		err := q.Order("created_at ASC").First(&cand).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return "", "", ErrProxyPoolExhausted
			}
			return "", "", err
		}
		plain := strings.TrimSpace(string(cand.ProxyURL))
		if plain == "" {
			skipped = append(skipped, cand.ID)
			continue
		}
		if exchangeType != "" {
			var samePlatformCount int64
			countErr := tx.Table("outbound_proxy_pool AS p").
				Joins("JOIN exchanges e ON e.id = p.assigned_exchange_id").
				Where("p.assigned_user_id = ? AND p.display_host = ?", userID, cand.DisplayHost).
				Where("LOWER(e.exchange_type) = ?", exchangeType).
				Where("p.expires_at IS NULL OR p.expires_at > ?", now).
				Count(&samePlatformCount).Error
			if countErr != nil {
				return "", "", countErr
			}
			if samePlatformCount > 0 {
				skipped = append(skipped, cand.ID)
				continue
			}
		}
		if hcErr := TCPHealthCheckProxyURL(plain, 4*time.Second); hcErr != nil {
			skipped = append(skipped, cand.ID)
			_ = faultStore.RecordFault(RecordFaultInput{
				UserID:       userID,
				ExchangeID:   exchangeID,
				ProxyURL:     plain,
				DisplayHost:  cand.DisplayHost,
				ErrorType:    classifyProxyHealthError(hcErr),
				ErrorMessage: "分配前健康检查失败: " + hcErr.Error(),
			})
			logger.Warnf("⚠️ 代理池：跳过不可达出口 host=%s pool_id=%s: %v", cand.DisplayHost, cand.ID, hcErr)
			continue
		}
		res := tx.Model(&OutboundProxyPool{}).
			Where("id = ? AND COALESCE(assigned_exchange_id, '') = ''", cand.ID).
			Updates(map[string]interface{}{
				"assigned_user_id":     userID,
				"assigned_exchange_id": exchangeID,
				"assigned_at":          now,
				"updated_at":           now,
			})
		if res.Error != nil {
			return "", "", res.Error
		}
		if res.RowsAffected == 1 {
			logger.Infof("📌 代理池：分配 pool_id=%s → exchange_id=%s user=%s host=%s", cand.ID, exchangeID, userID, cand.DisplayHost)
			return cand.ID, plain, nil
		}
	}
	return "", "", fmt.Errorf("代理分配冲突，请重试")
}

// ProxyBindingForExchange 该交易所若绑定了池条目，返回到期时间与展示用主机（白名单 IP/域名，与 display_host 一致）
// AssignToExchange 将一条**当前未分配**的池记录绑定到指定交易所，并把明文代理 URL 写入 exchanges.outbound_proxy_url。
// 绑定前会解除该 exchange 上已有的池占用（同 ReleaseByExchangeID），避免双占用。
func (s *ProxyPoolStore) AssignToExchange(poolID, userID, exchangeID string) error {
	poolID = strings.TrimSpace(poolID)
	userID = strings.TrimSpace(userID)
	exchangeID = strings.TrimSpace(exchangeID)
	if poolID == "" || userID == "" || exchangeID == "" {
		return fmt.Errorf("poolID、userID、exchangeID 不能为空")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.releaseByExchangeIDTx(tx, exchangeID); err != nil {
			return err
		}
		var pool OutboundProxyPool
		if err := tx.Where("id = ?", poolID).First(&pool).Error; err != nil {
			return err
		}
		if strings.TrimSpace(pool.AssignedExchangeID) != "" {
			return fmt.Errorf("该池条目已分配给其他交易所，请先对其执行 release")
		}
		plain := strings.TrimSpace(string(pool.ProxyURL))
		if plain == "" {
			return fmt.Errorf("池条目中代理 URL 为空")
		}
		if hcErr := TCPHealthCheckProxyURL(plain, 4*time.Second); hcErr != nil {
			_ = NewOutboundProxyFaultStore(s.db).RecordFault(RecordFaultInput{
				UserID:       userID,
				ExchangeID:   exchangeID,
				ProxyURL:     plain,
				DisplayHost:  pool.DisplayHost,
				ErrorType:    classifyProxyHealthError(hcErr),
				ErrorMessage: "手动分配前健康检查失败: " + hcErr.Error(),
			})
			return fmt.Errorf("代理出口不可达（%s）: %w", pool.DisplayHost, hcErr)
		}
		var ex Exchange
		if err := tx.Where("id = ? AND user_id = ?", exchangeID, userID).First(&ex).Error; err != nil {
			return fmt.Errorf("交易所不存在或不属于该用户: %w", err)
		}
		now := time.Now().UTC()
		res := tx.Model(&OutboundProxyPool{}).
			Where("id = ? AND COALESCE(assigned_exchange_id, '') = ''", poolID).
			Updates(map[string]interface{}{
				"assigned_user_id":     userID,
				"assigned_exchange_id": exchangeID,
				"assigned_at":          now,
				"updated_at":           now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return fmt.Errorf("分配失败：条目可能被并发占用，请重试")
		}
		up := map[string]interface{}{
			"outbound_proxy_url": crypto.EncryptedString(plain),
			"updated_at":         now,
		}
		if err := tx.Model(&Exchange{}).Where("id = ? AND user_id = ?", exchangeID, userID).Updates(up).Error; err != nil {
			return err
		}
		logger.Infof("📌 代理池：手动分配 pool_id=%s → exchange_id=%s user=%s host=%s", poolID, exchangeID, userID, pool.DisplayHost)
		return nil
	})
}

func (s *ProxyPoolStore) releaseByExchangeIDTx(tx *gorm.DB, exchangeID string) error {
	exchangeID = strings.TrimSpace(exchangeID)
	if exchangeID == "" {
		return nil
	}
	res := tx.Model(&OutboundProxyPool{}).
		Where("assigned_exchange_id = ?", exchangeID).
		Updates(map[string]interface{}{
			"assigned_user_id":     "",
			"assigned_exchange_id": "",
			"assigned_at":          nil,
			"updated_at":           time.Now().UTC(),
		})
	return res.Error
}

func (s *ProxyPoolStore) ProxyBindingForExchange(exchangeID string) (bound bool, expiresAt *time.Time, displayHost string, err error) {
	exchangeID = strings.TrimSpace(exchangeID)
	if exchangeID == "" {
		return false, nil, "", nil
	}
	var row OutboundProxyPool
	if err := s.db.Select("expires_at", "display_host").Where("assigned_exchange_id = ?", exchangeID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil, "", nil
		}
		return false, nil, "", err
	}
	dh := strings.TrimSpace(row.DisplayHost)
	if dh == "—" {
		dh = ""
	}
	return true, row.ExpiresAt, dh, nil
}
