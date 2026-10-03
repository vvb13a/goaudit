package service

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/vvb13a/goaudit/domain"
	"github.com/vvb13a/goaudit/store"
	"golang.org/x/net/html"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GraphService builds and reads the link graph of an audit. It is independent
// of the checks: extraction only needs the fetched document, and persistence
// runs at the end of a run once every URL has been visited. Nodes and edges are
// scoped by audit id, nodes are unique per audit and edges reference nodes by
// id.
type GraphService struct {
	db *gorm.DB
}

func NewGraphService(db *gorm.DB) *GraphService {
	return &GraphService{db: db}
}

// extensionFiletypes maps a lower-cased URL extension to a coarse filetype.
var extensionFiletypes = map[string]string{
	".html":        "html",
	".htm":         "html",
	".xhtml":       "html",
	".css":         "css",
	".js":          "js",
	".mjs":         "js",
	".cjs":         "js",
	".json":        "json",
	".xml":         "xml",
	".rss":         "xml",
	".atom":        "xml",
	".txt":         "txt",
	".pdf":         "pdf",
	".jpg":         "jpg",
	".jpeg":        "jpg",
	".png":         "png",
	".webp":        "webp",
	".gif":         "gif",
	".svg":         "svg",
	".ico":         "ico",
	".avif":        "avif",
	".bmp":         "bmp",
	".woff":        "font",
	".woff2":       "font",
	".ttf":         "font",
	".otf":         "font",
	".eot":         "font",
	".mp4":         "video",
	".webm":        "video",
	".ogg":         "audio",
	".mp3":         "audio",
	".wav":         "audio",
	".webmanifest": "manifest",
	".manifest":    "manifest",
}

// Extract collects every outbound reference from an HTML document. URLs are
// resolved against the document's base, made absolute and fragment-free, and
// deduplicated per reference kind.
func (s *GraphService) Extract(doc *domain.Document) []domain.RawLink {
	if doc == nil || !doc.IsHTML() || len(doc.Body) == 0 {
		return nil
	}
	base := doc.FinalURL
	if base == "" {
		base = doc.URL
	}
	baseParsed, err := url.Parse(base)
	if err != nil {
		return nil
	}
	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return nil
	}

	seen := make(map[string]struct{})
	var links []domain.RawLink

	add := func(raw, linkType, filetypeHint string) {
		if linkType == "" {
			return
		}
		target, ok := resolveLink(baseParsed, raw)
		if !ok {
			return
		}
		key := linkType + "\x00" + target
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		links = append(links, domain.RawLink{
			URL:      target,
			Type:     linkType,
			Filetype: detectFiletype(target, filetypeHint),
		})
	}

	var traverse func(n *html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch strings.ToLower(n.Data) {
			case "a":
				add(attr(n, "href"), "hyperlink", "html")
			case "link":
				rel := strings.ToLower(attr(n, "rel"))
				switch {
				case strings.Contains(rel, "stylesheet"):
					add(attr(n, "href"), "stylesheet", "css")
				case strings.Contains(rel, "icon"):
					add(attr(n, "href"), "icon", "ico")
				case strings.Contains(rel, "manifest"):
					add(attr(n, "href"), "manifest", "manifest")
				case strings.Contains(rel, "preload"), strings.Contains(rel, "modulepreload"):
					add(attr(n, "href"), "preload", "")
				}
			case "script":
				add(attr(n, "src"), "script", "js")
			case "img":
				add(attr(n, "src"), "image", "image")
				addSrcset(attr(n, "srcset"), add)
			case "source":
				add(attr(n, "src"), "source", "media")
				addSrcset(attr(n, "srcset"), add)
			case "iframe":
				add(attr(n, "src"), "iframe", "html")
			case "video":
				add(attr(n, "src"), "video", "video")
			case "audio":
				add(attr(n, "src"), "audio", "audio")
			case "track":
				add(attr(n, "src"), "track", "vtt")
			case "embed":
				add(attr(n, "src"), "embed", "")
			case "object":
				add(attr(n, "data"), "object", "")
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}
	traverse(root)

	return links
}

