package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/vvb13a/goaudit/domain"
	"github.com/vvb13a/goaudit/store"
	"gorm.io/gorm"
)

// applyIssueFilter pushes the dimensions of an issue filter into the query.
// Each dimension becomes an IN set and the dimensions combine with AND; an
// empty dimension is skipped, so the zero-value filter leaves the query
// untouched.
func applyIssueFilter(q *gorm.DB, filter domain.IssueFilter) *gorm.DB {
	if len(filter.Severities) > 0 {
		q = q.Where("severity IN ?", stringValues(filter.Severities))
	}
	if len(filter.Lifecycles) > 0 {
		q = q.Where("lifecycle IN ?", stringValues(filter.Lifecycles))
	}
	if len(filter.CheckNames) > 0 {
		q = q.Where("check_name IN ?", filter.CheckNames)
	}
	if len(filter.Categories) > 0 {
		q = q.Where("category IN ?", stringValues(filter.Categories))
	}
	if filter.URLContains != "" {
		q = q.Where(`url LIKE ? ESCAPE '\'`, likeContains(filter.URLContains))
	}
	if filter.MessageContains != "" {
		q = q.Where(`message LIKE ? ESCAPE '\'`, likeContains(filter.MessageContains))
	}
	return q
}

// likeContains wraps a substring in a LIKE pattern, escaping the wildcard
// characters so user input matches literally. Queries pair it with
// ESCAPE '\'.
func likeContains(value string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
	return "%" + escaped + "%"
}

// IssueSort describes the ordering of an issue page. Field is a canonical
// column keyword (see issueSortColumns); an unknown or empty field falls back
// to the stored order (rowid), which is stable across pages.
type IssueSort struct {
	Field string
	Desc  bool
}

// issueSortColumns maps the sort keywords the API exposes to SQL. Severity,
// lifecycle and category order by their domain display order rather than
// lexically, so "desc" reads most severe/first-state first.
var issueSortColumns = map[string]string{
	"severity":   `CASE severity WHEN 'fatal' THEN 0 WHEN 'error' THEN 1 WHEN 'warning' THEN 2 WHEN 'notice' THEN 3 WHEN 'info' THEN 4 WHEN 'success' THEN 5 ELSE 6 END`,
	"lifecycle":  `CASE lifecycle WHEN 'new' THEN 0 WHEN 'open' THEN 1 WHEN 'resurfaced' THEN 2 WHEN 'degraded' THEN 3 WHEN 'improved' THEN 4 WHEN 'resolved' THEN 5 WHEN 'passed' THEN 6 ELSE 7 END`,
	"category":   `CASE category WHEN 'seo' THEN 0 WHEN 'security' THEN 1 WHEN 'performance' THEN 2 WHEN 'accessibility' THEN 3 WHEN 'headers' THEN 4 WHEN 'content' THEN 5 WHEN 'general' THEN 6 ELSE 7 END`,
	"check":      "check_name",
	"check_name": "check_name",
	"url":        "url",
	"message":    "message",
	"created_at": "created_at",
	"updated_at": "updated_at",
}

// applyIssueSort orders an issue query by the given sort. Unknown fields keep
// the stored order. rowid is appended as a tiebreaker so equal keys never
// reorder between pages.
func applyIssueSort(q *gorm.DB, sort IssueSort) *gorm.DB {
	column, ok := issueSortColumns[sort.Field]
	if !ok {
		return q.Order("rowid")
	}
	dir := "ASC"
	if sort.Desc {
		dir = "DESC"
	}
	return q.Order(column + " " + dir).Order("rowid")
}

// URLSort describes the ordering of an audited URL page. Field is a
// canonical column keyword (see urlSortColumns); an unknown or empty field
// falls back to the stored order (rowid), which is stable across pages.
type URLSort struct {
	Field string
	Desc  bool
}

