package service

import (
	"context"
	"fmt"
	"time"

	"github.com/vvb13a/goaudit/domain"
)

type ChecklistService struct {
	repo     domain.ChecklistRepository
	registry *CheckRegistry
}

func NewChecklistService(repo domain.ChecklistRepository, registry *CheckRegistry) *ChecklistService {
	return &ChecklistService{
		repo:     repo,
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

	return s.repo.Save(ctx, cl)
}

func (s *ChecklistService) Update(ctx context.Context, cl *domain.Checklist) error {
	if err := cl.Validate(); err != nil {
		return err
	}

	if _, err := s.registry.Resolve(cl.CheckNames); err != nil {
		return err
	}

	existing, err := s.repo.GetByID(ctx, cl.ID)
	if err != nil {
		return err
	}

	cl.CreatedAt = existing.CreatedAt
	cl.UpdatedAt = time.Now().UTC()
	cl.IsActive = existing.IsActive

	return s.repo.Save(ctx, cl)
}

func (s *ChecklistService) GetByID(ctx context.Context, id string) (*domain.Checklist, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *ChecklistService) GetActive(ctx context.Context) (*domain.Checklist, error) {
	return s.repo.GetActive(ctx)
}

func (s *ChecklistService) SetActive(ctx context.Context, id string) error {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return err
	}
	return s.repo.SetActive(ctx, id)
}

func (s *ChecklistService) List(ctx context.Context) ([]*domain.Checklist, error) {
	return s.repo.List(ctx)
}

func (s *ChecklistService) Delete(ctx context.Context, id string) error {
	cl, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if cl.IsActive {
		return domain.ErrCannotDeleteActive
	}

	return s.repo.Delete(ctx, id)
}
