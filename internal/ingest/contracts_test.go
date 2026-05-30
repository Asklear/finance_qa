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

func TestParseFundIncomeQuarterRowsExtractsInvoiceOpenOffsetFromCellNotes(t *testing.T) {
	rows := [][]string{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价（含税/元）", "1月", "", "", "", "", "2月", "", "", "", "", "3月", "", "", "", "", "26年Q1合计", "备注"},
		{"", "", "", "", "", "", "数量", "结算金额", "是否开票", "开票金额", "收款金额", "数量", "结算金额", "是否开票", "开票金额", "收款金额", "数量", "结算金额", "是否开票", "开票金额", "收款金额", "", ""},
		{"倍壮（上海）信息技术有限公司", "信息服务协议", "2025-11-01", "2026-10-31", "月度", "0.05", "117085", "8854.25", "是", "11854.25", "8854.25", "", "", "", "", "", "", "", "", "", "", "12593.05", "合同2026年10月到期"},
	}
	cellNotes := contractSourceCellNotes{
		"J3": {Author: "28527", Text: "3000元2025年11月已经收到但当时未开票"},
	}

	out, groups, cleanup := parseFundIncomeQuarterRows("26年Q1收入明细", rows, nil, cellNotes)
	if len(cleanup) != 0 || len(groups) != 0 {
		t.Fatalf("groups=%#v cleanup=%#v", groups, cleanup)
	}
	if len(out) != 1 {
		t.Fatalf("out length = %d, want 1: %#v", len(out), out)
	}
	if got, want := out[0].InvoiceOpenOffsetAmount, float64(3000); got != want {
		t.Fatalf("invoice_open_offset_amount = %.2f, want %.2f", got, want)
	}
	if !strings.Contains(out[0].InvoiceOpenOffsetReason, "3000元2025年11月已经收到但当时未开票") {
		t.Fatalf("invoice_open_offset_reason = %q", out[0].InvoiceOpenOffsetReason)
	}
}

func TestContractInvoiceOpenOffsetIgnoresNonOffsetNumericComments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
	}{
		{
			name: "payment terms",
			text: "季度结束甲方5个工作日内提交结算单，5个工作日内进行确认，开票15个工作日内进行付款",
		},
		{
			name: "settlement schedule",
			text: "分两次结算，首次1,261,208.00元，第二次204,792.00元。具体付款节奏以吴总通知为准。",
		},
		{
			name: "fx estimate",
			text: "首期40%按照汇率6.8结算，剩余60%按6.8暂估；首期相当收到4.8个月",
		},
		{
			name: "received date without amount",
			text: "1月15日已收到",
		},
		{
			name: "cost payable difference",
			text: "付款金额与已开票差额 应付：876127.10",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			offset := contractInvoiceOpenOffsetFromNotes("", contractSourceCellNotes{
				"A1": {Text: tc.text},
			})
			if offset.Amount != 0 || offset.Reason != "" {
				t.Fatalf("offset = %#v, want no invoice-open offset for %q", offset, tc.text)
			}
		})
	}
}

func TestParseFundIncomeQuarterRowsSupportsSettlementReceivedLayout(t *testing.T) {
	rows := [][]string{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价（含税/元）", "10月", "", "11月", "", "12月", "", "Q4 ", "备注"},
		{"", "", "", "", "", "", "结算金额", "收款金额", "结算金额", "收款金额", "结算金额", "收款金额", "总收入", ""},
		{"辽宁金程信息科技有限公司", "行业商品数据采购合同-A01", "2025-08-26", "2026-08-25", "月度", "0.504", "100", "90", "200", "180", "300", "0", "600", "Q4新结构"},
	}

	out, groups, cleanup := parseFundIncomeQuarterRows("25年Q4收入明细", rows, nil, nil)
	if len(cleanup) != 0 {
		t.Fatalf("cleanup = %#v", cleanup)
	}
	if len(groups) != 0 {
		t.Fatalf("groups = %#v", groups)
	}
	if len(out) != 3 {
		t.Fatalf("out length = %d, want 3: %#v", len(out), out)
	}

	got := map[string]contractFundIncomeRow{}
	for _, row := range out {
		got[row.YearMonth] = row
		if row.IsInvoiced != "否" {
			t.Fatalf("%s is_invoiced = %q, want default 否", row.YearMonth, row.IsInvoiced)
		}
		if row.InvoiceAmount != 0 {
			t.Fatalf("%s invoice_amount = %v, want 0", row.YearMonth, row.InvoiceAmount)
		}
		if row.Quantity != "" {
			t.Fatalf("%s quantity = %q, want empty", row.YearMonth, row.Quantity)
		}
	}
	assertFundIncomeAmounts(t, got["2025-10"], 100, 90)
	assertFundIncomeAmounts(t, got["2025-11"], 200, 180)
	assertFundIncomeAmounts(t, got["2025-12"], 300, 0)
}

