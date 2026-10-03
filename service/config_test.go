package service

import (
	"testing"
	"time"

	"github.com/vvb13a/goaudit/domain"
)

func TestRequestDelayForClassifiesAssets(t *testing.T) {
	cfg := Config{
		RequestDelayMs:      100,
		AssetRequestDelayMs: 0,
	}

	if got := cfg.RequestDelayFor("html"); got != 100*time.Millisecond {
		t.Errorf("html delay = %v, want 100ms", got)
	}
	if got := cfg.RequestDelayFor("other"); got != 100*time.Millisecond {
		t.Errorf("other delay = %v, want 100ms", got)
	}
	for _, ft := range []string{"css", "js", "jpg", "png", "font", "video", "audio", "json", "image", "media"} {
		if got := cfg.RequestDelayFor(ft); got != 0 {
			t.Errorf("%s delay = %v, want 0", ft, got)
		}
	}
}

func TestStaticAssetFiletypes(t *testing.T) {
	assets := []string{
		"https://cdn.example.com/site.css",
		"https://cdn.example.com/app.js",
		"https://cdn.example.com/img/photo.webp",
		"https://cdn.example.com/fonts/inter.woff2",
		"https://cdn.example.com/media/clip.mp4",
	}
	for _, raw := range assets {
		if ft := detectFiletype(raw, ""); !isStaticAssetFiletype(ft) {
			t.Errorf("detectFiletype(%q) = %q, want a static asset", raw, ft)
		}
	}

	documents := []string{
		"https://example.com/",
		"https://example.com/page.html",
		"https://example.com/api/items",
	}
	for _, raw := range documents {
		if ft := detectFiletype(raw, ""); isStaticAssetFiletype(ft) {
			t.Errorf("detectFiletype(%q) = %q, want document pacing", raw, ft)
		}
	}
}

func TestTargetURLsCarriesFiletypeHint(t *testing.T) {
	audit := &domain.Audit{
		Urls: []*domain.AuditedUrl{
			{
				URL: "https://example.com/page",
				Links: []domain.RawLink{
					{URL: "https://cdn.example.com/static/app", Type: "script", Filetype: "js"},
					{URL: "https://cdn.example.com/site.css", Type: "stylesheet", Filetype: "css"},
					{URL: "https://cdn.example.com/photo", Type: "image", Filetype: "jpg"},
					{URL: "https://example.com/other", Type: "hyperlink", Filetype: "html"},
				},
			},
			{URL: "https://example.com/other"},
		},
	}

	targets := NewGraphService(nil).TargetURLs(audit)
	got := make(map[string]string, len(targets))
	for _, target := range targets {
		got[target.URL] = target.Filetype
	}

	if _, ok := got["https://example.com/other"]; ok {
		t.Error("audited page should not be a validation target")
	}
	want := map[string]string{
		"https://cdn.example.com/static/app": "js",
		"https://cdn.example.com/site.css":   "css",
		"https://cdn.example.com/photo":      "jpg",
	}
	for url, ft := range want {
		if got[url] != ft {
			t.Errorf("target %q filetype = %q, want %q", url, got[url], ft)
		}
	}
}

func TestDedupeTargetsMergesHint(t *testing.T) {
	targets := []ValidationTarget{
		{URL: "https://cdn.example.com/static/app"},
		{URL: "https://cdn.example.com/static/app", Filetype: "js"},
		{URL: "https://cdn.example.com/static/app", Filetype: "css"},
	}

	out := dedupeTargets(targets)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1", len(out))
	}
	if out[0].Filetype != "js" {
		t.Errorf("filetype = %q, want js (first non-empty hint wins)", out[0].Filetype)
	}
}
