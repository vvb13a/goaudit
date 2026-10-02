package store

import "time"

// LinkTarget is the global validation state of one link target URL. It is
// keyed by the normalized URL, not the audit, so a target referenced by
// several audits or runs is fetched once and reused until its result expires.
type LinkTarget struct {
	URL         string    `gorm:"column:url;primaryKey;type:text"`
	StatusCode  int       `gorm:"column:status_code"`
	Error       string    `gorm:"column:error;type:text;not null;default:''"`
	FinalURL    string    `gorm:"column:final_url;type:text;not null;default:''"`
	ValidatedAt time.Time `gorm:"column:validated_at"`
	ExpiresAt   time.Time `gorm:"column:expires_at;index:idx_link_targets_expires_at"`
}

func (LinkTarget) TableName() string { return "link_targets" }
