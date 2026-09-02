package domain

type Issue struct {
	CheckName string         `json:"check_name"`
	Category  Category       `json:"category"`
	Severity  Severity       `json:"severity"`
	Passed    bool           `json:"passed"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
}

func (i Issue) IsFailure() bool {
	return !i.Passed && i.Severity.IsFailure()
}

func NewRawIssue(checkName string, category Category, severity Severity, message string, details map[string]any) Issue {
	return Issue{
		CheckName: checkName,
		Category:  category,
		Severity:  severity,
		Passed:    !severity.IsFailure(),
		Message:   message,
		Details:   details,
	}
}
