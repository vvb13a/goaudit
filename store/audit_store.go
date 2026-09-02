package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/vvb13a/goaudit/domain"
)

type SQLAuditStore struct {
	db *sql.DB
	q  *Queries
}

func NewSQLAuditStore(db *sql.DB, q *Queries) *SQLAuditStore {
	return &SQLAuditStore{
		db: db,
		q:  q,
	}
}

// Save atomically persists the Audit and all child Reports in a single transaction.
func (s *SQLAuditStore) Save(ctx context.Context, a *domain.Audit) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := s.q.WithTx(tx)

	// 1. Insert parent Audit
	err = qtx.CreateAudit(ctx, CreateAuditParams{
		ID:              a.ID,
		PlanID:          toNullString(a.PlanID),
		PlanName:        a.PlanName,
		ChecklistID:     toNullString(a.ChecklistID),
		ChecklistName:   a.ChecklistName,
		StartedAt:       a.StartedAt,
		DurationMs:      a.Duration.Milliseconds(),
		TotalEndpoints:  int64(a.Summary.TotalCount),
		PassedCount:     int64(a.Summary.PassedCount),
		FailedCount:     int64(a.Summary.FailedCount),
		SkippedCount:    int64(a.Summary.SkippedCount),
		HighestSeverity: string(a.Summary.HighestSeverity),
	})
	if err != nil {
		return fmt.Errorf("insert audit: %w", err)
	}

	// 2. Insert all child Reports
	for i, r := range a.Reports {
		issuesJSON, err := json.Marshal(r.Issues)
		if err != nil {
			return fmt.Errorf("marshal issues json: %w", err)
		}

		err = qtx.CreateReport(ctx, CreateReportParams{
			ID:              fmt.Sprintf("%s_r_%d", a.ID, i+1),
			AuditID:         a.ID,
			Url:             r.URL,
			FinalUrl:        r.FinalURL,
			StatusCode:      int64(r.StatusCode),
			DurationMs:      r.Duration.Milliseconds(),
			PassedCount:     int64(r.Summary.PassedCount),
			FailedCount:     int64(r.Summary.FailedCount),
			SkippedCount:    int64(r.Summary.SkippedCount),
			HighestSeverity: string(r.Summary.HighestSeverity),
			Issues:          string(issuesJSON),
		})
		if err != nil {
			return fmt.Errorf("insert report: %w", err)
		}
	}

	return tx.Commit()
}

// GetByID retrieves a full Audit record and hydrates all child Reports with their Issues.
func (s *SQLAuditStore) GetByID(ctx context.Context, id string) (*domain.Audit, error) {
	row, err := s.q.GetAudit(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrAuditNotFound
		}
		return nil, fmt.Errorf("get audit by id: %w", err)
	}

	audit := toDomainAudit(row)

	// Hydrate child reports
	reportRows, err := s.q.ListReportsByAuditID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list reports for audit: %w", err)
	}

	audit.Reports = make([]*domain.Report, 0, len(reportRows))
	for _, rr := range reportRows {
		r, err := toDomainReport(rr)
		if err != nil {
			return nil, err
		}
		audit.Reports = append(audit.Reports, r)
	}

	return audit, nil
}

// List retrieves audits matching criteria (omitting heavy reports for speed).
func (s *SQLAuditStore) List(ctx context.Context, filter domain.AuditFilter) ([]*domain.Audit, error) {
	var planIDNull sql.NullString
	if filter.PlanID != nil && *filter.PlanID != "" {
		planIDNull = sql.NullString{String: *filter.PlanID, Valid: true}
	}

	var severityNull sql.NullString
	if filter.HighestSeverity != nil && filter.HighestSeverity.IsValid() {
		severityNull = sql.NullString{String: string(*filter.HighestSeverity), Valid: true}
	}

	limit := int64(50)
	if filter.Limit > 0 {
		limit = int64(filter.Limit)
	}

	offset := int64(0)
	if filter.Offset > 0 {
		offset = int64(filter.Offset)
	}

	rows, err := s.q.ListAudits(ctx, ListAuditsParams{
		PlanID:          planIDNull,
		HighestSeverity: severityNull,
		Limit:           limit,
		Offset:          offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list audits: %w", err)
	}

	audits := make([]*domain.Audit, 0, len(rows))
	for _, r := range rows {
		audits = append(audits, toDomainAudit(r))
	}

	return audits, nil
}

// Delete removes an audit (child reports cascade-delete via SQLite foreign keys).
func (s *SQLAuditStore) Delete(ctx context.Context, id string) error {
	return s.q.DeleteAudit(ctx, id)
}

// --- Mapping Helpers ---

func toDomainAudit(row Audit) *domain.Audit {
	return &domain.Audit{
		ID:            row.ID,
		PlanID:        row.PlanID.String,
		PlanName:      row.PlanName,
		ChecklistID:   row.ChecklistID.String,
		ChecklistName: row.ChecklistName,
		StartedAt:     row.StartedAt,
		Duration:      time.Duration(row.DurationMs) * time.Millisecond,
		Summary: domain.Summary{
			TotalCount:      int(row.TotalEndpoints),
			PassedCount:     int(row.PassedCount),
			FailedCount:     int(row.FailedCount),
			SkippedCount:    int(row.SkippedCount),
			HighestSeverity: domain.Severity(row.HighestSeverity),
		},
	}
}

func toDomainReport(row Report) (*domain.Report, error) {
	var issues []domain.Issue
	if err := json.Unmarshal([]byte(row.Issues), &issues); err != nil {
		return nil, fmt.Errorf("unmarshal report issues: %w", err)
	}

	return &domain.Report{
		URL:        row.Url,
		FinalURL:   row.FinalUrl,
		StatusCode: int(row.StatusCode),
		Duration:   time.Duration(row.DurationMs) * time.Millisecond,
		Summary: domain.Summary{
			TotalCount:      len(issues),
			PassedCount:     int(row.PassedCount),
			FailedCount:     int(row.FailedCount),
			SkippedCount:    int(row.SkippedCount),
			HighestSeverity: domain.Severity(row.HighestSeverity),
		},
		Issues: issues,
	}, nil
}

func toNullString(s string) sql.NullString {
	return sql.NullString{
		String: s,
		Valid:  s != "",
	}
}
