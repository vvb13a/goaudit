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

type PlanService struct {
	db *gorm.DB
}

func NewPlanService(db *gorm.DB) *PlanService {
	return &PlanService{db: db}
}

func (s *PlanService) Create(ctx context.Context, plan *domain.Plan) error {
	if err := plan.Validate(); err != nil {
		return err
	}

	if plan.ID == "" {
		plan.ID = fmt.Sprintf("plan_%d", time.Now().UnixNano())
	}
	now := time.Now().UTC()
	plan.CreatedAt = now
	plan.UpdatedAt = now

	if err := s.db.WithContext(ctx).Create(store.PlanModel(plan)).Error; err != nil {
		return fmt.Errorf("create plan: %w", err)
	}
	return nil
}

func (s *PlanService) Update(ctx context.Context, plan *domain.Plan) error {
	if err := plan.Validate(); err != nil {
		return err
	}

	var existing store.Plan
	err := s.db.WithContext(ctx).First(&existing, "id = ?", plan.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrPlanNotFound
	}
	if err != nil {
		return fmt.Errorf("fetch plan: %w", err)
	}

	plan.CreatedAt = existing.CreatedAt
	plan.UpdatedAt = time.Now().UTC()

	if err := s.db.WithContext(ctx).Save(store.PlanModel(plan)).Error; err != nil {
		return fmt.Errorf("update plan: %w", err)
	}
	return nil
}

func (s *PlanService) GetByID(ctx context.Context, id string) (*domain.Plan, error) {
	var model store.Plan
	err := s.db.WithContext(ctx).First(&model, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrPlanNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get plan: %w", err)
	}
	return model.ToDomain(), nil
}

func (s *PlanService) List(ctx context.Context) ([]*domain.Plan, error) {
	var models []store.Plan
	if err := s.db.WithContext(ctx).Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}

	plans := make([]*domain.Plan, 0, len(models))
	for i := range models {
		plans = append(plans, models[i].ToDomain())
	}
	return plans, nil
}

func (s *PlanService) Delete(ctx context.Context, id string) error {
	if _, err := s.GetByID(ctx, id); err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Delete(&store.Plan{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete plan: %w", err)
	}
	return nil
}
