package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/vvb13a/goaudit/domain"
)

type SQLPlanStore struct {
	q *Queries
}

func NewSQLPlanStore(q *Queries) *SQLPlanStore {
	return &SQLPlanStore{q: q}
}

func (s *SQLPlanStore) Save(ctx context.Context, p *domain.Plan) error {
	urlsJSON, err := json.Marshal(p.URLs)
	if err != nil {
		return err
	}

	err = s.q.UpdatePlan(ctx, UpdatePlanParams{
		ID:        p.ID,
		Name:      p.Name,
		Urls:      string(urlsJSON),
		UpdatedAt: p.UpdatedAt,
	})
	if err != nil {
		return s.q.CreatePlan(ctx, CreatePlanParams{
			ID:        p.ID,
			Name:      p.Name,
			Urls:      string(urlsJSON),
			CreatedAt: p.CreatedAt,
			UpdatedAt: p.UpdatedAt,
		})
	}
	return nil
}

func (s *SQLPlanStore) GetByID(ctx context.Context, id string) (*domain.Plan, error) {
	row, err := s.q.GetPlan(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrPlanNotFound
		}
		return nil, err
	}

	var urls []string
	_ = json.Unmarshal([]byte(row.Urls), &urls)

	return &domain.Plan{
		ID:        row.ID,
		Name:      row.Name,
		URLs:      urls,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}, nil
}

func (s *SQLPlanStore) List(ctx context.Context) ([]*domain.Plan, error) {
	rows, err := s.q.ListPlans(ctx)
	if err != nil {
		return nil, err
	}

	plans := make([]*domain.Plan, 0, len(rows))
	for _, r := range rows {
		var urls []string
		_ = json.Unmarshal([]byte(r.Urls), &urls)
		plans = append(plans, &domain.Plan{
			ID:        r.ID,
			Name:      r.Name,
			URLs:      urls,
			CreatedAt: r.CreatedAt,
			UpdatedAt: r.UpdatedAt,
		})
	}
	return plans, nil
}

func (s *SQLPlanStore) Delete(ctx context.Context, id string) error {
	return s.q.DeletePlan(ctx, id)
}
