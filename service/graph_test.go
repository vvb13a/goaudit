package service

import (
	"net/http"
	"strings"
	"testing"

	"github.com/vvb13a/goaudit/domain"
)

func TestIsJunkLink(t *testing.T) {
	junk := []string{
		"%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%3E%3C%2Fsvg%3E",
		"%3csvg%3e",
		"<svg></svg>",
		`"quoted"`,
		"/foo bar",
	}
	for _, raw := range junk {
		if !isJunkLink(raw) {
			t.Errorf("expected %q to be junk", raw)
		}
	}

	ok := []string{
		"https://example.com/image.jpg",
		"/assets/app.js",
		"https://example.com/a%20b.png",
		"https://example.com/path?q=1",
	}
	for _, raw := range ok {
		if isJunkLink(raw) {
			t.Errorf("did not expect %q to be junk", raw)
		}
	}
}

func TestExtractClassifiesContainerAndRole(t *testing.T) {
	body := `<html><head>` +
		`<link rel="stylesheet" href="/style.css">` +
		`</head><body>` +
		`<header><nav><a href="/nav">Nav</a></nav></header>` +
		`<main>` +
		`<a href="/content">Content</a>` +
		`<a href="/injected" data-audit-role="injected">Injected</a>` +
		`<a href="/same">Header</a>` +
		`<article><footer><a href="/article-footer">Article footer</a></footer></article>` +
		`</main>` +
		`<footer><a href="/footer">Footer</a></footer>` +
		`</body></html>`

	doc := &domain.Document{
		URL:      "https://example.com/page",
		FinalURL: "https://example.com/page",
		Headers:  http.Header{"Content-Type": []string{"text/html"}},
		Body:     []byte(body),
	}

	links := NewGraphService(nil).Extract(doc)
	byURL := make(map[string]domain.RawLink, len(links))
	for _, l := range links {
		byURL[strings.TrimPrefix(l.URL, "https://example.com")] = l
	}

	cases := []struct {
		path, linkType, container, role string
	}{
		{"/style.css", "stylesheet", "head", "resource"},
		{"/nav", "hyperlink", "header", "nav"},
		{"/content", "hyperlink", "body", "content"},
		{"/injected", "hyperlink", "body", "injected"},
		{"/article-footer", "hyperlink", "body", "content"},
		{"/footer", "hyperlink", "footer", "nav"},
	}
	for _, c := range cases {
		got, ok := byURL[c.path]
		if !ok {
			t.Errorf("expected %q to be extracted", c.path)
			continue
		}
		if got.Type != c.linkType || got.Container != c.container || got.Role != c.role {
			t.Errorf("%q = type %q container %q role %q, want %q %q %q",
				c.path, got.Type, got.Container, got.Role, c.linkType, c.container, c.role)
		}
	}
}

func TestExtractSplitsByContainer(t *testing.T) {
	body := `<html><body>` +
		`<header><a href="/same">Header</a></header>` +
		`<main><a href="/same">Body</a></main>` +
		`</body></html>`

	doc := &domain.Document{
		URL:      "https://example.com/page",
		FinalURL: "https://example.com/page",
		Headers:  http.Header{"Content-Type": []string{"text/html"}},
		Body:     []byte(body),
	}

	var containers []string
	for _, l := range NewGraphService(nil).Extract(doc) {
		if l.URL == "https://example.com/same" {
			containers = append(containers, l.Container)
		}
	}
	if len(containers) != 2 {
		t.Fatalf("expected the same link in two containers, got %v", containers)
	}
}

func TestExtractFiltersSrcsetDataURI(t *testing.T) {
	body := `<html><body>` +
		`<img src="https://example.com/photo.jpg" srcset="` +
		`https://example.com/photo-2x.jpg 2x, ` +
		`data:image/svg+xml,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%3E%3C%2Fsvg%3E 1x">` +
		`</body></html>`

	doc := &domain.Document{
		URL:      "https://example.com/page",
		FinalURL: "https://example.com/page",
		Headers:  http.Header{"Content-Type": []string{"text/html"}},
		Body:     []byte(body),
	}

	links := NewGraphService(nil).Extract(doc)

	got := make(map[string]bool, len(links))
	for _, l := range links {
		if strings.Contains(l.URL, "%3C") || strings.Contains(l.URL, "%3c") || strings.Contains(l.URL, "<") {
			t.Fatalf("junk link extracted: %q", l.URL)
		}
		got[l.URL] = true
	}

	for _, want := range []string{"https://example.com/photo.jpg", "https://example.com/photo-2x.jpg"} {
		if !got[want] {
			t.Errorf("expected %q to be extracted, got %v", want, got)
		}
	}
}
