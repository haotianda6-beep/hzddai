package store

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

const sqliteBusyRetryAttempts = 24

func isSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked") ||
		strings.Contains(message, "sqlite_busy") ||
		strings.Contains(message, "sqlite_locked")
}

// StoreErrorCategory keeps internal failures actionable without persisting SQL or credentials.
func StoreErrorCategory(err error) string {
	switch {
	case err == nil:
		return "none"
	case isSQLiteBusyError(err):
		return "sqlite_busy"
	case errors.Is(err, gorm.ErrRecordNotFound):
		return "not_found"
	case strings.Contains(strings.ToLower(err.Error()), "constraint"):
		return "constraint"
	default:
		return "database_error"
	}
}

func retrySQLiteBusy(db *gorm.DB, operation func() error) error {
	var err error
	for attempt := 0; attempt < sqliteBusyRetryAttempts; attempt++ {
		err = operation()
		if err == nil || db == nil || db.Dialector.Name() != "sqlite" || !isSQLiteBusyError(err) {
			return err
		}
		delay := time.Duration(1<<min(attempt, 5)) * time.Millisecond
		delay += time.Duration(time.Now().UnixNano() % int64(4*time.Millisecond))
		time.Sleep(delay)
	}
	return err
}
