package ingest

import (
	"reflect"
	"testing"
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
