package query

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestBuildCoreMetricCoverageAppliesTruncationAndNoDataRules(t *testing.T) {
	cases := []struct {
		name           string
		requestedFrom  string
		requestedTo    string
		availableTo    string
		wantActualFrom string
		wantActualTo   string
		wantTruncated  bool
		wantHasData    bool
	}{
		{
			name:           "truncate to latest available period",
			requestedFrom:  "2026-01",
			requestedTo:    "2026-04",
			availableTo:    "2026-03",
			wantActualFrom: "2026-01",
			wantActualTo:   "2026-03",
			wantTruncated:  true,
			wantHasData:    true,
		},
		{
			name:           "no data when requested range starts after latest available period",
			requestedFrom:  "2026-05",
			requestedTo:    "2026-06",
			availableTo:    "2026-03",
			wantActualFrom: "",
			wantActualTo:   "",
			wantTruncated:  false,
			wantHasData:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildCoreMetricCoverage(tc.requestedFrom, tc.requestedTo, tc.availableTo)
			if got.ActualFrom != tc.wantActualFrom || got.ActualTo != tc.wantActualTo {
				t.Fatalf("buildCoreMetricCoverage(%s,%s,%s) actual=%s~%s, want %s~%s", tc.requestedFrom, tc.requestedTo, tc.availableTo, got.ActualFrom, got.ActualTo, tc.wantActualFrom, tc.wantActualTo)
			}
			if got.Truncated != tc.wantTruncated {
				t.Fatalf("buildCoreMetricCoverage(%s,%s,%s) truncated=%t, want %t", tc.requestedFrom, tc.requestedTo, tc.availableTo, got.Truncated, tc.wantTruncated)
			}
			if got.HasData != tc.wantHasData {
				t.Fatalf("buildCoreMetricCoverage(%s,%s,%s) hasData=%t, want %t", tc.requestedFrom, tc.requestedTo, tc.availableTo, got.HasData, tc.wantHasData)
			}
		})
	}
}

func TestResolveCoreMetricCoverageIgnoresContractOnlyMonths(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "contract-only-coverage.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE fin_contracts (
			contract_id TEXT PRIMARY KEY,
			customer_name TEXT,
			contract_content TEXT
		)`,
		`CREATE TABLE fin_fund_income (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			contract_id TEXT,
			year_month TEXT
		)`,
		`INSERT INTO fin_contracts(contract_id, customer_name, contract_content)
		 VALUES ('C1', '测试客户有限公司', '测试项目')`,
		`INSERT INTO fin_fund_income(contract_id, year_month)
		 VALUES ('C1', '2025-10')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec stmt failed: %v", err)
		}
	}

	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	coverage := engine.resolveCoreMetricCoverage("2025-10", "2025-10")
	if coverage.AvailableTo != "" {
		t.Fatalf("available_to = %q, want empty without financial tables", coverage.AvailableTo)
	}
	if coverage.HasData {
		t.Fatalf("has_data = true, want false without financial tables")
	}
}

func TestResolveCoreMetricCoverageUsesJournalVoucherDateWhenPeriodMissing(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "journal-date-coverage.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE journal (
			company TEXT,
			period TEXT,
			voucher_date TEXT
		)`,
		`INSERT INTO journal(company, period, voucher_date)
		 VALUES ('测试公司', NULL, '2026-02-15')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec stmt failed: %v", err)
		}
	}

	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	coverage := engine.resolveCoreMetricCoverage("2026-02", "2026-02")
	if coverage.AvailableTo != "2026-02" {
		t.Fatalf("available_to = %q, want 2026-02 from voucher_date", coverage.AvailableTo)
	}
	if !coverage.HasData {
		t.Fatalf("has_data = false, want true with journal voucher_date")
	}
}

func TestResolveCoreMetricCoverageForRequestUsesMetricSpecificAvailability(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "metric-specific-coverage.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE income_statement (
			company TEXT,
			period TEXT,
			item_name TEXT,
			current_amount REAL,
			cumulative_amount REAL
		)`,
		`CREATE TABLE journal (
			company TEXT,
			period TEXT,
			voucher_date TEXT,
			account_code TEXT,
			summary TEXT
		)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount)
		 VALUES ('测试公司', '2026-01', '一、营业收入', 1200, 1200)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount)
		 VALUES ('测试公司', '2026-01', '五、净利润', 800, 800)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount)
		 VALUES ('测试公司', '2026-02', '一、营业收入', 800, 2000)`,
		`INSERT INTO income_statement(company, period, item_name, current_amount, cumulative_amount)
		 VALUES ('测试公司', '2026-02', '五、净利润', 500, 1300)`,
		`INSERT INTO journal(company, period, voucher_date, account_code, summary)
		 VALUES ('测试公司', NULL, '2026-03-31', '66020101', '计提3月工资')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec stmt failed: %v", err)
		}
	}

	engine, err := NewEngine(dbPath, "测试公司")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	revenueCoverage := engine.resolveCoreMetricCoverageForRequest("2026-01", "2026-03", resolveCoreMetricRequest("2026年第一季度营收", "收入"))
	if revenueCoverage.AvailableTo != "2026-02" {
		t.Fatalf("revenue available_to = %q, want 2026-02", revenueCoverage.AvailableTo)
	}
	if !revenueCoverage.Truncated || revenueCoverage.ActualTo != "2026-02" {
		t.Fatalf("revenue coverage = %+v, want truncated to 2026-02", revenueCoverage)
	}

	profitCoverage := engine.resolveCoreMetricCoverageForRequest("2026-01", "2026-03", resolveCoreMetricRequest("2026年第一季度利润", "利润"))
	if profitCoverage.AvailableTo != "2026-03" {
		t.Fatalf("profit available_to = %q, want 2026-03", profitCoverage.AvailableTo)
	}
	if profitCoverage.Truncated {
		t.Fatalf("profit coverage truncated = true, want false: %+v", profitCoverage)
	}

	mixedCoverage := engine.resolveCoreMetricCoverageForRequest("2026-01", "2026-03", resolveCoreMetricRequest("2026年第一季度收入、成本、利润分别是多少？", "核心指标"))
	if mixedCoverage.AvailableTo != "2026-02" {
		t.Fatalf("mixed available_to = %q, want 2026-02", mixedCoverage.AvailableTo)
	}
	if !mixedCoverage.Truncated || mixedCoverage.ActualTo != "2026-02" {
		t.Fatalf("mixed coverage = %+v, want truncated to 2026-02", mixedCoverage)
	}
}