// attr returns the value of the named attribute, empty when absent.
func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return strings.TrimSpace(a.Val)
		}
	}
	return ""
}

// addSrcset feeds every URL candidate of a srcset attribute to add. Each
// candidate is "url [descriptor]"; the descriptor is ignored.
func addSrcset(value string, add func(raw, linkType, filetypeHint string)) {
	for _, candidate := range strings.Split(value, ",") {
		fields := strings.Fields(strings.TrimSpace(candidate))
		if len(fields) == 0 {
			continue
		}
		add(fields[0], "image", "image")
	}
}

// resolveLink resolves a raw href/src against the document base and returns the
// absolute, fragment-free URL. Non-navigable references (fragments, mailto,
// javascript, data, ...) and non-http(s) schemes are dropped.
func resolveLink(base *url.URL, raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if isJunkLink(raw) {
		return "", false
	}
	lower := strings.ToLower(raw)
	for _, prefix := range []string{"#", "mailto:", "tel:", "javascript:", "data:", "blob:"} {
		if strings.HasPrefix(lower, prefix) {
			return "", false
		}
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	resolved := base.ResolveReference(ref)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return "", false
	}
	resolved.Fragment = ""
	return resolved.String(), true
}

// isJunkLink reports whether a raw href/src is not a usable resource URL.
// Blur placeholders are often inline data:image/svg+xml URIs; when such a URI
// sits in a srcset, the comma it contains is a candidate separator, so the
// markup leaks out as a "relative" URL (e.g. "%3Csvg ...") that would otherwise
// resolve to a bogus page-relative node. Angle brackets, embedded whitespace
// and encoded markup are all rejected.
func isJunkLink(raw string) bool {
	if strings.ContainsAny(raw, "<>\"") {
		return true
	}
	if strings.ContainsAny(raw, " \t\n\r\f") {
		return true
	}
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "%3c") || strings.Contains(lower, "%3e") {
		return true
	}
	if decoded, err := url.PathUnescape(raw); err == nil && strings.ContainsAny(decoded, "<>\"") {
		return true
	}
	return false
}

// normalizeGraphURL is the canonical node key: lower-cased scheme and host and
// no fragment.
func normalizeGraphURL(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	return u.String()
}

// urlMatchKey is a comparison key that treats an empty path and "/" as equal,
// so a configured target ("https://example.com") still matches the node of the
// fetched page ("https://example.com/") after a redirect or slash addition.
func urlMatchKey(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return strings.TrimSpace(raw)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String()
}

// detectFiletype classifies a URL by its extension, falling back to the
// referencing element's hint and finally to "other".
func detectFiletype(rawURL, hint string) string {
	if u, err := url.Parse(rawURL); err == nil {
		ext := strings.ToLower(path.Ext(u.Path))
		if ft, ok := extensionFiletypes[ext]; ok {
			return ft
		}
	}
	if hint != "" {
		return hint
	}
	return "other"
}

