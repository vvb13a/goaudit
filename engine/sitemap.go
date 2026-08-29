package engine

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
)

type urlSetXML struct {
	XMLName xml.Name `xml:"urlset"`
	URLs    []struct {
		Loc string `xml:"loc"`
	} `xml:"url"`
}

type sitemapIndexXML struct {
	XMLName  xml.Name `xml:"sitemapindex"`
	Sitemaps []struct {
		Loc string `xml:"loc"`
	} `xml:"sitemap"`
}

func IsSitemapURL(targetURL string) bool {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return strings.HasSuffix(strings.ToLower(targetURL), ".xml")
	}
	return strings.HasSuffix(strings.ToLower(parsed.Path), ".xml")
}

func ParseSitemap(ctx context.Context, fetcher *Fetcher, sitemapURL string, maxDepth int) ([]string, error) {
	if maxDepth <= 0 {
		return nil, fmt.Errorf("sitemap recursion depth exceeded at %s", sitemapURL)
	}

	doc, err := fetcher.Fetch(ctx, sitemapURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sitemap %s: %w", sitemapURL, err)
	}

	var indexDoc sitemapIndexXML
	if err := xml.Unmarshal(doc.Body, &indexDoc); err == nil && len(indexDoc.Sitemaps) > 0 {
		var collected []string
		for _, sm := range indexDoc.Sitemaps {
			childURL := strings.TrimSpace(sm.Loc)
			if childURL == "" {
				continue
			}
			childURLs, err := ParseSitemap(ctx, fetcher, childURL, maxDepth-1)
			if err != nil {
				continue
			}
			collected = append(collected, childURLs...)
		}
		return collected, nil
	}

	var urlSetDoc urlSetXML
	if err := xml.Unmarshal(doc.Body, &urlSetDoc); err == nil && len(urlSetDoc.URLs) > 0 {
		var collected []string
		for _, u := range urlSetDoc.URLs {
			loc := strings.TrimSpace(u.Loc)
			if loc != "" {
				collected = append(collected, loc)
			}
		}
		return collected, nil
	}

	return nil, fmt.Errorf("unable to parse %s as a valid sitemap or sitemap index", sitemapURL)
}
