package store

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// UserStore user storage
type UserStore struct {
	db *gorm.DB
}

// User user model
type User struct {
	ID              string  `gorm:"primaryKey" json:"id"`
	Email           string  `gorm:"uniqueIndex:idx_users_email;not null" json:"email"`
	PasswordHash    string  `gorm:"column:password_hash;not null" json:"-"`
	DisplayName     string  `gorm:"column:display_name;not null;default:''" json:"display_name"`
	AvatarURL       string  `gorm:"column:avatar_url;not null;default:''" json:"avatar_url"`
	BalanceUSDT     float64 `gorm:"column:balance_usdt;default:0" json:"balance_usdt"`
	InviteCode      string  `gorm:"column:invite_code;not null;default:'';index" json:"invite_code"`
	InvitedByUserID string  `gorm:"column:invited_by_user_id;not null;default:'';index" json:"invited_by_user_id"`
	ProfileNamed    bool    `gorm:"column:profile_named;not null;default:false" json:"profile_named"`
	// MarketWeeklyTrialUsed 为 true 表示该账号已使用过「策略市场周卡体验」资格（全站仅一次，与具体策略无关）
	MarketWeeklyTrialUsed bool      `gorm:"column:market_weekly_trial_used;not null;default:false" json:"market_weekly_trial_used"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

func (User) TableName() string { return "users" }

// NewUserStore creates a new UserStore
func NewUserStore(db *gorm.DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) initTables() error {
	// For PostgreSQL with existing table, skip AutoMigrate to avoid index conflicts
	if s.db.Dialector.Name() == "postgres" {
		var tableExists int64
		s.db.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'users'`).Scan(&tableExists)

		if tableExists > 0 {
			// Table exists - manually ensure all columns exist
			// Core columns (should already exist)
			s.db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS email TEXT NOT NULL DEFAULT ''`)
			s.db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash TEXT NOT NULL DEFAULT ''`)
			s.db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP`)
			s.db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP`)
			s.db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS display_name TEXT NOT NULL DEFAULT ''`)
			s.db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_url TEXT NOT NULL DEFAULT ''`)
			s.db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS balance_usdt DOUBLE PRECISION NOT NULL DEFAULT 0`)
			s.db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS invite_code TEXT NOT NULL DEFAULT ''`)
			s.db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS invited_by_user_id TEXT NOT NULL DEFAULT ''`)
			s.db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS profile_named BOOLEAN NOT NULL DEFAULT FALSE`)
			s.db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS market_weekly_trial_used BOOLEAN NOT NULL DEFAULT FALSE`)
			s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_invite_code ON users(invite_code) WHERE invite_code <> ''`)

			// Ensure unique index exists on email (don't care about the name)
			var indexExists int64
			s.db.Raw(`
				SELECT COUNT(*) FROM pg_indexes
				WHERE tablename = 'users' AND indexdef LIKE '%email%' AND indexdef LIKE '%UNIQUE%'
			`).Scan(&indexExists)

			if indexExists == 0 {
				s.db.Exec("CREATE UNIQUE INDEX idx_users_email ON users(email)")
			}

			return nil
		}
	}
	return s.db.AutoMigrate(&User{})
}

// Create creates user
func (s *UserStore) Create(user *User) error {
	if strings.TrimSpace(user.InviteCode) == "" {
		user.InviteCode = s.GenerateInviteCode()
	}
	return s.db.Create(user).Error
}

func (s *UserStore) GenerateInviteCode() string {
	for i := 0; i < 20; i++ {
		code := RandomInviteCode()
		var n int64
		if err := s.db.Model(&User{}).Where("invite_code = ?", code).Count(&n).Error; err == nil && n == 0 {
			return code
		}
	}
	return strings.ToUpper(strings.ReplaceAll(fmt.Sprintf("%08x", time.Now().UnixNano()), "-", ""))[:8]
}

