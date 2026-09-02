package store

import (
	"fmt"

	sqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// Open opens (creating if needed) the SQLite database, applies schema
// auto-migrations via GORM, and returns the shared *gorm.DB handle.
func Open(path string) (*gorm.DB, error) {
	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	if err := db.AutoMigrate(&Plan{}, &Checklist{}, &Audit{}, &Report{}); err != nil {
		return nil, fmt.Errorf("auto-migrate schema: %w", err)
	}

	return db, nil
}
