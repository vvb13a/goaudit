package engine

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vvb13a/goaudit/config"
	"github.com/vvb13a/goaudit/data"
)

type ProgressCallback func(currentURL string, completed int, total int)

type Runner struct {
	fetcher *Fetcher
	checks  []data.Check
	cfg     *config.Config
}

func NewRunner(fetcher *Fetcher, cfg *config.Config, checks ...data.Check) *Runner {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	return &Runner{
		fetcher: fetcher,
		cfg:     cfg,
		checks:  checks,
	}
}

func (r *Runner) AuditPlan(
	ctx context.Context,
	planName string,
	rawURLs []string,
	checklistName string,
	onProgress ProgressCallback,
) (*data.Audit, error) {
	resolvedURLs, err := r.ResolveURLs(ctx, rawURLs)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve targets: %w", err)
	}

	total := len(resolvedURLs)
	if total == 0 {
		return nil, fmt.Errorf("no URLs found to audit")
	}

	startedAt := time.Now()
	var (
		reports     = make([]*data.Report, total)
		mu          sync.Mutex
		wg          sync.WaitGroup
		completed   int64
		firstErr    error
		workerLimit = r.cfg.MaxConcurrency
		semaphore   = make(chan struct{}, workerLimit)
	)

	// Notify initial state
	if onProgress != nil {
		onProgress("Starting audit...", 0, total)
	}

	// 2. Concurrent worker pool
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

			if r.cfg.RequestDelayMs > 0 {
				time.Sleep(r.cfg.RequestDelay())
			}

			if onProgress != nil {
				count := int(atomic.LoadInt64(&completed))
				onProgress(u, count, total)
			}

			rep, err := r.Audit(ctx, u)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("error auditing %s: %w", u, err)
				}
				mu.Unlock()
				return
			}

			reports[idx] = rep
			currentCompleted := int(atomic.AddInt64(&completed, 1))

			if onProgress != nil {
				onProgress(u, currentCompleted, total)
			}
		}(i, targetURL)
	}

	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	var (
		passedCount int
		failedCount int
		highestSev  = data.SeveritySuccess
		validReps   []*data.Report
	)

	for _, rep := range reports {
		if rep == nil {
			continue
		}
		validReps = append(validReps, rep)
		passedCount += rep.Summary.PassedCount
		failedCount += rep.Summary.FailedCount
		if rep.Summary.HighestSeverity.IsHigherThan(highestSev) {
			highestSev = rep.Summary.HighestSeverity
		}
	}

	return &data.Audit{
		PlanName:        planName,
		ChecklistName:   checklistName,
		StartedAt:       startedAt,
		Duration:        time.Since(startedAt),
		TotalEndpoints:  len(validReps),
		PassedCount:     passedCount,
		FailedCount:     failedCount,
		HighestSeverity: highestSev,
		Reports:         validReps,
	}, nil
}

func (r *Runner) ResolveURLs(ctx context.Context, rawURLs []string) ([]string, error) {
	var resolved []string
	seen := make(map[string]struct{})

	for _, targetURL := range rawURLs {
		if IsSitemapURL(targetURL) {
			sitemapURLs, err := ParseSitemap(ctx, r.fetcher, targetURL, r.cfg.MaxSitemapDepth)
			if err != nil {
				return nil, fmt.Errorf("error resolving sitemap '%s': %w", targetURL, err)
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

func (r *Runner) Audit(ctx context.Context, targetURL string) (*data.Report, error) {
	doc, err := r.fetcher.Fetch(ctx, targetURL)
	if err != nil {
		return nil, err
	}

	var (
		issues       []data.Issue
		skippedCount int
		mu           sync.Mutex
		wg           sync.WaitGroup
	)

	for _, check := range r.checks {
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		go func(c data.Check) {
			defer wg.Done()

			if ctx.Err() != nil {
				return
			}

			if !c.Supports(doc) {
				mu.Lock()
				skippedCount++
				mu.Unlock()
				return
			}

			results := c.Apply(ctx, doc)
			mu.Lock()
			issues = append(issues, results...)
			mu.Unlock()
		}(check)
	}

	wg.Wait()

	var (
		passedCount int
		failedCount int
		highestSev  = data.SeveritySuccess
	)

	for _, issue := range issues {
		if issue.Passed {
			passedCount++
		} else {
			failedCount++
			if issue.Severity.IsHigherThan(highestSev) {
				highestSev = issue.Severity
			}
		}
	}

	return &data.Report{
		URL:        doc.URL,
		FinalURL:   doc.FinalURL,
		StatusCode: doc.StatusCode,
		Duration:   doc.Duration,
		Summary: data.Summary{
			TotalIssues:     len(issues),
			PassedCount:     passedCount,
			SkippedCount:    skippedCount,
			FailedCount:     failedCount,
			HighestSeverity: highestSev,
		},
		Issues: issues,
	}, nil
}