func (s *UserStore) GetByInviteCode(code string) (*User, error) {
	code = NormalizeInviteCode(code)
	var user User
	err := s.db.Where("invite_code = ?", code).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByEmail gets user by email
func (s *UserStore) GetByEmail(email string) (*User, error) {
	var user User
	err := s.db.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByID gets user by ID
func (s *UserStore) GetByID(userID string) (*User, error) {
	var user User
	err := s.db.Where("id = ?", userID).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetMapByIDs returns users keyed by id (missing ids are simply omitted).
func (s *UserStore) GetMapByIDs(ids []string) (map[string]User, error) {
	if len(ids) == 0 {
		return map[string]User{}, nil
	}
	var users []User
	if err := s.db.Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	out := make(map[string]User, len(users))
	for _, u := range users {
		out[u.ID] = u
	}
	return out, nil
}

// EnsureProfileDefaults 老用户登录时补齐随机昵称与默认头像
func (s *UserStore) EnsureProfileDefaults(userID string) (*User, error) {
	u, err := s.GetByID(userID)
	if err != nil {
		return nil, err
	}
	dn := strings.TrimSpace(u.DisplayName)
	av := strings.TrimSpace(u.AvatarURL)
	ic := strings.TrimSpace(u.InviteCode)
	if dn != "" && av != "" && ic != "" {
		return u, nil
	}
	updates := map[string]interface{}{"updated_at": time.Now().UTC()}
	if av == "" {
		updates["avatar_url"] = DefaultAvatarURL(userID)
	}
	if strings.TrimSpace(u.InviteCode) == "" {
		updates["invite_code"] = s.GenerateInviteCode()
	}
	if err := s.db.Model(&User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetByID(userID)
}

// UpdatePublicProfile 更新展示昵称（头像仍由系统种子 URL 决定，避免任意 URL 风险）
func (s *UserStore) UpdatePublicProfile(userID, displayName string) error {
	return s.db.Model(&User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"display_name":  displayName,
		"profile_named": true,
		"updated_at":    time.Now().UTC(),
	}).Error
}

// Count returns the total number of users
func (s *UserStore) Count() (int, error) {
	var count int64
	err := s.db.Model(&User{}).Count(&count).Error
	return int(count), err
}

// GetAllIDs gets all user IDs
func (s *UserStore) GetAllIDs() ([]string, error) {
	var userIDs []string
	err := s.db.Model(&User{}).Order("id").Pluck("id", &userIDs).Error
	return userIDs, err
}

// GetAll returns all users ordered by creation time.
func (s *UserStore) GetAll() ([]User, error) {
	var users []User
	err := s.db.Model(&User{}).Order("created_at").Find(&users).Error
	return users, err
}

func (s *UserStore) ListInvitedUsers(inviterID string) ([]User, error) {
	var users []User
	err := s.db.Where("invited_by_user_id = ?", inviterID).Order("created_at DESC").Find(&users).Error
	return users, err
}

// ListInviteDescendants 返回邀请树下全部下级用户（不含根），任意深度；基于 invited_by_user_id 递归。
func (s *UserStore) ListInviteDescendants(rootUserID string) ([]User, error) {
	rootUserID = strings.TrimSpace(rootUserID)
	if rootUserID == "" {
		return []User{}, nil
	}
	var users []User
	switch s.db.Dialector.Name() {
	case "sqlite":
		err := s.db.Raw(`
WITH RECURSIVE sub AS (
  SELECT * FROM users WHERE invited_by_user_id = ?
  UNION ALL
  SELECT u.* FROM users u INNER JOIN sub ON u.invited_by_user_id = sub.id
)
SELECT * FROM sub ORDER BY created_at ASC`, rootUserID).Scan(&users).Error
		return users, err
	case "postgres":
		err := s.db.Raw(`
WITH RECURSIVE sub AS (
  SELECT * FROM users WHERE invited_by_user_id = $1
  UNION ALL
  SELECT u.* FROM users u INNER JOIN sub ON u.invited_by_user_id = sub.id
)
SELECT * FROM sub ORDER BY created_at ASC`, rootUserID).Scan(&users).Error
		return users, err
	default:
		return nil, fmt.Errorf("ListInviteDescendants: unsupported dialect %s", s.db.Dialector.Name())
	}
}

// CountDirectInvitesByInviterIDs 统计每位邀请人的直接下级人数（invited_by_user_id = inviter）。
func (s *UserStore) CountDirectInvitesByInviterIDs(inviterIDs []string) (map[string]int64, error) {
	out := make(map[string]int64)
	clean := make([]string, 0, len(inviterIDs))
	seen := map[string]bool{}
	for _, id := range inviterIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		clean = append(clean, id)
	}
	if len(clean) == 0 {
		return out, nil
	}
	var rows []struct {
		InviterID string `gorm:"column:invited_by_user_id"`
		N         int64
	}
	err := s.db.Model(&User{}).
		Select("invited_by_user_id, COUNT(*) AS n").
		Where("invited_by_user_id IN ?", clean).
		Group("invited_by_user_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.InviterID] = row.N
	}
	return out, nil
}

// BackfillMarketWeeklyTrialUsedFromLedger 将历史上买过周卡的用户标记为「已用体验」，与现网 wallet_ledgers 对齐（可重复执行）。
func (s *UserStore) BackfillMarketWeeklyTrialUsedFromLedger() error {
	switch s.db.Dialector.Name() {
	case "sqlite":
		return s.db.Exec(`
UPDATE users SET market_weekly_trial_used = 1, updated_at = ?
WHERE id IN (
  SELECT DISTINCT user_id FROM wallet_ledgers
  WHERE reason IN ('market_subscription_weekly','market_subscription_weekly_trial')
)`, time.Now().UTC()).Error
	case "postgres":
		return s.db.Exec(`
UPDATE users SET market_weekly_trial_used = TRUE, updated_at = NOW()
WHERE id IN (
  SELECT DISTINCT user_id FROM wallet_ledgers
  WHERE reason IN ('market_subscription_weekly','market_subscription_weekly_trial')
)`).Error
	default:
		return fmt.Errorf("BackfillMarketWeeklyTrialUsedFromLedger: unsupported dialect %s", s.db.Dialector.Name())
	}
}

// TryClaimMarketWeeklyTrial 在事务内原子抢占「周卡体验」资格：成功则 ok=true；已用过则 ok=false。
func (s *UserStore) TryClaimMarketWeeklyTrial(tx *gorm.DB, userID string) (ok bool, err error) {
	db := tx
	if db == nil {
		db = s.db
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false, fmt.Errorf("empty user id")
	}
	now := time.Now().UTC()
	switch db.Dialector.Name() {
	case "sqlite":
		r := db.Exec(`
UPDATE users SET market_weekly_trial_used = 1, updated_at = ?
WHERE id = ? AND COALESCE(market_weekly_trial_used, 0) = 0`, now, userID)
		return r.RowsAffected > 0, r.Error
	case "postgres":
		r := db.Exec(`
UPDATE users SET market_weekly_trial_used = TRUE, updated_at = ?
WHERE id = ? AND (market_weekly_trial_used IS NULL OR market_weekly_trial_used = FALSE)`, now, userID)
		return r.RowsAffected > 0, r.Error
	default:
		res := db.Model(&User{}).
			Where("id = ? AND market_weekly_trial_used = ?", userID, false).
			Updates(map[string]interface{}{
				"market_weekly_trial_used": true,
				"updated_at":               now,
			})
		return res.RowsAffected > 0, res.Error
	}
}

// UpdatePassword updates password
func (s *UserStore) UpdatePassword(userID, passwordHash string) error {
	return s.db.Model(&User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"password_hash": passwordHash,
		"updated_at":    time.Now().UTC(),
	}).Error
}

// DeleteAll deletes all users (reset system to uninitialized state)
func (s *UserStore) DeleteAll() error {
	return s.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&User{}).Error
}

