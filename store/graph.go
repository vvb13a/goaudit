package store

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/vvb13a/goaudit/domain"
)

// GraphNode mirrors the "graph_nodes" table. Nodes are unique per audit and
// addressed by a deterministic id derived from (audit, url). The link counts
// are stored denormalized so the node list can sort and display them without
// aggregating the edge table.
type GraphNode struct {
	ID        string    `gorm:"column:id;primaryKey;type:text"`
	AuditID   string    `gorm:"column:audit_id;type:text;not null;index:idx_graph_nodes_audit_id;uniqueIndex:idx_graph_nodes_audit_url"`
	URL       string    `gorm:"column:url;type:text;not null;uniqueIndex:idx_graph_nodes_audit_url"`
	Filetype  string    `gorm:"column:filetype;type:text;not null;index:idx_graph_nodes_filetype"`
	External  bool      `gorm:"column:external;not null;default:false"`
	IsRoot    bool      `gorm:"column:is_root;not null;default:false"`
	Audited   bool      `gorm:"column:audited;not null;default:false"`
	InLinks   int       `gorm:"column:in_links;not null;default:0"`
	OutLinks  int       `gorm:"column:out_links;not null;default:0"`
	FirstSeen time.Time `gorm:"column:first_seen"`
	LastSeen  time.Time `gorm:"column:last_seen"`

	StatusCode    int       `gorm:"column:status_code;not null;default:0;index:idx_graph_nodes_status_code"`
	LastValidated time.Time `gorm:"column:last_validated"`
}

func (GraphNode) TableName() string { return "graph_nodes" }

// GraphNodeID derives the deterministic row id of a node from its unique
// (audit, normalized url) combination.
func GraphNodeID(auditID, url string) string {
	sum := sha256.Sum256([]byte(auditID + "\x00" + url))
	return "gn_" + hex.EncodeToString(sum[:16])
}

func GraphNodeModel(auditID string, n *domain.GraphNode) *GraphNode {
	return &GraphNode{
		ID:        GraphNodeID(auditID, n.URL),
		AuditID:   auditID,
		URL:       n.URL,
		Filetype:  n.Filetype,
		External:  n.External,
		IsRoot:    n.IsRoot,
		Audited:   n.Audited,
		InLinks:   n.InLinks,
		OutLinks:  n.OutLinks,
		FirstSeen: n.FirstSeen,
		LastSeen:  n.LastSeen,

		StatusCode:    n.StatusCode,
		LastValidated: n.LastValidated,
	}
}

func (m *GraphNode) ToDomain() *domain.GraphNode {
	return &domain.GraphNode{
		ID:        m.ID,
		URL:       m.URL,
		Filetype:  m.Filetype,
		External:  m.External,
		IsRoot:    m.IsRoot,
		Audited:   m.Audited,
		InLinks:   m.InLinks,
		OutLinks:  m.OutLinks,
		FirstSeen: m.FirstSeen,
		LastSeen:  m.LastSeen,

		StatusCode:    m.StatusCode,
		LastValidated: m.LastValidated,
	}
}

// GraphEdge mirrors the "graph_edges" table. Edges reference nodes by id; the
// unique index keeps one row per (audit, source, target, type, container, role)
// with a count of the repeated references.
type GraphEdge struct {
	ID           string `gorm:"column:id;primaryKey;type:text"`
	AuditID      string `gorm:"column:audit_id;type:text;not null;index:idx_graph_edges_audit_id;uniqueIndex:idx_graph_edges_unique"`
	SourceNodeID string `gorm:"column:source_node_id;type:text;not null;index:idx_graph_edges_source;uniqueIndex:idx_graph_edges_unique"`
	TargetNodeID string `gorm:"column:target_node_id;type:text;not null;index:idx_graph_edges_target;uniqueIndex:idx_graph_edges_unique"`
	Type         string `gorm:"column:edge_type;type:text;not null;index:idx_graph_edges_type;uniqueIndex:idx_graph_edges_unique"`
	Container    string `gorm:"column:container;type:text;not null;default:'';index:idx_graph_edges_container;uniqueIndex:idx_graph_edges_unique"`
	Role         string `gorm:"column:role;type:text;not null;default:'';index:idx_graph_edges_role;uniqueIndex:idx_graph_edges_unique"`
	Count        int    `gorm:"column:count;not null;default:1"`
}

func (GraphEdge) TableName() string { return "graph_edges" }

// GraphEdgeID derives the deterministic row id of an edge from its unique
// (audit, source, target, type, container, role) combination.
func GraphEdgeID(auditID, sourceID, targetID, edgeType, container, role string) string {
	sum := sha256.Sum256([]byte(auditID + "\x00" + sourceID + "\x00" + targetID + "\x00" + edgeType + "\x00" + container + "\x00" + role))
	return "ge_" + hex.EncodeToString(sum[:16])
}

func GraphEdgeModel(auditID string, e *domain.GraphEdge) *GraphEdge {
	return &GraphEdge{
		ID:           GraphEdgeID(auditID, e.SourceNodeID, e.TargetNodeID, e.Type, e.Container, e.Role),
		AuditID:      auditID,
		SourceNodeID: e.SourceNodeID,
		TargetNodeID: e.TargetNodeID,
		Type:         e.Type,
		Container:    e.Container,
		Role:         e.Role,
		Count:        e.Count,
	}
}

func (m *GraphEdge) ToDomain() *domain.GraphEdge {
	return &domain.GraphEdge{
		ID:           m.ID,
		SourceNodeID: m.SourceNodeID,
		TargetNodeID: m.TargetNodeID,
		Type:         m.Type,
		Container:    m.Container,
		Role:         m.Role,
		Count:        m.Count,
	}
}

// GraphEdgeRow is the scan target of the denormalized edge listing: the edge
// columns joined with the source and target node columns.
type GraphEdgeRow struct {
	ID             string `gorm:"column:id"`
	Type           string `gorm:"column:type"`
	Container      string `gorm:"column:container"`
	Role           string `gorm:"column:role"`
	Count          int    `gorm:"column:count"`
	SourceNodeID   string `gorm:"column:source_node_id"`
	SourceURL      string `gorm:"column:source_url"`
	SourceFiletype string `gorm:"column:source_filetype"`
	TargetNodeID   string `gorm:"column:target_node_id"`
	TargetURL      string `gorm:"column:target_url"`
	TargetFiletype string `gorm:"column:target_filetype"`
	TargetExternal bool   `gorm:"column:target_external"`
}

func (m *GraphEdgeRow) ToDomain() *domain.GraphEdgeRow {
	return &domain.GraphEdgeRow{
		ID:             m.ID,
		Type:           m.Type,
		Container:      m.Container,
		Role:           m.Role,
		Count:          m.Count,
		SourceNodeID:   m.SourceNodeID,
		SourceURL:      m.SourceURL,
		SourceFiletype: m.SourceFiletype,
		TargetNodeID:   m.TargetNodeID,
		TargetURL:      m.TargetURL,
		TargetFiletype: m.TargetFiletype,
		TargetExternal: m.TargetExternal,
	}
}
