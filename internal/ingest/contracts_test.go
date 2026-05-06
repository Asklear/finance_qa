package ingest

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"financeqa/internal/dimensions"

	"github.com/xuri/excelize/v2"
	_ "modernc.org/sqlite"
)

func TestParseFundIncomeQuarterRowsCapturesRemarksColumn(t *testing.T) {
	rows := [][]string{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价（含税/元）", "1月", "", "", "", "", "2月", "", "", "", "", "3月", "", "", "", "", "26年Q1合计", "备注"},
		{"", "", "", "", "", "", "数量", "结算金额", "是否开票", "开票金额", "收款金额", "数量", "结算金额", "是否开票", "开票金额", "收款金额", "数量", "结算金额", "是否开票", "开票金额", "收款金额", "", ""},
		{"四川其妙科技有限公司", "行业商品数据采购合同-A01", "45895", "46259", "月度", "0.504", "1753565", "883796.76", "是", "1668149.01", "1668149.01", "2359525", "1189200.6", "否", "0", "0", "3182709", "1604085.34", "否", "0", "0", "3677082.7", "合同2026年8月到期"},
	}

	out, groups, cleanup := parseFundIncomeQuarterRows("26年Q1收入明细", rows, nil, nil)
	if len(cleanup) != 0 {
		t.Fatalf("cleanup = %#v", cleanup)
	}
	if len(groups) != 0 {
		t.Fatalf("groups = %#v", groups)
	}
	if len(out) != 3 {
		t.Fatalf("out length = %d, want 3", len(out))
	}
	gotRemarks := make([]string, 0, len(out))
	for _, row := range out {
		gotRemarks = append(gotRemarks, row.Remarks)
		if row.YearMonth == "2026-03" && row.Remarks != "合同2026年8月到期" {
			t.Fatalf("march row remarks = %q", row.Remarks)
		}
	}
	if !reflect.DeepEqual(gotRemarks, []string{"合同2026年8月到期", "合同2026年8月到期", "合同2026年8月到期"}) {
		t.Fatalf("remarks = %#v", gotRemarks)
	}
}

func TestImportContractWorkbookPersistsFundIncomeRemarksAndCellNotes(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "fund-income-remarks.sqlite")
	workbookPath := filepath.Join(t.TempDir(), "fund-income.xlsx")
	writeFundIncomeRemarksWorkbook(t, workbookPath)

	importer := NewImporter(dimensions.NewManager(nil))
	summary, err := importer.ImportFileWithOptions(ctx, dbPath, workbookPath, ImportOptions{})
	if err != nil {
		t.Fatalf("ImportFileWithOptions: %v", err)
	}
	if summary.RecordCount != 3 {
		t.Fatalf("record count = %d, want 3", summary.RecordCount)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var remarks, sourceCellNotes string
	if err := db.QueryRowContext(ctx, `
SELECT COALESCE(remarks, ''), COALESCE(source_cell_notes, '')
FROM fin_fund_income
WHERE year_month = '2026-03'
`).Scan(&remarks, &sourceCellNotes); err != nil {
		t.Fatalf("query fin_fund_income: %v", err)
	}
	if remarks != "合同2026年8月到期" {
		t.Fatalf("remarks = %q", remarks)
	}
	if sourceCellNotes == "" || !containsAll(sourceCellNotes, "F3", "批注仍要保留") {
		t.Fatalf("source_cell_notes = %q", sourceCellNotes)
	}
}

func writeFundIncomeRemarksWorkbook(t *testing.T, path string) {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	const sheet = "26年Q1收入明细"
	defaultSheet := f.GetSheetName(0)
	if err := f.SetSheetName(defaultSheet, sheet); err != nil {
		t.Fatalf("SetSheetName: %v", err)
	}
	values := map[string]any{
		"A1": "客户名称", "B1": "合同内容", "C1": "合同开始时间", "D1": "合同终止时间", "E1": "结算周期", "F1": "结算单价（含税/元）",
		"G1": "1月", "L1": "2月", "Q1": "3月", "V1": "26年Q1合计", "W1": "备注",
		"G2": "数量", "H2": "结算金额", "I2": "是否开票", "J2": "开票金额", "K2": "收款金额",
		"L2": "数量", "M2": "结算金额", "N2": "是否开票", "O2": "开票金额", "P2": "收款金额",
		"Q2": "数量", "R2": "结算金额", "S2": "是否开票", "T2": "开票金额", "U2": "收款金额",
		"A3": "四川其妙科技有限公司", "B3": "行业商品数据采购合同-A01", "C3": "2026-01-01", "D3": "2026-12-31", "E3": "月度", "F3": "0.504",
		"G3": "10", "H3": "100", "I3": "是", "J3": "100", "K3": "100",
		"L3": "20", "M3": "200", "N3": "否", "O3": "0", "P3": "0",
		"Q3": "30", "R3": "300", "S3": "否", "T3": "0", "U3": "0", "V3": "600", "W3": "合同2026年8月到期",
	}
	for cell, value := range values {
		if err := f.SetCellValue(sheet, cell, value); err != nil {
			t.Fatalf("SetCellValue %s: %v", cell, err)
		}
	}
	if err := f.AddComment(sheet, excelize.Comment{Cell: "F3", Author: "李旭", Text: "批注仍要保留"}); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}
