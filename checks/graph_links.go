package checks

import (
	"context"
	"fmt"
	"strings"

	"github.com/vvb13a/goaudit/domain"
)

// graphLinkCheck validates a category of link targets against the run's
// persisted validation results. It is a graph check: it never runs against a
// single document, but once at the end of a run, and it files one aggregated
// issue per source page listing every broken target in its category.
type graphLinkCheck struct {
	info  domain.CheckInfo
	noun  string
	match func(target domain.TargetStatus) bool
}

func (c *graphLinkCheck) Info() domain.CheckInfo { return c.info }

// Supports always reports false: a graph check is not applied to documents.
func (c *graphLinkCheck) Supports(*domain.Document) bool { return false }

// Apply is never called; Evaluate is used instead.
func (c *graphLinkCheck) Apply(context.Context, *domain.Document) domain.Issue {
	return domain.NewPassIssue(c, "Evaluated against the link graph after the run.")
}

func (c *graphLinkCheck) Evaluate(ctx context.Context, view domain.GraphView) []domain.PageIssue {
	type sourceAgg struct {
		checked  map[string]struct{}
		findings []domain.Finding
	}

	bySource := make(map[string]*sourceAgg)
	validated := 0

	for _, edge := range view.Edges() {
		if ctx.Err() != nil {
			break
		}
		target, ok := view.Target(edge.TargetURL)
		if !ok || !c.match(target) || !target.Validated {
			continue
		}
		validated++

		agg := bySource[edge.SourceURL]
		if agg == nil {
			agg = &sourceAgg{checked: make(map[string]struct{})}
			bySource[edge.SourceURL] = agg
		}
		agg.checked[edge.TargetURL] = struct{}{}
		if severity, broken := brokenTarget(target); broken {
			agg.findings = append(agg.findings, brokenTargetFinding(target, severity))
		}
	}

	// No target of this category could be judged: emit a single notice instead
	// of a misleading pass.
	if validated == 0 {
		return []domain.PageIssue{c.notice(view)}
	}

	issues := make([]domain.PageIssue, 0, len(bySource))
	for source, agg := range bySource {
		checked := len(agg.checked)
		if len(agg.findings) == 0 {
			issues = append(issues, domain.PageIssue{
				URL: source,
				Issue: domain.NewPassIssueWithDetails(
					c,
					fmt.Sprintf("All %d %s are reachable.", checked, c.noun),
					map[string]any{"links_checked": checked},
				),
			})
			continue
		}

		builder := domain.NewIssueBuilder()
		for _, finding := range agg.findings {
			builder.Add(finding)
		}
		details := builder.Details()
		details["links_checked"] = checked
		issues = append(issues, domain.PageIssue{
			URL: source,
			Issue: domain.NewFailIssue(
				c,
				builder.Severity(),
				fmt.Sprintf("%d of %d %s are broken.", len(agg.findings), checked, c.noun),
				details,
			),
		})
	}
	return issues
}

// notice reports that the check ran without any validation result to work on.
func (c *graphLinkCheck) notice(view domain.GraphView) domain.PageIssue {
	sources := view.Sources()
	url := ""
	if len(sources) > 0 {
		url = sources[0]
	}
	return domain.PageIssue{
		URL: url,
		Issue: domain.NewRawIssue(
			c.info.Name,
			c.info.Category,
			domain.SeverityNotice,
			fmt.Sprintf("No %s could be validated (the link graph or link validation is disabled).", c.noun),
			nil,
		),
	}
}

// brokenTarget classifies a validated target. A missing resource (404/410), a
// server error (5xx) or a connection failure is an error; a forbidden resource
// (403) and the remaining 4xx are warnings, since validation may simply be
// blocked (e.g. abuse protection) rather than genuinely broken.
func brokenTarget(target domain.TargetStatus) (domain.Severity, bool) {
	switch {
	case target.Error != "" || target.StatusCode == 0:
		return domain.SeverityError, true
	case target.StatusCode >= 500:
		return domain.SeverityError, true
	case target.StatusCode == 404 || target.StatusCode == 410:
		return domain.SeverityError, true
	case target.StatusCode >= 400:
		return domain.SeverityWarning, true
	default:
		return domain.SeveritySuccess, false
	}
}

