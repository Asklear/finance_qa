package accounting

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestAccountClassificationAndCashHelpers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		code string
		want AccountCategory
	}{
		{code: "", want: ""},
		{code: "1002", want: CategoryAsset},
		{code: "2202", want: CategoryLiability},
		{code: "4001", want: CategoryCost},
		{code: "6001", want: CategoryRevenue},
		{code: "6301", want: CategoryRevenue},
		{code: "6602", want: CategoryExpense},
	}
	for _, tc := range cases {
		if got := CategoryForCode(tc.code); got != tc.want {
			t.Fatalf("CategoryForCode(%q) = %q, want %q", tc.code, got, tc.want)
		}
	}
	if got := NormalDirection(CategoryAsset); got != "借" {
		t.Fatalf("asset normal direction = %q", got)
	}
	if got := NormalDirection(CategoryRevenue); got != "贷" {
		t.Fatalf("revenue normal direction = %q", got)
	}
	if !IsCashRelated("100201") || !IsCashRelated("100101") || IsCashRelated("112201") {
		t.Fatal("cash account helper classified accounts incorrectly")
	}
	if got := lastDayOfMonth("2024-02"); got != "2024-02-29" {
		t.Fatalf("lastDayOfMonth leap year = %q", got)
	}
}

func TestCalculatorComputesJournalCashAccrualAndBalance(t *testing.T) {
	db := openAccountingTestDB(t)
	seedAccountingTestData(t, db)

	calc := NewCalculator(db)
	monthly, err := calc.ComputeMonthlyFromJournal("优集科技", 2026, 3)
	if err != nil {
		t.Fatalf("ComputeMonthlyFromJournal: %v", err)
	}
	if monthly.Revenue != 900 || monthly.Cost != 450 || monthly.Profit != 450 {
		t.Fatalf("monthly metrics = %+v, want revenue=900 cost=450 profit=450", monthly)
	}

	statement, err := calc.ComputeIncomeStatement("优集科技", 2026, 3)
	if err != nil {
		t.Fatalf("ComputeIncomeStatement: %v", err)
	}
	if statement.Revenue != 900 || statement.Cost != 200 || statement.AdminExpense != 250 || statement.OperatingProfit != 450 {
		t.Fatalf("income statement = %+v", statement)
	}

	cash, err := calc.ComputeCashFlow("优集科技", "2026-03", "2026-03")
	if err != nil {
		t.Fatalf("ComputeCashFlow: %v", err)
	}
	if cash.Income != 500 || cash.Expense != 125 || cash.Net != 375 {
		t.Fatalf("cash flow = %+v", cash)
	}

	dual, err := calc.ComputeDualPerspective("优集科技", 2026, 3)
	if err != nil {
		t.Fatalf("ComputeDualPerspective: %v", err)
	}
	if dual.Accrual.Revenue != 1200 || dual.Accrual.TotalCost != 800 || dual.Accrual.Profit != 400 {
		t.Fatalf("dual accrual = %+v", dual.Accrual)
	}
	if dual.Cash.Net != 375 {
		t.Fatalf("dual cash net = %.2f, want 375", dual.Cash.Net)
	}

	balanceRows, err := calc.ComputeBalanceFromJournal("优集科技", 2026, 3)
	if err != nil {
		t.Fatalf("ComputeBalanceFromJournal: %v", err)
	}
	ar := findBalanceRow(t, balanceRows, "112201")
	if ar.OpeningDebit != 100 || ar.CurrentDebit != 30 || ar.CurrentCred != 80 || ar.ClosingDebit != 50 || ar.ClosingCred != 0 {
		t.Fatalf("AR balance row = %+v", ar)
	}
}

func openAccountingTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "accounting.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	stmts := []string{
		`CREATE TABLE journal (
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
		)`,
		`CREATE TABLE bank_statement (
			company TEXT,
			transaction_date TEXT,
			debit_amount REAL,
			credit_amount REAL
		)`,
		`CREATE TABLE income_statement (
			company TEXT,
			period TEXT,
			item_name TEXT,
			current_amount REAL
		)`,
		`CREATE TABLE balance_detail (
			company TEXT,
			year INTEGER,
			account_code TEXT,
			account_name TEXT,
			opening_debit REAL,
			opening_credit REAL
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create accounting schema: %v", err)
		}
	}
	return db
}

func seedAccountingTestData(t *testing.T, db *sql.DB) {
	t.Helper()

	stmts := []string{
		`INSERT INTO journal VALUES ('优集科技','2026-03','2026-03-01','V1','600101','主营业务收入','客户服务收入','贷',1000,0,1000,'客户A')`,
		`INSERT INTO journal VALUES ('优集科技','2026-03','2026-03-02','V2','600101','主营业务收入','销售折让','借',100,100,0,'客户A')`,
		`INSERT INTO journal VALUES ('优集科技','2026-03','2026-03-03','V3','660201','管理费用','办公室费用','借',300,300,0,'供应商A')`,
		`INSERT INTO journal VALUES ('优集科技','2026-03','2026-03-04','V4','660201','管理费用','费用冲回','贷',50,0,50,'供应商A')`,
		`INSERT INTO journal VALUES ('优集科技','2026-03','2026-03-05','V5','640101','营业成本','供应商成本','借',200,200,0,'供应商B')`,
		`INSERT INTO journal VALUES ('优集科技','2026-03','2026-03-06','V6','600101','主营业务收入','期间损益结转','贷',999,0,999,'结转')`,
		`INSERT INTO journal VALUES ('优集科技','2026-03','2026-03-07','V7','112201','应收账款','新增应收','借',30,30,0,'客户B')`,
		`INSERT INTO journal VALUES ('优集科技','2026-03','2026-03-08','V8','112201','应收账款','客户回款','贷',80,0,80,'客户B')`,
		`INSERT INTO bank_statement VALUES ('优集科技','2026-03-10',125,500)`,
		`INSERT INTO income_statement VALUES ('优集科技','2026-03','营业收入',1200)`,
		`INSERT INTO income_statement VALUES ('优集科技','2026-03','净利润',400)`,
		`INSERT INTO balance_detail VALUES ('优集科技',2026,'112201','应收账款',100,0)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed accounting data: %v", err)
		}
	}
}

func findBalanceRow(t *testing.T, rows []BalanceDetailRow, code string) BalanceDetailRow {
	t.Helper()
	for _, row := range rows {
		if row.AccountCode == code {
			return row
		}
	}
	t.Fatalf("missing balance row %s in %+v", code, rows)
	return BalanceDetailRow{}
}