func TestParseFundIncomeQuarterRowsSupportsFutureQ4FullMetricLayout(t *testing.T) {
	rows := [][]string{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价（含税/元）", "10月", "", "", "", "", "11月", "", "", "", "", "12月", "", "", "", "", "25年Q4合计", "备注"},
		{"", "", "", "", "", "", "数量", "结算金额", "是否开票", "开票金额", "收款金额", "数量", "结算金额", "是否开票", "开票金额", "收款金额", "数量", "结算金额", "是否开票", "开票金额", "收款金额", "", ""},
		{"辽宁金程信息科技有限公司", "行业商品数据采购合同-A01", "2025-08-26", "2026-08-25", "月度", "0.504", "10", "100", "是", "100", "90", "20", "200", "否", "0", "180", "30", "300", "是", "300", "0", "600", "未来Q4完整结构"},
	}

	out, groups, cleanup := parseFundIncomeQuarterRows("25年Q4收入明细", rows, nil, nil)
	if len(cleanup) != 0 {
		t.Fatalf("cleanup = %#v", cleanup)
	}
	if len(groups) != 0 {
		t.Fatalf("groups = %#v", groups)
	}
	if len(out) != 3 {
		t.Fatalf("out length = %d, want 3: %#v", len(out), out)
	}

	got := map[string]contractFundIncomeRow{}
	for _, row := range out {
		got[row.YearMonth] = row
		if row.Remarks != "未来Q4完整结构" {
			t.Fatalf("%s remarks = %q", row.YearMonth, row.Remarks)
		}
	}
	assertFundIncomeFullMetricRow(t, got["2025-10"], "10", 100, "是", 100, 90)
	assertFundIncomeFullMetricRow(t, got["2025-11"], "20", 200, "否", 0, 180)
	assertFundIncomeFullMetricRow(t, got["2025-12"], "30", 300, "是", 300, 0)
}

func TestParseFundIncomeQuarterRowsSupportsSettlementReceivedLayoutMergedGroups(t *testing.T) {
	rows := [][]string{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价（含税/元）", "10月", "", "11月", "", "12月", "", "Q4 ", "备注"},
		{"", "", "", "", "", "", "结算金额", "收款金额", "结算金额", "收款金额", "结算金额", "收款金额", "总收入", ""},
		{"Yipit,LLC", "数据采购合同-快手", "2025-10-01", "2026-09-30", "季度", "固定金额", "100", "90", "200", "180", "300", "0", "600", ""},
		{"Yipit,LLC", "数据采购合同-抖音", "2025-10-10", "2026-10-09", "季度", "固定金额", "", "", "", "", "", "", "", ""},
		{"Yipit,LLC", "数据采购合同-shopee", "2025-09-21", "2026-09-20", "季度", "固定金额", "", "", "", "", "", "", "", ""},
		{"Yipit,LLC", "数据采购合同-Tmal", "2025-10-13", "2026-10-12", "季度", "固定金额", "", "", "", "", "", "", "", ""},
	}
	mergedRanges := []contractMergedCellRange{
		{StartRow: 2, EndRow: 5, StartCol: 6, EndCol: 6},
		{StartRow: 2, EndRow: 5, StartCol: 7, EndCol: 7},
		{StartRow: 2, EndRow: 5, StartCol: 8, EndCol: 8},
		{StartRow: 2, EndRow: 5, StartCol: 9, EndCol: 9},
		{StartRow: 2, EndRow: 5, StartCol: 10, EndCol: 10},
		{StartRow: 2, EndRow: 5, StartCol: 11, EndCol: 11},
	}

	out, groups, cleanup := parseFundIncomeQuarterRows("25年Q4收入明细", rows, mergedRanges, nil)
	if len(out) != 0 {
		t.Fatalf("out = %#v, want only grouped rows", out)
	}
	if len(groups) != 3 {
		t.Fatalf("groups length = %d, want 3: %#v", len(groups), groups)
	}
	if len(cleanup) != 12 {
		t.Fatalf("cleanup length = %d, want 12: %#v", len(cleanup), cleanup)
	}

	got := map[string]contractFundIncomeGroupRow{}
	for _, row := range groups {
		got[row.YearMonth] = row
		if row.IsInvoiced != "否" {
			t.Fatalf("%s is_invoiced = %q, want default 否", row.YearMonth, row.IsInvoiced)
		}
		if row.InvoiceAmount != 0 {
			t.Fatalf("%s invoice_amount = %v, want 0", row.YearMonth, row.InvoiceAmount)
		}
		if len(row.Members) != 4 {
			t.Fatalf("%s members length = %d, want 4: %#v", row.YearMonth, len(row.Members), row.Members)
		}
	}
	assertFundIncomeGroupAmounts(t, got["2025-10"], 100, 90)
	assertFundIncomeGroupAmounts(t, got["2025-11"], 200, 180)
	assertFundIncomeGroupAmounts(t, got["2025-12"], 300, 0)
}

