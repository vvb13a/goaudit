package domain

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"
)

var (
	ErrPlanNotFound     = errors.New("plan not found")
	ErrPlanNameRequired = errors.New("plan name is required")
	ErrPlanEmptyURLs    = errors.New("plan must contain at least one URL")
	ErrInvalidPlanURL   = errors.New("plan contains an invalid URL")
)

type Plan struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URLs      []string  `json:"urls"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (p *Plan) URLCount() int {
	return len(p.URLs)
}

func (p *Plan) HasURL(targetURL string) bool {
	return slices.Contains(p.URLs, strings.TrimSpace(targetURL))
}

func (p *Plan) AddURL(targetURL string) {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL != "" && !p.HasURL(targetURL) {
		p.URLs = append(p.URLs, targetURL)
	}
}

func (p *Plan) RemoveURL(targetURL string) {
	targetURL = strings.TrimSpace(targetURL)
	p.URLs = slices.DeleteFunc(p.URLs, func(u string) bool {
		return u == targetURL
	})
}

func (p *Plan) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return ErrPlanNameRequired
	}
	if len(p.URLs) == 0 {
		return ErrPlanEmptyURLs
	}

	for _, rawURL := range p.URLs {
		parsed, err := url.ParseRequestURI(rawURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return ErrInvalidPlanURL
		}
	}
	return nil
}

type PlanRepository interface {
	Save(ctx context.Context, plan *Plan) error
	GetByID(ctx context.Context, id string) (*Plan, error)
	List(ctx context.Context) ([]*Plan, error)
	Delete(ctx context.Context, id string) error
}
