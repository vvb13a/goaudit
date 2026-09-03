package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/vvb13a/goaudit/domain"
)

const (
	sheetSummary = "Summary"
	sheetReports = "Reports"
	sheetIssues  = "Issues"
)

// ExcelService renders domain audits as styled Excel workbooks: a summary
// sheet with run metadata and issue totals, one row per audited endpoint,
// and one row per issue.
type ExcelService struct{}

func NewExcelService() *ExcelService {
	return &ExcelService{}
}

// Export renders the audit into an in-memory xlsx workbook and returns its
// raw bytes.
func (s *ExcelService) Export(a *domain.Audit) ([]byte, error) {
	f, err := s.build(a)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("encode workbook: %w", err)
	}
	return data.Bytes(), nil
}

// ExportTo renders the audit into an xlsx file at the given path.
func (s *ExcelService) ExportTo(a *domain.Audit, path string) error {
	f, err := s.build(a)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := f.SaveAs(path); err != nil {
		return fmt.Errorf("save workbook: %w", err)
	}
	return nil
}

func (s *ExcelService) build(a *domain.Audit) (f *excelize.File, err error) {
	if a == nil {
		return nil, domain.ErrInvalidAudit
	}

	f = excelize.NewFile()
	defer func() {
		if err != nil {
			f.Close()
		}
	}()

	if err = f.SetSheetName("Sheet1", sheetSummary); err != nil {
		return nil, err
	}
	if _, err = f.NewSheet(sheetReports); err != nil {
		return nil, err
	}
	if _, err = f.NewSheet(sheetIssues); err != nil {
		return nil, err
	}

	headerStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "#FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#4472C4"}},
	})
	if err != nil {
		return nil, err
	}
	titleStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 14},
	})
	if err != nil {
		return nil, err
	}
	labelStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
	})
	if err != nil {
		return nil, err
	}

	if err = s.writeSummary(f, a, titleStyle, labelStyle); err != nil {
		return nil, err
	}
	if err = s.writeReports(f, a, headerStyle); err != nil {
		return nil, err
	}
	if err = s.writeIssues(f, a, headerStyle); err != nil {
		return nil, err
	}

	return f, nil
}

func (s *ExcelService) writeSummary(f *excelize.File, a *domain.Audit, titleStyle, labelStyle int) error {
	meta := [][2]string{
		{"Plan", a.PlanName},
		{"Checklist", a.ChecklistName},
		{"Audit ID", a.ID},
		{"Started", formatTimestamp(a.StartedAt)},
		{"Duration", a.Duration.Round(time.Millisecond).String()},
		{"Endpoints audited", fmt.Sprintf("%d", len(a.Reports))},
		{"Failed endpoints", fmt.Sprintf("%d", a.Summary.FailedCount)},
	}

	if err := f.SetCellValue(sheetSummary, "A1", "Audit: "+a.PlanName); err != nil {
		return err
	}
	if err := f.SetCellStyle(sheetSummary, "A1", "B1", titleStyle); err != nil {
		return err
	}
	if err := f.MergeCell(sheetSummary, "A1", "B1"); err != nil {
		return err
	}

	row := 3
	for i, pair := range meta {
		cellA := fmt.Sprintf("A%d", row+i)
		cellB := fmt.Sprintf("B%d", row+i)
		if err := f.SetCellValue(sheetSummary, cellA, pair[0]); err != nil {
			return err
		}
		if err := f.SetCellStyle(sheetSummary, cellA, cellA, labelStyle); err != nil {
			return err
		}
		if err := f.SetCellValue(sheetSummary, cellB, pair[1]); err != nil {
			return err
		}
	}

	row += len(meta) + 1
	label := fmt.Sprintf("A%d", row)
	if err := f.SetCellValue(sheetSummary, label, "Issue Summary"); err != nil {
		return err
	}
	if err := f.SetCellStyle(sheetSummary, label, label, labelStyle); err != nil {
		return err
	}

	counts := severityCounts(a)

	headerRow := row + 1
	if err := setRow(f, sheetSummary, headerRow, []any{"Severity", "Issues"}); err != nil {
		return err
	}
	last := fmt.Sprintf("B%d", headerRow)
	if err := f.SetCellStyle(sheetSummary, fmt.Sprintf("A%d", headerRow), last, labelStyle); err != nil {
		return err
	}

	dataRow := headerRow + 1
	for _, sev := range severityRanking {
		display := severityLabel(sev)
		if err := setRow(f, sheetSummary, dataRow, []any{display, counts[sev]}); err != nil {
			return err
		}
		dataRow++
	}

	if err := f.SetColWidth(sheetSummary, "A", "A", 28); err != nil {
		return err
	}
	if err := f.SetColWidth(sheetSummary, "B", "B", 40); err != nil {
		return err
	}
	return nil
}

