package store

import (
	"fmt"

	sqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open opens (creating if needed) the SQLite database, applies schema
// auto-migrations via GORM, and returns the shared *gorm.DB handle.
func Open(path string) (*gorm.DB, error) {
	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	// The TUI owns the terminal, so GORM must not write to stdout: its default
	// logger traces every query slower than 200ms, which would corrupt the
	// screen. Errors are still surfaced through the returned error values.
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	if err := db.AutoMigrate(&Audit{}, &AuditedUrl{}, &Issue{}, &AuditSnapshot{}); err != nil {
		return nil, fmt.Errorf("auto-migrate schema: %w", err)
	}

	return db, nil
}
