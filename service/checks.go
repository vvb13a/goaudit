package service

import (
	"fmt"
	"sort"
	"sync"

	"github.com/vvb13a/goaudit/domain"
)

type CheckRegistry struct {
	mu     sync.RWMutex
	checks map[string]domain.Check
}

func NewCheckRegistry(checks ...domain.Check) *CheckRegistry {
	r := &CheckRegistry{
		checks: make(map[string]domain.Check),
	}
	r.Register(checks...)
	return r
}

func (r *CheckRegistry) Register(checks ...domain.Check) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, c := range checks {
		r.checks[c.Info().Name] = c
	}
}

func (r *CheckRegistry) Get(name string) (domain.Check, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.checks[name]
	return c, ok
}

func (r *CheckRegistry) Exists(name string) bool {
	_, ok := r.Get(name)
	return ok
}

func (r *CheckRegistry) Resolve(names []string) ([]domain.Check, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var resolved []domain.Check
	var missing []string

	for _, name := range names {
		if c, exists := r.checks[name]; exists {
			resolved = append(resolved, c)
		} else {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("unknown checks: %v", missing)
	}

	return resolved, nil
}

func (r *CheckRegistry) All() []domain.Check {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]domain.Check, 0, len(r.checks))
	for _, c := range r.checks {
		list = append(list, c)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].Info().Name < list[j].Info().Name
	})

	return list
}

func (r *CheckRegistry) ByCategory(cat domain.Category) []domain.Check {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []domain.Check
	for _, c := range r.checks {
		if c.Info().Category == cat {
			list = append(list, c)
		}
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].Info().Name < list[j].Info().Name
	})

	return list
}
