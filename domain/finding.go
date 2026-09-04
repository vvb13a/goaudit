package domain

// Finding is one repeated problem detected by a check. Checks that used to
// emit one issue per occurrence now collect their occurrences with
// IssueBuilder and aggregate them into the details of a single issue.
type Finding struct {
	Type     string         `json:"type"`
	Severity Severity       `json:"severity"`
	Message  string         `json:"message,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

// IssueBuilder collects repeated findings for a single check so they can be
// emitted as the details of one aggregated issue. The aggregated issue
// carries the highest severity found among its findings.
type IssueBuilder struct {
	findings []Finding
	severity Severity
	hasWorst bool
}

func NewIssueBuilder() *IssueBuilder {
	return &IssueBuilder{}
}

// Add records one finding, keeping the highest severity seen so far.
func (b *IssueBuilder) Add(finding Finding) {
	if !b.hasWorst || finding.Severity.IsHigherThan(b.severity) {
		b.severity = finding.Severity
		b.hasWorst = true
	}
	b.findings = append(b.findings, finding)
}

func (b *IssueBuilder) HasFindings() bool {
	return len(b.findings) > 0
}

func (b *IssueBuilder) Count() int {
	return len(b.findings)
}

// CountByType returns how many of the collected findings have the given
// type.
func (b *IssueBuilder) CountByType(typ string) int {
	count := 0
	for _, finding := range b.findings {
		if finding.Type == typ {
			count++
		}
	}
	return count
}

// Severity returns the highest severity among the collected findings, or
// SeveritySuccess when no finding was added.
func (b *IssueBuilder) Severity() Severity {
	if !b.hasWorst {
		return SeveritySuccess
	}
	return b.severity
}

// Details returns the evidence map holding all collected findings, ready to
// be attached to the aggregated issue emitted by the check.
func (b *IssueBuilder) Details() map[string]any {
	return map[string]any{
		"finding_count": b.Count(),
		"findings":      b.findings,
	}
}
