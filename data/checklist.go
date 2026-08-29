package data

import (
	"slices"
	"time"
)

type Checklist struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	CheckNames []string  `json:"check_names"`
	IsDefault  bool      `json:"is_default"`
	CreatedAt  time.Time `json:"created_at"`
}

func (c *Checklist) HasCheck(name string) bool {
	return slices.Contains(c.CheckNames, name)
}
