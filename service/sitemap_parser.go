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

// DocumentFetcher defines the minimal fetcher contract needed to retrieve sitemaps.
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

// IsSitemapURL checks if a target URL points to a sitemap XML or GZ file.
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

// SitemapParser handles recursive discovery and parsing of XML and GZ sitemaps.
type SitemapParser struct {
	fetcher  DocumentFetcher
	maxDepth int
}

func NewSitemapParser(fetcher DocumentFetcher, maxDepth int) *SitemapParser {
	if maxDepth <= 0 {
		maxDepth = 3 // Standard default depth for nested indices
	}
	return &SitemapParser{
		fetcher:  fetcher,
		maxDepth: maxDepth,
	}
}

// Parse extracts all unique target URLs from a sitemap or nested sitemap index.
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
		return nil // Max depth reached, stop recursion gracefully
	}

	if visited[currentURL] {
		return nil // Avoid infinite cycles
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

	// 1. Try parsing as a Sitemap Index (<sitemapindex>)
	var indexDoc sitemapIndexXML
	if err := xml.Unmarshal(bodyBytes, &indexDoc); err == nil && len(indexDoc.Sitemaps) > 0 {
		for _, sm := range indexDoc.Sitemaps {
			childURL := strings.TrimSpace(sm.Loc)
			if childURL == "" {
				continue
			}

			if err := p.parseRecursive(ctx, childURL, depth-1, visited, uniqueURLs); err != nil {
				// Log or continue past broken child sitemaps without failing the entire run
				continue
			}
		}
		return nil
	}

	// 2. Try parsing as a Standard URL Set (<urlset>)
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

// decompressIfNeeded handles .xml.gz payloads transparently.
func decompressIfNeeded(sitemapURL string, body []byte) ([]byte, error) {
	// Check if URL ends with .gz or starts with gzip magic bytes (0x1f, 0x8b)
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
