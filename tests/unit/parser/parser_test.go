package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"financeqa/internal/parser"
)

func TestExtractMetadataForBankStatementSample(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "交易查询，模拟财务科技有限公司，125922640010001，人民币，20260101-20260228，共93笔_20260401121229.xlsx")
	requireFixture(t, path)
	meta, err := parser.ExtractMetadata(path)
	if err != nil {
		t.Fatalf("ExtractMetadata failed: %v", err)
	}

	if meta.ReportType != "bank_statement" {
		t.Fatalf("report type = %q, want bank_statement", meta.ReportType)
	}
	if meta.Company != "模拟财务科技有限公司" {
		t.Fatalf("company = %q", meta.Company)
	}
	if meta.PeriodStart != "2026-01" || meta.PeriodEnd != "2026-02" {
		t.Fatalf("periods = %s to %s", meta.PeriodStart, meta.PeriodEnd)
	}
}

func TestParseBankStatementSample(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "交易查询，模拟财务科技有限公司，125922640010001，人民币，20260101-20260228，共93笔_20260401121229.xlsx")
	requireFixture(t, path)
	result, err := parser.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if len(result.Data) != 93 {
		t.Fatalf("record count = %d, want 93", len(result.Data))
	}
	first := result.Data[0]
	if got := first["transaction_date"]; got != "2026-01-02" {
		t.Fatalf("first transaction date = %v", got)
	}
	if got := first["debit_amount"]; got != 30.63 {
		t.Fatalf("first debit_amount = %v", got)
	}
	if got := first["counterparty_name"]; got != "网上电子汇划收入" {
		t.Fatalf("first counterparty_name = %v", got)
	}
}

func TestParseIncomeStatementSample(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "模拟财务2026.2利润表.xls")
	requireFixture(t, path)
	result, err := parser.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if result.Metadata.ReportType != "income_statement" {
		t.Fatalf("report type = %q", result.Metadata.ReportType)
	}
	if len(result.Data) < 5 {
		t.Fatalf("record count = %d, want at least 5", len(result.Data))
	}
	foundRevenue := false
	for _, row := range result.Data {
		if row["item_name"] == "一、营业收入" {
			foundRevenue = true
			break
		}
	}
	if !foundRevenue {
		t.Fatal("expected parsed income statement to include 营业收入")
	}
}

func TestParseBalanceSheetAndBalanceDetailSamples(t *testing.T) {
	tests := []struct {
		name string
		file string
		want string
	}{
		{name: "balance sheet", file: "模拟财务2026.2资产负债表.xls", want: "balance_sheet"},
		{name: "balance detail", file: "模拟财务2026.1-2月余额表-end.xls", want: "balance_detail"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join("..", "..", "testdata", tc.file)
			requireFixture(t, path)
			result, err := parser.ParseFile(path)
			if err != nil {
				t.Fatalf("ParseFile failed: %v", err)
			}
			if result.Metadata.ReportType != tc.want {
				t.Fatalf("report type = %q, want %q", result.Metadata.ReportType, tc.want)
			}
			if len(result.Data) == 0 {
				t.Fatal("expected parsed rows")
			}
		})
	}
}

func requireFixture(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Skipf("fixture not present: %v", err)
	}
}
