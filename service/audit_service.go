package service

import (
	"context"
	"fmt"

	"github.com/vvb13a/goaudit/domain"
)

type AuditService struct {
	plans      domain.PlanRepository
	checklists domain.ChecklistRepository
	audits     domain.AuditRepository
	registry   *CheckRegistry
	runner     *Runner
}

func NewAuditService(
	plans domain.PlanRepository,
	checklists domain.ChecklistRepository,
	audits domain.AuditRepository,
	registry *CheckRegistry,
	runner *Runner,
) *AuditService {
	return &AuditService{
		plans:      plans,
		checklists: checklists,
		audits:     audits,
		registry:   registry,
		runner:     runner,
	}
}

// RunPlan executes an audit for a saved Plan using the currently active Checklist.
func (s *AuditService) RunPlan(
	ctx context.Context,
	planID string,
	onProgress ProgressCallback,
) (*domain.Audit, error) {
	// 1. Fetch target Plan
	plan, err := s.plans.GetByID(ctx, planID)
	if err != nil {
		return nil, fmt.Errorf("fetch plan: %w", err)
	}

	// 2. Fetch the currently active Checklist
	activeChecklist, err := s.checklists.GetActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch active checklist: %w", err)
	}

	// 3. Resolve check names to executable domain.Check instances
	checks, err := s.registry.Resolve(activeChecklist.CheckNames)
	if err != nil {
		return nil, fmt.Errorf("resolve checklist checks: %w", err)
	}

	// 4. Run the execution engine
	audit, err := s.runner.ExecutePlan(ctx, plan, activeChecklist, checks, onProgress)
	if err != nil {
		return nil, fmt.Errorf("execute audit run: %w", err)
	}

	// 5. Persist the historical audit record & reports
	if err := s.audits.Save(ctx, audit); err != nil {
		return nil, fmt.Errorf("save audit results: %w", err)
	}

	return audit, nil
}

// RunAdHoc executes a quick audit on arbitrary URLs without needing a pre-saved Plan.
// Useful for CLI single-URL checks or quick scratchpad runs in the UI/TUI.
func (s *AuditService) RunAdHoc(
	ctx context.Context,
	urls []string,
	checkNames []string,
	onProgress ProgressCallback,
) (*domain.Audit, error) {
	var checklist *domain.Checklist
	var checks []domain.Check
	var err error

	// If no specific check names provided, use the active checklist
	if len(checkNames) == 0 {
		checklist, err = s.checklists.GetActive(ctx)
		if err != nil {
			return nil, fmt.Errorf("fetch active checklist: %w", err)
		}
		checks, err = s.registry.Resolve(checklist.CheckNames)
		if err != nil {
			return nil, fmt.Errorf("resolve checklist checks: %w", err)
		}
	} else {
		checklist = &domain.Checklist{
			Name:       "Ad-hoc Ruleset",
			CheckNames: checkNames,
		}
		checks, err = s.registry.Resolve(checkNames)
		if err != nil {
			return nil, fmt.Errorf("resolve check names: %w", err)
		}
	}

	adHocPlan := &domain.Plan{
		Name: "Ad-hoc Run",
		URLs: urls,
	}

	audit, err := s.runner.ExecutePlan(ctx, adHocPlan, checklist, checks, onProgress)
	if err != nil {
		return nil, fmt.Errorf("execute ad-hoc audit: %w", err)
	}

	// Optionally persist or return directly
	if err := s.audits.Save(ctx, audit); err != nil {
		return nil, fmt.Errorf("save ad-hoc audit: %w", err)
	}

	return audit, nil
}

// GetByID retrieves a historical audit with all hydrated reports.
func (s *AuditService) GetByID(ctx context.Context, id string) (*domain.Audit, error) {
	return s.audits.GetByID(ctx, id)
}

// List returns audits matching the filter criteria.
func (s *AuditService) List(ctx context.Context, filter domain.AuditFilter) ([]*domain.Audit, error) {
	return s.audits.List(ctx, filter)
}

// Delete removes an audit record and its associated reports.
func (s *AuditService) Delete(ctx context.Context, id string) error {
	return s.audits.Delete(ctx, id)
}
