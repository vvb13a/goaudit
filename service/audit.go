package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/vvb13a/goaudit/domain"
	"github.com/vvb13a/goaudit/store"
	"gorm.io/gorm"
)

type AuditService struct {
	db    *gorm.DB
	excel *ExcelService
}

func NewAuditService(db *gorm.DB, excel *ExcelService) *AuditService {
	return &AuditService{db: db, excel: excel}
}

func (s *AuditService) Create(ctx context.Context, a *domain.Audit) error {
	reportModels := make([]*store.Report, 0, len(a.Reports))
	for i, r := range a.Reports {
		reportModels = append(reportModels, store.ReportModel(a.ID, fmt.Sprintf("%s_r_%d", a.ID, i+1), r))
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
		return nil
	})
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

	audit.Reports = make([]*domain.Report, 0, len(reportModels))
	for i := range reportModels {
		audit.Reports = append(audit.Reports, reportModels[i].ToDomain())
	}

	return audit, nil
}

func (s *AuditService) List(ctx context.Context, filter domain.AuditFilter) ([]*domain.Audit, error) {
	query := s.db.WithContext(ctx).Model(&store.Audit{}).Order("started_at DESC")

	if filter.PlanID != nil && *filter.PlanID != "" {
		query = query.Where("plan_id = ?", *filter.PlanID)
	}
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
	// Remove the exported workbook before the database row so that a failed
	// cleanup leaves the audit fully intact instead of half-deleted.
	if s.excel != nil {
		if err := s.excel.RemoveAuditFile(id); err != nil {
			return fmt.Errorf("delete audit: %w", err)
		}
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("audit_id = ?", id).Delete(&store.Report{}).Error; err != nil {
			return err
		}
		return tx.Delete(&store.Audit{}, "id = ?", id).Error
	})
	if err != nil {
		return fmt.Errorf("delete audit: %w", err)
	}
	return nil
}
