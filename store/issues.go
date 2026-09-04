package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/vvb13a/goaudit/domain"
)

// Issue mirrors the "issues" table. Checks emit a single issue per
// (audit, url, check) combination, so that tuple is unique within an audit
// and every issue row is addressed by a deterministic id derived from it.
type Issue struct {
	ID            string    `gorm:"column:id;primaryKey;type:text"`
	AuditID       string    `gorm:"column:audit_id;type:text;not null;index:idx_issues_audit_id;uniqueIndex:idx_issues_audit_url_check"`
	URL           string    `gorm:"column:url;type:text;not null;uniqueIndex:idx_issues_audit_url_check"`
	CheckName     string    `gorm:"column:check_name;type:text;not null;uniqueIndex:idx_issues_audit_url_check"`
	Category      string    `gorm:"column:category;type:text;not null"`
	Severity      string    `gorm:"column:severity;type:text;not null"`
	Message       string    `gorm:"column:message;type:text;not null"`
	Evidence      string    `gorm:"column:evidence;type:text;not null;default:'{}'"`
	PriorSeverity *string   `gorm:"column:prior_severity;type:text"`
	Lifecycle     string    `gorm:"column:lifecycle;type:text;not null;default:''"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (Issue) TableName() string { return "issues" }

// IssueID derives the deterministic row id for one issue from its unique
// (audit, url, check) combination. The id is stable across reruns of the same
// audit, which lets rerun updates upsert issues in place.
func IssueID(auditID, url, checkName string) string {
	sum := sha256.Sum256([]byte(auditID + "\x00" + url + "\x00" + checkName))
	return "iss_" + hex.EncodeToString(sum[:16])
}

// IssueModel converts a check result into a row for the given audit and url.
// The row metadata (PriorSeverity, Lifecycle, timestamps) is read from the
// domain issue, where the persistence layer stamps it before mapping; at is
// the run timestamp used for new rows and updated_at.
func IssueModel(auditID, url string, iss *domain.Issue, at time.Time) *Issue {
	created := iss.CreatedAt
	if created.IsZero() {
		created = at
	}
	return &Issue{
		ID:            IssueID(auditID, url, iss.CheckName),
		AuditID:       auditID,
		URL:           url,
		CheckName:     iss.CheckName,
		Category:      string(iss.Category),
		Severity:      string(iss.Severity),
		Message:       iss.Message,
		Evidence:      detailsJSON(iss.Details),
		PriorSeverity: optionalSeverity(iss.PriorSeverity),
		Lifecycle:     string(iss.Lifecycle),
		CreatedAt:     created,
		UpdatedAt:     at,
	}
}

// optionalSeverity converts a severity into a nullable column value.
func optionalSeverity(s domain.Severity) *string {
	if s == "" {
		return nil
	}
	v := string(s)
	return &v
}

func (m *Issue) ToDomain() *domain.Issue {
	issue := &domain.Issue{
		ID:        m.ID,
		URL:       m.URL,
		CheckName: m.CheckName,
		Category:  domain.Category(m.Category),
		Severity:  domain.Severity(m.Severity),
		Passed:    !domain.Severity(m.Severity).IsFailure(),
		Message:   m.Message,
		Lifecycle: domain.IssueLifecycle(m.Lifecycle),
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
	if m.PriorSeverity != nil {
		issue.PriorSeverity = domain.Severity(*m.PriorSeverity)
	}
	if m.Evidence != "" && m.Evidence != "{}" {
		var details map[string]any
		if err := json.Unmarshal([]byte(m.Evidence), &details); err == nil && len(details) > 0 {
			issue.Details = details
		}
	}
	return issue
}

// detailsJSON encodes issue details as the evidence column value, using the
// canonical "{}" placeholder when there is no evidence.
func detailsJSON(details map[string]any) string {
	if len(details) == 0 {
		return "{}"
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return "{}"
	}
	return string(raw)
}