// Persist replaces the audit's stored graph with the one derived from the run:
// every audited page becomes a node, every extracted link becomes an edge, and
// link targets that are not audited become target-only nodes. It runs inside
// the caller's transaction so the graph lands atomically with the run data.
func (s *GraphService) Persist(ctx context.Context, tx *gorm.DB, auditID string, audit *domain.Audit) error {
	if s == nil || tx == nil || audit == nil || auditID == "" {
		return nil
	}

	now := time.Now().UTC()

	// Load the stored state of the audit's current nodes so first_seen and the
	// last validation result survive the rebuild; nodes are reconciled rather
	// than wiped.
	var previous []store.GraphNode
	if err := tx.Select("id, first_seen, status_code, last_validated").
		Where("audit_id = ?", auditID).Find(&previous).Error; err != nil {
		return err
	}
	existingFirst := make(map[string]time.Time, len(previous))
	existingStatus := make(map[string]int, len(previous))
	existingValidated := make(map[string]time.Time, len(previous))
	for i := range previous {
		existingFirst[previous[i].ID] = previous[i].FirstSeen
		existingStatus[previous[i].ID] = previous[i].StatusCode
		existingValidated[previous[i].ID] = previous[i].LastValidated
	}

	rootURLs := make(map[string]struct{}, len(audit.Targets))
	for _, t := range audit.Targets {
		rootURLs[urlMatchKey(t)] = struct{}{}
	}
	rootHosts := hostSet(audit.Targets)

	nodes := make(map[string]*domain.GraphNode)
	ensure := func(rawURL, hint string) *domain.GraphNode {
		nurl := normalizeGraphURL(rawURL)
		if nurl == "" {
			return nil
		}
		if n, ok := nodes[nurl]; ok {
			if ft := detectFiletype(nurl, hint); ft != "other" && n.Filetype == "other" {
				n.Filetype = ft
			}
			return n
		}
		node := &domain.GraphNode{
			ID:       store.GraphNodeID(auditID, nurl),
			URL:      nurl,
			Filetype: detectFiletype(nurl, hint),
			External: !hostIn(nurl, rootHosts),
		}
		if _, ok := rootURLs[urlMatchKey(nurl)]; ok {
			node.IsRoot = true
		}
		nodes[nurl] = node
		return node
	}

	edgeCount := make(map[string]int)
	edgeType := make(map[string]string)
	edgeSource := make(map[string]string)
	edgeTarget := make(map[string]string)

	for _, u := range audit.Urls {
		if u == nil {
			continue
		}
		pageURL := issueURL(u)
		source := ensure(pageURL, "html")
		if source == nil {
			continue
		}
		// Pages fetched during the run are audited: their status is known from
		// the fetch, so link validation skips them and only checks targets.
		source.Audited = true
		source.StatusCode = u.StatusCode
		source.LastValidated = now
		for _, link := range u.Links {
			target := ensure(link.URL, link.Filetype)
			if target == nil {
				continue
			}
			id := store.GraphEdgeID(auditID, source.ID, target.ID, link.Type)
			edgeCount[id]++
			edgeType[id] = link.Type
			edgeSource[id] = source.ID
			edgeTarget[id] = target.ID
			source.OutLinks++
			target.InLinks++
		}
	}

	// Targets take the freshest validation result from the global link_targets
	// table (populated before persistence); otherwise their previous stored
	// result is carried over.
	var targetURLs []string
	for _, n := range nodes {
		if !n.Audited {
			targetURLs = append(targetURLs, n.URL)
		}
	}
	freshTargets, err := loadLinkTargets(tx, targetURLs)
	if err != nil {
		return err
	}

	nodeModels := make([]*store.GraphNode, 0, len(nodes))
	for _, n := range nodes {
		if first, ok := existingFirst[n.ID]; ok && !first.IsZero() {
			n.FirstSeen = first
		} else {
			n.FirstSeen = now
		}
		n.LastSeen = now
		if !n.Audited {
			if lt, ok := freshTargets[n.URL]; ok {
				n.StatusCode = lt.StatusCode
				n.LastValidated = lt.ValidatedAt
			} else {
				if status, ok := existingStatus[n.ID]; ok {
					n.StatusCode = status
				}
				if validated, ok := existingValidated[n.ID]; ok {
					n.LastValidated = validated
				}
			}
		}
		nodeModels = append(nodeModels, store.GraphNodeModel(auditID, n))
	}
	edgeModels := make([]*store.GraphEdge, 0, len(edgeCount))
	for id, count := range edgeCount {
		edgeModels = append(edgeModels, store.GraphEdgeModel(auditID, &domain.GraphEdge{
			SourceNodeID: edgeSource[id],
			TargetNodeID: edgeTarget[id],
			Type:         edgeType[id],
			Count:        count,
		}))
	}

	// Edges are fully replaced each run; nodes are reconciled so first_seen
	// survives across runs and only last_seen moves.
	if err := tx.Where("audit_id = ?", auditID).Delete(&store.GraphEdge{}).Error; err != nil {
		return err
	}
	ids := make([]string, 0, len(nodeModels))
	for _, m := range nodeModels {
		ids = append(ids, m.ID)
	}
	stale := tx.Where("audit_id = ?", auditID)
	if len(ids) > 0 {
		stale = stale.Where("id NOT IN ?", ids)
	}
	if err := stale.Delete(&store.GraphNode{}).Error; err != nil {
		return err
	}
	if len(nodeModels) > 0 {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"url", "filetype", "external", "is_root", "audited", "in_links", "out_links", "last_seen", "status_code", "last_validated"}),
		}).CreateInBatches(nodeModels, createBatchRows).Error; err != nil {
			return err
		}
	}
	if err := createInBatches(tx, edgeModels); err != nil {
		return err
	}
	return nil
}