func TestParseFundIncomeQuarterRowsKeepsRemarksOnlyRows(t *testing.T) {
	rows := [][]string{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价（含税/元）", "1月", "", "", "", "", "2月", "", "", "", "", "3月", "", "", "", "", "26年Q1合计", "备注"},
		{"", "", "", "", "", "", "数量", "结算金额", "是否开票", "开票金额", "收款金额", "数量", "结算金额", "是否开票", "开票金额", "收款金额", "数量", "结算金额", "是否开票", "开票金额", "收款金额", "", ""},
		{"京东", "对应-众信成本", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "谈判中"},
		{"Yipit,LLC", "数据采购合同-抖音", "45940", "46304", "季度", "固定金额", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "25875"},
		{"Yipit,LLC", "数据采购合同-shopee", "45921", "46285", "季度", "固定金额", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "37500"},
		{"Yipit,LLC", "数据采购合同-Tmal", "45943", "46307", "季度", "固定金额", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "61625"},
	}

	out, groups, cleanup := parseFundIncomeQuarterRows("26年Q1收入明细", rows, nil, nil)
	if len(cleanup) != 0 || len(groups) != 0 {
		t.Fatalf("groups=%#v cleanup=%#v", groups, cleanup)
	}
	if len(out) != 4 {
		t.Fatalf("out length = %d, want 4: %#v", len(out), out)
	}
	got := map[string]contractFundIncomeRow{}
	for _, row := range out {
		got[row.Content] = row
	}
	for content, wantRemark := range map[string]string{
		"对应-众信成本":       "谈判中",
		"数据采购合同-抖音":     "25875",
		"数据采购合同-shopee": "37500",
		"数据采购合同-Tmal":   "61625",
	} {
		row, ok := got[content]
		if !ok {
			t.Fatalf("missing content %q in %#v", content, out)
		}
		if row.YearMonth != "2026-03" || row.Remarks != wantRemark {
			t.Fatalf("%s year_month=%q remarks=%q", content, row.YearMonth, row.Remarks)
		}
		if row.SettlementAmount != 0 || row.ReceivedAmount != 0 || row.InvoiceAmount != 0 {
			t.Fatalf("%s amounts settlement=%v received=%v invoice=%v", content, row.SettlementAmount, row.ReceivedAmount, row.InvoiceAmount)
		}
	}
}

func assertFundIncomeAmounts(t *testing.T, row contractFundIncomeRow, settlement, received float64) {
	t.Helper()
	if row.SettlementAmount != settlement || row.ReceivedAmount != received {
		t.Fatalf("%s amounts settlement=%v received=%v, want settlement=%v received=%v", row.YearMonth, row.SettlementAmount, row.ReceivedAmount, settlement, received)
	}
}

func assertFundIncomeGroupAmounts(t *testing.T, row contractFundIncomeGroupRow, settlement, received float64) {
	t.Helper()
	if row.SettlementAmount != settlement || row.ReceivedAmount != received {
		t.Fatalf("%s group amounts settlement=%v received=%v, want settlement=%v received=%v", row.YearMonth, row.SettlementAmount, row.ReceivedAmount, settlement, received)
	}
}

