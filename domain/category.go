package domain

import (
	"fmt"
	"strings"
)

type Category string

const (
	CategorySEO           Category = "seo"
	CategorySecurity      Category = "security"
	CategoryPerformance   Category = "performance"
	CategoryAccessibility Category = "accessibility"
	CategoryHeaders       Category = "headers"
	CategoryContent       Category = "content"
	CategoryGeneral       Category = "general"
)

var AllCategories = []Category{
	CategorySEO,
	CategorySecurity,
	CategoryPerformance,
	CategoryAccessibility,
	CategoryHeaders,
	CategoryContent,
	CategoryGeneral,
}

func (c Category) IsValid() bool {
	for _, valid := range AllCategories {
		if c == valid {
			return true
		}
	}
	return false
}

func (c Category) DisplayName() string {
	switch c {
	case CategorySEO:
		return "SEO"
	case CategorySecurity:
		return "Security"
	case CategoryPerformance:
		return "Performance"
	case CategoryAccessibility:
		return "Accessibility"
	case CategoryHeaders:
		return "HTTP Headers"
	case CategoryContent:
		return "Content & Markup"
	default:
		return "General"
	}
}

func (c Category) String() string {
	return string(c)
}

func ParseCategory(raw string) (Category, error) {
	normalized := Category(strings.ToLower(strings.TrimSpace(raw)))
	if !normalized.IsValid() {
		return "", fmt.Errorf("invalid category %q", raw)
	}
	return normalized, nil
}
