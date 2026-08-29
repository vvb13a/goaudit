package data

type Severity string

const (
	SeveritySuccess Severity = "success"
	SeverityInfo    Severity = "info"
	SeverityNotice  Severity = "notice"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
	SeverityFatal   Severity = "fatal"
)

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

func (s Severity) IsHigherThan(other Severity) bool {
	return s.Weight() > other.Weight()
}
