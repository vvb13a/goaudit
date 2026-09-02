package domain

import (
	"fmt"
	"strings"
)

type Severity string

const (
	SeveritySuccess Severity = "success"
	SeverityInfo    Severity = "info"
	SeverityNotice  Severity = "notice"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
	SeverityFatal   Severity = "fatal"
)

var AllSeverities = []Severity{
	SeveritySuccess,
	SeverityInfo,
	SeverityNotice,
	SeverityWarning,
	SeverityError,
	SeverityFatal,
}

func (s Severity) Weight() int {
	switch s {
	case SeveritySuccess:
		return 0
	case SeverityInfo:
		return 10
	case SeverityNotice:
		return 20
	case SeverityWarning:
		return 30
	case SeverityError:
		return 40
	case SeverityFatal:
		return 50
	default:
		return -1
	}
}

func (s Severity) IsValid() bool {
	return s.Weight() >= 0
}

func (s Severity) IsHigherThan(other Severity) bool {
	return s.Weight() > other.Weight()
}

func (s Severity) IsFailure() bool {
	return s.Weight() >= SeverityWarning.Weight()
}

func (s Severity) String() string {
	return string(s)
}

func ParseSeverity(raw string) (Severity, error) {
	normalized := Severity(strings.ToLower(strings.TrimSpace(raw)))
	if !normalized.IsValid() {
		return "", fmt.Errorf("invalid severity %q: must be one of [success, info, notice, warning, error, fatal]", raw)
	}
	return normalized, nil
}
