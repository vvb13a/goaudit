package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vvb13a/goaudit/domain"
	"github.com/vvb13a/goaudit/store"
	"gorm.io/gorm"
)

type AuditService struct {
	db    *gorm.DB
	excel *ExcelService
	html  *HtmlService
}

func NewAuditService(db *gorm.DB, excel *ExcelService, html *HtmlService) *AuditService {
	return &AuditService{db: db, excel: excel, html: html}
}

// Create persists a freshly run audit as a new record. Every audited URL of
// the run is stored as a new row (State new) together with its issues. The
// per-URL UrlSummary values are computed here at persisting time and the
// audit summary is derived from them.
func (s *AuditService) Create(ctx context.Context, a *domain.Audit) error {
	now := time.Now().UTC()
	urlModels := make([]*store.AuditedUrl, 0, len(a.Urls))
	issueModels := make([]*store.Issue, 0)
	seen := make(map[string]struct{})

	for i := range a.Urls {
		u := a.Urls[i]
		u.CalculateSummary()
		u.State = domain.UrlStateNew
		u.CreatedAt = now
		u.LastAudited = now
		urlModels = append(urlModels, store.AuditedUrlModel(a.ID, u))
		for j := range u.Issues {
			issue := &u.Issues[j]
			url := issueURL(u)
			id := store.IssueID(a.ID, url, issue.CheckName)
			if _, dup := seen[id]; dup {
				// Same page reached through two targets: the issue row is
				// stored once and re-attached to every matching URL.
				continue
			}
			seen[id] = struct{}{}
			// First observation of the combination: no prior state, "new".
			issue.PriorSeverity = ""
			issue.Lifecycle = domain.LifecycleNew
			issueModels = append(issueModels, store.IssueModel(a.ID, url, issue, now))
		}
	}

	a.CalculateScore()

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(store.AuditModel(a)).Error; err != nil {
			return fmt.Errorf("insert audit: %w", err)
		}
		if len(urlModels) > 0 {
			if err := tx.Create(urlModels).Error; err != nil {
				return fmt.Errorf("insert audited urls: %w", err)
			}
		}
		if len(issueModels) > 0 {
			if err := tx.Create(issueModels).Error; err != nil {
				return fmt.Errorf("insert issues: %w", err)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	return s.captureSnapshot(ctx, a.ID)
}

// ReplaceRun persists a rerun of an existing audit without creating a new
// audit record. The audited URL rows survive the rerun: URLs that come back
// are updated in place and flipped to active, URLs that appear for the first
// time are inserted as new, and previously stored URLs that did not reappear
// are kept and marked missing. Their issues are replaced by the fresh
// results of the run and every issue keeps its identity (audit, url, check).
// The stored severity moves to prior_severity and the fresh severity becomes
// the current one, with the lifecycle derived from the transition. When an
// issue already carries a prior_severity and its severity did not change,
// both columns are left untouched.
func (s *AuditService) ReplaceRun(ctx context.Context, a *domain.Audit) error {
	now := time.Now().UTC()

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Capture the previous state of the audit before replacing the rows.
		var previous []store.Issue
		if err := tx.Where("audit_id = ?", a.ID).Find(&previous).Error; err != nil {
			return fmt.Errorf("load previous issues: %w", err)
		}
		oldByID := make(map[string]store.Issue, len(previous))
		for _, row := range previous {
			oldByID[row.ID] = row
		}

		var previousURLs []store.AuditedUrl
		if err := tx.Where("audit_id = ?", a.ID).Find(&previousURLs).Error; err != nil {
			return fmt.Errorf("load previous urls: %w", err)
		}
		oldByURL := make(map[string]store.AuditedUrl, len(previousURLs))
		for _, row := range previousURLs {
			oldByURL[row.URL] = row
		}

		if err := tx.Where("audit_id = ?", a.ID).Delete(&store.Issue{}).Error; err != nil {
			return fmt.Errorf("replace issues: %w", err)
		}

		present := make(map[string]struct{}, len(a.Urls))
		inserts := make([]*store.AuditedUrl, 0, len(a.Urls))
		updates := make([]*store.AuditedUrl, 0, len(a.Urls))
		issueModels := make([]*store.Issue, 0)
		seen := make(map[string]struct{})

		for i := range a.Urls {
			u := a.Urls[i]
			u.CalculateSummary()
			present[u.URL] = struct{}{}

			if old, ok := oldByURL[u.URL]; ok {
				u.State = domain.UrlStateActive
				u.CreatedAt = old.CreatedAt
				u.LastAudited = now
				updates = append(updates, store.AuditedUrlModel(a.ID, u))
			} else {
				u.State = domain.UrlStateNew
				u.CreatedAt = now
				u.LastAudited = now
				inserts = append(inserts, store.AuditedUrlModel(a.ID, u))
			}

			for j := range u.Issues {
				issue := &u.Issues[j]
				url := issueURL(u)
				id := store.IssueID(a.ID, url, issue.CheckName)
				if _, dup := seen[id]; dup {
					continue
				}
				seen[id] = struct{}{}
				if oldRow, ok := oldByID[id]; ok {
					applyIssueTransition(&oldRow, issue, now)
				} else {
					issue.PriorSeverity = ""
					issue.Lifecycle = domain.LifecycleNew
				}
				issueModels = append(issueModels, store.IssueModel(a.ID, url, issue, now))
			}
		}

		a.CalculateScore()

		if err := tx.Save(store.AuditModel(a)).Error; err != nil {
			return fmt.Errorf("update audit: %w", err)
		}

		for _, model := range updates {
			if err := tx.Save(model).Error; err != nil {
				return fmt.Errorf("update audited urls: %w", err)
			}
		}
		if len(inserts) > 0 {
			if err := tx.Create(inserts).Error; err != nil {
				return fmt.Errorf("insert audited urls: %w", err)
			}
		}
		// URLs audited by an earlier run that did not reappear stay stored
		// and are flipped to missing. Their issues were removed above; their
		// summary and timestamps keep describing the last run they were in.
		for url, old := range oldByURL {
			if _, ok := present[url]; ok {
				continue
			}
			old.State = string(domain.UrlStateMissing)
			if err := tx.Save(&old).Error; err != nil {
				return fmt.Errorf("mark missing urls: %w", err)
			}
		}
		if len(issueModels) > 0 {
			if err := tx.Create(issueModels).Error; err != nil {
				return fmt.Errorf("insert issues: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	return s.captureSnapshot(ctx, a.ID)
}

// captureSnapshot stores the dashboard state of the audit as a new snapshot
// row after a run. The snapshot is read back through GetByID so it matches
// exactly what the dashboard shows, including URL rows kept as missing from
// earlier runs.
func (s *AuditService) captureSnapshot(ctx context.Context, auditID string) error {
	a, err := s.GetByID(ctx, auditID)
	if err != nil {
		return fmt.Errorf("load audit for snapshot: %w", err)
	}
	snap := domain.NewAuditSnapshot(a, time.Now().UTC())
	if err := s.db.WithContext(ctx).Create(store.AuditSnapshotModel(&snap)).Error; err != nil {
		return fmt.Errorf("insert audit snapshot: %w", err)
	}
	return nil
}

// applyIssueTransition rotates the stored severity into prior_severity and
// stamps the lifecycle of the change. Caveat: when the previous row already
// carries a prior_severity and the fresh severity equals the stored severity,
// neither column is updated, so an earlier transition (e.g. degraded) is not
// overwritten by an unchanged rerun. The row creation timestamp is preserved.
func applyIssueTransition(previous *store.Issue, issue *domain.Issue, now time.Time) {
	oldSeverity := domain.Severity(previous.Severity)
	newSeverity := issue.Severity

	if previous.CreatedAt.IsZero() {
		issue.CreatedAt = now
	} else {
		issue.CreatedAt = previous.CreatedAt
	}

	if previous.PriorSeverity != nil && newSeverity == oldSeverity {
		issue.Severity = oldSeverity
		issue.PriorSeverity = domain.Severity(*previous.PriorSeverity)
	} else {
		issue.PriorSeverity = oldSeverity
	}

	issue.Lifecycle = domain.ComputeLifecycle(issue.PriorSeverity, issue.PriorSeverity != "", issue.Severity)
}

// issueURL returns the identity URL of an issue: the final URL of its audited
// URL when known, otherwise the audited URL (e.g. on fetch failures).
func issueURL(u *domain.AuditedUrl) string {
	if u.FinalURL != "" {
		return u.FinalURL
	}
	return u.URL
}

func (s *AuditService) GetByID(ctx context.Context, id string) (*domain.Audit, error) {
	var auditModel store.Audit
	err := s.db.WithContext(ctx).First(&auditModel, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrAuditNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get audit by id: %w", err)
	}

	audit := auditModel.ToDomain()

	var urlModels []store.AuditedUrl
	err = s.db.WithContext(ctx).Order("rowid").Where("audit_id = ?", id).Find(&urlModels).Error
	if err != nil {
		return nil, fmt.Errorf("list audited urls for audit: %w", err)
	}

	var issueModels []store.Issue
	err = s.db.WithContext(ctx).Order("rowid").Where("audit_id = ?", id).Find(&issueModels).Error
	if err != nil {
		return nil, fmt.Errorf("list issues for audit: %w", err)
	}

	audit.Urls = make([]*domain.AuditedUrl, 0, len(urlModels))
	for i := range urlModels {
		audit.Urls = append(audit.Urls, urlModels[i].ToDomain())
	}

	// Re-attach the issues of the latest run to their URLs. Rows are
	// addressed by final URL; when several targets resolve to the same page
	// (same final URL), the single stored row is attached to every matching
	// URL because the page was checked identically for each of them. URLs
	// stored as missing have no issues of the latest run and keep the
	// summary of the last run they were audited in.
	for i := range issueModels {
		issue := issueModels[i].ToDomain()
		for _, u := range audit.Urls {
			if issueURL(u) == issue.URL {
				u.Issues = append(u.Issues, *issue)
			}
		}
	}

	return audit, nil
}

// GetConfig returns the stored run configuration of an audit without loading
// its audited urls. It is used by the audit editors.
func (s *AuditService) GetConfig(ctx context.Context, id string) (*domain.Audit, error) {
	var auditModel store.Audit
	err := s.db.WithContext(ctx).First(&auditModel, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrAuditNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get audit config: %w", err)
	}
	return auditModel.ToDomain(), nil
}

// UpdateConfig updates the self-contained run configuration of an existing
// audit (its name, description, targets, check names and engine config)
// without touching the stored reports or summary. It updates through the
// store model so the JSON serializers of the slice columns apply, and selects
// every column explicitly so empty values (e.g. a cleared description) are
// persisted too. The config is canonicalized: unknown, partial or wrong
// entries fall back to the defaults.
func (s *AuditService) UpdateConfig(ctx context.Context, id, name, description string, targets, checkNames []string, config json.RawMessage) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("audit name is required")
	}
	if len(targets) == 0 {
		return fmt.Errorf("audit must contain at least one target URL")
	}
	if id == "" {
		return domain.ErrInvalidAudit
	}

	canonicalConfig, err := json.Marshal(MergeConfig(*DefaultConfig(), config))
	if err != nil {
		return fmt.Errorf("normalize audit config: %w", err)
	}

	result := s.db.WithContext(ctx).
		Model(&store.Audit{}).
		Where("id = ?", id).
		Select("name", "description", "targets", "check_names", "config").
		Updates(&store.Audit{
			ID:          id,
			Name:        name,
			Description: description,
			Targets:     targets,
			CheckNames:  checkNames,
			Config:      string(canonicalConfig),
		})
	if result.Error != nil {
		return fmt.Errorf("update audit config: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrAuditNotFound
	}
	return nil
}

func (s *AuditService) List(ctx context.Context, filter domain.AuditFilter) ([]*domain.Audit, error) {
	query := s.db.WithContext(ctx).Model(&store.Audit{}).Order("started_at DESC")

	limit := 50
	if filter.Limit > 0 {
		limit = filter.Limit
	}
	offset := 0
	if filter.Offset > 0 {
		offset = filter.Offset
	}

	var models []store.Audit
	if err := query.Limit(limit).Offset(offset).Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list audits: %w", err)
	}

	audits := make([]*domain.Audit, 0, len(models))
	for i := range models {
		audits = append(audits, models[i].ToDomain())
	}
	return audits, nil
}

func (s *AuditService) Delete(ctx context.Context, id string) error {
	// Remove the exported workbooks and reports before the database row so
	// that a failed cleanup leaves the audit fully intact instead of
	// half-deleted.
	if s.excel != nil {
		if err := s.excel.RemoveAuditFile(id); err != nil {
			return fmt.Errorf("delete audit: %w", err)
		}
	}
	if s.html != nil {
		if err := s.html.RemoveAuditFile(id); err != nil {
			return fmt.Errorf("delete audit: %w", err)
		}
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("audit_id = ?", id).Delete(&store.AuditedUrl{}).Error; err != nil {
			return err
		}
		if err := tx.Where("audit_id = ?", id).Delete(&store.Issue{}).Error; err != nil {
			return err
		}
		return tx.Delete(&store.Audit{}, "id = ?", id).Error
	})
	if err != nil {
		return fmt.Errorf("delete audit: %w", err)
	}
	return nil
}
