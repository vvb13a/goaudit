package service

import (
	"bytes"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/vvb13a/goaudit/domain"

	"golang.org/x/net/html"
)

// maxTitleRunes bounds the stored page title so a malformed or hostile
// document cannot bloat the audited URL row.
const maxTitleRunes = 512

// extractTitle returns the trimmed text of the first <title> in the document
// head, capped at maxTitleRunes, or "" when there is none. It mirrors the
// title check's notion of the title (only a title inside <head> counts).
func extractTitle(body []byte) string {
	root, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return ""
	}

	var title string
	var find func(n *html.Node, inHead bool) bool
	find = func(n *html.Node, inHead bool) bool {
		if n.Type == html.ElementNode {
			name := strings.ToLower(n.Data)
			if name == "head" {
				inHead = true
			}
			if inHead && name == "title" {
				title = strings.TrimSpace(textContent(n))
				return true
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if find(child, inHead) {
				return true
			}
		}
		return false
	}
	find(root, false)

	if utf8.RuneCountInString(title) > maxTitleRunes {
		title = string([]rune(title)[:maxTitleRunes])
	}
	return title
}

// extractEditLink returns the absolute URL of the page's edit link. It is
// declared either as <link rel="edit" href="..."> in the document or as an
// RFC 8288 Link header with rel="edit" (for pages that cannot inject markup).
// A relative href resolves against the document's final URL. It returns ""
// when the page declares no edit link.
func extractEditLink(doc *domain.Document) string {
	if doc == nil {
		return ""
	}

	raw := ""
	if doc.IsHTML() {
		raw = editLinkFromHTML(doc.Body)
	}
	if raw == "" {
		raw = editLinkFromHeader(doc.Header("Link"))
	}
	if raw == "" {
		return ""
	}

	base := doc.FinalURL
	if base == "" {
		base = doc.URL
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return ""
	}
	resolved, ok := resolveLink(baseURL, raw)
	if !ok {
		return ""
	}
	return resolved
}

// editLinkFromHTML returns the href of the first <link rel="edit"> in the
// document.
func editLinkFromHTML(body []byte) string {
	root, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return ""
	}

	var found string
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if found != "" {
			return
		}
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "link") {
			if relHasToken(attr(n, "rel"), "edit") {
				if href := attr(n, "href"); href != "" {
					found = href
					return
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return found
}

// editLinkFromHeader returns the target of the first rel="edit" reference in
// an RFC 8288 Link header value.
func editLinkFromHeader(raw string) string {
	for _, part := range splitLinkHeader(raw) {
		target, params := parseLinkPart(part)
		if target == "" {
			continue
		}
		if relHasToken(linkParam(params, "rel"), "edit") {
			return target
		}
	}
	return ""
}

// splitLinkHeader splits a Link header on its top-level commas, ignoring the
// commas inside the <...> target of each reference.
func splitLinkHeader(raw string) []string {
	var parts []string
	depth, start := 0, 0
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, raw[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, raw[start:])
}

// parseLinkPart returns the <target> and the ";"-separated parameters of one
// Link header reference.
func parseLinkPart(part string) (target string, params []string) {
	part = strings.TrimSpace(part)
	if !strings.HasPrefix(part, "<") {
		return "", nil
	}
	end := strings.Index(part, ">")
	if end < 0 {
		return "", nil
	}
	target = strings.TrimSpace(part[1:end])
	for _, p := range strings.Split(part[end+1:], ";") {
		if p = strings.TrimSpace(p); p != "" {
			params = append(params, p)
		}
	}
	return target, params
}

// linkParam returns the value of the named (case-insensitive) parameter,
// stripping optional quotes.
func linkParam(params []string, name string) string {
	for _, p := range params {
		k, v, ok := strings.Cut(p, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(k), name) {
			continue
		}
		return strings.Trim(strings.TrimSpace(v), `"`)
	}
	return ""
}

// relHasToken reports whether a space-separated rel value contains the token.
func relHasToken(raw, token string) bool {
	for _, f := range strings.Fields(strings.ToLower(raw)) {
		if f == token {
			return true
		}
	}
	return false
}

// textContent concatenates the text nodes below n.
func textContent(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		b.WriteString(textContent(child))
	}
	return b.String()
}
