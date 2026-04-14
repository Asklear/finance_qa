package ingest_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"financeqa/internal/ingest"
	"financeqa/internal/parser"
)

func TestImportParsed_BankStatement_IncrementalLatestKeepsHistoryAndDedupes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "policy_bank.db")
	imp := ingest.NewImporter(nil)

	first := parser.ParseResult{
		Metadata: parser.FileMetadata{
			Company:    "测试公司",
			ReportType: "bank_statement",
		},
		Data: []parser.Record{
			{
				"company":              "测试公司",
				"account_no":           "A001",
				"account_name":         "基本户",
				"currency":             "人民币",
				"transaction_date":     "2026-03-01",
				"transaction_time":     "09:00:00",
				"transaction_type":     "转账",
				"debit_amount":         100.0,
				"credit_amount":        0.0,
				"balance":              900.0,
				"summary":              "付款A",
				"counterparty_name":    "供应商A",
				"counterparty_account": "CP-A",
			},
			{
				"company":              "测试公司",
				"account_no":           "A001",
				"account_name":         "基本户",
				"currency":             "人民币",
				"transaction_date":     "2026-03-02",
				"transaction_time":     "10:00:00",
				"transaction_type":     "转账",
				"debit_amount":         200.0,
				"credit_amount":        0.0,
				"balance":              700.0,
				"summary":              "付款B",
				"counterparty_name":    "供应商B",
				"counterparty_account": "CP-B",
			},
		},
	}
	if err := imp.ImportParsed(ctx, dbPath, first, false); err != nil {
		t.Fatalf("first import failed: %v", err)
	}

	second := parser.ParseResult{
		Metadata: parser.FileMetadata{
			Company:    "测试公司",
			ReportType: "bank_statement",
		},
		Data: []parser.Record{
			{
				"company":              "测试公司",
				"account_no":           "A001",
				"account_name":         "基本户",
				"currency":             "人民币",
				"transaction_date":     "2026-03-01",
				"transaction_time":     "09:00:00",
				"transaction_type":     "转账",
				"debit_amount":         100.0,
				"credit_amount":        0.0,
				"balance":              900.0,
				"summary":              "付款A",
				"counterparty_name":    "供应商A",
				"counterparty_account": "CP-A",
			},
		},
	}
	if err := imp.ImportParsed(ctx, dbPath, second, false); err != nil {
		t.Fatalf("second import failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var rows int
	if err := db.QueryRow(`SELECT COUNT(1) FROM bank_statement`).Scan(&rows); err != nil {
		t.Fatalf("count bank_statement failed: %v", err)
	}
	if rows != 2 {
		t.Fatalf("expected 2 rows after incremental_latest import, got %d", rows)
	}
}

func TestImportParsed_IncomeStatement_FullReplaceOverridesIncrementalFlag(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "policy_income.db")
	imp := ingest.NewImporter(nil)

	first := parser.ParseResult{
		Metadata: parser.FileMetadata{
			Company:    "测试公司",
			ReportType: "income_statement",
			PeriodEnd:  "2026-03",
		},
		Data: []parser.Record{
			{"company": "测试公司", "period": "2026-03", "item_name": "一、营业收入", "current_amount": 1000.0, "cumulative_amount": 1000.0},
			{"company": "测试公司", "period": "2026-03", "item_name": "减：营业成本", "current_amount": 500.0, "cumulative_amount": 500.0},
		},
	}
	if err := imp.ImportParsed(ctx, dbPath, first, true); err != nil {
		t.Fatalf("first import failed: %v", err)
	}

	second := parser.ParseResult{
		Metadata: parser.FileMetadata{
			Company:    "测试公司",
			ReportType: "income_statement",
			PeriodEnd:  "2026-03",
		},
		Data: []parser.Record{
			{"company": "测试公司", "period": "2026-03", "item_name": "一、营业收入", "current_amount": 1200.0, "cumulative_amount": 2200.0},
		},
	}
	if err := imp.ImportParsed(ctx, dbPath, second, true); err != nil {
		t.Fatalf("second import failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var rows int
	if err := db.QueryRow(`SELECT COUNT(1) FROM income_statement WHERE company = '测试公司' AND period = '2026-03'`).Scan(&rows); err != nil {
		t.Fatalf("count income_statement failed: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected full_replace policy to keep only latest period rows, got %d", rows)
	}
}

func TestImportParsed_BalanceDetail_IncrementalLatestKeepsLatestByPeriodAccount(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "policy_balance_detail.db")
	imp := ingest.NewImporter(nil)

	first := parser.ParseResult{
		Metadata: parser.FileMetadata{
			Company:     "测试公司",
			ReportType:  "balance_detail",
			PeriodStart: "2026-01",
			PeriodEnd:   "2026-03",
		},
		Data: []parser.Record{
			{
				"company":        "测试公司",
				"year":           2026,
				"period":         "2026-03",
				"account_code":   "100201",
				"account_name":   "银行存款",
				"account_level":  2,
				"opening_debit":  100.0,
				"opening_credit": 0.0,
				"current_debit":  200.0,
				"current_credit": 0.0,
				"closing_debit":  300.0,
				"closing_credit": 0.0,
			},
		},
	}
	if err := imp.ImportParsed(ctx, dbPath, first, false); err != nil {
		t.Fatalf("first import failed: %v", err)
	}

	second := parser.ParseResult{
		Metadata: parser.FileMetadata{
			Company:     "测试公司",
			ReportType:  "balance_detail",
			PeriodStart: "2026-01",
			PeriodEnd:   "2026-03",
		},
		Data: []parser.Record{
			{
				"company":        "测试公司",
				"year":           2026,
				"period":         "2026-03",
				"account_code":   "100201",
				"account_name":   "银行存款",
				"account_level":  2,
				"opening_debit":  101.0,
				"opening_credit": 0.0,
				"current_debit":  201.0,
				"current_credit": 0.0,
				"closing_debit":  302.0,
				"closing_credit": 0.0,
			},
		},
	}
	if err := imp.ImportParsed(ctx, dbPath, second, false); err != nil {
		t.Fatalf("second import failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var rows int
	if err := db.QueryRow(`SELECT COUNT(1) FROM balance_detail WHERE company='测试公司' AND period='2026-03' AND account_code='100201'`).Scan(&rows); err != nil {
		t.Fatalf("count balance_detail failed: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected deduped balance_detail rows=1, got %d", rows)
	}

	var closingDebit float64
	if err := db.QueryRow(`SELECT closing_debit FROM balance_detail WHERE company='测试公司' AND period='2026-03' AND account_code='100201'`).Scan(&closingDebit); err != nil {
		t.Fatalf("query latest balance_detail failed: %v", err)
	}
	if closingDebit != 302.0 {
		t.Fatalf("expected latest row kept with closing_debit=302, got %.2f", closingDebit)
	}
}

func TestImportParsedWithOptions_CompanyOverride_RewritesRowCompany(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "policy_company_override.db")
	imp := ingest.NewImporter(nil)

	result := parser.ParseResult{
		Metadata: parser.FileMetadata{
			Company:    "DefaultCompany",
			ReportType: "income_statement",
			PeriodEnd:  "2026-03",
		},
		Data: []parser.Record{
			{"company": "DefaultCompany", "period": "2026-03", "item_name": "一、营业收入", "current_amount": 123.0, "cumulative_amount": 123.0},
		},
	}
	if err := imp.ImportParsedWithOptions(ctx, dbPath, result, ingest.ImportOptions{
		Incremental:     false,
		CompanyOverride: "南京优集数据科技有限公司",
	}); err != nil {
		t.Fatalf("import with company override failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var got string
	if err := db.QueryRow(`SELECT company FROM income_statement LIMIT 1`).Scan(&got); err != nil {
		t.Fatalf("query imported company failed: %v", err)
	}
	if got != "南京优集数据科技有限公司" {
		t.Fatalf("company=%s, want 南京优集数据科技有限公司", got)
	}
}

func TestImportParsedWithOptions_InvalidCompanyWithoutOverride_ShouldFail(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "policy_company_invalid.db")
	imp := ingest.NewImporter(nil)

	result := parser.ParseResult{
		Metadata: parser.FileMetadata{
			Company:    "20260101-20260331",
			ReportType: "income_statement",
			PeriodEnd:  "2026-03",
		},
		Data: []parser.Record{
			{"company": "20260101-20260331", "period": "2026-03", "item_name": "一、营业收入", "current_amount": 123.0, "cumulative_amount": 123.0},
		},
	}
	err := imp.ImportParsedWithOptions(ctx, dbPath, result, ingest.ImportOptions{Incremental: false})
	if err == nil {
		t.Fatalf("expected invalid company import to fail without override")
	}
}