func (s *ExcelService) writeReports(f *excelize.File, a *domain.Audit, headerStyle int) error {
	headers := []string{"URL", "Final URL", "Result", "Status Code", "Duration (s)", "Issues", "Failed", "Passed", "Highest Severity"}
	if err := setRow(f, sheetReports, 1, rowOf(headers)); err != nil {
		return err
	}
	lastHeader := colName(len(headers)) + "1"
	if err := f.SetCellStyle(sheetReports, "A1", lastHeader, headerStyle); err != nil {
		return err
	}

	row := 2
	for _, rep := range a.Reports {
		result := "PASSED"
		if rep.Summary.FailedCount > 0 {
			result = "FAILED"
		}
		values := []any{
			rep.URL,
			rep.FinalURL,
			result,
			rep.StatusCode,
			rep.Duration.Seconds(),
			rep.Summary.TotalCount,
			rep.Summary.FailedCount,
			rep.Summary.PassedCount,
			severityLabel(rep.Summary.HighestSeverity),
		}
		if err := setRow(f, sheetReports, row, values); err != nil {
			return err
		}
		if err := setHyperlink(f, sheetReports, fmt.Sprintf("A%d", row), rep.URL); err != nil {
			return err
		}
		if err := setHyperlink(f, sheetReports, fmt.Sprintf("B%d", row), rep.FinalURL); err != nil {
			return err
		}
		row++
	}

	if err := f.SetPanes(sheetReports, &excelize.Panes{Freeze: true, YSplit: 1}); err != nil {
		return err
	}
	if err := f.AutoFilter(sheetReports, fmt.Sprintf("A1:%s%d", colName(len(headers)), row-1), nil); err != nil {
		return err
	}
	return s.setWidths(f, sheetReports, []float64{46, 42, 10, 12, 12, 9, 9, 9, 18})
}

func (s *ExcelService) writeIssues(f *excelize.File, a *domain.Audit, headerStyle int) error {
	headers := []string{"URL", "Severity", "Status", "Category", "Check", "Message", "Evidence"}
	if err := setRow(f, sheetIssues, 1, rowOf(headers)); err != nil {
		return err
	}
	lastHeader := colName(len(headers)) + "1"
	if err := f.SetCellStyle(sheetIssues, "A1", lastHeader, headerStyle); err != nil {
		return err
	}

	row := 2
	for _, rep := range a.Reports {
		sorted := sortedIssues(rep.Issues)
		for _, iss := range sorted {
			status := "FAILED"
			if iss.Passed {
				status = "PASSED"
			}
			evidence := ""
			if len(iss.Details) > 0 {
				if raw, err := json.Marshal(iss.Details); err == nil {
					evidence = string(raw)
				}
			}
			values := []any{
				rep.URL,
				severityLabel(iss.Severity),
				status,
				iss.Category.DisplayName(),
				iss.CheckName,
				iss.Message,
				evidence,
			}
			if err := setRow(f, sheetIssues, row, values); err != nil {
				return err
			}
			if err := setHyperlink(f, sheetIssues, fmt.Sprintf("A%d", row), rep.URL); err != nil {
				return err
			}
			row++
		}
	}

	if err := f.SetPanes(sheetIssues, &excelize.Panes{Freeze: true, YSplit: 1}); err != nil {
		return err
	}
	if err := f.AutoFilter(sheetIssues, fmt.Sprintf("A1:%s%d", colName(len(headers)), row-1), nil); err != nil {
		return err
	}
	return s.setWidths(f, sheetIssues, []float64{46, 10, 10, 18, 26, 70, 60})
}

func (s *ExcelService) setWidths(f *excelize.File, sheet string, widths []float64) error {
	for i, w := range widths {
		col := colName(i + 1)
		if err := f.SetColWidth(sheet, col, col, w); err != nil {
			return err
		}
	}
	return nil
}

func (s *ExcelService) style(cell string) {}

func setRow(f *excelize.File, sheet string, row int, values []any) error {
	start, err := excelize.CoordinatesToCellName(1, row)
	if err != nil {
		return err
	}
	return f.SetSheetRow(sheet, start, &values)
}

func rowOf(values []string) []any {
	anyValues := make([]any, len(values))
	for i, v := range values {
		anyValues[i] = v
	}
	return anyValues
}

func setHyperlink(f *excelize.File, sheet, cell, url string) error {
	if url == "" {
		return nil
	}
	return f.SetCellHyperLink(sheet, cell, url, "External")
}

func colName(index int) string {
	name, _ := excelize.ColumnNumberToName(index)
	return name
}

func formatTimestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04")
}

var severityRanking = []domain.Severity{
	domain.SeverityFatal,
	domain.SeverityError,
	domain.SeverityWarning,
	domain.SeverityNotice,
	domain.SeverityInfo,
	domain.SeveritySuccess,
}

// severityCounts tallies issues across all reports of the audit by severity.
func severityCounts(a *domain.Audit) map[domain.Severity]int {
	counts := make(map[domain.Severity]int)
	for _, rep := range a.Reports {
		for _, iss := range rep.Issues {
			counts[iss.Severity]++
		}
	}
	return counts
}

// sortedIssues returns the issues sorted by severity, highest first.
func sortedIssues(issues []domain.Issue) []domain.Issue {
	out := make([]domain.Issue, len(issues))
	copy(out, issues)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Severity.Weight() > out[j].Severity.Weight()
	})
	return out
}

func severityLabel(s domain.Severity) string {
	switch s {
	case domain.SeverityFatal:
		return "FATAL"
	case domain.SeverityError:
		return "ERROR"
	case domain.SeverityWarning:
		return "WARN"
	case domain.SeverityNotice:
		return "NOTICE"
	case domain.SeverityInfo:
		return "INFO"
	case domain.SeveritySuccess:
		return "PASS"
	default:
		return string(s)
	}
}
