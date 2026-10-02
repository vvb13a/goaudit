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
