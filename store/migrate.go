package store

import (
	"encoding/json"
	"strings"

	"gorm.io/gorm"
)

// migrateRemovedInfoSeverity applies the one-time cleanup after the "info"
// severity was removed. The only producer of info issues, transfer_stats_log,
// is gone, so its rows are deleted; the info key is dropped from the stored
// severity_counts JSON; and URLs whose highest severity was info are re-tagged
// as success (info was never a failure, so their score is unchanged). The
// migration is idempotent and safe to run on every startup.
func migrateRemovedInfoSeverity(db *gorm.DB) error {
	if err := db.Exec("DELETE FROM issues WHERE severity = ?", "info").Error; err != nil {
		return err
	}

	for _, spec := range []struct{ table, column string }{
		{"audited_urls", "severity_counts"},
		{"audit_snapshots", "severity_counts"},
	} {
		if err := dropJSONKey(db, spec.table, spec.column, "info"); err != nil {
			return err
		}
	}

	return db.Exec(
		"UPDATE audited_urls SET highest_severity = ? WHERE highest_severity = ?",
		"success", "info",
	).Error
}

// migrateRemovedTransferStatsCheck strips the deleted transfer_stats_log
// check from the stored check_names of every audit, so reruns resolve against
// the current registry. It is idempotent.
func migrateRemovedTransferStatsCheck(db *gorm.DB) error {
	return removeStringFromJSONArray(db, "audits", "check_names", "transfer_stats_log")
}

// migrateRemovedInternalLinksCheck strips the removed internal_links check from
// the stored check_names of every audit. The external_links name is kept: the
// new graph check reuses it. It is idempotent.
func migrateRemovedInternalLinksCheck(db *gorm.DB) error {
	return removeStringFromJSONArray(db, "audits", "check_names", "internal_links")
}

// migrateGraphNodeValidation backfills the status_code/last_validated columns
// of graph_nodes, which were added after the table already existed. Non-audited
// nodes take their result from the global link_targets table, audited pages
// from the fetch recorded in audited_urls. It is idempotent.
func migrateGraphNodeValidation(db *gorm.DB) error {
	if err := db.Exec(`
		UPDATE graph_nodes
		SET status_code = (SELECT t.status_code FROM link_targets t WHERE t.url = graph_nodes.url),
		    last_validated = (SELECT t.validated_at FROM link_targets t WHERE t.url = graph_nodes.url)
		WHERE EXISTS (SELECT 1 FROM link_targets t WHERE t.url = graph_nodes.url)
	`).Error; err != nil {
		return err
	}
	return db.Exec(`
		UPDATE graph_nodes
		SET status_code = (SELECT a.status_code FROM audited_urls a
			WHERE a.audit_id = graph_nodes.audit_id
			  AND (a.final_url = graph_nodes.url OR a.url = graph_nodes.url)
			ORDER BY a.last_audited_at DESC LIMIT 1),
		    last_validated = (SELECT a.last_audited_at FROM audited_urls a
			WHERE a.audit_id = graph_nodes.audit_id
			  AND (a.final_url = graph_nodes.url OR a.url = graph_nodes.url)
			ORDER BY a.last_audited_at DESC LIMIT 1)
		WHERE audited = 1
		  AND EXISTS (SELECT 1 FROM audited_urls a
			WHERE a.audit_id = graph_nodes.audit_id
			  AND (a.final_url = graph_nodes.url OR a.url = graph_nodes.url))
	`).Error
}

// migrateGraphEdgeContext rebuilds the graph_edges unique index to include the
// container and role columns, which were added after the table already existed.
// GORM AutoMigrate adds the columns but leaves the old index in place, so an
// edge linking the same pair from two containers would collide. The migration
// is a no-op once the index already covers the new columns; edges are rebuilt
// on the next run, so their stale rows are left untouched.
func migrateGraphEdgeContext(db *gorm.DB) error {
	var rows []struct{ SQL string }
	if err := db.Raw("SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?", "idx_graph_edges_unique").
		Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 || strings.Contains(rows[0].SQL, "container") {
		return nil
	}
	if err := db.Exec("DROP INDEX idx_graph_edges_unique").Error; err != nil {
		return err
	}
	return db.Exec(
		"CREATE UNIQUE INDEX idx_graph_edges_unique ON graph_edges " +
			"(audit_id, source_node_id, target_node_id, edge_type, container, role)",
	).Error
}

// migrateJunkGraphNodes removes graph nodes whose URL is markup that leaked out
// of a srcset (encoded angle brackets, e.g. "%3Csvg..."), together with their
// edges. New graphs never contain these because extraction filters them; this
// cleans up graphs built before that filter existed. It is idempotent.
func migrateJunkGraphNodes(db *gorm.DB) error {
	const junk = "instr(lower(url), '%3c') > 0 OR instr(lower(url), '%3e') > 0"
	if err := db.Exec(`
		DELETE FROM graph_edges
		WHERE source_node_id IN (SELECT id FROM graph_nodes WHERE ` + junk + `)
		   OR target_node_id IN (SELECT id FROM graph_nodes WHERE ` + junk + `)
	`).Error; err != nil {
		return err
	}
	return db.Exec("DELETE FROM graph_nodes WHERE " + junk).Error
}

// removeStringFromJSONArray drops one value from a JSON-encoded string array
// stored in a text column.
func removeStringFromJSONArray(db *gorm.DB, table, column, value string) error {
	var rows []jsonKeyRow
	if err := db.Table(table).
		Select("id, "+column+" AS raw").
		Where(column+" LIKE ?", "%\""+value+"\"%").
		Scan(&rows).Error; err != nil {
		return err
	}

	for _, row := range rows {
		var values []string
		if err := json.Unmarshal([]byte(row.Raw), &values); err != nil {
			continue
		}
		kept := values[:0]
		for _, v := range values {
			if v != value {
				kept = append(kept, v)
			}
		}
		if len(kept) == len(values) {
			continue
		}
		raw, err := json.Marshal(kept)
		if err != nil {
			continue
		}
		if err := db.Table(table).Where("id = ?", row.ID).Update(column, string(raw)).Error; err != nil {
			return err
		}
	}
	return nil
}

type jsonKeyRow struct {
	ID  string
	Raw string
}

// dropJSONKey removes one key from a JSON object stored in a text column of
// the given table, leaving every other key untouched.
func dropJSONKey(db *gorm.DB, table, column, key string) error {
	var rows []jsonKeyRow
	if err := db.Table(table).
		Select("id, "+column+" AS raw").
		Where(column+" LIKE ?", "%\""+key+"\"%").
		Scan(&rows).Error; err != nil {
		return err
	}

	for _, row := range rows {
		var fields map[string]any
		if err := json.Unmarshal([]byte(row.Raw), &fields); err != nil {
			continue
		}
		if _, ok := fields[key]; !ok {
			continue
		}
		delete(fields, key)

		raw, err := json.Marshal(fields)
		if err != nil {
			continue
		}
		if err := db.Table(table).Where("id = ?", row.ID).Update(column, string(raw)).Error; err != nil {
			return err
		}
	}
	return nil
}
