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

type SQLChecklistStore struct {
	db *sql.DB
	q  *Queries
}

func NewSQLChecklistStore(db *sql.DB, q *Queries) *SQLChecklistStore {
	return &SQLChecklistStore{
		db: db,
		q:  q,
	}
}

// Save inserts a new checklist or updates an existing one.
func (s *SQLChecklistStore) Save(ctx context.Context, cl *domain.Checklist) error {
	checkNamesJSON, err := json.Marshal(cl.CheckNames)
	if err != nil {
		return fmt.Errorf("marshal check names: %w", err)
	}

	// Try updating first; if it doesn't exist, create it
	err = s.q.UpdateChecklist(ctx, UpdateChecklistParams{
		ID:          cl.ID,
		Name:        cl.Name,
		Description: cl.Description,
		CheckNames:  string(checkNamesJSON),
		UpdatedAt:   cl.UpdatedAt,
	})
	if err != nil {
		return s.q.CreateChecklist(ctx, CreateChecklistParams{
			ID:          cl.ID,
			Name:        cl.Name,
			Description: cl.Description,
			CheckNames:  string(checkNamesJSON),
			IsActive:    cl.IsActive,
			CreatedAt:   cl.CreatedAt,
			UpdatedAt:   cl.UpdatedAt,
		})
	}

	return nil
}

// GetByID retrieves a single checklist by its ID.
func (s *SQLChecklistStore) GetByID(ctx context.Context, id string) (*domain.Checklist, error) {
	row, err := s.q.GetChecklist(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrChecklistNotFound
		}
		return nil, fmt.Errorf("get checklist by id: %w", err)
	}

	return toDomainChecklist(row)
}

// GetActive retrieves the single currently active checklist.
func (s *SQLChecklistStore) GetActive(ctx context.Context) (*domain.Checklist, error) {
	row, err := s.q.GetActiveChecklist(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrChecklistNotFound
		}
		return nil, fmt.Errorf("get active checklist: %w", err)
	}

	return toDomainChecklist(row)
}

// SetActive atomically deactivates all checklists and marks the specified ID as active.
func (s *SQLChecklistStore) SetActive(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := s.q.WithTx(tx)

	// 1. Deactivate all checklists
	if err := qtx.DeactivateAllChecklists(ctx); err != nil {
		return fmt.Errorf("deactivate checklists: %w", err)
	}

	// 2. Activate target checklist
	err = qtx.SetActiveChecklist(ctx, SetActiveChecklistParams{
		ID:        id,
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("set active checklist: %w", err)
	}

	return tx.Commit()
}

// List returns all checklists ordered by creation date descending.
func (s *SQLChecklistStore) List(ctx context.Context) ([]*domain.Checklist, error) {
	rows, err := s.q.ListChecklists(ctx)
	if err != nil {
		return nil, fmt.Errorf("list checklists: %w", err)
	}

	checklists := make([]*domain.Checklist, 0, len(rows))
	for _, r := range rows {
		cl, err := toDomainChecklist(r)
		if err != nil {
			return nil, err
		}
		checklists = append(checklists, cl)
	}

	return checklists, nil
}

// Delete removes a checklist by ID.
func (s *SQLChecklistStore) Delete(ctx context.Context, id string) error {
	if err := s.q.DeleteChecklist(ctx, id); err != nil {
		return fmt.Errorf("delete checklist: %w", err)
	}
	return nil
}

// Helper function to map a sqlc Checklist row into a domain.Checklist entity.
func toDomainChecklist(row Checklist) (*domain.Checklist, error) {
	var checkNames []string
	if err := json.Unmarshal([]byte(row.CheckNames), &checkNames); err != nil {
		return nil, fmt.Errorf("unmarshal check names json: %w", err)
	}

	return &domain.Checklist{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		CheckNames:  checkNames,
		IsActive:    row.IsActive,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}, nil
}