// urlSortColumns maps the sort keywords the API exposes to SQL. State and
// highest severity order by their domain display order rather than
// lexically, so "desc" reads most severe/most recent-state first.
var urlSortColumns = map[string]string{
	"url":         "url",
	"title":       "title",
	"duration":    "duration_ms",
	"duration_ms": "duration_ms",
	"state":       `CASE state WHEN 'new' THEN 0 WHEN 'active' THEN 1 WHEN 'missing' THEN 2 ELSE 3 END`,
	"highest":     `CASE highest_severity WHEN 'fatal' THEN 0 WHEN 'error' THEN 1 WHEN 'warning' THEN 2 WHEN 'notice' THEN 3 WHEN 'info' THEN 4 WHEN 'success' THEN 5 ELSE 6 END`,
	"severity":    `CASE highest_severity WHEN 'fatal' THEN 0 WHEN 'error' THEN 1 WHEN 'warning' THEN 2 WHEN 'notice' THEN 3 WHEN 'info' THEN 4 WHEN 'success' THEN 5 ELSE 6 END`,
	"score":       "score",
	"status":      "status_code",
	"status_code": "status_code",
}

// applyURLSort orders an audited URL query by the given sort. Unknown fields
// keep the stored order. rowid is appended as a tiebreaker so equal keys
// never reorder between pages.
func applyURLSort(q *gorm.DB, sort URLSort) *gorm.DB {
	column, ok := urlSortColumns[sort.Field]
	if !ok {
		return q.Order("rowid")
	}
	dir := "ASC"
	if sort.Desc {
		dir = "DESC"
	}
	return q.Order(column + " " + dir).Order("rowid")
}

// applyURLFilter pushes the dimensions of an audited URL filter into the
// query: states and highest severities become IN sets and the duration (the
// stored millisecond column) and score bounds become inclusive range
// predicates. A zero bound is skipped, so the zero-value filter leaves the
// query untouched.
func applyURLFilter(q *gorm.DB, filter domain.URLFilter) *gorm.DB {
	if len(filter.States) > 0 {
		q = q.Where("state IN ?", stringValues(filter.States))
	}
	if len(filter.HighestSeverities) > 0 {
		q = q.Where("highest_severity IN ?", stringValues(filter.HighestSeverities))
	}
	if trimmed := strings.TrimSpace(filter.URLContains); trimmed != "" {
		q = q.Where("url LIKE ? ESCAPE '\\'", likeContains(trimmed))
	}
	if filter.DurationMin > 0 {
		q = q.Where("duration_ms >= ?", filter.DurationMin.Milliseconds())
	}
	if filter.DurationMax > 0 {
		q = q.Where("duration_ms <= ?", filter.DurationMax.Milliseconds())
	}
	if filter.ScoreMin > 0 {
		q = q.Where("score >= ?", filter.ScoreMin)
	}
	if filter.ScoreMax > 0 {
		q = q.Where("score <= ?", filter.ScoreMax)
	}
	return q
}

// stringValues converts a slice of string-backed enum values for use in a
// SQL IN clause.
func stringValues[T ~string](values []T) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return out
}

// IssueDistribution counts the issues of the audit by severity, honoring the
// given filter. Issues are read straight from the issues table: one row is
// one check result of one page, so the counts match the issues tab's
// severity widget and their total is the number of rows the tab would list.
func (s *AuditService) IssueDistribution(ctx context.Context, auditID string, filter domain.IssueFilter) (domain.SeverityCounts, error) {
	type severityRow struct {
		Severity string
		Count    int
	}
	q := applyIssueFilter(s.db.WithContext(ctx).Model(&store.Issue{}).
		Select("severity AS severity, count(*) AS count").
		Where("audit_id = ?", auditID), filter)

	var rows []severityRow
	if err := q.Group("severity").Scan(&rows).Error; err != nil {
		return domain.SeverityCounts{}, fmt.Errorf("count issues by severity: %w", err)
	}

	var counts domain.SeverityCounts
	for _, r := range rows {
		sev, err := domain.ParseSeverity(r.Severity)
		if err != nil {
			continue
		}
		for i := 0; i < r.Count; i++ {
			counts.Inc(sev)
		}
	}
	return counts, nil
}

// ListIssuesPage returns one page of issues of the audit in stored order
// (rowid), starting at the given offset and capped at limit rows, honoring
// the given filter. Every row is one check result of the page it was checked
// against; its URL is the page's identity URL (the final URL, falling back
// to the audited URL).
func (s *AuditService) ListIssuesPage(ctx context.Context, auditID string, filter domain.IssueFilter, limit, offset int) ([]*domain.Issue, error) {
	return s.ListIssuesPageSorted(ctx, auditID, filter, IssueSort{}, limit, offset)
}

