package service

import (
	"context"
	"fmt"
	"time"

	"github.com/vvb13a/goaudit/domain"
)

type PlanService struct {
	repo domain.PlanRepository
}

func NewPlanService(repo domain.PlanRepository) *PlanService {
	return &PlanService{
		repo: repo,
	}
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

	return s.repo.Save(ctx, plan)
}

func (s *PlanService) Update(ctx context.Context, plan *domain.Plan) error {
	if err := plan.Validate(); err != nil {
		return err
	}

	existing, err := s.repo.GetByID(ctx, plan.ID)
	if err != nil {
		return err
	}

	plan.CreatedAt = existing.CreatedAt
	plan.UpdatedAt = time.Now().UTC()

	return s.repo.Save(ctx, plan)
}

func (s *PlanService) GetByID(ctx context.Context, id string) (*domain.Plan, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *PlanService) List(ctx context.Context) ([]*domain.Plan, error) {
	return s.repo.List(ctx)
}

func (s *PlanService) Delete(ctx context.Context, id string) error {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}
