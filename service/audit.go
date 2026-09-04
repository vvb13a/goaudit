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

func (s *AuditService) Create(ctx context.Context, a *domain.Audit) error {
	reportModels := make([]*store.Report, 0, len(a.Reports))
	issueModels := make([]*store.Issue, 0)
	now := time.Now().UTC()
	seen := make(map[string]struct{})

	for i, r := range a.Reports {
		reportModels = append(reportModels, store.ReportModel(a.ID, fmt.Sprintf("%s_r_%d", a.ID, i+1), r))
		for j := range r.Issues {
			issue := &r.Issues[j]
			url := issueURL(r)
			id := store.IssueID(a.ID, url, issue.CheckName)
			if _, dup := seen[id]; dup {
				// Same page reached through two targets: the issue row is
				// stored once and re-attached to every matching report.
				continue
			}
			seen[id] = struct{}{}
			issueModels = append(issueModels, store.IssueModel(a.ID, url, issue, nil, now))
		}
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(store.AuditModel(a)).Error; err != nil {
			return fmt.Errorf("insert audit: %w", err)
		}
		if len(reportModels) > 0 {
			if err := tx.Create(reportModels).Error; err != nil {
				return fmt.Errorf("insert reports: %w", err)
			}
		}
		if len(issueModels) > 0 {
			if err := tx.Create(issueModels).Error; err != nil {
				return fmt.Errorf("insert issues: %w", err)
			}
		}
		return nil
	})
}

// issueURL returns the identity URL of an issue: the final URL of its report
// when known, otherwise the audited URL (e.g. on fetch failures).
func issueURL(r *domain.Report) string {
	if r.FinalURL != "" {
		return r.FinalURL
	}
	return r.URL
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

	var reportModels []store.Report
	err = s.db.WithContext(ctx).Order("rowid").Where("audit_id = ?", id).Find(&reportModels).Error
	if err != nil {
		return nil, fmt.Errorf("list reports for audit: %w", err)
	}

	var issueModels []store.Issue
	err = s.db.WithContext(ctx).Order("rowid").Where("audit_id = ?", id).Find(&issueModels).Error
	if err != nil {
		return nil, fmt.Errorf("list issues for audit: %w", err)
	}

	audit.Reports = make([]*domain.Report, 0, len(reportModels))
	for i := range reportModels {
		audit.Reports = append(audit.Reports, reportModels[i].ToDomain())
	}

	// Re-attach issues to their reports and recompute the per-URL summaries
	// from the stored rows. Rows are addressed by final URL; when several
	// targets resolve to the same page (same final URL), the single stored
	// row is attached to every matching report because the page was checked
	// identically for each of them.
	for i := range issueModels {
		issue := issueModels[i].ToDomain()
		for _, rep := range audit.Reports {
			if issueURL(rep) == issue.URL {
				rep.Issues = append(rep.Issues, *issue)
			}
		}
	}
	for _, rep := range audit.Reports {
		rep.CalculateSummary()
	}

	return audit, nil
}

// GetConfig returns the stored run configuration of an audit without loading
// its reports. It is used by the audit editors.
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

	if filter.HighestSeverity != nil && filter.HighestSeverity.IsValid() {
		query = query.Where("highest_severity = ?", string(*filter.HighestSeverity))
	}

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
		if err := tx.Where("audit_id = ?", id).Delete(&store.Report{}).Error; err != nil {
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