func brokenTargetFinding(target domain.TargetStatus, severity domain.Severity) domain.Finding {
	data := map[string]any{
		"link_url":    target.URL,
		"status_code": target.StatusCode,
	}
	if target.FinalURL != "" && target.FinalURL != target.URL {
		data["final_url"] = target.FinalURL
	}
	if target.Error != "" {
		data["error_message"] = target.Error
	}
	return domain.Finding{
		Type:     "broken_link",
		Severity: severity,
		Message:  statusMessage(target),
		Data:     data,
	}
}

func statusMessage(target domain.TargetStatus) string {
	switch {
	case target.Error != "":
		return fmt.Sprintf("Could not connect: %s", target.Error)
	case target.StatusCode == 404 || target.StatusCode == 410:
		return fmt.Sprintf("Target is not found (HTTP %d).", target.StatusCode)
	case target.StatusCode == 403:
		return "Target is forbidden (HTTP 403); validation may be blocked."
	case target.StatusCode >= 500:
		return fmt.Sprintf("Target returned a server error (HTTP %d).", target.StatusCode)
	case target.StatusCode >= 400:
		return fmt.Sprintf("Target is not accessible (HTTP %d).", target.StatusCode)
	default:
		return fmt.Sprintf("Target returned an unexpected status (HTTP %d).", target.StatusCode)
	}
}

// NewExternalLinksCheck validates outbound links to other hosts.
func NewExternalLinksCheck() *graphLinkCheck {
	return &graphLinkCheck{
		info: domain.CheckInfo{
			Name:        "external_links",
			Label:       "External Links",
			Description: "Validates outbound links to other hosts after the run.",
			Category:    domain.CategoryGeneral,
			Scope:       domain.ScopeGraph,
		},
		noun:  "external links",
		match: func(t domain.TargetStatus) bool { return t.External },
	}
}

// NewDocumentLinksCheck validates links to internal documents (HTML/XML).
func NewDocumentLinksCheck() *graphLinkCheck {
	return &graphLinkCheck{
		info: domain.CheckInfo{
			Name:        "document_links",
			Label:       "Document Links",
			Description: "Validates links to internal documents (HTML/XML) after the run.",
			Category:    domain.CategoryGeneral,
			Scope:       domain.ScopeGraph,
		},
		noun: "document links",
		match: func(t domain.TargetStatus) bool {
			return !t.External && (t.Filetype == "html" || t.Filetype == "xml")
		},
	}
}

// NewAssetLinksCheck validates links to stylesheets and scripts.
func NewAssetLinksCheck() *graphLinkCheck {
	return &graphLinkCheck{
		info: domain.CheckInfo{
			Name:        "asset_links",
			Label:       "Asset Links",
			Description: "Validates links to stylesheets and scripts after the run.",
			Category:    domain.CategoryGeneral,
			Scope:       domain.ScopeGraph,
		},
		noun: "asset links",
		match: func(t domain.TargetStatus) bool {
			return t.Filetype == "css" || t.Filetype == "js"
		},
	}
}

// NewMediaLinksCheck validates links to images, video, audio and PDFs.
func NewMediaLinksCheck() *graphLinkCheck {
	return &graphLinkCheck{
		info: domain.CheckInfo{
			Name:        "media_links",
			Label:       "Media Links",
			Description: "Validates links to images, video, audio and PDFs after the run.",
			Category:    domain.CategoryGeneral,
			Scope:       domain.ScopeGraph,
		},
		noun:  "media links",
		match: isMediaFiletype,
	}
}

var mediaFiletypes = map[string]struct{}{
	"pdf": {}, "image": {}, "jpg": {}, "jpeg": {}, "png": {}, "webp": {},
	"gif": {}, "svg": {}, "ico": {}, "avif": {}, "bmp": {},
	"video": {}, "audio": {}, "mp4": {}, "webm": {}, "mp3": {}, "ogg": {}, "wav": {},
}

func isMediaFiletype(t domain.TargetStatus) bool {
	_, ok := mediaFiletypes[strings.ToLower(t.Filetype)]
	return ok
}