// ListIssuesPageSorted returns one page of issues of the audit in the given
// sort order, starting at the given offset and capped at limit rows, honoring
// the given filter. Every row is one check result of the page it was checked
// against; its URL is the page's identity URL (the final URL, falling back to
// the audited URL).
func (s *AuditService) ListIssuesPageSorted(ctx context.Context, auditID string, filter domain.IssueFilter, sort IssueSort, limit, offset int) ([]*domain.Issue, error) {
	if limit <= 0 {
		limit = 250
	}
	if offset < 0 {
		offset = 0
	}

	q := applyIssueFilter(s.db.WithContext(ctx).Model(&store.Issue{}).
		Where("audit_id = ?", auditID), filter)

	var models []store.Issue
	if err := applyIssueSort(q, sort).Limit(limit).Offset(offset).Find(&models).Error; err != nil {
		return nil, fmt.Errorf("page issues for audit: %w", err)
	}

	issues := make([]*domain.Issue, 0, len(models))
	for i := range models {
		issues = append(issues, models[i].ToDomain())
	}
	return issues, nil
}

// issueBriefColumns are the columns a list row needs to render. The evidence
// column (the per-issue findings, which can be large) is deliberately left
// out so browsing a page never loads findings for issues the user has not
// opened; GetIssue loads them for a single issue on demand.
var issueBriefColumns = []string{
	"id", "url", "check_name", "category", "severity", "message",
	"prior_severity", "lifecycle", "created_at", "updated_at",
}

// ListIssuesPageBrief is ListIssuesPageSorted without the per-issue findings:
// the returned issues carry no Details. It backs list views whose detail pane
// loads a single issue separately.
func (s *AuditService) ListIssuesPageBrief(ctx context.Context, auditID string, filter domain.IssueFilter, sort IssueSort, limit, offset int) ([]*domain.Issue, error) {
	if limit <= 0 {
		limit = 250
	}
	if offset < 0 {
		offset = 0
	}

	q := applyIssueFilter(s.db.WithContext(ctx).Model(&store.Issue{}).
		Select(issueBriefColumns).
		Where("audit_id = ?", auditID), filter)

	var models []store.Issue
	if err := applyIssueSort(q, sort).Limit(limit).Offset(offset).Find(&models).Error; err != nil {
		return nil, fmt.Errorf("page issues for audit: %w", err)
	}

	issues := make([]*domain.Issue, 0, len(models))
	for i := range models {
		issues = append(issues, models[i].ToDomain())
	}
	return issues, nil
}

// GetIssue returns one stored issue with its findings. The issue is addressed
// by its audit id and issue id so a stale row of another audit cannot be
// served.
func (s *AuditService) GetIssue(ctx context.Context, auditID, issueID string) (*domain.Issue, error) {
	var model store.Issue
	err := s.db.WithContext(ctx).
		Where("audit_id = ? AND id = ?", auditID, issueID).
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrIssueNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get issue: %w", err)
	}
	return model.ToDomain(), nil
}

// CountIssues returns how many issues of the audit match the filter.
func (s *AuditService) CountIssues(ctx context.Context, auditID string, filter domain.IssueFilter) (int, error) {
	var count int64
	q := applyIssueFilter(s.db.WithContext(ctx).Model(&store.Issue{}).
		Where("audit_id = ?", auditID), filter)
	if err := q.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count issues: %w", err)
	}
	return int(count), nil
}

// CheckFilter narrows a check summary query. NameContains matches the check
// name case-insensitively; Categories and Highest are OR-sets; the count
// minimums keep checks whose matching severity count is at least the given
// value. A zero minimum places no constraint.
type CheckFilter struct {
	NameContains string
	Categories   []domain.Category
	Highest      []domain.Severity
	FatalMin     int
	ErrorMin     int
	WarningMin   int
	NoticeMin    int
	SuccessMin   int
}

// CheckSort describes the ordering of a check summary list. An unknown or
// empty field orders by name.
type CheckSort struct {
	Field string
	Desc  bool
}

// CheckPage is one page of a check summary query together with the totals of
// the whole filtered set: how many checks matched and how many of them had
// each severity as their highest.
type CheckPage struct {
	Items         []*domain.CheckSummary
	Total         int
	HighestCounts domain.SeverityCounts
}

