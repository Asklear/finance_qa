package parser

import "testing"

func TestParseIncomeStatementRowsExtractsCompanyFromSheet(t *testing.T) {
	t.Parallel()

	rows := [][]string{
		{},
		{"单位名称：南京优集数据科技有限公司"},
		{},
		{"一、营业收入", "100.50", "300.50"},
	}
	meta := FileMetadata{Company: "DefaultCompany", PeriodEnd: "2025-12"}

	out := parseIncomeStatementRows(rows, meta)
	if len(out) != 1 {
		t.Fatalf("row count = %d, want 1", len(out))
	}
	if got := out[0]["company"]; got != "南京优集数据科技有限公司" {
		t.Fatalf("company = %v", got)
	}
	if got := out[0]["current_amount"]; got != 100.50 {
		t.Fatalf("current_amount = %v", got)
	}
	if got := out[0]["cumulative_amount"]; got != 300.50 {
		t.Fatalf("cumulative_amount = %v", got)
	}
}

func TestParseBalanceSheetRowsParsesBothSides(t *testing.T) {
	t.Parallel()

	rows := [][]string{
		{},
		{"单位名称：南京优集数据科技有限公司"},
		{},
		{},
		{"货币资金", "120", "100", "应付账款", "220", "200"},
	}
	meta := FileMetadata{Company: "DefaultCompany", PeriodEnd: "2025-12"}

	out := parseBalanceSheetRows(rows, meta)
	if len(out) != 2 {
		t.Fatalf("row count = %d, want 2", len(out))
	}
	if got := out[0]["account_name"]; got != "货币资金" {
		t.Fatalf("left account name = %v", got)
	}
	if got := out[0]["closing_balance"]; got != 120.0 {
		t.Fatalf("left closing balance = %v", got)
	}
	if got := out[1]["account_name"]; got != "应付账款" {
		t.Fatalf("right account name = %v", got)
	}
	if got := out[1]["opening_balance"]; got != 200.0 {
		t.Fatalf("right opening balance = %v", got)
	}
}

func TestParseBalanceDetailRowsDetectsOpeningPeriodAndSkipsSummary(t *testing.T) {
	t.Parallel()

	rows := [][]string{
		{"会计年度", "2025.08-2025.12"},
		{"2025", "12", "600101", "技术服务费", "", "1", "2", "3", "4", "5", "6"},
		{"2025", "12", "合计", "总计", "", "0", "0", "0", "0", "0", "0"},
	}
	meta := FileMetadata{Company: "南京优集数据科技有限公司", PeriodEnd: "2025-12"}

	out := parseBalanceDetailRows(rows, meta)
	if len(out) != 1 {
		t.Fatalf("row count = %d, want 1", len(out))
	}
	if got := out[0]["opening_period"]; got != "2025-08" {
		t.Fatalf("opening_period = %v", got)
	}
	if got := out[0]["period"]; got != "2025-12" {
		t.Fatalf("period = %v", got)
	}
	if got := out[0]["account_level"]; got != 2 {
		t.Fatalf("account_level = %v", got)
	}
}

func TestParseJournalRowsNormalizesSummaryAndDebitCredit(t *testing.T) {
	t.Parallel()

	rows := [][]string{
		{"日期", "凭证号数", "科目编码", "科目名称", "摘要", "方向", "数量", "外币", "金额"},
		{"2025.12.31", "记-0001", "100201", "招商银行", "收到\r\n客户回款", "借", "", "", "123.45"},
		{"2025.12.31", "记-0001", "112201", "应收账款", "冲销应收", "贷", "", "", "123.45"},
	}
	meta := FileMetadata{Company: "南京优集数据科技有限公司"}

	out := parseJournalRows(rows, meta)
	if len(out) != 2 {
		t.Fatalf("row count = %d, want 2", len(out))
	}
	if got := out[0]["summary"]; got != "收到客户回款" {
		t.Fatalf("summary = %v", got)
	}
	if got := out[0]["debit_amount"]; got != 123.45 {
		t.Fatalf("debit_amount = %v", got)
	}
	if got := out[1]["credit_amount"]; got != 123.45 {
		t.Fatalf("credit_amount = %v", got)
	}
	if got := out[0]["period"]; got != "2025-12" {
		t.Fatalf("period = %v", got)
	}
}

func TestPeriodHelpersNormalizeRanges(t *testing.T) {
	t.Parallel()

	start, end := parsePeriodRangeText("2025.8-2025.12")
	if start != "2025-08" || end != "2025-12" {
		t.Fatalf("range = %s to %s", start, end)
	}
	if got := normalizePeriod("202512"); got != "2025-12" {
		t.Fatalf("normalizePeriod = %q", got)
	}
	if year, month := parsePeriodYYYYMM("2025-12"); year != 2025 || month != 12 {
		t.Fatalf("parsePeriodYYYYMM = %d-%d", year, month)
	}
}
