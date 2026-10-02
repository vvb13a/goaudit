package service

import (
	"github.com/vvb13a/goaudit/domain"
	"github.com/vvb13a/goaudit/store"
)

type graphView struct {
	edges   []domain.LinkRef
	targets map[string]domain.TargetStatus
	sources []string
}

func (v *graphView) Edges() []domain.LinkRef { return v.edges }

func (v *graphView) Target(url string) (domain.TargetStatus, bool) {
	target, ok := v.targets[url]
	return target, ok
}

func (v *graphView) Sources() []string { return v.sources }

// TargetURLs returns the distinct, normalized URLs of the run's link targets
// that are not themselves audited pages (those already have a fetch status).
func (s *GraphService) TargetURLs(a *domain.Audit) []string {
	audited := make(map[string]struct{}, len(a.Urls))
	for _, u := range a.Urls {
		audited[normalizeGraphURL(issueURL(u))] = struct{}{}
	}

	seen := make(map[string]struct{})
	var urls []string
	for _, u := range a.Urls {
		if u == nil {
			continue
		}
		for _, link := range u.Links {
			target := normalizeGraphURL(link.URL)
			if _, ok := audited[target]; ok {
				continue
			}
			if _, ok := seen[target]; ok {
				continue
			}
			seen[target] = struct{}{}
			urls = append(urls, target)
		}
	}
	return urls
}

// BuildView builds the in-memory graph view the graph checks evaluate.
// statuses holds the validation results keyed by normalized URL; it is nil when
// validation did not run.
func (s *GraphService) BuildView(a *domain.Audit, statuses map[string]store.LinkTarget) domain.GraphView {
	rootHosts := hostSet(a.Targets)
	view := &graphView{
		targets: make(map[string]domain.TargetStatus),
	}

	// Audited pages first, so a link target that is also an audited page keeps
	// its fetch status rather than being treated as an unvalidated target.
	for _, u := range a.Urls {
		if u == nil {
			continue
		}
		source := normalizeGraphURL(issueURL(u))
		if _, ok := view.targets[source]; !ok {
			view.targets[source] = domain.TargetStatus{
				URL:        source,
				Filetype:   detectFiletype(source, "html"),
				External:   !hostIn(source, rootHosts),
				Audited:    true,
				StatusCode: u.StatusCode,
				FinalURL:   issueURL(u),
				Validated:  true,
			}
		}
		view.sources = append(view.sources, source)
	}

	for _, u := range a.Urls {
		if u == nil {
			continue
		}
		source := normalizeGraphURL(issueURL(u))
		for _, link := range u.Links {
			target := normalizeGraphURL(link.URL)
			view.edges = append(view.edges, domain.LinkRef{
				SourceURL: source,
				TargetURL: target,
				Type:      link.Type,
			})
			if _, ok := view.targets[target]; ok {
				continue
			}
			status := domain.TargetStatus{
				URL:      target,
				Filetype: detectFiletype(target, link.Filetype),
				External: !hostIn(target, rootHosts),
			}
			if lt, ok := statuses[target]; ok {
				status.StatusCode = lt.StatusCode
				status.Error = lt.Error
				status.FinalURL = lt.FinalURL
				status.Validated = true
			}
			view.targets[target] = status
		}
	}
	return view
}
