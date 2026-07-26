package store

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
)

// OutboundProxyFaultEvent 出口代理访问失败（管理端告警，同 trader+代理 5 分钟内合并）
type OutboundProxyFaultEvent struct {
	ID            int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID        string    `gorm:"column:user_id;index" json:"user_id"`
	TraderID      string    `gorm:"column:trader_id;index" json:"trader_id"`
	ExchangeID    string    `gorm:"column:exchange_id;index" json:"exchange_id"`
	ProxyRedacted string    `gorm:"column:proxy_redacted;not null;default:'';index" json:"proxy_redacted"`
	DisplayHost   string    `gorm:"column:display_host;not null;default:''" json:"display_host"`
	ErrorType     string    `gorm:"column:error_type;not null;default:''" json:"error_type"`
	LastError     string    `gorm:"column:last_error;not null;default:''" json:"last_error"`
	HitCount      int       `gorm:"column:hit_count;not null;default:1" json:"hit_count"`
	FirstAt       time.Time `gorm:"column:first_at;index" json:"first_at"`
	LastAt        time.Time `gorm:"column:last_at;index" json:"last_at"`
}

func (OutboundProxyFaultEvent) TableName() string { return "outbound_proxy_fault_events" }

type OutboundProxyFaultStore struct {
	db *gorm.DB
}

func NewOutboundProxyFaultStore(db *gorm.DB) *OutboundProxyFaultStore {
	return &OutboundProxyFaultStore{db: db}
}

func (s *OutboundProxyFaultStore) initTables() error {
	return s.db.AutoMigrate(&OutboundProxyFaultEvent{})
}

const outboundProxyFaultMergeWindow = 5 * time.Minute

// RecordFaultInput 记录一次经代理的 REST 失败（仅当 ProxyURL 非空时调用）
type RecordFaultInput struct {
	UserID        string
	TraderID      string
	ExchangeID    string
	ProxyURL      string
	ProxyRedacted string
	DisplayHost   string
	ErrorType     string
	ErrorMessage  string
}

// RecordFault 写入或合并近期同类事件
func (s *OutboundProxyFaultStore) RecordFault(in RecordFaultInput) error {
	proxyKey := strings.TrimSpace(in.ProxyRedacted)
	if proxyKey == "" {
		proxyKey = RedactProxyURL(in.ProxyURL)
	}
	if proxyKey == "" || proxyKey == "direct" {
		return nil
	}
	traderID := strings.TrimSpace(in.TraderID)
	userID := strings.TrimSpace(in.UserID)
	exchangeID := strings.TrimSpace(in.ExchangeID)
	errType := strings.TrimSpace(in.ErrorType)
	if errType == "" {
		errType = "other"
	}
	msg := truncateRunes(strings.TrimSpace(in.ErrorMessage), 480)
	displayHost := strings.TrimSpace(in.DisplayHost)
	if displayHost == "" && exchangeID != "" {
		var pool OutboundProxyPool
		if s.db.Where("assigned_exchange_id = ?", exchangeID).First(&pool).Error == nil {
			displayHost = strings.TrimSpace(pool.DisplayHost)
		}
	}
	now := time.Now().UTC()
	cutoff := now.Add(-outboundProxyFaultMergeWindow)

	var existing OutboundProxyFaultEvent
	q := s.db.Where("proxy_redacted = ? AND last_at >= ?", proxyKey, cutoff)
	if traderID != "" {
		q = q.Where("trader_id = ?", traderID)
	} else if userID != "" {
		q = q.Where("user_id = ? AND (trader_id = '' OR trader_id IS NULL)", userID)
	}
	if err := q.Order("last_at DESC").First(&existing).Error; err == nil {
		return s.db.Model(&existing).Updates(map[string]interface{}{
			"hit_count":    gorm.Expr("hit_count + 1"),
			"last_at":      now,
			"error_type":   errType,
			"last_error":   msg,
			"display_host": displayHost,
			"exchange_id":  exchangeID,
		}).Error
	}

	row := OutboundProxyFaultEvent{
		UserID:        userID,
		TraderID:      traderID,
		ExchangeID:    exchangeID,
		ProxyRedacted: proxyKey,
		DisplayHost:   displayHost,
		ErrorType:     errType,
		LastError:     msg,
		HitCount:      1,
		FirstAt:       now,
		LastAt:        now,
	}
	return s.db.Create(&row).Error
}

// AdminOutboundProxyFaultRow 管理端列表行
type AdminOutboundProxyFaultRow struct {
	OutboundProxyFaultEvent
	UserEmail       string `json:"user_email"`
	UserDisplayName string `json:"user_display_name"`
	TraderName      string `json:"trader_name"`
}

// ListAdminRecent 最近代理故障（默认 50 条）
func (s *OutboundProxyFaultStore) ListAdminRecent(limit int) ([]AdminOutboundProxyFaultRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var events []OutboundProxyFaultEvent
	if err := s.db.Order("last_at DESC").Limit(limit).Find(&events).Error; err != nil {
		return nil, err
	}
	out := make([]AdminOutboundProxyFaultRow, 0, len(events))
	for _, ev := range events {
		row := AdminOutboundProxyFaultRow{OutboundProxyFaultEvent: ev}
		if ev.UserID != "" {
			if u, err := NewUserStore(s.db).GetByID(ev.UserID); err == nil && u != nil {
				row.UserEmail = u.Email
				row.UserDisplayName = u.DisplayName
			}
		}
		if ev.TraderID != "" {
			if t, err := NewTraderStore(s.db).GetByID(ev.TraderID); err == nil && t != nil {
				row.TraderName = t.Name
			}
		}
		out = append(out, row)
	}
	return out, nil
}

// RedactProxyURL 脱敏代理 URL（保留 scheme 与 host:port）
func RedactProxyURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		if len(raw) <= 32 {
			return raw
		}
		return raw[:12] + "…" + raw[len(raw)-8:]
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		switch strings.ToLower(u.Scheme) {
		case "https":
			port = "443"
		case "http":
			port = "80"
		default:
			port = "1080"
		}
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "" {
		scheme = "socks5"
	}
	return fmt.Sprintf("%s://***@%s:%s", scheme, host, port)
}

func truncateRunes(s string, max int) string {
	rs := []rune(s)
	if len(rs) <= max {
		return s
	}
	return string(rs[:max]) + "…"
}