func assertFundIncomeFullMetricRow(t *testing.T, row contractFundIncomeRow, quantity string, settlement float64, isInvoiced string, invoice float64, received float64) {
	t.Helper()
	if row.Quantity != quantity || row.SettlementAmount != settlement || row.IsInvoiced != isInvoiced || row.InvoiceAmount != invoice || row.ReceivedAmount != received {
		t.Fatalf("%s row = quantity=%q settlement=%v is_invoiced=%q invoice=%v received=%v, want quantity=%q settlement=%v is_invoiced=%q invoice=%v received=%v",
			row.YearMonth, row.Quantity, row.SettlementAmount, row.IsInvoiced, row.InvoiceAmount, row.ReceivedAmount,
			quantity, settlement, isInvoiced, invoice, received)
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

func TestImportContractWorkbookPersistsFutureQ4FullMetricLayout(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "future-q4-full-layout.sqlite")
	workbookPath := filepath.Join(t.TempDir(), "future-q4-full-layout.xlsx")
	writeFutureQ4FullMetricFundWorkbook(t, workbookPath)

	importer := NewImporter(dimensions.NewManager(nil))
	summary, err := importer.ImportFileWithOptions(ctx, dbPath, workbookPath, ImportOptions{})
	if err != nil {
		t.Fatalf("ImportFileWithOptions: %v", err)
	}
	if summary.RecordCount != 3 {
		t.Fatalf("record count = %d, want 3", summary.RecordCount)
	}
	if summary.PeriodStart != "2025-10" || summary.PeriodEnd != "2025-12" {
		t.Fatalf("period = %s~%s, want 2025-10~2025-12", summary.PeriodStart, summary.PeriodEnd)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	rows, err := db.QueryContext(ctx, `
SELECT year_month, quantity, settlement_amount, is_invoiced, invoice_amount, received_amount
FROM fin_fund_income
ORDER BY year_month
`)
	if err != nil {
		t.Fatalf("query fin_fund_income: %v", err)
	}
	defer func() { _ = rows.Close() }()

	got := map[string]contractFundIncomeRow{}
	for rows.Next() {
		var row contractFundIncomeRow
		if err := rows.Scan(&row.YearMonth, &row.Quantity, &row.SettlementAmount, &row.IsInvoiced, &row.InvoiceAmount, &row.ReceivedAmount); err != nil {
			t.Fatalf("scan fin_fund_income: %v", err)
		}
		got[row.YearMonth] = row
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate fin_fund_income: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("row count = %d, want 3: %#v", len(got), got)
	}
	assertFundIncomeFullMetricRow(t, got["2025-10"], "10", 100, "是", 100, 90)
	assertFundIncomeFullMetricRow(t, got["2025-11"], "20", 200, "否", 0, 180)
	assertFundIncomeFullMetricRow(t, got["2025-12"], "30", 300, "是", 300, 0)
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

func writeFutureQ4FullMetricFundWorkbook(t *testing.T, path string) {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	const sheet = "25年Q4收入明细"
	defaultSheet := f.GetSheetName(0)
	if err := f.SetSheetName(defaultSheet, sheet); err != nil {
		t.Fatalf("SetSheetName: %v", err)
	}
	rows := [][]any{
		{"客户名称", "合同内容", "合同开始时间", "合同终止时间", "结算周期", "结算单价（含税/元）", "10月", "", "", "", "", "11月", "", "", "", "", "12月", "", "", "", "", "25年Q4合计", "备注"},
		{"", "", "", "", "", "", "数量", "结算金额", "是否开票", "开票金额", "收款金额", "数量", "结算金额", "是否开票", "开票金额", "收款金额", "数量", "结算金额", "是否开票", "开票金额", "收款金额", "", ""},
		{"辽宁金程信息科技有限公司", "行业商品数据采购合同-A01", "2025-08-26", "2026-08-25", "月度", "0.504", "10", "100", "是", "100", "90", "20", "200", "否", "0", "180", "30", "300", "是", "300", "0", "600", "未来Q4完整结构"},
	}
	for rIdx, row := range rows {
		for cIdx, value := range row {
			cell, err := excelize.CoordinatesToCellName(cIdx+1, rIdx+1)
			if err != nil {
				t.Fatalf("CoordinatesToCellName: %v", err)
			}
			if err := f.SetCellValue(sheet, cell, value); err != nil {
				t.Fatalf("SetCellValue %s: %v", cell, err)
			}
		}
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
