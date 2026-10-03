package service

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/vvb13a/goaudit/store"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ValidationTarget is a link target to validate together with the filetype
// hint from the referencing element. The hint classifies extensionless CDN
// URLs (e.g. a script loaded from /static/app) so validation can pace assets
// differently from documents.
type ValidationTarget struct {
	URL      string
	Filetype string
}

// LinkValidator validates the graph's target nodes in bulk and stores the
// results in the global link_targets table. A target is only fetched when its
// stored result is missing or older than the configured TTL, so repeated runs
// (and other audits) reuse it.
type LinkValidator struct {
	db *gorm.DB
}

func NewLinkValidator(db *gorm.DB) *LinkValidator {
	return &LinkValidator{db: db}
}

// Validate fetches the given target URLs in bulk, skipping any whose stored
// result is still fresh, and upserts the results into the global link_targets
// table. It returns the resulting status keyed by URL (fresh, reused or
// previously stored) so the caller can run graph checks without re-reading.
func (v *LinkValidator) Validate(ctx context.Context, cfg Config, targets []ValidationTarget, onProgress func(completed, total int)) (map[string]store.LinkTarget, error) {
	statuses := make(map[string]store.LinkTarget)
	if v == nil || v.db == nil || len(targets) == 0 {
		if onProgress != nil {
			onProgress(0, 0)
		}
		return statuses, nil
	}

	unique := dedupeTargets(targets)
	uniqueURLs := make([]string, len(unique))
	for i := range unique {
		uniqueURLs[i] = unique[i].URL
	}
	now := time.Now().UTC()
	ttl := cfg.LinkCacheTTL()

	fresh := make(map[string]struct{})
	if ttl > 0 {
		for _, chunk := range chunkStrings(uniqueURLs, 400) {
			var freshURLs []string
			if err := v.db.WithContext(ctx).Model(&store.LinkTarget{}).
				Where("url IN ? AND expires_at > ?", chunk, now).
				Pluck("url", &freshURLs).Error; err != nil {
				return nil, err
			}
			for _, u := range freshURLs {
				fresh[u] = struct{}{}
			}
		}
	}

	pending := make([]ValidationTarget, 0, len(unique))
	for _, t := range unique {
		if _, ok := fresh[t.URL]; !ok {
			pending = append(pending, t)
		}
	}

	// A target that is also a page the checks workflow already fetched (this or
	// another audit) is validated already: reuse its stored status instead of
	// re-fetching the document, and seed it into link_targets so it is shared.
	if len(pending) > 0 && ttl > 0 {
		reused, err := v.reuseAudited(ctx, targetURLs(pending), ttl, now)
		if err != nil {
			return nil, err
		}
		if len(reused) > 0 {
			remaining := make([]ValidationTarget, 0, len(pending))
			for _, t := range pending {
				if _, ok := reused[t.URL]; ok {
					continue
				}
				remaining = append(remaining, t)
			}
			pending = remaining
		}
	}

	total := len(pending)
	if onProgress != nil {
		onProgress(0, total)
	}

	if total > 0 {
		client := &http.Client{
			Timeout: cfg.HTTPTimeout(),
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		}

		// Assets and documents are paced independently so a large asset graph
		// does not throttle, and is not throttled by, the document targets.
		docConcurrency := cfg.MaxConcurrency
		if docConcurrency <= 0 {
			docConcurrency = 5
		}
		assetConcurrency := cfg.AssetMaxConcurrency
		if assetConcurrency <= 0 {
			assetConcurrency = 20
		}
		docSem := make(chan struct{}, docConcurrency)
		assetSem := make(chan struct{}, assetConcurrency)

		var (
			wg        sync.WaitGroup
			mu        sync.Mutex
			completed int
			writeErr  error
		)

		for _, target := range pending {
			if ctx.Err() != nil {
				break
			}
			filetype := detectFiletype(target.URL, target.Filetype)
			sem := docSem
			delay := cfg.RequestDelayFor(filetype)
			if isStaticAssetFiletype(filetype) {
				sem = assetSem
			}
			wg.Add(1)
			sem <- struct{}{}
			go func(u string, sem chan struct{}, delay time.Duration) {
				defer wg.Done()
				defer func() { <-sem }()
				if delay > 0 {
					time.Sleep(delay)
				}

				result := v.fetch(ctx, client, cfg.UserAgent, u)

				mu.Lock()
				defer mu.Unlock()
				if writeErr == nil {
					if err := v.store(ctx, result, ttl); err != nil {
						writeErr = err
					}
				}
				completed++
				if onProgress != nil {
					onProgress(completed, total)
				}
			}(target.URL, sem, delay)
		}
		wg.Wait()

		if writeErr != nil {
			return nil, writeErr
		}
	}

	// Load every result (fetched, reused or pre-existing) for the caller.
	for _, chunk := range chunkStrings(uniqueURLs, 400) {
		var rows []store.LinkTarget
		if err := v.db.WithContext(ctx).Where("url IN ?", chunk).Find(&rows).Error; err != nil {
			return nil, err
		}
		for i := range rows {
			statuses[rows[i].URL] = rows[i]
		}
	}
	return statuses, ctx.Err()
}