// ListCheckSummariesPage returns one page of the audit's check summaries
// filtered and sorted as requested, plus the filtered-set totals.
func (s *AuditService) ListCheckSummariesPage(ctx context.Context, auditID string, filter CheckFilter, sortSpec CheckSort, limit, offset int) (CheckPage, error) {
	summaries, err := s.CheckSummaries(ctx, auditID)
	if err != nil {
		return CheckPage{}, err
	}

	filtered := applyCheckFilter(summaries, filter)
	applyCheckSort(filtered, sortSpec)

	total := len(filtered)
	var highestCounts domain.SeverityCounts
	for _, c := range filtered {
		highestCounts.Add(c.Severity.Highest(), 1)
	}

	if limit <= 0 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return CheckPage{
		Items:         filtered[offset:end],
		Total:         total,
		HighestCounts: highestCounts,
	}, nil
}

func applyCheckFilter(summaries []*domain.CheckSummary, filter CheckFilter) []*domain.CheckSummary {
	name := strings.ToLower(strings.TrimSpace(filter.NameContains))
	cats := make(map[domain.Category]struct{}, len(filter.Categories))
	for _, c := range filter.Categories {
		cats[c] = struct{}{}
	}
	highest := make(map[domain.Severity]struct{}, len(filter.Highest))
	for _, h := range filter.Highest {
		highest[h] = struct{}{}
	}

	out := make([]*domain.CheckSummary, 0, len(summaries))
	for _, c := range summaries {
		if name != "" && !strings.Contains(strings.ToLower(c.Name), name) {
			continue
		}
		if len(cats) > 0 {
			if _, ok := cats[c.Category]; !ok {
				continue
			}
		}
		if len(highest) > 0 {
			if _, ok := highest[c.Severity.Highest()]; !ok {
				continue
			}
		}
		if c.Severity.Fatal < filter.FatalMin ||
			c.Severity.Error < filter.ErrorMin ||
			c.Severity.Warning < filter.WarningMin ||
			c.Severity.Notice < filter.NoticeMin ||
			c.Severity.Success < filter.SuccessMin {
			continue
		}
		out = append(out, c)
	}
	return out
}

func applyCheckSort(summaries []*domain.CheckSummary, sortSpec CheckSort) {
	sort.SliceStable(summaries, func(i, j int) bool {
		a, b := summaries[i], summaries[j]
		var c int
		switch sortSpec.Field {
		case "category":
			c = categoryRank(a.Category) - categoryRank(b.Category)
			if c == 0 {
				c = strings.Compare(a.Name, b.Name)
			}
		case "total":
			c = a.Total - b.Total
		case "fatal":
			c = a.Severity.Fatal - b.Severity.Fatal
		case "error":
			c = a.Severity.Error - b.Severity.Error
		case "warning":
			c = a.Severity.Warning - b.Severity.Warning
		case "notice":
			c = a.Severity.Notice - b.Severity.Notice
		case "success":
			c = a.Severity.Success - b.Severity.Success
		case "highest":
			c = a.Severity.Highest().Weight() - b.Severity.Highest().Weight()
		default:
			c = strings.Compare(a.Name, b.Name)
		}
		if sortSpec.Desc {
			return c > 0
		}
		return c < 0
	})
}

// CheckSummaries groups the stored issues of the audit by check and returns
// one summary per check with its category, total issue count and severity
// distribution. Checks are ordered by category and then by name.
func (s *AuditService) CheckSummaries(ctx context.Context, auditID string) ([]*domain.CheckSummary, error) {
	type checkRow struct {
		CheckName string
		Category  string
		Severity  string
		Count     int
	}
	var rows []checkRow
	if err := s.db.WithContext(ctx).Model(&store.Issue{}).
		Select("check_name AS check_name, category AS category, severity AS severity, count(*) AS count").
		Where("audit_id = ?", auditID).
		Group("check_name, category, severity").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("summarize checks: %w", err)
	}

	byName := make(map[string]*domain.CheckSummary, len(rows))
	summaries := make([]*domain.CheckSummary, 0, len(rows))
	for _, r := range rows {
		summary := byName[r.CheckName]
		if summary == nil {
			summary = &domain.CheckSummary{Name: r.CheckName, Category: domain.Category(r.Category)}
			byName[r.CheckName] = summary
			summaries = append(summaries, summary)
		}
		summary.Total += r.Count
		if sev, err := domain.ParseSeverity(r.Severity); err == nil {
			summary.Severity.Add(sev, r.Count)
		}
	}

	sort.SliceStable(summaries, func(i, j int) bool {
		ri, rj := categoryRank(summaries[i].Category), categoryRank(summaries[j].Category)
		if ri != rj {
			return ri < rj
		}
		return summaries[i].Name < summaries[j].Name
	})
	return summaries, nil
}

