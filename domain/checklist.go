package domain

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
)

var (
	ErrChecklistNotFound     = errors.New("checklist not found")
	ErrChecklistNameRequired = errors.New("checklist name is required")
	ErrChecklistEmptyChecks  = errors.New("checklist must contain at least one check")
	ErrCannotDeleteActive    = errors.New("cannot delete the currently active checklist")
)

type Checklist struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CheckNames  []string  `json:"check_names"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (c *Checklist) HasCheck(name string) bool {
	return slices.Contains(c.CheckNames, name)
}

func (c *Checklist) AddCheck(name string) {
	name = strings.TrimSpace(name)
	if name != "" && !c.HasCheck(name) {
		c.CheckNames = append(c.CheckNames, name)
	}
}

func (c *Checklist) RemoveCheck(name string) {
	c.CheckNames = slices.DeleteFunc(c.CheckNames, func(n string) bool {
		return n == name
	})
}

func (c *Checklist) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return ErrChecklistNameRequired
	}
	if len(c.CheckNames) == 0 {
		return ErrChecklistEmptyChecks
	}
	return nil
}

type ChecklistRepository interface {
	Save(ctx context.Context, checklist *Checklist) error
	GetByID(ctx context.Context, id string) (*Checklist, error)
	GetActive(ctx context.Context) (*Checklist, error)
	SetActive(ctx context.Context, id string) error
	List(ctx context.Context) ([]*Checklist, error)
	Delete(ctx context.Context, id string) error
}
