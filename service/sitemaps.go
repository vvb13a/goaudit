package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/vvb13a/goaudit/domain"
)

type DocumentFetcher interface {
	Fetch(ctx context.Context, targetURL string) (*domain.Document, error)
}

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
	var path string
	if err != nil {
		path = strings.ToLower(targetURL)
	} else {
		path = strings.ToLower(parsed.Path)
	}

	return strings.HasSuffix(path, ".xml") ||
		strings.HasSuffix(path, ".xml.gz") ||
		strings.HasSuffix(path, "/sitemap")
}

type SitemapParser struct {
	fetcher  DocumentFetcher
	maxDepth int
}

func NewSitemapParser(fetcher DocumentFetcher, maxDepth int) *SitemapParser {
	if maxDepth <= 0 {
		maxDepth = 3
	}
	return &SitemapParser{
		fetcher:  fetcher,
		maxDepth: maxDepth,
	}
}

func (p *SitemapParser) Parse(ctx context.Context, sitemapURL string) ([]string, error) {
	visited := make(map[string]bool)
	uniqueURLs := make(map[string]struct{})

	err := p.parseRecursive(ctx, sitemapURL, p.maxDepth, visited, uniqueURLs)
	if err != nil {
		return nil, err
	}

	collected := make([]string, 0, len(uniqueURLs))
	for u := range uniqueURLs {
		collected = append(collected, u)
	}

	return collected, nil
}

func (p *SitemapParser) parseRecursive(
	ctx context.Context,
	currentURL string,
	depth int,
	visited map[string]bool,
	uniqueURLs map[string]struct{},
) error {
	if depth <= 0 {
		return nil
	}

	if visited[currentURL] {
		return nil
	}
	visited[currentURL] = true

	if err := ctx.Err(); err != nil {
		return err
	}

	doc, err := p.fetcher.Fetch(ctx, currentURL)
	if err != nil {
		return fmt.Errorf("fetch sitemap %s: %w", currentURL, err)
	}

	bodyBytes, err := decompressIfNeeded(currentURL, doc.Body)
	if err != nil {
		return fmt.Errorf("decompress sitemap %s: %w", currentURL, err)
	}

	var indexDoc sitemapIndexXML
	if err := xml.Unmarshal(bodyBytes, &indexDoc); err == nil && len(indexDoc.Sitemaps) > 0 {
		for _, sm := range indexDoc.Sitemaps {
			childURL := strings.TrimSpace(sm.Loc)
			if childURL == "" {
				continue
			}

			if err := p.parseRecursive(ctx, childURL, depth-1, visited, uniqueURLs); err != nil {
				continue
			}
		}
		return nil
	}

	var urlSetDoc urlSetXML
	if err := xml.Unmarshal(bodyBytes, &urlSetDoc); err == nil && len(urlSetDoc.URLs) > 0 {
		for _, u := range urlSetDoc.URLs {
			loc := strings.TrimSpace(u.Loc)
			if loc != "" && (strings.HasPrefix(loc, "http://") || strings.HasPrefix(loc, "https://")) {
				uniqueURLs[loc] = struct{}{}
			}
		}
		return nil
	}

	return fmt.Errorf("unable to parse %s as a valid XML sitemap", currentURL)
}

func decompressIfNeeded(sitemapURL string, body []byte) ([]byte, error) {
	if strings.HasSuffix(strings.ToLower(sitemapURL), ".gz") || (len(body) > 2 && body[0] == 0x1f && body[1] == 0x8b) {
		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		defer reader.Close()

		return io.ReadAll(reader)
	}

	return body, nil
}