// AddBalanceDelta 原子增减余额（用于充值、策略购买、后台调账）。delta 为负时表示扣款。
// tx 可为 nil，则使用默认 db。成功返回新余额；余额不足时 ok=false 且不报错（由调用方判断）。
func (s *UserStore) AddBalanceDelta(tx *gorm.DB, userID string, delta float64) (newBalance float64, ok bool, err error) {
	db := tx
	if db == nil {
		db = s.db
	}
	res := db.Model(&User{}).
		Where("id = ? AND COALESCE(balance_usdt,0) + ? >= ?", userID, delta, -1e-9).
		Updates(map[string]interface{}{
			"balance_usdt": gorm.Expr("COALESCE(balance_usdt,0) + ?", delta),
			"updated_at":   time.Now().UTC(),
		})
	if res.Error != nil {
		return 0, false, res.Error
	}
	if res.RowsAffected == 0 {
		var u User
		if e := db.Where("id = ?", userID).First(&u).Error; e != nil {
			return 0, false, e
		}
		return u.BalanceUSDT, false, nil
	}
	var u User
	if err := db.Where("id = ?", userID).First(&u).Error; err != nil {
		return 0, false, err
	}
	return u.BalanceUSDT, true, nil
}

// AddBalanceDeltaAllowNegative 原子增减余额，允许扣成负数。
// 只用于已经发生的按次计费补扣/欠费记账；购买、普通调账仍应使用 AddBalanceDelta。
func (s *UserStore) AddBalanceDeltaAllowNegative(tx *gorm.DB, userID string, delta float64) (newBalance float64, err error) {
	db := tx
	if db == nil {
		db = s.db
	}
	res := db.Model(&User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"balance_usdt": gorm.Expr("COALESCE(balance_usdt,0) + ?", delta),
			"updated_at":   time.Now().UTC(),
		})
	if res.Error != nil {
		return 0, res.Error
	}
	if res.RowsAffected == 0 {
		return 0, gorm.ErrRecordNotFound
	}
	var u User
	if err := db.Where("id = ?", userID).First(&u).Error; err != nil {
		return 0, err
	}
	return u.BalanceUSDT, nil
}

// SetBalanceAbsolute 将余额设为绝对值（仅后台管理使用）
func (s *UserStore) SetBalanceAbsolute(tx *gorm.DB, userID string, absolute float64) error {
	db := tx
	if db == nil {
		db = s.db
	}
	if absolute < 0 {
		return fmt.Errorf("balance cannot be negative")
	}
	return db.Model(&User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"balance_usdt": absolute,
		"updated_at":   time.Now().UTC(),
	}).Error
}

// EnsureAdmin ensures admin user exists
func (s *UserStore) EnsureAdmin() error {
	var count int64
	s.db.Model(&User{}).Where("id = ?", "admin").Count(&count)
	if count > 0 {
		return nil
	}
	return s.Create(&User{
		ID:           "admin",
		Email:        "admin@localhost",
		PasswordHash: "",
	})
}
