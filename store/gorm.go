package store

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// GormDB is the global GORM database connection
var gormDB *gorm.DB

// DB returns the GORM database connection
func DB() *gorm.DB {
	return gormDB
}

// InitGorm initializes GORM with SQLite
func InitGorm(dbPath string) (*gorm.DB, error) {
	// mattn/go-sqlite3 的连接参数会对连接池里的每个新连接生效。
	// 仅靠 db.Exec(PRAGMA ...) 只能设置碰巧取到的一个连接。
	//
	// WAL：允许并发读写，避免“交易写库 → 看板/策略市场读接口卡死”
	// synchronous=NORMAL：WAL 常用配置，减少写入持锁时间
	// busy_timeout：锁冲突时等待一会儿再失败
	// foreign_keys：开启外键约束
	dsn := fmt.Sprintf(
		"file:%s?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=10000&_foreign_keys=1",
		dbPath,
	)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
		// Use UTC for all auto-generated timestamps (autoCreateTime, autoUpdateTime)
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}

	// Set connection pool for SQLite
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	// 关键：不要把 SQLite 限死为 1 个连接。
	// 否则交易循环写库占着连接时，所有读接口都会“排队等连接”，前端就会表现为“网络错误/一直加载”。
	sqlDB.SetMaxOpenConns(5)
	sqlDB.SetMaxIdleConns(5)

	// 兜底再执行一次（对当前连接生效；全连接一致性由 DSN 参数保证）
	db.Exec("PRAGMA foreign_keys = ON")
	db.Exec("PRAGMA journal_mode = WAL")
	db.Exec("PRAGMA synchronous = NORMAL")
	db.Exec("PRAGMA busy_timeout = 10000")

	gormDB = db
	return db, nil
}

// InitGormPostgres initializes GORM with PostgreSQL
func InitGormPostgres(host string, port int, user, password, dbname, sslmode string) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
		// Use UTC for all auto-generated timestamps (autoCreateTime, autoUpdateTime)
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open PostgreSQL database: %w", err)
	}

	// Set connection pool for PostgreSQL
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)

	gormDB = db
	return db, nil
}

// InitGormWithConfig initializes GORM with provided configuration
// Uses DBConfig from driver.go
func InitGormWithConfig(cfg DBConfig) (*gorm.DB, error) {
	switch cfg.Type {
	case DBTypeSQLite:
		return InitGorm(cfg.Path)

	case DBTypePostgres:
		return InitGormPostgres(
			cfg.Host,
			cfg.Port,
			cfg.User,
			cfg.Password,
			cfg.DBName,
			cfg.SSLMode,
		)

	default:
		return nil, fmt.Errorf("unsupported DB_TYPE: %s (use 'sqlite' or 'postgres')", cfg.Type)
	}
}

// ============================================================================
// Query Scopes - Reusable query helpers
// ============================================================================

// ForUser returns a scope that filters by user_id
func ForUser(userID string) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("user_id = ?", userID)
	}
}

// ForTrader returns a scope that filters by trader_id
func ForTrader(traderID string) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("trader_id = ?", traderID)
	}
}

// OpenPositions returns a scope for open positions
func OpenPositions() func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("status = ?", "OPEN")
	}
}

// ClosedPositions returns a scope for closed positions
func ClosedPositions() func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("status = ?", "CLOSED")
	}
}

// OrderByCreatedDesc returns a scope that orders by created_at DESC
func OrderByCreatedDesc() func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Order("created_at DESC")
	}
}

// OrderByUpdatedDesc returns a scope that orders by updated_at DESC
func OrderByUpdatedDesc() func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Order("updated_at DESC")
	}
}

// Paginate returns a scope for pagination
func Paginate(limit, offset int) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Limit(limit).Offset(offset)
	}
}
