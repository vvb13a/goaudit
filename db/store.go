package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/vvb13a/goaudit/data"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

func Open(path string) (*DB, error) {
	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to run database migrations: %w", err)
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS plans (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		urls TEXT NOT NULL,
		created_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS checklists (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		check_names TEXT NOT NULL,
		is_default BOOLEAN NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS inspections (
		id TEXT PRIMARY KEY,
		plan_id TEXT,
		plan_name TEXT NOT NULL,
		checklist_id TEXT,
		checklist_name TEXT NOT NULL,
		started_at DATETIME NOT NULL,
		duration_ms INTEGER NOT NULL,
		total_endpoints INTEGER NOT NULL,
		passed_count INTEGER NOT NULL,
		failed_count INTEGER NOT NULL,
		highest_severity TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS reports (
		id TEXT PRIMARY KEY,
		inspection_id TEXT NOT NULL REFERENCES inspections(id) ON DELETE CASCADE,
		url TEXT NOT NULL,
		final_url TEXT NOT NULL,
		status_code INTEGER NOT NULL,
		duration_ms INTEGER NOT NULL,
		passed_count INTEGER NOT NULL,
		failed_count INTEGER NOT NULL,
		highest_severity TEXT NOT NULL,
		issues TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_reports_insp ON reports(inspection_id);
	`
	_, err := db.conn.Exec(schema)
	return err
}

// -------------------------------------------------------------------------
// Plan Helpers
// -------------------------------------------------------------------------

func (db *DB) SavePlan(p *data.Plan) error {
	if p.ID == "" {
		p.ID = fmt.Sprintf("plan_%d", time.Now().UnixNano())
		p.CreatedAt = time.Now()
	}
	urlsJSON, _ := json.Marshal(p.URLs)
	query := `INSERT INTO plans (id, name, urls, created_at) VALUES (?, ?, ?, ?)
	          ON CONFLICT(id) DO UPDATE SET name=excluded.name, urls=excluded.urls`
	_, err := db.conn.Exec(query, p.ID, p.Name, string(urlsJSON), p.CreatedAt)
	return err
}

func (db *DB) ListPlans() ([]*data.Plan, error) {
	rows, err := db.conn.Query(`SELECT id, name, urls, created_at FROM plans ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plans []*data.Plan
	for rows.Next() {
		var p data.Plan
		var raw string
		if err := rows.Scan(&p.ID, &p.Name, &raw, &p.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(raw), &p.URLs)
		plans = append(plans, &p)
	}
	return plans, nil
}

func (db *DB) DeletePlan(id string) error {
	_, err := db.conn.Exec(`DELETE FROM plans WHERE id = ?`, id)
	return err
}

// -------------------------------------------------------------------------
// Checklist Helpers
// -------------------------------------------------------------------------

func (db *DB) SaveChecklist(c *data.Checklist) error {
	if c.ID == "" {
		c.ID = fmt.Sprintf("chk_%d", time.Now().UnixNano())
		c.CreatedAt = time.Now()
	}
	checksJSON, _ := json.Marshal(c.CheckNames)

	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if c.IsDefault {
		if _, err := tx.Exec(`UPDATE checklists SET is_default = 0`); err != nil {
			return err
		}
	}

	query := `INSERT INTO checklists (id, name, check_names, is_default, created_at) VALUES (?, ?, ?, ?, ?)
	          ON CONFLICT(id) DO UPDATE SET name=excluded.name, check_names=excluded.check_names, is_default=excluded.is_default`
	if _, err := tx.Exec(query, c.ID, c.Name, string(checksJSON), c.IsDefault, c.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) ListChecklists() ([]*data.Checklist, error) {
	rows, err := db.conn.Query(`SELECT id, name, check_names, is_default, created_at FROM checklists ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checklists []*data.Checklist
	for rows.Next() {
		var c data.Checklist
		var raw string
		if err := rows.Scan(&c.ID, &c.Name, &raw, &c.IsDefault, &c.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(raw), &c.CheckNames)
		checklists = append(checklists, &c)
	}
	return checklists, nil
}

func (db *DB) SetDefaultChecklist(id string) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE checklists SET is_default = 0`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE checklists SET is_default = 1 WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) DeleteChecklist(id string) error {
	_, err := db.conn.Exec(`DELETE FROM checklists WHERE id = ?`, id)
	return err
}

// -------------------------------------------------------------------------
// Inspection & Report Helpers
// -------------------------------------------------------------------------

func (db *DB) SaveInspection(insp *data.Inspection) error {
	if insp.ID == "" {
		insp.ID = fmt.Sprintf("insp_%d", time.Now().UnixNano())
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	inspQuery := `INSERT INTO inspections (
		id, plan_id, plan_name, checklist_id, checklist_name,
		started_at, duration_ms, total_endpoints, passed_count, failed_count, highest_severity
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = tx.Exec(inspQuery,
		insp.ID, insp.PlanID, insp.PlanName, insp.ChecklistID, insp.ChecklistName,
		insp.StartedAt, insp.Duration.Milliseconds(), insp.TotalEndpoints,
		insp.PassedCount, insp.FailedCount, string(insp.HighestSeverity),
	)
	if err != nil {
		return err
	}

	repQuery := `INSERT INTO reports (
		id, inspection_id, url, final_url, status_code,
		duration_ms, passed_count, failed_count, highest_severity, issues
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	for i, r := range insp.Reports {
		reportID := fmt.Sprintf("%s_r_%d", insp.ID, i+1)
		issuesJSON, _ := json.Marshal(r.Issues)
		_, err = tx.Exec(repQuery,
			reportID, insp.ID, r.URL, r.FinalURL, r.StatusCode,
			r.Duration.Milliseconds(), r.Summary.PassedCount, r.Summary.FailedCount,
			string(r.Summary.HighestSeverity), string(issuesJSON),
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) ListInspections() ([]*data.Inspection, error) {
	query := `SELECT id, plan_id, plan_name, checklist_id, checklist_name,
	                 started_at, duration_ms, total_endpoints, passed_count, failed_count, highest_severity
	          FROM inspections ORDER BY started_at DESC`
	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*data.Inspection
	for rows.Next() {
		var insp data.Inspection
		var durationMs int64
		var sev string
		var planID, checklistID sql.NullString

		if err := rows.Scan(&insp.ID, &planID, &insp.PlanName, &checklistID, &insp.ChecklistName,
			&insp.StartedAt, &durationMs, &insp.TotalEndpoints, &insp.PassedCount, &insp.FailedCount, &sev); err != nil {
			return nil, err
		}

		insp.PlanID = planID.String
		insp.ChecklistID = checklistID.String
		insp.Duration = time.Duration(durationMs) * time.Millisecond
		insp.HighestSeverity = data.Severity(sev)

		reports, _ := db.loadReports(insp.ID)
		insp.Reports = reports

		list = append(list, &insp)
	}
	return list, nil
}

func (db *DB) loadReports(inspectionID string) ([]*data.Report, error) {
	rows, err := db.conn.Query(`SELECT url, final_url, status_code, duration_ms, passed_count, failed_count, highest_severity, issues
	                            FROM reports WHERE inspection_id = ?`, inspectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []*data.Report
	for rows.Next() {
		var r data.Report
		var durationMs int64
		var sev, raw string

		if err := rows.Scan(&r.URL, &r.FinalURL, &r.StatusCode, &durationMs,
			&r.Summary.PassedCount, &r.Summary.FailedCount, &sev, &raw); err != nil {
			return nil, err
		}

		r.Duration = time.Duration(durationMs) * time.Millisecond
		r.Summary.HighestSeverity = data.Severity(sev)
		_ = json.Unmarshal([]byte(raw), &r.Issues)
		r.Summary.TotalIssues = len(r.Issues)

		reports = append(reports, &r)
	}
	return reports, nil
}

func (db *DB) DeleteInspection(id string) error {
	_, err := db.conn.Exec(`DELETE FROM inspections WHERE id = ?`, id)
	return err
}
