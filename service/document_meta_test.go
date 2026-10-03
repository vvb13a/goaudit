package service

import (
	"net/http"
	"strings"
	"testing"

	"github.com/vvb13a/goaudit/domain"
)

func TestExtractTitle(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "head title",
			body: `<html><head><title>Hello &amp; welcome</title></head><body></body></html>`,
			want: "Hello & welcome",
		},
		{
			name: "trimmed whitespace",
			body: `<html><head><title>  Spaced  </title></head><body></body></html>`,
			want: "Spaced",
		},
		{
			name: "title only in body is ignored",
			body: `<html><head></head><body><title>Body title</title></body></html>`,
			want: "",
		},
		{
			name: "missing title",
			body: `<html><head></head><body></body></html>`,
			want: "",
		},
	}

	for _, c := range cases {
		if got := extractTitle([]byte(c.body)); got != c.want {
			t.Errorf("%s: extractTitle = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestExtractTitleCapsLength(t *testing.T) {
	long := strings.Repeat("a", maxTitleRunes+50)
	body := `<html><head><title>` + long + `</title></head><body></body></html>`
	got := extractTitle([]byte(body))
	if len([]rune(got)) != maxTitleRunes {
		t.Errorf("title length = %d, want %d", len([]rune(got)), maxTitleRunes)
	}
}

func TestExtractEditLink(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		link    string
		want    string
		isHTML  bool
		baseURL string
	}{
		{
			name:    "relative head link resolves against base",
			body:    `<html><head><link rel="edit" href="/wp-admin/post.php?post=1&action=edit"></head></html>`,
			isHTML:  true,
			baseURL: "https://example.com/blog/post",
			want:    "https://example.com/wp-admin/post.php?post=1&action=edit",
		},
		{
			name:    "absolute head link",
			body:    `<html><head><link rel="edit" href="https://cms.example/edit/1"></head></html>`,
			isHTML:  true,
			baseURL: "https://example.com/blog/post",
			want:    "https://cms.example/edit/1",
		},
		{
			name:    "rel token with others",
			body:    `<html><head><link rel="alternate edit" href="/edit"></head></html>`,
			isHTML:  true,
			baseURL: "https://example.com/",
			want:    "https://example.com/edit",
		},
		{
			name:    "link header",
			body:    `<html><head></head></html>`,
			link:    `<https://cms.example/edit/1>; rel="edit"`,
			isHTML:  true,
			baseURL: "https://example.com/",
			want:    "https://cms.example/edit/1",
		},
		{
			name:    "link header among others",
			body:    `<html><head></head></html>`,
			link:    `<https://example.com/next>; rel="next", <https://cms.example/e/2>; rel="edit"`,
			isHTML:  true,
			baseURL: "https://example.com/",
			want:    "https://cms.example/e/2",
		},
		{
			name:    "head link wins over header",
			body:    `<html><head><link rel="edit" href="/edit-inline"></head></html>`,
			link:    `<https://cms.example/edit>; rel="edit"`,
			isHTML:  true,
			baseURL: "https://example.com/",
			want:    "https://example.com/edit-inline",
		},
		{
			name:    "none declared",
			body:    `<html><head></head><body></body></html>`,
			isHTML:  true,
			baseURL: "https://example.com/",
			want:    "",
		},
		{
			name:    "non-http scheme is ignored",
			body:    `<html><head><link rel="edit" href="mailto:edit@example.com"></head></html>`,
			isHTML:  true,
			baseURL: "https://example.com/",
			want:    "",
		},
	}

	for _, c := range cases {
		headers := http.Header{}
		if c.link != "" {
			headers.Set("Link", c.link)
		}
		doc := &domain.Document{
			URL:      c.baseURL,
			FinalURL: c.baseURL,
			Headers:  headers,
			Body:     []byte(c.body),
		}
		if !c.isHTML {
			doc.Headers.Set("Content-Type", "application/octet-stream")
		} else {
			doc.Headers.Set("Content-Type", "text/html")
		}
		if got := extractEditLink(doc); got != c.want {
			t.Errorf("%s: extractEditLink = %q, want %q", c.name, got, c.want)
		}
	}
}
