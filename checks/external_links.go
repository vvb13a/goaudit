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

type ExternalLinksCheck struct {
	Severity       domain.Severity
	Timeout        time.Duration
	MaxConcurrency int
	Client         *http.Client
	Cache          *service.LinkCache
}

func NewExternalLinksCheck(cache *service.LinkCache) *ExternalLinksCheck {
	if cache == nil {
		cache = service.NewLinkCache(10 * time.Minute)
	}
	timeout := 5 * time.Second

	return &ExternalLinksCheck{
		Severity:       domain.SeverityWarning,
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

func (c *ExternalLinksCheck) Info() domain.CheckInfo {
	return domain.CheckInfo{
		Name:        "external_links",
		Description: "Checks that outbound links on the page are reachable.",
		Category:    domain.CategoryGeneral,
	}
}

func (c *ExternalLinksCheck) Supports(doc *domain.Document) bool {
	return doc.IsHTML()
}

func (c *ExternalLinksCheck) Apply(ctx context.Context, doc *domain.Document) domain.Issue {
	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return domain.NewFailIssue(
			c,
			domain.SeverityError,
			fmt.Sprintf("Error during external link check: %s", err.Error()),
			nil,
		)
	}

	externalUrls := c.collectExternalUrls(root, doc)
	if len(externalUrls) == 0 {
		return domain.NewPassIssue(
			c,
			"No external links found to check.",
		)
	}

	var (
		builder   = domain.NewIssueBuilder()
		toCheck   []string
		mu        sync.Mutex
		wg        sync.WaitGroup
		semaphore = make(chan struct{}, c.MaxConcurrency)
	)

	for _, targetURL := range externalUrls {
		if cached, ok := c.Cache.Get(targetURL); ok {
			if !cached.Passed {
				builder.Add(c.buildFinding(targetURL, cached.StatusCode, cached.ErrorMessage))
			}
		} else {
			toCheck = append(toCheck, targetURL)
		}
	}

	for _, targetURL := range toCheck {
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		semaphore <- struct{}{}

		go func(target string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			passed, statusCode, errMsg := c.validateExternalURL(ctx, target)

			c.Cache.Set(target, passed, statusCode, errMsg)

			if !passed {
				mu.Lock()
				builder.Add(c.buildFinding(target, statusCode, errMsg))
				mu.Unlock()
			}
		}(targetURL)
	}

	wg.Wait()

	if builder.HasFindings() {
		return domain.NewFailIssue(
			c,
			builder.Severity(),
			fmt.Sprintf("%d external link(s) appear to be broken or inaccessible.", builder.Count()),
			builder.Details(),
		)
	}

	return domain.NewPassIssueWithDetails(
		c,
		"All external links are accessible.",
		map[string]any{
			"external_urls": externalUrls,
		},
	)
}

func (c *ExternalLinksCheck) validateExternalURL(ctx context.Context, targetURL string) (bool, int, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, targetURL, nil)
	if err != nil {
		return false, 0, err.Error()
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AuditEngine/1.0)")

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

func (c *ExternalLinksCheck) buildFinding(targetURL string, statusCode int, errMsg string) domain.Finding {
	if errMsg != "" {
		return domain.Finding{
			Type:     "connection_error",
			Severity: c.Severity,
			Message:  "Could not connect to the external link.",
			Data: map[string]any{
				"link_url":      targetURL,
				"error_message": errMsg,
			},
		}
	}

	return domain.Finding{
		Type:     "broken_link",
		Severity: c.Severity,
		Message:  fmt.Sprintf("External link is broken or inaccessible. Responded with status code: %d", statusCode),
		Data: map[string]any{
			"link_url":    targetURL,
			"status_code": statusCode,
		},
	}
}

func (c *ExternalLinksCheck) collectExternalUrls(root *html.Node, doc *domain.Document) []string {
	baseParsed, err := url.Parse(doc.URL)
	if err != nil {
		return nil
	}
	baseHostname := strings.ToLower(baseParsed.Hostname())

	urlMap := make(map[string]struct{})

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
					if err == nil && parsed.Host != "" {
						scheme := strings.ToLower(parsed.Scheme)
						if (scheme == "http" || scheme == "https") &&
							!strings.EqualFold(parsed.Hostname(), baseHostname) {
							urlMap[href] = struct{}{}
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

	var externalUrls []string
	for targetURL := range urlMap {
		externalUrls = append(externalUrls, targetURL)
	}

	return externalUrls
}