// categoryRank returns the display order of a category, with unknown
// categories sorted last.
func categoryRank(c domain.Category) int {
	for i, cat := range domain.AllCategories {
		if cat == c {
			return i
		}
	}
	return len(domain.AllCategories)
}

// IssueCheckNames returns the distinct check names observed in the stored
// issues of the audit, ordered by name. The issues tab uses them as the
// options of its check filter.
func (s *AuditService) IssueCheckNames(ctx context.Context, auditID string) ([]string, error) {
	var names []string
	if err := s.db.WithContext(ctx).Model(&store.Issue{}).
		Where("audit_id = ?", auditID).
		Distinct().
		Order("check_name").
		Pluck("check_name", &names).Error; err != nil {
		return nil, fmt.Errorf("list issue check names: %w", err)
	}
	return names, nil
}

// DashboardMetrics aggregates the state an audit dashboard shows without
// loading its URL and issue rows: URL state counts and the severity and
// lifecycle distributions over every stored issue of the audit.
func (s *AuditService) DashboardMetrics(ctx context.Context, auditID string) (*domain.AuditMetrics, error) {
	type sevLifeRow struct {
		Severity  string
		Lifecycle string
		Count     int
	}
	var rows []sevLifeRow
	if err := s.db.WithContext(ctx).Table("issues").
		Select("severity AS severity, lifecycle AS lifecycle, count(*) AS count").
		Where("audit_id = ?", auditID).
		Group("severity, lifecycle").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("aggregate issue metrics: %w", err)
	}

	metrics := &domain.AuditMetrics{}
	for _, r := range rows {
		if sev, err := domain.ParseSeverity(r.Severity); err == nil {
			for i := 0; i < r.Count; i++ {
				metrics.Severity.Inc(sev)
			}
		}
		switch domain.IssueLifecycle(r.Lifecycle) {
		case domain.LifecycleNew:
			metrics.Lifecycle.New += r.Count
		case domain.LifecycleOpen:
			metrics.Lifecycle.Open += r.Count
		case domain.LifecycleResurfaced:
			metrics.Lifecycle.Resurfaced += r.Count
		case domain.LifecycleResolved:
			metrics.Lifecycle.Resolved += r.Count
		case domain.LifecycleImproved:
			metrics.Lifecycle.Improved += r.Count
		case domain.LifecycleDegraded:
			metrics.Lifecycle.Degraded += r.Count
		case domain.LifecyclePassed:
			metrics.Lifecycle.Passed += r.Count
		}
	}

	type stateRow struct {
		State string
		Count int
	}
	var states []stateRow
	if err := s.db.WithContext(ctx).Model(&store.AuditedUrl{}).
		Select("state AS state, count(*) AS count").
		Where("audit_id = ?", auditID).
		Group("state").
		Scan(&states).Error; err != nil {
		return nil, fmt.Errorf("count audited url states: %w", err)
	}
	for _, r := range states {
		switch domain.UrlState(r.State) {
		case domain.UrlStateNew:
			metrics.URLStates.New += r.Count
		case domain.UrlStateActive:
			metrics.URLStates.Active += r.Count
		case domain.UrlStateMissing:
			metrics.URLStates.Missing += r.Count
		}
	}
	return metrics, nil
}

// URLAggregates computes the aggregate state of the audited URL rows of the
// audit that match the given filter: per-state counts plus the average
// duration and score across those rows, matching the urls tab's summary
// widget for its current filter.
func (s *AuditService) URLAggregates(ctx context.Context, auditID string, filter domain.URLFilter) (*domain.URLAggregates, error) {
	var rows []store.AuditedUrl
	q := applyURLFilter(s.db.WithContext(ctx).
		Select("state", "duration_ms", "score").
		Where("audit_id = ?", auditID), filter)
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("aggregate audited urls: %w", err)
	}

	agg := &domain.URLAggregates{}
	var durationSum int64
	var scoreSum float64
	for _, row := range rows {
		switch domain.UrlState(row.State) {
		case domain.UrlStateNew:
			agg.URLStates.New++
		case domain.UrlStateActive:
			agg.URLStates.Active++
		case domain.UrlStateMissing:
			agg.URLStates.Missing++
		}
		durationSum += row.DurationMs
		scoreSum += row.Score
	}
	if n := len(rows); n > 0 {
		agg.AvgDuration = time.Duration(durationSum/int64(n)) * time.Millisecond
		agg.AvgScore = scoreSum / float64(n)
	}
	return agg, nil
}

