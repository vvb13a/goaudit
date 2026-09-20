package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
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

// createBatchRows bounds how many rows one INSERT statement carries. SQLite
// rejects statements whose bound variables exceed its compiled limit, which
// varies between builds; 80 rows stay below even the most restrictive ones.
const createBatchRows = 80

// createInBatches inserts slice rows in chunks so a single statement never
// exceeds the SQLite variable limit.
func createInBatches(tx *gorm.DB, rows any) error {
	v := reflect.ValueOf(rows)
	if v.Kind() != reflect.Slice || v.Len() == 0 {
		return nil
	}
	for start := 0; start < v.Len(); start += createBatchRows {
		end := start + createBatchRows
		if end > v.Len() {
			end = v.Len()
		}
		if err := tx.Create(v.Slice(start, end).Interface()).Error; err != nil {
			return err
		}
	}
	return nil
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
		if err := createInBatches(tx, urlModels); err != nil {
			return fmt.Errorf("insert audited urls: %w", err)
		}
		if err := createInBatches(tx, issueModels); err != nil {
			return fmt.Errorf("insert issues: %w", err)
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
		if err := createInBatches(tx, inserts); err != nil {
			return fmt.Errorf("insert audited urls: %w", err)
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
		if err := createInBatches(tx, issueModels); err != nil {
			return fmt.Errorf("insert issues: %w", err)
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

// ApplyURLRecheck persists a re-audit of a single audited URL of an audit
// without touching the audit's run history: no snapshot row is recorded and
// the audit record keeps its run metadata (started at, duration). Only the
// URL row, its issues and the derived audit score change — the URL rows and
// issues of every other URL stay untouched. The fresh result is the raw
// engine output for that URL; its lifecycle metadata and summary are derived
// here, using the previous issue rows of the URL for transitions.
func (s *AuditService) ApplyURLRecheck(ctx context.Context, auditID string, fresh *domain.AuditedUrl) error {
	if auditID == "" || fresh == nil || fresh.URL == "" {
		return domain.ErrInvalidAudit
	}
	now := time.Now().UTC()

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row store.AuditedUrl
		err := tx.Where("audit_id = ? AND url = ?", auditID, fresh.URL).First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("url %q is not part of audit %s", fresh.URL, auditID)
		}
		if err != nil {
			return fmt.Errorf("load audited url: %w", err)
		}

		oldIdentity := row.FinalURL
		if oldIdentity == "" {
			oldIdentity = row.URL
		}
		newIdentity := fresh.FinalURL
		if newIdentity == "" {
			newIdentity = fresh.URL
		}

		// The issues of the previous result of this URL act as the prior
		// state for lifecycle transitions.
		var previous []store.Issue
		if err := tx.Where("audit_id = ? AND url = ?", auditID, oldIdentity).Find(&previous).Error; err != nil {
			return fmt.Errorf("load previous issues: %w", err)
		}
		oldByID := make(map[string]store.Issue, len(previous))
		for _, iss := range previous {
			oldByID[iss.ID] = iss
		}

		// Replace the stored issues of the URL. When the final URL changed,
		// rows shared with other URLs keep describing the shared page and
		// are left in place; orphaned ones are removed.
		if oldIdentity == newIdentity {
			if err := tx.Where("audit_id = ? AND url = ?", auditID, oldIdentity).Delete(&store.Issue{}).Error; err != nil {
				return fmt.Errorf("replace issues: %w", err)
			}
		} else {
			var sharers int64
			if err := tx.Model(&store.AuditedUrl{}).
				Where("audit_id = ? AND url <> ? AND final_url = ?", auditID, fresh.URL, oldIdentity).
				Count(&sharers).Error; err != nil {
				return fmt.Errorf("count shared urls: %w", err)
			}
			if sharers == 0 {
				if err := tx.Where("audit_id = ? AND url = ?", auditID, oldIdentity).Delete(&store.Issue{}).Error; err != nil {
					return fmt.Errorf("replace issues: %w", err)
				}
			}
		}

		// Lifecycle metadata of the row: the row keeps its creation time and
		// its state unless it was missing, in which case the recheck revives
		// it.
		fresh.CreatedAt = row.CreatedAt
		fresh.LastAudited = now
		if row.State == string(domain.UrlStateMissing) {
			fresh.State = domain.UrlStateActive
		} else {
			fresh.State = domain.UrlState(row.State)
		}
		fresh.CalculateSummary()

		if err := tx.Save(store.AuditedUrlModel(auditID, fresh)).Error; err != nil {
			return fmt.Errorf("update audited url: %w", err)
		}

		seen := make(map[string]struct{})
		issueModels := make([]*store.Issue, 0, len(fresh.Issues))
		for j := range fresh.Issues {
			issue := &fresh.Issues[j]
			id := store.IssueID(auditID, newIdentity, issue.CheckName)
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
			issueModels = append(issueModels, store.IssueModel(auditID, newIdentity, issue, now))
		}
		if err := createInBatches(tx, issueModels); err != nil {
			return fmt.Errorf("insert issues: %w", err)
		}

		// The derived audit score changes with the URL; the run metadata of
		// the audit row stays untouched.
		var urlModels []store.AuditedUrl
		if err := tx.Where("audit_id = ?", auditID).Order("rowid").Find(&urlModels).Error; err != nil {
			return fmt.Errorf("load audited urls: %w", err)
		}
		var scoreSum float64
		scored := 0
		for i := range urlModels {
			u := urlModels[i].ToDomain()
			if u.State == domain.UrlStateMissing {
				continue
			}
			scoreSum += u.Summary.Score
			scored++
		}
		var score float64
		if scored > 0 {
			score = scoreSum / float64(scored)
		}
		if err := tx.Model(&store.Audit{}).Where("id = ?", auditID).Update("score", score).Error; err != nil {
			return fmt.Errorf("update audit score: %w", err)
		}
		return nil
	})
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

// CreateBlank persists the run configuration of a new audit without running
// it: the record carries name, description, targets, check names and the
// canonicalized engine config but no URLs or issues yet. The first run
// happens later through ReplaceRun, so creating an audit never triggers one.
func (s *AuditService) CreateBlank(ctx context.Context, name, description string, targets, checkNames []string, config json.RawMessage) (*domain.Audit, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("audit name is required")
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("audit must contain at least one target URL")
	}
	if len(checkNames) == 0 {
		return nil, fmt.Errorf("audit must contain at least one check")
	}

	canonicalConfig, err := json.Marshal(MergeConfig(*DefaultConfig(), config))
	if err != nil {
		return nil, fmt.Errorf("normalize audit config: %w", err)
	}

	audit := &domain.Audit{
		ID:          fmt.Sprintf("aud_%d", time.Now().UnixNano()),
		Name:        name,
		Description: description,
		Targets:     targets,
		CheckNames:  checkNames,
		Config:      canonicalConfig,
		StartedAt:   time.Now().UTC(),
	}
	if err := s.db.WithContext(ctx).Create(store.AuditModel(audit)).Error; err != nil {
		return nil, fmt.Errorf("insert audit: %w", err)
	}
	return audit, nil
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

// ListSnapshots returns the newest run snapshots of the audit, newest
// first, capped at limit.
func (s *AuditService) ListSnapshots(ctx context.Context, auditID string, limit int) ([]*domain.AuditSnapshot, error) {
	var models []store.AuditSnapshot
	query := s.db.WithContext(ctx).Where("audit_id = ?", auditID).Order("created_at DESC").Order("rowid DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list audit snapshots: %w", err)
	}

	snapshots := make([]*domain.AuditSnapshot, 0, len(models))
	for i := range models {
		snapshots = append(snapshots, models[i].ToDomain())
	}
	return snapshots, nil
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
		if err := tx.Where("audit_id = ?", id).Delete(&store.AuditSnapshot{}).Error; err != nil {
			return err
		}
		return tx.Delete(&store.Audit{}, "id = ?", id).Error
	})
	if err != nil {
		return fmt.Errorf("delete audit: %w", err)
	}
	return nil
}

