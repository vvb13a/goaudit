package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vvb13a/goaudit/domain"
)

type ProgressCallback func(currentURL string, completed int, total int)

type RunnerConfig struct {
	Concurrency     int
	RequestDelay    time.Duration
	MaxSitemapDepth int
}

func DefaultRunnerConfig() RunnerConfig {
	return RunnerConfig{
		Concurrency:     5,
		RequestDelay:    0,
		MaxSitemapDepth: 3,
	}
}

type Runner struct {
	fetcher *Fetcher
	cfg     RunnerConfig
}

func NewRunner(fetcher *Fetcher, cfg RunnerConfig) *Runner {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 5
	}
	if cfg.MaxSitemapDepth <= 0 {
		cfg.MaxSitemapDepth = 3
	}
	return &Runner{
		fetcher: fetcher,
		cfg:     cfg,
	}
}

// ExecuteAudit runs every check against each resolved target and returns the
// resulting audit. The audit is self-contained: it stores the given name, the
// targets as entered and the names of the checks that ran.
func (r *Runner) ExecuteAudit(
	ctx context.Context,
	name string,
	targets []string,
	checks []domain.Check,
	onProgress ProgressCallback,
) (*domain.Audit, error) {
	resolvedURLs, err := r.ResolveURLs(ctx, targets)
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

	audit := &domain.Audit{
		ID:         fmt.Sprintf("aud_%d", time.Now().UnixNano()),
		Name:       name,
		Targets:    targets,
		CheckNames: checkNames(checks),
		StartedAt:  time.Now().UTC(),
		Reports:    make([]*domain.Report, total),
	}

	if onProgress != nil {
		onProgress("Starting audit...", 0, total)
	}

	var (
		wg        sync.WaitGroup
		completed int64
		semaphore = make(chan struct{}, r.cfg.Concurrency)
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

			if r.cfg.RequestDelay > 0 {
				time.Sleep(r.cfg.RequestDelay)
			}

			report := r.AuditURL(ctx, u, checks)
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

func (r *Runner) AuditURL(ctx context.Context, targetURL string, checks []domain.Check) *domain.Report {
	report := &domain.Report{
		URL: targetURL,
	}

	doc, err := r.fetcher.Fetch(ctx, targetURL)
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

func (r *Runner) ResolveURLs(ctx context.Context, rawURLs []string) ([]string, error) {
	var resolved []string
	seen := make(map[string]struct{})
	sitemapParser := NewSitemapParser(r.fetcher, r.cfg.MaxSitemapDepth)

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
