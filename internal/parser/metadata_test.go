package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectReportTypeRecognizesKnownFilenames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "bank", path: "交易查询，南京优集数据科技有限公司，123，人民币，20260101-20260131.xlsx", want: "bank_statement"},
		{name: "journal", path: "南京优集2026年序时账.xls", want: "journal"},
		{name: "balance detail", path: "2025年发生额及余额表.xls", want: "balance_detail"},
		{name: "balance sheet", path: "南京优集2025.12资产负债表.xls", want: "balance_sheet"},
		{name: "income statement", path: "南京优集2025.12利润表.xls", want: "income_statement"},
		{name: "unknown", path: "notes.txt", want: "unknown"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := DetectReportType(tc.path); got != tc.want {
				t.Fatalf("DetectReportType(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestExtractMetadataParsesBankStatementFilename(t *testing.T) {
	t.Parallel()

	path := writeTempNamedFile(t, "交易查询，南京优集数据科技有限公司，125922640010001，人民币，20250801-20251231，共73笔_20260421180110.xlsx")
	meta, err := ExtractMetadata(path)
	if err != nil {
		t.Fatalf("ExtractMetadata failed: %v", err)
	}

	if meta.ReportType != "bank_statement" {
		t.Fatalf("report type = %q", meta.ReportType)
	}
	if meta.Company != "南京优集数据科技有限公司" {
		t.Fatalf("company = %q", meta.Company)
	}
	if meta.PeriodStart != "2025-08" || meta.PeriodEnd != "2025-12" {
		t.Fatalf("periods = %s to %s", meta.PeriodStart, meta.PeriodEnd)
	}
	if meta.FilePath != path {
		t.Fatalf("file path = %q, want %q", meta.FilePath, path)
	}
}

func TestExtractMetadataParsesIncomeStatementFilename(t *testing.T) {
	t.Parallel()

	path := writeTempNamedFile(t, "南京优集2025.12利润表.xls")
	meta, err := ExtractMetadata(path)
	if err != nil {
		t.Fatalf("ExtractMetadata failed: %v", err)
	}

	if meta.ReportType != "income_statement" {
		t.Fatalf("report type = %q", meta.ReportType)
	}
	if meta.Company != "南京优集" {
		t.Fatalf("company = %q", meta.Company)
	}
	if meta.PeriodStart != "2025-12" || meta.PeriodEnd != "2025-12" {
		t.Fatalf("periods = %s to %s", meta.PeriodStart, meta.PeriodEnd)
	}
}

func TestExtractMetadataDefaultsCompanyWhenFilenameHasNoCompany(t *testing.T) {
	t.Parallel()

	path := writeTempNamedFile(t, "2025.12利润表.xls")
	meta, err := ExtractMetadata(path)
	if err != nil {
		t.Fatalf("ExtractMetadata failed: %v", err)
	}

	if meta.Company != "DefaultCompany" {
		t.Fatalf("company = %q", meta.Company)
	}
	if meta.PeriodStart != "2025-12" || meta.PeriodEnd != "2025-12" {
		t.Fatalf("periods = %s to %s", meta.PeriodStart, meta.PeriodEnd)
	}
}

func TestParseFileRejectsUnsupportedFileType(t *testing.T) {
	t.Parallel()

	path := writeTempNamedFile(t, "notes.txt")
	_, err := ParseFile(path)
	if err == nil {
		t.Fatal("expected ParseFile to reject unsupported file")
	}
}

func writeTempNamedFile(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("fixture"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}