// loadLinkTargets loads the global validation results for the given URLs,
// batching the IN clause to stay under SQLite's variable limit.
func loadLinkTargets(tx *gorm.DB, urls []string) (map[string]store.LinkTarget, error) {
	result := make(map[string]store.LinkTarget, len(urls))
	const chunk = 400
	for start := 0; start < len(urls); start += chunk {
		end := start + chunk
		if end > len(urls) {
			end = len(urls)
		}
		var rows []store.LinkTarget
		if err := tx.Where("url IN ?", urls[start:end]).Find(&rows).Error; err != nil {
			return nil, err
		}
		for i := range rows {
			result[rows[i].URL] = rows[i]
		}
	}
	return result, nil
}

// Delete removes every graph node and edge of an audit. It runs inside the
// caller's transaction.
func (s *GraphService) Delete(tx *gorm.DB, auditID string) error {
	if s == nil || tx == nil || auditID == "" {
		return nil
	}
	if err := tx.Where("audit_id = ?", auditID).Delete(&store.GraphEdge{}).Error; err != nil {
		return err
	}
	return tx.Where("audit_id = ?", auditID).Delete(&store.GraphNode{}).Error
}

// ListNodes returns one page of nodes and the total matching the filter.
func (s *GraphService) ListNodes(ctx context.Context, auditID string, f domain.GraphNodeFilter, limit, offset int) ([]*domain.GraphNode, int, error) {
	if s == nil || s.db == nil {
		return nil, 0, nil
	}
	q := s.db.WithContext(ctx).Model(&store.GraphNode{}).Where("audit_id = ?", auditID)
	if len(f.Filetypes) > 0 {
		q = q.Where("filetype IN ?", f.Filetypes)
	}
	if f.External != nil {
		q = q.Where("external = ?", *f.External)
	}
	if f.IsRoot != nil {
		q = q.Where("is_root = ?", *f.IsRoot)
	}
	if f.Search != "" {
		q = q.Where("url LIKE ?", likeContains(f.Search))
	}
	if f.FirstSeenFrom != nil {
		q = q.Where("first_seen >= ?", *f.FirstSeenFrom)
	}
	if f.FirstSeenTo != nil {
		q = q.Where("first_seen <= ?", *f.FirstSeenTo)
	}
	if f.LastSeenFrom != nil {
		q = q.Where("last_seen >= ?", *f.LastSeenFrom)
	}
	if f.LastSeenTo != nil {
		q = q.Where("last_seen <= ?", *f.LastSeenTo)
	}
	if len(f.Statuses) > 0 {
		q = q.Where("status_code IN ?", f.Statuses)
	}
	if f.LastValidatedFrom != nil {
		q = q.Where("last_validated >= ?", *f.LastValidatedFrom)
	}
	if f.LastValidatedTo != nil {
		q = q.Where("last_validated <= ?", *f.LastValidatedTo)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var models []store.GraphNode
	if err := q.Order(nodeOrder(f.Sort, f.Order)).Limit(limit).Offset(offset).Find(&models).Error; err != nil {
		return nil, 0, err
	}
	out := make([]*domain.GraphNode, 0, len(models))
	for i := range models {
		out = append(out, models[i].ToDomain())
	}
	return out, int(total), nil
}

// ListEdges returns one page of denormalized edges and the total matching the
// filter.
func (s *GraphService) ListEdges(ctx context.Context, auditID string, f domain.GraphEdgeFilter, limit, offset int) ([]*domain.GraphEdgeRow, int, error) {
	if s == nil || s.db == nil {
		return nil, 0, nil
	}
	base := func() *gorm.DB {
		q := s.db.WithContext(ctx).Table("graph_edges AS e").
			Joins("JOIN graph_nodes AS s ON s.id = e.source_node_id").
			Joins("JOIN graph_nodes AS t ON t.id = e.target_node_id").
			Where("e.audit_id = ?", auditID)
		if len(f.Types) > 0 {
			q = q.Where("e.edge_type IN ?", f.Types)
		}
		if f.SourceNodeID != "" {
			q = q.Where("e.source_node_id = ?", f.SourceNodeID)
		}
		if f.TargetNodeID != "" {
			q = q.Where("e.target_node_id = ?", f.TargetNodeID)
		}
		if f.SourceContains != "" {
			q = q.Where("s.url LIKE ?", likeContains(f.SourceContains))
		}
		if f.TargetContains != "" {
			q = q.Where("t.url LIKE ?", likeContains(f.TargetContains))
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []store.GraphEdgeRow
	err := base().
		Select("e.id AS id, e.edge_type AS type, e.count AS count, " +
			"e.source_node_id AS source_node_id, s.url AS source_url, s.filetype AS source_filetype, " +
			"e.target_node_id AS target_node_id, t.url AS target_url, t.filetype AS target_filetype, t.external AS target_external").
		Order(edgeOrder(f.Sort, f.Order)).
		Limit(limit).Offset(offset).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	out := make([]*domain.GraphEdgeRow, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].ToDomain())
	}
	return out, int(total), nil
}

// GetNode returns one node of the audit's graph by id.
func (s *GraphService) GetNode(ctx context.Context, auditID, nodeID string) (*domain.GraphNode, error) {
	if s == nil || s.db == nil {
		return nil, domain.ErrGraphNodeNotFound
	}
	var model store.GraphNode
	err := s.db.WithContext(ctx).Where("audit_id = ? AND id = ?", auditID, nodeID).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrGraphNodeNotFound
	}
	if err != nil {
		return nil, err
	}
	return model.ToDomain(), nil
}

