package ingest

import (
	"context"
	"os"
	"reflect"
	"testing"

	dbschema "financeqa/internal/db"
	"financeqa/internal/parser"
)

func TestContractWorkbookFileMappingsBuildsSortedUniqueQuarterMappings(t *testing.T) {
	t.Parallel()

	got := contractWorkbookFileMappings(contractImportBundle{
		RevenueRows: []contractRevenueSettlementRow{
			{YearMonth: "2026-02"},
			{YearMonth: "2026-01"},
		},
		FundRows: []contractFundIncomeRow{
			{YearMonth: "2026-03"},
		},
		CostRows: []contractCostSettlementRow{
			{YearMonth: "2026-04"},
			{YearMonth: "bad-period"},
		},
		CostGroupRows: []contractCostSettlementGroupRow{
			{YearMonth: "2026-06"},
		},
	})

	want := []contractWorkbookFileMapping{
		{TableType: "cost-settlements", Period: "2026-Q2", Description: "2026-Q2合同成本表（飞书财务表自动同步）"},
		{TableType: "cost-settlements", Period: "bad-period", Description: "bad-period合同成本表（飞书财务表自动同步）"},
		{TableType: "fund-income", Period: "2026-Q1", Description: "2026-Q1资金收入表（飞书财务表自动同步）"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("contractWorkbookFileMappings() = %#v, want %#v", got, want)
	}
}

func TestContractFinanceMappingPeriodNormalizesMonthsToQuarter(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"":        "",
		"2026-01": "2026-Q1",
		"2026-04": "2026-Q2",
		"2026-07": "2026-Q3",
		"2026-10": "2026-Q4",
		"2026-13": "2026-13",
		"2026-Q1": "2026-Q1",
	}
	for input, want := range cases {
		if got := contractFinanceMappingPeriod(input); got != want {
			t.Fatalf("contractFinanceMappingPeriod(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSourceMetadataHelpersFormatAndMergeDisplayFields(t *testing.T) {
	t.Parallel()

	if got := mergeSourceMetadataStrings([]string{" A ", "B"}, []string{"B", "", "C"}); !reflect.DeepEqual(got, []string{"A", "B", "C"}) {
		t.Fatalf("mergeSourceMetadataStrings() = %#v", got)
	}
	if got := formatMergedWorkbookDisplay(nil, nil, " fallback "); got != "fallback" {
		t.Fatalf("formatMergedWorkbookDisplay fallback = %q", got)
	}
	if got := formatMergedWorkbookDisplay([]string{"收入表.xlsx"}, []string{"26年Q1收入明细"}, ""); got != "《收入表.xlsx》的【26年Q1收入明细】" {
		t.Fatalf("single workbook display = %q", got)
	}
	if got := formatMergedWorkbookDisplay([]string{"A.xlsx", "B.xlsx"}, nil, ""); got != "《A.xlsx》；《B.xlsx》" {
		t.Fatalf("multi workbook display = %q", got)
	}
	if got := workbookDisplayName(" /tmp/优集收入、成本计算表 - 上传.xlsx "); got != "优集收入、成本计算表-上传.xlsx" {
		t.Fatalf("workbookDisplayName() = %q", got)
	}
}

func TestSourceMetadataHelpersSQLitePathAndNullableFileSize(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"":                         true,
		":memory:":                 true,
		"/tmp/financeqa.db":        true,
		"/tmp/financeqa.sqlite3":   true,
		"postgres://localhost/db":  false,
		"host=localhost dbname=x":  false,
		"postgresql-production-db": false,
	}
	for input, want := range cases {
		if got := looksLikeSQLiteImportPath(input); got != want {
			t.Fatalf("looksLikeSQLiteImportPath(%q) = %v, want %v", input, got, want)
		}
	}
	if got := nullableFileSize(0); got != nil {
		t.Fatalf("nullableFileSize(0) = %#v, want nil", got)
	}
	if got := nullableFileSize(128); got != int64(128) {
		t.Fatalf("nullableFileSize(128) = %#v, want 128", got)
	}
}

func TestAnnotateImportedReportSourceWritesFileMappingVersion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := t.TempDir() + "/financeqa.sqlite"
	if err := dbschema.Bootstrap(ctx, dbPath); err != nil {
		t.Fatalf("bootstrap db: %v", err)
	}
	filePath := t.TempDir() + "/招商银行对公户 交易查询 20260101-20260331.xlsx"
	if err := os.WriteFile(filePath, []byte("bank statement fixture"), 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}

	err := annotateImportedReportSource(ctx, dbPath, parser.FileMetadata{
		ReportType:  "bank_statement",
		Company:     "灵犀数据科技有限公司",
		PeriodStart: "2026-01",
		PeriodEnd:   "2026-03",
	}, filePath, ImportOptions{
		CompanyOverride:  "灵犀数据科技有限公司",
		SourceFileName:   "fin-bank-q1.xlsx",
		SourceStorageKey: "assets/flow-fin-ledger/fin-bank-q1.xlsx",
	})
	if err != nil {
		t.Fatalf("annotate imported report source: %v", err)
	}

	db, err := dbschema.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var tableType, period, company, fileName, sourceHash, sourceVersionID string
	if err := db.QueryRowContext(ctx, `
SELECT table_type, period, company, file_name, source_file_hash, source_version_id
FROM fin_file_mappings
WHERE table_type = 'bank-statement'
`).Scan(&tableType, &period, &company, &fileName, &sourceHash, &sourceVersionID); err != nil {
		t.Fatalf("read fin_file_mappings: %v", err)
	}
	if tableType != "bank-statement" || period != "2026-Q1" || company != "灵犀数据科技有限公司" || fileName != "fin-bank-q1.xlsx" {
		t.Fatalf("unexpected mapping row: type=%q period=%q company=%q file=%q", tableType, period, company, fileName)
	}
	if len(sourceHash) != 12 {
		t.Fatalf("source_file_hash length = %d, want 12 (%q)", len(sourceHash), sourceHash)
	}
	if sourceVersionID != "fin-bank-q1.xlsx:"+sourceHash {
		t.Fatalf("source_version_id = %q, want %q", sourceVersionID, "fin-bank-q1.xlsx:"+sourceHash)
	}
}