// ResetData clears the run data of an audit while keeping the audit record
// and its self-contained run configuration (name, description, targets,
// checks and engine config) intact. The stored audited URLs, issues, run
// snapshots and exported files are deleted and the derived summary (score,
// duration) is zeroed; started_at moves to now so the record reads like a
// fresh, never-run audit.
func (s *AuditService) ResetData(ctx context.Context, id string) error {
	// Remove the exported workbooks and reports first so a failed cleanup
	// leaves the audit data fully intact.
	if s.excel != nil {
		if err := s.excel.RemoveAuditFile(id); err != nil {
			return fmt.Errorf("reset audit: %w", err)
		}
	}
	if s.html != nil {
		if err := s.html.RemoveAuditFile(id); err != nil {
			return fmt.Errorf("reset audit: %w", err)
		}
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// The audit row doubles as the existence check: resetting an audit
		// that does not exist must fail instead of silently deleting data
		// of a stale id.
		result := tx.Model(&store.Audit{}).
			Where("id = ?", id).
			Select("score", "duration_ms", "started_at").
			Updates(&store.Audit{Score: 0, DurationMs: 0, StartedAt: time.Now().UTC()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return domain.ErrAuditNotFound
		}
		if err := tx.Where("audit_id = ?", id).Delete(&store.AuditSnapshot{}).Error; err != nil {
			return err
		}
		if err := tx.Where("audit_id = ?", id).Delete(&store.AuditedUrl{}).Error; err != nil {
			return err
		}
		if err := tx.Where("audit_id = ?", id).Delete(&store.Issue{}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("reset audit: %w", err)
	}
	return nil
}

// Duplicate clones the self-contained run configuration of an existing
// audit into a new audit record without any run data (no URLs, issues or
// snapshots). The clone's name is the source name suffixed with DUPLICATE.
// Audits created before run configuration was stored inline fall back to
// the URLs of their latest run and to the checks observed in their issues.
func (s *AuditService) Duplicate(ctx context.Context, id string) (*domain.Audit, error) {
	src, err := s.GetConfig(ctx, id)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(src.Name)
	if name == "" {
		name = "Untitled Audit"
	}
	description := src.Description
	targets := src.Targets
	checkNames := src.CheckNames

	// Legacy audits: their records predate the inline run configuration.
	if len(targets) == 0 || len(checkNames) == 0 {
		full, err := s.GetByID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("duplicate audit: %w", err)
		}
		if description == "" {
			description = full.Description
		}
		if len(targets) == 0 {
			for _, u := range full.Urls {
				// URLs that did not reappear in the latest run are skipped:
				// a clone re-checks the URLs the source last audited.
				if u.State == domain.UrlStateMissing {
					continue
				}
				targets = append(targets, u.URL)
			}
		}
		if len(checkNames) == 0 {
			var names []string
			if err := s.db.WithContext(ctx).Model(&store.Issue{}).
				Where("audit_id = ?", id).
				Distinct("check_name").
				Order("check_name").
				Pluck("check_name", &names).Error; err != nil {
				return nil, fmt.Errorf("duplicate audit: collect checks: %w", err)
			}
			checkNames = names
		}
	}

	return s.CreateBlank(ctx, name+" DUPLICATE", description, targets, checkNames, src.Config)
}