// Summary returns the graph's headline totals and filetype and status-code
// distributions.
func (s *GraphService) Summary(ctx context.Context, auditID string) (*domain.GraphSummary, error) {
	sum := &domain.GraphSummary{
		Filetypes: []domain.FiletypeCount{},
		Statuses:  []domain.StatusCodeCount{},
	}
	if s == nil || s.db == nil {
		return sum, nil
	}

	var nodeCount, edgeCount, externalCount, rootCount int64
	db := s.db.WithContext(ctx)
	if err := db.Model(&store.GraphNode{}).Where("audit_id = ?", auditID).Count(&nodeCount).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&store.GraphNode{}).Where("audit_id = ? AND external = ?", auditID, true).Count(&externalCount).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&store.GraphNode{}).Where("audit_id = ? AND is_root = ?", auditID, true).Count(&rootCount).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&store.GraphEdge{}).Where("audit_id = ?", auditID).Count(&edgeCount).Error; err != nil {
		return nil, err
	}

	type bucket struct {
		Filetype string
		Count    int
	}
	var buckets []bucket
	if err := db.Model(&store.GraphNode{}).
		Select("filetype AS filetype, COUNT(*) AS count").
		Where("audit_id = ?", auditID).
		Group("filetype").
		Order("count DESC").
		Scan(&buckets).Error; err != nil {
		return nil, err
	}
	for _, b := range buckets {
		sum.Filetypes = append(sum.Filetypes, domain.FiletypeCount{Filetype: b.Filetype, Count: b.Count})
	}

	type statusBucket struct {
		StatusCode int
		Count      int
	}
	var statusBuckets []statusBucket
	if err := db.Model(&store.GraphNode{}).
		Select("status_code AS status_code, COUNT(*) AS count").
		Where("audit_id = ?", auditID).
		Group("status_code").
		Order("count DESC").
		Scan(&statusBuckets).Error; err != nil {
		return nil, err
	}
	for _, b := range statusBuckets {
		sum.Statuses = append(sum.Statuses, domain.StatusCodeCount{StatusCode: b.StatusCode, Count: b.Count})
	}

	sum.TotalNodes = int(nodeCount)
	sum.TotalEdges = int(edgeCount)
	sum.ExternalNodes = int(externalCount)
	sum.RootNodes = int(rootCount)
	return sum, nil
}

