package integration_test

import (
	"path/filepath"
	"strings"
	"testing"

	"financeqa/internal/accounting"
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
	if note := asString(monthly["tax_inclusion_note"]); note == "" {
		t.Fatalf("journal fallback should expose tax_inclusion_note, got monthly=%v", monthly)
	}
	if !strings.Contains(res.Message, "含税口径") {
		t.Fatalf("journal fallback message should mention tax inclusion note, got %q", res.Message)
	}
}

func TestMonthlySummary_ProfitUsesRevenueMinusCostPlusNonOperatingItems(t *testing.T) {
	dbPath := setupMonthlySummaryAuthorityDB(t)
	runSQLite(t, dbPath, `
DELETE FROM income_statement WHERE company = '模拟财务科技有限公司' AND period = '2026-02';
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','营业收入',1000,1000);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','营业成本',700,700);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','管理费用',150,150);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','营业外收入',20,20);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','营业外支出',5,5);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','净利润',999,999);
`)

	eng, err := query.NewEngine(dbPath, "模拟财务")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer eng.Close()

	res := eng.Query("2026年2月收入、成本、利润分别是多少")
	if !res.Success {
		t.Fatalf("query failed: %s", res.Message)
	}

	monthly := mustMap(t, res.Data["monthly"], "monthly")
	if got := numberFromMap(t, monthly, "revenue"); got != 1000 {
		t.Fatalf("monthly revenue = %.2f, want 1000", got)
	}
	if got := numberFromMap(t, monthly, "cost"); got != 850 {
		t.Fatalf("monthly cost = %.2f, want 850", got)
	}
	if got := numberFromMap(t, monthly, "profit"); got != 165 {
		t.Fatalf("monthly profit = %.2f, want 165 (1000 - 850 + 20 - 5)", got)
	}
	if got := numberFromMap(t, monthly, "non_operating_income"); got != 20 {
		t.Fatalf("monthly non_operating_income = %.2f, want 20", got)
	}
	if got := numberFromMap(t, monthly, "non_operating_expense"); got != 5 {
		t.Fatalf("monthly non_operating_expense = %.2f, want 5", got)
	}
}

func TestQuarterSummary_UsesOpeningBoundaryAwareCumulativeRange(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "quarter_cumulative_authority.db")
	runSQLite(t, dbPath, `
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
CREATE TABLE balance_detail (
  company TEXT,
  year INTEGER,
  period TEXT,
  account_code TEXT,
  account_name TEXT,
  opening_debit REAL,
  opening_credit REAL,
  current_debit REAL,
  current_credit REAL,
  closing_debit REAL,
  closing_credit REAL,
  opening_period TEXT
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

INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2025-12','营业收入',100,3480586.52);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2025-12','营业成本',90,3244959.75);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2025-12','管理费用',5,719387.58);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2025-12','财务费用',1,4857.37);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2025-12','税金及附加',1,771.49);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2025-12','营业外收入',1,0.94);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2025-12','利润总额',5,-489388.73);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2025-12','净利润',5,-489388.73);

INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-01','营业收入',2000000.00,2000000.00);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-01','营业成本',1700000.00,1700000.00);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-01','管理费用',150000.00,150000.00);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-01','财务费用',1000.00,1000.00);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-01','税金及附加',500.00,500.00);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-01','营业外收入',0.00,0.00);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-01','利润总额',148500.00,148500.00);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-01','净利润',148500.00,148500.00);

INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','营业收入',5480586.52,7480586.52);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','营业成本',4704959.75,6404959.75);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','管理费用',719387.58,869387.58);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','财务费用',4857.37,5857.37);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','税金及附加',771.49,1271.49);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','营业外收入',0.19,0.19);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','利润总额',-2756.97,145743.03);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-02','净利润',-2756.97,145743.03);

INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-03','营业收入',3354377.09,10834963.61);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-03','营业成本',3075558.86,9480518.61);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-03','管理费用',163784.07,1033171.65);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-03','财务费用',1078.93,6936.30);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-03','税金及附加',845.15,2116.64);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-03','营业外收入',0.12,0.31);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-03','利润总额',166477.69,312220.72);
INSERT INTO income_statement VALUES ('模拟财务科技有限公司','2026-03','净利润',166477.69,312220.72);

INSERT INTO balance_detail VALUES ('模拟财务科技有限公司', 2026, '2026-03', '1122', '应收账款', 0, 0, 0, 0, 0, 0, '2026-01');
`)

	eng, err := query.NewEngine(dbPath, "模拟财务")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer eng.Close()

	res := eng.Query("2026年第一季度收入、成本、利润分别是多少")
	if !res.Success {
		t.Fatalf("query failed: %s", res.Message)
	}

	book, ok := res.Data["account_view"].(accounting.AccrualPerspective)
	if !ok {
		t.Fatalf("account_view should be accounting.AccrualPerspective, got %T", res.Data["account_view"])
	}
	if got := book.Revenue; got != 10834963.61 {
		t.Fatalf("account revenue = %.2f, want 10834963.61", got)
	}
	if got := book.TotalCost; got != 10522743.20 {
		t.Fatalf("account total cost = %.2f, want 10522743.20", got)
	}
	if got := book.Profit; got != 312220.72 {
		t.Fatalf("account profit = %.2f, want 312220.72", got)
	}

	validation := mustMap(t, res.Data["range_validation"], "range_validation")
	if got := asString(validation["opening_boundary"]); got != "2026-01" {
		t.Fatalf("opening_boundary = %q, want 2026-01", got)
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
