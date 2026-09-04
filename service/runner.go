package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vvb13a/goaudit/domain"
)

type ProgressCallback func(currentURL string, completed int, total int)

// Runner executes audits. It is stateless: each run builds its own fetcher
// and pacing settings from the effective Config of the audit being run, so
// every audit can carry its own engine configuration.
type Runner struct{}

func NewRunner() *Runner {
	return &Runner{}
}

// ExecuteAudit runs every check against each resolved target and returns the
// resulting audit. cfg is the effective configuration of the audit (already
// resolved from the audit record over the app-level defaults). The audit is
// self-contained: it stores the given name, description, the targets as
// entered, the names of the checks that ran and the resolved config.
func (r *Runner) ExecuteAudit(
	ctx context.Context,
	name string,
	description string,
	targets []string,
	checks []domain.Check,
	cfg Config,
	onProgress ProgressCallback,
) (*domain.Audit, error) {
	concurrency := cfg.MaxConcurrency
	if concurrency <= 0 {
		concurrency = 5
	}
	maxDepth := cfg.MaxSitemapDepth
	if maxDepth <= 0 {
		maxDepth = 3
	}

	fetcher := NewFetcher().
		WithTimeout(cfg.HTTPTimeout()).
		WithUserAgent(cfg.UserAgent).
		WithHeader("X-Audit-Engine", "true")

	resolvedURLs, err := r.resolveTargets(ctx, fetcher, maxDepth, targets)
	if err != nil {
		return nil, fmt.Errorf("resolve targets: %w", err)
	}

	total := len(resolvedURLs)
	if total == 0 {
		return nil, fmt.Errorf("no target URLs found to audit")
	}

	if name == "" {
		name = "Audit"
	}

	configRaw, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("encode audit config: %w", err)
	}

	audit := &domain.Audit{
		ID:          fmt.Sprintf("aud_%d", time.Now().UnixNano()),
		Name:        name,
		Description: description,
		Targets:     targets,
		CheckNames:  checkNames(checks),
		Config:      configRaw,
		StartedAt:   time.Now().UTC(),
		Reports:     make([]*domain.Report, total),
	}

	if onProgress != nil {
		onProgress("Starting audit...", 0, total)
	}

	var (
		wg        sync.WaitGroup
		completed int64
		semaphore = make(chan struct{}, concurrency)
		delay     = cfg.RequestDelay()
	)

	for i, targetURL := range resolvedURLs {
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		semaphore <- struct{}{}

		go func(idx int, u string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			if ctx.Err() != nil {
				return
			}

			if delay > 0 {
				time.Sleep(delay)
			}

			report := r.auditURL(ctx, fetcher, u, checks)
			audit.Reports[idx] = report

			currentCompleted := int(atomic.AddInt64(&completed, 1))
			if onProgress != nil {
				onProgress(u, currentCompleted, total)
			}
		}(i, targetURL)
	}

	wg.Wait()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	cleanReports := make([]*domain.Report, 0, len(audit.Reports))
	for _, rep := range audit.Reports {
		if rep != nil {
			cleanReports = append(cleanReports, rep)
		}
	}
	audit.Reports = cleanReports

	audit.Duration = time.Since(audit.StartedAt)
	audit.CalculateSummary()

	return audit, nil
}

// checkNames returns the names of the given checks in order.
func checkNames(checks []domain.Check) []string {
	names := make([]string, 0, len(checks))
	for _, c := range checks {
		names = append(names, c.Info().Name)
	}
	return names
}

func (r *Runner) auditURL(ctx context.Context, fetcher *Fetcher, targetURL string, checks []domain.Check) *domain.Report {
	report := &domain.Report{
		URL: targetURL,
	}

	doc, err := fetcher.Fetch(ctx, targetURL)
	if err != nil {
		report.Issues = []domain.Issue{
			domain.NewRawIssue(
				"http_fetch_success",
				domain.CategoryGeneral,
				domain.SeverityFatal,
				fmt.Sprintf("Failed to fetch page: %v", err),
				nil,
			),
		}
		report.CalculateSummary()
		return report
	}

	report.FinalURL = doc.FinalURL
	report.StatusCode = doc.StatusCode
	report.Duration = doc.Duration

	for _, check := range checks {
		if !check.Supports(doc) {
			continue
		}

		issue := check.Apply(ctx, doc)
		report.Issues = append(report.Issues, issue)
	}

	report.CalculateSummary()
	return report
}

// resolveTargets expands the raw target list into concrete URLs, resolving
// sitemap entries with the given fetcher and depth limit.
func (r *Runner) resolveTargets(ctx context.Context, fetcher *Fetcher, maxDepth int, rawURLs []string) ([]string, error) {
	var resolved []string
	seen := make(map[string]struct{})
	sitemapParser := NewSitemapParser(fetcher, maxDepth)

	for _, targetURL := range rawURLs {
		if IsSitemapURL(targetURL) {
			sitemapURLs, err := sitemapParser.Parse(ctx, targetURL)
			if err != nil {
				return nil, fmt.Errorf("parse sitemap '%s': %w", targetURL, err)
			}
			for _, u := range sitemapURLs {
				if _, exists := seen[u]; !exists {
					seen[u] = struct{}{}
					resolved = append(resolved, u)
				}
			}
		} else {
			if _, exists := seen[targetURL]; !exists {
				seen[targetURL] = struct{}{}
				resolved = append(resolved, targetURL)
			}
		}
	}

	return resolved, nil
}