// ListURLsPage returns one page of the audited URL rows of the audit that
// match the given filter, in stored order (rowid), starting at the given
// offset and capped at limit rows, plus the total number of matching rows.
// No issues are attached; use URLIssues to load the issues of the selected
// URL.
func (s *AuditService) ListURLsPage(ctx context.Context, auditID string, filter domain.URLFilter, limit, offset int) ([]*domain.AuditedUrl, int, error) {
	return s.ListURLsPageSorted(ctx, auditID, filter, URLSort{}, limit, offset)
}

// ListURLsPageSorted is ListURLsPage with an explicit ordering. Unknown sort
// fields fall back to the stored order (rowid).
func (s *AuditService) ListURLsPageSorted(ctx context.Context, auditID string, filter domain.URLFilter, sort URLSort, limit, offset int) ([]*domain.AuditedUrl, int, error) {
	if limit <= 0 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	var total int64
	countQuery := applyURLFilter(s.db.WithContext(ctx).Model(&store.AuditedUrl{}).
		Where("audit_id = ?", auditID), filter)
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count audited urls: %w", err)
	}

	var models []store.AuditedUrl
	pageQuery := applyURLFilter(s.db.WithContext(ctx).
		Where("audit_id = ?", auditID), filter)
	if err := applyURLSort(pageQuery, sort).
		Limit(limit).Offset(offset).
		Find(&models).Error; err != nil {
		return nil, 0, fmt.Errorf("page audited urls: %w", err)
	}

	urls := make([]*domain.AuditedUrl, 0, len(models))
	for i := range models {
		urls = append(urls, models[i].ToDomain())
	}
	return urls, int(total), nil
}

// URLIssues returns the stored issues of the given audited URL of the audit.
// Issue rows are addressed by the identity URL of the page that was checked
// (the URL's final URL, falling back to its raw URL), so the issues of a
// page are returned whichever audited URL reached it.
func (s *AuditService) URLIssues(ctx context.Context, auditID, url string) ([]*domain.Issue, error) {
	if url == "" {
		return nil, domain.ErrInvalidAudit
	}

	var row store.AuditedUrl
	if err := s.db.WithContext(ctx).
		Where("audit_id = ? AND url = ?", auditID, url).
		First(&row).Error; err != nil {
		return nil, fmt.Errorf("load audited url %q: %w", url, err)
	}

	identity := row.FinalURL
	if identity == "" {
		identity = row.URL
	}

	var models []store.Issue
	if err := s.db.WithContext(ctx).
		Where("audit_id = ? AND url = ?", auditID, identity).
		Order("rowid").
		Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list issues of url %q: %w", url, err)
	}

	issues := make([]*domain.Issue, 0, len(models))
	for i := range models {
		issues = append(issues, models[i].ToDomain())
	}
	return issues, nil
}

// ResolveRecheckTarget maps the page URL of an issue to the audited URL row
// it belongs to, so a recheck re-audits the configured target that reached
// the page. Pages reached directly are their own target; pages reached
// through a redirect resolve to the first audited URL that landed on them.
func (s *AuditService) ResolveRecheckTarget(ctx context.Context, auditID, pageURL string) (string, error) {
	var row store.AuditedUrl
	err := s.db.WithContext(ctx).
		Where("audit_id = ? AND url = ?", auditID, pageURL).
		First(&row).Error
	if err == nil {
		return row.URL, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("resolve recheck target for %q: %w", pageURL, err)
	}

	err = s.db.WithContext(ctx).
		Where("audit_id = ? AND final_url = ?", auditID, pageURL).
		Order("rowid").
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", fmt.Errorf("page %q is not part of audit %s", pageURL, auditID)
		}
		return "", fmt.Errorf("resolve recheck target for %q: %w", pageURL, err)
	}
	return row.URL, nil
}

// DistinctIssueChecks counts the distinct checks observed in the stored
// issues of the audit. It is the fallback for the dashboard's check count on
// audits whose record predates stored check names.
func (s *AuditService) DistinctIssueChecks(ctx context.Context, auditID string) (int, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&store.Issue{}).
		Where("audit_id = ?", auditID).
		Distinct("check_name").
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count distinct checks: %w", err)
	}
	return int(count), nil
}
