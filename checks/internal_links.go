package checks

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/vvb13a/goaudit/domain"
	"github.com/vvb13a/goaudit/service"

	"golang.org/x/net/html"
)

type InternalLinksCheck struct {
	Severity       domain.Severity
	Timeout        time.Duration
	MaxConcurrency int
	Client         *http.Client
	Cache          *service.LinkCache
}

func NewInternalLinksCheck(cache *service.LinkCache) *InternalLinksCheck {
	if cache == nil {
		cache = service.NewLinkCache(10 * time.Minute)
	}
	timeout := 5 * time.Second

	return &InternalLinksCheck{
		Severity:       domain.SeverityError,
		Timeout:        timeout,
		MaxConcurrency: 10,
		Cache:          cache,
		Client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("stopped after 10 redirects")
				}
				return nil
			},
		},
	}
}

func (c *InternalLinksCheck) Info() domain.CheckInfo {
	return domain.CheckInfo{
		Name:        "internal_links",
		Description: "Checks that in-page links to the same site resolve successfully.",
		Category:    domain.CategoryGeneral,
	}
}

func (c *InternalLinksCheck) Supports(doc *domain.Document) bool {
	return doc.IsHTML()
}

func (c *InternalLinksCheck) Apply(ctx context.Context, doc *domain.Document) []domain.Issue {
	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return []domain.Issue{
			domain.NewFailIssue(
				c,
				domain.SeverityError,
				fmt.Sprintf("Error during internal link check: %s", err.Error()),
				nil,
			),
		}
	}

	baseParsed, err := url.Parse(doc.URL)
	if err != nil {
		return nil
	}

	internalPaths := c.collectInternalPaths(root, baseParsed)
	if len(internalPaths) == 0 {
		return []domain.Issue{
			domain.NewPassIssue(
				c,
				"No internal links found to check.",
			),
		}
	}

	var (
		detectedIssues []domain.Issue
		toCheck        []string
		mu             sync.Mutex
		wg             sync.WaitGroup
		semaphore      = make(chan struct{}, c.MaxConcurrency)
	)

	for _, path := range internalPaths {
		if cached, ok := c.Cache.Get(path); ok {
			if !cached.Passed {
				detectedIssues = append(detectedIssues, c.buildIssue(path, cached.StatusCode, cached.ErrorMessage))
			}
		} else {
			toCheck = append(toCheck, path)
		}
	}

	for _, path := range toCheck {
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		semaphore <- struct{}{}

		go func(targetPath string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			resolvedURL := baseParsed.ResolveReference(&url.URL{Path: targetPath}).String()
			passed, statusCode, errMsg := c.validatePath(ctx, resolvedURL)

			c.Cache.Set(targetPath, passed, statusCode, errMsg)

			if !passed {
				mu.Lock()
				detectedIssues = append(detectedIssues, c.buildIssue(targetPath, statusCode, errMsg))
				mu.Unlock()
			}
		}(path)
	}

	wg.Wait()

	if len(detectedIssues) > 0 {
		return detectedIssues
	}

	return []domain.Issue{
		domain.NewPassIssue(
			c,
			"All internal links appear to be valid.",
		),
	}
}

func (c *InternalLinksCheck) validatePath(ctx context.Context, resolvedURL string) (bool, int, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, resolvedURL, nil)
	if err != nil {
		return false, 0, err.Error()
	}
	req.Header.Set("User-Agent", "Go-Audit-Engine/1.0")

	resp, err := c.Client.Do(req)
	if err != nil {
		return false, 0, err.Error()
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return false, resp.StatusCode, ""
	}

	return true, resp.StatusCode, ""
}

func (c *InternalLinksCheck) buildIssue(path string, statusCode int, errMsg string) domain.Issue {
	details := map[string]any{
		"issue_type": "unroutable_link",
		"link_path":  path,
	}
	if statusCode > 0 {
		details["status_code"] = statusCode
	}
	if errMsg != "" {
		details["error_message"] = errMsg
	}

	return domain.NewFailIssue(
		c,
		c.Severity,
		fmt.Sprintf("Internal link appears to be broken. Path '%s' is not routable.", path),
		details,
	)
}

func (c *InternalLinksCheck) collectInternalPaths(root *html.Node, baseParsed *url.URL) []string {
	baseHostname := strings.ToLower(baseParsed.Hostname())
	pathSet := make(map[string]struct{})

	var traverse func(n *html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "a") {
			for _, attr := range n.Attr {
				if strings.EqualFold(attr.Key, "href") {
					href := strings.TrimSpace(attr.Val)

					if href == "" ||
						strings.HasPrefix(href, "#") ||
						strings.HasPrefix(href, "mailto:") ||
						strings.HasPrefix(href, "tel:") ||
						strings.HasPrefix(href, "javascript:") {
						break
					}

					parsed, err := url.Parse(href)
					if err == nil {
						if parsed.Host == "" || strings.EqualFold(parsed.Hostname(), baseHostname) {
							cleanPath := parsed.Path
							if cleanPath == "" {
								cleanPath = "/"
							}
							pathSet[cleanPath] = struct{}{}
						}
					}
					break
				}
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}

	traverse(root)

	var paths []string
	for p := range pathSet {
		paths = append(paths, p)
	}
	return paths
}