// ListGraphNodes returns one page of the audit's graph nodes.
func (s *AuditService) ListGraphNodes(ctx context.Context, auditID string, f domain.GraphNodeFilter, limit, offset int) ([]*domain.GraphNode, int, error) {
	return s.graph.ListNodes(ctx, auditID, f, limit, offset)
}

// ListGraphEdges returns one page of the audit's graph edges, denormalized with
// the source and target node URLs.
func (s *AuditService) ListGraphEdges(ctx context.Context, auditID string, f domain.GraphEdgeFilter, limit, offset int) ([]*domain.GraphEdgeRow, int, error) {
	return s.graph.ListEdges(ctx, auditID, f, limit, offset)
}

// GraphSummary returns the audit's graph headline totals and filetype
// distribution.
func (s *AuditService) GraphSummary(ctx context.Context, auditID string) (*domain.GraphSummary, error) {
	return s.graph.Summary(ctx, auditID)
}

// GetGraphNode returns one node of the audit's graph by id.
func (s *AuditService) GetGraphNode(ctx context.Context, auditID, nodeID string) (*domain.GraphNode, error) {
	return s.graph.GetNode(ctx, auditID, nodeID)
}

// nodeOrder maps a requested sort field to a safe column order clause.
func nodeOrder(sortField, order string) string {
	dir := orderDir(order)
	switch sortField {
	case "filetype":
		return "filetype " + dir + ", url ASC"
	case "in_links":
		return "in_links " + dir + ", url ASC"
	case "out_links":
		return "out_links " + dir + ", url ASC"
	case "external":
		return "external " + dir + ", url ASC"
	case "first_seen":
		return "first_seen " + dir + ", url ASC"
	case "last_seen":
		return "last_seen " + dir + ", url ASC"
	case "status_code":
		return "status_code " + dir + ", url ASC"
	case "last_validated":
		return "last_validated " + dir + ", url ASC"
	default:
		return "url " + dir
	}
}

// edgeOrder maps a requested sort field to a safe column order clause.
func edgeOrder(sortField, order string) string {
	dir := orderDir(order)
	switch sortField {
	case "target":
		return "t.url " + dir
	case "type":
		return "e.edge_type " + dir + ", s.url ASC"
	default:
		return "s.url " + dir + ", t.url ASC"
	}
}

func orderDir(order string) string {
	if strings.EqualFold(order, "desc") {
		return "DESC"
	}
	return "ASC"
}

// hostSet collects the lower-cased hostnames of the audit's targets.
func hostSet(targets []string) map[string]struct{} {
	set := make(map[string]struct{}, len(targets))
	for _, t := range targets {
		if u, err := url.Parse(strings.TrimSpace(t)); err == nil && u.Hostname() != "" {
			set[strings.ToLower(u.Hostname())] = struct{}{}
		}
	}
	return set
}

// hostIn reports whether the URL's host is one of the target hosts.
func hostIn(rawURL string, set map[string]struct{}) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	_, ok := set[strings.ToLower(u.Hostname())]
	return ok
}
