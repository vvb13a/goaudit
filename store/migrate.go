package store

import (
	"encoding/json"

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
