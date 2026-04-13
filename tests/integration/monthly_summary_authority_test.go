package integration_test

import (
	"path/filepath"
	"testing"

	"financeqa/internal/query"

	_ "modernc.org/sqlite"
)

func TestMonthlySummary_PrefersIncomeStatementCurrentAmount(t *testing.T) {
	dbPath := setupMonthlySummaryAuthorityDB(t)
	eng, err := query.NewEngine(dbPath, "模拟财务")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer eng.Close()

	res := eng.Query("2026年2月经营状况")
	if !res.Success {
		t.Fatalf("query failed: %s", res.Message)
	}

	monthly := mustMap(t, res.Data["monthly"], "monthly")
	if source := asString(monthly["source"]); source != "income_statement" {
		t.Fatalf("monthly source = %q, want income_statement", source)
	}

	if got := numberFromMap(t, monthly, "revenue"); got != 1000 {
		t.Fatalf("monthly revenue = %.2f, want 1000", got)
	}
	if got := numberFromMap(t, monthly, "cost"); got != 950 {
		t.Fatalf("monthly cost = %.2f, want 950", got)
	}
	if got := numberFromMap(t, monthly, "profit"); got != 50 {
		t.Fatalf("monthly profit = %.2f, want 50", got)
	}

	book := mustMap(t, res.Data["财务做账口径(看利润)"], "财务做账口径(看利润)")
	if got := numberFromMap(t, book, "营业收入"); got != 1000 {
		t.Fatalf("book revenue = %.2f, want 1000", got)
	}
	if got := numberFromMap(t, book, "营业成本及费用"); got != 950 {
		t.Fatalf("book total cost = %.2f, want 950", got)
	}
	if got := numberFromMap(t, book, "账面利润"); got != 50 {
		t.Fatalf("book profit = %.2f, want 50", got)
	}
}

func TestMonthlySummary_FallbackToJournalWhenIncomeStatementIncomplete(t *testing.T) {
	dbPath := setupMonthlySummaryAuthorityDB(t)
	runSQLite(t, dbPath, `
DELETE FROM income_statement WHERE company = '模拟财务科技有限公司' AND period = '2026-02' AND item_name LIKE '%净利润%';
`)

	eng, err := query.NewEngine(dbPath, "模拟财务")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer eng.Close()

	res := eng.Query("2026年2月经营状况")
	if !res.Success {
		t.Fatalf("query failed: %s", res.Message)
	}

	monthly := mustMap(t, res.Data["monthly"], "monthly")
	if source := asString(monthly["source"]); source != "journal_fallback" {
		t.Fatalf("monthly source = %q, want journal_fallback", source)
	}

	if got := numberFromMap(t, monthly, "revenue"); got != 2000 {
		t.Fatalf("monthly revenue = %.2f, want 2000", got)
	}
	if got := numberFromMap(t, monthly, "cost"); got != 1200 {
		t.Fatalf("monthly cost = %.2f, want 1200", got)
	}
	if got := numberFromMap(t, monthly, "profit"); got != 800 {
		t.Fatalf("monthly profit = %.2f, want 800", got)
	}
}

func setupMonthlySummaryAuthorityDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "monthly_summary_authority.db")

	sql := `
CREATE TABLE income_statement (
  company TEXT,
  period TEXT,
  item_name TEXT,
  current_amount REAL,
  cumulative_amount REAL
);
CREATE TABLE bank_statement (
  company TEXT,
  transaction_date TEXT,
  credit_amount REAL,
  debit_amount REAL,
  counterparty_name TEXT,
  summary TEXT
);
CREATE TABLE journal (
  company TEXT,
  period TEXT,
  voucher_date TEXT,
  voucher_no TEXT,
  account_code TEXT,
  account_name TEXT,
  summary TEXT,
  direction TEXT,
  amount REAL,
  debit_amount REAL,
  credit_amount REAL,
  counterparty TEXT
);
CREATE TABLE balance_sheet (
  company TEXT,
  period TEXT,
  account_code TEXT,
  account_name TEXT,
  opening_balance REAL,
  closing_balance REAL
);
CREATE TABLE cas_mapping (
  standard_code TEXT PRIMARY KEY,
  standard_name TEXT NOT NULL,
  category TEXT
);

INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','营业收入',1000,1000);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','营业成本',800,800);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','管理费用',150,150);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','净利润',50,50);

INSERT INTO bank_statement VALUES ('模拟财务科技有限公司','2026-02-10',1200,300,'客户A','收付');

INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-02','2026-02-15','V001','600101','主营业务收入','确认收入','贷',2000,0,2000,'客户A');
INSERT INTO journal VALUES ('模拟财务科技有限公司','2026-02','2026-02-16','V002','660201','管理费用','确认费用','借',1200,1200,0,'供应商A');
`
	runSQLite(t, dbPath, sql)
	return dbPath
}

