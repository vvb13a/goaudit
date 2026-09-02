package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vvb13a/goaudit/domain"
	"github.com/vvb13a/goaudit/store"
	"gorm.io/gorm"
)

type ChecklistService struct {
	db       *gorm.DB
	registry *CheckRegistry
}

func NewChecklistService(db *gorm.DB, registry *CheckRegistry) *ChecklistService {
	return &ChecklistService{
		db:       db,
		registry: registry,
	}
}

func (s *ChecklistService) Create(ctx context.Context, cl *domain.Checklist) error {
	if err := cl.Validate(); err != nil {
		return err
	}

	if _, err := s.registry.Resolve(cl.CheckNames); err != nil {
		return err
	}

	if cl.ID == "" {
		cl.ID = fmt.Sprintf("chk_%d", time.Now().UnixNano())
	}
	now := time.Now().UTC()
	cl.CreatedAt = now
	cl.UpdatedAt = now

	if err := s.db.WithContext(ctx).Create(store.ChecklistModel(cl)).Error; err != nil {
		return fmt.Errorf("create checklist: %w", err)
	}
	return nil
}

func (s *ChecklistService) Update(ctx context.Context, cl *domain.Checklist) error {
	if err := cl.Validate(); err != nil {
		return err
	}

	if _, err := s.registry.Resolve(cl.CheckNames); err != nil {
		return err
	}

	var existing store.Checklist
	err := s.db.WithContext(ctx).First(&existing, "id = ?", cl.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrChecklistNotFound
	}
	if err != nil {
		return fmt.Errorf("fetch checklist: %w", err)
	}

	cl.CreatedAt = existing.CreatedAt
	cl.UpdatedAt = time.Now().UTC()
	cl.IsActive = existing.IsActive

	if err := s.db.WithContext(ctx).Save(store.ChecklistModel(cl)).Error; err != nil {
		return fmt.Errorf("update checklist: %w", err)
	}
	return nil
}

func (s *ChecklistService) GetByID(ctx context.Context, id string) (*domain.Checklist, error) {
	var model store.Checklist
	err := s.db.WithContext(ctx).First(&model, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrChecklistNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get checklist: %w", err)
	}
	return model.ToDomain(), nil
}

func (s *ChecklistService) GetActive(ctx context.Context) (*domain.Checklist, error) {
	var model store.Checklist
	err := s.db.WithContext(ctx).Where("is_active = ?", true).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrChecklistNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get active checklist: %w", err)
	}
	return model.ToDomain(), nil
}

func (s *ChecklistService) SetActive(ctx context.Context, id string) error {
	if _, err := s.GetByID(ctx, id); err != nil {
		return err
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&store.Checklist{}).Where("1 = 1").Update("is_active", false).Error; err != nil {
			return fmt.Errorf("deactivate checklists: %w", err)
		}

		updates := map[string]any{
			"is_active":  true,
			"updated_at": time.Now().UTC(),
		}
		if err := tx.Model(&store.Checklist{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return fmt.Errorf("set active checklist: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *ChecklistService) List(ctx context.Context) ([]*domain.Checklist, error) {
	var models []store.Checklist
	if err := s.db.WithContext(ctx).Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list checklists: %w", err)
	}

	checklists := make([]*domain.Checklist, 0, len(models))
	for i := range models {
		checklists = append(checklists, models[i].ToDomain())
	}
	return checklists, nil
}

func (s *ChecklistService) Delete(ctx context.Context, id string) error {
	cl, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if cl.IsActive {
		return domain.ErrCannotDeleteActive
	}

	if err := s.db.WithContext(ctx).Delete(&store.Checklist{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete checklist: %w", err)
	}
	return nil
}

func (s *ChecklistService) SeedDefault(ctx context.Context, r *CheckRegistry) {
	checklists, err := s.List(ctx)
	if err != nil || len(checklists) > 0 {
		return
	}

	allChecks := r.All()
	names := make([]string, 0, len(allChecks))
	for _, c := range allChecks {
		names = append(names, c.Info().Name)
	}

	initial := &domain.Checklist{
		Name:        "Full Audit (All Checks)",
		Description: "Runs all registered SEO, security, and performance rules.",
		CheckNames:  names,
		IsActive:    true,
	}

	_ = s.Create(ctx, initial)
}