// dedupeTargets returns the targets with duplicate URLs removed, preserving
// order. When a duplicate carries a filetype hint the first one lacks, the
// hint is filled in.
func dedupeTargets(targets []ValidationTarget) []ValidationTarget {
	idx := make(map[string]int, len(targets))
	out := make([]ValidationTarget, 0, len(targets))
	for _, t := range targets {
		if t.URL == "" {
			continue
		}
		if i, ok := idx[t.URL]; ok {
			if out[i].Filetype == "" {
				out[i].Filetype = t.Filetype
			}
			continue
		}
		idx[t.URL] = len(out)
		out = append(out, t)
	}
	return out
}

// targetURLs extracts the URLs of the given targets, preserving order.
func targetURLs(targets []ValidationTarget) []string {
	urls := make([]string, len(targets))
	for i := range targets {
		urls[i] = targets[i].URL
	}
	return urls
}

// chunkStrings splits values into slices of at most size.
func chunkStrings(values []string, size int) [][]string {
	var chunks [][]string
	for start := 0; start < len(values); start += size {
		end := start + size
		if end > len(values) {
			end = len(values)
		}
		chunks = append(chunks, values[start:end])
	}
	return chunks
}

// reuseAudited seeds link_targets from audited_urls for the given target URLs
// whose last fetch is within the TTL, and returns the set of URLs that were
// reused. It matches on either the fetched URL or its final URL, globally
// across audits, so a document fetched once serves every graph that references
// it.
func (v *LinkValidator) reuseAudited(ctx context.Context, urls []string, ttl time.Duration, now time.Time) (map[string]struct{}, error) {
	reused := make(map[string]struct{})
	if ttl <= 0 || len(urls) == 0 {
		return reused, nil
	}

	type auditedRow struct {
		URL           string
		FinalURL      string
		StatusCode    int64
		LastAuditedAt time.Time
	}
	var rows []auditedRow
	for _, chunk := range chunkStrings(urls, 400) {
		var part []auditedRow
		if err := v.db.WithContext(ctx).Table("audited_urls").
			Select("url, final_url, status_code, last_audited_at").
			Where("(url IN ? OR final_url IN ?)", chunk, chunk).
			Where("last_audited_at > ?", now.Add(-ttl)).
			Scan(&part).Error; err != nil {
			return nil, err
		}
		rows = append(rows, part...)
	}
	if len(rows) == 0 {
		return reused, nil
	}

	byKey := make(map[string]auditedRow, len(rows)*2)
	for _, row := range rows {
		byKey[row.URL] = row
		if row.FinalURL != "" {
			byKey[row.FinalURL] = row
		}
	}

	for _, u := range urls {
		row, ok := byKey[u]
		if !ok {
			continue
		}
		result := store.LinkTarget{
			URL:         u,
			StatusCode:  int(row.StatusCode),
			FinalURL:    row.FinalURL,
			ValidatedAt: row.LastAuditedAt,
		}
		if err := v.store(ctx, result, ttl); err != nil {
			return nil, err
		}
		reused[u] = struct{}{}
	}
	return reused, nil
}

// fetch performs a GET (HEAD is avoided because many servers reject it) and
// records the status and final URL.
func (v *LinkValidator) fetch(ctx context.Context, client *http.Client, userAgent, target string) store.LinkTarget {
	result := store.LinkTarget{URL: target, ValidatedAt: time.Now().UTC()}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	req.Header.Set("X-Audit-Engine", "true")

	resp, err := client.Do(req)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	if resp.Request != nil && resp.Request.URL != nil {
		result.FinalURL = resp.Request.URL.String()
	}
	// Drain a bounded amount so the connection can be reused without pulling
	// whole large assets.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
	return result
}

// store upserts one validation result, stamping its expiry from the TTL. A
// non-positive TTL means the result is always considered stale.
func (v *LinkValidator) store(ctx context.Context, result store.LinkTarget, ttl time.Duration) error {
	if ttl > 0 {
		result.ExpiresAt = result.ValidatedAt.Add(ttl)
	} else {
		result.ExpiresAt = time.Time{}
	}
	return v.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "url"}},
		UpdateAll: true,
	}).Create(&result).Error
}
