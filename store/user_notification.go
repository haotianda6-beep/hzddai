package store

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// UserNotification 站内系统通知（充值成功、管理员入账等）
// Broadcast=true 时为全站公告：List 时对任意登录用户可见；普通行仅 user_id 本人可见。
type UserNotification struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    string    `gorm:"column:user_id;index:idx_user_notifications_user;not null;default:''" json:"user_id"`
	Broadcast bool      `gorm:"column:broadcast;not null;default:false;index" json:"broadcast"`
	Title     string    `gorm:"column:title;not null" json:"title"`
	Body      string    `gorm:"column:body;type:text;not null" json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

func (UserNotification) TableName() string { return "user_notifications" }

// NotificationStore 用户通知
type NotificationStore struct {
	db *gorm.DB
}

func NewNotificationStore(db *gorm.DB) *NotificationStore {
	return &NotificationStore{db: db}
}

func (s *NotificationStore) initTables() error {
	return s.db.AutoMigrate(&UserNotification{})
}

func (s *NotificationStore) Add(userID, title, body string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return fmt.Errorf("user_id required for personal notification")
	}
	row := &UserNotification{
		UserID:    userID,
		Broadcast: false,
		Title:     title,
		Body:      body,
	}
	return s.db.Create(row).Error
}

// AddBroadcast 全站公告：所有登录用户拉取通知列表时都会看到
func (s *NotificationStore) AddBroadcast(title, body string) error {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if title == "" || body == "" {
		return fmt.Errorf("title and body required")
	}
	row := &UserNotification{
		UserID:    "",
		Broadcast: true,
		Title:     title,
		Body:      body,
	}
	return s.db.Create(row).Error
}

func (s *NotificationStore) List(userID string, limit int) ([]UserNotification, error) {
	if limit <= 0 || limit > 100 {
		limit = 40
	}
	userID = strings.TrimSpace(userID)
	var rows []UserNotification
	// 本人私信 OR 全站公告
	err := s.db.Where("broadcast = ? OR user_id = ?", true, userID).
		Order("id DESC").Limit(limit).Find(&rows).Error
	return rows, err
}
