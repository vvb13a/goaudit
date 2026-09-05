package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/vvb13a/goaudit/domain"
)

// AuditedUrl mirrors the "audited_urls" table. It stores the state of every
// URL an audit has ever audited: rows survive reruns so a URL that stops
// appearing can be marked missing instead of vanishing. The issues of a URL
// live in the dedicated "issues" table and are re-attached on read. The
// lifecycle metadata (State, CreatedAt, LastAuditedAt) and the UrlSummary
// are computed by the persistence layer and stored on the row.
type AuditedUrl struct {
	ID              string    `gorm:"column:id;primaryKey;type:text"`
	AuditID         string    `gorm:"column:audit_id;type:text;not null;uniqueIndex:idx_audited_urls_audit_url"`
	URL             string    `gorm:"column:url;type:text;not null;uniqueIndex:idx_audited_urls_audit_url"`
	FinalURL        string    `gorm:"column:final_url;type:text;not null"`
	StatusCode      int64     `gorm:"column:status_code"`
	DurationMs      int64     `gorm:"column:duration_ms"`
	State           string    `gorm:"column:state;type:text;not null;default:'active'"`
	CreatedAt       time.Time `gorm:"column:created_at"`
	LastAuditedAt   time.Time `gorm:"column:last_audited_at"`
	SeverityCounts  string    `gorm:"column:severity_counts;type:text;not null;default:'{}'"`
	HighestSeverity string    `gorm:"column:highest_severity;type:text;not null"`
	Score           float64   `gorm:"column:score;type:real;not null;default:0"`
}

func (AuditedUrl) TableName() string { return "audited_urls" }

// AuditedUrlID derives the deterministic row id of an audited URL from its
// unique (audit, url) combination. The id is stable across reruns of the
// same audit, which lets rerun updates update rows in place.
func AuditedUrlID(auditID, url string) string {
	sum := sha256.Sum256([]byte(auditID + "\x00" + url))
	return "aur_" + hex.EncodeToString(sum[:16])
}

// AuditedUrlModel converts a domain audited URL into a row model. The
// lifecycle metadata and the summary on the domain value are expected to be
// filled in by the persistence layer already.
func AuditedUrlModel(auditID string, u *domain.AuditedUrl) *AuditedUrl {
	return &AuditedUrl{
		ID:              AuditedUrlID(auditID, u.URL),
		AuditID:         auditID,
		URL:             u.URL,
		FinalURL:        u.FinalURL,
		StatusCode:      int64(u.StatusCode),
		DurationMs:      u.Duration.Milliseconds(),
		State:           string(u.State),
		CreatedAt:       u.CreatedAt,
		LastAuditedAt:   u.LastAudited,
		SeverityCounts:  severityCountsJSON(u.Summary.SeverityCounts),
		HighestSeverity: string(u.Summary.HighestSeverity),
		Score:           u.Summary.Score,
	}
}

func (m *AuditedUrl) ToDomain() *domain.AuditedUrl {
	return &domain.AuditedUrl{
		URL:         m.URL,
		FinalURL:    m.FinalURL,
		StatusCode:  int(m.StatusCode),
		Duration:    time.Duration(m.DurationMs) * time.Millisecond,
		State:       domain.UrlState(m.State),
		CreatedAt:   m.CreatedAt,
		LastAudited: m.LastAuditedAt,
		Summary: domain.UrlSummary{
			SeverityCounts:  severityCountsFromJSON(m.SeverityCounts),
			HighestSeverity: domain.Severity(m.HighestSeverity),
			Score:           m.Score,
		},
	}
}

// severityCountsJSON encodes severity counts as the column value, using the
// canonical "{}" placeholder for a zero summary.
func severityCountsJSON(counts domain.SeverityCounts) string {
	if counts == (domain.SeverityCounts{}) {
		return "{}"
	}
	raw, err := json.Marshal(counts)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

// severityCountsFromJSON decodes the severity counts column value back into
// typed counts, returning a zero value for empty or unparsable input.
func severityCountsFromJSON(raw string) domain.SeverityCounts {
	if raw == "" || raw == "{}" {
		return domain.SeverityCounts{}
	}
	var counts domain.SeverityCounts
	if err := json.Unmarshal([]byte(raw), &counts); err != nil {
		return domain.SeverityCounts{}
	}
	return counts
}
