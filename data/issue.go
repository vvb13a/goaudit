package data

type Issue struct {
	CheckName string         `json:"check_name"`
	Checklist string         `json:"checklist"`
	Severity  Severity       `json:"severity"`
	Passed    bool           `json:"passed"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
}
